// Copyright 2019 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"github.com/zcalusic/sysinfo"
	"golang.org/x/sys/unix"

	. "github.com/netapp/trident/logger"
)

type statFSResult struct {
	Output unix.Statfs_t
	Error  error
}

// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func GetFilesystemStats(
	ctx context.Context, path string,
) (available, capacity, usage, inodes, inodesFree, inodesUsed int64, err error) {

	Logc(ctx).Debug(">>>> osutils_linux.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< osutils_linux.GetFilesystemStats")

	timedOut := false
	var timeout = 30 * time.Second
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
	capacity = int64(size)
	usage = int64(capacity - available)
	inodes = int64(buf.Files)
	inodesFree = int64(buf.Ffree)
	inodesUsed = inodes - inodesFree
	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}

// getFilesystemSize returns the size of the filesystem for the given path.
// The caller of the func is responsible for verifying the mountPoint existence and readiness.
func getFilesystemSize(ctx context.Context, path string) (int64, error) {

	Logc(ctx).Debug(">>>> osutils_linux.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< osutils_linux.getFilesystemSize")

	_, size, _, _, _, _, err := GetFilesystemStats(ctx, path)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// getISCSIDiskSize queries the current block size in bytes
func getISCSIDiskSize(ctx context.Context, devicePath string) (int64, error) {

	fields := log.Fields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils_linux.getISCSIDiskSize")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils_linux.getISCSIDiskSize")

	disk, err := os.Open(devicePath)
	if err != nil {
		Logc(ctx).Error("Failed to open disk.")
		return 0, fmt.Errorf("failed to open disk %s: %s", devicePath, err)
	}
	defer disk.Close()

	var size int64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, disk.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		err := os.NewSyscallError("ioctl", errno)
		Logc(ctx).Error("BLKGETSIZE64 ioctl failed")
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

func determineNFSPackages(ctx context.Context, host HostSystem) ([]string, error) {

	var packages []string

	switch host.OS.Distro {
	case Centos, RHEL:
		packages = append(packages, "nfs-utils")
	case Ubuntu:
		packages = append(packages, "nfs-common")
	default:
		err := fmt.Errorf("unsupported Linux distro")
		Logc(ctx).WithField("distro", host.OS.Distro).Error(err)
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"distro":   host.OS.Distro,
		"packages": packages,
	}).Debug("Determined NFS packages based on distro.")

	return packages, nil
}

func PrepareNFSPackagesOnHost(ctx context.Context, host HostSystem) error {

	Logc(ctx).Debug(">>>> osutils_linux.PrepareNFSPackagesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.PrepareNFSPackagesOnHost")

	packages, err := determineNFSPackages(ctx, host)
	if err != nil {
		err = fmt.Errorf("could not determine NFS packages; %+v", err)
		Logc(ctx).Error(err)
		return err
	}

	var notInstalled []string
	_, notInstalled, err = checkPackagesOnHost(ctx, packages, host)
	if err != nil {
		err = fmt.Errorf("error checking for installed nfs packages; %+v", err)
		Logc(ctx).WithField("packages", packages)
		return err
	}

	err = installPackagesOnHost(ctx, notInstalled, host)
	if err != nil {
		err = fmt.Errorf("error installing packages on host; %+v", err)
		Logc(ctx).WithField("packages", packages).Error(err)
		return err
	}
	return nil
}

func checkPackagesOnHost(
	ctx context.Context, packages []string, host HostSystem,
) (installed, notInstalled []string, err error) {

	for _, pkg := range packages {
		found, err2 := packageInstalledOnHost(ctx, pkg, host)
		if err2 != nil {
			err = fmt.Errorf("error checking for package on host; %+v", err2)
			Logc(ctx).WithField("package", pkg).Error(err)
			return
		}
		if found {
			installed = append(installed, pkg)
		} else {
			notInstalled = append(notInstalled, pkg)
		}
	}
	return
}

