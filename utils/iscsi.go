// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
)

const (
	iSCSIErrNoObjsFound                 = 21
	ISCSIErrLoginAuthFailed             = 24
	multipathDeviceDiscoveryTimeoutSecs = 90
	temporaryMountDir                   = "/tmp_mnt"
	volumeMountDir                      = "/vol_mnt"
	unknownFstype                       = "<unknown>"
	iscsiadmLoginTimeout                = "10"
	iscsiadmLoginRetryMax               = "1"
	iSCSISessionStateLoggedIn           = "LOGGED_IN"

	// ISCSI 'multipath -ll' command out states for each HCTL
	iSCSIMultipathStateActive  = "active"
	iSCSIMultipathStateReady   = "ready"
	iSCSIMultipathStateRunning = "running"

	// ISCSI self-healing actions
	ISCSIActionScan            = 0
	ISCSIActionLoginScan       = 1
	ISCSIActionLogoutLoginScan = 2
	maxAttemptsForLogout       = 3
)

var (
	// PublishedPortalLUNs is to track published portal-LUN in memory map
	PublishedPortalLUNs = &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}

	// DiffPortalLUNs is to track difference of published portal-LUN state with current state of the node
	DiffPortalLUNs = &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}

	// currentFixAttempt is running counter for fix attempt for a session
	currentFixAttemptCount int32
)

var IscsiUtils = NewIscsiReconcileUtils()

// AttachISCSIVolumeRetry attaches a volume with retry by invoking AttachISCSIVolume with backoff.
func AttachISCSIVolumeRetry(
	ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo, secrets map[string]string, timeout time.Duration,
) error {
	Logc(ctx).Debug(">>>> iscsi.AttachISCSIVolumeRetry")
	defer Logc(ctx).Debug("<<<< iscsi.AttachISCSIVolumeRetry")
	var err error

	if err = iSCSIPreChecks(ctx); err != nil {
		return err
	}

	checkAttachISCSIVolume := func() error {
		return AttachISCSIVolume(ctx, name, mountpoint, publishInfo, secrets)
	}

	attachNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Attach iSCSI volume is not yet through, waiting.")
	}

	attachBackoff := backoff.NewExponentialBackOff()
	attachBackoff.InitialInterval = 1 * time.Second
	attachBackoff.Multiplier = 1.414 // approx sqrt(2)
	attachBackoff.RandomizationFactor = 0.1
	attachBackoff.MaxElapsedTime = timeout

	err = backoff.RetryNotify(checkAttachISCSIVolume, attachBackoff, attachNotify)
	return err
}

// AttachISCSIVolume attaches the volume to the local host.  This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.  If the mountpoint
// parameter is specified, the volume will be mounted.  The device path is set on the in-out publishInfo parameter
// so that it may be mounted later instead.
func AttachISCSIVolume(ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo, secrets map[string]string) error {
	Logc(ctx).Debug(">>>> iscsi.AttachISCSIVolume")
	defer Logc(ctx).Debug("<<<< iscsi.AttachISCSIVolume")

	var err error
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

	Logc(ctx).WithFields(log.Fields{
		"volume":         name,
		"mountpoint":     mountpoint,
		"lunID":          lunID,
		"portals":        portals,
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"iscsiInterface": publishInfo.IscsiInterface,
		"fstype":         publishInfo.FilesystemType,
	}).Debug("Attaching iSCSI volume.")

	if err = iSCSIPreChecks(ctx); err != nil {
		return err
	}
	// Ensure we are logged into correct portals
	pendingPortalsToLogin, loggedIn, err := portalsToLogin(ctx, publishInfo.IscsiTargetIQN, portals)
	if err != nil {
		return err
	}

	newLogin, err := EnsureISCSISessions(ctx, publishInfo, pendingPortalsToLogin)

	if !loggedIn && !newLogin {
		return err
	}

	// First attempt to fix invalid serials by rescanning them
	err = handleInvalidSerials(ctx, lunID, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunSerial, rescanOneLun)
	if err != nil {
		return err
	}

	// Then attempt to fix invalid serials by purging them (to be scanned
	// again later)
	err = handleInvalidSerials(ctx, lunID, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunSerial, purgeOneLun)
	if err != nil {
		return err
	}

	// Scan the target and wait for the device(s) to appear
	err = waitForDeviceScan(ctx, lunID, publishInfo.IscsiTargetIQN)
	if err != nil {
		Logc(ctx).Errorf("Could not find iSCSI device: %+v", err)
		return err
	}

	// At this point if the serials are still invalid, give up so the
	// caller can retry (invoking the remediation steps above in the
	// process, if they haven't already been run).
	failHandler := func(ctx context.Context, path string) error {
		Logc(ctx).Error("Detected LUN serial number mismatch, attaching volume would risk data corruption, giving up")
		return fmt.Errorf("LUN serial number mismatch, kernel has stale cached data")
	}
	err = handleInvalidSerials(ctx, lunID, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunSerial, failHandler)
	if err != nil {
		return err
	}

	// Wait for multipath device i.e. /dev/dm-* for the given LUN
	err = waitForMultipathDeviceForLUN(ctx, lunID, publishInfo.IscsiTargetIQN)
	if err != nil {
		return err
	}

	// Lookup all the SCSI device information
	deviceInfo, err := getDeviceInfoForLUN(ctx, lunID, publishInfo.IscsiTargetIQN, false, false)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %v", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get iSCSI device information for LUN %d", lunID)
	}

	Logc(ctx).WithFields(log.Fields{
		"scsiLun":         deviceInfo.LUN,
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
		"iqn":             deviceInfo.IQN,
	}).Debug("Found device.")

	// Make sure we use the proper device (multipath if in use)
	deviceToUse := deviceInfo.Devices[0]
	if deviceInfo.MultipathDevice != "" {
		deviceToUse = deviceInfo.MultipathDevice
	}
	if deviceToUse == "" {
		return fmt.Errorf("could not determine device to use for %v", name)
	}
	devicePath := "/dev/" + deviceToUse
	if err := waitForDevice(ctx, devicePath); err != nil {
		return fmt.Errorf("could not find device %v; %s", devicePath, err)
	}

	var isLUKSDevice, luksFormatted bool
	if publishInfo.LUKSEncryption != "" {
		isLUKSDevice, err = strconv.ParseBool(publishInfo.LUKSEncryption)
		if err != nil {
			return fmt.Errorf("could not parse LUKSEncryption into a bool, got %v", publishInfo.LUKSEncryption)
		}
	}
	if isLUKSDevice {
		// Try to Open with current luks passphrase
		luksPassphrase, ok := secrets["luks-passphrase"]
		if !ok {
			return fmt.Errorf("could not open LUKS device, no passphrase provided")
		}
		luksFormatted, devicePath, err = EnsureLUKSDevice(ctx, devicePath, name, luksPassphrase)
		if err != nil {
			return fmt.Errorf("could not open LUKS device; %v", err)
		}
	}

	// Return the device in the publish info in case the mount will be done later
	publishInfo.DevicePath = devicePath

	if publishInfo.FilesystemType == fsRaw {
		return nil
	}

	existingFstype, err := getDeviceFSType(ctx, devicePath)
	if err != nil {
		return err
	}
	if existingFstype == "" {
		if !isLUKSDevice {
			if unformatted, err := isDeviceUnformatted(ctx, devicePath); err != nil {
				Logc(ctx).WithField("device",
					devicePath).Errorf("Unable to identify if the device is unformatted; err: %v", err)
				return err
			} else if !unformatted {
				Logc(ctx).WithField("device", devicePath).Errorf("Device is not unformatted; err: %v", err)
				return fmt.Errorf("device %v is not unformatted", devicePath)
			}
		} else {
			// We can safely assume if we just luksFormatted the device, we can also add a filesystem without dataloss
			if !luksFormatted {
				Logc(ctx).WithField("device",
					devicePath).Errorf("Unable to identify if the luks device is empty; err: %v", err)
				return err
			}
		}

		Logc(ctx).WithFields(log.Fields{"volume": name, "fstype": publishInfo.FilesystemType}).Debug("Formatting LUN.")
		err := formatVolume(ctx, devicePath, publishInfo.FilesystemType)
		if err != nil {
			return fmt.Errorf("error formatting LUN %s, device %s: %v", name, deviceToUse, err)
		}
	} else if existingFstype != unknownFstype && existingFstype != publishInfo.FilesystemType {
		Logc(ctx).WithFields(log.Fields{
			"volume":          name,
			"existingFstype":  existingFstype,
			"requestedFstype": publishInfo.FilesystemType,
		}).Error("LUN already formatted with a different file system type.")
		return fmt.Errorf("LUN %s, device %s already formatted with other filesystem: %s",
			name, deviceToUse, existingFstype)
	} else {
		Logc(ctx).WithFields(log.Fields{
			"volume": name,
			"fstype": deviceInfo.Filesystem,
		}).Debug("LUN already formatted.")
	}

	// Attempt to resolve any filesystem inconsistencies that might be due to dirty node shutdowns, cloning
	// in-use volumes, or creating volumes from snapshots taken from in-use volumes.  This is only safe to do
	// if a device is not mounted.  The fsck command returns a non-zero exit code if filesystem errors are found,
	// even if they are completely and automatically fixed, so we don't return any error here.
	mounted, err := IsMounted(ctx, devicePath, "", "")
	if err != nil {
		return err
	}
	if !mounted {
		_ = repairVolume(ctx, devicePath, publishInfo.FilesystemType)
	}

	// Optionally mount the device
	if mountpoint != "" {
		if err := MountDevice(ctx, devicePath, mountpoint, publishInfo.MountOptions, false); err != nil {
			return fmt.Errorf("error mounting LUN %v, device %v, mountpoint %v; %s",
				name, deviceToUse, mountpoint, err)
		}
	}

	return nil
}

