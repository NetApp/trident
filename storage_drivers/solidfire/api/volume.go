// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

// GetVolumeByID tbd
func (c *Client) GetVolumeByID(volID int64) (v Volume, err error) {
	var req ListActiveVolumesRequest
	req.StartVolumeID = volID
	req.Limit = 1
	volumes, err := c.ListActiveVolumes(&req)
	if err != nil {
		return v, err
	}
	if len(volumes) < 1 {
		return Volume{}, fmt.Errorf("failed to find volume with ID: %d", volID)
	}
	return volumes[0], nil
}

// ListActiveVolumes tbd
func (c *Client) ListActiveVolumes(listVolReq *ListActiveVolumesRequest) (volumes []Volume, err error) {
	response, err := c.Request("ListActiveVolumes", listVolReq, NewReqID())
	if err != nil {
		log.Errorf("Error response from ListActiveVolumes request: %+v ", err)
		return nil, errors.New("device API error")
	}
	var result ListVolumesResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling ListActiveVolumes API response: %+v", err)
		return nil, errors.New("json-decode error")
	}
	volumes = result.Result.Volumes
	return volumes, err
}

func (c *Client) CloneVolume(req *CloneVolumeRequest) (vol Volume, err error) {
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
				log.Warningf("detected SliceNotRegistered on Clone operation, retrying in %+v seconds", 2+retry)
				time.Sleep(time.Second * time.Duration(2+retry))
				retry++
			} else if strings.Contains(errorMessage, "xInvalidParameter") {
				log.Warningf("detected xInvalidParameter on Clone operation, retrying in %+v seconds", 2+retry)
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
	log.Info("clone request was successful")

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling CloneVolume API response: %+v", err)
		return Volume{}, errors.New("json-decode error")
	}

	retry = 0
	for retry < 5 {
		vol, err = c.GetVolumeByID(result.Result.VolumeID)
		if err == nil {
			break
		}
		log.Warningf("Failed to get volume by ID, retrying in %+v seconds", 2+retry)
		time.Sleep(time.Second * time.Duration(2+retry))
		retry++
	}
	return vol, err
}

// CreateVolume tbd
func (c *Client) CreateVolume(createReq *CreateVolumeRequest) (vol Volume, err error) {
	response, err := c.Request("CreateVolume", createReq, NewReqID())
	if err != nil {
		log.Errorf("Error response from CreateVolume request: %+v ", err)
		return Volume{}, errors.New("device API error")
	}
	var result CreateVolumeResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling CreateVolume API response: %+v", err)
		return Volume{}, errors.New("json-decode error")
	}

	vol, err = c.GetVolumeByID(result.Result.VolumeID)
	return vol, err
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
	tgt := &utils.ISCSITargetInfo{
		IP:     c.SVIP,
		Portal: c.SVIP,
		Iqn:    v.Iqn,
	}
	err = utils.ISCSIDisableDelete(tgt)
	return
}

// AttachVolume tbd
func (c *Client) AttachVolume(v *Volume, iface string) (err error) {
	var req GetAccountByIDRequest

	if c.SVIP == "" {
		err = errors.New("unable to perform iSCSI actions without setting SVIP")
		log.Errorf("Unable to attach volume: SVIP is NOT set")
		return err
	}

	if utils.ISCSISupported() == false {
		err := errors.New("unable to attach: open-iscsi tools not found on host")
		log.Errorf("Unable to attach volume: open-iscsi utils not found")
		return err
	}

	req.AccountID = v.AccountID
	a, err := c.GetAccountByID(&req)
	if err != nil {
		log.Errorf("Failed to get account %v: %+v ", v.AccountID, err)
		return errors.New("volume attach failure")
	}

	err = utils.LoginWithChap(v.Iqn, c.SVIP, a.Username, a.InitiatorSecret, iface,
		c.Config.DebugTraceFlags["sensitive"])
	if err != nil {
		log.Errorf("Failed to login with CHAP credentials: %+v ", err)
		return err
	}

	return nil
}

func (c *Client) ModifyVolume(req *ModifyVolumeRequest) (err error) {
	_, err = c.Request("ModifyVolume", req, NewReqID())
	if err != nil {
		log.Errorf("Error response from ModifyVolume request: %+v ", err)
		return errors.New("device API error")
	}
	return err
}
