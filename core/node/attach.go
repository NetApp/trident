// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

const (
	AttachISCSIVolumeTimeoutShort = 20 * time.Second
	AttachFCPVolumeTimeoutShort   = 20 * time.Second
)

var (
	afterInitialTrackingInfoWrite  = fiji.Register("afterInitialTrackingInfoWrite", "node_core")
	betweenAttachAndLUKSPassphrase = fiji.Register("betweenAttachAndLUKSPassphrase", "node_core")
	beforeTrackingInfoWrite        = fiji.Register("beforeTrackingInfoWrite", "node_core")
)

// AttachRequest holds inputs for staging a volume on the node (CSI NodeStageVolume).
// PublishInfo carries protocol-specific publish context; SharedTarget applies to block
// protocols and controls shared-target iSCSI logout behavior during detach.
type AttachRequest struct {
	PublishInfo  *models.VolumePublishInfo
	SharedTarget bool
}

func (c *Core) Attach(ctx context.Context, volumeID string, req AttachRequest) (err error) {
	if volumeID == "" {
		return errors.InvalidInputError("volumeID is empty")
	}
	publishInfo := req.PublishInfo
	if publishInfo == nil {
		return errors.InvalidInputError("nil publishInfo")
	}

	switch publishInfo.StorageProtocol {
	case FCP, ISCSI, NVMe:
		publishInfo.SharedTarget = req.SharedTarget
	}

	protocol := publishInfo.StorageProtocol
	targetPath := publishInfo.GlobalMount
	fields := LogFields{
		"Method":     "Attach",
		"Type":       "Node_Core",
		"Volume":     volumeID,
		"TargetPath": targetPath,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Attach")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Attach")

	if err = c.checkReady(); err != nil {
		return err
	}
	release, err := c.acquireVolumeLock(ctx, volumeID)
	if err != nil {
		return err
	}
	defer release()

	trackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		PublishedPaths:    map[string]struct{}{},
	}
	if err = c.localStore.WriteTrackingInfo(ctx, volumeID, trackingInfo); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeID":          volumeID,
			"stagingTargetPath": targetPath,
		}).WithError(err).Error("Could not write tracking file.")
		return err
	}

	// afterInitialTrackingInfoWrite/beforeTrackingInfoWrite below are scoped to FCP/iSCSI/NVMe
	// (and NVMe-only, respectively) to match the fault points these protocols exercised prior to
	// the node core: NFS/SMB never had an equivalent initial tracking-file write to hook.
	switch protocol {
	case FCP, ISCSI, NVMe:
		if err := afterInitialTrackingInfoWrite.Inject(); err != nil {
			return err
		}
	}

	defer func() {
		if protocol == NVMe {
			if injectErr := beforeTrackingInfoWrite.Inject(); injectErr != nil {
				err = injectErr
			}
		}

		// Always write a volume tracking info for use in node publish & unstage calls.
		trackingInfo = &models.VolumeTrackingInfo{
			VolumePublishInfo: *publishInfo, // This may be healed during attach. Update it to the latest value.
			PublishedPaths:    map[string]struct{}{},
		}
		if fileErr := c.localStore.WriteTrackingInfo(ctx, volumeID, trackingInfo); fileErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID":          volumeID,
				"stagingTargetPath": targetPath,
			}).WithError(fileErr).Error("Could not write tracking file.")

			// If an attachment error exists, then we should capture that failure along with a write file error.
			if err != nil {
				err = fmt.Errorf("attachment failed: %v; could not write tracking file: %v", err, fileErr)
			} else {
				err = fmt.Errorf("could not write tracking file: %v", fileErr)
			}
		}
	}()

	// Dispatch to the various protocols.
	switch protocol {
	case NFS:
		err = c.attachNFSVolume(ctx, publishInfo)
	case SMB:
		err = c.attachSMBVolume(ctx, volumeID, publishInfo)
	case NVMe:
		err = c.attachNVMeVolume(ctx, volumeID, publishInfo)
	case ISCSI:
		err = c.attachISCSIVolume(ctx, volumeID, publishInfo)
	case FCP:
		err = c.attachFCPVolume(ctx, volumeID, publishInfo)
	default:
		err = errors.UnsupportedError("unknown storage protocol")
	}
	return err
}

