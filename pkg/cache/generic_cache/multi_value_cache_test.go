// Copyright 2025 NetApp, Inc. All Rights Reserved.

package generic_cache

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestMultiValueCache_SetIdempotent(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Add value first time
	err := cache.Set("sc1", "vol1")
	assert.NoError(t, err)
	values := cache.Get("sc1")
	assert.Equal(t, []string{"vol1"}, values)

	// Add same value again - should return error
	err = cache.Set("sc1", "vol1")
	assert.Error(t, err)
	assert.True(t, errors.IsAlreadyExistsError(err), "Should return AlreadyExistsError")
	values = cache.Get("sc1")
	assert.Equal(t, []string{"vol1"}, values, "Cache should remain unchanged")

	// Add third time
	err = cache.Set("sc1", "vol1")
	assert.Error(t, err)
	assert.True(t, errors.IsAlreadyExistsError(err), "Should return AlreadyExistsError")
	values = cache.Get("sc1")
	assert.Equal(t, []string{"vol1"}, values, "Cache should remain unchanged")
}

func TestMultiValueCache_SetMultipleValues(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Add multiple different values
	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")
	_ = cache.Set("sc1", "vol3")

	values := cache.Get("sc1")
	assert.Len(t, values, 3)
	assert.Contains(t, values, "vol1")
	assert.Contains(t, values, "vol2")
	assert.Contains(t, values, "vol3")

	// Add duplicates - should not increase count
	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")

	values = cache.Get("sc1")
	assert.Len(t, values, 3, "Adding duplicates should not increase count")
}

func TestMultiValueCache_DeleteIdempotent(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")

	// Delete value
	err := cache.Delete("sc1", "vol1")
	assert.NoError(t, err)
	values := cache.Get("sc1")
	assert.Equal(t, []string{"vol2"}, values)

	// Delete same value again - should return error for non-existent key-value pair
	err = cache.Delete("sc1", "vol1")
	assert.Error(t, err)
	assert.True(t, errors.IsValueError(err), "Should return ValueError")
	values = cache.Get("sc1")
	assert.Equal(t, []string{"vol2"}, values, "Cache should remain unchanged")

	// Delete third time
	err = cache.Delete("sc1", "vol1")
	assert.Error(t, err)
	assert.True(t, errors.IsValueError(err), "Should return ValueError")
	values = cache.Get("sc1")
	assert.Equal(t, []string{"vol2"}, values, "Cache should remain unchanged")
}

func TestMultiValueCache_DeleteNonExistentKey(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Delete from non-existent key - should return error
	err := cache.Delete("nonexistent", "vol1")
	assert.Error(t, err)
	assert.True(t, errors.IsKeyError(err), "Should return KeyError")

	values := cache.Get("nonexistent")
	assert.Empty(t, values)
}

func TestMultiValueCache_FindKeyForValue(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")
	_ = cache.Set("sc2", "vol3")

	// Find key for value - O(1) operation
	keys := cache.FindKeyForValue("vol1")
	assert.Equal(t, []string{"sc1"}, keys)

	keys = cache.FindKeyForValue("vol3")
	assert.Equal(t, []string{"sc2"}, keys)

	// Non-existent value
	keys = cache.FindKeyForValue("nonexistent")
	assert.Empty(t, keys)
}

func TestMultiValueCache_ReverseIndexUpdated(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	_ = cache.Set("sc1", "vol1")
	keys := cache.FindKeyForValue("vol1")
	assert.Equal(t, []string{"sc1"}, keys)

	// Delete value - reverse index should be updated
	_ = cache.Delete("sc1", "vol1")
	keys = cache.FindKeyForValue("vol1")
	assert.Empty(t, keys, "Reverse index should be updated after delete")

	// Add again - reverse index should be restored
	_ = cache.Set("sc1", "vol1")
	keys = cache.FindKeyForValue("vol1")
	assert.Equal(t, []string{"sc1"}, keys, "Reverse index should be restored after re-adding")
}

func TestMultiValueCache_GetReturnsDefensiveCopy(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")

	// Get values
	values1 := cache.Get("sc1")
	values2 := cache.Get("sc1")

	// Modify returned slice
	values1[0] = "modified"

	// Original cache should not be affected
	values3 := cache.Get("sc1")
	assert.NotContains(t, values3, "modified", "Cache should return defensive copy")

	// Second get should also not be affected
	assert.NotContains(t, values2, "modified", "Modifying returned slice should not affect other gets")
}

