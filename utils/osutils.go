// Copyright 2021 NetApp, Inc. All Rights Reserved.

package utils

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
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
	iSCSIDeviceDiscoveryTimeoutSecs     = 90
	multipathDeviceDiscoveryTimeoutSecs = 90
	resourceDeletionTimeoutSecs         = 40
	fsRaw                               = "raw"
	temporaryMountDir                   = "/tmp_mnt"
	unknownFstype                       = "<unknown>"
)

var xtermControlRegex = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)
var portalPortPattern = regexp.MustCompile(`.+:\d+$`)
var pidRunningOrIdleRegex = regexp.MustCompile(`pid \d+ (running|idle)`)
var pidRegex = regexp.MustCompile(`^\d+$`)
var chrootPathPrefix string

func IPv6Check(ip string) bool {
	return strings.Count(ip, ":") >= 2
}

func init() {
	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		chrootPathPrefix = "/host"
	} else {
		chrootPathPrefix = ""
	}
}

// Attach the volume to the local host.  This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.
func AttachNFSVolume(ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo) error {

	Logc(ctx).Debug(">>>> osutils.AttachNFSVolume")
	defer Logc(ctx).Debug("<<<< osutils.AttachNFSVolume")

	var exportPath = fmt.Sprintf("%s:%s", publishInfo.NfsServerIP, publishInfo.NfsPath)
	var options = publishInfo.MountOptions

	Logc(ctx).WithFields(log.Fields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug("Publishing NFS volume.")

	return mountNFSPath(ctx, exportPath, mountpoint, options)
}

// AttachISCSIVolume attaches the volume to the local host.  This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.  If the mountpoint
// parameter is specified, the volume will be mounted.  The device path is set on the in-out publishInfo parameter
// so that it may be mounted later instead.
func AttachISCSIVolume(ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo) error {

	Logc(ctx).Debug(">>>> osutils.AttachISCSIVolume")
	defer Logc(ctx).Debug("<<<< osutils.AttachISCSIVolume")

	var err error
	var lunID = int(publishInfo.IscsiLunNumber)

	var bkportal []string
	var portalIps []string
	bkportal = append(bkportal, ensureHostportFormatted(publishInfo.IscsiTargetPortal))
	portalIps = append(portalIps, getHostportIP(publishInfo.IscsiTargetPortal))

	for _, p := range publishInfo.IscsiPortals {
		bkportal = append(bkportal, ensureHostportFormatted(p))
		portalIps = append(portalIps, getHostportIP(p))
	}

	var targetIQN = publishInfo.IscsiTargetIQN
	var username = publishInfo.IscsiUsername                  // unidirectional CHAP field
	var initiatorSecret = publishInfo.IscsiInitiatorSecret    // unidirectional CHAP field
	var targetUsername = publishInfo.IscsiTargetUsername      // bidirectional CHAP field
	var targetInitiatorSecret = publishInfo.IscsiTargetSecret // bidirectional CHAP field
	var iscsiInterface = publishInfo.IscsiInterface
	var lunSerial = publishInfo.IscsiLunSerial
	var fstype = publishInfo.FilesystemType
	var options = publishInfo.MountOptions

	if iscsiInterface == "" {
		iscsiInterface = "default"
	}

	Logc(ctx).WithFields(log.Fields{
		"volume":         name,
		"mountpoint":     mountpoint,
		"lunID":          lunID,
		"targetPortals":  bkportal,
		"targetIQN":      targetIQN,
		"iscsiInterface": iscsiInterface,
		"fstype":         fstype,
	}).Debug("Attaching iSCSI volume.")

	if !ISCSISupported(ctx) {
		err := errors.New("unable to attach: open-iscsi tools not found on host")
		Logc(ctx).Errorf("Unable to attach volume: open-iscsi utils not found")
		return err
	}

	// Ensure we are logged into correct portals
	if publishInfo.UseCHAP {
		bkPortalsToLogin, err := portalsToLogin(ctx, targetIQN, bkportal)
		if err != nil {
			return err
		}

		for _, portal := range bkPortalsToLogin {
			err = loginWithChap(
				ctx, targetIQN, portal, username, initiatorSecret, targetUsername,
				targetInitiatorSecret, iscsiInterface)
			if err != nil {
				Logc(ctx).Errorf("Failed to login with CHAP credentials: %+v ", err)
				return fmt.Errorf("iSCSI login error: %v", err)
			}
		}
	} else {
		portalIpsToLogin, err := portalsIpsToLogin(ctx, targetIQN, portalIps)
		if err != nil {
			return err
		}

		err = EnsureISCSISessions(ctx, targetIQN, iscsiInterface, portalIpsToLogin)
		if err != nil {
			return fmt.Errorf("iSCSI session error: %v", err)
		}
	}

	// First attempt to fix invalid serials by rescanning them
	err = handleInvalidSerials(ctx, lunID, targetIQN, lunSerial, rescanOneLun)
	if err != nil {
		return err
	}

	// Then attempt to fix invalid serials by purging them (to be scanned
	// again later)
	err = handleInvalidSerials(ctx, lunID, targetIQN, lunSerial, purgeOneLun)
	if err != nil {
		return err
	}

	// If LUN isn't present, scan the target and wait for the device(s) to appear
	// if not attached need to scan
	shouldScan := !IsAlreadyAttached(ctx, lunID, targetIQN)
	err = waitForDeviceScanIfNeeded(ctx, lunID, targetIQN, shouldScan)
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
	err = handleInvalidSerials(ctx, lunID, targetIQN, lunSerial, failHandler)
	if err != nil {
		return err
	}

	err = waitForMultipathDeviceForLUN(ctx, lunID, targetIQN)
	if err != nil {
		return err
	}

	// Lookup all the SCSI device information, and include filesystem type only if not raw block volume
	needFSType := fstype != fsRaw

	deviceInfo, err := getDeviceInfoForLUN(ctx, lunID, targetIQN, needFSType)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %v", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get iSCSI device information for LUN %d", lunID)
	}

	Logc(ctx).WithFields(log.Fields{
		"scsiLun":         deviceInfo.LUN,
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
		"fsType":          deviceInfo.Filesystem,
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

	// Return the device in the publish info in case the mount will be done later
	publishInfo.DevicePath = devicePath

	if fstype == fsRaw {
		return nil
	}

	existingFstype := deviceInfo.Filesystem
	if existingFstype == "" {
		Logc(ctx).WithFields(log.Fields{"volume": name, "fstype": fstype}).Debug("Formatting LUN.")
		err := formatVolume(ctx, devicePath, fstype)
		if err != nil {
			return fmt.Errorf("error formatting LUN %s, device %s: %v", name, deviceToUse, err)
		}
	} else if existingFstype != unknownFstype && existingFstype != fstype {
		Logc(ctx).WithFields(log.Fields{
			"volume":          name,
			"existingFstype":  existingFstype,
			"requestedFstype": fstype,
		}).Error("LUN already formatted with a different file system type.")
		return fmt.Errorf("LUN %s, device %s already formatted with other filesystem: %s",
			name, deviceToUse, existingFstype)
	} else {
		Logc(ctx).WithFields(log.Fields{
			"volume": name,
			"fstype": deviceInfo.Filesystem,
		}).Debug("LUN already formatted.")
	}

	// Optionally mount the device
	if mountpoint != "" {
		if err := MountDevice(ctx, devicePath, mountpoint, options, false); err != nil {
			return fmt.Errorf("error mounting LUN %v, device %v, mountpoint %v; %s",
				name, deviceToUse, mountpoint, err)
		}
	}

	return nil
}

// DFInfo data structure for wrapping the parsed output from the 'df' command
type DFInfo struct {
	Target string
	Source string
}

// GetDFOutput returns parsed DF output
func GetDFOutput(ctx context.Context) ([]DFInfo, error) {

	Logc(ctx).Debug(">>>> osutils.GetDFOutput")
	defer Logc(ctx).Debug("<<<< osutils.GetDFOutput")

	var result []DFInfo
	out, err := execCommand(ctx, "df", "--output=target,source")
	if err != nil {
		// df returns an error if there's a stale file handle that we can
		// safely ignore. There may be other reasons. Consider it a warning if
		// it printed anything to stdout.
		if len(out) == 0 {
			Logc(ctx).Error("Error encountered gathering df output.")
			return nil, err
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {

		a := strings.Fields(l)
		if len(a) > 1 {
			result = append(result, DFInfo{
				Target: a[0],
				Source: a[1],
			})
		}
	}
	if len(result) > 1 {
		return result[1:], nil
	}
	return result, nil
}

// GetInitiatorIqns returns parsed contents of /etc/iscsi/initiatorname.iscsi
func GetInitiatorIqns(ctx context.Context) ([]string, error) {

	Logc(ctx).Debug(">>>> osutils.GetInitiatorIqns")
	defer Logc(ctx).Debug("<<<< osutils.GetInitiatorIqns")

	iqns := make([]string, 0)

	out, err := execCommand(ctx, "cat", "/etc/iscsi/initiatorname.iscsi")
	if err != nil {
		Logc(ctx).WithField("Error", err).Warn("Could not read initiatorname.iscsi; perhaps iSCSI is not installed?")
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "InitiatorName=") {
			iqns = append(iqns, strings.Split(l, "=")[1])
		}
	}
	return iqns, nil
}

// GetIPAddresses returns the sorted list of Global Unicast IP addresses available to Trident
func GetIPAddresses(ctx context.Context) ([]string, error) {

	Logc(ctx).Debug(">>>> osutils.GetIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils.GetIPAddresses")

	ipAddrs := make([]string, 0)
	addrsMap := make(map[string]struct{})

	// Get the set of potentially viable IP addresses for this host in an OS-appropriate way.
	addrs, err := getIPAddresses(ctx)
	if err != nil {
		err = fmt.Errorf("could not gather system IP addresses; %v", err)
		Logc(ctx).Error(err)
		return nil, err
	}

	// Strip netmask and use a map to ensure addresses are deduplicated.
	for _, addr := range addrs {

		// net.Addr are of form 1.2.3.4/32, but IP needs 1.2.3.4, so we must strip the netmask (also works for IPv6)
		parsedAddr := net.ParseIP(strings.Split(addr.String(), "/")[0])

		Logc(ctx).WithField("IPAddress", parsedAddr.String()).Debug("Discovered potentially viable IP address.")

		addrsMap[parsedAddr.String()] = struct{}{}
	}

	for addr := range addrsMap {
		ipAddrs = append(ipAddrs, addr)
	}
	sort.Strings(ipAddrs)
	return ipAddrs, nil
}

// PathExists returns true if the file/directory at the specified path exists,
// false otherwise or if an error occurs.
func PathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

// EnsureFileExists makes sure that file of given name exists
func EnsureFileExists(ctx context.Context, path string) error {

	fields := log.Fields{"path": path}
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is a directory")
			return fmt.Errorf("path exists but is a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if file exists; %s", err)
		return fmt.Errorf("can't determine if file %s exists; %s", path, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC, 0600)
	if nil != err {
		Logc(ctx).WithFields(fields).Errorf("OpenFile failed; %s", err)
		return fmt.Errorf("failed to create file %s; %s", path, err)
	}
	file.Close()

	return nil
}

// DeleteResourceAtPath makes sure that given named file or (empty) directory is removed
func DeleteResourceAtPath(ctx context.Context, resource string) error {
	return waitForResourceDeletionAtPath(ctx, resource)
}

// waitForResourceDeletionAtPath accepts a resource name and waits until it is deleted and returns error if it times out
func waitForResourceDeletionAtPath(ctx context.Context, resource string) error {

	fields := log.Fields{"resource": resource}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.waitForResourceDeletionAtPath")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.waitForResourceDeletionAtPath")

	maxDuration := resourceDeletionTimeoutSecs * time.Second

	checkResourceDeletion := func() error {
		if _, err := os.Stat(resource); err == nil {
			if err = os.Remove(resource); err != nil {
				Logc(ctx).WithFields(fields).Debugf("Failed to remove resource, %s", err)
				return fmt.Errorf("failed to remove resource %s; %s", resource, err)
			}
			return nil
		} else if !os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).Debugf("Can't determine if resource exists; %s", err)
			return fmt.Errorf("can't determine if resource %s exists; %s", resource, err)
		}

		return nil
	}

	deleteNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Resource not deleted yet, waiting.")
	}

	deleteBackoff := backoff.NewExponentialBackOff()
	deleteBackoff.InitialInterval = 1 * time.Second
	deleteBackoff.Multiplier = 1.414 // approx sqrt(2)
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.MaxElapsedTime = maxDuration

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkResourceDeletion, deleteBackoff, deleteNotify); err != nil {
		return fmt.Errorf("could not delete resource after %3.2f seconds", maxDuration.Seconds())
	} else {
		Logc(ctx).WithField("resource", resource).Debug("Resource deleted.")
		return nil
	}
}

