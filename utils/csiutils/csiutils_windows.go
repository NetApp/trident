// Copyright 2022 NetApp, Inc. All Rights Reserved.
// This file contains the code derived from https://github.com/kubernetes-csi/csi-driver-smb/blob/master/pkg/mounter/safe_mounter_windows.go

package csiutils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fs "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1"
	smb "github.com/kubernetes-csi/csi-proxy/client/api/smb/v1"
	volume "github.com/kubernetes-csi/csi-proxy/client/api/volume/v1"
	fsclient "github.com/kubernetes-csi/csi-proxy/client/groups/filesystem/v1"
	smbclient "github.com/kubernetes-csi/csi-proxy/client/groups/smb/v1"
	volumeclient "github.com/kubernetes-csi/csi-proxy/client/groups/volume/v1"
	"k8s.io/mount-utils"

	. "github.com/netapp/trident/logging"
)

var _ CSIProxyUtils = &csiProxyUtils{}

type csiProxyUtils struct {
	FsClient     *fsclient.Client
	SMBClient    *smbclient.Client
	VolumeClient *volumeclient.Client
}

func normalizeWindowsPath(path string) string {
	normalizedPath := strings.Replace(path, "/", "\\", -1)
	if strings.HasPrefix(normalizedPath, "\\") {
		normalizedPath = "c:" + normalizedPath
	}

	return normalizedPath
}

func (mounter *csiProxyUtils) GetFilesystemUsage(ctx context.Context, path string) (capacity, used int64, err error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"path": path,
	}).Debug(">>>> csiutils.GetFilesystemUsage")
	defer Logc(ctx).Debug("<<<< csiutils.GetFilesystemUsage")

	volIdRequest := &volume.GetVolumeIDFromTargetPathRequest{
		TargetPath: normalizeWindowsPath(path),
	}
	volIdResponse, err := mounter.VolumeClient.GetVolumeIDFromTargetPath(ctx, volIdRequest)
	if err != nil {
		return 0, 0, err
	}

	statsRequest := &volume.GetVolumeStatsRequest{
		VolumeId: volIdResponse.VolumeId,
	}
	statsResponse, err := mounter.VolumeClient.GetVolumeStats(ctx, statsRequest)
	if err != nil {
		return 0, 0, err
	}
	return int64(statsResponse.TotalBytes), int64(statsResponse.UsedBytes), nil
}

func (mounter *csiProxyUtils) SMBMount(ctx context.Context, source, target, fsType, username, password string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"remote path": source,
		"local path":  target,
	}).Debug(">>>> csiutils.SMBMount")
	defer Logc(ctx).Debug("<<<< csiutils.SMBMount")

	if len(username) == 0 || len(password) == 0 {
		return errors.New("empty username or password is not allowed")
	}

	parentDir := filepath.Dir(target)
	parentExists, err := mounter.ExistsPath(ctx, parentDir)
	if err != nil {
		return fmt.Errorf("parent dir: %s exist check failed with err: %v", parentDir, err)
	}

	if !parentExists {
		Logc(ctx).WithFields(LogFields{
			"parent directory": parentDir,
		}).Info("Parent directory does not exists. Creating the directory")
		if err := mounter.MakeDir(ctx, parentDir); err != nil {
			return fmt.Errorf("create of parent dir: %s dailed with error: %v", parentDir, err)
		}
	}

	source = strings.Replace(source, "/", "\\", -1)
	smbMountRequest := &smb.NewSmbGlobalMappingRequest{
		LocalPath:  normalizeWindowsPath(target),
		RemotePath: source,
		Username:   username,
		Password:   password,
	}
	if _, err := mounter.SMBClient.NewSmbGlobalMapping(context.Background(), smbMountRequest); err != nil {
		return fmt.Errorf("smb mapping failed with error: %v", err)
	}
	return nil
}

func (mounter *csiProxyUtils) SMBUnmount(ctx context.Context, mappingPath, target string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"local path": target,
	}).Debug(">>>> csiutils.SMBUnmount")
	defer Logc(ctx).Debug("<<<< csiutils.SMBUnmount")

	err := mounter.Rmdir(ctx, target)
	if err != nil {
		return fmt.Errorf("Failed to remove smb mount directory; %v", err)
	}
	// Removing the smb mapping on the host based on volume id
	removeSmbMappingRequest := &smb.RemoveSmbGlobalMappingRequest{
		RemotePath: mappingPath,
	}
	if _, err = mounter.SMBClient.RemoveSmbGlobalMapping(context.Background(), removeSmbMappingRequest); err != nil {
		return fmt.Errorf("remove smb global mapping failed with error: %v", err)
	}
	return nil
}