func TestMultiValueCache_PerformanceCharacteristics(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Add 1000 volumes to one SC - O(1) per operation
	for i := 0; i < 1000; i++ {
		_ = cache.Set("sc1", string(rune(i)))
	}

	// Get should be O(1) - instant return of cached array
	values := cache.Get("sc1")
	assert.Len(t, values, 1000)

	// FindKeyForValue should be O(1) - direct map lookup
	keys := cache.FindKeyForValue(string(rune(500)))
	assert.Equal(t, []string{"sc1"}, keys)

	// Delete should be O(1) - swap-and-pop technique
	err := cache.Delete("sc1", string(rune(500)))
	assert.NoError(t, err)
	values = cache.Get("sc1")
	assert.Len(t, values, 999, "Delete should be O(1) using swap-and-pop")

	// Adding duplicate 1000 times should return error (no array growth)
	newValue := string(rune(2000)) // Use a value that doesn't exist yet
	for i := 0; i < 1000; i++ {
		err := cache.Set("sc1", newValue)
		if i == 0 {
			assert.NoError(t, err, "First add should succeed")
		} else {
			assert.Error(t, err, "Duplicate adds should return error")
			assert.True(t, errors.IsAlreadyExistsError(err), "Should be AlreadyExistsError")
		}
	}
	values = cache.Get("sc1")
	assert.Len(t, values, 1000, "Array should have 1000 elements (999 after delete + 1 new)")
}

func TestMultiValueCache_StorageClassVolumeScenario(t *testing.T) {
	// Real-world scenario: StorageClass -> Volumes mapping
	cache := NewMultiValueCache[string, string]()

	// Initial volume creation
	_ = cache.Set("gold-sc", "pvc-1.node-a")
	_ = cache.Set("gold-sc", "pvc-2.node-a")
	_ = cache.Set("silver-sc", "pvc-3.node-b")

	// Duplicate TVP event (volume already tracked) - should return error
	err := cache.Set("gold-sc", "pvc-1.node-a")
	assert.Error(t, err)
	assert.True(t, errors.IsAlreadyExistsError(err), "Duplicate TVP event should return error")
	volumes := cache.Get("gold-sc")
	assert.Len(t, volumes, 2, "Cache should remain unchanged")

	// Find which SC a volume belongs to
	scNames := cache.FindKeyForValue("pvc-1.node-a")
	assert.Equal(t, []string{"gold-sc"}, scNames)

	// Volume deletion
	_ = cache.Delete("gold-sc", "pvc-1.node-a")
	volumes = cache.Get("gold-sc")
	assert.Len(t, volumes, 1)
	assert.Equal(t, []string{"pvc-2.node-a"}, volumes)

	// Duplicate deletion (should return error now)
	err = cache.Delete("gold-sc", "pvc-1.node-a")
	assert.Error(t, err)
	assert.True(t, errors.IsValueError(err), "Duplicate deletion should return error")
	volumes = cache.Get("gold-sc")
	assert.Len(t, volumes, 1, "Cache should remain unchanged")

	// StorageClass annotation change - get all volumes
	volumes = cache.Get("gold-sc")
	assert.Len(t, volumes, 1)
	volumes = cache.Get("silver-sc")
	assert.Len(t, volumes, 1)
}

func TestMultiValueCache_EmptyKeyRejected(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	err := cache.Set("", "vol1")
	assert.Error(t, err, "Empty key should be rejected")
}

func TestMultiValueCache_DeleteKeyRemovesReverseIndex(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")

	// Verify reverse index
	keys := cache.FindKeyForValue("vol1")
	assert.Equal(t, []string{"sc1"}, keys)

	// Delete entire key
	_ = cache.DeleteKey("sc1")

	// Reverse index should be cleaned up
	keys = cache.FindKeyForValue("vol1")
	assert.Empty(t, keys, "Reverse index should be cleaned up after DeleteKey")

	keys = cache.FindKeyForValue("vol2")
	assert.Empty(t, keys, "Reverse index should be cleaned up for all values")

	// Key should not exist
	assert.False(t, cache.Has("sc1"))
}

func TestMultiValueCache_DeleteKey_NonExistent(t *testing.T) {
	// Test error behavior - deleting non-existent key should return error
	cache := NewMultiValueCache[string, string]()

	err := cache.DeleteKey("nonexistent")
	assert.Error(t, err)
	assert.True(t, errors.IsKeyError(err), "Should return KeyError for non-existent key")

	// Add some data
	_ = cache.Set("key1", "val1")

	// Delete different non-existent key
	err = cache.DeleteKey("key2")
	assert.Error(t, err)
	assert.True(t, errors.IsKeyError(err), "Should return KeyError for non-existent key")

	// Original data should remain
	assert.True(t, cache.Has("key1"))
	assert.Equal(t, []string{"val1"}, cache.Get("key1"))
}

