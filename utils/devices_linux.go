// Copyright 2024 NetApp, Inc. All Rights Reserved.

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

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

const (
	luksCommandTimeout time.Duration = time.Second * 30
	luksCypherMode                   = "aes-xts-plain64"
	luksType                         = "luks2"
	// Return code for "no permission (bad passphrase)" from cryptsetup command
	luksCryptsetupBadPassphraseReturnCode = 2

	// Return codes for `cryptsetup status`
	cryptsetupStatusDeviceDoesExistStatusCode    = 0
	cryptsetupStatusDeviceDoesNotExistStatusCode = 4

	// Return codes for `cryptsetup isLuks`
	cryptsetupIsLuksDeviceIsLuksStatusCode    = 0
	cryptsetupIsLuksDeviceIsNotLuksStatusCode = 1
)

var (
	afterLuksClose                            = fiji.Register("afterLuksClose", "devices_linux")
	beforeLuksClose                           = fiji.Register("beforeLuksClose", "devices_linux")
	beforeLuksCheck                           = fiji.Register("beforeLuksCheck", "devices_linux")
	beforeBlockDeviceFlushBuffer              = fiji.Register("beforeBlockDeviceFlushBuffer", "devices_linux")
	beforeCryptSetupFormat                    = fiji.Register("beforeCryptSetupFormat", "node_server")
	duringOpenBeforeCryptSetupOpen            = fiji.Register("duringOpenBeforeCryptSetupOpen", "devices_linux")
	duringRotatePassphraseBeforeLuksKeyChange = fiji.Register("duringRotatePassphraseBeforeLuksKeyChange", "devices_linux")
)

// flushOneDevice flushes any outstanding I/O to a disk
func flushOneDevice(ctx context.Context, devicePath string) error {
	fields := LogFields{"rawDevicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.flushOneDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.flushOneDevice")

	if err := beforeBlockDeviceFlushBuffer.Inject(); err != nil {
		return err
	}

	out, err := command.ExecuteWithTimeout(
		ctx, "blockdev", deviceOperationsTimeout, true, "--flushbufs", devicePath,
	)
	if err != nil {
		Logc(ctx).WithFields(
			LogFields{
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
	fields := LogFields{"rawDevicePath": devicePath}
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

// getISCSIDiskSize queries the current block size in bytes
func getFCPDiskSize(ctx context.Context, devicePath string) (int64, error) {
	fields := LogFields{"rawDevicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.getFCPDiskSize")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.getFCPDiskSize")

	disk, err := os.Open(devicePath)
	if err != nil {
		Logc(ctx).Error("Failed to open disk.")
		return 0, fmt.Errorf("failed to open disk %s: %s", devicePath, err)
	}
	defer disk.Close()

	var size int64
	// TODO (vhs): Check if syscall can be avoided.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, disk.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		err := os.NewSyscallError("ioctl", errno)
		Logc(ctx).Error("BLKGETSIZE64 ioctl failed: ", err)
		return 0, fmt.Errorf("BLKGETSIZE64 ioctl failed %s: %s", devicePath, err)
	}

	return size, nil
}

// IsLUKSFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsLUKSFormatted(ctx context.Context) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	if d.RawDevicePath() == "" {
		return false, fmt.Errorf("no device path for LUKS device")
	}
	device := d.RawDevicePath()

	Logc(ctx).WithField("device", device).Debug("Checking if device is a LUKS device.")

	if err := beforeLuksCheck.Inject(); err != nil {
		return false, err
	}

	output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, "", "isLuks", device,
	)
	if err != nil {
		fields := LogFields{"device": device, "output": string(output)}

		// If the error isn't an exit error, then some other issue happened.
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			Logc(ctx).WithFields(fields).WithError(err).Debug("Failed to check if device is LUKS.")
			return false, fmt.Errorf("could not check if device is a LUKS device: %v", err)
		}

		// If any status code aside from "0" and "1" occur, fail. Checking for "0" is an extra safety precaution.
		status := exitError.ExitCode()
		if status != cryptsetupIsLuksDeviceIsNotLuksStatusCode && status != cryptsetupIsLuksDeviceIsLuksStatusCode {
			Logc(ctx).WithFields(fields).WithError(exitError).Debug("Failed to check if device is a LUKS device.")
			return false, fmt.Errorf("unrecognized exit error from cryptsetup isLuks; %w", exitError)
		}

		// At this point we know the device is not LUKS formatted.
		Logc(ctx).WithFields(fields).WithError(exitError).Info("Device is not a LUKS device.")
		return false, nil
	}

	Logc(ctx).WithField("device", device).Debug("Device is a LUKS device.")
	return true, nil
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	return IsLUKSDeviceOpen(ctx, d.MappedDevicePath())
}

