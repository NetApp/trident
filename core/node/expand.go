// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

// ExpandRequest holds inputs for node-side volume expansion (CSI NodeExpandVolume).
type ExpandRequest struct {
	MountPath     string
	RequiredBytes int64
	Secrets       map[string]string
}

// Expand grows an attached volume's usable capacity to requiredBytes. This corresponds to
// CSI's NodeExpandVolume. The CO only calls this for Block-protocol volumes; File-protocol
// volumes have nothing to expand at the node (the backend/filesystem already grew).
func (c *Core) Expand(ctx context.Context, volume string, req ExpandRequest) error {
	mountPath := req.MountPath
	requiredBytes := req.RequiredBytes
	secrets := req.Secrets

	if volume == "" {
		return errors.InvalidInputError("volume is empty")
	}
	if mountPath == "" {
		return errors.InvalidInputError("volume path is empty")
	}

	fields := LogFields{
		"Method":        "Expand",
		"Type":          "Node_Core",
		"Volume":        volume,
		"VolumePath":    mountPath,
		"RequiredBytes": requiredBytes,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Expand")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Expand")

	if err := c.checkReady(); err != nil {
		return err
	}
	release, err := c.acquireVolumeLock(ctx, volume)
	if err != nil {
		return err
	}
	defer release()

	trackingInfo, err := c.localStore.ReadTrackingInfo(ctx, volume)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return errors.NotFoundError("unable to find tracking file for volume: %s ; needed it for resize", volume)
		}
		return err
	}
	publishInfo := &trackingInfo.VolumePublishInfo
	protocol := publishInfo.StorageProtocol
	fields["Protocol"] = protocol

	stagingTargetPath := publishInfo.GlobalMount
	if stagingTargetPath != mountPath {
		Logc(ctx).WithFields(LogFields{
			"stagingTargetPath": stagingTargetPath,
			"volumePath":        mountPath,
			"volumeId":          volume,
		}).Warn("Received something other than the expected stagingTargetPath.")
	}

	switch protocol {
	case NFS, SMB:
		Logc(ctx).WithFields(fields).Info("Filesystem expansion check is not required for protocol.")
		return nil
	case ISCSI:
		return c.expandISCSIVolume(ctx, volume, publishInfo, requiredBytes, secrets)
	case FCP:
		return c.expandFCPVolume(ctx, volume, publishInfo, requiredBytes, secrets)
	case NVMe:
		return c.expandNVMeVolume(ctx, volume, publishInfo, requiredBytes, secrets)
	default:
		return errors.PreconditionError("unknown storage protocol")
	}
}

