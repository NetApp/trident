package concurrent_cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/object"
)

func TestObjectStoreSubqueries(t *testing.T) {
	Initialize()

	store := newTestObjectStore("store1", "store-id-1")
	objectStores.data[store.StoreUUID()] = store
	objectStores.key.data[store.GetUniqueKey()] = store.StoreUUID()

	_, results, unlock, err := Lock(context.Background(), Query(ListObjectStores()))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].ObjectStores, 1)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStores[0].StoreUUID())
	assert.NotSame(t, store, results[0].ObjectStores[0])
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(FilterObjectStoresForBucket("missing-bucket")))
	require.NoError(t, err)
	require.Len(t, results[0].ObjectStores, 1)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStores[0].StoreUUID())
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ReadObjectStore(store.StoreUUID())))
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStore.Read.StoreUUID())
	assert.NotSame(t, store, results[0].ObjectStore.Read)
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(ReadObjectStoreByName(store.GetUniqueKey())))
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStore.Read.StoreUUID())
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(InconsistentReadObjectStore(store.StoreUUID())))
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStore.Read.StoreUUID())
	unlock()

	_, results, unlock, err = Lock(context.Background(), Query(InconsistentReadObjectStoreByName(store.GetUniqueKey())))
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStore.Read.StoreUUID())
	unlock()

	renamedStore := newTestObjectStore("store2", store.StoreUUID())
	_, results, unlock, err = Lock(
		context.Background(), Query(UpsertObjectStore(store.StoreUUID(), store.GetUniqueKey(), renamedStore.GetUniqueKey())),
	)
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	results[0].ObjectStore.Upsert(renamedStore)
	unlock()

	assert.Same(t, renamedStore, objectStores.data[store.StoreUUID()])
	assert.NotContains(t, objectStores.key.data, store.GetUniqueKey())
	assert.Equal(t, store.StoreUUID(), objectStores.key.data[renamedStore.GetUniqueKey()])

	_, results, unlock, err = Lock(context.Background(), Query(DeleteObjectStore(store.StoreUUID())))
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	results[0].ObjectStore.Delete()
	unlock()

	assert.NotContains(t, objectStores.data, store.StoreUUID())
	assert.NotContains(t, objectStores.key.data, renamedStore.GetUniqueKey())
}

