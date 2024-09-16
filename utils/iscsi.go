// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

const (
	temporaryMountDir          = "/tmp_mnt"
	iSCSIMaxFlushWaitDuration  = 6 * time.Minute
	SessionSourceNodeStage     = "nodeStage"
	SessionSourceTrackingInfo  = "trackingInfo"
	SessionSourceCurrentStatus = "currentStatus"

	unknownFstype = "<unknown>"
)

var (
	IscsiUtils  = iscsi.NewReconcileUtils(chrootPathPrefix, NewOSClient())
	iscsiClient = iscsi.NewDetailed(chrootPathPrefix, command, iscsi.DefaultSelfHealingExclusion, NewOSClient(),
		NewDevicesClient(), NewFilesystemClient(), NewMountClient(), IscsiUtils)

	// Non-persistent map to maintain flush delays/errors if any, for device path(s).
	iSCSIVolumeFlushExceptions = make(map[string]time.Time)

	// perNodeIgroupRegex is used to ensure an igroup meets the following format:
	// <up to and including 59 characters of a container orchestrator node name>-<36 characters of trident version uuid>
	// ex: Kubernetes-NodeA-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z
	perNodeIgroupRegex                = regexp.MustCompile(`^[0-9A-z\-.]{1,59}-[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}`)
	duringRescanOneLunBeforeFileWrite = fiji.Register("duringRescanOneLunBeforeFileWrite", "iscsi")
)

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

	if err = duringRescanOneLunBeforeFileWrite.Inject(); err != nil {
		return err
	}

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

