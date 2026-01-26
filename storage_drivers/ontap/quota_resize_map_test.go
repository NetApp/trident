// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewQuotaResizeMap(t *testing.T) {
	m := newQuotaResizeMap()
	assert.NotNil(t, m)
	assert.NotNil(t, m.data)
	assert.Equal(t, 0, m.Len())
}

func TestQuotaResizeMap_Set(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	assert.True(t, m.Get("flexvol1"))

	m.Set("flexvol2", false)
	assert.False(t, m.Get("flexvol2"))

	// Overwrite existing value
	m.Set("flexvol1", false)
	assert.False(t, m.Get("flexvol1"))
}

func TestQuotaResizeMap_Get(t *testing.T) {
	m := newQuotaResizeMap()

	// Get non-existent key returns false
	assert.False(t, m.Get("nonexistent"))

	m.Set("flexvol1", true)
	assert.True(t, m.Get("flexvol1"))

	m.Set("flexvol2", false)
	assert.False(t, m.Get("flexvol2"))
}

func TestQuotaResizeMap_Delete(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	m.Set("flexvol2", true)
	assert.Equal(t, 2, m.Len())

	m.Delete("flexvol1")
	assert.False(t, m.Get("flexvol1"))
	assert.Equal(t, 1, m.Len())

	// Delete non-existent key is safe
	m.Delete("nonexistent")
	assert.Equal(t, 1, m.Len())
}

func TestQuotaResizeMap_GetPending(t *testing.T) {
	m := newQuotaResizeMap()

	// Empty map returns empty slice
	pending := m.GetPending()
	assert.Empty(t, pending)

	// Add some entries
	m.Set("flexvol1", true)
	m.Set("flexvol2", true)
	m.Set("flexvol3", false)
	m.Set("flexvol4", true)

	pending = m.GetPending()
	assert.Len(t, pending, 3) // Only true values

	// Verify all expected FlexVols are present
	expectedPending := map[string]bool{
		"flexvol1": true,
		"flexvol2": true,
		"flexvol4": true,
	}
	for _, flexvol := range pending {
		assert.True(t, expectedPending[flexvol],
			"Unexpected flexvol %s in pending list", flexvol)
		delete(expectedPending, flexvol)
	}
	assert.Empty(t, expectedPending, "Missing expected FlexVols in pending list")

	// Verify false value is not included
	for _, flexvol := range pending {
		assert.NotEqual(t, "flexvol3", flexvol)
	}
}

func TestQuotaResizeMap_Len(t *testing.T) {
	m := newQuotaResizeMap()

	assert.Equal(t, 0, m.Len())

	m.Set("flexvol1", true)
	assert.Equal(t, 1, m.Len())

	m.Set("flexvol2", false)
	assert.Equal(t, 2, m.Len())

	m.Delete("flexvol1")
	assert.Equal(t, 1, m.Len())

	m.Clear()
	assert.Equal(t, 0, m.Len())
}

func TestQuotaResizeMap_Clear(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	m.Set("flexvol2", true)
	m.Set("flexvol3", false)
	assert.Equal(t, 3, m.Len())

	m.Clear()
	assert.Equal(t, 0, m.Len())
	assert.False(t, m.Get("flexvol1"))
	assert.False(t, m.Get("flexvol2"))
	assert.False(t, m.Get("flexvol3"))
	assert.Empty(t, m.GetPending())
}

// TestQuotaResizeMap_ConcurrentWrites tests concurrent writes don't cause data races
func TestQuotaResizeMap_ConcurrentWrites(t *testing.T) {
	m := newQuotaResizeMap()
	var wg sync.WaitGroup

	// Launch 100 goroutines writing different keys
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			flexvol := fmt.Sprintf("flexvol%d", id)
			m.Set(flexvol, true)
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 100, m.Len())
}

// TestQuotaResizeMap_ConcurrentReads tests concurrent reads don't cause data races
func TestQuotaResizeMap_ConcurrentReads(t *testing.T) {
	m := newQuotaResizeMap()

	// Pre-populate map
	for i := 0; i < 50; i++ {
		m.Set(fmt.Sprintf("flexvol%d", i), true)
	}

	var wg sync.WaitGroup

	// Launch 100 goroutines reading
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			flexvol := fmt.Sprintf("flexvol%d", id%50)
			m.Get(flexvol)
		}(i)
	}

	wg.Wait()
}

