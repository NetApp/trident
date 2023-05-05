// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for Darwin flavor

package utils

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// flushOneDevice unused stub function
func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.flushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.flushOneDevice")
	return errors.UnsupportedError("flushOneDevice is not supported for darwin")
}

// getISCSIDiskSize unused stub function
func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< devices_darwin.getISCSIDiskSize")
	return 0, errors.UnsupportedError("getBlockSize is not supported for darwin")
}

func IsOpenLUKSDevice(ctx context.Context, devicePath string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_darwin.IsLUKSDeviceOpen")
	defer Logc(ctx).Debug("<<<< devices_darwin.IsLUKSDeviceOpen")
	return false, errors.UnsupportedError("IsLUKSDeviceOpen is not supported for darwin")
}

func EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_darwin.EnsureLUKSDeviceClosed")
	defer Logc(ctx).Debug("<<<< devices_darwin.EnsureLUKSDeviceClosed")
	return errors.UnsupportedError("EnsureLUKSDeviceClosed is not supported for darwin")
}

func getDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices_darwin.getDeviceFSType")
	return "", errors.UnsupportedError("getDeviceFSTypeis not supported for darwin")
}

func getDeviceFSTypeRetry(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getDeviceFSTypeRetry")
	defer Logc(ctx).Debug("<<<< devices_darwin.getDeviceFSTypeRetry")
	return "", errors.UnsupportedError("getDeviceFSTypeRetry is not supported for darwin")
}

// IsLUKSFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsLUKSFormatted(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.IsLUKSFormatted")
	defer Logc(ctx).Debug("<<<< devices_darwin.IsLUKSFormatted")
	return false, errors.UnsupportedError("IsLUKSFormatted is not supported for darwin")
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.IsOpen")
	defer Logc(ctx).Debug("<<<< devices_darwin.IsOpen")
	return false, errors.UnsupportedError("IsOpen is not supported for darwin")
}

// LUKSFormat sets up LUKS headers on the device with the specified passphrase, this destroys data on the device
func (d *LUKSDevice) LUKSFormat(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.LUKSFormat")
	defer Logc(ctx).Debug("<<<< devices_darwin.LUKSFormat")
	return errors.UnsupportedError("LUKSFormat is not supported for darwin")
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.Open")
	defer Logc(ctx).Debug("<<<< devices_darwin.Open")
	return errors.UnsupportedError("Open is not supported for darwin")
}

// Close performs a luksClose on the LUKS device
func (d *LUKSDevice) Close(ctx context.Context) error {
	Logc(ctx).Debug(">>>> devices_darwin.Close")
	defer Logc(ctx).Debug("<<<< devices_darwin.Close")
	return errors.UnsupportedError("Close is not supported for darwin")
}

func (d *LUKSDevice) RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.RotatePassphrase")
	defer Logc(ctx).Debug("<<<< devices_darwin.RotatePassphrase")
	return errors.UnsupportedError("RotatePassphrase is not supported for darwin")
}

func (d *LUKSDevice) CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.CheckPassphrase")
	defer Logc(ctx).Debug("<<<< devices_darwin.CheckPassphrase")
	return false, errors.UnsupportedError("CheckPassphrase is not supported for darwin")
}

func GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, luksDevicePath string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.GetUnderlyingDevicePathForLUKSDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.GetUnderlyingDevicePathForLUKSDevice")
	return "", errors.UnsupportedError("GetUnderlyingDevicePathForLUKSDevice is not supported for darwin")
}

// Resize performs a luksResize on the LUKS device
func (d *LUKSDevice) Resize(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.Resize")
	defer Logc(ctx).Debug("<<<< devices_darwin.Resize")
	return errors.UnsupportedError("Resize is not supported for darwin")
}
