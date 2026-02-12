// Copyright 2025 NetApp, Inc. All Rights Reserved.

package generic_cache

import (
	"slices"
	"sync"

	"github.com/netapp/trident/utils/errors"
)

// MultiValueCache implements the Cache[K, V] interface for many-to-many relationships
// with idempotent operations and O(1) performance for ALL operations in BOTH directions.
//
// Key features:
// - Idempotent Set: Adding the same key-value pair multiple times has no effect - O(1)
// - O(1) Delete: Uses swap-and-pop technique for constant-time deletion in both directions
// - O(1) Get: Returns pre-computed array copy (no iteration)
// - O(1) FindKeyForValue: Returns pre-computed array copy (symmetric with Get)
// - Fully symmetric: Both forward (K→V) and reverse (V→K) use same optimization
// - Thread-safe: Uses RWMutex for concurrent access
//
// Data structures (fully symmetric):
// Forward mapping:
// - mapping: map[K]map[V]int stores the index of each value in arrays[K]
// - arrays: map[K][]V cached arrays for instant Get(K) returns
//
// Reverse mapping (symmetric!):
// - valueToKeys: map[V]map[K]int stores the index of each key in reverseArrays[V]
// - reverseArrays: map[V][]K cached arrays for instant FindKeyForValue(V) returns
//
// The key optimization is storing array indices in both mapping and valueToKeys:
// - Set: O(1) - append to both arrays, store indices in both maps
// - Delete: O(1) - swap-and-pop in both directions, update both indices
// - Get: O(1) - return cached array copy from arrays
// - FindKeyForValue: O(1) - return cached array copy from reverseArrays
//
// Example use case: Many-to-many relationships like Tags → Resources
// - One tag can have multiple resources
// - One resource can have multiple tags
// - Need fast lookup in both directions
type MultiValueCache[K comparable, V comparable] struct {
	mu            sync.RWMutex
	mapping       map[K]map[V]int // forward: key -> value -> index in arrays[key]
	arrays        map[K][]V       // forward: key -> cached array
	valueToKeys   map[V]map[K]int // reverse: value -> key -> index in reverseArrays[value]
	reverseArrays map[V][]K       // reverse: value -> cached array of keys
}

// NewMultiValueCache creates a new idempotent multi-value cache.
func NewMultiValueCache[K comparable, V comparable]() *MultiValueCache[K, V] {
	return &MultiValueCache[K, V]{
		mapping:       make(map[K]map[V]int),
		arrays:        make(map[K][]V),
		valueToKeys:   make(map[V]map[K]int),
		reverseArrays: make(map[V][]K),
	}
}

// Set adds a value to the set of values for a key.
// Returns error if the same key-value pair already exists.
// O(1) operation - updates both forward and reverse mappings.
func (c *MultiValueCache[K, V]) Set(key K, value V) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for zero value key
	var zeroK K
	if key == zeroK {
		return errors.ZeroValueError("key must not be zero value")
	}

	// Initialize forward mapping if first value for this key
	if c.mapping[key] == nil {
		c.mapping[key] = make(map[V]int)
		c.arrays[key] = []V{}
	}

	// Initialize reverse mapping if first key for this value
	if c.valueToKeys[value] == nil {
		c.valueToKeys[value] = make(map[K]int)
		c.reverseArrays[value] = []K{}
	}

	// Check if already present - return error instead of being idempotent
	if _, exists := c.mapping[key][value]; exists {
		return errors.AlreadyExistsError("key %v with value %v already exists in cache", key, value)
	}

	// Forward: Append value to arrays[key] and store its index
	forwardIndex := len(c.arrays[key])
	c.arrays[key] = append(c.arrays[key], value)
	c.mapping[key][value] = forwardIndex

	// Reverse: Append key to reverseArrays[value] and store its index
	reverseIndex := len(c.reverseArrays[value])
	c.reverseArrays[value] = append(c.reverseArrays[value], key)
	c.valueToKeys[value][key] = reverseIndex

	return nil
}

// Get returns all values for a key as a defensive copy.
// O(1) operation - returns cached array copy (no iteration needed).
func (c *MultiValueCache[K, V]) Get(key K) []V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	values := c.arrays[key]
	if values == nil {
		return []V{}
	}

	// Return defensive copy
	return slices.Clone(values)
}

