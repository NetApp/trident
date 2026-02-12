//go:build windows

// Copyright 2025 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling block devices for Windows flavor

package blockdevice

import (
	"context"

	"github.com/netapp/trident/utils/errors"
)

// GetBlockDeviceStats is not supported on Windows
func (c *Client) GetBlockDeviceStats(ctx context.Context, devicePath, protocol string) (used, available, capacity int64, err error) {
	return 0, 0, 0, errors.UnsupportedError("GetBlockDeviceStats is not supported on Windows")
}