// GetInitiatorIqns returns parsed contents of /etc/iscsi/initiatorname.iscsi
func GetInitiatorIqns(ctx context.Context) ([]string, error) {
	Logc(ctx).Debug(">>>> iscsi.GetInitiatorIqns")
	defer Logc(ctx).Debug("<<<< iscsi.GetInitiatorIqns")

	out, err := execCommand(ctx, "cat", "/etc/iscsi/initiatorname.iscsi")
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

// GetSysfsBlockDirsForLUN returns the list of directories in sysfs where the block devices should appear
// after the scan is successful. One directory is returned for each path in the host session map.
func (h *IscsiReconcileHelper) GetSysfsBlockDirsForLUN(lunID int, hostSessionMap map[int]int) []string {
	paths := make([]string, 0)
	for hostNumber, sessionNumber := range hostSessionMap {
		p := fmt.Sprintf(
			chrootPathPrefix+"/sys/class/scsi_host/host%d/device/session%d/iscsi_session/session%d/device/target%d:0:0/%d:0:0:%d",
			hostNumber, sessionNumber, sessionNumber, hostNumber, hostNumber, lunID)
		paths = append(paths, p)
	}
	return paths
}

// GetDevicesForLUN find the /dev/sd* device names for an iSCSI LUN.
func (h *IscsiReconcileHelper) GetDevicesForLUN(paths []string) ([]string, error) {
	devices := make([]string, 0)
	for _, p := range paths {
		dirname := p + "/block"
		exists, err := PathExists(dirname)
		if !exists || err != nil {
			continue
		}
		dirFd, err := os.Open(dirname)
		if err != nil {
			return nil, err
		}
		list, err := dirFd.Readdir(1)
		dirFd.Close()
		if err != nil {
			return nil, err
		}
		if 0 == len(list) {
			continue
		}
		devices = append(devices, list[0].Name())
	}
	return devices, nil
}

// waitForDeviceScan scans all paths to a specific LUN and waits until all
// SCSI disk-by-path devices for that LUN are present on the host.
func waitForDeviceScan(ctx context.Context, lunID int, iSCSINodeName string) error {
	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.waitForDeviceScan")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.waitForDeviceScan")

	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	Logc(ctx).WithField("hostSessionMap", hostSessionMap).Debug("Built iSCSI host/session map.")
	hosts := make([]int, 0)
	for hostNumber := range hostSessionMap {
		hosts = append(hosts, hostNumber)
	}

	if err := iSCSIScanTargetLUN(ctx, lunID, hosts); err != nil {
		Logc(ctx).WithField("scanError", err).Error("Could not scan for new LUN.")
	}

	paths := IscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	Logc(ctx).Debugf("Scanning paths: %v", paths)
	found := make([]string, 0)

	allDevicesExist := true

	// Check if all paths present, and return nil (success) if so
	for _, p := range paths {
		dirname := p + "/block"
		exists, err := PathExists(dirname)
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
		if _, err := execCommand(ctx, "ls", "-al", "/dev"); err != nil {
			Logc(ctx).Warnf("Could not run ls -al /dev: %v", err)
		}
		if _, err := execCommand(ctx, "ls", "-al", "/dev/mapper"); err != nil {
			Logc(ctx).Warnf("Could not run ls -al /dev/mapper: %v", err)
		}
		if _, err := execCommand(ctx, "ls", "-al", "/dev/disk/by-path"); err != nil {
			Logc(ctx).Warnf("Could not run ls -al /dev/disk/by-path: %v", err)
		}
		if _, err := execCommand(ctx, "lsscsi"); err != nil {
			Logc(ctx).Warnf("Could not run lsscsi: %v", err)
		}
		if _, err := execCommand(ctx, "lsscsi", "-t"); err != nil {
			Logc(ctx).Warnf("Could not run lsscsi -t: %v", err)
		}
		if _, err := execCommand(ctx, "free"); err != nil {
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

// ISCSISupported returns true if iscsiadm is installed and in the PATH.
func ISCSISupported(ctx context.Context) bool {
	Logc(ctx).Debug(">>>> iscsi.ISCSISupported")
	defer Logc(ctx).Debug("<<<< iscsi.ISCSISupported")

	// run the iscsiadm command to show version to check if iscsiadm is installed
	_, err := execIscsiadmCommand(ctx, "-V")
	if err != nil {
		Logc(ctx).Debug("iscsiadm tools not found on this host.")
		return false
	}
	return true
}

// ISCSIDiscoveryInfo contains information about discovered iSCSI targets.
type ISCSIDiscoveryInfo struct {
	Portal     string
	PortalIP   string
	TargetName string
}

// iSCSIDiscovery uses the 'iscsiadm' command to perform discovery.
func iSCSIDiscovery(ctx context.Context, portal string) ([]ISCSIDiscoveryInfo, error) {
	Logc(ctx).WithField("portal", portal).Debug(">>>> iscsi.iSCSIDiscovery")
	defer Logc(ctx).Debug("<<<< iscsi.iSCSIDiscovery")

	out, err := execIscsiadmCommand(ctx, "-m", "discovery", "-t", "sendtargets", "-p", portal)
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

	var discoveryInfo []ISCSIDiscoveryInfo

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		a := strings.Fields(l)
		if len(a) >= 2 {

			portalIP := ""
			if IPv6Check(a[0]) {
				// This is an IPv6 address
				portalIP = strings.Split(a[0], "]")[0]
				portalIP += "]"
			} else {
				portalIP = strings.Split(a[0], ":")[0]
			}

			discoveryInfo = append(discoveryInfo, ISCSIDiscoveryInfo{
				Portal:     a[0],
				PortalIP:   portalIP,
				TargetName: a[1],
			})

			Logc(ctx).WithFields(log.Fields{
				"Portal":     a[0],
				"PortalIP":   portalIP,
				"TargetName": a[1],
			}).Debug("Adding iSCSI discovery info.")
		}
	}
	return discoveryInfo, nil
}

// ISCSISessionInfo contains information about iSCSI sessions.
type ISCSISessionInfo struct {
	SID        string
	Portal     string
	PortalIP   string
	TargetName string
}

// getISCSISessionInfo parses output from 'iscsiadm -m session' and returns the parsed output.
func getISCSISessionInfo(ctx context.Context) ([]ISCSISessionInfo, error) {
	Logc(ctx).Debug(">>>> iscsi.getISCSISessionInfo")
	defer Logc(ctx).Debug("<<<< iscsi.getISCSISessionInfo")

	out, err := execIscsiadmCommand(ctx, "-m", "session")
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == iSCSIErrNoObjsFound {
			Logc(ctx).Debug("No iSCSI session found.")
			return []ISCSISessionInfo{}, nil
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

	var sessionInfo []ISCSISessionInfo

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

			sessionInfo = append(sessionInfo, ISCSISessionInfo{
				SID:        sid,
				Portal:     a[2],
				PortalIP:   portalIP,
				TargetName: a[3],
			})

			Logc(ctx).WithFields(log.Fields{
				"SID":        sid,
				"Portal":     a[2],
				"PortalIP":   portalIP,
				"TargetName": a[3],
			}).Debug("Adding iSCSI session info.")
		}
	}

	return sessionInfo, nil
}

// ISCSILogout logs out from the supplied target
func ISCSILogout(ctx context.Context, targetIQN, targetPortal string) error {
	logFields := log.Fields{
		"targetIQN":    targetIQN,
		"targetPortal": targetPortal,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.ISCSILogout")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< iscsi.ISCSILogout")

	defer listAllISCSIDevices(ctx)
	if _, err := execIscsiadmCommand(ctx, "-m", "node", "-T", targetIQN, "--portal", targetPortal, "-u"); err != nil {
		Logc(ctx).WithField("error", err).Debug("Error during iSCSI logout.")
	}

	// We used to delete the iscsi "node" at this point but that could interfere with
	// another iSCSI client (such as kubelet with and "iscsi" PV) attempting to use
	// the same node.

	listAllISCSIDevices(ctx)
	return nil
}

// iSCSISessionExists checks to see if a session exists to the specified portal.
func iSCSISessionExists(ctx context.Context, portal string) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi.iSCSISessionExists")
	defer Logc(ctx).Debug("<<<< iscsi.iSCSISessionExists")

	sessionInfo, err := getISCSISessionInfo(ctx)
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

// iSCSISessionExistsToTargetIQN checks to see if a session exists to the specified target.
func iSCSISessionExistsToTargetIQN(ctx context.Context, targetIQN string) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi.iSCSISessionExistsToTargetIQN")
	defer Logc(ctx).Debug("<<<< iscsi.iSCSISessionExistsToTargetIQN")

	sessionInfo, err := getISCSISessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return false, err
	}

	for _, e := range sessionInfo {
		if e.TargetName == targetIQN {
			return true, nil
		}
	}

	return false, nil
}

// portalsToLogin checks to see if session to for all the specified portals exist for the specified
// target. If a session does not exist for a give portal it is added to list of portals that Trident
// needs to login to.
func portalsToLogin(ctx context.Context, targetIQN string, portals []string) ([]string, bool, error) {
	logFields := log.Fields{
		"targetIQN": targetIQN,
		"portals":   portals,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.portalsToLogin")
	defer Logc(ctx).Debug("<<<< iscsi.portalsToLogin")

	portalsNotLoggedIn := make([]string, len(portals))
	copy(portalsNotLoggedIn, portals)

	sessionInfo, err := getISCSISessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return portalsNotLoggedIn, false, err
	}

	for _, e := range sessionInfo {
		if e.TargetName == targetIQN {
			// Portals (portalsNotLoggedIn) may/may not contain anything after ":", so instead of matching complete
			// portal value (with e.Portal), check if e.Portal's IP address matches portal's IP address
			matchFunc := func(main, val string) bool {
				mainIpAddress := parseHostportIP(main)
				valIpAddress := parseHostportIP(val)

				return mainIpAddress == valIpAddress
			}
			portalsNotLoggedIn = RemoveStringFromSliceConditionally(portalsNotLoggedIn, e.Portal, matchFunc)
		}
	}

	loggedIn := len(portals) != len(portalsNotLoggedIn)
	return portalsNotLoggedIn, loggedIn, nil
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

// iSCSIScanTargetLUN scans a single LUN on an iSCSI target to discover it.
func iSCSIScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	fields := log.Fields{"hosts": hosts, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.iSCSIScanTargetLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.iSCSIScanTargetLUN")

	var (
		f   *os.File
		err error
	)

	listAllISCSIDevices(ctx)
	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200); err != nil {
			Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
			return err
		}

		scanCmd := fmt.Sprintf("0 0 %d", lunID)
		if written, err := f.WriteString(scanCmd); err != nil {
			Logc(ctx).WithFields(log.Fields{"file": filename, "error": err}).Warning("Could not write to file.")
			f.Close()
			return err
		} else if written == 0 {
			Logc(ctx).WithField("file", filename).Warning("No data written to file.")
			f.Close()
			return fmt.Errorf("no data written to %s", filename)
		}

		f.Close()

		listAllISCSIDevices(ctx)
		Logc(ctx).WithFields(log.Fields{
			"scanCmd":  scanCmd,
			"scanFile": filename,
		}).Debug("Invoked single-LUN scan.")
	}

	return nil
}

