// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"

	"github.com/netapp/trident/utils/models"
)

// TODO (vivintw) this file needs to be removed once all the files under utils are packaged correctly.

func GetDeviceInfoForLUN(
	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, isDetachCall bool,
) (*models.ScsiDeviceInfo, error) {
	return iscsiClient.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, isDetachCall)
}

func GetDeviceInfoForFCPLUN(
	ctx context.Context, hostSessionMap []map[string]int, lunID int, iSCSINodeName string, isDetachCall bool,
) (*models.ScsiDeviceInfo, error) {
	deviceInfo, err := fcpClient.GetDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, false)
	if err != nil {
		return nil, err
	}

	return &models.ScsiDeviceInfo{
		ScsiDeviceAddress: models.ScsiDeviceAddress{
			Host:    deviceInfo.Host,
			Channel: deviceInfo.Channel,
			Target:  deviceInfo.Target,
			LUN:     deviceInfo.LUN,
		},
		Devices:         deviceInfo.Devices,
		MultipathDevice: deviceInfo.MultipathDevice,
		WWNN:            deviceInfo.WWNN,
		SessionNumber:   deviceInfo.SessionNumber,
	}, nil
}

func PrepareDeviceForRemoval(ctx context.Context, deviceInfo *models.ScsiDeviceInfo, publishInfo *models.VolumePublishInfo,
	allPublishInfos []models.VolumePublishInfo, ignoreErrors, force bool,
) (string, error) {
	return iscsiClient.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, allPublishInfos, ignoreErrors, force)
}
