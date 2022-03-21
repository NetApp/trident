// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This package provides a high-level interface to the E-series Web Services Proxy REST API.
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
)

const maxNameLength = 30
const NullRef = "0000000000000000000000000000000000000000"
const hostMappingType = "host"
const hostGroupMappingType = "cluster"
const defaultPoolSearchPattern = ".+"

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {
	// Web Proxy Services Info
	WebProxyHostname  string
	WebProxyPort      string
	WebProxyUseHTTP   bool
	WebProxyVerifyTLS bool
	Username          string
	Password          string

	// Array Info
	ControllerA   string
	ControllerB   string
	PasswordArray string

	// Options
	PoolNameSearchPattern string
	DebugTraceFlags       map[string]bool

	// Host Connectivity
	HostDataIP string // for iSCSI with multipathing this can be either IP or host

	// Internal Config Variables
	ArrayID                       string // Unique ID for array once added to web proxy services
	CompiledPoolNameSearchPattern *regexp.Regexp

	// Storage protocol of the driver (iSCSI, FC, etc)
	Protocol    string
	AccessGroup string
	HostType    string

	DriverName    string
	Telemetry     map[string]string
	ConfigVersion int
}

// Client is the object to use for interacting with the E-series API.
type Client struct {
	config *ClientConfig
	m      *sync.Mutex
}

// NewAPIClient is a factory method for creating a new instance.
func NewAPIClient(ctx context.Context, config ClientConfig) *Client {
	c := &Client{
		config: &config,
		m:      &sync.Mutex{},
	}

	// Initialize internal config variables
	c.config.ArrayID = ""

	compiledRegex, err := regexp.Compile(c.config.PoolNameSearchPattern)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"PoolNameSearchPattern": c.config.PoolNameSearchPattern,
			"DefaultSearchPattern":  defaultPoolSearchPattern,
			"Error":                 err,
		}).Warn("Could not compile PoolNameSearchPattern regular expression, using default pattern.")
		compiledRegex, _ = regexp.Compile(".+")
	}
	c.config.CompiledPoolNameSearchPattern = compiledRegex

	return c
}

func (d Client) makeVolumeTags() []VolumeTag {

	return []VolumeTag{
		{"IF", d.config.Protocol},
		{"version", d.config.Telemetry["version"]},
		{"platform", tridentconfig.OrchestratorTelemetry.Platform},
		{"platformVersion", tridentconfig.OrchestratorTelemetry.PlatformVersion},
		{"plugin", d.config.Telemetry["plugin"]},
		{"storagePrefix", d.config.Telemetry["storagePrefix"]},
	}
}

// InvokeAPI makes a REST call to the Web Services Proxy. The body must be a marshaled JSON byte array (or nil).
// The method is the HTTP verb (i.e. GET, POST, ...).  The resource path is appended to the base URL to identify
// the desired server resource; it should start with '/'.
func (d Client) InvokeAPI(
	ctx context.Context, requestBody []byte, method string, resourcePath string,
) (*http.Response, []byte, error) {

	// Default to secure connection
	scheme, port := "https", "8443"

	// Allow insecure override
	if d.config.WebProxyUseHTTP {
		scheme, port = "http", "8080"
	}

	// Allow port override
	if d.config.WebProxyPort != "" {
		port = d.config.WebProxyPort
	}

	// Build URL
	url := scheme + "://" + d.config.WebProxyHostname + ":" + port + "/devmgr/v2/storage-systems/" + d.config.ArrayID + resourcePath

	var request *http.Request
	var err error
	var prettyResponseBuffer bytes.Buffer

	// Create the request
	if requestBody == nil {
		request, err = http.NewRequestWithContext(ctx, method, url, nil)
	} else {
		request, err = http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", fmt.Sprint(ctx.Value(ContextKeyRequestID)))
	request.SetBasicAuth(d.config.Username, d.config.Password)

	// Log the request
	if d.config.DebugTraceFlags["api"] {
		// Suppress the empty POST body since it contains the array password
		utils.LogHTTPRequest(request, []byte("<suppressed>"), true)
	}

	// Send the request
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !d.config.WebProxyVerifyTLS, // Allow certificate validation override
			MinVersion:         tridentconfig.MinClientTLSVersion,
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(tridentconfig.StorageAPITimeoutSeconds * time.Second),
	}

	response, err := client.Do(request)
	if err != nil {
		Logc(ctx).Warnf("Error communicating with Web Services Proxy. %v", err)
		return nil, nil, err
	}
	defer response.Body.Close()

	responseBody := []byte{}
	if err == nil {

		responseBody, err = ioutil.ReadAll(response.Body)

		if method == "GET" && resourcePath == "/volumes" {
			// Suppress the potentially huge GET /volumes body unless asked for explicitly
			if d.config.DebugTraceFlags["api_get_volumes"] {
				if err := json.Indent(&prettyResponseBuffer, responseBody, "", "  "); err != nil {
					Logc(ctx).Errorf("Could not format API request for logging; %v", err)
				} else {
					utils.LogHTTPResponse(ctx, response, prettyResponseBuffer.Bytes(), false)
				}
			} else if d.config.DebugTraceFlags["api"] {
				utils.LogHTTPResponse(ctx, response, []byte("<suppressed>"), true)
			}
		} else {
			if d.config.DebugTraceFlags["api"] {
				if err := json.Indent(&prettyResponseBuffer, responseBody, "", "  "); err != nil {
					Logc(ctx).Errorf("Could not format API request for logging; %v", err)
				} else {
					utils.LogHTTPResponse(ctx, response, prettyResponseBuffer.Bytes(), false)
				}
			}
		}
	}

	return response, responseBody, err
}