// IsAlreadyAttached checks if there is already an established iSCSI session to the specified LUN.
func IsAlreadyAttached(ctx context.Context, lunID int, targetIqn string) bool {
	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, targetIqn)
	if len(hostSessionMap) == 0 {
		return false
	}

	paths := IscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := IscsiUtils.GetDevicesForLUN(paths)
	if nil != err {
		return false
	}

	// return true even if a single device exists
	return 0 < len(devices)
}

// getLunSerial get Linux's idea of what the LUN serial number is
func getLunSerial(ctx context.Context, path string) (string, error) {
	Logc(ctx).WithField("path", path).Debug("Get LUN Serial")
	// We're going to read the SCSI VPD page 80 serial number
	// information. Linux helpfully provides this through sysfs
	// so we don't need to open the device and send the ioctl
	// ourselves.
	filename := path + "/vpd_pg80"
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	if 4 > len(b) || 0x80 != b[1] {
		Logc(ctx).WithFields(log.Fields{
			"data": b,
		}).Error("VPD page 80 format check failed")
		return "", fmt.Errorf("malformed VPD page 80 data")
	}
	length := int(binary.BigEndian.Uint16(b[2:4]))
	if len(b) != length+4 {
		Logc(ctx).WithFields(log.Fields{
			"actual":   len(b),
			"expected": length + 4,
		}).Error("VPD page 80 length check failed")
		return "", fmt.Errorf("incorrect length for VPD page 80 serial number")
	}
	return string(b[4:]), nil
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
		Logc(ctx).WithFields(log.Fields{"file": filename, "error": err}).Warning("Could not write to file.")
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
		Logc(ctx).WithFields(log.Fields{"file": filename, "error": err}).Warning("Could not write to file.")
		return err
	}
	if written == 0 {
		Logc(ctx).WithField("file", filename).Warning("No data written to file.")
		return errors.New("too few bytes written to sysfs file")
	}

	return nil
}

// handleInvalidSerials checks the LUN serial number for each path of a given LUN, and
// if it doesn't match the expected value, runs a handler function.
func handleInvalidSerials(
	ctx context.Context, lunID int, targetIqn, expectedSerial string,
	handler func(ctx context.Context, path string) error,
) error {
	if "" == expectedSerial {
		// Empty string means don't care
		return nil
	}

	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, targetIqn)
	paths := IscsiUtils.GetSysfsBlockDirsForLUN(lunID, hostSessionMap)
	for _, path := range paths {
		serial, err := getLunSerial(ctx, path)
		if err != nil {
			if os.IsNotExist(err) {
				// LUN either isn't scanned yet, or this kernel
				// doesn't support VPD page 80 in sysfs. Assume
				// correctness and move on
				Logc(ctx).WithFields(log.Fields{
					"lun":    lunID,
					"target": targetIqn,
					"path":   path,
				}).Debug("LUN serial check skipped")
				continue
			}
			return err
		}

		if serial != expectedSerial {
			Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
				"serial": serial,
				"lun":    lunID,
				"target": targetIqn,
				"path":   path,
			}).Debug("LUN serial check passed")
		}
	}

	return nil
}

// GetISCSIHostSessionMapForTarget returns a map of iSCSI host numbers to iSCSI session numbers
// for a given iSCSI target.
func (h *IscsiReconcileHelper) GetISCSIHostSessionMapForTarget(ctx context.Context, iSCSINodeName string) map[int]int {
	fields := log.Fields{"iSCSINodeName": iSCSINodeName}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.GetISCSIHostSessionMapForTarget")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.GetISCSIHostSessionMapForTarget")

	var (
		hostNumber    int
		sessionNumber int
	)

	hostSessionMap := make(map[int]int)

	sysPath := chrootPathPrefix + "/sys/class/iscsi_host/"
	if hostDirs, err := ioutil.ReadDir(sysPath); err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return hostSessionMap
	} else {
		for _, hostDir := range hostDirs {

			hostName := hostDir.Name()
			if !strings.HasPrefix(hostName, "host") {
				continue
			} else if hostNumber, err = strconv.Atoi(strings.TrimPrefix(hostName, "host")); err != nil {
				Logc(ctx).WithField("host", hostName).Error("Could not parse host number")
				continue
			}

			devicePath := sysPath + hostName + "/device/"
			if deviceDirs, err := ioutil.ReadDir(devicePath); err != nil {
				Logc(ctx).WithFields(log.Fields{
					"error":      err,
					"devicePath": devicePath,
				}).Error("Could not read device path.")
				return hostSessionMap
			} else {
				for _, deviceDir := range deviceDirs {

					sessionName := deviceDir.Name()
					if !strings.HasPrefix(sessionName, "session") {
						continue
					} else if sessionNumber, err = strconv.Atoi(strings.TrimPrefix(sessionName,
						"session")); err != nil {
						Logc(ctx).WithField("session", sessionName).Error("Could not parse session number")
						continue
					}

					targetNamePath := devicePath + sessionName + "/iscsi_session/" + sessionName + "/targetname"
					if targetName, err := ioutil.ReadFile(targetNamePath); err != nil {
						Logc(ctx).WithFields(log.Fields{
							"path":  targetNamePath,
							"error": err,
						}).Error("Could not read targetname file")
					} else if strings.TrimSpace(string(targetName)) == iSCSINodeName {

						Logc(ctx).WithFields(log.Fields{
							"hostNumber":    hostNumber,
							"sessionNumber": sessionNumber,
						}).Debug("Found iSCSI host/session.")

						hostSessionMap[hostNumber] = sessionNumber
					}
				}
			}
		}
	}

	return hostSessionMap
}

// ISCSITargetHasMountedDevice returns true if this host has any mounted devices on the specified target.
func ISCSITargetHasMountedDevice(ctx context.Context, targetIQN string) (bool, error) {
	mountedISCSIDevices, err := GetMountedISCSIDevices(ctx)
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

// multipathdIsRunning returns true if the multipath daemon is running.
func multipathdIsRunning(ctx context.Context) bool {
	Logc(ctx).Debug(">>>> iscsi.multipathdIsRunning")
	defer Logc(ctx).Debug("<<<< iscsi.multipathdIsRunning")

	// use pgrep to look for mulipathd in the list of running processes
	out, err := execCommand(ctx, "pgrep", "multipathd")
	if err == nil {
		pid := strings.TrimSpace(string(out))
		if pidRegex.MatchString(pid) {
			Logc(ctx).WithField("pid", pid).Debug("multipathd is running")
			return true
		}
	} else {
		Logc(ctx).Error(err)
	}

	out, err = execCommand(ctx, "multipathd", "show", "daemon")
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

// getTargets gets a list of discovered iSCSI targets
func getTargets(ctx context.Context, tp string) ([]string, error) {
	Logc(ctx).WithFields(log.Fields{
		"Portal": tp,
	}).Debug(">>>> iscsi.getTargets")
	defer Logc(ctx).Debug("<<<< iscsi.getTargets")

	output, err := execCommand(ctx, "iscsiadm", "-m", "node")
	if nil != err {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if iSCSIErrNoObjsFound == status.ExitStatus() {
					Logc(ctx).Debug("No iSCSI nodes found.")
					// No records
					return nil, nil
				}
			}
		}
		Logc(ctx).WithFields(log.Fields{
			"error":  err,
			"output": string(output),
		}).Error("Failed to list nodes")
		return nil, fmt.Errorf("failed to list nodes: %v", err)
	}
	return filterTargets(string(output), tp), nil
}

