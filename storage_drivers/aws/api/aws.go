// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This package provides a high-level interface to the NetApp AWS Cloud Volumes NFS REST API.
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

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
)

// Sample curl command to invoke the REST interface:
// curl -H "Api-Key:<apiKey>" -H "Secret-Key:<secretKey>" https://cds-aws-bundles.netapp.com:8080/v1/Snapshots

const httpTimeoutSeconds = 30
const retryTimeoutSeconds = 30
const VolumeCreateTimeout = 10 * time.Second
const DefaultTimeout = 120 * time.Second
const MaxLabelLength = 255

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {

	// AWS CVS API authentication parameters
	APIURL    string
	APIKey    string
	SecretKey string
	ProxyURL  string

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
func (d *Client) InvokeAPI(ctx context.Context, requestBody []byte, method string, awsURL string) (*http.Response,
	[]byte, error) {

	var request *http.Request
	var response *http.Response
	var err error

	if requestBody == nil {
		request, err = http.NewRequestWithContext(ctx, method, awsURL, nil)
	} else {
		request, err = http.NewRequestWithContext(ctx, method, awsURL, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("API-Key", d.config.APIKey)
	request.Header.Set("Secret-Key", d.config.SecretKey)
	request.Header.Set("X-Request-ID", fmt.Sprint(ctx.Value(ContextKeyRequestID)))

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
	response, err = d.invokeAPINoRetry(client, request)

	if response == nil && err != nil {
		Logc(ctx).Warnf("Error communicating with AWS REST interface. %v", err)
		return nil, nil, err
	} else if response != nil {
		defer func() { _ = response.Body.Close() }()
	} else {
		Logc(ctx).Warnf("Error communicating with AWS REST interface. nil response")
		return nil, nil, fmt.Errorf("nil response")
	}

	var responseBody []byte
	if err == nil {

		responseBody, err = ioutil.ReadAll(response.Body)

		if d.config.DebugTraceFlags["api"] {
			utils.LogHTTPResponse(ctx, response, responseBody, false)
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

	versionURL, err := url.Parse(d.config.APIURL)
	if err != nil {
		return nil, nil, err
	}
	versionURL.Path = "/version"

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", versionURL.String())
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

func (d *Client) GetRegions(ctx context.Context) (*[]Region, error) {

	resourcePath := "/Storage/Regions"

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
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

	Logc(ctx).WithField("count", len(regions)).Info("Read regions.")

	return &regions, nil
}

func (d *Client) GetVolumes(ctx context.Context) (*[]FileSystem, error) {

	resourcePath := "/FileSystems"

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
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

	Logc(ctx).WithField("count", len(filesystems)).Debug("Read filesystems.")

	return &filesystems, nil
}

func (d *Client) GetVolumeByName(ctx context.Context, name string) (*FileSystem, error) {

	filesystems, err := d.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	matchingFilesystems := make([]FileSystem, 0)

	for _, filesystem := range *filesystems {
		if filesystem.Name == name {
			matchingFilesystems = append(matchingFilesystems, filesystem)
		}
	}

	if len(matchingFilesystems) == 0 {
		return nil, fmt.Errorf("filesystem with name %s not found", name)
	} else if len(matchingFilesystems) > 1 {
		return nil, fmt.Errorf("multiple filesystems with name %s found", name)
	}

	return &matchingFilesystems[0], nil
}

func (d *Client) GetVolumeByCreationToken(ctx context.Context, creationToken string) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
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
		return nil, utils.NotFoundError(fmt.Sprintf("filesystem with creationToken %s not found", creationToken))
	} else if len(filesystems) > 1 {
		return nil, fmt.Errorf("multiple filesystems with creationToken %s found", creationToken)
	}

	return &filesystems[0], nil
}

func (d *Client) VolumeExistsByCreationToken(ctx context.Context, creationToken string) (bool, *FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems?creationToken=%s", creationToken)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
	if err != nil {
		return false, nil, errors.New("failed to get filesystem")
	}

	if err := d.getErrorFromAPIResponse(response, responseBody); err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
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

func (d *Client) GetVolumeByID(ctx context.Context, fileSystemId string) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s", fileSystemId)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
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

func (d *Client) WaitForVolumeState(
	ctx context.Context, filesystem *FileSystem, desiredState string, abortStates []string,
	maxElapsedTime time.Duration,
) (string, error) {

	volumeState := ""

	checkVolumeState := func() error {

		f, err := d.GetVolumeByID(ctx, filesystem.FileSystemID)
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

	Logc(ctx).WithField("desiredState", desiredState).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			Logc(ctx).Errorf("Volume reached terminal state: %v", terminalStateErr)
		} else {
			Logc(ctx).Errorf("Volume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debug("Desired volume state reached.")

	return volumeState, nil
}

func (d *Client) CreateVolume(ctx context.Context, request *FilesystemCreateRequest) (*FileSystem, error) {

	resourcePath := "/FileSystems"

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", d.makeURL(resourcePath))
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

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Filesystem created.")

	return &filesystem, nil
}

func (d *Client) RenameVolume(ctx context.Context, filesystem *FileSystem, newName string) (*FileSystem, error) {

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

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
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

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Filesystem renamed.")

	return filesystem, nil
}

func (d *Client) RelabelVolume(ctx context.Context, filesystem *FileSystem, labels []string) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s", filesystem.FileSystemID)

	request := &FilesystemRenameRelabelRequest{
		Region:        filesystem.Region,
		CreationToken: filesystem.CreationToken,
		ServiceLevel:  filesystem.ServiceLevel,
		Labels:        labels,
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

	err = json.Unmarshal(responseBody, filesystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Filesystem relabeled.")

	return filesystem, nil
}

func (d *Client) RenameRelabelVolume(
	ctx context.Context, filesystem *FileSystem, newName string, labels []string,
) (*FileSystem, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s", filesystem.FileSystemID)

	request := &FilesystemRenameRelabelRequest{
		Name:          newName,
		Region:        filesystem.Region,
		CreationToken: filesystem.CreationToken,
		ServiceLevel:  filesystem.ServiceLevel,
		Labels:        labels,
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

	err = json.Unmarshal(responseBody, filesystem)
	if err != nil {
		return nil, fmt.Errorf("could not parse filesystem data: %s; %v", string(responseBody), err)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Debug("Filesystem renamed & relabeled.")

	return filesystem, nil
}

func (d *Client) ResizeVolume(ctx context.Context, filesystem *FileSystem, newSizeBytes int64) (*FileSystem, error) {

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

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "PUT", d.makeURL(resourcePath))
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

	Logc(ctx).WithFields(log.Fields{
		"size":          newSizeBytes,
		"creationToken": request.CreationToken,
		"statusCode":    response.StatusCode,
	}).Info("Filesystem resized.")

	return filesystem, nil
}

func (d *Client) DeleteVolume(ctx context.Context, filesystem *FileSystem) error {

	resourcePath := fmt.Sprintf("/FileSystems/%s", filesystem.FileSystemID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "DELETE", d.makeURL(resourcePath))
	if err != nil {
		return errors.New("failed to delete volume")
	}

	err = d.getErrorFromAPIResponse(response, responseBody)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"volume": filesystem.CreationToken,
	}).Info("Filesystem deleted.")

	return nil
}

func (d *Client) GetMountTargetsForVolume(ctx context.Context, filesystem *FileSystem) (*[]MountTarget, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s/MountTargets", filesystem.FileSystemID)

	response, responseBody, err := d.InvokeAPI(ctx, nil, "GET", d.makeURL(resourcePath))
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

	Logc(ctx).WithField("count", len(mountTargets)).Debug("Read mount targets for filesystem.")

	return &mountTargets, nil
}

func (d *Client) GetSnapshotsForVolume(ctx context.Context, filesystem *FileSystem) (*[]Snapshot, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s/Snapshots", filesystem.FileSystemID)

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

	Logc(ctx).WithField("count", len(snapshots)).Debug("Read filesystem snapshots.")

	return &snapshots, nil
}

func (d *Client) GetSnapshotForVolume(
	ctx context.Context, filesystem *FileSystem, snapshotName string,
) (*Snapshot, error) {

	snapshots, err := d.GetSnapshotsForVolume(ctx, filesystem)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range *snapshots {
		if snapshot.Name == snapshotName {

			Logc(ctx).WithFields(log.Fields{
				"snapshot":   snapshotName,
				"filesystem": filesystem.CreationToken,
			}).Debug("Found filesystem snapshot.")

			return &snapshot, nil
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshot":   snapshotName,
		"filesystem": filesystem.CreationToken,
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

func (d *Client) CreateSnapshot(ctx context.Context, request *SnapshotCreateRequest) (*Snapshot, error) {

	resourcePath := fmt.Sprintf("/FileSystems/%s/Snapshots", request.FileSystemID)

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("could not marshal JSON request: %v; %v", request, err)
	}

	response, responseBody, err := d.InvokeAPI(ctx, jsonRequest, "POST", d.makeURL(resourcePath))
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

	Logc(ctx).WithFields(log.Fields{
		"name":       request.Name,
		"statusCode": response.StatusCode,
	}).Info("Filesystem snapshot created.")

	return &snapshot, nil
}

func (d *Client) RestoreSnapshot(ctx context.Context, filesystem *FileSystem, snapshot *Snapshot) error {

	resourcePath := fmt.Sprintf("/FileSystems/%s/Revert", filesystem.FileSystemID)

	snapshotRevertRequest := &SnapshotRevertRequest{
		FileSystemID: filesystem.FileSystemID,
		Region:       filesystem.Region,
		SnapshotID:   snapshot.SnapshotID,
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
		"snapshot":   snapshot.Name,
		"filesystem": filesystem.CreationToken,
	}).Info("Filesystem reverted to snapshot.")

	return nil
}

func (d *Client) DeleteSnapshot(ctx context.Context, filesystem *FileSystem, snapshot *Snapshot) error {

	resourcePath := fmt.Sprintf("/FileSystems/%s/Snapshots/%s", filesystem.FileSystemID, snapshot.SnapshotID)

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
		"volume":   filesystem.CreationToken,
	}).Info("Deleted filesystem snapshot.")

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