// TestQuotaResizeMap_ConcurrentReadWrite tests concurrent reads and writes
func TestQuotaResizeMap_ConcurrentReadWrite(t *testing.T) {
	m := newQuotaResizeMap()
	var wg sync.WaitGroup

	// Launch writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			flexvol := fmt.Sprintf("flexvol%d", id)
			m.Set(flexvol, true)
			m.Set(flexvol, false)
		}(i)
	}

	// Launch readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			flexvol := fmt.Sprintf("flexvol%d", id)
			m.Get(flexvol)
			m.GetPending()
		}(i)
	}

	// Launch deleters
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			flexvol := fmt.Sprintf("flexvol%d", id)
			m.Delete(flexvol)
		}(i)
	}

	wg.Wait()
	// No assertion needed - test passes if no data race detected
}

// TestQuotaResizeMap_GetPendingSnapshot tests that GetPending returns a snapshot
func TestQuotaResizeMap_GetPendingSnapshot(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	m.Set("flexvol2", true)

	// Get snapshot
	pending := m.GetPending()
	assert.Len(t, pending, 2)

	// Modify map after getting snapshot
	m.Set("flexvol3", true)
	m.Delete("flexvol1")

	// Original snapshot unchanged
	assert.Len(t, pending, 2)

	// New snapshot reflects changes
	newPending := m.GetPending()
	assert.Len(t, newPending, 2) // flexvol2 and flexvol3
}

// TestQuotaResizeMap_SetSameKeyMultipleTimes tests overwriting same key
func TestQuotaResizeMap_SetSameKeyMultipleTimes(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	assert.True(t, m.Get("flexvol1"))
	assert.Equal(t, 1, m.Len())

	m.Set("flexvol1", false)
	assert.False(t, m.Get("flexvol1"))
	assert.Equal(t, 1, m.Len()) // Still just one entry

	m.Set("flexvol1", true)
	assert.True(t, m.Get("flexvol1"))
	assert.Equal(t, 1, m.Len())
}

// Corner Case Tests

// TestQuotaResizeMap_EmptyStringKey tests handling of empty string as key
func TestQuotaResizeMap_EmptyStringKey(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("", true)
	assert.True(t, m.Get(""))
	assert.Equal(t, 1, m.Len())

	pending := m.GetPending()
	assert.Len(t, pending, 1)
	assert.Equal(t, "", pending[0])

	m.Delete("")
	assert.False(t, m.Get(""))
	assert.Equal(t, 0, m.Len())
}

// TestQuotaResizeMap_VeryLongKey tests handling of very long FlexVol names
func TestQuotaResizeMap_VeryLongKey(t *testing.T) {
	m := newQuotaResizeMap()

	// ONTAP FlexVol names can be up to 203 characters
	longKey := ""
	for i := 0; i < 203; i++ {
		longKey += "a"
	}

	m.Set(longKey, true)
	assert.True(t, m.Get(longKey))
	assert.Equal(t, 1, m.Len())

	pending := m.GetPending()
	assert.Len(t, pending, 1)
	assert.Equal(t, longKey, pending[0])
}

// TestQuotaResizeMap_GetPendingAllFalse tests GetPending with all false values
func TestQuotaResizeMap_GetPendingAllFalse(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", false)
	m.Set("flexvol2", false)
	m.Set("flexvol3", false)

	pending := m.GetPending()
	assert.Empty(t, pending, "GetPending should return empty slice when all values are false")
	assert.Equal(t, 3, m.Len(), "Map should still contain entries")
}

// TestQuotaResizeMap_RapidSetDeleteCycles tests thrashing scenario
func TestQuotaResizeMap_RapidSetDeleteCycles(t *testing.T) {
	m := newQuotaResizeMap()

	// Rapidly add and remove same key
	for i := 0; i < 100; i++ {
		m.Set("flexvol1", true)
		m.Delete("flexvol1")
	}

	assert.False(t, m.Get("flexvol1"))
	assert.Equal(t, 0, m.Len())
}

// TestQuotaResizeMap_SetDeleteSet tests Set after Delete
func TestQuotaResizeMap_SetDeleteSet(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	assert.True(t, m.Get("flexvol1"))
	assert.Equal(t, 1, m.Len())

	m.Delete("flexvol1")
	assert.False(t, m.Get("flexvol1"))
	assert.Equal(t, 0, m.Len())

	// Set again after delete
	m.Set("flexvol1", false)
	assert.False(t, m.Get("flexvol1"))
	assert.Equal(t, 1, m.Len())
}

