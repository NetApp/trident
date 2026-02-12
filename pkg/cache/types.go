// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cache

// Cache defines a generic interface for caching with type parameters.
// K must be comparable (suitable for use as map keys).
// V must be comparable (for value equality checks in Delete and FindKeyForValue).
// It provides a uniform API for different cache types (SC→Volumes, Volume→Policy, etc.)
// All implementations are thread-safe.
type Cache[K comparable, V comparable] interface {
	// Set adds or updates a value for a key.
	// For single-value caches, this replaces the existing value.
	// For multi-value caches, this appends the value.
	Set(key K, value V) error

	// Get retrieves all values associated with a key.
	// Returns empty slice if key doesn't exist.
	// Always returns a defensive copy to prevent external mutation.
	Get(key K) []V

	// Delete removes a specific value from a key's list (multi-value cache).
	// For single-value caches: removes the key entirely (value parameter ignored).
	// Returns error only for invalid inputs.
	Delete(key K, value V) error

	// DeleteKey removes all values associated with a key.
	// This is clearer than using Delete with a zero value.
	DeleteKey(key K) error

	// Has checks if a key exists in the cache.
	Has(key K) bool

	// Len returns the number of keys in the cache.
	Len() int

	// Clear removes all entries from the cache.
	Clear()

	// FindKeyForValue searches for all keys that contain the given value.
	// Returns a slice of keys. Empty slice if value not found.
	// For single-value caches: returns at most one key.
	// For multi-value caches: can return multiple keys if same value exists in multiple keys.
	// Always returns a defensive copy to prevent external mutation.
	FindKeyForValue(value V) []K

	// List returns all key-value pairs in the cache as a defensive copy.
	// For multi-value caches: returns map[K][]V
	// For single-value caches: returns map[K][]V (each V wrapped in slice for consistency)
	List() map[K][]V
}
