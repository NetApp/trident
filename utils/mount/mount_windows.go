// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fs "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1"
	smb "github.com/kubernetes-csi/csi-proxy/client/api/smb/v1"
	fsclient "github.com/kubernetes-csi/csi-proxy/client/groups/filesystem/v1"
	smbclient "github.com/kubernetes-csi/csi-proxy/client/groups/smb/v1"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type WindowsClient struct {
	fsClient  FilesystemClient
	smbClient SmbClient
}

var _ mountOS = &WindowsClient{}

func newOsSpecificClient() (*WindowsClient, error) {
	fsClient, err := fsclient.NewClient()
	if err != nil {
		return nil, err
	}

	smbClient, err := smbclient.NewClient()
	if err != nil {
		return nil, err
	}

	return newOsSpecificClientDetailed(fsClient, smbClient), nil
}

func newOsSpecificClientDetailed(fsClient FilesystemClient, smbClient SmbClient) *WindowsClient {
	return &WindowsClient{
		fsClient:  fsClient,
		smbClient: smbClient,
	}
}

// IsMounted verifies if the supplied device is attached at the supplied location.
func (c *WindowsClient) IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsMounted")
	defer Logc(ctx).Debug("<<<< mount_windows.IsMounted")

	return c.existsPath(ctx, mountpoint)
}

// Umount to remove symlinks
func (c *WindowsClient) Umount(ctx context.Context, mountpoint string) error {
	Logc(ctx).Debug(">>>> mount_windows.Umount")
	defer Logc(ctx).Debug("<<<< mount_windows.Umount")

	return c.rmdir(ctx, mountpoint)
}

// IsLikelyNotMountPoint  unused stub function
func (c *WindowsClient) IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.IsLikelyNotMountPoint")

	exists, err := c.existsPath(ctx, mountpoint)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, os.ErrNotExist
	}

	isSymlink, err := c.isSymlink(ctx, mountpoint)
	if err != nil {
		return false, err
	}

	return !isSymlink, nil
}

func (c *WindowsClient) isSymlink(ctx context.Context, mountpoint string) (bool, error) {
	response, err := c.fsClient.IsSymlink(ctx, &fs.IsSymlinkRequest{Path: normalizeWindowsPath(mountpoint)})
	if err != nil {
		return false, err
	}

	return response.IsSymlink, nil
}

// MountNFSPath is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) MountNFSPath(ctx context.Context, exportPath, mountpoint, options string) error {
	Logc(ctx).Debug(">>>> mount_windows.mountNFSPath")
	defer Logc(ctx).Debug("<<<< mount_windows.mountNFSPath")
	return errors.UnsupportedError("mountNFSPath is not supported on non-linux platform")
}

// MountSMBPath attaches the supplied SMB share at the supplied location with options.
func (c *WindowsClient) MountSMBPath(ctx context.Context, exportPath, mountPoint, username, password string) error {
	Logc(ctx).WithFields(LogFields{
		"exportPath": exportPath,
		"mountpoint": mountPoint,
	}).Debug(">>>> mount_windows.mountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_windows.mountSMBPath")

	exists, err := c.existsPath(ctx, mountPoint)
	if err != nil {
		return err
	}

	if exists {
		Logc(ctx).Infof("Removing path: %s", mountPoint)
		if err = c.rmdir(ctx, mountPoint); err != nil {
			return err
		}
	}

	if len(username) == 0 || len(password) == 0 {
		return errors.New("empty username or password is not allowed")
	}

	targetParentDir := filepath.Dir(mountPoint)
	parentExists, err := c.existsPath(ctx, targetParentDir)
	if err != nil {
		return fmt.Errorf("parent dir: %s exist check failed with err: %v", targetParentDir, err)
	}

	if !parentExists {
		Logc(ctx).WithFields(LogFields{
			"parent directory": targetParentDir,
		}).Info("Parent directory does not exists. Creating the directory")

		if err = c.makeDir(ctx, targetParentDir); err != nil {
			return fmt.Errorf("create of parent dir: %s failed with error: %v", targetParentDir, err)
		}
	}

	if err = c.NewSmbGlobalMapping(ctx, exportPath, mountPoint, username, password); err != nil {
		return fmt.Errorf("smb mapping failed with error: %v", err)
	}

	return nil
}

