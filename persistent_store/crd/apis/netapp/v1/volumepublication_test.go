// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/models"
)

// TestTridentVolumePublication_Apply tests that all fields are correctly copied to CRD
func TestTridentVolumePublication_Apply(t *testing.T) {
	persistent := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       true,
		AccessMode:     5,
		AutogrowPolicy: "aggressive",
		StorageClass:   "premium",
		BackendUUID:    "backend-uuid-123",
		Pool:           "pool-xyz",
	}

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
	}

	err := tvp.Apply(persistent)
	assert.NoError(t, err, "Apply should not return error")

	// Verify all fields are set correctly
	assert.Equal(t, persistent.VolumeName, tvp.VolumeID)
	assert.Equal(t, persistent.NodeName, tvp.NodeID)
	assert.Equal(t, persistent.ReadOnly, tvp.ReadOnly)
	assert.Equal(t, persistent.AccessMode, tvp.AccessMode)
	assert.Equal(t, persistent.AutogrowPolicy, tvp.AutogrowPolicy)
	assert.Equal(t, persistent.StorageClass, tvp.StorageClass)
	assert.Equal(t, persistent.BackendUUID, tvp.BackendUUID)
	assert.Equal(t, persistent.Pool, tvp.Pool)

	// Verify node label is set
	assert.NotNil(t, tvp.ObjectMeta.Labels)
	assert.Equal(t, persistent.NodeName, tvp.ObjectMeta.Labels[config.TridentNodeNameLabel])
}

// TestTridentVolumePublication_Apply_EmptyFields tests Apply with empty new fields
func TestTridentVolumePublication_Apply_EmptyFields(t *testing.T) {
	persistent := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "",
		StorageClass:   "",
		BackendUUID:    "",
		Pool:           "",
	}

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
	}

	err := tvp.Apply(persistent)
	assert.NoError(t, err, "Apply should not return error")

	// Verify empty fields are handled correctly
	assert.Equal(t, "", tvp.AutogrowPolicy)
	assert.Equal(t, "", tvp.StorageClass)
	assert.Equal(t, "", tvp.BackendUUID)
	assert.Equal(t, "", tvp.Pool)
}

// TestTridentVolumePublication_Apply_UpdatesExistingFields tests that Apply updates existing fields
func TestTridentVolumePublication_Apply_UpdatesExistingFields(t *testing.T) {
	persistent := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "new-volume",
		NodeName:       "new-node",
		ReadOnly:       true,
		AccessMode:     3,
		AutogrowPolicy: "aggressive",
		StorageClass:   "gold",
		BackendUUID:    "new-backend",
		Pool:           "new-pool",
	}

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
		VolumeID:       "old-volume",
		NodeID:         "old-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "conservative",
		StorageClass:   "silver",
		BackendUUID:    "old-backend",
		Pool:           "old-pool",
	}

	err := tvp.Apply(persistent)
	assert.NoError(t, err, "Apply should not return error")

	// Verify all fields are updated
	assert.Equal(t, "new-volume", tvp.VolumeID)
	assert.Equal(t, "new-node", tvp.NodeID)
	assert.Equal(t, true, tvp.ReadOnly)
	assert.Equal(t, int32(3), tvp.AccessMode)
	assert.Equal(t, "aggressive", tvp.AutogrowPolicy)
	assert.Equal(t, "gold", tvp.StorageClass)
	assert.Equal(t, "new-backend", tvp.BackendUUID)
	assert.Equal(t, "new-pool", tvp.Pool)
}

// TestTridentVolumePublication_Persistent tests that all fields are correctly copied from CRD
func TestTridentVolumePublication_Persistent(t *testing.T) {
	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
		VolumeID:       "test-volume",
		NodeID:         "test-node",
		ReadOnly:       true,
		AccessMode:     5,
		AutogrowPolicy: "aggressive",
		StorageClass:   "premium",
		BackendUUID:    "backend-uuid-123",
		Pool:           "pool-xyz",
	}

	persistent, err := tvp.Persistent()
	assert.NoError(t, err, "Persistent should not return error")
	assert.NotNil(t, persistent)

	// Verify all fields are set correctly
	assert.Equal(t, tvp.Name, persistent.Name)
	assert.Equal(t, tvp.VolumeID, persistent.VolumeName)
	assert.Equal(t, tvp.NodeID, persistent.NodeName)
	assert.Equal(t, tvp.ReadOnly, persistent.ReadOnly)
	assert.Equal(t, tvp.AccessMode, persistent.AccessMode)
	assert.Equal(t, tvp.AutogrowPolicy, persistent.AutogrowPolicy)
	assert.Equal(t, tvp.StorageClass, persistent.StorageClass)
	assert.Equal(t, tvp.BackendUUID, persistent.BackendUUID)
	assert.Equal(t, tvp.Pool, persistent.Pool)
}

// TestTridentVolumePublication_Persistent_EmptyFields tests Persistent with empty new fields
func TestTridentVolumePublication_Persistent_EmptyFields(t *testing.T) {
	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
		VolumeID:       "test-volume",
		NodeID:         "test-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "",
		StorageClass:   "",
		BackendUUID:    "",
		Pool:           "",
	}

	persistent, err := tvp.Persistent()
	assert.NoError(t, err, "Persistent should not return error")

	// Verify empty fields are preserved
	assert.Equal(t, "", persistent.AutogrowPolicy)
	assert.Equal(t, "", persistent.StorageClass)
	assert.Equal(t, "", persistent.BackendUUID)
	assert.Equal(t, "", persistent.Pool)
}