// AboutInfo retrieves information about this E-Series system
func (d Client) AboutInfo(ctx context.Context) (*AboutResponse, error) {

	// Default to secure connection
	scheme, port := "https", "8443"

	// Allow insecure override
	if d.config.WebProxyUseHTTP {
		scheme, port = "http", "8080"
	}

	// Allow port override
	if d.config.WebProxyPort != "" {
		port = d.config.WebProxyPort
	}

	// Build URL
	url := scheme + "://" + d.config.WebProxyHostname + ":" + port + "/devmgr/utils/about"

	var request *http.Request
	var err error
	var prettyResponseBuffer bytes.Buffer

	// Create the request
	request, err = http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("accept", "application/json")
	request.Header.Set("X-Request-ID", fmt.Sprint(ctx.Value(ContextKeyRequestID)))
	request.SetBasicAuth(d.config.Username, d.config.Password)

	// Log the request
	if d.config.DebugTraceFlags["api"] {
		utils.LogHTTPRequest(request, []byte("<suppressed>"), true)
	}

	// Send the request
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !d.config.WebProxyVerifyTLS, // Allow certificate validation override
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   tridentconfig.StorageAPITimeoutSeconds * time.Second,
	}
	response, err := client.Do(request)
	if err != nil {
		Logc(ctx).Warnf("Error communicating with Web Services Proxy. %v", err)
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, Error{
			Code:    response.StatusCode,
			Message: "could not get about information",
		}
	}

	responseBody := []byte{}
	if err == nil {
		responseBody, err = ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		if d.config.DebugTraceFlags["api"] {
			if err := json.Indent(&prettyResponseBuffer, responseBody, "", "  "); err != nil {
				Logc(ctx).Errorf("Could not format API request for logging; %v", err)
			} else {
				utils.LogHTTPResponse(ctx, response, prettyResponseBuffer.Bytes(), false)
			}
		}
	}

	aboutInfo := &AboutResponse{}
	err = json.Unmarshal(responseBody, &aboutInfo)
	if err != nil {
		return nil, fmt.Errorf("could not parse response: %s. %v", string(responseBody), err)
	}

	return aboutInfo, err
}

// Connect to the E-Series system via the Web Services Proxy or the onboard system, as appropriate
func (d Client) Connect(ctx context.Context) (string, error) {
	aboutInfo, err := d.AboutInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("could not determine proxy status: %v", err)
	}
	if aboutInfo == nil {
		return "", fmt.Errorf("could not determine proxy status: aboutInfo was empty")
	}
	if aboutInfo.RunningAsProxy {
		return d.connectUsingPOST(ctx)
	}
	return d.connectUsingGET(ctx)
}

// connectUsingPOST connects to the Web Services Proxy via POST
func (d Client) connectUsingPOST(ctx context.Context) (string, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Connect",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Connect")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Connect")
	}

	// Send a login/connect request for array to web services proxy
	request := MsgConnect{[]string{d.config.ControllerA, d.config.ControllerB}, d.config.PasswordArray}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	// Send the message (using HTTP POST)
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", "")
	if err != nil {
		return "", fmt.Errorf("could not log into the Web Services Proxy: %v", err)
	}

	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return "", Error{
			Code:    response.StatusCode,
			Message: "could not add storage array to Web Services Proxy",
		}
	}

	// Parse JSON data
	responseData := MsgConnectResponse{}
	if err := json.Unmarshal(responseBody, &responseData); err != nil {
		return "", fmt.Errorf("could not parse connect response: %s; %v", string(responseBody), err)
	}

	if responseData.ArrayID == "" {
		return "", errors.New("invalid ArrayID received from Web Services Proxy")
	}

	d.config.ArrayID = responseData.ArrayID
	AlreadyRegistered := responseData.AlreadyExists

	Logc(ctx).WithFields(log.Fields{
		"ArrayID":           d.config.ArrayID,
		"AlreadyRegistered": AlreadyRegistered,
	}).Debug("Connected to Web Services Proxy.")

	return d.config.ArrayID, nil
}

// connectUsingGET connects to the onboard Web Services via GET
func (d Client) connectUsingGET(ctx context.Context) (string, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Connect",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Connect")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Connect")
	}

	// Send a login/connect request for array to web services proxy
	request := MsgConnect{[]string{d.config.ControllerA, d.config.ControllerB}, d.config.PasswordArray}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	// Send the message (using HTTP GET)
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "GET", "")
	if err != nil {
		return "", fmt.Errorf("could not log into the Web Services Proxy: %v", err)
	}

	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return "", Error{
			Code:    response.StatusCode,
			Message: "could not add storage array to Web Services Proxy",
		}
	}

	// Parse JSON data (the result is wrapped in a list, unlike the POST which is a single element)
	responseData := []MsgConnectResponse{}
	if err := json.Unmarshal(responseBody, &responseData); err != nil {
		return "", fmt.Errorf("could not parse connect response: %s; %v", string(responseBody), err)
	}

	if len(responseData) < 1 {
		return "", errors.New("invalid response received from Web Services Proxy")
	}

	if responseData[0].ArrayID == "" {
		return "", errors.New("invalid ArrayID received from Web Services Proxy")
	}

	d.config.ArrayID = responseData[0].ArrayID
	AlreadyRegistered := responseData[0].AlreadyExists

	Logc(ctx).WithFields(log.Fields{
		"ArrayID":           d.config.ArrayID,
		"AlreadyRegistered": AlreadyRegistered,
	}).Debug("Connected to Web Services Proxy.")

	return d.config.ArrayID, nil
}

// GetStorageSystem returns a struct detailing the storage system.
func (d Client) GetStorageSystem(ctx context.Context) (*StorageSystem, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetStorageSystem",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetStorageSystem")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetStorageSystem")
	}

	// Query array
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/")
	if err != nil {
		return nil, errors.New("could not read storage system")
	}

	if response.StatusCode != http.StatusOK {
		return nil, Error{
			Code:    response.StatusCode,
			Message: "could not read storage system",
		}
	}

	storageSystem := StorageSystem{}
	err = json.Unmarshal(responseBody, &storageSystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse storage system data: %s. %v", string(responseBody), err)
	}

	Logc(ctx).WithField("Name", storageSystem.Name).Debug("Read storage system.")

	return &storageSystem, nil
}

// GetChassisSerialNumber returns the chassis serial number for this storage system.
func (d Client) GetChassisSerialNumber(ctx context.Context) (string, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetChassisSerialNumber",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetChassisSerialNumber")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetChassisSerialNumber")
	}

	storageSystem, err := d.GetStorageSystem(ctx)
	if err != nil {
		return "", err
	}

	if storageSystem.ChassisSerialNumber == "" {
		return "", errors.New("chassis serial number is blank")
	}

	return storageSystem.ChassisSerialNumber, nil
}

