// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This package provides a high-level interface to the NetApp GCP Cloud Volumes NFS REST API.
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
	"net/url"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

// Sample curl command to invoke the REST interface:
// Get fresh token from Trident debug logs
// export Bearer="<token>"
// curl https://cloudvolumesgcp-api.netapp.com/version --insecure -X GET -H "Content-Type: application/json" -H "Authorization: Bearer $Bearer"

const (
	httpTimeoutSeconds    = 30
	retryTimeoutSeconds   = 30
	VolumeCreateTimeout   = 10 * time.Second
	DefaultTimeout        = 120 * time.Second
	defaultAPIURL         = "https://cloudvolumesgcp-api.netapp.com"
	defaultAPIAudienceURL = defaultAPIURL
	MaxLabelLength        = 255
)

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {
	utils.MountPoint
	// GCP project number
	ProjectNumber string

	// GCP CVS API authentication parameters
	APIKey drivers.GCPPrivateKey

	// GCP region
	APIRegion string

	// URL for accessing the API via an HTTP/HTTPS proxy
	ProxyURL string

	// Options
	DebugTraceFlags map[string]bool

	// CVS api url
	APIURL string

	// CVS api audience url
	APIAudienceURL string
}

type Client struct {
	config      *ClientConfig
	tokenSource *oauth2.TokenSource
	token       *oauth2.Token
	m           *sync.Mutex
}

// NewDriver is a factory method for creating a new instance.
func NewDriver(config ClientConfig) *Client {
	if config.APIURL == "" {
		config.APIURL = defaultAPIURL
	}
	if config.APIAudienceURL == "" {
		config.APIAudienceURL = defaultAPIAudienceURL
	}

	d := &Client{
		config: &config,
		m:      &sync.Mutex{},
	}

	return d
}

func (d *Client) makeURL(resourcePath string) string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s%s", d.config.APIURL, d.config.ProjectNumber,
		d.config.APIRegion, resourcePath)
}

func (d *Client) initTokenSource(ctx context.Context) error {
	keyBytes, err := json.Marshal(d.config.APIKey)
	if err != nil {
		return err
	}

	tokenSource, err := google.JWTAccessTokenSourceFromJSON(keyBytes, d.config.APIAudienceURL)
	if err != nil {
		return err
	}
	d.tokenSource = &tokenSource

	Logc(ctx).Debug("Got token source.")

	return nil
}

func (d *Client) refreshToken(ctx context.Context) error {
	var err error

	// Get token source (once per API client instance)
	if d.tokenSource == nil {
		if err = d.initTokenSource(ctx); err != nil {
			return err
		}
	}

	// If we have a valid token, just return
	if d.token != nil && d.token.Valid() {

		Logc(ctx).WithFields(log.Fields{
			"type":    d.token.Type(),
			"expires": d.token.Expiry.String(),
		}).Trace("Token still valid.")

		return nil
	}

	// Get a fresh token
	d.token, err = (*d.tokenSource).Token()
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"type":    d.token.Type(),
		"expires": d.token.Expiry.String(),
	}).Debug("Got fresh token.")

	return nil
}

