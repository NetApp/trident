// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

//go:generate mockgen -destination=../../mocks/mock_utils/mock_fcp/mock_fcp_client.go github.com/netapp/trident/utils/fcp FCP

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/netapp/trident/internal/fiji"
	"github.com/netapp/trident/pkg/collection"

	. "github.com/netapp/trident/logging"

	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/osutils"
)

const (
	DevPrefix                 = "/dev/"
	DevMapperRoot             = "/dev/mapper/"
	devicesRemovalMaxWaitTime = 5 * time.Second

	// REDACTED is a copy of what is in utils package.
	// we can reference that once we do not have any references into the utils package
	REDACTED = "<REDACTED>"
)

var (
	pidRegex                 = regexp.MustCompile(`^\d+$`)
	pidRunningOrIdleRegex    = regexp.MustCompile(`pid \d+ (running|idle)`)
	tagsWithIndentationRegex = regexp.MustCompile(`(?m)^[\t ]*find_multipaths[\t ]*["|']?(?P<tagName>[\w-_]+)["|']?[\t ]*$`)
	beforeFlushDevice        = fiji.Register("beforeFlushDevice", "devices")
)

type FCP interface {
	AttachVolumeRetry(
		ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo,
		secrets map[string]string, timeout time.Duration,
	) (int64, error)
	PreChecks(ctx context.Context) error
	RescanDevices(ctx context.Context, targetWWNN string, lunID int32, minSize int64) error
	IsAlreadyAttached(ctx context.Context, lunID int, targetWWNN string) bool
	GetDeviceInfoForLUN(
		ctx context.Context, hostSessionMap []map[string]int, lunID int, fcpNodeName string, needFSType bool,
	) (*models.ScsiDeviceInfo, error)
	PrepareDeviceForRemoval(
		ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
		allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool,
	) (string, error)
	GetDeviceInfoForFCPLUN(
		ctx context.Context, hostSessionMap []map[string]int, lunID int, iSCSINodeName string, isDetachCall bool,
	) (*models.ScsiDeviceInfo, error)
}

// DefaultSelfHealingExclusion Exclusion list contains keywords if found in any Target WWNN should not be considered for
// self-healing.
var DefaultSelfHealingExclusion []string

type OS interface {
	PathExists(path string) (bool, error)
}

type Devices interface {
	WaitForDevice(ctx context.Context, device string) error
	GetDeviceFSType(ctx context.Context, device string) (string, error)
	NewLUKSDevice(rawDevicePath, volumeId string) (luks.Device, error)
	EnsureLUKSDeviceMappedOnHost(ctx context.Context, name string, secrets map[string]string) (bool, error)
	IsDeviceUnformatted(ctx context.Context, device string) (bool, error)
	EnsureDeviceReadable(ctx context.Context, device string) error
}

type FileSystem interface {
	FormatVolume(ctx context.Context, device, fstype, options string) error
	RepairVolume(ctx context.Context, device, fstype string)
}

type Mount interface {
	IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error)
	MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error)
}

type Client struct {
	chrootPathPrefix     string
	command              tridentexec.Command
	selfHealingExclusion []string
	osClient             OS
	deviceClient         devices.Devices
	fileSystemClient     FileSystem
	mountClient          Mount
	fcpUtils             FcpReconcileUtils
}

func New() (*Client, error) {
	chrootPathPrefix := osutils.ChrootPathPrefix

	osClient := osutils.New()
	reconcileutils := NewReconcileUtils(chrootPathPrefix, osClient)

	mountClient, err := mount.New()
	if err != nil {
		return nil, err
	}

	fileSystemClient := filesystem.New(mountClient)

	return NewDetailed(
		chrootPathPrefix, tridentexec.NewCommand(), DefaultSelfHealingExclusion, osClient,
		devices.New(), fileSystemClient, mountClient, reconcileutils,
	), nil
}

func NewDetailed(
	chrootPathPrefix string, command tridentexec.Command, selfHealingExclusion []string,
	osClient OS, deviceClient devices.Devices, fs FileSystem, mount Mount,
	fcpUtils FcpReconcileUtils,
) *Client {
	return &Client{
		chrootPathPrefix:     chrootPathPrefix,
		command:              command,
		osClient:             osClient,
		deviceClient:         deviceClient,
		fileSystemClient:     fs,
		mountClient:          mount,
		fcpUtils:             fcpUtils,
		selfHealingExclusion: selfHealingExclusion,
	}
}

