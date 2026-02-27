// Copyright 2026 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for linux flavor

package devices

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/netapp/trident/utils/models"
)

const (
	luksCloseTimeout                     = 30 * time.Second
	luksCloseMaxWaitDuration             = 2 * time.Minute
	luksCloseDeviceSafelyClosedExitCode  = 0
	luksCloseDeviceAlreadyClosedExitCode = 4

	scsiDeviceStateRunning = "running"
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
		Logc(ctx).WithFields(LogFields{
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

// deviceState gets the state of a SCSI device from the sysfs block SCSI device state file.
// This will not work for non-SCSI devices.
func (c *Client) deviceState(ctx context.Context, deviceName string) (string, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.deviceState")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.deviceState")

	deviceName = strings.TrimPrefix(deviceName, DevPrefix)
	if deviceName == "" {
		return "", fmt.Errorf("device name is empty")
	}

	// Read in the device state from the sysfs block SCSI device state file.
	// filePath = "/sys/block/<deviceName>/device/state"
	const sysBlockDeviceStatePath = "/sys/block/%s/device/state"
	filePath := filepath.Join(c.chrootPathPrefix, fmt.Sprintf(sysBlockDeviceStatePath, deviceName))
	out, err := c.osFs.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read device state for %s: %w", deviceName, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// deviceSize gets the size of a device by name from the Kernel's virtual block device interface.
// It returns the size in bytes of a block device as seen by the kernel.
func (c *Client) deviceSize(ctx context.Context, deviceName string) (int64, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.deviceSize")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.deviceSize")

	// Callers may supply the full device path ("/dev/dm-#", "/dev/sdX", etc.) or the device name ("dm-#", "sdX", etc.).
	deviceName = strings.TrimPrefix(deviceName, DevPrefix)
	if deviceName == "" {
		return 0, fmt.Errorf("device name is empty")
	}

	const (
		// sysBlockSizePath is the path to the sysfs block size file.
		// This file reports the number of 512-byte sectors on the device.
		sysBlockSizePath = "/sys/block/%s/size"
		// kernelSectorSize is the sector size used by the kernel to express the capacity of a device in /sys/block/<dev>/size.
		// The kernel shouuld express this value in 512-byte units regardless of the device's physical or logical block size.
		// This is a long-standing convention relied upon by userspace tools like lsblk, fdisk, and parted.
		kernelSectorSize = int64(512)
	)

	// filePath = "/sys/block/<deviceName>/size"
	filePath := filepath.Join(c.chrootPathPrefix, fmt.Sprintf(sysBlockSizePath, deviceName))
	out, err := c.osFs.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read device size for %s: %w", deviceName, err)
	}

	sectorCount, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse device size for %s: %w", deviceName, err)
	}
	if sectorCount <= 0 {
		return 0, fmt.Errorf("unexpected sector count %d for %s", sectorCount, deviceName)
	}

	return sectorCount * kernelSectorSize, nil
}

// isRunning reads a SCSI device state from the sysfs block SCSI device state file and
// returns whether the device is in a "running" state.
func (c *Client) isRunning(ctx context.Context, device string) bool {
	fields := LogFields{"device": device}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.isRunning")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.isRunning")

	deviceState, err := c.deviceState(ctx, device)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Debug("Failed to get device state; treating device as unhealthy.")
		return false
	}
	return deviceState == scsiDeviceStateRunning
}

func (c *Client) discoverUnhealthyDevices(ctx context.Context, devices []string) []string {
	fields := LogFields{"devices": devices}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.discoverUnhealthyDevices")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.discoverUnhealthyDevices")

	unhealthyDevices := make([]string, 0)
	for _, device := range devices {
		// Any non-running state is considered unhealthy.
		if !c.isRunning(ctx, device) {
			unhealthyDevices = append(unhealthyDevices, device)
		}
	}

	return unhealthyDevices
}

