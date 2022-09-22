// Copyright 2022 NetApp, Inc. All Rights Reserved.
package utils

import (
	"fmt"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

func IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_darwin.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsLikelyNotMountPoint")
	return false, UnsupportedError("IsLikelyNotMountPoint is not supported on non-linux platform")
}

func IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_darwin.IsMounted")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsMounted")
	return false, UnsupportedError("IsMounted is not supported on non-linux platform")
}

// mountNFSPath is a dummy added for compilation on non-linux platform.
func mountNFSPath(ctx context.Context, exportPath, mountpoint, options string) error {
	Logc(ctx).Debug(">>>> mount_darwin.mountNFSPath")
	defer Logc(ctx).Debug("<<<< mount_darwin.mountNFSPath")
	return UnsupportedError("mountNFSPath is not supported on non-linux platform")
}

// mountSMBPath is a dummy added for compilation on non-windows platform.
func mountSMBPath(ctx context.Context, exportPath, mountpoint, username, password string) error {
	Logc(ctx).Debug(">>>> mount_darwin.mountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_darwin.mountSMBPath")
	return UnsupportedError("mountSMBPath is not supported on non-windows platform")
}

// UmountSMBPath is a dummy added for compilation on non-windows platform.
func UmountSMBPath(ctx context.Context, mappingPath, target string) error {
	Logc(ctx).Debug(">>>> mount_darwin.UmountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_darwin.UmountSMBPath")
	return UnsupportedError("UmountSMBPath is not supported on non-windows platform")
}

// WindowsBindMount is a dummy added for compilation on non-windows platform.
func WindowsBindMount(ctx context.Context, source, target string, options []string) error {
	Logc(ctx).Debug(">>>> mount_darwin.WindowsBindMount")
	defer Logc(ctx).Debug("<<<< mount_darwin.WindowsBindMount")
	return UnsupportedError("WindowsBindMount is not supported on non-windows platform")
}

// IsNFSShareMounted is a dummy added for compilation on non-linux platform.
func IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_darwin.IsNFSShareMounted")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsNFSShareMounted")
	return false, UnsupportedError("IsNFSShareMounted is not supported on non-linux platform")
}

// MountDevice is a dummy added for compilation on non-linux platform.
func MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error) {
	Logc(ctx).Debug(">>>> mount_darwin.MountDevice")
	defer Logc(ctx).Debug("<<<< mount_darwin.MountDevice")
	return UnsupportedError("MountDevice is not supported on non-linux platform")
}

// Umount is a dummy added for compilation on darwin.
func Umount(ctx context.Context, mountpoint string) (err error) {
	Logc(ctx).Debug(">>>> mount_darwin.Umount")
	defer Logc(ctx).Debug("<<<< mount_darwin.Umount")
	return UnsupportedError("Umount is not supported on darwin")
}

// RemoveMountPoint is a dummy added for compilation on non-linux platform.
func RemoveMountPoint(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount_darwin.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.RemoveMountPoint")
	return UnsupportedError("RemoveMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveTemporaryMountPoint is a dummy added for compilation on non-linux platform.
func UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount_darwin.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.UmountAndRemoveTemporaryMountPoint")
	return UnsupportedError("UmountAndRemoveTemporaryMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveMountPoint is a dummy added for compilation on non-linux platform.
func UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount_darwin.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.UmountAndRemoveMountPoint")
	return UnsupportedError("UmountAndRemoveMountPoint is not supported on non-linux platform")
}

// RemountDevice is a dummy added for compilation on non-linux platform.
func RemountDevice(ctx context.Context, mountpoint, options string) (err error) {
	Logc(ctx).Debug(">>>> mount_darwin.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount_darwin.RemountDevice")
	return UnsupportedError("RemountDevice is not supported on non-linux platform")
}

// GetMountInfo is a dummy added for compilation on non-linux platform.
func GetMountInfo(ctx context.Context) ([]MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_darwin.GetMountInfo")
	defer Logc(ctx).Debug("<<<< mount_darwin.GetMountInfo")
	return nil, UnsupportedError("GetMountInfo is not supported on non-linux platform")
}

// IsCompatible checks for compatibility of protocol and platform
func IsCompatible(ctx context.Context, protocol string) error {
	Logc(ctx).Debug(">>>> mount_darwin.IsCompatible")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsCompatible")

	return fmt.Errorf("mounting %s volume is not supported on darwin", protocol)
}