// updateDiscoveryDb update the iscsi discoverydb with the passed values
func updateDiscoveryDb(ctx context.Context, tp, iface, key, value string) error {
	Logc(ctx).WithFields(log.Fields{
		"Key":       key,
		"Value":     value,
		"Portal":    tp,
		"Interface": iface,
	}).Debug(">>>> iscsi.updateDiscoveryDb")
	defer Logc(ctx).Debug("<<<< iscsi.updateDiscoveryDb")

	output, err := execCommand(ctx, "iscsiadm", "-m", "discoverydb",
		"-t", "st", "-p", tp, "-I", iface, "-o", "update", "-n", key, "-v", value)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
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

// ensureIscsiTarget creates the iSCSI target if we haven't done so already
// This function first checks if the target is already known, and if not,
// uses sendtargets to try to discover it. Because sendtargets will find
// all of the targets given just 1 portal, it will be very common to hit
// the case where the target is already known.
// Note: Adding iSCSI targets using sendtargets rather than static discover
// ensures that targets are added with the correct target group portal tags.
func ensureIscsiTarget(
	ctx context.Context, tp, targetIqn, username, password, targetUsername, targetInitiatorSecret, iface string,
) error {
	Logc(ctx).WithFields(log.Fields{
		"IQN":       targetIqn,
		"Portal":    tp,
		"Interface": iface,
	}).Debug(">>>> iscsi.ensureIscsiTarget")
	defer Logc(ctx).Debug("<<<< iscsi.ensureIscsiTarget")

	targets, err := getTargets(ctx, tp)
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
		_, _ = execCommand(ctx, "iscsiadm", "-m", "discoverydb", "-t", "st", "-p", tp, "-I", iface, "-o", "new")

		err = updateDiscoveryDb(ctx, tp, iface, "discovery.sendtargets.auth.authmethod", "CHAP")
		if err != nil {
			// Already logged
			return err
		}

		err = updateDiscoveryDb(ctx, tp, iface, "discovery.sendtargets.auth.username", username)
		if err != nil {
			// Already logged
			return err
		}

		err = updateDiscoveryDb(ctx, tp, iface, "discovery.sendtargets.auth.password", password)
		if err != nil {
			// Already logged
			return err
		}

		if targetUsername != "" && targetInitiatorSecret != "" {
			// Bidirectional CHAP case

			err = updateDiscoveryDb(ctx, tp, iface, "discovery.sendtargets.auth.username_in", targetUsername)
			if err != nil {
				// Already logged
				return err
			}

			err = updateDiscoveryDb(ctx, tp, iface, "discovery.sendtargets.auth.password_in", targetInitiatorSecret)
			if err != nil {
				// Already logged
				return err
			}
		}
	}

	// Discovery is here. This will populate the iscsiadm database with the
	// ALL of the nodes known to the given portal.
	output, err := execCommand(ctx, "iscsiadm", "-m", "discoverydb",
		"-t", "st", "-p", tp, "-I", iface, "-D")
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"portal": tp,
			"error":  err,
			"output": string(output),
		}).Error("Failed to discover targets")
		return fmt.Errorf("failed to discover targets: %v", err)
	}

	targets = filterTargets(string(output), tp)
	for _, iqn := range targets {
		if targetIqn == iqn {
			Logc(ctx).WithField("Target", iqn).Info("Target discovered successfully")
			// Discovered successfully
			return nil
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"portal": tp,
		"iqn":    targetIqn,
	}).Warning("Target not discovered")
	return fmt.Errorf("target not discovered")
}

// configureISCSITarget updates an iSCSI target configuration values.
func configureISCSITarget(ctx context.Context, iqn, portal, name, value string) error {
	Logc(ctx).WithFields(log.Fields{
		"IQN":    iqn,
		"Portal": portal,
		"Name":   name,
		"Value":  value,
	}).Debug(">>>> iscsi.configureISCSITarget")
	defer Logc(ctx).Debug("<<<< iscsi.configureISCSITarget")

	args := []string{"-m", "node", "-T", iqn, "-p", formatPortal(portal), "-o", "update", "-n", name, "-v", value}
	if _, err := execIscsiadmCommand(ctx, args...); err != nil {
		Logc(ctx).WithField("error", err).Warn("Error configuring iSCSI target.")
		return err
	}
	return nil
}

