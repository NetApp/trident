// Copyright 2019 NetApp, Inc. All Rights Reserved.

// This package provides a high-level interface to the NetApp AWS Cloud Volumes NFS REST API.
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

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

// Sample curl command to invoke the REST interface:
// curl -H "Api-Key:<apiKey>" -H "Secret-Key:<secretKey>" https://cds-aws-bundles.netapp.com:8080/v1/Snapshots

const httpTimeoutSeconds = 30
const retryTimeoutSeconds = 30
const createTimeoutSeconds = 110

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {

	// AWS CVS API authentication parameters
	APIURL    string
	APIKey    string
	SecretKey string

	// Options
	DebugTraceFlags map[string]bool
}

type Client struct {
	config *ClientConfig
	m      *sync.Mutex
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
	return fmt.Sprintf("%s%s", d.config.APIURL, resourcePath)
}

// InvokeAPI makes a REST call to the cloud volumes REST service. The body must be a marshaled JSON byte array (or nil).
// The method is the HTTP verb (i.e. GET, POST, ...).
func (d *Client) InvokeAPI(requestBody []byte, method string, url string) (*http.Response, []byte, error) {

	var request *http.Request
	var response *http.Response
	var err error

	if requestBody == nil {
		request, err = http.NewRequest(method, url, nil)
	} else {
		request, err = http.NewRequest(method, url, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("API-Key", d.config.APIKey)
	request.Header.Set("Secret-Key", d.config.SecretKey)

	// Allow certificate validation override
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	if d.config.DebugTraceFlags["api"] {
		utils.LogHTTPRequest(request, requestBody)
	}

	// Send the request
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(httpTimeoutSeconds * time.Second),
	}
	response, err = d.invokeAPINoRetry(client, request)

	if response == nil && err != nil {
		log.Warnf("Error communicating with AWS REST interface. %v", err)
		return nil, nil, err
	}
	defer response.Body.Close()

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

	versionURL, err := url.Parse(d.config.APIURL)
	if err != nil {
		return nil, nil, err
	}
	versionURL.Path = "/version"

	response, responseBody, err := d.InvokeAPI(nil, "GET", versionURL.String())
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
	}).Debug("Read CVS version.")

	return apiVersion, sdeVersion, nil
}

func (d *Client) GetRegions() (*[]Region, error) {

	resourcePath := "/Storage/Regions"

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read regions")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var regions []Region
	err = json.Unmarshal(responseBody, &regions)
	if err != nil {
		return nil, fmt.Errorf("could not parse region data: %s; %v", string(responseBody), err)
	}

	log.WithField("count", len(regions)).Debug("Read regions.")

	return &regions, nil
}

func (d *Client) GetVolumes() (*[]FileSystem, error) {

	resourcePath := "/FileSystems"

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read filesystems")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var filesystems []FileSystem
	err = json.Unmarshal(responseBody, &filesystems)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	log.WithField("count", len(filesystems)).Debug("Read filesystems.")

	return &filesystems, nil
}

func (d *Client) GetVolumeByCreationToken(creationToken string) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to get filesystem")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var filesystems []FileSystem
	err = json.Unmarshal(responseBody, &filesystems)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	if len(filesystems) == 0 {
		return nil, fmt.Errorf("filesystem with creationToken %s not found", creationToken)
	} else if len(filesystems) > 1 {
		return nil, fmt.Errorf("multiple filesystems with creationToken %s found", creationToken)
	}

	return &filesystems[0], nil
}

func (d *Client) VolumeExistsByCreationToken(creationToken string) (bool, *FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return false, nil, errors.New("failed to get filesystem")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return false, nil, err
	}

	var filesystems []FileSystem
	err = json.Unmarshal(responseBody, &filesystems)
	if err != nil {
		return false, nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	if len(filesystems) == 0 {
		return false, nil, nil
	}
	return true, &filesystems[0], nil
}

func (d *Client) GetVolumeByID(fileSystemId string) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s", fileSystemId)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to get filesystem")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var filesystem FileSystem
	err = json.Unmarshal(responseBody, &filesystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	return &filesystem, nil
}

