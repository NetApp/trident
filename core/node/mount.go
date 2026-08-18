// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"os"
	"strings"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

const snswAlreadyPublishedElsewhereMsg = "volume uses SINGLE_NODE_SINGLE_WRITER access mode and is already mounted at a different target path"

// MountRequest holds volume-specific publish options (CSI NodePublishVolume).
type MountRequest struct {
	TargetPath             string
	ReadOnly               bool
	Secrets                map[string]string
	SingleNodeSingleWriter bool
}

func (c *Core) Mount(ctx context.Context, volumeID string, req MountRequest) error {
	targetPath := req.TargetPath
	if volumeID == "" {
		return errors.InvalidInputError("volume is empty")
	}
	if targetPath == "" {
		return errors.InvalidInputError("target path is empty")
	}

	fields := LogFields{
		"Method":     "Mount",
		"Type":       "Node_Core",
		"Volume":     volumeID,
		"TargetPath": targetPath,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Mount")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Mount")

	if err := c.checkReady(); err != nil {
		return err
	}
	release, err := c.acquireVolumeLock(ctx, volumeID)
	if err != nil {
		return err
	}
	defer release()

	trackingInfo, err := c.localStore.ReadTrackingInfo(ctx, volumeID)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return errors.PreconditionError("volume %s is not staged: %v", volumeID, err)
		}
		return err
	}
	if req.SingleNodeSingleWriter && isVolumePublishedElsewhere(trackingInfo, targetPath) {
		return errors.PreconditionError(snswAlreadyPublishedElsewhereMsg)
	}

	publishInfo := &trackingInfo.VolumePublishInfo

	if req.ReadOnly {
		publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}
	protocol := publishInfo.GetStorageProtocol()
	fields["Protocol"] = protocol

	switch protocol {
	case NFS:
		return c.mountNFSVolume(ctx, volumeID, targetPath, publishInfo)
	case SMB:
		return c.mountSMBVolume(ctx, volumeID, targetPath, publishInfo)
	case FCP:
		return c.mountFCPVolume(ctx, volumeID, targetPath, publishInfo, req.Secrets)
	case ISCSI:
		return c.mountISCSIVolume(ctx, volumeID, targetPath, publishInfo, req.Secrets)
	case NVMe:
		return c.mountNVMeVolume(ctx, volumeID, targetPath, publishInfo, req.Secrets)
	default:
		return errors.PreconditionError("unknown storage protocol")
	}
}

func (c *Core) mountNFSVolume(
	ctx context.Context, volumeID, targetPath string, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> mountNFSVolume")
	defer Logc(ctx).Debug("<<<< mountNFSVolume")

	release, err := c.acquireLimiter(ctx, mountNFSVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	notMnt, err := c.mount.IsLikelyNotMountPoint(ctx, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(targetPath, 0o750); mkErr != nil {
				return mkErr
			}
			notMnt = true
		} else {
			return err
		}
	}
	if !notMnt {
		return nil
	}

	if err = c.mount.AttachNFSVolume(ctx, publishInfo.InternalID, targetPath, publishInfo); err != nil {
		if os.IsPermission(err) {
			return errors.PermissionDeniedError("authentication issue when attaching NFS volume %s: %v", volumeID, err)
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return errors.InvalidInputError(err.Error())
		}
		return errors.InternalError("failed to attach NFS volume %s: %v", volumeID, err)
	}

	return c.localStore.AddPublishedPath(ctx, volumeID, targetPath)
}

func (c *Core) mountSMBVolume(
	ctx context.Context, volumeID, targetPath string, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> mountSMBVolume")
	defer Logc(ctx).Debug("<<<< mountSMBVolume")

	release, err := c.acquireLimiter(ctx, mountSMBVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	source := publishInfo.GlobalMount
	if source == "" {
		return errors.InternalError("staging target not available for volume")
	}

	notMnt, err := c.mount.IsLikelyNotMountPoint(ctx, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(targetPath, 0o750); mkErr != nil {
				return mkErr
			}
			notMnt = true
		} else {
			return err
		}
	}
	if !notMnt {
		return nil
	}

	mountOptions := []string{"bind"}
	if strings.Contains(publishInfo.MountOptions, "ro") {
		mountOptions = append(mountOptions, "ro")
	}

	if err = c.mount.WindowsBindMount(ctx, source, targetPath, mountOptions); err != nil {
		return err
	}

	return c.localStore.AddPublishedPath(ctx, volumeID, targetPath)
}

