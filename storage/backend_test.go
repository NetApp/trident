// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/errors"
)

func TestBackendState(t *testing.T) {
	tests := map[string]struct {
		input     BackendState
		output    string
		predicate func(BackendState) bool
	}{
		"Unknown state (bad)": {
			input:  "",
			output: "unknown",
			predicate: func(input BackendState) bool {
				return input.IsUnknown()
			},
		},
		"Unknown state": {
			input:  Unknown,
			output: "unknown",
			predicate: func(input BackendState) bool {
				return input.IsUnknown()
			},
		},
		"Online state": {
			input:  Online,
			output: "online",
			predicate: func(input BackendState) bool {
				return input.IsOnline()
			},
		},
		"Offline state": {
			input:  Offline,
			output: "offline",
			predicate: func(input BackendState) bool {
				return input.IsOffline()
			},
		},
		"Deleting state": {
			input:  Deleting,
			output: "deleting",
			predicate: func(input BackendState) bool {
				return input.IsDeleting()
			},
		},
		"Failed state": {
			input:  Failed,
			output: "failed",
			predicate: func(input BackendState) bool {
				return input.IsFailed()
			},
		},
	}
	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				assert.Equal(t, test.input.String(), test.output, "Strings not equal")
				assert.True(t, test.predicate(test.input), "Predicate failed")
			},
		)
	}
}

func TestDeleteSnapshot_BackendOffline(t *testing.T) {
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:      "",
		Name:         volumeName,
		InternalName: volumeInternalName,
	}
	snapName := "snapshot"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
	}

	backend := &StorageBackend{
		state: Offline,
	}

	// Both volume and snapshot not managed
	err := backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")
}

func TestDeleteSnapshot_NotManaged(t *testing.T) {
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:             "",
		Name:                volumeName,
		InternalName:        volumeInternalName,
		ImportOriginalName:  "import-" + volumeName,
		ImportBackendUUID:   "import-" + backendUUID,
		ImportNotManaged:    true,
		LUKSPassphraseNames: nil,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}

	backend := &StorageBackend{
		state: Online,
	}

	// Both volume and snapshot not managed
	err := backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")

	// Volume not managed
	volumeConfig.ImportNotManaged = true
	snapConfig.ImportNotManaged = false
	err = backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")

	// Snapshot not managed
	volumeConfig.ImportNotManaged = false
	snapConfig.ImportNotManaged = true
	err = backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")
}

func TestCloneVolume_FeatureDisabled(t *testing.T) {
	defer func() { tridentconfig.DisableExtraFeatures = false }()
	tridentconfig.DisableExtraFeatures = true

	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:      "",
		Name:         volumeName,
		InternalName: volumeInternalName,
	}
	volumeConfigDest := &VolumeConfig{
		Version:       "",
		Name:          "pvc-deadbeef-8240-4fd8-97bc-868bf064ecd4",
		InternalName:  "trident_pvc_deadbeef_8240_4fd8_97bc_868bf064ecd4",
		ReadOnlyClone: true,
	}

	backend := &StorageBackend{
		state: Offline,
	}
	pool := NewStoragePool(nil, "test-pool1")

	// Both volume and snapshot not managed
	_, err := backend.CloneVolume(context.Background(), volumeConfig, volumeConfigDest, pool, false)

	assert.Error(t, err, "expected err")
	assert.True(t, errors.IsUnsupportedError(err))
}

func TestCloneVolume_BackendOffline(t *testing.T) {
	defer func() { tridentconfig.DisableExtraFeatures = false }()
	tridentconfig.DisableExtraFeatures = false

	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:       "",
		Name:          volumeName,
		InternalName:  volumeInternalName,
		ReadOnlyClone: true,
	}
	volumeConfigDest := &VolumeConfig{
		Version:       "",
		Name:          "pvc-deadbeef-8240-4fd8-97bc-868bf064ecd4",
		InternalName:  "trident_pvc_deadbeef_8240_4fd8_97bc_868bf064ecd4",
		ReadOnlyClone: false,
	}

	backend := &StorageBackend{
		state: Offline,
		name:  "test-backend",
	}
	pool := NewStoragePool(nil, "test-pool1")

	// Both volume and snapshot not managed
	_, err := backend.CloneVolume(context.Background(), volumeConfig, volumeConfigDest, pool, false)

	assert.Errorf(t, err, "expected err")
	assert.Equal(t, err.Error(), "backend test-backend is not Online")
}
