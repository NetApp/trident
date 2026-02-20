// Copyright 2025 NetApp, Inc. All Rights Reserved.

package iscsi

//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_client.go github.com/netapp/trident/utils/iscsi ISCSI
//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_iscsi_os_client.go github.com/netapp/trident/utils/iscsi OS

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/afero"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/network"
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
	DevPrefix = "/dev/"

	sessionStateLoggedIn       = "LOGGED_IN"
	SessionInfoSource          = "sessionSource"
	SessionSourceCurrentStatus = "currentStatus"
	SessionSourceNodeStage     = "nodeStage"
	SessionSourceTrackingInfo  = "trackingInfo"

	iscsiadmLoginTimeoutValue       = 10
	iscsiadmLoginTimeout            = iscsiadmLoginTimeoutValue * time.Second
	iscsiadmAccessiblePortalTimeout = 15 * time.Second
	iscsiadmLoginRetryMax           = "1"

	errTransport        = 4  // ISCSI_ERR_TRANS
	errTransportTimeout = 8  // ISCSI_ERR_TRANS_TIMEOUT
	errPDUTimeout       = 11 // ISCSI_ERR_PDU_TIMEOUT
	errNoObjsFound      = 21
	errLoginAuthFailed  = 24 // ISCSI_ERR_LOGIN_AUTH_FAILED

	// REDACTED is a copy of what is in utils package.
	// we can reference that once we do not have any references into the utils package
	REDACTED = "<REDACTED>"

	temporaryMountDir = "/tmp_mnt"

	devicesRemovalMaxWaitTime = 5 * time.Second
)