// rescanDevice tells the kernel to rescan a single SCSI disk/block device.
// This tells the kernel to re-query the device size by issuing a SCSI 'READ CAPACITY' command.
func (c *Client) rescanDevice(ctx context.Context, deviceName string) error {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.rescanDevice")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.rescanDevice")

	// Callers may supply the full device path or the device name.
	deviceName = strings.TrimPrefix(deviceName, DevPrefix)
	if deviceName == "" {
		return fmt.Errorf("device name is empty")
	}

	const sysBlockDeviceRescanPath = "/sys/block/%s/device/rescan"
	filePath := filepath.Join(c.chrootPathPrefix, fmt.Sprintf(sysBlockDeviceRescanPath, deviceName))
	fields["filepath"] = filePath

	file, err := c.osFs.OpenFile(filePath, os.O_WRONLY, 0)
	if err != nil {
		Logc(ctx).WithFields(fields).Warning("Could not open file for writing.")
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}

	defer func() {
		_ = file.Close()
	}()

	written, err := file.WriteString("1")
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Warn("Could not write to file.")
		return fmt.Errorf("failed to write to file %s: %w", filePath, err)
	} else if written == 0 {
		Logc(ctx).WithFields(fields).Warn("Zero bytes written to file.")
		return fmt.Errorf("no data written to %s", filePath)
	}

	return nil
}

// rescanUndersizedDevices rescans any of the supplied devices that are smaller than supplied targetSizeBytes.
// It returns nil if no rescans were needed or at least one succeeded and an error if all rescans failed.
func (c *Client) rescanUndersizedDevices(ctx context.Context, devices []string, targetSizeBytes int64) error {
	if len(devices) == 0 {
		return errors.New("no devices provided to rescan")
	}
	if targetSizeBytes <= 0 {
		return fmt.Errorf("invalid minimum size '%d': value must be greater than 0", targetSizeBytes)
	}
	fields := LogFields{
		"devices":         devices,
		"targetSizeBytes": targetSizeBytes,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.rescanUndersizedDevices")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.rescanUndersizedDevices")

	var rescanErrs error
	devicesUndersize := make([]string, 0)
	devicesRescanned := make([]string, 0)
	for _, device := range devices {
		sizeBytes, err := c.deviceSize(ctx, device)
		if err == nil && sizeBytes >= targetSizeBytes {
			continue // Already at target size.
		}

		// Read the SCSI device state for every undersized/unreadable device.
		// This is cheap (one sysfs read) and provides immediate diagnostics:
		devicesUndersize = append(devicesUndersize, device)

		// Rescan the device. Track failures separately from size-read errors.
		if scanErr := c.rescanDevice(ctx, device); scanErr != nil {
			rescanErrs = errors.Join(rescanErrs, fmt.Errorf("failed to rescan %s: %w", device, scanErr))
			continue
		}

		devicesRescanned = append(devicesRescanned, device)
	}

	// If no devices are undersized, return early.
	if len(devicesUndersize) == 0 {
		Logc(ctx).WithFields(fields).Debug("All devices at target size; no rescans needed.")
		return nil
	}

	// If some devices were undersized, but none were rescanned, return an error.
	if len(devicesRescanned) == 0 {
		Logc(ctx).WithFields(fields).WithFields(LogFields{
			"undersizedDevices": devicesUndersize,
		}).WithError(rescanErrs).Warn("All device rescans failed; connection to storage may be unstable.")
		return fmt.Errorf("no devices could be rescanned: %w", rescanErrs)
	}

	// Log partial failures so operators can see which devices stayed down,
	// even though at least one rescan succeeded and we're returning nil.
	if rescanErrs != nil {
		Logc(ctx).WithFields(fields).WithFields(LogFields{
			"undersizedDevices": devicesUndersize,
			"devicesRescanned":  devicesRescanned,
		}).WithError(rescanErrs).Warn("Some device rescans failed; connection to storage may be unstable.")
	}

	Logc(ctx).WithFields(LogFields{
		"undersizedDevices": devicesUndersize,
		"devicesRescanned":  devicesRescanned,
	}).Debug("Devices rescanned.")
	return nil
}

