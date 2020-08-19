// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/netapp/trident/logger"
)

// CreateVolumeAccessGroup tbd
func (c *Client) CreateVolumeAccessGroup(ctx context.Context, r *CreateVolumeAccessGroupRequest) (vagID int64,
	err error) {

	var result CreateVolumeAccessGroupResult
	response, err := c.Request(ctx, "CreateVolumeAccessGroup", r, NewReqID())
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling CreateVolumeAccessGroupResult API response: %+v", err)
		return 0, errors.New("json-decode error")
	}
	vagID = result.Result.VagID
	return

}

// ListVolumeAccessGroups tbd
func (c *Client) ListVolumeAccessGroups(
	ctx context.Context, r *ListVolumeAccessGroupsRequest,
) (vags []VolumeAccessGroup, err error) {

	response, err := c.Request(ctx, "ListVolumeAccessGroups", r, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error in ListVolumeAccessGroupResult API response: %+v", err)
		return nil, errors.New("failed to retrieve VAG list")
	}
	var result ListVolumesAccessGroupsResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling ListVolumeAccessGroupResult API response: %+v", err)
		return nil, errors.New("json-decode error")
	}
	vags = result.Result.Vags
	return
}

// AddInitiatorsToVolumeAccessGroup tbd
func (c *Client) AddInitiatorsToVolumeAccessGroup(
	ctx context.Context, r *AddInitiatorsToVolumeAccessGroupRequest,
) error {

	_, err := c.Request(ctx, "AddInitiatorsToVolumeAccessGroup", r, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error in AddInitiator to VAG API response: %+v", err)
		return errors.New("failed to add initiator to VAG")
	}
	return nil
}
