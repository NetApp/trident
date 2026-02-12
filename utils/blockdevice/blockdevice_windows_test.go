//go:build windows

// Copyright 2025 NetApp, Inc. All Rights Reserved.

package blockdevice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestGetBlockDeviceStats_Windows(t *testing.T) {
	ctx := context.Background()
	client := New()

	tests := []struct {
		name       string
		devicePath string
		protocol   string
	}{
		{
			name:       "iSCSI device",
			devicePath: "\\\\.\\PhysicalDrive1",
			protocol:   "iscsi",
		},
		{
			name:       "NVMe device",
			devicePath: "\\\\.\\PhysicalDrive2",
			protocol:   "nvme",
		},
		{
			name:       "FC device",
			devicePath: "\\\\.\\PhysicalDrive3",
			protocol:   "fc",
		},
		{
			name:       "Auto-detect protocol",
			devicePath: "\\\\.\\PhysicalDrive0",
			protocol:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used, available, capacity, err := client.GetBlockDeviceStats(ctx, tt.devicePath, tt.protocol)

			assert.Equal(t, int64(0), used, "used should be 0 on windows")
			assert.Equal(t, int64(0), available, "available should be 0 on windows")
			assert.Equal(t, int64(0), capacity, "capacity should be 0 on windows")
			assert.Error(t, err, "should return error on windows")
			assert.True(t, errors.IsUnsupportedError(err), "should return UnsupportedError on windows")
		})
	}
}