func (c *Core) attachNFSVolume(ctx context.Context, publishInfo *models.VolumePublishInfo) error {
	Logc(ctx).Debug(">>>> attachNFSVolume")
	defer Logc(ctx).Debug("<<<< attachNFSVolume")

	release, err := c.acquireLimiter(ctx, attachNFSVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	return c.mount.IsCompatible(ctx, publishInfo.FilesystemType)
}

func (c *Core) attachSMBVolume(ctx context.Context, volume string, publishInfo *models.VolumePublishInfo) error {
	Logc(ctx).Debug(">>>> attachSMBVolume")
	defer Logc(ctx).Debug("<<<< attachSMBVolume")

	release, err := c.acquireLimiter(ctx, attachSMBVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	if err := c.mount.IsCompatible(ctx, publishInfo.FilesystemType); err != nil {
		return err
	}

	// Remote-map the SMB share to the staging path.
	return c.mount.AttachSMBVolume(
		ctx, volume, publishInfo.GlobalMount, publishInfo.SMBADUser, publishInfo.SMBADPass, publishInfo,
	)
}

func (c *Core) attachISCSIVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo,
) (err error) {
	Logc(ctx).Debug(">>>> attachISCSIVolume")
	defer Logc(ctx).Debug("<<<< attachISCSIVolume")

	release, err := c.acquireLimiter(ctx, attachISCSIVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	iSCSINodeOperationWaitingCount.Add(1)
	iSCSISelfHealingLock.RLock()
	defer iSCSISelfHealingLock.RUnlock()
	iSCSINodeOperationWaitingCount.Add(-1)

	var mpathSize int64
	mpathSize, err = c.ensureAttachISCSIVolume(ctx, volume, publishInfo, AttachISCSIVolumeTimeoutShort)
	if err != nil {
		return err
	}

	// Cryptsetup format if necessary and map to host
	luksFormatted, safeToFsFormat, err := luks.EnsureCryptsetupFormattedAndMappedOnHost(
		ctx, publishInfo.InternalID, publishInfo, publishInfo.Secrets, c.cmd, c.dev,
	)
	if err != nil {
		return err
	}

	// Format and mount if necessary
	if err = c.iscsi.EnsureVolumeFormattedAndMounted(
		ctx, publishInfo.InternalID, "", publishInfo, luksFormatted, safeToFsFormat,
	); err != nil {
		return err
	}

	if convert.ToBool(publishInfo.LUKSEncryption) {
		if err = betweenAttachAndLUKSPassphrase.Inject(); err != nil {
			return err
		}
		luksDevice := luks.NewDevice(publishInfo.DevicePath, publishInfo.InternalID, c.cmd, c.dev)

		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, luksDevice, volume, publishInfo.Secrets, true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase; %w", err)
		}
	}

	if mpathSize > 0 {
		Logc(ctx).Warn("Multipath device size may not be correct, performing gratuitous resize.")
		if err = c.expandISCSIVolume(ctx, volume, publishInfo, mpathSize, publishInfo.Secrets); err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID":        volume,
				"multipathSize":   mpathSize,
				"multipathDevice": publishInfo.DevicePath,
			}).WithError(err).Warn("Attempt to perform gratuitous resize failed.")
		}
	}

	// Update in-mem map used for self-healing; do it after a volume has been staged.
	// Beyond here if there is a problem with the session or there are missing LUNs
	// then self-healing should be able to fix those issues.
	newCtx := context.WithValue(ctx, iscsi.SessionInfoSource, iscsi.SessionSourceNodeStage)

	// Acquiring the global self-healing session lock may impact parallelism,
	// but self-healing session operations are minimal and should complete quickly.
	// Therefore, a slight performance impact is acceptable to keep the code clean and maintainable.
	lockContext := "attachISCSIVolume.AddSession"
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
		return errors.MaxWaitExceededError("request waited too long for the lock")
	}
	c.iscsi.AddSession(newCtx, publishedISCSISessions, publishInfo, volume, "", models.NotInvalid)
	locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
	return nil
}

// ensureAttachISCSIVolume attempts to attach the volume to the local host with a retry logic
// based on the publish information passed in. It returns the multipath device size, if any,
// so callers can perform a gratuitous resize after formatting/mounting.
func (c *Core) ensureAttachISCSIVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo, attachTimeout time.Duration,
) (int64, error) {
	var err error
	var mpathSize int64

	Logc(ctx).Debug(">>>> ensureAttachISCSIVolume")
	defer Logc(ctx).Debug("<<<< ensureAttachISCSIVolume")

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if mpathSize, err = c.iscsi.AttachVolumeRetry(ctx, publishInfo, attachTimeout); err != nil {
		// Did we fail to log in?
		if !errors.IsAuthError(err) {
			return mpathSize, err
		}

		// Update CHAP info from the controller and try one more time.
		Logc(ctx).Warn("iSCSI login failed; will retrieve CHAP credentials from Trident controller and try again.")
		chapInfo, chapErr := c.controller.CHAPInfo(ctx, volume, c.hostName)
		if chapErr != nil {
			msg := "could not retrieve CHAP credentials from Trident controller"
			Logc(ctx).WithError(chapErr).Error(msg)
			return mpathSize, errors.New(msg)
		}
		publishInfo.IscsiChapInfo = *chapInfo

		if mpathSize, err = c.iscsi.AttachVolumeRetry(ctx, publishInfo, attachTimeout); err != nil {
			// Bail out no matter what as we've now tried with updated credentials
			return mpathSize, err
		}
	}

	return mpathSize, nil
}