func packageInstalledOnHost(ctx context.Context, pkg string, host HostSystem) (bool, error) {

	// Determine command to use
	cmd, err := getPackageManagerForHost(ctx, host)
	if err != nil {
		err = fmt.Errorf("error determining host's package manager; %+v", err)
		Logc(ctx).Error(err)
		return false, err
	}

	// Determine arguments for command
	var args []string
	switch host.OS.Distro {
	case Centos, RHEL:
		args = []string{"list", "installed", pkg}
	case Ubuntu:
		args = []string{"list", "--installed", pkg}
	default:
		err := fmt.Errorf("unsupported distro: %s", host.OS.Distro)
		Logc(ctx).Error(err)
		return false, err
	}

	var output []byte
	output, err = execCommandWithTimeout(ctx, cmd, 30, true, args...)
	switch host.OS.Distro {
	case Centos, RHEL:
		// For CentOS and RHEL we can simply check the return code of the package list command
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() == 1 {
					// package list exited with code 1, probably indicates package not found
					return false, nil
				} else {
					// errored exit code other than 1 indicates unknown error
					err = fmt.Errorf("unexpected error while checking if package is installed; %s; %+v",
						string(output), err)
					Logc(ctx).WithField("package", pkg).Error(err)
					return false, err
				}
			} else {
				// unknown error type
				err = fmt.Errorf("unexpected error while listing package; %+v", err)
				Logc(ctx).WithField("package", pkg).Error(err)
				return false, err
			}
		}
	case Ubuntu:
		// For Ubuntu we must parse the output of the apt list command to see if it includes the package
		// name as the return code is always 0 except on errors.
		if err != nil {
			err = fmt.Errorf("unexpected error while checking if package is installed; %s; %+v",
				string(output), err)
			Logc(ctx).WithField("package", pkg).Error(err)
			return false, err
		}
		if found, matchErr := regexp.Match(pkg, output); matchErr != nil {
			err = fmt.Errorf("error parsing %s output; %+v", cmd, matchErr)
			Logc(ctx).Error(err)
			return false, err
		} else if !found {
			return false, nil
		}
	}
	return true, nil
}

func getPackageManagerForHost(ctx context.Context, host HostSystem) (string, error) {
	switch host.OS.Distro {
	case Centos, RHEL:
		version, err := strconv.ParseInt(host.OS.Version, 10, 0)
		if err != nil {
			err = fmt.Errorf("error parsing OS version; %+v", err)
			Logc(ctx).WithField("version", host.OS.Version).Error(err)
			return "", err
		}
		if version < int64(8) {
			return "yum", nil
		} else {
			return "dnf", nil
		}
	case Ubuntu:
		return "apt", nil
	default:
		err := fmt.Errorf("unsupported distro: %s", host.OS.Distro)
		Logc(ctx).Error(err)
		return "", err
	}
}

func installPackagesOnHost(ctx context.Context, packages []string, host HostSystem) error {

	if len(packages) == 0 {
		Logc(ctx).Debug("No packages to install.")
		return nil
	}

	// Determine command to use
	pkgMgr, err := getPackageManagerForHost(ctx, host)
	if err != nil {
		err = fmt.Errorf("error determining host's package manager; %+v", err)
		Logc(ctx).Error(err)
		return err
	}

	// Update repo lists
	cmd := ""
	switch host.OS.Distro {
	case Ubuntu:
		cmd = "update"
	case Centos, RHEL:
		cmd = "check-update"
	default:
		err = fmt.Errorf("unsupported distro: %s", host.OS.Distro)
		Logc(ctx).Error(err)
		return err
	}
	var output []byte
	output, err = execCommandWithTimeout(ctx, pkgMgr, 300, true, cmd)
	if err != nil {
		switch pkgMgr {
		case "dnf", "yum":
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() == 100 {
					// yum/dnf exit code 100 indicates packages are available for update
					break
				}
			}
			fallthrough
		default:
			err = fmt.Errorf("error updating package list; %s; %+v", string(output), err)
			Logc(ctx).Error(err)
			return err
		}
	}

	args := []string{"install", "-y"}
	args = append(args, packages...)
	timeout := Max(int64(120*len(packages)), 120) // Increase timeout based on number of packages that need installed
	output, err = execCommandWithTimeout(ctx, pkgMgr, time.Duration(timeout), true, args...)
	if err != nil {
		err = fmt.Errorf("problem installing packages with %s; %s; %+v", pkgMgr, string(output), err)
		Logc(ctx).WithField("packages", packages).Error(err)
		return err
	}

	return nil
}

func PrepareNFSServicesOnHost(ctx context.Context) error {

	Logc(ctx).Debug(">>>> osutils_linux.PrepareNFSServicesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.PrepareNFSServicesOnHost")

	var services = []string{"rpc-statd"}

	for _, service := range services {
		fields := log.Fields{"service": service}
		active, err := ServiceActiveOnHost(ctx, service)
		if err != nil {
			Logc(ctx).WithFields(fields).Error(err)
		} else if !active {
			Logc(ctx).WithFields(fields).Debug("Required service not active, enabling...")
			err = enableAndStartServiceOnHost(ctx, service)
			if err != nil {
				Logc(ctx).WithFields(fields).Error(err)
				return err
			}
			Logc(ctx).WithFields(fields).Debug("Required service enabled.")
		} else {
			Logc(ctx).WithFields(fields).Debug("Required service already active on host.")
		}
	}
	return nil
}

