// Copyright 2018 NetApp, Inc. All Rights Reserved.

package utils

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"
)

const iSCSIErrNoObjsFound = 21
const iSCSIDeviceDiscoveryTimeoutSecs = 90
const multipathDeviceDiscoveryTimeoutSecs = 90

var xtermControlRegex = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)
var pidRunningRegex = regexp.MustCompile(`pid \d+ running`)
var pidRegex = regexp.MustCompile(`^\d+$`)
var chrootPathPrefix string

func init() {

	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		chrootPathPrefix = "/host"
	} else {
		chrootPathPrefix = ""
	}
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

	var iqns []string
	out, err := execCommand("cat", "/etc/iscsi/initiatorname.iscsi")
	if err != nil {
		log.Error("Error gathering initiator names.")
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

// PathExists returns true if the file/directory at the specified path exists,
// false otherwise or if an error occurs.
func PathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

// GetDevicePathsForISCSIPortals synthesizes the disk-by-path device names for an iSCSI LUN.
func GetDevicePathsForISCSIPortals(lunID int, iSCSINodeName string, iSCSIInterfaces []string) []string {
	paths := make([]string, 0)
	for _, iSCSIInterface := range iSCSIInterfaces {
		path := fmt.Sprintf("/dev/disk/by-path/ip-%s-iscsi-%s-lun-%d", iSCSIInterface, iSCSINodeName, lunID)
		paths = append(paths, path)
	}
	return paths
}

// RescanTargetAndWaitForDevice rescans all paths to a specific LUN and waits until all
// SCSI disk-by-path devices for that LUN are present on the host.
func RescanTargetAndWaitForDevice(lunID int, iSCSINodeName string, iSCSIInterfaces []string) error {

	fields := log.Fields{
		"lunID":           lunID,
		"iSCSINodeName":   iSCSINodeName,
		"iSCSIInterfaces": iSCSIInterfaces,
	}
	log.WithFields(fields).Debug(">>>> osutils.RescanTargetAndWaitForDevice")
	defer log.WithFields(fields).Debug("<<<< osutils.RescanTargetAndWaitForDevice")

	maxDuration := iSCSIDeviceDiscoveryTimeoutSecs * time.Second

	paths := GetDevicePathsForISCSIPortals(lunID, iSCSINodeName, iSCSIInterfaces)

	if err := ISCSIRescanTargetLUN(lunID, iSCSINodeName); err != nil {
		log.WithField("rescanError", err).Error("Could not rescan for new LUN.")
	}

	checkPathsExist := func() error {

		// Check if all paths present, and return nil (success) if so
		anyMissingPaths := false
		for _, path := range paths {
			if !PathExists(path) {
				anyMissingPaths = true
			}
		}
		if anyMissingPaths {
			return errors.New("one or more devices aren't yet present")
		}
		return nil
	}

	devicesNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Devices not yet present, waiting.")
	}

	deviceBackoff := backoff.NewExponentialBackOff()
	deviceBackoff.InitialInterval = 1 * time.Second
	deviceBackoff.Multiplier = 1.3
	deviceBackoff.RandomizationFactor = 0.1
	deviceBackoff.MaxElapsedTime = maxDuration

	// Run the check/rescan using an exponential backoff
	if err := backoff.RetryNotify(checkPathsExist, deviceBackoff, devicesNotify); err != nil {
		log.Warnf("Could not find all devices after %3.2f seconds.", maxDuration.Seconds())

		// In the case of a failure, log info about what devices are present
		execCommand("ls", "-al", "/dev")
		execCommand("ls", "-al", "/dev/mapper")
		execCommand("ls", "-al", "/dev/disk/by-path")
		execCommand("lsscsi")
		execCommand("lsscsi", "-t")
		execCommand("free")
		return err
	}

	log.Debug("All devices found.")
	return nil
}

// ScsiDeviceInfo contains information about SCSI devices
type ScsiDeviceInfo struct {
	Host            string
	Channel         string
	Target          string
	LUN             string
	Device          string
	MultipathDevice string
	Filesystem      string
	IQN             string
	DeviceNameMap   map[string]string
	HostSessionMap  map[int]int
}