func (c *Core) attachFCPVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo,
) (err error) {
	Logc(ctx).Debug(">>>> attachFCPVolume")
	defer Logc(ctx).Debug("<<<< attachFCPVolume")

	release, err := c.acquireLimiter(ctx, attachFCPVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	var mpathSize int64
	// Attach the volume to the node
	mpathSize, err = c.ensureAttachFCPVolume(ctx, publishInfo, AttachFCPVolumeTimeoutShort)
	if err != nil {
		return err
	}

	// Cryptsetup format if necessary and map to host
	luksFormatted, safeToFsFormat, err := luks.EnsureCryptsetupFormattedAndMappedOnHost(
		ctx, publishInfo.InternalID, publishInfo, publishInfo.Secrets, c.cmd, c.dev,
	)
	if err != nil {
		return err
	}

	// Format and mount if necessary
	if err = c.fcp.EnsureVolumeFormattedAndMounted(
		ctx, publishInfo.InternalID, "", publishInfo, luksFormatted, safeToFsFormat,
	); err != nil {
		return err
	}

	if convert.ToBool(publishInfo.LUKSEncryption) {
		if err = betweenAttachAndLUKSPassphrase.Inject(); err != nil {
			return err
		}
		luksDevice := luks.NewDevice(publishInfo.DevicePath, publishInfo.InternalID, c.cmd, c.dev)

		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, luksDevice, volume, publishInfo.Secrets, true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase; %w", err)
		}
	}

	if mpathSize > 0 {
		Logc(ctx).Warn("Multipath device size may not be correct, performing gratuitous resize.")
		err = c.expandFCPVolume(ctx, volume, publishInfo, mpathSize, publishInfo.Secrets)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID":        volume,
				"multipathSize":   mpathSize,
				"multipathDevice": publishInfo.DevicePath,
			}).WithError(err).Warn("Attempt to perform gratuitous resize failed.")
		}
	}

	return nil
}

// ensureAttachFCPVolume attempts to attach the volume to the local host
// with a retry logic based on the publish information passed in.
func (c *Core) ensureAttachFCPVolume(
	ctx context.Context, publishInfo *models.VolumePublishInfo, attachTimeout time.Duration,
) (int64, error) {
	var err error
	var mpathSize int64

	Logc(ctx).Debug(">>>> ensureAttachFCPVolume")
	defer Logc(ctx).Debug("<<<< ensureAttachFCPVolume")

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if mpathSize, err = c.fcp.AttachVolumeRetry(ctx, publishInfo, attachTimeout); err != nil {
		return mpathSize, err
	}

	return mpathSize, nil
}

func (c *Core) attachNVMeVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> attachNVMeVolume")
	defer Logc(ctx).Debug("<<<< attachNVMeVolume")

	release, err := c.acquireLimiter(ctx, attachNVMeVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	// Acquire self-healing read lock to allow parallel operations
	nvmeNodeOperationWaitingCount.Add(1)
	nvmeSelfHealingLock.RLock()
	defer nvmeSelfHealingLock.RUnlock()
	nvmeNodeOperationWaitingCount.Add(-1)

	err = c.nvme.AttachNVMeVolumeRetry(ctx, publishInfo, nvme.NVMeAttachTimeout)
	if err != nil {
		return err
	}

	// Cryptsetup format if necessary and map to host
	luksFormatted, safeToFormat, err := c.nvme.EnsureCryptsetupFormattedAndMappedOnHost(
		ctx, publishInfo.InternalID, publishInfo, publishInfo.Secrets,
	)
	if err != nil {
		return err
	}

	// Format and mount if necessary
	if err = c.nvme.EnsureVolumeFormattedAndMounted(
		ctx, publishInfo.InternalID, "", publishInfo, luksFormatted, safeToFormat,
	); err != nil {
		return err
	}

	if convert.ToBool(publishInfo.LUKSEncryption) {
		if err = betweenAttachAndLUKSPassphrase.Inject(); err != nil {
			return err
		}
		luksDevice := luks.NewDevice(publishInfo.DevicePath, publishInfo.InternalID, c.cmd, c.dev)

		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, luksDevice, volume, publishInfo.Secrets, true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase; %w", err)
		}
	}

	lockContext := "nodeStageNVMeVolume.AddSession"
	if !attemptLock(ctx, lockContext, nvmeSelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)
		return errors.MaxWaitExceededError("request waited too long for the lock")
	}
	c.nvme.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)
	locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)
	return nil
}
