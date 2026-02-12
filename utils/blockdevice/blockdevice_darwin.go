//go:build darwin

// Copyright 2025 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling block devices for Darwin flavor

package blockdevice

import (
	"context"

	"github.com/netapp/trident/utils/errors"
)

// GetBlockDeviceStats is not supported on macOS
func (c *Client) GetBlockDeviceStats(ctx context.Context, devicePath, protocol string) (used, available, capacity int64, err error) {
	return 0, 0, 0, errors.UnsupportedError("GetBlockDeviceStats is not supported on macOS")
}
