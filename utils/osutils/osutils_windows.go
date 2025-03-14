// Copyright 2022 NetApp, Inc. All Rights Reserved.

package osutils

import (
	"context"
	"net"
	"path"
	"strings"

	"github.com/elastic/go-sysinfo"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// The Trident build process builds the Trident CLI client for both linux and windows.
// At compile time golang will type checks the entire code base. Since the CLI is part
// of the Trident code base this file exists to handle windows specific code.

// getIPAddresses unused stub function
func (o *OSUtils) getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_windows.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_windows.getIPAddresses")
	return nil, errors.UnsupportedError("getIPAddresses is not supported for windows")
}

// SMBActiveOnHost will always return true as it is native to windows
func SMBActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.SMBActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.SMBActiveOnHost")
	return true, nil
}

// GetHostSystemInfo returns the information about OS type and platform
func (o *OSUtils) GetHostSystemInfo(ctx context.Context) (*models.HostSystem, error) {
	osInfo, err := sysinfo.Host()
	if err != nil {
		return nil, err
	}

	// For windows, host.OS.Distro will correspond 'windows'
	// host.OS.Version will correspond to 'Windows Server 20xx Datacenter'

	host := &models.HostSystem{}
	host.OS.Distro = osInfo.Info().OS.Platform
	host.OS.Version = osInfo.Info().OS.Name

	return host, nil
}

// NFSActiveOnHost unused stub function
func (o *OSUtils) NFSActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.NFSActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.NFSActiveOnHost")
	return false, errors.UnsupportedError("NFSActiveOnHost is not supported for windows")
}

// GetTargetFilePath method returns the path of target file based on OS.
func GetTargetFilePath(ctx context.Context, resourcePath, arg string) string {
	Logc(ctx).Debug(">>>> osutils_windows.GetTargetFilePath")
	defer Logc(ctx).Debug("<<<< osutils_windows.GetTargetFilePath")
	filePath := path.Join(resourcePath, arg)
	return strings.Replace(filePath, "/", "\\", -1)
}

// IsLikelyDir determines if mountpoint is a directory
func (o *OSUtils) IsLikelyDir(mountpoint string) (bool, error) {
	return o.PathExists(mountpoint)
}

// ServiceActiveOnHost checks if the service is currently running
func (o *OSUtils) ServiceActiveOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.ServiceActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.ServiceActiveOnHost")
	return false, errors.UnsupportedError("ServiceActiveOnHost is not supported for windows")
}
