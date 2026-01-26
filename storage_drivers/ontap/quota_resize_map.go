// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"sync"
)

// quotaResizeMap is a thread-safe map for tracking FlexVols that need quota resize operations.
// This map is accessed concurrently by:
// - Volume create/delete/resize operations that modify qtree quotas
// - Background housekeeping tasks that process pending quota resize operations
//
// Design Note: sync.Map was considered but not used because:
// 1. Type safety: sync.Map requires interface{} with runtime type assertions, our wrapper is type-safe
// 2. Write-heavy workload: Every volume create/delete writes to this map, sync.Map is optimized for read-heavy
// 3. Snapshot semantics: GetPending() needs a consistent snapshot for safe iteration during housekeeping
// 4. Simplicity: Standard map with RWMutex is easier to understand and maintain for this use case
type quotaResizeMap struct {
	data map[string]bool
	mu   sync.RWMutex
}

// newQuotaResizeMap creates a new thread-safe quota resize map.
func newQuotaResizeMap() *quotaResizeMap {
	return &quotaResizeMap{
		data: make(map[string]bool),
	}
}

// Set marks a FlexVol as needing a quota resize operation.
func (m *quotaResizeMap) Set(flexvol string, needsResize bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[flexvol] = needsResize
}

// Get returns whether a FlexVol needs a quota resize operation.
func (m *quotaResizeMap) Get(flexvol string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[flexvol]
}

// Delete removes a FlexVol from the quota resize map.
func (m *quotaResizeMap) Delete(flexvol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, flexvol)
}

// GetPending returns a snapshot of all FlexVols currently marked for quota resize.
// This returns a copy of the keys to allow safe iteration without holding the lock.
func (m *quotaResizeMap) GetPending() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pending := make([]string, 0, len(m.data))
	for flexvol, needsResize := range m.data {
		if needsResize {
			pending = append(pending, flexvol)
		}
	}
	return pending
}

// Len returns the number of FlexVols in the map.
func (m *quotaResizeMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Clear removes all entries from the map.
func (m *quotaResizeMap) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]bool)
}
