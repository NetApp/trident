// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"path"
	"time"

	"github.com/cenkalti/backoff/v4"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

const (
	fcpNodeUnstageMaxDuration   = 15 * time.Second
	iSCSINodeUnstageMaxDuration = 15 * time.Second
	nvmeMaxFlushWaitDuration    = 6 * time.Minute

	// removeMultipathDeviceMappingRetries/Delay bound how long we retry tearing down
	// a multipath device mapping before giving up.
	removeMultipathDeviceMappingRetries    = 4
	removeMultipathDeviceMappingRetryDelay = 500 * time.Millisecond

	// nvmeFlushRetryMapLock is the LockID for the self-healing global lock guarding
	// NVMeNamespacesFlushRetry.
	nvmeFlushRetryMapLock = "nvmeFlushRetryMapLock"
)

var (
	fcpUtils   = utils.FcpUtils
	iscsiUtils = iscsi.IscsiUtils

	// NVMeNamespacesFlushRetry - Non-persistent map of Namespaces to maintain the flush errors if any.
	// During NodeUnstageVolume, Trident shall return success after specific wait time (nvmeMaxFlushWaitDuration).
	NVMeNamespacesFlushRetry = make(map[string]time.Time)

	duringIscsiLogout         = fiji.Register("duringIscsiLogout", "node_core")
	afterNvmeLuksDeviceClosed = fiji.Register("afterNvmeLuksDeviceClosed", "node_core")
	afterNvmeDisconnect       = fiji.Register("afterNvmeDisconnect", "node_core")
)

// DetachRequest holds inputs for unstaging a volume (CSI NodeUnstageVolume).
type DetachRequest struct{}

func (c *Core) Detach(ctx context.Context, volume string, _ DetachRequest) error {
	fields := LogFields{
		"Method": "Detach",
		"Type":   "Node_Core",
		"Volume": volume,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Detach")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Detach")

	if volume == "" {
		return errors.New("volume is empty")
	}

	if err := c.checkReady(); err != nil {
		return err
	}
	release, err := c.acquireVolumeLock(ctx, volume)
	if err != nil {
		return err
	}
	defer release()

	return c.detach(ctx, volume, false)
}

// detach tears down a volume attachment on a host.
// It assumes the caller holds any required locking.
func (c *Core) detach(ctx context.Context, volume string, force bool) error {
	if volume == "" {
		return errors.New("volume is empty")
	}

	fields := LogFields{
		"Method": "detach",
		"Type":   "Node_Core",
		"Volume": volume,
		"Force":  force,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> detach")
	defer Logc(ctx).WithFields(fields).Trace("<<<< detach")

	trackingInfo, err := c.localStore.ReadTrackingInfo(ctx, volume)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(fields).Warning("Volume tracking info file not found, returning success.")
			return nil
		}

		file := c.trackingFilePath(volume)
		if errors.IsInvalidJSONError(err) {
			errMsgTemplate := "The volume tracking file is not readable because it was not valid JSON: %s ."
			Logc(ctx).WithFields(fields).WithError(err).Errorf(errMsgTemplate, file)
		} else {
			Logc(ctx).WithFields(fields).WithError(err).Errorf("Unable to read the volume tracking file %s", file)
		}
		if errors.IsInvalidJSONError(err) {
			return errors.InternalError("unable to read the volume tracking file %s; %v", file, err)
		}
		return err
	}
	publishInfo := &trackingInfo.VolumePublishInfo
	protocol := publishInfo.StorageProtocol
	fields["Protocol"] = protocol

	switch protocol {
	case NFS:
		return c.detachNFSVolume(ctx, volume)
	case SMB:
		return c.detachSMBVolume(ctx, volume, trackingInfo)
	case FCP:
		return c.detachFCPVolumeRetry(ctx, volume, publishInfo, force)
	case ISCSI:
		return c.detachISCSIVolumeRetry(ctx, volume, trackingInfo, force)
	case NVMe:
		return c.detachNVMeVolume(ctx, volume, publishInfo, force)
	default:
		return errors.PreconditionError("unknown storage protocol")
	}
}

