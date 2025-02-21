// Copyright 2025 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"

	"github.com/netapp/trident/utils/models"
)

// this is a temporary file that will be used to expose the internal functions of the iscsi package till we move
// all the iscsi ui related functions to the iscsi package
// TODO (vivintw) remove this file once the refactoring is done.

func (client *Client) ExecIscsiadmCommand(ctx context.Context, args ...string) ([]byte, error) {
	return client.execIscsiadmCommand(ctx, args...)
}

func (client *Client) ListAllDevices(ctx context.Context) {
	client.listAllDevices(ctx)
}

func (client *Client) GetSessionInfo(ctx context.Context) ([]SessionInfo, error) {
	return client.getSessionInfo(ctx)
}

func (client *Client) Supported(ctx context.Context) bool {
	return client.supported(ctx)
}

func (client *Client) SessionExists(ctx context.Context, portal string) (bool, error) {
	return client.sessionExists(ctx, portal)
}

func (client *Client) ConfigureTarget(ctx context.Context, iqn, portal, name, value string) error {
	return client.configureTarget(ctx, iqn, portal, name, value)
}

func (client *Client) ScanTargetLUN(ctx context.Context, lunID int, hosts []int) error {
	return client.scanTargetLUN(ctx, lunID, hosts)
}

func (client *Client) GetDeviceInfoForLUN(
	ctx context.Context, hostSessionMap map[int]int, lunID int, iSCSINodeName string, needFSType bool,
) (*models.ScsiDeviceInfo, error) {
	return client.getDeviceInfoForLUN(ctx, hostSessionMap, lunID, iSCSINodeName, needFSType)
}

func (client *Client) FindMultipathDeviceForDevice(ctx context.Context, device string) string {
	return client.findMultipathDeviceForDevice(ctx, device)
}

func (client *Client) GetLunSerial(ctx context.Context, path string) (string, error) {
	return client.getLunSerial(ctx, path)
}

func (client *Client) GetSessionState(ctx context.Context, sessionID string) string {
	return client.getSessionState(ctx, sessionID)
}

func (client *Client) GetSessionConnectionsState(ctx context.Context, sessionID string) []string {
	return client.getSessionConnectionsState(ctx, sessionID)
}

func (client *Client) IsSessionStale(ctx context.Context, sessionID string) bool {
	return client.isSessionStale(ctx, sessionID)
}
