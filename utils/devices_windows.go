// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for windows flavor

package utils

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

// flushOneDevice unused stub function
func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.flushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.flushOneDevice")
	return UnsupportedError("flushOneDevice is not supported for windows")
}

// getISCSIDiskSize unused stub function
func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_windows.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< devices_windows.getISCSIDiskSize")
	return 0, UnsupportedError("getBlockSize is not supported for windows")
}

func EnsureLUKSDevice(ctx context.Context, device, volumeId, luksPassphrase string) (bool, string, error) {
	Logc(ctx).Debug(">>>> devices_windows.EnsureLUKSDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.EnsureLUKSDevice")
	return false, "", UnsupportedError("EnsureLUKSDevice is not supported for windows")
}

func IsOpenLUKSDevice(ctx context.Context, devicePath string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.IsLUKSDeviceOpen")
	defer Logc(ctx).Debug("<<<< devices_windows.IsLUKSDeviceOpen")
	return false, UnsupportedError("IsLUKSDeviceOpen is not supported for windows")
}

func EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.EnsureLUKSDeviceClosed")
	defer Logc(ctx).Debug("<<<< devices_windows.EnsureLUKSDeviceClosed")
	return UnsupportedError("EnsureLUKSDeviceClosed is not supported for windows")
}

func getDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.getDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices_windows.getDeviceFSType")
	return "", UnsupportedError("getDeviceFSTypeis not supported for windows")
}

func getDeviceFSTypeRetry(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.getDeviceFSTypeRetry")
	defer Logc(ctx).Debug("<<<< devices_windows.getDeviceFSTypeRetry")
	return "", UnsupportedError("getDeviceFSTypeRetry is not supported for windows")
}
