// Copyright 2023 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"time"

	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	storagedrivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

// establishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship
func establishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string, d api.OntapAPI,
) error {
	if localInternalVolumeName == "" {
		return fmt.Errorf("invalid volume name")
	}
	localSVMName := d.SVMName()
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	// Ensure the destination is a DP volume
	volume, err := d.VolumeInfo(ctx, localInternalVolumeName)
	if err != nil {
		return err
	}

	if !volume.DPVolume {
		return fmt.Errorf("mirrors can only be established with empty DP volumes as the destination")
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if api.IsNotFoundError(err) {
			// create and initialize snapmirror if not found
			if err := d.SnapmirrorCreate(ctx,
				localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName,
				replicationPolicy, replicationSchedule,
			); err != nil {
				return err
			}

			snapmirror, err = d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err != nil {
				return err
			}

		} else {
			return err
		}
	}

	// Check last transfer type as empty or idle
	if snapmirror.State.IsUninitialized() &&
		(snapmirror.RelationshipStatus.IsIdle() || snapmirror.LastTransferType == "") {
		err = d.SnapmirrorInitialize(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror initialize")
			return err
		}
		// Ensure state is inititialized
		snapmirror, err = d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
		if snapmirror.State.IsUninitialized() || snapmirror.RelationshipStatus.IsTransferring() {
			return api.NotReadyError("Snapmirror not yet initialized, snapmirror not ready")
		}
	}

	return nil
}

// reestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously
func reestablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
	d api.OntapAPI,
) error {
	if localInternalVolumeName == "" {
		return fmt.Errorf("invalid volume name")
	}
	localSVMName := d.SVMName()
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	// Check if a snapmirror relationship already exists
	snapmirror, snapmirrorGetErr := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName,
		remoteSVMName)
	if snapmirrorGetErr != nil {
		if api.IsNotFoundError(snapmirrorGetErr) {
			if replicationPolicy != "" {
				snapmirrorPolicy, err := d.SnapmirrorPolicyGet(ctx, replicationPolicy)
				if err != nil {
					return err
				}
				if snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeSync() {
					if err := d.SnapmirrorDeleteViaDestination(ctx, localInternalVolumeName, localSVMName); err != nil {
						return err
					}
				}
			}

			// create and initialize snapmirror if not found
			if err := d.SnapmirrorCreate(ctx,
				localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName,
				replicationPolicy, replicationSchedule,
			); err != nil {
				return err
			}
			_, err = d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err != nil {
				return err
			}
		} else {
			return snapmirrorGetErr
		}
	} else {
		// If the snapmirror is already established we have nothing to do
		if !snapmirror.State.IsUninitialized() || snapmirror.LastTransferType != "" &&
			snapmirror.RelationshipStatus.IsIdle() {
			return nil
		}
	}

	// Resync the relationship
	err = d.SnapmirrorResync(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		return err
	}

	// Verify the state of the relationship
	snapmirror, err = d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if api.IsNotFoundError(err) {
			return errors.ReconcileIncompleteError("reconcile incomplete")
		} else {
			return err
		}
	}

	// Check if the snapmirror is healthy
	if !snapmirror.IsHealthy {
		err = fmt.Errorf(snapmirror.UnhealthyReason)
		Logc(ctx).WithError(err).Error("Error on snapmirror resync")
		deleteErr := d.SnapmirrorDelete(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			Logc(ctx).WithError(deleteErr).Error("Error on snapmirror delete")
		}
		return err
	}
	return nil
}

// promoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced
func promoteMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotHandle, replicationPolicy string, d api.OntapAPI,
) (bool, error) {
	if remoteVolumeHandle == "" {
		return false, nil
	}
	if localInternalVolumeName == "" {
		return false, fmt.Errorf("invalid volume name")
	}
	localSVMName := d.SVMName()
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return false, fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)

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
			foundSnapshot, err := isSnapshotPresent(ctx, snapshotHandle, localInternalVolumeName, d)
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
		err = d.SnapmirrorQuiesce(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			if api.IsNotReadyError(err) {
				Logc(ctx).WithError(err).Error("Snapmirror quiesce is not finished")
			}
			return false, err
		}

		errAbort := d.SnapmirrorAbort(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
		if api.IsNotReadyError(errAbort) {
			// Check if we're still aborting - ZAPI returns a generic 13001 error code when an abort is already
			// in progress
			Logc(ctx).WithError(errAbort).Error("Snapmirror abort is not finished")
			return false, errAbort
		}

		snapmirror, err = d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
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
			err := d.SnapmirrorBreak(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName,
				snapshotName)
			if err != nil {
				if api.IsNotReadyError(err) {
					Logc(ctx).WithError(err).Error("Snapmirror break is not finished")
				}
				return false, err
			}
		}

		err = d.SnapmirrorDelete(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err != nil {
			return false, err
		}
	}

	return false, nil
}

