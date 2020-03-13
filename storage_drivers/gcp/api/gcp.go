// Copyright 2019 NetApp, Inc. All Rights Reserved.

// This package provides a high-level interface to the NetApp GCP Cloud Volumes NFS REST API.
package api

import (
	"bytes"
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

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

// Sample curl command to invoke the REST interface:
// Get fresh token from Trident debug logs
// export Bearer="<token>"
// curl https://cloudvolumesgcp-api.netapp.com/version --insecure -X GET -H "Content-Type: application/json" -H "Authorization: Bearer $Bearer"

const httpTimeoutSeconds = 30
const retryTimeoutSeconds = 30
const VolumeCreateTimeout = 10 * time.Second
const DefaultTimeout = 120 * time.Second
const apiServer = "https://cloudvolumesgcp-api.netapp.com"
const apiAudience = apiServer

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {

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
}

type Client struct {
	config      *ClientConfig
	tokenSource *oauth2.TokenSource
	token       *oauth2.Token
	m           *sync.Mutex
}

// NewDriver is a factory method for creating a new instance.
func NewDriver(config ClientConfig) *Client {

	d := &Client{
		config: &config,
		m:      &sync.Mutex{},
	}

	return d
}

func (d *Client) makeURL(resourcePath string) string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s%s", apiServer, d.config.ProjectNumber, d.config.APIRegion, resourcePath)
}

func (d *Client) initTokenSource() error {

	keyBytes, err := json.Marshal(d.config.APIKey)
	if err != nil {
		return err
	}

	tokenSource, err := google.JWTAccessTokenSourceFromJSON(keyBytes, apiAudience)
	if err != nil {
		return err
	}
	d.tokenSource = &tokenSource

	log.Debug("Got token source.")

	return nil
}

func (d *Client) refreshToken() error {

	var err error

	// Get token source (once per API client instance)
	if d.tokenSource == nil {
		if err = d.initTokenSource(); err != nil {
			return err
		}
	}

	// If we have a valid token, just return
	if d.token != nil && d.token.Valid() {

		log.WithFields(log.Fields{
			"type":    d.token.Type(),
			"token":   d.token.AccessToken,
			"expires": d.token.Expiry.String(),
		}).Trace("Token still valid.")

		return nil
	}

	// Get a fresh token
	d.token, err = (*d.tokenSource).Token()
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"type":    d.token.Type(),
		"token":   d.token.AccessToken,
		"expires": d.token.Expiry.String(),
	}).Debug("Got fresh token.")

	return nil
}

// InvokeAPI makes a REST call to the cloud volumes REST service. The body must be a marshaled JSON byte array (or nil).
// The method is the HTTP verb (i.e. GET, POST, ...).
func (d *Client) InvokeAPI(requestBody []byte, method string, gcpURL string) (*http.Response, []byte, error) {

	var request *http.Request
	var response *http.Response
	var err error

	if err = d.refreshToken(); err != nil {
		return nil, nil, fmt.Errorf("cannot invoke API %s, no valid token; %v", gcpURL, err)
	}

	if requestBody == nil {
		request, err = http.NewRequest(method, gcpURL, nil)
	} else {
		request, err = http.NewRequest(method, gcpURL, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	d.token.SetAuthHeader(request)

	tr := &http.Transport{}
	// Use ProxyUrl if set
	proxyURL := d.config.ProxyURL

	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, nil, err
		}

		tr.Proxy = http.ProxyURL(proxy)

		// Skip certificate validation
		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else {

		// Require certificate validation if not using a proxy
		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: false,
		}
	}

	if d.config.DebugTraceFlags["api"] {
		utils.LogHTTPRequest(request, requestBody)
	}

	// Send the request
	client := &http.Client{
		Transport: tr,
		Timeout:   httpTimeoutSeconds * time.Second,
	}
	response, err = d.invokeAPIWithRetry(client, request)

	if response == nil && err != nil {
		log.Warnf("Error communicating with GCP REST interface. %v", err)
		return nil, nil, err
	} else if response != nil {
		defer func() { _ = response.Body.Close() }()
	}

	var responseBody []byte
	if err == nil {

		responseBody, err = ioutil.ReadAll(response.Body)

		if d.config.DebugTraceFlags["api"] {
			utils.LogHTTPResponse(response, responseBody)
		}
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
		log.WithFields(log.Fields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Retrying API.")
	}
	invokeBackoff := backoff.NewExponentialBackOff()
	invokeBackoff.MaxElapsedTime = retryTimeoutSeconds * time.Second

	if err := backoff.RetryNotify(invoke, invokeBackoff, invokeNotify); err != nil {
		log.Errorf("API has not succeeded after %3.2f seconds.", invokeBackoff.MaxElapsedTime.Seconds())
		return response, err
	}

	return response, nil
}

func (d *Client) GetVersion() (*utils.Version, *utils.Version, error) {

	resourcePath := "/version"

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
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

	log.WithFields(log.Fields{
		"apiVersion": apiVersion.String(),
		"sdeVersion": sdeVersion.String(),
	}).Info("Read CVS version.")

	return apiVersion, sdeVersion, nil
}

func (d *Client) GetServiceLevels() (map[string]string, error) {

	resourcePath := "/Storage/ServiceLevels"

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
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

	log.WithFields(log.Fields{
		"count":  len(serviceLevels),
		"levels": serviceLevelMap,
	}).Info("Read service levels.")

	return serviceLevelMap, nil
}

func (d *Client) GetVolumes() (*[]Volume, error) {

	resourcePath := "/Volumes"

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
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

	log.WithField("count", len(volumes)).Debug("Read volumes.")

	return &volumes, nil
}

func (d *Client) GetVolumeByName(name string) (*Volume, error) {

	volumes, err := d.GetVolumes()
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
		return nil, fmt.Errorf("volume with name %s not found", name)
	} else if len(matchingVolumes) > 1 {
		return nil, fmt.Errorf("multiple volumes with name %s found", name)
	}

	return &matchingVolumes[0], nil
}

