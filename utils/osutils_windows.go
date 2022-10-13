// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"net"
	"path"
	"strings"

	"github.com/elastic/go-sysinfo"

	. "github.com/netapp/trident/logger"
)

// The Trident build process builds the Trident CLI client for both linux and windows.
// At compile time golang will type checks the entire code base. Since the CLI is part
// of the Trident code base this file exists to handle windows specific code.

// getIPAddresses unused stub function
func getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_windows.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_windows.getIPAddresses")
	return nil, UnsupportedError("getIPAddresses is not supported for windows")
}

// SMBActiveOnHost will always return true as it is native to windows
func SMBActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.SMBActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.SMBActiveOnHost")
	return true, nil
}

// GetHostSystemInfo returns the information about OS type and platform
func GetHostSystemInfo(ctx context.Context) (*HostSystem, error) {
	osInfo, err := sysinfo.Host()
	if err != nil {
		return nil, err
	}

	// For windows, host.OS.Distro will correspond 'windows'
	// host.OS.Version will correspond to 'Windows Server 20xx Datacenter'

	host := &HostSystem{}
	host.OS.Distro = osInfo.Info().OS.Platform
	host.OS.Version = osInfo.Info().OS.Name

	return host, nil
}

// NFSActiveOnHost unused stub function
func NFSActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.NFSActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.NFSActiveOnHost")
	return false, UnsupportedError("NFSActiveOnHost is not supported for windows")
}

// GetTargetFilePath method returns the path of target file based on OS.
func GetTargetFilePath(ctx context.Context, resourcePath, arg string) string {
	Logc(ctx).Debug(">>>> osutils_windows.GetTargetFilePath")
	defer Logc(ctx).Debug("<<<< osutils_windows.GetTargetFilePath")
	filePath := path.Join(resourcePath, arg)
	return strings.Replace(filePath, "/", "\\", -1)
}
