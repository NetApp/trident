//go:build darwin

// Copyright 2025 NetApp, Inc. All Rights Reserved.

package blockdevice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestGetBlockDeviceStats_Darwin(t *testing.T) {
	ctx := context.Background()
	client := New()

	tests := []struct {
		name       string
		devicePath string
		protocol   string
	}{
		{
			name:       "iSCSI device",
			devicePath: "/dev/disk2",
			protocol:   "iscsi",
		},
		{
			name:       "NVMe device",
			devicePath: "/dev/nvme0n1",
			protocol:   "nvme",
		},
		{
			name:       "FC device",
			devicePath: "/dev/disk3",
			protocol:   "fc",
		},
		{
			name:       "Auto-detect protocol",
			devicePath: "/dev/disk1",
			protocol:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used, available, capacity, err := client.GetBlockDeviceStats(ctx, tt.devicePath, tt.protocol)

			assert.Equal(t, int64(0), used, "used should be 0 on darwin")
			assert.Equal(t, int64(0), available, "available should be 0 on darwin")
			assert.Equal(t, int64(0), capacity, "capacity should be 0 on darwin")
			assert.Error(t, err, "should return error on darwin")
			assert.True(t, errors.IsUnsupportedError(err), "should return UnsupportedError on darwin")
		})
	}
}