// resizeMultipathMap issues a multipathd resize map command to the host.
// It sends "resize map <device mapper name>" to the multipathd daemon which triggers resize_map(device)
// inside the multipathd daemon.
// If any paths are unhealthy, this command will fail with exit status 1.
func (c *Client) resizeMultipathMap(ctx context.Context, mapperName string) error {
	fields := LogFields{"mapperName": mapperName}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.resizeMultipathMap")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.resizeMultipathMap")

	// Use the single-command form of the multipathd CLI: -k<command>.
	// This must be a single argv entry so the shell doesn't split it into separate arguments.
	// "resize map <device>" → resize_map(device)
	const resizeCmd = "-kresize map %s"
	resizeDeviceCmd := fmt.Sprintf(resizeCmd, mapperName)
	out, err := c.command.ExecuteWithTimeout(ctx, "multipathd", 10*time.Second, true, resizeDeviceCmd)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"output": string(out),
		}).WithError(err).Error("Failed to resize multipath map.")
		return fmt.Errorf("failed to resize multipath map %s: %w", mapperName, err)
	}

	return nil
}

// getDeviceMapperName reads the name of a device mapper from the sysfs block dm-# device name file.
// Linux utils like multipath-tools and cryptsetup use the device mapper name to identify the device mapper.
func (c *Client) getDeviceMapperName(ctx context.Context, device string) (string, error) {
	fields := LogFields{"device": device}
	Logc(ctx).WithFields(fields).Trace(">>>> devices_linux.getDeviceMapperName")
	defer Logc(ctx).WithFields(fields).Trace("<<<< devices_linux.getDeviceMapperName")

	const sysBlockDMDeviceNamePath = "/sys/block/%s/dm/name"
	dmNamePath := filepath.Join(c.chrootPathPrefix, fmt.Sprintf(sysBlockDMDeviceNamePath, device))
	exists, err := PathExists(c.osFs, dmNamePath)
	if !exists || err != nil {
		return "", errors.NotFoundError("multipath device '%s' name not found", device)
	}

	dmNameRaw, err := c.osFs.ReadFile(dmNamePath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(dmNameRaw)), nil
}

