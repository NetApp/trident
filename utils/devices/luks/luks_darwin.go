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

// lUKSFormat attempts to set up LUKS headers on a device with the specified passphrase, but bails if the
// underlying device already has a format present that is not LUKS.
func (d *LUKSDevice) lUKSFormat(ctx context.Context, _ string) error {
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

func GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, command exec.Command, luksDevicePath string) (string, error) {
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