// AttachVolumeRetry attaches a volume with retry by invoking AttachVolume with backoff.
func (client *Client) AttachVolumeRetry(
	ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
	timeout time.Duration,
) (int64, error) {
	Logc(ctx).Debug(">>>> fcp.AttachVolumeRetry")
	defer Logc(ctx).Debug("<<<< fcp.AttachVolumeRetry")
	var err error
	var mpathSize int64

	if err = client.PreChecks(ctx); err != nil {
		return mpathSize, err
	}

	checkAttachFCPVolume := func() error {
		mpathSize, err = client.AttachVolume(ctx, name, mountpoint, publishInfo, secrets)
		return err
	}

	attachNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
		}).Debug("Attach FCP volume is not complete, waiting.")
	}

	attachBackoff := backoff.NewExponentialBackOff()
	attachBackoff.InitialInterval = 1 * time.Second
	attachBackoff.Multiplier = 1.414 // approx sqrt(2)
	attachBackoff.RandomizationFactor = 0.1
	attachBackoff.MaxElapsedTime = timeout

	err = backoff.RetryNotify(checkAttachFCPVolume, attachBackoff, attachNotify)
	return mpathSize, err
}

func (client *Client) RescanDevices(ctx context.Context, targetWWNN string, lunID int32, minSize int64) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	fields := LogFields{"targetWWNN": targetWWNN, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.RescanDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.RescanDevices")

	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, targetWWNN)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no FCP hosts found for target %s", targetWWNN)
	}

	deviceInfo, err := client.GetDeviceInfoForLUN(ctx, hostSessionMap, int(lunID), targetWWNN, false)
	if err != nil {
		return fmt.Errorf("error getting FCP device information: %s", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get FCP device information for LUN: %d", lunID)
	}

	allLargeEnough := true
	for _, diskDevice := range deviceInfo.Devices {
		size, err := client.deviceClient.GetDiskSize(ctx, DevPrefix+diskDevice)
		if err != nil {
			return err
		}
		if size < minSize {
			allLargeEnough = false
		} else {
			continue
		}

		err = client.rescanDisk(ctx, diskDevice)
		if err != nil {
			Logc(ctx).WithField("diskDevice", diskDevice).Error("Failed to rescan disk.")
			return fmt.Errorf("failed to rescan disk %s: %s", diskDevice, err)
		}
	}

	if !allLargeEnough {
		time.Sleep(time.Second)
		for _, diskDevice := range deviceInfo.Devices {
			size, err := client.deviceClient.GetDiskSize(ctx, DevPrefix+diskDevice)
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
		size, err := client.deviceClient.GetDiskSize(ctx, DevPrefix+multipathDevice)
		if err != nil {
			return err
		}

		fields = LogFields{"size": size, "minSize": minSize}
		if size < minSize {
			Logc(ctx).WithFields(fields).Debug("Reloading the multipath device.")
			err := client.reloadMultipathDevice(ctx, multipathDevice)
			if err != nil {
				return err
			}
			time.Sleep(time.Second)
			size, err = client.deviceClient.GetDiskSize(ctx, DevPrefix+multipathDevice)
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

// rescanDisk causes the kernel to rescan a single SCSI disk/block device.
// This is how size changes are found when expanding a volume.
func (client *Client) rescanDisk(ctx context.Context, deviceName string) error {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.rescanDisk")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.rescanDisk")

	client.deviceClient.ListAllDevices(ctx)
	filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/block/%s/device/rescan", deviceName)
	Logc(ctx).WithField("filename", filename).Debug("Opening file for writing.")

	f, err := os.OpenFile(filename, os.O_WRONLY, 0)
	if err != nil {
		Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
		return err
	}

	defer func() {
		_ = f.Close()
	}()

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

	client.deviceClient.ListAllDevices(ctx)
	return nil
}

func (client *Client) reloadMultipathDevice(ctx context.Context, multipathDevice string) error {
	fields := LogFields{"multipathDevice": multipathDevice}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.reloadMultipathDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.reloadMultipathDevice")

	if multipathDevice == "" {
		return fmt.Errorf("cannot reload an empty multipathDevice")
	}

	_, err := client.command.ExecuteWithTimeout(ctx, "multipath", 10*time.Second, true, "-r",
		DevPrefix+multipathDevice)
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

// IsAlreadyAttached checks if there is already an established FCP session to the specified LUN.
func (client *Client) IsAlreadyAttached(ctx context.Context, lunID int, targetWWNN string) bool {
	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, targetWWNN)
	if len(hostSessionMap) == 0 {
		return false
	}

	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := client.fcpUtils.GetDevicesForLUN(paths)
	if nil != err {
		return false
	}

	// return true even if a single device exists
	return 0 < len(devices)
}

// AttachVolume attaches the volume to the local host.
// This method must be able to accomplish its task using only the publish information passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.
// If the mountpoint parameter is specified, the volume will be mounted to it.
// The device path is set on the in-out publishInfo parameter so that it may be mounted later instead.
// If multipath device size is found to be inconsistent with device size, then the correct size is returned.
func (client *Client) AttachVolume(
	ctx context.Context, name, mountPoint string, publishInfo *models.VolumePublishInfo,
	secrets map[string]string,
) (int64, error) {
	Logc(ctx).Debug(">>>> fcp.AttachVolume")
	defer Logc(ctx).Debug("<<<< fcp.AttachVolume")

	var err error
	var mpathSize int64
	lunID := int(publishInfo.FCPLunNumber)

	Logc(ctx).WithFields(LogFields{
		"volume":     name,
		"mountPoint": mountPoint,
		"lunID":      lunID,
		"targetWWNN": publishInfo.FCTargetWWNN,
		"fstype":     publishInfo.FilesystemType,
	}).Debug("Attaching FCP SCSI volume.")

	if err = client.PreChecks(ctx); err != nil {
		return mpathSize, err
	}

	// Check if zoning exists for the target
	if isZoned, err := client.fcpUtils.CheckZoningExistsWithTarget(ctx, publishInfo.FCTargetWWNN); !isZoned {
		return mpathSize, err
	}

	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, publishInfo.FCTargetWWNN)
	if len(hostSessionMap) == 0 {
		return mpathSize, fmt.Errorf("no FCP hosts found for target %s", publishInfo.FCTargetWWNN)
	}
	// First attempt to fix invalid serials by rescanning them
	err = client.handleInvalidSerials(ctx, hostSessionMap, lunID, publishInfo.FCTargetWWNN, publishInfo.FCPLunSerial,
		rescanOneLun)
	if err != nil {
		return mpathSize, err
	}

	// Then attempt to fix invalid serials by purging them (to be scanned
	// again later)
	err = client.handleInvalidSerials(ctx, hostSessionMap, lunID, publishInfo.FCTargetWWNN, publishInfo.FCPLunSerial,
		purgeOneLun)
	if err != nil {
		return mpathSize, err
	}

	// Scan the target and wait for the device(s) to appear
	err = client.waitForDeviceScan(ctx, hostSessionMap, lunID, publishInfo.FCTargetWWNN)
	if err != nil {
		Logc(ctx).Errorf("Could not find FCP SCSI device: %+v", err)
		return mpathSize, err
	}

	// At this point if the serials are still invalid, give up so the
	// caller can retry (invoking the remediation steps above in the
	// process, if they haven't already been run).
	failHandler := func(ctx context.Context, path string) error {
		Logc(ctx).Error("Detected LUN serial number mismatch, attaching volume would risk data corruption, giving up")
		return fmt.Errorf("LUN serial number mismatch, kernel has stale cached data")
	}
	err = client.handleInvalidSerials(ctx, hostSessionMap, lunID, publishInfo.FCTargetWWNN, publishInfo.FCPLunSerial,
		failHandler)
	if err != nil {
		return mpathSize, err
	}

	// Wait for multipath device i.e. /dev/dm-* for the given LUN
	err = client.waitForMultipathDeviceForLUN(ctx, hostSessionMap, lunID, publishInfo.FCTargetWWNN)
	if err != nil {
		return mpathSize, err
	}

	// Lookup all the SCSI device information
	deviceInfo, err := client.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, publishInfo.FCTargetWWNN, false)
	if err != nil {
		return mpathSize, fmt.Errorf("error getting FCP device information: %v", err)
	} else if deviceInfo == nil {
		return mpathSize, fmt.Errorf("could not get FCP device information for LUN %d", lunID)
	}

	Logc(ctx).WithFields(LogFields{
		"scsiLun":         deviceInfo.LUN,
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
		"WWNN":            deviceInfo.WWNN,
	}).Debug("Found device.")

	// Make sure we use the proper device
	deviceToUse := deviceInfo.MultipathDevice

	// To avoid LUN ID conflict with a ghost device below checks
	// are necessary:
	// Conflict 1: Due to race conditions, it is possible a ghost
	//             DM device is discovered instead of the actual
	//             DM device.
	// Conflict 2: Some OS like RHEL displays the ghost device size
	//             instead of the actual LUN size.
	//
	// Below check ensures that the correct device with the correct
	// size is being discovered.

	// If LUN Serial Number exists, then compare it with DM
	// device's UUID in sysfs
	if err = client.verifyMultipathDeviceSerial(ctx, deviceToUse, publishInfo.FCPLunSerial); err != nil {
		return mpathSize, err
	}

	// Once the multipath device has been found, compare its size with
	// the size of one of the devices, if it differs then mark it for
	// resize after the staging.
	correctMpathSize, mpathSizeCorrect, err := client.deviceClient.VerifyMultipathDeviceSize(ctx, deviceToUse,
		deviceInfo.Devices[0])
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"scsiLun":         deviceInfo.LUN,
			"multipathDevice": deviceInfo.MultipathDevice,
			"device":          deviceInfo.Devices[0],
			"WWNN":            deviceInfo.WWNN,
			"err":             err,
		}).Error("Failed to verify multipath device size.")

		return mpathSize, fmt.Errorf("failed to verify multipath device %s size", deviceInfo.MultipathDevice)
	}

	if !mpathSizeCorrect {
		mpathSize = correctMpathSize

		Logc(ctx).WithFields(LogFields{
			"scsiLun":         deviceInfo.LUN,
			"multipathDevice": deviceInfo.MultipathDevice,
			"device":          deviceInfo.Devices[0],
			"WWNN":            deviceInfo.WWNN,
			"mpathSize":       mpathSize,
		}).Error("Multipath device size does not match device size.")
	}

	if deviceToUse == "" {
		return mpathSize, fmt.Errorf("could not determine device to use for %v", name)
	}

	devicePath := "/dev/" + deviceToUse
	if err := client.deviceClient.WaitForDevice(ctx, devicePath); err != nil {
		return mpathSize, fmt.Errorf("could not find device %v; %s", devicePath, err)
	}

	var isLUKSDevice, luksFormatted bool
	if publishInfo.LUKSEncryption != "" {
		isLUKSDevice, err = strconv.ParseBool(publishInfo.LUKSEncryption)
		if err != nil {
			return mpathSize, fmt.Errorf("could not parse LUKSEncryption into a bool, got %v",
				publishInfo.LUKSEncryption)
		}
	}
	// Return the device in the publish info in case the mount will be done later
	publishInfo.DevicePath = devicePath

	if isLUKSDevice {
		luksDevice := luks.NewLUKSDevice(devicePath, name, client.command)
		luksFormatted, err = luksDevice.EnsureLUKSDeviceMappedOnHost(ctx, name, secrets)
		if err != nil {
			return mpathSize, err
		}
		devicePath = luksDevice.MappedDevicePath()
	}

	if publishInfo.FilesystemType == filesystem.Raw {
		return mpathSize, nil
	}

	existingFstype, err := client.deviceClient.GetDeviceFSType(ctx, devicePath)
	if err != nil {
		return mpathSize, err
	}
	if existingFstype == "" {
		if !isLUKSDevice {
			if unformatted, err := client.deviceClient.IsDeviceUnformatted(ctx, devicePath); err != nil {
				Logc(ctx).WithField(
					"device", devicePath,
				).WithError(err).Errorf("Unable to identify if the device is unformatted; err: %v", err)
				return mpathSize, err
			} else if !unformatted {
				Logc(ctx).WithError(err).
					WithField("device", devicePath).Errorf("Device is not unformatted; err: %v", err)
				return mpathSize, fmt.Errorf("device %v is not unformatted", devicePath)
			}
		} else {
			// We can safely assume if we just luksFormatted the device, we can also add a filesystem without dataloss
			if !luksFormatted {
				Logc(ctx).WithField(
					"device", devicePath,
				).Errorf("Unable to identify if the luks device is empty; err: %v", err)
				return mpathSize, err
			}
		}

		Logc(ctx).WithFields(LogFields{"volume": name, "fstype": publishInfo.FilesystemType}).Debug("Formatting LUN.")
		err := client.fileSystemClient.FormatVolume(
			ctx, devicePath, publishInfo.FilesystemType, publishInfo.FormatOptions,
		)
		if err != nil {
			return mpathSize, fmt.Errorf("error formatting LUN %s, device %s: %v", name, deviceToUse, err)
		}
	} else if existingFstype != filesystem.UnknownFstype && existingFstype != publishInfo.FilesystemType {
		Logc(ctx).WithFields(LogFields{
			"volume":          name,
			"existingFstype":  existingFstype,
			"requestedFstype": publishInfo.FilesystemType,
		}).Error("LUN already formatted with a different file system type.")
		return mpathSize, fmt.Errorf("LUN %s, device %s already formatted with other filesystem: %s",
			name, deviceToUse, existingFstype)
	} else {
		Logc(ctx).WithFields(LogFields{
			"volume": name,
			"fstype": deviceInfo.Filesystem,
		}).Debug("LUN already formatted.")
	}

	// Attempt to resolve any filesystem inconsistencies that might be due to dirty node shutdowns, cloning
	// in-use volumes, or creating volumes from snapshots taken from in-use volumes.  This is only safe to do
	// if a device is not mounted.  The fsck command returns a non-zero exit code if filesystem errors are found,
	// even if they are completely and automatically fixed, so we don't return any error here.
	mounted, err := client.mountClient.IsMounted(ctx, devicePath, "", "")
	if err != nil {
		return mpathSize, err
	}
	if !mounted {
		client.fileSystemClient.RepairVolume(ctx, devicePath, publishInfo.FilesystemType)
	}

	// Optionally mount the device
	if mountPoint != "" {
		if err := client.mountClient.MountDevice(ctx, devicePath, mountPoint, publishInfo.MountOptions,
			false); err != nil {
			return mpathSize, fmt.Errorf("error mounting LUN %v, device %v, mountpoint %v; %s",
				name, deviceToUse, mountPoint, err)
		}
	}

	return mpathSize, nil
}

