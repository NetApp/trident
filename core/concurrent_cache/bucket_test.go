package concurrent_cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/object"
)

func TestBucketSubqueries(t *testing.T) {
	Initialize()

	store := newTestObjectStore("store1", "store-id-1")
	objectStores.data[store.StoreUUID()] = store
	objectStores.key.data[store.GetUniqueKey()] = store.StoreUUID()

	otherStore := newTestObjectStore("store2", "store-id-2")
	objectStores.data[otherStore.StoreUUID()] = otherStore
	objectStores.key.data[otherStore.GetUniqueKey()] = otherStore.StoreUUID()

	cachedBucket := &object.Bucket{
		Name:      "bucket1",
		StoreName: store.GetUniqueKey(),
	}
	buckets.data[cachedBucket.Name] = cachedBucket

	otherBucket := &object.Bucket{
		Name:      "bucket2",
		StoreName: otherStore.GetUniqueKey(),
	}
	buckets.data[otherBucket.Name] = otherBucket

	_, results, unlock, err := Lock(context.Background(), Query(ListBuckets()))
	require.NoError(t, err)
	require.Len(t, results[0].Buckets, 2)
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ListBucketsForObjectStore(cachedBucket.GetObjectStoreID())))
	require.NoError(t, err)
	require.Len(t, results[0].Buckets, 1)
	assert.Equal(t, cachedBucket.Name, results[0].Buckets[0].Name)
	assert.NotSame(t, cachedBucket, results[0].Buckets[0])
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ReadBucket(cachedBucket.Name)))
	require.NoError(t, err)
	require.NotNil(t, results[0].Bucket.Read)
	assert.Equal(t, cachedBucket, results[0].Bucket.Read)
	assert.NotSame(t, cachedBucket, results[0].Bucket.Read)
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(InconsistentReadBucket(cachedBucket.Name)))
	require.NoError(t, err)
	require.NotNil(t, results[0].Bucket.Read)
	assert.Equal(t, cachedBucket.Name, results[0].Bucket.Read.Name)
	unlock()

	updatedBucket := cachedBucket.SmartCopy().(*object.Bucket)
	updatedBucket.StoreName = store.GetUniqueKey()
	_, results, unlock, err = Lock(
		context.Background(), Query(UpsertBucket(cachedBucket.Name, store.StoreUUID())),
	)
	require.NoError(t, err)
	require.NotNil(t, results[0].Bucket.Read)
	results[0].Bucket.Upsert(updatedBucket)
	unlock()

	assert.Same(t, updatedBucket, buckets.data[cachedBucket.Name])

	_, results, unlock, err = Lock(context.Background(), Query(DeleteBucket(cachedBucket.Name)))
	require.NoError(t, err)
	require.NotNil(t, results[0].Bucket.Read)
	results[0].Bucket.Delete()
	unlock()

	assert.NotContains(t, buckets.data, cachedBucket.Name)
}

func TestBucketDependencyFromCachedBucket(t *testing.T) {
	Initialize()

	store := newTestObjectStore("store1", "store-id-1")
	objectStores.data[store.StoreUUID()] = store
	objectStores.key.data[store.GetUniqueKey()] = store.StoreUUID()
	cachedBucket := &object.Bucket{
		Name:      "bucket-id-1",
		StoreName: store.GetUniqueKey(),
	}
	buckets.data[cachedBucket.Name] = cachedBucket

	query := Query(UpsertBucket(cachedBucket.Name, ""))
	resourceIndices, err := dedupe(query)
	require.NoError(t, err)
	query, roots := buildTrees(resourceIndices, query)
	require.Len(t, roots, 1)
	require.NoError(t, fillInIDs(0, query))
	require.Len(t, query[0].dependencies, 1)
	assert.Equal(t, objectStore, query[query[0].dependencies[0]].res)
	assert.Equal(t, store.StoreUUID(), query[query[0].dependencies[0]].id)
}

func TestUpsertBucketSetsObjectStoreDependency(t *testing.T) {
	Initialize()

	query := Query(UpsertBucket("bucket-id-1", "store-id-1"))
	resourceIndices, err := dedupe(query)
	require.NoError(t, err)
	query, roots := buildTrees(resourceIndices, query)
	require.Len(t, roots, 1)
	require.NoError(t, fillInIDs(roots[0], query))

	require.Len(t, query[roots[0]].dependencies, 1)
	storeQuery := query[query[roots[0]].dependencies[0]]
	assert.Equal(t, objectStore, storeQuery.res)
	assert.Equal(t, "store-id-1", storeQuery.id)
}