// GetAllVolumeIDs returns names of all the volume IDs based on tracking files
func GetAllVolumeIDs(ctx context.Context, trackingFileDirectory string) []string {
	Logc(ctx).WithField("trackingFileDirectory", trackingFileDirectory).Debug(">>>> iscsi.GetAllVolumeIDs")
	defer Logc(ctx).Debug("<<<< iscsi.GetAllVolumeIDs")

	files, err := ioutil.ReadDir(trackingFileDirectory)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
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

// LoginISCSITarget logs in to an iSCSI target.
func LoginISCSITarget(ctx context.Context, publishInfo *VolumePublishInfo, portal string) error {
	Logc(ctx).WithFields(log.Fields{
		"IQN":     publishInfo.IscsiTargetIQN,
		"Portal":  portal,
		"iface":   publishInfo.IscsiInterface,
		"useCHAP": publishInfo.UseCHAP,
	}).Debug(">>>> iscsi.LoginISCSITarget")
	defer Logc(ctx).Debug("<<<< iscsi.LoginISCSITarget")

	args := []string{"-m", "node", "-T", publishInfo.IscsiTargetIQN, "-p", formatPortal(portal)}
	listAllISCSIDevices(ctx)
	if publishInfo.UseCHAP {
		secretsToRedact := map[string]string{
			"--value=" + publishInfo.IscsiUsername:        "--value=" + REDACTED,
			"--value=" + publishInfo.IscsiInitiatorSecret: "--value=" + REDACTED,
			"--value=" + publishInfo.IscsiTargetUsername:  "--value=" + REDACTED,
			"--value=" + publishInfo.IscsiTargetSecret:    "--value=" + REDACTED,
		}
		authMethodArgs := append(args,
			[]string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
		if _, err := execIscsiadmCommand(ctx, authMethodArgs...); err != nil {
			Logc(ctx).Error("Error running iscsiadm set authmethod.")
			return err
		}

		authUserArgs := append(args,
			[]string{"--op=update", "--name", "node.session.auth.username", "--value=" + publishInfo.IscsiUsername}...)
		if _, err := execIscsiadmCommandRedacted(ctx, authUserArgs, secretsToRedact); err != nil {
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
		if _, err := execIscsiadmCommandRedacted(ctx, authPasswordArgs, secretsToRedact); err != nil {
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
			if _, err := execIscsiadmCommandRedacted(ctx, targetAuthUserArgs, secretsToRedact); err != nil {
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
			if _, err := execIscsiadmCommandRedacted(ctx, targetAuthPasswordArgs, secretsToRedact); err != nil {
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
			"--value=" + iscsiadmLoginTimeout,
		}...)
	if _, err := execIscsiadmCommand(ctx, loginTimeOutArgs...); err != nil {
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
	if _, err := execIscsiadmCommand(ctx, loginRetryMaxArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm set login retry max.")
		return err
	}

	loginArgs := append(args, []string{"--login"}...)
	if _, err := execIscsiadmCommandWithTimeout(ctx, 10*time.Second, loginArgs...); err != nil {
		Logc(ctx).WithField("error", err).Error("Error logging in to iSCSI target.")
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == ISCSIErrLoginAuthFailed {
			return AuthError("iSCSI login failed: CHAP authorization failure")
		}

		return err
	}
	listAllISCSIDevices(ctx)
	return nil
}

// EnsureISCSISessions this is to make sure that Trident establishes iSCSI sessions with the given list of portals
func EnsureISCSISessions(ctx context.Context, publishInfo *VolumePublishInfo, portals []string) (bool, error) {
	logFields := log.Fields{
		"targetIQN":  publishInfo.IscsiTargetIQN,
		"portalsIps": portals,
		"iface":      publishInfo.IscsiInterface,
		"useCHAP":    publishInfo.UseCHAP,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.EnsureISCSISessions")
	defer Logc(ctx).Debug("<<<< iscsi.EnsureISCSISessions")

	loggedInPortals := make([]string, 0)

	loginFailedDueToChap := false

	for _, portal := range portals {
		listAllISCSIDevices(ctx)

		formattedPortal := formatPortal(portal)
		if err := ensureIscsiTarget(ctx, formattedPortal, publishInfo.IscsiTargetIQN, publishInfo.IscsiUsername,
			publishInfo.IscsiInitiatorSecret, publishInfo.IscsiTargetUsername, publishInfo.IscsiTargetSecret,
			publishInfo.IscsiInterface); nil != err {
			Logc(ctx).WithFields(log.Fields{
				"tp":        formattedPortal,
				"targetIqn": publishInfo.IscsiTargetIQN,
				"iface":     publishInfo.IscsiInterface,
				"err":       err,
			}).Errorf("unable to ensure iSCSI target exists: %v", err)
			continue
		}

		// Set scanning to manual
		// Swallow this error, someone is running an old version of Debian/Ubuntu
		_ = configureISCSITarget(ctx, publishInfo.IscsiTargetIQN, portal, "node.session.scan", "manual")

		// replacement_timeout controls how long iSCSI layer should wait for a timed-out path/session to reestablish
		// itself before failing any commands on it.
		timeout_param := "node.session.timeo.replacement_timeout"
		if err := configureISCSITarget(ctx, publishInfo.IscsiTargetIQN, portal, timeout_param, "5"); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"iqn":    publishInfo.IscsiTargetIQN,
				"portal": portal,
				"name":   timeout_param,
				"value":  "5",
				"err":    err,
			}).Errorf("set replacement timeout failed: %v", err)
			continue
		}

		// Log in to target
		if err := LoginISCSITarget(ctx, publishInfo, portal); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"err":      err,
				"portalIP": portal,
			}).Error("Login to iSCSI target failed.")
			if !loginFailedDueToChap {
				if IsAuthError(err) {
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
		sessionExists, err := iSCSISessionExists(ctx, parseHostportIP(portalInfo))
		if err != nil {
			Logc(ctx).WithFields(log.Fields{
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
			return successfulLogin, AuthError("iSCSI login failed: CHAP authorization failure")
		}
	}

	return successfulLogin, errors.New("iSCSI login failed")
}

func EnsureISCSISessionsWithPortalDiscovery(ctx context.Context, hostDataIPs []string) error {
	for _, ip := range hostDataIPs {
		if err := EnsureISCSISessionWithPortalDiscovery(ctx, ip); nil != err {
			return err
		}
	}
	return nil
}

func EnsureISCSISessionWithPortalDiscovery(ctx context.Context, hostDataIP string) error {
	Logc(ctx).WithField("hostDataIP", hostDataIP).Debug(">>>> iscsi.EnsureISCSISessionWithPortalDiscovery")
	defer Logc(ctx).Debug("<<<< iscsi.EnsureISCSISessionWithPortalDiscovery")

	// Ensure iSCSI is supported on system
	if !ISCSISupported(ctx) {
		return errors.New("iSCSI support not detected")
	}

	// Ensure iSCSI session exists for the specified iSCSI portal
	sessionExists, err := iSCSISessionExists(ctx, hostDataIP)
	if err != nil {
		return fmt.Errorf("could not check for iSCSI session: %v", err)
	}
	if !sessionExists {

		// Run discovery in case we haven't seen this target from this host
		targets, err := iSCSIDiscovery(ctx, hostDataIP)
		if err != nil {
			return fmt.Errorf("could not run iSCSI discovery: %v", err)
		}
		if len(targets) == 0 {
			return errors.New("iSCSI discovery found no targets")
		}

		Logc(ctx).WithFields(log.Fields{
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
				_ = configureISCSITarget(ctx, target.TargetName, target.PortalIP, "node.session.scan", "manual")

				// Update replacement timeout
				err = configureISCSITarget(
					ctx, target.TargetName, target.PortalIP, "node.session.timeo.replacement_timeout", "5")
				if err != nil {
					return fmt.Errorf("set replacement timeout failed: %v", err)
				}
				// Log in to target
				publishInfo := &VolumePublishInfo{}
				publishInfo.UseCHAP = false
				publishInfo.IscsiTargetIQN = target.TargetName
				err = LoginISCSITarget(ctx, publishInfo, target.PortalIP)
				if err != nil {
					return fmt.Errorf("login to iSCSI target failed: %v", err)
				}
			}
		}

		// Recheck to ensure a session is now open
		sessionExists, err = iSCSISessionExists(ctx, hostDataIP)
		if err != nil {
			return fmt.Errorf("could not recheck for iSCSI session: %v", err)
		}
		if !sessionExists {
			return fmt.Errorf("Expected iSCSI session %v NOT found, please login to the iSCSI portal", hostDataIP)
		}
	}

	Logc(ctx).WithField("hostDataIP", hostDataIP).Debug("Found session to iSCSI portal.")

	return nil
}

// execIscsiadmCommand uses the 'iscsiadm' command to perform operations without logging specified secrets
func execIscsiadmCommandRedacted(ctx context.Context, args []string, secretsToRedact map[string]string) ([]byte,
	error,
) {
	return execCommandRedacted(ctx, "iscsiadm", args, secretsToRedact)
}

// execIscsiadmCommand uses the 'iscsiadm' command to perform operations
func execIscsiadmCommand(ctx context.Context, args ...string) ([]byte, error) {
	return execCommand(ctx, "iscsiadm", args...)
}

// execIscsiadmCommandWithTimeout uses the 'iscsiadm' command to perform operations with timeout
func execIscsiadmCommandWithTimeout(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	return execCommandWithTimeout(ctx, "iscsiadm", timeout, true, args...)
}

// SafeToLogOut looks for remaining block devices on a given iSCSI host, and returns
// true if there are none, indicating that logging out would be safe.
func SafeToLogOut(ctx context.Context, hostNumber, sessionNumber int) bool {
	Logc(ctx).Debug(">>>> iscsi.SafeToLogOut")
	defer Logc(ctx).Debug("<<<< iscsi.SafeToLogOut")

	devicePath := fmt.Sprintf("/sys/class/iscsi_host/host%d/device", hostNumber)

	// The list of block devices on the scsi bus will be in a
	// directory called "target%d:%d:%d".
	// See drivers/scsi/scsi_scan.c in Linux
	// We assume the channel/bus and device/controller are always zero for iSCSI
	targetPath := devicePath + fmt.Sprintf("/session%d/target%d:0:0", sessionNumber, hostNumber)
	dirs, err := ioutil.ReadDir(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		Logc(ctx).WithFields(log.Fields{
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

// identifyFindMultipathsValue reads /etc/multipath.conf and identifies find_multipaths value (if set)
func identifyFindMultipathsValue(ctx context.Context) (string, error) {
	if output, err := execCommandWithTimeout(ctx, "multipathd", 5*time.Second, false, "show", "config"); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error": err,
		}).Error("Could not read multipathd configuration")

		return "", fmt.Errorf("could not read multipathd configuration: %v", err)
	} else {
		findMultipathsValue := GetFindMultipathValue(string(output))

		Logc(ctx).WithField("findMultipathsValue", findMultipathsValue).Debug("Multipath find_multipaths value found.")

		return findMultipathsValue, nil
	}
}

// iSCSIPreChecks to check if all the required tools are present and configured correctly for the  volume
// attachment to go through
func iSCSIPreChecks(ctx context.Context) error {
	if !ISCSISupported(ctx) {
		err := errors.New("unable to attach: open-iscsi tools not found on host")
		return err
	}

	if !multipathdIsRunning(ctx) {
		return fmt.Errorf("multipathd is not running")
	} else {
		if findMultipathsValue, err := identifyFindMultipathsValue(ctx); err != nil {
			// If Trident is unable to find the find_multipaths value, assume it to be default "no"
			Logc(ctx).Errorf("unable to get the find_multipaths value from the /etc/multipath.conf: %v", err)
		} else if findMultipathsValue == "yes" || findMultipathsValue == "smart" {
			return fmt.Errorf("multipathd: unsupported find_multipaths: %s value;"+
				" please set the value to no in /etc/multipath.conf file", findMultipathsValue)
		}
	}

	return nil
}

// ReconcileISCSIVolumeInfo returns true if any of the expected conditions for a present volume are true (e.g. the
// expected LUN exists).
func (h *IscsiReconcileHelper) ReconcileISCSIVolumeInfo(
	ctx context.Context, trackingInfo *VolumeTrackingInfo,
) (bool, error) {
	pubInfo := trackingInfo.VolumePublishInfo
	lun := int(pubInfo.IscsiLunNumber)
	iqn := pubInfo.IscsiTargetIQN

	sessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, iqn)
	if len(sessionMap) > 0 {
		return true, nil
	}

	paths := IscsiUtils.GetSysfsBlockDirsForLUN(lun, sessionMap)
	devices, err := IscsiUtils.GetDevicesForLUN(paths)
	if err != nil {
		return false, err
	}
	if len(devices) > 0 {
		return true, nil
	}

	return false, nil
}

type SessionPortalInfo struct {
	PortalIP  string
	TargetIQN string
}

// GetCurrentSessionPortalMap - gets the current Session and associated portal map from "iscsiadm -m session"
func GetCurrentSessionPortalMap(ctx context.Context) (map[string]SessionPortalInfo, error) {
	Logc(ctx).Debug(">>>> iscsi.GetCurrentSessionPortalMap")
	defer Logc(ctx).Debug("<<<< iscsi.GetCurrentSessionPortalMap")

	mapSessionToPortal := make(map[string]SessionPortalInfo)

	sessionInfo, err := getISCSISessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return nil, err
	}

	for _, sInfo := range sessionInfo {
		mapSessionToPortal[sInfo.SID] = SessionPortalInfo{sInfo.PortalIP, sInfo.TargetName}
	}

	Logc(ctx).Debugf("Session:Portal Map - %v", mapSessionToPortal)
	return mapSessionToPortal, nil
}

// GetCurrentMultipathActiveLUNs - extracts the active LUUs as per multipath -ll command output
func GetCurrentMultipathActiveLUNs(ctx context.Context) (map[string]struct{}, error) {
	Logc(ctx).Debug(">>>> iscsi.getCurrentMultipathActiveLUNs")
	defer Logc(ctx).Debug("<<<< iscsi.getCurrentMultipathActiveLUNs")

	activeHCTLs := make(map[string]struct{})

	content, err := execCommand(ctx, "multipath", "-ll")
	if err != nil {
		Logc(ctx).WithField("error", err).Debug("Problem checking multipath info.")
		return nil, err
	}

	/*
		#multipath -ll | grep running

		`- 2:0:0:0 sda 8:0  active ready running
		`- 3:0:0:4 sde 8:64 active ready running
		`- 3:0:0:3 sdd 8:48 active ready running
		`- 2:0:0:1 sdc 8:32 active ready running
		`- 2:0:0:2 sdb 8:16 failed faulty running

		a[1] == 2:0:0:0 hctl
		a[4] == active, failed, or i/o waiting
		a[5] == ready/faulty
		a[6] == running <-- Shows all the previously attached LUNs irrespective of their state
	*/

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, l := range lines {
		if !strings.Contains(l, "running") {
			continue
		}
		devInfo := strings.Fields(l)
		if len(devInfo) > 6 {
			if devInfo[4] == iSCSIMultipathStateActive &&
				devInfo[5] == iSCSIMultipathStateReady &&
				devInfo[6] == iSCSIMultipathStateRunning {
				activeHCTLs[devInfo[1]] = struct{}{}
			} else {
				Logc(ctx).WithField("HCTL", devInfo[1]).Debug("Skipping faulty connection.")
			}
		}
	}

	return activeHCTLs, nil
}

// GetISCSISessionLUNsInfo - extracts the Luns attached in a session from
// iscsiadm -m session -r<sid> -P3| grep Lun
func GetISCSISessionLUNsInfo(ctx context.Context, sid string) ([]string, error) {
	Logc(ctx).Debug(">>>> iscsi.GetISCSISessionLUNsInfo")
	defer Logc(ctx).Debug("<<<< iscsi.GetISCSISessionLUNsInfo")

	/*
		#iscsiadm -m session -r6 -P3 | grep Lun
			scsi3 Channel 00 Id 0 Lun: 3
			scsi3 Channel 00 Id 0 Lun: 4

		a[0]==host
		a[2]==channel
		a[4]==target
		a[5]==lun

	*/

	var hctlInfo []string

	content, err := execIscsiadmCommand(ctx, "-m", "session", "-r", sid, "-P3")
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == iSCSIErrNoObjsFound {
			Logc(ctx).Debug("No iSCSI session found.")
			return nil, nil
		} else {
			Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
			return nil, err
		}
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, l := range lines {
		if !strings.Contains(l, "Lun:") {
			// skip this line
			continue
		}

		devInfo := strings.Fields(l)
		if len(devInfo) > 6 {
			hostNum := devInfo[0]
			hostNum = hostNum[4:] // extract the host number
			channelNum := 0
			_, err := fmt.Sscan(devInfo[2], &channelNum)
			if err != nil {
				Logc(ctx).WithField("Error", err).Debug("Error converting value; setting channel as 0.")
				channelNum = 0
			}
			ch := strconv.Itoa(channelNum)
			hctl := fmt.Sprintf("%s:%s:%s:%s", hostNum, ch, devInfo[4], devInfo[6])

			hctlInfo = append(hctlInfo, hctl)
		}
	}

	return hctlInfo, nil
}

// IsISCSISessionStale - reads /sys/class/iscsi_session/session<sid>/state and returns true if it is not "LOGGED_IN".git
// Looks that the state of an already established session to identify if it is
// logged in or not, if it is not logged in then it could be a stale session.
// For now, we are relying on the sysfs files
func IsISCSISessionStale(ctx context.Context, sid string) bool {
	Logc(ctx).Debug(">>>> iscsi.IsISCSISessionStale")
	defer Logc(ctx).Debug("<<<< iscsi.IsISCSISessionStale")

	// Find the session state from the session at /sys/class/iscsi_session/sessionXXX/state
	filename := fmt.Sprintf(chrootPathPrefix+"/sys/class/iscsi_session/session%s/state", sid)
	sessionStateBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"path":  filename,
			"error": err,
		}).Error("Could not read session state file")
		return false
	}

	sessionState := strings.TrimSpace(string(sessionStateBytes))

	Logc(ctx).WithFields(log.Fields{
		"sessionID":    sid,
		"sessionState": sessionState,
		"sysfsFile":    filename,
	}).Debug("Found iSCSI session state.")

	return sessionState != iSCSISessionStateLoggedIn
}

// InitiateScanForLuns scans all paths to each of the LUNs passed.
func InitiateScanForLuns(ctx context.Context, luns []int32, iSCSINodeName string) error {
	fields := log.Fields{
		"lunIDs":        luns,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.InitiateScanForLuns")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.InitiateScanForLuns")

	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	Logc(ctx).WithField("hostSessionMap", hostSessionMap).Debug("Built iSCSI host/session map.")
	hosts := make([]int, 0)
	for hostNumber := range hostSessionMap {
		hosts = append(hosts, hostNumber)
	}

	for _, lun := range luns {
		if err := iSCSIScanTargetLUN(ctx, int(lun), hosts); err != nil {
			Logc(ctx).WithField("scanError", err).Error("Could not scan for new LUN.")
		}
	}

	return nil
}

// RemoveFromPortalLUNMapping - removes a portal entry in the given map of portal to LUN mapping
func RemoveFromPortalLUNMapping(ctx context.Context, publishInfo *VolumePublishInfo, portalLUNMap *PortalLUNMapping) {
	if portalLUNMap == nil || len(portalLUNMap.PortalToLUNMapping) == 0 {
		Logc(ctx).Debug("Portal to LUN mapping is empty, nothing to remove.")
		return
	}
	lId := publishInfo.IscsiLunNumber
	allPortals := append(publishInfo.IscsiPortals, publishInfo.IscsiTargetPortal)
	for _, portal := range allPortals {
		portalLUNMap.RemoveLUNFromPortal(portal, lId)
	}
}

// AddToPortalLUNMapping - adds a portal entry to the global PortalLunMapping map. Extracts the
// required iSCSI Target IQN, CHAP Credentials if any from the provided VolumePublishInfo and
// populates the map against the portal
func AddToPortalLUNMapping(ctx context.Context, publishInfo *VolumePublishInfo, volId string) {
	if PublishedPortalLUNs == nil {
		// Initialize and use it
		PublishedPortalLUNs = &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
	}

	newLUNData := LUNData{publishInfo.IscsiLunNumber, volId}

	// Extract required portal info
	credentials := CHAPCredentials{
		publishInfo.IscsiUsername,
		publishInfo.IscsiInitiatorSecret,
		publishInfo.IscsiTargetUsername,
		publishInfo.IscsiTargetSecret,
	}
	portalInfo := PortalInfo{
		publishInfo.IscsiTargetIQN,
		publishInfo.UseCHAP,
		credentials,
		0,
	}

	allPortals := append(publishInfo.IscsiPortals, publishInfo.IscsiTargetPortal)
	for _, portal := range allPortals {
		if !PublishedPortalLUNs.CheckPortalExists(portal) {
			if err := PublishedPortalLUNs.AddPortal(portal, portalInfo); err != nil {
				Logc(ctx).Errorf("Failed to add portal %v to self-healing map; err: %v", portal, err)
			}
		}
		if err := PublishedPortalLUNs.AddLUNToPortal(portal, newLUNData); err != nil {
			Logc(ctx).Errorf("Failed to add LUN %v to portal %v in self-healing map; err:  %v", newLUNData, portal, err)
		}
	}
}

// PopulateCurrentPortalLUNState - populates the "portLunsMap" representing current state of the node.
// Caller of this should provide the "SessionID : <Portal IP, TargetIQN>" information
//  1. Gets active HCTLs from 'multipath -ll' info
//  2. Navigate through SID:Portal map, verify if that is not a stale session.
//     Read the sysfs file /sys/class/iscsi_session/session<id>/state to determine state of session
//     a) If stale session (state != LOGGED_IN), skip to next entry in the map
//     b) If session is active, navigate through the list of LUNs behind this session:
//     - compare with active HCTL info extracted in step 1
//     - if it is not in faulty state, add LUN id to portLUNsMap, else skip to next LUN
func PopulateCurrentPortalLUNState(
	ctx context.Context,
	sidPortalMap map[string]SessionPortalInfo,
) (*PortalLUNMapping, error) {
	var err error
	var lInfo int64
	// Declare to track active host:channel:target:LUN
	var hctls []string
	var mpInfo map[string]struct{}

	portLUNsMap := &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}

	// Get multipath -ll information
	if mpInfo, err = GetCurrentMultipathActiveLUNs(ctx); err != nil {
		Logc(ctx).WithField("Error", err).Debug("Could not get multipath info.")
		return portLUNsMap, err
	}

	for sid, portal := range sidPortalMap {
		if IsISCSISessionStale(ctx, sid) {
			Logc(ctx).WithFields(log.Fields{
				"SessionID": sid,
				"Portal":    portal,
			}).Debug("session is stale.")
			// Stale session, skip.
			continue
		}

		// Add portal to current map first
		portalInfo := PortalInfo{ISCSITargetIQN: portal.TargetIQN}
		if err := portLUNsMap.AddPortal(portal.PortalIP, portalInfo); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"Portal": portal,
				"Error":  err,
			}).Debug("Could not add portal to current state map.")
			// Skip, proceed to verify next session
			continue
		}

		if hctls, err = GetISCSISessionLUNsInfo(ctx, sid); err != nil {
			Logc(ctx).WithField("Error", err).Debug("iSCSI sessions not found.")
			continue
		}

		for _, hctl := range hctls {
			if _, found := mpInfo[hctl]; found {
				hctl = strings.Split(hctl, ":")[3]
				if lInfo, err = strconv.ParseInt(hctl, 10, 32); err != nil {
					Logc(ctx).WithField("LUNNumber", lInfo).Errorf("Could not parse LUN number")
					// Skip, proceed to next LUN
					continue
				}

				lData := LUNData{LUN: int32(lInfo)}
				if err := portLUNsMap.AddLUNToPortal(portal.PortalIP, lData); err != nil {
					Logc(ctx).WithFields(log.Fields{
						"Portal":    portal.PortalIP,
						"LUNNumber": lInfo,
						"Error":     err,
					}).Error("Could not add LUN to portal")
				} else {
					Logc(ctx).WithFields(log.Fields{
						"LUNId":  lInfo,
						"Portal": portal.PortalIP,
					}).Debug("LUN added to portal in current state.")
				}
			}
			// else - multipath reports faulty or failed for this lun hence not found, skip the lun
		}
	}

	Logc(ctx).Debugf("Current state - Port Luns Map: %v", portLUNsMap)
	return portLUNsMap, nil
}

