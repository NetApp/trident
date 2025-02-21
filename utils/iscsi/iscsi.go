// Copyright 2025 NetApp, Inc. All Rights Reserved.

package iscsi

//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_client.go github.com/netapp/trident/utils/iscsi ISCSI
//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_os_client.go github.com/netapp/trident/utils/iscsi OS
//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_devices_client.go github.com/netapp/trident/utils/iscsi Devices
//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_filesystem_client.go github.com/netapp/trident/utils/iscsi FileSystem
//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_mount_client.go github.com/netapp/trident/utils/iscsi Mount

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/afero"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

const (
	unknownFstype = "<unknown>"

	DevPrefix     = "/dev/"
	DevMapperRoot = "/dev/mapper/"

	sessionStateLoggedIn     = "LOGGED_IN"
	SessionInfoSource        = "sessionSource"
	sessionConnectionStateUp = "up"

	iscsiadmLoginTimeoutValue = 10
	iscsiadmLoginTimeout      = iscsiadmLoginTimeoutValue * time.Second
	iscsiadmLoginRetryMax     = "1"

	errNoObjsFound     = 21
	errLoginAuthFailed = 24

	// REDACTED is a copy of what is in utils package.
	// we can reference that once we do not have any references into the utils package
	REDACTED = "<REDACTED>"

	temporaryMountDir = "/tmp_mnt"
)

var (
	portalPortPattern     = regexp.MustCompile(`.+:\d+$`)
	pidRegex              = regexp.MustCompile(`^\d+$`)
	pidRunningOrIdleRegex = regexp.MustCompile(`pid \d+ (running|idle)`)

	duringScanTargetLunAfterFileOpen               = fiji.Register("duringISCSIScanTargetLunAfterFileOpen", "iscsi")
	duringConfigureISCSITargetBeforeISCSIAdmUpdate = fiji.Register("duringConfigureISCSITargetBeforeISCSIAdmUpdate", "iscsi")
	duringPurgeOneLunBeforeFileWrite               = fiji.Register("duringPurgeOneLunBeforeFileWrite", "iscsi")
	beforeIscsiLogout                              = fiji.Register("beforeIscsiLogout", "iscsi")
)

type ISCSI interface {
	AttachVolumeRetry(ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo,
		secrets map[string]string, timeout time.Duration) (int64, error)
	AddSession(
		ctx context.Context, sessions *models.ISCSISessions, publishInfo *models.VolumePublishInfo,
		volID, sessionNumber string, reasonInvalid models.PortalInvalid,
	)
	PreChecks(ctx context.Context) error
	RescanDevices(ctx context.Context, targetIQN string, lunID int32, minSize int64) error
	IsAlreadyAttached(ctx context.Context, lunID int, targetIqn string) bool
	RemoveLUNFromSessions(ctx context.Context, publishInfo *models.VolumePublishInfo, sessions *models.ISCSISessions)
	RemovePortalsFromSession(ctx context.Context, publishInfo *models.VolumePublishInfo, sessions *models.ISCSISessions)
	TargetHasMountedDevice(ctx context.Context, targetIQN string) (bool, error)
	SafeToLogOut(ctx context.Context, hostNumber, sessionNumber int) bool
	Logout(ctx context.Context, targetIQN, targetPortal string) error
}

// Exclusion list contains keywords if found in any Target IQN should not be considered for
// self-healing.
// solidfire: Exclude solidfire for now. Solidfire maintains a different handle 'Current Portal'
//            which is not published or captured in VolumePublishInfo, current self-healing logic does not
//            work for logout, login or scan as it is designed to work with published portal information.

var DefaultSelfHealingExclusion = []string{"solidfire"}

type OS interface {
	PathExists(path string) (bool, error)
}

type Devices interface {
	WaitForDevice(ctx context.Context, device string) error
	GetDeviceFSType(ctx context.Context, device string) (string, error)
	NewLUKSDevice(rawDevicePath, volumeId string) (models.LUKSDeviceInterface, error)
	EnsureLUKSDeviceMappedOnHost(ctx context.Context, luksDevice models.LUKSDeviceInterface, name string,
		secrets map[string]string) (bool, error)
	IsDeviceUnformatted(ctx context.Context, device string) (bool, error)
	EnsureDeviceReadable(ctx context.Context, device string) error
	GetISCSIDiskSize(ctx context.Context, devicePath string) (int64, error)
	GetMountedISCSIDevices(ctx context.Context) ([]*models.ScsiDeviceInfo, error)
	MultipathFlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo) error
	CompareWithPublishedDevicePath(
		ctx context.Context, publishInfo *models.VolumePublishInfo,
		deviceInfo *models.ScsiDeviceInfo) (bool, error)
	CompareWithPublishedSerialNumber(
		ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
	) (bool, error)
	CompareWithAllPublishInfos(
		ctx context.Context, publishInfo *models.VolumePublishInfo,
		allPublishInfos []models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
	) error
	RemoveSCSIDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, ignoreErrors, skipFlush bool) (bool, error)
	GetLUKSDeviceForMultipathDevice(multipathDevice string) (string, error)
	CloseLUKSDevice(ctx context.Context, devicePath string) error
	RemoveMultipathDeviceMapping(ctx context.Context, devicePath string) error
	GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, luksDevicePath string) (string, error)
	GetDMDeviceForMapperPath(ctx context.Context, mapperPath string) (string, error)
	GetDeviceInfoForLUN(
		ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, isDetachCall bool,
	) (*models.ScsiDeviceInfo, error)
	EnsureLUKSDeviceClosed(ctx context.Context, devicePath string) error
	EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error
	PrepareDeviceForRemoval(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
		allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool) (string, error)
	NewLUKSDeviceFromMappingPath(ctx context.Context, mappingPath, volumeId string) (models.LUKSDeviceInterface, error)
}

type FileSystem interface {
	FormatVolume(ctx context.Context, device, fstype, formatOptions string) error
	RepairVolume(ctx context.Context, device, fstype string)
}

type Mount interface {
	IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error)
	MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error)
	UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error
}

type Client struct {
	chrootPathPrefix     string
	command              tridentexec.Command
	selfHealingExclusion []string
	osClient             OS
	deviceClient         Devices
	fileSystemClient     FileSystem
	mountClient          Mount
	iscsiUtils           IscsiReconcileUtils
	os                   afero.Afero
}

func New(osClient OS, deviceClient Devices, fileSystemClient FileSystem, mountClient Mount) *Client {
	chrootPathPrefix := ""
	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		chrootPathPrefix = "/host"
	}

	reconcileutils := NewReconcileUtils(chrootPathPrefix, osClient)
	osUtils := afero.Afero{Fs: afero.NewOsFs()}

	return NewDetailed(chrootPathPrefix, tridentexec.NewCommand(), DefaultSelfHealingExclusion, osClient,
		deviceClient, fileSystemClient, mountClient, reconcileutils, osUtils)
}

