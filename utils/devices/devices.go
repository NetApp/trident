// Copyright 2024 NetApp, Inc. All Rights Reserved.

//go:generate mockgen -destination=../../mocks/mock_utils/mock_devices/mock_devices_client.go github.com/netapp/trident/utils/devices Devices

package devices

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/net/context"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/durations"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

const (
	DevPrefix               = "/dev/"
	DevMapperRoot           = "/dev/mapper/"
	deviceOperationsTimeout = 5 * time.Second
)

var (
	// Non-persistent map to maintain flush delays/errors if any, for device path(s).
	volumeFlushExceptions = make(map[string]time.Time)
	maxFlushWaitDuration  = 6 * time.Minute

	duringScanTargetLunAfterFileOpen = fiji.Register("duringISCSIScanTargetLunAfterFileOpen", "iscsi")
	beforeFlushMultipathDevice       = fiji.Register("beforeFlushMultipathDevice", "devices")
	beforeRemoveFile                 = fiji.Register("beforeRemoveFile", "iscsi")

	LuksCloseDurations = durations.TimeDuration{}
)

type Devices interface {
	FlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, force bool) error
	FlushOneDevice(ctx context.Context, devicePath string) error
	EnsureDeviceReadable(ctx context.Context, device string) error
	IsDeviceUnformatted(ctx context.Context, device string) (bool, error)
	ListAllDevices(ctx context.Context)
	WaitForDevice(ctx context.Context, device string) error
	GetDeviceFSType(ctx context.Context, device string) (string, error)
	FindMultipathDeviceForDevice(ctx context.Context, device string) string
	FindDevicesForMultipathDevice(ctx context.Context, device string) []string
	VerifyMultipathDeviceSize(ctx context.Context, multipathDevice, device string) (int64, bool, error)
	GetDiskSize(ctx context.Context, devicePath string) (int64, error)
	MultipathFlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo) error
	RemoveDevice(ctx context.Context, devices []string, ignoreErrors bool) error
	VerifyMultipathDevice(
		ctx context.Context, publishInfo *models.VolumePublishInfo,
		allPublishInfos []models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
	) (bool, error)
	GetLunSerial(ctx context.Context, path string) (string, error)
	GetMultipathDeviceUUID(multipathDevicePath string) (string, error)
	GetLUKSDeviceForMultipathDevice(multipathDevice string) (string, error)
	ScanTargetLUN(ctx context.Context, lunID int, hosts []int) error
	CloseLUKSDevice(ctx context.Context, devicePath string) error
	EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error
	EnsureLUKSDeviceClosed(ctx context.Context, devicePath string) error
	RemoveMultipathDeviceMapping(ctx context.Context, devicePath string) error
	WaitForDevicesRemoval(ctx context.Context, devicePathPrefix string, deviceNames []string,
		maxWaitTime time.Duration,
	) error
}

type Client struct {
	chrootPathPrefix string
	command          exec.Command
	osFs             afero.Afero
}

func New() *Client {
	return NewDetailed(exec.NewCommand(), afero.NewOsFs())
}

func NewDetailed(command exec.Command, osFs afero.Fs) *Client {
	chrootPathPrefix := ""
	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		chrootPathPrefix = "/host"
	}
	return &Client{
		chrootPathPrefix: chrootPathPrefix,
		command:          command,
		osFs:             afero.Afero{Fs: osFs},
	}
}

// EnsureDeviceReadable reads first 4 KiBs of the device to ensures it is readable
func (c *Client) EnsureDeviceReadable(ctx context.Context, device string) error {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.EnsureDeviceReadable")
	defer Logc(ctx).Debug("<<<< devices.EnsureDeviceReadable")

	args := []string{"if=" + device, "bs=4096", "count=1", "status=none"}
	out, err := c.command.ExecuteWithTimeout(ctx, "dd", deviceOperationsTimeout, false, args...)
	if err != nil {
		Logc(ctx).WithFields(LogFields{"error": err, "device": device}).Error("failed to read the device")
		return err
	}

	// Ensure 4KiB of data read
	if len(out) != 4096 {
		Logc(ctx).WithFields(LogFields{"error": err, "device": device}).Error("read number of bytes not 4KiB")
		return fmt.Errorf("did not read 4KiB bytes from the device %v, instead read %d bytes", device, len(out))
	}

	return nil
}

