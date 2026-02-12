// Copyright 2025 NetApp, Inc. All Rights Reserved.

package generic_cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestSingleValueCache_New(t *testing.T) {
	cache := NewSingleValueCache[string, string]()
	assert.NotNil(t, cache)
	assert.Equal(t, 0, cache.Len())
}

func TestSingleValueCache_Set(t *testing.T) {
	tests := []struct {
		name                string
		key                 string
		value               string
		expectError         bool
		checkZeroValueError bool
	}{
		{
			name:        "valid key and value",
			key:         "key1",
			value:       "value1",
			expectError: false,
		},
		{
			name:                "empty key",
			key:                 "",
			value:               "value1",
			expectError:         true,
			checkZeroValueError: true,
		},
		{
			name:        "empty value is allowed",
			key:         "key1",
			value:       "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewSingleValueCache[string, string]()
			err := cache.Set(tt.key, tt.value)

			if tt.expectError {
				assert.Error(t, err)
				if tt.checkZeroValueError {
					assert.True(t, errors.IsZeroValueError(err), "Should return ZeroValueError")
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, cache.Has(tt.key))
				values := cache.Get(tt.key)
				assert.Len(t, values, 1)
				assert.Equal(t, tt.value, values[0])
			}
		})
	}
}

func TestSingleValueCache_Set_Overwrite(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	assert.NoError(t, cache.Set("key1", "value1"))
	values := cache.Get("key1")
	assert.Equal(t, "value1", values[0])

	// Overwrite with new value
	assert.NoError(t, cache.Set("key1", "value2"))
	values = cache.Get("key1")
	assert.Len(t, values, 1)
	assert.Equal(t, "value2", values[0])
}

func TestSingleValueCache_Set_Idempotent(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	// Add value first time
	err := cache.Set("key1", "value1")
	assert.NoError(t, err)
	values := cache.Get("key1")
	assert.Equal(t, []string{"value1"}, values)

	// Add same key-value pair again - should return error
	err = cache.Set("key1", "value1")
	assert.Error(t, err)
	assert.True(t, errors.IsAlreadyExistsError(err), "Should return AlreadyExistsError")
	values = cache.Get("key1")
	assert.Equal(t, []string{"value1"}, values, "Cache should remain unchanged")

	// Add third time - still returns error
	err = cache.Set("key1", "value1")
	assert.Error(t, err)
	assert.True(t, errors.IsAlreadyExistsError(err), "Should return AlreadyExistsError")
	values = cache.Get("key1")
	assert.Equal(t, []string{"value1"}, values, "Cache should remain unchanged")

	// Verify reverse index is not polluted
	keys := cache.FindKeyForValue("value1")
	assert.Len(t, keys, 1, "Reverse index should only have one entry for the key")
	assert.Contains(t, keys, "key1")
}

func TestSingleValueCache_Get(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	// Get non-existent key returns empty slice
	values := cache.Get("nonexistent")
	assert.Empty(t, values)
	assert.NotNil(t, values)

	// Add value and get it
	assert.NoError(t, cache.Set("key1", "value1"))
	values = cache.Get("key1")
	assert.Len(t, values, 1)
	assert.Equal(t, "value1", values[0])
}

func TestSingleValueCache_Delete(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	assert.NoError(t, cache.Set("key1", "value1"))
	assert.True(t, cache.Has("key1"))

	// Delete key (value parameter is ignored)
	err := cache.Delete("key1", "ignored")
	assert.NoError(t, err)
	assert.False(t, cache.Has("key1"))
	assert.Empty(t, cache.Get("key1"))
}

func TestSingleValueCache_Delete_NonExistent(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	// Deleting non-existent key should return error
	err := cache.Delete("nonexistent", "")
	assert.Error(t, err)
	assert.True(t, errors.IsKeyError(err), "Should return KeyError")
}

func TestSingleValueCache_DeleteKey(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	assert.NoError(t, cache.Set("key1", "value1"))
	assert.True(t, cache.Has("key1"))

	// Delete key using DeleteKey method
	err := cache.DeleteKey("key1")
	assert.NoError(t, err)
	assert.False(t, cache.Has("key1"))
	assert.Empty(t, cache.Get("key1"))
}

