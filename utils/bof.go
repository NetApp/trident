// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
)

func AttachBlockOnFileVolume(
	ctx context.Context, mountPath string, publishInfo *VolumePublishInfo,
) (string, string, error) {
	Logc(ctx).Debug(">>>> bof.AttachBlockOnFileVolume")
	defer Logc(ctx).Debug("<<<< bof.AttachBlockOnFileVolume")

	exportPath := fmt.Sprintf("%s:%s", publishInfo.NfsServerIP, publishInfo.NfsPath)
	options := publishInfo.MountOptions
	deviceOptions := publishInfo.SubvolumeMountOptions
	nfsMountpoint := publishInfo.NFSMountpoint
	loopFile := path.Join(nfsMountpoint, publishInfo.SubvolumeName)

	var deviceMountpoint string
	if mountPath != "" {
		deviceMountpoint = path.Join(mountPath, volumeMountDir)
	}

	fsType, err := GetVerifiedBlockFsType(publishInfo.FilesystemType)
	if err != nil {
		return "", "", err
	}

	Logc(ctx).WithFields(log.Fields{
		"volume":        loopFile,
		"exportPath":    exportPath,
		"nfsMountpoint": nfsMountpoint,
		"filesystem":    fsType,
		"options":       options,
	}).Debug("Attaching block-on-file volume.")

	// Determine if the NFS share is already mounted
	nfsShareMounted, err := IsNFSShareMounted(ctx, exportPath, nfsMountpoint)
	if err != nil {
		return "", "", err
	}

	// Mount the NFS share if it isn't already mounted
	if nfsShareMounted {
		Logc(ctx).WithFields(log.Fields{
			"exportPath":    exportPath,
			"nfsMountpoint": nfsMountpoint,
		}).Debug("NFS share already mounted.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"exportPath":    exportPath,
			"nfsMountpoint": nfsMountpoint,
		}).Debug("Mounting NFS share.")

		err := mountNFSPath(ctx, exportPath, nfsMountpoint, options)
		if err != nil {
			return "", "", err
		}
	}

	// Attach the NFS-backed loop file to a loop device only if it isn't already attached
	attached, loopDevice, err := GetLoopDeviceAttachedToFile(ctx, loopFile)
	if err != nil {
		return "", "", err
	}
	if !attached {
		if _, err := attachLoopDevice(ctx, loopFile); err != nil {
			return "", "", err
		}

		if attached, loopDevice, err = GetLoopDeviceAttachedToFile(ctx, loopFile); err != nil {
			return "", "", err
		} else if !attached {
			return "", "", fmt.Errorf("could not detect new loop device attachment to %s", loopFile)
		}
	}

	if fsType != fsRaw {
		err = ensureDeviceReadableWithRetry(ctx, loopDevice.Name)
		if err != nil {
			return "", "", err
		}

		existingFstype, err := getDeviceFSTypeRetry(ctx, loopDevice.Name)
		if err != nil {
			return "", "", err
		}

		if existingFstype == "" {
			Logc(ctx).WithFields(log.Fields{"device": loopDevice.Name, "fsType": fsType}).Debug("Formatting Device.")
			err := formatVolumeRetry(ctx, loopDevice.Name, fsType)
			if err != nil {
				return "", "", fmt.Errorf("error formatting device %s: %v", loopDevice.Name, err)
			}
		} else if existingFstype != unknownFstype && existingFstype != fsType {
			Logc(ctx).WithFields(log.Fields{
				"device":          loopDevice.Name,
				"existingFstype":  existingFstype,
				"requestedFstype": fsType,
			}).Error("Device formatted with a different file system type.")
			return "", "", fmt.Errorf("device %s already formatted with other filesystem: %s", loopDevice.Name,
				existingFstype)
		} else {
			Logc(ctx).WithFields(log.Fields{
				"device": loopDevice.Name,
				"fstype": fsType,
			}).Debug("Device already formatted.")
		}
	}

	mounted, err := IsMounted(ctx, loopDevice.Name, "", "")
	if err != nil {
		return "", "", err
	}
	if !mounted {
		_ = repairVolume(ctx, loopDevice.Name, fsType)
	}

	if deviceMountpoint != "" {
		err = MountDevice(ctx, loopDevice.Name, deviceMountpoint, deviceOptions, false)
		if err != nil {
			return "", "", fmt.Errorf("error mounting device %v, mountpoint %v; %s",
				loopDevice.Name, deviceMountpoint, err)
		}
	}

	return loopDevice.Name, deviceMountpoint, nil
}