// InvokeAPI makes a REST call to the cloud volumes REST service. The body must be a marshaled JSON byte array (or nil).
// The method is the HTTP verb (i.e. GET, POST, ...).
func (d *Client) InvokeAPI(
	ctx context.Context, requestBody []byte, method, gcpURL string,
) (*http.Response, []byte, error) {
	var request *http.Request
	var response *http.Response
	var err error

	if err = d.refreshToken(ctx); err != nil {
		return nil, nil, fmt.Errorf("cannot invoke API %s, no valid token; %v", gcpURL, err)
	}

	if requestBody == nil {
		request, err = http.NewRequestWithContext(ctx, method, gcpURL, nil)
	} else {
		request, err = http.NewRequestWithContext(ctx, method, gcpURL, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("X-Request-ID", fmt.Sprint(ctx.Value(ContextKeyRequestID)))
	d.token.SetAuthHeader(request)

	tr := &http.Transport{}
	// Use ProxyUrl if set
	proxyURL := d.config.ProxyURL

	tr.TLSClientConfig = &tls.Config{
		MinVersion: config.MinClientTLSVersion,
	}

	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, nil, err
		}

		tr.Proxy = http.ProxyURL(proxy)

		// Skip certificate validation
		tr.TLSClientConfig.InsecureSkipVerify = true
	} else {
		// Require certificate validation if not using a proxy
		tr.TLSClientConfig.InsecureSkipVerify = false
	}

	if d.config.DebugTraceFlags["api"] {
		utils.LogHTTPRequest(request, requestBody, false)
	}

	// Send the request
	client := &http.Client{
		Transport: tr,
		Timeout:   httpTimeoutSeconds * time.Second,
	}
	response, err = d.invokeAPIWithRetry(client, request)

	if response != nil {
		defer func() { _ = response.Body.Close() }()
	}

	if response == nil || err != nil {
		if err == nil {
			err = errors.New("api response is nil")
		}
		Logc(ctx).Warnf("Error communicating with GCP REST interface. %v", err)
		return nil, nil, err
	}

	var responseBody []byte

	responseBody, err = ioutil.ReadAll(response.Body)
	if d.config.DebugTraceFlags["api"] {
		utils.LogHTTPResponse(ctx, response, responseBody, false)
	}

	return response, responseBody, err
}

func (d *Client) invokeAPINoRetry(client *http.Client, request *http.Request) (*http.Response, error) {
	return client.Do(request)
}

func (d *Client) invokeAPIWithRetry(client *http.Client, request *http.Request) (*http.Response, error) {
	var response *http.Response
	var err error

	invoke := func() error {
		response, err = d.invokeAPINoRetry(client, request)

		// Return a permanent error to stop retrying if we couldn't invoke the API at all
		if err != nil {
			return backoff.Permanent(err)
		} else if response == nil {
			return backoff.Permanent(errors.New("API invocation did not return a response"))
		}

		// The API can be flaky, so retry if we got a 403 (Forbidden)
		if response.StatusCode == 403 {
			return errors.New("API result is 403")
		}

		return nil
	}
	invokeNotify := func(err error, duration time.Duration) {
		Logc(request.Context()).WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Retrying API.")
	}
	invokeBackoff := backoff.NewExponentialBackOff()
	invokeBackoff.MaxElapsedTime = retryTimeoutSeconds * time.Second

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		Logc(request.Context()).Errorf("API has not succeeded after %3.2f seconds.",
			invokeBackoff.MaxElapsedTime.Seconds())
		return response, err
	}

	return response, nil
}

func (d *Client) GetVersion(ctx context.Context) (*utils.Version, *utils.Version, error) {
	resourcePath := "/version"

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, nil, errors.New("failed to read version")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, nil, err
	}

	var version VersionResponse
	err = json.Unmarshal(responseBody, &version)
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse version data: %s; %v", string(responseBody), err)
	}

	apiVersion, err := utils.ParseSemantic(version.APIVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid semantic version for API version (%s): %v", version.APIVersion, err)
	}

	sdeVersion, err := utils.ParseSemantic(version.SdeVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid semantic version for SDE version (%s): %v", version.SdeVersion, err)
	}

	Logc(ctx).WithFields(log.Fields{
		"apiVersion": apiVersion.String(),
		"sdeVersion": sdeVersion.String(),
	}).Info("Read CVS version.")

	return apiVersion, sdeVersion, nil
}

func (d *Client) GetServiceLevels(ctx context.Context) (map[string]string, error) {
	resourcePath := "/Storage/ServiceLevels"

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read service levels")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var serviceLevels ServiceLevelsResponse
	err = json.Unmarshal(responseBody, &serviceLevels)
	if err != nil {
		return nil, fmt.Errorf("could not parse service level data: %s; %v", string(responseBody), err)
	}

	serviceLevelMap := make(map[string]string)
	for _, serviceLevel := range serviceLevels {
		serviceLevelMap[serviceLevel.Name] = serviceLevel.Performance
	}

	Logc(ctx).WithFields(log.Fields{
		"count":  len(serviceLevels),
		"levels": serviceLevelMap,
	}).Info("Read service levels.")

	return serviceLevelMap, nil
}

