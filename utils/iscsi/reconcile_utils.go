// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

//go:generate mockgen -destination=../../mocks/mock_utils/mock_iscsi/mock_reconcile_utils.go github.com/netapp/trident/utils/iscsi IscsiReconcileUtils

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
)

type IscsiReconcileUtils interface {
	GetISCSIHostSessionMapForTarget(context.Context, string) map[int]int
	GetSysfsBlockDirsForLUN(int, map[int]int) []string
	GetDevicesForLUN(paths []string) ([]string, error)
	ReconcileISCSIVolumeInfo(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (bool, error)
}

type IscsiReconcileHelper struct {
	chrootPathPrefix string
	osClient         OS
}

func NewReconcileUtils(chrootPathPrefix string, osClient OS) IscsiReconcileUtils {
	return &IscsiReconcileHelper{
		chrootPathPrefix: chrootPathPrefix,
		osClient:         osClient,
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
	if hostDirs, err := os.ReadDir(sysPath); err != nil {
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
			if deviceDirs, err := os.ReadDir(devicePath); err != nil {
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
					if targetName, err := os.ReadFile(targetNamePath); err != nil {
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