// isSnapshotPresent returns whether the given snapshot is found on the snapmirror snapshot list
func isSnapshotPresent(ctx context.Context, snapshotHandle, localInternalVolumeName string, d api.OntapAPI) (bool, error) {
	_, snapshotName, err := storage.ParseSnapshotID(snapshotHandle)
	if err != nil {
		return false, err
	}

	snapshot, err := d.VolumeSnapshotInfo(ctx, snapshotName, localInternalVolumeName)
	if err != nil {
		return false, err
	}

	return snapshot.Name == snapshotName, nil
}

// getMirrorStatus returns the current state of a snapmirror relationship
func getMirrorStatus(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string, d api.OntapAPI,
) (string, error) {
	// Empty remote means there is no mirror to check for
	if remoteVolumeHandle == "" {
		return "", nil
	}

	if localInternalVolumeName == "" {
		return "", fmt.Errorf("invalid volume name")
	}

	localSVMName := d.SVMName()
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return "", fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if !api.IsNotFoundError(err) {
			return v1.MirrorStatePromoted, nil
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return "", nil
		}
	}

	// Translate the snapmirror status to a mirror status
	Logc(ctx).WithField("snapmirror.RelationshipStatus", snapmirror.RelationshipStatus).Debug("Checking snapmirror relationship status.")
	switch snapmirror.RelationshipStatus {
	case api.SnapmirrorStatusBreaking:
		return v1.MirrorStatePromoting, nil
	case api.SnapmirrorStatusQuiescing:
		return v1.MirrorStatePromoting, nil
	case api.SnapmirrorStatusAborting:
		return v1.MirrorStatePromoting, nil
	default:
		Logc(ctx).WithField("snapmirror.State", snapmirror.State).Debug("Checking snapmirror state.")
		switch snapmirror.State {
		case api.SnapmirrorStateBrokenOffZapi, api.SnapmirrorStateBrokenOffRest:
			if snapmirror.RelationshipStatus == api.SnapmirrorStatusTransferring {
				return v1.MirrorStateEstablishing, nil
			}
			return v1.MirrorStatePromoting, nil
		case api.SnapmirrorStateUninitialized, api.SnapmirrorStateSynchronizing:
			return v1.MirrorStateEstablishing, nil
		case api.SnapmirrorStateSnapmirrored, api.SnapmirrorStateInSync:
			return v1.MirrorStateEstablished, nil
		}
	}

	Logc(ctx).WithError(err).Error("Unknown snapmirror status returned")
	return "", nil
}

func checkSVMPeered(
	ctx context.Context, volConfig *storage.VolumeConfig, svm string, d api.OntapAPI,
) error {
	remoteSVM, _, err := parseVolumeHandle(volConfig.PeerVolumeHandle)
	if err != nil {
		err = fmt.Errorf("could not determine required peer SVM; %v", err)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	if remoteSVM == svm {
		Logc(ctx).Info("TMR is in single SVM peer mode. Both source and destination volumes are on a single SVM.")
		return nil
	}
	peeredVservers, _ := d.GetSVMPeers(ctx)
	if !utils.SliceContainsString(peeredVservers, remoteSVM) {
		err = fmt.Errorf("backend SVM %v is not peered with required SVM %v", svm, remoteSVM)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	return nil
}

// validateReplicationPolicy checks that a given replication policy is valid and returns if the policy is
// of async type
func validateReplicationPolicy(ctx context.Context, policyName string, d api.OntapAPI) (bool, error) {
	if policyName == "" {
		return false, nil
	}

	// Validate replication options
	snapmirrorPolicy, err := d.SnapmirrorPolicyGet(ctx, policyName)
	if err != nil {
		return false, fmt.Errorf("error getting snapmirror policy: %v", err)
	}
	if snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeSync() {
		// If the policy is synchronous we're fine
		return false, nil
	} else if !snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeAsync() {
		return false, fmt.Errorf("unsupported mirror policy type %v, must be %v or %v",
			snapmirrorPolicy.Type, api.SnapmirrorPolicyZAPITypeSync, api.SnapmirrorPolicyZAPITypeAsync)
	}

	// If the policy is async, check below for correct rule
	// Check async policies for the "all_source_snapshots" rule
	if snapmirrorPolicy.Type.IsSnapmirrorPolicyTypeAsync() {
		if snapmirrorPolicy.CopyAllSnapshots {
			return true, nil
		}
		return true, fmt.Errorf("snapmirror policy %v is of type %v and is missing the %v rule",
			policyName, api.SnapmirrorPolicyZAPITypeAsync, api.SnapmirrorPolicyRuleAll)
	}
	return false, nil
}

func validateReplicationSchedule(ctx context.Context, replicationSchedule string, d api.OntapAPI) error {
	if replicationSchedule != "" {
		if _, err := d.JobScheduleExists(ctx, replicationSchedule); err != nil {
			return err
		}
	}

	return nil
}

func validateReplicationConfig(
	ctx context.Context, replicationPolicy, replicationSchedule string, d api.OntapAPI,
) error {
	if _, err := validateReplicationPolicy(ctx, replicationPolicy, d); err != nil {
		return fmt.Errorf("failed to validate replication policy: %v", replicationPolicy)
	}

	if err := validateReplicationSchedule(ctx, replicationSchedule, d); err != nil {
		return fmt.Errorf("failed to validate replication schedule: %v", replicationSchedule)
	}

	return nil
}

// releaseMirror will release the snapmirror relationship data of the source volume
func releaseMirror(ctx context.Context, localInternalVolumeName string, d api.OntapAPI) error {
	// release any previous snapmirror relationship
	if localInternalVolumeName == "" {
		return fmt.Errorf("invalid volume name")
	}
	localSVMName := d.SVMName()
	err := d.SnapmirrorRelease(ctx, localInternalVolumeName, localSVMName)
	if err != nil {
		return err
	}
	return nil
}

// getReplicationDetails returns the replication policy and schedule of a snapmirror relationship
// and the SVM name and UUID
func getReplicationDetails(ctx context.Context, localInternalVolumeName, remoteVolumeHandle string, d api.OntapAPI) (string, string, string, error) {
	localSVMName := d.SVMName()

	// Empty remote means there is no mirror to check for
	if remoteVolumeHandle == "" {
		return "", "", localSVMName, nil
	}

	if localInternalVolumeName == "" {
		return "", "", localSVMName, fmt.Errorf("invalid volume name")
	}

	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return "", "", localSVMName, fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v",
			remoteVolumeHandle,
			err)
	}

	snapmirror, err := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if !api.IsNotFoundError(err) {
			return "", "", localSVMName, err
		} else {
			Logc(ctx).WithError(err).Error("error on snapmirror get")
			return "", "", localSVMName, err
		}
	}

	return snapmirror.ReplicationPolicy, snapmirror.ReplicationSchedule, localSVMName, nil
}