// GetNextSessionToHeal - chooses the next session from the list of stale sessions if any.
// Caller should provide the current state of the node. This compares the pristine PublishedPortalLuns,
// prepares the difference of current with published.
// Navigate through the list and pick up a session randomly.
//   - Not a perfect fairness algorithm to pick the session for healing, but to mitigate some starvation.
func GetNextSessionToHeal(ctx context.Context, currentPortalToLUNState PortalLUNMapping) (string, []int32, int) {
	var returnPortal string
	var err error
	var listLUNsToBeScanned []int32
	var lastFixCounter int32

	if PublishedPortalLUNs.IsEmpty() {
		// Clear DiffPortalLuns, cleanup
		DiffPortalLUNs.RemoveAllPortals()
		return "", nil, 0
	}

	// List out portals that are in PublishedPortalLUNs but not in currentPortalToLUNState.
	// For all the portals exist in PublishedPortalLUNs
	// 1. If a portal does not exist in current state, then need to login and scan.
	// 2. If a portal exists in current state too, identify any missing LUNs, if there are missing LUNs,
	//    need to scan those LUNs, else no action is required.
	//
	// Self-healing does not concern about additional portals in current state compared to published.
	for portal, portalLunData := range PublishedPortalLUNs.PortalToLUNMapping {
		if found := currentPortalToLUNState.CheckPortalExists(portal); !found {
			// No session found, make an entry with empty LUN list
			if err = DiffPortalLUNs.AddPortal(portal, *portalLunData.PortalInfoValue); err != nil {
				Logc(ctx).Debugf("could not add portal %v to diff map", portal)
			}
			continue
		}

		// Portal found in current state, check if there are any missing LUNs from published map
		currentPortLUNInfo := currentPortalToLUNState.GetPortalLUNMapping(portal).LUNInfoValue
		pristinePortLUNInfo := portalLunData.LUNInfoValue
		missingLUNs := pristinePortLUNInfo.IdentifyMissingLUNs(*currentPortLUNInfo)
		if len(missingLUNs) == 0 {
			// No missing LUNs, remove from DiffPortalLUNs if already exists
			DiffPortalLUNs.RemovePortal(portal)
			continue
		}

		// Found missing LUNs
		lData := BuildLUNData(missingLUNs)
		if found := DiffPortalLUNs.CheckPortalExists(portal); !found {
			if err = DiffPortalLUNs.AddPortal(portal, *portalLunData.PortalInfoValue); err != nil {
				Logc(ctx).Debugf("Could not add portal %v to diff map", portal)
			} else {
				// Getting into diff map now (not in immediate prev cycle), set the Fix attempt to running counter.
				// To avoid the corner case of scheduling it for logout action on first fix attempt,
				// assign last fix attempt equal to multiple of maxAttemptsForLogout
				lfa := currentFixAttemptCount
				if lfa%maxAttemptsForLogout == maxAttemptsForLogout-1 {
					lfa++
				}
				// swallow error, error cannot occur as we just added portal
				_ = DiffPortalLUNs.SetLastFixAttemptForPortal(portal, lfa)
				_ = PublishedPortalLUNs.SetLastFixAttemptForPortal(portal, lfa)
			}
		}
		if err = DiffPortalLUNs.AddLUNsToPortal(portal, lData); err != nil {
			Logc(ctx).Debug("Portal information is not valid.")
		}
	}

	Logc(ctx).Debugf("Difference of current portals from Published Portal Lun Mapping: %v", DiffPortalLUNs)
	if DiffPortalLUNs.IsEmpty() {
		// All sessions are healthy. Reset counters in the original memory map 'PublishedPortalLUNs'
		PublishedPortalLUNs.ResetAllLastFixAttempt()
		currentFixAttemptCount = 0
		// Return empty portal, and caller should determine to ignore calling healing attempt on empty
		return "", nil, 0
	}

	pickNone := true
	for portal, portalLunData := range DiffPortalLUNs.PortalToLUNMapping {
		returnPortal = portal
		if portalLunData.PortalInfoValue.GetLastFixAttempt() < currentFixAttemptCount {
			pickNone = false
			break
		}
	}

	if pickNone {
		// all sessions are tried equal number of times. pick any, here we pick the last one  while navigating
		// through DiffPortalLuns, assuming we get random entry from map data structure

		// Increment Fix Attempt counter as all portals got equal number of chances for healing
		currentFixAttemptCount++
	}

	_ = DiffPortalLUNs.IncLastFixAttemptForPortal(returnPortal)
	_ = PublishedPortalLUNs.IncLastFixAttemptForPortal(returnPortal)

	listLUNsToBeScanned, err = DiffPortalLUNs.GetLUNsForPortal(returnPortal)
	if err != nil {
		// Very unlikely, skip this cycle
		Logc(ctx).Debug("Could not get LUNs")
		return "", nil, 0
	}
	lastFixCounter, err = DiffPortalLUNs.GetLastFixAttemptForPortal(returnPortal)
	if err != nil {
		Logc(ctx).Debugf("could not get last fixed attempt for portal: %v", returnPortal)
		lastFixCounter = currentFixAttemptCount
	}
	Logc(ctx).WithFields(log.Fields{
		"Portal":         returnPortal,
		"LastFixAttempt": lastFixCounter,
	}).Debug("Session requires heal.")

	if lastFixCounter%maxAttemptsForLogout == 0 {
		// Logout and login, scan all LUNs
		if allLuns, err := PublishedPortalLUNs.GetLUNsForPortal(returnPortal); err == nil {
			return returnPortal, allLuns, ISCSIActionLogoutLoginScan
		} else {
			Logc(ctx).Debugf("Could not get list of LUNs, error: %v", err)
		}
	}

	if len(listLUNsToBeScanned) == 0 {
		// Try logging in and scan LUNs
		if allLuns, err := PublishedPortalLUNs.GetLUNsForPortal(returnPortal); err == nil {
			return returnPortal, allLuns, ISCSIActionLoginScan
		} else {
			Logc(ctx).Debugf("Could not get list of LUNs, error: %v", err)
		}
	}

	// Some LUNs are missing, just scan the portal in hope to get the devices online.
	return returnPortal, listLUNsToBeScanned, ISCSIActionScan
}