// IsDeviceUnformatted reads first 2 MiBs of the device to identify if it is unformatted and contains all zeros
func (c *Client) IsDeviceUnformatted(ctx context.Context, device string) (bool, error) {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.IsDeviceUnformatted")
	defer Logc(ctx).Debug("<<<< devices.IsDeviceUnformatted")

	args := []string{"if=" + device, "bs=4096", "count=512", "status=none"}
	out, err := c.command.ExecuteWithTimeout(ctx, "dd", deviceOperationsTimeout, false, args...)
	if err != nil {
		Logc(ctx).WithFields(LogFields{"error": err, "device": device}).Error("failed to read the device")
		return false, err
	}

	// Ensure 2MiB of data read
	if len(out) != 2097152 {
		Logc(ctx).WithFields(LogFields{"error": err, "device": device}).Error("read number of bytes not 2MiB")
		return false, fmt.Errorf("did not read 2MiB bytes from the device %v, instead read %d bytes; unable to "+
			"ensure if the device is actually unformatted", device, len(out))
	}

	Logc(ctx).WithField("device", device).Debug("Verified correct number of bytes read.")

	// Ensure all zeros
	if outWithoutZeros := bytes.Trim(out, "\x00"); len(outWithoutZeros) != 0 {
		Logc(ctx).WithFields(LogFields{"error": err, "device": device}).Error("device contains non-zero values")
		return false, nil
	}

	Logc(ctx).WithFields(LogFields{"device": device}).Info("Device is unformatted.")

	return true, nil
}

// WaitForDevicesRemoval waits for devices to be removed from the system.
func (c *Client) WaitForDevicesRemoval(ctx context.Context, devicePathPrefix string, deviceNames []string,
	maxWaitTime time.Duration,
) error {
	startTime := time.Now()
	for time.Since(startTime) < maxWaitTime {
		anyExist := false
		for _, device := range deviceNames {
			path := filepath.Join(devicePathPrefix, device)
			if _, err := c.osFs.Stat(path); !os.IsNotExist(err) {
				anyExist = true
				break
			}
		}
		if !anyExist {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	Logc(ctx).WithField("devices", deviceNames).Debug("Timed out waiting for devices to be removed.")
	return errors.TimeoutError("timed out waiting for devices to be removed")
}

// canFlushMultipathDevice determines whether device can be flushed.
//  1. Check the health of path by executing 'multipath -C <devicePath>'
//  2. If no error, return nil.
//  3. Else, error or 'no usable paths found'
//     Check for maxFlushWaitDuration expired, if expired return TimeoutError.
//     else, return FlushError.
func (c *Client) canFlushMultipathDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).WithField("device", devicePath).Debug(">>>> devices.canFlushMultipathDevice")
	defer Logc(ctx).Debug("<<<< devices.canFlushMultipathDevice")

	out, err := c.command.ExecuteWithTimeout(ctx, "multipath", deviceOperationsTimeout, true, "-C", devicePath)
	if err == nil {
		delete(volumeFlushExceptions, devicePath)
		return nil
	}

	outString := string(out)
	Logc(ctx).WithFields(LogFields{
		"error":  err,
		"device": devicePath,
		"output": outString,
	}).Error("Flush pre-check failed for the device.")

	if !strings.Contains(outString, "no usable paths found") {
		return errors.ISCSIDeviceFlushError("multipath device is not ready for flush")
	}

	// Apply timeout only for the case LUN is made offline or deleted,
	// i.e., "no usable paths found" is returned on health check of the path.
	if volumeFlushExceptions[devicePath].IsZero() {
		volumeFlushExceptions[devicePath] = time.Now()
	} else {
		elapsed := time.Since(volumeFlushExceptions[devicePath])
		if elapsed > maxFlushWaitDuration {
			Logc(ctx).WithFields(
				LogFields{
					"device":  devicePath,
					"elapsed": elapsed,
					"maxWait": maxFlushWaitDuration,
				}).Debug("Volume is not safe to remove, but max flush wait time is expired, skip flush.")
			delete(volumeFlushExceptions, devicePath)
			return errors.TimeoutError(fmt.Sprintf("Max flush wait time expired. Elapsed: %v", elapsed))
		}
	}

	return errors.ISCSIDeviceFlushError("multipath device is unavailable")
}