func (d *Client) WaitForVolumeState(filesystem *FileSystem, desiredState string, abortStates []string) error {

	checkVolumeState := func() error {

		f, err := d.GetVolumeByID(filesystem.FileSystemID)
		if err != nil {
			return fmt.Errorf("could not get volume status; %v", err)
		}

		if f.LifeCycleState == desiredState {
			return nil
		}

		if f.LifeCycleStateDetails != "" {
			err = fmt.Errorf("volume state is %s, not %s: %s",
				f.LifeCycleState, desiredState, f.LifeCycleStateDetails)
		} else {
			err = fmt.Errorf("volume state is %s, not %s", f.LifeCycleState, desiredState)
		}

		// Return a permanent error to stop retrying if we reached one of the abort states
		for _, abortState := range abortStates {
			if f.LifeCycleState == abortState {
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
	stateBackoff.MaxElapsedTime = createTimeoutSeconds * time.Second
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 2 * time.Second
	stateBackoff.Multiplier = 1.414

	log.WithField("desiredState", desiredState).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			log.Errorf("Volume reached terminal state: %v", terminalStateErr)
		} else {
			log.Errorf("Volume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	log.WithField("desiredState", desiredState).Debug("Desired volume state reached.")

	return nil
}

func (d *Client) CreateVolume(request *FilesystemCreateRequest) (*FileSystem, error) {

	resourcePath := "/FileSystems"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var filesystem FileSystem
	err = json.Unmarshal(responseBody, &filesystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	log.WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Filesystem created.")

	return &filesystem, nil
}

func (d *Client) RenameVolume(filesystem *FileSystem, newName string) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s", filesystem.FileSystemID)

	request := &FilesystemRenameRequest{
		Name:          newName,
		Region:        filesystem.Region,
		CreationToken: filesystem.CreationToken,
		ServiceLevel:  filesystem.ServiceLevel,
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

	err = json.Unmarshal(responseBody, filesystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	log.WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Filesystem renamed.")

	return filesystem, nil
}

func (d *Client) ResizeVolume(filesystem *FileSystem, newSizeBytes int64) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s", filesystem.FileSystemID)

	request := &FilesystemResizeRequest{
		Region:        filesystem.Region,
		CreationToken: filesystem.CreationToken,
		QuotaInBytes:  newSizeBytes,
		ServiceLevel:  filesystem.ServiceLevel,
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

	err = json.Unmarshal(responseBody, filesystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	log.WithFields(log.Fields{
		"size":          newSizeBytes,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Filesystem resized.")

	return filesystem, nil
}

func (d *Client) DeleteVolume(filesystem *FileSystem) error {

	resourcePath := fmt.Sprintf("/FileSystems/%s", filesystem.FileSystemID)

	response, responseBody, err := d.InvokeAPI(nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"volume": filesystem.CreationToken,
	}).Debug("Deleted volume.")

	return nil
}

func (d *Client) GetMountTargetsForVolume(filesystem *FileSystem) (*[]MountTarget, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s/MountTargets", filesystem.FileSystemID)

	response, responseBody, err := d.InvokeAPI(nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return nil, errors.New("failed to read mount targets")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return nil, err
	}

	var mountTargets []MountTarget
	err = json.Unmarshal(responseBody, &mountTargets)
	if err != nil {
		return nil, fmt.Errorf("could not parse mount target data: %s; %v", string(responseBody), err)
	}

	log.WithField("count", len(mountTargets)).Debug("Read mount targets.")

	return &mountTargets, nil
}

func (d *Client) GetSnapshotsForVolume(filesystem *FileSystem) (*[]Snapshot, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s/Snapshots", filesystem.FileSystemID)

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

	log.WithField("count", len(snapshots)).Debug("Read snapshots.")

	return &snapshots, nil
}

func (d *Client) GetSnapshotForVolume(filesystem *FileSystem, snapshotName string) (*Snapshot, error) {

	snapshots, err := d.GetSnapshotsForVolume(filesystem)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range *snapshots {
		if snapshot.Name == snapshotName {

			log.WithFields(log.Fields{
				"snapshot":   snapshotName,
				"filesystem": filesystem.CreationToken,
			}).Debug("Found snapshot.")

			return &snapshot, nil
		}
	}

	log.WithFields(log.Fields{
		"snapshot":   snapshotName,
		"filesystem": filesystem.CreationToken,
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

func (d *Client) WaitForSnapshotState(snapshot *Snapshot, desiredState string, abortStates []string) error {

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
	stateBackoff.MaxElapsedTime = createTimeoutSeconds * time.Second
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

func (d *Client) CreateSnapshot(request *SnapshotCreateRequest) (*Snapshot, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s/Snapshots", request.FileSystemID)

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(jsonRequest, "POST", d.makeURL(resourcePath))
	if err != nil {
		return nil, err
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

	log.WithFields(log.Fields{
		"name":       request.Name,
		"statusCode": response.StatusCode,
	}).Debug("Snapshot created.")

	return &snapshot, nil
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

func APIServiceLevelFromUserServiceLevel(userServiceLevel string) string {
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