// luksFormat sets up LUKS headers on the device with the specified passphrase, this destroys data on the device
func (d *LUKSDevice) luksFormat(ctx context.Context, luksPassphrase string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	if d.RawDevicePath() == "" {
		return fmt.Errorf("no device path for LUKS device")
	}
	device := d.RawDevicePath()

	Logc(ctx).WithField("device", device).Debug("Formatting LUKS device.")

	if err := beforeCryptSetupFormat.Inject(); err != nil {
		return err
	}

	if output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "luksFormat", device,
		"--type", "luks2", "-c", "aes-xts-plain64",
	); err != nil {
		Logc(ctx).WithFields(LogFields{
			"device": device,
			"output": string(output),
		}).WithError(err).Error("Failed to format LUKS device.")
		return fmt.Errorf("could not format LUKS device; %w", err)
	}

	return nil
}

// LUKSFormat attempts to set up LUKS headers on a device with the specified passphrase, but bails out if the
// underlying device already has a format present.
func (d *LUKSDevice) LUKSFormat(ctx context.Context, luksPassphrase string) error {
	fields := LogFields{"device": d.RawDevicePath()}
	Logc(ctx).WithFields(fields).Debug("Attempting to LUKS format device.")

	// Check if the device is already LUKS formatted.
	if luksFormatted, err := d.IsLUKSFormatted(ctx); err != nil {
		return fmt.Errorf("failed to check if device is LUKS formatted; %w", err)
	} else if luksFormatted {
		Logc(ctx).WithFields(fields).Debug("Device is already LUKS formatted.")
		return nil
	}

	// Ensure the device is empty before attempting LUKS format.
	if unformatted, err := isDeviceUnformatted(ctx, d.RawDevicePath()); err != nil {
		return fmt.Errorf("failed to check if device is unformatted; %w", err)
	} else if !unformatted {
		return fmt.Errorf("cannot LUKS format device; device is not empty")
	}

	// Attempt LUKS format.
	if err := d.luksFormat(ctx, luksPassphrase); err != nil {
		return fmt.Errorf("failed to LUKS format device; %w", err)
	}

	// At this point, the device should be LUKS formatted. If it still is not formatted, fail.
	if luksFormatted, err := d.IsLUKSFormatted(ctx); err != nil {
		return fmt.Errorf("failed to check if device is LUKS formatted; %w", err)
	} else if !luksFormatted {
		return fmt.Errorf("device is not LUKS formatted")
	}

	Logc(ctx).WithFields(fields).Debug("Device is LUKS formatted.")
	return nil
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	if d.RawDevicePath() == "" {
		return fmt.Errorf("no device path for LUKS device")
	}
	device := d.RawDevicePath()
	luksDeviceName := d.MappedDeviceName()

	fields := LogFields{"device": device, "luksDeviceName": luksDeviceName}
	Logc(ctx).WithFields(fields).Debug("Opening LUKS device.")

	if err := duringOpenBeforeCryptSetupOpen.Inject(); err != nil {
		return err
	}

	output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "open", device,
		luksDeviceName, "--type", "luks2",
	)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"device":         device,
			"luksDeviceName": luksDeviceName,
			"output":         string(output),
		}).WithError(err).Info("Failed to open LUKS device.")

		// Exit code 2 means bad passphrase
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ExitCode() == luksCryptsetupBadPassphraseReturnCode {
			return fmt.Errorf("no key available with this passphrase; %v", err)
		}

		return fmt.Errorf("could not open LUKS device; %v", err)
	}

	Logc(ctx).WithFields(fields).Debug("Opened LUKS device.")
	return nil
}