// TestTridentVolumePublication_RoundTrip tests that Apply and Persistent are symmetric
func TestTridentVolumePublication_RoundTrip(t *testing.T) {
	original := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       true,
		AccessMode:     7,
		AutogrowPolicy: "moderate",
		StorageClass:   "platinum",
		BackendUUID:    "backend-456",
		Pool:           "pool-789",
	}

	// Convert to CRD
	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: original.Name,
		},
	}
	err := tvp.Apply(original)
	assert.NoError(t, err)

	// Convert back to persistent
	result, err := tvp.Persistent()
	assert.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.Name, result.Name)
	assert.Equal(t, original.VolumeName, result.VolumeName)
	assert.Equal(t, original.NodeName, result.NodeName)
	assert.Equal(t, original.ReadOnly, result.ReadOnly)
	assert.Equal(t, original.AccessMode, result.AccessMode)
	assert.Equal(t, original.AutogrowPolicy, result.AutogrowPolicy)
	assert.Equal(t, original.StorageClass, result.StorageClass)
	assert.Equal(t, original.BackendUUID, result.BackendUUID)
	assert.Equal(t, original.Pool, result.Pool)
}

// TestTridentVolumePublication_Apply_WithSpecialCharacters tests new fields with special characters
func TestTridentVolumePublication_Apply_WithSpecialCharacters(t *testing.T) {
	persistent := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "policy-with-dashes",
		StorageClass:   "storage-class-with-dashes-123",
		BackendUUID:    "backend-uuid-with-dashes-abc-def",
		Pool:           "pool_with_underscores",
	}

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
	}

	err := tvp.Apply(persistent)
	assert.NoError(t, err)

	// Verify special characters are preserved
	assert.Equal(t, "storage-class-with-dashes-123", tvp.StorageClass)
	assert.Equal(t, "backend-uuid-with-dashes-abc-def", tvp.BackendUUID)
	assert.Equal(t, "pool_with_underscores", tvp.Pool)
}

// TestTridentVolumePublication_Apply_PreservesExistingLabels tests that Apply preserves existing labels
func TestTridentVolumePublication_Apply_PreservesExistingLabels(t *testing.T) {
	persistent := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "new-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "aggressive",
		StorageClass:   "gold",
		BackendUUID:    "backend-123",
		Pool:           "pool-456",
	}

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
			Labels: map[string]string{
				"existing-label": "existing-value",
				"another-label":  "another-value",
			},
		},
	}

	err := tvp.Apply(persistent)
	assert.NoError(t, err)

	// Verify existing labels are preserved
	assert.Equal(t, "existing-value", tvp.ObjectMeta.Labels["existing-label"])
	assert.Equal(t, "another-value", tvp.ObjectMeta.Labels["another-label"])

	// Verify node label is set correctly
	assert.Equal(t, "new-node", tvp.ObjectMeta.Labels[config.TridentNodeNameLabel])
}

// TestTridentVolumePublication_Persistent_WithLongFieldValues tests handling of long field values
func TestTridentVolumePublication_Persistent_WithLongFieldValues(t *testing.T) {
	// Create very long field values
	longStorageClass := "very-long-storage-class-name-that-exceeds-normal-expectations-for-testing-purposes-123456789"
	longBackendUUID := "very-long-backend-uuid-value-with-many-characters-for-comprehensive-testing-abc-def-ghi-jkl"
	longPool := "very-long-pool-name-that-tests-maximum-length-handling-in-the-system-pool-xyz-123"

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
		VolumeID:       "test-volume",
		NodeID:         "test-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "aggressive",
		StorageClass:   longStorageClass,
		BackendUUID:    longBackendUUID,
		Pool:           longPool,
	}

	persistent, err := tvp.Persistent()
	assert.NoError(t, err)
	assert.NotNil(t, persistent)

	// Verify long values are preserved exactly
	assert.Equal(t, longStorageClass, persistent.StorageClass)
	assert.Equal(t, longBackendUUID, persistent.BackendUUID)
	assert.Equal(t, longPool, persistent.Pool)
}

// TestTridentVolumePublication_Apply_MultipleApplyCalls tests idempotency
func TestTridentVolumePublication_Apply_MultipleApplyCalls(t *testing.T) {
	persistent1 := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       true,
		AccessMode:     3,
		AutogrowPolicy: "aggressive",
		StorageClass:   "first-class",
		BackendUUID:    "first-backend",
		Pool:           "first-pool",
	}

	persistent2 := &models.VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node-updated",
		ReadOnly:       false,
		AccessMode:     5,
		AutogrowPolicy: "moderate",
		StorageClass:   "second-class",
		BackendUUID:    "second-backend",
		Pool:           "second-pool",
	}

	tvp := &TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pub",
		},
	}

	// Apply first time
	err := tvp.Apply(persistent1)
	assert.NoError(t, err)
	assert.Equal(t, "first-class", tvp.StorageClass)
	assert.Equal(t, "first-backend", tvp.BackendUUID)
	assert.Equal(t, "first-pool", tvp.Pool)

	// Apply second time with different values (should update)
	err = tvp.Apply(persistent2)
	assert.NoError(t, err)
	assert.Equal(t, "second-class", tvp.StorageClass)
	assert.Equal(t, "second-backend", tvp.BackendUUID)
	assert.Equal(t, "second-pool", tvp.Pool)
	assert.Equal(t, "test-node-updated", tvp.ObjectMeta.Labels[config.TridentNodeNameLabel])
}
