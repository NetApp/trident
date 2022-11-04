// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for linux flavor

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"

	. "github.com/netapp/trident/logger"
)

const (
	luksCommandTimeout time.Duration = time.Second * 300
)

// flushOneDevice flushes any outstanding I/O to a disk
func flushOneDevice(ctx context.Context, devicePath string) error {
	fields := log.Fields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.flushOneDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.flushOneDevice")

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

// getISCSIDiskSize queries the current block size in bytes
func getISCSIDiskSize(ctx context.Context, devicePath string) (int64, error) {
	fields := log.Fields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.getISCSIDiskSize")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.getISCSIDiskSize")

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

// LUKSDevicePath returns the location of the LUKS device when opened
func (d *LUKSDevice) LUKSDevicePath() string {
	return "/dev/mapper/" + d.luksDeviceName
}

// LUKSDeviceName returns the name of the LUKS device when opened
func (d *LUKSDevice) LUKSDeviceName() string {
	return d.luksDeviceName
}

// DevicePath returns the original device location, such as the iscsi device or multipath device. Ex: /dev/sdb
func (d *LUKSDevice) DevicePath() string {
	return d.devicePath
}

// IsLUKSFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsLUKSFormatted(ctx context.Context) (bool, error) {
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, "", "isLuks", d.DevicePath())
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			Logc(ctx).WithFields(log.Fields{
				"device": d.DevicePath(),
				"error":  err.Error(),
				"output": string(output),
			}).Debug("Failed to check if device is LUKS")
			return false, fmt.Errorf("could not check if device is LUKS: %v", err)
		}
		return false, nil
	}
	return true, nil
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	return IsLUKSDeviceOpen(ctx, d.LUKSDevicePath())
}

// LUKSFormat sets up LUKS headers on the device with the specified passphrase, this destroys data on the device
func (d *LUKSDevice) LUKSFormat(ctx context.Context, luksPassphrase string) error {
	device := d.DevicePath()
	Logc(ctx).WithFields(log.Fields{
		"devicePath": device,
	}).Debug("Formatting LUKS device")
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "luksFormat", device, "--type", "luks2", "-c", "aes-xts-plain64")
	if nil != err {
		Logc(ctx).WithFields(log.Fields{
			"device": device,
			"error":  err.Error(),
			"output": string(output),
		}).Debug("Failed to format LUKS device")
		return fmt.Errorf("could not format LUKS device; %v", err)
	}
	return nil
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	device := d.DevicePath()
	luksDeviceName := d.LUKSDeviceName()
	Logc(ctx).WithFields(log.Fields{
		"devicePath": device,
	}).Debug("Opening LUKS device")
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "open", device, luksDeviceName, "--type", "luks2")
	if nil != err {
		Logc(ctx).WithFields(log.Fields{
			"device": device,
			"error":  err.Error(),
			"output": string(output),
		}).Info("Failed to open LUKS device")

		// Exit code 2 means bad passphrase
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == 2 {
			return fmt.Errorf("no key available with this passphrase; %v", err)
		}

		return fmt.Errorf("could not open LUKS device; %v", err)
	}
	return nil
}

// Close performs a luksClose on the LUKS device
func (d *LUKSDevice) Close(ctx context.Context) error {
	// Need to Close the LUKS device
	return closeLUKSDevice(ctx, d.LUKSDevicePath())
}

// EnsureLUKSDevice ensures the specified device is LUKS formatted and opened, returns whether a luksFormat of the device was needed
func EnsureLUKSDevice(ctx context.Context, device, volumeId, luksPassphrase string) (bool, string, error) {
	luksDeviceName := "luks-" + volumeId
	luksDevice := &LUKSDevice{device, luksDeviceName}
	Logc(ctx).WithFields(log.Fields{
		"volume":     volumeId,
		"device":     device,
		"luksDevice": luksDevice.LUKSDevicePath(),
	}).Info("Opening encrypted volume")

	return ensureLUKSDevice(ctx, luksDevice, luksPassphrase)
}

// ensureLUKSDevice ensures the specified device is LUKS formatted and opened
func ensureLUKSDevice(ctx context.Context, luksDevice LUKSDeviceInterface, luksPassphrase string) (formatted bool, luksDevicePath string, err error) {
	formatted = false
	// Check if LUKS device is already opened
	isOpen, err := luksDevice.IsOpen(ctx)
	if err != nil {
		return formatted, "", err
	}
	if isOpen {
		return formatted, luksDevice.LUKSDevicePath(), nil
	}

	// Check if the device is LUKS formatted
	isLUKS, err := luksDevice.IsLUKSFormatted(ctx)
	if err != nil {
		return formatted, "", err
	}

	// Install LUKS on the device if needed
	if !isLUKS {
		// Ensure device is empty before we format it with luks
		unformatted, err := isDeviceUnformatted(ctx, luksDevice.DevicePath())
		if err != nil {
			return formatted, "", err
		}
		if !unformatted {
			return formatted, "", fmt.Errorf("cannot LUKS format device; device is not empty")
		}
		err = luksDevice.LUKSFormat(ctx, luksPassphrase)
		if err != nil {
			return formatted, "", err
		}
		formatted = true
	}

	// Open the device, creating a new LUKS encrypted device with the name chosen by us
	err = luksDevice.Open(ctx, luksPassphrase)
	if nil != err {
		return formatted, "", fmt.Errorf("could not open LUKS device; %v", err)
	}

	return formatted, luksDevice.LUKSDevicePath(), nil
}

