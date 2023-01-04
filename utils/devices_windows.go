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

// IsLUKSFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsLUKSFormatted(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.IsLUKSFormatted")
	defer Logc(ctx).Debug("<<<< devices_windows.IsLUKSFormatted")
	return false, UnsupportedError("IsLUKSFormatted is not supported for windows")
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.IsOpen")
	defer Logc(ctx).Debug("<<<< devices_windows.IsOpen")
	return false, UnsupportedError("IsOpen is not supported for windows")
}

// LUKSFormat sets up LUKS headers on the device with the specified passphrase, this destroys data on the device
func (d *LUKSDevice) LUKSFormat(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.LUKSFormat")
	defer Logc(ctx).Debug("<<<< devices_windows.LUKSFormat")
	return UnsupportedError("LUKSFormat is not supported for windows")
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.Open")
	defer Logc(ctx).Debug("<<<< devices_windows.Open")
	return UnsupportedError("Open is not supported for windows")
}

// Close performs a luksClose on the LUKS device
func (d *LUKSDevice) Close(ctx context.Context) error {
	Logc(ctx).Debug(">>>> devices_windows.Close")
	defer Logc(ctx).Debug("<<<< devices_windows.Close")
	return UnsupportedError("Close is not supported for windows")
}

func (d *LUKSDevice) RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.RotatePassphrase")
	defer Logc(ctx).Debug("<<<< devices_windows.RotatePassphrase")
	return UnsupportedError("RotatePassphrase is not supported for windows")
}
