package concurrent_cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/object"
)

func TestBucketAccessSubqueries(t *testing.T) {
	Initialize()

	store := newTestObjectStore("store1", "store-id-1")
	objectStores.data[store.StoreUUID()] = store
	objectStores.key.data[store.GetUniqueKey()] = store.StoreUUID()

	otherStore := newTestObjectStore("store2", "store-id-2")
	objectStores.data[otherStore.StoreUUID()] = otherStore
	objectStores.key.data[otherStore.GetUniqueKey()] = otherStore.StoreUUID()

	accessID := "access-id-1"
	access := &object.BucketAccess{
		Name:      accessID,
		StoreName: store.GetUniqueKey(),
	}
	bucketAccesses.data[accessID] = access

	otherAccess := &object.BucketAccess{
		Name:      "access-id-2",
		StoreName: otherStore.GetUniqueKey(),
	}
	bucketAccesses.data["access-id-2"] = otherAccess

	_, results, unlock, err := Lock(context.Background(), Query(ListBucketAccesses()))
	require.NoError(t, err)
	require.Len(t, results[0].BucketAccesses, 2)
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ListBucketAccessesForObjectStore(access.GetStoreName())))
	require.NoError(t, err)
	require.Len(t, results[0].BucketAccesses, 1)
	assert.Equal(t, access, results[0].BucketAccesses[0])
	assert.NotSame(t, access, results[0].BucketAccesses[0])
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ReadBucketAccess(accessID)))
	require.NoError(t, err)
	require.NotNil(t, results[0].BucketAccess.Read)
	assert.Equal(t, access, results[0].BucketAccess.Read)
	assert.NotSame(t, access, results[0].BucketAccess.Read)
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(InconsistentReadBucketAccess(accessID)))
	require.NoError(t, err)
	require.NotNil(t, results[0].BucketAccess.Read)
	assert.Equal(t, access.StoreName, results[0].BucketAccess.Read.StoreName)
	unlock()

	updatedAccess := access.SmartCopy().(*object.BucketAccess)
	_, results, unlock, err = Lock(context.Background(), Query(UpsertBucketAccess(accessID, "")))
	require.NoError(t, err)
	require.NotNil(t, results[0].BucketAccess.Read)
	results[0].BucketAccess.Upsert(updatedAccess)
	unlock()

	assert.Same(t, updatedAccess, bucketAccesses.data[accessID])

	_, results, unlock, err = Lock(context.Background(), Query(DeleteBucketAccess(accessID)))
	require.NoError(t, err)
	require.NotNil(t, results[0].BucketAccess.Read)
	results[0].BucketAccess.Delete()
	unlock()

	assert.NotContains(t, bucketAccesses.data, accessID)
}

func TestBucketAccessDependsOnObjectStore(t *testing.T) {
	Initialize()

	query := Query(UpsertBucketAccess("access-id-1", ""))
	resourceIndices, err := dedupe(query)
	require.NoError(t, err)
	query, roots := buildTrees(resourceIndices, query)
	require.Len(t, roots, 1)
	require.NoError(t, fillInIDs(roots[0], query))

	assert.NotEmpty(t, query[roots[0]].dependencies)
	assert.Len(t, query, 2)
}
