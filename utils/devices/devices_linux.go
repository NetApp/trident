// Copyright 2025 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for linux flavor

package devices

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/net/context"
	"golang.org/x/sys/unix"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
)

const (
	luksCloseTimeout         = 30 * time.Second
	luksCloseMaxWaitDuration = 2 * time.Minute
)

var (
	afterLuksClose               = fiji.Register("afterLuksClose", "devices_linux")
	beforeLuksClose              = fiji.Register("beforeLuksClose", "devices_linux")
	beforeBlockDeviceFlushBuffer = fiji.Register("beforeBlockDeviceFlushBuffer", "devices_linux")
)

// FlushOneDevice flushes any outstanding I/O to a disk
func (c *Client) FlushOneDevice(ctx context.Context, devicePath string) error {
	fields := LogFields{"rawDevicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.flushOneDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.flushOneDevice")

	if err := beforeBlockDeviceFlushBuffer.Inject(); err != nil {
		return err
	}

	out, err := c.command.ExecuteWithTimeout(
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

// GetDiskSize queries the current block size in bytes
func (c *DiskSizeGetter) GetDiskSize(ctx context.Context, devicePath string) (int64, error) {
	fields := LogFields{"rawDevicePath": devicePath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices_linux.GetDiskSize")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices_linux.GetDiskSize")

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

// VerifyMultipathDeviceSize compares the size of the DM device with the size
// of a device to ensure correct DM device has the correct size.
func (c *Client) VerifyMultipathDeviceSize(
	ctx context.Context, multipathDevice, device string,
) (int64, bool, error) {
	deviceSize, err := c.GetDiskSize(ctx, "/dev/"+device)
	if err != nil {
		return 0, false, err
	}

	mpathSize, err := c.GetDiskSize(ctx, "/dev/"+multipathDevice)
	if err != nil {
		return 0, false, err
	}

	if deviceSize != mpathSize {
		return deviceSize, false, nil
	}

	Logc(ctx).WithFields(LogFields{
		"multipathDevice": multipathDevice,
		"device":          device,
	}).Debug("Multipath device size check passed.")

	return 0, true, nil
}

// CloseLUKSDevice performs a luksClose on the device at the specified path (example: "/dev/mapper/<luks>").
func (c *Client) CloseLUKSDevice(ctx context.Context, devicePath string) error {
	if err := beforeLuksClose.Inject(); err != nil {
		return err
	}

	output, err := c.command.ExecuteWithTimeoutAndInput(
		ctx, "cryptsetup", luksCloseTimeout, true, "", "luksClose", devicePath,
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

// EnsureLUKSDeviceClosed ensures there is no open LUKS device at the specified path (example: "/dev/mapper/<luks>").
func (c *Client) EnsureLUKSDeviceClosed(ctx context.Context, devicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)
	_, err := c.osFs.Stat(devicePath)
	if err == nil {
		return c.CloseLUKSDevice(ctx, devicePath)
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(LogFields{
			"device": devicePath,
			"error":  err.Error(),
		}).Debug("Failed to stat device.")
		return fmt.Errorf("could not stat device: %s; %v", devicePath, err)
	}
	Logc(ctx).WithFields(LogFields{
		"device": devicePath,
	}).Debug("LUKS device not found.")

	return nil
}

func (c *Client) EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error {
	if err := c.EnsureLUKSDeviceClosed(ctx, luksDevicePath); err != nil {
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

// GetDeviceFSType returns the filesystem for the supplied device.
func (c *Client) GetDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.getDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices.getDeviceFSType")

	// blkid return status=2 both in case of an unformatted filesystem as well as for the case when it is
	// unable to get the filesystem (e.g. IO error), therefore ensure device is available before calling blkid
	if err := c.WaitForDevice(ctx, device); err != nil {
		return "", fmt.Errorf("could not find device before checking for the filesystem %v; %s.", device, err)
	}

	out, err := c.command.ExecuteWithTimeout(ctx, "blkid", 5*time.Second, true, device)
	if err != nil {
		if errors.IsTimeoutError(err) {
			c.ListAllDevices(ctx)
			return "", err
		} else if exitErr, ok := err.(execCmd.ExitError); ok && exitErr.ExitCode() == 2 {
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
		if unformatted, err := c.IsDeviceUnformatted(ctx, device); err != nil {
			Logc(ctx).WithFields(LogFields{
				"device": device,
				"err":    err,
			}).Debugf("Unable to identify if the device is not unformatted.")
		} else if !unformatted {
			Logc(ctx).WithField("device", device).Debugf("Device is not unformatted.")
			return filesystem.UnknownFstype, nil
		} else {
			// If we are here blkid should have not retured exit status 0, we need to retry.
			Logc(ctx).WithField("device", device).Errorf("Device is unformatted.")
		}

		return "", errors.New("unable to identify fsType")
	}

	return fsType, nil
}

// ListAllDevices logs info about session and what devices are present
func (c *Client) ListAllDevices(ctx context.Context) {
	Logc(ctx).Trace(">>>> devices.ListAllDevices")
	defer Logc(ctx).Trace("<<<< devices.ListAllDevices")
	// Log information about all the devices
	dmLog := make([]string, 0)
	sdLog := make([]string, 0)
	sysLog := make([]string, 0)
	entries, _ := c.osFs.ReadDir(DevPrefix)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "dm-") {
			dmLog = append(dmLog, entry.Name())
		}
		if strings.HasPrefix(entry.Name(), "sd") {
			sdLog = append(sdLog, entry.Name())
		}
	}

	entries, _ = c.osFs.ReadDir("/sys/block/")
	for _, entry := range entries {
		sysLog = append(sysLog, entry.Name())
	}

	// TODO: Call this only when verbose logging requires beyond debug level.
	// out1, _ := command.ExecuteWithTimeout(ctx, "multipath", deviceOperationsTimeout, true, "-ll")
	// out2, _ := execIscsiadmCommand(ctx, "-m", "session")
	Logc(ctx).WithFields(LogFields{
		"/dev/dm-*":    dmLog,
		"/dev/sd*":     sdLog,
		"/sys/block/*": sysLog,
		//	"multipath -ll output":       string(out1),
		//	"iscsiadm -m session output": string(out2),
	}).Trace("Listing all devices.")
}

// WaitForDevice accepts a device name and checks if it is present
func (c *Client) WaitForDevice(ctx context.Context, device string) error {
	fields := LogFields{"device": device}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.waitForDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.waitForDevice")

	exists, err := PathExists(c.osFs, device)
	if !exists || err != nil {
		return errors.New("device not yet present")
	} else {
		Logc(ctx).WithField("device", device).Debug("Device found.")
	}
	return nil
}
