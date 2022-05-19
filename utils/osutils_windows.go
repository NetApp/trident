//go:build windows
// +build windows

// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"errors"
	"net"

	. "github.com/netapp/trident/logger"
)

// The Trident build process builds the Trident CLI client for both linux and windows.
// At compile time golang will type checks the entire code base. Since the CLI is part
// of the Trident code base this file exists to handle windows specific code.

func getIPAddresses(ctx context.Context) ([]net.Addr, error) {
	Logc(ctx).Debug(">>>> osutils_windows.getIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils_windows.getIPAddresses")
	return nil, errors.New("getIPAddresses is not supported for windows")
}

func getFilesystemSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> osutils_windows.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< osutils_windows.getFilesystemSize")
	return 0, errors.New("getFilesystemSize is not supported for windows")
}

func GetFilesystemStats(ctx context.Context, _ string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> osutils_windows.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< osutils_windows.GetFilesystemStats")
	return 0, 0, 0, 0, 0, 0, errors.New("GetFilesystemStats is not supported for windows")
}

func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> osutils_windows.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< osutils_windows.getISCSIDiskSize")
	return 0, errors.New("getBlockSize is not supported for windows")
}

func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> osutils_windows.flushOneDevice")
	defer Logc(ctx).Debug("<<<< osutils_windows.flushOneDevice")
	return errors.New("flushOneDevice is not supported for windows")
}

func GetHostSystemInfo(ctx context.Context) (*HostSystem, error) {
	Logc(ctx).Debug(">>>> osutils_windows.GetHostSystemInfo")
	defer Logc(ctx).Debug("<<<< osutils_windows.GetHostSystemInfo")
	msg := "GetHostSystemInfo is not is not supported for windows"
	return nil, UnsupportedError(msg)
}

func PrepareNFSPackagesOnHost(ctx context.Context, host HostSystem) error {
	Logc(ctx).Debug(">>>> osutils_windows.PrepareNFSPackagesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.PrepareNFSPackagesOnHost")
	msg := "PrepareNFSPackagesOnHost is not is not supported for windows"
	return UnsupportedError(msg)
}

func PrepareNFSServicesOnHost(ctx context.Context) error {
	Logc(ctx).Debug(">>>> osutils_windows.PrepareNFSServicesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.PrepareNFSServicesOnHost")
	msg := "PrepareNFSServicesOnHost is not is not supported for windows"
	return UnsupportedError(msg)
}

func ServiceActiveOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.ServiceActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.ServiceActiveOnHost")
	msg := "ServiceActiveOnHost is not is not supported for windows"
	return false, UnsupportedError(msg)
}

func ServiceEnabledOnHost(ctx context.Context, service string) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.ServiceEnabledOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.ServiceEnabledOnHost")
	msg := "ServiceEnabledOnHost is not is not supported for windows"
	return false, UnsupportedError(msg)
}

func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.ISCSIActiveOnHost")
	msg := "ISCSIActiveOnHost is not is not supported for windows"
	return false, UnsupportedError(msg)
}

func PrepareISCSIPackagesOnHost(ctx context.Context, host HostSystem, iscsiPreconfigured bool) error {
	Logc(ctx).Debug(">>>> osutils_windows.PrepareISCSIPackagesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.PrepareISCSIPackagesOnHost")
	msg := "PrepareISCSIPackagesOnHost is not is not supported for windows"
	return UnsupportedError(msg)
}

func PrepareISCSIServicesOnHost(ctx context.Context, host HostSystem) error {
	Logc(ctx).Debug(">>>> osutils_windows.PrepareISCSIServicesOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.PrepareISCSIServicesOnHost")
	msg := "PrepareISCSIServicesOnHost is not is not supported for windows"
	return UnsupportedError(msg)
}