// MultipathFlushDevice invokes the 'multipath' commands to flush paths for a single device.
func (c *Client) MultipathFlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo) error {
	Logc(ctx).WithField("device", deviceInfo.MultipathDevice).Debug(">>>> devices.MultipathFlushDevice")
	defer Logc(ctx).Debug("<<<< devices.MultipathFlushDevice")

	if deviceInfo.MultipathDevice == "" {
		return nil
	}

	devicePath := DevPrefix + deviceInfo.MultipathDevice

	deviceErr := c.canFlushMultipathDevice(ctx, devicePath)
	if deviceErr != nil {
		if errors.IsISCSIDeviceFlushError(deviceErr) {
			Logc(ctx).WithFields(
				LogFields{
					"device": devicePath,
				}).WithError(deviceErr).Debug("Flush failed.")
			return deviceErr
		}
		if errors.IsTimeoutError(deviceErr) {
			Logc(ctx).WithFields(LogFields{
				"device": devicePath,
				"lun":    deviceInfo.LUN,
				"host":   deviceInfo.Host,
			}).WithError(deviceErr).Debug("Flush timed out.")
			return deviceErr
		}
	}

	err := c.FlushOneDevice(ctx, devicePath)
	if err != nil {
		// Ideally this should not be the case, otherwise, we may need
		// to add more checks in canFlushMultipathDevice()
		return err
	}

	if err = c.RemoveMultipathDeviceMapping(ctx, devicePath); err != nil {
		Logc(ctx).WithFields(LogFields{
			"device": devicePath,
			"lun":    deviceInfo.LUN,
			"host":   deviceInfo.Host,
		}).WithError(deviceErr).Debug("Error during multipath flush.")
		return err
	}
	return nil
}

// flushDevice flushes any outstanding I/O to all paths to a device.
func (c *Client) FlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, force bool) error {
	Logc(ctx).Debug(">>>> devices.FlushDevice")
	defer Logc(ctx).Debug("<<<< devices.FlushDevice")

	for _, device := range deviceInfo.Devices {
		err := c.FlushOneDevice(ctx, DevPrefix+device)
		if err != nil && !force {
			// Return error only if this is a standalone device, i.e. no multipath device is present for this device.
			// If a multipath device exists, then it should be flushed before flushing the device,
			// hence ignore the error for this device.
			if deviceInfo.MultipathDevice == "" {
				return err
			}
		}
	}

	return nil
}

// FindDevicesForMultipathDevice finds the constituent devices for a devicemapper parent device like /dev/dm-0.
func (c *Client) FindDevicesForMultipathDevice(ctx context.Context, device string) []string {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.findDevicesForMultipathDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< devices.findDevicesForMultipathDevice")

	devices := make([]string, 0)

	slavesDir := c.chrootPathPrefix + "/sys/block/" + device + "/slaves"
	if dirs, err := os.ReadDir(slavesDir); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.HasPrefix(name, "sd") {
				devices = append(devices, name)
			}
		}
	}

	if len(devices) == 0 {
		Logc(ctx).WithField("device", device).Debug("Could not find devices for multipath device.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"device":  device,
			"devices": devices,
		}).Debug("Found devices for multipath device.")
	}

	return devices
}

// compareWithPublishedDevicePath verifies that published path matches the discovered device path
func (c *Client) compareWithPublishedDevicePath(
	ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	isProbablyGhostDevice := false
	discoverMpath := strings.TrimPrefix(deviceInfo.MultipathDevice, DevPrefix)
	publishedMpath := strings.TrimPrefix(publishInfo.DevicePath, DevPrefix)

	if discoverMpath != publishedMpath {
		// If this is the case, a wrong multipath device has been identified.
		// Reset the Multipath device and disks
		Logc(ctx).WithFields(LogFields{
			"lun":                       publishInfo.IscsiLunNumber,
			"discoveredMultipathDevice": discoverMpath,
			"publishedMultipathDevice":  publishedMpath,
		}).Debug("Discovered multipath device may not be correct.")

		deviceInfo.MultipathDevice = strings.TrimPrefix(publishedMpath, DevPrefix)
		deviceInfo.Devices = []string{}

		// Get Device based on the multipath value at the same time identify if it is a ghost device.
		devices, err := c.GetMultipathDeviceDisks(ctx, deviceInfo.MultipathDevice)
		if err != nil {
			return false, fmt.Errorf("failed to verify multipath disks for '%v'; %v ",
				deviceInfo.MultipathDevice, err)
		}

		isProbablyGhostDevice = devices == nil || len(devices) == 0
		if isProbablyGhostDevice {
			Logc(ctx).WithFields(LogFields{
				"lun":             publishInfo.IscsiLunNumber,
				"multipathDevice": deviceInfo.MultipathDevice,
			}).Debug("Multipath device may be a ghost device.")
		} else {
			deviceInfo.Devices = devices
		}

		Logc(ctx).WithFields(LogFields{
			"lun":             publishInfo.IscsiLunNumber,
			"multipathDevice": deviceInfo.MultipathDevice,
			"devices":         deviceInfo.Devices,
		}).Debug("Updated Multipath device and devices.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"lun":                       publishInfo.IscsiLunNumber,
			"publishedMultipathDevice":  publishedMpath,
			"discoveredMultipathDevice": discoverMpath,
			"devices":                   deviceInfo.Devices,
		}).Debug("Discovered multipath device is valid.")
	}

	return isProbablyGhostDevice, nil
}