func TestFilterObjectStoresForBucket(t *testing.T) {
	Initialize()

	store1 := newTestObjectStore("store1", "store-id-1")
	store2 := newTestObjectStore("store2", "store-id-2")
	objectStores.data[store1.StoreUUID()] = store1
	objectStores.key.data[store1.GetUniqueKey()] = store1.StoreUUID()
	objectStores.data[store2.StoreUUID()] = store2
	objectStores.key.data[store2.GetUniqueKey()] = store2.StoreUUID()

	t.Run("unknown bucket returns all stores", func(t *testing.T) {
		_, results, unlock, err := Lock(context.Background(), Query(FilterObjectStoresForBucket("missing-bucket")))
		require.NoError(t, err)
		defer unlock()

		require.Len(t, results[0].ObjectStores, 2)
		got := objectStoreUUIDs(results[0].ObjectStores)
		assert.ElementsMatch(t, []string{store1.StoreUUID(), store2.StoreUUID()}, got)
		assert.NotContains(t, results[0].ObjectStores, nil)
		assert.NotSame(t, store1, findObjectStore(results[0].ObjectStores, store1.StoreUUID()))
	})

	t.Run("preferred store is first and not duplicated", func(t *testing.T) {
		cachedBucket := &object.Bucket{Name: "bucket1", StoreUUID: store1.StoreUUID()}
		buckets.data[cachedBucket.Name] = cachedBucket

		_, results, unlock, err := Lock(context.Background(), Query(FilterObjectStoresForBucket(cachedBucket.Name)))
		require.NoError(t, err)
		defer unlock()

		require.Len(t, results[0].ObjectStores, 2)
		require.NotNil(t, results[0].ObjectStores[0])
		assert.Equal(t, store1.StoreUUID(), results[0].ObjectStores[0].StoreUUID())
		assert.Equal(t, store2.StoreUUID(), results[0].ObjectStores[1].StoreUUID())
		assert.NotSame(t, store1, results[0].ObjectStores[0])
	})

	t.Run("empty store UUID has no preferred store and no placeholder", func(t *testing.T) {
		cachedBucket := &object.Bucket{Name: "bucket-unplaced"}
		buckets.data[cachedBucket.Name] = cachedBucket

		_, results, unlock, err := Lock(context.Background(), Query(FilterObjectStoresForBucket(cachedBucket.Name)))
		require.NoError(t, err)
		defer unlock()

		require.Len(t, results[0].ObjectStores, 2)
		assert.NotContains(t, results[0].ObjectStores, nil)
		assert.ElementsMatch(t, []string{store1.StoreUUID(), store2.StoreUUID()}, objectStoreUUIDs(results[0].ObjectStores))
	})

	t.Run("unknown store UUID prepends a nil placeholder", func(t *testing.T) {
		cachedBucket := &object.Bucket{Name: "bucket-orphan", StoreUUID: "missing-store"}
		buckets.data[cachedBucket.Name] = cachedBucket

		_, results, unlock, err := Lock(context.Background(), Query(FilterObjectStoresForBucket(cachedBucket.Name)))
		require.NoError(t, err)
		defer unlock()

		require.Len(t, results[0].ObjectStores, 3)
		assert.Nil(t, results[0].ObjectStores[0])
		got := objectStoreUUIDs(results[0].ObjectStores[1:])
		assert.ElementsMatch(t, []string{store1.StoreUUID(), store2.StoreUUID()}, got)
	})
}

func objectStoreUUIDs(stores []object.Store) []string {
	uuids := make([]string, 0, len(stores))
	for _, store := range stores {
		if store != nil {
			uuids = append(uuids, store.StoreUUID())
		}
	}
	return uuids
}

func findObjectStore(stores []object.Store, uuid string) object.Store {
	for _, store := range stores {
		if store != nil && store.StoreUUID() == uuid {
			return store
		}
	}
	return nil
}

func TestDeleteObjectStoreByName(t *testing.T) {
	Initialize()

	store := newTestObjectStore("store1", "store-id-1")
	objectStores.data[store.StoreUUID()] = store
	objectStores.key.data[store.GetUniqueKey()] = store.StoreUUID()

	_, results, unlock, err := Lock(context.Background(), Query(DeleteObjectStore(store.GetUniqueKey())))
	require.NoError(t, err)
	assert.Nil(t, results[0].ObjectStore.Read)
	unlock()
	assert.Contains(t, objectStores.data, store.StoreUUID())

	_, results, unlock, err = Lock(context.Background(), Query(DeleteObjectStoreByName(store.GetUniqueKey())))
	require.NoError(t, err)
	require.NotNil(t, results[0].ObjectStore.Read)
	assert.Equal(t, store.StoreUUID(), results[0].ObjectStore.Read.StoreUUID())
	results[0].ObjectStore.Delete()
	unlock()

	assert.NotContains(t, objectStores.data, store.StoreUUID())
	assert.NotContains(t, objectStores.key.data, store.GetUniqueKey())
}

func TestUpsertObjectStoreUsesStoreID(t *testing.T) {
	Initialize()

	store := newTestObjectStore("store1", "store-id-1")
	_, results, unlock, err := Lock(context.Background(), Query(UpsertObjectStore("", "", store.GetUniqueKey())))
	require.NoError(t, err)
	results[0].ObjectStore.Upsert(store)
	unlock()

	assert.Same(t, store, objectStores.data[store.StoreUUID()])
	assert.Equal(t, store.StoreUUID(), objectStores.key.data[store.GetUniqueKey()])
}
