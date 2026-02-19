// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"

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
				Protocol:     "file",
				VolumeMode:   config.VolumeMode("Filesystem"),
				AccessMode:   config.ReadWriteMany,
			},
			expected: false, // NAS thick volumes are eligible
		},
		{
			name: "FreshThickProvisioningSAN",
			config: &storage.VolumeConfig{
				SpaceReserve: "volume",
				Protocol:     "block",
				VolumeMode:   config.VolumeMode("Block"),
				AccessMode:   config.ReadWriteOnce,
			},
			expected: true, // Fresh thick SAN volumes are now ineligible
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

// TestLabelsNeedSync tests the label synchronization comparison logic
func TestLabelsNeedSync(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]string
		desired  map[string]string
		expected bool
	}{
		{
			name:     "BothNil",
			current:  nil,
			desired:  nil,
			expected: false,
		},
		{
			name:     "CurrentNilDesiredEmpty",
			current:  nil,
			desired:  map[string]string{},
			expected: false,
		},
		{
			name:     "CurrentEmptyDesiredNil",
			current:  map[string]string{},
			desired:  nil,
			expected: false,
		},
		{
			name:     "BothEmpty",
			current:  map[string]string{},
			desired:  map[string]string{},
			expected: false,
		},
		{
			name:    "DesiredLabelMissing",
			current: map[string]string{},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
			expected: true,
		},
		{
			name: "DesiredLabelValueMismatch",
			current: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-2",
			},
			expected: true,
		},
		{
			name: "LabelsMatch",
			current: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
			expected: false,
		},
		{
			name: "CurrentHasExtraLabels",
			current: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"extra-label":               "extra-value",
			},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
			expected: false, // Extra labels don't trigger sync
		},
		{
			name: "MultipleDesiredLabelsAllMatch",
			current: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"label-2":                   "value-2",
			},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"label-2":                   "value-2",
			},
			expected: false,
		},
		{
			name: "MultipleDesiredLabelsOneMismatch",
			current: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"label-2":                   "value-2",
			},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"label-2":                   "value-3", // Mismatch
			},
			expected: true,
		},
		{
			name: "MultipleDesiredLabelsOneMissing",
			current: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
			desired: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"label-2":                   "value-2", // Missing
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := labelsNeedSync(tt.current, tt.desired)
			assert.Equal(t, tt.expected, result, "Expected labelsNeedSync to return %v for %s", tt.expected, tt.name)
		})
	}
}