// compareWithPublishedSerialNumber verifies that device serial number matches the discovered LUNs
func (c *Client) compareWithPublishedSerialNumber(
	ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	isProbablyGhostDevice := false
	lunSerialCheckPassed := false

	for _, path := range deviceInfo.DevicePaths {
		serial, err := c.GetLunSerial(ctx, path)
		if err != nil {
			// LUN either isn't scanned yet, or this kernel
			// doesn't support VPD page 80 in sysfs. Assume
			// correctness and move on
			Logc(ctx).WithError(err).WithFields(LogFields{
				"lun":  publishInfo.IscsiLunNumber,
				"path": path,
			}).Error("LUN serial check skipped.")
			continue
		}

		lunSerialCheckPassed = serial != publishInfo.IscsiLunSerial
		if lunSerialCheckPassed {
			Logc(ctx).WithFields(LogFields{
				"lun":  publishInfo.IscsiLunNumber,
				"path": path,
			}).Error("LUN serial check failed.")
			break
		}
	}

	// It means the multipath device found was wrong
	if !lunSerialCheckPassed {

		// Get Device based on the serial number and at the same time identify if it is a ghost device.
		// Multipath UUID contains LUN serial in hex format
		lunSerialHex := hex.EncodeToString([]byte(publishInfo.IscsiLunSerial))
		multipathDevice, err := c.GetMultipathDeviceBySerial(ctx, lunSerialHex)
		if err != nil {
			return false, fmt.Errorf("failed to verify multipath device for serial '%v'; %v ",
				publishInfo.IscsiLunSerial, err)
		}

		deviceInfo.MultipathDevice = strings.TrimPrefix(multipathDevice, DevPrefix)

		// Get Device based on the multipath value at the same time identify if it is a ghost device.
		devices, err := c.GetMultipathDeviceDisks(ctx, multipathDevice)
		if err != nil {
			return false, fmt.Errorf("failed to verify multipath disks for '%v', "+
				"serial '%v'; %v", multipathDevice, publishInfo.IscsiLunSerial, err)
		}

		isProbablyGhostDevice = devices == nil || len(devices) == 0
		if isProbablyGhostDevice {
			Logc(ctx).WithFields(LogFields{
				"lun":             publishInfo.IscsiLunNumber,
				"multipathDevice": multipathDevice,
			}).Debug("Multipath device may be a ghost device.")
		} else {
			deviceInfo.Devices = devices
		}
	}

	Logc(ctx).WithFields(LogFields{
		"lun":             publishInfo.IscsiLunNumber,
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
	}).Debug("Discovered multipath device and devices have valid serial number.")

	return isProbablyGhostDevice, nil
}