func DetachBlockOnFileVolume(ctx context.Context, loopDevice, loopFile string) error {
	Logc(ctx).Debug(">>>> bof.DetachBlockOnFileVolume")
	defer Logc(ctx).Debug("<<<< bof.DetachBlockOnFileVolume")

	loopDeviceAttached, err := IsLoopDeviceAttachedToFile(ctx, loopDevice, loopFile)
	if err != nil {
		return fmt.Errorf("unable to identify if loop device '%s' is attached to file '%s': %v",
			loopDevice, loopFile, err)
	}

	if loopDeviceAttached {
		if err := detachLoopDevice(ctx, loopDevice); err != nil {
			return fmt.Errorf("unable to detach loop device '%s': %v", loopDevice, err)
		}
	}

	return nil
}

type LoopDevicesResponse struct {
	LoopDevices []LoopDevice `json:"loopdevices"`
}

// LoopDevice contains the details of a loopback device and is designed to be populated from the result
// of "losetup --list --json".  Note that some fields changed from string to int/bool between Ubuntu 16.04
// and 18.04, and likely other Linux distributions as well.  The affected fields are unneeded here and are
// included but commented out for clarity.
type LoopDevice struct {
	Name string `json:"name"`
	// Sizelimit int64  `json:"sizelimit"`
	// Offset    int64  `json:"offset"`
	// Autoclear bool   `json:"autoclear"`
	// Ro        bool   `json:"ro"`
	BackFile string `json:"back-file"`
}

func getLoopDeviceInfo(ctx context.Context) ([]LoopDevice, error) {
	Logc(ctx).Debug(">>>> bof.getLoopDeviceInfo")
	defer Logc(ctx).Debug("<<<< bof.getLoopDeviceInfo")

	out, err := execCommandWithTimeout(ctx, "losetup", 10, true, "--list", "--json")
	if err != nil {
		Logc(ctx).WithError(err).Error("Getting loop device information failed")
		return nil, err
	}

	if len(out) == 0 {
		return []LoopDevice{}, nil
	}

	var loopDevicesResponse LoopDevicesResponse
	err = json.Unmarshal(out, &loopDevicesResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal loop device response: %v", err)
	}

	return loopDevicesResponse.LoopDevices, nil
}

func GetLoopDeviceAttachedToFile(ctx context.Context, loopFile string) (bool, *LoopDevice, error) {
	Logc(ctx).WithField("loopFile", loopFile).Debug(">>>> bof.GetLoopDeviceAttachedToFile")
	defer Logc(ctx).Debug("<<<< bof.GetLoopDeviceAttachedToFile")

	devices, err := getLoopDeviceInfo(ctx)
	if err != nil {
		return false, nil, err
	}

	// Always consider the last part (after last "/") of the back-file filepath returned by losetup
	_, loopFileBackFileName := filepath.Split(loopFile)

	for _, device := range devices {
		_, deviceBackFileName := filepath.Split(device.BackFile)
		if deviceBackFileName == loopFileBackFileName {
			Logc(ctx).WithFields(log.Fields{
				"loopFile":   loopFile,
				"loopDevice": device.Name,
			}).Debug("Found loop device.")
			return true, &device, nil
		}
	}

	return false, nil, nil
}

func GetAllLoopDeviceBackFiles(ctx context.Context) ([]string, error) {
	Logc(ctx).Debug(">>>> bof.GetAllLoopDeviceBackFiles")
	defer Logc(ctx).Debug("<<<< bof.GetAllLoopDeviceBackFiles")

	devices, err := getLoopDeviceInfo(ctx)
	if err != nil {
		return nil, err
	}

	// Get a list of all back-files
	deviceBackFiles := make([]string, len(devices))

	// Always consider the last part (after last "/") of the back-file filepath returned by losetup
	for _, device := range devices {
		_, deviceBackFileName := filepath.Split(device.BackFile)
		deviceBackFiles = append(deviceBackFiles, deviceBackFileName)
	}

	return deviceBackFiles, nil
}

func IsLoopDeviceAttachedToFile(ctx context.Context, loopDevice, loopFile string) (bool, error) {
	Logc(ctx).WithFields(log.Fields{
		"loopFile":   loopFile,
		"loopDevice": loopDevice,
	}).Debug(">>>> bof.IsLoopDeviceAttachedToFile")
	defer Logc(ctx).Debug("<<<< bof.IsLoopDeviceAttachedToFile")

	var isAttached bool
	devices, err := getLoopDeviceInfo(ctx)
	if err != nil {
		return isAttached, err
	}

	_, loopBackFileName := filepath.Split(loopFile)

	for _, device := range devices {
		_, deviceBackFileName := filepath.Split(device.BackFile)
		if device.Name == loopDevice && deviceBackFileName == loopBackFileName {
			Logc(ctx).WithFields(log.Fields{
				"loopFile":   loopFile,
				"loopDevice": device.Name,
			}).Debug("Found device attached.")
			return true, nil
		}
	}

	return false, nil
}

