// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/durations"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

// LUKS2 requires ~16MiB for overhead. Default to 18MiB just in case.
const LUKSMetadataSize = 18874368

const (
	luksDevicePrefix = "luks-"
)

var (
	beforeFlushMultipathDevice = fiji.Register("beforeFlushMultipathDevice", "devices")
	beforeFlushDevice          = fiji.Register("beforeFlushDevice", "devices")
	beforeRemoveFile           = fiji.Register("beforeRemoveFile", "devices")

	LuksCloseDurations = durations.TimeDuration{}
)

// waitForDevice accepts a device name and checks if it is present
func waitForDevice(ctx context.Context, device string) error {
	fields := LogFields{"device": device}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.waitForDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.waitForDevice")

	exists, err := PathExists(device)
	if !exists || err != nil {
		return errors.New("device not yet present")
	} else {
		Logc(ctx).WithField("device", device).Debug("Device found.")
	}
	return nil
}

// flushDevice flushes any outstanding I/O to all paths to a device.
func flushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, force bool) error {
	Logc(ctx).Debug(">>>> devices.flushDevice")
	defer Logc(ctx).Debug("<<<< devices.flushDevice")

	for _, device := range deviceInfo.Devices {
		err := flushOneDevice(ctx, iscsi.DevPrefix+device)
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

// ensureDeviceReadable reads first 4 KiBs of the device to ensures it is readable
func ensureDeviceReadable(ctx context.Context, device string) error {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.ensureDeviceReadable")
	defer Logc(ctx).Debug("<<<< devices.ensureDeviceReadable")

	args := []string{"if=" + device, "bs=4096", "count=1", "status=none"}
	out, err := command.ExecuteWithTimeout(ctx, "dd", deviceOperationsTimeout, false, args...)
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

// isDeviceUnformatted reads first 2 MiBs of the device to identify if it is unformatted and contains all zeros
func isDeviceUnformatted(ctx context.Context, device string) (bool, error) {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.isDeviceUnformatted")
	defer Logc(ctx).Debug("<<<< devices.isDeviceUnformatted")

	args := []string{"if=" + device, "bs=4096", "count=512", "status=none"}
	out, err := command.ExecuteWithTimeout(ctx, "dd", deviceOperationsTimeout, false, args...)
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

func GetDeviceNameFromMount(ctx context.Context, mountpath string) (string, int, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	fields := LogFields{"mountpath": mountpath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.GetDeviceNameFromMount")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.GetDeviceNameFromMount")

	mps, err := listProcMounts(procMountsPath)
	if err != nil {
		return "", 0, err
	}

	// Find the device name.
	// FIXME if multiple devices mounted on the same mount path, only the first one is returned
	device := ""
	// If mountPath is symlink, need get its target path.
	slTarget, err := filepath.EvalSymlinks(mountpath)
	if err != nil {
		slTarget = mountpath
	}
	for i := range mps {
		if mps[i].Path == slTarget {
			device = mps[i].Device
			break
		}
	}

	// Find all references to the device.
	refCount := 0
	for i := range mps {
		if mps[i].Device == device {
			refCount++
		}
	}

	Logc(ctx).WithFields(LogFields{
		"mountpath": mountpath,
		"device":    device,
		"refCount":  refCount,
	}).Debug("Found device from mountpath.")

	return device, refCount, nil
}

// removeDevice tells Linux that a device will be removed.
func removeDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, ignoreErrors bool) error {
	Logc(ctx).Debug(">>>> devices.removeDevice")
	defer Logc(ctx).Debug("<<<< devices.removeDevice")

	var (
		f   *os.File
		err error
	)

	listAllISCSIDevices(ctx)
	for _, deviceName := range deviceInfo.Devices {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/block/%s/device/delete", deviceName)
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
	listAllISCSIDevices(ctx)

	return nil
}

// canFlushMultipathDevice determines whether device can be flushed.
//  1. Check the health of path by executing 'multipath -C <devicePath>'
//  2. If no error, return nil.
//  3. Else, error or 'no usable paths found'
//     Check for maxFlushWaitDuration expired, if expired return TimeoutError.
//     else, return FlushError.
func canFlushMultipathDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).WithField("device", devicePath).Debug(">>>> devices.canFlushMultipathDevice")
	defer Logc(ctx).Debug("<<<< devices.canFlushMultipathDevice")

	out, err := command.ExecuteWithTimeout(ctx, "multipath", deviceOperationsTimeout, true, "-C", devicePath)
	if err == nil {
		delete(iSCSIVolumeFlushExceptions, devicePath)
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
	if iSCSIVolumeFlushExceptions[devicePath].IsZero() {
		iSCSIVolumeFlushExceptions[devicePath] = time.Now()
	} else {
		elapsed := time.Since(iSCSIVolumeFlushExceptions[devicePath])
		if elapsed > iSCSIMaxFlushWaitDuration {
			Logc(ctx).WithFields(
				LogFields{
					"device":  devicePath,
					"elapsed": elapsed,
					"maxWait": iSCSIMaxFlushWaitDuration,
				}).Debug("Volume is not safe to remove, but max flush wait time is expired, skip flush.")
			delete(iSCSIVolumeFlushExceptions, devicePath)
			return errors.TimeoutError(fmt.Sprintf("Max flush wait time expired. Elapsed: %v", elapsed))
		}
	}

	return errors.ISCSIDeviceFlushError("multipath device is unavailable")
}

// multipathFlushDevice invokes the 'multipath' commands to flush paths for a single device.
func multipathFlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo) error {
	Logc(ctx).WithField("device", deviceInfo.MultipathDevice).Debug(">>>> devices.multipathFlushDevice")
	defer Logc(ctx).Debug("<<<< devices.multipathFlushDevice")

	if deviceInfo.MultipathDevice == "" {
		return nil
	}

	devicePath := iscsi.DevPrefix + deviceInfo.MultipathDevice

	deviceErr := canFlushMultipathDevice(ctx, devicePath)
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

	err := flushOneDevice(ctx, devicePath)
	if err != nil {
		// Ideally this should not be the case, otherwise, we may need
		// to add more checks in canFlushMultipathDevice()
		return err
	}

	if err = RemoveMultipathDeviceMapping(ctx, devicePath); err != nil {
		Logc(ctx).WithFields(LogFields{
			"device": devicePath,
			"lun":    deviceInfo.LUN,
			"host":   deviceInfo.Host,
		}).WithError(deviceErr).Debug("Error during multipath flush.")
		return err
	}
	return nil
}

// GetMountedISCSIDevices returns a list of iSCSI devices that are *mounted* on this host.
func GetMountedISCSIDevices(ctx context.Context) ([]*models.ScsiDeviceInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices.GetMountedISCSIDevices")
	defer Logc(ctx).Debug("<<<< devices.GetMountedISCSIDevices")

	procSelfMountinfo, err := listProcMountinfo(procSelfMountinfoPath)
	if err != nil {
		return nil, err
	}

	// Get a list of all mounted /dev devices
	mountedDevices := make([]string, 0)
	for _, procMount := range procSelfMountinfo {

		hasDevMountSourcePrefix := strings.HasPrefix(procMount.MountSource, iscsi.DevPrefix)
		hasPvcMountPoint := strings.Contains(procMount.MountPoint, "/pvc-")

		if !hasPvcMountPoint {
			continue
		}

		var mountedDevice string
		// Resolve any symlinks to get the real device
		if hasDevMountSourcePrefix {
			device, err := filepath.EvalSymlinks(procMount.MountSource)
			if err != nil {
				Logc(ctx).Error(err)
				continue
			}
			mountedDevice = strings.TrimPrefix(device, iscsi.DevPrefix)
		} else {
			mountedDevice = strings.TrimPrefix(procMount.Root, "/")
		}

		mountedDevices = append(mountedDevices, mountedDevice)
	}

	// Get all known iSCSI devices
	iscsiDevices, err := GetISCSIDevices(ctx, false)
	if err != nil {
		return nil, err
	}

	mountedISCSIDevices := make([]*models.ScsiDeviceInfo, 0)

	// For each mounted device, look for a matching iSCSI device
	for _, mountedDevice := range mountedDevices {
	iSCSIDeviceLoop:
		for _, iscsiDevice := range iscsiDevices {
			// First look for a multipath device match
			if mountedDevice == iscsiDevice.MultipathDevice {
				mountedISCSIDevices = append(mountedISCSIDevices, iscsiDevice)
				break iSCSIDeviceLoop

			} else {
				// Then look for a slave device match
				for _, iscsiSlaveDevice := range iscsiDevice.Devices {
					if mountedDevice == iscsiSlaveDevice {
						mountedISCSIDevices = append(mountedISCSIDevices, iscsiDevice)
						break iSCSIDeviceLoop
					}
				}
			}
		}
	}

	for _, md := range mountedISCSIDevices {
		Logc(ctx).WithFields(LogFields{
			"host":            md.Host,
			"lun":             md.LUN,
			"devices":         md.Devices,
			"multipathDevice": md.MultipathDevice,
			"iqn":             md.IQN,
		}).Debug("Found mounted iSCSI device.")
	}

	return mountedISCSIDevices, nil
}

// GetISCSIDevices returns a list of iSCSI devices that are attached to (but not necessarily mounted on) this host.
func GetISCSIDevices(ctx context.Context, getCredentials bool) ([]*models.ScsiDeviceInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices.GetISCSIDevices")
	defer Logc(ctx).Debug("<<<< devices.GetISCSIDevices")

	devices := make([]*models.ScsiDeviceInfo, 0)
	hostSessionMapCache := make(map[string]map[int]int)

	// Start by reading the sessions from /sys/class/iscsi_session
	sysPath := chrootPathPrefix + "/sys/class/iscsi_session/"
	sessionDirs, err := os.ReadDir(sysPath)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return nil, err
	}

	// Loop through each of the iSCSI sessions
	for _, sessionDir := range sessionDirs {

		var sessionNumber int
		var iscsiChapInfo models.IscsiChapInfo
		sessionName := sessionDir.Name()

		if !strings.HasPrefix(sessionName, "session") {
			continue
		} else if sessionNumber, err = strconv.Atoi(strings.TrimPrefix(sessionName, "session")); err != nil {
			Logc(ctx).WithField("session", sessionName).Error("Could not parse session number")
			return nil, err
		}

		// Find the target IQN and Credentials from the session at /sys/class/iscsi_session/sessionXXX/targetname
		sessionPath := sysPath + sessionName
		sessionFiles := map[string]string{"targetname": "targetIQN"}
		if getCredentials {
			sessionFiles["username"] = "IscsiUsername"
			sessionFiles["username_in"] = "IscsiTargetUsername"
			sessionFiles["password"] = "IscsiInitiatorSecret"
			sessionFiles["password_in"] = "IscsiTargetSecret"
		}

		sessionValues := make(map[string]string, len(sessionFiles))
		for file, value := range sessionFiles {
			path := sessionPath + "/" + file
			fileBytes, err := os.ReadFile(path)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"path":  path,
					"error": err,
				}).Errorf("Could not read %v file", file)
				return nil, err
			}

			// When CHAP not in use instead of empty
			// credentials they are "(null)" in sysfs
			fileContent := strings.TrimSpace(string(fileBytes))
			if fileContent == "(null)" {
				fileContent = ""
			}

			sessionValues[value] = fileContent
		}

		targetIQN := sessionValues["targetIQN"]

		if getCredentials {
			iscsiChapInfo = models.IscsiChapInfo{
				IscsiUsername:        sessionValues["IscsiUsername"],
				IscsiInitiatorSecret: sessionValues["IscsiInitiatorSecret"],
				IscsiTargetUsername:  sessionValues["IscsiTargetUsername"],
				IscsiTargetSecret:    sessionValues["IscsiTargetSecret"],
			}

			if iscsiChapInfo != (models.IscsiChapInfo{}) {
				iscsiChapInfo.UseCHAP = true
			}
		}

		Logc(ctx).WithFields(LogFields{
			"targetIQN":   targetIQN,
			"sessionName": sessionName,
		}).Debug("Found iSCSI session / target IQN.")

		// Find the one target at /sys/class/iscsi_session/sessionXXX/device/targetHH:BB:DD (host:bus:device)
		sessionDevicePath := sessionPath + "/device/"
		targetDirs, err := os.ReadDir(sessionDevicePath)
		if err != nil {
			Logc(ctx).WithField("error", err).Errorf("Could not read %s", sessionDevicePath)
			return nil, err
		}

		// Get the one target directory
		hostBusDeviceName := ""
		targetDirName := ""
		for _, targetDir := range targetDirs {

			targetDirName = targetDir.Name()

			if strings.HasPrefix(targetDirName, "target") {
				hostBusDeviceName = strings.TrimPrefix(targetDirName, "target")
				break
			}
		}

		if hostBusDeviceName == "" {
			Logc(ctx).Warningf("Could not find a host:bus:device directory at %s", sessionDevicePath)
			continue
		}

		sessionDeviceHBDPath := sessionDevicePath + targetDirName + "/"

		Logc(ctx).WithFields(LogFields{
			"hbdPath": sessionDeviceHBDPath,
			"hbdName": hostBusDeviceName,
		}).Debug("Found host/bus/device path.")

		// Find the devices at /sys/class/iscsi_session/sessionXXX/device/targetHH:BB:DD/HH:BB:DD:LL (host:bus:device:lun)
		hostBusDeviceLunDirs, err := os.ReadDir(sessionDeviceHBDPath)
		if err != nil {
			Logc(ctx).WithField("error", err).Errorf("Could not read %s", sessionDeviceHBDPath)
			return nil, err
		}

		for _, hostBusDeviceLunDir := range hostBusDeviceLunDirs {

			hostBusDeviceLunDirName := hostBusDeviceLunDir.Name()
			if !strings.HasPrefix(hostBusDeviceLunDirName, hostBusDeviceName) {
				continue
			}

			sessionDeviceHBDLPath := sessionDeviceHBDPath + hostBusDeviceLunDirName + "/"

			Logc(ctx).WithFields(LogFields{
				"hbdlPath": sessionDeviceHBDLPath,
				"hbdlName": hostBusDeviceLunDirName,
			}).Debug("Found host/bus/device/LUN path.")

			hbdlValues := strings.Split(hostBusDeviceLunDirName, ":")
			if len(hbdlValues) != 4 {
				Logc(ctx).Errorf("Could not parse values from %s", hostBusDeviceLunDirName)
				return nil, err
			}

			hostNum := hbdlValues[0]
			busNum := hbdlValues[1]
			deviceNum := hbdlValues[2]
			lunNum := hbdlValues[3]

			blockPath := sessionDeviceHBDLPath + "/block/"

			// Find the block device at /sys/class/iscsi_session/sessionXXX/device/targetHH:BB:DD/HH:BB:DD:LL/block
			blockDeviceDirs, err := os.ReadDir(blockPath)
			if err != nil {
				Logc(ctx).WithField("error", err).Errorf("Could not read %s", blockPath)
				return nil, err
			}

			for _, blockDeviceDir := range blockDeviceDirs {

				blockDeviceName := blockDeviceDir.Name()

				Logc(ctx).WithField("blockDeviceName", blockDeviceName).Debug("Found block device.")

				// Find multipath device, if any
				var slaveDevices []string
				multipathDevice := findMultipathDeviceForDevice(ctx, blockDeviceName)
				if multipathDevice != "" {
					slaveDevices = findDevicesForMultipathDevice(ctx, multipathDevice)
				} else {
					slaveDevices = []string{blockDeviceName}
				}

				// Get the host/session map, using a cached value if available
				hostSessionMap, ok := hostSessionMapCache[targetIQN]
				if !ok {
					hostSessionMap = IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, targetIQN)
					hostSessionMapCache[targetIQN] = hostSessionMap
				}

				Logc(ctx).WithFields(LogFields{
					"host":            hostNum,
					"lun":             lunNum,
					"devices":         slaveDevices,
					"multipathDevice": multipathDevice,
					"iqn":             targetIQN,
					"sessionNumber":   sessionNumber,
					"CHAPInUse":       iscsiChapInfo.UseCHAP,
					"hostSessionMap":  hostSessionMap,
				}).Debug("Found iSCSI device.")

				device := &models.ScsiDeviceInfo{
					Host:            hostNum,
					Channel:         busNum,
					Target:          deviceNum,
					LUN:             lunNum,
					Devices:         slaveDevices,
					MultipathDevice: multipathDevice,
					IQN:             targetIQN,
					SessionNumber:   sessionNumber,
					CHAPInfo:        iscsiChapInfo,
				}

				devices = append(devices, device)
			}
		}
	}

	return devices, nil
}