// Mount just creates a soft link at target pointing to source.
func (mounter *csiProxyUtils) Mount(
	ctx context.Context, source, target, fstype string,
	options []string,
) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"old name": source,
		"new name": target,
	}).Debug(">>>> csiutils.Mount")
	defer Logc(ctx).Debug("<<<< csiutils.Mount")

	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	linkRequest := &fs.CreateSymlinkRequest{
		SourcePath: normalizeWindowsPath(source),
		TargetPath: normalizeWindowsPath(target),
	}
	_, err := mounter.FsClient.CreateSymlink(ctx, linkRequest)
	if err != nil {
		return err
	}
	return nil
}

func Split(r rune) bool {
	return r == ' ' || r == '/'
}

// Currently CSI-Proxy forces us to work with C:\var
// Rmdir - delete the given directory
// TODO: Call separate rmdir for pod context and plugin context. v1alpha1 for CSI
//
//	proxy does a relaxed check for prefix as c:\var\lib\kubelet, so we can do
//	rmdir with either pod or plugin context.
func (mounter *csiProxyUtils) Rmdir(ctx context.Context, path string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"remove directory": path,
	}).Debug(">>>> csiutils.Rmdir")
	defer Logc(ctx).Debug("<<<< csiutils.Rmdir")

	rmdirRequest := &fs.RmdirRequest{
		Path:  normalizeWindowsPath(path),
		Force: true,
	}
	_, err := mounter.FsClient.Rmdir(ctx, rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

// Unmount - Removes the directory - equivalent to unmount on Linux.
func (mounter *csiProxyUtils) Unmount(ctx context.Context, target string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"Unmount": target,
	}).Debug(">>>> csiutils.Unmount")
	defer Logc(ctx).Debug("<<<< csiutils.Unmount")

	return mounter.Rmdir(ctx, target)
}

func (mounter *csiProxyUtils) List(ctx context.Context) ([]mount.MountPoint, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.List")
	defer Logc(ctx).Debug("<<<< csiutils.List")

	return []mount.MountPoint{}, errors.New("List not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) IsMountPointMatch(ctx context.Context, mp mount.MountPoint, dir string) bool {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.IsMountPointMatch")
	defer Logc(ctx).Debug("<<<< csiutils.IsMountPointMatch")

	return mp.Path == dir
}

// IsLikelyMountPoint - If the directory does not exists, the function will return os.ErrNotExist error.
//
//	If the path exists, call to CSI proxy will check if its a link, if its a link then existence of target
//	path is checked.
func (mounter *csiProxyUtils) IsLikelyNotMountPoint(ctx context.Context, path string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"IsLikelyNotMountPoint": path,
	}).Debug(">>>> csiutils.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< csiutils.IsLikelyNotMountPoint")

	isExists, err := mounter.ExistsPath(ctx, path)
	if err != nil {
		return false, err
	}
	if !isExists {
		return true, os.ErrNotExist
	}

	response, err := mounter.FsClient.IsSymlink(ctx,
		&fs.IsSymlinkRequest{
			Path: normalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return !response.IsSymlink, nil
}

func (mounter *csiProxyUtils) PathIsDevice(ctx context.Context, pathname string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.PathIsDevice")
	defer Logc(ctx).Debug("<<<< csiutils.PathIsDevice")

	return false, errors.New("PathIsDevice not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) DeviceOpened(ctx context.Context, pathname string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.DeviceOpened")
	defer Logc(ctx).Debug("<<<< csiutils.DeviceOpened")

	return false, errors.New("DeviceOpened not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) GetDeviceNameFromMount(ctx context.Context, mountPath, pluginMountDir string) (string,
	error,
) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.GetDeviceNameFromMount")
	defer Logc(ctx).Debug("<<<< csiutils.GetDeviceNameFromMount")

	return "", errors.New("GetDeviceNameFromMount not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) MakeRShared(ctx context.Context, path string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.MakeRShared")
	defer Logc(ctx).Debug("<<<< csiutils.MakeRShared")

	return errors.New("MakeRShared not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) MakeFile(ctx context.Context, pathname string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.MakeFile")
	defer Logc(ctx).Debug("<<<< csiutils.MakeFile")

	return errors.New("MakeFile not implemented for CSIProxyUtils")
}

// MakeDir - Creates a directory. The CSI proxy takes in context information.
// Currently the make dir is only used from the staging code path, hence we call it
// with Plugin context..
func (mounter *csiProxyUtils) MakeDir(ctx context.Context, path string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"make directory": path,
	}).Debug(">>>> csiutils.MakeDir")
	defer Logc(ctx).Debug("<<<< csiutils.MakeDir")

	mkdirReq := &fs.MkdirRequest{
		Path: normalizeWindowsPath(path),
	}
	_, err := mounter.FsClient.Mkdir(ctx, mkdirReq)
	if err != nil {
		return err
	}

	return nil
}

// ExistsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (mounter *csiProxyUtils) ExistsPath(ctx context.Context, path string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithFields(LogFields{
		"exists path": path,
	}).Debug(">>>> csiutils.ExistsPath")
	defer Logc(ctx).Debug("<<<< csiutils.ExistsPath")

	isExistsResponse, err := mounter.FsClient.PathExists(ctx,
		&fs.PathExistsRequest{
			Path: normalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return isExistsResponse.Exists, err
}

// GetAPIVersions returns the versions of the client APIs this mounter is using.
func (mounter *csiProxyUtils) GetAPIVersions(ctx context.Context) string {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.GetAPIVersions")
	defer Logc(ctx).Debug("<<<< csiutils.GetAPIVersions")

	return fmt.Sprintf(
		"API Versions filesystem: %s, SMB: %s",
		fsclient.Version,
		smbclient.Version,
	)
}

func (mounter *csiProxyUtils) EvalHostSymlinks(ctx context.Context, pathname string) (string, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.EvalHostSymlinks")
	defer Logc(ctx).Debug("<<<< csiutils.EvalHostSymlinks")

	return "", errors.New("EvalHostSymlinks not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) GetMountRefs(ctx context.Context, pathname string) ([]string, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.GetMountRefs")
	defer Logc(ctx).Debug("<<<< csiutils.GetMountRefs")

	return []string{}, errors.New("GetMountRefs not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) GetFSGroup(ctx context.Context, pathname string) (int64, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.GetFSGroup")
	defer Logc(ctx).Debug("<<<< csiutils.GetFSGroup")

	return -1, errors.New("GetFSGroup not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) GetSELinuxSupport(ctx context.Context, pathname string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.GetSELinuxSupport")
	defer Logc(ctx).Debug("<<<< csiutils.GetSELinuxSupport")

	return false, errors.New("GetSELinuxSupport not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) GetMode(ctx context.Context, pathname string) (os.FileMode, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.GetMode")
	defer Logc(ctx).Debug("<<<< csiutils.GetMode")

	return 0, errors.New("GetMode not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) MountSensitive(
	ctx context.Context,
	source, target, fstype string, options, sensitiveOptions []string,
) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.MountSensitive")
	defer Logc(ctx).Debug("<<<< csiutils.MountSensitive")

	return errors.New("MountSensitive not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) MountSensitiveWithoutSystemd(
	ctx context.Context,
	source, target, fstype string, options, sensitiveOptions []string,
) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.MountSensitiveWithoutSystemd")
	defer Logc(ctx).Debug("<<<< csiutils.MountSensitiveWithoutSystemd")

	return errors.New("MountSensitiveWithoutSystemd not implemented for CSIProxyUtils")
}

func (mounter *csiProxyUtils) MountSensitiveWithoutSystemdWithMountFlags(
	ctx context.Context,
	source, target, fstype string, options, sensitiveOptions, mountFlags []string,
) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> csiutils.MountSensitiveWithoutSystemdWithMountFlags")
	defer Logc(ctx).Debug("<<<< csiutils.MountSensitiveWithoutSystemdWithMountFlags")

	return mounter.MountSensitive(ctx, source, target, fstype, options, sensitiveOptions /* sensitiveOptions */)
}

// NewCSIProxyMounter - creates a new CSI Proxy mounter struct which encompassed all the
// clients to the CSI proxy - filesystem, disk and volume clients.
func NewCSIProxyUtils() (*csiProxyUtils, error) {
	fsClient, err := fsclient.NewClient()
	if err != nil {
		return nil, err
	}
	smbClient, err := smbclient.NewClient()
	if err != nil {
		return nil, err
	}
	volumeClient, err := volumeclient.NewClient()
	if err != nil {
		return nil, err
	}

	return &csiProxyUtils{
		FsClient:     fsClient,
		SMBClient:    smbClient,
		VolumeClient: volumeClient,
	}, nil
}
