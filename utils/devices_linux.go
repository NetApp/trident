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
	luksCypherMode                   = "aes-xts-plain64"
	luksType                         = "luks2"
	// Return code for "no permission (bad passphrase)" from cryptsetup command
	luksCryptsetupBadPassphraseReturnCode = 2
)

// flushOneDevice flushes any outstanding I/O to a disk
func flushOneDevice(ctx context.Context, devicePath string) error {
	fields := log.Fields{"rawDevicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.flushOneDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.flushOneDevice")

	out, err := execCommandWithTimeout(ctx, "blockdev", deviceOperationsTimeout, true, "--flushbufs", devicePath)
	if err != nil {
		Logc(ctx).WithFields(
			log.Fields{
				"error":  err,
				"output": string(out),
				"device": devicePath,
			}).Debug("blockdev --flushbufs failed.")
		return fmt.Errorf("flush device failed for %s : %s", devicePath, err)
	}

	return nil
}

// getISCSIDiskSize queries the current block size in bytes
func getISCSIDiskSize(ctx context.Context, devicePath string) (int64, error) {
	fields := log.Fields{"rawDevicePath": devicePath}
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

// IsLUKSFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsLUKSFormatted(ctx context.Context) (bool, error) {
	if d.RawDevicePath() == "" {
		return false, fmt.Errorf("no device path for LUKS device")
	}
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, "", "isLuks", d.RawDevicePath())
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			Logc(ctx).WithFields(log.Fields{
				"device": d.RawDevicePath(),
				"error":  err.Error(),
				"output": string(output),
			}).Debug("Failed to check if device is LUKS.")
			return false, fmt.Errorf("could not check if device is LUKS: %v", err)
		}
		return false, nil
	}
	return true, nil
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	return IsLUKSDeviceOpen(ctx, d.MappedDevicePath())
}

// LUKSFormat sets up LUKS headers on the device with the specified passphrase, this destroys data on the device
func (d *LUKSDevice) LUKSFormat(ctx context.Context, luksPassphrase string) error {
	if d.RawDevicePath() == "" {
		return fmt.Errorf("no device path for LUKS device")
	}
	device := d.RawDevicePath()
	Logc(ctx).WithFields(log.Fields{
		"rawDevicePath": device,
	}).Debug("Formatting LUKS device.")
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "luksFormat", device, "--type", "luks2", "-c", "aes-xts-plain64")
	if nil != err {
		Logc(ctx).WithFields(log.Fields{
			"device": device,
			"error":  err.Error(),
			"output": string(output),
		}).Debug("Failed to format LUKS device.")
		return fmt.Errorf("could not format LUKS device; %v", err)
	}
	return nil
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	if d.RawDevicePath() == "" {
		return fmt.Errorf("no device path for LUKS device")
	}
	device := d.RawDevicePath()
	luksDeviceName := d.MappedDeviceName()
	Logc(ctx).WithFields(log.Fields{
		"rawDevicePath": device,
	}).Debug("Opening LUKS device.")
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "open", device, luksDeviceName, "--type", "luks2")
	if nil != err {
		Logc(ctx).WithFields(log.Fields{
			"device": device,
			"error":  err.Error(),
			"output": string(output),
		}).Info("Failed to open LUKS device.")

		// Exit code 2 means bad passphrase
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == luksCryptsetupBadPassphraseReturnCode {
			return fmt.Errorf("no key available with this passphrase; %v", err)
		}

		return fmt.Errorf("could not open LUKS device; %v", err)
	}
	return nil
}

// Close performs a luksClose on the LUKS device
func (d *LUKSDevice) Close(ctx context.Context) error {
	// Need to Close the LUKS device
	return closeLUKSDevice(ctx, d.MappedDevicePath())
}