var (
	command               = tridentexec.NewCommand()
	portalPortPattern     = regexp.MustCompile(`.+:\d+$`)
	pidRegex              = regexp.MustCompile(`^\d+$`)
	pidRunningOrIdleRegex = regexp.MustCompile(`pid \d+ (running|idle)`)

	iqnRegex      = regexp.MustCompile(`^\s*InitiatorName\s*=\s*(?P<iqn>\S+)(|\s+.*)$`)
	IscsiUtils    = NewReconcileUtils()
	devicesClient = devices.New()

	// perNodeIgroupRegex is used to ensure an igroup meets the following format:
	// <up to and including 59 characters of a container orchestrator node name>-<36 characters of trident version uuid>
	// ex: Kubernetes-NodeA-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z
	perNodeIgroupRegex = regexp.MustCompile(`^[0-9A-z\-.]{1,59}-[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}`)

	beforeFlushDevice                              = fiji.Register("beforeFlushDevice", "devices")
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
	ISCSIActiveOnHost(ctx context.Context, host models.HostSystem) (bool, error)
	GetDeviceInfoForLUN(
		ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, needFSType bool,
	) (*models.ScsiDeviceInfo, error)
	PrepareDeviceForRemoval(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
		allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool) (string, error)
	PopulateCurrentSessions(ctx context.Context, currentMapping *models.ISCSISessions) error
	InspectAllISCSISessions(
		ctx context.Context, publishedSessions, currentSessions *models.ISCSISessions,
		iSCSISessionWaitTime time.Duration,
	) ([]string, []string)
	EnsureSessionWithPortalDiscovery(ctx context.Context, hostDataIP string) error
	EnsureSessionsWithPortalDiscovery(ctx context.Context, hostDataIPs []string) error
	ISCSIDiscovery(ctx context.Context, portal string) ([]models.ISCSIDiscoveryInfo, error)
	Supported(ctx context.Context) bool
	IsPortalAccessible(ctx context.Context, portal string) (bool, error)
	IsSessionStale(ctx context.Context, sessionID string) bool
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

type Client struct {
	chrootPathPrefix     string
	command              tridentexec.Command
	selfHealingExclusion []string
	osClient             OS
	devices              devices.Devices
	fileSystemClient     filesystem.Filesystem
	mountClient          mount.Mount
	iscsiUtils           IscsiReconcileUtils
	os                   afero.Afero
	osUtils              osutils.Utils
}

func New() (*Client, error) {
	chrootPathPrefix := osutils.ChrootPathPrefix

	osUtils := osutils.New()
	reconcileutils := NewReconcileUtils()
	osFs := afero.Afero{Fs: afero.NewOsFs()}
	mountClient, err := mount.New()
	if err != nil {
		return nil, fmt.Errorf("error creating mount client: %v", err)
	}

	fsClient := filesystem.New(mountClient)

	devicesClient := devices.New()
	return NewDetailed(chrootPathPrefix, tridentexec.NewCommand(), DefaultSelfHealingExclusion, osUtils,
		devicesClient, fsClient, mountClient, reconcileutils, osFs, osUtils), nil
}

func NewDetailed(chrootPathPrefix string, command tridentexec.Command, selfHealingExclusion []string, osClient OS,
	devices devices.Devices, fileSystemClient filesystem.Filesystem, mountClient mount.Mount,
	iscsiUtils IscsiReconcileUtils, os afero.Afero, osUtils osutils.Utils,
) *Client {
	return &Client{
		chrootPathPrefix:     chrootPathPrefix,
		command:              command,
		osClient:             osClient,
		devices:              devices,
		fileSystemClient:     fileSystemClient,
		mountClient:          mountClient,
		iscsiUtils:           iscsiUtils,
		selfHealingExclusion: selfHealingExclusion,
		os:                   os,
		osUtils:              osUtils,
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
	portals = append(portals, network.EnsureHostportFormatted(publishInfo.IscsiTargetPortal))

	for _, p := range publishInfo.IscsiPortals {
		portals = append(portals, network.EnsureHostportFormatted(p))
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

	/*
		There is a known race condition with nodeUnstage in a specific scenario where both volumes share the same target portal.
		This occurs when one request with volume.uuid is entering nodeStage and another request with a different volume.uuid
		entering nodeUnstage, but both share the same target portal.

		In this case, there can be a situation where both workflows obtain their respective views or the views they were looking for.
		Meaning that nodeUnstage, when checking whether it is safe to log out from the session, affirms that it can safely log out.
		Meanwhile, nodeStage sees that the device has been discovered and proceeds after waitForDeviceScan.

		However, nodeStage will still error out either at waitForMultipathDeviceForLUN or GetDeviceInfoForLUN below.
		Our backoff mechanism should handle the re-login of the session in such cases.
	*/

	// Wait for multipath device i.e. /dev/dm-* for the given LUN
	err = client.waitForMultipathDeviceForLUN(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN)
	if err != nil {
		return mpathSize, err
	}

	// Lookup all the SCSI device information
	deviceInfo, err := client.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, publishInfo.IscsiTargetIQN, false)
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
	correctMpathSize, mpathSizeCorrect, err := client.devices.VerifyMultipathDeviceSize(ctx, deviceToUse,
		deviceInfo.Devices[0])
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

	if deviceToUse == "" {
		return mpathSize, fmt.Errorf("could not determine device to use for %v", name)
	}

	devicePath := "/dev/" + deviceToUse
	if err := client.devices.WaitForDevice(ctx, devicePath); err != nil {
		return mpathSize, fmt.Errorf("could not find device %v; %s", devicePath, err)
	}

	// Return the device in the publish info in case the mount will be done later
	publishInfo.DevicePath = devicePath

	// If LUKS encryption is requested, ensure the device is formatted and open.
	var luksFormatted bool
	var safeToFormat bool
	isLUKSDevice := convert.ToBool(publishInfo.LUKSEncryption)
	if isLUKSDevice {
		luksDevice := luks.NewDevice(devicePath, name, client.command)
		luksFormatted, safeToFormat, err = luksDevice.EnsureDeviceMappedOnHost(ctx, name, secrets)
		if err != nil {
			return mpathSize, err
		}

		devicePath = luksDevice.MappedDevicePath()
	}

	// Fail fast if the device should be a LUKS device but is not LUKS formatted.
	if isLUKSDevice && !luksFormatted {
		Logc(ctx).WithFields(LogFields{
			"devicePath":      publishInfo.DevicePath,
			"luksMapperPath":  devicePath,
			"isLUKSFormatted": luksFormatted,
			"isLUKSDevice":    isLUKSDevice,
		}).Error("Device should be a LUKS device but is not LUKS formatted.")
		return mpathSize, errors.New("device should be a LUKS device but is not LUKS formatted")
	}

	if publishInfo.FilesystemType == filesystem.Raw {
		return mpathSize, nil
	}

	var existingFstype string
	if isLUKSDevice && safeToFormat {
		existingFstype = ""
	} else {
		existingFstype, err = client.devices.GetDeviceFSType(ctx, devicePath)
		if err != nil {
			return mpathSize, err
		}
	}

	if existingFstype == "" {
		if !isLUKSDevice {
			if unformatted, err := client.devices.IsDeviceUnformatted(ctx, devicePath); err != nil {
				Logc(ctx).WithField(
					"device", devicePath,
				).WithError(err).Errorf("Unable to identify if the device is not formatted.")
				return mpathSize, err
			} else if !unformatted {
				Logc(ctx).WithField(
					"device", devicePath,
				).WithError(err).Errorf("Device is not unformatted.")
				return mpathSize, fmt.Errorf("device %v is not unformatted", devicePath)
			}
		}

		Logc(ctx).WithFields(LogFields{
			"volume":        name,
			"lunID":         lunID,
			"fstype":        publishInfo.FilesystemType,
			"formatOptions": publishInfo.FormatOptions,
		}).Debug("Formatting iSCSI LUN.")
		err = client.fileSystemClient.FormatVolume(ctx, devicePath, publishInfo.FilesystemType, publishInfo.FormatOptions)
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

// AddSession adds a portal and LUN data to the session map. Extracts the
// required iSCSI Target IQN, CHAP Credentials if any from the provided VolumePublishInfo and
// populates the map against the portal.
// NOTE: sessionNumber should only be passed if there is only one portal/targetportal entry in publishInfo.
func (client *Client) AddSession(
	ctx context.Context, sessions *models.ISCSISessions, publishInfo *models.VolumePublishInfo,
	volID, sessionNumber string, reasonInvalid models.PortalInvalid,
) {
	if sessions == nil {
		// Although sessions should never be nil with the current implementation, this check is included as a safeguard
		sessions = models.NewISCSISessions()
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

// filterDevicesBySize builds a map of disk devices to their size, filtered by a minimum size requirement.
// If any errors occur when checking the size of a device, it captures the error and moves onto the next device.
func (client *Client) filterDevicesBySize(
	ctx context.Context, deviceInfo *models.ScsiDeviceInfo, minSize int64,
) (map[string]int64, error) {
	var errs error
	deviceSizeMap := make(map[string]int64, 0)
	for _, diskDevice := range deviceInfo.Devices {
		size, err := client.devices.GetDiskSize(ctx, devices.DevPrefix+diskDevice)
		if err != nil {
			errs = errors.Join(errs, err)
			// Only consider devices whose size can be gathered.
			continue
		}

		if size < minSize {
			// Only consider devices that are undersized.
			deviceSizeMap[diskDevice] = size
		}
	}

	if errs != nil {
		return nil, errs
	}
	return deviceSizeMap, nil
}

// rescanDevices accepts a map of disk devices to sizes and initiates a rescan for each device.
// If any rescan fails it captures the error and moves onto the next rescanning the next device.
func (client *Client) rescanDevices(ctx context.Context, deviceSizeMap map[string]int64) error {
	var errs error
	for diskDevice := range deviceSizeMap {
		if err := client.rescanDisk(ctx, diskDevice); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to rescan disk %s: %s", diskDevice, err))
		}
	}

	if errs != nil {
		return errs
	}
	return nil
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
	deviceInfo, err := client.GetDeviceInfoForLUN(ctx, hostSessionMap, int(lunID), targetIQN, false)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %s", err)
	}

	// Get all disk devices that require a rescan.
	devicesBySize, err := client.filterDevicesBySize(ctx, deviceInfo, minSize)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to read disk size for devices.")
		return err
	}

	if len(devicesBySize) != 0 {
		fields = LogFields{
			"lunID":   lunID,
			"devices": devicesBySize,
			"minSize": minSize,
		}

		Logc(ctx).WithFields(fields).Debug("Found devices that require a rescan.")
		if err := client.rescanDevices(ctx, devicesBySize); err != nil {
			Logc(ctx).WithError(err).Error("Failed to initiate rescanning for devices.")
			return err
		}

		// Sleep for a second to give the SCSI subsystem time to rescan the devices.
		time.Sleep(time.Second)

		// Reread the devices to check if any are undersized.
		devicesBySize, err = client.filterDevicesBySize(ctx, deviceInfo, minSize)
		if err != nil {
			Logc(ctx).WithError(err).Error("Failed to read disk size for devices after rescan.")
			return err
		}

		if len(devicesBySize) != 0 {
			Logc(ctx).WithFields(fields).Error("Some devices are still undersized after rescan.")
			return fmt.Errorf("devices are still undersized after rescan")
		}
	}

	if deviceInfo.MultipathDevice != "" {
		multipathDevice := deviceInfo.MultipathDevice
		size, err := client.devices.GetDiskSize(ctx, devices.DevPrefix+multipathDevice)
		if err != nil {
			return err
		}

		fields = LogFields{"size": size, "minSize": minSize}
		if size < minSize {
			Logc(ctx).WithFields(fields).Debug("Reloading the multipath device.")
			if err := client.reloadMultipathDevice(ctx, multipathDevice); err != nil {
				return err
			}
			time.Sleep(time.Second)

			size, err := client.devices.GetDiskSize(ctx, devices.DevPrefix+multipathDevice)
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

	client.devices.ListAllDevices(ctx)
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

	client.devices.ListAllDevices(ctx)
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
		devices.DevPrefix+multipathDevice)
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
func (client *Client) GetDeviceInfoForLUN(
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

	devicesForLUN, err := client.iscsiUtils.GetDevicesForLUN(paths)
	if err != nil {
		return nil, err
	} else if len(devicesForLUN) == 0 {
		// No devices found may be due to a node reboot
		Logc(ctx).WithFields(fields).Info("No devices found for LUN.")
		return nil, nil
	}

	multipathDevice := ""
	for _, device := range devicesForLUN {
		multipathDevice = client.devices.FindMultipathDeviceForDevice(ctx, device)
		if multipathDevice != "" {
			break
		}
	}

	var devicePath string
	if multipathDevice != "" {
		devicePath = devices.DevPrefix + multipathDevice
	} else {
		devicePath = devices.DevPrefix + devicesForLUN[0]
	}

	fsType := ""
	if needFSType {
		if err = client.devices.EnsureDeviceReadable(ctx, devicePath); err != nil {
			return nil, err
		}

		fsType, err = client.devices.GetDeviceFSType(ctx, devicePath)
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
		ScsiDeviceAddress: models.ScsiDeviceAddress{LUN: strconv.Itoa(lunID)},
		MultipathDevice:   multipathDevice,
		Devices:           devicesForLUN,
		DevicePaths:       paths,
		Filesystem:        fsType,
		IQN:               iSCSINodeName,
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
		multipathDevice = client.devices.FindMultipathDeviceForDevice(ctx, device)
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
func (client *Client) waitForDeviceScan(ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string) error {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.waitForDeviceScan")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.waitForDeviceScan")

	deviceAddresses := make([]models.ScsiDeviceAddress, 0)
	for hostNumber := range hostSessionMap {
		deviceAddresses = append(deviceAddresses, models.ScsiDeviceAddress{
			Host:    strconv.Itoa(hostNumber),
			Channel: models.ScanSCSIDeviceAddressZero,
			Target:  models.ScanSCSIDeviceAddressZero,
			LUN:     strconv.Itoa(lunID),
		})
	}

	if err := client.devices.ScanTargetLUN(ctx, deviceAddresses); err != nil {
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
		if _, err := client.command.Execute(ctx, "ls", "-al", devices.DevMapperRoot); err != nil {
			Logc(ctx).Warnf("Could not run ls -al %s: %v", devices.DevMapperRoot, err)
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
	ctx context.Context, hostSessionMap map[int]int, lunID int, targetIqn,
	expectedSerial string, handler func(ctx context.Context, path string) error,
) error {
	if "" == expectedSerial {
		// Empty string means don't care
		return nil
	}

	paths := client.iscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	for _, path := range paths {
		serial, err := client.devices.GetLunSerial(ctx, path)
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
			mainIpAddress := network.ParseHostportIP(main)
			valIpAddress := network.ParseHostportIP(val)

			return mainIpAddress == valIpAddress
		}

		lenBeforeCheck := len(portalsNotLoggedIn)
		portalsNotLoggedIn = collection.RemoveStringConditionally(portalsNotLoggedIn, e.Portal, matchFunc)
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

func (client *Client) IsSessionStale(ctx context.Context, sessionID string) bool {
	Logc(ctx).WithField("sessionID", sessionID).Debug(">>>> iscsi.IsSessionStale")
	defer Logc(ctx).Debug("<<<< iscsi.IsSessionStale")
	return client.isSessionStale(ctx, sessionID)
}

// isSessionStale - reads /sys/class/iscsi_session/session<sid>/state and returns true if it is not "LOGGED_IN".
// Looks that the state of an already established session to identify if it is
// logged in or not, if it is not logged in then it could be a stale session.
// For now, we are relying on the sysfs files
func (client *Client) isSessionStale(ctx context.Context, sessionID string) bool {
	// Find the session state from the session at /sys/class/iscsi_session/sessionXXX/state
	filename := fmt.Sprintf(client.chrootPathPrefix+"/sys/class/iscsi_session/session%s/state", sessionID)
	sessionStateBytes, err := client.os.ReadFile(filename)
	if err != nil {
		Logc(ctx).WithField("path", filename).WithError(err).Error("Could not read session state file.")
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
			if network.IPv6Check(a[2]) {
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

	multipathDeviceUUID, err := client.devices.GetMultipathDeviceUUID(multipathDevice)
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
		client.devices.ListAllDevices(ctx)

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
		sessionExists, err := client.sessionExists(ctx, network.ParseHostportIP(portalInfo))
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
	client.devices.ListAllDevices(ctx)
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
	client.devices.ListAllDevices(ctx)
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

// PreChecks to check if all the required tools are present and configured correctly for the  volume
// attachment to go through
func (client *Client) PreChecks(ctx context.Context) error {
	if !client.Supported(ctx) {
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

// Supported returns true if iscsiadm is installed and in the PATH.
func (client *Client) Supported(ctx context.Context) bool {
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
	Logc(ctx).Debug(">>>> iscsi.RemoveLUNFromSessions")
	defer Logc(ctx).Debug("<<<< iscsi.RemoveLUNFromSessions")

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
	mountedISCSIDevices, err := client.GetMountedISCSIDevices(ctx)
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
	dirs, err := client.os.ReadDir(targetPath)
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
	Logc(ctx).Debug(">>>> iscsi.RemovePortalsFromSession")
	defer Logc(ctx).Debug("<<<< iscsi.RemovePortalsFromSession")

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

	defer client.devices.ListAllDevices(ctx)
	if err := beforeIscsiLogout.Inject(); err != nil {
		return err
	}
	if _, err := client.execIscsiadmCommand(ctx, "-m", "node", "-T", targetIQN, "--portal", targetPortal,
		"-u"); err != nil {
		Logc(ctx).WithField("error", err).Debug("Error during iSCSI logout.")
	}

	// We used to delete the iscsi "node" at this point but that could interfere with
	// another iSCSI client (such as kubelet with and "iscsi" PV) attempting to use
	// the same node.

	client.devices.ListAllDevices(ctx)
	return nil
}

// GetISCSIDevices returns a list of iSCSI devices that are attached to (but not necessarily mounted on) this host.
func (client *Client) GetISCSIDevices(ctx context.Context, getCredentials bool) ([]*models.ScsiDeviceInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices.GetISCSIDevices")
	defer Logc(ctx).Debug("<<<< devices.GetISCSIDevices")

	devices := make([]*models.ScsiDeviceInfo, 0)
	hostSessionMapCache := make(map[string]map[int]int)

	// Start by reading the sessions from /sys/class/iscsi_session
	sysPath := client.chrootPathPrefix + "/sys/class/iscsi_session/"
	sessionDirs, err := client.os.ReadDir(sysPath)
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
			fileBytes, err := client.os.ReadFile(path)
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
		targetDirs, err := client.os.ReadDir(sessionDevicePath)
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
		hostBusDeviceLunDirs, err := client.os.ReadDir(sessionDeviceHBDPath)
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
			blockDeviceDirs, err := client.os.ReadDir(blockPath)
			if err != nil {
				Logc(ctx).WithField("error", err).Errorf("Could not read %s", blockPath)
				return nil, err
			}

			for _, blockDeviceDir := range blockDeviceDirs {

				blockDeviceName := blockDeviceDir.Name()

				Logc(ctx).WithField("blockDeviceName", blockDeviceName).Debug("Found block device.")

				// Find multipath device, if any
				var slaveDevices []string
				multipathDevice := client.devices.FindMultipathDeviceForDevice(ctx, blockDeviceName)
				if multipathDevice != "" {
					slaveDevices = client.devices.FindDevicesForMultipathDevice(ctx, multipathDevice)
				} else {
					slaveDevices = []string{blockDeviceName}
				}

				// Get the host/session map, using a cached value if available
				hostSessionMap, ok := hostSessionMapCache[targetIQN]
				if !ok {
					hostSessionMap = client.iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, targetIQN)
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
					ScsiDeviceAddress: models.ScsiDeviceAddress{
						Host:    hostNum,
						Channel: busNum,
						Target:  deviceNum,
						LUN:     lunNum,
					},
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

// GetMountedISCSIDevices returns a list of iSCSI devices that are *mounted* on this host.
func (client *Client) GetMountedISCSIDevices(ctx context.Context) ([]*models.ScsiDeviceInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices.GetMountedISCSIDevices")
	defer Logc(ctx).Debug("<<<< devices.GetMountedISCSIDevices")

	procSelfMountinfo, err := client.mountClient.ListProcMountinfo()
	if err != nil {
		return nil, err
	}

	// Get a list of all mounted /dev devices
	mountedDevices := make([]string, 0)
	for _, procMount := range procSelfMountinfo {

		hasDevMountSourcePrefix := strings.HasPrefix(procMount.MountSource, DevPrefix)
		hasPvcMountPoint := strings.Contains(procMount.MountPoint, "/pvc-")

		if !hasPvcMountPoint {
			continue
		}

		var mountedDevice string
		// Resolve any symlinks to get the real device
		if hasDevMountSourcePrefix {
			device, err := client.osUtils.EvalSymlinks(procMount.MountSource)
			if err != nil {
				Logc(ctx).Error(err)
				continue
			}
			mountedDevice = strings.TrimPrefix(device, DevPrefix)
		} else {
			mountedDevice = strings.TrimPrefix(procMount.Root, "/")
		}

		mountedDevices = append(mountedDevices, mountedDevice)
	}

	// Get all known iSCSI devices
	iscsiDevices, err := client.GetISCSIDevices(ctx, false)
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
	client.devices.ListAllDevices(ctx)

	// Flush multipath device
	if !skipFlush {
		err := client.devices.MultipathFlushDevice(ctx, deviceInfo)
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
		err := client.devices.FlushDevice(ctx, deviceInfo, ignoreErrors)
		if err != nil && !ignoreErrors {
			return false, err
		}
	}

	// Remove device
	err := client.devices.RemoveDevice(ctx, deviceInfo.Devices, ignoreErrors)
	if err != nil && !ignoreErrors {
		return false, err
	}

	// Wait for device to be removed. Do not ignore errors here as we need the device removed
	// for the force removal of the multipath device to succeed.
	err = client.devices.WaitForDevicesRemoval(ctx, DevPrefix, deviceInfo.Devices,
		devicesRemovalMaxWaitTime)
	if err != nil {
		return false, err
	}

	client.devices.ListAllDevices(ctx)

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

// PrepareDeviceForRemoval informs Linux that a device will be removed, the function
// also verifies that device being removed is correct based on published device path,
// device serial number (if present) or comparing all publications (allPublishInfos) for
// LUN number uniqueness.
func (client *Client) PrepareDeviceForRemoval(
	ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool,
) (string, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	lunID := int(publishInfo.IscsiLunNumber)
	iSCSINodeName := publishInfo.IscsiTargetIQN

	fields := LogFields{
		"lunID":            lunID,
		"iSCSINodeName":    iSCSINodeName,
		"chrootPathPrefix": client.chrootPathPrefix,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> devices.PrepareDeviceForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< devices.PrepareDeviceForRemoval")

	// CSI Case
	// We can't verify a multipath device if we couldn't find it in sysfs.
	if publishInfo.IscsiTargetPortal != "" && deviceInfo.MultipathDevice != "" {
		_, err := client.devices.VerifyMultipathDevice(ctx, publishInfo, allPublishInfos, deviceInfo)
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

// iSCSIDiscovery uses the 'iscsiadm' command to perform discovery.
func (client *Client) ISCSIDiscovery(ctx context.Context, portal string) ([]models.ISCSIDiscoveryInfo, error) {
	Logc(ctx).WithField("portal", portal).Debug(">>>> iscsi.iSCSIDiscovery")
	defer Logc(ctx).Debug("<<<< iscsi.iSCSIDiscovery")

	out, err := client.execIscsiadmCommand(ctx, "-m", "discovery", "-t", "sendtargets", "-p", portal)
	if err != nil {
		return nil, err
	}

	/*
	   iscsiadm -m discovery -t st -p 10.63.152.249:3260
	   10.63.152.249:3260,1 iqn.1992-08.com.netapp:2752.600a0980006074c20000000056b32c4d
	   10.63.152.250:3260,2 iqn.1992-08.com.netapp:2752.600a0980006074c20000000056b32c4d
	   a[0]==10.63.152.249:3260,1
	   a[1]==iqn.1992-08.com.netapp:2752.600a0980006074c20000000056b32c4d
	   For IPv6
	   [fd20:8b1e:b258:2000:f816:3eff:feec:2]:3260,1038 iqn.1992-08.com.netapp:sn.7894d7af053711ea88b100a0b886136a
	   a[0]==[fd20:8b1e:b258:2000:f816:3eff:feec:2]:3260,1038
	   a[1]==iqn.1992-08.com.netapp:sn.7894d7af053711ea88b100a0b886136a
	*/

	var discoveryInfo []models.ISCSIDiscoveryInfo

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		a := strings.Fields(l)
		if len(a) >= 2 {

			portalIP := ""
			if network.IPv6Check(a[0]) {
				// This is an IPv6 address
				portalIP = strings.Split(a[0], "]")[0]
				portalIP += "]"
			} else {
				portalIP = strings.Split(a[0], ":")[0]
			}

			discoveryInfo = append(discoveryInfo, models.ISCSIDiscoveryInfo{
				Portal:     a[0],
				PortalIP:   portalIP,
				TargetName: a[1],
			})

			Logc(ctx).WithFields(LogFields{
				"Portal":     a[0],
				"PortalIP":   portalIP,
				"TargetName": a[1],
			}).Debug("Adding iSCSI discovery info.")
		}
	}
	return discoveryInfo, nil
}

func (client *Client) PopulateCurrentSessions(ctx context.Context, currentMapping *models.ISCSISessions) error {
	sessionInfos, err := client.getSessionInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get iSCSI session information")
	}

	// List of duplicate portals that can cause issues for self-healing should be excluded.
	var duplicatePortals []string

	sessionIDToPortalMapping := make(map[string]string, 0)
	portals := make(map[string]interface{}, 0)
	for _, sessionInfo := range sessionInfos {
		sessionIDToPortalMapping[sessionInfo.SID] = sessionInfo.PortalIP

		// Identify portals that may appear more than once in self-healing
		if _, found := portals[sessionInfo.PortalIP]; found {
			Logc(ctx).WithField("portal", sessionInfo.PortalIP).Warning(
				"More than one session found for portal; Portal will be excluded from iSCSI self-healing.")
			duplicatePortals = append(duplicatePortals, sessionInfo.PortalIP)
		} else {
			portals[sessionInfo.PortalIP] = new(interface{})
		}
	}

	// Get all known iSCSI devices
	iscsiDevices, err := client.GetISCSIDevices(ctx, true)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Failed to get list of iSCSI devices.")
		return err
	}

	if iscsiDevices == nil || len(iscsiDevices) == 0 {
		Logc(ctx).Debug("No iSCSI devices found.")
	}

	for _, iscsiDevice := range iscsiDevices {

		logFields := LogFields{
			"sessionNumber":   iscsiDevice.SessionNumber,
			"multipathDevice": iscsiDevice.MultipathDevice,
			"targetIQN":       iscsiDevice.IQN,
		}

		lunNumber, err := strconv.ParseInt(iscsiDevice.LUN, 10, 32)
		if err != nil {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Unable to convert LUN to int value.")
			continue
		}

		sessionNumber := strconv.Itoa(iscsiDevice.SessionNumber)
		targetPortal, ok := sessionIDToPortalMapping[sessionNumber]
		if !ok {
			Logc(ctx).WithFields(logFields).Error("Unable to find session information for session.")
			continue
		}

		var publishInfo models.VolumePublishInfo
		publishInfo.IscsiAccessInfo = models.IscsiAccessInfo{
			IscsiLunNumber:    int32(lunNumber),
			IscsiTargetIQN:    iscsiDevice.IQN,
			IscsiTargetPortal: targetPortal,
		}
		publishInfo.IscsiChapInfo = iscsiDevice.CHAPInfo

		logFields["IscsiPortal"] = targetPortal

		reasonInvalid := models.NotInvalid
		if collection.ContainsString(duplicatePortals, targetPortal) {
			Logc(ctx).WithFields(logFields).Warning("Portal value is not unique.")

			reasonInvalid = models.DuplicatePortals // Could be a result of bug in open-iscsi
		} else if iscsiDevice.MultipathDevice == "" {
			Logc(ctx).WithFields(logFields).Warning("Multipath device not found.")

			reasonInvalid = models.MissingMpathDevice // Could be a result of previous invalid multipathing config
		}

		newCtx := context.WithValue(ctx, SessionInfoSource, SessionSourceCurrentStatus)
		client.AddSession(newCtx, currentMapping, &publishInfo, "", sessionNumber, reasonInvalid)
	}

	return nil
}

func (client *Client) EnsureSessionsWithPortalDiscovery(ctx context.Context, hostDataIPs []string) error {
	for _, ip := range hostDataIPs {
		if err := client.EnsureSessionWithPortalDiscovery(ctx, ip); nil != err {
			return err
		}
	}
	return nil
}

func (client *Client) EnsureSessionWithPortalDiscovery(ctx context.Context, hostDataIP string) error {
	Logc(ctx).WithField("hostDataIP", hostDataIP).Debug(">>>> iscsi.EnsureSessionWithPortalDiscovery")
	defer Logc(ctx).Debug("<<<< iscsi.EnsureSessionWithPortalDiscovery")

	// Ensure iSCSI is supported on system
	if !client.Supported(ctx) {
		return errors.New("iSCSI support not detected")
	}

	// Ensure iSCSI session exists for the specified iSCSI portal
	sessionExists, err := client.sessionExists(ctx, hostDataIP)
	if err != nil {
		return fmt.Errorf("could not check for iSCSI session: %v", err)
	}
	if !sessionExists {

		// Run discovery in case we haven't seen this target from this host
		targets, err := client.ISCSIDiscovery(ctx, hostDataIP)
		if err != nil {
			return fmt.Errorf("could not run iSCSI discovery: %v", err)
		}
		if len(targets) == 0 {
			return errors.New("iSCSI discovery found no targets")
		}

		Logc(ctx).WithFields(LogFields{
			"Targets": targets,
		}).Debug("Found matching iSCSI targets.")

		// Determine which target matches the portal we requested
		targetIndex := -1
		for i, target := range targets {
			if strings.Contains(target.PortalIP, hostDataIP) {
				targetIndex = i
				break
			}
		}

		if targetIndex == -1 {
			return fmt.Errorf("iSCSI discovery found no targets with portal %s", hostDataIP)
		}

		// To enable multipath, log in to each discovered target with the same IQN (target name)
		targetName := targets[targetIndex].TargetName
		for _, target := range targets {
			if target.TargetName == targetName {
				// Set scan to manual
				// Swallow this error, someone is running an old version of Debian/Ubuntu
				_ = client.configureTarget(ctx, target.TargetName, target.PortalIP, "node.session.scan", "manual")

				// Update replacement timeout
				err = client.configureTarget(
					ctx, target.TargetName, target.PortalIP, "node.session.timeo.replacement_timeout", "5")
				if err != nil {
					return fmt.Errorf("set replacement timeout failed: %v", err)
				}
				// Log in to target
				publishInfo := &models.VolumePublishInfo{}
				publishInfo.UseCHAP = false
				publishInfo.IscsiTargetIQN = target.TargetName
				err = client.LoginTarget(ctx, publishInfo, target.PortalIP)
				if err != nil {
					return fmt.Errorf("login to iSCSI target failed: %v", err)
				}
			}
		}

		// Recheck to ensure a session is now open
		sessionExists, err = client.sessionExists(ctx, hostDataIP)
		if err != nil {
			return fmt.Errorf("could not recheck for iSCSI session: %v", err)
		}
		if !sessionExists {
			return fmt.Errorf("expected iSCSI session %v NOT found, please login to the iSCSI portal", hostDataIP)
		}
	}

	Logc(ctx).WithField("hostDataIP", hostDataIP).Debug("Found session to iSCSI portal.")

	return nil
}

// InspectAllISCSISessions goes through each iSCSI session in published sessions and creates a list of
// sorted stale iSCSI portals and a sorted list of non-stale iSCSI portals.
// NOTE: Since we do not expect notStalePortals to be very-large (in millions or even in 1000s),
// sorting on the basis of lastAccessTime should not be an expensive operation.
func (client *Client) InspectAllISCSISessions(
	ctx context.Context, publishedSessions, currentSessions *models.ISCSISessions,
	iSCSISessionWaitTime time.Duration,
) ([]string, []string) {
	timeNow := time.Now()
	Logc(ctx).Debugf("Inspecting iSCSI sessions at %v", timeNow)

	if publishedSessions.IsEmpty() {
		Logc(ctx).Debug("Skipping session inspection; no published iSCSI sessions found.")
		return nil, nil
	}

	noCurrentSessionExists := currentSessions.IsEmpty()
	if noCurrentSessionExists {
		Logc(ctx).Debug("No current iSCSI session found.")
	}

	var candidateStalePortals, candidateNonStalePortal []string

	for portal, publishedSessionData := range publishedSessions.Info {
		logFields := LogFields{"portal": portal}

		var publishedPortalInfo, currentPortalInfo *models.PortalInfo

		// GET publishedSessionData and publishedPortalInfo
		if publishedSessionData == nil {
			Logc(ctx).WithFields(logFields).Warning(
				"Ignoring portal; published sessions is missing portal's session data.")
			continue
		}

		if !publishedSessionData.PortalInfo.HasTargetIQN() {
			Logc(ctx).WithFields(logFields).Warning(
				"Ignoring portal; published session's data is missing portal's target IQN.")
			continue
		}
		publishedPortalInfo = &publishedSessionData.PortalInfo

		if noCurrentSessionExists {
			Logc(ctx).WithFields(logFields).Debugf("Portal requires %v; no current session found.", models.LoginScan)
			publishedSessionData.Remediation = models.LoginScan

			candidateNonStalePortal = append(candidateNonStalePortal, portal)
			continue
		}

		// GET currentSessionData and currentPortalInfo
		currentSessionData, err := currentSessions.ISCSISessionData(portal)
		if err != nil {
			if errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(logFields).Warning("Portal is missing from the current sessions.")
				publishedSessionData.Remediation = models.LoginScan

				candidateNonStalePortal = append(candidateNonStalePortal, portal)
			} else {
				Logc(ctx).WithFields(logFields).Error("Unable to get current session information.")
			}
			continue
		} else if currentSessionData == nil {
			Logc(ctx).WithFields(logFields).Warning("Ignoring portal; current session(s) are missing information.")
			continue
		}
		currentPortalInfo = &currentSessionData.PortalInfo

		// Run some validation to ensure currentPortalInfo is not missing any key information and is valid
		if !currentPortalInfo.HasTargetIQN() {
			Logc(ctx).WithFields(logFields).Warning("Ignoring portal; current session data is missing portal's target IQN.")
			continue
		} else if !currentPortalInfo.IsValid() {
			Logc(ctx).WithFields(logFields).Warningf("Ignoring portal; current session data is invalid: %v",
				currentPortalInfo.ReasonInvalid)
			continue
		} else if publishedPortalInfo.ISCSITargetIQN != currentPortalInfo.ISCSITargetIQN {
			// Should never be here
			Logc(ctx).WithFields(logFields).Warningf(
				"Ignoring portal; published session's target IQN '%v' does not match current session's target IQN '%v'",
				publishedPortalInfo.ISCSITargetIQN, currentPortalInfo.ISCSITargetIQN)
			continue
		}

		// At this stage we know Session exists, based on the session number from current session data
		// we can identify if the session is logged in or not (stale state).
		// If stale, an attempt is made to identify if the session should be fixed right now or there is a need to wait.
		if client.isSessionStale(ctx, currentPortalInfo.SessionNumber) {
			action := client.isStalePortal(ctx, publishedPortalInfo, currentPortalInfo, iSCSISessionWaitTime, timeNow, portal)
			publishedSessionData.Remediation = action

			if action != models.NoAction {
				candidateStalePortals = append(candidateStalePortals, portal)
			}
			continue
		}

		// If session is not stale anymore ensure portal's FirstIdentifiedStaleAt value is reset
		if publishedPortalInfo.IsFirstIdentifiedStaleAtSet() {
			Logc(ctx).WithFields(logFields).Debug("Portal not stale anymore.")
			publishedPortalInfo.ResetFirstIdentifiedStaleAt()
		}

		action := isNonStalePortal(ctx, publishedSessionData, currentSessionData, portal)
		publishedSessionData.Remediation = action

		if action != models.NoAction {
			candidateNonStalePortal = append(candidateNonStalePortal, portal)
		} else {
			// The reason we update the last access time for healthy portals here is to ensure these portals
			// do not get higher priority in the next cycle (in case these go bad).
			Logc(ctx).WithFields(logFields).Debug("Portal requires no remediation.")
			publishedPortalInfo.LastAccessTime = timeNow
		}
	}

	// Sort portals based on last access time
	SortPortals(candidateStalePortals, publishedSessions)
	SortPortals(candidateNonStalePortal, publishedSessions)

	return candidateStalePortals, candidateNonStalePortal
}

func (client *Client) IsPortalAccessible(ctx context.Context, portal string) (bool, error) {
	fields := LogFields{"portal": portal}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.IsPortalAccessible")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.IsPortalAccessible")

	isAccessible, err := client.isPortalAccessible(ctx, portal)
	if !isAccessible {
		Logc(ctx).Warn("Portal may require manual intervention; storage network connection is unstable.")
		return false, err
	}
	return true, err
}

// isPortalAccessible checks if the supplied portal is responsive.
// If the portal is reachable and does not use CHAP, this should return true
// If the portal is reachable, CHAP is in use but with stale credentials, this should fail with an authentication error.
// If the portal is not reachable, this should fail with a connection error.
// If it's unclear if the portal is reachable or unreachable, an error will be returned.
func (client *Client) isPortalAccessible(ctx context.Context, portal string) (bool, error) {
	args := []string{"-m", "discovery", "-t", "sendtargets", "-p", portal}
	_, err := client.execIscsiadmCommandWithTimeout(ctx, iscsiadmAccessiblePortalTimeout, args...)
	if err != nil {
		// A timeout here indicates that iscsiadm command didn't return or exit in time.
		// In this case, we cannot determine if the portal is accessible or not, so return early.
		if errors.IsTimeoutError(err) {
			return false, fmt.Errorf("failed to determine if portal is accessible; %w", err)
		}

		exitErr, ok := err.(tridentexec.ExitError)
		if !ok {
			return false, fmt.Errorf("failed to determine if portal is accessible; %w", err)
		}

		// Differentiate between different exit codes.
		switch exitErr.ExitCode() {
		case errLoginAuthFailed:
			return true, errors.AuthError("CHAP authorization failure")
		case errPDUTimeout, errTransportTimeout, errTransport:
			return false, errors.WrapWithConnectionError(err, "inaccessible portal '%s'", portal)
		default:
			return false, fmt.Errorf("unrecognized error: %w", err)
		}
	}

	return true, nil
}

// isStalePortal attempts to identify if a session should be immediately fixed or not using published
// and current credentials or based on iSCSI session wait timer.
// For CHAP: It first inspects published sessions and compare CHAP credentials with current in-use
// FOR CHAP & non-CHAP:
// 1. If the FirstIdentifiedStaleAt is not set, set it; else
// 2. Compare timeNow with the FirstIdentifiedStaleAt; if it exceeds iSCSISessionWaitTime then it returns LoginScan.
// 3. If it exceeds iSCSISessionWaitTime and CHAP credentials are different, it returns LogoutLoginScan.
func (client *Client) isStalePortal(
	ctx context.Context, publishedPortalInfo, currentPortalInfo *models.PortalInfo, iSCSISessionWaitTime time.Duration,
	timeNow time.Time, portal string,
) models.ISCSIAction {
	fields := LogFields{
		"portal":    portal,
		"CHAPInUse": publishedPortalInfo.CHAPInUse(),
	}

	// Identified stale at Time.Now(), and remains there until we remediate it.
	if !publishedPortalInfo.IsFirstIdentifiedStaleAtSet() {
		Logc(ctx).WithFields(fields).Warnf("Portal identified to be stale at %s.", timeNow)
		publishedPortalInfo.FirstIdentifiedStaleAt = timeNow
		return models.NoAction
	} else if timeNow.Sub(publishedPortalInfo.FirstIdentifiedStaleAt) < iSCSISessionWaitTime {
		Logc(ctx).WithFields(fields).Warnf("Portal has not exceeded stale wait time at %s.", timeNow)
		return models.NoAction
	}

	if publishedPortalInfo.CHAPInUse() && (publishedPortalInfo.Credentials != currentPortalInfo.Credentials) {
		Logc(ctx).WithFields(fields).Warn("Portal's published credentials do not match current credentials.")
		return models.LogoutLoginScan
	}

	return models.LoginScan
}

// isNonStalePortal attempts to identify if a session has any issues other than being stale.
// For sessions with no current issues it attempts to identify any strange behavior or state it should
// not be in.
func isNonStalePortal(
	ctx context.Context, publishedSessionData, currentSessionData *models.ISCSISessionData,
	portal string,
) models.ISCSIAction {
	logFields := LogFields{
		"portal":            portal,
		"CHAPInUse":         publishedSessionData.PortalInfo.CHAPInUse(),
		"sessionInfoSource": publishedSessionData.PortalInfo.Source,
	}

	// Identify if LUNs in published sessions are missing from the current sessions
	publishedLUNs := publishedSessionData.LUNs
	if missingLUNs := currentSessionData.LUNs.IdentifyMissingLUNs(publishedLUNs); len(missingLUNs) > 0 {
		Logc(ctx).WithFields(logFields).Warningf("Portal is missing LUN Number(s): %v.", missingLUNs)
		return models.Scan
	}

	// Additional checks are for informational purposes only - no immediate action required.

	// Verify CHAP credentials are not stale, for this logic to work correct ensure last portal
	// update came from NodeStage and not from Tracking Info.
	if publishedSessionData.PortalInfo.Source == SessionSourceNodeStage {
		if publishedSessionData.PortalInfo.CHAPInUse() {
			if !currentSessionData.PortalInfo.CHAPInUse() {
				Logc(ctx).WithFields(logFields).Warning("Portal should be using CHAP.")
			} else if publishedSessionData.PortalInfo.Credentials != currentSessionData.PortalInfo.Credentials {
				Logc(ctx).WithFields(logFields).Warning("Portal may have stale CHAP credentials.")
			}
		} else {
			if currentSessionData.PortalInfo.CHAPInUse() {
				Logc(ctx).WithFields(logFields).Warning("Portal should not be using CHAP.")
			}
		}
	}

	return models.NoAction
}

// SortPortals sorts portals on the basis of their lastAccessTime
func SortPortals(portals []string, publishedSessions *models.ISCSISessions) {
	if len(portals) > 1 {
		sort.Slice(portals, func(i, j int) bool {
			// Get last access time from publish info for both
			iPortal := portals[i]
			jPortal := portals[j]

			iLastAccessTime := publishedSessions.Info[iPortal].PortalInfo.LastAccessTime
			jLastAccessTime := publishedSessions.Info[jPortal].PortalInfo.LastAccessTime

			return iLastAccessTime.Before(jLastAccessTime)
		})
	}
}

// IsPerNodeIgroup accepts an igroup and returns whether that igroup matches the per-node igroup schema.
func IsPerNodeIgroup(igroup string) bool {
	// Trident never creates per-node igroups with "trident" in the name.
	if strings.Contains(igroup, config.OrchestratorName) {
		return false
	}
	return perNodeIgroupRegex.MatchString(igroup)
}

// GetInitiatorIqns returns parsed contents of /etc/iscsi/initiatorname.iscsi
func GetInitiatorIqns(ctx context.Context) ([]string, error) {
	Logc(ctx).Debug(">>>> iscsi.GetInitiatorIqns")
	defer Logc(ctx).Debug("<<<< iscsi.GetInitiatorIqns")

	out, err := command.Execute(ctx, "cat", "/etc/iscsi/initiatorname.iscsi")
	if err != nil {
		Logc(ctx).WithField("Error", err).Warn("Could not read initiatorname.iscsi; perhaps iSCSI is not installed?")
		return nil, err
	}

	return parseInitiatorIQNs(ctx, string(out)), nil
}

// parseInitiatorIQNs accepts the contents of /etc/iscsi/initiatorname.iscsi and returns the IQN(s).
func parseInitiatorIQNs(ctx context.Context, contents string) []string {
	iqns := make([]string, 0)

	lines := strings.Split(contents, "\n")
	for _, line := range lines {

		match := iqnRegex.FindStringSubmatch(line)

		if match == nil {
			continue
		}

		paramsMap := make(map[string]string)
		for i, name := range iqnRegex.SubexpNames() {
			if i > 0 && i <= len(match) {
				paramsMap[name] = match[i]
			}
		}

		if iqn, ok := paramsMap["iqn"]; ok {
			iqns = append(iqns, iqn)
		}
	}

	return iqns
}

// GetAllVolumeIDs returns names of all the volume IDs based on tracking files
func GetAllVolumeIDs(ctx context.Context, trackingFileDirectory string) []string {
	Logc(ctx).WithField("trackingFileDirectory", trackingFileDirectory).Debug(">>>> iscsi.GetAllVolumeIDs")
	defer Logc(ctx).Debug("<<<< iscsi.GetAllVolumeIDs")

	files, err := os.ReadDir(trackingFileDirectory)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
		}).Warn("Failed to get list of tracking files.")

		return nil
	}

	if len(files) == 0 {
		Logc(ctx).Debug("No tracking file found.")
		return nil
	}

	volumeIDs := make([]string, 0)
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "pvc-") && strings.HasSuffix(file.Name(), ".json") {
			volumeIDs = append(volumeIDs, strings.TrimSuffix(file.Name(), ".json"))
		}
	}

	if len(volumeIDs) == 0 {
		Logc(ctx).Debug("No volume ID found.")
	}

	return volumeIDs
}

// InitiateScanForLuns initiates scans for LUNs in a given target against all hosts.
func InitiateScanForLuns(ctx context.Context, luns []int32, target string) error {
	fields := LogFields{
		"luns":   luns,
		"target": target,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.InitiateScanForLuns")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.InitiateScanForLuns")

	deviceAddresses, err := IscsiUtils.DiscoverSCSIAddressMapForTarget(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to discover SCSI address map for target: '%s'; %w", target, err)
	} else if len(deviceAddresses) == 0 {
		return fmt.Errorf("no SCSI addresses found for target: '%s'", target)
	}

	// Build a set of all device addresses -> luns.
	// This should have entries like so: "10:0:0:0", "10:0:0:1", "11:0:0:0", "11:0:0:1", etc.
	// As an example, if 10 LUNs require scan:
	//  "10:0:0:0", "10:0:0:1",...,"10:0:0:9"
	//  "11:0:0:0", "11:0:0:0",...,"11:0:0:9"
	deviceAddressesWithLUNs := make([]models.ScsiDeviceAddress, 0)
	for _, lun := range luns {
		for _, address := range deviceAddresses {
			deviceAddressesWithLUNs = append(deviceAddressesWithLUNs, models.ScsiDeviceAddress{
				Host:    address.Host,
				Channel: address.Channel,
				Target:  address.Target,
				LUN:     strconv.Itoa(int(lun)),
			})
		}
	}

	if err := devicesClient.ScanTargetLUN(ctx, deviceAddressesWithLUNs); err != nil {
		Logc(ctx).WithError(err).Error("Could not initiate scan.")
		return fmt.Errorf("failed to initiate scan; %w", err)
	}

	return nil
}