// findDevicesForMultipathDevice finds the constituent devices for a devicemapper parent device like /dev/dm-0.
func findDevicesForMultipathDevice(ctx context.Context, device string) []string {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.findDevicesForMultipathDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< devices.findDevicesForMultipathDevice")

	devices := make([]string, 0)

	slavesDir := chrootPathPrefix + "/sys/block/" + device + "/slaves"
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
func compareWithPublishedDevicePath(
	ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	isProbablyGhostDevice := false
	discoverMpath := strings.TrimPrefix(deviceInfo.MultipathDevice, iscsi.DevPrefix)
	publishedMpath := strings.TrimPrefix(publishInfo.DevicePath, iscsi.DevPrefix)

	if discoverMpath != publishedMpath {
		// If this is the case, a wrong multipath device has been identified.
		// Reset the Multipath device and disks
		Logc(ctx).WithFields(LogFields{
			"lun":                       publishInfo.IscsiLunNumber,
			"discoveredMultipathDevice": discoverMpath,
			"publishedMultipathDevice":  publishedMpath,
		}).Debug("Discovered multipath device may not be correct,")

		deviceInfo.MultipathDevice = strings.TrimPrefix(publishedMpath, iscsi.DevPrefix)
		deviceInfo.Devices = []string{}

		// Get Device based on the multipath value at the same time identify if it is a ghost device.
		devices, err := IscsiUtils.GetMultipathDeviceDisks(ctx, deviceInfo.MultipathDevice)
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
func compareWithPublishedSerialNumber(
	ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	isProbablyGhostDevice := false
	lunSerialCheckPassed := false

	for _, path := range deviceInfo.DevicePaths {
		serial, err := getLunSerial(ctx, path)
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
		multipathDevice, err := IscsiUtils.GetMultipathDeviceBySerial(ctx, lunSerialHex)
		if err != nil {
			return false, fmt.Errorf("failed to verify multipath device for serial '%v'; %v ",
				publishInfo.IscsiLunSerial, err)
		}

		deviceInfo.MultipathDevice = strings.TrimPrefix(multipathDevice, iscsi.DevPrefix)

		// Get Device based on the multipath value at the same time identify if it is a ghost device.
		devices, err := IscsiUtils.GetMultipathDeviceDisks(ctx, multipathDevice)
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
func compareWithAllPublishInfos(
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
		listAllISCSIDevices(ctx)

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

// verifyMultipathDevice verifies that device being removed is correct based on published device path,
// device serial number (if present) or comparing all publications (allPublishInfos) for
// LUN number uniqueness.
func verifyMultipathDevice(
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
		return compareWithPublishedDevicePath(ctx, publishInfo, deviceInfo)
	} else if publishInfo.IscsiLunSerial != "" {
		return compareWithPublishedSerialNumber(ctx, publishInfo, deviceInfo)
	}

	return false, compareWithAllPublishInfos(ctx, publishInfo, allPublishInfos, deviceInfo)
}

// PrepareDeviceForRemoval informs Linux that a device will be removed, the function
// also verifies that device being removed is correct based on published device path,
// device serial number (if present) or comparing all publications (allPublishInfos) for
// LUN number uniqueness.
func PrepareDeviceForRemoval(
	ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool,
) (string, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	lunID := int(publishInfo.IscsiLunNumber)
	iSCSINodeName := publishInfo.IscsiTargetIQN

	fields := LogFields{
		"lunID":            lunID,
		"iSCSINodeName":    iSCSINodeName,
		"chrootPathPrefix": chrootPathPrefix,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.PrepareDeviceForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.PrepareDeviceForRemoval")

	// CSI Case
	if publishInfo.IscsiTargetPortal != "" {
		_, err := verifyMultipathDevice(ctx, publishInfo, allPublishInfos, deviceInfo)
		if err != nil {
			return "", err
		}
	}

	var multipathDevice string
	performDeferredDeviceRemoval, err := removeSCSIDevice(ctx, deviceInfo, ignoreErrors, force)
	if performDeferredDeviceRemoval && deviceInfo.MultipathDevice != "" {
		multipathDevice = iscsi.DevPrefix + deviceInfo.MultipathDevice
		Logc(ctx).WithFields(LogFields{
			"lunID":           lunID,
			"multipathDevice": multipathDevice,
		}).Debug("Discovered unmapped multipath device when removing SCSI device.")
	}

	return multipathDevice, err
}

// PrepareDeviceAtMountPathForRemoval informs Linux that a device will be removed.
// Unused stub.
func PrepareDeviceAtMountPathForRemoval(ctx context.Context, mountpoint string, unmount, unsafe, force bool) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	fields := LogFields{"mountpoint": mountpoint}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.PrepareDeviceAtMountPathForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.PrepareDeviceAtMountPathForRemoval")

	deviceInfo, err := getDeviceInfoForMountPath(ctx, mountpoint)
	if err != nil {
		return err
	}

	if unmount {
		if err := Umount(ctx, mountpoint); err != nil {
			return err
		}
	}

	_, err = removeSCSIDevice(ctx, deviceInfo, unsafe, force)

	return err
}

// RemoveMultipathDeviceMapping uses "multipath -f <devicePath>" to flush(remove) unused map.
// Unused maps can happen when Unstage is called on offline/deleted LUN.
func RemoveMultipathDeviceMapping(ctx context.Context, devicePath string) error {
	Logc(ctx).WithField("devicePath", devicePath).Debug(">>>> devices.RemoveMultipathDevicemapping")
	defer Logc(ctx).Debug("<<<< devices.RemoveMultipathDeviceMapping")

	if devicePath == "" {
		return nil
	}
	if err := beforeFlushMultipathDevice.Inject(); err != nil {
		return err
	}
	out, err := command.ExecuteWithTimeout(ctx, "multipath", 10*time.Second, false, "-f", devicePath)
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

// removeSCSIDevice informs Linux that a device will be removed.  The deviceInfo provided only needs
// the devices and multipathDevice fields set.
// IMPORTANT: The unsafe and force arguments have significant ramifications. Setting ignoreErrors=true will cause the
// function to ignore errors, and try to the remove the device even if that results in data loss, data corruption,
// or putting the system into an invalid state. Setting skipFlush=true will cause data loss, as it does not wait for the
// device to flush any remaining data, but this option is provided to avoid an indefinite hang of flush operation in
// case of an end device is in bad state. Setting ignoreErrors=false and skipFlush=false will fail at the first problem
// encountered, so that callers can be assured that a successful return indicates that the device was cleanly removed.
// This is important because while most of the time the top priority is to avoid data
// loss or data corruption, there are times when data loss is unavoidable, or has already
// happened, and in those cases it's better to be able to clean up than to be stuck in an
// endless retry loop.
func removeSCSIDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, ignoreErrors, skipFlush bool) (bool, error) {
	listAllISCSIDevices(ctx)

	// Flush multipath device
	if !skipFlush {
		err := multipathFlushDevice(ctx, deviceInfo)
		if err != nil {
			if errors.IsTimeoutError(err) {
				// Proceed to removeDevice(), ignore any errors.
				ignoreErrors = true
			} else if !ignoreErrors {
				return false, err
			}
		}
	}

	// Flush devices
	if !skipFlush {
		if err := beforeFlushDevice.Inject(); err != nil {
			return false, err
		}
		err := flushDevice(ctx, deviceInfo, ignoreErrors)
		if err != nil && !ignoreErrors {
			return false, err
		}
	}

	// Remove device
	err := removeDevice(ctx, deviceInfo, ignoreErrors)
	if err != nil && !ignoreErrors {
		return false, err
	}

	// Give the host a chance to fully process the removal
	time.Sleep(time.Second)
	listAllISCSIDevices(ctx)

	// If ignoreErrors was set to true while entering into this function and
	// multipathFlushDevice above is executed successfully then multipath device
	// mapping would have been removed there. However, we still may attempt
	// executing RemoveMultipathDeviceMapping() one more time because of below
	// bool return. In RemoveMultipathDeviceMapping() we swallow error for now.
	// In case RemoveMultipathDeviceMapping() changes in future to handle error,
	// one may need to revisit the below bool ignoreErrors being set on timeout error
	// resulting from multipathFlushDevice() call at the start of this function.
	return ignoreErrors || skipFlush, nil
}

// getDeviceInfoForMountPath discovers the device that is currently mounted at the specified mount path.  It
// uses the ScsiDeviceInfo struct so that it may return a multipath device (if any) plus one or more underlying
// physical devices.
func getDeviceInfoForMountPath(ctx context.Context, mountpath string) (*models.ScsiDeviceInfo, error) {
	fields := LogFields{"mountpath": mountpath}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.getDeviceInfoForMountPath")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.getDeviceInfoForMountPath")

	device, _, err := GetDeviceNameFromMount(ctx, mountpath)
	if err != nil {
		return nil, err
	}

	device, err = filepath.EvalSymlinks(device)
	if err != nil {
		return nil, err
	}

	device = strings.TrimPrefix(device, iscsi.DevPrefix)

	var deviceInfo *models.ScsiDeviceInfo

	if !strings.HasPrefix(device, "dm-") {
		deviceInfo = &models.ScsiDeviceInfo{
			Devices: []string{device},
		}
	} else {
		deviceInfo = &models.ScsiDeviceInfo{
			Devices:         findDevicesForMultipathDevice(ctx, device),
			MultipathDevice: device,
		}
	}

	Logc(ctx).WithFields(LogFields{
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
	}).Debug("Found SCSI device.")

	return deviceInfo, nil
}

type LUKSDevice struct {
	rawDevicePath string
	mappingName   string
}

func NewLUKSDevice(rawDevicePath, volumeId string) (*LUKSDevice, error) {
	luksDeviceName := luksDevicePrefix + volumeId
	return &LUKSDevice{rawDevicePath, luksDeviceName}, nil
}

// MappedDevicePath returns the location of the LUKS device when opened.
func (d *LUKSDevice) MappedDevicePath() string {
	return iscsi.DevMapperRoot + d.mappingName
}

// MappedDeviceName returns the name of the LUKS device when opened.
func (d *LUKSDevice) MappedDeviceName() string {
	return d.mappingName
}

// RawDevicePath returns the original device location, such as the iscsi device or multipath device. Ex: /dev/sdb
func (d *LUKSDevice) RawDevicePath() string {
	return d.rawDevicePath
}

// EnsureFormattedAndOpen ensures the specified device is LUKS formatted and opened.
func (d *LUKSDevice) EnsureFormattedAndOpen(ctx context.Context, luksPassphrase string) (formatted bool, err error) {
	return ensureLUKSDevice(ctx, d, luksPassphrase)
}

func NewLUKSDeviceFromMappingPath(ctx context.Context, mappingPath, volumeId string) (*LUKSDevice, error) {
	rawDevicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, mappingPath)
	if err != nil {
		return nil, fmt.Errorf("could not determine underlying device for LUKS mapping; %v", err)
	}
	return NewLUKSDevice(rawDevicePath, volumeId)
}

func ensureLUKSDevice(ctx context.Context, luksDevice models.LUKSDeviceInterface, luksPassphrase string) (bool, error) {
	// First check if LUKS device is already opened. This is OK to check even if the device isn't LUKS formatted.
	if isOpen, err := luksDevice.IsOpen(ctx); err != nil {
		// If the LUKS device isn't found, it means that we need to check if the device is LUKS formatted.
		// If it isn't, then we should format it and attempt to open it.
		// If any other error occurs, bail out.
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithError(err).Error("Could not check if device is an open LUKS device.")
			return false, err
		}
	} else if isOpen {
		Logc(ctx).Debug("Device is LUKS formatted and open.")
		return true, nil
	}

	if err := luksDevice.LUKSFormat(ctx, luksPassphrase); err != nil {
		Logc(ctx).WithError(err).Error("Could not LUKS format device.")
		return false, fmt.Errorf("could not LUKS format device; %w", err)
	}

	// At this point, we should be able to open the device.
	if err := luksDevice.Open(ctx, luksPassphrase); err != nil {
		// At this point, we couldn't open the LUKS device, but we do know
		// the device is LUKS formatted because LUKSFormat didn't fail.
		Logc(ctx).WithError(err).Error("Could not open LUKS formatted device.")
		return true, fmt.Errorf("could not open LUKS device; %v", err)
	}

	Logc(ctx).Debug("Device is LUKS formatted and open.")
	return true, nil
}

func GetLUKSPassphrasesFromSecretMap(secrets map[string]string) (string, string, string, string) {
	var luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase string
	luksPassphrase = secrets["luks-passphrase"]
	luksPassphraseName = secrets["luks-passphrase-name"]
	previousLUKSPassphrase = secrets["previous-luks-passphrase"]
	previousLUKSPassphraseName = secrets["previous-luks-passphrase-name"]

	return luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase
}

// EnsureLUKSDeviceMappedOnHost ensures the specified device is LUKS formatted, opened, and has the current passphrase.
func EnsureLUKSDeviceMappedOnHost(
	ctx context.Context, luksDevice models.LUKSDeviceInterface, name string, secrets map[string]string,
) (bool, error) {
	// Try to Open with current luks passphrase
	luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase := GetLUKSPassphrasesFromSecretMap(secrets)
	if luksPassphrase == "" {
		return false, fmt.Errorf("LUKS passphrase cannot be empty")
	}
	if luksPassphraseName == "" {
		return false, fmt.Errorf("LUKS passphrase name cannot be empty")
	}

	Logc(ctx).WithFields(LogFields{
		"volume":               name,
		"luks-passphrase-name": luksPassphraseName,
	}).Info("Opening encrypted volume.")
	luksFormatted, err := luksDevice.EnsureFormattedAndOpen(ctx, luksPassphrase)
	if err == nil {
		return luksFormatted, nil
	}

	// If we failed to open, try previous passphrase
	if previousLUKSPassphrase == "" {
		// Return original error if there is no previous passphrase to use
		return luksFormatted, fmt.Errorf("could not open LUKS device; %v", err)
	}
	if luksPassphrase == previousLUKSPassphrase {
		return luksFormatted, fmt.Errorf("could not open LUKS device, previous passphrase matches current")
	}
	if previousLUKSPassphraseName == "" {
		return luksFormatted, fmt.Errorf("could not open LUKS device, no previous passphrase name provided")
	}
	Logc(ctx).WithFields(LogFields{
		"volume":               name,
		"luks-passphrase-name": previousLUKSPassphraseName,
	}).Info("Opening encrypted volume.")
	luksFormatted, err = luksDevice.EnsureFormattedAndOpen(ctx, previousLUKSPassphrase)
	if err != nil {
		return luksFormatted, fmt.Errorf("could not open LUKS device; %v", err)
	}

	return luksFormatted, nil
}

func ResizeLUKSDevice(ctx context.Context, luksDevicePath, luksPassphrase string) error {
	luksDeviceName := filepath.Base(luksDevicePath)
	luksDevice := &LUKSDevice{"", luksDeviceName}
	return luksDevice.Resize(ctx, luksPassphrase)
}

// IsLegacyLUKSDevicePath returns true if the device path points to mapped LUKS device instead of mpath device.
func IsLegacyLUKSDevicePath(devicePath string) bool {
	return strings.Contains(devicePath, "luks")
}

func GetLUKSDeviceForMultipathDevice(multipathDevice string) (string, error) {
	const luksDeviceUUIDPrefix = "CRYPT-LUKS2"
	const luksDeviceUUIDNameOffset = 45

	dmDevice := strings.TrimSuffix(strings.TrimPrefix(multipathDevice, "/dev/"), "/")

	// Get holder of mpath device
	dirents, err := os.ReadDir(fmt.Sprintf("/sys/block/%s/holders/", dmDevice))
	if err != nil {
		return "", err
	}
	if len(dirents) != 1 {
		return "", fmt.Errorf("%s has unexpected number of holders, expected 1", dmDevice)
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

	return iscsi.DevMapperRoot + strings.TrimRight(string(b[luksDeviceUUIDNameOffset:]), "\n"), nil
}

// ------ TODO remove:  temporary functions to bridge the gap while we transition to the new iscsi client ------

func GetDeviceInfoForLUN(
	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, isDetachCall bool,
) (*models.ScsiDeviceInfo, error) {
	// TODO: check if we require both isDetachCall and needFSType
	return iscsiClient.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, isDetachCall)
}

func GetDeviceInfoForFCPLUN(
	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, isDetachCall bool,
) (*models.ScsiDeviceInfo, error) {
	// TODO: check if we require both isDetachCall and needFSType
	deviceInfo, err := fcpClient.GetDeviceInfoForLUN(ctx, lunID, iSCSINodeName, false, isDetachCall)
	if err != nil {
		return nil, err
	}

	return &models.ScsiDeviceInfo{
		Host:            deviceInfo.Host,
		Channel:         deviceInfo.Channel,
		Target:          deviceInfo.Target,
		LUN:             deviceInfo.LUN,
		Devices:         deviceInfo.Devices,
		MultipathDevice: deviceInfo.MultipathDevice,
		WWNN:            deviceInfo.WWNN,
		SessionNumber:   deviceInfo.SessionNumber,
	}, nil
}

func findMultipathDeviceForDevice(ctx context.Context, device string) string {
	return iscsiClient.FindMultipathDeviceForDevice(ctx, device)
}

func getLunSerial(ctx context.Context, path string) (string, error) {
	return iscsiClient.GetLunSerial(ctx, path)
}
