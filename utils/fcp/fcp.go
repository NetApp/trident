// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

//go:generate mockgen -destination=../../mocks/mock_utils/mock_fcp/mock_fcp_client.go github.com/netapp/trident/utils/fcp FCP

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/mount"

	// "github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

const (
	unknownFstype = "<unknown>"

	DevPrefix     = "/dev/"
	DevMapperRoot = "/dev/mapper/"

	// REDACTED is a copy of what is in utils package.
	// we can reference that once we do not have any references into the utils package
	REDACTED = "<REDACTED>"
)

var (
	pidRegex                 = regexp.MustCompile(`^\d+$`)
	pidRunningOrIdleRegex    = regexp.MustCompile(`pid \d+ (running|idle)`)
	tagsWithIndentationRegex = regexp.MustCompile(`(?m)^[\t ]*find_multipaths[\t ]*["|']?(?P<tagName>[\w-_]+)["|']?[\t ]*$`)

	// duringScanTargetLunAfterFileOpen          = fiji.Register("duringFCPScanTargetLunAfterFileOpen", "fcp")
	// duringConfigureTargetBeforeFCPAdmUpdate = fiji.Register("duringConfigureFCPTargetBeforeFCPAdmUpdate", "fcp")
	// duringPurgeOneLunBeforeFileWrite          = fiji.Register("duringPurgeOneLunBeforeFileWrite", "fcp")
)

type FCP interface {
	AttachVolumeRetry(
		ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo,
		secrets map[string]string, timeout time.Duration,
	) (int64, error)
	PreChecks(ctx context.Context) error
	RescanDevices(ctx context.Context, targetWWNN string, lunID int32, minSize int64) error
	IsAlreadyAttached(ctx context.Context, lunID int, targetWWNN string) bool
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
	NewLUKSDevice(rawDevicePath, volumeId string) (models.LUKSDeviceInterface, error)
	EnsureLUKSDeviceMappedOnHost(
		ctx context.Context, luksDevice models.LUKSDeviceInterface, name string,
		secrets map[string]string,
	) (bool, error)
	IsDeviceUnformatted(ctx context.Context, device string) (bool, error)
	EnsureDeviceReadable(ctx context.Context, device string) error
	GetFCPDiskSize(ctx context.Context, devicePath string) (int64, error)
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
	deviceClient         Devices
	fileSystemClient     FileSystem
	mountClient          Mount
	fcpUtils             FcpReconcileUtils
}

func New(osClient OS, deviceClient Devices, fileSystemClient FileSystem) (*Client, error) {
	chrootPathPrefix := ""
	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		chrootPathPrefix = "/host"
	}

	reconcileutils := NewReconcileUtils(chrootPathPrefix, osClient)

	mountClient, err := mount.New()
	if err != nil {
		return nil, err
	}

	return NewDetailed(chrootPathPrefix, tridentexec.NewCommand(), DefaultSelfHealingExclusion, osClient,
		deviceClient, fileSystemClient, mountClient, reconcileutils), nil
}