// GetDeviceInfoForLUN finds iSCSI devices using /dev/disk/by-path values.  This method should be
// called after calling RescanTargetAndWaitForDevice so that the device paths are known to exist.
func GetDeviceInfoForLUN(lunID int, iSCSINodeName string, iSCSIInterfaces []string) (*ScsiDeviceInfo, error) {

	fields := log.Fields{
		"lunID":           lunID,
		"iSCSINodeName":   iSCSINodeName,
		"iSCSIInterfaces": iSCSIInterfaces,
	}
	log.WithFields(fields).Debug(">>>> osutils.GetDeviceInfoForLUN")
	defer log.WithFields(fields).Debug("<<<< osutils.GetDeviceInfoForLUN")

	if len(iSCSIInterfaces) == 0 {
		return nil, errors.New("must specify one or more iSCSI interfaces")
	}

	paths := GetDevicePathsForISCSIPortals(lunID, iSCSINodeName, iSCSIInterfaces)
	deviceNameMap := make(map[string]string)

	for _, path := range paths {

		// Ensure all paths exist
		if !PathExists(path) {
			return nil, fmt.Errorf("path %s does not exist", path)
		}

		// Get the device name from the path and save it
		if deviceName, err := findDeviceForPath(path); err != nil {
			return nil, err
		} else {
			deviceNameMap[path] = deviceName
		}
	}

	multipathDevFile := waitForMultipathDevice(paths)

	fsType := ""
	if multipathDevFile != "" {
		fsType = getFSType(multipathDevFile)
	} else {
		fsType = getFSType(paths[0])
	}

	hostSessionMap := getISCSIHostSessionMapForTarget(iSCSINodeName)

	log.WithFields(log.Fields{
		"LUN":              strconv.Itoa(lunID),
		"multipathDevFile": multipathDevFile,
		"devFile":          paths[0],
		"fsType":           fsType,
		"deviceNames":      deviceNameMap,
		"hostSessionMap":   hostSessionMap,
	}).Debug("Found SCSI device.")

	info := &ScsiDeviceInfo{
		LUN:             strconv.Itoa(lunID),
		MultipathDevice: multipathDevFile,
		Device:          paths[0],
		Filesystem:      fsType,
		IQN:             iSCSINodeName,
		DeviceNameMap:   deviceNameMap,
		HostSessionMap:  hostSessionMap,
	}

	return info, nil
}

// waitForMultipathDevice accepts a list of /dev/disk/by-path device paths and waits until
// a multipath device is present for at least one of those.  It returns the name of the
// multipath device, or an empty string if multipathd isn't running or there is only one path.
func waitForMultipathDevice(devices []string) string {

	fields := log.Fields{"devices": devices}
	log.WithFields(fields).Debug(">>>> osutils.waitForMultipathDevice")
	defer log.WithFields(fields).Debug("<<<< osutils.waitForMultipathDevice")

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
	multipathDeviceBackoff.Multiplier = 1.3
	multipathDeviceBackoff.RandomizationFactor = 0.1
	multipathDeviceBackoff.MaxElapsedTime = maxDuration

	// Run the check/rescan using an exponential backoff
	if err := backoff.RetryNotify(checkMultipathDeviceExists, multipathDeviceBackoff, deviceNotify); err != nil {
		log.Warnf("Could not find multipath device after %3.2f seconds.", maxDuration.Seconds())
	} else {
		log.WithField("multipathDevice", multipathDevice).Debug("Multipath device found.")
	}
	return multipathDevice
}

// findMultipathDeviceForDevice finds the devicemapper parent of a device name like /dev/sdx.
func findMultipathDeviceForDevice(device string) string {

	log.WithField("device", device).Debug(">>>> osutils.findMultipathDeviceForDevice")
	defer log.WithField("device", device).Debug("<<<< osutils.findMultipathDeviceForDevice")

	disk, err := findDeviceForPath(device)
	if err != nil {
		log.WithField("error", err).Debug("Could not find multipath device for device.")
		return ""
	}
	sysPath := "/sys/block/"
	if dirs, err := ioutil.ReadDir(sysPath); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.HasPrefix(name, "dm-") {
				if _, err1 := os.Lstat(sysPath + name + "/slaves/" + disk); err1 == nil {
					log.WithField("device", "/dev/"+name).Debug("Found multipath device for device.")
					return "/dev/" + name
				}
			}
		}
	}

	log.WithField("device", device).Debug("Could not find multipath device for device.")
	return ""
}

// findDeviceForPath finds the underlaying disk for a linked path such as /dev/disk/by-path/XXXX or
// /dev/mapper/XXXX, returning sdX or hdX, etc. If /dev/sdX is passed in then sdX will be returned.
func findDeviceForPath(path string) (string, error) {

	log.Debug(">>>> osutils.findDeviceForPath")
	defer log.Debug("<<<< osutils.findDeviceForPath")

	devicePath, err := filepath.EvalSymlinks(path)
	if err != nil {
		log.WithField("error", err).Debug("Could not find device for path.")
		return "", err
	}
	// if path /dev/hdX split into "", "dev", "hdX" then we will return just the last part
	parts := strings.Split(devicePath, "/")
	if len(parts) == 3 && strings.HasPrefix(parts[1], "dev") {
		log.WithFields(log.Fields{"device": parts[2], "path": path}).Debug("Found device from path.")
		return parts[2], nil
	}
	log.Debug("Illegal path for device.")
	return "", errors.New("illegal path for device " + devicePath)
}

