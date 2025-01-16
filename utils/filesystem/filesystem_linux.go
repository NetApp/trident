// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This file should only contain functions for handling the filesystem for Linux flavor

package filesystem

import (
	"context"
	"fmt"
	"time"

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
	usage = capacity - available
	inodes = int64(buf.Files)
	inodesFree = int64(buf.Ffree)
	inodesUsed = inodes - inodesFree
	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}

// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func (f *FSClient) getFilesystemSize(ctx context.Context, path string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_linux.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< filesystem_linux.getFilesystemSize")

	_, size, _, _, _, _, err := f.GetFilesystemStats(ctx, path)
	if err != nil {
		return 0, err
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
