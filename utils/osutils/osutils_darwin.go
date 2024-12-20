// Copyright 2022 NetApp, Inc. All Rights Reserved.

package osutils

import (
	"context"
	"net"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// The Trident build process builds the Trident CLI client for both linux and darwin.
// At compile time golang will type checks the entire code base. Since the CLI is part
// of the Trident code base this file exists to handle darwin specific code.

// getIPAddresses unused stub function
func (o *OSUtils) getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_darwin.getIPAddresses")
	return nil, errors.UnsupportedError("getIPAddresses is not supported for darwin")
}

// GetHostSystemInfo unused stub function
func (o *OSUtils) GetHostSystemInfo(ctx context.Context) (*models.HostSystem, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.GetHostSystemInfo")
	defer Logc(ctx).Debug("<<<< osutils_darwin.GetHostSystemInfo")
	return nil, errors.UnsupportedError("GetHostSystemInfo is not supported for darwin")
}

// NFSActiveOnHost unused stub function
func (o *OSUtils) NFSActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.NFSActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.NFSActiveOnHost")
	return false, errors.UnsupportedError("NFSActiveOnHost is not supported for darwin")
}

// IsLikelyDir determines if mountpoint is a directory
func (o *OSUtils) IsLikelyDir(mountpoint string) (bool, error) {
	stat, err := o.osFs.Stat(mountpoint)
	if err != nil {
		return false, err
	}

	return stat.IsDir(), nil
}

// GetTargetFilePath method returns the path of target file based on OS.
func GetTargetFilePath(ctx context.Context, resourcePath, arg string) string {
	Logc(ctx).Debug(">>>> osutils_darwin.GetTargetFilePath")
	defer Logc(ctx).Debug("<<<< osutils_darwin.GetTargetFilePath")
	return ""
}

// SMBActiveOnHost will always return false on non-windows platform
func SMBActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.SMBActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.SMBActiveOnHost")
	return false, errors.UnsupportedError("SMBActiveOnHost is not supported for darwin")
}

// ServiceActiveOnHost checks if the service is currently running
func (o *OSUtils) ServiceActiveOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.ServiceActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.ServiceActiveOnHost")
	return false, errors.UnsupportedError("ServiceActiveOnHost is not supported for darwin")
}