func NewDetailed(chrootPathPrefix string, command tridentexec.Command, selfHealingExclusion []string, osClient OS,
	deviceClient Devices, fileSystemClient FileSystem, mountClient Mount, iscsiUtils IscsiReconcileUtils,
	os afero.Afero,
) *Client {
	return &Client{
		chrootPathPrefix:     chrootPathPrefix,
		command:              command,
		osClient:             osClient,
		deviceClient:         deviceClient,
		fileSystemClient:     fileSystemClient,
		mountClient:          mountClient,
		iscsiUtils:           iscsiUtils,
		selfHealingExclusion: selfHealingExclusion,
		os:                   os,
	}
}

// AttachVolumeRetry attaches a volume with retry by invoking AttachVolume with backoff.
func (client *Client) AttachVolumeRetry(
	ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
	timeout time.Duration,
) (int64, error) {
	Logc(ctx).Debug(">>>> iscsi.AttachVolumeRetry")
	defer Logc(ctx).Debug("<<<< iscsi.AttachVolumeRetry")
	var err error
	var mpathSize int64

	if err = client.PreChecks(ctx); err != nil {
		return mpathSize, err
	}

	checkAttachISCSIVolume := func() error {
		mpathSize, err = client.AttachVolume(ctx, name, mountpoint, publishInfo, secrets)
		return err
	}

	attachNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
		}).Debug("Attach iSCSI volume is not complete, waiting.")
	}

	attachBackoff := backoff.NewExponentialBackOff()
	attachBackoff.InitialInterval = 1 * time.Second
	attachBackoff.Multiplier = 1.414 // approx sqrt(2)
	attachBackoff.RandomizationFactor = 0.1
	attachBackoff.MaxElapsedTime = timeout

	err = backoff.RetryNotify(checkAttachISCSIVolume, attachBackoff, attachNotify)
	return mpathSize, err
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
	Logc(ctx).Debug(">>>> iscsi.AttachVolume")
	defer Logc(ctx).Debug("<<<< iscsi.AttachVolume")

	var err error
	var mpathSize int64
	lunID := int(publishInfo.IscsiLunNumber)

	var portals []string

	// IscsiTargetPortal is one of the ports on the target and IscsiPortals
	// are rest of the target ports for establishing iSCSI session.
	// If the target has multiple portals, then there will be multiple iSCSI sessions.
	portals = append(portals, ensureHostportFormatted(publishInfo.IscsiTargetPortal))

	for _, p := range publishInfo.IscsiPortals {
		portals = append(portals, ensureHostportFormatted(p))
	}

	if publishInfo.IscsiInterface == "" {
		publishInfo.IscsiInterface = "default"
	}

	Logc(ctx).WithFields(LogFields{
		"volume":         name,
		"mountPoint":     mountPoint,
		"lunID":          lunID,
		"portals":        portals,
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"iscsiInterface": publishInfo.IscsiInterface,
		"fstype":         publishInfo.FilesystemType,
	}).Debug("Attaching iSCSI volume.")

	if err = client.PreChecks(ctx); err != nil {
		return mpathSize, err
	}
	// Ensure we are logged into correct portals
	pendingPortalsToLogin, loggedIn, err := client.portalsToLogin(ctx, publishInfo.IscsiTargetIQN, portals)
	if err != nil {
		return mpathSize, err
	}

	newLogin, err := client.EnsureSessions(ctx, publishInfo, pendingPortalsToLogin)

	if !loggedIn && !newLogin {
		return mpathSize, err
	}

	hostSessionMap := client.iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, publishInfo.IscsiTargetIQN)
	if len(hostSessionMap) == 0 {
		return mpathSize, fmt.Errorf("no iSCSI hosts found for target %s", publishInfo.IscsiTargetIQN)
	}

	// First attempt to fix invalid serials by rescanning them
	err = client.handleInvalidSerials(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunSerial,
		client.rescanOneLun)
	if err != nil {
		return mpathSize, err
	}

	// Then attempt to fix invalid serials by purging them (to be scanned
	// again later)
	err = client.handleInvalidSerials(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunSerial,
		client.purgeOneLun)
	if err != nil {
		return mpathSize, err
	}

	// Scan the target and wait for the device(s) to appear
	err = client.waitForDeviceScan(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN)
	if err != nil {
		Logc(ctx).Errorf("Could not find iSCSI device: %+v", err)
		return mpathSize, err
	}

	// At this point if the serials are still invalid, give up so the
	// caller can retry (invoking the remediation steps above in the
	// process, if they haven't already been run).
	failHandler := func(ctx context.Context, path string) error {
		Logc(ctx).Error("Detected LUN serial number mismatch, attaching volume would risk data corruption, giving up")
		return fmt.Errorf("LUN serial number mismatch, kernel has stale cached data")
	}
	err = client.handleInvalidSerials(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunSerial, failHandler)
	if err != nil {
		return mpathSize, err
	}

	// Wait for multipath device i.e. /dev/dm-* for the given LUN
	err = client.waitForMultipathDeviceForLUN(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN)
	if err != nil {
		return mpathSize, err
	}

	// Lookup all the SCSI device information
	deviceInfo, err := client.getDeviceInfoForLUN(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN, false)
	if err != nil {
		return mpathSize, fmt.Errorf("error getting iSCSI device information: %v", err)
	} else if deviceInfo == nil {
		return mpathSize, fmt.Errorf("could not get iSCSI device information for LUN %d", lunID)
	}

	Logc(ctx).WithFields(LogFields{
		"scsiLun":         deviceInfo.LUN,
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
		"iqn":             deviceInfo.IQN,
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
	if err = client.verifyMultipathDeviceSerial(ctx, deviceToUse, publishInfo.IscsiLunSerial); err != nil {
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
			"iqn":             deviceInfo.IQN,
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
			"iqn":             deviceInfo.IQN,
			"mpathSize":       mpathSize,
		}).Error("Multipath device size does not match device size.")
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
		luksDevice, _ := client.deviceClient.NewLUKSDevice(devicePath, name)
		luksFormatted, err = client.deviceClient.EnsureLUKSDeviceMappedOnHost(ctx, luksDevice, name, secrets)
		if err != nil {
			return mpathSize, err
		}
		devicePath = luksDevice.MappedDevicePath()
	}

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
		if err = client.fileSystemClient.FormatVolume(ctx, devicePath, publishInfo.FilesystemType, publishInfo.FormatOptions); err != nil {
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

// AddSession adds a portal and LUN data to the session map. Extracts the
// required iSCSI Target IQN, CHAP Credentials if any from the provided VolumePublishInfo and
// populates the map against the portal.
// NOTE: sessionNumber should only be passed if there is only one portal/targetportal entry in publishInfo.
func (client *Client) AddSession(
	ctx context.Context, sessions *models.ISCSISessions, publishInfo *models.VolumePublishInfo,
	volID, sessionNumber string, reasonInvalid models.PortalInvalid,
) {
	if sessions == nil {
		// Initialize and use it
		sessions = &models.ISCSISessions{Info: make(map[string]*models.ISCSISessionData)}
	}

	iSCSITargetIQN := publishInfo.IscsiTargetIQN

	// Check if TargetIQN is empty
	if reasonInvalid == models.NotInvalid {
		if iSCSITargetIQN == "" {
			Logc(ctx).Errorf("Portal '%v' is missing target IQN; it may not be considered for iSCSI self-healing.",
				publishInfo.IscsiTargetPortal)
			reasonInvalid = models.MissingTargetIQN
		}
	}

	// Check if TargetIQN contains keywords that is part of the exclusion list
	for _, keyword := range client.selfHealingExclusion {
		if strings.Contains(iSCSITargetIQN, keyword) {
			Logc(ctx).Errorf("Failed to add portal %v to self-healing map; target IQN '%v' contains `%v` keyword",
				publishInfo.IscsiTargetPortal, iSCSITargetIQN, keyword)
			return
		}
	}

	newLUNData := models.LUNData{
		LUN:   publishInfo.IscsiLunNumber,
		VolID: volID,
	}

	// Capture the source of the session information
	var source string
	if ctxSource := ctx.Value(SessionInfoSource); ctxSource != nil {
		source = fmt.Sprintf("%v", ctxSource)
	}

	// Extract required portal info
	credentials := models.IscsiChapInfo{
		UseCHAP:              publishInfo.UseCHAP,
		IscsiUsername:        publishInfo.IscsiUsername,
		IscsiInitiatorSecret: publishInfo.IscsiInitiatorSecret,
		IscsiTargetUsername:  publishInfo.IscsiTargetUsername,
		IscsiTargetSecret:    publishInfo.IscsiTargetSecret,
	}
	portalInfo := models.PortalInfo{
		ISCSITargetIQN:         publishInfo.IscsiTargetIQN,
		Credentials:            credentials,
		LastAccessTime:         time.Time{},
		FirstIdentifiedStaleAt: time.Time{},
		SessionNumber:          sessionNumber,
		ReasonInvalid:          reasonInvalid,
		Source:                 source,
	}

	allPortals := append(publishInfo.IscsiPortals, publishInfo.IscsiTargetPortal)
	for _, portal := range allPortals {

		if !sessions.CheckPortalExists(portal) {
			if err := sessions.AddPortal(portal, portalInfo); err != nil {
				Logc(ctx).Errorf("Failed to add portal %v to self-healing map; err: %v", portal, err)
				continue
			}
		} else {
			if portalUpdates, err := sessions.UpdateAndRecordPortalInfoChanges(portal, portalInfo); err != nil {
				Logc(ctx).Errorf("Failed to update portal %v in self-healing map; err: %v", portal, err)
				continue
			} else if portalUpdates != "" {
				Logc(ctx).Debugf("Changes to portal %v: %v", portal, portalUpdates)
			}
		}

		if err := sessions.AddLUNToPortal(portal, newLUNData); err != nil {
			Logc(ctx).Errorf("Failed to add LUN %v to portal %v in self-healing map; err:  %v", newLUNData, portal, err)
		}
	}
}

func (client *Client) RescanDevices(ctx context.Context, targetIQN string, lunID int32, minSize int64) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	fields := LogFields{"targetIQN": targetIQN, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.RescanDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.RescanDevices")

	hostSessionMap := client.iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, targetIQN)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("error getting iSCSI device information: no host session found")
	}
	deviceInfo, err := client.getDeviceInfoForLUN(ctx, hostSessionMap, int(lunID), targetIQN, false)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %s", err)
	}

	allLargeEnough := true
	for _, diskDevice := range deviceInfo.Devices {
		size, err := client.deviceClient.GetISCSIDiskSize(ctx, DevPrefix+diskDevice)
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
			size, err := client.deviceClient.GetISCSIDiskSize(ctx, DevPrefix+diskDevice)
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
		size, err := client.deviceClient.GetISCSIDiskSize(ctx, DevPrefix+multipathDevice)
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
			size, err = client.deviceClient.GetISCSIDiskSize(ctx, DevPrefix+multipathDevice)
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

// rescanDisk causes the kernel to rescan a single iSCSI disk/block device.
// This is how size changes are found when expanding a volume.
func (client *Client) rescanDisk(ctx context.Context, deviceName string) error {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.rescanDisk")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.rescanDisk")

	client.listAllDevices(ctx)
	filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/block/%s/device/rescan", deviceName)
	Logc(ctx).WithField("filename", filename).Debug("Opening file for writing.")

	f, err := client.os.OpenFile(filename, os.O_WRONLY, 0)
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

	client.listAllDevices(ctx)
	return nil
}