// GetDeviceInfoForLUN finds FCP devices using /dev/disk/by-path values.  This method should be
// called after calling waitForDeviceScan so that the device paths are known to exist.
func (client *Client) GetDeviceInfoForLUN(
	ctx context.Context, hostSessionMap []map[string]int, lunID int, fcpNodeName string, needFSType bool,
) (*models.ScsiDeviceInfo, error) {
	fields := LogFields{
		"lunID":       lunID,
		"fcpNodeName": fcpNodeName,
		"needFSType":  needFSType,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.getDeviceInfoForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.getDeviceInfoForLUN")

	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devicesForLUN, err := client.fcpUtils.GetDevicesForLUN(paths)
	if err != nil {
		return nil, err
	} else if 0 == len(devicesForLUN) {
		return nil, fmt.Errorf("scan not completed for LUN %d on target %s", lunID, fcpNodeName)
	}

	multipathDevice := ""
	for _, device := range devicesForLUN {
		multipathDevice = client.deviceClient.FindMultipathDeviceForDevice(ctx, device)
		if multipathDevice != "" {
			break
		}
	}

	var devicePath string
	if multipathDevice != "" {
		devicePath = DevPrefix + multipathDevice
	} else {
		devicePath = DevPrefix + devicesForLUN[0]
	}

	fsType := ""
	if needFSType {
		err = client.deviceClient.EnsureDeviceReadable(ctx, devicePath)
		if err != nil {
			return nil, err
		}

		fsType, err = client.deviceClient.GetDeviceFSType(ctx, devicePath)
		if err != nil {
			return nil, err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"lun":             strconv.Itoa(lunID),
		"multipathDevice": multipathDevice,
		"fsType":          fsType,
		"deviceNames":     devicesForLUN,
		"hostSessionMap":  hostSessionMap,
	}).Debug("Found SCSI device.")

	info := &models.ScsiDeviceInfo{
		ScsiDeviceAddress: models.ScsiDeviceAddress{
			LUN: strconv.Itoa(lunID),
		},
		MultipathDevice: multipathDevice,
		Devices:         devicesForLUN,
		DevicePaths:     paths,
		Filesystem:      fsType,
		WWNN:            fcpNodeName,
	}

	return info, nil
}

// purgeOneLun issues a delete for one LUN, based on the sysfs path
func purgeOneLun(ctx context.Context, path string) error {
	Logc(ctx).WithField("path", path).Debug("Purging one LUN")
	filename := path + "/delete"

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200)
	if err != nil {
		Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
		return err
	}
	defer f.Close()

	// Deleting a LUN is achieved by writing the string "1" to the "delete" file
	written, err := f.WriteString("1")
	if err != nil {
		Logc(ctx).WithFields(LogFields{"file": filename, "error": err}).Warning("Could not write to file.")
		return err
	}
	if written == 0 {
		Logc(ctx).WithField("file", filename).Warning("No data written to file.")
		return errors.New("too few bytes written to sysfs file")
	}

	return nil
}

