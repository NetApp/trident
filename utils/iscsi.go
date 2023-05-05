// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

const (
	iSCSIErrNoObjsFound                 = 21
	ISCSIErrLoginAuthFailed             = 24
	multipathDeviceDiscoveryTimeoutSecs = 90
	temporaryMountDir                   = "/tmp_mnt"
	volumeMountDir                      = "/vol_mnt"
	unknownFstype                       = "<unknown>"
	iscsiadmLoginTimeoutValue           = 10
	iscsiadmLoginTimeout                = iscsiadmLoginTimeoutValue * time.Second
	iscsiadmLoginRetryMax               = "1"
	iSCSISessionStateLoggedIn           = "LOGGED_IN"
	iSCSIMaxFlushWaitDuration           = 6 * time.Minute

	SessionInfoSource          = "sessionSource"
	SessionSourceNodeStage     = "nodeStage"
	SessionSourceTrackingInfo  = "trackingInfo"
	SessionSourceCurrentStatus = "currentStatus"
)

var (

	// Exclusion list contains keywords if found in any Target IQN should not be considered for
	// self-healing.
	// solidfire: Exclude solidfire for now. Solidfire maintains a different handle 'Current Portal'
	//            which is not published or captured in VolumePublishInfo, current self-healing logic does not
	//            work for logout, login or scan as it is designed to work with published portal information.
	iSCSISelfHealingExclusion = []string{"solidfire"}

	IscsiUtils = NewIscsiReconcileUtils()

	// Non-persistent map to maintain flush delays/errors if any, for device path(s).
	iSCSIVolumeFlushExceptions = make(map[string]time.Time)
)

