// Copyright 2016 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"errors"

	log "github.com/Sirupsen/logrus"
)

// CreateVolumeAccessGroup tbd
func (c *Client) CreateVolumeAccessGroup(r *CreateVolumeAccessGroupRequest) (vagID int64, err error) {
	var result CreateVolumeAccessGroupResult
	response, err := c.Request("CreateVolumeAccessGroup", r, NewReqID())
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("error detected unmarshalling CreateVolumeAccessGroupResult API response: %+v", err)
		return 0, errors.New("json-decode error")
	}
	vagID = result.Result.VagID
	return

}

// ListVolumeAccessGroups tbd
func (c *Client) ListVolumeAccessGroups(r *ListVolumeAccessGroupsRequest) (vags []VolumeAccessGroup, err error) {
	response, err := c.Request("ListVolumeAccessGroups", r, NewReqID())
	if err != nil {
		log.Errorf("error in ListVolumeAccessGroupResult API response: %+v", err)
		return nil, errors.New("failed to retrieve VAG list")
	}
	var result ListVolumesAccessGroupsResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("error detected unmarshalling ListVolumeAccessGroupResult API response: %+v", err)
		return nil, errors.New("json-decode error")
	}
	vags = result.Result.Vags
	return
}

// AddInitiatorsToVolumeAccessGroup tbd
func (c *Client) AddInitiatorsToVolumeAccessGroup(r *AddInitiatorsToVolumeAccessGroupRequest) error {
	_, err := c.Request("AddInitiatorsToVolumeAccessGroup", r, NewReqID())
	if err != nil {
		log.Errorf("error in AddInitiator to VAG API response: %+v", err)
		return errors.New("failed to add initiator to VAG")
	}
	return nil
}