// ExpandMultipathDevice polls the multipath device for the specified LUN until it reports a size >= targetSizeBytes.
// On each iteration it rescans any undersized SCSI paths and reloads the multipath device map,
// then confirms convergence by requiring consecutive stable size reads on the multipath device.
// It returns nil once convergence is confirmed, or an error if the context deadline is reached first.
// If the context has no deadline, a default timeout is applied to guarantee bounded execution.
func (c *Client) ExpandMultipathDevice(
	ctx context.Context, getter models.SCSIDeviceInfoGetter, targetSizeBytes int64,
) error {
	if getter == nil {
		return errors.New("device info getter is nil")
	}

	const (
		requiredStableReads  = 3
		stableReadInterval   = 3 * time.Second
		defaultResizeTimeout = 60 * time.Second
	)

	// Validate the specified minimum size.
	if targetSizeBytes <= 0 {
		return errors.New("minimum size must be greater than 0")
	}

	Logc(ctx).WithFields(LogFields{
		"targetSizeBytes": targetSizeBytes,
	}).Debug(">>>> devices_linux.ExpandMultipathDevice")
	defer Logc(ctx).Debug("<<<< devices_linux.ExpandMultipathDevice")

	// If the context doesn't have a timeout, add one.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultResizeTimeout)
		defer cancel()
	}

	var stableReads int
	var lastKnownSizeBytes int64
	var lastKnownMpathName string
	var lastKnownDeviceInfo *models.ScsiDeviceInfo
	timer := time.NewTimer(0) // First attempt fires immediately.
	defer timer.Stop()

	// This loop will continue until the context is done or the volume size has converged.
	// The size converges when the multipath device size is at or above the target size for a few consecutive reads.
	// If the context is done or the volume size has not converged after the default timeout, an error is returned.
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"multipath device '%s' size '%d' did not reach target size '%d' bytes: %w",
				lastKnownMpathName, lastKnownSizeBytes, targetSizeBytes, ctx.Err(),
			)

		case <-timer.C:
			// Schedule the next retry; the select will block until it fires or the context is done.
			timer.Reset(stableReadInterval)

			// Discover SCSI devices and multipath device for this LUN.
			// This must always happen to mitigate race conditions with freshly appearing paths and devices.
			deviceInfo, err := getter(ctx)
			if err != nil {
				Logc(ctx).WithError(err).Warn("Failed to get device information; retrying.")
				stableReads = 0
				continue
			} else if deviceInfo == nil {
				Logc(ctx).Warn("No device information found; retrying.")
				stableReads = 0
				continue
			}

			// If no multipath device is found, reset the stable read counter and try again.
			if deviceInfo.MultipathDevice == "" {
				Logc(ctx).Warn("No multipath device found for LUN; retrying.")
				stableReads = 0
				continue
			} else if len(deviceInfo.Devices) == 0 {
				Logc(ctx).Warn("No underlying SCSI devices found for LUN; retrying.")
				stableReads = 0
				continue
			}

			// Underlying paths and dm-slaves could change between convergence loop iterations due to
			// concurrently or late arriving iSCSI sessions, loss of an iSCSI session entirely, etc.
			// Track the last known device info so we're always working with the latest.
			if lastKnownDeviceInfo == nil {
				Logc(ctx).WithField(
					"deviceInfo", deviceInfo,
				).Debug("Discovered device info during multipath device expansion.")
				lastKnownDeviceInfo = deviceInfo.Copy()
			}

			// If any device info has changed reset the stable read counter and try again.
			// This can happen due to a multipath dm-# device, new device paths due to
			// sessions and paths being added or removed on the host in parallel.
			if !lastKnownDeviceInfo.Equal(deviceInfo) {
				Logc(ctx).WithFields(LogFields{
					"newDeviceInfo": deviceInfo,
					"oldDeviceInfo": lastKnownDeviceInfo,
				}).Info("Device info changed during multipath device expansion.")

				stableReads = 0
				lastKnownDeviceInfo = deviceInfo.Copy()
			}
			// Track this separately for observability.
			lastKnownMpathName = deviceInfo.MultipathDevice

			// Discover if there are any unhealthy devices.
			// If a single device is unhealthy, do not initiate rescans or resize the multipath map.
			unhealthyDevices := c.discoverUnhealthyDevices(ctx, deviceInfo.Devices)
			if len(unhealthyDevices) != 0 {
				Logc(ctx).WithFields(LogFields{
					"lunID":            deviceInfo.LUN,
					"multipathDevice":  deviceInfo.MultipathDevice,
					"allDevices":       deviceInfo.Devices,
					"unhealthyDevices": unhealthyDevices,
				}).Warn("Volume expansion cannot proceed while some devices are unhealthy; connection to storage may be unstable.")
				return fmt.Errorf("volume expansion cannot proceed; some devices %v are unhealthy", unhealthyDevices)
			}

			// Always rescan undersized dm-slaves.
			// This call is idempotent and will only rescan devices that are undersized.
			if err := c.rescanUndersizedDevices(ctx, deviceInfo.Devices, targetSizeBytes); err != nil {
				Logc(ctx).WithError(err).Warn("Failed to read or rescan devices; retrying.")
				stableReads = 0
				continue
			}

			// Read the size of the multipath device.
			mpathSizeBytes, err := c.deviceSize(ctx, deviceInfo.MultipathDevice)
			if err != nil {
				Logc(ctx).WithError(err).Warn("Failed to read multipath device size; retrying.")
				stableReads = 0
				continue
			}
			// Track this separately for observability.
			lastKnownSizeBytes = mpathSizeBytes

			// Multipath device is undersized — reconfigure the multipath device map.
			if mpathSizeBytes < targetSizeBytes {
				fields := LogFields{
					"lunID":           deviceInfo.LUN,
					"multipathDevice": deviceInfo.MultipathDevice,
					"deviceSizeBytes": mpathSizeBytes,
					"targetSizeBytes": targetSizeBytes,
				}
				Logc(ctx).WithFields(fields).Debug("Multipath device requires expansion.")

				// Get the device mapper name from the multipath device path.
				// "/dev/dm-#" -> "3600a098038314865515d4c5a70644636"
				mapperName, err := c.getDeviceMapperName(ctx, deviceInfo.MultipathDevice)
				if err != nil {
					Logc(ctx).WithFields(fields).WithError(err).Warn("Failed to get device mapper name; retrying.")
					stableReads = 0
					continue
				}

				// Resize the multipath device map.
				if err = c.resizeMultipathMap(ctx, mapperName); err != nil {
					Logc(ctx).WithFields(fields).WithError(err).Warn("Failed to resize multipath map; retrying.")
				}

				stableReads = 0
				continue
			}

			// Multipath device is at or above target size — count a stable read.
			stableReads++
			fields := LogFields{
				"stableReads":     stableReads,
				"lunID":           deviceInfo.LUN,
				"multipathDevice": deviceInfo.MultipathDevice,
				"deviceSizeBytes": mpathSizeBytes,
				"targetSizeBytes": targetSizeBytes,
			}

			// If stable reads are less than required, continue to the next iteration. Otherwise return success.
			if stableReads < requiredStableReads {
				Logc(ctx).WithFields(fields).Debug("Multipath device now at target size; re-reading size to confirm size convergence.")
				continue
			}

			Logc(ctx).WithFields(fields).Info("Multipath device expanded.")
			return nil
		}
	}
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
// It gracefully handles the cases where a LUKS device has already been closed or the device doesn't exist.
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
		fields := LogFields{"luksDevicePath": devicePath, "output": string(output), "err": err.Error()}
		var exitErr execCmd.ExitError
		if !errors.As(err, &exitErr) {
			Logc(ctx).WithFields(fields).Error("Failed to close LUKS device with unknown error.")
			return fmt.Errorf("failed to close LUKS device %s; %w", devicePath, err)
		}

		switch exitErr.ExitCode() {
		// exit code "0" and "4" are safe to ignore. "0" will likely never be hit but check for it regardless.
		case luksCloseDeviceSafelyClosedExitCode, luksCloseDeviceAlreadyClosedExitCode:
			Logc(ctx).WithFields(fields).Debug("LUKS device is already closed or did not exist.")
			return nil
		default:
			Logc(ctx).WithFields(fields).Error("Failed to close LUKS device.")
			return fmt.Errorf("exit code '%d' when closing LUKS device '%s'; %w", exitErr.ExitCode(), devicePath, err)
		}
	}

	Logc(ctx).WithField("luksDevicePath", devicePath).Debug("Closed LUKS device.")
	return nil
}

// EnsureLUKSDeviceClosed ensures there is no open LUKS device at the specified path (example: "/dev/mapper/<luks>").
func (c *Client) EnsureLUKSDeviceClosed(ctx context.Context, devicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)
	fields := LogFields{"luksDevicePath": devicePath}

	if err := c.CloseLUKSDevice(ctx, devicePath); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not close LUKS device.")
		return fmt.Errorf("could not close LUKS device %s; %w", devicePath, err)
	}

	// If LUKS close succeeded, the block device node should be gone.
	// It's the responsibility of the kernel and udev to manage /dev/mapper entries.
	// If the /dev/mapper entry lives on, log a warning and return success.
	if _, err := c.osFs.Stat(devicePath); err == nil {
		Logc(ctx).WithFields(fields).Warn("Stale device mapper file found for LUKS device. Is udev is running?")
	}

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
			Logc(ctx).WithFields(LogFields{
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
		return "", fmt.Errorf("could not find device before checking for the filesystem %v; %s", device, err)
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
