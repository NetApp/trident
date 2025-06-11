package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSnapshot_SmartCopy test that a deep copy is deeply equal but does not point to the same memory
func TestSnapshot_SmartCopy(t *testing.T) {
	// Create a snapshot
	snapshot := NewSnapshot(&SnapshotConfig{
		Version:             "1",
		Name:                "test-snapshot",
		InternalName:        "test-snapshot-internal",
		VolumeName:          "test-volume",
		VolumeInternalName:  "test-volume-internal",
		LUKSPassphraseNames: []string{"passphrase1", "passphrase2"},
	}, "2023-10-01T00:00:00Z", 1024, SnapshotStateOnline)

	// Create a deep copy of the snapshot
	copiedSnapshot := snapshot.SmartCopy().(*Snapshot)

	// Check that the copied snapshot is deeply equal to the original
	assert.Equal(t, snapshot, copiedSnapshot)

	// Check that the copied snapshot does not point to the same memory
	assert.False(t, snapshot == copiedSnapshot)
}