// compareWithAllPublishInfos comparing all publications (allPublishInfos) for
// LUN number uniqueness, if more than one publication exists with the same LUN number
// then it indicates a larger problem that user needs to manually fix
func (c *Client) compareWithAllPublishInfos(
	ctx context.Context, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) error {
	// During unstaging at least 1 publish info should exist else
	// there is some issue on the node.
	if len(allPublishInfos) < 1 {
		Logc(ctx).WithFields(LogFields{
			"lun": publishInfo.IscsiLunNumber,
		}).Debug("Missing all the publish infos; re-requesting.")

		return errors.ISCSISameLunNumberError(fmt.Sprintf(
			"failed to verify multipath device '%v' with LUN number '%v' due to missing publish infos",
			deviceInfo.MultipathDevice, publishInfo.IscsiLunNumber))
	}

	// Identify if multiple publishInfos for a given targetIQN have the same LUN Number
	var count int
	for _, info := range allPublishInfos {
		if publishInfo.IscsiLunNumber == info.IscsiLunNumber && publishInfo.IscsiTargetIQN == info.IscsiTargetIQN {
			count++
		}
	}

	if count > 1 {
		c.ListAllDevices(ctx)

		Logc(ctx).WithFields(LogFields{
			"lun":   publishInfo.IscsiLunNumber,
			"count": count,
		}).Error("Found multiple publish infos with same LUN ID.")

		return fmt.Errorf("found multiple publish infos with same LUN ID '%d'; user need to correct the publish"+
			" information by including the missing 'devicePath' based on `multipath -ll` output",
			publishInfo.IscsiLunNumber)
	}

	Logc(ctx).WithFields(LogFields{
		"lun":   publishInfo.IscsiLunNumber,
		"count": count,
	}).Debug("Found publish info with the same LUN ID.")

	return nil
}

// VerifyMultipathDevice verifies that device being removed is correct based on published device path,
// device serial number (if present) or comparing all publications (allPublishInfos) for
// LUN number uniqueness.
func (c *Client) VerifyMultipathDevice(
	ctx context.Context, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	// Ensure a correct multipath device is being discovered.
	// Following steps can be performed:
	// 1. If DM device is known, compare it with deviceInfo.MultipathDevice
	//      If no match check if the DM device is a ghost device by checking /sys/block.../slaves and remove it.
	// 2. Else if LUN SerialNumber is available, compare it with deviceInfo.Devices Serial Number
	//      If no match, find a DM device with the matching serial number,
	//      if a ghost device by checking /sys/block.../uuid then remove it.
	// 3. Else if Check all tracking infos to ensure no more than 1 tracking files have the same LUN number.
	//      If multiple are found, then it requires user intervention.

	if publishInfo.DevicePath != "" {
		return c.compareWithPublishedDevicePath(ctx, publishInfo, deviceInfo)
	} else if publishInfo.IscsiLunSerial != "" {
		return c.compareWithPublishedSerialNumber(ctx, publishInfo, deviceInfo)
	}

	return false, c.compareWithAllPublishInfos(ctx, publishInfo, allPublishInfos, deviceInfo)
}

// RemoveMultipathDeviceMapping uses "multipath -f <devicePath>" to flush(remove) unused map.
// Unused maps can happen when Unstage is called on offline/deleted LUN.
func (c *Client) RemoveMultipathDeviceMapping(ctx context.Context, devicePath string) error {
	Logc(ctx).WithField("devicePath", devicePath).Debug(">>>> devices.RemoveMultipathDevicemapping")
	defer Logc(ctx).Debug("<<<< devices.RemoveMultipathDeviceMapping")

	if devicePath == "" {
		return nil
	}
	if err := beforeFlushMultipathDevice.Inject(); err != nil {
		return err
	}
	out, err := c.command.ExecuteWithTimeout(ctx, "multipath", 10*time.Second, false, "-f", devicePath)
	if err != nil {
		pathAlreadyRemoved := strings.Contains(string(out), fmt.Sprintf("'%s' is not a valid argument", devicePath))
		if pathAlreadyRemoved {
			Logc(ctx).WithFields(LogFields{
				"output":     string(out),
				"devicePath": devicePath,
			}).WithError(err).Debug("Multipath device already removed.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"output":     string(out),
				"devicePath": devicePath,
			}).WithError(err).Error("Error encountered in multipath flush(remove) mapping command.")
			return fmt.Errorf("failed to flush multipath device: %w", err)
		}
	}

	return nil
}

// ------ TODO remove:  temporary functions to bridge the gap while we transition to the new iscsi client ------

//func GetDeviceInfoForLUN(
//	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, isDetachCall bool,
//) (*models.ScsiDeviceInfo, error) {
//	// TODO: check if we require both isDetachCall and needFSType
//	return iscsiClient.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, isDetachCall)
//}

