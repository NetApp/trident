// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
)

// Part of the published path for raw devices
const rawDevicePublishPath = "plugins/kubernetes.io/csi/volumeDevices/publish/pvc-"

// Regex to identify published path for mounted devices
var pvMountpointRegex = regexp.MustCompile(`^(.*pods)(.*volumes)(.*pvc-).*$`)

// IsLikelyNotMountPoint uses heuristics to determine if a directory is not a mountpoint.
// It should return ErrNotExist when the directory does not exist.
// IsLikelyNotMountPoint does NOT properly detect all mountpoint types
// most notably Linux bind mounts and symbolic links. For callers that do not
// care about such situations, this is a faster alternative to scanning the list of mounts.
// A return value of false means the directory is definitely a mount point.
// A return value of true means it's not a mount this function knows how to find,
// but it could still be a mount point.
func IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	fields := LogFields{"mountpoint": mountpoint}
	Logc(ctx).WithFields(fields).Debug(">>>> mount_linux.IsLikelyNotMountPoint")
	defer Logc(ctx).WithFields(fields).Debug("<<<< mount_linux.IsLikelyNotMountPoint")

	stat, err := os.Stat(mountpoint)
	if err != nil {
		return true, err
	}
	rootStat, err := os.Lstat(filepath.Dir(strings.TrimSuffix(mountpoint, "/")))
	if err != nil {
		return true, err
	}
	// If the directory has a different device as parent, then it is a mountpoint.
	if stat.Sys().(*syscall.Stat_t).Dev != rootStat.Sys().(*syscall.Stat_t).Dev {
		Logc(ctx).WithFields(fields).Debug("Path is a mountpoint.")
		return false, nil
	}

	Logc(ctx).WithFields(fields).Debug("Path is likely not a mountpoint.")
	return true, nil
}

// IsMounted verifies if the supplied device is attached at the supplied location.
// IsMounted checks whether the specified device is attached at the given mountpoint.
// If no source device is specified, any existing mount with the specified mountpoint returns true.
// If no mountpoint is specified, any existing mount with the specified device returns true.
func IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	logFields := LogFields{"source": sourceDevice, "target": mountpoint}
	Logc(ctx).WithFields(logFields).Debug(">>>> mount_linux.IsMounted")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< mount_linux.IsMounted")

	sourceDevice = strings.TrimPrefix(sourceDevice, "/dev/")

	// Ensure at least one arg was specified
	if sourceDevice == "" && mountpoint == "" {
		return false, errors.New("no device or mountpoint specified")
	}

	// Read the system mounts
	procSelfMountinfo, err := listProcMountinfo(procSelfMountinfoPath)
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

			if strings.HasPrefix(procMount.MountSource, "/dev/") {
				procSourceDevice = strings.TrimPrefix(procMount.MountSource, "/dev/")
				if sourceDevice != procSourceDevice {
					// Resolve any symlinks to get the real device
					procSourceDevice, err = filepath.EvalSymlinks(procMount.MountSource)
					if err != nil {
						Logc(ctx).Error(err)
						continue
					}
					procSourceDevice = strings.TrimPrefix(procSourceDevice, "/dev/")
				}
			}

			if sourceDevice != procSourceDevice {
				continue
			} else {
				Logc(ctx).Debugf("Device found: %v", sourceDevice)

				if err = CheckMountOptions(ctx, procMount, mountOptions); err != nil {
					Logc(ctx).WithFields(logFields).WithError(err).Warning("Checking mount options failed.")
				}
			}
		}

		Logc(ctx).WithFields(logFields).Debug("Mount information found.")
		return true, nil
	}

	Logc(ctx).WithFields(logFields).Debug("Mount information not found.")
	return false, nil
}

// PVMountpointMappings identifies devices corresponding to published paths
func PVMountpointMappings(ctx context.Context) (map[string]string, error) {
	Logc(ctx).Debug(">>>> mount_linux.PVMountpointMappings")
	defer Logc(ctx).Debug("<<<< mount_linux.PVMountpointMappings")

	mappings := make(map[string]string)

	// Read the system mounts
	procSelfMountinfo, err := listProcMountinfo(procSelfMountinfoPath)
	if err != nil {
		Logc(ctx).Errorf("checking mounts failed; %s", err)
		return nil, fmt.Errorf("checking mounts failed; %s", err)
	}

	// Check each mount for K8s-based mounts
	for _, procMount := range procSelfMountinfo {
		if pvMountpointRegex.MatchString(procMount.MountPoint) ||
			strings.Contains(procMount.MountPoint, rawDevicePublishPath) {

			// In case of raw block volumes device is at the `procMount.Root`
			procSourceDevice := strings.TrimPrefix(procMount.Root, "/")

			if strings.HasPrefix(procMount.MountSource, "/dev/") {
				procSourceDevice, err = filepath.EvalSymlinks(procMount.MountSource)
				if err != nil {
					Logc(ctx).Error(err)
					continue
				}
				procSourceDevice = strings.TrimPrefix(procSourceDevice, "/dev/")
			}

			if procSourceDevice != "" {
				mappings[procMount.MountPoint] = "/dev/" + procSourceDevice
			}
		}
	}

	return mappings, nil
}