func (c *Core) mountFCPVolume(
	ctx context.Context, volumeID, targetPath string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> mountFCPVolume")
	defer Logc(ctx).Debug("<<<< mountFCPVolume")

	release, err := c.acquireLimiter(ctx, mountFCPVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		var luksDevice luks.Device
		if luks.IsLegacyDevicePath(devicePath) {
			luksDevice, err = luks.NewDeviceFromMappingPath(ctx, c.cmd, c.dev, devicePath, publishInfo.InternalID)
			if err != nil {
				return err
			}
		} else {
			luksDevice = luks.NewDevice(devicePath, publishInfo.InternalID, c.cmd, c.dev)
		}

		if err = ensureLUKSVolumePassphrase(ctx, luksDevice, volumeID, secrets, false); err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}

		devicePath = luksDevice.MappedDevicePath()
	}

	return c.mountDeviceAtTargetPath(ctx, volumeID, targetPath, devicePath, publishInfo)
}

func (c *Core) mountNVMeVolume(
	ctx context.Context, volumeID, targetPath string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> mountNVMeVolume")
	defer Logc(ctx).Debug("<<<< mountNVMeVolume")

	release, err := c.acquireLimiter(ctx, mountNVMeVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		luksDevice := luks.NewDevice(devicePath, publishInfo.InternalID, c.cmd, c.dev)

		if err := ensureLUKSVolumePassphrase(ctx, luksDevice, volumeID, secrets, false); err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}

		devicePath = luksDevice.MappedDevicePath()
	}

	return c.mountDeviceAtTargetPath(ctx, volumeID, targetPath, devicePath, publishInfo)
}

// mountDeviceAtTargetPath mounts a raw block device at targetPath, shared by the FCP and NVMe
// stopgap publish paths.
func (c *Core) mountDeviceAtTargetPath(
	ctx context.Context, volumeID, targetPath, devicePath string, publishInfo *models.VolumePublishInfo,
) error {
	isRawBlock := publishInfo.FilesystemType == filesystem.Raw
	if isRawBlock {
		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}
		if err := c.mount.MountDevice(ctx, devicePath, targetPath, publishInfo.MountOptions, true); err != nil {
			return errors.InternalError("unable to bind mount raw device; %v", err)
		}
	} else {
		if err := c.mount.MountDevice(ctx, devicePath, targetPath, publishInfo.MountOptions, false); err != nil {
			return errors.InternalError("unable to mount device; %v", err)
		}
	}

	return c.localStore.AddPublishedPath(ctx, volumeID, targetPath)
}

func (c *Core) mountISCSIVolume(
	ctx context.Context, volumeID, targetPath string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> mountISCSIVolume")
	defer Logc(ctx).Debug("<<<< mountISCSIVolume")

	release, err := c.acquireLimiter(ctx, mountISCSIVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		// Rotate the LUKS passphrase if needed; on failure, log and continue to publish.
		var luksDevice luks.Device
		if luks.IsLegacyDevicePath(devicePath) {
			// Supports legacy volumes that store the LUKS device path.
			luksDevice, err = luks.NewDeviceFromMappingPath(ctx, c.cmd, c.dev, devicePath, publishInfo.InternalID)
			if err != nil {
				return err
			}
		} else {
			luksDevice = luks.NewDevice(publishInfo.DevicePath, publishInfo.InternalID, c.cmd, c.dev)
		}

		// Secrets come from the Mount() caller (CSI's req.GetSecrets()), not publishInfo.Secrets,
		// which is marked json:"-" and never survives a round trip through the tracking file.
		if err = ensureLUKSVolumePassphrase(ctx, luksDevice, volumeID, secrets, false); err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}

		// Mount the LUKS device instead of the multipath device.
		devicePath = luksDevice.MappedDevicePath()
	}

	return c.mountDeviceAtTargetPath(ctx, volumeID, targetPath, devicePath, publishInfo)
}

func isVolumePublishedElsewhere(trackingInfo *models.VolumeTrackingInfo, targetPath string) bool {
	if _, ok := trackingInfo.PublishedPaths[targetPath]; ok {
		return false
	}
	return len(trackingInfo.PublishedPaths) > 0
}
