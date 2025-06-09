// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// volumeGroupSnapshotBackoff allocates some time to simulate backoffs because the CSI snapshotter
// has no exponential backoff for retries.
var volumeGroupSnapshotBackoff = 10 * time.Second

func (p *Plugin) GroupControllerGetCapabilities(
	ctx context.Context, _ *csi.GroupControllerGetCapabilitiesRequest,
) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowControllerGetCapabilities)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "GroupControllerGetCapabilities", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Trace(">>>> GroupControllerGetCapabilities")
	defer Logc(ctx).WithFields(fields).Trace("<<<< GroupControllerGetCapabilities")

	return &csi.GroupControllerGetCapabilitiesResponse{Capabilities: p.gcsCap}, nil
}

func (p *Plugin) CreateVolumeGroupSnapshot(
	ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest,
) (*csi.CreateVolumeGroupSnapshotResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowGroupSnapshotCreate)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{
		"Method":  "CreateVolumeGroupSnapshot",
		"Type":    p.name,
		"name":    req.GetName(),
		"volumes": req.GetSourceVolumeIds(),
	}
	Logc(ctx).WithFields(fields).Debug(">>>> CreateVolumeGroupSnapshot")
	defer Logc(ctx).WithFields(fields).Debug("<<<< CreateVolumeGroupSnapshot")

	// Validate the request.
	groupName := req.GetName()
	volumeNames := req.GetSourceVolumeIds()
	if groupName == "" {
		Logc(ctx).WithFields(fields).Error("No group name supplied.")
		return nil, status.Error(codes.InvalidArgument, "group snapshot ID is required")
	} else if len(volumeNames) == 0 {
		Logc(ctx).WithFields(fields).Error("No volume names / IDs supplied.")
		return nil, status.Error(codes.InvalidArgument, "at least one source volume ID is required")
	}

	// Get the config and validate it.
	config, err := p.controllerHelper.GetGroupSnapshotConfigForCreate(ctx, groupName, volumeNames)
	if err != nil {
		return nil, p.getCSIErrorForOrchestratorError(err)
	} else if err := config.Validate(); err != nil {
		Logc(ctx).WithFields(fields).Error("Group snapshot config was invalid.")
		// CSI snapshotter has no exponential backoff for retries, so slow it down here.
		time.Sleep(volumeGroupSnapshotBackoff)
		return nil, status.Error(codes.InvalidArgument, "group snapshot was invalid")
	}

	// Check if the group snapshot exists.
	groupSnapshot, err := p.orchestrator.GetGroupSnapshot(ctx, groupName)
	if err != nil && !errors.IsNotFoundError(err) {
		Logc(ctx).WithFields(fields).Error("Could not check if group snapshot exists.")
		// CSI snapshotter has no exponential backoff for retries, so slow it down here.
		time.Sleep(volumeGroupSnapshotBackoff)
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// The group snapshot doesn't exist. Create it.
	if groupSnapshot == nil {
		groupSnapshot, err = p.orchestrator.CreateGroupSnapshot(ctx, config)
		if err != nil {
			// This operation must be idempotent. Attempt to clean up any artifacts from the failed create.
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to create group snapshot; cleaning up failed snapshots.")
			if deleteErr := p.orchestrator.DeleteGroupSnapshot(ctx, groupName); deleteErr != nil {
				Logc(ctx).WithFields(fields).WithError(deleteErr).Error("Failed to clean up failed snapshots.")
				err = errors.Join(err, deleteErr)
			}

			// CSI snapshotter has no exponential backoff for retries, so slow it down here.
			time.Sleep(volumeGroupSnapshotBackoff)
			return nil, p.getCSIErrorForOrchestratorError(err)
		}
		if groupSnapshot == nil {
			Logc(ctx).WithFields(fields).Error("Failed to create group snapshot.")
			// CSI snapshotter has no exponential backoff for retries, so slow it down here.
			time.Sleep(volumeGroupSnapshotBackoff)
			return nil, status.Error(codes.Internal, "group snapshot was unexpectedly nil after creation")
		}
	}

	// List the snapshots by the group snapshot.
	snapshots, err := p.orchestrator.ListSnapshotsForGroup(ctx, groupSnapshot.ID())
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not list snapshots for group snapshot.")

		// CSI snapshotter has no exponential backoff for retries, so slow it down here.
		time.Sleep(volumeGroupSnapshotBackoff)
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Ensure the group snapshot and the grouped snapshots satisfies the request.
	if err := p.validateGroupSnapshot(ctx, groupSnapshot, snapshots, volumeNames); err != nil {
		Logc(ctx).WithError(err).WithFields(fields).Error("Group snapshot exists but is incompatible.")
		// CSI snapshotter has no exponential backoff for retries, so slow it down here.
		time.Sleep(volumeGroupSnapshotBackoff)
		return nil, err
	}

	// Create a CSI group snapshot from the Trident group snapshot.
	var csiGroupSnapshot *csi.VolumeGroupSnapshot
	csiGroupSnapshot, err = p.getCSIGroupSnapshotFromTridentGroupSnapshot(ctx, groupSnapshot, snapshots)
	if err != nil {
		// CSI snapshotter has no exponential backoff for retries, so slow it down here.
		time.Sleep(volumeGroupSnapshotBackoff)
		return nil, status.Error(codes.Internal, "could not convert group snapshot to CSI group snapshot")
	}

	return &csi.CreateVolumeGroupSnapshotResponse{GroupSnapshot: csiGroupSnapshot}, nil
}