// GetVolumePools reads all pools on the array, including volume groups and dynamic disk pools. It then
// filters them based on several selection parameters and returns the ones that match.
func (d Client) GetVolumePools(
	ctx context.Context, mediaType string, minFreeSpaceBytes uint64, poolName string,
) ([]VolumeGroupEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":            "GetVolumePools",
			"Type":              "Client",
			"mediaType":         mediaType,
			"minFreeSpaceBytes": minFreeSpaceBytes,
			"poolName":          poolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolumePools")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumePools")
	}

	// Get the storage pools (includes volume RAID groups and dynamic disk pools)
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/storage-pools")
	if err != nil {
		return nil, fmt.Errorf("could not get storage pools: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, Error{
			Code:    response.StatusCode,
			Message: "could not get storage pools",
		}
	}

	// Parse JSON data
	allPools := make([]VolumeGroupEx, 0)
	if err := json.Unmarshal(responseBody, &allPools); err != nil {
		return nil, fmt.Errorf("could not parse storage pool data: %s; %v", string(responseBody), err)
	}

	// Return only pools that match the requested criteria
	matchingPools := make([]VolumeGroupEx, 0)
	for _, pool := range allPools {

		Logc(ctx).WithFields(log.Fields{
			"Name": pool.Label,
			"Pool": pool,
		}).Debug("Considering pool.")

		// Pool must match regex from config
		if !d.config.CompiledPoolNameSearchPattern.MatchString(pool.Label) {
			Logc(ctx).WithFields(log.Fields{"Name": pool.Label}).Debug("Pool does not match search pattern.")
			continue
		}

		// Pool must be online
		if pool.IsOffline {
			Logc(ctx).WithFields(log.Fields{"Name": pool.Label}).Debug("Pool is offline.")
			continue
		}

		// Pool name
		if poolName != "" {
			if poolName != pool.Label {
				Logc(ctx).WithFields(log.Fields{
					"Name":          pool.Label,
					"RequestedName": poolName,
				}).Debug("Pool does not match requested pool name.")
				continue
			}
		}

		// Drive media type
		if mediaType != "" {
			if mediaType != pool.DriveMediaType {
				Logc(ctx).WithFields(log.Fields{
					"Name":               pool.Label,
					"MediaType":          pool.DriveMediaType,
					"RequestedMediaType": mediaType,
				}).Debug("Pool does not match requested media type.")
				continue
			}
		}

		// Free space
		if minFreeSpaceBytes > 0 {
			poolFreeSpace, err := strconv.ParseUint(pool.FreeSpace, 10, 64)
			if err != nil {
				Logc(ctx).WithFields(log.Fields{
					"Name":  pool.Label,
					"Error": err,
				}).Warn("Could not parse free space for pool.")
				continue
			}
			if poolFreeSpace < minFreeSpaceBytes {
				Logc(ctx).WithFields(log.Fields{
					"Name":           pool.Label,
					"FreeSpace":      poolFreeSpace,
					"RequestedSpace": minFreeSpaceBytes,
				}).Debug("Pool does not have sufficient free space.")
				continue
			}
		}

		// Everything matched
		matchingPools = append(matchingPools, pool)
	}

	return matchingPools, nil
}

// GetVolumePoolByRef returns the pool with the specified volumeGroupRef.
func (d Client) GetVolumePoolByRef(ctx context.Context, volumeGroupRef string) (VolumeGroupEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":         "GetVolumePoolByRef",
			"Type":           "Client",
			"volumeGroupRef": volumeGroupRef,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolumePoolByRef")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumePoolByRef")
	}

	// Get the storage pool (may be either volume RAID group or dynamic disk pool)
	resourcePath := "/storage-pools/" + volumeGroupRef
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", resourcePath)
	if err != nil {
		return VolumeGroupEx{}, fmt.Errorf("could not get storage pool: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return VolumeGroupEx{}, Error{
			Code:    response.StatusCode,
			Message: "could not get storage pool",
		}
	}

	// Parse JSON data
	pool := VolumeGroupEx{}
	if err := json.Unmarshal(responseBody, &pool); err != nil {
		return VolumeGroupEx{}, fmt.Errorf("could not parse storage pool data: %s; %v", string(responseBody), err)
	}

	return pool, nil
}

// GetVolumes returns an array containing all the volumes on the array.
func (d Client) GetVolumes(ctx context.Context) ([]VolumeEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetVolumes",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolumes")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumes")
	}

	// Query volumes on array
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/volumes")
	if err != nil {
		return nil, errors.New("failed to read volumes")
	}

	if response.StatusCode != http.StatusOK {
		return nil, Error{
			Code:    response.StatusCode,
			Message: "failed to read volumes",
		}
	}

	volumes := make([]VolumeEx, 0)
	err = json.Unmarshal(responseBody, &volumes)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s. %v", string(responseBody), err)
	}

	Logc(ctx).WithField("Count", len(volumes)).Debug("Read volumes.")

	return volumes, nil
}

// ListVolumes returns an array containing all the volume names on the array.
func (d Client) ListVolumes(ctx context.Context) ([]string, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ListVolumes",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ListVolumes")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ListVolumes")
	}

	volumes, err := d.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	volumeNames := make([]string, 0, len(volumes))

	for _, vol := range volumes {
		volumeNames = append(volumeNames, vol.Label)
	}

	Logc(ctx).WithField("Count", len(volumeNames)).Debug("Read volume names.")

	return volumeNames, nil
}

// GetVolume returns a volume structure from the array whose label matches the specified name. Use this method sparingly,
// at most once per workflow, because the Web Services Proxy does not support server-side filtering so the only choice is to
// read all volumes to find the one of interest. Most methods in this module operate on the returned VolumeEx structure, not
// the volume name, to minimize the need for calling this method.
func (d Client) GetVolume(ctx context.Context, name string) (VolumeEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetVolume",
			"Type":   "Client",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolume")
	}

	volumes, err := d.GetVolumes(ctx)
	if err != nil {
		return VolumeEx{}, err
	}

	for _, vol := range volumes {
		if vol.Label == name {
			return vol, nil
		}
	}

	return VolumeEx{}, nil
}

// GetVolumeByRef gets a single volume from the array.
func (d Client) GetVolumeByRef(ctx context.Context, volumeRef string) (VolumeEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "GetVolumeByRef",
			"Type":      "Client",
			"volumeRef": volumeRef,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolumeByRef")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumeByRef")
	}

	// Get a single volume by its ref ID from storage array
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/volumes/"+volumeRef)
	if err != nil {
		return VolumeEx{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return VolumeEx{}, Error{
			Code:    response.StatusCode,
			Message: "failed to read volume",
		}
	}

	var volume VolumeEx
	err = json.Unmarshal(responseBody, &volume)
	if err != nil {
		return VolumeEx{}, fmt.Errorf("could not parse volume data: %s. %v", string(responseBody), err)
	}

	return volume, nil
}