func (c *Core) detachNFSVolume(ctx context.Context, volume string) error {
	Logc(ctx).Debug(">>>> detachNFSVolume")
	defer Logc(ctx).Debug("<<<< detachNFSVolume")

	release, err := c.acquireLimiter(ctx, detachNFSVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := c.localStore.DeleteTrackingInfo(ctx, volume); err != nil {
		return err
	}
	return nil
}

func (c *Core) detachSMBVolume(ctx context.Context, volume string, trackingInfo *models.VolumeTrackingInfo) error {
	Logc(ctx).Debug(">>>> detachSMBVolume")
	defer Logc(ctx).Debug("<<<< detachSMBVolume")

	release, err := c.acquireLimiter(ctx, detachSMBVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	stagingTargetPath := trackingInfo.VolumePublishInfo.GlobalMount

	mappingPath, err := c.fs.GetUnmountPath(ctx, trackingInfo)
	if err != nil {
		return err
	}

	err = c.mount.UmountSMBPath(ctx, mappingPath, stagingTargetPath)

	// Delete the device info we saved to the volume tracking info path so unstage can succeed,
	// regardless of the unmount outcome above.
	if delErr := c.localStore.DeleteTrackingInfo(ctx, volume); delErr != nil {
		return delErr
	}

	return err
}

func (c *Core) detachFCPVolumeRetry(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo, force bool,
) error {
	Logc(ctx).Debug(">>>> detachFCPVolumeRetry")
	defer Logc(ctx).Debug("<<<< detachFCPVolumeRetry")

	// Acquired once here rather than per detachFCPVolume retry attempt below.
	release, err := c.acquireLimiter(ctx, detachFCPVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	detachFCPVolumeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Failed to unstage the volume, retrying.")
	}

	detachFCPVolumeAttempt := func() error {
		return c.detachFCPVolume(ctx, volume, publishInfo, force)
	}

	detachFCPVolumeBackoff := backoff.NewExponentialBackOff()
	detachFCPVolumeBackoff.InitialInterval = 1 * time.Second
	detachFCPVolumeBackoff.Multiplier = 1.414 // approx sqrt(2)
	detachFCPVolumeBackoff.RandomizationFactor = 0.1
	detachFCPVolumeBackoff.MaxElapsedTime = fcpNodeUnstageMaxDuration

	if err := backoff.RetryNotify(detachFCPVolumeAttempt, detachFCPVolumeBackoff, detachFCPVolumeNotify); err != nil {
		Logc(ctx).Error("failed to unstage volume")
		return err
	}
	return nil
}

func (c *Core) detachFCPVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo, force bool,
) error {
	hostSessionMap := fcpUtils.GetFCPHostSessionMapForTarget(ctx, publishInfo.FCTargetWWNN)
	paths := fcpUtils.GetSysfsBlockDirsForLUN(int(publishInfo.FCPLunNumber), hostSessionMap)
	deviceNames, err := fcpUtils.GetDevicesForLUN(paths)
	if err != nil {
		return fmt.Errorf("could not get devices for LUN: %v", err)
	}
	if len(deviceNames) == 0 {
		// If we are in this block it likely means we have errored or had a pod restart
		// before the tracking file has been removed. We need to ensure the device was removed and remove the
		// tracking file, without going through the rest of the detach process.
		if convert.ToBool(publishInfo.LUKSEncryption) {
			var luksMapperPath string
			fields := LogFields{"device": publishInfo.DevicePath}
			// Set device path to dm device to correctly verify legacy volumes.
			if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
				luksMapperPath = publishInfo.DevicePath
				dmPath, dmErr := luks.GetDmDevicePathFromLUKSLegacyPath(ctx, c.cmd, publishInfo.DevicePath)
				if dmErr != nil {
					Logc(ctx).WithFields(fields).WithError(dmErr).Warn(
						"Could not determine dm device path from legacy LUKS device path. " +
							"Continuing with device removal.")
				} else {
					publishInfo.DevicePath = dmPath
				}
			} else {
				luksMapperPath, err = c.dev.GetLUKSDeviceForMultipathDevice(publishInfo.DevicePath)
				if err != nil {
					if !errors.IsNotFoundError(err) {
						Logc(ctx).WithFields(fields).WithError(err).Warn(
							"Could not determine LUKS device path from multipath device. " +
								"Continuing with device removal.")
					}
					Logc(ctx).WithFields(fields).Info("No LUKS device path found from multipath device.")
				}
			}
			if err = c.dev.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath); err != nil {
				Logc(ctx).WithError(err).Debug("Unable to remove LUKS device. Continuing with tracking file removal.")
			}
		}
		if err = c.dev.RemoveMultipathDeviceMappingWithRetries(ctx, publishInfo.DevicePath,
			removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
			Logc(ctx).Warn("Unable to remove multipath device. Continuing with tracking file removal.")
		}
		if err = c.localStore.DeleteTrackingInfo(ctx, volume); err != nil {
			return err
		}
		return nil
	}

	deviceInfo, err := c.fcp.GetDeviceInfoForLUN(ctx, hostSessionMap, int(publishInfo.FCPLunNumber),
		publishInfo.FCTargetWWNN, false)
	if err != nil {
		return fmt.Errorf("could not get device info: %v", err)
	}
	if deviceInfo == nil {
		Logc(ctx).Debug("Could not find devices, nothing to do.")
		return nil
	}

	var luksMapperPath string
	if convert.ToBool(publishInfo.LUKSEncryption) && deviceInfo.MultipathDevice != "" {
		fields := LogFields{
			"luksDevicePath":  publishInfo.DevicePath,
			"lunID":           publishInfo.FCPLunNumber,
			"multipathDevice": deviceInfo.MultipathDevice,
		}

		luksMapperPath, err = c.dev.GetLUKSDeviceForMultipathDevice(deviceInfo.MultipathDevice)
		if err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get LUKS device path from multipath device.")
				return err
			}
			Logc(ctx).WithFields(fields).Info("No LUKS device path found from multipath device.")
		}

		if luksMapperPath != "" {
			fields["mapperPath"] = luksMapperPath
			if err = c.dev.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath); err != nil {
				if !errors.IsMaxWaitExceededError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
					return err
				}
				Logc(ctx).WithFields(fields).WithError(err).
					Debug("LUKS close wait time exceeded continuing with device removal.")
			}
		}

		if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
			publishInfo.DevicePath = deviceInfo.MultipathDevice
		}
	}

	// Delete the device from the host.
	unmappedMpathDevice, err := c.fcp.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, nil, c.unsafeDetach, force)
	if err != nil {
		if errors.IsFCPSameLunNumberError(err) {
			// There is a need to pass all the publish infos this time
			unmappedMpathDevice, err = c.fcp.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo,
				c.readAllTrackingFiles(ctx), c.unsafeDetach, force)
		}
		if err != nil && !c.unsafeDetach {
			return err
		}
	}

	stagingTargetPath := publishInfo.GlobalMount

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err = c.mount.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volume,
			"stagingTargetPath": stagingTargetPath,
		}).WithError(err).Errorf("Failed to remove directory in staging target path.")
		return fmt.Errorf("failed to remove temporary directory in staging target path %s; %w", stagingTargetPath, err)
	}

	// If the LUKS device still exists, it means the device was unable to be closed prior to removing the block
	// device. This can happen if the LUN was deleted or offline. It should be removable by this point.
	// It needs to be removed prior to removing the 'unmappedMpathDevice' device below.
	if luksMapperPath != "" {
		if err = c.dev.EnsureLUKSDeviceClosed(ctx, luksMapperPath); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": luksMapperPath,
			}).WithError(err).Warning("Unable to remove LUKS mapper device.")
		}
		devices.LuksCloseDurations.RemoveDurationTracking(luksMapperPath)
	}

	// If there is multipath device, flush(remove) mappings
	if err = c.dev.RemoveMultipathDeviceMappingWithRetries(ctx, unmappedMpathDevice,
		removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
		return err
	}

	if err = c.localStore.DeleteTrackingInfo(ctx, volume); err != nil {
		return err
	}

	return nil
}