// rescanOneLun issues a rescan for one LUN, based on the sysfs path
func rescanOneLun(ctx context.Context, path string) error {
	Logc(ctx).WithField("path", path).Debug("Rescaning one LUN")
	filename := path + "/rescan"

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200)
	if err != nil {
		Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
		return err
	}
	defer f.Close()

	written, err := f.WriteString("1")
	if err != nil {
		Logc(ctx).WithFields(LogFields{"file": filename, "error": err}).Warning("Could not write to file.")
		return err
	}
	if written == 0 {
		Logc(ctx).WithField("file", filename).Warning("No data written to file.")
		return errors.New("too few bytes written to sysfs file")
	}

	return nil
}

// waitForMultipathDeviceForLUN
// for the given LUN, this function waits for the associated multipath device to be present
// first find the /dev/sd* devices assocaited with the LUN
// Wait for the maultipath device dm-* for the /dev/sd* devices.
func (client *Client) waitForMultipathDeviceForLUN(ctx context.Context, hostSessionMap []map[string]int, lunID int,
	fcpNodeName string,
) error {
	fields := LogFields{
		"lunID":       lunID,
		"fcpNodeName": fcpNodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.waitForMultipathDeviceForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.waitForMultipathDeviceForLUN")

	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := client.fcpUtils.GetDevicesForLUN(paths)
	if err != nil {
		return err
	}

	_, err = client.waitForMultipathDeviceForDevices(ctx, devices)

	return err
}

// waitForMultipathDeviceForDevices accepts a list of sd* device names which are associated with same LUN
// and waits until a multipath device is present for at least one of those.  It returns the name of the
// multipath device, or an empty string if multipathd isn't running or there is only one path.
func (client *Client) waitForMultipathDeviceForDevices(ctx context.Context, devices []string) (string, error) {
	fields := LogFields{"devices": devices}

	Logc(ctx).WithFields(fields).Debug(">>>> fcp.waitForMultipathDeviceForDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.waitForMultipathDeviceForDevices")

	multipathDevice := ""

	for _, device := range devices {
		multipathDevice = client.deviceClient.FindMultipathDeviceForDevice(ctx, device)
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

// waitForDeviceScan scans all paths to a specific LUN and waits until all
// SCSI disk-by-path devices for that LUN are present on the host.
func (client *Client) waitForDeviceScan(ctx context.Context, hostSessionMap []map[string]int, lunID int,
	fcpNodeName string,
) error {
	fields := LogFields{
		"lunID":       lunID,
		"fcpNodeName": fcpNodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.waitForDeviceScan")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.waitForDeviceScan")

	Logc(ctx).WithField("hostSessionMap", hostSessionMap).Debug("Built FCP host/session map.")

	deviceAddresses := make([]models.ScsiDeviceAddress, 0)
	var hostNum, targetID int
	var err error

	// Construct the host and target lists required for the scan.
	for _, hostNumber := range hostSessionMap {
		for host := range hostNumber {
			re := regexp.MustCompile(`rport-(\d+):`)
			hostStr := re.FindStringSubmatch(host)
			if len(hostStr) > 1 {
				if hostNum, err = strconv.Atoi(hostStr[1]); err != nil {
					Logc(ctx).WithField("port", hostNum).Error("Could not parse port number")
					continue
				}
			}

			// Read the target IDs from the sysfs path.
			targetIDFile := fmt.Sprintf("/sys/class/fc_host/host%d/device/%s/fc_remote_ports/%s/scsi_target_id",
				hostNum, host, host)
			if targetIDRaw, err := os.ReadFile(targetIDFile); err != nil {
				Logc(ctx).WithField("targetIDFile", targetIDFile).Error("Could not find the target ID file")
			} else {
				targetIDStr := strings.TrimSpace(string(targetIDRaw))
				if targetID, err = strconv.Atoi(targetIDStr); err != nil {
					Logc(ctx).WithField("targetID", targetIDStr).Error("Could not parse target ID")
					continue
				}

				deviceAddress := models.ScsiDeviceAddress{
					Host:    strconv.Itoa(hostNum),
					Channel: models.ScanAllSCSIDeviceAddress,
					Target:  strconv.Itoa(targetID),
					LUN:     strconv.Itoa(lunID),
				}
				if !collection.Contains(deviceAddresses, deviceAddress) {
					deviceAddresses = append(deviceAddresses, deviceAddress)
				}
			}
		}
	}

	if err := client.deviceClient.ScanTargetLUN(ctx, deviceAddresses); err != nil {
		Logc(ctx).WithField("scanError", err).Error("Could not scan for new LUN.")
	}

	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	Logc(ctx).Debugf("Scanning paths: %v", paths)
	found := make([]string, 0)

	allDevicesExist := true

	// Check if all paths present, and return nil (success) if so
	for _, p := range paths {
		dirname := p + "/block"
		exists, err := client.osClient.PathExists(dirname)
		if !exists || err != nil {
			// Set flag to false as device is missing
			allDevicesExist = false
		} else {
			found = append(found, dirname)
			Logc(ctx).Debugf("Paths found: %v", dirname)
		}
	}

	if len(found) == 0 {

		Logc(ctx).Warnf("Could not find any devices ")

		// log info about current status of host when no devices are found
		if _, err := client.command.Execute(ctx, "ls", "-al", "/dev"); err != nil {
			Logc(ctx).Warnf("Could not run ls -al /dev: %v", err)
		}
		if _, err := client.command.Execute(ctx, "ls", "-al", DevMapperRoot); err != nil {
			Logc(ctx).Warnf("Could not run ls -al %s: %v", DevMapperRoot, err)
		}
		if _, err := client.command.Execute(ctx, "ls", "-al", "/dev/disk/by-path"); err != nil {
			Logc(ctx).Warnf("Could not run ls -al /dev/disk/by-path: %v", err)
		}
		if _, err := client.command.Execute(ctx, "lsscsi"); err != nil {
			Logc(ctx).Infof("Could not collect debug info, run lsscsi: %v", err)
		}
		if _, err := client.command.Execute(ctx, "lsscsi", "-t"); err != nil {
			Logc(ctx).Infof("Could not collect debug info, run lsscsi -t: %v", err)
		}
		if _, err := client.command.Execute(ctx, "free"); err != nil {
			Logc(ctx).Warnf("Could not run free: %v", err)
		}

		return errors.New("no devices present yet")

	}

	if allDevicesExist {
		// We have found all devices.
		Logc(ctx).Debugf("All Paths found: %v", found)
	} else {
		// We have found some devices but not all.
		Logc(ctx).Debugf("Some Paths found: %v", found)
	}
	return nil
}

// handleInvalidSerials checks the LUN serial number for each path of a given LUN, and
// if it doesn't match the expected value, runs a handler function.
func (client *Client) handleInvalidSerials(
	ctx context.Context, hostSessionMap []map[string]int, lunID int, targetWWNN, expectedSerial string,
	handler func(ctx context.Context, path string) error,
) error {
	if "" == expectedSerial {
		// Empty string means don't care
		return nil
	}

	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	for _, path := range paths {
		serial, err := client.deviceClient.GetLunSerial(ctx, path)
		if err != nil {
			if os.IsNotExist(err) {
				// LUN either isn't scanned yet, or this kernel
				// doesn't support VPD page 80 in sysfs. Assume
				// correctness and move on
				Logc(ctx).WithFields(LogFields{
					"lun":    lunID,
					"target": targetWWNN,
					"path":   path,
				}).Debug("LUN serial check skipped")
				continue
			}
			return err
		}

		if serial != expectedSerial {
			Logc(ctx).WithFields(LogFields{
				"expected": expectedSerial,
				"actual":   serial,
				"lun":      lunID,
				"target":   targetWWNN,
				"path":     path,
			}).Warn("LUN serial check failed")
			err = handler(ctx, path)
			if err != nil {
				return err
			}
		} else {
			Logc(ctx).WithFields(LogFields{
				"serial": serial,
				"lun":    lunID,
				"target": targetWWNN,
				"path":   path,
			}).Debug("LUN serial check passed")
		}
	}

	return nil
}

// verifyMultipathDeviceSerial compares the serial number of the DM device with the serial
// of the LUN to ensure correct DM device has been discovered
func (client *Client) verifyMultipathDeviceSerial(
	ctx context.Context, multipathDevice, lunSerial string,
) error {
	if lunSerial == "" {
		// Empty string means don't care
		return nil
	}

	// Multipath UUID contains LUN serial in hex format
	lunSerialHex := hex.EncodeToString([]byte(lunSerial))

	multipathDeviceUUID, err := client.deviceClient.GetMultipathDeviceUUID(multipathDevice)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// If UUID does not exist, then it is hard to verify the DM serial
			Logc(ctx).WithFields(LogFields{
				"multipathDevice": multipathDevice,
				"lunSerialNumber": lunSerial,
			}).Warn("Unable to verify multipath device serial.")

			return nil
		}

		Logc(ctx).WithFields(LogFields{
			"multipathDevice": multipathDevice,
			"lunSerialNumber": lunSerial,
			"error":           err,
		}).Error("Failed to verify multipath device serial.")

		return err
	}

	if !strings.Contains(multipathDeviceUUID, lunSerialHex) {
		Logc(ctx).WithFields(LogFields{
			"multipathDevice":     multipathDevice,
			"lunSerialNumber":     lunSerial,
			"lunSerialNumberHex":  lunSerialHex,
			"multipathDeviceUUID": multipathDeviceUUID,
		}).Error("Failed to verify multipath device serial.")

		return fmt.Errorf("multipath device '%s' serial check failed", multipathDevice)
	}

	Logc(ctx).WithFields(LogFields{
		"multipathDevice":     multipathDevice,
		"lunSerialNumber":     lunSerial,
		"lunSerialNumberHex":  lunSerialHex,
		"multipathDeviceUUID": multipathDeviceUUID,
	}).Debug("Multipath device serial check passed.")

	return nil
}

// PreChecks to check if all the required tools are present and configured correctly for the  volume
// attachment to go through
func (client *Client) PreChecks(ctx context.Context) error {
	if !client.multipathdIsRunning(ctx) {
		return fmt.Errorf("multipathd is not running")
	}

	findMultipathsValue, err := client.identifyFindMultipathsValue(ctx)
	if err != nil {
		// If Trident is unable to find the find_multipaths value, assume it to be default "no"
		Logc(ctx).Errorf("unable to get the find_multipaths value from the /etc/multipath.conf: %v", err)
	}

	if findMultipathsValue == "yes" || findMultipathsValue == "smart" {
		return fmt.Errorf(
			"multipathd: unsupported find_multipaths: %s value;"+
				" please set the value to no in /etc/multipath.conf file", findMultipathsValue,
		)
	}

	return nil
}

// multipathdIsRunning returns true if the multipath daemon is running.
func (client *Client) multipathdIsRunning(ctx context.Context) bool {
	Logc(ctx).Debug(">>>> fcp.multipathdIsRunning")
	defer Logc(ctx).Debug("<<<< fcp.multipathdIsRunning")

	// use pgrep to look for mulipathd in the list of running processes
	out, err := client.command.Execute(ctx, "pgrep", "multipathd")
	if err == nil {
		pid := strings.TrimSpace(string(out))
		if pidRegex.MatchString(pid) {
			Logc(ctx).WithField("pid", pid).Debug("multipathd is running")
			return true
		}
	} else {
		Logc(ctx).Error(err)
	}

	out, err = client.command.Execute(ctx, "multipathd", "show", "daemon")
	if err == nil {
		if pidRunningOrIdleRegex.MatchString(string(out)) {
			Logc(ctx).Debug("multipathd is running")
			return true
		}
	} else {
		Logc(ctx).Error(err)
	}

	return false
}

// identifyFindMultipathsValue reads /etc/multipath.conf and identifies find_multipaths value (if set)
func (client *Client) identifyFindMultipathsValue(ctx context.Context) (string, error) {
	output, err := client.command.ExecuteWithTimeout(ctx, "multipathd", 5*time.Second, false, "show", "config")
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
		}).Error("Could not read multipathd configuration")

		return "", fmt.Errorf("could not read multipathd configuration: %v", err)
	}

	findMultipathsValue := getFindMultipathValue(string(output))
	Logc(ctx).WithField("findMultipathsValue", findMultipathsValue).Debug("Multipath find_multipaths value found.")
	return findMultipathsValue, nil
}

