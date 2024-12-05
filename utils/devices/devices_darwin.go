// Copyright 2024 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for Darwin flavor

package devices

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// flushOneDevice unused stub function
func (c *Client) FlushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.FlushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.FlushOneDevice")
	return errors.UnsupportedError("FlushOneDevice is not supported for darwin")
}

func (c *Client) ListAllDevices(ctx context.Context) {
	Logc(ctx).Debug(">>>> devices_darwin.ListAllDevices")
	defer Logc(ctx).Debug("<<<< devices_darwin.ListAllDevices")
	Logc(ctx).Debug("ListAllDevices is not supported for darwin")
}

func (c *Client) WaitForDevice(ctx context.Context, device string) error {
	Logc(ctx).Debug(">>>> devices_darwin.WaitForDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.WaitForDevice")
	return errors.UnsupportedError("ListAllDevices is not supported for darwin")
}

func (c *Client) VerifyMultipathDeviceSize(ctx context.Context, multipathDevice, device string) (int64, bool, error) {
	Logc(ctx).Debug(">>>> devices_darwin.VerifyMultipathDeviceSize")
	defer Logc(ctx).Debug("<<<< devices_darwin.VerifyMultipathDeviceSize")
	return 0, false, errors.UnsupportedError("VerifyMultipathDeviceSize is not supported for darwin")
}

// GetDiskSize unused stub function
func (c *Client) GetDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_darwin.GetDiskSize")
	defer Logc(ctx).Debug("<<<< devices_darwin.GetDiskSize")
	return 0, errors.UnsupportedError("GetDiskSize is not supported for darwin")
}

func (c *Client) EnsureLUKSDeviceClosed(ctx context.Context, luksDevicePath string) error {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> devices_darwin.EnsureLUKSDeviceClosed")
	defer Logc(ctx).Debug("<<<< devices_darwin.EnsureLUKSDeviceClosed")
	return errors.UnsupportedError("EnsureLUKSDeviceClosed is not supported for darwin")
}

func (c *Client) GetDeviceFSType(ctx context.Context, device string) (string, error) {
	Logc(ctx).Debug(">>>> devices_darwin.GetDeviceFSType")
	defer Logc(ctx).Debug("<<<< devices_darwin.GetDeviceFSType")
	return "", errors.UnsupportedError("GetDeviceFSTypeis not supported for darwin")
}

func (c *Client) EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.EnsureLUKSDeviceClosedWithMaxWaitLimit")
	defer Logc(ctx).Debug("<<<< devices_darwin.EnsureLUKSDeviceClosedWithMaxWaitLimit")
	return errors.UnsupportedError("EnsureLUKSDeviceClosedWithMaxWaitLimit is not supported for darwin")
}

func (c *Client) CloseLUKSDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.CloseLUKSDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.CloseLUKSDevice")
	return errors.UnsupportedError("CloseLUKSDevice is not supported for darwin")
}
