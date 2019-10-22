// Copyright 2019 NetApp, Inc. All Rights Reserved.

package utils

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func getFilesystemSize(path string) (int64, error) {
	log.Debug(">>>> osutils_linux.getFilesystemSize")
	defer log.Debug("<<<< osutils_linux.getFilesystemSize")

	// Warning: syscall.Statfs_t uses types that are OS and arch dependent. The following code has been
	// confirmed to work with Linux/amd64 and Darwin/amd64.
	var buf syscall.Statfs_t
	err := syscall.Statfs(path, &buf)
	if err != nil {
		log.WithField("path", path).Errorf("Failed to statfs: %s", err)
		return 0, fmt.Errorf("couldn't get filesystem stats %s: %s", path, err)
	}

	size := int64(buf.Blocks) * buf.Bsize
	log.WithFields(log.Fields{
		"path":   path,
		"size":   size,
		"bsize":  buf.Bsize,
		"blocks": buf.Blocks,
		"avail":  buf.Bavail,
		"free":   buf.Bfree,
	}).Debug("Filesystem size information")
	return size, nil
}

// getISCSIDiskSize queries the current block size in bytes
func getISCSIDiskSize(devicePath string) (int64, error) {
	fields := log.Fields{"devicePath": devicePath}
	log.WithFields(fields).Debug(">>>> osutils_linux.getISCSIDiskSize")
	defer log.WithFields(fields).Debug("<<<< osutils_linux.getISCSIDiskSize")

	disk, err := os.Open(devicePath)
	if err != nil {
		log.Error("Failed to open disk.")
		return 0, fmt.Errorf("failed to open disk %s: %s", devicePath, err)
	}
	defer disk.Close()

	var size int64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, disk.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		err := os.NewSyscallError("ioctl", errno)
		log.Error("BLKGETSIZE64 ioctl failed")
		return 0, fmt.Errorf("BLKGETSIZE64 ioctl failed %s: %s", devicePath, err)
	}

	return size, nil
}