func ResizeLoopDevice(ctx context.Context, loopDevice, loopFile string, requiredBytes int64) error {
	Logc(ctx).WithFields(log.Fields{
		"loopFile":      loopFile,
		"loopDevice":    loopDevice,
		"requiredBytes": requiredBytes,
	}).Debug(">>>> bof.ResizeLoopDevice")
	defer Logc(ctx).Debug("<<<< bof.ResizeLoopDevice")

	var err error = nil

	loopFileInfo, err := os.Stat(loopFile)
	if err != nil {
		Logc(ctx).WithField("loopFile", loopFile).WithError(err).Error("Failed to get loop file size")
		return fmt.Errorf("failed to get loop file size: %s, %s", loopFile, err.Error())
	}

	loopFileSize := loopFileInfo.Size()

	if loopFileSize < requiredBytes {
		err = fmt.Errorf("loopFileSize must be greater than or equal to requiredBytes: %d, %d", loopFileSize,
			requiredBytes)
		Logc(ctx).WithFields(log.Fields{
			"loopFileSize":  loopFileSize,
			"requiredBytes": requiredBytes,
		}).WithError(err).Error("LoopFileSize must be greater than or equal to requiredBytes")
		return err
	}

	_, err = execCommandWithTimeout(ctx, "losetup", 10, true, "--set-capacity",
		loopDevice)
	if err != nil {
		Logc(ctx).WithField("loopDevice", loopDevice).WithError(err).Error("Failed to resize the loop device")
		return fmt.Errorf("failed to resize the loop device: %s, %s", loopDevice, err.Error())
	}

	loopDeviceSize, err := getISCSIDiskSize(ctx, loopDevice)
	if err != nil {
		Logc(ctx).WithField("loopDevice", loopDevice).WithError(err).Error(
			"Failed to get the loop device size")
		return fmt.Errorf("failed to get the loop device size: %s, %s", loopDevice, err.Error())
	}

	if loopDeviceSize < requiredBytes {
		err = fmt.Errorf("disk size not large enough after resize: %d < %d", loopDeviceSize, requiredBytes)
		Logc(ctx).WithFields(log.Fields{
			"loopDeviceSize": loopDeviceSize,
			"requiredBytes":  requiredBytes,
		}).WithError(err).Error("Disk size not large enough after resize.")
		return err
	}

	return err
}

// attachLoopDevice invokes losetup to attach a file to a loop device.
func attachLoopDevice(ctx context.Context, loopFile string) (string, error) {
	Logc(ctx).WithField("loopFile", loopFile).Debug(">>>> bof.attachLoopDevice")
	defer Logc(ctx).Debug("<<<< bof.attachLoopDevice")

	out, err := execCommandWithTimeout(ctx, "losetup", 10, true, "--find", "--show", "--direct-io", "--nooverlap",
		loopFile)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to attach loop file.")
		return "", fmt.Errorf("failed to attach loop file: %v", err)
	}

	device := strings.TrimSpace(string(out))
	if deviceRegex.MatchString(device) {
		return device, nil
	}

	return "", fmt.Errorf("losetup returned an invalid value (%s)", device)
}

// detachLoopDevice invokes losetup to detach a file from a loop device.
func detachLoopDevice(ctx context.Context, loopDeviceName string) error {
	Logc(ctx).WithField("loopDeviceName", loopDeviceName).Debug(">>>> bof.DetachLoopDevice")
	defer Logc(ctx).Debug("<<<< bof.DetachLoopDevice")

	_, err := execCommandWithTimeout(ctx, "losetup", 10, true, "--detach", loopDeviceName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to detach loop device")
		return fmt.Errorf("failed to detach loop device '%s': %v", loopDeviceName, err)
	}

	return nil
}