// TestQuotaResizeMap_MultipleGetPendingSnapshots tests snapshot independence
func TestQuotaResizeMap_MultipleGetPendingSnapshots(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	m.Set("flexvol2", true)

	// Get multiple snapshots
	snapshot1 := m.GetPending()
	snapshot2 := m.GetPending()
	snapshot3 := m.GetPending()

	assert.Len(t, snapshot1, 2)
	assert.Len(t, snapshot2, 2)
	assert.Len(t, snapshot3, 2)

	// Modify one snapshot (shouldn't affect others or map)
	snapshot1[0] = "modified"
	assert.NotEqual(t, snapshot1[0], snapshot2[0], "Snapshots should be independent")
	assert.True(t, m.Get("flexvol1"), "Map should be unaffected by snapshot modification")
}

// TestQuotaResizeMap_ConcurrentClear tests Clear during concurrent operations
func TestQuotaResizeMap_ConcurrentClear(t *testing.T) {
	m := newQuotaResizeMap()
	var wg sync.WaitGroup

	// Pre-populate
	for i := 0; i < 50; i++ {
		m.Set(fmt.Sprintf("flexvol%d", i), true)
	}

	// Launch concurrent operations
	for i := 0; i < 25; i++ {
		// Readers
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.GetPending()
			m.Len()
		}()

		// Writers
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.Set(fmt.Sprintf("flexvol%d", id+50), true)
		}(i)
	}

	// Clear in the middle of operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.Clear()
	}()

	wg.Wait()
	// No assertion needed - test passes if no data race and no panic
}

// TestQuotaResizeMap_DeleteDuringIteration tests deleting keys that were in snapshot
func TestQuotaResizeMap_DeleteDuringIteration(t *testing.T) {
	m := newQuotaResizeMap()

	m.Set("flexvol1", true)
	m.Set("flexvol2", true)
	m.Set("flexvol3", true)

	// Get snapshot for iteration
	pending := m.GetPending()
	assert.Len(t, pending, 3)

	// Delete keys while "iterating" over snapshot
	for _, flexvol := range pending {
		// Snapshot iteration is safe even if we delete
		m.Delete(flexvol)
	}

	// All should be deleted
	assert.Equal(t, 0, m.Len())
	assert.Empty(t, m.GetPending())
}

// TestQuotaResizeMap_ConcurrentGetPending tests multiple concurrent GetPending calls
func TestQuotaResizeMap_ConcurrentGetPending(t *testing.T) {
	m := newQuotaResizeMap()

	// Pre-populate
	for i := 0; i < 100; i++ {
		m.Set(fmt.Sprintf("flexvol%d", i), true)
	}

	var wg sync.WaitGroup
	results := make([][]string, 50)

	// Launch 50 concurrent GetPending calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			results[id] = m.GetPending()
		}(i)
	}

	wg.Wait()

	// All snapshots should have same length
	for i, snapshot := range results {
		assert.Len(t, snapshot, 100, "Snapshot %d should have 100 entries", i)
	}
}

// TestQuotaResizeMap_SpecialCharactersInKey tests keys with special characters
func TestQuotaResizeMap_SpecialCharactersInKey(t *testing.T) {
	m := newQuotaResizeMap()

	specialKeys := []string{
		"flexvol-with-dashes",
		"flexvol_with_underscores",
		"flexvol.with.dots",
		"flexvol:with:colons",
		"flexvol with spaces",
		"flexvol/with/slashes",
	}

	for _, key := range specialKeys {
		m.Set(key, true)
		assert.True(t, m.Get(key), "Should handle key: %s", key)
	}

	pending := m.GetPending()
	assert.Len(t, pending, len(specialKeys))
}

// TestQuotaResizeMap_StressTest tests with large number of entries
func TestQuotaResizeMap_StressTest(t *testing.T) {
	m := newQuotaResizeMap()

	// Add 10,000 FlexVols (stress test)
	const numFlexvols = 10000
	for i := 0; i < numFlexvols; i++ {
		m.Set(fmt.Sprintf("flexvol%d", i), i%2 == 0) // Every other one is true
	}

	assert.Equal(t, numFlexvols, m.Len())

	// GetPending should only return ~5000 (the true ones)
	pending := m.GetPending()
	assert.InDelta(t, numFlexvols/2, len(pending), 1, "Should have ~half entries pending")
}

// TestQuotaResizeMap_ConcurrentSetSameKey tests multiple goroutines setting same key
func TestQuotaResizeMap_ConcurrentSetSameKey(t *testing.T) {
	m := newQuotaResizeMap()
	var wg sync.WaitGroup

	// 100 goroutines all writing to same key with alternating values
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.Set("contended-key", id%2 == 0)
		}(i)
	}

	wg.Wait()

	// Map should still have exactly 1 entry (last writer wins)
	assert.Equal(t, 1, m.Len())
	// Value could be either true or false (race between goroutines)
	_ = m.Get("contended-key") // Just verify no panic
}