func TestMultiValueCache_DeleteKey_SharedValues(t *testing.T) {
	// Test DeleteKey when values are shared with other keys
	// This covers the case where reverse cleanup happens but value maps aren't fully deleted
	cache := NewMultiValueCache[string, string]()

	// key1 and key2 both point to val1
	_ = cache.Set("key1", "val1")
	_ = cache.Set("key1", "val2")
	_ = cache.Set("key2", "val1")
	_ = cache.Set("key2", "val3")

	// Verify shared value
	keys := cache.FindKeyForValue("val1")
	assert.Len(t, keys, 2, "val1 should be associated with 2 keys")

	// Delete key1
	err := cache.DeleteKey("key1")
	assert.NoError(t, err)

	// key1 should be gone
	assert.False(t, cache.Has("key1"))
	assert.Empty(t, cache.Get("key1"))

	// key2 should still exist
	assert.True(t, cache.Has("key2"))
	vals := cache.Get("key2")
	assert.Len(t, vals, 2)
	assert.Contains(t, vals, "val1")
	assert.Contains(t, vals, "val3")

	// val1 reverse index should only have key2 now
	keys = cache.FindKeyForValue("val1")
	assert.Equal(t, []string{"key2"}, keys, "val1 should only be associated with key2 now")

	// val2 should have no keys (was only in key1)
	keys = cache.FindKeyForValue("val2")
	assert.Empty(t, keys, "val2 should have no associated keys after key1 deletion")

	// val3 should still have key2
	keys = cache.FindKeyForValue("val3")
	assert.Equal(t, []string{"key2"}, keys)
}

func TestMultiValueCache_DeleteKey_SwapAndPop(t *testing.T) {
	// Test swap-and-pop logic in reverse array cleanup
	// This covers the case where reverseIndex != reverseLastIndex
	cache := NewMultiValueCache[string, string]()

	// Create a scenario where one value is associated with multiple keys
	// and the key we delete is NOT the last one in the reverse array
	_ = cache.Set("key1", "shared")
	_ = cache.Set("key2", "shared")
	_ = cache.Set("key3", "shared")

	// Verify all keys are in reverse index
	keys := cache.FindKeyForValue("shared")
	assert.Len(t, keys, 3)

	// Delete key2 (should be in middle of reverse array, triggering swap-and-pop)
	err := cache.DeleteKey("key2")
	assert.NoError(t, err)

	// key2 should be gone
	assert.False(t, cache.Has("key2"))

	// key1 and key3 should remain
	assert.True(t, cache.Has("key1"))
	assert.True(t, cache.Has("key3"))

	// Reverse index should have 2 keys
	keys = cache.FindKeyForValue("shared")
	assert.Len(t, keys, 2, "shared should be associated with 2 keys after deletion")
	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key3")
	assert.NotContains(t, keys, "key2")
}

func TestMultiValueCache_DeleteKey_MultipleValues(t *testing.T) {
	// Test DeleteKey with multiple values, some exclusive to the key
	cache := NewMultiValueCache[string, string]()

	_ = cache.Set("key1", "exclusive1")
	_ = cache.Set("key1", "exclusive2")
	_ = cache.Set("key1", "shared")
	_ = cache.Set("key2", "shared")

	err := cache.DeleteKey("key1")
	assert.NoError(t, err)

	// key1 should be gone
	assert.False(t, cache.Has("key1"))
	assert.Empty(t, cache.Get("key1"))

	// Exclusive values should have no reverse mappings
	assert.Empty(t, cache.FindKeyForValue("exclusive1"))
	assert.Empty(t, cache.FindKeyForValue("exclusive2"))

	// Shared value should still map to key2
	keys := cache.FindKeyForValue("shared")
	assert.Equal(t, []string{"key2"}, keys)

	// key2 should be unaffected
	assert.True(t, cache.Has("key2"))
	assert.Equal(t, []string{"shared"}, cache.Get("key2"))
}