// mountNFSPath attaches the supplied NFS share at the supplied location with options.
func mountNFSPath(ctx context.Context, exportPath, mountpoint, options string) (err error) {
	Logc(ctx).WithFields(LogFields{
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount_linux.mountNFSPath")
	defer Logc(ctx).Debug("<<<< mount_linux.mountNFSPath")

	// Build the command
	var args []string
	mountCommand := "mount.nfs"
	nfsVersRegex := regexp.MustCompile(`nfsvers\s*=\s*4(\.\d+)?`)
	if nfsVersRegex.MatchString(options) {
		mountCommand = "mount.nfs4"
	}
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), exportPath, mountpoint}
	} else {
		args = []string{exportPath, mountpoint}
	}

	// Create the mount point dir if necessary
	if _, err = command.Execute(ctx, "mkdir", "-p", mountpoint); err != nil {
		Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
	}

	if out, err := command.Execute(ctx, mountCommand, args...); err != nil {
		Logc(ctx).WithField("output", string(out)).Debug("Mount failed.")
		return fmt.Errorf("error mounting NFS volume %v on mountpoint %v: %v", exportPath, mountpoint, err)
	}

	return nil
}

// UmountAndRemoveTemporaryMountPoint unmounts and removes the temporaryMountDir
func UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount_linux.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount_linux.UmountAndRemoveTemporaryMountPoint")

	return UmountAndRemoveMountPoint(ctx, path.Join(mountPath, temporaryMountDir))
}

// UmountAndRemoveMountPoint unmounts and removes the mountPoint
func UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount_linux.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_linux.UmountAndRemoveMountPoint")

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

func IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	fields := LogFields{
		"exportPath": exportPath,
		"target":     mountpoint,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> mount_linux.IsNFSShareMounted")
	defer Logc(ctx).WithFields(fields).Debug("<<<< mount_linux.IsNFSShareMounted")

	mounts, err := GetSelfMountInfo(ctx)
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

// GetSelfMountInfo returns the list of mounts found in /proc/self/mountinfo
func GetSelfMountInfo(ctx context.Context) ([]MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_linux.GetSelfMountInfo")
	defer Logc(ctx).Debug("<<<< mount_linux.GetSelfMountInfo")

	return listProcMountinfo(procSelfMountinfoPath)
}

//  GetHostMountInfo returns the list of mounts found in /proc/1/mountinfo
func GetHostMountInfo(ctx context.Context) ([]MountInfo, error) {
	Logc(ctx).Debug(">>>> mount.GetHostMountInfo")
	defer Logc(ctx).Debug("<<<< mount.GetHostMountInfo")

	return listProcMountinfo(procHostMountinfoPath)
}

// MountDevice attaches the supplied device at the supplied location.  Use this for iSCSI devices.
func MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error) {
	Logc(ctx).WithFields(LogFields{
		"device":     device,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount_linux.MountDevice")
	defer Logc(ctx).Debug("<<<< mount_linux.MountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), device, mountpoint}
	} else {
		args = []string{device, mountpoint}
	}

	mounted, _ := IsMounted(ctx, device, mountpoint, options)
	exists, _ := PathExists(mountpoint)

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
		if _, err = command.Execute(ctx, "mount", args...); err != nil {
			Logc(ctx).WithField("error", err).Error("Mount failed.")
		}
	}

	return
}

// RemountDevice remounts the mountpoint with supplied mount options.
func RemountDevice(ctx context.Context, mountpoint, options string) (err error) {
	Logc(ctx).WithFields(LogFields{
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount_linux.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount_linux.RemountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), mountpoint}
	} else {
		args = []string{mountpoint}
	}

	if _, err = command.Execute(ctx, "mount", args...); err != nil {
		Logc(ctx).WithField("error", err).Error("Remounting failed.")
	}

	return
}

// Umount detaches from the supplied location.
func Umount(ctx context.Context, mountpoint string) (err error) {
	Logc(ctx).WithField("mountpoint", mountpoint).Debug(">>>> mount_linux.Umount")
	defer Logc(ctx).Debug("<<<< mount_linux.Umount")

	var out []byte
	if out, err = command.ExecuteWithTimeout(ctx, "umount", umountTimeout, true, mountpoint); err != nil {
		if strings.Contains(string(out), umountNotMounted) {
			err = nil
		}
		if errors.IsTimeoutError(err) {
			Logc(ctx).WithField("error", err).Error("Umount failed, attempting to force umount")
			out, err = command.ExecuteWithTimeout(ctx, "umount", umountTimeout, true, mountpoint, "-f")
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
	Logc(ctx).Debug(">>>> mount_linux.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_linux.RemoveMountPoint")

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

// mountSMBPath is a dummy added for compilation on non-windows platform.
func mountSMBPath(ctx context.Context, exportPath, mountpoint, username, password string) error {
	Logc(ctx).Debug(">>>> mount_linux.mountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_linux.mountSMBPath")
	return errors.UnsupportedError("mountSMBPath is not supported on non-windows platform")
}

// UmountSMBPath is a dummy added for compilation on non-windows platform.
func UmountSMBPath(ctx context.Context, mappingPath, target string) (err error) {
	Logc(ctx).Debug(">>>> mount_linux.UmountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_linux.UmountSMBPath")
	return errors.UnsupportedError("UmountSMBPath is not supported on non-windows platform")
}

// WindowsBindMount is a dummy added for compilation on non-windows platform.
func WindowsBindMount(ctx context.Context, source, target string, options []string) (err error) {
	Logc(ctx).Debug(">>>> mount_linux.WindowsBindMount")
	defer Logc(ctx).Debug("<<<< mount_linux.WindowsBindMount")
	return errors.UnsupportedError("WindowsBindMount is not supported on non-windows platform")
}

// IsCompatible checks for compatibility of protocol and platform
func IsCompatible(ctx context.Context, protocol string) error {
	Logc(ctx).Debug(">>>> mount_linux.IsCompatible")
	defer Logc(ctx).Debug("<<<< mount_linux.IsCompatible")

	if protocol != sa.NFS {
		return fmt.Errorf("mounting %s volume is not supported on linux", protocol)
	} else {
		return nil
	}
}