// verifyMultipathDeviceSerial compares the serial number of the DM device with the serial
// of the LUN to ensure correct DM device has been discovered
func verifyMultipathDeviceSerial(
	ctx context.Context, multipathDevice, lunSerial string,
) error {
	if lunSerial == "" {
		// Empty string means don't care
		return nil
	}

	// Multipath UUID contains LUN serial in hex format
	lunSerialHex := hex.EncodeToString([]byte(lunSerial))

	multipathDeviceUUID, err := IscsiUtils.GetMultipathDeviceUUID(multipathDevice)
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
func verifyMultipathDeviceSize(
	ctx context.Context, multipathDevice, device string,
) (int64, bool, error) {
	deviceSize, err := getISCSIDiskSize(ctx, "/dev/"+device)
	if err != nil {
		return 0, false, err
	}

	mpathSize, err := getISCSIDiskSize(ctx, "/dev/"+multipathDevice)
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
				publishInfo := &models.VolumePublishInfo{}
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

// InitiateScanForLun scans all paths on a host to a single LUN.
func InitiateScanForLun(ctx context.Context, lunID int, iSCSINodeName string) error {
	fields := LogFields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.InitiateScanForLun")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.InitiateScanForLun")

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

	return nil
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

// RemoveLUNFromSessions removes portal LUN mappings
func RemoveLUNFromSessions(ctx context.Context, publishInfo *models.VolumePublishInfo, sessions *models.ISCSISessions) {
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
func RemovePortalsFromSession(
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

func PopulateCurrentSessions(ctx context.Context, currentMapping *models.ISCSISessions) error {
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

		var publishInfo models.VolumePublishInfo
		publishInfo.IscsiAccessInfo = models.IscsiAccessInfo{
			IscsiLunNumber:    int32(lunNumber),
			IscsiTargetIQN:    iscsiDevice.IQN,
			IscsiTargetPortal: targetPortal,
		}
		publishInfo.IscsiChapInfo = iscsiDevice.CHAPInfo

		logFields["IscsiPortal"] = targetPortal

		reasonInvalid := models.NotInvalid
		if SliceContainsString(duplicatePortals, targetPortal) {
			Logc(ctx).WithFields(logFields).Warning("Portal value is not unique.")

			reasonInvalid = models.DuplicatePortals // Could be a result of bug in open-iscsi
		} else if iscsiDevice.MultipathDevice == "" {
			Logc(ctx).WithFields(logFields).Warning("Multipath device not found.")

			reasonInvalid = models.MissingMpathDevice // Could be a result of previous invalid multipathing config
		}

		newCtx := context.WithValue(ctx, iscsi.SessionInfoSource, SessionSourceCurrentStatus)
		AddISCSISession(newCtx, currentMapping, &publishInfo, "", sessionNumber, reasonInvalid)
	}

	return nil
}

// InspectAllISCSISessions goes through each iSCSI session in published sessions and creates a list of
// sorted stale iSCSI portals and a sorted list of non-stale iSCSI portals.
// NOTE: Since we do not expect notStalePortals to be very-large (in millions or even in 1000s),
// sorting on the basis of lastAccessTime should not be an expensive operation.
func InspectAllISCSISessions(
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
		if IsISCSISessionStale(ctx, currentPortalInfo.SessionNumber) {
			action := isStalePortal(ctx, publishedPortalInfo, currentPortalInfo, iSCSISessionWaitTime, timeNow, portal)
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
func isStalePortal(
	ctx context.Context, publishedPortalInfo, currentPortalInfo *models.PortalInfo,
	iSCSISessionWaitTime time.Duration, timeNow time.Time, portal string,
) models.ISCSIAction {
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
			return models.LogoutLoginScan
		}
	}

	if !publishedPortalInfo.IsFirstIdentifiedStaleAtSet() {
		Logc(ctx).WithFields(logFields).Warningf("Portal identified to be stale at %v.", timeNow)
		publishedPortalInfo.FirstIdentifiedStaleAt = timeNow
	} else if timeNow.Sub(publishedPortalInfo.FirstIdentifiedStaleAt) >= iSCSISessionWaitTime {
		Logc(ctx).WithFields(logFields).Warningf("Portal exceeded stale wait time at %v; adding to stale portals list.",
			timeNow)
		return models.LogoutLoginScan
	} else {
		Logc(ctx).WithFields(logFields).Warningf("Portal has not exceeded stale wait time at %v.", timeNow)
	}

	return models.NoAction
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

// GetDMDeviceForMapperPath returns the DM device corresponding to mapper device.
// Ex: for "/dev/mapper/3600a09807a415273323f576776372d4d -> ../dm-0" /dev/dm-0 is returned
func GetDMDeviceForMapperPath(ctx context.Context, mapperPath string) (string, error) {
	// Check if the input is a valid mapper device path
	if !strings.HasPrefix(mapperPath, "/dev/mapper/") {
		return "", fmt.Errorf("invalid mapper device path: %s", mapperPath)
	}

	// Use EvalSymlinks to resolve the symlink to its target
	dmDevicePath, err := filepath.EvalSymlinks(mapperPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlink: %s", err)
	}
	Logc(ctx).WithFields(LogFields{
		"multipathDevicePath":    mapperPath,
		"deviceMapperDevicePath": dmDevicePath,
	}).Debug("Discovered device-mapper device path for multipath device path.")

	return dmDevicePath, nil
}

// ------ TODO remove:  temporary functions to bridge the gap while we transition to the new iscsi client ------

func execIscsiadmCommand(ctx context.Context, args ...string) ([]byte, error) {
	return iscsiClient.ExecIscsiadmCommand(ctx, args...)
}

func listAllISCSIDevices(ctx context.Context) {
	iscsi.ListAllDevices(ctx)
}

func getISCSISessionInfo(ctx context.Context) ([]iscsi.SessionInfo, error) {
	return iscsiClient.GetSessionInfo(ctx)
}

func ISCSISupported(ctx context.Context) bool {
	return iscsiClient.Supported(ctx)
}

func iSCSISessionExists(ctx context.Context, portal string) (bool, error) {
	return iscsiClient.SessionExists(ctx, portal)
}

func configureISCSITarget(ctx context.Context, iqn, portal, name, value string) error {
	return iscsiClient.ConfigureTarget(ctx, iqn, portal, name, value)
}

func LoginISCSITarget(ctx context.Context, publishInfo *models.VolumePublishInfo, portal string) error {
	return iscsiClient.LoginTarget(ctx, publishInfo, portal)
}

func iSCSIScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	return iscsiClient.ScanTargetLUN(ctx, lunID, hosts)
}

func IsISCSISessionStale(ctx context.Context, sessionNumber string) bool {
	return iscsiClient.IsSessionStale(ctx, sessionNumber)
}

func AddISCSISession(
	ctx context.Context, sessions *models.ISCSISessions, publishInfo *models.VolumePublishInfo,
	volID, sessionNumber string, reasonInvalid models.PortalInvalid,
) {
	iscsiClient.AddSession(ctx, sessions, publishInfo, volID, sessionNumber, reasonInvalid)
}
