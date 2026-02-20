// Copyright 2025 NetApp, Inc. All Rights Reserved.

package luks

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

func (d *LUKSDevice) CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.CheckPassphrase")
	defer Logc(ctx).Debug("<<<< devices_windows.CheckPassphrase")
	return false, errors.UnsupportedError("CheckPassphrase is not supported for windows")
}

func (d *LUKSDevice) RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.RotatePassphrase")
	defer Logc(ctx).Debug("<<<< devices_windows.RotatePassphrase")
	return errors.UnsupportedError("RotatePassphrase is not supported for windows")
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

// formatUnformattedDevice attempts to set up LUKS headers on a device with the specified passphrase, but bails if thea
// underlying device already has a format present that is not LUKS.
func (d *LUKSDevice) formatUnformattedDevice(ctx context.Context, _ string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.formatUnformattedDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.formatUnformattedDevice")
	return false, errors.UnsupportedError("formatUnformattedDevice is not supported for windows")
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_windows.Open")
	defer Logc(ctx).Debug("<<<< devices_windows.Open")
	return errors.UnsupportedError("Open is not supported for windows")
}

// Resize performs a luksResize on the LUKS device
func (d *LUKSDevice) Resize(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.Resize")
	defer Logc(ctx).Debug("<<<< devices_darwin.Resize")
	return errors.UnsupportedError("Resize is not supported for darwin")
}

func GetUnderlyingDevicePathForDevice(ctx context.Context, command exec.Command, luksDevicePath string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.GetUnderlyingDevicePathForDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.GetUnderlyingDevicePathForDevice")
	return "", errors.UnsupportedError("GetUnderlyingDevicePathForDevice is not supported for windows")
}

func IsOpen(ctx context.Context, devicePath string) (bool, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_windows.IsOpen")
	defer Logc(ctx).Debug("<<<< devices_windows.IsOpen")
	return false, errors.UnsupportedError("IsOpen is not supported for windows")
}

func (d *LUKSDevice) isMappingStale(_ context.Context) bool {
	return false
}
