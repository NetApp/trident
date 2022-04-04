// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logger"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	storagedrivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
)

// establishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship
func establishMirror(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string, d api.OntapAPI,
) error {
	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	// Ensure the destination is a DP volume
	volume, err := d.VolumeInfo(ctx, localFlexvolName)
	if err != nil {
		return err
	}

	if !volume.DPVolume {
		return fmt.Errorf("mirrors can only be established with empty DP volumes as the destination")
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)

	if err != nil {
		if api.IsNotFoundError(err) {

			// create and initialize snapmirror if not found
			if err := d.SnapmirrorCreate(ctx,
				localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
				replicationPolicy, replicationSchedule,
			); err != nil {
				return err
			}

			snapmirror, err = d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err != nil {
				return err
			}

		} else {
			return err
		}
	}

	if snapmirror.State.IsUninitialized() && snapmirror.RelationshipStatus.IsIdle() {
		err = d.SnapmirrorInitialize(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror initialize")
			return err
		}
		// Ensure state is inititialized
		snapmirror, err = d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if snapmirror.State.IsUninitialized() {
			return api.NotReadyError("Snapmirror not yet initialized, snapmirror not ready")
		}
	}

	return nil
}

// reestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously
func reestablishMirror(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
	d api.OntapAPI,
) error {
	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	// Check if a snapmirror relationship already exists
	snapmirror, err := d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if api.IsNotFoundError(err) {
			// create and initialize snapmirror if not found
			if err := d.SnapmirrorCreate(ctx,
				localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
				replicationPolicy, replicationSchedule,
			); err != nil {
				return err
			}
			_, err = d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// If the snapmirror is already established we have nothing to do
		if !snapmirror.State.IsUninitialized() || snapmirror.LastTransferType != "" &&
			snapmirror.RelationshipStatus.IsIdle() {
			return nil
		}
	}

	// Resync the relationship
	err = d.SnapmirrorResync(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		return err
	}

	// Verify the state of the relationship
	snapmirror, err = d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if api.IsNotFoundError(err) {
			return utils.ReconcileIncompleteError()
		} else {
			return err
		}
	}

	// Check if the snapmirror is healthy
	if !snapmirror.IsHealthy {
		err = fmt.Errorf(snapmirror.UnhealthyReason)
		Logc(ctx).WithError(err).Error("Error on snapmirror resync")
		d.SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)

		return err
	}
	return nil
}

// promoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced
func promoteMirror(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, snapshotHandle,
	replicationPolicy string, d api.OntapAPI,
) (bool, error) {
	if remoteVolumeHandle == "" {
		return false, nil
	}

	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return false, fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return false, fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)

	if err == nil || api.IsNotFoundError(err) {

		if replicationPolicy != "" {
			snapmirrorPolicy, err := d.SnapmirrorPolicyGet(ctx, replicationPolicy)
			if err != nil {
				return false, err
			}
			// If the policy is a synchronous type we shouldn't wait for a snapshot
			if snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeSync() {
				snapshotHandle = ""
			}
		}

		// Check for snapshot
		if snapshotHandle != "" {
			foundSnapshot, err := isSnapshotPresent(ctx, snapshotHandle, localFlexvolName, d)
			if err != nil {
				return false, err
			}
			if !foundSnapshot {
				return true, nil
			}
		}
	} else {
		return false, err
	}

	if err == nil {
		err = d.SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			if api.IsNotReadyError(err) {
				Logc(ctx).WithError(err).Error("Snapmirror quiesce is not finished")
			}
			return false, err
		}

		errAbort := d.SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if api.IsNotReadyError(errAbort) {
			// Check if we're still aborting - ZAPI returns a generic 13001 error code when an abort is already
			// in progress
			Logc(ctx).WithError(errAbort).Error("Snapmirror abort is not finished")
			return false, errAbort
		}

		snapmirror, err = d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			return false, err
		}

		// Break if snapmirror is initialized, otherwise it will fail saying the volume is not initialized
		if !snapmirror.State.IsUninitialized() {
			snapshotName := ""
			if snapshotHandle != "" {
				_, snapshotName, err = storage.ParseSnapshotID(snapshotHandle)
				if err != nil {
					return false, err
				}
				Logc(ctx).Debugf("Restoring volume %s to snapshot %s based on specified latest snapshot handle",
					remoteFlexvolName, snapshotName)
			}
			err := d.SnapmirrorBreak(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
				snapshotName)
			if err != nil {
				if api.IsNotReadyError(err) {
					Logc(ctx).WithError(err).Error("Snapmirror break is not finished")
				}
				return false, err
			}
		}

		err = d.SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			return false, err
		}
	}

	return false, nil
}

