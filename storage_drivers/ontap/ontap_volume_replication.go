// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"strings"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

const (
	SnapmirrorStatusIdle         = "idle"
	SnapmirrorStatusTransferring = "transferring"
	SnapmirrorStatusChecking     = "checking"
	SnapmirrorStatusQuiescing    = "quiescing"
	SnapmirrorStatusQuiesced     = "quiesced"
	SnapmirrorStatusQueued       = "queued"
	SnapmirrorStatusPreparing    = "preparing"
	SnapmirrorStatusFinalizing   = "finalizing"
	SnapmirrorStatusAborting     = "aborting"
	SnapmirrorStatusBreaking     = "breaking"

	SnapmirrorStateUninitialized = "uninitialized"
	SnapmirrorStateSnapmirrored  = "snapmirrored"
	SnapmirrorStateBroken        = "broken-off"

	SnapmirrorPolicyTypeSync        = "sync_mirror"
	SnapmirrorPolicyTypeAsync       = "async_mirror"
	SnapmirrorPolicyTypeVault       = "vault"
	SnapmirrorPolicyTypeMirrorVault = "mirror_vault"

	SnapmirrorPolicyRuleAll = "all_source_snapshots"
)

// establishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship
func establishMirror(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle string, ontap *api.Client,
	driverConfig *storagedrivers.OntapStorageDriverConfig,
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
	volType, err := ontap.VolumeGetType(localFlexvolName)
	if err != nil {
		msg := "could not determine volume type"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	if volType != "dp" {
		return fmt.Errorf("mirrors can only be established with empty DP volumes as the destination")
	}

	// Check if a snapmirror relationship already exists
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	relationshipNotFound := false
	initialized := false
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return err
			}
		}
	}
	if relationshipNotFound {
		_, err := ontap.SnapmirrorCreate(
			localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
			driverConfig.ReplicationPolicy, driverConfig.ReplicationSchedule,
		)
		if err != nil {
			if zerr, ok := err.(api.ZapiError); ok && zerr.Code() != azgo.EDUPLICATEENTRY {
				Logc(ctx).WithError(err).Error("Error on snapmirror create")
				return err
			}
		}
		// Get the snapmirror after creation
		snapmirror, err = ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapmirror, err); err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return err
		}
	}

	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	initialized = info.MirrorState() != SnapmirrorStateUninitialized
	snapmirrorIdle := info.RelationshipStatus() == SnapmirrorStatusIdle

	// Initialize the snapmirror
	if !initialized && snapmirrorIdle {
		_, err := ontap.SnapmirrorInitialize(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if zerr, ok := err.(api.ZapiError); ok {
			if !zerr.IsPassed() {
				// Snapmirror is current initializing
				if zerr.Code() == azgo.ETRANSFERINPROGRESS {
					Logc(ctx).Debug("snapmirror transfer already in progress")
				} else {
					Logc(ctx).WithError(err).Error("Error on snapmirror initialize")
					return err
				}
			}
		}

	}
	return nil
}

// reestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously
func reestablishMirror(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, ontap *api.Client,
	driverConfig *storagedrivers.OntapStorageDriverConfig,
) error {
	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	relationshipNotFound := false
	initialized := false
	snapmirrorIdle := true

	// Check if a snapmirror relationship already exists
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return err
			}
		}
	}
	if snapmirror != nil && !relationshipNotFound {
		attributes := snapmirror.Result.Attributes()
		info := attributes.SnapmirrorInfo()
		initialized = info.MirrorState() != SnapmirrorStateUninitialized || info.LastTransferType() != ""
		snapmirrorIdle = info.RelationshipStatus() == SnapmirrorStatusIdle
	}

	// If the snapmirror is already established we have nothing to do
	if initialized && snapmirrorIdle {
		return nil
	}

	// Create the relationship if it doesn't exist
	if relationshipNotFound {
		snapCreate, err := ontap.SnapmirrorCreate(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
			driverConfig.ReplicationPolicy, driverConfig.ReplicationSchedule)
		if err = api.GetError(ctx, snapCreate, err); err != nil {
			if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EDUPLICATEENTRY {
				Logc(ctx).WithError(err).Error("Error on snapmirror create")
				return err
			}
		}
	}

	// Resync the relationship
	snapResync, err := ontap.SnapmirrorResync(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapResync, err); err != nil {
		if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.ETRANSFERINPROGRESS {
			Logc(ctx).WithError(err).Error("Error on snapmirror resync")
			// If we fail on the resync, we need to cleanup the snapmirror
			// it will be recreated in a future TMR reconcile loop through this function
			snapDelete, deleteErr := ontap.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName,
				remoteSVMName)
			if deleteErr = api.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
				Logc(ctx).WithError(deleteErr).Warn("Error on snapmirror delete")
			}
			return err
		}
	}

	// Verify the state of the relationship
	snapmirror, err = ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			// If the snapmirror does not exist yet, we need to check back later
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return utils.ReconcileIncompleteError()
			}
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return err
		}
	}
	// Check if the snapmirror is healthy
	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	if !info.IsHealthy() {
		err = fmt.Errorf(info.UnhealthyReason())
		Logc(ctx).WithError(err).Error("Error on snapmirror resync")
		snapDelete, deleteErr := ontap.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName,
			remoteSVMName)
		if deleteErr = api.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
			Logc(ctx).WithError(deleteErr).Warn("Error on snapmirror delete")
		}
		return err
	}
	return nil
}

// promoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced
func promoteMirror(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, snapshotHandle string,
	ontap *api.Client, driverConfig *storagedrivers.OntapStorageDriverConfig,
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

	relationshipNotFound := false
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return false, err
			}
		}
	}

	if driverConfig.ReplicationPolicy != "" {
		smPolicy, err := ontap.SnapmirrorPolicyGet(ctx, driverConfig.ReplicationPolicy)
		if err != nil {
			return false, err
		}
		// If the policy is a synchronous type we shouldn't wait for a snapshot
		if smPolicy.Type() == SnapmirrorPolicyTypeSync {
			snapshotHandle = ""
		}
	}

	// Check for snapshot
	if snapshotHandle != "" {
		snapshotTokens := strings.Split(snapshotHandle, "/")
		if len(snapshotTokens) != 2 {
			return false, fmt.Errorf("invalid snapshot handle %v", snapshotHandle)
		}
		_, snapshotName, err := storage.ParseSnapshotID(snapshotHandle)
		if err != nil {
			return false, err
		}

		found := false
		snapshotResponse, _ := ontap.SnapshotList(localFlexvolName)
		snapshotList := snapshotResponse.Result.AttributesList()
		for _, snapshot := range snapshotList.SnapshotInfo() {
			if snapshot.Name() == snapshotName {
				found = true
				break
			}
		}

		if !found {
			Logc(ctx).WithField("snapshot", snapshotHandle).Debug("Snapshot not yet present.")
			return true, nil
		}
	}

	if !relationshipNotFound {
		snapQuiesce, err := ontap.SnapmirrorQuiesce(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapQuiesce, err); err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror quiesce")
			return false, err
		}

		snapAbort, err := ontap.SnapmirrorAbort(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapAbort, err); err != nil {
			if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.ENOTRANSFERINPROGRESS {
				// Check if we're still aborting - ZAPI returns a generic 13001 error code when an abort is already
				// in progress
				if aborting, err2 := isSnapmirrorAborting(
					ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, ontap,
				); err2 != nil {
					msg := "error checking if snapmirror is aborting"
					Logc(ctx).WithError(err2).Error(msg)
					return false, fmt.Errorf(msg)
				} else if aborting {
					return false, nil
				}
				Logc(ctx).WithError(err).Error("Error on snapmirror abort")
				return false, err
			}
		}

		snapmirror, err = ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapmirror, err); err != nil {
			if zerr, ok := err.(api.ZapiError); ok {
				if !zerr.IsPassed() {
					Logc(ctx).WithError(err).Error("Error on snapmirror get")
					return false, err
				}
			}
		}

		// Skip the break if snapmirror is uninitialized, otherwise it will fail saying the volume is not initialized
		snapmirrorAttributes := snapmirror.Result.Attributes()
		snapmirrorInfo := snapmirrorAttributes.SnapmirrorInfo()
		if snapmirrorInfo.MirrorState() != SnapmirrorStateUninitialized {
			snapBreak, err := ontap.SnapmirrorBreak(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err = api.GetError(ctx, snapBreak, err); err != nil {
				if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EDEST_ISNOT_DP_VOLUME {
					Logc(ctx).WithError(err).Info("Error on snapmirror break")
					return false, err
				}
			}
		}

		snapDelete, err := ontap.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapDelete, err); err != nil {
			Logc(ctx).WithError(err).Info("Error on snapmirror delete")
			return false, err
		}
	}
	return false, nil
}