// closeLUKSDevice performs a luksClose on the specified LUKS device
func closeLUKSDevice(ctx context.Context, luksDevicePath string) error {
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, "", "luksClose", luksDevicePath)
	if nil != err {
		log.WithFields(log.Fields{
			"LUKSDeviceName": luksDevicePath,
			"error":          err.Error(),
			"output":         string(output),
		}).Debug("Failed to Close LUKS device")
		return fmt.Errorf("failed to Close LUKS device %s; %v", luksDevicePath, err)
	}
	return nil
}

// IsLUKSDeviceOpen returns whether the specific LUKS device is currently open
func IsLUKSDeviceOpen(ctx context.Context, luksDevicePath string) (bool, error) {
	_, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, "", "status", luksDevicePath)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// EnsureLUKSDeviceClosed ensures there is not an open LUKS device at the specified path
func EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	_, err := osFs.Stat(luksDevicePath)
	if err == nil {
		// Need to Close the LUKS device
		return closeLUKSDevice(ctx, luksDevicePath)
	} else if os.IsNotExist(err) {
		Logc(ctx).WithFields(log.Fields{
			"device": luksDevicePath,
		}).Debug("LUKS device not found")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"device": luksDevicePath,
			"error":  err.Error(),
		}).Debug("Failed to stat device")
		return fmt.Errorf("could not stat device: %s %v", luksDevicePath, err)
	}
	return nil
}

// getDeviceFSType returns the filesystem for the supplied device.
func getDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.getDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices.getDeviceFSType")

	// blkid return status=2 both in case of an unformatted filesystem as well as for the case when it is
	// unable to get the filesystem (e.g. IO error), therefore ensure device is available before calling blkid
	if err := waitForDevice(ctx, device); err != nil {
		return "", fmt.Errorf("could not find device before checking for the filesystem %v; %s", device, err)
	}

	out, err := execCommandWithTimeout(ctx, "blkid", 5*time.Second, true, device)
	if err != nil {
		if IsTimeoutError(err) {
			listAllISCSIDevices(ctx)
			return "", err
		} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			// EITHER: Disk device is unformatted.
			// OR: For 'blkid', if the specified token (TYPE/PTTYPE, etc) was
			// not found, or no (specified) devices could be identified, an
			// exit code of 2 is returned.

			Logc(ctx).WithField("device", device).Infof("Could not get FSType for device; err: %v", err)
			return "", nil
		}

		Logc(ctx).WithField("device", device).Errorf("Could not determine FSType for device; err: %v", err)
		return "", err
	}

	var fsType string

	if strings.Contains(string(out), "TYPE=") {
		for _, v := range strings.Split(string(out), " ") {
			if strings.Contains(v, "TYPE=") {
				fsType = strings.Split(v, "=")[1]
				fsType = strings.Replace(fsType, "\"", "", -1)
				fsType = strings.TrimSpace(fsType)
			}
		}
	}

	if fsType == "" {
		Logc(ctx).WithField("out", string(out)).Errorf("Unable to identify fsType.")

		//  Read the device to see if it is in fact formatted
		if unformatted, err := isDeviceUnformatted(ctx, device); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"device": device,
				"err":    err,
			}).Debugf("Unable to identify if the device is not unformatted.")
		} else if !unformatted {
			Logc(ctx).WithField("device", device).Debugf("Device is not unformatted.")
			return unknownFstype, nil
		} else {
			// If we are here blkid should have not retured exit status 0, we need to retry.
			Logc(ctx).WithField("device", device).Errorf("Device is unformatted.")
		}

		return "", fmt.Errorf("unable to identify fsType")
	}

	return fsType, nil
}

// getDeviceFSTypeRetry returns the filesystem for the supplied device.
// This function will be deleted when BOF is moved to centralized retry
func getDeviceFSTypeRetry(ctx context.Context, device string) (string, error) {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.getDeviceFSTypeRetry")
	defer Logc(ctx).Debug("<<<< devices.getDeviceFSTypeRetry")

	maxDuration := multipathDeviceDiscoveryTimeoutSecs * time.Second

	checkDeviceFSType := func() error {
		_, err := getDeviceFSType(ctx, device)
		return err
	}

	FSTypeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("FS type not available yet, waiting.")
	}

	fsTypeBackoff := backoff.NewExponentialBackOff()
	fsTypeBackoff.InitialInterval = 1 * time.Second
	fsTypeBackoff.Multiplier = 1.414 // approx sqrt(2)
	fsTypeBackoff.RandomizationFactor = 0.1
	fsTypeBackoff.MaxElapsedTime = maxDuration

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkDeviceFSType, fsTypeBackoff, FSTypeNotify); err != nil {
		return "", fmt.Errorf("could not determine FS type after %3.2f seconds", maxDuration.Seconds())
	} else {
		Logc(ctx).WithField("FS type", device).Debug("Able to determine FS type.")
		fstype, err := getDeviceFSType(ctx, device)
		return fstype, err
	}
}