// ensureVolumeTagsWithRetry attempts to add missing tags to the supplied volume (if needed)
func (d Client) ensureVolumeTagsWithRetry(ctx context.Context, volumeRef string, tags []VolumeTag) (VolumeEx, error) {
	fixNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debugf("Failed to correct tags for volume %s; retrying", volumeRef)
	}

	attemptToFixTags := func() error {
		retryVolume, retryError := d.GetVolumeByRef(ctx, volumeRef)
		if retryError != nil {
			return retryError
		}

		if d.volumeHasTags(retryVolume, tags) {
			// the expected tags are present, we can successfully return
			// note: this 422 can occur from the original CreateVolume call OR this function's UpdateVolumeTags call
			Logc(ctx).WithFields(log.Fields{
				"Name":           retryVolume.Label,
				"VolumeRef":      retryVolume.VolumeRef,
				"VolumeGroupRef": retryVolume.VolumeGroupRef,
			}).Debug("Volume (re-read after HTTP 422) now has expected tags.")
			return nil
		}

		// the expected tags are missing, try to update the volume with the expected tags
		Logc(ctx).WithField("Name", retryVolume.Label).Debug("Re-read volume tags mismatch.")

		updateVolume, updateError := d.UpdateVolumeTags(ctx, volumeRef, tags)
		if updateError != nil {
			// this update could've failed with another 422, return an error and try again
			return updateError
		}

		if d.volumeHasTags(updateVolume, tags) {
			// the expected tags are present on the updated volume, we can successfully return
			Logc(ctx).WithFields(log.Fields{
				"Name":           updateVolume.Label,
				"VolumeRef":      updateVolume.VolumeRef,
				"VolumeGroupRef": updateVolume.VolumeGroupRef,
			}).Debug("Updated volume (re-read after HTTP 422) now has expected tags.")
			return nil
		}

		// the volume is still missing the expected tags, return an error so we will retry all of this again
		return fmt.Errorf("volume %s missing expected tags %v", volumeRef, tags)
	}

	maxDuration := 30 * time.Second

	fixBackoff := backoff.NewExponentialBackOff()
	fixBackoff.InitialInterval = 2 * time.Second
	fixBackoff.Multiplier = 2
	fixBackoff.RandomizationFactor = 0.1
	fixBackoff.MaxElapsedTime = maxDuration

	// Read and correct the volume tags, if needed, with an exponential backoff
	if err := backoff.RetryNotify(attemptToFixTags, fixBackoff, fixNotify); err != nil {
		Logc(ctx).Errorf("could not correct tags for volume %v after %3.2f seconds.", volumeRef, maxDuration.Seconds())
		return VolumeEx{}, err
	}

	return d.GetVolumeByRef(ctx, volumeRef)
}

// CreateVolume creates a volume (i.e. a LUN) on the array, and it returns the resulting VolumeEx structure.
func (d Client) CreateVolume(
	ctx context.Context, name string, volumeGroupRef string, size uint64, mediaType, fstype string,
) (VolumeEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":         "CreateVolume",
			"Type":           "Client",
			"name":           name,
			"volumeGroupRef": volumeGroupRef,
			"size":           size,
			"mediaType":      mediaType,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateVolume")
	}

	// Ensure that we do not exceed the maximum allowed volume length
	if len(name) > maxNameLength {
		return VolumeEx{}, fmt.Errorf("the volume name %v exceeds the maximum length of %d characters", name,
			maxNameLength)
	}

	// Copy static volume metadata and add fstype
	tags := d.makeVolumeTags()
	tags = append(tags, VolumeTag{"fstype", fstype})

	// Set up the volume create request
	request := VolumeCreateRequest{
		VolumeGroupRef: volumeGroupRef,
		Name:           name,
		SizeUnit:       "kb",
		Size:           int(size / 1024), // The API requires Size to be an int (not int64) so pass as an int but in KB.
		SegmentSize:    128,
		VolumeTags:     tags,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return VolumeEx{}, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	// Create the volume
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", "/volumes")
	if err != nil {
		return VolumeEx{}, fmt.Errorf("API invocation failed. %v", err)
	}

	// Work around Web Services Proxy bug by re-reading the volume we (hopefully) just created
	if response.StatusCode == http.StatusUnprocessableEntity {
		Logc(ctx).Debugf("Volume create failed with 422 response, attempting to re-read volume %v", name)

		retryVolume, getError := d.GetVolume(ctx, name)
		if getError != nil {
			return VolumeEx{}, getError
		}

		result, retryErr := d.ensureVolumeTagsWithRetry(ctx, retryVolume.VolumeRef, tags)
		if retryErr != nil {
			// best effort cleanup of the volume we created that never received its required tags
			if deleteErr := d.DeleteVolume(ctx, retryVolume); deleteErr != nil {
				return VolumeEx{}, fmt.Errorf("%v; %v", retryErr.Error(), deleteErr.Error())
			}
			return VolumeEx{}, retryErr
		}
		return result, nil
	}

	if response.StatusCode != http.StatusOK {
		apiError := d.getErrorFromHTTPResponse(response, responseBody)
		apiError.Message = fmt.Sprintf("could not create volume %s; %s", name, apiError.Message)
		return VolumeEx{}, apiError
	}

	// Parse JSON volume data
	vol := VolumeEx{}
	if err := json.Unmarshal(responseBody, &vol); err != nil {
		return VolumeEx{}, fmt.Errorf("could not parse API response: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":           vol.Label,
		"VolumeRef":      vol.VolumeRef,
		"VolumeGroupRef": vol.VolumeGroupRef,
	}).Debug("Created volume.")

	return vol, nil
}

func (d Client) volumeHasTags(volume VolumeEx, tags []VolumeTag) bool {

	for _, tag := range tags {

		tagFound := false

		for _, volumeTag := range volume.VolumeTags {
			if volumeTag.Equals(tag) {
				tagFound = true
				break
			}
		}

		if !tagFound {
			return false
		}
	}

	return true
}