// GetFindMultipathValue returns the value of find_multipaths
// Returned values:
// no (or off): Create a multipath device for every path that is not explicitly disabled
// yes (or on): Create a device if one of some conditions are met
// other possible values: smart, greedy, strict
func getFindMultipathValue(text string) string {
	tag := tagsWithIndentationRegex.FindStringSubmatch(text)
	// Since we have two of `()` in the pattern, we want to use the tag identified by the second `()`.
	if len(tag) > 1 {
		if tag[1] == "off" {
			return "no"
		} else if tag[1] == "on" {
			return "yes"
		}

		return tag[1]
	}

	return ""
}

// PrepareDeviceForRemoval informs Linux that a device will be removed, the function
// also verifies that device being removed is correct based on published device path,
// device serial number (if present) or comparing all publications (allPublishInfos) for
// LUN number uniqueness.
func (client *Client) PrepareDeviceForRemoval(
	ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool,
) (string, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	lunID := int(publishInfo.FCPLunNumber)
	fcpNodeName := publishInfo.FCTargetWWNN

	fields := LogFields{
		"lunID":            lunID,
		"fcpNodeName":      fcpNodeName,
		"chrootPathPrefix": client.chrootPathPrefix,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.PrepareDeviceForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.PrepareDeviceForRemoval")

	// CSI Case
	// We can't verify a multipath device if we couldn't find it in sysfs.
	if deviceInfo.MultipathDevice != "" {
		_, err := client.deviceClient.VerifyMultipathDevice(ctx, publishInfo, allPublishInfos, deviceInfo)
		if err != nil {
			return "", err
		}
	}

	var multipathDevice string
	performDeferredDeviceRemoval, err := client.removeSCSIDevice(ctx, deviceInfo, ignoreErrors, force)
	if performDeferredDeviceRemoval && deviceInfo.MultipathDevice != "" {
		multipathDevice = DevPrefix + deviceInfo.MultipathDevice
		Logc(ctx).WithFields(LogFields{
			"lunID":           lunID,
			"multipathDevice": multipathDevice,
		}).Debug("Discovered unmapped multipath device when removing SCSI device.")
	}

	return multipathDevice, err
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
func (client *Client) removeSCSIDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, ignoreErrors,
	skipFlush bool,
) (bool, error) {
	client.deviceClient.ListAllDevices(ctx)

	// Flush multipath device
	if !skipFlush {
		err := client.deviceClient.MultipathFlushDevice(ctx, deviceInfo)
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
		err := client.deviceClient.FlushDevice(ctx, deviceInfo, ignoreErrors)
		if err != nil && !ignoreErrors {
			return false, err
		}
	}

	// Remove device
	err := client.deviceClient.RemoveDevice(ctx, deviceInfo.Devices, ignoreErrors)
	if err != nil && !ignoreErrors {
		return false, err
	}

	// Wait for device to be removed. Do not ignore errors here as we need the device removed
	// for the force removal of the multipath device to succeed.
	err = client.deviceClient.WaitForDevicesRemoval(ctx, DevPrefix, deviceInfo.Devices,
		devicesRemovalMaxWaitTime)
	if err != nil {
		return false, err
	}

	client.deviceClient.ListAllDevices(ctx)

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

func (client *Client) GetDeviceInfoForFCPLUN(
	ctx context.Context, hostSessionMap []map[string]int, lunID int, iSCSINodeName string, isDetachCall bool,
) (*models.ScsiDeviceInfo, error) {
	deviceInfo, err := client.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, false)
	if err != nil {
		return nil, err
	}

	return &models.ScsiDeviceInfo{
		ScsiDeviceAddress: models.ScsiDeviceAddress{
			Host:    deviceInfo.Host,
			Channel: deviceInfo.Channel,
			Target:  deviceInfo.Target,
			LUN:     deviceInfo.LUN,
		},
		Devices:         deviceInfo.Devices,
		MultipathDevice: deviceInfo.MultipathDevice,
		WWNN:            deviceInfo.WWNN,
		SessionNumber:   deviceInfo.SessionNumber,
	}, nil
}
