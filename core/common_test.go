// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

func TestGetProtocol(t *testing.T) {
	type accessVariables struct {
		volumeMode config.VolumeMode
		accessMode config.AccessMode
		protocol   config.Protocol
		expected   config.Protocol
	}

	accessModesPositiveTests := []accessVariables{
		// This is the complete set of permutations.  Negative rows are commented out.
		{config.Filesystem, config.ModeAny, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ModeAny, config.File, config.File},
		{config.Filesystem, config.ModeAny, config.Block, config.Block},
		{config.Filesystem, config.ReadWriteOnce, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteOnce, config.File, config.File},
		{config.Filesystem, config.ReadWriteOnce, config.Block, config.Block},
		{config.Filesystem, config.ReadWriteOncePod, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteOncePod, config.File, config.File},
		{config.Filesystem, config.ReadWriteOncePod, config.Block, config.Block},
		{config.Filesystem, config.ReadOnlyMany, config.Block, config.Block},
		{config.Filesystem, config.ReadOnlyMany, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadOnlyMany, config.File, config.File},
		{config.Filesystem, config.ReadWriteMany, config.ProtocolAny, config.File},
		{config.Filesystem, config.ReadWriteMany, config.File, config.File},
		// {config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteOnce, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		// {config.RawBlock, config.ReadWriteOncePod, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteOncePod, config.Block, config.Block},
		{config.RawBlock, config.ReadOnlyMany, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadOnlyMany, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteMany, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.Block, config.Block},
	}

	accessModesNegativeTests := []accessVariables{
		{config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOncePod, config.File, config.ProtocolAny},

		{config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
	}

	for _, tc := range accessModesPositiveTests {
		protocolLocal, err := getProtocol(coreCtx, tc.volumeMode, tc.accessMode, tc.protocol)
		assert.Nil(t, err, nil)
		assert.Equal(t, tc.expected, protocolLocal, "expected both the protocols to be equal!")
	}

	for _, tc := range accessModesNegativeTests {
		protocolLocal, err := getProtocol(coreCtx, tc.volumeMode, tc.accessMode, tc.protocol)
		assert.NotNil(t, err)
		assert.Equal(t, tc.expected, protocolLocal, "expected both the protocols to be equal!")
	}
}

func TestGenerateVolumePublication(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Name: "test-volume",
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:     "test-node",
		StorageClass: "premium",
		BackendUUID:  "backend-uuid-123",
		Pool:         "pool-xyz",
		VolumeAccessInfo: models.VolumeAccessInfo{
			ReadOnly:   true,
			AccessMode: 5,
		},
	}

	result := generateVolumePublication(volConfig.Name, publishInfo, volConfig)

	// Verify all fields are set correctly
	assert.NotNil(t, result)
	assert.Equal(t, models.GenerateVolumePublishName(volConfig.Name, publishInfo.HostName), result.Name)
	assert.Equal(t, volConfig.Name, result.VolumeName)
	assert.Equal(t, publishInfo.HostName, result.NodeName)
	assert.Equal(t, publishInfo.ReadOnly, result.ReadOnly)
	assert.Equal(t, publishInfo.AccessMode, result.AccessMode)
	assert.Equal(t, publishInfo.StorageClass, result.StorageClass)
	assert.Equal(t, publishInfo.BackendUUID, result.BackendUUID)
	assert.Equal(t, publishInfo.Pool, result.Pool)
}

func TestGenerateVolumePublication_EmptyFields(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Name: "test-volume",
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:     "test-node",
		StorageClass: "",
		BackendUUID:  "",
		Pool:         "",
		VolumeAccessInfo: models.VolumeAccessInfo{
			ReadOnly:   false,
			AccessMode: 1,
		},
	}

	result := generateVolumePublication(volConfig.Name, publishInfo, volConfig)

	// Verify empty fields are preserved
	assert.NotNil(t, result)
	assert.Equal(t, "", result.StorageClass)
	assert.Equal(t, "", result.BackendUUID)
	assert.Equal(t, "", result.Pool)
}