func isSnapmirrorAborting(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string, ontap *api.Client,
) (bool, error) {
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		msg := "error on snapmirror get"
		Logc(ctx).WithError(err).Error(msg)
		return false, fmt.Errorf(msg)
	}
	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	return info.RelationshipStatus() == SnapmirrorStatusAborting, nil
}

// getMirrorStatus returns the current state of a snapmirror relationship
func getMirrorStatus(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, ontap *api.Client,
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

	created := true
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EOBJECTNOTFOUND {
			// If snapmirror is gone then the volume is promoted
			return v1.MirrorStatePromoted, nil
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			created = false
		}
	}

	// Translate the snapmirror status to a mirror status
	if created {
		attributes := snapmirror.Result.Attributes()
		info := attributes.SnapmirrorInfo()
		mirrorState := info.MirrorState()
		relationshipStatus := info.RelationshipStatus()
		switch relationshipStatus {
		case SnapmirrorStatusBreaking:
			return v1.MirrorStatePromoting, nil
		case SnapmirrorStatusQuiescing:
			return v1.MirrorStatePromoting, nil
		case SnapmirrorStatusAborting:
			return v1.MirrorStatePromoting, nil
		default:
			switch mirrorState {
			case SnapmirrorStateBroken:
				if relationshipStatus == SnapmirrorStatusTransferring {
					return v1.MirrorStateEstablishing, nil
				}
				return v1.MirrorStatePromoting, nil
			case SnapmirrorStateUninitialized:
				return v1.MirrorStateEstablishing, nil
			case SnapmirrorStateSnapmirrored:
				return v1.MirrorStateEstablished, nil
			}
		}
	}
	return "", nil
}