func (c *Core) detachISCSIVolumeRetry(
	ctx context.Context, volume string, trackingInfo *models.VolumeTrackingInfo, force bool,
) error {
	Logc(ctx).Debug(">>>> detachISCSIVolumeRetry")
	defer Logc(ctx).Debug("<<<< detachISCSIVolumeRetry")

	// Acquired once here rather than per detachISCSIVolume retry attempt below.
	release, err := c.acquireLimiter(ctx, detachISCSIVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	// TODO (websterj): Rip this out when self-healing is refactored. Mirrors the RLock taken by
	// attachISCSIVolume: without it, the self-healing sweep's write lock can run a re-login/rescan
	// against a portal concurrently with this detach's own logout/device teardown.
	iSCSINodeOperationWaitingCount.Add(1)
	iSCSISelfHealingLock.RLock()
	defer iSCSISelfHealingLock.RUnlock()
	iSCSINodeOperationWaitingCount.Add(-1)

	detachISCSIVolumeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Failed to unstage the volume, retrying.")
	}

	detachISCSIVolumeAttempt := func() error {
		return c.detachISCSIVolume(ctx, volume, trackingInfo, force)
	}

	detachISCSIVolumeBackoff := backoff.NewExponentialBackOff()
	detachISCSIVolumeBackoff.InitialInterval = 1 * time.Second
	detachISCSIVolumeBackoff.Multiplier = 1.414 // approx sqrt(2)
	detachISCSIVolumeBackoff.RandomizationFactor = 0.1
	detachISCSIVolumeBackoff.MaxElapsedTime = iSCSINodeUnstageMaxDuration

	if err := backoff.RetryNotify(detachISCSIVolumeAttempt, detachISCSIVolumeBackoff, detachISCSIVolumeNotify); err != nil {
		Logc(ctx).Error("failed to unstage volume")
		return err
	}
	return nil
}