func TestSingleValueCache_Has(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	assert.False(t, cache.Has("key1"))

	assert.NoError(t, cache.Set("key1", "value1"))
	assert.True(t, cache.Has("key1"))

	cache.Delete("key1", "")
	assert.False(t, cache.Has("key1"))
}

func TestSingleValueCache_Len(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	assert.Equal(t, 0, cache.Len())

	assert.NoError(t, cache.Set("key1", "value1"))
	assert.Equal(t, 1, cache.Len())

	assert.NoError(t, cache.Set("key1", "value2")) // Overwrite, still 1
	assert.Equal(t, 1, cache.Len())

	assert.NoError(t, cache.Set("key2", "value1"))
	assert.Equal(t, 2, cache.Len())

	cache.Delete("key1", "")
	assert.Equal(t, 1, cache.Len())
}

func TestSingleValueCache_Clear(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	assert.NoError(t, cache.Set("key1", "value1"))
	assert.NoError(t, cache.Set("key2", "value2"))
	assert.Equal(t, 2, cache.Len())

	cache.Clear()
	assert.Equal(t, 0, cache.Len())
	assert.False(t, cache.Has("key1"))
	assert.False(t, cache.Has("key2"))
}

func TestSingleValueCache_FindKeyForValue(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	// Not found
	keys := cache.FindKeyForValue("value1")
	assert.Empty(t, keys)

	// Add values
	assert.NoError(t, cache.Set("key1", "value1"))
	assert.NoError(t, cache.Set("key2", "value2"))

	// Find existing values
	keys = cache.FindKeyForValue("value1")
	assert.Len(t, keys, 1)
	assert.Contains(t, keys, "key1")

	keys = cache.FindKeyForValue("value2")
	assert.Len(t, keys, 1)
	assert.Contains(t, keys, "key2")

	// Not found
	keys = cache.FindKeyForValue("nonexistent")
	assert.Empty(t, keys)
}

// TestSingleValueCache_FindKeyForValue_MultipleKeys tests finding all keys that map to same value
func TestSingleValueCache_FindKeyForValue_MultipleKeys(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	// Multiple keys mapping to the same value (many-to-one)
	assert.NoError(t, cache.Set("volume1", "policyA"))
	assert.NoError(t, cache.Set("volume2", "policyA"))
	assert.NoError(t, cache.Set("volume3", "policyB"))

	// Find all keys that map to "policyA" - should return both volume1 and volume2
	keys := cache.FindKeyForValue("policyA")
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "volume1")
	assert.Contains(t, keys, "volume2")

	// Find key that maps to "policyB" - should return only volume3
	keys = cache.FindKeyForValue("policyB")
	assert.Len(t, keys, 1)
	assert.Contains(t, keys, "volume3")

	// Not found
	keys = cache.FindKeyForValue("policyC")
	assert.Empty(t, keys)
}

func TestSingleValueCache_List(t *testing.T) {
	cache := NewSingleValueCache[string, string]()

	// Empty cache
	list := cache.List()
	assert.Empty(t, list)

	// Add some values
	cache.Set("vol1", "policy1")
	cache.Set("vol2", "policy2")

	list = cache.List()
	assert.Len(t, list, 2)
	assert.Equal(t, []string{"policy1"}, list["vol1"])
	assert.Equal(t, []string{"policy2"}, list["vol2"])

	// Verify it's a defensive copy - mutation shouldn't affect cache
	list["vol1"][0] = "modified"
	originalValue := cache.Get("vol1")
	assert.Equal(t, []string{"policy1"}, originalValue)
}

func TestSingleValueCache_Concurrent(t *testing.T) {
	cache := NewSingleValueCache[string, string]()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = cache.Set("key1", string(rune('a'+n%26)))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cache.Get("key1")
		}()
	}

	wg.Wait()

	// Verify cache is still valid
	assert.True(t, cache.Has("key1"))
	values := cache.Get("key1")
	assert.Len(t, values, 1)
}