func checkSVMPeered(
	ctx context.Context, volConfig *storage.VolumeConfig, ontap *api.Client,
	driverConfig *storagedrivers.OntapStorageDriverConfig,
) error {
	remoteSVM, _, err := parseVolumeHandle(volConfig.PeerVolumeHandle)
	if err != nil {
		err = fmt.Errorf("could not determine required peer SVM; %v", err)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	peeredVservers, _ := ontap.GetPeeredVservers(ctx)
	if !utils.SliceContainsString(peeredVservers, remoteSVM) {
		err = fmt.Errorf("backend SVM %v is not peered with required SVM %v", driverConfig.SVM, remoteSVM)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	return nil
}

func ValidateReplicationPolicy(ctx context.Context, policyName string, api *api.Client) error {
	if policyName == "" {
		return nil
	}

	// Validate replication options
	exists, err := api.SnapmirrorPolicyExists(ctx, policyName)
	if err != nil {
		return fmt.Errorf("failed to list replication policies: %v", err)
	} else if !exists {
		return fmt.Errorf("specified replicationPolicy %v does not exist", policyName)
	}

	smPolicy, err := api.SnapmirrorPolicyGet(ctx, policyName)
	if err != nil {
		return fmt.Errorf("error getting snapmirror policy: %v", err)
	}

	switch smPolicy.Type() {
	case SnapmirrorPolicyTypeSync:
		// If the policy is synchronous we're fine
		return nil
	case SnapmirrorPolicyTypeAsync:
		// If the policy is async, check below for correct rule
		break
	default:
		return fmt.Errorf("unsupported mirror policy type %v, must be %v or %v",
			smPolicy.Type(), SnapmirrorPolicyTypeSync, SnapmirrorPolicyTypeAsync)
	}

	// Check async policies for the "all_source_snapshots" rule
	rulesList := smPolicy.SnapmirrorPolicyRules()
	rules := rulesList.SnapmirrorPolicyRuleInfo()

	for _, rule := range rules {
		if rule.SnapmirrorLabel() == SnapmirrorPolicyRuleAll {
			return nil
		}
	}
	return fmt.Errorf("snapmirror policy %v is of type %v and is missing the %v rule",
		policyName, SnapmirrorPolicyTypeAsync, SnapmirrorPolicyRuleAll)
}

func ValidateReplicationSchedule(ctx context.Context, replicationSchedule string, api *api.Client) error {
	if replicationSchedule != "" {
		exists, err := api.JobScheduleExists(ctx, replicationSchedule)
		if err != nil {
			return fmt.Errorf("failed to list job schedules: %v", err)
		} else if !exists {
			return fmt.Errorf("specified replicationSchedule %v does not exist", replicationSchedule)
		}
	}

	return nil
}

func ValidateReplicationConfig(
	ctx context.Context, config *storagedrivers.OntapStorageDriverConfig, api *api.Client,
) error {
	if err := ValidateReplicationPolicy(ctx, config.ReplicationPolicy, api); err != nil {
		return fmt.Errorf("failed to validate replication policy: %v", config.ReplicationPolicy)
	}

	// TODO: Check for replication policy (about rules) is of type async and replication schedule is empty,
	//  log a message

	if err := ValidateReplicationSchedule(ctx, config.ReplicationSchedule, api); err != nil {
		return fmt.Errorf("failed to validate replication schedule: %v", config.ReplicationSchedule)
	}

	return nil
}

// //////////////////////////////////////////////////////////////////////////////////////////////////////
// TODO: Refactor duplicate functions when combining ontap_nas and ontap_nas_abstraction files
// //////////////////////////////////////////////////////////////////////////////////////////////////////

// establishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship
func establishMirrorAbstraction(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle string, ontap api.OntapAPI,
	driverConfig *storagedrivers.OntapStorageDriverConfig,
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
	volType, err := ontap.VolumeGetType(localFlexvolName)
	if err != nil {
		msg := "could not determine volume type"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	if volType != "dp" {
		return fmt.Errorf("mirrors can only be established with empty DP volumes as the destination")
	}

	// Check if a snapmirror relationship already exists
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	relationshipNotFound := false
	initialized := false
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return err
			}
		}
	}
	if relationshipNotFound {
		_, err := ontap.SnapmirrorCreate(
			localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
			driverConfig.ReplicationPolicy, driverConfig.ReplicationSchedule,
		)
		if err != nil {
			if zerr, ok := err.(api.ZapiError); ok && zerr.Code() != azgo.EDUPLICATEENTRY {
				Logc(ctx).WithError(err).Error("Error on snapmirror create")
				return err
			}
		}
		// Get the snapmirror after creation
		snapmirror, err = ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapmirror, err); err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return err
		}
	}

	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	initialized = info.MirrorState() != SnapmirrorStateUninitialized
	snapmirrorIdle := info.RelationshipStatus() == SnapmirrorStatusIdle

	// Initialize the snapmirror
	if !initialized && snapmirrorIdle {
		_, err := ontap.SnapmirrorInitialize(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if zerr, ok := err.(api.ZapiError); ok {
			if !zerr.IsPassed() {
				// Snapmirror is current initializing
				if zerr.Code() == azgo.ETRANSFERINPROGRESS {
					Logc(ctx).Debug("snapmirror transfer already in progress")
				} else {
					Logc(ctx).WithError(err).Error("Error on snapmirror initialize")
					return err
				}
			}
		}

	}
	return nil
}

// reestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously
func reestablishMirrorAbstraction(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, ontap api.OntapAPI,
	driverConfig *storagedrivers.OntapStorageDriverConfig,
) error {
	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	relationshipNotFound := false
	initialized := false
	snapmirrorIdle := true

	// Check if a snapmirror relationship already exists
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return err
			}
		}
	}
	if snapmirror != nil && !relationshipNotFound {
		attributes := snapmirror.Result.Attributes()
		info := attributes.SnapmirrorInfo()
		initialized = info.MirrorState() != SnapmirrorStateUninitialized || info.LastTransferType() != ""
		snapmirrorIdle = info.RelationshipStatus() == SnapmirrorStatusIdle
	}

	// If the snapmirror is already established we have nothing to do
	if initialized && snapmirrorIdle {
		return nil
	}

	// Create the relationship if it doesn't exist
	if relationshipNotFound {
		snapCreate, err := ontap.SnapmirrorCreate(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
			driverConfig.ReplicationPolicy, driverConfig.ReplicationSchedule)
		if err = api.GetError(ctx, snapCreate, err); err != nil {
			if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EDUPLICATEENTRY {
				Logc(ctx).WithError(err).Error("Error on snapmirror create")
				return err
			}
		}
	}

	// Resync the relationship
	snapResync, err := ontap.SnapmirrorResync(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapResync, err); err != nil {
		if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.ETRANSFERINPROGRESS {
			Logc(ctx).WithError(err).Error("Error on snapmirror resync")
			// If we fail on the resync, we need to cleanup the snapmirror
			// it will be recreated in a future TMR reconcile loop through this function
			snapDelete, deleteErr := ontap.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName,
				remoteSVMName)
			if deleteErr = api.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
				Logc(ctx).WithError(deleteErr).Warn("Error on snapmirror delete")
			}
			return err
		}
	}

	// Verify the state of the relationship
	snapmirror, err = ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			// If the snapmirror does not exist yet, we need to check back later
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return utils.ReconcileIncompleteError()
			}
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return err
		}
	}
	// Check if the snapmirror is healthy
	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	if !info.IsHealthy() {
		err = fmt.Errorf(info.UnhealthyReason())
		Logc(ctx).WithError(err).Error("Error on snapmirror resync")
		snapDelete, deleteErr := ontap.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName,
			remoteSVMName)
		if deleteErr = api.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
			Logc(ctx).WithError(deleteErr).Warn("Error on snapmirror delete")
		}
		return err
	}
	return nil
}

// promoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced
func promoteMirrorAbstraction(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, snapshotHandle string,
	ontap api.OntapAPI, driverConfig *storagedrivers.OntapStorageDriverConfig,
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

	relationshipNotFound := false
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return false, err
			}
		}
	}

	if driverConfig.ReplicationPolicy != "" {
		smPolicy, err := ontap.SnapmirrorPolicyGet(ctx, driverConfig.ReplicationPolicy)
		if err != nil {
			return false, err
		}
		// If the policy is a synchronous type we shouldn't wait for a snapshot
		if smPolicy.Type() == SnapmirrorPolicyTypeSync {
			snapshotHandle = ""
		}
	}

	// Check for snapshot
	if snapshotHandle != "" {
		snapshotTokens := strings.Split(snapshotHandle, "/")
		if len(snapshotTokens) != 2 {
			return false, fmt.Errorf("invalid snapshot handle %v", snapshotHandle)
		}
		_, snapshotName, err := storage.ParseSnapshotID(snapshotHandle)
		if err != nil {
			return false, err
		}

		found := false
		snapshotList, _ := ontap.VolumeSnapshotList(ctx, localFlexvolName)
		for _, snapshot := range snapshotList {
			if snapshot.Name == snapshotName {
				found = true
				break
			}
		}

		if !found {
			Logc(ctx).WithField("snapshot", snapshotHandle).Debug("Snapshot not yet present.")
			return true, nil
		}
	}

	if !relationshipNotFound {
		snapQuiesce, err := ontap.SnapmirrorQuiesce(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapQuiesce, err); err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror quiesce")
			return false, err
		}

		snapAbort, err := ontap.SnapmirrorAbort(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapAbort, err); err != nil {
			if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.ENOTRANSFERINPROGRESS {
				// Check if we're still aborting - ZAPI returns a generic 13001 error code when an abort is already
				// in progress
				if aborting, err2 := isSnapmirrorAbortingAbstraction(
					ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, ontap,
				); err2 != nil {
					msg := "error checking if snapmirror is aborting"
					Logc(ctx).WithError(err2).Error(msg)
					return false, fmt.Errorf(msg)
				} else if aborting {
					return false, nil
				}
				Logc(ctx).WithError(err).Error("Error on snapmirror abort")
				return false, err
			}
		}

		snapmirror, err = ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapmirror, err); err != nil {
			if zerr, ok := err.(api.ZapiError); ok {
				if !zerr.IsPassed() {
					Logc(ctx).WithError(err).Error("Error on snapmirror get")
					return false, err
				}
			}
		}

		// Skip the break if snapmirror is uninitialized, otherwise it will fail saying the volume is not initialized
		snapmirrorAttributes := snapmirror.Result.Attributes()
		snapmirrorInfo := snapmirrorAttributes.SnapmirrorInfo()
		if snapmirrorInfo.MirrorState() != SnapmirrorStateUninitialized {
			snapBreak, err := ontap.SnapmirrorBreak(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err = api.GetError(ctx, snapBreak, err); err != nil {
				if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EDEST_ISNOT_DP_VOLUME {
					Logc(ctx).WithError(err).Info("Error on snapmirror break")
					return false, err
				}
			}
		}

		snapDelete, err := ontap.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapDelete, err); err != nil {
			Logc(ctx).WithError(err).Info("Error on snapmirror delete")
			return false, err
		}
	}
	return false, nil
}

func isSnapmirrorAbortingAbstraction(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string, ontap api.OntapAPI,
) (bool, error) {
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		msg := "error on snapmirror get"
		Logc(ctx).WithError(err).Error(msg)
		return false, fmt.Errorf(msg)
	}
	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	return info.RelationshipStatus() == SnapmirrorStatusAborting, nil
}

// getMirrorStatus returns the current state of a snapmirror relationship
func getMirrorStatusAbstraction(
	ctx context.Context, localVolumeHandle string, remoteVolumeHandle string, ontap api.OntapAPI,
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

	created := true
	snapmirror, err := ontap.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EOBJECTNOTFOUND {
			// If snapmirror is gone then the volume is promoted
			return v1.MirrorStatePromoted, nil
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			created = false
		}
	}

	// Translate the snapmirror status to a mirror status
	if created {
		attributes := snapmirror.Result.Attributes()
		info := attributes.SnapmirrorInfo()
		mirrorState := info.MirrorState()
		relationshipStatus := info.RelationshipStatus()
		switch relationshipStatus {
		case SnapmirrorStatusBreaking:
			return v1.MirrorStatePromoting, nil
		case SnapmirrorStatusQuiescing:
			return v1.MirrorStatePromoting, nil
		case SnapmirrorStatusAborting:
			return v1.MirrorStatePromoting, nil
		default:
			switch mirrorState {
			case SnapmirrorStateBroken:
				if relationshipStatus == SnapmirrorStatusTransferring {
					return v1.MirrorStateEstablishing, nil
				}
				return v1.MirrorStatePromoting, nil
			case SnapmirrorStateUninitialized:
				return v1.MirrorStateEstablishing, nil
			case SnapmirrorStateSnapmirrored:
				return v1.MirrorStateEstablished, nil
			}
		}
	}
	return "", nil
}