func (p *Plugin) GetVolumeGroupSnapshot(
	ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest,
) (*csi.GetVolumeGroupSnapshotResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowGroupSnapshotGet)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{
		"Method":    "GetVolumeGroupSnapshot",
		"Type":      p.name,
		"name":      req.GetGroupSnapshotId(),
		"snapshots": req.GetSnapshotIds(),
	}
	Logc(ctx).WithFields(fields).Debug(">>>> GetVolumeGroupSnapshot")
	defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumeGroupSnapshot")

	groupName := req.GetGroupSnapshotId()
	snapshotIDs := req.GetSnapshotIds()
	if groupName == "" {
		return nil, status.Error(codes.InvalidArgument, "group snapshot ID is required")
	}

	// Pre-provisioned workflows may utilize this workflow in the future.
	// When support is added, this method will need to be updated.

	// Check if the group snapshot even exists. If not, return early.
	groupSnapshot, err := p.orchestrator.GetGroupSnapshot(ctx, req.GroupSnapshotId)
	if err != nil && !errors.IsNotFoundError(err) {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not check if group snapshot exists.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}
	if groupSnapshot == nil {
		// The group snapshot does not exist. Return NotFound.
		Logc(ctx).WithFields(fields).Debug("Group snapshot not found.")
		return nil, status.Error(codes.NotFound, "group snapshot not found")
	}

	// The group snapshot does exist. List the constituent snapshots then create a CSI group snapshot.
	snapshots, err := p.orchestrator.ListSnapshotsForGroup(ctx, groupName)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not list snapshots for group snapshot.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Check if the get request included a set of snapshot IDs.
	if len(snapshotIDs) > 0 && len(snapshots) != len(snapshotIDs) {
		Logc(ctx).WithFields(fields).Error("Group snapshot contains contains a different number of snapshots than the requested snapshots.")
		return nil, status.Error(codes.InvalidArgument, "group snapshot mismatch")
	}

	csiGroupSnapshot, err := p.getCSIGroupSnapshotFromTridentGroupSnapshot(ctx, groupSnapshot, snapshots)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not convert group snapshot to CSI group snapshot")
	}

	return &csi.GetVolumeGroupSnapshotResponse{GroupSnapshot: csiGroupSnapshot}, nil
}

func (p *Plugin) DeleteVolumeGroupSnapshot(
	ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest,
) (*csi.DeleteVolumeGroupSnapshotResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowGroupSnapshotDelete)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{
		"Method":    "DeleteVolumeGroupSnapshot",
		"Type":      p.name,
		"name":      req.GetGroupSnapshotId(),
		"snapshots": req.GetSnapshotIds(),
	}
	Logc(ctx).WithFields(fields).Debug(">>>> DeleteVolumeGroupSnapshot")
	defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteVolumeGroupSnapshot")

	groupName := req.GetGroupSnapshotId()
	snapshotIDs := req.GetSnapshotIds()
	if groupName == "" {
		return nil, status.Error(codes.InvalidArgument, "group snapshot ID is required")
	}

	// Check if the group snapshot exists.
	groupSnap, err := p.orchestrator.GetGroupSnapshot(ctx, groupName)
	if err != nil && !errors.IsNotFoundError(err) {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not check if group snapshot exists.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Always attempt to delete the entire group snapshot if it exists.
	if groupSnap != nil {
		if err := p.orchestrator.DeleteGroupSnapshot(ctx, groupName); err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(fields).Error("Could not delete group snapshot.")
				return nil, p.getCSIErrorForOrchestratorError(err)
			}
			Logc(ctx).WithFields(fields).Debug("Group snapshot not found; ensuring all constituent snapshots are gone.")
		}
	}

	// The call above should attempt to delete all constituent snapshots, but if for
	// some reason the group is already gone, we should still try to delete the individual snapshots.
	if err := p.deleteGroupedSnapshots(ctx, snapshotIDs); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not delete snapshots in group.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

