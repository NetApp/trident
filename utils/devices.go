// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
)

const luksDevicePrefix = "luks-"

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
func flushDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo, force bool) error {
	Logc(ctx).Debug(">>>> devices.flushDevice")
	defer Logc(ctx).Debug("<<<< devices.flushDevice")

	for _, device := range deviceInfo.Devices {
		err := flushOneDevice(ctx, "/dev/"+device)
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

// ensureDeviceReadableWithRetry reads first 4 KiBs of the device to ensures it is readable and retries on errors.
// This function will be deleted when BOF is moved to centralized retry.
func ensureDeviceReadableWithRetry(ctx context.Context, device string) error {
	readNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Failed to read the device, retrying.")
	}

	attemptToRead := func() error {
		return ensureDeviceReadable(ctx, device)
	}

	maxDuration := 30 * time.Second

	readBackoff := backoff.NewExponentialBackOff()
	readBackoff.InitialInterval = 2 * time.Second
	readBackoff.Multiplier = 2
	readBackoff.RandomizationFactor = 0.1
	readBackoff.MaxElapsedTime = maxDuration

	// Run the read check using an exponential backoff
	if err := backoff.RetryNotify(attemptToRead, readBackoff, readNotify); err != nil {
		Logc(ctx).Errorf("Could not read device %v after %3.2f seconds.", device, maxDuration.Seconds())
		return err
	}

	return nil
}

// ensureDeviceReadable reads first 4 KiBs of the device to ensures it is readable
func ensureDeviceReadable(ctx context.Context, device string) error {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.ensureDeviceReadable")
	defer Logc(ctx).Debug("<<<< devices.ensureDeviceReadable")

	args := []string{"if=" + device, "bs=4096", "count=1", "status=none"}
	out, err := execCommandWithTimeout(ctx, "dd", deviceOperationsTimeout, false, args...)
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
	out, err := execCommandWithTimeout(ctx, "dd", deviceOperationsTimeout, false, args...)
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

func ISCSIRescanDevices(ctx context.Context, targetIQN string, lunID int32, minSize int64) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	fields := LogFields{"targetIQN": targetIQN, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.ISCSIRescanDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.ISCSIRescanDevices")

	deviceInfo, err := getDeviceInfoForLUN(ctx, int(lunID), targetIQN, false, false)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %s", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get iSCSI device information for LUN: %d", lunID)
	}

	allLargeEnough := true
	for _, diskDevice := range deviceInfo.Devices {
		size, err := getISCSIDiskSize(ctx, "/dev/"+diskDevice)
		if err != nil {
			return err
		}
		if size < minSize {
			allLargeEnough = false
		} else {
			continue
		}

		err = iSCSIRescanDisk(ctx, diskDevice)
		if err != nil {
			Logc(ctx).WithField("diskDevice", diskDevice).Error("Failed to rescan disk.")
			return fmt.Errorf("failed to rescan disk %s: %s", diskDevice, err)
		}
	}

	if !allLargeEnough {
		time.Sleep(time.Second)
		for _, diskDevice := range deviceInfo.Devices {
			size, err := getISCSIDiskSize(ctx, "/dev/"+diskDevice)
			if err != nil {
				return err
			}
			if size < minSize {
				Logc(ctx).Error("Disk size not large enough after resize.")
				return fmt.Errorf("disk size not large enough after resize: %d, %d", size, minSize)
			}
		}
	}

	if deviceInfo.MultipathDevice != "" {
		multipathDevice := deviceInfo.MultipathDevice
		size, err := getISCSIDiskSize(ctx, "/dev/"+multipathDevice)
		if err != nil {
			return err
		}

		fields = LogFields{"size": size, "minSize": minSize}
		if size < minSize {
			Logc(ctx).WithFields(fields).Debug("Reloading the multipath device.")
			err := reloadMultipathDevice(ctx, multipathDevice)
			if err != nil {
				return err
			}
			time.Sleep(time.Second)
			size, err = getISCSIDiskSize(ctx, "/dev/"+multipathDevice)
			if err != nil {
				return err
			}
			if size < minSize {
				Logc(ctx).Error("Multipath device not large enough after resize.")
				return fmt.Errorf("multipath device not large enough after resize: %d < %d", size, minSize)
			}
		} else {
			Logc(ctx).WithFields(fields).Debug("Not reloading the multipath device because the size is greater than or equal to the minimum size.")
		}
	}

	return nil
}