// Close performs a luksClose on the LUKS device
func (d *LUKSDevice) Close(ctx context.Context) error {
	// Need to Close the LUKS device
	return closeLUKSDevice(ctx, d.MappedDevicePath())
}

// closeLUKSDevice performs a luksClose on the device at the specified path (example: "/dev/mapper/<luks>").
func closeLUKSDevice(ctx context.Context, devicePath string) error {
	if err := beforeLuksClose.Inject(); err != nil {
		return err
	}

	output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, "", "luksClose", devicePath,
	)

	if err := afterLuksClose.Inject(); err != nil {
		return err
	}

	if err != nil {
		fields := LogFields{"luksDevicePath": devicePath, "output": string(output)}
		Logc(ctx).WithFields(fields).WithError(err).Debug("Failed to close LUKS device")
		return fmt.Errorf("failed to close LUKS device %s; %w", devicePath, err)
	}

	Logc(ctx).WithField("luksDevicePath", devicePath).Debug("Closed LUKS device.")
	return nil
}

// IsLUKSDeviceOpen returns whether the device at the specified path is an open LUKS device.
func IsLUKSDeviceOpen(ctx context.Context, devicePath string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).WithField("devicePath", devicePath).Debug("Checking if device is an open LUKS device.")
	output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, "", "status", devicePath,
	)
	if err != nil {
		fields := LogFields{"devicePath": devicePath, "output": string(output)}

		exitError, ok := err.(*exec.ExitError)
		if !ok {
			return false, err
		}

		switch exitError.ExitCode() {
		case cryptsetupStatusDeviceDoesNotExistStatusCode:
			// Status code 4 from `cryptsetup status` means the device is not open.
			Logc(ctx).WithFields(fields).WithError(err).Debug("LUKS device does not exist.")
			return false, errors.NotFoundError("device at [%s] does not exist; %v", devicePath, exitError)
		case cryptsetupStatusDeviceDoesExistStatusCode:
			// Status code 0 from `cryptsetup status` means this LUKS device has been formatted and opened before.
			// Low probability that this will be hit, but keep it in as a safety precaution.
			Logc(ctx).WithFields(fields).WithError(err).Debug("Encountered error but device is an open LUKS device.")
			return true, nil
		default:
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to check if device is open.")
			return false, fmt.Errorf("failed to check if device is open: [%s]; %v", devicePath, err)
		}
	}

	Logc(ctx).WithField("devicePath", devicePath).Debug("Device is an open LUKS device.")
	return true, nil
}

// EnsureLUKSDeviceClosed ensures there is no open LUKS device at the specified path (example: "/dev/mapper/<luks>").
func EnsureLUKSDeviceClosed(ctx context.Context, devicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)
	_, err := osFs.Stat(devicePath)
	if err == nil {
		return closeLUKSDevice(ctx, devicePath)
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(LogFields{
			"device": devicePath,
			"error":  err.Error(),
		}).Debug("Failed to stat device")
		return fmt.Errorf("could not stat device: %s %v.", devicePath, err)
	}
	Logc(ctx).WithFields(LogFields{
		"device": devicePath,
	}).Debug("LUKS device not found.")

	return nil
}

func EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error {
	if err := EnsureLUKSDeviceClosed(ctx, luksDevicePath); err != nil {
		LuksCloseDurations.InitStartTime(luksDevicePath)
		elapsed, durationErr := LuksCloseDurations.GetCurrentDuration(luksDevicePath)
		if durationErr != nil {
			return durationErr
		}
		if elapsed > luksCloseMaxWaitDuration {
			Logc(ctx).WithFields(
				LogFields{
					"device":  luksDevicePath,
					"elapsed": elapsed,
					"maxWait": luksDevicePath,
				}).Debug("LUKS close max wait time expired, continuing with removal.")
			return errors.MaxWaitExceededError(fmt.Sprintf("LUKS close wait time expired. Elapsed: %v", elapsed))
		}
		return err
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

	out, err := command.ExecuteWithTimeout(ctx, "blkid", 5*time.Second, true, device)
	if err != nil {
		if errors.IsTimeoutError(err) {
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
			Logc(ctx).WithFields(LogFields{
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

// RotatePassphrase changes the passphrase in passphrase slot 1, in place, to the specified passphrase.
func (d *LUKSDevice) RotatePassphrase(
	ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string,
) error {
	if d.RawDevicePath() == "" {
		return fmt.Errorf("no device path for LUKS device")
	}
	Logc(ctx).WithFields(LogFields{
		"volume":           volumeId,
		"device":           d.RawDevicePath(),
		"mappedDevicePath": d.MappedDevicePath(),
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
		Log().WithFields(LogFields{
			"error": err,
		}).Error("Failed to create passphrase file for LUKS.")
		return fmt.Errorf("failed to create passphrase files for LUKS; %v", err)
	}
	defer unix.Close(fd)

	// Rely on Linux's ability to reference already-open files with /dev/fd/*
	oldKeyFilename := fmt.Sprintf("/dev/fd/%d", fd)

	if err = duringRotatePassphraseBeforeLuksKeyChange.Inject(); err != nil {
		return err
	}

	output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "luksChangeKey", "-d",
		oldKeyFilename, d.RawDevicePath(),
	)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"device": d.RawDevicePath(),
			"output": string(output),
		}).WithError(err).Error("Error changing LUKS passphrase.")
		return fmt.Errorf("could not change LUKS passphrase; %v", err)
	}
	Logc(ctx).WithFields(LogFields{
		"volume":           volumeId,
		"device":           d.RawDevicePath(),
		"mappedDevicePath": d.MappedDevicePath(),
	}).Info("Rotated LUKS passphrase for encrypted volume.")
	return nil
}

// Resize performs a luksResize on the LUKS device
func (d *LUKSDevice) Resize(ctx context.Context, luksPassphrase string) error {
	output, err := command.ExecuteWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true,
		luksPassphrase, "resize", d.MappedDevicePath(),
	)
	if nil != err {
		log.WithFields(log.Fields{
			"MappedDevicePath": d.MappedDevicePath(),
			"error":            err.Error(),
			"output":           string(output),
		}).Debug("Failed to resize LUKS device")

		// Exit code 2 means bad passphrase
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == luksCryptsetupBadPassphraseReturnCode {
			return errors.IncorrectLUKSPassphraseError(fmt.Sprintf("no key available with this passphrase; %v", err))
		}
		return fmt.Errorf("failed to resize LUKS device %s; %v", d.MappedDevicePath(), err)
	}
	return nil
}

// GetUnderlyingDevicePathForLUKSDevice returns the device mapped to the LUKS device
// uses cryptsetup status <luks-device> and parses the output
func GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, luksDevicePath string) (string, error) {
	out, err := command.ExecuteWithTimeoutAndInput(ctx, "cryptsetup", luksCommandTimeout, true,
		"", "status", luksDevicePath,
	)

	output := string(out)
	if err != nil {
		return "", err
	} else if !strings.Contains(output, "device:") {
		return "", fmt.Errorf("cryptsetup status command output does not contain a device entry")
	}

	var devicePath string
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "device:") {
			continue
		}

		// If the "device: ..." line from cryptsetup must be able to be broken up into at least 2 fields.
		// line: " device:  /dev/mapper/3600a09807770457a795d526950374c76"
		// pieces: ["device: "," /dev/mapper/3600a09807770457a795d526950374c76"]
		pieces := strings.Fields(line)
		if len(pieces) != 2 {
			break
		}

		devicePath = strings.TrimSpace(pieces[1])
		break
	}

	if devicePath == "" {
		return "", fmt.Errorf("failed to get underlying device path from device: %s", luksDevicePath)
	}

	return devicePath, nil
}

// CheckPassphrase returns whether the specified passphrase string is the current LUKS passphrase for the device
func (d *LUKSDevice) CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error) {
	device := d.RawDevicePath()
	luksDeviceName := d.MappedDeviceName()
	output, err := command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCommandTimeout, true, luksPassphrase, "open", device,
		luksDeviceName, "--type", "luks2", "--test-passphrase",
	)
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == luksCryptsetupBadPassphraseReturnCode {
			return false, nil
		}
		Logc(ctx).WithError(err).Errorf("Cryptsetup command failed, output: %s", output)
		return false, err
	}
	return true, nil
}