func (c *WindowsClient) UmountSMBPath(ctx context.Context, mappingPath, path string) error {
	Logc(ctx).WithFields(LogFields{
		"path": path,
	}).Debug(">>>> mount_windows.UmountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_windows.UmountSMBPath")

	if err := c.rmdir(ctx, path); err != nil {
		return fmt.Errorf("failed to remove smb mount directory; %v", err)
	}

	if err := c.removeSmbGlobalMapping(ctx, mappingPath); err != nil {
		return fmt.Errorf("remove smb global mapping failed with error: %v", err)
	}

	return nil
}

// WindowsBindMount Handles local bind mount (I.E. Symlink)
func (c *WindowsClient) WindowsBindMount(ctx context.Context, source, target string, options []string) error {
	Logc(ctx).Debug(">>>> mount_windows.WindowsBindMount")
	defer Logc(ctx).Debug("<<<< mount_windows.WindowsBindMount")

	if err := c.rmdir(ctx, target); err != nil {
		return fmt.Errorf("prepare publish failed for %s with error: %v", target, err)
	}

	if err := c.Mount(ctx, source, target, sa.SMB, options); err != nil {
		return fmt.Errorf("error mounting SMB volume %v on mountpoint %v: %v", source, target, err)
	}

	return nil
}

// Mount just creates a soft link at target pointing to source.
func (c *WindowsClient) Mount(ctx context.Context, source, target, fsType string, options []string) error {
	Logc(ctx).WithFields(LogFields{
		"old name": source,
		"new name": target,
	}).Debug(">>>> mount_windows.Mount")
	defer Logc(ctx).Debug("<<<< mount_windows.Mount")

	// Mount is called after the format is done.
	// TODO: Confirm that fsType is empty.
	return c.createSymlink(ctx, source, target)
}

// IsNFSShareMounted is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsNFSShareMounted")
	defer Logc(ctx).Debug("<<<< mount_windows.IsNFSShareMounted")
	return false, errors.UnsupportedError("IsNFSShareMounted is not supported on non-linux platform")
}

// MountDevice is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) MountDevice(ctx context.Context, device, mountpoint, options string,
	isMountPointFile bool,
) error {
	Logc(ctx).Debug(">>>> mount_windows.MountDevice")
	defer Logc(ctx).Debug("<<<< mount_windows.MountDevice")
	return errors.UnsupportedError("MountDevice is not supported on non-linux platform")
}

// RemoveMountPoint is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) RemoveMountPoint(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount_windows.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.RemoveMountPoint")
	return errors.UnsupportedError("RemoveMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveTemporaryMountPoint is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount_windows.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.UmountAndRemoveTemporaryMountPoint")
	return errors.UnsupportedError("UmountAndRemoveTemporaryMountPoint is not supported on non-linux platform")
}

// UmountAndRemoveMountPoint is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount_windows.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.UmountAndRemoveMountPoint")
	return errors.UnsupportedError("UmountAndRemoveMountPoint is not supported on non-linux platform")
}

// RemountDevice is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) RemountDevice(ctx context.Context, mountpoint, options string) error {
	Logc(ctx).Debug(">>>> mount_windows.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount_windows.RemountDevice")
	return errors.UnsupportedError("RemountDevice is not supported on non-linux platform")
}

// GetSelfMountInfo is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) GetSelfMountInfo(ctx context.Context) ([]models.MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_darwin.GetSelfMountInfo")
	defer Logc(ctx).Debug("<<<< mount_darwin.GetSelfMountInfo")
	return nil, errors.UnsupportedError("GetSelfMountInfo is not supported on non-linux platform")
}

// GetHostMountInfo is a dummy added for compilation on non-linux platform.
func (c *WindowsClient) GetHostMountInfo(ctx context.Context) ([]models.MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_darwin.GetHostMountInfo")
	defer Logc(ctx).Debug("<<<< mount_darwin.GetHostMountInfo")
	return nil, errors.UnsupportedError("GetHostMountInfo is not supported on non-linux platform")
}