// EnsureDirExists makes sure that given directory structure exists
func EnsureDirExists(ctx context.Context, path string) error {

	fields := log.Fields{
		"path": path,
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is not a directory")
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if directory exists; %s", err)
		return fmt.Errorf("can't determine if directory %s exists; %s", path, err)
	}

	err := os.MkdirAll(path, 0755)
	if err != nil {
		Logc(ctx).WithFields(fields).Errorf("Mkdir failed; %s", err)
		return fmt.Errorf("failed to mkdir %s; %s", path, err)
	}

	return nil
}

// getSysfsBlockDirsForLUN returns the list of directories in sysfs where the block devices should appear
// after the scan is successful. One directory is returned for each path in the host session map.
func getSysfsBlockDirsForLUN(lunID int, hostSessionMap map[int]int) []string {

	paths := make([]string, 0)
	for hostNumber, sessionNumber := range hostSessionMap {
		p := fmt.Sprintf(
			chrootPathPrefix+"/sys/class/scsi_host/host%d/device/session%d/iscsi_session/session%d/device/target%d:0:0/%d:0:0:%d",
			hostNumber, sessionNumber, sessionNumber, hostNumber, hostNumber, lunID)
		paths = append(paths, p)
	}
	return paths
}

// getDevicesForLUN find the /dev/sd* device names for an iSCSI LUN.
func getDevicesForLUN(paths []string) ([]string, error) {

	devices := make([]string, 0)
	for _, p := range paths {
		dirname := p + "/block"
		if !PathExists(dirname) {
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

// waitForDeviceScanIfNeeded scans all paths to a specific LUN and waits until all
// SCSI disk-by-path devices for that LUN are present on the host.
func waitForDeviceScanIfNeeded(ctx context.Context, lunID int, iSCSINodeName string, shouldScan bool) error {

	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.waitForDeviceScanIfNeeded")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.waitForDeviceScanIfNeeded")

	hostSessionMap := GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	Logc(ctx).WithField("hostSessionMap", hostSessionMap).Debug("Built iSCSI host/session map.")
	hosts := make([]int, 0)
	for hostNumber := range hostSessionMap {
		hosts = append(hosts, hostNumber)
	}

	if shouldScan {
		if err := iSCSIScanTargetLUN(ctx, lunID, hosts); err != nil {
			Logc(ctx).WithField("scanError", err).Error("Could not scan for new LUN.")
		}
	}

	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)
	Logc(ctx).Debugf("Scanning paths: %v", paths)
	found := make([]string, 0)

	checkAllDevicesExist := func() error {

		found := make([]string, 0)
		// Check if any paths present, and return nil (success) if so
		for _, p := range paths {
			dirname := p + "/block"
			if !PathExists(dirname) {
				return errors.New("device not present yet")
			}
			found = append(found, dirname)
		}
		return nil
	}

	devicesNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("All devices not yet present, waiting.")
	}

	deviceBackoff := backoff.NewExponentialBackOff()
	deviceBackoff.InitialInterval = 1 * time.Second
	deviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	deviceBackoff.RandomizationFactor = 0.1
	deviceBackoff.MaxElapsedTime = 5 * time.Second

	if err := backoff.RetryNotify(checkAllDevicesExist, deviceBackoff, devicesNotify); err == nil {
		Logc(ctx).Debugf("Paths found: %v", found)
		return nil
	}

	Logc(ctx).Debugf("Paths found so far: %v", found)

	checkAnyDeviceExists := func() error {

		found := make([]string, 0)
		// Check if any paths present, and return nil (success) if so
		for _, p := range paths {
			dirname := p + "/block"
			if PathExists(dirname) {
				found = append(found, dirname)
			}
		}
		if 0 == len(found) {
			return errors.New("no devices present yet")
		}
		return nil
	}

	devicesNotify = func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("No devices present yet, waiting.")
	}

	deviceBackoff = backoff.NewExponentialBackOff()
	deviceBackoff.InitialInterval = 1 * time.Second
	deviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	deviceBackoff.RandomizationFactor = 0.1
	deviceBackoff.MaxElapsedTime = (iSCSIDeviceDiscoveryTimeoutSecs - 5) * time.Second

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(checkAnyDeviceExists, deviceBackoff, devicesNotify); err != nil {
		Logc(ctx).Warnf("Could not find all devices after %d seconds.", iSCSIDeviceDiscoveryTimeoutSecs)

		// In the case of a failure, log info about what devices are present
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
		return err
	}

	Logc(ctx).Debugf("Paths found: %v", found)
	return nil
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
	HostSessionMap  map[int]int
}