func reloadMultipathDevice(ctx context.Context, multipathDevice string) error {
	fields := LogFields{"multipathDevice": multipathDevice}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.reloadMultipathDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.reloadMultipathDevice")

	if multipathDevice == "" {
		return fmt.Errorf("cannot reload an empty multipathDevice")
	}

	_, err := execCommandWithTimeout(ctx, "multipath", 30*time.Second, true, "-r", "/dev/"+multipathDevice)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"device": multipathDevice,
			"error":  err,
		}).Error("Failed to reload multipathDevice.")
		return fmt.Errorf("failed to reload multipathDevice %s: %s", multipathDevice, err)
	}

	Logc(ctx).WithFields(fields).Debug("Multipath device reloaded.")
	return nil
}

// iSCSIRescanDisk causes the kernel to rescan a single iSCSI disk/block device.
// This is how size changes are found when expanding a volume.
func iSCSIRescanDisk(ctx context.Context, deviceName string) error {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.iSCSIRescanDisk")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.iSCSIRescanDisk")

	listAllISCSIDevices(ctx)
	filename := fmt.Sprintf(chrootPathPrefix+"/sys/block/%s/device/rescan", deviceName)
	Logc(ctx).WithField("filename", filename).Debug("Opening file for writing.")

	f, err := os.OpenFile(filename, os.O_WRONLY, 0)
	if err != nil {
		Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
		return err
	}
	defer f.Close()

	written, err := f.WriteString("1")
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"file":  filename,
			"error": err,
		}).Warning("Could not write to file.")
		return err
	} else if written == 0 {
		Logc(ctx).WithField("file", filename).Warning("Zero bytes written to file.")
		return fmt.Errorf("no data written to %s", filename)
	}

	listAllISCSIDevices(ctx)
	return nil
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

// In the case of a iscsi trace debug, log info about session and what devices are present
func listAllISCSIDevices(ctx context.Context) {
	if !IsLevelEnabled(log.TraceLevel) {
		// Don't even run the commands if trace logging is not enabled
		return
	}

	Logc(ctx).Trace(">>>> devices.listAllISCSIDevices")
	defer Logc(ctx).Trace("<<<< devices.listAllISCSIDevices")
	// Log information about all the devices
	dmLog := make([]string, 0)
	sdLog := make([]string, 0)
	sysLog := make([]string, 0)
	entries, _ := ioutil.ReadDir("/dev/")
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "dm-") {
			dmLog = append(dmLog, entry.Name())
		}
		if strings.HasPrefix(entry.Name(), "sd") {
			sdLog = append(sdLog, entry.Name())
		}
	}

	entries, _ = ioutil.ReadDir("/sys/block/")
	for _, entry := range entries {
		sysLog = append(sysLog, entry.Name())
	}
	out1, _ := execCommandWithTimeout(ctx, "multipath", deviceOperationsTimeout, true, "-ll")
	out2, _ := execIscsiadmCommand(ctx, "-m", "session")
	Logc(ctx).WithFields(LogFields{
		"/dev/dm-*":                  dmLog,
		"/dev/sd*":                   sdLog,
		"/sys/block/*":               sysLog,
		"multipath -ll output":       string(out1),
		"iscsiadm -m session output": string(out2),
	}).Trace("Listing all iSCSI Devices.")
}

// removeDevice tells Linux that a device will be removed.
func removeDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo, ignoreErrors bool) error {
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

	out, err := execCommandWithTimeout(ctx, "multipath", deviceOperationsTimeout, true, "-C", devicePath)
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
		return ISCSIDeviceFlushError("multipath device is not ready for flush")
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
			return TimeoutError(fmt.Sprintf("Max flush wait time expired. Elapsed: %v", elapsed))
		}
	}

	return ISCSIDeviceFlushError("multipath device is unavailable")
}

