// Copyright 2025 NetApp, Inc. All Rights Reserved.

package blockdevice

//go:generate mockgen -destination=../../mocks/mock_utils/mock_blockdevice/mock_blockdevice_client.go github.com/netapp/trident/utils/blockdevice BlockDevice

import (
	"context"
)

// BlockDevice defines interface for block device statistics
type BlockDevice interface {
	// GetBlockDeviceStats returns capacity statistics for a block device
	// devicePath: path to block device (e.g., /dev/sda, /dev/mapper/mpatha, /dev/nvme0n1)
	// protocol: storage protocol type ("iscsi", "nvme", "fc", or empty for auto-detection)
	// Returns: (used, available, capacity, error)
	GetBlockDeviceStats(ctx context.Context, devicePath, protocol string) (used, available, capacity int64, err error)
}

// Client manages block device operations
type Client struct {
	// Client implementation is OS-specific
}

// New creates a new BlockDevice client
func New() *Client {
	return &Client{}
}