// TestSyncVolumePublicationFields tests the synchronization of VP fields from Volume
func TestSyncVolumePublicationFields(t *testing.T) {
	tests := []struct {
		name                   string
		vol                    *storage.Volume
		vp                     *models.VolumePublication
		expectedSyncNeeded     bool
		expectedStorageClass   string
		expectedBackendUUID    string
		expectedPool           string
		expectedAutogrowPolicy string
		expectedAutogrowInelig bool
		expectedLabels         map[string]string
	}{
		{
			name: "NoSyncNeeded_AllFieldsMatch",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     false,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_StorageClassChanged",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "gold",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "gold",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_BackendUUIDChanged",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-456",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-456",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_PoolChanged",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-2",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-2",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_AutogrowPolicyChanged",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-2",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-2",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_AutogrowIneligibleChanged",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
					ImportNotManaged:        true, // Makes it ineligible
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: true,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_LabelsMissing",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels:             map[string]string{}, // Missing node label
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_LabelsNil",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels:             nil, // Nil labels
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_LabelValueMismatch",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "wrong-node",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "NoSyncNeeded_ExtraLabelsPreserved",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
					"extra-label":               "extra-value",
				},
			},
			expectedSyncNeeded:     false,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
				"extra-label":               "extra-value",
			},
		},
		{
			name: "SyncNeeded_MultipleFieldsChanged",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "gold",
					RequestedAutogrowPolicy: "policy-3",
					ReadOnlyClone:           true, // Makes it ineligible
				},
				BackendUUID: "backend-789",
				Pool:        "pool-3",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "wrong-node",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "gold",
			expectedBackendUUID:    "backend-789",
			expectedPool:           "pool-3",
			expectedAutogrowPolicy: "policy-3",
			expectedAutogrowInelig: true,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_AutogrowIneligibleDoesNotBlockOtherSyncs",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "platinum",
					RequestedAutogrowPolicy: "policy-4",
					ImportNotManaged:        true, // Makes it ineligible
				},
				BackendUUID: "backend-999",
				Pool:        "pool-4",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-2",
				StorageClass:       "gold",
				BackendUUID:        "backend-888",
				Pool:               "pool-3",
				AutogrowPolicy:     "policy-3",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-2",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "platinum",
			expectedBackendUUID:    "backend-999",
			expectedPool:           "pool-4",
			expectedAutogrowPolicy: "policy-4",
			expectedAutogrowInelig: true,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-2",
			},
		},
		{
			name: "SyncNeeded_EmptyToPopulatedFields",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "premium",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "",
				BackendUUID:        "",
				Pool:               "",
				AutogrowPolicy:     "",
				AutogrowIneligible: false,
				Labels:             nil,
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "premium",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_PopulatedToEmptyFields",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "",
					RequestedAutogrowPolicy: "",
				},
				BackendUUID: "",
				Pool:        "",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "premium",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "",
			expectedBackendUUID:    "",
			expectedPool:           "",
			expectedAutogrowPolicy: "",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_BackendUpdateScenario_AllBackendFieldsChange",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "gold",
					RequestedAutogrowPolicy: "policy-1",
				},
				BackendUUID: "new-backend-uuid", // Changed during backend update
				Pool:        "new-pool",         // Changed with backend
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "gold", // Same
				BackendUUID:        "old-backend-uuid",
				Pool:               "old-pool",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "gold",
			expectedBackendUUID:    "new-backend-uuid", // Updated
			expectedPool:           "new-pool",         // Updated
			expectedAutogrowPolicy: "policy-1",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_BackendUpdateWithStorageClassChange",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "platinum", // Also changed
					RequestedAutogrowPolicy: "policy-2",
				},
				BackendUUID: "new-backend-xyz",
				Pool:        "new-pool-xyz",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "gold",
				BackendUUID:        "old-backend-xyz",
				Pool:               "old-pool-xyz",
				AutogrowPolicy:     "policy-1",
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "platinum",        // Updated
			expectedBackendUUID:    "new-backend-xyz", // Updated
			expectedPool:           "new-pool-xyz",    // Updated
			expectedAutogrowPolicy: "policy-2",        // Updated
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_AutogrowPolicyPropagation_RWXScenario",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "shared-storage",
					RequestedAutogrowPolicy: "aggressive-new", // Policy updated
					AccessMode:              config.ReadWriteMany,
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-2", // One of multiple nodes
				StorageClass:       "shared-storage",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "aggressive-old", // Old policy
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-2",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "shared-storage",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "aggressive-new", // Updated policy
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-2",
			},
		},
		{
			name: "SyncNeeded_AutogrowPolicyRemoved_RWXScenario",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "shared-storage",
					RequestedAutogrowPolicy: "", // Policy removed
					AccessMode:              config.ReadWriteMany,
				},
				BackendUUID: "backend-123",
				Pool:        "pool-1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-3",
				StorageClass:       "shared-storage",
				BackendUUID:        "backend-123",
				Pool:               "pool-1",
				AutogrowPolicy:     "policy-old", // Had policy
				AutogrowIneligible: false,
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-3",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "shared-storage",
			expectedBackendUUID:    "backend-123",
			expectedPool:           "pool-1",
			expectedAutogrowPolicy: "", // Cleared
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-3",
			},
		},
		{
			name: "SyncNeeded_BootstrapUpgrade_AllLegacyFieldsEmpty",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "gold",
					RequestedAutogrowPolicy: "conservative",
				},
				BackendUUID: "backend-abc",
				Pool:        "pool-xyz",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "", // Empty in old version
				BackendUUID:        "", // Empty in old version
				Pool:               "", // Empty in old version
				AutogrowPolicy:     "", // Empty in old version
				AutogrowIneligible: false,
				Labels:             nil, // Nil in old version
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "gold",
			expectedBackendUUID:    "backend-abc",
			expectedPool:           "pool-xyz",
			expectedAutogrowPolicy: "conservative",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
		{
			name: "SyncNeeded_BootstrapUpgrade_PartialLegacyFields",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "silver",
					RequestedAutogrowPolicy: "moderate",
				},
				BackendUUID: "backend-def",
				Pool:        "pool-abc",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-4",
				StorageClass:       "silver",            // Already set
				BackendUUID:        "",                  // Missing
				Pool:               "",                  // Missing
				AutogrowPolicy:     "",                  // Missing
				AutogrowIneligible: false,               // Default
				Labels:             map[string]string{}, // Empty map
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "silver",
			expectedBackendUUID:    "backend-def",
			expectedPool:           "pool-abc",
			expectedAutogrowPolicy: "moderate",
			expectedAutogrowInelig: false,
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-4",
			},
		},
		{
			name: "SyncNeeded_ImportNASWithThickProvisioning_CorrectlyEligible",
			vol: &storage.Volume{
				Config: &storage.VolumeConfig{
					StorageClass:            "nas-gold",
					RequestedAutogrowPolicy: "aggressive",
					ImportOriginalName:      "original-nas-vol",
					SpaceReserve:            "volume", // Thick provisioning
					Protocol:                config.File,
					VolumeMode:              config.Filesystem,
					AccessMode:              config.ReadWriteMany,
				},
				BackendUUID: "backend-nas-1",
				Pool:        "aggr1",
			},
			vp: &models.VolumePublication{
				NodeName:           "node-1",
				StorageClass:       "nas-gold",
				BackendUUID:        "backend-nas-1",
				Pool:               "aggr1",
				AutogrowPolicy:     "aggressive",
				AutogrowIneligible: true, // Incorrectly marked ineligible initially
				Labels: map[string]string{
					config.TridentNodeNameLabel: "node-1",
				},
			},
			expectedSyncNeeded:     true,
			expectedStorageClass:   "nas-gold",
			expectedBackendUUID:    "backend-nas-1",
			expectedPool:           "aggr1",
			expectedAutogrowPolicy: "aggressive",
			expectedAutogrowInelig: false, // Should be eligible (NAS volumes can grow even with thick provisioning)
			expectedLabels: map[string]string{
				config.TridentNodeNameLabel: "node-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncVolumePublicationFields(tt.vol, tt.vp)
			assert.Equal(t, tt.expectedSyncNeeded, result, "Expected syncVolumePublicationFields to return %v for %s", tt.expectedSyncNeeded, tt.name)

			// Verify all fields are correctly updated regardless of sync result
			assert.Equal(t, tt.expectedStorageClass, tt.vp.StorageClass, "StorageClass mismatch")
			assert.Equal(t, tt.expectedBackendUUID, tt.vp.BackendUUID, "BackendUUID mismatch")
			assert.Equal(t, tt.expectedPool, tt.vp.Pool, "Pool mismatch")
			assert.Equal(t, tt.expectedAutogrowPolicy, tt.vp.AutogrowPolicy, "AutogrowPolicy mismatch")
			assert.Equal(t, tt.expectedAutogrowInelig, tt.vp.AutogrowIneligible, "AutogrowIneligible mismatch")
			assert.Equal(t, tt.expectedLabels, tt.vp.Labels, "Labels mismatch")
		})
	}
}