// UpdateVolumeTags updates the tags for a given volume
func (d Client) UpdateVolumeTags(
	ctx context.Context, volumeRef string, tags []VolumeTag,
) (VolumeEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "UpdateVolumeTags",
			"Type":      "Client",
			"volumeRef": volumeRef,
			"tags":      tags,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> UpdateVolumeTags")
		defer Logc(ctx).WithFields(fields).Debug("<<<< UpdateVolumeTags")
	}

	// We need a valid volumeRef
	if len(volumeRef) <= 0 {
		return VolumeEx{}, fmt.Errorf("the volumeRef is invalid")
	}

	// Set up the volume update request
	request := VolumeUpdateRequest{
		VolumeTags: tags,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return VolumeEx{}, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	// Update the volume
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", "/volumes/"+volumeRef)
	if err != nil {
		return VolumeEx{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		apiError := d.getErrorFromHTTPResponse(response, responseBody)
		apiError.Message = fmt.Sprintf("could not update volume %s; %s", volumeRef, apiError.Message)
		return VolumeEx{}, apiError
	}

	// Parse JSON volume data
	vol := VolumeEx{}
	if err := json.Unmarshal(responseBody, &vol); err != nil {
		return VolumeEx{}, fmt.Errorf("could not parse API response: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":           vol.Label,
		"VolumeRef":      vol.VolumeRef,
		"VolumeGroupRef": vol.VolumeGroupRef,
	}).Debug("Updated volume.")

	return vol, nil
}

// ResizingVolume checks to see if an expand operation is in progress for the volume.
func (d Client) ResizingVolume(ctx context.Context, volume VolumeEx) (bool, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ResizingVolume",
			"Type":   "Client",
			"name":   volume.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ResizingVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ResizingVolume")
	}

	resourcePath := "/volumes/" + volume.VolumeRef + "/expand"
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", resourcePath)
	if err != nil {
		return false, fmt.Errorf("API invocation failed. %v", err)
	}

	// Parse JSON data
	responseData := VolumeResizeStatusResponse{}
	if err := json.Unmarshal(responseBody, &responseData); err != nil {
		return false, fmt.Errorf("could not parse volume resize status response: %s; %v", string(responseBody), err)
	}
	Logc(ctx).WithFields(log.Fields{
		"percentComplete":  responseData.PercentComplete,
		"timeToCompletion": responseData.TimeToCompletion,
		"action":           responseData.Action,
		"name":             volume.Label,
	}).Debug("Volume resize status.")

	// Evaluate resize status
	switch response.StatusCode {
	case http.StatusOK:
		if responseData.Action != "none" {
			return true, fmt.Errorf("volume resize operation in progress with action: %v", responseData.Action)
		} else {
			return false, nil
		}
	default:
		apiError := d.getErrorFromHTTPResponse(response, responseBody)
		apiError.Message = fmt.Sprintf("could not get resize status %s; %s", volume.Label, apiError.Message)
		return false, apiError
	}
}

// ResizeVolume expands a volume's size. Thin provisioning is not supported on E-series
// so we only call the API for expanding thick provisioned volumes.
func (d Client) ResizeVolume(ctx context.Context, volume VolumeEx, size uint64) error {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ResizeVolume",
			"Type":   "Client",
			"name":   volume.Label,
			"size":   size,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ResizeVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ResizeVolume")
	}

	// The API requires size to be an int (not uint64) so pass as an int but in KB.
	expansionSize := int(size / 1024)

	// Set up the volume resize request
	request := VolumeResizeRequest{
		ExpansionSize: expansionSize,
		SizeUnit:      "kb",
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}
	resourcePath := "/volumes/" + volume.VolumeRef + "/expand"
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", resourcePath)
	if err != nil {
		return fmt.Errorf("API invocation failed: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		apiError := d.getErrorFromHTTPResponse(response, responseBody)
		Logc(ctx).WithFields(log.Fields{
			"name":       volume,
			"statusCode": response.StatusCode,
			"message":    apiError.Message,
		}).Error("Volume resize failed!")
		apiError.Message = fmt.Sprintf("resize failed for volume %s", volume.Label)
		return apiError
	}

	return nil
}

// DeleteVolume deletes a volume from the array.
func (d Client) DeleteVolume(ctx context.Context, volume VolumeEx) error {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "DeleteVolume",
			"Type":   "Client",
			"name":   volume.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteVolume")
	}

	// Remove this volume from storage array
	response, responseBody, err := d.InvokeAPI(ctx, nil, "DELETE", "/volumes/"+volume.VolumeRef)
	if err != nil {
		return fmt.Errorf("API invocation failed. %v", err)
	}

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNoContent:
	case http.StatusUnprocessableEntity:
	case http.StatusNotFound:
	case http.StatusGone:
		break
	default:
		apiError := d.getErrorFromHTTPResponse(response, responseBody)
		apiError.Message = fmt.Sprintf("could not destroy volume %s; %s", volume.Label, apiError.Message)
		return apiError
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":           volume.Label,
		"VolumeRef":      volume.VolumeRef,
		"VolumeGroupRef": volume.VolumeGroupRef,
	}).Debug("Deleted volume.")

	return nil
}

// EnsureHostForIQN handles automatic E-series Host and Host Group creation. Given the IQN of a host, this method
// verifies whether a Host is already configured on the array. If so, the Host info is returned and no further action is
// taken. If not, this method chooses a unique name for the Host and creates it on the array. Once the Host is created,
// it is placed in the Host Group used for nDVP volumes.
func (d Client) EnsureHostForIQN(ctx context.Context, iqn string) (HostEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "EnsureHostForIQN",
			"Type":   "Client",
			"iqn":    iqn,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> EnsureHostForIQN")
		defer Logc(ctx).WithFields(fields).Debug("<<<< EnsureHostForIQN")
	}

	// See if there is already a host for the specified IQN
	host, err := d.GetHostForIQN(ctx, iqn)
	if err != nil {
		return HostEx{}, fmt.Errorf("could not ensure host for IQN %s: %v", iqn, err)
	}

	// If we found a host, return it and leave well enough alone, since the host could have been defined
	// by nDVP or by an admin. Otherwise, we'll create a host for our purposes.
	if host.HostRef != "" {

		Logc(ctx).WithFields(log.Fields{
			"IQN":        iqn,
			"HostRef":    host.HostRef,
			"ClusterRef": host.ClusterRef,
		}).Debug("Host already exists for IQN.")

		return host, nil
	}

	// Pick a host name
	hostname := d.createNameForHost(iqn)

	// Ensure we have a group for the new host. If for any reason this fails, we'll create the host anyway with no group.
	hostGroup, err := d.EnsureHostGroup(ctx)
	if err != nil {
		Logc(ctx).Warn("Could not ensure host group for new host.")
	}

	// Create the new host in the group
	return d.CreateHost(ctx, hostname, iqn, d.config.HostType, hostGroup)
}

func (d Client) createNameForHost(iqn string) string {

	// Get unique hostname suffix up to 10 chars, either the last part of the IQN or a random sequence
	var uniqueSuffix = utils.RandomString(10)
	index := strings.LastIndex(iqn, ":")
	if (index >= 0) && (len(iqn) > index+2) {
		uniqueSuffix = iqn[index+1:]
	}
	if len(uniqueSuffix) > 10 {
		uniqueSuffix = uniqueSuffix[:10]
	}

	// Pick a host name, incorporating the local hostname value if possible
	hostname, err := os.Hostname()
	if err != nil {
		hostname = utils.RandomString(maxNameLength)
	}

	// Use as much of the hostname as will fit
	maxLength := maxNameLength - (len(uniqueSuffix) + 1)
	if len(hostname) > maxLength {
		hostname = hostname[0:maxLength]
	}

	return hostname + "_" + uniqueSuffix
}

