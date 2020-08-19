// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
)

// ListVolumesForAccount tbd
func (c *Client) ListVolumesForAccount(
	ctx context.Context, listReq *ListVolumesForAccountRequest,
) (volumes []Volume, err error) {

	response, err := c.Request(ctx, "ListVolumesForAccount", listReq, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error detected in ListVolumesForAccount API response: %+v", err)
		return nil, errors.New("device API error")
	}
	var result ListVolumesResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling ListVolumesForAccount API response: %+v", err)
		return nil, errors.New("json-decode error")
	}
	volumes = result.Result.Volumes
	return volumes, err
}

// GetVolumeByID returns the volume with the specified ID.
func (c *Client) GetVolumeByID(ctx context.Context, volID int64) (Volume, error) {

	var limit int64 = 1

	listRequest := &ListVolumesRequest{
		Accounts:      []int64{c.AccountID},
		StartVolumeID: &volID,
		Limit:         &limit,
	}

	volumes, err := c.ListVolumes(ctx, listRequest)
	if err != nil {
		return Volume{}, err
	}

	// This API isn't guaranteed to return the volume being sought, so make sure the result matches the request!
	if volumes == nil || len(volumes) == 0 || volumes[0].VolumeID != volID {
		return Volume{}, fmt.Errorf("volume %d not found", volID)
	}

	return volumes[0], nil
}

// WaitForVolumeByID polls for the volume with specified ID to appear, with backoff retry logic.
func (c *Client) WaitForVolumeByID(ctx context.Context, volID int64) (Volume, error) {

	volume := Volume{}

	checkVolumeExists := func() error {
		var err error
		volume, err = c.GetVolumeByID(ctx, volID)
		if err != nil {
			return fmt.Errorf("volume %d does not yet exist; %v", volID, err)
		}
		return nil
	}
	volumeExistsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Volume not yet present, waiting.")
	}
	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 2 * time.Second
	volumeBackoff.Multiplier = 1.414
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = 30 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeExists, volumeBackoff, volumeExistsNotify); err != nil {
		Logc(ctx).WithField("volumeID", volID).Warnf(
			"Could not find volume after %3.2f seconds.", volumeBackoff.MaxElapsedTime.Seconds())
		return volume, fmt.Errorf("volume %d does not exist", volID)
	} else {
		Logc(ctx).WithField("volumeID", volID).Debug("Volume found.")
		return volume, nil
	}
}

// ListVolumes returns all volumes using the specified request object.
func (c *Client) ListVolumes(ctx context.Context, listVolReq *ListVolumesRequest) (volumes []Volume, err error) {

	response, err := c.Request(ctx, "ListVolumes", listVolReq, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error response from ListVolumes request: %v ", err)
		return nil, errors.New("device API error")
	}
	var result ListVolumesResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling ListVolumes API response: %v", err)
		return nil, errors.New("json-decode error")
	}
	volumes = result.Result.Volumes
	return volumes, err
}

