// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

//go:generate mockgen -destination=../../mocks/mock_utils/mock_fcp/mock_reconcile_utils.go github.com/netapp/trident/utils/fcp FcpReconcileUtils

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type FcpReconcileUtils interface {
	GetFCPHostSessionMapForTarget(context.Context, string) []map[string]int
	GetSysfsBlockDirsForLUN(int, []map[string]int) []string
	GetDevicesForLUN(paths []string) ([]string, error)
	ReconcileFCPVolumeInfo(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (bool, error)
	CheckZoningExistsWithTarget(context.Context, string) (bool, error)
}

type FcpReconcileHelper struct {
	chrootPathPrefix string
	osClient         OS
}

func NewReconcileUtils(chrootPathPrefix string, osClient OS) FcpReconcileUtils {
	return &FcpReconcileHelper{
		chrootPathPrefix: chrootPathPrefix,
		osClient:         osClient,
	}
}

// ReconcileFCPVolumeInfo returns true if any of the expected conditions for a present volume are true (e.g. the
// expected LUN exists).
func (h *FcpReconcileHelper) ReconcileFCPVolumeInfo(
	ctx context.Context, trackingInfo *models.VolumeTrackingInfo,
) (bool, error) {
	pubInfo := trackingInfo.VolumePublishInfo
	lun := int(pubInfo.FCPLunNumber)
	wwnn := pubInfo.FCTargetWWNN

	sessionMap := h.GetFCPHostSessionMapForTarget(ctx, wwnn)
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

// GetFCPHostSessionMapForTarget returns a map of FCP host numbers to FCP session numbers
// for a given FCP target.
func (h *FcpReconcileHelper) GetFCPHostSessionMapForTarget(
	ctx context.Context,
	fcpNodeName string,
) []map[string]int {
	fields := LogFields{"fcpNodeName": fcpNodeName}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.GetFCPHostSessionMapForTarget")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.GetFCPHostSessionMapForTarget")

	var (
		portNumber int
		targetName []byte
	)

	hostportMap := make([]map[string]int, 0)

	re := regexp.MustCompile(`rport-(\d+):`)

	sysPath := h.chrootPathPrefix + "/sys/class/fc_remote_ports/"
	if rportDirs, err := os.ReadDir(sysPath); err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return hostportMap
	} else {
		for _, rportDir := range rportDirs {

			rportDirName := rportDir.Name()
			if !strings.HasPrefix(rportDirName, "rport") {
				continue
			}

			devicePath := sysPath + rportDirName
			targetNamePath := devicePath + "/node_name"

			if targetName, err = os.ReadFile(targetNamePath); err != nil {
				Logc(ctx).WithFields(LogFields{
					"path":  targetNamePath,
					"error": err,
				}).Error("Could not read target name file")
				continue
			}

			if MatchWorldWideNames(string(targetName), fcpNodeName, false) {
				fcHostPath := h.chrootPathPrefix + "/sys/class/fc_host/"

				var hostNumber string
				hostNumberStr := re.FindStringSubmatch(rportDirName)
				if len(hostNumberStr) > 1 {
					hostNumber = hostNumberStr[1]
				}

				hostPath := fcHostPath + "host" + hostNumber + "/" + "device/" + rportDirName + "/"

				targetDirs, err := os.ReadDir(hostPath)
				for _, targetDir := range targetDirs {
					// get last  digit of the target dir target10:0:1
					targetDirStr := targetDir.Name()
					if !strings.HasPrefix(targetDirStr, "target") {
						continue
					}

					portNum := strings.Split(targetDirStr, ":")
					if len(portNum) > 2 {
						if portNumber, err = strconv.Atoi(portNum[2]); err != nil {
							Logc(ctx).Error("Could not parse port number")
							continue
						}
					} else {
						Logc(ctx).Error("Could not parse port number")
						continue
					}

				}

				hostportMap = append(hostportMap, map[string]int{rportDirName: portNumber})

			}

		}
	}

	return hostportMap
}

// GetSysfsBlockDirsForLUN returns the list of directories in sysfs where the block devices should appear
// after the scan is successful. One directory is returned for each path in the host session map.
func (h *FcpReconcileHelper) GetSysfsBlockDirsForLUN(lunID int, hostSessionMap []map[string]int) []string {
	paths := make([]string, 0)
	var hostNumber int
	var err error
	re := regexp.MustCompile(`rport-(\d+):`)
	for _, hostPortMap := range hostSessionMap {
		for hostClass, sessionNumber := range hostPortMap {
			portNumberStr := re.FindStringSubmatch(hostClass)
			if len(portNumberStr) > 1 {
				if hostNumber, err = strconv.Atoi(portNumberStr[1]); err != nil {
					continue
				}
			} else {
				continue
			}

			p := fmt.Sprintf(
				h.chrootPathPrefix+"/sys/class/fc_host/host%d/device/%s/target%d:0:%d/%d:0:%d:%d",
				hostNumber, hostClass, hostNumber, sessionNumber, hostNumber, sessionNumber, lunID)
			paths = append(paths, p)
		}
	}
	return paths
}

// GetDevicesForLUN find the /dev/sd* device names for an FCP LUN.
func (h *FcpReconcileHelper) GetDevicesForLUN(paths []string) ([]string, error) {
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
		err = dirFd.Close()
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

// CheckZoningExistsWithTarget checks if the target is zoned with the initiator.
func (h *FcpReconcileHelper) CheckZoningExistsWithTarget(ctx context.Context, targetNodeName string) (bool, error) {
	fields := LogFields{"targetNodeName": targetNodeName}
	Logc(ctx).WithFields(fields).Debug(">>>> fcp.CheckZoningExistsWithTarget")
	defer Logc(ctx).WithFields(fields).Debug("<<<< fcp.CheckZoningExistsWithTarget")

	sysPath := h.chrootPathPrefix + "/sys/class/fc_remote_ports/"
	rportDirs, err := os.ReadDir(sysPath)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return false, err
	}

	for _, rportDir := range rportDirs {
		rportDirName := rportDir.Name()
		if !strings.HasPrefix(rportDirName, "rport") {
			continue
		}

		devicePath := sysPath + rportDirName
		nodeNamePath := devicePath + "/node_name"
		nodeName, err := os.ReadFile(nodeNamePath)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"path":  nodeNamePath,
				"error": err,
			}).Error("Could not read target name file")
			continue
		}

		if !MatchWorldWideNames(string(nodeName), targetNodeName, false) {
			// Skip the check for non-relevant target
			continue
		}

		portStatus, err := os.ReadFile(devicePath + "/port_state")
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"path":  devicePath + "/port_state",
				"error": err,
			}).Error("Could not read port state file")
			continue
		}

		portStatusStr := strings.TrimSpace(string(portStatus))
		if portStatusStr == "Online" {
			return true, nil
		}
	}

	return false, errors.New("no zoned ports found")
}
