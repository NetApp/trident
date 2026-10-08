// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"
	"slices"
	"time"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// PruneAttachmentTimeoutShort bounds how long a single PruneAttachment attempt is given to
// tear down stale sessions/paths for an existing attachment before giving up.
const PruneAttachmentTimeoutShort = 15 * time.Second

// PruneRequest removes stale attachment paths
type PruneRequest struct {
	models.VolumeAccessInfo
	Protocol tridentconfig.Protocol
}

// Prune removes obsolete sessions and paths from an existing block attachment without
// tearing down the volume. Callers pass the access info to retain; protocol-specific handlers
// drop everything else that is safe to remove. It is the counterpart to Graft.
func (c *Core) Prune(
	ctx context.Context, volumeID string, req PruneRequest,
) (*models.PruneAttachmentResponse, error) {
	if volumeID == "" {
		return nil, errors.InvalidInputError("volume is empty")
	}

	fields := LogFields{
		"Method": "Prune",
		"Type":   "Node_Core",
		"Volume": volumeID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Prune")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Prune")

	if err := c.checkReady(); err != nil {
		return nil, err
	}
	release, err := c.acquireVolumeLock(ctx, volumeID)
	if err != nil {
		return nil, err
	}
	defer release()

	// Get the published info this node has a record of, if any.
	trackingInfo, err := c.nodeHelper.ReadTrackingInfo(ctx, volumeID)
	if err != nil && !errors.IsNotFoundError(err) {
		Logc(ctx).WithFields(fields).WithError(err).Warn(
			"Error reading tracking file for volume; stale sessions may persist. Continuing with prune attachment workflow.")
	}

	// iSCSI replaces VolumeAccessInfo with the request. NVMe keeps the tracked publish info;
	// pruneNVMeAttachment copies only the requested target IPs.
	storageProtocol := blockAttachmentProtocol(trackingInfo, req.VolumeAccessInfo)

	publishInfo := &models.VolumePublishInfo{}
	if trackingInfo != nil {
		publishInfo = &trackingInfo.VolumePublishInfo
	}
	if storageProtocol != NVMe {
		publishInfo.VolumeAccessInfo = convert.ToVal(req.VolumeAccessInfo.DeepCopy())
	}

	switch req.Protocol {
	case tridentconfig.Block:
		switch storageProtocol {
		case NVMe:
			return c.pruneNVMeAttachment(ctx, volumeID, req, publishInfo)
		default:
			return c.pruneISCSIAttachment(ctx, volumeID, req, publishInfo)
		}
	case tridentconfig.File:
		fallthrough
	default:
		msg := fmt.Sprintf("operation not supported with %s protocol", req.Protocol)
		return nil, errors.TerminalReconciliationError(msg)
	}
}

// pruneNVMeAttachment removes obsolete host paths while retaining at least one path, then records
// the desired target set in tracking and the in-memory self-healing session map. publishInfo is
// the tracked publish info. A nil or non-NVMe value means this node has no NVMe attachment. Only the
// target IPs are taken from the request; the tracked NVMe identity and other publish fields are retained.
// A subsystem the kernel does not have is an empty prune: nothing is disconnected, and the desired
// targets are still recorded so a later graft can connect them.
func (c *Core) pruneNVMeAttachment(
	ctx context.Context, volumeID string, req PruneRequest, publishInfo *models.VolumePublishInfo,
) (*models.PruneAttachmentResponse, error) {
	if publishInfo == nil || publishInfo.GetStorageProtocol() != NVMe {
		if isNVMeTargetIPsOnly(req.VolumeAccessInfo) {
			Logc(ctx).WithField("volume", volumeID).Debug(
				"Volume has no NVMe attachment on this node; nothing to prune.")
			return &models.PruneAttachmentResponse{VolumeName: volumeID, Protocol: req.Protocol}, nil
		}
		return nil, errors.TerminalReconciliationError("NVMe tracking info not found")
	}

	release, err := c.acquireLimiter(ctx, pruneNVMeAttachmentKey)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := validateNVMeIdentity(req.VolumeAccessInfo, publishInfo); err != nil {
		return nil, err
	}
	publishInfo.NVMeTargetIPs = slices.Clone(req.NVMeTargetIPs)

	nvmeNodeOperationWaitingCount.Add(1)
	nvmeSelfHealingLock.RLock()
	defer nvmeSelfHealingLock.RUnlock()
	nvmeNodeOperationWaitingCount.Add(-1)

	lockContext := "pruneNVMeAttachment.UpdateSession"
	if !attemptLock(ctx, lockContext, nvmeSelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)
		return nil, errors.MaxWaitExceededError("request waited too long for the lock")
	}
	defer locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)

	subsystem, err := c.nvme.GetNVMeSubsystem(ctx, publishInfo.NVMeSubsystemNQN)
	switch {
	case errors.IsNotFoundError(err):
		Logc(ctx).WithField("volume", volumeID).Debug(
			"NVMe subsystem has no host paths; recording the desired targets without pruning.")
	case err != nil:
		return nil, err
	default:
		subsystem.PrunePaths(ctx, publishInfo.NVMeTargetIPs)
	}

	if err := c.nodeHelper.UpdatePublishInfo(ctx, volumeID, publishInfo); err != nil {
		return nil, err
	}

	// AddPublishedNVMeSession merges target IPs, so remove the previous desired set first.
	if session := publishedNVMeSessions.Info[publishInfo.NVMeSubsystemNQN]; session != nil {
		publishedNVMeSessions.RemoveTargetIPsFromSession(
			publishInfo.NVMeSubsystemNQN, slices.Clone(session.NVMeTargetIPs),
		)
	}
	c.nvme.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)

	return &models.PruneAttachmentResponse{
		VolumeAccessInfo: req.VolumeAccessInfo,
		VolumeName:       volumeID,
		Protocol:         req.Protocol,
	}, nil
}

