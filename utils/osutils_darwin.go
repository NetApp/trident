// Copyright 2019 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"errors"
	"net"

	. "github.com/netapp/trident/logger"
)

// The Trident build process builds the Trident CLI client for both linux and darwin.
// At compile time golang will type checks the entire code base. Since the CLI is part
// of the Trident code base this file exists to handle darwin specific code.

func getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_darwin.getIPAddresses")
	return nil, errors.New("getIPAddresses is not supported for darwin")
}

func getFilesystemSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< osutils_darwin.getFilesystemSize")
	return 0, errors.New("getFilesystemSize is not supported for darwin")
}

func GetFilesystemStats(ctx context.Context, _ string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< osutils_darwin.GetFilesystemStats")
	return 0, 0, 0, 0, 0, 0, errors.New("GetFilesystemStats is not supported for darwin")
}

func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< osutils_darwin.getISCSIDiskSize")
	return 0, errors.New("getBlockSize is not supported for darwin")
}

func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> osutils_darwin.flushOneDevice")
	defer Logc(ctx).Debug("<<<< osutils_darwin.flushOneDevice")
	return errors.New("flushOneDevice is not supported for darwin")
}

func GetHostSystemInfo(ctx context.Context) (*HostSystem, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.GetHostSystemInfo")
	defer Logc(ctx).Debug("<<<< osutils_darwin.GetHostSystemInfo")
	msg := "GetHostSystemInfo is not is not supported for darwin"
	return nil, UnsupportedError(msg)
}

func PrepareNFSPackagesOnHost(ctx context.Context, host HostSystem) error {
	Logc(ctx).Debug(">>>> osutils_darwin.PrepareNFSPackagesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.PrepareNFSPackagesOnHost")
	msg := "PrepareNFSPackagesOnHost is not is not supported for darwin"
	return UnsupportedError(msg)
}

func PrepareNFSServicesOnHost(ctx context.Context) error {
	Logc(ctx).Debug(">>>> osutils_darwin.PrepareNFSServicesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.PrepareNFSServicesOnHost")
	msg := "PrepareNFSServicesOnHost is not is not supported for darwin"
	return UnsupportedError(msg)
}

func ServiceActiveOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.ServiceActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.ServiceActiveOnHost")
	msg := "ServiceActiveOnHost is not is not supported for darwin"
	return false, UnsupportedError(msg)
}

func ServiceEnabledOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.ServiceEnabledOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.ServiceEnabledOnHost")
	msg := "ServiceEnabledOnHost is not is not supported for darwin"
	return false, UnsupportedError(msg)
}

func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_darwin.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.ISCSIActiveOnHost")
	msg := "ISCSIActiveOnHost is not is not supported for darwin"
	return false, UnsupportedError(msg)
}

func PrepareISCSIPackagesOnHost(ctx context.Context, host HostSystem, iscsiPreconfigured bool) error {
	Logc(ctx).Debug(">>>> osutils_darwin.PrepareISCSIPackagesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.PrepareISCSIPackagesOnHost")
	msg := "PrepareISCSIPackagesOnHost is not is not supported for darwin"
	return UnsupportedError(msg)
}

func PrepareISCSIServicesOnHost(ctx context.Context, host HostSystem) error {
	Logc(ctx).Debug(">>>> osutils_darwin.PrepareISCSIServicesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_darwin.PrepareISCSIServicesOnHost")
	msg := "PrepareISCSIServicesOnHost is not is not supported for darwin"
	return UnsupportedError(msg)
}