func (client *Client) reloadMultipathDevice(ctx context.Context, multipathDevice string) error {
	fields := LogFields{"multipathDevice": multipathDevice}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.reloadMultipathDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.reloadMultipathDevice")

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

// IsAlreadyAttached checks if there is already an established iSCSI session to the specified LUN.
func (client *Client) IsAlreadyAttached(ctx context.Context, lunID int, targetIqn string) bool {
	hostSessionMap := client.iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, targetIqn)
	if len(hostSessionMap) == 0 {
		return false
	}

	paths := client.iscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := client.iscsiUtils.GetDevicesForLUN(paths)
	if nil != err {
		return false
	}

	// return true even if a single device exists
	return 0 < len(devices)
}

// getDeviceInfoForLUN finds iSCSI devices using /dev/disk/by-path values.  This method should be
// called after calling waitForDeviceScan so that the device paths are known to exist.
func (client *Client) getDeviceInfoForLUN(
	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, needFSType bool,
) (*models.ScsiDeviceInfo, error) {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
		"needFSType":    needFSType,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.getDeviceInfoForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.getDeviceInfoForLUN")

	paths := client.iscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := client.iscsiUtils.GetDevicesForLUN(paths)
	if err != nil {
		return nil, err
	} else if 0 == len(devices) {
		return nil, fmt.Errorf("scan not completed for LUN %d on target %s", lunID, iSCSINodeName)
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
		if err = client.deviceClient.EnsureDeviceReadable(ctx, devicePath); err != nil {
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

	info := &models.ScsiDeviceInfo{
		LUN:             strconv.Itoa(lunID),
		MultipathDevice: multipathDevice,
		Devices:         devices,
		DevicePaths:     paths,
		Filesystem:      fsType,
		IQN:             iSCSINodeName,
	}

	return info, nil
}

// purgeOneLun issues a delete for one LUN, based on the sysfs path
func (client *Client) purgeOneLun(ctx context.Context, path string) error {
	Logc(ctx).WithField("path", path).Debug("Purging one LUN")
	filename := path + "/delete"

	f, err := client.os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200)
	if err != nil {
		Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
		return err
	}
	defer f.Close()

	if err = duringPurgeOneLunBeforeFileWrite.Inject(); err != nil {
		return err
	}

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
func (client *Client) rescanOneLun(ctx context.Context, path string) error {
	Logc(ctx).WithField("path", path).Debug("Rescaning one LUN")
	filename := path + "/rescan"

	f, err := client.os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200)
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
func (client *Client) waitForMultipathDeviceForLUN(ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string) error {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.waitForMultipathDeviceForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.waitForMultipathDeviceForLUN")

	paths := client.iscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := client.iscsiUtils.GetDevicesForLUN(paths)
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

	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.waitForMultipathDeviceForDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.waitForMultipathDeviceForDevices")

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
	Logc(ctx).WithField("device", device).Debug(">>>> iscsi.findMultipathDeviceForDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< iscsi.findMultipathDeviceForDevice")

	holdersDir := client.chrootPathPrefix + "/sys/block/" + device + "/holders"
	if dirs, err := client.os.ReadDir(holdersDir); err == nil {
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
func (client *Client) waitForDeviceScan(ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string) error {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.waitForDeviceScan")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.waitForDeviceScan")

	hosts := make([]int, 0)
	for hostNumber := range hostSessionMap {
		hosts = append(hosts, hostNumber)
	}

	if err := client.scanTargetLUN(ctx, lunID, hosts); err != nil {
		Logc(ctx).WithField("scanError", err).Error("Could not scan for new LUN.")
	}

	paths := client.iscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
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

// scanTargetLUN scans a single LUN or all the LUNs on an iSCSI target to discover it.
// If all the LUNs are to be scanned please pass -1 for lunID.
func (client *Client) scanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
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

	client.listAllDevices(ctx)
	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = client.os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200); err != nil {
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

		client.listAllDevices(ctx)
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
	ctx context.Context, hostSessionMap map[int]int, lunID int, targetIqn,
	expectedSerial string, handler func(ctx context.Context, path string) error,
) error {
	if "" == expectedSerial {
		// Empty string means don't care
		return nil
	}

	paths := client.iscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	for _, path := range paths {
		serial, err := client.getLunSerial(ctx, path)
		if err != nil {
			if os.IsNotExist(err) {
				// LUN either isn't scanned yet, or this kernel
				// doesn't support VPD page 80 in sysfs. Assume
				// correctness and move on
				Logc(ctx).WithFields(LogFields{
					"lun":    lunID,
					"target": targetIqn,
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
				"target":   targetIqn,
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
				"target": targetIqn,
				"path":   path,
			}).Debug("LUN serial check passed")
		}
	}

	return nil
}

// getLunSerial get Linux's idea of what the LUN serial number is
func (client *Client) getLunSerial(ctx context.Context, path string) (string, error) {
	Logc(ctx).WithField("path", path).Debug("Get LUN Serial")
	// We're going to read the SCSI VPD page 80 serial number
	// information. Linux helpfully provides this through sysfs
	// so we don't need to open the device and send the ioctl
	// ourselves.
	filename := path + "/vpd_pg80"
	b, err := client.os.ReadFile(filename)
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

// portalsToLogin checks to see if session to for all the specified portals exist for the specified
// target. If a session does not exist for a give portal it is added to list of portals that Trident
// needs to login to.
func (client *Client) portalsToLogin(ctx context.Context, targetIQN string, portals []string) ([]string, bool, error) {
	logFields := LogFields{
		"targetIQN": targetIQN,
		"portals":   portals,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.portalsToLogin")
	defer Logc(ctx).Debug("<<<< iscsi.portalsToLogin")

	portalsInStaleState := make([]string, 0)
	portalsNotLoggedIn := make([]string, len(portals))
	copy(portalsNotLoggedIn, portals)

	sessionInfo, err := client.getSessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return portalsNotLoggedIn, false, err
	}

	for _, e := range sessionInfo {

		if e.TargetName != targetIQN {
			continue
		}

		// Portals (portalsNotLoggedIn) may/may not contain anything after ":", so instead of matching complete
		// portal value (with e.Portal), check if e.Portal's IP address matches portal's IP address
		matchFunc := func(main, val string) bool {
			mainIpAddress := models.ParseHostportIP(main)
			valIpAddress := models.ParseHostportIP(val)

			return mainIpAddress == valIpAddress
		}

		lenBeforeCheck := len(portalsNotLoggedIn)
		portalsNotLoggedIn = RemoveStringFromSliceConditionally(portalsNotLoggedIn, e.Portal, matchFunc)
		lenAfterCheck := len(portalsNotLoggedIn)

		// If the portal is logged in ensure it is not stale
		if lenBeforeCheck != lenAfterCheck {
			if client.isSessionStale(ctx, e.SID) {
				portalsInStaleState = append(portalsInStaleState, e.Portal)
			}
		}
	}

	if len(portals) == len(portalsInStaleState) {
		return nil, false, fmt.Errorf("no new session to establish and existing session(s) might be in unhealthy state")
	}

	loggedIn := len(portals) != (len(portalsNotLoggedIn) + len(portalsInStaleState))
	return portalsNotLoggedIn, loggedIn, nil
}

// getSessionConnectionsState returns the state of iscsi session connections stored in:
// '/sys/class/iscsi_session/session<ID>/device/connection<ID>:0/iscsi_connection/connection<ID>:0.
func (client *Client) getSessionConnectionsState(ctx context.Context, sessionID string) []string {
	Logc(ctx).WithField("sessionID", sessionID).Debug(">>>> iscsi.getSessionConnectionsState")
	defer Logc(ctx).Debug("<<<< iscsi.getSessionConnectionsState")

	// Find the session device dirs under: '/sys/class/iscsi_session/session<ID>/device/'.
	sessionName := fmt.Sprintf("session%s", sessionID)
	sessionDevicePath := filepath.Join(client.chrootPathPrefix, "sys", "class", "iscsi_session", sessionName, "device")
	sessionDeviceEntries, err := client.os.ReadDir(sessionDevicePath)
	if err != nil {
		Logc(ctx).WithField("path", sessionDevicePath).WithError(err).Error("Could not read session dirs.")
		return nil
	}

	const notFound = "<NOT FOUND>"
	var errs error

	// Dynamically discover the 'state' for all underlying connections and return them.
	connectionStates := make([]string, 0)
	for _, entry := range sessionDeviceEntries {
		// Only consider: `/sys/class/iscsi_session/session<ID>/device/connection<ID>:0`
		connection := entry.Name()
		if !strings.HasPrefix(connection, "connection") {
			continue
		}

		// At this point, we know we're looking at something like:
		// '/sys/class/iscsi_session/session<ID>/device/connection<ID>:0' but we need:
		// '/sys/class/iscsi_session/session<ID>/device/connection<ID>:0/iscsi_connection/connection<ID>:0'
		state := notFound
		statePath := filepath.Join(sessionDevicePath, connection, "iscsi_connection", connection, "state")
		rawState, err := client.os.ReadFile(statePath)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to read session state at: '%s'; %w", statePath, err))
		} else if len(rawState) != 0 {
			state = strings.TrimSpace(string(rawState))
		}

		// If the connection state is "up" or not found, further inspection won't be helpful. Ignore this and move on.
		if state == sessionConnectionStateUp || state == notFound {
			continue
		}

		// Get the persistent address. This is the IP associated with a session.
		address := notFound
		addrPath := filepath.Join(sessionDevicePath, connection, "iscsi_connection", connection, "persistent_address")
		rawAddress, err := client.os.ReadFile(addrPath)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to read connection IP at: '%s'; %w", addrPath, err))
		} else if len(rawAddress) != 0 {
			address = strings.TrimSpace(string(rawAddress))
		}

		// Get the persistent port. This is the port associated with a session.
		port := notFound
		portPath := filepath.Join(sessionDevicePath, connection, "iscsi_connection", connection, "persistent_port")
		rawPort, err := client.os.ReadFile(portPath)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to read connection port at: '%s'; %w", portPath, err))
		} else if len(rawPort) != 0 {
			port = strings.TrimSpace(string(rawPort))
		}

		portal := fmt.Sprintf("%s:%s", address, port)

		// This will allow Trident to communicate which portals have bad connections.
		connectionState := fmt.Sprintf("\"portal:'%s'; connection:'%s'; state:'%s'\"", portal, connection, state)
		connectionStates = append(connectionStates, connectionState)
	}

	if errs != nil {
		Logc(ctx).WithError(errs).Error("Could not discover state of iSCSI connections.")
	}

	return connectionStates
}

func (client *Client) getSessionState(ctx context.Context, sessionID string) string {
	Logc(ctx).WithField("sessionID", sessionID).Debug(">>>> iscsi.getSessionState")
	defer Logc(ctx).Debug("<<<< iscsi.getSessionState")

	// Find the session state from the session at /sys/class/iscsi_session/sessionXXX/state
	filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/class/iscsi_session/session%s/state", sessionID)
	sessionStateBytes, err := client.os.ReadFile(filename)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"path":  filename,
			"error": err,
		}).Error("Could not read session state file.")
		return ""
	}

	sessionState := strings.TrimSpace(string(sessionStateBytes))
	Logc(ctx).WithFields(LogFields{
		"sessionID":    sessionID,
		"sessionState": sessionState,
		"sysfsFile":    filename,
	}).Debug("Found iSCSI session state.")

	return sessionState
}