// FindMultipathDeviceForDevice finds the devicemapper parent of a device name like /dev/sdx.
func (c *Client) FindMultipathDeviceForDevice(ctx context.Context, device string) string {
	Logc(ctx).WithField("device", device).Debug(">>>> iscsi.findMultipathDeviceForDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< iscsi.findMultipathDeviceForDevice")

	holdersDir := c.chrootPathPrefix + "/sys/block/" + device + "/holders"
	if dirs, err := c.osFs.ReadDir(holdersDir); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.HasPrefix(name, "dm-") {
				return name
			}
		}
	}

	Logc(ctx).WithField("device", device).Debug("Could not find multipath device for device.")
	return ""
}

// GetMultipathDeviceDisks find the /sys/block/dmX/slaves/sdX disks.
func (c *Client) GetMultipathDeviceDisks(
	ctx context.Context, multipathDevicePath string,
) ([]string, error) {
	devices := make([]string, 0)
	multipathDevice := strings.TrimPrefix(multipathDevicePath, "/dev/")

	diskPath := c.chrootPathPrefix + fmt.Sprintf("/sys/block/%s/slaves/", multipathDevice)
	diskDirs, err := os.ReadDir(diskPath)
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Could not read %s", diskPath)
		return nil, fmt.Errorf("failed to identify multipath device disks; unable to read '%s'", diskPath)
	}

	for _, diskDir := range diskDirs {
		contentName := diskDir.Name()
		if !strings.HasPrefix(contentName, "sd") {
			continue
		}

		devices = append(devices, contentName)
	}

	return devices, nil
}

// GetMultipathDeviceBySerial find DM device whose UUID /sys/block/dmX/dm/uuid contains serial in hex format.
func (c *Client) GetMultipathDeviceBySerial(ctx context.Context, hexSerial string) (string, error) {
	sysPath := c.chrootPathPrefix + "/sys/block/"

	blockDirs, err := os.ReadDir(sysPath)
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Could not read %s", sysPath)
		return "", fmt.Errorf("failed to find multipath device by serial; unable to read '%s'", sysPath)
	}

	for _, blockDir := range blockDirs {
		dmDeviceName := blockDir.Name()
		if !strings.HasPrefix(dmDeviceName, "dm-") {
			continue
		}

		uuid, err := c.GetMultipathDeviceUUID(dmDeviceName)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"UUID":            hexSerial,
				"multipathDevice": dmDeviceName,
				"err":             err,
			}).Error("Failed to get UUID of multipath device.")
			continue
		}

		if strings.Contains(uuid, hexSerial) {
			Logc(ctx).WithFields(LogFields{
				"UUID":            hexSerial,
				"multipathDevice": dmDeviceName,
			}).Debug("Found multipath device by UUID.")
			return dmDeviceName, nil
		}
	}

	return "", errors.NotFoundError("no multipath device found")
}

// GetMultipathDeviceUUID find the /sys/block/dmX/dm/uuid UUID that contains DM device serial in hex format.
func (c *Client) GetMultipathDeviceUUID(multipathDevicePath string) (string, error) {
	multipathDevice := strings.TrimPrefix(multipathDevicePath, "/dev/")

	deviceUUIDPath := c.chrootPathPrefix + fmt.Sprintf("/sys/block/%s/dm/uuid", multipathDevice)

	exists, err := PathExists(deviceUUIDPath)
	if !exists || err != nil {
		return "", errors.NotFoundError("multipath device '%s' UUID not found", multipathDevice)
	}

	UUID, err := os.ReadFile(deviceUUIDPath)
	if err != nil {
		return "", err
	}

	return string(UUID), nil
}

// removeDevice tells Linux that a device will be removed.
func (c *Client) RemoveDevice(ctx context.Context, devices []string, ignoreErrors bool) error {
	Logc(ctx).Debug(">>>> devices.removeDevice")
	defer Logc(ctx).Debug("<<<< devices.removeDevice")

	var (
		f   *os.File
		err error
	)

	c.ListAllDevices(ctx)
	for _, deviceName := range devices {

		filename := fmt.Sprintf(c.chrootPathPrefix+"/sys/block/%s/device/delete", deviceName)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200); err != nil {
			Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
			if ignoreErrors {
				continue
			}
			return err
		}

		if err := beforeRemoveFile.Inject(); err != nil {
			return err
		}
		if written, err := f.WriteString("1"); err != nil {
			Logc(ctx).WithFields(LogFields{"file": filename, "error": err}).Warning("Could not write to file.")
			f.Close()
			if ignoreErrors {
				continue
			}
			return err
		} else if written == 0 {
			Logc(ctx).WithField("file", filename).Warning("No data written to file.")
			f.Close()
			if ignoreErrors {
				continue
			}
			return errors.New("too few bytes written to sysfs file")
		}

		f.Close()

		Logc(ctx).WithField("scanFile", filename).Debug("Invoked device delete.")
	}
	c.ListAllDevices(ctx)

	return nil
}