func (d Client) createNameForPort(host string) string {

	suffix := "_port"
	hostname := host

	maxLength := maxNameLength - len(suffix)
	if len(hostname) > maxLength {
		hostname = hostname[0:maxLength]
	}

	return hostname + suffix
}

// GetHostForIQN queries the Host objects on the array an returns one matching the supplied IQN. An empty struct is
// returned if a matching host is not found, so the caller should check for empty values in the result.
func (d Client) GetHostForIQN(ctx context.Context, iqn string) (HostEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetHostForIQN",
			"Type":   "Client",
			"iqn":    iqn,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetHostForIQN")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetHostForIQN")
	}

	// Get hosts
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/hosts")
	if err != nil {
		return HostEx{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return HostEx{}, Error{
			Code:    response.StatusCode,
			Message: "could not get hosts from array",
		}
	}

	// Parse JSON data
	hosts := make([]HostEx, 0)
	if err := json.Unmarshal(responseBody, &hosts); err != nil {
		return HostEx{}, fmt.Errorf("could not parse host data: %s; %v", string(responseBody), err)
	}

	// Find initiator with matching IQN
	for _, host := range hosts {
		for _, f := range host.Initiators {
			Logc(ctx).WithFields(log.Fields{
				"f.NodeName.IoInterfaceType": f.NodeName.IoInterfaceType,
				"f.NodeName.IscsiNodeName":   f.NodeName.IscsiNodeName,
				"f.InitiatorNodeName.NodeName.IoInterfaceType	": f.InitiatorNodeName.NodeName.IoInterfaceType,
				"f.InitiatorNodeName.NodeName.IscsiNodeName": f.InitiatorNodeName.NodeName.IscsiNodeName,
				"iqn": iqn,
			}).Debug("Comparing...")

			if f.NodeName.IoInterfaceType == "iscsi" && f.NodeName.IscsiNodeName == iqn {

				Logc(ctx).WithFields(log.Fields{
					"Name": host.Label,
					"IQN":  iqn,
				}).Debug("Found host.")

				return host, nil

			} else if f.InitiatorNodeName.NodeName.IoInterfaceType == "iscsi" && f.InitiatorNodeName.NodeName.IscsiNodeName == iqn {

				Logc(ctx).WithFields(log.Fields{
					"Name": host.Label,
					"IQN":  iqn,
				}).Debug("Found host.")

				return host, nil
			}
		}
	}

	// Nothing failed, so return an empty structure if we didn't find anything
	Logc(ctx).WithField("IQN", iqn).Debug("No host found.")
	return HostEx{}, nil
}

// CreateHost creates a Host on the array. If a HostGroup is specified, the Host is placed in that group.
func (d Client) CreateHost(ctx context.Context, name, iqn, hostType string, hostGroup HostGroup) (HostEx, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "CreateHost",
			"Type":      "Client",
			"name":      name,
			"iqn":       iqn,
			"hostType":  hostType,
			"hostGroup": hostGroup.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateHost")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateHost")
	}

	// Set up the host create request
	var request HostCreateRequest
	request.Name = name
	request.HostType.Index = d.getBestIndexForHostType(ctx, hostType)
	request.GroupID = hostGroup.ClusterRef
	request.Ports = make([]HostPort, 1)
	request.Ports[0].Label = d.createNameForPort(name)
	request.Ports[0].Port = iqn
	request.Ports[0].Type = "iscsi"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return HostEx{}, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":  name,
		"Group": hostGroup.Label,
		"IQN":   iqn,
	}).Debug("Creating host.")

	// Create the host
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", "/hosts")
	if err != nil {
		return HostEx{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return HostEx{}, Error{
			Code:    response.StatusCode,
			Message: fmt.Sprintf("could not create host %s", name),
		}
	}

	// Parse JSON data
	host := HostEx{}
	if err := json.Unmarshal(responseBody, &host); err != nil {
		return HostEx{}, fmt.Errorf("could not parse host data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":          host.Label,
		"HostRef":       host.HostRef,
		"ClusterRef":    host.ClusterRef,
		"HostTypeIndex": host.HostTypeIndex,
	}).Debug("Created host.")

	return host, nil
}

func (d Client) getBestIndexForHostType(ctx context.Context, hostType string) int {

	hostTypeIndex := -1

	// Try the mapped values first
	_, ok := HostTypes[hostType]
	if ok {
		hostTypeIndex, _ = d.getIndexForHostType(ctx, HostTypes[hostType])
	}

	// If not found, try matching the E-series host type codes directly
	if hostTypeIndex == -1 {
		hostTypeIndex, _ = d.getIndexForHostType(ctx, hostType)
	}

	// If still not found, fall back to standard Linux DM-MPIO multipath driver
	if hostTypeIndex == -1 {
		hostTypeIndex, _ = d.getIndexForHostType(ctx, "LnxALUA")
	}

	// Failing that, use index 0, which should be the factory default
	if hostTypeIndex == -1 {
		hostTypeIndex = 0
	}

	return hostTypeIndex
}

// getIndexForHostType queries the array for a host type matching the specified value. If found, it returns the
// index by which the type is known on the array. If not found, it returns -1.
func (d Client) getIndexForHostType(ctx context.Context, hostTypeCode string) (int, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "getIndexForHostType",
			"Type":         "Client",
			"hostTypeCode": hostTypeCode,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> getIndexForHostType")
		defer Logc(ctx).WithFields(fields).Debug("<<<< getIndexForHostType")
	}

	// Get host types
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/host-types")
	if err != nil {
		return -1, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return -1, Error{
			Code:    response.StatusCode,
			Message: "could not get host types from array",
		}
	}

	// Parse JSON data
	hostTypes := make([]HostType, 0)
	if err := json.Unmarshal(responseBody, &hostTypes); err != nil {
		return -1, fmt.Errorf("could not parse host type data: %s; %v", string(responseBody), err)
	}

	// Find host type with matching code
	for _, hostType := range hostTypes {
		if hostType.Code == hostTypeCode {

			Logc(ctx).WithFields(log.Fields{
				"Name":  hostType.Name,
				"Index": hostType.Index,
				"Code":  hostType.Code,
			}).Debug("Host type found.")

			return hostType.Index, nil
		}
	}

	Logc(ctx).WithField("Code", hostTypeCode).Debug("Host type not found.")
	return -1, nil
}