// getDeviceInfoForLUN finds iSCSI devices using /dev/disk/by-path values.  This method should be
// called after calling waitForDeviceScanIfNeeded so that the device paths are known to exist.
func getDeviceInfoForLUN(
	ctx context.Context, lunID int, iSCSINodeName string, needFSType bool,
) (*ScsiDeviceInfo, error) {

	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
		"needFSType":    needFSType,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.getDeviceInfoForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.getDeviceInfoForLUN")

	hostSessionMap := GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return nil, fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := getDevicesForLUN(paths)
	if nil != err {
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

	err = ensureDeviceReadableWithRetry(ctx, devicePath)
	if err != nil {
		return nil, err
	}

	fsType := ""
	if needFSType {

		fsType, err = getFSType(ctx, devicePath)
		if err != nil {
			return nil, err
		}
	}

	Logc(ctx).WithFields(log.Fields{
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

	fields := log.Fields{"mountpath": mountpath}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.getDeviceInfoForMountPath")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.getDeviceInfoForMountPath")

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

	Logc(ctx).WithFields(log.Fields{
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
	}).Debug("Found SCSI device.")

	return deviceInfo, nil
}

// waitForMultipathDeviceForLUN
func waitForMultipathDeviceForLUN(ctx context.Context, lunID int, iSCSINodeName string) error {

	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.waitForMultipathDeviceForLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.waitForMultipathDeviceForLUN")

	hostSessionMap := GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := getDevicesForLUN(paths)
	if nil != err {
		return err
	}

	waitForMultipathDeviceForDevices(ctx, devices)
	return nil
}

// waitForMultipathDeviceForDevices accepts a list of sd* device names and waits until
// a multipath device is present for at least one of those.  It returns the name of the
// multipath device, or an empty string if multipathd isn't running or there is only one path.
func waitForMultipathDeviceForDevices(ctx context.Context, devices []string) string {

	fields := log.Fields{"devices": devices}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.waitForMultipathDeviceForDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.waitForMultipathDeviceForDevices")

	if len(devices) <= 1 {
		Logc(ctx).Debugf("Skipping multipath discovery, %d device(s) specified.", len(devices))
		return ""
	} else if !multipathdIsRunning(ctx) {
		Logc(ctx).Debug("Skipping multipath discovery, multipathd isn't running.")
		return ""
	}

	maxDuration := multipathDeviceDiscoveryTimeoutSecs * time.Second
	multipathDevice := ""

	checkMultipathDeviceExists := func() error {

		for _, device := range devices {
			multipathDevice = findMultipathDeviceForDevice(ctx, device)
			if multipathDevice != "" {
				return nil
			}
		}
		if multipathDevice == "" {
			return errors.New("multipath device not yet present")
		}
		return nil
	}

	deviceNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Multipath device not yet present, waiting.")
	}

	multipathDeviceBackoff := backoff.NewExponentialBackOff()
	multipathDeviceBackoff.InitialInterval = 1 * time.Second
	multipathDeviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	multipathDeviceBackoff.RandomizationFactor = 0.1
	multipathDeviceBackoff.MaxElapsedTime = maxDuration

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(checkMultipathDeviceExists, multipathDeviceBackoff, deviceNotify); err != nil {
		Logc(ctx).Warnf("Could not find multipath device after %3.2f seconds.", maxDuration.Seconds())
	} else {
		Logc(ctx).WithField("multipathDevice", multipathDevice).Debug("Multipath device found.")
	}
	return multipathDevice
}

// waitForDevice accepts a device name and waits until it is present and returns error if it times out
func waitForDevice(ctx context.Context, device string) error {

	fields := log.Fields{"device": device}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.waitForDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.waitForDevice")

	maxDuration := multipathDeviceDiscoveryTimeoutSecs * time.Second

	checkDeviceExists := func() error {
		if !PathExists(device) {
			return errors.New("device not yet present")
		}
		return nil
	}

	deviceNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Device not yet present, waiting.")
	}

	deviceBackoff := backoff.NewExponentialBackOff()
	deviceBackoff.InitialInterval = 1 * time.Second
	deviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	deviceBackoff.RandomizationFactor = 0.1
	deviceBackoff.MaxElapsedTime = maxDuration

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkDeviceExists, deviceBackoff, deviceNotify); err != nil {
		return fmt.Errorf("could not find device after %3.2f seconds", maxDuration.Seconds())
	} else {
		Logc(ctx).WithField("device", device).Debug("Device found.")
		return nil
	}
}

// findMultipathDeviceForDevice finds the devicemapper parent of a device name like /dev/sdx.
func findMultipathDeviceForDevice(ctx context.Context, device string) string {

	Logc(ctx).WithField("device", device).Debug(">>>> osutils.findMultipathDeviceForDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< osutils.findMultipathDeviceForDevice")

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

	Logc(ctx).WithField("device", device).Debug(">>>> osutils.findDevicesForMultipathDevice")
	defer Logc(ctx).WithField("device", device).Debug("<<<< osutils.findDevicesForMultipathDevice")

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
		Logc(ctx).WithFields(log.Fields{
			"device":  device,
			"devices": devices,
		}).Debug("Found devices for multipath device.")
	}

	return devices
}

// PrepareDeviceForRemoval informs Linux that a device will be removed.
func PrepareDeviceForRemoval(ctx context.Context, lunID int, iSCSINodeName string, force bool) error {

	fields := log.Fields{
		"lunID":            lunID,
		"iSCSINodeName":    iSCSINodeName,
		"chrootPathPrefix": chrootPathPrefix,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.PrepareDeviceForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.PrepareDeviceForRemoval")

	deviceInfo, err := getDeviceInfoForLUN(ctx, lunID, iSCSINodeName, false)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error": err,
			"lunID": lunID,
		}).Warn("Could not get device info for removal, skipping host removal steps.")
		return err
	}

	return removeSCSIDevice(ctx, deviceInfo, force)
}

// PrepareDeviceAtMountPathForRemoval informs Linux that a device will be removed.
func PrepareDeviceAtMountPathForRemoval(ctx context.Context, mountpoint string, unmount, force bool) error {

	fields := log.Fields{"mountpoint": mountpoint}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.PrepareDeviceAtMountPathForRemoval")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.PrepareDeviceAtMountPathForRemoval")

	deviceInfo, err := getDeviceInfoForMountPath(ctx, mountpoint)
	if err != nil {
		return err
	}

	if unmount {
		if err := Umount(ctx, mountpoint); err != nil {
			return err
		}
	}

	return removeSCSIDevice(ctx, deviceInfo, force)
}