// multipathFlushDevice invokes the 'multipath' commands to flush paths for a single device.
func multipathFlushDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo) error {
	Logc(ctx).WithField("device", deviceInfo.MultipathDevice).Debug(">>>> devices.multipathFlushDevice")
	defer Logc(ctx).Debug("<<<< devices.multipathFlushDevice")

	if deviceInfo.MultipathDevice == "" {
		return nil
	}

	devicePath := "/dev/" + deviceInfo.MultipathDevice

	deviceErr := canFlushMultipathDevice(ctx, devicePath)
	if deviceErr != nil {
		if IsISCSIDeviceFlushError(deviceErr) {
			Logc(ctx).WithFields(
				LogFields{
					"error":  deviceErr,
					"device": devicePath,
				}).Debug("Flush failed.")
			return deviceErr
		}
		if IsTimeoutError(deviceErr) {
			Logc(ctx).WithFields(LogFields{
				"error":  deviceErr,
				"device": devicePath,
				"LUN":    deviceInfo.LUN,
				"host":   deviceInfo.Host,
			}).Debug("Flush timed out.")
			return deviceErr
		}
	}

	err := flushOneDevice(ctx, devicePath)
	if err != nil {
		// Ideally this should not be the case, otherwise, we may need
		// to add more checks in canFlushMultipathDevice()
		return err
	}

	RemoveMultipathDeviceMapping(ctx, devicePath)
	return nil
}

// GetMountedISCSIDevices returns a list of iSCSI devices that are *mounted* on this host.
func GetMountedISCSIDevices(ctx context.Context) ([]*ScsiDeviceInfo, error) {
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

		hasDevMountSourcePrefix := strings.HasPrefix(procMount.MountSource, "/dev/")
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
			mountedDevice = strings.TrimPrefix(device, "/dev/")
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

	mountedISCSIDevices := make([]*ScsiDeviceInfo, 0)

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
			"hostSessionMap":  md.HostSessionMap,
		}).Debug("Found mounted iSCSI device.")
	}

	return mountedISCSIDevices, nil
}

// GetISCSIDevices returns a list of iSCSI devices that are attached to (but not necessarily mounted on) this host.
func GetISCSIDevices(ctx context.Context, getCredentials bool) ([]*ScsiDeviceInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices.GetISCSIDevices")
	defer Logc(ctx).Debug("<<<< devices.GetISCSIDevices")

	devices := make([]*ScsiDeviceInfo, 0)
	hostSessionMapCache := make(map[string]map[int]int)

	// Start by reading the sessions from /sys/class/iscsi_session
	sysPath := chrootPathPrefix + "/sys/class/iscsi_session/"
	sessionDirs, err := ioutil.ReadDir(sysPath)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return nil, err
	}

	// Loop through each of the iSCSI sessions
	for _, sessionDir := range sessionDirs {

		var sessionNumber int
		var iscsiChapInfo IscsiChapInfo
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
			fileBytes, err := ioutil.ReadFile(path)
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
			iscsiChapInfo = IscsiChapInfo{
				IscsiUsername:        sessionValues["IscsiUsername"],
				IscsiInitiatorSecret: sessionValues["IscsiInitiatorSecret"],
				IscsiTargetUsername:  sessionValues["IscsiTargetUsername"],
				IscsiTargetSecret:    sessionValues["IscsiTargetSecret"],
			}

			if iscsiChapInfo != (IscsiChapInfo{}) {
				iscsiChapInfo.UseCHAP = true
			}
		}

		Logc(ctx).WithFields(LogFields{
			"targetIQN":   targetIQN,
			"sessionName": sessionName,
		}).Debug("Found iSCSI session / target IQN.")

		// Find the one target at /sys/class/iscsi_session/sessionXXX/device/targetHH:BB:DD (host:bus:device)
		sessionDevicePath := sessionPath + "/device/"
		targetDirs, err := ioutil.ReadDir(sessionDevicePath)
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
		hostBusDeviceLunDirs, err := ioutil.ReadDir(sessionDeviceHBDPath)
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
			blockDeviceDirs, err := ioutil.ReadDir(blockPath)
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

				device := &ScsiDeviceInfo{
					Host:            hostNum,
					Channel:         busNum,
					Target:          deviceNum,
					LUN:             lunNum,
					Devices:         slaveDevices,
					MultipathDevice: multipathDevice,
					IQN:             targetIQN,
					SessionNumber:   sessionNumber,
					CHAPInfo:        iscsiChapInfo,
					HostSessionMap:  hostSessionMap,
				}

				devices = append(devices, device)
			}
		}
	}

	return devices, nil
}