func (d *Client) GetVolumes(ctx context.Context) (*[]Volume, error) {
	resourcePath := "/Volumes"

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var volumes []Volume
	err = json.Unmarshal(responseBody, &volumes)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithField("count", len(volumes)).Debug("Read volumes.")

	return &volumes, nil
}

func (d *Client) GetVolumeByName(ctx context.Context, name string) (*Volume, error) {
	volumes, err := d.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	matchingVolumes := make([]Volume, 0)

	for _, volume := range *volumes {
		if volume.Name == name {
			matchingVolumes = append(matchingVolumes, volume)
		}
	}

	if len(matchingVolumes) == 0 {
		return nil, utils.NotFoundError(fmt.Sprintf("volume with name %s not found", name))
	} else if len(matchingVolumes) > 1 {
		return nil, fmt.Errorf("multiple volumes with name %s found", name)
	}

	return &matchingVolumes[0], nil
}

func (d *Client) GetVolumeByCreationToken(ctx context.Context, creationToken string) (*Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var volumes []Volume
	err = json.Unmarshal(responseBody, &volumes)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	if len(volumes) == 0 {
		return nil, utils.NotFoundError(fmt.Sprintf("volume with creationToken %s not found", creationToken))
	} else if len(volumes) > 1 {
		return nil, fmt.Errorf("multiple volumes with creationToken %s found", creationToken)
	}

	return &volumes[0], nil
}

func (d *Client) VolumeExistsByCreationToken(ctx context.Context, creationToken string) (bool, *Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return false, nil, err
	}

	if err := d.getErrorFromAPIResponse(response, responseBody); err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	}

	var volumes []Volume
	err = json.Unmarshal(responseBody, &volumes)
	if err != nil {
		return false, nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	if len(volumes) == 0 {
		return false, nil, nil
	}
	return true, &volumes[0], nil
}

func (d *Client) GetVolumeByID(ctx context.Context, volumeId string) (*Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s", volumeId)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to get volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var volume Volume
	err = json.Unmarshal(responseBody, &volume)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	return &volume, nil
}

