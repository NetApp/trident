// Copyright 2025 NetApp, Inc. All Rights Reserved.

package generic_cache

import (
	"sync"

	"github.com/netapp/trident/utils/errors"
)

// SingleValueCache implements the Cache[K, V] interface (defined in parent cache package)
// for one-to-one relationships with generic types.
// K must be comparable (suitable for map keys).
// V must also be comparable (for value lookup).
// Example: Volume (string) → PolicyName (string), PVC (string) → SnapshotName (string)
// Thread-safe implementation using RWMutex.
// Maintains a reverse index (valueToKeys) for O(1) value lookups.
type SingleValueCache[K comparable, V comparable] struct {
	mu          sync.RWMutex
	mapping     map[K]V   // key -> value
	valueToKeys map[V][]K // reverse index: value -> all keys with that value
}

// NewSingleValueCache creates a new single-value cache.
func NewSingleValueCache[K comparable, V comparable]() *SingleValueCache[K, V] {
	return &SingleValueCache[K, V]{
		mapping:     make(map[K]V),
		valueToKeys: make(map[V][]K),
	}
}

// Set stores a single value for a key.
// Returns error if the same key-value pair already exists.
// If the key exists with a different value, it replaces the old value.
// Key must not be zero value. Value can be any valid V type (including zero).
func (c *SingleValueCache[K, V]) Set(key K, value V) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for zero value key
	var zeroK K
	if key == zeroK {
		return errors.ZeroValueError("key must not be zero value")
	}

	// Check if key already exists with the same value - return error
	if oldValue, exists := c.mapping[key]; exists && oldValue == value {
		return errors.AlreadyExistsError("key %v with value %v already exists in cache", key, value)
	}

	// If key already exists with a different value, remove old reverse index entry
	if oldValue, exists := c.mapping[key]; exists {
		c.removeKeyFromReverseIndex(key, oldValue)
	}

	// Set the new value
	c.mapping[key] = value

	// Update reverse index - add this key to the list of keys for this value
	c.valueToKeys[value] = append(c.valueToKeys[value], key)

	return nil
}

// Get returns the value for a key as a single-element slice.
// Returns empty slice if key doesn't exist.
func (c *SingleValueCache[K, V]) Get(key K) []V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.mapping[key]
	if !ok {
		return []V{}
	}

	return []V{value}
}

// Delete removes a key from the cache.
// The value parameter is ignored for single-value caches.
func (c *SingleValueCache[K, V]) Delete(key K, value V) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.mapping[key]; !exists {
		return errors.KeyNotFoundError(key)
	}

	// Remove from reverse index
	if oldValue, exists := c.mapping[key]; exists {
		c.removeKeyFromReverseIndex(key, oldValue)
	}

	delete(c.mapping, key)
	return nil
}

// DeleteKey removes a key from the cache.
// This is the same as Delete for single-value caches.
func (c *SingleValueCache[K, V]) DeleteKey(key K) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.mapping[key]; !exists {
		return errors.KeyNotFoundError(key)
	}

	// Remove from reverse index
	if oldValue, exists := c.mapping[key]; exists {
		c.removeKeyFromReverseIndex(key, oldValue)
	}

	delete(c.mapping, key)
	return nil
}

// Has checks if a key exists in the cache.
func (c *SingleValueCache[K, V]) Has(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.mapping[key]
	return ok
}

// Len returns the number of keys in the cache.
func (c *SingleValueCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.mapping)
}

// Clear removes all entries from the cache.
func (c *SingleValueCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mapping = make(map[K]V)
	c.valueToKeys = make(map[V][]K)
}

// FindKeyForValue searches for all keys that map to the given value.
// Returns slice with all matching keys, empty slice if none found.
// This is an O(1) operation using the reverse index.
// Note: Multiple keys can map to the same value (many-to-one).
// Returns a defensive copy to prevent external mutation.
func (c *SingleValueCache[K, V]) FindKeyForValue(value V) []K {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := c.valueToKeys[value]
	if keys == nil {
		return []K{}
	}

	// Return defensive copy
	result := make([]K, len(keys))
	copy(result, keys)
	return result
}

// removeKeyFromReverseIndex removes a specific key from a value's key list in the reverse index.
// Must be called with lock held.
func (c *SingleValueCache[K, V]) removeKeyFromReverseIndex(key K, value V) {
	keys := c.valueToKeys[value]
	for i, k := range keys {
		if k == key {
			// Remove by replacing with last element and truncating
			c.valueToKeys[value][i] = keys[len(keys)-1]
			c.valueToKeys[value] = keys[:len(keys)-1]

			// Clean up empty slices
			if len(c.valueToKeys[value]) == 0 {
				delete(c.valueToKeys, value)
			}
			break
		}
	}
}

// List returns all key-value pairs in the cache as a defensive copy.
// Returns map[K][]V with each single value wrapped in a slice for consistency with the interface.
func (c *SingleValueCache[K, V]) List() map[K][]V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[K][]V, len(c.mapping))
	for key, value := range c.mapping {
		// Wrap single value in slice for consistent interface
		result[key] = []V{value}
	}
	return result
}
