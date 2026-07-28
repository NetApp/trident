// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"
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

// PruneRequest removes stale attachment paths during volume-move reconciliation.
type PruneRequest struct {
	models.VolumeAccessInfo
	Protocol tridentconfig.Protocol
}

// Prune removes unwanted iSCSI sessions and paths for a LUN after a volume has moved,
// without disrupting other LUNs or sessions. If it is safe to log out, this operation may safely
// log out of a session. It is the counterpart to GraftAttachment in the volume-move workflow.
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
	trackingInfo, err := c.localStore.ReadTrackingInfo(ctx, volumeID)
	if err != nil && !errors.IsNotFoundError(err) {
		Logc(ctx).WithFields(fields).WithError(err).Warn(
			"Error reading tracking file for volume; stale sessions may persist. Continuing with prune attachment workflow.")
	}

	publishInfo := &models.VolumePublishInfo{}
	if trackingInfo != nil {
		// Init the publish info from the tracking info.
		publishInfo = &trackingInfo.VolumePublishInfo
	}
	publishInfo.VolumeAccessInfo = convert.ToVal(req.VolumeAccessInfo.DeepCopy())

	switch req.Protocol {
	case tridentconfig.Block:
		return c.pruneISCSIAttachment(ctx, volumeID, req, publishInfo)
	case tridentconfig.File:
		fallthrough
	default:
		msg := fmt.Sprintf("operation not supported with %s protocol", req.Protocol)
		return nil, errors.TerminalReconciliationError(msg)
	}
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