func (d *Client) WaitForVolumeStates(
	ctx context.Context, volume *Volume, desiredStates, abortStates []string, maxElapsedTime time.Duration,
) (string, error) {
	volumeState := ""

	checkVolumeState := func() error {
		f, err := d.GetVolumeByID(ctx, volume.VolumeID)
		if err != nil {
			volumeState = ""
			return fmt.Errorf("could not get volume status; %v", err)
		}

		volumeState = f.LifeCycleState

		if utils.SliceContainsString(desiredStates, volumeState) {
			return nil
		}

		if f.LifeCycleStateDetails != "" {
			err = fmt.Errorf("volume state is %s, not any of %s: %s",
				f.LifeCycleState, desiredStates, f.LifeCycleStateDetails)
		} else {
			err = fmt.Errorf("volume state is %s, not any of %s", f.LifeCycleState, desiredStates)
		}

		// Return a permanent error to stop retrying if we reached one of the abort states
		for _, abortState := range abortStates {
			if volumeState == abortState {
				return backoff.Permanent(TerminalState(err))
			}
		}

		return err
	}
	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Waiting for volume state.")
	}
	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = backoff.DefaultInitialInterval
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredStates", desiredStates).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			Logc(ctx).Errorf("Volume reached terminal state: %v", terminalStateErr)
		} else {
			Logc(ctx).Errorf("Volume state was not any of %s after %3.2f seconds.",
				desiredStates, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredStates", desiredStates).Debug("Desired volume state reached.")

	return volumeState, nil
}

func (d *Client) CreateVolume(ctx context.Context, request *VolumeCreateRequest) error {
	resourcePath := "/Volumes"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	// var volume Volume
	// err = json.Unmarshal(responseBody, &volume)
	// if err != nil {
	//	return fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	// }

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Volume created.")

	return nil
}

func (d *Client) RenameVolume(ctx context.Context, volume *Volume, newName string) (*Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	request := &VolumeRenameRequest{
		Name:          newName,
		Region:        volume.Region,
		CreationToken: volume.CreationToken,
		ServiceLevel:  volume.ServiceLevel,
		QuotaInBytes:  volume.QuotaInBytes,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(responseBody, volume)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Volume renamed.")

	return volume, nil
}

func (d *Client) ChangeVolumeUnixPermissions(ctx context.Context, volume *Volume, newUnixPermissions string) (*Volume,
	error,
) {
	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	request := &VolumeChangeUnixPermissionsRequest{
		Region:          volume.Region,
		CreationToken:   volume.CreationToken,
		UnixPermissions: newUnixPermissions,
		QuotaInBytes:    volume.QuotaInBytes,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(responseBody, volume)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":            volume.Name,
		"unixPermissions": newUnixPermissions,
		"creationToken":   request.CreationToken,
		"statusCode":      response.StatusCode,
	}).Debug("Volume permissions changed.")

	return volume, nil
}

func (d *Client) RelabelVolume(ctx context.Context, volume *Volume, labels []string) (*Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	request := &VolumeRenameRelabelRequest{
		Region:        volume.Region,
		CreationToken: volume.CreationToken,
		ProtocolTypes: volume.ProtocolTypes,
		Labels:        labels,
		QuotaInBytes:  volume.QuotaInBytes,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(responseBody, volume)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Volume relabeled.")

	return volume, nil
}

func (d *Client) RenameRelabelVolume(
	ctx context.Context, volume *Volume, newName string, labels []string,
) (*Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	request := &VolumeRenameRelabelRequest{
		Name:          newName,
		Region:        volume.Region,
		CreationToken: volume.CreationToken,
		ProtocolTypes: volume.ProtocolTypes,
		// ServiceLevel:  volume.ServiceLevel,
		Labels: labels,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(responseBody, volume)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Volume renamed & relabeled.")

	return volume, nil
}

func (d *Client) ResizeVolume(ctx context.Context, volume *Volume, newSizeBytes int64) (*Volume, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	request := &VolumeResizeRequest{
		Region:        volume.Region,
		CreationToken: volume.CreationToken,
		ProtocolTypes: volume.ProtocolTypes,
		QuotaInBytes:  newSizeBytes,
		ServiceLevel:  volume.ServiceLevel,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(responseBody, volume)
	if err != nil {
		return nil, fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"size":          newSizeBytes,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Volume resized.")

	return volume, nil
}

func (d *Client) DeleteVolume(ctx context.Context, volume *Volume) error {
	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"volume": volume.CreationToken,
	}).Info("Volume deleted.")

	return nil
}

func (d *Client) GetSnapshotsForVolume(ctx context.Context, volume *Volume) (*[]Snapshot, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s/Snapshots", volume.VolumeID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read snapshots")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var snapshots []Snapshot
	err = json.Unmarshal(responseBody, &snapshots)
	if err != nil {
		return nil, fmt.Errorf("could not parse snapshot data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithField("count", len(snapshots)).Debug("Read volume snapshots.")

	return &snapshots, nil
}

func (d *Client) GetSnapshotForVolume(ctx context.Context, volume *Volume, snapshotName string) (*Snapshot, error) {
	snapshots, err := d.GetSnapshotsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range *snapshots {
		if snapshot.Name == snapshotName {

			Logc(ctx).WithFields(log.Fields{
				"snapshot": snapshotName,
				"volume":   volume.CreationToken,
			}).Debug("Found volume snapshot.")

			return &snapshot, nil
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshot": snapshotName,
		"volume":   volume.CreationToken,
	}).Error("Snapshot not found.")

	return nil, fmt.Errorf("snapshot %s not found", snapshotName)
}

func (d *Client) GetSnapshotByID(ctx context.Context, snapshotId string) (*Snapshot, error) {
	resourcePath := fmt.Sprintf("/Snapshots/%s", snapshotId)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to get snapshot")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var snapshot Snapshot
	err = json.Unmarshal(responseBody, &snapshot)
	if err != nil {
		return nil, fmt.Errorf("could not parse snapshot data: %s; %v", string(responseBody), err)
	}

	return &snapshot, nil
}

func (d *Client) WaitForSnapshotState(
	ctx context.Context, snapshot *Snapshot, desiredState string, abortStates []string, maxElapsedTime time.Duration,
) error {
	checkSnapshotState := func() error {
		s, err := d.GetSnapshotByID(ctx, snapshot.SnapshotID)
		if err != nil {
			return fmt.Errorf("could not get snapshot status; %v", err)
		}

		if s.LifeCycleState == desiredState {
			return nil
		}

		if s.LifeCycleStateDetails != "" {
			err = fmt.Errorf("snapshot state is %s, not %s: %s",
				s.LifeCycleState, desiredState, s.LifeCycleStateDetails)
		} else {
			err = fmt.Errorf("snapshot state is %s, not %s", s.LifeCycleState, desiredState)
		}

		// Return a permanent error to stop retrying if we reached one of the abort states
		for _, abortState := range abortStates {
			if s.LifeCycleState == abortState {
				return backoff.Permanent(TerminalState(err))
			}
		}

		return err
	}
	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Waiting for snapshot state.")
	}
	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 2 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredState", desiredState).Info("Waiting for snapshot state.")

	if err := backoff.RetryNotify(checkSnapshotState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			Logc(ctx).Errorf("Snapshot reached terminal state: %v", terminalStateErr)
		} else {
			Logc(ctx).Errorf("Snapshot state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debug("Desired snapshot state reached.")

	return nil
}

func (d *Client) CreateSnapshot(ctx context.Context, request *SnapshotCreateRequest) error {
	resourcePath := "/Snapshots"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"name":       request.Name,
		"statusCode": response.StatusCode,
	}).Info("Volume snapshot created.")

	return nil
}

func (d *Client) RestoreSnapshot(ctx context.Context, volume *Volume, snapshot *Snapshot) error {
	resourcePath := fmt.Sprintf("/Volumes/%s/Revert", volume.VolumeID)

	snapshotRevertRequest := &SnapshotRevertRequest{
		Name:   snapshot.Name,
		Region: volume.Region,
	}

	jsonRequest, err := json.Marshal(snapshotRevertRequest)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", snapshotRevertRequest, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	if err = d.getErrorFromAPIResponse(response, responseBody); err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshot": snapshot.Name,
		"volume":   volume.CreationToken,
	}).Info("Volume reverted to snapshot.")

	return nil
}

func (d *Client) DeleteSnapshot(ctx context.Context, volume *Volume, snapshot *Snapshot) error {
	resourcePath := fmt.Sprintf("/Volumes/%s/Snapshots/%s", volume.VolumeID, snapshot.SnapshotID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete snapshot")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshot": snapshot.Name,
		"volume":   volume.CreationToken,
	}).Info("Deleted volume snapshot.")

	return nil
}

