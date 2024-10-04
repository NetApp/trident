// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"

	"github.com/netapp/trident/utils/models"
)

// TODO (vivintw) this file needs to be removed once all the files under utils are packaged correctly.

type OSClient struct{}

func NewOSClient() OSClient {
	return OSClient{}
}

func (c OSClient) PathExists(path string) (bool, error) {
	return PathExists(path)
}

type DevicesClient struct{}

func NewDevicesClient() DevicesClient {
	return DevicesClient{}
}

func (c DevicesClient) WaitForDevice(ctx context.Context, device string) error {
	return waitForDevice(ctx, device)
}

func (c DevicesClient) GetDeviceFSType(ctx context.Context, device string) (string, error) {
	return getDeviceFSType(ctx, device)
}

func (c DevicesClient) NewLUKSDevice(rawDevicePath, volumeId string) (models.LUKSDeviceInterface, error) {
	return NewLUKSDevice(rawDevicePath, volumeId)
}

func (c DevicesClient) EnsureLUKSDeviceMappedOnHost(ctx context.Context, luksDevice models.LUKSDeviceInterface,
	name string, secrets map[string]string,
) (bool, error) {
	return EnsureLUKSDeviceMappedOnHost(ctx, luksDevice, name, secrets)
}

func (c DevicesClient) IsDeviceUnformatted(ctx context.Context, device string) (bool, error) {
	return isDeviceUnformatted(ctx, device)
}

func (c DevicesClient) EnsureDeviceReadable(ctx context.Context, device string) error {
	return ensureDeviceReadable(ctx, device)
}

func (c DevicesClient) GetISCSIDiskSize(ctx context.Context, devicePath string) (int64, error) {
	return getISCSIDiskSize(ctx, devicePath)
}

type FilesystemClient struct{}

func NewFilesystemClient() FilesystemClient {
	return FilesystemClient{}
}

func (c FilesystemClient) FormatVolume(ctx context.Context, device, fstype, options string) error {
	return formatVolume(ctx, device, fstype, options)
}

func (c FilesystemClient) RepairVolume(ctx context.Context, device, fstype string) {
	repairVolume(ctx, device, fstype)
}

type MountClient struct{}

func NewMountClient() MountClient {
	return MountClient{}
}

func (c MountClient) IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	return IsMounted(ctx, sourceDevice, mountpoint, mountOptions)
}

func (c MountClient) MountDevice(ctx context.Context, device, mountpoint, options string, isMountPointFile bool) (err error) {
	return MountDevice(ctx, device, mountpoint, options, isMountPointFile)
}