func (d *Client) GetVolumeByCreationToken(creationToken string) (*Volume, error) {

	resourcePath := fmt.Sprintf("/Volumes?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to get volume")
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
		return nil, fmt.Errorf("volume with creationToken %s not found", creationToken)
	} else if len(volumes) > 1 {
		return nil, fmt.Errorf("multiple volumes with creationToken %s found", creationToken)
	}

	return &volumes[0], nil
}

func (d *Client) VolumeExistsByCreationToken(creationToken string) (bool, *Volume, error) {

	resourcePath := fmt.Sprintf("/Volumes?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return false, nil, errors.New("failed to get volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return false, nil, err
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

func (d *Client) GetVolumeByID(volumeId string) (*Volume, error) {

	resourcePath := fmt.Sprintf("/Volumes/%s", volumeId)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
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

func (d *Client) WaitForVolumeState(
	volume *Volume, desiredState string, abortStates []string, maxElapsedTime time.Duration,
) (string, error) {

	volumeState := ""

	checkVolumeState := func() error {

		f, err := d.GetVolumeByID(volume.VolumeID)
		if err != nil {
			volumeState = ""
			return fmt.Errorf("could not get volume status; %v", err)
		}

		volumeState = f.LifeCycleState

		if volumeState == desiredState {
			return nil
		}

		if f.LifeCycleStateDetails != "" {
			err = fmt.Errorf("volume state is %s, not %s: %s", volumeState, desiredState, f.LifeCycleStateDetails)
		} else {
			err = fmt.Errorf("volume state is %s, not %s", volumeState, desiredState)
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
		log.WithFields(log.Fields{
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

	log.WithField("desiredState", desiredState).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			log.Errorf("Volume reached terminal state: %v", terminalStateErr)
		} else {
			log.Errorf("Volume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	log.WithField("desiredState", desiredState).Debug("Desired volume state reached.")

	return volumeState, nil
}

func (d *Client) CreateVolume(request *VolumeCreateRequest) error {

	resourcePath := "/Volumes"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	//var volume Volume
	//err = json.Unmarshal(responseBody, &volume)
	//if err != nil {
	//	return fmt.Errorf("could not parse volume data: %s; %v", string(responseBody), err)
	//}

	log.WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Volume created.")

	return nil
}

func (d *Client) RenameVolume(volume *Volume, newName string) (*Volume, error) {

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

	response, responseBody, err := d.InvokeAPI(jsonRequest, "PUT", d.makeURL(resourcePath))
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

	log.WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Volume renamed.")

	return volume, nil
}

func (d *Client) RelabelVolume(volume *Volume, labels []string) (*Volume, error) {

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

	response, responseBody, err := d.InvokeAPI(jsonRequest, "PUT", d.makeURL(resourcePath))
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

	log.WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Volume relabeled.")

	return volume, nil
}

func (d *Client) RenameRelabelVolume(volume *Volume, newName string, labels []string) (*Volume, error) {

	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	request := &VolumeRenameRelabelRequest{
		Name:          newName,
		Region:        volume.Region,
		CreationToken: volume.CreationToken,
		ProtocolTypes: volume.ProtocolTypes,
		//ServiceLevel:  volume.ServiceLevel,
		Labels: labels,
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(jsonRequest, "PUT", d.makeURL(resourcePath))
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

	log.WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Volume renamed & relabeled.")

	return volume, nil
}

func (d *Client) ResizeVolume(volume *Volume, newSizeBytes int64) (*Volume, error) {

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

	response, responseBody, err := d.InvokeAPI(jsonRequest, "PUT", d.makeURL(resourcePath))
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

	log.WithFields(log.Fields{
		"size":          newSizeBytes,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Volume resized.")

	return volume, nil
}

func (d *Client) DeleteVolume(volume *Volume) error {

	resourcePath := fmt.Sprintf("/Volumes/%s", volume.VolumeID)

	response, responseBody, err := d.InvokeAPI(nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"volume": volume.CreationToken,
	}).Info("Volume deleted.")

	return nil
}

func (d *Client) GetSnapshotsForVolume(volume *Volume) (*[]Snapshot, error) {

	resourcePath := fmt.Sprintf("/Volumes/%s/Snapshots", volume.VolumeID)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
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

	log.WithField("count", len(snapshots)).Debug("Read volume snapshots.")

	return &snapshots, nil
}

func (d *Client) GetSnapshotForVolume(volume *Volume, snapshotName string) (*Snapshot, error) {

	snapshots, err := d.GetSnapshotsForVolume(volume)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range *snapshots {
		if snapshot.Name == snapshotName {

			log.WithFields(log.Fields{
				"snapshot": snapshotName,
				"volume":   volume.CreationToken,
			}).Debug("Found volume snapshot.")

			return &snapshot, nil
		}
	}

	log.WithFields(log.Fields{
		"snapshot": snapshotName,
		"volume":   volume.CreationToken,
	}).Error("Snapshot not found.")

	return nil, fmt.Errorf("snapshot %s not found", snapshotName)
}

func (d *Client) GetSnapshotByID(snapshotId string) (*Snapshot, error) {

	resourcePath := fmt.Sprintf("/Snapshots/%s", snapshotId)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
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
	snapshot *Snapshot, desiredState string, abortStates []string, maxElapsedTime time.Duration,
) error {

	checkSnapshotState := func() error {

		s, err := d.GetSnapshotByID(snapshot.SnapshotID)
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
		log.WithFields(log.Fields{
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

	log.WithField("desiredState", desiredState).Info("Waiting for snapshot state.")

	if err := backoff.RetryNotify(checkSnapshotState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			log.Errorf("Snapshot reached terminal state: %v", terminalStateErr)
		} else {
			log.Errorf("Snapshot state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	log.WithField("desiredState", desiredState).Debug("Desired snapshot state reached.")

	return nil
}

func (d *Client) CreateSnapshot(request *SnapshotCreateRequest) error {

	resourcePath := "/Snapshots"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	//var snapshot Snapshot
	//err = json.Unmarshal(responseBody, &snapshot)
	//if err != nil {
	//	return nil, fmt.Errorf("could not parse snapshot data: %s; %v", string(responseBody), err)
	//}

	log.WithFields(log.Fields{
		"name":       request.Name,
		"statusCode": response.StatusCode,
	}).Info("Volume snapshot created.")

	return nil
}

func (d *Client) RestoreSnapshot(volume *Volume, snapshot *Snapshot) error {

	resourcePath := fmt.Sprintf("/Volumes/%s/Revert", volume.VolumeID)

	snapshotRevertRequest := &SnapshotRevertRequest{
		Name:   snapshot.Name,
		Region: volume.Region,
	}

	jsonRequest, err := json.Marshal(snapshotRevertRequest)
	if err != nil {
		return fmt.Errorf("could not marshal JSON request: %v; %v", snapshotRevertRequest, err)
	}

	response, responseBody, err := d.InvokeAPI(jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return err
	}

	if err = d.getErrorFromAPIResponse(response, responseBody); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"snapshot": snapshot.Name,
		"volume":   volume.CreationToken,
	}).Info("Volume reverted to snapshot.")

	return nil
}

func (d *Client) DeleteSnapshot(volume *Volume, snapshot *Snapshot) error {

	resourcePath := fmt.Sprintf("/Volumes/%s/Snapshots/%s", volume.VolumeID, snapshot.SnapshotID)

	response, responseBody, err := d.InvokeAPI(nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete snapshot")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"snapshot": snapshot.Name,
		"volume":   volume.CreationToken,
	}).Info("Deleted volume snapshot.")

	return nil
}

func (d *Client) getErrorFromAPIResponse(response *http.Response, responseBody []byte) error {

	if response.StatusCode >= 300 {
		// Parse JSON error data
		var responseData CallResponseError
		if err := json.Unmarshal(responseBody, &responseData); err != nil {
			return fmt.Errorf("could not parse API error response: %s; %v", string(responseBody), err)
		} else {
			return Error{response.StatusCode, responseData.Code, responseData.Message}
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
	case StateCreating, StateUpdating, StateDeleting:
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
