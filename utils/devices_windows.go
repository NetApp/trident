// Copyright 2024 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for windows flavor

package utils

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// flushOneDevice unused stub function
func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.flushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.flushOneDevice")
	return errors.UnsupportedError("flushOneDevice is not supported for windows")
}

// getISCSIDiskSize unused stub function
func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_windows.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< devices_windows.getISCSIDiskSize")
	return 0, errors.UnsupportedError("getBlockSize is not supported for windows")
}

func IsOpenLUKSDevice(ctx context.Context, devicePath string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_windows.IsLUKSDeviceOpen")
	defer Logc(ctx).Debug("<<<< devices_windows.IsLUKSDeviceOpen")
	return false, errors.UnsupportedError("IsLUKSDeviceOpen is not supported for windows")
}

func EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_windows.EnsureLUKSDeviceClosed")
	defer Logc(ctx).Debug("<<<< devices_windows.EnsureLUKSDeviceClosed")
	return errors.UnsupportedError("EnsureLUKSDeviceClosed is not supported for windows")
}

func getDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.getDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices_windows.getDeviceFSType")
	return "", errors.UnsupportedError("getDeviceFSTypeis not supported for windows")
}

func getDeviceFSTypeRetry(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.getDeviceFSTypeRetry")
	defer Logc(ctx).Debug("<<<< devices_windows.getDeviceFSTypeRetry")
	return "", errors.UnsupportedError("getDeviceFSTypeRetry is not supported for windows")
}

// IsLUKSFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsLUKSFormatted(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.IsLUKSFormatted")
	defer Logc(ctx).Debug("<<<< devices_windows.IsLUKSFormatted")
	return false, errors.UnsupportedError("IsLUKSFormatted is not supported for windows")
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.IsOpen")
	defer Logc(ctx).Debug("<<<< devices_windows.IsOpen")
	return false, errors.UnsupportedError("IsOpen is not supported for windows")
}

// LUKSFormat attempts to set up LUKS headers on a device with the specified passphrase, but bails if the
// underlying device already has a format present that is not LUKS.
func (d *LUKSDevice) LUKSFormat(ctx context.Context, _ string) error {
	Logc(ctx).Debug(">>>> devices_windows.LUKSFormat")
	defer Logc(ctx).Debug("<<<< devices_windows.LUKSFormat")
	return errors.UnsupportedError("LUKSFormat is not supported for windows")
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.Open")
	defer Logc(ctx).Debug("<<<< devices_windows.Open")
	return errors.UnsupportedError("Open is not supported for windows")
}

// Close performs a luksClose on the LUKS device
func (d *LUKSDevice) Close(ctx context.Context) error {
	Logc(ctx).Debug(">>>> devices_windows.Close")
	defer Logc(ctx).Debug("<<<< devices_windows.Close")
	return errors.UnsupportedError("Close is not supported for windows")
}

func (d *LUKSDevice) RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.RotatePassphrase")
	defer Logc(ctx).Debug("<<<< devices_windows.RotatePassphrase")
	return errors.UnsupportedError("RotatePassphrase is not supported for windows")
}

// Resize performs a luksResize on the LUKS device
func (d *LUKSDevice) Resize(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.Resize")
	defer Logc(ctx).Debug("<<<< devices_darwin.Resize")
	return errors.UnsupportedError("Resize is not supported for darwin")
}

func (d *LUKSDevice) CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.CheckPassphrase")
	defer Logc(ctx).Debug("<<<< devices_windows.CheckPassphrase")
	return false, errors.UnsupportedError("CheckPassphrase is not supported for windows")
}

func GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, luksDevicePath string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.GetUnderlyingDevicePathForLUKSDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.GetUnderlyingDevicePathForLUKSDevice")
	return "", errors.UnsupportedError("GetUnderlyingDevicePathForLUKSDevice is not supported for windows")
}

func EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.EnsureLUKSDeviceClosedWithMaxWaitLimit")
	defer Logc(ctx).Debug("<<<< devices_windows.EnsureLUKSDeviceClosedWithMaxWaitLimit")
	return errors.UnsupportedError("EnsureLUKSDeviceClosedWithMaxWaitLimit is not supported for windows")
}