// removeSCSIDevice informs Linux that a device will be removed.  The deviceInfo provided only needs
// the devices and multipathDevice fields set.
// IMPORTANT: The force argument has significant ramifications. Setting force=true will cause
// the function to ignore errors, and try as hard as possible to remove the device, even if
// that results in data loss, data corruption, or putting the system into an invalid state.
// Setting force=false will fail at the first problem encountered, so that callers can be
// assured that a successful return indicates that the device was cleanly removed.
// This is important because while most of the time the top priority is to avoid data
// loss or data corruption, there are times when data loss is unavoidable, or has already
// happened, and in those cases it's better to be able to clean up than to be stuck in an
// endless retry loop.
func removeSCSIDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo, force bool) error {

	listAllISCSIDevices(ctx)

	// Flush multipath device
	err := multipathFlushDevice(ctx, deviceInfo)
	if nil != err && !force {
		return err
	}

	// Flush devices
	err = flushDevice(ctx, deviceInfo, force)
	if nil != err && !force {
		return err
	}

	// Remove device
	err = removeDevice(ctx, deviceInfo, force)
	if nil != err && !force {
		return err
	}

	// Give the host a chance to fully process the removal
	time.Sleep(time.Second)
	listAllISCSIDevices(ctx)

	return nil
}

// ISCSISupported returns true if iscsiadm is installed and in the PATH.
func ISCSISupported(ctx context.Context) bool {

	Logc(ctx).Debug(">>>> osutils.ISCSISupported")
	defer Logc(ctx).Debug("<<<< osutils.ISCSISupported")

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

	Logc(ctx).WithField("portal", portal).Debug(">>>> osutils.iSCSIDiscovery")
	defer Logc(ctx).Debug("<<<< osutils.iSCSIDiscovery")

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

	Logc(ctx).Debug(">>>> osutils.getISCSISessionInfo")
	defer Logc(ctx).Debug("<<<< osutils.getISCSISessionInfo")

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
	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.ISCSILogout")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< osutils.ISCSILogout")

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

// UmountAndRemoveTemporaryMountPoint unmounts and removes the TemporaryMountDir
func UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {

	Logc(ctx).Debug(">>>> osutils.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< osutils.UmountAndRemoveTemporaryMountPoint")

	// Delete the temporary mount point if it exists.
	tmpDir := path.Join(mountPath, temporaryMountDir)
	if _, err := os.Stat(tmpDir); err == nil {
		if err = removeMountPoint(ctx, tmpDir); err != nil {
			return fmt.Errorf("failed to remove directory in staging target path %s; %s", tmpDir, err)
		}
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithField("temporaryMountPoint", tmpDir).Errorf("Can't determine if temporary dir path exists; %s", err)
		return fmt.Errorf("can't determine if temporary dir path %s exists; %s", tmpDir, err)
	}

	return nil
}

// removeMountPoint attempts to unmount and remove the directory of the mountPointPath
func removeMountPoint(ctx context.Context, mountPointPath string) error {

	Logc(ctx).Debug(">>>> osutils.removeMountPoint")
	defer Logc(ctx).Debug("<<<< osutils.removeMountPoint")

	err := Umount(ctx, mountPointPath)
	if err != nil {
		Logc(ctx).WithField("mountPointPath", mountPointPath).Errorf("Umount failed; %s", err)
		return err
	}

	err = os.Remove(mountPointPath)
	if err != nil {
		Logc(ctx).WithField("mountPointPath", mountPointPath).Errorf("Remove dir failed; %s", err)
		return fmt.Errorf("failed to remove dir %s; %s", mountPointPath, err)
	}
	return nil
}

// mountFilesystemForResize expands a filesystem. The xfs_growfs utility requires a mount point to expand the
// filesystem. Determining the size of the filesystem requires that the filesystem be mounted.
func mountFilesystemForResize(
	ctx context.Context, devicePath string, stagedTargetPath string, mountOptions string,
) (string, error) {

	logFields := log.Fields{
		"devicePath":       devicePath,
		"stagedTargetPath": stagedTargetPath,
		"mountOptions":     mountOptions,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.mountAndExpandFilesystem")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< osutils.mountAndExpandFilesystem")

	tmpMountPoint := path.Join(stagedTargetPath, temporaryMountDir)
	err := MountDevice(ctx, devicePath, tmpMountPoint, mountOptions, false)
	if err != nil {
		return "", fmt.Errorf("unable to mount device; %s", err)
	}
	return tmpMountPoint, nil
}

// ExpandISCSIFilesystem will expand the filesystem of an already expanded volume.
func ExpandISCSIFilesystem(
	ctx context.Context, publishInfo *VolumePublishInfo, stagedTargetPath string,
) (int64, error) {

	devicePath := publishInfo.DevicePath
	logFields := log.Fields{
		"devicePath":       devicePath,
		"stagedTargetPath": stagedTargetPath,
		"mountOptions":     publishInfo.MountOptions,
		"filesystemType":   publishInfo.FilesystemType,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.ExpandISCSIFilesystem")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< osutils.ExpandISCSIFilesystem")

	tmpMountPoint, err := mountFilesystemForResize(
		ctx, publishInfo.DevicePath, stagedTargetPath, publishInfo.MountOptions)
	if err != nil {
		return 0, err
	}
	defer removeMountPoint(ctx, tmpMountPoint) //nolint
	// Don't need to verify the filesystem type as the resize utilities will throw an error if the filesystem
	// is not the correct type.
	var size int64
	switch publishInfo.FilesystemType {
	case "xfs":
		size, err = expandFilesystem(ctx, "xfs_growfs", tmpMountPoint, tmpMountPoint)
	case "ext3", "ext4":
		size, err = expandFilesystem(ctx, "resize2fs", devicePath, tmpMountPoint)
	default:
		err = fmt.Errorf("unsupported file system type: %s", publishInfo.FilesystemType)
	}
	if err != nil {
		return 0, err
	}
	return size, nil
}

func expandFilesystem(ctx context.Context, cmd string, cmdArguments string, tmpMountPoint string) (int64, error) {

	logFields := log.Fields{
		"cmd":           cmd,
		"cmdArguments":  cmdArguments,
		"tmpMountPoint": tmpMountPoint,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.expandFilesystem")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< osutils.expandFilesystem")

	preExpandSize, err := getFilesystemSize(ctx, tmpMountPoint)
	if err != nil {
		return 0, err
	}
	_, err = execCommand(ctx, cmd, cmdArguments)
	if err != nil {
		Logc(ctx).Errorf("Expanding filesystem failed; %s", err)
		return 0, err
	}

	postExpandSize, err := getFilesystemSize(ctx, tmpMountPoint)
	if err != nil {
		return 0, err
	}

	if postExpandSize == preExpandSize {
		Logc(ctx).Warnf("Failed to expand filesystem; size=%d", postExpandSize)
	}

	return postExpandSize, nil
}

