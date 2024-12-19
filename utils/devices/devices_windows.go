// Copyright 2024 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for windows flavor

package devices

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// FlushOneDevice unused stub function
func (c *Client) FlushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.FlushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.FlushOneDevice")
	return errors.UnsupportedError("FlushOneDevice is not supported for windows")
}

func (c *Client) ListAllDevices(ctx context.Context) {
	Logc(ctx).Debug(">>>> devices_windows.ListAllDevices")
	defer Logc(ctx).Debug("<<<< devices_windows.ListAllDevices")
	Logc(ctx).Debug("ListAllDevices is not supported for windows")
}

func (c *Client) WaitForDevice(ctx context.Context, device string) error {
	Logc(ctx).Debug(">>>> devices_windows.WaitForDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.WaitForDevice")
	return errors.UnsupportedError("ListAllDevices is not supported for windows")
}

func (c *Client) VerifyMultipathDeviceSize(ctx context.Context, multipathDevice, device string) (int64, bool, error) {
	Logc(ctx).Debug(">>>> devices_windows.VerifyMultipathDeviceSize")
	defer Logc(ctx).Debug("<<<< devices_windows.VerifyMultipathDeviceSize")
	return 0, false, errors.UnsupportedError("VerifyMultipathDeviceSize is not supported for windows")
}

// GetDiskSize unused stub function
func (c *DiskSizeGetter) GetDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_windows.GetDiskSize")
	defer Logc(ctx).Debug("<<<< devices_windows.GetDiskSize")
	return 0, errors.UnsupportedError("GetDiskSize is not supported for windows")
}

func (c *Client) EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_windows.EnsureLUKSDeviceClosed")
	defer Logc(ctx).Debug("<<<< devices_windows.EnsureLUKSDeviceClosed")
	return errors.UnsupportedError("EnsureLUKSDeviceClosed is not supported for windows")
}

func (c *Client) GetDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_windows.GetDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices_windows.GetDeviceFSType")
	return "", errors.UnsupportedError("GetDeviceFSTypeis not supported for windows")
}

func (c *Client) EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.EnsureLUKSDeviceClosedWithMaxWaitLimit")
	defer Logc(ctx).Debug("<<<< devices_windows.EnsureLUKSDeviceClosedWithMaxWaitLimit")
	return errors.UnsupportedError("EnsureLUKSDeviceClosedWithMaxWaitLimit is not supported for windows")
}

func (c *Client) CloseLUKSDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.CloseLUKSDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.CloseLUKSDevice")
	return errors.UnsupportedError("CloseLUKSDevice is not supported for windows")
}
