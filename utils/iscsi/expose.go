// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
)

// this is a temporary file that will be used to expose the internal functions of the iscsi package till we move
// all the iscsi ui related functions to the iscsi package
// TODO (vivintw) remove this file once the refactoring is done.

func (client *Client) ExecIscsiadmCommand(ctx context.Context, args ...string) ([]byte, error) {
	return client.execIscsiadmCommand(ctx, args...)
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

func (client *Client) IsSessionStale(ctx context.Context, sessionID string) bool {
	return client.isSessionStale(ctx, sessionID)
}
