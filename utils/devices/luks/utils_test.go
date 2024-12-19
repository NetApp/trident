// Copyright 2024 NetApp, Inc. All Rights Reserved.

package luks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLegacyLUKSDevicePath(t *testing.T) {
	tests := map[string]struct {
		name       string
		devicePath string
		expected   bool
	}{
		"legacy luks device path": {
			devicePath: "/dev/mapper/luks-trident_pvc_4b7874ba_58d7_4d93_8d36_09a09b837f81",
			expected:   true,
		},
		"non-legacy luks device path": {
			devicePath: "/dev/mapper/mpath-36001405b09b0d1f4d0000000000000a1",
			expected:   false,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expected, IsLegacyLUKSDevicePath(params.devicePath))
		})
	}
}