// CloneVolume invokes the supplied clone volume request.  It waits for the source volume
// (which itself may be new in a test scenario) to be ready to be cloned, and it waits for
// the clone to exist.
func (c *Client) CloneVolume(ctx context.Context, req *CloneVolumeRequest) (Volume, error) {

	var (
		cloneError error
		response   []byte
		result     CloneVolumeResult
	)

	cloneExists := func() error {
		response, cloneError = c.Request(ctx, "CloneVolume", req, NewReqID())
		if cloneError != nil {
			errorMessage := cloneError.Error()
			if strings.Contains(errorMessage, "SliceNotRegistered") {
				return fmt.Errorf("detected SliceNotRegistered on Clone operation")
			} else if strings.Contains(errorMessage, "xInvalidParameter") {
				return fmt.Errorf("detected xInvalidParameter on Clone operation")
			} else if strings.Contains(errorMessage, "xNotReadyForIO") {
				return fmt.Errorf("detected xNotReadyForIO on Clone operation")
			} else {
				Logc(ctx).Debugf("encountered err: %s during volume clone operation", cloneError)
				backoff.Permanent(cloneError)
			}
		}
		return nil
	}
	cloneExistsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debugf("Clone not yet present, waiting; err: %+v", err)
	}

	cloneBackoff := backoff.NewExponentialBackOff()
	cloneBackoff.InitialInterval = 2 * time.Second
	cloneBackoff.Multiplier = 1.414
	cloneBackoff.RandomizationFactor = 0.1
	cloneBackoff.MaxElapsedTime = 30 * time.Second
	cloneBackoff.MaxInterval = 5 * time.Second

	// Sometimes it can take a few seconds for the Slice to finalize even though the Volume reports ready.
	if err := backoff.RetryNotify(cloneExists, cloneBackoff, cloneExistsNotify); err != nil {
		Logc(ctx).WithField("clone", cloneBackoff).Warnf("Failed to clone volume after %3.2f seconds.",
			cloneBackoff.MaxElapsedTime.Seconds())
		return Volume{}, fmt.Errorf("failed to clone volume: %s", req.Name)
	}

	Logc(ctx).Info("Clone request succeeded")

	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling CloneVolume API response: %v", err)
		return Volume{}, errors.New("json-decode error")
	}

	return c.WaitForVolumeByID(ctx, result.Result.VolumeID)
}

// CreateVolume tbd
func (c *Client) CreateVolume(ctx context.Context, createReq *CreateVolumeRequest) (Volume, error) {

	response, err := c.Request(ctx, "CreateVolume", createReq, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error response from CreateVolume request: %v ", err)
		return Volume{}, errors.New("device API error")
	}
	var result CreateVolumeResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling CreateVolume API response: %v", err)
		return Volume{}, errors.New("json-decode error")
	}

	return c.WaitForVolumeByID(ctx, result.Result.VolumeID)
}

// AddVolumesToAccessGroup tbd
func (c *Client) AddVolumesToAccessGroup(ctx context.Context, req *AddVolumesToVolumeAccessGroupRequest) (err error) {

	_, err = c.Request(ctx, "AddVolumesToVolumeAccessGroup", req, NewReqID())
	if err != nil {
		if apiErr, ok := err.(Error); ok && apiErr.Fields.Name == "xAlreadyInVolumeAccessGroup" {
			return nil
		}
		Logc(ctx).Errorf("error response from Add to VAG request: %+v ", err)
		return errors.New("device API error")
	}
	return err
}

// DeleteRange tbd
func (c *Client) DeleteRange(ctx context.Context, startID, endID int64) {
	idx := startID
	for idx < endID {
		c.DeleteVolume(ctx, idx)
	}
	return
}

// DeleteVolume tbd
func (c *Client) DeleteVolume(ctx context.Context, volumeID int64) (err error) {

	// TODO(jdg): Add options like purge=True|False, range, ALL etc
	var req DeleteVolumeRequest
	req.VolumeID = volumeID
	_, err = c.Request(ctx, "DeleteVolume", req, NewReqID())
	if err != nil {
		// TODO: distinguish what the error was?
		Logc(ctx).Errorf("Error response from DeleteVolume request: %+v ", err)
		return errors.New("device API error")
	}
	_, err = c.Request(ctx, "PurgeDeletedVolume", req, NewReqID())
	return
}

// DetachVolume tbd
func (c *Client) DetachVolume(ctx context.Context, v Volume) (err error) {

	if c.SVIP == "" {
		Logc(ctx).Errorf("error response from DetachVolume request: %+v ", err)
		return errors.New("detach volume error")
	}

	err = utils.ISCSIDisableDelete(ctx, v.Iqn, c.SVIP)
	return
}

func (c *Client) ModifyVolume(ctx context.Context, req *ModifyVolumeRequest) (err error) {

	_, err = c.Request(ctx, "ModifyVolume", req, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error response from ModifyVolume request: %+v ", err)
		return errors.New("device API error")
	}
	return err
}
