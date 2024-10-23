// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"fmt"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type DarwinClient struct{}

var _ mountOS = &DarwinClient{}

func newOsSpecificClient() (*DarwinClient, error) {
	return &DarwinClient{}, nil
}

func (client *DarwinClient) IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_darwin.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsLikelyNotMountPoint")
	return false, errors.UnsupportedError("IsLikelyNotMountPoint is not supported on non-linux platform")
}

func (client *DarwinClient) IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool,
	error,
) {
	Logc(ctx).Debug(">>>> mount_darwin.IsMounted")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsMounted")
	return false, errors.UnsupportedError("IsMounted is not supported on non-linux platform")
}

// MountNFSPath is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) MountNFSPath(ctx context.Context, exportPath, mountpoint, options string) error {
	Logc(ctx).Debug(">>>> mount_darwin.MountNFSPath")
	defer Logc(ctx).Debug("<<<< mount_darwin.MountNFSPath")
	return errors.UnsupportedError("MountNFSPath is not supported on non-linux platform")
}

// MountSMBPath is a dummy added for compilation on non-windows platform.
func (client *DarwinClient) MountSMBPath(ctx context.Context, exportPath, mountpoint, username, password string) error {
	Logc(ctx).Debug(">>>> mount_darwin.MountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_darwin.MountSMBPath")
	return errors.UnsupportedError("MountSMBPath is not supported on non-windows platform")
}

// UmountSMBPath is a dummy added for compilation on non-windows platform.
func (client *DarwinClient) UmountSMBPath(ctx context.Context, mappingPath, target string) error {
	Logc(ctx).Debug(">>>> mount_darwin.UmountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_darwin.UmountSMBPath")
	return errors.UnsupportedError("UmountSMBPath is not supported on non-windows platform")
}

// WindowsBindMount is a dummy added for compilation on non-windows platform.
func (client *DarwinClient) WindowsBindMount(ctx context.Context, source, target string, options []string) error {
	Logc(ctx).Debug(">>>> mount_darwin.WindowsBindMount")
	defer Logc(ctx).Debug("<<<< mount_darwin.WindowsBindMount")
	return errors.UnsupportedError("WindowsBindMount is not supported on non-windows platform")
}

// IsNFSShareMounted is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_darwin.IsNFSShareMounted")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsNFSShareMounted")
	return false, errors.UnsupportedError("IsNFSShareMounted is not supported on non-linux platform")
}

// MountDevice is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) MountDevice(ctx context.Context, device, mountpoint, options string,
	isMountPointFile bool,
) (err error) {
	Logc(ctx).Debug(">>>> mount_darwin.MountDevice")
	defer Logc(ctx).Debug("<<<< mount_darwin.MountDevice")
	return errors.UnsupportedError("MountDevice is not supported on non-linux platform")
}

// Umount is a dummy added for compilation on darwin.
func (client *DarwinClient) Umount(ctx context.Context, mountpoint string) (err error) {
	Logc(ctx).Debug(">>>> mount_darwin.Umount")
	defer Logc(ctx).Debug("<<<< mount_darwin.Umount")
	return errors.UnsupportedError("Umount is not supported on darwin")
}

// RemoveMountPoint is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) RemoveMountPoint(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount_darwin.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.RemoveMountPoint")
	return errors.UnsupportedError("RemoveMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveTemporaryMountPoint is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount_darwin.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.UmountAndRemoveTemporaryMountPoint")
	return errors.UnsupportedError("UmountAndRemoveTemporaryMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveMountPoint is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount_darwin.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_darwin.UmountAndRemoveMountPoint")
	return errors.UnsupportedError("UmountAndRemoveMountPoint is not supported on non-linux platform")
}

// RemountDevice is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) RemountDevice(ctx context.Context, mountpoint, options string) (err error) {
	Logc(ctx).Debug(">>>> mount_darwin.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount_darwin.RemountDevice")
	return errors.UnsupportedError("RemountDevice is not supported on non-linux platform")
}

// GetSelfMountInfo is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) GetSelfMountInfo(ctx context.Context) ([]models.MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_darwin.GetSelfMountInfo")
	defer Logc(ctx).Debug("<<<< mount_darwin.GetSelfMountInfo")
	return nil, errors.UnsupportedError("GetSelfMountInfo is not supported on non-linux platform")
}

// GetHostMountInfo is a dummy added for compilation on non-linux platform.
func (client *DarwinClient) GetHostMountInfo(ctx context.Context) ([]models.MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_darwin.GetHostMountInfo")
	defer Logc(ctx).Debug("<<<< mount_darwin.GetHostMountInfo")
	return nil, errors.UnsupportedError("GetHostMountInfo is not supported on non-linux platform")
}

// IsCompatible checks for compatibility of protocol and platform
func (client *DarwinClient) IsCompatible(ctx context.Context, protocol string) error {
	Logc(ctx).Debug(">>>> mount_darwin.IsCompatible")
	defer Logc(ctx).Debug("<<<< mount_darwin.IsCompatible")

	return fmt.Errorf("mounting %s volume is not supported on darwin", protocol)
}

// PVMountpointMappings identifies devices corresponding to published paths
func (client *DarwinClient) PVMountpointMappings(ctx context.Context) (map[string]string, error) {
	Logc(ctx).Debug(">>>> mount_darwin.PVMountpointMappings")
	defer Logc(ctx).Debug("<<<< mount_darwin.PVMountpointMappings")
	return make(map[string]string), errors.UnsupportedError("PVMountpointMappings is not supported on darwin ")
}

func (client *DarwinClient) ListProcMounts(mountFilePath string) ([]models.MountPoint, error) {
	Log().Debug(">>>> mount_darwin.ListProcMounts")
	defer Log().Debug("<<<< mount_darwin.ListProcMounts")

	return nil, errors.UnsupportedError("ListProcMounts is not supported on darwin ")
}

func (client *DarwinClient) ListProcMountinfo() ([]models.MountInfo, error) {
	Log().Debug(">>>> mount_darwin.ListProcMountinfo")
	defer Log().Debug("<<<< mount_darwin.ListProcMountinfo")

	return nil, errors.UnsupportedError("ListProcMounts is not supported on darwin ")
}