// IsSessionStale - reads /sys/class/iscsi_session/session<sid>/state and returns true if it is not "LOGGED_IN".
// Looks that the state of an already established session to identify if it is
// logged in or not, if it is not logged in then it could be a stale session.
// For now, we are relying on the sysfs files
func (client *Client) isSessionStale(ctx context.Context, sessionID string) bool {
	Logc(ctx).WithField("sessionID", sessionID).Debug(">>>> iscsi.IsSessionStale")
	defer Logc(ctx).Debug("<<<< iscsi.IsSessionStale")

	// Find the session state from the session at /sys/class/iscsi_session/sessionXXX/state
	filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/class/iscsi_session/session%s/state", sessionID)
	sessionStateBytes, err := client.os.ReadFile(filename)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"path":  filename,
			"error": err,
		}).Error("Could not read session state file")
		return false
	}

	sessionState := strings.TrimSpace(string(sessionStateBytes))

	Logc(ctx).WithFields(LogFields{
		"sessionID":    sessionID,
		"sessionState": sessionState,
		"sysfsFile":    filename,
	}).Debug("Found iSCSI session state.")

	return sessionState != sessionStateLoggedIn
}

// SessionInfo contains information about iSCSI sessions.
type SessionInfo struct {
	SID        string
	Portal     string
	PortalIP   string
	TargetName string
}

