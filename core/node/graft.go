// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

// GraftAttachmentTimeoutShort bounds how long a single GraftAttachment attempt is given to
// establish new sessions/paths for an existing attachment before giving up.
const GraftAttachmentTimeoutShort = AttachISCSIVolumeTimeoutShort

// GraftRequest extends an existing block attachment during volume-move reconciliation.
type GraftRequest struct {
	models.VolumeAccessInfo
	Protocol tridentconfig.Protocol
}

// Graft reconciles an existing volume attachment by extending it with new sessions
// and portal information without tearing down the current paths or devices. It reads the
// persisted tracking info for the named volume (if any), validates that invariant fields (LUN
// number, target IQN, portals) are consistent with the incoming request, then delegates to the
// protocol-specific graft handler. It is intended to support volume-move workflows, where the
// volume move controller asks every node the volume is (or will be) published to, to extend its
// attachment to a new set of paths. Only Block (iSCSI) protocol is supported today;
// File protocol returns a terminal reconciliation error.
func (c *Core) Graft(
	ctx context.Context, volumeID string, req GraftRequest,
) (*models.GraftAttachmentResponse, error) {
	if volumeID == "" {
		return nil, errors.InvalidInputError("volume is empty")
	}

	fields := LogFields{
		"Method": "Graft",
		"Type":   "Node_Core",
		"Volume": volumeID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Graft")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Graft")

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
		return nil, err
	}

	publishInfo := &models.VolumePublishInfo{}
	if trackingInfo != nil {
		// Init the publish info from the tracking info.
		publishInfo = &trackingInfo.VolumePublishInfo

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
	}
	publishInfo.VolumeAccessInfo = convert.ToVal(req.VolumeAccessInfo.DeepCopy())

	switch req.Protocol {
	case tridentconfig.Block:
		return c.graftISCSIAttachment(ctx, volumeID, req, publishInfo)
	case tridentconfig.File:
		fallthrough
	default:
		msg := fmt.Sprintf("operation not supported with %s protocol", req.Protocol)
		return nil, errors.TerminalReconciliationError(msg)
	}
}

// graftISCSIAttachment extends an existing iSCSI attachment for a LUN by establishing new
// sessions and waiting for new device paths to exist through those sessions. It does not remove
// existing iSCSI paths or devices; see PruneAttachment for that.
func (c *Core) graftISCSIAttachment(
	ctx context.Context, volumeID string, req GraftRequest, publishInfo *models.VolumePublishInfo,
) (*models.GraftAttachmentResponse, error) {
	if publishInfo == nil {
		return nil, errors.TerminalReconciliationError("publish info is nil")
	}

	fields := LogFields{"volume": volumeID, "lunID": publishInfo.IscsiLunNumber}
	Logc(ctx).WithFields(fields).Debug(">>>> graftISCSIAttachment")
	defer Logc(ctx).WithFields(fields).Debug("<<<< graftISCSIAttachment")

	release, err := c.acquireLimiter(ctx, graftISCSIAttachmentKey)
	if err != nil {
		return nil, err
	}
	defer release()

	iSCSINodeOperationWaitingCount.Add(1)
	iSCSISelfHealingLock.RLock()
	defer iSCSISelfHealingLock.RUnlock()
	iSCSINodeOperationWaitingCount.Add(-1)

	if publishInfo.UseCHAP {
		if err := c.decryptCHAPAccessInfo(ctx, &publishInfo.VolumeAccessInfo); err != nil {
			Logc(ctx).WithError(err).Warn("Could not decrypt CHAP credentials.")
			return nil, err
		}
	}

	// Ensure the existing attachment is extended.
	attachInfo, err := c.iscsi.GraftAttachmentRetry(ctx, publishInfo, GraftAttachmentTimeoutShort)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not extend existing attachment.")
		return nil, err
	}

	// Overwrite the access info.
	publishInfo.VolumeAccessInfo = convert.ToVal(attachInfo.VolumeAccessInfo.DeepCopy())

	// Update the tracking file.
	if err := c.localStore.UpdatePublishInfo(ctx, volumeID, publishInfo); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not update publish info.")
		return nil, err
	}

	// Update the self-healing map. This will have ALL sessions for a period of time; pre-existing
	// sessions and new sessions. This isn't great for busy systems, but during a volume move
	// operation, we cannot assume it is safe to omit new sessions or remove old sessions.
	newCtx := context.WithValue(ctx, iscsi.SessionInfoSource, iscsi.SessionSourceNodeGraft)
	lockContext := "graftISCSIAttachment.AddSession"
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
		return nil, errors.MaxWaitExceededError("request waited too long for the lock")
	}
	c.iscsi.AddSession(newCtx, publishedISCSISessions, publishInfo, volumeID, "", models.NotInvalid)
	locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)

	return &models.GraftAttachmentResponse{
		VolumeAccessInfo: attachInfo.VolumeAccessInfo,
		VolumeName:       volumeID,
		Protocol:         req.Protocol,
	}, nil
}

// decryptCHAPAccessInfo decrypts CHAP credentials within an access info in-place using the
// Core's configured AES key.
func (c *Core) decryptCHAPAccessInfo(ctx context.Context, accessInfo *models.VolumeAccessInfo) error {
	if accessInfo == nil || !accessInfo.UseCHAP {
		return nil
	}

	initiatorUser, err := crypto.DecryptStringWithAES(accessInfo.IscsiUsername, c.aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting iSCSI username; %v", err)
		return errors.New("error decrypting iscsi username")
	}
	initiatorPass, err := crypto.DecryptStringWithAES(accessInfo.IscsiInitiatorSecret, c.aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting initiator secret; %v", err)
		return errors.New("error decrypting initiator secret")
	}
	targetUser, err := crypto.DecryptStringWithAES(accessInfo.IscsiTargetUsername, c.aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting target username; %v", err)
		return errors.New("error decrypting target username")
	}
	targetPass, err := crypto.DecryptStringWithAES(accessInfo.IscsiTargetSecret, c.aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting target secret; %v", err)
		return errors.New("error decrypting target secret")
	}

	accessInfo.IscsiUsername = initiatorUser
	accessInfo.IscsiInitiatorSecret = initiatorPass
	accessInfo.IscsiTargetUsername = targetUser
	accessInfo.IscsiTargetSecret = targetPass

	return nil
}