func TestMultiValueCache_SwapAndPopBehavior(t *testing.T) {
	// Test that demonstrates O(1) delete using swap-and-pop
	cache := NewMultiValueCache[string, string]()

	// Add values in order
	_ = cache.Set("sc1", "vol-A")
	_ = cache.Set("sc1", "vol-B")
	_ = cache.Set("sc1", "vol-C")
	_ = cache.Set("sc1", "vol-D")
	_ = cache.Set("sc1", "vol-E")

	// Delete middle element (vol-C at index 2)
	// Should swap with last element (vol-E) and truncate
	_ = cache.Delete("sc1", "vol-C")

	values := cache.Get("sc1")
	assert.Len(t, values, 4)
	// Order may have changed due to swap-and-pop
	assert.Contains(t, values, "vol-A")
	assert.Contains(t, values, "vol-B")
	assert.Contains(t, values, "vol-D")
	assert.Contains(t, values, "vol-E")
	assert.NotContains(t, values, "vol-C")

	// Verify reverse index still works
	keys := cache.FindKeyForValue("vol-E")
	assert.Equal(t, []string{"sc1"}, keys, "Reverse index should be updated after swap")

	keys = cache.FindKeyForValue("vol-C")
	assert.Empty(t, keys, "Deleted value should not be in reverse index")

	// Delete first element
	_ = cache.Delete("sc1", "vol-A")
	values = cache.Get("sc1")
	assert.Len(t, values, 3)

	// Delete last element
	lastValue := values[len(values)-1]
	_ = cache.Delete("sc1", lastValue)
	values = cache.Get("sc1")
	assert.Len(t, values, 2)

	// All operations should be O(1)
	assert.Contains(t, values, "vol-B")
	assert.Contains(t, values, "vol-D")
}

func TestMultiValueCache_ManyToManyRelationship(t *testing.T) {
	// Test many-to-many: One value can belong to multiple keys
	cache := NewMultiValueCache[string, string]()

	// vol1 belongs to both sc1 and sc2 (shared volume)
	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc2", "vol1")
	_ = cache.Set("sc1", "vol2")
	_ = cache.Set("sc2", "vol3")

	// Forward lookup
	values1 := cache.Get("sc1")
	assert.Len(t, values1, 2)
	assert.Contains(t, values1, "vol1")
	assert.Contains(t, values1, "vol2")

	values2 := cache.Get("sc2")
	assert.Len(t, values2, 2)
	assert.Contains(t, values2, "vol1")
	assert.Contains(t, values2, "vol3")

	// Reverse lookup: vol1 belongs to both sc1 and sc2
	keys := cache.FindKeyForValue("vol1")
	assert.Len(t, keys, 2, "vol1 should belong to 2 keys")
	assert.Contains(t, keys, "sc1")
	assert.Contains(t, keys, "sc2")

	// Reverse lookup: vol2 belongs only to sc1
	keys = cache.FindKeyForValue("vol2")
	assert.Len(t, keys, 1)
	assert.Contains(t, keys, "sc1")

	// Delete vol1 from sc1 (but still in sc2)
	_ = cache.Delete("sc1", "vol1")

	// Forward: sc1 should not have vol1 anymore
	values1 = cache.Get("sc1")
	assert.Len(t, values1, 1)
	assert.NotContains(t, values1, "vol1")

	// Reverse: vol1 should only belong to sc2 now
	keys = cache.FindKeyForValue("vol1")
	assert.Len(t, keys, 1)
	assert.Equal(t, []string{"sc2"}, keys)

	// Delete vol1 from sc2 (now completely removed)
	_ = cache.Delete("sc2", "vol1")

	// Reverse: vol1 should not belong to any key
	keys = cache.FindKeyForValue("vol1")
	assert.Empty(t, keys, "vol1 should be completely removed from reverse mapping")
}

func TestMultiValueCache_SymmetricPerformance(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Create many-to-many relationships
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		for j := 0; j < 10; j++ {
			value := fmt.Sprintf("val-%d", j)
			_ = cache.Set(key, value)
		}
	}

	// Forward Get should be O(1)
	values := cache.Get("key-5")
	assert.Len(t, values, 10)

	// Reverse FindKeyForValue should also be O(1)
	keys := cache.FindKeyForValue("val-5")
	assert.Len(t, keys, 100, "val-5 should belong to all 100 keys")

	// Delete should be O(1) in both directions
	_ = cache.Delete("key-5", "val-5")

	values = cache.Get("key-5")
	assert.Len(t, values, 9)

	keys = cache.FindKeyForValue("val-5")
	assert.Len(t, keys, 99, "val-5 should now belong to 99 keys")
}

func TestMultiValueCache_ConcurrentAccess(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// This is a basic concurrency test - proper testing would use race detector
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = cache.Set("sc1", fmt.Sprintf("val-%d", i))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = cache.Get("sc1")
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	// Should not panic or deadlock
	values := cache.Get("sc1")
	assert.Len(t, values, 100)
}

