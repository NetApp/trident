package storage

import (
	"regexp"
	"sync"
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

func TestGenerateUniqueSnapshotName_Format(t *testing.T) {
	name := GenerateUniqueSnapshotName()
	// Should match: 16-char timestamp (e.g. 20260407T064737Z) + 6 lowercase alphanumeric chars
	pattern := regexp.MustCompile(`^\d{8}T\d{6}Z[a-z0-9]{6}$`)
	assert.Regexp(t, pattern, name)
}

func TestGenerateUniqueSnapshotName_Length(t *testing.T) {
	name := GenerateUniqueSnapshotName()
	// 16 (timestamp) + 6 (random suffix) = 22
	assert.Equal(t, 22, len(name))
}

func TestGenerateUniqueSnapshotName_Uniqueness(t *testing.T) {
	name1 := GenerateUniqueSnapshotName()
	name2 := GenerateUniqueSnapshotName()
	assert.NotEqual(t, name1, name2)
}

func TestGenerateUniqueSnapshotName_ConcurrentFormat(t *testing.T) {
	const goroutines = 10
	names := make([]string, goroutines)
	var wg sync.WaitGroup
	pattern := regexp.MustCompile(`^\d{8}T\d{6}Z[a-z0-9]{6}$`)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			names[idx] = GenerateUniqueSnapshotName()
		}(i)
	}
	wg.Wait()

	for _, name := range names {
		assert.Regexp(t, pattern, name)
		assert.Equal(t, 22, len(name))
	}
}