func checkSVMPeeredAbstraction(
	ctx context.Context, volConfig *storage.VolumeConfig, ontap api.OntapAPI,
	driverConfig *storagedrivers.OntapStorageDriverConfig,
) error {
	remoteSVM, _, err := parseVolumeHandle(volConfig.PeerVolumeHandle)
	if err != nil {
		err = fmt.Errorf("could not determine required peer SVM; %v", err)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	peeredVservers, _ := ontap.GetPeeredVservers(ctx)
	if !utils.SliceContainsString(peeredVservers, remoteSVM) {
		err = fmt.Errorf("backend SVM %v is not peered with required SVM %v", driverConfig.SVM, remoteSVM)
		return storagedrivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}
	return nil
}

func ValidateReplicationPolicyAbstraction(ctx context.Context, policyName string, api api.OntapAPI) error {
	if policyName == "" {
		return nil
	}

	// Validate replication options
	exists, err := api.SnapmirrorPolicyExists(ctx, policyName)
	if err != nil {
		return fmt.Errorf("failed to list replication policies: %v", err)
	} else if !exists {
		return fmt.Errorf("specified replicationPolicy %v does not exist", policyName)
	}

	smPolicy, err := api.SnapmirrorPolicyGet(ctx, policyName)
	if err != nil {
		return fmt.Errorf("error getting snapmirror policy: %v", err)
	}

	switch smPolicy.Type() {
	case SnapmirrorPolicyTypeSync:
		// If the policy is synchronous we're fine
		return nil
	case SnapmirrorPolicyTypeAsync:
		// If the policy is async, check below for correct rule
		break
	default:
		return fmt.Errorf("unsupported mirror policy type %v, must be %v or %v",
			smPolicy.Type(), SnapmirrorPolicyTypeSync, SnapmirrorPolicyTypeAsync)
	}

	// Check async policies for the "all_source_snapshots" rule
	rulesList := smPolicy.SnapmirrorPolicyRules()
	rules := rulesList.SnapmirrorPolicyRuleInfo()

	for _, rule := range rules {
		if rule.SnapmirrorLabel() == SnapmirrorPolicyRuleAll {
			return nil
		}
	}
	return fmt.Errorf("snapmirror policy %v is of type %v and is missing the %v rule",
		policyName, SnapmirrorPolicyTypeAsync, SnapmirrorPolicyRuleAll)
}

func ValidateReplicationScheduleAbstraction(ctx context.Context, replicationSchedule string, api api.OntapAPI) error {
	if replicationSchedule != "" {
		exists, err := api.JobScheduleExists(ctx, replicationSchedule)
		if err != nil {
			return fmt.Errorf("failed to list job schedules: %v", err)
		} else if !exists {
			return fmt.Errorf("specified replicationSchedule %v does not exist", replicationSchedule)
		}
	}

	return nil
}

func ValidateReplicationConfigAbstraction(
	ctx context.Context, config *storagedrivers.OntapStorageDriverConfig, api api.OntapAPI,
) error {
	if err := ValidateReplicationPolicyAbstraction(ctx, config.ReplicationPolicy, api); err != nil {
		return fmt.Errorf("failed to validate replication policy: %v", config.ReplicationPolicy)
	}

	// TODO: Check for replication policy (about rules) is of type async and replication schedule is empty,
	//  log a message

	if err := ValidateReplicationScheduleAbstraction(ctx, config.ReplicationSchedule, api); err != nil {
		return fmt.Errorf("failed to validate replication schedule: %v", config.ReplicationSchedule)
	}

	return nil
}