// getSessionInfo parses output from 'iscsiadm -m session' and returns the parsed output.
func (client *Client) getSessionInfo(ctx context.Context) ([]SessionInfo, error) {
	Logc(ctx).Debug(">>>> iscsi.getSessionInfo")
	defer Logc(ctx).Debug("<<<< iscsi.getSessionInfo")

	out, err := client.execIscsiadmCommand(ctx, "-m", "session")
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == errNoObjsFound {
			Logc(ctx).Debug("No iSCSI session found.")
			return []SessionInfo{}, nil
		} else {
			Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
			return nil, err
		}
	}

	/*
	   # iscsiadm -m session
	   tcp: [3] 10.0.207.7:3260,1028 iqn.1992-08.com.netapp:sn.afbb1784f77411e582f8080027e22798:vs.3 (non-flash)
	   tcp: [4] 10.0.207.9:3260,1029 iqn.1992-08.com.netapp:sn.afbb1784f77411e582f8080027e22798:vs.3 (non-flash)
	   a[0]==tcp:
	   a[1]==[4]
	   a[2]==10.0.207.9:3260,1029
	   a[3]==iqn.1992-08.com.netapp:sn.afbb1784f77411e582f8080027e22798:vs.3
	   a[4]==(non-flash)
	*/

	var sessionInfo []SessionInfo

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {

		a := strings.Fields(l)
		if len(a) > 3 {
			sid := a[1]
			sid = sid[1 : len(sid)-1]

			portalIP := ""
			if IPv6Check(a[2]) {
				// This is an IPv6 address
				portalIP = strings.Split(a[2], "]")[0]
				portalIP += "]"
			} else {
				portalIP = strings.Split(a[2], ":")[0]
			}

			sessionInfo = append(sessionInfo, SessionInfo{
				SID:        sid,
				Portal:     a[2],
				PortalIP:   portalIP,
				TargetName: a[3],
			})

			Logc(ctx).WithFields(LogFields{
				"SID":        sid,
				"Portal":     a[2],
				"PortalIP":   portalIP,
				"TargetName": a[3],
			}).Debug("Adding iSCSI session info.")
		}
	}

	return sessionInfo, nil
}

