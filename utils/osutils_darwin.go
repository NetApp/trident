// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"net"
	"os"

	. "github.com/netapp/trident/logger"
)

// The Trident build process builds the Trident CLI client for both linux and darwin.
// At compile time golang will type checks the entire code base. Since the CLI is part
// of the Trident code base this file exists to handle darwin specific code.

// getIPAddresses unused stub function
func getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_darwin.getIPAddresses")
	return nil, UnsupportedError("getIPAddresses is not supported for darwin")
}

// GetHostSystemInfo unused stub function
func GetHostSystemInfo(ctx context.Context) (*HostSystem, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.GetHostSystemInfo")
	defer Logc(ctx).Debug("<<<< osutils_darwin.GetHostSystemInfo")
	return nil, UnsupportedError("GetHostSystemInfo is not supported for darwin")
}

// NFSActiveOnHost unused stub function
func NFSActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.NFSActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.NFSActiveOnHost")
	return false, UnsupportedError("NFSActiveOnHost is not supported for darwin")
}

// IsLikelyDir determines if mountpoint is a directory
func IsLikelyDir(mountpoint string) (bool, error) {
	stat, err := os.Stat(mountpoint)
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
	return false, UnsupportedError("SMBActiveOnHost is not supported for darwin")
}
