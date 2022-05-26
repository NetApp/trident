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
	msg := "GetHostSystemInfo is not supported for windows"
	return nil, UnsupportedError(msg)
}

func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.ISCSIActiveOnHost")
	msg := "ISCSIActiveOnHost is not supported for windows"
	return false, UnsupportedError(msg)
}

func NFSActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> osutils_windows.NFSActiveOnHost")
	defer Logc(ctx).Debug("<<<< osutils_windows.NFSActiveOnHost")
	msg := "NFSActiveOnHost is not supported for windows"
	return false, UnsupportedError(msg)
}