// isSnapshotPresent returns whether the given snapshot is found on the snapmirror snapshot list
func isSnapshotPresent(ctx context.Context, snapshotHandle, localFlexvolName string, d api.OntapAPI) (bool, error) {
	found := false

	_, snapshotName, err := storage.ParseSnapshotID(snapshotHandle)
	if err != nil {
		return found, err
	}

	snapshots, err := d.VolumeSnapshotList(ctx, localFlexvolName)
	if err != nil {
		return found, err
	}

	for _, snapshot := range snapshots {
		if snapshot.Name == snapshotName {
			found = true
		}
	}
	if !found {
		Logc(ctx).WithField("snapshot", snapshotHandle).Debug("Snapshot not yet present.")
		return found, nil
	}
	return found, nil
}

// getMirrorStatus returns the current state of a snapmirror relationship
func getMirrorStatus(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, d api.OntapAPI,
) (string, error) {
	// Empty remote means there is no mirror to check for
	if remoteVolumeHandle == "" {
		return "", nil
	}

	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return "", fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return "", fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if !api.IsNotFoundError(err) {
			return v1.MirrorStatePromoted, nil
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return "", nil
		}
	}

	// Translate the snapmirror status to a mirror status
	switch snapmirror.RelationshipStatus {
	case api.SnapmirrorStatusBreaking:
		return v1.MirrorStatePromoting, nil
	case api.SnapmirrorStatusQuiescing:
		return v1.MirrorStatePromoting, nil
	case api.SnapmirrorStatusAborting:
		return v1.MirrorStatePromoting, nil
	default:
		switch snapmirror.State {
		case api.SnapmirrorStateBroken:
			if snapmirror.RelationshipStatus == api.SnapmirrorStatusTransferring {
				return v1.MirrorStateEstablishing, nil
			}
			return v1.MirrorStatePromoting, nil
		case api.SnapmirrorStateUninitialized:
			return v1.MirrorStateEstablishing, nil
		case api.SnapmirrorStateSnapmirrored:
			return v1.MirrorStateEstablished, nil
		}
	}

	Logc(ctx).WithError(err).Error("Unknown snapmirror status returned")
	return "", nil
}

func checkSVMPeeredAbstraction(
	ctx context.Context, volConfig *storage.VolumeConfig, svm string, d api.OntapAPI,
) error {
	remoteSVM, _, err := parseVolumeHandle(volConfig.PeerVolumeHandle)
	if err != nil {
		err = fmt.Errorf("could not determine required peer SVM; %v", err)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	peeredVservers, _ := d.GetSVMPeers(ctx)
	if !utils.SliceContainsString(peeredVservers, remoteSVM) {
		err = fmt.Errorf("backend SVM %v is not peered with required SVM %v", svm, remoteSVM)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	return nil
}

func validateReplicationPolicyAbstraction(ctx context.Context, policyName string, d api.OntapAPI) error {
	if policyName == "" {
		return nil
	}

	// Validate replication options
	snapmirrorPolicy, err := d.SnapmirrorPolicyGet(ctx, policyName)
	if err != nil {
		return fmt.Errorf("error getting snapmirror policy: %v", err)
	}

	if snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeSync() {
		// If the policy is synchronous we're fine
		return nil
	} else if !snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeAsync() {
		return fmt.Errorf("unsupported mirror policy type %v, must be %v or %v",
			snapmirrorPolicy.Type, api.SnapmirrorPolicyTypeSync, api.SnapmirrorPolicyTypeAsync)
	}

	// If the policy is async, check below for correct rule
	// Check async policies for the "all_source_snapshots" rule
	if snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeAsync() {
		for rule := range snapmirrorPolicy.Rules {
			if rule == api.SnapmirrorPolicyRuleAll {
				return nil
			}
		}

		return fmt.Errorf("snapmirror policy %v is of type %v and is missing the %v rule",
			policyName, api.SnapmirrorPolicyTypeAsync, api.SnapmirrorPolicyRuleAll)

	}
	return nil
}

func validateReplicationSchedule(ctx context.Context, replicationSchedule string, d api.OntapAPI) error {
	if replicationSchedule != "" {
		if err := d.JobScheduleExists(ctx, replicationSchedule); err != nil {
			return err
		}
	}

	return nil
}

func validateReplicationConfig(
	ctx context.Context, replicationPolicy, replicationSchedule string, d api.OntapAPI,
) error {
	if err := validateReplicationPolicyAbstraction(ctx, replicationPolicy, d); err != nil {
		return fmt.Errorf("failed to validate replication policy: %v", replicationPolicy)
	}

	// TODO: Check for replication policy (about rules) is of type async and replication schedule is empty,
	//  log a message

	if err := validateReplicationSchedule(ctx, replicationSchedule, d); err != nil {
		return fmt.Errorf("failed to validate replication schedule: %v", replicationSchedule)
	}

	return nil
}
