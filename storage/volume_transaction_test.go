// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVolumeTransaction_Name_AddSnapshot(t *testing.T) {
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               "test-snapshot",
		VolumeName:         "test-volume",
		InternalName:       "test-snapshot-internal",
		VolumeInternalName: "test-volume-internal",
	}

	transaction := &VolumeTransaction{
		Op:             AddSnapshot,
		SnapshotConfig: snapConfig,
	}

	expectedName := snapConfig.ID()
	actualName := transaction.Name()

	assert.Equal(t, expectedName, actualName, "Name() should return snapshot ID for AddSnapshot operation")
}

func TestVolumeTransaction_Name_DeleteSnapshot(t *testing.T) {
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               "test-snapshot",
		VolumeName:         "test-volume",
		InternalName:       "test-snapshot-internal",
		VolumeInternalName: "test-volume-internal",
	}

	transaction := &VolumeTransaction{
		Op:             DeleteSnapshot,
		SnapshotConfig: snapConfig,
	}

	expectedName := snapConfig.ID()
	actualName := transaction.Name()

	assert.Equal(t, expectedName, actualName, "Name() should return snapshot ID for DeleteSnapshot operation")
}

func TestVolumeTransaction_Name_AddGroupSnapshot(t *testing.T) {
	groupSnapConfig := &GroupSnapshotConfig{
		Version:      "1",
		Name:         "test-group-snapshot",
		InternalName: "test-group-snapshot-internal",
		VolumeNames:  []string{"volume1", "volume2"},
	}

	transaction := &VolumeTransaction{
		Op:                  AddGroupSnapshot,
		GroupSnapshotConfig: groupSnapConfig,
	}

	expectedName := groupSnapConfig.ID()
	actualName := transaction.Name()

	assert.Equal(t, expectedName, actualName, "Name() should return group snapshot ID for AddGroupSnapshot operation")
}

func TestVolumeTransaction_Name_DeleteGroupSnapshot(t *testing.T) {
	groupSnapConfig := &GroupSnapshotConfig{
		Version:      "1",
		Name:         "test-group-snapshot",
		InternalName: "test-group-snapshot-internal",
		VolumeNames:  []string{"volume1", "volume2"},
	}

	transaction := &VolumeTransaction{
		Op:                  DeleteGroupSnapshot,
		GroupSnapshotConfig: groupSnapConfig,
	}

	expectedName := groupSnapConfig.ID()
	actualName := transaction.Name()

	assert.Equal(t, expectedName, actualName, "Name() should return group snapshot ID for DeleteGroupSnapshot operation")
}

func TestVolumeTransaction_Name_VolumeCreating(t *testing.T) {
	volumeCreatingConfig := &VolumeCreatingConfig{
		VolumeConfig: VolumeConfig{
			Name: "test-volume-creating",
		},
	}

	transaction := &VolumeTransaction{
		Op:                   VolumeCreating,
		VolumeCreatingConfig: volumeCreatingConfig,
	}

	expectedName := volumeCreatingConfig.Name
	actualName := transaction.Name()

	assert.Equal(t, expectedName, actualName, "Name() should return VolumeCreatingConfig.Name for VolumeCreating operation")
}

func TestVolumeTransaction_Name_DefaultOperations(t *testing.T) {
	volumeConfig := &VolumeConfig{
		Version:      "1",
		Name:         "test-volume",
		InternalName: "test-volume-internal",
		Size:         "1Gi",
	}

	// Test all default operations (AddVolume, DeleteVolume, ImportVolume, ResizeVolume)
	operations := []VolumeOperation{
		AddVolume,
		DeleteVolume,
		ImportVolume,
		ResizeVolume,
	}

	for _, op := range operations {
		t.Run(string(op), func(t *testing.T) {
			transaction := &VolumeTransaction{
				Op:     op,
				Config: volumeConfig,
			}

			expectedName := volumeConfig.Name
			actualName := transaction.Name()

			assert.Equal(t, expectedName, actualName, "Name() should return Config.Name for %s operation", op)
		})
	}
}
