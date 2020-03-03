// Copyright 2020 NetApp, Inc. All Rights Reserved.

package utils

import (
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
)

const (
	iSCSIErrNoObjsFound                 = 21
	iSCSIDeviceDiscoveryTimeoutSecs     = 90
	multipathDeviceDiscoveryTimeoutSecs = 90
	resourceDeletionTimeoutSecs         = 40
	fsRaw                               = "raw"
	temporaryMountDir                   = "/tmp_mnt"
)

var xtermControlRegex = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)
var pidRunningRegex = regexp.MustCompile(`pid \d+ running`)
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
func AttachNFSVolume(name, mountpoint string, publishInfo *VolumePublishInfo) error {

	log.Debug(">>>> osutils.AttachNFSVolume")
	defer log.Debug("<<<< osutils.AttachNFSVolume")

	var exportPath = fmt.Sprintf("%s:%s", publishInfo.NfsServerIP, publishInfo.NfsPath)
	var options = publishInfo.MountOptions

	log.WithFields(log.Fields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug("Publishing NFS volume.")

	return mountNFSPath(exportPath, mountpoint, options)
}

// Attach the volume to the local host.  This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.  If the mountpoint
// parameter is specified, the volume will be mounted.  The device path is set on the in-out publishInfo parameter
// so that it may be mounted later instead.
func AttachISCSIVolume(name, mountpoint string, publishInfo *VolumePublishInfo) error {

	log.Debug(">>>> osutils.AttachISCSIVolume")
	defer log.Debug("<<<< osutils.AttachISCSIVolume")

	var err error
	var lunID = int(publishInfo.IscsiLunNumber)

	var bkportal []string
	var portalIps []string
	bkportal = append(bkportal, publishInfo.IscsiTargetPortal)

	if IPv6Check(publishInfo.IscsiTargetPortal) {
		// this is an IPv6 address
		portalIps = append(portalIps, strings.Split(publishInfo.IscsiTargetPortal, "]")[0])
	} else {
		portalIps = append(portalIps, strings.Split(publishInfo.IscsiTargetPortal, ":")[0])
	}
	for _, p := range publishInfo.IscsiPortals {
		bkportal = append(bkportal, p)
		portalIps = append(portalIps, strings.Split(p, ":")[0])
	}

	var targetIQN = publishInfo.IscsiTargetIQN
	var username = publishInfo.IscsiUsername
	var initiatorSecret = publishInfo.IscsiInitiatorSecret
	var iscsiInterface = publishInfo.IscsiInterface
	var fstype = publishInfo.FilesystemType
	var options = publishInfo.MountOptions

	log.WithFields(log.Fields{
		"volume":        name,
		"mountpoint":    mountpoint,
		"lunID":         lunID,
		"targetPortals": bkportal,
		"targetIQN":     targetIQN,
		"fstype":        fstype,
	}).Debug("Attaching iSCSI volume.")

	if ISCSISupported() == false {
		err := errors.New("unable to attach: open-iscsi tools not found on host")
		log.Errorf("Unable to attach volume: open-iscsi utils not found")
		return err
	}

	// If not logged in, login first
	sessionExists, err := iSCSISessionExistsToTargetIQN(targetIQN)
	if err != nil {
		return err
	}
	if !sessionExists {
		if publishInfo.UseCHAP {
			for _, portal := range bkportal {
				err = loginWithChap(targetIQN, portal, username, initiatorSecret, iscsiInterface, false)
				if err != nil {
					log.Errorf("Failed to login with CHAP credentials: %+v ", err)
					return fmt.Errorf("iSCSI login error: %v", err)
				}
			}
		} else {
			err = EnsureISCSISessions(portalIps)
			if err != nil {
				return fmt.Errorf("iSCSI session error: %v", err)
			}
		}
	}

	// If LUN isn't present, scan the target and wait for the device(s) to appear
	// if not attached need to scan
	shouldScan := !IsAlreadyAttached(lunID, targetIQN)
	err = waitForDeviceScanIfNeeded(lunID, targetIQN, shouldScan)
	if err != nil {
		log.Errorf("Could not find iSCSI device: %+v", err)
		return err
	}

	err = waitForMultipathDeviceForLUN(lunID, targetIQN)
	if err != nil {
		return err
	}

	// Lookup all the SCSI device information
	deviceInfo, err := getDeviceInfoForLUN(lunID, targetIQN)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %v", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get iSCSI device information for LUN %d", lunID)
	}

	log.WithFields(log.Fields{
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
	if err := waitForDevice(devicePath); err != nil {
		return fmt.Errorf("could not find device %v; %s", devicePath, err)
	}

	// Return the device in the publish info in case the mount will be done later
	publishInfo.DevicePath = devicePath

	if fstype == fsRaw {
		return nil
	}

	existingFstype := deviceInfo.Filesystem
	if existingFstype == "" {
		log.WithFields(log.Fields{"volume": name, "fstype": fstype}).Debug("Formatting LUN.")
		err := formatVolume(devicePath, fstype)
		if err != nil {
			return fmt.Errorf("error formatting LUN %s, device %s: %v", name, deviceToUse, err)
		}
	} else if existingFstype != fstype {
		log.WithFields(log.Fields{
			"volume":          name,
			"existingFstype":  existingFstype,
			"requestedFstype": fstype,
		}).Error("LUN already formatted with a different file system type.")
		return fmt.Errorf("LUN %s, device %s already formatted with other filesystem: %s",
			name, deviceToUse, existingFstype)
	} else {
		log.WithFields(log.Fields{
			"volume": name,
			"fstype": deviceInfo.Filesystem,
		}).Debug("LUN already formatted.")
	}

	// Optionally mount the device
	if mountpoint != "" {
		if err := MountDevice(devicePath, mountpoint, options, false); err != nil {
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
func GetDFOutput() ([]DFInfo, error) {

	log.Debug(">>>> osutils.GetDFOutput")
	defer log.Debug("<<<< osutils.GetDFOutput")

	var result []DFInfo
	out, err := execCommand("df", "--output=target,source")
	if err != nil {
		// df returns an error if there's a stale file handle that we can
		// safely ignore. There may be other reasons. Consider it a warning if
		// it printed anything to stdout.
		if len(out) == 0 {
			log.Error("Error encountered gathering df output.")
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
func GetInitiatorIqns() ([]string, error) {

	log.Debug(">>>> osutils.GetInitiatorIqns")
	defer log.Debug("<<<< osutils.GetInitiatorIqns")

	iqns := make([]string, 0)

	out, err := execCommand("cat", "/etc/iscsi/initiatorname.iscsi")
	if err != nil {
		log.WithField("Error", err).Warn("Could not read initiatorname.iscsi; perhaps iSCSI is not installed?")
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
func GetIPAddresses() ([]string, error) {
	ipAddrs := make([]string, 0)
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		err = fmt.Errorf("could not gather system IP addresses; %v", err)
		log.Error(err)
		return nil, err
	}

	// Remove obviously bad addresses such as loopback/linklocal
	for _, addr := range addrs {
		// net.Addr are of form 1.2.3.4/32, but IP needs 1.2.3.4, so we must strip the netmask (also works for IPv6)
		parsedAddr := net.ParseIP(strings.Split(addr.String(), "/")[0])
		if parsedAddr.IsGlobalUnicast() {
			log.WithField("IPAddress", parsedAddr.String()).Debug("Discovered potentially viable IP address.")
			ipAddrs = append(ipAddrs, parsedAddr.String())
		} else {
			log.WithField("IPAddress", parsedAddr.String()).Debug("Ignoring unusable IP address.")
		}
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
func EnsureFileExists(path string) error {
	fields := log.Fields{"path": path}
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			log.WithFields(fields).Error("Path exists but is a directory")
			return fmt.Errorf("path exists but is a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		log.WithFields(fields).Errorf("Can't determine if file exists; %s", err)
		return fmt.Errorf("can't determine if file %s exists; %s", path, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC, 0600)
	if nil != err {
		log.WithFields(fields).Errorf("OpenFile failed; %s", err)
		return fmt.Errorf("failed to create file %s; %s", path, err)
	}
	file.Close()

	return nil
}

// DeleteResourceAtPath makes sure that given named file or (empty) directory is removed
func DeleteResourceAtPath(resource string) error {
	return waitForResourceDeletionAtPath(resource)
}

// waitForResourceDeletionAtPath accepts a resource name and waits until it is deleted and returns error if it times out
func waitForResourceDeletionAtPath(resource string) error {

	fields := log.Fields{"resource": resource}
	log.WithFields(fields).Debug(">>>> osutils.waitForResourceDeletionAtPath")
	defer log.WithFields(fields).Debug("<<<< osutils.waitForResourceDeletionAtPath")

	maxDuration := resourceDeletionTimeoutSecs * time.Second

	checkResourceDeletion := func() error {
		if _, err := os.Stat(resource); err == nil {
			if err = os.Remove(resource); err != nil {
				log.WithFields(fields).Debugf("Failed to remove resource, %s", err)
				return fmt.Errorf("Failed to remove resource %s; %s", resource, err)
			}
			return nil
		} else if !os.IsNotExist(err) {
			log.WithFields(fields).Debugf("Can't determine if resource exists; %s", err)
			return fmt.Errorf("can't determine if resource %s exists; %s", resource, err)
		}

		return nil
	}

	deleteNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Resource not deleted yet, waiting.")
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
		log.WithField("resource", resource).Debug("Resource deleted.")
		return nil
	}
}

// EnsureDirExists makes sure that given directory structure exists
func EnsureDirExists(path string) error {
	fields := log.Fields{
		"path": path,
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			log.WithFields(fields).Error("Path exists but is not a directory")
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		log.WithFields(fields).Errorf("Can't determine if directory exists; %s", err)
		return fmt.Errorf("can't determine if directory %s exists; %s", path, err)
	}

	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.WithFields(fields).Errorf("Mkdir failed; %s", err)
		return fmt.Errorf("failed to mkdir %s; %s", path, err)
	}

	return nil
}

// getSysfsBlockDirsForLUN returns the list of directories in sysfs where the block devices should appear
// after the scan is successful. One directory is returned for each path in the host session map.
func getSysfsBlockDirsForLUN(lunID int, hostSessionMap map[int]int) []string {

	paths := make([]string, 0)
	for hostNumber, sessionNumber := range hostSessionMap {
		path := fmt.Sprintf(chrootPathPrefix+"/sys/class/scsi_host/host%d/device/session%d/iscsi_session/session%d/device/target%d:0:0/%d:0:0:%d",
			hostNumber, sessionNumber, sessionNumber, hostNumber, hostNumber, lunID)
		paths = append(paths, path)
	}
	return paths
}

// getDevicesForLUN find the /dev/sd* device names for an iSCSI LUN.
func getDevicesForLUN(paths []string) ([]string, error) {

	devices := make([]string, 0)
	for _, path := range paths {
		dirname := path + "/block"
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
func waitForDeviceScanIfNeeded(lunID int, iSCSINodeName string, shouldScan bool) error {

	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	log.WithFields(fields).Debug(">>>> osutils.waitForDeviceScanIfNeeded")
	defer log.WithFields(fields).Debug("<<<< osutils.waitForDeviceScanIfNeeded")

	hostSessionMap := getISCSIHostSessionMapForTarget(iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	log.WithField("hostSessionMap", hostSessionMap).Debug("Built iSCSI host/session map.")
	hosts := make([]int, 0)
	for hostNumber := range hostSessionMap {
		hosts = append(hosts, hostNumber)
	}

	if shouldScan {
		if err := iSCSIScanTargetLUN(lunID, hosts); err != nil {
			log.WithField("scanError", err).Error("Could not scan for new LUN.")
		}
	}

	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)
	log.Debugf("Scanning paths: %v", paths)
	found := make([]string, 0)

	checkAllDevicesExist := func() error {

		found := make([]string, 0)
		// Check if any paths present, and return nil (success) if so
		for _, path := range paths {
			dirname := path + "/block"
			if !PathExists(dirname) {
				return errors.New("device not present yet")
			}
			found = append(found, dirname)
		}
		return nil
	}

	devicesNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("All devices not yet present, waiting.")
	}

	deviceBackoff := backoff.NewExponentialBackOff()
	deviceBackoff.InitialInterval = 1 * time.Second
	deviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	deviceBackoff.RandomizationFactor = 0.1
	deviceBackoff.MaxElapsedTime = 5 * time.Second

	if err := backoff.RetryNotify(checkAllDevicesExist, deviceBackoff, devicesNotify); err == nil {
		log.Debugf("Paths found: %v", found)
		return nil
	}

	log.Debugf("Paths found so far: %v", found)

	checkAnyDeviceExists := func() error {

		found := make([]string, 0)
		// Check if any paths present, and return nil (success) if so
		for _, path := range paths {
			dirname := path + "/block"
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
		log.WithField("increment", duration).Debug("No devices present yet, waiting.")
	}

	deviceBackoff = backoff.NewExponentialBackOff()
	deviceBackoff.InitialInterval = 1 * time.Second
	deviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	deviceBackoff.RandomizationFactor = 0.1
	deviceBackoff.MaxElapsedTime = (iSCSIDeviceDiscoveryTimeoutSecs - 5) * time.Second

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(checkAnyDeviceExists, deviceBackoff, devicesNotify); err != nil {
		log.Warnf("Could not find all devices after %d seconds.", iSCSIDeviceDiscoveryTimeoutSecs)

		// In the case of a failure, log info about what devices are present
		execCommand("ls", "-al", "/dev")
		execCommand("ls", "-al", "/dev/mapper")
		execCommand("ls", "-al", "/dev/disk/by-path")
		execCommand("lsscsi")
		execCommand("lsscsi", "-t")
		execCommand("free")
		return err
	}

	log.Debugf("Paths found: %v", found)
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
func getDeviceInfoForLUN(lunID int, iSCSINodeName string) (*ScsiDeviceInfo, error) {

	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	log.WithFields(fields).Debug(">>>> osutils.getDeviceInfoForLUN")
	defer log.WithFields(fields).Debug("<<<< osutils.getDeviceInfoForLUN")

	hostSessionMap := getISCSIHostSessionMapForTarget(iSCSINodeName)
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
		multipathDevice = findMultipathDeviceForDevice(device)
		if multipathDevice != "" {
			break
		}
	}

	fsType := ""
	if multipathDevice != "" {
		fsType = getFSType("/dev/" + multipathDevice)
	} else {
		fsType = getFSType("/dev/" + devices[0])
	}

	log.WithFields(log.Fields{
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
func getDeviceInfoForMountPath(mountpath string) (*ScsiDeviceInfo, error) {

	fields := log.Fields{"mountpath": mountpath}
	log.WithFields(fields).Debug(">>>> osutils.getDeviceInfoForMountPath")
	defer log.WithFields(fields).Debug("<<<< osutils.getDeviceInfoForMountPath")

	device, _, err := GetDeviceNameFromMount(mountpath)
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
			Devices:         findDevicesForMultipathDevice(device),
			MultipathDevice: device,
		}
	}

	log.WithFields(log.Fields{
		"multipathDevice": deviceInfo.MultipathDevice,
		"devices":         deviceInfo.Devices,
	}).Debug("Found SCSI device.")

	return deviceInfo, nil
}

// waitForMultipathDeviceForLUN
func waitForMultipathDeviceForLUN(lunID int, iSCSINodeName string) error {
	fields := log.Fields{
		"lunID":         lunID,
		"iSCSINodeName": iSCSINodeName,
	}
	log.WithFields(fields).Debug(">>>> osutils.waitForMultipathDeviceForLUN")
	defer log.WithFields(fields).Debug("<<<< osutils.waitForMultipathDeviceForLUN")

	hostSessionMap := getISCSIHostSessionMapForTarget(iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	paths := getSysfsBlockDirsForLUN(lunID, hostSessionMap)

	devices, err := getDevicesForLUN(paths)
	if nil != err {
		return err
	}

	waitForMultipathDeviceForDevices(devices)
	return nil
}

// waitForMultipathDeviceForDevices accepts a list of sd* device names and waits until
// a multipath device is present for at least one of those.  It returns the name of the
// multipath device, or an empty string if multipathd isn't running or there is only one path.
func waitForMultipathDeviceForDevices(devices []string) string {

	fields := log.Fields{"devices": devices}
	log.WithFields(fields).Debug(">>>> osutils.waitForMultipathDeviceForDevices")
	defer log.WithFields(fields).Debug("<<<< osutils.waitForMultipathDeviceForDevices")

	if len(devices) <= 1 {
		log.Debugf("Skipping multipath discovery, %d device(s) specified.", len(devices))
		return ""
	} else if !multipathdIsRunning() {
		log.Debug("Skipping multipath discovery, multipathd isn't running.")
		return ""
	}

	maxDuration := multipathDeviceDiscoveryTimeoutSecs * time.Second
	multipathDevice := ""

	checkMultipathDeviceExists := func() error {

		for _, device := range devices {
			multipathDevice = findMultipathDeviceForDevice(device)
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
		log.WithField("increment", duration).Debug("Multipath device not yet present, waiting.")
	}

	multipathDeviceBackoff := backoff.NewExponentialBackOff()
	multipathDeviceBackoff.InitialInterval = 1 * time.Second
	multipathDeviceBackoff.Multiplier = 1.414 // approx sqrt(2)
	multipathDeviceBackoff.RandomizationFactor = 0.1
	multipathDeviceBackoff.MaxElapsedTime = maxDuration

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(checkMultipathDeviceExists, multipathDeviceBackoff, deviceNotify); err != nil {
		log.Warnf("Could not find multipath device after %3.2f seconds.", maxDuration.Seconds())
	} else {
		log.WithField("multipathDevice", multipathDevice).Debug("Multipath device found.")
	}
	return multipathDevice
}

// waitForDevice accepts a device name and waits until it is present and returns error if it times out
func waitForDevice(device string) error {

	fields := log.Fields{"device": device}
	log.WithFields(fields).Debug(">>>> osutils.waitForDevice")
	defer log.WithFields(fields).Debug("<<<< osutils.waitForDevice")

	maxDuration := multipathDeviceDiscoveryTimeoutSecs * time.Second

	checkDeviceExists := func() error {
		if !PathExists(device) {
			return errors.New("device not yet present")
		}
		return nil
	}

	deviceNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Device not yet present, waiting.")
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
		log.WithField("device", device).Debug("Device found.")
		return nil
	}
}

// findMultipathDeviceForDevice finds the devicemapper parent of a device name like /dev/sdx.
func findMultipathDeviceForDevice(device string) string {

	log.WithField("device", device).Debug(">>>> osutils.findMultipathDeviceForDevice")
	defer log.WithField("device", device).Debug("<<<< osutils.findMultipathDeviceForDevice")

	holdersDir := chrootPathPrefix + "/sys/block/" + device + "/holders"
	if dirs, err := ioutil.ReadDir(holdersDir); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.HasPrefix(name, "dm-") {
				return name
			}
		}
	}

	log.WithField("device", device).Debug("Could not find multipath device for device.")
	return ""
}

// findDevicesForMultipathDevice finds the constituent devices for a devicemapper parent device like /dev/dm-0.
func findDevicesForMultipathDevice(device string) []string {

	log.WithField("device", device).Debug(">>>> osutils.findDevicesForMultipathDevice")
	defer log.WithField("device", device).Debug("<<<< osutils.findDevicesForMultipathDevice")

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
		log.WithField("device", device).Debug("Could not find devices for multipath device.")
	} else {
		log.WithFields(log.Fields{
			"device":  device,
			"devices": devices,
		}).Debug("Found devices for multipath device.")
	}

	return devices
}

// PrepareDeviceForRemoval informs Linux that a device will be removed.
func PrepareDeviceForRemoval(lunID int, iSCSINodeName string) {

	fields := log.Fields{
		"lunID":            lunID,
		"iSCSINodeName":    iSCSINodeName,
		"chrootPathPrefix": chrootPathPrefix,
	}
	log.WithFields(fields).Debug(">>>> osutils.PrepareDeviceForRemoval")
	defer log.WithFields(fields).Debug("<<<< osutils.PrepareDeviceForRemoval")

	deviceInfo, err := getDeviceInfoForLUN(lunID, iSCSINodeName)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"lunID": lunID,
		}).Info("Could not get device info for removal, skipping host removal steps.")
		return
	}

	removeSCSIDevice(deviceInfo)
}

// PrepareDeviceAtMountPathForRemoval informs Linux that a device will be removed.
func PrepareDeviceAtMountPathForRemoval(mountpoint string, unmount bool) error {

	fields := log.Fields{"mountpoint": mountpoint}
	log.WithFields(fields).Debug(">>>> osutils.PrepareDeviceAtMountPathForRemoval")
	defer log.WithFields(fields).Debug("<<<< osutils.PrepareDeviceAtMountPathForRemoval")

	deviceInfo, err := getDeviceInfoForMountPath(mountpoint)
	if err != nil {
		return err
	}

	if unmount {
		if err := Umount(mountpoint); err != nil {
			return err
		}
	}

	removeSCSIDevice(deviceInfo)
	return nil
}

// removeSCSIDevice informs Linux that a device will be removed.  The deviceInfo provided only needs
// the devices and multipathDevice fields set.
func removeSCSIDevice(deviceInfo *ScsiDeviceInfo) {

	// Flush multipath device
	multipathFlushDevice(deviceInfo)

	// Flush devices
	flushDevice(deviceInfo)

	// Remove device
	removeDevice(deviceInfo)

	// Give the host a chance to fully process the removal
	time.Sleep(time.Second)
}

// ISCSISupported returns true if iscsiadm is installed and in the PATH.
func ISCSISupported() bool {

	log.Debug(">>>> osutils.ISCSISupported")
	defer log.Debug("<<<< osutils.ISCSISupported")

	_, err := execIscsiadmCommand("-V")
	if err != nil {
		log.Debug("iscsiadm tools not found on this host.")
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
func iSCSIDiscovery(portal string) ([]ISCSIDiscoveryInfo, error) {

	log.WithField("portal", portal).Debug(">>>> osutils.iSCSIDiscovery")
	defer log.Debug("<<<< osutils.iSCSIDiscovery")

	out, err := execIscsiadmCommand("-m", "discovery", "-t", "sendtargets", "-p", portal)
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

			log.WithFields(log.Fields{
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
func getISCSISessionInfo() ([]ISCSISessionInfo, error) {

	log.Debug(">>>> osutils.getISCSISessionInfo")
	defer log.Debug("<<<< osutils.getISCSISessionInfo")

	out, err := execIscsiadmCommand("-m", "session")
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == iSCSIErrNoObjsFound {
			log.Debug("No iSCSI session found.")
			return []ISCSISessionInfo{}, nil
		} else {
			log.WithField("error", err).Error("Problem checking iSCSI sessions.")
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

			log.WithFields(log.Fields{
				"SID":        sid,
				"Portal":     a[2],
				"PortalIP":   portalIP,
				"TargetName": a[3],
			}).Debug("Adding iSCSI session info.")
		}
	}

	return sessionInfo, nil
}

// ISCSIDisableDelete logs out from the supplied target and removes the iSCSI device.
func ISCSIDisableDelete(targetIQN, targetPortal string) error {

	logFields := log.Fields{
		"targetIQN":    targetIQN,
		"targetPortal": targetPortal,
	}
	log.WithFields(logFields).Debug(">>>> osutils.ISCSIDisableDelete")
	defer log.WithFields(logFields).Debug("<<<< osutils.ISCSIDisableDelete")

	_, err := execIscsiadmCommand("-m", "node", "-T", targetIQN, "--portal", targetPortal, "-u")
	if err != nil {
		log.WithField("error", err).Debug("Error during iSCSI logout.")
	}

	_, err = execIscsiadmCommand("-m", "node", "-o", "delete", "-T", targetIQN)
	return err
}

// UmountAndRemoveTemporaryMountPoint unmounts and removes the TemporaryMountDir
func UmountAndRemoveTemporaryMountPoint(mountPath string) error {
	log.Debug(">>>> osutils.UmountAndRemoveTemporaryMountPoint")
	defer log.Debug("<<<< osutils.UmountAndRemoveTemporaryMountPoint")

	// Delete the temporary mount point if it exists.
	tmpDir := path.Join(mountPath, temporaryMountDir)
	if _, err := os.Stat(tmpDir); err == nil {
		if err = removeMountPoint(tmpDir); err != nil {
			return fmt.Errorf("failed to remove directory in staging target path %s; %s", tmpDir, err)
		}
	} else if !os.IsNotExist(err) {
		log.WithField("temporaryMountPoint", tmpDir).Errorf("Can't determine if temporary dir path exists; %s", err)
		return fmt.Errorf("can't determine if temporary dir path %s exists; %s", tmpDir, err)
	}

	return nil
}

// removeMountPoint attempts to unmount and remove the directory of the mountPointPath
func removeMountPoint(mountPointPath string) error {
	log.Debug(">>>> osutils.removeMountPoint")
	defer log.Debug("<<<< osutils.removeMountPoint")

	err := Umount(mountPointPath)
	if err != nil {
		log.WithField("mountPointPath", mountPointPath).Errorf("Umount failed; %s", err)
		return err
	}

	err = os.Remove(mountPointPath)
	if err != nil {
		log.WithField("mountPointPath", mountPointPath).Errorf("Remove dir failed; %s", err)
		return fmt.Errorf("failed to remove dir %s; %s", mountPointPath, err)
	}
	return nil
}

// mountFilesystemForResize expands a filesystem. The xfs_growfs utility requires a mount point to expand the
// filesystem. Determining the size of the filesystem requires that the filesystem be mounted.
func mountFilesystemForResize(devicePath string, stagedTargetPath string, mountOptions string) (string, error) {
	logFields := log.Fields{
		"devicePath":       devicePath,
		"stagedTargetPath": stagedTargetPath,
		"mountOptions":     mountOptions,
	}
	log.WithFields(logFields).Debug(">>>> osutils.mountAndExpandFilesystem")
	defer log.WithFields(logFields).Debug("<<<< osutils.mountAndExpandFilesystem")

	tmpMountPoint := path.Join(stagedTargetPath, temporaryMountDir)
	err := MountDevice(devicePath, tmpMountPoint, mountOptions, false)
	if err != nil {
		return "", fmt.Errorf("unable to mount device; %s", err)
	}
	return tmpMountPoint, nil
}

// ExpandISCSIFilesystem will expand the filesystem of an already expanded volume.
func ExpandISCSIFilesystem(publishInfo *VolumePublishInfo, stagedTargetPath string) (int64, error) {
	devicePath := publishInfo.DevicePath
	logFields := log.Fields{
		"devicePath":       devicePath,
		"stagedTargetPath": stagedTargetPath,
		"mountOptions":     publishInfo.MountOptions,
		"filesystemType":   publishInfo.FilesystemType,
	}
	log.WithFields(logFields).Debug(">>>> osutils.ExpandISCSIFilesystem")
	defer log.WithFields(logFields).Debug("<<<< osutils.ExpandISCSIFilesystem")

	tmpMountPoint, err := mountFilesystemForResize(publishInfo.DevicePath, stagedTargetPath, publishInfo.MountOptions)
	if err != nil {
		return 0, err
	}
	defer removeMountPoint(tmpMountPoint)

	// Don't need to verify the filesystem type as the resize utilities will throw an error if the filesystem
	// is not the correct type.
	var size int64
	switch publishInfo.FilesystemType {
	case "xfs":
		size, err = expandFilesystem("xfs_growfs", tmpMountPoint, tmpMountPoint)
		if err != nil {
			return 0, err
		}
	case "ext3", "ext4":
		size, err = expandFilesystem("resize2fs", devicePath, tmpMountPoint)
		if err != nil {
			return 0, err
		}
	default:
		err = fmt.Errorf("unsupported file system type: %s", publishInfo.FilesystemType)
	}

	return size, err
}

func expandFilesystem(cmd string, cmdArguments string, tmpMountPoint string) (int64, error) {
	logFields := log.Fields{
		"cmd":           cmd,
		"cmdArguments":  cmdArguments,
		"tmpMountPoint": tmpMountPoint,
	}
	log.WithFields(logFields).Debug(">>>> osutils.expandFilesystem")
	defer log.WithFields(logFields).Debug("<<<< osutils.expandFilesystem")

	preExpandSize, err := getFilesystemSize(tmpMountPoint)
	if err != nil {
		return 0, err
	}
	_, err = execCommand(cmd, cmdArguments)
	if err != nil {
		log.Errorf("Expanding filesystem failed; %s", err)
		return 0, err
	}

	postExpandSize, err := getFilesystemSize(tmpMountPoint)
	if err != nil {
		return 0, err
	}

	if postExpandSize == preExpandSize {
		log.Warnf("Failed to expand filesystem; size=%d", postExpandSize)
	}

	return postExpandSize, nil
}

// iSCSISessionExists checks to see if a session exists to the specified portal.
func iSCSISessionExists(portal string) (bool, error) {

	log.Debug(">>>> osutils.iSCSISessionExists")
	defer log.Debug("<<<< osutils.iSCSISessionExists")

	sessionInfo, err := getISCSISessionInfo()
	if err != nil {
		log.WithField("error", err).Error("Problem checking iSCSI sessions.")
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
func iSCSISessionExistsToTargetIQN(targetIQN string) (bool, error) {

	log.Debug(">>>> osutils.iSCSISessionExistsToTargetIQN")
	defer log.Debug("<<<< osutils.iSCSISessionExistsToTargetIQN")

	sessionInfo, err := getISCSISessionInfo()
	if err != nil {
		log.WithField("error", err).Error("Problem checking iSCSI sessions.")
		return false, err
	}

	for _, e := range sessionInfo {
		if e.TargetName == targetIQN {
			return true, nil
		}
	}

	return false, nil
}

func ISCSIRescanDevices(targetIQN string, lunID int32, minSize int64) error {
	fields := log.Fields{"targetIQN": targetIQN, "lunID": lunID}
	log.WithFields(fields).Debug(">>>> osutils.ISCSIRescanDevices")
	defer log.WithFields(fields).Debug("<<<< osutils.ISCSIRescanDevices")

	deviceInfo, err := getDeviceInfoForLUN(int(lunID), targetIQN)
	if err != nil {
		return fmt.Errorf("error getting iSCSI device information: %s", err)
	} else if deviceInfo == nil {
		return fmt.Errorf("could not get iSCSI device information for LUN: %d", lunID)
	}

	allLargeEnough := true
	for _, diskDevice := range deviceInfo.Devices {
		size, err := getISCSIDiskSize("/dev/" + diskDevice)
		if err != nil {
			return err
		}
		if size < minSize {
			allLargeEnough = false
		} else {
			continue
		}

		err = iSCSIRescanDisk(diskDevice)
		if err != nil {
			log.WithField("diskDevice", diskDevice).Error("Failed to rescan disk.")
			return fmt.Errorf("failed to rescan disk %s: %s", diskDevice, err)
		}
	}

	if !allLargeEnough {
		time.Sleep(time.Second)
		for _, diskDevice := range deviceInfo.Devices {
			size, err := getISCSIDiskSize("/dev/" + diskDevice)
			if err != nil {
				return err
			}
			if size < minSize {
				log.Error("Disk size not large enough after resize.")
				return fmt.Errorf("disk size not large enough after resize: %d, %d", size, minSize)
			}
		}
	}

	if deviceInfo.MultipathDevice != "" {
		multipathDevice := deviceInfo.MultipathDevice
		size, err := getISCSIDiskSize("/dev/" + multipathDevice)
		if err != nil {
			return err
		}

		if size < minSize {
			err := reloadMultipathDevice(multipathDevice)
			if err != nil {
				return err
			}
			time.Sleep(time.Second)
			size, err = getISCSIDiskSize("/dev/" + multipathDevice)
			if err != nil {
				return err
			}
			if size < minSize {
				log.Error("Multipath device not large enough after resize.")
				return fmt.Errorf("multipath device not large enough after resize: %d < %d", size, minSize)
			}
		}
	}

	return nil
}

func reloadMultipathDevice(multipathDevice string) error {
	fields := log.Fields{"multipathDevice": multipathDevice}
	log.WithFields(fields).Debug(">>>> osutils.reloadMultipathDevice")
	defer log.WithFields(fields).Debug("<<<< osutils.reloadMultipathDevice")

	if multipathDevice == "" {
		return fmt.Errorf("cannot reload an empty multipathDevice")
	}

	_, err := execCommandWithTimeout("multipath", 30, "-r", "/dev/"+multipathDevice)
	if err != nil {
		log.WithFields(log.Fields{
			"device": multipathDevice,
			"error":  err,
		}).Error("Failed to reload multipathDevice.")
		return fmt.Errorf("failed to reload multipathDevice %s: %s", multipathDevice, err)
	}

	log.WithFields(fields).Debug("Multipath device reloaded.")
	return nil
}

// iSCSIRescanDisk causes the kernel to rescan a single iSCSI disk/block device.
// This is how size changes are found when expanding a volume.
func iSCSIRescanDisk(deviceName string) error {
	fields := log.Fields{"deviceName": deviceName}
	log.WithFields(fields).Debug(">>>> osutils.iSCSIRescanDisk")
	defer log.WithFields(fields).Debug("<<<< osutils.iSCSIRescanDisk")

	filename := fmt.Sprintf(chrootPathPrefix+"/sys/block/%s/device/rescan", deviceName)
	log.WithField("filename", filename).Debug("Opening file for writing.")

	f, err := os.OpenFile(filename, os.O_WRONLY, 0)
	if err != nil {
		log.WithField("file", filename).Warning("Could not open file for writing.")
		return err
	}
	defer f.Close()

	written, err := f.WriteString("1")
	if err != nil {
		log.WithFields(log.Fields{
			"file":  filename,
			"error": err,
		}).Warning("Could not write to file.")
		return err
	} else if written == 0 {
		log.WithField("file", filename).Warning("Zero bytes written to file.")
		return fmt.Errorf("no data written to %s", filename)
	}

	return nil
}

// iSCSIScanTargetLUN scans a single LUN on an iSCSI target to discover it.
func iSCSIScanTargetLUN(lunID int, hosts []int) error {

	fields := log.Fields{"hosts": hosts, "lunID": lunID}
	log.WithFields(fields).Debug(">>>> osutils.iSCSIScanTargetLUN")
	defer log.WithFields(fields).Debug("<<<< osutils.iSCSIScanTargetLUN")

	var (
		f   *os.File
		err error
	)

	for _, hostNumber := range hosts {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/class/scsi_host/host%d/scan", hostNumber)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0200); err != nil {
			log.WithField("file", filename).Warning("Could not open file for writing.")
			return err
		}

		scanCmd := fmt.Sprintf("0 0 %d", lunID)
		if written, err := f.WriteString(scanCmd); err != nil {
			log.WithFields(log.Fields{"file": filename, "error": err}).Warning("Could not write to file.")
			f.Close()
			return err
		} else if written == 0 {
			log.WithField("file", filename).Warning("No data written to file.")
			f.Close()
			return fmt.Errorf("no data written to %s", filename)
		}

		f.Close()

		log.WithFields(log.Fields{
			"scanCmd":  scanCmd,
			"scanFile": filename,
		}).Debug("Invoked single-LUN scan.")
	}

	return nil
}

// IsAlreadyAttached checks if there is already an established iSCSI session to the specified LUN.
func IsAlreadyAttached(lunID int, targetIqn string) bool {

	hostSessionMap := getISCSIHostSessionMapForTarget(targetIqn)
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

// getISCSIHostSessionMapForTarget returns a map of iSCSI host numbers to iSCSI session numbers
// for a given iSCSI target.
func getISCSIHostSessionMapForTarget(iSCSINodeName string) map[int]int {

	fields := log.Fields{"iSCSINodeName": iSCSINodeName}
	log.WithFields(fields).Debug(">>>> osutils.getISCSIHostSessionMapForTarget")
	defer log.WithFields(fields).Debug("<<<< osutils.getISCSIHostSessionMapForTarget")

	var (
		hostNumber    int
		sessionNumber int
	)

	hostSessionMap := make(map[int]int)

	sysPath := chrootPathPrefix + "/sys/class/iscsi_host/"
	if hostDirs, err := ioutil.ReadDir(sysPath); err != nil {
		log.WithField("error", err).Errorf("Could not read %s", sysPath)
		return hostSessionMap
	} else {
		for _, hostDir := range hostDirs {

			hostName := hostDir.Name()
			if !strings.HasPrefix(hostName, "host") {
				continue
			} else if hostNumber, err = strconv.Atoi(strings.TrimPrefix(hostName, "host")); err != nil {
				log.WithField("host", hostName).Error("Could not parse host number")
				continue
			}

			devicePath := sysPath + hostName + "/device/"
			if deviceDirs, err := ioutil.ReadDir(devicePath); err != nil {
				log.WithFields(log.Fields{
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
						log.WithField("session", sessionName).Error("Could not parse session number")
						continue
					}

					targetNamePath := devicePath + sessionName + "/iscsi_session/" + sessionName + "/targetname"
					if targetName, err := ioutil.ReadFile(targetNamePath); err != nil {

						log.WithFields(log.Fields{
							"path":  targetNamePath,
							"error": err,
						}).Error("Could not read targetname file")

					} else if strings.TrimSpace(string(targetName)) == iSCSINodeName {

						log.WithFields(log.Fields{
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
func GetISCSIDevices() ([]*ScsiDeviceInfo, error) {

	log.Debug(">>>> osutils.GetISCSIDevices")
	defer log.Debug("<<<< osutils.GetISCSIDevices")

	devices := make([]*ScsiDeviceInfo, 0)
	hostSessionMapCache := make(map[string]map[int]int)

	// Start by reading the sessions from /sys/class/iscsi_session
	sysPath := chrootPathPrefix + "/sys/class/iscsi_session/"
	sessionDirs, err := ioutil.ReadDir(sysPath)
	if err != nil {
		log.WithField("error", err).Errorf("Could not read %s", sysPath)
		return nil, err
	}

	// Loop through each of the iSCSI sessions
	for _, sessionDir := range sessionDirs {

		sessionName := sessionDir.Name()
		if !strings.HasPrefix(sessionName, "session") {
			continue
		} else if _, err = strconv.Atoi(strings.TrimPrefix(sessionName, "session")); err != nil {
			log.WithField("session", sessionName).Error("Could not parse session number")
			return nil, err
		}

		// Find the target IQN from the session at /sys/class/iscsi_session/sessionXXX/targetname
		sessionPath := sysPath + sessionName
		targetNamePath := sessionPath + "/targetname"
		targetNameBytes, err := ioutil.ReadFile(targetNamePath)
		if err != nil {
			log.WithFields(log.Fields{
				"path":  targetNamePath,
				"error": err,
			}).Error("Could not read targetname file")
			return nil, err
		}

		targetIQN := strings.TrimSpace(string(targetNameBytes))

		log.WithFields(log.Fields{
			"targetIQN":   targetIQN,
			"sessionName": sessionName,
		}).Debug("Found iSCSI session / target IQN.")

		// Find the one target at /sys/class/iscsi_session/sessionXXX/device/targetHH:BB:DD (host:bus:device)
		sessionDevicePath := sessionPath + "/device/"
		targetDirs, err := ioutil.ReadDir(sessionDevicePath)
		if err != nil {
			log.WithField("error", err).Errorf("Could not read %s", sessionDevicePath)
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
			log.Warningf("Could not find a host:bus:device directory at %s", sessionDevicePath)
			continue
		}

		sessionDeviceHBDPath := sessionDevicePath + targetDirName + "/"

		log.WithFields(log.Fields{
			"hbdPath": sessionDeviceHBDPath,
			"hbdName": hostBusDeviceName,
		}).Debug("Found host/bus/device path.")

		// Find the devices at /sys/class/iscsi_session/sessionXXX/device/targetHH:BB:DD/HH:BB:DD:LL (host:bus:device:lun)
		hostBusDeviceLunDirs, err := ioutil.ReadDir(sessionDeviceHBDPath)
		if err != nil {
			log.WithField("error", err).Errorf("Could not read %s", sessionDeviceHBDPath)
			return nil, err
		}

		for _, hostBusDeviceLunDir := range hostBusDeviceLunDirs {

			hostBusDeviceLunDirName := hostBusDeviceLunDir.Name()
			if !strings.HasPrefix(hostBusDeviceLunDirName, hostBusDeviceName) {
				continue
			}

			sessionDeviceHBDLPath := sessionDeviceHBDPath + hostBusDeviceLunDirName + "/"

			log.WithFields(log.Fields{
				"hbdlPath": sessionDeviceHBDLPath,
				"hbdlName": hostBusDeviceLunDirName,
			}).Debug("Found host/bus/device/LUN path.")

			hbdlValues := strings.Split(hostBusDeviceLunDirName, ":")
			if len(hbdlValues) != 4 {
				log.Errorf("Could not parse values from %s", hostBusDeviceLunDirName)
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
				log.WithField("error", err).Errorf("Could not read %s", blockPath)
				return nil, err
			}

			for _, blockDeviceDir := range blockDeviceDirs {

				blockDeviceName := blockDeviceDir.Name()

				log.WithField("blockDeviceName", blockDeviceName).Debug("Found block device.")

				// Find multipath device, if any
				var slaveDevices []string
				multipathDevice := findMultipathDeviceForDevice(blockDeviceName)
				if multipathDevice != "" {
					slaveDevices = findDevicesForMultipathDevice(multipathDevice)
				} else {
					slaveDevices = []string{blockDeviceName}
				}

				// Get the host/session map, using a cached value if available
				hostSessionMap, ok := hostSessionMapCache[targetIQN]
				if !ok {
					hostSessionMap = getISCSIHostSessionMapForTarget(targetIQN)
					hostSessionMapCache[targetIQN] = hostSessionMap
				}

				log.WithFields(log.Fields{
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
func IsMounted(sourceDevice, mountpoint string) (bool, error) {

	fields := log.Fields{
		"source": sourceDevice,
		"target": mountpoint,
	}
	log.WithFields(fields).Debug(">>>> osutils.IsMounted")
	defer log.WithFields(fields).Debug("<<<< osutils.IsMounted")

	procSelfMountinfo, err := listProcSelfMountinfo(procSelfMountinfoPath)

	if err != nil {
		log.WithFields(fields).Errorf("checking mounted failed; %s", err)
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

		log.Debugf("Mountpoint found: %v", procMount)

		if sourceDevice == "" {
			log.Debugf("Source device: none, Target: %s, is mounted: true", mountpoint)
			return true, nil
		}

		hasDevMountSourcePrefix := strings.HasPrefix(procMount.MountSource, "/dev/")

		var mountedDevice string
		// Resolve any symlinks to get the real device
		if hasDevMountSourcePrefix {
			device, err := filepath.EvalSymlinks(procMount.MountSource)
			if err != nil {
				log.Error(err)
				continue
			}
			mountedDevice = strings.TrimPrefix(device, "/dev/")
		} else {
			mountedDevice = strings.TrimPrefix(procMount.Root, "/")
		}

		if sourceDeviceName == mountedDevice {
			log.Debugf("Source device: %s, Target: %s, is mounted: true", sourceDeviceName, mountpoint)
			return true, nil
		}
	}

	log.Debugf("Source device: %s, Target: %s, is mounted: false", sourceDevice, mountpoint)
	return false, nil
}

// GetMountedISCSIDevices returns a list of iSCSI devices that are *mounted* on this host.
func GetMountedISCSIDevices() ([]*ScsiDeviceInfo, error) {

	log.Debug(">>>> osutils.GetMountedISCSIDevices")
	defer log.Debug("<<<< osutils.GetMountedISCSIDevices")

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
				log.Error(err)
				continue
			}
			mountedDevice = strings.TrimPrefix(device, "/dev/")
		} else {
			mountedDevice = strings.TrimPrefix(procMount.Root, "/")
		}

		mountedDevices = append(mountedDevices, mountedDevice)
	}

	// Get all known iSCSI devices
	iscsiDevices, err := GetISCSIDevices()
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
		log.WithFields(log.Fields{
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
func ISCSITargetHasMountedDevice(targetIQN string) (bool, error) {

	mountedISCSIDevices, err := GetMountedISCSIDevices()
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
func multipathFlushDevice(deviceInfo *ScsiDeviceInfo) {

	log.WithField("device", deviceInfo.MultipathDevice).Debug(">>>> osutils.multipathFlushDevice")
	defer log.Debug("<<<< osutils.multipathFlushDevice")

	if deviceInfo.MultipathDevice == "" {
		return
	}

	_, err := execCommandWithTimeout("multipath", 30, "-f", "/dev/"+deviceInfo.MultipathDevice)
	if err != nil {
		// nothing to do if it generates an error but log it
		log.WithFields(log.Fields{
			"device": deviceInfo.MultipathDevice,
			"error":  err,
		}).Warning("Error encountered in multipath flush device command.")
	}
}

// flushDevice flushes any outstanding I/O to all paths to a device.
func flushDevice(deviceInfo *ScsiDeviceInfo) {

	log.Debug(">>>> osutils.flushDevice")
	defer log.Debug("<<<< osutils.flushDevice")

	for _, device := range deviceInfo.Devices {
		_, err := execCommandWithTimeout("blockdev", 5, "--flushbufs", "/dev/"+device)
		if err != nil {
			// nothing to do if it generates an error but log it
			log.WithFields(log.Fields{
				"device": device,
				"error":  err,
			}).Warning("Error encountered in blockdev --flushbufs command.")
		}
	}
}

// removeDevice tells Linux that a device will be removed.
func removeDevice(deviceInfo *ScsiDeviceInfo) {

	log.Debug(">>>> osutils.removeDevice")
	defer log.Debug("<<<< osutils.removeDevice")

	var (
		f   *os.File
		err error
	)

	for _, deviceName := range deviceInfo.Devices {

		filename := fmt.Sprintf(chrootPathPrefix+"/sys/block/%s/device/delete", deviceName)
		if f, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0200); err != nil {
			log.WithField("file", filename).Warning("Could not open file for writing.")
			return
		}

		if written, err := f.WriteString("1"); err != nil {
			log.WithFields(log.Fields{"file": filename, "error": err}).Warning("Could not write to file.")
			f.Close()
			return
		} else if written == 0 {
			log.WithField("file", filename).Warning("No data written to file.")
			f.Close()
			return
		}

		f.Close()

		log.WithField("scanFile", filename).Debug("Invoked device delete.")
	}
}

// multipathdIsRunning returns true if the multipath daemon is running.
func multipathdIsRunning() bool {

	log.Debug(">>>> osutils.multipathdIsRunning")
	defer log.Debug("<<<< osutils.multipathdIsRunning")

	out, err := execCommand("pgrep", "multipathd")
	if err == nil {
		pid := strings.TrimSpace(string(out))
		if pidRegex.MatchString(pid) {
			log.WithField("pid", pid).Debug("multipathd is running")
			return true
		}
	} else {
		log.Error(err)
	}

	out, err = execCommand("multipathd", "show", "daemon")
	if err == nil {
		if pidRunningRegex.MatchString(string(out)) {
			log.Debug("multipathd is running")
			return true
		}
	} else {
		log.Error(err)
	}

	return false
}

// getFSType returns the filesystem for the supplied device.
func getFSType(device string) string {

	log.WithField("device", device).Debug(">>>> osutils.getFSType")
	defer log.Debug("<<<< osutils.getFSType")

	fsType := ""
	out, err := execCommand("blkid", device)
	if err != nil {
		log.WithField("device", device).Debug("Could not get FSType for device.")
		return fsType
	}

	if strings.Contains(string(out), "TYPE=") {
		for _, v := range strings.Split(string(out), " ") {
			if strings.Contains(v, "TYPE=") {
				fsType = strings.Split(v, "=")[1]
				fsType = strings.Replace(fsType, "\"", "", -1)
				fsType = strings.TrimSpace(fsType)
			}
		}
	}
	return fsType
}

// formatVolume creates a filesystem for the supplied device of the supplied type.
func formatVolume(device, fstype string) error {

	logFields := log.Fields{"device": device, "fsType": fstype}
	log.WithFields(logFields).Debug(">>>> osutils.formatVolume")
	defer log.WithFields(logFields).Debug("<<<< osutils.formatVolume")

	maxDuration := 30 * time.Second

	formatVolume := func() error {

		var err error

		switch fstype {
		case "xfs":
			_, err = execCommand("mkfs.xfs", "-f", device)
		case "ext3":
			_, err = execCommand("mkfs.ext3", "-F", device)
		case "ext4":
			_, err = execCommand("mkfs.ext4", "-F", device)
		default:
			return fmt.Errorf("unsupported file system type: %s", fstype)
		}

		return err
	}

	formatNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Format failed, retrying.")
	}

	formatBackoff := backoff.NewExponentialBackOff()
	formatBackoff.InitialInterval = 2 * time.Second
	formatBackoff.Multiplier = 2
	formatBackoff.RandomizationFactor = 0.1
	formatBackoff.MaxElapsedTime = maxDuration

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(formatVolume, formatBackoff, formatNotify); err != nil {
		log.Warnf("Could not format device after %3.2f seconds.", maxDuration.Seconds())
		return err
	}

	log.WithFields(logFields).Info("Device formatted.")
	return nil
}

// MountDevice attaches the supplied device at the supplied location.  Use this for iSCSI devices.
func MountDevice(device, mountpoint, options string, isMountPointFile bool) (err error) {

	log.WithFields(log.Fields{
		"device":     device,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> osutils.MountDevice")
	defer log.Debug("<<<< osutils.MountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-o", strings.TrimPrefix(options, "-o "), device, mountpoint}
	} else {
		args = []string{device, mountpoint}
	}

	mounted, _ := IsMounted(device, mountpoint)
	exists := PathExists(mountpoint)

	log.Debugf("Already mounted: %v, mountpoint exists: %v", mounted, exists)

	if !exists {
		if isMountPointFile {
			if err = EnsureFileExists(mountpoint); err != nil {
				log.WithField("error", err).Warning("File check failed.")
			}
		} else {
			if err = EnsureDirExists(mountpoint); err != nil {
				log.WithField("error", err).Warning("Mkdir failed.")
			}
		}
	}

	if !mounted {
		if _, err = execCommand("mount", args...); err != nil {
			log.WithField("error", err).Error("Mount failed.")
		}
	}

	return
}

// mountNFSPath attaches the supplied NFS share at the supplied location with options.
func mountNFSPath(exportPath, mountpoint, options string) (err error) {

	log.WithFields(log.Fields{
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> osutils.mountNFSPath")
	defer log.Debug("<<<< osutils.mountNFSPath")

	// Build the command
	var args []string
	if len(options) > 0 {
		args = []string{"-t", "nfs", "-o", strings.TrimPrefix(options, "-o "), exportPath, mountpoint}
	} else {
		args = []string{"-t", "nfs", exportPath, mountpoint}
	}

	// Create the mount point dir if necessary
	if _, err = execCommand("mkdir", "-p", mountpoint); err != nil {
		log.WithField("error", err).Warning("Mkdir failed.")
	}

	if out, err := execCommand("mount", args...); err != nil {
		log.WithField("output", string(out)).Debug("Mount failed.")
		return fmt.Errorf("error mounting NFS volume %v on mountpoint %v: %v", exportPath, mountpoint, err)
	}

	return nil
}

// Umount detaches from the supplied location.
func Umount(mountpoint string) (err error) {

	log.WithField("mountpoint", mountpoint).Debug(">>>> osutils.Umount")
	defer log.Debug("<<<< osutils.Umount")

	if _, err = execCommand("umount", mountpoint); err != nil {
		log.WithField("error", err).Error("Umount failed.")
	}
	return
}

// loginISCSITarget logs in to an iSCSI target.
func loginISCSITarget(iqn, portal string) error {

	log.WithFields(log.Fields{
		"IQN":    iqn,
		"Portal": portal,
	}).Debug(">>>> osutils.loginISCSITarget")
	defer log.Debug("<<<< osutils.loginISCSITarget")

	args := []string{"-m", "node", "-T", iqn, "-l", "-p", portal + ":3260"}

	if _, err := execIscsiadmCommand(args...); err != nil {
		log.WithField("error", err).Error("Error logging in to iSCSI target.")
		return err
	}
	return nil
}

// loginWithChap will login to the iSCSI target with the supplied credentials.
func loginWithChap(tiqn, portal, username, password, iface string, logSensitiveInfo bool) error {

	logFields := log.Fields{
		"IQN":      tiqn,
		"portal":   portal,
		"username": username,
		"password": "****",
		"iface":    iface,
	}
	if logSensitiveInfo {
		logFields["password"] = password
	}
	log.WithFields(logFields).Debug(">>>> osutils.loginWithChap")
	defer log.Debug("<<<< osutils.loginWithChap")

	args := []string{"-m", "node", "-T", tiqn, "-p", portal + ":3260"}

	createArgs := append(args, []string{"--interface", iface, "--op", "new"}...)
	if _, err := execIscsiadmCommand(createArgs...); err != nil {
		log.Error("Error running iscsiadm node create.")
		return err
	}

	authMethodArgs := append(args, []string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
	if _, err := execIscsiadmCommand(authMethodArgs...); err != nil {
		log.Error("Error running iscsiadm set authmethod.")
		return err
	}

	authUserArgs := append(args, []string{"--op=update", "--name", "node.session.auth.username", "--value=" + username}...)
	if _, err := execIscsiadmCommand(authUserArgs...); err != nil {
		log.Error("Error running iscsiadm set authuser.")
		return err
	}

	authPasswordArgs := append(args, []string{"--op=update", "--name", "node.session.auth.password", "--value=" + password}...)
	if _, err := execIscsiadmCommand(authPasswordArgs...); err != nil {
		log.Error("Error running iscsiadm set authpassword.")
		return err
	}

	loginArgs := append(args, []string{"--login"}...)
	if _, err := execIscsiadmCommand(loginArgs...); err != nil {
		log.Error("Error running iscsiadm login.")
		return err
	}

	return nil
}

func EnsureISCSISessions(hostDataIPs []string) error {
	for _, ip := range hostDataIPs {
		if err := EnsureISCSISession(ip); nil != err {
			return err
		}
	}
	return nil
}

func EnsureISCSISession(hostDataIP string) error {

	log.WithField("hostDataIP", hostDataIP).Debug(">>>> osutils.EnsureISCSISession")
	defer log.Debug("<<<< osutils.EnsureISCSISession")

	// Ensure iSCSI is supported on system
	if !ISCSISupported() {
		return errors.New("iSCSI support not detected")
	}

	// Ensure iSCSI session exists for the specified iSCSI portal
	sessionExists, err := iSCSISessionExists(hostDataIP)
	if err != nil {
		return fmt.Errorf("could not check for iSCSI session: %v", err)
	}
	if !sessionExists {

		// Run discovery in case we haven't seen this target from this host
		targets, err := iSCSIDiscovery(hostDataIP)
		if err != nil {
			return fmt.Errorf("could not run iSCSI discovery: %v", err)
		}
		if len(targets) == 0 {
			return errors.New("iSCSI discovery found no targets")
		}

		log.WithFields(log.Fields{
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

				// Log in to target
				err = loginISCSITarget(target.TargetName, target.PortalIP)
				if err != nil {
					return fmt.Errorf("login to iSCSI target failed: %v", err)
				}
			}
		}

		// Recheck to ensure a session is now open
		sessionExists, err = iSCSISessionExists(hostDataIP)
		if err != nil {
			return fmt.Errorf("could not recheck for iSCSI session: %v", err)
		}
		if !sessionExists {
			return fmt.Errorf("expected iSCSI session %v NOT found, please login to the iSCSI portal", hostDataIP)
		}
	}

	log.WithField("hostDataIP", hostDataIP).Debug("Found session to iSCSI portal.")

	return nil
}

// execIscsiadmCommand uses the 'iscsiadm' command to perform operations
func execIscsiadmCommand(args ...string) ([]byte, error) {
	return execCommand("iscsiadm", args...)
}

// execCommand invokes an external process
func execCommand(name string, args ...string) ([]byte, error) {

	log.WithFields(log.Fields{
		"command": name,
		"args":    args,
	}).Debug(">>>> osutils.execCommand.")

	out, err := exec.Command(name, args...).CombinedOutput()

	log.WithFields(log.Fields{
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
func execCommandWithTimeout(name string, timeoutSeconds time.Duration, args ...string) ([]byte, error) {

	timeout := timeoutSeconds * time.Second

	log.WithFields(log.Fields{
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
			log.WithFields(log.Fields{
				"process": name,
				"error":   err,
			}).Error("failed to kill process")
			result = execCommandResult{Output: nil, Error: err}
		} else {
			log.WithFields(log.Fields{
				"process": name,
			}).Error("process killed after timeout")
			result = execCommandResult{Output: nil, Error: errors.New("process killed after timeout")}
		}
	case result = <-done:
		break
	}

	log.WithFields(log.Fields{
		"command": name,
		"output":  sanitizeString(string(result.Output)),
		"error":   result.Error,
	}).Debug("<<<< osutils.execCommandWithTimeout.")

	return result.Output, result.Error
}

func sanitizeString(s string) string {
	// Strip xterm color & movement characters
	s = xtermControlRegex.ReplaceAllString(s, "")
	// Strip trailing newline
	s = strings.TrimSuffix(s, "\n")
	return s
}