// mirrorUpdate attempts a mirror update for the specified Flexvol.
func mirrorUpdate(ctx context.Context, localInternalVolumeName, snapshotName string, d api.OntapAPI) error {
	if localInternalVolumeName == "" {
		return fmt.Errorf("invalid volume name")
	}

	err := d.SnapmirrorUpdate(ctx, localInternalVolumeName, snapshotName)
	if err != nil {
		return err
	}
	return errors.InProgressError("mirror update started")
}

// checkMirrorTransferState will look at the transfer state of the mirror relationship to determine if it is failed,
// succeeded or in progress and return the EndTransferTime if it succeeded or failed
func checkMirrorTransferState(ctx context.Context, localInternalVolumeName string, d api.OntapAPI) (*time.Time, error) {
	localSVMName := d.SVMName()
	if localInternalVolumeName == "" {
		return nil, fmt.Errorf("invalid volume name")
	}

	mirror, err := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "")
	if err != nil {
		return nil, err
	}
	if mirror == nil {
		return nil, fmt.Errorf("could not get mirror")
	}

	// May need to add finalizing state, similar to transferring
	switch mirror.RelationshipStatus {
	case api.SnapmirrorStatusTransferring:
		// return InProgressError and requeue
		return nil, errors.InProgressError("mirror update not complete, still transferring")
	case api.SnapmirrorStatusFailed:
		// return failed
		return nil, fmt.Errorf("mirror update failed, %v", mirror.UnhealthyReason)
	case api.SnapmirrorStatusSuccess, api.SnapmirrorStatusIdle:
		// If mirror is not healthy return error
		if !mirror.IsHealthy {
			return nil, fmt.Errorf("mirror update failed, %v", mirror.UnhealthyReason)
		}
		if mirror.EndTransferTime == nil {
			return nil, fmt.Errorf("mirror does not have a transfer time")
		}
		// return no error, mirror update succeeded
		return mirror.EndTransferTime, nil
	default:
		// return deferred error and remain in progress to retry and recheck the state
		return nil, errors.InProgressError(fmt.Sprintf("unexpected mirror transfer status %v for update",
			mirror.RelationshipStatus))
	}
}

// getMirrorTransferTime returns the transfer time of the mirror relationship
func getMirrorTransferTime(ctx context.Context, localInternalVolumeName string, d api.OntapAPI) (*time.Time, error) {
	localSVMName := d.SVMName()
	if localInternalVolumeName == "" {
		return nil, fmt.Errorf("invalid volume name")
	}

	mirror, err := d.SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "")
	if err != nil {
		return nil, err
	}
	if mirror == nil {
		return nil, fmt.Errorf("could not get mirror")
	}
	if mirror.EndTransferTime == nil {
		return nil, nil
	}

	return mirror.EndTransferTime, nil
}
