// Copyright 2025 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/afero"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/osutils"
)

const (
	SessionSourceTrackingInfo = "trackingInfo"
)

var (
	iqnRegex       = regexp.MustCompile(`^\s*InitiatorName\s*=\s*(?P<iqn>\S+)(|\s+.*)$`)
	mountClient, _ = mount.New()
	IscsiUtils     = iscsi.NewReconcileUtils()
	devicesClient  = devices.New()
	IscsiClient    = iscsi.NewDetailed(osutils.ChrootPathPrefix, command, iscsi.DefaultSelfHealingExclusion,
		osutils.New(),
		devicesClient, filesystem.New(mountClient), mountClient, IscsiUtils, afero.Afero{Fs: afero.NewOsFs()}, nil)

	// perNodeIgroupRegex is used to ensure an igroup meets the following format:
	// <up to and including 59 characters of a container orchestrator node name>-<36 characters of trident version uuid>
	// ex: Kubernetes-NodeA-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z
	perNodeIgroupRegex = regexp.MustCompile(`^[0-9A-z\-.]{1,59}-[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}`)

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
		serial, err := devicesClient.GetLunSerial(ctx, path)
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

	multipathDeviceUUID, err := devicesClient.GetMultipathDeviceUUID(multipathDevice)
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
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)
	for hostNumber := range hostSessionMap {
		deviceAddresses = append(deviceAddresses, models.ScsiDeviceAddress{
			Host:    strconv.Itoa(hostNumber),
			Channel: models.ScanSCSIDeviceAddressZero,
			Target:  models.ScanSCSIDeviceAddressZero,
			LUN:     strconv.Itoa(lunID),
		})
	}

	if err := devicesClient.ScanTargetLUN(ctx, deviceAddresses); err != nil {
		Logc(ctx).WithField("scanError", err).Error("Could not scan for new LUN.")
	}

	return nil
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

// InitiateScanForAllLUNs scans all paths to each of the LUNs passed.
func InitiateScanForAllLUNs(ctx context.Context, iSCSINodeName string) error {
	fields := LogFields{"iSCSINodeName": iSCSINodeName}

	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.InitiateScanForAllLUNs")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.InitiateScanForAllLUNs")

	// Setting lunID to - so that all the LUNs are scanned.
	lunID := models.ScanAllSCSIDeviceAddress

	hostSessionMap := IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, iSCSINodeName)
	if len(hostSessionMap) == 0 {
		return fmt.Errorf("no iSCSI hosts found for target %s", iSCSINodeName)
	}

	Logc(ctx).WithField("hostSessionMap", hostSessionMap).Debug("Built iSCSI host/session map.")
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)
	for hostNumber := range hostSessionMap {
		deviceAddresses = append(deviceAddresses, models.ScsiDeviceAddress{
			Host:    strconv.Itoa(hostNumber),
			Channel: models.ScanSCSIDeviceAddressZero,
			Target:  models.ScanSCSIDeviceAddressZero,
			LUN:     lunID,
		})
	}

	if err := devicesClient.ScanTargetLUN(ctx, deviceAddresses); err != nil {
		Logc(ctx).WithError(err).Error("Could not scan for new LUN.")
	}

	return nil
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
