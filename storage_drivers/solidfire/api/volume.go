// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

// ListVolumesForAccount tbd
func (c *Client) ListVolumesForAccount(listReq *ListVolumesForAccountRequest) (volumes []Volume, err error) {
	response, err := c.Request("ListVolumesForAccount", listReq, NewReqID())
	if err != nil {
		log.Errorf("Error detected in ListVolumesForAccount API response: %+v", err)
		return nil, errors.New("device API error")
	}
	var result ListVolumesResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling ListVolumesForAccount API response: %+v", err)
		return nil, errors.New("json-decode error")
	}
	volumes = result.Result.Volumes
	return volumes, err
}

// GetVolumeByID returns the volume with the specified ID.
func (c *Client) GetVolumeByID(volID int64) (Volume, error) {

	var limit int64 = 1

	listRequest := &ListVolumesRequest{
		Accounts:      []int64{c.AccountID},
		StartVolumeID: &volID,
		Limit:         &limit,
	}

	volumes, err := c.ListVolumes(listRequest)
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
func (c *Client) WaitForVolumeByID(volID int64) (Volume, error) {

	volume := Volume{}

	checkVolumeExists := func() error {
		var err error
		volume, err = c.GetVolumeByID(volID)
		if err != nil {
			return fmt.Errorf("volume %d does not yet exist; %v", volID, err)
		}
		return nil
	}
	volumeExistsNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Volume not yet present, waiting.")
	}
	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 2 * time.Second
	volumeBackoff.Multiplier = 1.414
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = 30 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeExists, volumeBackoff, volumeExistsNotify); err != nil {
		log.WithField("volumeID", volID).Warnf(
			"Could not find volume after %3.2f seconds.", volumeBackoff.MaxElapsedTime)
		return volume, fmt.Errorf("volume %d does not exist", volID)
	} else {
		log.WithField("volumeID", volID).Debug("Volume found.")
		return volume, nil
	}
}

// ListVolumes returns all volumes using the specified request object.
func (c *Client) ListVolumes(listVolReq *ListVolumesRequest) (volumes []Volume, err error) {
	response, err := c.Request("ListVolumes", listVolReq, NewReqID())
	if err != nil {
		log.Errorf("Error response from ListVolumes request: %v ", err)
		return nil, errors.New("device API error")
	}
	var result ListVolumesResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling ListVolumes API response: %v", err)
		return nil, errors.New("json-decode error")
	}
	volumes = result.Result.Volumes
	return volumes, err
}

// CloneVolume invokes the supplied clone volume request.  It waits for the source volume
// (which itself may be new in a test scenario) to be ready to be cloned, and it waits for
// the clone to exist.
func (c *Client) CloneVolume(req *CloneVolumeRequest) (Volume, error) {
	var cloneError error
	var response []byte
	var result CloneVolumeResult

	// We use this loop to deal with things like trying to immediately clone
	// from a volume that was just created.  Sometimes it can take a few
	// seconds for the Slice to finalize even though the Volume reports ready.
	// We'll do a backoff retry loop here, at some point would be handy go have
	// a global util for us to use for any call
	retry := 0
	for retry < 10 {
		response, cloneError = c.Request("CloneVolume", req, NewReqID())
		if cloneError != nil {
			errorMessage := cloneError.Error()
			if strings.Contains(errorMessage, "SliceNotRegistered") {
				log.Warningf("detected SliceNotRegistered on Clone operation, retrying in %d seconds", 2+retry)
				time.Sleep(time.Second * time.Duration(2+retry))
				retry++
			} else if strings.Contains(errorMessage, "xInvalidParameter") {
				log.Warningf("detected xInvalidParameter on Clone operation, retrying in %d seconds", 2+retry)
				time.Sleep(time.Second * time.Duration(2+retry))
				retry++
			} else {
				break
			}
		} else {
			break
		}
	}

	if cloneError != nil {
		log.Errorf("Failed to clone volume: %+v", cloneError)
		return Volume{}, cloneError
	}
	log.Info("Clone request succeeded")

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling CloneVolume API response: %v", err)
		return Volume{}, errors.New("json-decode error")
	}

	return c.WaitForVolumeByID(result.Result.VolumeID)
}

// CreateVolume tbd
func (c *Client) CreateVolume(createReq *CreateVolumeRequest) (Volume, error) {
	response, err := c.Request("CreateVolume", createReq, NewReqID())
	if err != nil {
		log.Errorf("Error response from CreateVolume request: %v ", err)
		return Volume{}, errors.New("device API error")
	}
	var result CreateVolumeResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling CreateVolume API response: %v", err)
		return Volume{}, errors.New("json-decode error")
	}

	return c.WaitForVolumeByID(result.Result.VolumeID)
}

// AddVolumesToAccessGroup tbd
func (c *Client) AddVolumesToAccessGroup(req *AddVolumesToVolumeAccessGroupRequest) (err error) {
	_, err = c.Request("AddVolumesToVolumeAccessGroup", req, NewReqID())
	if err != nil {
		if apiErr, ok := err.(Error); ok && apiErr.Fields.Name == "xAlreadyInVolumeAccessGroup" {
			return nil
		}
		log.Errorf("error response from Add to VAG request: %+v ", err)
		return errors.New("device API error")
	}
	return err
}

// DeleteRange tbd
func (c *Client) DeleteRange(startID, endID int64) {
	idx := startID
	for idx < endID {
		c.DeleteVolume(idx)
	}
	return
}

// DeleteVolume tbd
func (c *Client) DeleteVolume(volumeID int64) (err error) {
	// TODO(jdg): Add options like purge=True|False, range, ALL etc
	var req DeleteVolumeRequest
	req.VolumeID = volumeID
	_, err = c.Request("DeleteVolume", req, NewReqID())
	if err != nil {
		// TODO: distinguish what the error was?
		log.Errorf("Error response from DeleteVolume request: %+v ", err)
		return errors.New("device API error")
	}
	_, err = c.Request("PurgeDeletedVolume", req, NewReqID())
	return
}

// DetachVolume tbd
func (c *Client) DetachVolume(v Volume) (err error) {
	if c.SVIP == "" {
		log.Errorf("error response from DetachVolume request: %+v ", err)
		return errors.New("detach volume error")
	}

	err = utils.ISCSIDisableDelete(v.Iqn, c.SVIP)
	return
}

func (c *Client) ModifyVolume(req *ModifyVolumeRequest) (err error) {
	_, err = c.Request("ModifyVolume", req, NewReqID())
	if err != nil {
		log.Errorf("Error response from ModifyVolume request: %+v ", err)
		return errors.New("device API error")
	}
	return err
}