// getCSIGroupSnapshotFromTridentGroupSnapshot converts a Trident group snapshot into a CSI group snapshot.
// It accepts a context, a group snapshot, and a list of snapshots.
// In the future, it may not require the snapshots, but for now, it does to ensure that all snapshots are ready.
func (p *Plugin) getCSIGroupSnapshotFromTridentGroupSnapshot(
	ctx context.Context, groupSnapshot *storage.GroupSnapshotExternal, snapshots []*storage.SnapshotExternal,
) (*csi.VolumeGroupSnapshot, error) {
	if groupSnapshot == nil {
		return nil, status.Error(codes.Internal, "group snapshot was nil")
	}

	fields := LogFields{
		"groupName":   groupSnapshot.ID(),
		"snapshotIDs": groupSnapshot.GetSnapshotIDs(),
	}
	Logc(ctx).WithFields(fields).Debug("Constructing CSI group snapshot from Trident group snapshot.")

	// Check the preconditions for converting a group snapshot.
	if groupSnapshot == nil {
		Logc(ctx).Error("Group snapshot was nil.")
		return nil, status.Error(codes.Internal, "group snapshot was nil")
	} else if err := groupSnapshot.Validate(); err != nil {
		Logc(ctx).WithField("groupSnapshot", groupSnapshot.ID()).Error("Group snapshot was invalid.")
		return nil, status.Error(codes.Internal, "group snapshot was invalid")
	}

	// Get the creation time of the group. This should be the same for all constituent snapshots.
	groupCreatedSeconds, err := time.Parse(time.RFC3339, groupSnapshot.GetCreated())
	if err != nil {
		Logc(ctx).WithFields(fields).Error("Could not parse RFC3339 group snapshot time.")
		return nil, status.Error(codes.Internal, "could not parse group snapshot time")
	}

	var errs error
	csiSnapshots := make([]*csi.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotID := snapshot.ID()
		createdSeconds, err := time.Parse(time.RFC3339, snapshot.Created)
		if err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Could not parse RFC3339 snapshot time.")
			createdSeconds = groupCreatedSeconds
		}

		snapshotReady := snapshot.State == storage.SnapshotStateOnline
		if !snapshotReady {
			Logc(ctx).WithFields(fields).WithField(
				"snapshotState", snapshot.State,
			).Errorf("Snapshot '%s' in group is unexpectedly not ready.", snapshotID)
			errs = errors.Join(errs, fmt.Errorf("snapshot '%s' in group is not ready", snapshotID))
			continue
		}

		csiSnapshots = append(csiSnapshots, &csi.Snapshot{
			SnapshotId:     snapshotID,
			SourceVolumeId: snapshot.Config.VolumeName,
			// The following fields are not used by Kubernetes CSI.
			// It would simplify the CreateVolumeGroupSnapshot workflow to remove them,
			// but we may need to add the back in the future should the APIs change.
			// Leave them here until the Kubernetes VolumeGroupSnapshot APIs are GA.
			SizeBytes:    snapshot.SizeBytes,
			CreationTime: &timestamp.Timestamp{Seconds: createdSeconds.Unix()},
			ReadyToUse:   snapshotReady,
		})
	}

	if errs != nil {
		Logc(ctx).WithFields(fields).WithError(errs).Error("Some snapshots in group are not ready.")
		return nil, status.Error(codes.Internal, "not all snapshots in group are ready")
	}

	return &csi.VolumeGroupSnapshot{
		GroupSnapshotId: groupSnapshot.ID(),
		Snapshots:       csiSnapshots,
		CreationTime:    &timestamp.Timestamp{Seconds: groupCreatedSeconds.Unix()},
		ReadyToUse:      true,
	}, nil
}

