// Copyright 2022 Netapp Inc. All Rights Reserved.

// This file should only contain functions for handling the filesystem for Linux flavor

package utils

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	. "github.com/netapp/trident/logger"
)

// GetFilesystemStats returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func GetFilesystemStats(
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
	size := int64(buf.Blocks) * buf.Bsize
	Logc(ctx).WithFields(log.Fields{
		"path":   path,
		"size":   size,
		"bsize":  buf.Bsize,
		"blocks": buf.Blocks,
		"avail":  buf.Bavail,
		"free":   buf.Bfree,
	}).Debug("Filesystem size information")

	available = int64(buf.Bavail) * buf.Bsize
	capacity = size
	usage = capacity - available
	inodes = int64(buf.Files)
	inodesFree = int64(buf.Ffree)
	inodesUsed = inodes - inodesFree
	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}

// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func getFilesystemSize(ctx context.Context, path string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_linux.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< filesystem_linux.getFilesystemSize")

	_, size, _, _, _, _, err := GetFilesystemStats(ctx, path)
	if err != nil {
		return 0, err
	}

	return size, nil
}