func NewDetailed(
	chrootPathPrefix string, command tridentexec.Command, selfHealingExclusion []string,
	osClient OS, deviceClient Devices, fileSystemClient FileSystem, mountClient Mount,
	fcpUtils FcpReconcileUtils,
) *Client {
	return &Client{
		chrootPathPrefix:     chrootPathPrefix,
		command:              command,
		osClient:             osClient,
		deviceClient:         deviceClient,
		fileSystemClient:     fileSystemClient,
		mountClient:          mountClient,
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

	deviceInfo, err := client.getDeviceInfoForLUN(ctx, int(lunID), targetWWNN, false, false)
	if err != nil {
		return fmt.Errorf("error getting FCP device information: %s", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get FCP device information for LUN: %d", lunID)
	}

	allLargeEnough := true
	for _, diskDevice := range deviceInfo.Devices {
		size, err := client.deviceClient.GetFCPDiskSize(ctx, DevPrefix+diskDevice)
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
			size, err := client.deviceClient.GetFCPDiskSize(ctx, DevPrefix+diskDevice)
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
		size, err := client.deviceClient.GetFCPDiskSize(ctx, DevPrefix+multipathDevice)
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
			// TODO (vhs): Introduce a backoff rather than a fixed delay.
			time.Sleep(time.Second)
			size, err = client.deviceClient.GetFCPDiskSize(ctx, DevPrefix+multipathDevice)
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

	listAllDevices(ctx)
	filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/block/%s/device/rescan", deviceName)
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

	listAllDevices(ctx)
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

	// First attempt to fix invalid serials by rescanning them
	err = client.handleInvalidSerials(ctx, lunID, publishInfo.FCTargetWWNN, publishInfo.FCPLunSerial, rescanOneLun)
	if err != nil {
		return mpathSize, err
	}

	// Then attempt to fix invalid serials by purging them (to be scanned
	// again later)
	err = client.handleInvalidSerials(ctx, lunID, publishInfo.FCTargetWWNN, publishInfo.FCPLunSerial, purgeOneLun)
	if err != nil {
		return mpathSize, err
	}

	// Scan the target and wait for the device(s) to appear
	err = client.waitForDeviceScan(ctx, lunID, publishInfo.FCTargetWWNN)
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
	err = client.handleInvalidSerials(ctx, lunID, publishInfo.FCTargetWWNN, publishInfo.FCPLunSerial, failHandler)
	if err != nil {
		return mpathSize, err
	}

	// Wait for multipath device i.e. /dev/dm-* for the given LUN
	err = client.waitForMultipathDeviceForLUN(ctx, lunID, publishInfo.FCTargetWWNN)
	if err != nil {
		return mpathSize, err
	}

	// Lookup all the SCSI device information
	deviceInfo, err := client.getDeviceInfoForLUN(ctx, lunID, publishInfo.FCTargetWWNN, false, false)
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
	deviceToUse := deviceInfo.Devices[0]
	if deviceInfo.MultipathDevice != "" {
		deviceToUse = deviceInfo.MultipathDevice

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
		correctMpathSize, mpathSizeCorrect, err := client.verifyMultipathDeviceSize(ctx, deviceToUse, deviceInfo.Devices[0])
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
	} else {
		return mpathSize, fmt.Errorf("could not find multipath device for LUN %d", lunID)
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

	if isLUKSDevice {
		luksDevice, _ := client.deviceClient.NewLUKSDevice(devicePath, name)
		luksFormatted, err = client.deviceClient.EnsureLUKSDeviceMappedOnHost(ctx, luksDevice, name, secrets)
		if err != nil {
			return mpathSize, err
		}
		devicePath = luksDevice.MappedDevicePath()
	}

	// Return the device in the publish info in case the mount will be done later
	publishInfo.DevicePath = devicePath

	if publishInfo.FilesystemType == config.FsRaw {
		return mpathSize, nil
	}

	existingFstype, err := client.deviceClient.GetDeviceFSType(ctx, devicePath)
	if err != nil {
		return mpathSize, err
	}
	if existingFstype == "" {
		if !isLUKSDevice {
			if unformatted, err := client.deviceClient.IsDeviceUnformatted(ctx, devicePath); err != nil {
				Logc(ctx).WithField("device",
					devicePath).Errorf("Unable to identify if the device is unformatted; err: %v", err)
				return mpathSize, err
			} else if !unformatted {
				Logc(ctx).WithField("device", devicePath).Errorf("Device is not unformatted; err: %v", err)
				return mpathSize, fmt.Errorf("device %v is not unformatted", devicePath)
			}
		} else {
			// We can safely assume if we just luksFormatted the device, we can also add a filesystem without dataloss
			if !luksFormatted {
				Logc(ctx).WithField("device",
					devicePath).Errorf("Unable to identify if the luks device is empty; err: %v", err)
				return mpathSize, err
			}
		}

		Logc(ctx).WithFields(LogFields{"volume": name, "fstype": publishInfo.FilesystemType}).Debug("Formatting LUN.")
		err := client.fileSystemClient.FormatVolume(ctx, devicePath, publishInfo.FilesystemType, publishInfo.FormatOptions)
		if err != nil {
			return mpathSize, fmt.Errorf("error formatting LUN %s, device %s: %v", name, deviceToUse, err)
		}
	} else if existingFstype != unknownFstype && existingFstype != publishInfo.FilesystemType {
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

// ScsiDeviceInfo contains information about SCSI devices
type ScsiDeviceInfo struct {
	Host            string
	Channel         string
	Target          string
	LUN             string
	Devices         []string
	DevicePaths     []string
	MultipathDevice string
	Filesystem      string
	WWNN            string
	SessionNumber   int
	HostSessionMap  map[int]int
}

// getDeviceInfoForLUN finds FCP devices using /dev/disk/by-path values.  This method should be
// called after calling waitForDeviceScan so that the device paths are known to exist.
func (client *Client) getDeviceInfoForLUN(
	ctx context.Context, lunID int, fcpNodeName string, needFSType, isDetachCall bool,
) (*ScsiDeviceInfo, error) {
	fields := LogFields{
		"lunID":       lunID,
		"fcpNodeName": fcpNodeName,
		"needFSType":  needFSType,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.getDeviceInfoForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.getDeviceInfoForLUN")

	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, fcpNodeName)

	// During detach if hostSessionMap count is zero, we should be fine
	if len(hostSessionMap) == 0 {
		if isDetachCall {
			Logc(ctx).WithFields(fields).Debug("No fcp hosts found for target.")
			return nil, nil
		} else {
			return nil, fmt.Errorf("no fcp hosts found for target %s", fcpNodeName)
		}
	}

	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := client.fcpUtils.GetDevicesForLUN(paths)
	if err != nil {
		return nil, err
	} else if 0 == len(devices) {
		return nil, fmt.Errorf("scan not completed for LUN %d on target %s", lunID, fcpNodeName)
	}

	multipathDevice := ""
	for _, device := range devices {
		multipathDevice = client.findMultipathDeviceForDevice(ctx, device)
		if multipathDevice != "" {
			break
		}
	}

	var devicePath string
	if multipathDevice != "" {
		devicePath = DevPrefix + multipathDevice
	} else {
		devicePath = DevPrefix + devices[0]
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
		"deviceNames":     devices,
		"hostSessionMap":  hostSessionMap,
	}).Debug("Found SCSI device.")

	info := &ScsiDeviceInfo{
		LUN:             strconv.Itoa(lunID),
		MultipathDevice: multipathDevice,
		Devices:         devices,
		DevicePaths:     paths,
		Filesystem:      fsType,
		WWNN:            fcpNodeName,
		// TODO (vhs): Check hostSessionMap is really required
		// HostSessionMap:  hostSessionMap,
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
func (client *Client) waitForMultipathDeviceForLUN(ctx context.Context, lunID int, fcpNodeName string) error {
	fields := LogFields{
		"lunID":       lunID,
		"fcpNodeName": fcpNodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.waitForMultipathDeviceForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.waitForMultipathDeviceForLUN")

	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, fcpNodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no FCP hosts found for target %s", fcpNodeName)
	}

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
		multipathDevice = client.findMultipathDeviceForDevice(ctx, device)
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
func (client *Client) findMultipathDeviceForDevice(ctx context.Context, device string) string {
	Logc(ctx).WithField("device", device).Debug(">>>> fcp.findMultipathDeviceForDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< fcp.findMultipathDeviceForDevice")

	holdersDir := client.chrootPathPrefix + "/sys/block/" + device + "/holders"
	if dirs, err := os.ReadDir(holdersDir); err == nil {
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

// waitForDeviceScan scans all paths to a specific LUN and waits until all
// SCSI disk-by-path devices for that LUN are present on the host.
func (client *Client) waitForDeviceScan(ctx context.Context, lunID int, fcpNodeName string) error {
	fields := LogFields{
		"lunID":       lunID,
		"fcpNodeName": fcpNodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.waitForDeviceScan")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.waitForDeviceScan")

	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, fcpNodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no FCP hosts found for target %s", fcpNodeName)
	}

	Logc(ctx).WithField("hostSessionMap", hostSessionMap).Debug("Built FCP host/session map.")
	hosts := make([]int, 0)
	var hostNum int
	var err error
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
			hosts = append(hosts, hostNum)
		}
	}

	if err := client.ScanTargetLUN(ctx, lunID, hosts); err != nil {
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

// scanTargetLUN scans a single LUN or all the LUNs on an FCP target to discover it.
// If all the LUNs are to be scanned please pass -1 for lunID.
func (client *Client) scanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	fields := LogFields{"hosts": hosts, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.scanTargetLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.scanTargetLUN")

	var (
		f   *os.File
		err error
	)

	// By default, scan for all the LUNs
	scanCmd := "0 0 -"
	if lunID >= 0 {
		scanCmd = fmt.Sprintf("0 0 %d", lunID)
	}

	listAllDevices(ctx)
	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200); err != nil {
			Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
			return err
		}

		// if err = duringScanTargetLunAfterFileOpen.Inject(); err != nil {
		//	return err
		// }

		if written, err := f.WriteString(scanCmd); err != nil {
			Logc(ctx).WithFields(LogFields{"file": filename, "error": err}).Warning("Could not write to file.")
			f.Close()
			return err
		} else if written == 0 {
			Logc(ctx).WithField("file", filename).Warning("No data written to file.")
			f.Close()
			return fmt.Errorf("no data written to %s", filename)
		}

		f.Close()

		listAllDevices(ctx)
		Logc(ctx).WithFields(LogFields{
			"scanCmd":  scanCmd,
			"scanFile": filename,
			"host":     hostNumber,
		}).Debug("Invoked SCSI scan for host.")
	}

	return nil
}

// handleInvalidSerials checks the LUN serial number for each path of a given LUN, and
// if it doesn't match the expected value, runs a handler function.
func (client *Client) handleInvalidSerials(
	ctx context.Context, lunID int, targetWWNN, expectedSerial string,
	handler func(ctx context.Context, path string) error,
) error {
	if "" == expectedSerial {
		// Empty string means don't care
		return nil
	}

	hostSessionMap := client.fcpUtils.GetFCPHostSessionMapForTarget(ctx, targetWWNN)
	paths := client.fcpUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	for _, path := range paths {
		serial, err := getLunSerial(ctx, path)
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

// getLunSerial get Linux's idea of what the LUN serial number is
func getLunSerial(ctx context.Context, path string) (string, error) {
	Logc(ctx).WithField("path", path).Debug("Get LUN Serial")
	// We're going to read the SCSI VPD page 80 serial number
	// information. Linux helpfully provides this through sysfs
	// so we don't need to open the device and send the ioctl
	// ourselves.
	filename := path + "/vpd_pg80"
	b, err := os.ReadFile(filename)
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

	multipathDeviceUUID, err := client.fcpUtils.GetMultipathDeviceUUID(multipathDevice)
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

// verifyMultipathDeviceSize compares the size of the DM device with the size
// of a device to ensure correct DM device has the correct size.
func (client *Client) verifyMultipathDeviceSize(
	ctx context.Context, multipathDevice, device string,
) (int64, bool, error) {
	deviceSize, err := client.deviceClient.GetFCPDiskSize(ctx, "/dev/"+device)
	if err != nil {
		return 0, false, err
	}

	mpathSize, err := client.deviceClient.GetFCPDiskSize(ctx, "/dev/"+multipathDevice)
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

// In the case of FCP trace debug, log info about session and what devices are present
func listAllDevices(ctx context.Context) {
	Logc(ctx).Trace(">>>> fcp.listAllDevices")
	defer Logc(ctx).Trace("<<<< fcp.listAllDevices")
	// Log information about all the devices
	dmLog := make([]string, 0)
	sdLog := make([]string, 0)
	sysLog := make([]string, 0)
	entries, _ := os.ReadDir(DevPrefix)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "dm-") {
			dmLog = append(dmLog, entry.Name())
		}
		if strings.HasPrefix(entry.Name(), "sd") {
			sdLog = append(sdLog, entry.Name())
		}
	}

	entries, _ = os.ReadDir("/sys/block/")
	for _, entry := range entries {
		sysLog = append(sysLog, entry.Name())
	}

	Logc(ctx).WithFields(LogFields{
		"/dev/dm-*":    dmLog,
		"/dev/sd*":     sdLog,
		"/sys/block/*": sysLog,
	}).Trace("Listing all FCP Devices.")
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
		return fmt.Errorf("multipathd: unsupported find_multipaths: %s value;"+
			" please set the value to no in /etc/multipath.conf file", findMultipathsValue)
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

	findMultipathsValue := GetFindMultipathValue(string(output))
	Logc(ctx).WithField("findMultipathsValue", findMultipathsValue).Debug("Multipath find_multipaths value found.")
	return findMultipathsValue, nil
}

// GetFindMultipathValue returns the value of find_multipaths
// Returned values:
// no (or off): Create a multipath device for every path that is not explicitly disabled
// yes (or on): Create a device if one of some conditions are met
// other possible values: smart, greedy, strict
func GetFindMultipathValue(text string) string {
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