func (c *Core) detachISCSIVolume(
	ctx context.Context, volume string, trackingInfo *models.VolumeTrackingInfo, force bool,
) error {
	Logc(ctx).Debug(">>>> detachISCSIVolume")
	defer Logc(ctx).Debug("<<<< detachISCSIVolume")

	publishInfo := &trackingInfo.VolumePublishInfo

	// Default the device path to the value in the tracking file.
	devicePath := publishInfo.DevicePath

	// For some iSCSI backends, Trident cannot rely on the LUN serial.
	if publishInfo.IscsiLunSerial != "" {
		// Derive the device path using the LunSerial. The publishInfo.DevicePath may be incorrect due to Kernel
		// actions. Fallback to using the publishInfo.DevicePath if the multipath device cannot be derived.
		multipathDevice, err := c.dev.GetMultipathDeviceBySerial(ctx, hex.EncodeToString([]byte(publishInfo.IscsiLunSerial)))
		if err != nil {
			Logc(ctx).WithError(err).WithField("LunSerial", publishInfo.IscsiLunSerial).Debug(
				"Error finding multipath device by serial.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"multipathDevice": multipathDevice,
				"LunSerial":       publishInfo.IscsiLunSerial,
			}).Debug("Found multipath device by serial.")
			devicePath = iscsi.DevPrefix + multipathDevice
		}
	}
	publishInfo.DevicePath = devicePath

	hostSessionMap := iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, publishInfo.IscsiTargetIQN)
	if len(hostSessionMap) == 0 {
		// If we are in this block it likely means we have errored or had a pod restart after the iSCSI logout
		// and before the tracking file has been removed. We need to ensure the device was removed and remove
		// the tracking file, without going through the rest of the detach process.
		if convert.ToBool(publishInfo.LUKSEncryption) {
			var err error
			var luksMapperPath string
			fields := LogFields{"device": publishInfo.DevicePath}
			// Set device path to dm device to correctly verify legacy volumes.
			if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
				luksMapperPath = publishInfo.DevicePath
				dmPath, dmErr := luks.GetDmDevicePathFromLUKSLegacyPath(ctx, c.cmd, publishInfo.DevicePath)
				if dmErr != nil {
					Logc(ctx).WithFields(fields).WithError(dmErr).Warn(
						"Could not determine dm device path from legacy LUKS device path. " +
							"Continuing with device removal.")
				} else {
					publishInfo.DevicePath = dmPath
				}
			} else {
				luksMapperPath, err = c.dev.GetLUKSDeviceForMultipathDevice(publishInfo.DevicePath)
				if err != nil {
					if !errors.IsNotFoundError(err) {
						Logc(ctx).WithFields(fields).WithError(err).Warn(
							"Could not determine LUKS device path from multipath device. " +
								"Continuing with device removal.")
					}
					Logc(ctx).WithFields(fields).Info("No LUKS device path found from multipath device.")
				}
			}
			if err = c.dev.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath); err != nil {
				Logc(ctx).WithError(err).Debug("Unable to remove LUKS device. Continuing with tracking file removal.")
			}
		}
		if err := c.dev.RemoveMultipathDeviceMappingWithRetries(ctx, publishInfo.DevicePath,
			removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
			Logc(ctx).Warn("Unable to remove multipath device. Continuing with tracking file removal.")
		}
		return c.localStore.DeleteTrackingInfo(ctx, volume)
	}

	deviceInfo, err := c.iscsi.GetDeviceInfoForLUN(ctx, hostSessionMap, int(publishInfo.IscsiLunNumber),
		publishInfo.IscsiTargetIQN, false)
	if err != nil {
		Logc(ctx).WithError(err).Debug("Could not find devices.")
		return fmt.Errorf("could not get device info: %v", err)
	} else if deviceInfo == nil {
		Logc(ctx).Debug("No devices found.")
	}

	// Acquiring the global self-healing session lock may impact parallelism,
	// but self-healing session operations are minimal and should complete quickly.
	// Therefore, a slight performance impact is acceptable to keep the code clean and maintainable.
	lockContext := "detachISCSIVolume.RemoveLUNFromSessions"
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
		return errors.MaxWaitExceededError("request waited too long for the lock")
	}
	c.iscsi.RemoveLUNFromSessions(ctx, publishInfo, publishedISCSISessions)
	locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)

	var luksMapperPath string
	if convert.ToBool(publishInfo.LUKSEncryption) && publishInfo.DevicePath != "" {
		fields := LogFields{
			"lunID":           publishInfo.IscsiLunNumber,
			"publishedDevice": publishInfo.DevicePath,
		}

		// Use publishInfo.DevicePath (serial-resolved) rather than deviceInfo.MultipathDevice
		// (sysfs LUN discovery); the two can disagree when the kernel renumbers dm devices or
		// discovery finds a stale/wrong multipath node.
		luksMapperPath, err = c.dev.GetLUKSDeviceForMultipathDevice(publishInfo.DevicePath)
		if err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get LUKS device path from multipath device.")
				return err
			}
			Logc(ctx).WithFields(fields).Info("No LUKS device path found from multipath device.")
		}

		if luksMapperPath != "" {
			fields["luksDevice"] = luksMapperPath
			err = c.dev.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath)
			if err != nil {
				if !errors.IsMaxWaitExceededError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
					return err
				}
				Logc(ctx).WithFields(fields).WithError(err).Debug("LUKS close wait time exceeded, continuing with device removal.")
			}
		}

		// Set device path to dm device to correctly verify legacy volumes.
		if deviceInfo != nil && luks.IsLegacyDevicePath(publishInfo.DevicePath) {
			publishInfo.DevicePath = deviceInfo.MultipathDevice
		}
	}

	// Delete the device from the host.
	// Default this value to the healed value from before. This must be tracked because if the SCSI devices
	// are already gone, the deviceInfo above may be nil.
	mpathDevicePath := publishInfo.DevicePath
	if deviceInfo != nil {
		unmappedMpathDevice, prepErr := c.iscsi.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, nil,
			c.unsafeDetach, force)
		if prepErr != nil {
			if errors.IsISCSISameLunNumberError(prepErr) {
				// There is a need to pass all the publish infos this time
				mpathDevicePath, prepErr = c.iscsi.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo,
					c.readAllTrackingFiles(ctx), c.unsafeDetach, force)
			}
			if prepErr != nil && !c.unsafeDetach {
				return prepErr
			}
		}
		if unmappedMpathDevice != "" {
			mpathDevicePath = unmappedMpathDevice
		}
	}

	// Always check for a ghost multipath device.
	if mpathDevicePath != "" {
		if err = c.dev.RemoveGhostMultipathDevice(ctx, mpathDevicePath, publishInfo.IscsiLunSerial); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": mpathDevicePath,
			}).WithError(err).Warn("Failed to remove ghost multipath device.")
		}
	}

	// Logout of the iSCSI session if appropriate for each applicable host.
	logout := true
	if publishInfo.SharedTarget {
		// Check for any remaining mounts for this iSCSI target.
		anyMounts, mountErr := c.iscsi.TargetHasMountedDevice(ctx, publishInfo.IscsiTargetIQN)
		// It's only safe to logout if there are no mounts and no error occurred when checking.
		logout = !anyMounts && mountErr == nil

		// Since there are no mounts and no error occurred, we should check the hosts for any remaining devices.
		if logout {
			for hostNumber, sessionNumber := range hostSessionMap {
				if !c.iscsi.SafeToLogOut(ctx, hostNumber, sessionNumber) {
					// If even one host session is in use, we can't logout of the iSCSI sessions.
					logout = false
					break
				}
			}
		}
	}

	if logout {
		Logc(ctx).Debug("Safe to log out.")

		// Acquiring the global self-healing session lock may impact parallelism,
		// but self-healing session operations are minimal and should complete quickly.
		// Therefore, a slight performance impact is acceptable to keep the code clean and maintainable.
		lockContext = "detachISCSIVolume.RemovePortalsFromSession"
		if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, sharedLocksNodeLockTimeout) {
			locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
			return errors.MaxWaitExceededError("request waited too long for the lock")
		}
		c.iscsi.RemovePortalsFromSession(ctx, publishInfo, publishedISCSISessions)
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)

		if err = c.iscsi.Logout(ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiTargetPortal); err != nil {
			Logc(ctx).Error(err)
		}

		for _, portal := range publishInfo.IscsiPortals {
			if err = duringIscsiLogout.Inject(); err != nil {
				return err
			}

			if err = c.iscsi.Logout(ctx, publishInfo.IscsiTargetIQN, portal); err != nil {
				Logc(ctx).Error(err)
			}
		}
	}

	stagingTargetPath := publishInfo.GlobalMount

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err = c.mount.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volume,
			"stagingTargetPath": stagingTargetPath,
		}).WithError(err).Errorf("Failed to remove directory in staging target path.")
		return fmt.Errorf("failed to remove temporary directory in staging target path %s; %w", stagingTargetPath, err)
	}

	// If the LUKS device still exists, it means the device was unable to be closed prior to removing the block
	// device. This can happen if the LUN was deleted or offline. It should be removable by this point. It needs
	// to be removed prior to removing the 'mpathDevicePath' device below.
	if luksMapperPath != "" {
		// EnsureLUKSDeviceClosed will not return an error if the device is already closed or removed.
		if err = c.dev.EnsureLUKSDeviceClosed(ctx, luksMapperPath); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": luksMapperPath,
			}).WithError(err).Warning("Unable to remove LUKS mapper device.")
		}
		devices.LuksCloseDurations.RemoveDurationTracking(luksMapperPath)
	}

	// If there is multipath device, flush(remove) mappings.
	if err = c.dev.RemoveMultipathDeviceMappingWithRetries(ctx, mpathDevicePath,
		removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
		return err
	}

	return c.localStore.DeleteTrackingInfo(ctx, volume)
}