func TestGenerateVolumePublication_AllFieldsPopulated(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Name: "my-important-volume",
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:     "worker-node-01",
		StorageClass: "gold-storage",
		BackendUUID:  "ontap-nas-backend-uuid",
		Pool:         "aggr1",
		VolumeAccessInfo: models.VolumeAccessInfo{
			ReadOnly:   false,
			AccessMode: 3,
		},
	}

	result := generateVolumePublication(volConfig.Name, publishInfo, volConfig)

	// Verify the generated publication has all expected values
	assert.NotNil(t, result)
	assert.Contains(t, result.Name, volConfig.Name)
	assert.Contains(t, result.Name, publishInfo.HostName)
	assert.Equal(t, volConfig.Name, result.VolumeName)
	assert.Equal(t, "worker-node-01", result.NodeName)
	assert.Equal(t, false, result.ReadOnly)
	assert.Equal(t, int32(3), result.AccessMode)
	assert.Equal(t, "gold-storage", result.StorageClass)
	assert.Equal(t, "ontap-nas-backend-uuid", result.BackendUUID)
	assert.Equal(t, "aggr1", result.Pool)
}

// TestIsVolumeAutogrowIneligible tests the eligibility logic for autogrow monitoring
func TestIsVolumeAutogrowIneligible(t *testing.T) {
	tests := []struct {
		name     string
		config   *storage.VolumeConfig
		expected bool
	}{
		{
			name: "UnmanagedImport",
			config: &storage.VolumeConfig{
				ImportNotManaged: true,
			},
			expected: true,
		},
		{
			name: "ManagedImportThickProvisioning",
			config: &storage.VolumeConfig{
				ImportOriginalName: "original-vol",
				SpaceReserve:       "volume",
				Protocol:           "block",
				VolumeMode:         config.VolumeMode("Block"),
				AccessMode:         config.ReadWriteOnce,
			},
			expected: true,
		},
		{
			name: "ManagedImportThickProvisioningNAS",
			config: &storage.VolumeConfig{
				ImportOriginalName: "original-vol",
				SpaceReserve:       "volume",
				Protocol:           "file",
				VolumeMode:         config.VolumeMode("Filesystem"),
				AccessMode:         config.ReadWriteMany,
			},
			expected: false,
		},
		{
			name: "ManagedImportThinProvisioning",
			config: &storage.VolumeConfig{
				ImportOriginalName: "original-vol",
				SpaceReserve:       "none",
				Protocol:           "block",
			},
			expected: false,
		},
		{
			name: "ManagedImportThinProvisioningEmptyString",
			config: &storage.VolumeConfig{
				ImportOriginalName: "original-vol",
				SpaceReserve:       "",
				Protocol:           "block",
			},
			expected: false,
		},
		{
			name: "ReadOnlyClone",
			config: &storage.VolumeConfig{
				ReadOnlyClone: true,
			},
			expected: true,
		},
		{
			name:     "NormalVolume",
			config:   &storage.VolumeConfig{},
			expected: false,
		},
		{
			name: "NormalVolumeWithThinProvisioning",
			config: &storage.VolumeConfig{
				SpaceReserve: "none",
			},
			expected: false,
		},
		{
			name: "NormalVolumeWithThickProvisioning",
			config: &storage.VolumeConfig{
				SpaceReserve: "volume",
			},
			expected: false,
		},
		{
			name: "UnmanagedImportWithThickProvisioning",
			config: &storage.VolumeConfig{
				ImportNotManaged: true,
				SpaceReserve:     "volume",
			},
			expected: true,
		},
		{
			name: "ReadOnlyCloneWithImport",
			config: &storage.VolumeConfig{
				ReadOnlyClone:      true,
				ImportOriginalName: "original",
			},
			expected: true,
		},
		{
			name: "SubordinateVolume",
			config: &storage.VolumeConfig{
				ShareSourceVolume: "source-vol",
			},
			expected: false,
		},
		{
			name: "SubordinateVolumeWithOtherFlags",
			config: &storage.VolumeConfig{
				ShareSourceVolume: "source-vol",
				SpaceReserve:      "none",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVolumeAutogrowIneligible(tt.config)
			assert.Equal(t, tt.expected, result, "Expected isVolumeAutogrowIneligible to return %v for %s", tt.expected, tt.name)
		})
	}
}