// Delete removes a specific key-value pair from the cache.
// Idempotent: Removing a non-existent pair is a no-op.
// O(1) operation using swap-and-pop technique in BOTH directions:
// Forward: Remove value from key's array
// Reverse: Remove key from value's array
func (c *MultiValueCache[K, V]) Delete(key K, value V) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if pair exists in forward mapping
	if c.mapping[key] == nil {
		return errors.KeyNotFoundError(key)
	}

	forwardIndex, exists := c.mapping[key][value]
	if !exists {
		return errors.ValueNotFoundError(value, key)
	}

	// === Forward deletion: Remove value from arrays[key] using swap-and-pop ===
	forwardArr := c.arrays[key]
	forwardLastIndex := len(forwardArr) - 1

	if forwardIndex != forwardLastIndex {
		// Swap with last element
		lastValue := forwardArr[forwardLastIndex]
		forwardArr[forwardIndex] = lastValue
		// Update the moved element's index in the forward map
		c.mapping[key][lastValue] = forwardIndex
	}

	// Truncate forward array
	c.arrays[key] = forwardArr[:forwardLastIndex]

	// Remove value from forward map
	delete(c.mapping[key], value)

	// Clean up empty key in forward mapping
	if len(c.mapping[key]) == 0 {
		delete(c.mapping, key)
		delete(c.arrays, key)
	}

	// === Reverse deletion: Remove key from reverseArrays[value] using swap-and-pop ===
	if c.valueToKeys[value] == nil {
		return nil // Should not happen, but be defensive
	}

	reverseIndex, exists := c.valueToKeys[value][key]
	if !exists {
		return nil // Should not happen, but be defensive
	}

	reverseArr := c.reverseArrays[value]
	reverseLastIndex := len(reverseArr) - 1

	if reverseIndex != reverseLastIndex {
		// Swap with last element
		lastKey := reverseArr[reverseLastIndex]
		reverseArr[reverseIndex] = lastKey
		// Update the moved element's index in the reverse map
		c.valueToKeys[value][lastKey] = reverseIndex
	}

	// Truncate reverse array
	c.reverseArrays[value] = reverseArr[:reverseLastIndex]

	// Remove key from reverse map
	delete(c.valueToKeys[value], key)

	// Clean up empty value in reverse mapping
	if len(c.valueToKeys[value]) == 0 {
		delete(c.valueToKeys, value)
		delete(c.reverseArrays, value)
	}

	return nil
}

// DeleteKey removes all values for a key.
// Also cleans up reverse mappings for all affected values.
// O(n) where n = number of values for this key (typically small).
func (c *MultiValueCache[K, V]) DeleteKey(key K) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.mapping[key] == nil {
		return errors.KeyNotFoundError(key)
	}

	// For each value associated with this key, remove key from reverse mapping
	for value := range c.mapping[key] {
		if c.valueToKeys[value] != nil {
			reverseIndex, exists := c.valueToKeys[value][key]
			if exists {
				// Use swap-and-pop to remove key from reverseArrays[value]
				reverseArr := c.reverseArrays[value]
				reverseLastIndex := len(reverseArr) - 1

				if reverseIndex != reverseLastIndex {
					lastKey := reverseArr[reverseLastIndex]
					reverseArr[reverseIndex] = lastKey
					c.valueToKeys[value][lastKey] = reverseIndex
				}

				c.reverseArrays[value] = reverseArr[:reverseLastIndex]
				delete(c.valueToKeys[value], key)

				// Clean up empty value
				if len(c.valueToKeys[value]) == 0 {
					delete(c.valueToKeys, value)
					delete(c.reverseArrays, value)
				}
			}
		}
	}

	// Remove forward mappings
	delete(c.mapping, key)
	delete(c.arrays, key)
	return nil
}

// Has checks if a key exists in the cache.
// O(1) operation.
func (c *MultiValueCache[K, V]) Has(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.mapping[key]
	return ok
}

// Len returns the number of keys in the cache.
// O(1) operation.
func (c *MultiValueCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.mapping)
}

// Clear removes all entries from the cache.
func (c *MultiValueCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mapping = make(map[K]map[V]int)
	c.arrays = make(map[K][]V)
	c.valueToKeys = make(map[V]map[K]int)
	c.reverseArrays = make(map[V][]K)
}

// FindKeyForValue returns all keys that contain the given value.
// O(1) operation - returns cached array copy (symmetric with Get).
// Supports many-to-many relationships where one value can belong to multiple keys.
func (c *MultiValueCache[K, V]) FindKeyForValue(value V) []K {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := c.reverseArrays[value]
	if keys == nil {
		return []K{}
	}

	// Return defensive copy (symmetric with Get)
	return slices.Clone(keys)
}

// List returns all key-value pairs in the cache as a defensive copy.
// Returns map[K][]V with copies of all arrays.
func (c *MultiValueCache[K, V]) List() map[K][]V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[K][]V, len(c.arrays))
	for key, values := range c.arrays {
		// Make defensive copy of each array
		result[key] = slices.Clone(values)
	}
	return result
}
