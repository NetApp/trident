// Copyright 2025 NetApp, Inc. All Rights Reserved.

package iscsi

//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_reconcile_utils.go github.com/netapp/trident/utils/iscsi IscsiReconcileUtils

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

type IscsiReconcileUtils interface {
	GetISCSIHostSessionMapForTarget(context.Context, string) map[int]int
	DiscoverSCSIAddressMapForTarget(ctx context.Context, targetIQN string) (map[string]models.ScsiDeviceAddress, error)
	GetSysfsBlockDirsForLUN(int, map[int]int) []string
	GetDevicesForLUN(paths []string) ([]string, error)
	ReconcileISCSIVolumeInfo(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (bool, error)
}

type IscsiReconcileHelper struct {
	chrootPathPrefix string
	osClient         OS
	osFs             afero.Afero
}

func NewReconcileUtils() IscsiReconcileUtils {
	return NewReconcileUtilsDetailed(osutils.ChrootPathPrefix, osutils.New(), afero.Afero{Fs: afero.NewOsFs()})
}

func NewReconcileUtilsDetailed(chrootPathPrefix string, osClient OS, osFs afero.Afero) IscsiReconcileUtils {
	return &IscsiReconcileHelper{
		chrootPathPrefix: chrootPathPrefix,
		osClient:         osClient,
		osFs:             osFs,
	}
}

// ReconcileISCSIVolumeInfo returns true if any of the expected conditions for a present volume are true (e.g. the
// expected LUN exists).
func (h *IscsiReconcileHelper) ReconcileISCSIVolumeInfo(
	ctx context.Context, trackingInfo *models.VolumeTrackingInfo,
) (bool, error) {
	pubInfo := trackingInfo.VolumePublishInfo
	lun := int(pubInfo.IscsiLunNumber)
	iqn := pubInfo.IscsiTargetIQN

	sessionMap := h.GetISCSIHostSessionMapForTarget(ctx, iqn)
	if len(sessionMap) > 0 {
		return true, nil
	}

	paths := h.GetSysfsBlockDirsForLUN(lun, sessionMap)
	devices, err := h.GetDevicesForLUN(paths)
	if err != nil {
		return false, err
	}
	if len(devices) > 0 {
		return true, nil
	}

	return false, nil
}

// DiscoverSCSIAddressMapForTarget creates a map of unique "host:channel:targetID" to ScsiDeviceAddresses that
// exist for active sessions on a given target IQN. We can rely on "host:channel:targetID" for our keys safely because
// we filter by target IQN. The resulting map should be used with a set of LUN IDs to initiate precise LUN scanning.
func (h *IscsiReconcileHelper) DiscoverSCSIAddressMapForTarget(
	ctx context.Context, targetIQN string,
) (map[string]models.ScsiDeviceAddress, error) {
	fields := LogFields{"iSCSINodeName": targetIQN}
	Logc(ctx).WithFields(fields).Debug(">>>> iscsi.DiscoverSCSIAddressMapForTarget")
	defer Logc(ctx).WithFields(fields).Debug("<<<< iscsi.DiscoverSCSIAddressMapForTarget")

	// deviceMap is a map of "h:c:t" -> ScsiDeviceAddress.
	deviceMap := make(map[string]models.ScsiDeviceAddress)

	// Read in everything under: '/sys/class/scsi_host/'.
	scsiHostPath := filepath.Join(h.chrootPathPrefix, "sys", "class", "scsi_host")
	hostEntries, err := h.osFs.ReadDir(scsiHostPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts; %w", err)
	}

	// Search through each dir under: '/sys/class/scsi_host/'.
	for _, hostEntry := range hostEntries {
		hostName := hostEntry.Name() // example: "host10" from "/sys/class/scsi_host/host10"
		if !strings.HasPrefix(hostName, "host") {
			continue
		}

		// Read in all dirs under: '/sys/class/scsi_host/host#/device'.
		hostDevicePath := filepath.Join(scsiHostPath, hostName, "device")
		hostDeviceEntries, err := h.osFs.ReadDir(hostDevicePath)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Could not read host device entries at: '%s'.", hostDevicePath)
			continue
		}

		// Look for "session#" within the device directory entries.
		// It's possible that multiple sessions that Trident setup can exist for a given host.
		// Example:
		//  '/sys/class/scsi_host/host10/device/session1'
		//  '/sys/class/scsi_host/host10/device/session2'
		for _, hostDeviceEntry := range hostDeviceEntries {
			sessionName := hostDeviceEntry.Name()
			if !strings.HasPrefix(hostDeviceEntry.Name(), "session") {
				continue
			}

			// Check if the iscsi session exists: '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#'.
			sessionPath := filepath.Join(hostDevicePath, sessionName, "iscsi_session", sessionName)
			if sessionExists, err := h.osFs.Exists(sessionPath); err != nil {
				Logc(ctx).WithError(err).Errorf("Could not read iscsi session path at: '%s'", sessionPath)
				continue
			} else if !sessionExists {
				Logc(ctx).Debugf("iSCSI session path '%s' does not exist.", sessionPath)
				continue
			}

			// Read in target IQN from: '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/targetname'.
			targetNamePath := filepath.Join(sessionPath, "targetname")
			contents, err := h.osFs.ReadFile(targetNamePath)
			if err != nil {
				Logc(ctx).WithError(err).Errorf("Could not read target IQN at: '%s'", targetNamePath)
				continue
			}

			// Ignore sessions that aren't connected to the expected target IQN.
			targetName := strings.TrimSpace(string(contents))
			if targetName != targetIQN {
				Logc(ctx).Debugf("IQN mismatch. '%s' != '%s'; ignoring session.", targetName, targetIQN)
				continue
			}

			// At this point, we know this session is for a NetApp target.
			// Read in all entries under: '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/device'
			sessionDevicePath := filepath.Join(sessionPath, "device")
			sessionDeviceEntries, err := h.osFs.ReadDir(sessionDevicePath)
			if err != nil {
				Logc(ctx).WithError(err).Errorf("Could not read session device entries at: '%s'.", sessionDevicePath)
				continue
			}

			// Search for the 'target<H:C:T>' directory under:
			// `/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/device`.
			for _, entry := range sessionDeviceEntries {
				entryName := entry.Name()
				if !strings.HasPrefix(entryName, "target") {
					continue
				}
				Logc(ctx).WithField("scsiTargetDevice", entryName).Debug("Found SCSI target device directory.")

				// At this point, we know we're looking at a target device directory.
				// '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/device/target<H:C:T>'
				var hostID, channelID, targetID string
				hctSuffix := strings.TrimPrefix(entryName, "target") // "targetH:C:T" -> "H:C:T"
				hctElems := strings.Split(hctSuffix, ":")            // "H:C:T" -> ["H","C","T"]
				if len(hctElems) != 3 {
					Logc(ctx).Errorf("Invalid format detected with: '%s'; expected 'target<H:C:T>'", entryName)
					continue
				}
				// It can be safely assumed that if these elements exist, the kernel has assigned them valid values.
				hostID, channelID, targetID = hctElems[0], hctElems[1], hctElems[2]
				fields := LogFields{
					"hostID":    hostID,
					"channelID": channelID,
					"targetID":  targetID,
				}

				// Build unique key "hostID:channelID:targetID"
				// Filtering by targetIQN above should remove chances of tracking scsi addresses not owned by Trident.
				// It is technically possible for a given host to have multiple channels and multiple targetIDs, we
				// can probably safely assume the targetID will remain the same for a given backend.
				Logc(ctx).WithFields(fields).Debug("Discovered host, channel and target ID.")
				key := fmt.Sprintf("%s:%s:%s", hostID, channelID, targetID)
				if _, exists := deviceMap[key]; !exists {
					deviceMap[key] = models.ScsiDeviceAddress{
						Host:    hostID,
						Channel: channelID,
						Target:  targetID,
					}
				}
			}
		}
	}

	return deviceMap, nil
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

	sysPath := h.chrootPathPrefix + "/sys/class/iscsi_host/"
	if hostDirs, err := h.osFs.ReadDir(sysPath); err != nil {
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
			if deviceDirs, err := h.osFs.ReadDir(devicePath); err != nil {
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
					if targetName, err := h.osFs.ReadFile(targetNamePath); err != nil {
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

// GetSysfsBlockDirsForLUN returns the list of directories in sysfs where the block devices should appear
// after the scan is successful. One directory is returned for each path in the host session map.
func (h *IscsiReconcileHelper) GetSysfsBlockDirsForLUN(lunID int, hostSessionMap map[int]int) []string {
	paths := make([]string, 0)
	for hostNumber, sessionNumber := range hostSessionMap {
		p := fmt.Sprintf(
			h.chrootPathPrefix+"/sys/class/scsi_host/host%d/device/session%d/iscsi_session/session%d/device/target%d"+
				":0:0/%d:0:0:%d",
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
		exists, err := h.osClient.PathExists(dirname)
		if !exists || err != nil {
			continue
		}
		dirFd, err := h.osFs.Open(dirname)
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
