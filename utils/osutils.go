// Copyright 2018 NetApp, Inc. All Rights Reserved.

package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

const IscsiErrNoObjsFound = 21

var XtermControlRegex = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)

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
	out, err := InvokeShellCommand("df", "--output=target,source")
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

func Stat(fileName string) (string, error) {
	statCmd := fmt.Sprintf("stat %v", fileName)
	out, err := InvokeShellCommand("sh", "-c", statCmd)
	return string(out), err
}

// GetInitiatorIqns returns parsed contents of /etc/iscsi/initiatorname.iscsi
func GetInitiatorIqns() ([]string, error) {

	log.Debug(">>>> osutils.GetInitiatorIqns")
	defer log.Debug("<<<< osutils.GetInitiatorIqns")

	var iqns []string
	out, err := InvokeShellCommand("cat", "/etc/iscsi/initiatorname.iscsi")
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

// WaitForPathToExist retries every second, up to numTries times, with increasing backoff, for the specified fileName to show up
func WaitForPathToExist(fileName string, numTries int) bool {

	log.WithField("file", fileName).Debug(">>>> osutils.WaitForPathToExist")
	defer log.Debug("<<<< osutils.WaitForPathToExist")

	for i := 0; i < numTries; i++ {
		_, err := Stat(fileName)
		if err == nil {
			log.WithFields(log.Fields{
				"file":     fileName,
				"attempt":  i,
				"numTries": numTries,
			}).Debug("Path found.")
			return true
		}
		time.Sleep(time.Second * time.Duration(2+i))
	}
	log.WithField("file", fileName).Warning("osutils.waitForPathToExist giving up.")
	return false
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
}

// GetDeviceInfoForLuns executes and parses the output from the 'lsscsi' command
func GetDeviceInfoForLuns() ([]ScsiDeviceInfo, error) {
	/*
		# lsscsi -t
		[0:0:0:0]    disk    sata:                           /dev/sda
		[5:0:0:0]    disk    iqn.1992-08.com.netapp:sn.afbb1784f77411e582f8080027e22798:vs.3,t,0x404  /dev/sdb
		[6:0:0:0]    disk    iqn.1992-08.com.netapp:sn.afbb1784f77411e582f8080027e22798:vs.3,t,0x405  /dev/sdc
		[7:0:0:0]    disk    iqn.1992-08.com.netapp:sn.d724e00bfa0311e582f8080027e22798:vs.4,t,0x407  /dev/sdd
		[8:0:0:0]    disk    iqn.1992-08.com.netapp:sn.d724e00bfa0311e582f8080027e22798:vs.4,t,0x408  /dev/sde
	*/

	log.Debug(">>>> osutils.GetDeviceInfoForLuns")
	defer log.Debug("<<<< osutils.GetDeviceInfoForLuns")

	multipathDetected := MultipathDetected()
	if !multipathDetected {
		log.Debug("Skipping multipath check, /sbin/multipath doesn't exist.")
	}

	out, err := InvokeShellCommand("lsscsi", "-t")
	if err != nil {
		return nil, err
	}

	var info []ScsiDeviceInfo

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {

		d := strings.Fields(line)
		if d == nil || len(d) < 4 {
			log.WithField("line", line).Debug("Could not parse output, skipping line.")
			continue
		}

		log.WithField("device", d).Debug("Processing SCSI device.")

		// Host, Channel, Target, LUN
		s := d[0]
		s = s[1 : len(s)-1]
		scsiBusInfo := strings.Split(s, ":")

		scsiHost := scsiBusInfo[0]
		scsiChannel := scsiBusInfo[1]
		scsiTarget := scsiBusInfo[2]
		scsiLun := scsiBusInfo[3]

		// IQN
		iqn := d[2]
		log.WithField("IQN", iqn).Debug("Found IQN.")

		// Device is the last field in the output
		devFile := d[len(d)-1]

		if !strings.HasPrefix(devFile, "/dev") {
			log.WithField("line", line).Debug("Could not find device file, skipping line.")
			continue
		}
		log.WithField("deviceFile", devFile).Debug("Found device file.")

		// Check to see if there's a multipath device
		multipathDevFile := ""
		if multipathDetected {
			log.Debug("Running multipath check.")

			lsblkCmd := fmt.Sprintf("lsblk %v -n -o name,type -r | grep mpath | cut -f1 -d\\ ", devFile)
			out2, err2 := InvokeShellCommand("sh", "-c", lsblkCmd)
			if err2 != nil {
				// this can be fine, for instance could be a floppy or cd-rom, later logic will error if we never find our device
				log.WithField("deviceFile", devFile).Debug("Could not perform multipath check for device, skipping.")
			} else {
				md := strings.Split(strings.TrimSpace(string(out2)), " ")
				if md != nil && len(md) > 0 && len(md[0]) > 0 {
					if strings.HasPrefix(md[0], "lsblk") || strings.HasSuffix(string(out2),
						"failed to get device path") {
						return nil, fmt.Errorf("problem checking device path while running multipath check for"+
							" device: %v: output: %v", devFile, string(out2))
					}
					log.WithField("md", md).Debug("Found multipath device path.")

					multipathDevFileToCheck := "/dev/mapper/" + md[0]
					_, err3 := Stat(multipathDevFileToCheck)
					if err3 == nil {
						multipathDevFile = multipathDevFileToCheck
					}
				}
			}
		}

		fsType := ""
		if multipathDevFile != "" {
			fsType = GetFSType(multipathDevFile)
		} else {
			fsType = GetFSType(devFile)
		}

		log.WithFields(log.Fields{
			"scsiHost":         scsiHost,
			"scsiChannel":      scsiChannel,
			"scsiTarget":       scsiTarget,
			"scsiLun":          scsiLun,
			"multipathDevFile": multipathDevFile,
			"devFile":          devFile,
			"fsType":           fsType,
			"iqn":              iqn,
		}).Debug("Found SCSI device.")

		info = append(info, ScsiDeviceInfo{
			Host:            scsiHost,
			Channel:         scsiChannel,
			Target:          scsiTarget,
			LUN:             scsiLun,
			MultipathDevice: multipathDevFile,
			Device:          devFile,
			Filesystem:      fsType,
			IQN:             iqn,
		})
	}

	return info, nil
}

// GetDeviceFileFromIscsiPath returns the /dev device for the supplied iscsiPath
func GetDeviceFileFromIscsiPath(iscsiPath string) string {

	log.WithField("iscsiPath", iscsiPath).Debug(">>>> osutils.GetDeviceFileFromIscsiPath")
	defer log.Debug("<<<< osutils.GetDeviceFileFromIscsiPath")

	out, err := InvokeShellCommand("ls", "-la", iscsiPath)
	if err != nil {
		log.WithField("iscsiPath", iscsiPath).Error("Error getting device file from iSCSI path.")
		return ""
	}
	d := strings.Split(string(out), "../../")

	log.WithFields(log.Fields{
		"device":    d,
		"iscsiPath": iscsiPath,
	}).Debug("Found device for iSCSI path.")

	devFile := "/dev/" + d[1]
	devFile = strings.TrimSpace(devFile)

	log.WithField("deviceFile", devFile).Debug("Using device file.")

	return devFile
}

// IscsiSupported returns true if iscsiadm is installed and in the PATH
func IscsiSupported() bool {

	log.Debug(">>>> osutils.IscsiSupported")
	defer log.Debug("<<<< osutils.IscsiSupported")

	_, err := InvokeIscsiadmCommand("-V")
	if err != nil {
		log.Debug("iscsiadm tools not found on this host.")
		return false
	}
	return true
}

// IscsiDiscoveryInfo contains information about discovered iSCSI targets
type IscsiDiscoveryInfo struct {
	Portal     string
	PortalIP   string
	TargetName string
}

// IscsiDiscovery uses the 'iscsiadm' command to perform discovery
func IscsiDiscovery(portal string) ([]IscsiDiscoveryInfo, error) {

	log.WithField("portal", portal).Debug(">>>> osutils.IscsiDiscovery")
	defer log.Debug("<<<< osutils.IscsiDiscovery")

	out, err := InvokeIscsiadmCommand("-m", "discovery", "-t", "sendtargets", "-p", portal)
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

	var discoveryInfo []IscsiDiscoveryInfo

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		a := strings.Fields(l)
		if len(a) >= 2 {

			portalIP := strings.Split(a[0], ":")[0]

			discoveryInfo = append(discoveryInfo, IscsiDiscoveryInfo{
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

// IscsiSessionInfo contains information about iSCSI sessions
type IscsiSessionInfo struct {
	SID        string
	Portal     string
	PortalIP   string
	TargetName string
}

// GetIscsiSessionInfo parses output from 'iscsiadm -m session' and returns the parsed output
func GetIscsiSessionInfo() ([]IscsiSessionInfo, error) {

	log.Debug(">>>> osutils.GetIscsiSessionInfo")
	defer log.Debug("<<<< osutils.GetIscsiSessionInfo")

	out, err := InvokeIscsiadmCommand("-m", "session")
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == IscsiErrNoObjsFound {
			log.Debug("No iSCSI session found.")
			return []IscsiSessionInfo{}, nil
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

	var sessionInfo []IscsiSessionInfo

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {

		a := strings.Fields(l)
		if len(a) > 3 {
			sid := a[1]
			sid = sid[1 : len(sid)-1]

			portalIP := strings.Split(a[2], ":")[0]
			sessionInfo = append(sessionInfo, IscsiSessionInfo{
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

// GetIscsiHostInfo parses output from 'iscsiadm -m host' and returns the parsed output
func GetIscsiHostInfo() ([]string, error) {

	log.Debug(">>>> osutils.GetIscsiHostInfo")
	defer log.Debug("<<<< osutils.GetIscsiHostInfo")

	out, err := InvokeIscsiadmCommand("-m", "host")
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() == IscsiErrNoObjsFound {
			log.Debug("No iSCSI hosts found.")
			return make([]string, 0), nil
		} else {
			log.WithField("error", err).Error("Problem checking iSCSI hosts.")
			return nil, err
		}
	}

	/*
	   # iscsiadm -m host
	   tcp: [33] 192.168.228.16,[<empty>],<empty> <empty>
	   tcp: [34] 192.168.228.16,[<empty>],<empty> <empty>

	   a[0]==tcp:
	   a[1]==[33]
	   a[2]==192.168.228.16,[<empty>],<empty>
	   a[3]==<empty>
	*/

	var hosts []string

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {

		a := strings.Fields(l)
		if len(a) > 3 {
			host := a[1]
			host = host[1 : len(host)-1]
			hosts = append(hosts, host)

			log.WithFields(log.Fields{"Host": host}).Debug("Adding iSCSI host.")
		}
	}

	return hosts, nil
}

// IscsiTargetInfo structure for usage with the iscsiadm command
type IscsiTargetInfo struct {
	IP        string
	Port      string
	Portal    string
	Iqn       string
	Lun       string
	Device    string
	Discovery string
}

// IscsiDisableDelete logout from the supplied target and remove the iSCSI device
func IscsiDisableDelete(tgt *IscsiTargetInfo) error {

	log.WithField("target", tgt).Debug(">>>> osutils.IscsiDisableDelete")
	defer log.Debug("<<<< osutils.IscsiDisableDelete")

	_, err := InvokeIscsiadmCommand("-m", "node", "-T", tgt.Iqn, "--portal", tgt.IP, "-u")
	if err != nil {
		log.WithField("error", err).Debug("Error during iSCSI logout.")
	}

	_, err = InvokeIscsiadmCommand("-m", "node", "-o", "delete", "-T", tgt.Iqn)
	return err
}

// IscsiSessionExists checks to see if a session exists to the sepecified portal
func IscsiSessionExists(portal string) (bool, error) {

	log.Debug(">>>> osutils.IscsiSessionExists")
	defer log.Debug("<<<< osutils.IscsiSessionExists")

	sessionInfo, err := GetIscsiSessionInfo()
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

// IscsiRescan uses the 'rescan-scsi-bus' command to perform rescanning of the SCSI bus
func IscsiRescan(remove bool) (err error) {

	log.Debug(">>>> osutils.IscsiRescan")
	defer log.Debug("<<<< osutils.IscsiRescan")

	defer UdevSettle()

	// Get iSCSI hosts so we don't have to rescan the entire SCSI subsystem
	hosts, err := GetIscsiHostInfo()
	if err != nil {
		log.WithField("error", err).Error("Could not find iSCSI hosts, scanning everything.")
		hosts = []string{}
	}

	// Build arguments for rescan command
	rescanArgs := []string{"--alltargets"}
	if remove {
		rescanArgs = append(rescanArgs, "--remove")
	}
	if len(hosts) > 0 {
		rescanArgs = append(rescanArgs, "--hosts="+strings.Join(hosts, ","))
	}
	if log.GetLevel() == log.DebugLevel {
		rescanArgs = append(rescanArgs, "-d")
	}

	// look for version of rescan-scsi-bus in known locations
	var rescanCommands = []string{
		"/usr/bin/rescan-scsi-bus.sh",
		"/sbin/rescan-scsi-bus",
		"/sbin/rescan-scsi-bus.sh",
		"/bin/rescan-scsi-bus.sh",
	}

	for _, rescanCommand := range rescanCommands {
		_, err = os.Lstat(rescanCommand)
		// The command exists in this location
		if !os.IsNotExist(err) {
			log.WithField("command", rescanCommand).Debug("Found SCSI rescan command.")
			_, rescanErr := InvokeShellCommand(rescanCommand, rescanArgs...)
			if rescanErr != nil {
				log.Error("Could not rescan SCSI bus.")
				return rescanErr
			} else {
				return
			}
		}
	}

	// Attempt to find the binary on the path
	_, err = InvokeShellCommand("rescan-scsi-bus.sh", rescanArgs...)
	if err != nil {
		log.Error("Could not rescan SCSI bus.")
		return
	}

	log.Warn("Unable to find rescan-scsi-bus command!")
	return
}

// UdevSettle invokes the 'udevadm settle' command
func UdevSettle() error {
	// creating new storage and attaching it to a host can trigger a ripple of udev activity.

	log.Debug(">>>> osutils.UdevSettle")
	defer log.Debug("<<<< osutils.UdevSettle")

	// back-to-back invoke /sbin/multipath, if it exists, to make sure that device discovery has settled down post rescan
	for i := 0; i < 2; i++ {
		Multipath()
		Multipath()

		// attempt to wait for inflight udev events to complete (should eventually timeout if they never complete)
		_, err := InvokeShellCommand("udevadm", "settle")
		if err != nil {
			return err
		}
	}

	return nil
}

// Multipath invokes the 'multipath' command
func Multipath() error {

	log.Debug(">>>> osutils.Multipath")
	defer log.Debug("<<<< osutils.Multipath")

	if !MultipathDetected() {
		log.Debug("Skipping multipath command, /sbin/multipath doesn't exist.")
		return nil
	}

	_, err := InvokeShellCommand("multipath")
	if err != nil {
		// nothing to really do if it generates an error but log and return it
		log.Debug("Error encountered in multipath command.")
	}
	return err
}

// MultipathFlush invokes the 'multipath' commands to flush paths that have been removed
func MultipathFlush() error {

	log.Debug(">>>> osutils.MultipathFlush")
	defer log.Debug("<<<< osutils.MultipathFlush")

	_, err := InvokeShellCommand("multipath", "-F")
	if err != nil {
		// nothing to really do if it generates an error but log and return it
		log.Debug("Error encountered in multipath flush unused paths command.")
	}
	return err
}

var mpChecked = false
var mpDetected = false
var mpMutex sync.Mutex

// MultipathDetected returns true if /sbin/multipath is installed and in the PATH
func MultipathDetected() bool {

	mpMutex.Lock()
	defer mpMutex.Unlock()

	if !mpChecked {
		_, errStat := Stat("/sbin/multipath")
		if errStat != nil {
			mpDetected = false
		} else {
			mpDetected = true
		}
		mpChecked = true
	}
	return mpDetected
}

// GetFSType returns the filesystem for the supplied device
func GetFSType(device string) string {

	log.WithField("device", device).Debug(">>>> osutils.GetFSType")
	defer log.Debug("<<<< osutils.GetFSType")

	fsType := ""
	out, err := InvokeShellCommand("blkid", device)
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

// FormatVolume creates a filesystem for the supplied device of the supplied type
func FormatVolume(device, fstype string) error {

	log.WithFields(log.Fields{
		"device": device,
		"fsType": fstype,
	}).Debug(">>>> osutils.FormatVolume")
	defer log.Debug("<<<< osutils.FormatVolume")

	var err error

	switch fstype {
	case "xfs":
		_, err = InvokeShellCommand("mkfs.xfs", "-f", device)
	case "ext3":
		_, err = InvokeShellCommand("mkfs.ext3", "-F", device)
	case "ext4":
		_, err = InvokeShellCommand("mkfs.ext4", "-F", device)
	default:
		return fmt.Errorf("unsupported file system type: %s", fstype)
	}

	return err
}

// Mount attaches the supplied device at the supplied location
func Mount(device, mountpoint string) error {

	log.WithFields(log.Fields{
		"device":     device,
		"mountpoint": mountpoint,
	}).Debug(">>>> osutils.Mount")
	defer log.Debug("<<<< osutils.Mount")

	_, err := InvokeShellCommand("mkdir", mountpoint)
	if err != nil {
		log.Warning("Mkdir failed.")
	}
	_, err = InvokeShellCommand("mount", device, mountpoint)
	if err != nil {
		log.Error("Mount failed.")
	}
	return err
}

// Umount detaches from the supplied location
func Umount(mountpoint string) error {

	log.WithField("mountpoint", mountpoint).Debug(">>>> osutils.Umount")
	defer log.Debug("<<<< osutils.Umount")

	_, err := InvokeShellCommand("umount", mountpoint)
	if err != nil {
		log.Error("Umount failed.")
	}
	return err
}

// Login to iSCSI target
func LoginIscsiTarget(iqn, portal string) error {

	log.WithFields(log.Fields{
		"IQN":    iqn,
		"Portal": portal,
	}).Debug(">>>> osutils.LoginIscsiTarget")
	defer log.Debug("<<<< osutils.LoginIscsiTarget")

	args := []string{"-m", "node", "-T", iqn, "-l", "-p", portal + ":3260"}

	if _, err := InvokeIscsiadmCommand(args...); err != nil {
		log.WithField("error", err).Error("Error logging in to iSCSI target.")
		return err
	}
	return nil
}

// LoginWithChap will login to the iSCSI target with the supplied credentials
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
	if _, err := InvokeIscsiadmCommand(createArgs...); err != nil {
		log.Error("Error running iscsiadm node create.")
		return err
	}

	authMethodArgs := append(args, []string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
	if _, err := InvokeIscsiadmCommand(authMethodArgs...); err != nil {
		log.Error("Error running iscsiadm set authmethod.")
		return err
	}

	authUserArgs := append(args, []string{"--op=update", "--name", "node.session.auth.username", "--value=" + username}...)
	if _, err := InvokeIscsiadmCommand(authUserArgs...); err != nil {
		log.Error("Error running iscsiadm set authuser.")
		return err
	}

	authPasswordArgs := append(args, []string{"--op=update", "--name", "node.session.auth.password", "--value=" + password}...)
	if _, err := InvokeIscsiadmCommand(authPasswordArgs...); err != nil {
		log.Error("Error running iscsiadm set authpassword.")
		return err
	}

	loginArgs := append(args, []string{"--login"}...)
	if _, err := InvokeIscsiadmCommand(loginArgs...); err != nil {
		log.Error("Error running iscsiadm login.")
		return err
	}

	return nil
}

func EnsureIscsiSession(hostDataIP string) error {

	log.WithField("hostDataIP", hostDataIP).Debug(">>>> osutils.EnsureIscsiSession")
	defer log.Debug("<<<< osutils.EnsureIscsiSession")

	// Ensure iSCSI is supported on system
	if !IscsiSupported() {
		return errors.New("iSCSI support not detected")
	}

	// Ensure iSCSI session exists for the specified iSCSI portal
	sessionExists, err := IscsiSessionExists(hostDataIP)
	if err != nil {
		return fmt.Errorf("could not check for iSCSI session: %v", err)
	}
	if !sessionExists {

		// Run discovery in case we haven't seen this target from this host
		targets, err := IscsiDiscovery(hostDataIP)
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
				err = LoginIscsiTarget(target.TargetName, target.PortalIP)
				if err != nil {
					return fmt.Errorf("login to iSCSI target failed: %v", err)
				}
			}
		}

		// Recheck to ensure a session is now open
		sessionExists, err = IscsiSessionExists(hostDataIP)
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

// InvokeIscsiadmCommand uses the 'iscsiadm' command to perform operations
func InvokeIscsiadmCommand(args ...string) ([]byte, error) {
	return InvokeShellCommand("iscsiadm", args...)
}

// InvokeShellCommand invokes an external shell command
func InvokeShellCommand(name string, args ...string) ([]byte, error) {

	log.WithFields(log.Fields{
		"command": name,
		"args":    args,
	}).Debug(">>>> osutils.InvokeShellCommand.")

	out, err := exec.Command(name, args...).CombinedOutput()

	log.WithFields(log.Fields{
		"command": name,
		"output":  sanitizeString(string(out)),
		"error":   err,
	}).Debug("<<<< osutils.InvokeShellCommand.")

	return out, err
}

func sanitizeString(s string) string {
	// Strip xterm color & movement characters
	s = XtermControlRegex.ReplaceAllString(s, "")
	// Strip trailing newline
	s = strings.TrimSuffix(s, "\n")
	return s
}
