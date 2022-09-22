// Copyright 2022 NetApp, Inc. All Rights Reserved.
package utils

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	sa "github.com/netapp/trident/storage_attribute"
)

func GetFilesystemStats(ctx context.Context, path string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> mount_windows.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< mount_windows.GetFilesystemStats")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).Errorf("Failed to instantiate CSI proxy clients. Error: %v", err)
	}
	total, used, err := csiproxy.GetFilesystemUsage(ctx, path)
	if err != nil {
		Logc(ctx).Errorf("Failed to instantiate CSI proxy clients. Error: %v", err)
	}
	Logc(ctx).Debugf("Total fs capacity: %d used: %d", total, used)
	return total - used, total, used, 0, 0, 0, nil
}

// IsMounted verifies if the supplied device is attached at the supplied location.
func IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsMounted")
	defer Logc(ctx).Debug("<<<< mount_windows.IsMounted")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to instantiate CSI proxy clients", err)
	}
	return csiproxy.ExistsPath(ctx, mountpoint)
}

// Umount to remove symlinks
func Umount(ctx context.Context, mountpoint string) error {
	Logc(ctx).Debug(">>>> mount_windows.Umount")
	defer Logc(ctx).Debug("<<<< mount_windows.Umount")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to instantiate CSI proxy clients", err)
	}
	return csiproxy.Rmdir(ctx, mountpoint)
}

// IsLikelyNotMountPoint  unused stub function
func IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.IsLikelyNotMountPoint")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to instantiate CSI proxy clients", err)
	}
	return csiproxy.IsLikelyNotMountPoint(ctx, mountpoint)
}

// IsLikelyDir determines if mountpoint is a directory
func IsLikelyDir(mountpoint string) (bool, error) {
	return PathExists(mountpoint)
}

// mountNFSPath is a dummy added for compilation on non-linux platform.
func mountNFSPath(ctx context.Context, exportPath, mountpoint, options string) error {
	Logc(ctx).Debug(">>>> mount_windows.mountNFSPath")
	defer Logc(ctx).Debug("<<<< mount_windows.mountNFSPath")
	return UnsupportedError("mountNFSPath is not supported on non-linux platform")
}

// mountSMBPath attaches the supplied SMB share at the supplied location with options.
func mountSMBPath(ctx context.Context, exportPath, mountpoint, username, password string) (err error) {
	Logc(ctx).WithFields(log.Fields{
		"exportPath": exportPath,
		"mountpoint": mountpoint,
	}).Debug(">>>> mount_windows.mountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_windows.mountSMBPath")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to instantiate CSI proxy clients", err)
	}
	isExists, err := csiproxy.ExistsPath(ctx, mountpoint)
	if err != nil {
		return err
	}

	if isExists {
		Logc(ctx).Infof("Removing path: %s", mountpoint)
		if err = csiproxy.Rmdir(ctx, mountpoint); err != nil {
			return err
		}
	}
	if err := csiproxy.SMBMount(ctx, exportPath, mountpoint, SMB, username, password); err != nil {
		return fmt.Errorf("error mounting SMB volume %v on mountpoint %v: %v", exportPath, mountpoint, err)
	}

	return nil
}

func UmountSMBPath(ctx context.Context, mappingPath, path string) (err error) {
	Logc(ctx).WithFields(log.Fields{
		"path": path,
	}).Debug(">>>> mount_windows.UmountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_windows.UmountSMBPath")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to instantiate CSI proxy clients", err)
	}
	if err := csiproxy.SMBUnmount(ctx, mappingPath, path); err != nil {
		return fmt.Errorf("error unmounting SMB volume %v on mountpoint %v", path, err)
	}

	return nil
}

// Handles local bind mount (I.E. Symlink)
func WindowsBindMount(ctx context.Context, source, target string, options []string) (err error) {
	Logc(ctx).Debug(">>>> mount_windows.WindowsBindMount")
	defer Logc(ctx).Debug("<<<< mount_windows.WindowsBindMount")
	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to instantiate CSI proxy clients", err)
	}
	if err = csiproxy.Rmdir(ctx, target); err != nil {
		return fmt.Errorf("prepare publish failed for %s with error: %v", target, err)
	}
	if err := csiproxy.Mount(ctx, source, target, SMB, options); err != nil {
		return fmt.Errorf("error mounting SMB volume %v on mountpoint %v: %v", source, target, err)
	}

	return nil
}

// IsNFSShareMounted is a dummy added for compilation on non-linux platform.
func IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsNFSShareMounted")
	defer Logc(ctx).Debug("<<<< mount_windows.IsNFSShareMounted")
	return false, UnsupportedError("IsNFSShareMounted is not supported on non-linux platform")
}

// MountDevice is a dummy added for compilation on non-linux platform.
func MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error) {
	Logc(ctx).Debug(">>>> mount_windows.MountDevice")
	defer Logc(ctx).Debug("<<<< mount_windows.MountDevice")
	return UnsupportedError("MountDevice is not supported on non-linux platform")
}

// RemoveMountPoint is a dummy added for compilation on non-linux platform.
func RemoveMountPoint(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount_windows.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.RemoveMountPoint")
	return UnsupportedError("RemoveMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveTemporaryMountPoint is a dummy added for compilation on non-linux platform.
func UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount_windows.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.UmountAndRemoveTemporaryMountPoint")
	return UnsupportedError("UmountAndRemoveTemporaryMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveMountPoint is a dummy added for compilation on non-linux platform.
func UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount_windows.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.UmountAndRemoveMountPoint")
	return UnsupportedError("UmountAndRemoveMountPoint is not supported on non-linux platform")
}

// RemountDevice is a dummy added for compilation on non-linux platform.
func RemountDevice(ctx context.Context, mountpoint, options string) (err error) {
	Logc(ctx).Debug(">>>> mount_windows.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount_windows.RemountDevice")
	return UnsupportedError("RemountDevice is not supported on non-linux platform")
}

// GetMountInfo is a dummy added for compilation on non-linux platform.
func GetMountInfo(ctx context.Context) ([]MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_windows.GetMountInfo")
	defer Logc(ctx).Debug("<<<< mount_windows.GetMountInfo")
	return nil, UnsupportedError("GetMountInfo is not supported on non-linux platform")
}

// IsCompatible checks for compatibility of protocol and platform
func IsCompatible(ctx context.Context, protocol string) error {
	Logc(ctx).Debug(">>>> mount_windows.IsCompatible")
	defer Logc(ctx).Debug("<<<< mount_windows.IsCompatible")

	if protocol != sa.SMB {
		return fmt.Errorf("mounting %s volume is not supported on windows", protocol)
	} else {
		return nil
	}
}