// iSCSISessionExists checks to see if a session exists to the specified portal.
func iSCSISessionExists(ctx context.Context, portal string) (bool, error) {

	Logc(ctx).Debug(">>>> osutils.iSCSISessionExists")
	defer Logc(ctx).Debug("<<<< osutils.iSCSISessionExists")

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

	Logc(ctx).Debug(">>>> osutils.iSCSISessionExistsToTargetIQN")
	defer Logc(ctx).Debug("<<<< osutils.iSCSISessionExistsToTargetIQN")

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

// portalsToLogin checks to see if session to for all the specified portals exist to the specified
// target. If a session does not exist for a give portal it is added to list of portals that Trident
// needs to login to.
func portalsToLogin(ctx context.Context, targetIQN string, portals []string) ([]string, error) {

	logFields := log.Fields{
		"targetIQN": targetIQN,
		"portals":   portals,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.portalsToLogin")
	defer Logc(ctx).Debug("<<<< osutils.portalsToLogin")

	portalsNotLoggedIn := make([]string, len(portals))
	copy(portalsNotLoggedIn, portals)

	sessionInfo, err := getISCSISessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return portalsNotLoggedIn, err
	}

	for _, e := range sessionInfo {
		if e.TargetName == targetIQN {

			// Portals (portalsNotLoggedIn) may/may not contain anything after ":", so instead of matching complete
			// portal value (with e.Portal), check if e.Portal's IP address matches portal's IP address
			matchFunc := func(main, val string) bool {
				mainIpAddress := getHostportIP(main)
				valIpAddress := getHostportIP(val)

				return mainIpAddress == valIpAddress
			}

			portalsNotLoggedIn = RemoveStringFromSliceConditionally(portalsNotLoggedIn, e.Portal, matchFunc)
		}
	}

	return portalsNotLoggedIn, nil
}

// portalsIpsToLogin checks to see if session to for all the specified portal IPs exist to the specified
// target. If a session does not exist for a give portal IP it is added to list of portals IPs that Trident
// needs to login to.
func portalsIpsToLogin(ctx context.Context, targetIQN string, portalsIps []string) ([]string, error) {

	logFields := log.Fields{
		"targetIQN":  targetIQN,
		"portalsIps": portalsIps,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.portalsIpsToLogin")
	defer Logc(ctx).Debug("<<<< osutils.portalsIpsToLogin")

	portalIpsNotLoggedIn := make([]string, len(portalsIps))
	copy(portalIpsNotLoggedIn, portalsIps)

	sessionInfo, err := getISCSISessionInfo(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Problem checking iSCSI sessions.")
		return portalIpsNotLoggedIn, err
	}

	for _, e := range sessionInfo {
		if e.TargetName == targetIQN {
			portalIpsNotLoggedIn = RemoveStringFromSlice(portalIpsNotLoggedIn, e.PortalIP)
		}
	}

	return portalIpsNotLoggedIn, nil
}

// getHostportIP returns just the IP address part of the given input IP address and strips any port information
func getHostportIP(hostport string) string {
	ipAddress := ""
	if IPv6Check(hostport) {
		// this is an IPv6 address, remove port value and add square brackets around the IP address
		if hostport[0] == '[' {
			ipAddress = strings.Split(hostport, "]")[0] + "]"
		} else {
			// assumption here is that without the square brackets its only IP address without port information
			ipAddress = "[" + hostport + "]"
		}
	} else {
		ipAddress = strings.Split(hostport, ":")[0]
	}

	return ipAddress
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

// formatPortal returns the iSCSI portal string, appending a port number if one isn't
// already present, and also appending a target portal group tag if one is not present
func formatPortal(portal string) string {
	if portalPortPattern.MatchString(portal) {
		return portal
	} else {
		return portal + ":3260"
	}
}

func ISCSIRescanDevices(ctx context.Context, targetIQN string, lunID int32, minSize int64) error {

	fields := log.Fields{"targetIQN": targetIQN, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.ISCSIRescanDevices")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.ISCSIRescanDevices")

	deviceInfo, err := getDeviceInfoForLUN(ctx, int(lunID), targetIQN, false)
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

		fields = log.Fields{"size": size, "minSize": minSize}
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

	fields := log.Fields{"multipathDevice": multipathDevice}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.reloadMultipathDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.reloadMultipathDevice")

	if multipathDevice == "" {
		return fmt.Errorf("cannot reload an empty multipathDevice")
	}

	_, err := execCommandWithTimeout(ctx, "multipath", 30, true, "-r", "/dev/"+multipathDevice)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
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

	fields := log.Fields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.iSCSIRescanDisk")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.iSCSIRescanDisk")

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
		Logc(ctx).WithFields(log.Fields{
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

// iSCSIScanTargetLUN scans a single LUN on an iSCSI target to discover it.
func iSCSIScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {

	fields := log.Fields{"hosts": hosts, "lunID": lunID}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.iSCSIScanTargetLUN")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.iSCSIScanTargetLUN")

	var (
		f   *os.File
		err error
	)

	listAllISCSIDevices(ctx)
	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0200); err != nil {
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

	hostSessionMap := GetISCSIHostSessionMapForTarget(ctx, targetIqn)
	if len(hostSessionMap) == 0 {
		return false
	}

	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := getDevicesForLUN(paths)
	if nil != err {
		return false
	}

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

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0200)
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

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0200)
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
func handleInvalidSerials(ctx context.Context, lunID int, targetIqn, expectedSerial string,
	handler func(ctx context.Context, path string) error,
) error {
	if "" == expectedSerial {
		// Empty string means don't care
		return nil
	}

	hostSessionMap := GetISCSIHostSessionMapForTarget(ctx, targetIqn)
	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)
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
func GetISCSIHostSessionMapForTarget(ctx context.Context, iSCSINodeName string) map[int]int {

	fields := log.Fields{"iSCSINodeName": iSCSINodeName}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.GetISCSIHostSessionMapForTarget")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.GetISCSIHostSessionMapForTarget")

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
					} else if sessionNumber, err = strconv.Atoi(strings.TrimPrefix(sessionName, "session")); err != nil {
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

// GetISCSIDevices returns a list of iSCSI devices that are attached to (but not necessarily mounted on) this host.
func GetISCSIDevices(ctx context.Context) ([]*ScsiDeviceInfo, error) {

	Logc(ctx).Debug(">>>> osutils.GetISCSIDevices")
	defer Logc(ctx).Debug("<<<< osutils.GetISCSIDevices")

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

		sessionName := sessionDir.Name()
		if !strings.HasPrefix(sessionName, "session") {
			continue
		} else if _, err = strconv.Atoi(strings.TrimPrefix(sessionName, "session")); err != nil {
			Logc(ctx).WithField("session", sessionName).Error("Could not parse session number")
			return nil, err
		}

		// Find the target IQN from the session at /sys/class/iscsi_session/sessionXXX/targetname
		sessionPath := sysPath + sessionName
		targetNamePath := sessionPath + "/targetname"
		targetNameBytes, err := ioutil.ReadFile(targetNamePath)
		if err != nil {
			Logc(ctx).WithFields(log.Fields{
				"path":  targetNamePath,
				"error": err,
			}).Error("Could not read targetname file")
			return nil, err
		}

		targetIQN := strings.TrimSpace(string(targetNameBytes))

		Logc(ctx).WithFields(log.Fields{
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

		Logc(ctx).WithFields(log.Fields{
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

			Logc(ctx).WithFields(log.Fields{
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
					hostSessionMap = GetISCSIHostSessionMapForTarget(ctx, targetIQN)
					hostSessionMapCache[targetIQN] = hostSessionMap
				}

				Logc(ctx).WithFields(log.Fields{
					"host":            hostNum,
					"lun":             lunNum,
					"devices":         slaveDevices,
					"multipathDevice": multipathDevice,
					"iqn":             targetIQN,
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
					HostSessionMap:  hostSessionMap,
				}

				devices = append(devices, device)
			}
		}
	}

	return devices, nil
}

// IsMounted verifies if the supplied device is attached at the supplied location.
func IsMounted(ctx context.Context, sourceDevice, mountpoint string) (bool, error) {

	fields := log.Fields{
		"source": sourceDevice,
		"target": mountpoint,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.IsMounted")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.IsMounted")

	procSelfMountinfo, err := listProcSelfMountinfo(procSelfMountinfoPath)

	if err != nil {
		Logc(ctx).WithFields(fields).Errorf("checking mounted failed; %s", err)
		return false, fmt.Errorf("checking mounted failed; %s", err)
	}

	var sourceDeviceName string
	if sourceDevice != "" && strings.HasPrefix(sourceDevice, "/dev/") {
		sourceDeviceName = strings.TrimPrefix(sourceDevice, "/dev/")
	}

	for _, procMount := range procSelfMountinfo {

		if !strings.Contains(procMount.MountPoint, mountpoint) {
			continue
		}

		Logc(ctx).Debugf("Mountpoint found: %v", procMount)

		if sourceDevice == "" {
			Logc(ctx).Debugf("Source device: none, Target: %s, is mounted: true", mountpoint)
			return true, nil
		}

		hasDevMountSourcePrefix := strings.HasPrefix(procMount.MountSource, "/dev/")

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

		if sourceDeviceName == mountedDevice {
			Logc(ctx).Debugf("Source device: %s, Target: %s, is mounted: true", sourceDeviceName, mountpoint)
			return true, nil
		}
	}

	Logc(ctx).Debugf("Source device: %s, Target: %s, is mounted: false", sourceDevice, mountpoint)
	return false, nil
}

// GetMountInfo returns the list of mounts found in /proc/self/mountinfo
func GetMountInfo(ctx context.Context) ([]MountInfo, error) {
	Logc(ctx).Debug(">>>> osutils.GetMountInfo")
	defer Logc(ctx).Debug("<<<< osutils.GetMountInfo")

	return listProcSelfMountinfo(procSelfMountinfoPath)
}

// GetMountedISCSIDevices returns a list of iSCSI devices that are *mounted* on this host.
func GetMountedISCSIDevices(ctx context.Context) ([]*ScsiDeviceInfo, error) {

	Logc(ctx).Debug(">>>> osutils.GetMountedISCSIDevices")
	defer Logc(ctx).Debug("<<<< osutils.GetMountedISCSIDevices")

	procSelfMountinfo, err := listProcSelfMountinfo(procSelfMountinfoPath)
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
	iscsiDevices, err := GetISCSIDevices(ctx)
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
		Logc(ctx).WithFields(log.Fields{
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

// multipathFlushDevice invokes the 'multipath' commands to flush paths for a single device.
func multipathFlushDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo) error {

	Logc(ctx).WithField("device", deviceInfo.MultipathDevice).Debug(">>>> osutils.multipathFlushDevice")
	defer Logc(ctx).Debug("<<<< osutils.multipathFlushDevice")

	if deviceInfo.MultipathDevice == "" {
		return nil
	}

	err := flushOneDevice(ctx, "/dev/"+deviceInfo.MultipathDevice)
	if err != nil {
		return err
	}

	_, err = execCommandWithTimeout(ctx, "multipath", 30, true, "-f", "/dev/"+deviceInfo.MultipathDevice)
	if err != nil {
		// nothing to do if it generates an error but log it
		Logc(ctx).WithFields(log.Fields{
			"device": deviceInfo.MultipathDevice,
			"error":  err,
		}).Error("Error encountered in multipath flush device command.")
		return err
	}

	return nil
}

// flushDevice flushes any outstanding I/O to all paths to a device.
func flushDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo, force bool) error {

	Logc(ctx).Debug(">>>> osutils.flushDevice")
	defer Logc(ctx).Debug("<<<< osutils.flushDevice")

	for _, device := range deviceInfo.Devices {
		err := flushOneDevice(ctx, "/dev/"+device)
		if err != nil && !force {
			return err
		}
	}

	return nil
}

// removeDevice tells Linux that a device will be removed.
func removeDevice(ctx context.Context, deviceInfo *ScsiDeviceInfo, force bool) error {

	Logc(ctx).Debug(">>>> osutils.removeDevice")
	defer Logc(ctx).Debug("<<<< osutils.removeDevice")

	var (
		f   *os.File
		err error
	)

	listAllISCSIDevices(ctx)
	for _, deviceName := range deviceInfo.Devices {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/block/%s/device/delete", deviceName)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0200); err != nil {
			Logc(ctx).WithField("file", filename).Warning("Could not open file for writing.")
			if force {
				continue
			}
			return err
		}

		if written, err := f.WriteString("1"); err != nil {
			Logc(ctx).WithFields(log.Fields{"file": filename, "error": err}).Warning("Could not write to file.")
			f.Close()
			if force {
				continue
			}
			return err
		} else if written == 0 {
			Logc(ctx).WithField("file", filename).Warning("No data written to file.")
			f.Close()
			if force {
				continue
			}
			return errors.New("too few bytes written to sysfs file")
		}

		f.Close()

		listAllISCSIDevices(ctx)
		Logc(ctx).WithField("scanFile", filename).Debug("Invoked device delete.")
	}

	return nil
}

// multipathdIsRunning returns true if the multipath daemon is running.
func multipathdIsRunning(ctx context.Context) bool {

	Logc(ctx).Debug(">>>> osutils.multipathdIsRunning")
	defer Logc(ctx).Debug("<<<< osutils.multipathdIsRunning")

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

// getFSType returns the filesystem for the supplied device.
func getFSType(ctx context.Context, device string) (string, error) {

	Logc(ctx).WithField("device", device).Debug(">>>> osutils.getFSType")
	defer Logc(ctx).Debug("<<<< osutils.getFSType")

	// blkid return status=2 both in case of an unformatted filesystem as well as for the case when it is
	// unable to get the filesystem (e.g. IO error), therefore ensure device is available before calling blkid
	if err := waitForDevice(ctx, device); err != nil {
		return "", fmt.Errorf("could not find device before checking for the filesystem %v; %s", device, err)
	}

	out, err := execCommandWithTimeout(ctx, "blkid", 5, true, device)
	if err != nil {
		if IsTimeoutError(err) {
			listAllISCSIDevices(ctx)
			return "", err
		} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			// EITHER: Disk device is unformatted.
			// OR: For 'blkid', if the specified token (TYPE/PTTYPE, etc) was
			// not found, or no (specified) devices could be identified, an
			// exit code of 2 is returned.

			Logc(ctx).WithField("device", device).Infof("Could not get FSType for device; err: %v", err)

			if unformatted, err := isDeviceUnformatted(ctx, device); err != nil {
				Logc(ctx).WithField("device",
					device).Errorf("Unable to identify if the device is unformatted; err: %v", err)
				return "", err
			} else if !unformatted {
				Logc(ctx).WithField("device", device).Errorf("Device is not unformatted; err: %v", err)
				return "", fmt.Errorf("device %v is not unformatted", device)
			}

			return "", nil
		}

		Logc(ctx).WithField("device", device).Errorf("Could not determine FSType for device; err: %v", err)
		return "", err
	}

	var fsType string

	if strings.Contains(string(out), "TYPE=") {
		for _, v := range strings.Split(string(out), " ") {
			if strings.Contains(v, "TYPE=") {
				fsType = strings.Split(v, "=")[1]
				fsType = strings.Replace(fsType, "\"", "", -1)
				fsType = strings.TrimSpace(fsType)
			}
		}
	}

	if fsType == "" {
		Logc(ctx).WithField("out", string(out)).Errorf("Unable to identify fsType.")

		//  Read the device to see if it is in fact formatted
		if unformatted, err := isDeviceUnformatted(ctx, device); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"device": device,
				"err":    err,
			}).Debugf("Unable to identify if the device is not unformatted.")
		} else if !unformatted {
			Logc(ctx).WithField("device", device).Debugf("Device is not unformatted.")
			return unknownFstype, nil
		} else {
			// If we are here blkid should have not retured exit status 0, we need to retry.
			Logc(ctx).WithField("device", device).Errorf("Device is unformatted.")
		}

		return "", fmt.Errorf("unable to identify fsType")
	}

	return fsType, nil
}

// ensureDeviceReadableWithRetry reads first 4 KiBs of the device to ensures it is readable and retries on errors
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

	Logc(ctx).WithField("device", device).Debug(">>>> osutils.ensureDeviceReadable")
	defer Logc(ctx).Debug("<<<< osutils.ensureDeviceReadable")

	args := []string{"if=" + device, "bs=4096", "count=1", "status=none"}
	out, err := execCommandWithTimeout(ctx, "dd", 5, false, args...)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{"error": err, "device": device}).Error("failed to read the device")
		return err
	}

	// Ensure 4KiB of data read
	if len(out) != 4096 {
		Logc(ctx).WithFields(log.Fields{"error": err, "device": device}).Error("read number of bytes not 4KiB")
		return fmt.Errorf("did not read 4KiB bytes from the device %v, instead read %d bytes", device, len(out))
	}

	return nil
}