// validateGroupSnapshot accepts a group snapshot ID/Name, a list of volume names, and a group snapshot object
// and checks if the group snapshot is compatible with the provided volume names. This means that the group snapshot
// contains snapshots for all the provided volume names and that no volume name is duplicated in the group snapshot.
func (p *Plugin) validateGroupSnapshot(
	ctx context.Context, groupSnapshot *storage.GroupSnapshotExternal,
	snapshots []*storage.SnapshotExternal, volumeNames []string,
) error {
	if groupSnapshot == nil {
		Logc(ctx).Error("Group snapshot was nil.")
		return status.Error(codes.Internal, "group snapshot was nil")
	} else if len(snapshots) == 0 {
		Logc(ctx).Error("No constituent snapshots were provided.")
		return status.Error(codes.Internal, "no constituent snapshots were provided")
	} else if len(volumeNames) == 0 {
		Logc(ctx).Error("No volume names were provided.")
		return status.Error(codes.Internal, "no volume names were provided")
	}

	groupName := groupSnapshot.ID()
	snapshotIDs := groupSnapshot.GetSnapshotIDs()
	fields := LogFields{
		"groupName":   groupName,
		"snapshotIDs": snapshotIDs,
		"volumeNames": volumeNames,
	}
	Logc(ctx).WithFields(fields).Debug("Validating group snapshot.")

	// Ensure there are no duplicate snapshots in the group snapshot.
	volumesToSnapshots := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotID := snapshot.ID()
		volumeName, snapName, err := storage.ParseSnapshotID(snapshotID)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Could not parse snapshot ID '%s'.", snapshotID)
			return status.Error(codes.Internal, "could not parse constituent snapshot ID")
		}

		// A volume can only be added to a group once. If duplicates exist, this group snapshot is invalid.
		if _, ok := volumesToSnapshots[volumeName]; ok {
			err := fmt.Errorf("multiple snapshots exist for volume '%s' within group '%s'", volumeName, groupName)
			Logc(ctx).WithError(err).Error("Group snapshot is incompatible with provided volume names.")
			return status.Error(codes.AlreadyExists, err.Error())
		}
		volumesToSnapshots[volumeName] = snapName
	}

	// Ensure there are no missing snapshots.
	if len(volumesToSnapshots) != len(volumeNames) {
		return status.Error(codes.AlreadyExists, "group snapshot exists but has mismatched volume names")
	}
	for _, volumeName := range volumeNames {
		if _, ok := volumesToSnapshots[volumeName]; !ok {
			err := fmt.Errorf("group snapshot exists but does not contain snapshot for volume '%s'", volumeName)
			Logc(ctx).WithError(err).Error("Group snapshot is incompatible with provided volume names.")
			return status.Error(codes.Internal, err.Error())
		}
	}

	// Ensure each constituent snapshot is valid.
	for _, snapshot := range snapshots {
		if err := snapshot.Validate(); err != nil {
			err = fmt.Errorf("group snapshot '%s' contains invalid constituent snapshot '%s'; %w", groupName, snapshot.ID(), err)
			Logc(ctx).WithError(err).Error("Group snapshot is incompatible with provided volume names.")
			return status.Error(codes.Internal, err.Error())
		}
		fields := LogFields{
			"snapshotID":     snapshot.ID(),
			"volumeName":     snapshot.Config.VolumeName,
			"state":          snapshot.State,
			"snapCreatedAt":  snapshot.Created,
			"groupCreatedAt": groupSnapshot.GetCreated(),
		}

		if snapshot.State != storage.SnapshotStateOnline {
			err := fmt.Errorf("snapshot '%s' in group '%s' is not ready", snapshot.ID(), groupName)
			Logc(ctx).WithFields(fields).WithError(err).Error("Constituent snapshot within group is not ready.")
			return status.Error(codes.Internal, err.Error())
		}
	}

	// If we made it this far, the group snapshot is compatible with the provided volume names.
	return nil
}

// deleteGroupedSnapshots is a final attempt to delete the constituent snapshots of a group snapshot should
// any still exist after the orchestrator core DeleteGroupSnapshot call.
// In most cases, this should be a NOP.
func (p *Plugin) deleteGroupedSnapshots(ctx context.Context, snapshotIDs []string) error {
	var errs error
	for _, snapshotID := range snapshotIDs {
		volumeName, snapName, err := storage.ParseSnapshotID(snapshotID)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Could not parse snapshot ID '%s'.", snapshotID)
			continue
		}

		// This shouldn't fail, even if the snapshot doesn't exist.
		// If it does fail, capture the error for logging.
		if err := p.orchestrator.DeleteSnapshot(ctx, volumeName, snapName); err != nil && !errors.IsNotFoundError(err) {
			err = fmt.Errorf("snapshot '%s' in group still exists", snapshotID)
			errs = errors.Join(errs, err) // Collect all errors for logging.
		}
	}

	return errs
}