func TestMultiValueCache_Len(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Empty cache should have length 0
	assert.Equal(t, 0, cache.Len())

	// Add one key with one value
	_ = cache.Set("key1", "val1")
	assert.Equal(t, 1, cache.Len())

	// Add another value to same key - length should stay 1
	_ = cache.Set("key1", "val2")
	assert.Equal(t, 1, cache.Len())

	// Add a new key - length should increase to 2
	_ = cache.Set("key2", "val1")
	assert.Equal(t, 2, cache.Len())

	// Add more keys
	_ = cache.Set("key3", "val3")
	_ = cache.Set("key4", "val4")
	assert.Equal(t, 4, cache.Len())

	// Delete a value but not all values for a key - length should stay same
	_ = cache.Delete("key1", "val1")
	assert.Equal(t, 4, cache.Len())

	// Delete last value for a key - length should decrease
	_ = cache.Delete("key1", "val2")
	assert.Equal(t, 3, cache.Len())

	// Delete entire key worth of values
	_ = cache.Delete("key2", "val1")
	assert.Equal(t, 2, cache.Len())
}

func TestMultiValueCache_Clear(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Add multiple keys with multiple values
	_ = cache.Set("key1", "val1")
	_ = cache.Set("key1", "val2")
	_ = cache.Set("key2", "val3")
	_ = cache.Set("key3", "val4")
	_ = cache.Set("key3", "val5")

	// Verify cache has data
	assert.Equal(t, 3, cache.Len())
	assert.Len(t, cache.Get("key1"), 2)

	// Clear the cache
	cache.Clear()

	// Verify cache is empty
	assert.Equal(t, 0, cache.Len())
	assert.Empty(t, cache.Get("key1"))
	assert.Empty(t, cache.Get("key2"))
	assert.Empty(t, cache.Get("key3"))

	// Verify reverse mappings are also cleared
	assert.Empty(t, cache.FindKeyForValue("val1"))
	assert.Empty(t, cache.FindKeyForValue("val3"))

	// Verify List returns empty
	list := cache.List()
	assert.Empty(t, list)

	// Verify we can add new entries after clear
	err := cache.Set("newkey", "newval")
	assert.NoError(t, err)
	assert.Equal(t, 1, cache.Len())
	assert.Equal(t, []string{"newval"}, cache.Get("newkey"))
}

func TestMultiValueCache_List(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Empty cache should return empty map
	list := cache.List()
	assert.NotNil(t, list)
	assert.Empty(t, list)

	// Add some data
	_ = cache.Set("sc1", "vol1")
	_ = cache.Set("sc1", "vol2")
	_ = cache.Set("sc2", "vol3")
	_ = cache.Set("sc3", "vol4")
	_ = cache.Set("sc3", "vol5")
	_ = cache.Set("sc3", "vol6")

	// Get list
	list = cache.List()
	assert.Len(t, list, 3)
	assert.ElementsMatch(t, []string{"vol1", "vol2"}, list["sc1"])
	assert.ElementsMatch(t, []string{"vol3"}, list["sc2"])
	assert.ElementsMatch(t, []string{"vol4", "vol5", "vol6"}, list["sc3"])

	// Verify defensive copy - modifying returned list should not affect cache
	list["sc1"][0] = "modified"
	list["sc2"] = []string{"new"}
	list["newkey"] = []string{"val"}

	// Original cache should be unchanged
	assert.Equal(t, []string{"vol1", "vol2"}, cache.Get("sc1"))
	assert.Equal(t, []string{"vol3"}, cache.Get("sc2"))
	assert.Empty(t, cache.Get("newkey"))
	assert.Equal(t, 3, cache.Len())
}

func TestMultiValueCache_List_AfterDelete(t *testing.T) {
	cache := NewMultiValueCache[string, string]()

	// Add data
	_ = cache.Set("key1", "val1")
	_ = cache.Set("key1", "val2")
	_ = cache.Set("key2", "val3")

	// Delete a value
	_ = cache.Delete("key1", "val1")

	// List should reflect the deletion
	list := cache.List()
	assert.Len(t, list, 2)
	assert.ElementsMatch(t, []string{"val2"}, list["key1"])
	assert.ElementsMatch(t, []string{"val3"}, list["key2"])

	// Delete all values for a key
	_ = cache.Delete("key1", "val2")

	// List should not contain the deleted key
	list = cache.List()
	assert.Len(t, list, 1)
	assert.NotContains(t, list, "key1")
	assert.ElementsMatch(t, []string{"val3"}, list["key2"])
}
