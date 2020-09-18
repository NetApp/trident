// Copyright 2019 NetApp, Inc. All Rights Reserved.

package utils

import (
        "fmt"
        "os"
        "syscall"
        "time"
        "unsafe"

        log "github.com/sirupsen/logrus"
        unix "golang.org/x/sys/unix"
)

type statFSResult struct {
        Output unix.Statfs_t
        Error  error
}

// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func GetFilesystemStats(path string) (available, capacity, usage, inodes, inodesFree, inodesUsed int64, err error) {
        log.Debug(">>>> osutils_linux.GetFilesystemStats")
        defer log.Debug("<<<< osutils_linux.GetFilesystemStats")

        timedOut := false
        var timeout time.Duration = 30 * time.Second
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
                log.WithField("path", path).Errorf("Failed to statfs: %s", result.Error)
                return 0, 0, 0, 0, 0, 0, fmt.Errorf("couldn't get filesystem stats %s: %s", path, result.Error)
        } else if timedOut {
                log.WithField("path", path).Errorf("Failed to statfs due to timeout")
                return 0, 0, 0, 0, 0, 0, fmt.Errorf("couldn't get filesystem stats %s: timeout", path)
        }

        buf := result.Output
        size := int64(buf.Blocks) * buf.Bsize
        log.WithFields(log.Fields{
                "path":   path,
		"size":   size,
		"bsize":  buf.Bsize,
		"blocks": buf.Blocks,
		"avail":  buf.Bavail,
                "free":   buf.Bfree,
        }).Debug("Filesystem size information")

        available = int64(buf.Bavail) * buf.Bsize
        capacity = int64(size)
        usage = int64(capacity - available)
        inodes = int64(buf.Files)
        inodesFree = int64(buf.Ffree)
        inodesUsed = inodes - inodesFree
        return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}


// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func getFilesystemSize(path string) (int64, error) {
        log.Debug(">>>> osutils_linux.getFilesystemSize")
        defer log.Debug("<<<< osutils_linux.getFilesystemSize")

        _, size, _, _, _, _, err := GetFilesystemStats(path)
        if err != nil {
                return 0, err
        }

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

// flushOneDevice flushes any outstanding I/O to a disk
func flushOneDevice(ctx context.Context, devicePath string) error {
	fields := log.Fields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils_linux.flushOneDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils_linux.flushOneDevice")

	disk, err := os.Open(devicePath)
	if err != nil {
		Logc(ctx).Error("Failed to open disk.")
		return fmt.Errorf("failed to open disk %s: %s", devicePath, err)
	}
	defer disk.Close()

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, disk.Fd(), unix.BLKFLSBUF, 0)
	if errno != 0 {
		err := os.NewSyscallError("ioctl", errno)
		Logc(ctx).Error("BLKFLSBUF ioctl failed")
		return fmt.Errorf("BLKFLSBUF ioctl failed %s: %s", devicePath, err)
	}

	return nil
}
