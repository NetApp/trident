// Copyright 2024 NetApp, Inc. All Rights Reserved.

package luks

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

func (d *LUKSDevice) CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.CheckPassphrase")
	defer Logc(ctx).Debug("<<<< devices_darwin.CheckPassphrase")
	return false, errors.UnsupportedError("CheckPassphrase is not supported for darwin")
}

func (d *LUKSDevice) RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.RotatePassphrase")
	defer Logc(ctx).Debug("<<<< devices_darwin.RotatePassphrase")
	return errors.UnsupportedError("RotatePassphrase is not supported for darwin")
}

// IsFormatted returns whether LUKS headers have been placed on the device
func (d *LUKSDevice) IsFormatted(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.IsFormatted")
	defer Logc(ctx).Debug("<<<< devices_darwin.IsFormatted")
	return false, errors.UnsupportedError("IsFormatted is not supported for darwin")
}

// IsOpen returns whether the device is an active luks device on the host
func (d *LUKSDevice) IsOpen(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.IsOpen")
	defer Logc(ctx).Debug("<<<< devices_darwin.IsOpen")
	return false, errors.UnsupportedError("IsOpen is not supported for darwin")
}

// formatUnformattedDevice attempts to set up LUKS headers on a device with the specified passphrase, but bails if the
// underlying device already has a format present that is not LUKS.
func (d *LUKSDevice) formatUnformattedDevice(ctx context.Context, _ string) error {
	Logc(ctx).Debug(">>>> devices_darwin.formatUnformattedDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.formatUnformattedDevice")
	return errors.UnsupportedError("formatUnformattedDevice is not supported for darwin")
}

// Open makes the device accessible on the host
func (d *LUKSDevice) Open(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.Open")
	defer Logc(ctx).Debug("<<<< devices_darwin.Open")
	return errors.UnsupportedError("Open is not supported for darwin")
}

func GetUnderlyingDevicePathForDevice(ctx context.Context, command exec.Command, luksDevicePath string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.GetUnderlyingDevicePathForDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.GetUnderlyingDevicePathForDevice")
	return "", errors.UnsupportedError("GetUnderlyingDevicePathForDevice is not supported for darwin")
}

// Resize performs a luksResize on the LUKS device
func (d *LUKSDevice) Resize(ctx context.Context, luksPassphrase string) error {
	Logc(ctx).Debug(">>>> devices_darwin.Resize")
	defer Logc(ctx).Debug("<<<< devices_darwin.Resize")
	return errors.UnsupportedError("Resize is not supported for darwin")
}