// TestVPSyncRateLimiting_BulkScenario tests rate limiting behavior used in SyncVolumePublications
// simulating multiple VPs needing sync (e.g., after upgrade or policy update to RWX volume)
func TestVPSyncRateLimiting_BulkScenario(t *testing.T) {
	// Save original rate limiter and restore after test
	originalLimiter := vpSyncRateLimiter
	defer func() { vpSyncRateLimiter = originalLimiter }()

	// Create a test rate limiter with same config as production (1 QPS, burst of 2)
	testLimiter := rate.NewLimiter(vpUpdateRateLimit, vpUpdateBurst)
	vpSyncRateLimiter = testLimiter

	// Simulate many VPs needing sync after upgrade (would hit rate limiter in production)
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			StorageClass:            "premium",
			RequestedAutogrowPolicy: "aggressive",
		},
		BackendUUID: "backend-123",
		Pool:        "pool-1",
	}

	numVPs := 5 // Use 5 VPs to keep test fast but still demonstrate rate limiting
	vpsNeedingSync := 0
	syncTimes := make([]time.Time, 0, numVPs)

	startTime := time.Now()

	// Simulate the sync process with rate limiting
	for i := 0; i < numVPs; i++ {
		vp := &models.VolumePublication{
			NodeName:           fmt.Sprintf("node-%d", i),
			StorageClass:       "", // Empty legacy field
			BackendUUID:        "", // Empty legacy field
			Pool:               "", // Empty legacy field
			AutogrowPolicy:     "", // Empty legacy field
			AutogrowIneligible: false,
			Labels:             nil,
		}

		// This is what happens in production: wait for rate limiter token
		// with exponential backoff if rate limited
		for !vpSyncRateLimiter.Allow() {
			// In production, this uses exponential backoff (500ms -> 750ms -> 1.125s -> 2s max)
			time.Sleep(100 * time.Millisecond) // Shortened for test performance
		}

		// Record when we got the token
		syncTimes = append(syncTimes, time.Now())

		// Perform the sync
		syncNeeded := syncVolumePublicationFields(vol, vp)
		if syncNeeded {
			vpsNeedingSync++
		}

		// Verify all fields were updated
		assert.Equal(t, "premium", vp.StorageClass)
		assert.Equal(t, "backend-123", vp.BackendUUID)
		assert.Equal(t, "pool-1", vp.Pool)
		assert.Equal(t, "aggressive", vp.AutogrowPolicy)
		assert.Equal(t, fmt.Sprintf("node-%d", i), vp.Labels[config.TridentNodeNameLabel])
	}

	totalDuration := time.Since(startTime)

	// All VPs should need syncing
	assert.Equal(t, numVPs, vpsNeedingSync, "All VPs should need syncing in bulk upgrade scenario")

	// Verify rate limiting behavior:
	// With burst=2, first 2 VPs should be fast, then rate limited to 1 QPS
	// First 2 should be nearly instantaneous (burst allows them)
	if len(syncTimes) >= 2 {
		firstTwoInterval := syncTimes[1].Sub(syncTimes[0])
		assert.Less(t, firstTwoInterval, 50*time.Millisecond,
			"First 2 VPs should sync quickly due to burst capacity")
	}

	// After burst exhausted, should see throttling
	if len(syncTimes) >= 3 {
		// The 3rd VP onwards should be rate limited
		throttledInterval := syncTimes[2].Sub(syncTimes[1])
		assert.Greater(t, throttledInterval, 50*time.Millisecond,
			"VPs after burst should be rate limited")
	}

	// Total duration should reflect rate limiting (with burst of 2, remaining 3 need ~300ms min)
	// In production: burst allows 2 immediate, then ~1 per second
	// Our test uses 100ms sleeps, so 3 throttled * 100ms = ~300ms minimum
	minExpectedDuration := 250 * time.Millisecond
	assert.Greater(t, totalDuration, minExpectedDuration,
		"Total sync duration should reflect rate limiting: %v", totalDuration)

	t.Logf("Rate limiting test completed:")
	t.Logf("  - Synced %d VPs in %v", numVPs, totalDuration)
	t.Logf("  - Burst allowed first 2 VPs immediately")
	t.Logf("  - Remaining VPs were rate limited")
	for i, syncTime := range syncTimes {
		elapsed := syncTime.Sub(startTime)
		t.Logf("  - VP %d synced at %v", i, elapsed)
	}
}

