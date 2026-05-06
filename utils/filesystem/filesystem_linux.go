// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This file should only contain functions for handling the filesystem for Linux flavor

package filesystem

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/netapp/trident/internal/syswrap"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// GetFilesystemStats returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func (f *FSClient) GetFilesystemStats(
	ctx context.Context, path string,
) (available, capacity, usage, inodes, inodesFree, inodesUsed int64, err error) {
	Logc(ctx).Debug(">>>> filesystem_linux.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< filesystem_linux.GetFilesystemStats")

	buf, err := syswrap.Statfs(ctx, path, 30*time.Second)
	if err != nil {
		Logc(ctx).WithField("path", path).Errorf("Failed to statfs: %s", err)
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("couldn't get filesystem stats %s: %s", path, err)
	}

	//goland:noinspection GoRedundantConversion
	size := int64(buf.Blocks) * int64(buf.Bsize)

	Logc(ctx).WithFields(LogFields{
		"path":   path,
		"size":   size,
		"bsize":  buf.Bsize,
		"blocks": buf.Blocks,
		"avail":  buf.Bavail,
		"free":   buf.Bfree,
	}).Debug("Filesystem size information")

	//goland:noinspection GoRedundantConversion
	available = int64(buf.Bavail) * int64(buf.Bsize)
	capacity = size
	// Usage should be calculated from Bfree (total free), not Bavail (available to non-root)
	// The difference between Bfree and Bavail is space reserved for root
	usage = (int64(buf.Blocks) - int64(buf.Bfree)) * int64(buf.Bsize)
	inodes = int64(buf.Files)
	inodesFree = int64(buf.Ffree)
	inodesUsed = inodes - inodesFree
	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}

// getBlockDeviceSize reports devicePath capacity via BLKGETSIZE64 (kernel-visible; may trail provisioned size slightly).
func (f *FSClient) getBlockDeviceSize(ctx context.Context, devicePath string) (int64, error) {
	Logc(ctx).WithField("devicePath", devicePath).Debug(">>>> filesystem_linux.getBlockDeviceSize")
	defer Logc(ctx).WithField("devicePath", devicePath).Debug("<<<< filesystem_linux.getBlockDeviceSize")

	disk, err := os.Open(devicePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open block device %s: %w", devicePath, err)
	}
	defer disk.Close()

	var size int64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, disk.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, fmt.Errorf("BLKGETSIZE64 ioctl failed for %s: %w", devicePath, os.NewSyscallError("ioctl", errno))
	}

	return size, nil
}

// GetDeviceFilePath returns the staging path for volume.
func GetDeviceFilePath(ctx context.Context, path, volumeId string) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_linux.GetDeviceFilePath")
	defer Logc(ctx).Debug("<<<< filesystem_linux.GetDeviceFilePath")

	return path, nil
}

// GetUnmountPath is a dummy added for compilation
func (f *FSClient) GetUnmountPath(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_linux.GetUnmountPath")
	defer Logc(ctx).Debug("<<<< filesystem_linux.GetUnmountPath")

	return "", errors.UnsupportedError("GetUnmountPath is not supported for linux")
}
