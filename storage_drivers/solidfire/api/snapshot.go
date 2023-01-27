// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
)

func (c *Client) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (snapshot Snapshot, err error) {
	response, err := c.Request(ctx, "CreateSnapshot", req, NewReqID())
	if err != nil {
		if strings.Contains(err.Error(), "xMaxSnapshotsPerVolumeExceeded") {
			return Snapshot{}, utils.MaxLimitReachedError(err.Error())
		} else {
			return Snapshot{}, err
		}
	}
	var result CreateSnapshotResult
	if err = json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling CreateSnapshot json response: %+v", err)
		return Snapshot{}, errors.New("json decode error")
	}
	return c.GetSnapshot(ctx, result.Result.SnapshotID, req.VolumeID, "")
}

func (c *Client) GetSnapshot(ctx context.Context, snapID, volID int64, sfName string) (s Snapshot, err error) {
	var listReq ListSnapshotsRequest
	listReq.VolumeID = volID
	snapshots, err := c.ListSnapshots(ctx, &listReq)
	if err != nil {
		Logc(ctx).Errorf("Error in GetSnapshot from ListSnapshots: %+v", err)
		return Snapshot{}, errors.New("failed to perform ListSnapshots")
	}
	for _, snap := range snapshots {
		if snapID == snap.SnapshotID {
			s = snap
			break
		} else if sfName != "" && sfName == snap.Name {
			s = snap
			break
		}
	}
	return s, err
}

func (c *Client) ListSnapshots(ctx context.Context, req *ListSnapshotsRequest) (snapshots []Snapshot, err error) {
	response, err := c.Request(ctx, "ListSnapshots", req, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error in ListSnapshots: %+v", err)
		return nil, errors.New("failed to retrieve snapshots")
	}
	var result ListSnapshotsResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling ListSnapshots json response: %+v", err)
		return nil, errors.New("json decode error")
	}
	snapshots = result.Result.Snapshots
	return
}

func (c *Client) RollbackToSnapshot(ctx context.Context, req *RollbackToSnapshotRequest) (newSnapID int64, err error) {
	response, err := c.Request(ctx, "RollbackToSnapshot", req, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error in RollbackToSnapshot: %+v", err)
		return 0, errors.New("failed to rollback snapshot")
	}
	var result RollbackToSnapshotResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling RollbackToSnapshot json response: %+v", err)
		return 0, errors.New("json decode error")
	}
	newSnapID = result.Result.SnapshotID
	err = nil
	return
}

func (c *Client) DeleteSnapshot(ctx context.Context, snapshotID int64) (err error) {
	// TODO(jdg): Add options like purge=True|False, range, ALL etc
	var req DeleteSnapshotRequest
	req.SnapshotID = snapshotID
	_, err = c.Request(ctx, "DeleteSnapshot", req, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error in DeleteSnapshot: %+v", err)
		return errors.New("failed to delete snapshot")
	}
	return
}