// ensureHostportFormatted ensures IPv6 hostport is in correct format
func ensureHostportFormatted(hostport string) string {
	// If this is an IPv6 address, ensure IP address is enclosed in square
	// brackets, as in "[::1]:80".
	if IPv6Check(hostport) && hostport[0] != '[' {
		// assumption here is that without the square brackets its only IP address without port information
		return "[" + hostport + "]"
	}

	return hostport
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

	multipathDeviceUUID, err := client.iscsiUtils.GetMultipathDeviceUUID(multipathDevice)
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
	deviceSize, err := client.deviceClient.GetISCSIDiskSize(ctx, "/dev/"+device)
	if err != nil {
		return 0, false, err
	}

	mpathSize, err := client.deviceClient.GetISCSIDiskSize(ctx, "/dev/"+multipathDevice)
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

// EnsureSessions this is to make sure that Trident establishes iSCSI sessions with the given list of portals
func (client *Client) EnsureSessions(ctx context.Context, publishInfo *models.VolumePublishInfo,
	portals []string,
) (bool, error) {
	logFields := LogFields{
		"targetIQN":  publishInfo.IscsiTargetIQN,
		"portalsIps": portals,
		"iface":      publishInfo.IscsiInterface,
		"useCHAP":    publishInfo.UseCHAP,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.EnsureSessions")
	defer Logc(ctx).Debug("<<<< iscsi.EnsureSessions")

	loggedInPortals := make([]string, 0)

	loginFailedDueToChap := false

	for _, portal := range portals {
		client.listAllDevices(ctx)

		formattedPortal := formatPortal(portal)
		if err := client.ensureTarget(ctx, formattedPortal, publishInfo.IscsiTargetIQN, publishInfo.IscsiUsername,
			publishInfo.IscsiInitiatorSecret, publishInfo.IscsiTargetUsername, publishInfo.IscsiTargetSecret,
			publishInfo.IscsiInterface); nil != err {
			Logc(ctx).WithFields(LogFields{
				"tp":        formattedPortal,
				"targetIqn": publishInfo.IscsiTargetIQN,
				"iface":     publishInfo.IscsiInterface,
				"err":       err,
			}).Errorf("unable to ensure iSCSI target exists: %v", err)

			if !loginFailedDueToChap {
				if errors.IsAuthError(err) {
					Logc(ctx).Debug("Unable to ensure iSCSI target exists - authorization failed using CHAP")
					loginFailedDueToChap = true
				}
			}

			continue
		}

		// Set scanning to manual
		// Swallow this error, someone is running an old version of Debian/Ubuntu
		const sessionScanParam = "node.session.scan"
		_ = client.configureTarget(ctx, publishInfo.IscsiTargetIQN, portal, sessionScanParam, "manual")

		// replacement_timeout controls how long iSCSI layer should wait for a timed-out path/session to reestablish
		// itself before failing any commands on it.
		const timeoutParam = "node.session.timeo.replacement_timeout"
		if err := client.configureTarget(ctx, publishInfo.IscsiTargetIQN, portal, timeoutParam, "5"); err != nil {
			Logc(ctx).WithFields(LogFields{
				"iqn":    publishInfo.IscsiTargetIQN,
				"portal": portal,
				"name":   timeoutParam,
				"value":  "5",
				"err":    err,
			}).Errorf("set replacement timeout failed: %v", err)
			continue
		}

		// Log in to target
		if err := client.LoginTarget(ctx, publishInfo, portal); err != nil {
			Logc(ctx).WithFields(LogFields{
				"err":      err,
				"portalIP": portal,
			}).Error("Login to iSCSI target failed.")
			if !loginFailedDueToChap {
				if errors.IsAuthError(err) {
					Logc(ctx).Debug("iSCSI login failed - authorization failed using CHAP")
					loginFailedDueToChap = true
				}
			}
			continue
		}

		loggedInPortals = append(loggedInPortals, portal)
	}

	var successfulLogin bool

	for _, portalInfo := range loggedInPortals {
		// Recheck to ensure a session is now open
		sessionExists, err := client.sessionExists(ctx, models.ParseHostportIP(portalInfo))
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"err":      err,
				"portalIP": portalInfo,
			}).Error("Could not recheck for iSCSI session.")
			continue
		}

		if !sessionExists {
			Logc(ctx).Errorf("Expected iSCSI session %v NOT found, please login to the iSCSI portal", portalInfo)
			continue
		}

		successfulLogin = true

		Logc(ctx).WithField("portal", portalInfo).Debug("Session established with iSCSI portal.")
	}

	if successfulLogin {
		return successfulLogin, nil
	}

	if publishInfo.UseCHAP && len(loggedInPortals) == 0 {
		// login failed for all portals using CHAP, verify if any login failed due to authorization failure,
		// return AuthError; NodeStageISCSIVolume() would handle appropriately
		if loginFailedDueToChap {
			return successfulLogin, errors.AuthError("iSCSI login failed: CHAP authorization failure")
		}
	}

	return successfulLogin, errors.New("iSCSI login failed")
}

// sessionExists checks to see if a session exists to the specified portal.
func (client *Client) sessionExists(ctx context.Context, portal string) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi.sessionExists")
	defer Logc(ctx).Debug("<<<< iscsi.sessionExists")

	sessionInfo, err := client.getSessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return false, err
	}

	for _, e := range sessionInfo {
		if strings.Contains(e.PortalIP, portal) {
			return true, nil
		}
	}

	return false, nil
}

// LoginTarget logs in to an iSCSI target.
func (client *Client) LoginTarget(ctx context.Context, publishInfo *models.VolumePublishInfo,
	portal string,
) error {
	Logc(ctx).WithFields(LogFields{
		"IQN":     publishInfo.IscsiTargetIQN,
		"Portal":  portal,
		"iface":   publishInfo.IscsiInterface,
		"useCHAP": publishInfo.UseCHAP,
	}).Debug(">>>> iscsi.LoginTarget")
	defer Logc(ctx).Debug("<<<< iscsi.LoginTarget")

	args := []string{"-m", "node", "-T", publishInfo.IscsiTargetIQN, "-p", formatPortal(portal)}
	client.listAllDevices(ctx)
	if publishInfo.UseCHAP {
		secretsToRedact := map[string]string{
			"--value=" + publishInfo.IscsiUsername:        "--value=" + REDACTED,
			"--value=" + publishInfo.IscsiInitiatorSecret: "--value=" + REDACTED,
			"--value=" + publishInfo.IscsiTargetUsername:  "--value=" + REDACTED,
			"--value=" + publishInfo.IscsiTargetSecret:    "--value=" + REDACTED,
		}
		authMethodArgs := append(args,
			[]string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
		if _, err := client.execIscsiadmCommand(ctx, authMethodArgs...); err != nil {
			Logc(ctx).Error("Error running iscsiadm set authmethod.")
			return err
		}

		authUserArgs := append(args,
			[]string{"--op=update", "--name", "node.session.auth.username", "--value=" + publishInfo.IscsiUsername}...)
		if _, err := client.execIscsiadmCommandRedacted(ctx, authUserArgs, secretsToRedact); err != nil {
			Logc(ctx).Error("Error running iscsiadm set authuser.")
			return err
		}

		authPasswordArgs := append(args,
			[]string{
				"--op=update",
				"--name",
				"node.session.auth.password",
				"--value=" + publishInfo.IscsiInitiatorSecret,
			}...)
		if _, err := client.execIscsiadmCommandRedacted(ctx, authPasswordArgs, secretsToRedact); err != nil {
			Logc(ctx).Error("Error running iscsiadm set authpassword.")
			return err
		}

		if publishInfo.IscsiTargetUsername != "" && publishInfo.IscsiTargetSecret != "" {
			targetAuthUserArgs := append(args,
				[]string{
					"--op=update",
					"--name",
					"node.session.auth.username_in",
					"--value=" + publishInfo.IscsiTargetUsername,
				}...)
			if _, err := client.execIscsiadmCommandRedacted(ctx, targetAuthUserArgs, secretsToRedact); err != nil {
				Logc(ctx).Error("Error running iscsiadm set authuser_in.")
				return err
			}

			targetAuthPasswordArgs := append(args,
				[]string{
					"--op=update",
					"--name",
					"node.session.auth.password_in",
					"--value=" + publishInfo.IscsiTargetSecret,
				}...)
			if _, err := client.execIscsiadmCommandRedacted(ctx, targetAuthPasswordArgs, secretsToRedact); err != nil {
				Logc(ctx).Error("Error running iscsiadm set authpassword_in.")
				return err
			}
		}
	}

	loginTimeOutArgs := append(args,
		[]string{
			"--op=update",
			"--name",
			"node.conn[0].timeo.login_timeout",
			fmt.Sprintf("--value=%d", iscsiadmLoginTimeoutValue),
		}...)
	if _, err := client.execIscsiadmCommand(ctx, loginTimeOutArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm set login timeout.")
		return err
	}

	loginRetryMaxArgs := append(args,
		[]string{
			"--op=update",
			"--name",
			"node.session.initial_login_retry_max",
			"--value=" + iscsiadmLoginRetryMax,
		}...)
	if _, err := client.execIscsiadmCommand(ctx, loginRetryMaxArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm set login retry max.")
		return err
	}

	loginArgs := append(args, []string{"--login"}...)
	if _, err := client.execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, loginArgs...); err != nil {
		Logc(ctx).WithField("error", err).Error("Error logging in to iSCSI target.")
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == errLoginAuthFailed {
			return errors.AuthError("iSCSI login failed: CHAP authorization failure")
		}

		return err
	}
	client.listAllDevices(ctx)
	return nil
}