// AttachISCSIVolumeRetry attaches a volume with retry by invoking AttachISCSIVolume with backoff.
func AttachISCSIVolumeRetry(
	ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo, secrets map[string]string, timeout time.Duration,
) error {
	Logc(ctx).Debug(">>>> iscsi.AttachISCSIVolumeRetry")
	defer Logc(ctx).Debug("<<<< iscsi.AttachISCSIVolumeRetry")
	var err error

	if err = ISCSIPreChecks(ctx); err != nil {
		return err
	}

	checkAttachISCSIVolume := func() error {
		return AttachISCSIVolume(ctx, name, mountpoint, publishInfo, secrets)
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

	Logc(ctx).WithFields(LogFields{
		"volume":         name,
		"mountpoint":     mountpoint,
		"lunID":          lunID,
		"portals":        portals,
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"iscsiInterface": publishInfo.IscsiInterface,
		"fstype":         publishInfo.FilesystemType,
	}).Debug("Attaching iSCSI volume.")

	if err = ISCSIPreChecks(ctx); err != nil {
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

	Logc(ctx).WithFields(LogFields{
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
		luksDevice, _ := NewLUKSDevice(devicePath, name)
		luksFormatted, err = EnsureLUKSDeviceMappedOnHost(ctx, luksDevice, name, secrets)
		if err != nil {
			return err
		}
		devicePath = luksDevice.MappedDevicePath()
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

		Logc(ctx).WithFields(LogFields{"volume": name, "fstype": publishInfo.FilesystemType}).Debug("Formatting LUN.")
		err := formatVolume(ctx, devicePath, publishInfo.FilesystemType)
		if err != nil {
			return fmt.Errorf("error formatting LUN %s, device %s: %v", name, deviceToUse, err)
		}
	} else if existingFstype != unknownFstype && existingFstype != publishInfo.FilesystemType {
		Logc(ctx).WithFields(LogFields{
			"volume":          name,
			"existingFstype":  existingFstype,
			"requestedFstype": publishInfo.FilesystemType,
		}).Error("LUN already formatted with a different file system type.")
		return fmt.Errorf("LUN %s, device %s already formatted with other filesystem: %s",
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
	fields := LogFields{
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
		if _, err := execCommand(ctx, "ls", "-al", devMapperRoot); err != nil {
			Logc(ctx).Warnf("Could not run ls -al %s: %v", devMapperRoot, err)
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

			Logc(ctx).WithFields(LogFields{
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

// ISCSILogout logs out from the supplied target
func ISCSILogout(ctx context.Context, targetIQN, targetPortal string) error {
	logFields := LogFields{
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
	logFields := LogFields{
		"targetIQN": targetIQN,
		"portals":   portals,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> iscsi.portalsToLogin")
	defer Logc(ctx).Debug("<<<< iscsi.portalsToLogin")

	portalsInStaleState := make([]string, 0)
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

			lenBeforeCheck := len(portalsNotLoggedIn)
			portalsNotLoggedIn = RemoveStringFromSliceConditionally(portalsNotLoggedIn, e.Portal, matchFunc)
			lenAfterCheck := len(portalsNotLoggedIn)

			// If the portal is logged in ensure it is not stale
			if lenBeforeCheck != lenAfterCheck {
				if IsISCSISessionStale(ctx, e.SID) {
					portalsInStaleState = append(portalsInStaleState, e.Portal)
				}
			}
		}
	}

	if len(portals) == len(portalsInStaleState) {
		return nil, false, fmt.Errorf("no new session to establish and existing session(s) might be in unhealthy state")
	}

	loggedIn := len(portals) != (len(portalsNotLoggedIn) + len(portalsInStaleState))
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

// iSCSIScanTargetLUN scans a single LUN or all the LUNs on an iSCSI target to discover it.
// If all the LUNs are to be scanned please pass -1 for lunID.
func iSCSIScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	fields := LogFields{"hosts": hosts, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.iSCSIScanTargetLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.iSCSIScanTargetLUN")

	var (
		f   *os.File
		err error
	)

	// By default, scan for all the LUNs
	scanCmd := "0 0 -"
	if lunID >= 0 {
		scanCmd = fmt.Sprintf("0 0 %d", lunID)
	}

	listAllISCSIDevices(ctx)
	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o200); err != nil {
			Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
			return err
		}

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

		listAllISCSIDevices(ctx)
		Logc(ctx).WithFields(LogFields{
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

// GetISCSIHostSessionMapForTarget returns a map of iSCSI host numbers to iSCSI session numbers
// for a given iSCSI target.
func (h *IscsiReconcileHelper) GetISCSIHostSessionMapForTarget(ctx context.Context, iSCSINodeName string) map[int]int {
	fields := LogFields{"iSCSINodeName": iSCSINodeName}
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
				Logc(ctx).WithFields(LogFields{
					"error":         err,
					"rawDevicePath": devicePath,
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
						Logc(ctx).WithFields(LogFields{
							"path":  targetNamePath,
							"error": err,
						}).Error("Could not read targetname file")
					} else if strings.TrimSpace(string(targetName)) == iSCSINodeName {

						Logc(ctx).WithFields(LogFields{
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
	Logc(ctx).WithFields(LogFields{
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
		Logc(ctx).WithFields(LogFields{
			"error":  err,
			"output": string(output),
		}).Error("Failed to list nodes")
		return nil, fmt.Errorf("failed to list nodes: %v", err)
	}
	return filterTargets(string(output), tp), nil
}

// updateDiscoveryDb update the iscsi discoverydb with the passed values
func updateDiscoveryDb(ctx context.Context, tp, iface, key, value string) error {
	Logc(ctx).WithFields(LogFields{
		"Key":       key,
		"Value":     value,
		"Portal":    tp,
		"Interface": iface,
	}).Debug(">>>> iscsi.updateDiscoveryDb")
	defer Logc(ctx).Debug("<<<< iscsi.updateDiscoveryDb")

	output, err := execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, "-m", "discoverydb",
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
	Logc(ctx).WithFields(LogFields{
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
		_, _ = execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, "-m", "discoverydb", "-t", "st", "-p", tp,
			"-I",
			iface, "-o", "new")

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
	output, err := execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, "-m", "discoverydb",
		"-t", "st", "-p", tp, "-I", iface, "-D")
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"portal": tp,
			"error":  err,
			"output": string(output),
		}).Error("Failed to discover targets")

		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == ISCSIErrLoginAuthFailed {
			return errors.AuthError("failed to discover targets: CHAP authorization failure")
		}

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

	Logc(ctx).WithFields(LogFields{
		"portal": tp,
		"iqn":    targetIqn,
	}).Warning("Target not discovered")
	return fmt.Errorf("target not discovered")
}

// configureISCSITarget updates an iSCSI target configuration values.
func configureISCSITarget(ctx context.Context, iqn, portal, name, value string) error {
	Logc(ctx).WithFields(LogFields{
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

// LoginISCSITarget logs in to an iSCSI target.
func LoginISCSITarget(ctx context.Context, publishInfo *VolumePublishInfo, portal string) error {
	Logc(ctx).WithFields(LogFields{
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
			fmt.Sprintf("--value=%d", iscsiadmLoginTimeoutValue),
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
	if _, err := execIscsiadmCommandWithTimeout(ctx, iscsiadmLoginTimeout, loginArgs...); err != nil {
		Logc(ctx).WithField("error", err).Error("Error logging in to iSCSI target.")
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == ISCSIErrLoginAuthFailed {
			return errors.AuthError("iSCSI login failed: CHAP authorization failure")
		}

		return err
	}
	listAllISCSIDevices(ctx)
	return nil
}

// EnsureISCSISessions this is to make sure that Trident establishes iSCSI sessions with the given list of portals
func EnsureISCSISessions(ctx context.Context, publishInfo *VolumePublishInfo, portals []string) (bool, error) {
	logFields := LogFields{
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
		_ = configureISCSITarget(ctx, publishInfo.IscsiTargetIQN, portal, "node.session.scan", "manual")

		// replacement_timeout controls how long iSCSI layer should wait for a timed-out path/session to reestablish
		// itself before failing any commands on it.
		timeout_param := "node.session.timeo.replacement_timeout"
		if err := configureISCSITarget(ctx, publishInfo.IscsiTargetIQN, portal, timeout_param, "5"); err != nil {
			Logc(ctx).WithFields(LogFields{
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
		sessionExists, err := iSCSISessionExists(ctx, parseHostportIP(portalInfo))
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
			return fmt.Errorf("expected iSCSI session %v NOT found, please login to the iSCSI portal", hostDataIP)
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

// identifyFindMultipathsValue reads /etc/multipath.conf and identifies find_multipaths value (if set)
func identifyFindMultipathsValue(ctx context.Context) (string, error) {
	if output, err := execCommandWithTimeout(ctx, "multipathd", 5*time.Second, false, "show", "config"); err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
		}).Error("Could not read multipathd configuration")

		return "", fmt.Errorf("could not read multipathd configuration: %v", err)
	} else {
		findMultipathsValue := GetFindMultipathValue(string(output))

		Logc(ctx).WithField("findMultipathsValue", findMultipathsValue).Debug("Multipath find_multipaths value found.")

		return findMultipathsValue, nil
	}
}

// ISCSIPreChecks to check if all the required tools are present and configured correctly for the  volume
// attachment to go through
func ISCSIPreChecks(ctx context.Context) error {
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

// IsISCSISessionStale - reads /sys/class/iscsi_session/session<sid>/state and returns true if it is not "LOGGED_IN".
// Looks that the state of an already established session to identify if it is
// logged in or not, if it is not logged in then it could be a stale session.
// For now, we are relying on the sysfs files
func IsISCSISessionStale(ctx context.Context, sessionID string) bool {
	Logc(ctx).WithField("sessionID", sessionID).Debug(">>>> iscsi.IsISCSISessionStale")
	defer Logc(ctx).Debug("<<<< iscsi.IsISCSISessionStale")

	// Find the session state from the session at /sys/class/iscsi_session/sessionXXX/state
	filename := fmt.Sprintf(chrootPathPrefix+"/sys/class/iscsi_session/session%s/state", sessionID)
	sessionStateBytes, err := ioutil.ReadFile(filename)
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

	return sessionState != iSCSISessionStateLoggedIn
}

// InitiateScanForLuns scans all paths to each of the LUNs passed.
func InitiateScanForLuns(ctx context.Context, luns []int32, iSCSINodeName string) error {
	fields := LogFields{
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

// RemoveLUNFromSessions removes portal LUN mappings
func RemoveLUNFromSessions(ctx context.Context, publishInfo *VolumePublishInfo, sessions *ISCSISessions) {
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

// RemovePortalsFromSession removes portals from portal LUN mapping
func RemovePortalsFromSession(ctx context.Context, publishInfo *VolumePublishInfo, sessions *ISCSISessions) {
	if sessions == nil || len(sessions.Info) == 0 {
		Logc(ctx).Debug("No sessions found, nothing to remove.")
		return
	}

	allPortals := append(publishInfo.IscsiPortals, publishInfo.IscsiTargetPortal)
	for _, portal := range allPortals {
		sessions.RemovePortal(portal)
	}
}

// AddISCSISession adds a portal and LUN data to the session map. Extracts the
// required iSCSI Target IQN, CHAP Credentials if any from the provided VolumePublishInfo and
// populates the map against the portal.
// NOTE: sessionNumber should only be passed if there is only one portal/targetportal entry in publishInfo.
func AddISCSISession(ctx context.Context, sessions *ISCSISessions, publishInfo *VolumePublishInfo,
	volID, sessionNumber string, reasonInvalid PortalInvalid,
) {
	if sessions == nil {
		// Initialize and use it
		sessions = &ISCSISessions{Info: make(map[string]*ISCSISessionData)}
	}

	iSCSITargetIQN := publishInfo.IscsiTargetIQN

	// Check if TargetIQN is empty
	if reasonInvalid == NotInvalid {
		if iSCSITargetIQN == "" {
			Logc(ctx).Errorf("Portal '%v' is missing target IQN; it may not be considered for iSCSI self-healing.",
				publishInfo.IscsiTargetPortal)
			reasonInvalid = MissingTargetIQN
		}
	}

	// Check if TargetIQN contains keywords that is part of the exclusion list
	for _, keyword := range iSCSISelfHealingExclusion {
		if strings.Contains(iSCSITargetIQN, keyword) {
			Logc(ctx).Errorf("Failed to add portal %v to self-healing map; target IQN '%v' contains `%v` keyword",
				publishInfo.IscsiTargetPortal, iSCSITargetIQN, keyword)
			return
		}
	}

	newLUNData := LUNData{
		LUN:   publishInfo.IscsiLunNumber,
		VolID: volID,
	}

	// Capture the source of the session information
	var source string
	if ctxSource := ctx.Value(SessionInfoSource); ctxSource != nil {
		source = fmt.Sprintf("%v", ctxSource)
	}

	// Extract required portal info
	credentials := IscsiChapInfo{
		UseCHAP:              publishInfo.UseCHAP,
		IscsiUsername:        publishInfo.IscsiUsername,
		IscsiInitiatorSecret: publishInfo.IscsiInitiatorSecret,
		IscsiTargetUsername:  publishInfo.IscsiTargetUsername,
		IscsiTargetSecret:    publishInfo.IscsiTargetSecret,
	}
	portalInfo := PortalInfo{
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

func PopulateCurrentSessions(ctx context.Context, currentMapping *ISCSISessions) error {
	sessionInfos, err := getISCSISessionInfo(ctx)
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
	iscsiDevices, err := GetISCSIDevices(ctx, true)
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

		lunNumber, err := strconv.ParseInt(iscsiDevice.LUN, 10, 0)
		if err != nil {
			logFields["error"] = err
			Logc(ctx).WithFields(logFields).Error("Unable to convert LUN to int value.")
			continue
		}

		sessionNumber := strconv.Itoa(iscsiDevice.SessionNumber)
		targetPortal, ok := sessionIDToPortalMapping[sessionNumber]
		if !ok {
			Logc(ctx).WithFields(logFields).Error("Unable to find session information for session.")
			continue
		}

		var publishInfo VolumePublishInfo
		publishInfo.IscsiAccessInfo = IscsiAccessInfo{
			IscsiLunNumber:    int32(lunNumber),
			IscsiTargetIQN:    iscsiDevice.IQN,
			IscsiTargetPortal: targetPortal,
		}
		publishInfo.IscsiChapInfo = iscsiDevice.CHAPInfo

		logFields["IscsiPortal"] = targetPortal

		reasonInvalid := NotInvalid
		if SliceContainsString(duplicatePortals, targetPortal) {
			Logc(ctx).WithFields(logFields).Warning("Portal value is not unique.")

			reasonInvalid = DuplicatePortals // Could be a result of bug in open-iscsi
		} else if iscsiDevice.MultipathDevice == "" {
			Logc(ctx).WithFields(logFields).Warning("Multipath device not found.")

			reasonInvalid = MissingMpathDevice // Could be a result of previous invalid multipathing config
		}

		newCtx := context.WithValue(ctx, SessionInfoSource, SessionSourceCurrentStatus)
		AddISCSISession(newCtx, currentMapping, &publishInfo, "", sessionNumber, reasonInvalid)
	}

	return nil
}

// InspectAllISCSISessions goes through each iSCSI session in published sessions and creates a list of
// sorted stale iSCSI portals and a sorted list of non-stale iSCSI portals.
// NOTE: Since we do not expect notStalePortals to be very-large (in millions or even in 1000s),
// sorting on the basis of lastAccessTime should not be an expensive operation.
func InspectAllISCSISessions(ctx context.Context, publishedSessions, currentSessions *ISCSISessions,
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

		var publishedPortalInfo, currentPortalInfo *PortalInfo

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
			Logc(ctx).WithFields(logFields).Debugf("Portal requires %v; no current session found.", LoginScan)
			publishedSessionData.Remediation = LoginScan

			candidateNonStalePortal = append(candidateNonStalePortal, portal)
			continue
		}

		// GET currentSessionData and currentPortalInfo
		currentSessionData, err := currentSessions.ISCSISessionData(portal)
		if err != nil {
			if errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(logFields).Warning("Portal is missing from the current sessions.")
				publishedSessionData.Remediation = LoginScan

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
		if IsISCSISessionStale(ctx, currentPortalInfo.SessionNumber) {
			action := isStalePortal(ctx, publishedPortalInfo, currentPortalInfo, iSCSISessionWaitTime, timeNow, portal)
			publishedSessionData.Remediation = action

			if action != NoAction {
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

		if action != NoAction {
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

// isStalePortal attempts to identify if a session should be immediately fixed or not using published
// and current credentials or based on iSCSI session wait timer.
// For CHAP: It first inspects published sessions and compare CHAP credentials with current in-use
//
//	CHAP credentials, if different it returns true, else we wait until session exceeds
//	iSCSISessionWaitTime that is the CHAP & non-CHAP scenario. For this logic to work correct
//	ensure last portal update came from NodeStage and not from Tracking Info.
//
// FOR CHAP & non-CHAP:
// 1. If the FirstIdentifiedStaleAt is not set, set it; else
// 2. Compare timeNow with the FirstIdentifiedStaleAt; if it exceeds iSCSISessionWaitTime then it returns true;
// 3. Else do nothing and continue
func isStalePortal(ctx context.Context, publishedPortalInfo, currentPortalInfo *PortalInfo,
	iSCSISessionWaitTime time.Duration, timeNow time.Time, portal string,
) ISCSIAction {
	logFields := LogFields{
		"portal":            portal,
		"CHAPInUse":         publishedPortalInfo.CHAPInUse(),
		"sessionInfoSource": publishedPortalInfo.Source,
	}

	// The reason we can rely on CHAP information from NodeStage and not from TrackingInfo is that
	// the multiple tracking information may contain different CHAP credentials for the same
	// portal and they may be read in any sequence.
	if publishedPortalInfo.CHAPInUse() && publishedPortalInfo.Source == SessionSourceNodeStage {
		if publishedPortalInfo.Credentials != currentPortalInfo.Credentials {
			Logc(ctx).WithFields(logFields).Warning("Portal's published credentials do not match current credentials" +
				" in-use.")
			return LogoutLoginScan
		}
	}

	if !publishedPortalInfo.IsFirstIdentifiedStaleAtSet() {
		Logc(ctx).WithFields(logFields).Warningf("Portal identified to be stale at %v.", timeNow)
		publishedPortalInfo.FirstIdentifiedStaleAt = timeNow
	} else if timeNow.Sub(publishedPortalInfo.FirstIdentifiedStaleAt) >= iSCSISessionWaitTime {
		Logc(ctx).WithFields(logFields).Warningf("Portal exceeded stale wait time at %v; adding to stale portals list.",
			timeNow)
		return LogoutLoginScan
	} else {
		Logc(ctx).WithFields(logFields).Warningf("Portal has not exceeded stale wait time at %v.", timeNow)
	}

	return NoAction
}

// isNonStalePortal attempts to identify if a session has any issues other than being stale.
// For sessions with no current issues it attempts to identify any strange behavior or state it should
// not be in.
func isNonStalePortal(ctx context.Context, publishedSessionData, currentSessionData *ISCSISessionData,
	portal string,
) ISCSIAction {
	logFields := LogFields{
		"portal":            portal,
		"CHAPInUse":         publishedSessionData.PortalInfo.CHAPInUse(),
		"sessionInfoSource": publishedSessionData.PortalInfo.Source,
	}

	// Identify if LUNs in published sessions are missing from the current sessions
	publishedLUNs := publishedSessionData.LUNs
	if missingLUNs := currentSessionData.LUNs.IdentifyMissingLUNs(publishedLUNs); len(missingLUNs) > 0 {
		Logc(ctx).WithFields(logFields).Warningf("Portal is missing LUN Number(s): %v.", missingLUNs)
		return Scan
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

	return NoAction
}

// SortPortals sorts portals on the basis of their lastAccessTime
func SortPortals(portals []string, publishedSessions *ISCSISessions) {
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

// InitiateScanForAllLUNs scans all paths to each of the LUNs passed.
func InitiateScanForAllLUNs(ctx context.Context, iSCSINodeName string) error {
	fields := LogFields{"iSCSINodeName": iSCSINodeName}

	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.InitiateScanForAllLUNs")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.InitiateScanForAllLUNs")

	// Setting lunID to -1 so that all the LUNs are scanned.
	lunID := -1

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
		Logc(ctx).WithError(err).Error("Could not scan for new LUN.")
	}

	return nil
}