// closeLUKSDevice performs a luksClose on the specified LUKS device
func closeLUKSDevice(ctx context.Context, luksDevicePath string) error {
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, "", "luksClose",
		luksDevicePath)
	if nil != err {
		log.WithFields(log.Fields{
			"MappedDeviceName": luksDevicePath,
			"error":            err.Error(),
			"output":           string(output),
		}).Debug("Failed to Close LUKS device.")
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
		}).Debug("LUKS device not found.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"device": luksDevicePath,
			"error":  err.Error(),
		}).Debug("Failed to stat device")
		return fmt.Errorf("could not stat device: %s %v.", luksDevicePath, err)
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
		return "", fmt.Errorf("could not find device before checking for the filesystem %v; %s.", device, err)
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

			Logc(ctx).WithField("device", device).Infof("Could not get FSType for device; err: %v.", err)
			return "", nil
		}

		Logc(ctx).WithField("device", device).Errorf("Could not determine FSType for device; err: %v.", err)
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

// RotatePassphrase changes the passphrase in passphrase slot 1, in place, to the specified passphrase.
func (d *LUKSDevice) RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error {
	if d.RawDevicePath() == "" {
		return fmt.Errorf("no device path for LUKS device")
	}
	Logc(ctx).WithFields(log.Fields{
		"volume":           volumeId,
		"device":           d.RawDevicePath(),
		"MappedDevicePath": d.MappedDevicePath(),
	}).Info("Rotating LUKS passphrase for encrypted volume.")

	// make sure the new passphrase is valid
	if previousLUKSPassphrase == "" {
		return fmt.Errorf("previous LUKS passphrase is empty")
	}
	if luksPassphrase == "" {
		return fmt.Errorf("new LUKS passphrase is empty")
	}

	// Write the old passphrase to an anonymous file in memory because we can't provide it on stdin
	tempFileName := fmt.Sprintf("luks_key_%s", volumeId)
	fd, err := generateAnonymousMemFile(tempFileName, previousLUKSPassphrase)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to create passphrase file for LUKS.")
		return fmt.Errorf("failed to create passphrase files for LUKS; %v", err)
	}
	defer unix.Close(fd)

	// Rely on Linux's ability to reference already-open files with /dev/fd/*
	oldKeyFilename := fmt.Sprintf("/dev/fd/%d", fd)

	_, err = execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "luksChangeKey", "-d", oldKeyFilename, d.RawDevicePath())
	if err != nil {
		return fmt.Errorf("could not change LUKS passphrase; %v", err)
	}
	Logc(ctx).WithFields(log.Fields{
		"volume":           volumeId,
		"device":           d.RawDevicePath(),
		"MappedDevicePath": d.MappedDevicePath(),
	}).Info("Rotated LUKS passphrase for encrypted volume.")
	return nil
}

// Resize performs a luksResize on the LUKS device
func (d *LUKSDevice) Resize(ctx context.Context, luksPassphrase string) error {
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "resize",
		d.MappedDevicePath())
	if nil != err {
		log.WithFields(log.Fields{
			"MappedDevicePath": d.MappedDevicePath(),
			"error":            err.Error(),
			"output":           string(output),
		}).Debug("Failed to resize LUKS device")

		// Exit code 2 means bad passphrase
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == 2 {
			return IncorrectLUKSPassphraseError(fmt.Sprintf("no key available with this passphrase; %v", err))
		}
		return fmt.Errorf("failed to resize LUKS device %s; %v", d.MappedDevicePath(), err)
	}
	return nil
}

// GetUnderlyingDevicePathForLUKSDevice returns the device mapped to the LUKS device
// uses cryptsetup status <luks-device> and parses the output
func GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, luksDevicePath string) (string, error) {
	output, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, "", "status", luksDevicePath)
	if err != nil {
		return "", err
	}

	var devicePath string
	if strings.Contains(string(output), "device:") {
		for _, v := range strings.Split(string(output), "\n") {
			if strings.Contains(v, "device:") {
				if len(strings.Fields(v)) != 2 {
					break
				}
				devicePath = strings.Fields(v)[1]
				devicePath = strings.TrimSpace(devicePath)
				break
			}
		}
	}

	if devicePath == "" {
		return "", fmt.Errorf("cryptsetup status command output does not contain a device")
	}
	return devicePath, nil
}

// CheckPassphrase returns whether the specified passphrase string is the current LUKS passphrase for the device
func (d *LUKSDevice) CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error) {
	device := d.RawDevicePath()
	luksDeviceName := d.MappedDeviceName()
	_, err := execCommandWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "open", device, luksDeviceName, "--type", "luks2", "--test-passphrase")
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == luksCryptsetupBadPassphraseReturnCode {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