// PrepareDeviceForRemoval informs Linux that a device will be removed.
func PrepareDeviceForRemoval(lunID int, iSCSINodeName string, iSCSIInterfaces []string) {

	fields := log.Fields{
		"lunID":            lunID,
		"iSCSINodeName":    iSCSINodeName,
		"iSCSIInterfaces":  iSCSIInterfaces,
		"chrootPathPrefix": chrootPathPrefix,
	}
	log.WithFields(fields).Debug(">>>> osutils.PrepareDeviceForRemoval")
	defer log.WithFields(fields).Debug("<<<< osutils.PrepareDeviceForRemoval")

	deviceInfo, err := GetDeviceInfoForLUN(lunID, iSCSINodeName, iSCSIInterfaces)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"lunID": lunID,
		}).Info("Could not get device info for removal, skipping host removal steps.")
		return
	}

	// Flush multipath device
	multipathFlushDevice(deviceInfo)

	// Flush devices
	flushDevice(deviceInfo)

	// Remove device
	removeDevice(deviceInfo)

	// Give the host a chance to fully process the removal
	time.Sleep(time.Second)
}

// GetDeviceFileFromISCSIPath returns the /dev device for the supplied iSCSIPath.
func GetDeviceFileFromISCSIPath(iSCSIPath string) string {

	logFields := log.Fields{"iSCSIPath": iSCSIPath}
	log.WithFields(logFields).Debug(">>>> osutils.GetDeviceFileFromISCSIPath")
	defer log.WithFields(logFields).Debug("<<<< osutils.GetDeviceFileFromISCSIPath")

	if !strings.HasPrefix(iSCSIPath, "/dev/") {
		log.WithFields(logFields).Warning("Not a /dev path.")
	}

	devicePath, err := filepath.EvalSymlinks(iSCSIPath)
	if err != nil {
		log.Errorf("%v", err)
		return ""
	}

	log.WithFields(log.Fields{
		"devicePath": devicePath,
		"iSCSIPath":  iSCSIPath,
	}).Debug("Found device for iSCSI path.")

	return devicePath
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
	*/

	var discoveryInfo []ISCSIDiscoveryInfo

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		a := strings.Fields(l)
		if len(a) >= 2 {

			portalIP := strings.Split(a[0], ":")[0]

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

			portalIP := strings.Split(a[2], ":")[0]
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

// ISCSITargetInfo structure for usage with the iscsiadm command.
type ISCSITargetInfo struct {
	IP        string
	Port      string
	Portal    string
	Iqn       string
	Lun       string
	Device    string
	Discovery string
}

// ISCSIDisableDelete logs out from the supplied target and removes the iSCSI device.
func ISCSIDisableDelete(tgt *ISCSITargetInfo) error {

	log.WithField("target", tgt).Debug(">>>> osutils.ISCSIDisableDelete")
	defer log.Debug("<<<< osutils.ISCSIDisableDelete")

	_, err := execIscsiadmCommand("-m", "node", "-T", tgt.Iqn, "--portal", tgt.IP, "-u")
	if err != nil {
		log.WithField("error", err).Debug("Error during iSCSI logout.")
	}

	_, err = execIscsiadmCommand("-m", "node", "-o", "delete", "-T", tgt.Iqn)
	return err
}

// ISCSISessionExists checks to see if a session exists to the specified portal.
func ISCSISessionExists(portal string) (bool, error) {

	log.Debug(">>>> osutils.ISCSISessionExists")
	defer log.Debug("<<<< osutils.ISCSISessionExists")

	sessionInfo, err := getISCSISessionInfo()
	if err != nil {
		log.WithField("error", err).Error("Problem checking iSCSI sessions.")
		return false, err
	}

	for _, e := range sessionInfo {
		if e.PortalIP == portal {
			return true, nil
		}
	}

	return false, nil
}

// ISCSIRescanTargetLUN rescans a single LUN on an iSCSI target.
func ISCSIRescanTargetLUN(lunID int, iSCSINodeName string) error {

	fields := log.Fields{"iSCSINodeName": iSCSINodeName, "lunID": lunID}
	log.WithFields(fields).Debug(">>>> osutils.ISCSIRescanTargetLUN")
	defer log.WithFields(fields).Debug("<<<< osutils.ISCSIRescanTargetLUN")

	hostSessionMap := getISCSIHostSessionMapForTarget(iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	log.WithField("hostSessionMap", hostSessionMap).Debug("Built iSCSI host/session map.")

	var (
		f   *os.File
		err error
	)

	for hostNumber := range hostSessionMap {

		filename := fmt.Sprintf("/sys/class/scsi_host/host%d/scan", hostNumber)
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
		}).Debug("Invoked single-LUN rescan.")
	}

	return nil
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

	sysPath := "/sys/class/iscsi_host/"
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
				log.WithField("error", err).Errorf("Could not read %f", devicePath)
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

// multipathFlushDevice invokes the 'multipath' commands to flush paths for a single device.
func multipathFlushDevice(deviceInfo *ScsiDeviceInfo) {

	log.WithField("device", deviceInfo.MultipathDevice).Debug(">>>> osutils.multipathFlushDevice")
	defer log.Debug("<<<< osutils.multipathFlushDevice")

	if deviceInfo.MultipathDevice == "" {
		return
	}

	_, err := execCommandWithTimeout("multipath", 30, "-f", deviceInfo.MultipathDevice)
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

	for diskByPath := range deviceInfo.DeviceNameMap {
		_, err := execCommandWithTimeout("blockdev", 5, "--flushbufs", diskByPath)
		if err != nil {
			// nothing to do if it generates an error but log it
			log.WithFields(log.Fields{
				"device": diskByPath,
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

	for _, deviceName := range deviceInfo.DeviceNameMap {

		filename := fmt.Sprintf("/sys/block/%s/device/delete", deviceName)
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

// FormatVolume creates a filesystem for the supplied device of the supplied type.
func FormatVolume(device, fstype string) error {

	logFields := log.Fields{"device": device, "fsType": fstype}
	log.WithFields(logFields).Debug(">>>> osutils.FormatVolume")
	defer log.WithFields(logFields).Debug("<<<< osutils.FormatVolume")

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

	// Run the check/rescan using an exponential backoff
	if err := backoff.RetryNotify(formatVolume, formatBackoff, formatNotify); err != nil {
		log.Warnf("Could not format device after %3.2f seconds.", maxDuration.Seconds())
		return err
	}

	log.WithFields(logFields).Info("Device formatted.")
	return nil
}

// Mount attaches the supplied device at the supplied location.
func Mount(device, mountpoint string) (err error) {

	log.WithFields(log.Fields{
		"device":     device,
		"mountpoint": mountpoint,
	}).Debug(">>>> osutils.Mount")
	defer log.Debug("<<<< osutils.Mount")

	if _, err = execCommand("mkdir", "-p", mountpoint); err != nil {
		log.WithField("error", err).Warning("Mkdir failed.")
	}
	if _, err = execCommand("mount", device, mountpoint); err != nil {
		log.WithField("error", err).Error("Mount failed.")
	}
	return
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

// LoginISCSITarget logs in to an iSCSI target.
func LoginISCSITarget(iqn, portal string) error {

	log.WithFields(log.Fields{
		"IQN":    iqn,
		"Portal": portal,
	}).Debug(">>>> osutils.LoginISCSITarget")
	defer log.Debug("<<<< osutils.LoginISCSITarget")

	args := []string{"-m", "node", "-T", iqn, "-l", "-p", portal + ":3260"}

	if _, err := execIscsiadmCommand(args...); err != nil {
		log.WithField("error", err).Error("Error logging in to iSCSI target.")
		return err
	}
	return nil
}

// LoginWithChap will login to the iSCSI target with the supplied credentials.
func LoginWithChap(tiqn, portal, username, password, iface string, logSensitiveInfo bool) error {

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
	log.WithFields(logFields).Debug(">>>> osutils.LoginWithChap")
	defer log.Debug("<<<< osutils.LoginWithChap")

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

func EnsureISCSISession(hostDataIP string) error {

	log.WithField("hostDataIP", hostDataIP).Debug(">>>> osutils.EnsureISCSISession")
	defer log.Debug("<<<< osutils.EnsureISCSISession")

	// Ensure iSCSI is supported on system
	if !ISCSISupported() {
		return errors.New("iSCSI support not detected")
	}

	// Ensure iSCSI session exists for the specified iSCSI portal
	sessionExists, err := ISCSISessionExists(hostDataIP)
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
			if target.PortalIP == hostDataIP {
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
				err = LoginISCSITarget(target.TargetName, target.PortalIP)
				if err != nil {
					return fmt.Errorf("login to iSCSI target failed: %v", err)
				}
			}
		}

		// Recheck to ensure a session is now open
		sessionExists, err = ISCSISessionExists(hostDataIP)
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