// IsCompatible checks for compatibility of protocol and platform
func (c *WindowsClient) IsCompatible(ctx context.Context, protocol string) error {
	Logc(ctx).Debug(">>>> mount_windows.IsCompatible")
	defer Logc(ctx).Debug("<<<< mount_windows.IsCompatible")

	if protocol != sa.SMB {
		return fmt.Errorf("mounting %s volume is not supported on windows", protocol)
	}

	return nil
}

// PVMountpointMappings identifies devices corresponding to published paths
func (c *WindowsClient) PVMountpointMappings(ctx context.Context) (map[string]string, error) {
	Logc(ctx).Debug(">>>> mount_windows.PVMountpointMappings")
	defer Logc(ctx).Debug("<<<< mount_windows.PVMountpointMappings")
	return make(map[string]string), errors.UnsupportedError("PVMountpointMappings is not supported on windows")
}

func (c *WindowsClient) ListProcMounts(mountFilePath string) ([]models.MountPoint, error) {
	Log().Debug(">>>> mount_windows.ListProcMounts")
	defer Log().Debug("<<<< mount_windows.ListProcMounts")
	return nil, errors.UnsupportedError("ListProcMounts is not supported on non-linux platform")
}

func (c *WindowsClient) ListProcMountinfo() ([]models.MountInfo, error) {
	Log().Debug(">>>> mount_windows.ListProcMountinfo")
	defer Log().Debug("<<<< mount_windows.ListProcMountinfo")
	return nil, errors.UnsupportedError("ListProcMountinfo is not supported on non-linux platform")
}

// existsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (c *WindowsClient) existsPath(ctx context.Context, path string) (bool, error) {
	isExistsResponse, err := c.fsClient.PathExists(ctx, &fs.PathExistsRequest{Path: normalizeWindowsPath(path)})
	if err != nil {
		return false, err
	}

	return isExistsResponse.Exists, err
}

// makeDir - Creates a directory. The CSI proxy takes in context information.
// Currently, the make dir is only used from the staging code path, hence we call it
// with Plugin context..
func (c *WindowsClient) makeDir(ctx context.Context, path string) error {
	Logc(ctx).WithFields(LogFields{
		"make directory": path,
	}).Debug(">>>> mount_windows.MakeDir")
	defer Logc(ctx).Debug("<<<< mount_windows.MakeDir")

	mkdirReq := &fs.MkdirRequest{
		Path: normalizeWindowsPath(path),
	}

	_, err := c.fsClient.Mkdir(ctx, mkdirReq)
	if err != nil {
		return err
	}

	return nil
}

func (c *WindowsClient) rmdir(ctx context.Context, path string) error {
	Logc(ctx).WithFields(LogFields{
		"remove directory": path,
	}).Debug(">>>> mount_windows.Rmdir")
	defer Logc(ctx).Debug("<<<< mount_windows.Rmdir")

	rmdirRequest := &fs.RmdirRequest{
		Path:  normalizeWindowsPath(path),
		Force: true,
	}
	_, err := c.fsClient.Rmdir(ctx, rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

func (c *WindowsClient) NewSmbGlobalMapping(ctx context.Context, source, target, username, password string) error {
	source = strings.Replace(source, "/", "\\", -1)

	smbMountRequest := &smb.NewSmbGlobalMappingRequest{
		LocalPath:  normalizeWindowsPath(target),
		RemotePath: source,
		Username:   username,
		Password:   password,
	}

	_, err := c.smbClient.NewSmbGlobalMapping(ctx, smbMountRequest)
	return err
}

// RemoveSmbGlobalMapping remove the smb mapping on the host based on volume id
func (c *WindowsClient) removeSmbGlobalMapping(ctx context.Context, mappingPath string) error {
	removeSmbMappingRequest := &smb.RemoveSmbGlobalMappingRequest{
		RemotePath: mappingPath,
	}

	_, err := c.smbClient.RemoveSmbGlobalMapping(ctx, removeSmbMappingRequest)
	return err
}

func (c *WindowsClient) createSymlink(ctx context.Context, source, target string) error {
	linkRequest := &fs.CreateSymlinkRequest{
		SourcePath: normalizeWindowsPath(source),
		TargetPath: normalizeWindowsPath(target),
	}

	_, err := c.fsClient.CreateSymlink(ctx, linkRequest)
	return err
}