// isDeviceUnformatted reads first 2 MiBs of the device to identify if it is unformatted and contains all zeros
func isDeviceUnformatted(ctx context.Context, device string) (bool, error) {

	Logc(ctx).WithField("device", device).Debug(">>>> osutils.isDeviceUnformatted")
	defer Logc(ctx).Debug("<<<< osutils.isDeviceUnformatted")

	args := []string{"if=" + device, "bs=4096", "count=512", "status=none"}
	out, err := execCommandWithTimeout(ctx, "dd", 5, false, args...)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{"error": err, "device": device}).Error("failed to read the device")
		return false, err
	}

	// Ensure 2MiB of data read
	if len(out) != 2097152 {
		Logc(ctx).WithFields(log.Fields{"error": err, "device": device}).Error("read number of bytes not 2MiB")
		return false, fmt.Errorf("did not read 2MiB bytes from the device %v, instead read %d bytes; unable to "+
			"ensure if the device is actually unformatted", device, len(out))
	}

	Logc(ctx).WithField("device", device).Debug("Verified correct number of bytes read.")

	// Ensure all zeros
	if outWithoutZeros := bytes.Trim(out, "\x00"); len(outWithoutZeros) != 0 {
		Logc(ctx).WithFields(log.Fields{"error": err, "device": device}).Error("device contains non-zero values")
		return false, nil
	}

	Logc(ctx).WithFields(log.Fields{"device": device}).Info("Device in unformatted.")

	return true, nil
}

