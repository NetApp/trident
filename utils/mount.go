// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// mountFilesystemForResize expands a filesystem. The xfs_growfs utility requires a mount point to expand the
// filesystem. Determining the size of the filesystem requires that the filesystem be mounted.
func mountFilesystemForResize(
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
	err := MountDevice(ctx, devicePath, tmpMountPoint, mountOptions, false)
	if err != nil {
		return "", fmt.Errorf("unable to mount device; %s", err)
	}
	return tmpMountPoint, nil
}

// CheckMountOptions check if the new mount options are different from already mounted options.
// Return an error if there is mismatch with the mount options.
func CheckMountOptions(ctx context.Context, procMount MountInfo, mountOptions string) error {
	// We have the source already mounted. Compare the mount options from the request.
	optionSlice := strings.Split(strings.TrimPrefix(mountOptions, "-o"), ",")
	for _, option := range optionSlice {
		if option != "" && !AreMountOptionsInList(option,
			procMount.MountOptions) && !AreMountOptionsInList(option,
			procMount.SuperOptions) {

			return errors.New("mismatch in mount option: " + option +
				", this might cause mount failure")
		}
	}
	return nil
}

const (
	umountNotMounted = "not mounted"
	umountTimeout    = 10 * time.Second
)

// RemoveMountPointRetry attempts to unmount and remove the directory of the mountPointPath.  This method should
// be idempotent and safe to call again if it fails the first time.
func RemoveMountPointRetry(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount.RemoveMountPoint")

	const removeMountPointMaxWait = 15 * time.Second

	removeMountPoint := func() error {
		return RemoveMountPoint(ctx, mountPointPath)
	}

	removeMountPointNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Remove mount point failed, retrying.")
	}

	removeMountPointBackoff := backoff.NewExponentialBackOff()
	removeMountPointBackoff.InitialInterval = 1 * time.Second
	removeMountPointBackoff.Multiplier = 2
	removeMountPointBackoff.RandomizationFactor = 0.1
	removeMountPointBackoff.MaxElapsedTime = removeMountPointMaxWait

	// Run the RemoveMountPoint using an exponential backoff
	if err := backoff.RetryNotify(removeMountPoint, removeMountPointBackoff, removeMountPointNotify); err != nil {
		Logc(ctx).Warnf("Could not remove device after %3.2f seconds.", removeMountPointMaxWait.Seconds())
		return err
	}

	Logc(ctx).Info("Device removed.")
	return nil
}

const (
	// How many times to retry for a consistent read of /proc/mounts.
	maxListTries = 3
	// Number of fields per line in /proc/mounts as per the fstab man page.
	expectedNumProcMntFieldsPerLine = 6
	// Minimum number of fields per line in /proc/self/mountinfo as per the proc man page.
	minNumProcSelfMntInfoFieldsPerLine = 10
	// Location of the mount file to use
	procMountsPath = "/proc/mounts"
	// Location of the mount file to use
	procSelfMountinfoPath = "/proc/self/mountinfo"
	// Location of the host mount file to use
	procHostMountinfoPath = "/proc/1/mountinfo"
)

// This represents a single line in /proc/mounts or /etc/fstab.
type MountPoint struct {
	Device string
	Path   string
	Type   string
	Opts   []string
	Freq   int
	Pass   int
}

// This represents a single line in /proc/self/mountinfo.
type MountInfo struct {
	MountId      int
	ParentId     int
	DeviceId     string
	Root         string
	MountPoint   string
	MountOptions []string
	// OptionalFields []string
	FsType       string
	MountSource  string
	SuperOptions []string
}

// listProcMountinfo (Available since Linux 2.6.26) lists information about mount points
// in the process's mount namespace. Ref: http://man7.org/linux/man-pages/man5/proc.5.html
// for /proc/[pid]/mountinfo
func listProcMountinfo(mountFilePath string) ([]MountInfo, error) {
	content, err := ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcMountinfo(content)
}

// parseProcMountinfo parses the output of /proc/self/mountinfo file into a slice of MountInfo struct
func parseProcMountinfo(content []byte) ([]MountInfo, error) {
	out := make([]MountInfo, 0)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			// The last split() item is empty string following the last \n
			continue
		}
		fields := strings.Fields(line)
		numFields := len(fields)
		if numFields < minNumProcSelfMntInfoFieldsPerLine {
			return nil, fmt.Errorf("wrong number of fields (expected at least %d, got %d): %s",
				minNumProcSelfMntInfoFieldsPerLine, numFields, line)
		}

		// separator must be in the 4th position from the end for the line to contain fsType, mountSource, and
		//  superOptions
		if fields[numFields-4] != "-" {
			return nil, fmt.Errorf("malformed mountinfo (could not find separator): %s", line)
		}

		// If root value is marked deleted, skip the entry
		if strings.Contains(fields[3], "deleted") {
			continue
		}

		mp := MountInfo{
			DeviceId:     fields[2],
			Root:         fields[3],
			MountPoint:   fields[4],
			MountOptions: strings.Split(fields[5], ","),
		}

		mountId, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, err
		}
		mp.MountId = mountId

		parentId, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, err
		}
		mp.ParentId = parentId

		mp.FsType = fields[numFields-3]
		mp.MountSource = fields[numFields-2]
		mp.SuperOptions = strings.Split(fields[numFields-1], ",")

		out = append(out, mp)
	}
	return out, nil
}

func listProcMounts(mountFilePath string) ([]MountPoint, error) {
	content, err := ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcMounts(content)
}

func parseProcMounts(content []byte) ([]MountPoint, error) {
	out := make([]MountPoint, 0)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			// the last split() item is empty string following the last \n
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != expectedNumProcMntFieldsPerLine {
			return nil, fmt.Errorf("wrong number of fields (expected %d, got %d): %s",
				expectedNumProcMntFieldsPerLine, len(fields), line)
		}

		mp := MountPoint{
			Device: fields[0],
			Path:   fields[1],
			Type:   fields[2],
			Opts:   strings.Split(fields[3], ","),
		}

		freq, err := strconv.Atoi(fields[4])
		if err != nil {
			return nil, err
		}
		mp.Freq = freq

		pass, err := strconv.Atoi(fields[5])
		if err != nil {
			return nil, err
		}
		mp.Pass = pass

		out = append(out, mp)
	}
	return out, nil
}