func (d *Client) GetBackupsForVolume(ctx context.Context, volume *Volume) (*[]Backup, error) {
	resourcePath := fmt.Sprintf("/Volumes/%s/Backups", volume.VolumeID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read backups")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var backups []Backup
	err = json.Unmarshal(responseBody, &backups)
	if err != nil {
		return nil, fmt.Errorf("could not parse backup data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithField("count", len(backups)).Debug("Read volume backups.")

	return &backups, nil
}

func (d *Client) GetBackupForVolume(ctx context.Context, volume *Volume, backupName string) (*Backup, error) {
	backups, err := d.GetBackupsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	for _, backup := range *backups {
		if backup.Name == backupName {

			Logc(ctx).WithFields(log.Fields{
				"backup": backupName,
				"volume": volume.CreationToken,
			}).Debug("Found volume backup.")

			return &backup, nil
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"backup": backupName,
		"volume": volume.CreationToken,
	}).Error("Backup not found.")

	return nil, fmt.Errorf("backup %s not found", backupName)
}

func (d *Client) GetBackupByID(ctx context.Context, backupId string) (*Backup, error) {
	resourcePath := fmt.Sprintf("/Backups/%s", backupId)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to get backup")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var backup Backup
	err = json.Unmarshal(responseBody, &backup)
	if err != nil {
		return nil, fmt.Errorf("could not parse backup data: %s; %v", string(responseBody), err)
	}

	return &backup, nil
}

func (d *Client) WaitForBackupStates(
	ctx context.Context, backup *Backup, desiredStates, abortStates []string, maxElapsedTime time.Duration,
) error {
	var b *Backup
	var err error

	checkBackupState := func() error {
		b, err = d.GetBackupByID(ctx, backup.BackupID)
		if err != nil {
			return fmt.Errorf("could not get backup status; %v", err)
		}

		if utils.SliceContainsString(desiredStates, b.LifeCycleState) {
			return nil
		}

		if b.LifeCycleStateDetails != "" {
			err = fmt.Errorf("backup state is %s, not any of %s: %s",
				b.LifeCycleState, desiredStates, b.LifeCycleStateDetails)
		} else {
			err = fmt.Errorf("backup state is %s, not any of %s", b.LifeCycleState, desiredStates)
		}

		// Return a permanent error to stop retrying if we reached one of the abort states
		for _, abortState := range abortStates {
			if b.LifeCycleState == abortState {
				return backoff.Permanent(TerminalState(err))
			}
		}

		return err
	}
	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Waiting for backup state.")
	}
	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 2 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredStates", desiredStates).Info("Waiting for backup state.")

	if err := backoff.RetryNotify(checkBackupState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			Logc(ctx).Errorf("Backup reached terminal state: %v", terminalStateErr)
		} else {
			Logc(ctx).Errorf("Backup state was not any of %s after %3.2f seconds.",
				desiredStates, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"state":         b.LifeCycleState,
		"desiredStates": desiredStates,
	}).Debug("Desired backup state reached.")

	return nil
}

func (d *Client) CreateBackup(ctx context.Context, request *BackupCreateRequest) error {
	resourcePath := "/Backups"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"name":       request.Name,
		"statusCode": response.StatusCode,
	}).Info("Volume backup created.")

	return nil
}

