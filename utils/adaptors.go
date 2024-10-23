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

func (c DevicesClient) MultipathFlushDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo) error {
	return multipathFlushDevice(ctx, deviceInfo)
}

func (c DevicesClient) CompareWithPublishedDevicePath(
	ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	return compareWithPublishedDevicePath(ctx, publishInfo, deviceInfo)
}

func (c DevicesClient) CompareWithPublishedSerialNumber(
	ctx context.Context, publishInfo *models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) (bool, error) {
	return compareWithPublishedSerialNumber(ctx, publishInfo, deviceInfo)
}

func (c DevicesClient) CompareWithAllPublishInfos(
	ctx context.Context, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, deviceInfo *models.ScsiDeviceInfo,
) error {
	return compareWithAllPublishInfos(ctx, publishInfo, allPublishInfos, deviceInfo)
}

func (c DevicesClient) RemoveSCSIDevice(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, ignoreErrors,
	skipFlush bool,
) (bool, error) {
	return removeSCSIDevice(ctx, deviceInfo, ignoreErrors, skipFlush)
}

func (c DevicesClient) GetDeviceInfoForLUN(
	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, isDetachCall bool,
) (*models.ScsiDeviceInfo, error) {
	return GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, isDetachCall)
}

func (c DevicesClient) EnsureLUKSDeviceMappedOnHost(ctx context.Context, luksDevice models.LUKSDeviceInterface,
	name string, secrets map[string]string,
) (bool, error) {
	return EnsureLUKSDeviceMappedOnHost(ctx, luksDevice, name, secrets)
}

func (c DevicesClient) CloseLUKSDevice(ctx context.Context, devicePath string) error {
	return closeLUKSDevice(ctx, devicePath)
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

func (c DevicesClient) GetFCPDiskSize(ctx context.Context, devicePath string) (int64, error) {
	return getFCPDiskSize(ctx, devicePath)
}

func (c DevicesClient) GetMountedISCSIDevices(ctx context.Context) ([]*models.ScsiDeviceInfo, error) {
	return GetMountedISCSIDevices(ctx)
}

func (c DevicesClient) GetUnderlyingDevicePathForLUKSDevice(ctx context.Context, luksDevicePath string) (string, error) {
	return GetUnderlyingDevicePathForLUKSDevice(ctx, luksDevicePath)
}

func (c DevicesClient) RemoveMultipathDeviceMapping(ctx context.Context, devicePath string) error {
	return RemoveMultipathDeviceMapping(ctx, devicePath)
}

func (c DevicesClient) GetDMDeviceForMapperPath(ctx context.Context, mapperPath string) (string, error) {
	return GetDMDeviceForMapperPath(ctx, mapperPath)
}

func (c DevicesClient) EnsureLUKSDeviceClosed(ctx context.Context, devicePath string) error {
	return EnsureLUKSDeviceClosed(ctx, devicePath)
}

func (c DevicesClient) EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx context.Context, luksDevicePath string) error {
	return EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksDevicePath)
}

func (c DevicesClient) PrepareDeviceForRemoval(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool,
) (string, error) {
	return PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, allPublishInfos, ignoreErrors, force)
}

func (c DevicesClient) GetLUKSDeviceForMultipathDevice(multipathDevice string) (string, error) {
	return GetLUKSDeviceForMultipathDevice(multipathDevice)
}

func (c DevicesClient) NewLUKSDeviceFromMappingPath(ctx context.Context, mappingPath,
	volumeId string,
) (models.LUKSDeviceInterface, error) {
	return NewLUKSDeviceFromMappingPath(ctx, mappingPath, volumeId)
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
