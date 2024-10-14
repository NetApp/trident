// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

import (
	"context"
	"os"
	"path"
	"strings"

	. "github.com/netapp/trident/logging"
)

// These functions are widely used throughout the codebase,so they could eventually live in utils or some other
// top-level package.
// TODO (vivintw) remove this file once the refactoring is done.

func GetFCPHostPortNames(ctx context.Context) ([]string, error) {
	var wwpns []string

	// TODO (vhs) : Get the chroot path from the config and prefix it to the path
	sysPath := "/sys/class/fc_host"

	rportDirs, err := os.ReadDir(sysPath)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Could not read %s", sysPath)
		return wwpns, err
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

		wwpns = append(wwpns, strings.TrimSpace(string(portName)))
	}

	return wwpns, nil
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