// waitForMultipathDeviceForDevices accepts a list of sd* device names which are associated with same LUN
// and waits until a multipath device is present for at least one of those.  It returns the name of the
// multipath device, or an empty string if multipathd isn't running or there is only one path.
func waitForMultipathDeviceForDevices(ctx context.Context, devices []string) (string, error) {
	fields := LogFields{"devices": devices}

	Logc(ctx).WithFields(fields).Debug(">>>> devices.waitForMultipathDeviceForDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.waitForMultipathDeviceForDevices")

	multipathDevice := ""

	for _, device := range devices {
		multipathDevice = findMultipathDeviceForDevice(ctx, device)
		if multipathDevice != "" {
			break
		}
	}

	if multipathDevice == "" {
		Logc(ctx).WithField("multipathDevice", multipathDevice).Warn("Multipath device not found.")
		return "", fmt.Errorf("multipath device not found when it is expected")

	} else {
		Logc(ctx).WithField("multipathDevice", multipathDevice).Debug("Multipath device found.")
	}

	return multipathDevice, nil
}

// findMultipathDeviceForDevice finds the devicemapper parent of a device name like /dev/sdx.
func findMultipathDeviceForDevice(ctx context.Context, device string) string {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.findMultipathDeviceForDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< devices.findMultipathDeviceForDevice")

	holdersDir := chrootPathPrefix + "/sys/block/" + device + "/holders"
	if dirs, err := ioutil.ReadDir(holdersDir); err == nil {
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

// findDevicesForMultipathDevice finds the constituent devices for a devicemapper parent device like /dev/dm-0.
func findDevicesForMultipathDevice(ctx context.Context, device string) []string {
	Logc(ctx).WithField("device", device).Debug(">>>> devices.findDevicesForMultipathDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< devices.findDevicesForMultipathDevice")

	devices := make([]string, 0)

	slavesDir := chrootPathPrefix + "/sys/block/" + device + "/slaves"
	if dirs, err := ioutil.ReadDir(slavesDir); err == nil {
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

// PrepareDeviceForRemoval informs Linux that a device will be removed.
func PrepareDeviceForRemoval(ctx context.Context, lunID int, iSCSINodeName string, ignoreErrors, force bool) (string, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	fields := LogFields{
		"lunID":            lunID,
		"iSCSINodeName":    iSCSINodeName,
		"chrootPathPrefix": chrootPathPrefix,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.PrepareDeviceForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.PrepareDeviceForRemoval")

	var multipathDevice string
	var performDeferredDeviceRemoval bool

	deviceInfo, err := getDeviceInfoForLUN(ctx, lunID, iSCSINodeName, false, true)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"lunID": lunID,
		}).Warn("Could not get device info for removal, skipping host removal steps.")
		return multipathDevice, err
	}

	if deviceInfo == nil {
		Logc(ctx).WithFields(LogFields{
			"lunID": lunID,
		}).Debug("No device found for removal, skipping host removal steps.")
		return multipathDevice, nil
	}

	performDeferredDeviceRemoval, err = removeSCSIDevice(ctx, deviceInfo, ignoreErrors, force)
	if performDeferredDeviceRemoval && deviceInfo.MultipathDevice != "" {
		multipathDevice = "/dev/" + deviceInfo.MultipathDevice
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
func RemoveMultipathDeviceMapping(ctx context.Context, devicePath string) {
	Logc(ctx).WithField("devicePath", devicePath).Debug(">>>> devices.RemoveMultipathDevicemapping")
	defer Logc(ctx).Debug("<<<< devices.RemoveMultipathDeviceMapping")

	if devicePath == "" {
		return
	}

	out, err := execCommandWithTimeout(ctx, "multipath", 10*time.Second, false, "-f", devicePath)
	if err != nil {
		// Nothing to do if it generates an error, but log it.
		Logc(ctx).WithFields(LogFields{
			"error":      err,
			"output":     string(out),
			"devicePath": devicePath,
		}).Error("Error encountered in multipath flush(remove) mapping command.")
	}

	exists, err := PathExists(devicePath)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Error trying to determine if device path exists.")
	} else if !exists {
		return
	}

	serial, err := execCommandWithTimeout(ctx, "multipath", 10*time.Second, true, "-l", devicePath, "-v", "1")
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error":  err,
			"output": string(serial),
		}).Error("Error trying to retrieve serial for device path.")
	}

	out1, err := execCommandWithTimeout(ctx, "dmsetup", 10*time.Second, false, "remove", "-f", string(serial))
	if err != nil {
		// Nothing to do if it generates an error, but log it.
		Logc(ctx).WithFields(LogFields{
			"error":      err,
			"output":     string(out1),
			"devicePath": devicePath,
		}).Error("Error encountered in dmsetup remove command.")
	}

	return
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
func removeSCSIDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo, ignoreErrors, skipFlush bool) (bool, error) {
	listAllISCSIDevices(ctx)

	// Flush multipath device
	if !skipFlush {
		err := multipathFlushDevice(ctx, deviceInfo)
		if err != nil {
			if IsTimeoutError(err) {
				// Proceed to removeDevice(), ignore any errors.
				ignoreErrors = true
			} else if !ignoreErrors {
				return false, err
			}
		}
	}

	// Flush devices
	if !skipFlush {
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

// ScsiDeviceInfo contains information about SCSI devices
type ScsiDeviceInfo struct {
	Host            string
	Channel         string
	Target          string
	LUN             string
	Devices         []string
	MultipathDevice string
	Filesystem      string
	IQN             string
	SessionNumber   int
	HostSessionMap  map[int]int
	CHAPInfo        IscsiChapInfo
}

// getDeviceInfoForLUN finds iSCSI devices using /dev/disk/by-path values.  This method should be
// called after calling waitForDeviceScan so that the device paths are known to exist.
func getDeviceInfoForLUN(
	ctx context.Context, lunID int, iSCSINodeName string, needFSType, isDetachCall bool,
) (*ScsiDeviceInfo, error) {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
		"needFSType":    needFSType,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.getDeviceInfoForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.getDeviceInfoForLUN")

	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)

	// During detach if hostSessionMap count is zero, we should be fine
	if len(hostSessionMap) == 0 {
		if isDetachCall {
			Logc(ctx).WithFields(fields).Debug("No iSCSI hosts found for target.")
			return nil, nil
		} else {
			return nil, fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
		}
	}

	paths := IscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := IscsiUtils.GetDevicesForLUN(paths)
	if err != nil {
		return nil, err
	} else if 0 == len(devices) {
		return nil, fmt.Errorf("scan not completed for LUN %d on target %s", lunID, iSCSINodeName)
	}

	multipathDevice := ""
	for _, device := range devices {
		multipathDevice = findMultipathDeviceForDevice(ctx, device)
		if multipathDevice != "" {
			break
		}
	}

	var devicePath string
	if multipathDevice != "" {
		devicePath = "/dev/" + multipathDevice
	} else {
		devicePath = "/dev/" + devices[0]
	}

	fsType := ""
	if needFSType {
		err = ensureDeviceReadable(ctx, devicePath)
		if err != nil {
			return nil, err
		}

		fsType, err = getDeviceFSType(ctx, devicePath)
		if err != nil {
			return nil, err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"LUN":             strconv.Itoa(lunID),
		"multipathDevice": multipathDevice,
		"fsType":          fsType,
		"deviceNames":     devices,
		"hostSessionMap":  hostSessionMap,
	}).Debug("Found SCSI device.")

	info := &ScsiDeviceInfo{
		LUN:             strconv.Itoa(lunID),
		MultipathDevice: multipathDevice,
		Devices:         devices,
		Filesystem:      fsType,
		IQN:             iSCSINodeName,
		HostSessionMap:  hostSessionMap,
	}

	return info, nil
}

// getDeviceInfoForMountPath discovers the device that is currently mounted at the specified mount path.  It
// uses the ScsiDeviceInfo struct so that it may return a multipath device (if any) plus one or more underlying
// physical devices.
func getDeviceInfoForMountPath(ctx context.Context, mountpath string) (*ScsiDeviceInfo, error) {
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

	device = strings.TrimPrefix(device, "/dev/")

	var deviceInfo *ScsiDeviceInfo

	if !strings.HasPrefix(device, "dm-") {
		deviceInfo = &ScsiDeviceInfo{
			Devices: []string{device},
		}
	} else {
		deviceInfo = &ScsiDeviceInfo{
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

// waitForMultipathDeviceForLUN
// for the given LUN, this function waits for the associated multipath device to be present
// first find the /dev/sd* devices assocaited with the LUN
// Wait for the maultipath device dm-* for the /dev/sd* devices.
func waitForMultipathDeviceForLUN(ctx context.Context, lunID int, iSCSINodeName string) error {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.waitForMultipathDeviceForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.waitForMultipathDeviceForLUN")

	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	paths := IscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := IscsiUtils.GetDevicesForLUN(paths)
	if err != nil {
		return err
	}

	_, err = waitForMultipathDeviceForDevices(ctx, devices)

	return err
}

func NewLUKSDevice(rawDevicePath, volumeId string) (*LUKSDevice, error) {
	luksDeviceName := luksDevicePrefix + volumeId
	return &LUKSDevice{rawDevicePath, luksDeviceName}, nil
}

func NewLUKSDeviceFromMappingPath(ctx context.Context, mappingPath, volumeId string) (*LUKSDevice, error) {
	rawDevicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, mappingPath)
	if err != nil {
		return nil, fmt.Errorf("could not determine underlying device for LUKS mapping; %v", err)
	}
	return NewLUKSDevice(rawDevicePath, volumeId)
}

// MappedDevicePath returns the location of the LUKS device when opened.
func (d *LUKSDevice) MappedDevicePath() string {
	return devMapperRoot + d.mappingName
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

func ensureLUKSDevice(ctx context.Context, luksDevice LUKSDeviceInterface, luksPassphrase string) (formatted bool, err error) {
	formatted = false
	// Check if LUKS device is already opened
	isOpen, err := luksDevice.IsOpen(ctx)
	if err != nil {
		return formatted, err
	}
	if isOpen {
		return formatted, nil
	}

	// Check if the device is LUKS formatted
	isLUKS, err := luksDevice.IsLUKSFormatted(ctx)
	if err != nil {
		return formatted, err
	}

	// Install LUKS on the device if needed
	if !isLUKS {
		// Ensure device is empty before we format it with LUKS
		unformatted, err := isDeviceUnformatted(ctx, luksDevice.RawDevicePath())
		if err != nil {
			return formatted, err
		}
		if !unformatted {
			return formatted, fmt.Errorf("cannot LUKS format device; device is not empty")
		}
		err = luksDevice.LUKSFormat(ctx, luksPassphrase)
		if err != nil {
			return formatted, err
		}
		formatted = true
	}

	// Open the device, creating a new LUKS encrypted device with the name chosen by us
	err = luksDevice.Open(ctx, luksPassphrase)
	if nil != err {
		return formatted, fmt.Errorf("could not open LUKS device; %v", err)
	}

	return formatted, nil
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
func EnsureLUKSDeviceMappedOnHost(ctx context.Context, luksDevice LUKSDeviceInterface, name string, secrets map[string]string) (bool, error) {
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