// configureTarget updates an iSCSI target configuration values.
func (client *Client) configureTarget(ctx context.Context, iqn, portal, name, value string) error {
	Logc(ctx).WithFields(LogFields{
		"IQN":    iqn,
		"Portal": portal,
		"Name":   name,
		"Value":  value,
	}).Debug(">>>> iscsi.configureTarget")
	defer Logc(ctx).Debug("<<<< iscsi.configureTarget")

	if err := duringConfigureISCSITargetBeforeISCSIAdmUpdate.Inject(); err != nil {
		return err
	}

	args := []string{"-m", "node", "-T", iqn, "-p", formatPortal(portal), "-o", "update", "-n", name, "-v", value}
	if _, err := client.execIscsiadmCommand(ctx, args...); err != nil {
		Logc(ctx).WithField("error", err).Warn("Error configuring iSCSI target.")
		return err
	}
	return nil
}

// formatPortal returns the iSCSI portal string, appending a port number if one isn't
// already present, and also appending a target portal group tag if one is not present
func formatPortal(portal string) string {
	if portalPortPattern.MatchString(portal) {
		return portal
	} else {
		return portal + ":3260"
	}
}

// In the case of iscsi trace debug, log info about session and what devices are present
func (client *Client) listAllDevices(ctx context.Context) {
	Logc(ctx).Trace(">>>> iscsi.listAllDevices")
	defer Logc(ctx).Trace("<<<< iscsi.listAllDevices")
	// Log information about all the devices
	dmLog := make([]string, 0)
	sdLog := make([]string, 0)
	sysLog := make([]string, 0)
	entries, _ := client.os.ReadDir(DevPrefix)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "dm-") {
			dmLog = append(dmLog, entry.Name())
		}
		if strings.HasPrefix(entry.Name(), "sd") {
			sdLog = append(sdLog, entry.Name())
		}
	}

	entries, _ = client.os.ReadDir("/sys/block/")
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
	}).Trace("Listing all iSCSI Devices.")
}

// PreChecks to check if all the required tools are present and configured correctly for the  volume
// attachment to go through
func (client *Client) PreChecks(ctx context.Context) error {
	if !client.supported(ctx) {
		return errors.New("unable to attach: open-iscsi tools not found on host")
	}

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
	Logc(ctx).Debug(">>>> iscsi.multipathdIsRunning")
	defer Logc(ctx).Debug("<<<< iscsi.multipathdIsRunning")

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
	// This matches pattern in a multiline string of type "    find_multipaths: yes"
	tagsWithIndentationRegex := regexp.MustCompile(`(?m)^[\t ]*find_multipaths[\t ]*["|']?(?P<tagName>[\w-_]+)["|']?[\t ]*$`)
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

// supported returns true if iscsiadm is installed and in the PATH.
func (client *Client) supported(ctx context.Context) bool {
	Logc(ctx).Debug(">>>> iscsi.supported")
	defer Logc(ctx).Debug("<<<< iscsi.supported")

	// run the iscsiadm command to show version to check if iscsiadm is installed
	_, err := client.execIscsiadmCommand(ctx, "-V")
	if err != nil {
		Logc(ctx).Debug("iscsiadm tools not found on this host.")
		return false
	}
	return true
}

// ensureTarget creates the iSCSI target if we haven't done so already
// This function first checks if the target is already known, and if not,
// uses sendtargets to try to discover it. Because sendtargets will find
// all of the targets given just 1 portal, it will be very common to hit
// the case where the target is already known.
// Note: Adding iSCSI targets using sendtargets rather than static discover
// ensures that targets are added with the correct target group portal tags.
func (client *Client) ensureTarget(
	ctx context.Context, targetPortal, targetIqn, username, password, targetUsername, targetInitiatorSecret,
	iface string,
) error {
	Logc(ctx).WithFields(LogFields{
		"IQN":       targetIqn,
		"Portal":    targetPortal,
		"Interface": iface,
	}).Debug(">>>> iscsi.ensureTarget")
	defer Logc(ctx).Debug("<<<< iscsi.ensureTarget")

	targets, err := client.getTargets(ctx, targetPortal)
	if err != nil {
		// Already logged
		return err
	}
	for _, iqn := range targets {
		if targetIqn == iqn {
			Logc(ctx).WithField("Target", iqn).Info("Target exists already")
			return nil
		}
	}

	if "" != username && "" != password {
		// To do discovery on a CHAP-enabled target, we need to set the CHAP
		// secrets on the discoverydb object before making the sendtargets
		// call.

		// Ignore result
		_, _ = client.execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, "-m", "discoverydb", "-t", "st", "-p",
			targetPortal,
			"-I",
			iface, "-o", "new")

		err = client.updateDiscoveryDb(ctx, targetPortal, iface, "discovery.sendtargets.auth.authmethod", "CHAP")
		if err != nil {
			// Already logged
			return err
		}

		err = client.updateDiscoveryDb(ctx, targetPortal, iface, "discovery.sendtargets.auth.username", username)
		if err != nil {
			// Already logged
			return err
		}

		err = client.updateDiscoveryDb(ctx, targetPortal, iface, "discovery.sendtargets.auth.password", password)
		if err != nil {
			// Already logged
			return err
		}

		if targetUsername != "" && targetInitiatorSecret != "" {
			// Bidirectional CHAP case

			err = client.updateDiscoveryDb(ctx, targetPortal, iface, "discovery.sendtargets.auth.username_in", targetUsername)
			if err != nil {
				// Already logged
				return err
			}

			err = client.updateDiscoveryDb(ctx, targetPortal, iface, "discovery.sendtargets.auth.password_in",
				targetInitiatorSecret)
			if err != nil {
				// Already logged
				return err
			}
		}
	}

	// Discovery is here. This will populate the iscsiadm database with the
	// All the nodes known to the given portal.
	output, err := client.execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, "-m", "discoverydb",
		"-t", "st", "-p", targetPortal, "-I", iface, "-D")
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"portal": targetPortal,
			"error":  err,
			"output": string(output),
		}).Error("Failed to discover targets")

		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == errLoginAuthFailed {
			return errors.AuthError("failed to discover targets: CHAP authorization failure")
		}

		return fmt.Errorf("failed to discover targets: %v", err)
	}

	targets = filterTargets(string(output), targetPortal)
	for _, iqn := range targets {
		if targetIqn == iqn {
			Logc(ctx).WithField("Target", iqn).Info("Target discovered successfully")
			// Discovered successfully
			return nil
		}
	}

	Logc(ctx).WithFields(LogFields{
		"portal": targetPortal,
		"iqn":    targetIqn,
	}).Warning("Target not discovered")
	return fmt.Errorf("target not discovered")
}

