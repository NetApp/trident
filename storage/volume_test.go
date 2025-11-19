// Copyright 2019 NetApp, Inc. All Rights Reserved.

package storage

import (
	"fmt"
	"testing"

	"github.com/netapp/trident/utils/models"

	"github.com/stretchr/testify/assert"
)

func TestVolumeState(t *testing.T) {
	tests := map[string]struct {
		input     VolumeState
		output    string
		predicate func(VolumeState) bool
	}{
		"Unknown state (bad)": {
			input:  "",
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Unknown state": {
			input:  VolumeStateUnknown,
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Online state": {
			input:  VolumeStateOnline,
			output: "online",
			predicate: func(input VolumeState) bool {
				return input.IsOnline()
			},
		},
		"Deleting state": {
			input:  VolumeStateDeleting,
			output: "deleting",
			predicate: func(input VolumeState) bool {
				return input.IsDeleting()
			},
		},
		"Upgrading state": {
			input:  VolumeStateUpgrading,
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Missing backend state": {
			input:  VolumeStateMissingBackend,
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Subordinate state": {
			input:  VolumeStateSubordinate,
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Custom invalid state": {
			input:  VolumeState("invalid"),
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)

		assert.Equal(t, test.input.String(), test.output, "Strings not equal")
		assert.True(t, test.predicate(test.input), "Predicate failed")
	}
}

func TestVolumeConfig_ConstructClone(t *testing.T) {
	tests := map[string]*VolumeConfig{
		"IscsiAccessInfoIsCloned": constructIscsiVolumeConfig(),
		"NfsAccessInfoIsCloned":   constructNFSVolumeConfig(),
		"SMBAccessInfoIsCloned":   constructSMBVolumeConfig(),
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			copy := test.ConstructClone()
			testConfigAddress := fmt.Sprintf("%p", &test)
			copyConfigAddress := fmt.Sprintf("%p", &copy)
			testAccessInfoAddress := fmt.Sprintf("%p", &test.AccessInfo)
			copyAccessInfoAddress := fmt.Sprintf("%p", &copy.AccessInfo)

			assert.EqualValues(t, test, copy, "Expected equal values")
			assert.NotEqual(t, testConfigAddress, copyConfigAddress, "Expected different addresses")
			assert.NotEqual(t, testAccessInfoAddress, copyAccessInfoAddress, "Expected different addresses")
		})
	}
}

func constructIscsiVolumeConfig() *VolumeConfig {
	return &VolumeConfig{
		Version:      "1",
		Name:         "foo",
		InternalName: "internal_foo",
		Size:         "1Gi",
		StorageClass: "san-sc",
		AccessMode:   "0",
		AccessInfo: models.VolumeAccessInfo{
			IscsiAccessInfo: models.IscsiAccessInfo{
				IscsiTargetPortal: "10.0.0.0",
				IscsiPortals: []string{
					"10.0.0.0", "10.0.0.1",
				},
				IscsiTargetIQN: "",
				IscsiLunNumber: 0,
				IscsiInterface: "",
				IscsiIgroup:    "per-node-igroup",
				IscsiChapInfo: models.IscsiChapInfo{
					UseCHAP:              false,
					IscsiUsername:        "user",
					IscsiInitiatorSecret: "shh!",
					IscsiTargetUsername:  "target",
					IscsiTargetSecret:    "ssh!",
				},
			},
			PublishEnforcement: true,
			ReadOnly:           false,
			AccessMode:         0,
		},
		BlockSize: "1",
	}
}

func constructNFSVolumeConfig() *VolumeConfig {
	return &VolumeConfig{
		Version:      "1",
		Name:         "foo",
		InternalName: "internal_foo",
		Size:         "1Gi",
		StorageClass: "nas-sc",
		AccessMode:   "1",
		AccessInfo: models.VolumeAccessInfo{
			NfsAccessInfo: models.NfsAccessInfo{
				NfsServerIP: "10.0.0.0",
				NfsPath:     "/nfsshare",
				NfsUniqueID: "uuid4",
			},
			PublishEnforcement: false,
			ReadOnly:           false,
			AccessMode:         1,
		},
		FileSystem: "ext4",
	}
}

func constructSMBVolumeConfig() *VolumeConfig {
	return &VolumeConfig{
		Version:      "1",
		Name:         "foo",
		InternalName: "internal_foo",
		Size:         "1Gi",
		StorageClass: "nas-sc",
		AccessMode:   "1",
		AccessInfo: models.VolumeAccessInfo{
			SMBAccessInfo: models.SMBAccessInfo{
				SMBServer: "server",
				SMBPath:   "/smbshare",
			},
			PublishEnforcement: false,
			ReadOnly:           false,
			AccessMode:         1,
		},
		FileSystem: "ext4",
	}
}

func TestVolume_SmartCopy(t *testing.T) {
	// Create a volume
	volume := NewVolume(constructIscsiVolumeConfig(), "uuid", "pool", false,
		VolumeStateOnline)

	// Create a deep copy of the volume
	copiedVolume := volume.SmartCopy().(*Volume)

	// Check that the copied volume is deeply equal to the original
	assert.Equal(t, volume, copiedVolume)

	// Check that the copied volume does not point to the same memory
	assert.False(t, volume == copiedVolume)
	assert.False(t, volume.Config == copiedVolume.Config)
}
