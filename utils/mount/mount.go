// Copyright 2025 NetApp, Inc. All Rights Reserved.

package mount

//go:generate mockgen -destination=../../mocks/mock_utils/mock_mount/mock_mount_client.go github.com/netapp/trident/utils/mount Mount
//go:generate mockgen -destination=../../mocks/mock_utils/mock_mount/mock_filesystem/mock_filesystem_client.go -package=mock_filesystem github.com/netapp/trident/utils/mount  FilesystemClient
//go:generate mockgen -destination=../../mocks/mock_utils/mock_mount/mock_smb/mock_smb_client.go -package=mock_smb github.com/netapp/trident/utils/mount  SmbClient

import (
	"context"
	"fmt"
	"path"
	"time"

	fs "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1"
	smb "github.com/kubernetes-csi/csi-proxy/client/api/smb/v1"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
)

const (
	umountNotMounted  = "not mounted"
	umountTimeout     = 10 * time.Second
	temporaryMountDir = "/tmp_mnt"

	// How many times to retry for a consistent read of /proc/mounts.
	maxListTries = 3
	// Number of fields per line in /proc/mounts as per the fstab man page.
	expectedNumProcMntFieldsPerLine = 6
	// Minimum number of fields per line in /proc/self/mountinfo as per the proc man page.
	minNumProcSelfMntInfoFieldsPerLine = 10
)

type Mount interface {
	mountOS
	MountFilesystemForResize(ctx context.Context, devicePath, stagedTargetPath, mountOptions string) (string, error)
	AttachNFSVolume(ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo) error
	AttachSMBVolume(ctx context.Context, name, mountpoint, username, password string, publishInfo *models.VolumePublishInfo) error
}

type mountOS interface {
	IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error)
	IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error)
	MountNFSPath(ctx context.Context, exportPath, mountpoint, options string) error
	MountSMBPath(ctx context.Context, exportPath, mountpoint, username, password string) error
	UmountSMBPath(ctx context.Context, mappingPath, target string) error
	WindowsBindMount(ctx context.Context, source, target string, options []string) error
	IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error)
	MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error)
	Umount(ctx context.Context, mountpoint string) (err error)
	RemoveMountPoint(ctx context.Context, mountPointPath string) error
	UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error
	UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error
	RemountDevice(ctx context.Context, mountpoint, options string) (err error)
	GetSelfMountInfo(ctx context.Context) ([]models.MountInfo, error)
	GetHostMountInfo(ctx context.Context) ([]models.MountInfo, error)
	IsCompatible(ctx context.Context, protocol string) error
	PVMountpointMappings(ctx context.Context) (map[string]string, error)
	ListProcMounts(mountFilePath string) ([]models.MountPoint, error)
	ListProcMountinfo() ([]models.MountInfo, error)
}

// FilesystemClient is a wrapper around the filesystem client to help generate mocks
type FilesystemClient interface {
	fs.FilesystemClient
}

// SmbClient is a wrapper around the smb client to help generate mocks
type SmbClient interface {
	smb.SmbClient
}

type Client struct {
	mountOS
}

func New() (*Client, error) {
	client, err := newOsSpecificClient()
	if err != nil {
		return nil, err
	}

	return NewDetailed(client), nil
}

func NewDetailed(mountOS mountOS) *Client {
	return &Client{
		mountOS: mountOS,
	}
}

// MountFilesystemForResize expands a filesystem. The xfs_growfs utility requires a mount point to expand the
// filesystem. Determining the size of the filesystem requires that the filesystem be mounted.
func (c *Client) MountFilesystemForResize(
	ctx context.Context, devicePath, stagedTargetPath, mountOptions string,
) (string, error) {
	logFields := LogFields{
		"rawDevicePath":    devicePath,
		"stagedTargetPath": stagedTargetPath,
		"mountOptions":     mountOptions,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> mount.mountFilesystemForResize")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< mount.mountFilesystemForResize")

	tmpMountPoint := path.Join(stagedTargetPath, temporaryMountDir)
	if err := c.MountDevice(ctx, devicePath, tmpMountPoint, mountOptions, false); err != nil {
		return "", fmt.Errorf("unable to mount device; %s", err)
	}
	return tmpMountPoint, nil
}

// AttachNFSVolume attaches the volume to the local host.
// This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.
func (c *Client) AttachNFSVolume(ctx context.Context, name, mountpoint string,
	publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> nfs.AttachNFSVolume")
	defer Logc(ctx).Debug("<<<< nfs.AttachNFSVolume")

	exportPath := fmt.Sprintf("%s:%s", publishInfo.NfsServerIP, publishInfo.NfsPath)
	options := publishInfo.MountOptions

	Logc(ctx).WithFields(LogFields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug("Publishing NFS volume.")

	return c.MountNFSPath(ctx, exportPath, mountpoint, options)
}

// AttachSMBVolume attaches the volume to the local host. This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.
func (c *Client) AttachSMBVolume(
	ctx context.Context, name, mountpoint, username, password string, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> smb.AttachSMBSVolume")
	defer Logc(ctx).Debug("<<<< smb.AttachSMBVolume")

	exportPath := fmt.Sprintf("\\\\%s%s", publishInfo.SMBServer, publishInfo.SMBPath)

	Logc(ctx).WithFields(LogFields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
	}).Debug("Publishing SMB volume.")

	return c.MountSMBPath(ctx, exportPath, mountpoint, username, password)
}