// updateDiscoveryDb update the iscsi discoverydb with the passed values
func (client *Client) updateDiscoveryDb(ctx context.Context, tp, iface, key, value string) error {
	Logc(ctx).WithFields(LogFields{
		"Key":       key,
		"Value":     value,
		"Portal":    tp,
		"Interface": iface,
	}).Debug(">>>> iscsi.updateDiscoveryDb")
	defer Logc(ctx).Debug("<<<< iscsi.updateDiscoveryDb")

	output, err := client.execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, "-m", "discoverydb",
		"-t", "st", "-p", tp, "-I", iface, "-o", "update", "-n", key, "-v", value)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"portal": tp,
			"error":  err,
			"key":    key,
			"value":  value,
			"output": string(output),
		}).Error("Failed to update discovery DB.")
		return fmt.Errorf("failed to update discovery db: %v", err)
	}

	return nil
}

// getTargets gets a list of discovered iSCSI targets
func (client *Client) getTargets(ctx context.Context, tp string) ([]string, error) {
	Logc(ctx).WithFields(LogFields{
		"Portal": tp,
	}).Debug(">>>> iscsi.getTargets")
	defer Logc(ctx).Debug("<<<< iscsi.getTargets")

	output, err := client.command.Execute(ctx, "iscsiadm", "-m", "node")
	if nil != err {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if errNoObjsFound == status.ExitStatus() {
					Logc(ctx).Debug("No iSCSI nodes found.")
					// No records
					return nil, nil
				}
			}
		}
		Logc(ctx).WithFields(LogFields{
			"error":  err,
			"output": string(output),
		}).Error("Failed to list nodes")
		return nil, fmt.Errorf("failed to list nodes: %v", err)
	}
	return filterTargets(string(output), tp), nil
}

// filterTargets parses the output of iscsiadm -m node or -m discoverydb -t st -D
// and returns the target IQNs for a given portal
func filterTargets(output, tp string) []string {
	regex := regexp.MustCompile(`^([^,]+),(-?\d+)\s+(.+)$`)
	targets := make([]string, 0)
	for _, line := range strings.Split(output, "\n") {
		if 0 == len(line) {
			continue
		}
		matches := regex.FindStringSubmatch(line)
		if 4 == len(matches) {
			portal := matches[1]
			iqn := matches[3]
			if tp == portal {
				targets = append(targets, iqn)
			}
		}
	}
	return targets
}

// execIscsiadmCommand uses the 'iscsiadm' command to perform operations
func (client *Client) execIscsiadmCommand(ctx context.Context, args ...string) ([]byte, error) {
	return client.command.Execute(ctx, "iscsiadm", args...)
}

// execIscsiadmCommandWithTimeout uses the 'iscsiadm' command to perform operations with timeout
func (client *Client) execIscsiadmCommandWithTimeout(ctx context.Context, timeout time.Duration,
	args ...string,
) ([]byte, error) {
	return client.command.ExecuteWithTimeout(ctx, "iscsiadm", timeout, true, args...)
}

// execIscsiadmCommandRedacted uses the 'iscsiadm' command to perform operations without logging specified secrets
func (client *Client) execIscsiadmCommandRedacted(ctx context.Context, args []string,
	secretsToRedact map[string]string) ([]byte,
	error,
) {
	return client.command.ExecuteRedacted(ctx, "iscsiadm", args, secretsToRedact)
}

// RemoveLUNFromSessions removes portal LUN mappings
func (client *Client) RemoveLUNFromSessions(ctx context.Context, publishInfo *models.VolumePublishInfo,
	sessions *models.ISCSISessions,
) {
	if sessions == nil || len(sessions.Info) == 0 {
		Logc(ctx).Debug("No sessions found, nothing to remove.")
		return
	}

	lunNumber := publishInfo.IscsiLunNumber
	allPortals := append(publishInfo.IscsiPortals, publishInfo.IscsiTargetPortal)
	for _, portal := range allPortals {
		sessions.RemoveLUNFromPortal(portal, lunNumber)
	}
}

// TargetHasMountedDevice returns true if this host has any mounted devices on the specified target.
func (client *Client) TargetHasMountedDevice(ctx context.Context, targetIQN string) (bool, error) {
	mountedISCSIDevices, err := client.deviceClient.GetMountedISCSIDevices(ctx)
	if err != nil {
		return false, err
	}

	for _, device := range mountedISCSIDevices {
		if device.IQN == targetIQN {
			return true, nil
		}
	}

	return false, nil
}

// SafeToLogOut looks for remaining block devices on a given iSCSI host, and returns
// true if there are none, indicating that logging out would be safe.
func (client *Client) SafeToLogOut(ctx context.Context, hostNumber, sessionNumber int) bool {
	Logc(ctx).Debug(">>>> iscsi.SafeToLogOut")
	defer Logc(ctx).Debug("<<<< iscsi.SafeToLogOut")

	devicePath := fmt.Sprintf("/sys/class/iscsi_host/host%d/device", hostNumber)

	// The list of block devices on the scsi bus will be in a
	// directory called "target%d:%d:%d".
	// See drivers/scsi/scsi_scan.c in Linux
	// We assume the channel/bus and device/controller are always zero for iSCSI
	targetPath := devicePath + fmt.Sprintf("/session%d/target%d:0:0", sessionNumber, hostNumber)
	dirs, err := os.ReadDir(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		Logc(ctx).WithFields(LogFields{
			"path":  targetPath,
			"error": err,
		}).Warn("Failed to read dir")
		return true
	}

	// The existence of any directories here indicate devices that
	// still exist, so report unsafe
	if 0 < len(dirs) {
		return false
	}

	return true
}

// RemovePortalsFromSession removes portals from portal LUN mapping
func (client *Client) RemovePortalsFromSession(
	ctx context.Context, publishInfo *models.VolumePublishInfo, sessions *models.ISCSISessions,
) {
	if sessions == nil || len(sessions.Info) == 0 {
		Logc(ctx).Debug("No sessions found, nothing to remove.")
		return
	}

	allPortals := append(publishInfo.IscsiPortals, publishInfo.IscsiTargetPortal)
	for _, portal := range allPortals {
		sessions.RemovePortal(portal)
	}
}

// Logout logs out from the supplied target
func (client *Client) Logout(ctx context.Context, targetIQN, targetPortal string) error {
	logFields := LogFields{
		"targetIQN":    targetIQN,
		"targetPortal": targetPortal,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.Logout")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< iscsi.Logout")

	defer client.listAllDevices(ctx)
	if err := beforeIscsiLogout.Inject(); err != nil {
		return err
	}
	if _, err := client.ExecIscsiadmCommand(ctx, "-m", "node", "-T", targetIQN, "--portal", targetPortal,
		"-u"); err != nil {
		Logc(ctx).WithField("error", err).Debug("Error during iSCSI logout.")
	}

	// We used to delete the iscsi "node" at this point but that could interfere with
	// another iSCSI client (such as kubelet with and "iscsi" PV) attempting to use
	// the same node.

	client.listAllDevices(ctx)
	return nil
}

type LuksCloseTimeDurations interface {
	InitLuksCloseStartTime(device string)
	GetLuksCloseDuration(device string) (time.Duration, error)
	RemoveLuksCloseDurationTracking(device string)
}