// TestVPSyncRateLimiting_BackoffBehavior tests the exponential backoff behavior
// when rate limiter is exhausted (simulating production SyncVolumePublications behavior)
func TestVPSyncRateLimiting_BackoffBehavior(t *testing.T) {
	// Save original rate limiter and restore after test
	originalLimiter := vpSyncRateLimiter
	defer func() { vpSyncRateLimiter = originalLimiter }()

	// Create a very restrictive rate limiter to force backoff (0.5 QPS, burst of 1)
	testLimiter := rate.NewLimiter(0.5, 1) // 1 every 2 seconds
	vpSyncRateLimiter = testLimiter

	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			StorageClass:            "gold",
			RequestedAutogrowPolicy: "policy-1",
		},
		BackendUUID: "backend-xyz",
		Pool:        "pool-xyz",
	}

	numVPs := 3
	backoffOccurred := false
	totalBackoffTime := time.Duration(0)

	startTime := time.Now()

	for i := 0; i < numVPs; i++ {
		vp := &models.VolumePublication{
			NodeName:           fmt.Sprintf("worker-%d", i),
			StorageClass:       "silver", // Different, needs sync
			BackendUUID:        "backend-xyz",
			Pool:               "pool-xyz",
			AutogrowPolicy:     "",
			AutogrowIneligible: false,
			Labels: map[string]string{
				config.TridentNodeNameLabel: fmt.Sprintf("worker-%d", i),
			},
		}

		// Simulate production backoff behavior using the same backoff library
		// Configure exponential backoff matching production (scaled down 10x for test speed)
		rateLimiterBackoff := backoff.NewExponentialBackOff()
		rateLimiterBackoff.InitialInterval = 50 * time.Millisecond // Production: 500ms
		rateLimiterBackoff.Multiplier = 1.5
		rateLimiterBackoff.MaxInterval = 200 * time.Millisecond // Production: 2s
		rateLimiterBackoff.RandomizationFactor = 0.2
		rateLimiterBackoff.MaxElapsedTime = 0 // Retry indefinitely
		rateLimiterBackoff.Reset()

		backoffAttempts := 0
		backoffStart := time.Now()

		for !vpSyncRateLimiter.Allow() {
			backoffAttempts++
			backoffOccurred = true

			// Get next backoff duration using the same algorithm as production
			waitDuration := rateLimiterBackoff.NextBackOff()
			time.Sleep(waitDuration)
		}

		backoffElapsed := time.Since(backoffStart)
		totalBackoffTime += backoffElapsed

		// Perform sync
		syncNeeded := syncVolumePublicationFields(vol, vp)
		assert.True(t, syncNeeded, "VP %d should need sync", i)
		assert.Equal(t, "gold", vp.StorageClass, "StorageClass should be updated")

		if backoffAttempts > 0 {
			t.Logf("VP %d experienced %d backoff attempts, total backoff: %v",
				i, backoffAttempts, backoffElapsed)
		}
	}

	totalDuration := time.Since(startTime)

	// With 0.5 QPS rate limit, we should see backoff behavior
	assert.True(t, backoffOccurred, "Rate limiting should trigger backoff")
	assert.Greater(t, totalBackoffTime, 100*time.Millisecond,
		"Should have accumulated significant backoff time")

	t.Logf("Backoff test completed:")
	t.Logf("  - Synced %d VPs in %v", numVPs, totalDuration)
	t.Logf("  - Total backoff time: %v", totalBackoffTime)
	t.Logf("  - Average backoff per VP: %v", totalBackoffTime/time.Duration(numVPs))
}