// formatVolume creates a filesystem for the supplied device of the supplied type.
func formatVolume(ctx context.Context, device, fstype string) error {

	logFields := log.Fields{"device": device, "fsType": fstype}
	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.formatVolume")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< osutils.formatVolume")

	maxDuration := 30 * time.Second

	formatVolume := func() error {

		var err error

		switch fstype {
		case "xfs":
			_, err = execCommand(ctx, "mkfs.xfs", "-f", device)
		case "ext3":
			_, err = execCommand(ctx, "mkfs.ext3", "-F", device)
		case "ext4":
			_, err = execCommand(ctx, "mkfs.ext4", "-F", device)
		default:
			return fmt.Errorf("unsupported file system type: %s", fstype)
		}

		return err
	}

	formatNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Format failed, retrying.")
	}

	formatBackoff := backoff.NewExponentialBackOff()
	formatBackoff.InitialInterval = 2 * time.Second
	formatBackoff.Multiplier = 2
	formatBackoff.RandomizationFactor = 0.1
	formatBackoff.MaxElapsedTime = maxDuration

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(formatVolume, formatBackoff, formatNotify); err != nil {
		Logc(ctx).Warnf("Could not format device after %3.2f seconds.", maxDuration.Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Device formatted.")
	return nil
}

// MountDevice attaches the supplied device at the supplied location.  Use this for iSCSI devices.
func MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error) {

	Logc(ctx).WithFields(log.Fields{
		"device":     device,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> osutils.MountDevice")
	defer Logc(ctx).Debug("<<<< osutils.MountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), device, mountpoint}
	} else {
		args = []string{device, mountpoint}
	}

	mounted, _ := IsMounted(ctx, device, mountpoint)
	exists := PathExists(mountpoint)

	Logc(ctx).Debugf("Already mounted: %v, mountpoint exists: %v", mounted, exists)

	if !exists {
		if isMountPointFile {
			if err = EnsureFileExists(ctx, mountpoint); err != nil {
				Logc(ctx).WithField("error", err).Warning("File check failed.")
			}
		} else {
			if err = EnsureDirExists(ctx, mountpoint); err != nil {
				Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
			}
		}
	}

	if !mounted {
		if _, err = execCommand(ctx, "mount", args...); err != nil {
			Logc(ctx).WithField("error", err).Error("Mount failed.")
		}
	}

	return
}

// mountNFSPath attaches the supplied NFS share at the supplied location with options.
func mountNFSPath(ctx context.Context, exportPath, mountpoint, options string) (err error) {

	Logc(ctx).WithFields(log.Fields{
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> osutils.mountNFSPath")
	defer Logc(ctx).Debug("<<<< osutils.mountNFSPath")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-t", "nfs", "-o", strings.TrimPrefix(options, "-o "), exportPath, mountpoint}
	} else {
		args = []string{"-t", "nfs", exportPath, mountpoint}
	}

	// Create the mount point dir if necessary
	if _, err = execCommand(ctx, "mkdir", "-p", mountpoint); err != nil {
		Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
	}

	if out, err := execCommand(ctx, "mount", args...); err != nil {
		Logc(ctx).WithField("output", string(out)).Debug("Mount failed.")
		return fmt.Errorf("error mounting NFS volume %v on mountpoint %v: %v", exportPath, mountpoint, err)
	}

	return nil
}

// Umount detaches from the supplied location.
func Umount(ctx context.Context, mountpoint string) (err error) {

	Logc(ctx).WithField("mountpoint", mountpoint).Debug(">>>> osutils.Umount")
	defer Logc(ctx).Debug("<<<< osutils.Umount")

	if _, err = execCommandWithTimeout(ctx, "umount", 10, true, mountpoint); err != nil {
		Logc(ctx).WithField("error", err).Error("Umount failed.")
		if IsTimeoutError(err) {
			var out []byte
			out, err = execCommandWithTimeout(ctx, "umount", 10, true, mountpoint, "-f")
			if strings.Contains(string(out), "not mounted") {
				err = nil
			}
		}
	}
	return
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
	}).Debug(">>>> osutils.getTargets")
	defer Logc(ctx).Debug("<<<< osutils.getTargets")

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

func updateDiscoveryDb(ctx context.Context, tp, iface, key, value string) error {
	Logc(ctx).WithFields(log.Fields{
		"Key":       key,
		"Value":     value,
		"Portal":    tp,
		"Interface": iface,
	}).Debug(">>>> osutils.updateDiscoveryDb")
	defer Logc(ctx).Debug("<<<< osutils.updateDiscoveryDb")

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
func ensureIscsiTarget(ctx context.Context, tp, targetIqn, username, password, targetUsername, targetInitiatorSecret, iface string) error {
	Logc(ctx).WithFields(log.Fields{
		"IQN":       targetIqn,
		"Portal":    tp,
		"Interface": iface,
	}).Debug(">>>> osutils.ensureIscsiTarget")
	defer Logc(ctx).Debug("<<<< osutils.ensureIscsiTarget")

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

// loginISCSITarget logs in to an iSCSI target.
func configureISCSITarget(ctx context.Context, iqn, portal, name, value string) error {

	Logc(ctx).WithFields(log.Fields{
		"IQN":    iqn,
		"Portal": portal,
		"Name":   name,
		"Value":  value,
	}).Debug(">>>> osutils.configureISCSITarget")
	defer Logc(ctx).Debug("<<<< osutils.configureISCSITarget")

	args := []string{"-m", "node", "-T", iqn, "-p", formatPortal(portal), "-o", "update", "-n", name, "-v", value}
	if _, err := execIscsiadmCommand(ctx, args...); err != nil {
		Logc(ctx).WithField("error", err).Warn("Error configuring iSCSI target.")
		return err
	}
	return nil
}

// loginISCSITarget logs in to an iSCSI target.
func loginISCSITarget(ctx context.Context, iqn, portal string) error {

	Logc(ctx).WithFields(log.Fields{
		"IQN":    iqn,
		"Portal": portal,
	}).Debug(">>>> osutils.loginISCSITarget")
	defer Logc(ctx).Debug("<<<< osutils.loginISCSITarget")

	args := []string{"-m", "node", "-T", iqn, "-l", "-p", formatPortal(portal)}
	listAllISCSIDevices(ctx)
	if _, err := execIscsiadmCommand(ctx, args...); err != nil {
		Logc(ctx).WithField("error", err).Error("Error logging in to iSCSI target.")
		return err
	}
	listAllISCSIDevices(ctx)
	return nil
}

// loginWithChap will login to the iSCSI target with the supplied credentials.
func loginWithChap(
	ctx context.Context, tiqn, portal, username, password, targetUsername, targetInitiatorSecret, iface string) error {

	logFields := log.Fields{
		"IQN":                   tiqn,
		"portal":                portal,
		"username":              username,
		"password":              "****",
		"targetUsername":        targetUsername,
		"targetInitiatorSecret": "****",
		"iface":                 iface,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.loginWithChap")
	defer Logc(ctx).Debug("<<<< osutils.loginWithChap")

	args := []string{"-m", "node", "-T", tiqn, "-p", formatPortal(portal)}

	listAllISCSIDevices(ctx)
	if err := ensureIscsiTarget(ctx, formatPortal(portal), tiqn, username, password, targetUsername, targetInitiatorSecret, iface); err != nil {
		Logc(ctx).Error("Error running iscsiadm node create.")
		return err
	}

	authMethodArgs := append(args, []string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
	if _, err := execIscsiadmCommand(ctx, authMethodArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm set authmethod.")
		return err
	}

	authUserArgs := append(args, []string{"--op=update", "--name", "node.session.auth.username", "--value=" + username}...)
	if _, err := execIscsiadmCommand(ctx, authUserArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm set authuser.")
		return err
	}

	authPasswordArgs := append(args, []string{"--op=update", "--name", "node.session.auth.password", "--value=" + password}...)
	if _, err := execIscsiadmCommand(ctx, authPasswordArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm set authpassword.")
		return err
	}

	if targetUsername != "" && targetInitiatorSecret != "" {
		targetAuthUserArgs := append(args, []string{"--op=update", "--name", "node.session.auth.username_in", "--value=" + targetUsername}...)
		if _, err := execIscsiadmCommand(ctx, targetAuthUserArgs...); err != nil {
			Logc(ctx).Error("Error running iscsiadm set authuser_in.")
			return err
		}

		targetAuthPasswordArgs := append(args, []string{"--op=update", "--name", "node.session.auth.password_in", "--value=" + targetInitiatorSecret}...)
		if _, err := execIscsiadmCommand(ctx, targetAuthPasswordArgs...); err != nil {
			Logc(ctx).Error("Error running iscsiadm set authpassword_in.")
			return err
		}
	}

	loginArgs := append(args, []string{"--login"}...)
	if _, err := execIscsiadmCommand(ctx, loginArgs...); err != nil {
		Logc(ctx).Error("Error running iscsiadm login.")
		return err
	}
	listAllISCSIDevices(ctx)
	return nil
}

func EnsureISCSISessions(ctx context.Context, targetIQN, iface string, portalsIps []string) error {

	logFields := log.Fields{
		"targetIQN":  targetIQN,
		"portalsIps": portalsIps,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> osutils.EnsureISCSISessions")
	defer Logc(ctx).Debug("<<<< osutils.EnsureISCSISessions")

	for _, portalIp := range portalsIps {
		listAllISCSIDevices(ctx)

		if err := ensureIscsiTarget(ctx, formatPortal(portalIp), targetIQN, "", "", "", "", iface); nil != err {
			// Logged
			return err
		}

		// Set scanning to manual
		// Swallow this error, someone is running an old version of Debian/Ubuntu
		_ = configureISCSITarget(ctx, targetIQN, portalIp, "node.session.scan", "manual")

		// Update replacement timeout
		if err := configureISCSITarget(
			ctx, targetIQN, portalIp, "node.session.timeo.replacement_timeout", "5"); err != nil {
			return fmt.Errorf("set replacement timeout failed: %v", err)
		}

		// Log in to target
		if err := loginISCSITarget(ctx, targetIQN, portalIp); err != nil {
			return fmt.Errorf("login to iSCSI target failed: %v", err)
		}
	}

	for _, portalIp := range portalsIps {
		// Recheck to ensure a session is now open
		sessionExists, err := iSCSISessionExists(ctx, portalIp)
		if err != nil {
			return fmt.Errorf("could not recheck for iSCSI session: %v", err)
		}
		if !sessionExists {
			return fmt.Errorf("expected iSCSI session %v NOT found, please login to the iSCSI portal", portalIp)
		}

		Logc(ctx).WithField("portalIp", portalIp).Debug("Session established with iSCSI portal.")
	}

	return nil
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

	Logc(ctx).WithField("hostDataIP", hostDataIP).Debug(">>>> osutils.EnsureISCSISessionWithPortalDiscovery")
	defer Logc(ctx).Debug("<<<< osutils.EnsureISCSISessionWithPortalDiscovery")

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
				err = loginISCSITarget(ctx, target.TargetName, target.PortalIP)
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

// execIscsiadmCommand uses the 'iscsiadm' command to perform operations
func execIscsiadmCommand(ctx context.Context, args ...string) ([]byte, error) {
	return execCommand(ctx, "iscsiadm", args...)
}

// execCommand invokes an external process
func execCommand(ctx context.Context, name string, args ...string) ([]byte, error) {

	Logc(ctx).WithFields(log.Fields{
		"command": name,
		"args":    args,
	}).Debug(">>>> osutils.execCommand.")

	out, err := exec.Command(name, args...).CombinedOutput()

	Logc(ctx).WithFields(log.Fields{
		"command": name,
		"output":  sanitizeString(string(out)),
		"error":   err,
	}).Debug("<<<< osutils.execCommand.")

	return out, err
}

// execCommandResult is used to return shell command results via channels between goroutines
type execCommandResult struct {
	Output []byte
	Error  error
}

// execCommand invokes an external shell command
func execCommandWithTimeout(
	ctx context.Context, name string, timeoutSeconds time.Duration, logOutput bool, args ...string,
) ([]byte, error) {

	timeout := timeoutSeconds * time.Second

	Logc(ctx).WithFields(log.Fields{
		"command":        name,
		"timeoutSeconds": timeout,
		"args":           args,
	}).Debug(">>>> osutils.execCommandWithTimeout.")

	cmd := exec.Command(name, args...)
	done := make(chan execCommandResult, 1)
	var result execCommandResult

	go func() {
		out, err := cmd.CombinedOutput()
		done <- execCommandResult{Output: out, Error: err}
	}()

	select {
	case <-time.After(timeout):
		if err := cmd.Process.Kill(); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"process": name,
				"error":   err,
			}).Error("failed to kill process")
			result = execCommandResult{Output: nil, Error: err}
		} else {
			Logc(ctx).WithFields(log.Fields{
				"process": name,
			}).Error("process killed after timeout")
			result = execCommandResult{Output: nil, Error: TimeoutError("process killed after timeout")}
		}
	case result = <-done:
		break
	}

	logFields := Logc(ctx).WithFields(log.Fields{
		"command": name,
		"error":   result.Error,
	})

	if logOutput {
		logFields.WithFields(log.Fields{
			"output": sanitizeString(string(result.Output)),
		})
	}

	logFields.Debug("<<<< osutils.execCommandWithTimeout.")

	return result.Output, result.Error
}

func sanitizeString(s string) string {
	// Strip xterm color & movement characters
	s = xtermControlRegex.ReplaceAllString(s, "")
	// Strip trailing newline
	s = strings.TrimSuffix(s, "\n")
	return s
}

// SafeToLogOut looks for remaining block devices on a given iSCSI host, and returns
// true if there are none, indicating that logging out would be safe.
func SafeToLogOut(ctx context.Context, hostNumber, sessionNumber int) bool {

	Logc(ctx).Debug(">>>> osutils.SafeToLogOut")
	defer Logc(ctx).Debug("<<<< osutils.SafeToLogOut")

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

// In the case of a iscsi trace debug, log info about session and what devices are present
func listAllISCSIDevices(ctx context.Context) {

	if !Logc(ctx).Logger.IsLevelEnabled(log.TraceLevel) {
		// Don't even run the commands if trace logging is not enabled
		return
	}

	Logc(ctx).Trace(">>>> osutils.listAllISCSIDevices")
	defer Logc(ctx).Trace("<<<< osutils.listAllISCSIDevices")
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
	out1, _ := execCommandWithTimeout(ctx, "multipath", 5, true, "-ll")
	out2, _ := execIscsiadmCommand(ctx, "-m", "session")
	Logc(ctx).WithFields(log.Fields{
		"/dev/dm-*":                  dmLog,
		"/dev/sd*":                   sdLog,
		"/sys/block/*":               sysLog,
		"multipath -ll output":       string(out1),
		"iscsiadm -m session output": string(out2),
	}).Trace("Listing all iSCSI Devices.")
}
