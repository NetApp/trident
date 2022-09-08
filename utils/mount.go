// Copyright 2022 NetApp, Inc. All Rights Reserved.
package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
)

// UmountAndRemoveTemporaryMountPoint unmounts and removes the temporaryMountDir
func UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount.UmountAndRemoveTemporaryMountPoint")

	return UmountAndRemoveMountPoint(ctx, path.Join(mountPath, temporaryMountDir))
}

// UmountAndRemoveMountPoint unmounts and removes the mountPoint
func UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount.UmountAndRemoveMountPoint")

	// Delete the mount point if it exists.
	if _, err := os.Stat(mountPoint); err == nil {
		if err = RemoveMountPoint(ctx, mountPoint); err != nil {
			return fmt.Errorf("failed to remove directory %s; %s", mountPoint, err)
		}
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithField("mountPoint", mountPoint).Errorf("Can't determine if mount point dir path exists; %s",
			err)
		return fmt.Errorf("can't determine if mount point dir path %s exists; %s", mountPoint, err)
	}

	return nil
}

// mountFilesystemForResize expands a filesystem. The xfs_growfs utility requires a mount point to expand the
// filesystem. Determining the size of the filesystem requires that the filesystem be mounted.
func mountFilesystemForResize(
	ctx context.Context, devicePath, stagedTargetPath, mountOptions string,
) (string, error) {
	logFields := log.Fields{
		"devicePath":       devicePath,
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

func IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	fields := log.Fields{
		"exportPath": exportPath,
		"target":     mountpoint,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> mount.IsNFSShareMounted")
	defer Logc(ctx).WithFields(fields).Debug("<<<< mount.IsNFSShareMounted")

	mounts, err := GetMountInfo(ctx)
	if err != nil {
		return false, err
	}

	for _, mount := range mounts {

		Logc(ctx).Tracef("Mount: %+v", mount)

		if mount.MountSource == exportPath && mount.MountPoint == mountpoint {
			Logc(ctx).Debug("NFS Share is mounted.")
			return true, nil
		}
	}

	Logc(ctx).Debug("NFS Share is not mounted.")
	return false, nil
}

// IsMounted verifies if the supplied device is attached at the supplied location.
// IsMounted checks whether the specified device is attached at the given mountpoint.
// If no source device is specified, any existing mount with the specified mountpoint returns true.
// If no mountpoint is specified, any existing mount with the specified device returns true.
func IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	logFields := log.Fields{"source": sourceDevice, "target": mountpoint}
	Logc(ctx).WithFields(logFields).Debug(">>>> mount.IsMounted")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< mount.IsMounted")

	sourceDevice = strings.TrimPrefix(sourceDevice, "/dev/")

	// Ensure at least one arg was specified
	if sourceDevice == "" && mountpoint == "" {
		return false, errors.New("no device or mountpoint specified")
	}

	// Read the system mounts
	procSelfMountinfo, err := listProcSelfMountinfo(procSelfMountinfoPath)
	if err != nil {
		Logc(ctx).WithFields(logFields).Errorf("checking mounts failed; %s", err)
		return false, fmt.Errorf("checking mounts failed; %s", err)
	}

	// Check each mount for a match of source device and/or mountpoint
	for _, procMount := range procSelfMountinfo {

		// If mountpoint was specified and doesn't match proc mount, move on
		if mountpoint != "" {
			if !strings.Contains(procMount.MountPoint, mountpoint) {
				continue
			} else {
				Logc(ctx).Debugf("Mountpoint found: %v", procMount)
			}
		}

		// If sourceDevice was specified and doesn't match proc mount, move on
		if sourceDevice != "" {

			procSourceDevice := strings.TrimPrefix(procMount.Root, "/")

			// Resolve any symlinks to get the real device
			if strings.HasPrefix(procMount.MountSource, "/dev/") {
				procSourceDevice, err = filepath.EvalSymlinks(procMount.MountSource)
				if err != nil {
					Logc(ctx).Error(err)
					continue
				}
				procSourceDevice = strings.TrimPrefix(procSourceDevice, "/dev/")
			}

			if sourceDevice != procSourceDevice {
				continue
			} else {
				Logc(ctx).Debugf("Device found: %v", sourceDevice)

				if err = CheckMountOptions(ctx, procMount, mountOptions); err != nil {
					Logc(ctx).WithFields(logFields).Errorf("checking mount options failed; %s", err)
				}
			}
		}

		Logc(ctx).WithFields(logFields).Debug("Mount information found.")
		return true, nil
	}

	Logc(ctx).WithFields(logFields).Debug("Mount information not found.")
	return false, nil
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

// GetMountInfo returns the list of mounts found in /proc/self/mountinfo
func GetMountInfo(ctx context.Context) ([]MountInfo, error) {
	Logc(ctx).Debug(">>>> mount.GetMountInfo")
	defer Logc(ctx).Debug("<<<< mount.GetMountInfo")

	return listProcSelfMountinfo(procSelfMountinfoPath)
}

// MountDevice attaches the supplied device at the supplied location.  Use this for iSCSI devices.
func MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error) {
	Logc(ctx).WithFields(log.Fields{
		"device":     device,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount.MountDevice")
	defer Logc(ctx).Debug("<<<< mount.MountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), device, mountpoint}
	} else {
		args = []string{device, mountpoint}
	}

	mounted, _ := IsMounted(ctx, device, mountpoint, options)
	exists := PathExists(mountpoint)

	Logc(ctx).Debugf("Already mounted: %v, mountpoint exists: %v", mounted, exists)

	if !exists {
		if isMountPointFile {
			if err = EnsureFileExists(ctx, mountpoint); err != nil {
				Logc(ctx).WithField("error", err).Warning("File check failed.")
			}
		} else {
			if err = EnsureDirExists(ctx, mountpoint); err != nil {
				Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
			}
		}
	}

	if !mounted {
		if _, err = execCommand(ctx, "mount", args...); err != nil {
			Logc(ctx).WithField("error", err).Error("Mount failed.")
		}
	}

	return
}

// RemountDevice remounts the mountpoint with supplied mount options.
func RemountDevice(ctx context.Context, mountpoint, options string) (err error) {
	Logc(ctx).WithFields(log.Fields{
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount.RemountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), mountpoint}
	} else {
		args = []string{mountpoint}
	}

	if _, err = execCommand(ctx, "mount", args...); err != nil {
		Logc(ctx).WithField("error", err).Error("Remounting failed.")
	}

	return
}

// mountNFSPath attaches the supplied NFS share at the supplied location with options.
func mountNFSPath(ctx context.Context, exportPath, mountpoint, options string) (err error) {
	Logc(ctx).WithFields(log.Fields{
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount.mountNFSPath")
	defer Logc(ctx).Debug("<<<< mount.mountNFSPath")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-t", "nfs", "-o", strings.TrimPrefix(options, "-o "), exportPath, mountpoint}
	} else {
		args = []string{"-t", "nfs", exportPath, mountpoint}
	}

	// Create the mount point dir if necessary
	if _, err = execCommand(ctx, "mkdir", "-p", mountpoint); err != nil {
		Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
	}

	if out, err := execCommand(ctx, "mount", args...); err != nil {
		Logc(ctx).WithField("output", string(out)).Debug("Mount failed.")
		return fmt.Errorf("error mounting NFS volume %v on mountpoint %v: %v", exportPath, mountpoint, err)
	}

	return nil
}

const (
	umountNotMounted = "not mounted"
	umountTimeout    = 10
)

// Umount detaches from the supplied location.
func Umount(ctx context.Context, mountpoint string) (err error) {
	Logc(ctx).WithField("mountpoint", mountpoint).Debug(">>>> mount.Umount")
	defer Logc(ctx).Debug("<<<< mount.Umount")

	var out []byte
	if out, err = execCommandWithTimeout(ctx, "umount", umountTimeout, true, mountpoint); err != nil {
		if strings.Contains(string(out), umountNotMounted) {
			err = nil
		}
		if IsTimeoutError(err) {
			Logc(ctx).WithField("error", err).Error("Umount failed, attempting to force umount")
			out, err = execCommandWithTimeout(ctx, "umount", umountTimeout, true, mountpoint, "-f")
			if strings.Contains(string(out), umountNotMounted) {
				err = nil
			}
		}
	}
	return
}

// RemoveMountPoint attempts to unmount and remove the directory of the mountPointPath.  This method should
// be idempotent and safe to call again if it fails the first time.
func RemoveMountPoint(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount.RemoveMountPoint")

	// If the directory does not exist, return nil.
	if _, err := os.Stat(mountPointPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("could not check for mount path %s; %v", mountPointPath, err)
	}

	// Unmount the path.  Umount() returns nil if the path exists but is not a mount.
	if err := Umount(ctx, mountPointPath); err != nil {
		Logc(ctx).WithField("mountPointPath", mountPointPath).Errorf("Umount failed; %s", err)
		return err
	}

	// Delete the mount path after it is unmounted (or confirmed to not be a mount point).
	if err := os.Remove(mountPointPath); err != nil {
		Logc(ctx).WithField("mountPointPath", mountPointPath).Errorf("Remove dir failed; %s", err)
		return fmt.Errorf("failed to remove dir %s; %s", mountPointPath, err)
	}

	return nil
}

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

// listProcSelfMountinfo (Available since Linux 2.6.26) lists information about mount points
// in the process's mount namespace. Ref: http://man7.org/linux/man-pages/man5/proc.5.html
// for /proc/[pid]/mountinfo
func listProcSelfMountinfo(mountFilePath string) ([]MountInfo, error) {
	content, err := ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcSelfMountinfo(content)
}

// parseProcSelfMountinfo parses the output of /proc/self/mountinfo file into a slice of MountInfo struct
func parseProcSelfMountinfo(content []byte) ([]MountInfo, error) {
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