func enableAndStartServiceOnHost(ctx context.Context, service string) error {

	var (
		active, enabled bool
		err             error
		output          []byte
	)

	// Re/start service
	if active, err = ServiceActiveOnHost(ctx, service); err != nil {
		return err
	} else if active {
		Logc(ctx).WithField("service", service).Debug("Restarting service.")
		output, err = execCommandWithTimeout(ctx, "systemctl", 30, true, "restart", service)
	} else {
		Logc(ctx).WithField("service", service).Debug("Starting service.")
		output, err = execCommandWithTimeout(ctx, "systemctl", 30, true, "start", service)
	}
	if err != nil {
		err = fmt.Errorf("error re/starting service; %s; %+v", string(output), err)
		return err
	}
	Logc(ctx).WithField("service", service).Debug("Service re/started.")

	// Enable service if not currently enabled
	if enabled, err = ServiceEnabledOnHost(ctx, service); err != nil {
		return err
	} else if !enabled {
		Logc(ctx).WithField("service", service).Debug("Enabling service.")
		output, err = execCommandWithTimeout(ctx, "systemctl", 30, true, "enable", service)
		if err != nil {
			err = fmt.Errorf("error enabling service; %s; %+v", string(output), err)
			return err
		}
		Logc(ctx).WithField("service", service).Debug("Service enabled.")
	}
	return nil
}

// ServiceActiveOnHost checks if the service is currently running
func ServiceActiveOnHost(ctx context.Context, service string) (bool, error) {

	Logc(ctx).Debug(">>>> osutils_linux.ServiceActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.ServiceActiveOnHost")

	output, err := execCommandWithTimeout(ctx, "systemctl", 30, true, "is-active", service)
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				Logc(ctx).WithField("service", service).Debug("Service is not active on the host.")
				return false, nil
			} else {
				err = fmt.Errorf("unexpected error while checking if service is active; %s; %+v", string(output), err)
				Logc(ctx).WithField("service", service).Error(err)
				return false, err
			}
		}
	}
	Logc(ctx).WithField("service", service).Debug("Service is active on the host.")
	return true, nil
}

// ServiceEnabledOnHost checks if the service is automatically enabled on boot
func ServiceEnabledOnHost(ctx context.Context, service string) (bool, error) {

	Logc(ctx).Debug(">>>> osutils_linux.ServiceEnabledOnHost")
	defer Logc(ctx).Debug("<<<< osutils_linux.ServiceEnabledOnHost")

	output, err := execCommandWithTimeout(ctx, "systemctl", 30, true, "is-enabled", service)
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				Logc(ctx).WithField("service", service).Debug("Service is not enabled on the host.")
				return false, nil
			} else {
				err = fmt.Errorf("unexpected error while checking if service is enabled; %s; %+v", string(output), err)
				Logc(ctx).WithField("service", service).Error(err)
				return false, err
			}
		}
	}
	Logc(ctx).WithField("service", service).Debug("Service is enabled on the host.")
	return true, nil
}

// Return information about the host system
func GetHostSystemInfo(ctx context.Context) (*HostSystem, error) {

	Logc(ctx).Debug(">>>> osutils_linux.GetHostSystemInfo")
	defer Logc(ctx).Debug("<<<< osutils_linux.GetHostSystemInfo")

	var (
		data []byte
		err  error
	)

	osInfo := sysinfo.OS{}
	msg := "Problem reading host system info."

	if RunningInContainer() {
		// Get the hosts' info via tridentctl because the sysInfo library needs to be chrooted in order to detect
		// the host OS and not the container's but chroot is irreversible and thus needs to run in a separate
		// short-lived binary
		data, err = execCommandWithTimeout(ctx, "tridentctl", 5, true, "system", "--chroot-path", "/host")
		if err != nil {
			Logc(ctx).WithField("err", err).Error(msg)
			return nil, err
		}
		err = json.Unmarshal(data, &osInfo)
		if err != nil {
			Logc(ctx).WithField("err", err).Error(msg)
			return nil, err
		}
	} else {
		// If we're not in a container, get the information directly
		var si sysinfo.SysInfo
		si.GetSysInfo()
		osInfo = si.OS
	}

	// sysInfo library is linux-only, so we must translate the data
	// into a platform agnostic struct here to return further up the stack
	host := &HostSystem{}
	host.OS.Distro = osInfo.Vendor
	host.OS.Version = osInfo.Version
	host.OS.Release = osInfo.Release
	return host, nil
}
