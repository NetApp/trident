// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

import "context"

// this is a temporary file that will be used to expose the internal functions of the iscsi package till we move
// all the iscsi ui related functions to the iscsi package
// TODO (vivintw) remove this file once the refactoring is done.

func ListAllDevices(ctx context.Context) {
	listAllDevices(ctx)
}

func (client *Client) ScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	return client.scanTargetLUN(ctx, lunID, hosts)
}

func (client *Client) GetDeviceInfoForLUN(
	ctx context.Context, lunID int, iSCSINodeName string, needFSType, isDetachCall bool,
) (*ScsiDeviceInfo, error) {
	return client.getDeviceInfoForLUN(ctx, lunID, iSCSINodeName, needFSType, isDetachCall)
}

func (client *Client) FindMultipathDeviceForDevice(ctx context.Context, device string) string {
	return client.findMultipathDeviceForDevice(ctx, device)
}

func GetLunSerial(ctx context.Context, path string) (string, error) {
	return getLunSerial(ctx, path)
}