// GetLunSerial get Linux's idea of what the LUN serial number is
func (c *Client) GetLunSerial(ctx context.Context, path string) (string, error) {
	Logc(ctx).WithField("path", path).Debug("Get LUN Serial")
	// We're going to read the SCSI VPD page 80 serial number
	// information. Linux helpfully provides this through sysfs
	// so we don't need to open the device and send the ioctl
	// ourselves.
	filename := path + "/vpd_pg80"
	b, err := c.osFs.ReadFile(filename)
	if err != nil {
		return "", err
	}
	if 4 > len(b) || 0x80 != b[1] {
		Logc(ctx).WithFields(LogFields{
			"data": b,
		}).Error("VPD page 80 format check failed")
		return "", fmt.Errorf("malformed VPD page 80 data")
	}
	length := int(binary.BigEndian.Uint16(b[2:4]))
	if len(b) != length+4 {
		Logc(ctx).WithFields(LogFields{
			"actual":   len(b),
			"expected": length + 4,
		}).Error("VPD page 80 length check failed")
		return "", fmt.Errorf("incorrect length for VPD page 80 serial number")
	}
	return string(b[4:]), nil
}

func (c *Client) GetLUKSDeviceForMultipathDevice(multipathDevice string) (string, error) {
	const luksDeviceUUIDPrefix = "CRYPT-LUKS2"
	const luksDeviceUUIDNameOffset = 45

	dmDevice := strings.TrimSuffix(strings.TrimPrefix(multipathDevice, "/dev/"), "/")

	// Get holder of mpath device
	dirents, err := os.ReadDir(fmt.Sprintf("/sys/block/%s/holders/", dmDevice))
	if err != nil {
		return "", err
	}

	if len(dirents) == 0 {
		return "", errors.NotFoundError("no holders found for %s", dmDevice)
	} else if len(dirents) > 1 {
		return "", fmt.Errorf("%s has %v holders; expected 1", dmDevice, len(dirents))
	}
	holder := dirents[0].Name()

	// Verify holder is LUKS device
	b, err := os.ReadFile(fmt.Sprintf("/sys/block/%s/dm/uuid", holder))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(string(b), luksDeviceUUIDPrefix) {
		return "", fmt.Errorf("%s is not a LUKS device", holder)
	}

	return DevMapperRoot + strings.TrimRight(string(b[luksDeviceUUIDNameOffset:]), "\n"), nil
}

// ScanTargetLUN scans a single LUN or all the LUNs on an iSCSI target to discover it.
// If all the LUNs are to be scanned please pass -1 for lunID.
func (c *Client) ScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	fields := LogFields{"hosts": hosts, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.scanTargetLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.scanTargetLUN")

	var (
		f   afero.File
		err error
	)

	// By default, scan for all the LUNs
	scanCmd := "0 0 -"
	if lunID >= 0 {
		scanCmd = fmt.Sprintf("0 0 %d", lunID)
	}

	c.ListAllDevices(ctx)
	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(c.chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = c.osFs.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200); err != nil {
			Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
			return err
		}

		if err = duringScanTargetLunAfterFileOpen.Inject(); err != nil {
			return err
		}

		if written, err := f.WriteString(scanCmd); err != nil {
			Logc(ctx).WithFields(LogFields{"file": filename, "error": err}).Warning("Could not write to file.")
			_ = f.Close()
			return err
		} else if written == 0 {
			Logc(ctx).WithField("file", filename).Warning("No data written to file.")
			_ = f.Close()
			return fmt.Errorf("no data written to %s", filename)
		}

		_ = f.Close()

		c.ListAllDevices(ctx)
		Logc(ctx).WithFields(LogFields{
			"scanCmd":  scanCmd,
			"scanFile": filename,
			"host":     hostNumber,
		}).Debug("Invoked SCSI scan for host.")
	}

	return nil
}