func (c *Core) expandISCSIVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo,
	requiredBytes int64, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> expandISCSIVolume")
	defer Logc(ctx).Debug("<<<< expandISCSIVolume")

	release, err := c.acquireLimiter(ctx, expandVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	lunID := int(publishInfo.IscsiLunNumber)
	Logc(ctx).WithFields(LogFields{
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for block device to expand.")

	// Capture the pre-expand size baseline before rescanning the device; the rescan itself grows
	// the device's reported size, so capturing the baseline afterward would make the "did the
	// filesystem actually grow" check below always pass trivially.
	preExpandDeviceSizeBytes, preExpandFilesystemSize, err := c.capturePreExpandSizeBaseline(ctx, publishInfo)
	if err != nil {
		return err
	}

	// Resize the volume (rescan + resize the SCSI device(s) and multipath map for the LUN).
	if err := c.iscsi.ExpandVolume(ctx, publishInfo, requiredBytes); err != nil {
		Logc(ctx).WithFields(LogFields{
			"lunID":      publishInfo.IscsiLunNumber,
			"devicePath": publishInfo.DevicePath,
		}).WithError(err).Error("Unable to resize device(s) for LUN.")
		return err
	}

	return c.expandFilesystemAndLUKS(
		ctx, volume, publishInfo, requiredBytes, secrets, preExpandDeviceSizeBytes, preExpandFilesystemSize,
	)
}

func (c *Core) expandFCPVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo,
	requiredBytes int64, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> expandFCPVolume")
	defer Logc(ctx).Debug("<<<< expandFCPVolume")

	release, err := c.acquireLimiter(ctx, expandVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	lunID := int(publishInfo.FCPLunNumber)
	Logc(ctx).WithFields(LogFields{
		"targetWWNN":     publishInfo.FCTargetWWNN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for block device to expand.")

	if !c.fcp.IsAlreadyAttached(ctx, lunID, publishInfo.FCTargetWWNN) {
		return fmt.Errorf("device %s to expand is not attached", publishInfo.DevicePath)
	}

	// Capture the pre-expand size baseline before rescanning the device; the rescan itself grows
	// the device's reported size, so capturing the baseline afterward would make the "did the
	// filesystem actually grow" check below always pass trivially.
	preExpandDeviceSizeBytes, preExpandFilesystemSize, err := c.capturePreExpandSizeBaseline(ctx, publishInfo)
	if err != nil {
		return err
	}

	if err := c.fcp.RescanDevices(ctx, publishInfo.FCTargetWWNN, publishInfo.FCPLunNumber, requiredBytes); err != nil {
		Logc(ctx).WithField("device", publishInfo.DevicePath).WithError(err).Error("Unable to scan device.")
		return err
	}

	return c.expandFilesystemAndLUKS(
		ctx, volume, publishInfo, requiredBytes, secrets, preExpandDeviceSizeBytes, preExpandFilesystemSize,
	)
}

func (c *Core) expandNVMeVolume(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo,
	requiredBytes int64, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> expandNVMeVolume")
	defer Logc(ctx).Debug("<<<< expandNVMeVolume")

	release, err := c.acquireLimiter(ctx, expandVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	// We don't need to rescan mount devices for the NVMe protocol backend. Automatic namespace
	// rescanning happens every time the NVMe controller is reset, or if the controller posts an
	// asynchronous event indicating namespace attributes have changed.
	Logc(ctx).WithField("volumeId", volume).Info("NVMe volume expansion check is not required.")

	preExpandDeviceSizeBytes, preExpandFilesystemSize, err := c.capturePreExpandSizeBaseline(ctx, publishInfo)
	if err != nil {
		return err
	}

	return c.expandFilesystemAndLUKS(
		ctx, volume, publishInfo, requiredBytes, secrets, preExpandDeviceSizeBytes, preExpandFilesystemSize,
	)
}

// capturePreExpandSizeBaseline reads the device and filesystem sizes before any protocol-specific
// rescan/resize runs. Callers MUST invoke this before triggering a rescan (e.g. FCP's
// RescanDevices): a rescan can grow the device's reported size immediately, so capturing the
// "pre-expand" baseline afterward would make expandFilesystemAndLUKS's growth-verification check
// trivially pass even when the filesystem failed to grow.
func (c *Core) capturePreExpandSizeBaseline(
	ctx context.Context, publishInfo *models.VolumePublishInfo,
) (preExpandDeviceSizeBytes, preExpandFilesystemSize int64, err error) {
	fsType, err := filesystem.VerifyFilesystemSupport(publishInfo.FilesystemType)
	if err != nil {
		return 0, 0, err
	}

	if fsType != filesystem.Raw {
		preExpandFilesystemSize, err = c.fs.GetFilesystemSize(ctx, publishInfo.GlobalMount)
		if err != nil {
			return 0, 0, err
		}
	}

	if publishInfo.DevicePath != "" {
		preExpandDeviceSizeBytes, err = c.dev.GetDiskSize(ctx, publishInfo.DevicePath)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": publishInfo.DevicePath,
			}).WithError(err).Warn("Failed to read pre-expand device size; skipping device growth check.")
			err = nil
		}
	}

	return preExpandDeviceSizeBytes, preExpandFilesystemSize, nil
}

// expandFilesystemAndLUKS performs the shared tail of the FCP/NVMe stopgap expand paths: resize
// the LUKS mapping (if encrypted) and then grow the filesystem to match the new device size.
// preExpandDeviceSizeBytes/preExpandFilesystemSize must have been captured by the caller via
// capturePreExpandSizeBaseline before any rescan, so the growth check below has an accurate
// "before" baseline.
func (c *Core) expandFilesystemAndLUKS(
	ctx context.Context, volume string, publishInfo *models.VolumePublishInfo, requiredBytes int64,
	secrets map[string]string, preExpandDeviceSizeBytes, preExpandFilesystemSize int64,
) error {
	fsType, err := filesystem.VerifyFilesystemSupport(publishInfo.FilesystemType)
	if err != nil {
		return err
	}

	stagingTargetPath := publishInfo.GlobalMount
	mountOptions := publishInfo.MountOptions

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		if !luks.IsLegacyDevicePath(devicePath) {
			devicePath, err = c.dev.GetLUKSDeviceForMultipathDevice(devicePath)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"volumeId":      volume,
					"publishedPath": publishInfo.DevicePath,
				}).WithError(err).Error("Failed to get LUKS device path from device path.")
				return err
			}
		}
		Logc(ctx).WithField("volumeId", volume).Info("Resizing the LUKS mapping.")

		passphrase, ok := secrets["luks-passphrase"]
		if !ok {
			return errors.InvalidInputError("cannot expand LUKS encrypted volume; no passphrase provided")
		} else if passphrase == "" {
			return errors.InvalidInputError("cannot expand LUKS encrypted volume; empty passphrase provided")
		}

		luksDevice := luks.NewDetailed("", filepath.Base(devicePath), c.cmd, c.dev, afero.NewOsFs())
		if err = luksDevice.Resize(ctx, passphrase); err != nil {
			if errors.IsIncorrectLUKSPassphraseError(err) {
				return errors.InvalidInputError(err.Error())
			}
			return errors.InternalError("failed to resize LUKS mapping for volume %s: %v", volume, err)
		}
	}

	var postExpandDeviceSizeBytes int64
	if publishInfo.DevicePath != "" {
		postExpandDeviceSizeBytes, err = c.dev.GetDiskSize(ctx, publishInfo.DevicePath)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": publishInfo.DevicePath,
			}).WithError(err).Warn("Failed to read post-expand device size; skipping device growth check.")
		}
	}
	devicesGrew := preExpandDeviceSizeBytes > 0 && postExpandDeviceSizeBytes > preExpandDeviceSizeBytes

	if fsType == filesystem.Raw {
		Logc(ctx).WithField("volumeId", volume).Info("Filesystem expansion completed.")
		return nil
	}

	newFilesystemSize, err := c.fs.ExpandFilesystemOnNode(
		ctx, publishInfo, devicePath, stagingTargetPath, fsType, mountOptions, requiredBytes)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"device":         publishInfo.DevicePath,
			"filesystemType": fsType,
		}).WithError(err).Error("Unable to expand filesystem.")
		return errors.InternalError("failed to expand filesystem for volume %s: %v", volume, err)
	}

	if devicesGrew && newFilesystemSize <= preExpandFilesystemSize {
		Logc(ctx).WithFields(LogFields{
			"preExpandFilesystemSize":   preExpandFilesystemSize,
			"newFilesystemSize":         newFilesystemSize,
			"preExpandDeviceSizeBytes":  preExpandDeviceSizeBytes,
			"postExpandDeviceSizeBytes": postExpandDeviceSizeBytes,
			"requiredBytes":             requiredBytes,
		}).Error("Filesystem did not grow despite block device growing during expand.")
		return errors.InternalError("filesystem size did not grow")
	}

	Logc(ctx).WithFields(LogFields{
		"filesystemSize": newFilesystemSize,
		"requiredBytes":  requiredBytes,
	}).Debug("Filesystem size after expand.")
	Logc(ctx).WithField("volumeId", volume).Info("Filesystem expansion completed.")
	return nil
}