// WaitForLoopDeviceDetach waits until a loop device is detached from the specified NFS mountpoint, and it returns
// the remaining number of attached devices.
func WaitForLoopDeviceDetach(
	ctx context.Context, nfsMountpoint, loopFile string, maxDuration time.Duration,
) error {
	logFields := log.Fields{"nfsMountpoint": nfsMountpoint, "loopFile": loopFile}
	Logc(ctx).WithFields(logFields).Debug(">>>> bof.WaitForLoopDevicesOnNFSMountpoint")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< bof.WaitForLoopDevicesOnNFSMountpoint")

	var loopDevice *LoopDevice
	var deviceAttached bool

	checkLoopDevices := func() error {
		var err error

		if deviceAttached, loopDevice, err = GetLoopDeviceAttachedToFile(ctx, loopFile); err != nil {
			return backoff.Permanent(err)
		} else if deviceAttached {
			return fmt.Errorf("loopback device '%s' still attached to '%s'", loopDevice.Name, loopFile)
		}
		return nil
	}

	loopDeviceNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"increment":     duration,
			"nfsMountpoint": nfsMountpoint,
			"error":         err,
		}).Debug("Loopback device still attached, waiting.")
	}

	loopDeviceBackoff := backoff.NewExponentialBackOff()
	loopDeviceBackoff.InitialInterval = 1 * time.Second
	loopDeviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	loopDeviceBackoff.RandomizationFactor = 0.1
	loopDeviceBackoff.MaxElapsedTime = maxDuration

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkLoopDevices, loopDeviceBackoff, loopDeviceNotify); err != nil {
		return fmt.Errorf("loopback device still attached after %3.2f seconds", maxDuration.Seconds())
	} else {
		Logc(ctx).WithFields(log.Fields{
			"nfsMountpoint": nfsMountpoint,
			"loopFile":      loopFile,
		}).Debug("Loopback device detached.")

		return nil
	}
}

// GetMountedLoopDevices returns a list of loop devices that are *mounted* on this host.
func GetMountedLoopDevices(ctx context.Context) ([]string, error) {
	Logc(ctx).Debug(">>>> bof.GetMountedLoopDevices")
	defer Logc(ctx).Debug("<<<< bof.GetMountedLoopDevices")

	procSelfMountInfo, err := listProcSelfMountinfo(procSelfMountinfoPath)
	if err != nil {
		return nil, err
	}

	// Get a list of all mounted /dev/loop devices
	mountedLoopDevices := make([]string, 0)
	for _, procMount := range procSelfMountInfo {

		hasLoopSourceSourcePrefix := strings.HasPrefix(procMount.MountSource, "/dev/loop")

		if !hasLoopSourceSourcePrefix {
			continue
		}

		Logc(ctx).WithFields(log.Fields{
			"loopDevice": procMount.MountSource,
			"mountpoint": procMount.MountPoint,
		}).Debug("Found mounted loop device.")

		mountedLoopDevices = append(mountedLoopDevices, procMount.MountSource)
	}

	return mountedLoopDevices, nil
}

// IsLoopDeviceMounted returns true if loop device is mounted
func IsLoopDeviceMounted(ctx context.Context, deviceName string) (mounted bool, err error) {
	Logc(ctx).WithField("deviceName", deviceName).Debug(">>>> bof.IsLoopDeviceMounted")
	defer Logc(ctx).WithFields(log.Fields{
		"deviceName": deviceName,
		"mounted":    mounted,
		"err":        err,
	}).Debug("<<<< bof.IsLoopDeviceMounted")

	mountedLoopDevices, err := GetMountedLoopDevices(ctx)
	if err != nil {
		return
	}

	for _, mountedLoopDevice := range mountedLoopDevices {
		if mountedLoopDevice == deviceName {
			Logc(ctx).Debug("Loop device is mounted.")
			mounted = true
			return
		}
	}

	return
}

// SafeToRemoveNFSMount returns true if NFS mount point has any subvolumes in use by loop, if not returns true
func SafeToRemoveNFSMount(ctx context.Context, nfsMountPoint string) bool {
	Logc(ctx).WithField("nfsMountPoint", nfsMountPoint).Debug(">>>> bof.SafeToRemoveNFSMount")
	defer Logc(ctx).WithField("nfsMountPoint", nfsMountPoint).Debug("<<<< bof.SafeToRemoveNFSMount")

	// Get names of all the loop backing files
	allBackFiles, err := GetAllLoopDeviceBackFiles(ctx)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error": err,
		}).Warn("Failed to get backing files.")

		return false
	}

	if len(allBackFiles) == 0 {
		Logc(ctx).Debug("No backfile found.")
		return true
	}

	// Get list of all the files in NFS volumes
	nfsFiles := make([]string, 0)

	files, err := ioutil.ReadDir(nfsMountPoint)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error": err,
		}).Warn("Failed to get list of filename in NFS mountpoint.")

		return false
	}

	if len(files) == 0 {
		Logc(ctx).Debug("No subvolume found in NFS mountpoint.")
		return true
	}

	for _, file := range files {
		if !file.IsDir() {
			nfsFiles = append(nfsFiles, file.Name())
		}
	}

	// Find intersection
	if _, containSome := SliceContainsElements(nfsFiles, allBackFiles); containSome {
		Logc(ctx).Debug("NFS mountpoint contain subvolumes in use.")
		return false
	}

	Logc(ctx).Debug("NFS mountpoint does not contain subvolumes in use.")
	return true
}