// EnsureHostGroup ensures that an E-series HostGroup exists to contain all Host objects created by the nDVP E-series driver.
// The group name is taken from the config structure. If the group exists, the group structure is returned and no further
// action is taken. If not, this method creates the group and returns the resulting group structure.
func (d Client) EnsureHostGroup(ctx context.Context) (HostGroup, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "EnsureHostGroup",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> EnsureHostGroup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< EnsureHostGroup")
	}

	hostGroupName := d.config.AccessGroup
	if len(hostGroupName) > maxNameLength {
		hostGroupName = hostGroupName[:maxNameLength]
	}

	// Get the group with the preconfigured name
	hostGroup, err := d.GetHostGroup(ctx, hostGroupName)
	if err != nil {
		return HostGroup{}, fmt.Errorf("could not ensure host group %s: %v", hostGroupName, err)
	}

	// Group found, so use it for host creation
	if hostGroup.ClusterRef != "" {
		Logc(ctx).WithFields(log.Fields{"Name": hostGroup.Label}).Debug("Host group found.")
		return hostGroup, nil
	}

	// Create the group
	hostGroup, err = d.CreateHostGroup(ctx, hostGroupName)
	if err != nil {
		return HostGroup{}, fmt.Errorf("could not create host group %s: %v", hostGroupName, err)
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":       hostGroup.Label,
		"ClusterRef": hostGroup.ClusterRef,
	}).Debug("Host group created.")

	return hostGroup, nil
}

// GetHostGroup returns an E-series HostGroup structure with the specified name. If no matching group is found, an
// empty structure is returned, so the caller should check for empty values in the result.
func (d Client) GetHostGroup(ctx context.Context, name string) (HostGroup, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetHostGroup",
			"Type":   "Client",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetHostGroup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetHostGroup")
	}

	// Get host groups
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/host-groups")
	if err != nil {
		return HostGroup{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return HostGroup{}, Error{
			Code:    response.StatusCode,
			Message: "could not get host groups from array",
		}
	}

	// Parse JSON data
	hostGroups := make([]HostGroup, 0)
	if err := json.Unmarshal(responseBody, &hostGroups); err != nil {
		return HostGroup{}, fmt.Errorf("could not parse host group data: %s; %v", string(responseBody), err)
	}

	for _, hostGroup := range hostGroups {
		if hostGroup.Label == name {
			return hostGroup, nil
		}
	}

	// Nothing failed, so return an empty structure if we didn't find anything
	return HostGroup{}, nil
}

// CreateHostGroup creates an E-series HostGroup object with the specified name and returns the resulting HostGroup structure.
func (d Client) CreateHostGroup(ctx context.Context, name string) (HostGroup, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "CreateHostGroup",
			"Type":   "Client",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateHostGroup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateHostGroup")
	}

	// Set up the host group create request
	request := HostGroupCreateRequest{
		Name:  name,
		Hosts: make([]string, 0),
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return HostGroup{}, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	// Create the host group
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", "/host-groups")
	if err != nil {
		return HostGroup{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return HostGroup{}, Error{
			Code:    response.StatusCode,
			Message: fmt.Sprintf("could not create host group %s", name),
		}
	}

	// Parse JSON data
	hostGroup := HostGroup{}
	if err := json.Unmarshal(responseBody, &hostGroup); err != nil {
		return HostGroup{}, fmt.Errorf("could not parse host data: %s; %v", string(responseBody), err)
	}

	return hostGroup, nil
}

// MapVolume maps a volume to the specified host and returns the resulting LUN mapping. If the volume is already mapped to the
// specified host, either directly or to the containing host group, no action is taken. If the volume is mapped to a different host,
// the method returns an error. Note that if the host is in a group, the volume will actually be mapped to the group instead of the
// individual host.
func (d Client) MapVolume(ctx context.Context, volume VolumeEx, host HostEx) (LUNMapping, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "MapVolume",
			"Type":       "Client",
			"volumeName": volume.Label,
			"hostName":   host.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> MapVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< MapVolume")
	}

	if !volume.IsMapped {

		// Volume is not already mapped, so map it now
		mapping, err := d.mapVolume(ctx, volume, host)

		// Work around Web Services Proxy bug by re-reading the map
		if apiError, ok := err.(Error); ok && apiError.Code == http.StatusUnprocessableEntity {

			Logc(ctx).Debug("Volume map failed with 422 response, attempting to re-read volume/map.")

			retryVolume, retryError := d.GetVolumeByRef(ctx, volume.VolumeRef)
			if retryError != nil {
				return LUNMapping{}, retryError
			} else if len(retryVolume.Mappings) == 0 {
				return LUNMapping{}, err
			}

			mapping = retryVolume.Mappings[0]

			Logc(ctx).WithFields(log.Fields{
				"Name":      retryVolume.Label,
				"VolumeRef": retryVolume.VolumeRef,
				"MapRef":    mapping.MapRef,
				"Type":      mapping.Type,
				"LunNumber": mapping.LunNumber,
			}).Debug("Volume mapped (re-read after HTTP 422).")

			return mapping, nil
		}

		return mapping, err

	} else {

		// Volume is mapped, so check if it is mapped to this host/group
		mappedToHost, mapping := d.volumeIsMappedToHost(ctx, volume, host)

		if mappedToHost {

			// Mapped here, so nothing to do
			Logc(ctx).WithFields(log.Fields{
				"Host":      host.Label,
				"Name":      volume.Label,
				"VolumeRef": volume.VolumeRef,
				"MapRef":    mapping.MapRef,
				"Type":      mapping.Type,
				"LunNumber": mapping.LunNumber,
			}).Debug("Volume already mapped to host.")

			return mapping, nil

		} else {

			// Mapped elsewhere, so return an error
			return LUNMapping{}, fmt.Errorf("volume %s is already mapped to a different host or host group",
				volume.Label)
		}
	}
}

// mapVolume maps a volume to a host with no checks for an existing mapping. If the host is in a host group, the volume is
// mapped to the group instead. The resulting mapping structure is returned.
func (d Client) mapVolume(ctx context.Context, volume VolumeEx, host HostEx) (LUNMapping, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "mapVolume",
			"Type":       "Client",
			"volumeName": volume.Label,
			"hostName":   host.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> mapVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< mapVolume")
	}

	// Map to host group if available, else a host
	targetID := host.HostRef
	if d.IsRefValid(host.ClusterRef) {
		targetID = host.ClusterRef
	}

	// Create a map request. The API proxy will pick a non-zero LUN number automatically.
	request := VolumeMappingCreateRequest{
		MappableObjectID: volume.VolumeRef,
		TargetID:         targetID,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return LUNMapping{}, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	// Create the mapping
	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", "/volume-mappings")
	if err != nil {
		return LUNMapping{}, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return LUNMapping{}, Error{
			Code:    response.StatusCode,
			Message: fmt.Sprintf("could not map volume %s", volume.Label),
		}
	}

	// Parse JSON data
	mapping := LUNMapping{}
	if err := json.Unmarshal(responseBody, &mapping); err != nil {
		return LUNMapping{}, fmt.Errorf("could not parse volume mapping data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":      volume.Label,
		"VolumeRef": volume.VolumeRef,
		"MapRef":    mapping.MapRef,
		"Type":      mapping.Type,
		"LunNumber": mapping.LunNumber,
	}).Debug("Volume mapped.")

	return mapping, nil
}