// ISCSISelfHealSession  - prepares the current state of portal to LUN mapping, pass it to
// GetNextSessionToHeal to pick the session. Call the handler to work on login, logout/login or scan action.
func ISCSISelfHealSession(ctx context.Context) (string, []int32, int, error) {
	var err error
	var currentMap *PortalLUNMapping
	var sidPortalMap map[string]SessionPortalInfo
	var portal string
	var listLUNs []int32
	var action int

	if err = iSCSIPreChecks(ctx); err != nil {
		return portal, listLUNs, action, err
	}

	sidPortalMap, err = GetCurrentSessionPortalMap(ctx)
	if err != nil {
		Logc(ctx).WithField("Error", err).
			Debug("Skipping self heal cycle - could not get session<->portal information")
		return portal, listLUNs, action, err
	}

	if currentMap, err = PopulateCurrentPortalLUNState(ctx, sidPortalMap); err != nil {
		Logc(ctx).WithField("Error", err).
			Debug("Skipping self heal cycle - could not determine current state: multipath tool error")
		return portal, listLUNs, action, err
	}
	Logc(ctx).WithField("Portal-LUNs", currentMap).Debug("Current state of portals on node")
	portal, listLUNs, action = GetNextSessionToHeal(ctx, *currentMap)
	Logc(ctx).WithFields(log.Fields{
		"Portal": portal,
		"LUNs":   listLUNs,
		"Action": action,
	}).Debugf("Processing self healing.")

	return portal, listLUNs, action, nil
}

// BuildLUNData - builds slice of LUNData with default volumeID from a given slice of LUN IDs.
func BuildLUNData(luns []int32) []LUNData {
	lData := make([]LUNData, len(luns))
	for i, lId := range luns {
		lData[i] = LUNData{LUN: lId}
	}
	return lData
}

// AddLUN - adds a LUN to the map of LUNs.
func (l *LUNInfo) AddLUN(m LUNData) {
	if l == nil {
		return
	}

	if l.LUNs == nil {
		l.LUNs = make(map[int32]string)
	}
	l.LUNs[m.LUN] = m.VolumeId
}

// AddLUNs - adds the list of LUNs to map of LUNs.
func (l *LUNInfo) AddLUNs(s []LUNData) {
	if l == nil {
		return
	}

	if l.LUNs == nil {
		// Map is empty, initialize.
		l.LUNs = make(map[int32]string)
	}

	for _, k := range s {
		l.LUNs[k.LUN] = k.VolumeId
	}
}

// RemoveLUN - removes given LUN from map of LUNs.
func (l *LUNInfo) RemoveLUN(x int32) {
	if l == nil {
		return
	}
	delete(l.LUNs, x)
}

// IsEmpty - verifies whether there are any LUNs present.
func (l *LUNInfo) IsEmpty() bool {
	if l == nil || len(l.LUNs) == 0 {
		return true
	}

	return false
}

// GetAllLUNs - returns list of LUN IDs.
func (l *LUNInfo) GetAllLUNs() []int32 {
	if l == nil {
		return []int32{}
	}

	luns := make([]int32, 0, len(l.LUNs))
	for k := range l.LUNs {
		luns = append(luns, k)
	}

	return luns
}

// CheckLUNExists - verifies whether the given LUN exists in map of LUNs.
func (l *LUNInfo) CheckLUNExists(x int32) bool {
	if l == nil {
		return false
	}
	if _, ok := l.LUNs[x]; ok {
		return true
	}
	return false
}

// IdentifyMissingLUNs - return the missing LUNs i.e., existing in l but not in m.
func (l *LUNInfo) IdentifyMissingLUNs(m LUNInfo) []int32 {
	if l == nil || len(l.LUNs) == 0 {
		return []int32{}
	}

	if len(m.LUNs) == 0 {
		return l.GetAllLUNs()
	}

	var luns []int32
	for k := range l.LUNs {
		if !m.CheckLUNExists(k) {
			luns = append(luns, k)
		}
	}

	return luns
}

// GetVolumeId - returns volume ID associated with LUN.
func (l *LUNInfo) GetVolumeId(x int32) (string, error) {
	if l == nil || len(l.LUNs) == 0 {
		return "", fmt.Errorf("invalid pointer to LUNInfo")
	}

	if volId, ok := l.LUNs[x]; ok {
		return volId, nil
	}

	return "", fmt.Errorf("LUN not found")
}

func (l *LUNInfo) String() string {
	return fmt.Sprintf("LUNs: %v", l.GetAllLUNs())
}

// GetTargetIQN - returns ISCSITargetIQN.
func (p *PortalInfo) GetTargetIQN() string {
	if p == nil {
		return ""
	}
	return p.ISCSITargetIQN
}

// CHAPInUse - returns the setting of CHAP.
func (p *PortalInfo) CHAPInUse() bool {
	if p == nil {
		return false
	}
	return p.UseCHAP
}

// GetCHAPCredentials - returns the credentials from the PortalInfo.
func (p *PortalInfo) GetCHAPCredentials() CHAPCredentials {
	if p == nil {
		return CHAPCredentials{}
	}
	return p.Credentials
}

// UpdateCHAPCredentials - updates CHAP credentials with provided.
func (p *PortalInfo) UpdateCHAPCredentials(credentials CHAPCredentials) {
	if p != nil {
		p.Credentials = credentials
	}
}

// IsValid verifies if there is non-nul Target IQN set.
func (p *PortalInfo) IsValid() bool {
	if p != nil && p.ISCSITargetIQN != "" {
		return true
	}
	return false
}

func (p *PortalInfo) String() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("iSCSITargetIQN: %v, useCHAP: %v, lastFixAttempt: %v",
		p.ISCSITargetIQN, p.UseCHAP, p.LastFixAttempt)
}

// GetLastFixAttempt - returns the running fix attempt counter if any for the portal.
func (p *PortalInfo) GetLastFixAttempt() int32 {
	if p == nil {
		return -1
	}
	return p.LastFixAttempt
}

// IncLastFixAttempt - increments fix attempt counter by 1.
func (p *PortalInfo) IncLastFixAttempt() {
	if p != nil {
		p.LastFixAttempt++
	}
}

// SetLastFixAttempt - sets fix attempt counter by given value.
func (p *PortalInfo) SetLastFixAttempt(val int32) {
	if p != nil {
		p.LastFixAttempt = val
	}
}

// IsEmpty - Verifies whether the portal to LUN mapping is empty or not.
func (p *PortalLUNMapping) IsEmpty() bool {
	return p == nil || len(p.PortalToLUNMapping) == 0
}

