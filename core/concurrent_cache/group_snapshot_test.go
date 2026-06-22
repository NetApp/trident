// Copyright 2026 NetApp, Inc. All Rights Reserved.

package concurrent_cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage"
)

func newTestGroupSnapshot(id string, volumeNames, snapshotIDs []string) *storage.GroupSnapshot {
	return storage.NewGroupSnapshot(
		&storage.GroupSnapshotConfig{
			Name:        id,
			VolumeNames: volumeNames,
		},
		snapshotIDs,
		"2026-01-01T00:00:00Z",
	)
}

// Step 2: verify the groupSnapshot resource plumbing (enum placement, rank, registration, schema).

func TestGroupSnapshot_ResourcePlumbing(t *testing.T) {
	Initialize()

	// The cache must be registered and initialized.
	c, ok := caches[groupSnapshot]
	assert.True(t, ok, "groupSnapshot must be registered in the caches map")
	assert.NotNil(t, c, "groupSnapshot cache must be non-nil")
	assert.NotNil(t, c.data, "groupSnapshot cache data map must be initialized")
	assert.NotNil(t, c.resourceLocks, "groupSnapshot cache resourceLocks must be initialized")
	assert.Nil(t, c.key, "groupSnapshot is not keyed by a unique key")

	// It is a parentless resource, so rank 0.
	assert.Equal(t, 0, resourceRanks[groupSnapshot], "groupSnapshot must be rank 0 (parentless)")
	assert.Empty(t, schema[groupSnapshot], "groupSnapshot must have no schema dependencies")

	// It must have a human-readable name.
	assert.Equal(t, "Group Snapshot", resourceNames[groupSnapshot])
	assert.Equal(t, "Group Snapshot", groupSnapshot.String())
}

// TestGroupSnapshot_LockOrderBeforeBackend is the critical ordering guarantee: a group-snapshot lock must sort
// before backend/volume/snapshot locks so that "lock group first, then NestedLock the constituent trees" is legal.
func TestGroupSnapshot_LockOrderBeforeBackend(t *testing.T) {
	Initialize()

	gs := Subquery{res: groupSnapshot, op: upsert}
	for _, dep := range []resource{backend, volume, snapshot} {
		other := Subquery{res: dep, op: read}
		assert.Negative(t, compareSubqueries(gs, other),
			"groupSnapshot must sort before %s", dep)
		assert.Positive(t, compareSubqueries(other, gs),
			"%s must sort after groupSnapshot", dep)
	}
}

// Step 3: exercise the query builders end-to-end through the real Lock API (single-threaded round trips).

func TestGroupSnapshot_CacheRoundTrip(t *testing.T) {
	Initialize()

	id := "groupsnapshot-13695cff-d8ab-486e-98df-971a6b235a67"
	gs := newTestGroupSnapshot(id, []string{"vol1", "vol2"}, []string{"vol1/snap1", "vol2/snap1"})

	// Upsert.
	_, results, unlock, err := Lock(context.Background(), Query(UpsertGroupSnapshot(id)))
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Nil(t, results[0].GroupSnapshot.Read, "no prior record should exist")
	assert.NotNil(t, results[0].GroupSnapshot.Upsert, "Upsert func should be provided")
	results[0].GroupSnapshot.Upsert(gs)
	unlock()

	// Read it back; the cache must hand out an independent deep copy.
	_, results, unlock, err = Lock(context.Background(), Query(ReadGroupSnapshot(id)))
	assert.NoError(t, err)
	got := results[0].GroupSnapshot.Read
	assert.NotNil(t, got, "group snapshot should be readable after upsert")
	assert.Equal(t, id, got.ID())
	assert.Equal(t, []string{"vol1", "vol2"}, got.GetVolumeNames())
	assert.Equal(t, []string{"vol1/snap1", "vol2/snap1"}, got.GetSnapshotIDs())
	assert.NotSame(t, gs, got, "read must return a copy, not the stored pointer")
	unlock()

	// List should contain exactly the one record.
	_, results, unlock, err = Lock(context.Background(), Query(ListGroupSnapshots()))
	assert.NoError(t, err)
	assert.Len(t, results[0].GroupSnapshots, 1)
	assert.Equal(t, id, results[0].GroupSnapshots[0].ID())
	unlock()

	// Delete.
	_, results, unlock, err = Lock(context.Background(), Query(DeleteGroupSnapshot(id)))
	assert.NoError(t, err)
	assert.NotNil(t, results[0].GroupSnapshot.Read, "Delete should read the existing record")
	assert.NotNil(t, results[0].GroupSnapshot.Delete)
	results[0].GroupSnapshot.Delete()
	unlock()

	// Read after delete must miss.
	_, results, unlock, err = Lock(context.Background(), Query(ReadGroupSnapshot(id)))
	assert.NoError(t, err)
	assert.Nil(t, results[0].GroupSnapshot.Read, "record should be gone after delete")
	unlock()

	// List must now be empty.
	_, results, unlock, err = Lock(context.Background(), Query(ListGroupSnapshots()))
	assert.NoError(t, err)
	assert.Empty(t, results[0].GroupSnapshots)
	unlock()
}

func TestGroupSnapshot_InconsistentRead(t *testing.T) {
	Initialize()

	id := "groupsnapshot-inconsistent"
	gs := newTestGroupSnapshot(id, []string{"vol1"}, []string{"vol1/snap1"})

	// Seed directly into the cache.
	groupSnapshots.lock()
	groupSnapshots.data[id] = gs
	groupSnapshots.unlock()

	_, results, unlock, err := Lock(context.Background(), Query(InconsistentReadGroupSnapshot(id)))
	assert.NoError(t, err)
	assert.NotNil(t, results[0].GroupSnapshot.Read)
	assert.Equal(t, id, results[0].GroupSnapshot.Read.ID())
	unlock()

	// Missing id yields a nil read, not an error.
	_, results, unlock, err = Lock(context.Background(), Query(InconsistentReadGroupSnapshot("does-not-exist")))
	assert.NoError(t, err)
	assert.Nil(t, results[0].GroupSnapshot.Read)
	unlock()
}

func TestGroupSnapshot_UpsertUpdatesExisting(t *testing.T) {
	Initialize()

	id := "groupsnapshot-update"
	original := newTestGroupSnapshot(id, []string{"vol1"}, []string{"vol1/snap1"})
	updated := newTestGroupSnapshot(id, []string{"vol1", "vol2"}, []string{"vol1/snap1", "vol2/snap1"})

	_, results, unlock, err := Lock(context.Background(), Query(UpsertGroupSnapshot(id)))
	assert.NoError(t, err)
	results[0].GroupSnapshot.Upsert(original)
	unlock()

	// Second upsert returns the prior record as Read and replaces it.
	_, results, unlock, err = Lock(context.Background(), Query(UpsertGroupSnapshot(id)))
	assert.NoError(t, err)
	assert.NotNil(t, results[0].GroupSnapshot.Read, "prior record should be returned on update")
	assert.Equal(t, []string{"vol1"}, results[0].GroupSnapshot.Read.GetVolumeNames())
	results[0].GroupSnapshot.Upsert(updated)
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ReadGroupSnapshot(id)))
	assert.NoError(t, err)
	assert.Equal(t, []string{"vol1", "vol2"}, results[0].GroupSnapshot.Read.GetVolumeNames())
	unlock()
}