// volumeIsMappedToHost checks whether a volume is mapped to the specified host (or containing host group). If the mapping
// exists, the method returns true with the associated mapping structure. If no mapping exists, or if the volume is mapped
// elsewhere, the method returns false with an empty structure.
func (d Client) volumeIsMappedToHost(ctx context.Context, volume VolumeEx, host HostEx) (bool, LUNMapping) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "volumeIsMappedToHost",
			"Type":       "Client",
			"volumeName": volume.Label,
			"hostName":   host.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> volumeIsMappedToHost")
		defer Logc(ctx).WithFields(fields).Debug("<<<< volumeIsMappedToHost")
	}

	// Quick check to skip everything else
	if !volume.IsMapped {
		Logc(ctx).WithField("Name", volume.Label).Debug("Volume is not mapped.")
		return false, LUNMapping{}
	}

	// E-series only supports a single mapping per volume
	if len(volume.Mappings) == 0 {
		Logc(ctx).WithField("Name", volume.Label).Debug("Volume has no mappings.")
		return false, LUNMapping{}
	}
	mapping := volume.Mappings[0]

	// Double check we're looking at the right volume
	if mapping.VolumeRef != volume.VolumeRef {
		return false, LUNMapping{}
	}

	// Match either a host or a host group
	switch mapping.Type {

	case hostGroupMappingType: // "cluster"

		if mapping.MapRef == host.ClusterRef {

			Logc(ctx).WithFields(log.Fields{
				"volumeName": volume.Label,
				"hostName":   host.Label,
			}).Debug("Volume is mapped to the host's enclosing group.")

			return true, mapping
		}

	case hostMappingType: // "host"

		if mapping.MapRef == host.HostRef {

			Logc(ctx).WithFields(log.Fields{
				"volumeName": volume.Label,
				"hostName":   host.Label,
			}).Debug("Volume is mapped to the host.")

			return true, mapping
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"volumeName": volume.Label,
		"hostName":   host.Label,
	}).Debug("Volume is mapped to a different host or group.")

	return false, LUNMapping{}
}

// UnmapVolume removes a mapping from the specified volume. If no map exists, no action is taken.
func (d Client) UnmapVolume(ctx context.Context, volume VolumeEx) error {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "UnmapVolume",
			"Type":   "Client",
			"volume": volume.Label,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> UnmapVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< UnmapVolume")
	}

	// If volume isn't mapped, there's nothing to do
	if !volume.IsMapped || len(volume.Mappings) == 0 {

		Logc(ctx).WithFields(log.Fields{
			"Name":      volume.Label,
			"VolumeRef": volume.VolumeRef,
		}).Warn("Volume unmap requested, but volume is not mapped.")

		return nil
	}
	mapping := volume.Mappings[0]

	// Remove this volume mapping from storage array
	response, _, err := d.InvokeAPI(ctx, nil, "DELETE", "/volume-mappings/"+mapping.LunMappingRef)
	if err != nil {
		return fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return Error{
			Code:    response.StatusCode,
			Message: fmt.Sprintf("could not unmap volume %s", volume.Label),
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"Name":      volume.Label,
		"VolumeRef": volume.VolumeRef,
		"MapRef":    mapping.MapRef,
		"Type":      mapping.Type,
		"LunNumber": mapping.LunNumber,
	}).Debug("Volume unmapped.")

	return nil
}

// GetTargetIQN returns the IQN for the array.
func (d *Client) GetTargetIQN(ctx context.Context) (string, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetTargetIqn",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetTargetIqn")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetTargetIqn")
	}

	// Query iSCSI target settings
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/iscsi/target-settings")
	if err != nil {
		return "", fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return "", Error{
			Code:    response.StatusCode,
			Message: "could not read iSCSI settings",
		}
	}

	var settings IscsiTargetSettings
	err = json.Unmarshal(responseBody, &settings)
	if err != nil {
		return "", fmt.Errorf("could not parse iSCSI settings data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"IargetIQN": settings.NodeName.IscsiNodeName,
	}).Debug("Got target iSCSI node name.")

	return settings.NodeName.IscsiNodeName, nil
}

// GetTargetSettings returns the iSCSI target settings for the array.
func (d *Client) GetTargetSettings(ctx context.Context) (*IscsiTargetSettings, error) {

	if d.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetTargetSettings",
			"Type":   "Client",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetTargetSettings")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetTargetSettings")
	}

	// Query iSCSI target settings
	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", "/iscsi/target-settings")
	if err != nil {
		return nil, fmt.Errorf("API invocation failed. %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, Error{
			Code:    response.StatusCode,
			Message: "could not read iSCSI settings",
		}
	}

	var settings IscsiTargetSettings
	err = json.Unmarshal(responseBody, &settings)
	if err != nil {
		return nil, fmt.Errorf("could not parse iSCSI settings data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"IargetIQN": settings.NodeName.IscsiNodeName,
	}).Debug("Got target iSCSI target settings.")

	return &settings, nil
}

// IsRefValid checks whether the supplied string is a valid E-series object reference as used by its REST API.
// Ref values are strings of all numerical digits that aren't all zeros (i.e. the null ref).
func (d Client) IsRefValid(ref string) bool {

	switch ref {
	case "", NullRef:
		return false
	default:
		return true
	}
}

// getErrorFromHTTPResponse converts error information from some E-series API responses into GoLang error objects that
// embed the additional error text.
func (d Client) getErrorFromHTTPResponse(response *http.Response, responseBody []byte) Error {

	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusUnprocessableEntity {

		// Parse JSON error data
		responseData := CallResponseError{}
		if err := json.Unmarshal(responseBody, &responseData); err != nil {
			return Error{
				Code:    response.StatusCode,
				Message: fmt.Sprintf("could not parse API error response: %s; %v", string(responseBody), err),
			}
		}

		return Error{
			Code: response.StatusCode,
			Message: fmt.Sprintf("API failed; Error: %s; Localized: %s",
				responseData.ErrorMsg, responseData.LocalizedMsg),
		}
	} else {

		// Other error
		return Error{
			Code:    response.StatusCode,
			Message: "API failed",
		}
	}
}