// AddPortal - creates an entry with no LUNs associated.
// Should be idempotent. Do not add if exists already.
func (p *PortalLUNMapping) AddPortal(portal string, portalInfo PortalInfo) error {
	var newLUNInfo LUNInfo

	if !portalInfo.IsValid() {
		return fmt.Errorf("invalid portal, no IQN found")
	}

	if p == nil {
		return fmt.Errorf("portalLunMapping is nil, initialize before use")
	}

	if p.PortalToLUNMapping == nil {
		p.PortalToLUNMapping = make(map[string]PortalLUNData)
	} else {
		if _, found := p.PortalToLUNMapping[portal]; found {
			return fmt.Errorf("portal already exists, not adding")
		}
	}

	newLUNInfo.LUNs = make(map[int32]string)
	portalLunData := PortalLUNData{
		PortalInfoValue: &portalInfo,
		LUNInfoValue:    &newLUNInfo,
	}
	p.PortalToLUNMapping[portal] = portalLunData

	return nil
}

// GetPortalLUNMapping - returns the portalInfo and if any LUNs behind.
func (p *PortalLUNMapping) GetPortalLUNMapping(portal string) *PortalLUNData {
	if p == nil || p.PortalToLUNMapping == nil {
		return nil
	}

	if portalLUNData, found := p.PortalToLUNMapping[portal]; found {
		return &portalLUNData
	}

	return nil
}

// AddLUNToPortal - adds portal and a LUN:VolumeID to map.
func (p *PortalLUNMapping) AddLUNToPortal(portal string, lData LUNData) error {
	portalLunData := p.GetPortalLUNMapping(portal)

	if portalLunData == nil {
		return fmt.Errorf("portal entry not found")
	}
	if !portalLunData.PortalInfoValue.IsValid() {
		return fmt.Errorf("invalid portal information")
	}

	portalLunData.LUNInfoValue.AddLUN(lData)

	return nil
}

// AddLUNsToPortal - adds portal and associated list of LUNs.
func (p *PortalLUNMapping) AddLUNsToPortal(portal string, luns []LUNData) error {
	portalLUNData := p.GetPortalLUNMapping(portal)

	if portalLUNData == nil {
		return fmt.Errorf("portal entry not found")
	}
	if !portalLUNData.PortalInfoValue.IsValid() {
		return fmt.Errorf("invalid portal information")
	}

	portalLUNData.LUNInfoValue.AddLUNs(luns)

	return nil
}

// CheckPortalExists - checks whether the portal is already in the map.
func (p *PortalLUNMapping) CheckPortalExists(portal string) bool {
	if ok := p.GetPortalLUNMapping(portal); ok != nil {
		return true
	}

	return false
}

// RemovePortal - removes portal from portal:LUNs in-mem map.
func (p *PortalLUNMapping) RemovePortal(portal string) {
	if p != nil {
		delete(p.PortalToLUNMapping, portal)
	}
}

// RemoveLUNFromPortal - removes one LUN from the given portal LUN mapping.
func (p *PortalLUNMapping) RemoveLUNFromPortal(portal string, l int32) {
	portalLUNData := p.GetPortalLUNMapping(portal)

	if portalLUNData == nil {
		// do nothing
		return
	}

	portalLUNData.LUNInfoValue.RemoveLUN(l)

	// if this is the last LUN, remove the portal entry
	if portalLUNData.LUNInfoValue.IsEmpty() {
		p.RemovePortal(portal)
	}
}

// String - prints values of the map.
func (p *PortalLUNMapping) String() string {
	if p.IsEmpty() {
		return "empty portal to LUN mapping info"
	}

	var sb strings.Builder
	for portal, portalLunData := range p.PortalToLUNMapping {
		portalInfoString := portalLunData.PortalInfoValue.String()
		lunInfoString := portalLunData.LUNInfoValue.String()
		sb.WriteString(fmt.Sprintf("Portal: {%v}, portalInfo: {%v}, LunInfo:  {%v} ",
			portal, portalInfoString, lunInfoString))
	}

	return sb.String()
}

// GetLUNsForPortal - returns the list of LunIDs associated with this portal/session.
func (p *PortalLUNMapping) GetLUNsForPortal(portal string) ([]int32, error) {
	portalLUNData := p.GetPortalLUNMapping(portal)

	if portalLUNData != nil {
		return portalLUNData.LUNInfoValue.GetAllLUNs(), nil
	} else {
		return nil, fmt.Errorf("portal entry not found, no LUNs returned")
	}
}

// GetLastFixAttemptForPortal - reads the number of attempts to fix any issue for the given portal.
func (p *PortalLUNMapping) GetLastFixAttemptForPortal(portal string) (int32, error) {
	portalLUNData := p.GetPortalLUNMapping(portal)

	if portalLUNData != nil {
		return portalLUNData.PortalInfoValue.GetLastFixAttempt(), nil
	}
	return 0, fmt.Errorf("portal entry not found")
}

// IncLastFixAttemptForPortal - increments counter.
func (p *PortalLUNMapping) IncLastFixAttemptForPortal(portal string) error {
	portalLUNData := p.GetPortalLUNMapping(portal)

	if portalLUNData != nil {
		portalLUNData.PortalInfoValue.IncLastFixAttempt()
		return nil
	}

	return fmt.Errorf("portal entry not found")
}

// SetLastFixAttemptForPortal - sets the running fix attempt counter for the given portal.
func (p *PortalLUNMapping) SetLastFixAttemptForPortal(portal string, val int32) error {
	portalLUNData := p.GetPortalLUNMapping(portal)

	if portalLUNData != nil {
		portalLUNData.PortalInfoValue.SetLastFixAttempt(val)
		return nil
	}

	return fmt.Errorf("portal entry not found")
}

// ResetAllLastFixAttempt - clear the counters
func (p *PortalLUNMapping) ResetAllLastFixAttempt() {
	if p.IsEmpty() {
		return
	}
	for key := range p.PortalToLUNMapping {
		_ = p.SetLastFixAttemptForPortal(key, 0)
	}
}

// RemoveAllPortals - clears all the contents in portal to LUN mapping.
func (p *PortalLUNMapping) RemoveAllPortals() {
	if p.IsEmpty() {
		return
	}

	for k := range p.PortalToLUNMapping {
		delete(p.PortalToLUNMapping, k)
	}
}

// UpdateChapInfoForPortal to update chap info for a given portal.
func (p *PortalLUNMapping) UpdateChapInfoForPortal(portal string, publishInfo *VolumePublishInfo) error {
	if p.IsEmpty() {
		return fmt.Errorf("portal entry not found")
	}
	var portalInfo *PortalInfo
	var err error
	chapInfo := CHAPCredentials{
		ISCSIUsername:        publishInfo.IscsiUsername,
		ISCSITargetUsername:  publishInfo.IscsiTargetUsername,
		ISCSITargetSecret:    publishInfo.IscsiTargetSecret,
		ISCSIInitiatorSecret: publishInfo.IscsiInitiatorSecret,
	}

	if portalInfo, err = p.GetPortalInfo(portal); err != nil {
		return fmt.Errorf("unable to update the chap info")
	}
	portalInfo.UpdateCHAPCredentials(chapInfo)

	return nil
}

// GetVolumeIDForPortal to get any valid volume ID behind a given portal.
func (p *PortalLUNMapping) GetVolumeIDForPortal(portal string) (string, error) {
	if p.IsEmpty() {
		return "", fmt.Errorf("portal entry not found")
	}
	if portal != "" {
		if lunInfoValue, err := p.GetLunInfo(portal); err != nil {
			return "", fmt.Errorf("portal is invalid")
		} else {
			for _, volumeID := range lunInfoValue.LUNs {
				if volumeID != "" {
					return volumeID, nil
				}
			}
			return "", fmt.Errorf("no valid volume id is found")
		}
	} else {
		return "", fmt.Errorf("portal is invalid")
	}
}

// GetPortalInfo to get portal info for a given portal.
func (p *PortalLUNMapping) GetPortalInfo(portal string) (*PortalInfo, error) {
	if p.IsEmpty() {
		return nil, fmt.Errorf("portal entry not found")
	}

	if portal != "" {
		portalLunData := p.GetPortalLUNMapping(portal)
		if portalLunData != nil {
			return portalLunData.PortalInfoValue, nil
		} else {
			return nil, fmt.Errorf("invalid portal lun data ")
		}
	} else {
		return nil, fmt.Errorf("invalid portal")
	}
}

// GetLunInfo to get LUN info value for a given portal.
func (p *PortalLUNMapping) GetLunInfo(portal string) (*LUNInfo, error) {
	if p.IsEmpty() {
		return nil, fmt.Errorf("portal entry not found")
	}

	if portal != "" {
		portalLunData := p.GetPortalLUNMapping(portal)
		if portalLunData != nil {
			return portalLunData.LUNInfoValue, nil
		} else {
			return nil, fmt.Errorf("no lun info found")
		}
	} else {
		return nil, fmt.Errorf("invalid portal")
	}
}

func (p *PortalLUNMapping) CreatePublishInfo(portal string) (*VolumePublishInfo, error) {
	if p.IsEmpty() {
		return nil, fmt.Errorf("portal entry not found")
	}

	portalLunData, err := p.GetPortalInfo(portal)
	if err != nil {
		return nil, err
	}

	chapCredentials := portalLunData.Credentials

	publishInfo := VolumePublishInfo{
		VolumeAccessInfo: VolumeAccessInfo{
			IscsiAccessInfo: IscsiAccessInfo{
				IscsiTargetIQN: portalLunData.ISCSITargetIQN,
				IscsiChapInfo: IscsiChapInfo{
					UseCHAP:              portalLunData.UseCHAP,
					IscsiUsername:        chapCredentials.ISCSIUsername,
					IscsiTargetUsername:  chapCredentials.ISCSITargetUsername,
					IscsiTargetSecret:    chapCredentials.ISCSITargetSecret,
					IscsiInitiatorSecret: chapCredentials.ISCSIInitiatorSecret,
				},
			},
		},
	}

	return &publishInfo, nil
}