func (d *Client) DeleteBackup(ctx context.Context, volume *Volume, backup *Backup) error {
	resourcePath := fmt.Sprintf("/Volumes/%s/Backups/%s", volume.VolumeID, backup.BackupID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete backup")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"backup": backup.Name,
		"volume": volume.CreationToken,
	}).Info("Deleted volume backup.")

	return nil
}

func (d *Client) getErrorFromAPIResponse(response *http.Response, responseBody []byte) error {
	if response.StatusCode >= 300 {
		// Parse JSON error data
		var responseData CallResponseError
		if err := json.Unmarshal(responseBody, &responseData); err != nil {
			return fmt.Errorf("could not parse API error response: %s; %v", string(responseBody), err)
		} else {
			switch response.StatusCode {
			case http.StatusNotFound:
				return utils.NotFoundError(responseData.Message)
			default:
				return Error{response.StatusCode, responseData.Code, responseData.Message}
			}
		}
	} else {
		return nil
	}
}

func IsValidUserServiceLevel(userServiceLevel string) bool {
	switch userServiceLevel {
	case UserServiceLevel1, UserServiceLevel2, UserServiceLevel3:
		return true
	default:
		return false
	}
}

func UserServiceLevelFromAPIServiceLevel(apiServiceLevel string) string {
	switch apiServiceLevel {
	default:
		fallthrough
	case APIServiceLevel1:
		return UserServiceLevel1
	case APIServiceLevel2:
		return UserServiceLevel2
	case APIServiceLevel3:
		return UserServiceLevel3
	}
}

func GCPAPIServiceLevelFromUserServiceLevel(userServiceLevel string) string {
	switch userServiceLevel {
	default:
		fallthrough
	case UserServiceLevel1:
		return APIServiceLevel1
	case UserServiceLevel2:
		return APIServiceLevel2
	case UserServiceLevel3:
		return APIServiceLevel3
	}
}

func IsTransitionalState(volumeState string) bool {
	switch volumeState {
	case StateCreating, StateUpdating, StateDeleting, StateRestoring:
		return true
	default:
		return false
	}
}

// TerminalStateError signals that the object is in a terminal state.  This is used to stop waiting on
// an object to change state.
type TerminalStateError struct {
	Err error
}

func (e *TerminalStateError) Error() string {
	return e.Err.Error()
}

// TerminalState wraps the given err in a *TerminalStateError.
func TerminalState(err error) *TerminalStateError {
	return &TerminalStateError{
		Err: err,
	}
}