// pruneISCSIAttachment tears down the iSCSI sessions/paths described in publishInfo that are no
// longer wanted for the given LUN, without disrupting other LUNs sharing the same sessions.
func (c *Core) pruneISCSIAttachment(
	ctx context.Context, volumeID string, req PruneRequest, publishInfo *models.VolumePublishInfo,
) (*models.PruneAttachmentResponse, error) {
	if publishInfo == nil {
		return nil, errors.TerminalReconciliationError("publish info is nil")
	}

	fields := LogFields{"volume": volumeID, "lunID": publishInfo.IscsiLunNumber}
	Logc(ctx).WithFields(fields).Debug(">>>> pruneISCSIAttachment")
	defer Logc(ctx).WithFields(fields).Debug("<<<< pruneISCSIAttachment")

	release, err := c.acquireLimiter(ctx, pruneISCSIAttachmentKey)
	if err != nil {
		return nil, err
	}
	defer release()

	// Look for unreconcilable arguments.
	if req.IscsiLunNumber != publishInfo.IscsiLunNumber {
		return nil, errors.TerminalReconciliationError("lun number mismatch")
	} else if req.IscsiTargetIQN != publishInfo.IscsiTargetIQN {
		return nil, errors.TerminalReconciliationError("target IQN mismatch")
	} else if len(req.IscsiPortals) == 0 {
		return nil, errors.TerminalReconciliationError("no portals specified")
	} else if req.IscsiTargetPortal == "" {
		return nil, errors.TerminalReconciliationError("no target portal specified")
	}

	iSCSINodeOperationWaitingCount.Add(1)
	iSCSISelfHealingLock.RLock()
	defer iSCSISelfHealingLock.RUnlock()
	iSCSINodeOperationWaitingCount.Add(-1)

	// Acquiring the global self-healing session lock may impact parallelism, but self-healing
	// session operations are minimal and should complete quickly. Therefore, a slight
	// performance impact is acceptable to keep the code clean and maintainable.
	lockContext := "pruneISCSIAttachment.RemovePortalsFromSession"
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
		return nil, errors.MaxWaitExceededError("request waited too long for the lock")
	}
	defer locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)

	// The publish info here contains the portals to RETAIN on the host, not the portals to remove.
	attachInfo, err := c.iscsi.PruneAttachmentRetry(ctx, publishInfo, PruneAttachmentTimeoutShort)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not prune existing attachment.")
		return nil, err
	}

	// NOTE: attachInfo has the stale portals; it does not typically contain the target portals.
	c.iscsi.RemovePortalsFromSession(ctx, attachInfo.VolumePublishInfo, publishedISCSISessions)

	return &models.PruneAttachmentResponse{
		VolumeAccessInfo: req.VolumeAccessInfo,
		VolumeName:       volumeID,
		Protocol:         req.Protocol,
	}, nil
}
