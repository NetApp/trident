// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This file should only contain functions for handling the filesystem for Linux flavor

package filesystem

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type statFSResult struct {
	Output unix.Statfs_t
	Error  error
}

// GetFilesystemStats returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func (f *FSClient) GetFilesystemStats(
	ctx context.Context, path string,
) (available, capacity, usage, inodes, inodesFree, inodesUsed int64, err error) {
	Logc(ctx).Debug(">>>> filesystem_linux.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< filesystem_linux.GetFilesystemStats")

	timedOut := false
	timeout := 30 * time.Second
	done := make(chan statFSResult, 1)
	var result statFSResult

	go func() {
		// Warning: syscall.Statfs_t uses types that are OS and arch dependent. The following code has been
		// confirmed to work with Linux/amd64 and Darwin/amd64.
		var buf unix.Statfs_t
		err := unix.Statfs(path, &buf)
		done <- statFSResult{Output: buf, Error: err}
	}()

	select {
	case <-time.After(timeout):
		timedOut = true
	case result = <-done:
		break
	}

	if result.Error != nil {
		Logc(ctx).WithField("path", path).Errorf("Failed to statfs: %s", result.Error)
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("couldn't get filesystem stats %s: %s", path, result.Error)
	} else if timedOut {
		Logc(ctx).WithField("path", path).Errorf("Failed to statfs due to timeout")
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("couldn't get filesystem stats %s: timeout", path)
	}

	buf := result.Output
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

// GenerateAnonymousMemFile uses linux syscall memfd_create to create an anonymous, temporary, in-memory file
// with the specified name and contents
func (f *FSClient) GenerateAnonymousMemFile(tempFileName, content string) (int, error) {
	fd, err := unix.MemfdCreate(tempFileName, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to create anonymous file; %v", err)
	}
	_, err = unix.Write(fd, []byte(content))
	if err != nil {
		return fd, fmt.Errorf("failed to write anonymous file; %v", err)
	}
	// Rewind back to the beginning
	_, err = unix.Seek(fd, 0, 0)
	if err != nil {
		return fd, fmt.Errorf("failed to rewind anonymous file; %v", err)
	}
	return fd, nil
}