func (c *Core) detachNVMeVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo, force bool,
) error {
	Logc(ctx).Debug(">>>> detachNVMeVolume")
	defer Logc(ctx).Debug("<<<< detachNVMeVolume")

	release, err := c.acquireLimiter(ctx, detachNVMeVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	// Acquire self-healing read lock to allow parallel operations
	nvmeNodeOperationWaitingCount.Add(1)
	nvmeSelfHealingLock.RLock()
	defer nvmeSelfHealingLock.RUnlock()
	nvmeNodeOperationWaitingCount.Add(-1)

	nvmeSubsys := c.nvme.NewNVMeSubsystem(ctx, publishInfo.NVMeSubsystemNQN)
	// Get the device using 'nvme-cli' commands. Flush the device IOs.
	// Proceed further with detach flow, if device is not found.
	nvmeDev, err := nvmeSubsys.GetNVMeDevice(ctx, publishInfo.NVMeNamespaceUUID)
	if err != nil && !errors.IsNotFoundError(err) {
		return fmt.Errorf("failed to get NVMe device; %v", err)
	}

	devicePath := publishInfo.DevicePath
	if nvmeDev != nil {
		devicePath = nvmeDev.GetPath()
	}

	var luksMapperPath string
	if convert.ToBool(publishInfo.LUKSEncryption) && devicePath != "" {
		fields := LogFields{
			"namespace":     publishInfo.NVMeNamespaceUUID,
			"devicePath":    devicePath,
			"publishedPath": publishInfo.DevicePath,
		}

		luksMapperPath, err = c.dev.GetLUKSDevicePathForDevicePath(ctx, devicePath)
		if err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get LUKS device path from device path.")
				return err
			}
			Logc(ctx).WithFields(fields).WithError(err).Debug("Failed to get LUKS device path from device path. " +
				"Device may already be removed.")
		}

		if luksMapperPath != "" {
			fields["luksMapperPath"] = luksMapperPath
			if err = c.dev.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath); err != nil {
				if !errors.IsMaxWaitExceededError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
					return err
				}
				Logc(ctx).WithFields(fields).WithError(err).Debug("LUKS close wait time exceeded, continuing with device removal.")
			}
			if err = afterNvmeLuksDeviceClosed.Inject(); err != nil {
				return err
			}
		}
	}

	// Attempt to flush the NVMe device.
	if !nvmeDev.IsNil() {
		if err = nvmeDev.FlushDevice(ctx, c.unsafeDetach, force); err != nil {
			locks.Lock(ctx, "detachNVMeVolume.FlushRetryCheck", nvmeFlushRetryMapLock)
			if NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID].IsZero() {
				NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID] = time.Now()
				locks.Unlock(ctx, "detachNVMeVolume.FlushRetryCheck", nvmeFlushRetryMapLock)
				return fmt.Errorf("failed to flush NVMe device; %v", err)
			}

			elapsed := time.Since(NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID])
			locks.Unlock(ctx, "detachNVMeVolume.FlushRetryCheck", nvmeFlushRetryMapLock)

			if elapsed <= nvmeMaxFlushWaitDuration {
				Logc(ctx).WithFields(LogFields{
					"devicePath": devicePath,
					"namespace":  publishInfo.NVMeNamespaceUUID,
					"elapsed":    elapsed,
				}).WithError(err).Debug("Could not flush NVMe device.")
				return fmt.Errorf("failed to flush NVMe device; %v", err)
			}

			Logc(ctx).WithFields(LogFields{
				"namespace": publishInfo.NVMeNamespaceUUID,
				"elapsed":   elapsed,
				"maxWait":   nvmeMaxFlushWaitDuration,
			}).Warn("Could not flush device within expected time period.")
		}

		locks.Lock(ctx, "detachNVMeVolume.FlushRetryDelete", nvmeFlushRetryMapLock)
		delete(NVMeNamespacesFlushRetry, publishInfo.NVMeNamespaceUUID)
		locks.Unlock(ctx, "detachNVMeVolume.FlushRetryDelete", nvmeFlushRetryMapLock)
	}

	lockContext := "detachNVMeVolume.RemovePublishedNVMeSession"
	if !attemptLock(ctx, lockContext, nvmeSelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)
		return errors.MaxWaitExceededError("request waited too long for the lock")
	}
	c.nvme.RemovePublishedNVMeSession(&publishedNVMeSessions, publishInfo.NVMeSubsystemNQN,
		publishInfo.NVMeNamespaceUUID)
	locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)

	// Disconnect the subsystem if needed (handled under lock to prevent race conditions).
	if err = c.disconnectNVMeSubsystemIfNeeded(ctx, nvmeSubsys, publishInfo); err != nil {
		Logc(ctx).WithError(err).Warn("Error during subsystem disconnect check.")
		// Continue with cleanup even if disconnect fails.
	}

	if err = afterNvmeDisconnect.Inject(); err != nil {
		return err
	}

	stagingTargetPath := publishInfo.GlobalMount

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err = c.mount.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %v", err)
		return fmt.Errorf("failed to remove temporary directory in staging target path %s; %w", stagingTargetPath, err)
	}

	if luksMapperPath != "" {
		if err = c.dev.EnsureLUKSDeviceClosed(ctx, luksMapperPath); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": luksMapperPath,
			}).WithError(err).Warning("Unable to remove LUKS mapper device.")
		}
		devices.LuksCloseDurations.RemoveDurationTracking(luksMapperPath)
	}

	if err = c.localStore.DeleteTrackingInfo(ctx, volume); err != nil {
		return err
	}

	return nil
}

func (c *Core) trackingFilePath(volume string) string {
	return path.Join(tridentconfig.VolumeTrackingInfoPath, volume+".json")
}
