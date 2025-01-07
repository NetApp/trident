// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

import (
	"context"
	"os"
	"path"
	"slices"
	"strings"

	. "github.com/netapp/trident/logging"
)

// These functions are widely used throughout the codebase,so they could eventually live in utils or some other
// top-level package.
// TODO (vivintw) remove this file once the refactoring is done.

// getFCPInitiatorPortName returns a map of initiator port names for each host.
// e.g. map[host11 : 0x50014380242c2b7d]
func getFCPInitiatorPortName(ctx context.Context) (map[string]string, error) {
	initiatorPortNameMap := make(map[string]string)

	// TODO (vhs) : Get the chroot path from the config and prefix it to the path
	sysPath := "/sys/class/fc_host"

	rportDirs, err := os.ReadDir(sysPath)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return initiatorPortNameMap, err
	}

	for _, rportDir := range rportDirs {
		hostName := rportDir.Name()
		if !strings.HasPrefix(hostName, "host") {
			continue
		}

		portName, err := os.ReadFile(path.Join(sysPath, hostName, "port_name"))
		if err != nil {
			Logc(ctx).WithField("error", err).Errorf("Could not read port_name for %s", hostName)
			continue
		}

		initiatorPortNameMap[hostName] = strings.TrimSpace(string(portName))
	}

	return initiatorPortNameMap, nil
}

// getFCPRPortsDirectories returns the directories under the given path that start with "rport".
// e.g. /sys/class/fc_host/host11/device/rport-11:0-1
func getFCPRPortsDirectories(ctx context.Context, path string) ([]string, error) {
	var dirNames []string

	rportDirs, err := os.ReadDir(path)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", path)
		return dirNames, err
	}

	for _, rportDir := range rportDirs {
		name := rportDir.Name()
		if strings.HasPrefix(name, "rport") {
			dirNames = append(dirNames, name)
		}
	}

	return dirNames, nil
}

// getFCPTargetPortNames returns a map of target port names for each host.
// e.g. map[host11 : [0x50014380242c2b7f, 0x50014380242c2b7e]]
func getFCPTargetPortNames(ctx context.Context) (map[string][]string, error) {
	targetPortNamesMap := make(map[string][]string)

	basePath := "/sys/class/fc_host"

	hosts, err := os.ReadDir(basePath)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", basePath)
		return targetPortNamesMap, err
	}

	for _, hostDir := range hosts {
		deviceRPortDirectoryPath := path.Join(basePath, hostDir.Name(), "device")
		deviceRPortDirs, err := getFCPRPortsDirectories(ctx, deviceRPortDirectoryPath)
		if err != nil {
			Logc(ctx).WithField("error", err).Errorf("Could not read %s", deviceRPortDirectoryPath)
			continue
		}

		for _, deviceRPortDir := range deviceRPortDirs {
			fcRemoteRPortsPath := path.Join(deviceRPortDirectoryPath, deviceRPortDir, "fc_remote_ports", deviceRPortDir, "node_name")
			nodeName, err := os.ReadFile(fcRemoteRPortsPath)
			if err != nil {
				Logc(ctx).WithField("error", err).Errorf("Could not read node name for %s", nodeName)
				continue
			}

			// Skip the node if it is not a valid node name
			if strings.TrimSpace(string(nodeName)) != "0x0" {
				targetPortNamesMap[hostDir.Name()] = append(targetPortNamesMap[hostDir.Name()], strings.TrimSpace(string(nodeName)))
			}
		}
	}

	// Remove duplicate node names
	for key, values := range targetPortNamesMap {
		targetPortNamesMap[key] = slices.Compact(values)
	}

	return targetPortNamesMap, nil
}

// GetFCPInitiatorTargetMap returns a map of initiator port name to target port names.
// e.g. map[0x50014380242c2b7d : [0x50014380242c2b7e]]
func GetFCPInitiatorTargetMap(ctx context.Context) (map[string][]string, error) {
	hostWWPNMap := make(map[string][]string)

	initiatorPortNameMap, err := getFCPInitiatorPortName(ctx)
	if err != nil {
		return hostWWPNMap, err
	}

	targetPortNamesMap, err := getFCPTargetPortNames(ctx)
	if err != nil {
		return hostWWPNMap, err
	}

	// Create a map of initiator to targets
	for initiator, iPortName := range initiatorPortNameMap {
		for target, tPortName := range targetPortNamesMap {
			if initiator == target {
				hostWWPNMap[iPortName] = tPortName
			}
		}
	}

	return hostWWPNMap, nil
}

// ConvertStrToWWNFormat converts a WWnumber from string to the format xx:xx:xx:xx:xx:xx:xx:xx.
func ConvertStrToWWNFormat(wwnStr string) string {
	wwn := ""
	for i := 0; i < len(wwnStr); i += 2 {
		wwn += wwnStr[i : i+2]
		if i+2 < len(wwnStr) {
			wwn += ":"
		}
	}
	return wwn
}
