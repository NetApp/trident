// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for Darwin flavor

package utils

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

// flushOneDevice unused stub function
func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.flushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.flushOneDevice")
	return UnsupportedError("flushOneDevice is not supported for darwin")
}

// getISCSIDiskSize unused stub function
func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< devices_darwin.getISCSIDiskSize")
	return 0, UnsupportedError("getBlockSize is not supported for darwin")
}

func EnsureLUKSDevice(ctx context.Context, device, volumeId, luksPassphrase string) (bool, string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.EnsureLUKSDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.EnsureLUKSDevice")
	return false, "", UnsupportedError("EnsureLUKSDevice is not supported for darwin")
}

func IsOpenLUKSDevice(ctx context.Context, devicePath string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.IsLUKSDeviceOpen")
	defer Logc(ctx).Debug("<<<< devices_darwin.IsLUKSDeviceOpen")
	return false, UnsupportedError("IsLUKSDeviceOpen is not supported for darwin")
}

func EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.EnsureLUKSDeviceClosed")
	defer Logc(ctx).Debug("<<<< devices_darwin.EnsureLUKSDeviceClosed")
	return UnsupportedError("EnsureLUKSDeviceClosed is not supported for darwin")
}

func getDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices_darwin.getDeviceFSType")
	return "", UnsupportedError("getDeviceFSTypeis not supported for darwin")
}

func getDeviceFSTypeRetry(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getDeviceFSTypeRetry")
	defer Logc(ctx).Debug("<<<< devices_darwin.getDeviceFSTypeRetry")
	return "", UnsupportedError("getDeviceFSTypeRetry is not supported for darwin")
}
