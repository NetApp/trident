// Copyright 2026 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	db "github.com/netapp/trident/core/concurrent_cache"
	"github.com/netapp/trident/object"
	"github.com/netapp/trident/object/drivers/fake"
	"github.com/netapp/trident/object/drivers/ontaps3"
	"github.com/netapp/trident/object/drivers/storagegrid"
	objectfactory "github.com/netapp/trident/object/factory"
	persistentstore "github.com/netapp/trident/persistent_store"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

func fakeObjectStoreConfig(name string) []byte {
	config, _ := json.Marshal(map[string]string{
		"objectStoreDriver": "fake",
		"objectStoreName":   name,
		"endpoint":          "https://localhost",
	})
	return config
}

func fakeObjectStoreSecretConfig() []byte {
	config, _ := json.Marshal(map[string]string{
		"user":     "alice",
		"password": "secret",
	})
	return config
}

// sgObjectStoreConfig/sgObjectStoreSecretConfig and ontaps3ObjectStoreConfig/
// ontaps3ObjectStoreSecretConfig build real-driver configs so
// TestConcurrentObjectStoreLifecycle can exercise every registered object-store driver
// through the same code path, not just "fake". Leave credentials unset (or point at an
// unreachable endpoint) to make the store's Initialize fail fast; callers skip the subtest
// in that case.
//
//	TRIDENT_TEST_SG_MGMT_ENDPOINT, TRIDENT_TEST_SG_USERNAME, TRIDENT_TEST_SG_PASSWORD,
//	TRIDENT_TEST_SG_ACCOUNT_ID
func sgObjectStoreConfig(name string) []byte {
	config, _ := json.Marshal(map[string]string{
		"objectStoreDriver":  storagegrid.DriverName,
		"objectStoreName":    name,
		"managementEndpoint": "https://webscalenextadm.netapp.com",
		"accountId":          "45275702274032441235",
	})
	return config
}

func sgObjectStoreSecretConfig() []byte {
	config, _ := json.Marshal(map[string]string{
		"username": "root",
		"password": "_trident123_",
	})
	return config
}

func ontaps3ObjectStoreConfig(name string) []byte {
	config, _ := json.Marshal(map[string]string{
		"objectStoreDriver":  ontaps3.DriverName,
		"objectStoreName":    name,
		"managementEndpoint": "10.10.10.1",
		"svmName":            "svmName",
		"storagePrefix":      "test_",
	})
	return config
}

func ontaps3ObjectStoreSecretConfig() []byte {
	config, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "<REDACTED>",
	})
	return config
}

// driverToConfigurator maps a registered driver name to a function returning that
// driver's (config, secretConfig) pair, letting TestConcurrentObjectStoreLifecycle
// loop over every supported object-store driver.
var driverToConfigurator = map[string]func(name string) (config, secretConfig []byte){
	"fake": func(name string) ([]byte, []byte) {
		return fakeObjectStoreConfig(name), fakeObjectStoreSecretConfig()
	},
	storagegrid.DriverName: func(name string) ([]byte, []byte) {
		return sgObjectStoreConfig(name), sgObjectStoreSecretConfig()
	},
	ontaps3.DriverName: func(name string) ([]byte, []byte) {
		return ontaps3ObjectStoreConfig(name), ontaps3ObjectStoreSecretConfig()
	},
}

func addOrUpdateFakeObjectStore(
	o *ConcurrentTridentOrchestrator, config, secretConfig []byte,
) (object.Store, error) {
	store, err := objectfactory.CreateStore(testCtx, "fake", secretConfig, config)
	if err != nil {
		return nil, err
	}
	return o.AddOrUpdateObjectStore(testCtx, store)
}

func setupObjectStoreOrchestrator(t *testing.T) (
	*ConcurrentTridentOrchestrator, func(string) string, *k8sclient.Clients,
) {
	t.Helper()
	db.Initialize()

	var (
		storeClient persistentstore.Client = inMemoryPersistence
		clients     *k8sclient.Clients
	)
	switch strings.ToUpper(os.Getenv("TRIDENT_TEST_PERSISTENCE")) {
	case "", "MEMORY":
	case "CRD":
		kubeconfig := os.Getenv("TRIDENT_TEST_KUBECONFIG")
		require.NotEmpty(t, kubeconfig, "TRIDENT_TEST_KUBECONFIG is required for CRD persistence tests")
		var err error
		storeClient, err = persistentstore.NewCRDClientV1("", kubeconfig)
		require.NoError(t, err)
		clients, err = k8sclient.CreateK8SClients("", kubeconfig, "")
		require.NoError(t, err)
		_, err = clients.TridentClient.TridentV1().TridentObjectStores(clients.Namespace).List(testCtx, metav1.ListOptions{})
		require.NoError(t, err, "TridentObjectStore CRD is not available")
		_, err = clients.TridentClient.TridentV1().TridentBuckets(clients.Namespace).List(testCtx, metav1.ListOptions{})
		require.NoError(t, err, "TridentBucket CRD is not available")
		_, err = clients.TridentClient.TridentV1().TridentBucketAccesses(clients.Namespace).List(testCtx, metav1.ListOptions{})
		require.NoError(t, err, "TridentBucketAccess CRD is not available")
	default:
		t.Fatalf("unsupported TRIDENT_TEST_PERSISTENCE value %q", os.Getenv("TRIDENT_TEST_PERSISTENCE"))
	}

	orchestrator, err := NewConcurrentTridentOrchestrator(storeClient)
	require.NoError(t, err)
	o := orchestrator.(*ConcurrentTridentOrchestrator)
	o.bootstrapped = true
	o.bootstrapError = nil

	prefix := fmt.Sprintf("cosi-%s-", uuid.NewString()[:8])
	t.Cleanup(func() {
		if clients == nil {
			persistenceCleanup(t, o)
			return
		}
		o.bgTasks.stop()
		cleanupCOSIResources(t, clients, prefix)
	})
	return o, func(name string) string { return prefix + name }, clients
}

func cleanupCOSIResources(t *testing.T, clients *k8sclient.Clients, prefix string) {
	t.Helper()
	api := clients.TridentClient.TridentV1()
	if accesses, err := api.TridentBucketAccesses(clients.Namespace).List(testCtx, metav1.ListOptions{}); err != nil {
		t.Errorf("unable to list bucket accesses during cleanup: %v", err)
	} else {
		for _, access := range accesses.Items {
			if strings.HasPrefix(access.Name, prefix) {
				if err := api.TridentBucketAccesses(clients.Namespace).Delete(
					testCtx, access.Name, metav1.DeleteOptions{},
				); err != nil {
					t.Errorf("unable to delete bucket access %s: %v", access.Name, err)
				}
			}
		}
	}
	if buckets, err := api.TridentBuckets(clients.Namespace).List(testCtx, metav1.ListOptions{}); err != nil {
		t.Errorf("unable to list buckets during cleanup: %v", err)
	} else {
		for _, bucket := range buckets.Items {
			if strings.HasPrefix(bucket.Name, prefix) {
				if err := api.TridentBuckets(clients.Namespace).Delete(
					testCtx, bucket.Name, metav1.DeleteOptions{},
				); err != nil {
					t.Errorf("unable to delete bucket %s: %v", bucket.Name, err)
				}
			}
		}
	}
	if stores, err := api.TridentObjectStores(clients.Namespace).List(testCtx, metav1.ListOptions{}); err != nil {
		t.Errorf("unable to list object stores during cleanup: %v", err)
	} else {
		for _, store := range stores.Items {
			if !strings.HasPrefix(store.Name, prefix) {
				continue
			}
			if store.HasTridentFinalizers() {
				copy := store.DeepCopy()
				copy.RemoveTridentFinalizers()
				if _, err := api.TridentObjectStores(clients.Namespace).Update(
					testCtx, copy, metav1.UpdateOptions{},
				); err != nil {
					t.Errorf("unable to remove object store finalizers from %s: %v", store.Name, err)
				}
			}
			if err := api.TridentObjectStores(clients.Namespace).Delete(
				testCtx, store.Name, metav1.DeleteOptions{},
			); err != nil {
				t.Errorf("unable to delete object store %s: %v", store.Name, err)
			}
		}
	}
	secretList, err := clients.KubeClient.CoreV1().Secrets(clients.Namespace).List(testCtx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("unable to list secrets during cleanup: %v", err)
	} else {
		for _, secret := range secretList.Items {
			if strings.HasPrefix(secret.Name, prefix) {
				if err := clients.K8SClient.DeleteSecret(secret.Name, secret.Namespace); err != nil {
					t.Errorf("unable to delete leftover secret %s: %v", secret.Name, err)
				}
			}
		}
	}
}

func TestConcurrentObjectStoreLifecycle(t *testing.T) {
	// Intentionally share the object store orchestrator.
	o, name, _ := setupObjectStoreOrchestrator(t)

	for driverName, configurator := range driverToConfigurator {
		storeName, bucketName := name(driverName), name(driverName+"-bucket")

		t.Run("Test_"+storeName, func(t *testing.T) {
			config, secretConfig := configurator(storeName)

			built, err := objectfactory.CreateStore(testCtx, driverName, secretConfig, config)
			require.NoError(t, err)
			store, err := o.AddOrUpdateObjectStore(testCtx, built)
			if err != nil {
				t.Skipf("skipping %s: store could not be initialized: %v", t.Name(), err)
			}
			require.NotNil(t, store)

			assert.Equal(t, storeName, store.Name())
			assert.NotEmpty(t, store.StoreUUID())

			// Re-adding the same store is an idempotent upsert, not a conflict.
			replayBuilt, err := objectfactory.CreateStore(testCtx, driverName, secretConfig, config)
			require.NoError(t, err)
			replay, err := o.AddOrUpdateObjectStore(testCtx, replayBuilt)
			require.NoError(t, err)
			assert.Equal(t, store.StoreUUID(), replay.StoreUUID())

			bucket, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
			require.NoError(t, err)
			require.NotNil(t, bucket)
			assert.Equal(t, bucketName, bucket.Name)
			assert.Equal(t, store.StoreUUID(), bucket.StoreUUID)

			got, err := replay.GetBucketByName(testCtx, bucketName)
			require.NoError(t, err)
			assert.Equal(t, bucketName, got.Name)

			err = o.DeleteObjectStore(testCtx, storeName)
			assert.True(t, errors.IsConflictError(err), err)
			deletingStore, err := o.GetObjectStore(testCtx, storeName)
			require.NoError(t, err)
			assert.Equal(t, object.StoreStateDeleting, deletingStore.State())

			require.NoError(t, o.DeleteBucket(testCtx, bucketName))
			_, err = replay.GetBucketByName(testCtx, bucketName)
			assert.True(t, errors.IsNotFoundError(err))

			require.NoError(t, o.DeleteBucket(testCtx, name("missing-bucket")))

			require.NoError(t, o.DeleteObjectStore(testCtx, storeName))
		})
	}
}

func TestConcurrentObjectStoreDeleteAfterRestartStillConflicts(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)

	err = o.DeleteObjectStore(testCtx, storeName)
	assert.True(t, errors.IsConflictError(err), err)

	// Restart loads the terminating store with its buckets, so delete still conflicts.
	db.Initialize()
	o.bootstrapped = false
	require.NoError(t, o.bootstrapObjectStores(testCtx))
	require.NoError(t, o.bootstrapBuckets(testCtx))

	err = o.DeleteObjectStore(testCtx, storeName)
	assert.True(t, errors.IsConflictError(err), err)
	assert.ErrorContains(t, err, "has buckets")

	bucket, err := o.GetBucket(testCtx, bucketName)
	require.NoError(t, err)
	assert.Equal(t, store.StoreUUID(), bucket.StoreUUID)
}

func TestListObjectStoresReturnsBootstrapError(t *testing.T) {
	o, _, _ := setupObjectStoreOrchestrator(t)
	o.bootstrapError = errors.NotReadyError()

	stores, err := o.ListObjectStores(testCtx)
	assert.Nil(t, stores)
	assert.True(t, errors.IsNotReadyError(err), err)
}

func TestConcurrentObjectStoreGetAndList(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeA, storeB := name("store-a"), name("store-b")

	stores, err := o.ListObjectStores(testCtx)
	require.NoError(t, err)
	assert.Empty(t, stores)

	_, err = o.GetObjectStore(testCtx, storeA)
	assert.True(t, errors.IsNotFoundError(err), err)

	_, err = addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeA), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	_, err = addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeB), fakeObjectStoreSecretConfig())
	require.NoError(t, err)

	got, err := o.GetObjectStore(testCtx, storeA)
	require.NoError(t, err)
	assert.Equal(t, storeA, got.Name())

	stores, err = o.ListObjectStores(testCtx)
	require.NoError(t, err)
	names := make([]string, 0, len(stores))
	for _, store := range stores {
		names = append(names, store.Name())
	}
	assert.ElementsMatch(t, []string{storeA, storeB}, names)

	require.NoError(t, o.DeleteObjectStore(testCtx, storeA))
	_, err = o.GetObjectStore(testCtx, storeA)
	assert.True(t, errors.IsNotFoundError(err), err)
}

func TestConcurrentObjectStoreUpdatePreservesUUID(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName := name("store-a")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	originalUUID := store.StoreUUID()

	updatedConfig, err := json.Marshal(map[string]string{
		"objectStoreDriver": "fake",
		"objectStoreName":   storeName,
		"endpoint":          "https://updated.example.com",
	})
	require.NoError(t, err)

	updated, err := addOrUpdateFakeObjectStore(o, updatedConfig, fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	assert.Equal(t, originalUUID, updated.StoreUUID())
	fakeStore, ok := updated.(*fake.ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "https://updated.example.com", fakeStore.Endpoint)
}

// TestConcurrentObjectStoreFailedUpdateLeavesStoreOffline: a spec change stops the previous
// instance before the replacement is initialized. If that initialization fails, the cached
// store is offline and buckets are not placed on it.
func TestConcurrentObjectStoreFailedUpdateLeavesStoreOffline(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	originalUUID := store.StoreUUID()

	badSecret, err := json.Marshal(map[string]string{"user": "alice"})
	require.NoError(t, err)

	updated, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), badSecret)
	require.Error(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, object.StoreStateOffline, updated.State())
	assert.Equal(t, originalUUID, updated.StoreUUID())

	cached, err := o.GetObjectStore(testCtx, storeName)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateOffline, cached.State())

	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	cachedFake, ok := cached.(*fake.ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, 0, cachedFake.Calls()["CreateBucket"], "an offline store must not be used to create buckets")
}

// TestConcurrentAddBucketPersistsInfo guards against a real regression: AddBucket used to call
// storeClient.UpsertBucket before the driver's CreateBucket populated Info, so the persisted
// copy never carried it - only the in-memory cache did, because the cache holds the same
// pointer CreateBucket later mutates. Checks the persisted copy directly via
// InMemoryClient.GetBucket, not the orchestrator's returned value, since that's exactly the
// distinction the bug hid behind. Memory-persistence only: inMemoryPersistence is not the
// backing store in TRIDENT_TEST_PERSISTENCE=CRD mode.
func TestConcurrentAddBucketPersistsInfo(t *testing.T) {
	if strings.ToUpper(os.Getenv("TRIDENT_TEST_PERSISTENCE")) == "CRD" {
		t.Skip("checks InMemoryClient's internal state directly; not applicable under CRD persistence")
	}

	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	_, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)

	created, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	require.NotNil(t, created.Info.S3, "sanity check: the driver should have populated Info")

	persisted, err := inMemoryPersistence.GetBucket(testCtx, bucketName)
	require.NoError(t, err, "bucket must be persisted, not just cached")
	require.NotNil(t, persisted.Info.S3, "persisted bucket must carry Info, not just the in-memory cache")
	assert.Equal(t, created.Info, persisted.Info)
}

// TestConcurrentAddBucketRetryIsIdempotent: retrying AddBucket for an already-completed bucket
// must not call the driver's CreateBucket again.
func TestConcurrentAddBucketRetryIsIdempotent(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	first, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	require.NotNil(t, first.Info.S3)
	assert.Equal(t, 1, fakeStore.Calls()["CreateBucket"])

	second, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	assert.Equal(t, first.Info, second.Info)
	assert.Equal(t, 1, fakeStore.Calls()["CreateBucket"], "retry of an already-completed bucket must not call CreateBucket again")
}

// TestConcurrentAddBucketResumesPartialAttempt: StoreUUID set but Info empty is a partial
// attempt (e.g. a crash mid-call), not a completed one - AddBucket must still call the driver.
func TestConcurrentAddBucketResumesPartialAttempt(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	// Simulate a crash: StoreUUID recorded, but CreateBucket never ran.
	_, results, unlock, err := db.Lock(testCtx, db.Query(db.UpsertBucket(bucketName, store.StoreUUID())))
	require.NoError(t, err)
	partial := results[0].Bucket.Read
	if partial == nil {
		partial = &object.Bucket{Name: bucketName}
	}
	partial.StoreUUID = store.StoreUUID()
	results[0].Bucket.Upsert(partial)
	unlock()
	require.Equal(t, 0, fakeStore.Calls()["CreateBucket"], "sanity check: driver must not have been called yet")

	resumed, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	require.NotNil(t, resumed.Info.S3, "partial attempt must resume, not short-circuit as already-done")
	assert.Equal(t, 1, fakeStore.Calls()["CreateBucket"])
}

// TestConcurrentAddBucketCreateConflictIsCleanedUp: a create conflict is the driver's to
// classify. AddBucket always asks the driver to delete afterwards. An idempotent delete (the
// bucket is not on this store) clears the placement and the create error is reported.
func TestConcurrentAddBucketCreateConflictIsCleanedUp(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	injected := errors.AlreadyExistsError("bucket name collides with an unrelated resource")
	fakeStore.SetError("CreateBucket", injected)

	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), injected.Error())
	assert.False(t, errors.IsAlreadyExistsError(err), "one store's conflict must not make the aggregate look terminal: %v", err)
	assert.Equal(t, 1, fakeStore.Calls()["DeleteBucket"], "a failed create is cleaned up by an idempotent delete")
	assert.NotContains(t, fakeStore.Buckets(), bucketName, "the conflicting bucket must not have been created here")

	cached, err := o.GetBucket(testCtx, bucketName)
	require.NoError(t, err)
	assert.Empty(t, cached.StoreUUID, "a successful cleanup must clear the store so another one can be tried")
}

// TestConcurrentAddBucketPartialFailureIsDeleted: some drivers (e.g. storage-grid, on a
// partial-configuration alert) report an error from CreateBucket even though the bucket now
// exists on the backend. AddBucket then calls DeleteBucket, and the driver removes the bucket
// it actually created so the next attempt starts clean.
func TestConcurrentAddBucketPartialFailureIsDeleted(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	// The bucket is genuinely created (and stored) but the driver still reports an error, the
	// same shape as a backend that returns a success alert alongside a partial-config failure.
	injected := fmt.Errorf("some sub-resource of the bucket failed to configure")
	fakeStore.SetErrorAfterCreate(injected)

	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), injected.Error())
	assert.Equal(t, 1, fakeStore.Calls()["DeleteBucket"], "a confirmed partial failure must be cleaned up, not resumed")
	assert.NotContains(t, fakeStore.Buckets(), bucketName, "the bucket must actually be gone after cleanup")

	// Clearing the injected error and retrying must create the bucket fresh and succeed - the
	// old one was deleted, so this is a brand new create, not a resumed partial attempt.
	fakeStore.SetErrorAfterCreate(nil)
	resumed, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	require.NotNil(t, resumed.Info.S3)
	assert.Equal(t, 2, fakeStore.Calls()["CreateBucket"], "the retry must create it again, since the old one was deleted")
}

// TestConcurrentAddBucketStopsWhenDeleteFails: if the driver's cleanup delete fails, AddBucket
// returns that error and leaves the store recorded. The driver is what decides a delete is
// unsafe (it could not tell whether the bucket exists, for example); core does not guess.
func TestConcurrentAddBucketStopsWhenDeleteFails(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	fakeStore.SetError("CreateBucket", fmt.Errorf("the store is unavailable"))
	deleteErr := fmt.Errorf("cannot tell whether the bucket exists")
	fakeStore.SetError("DeleteBucket", deleteErr)

	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	assert.ErrorIs(t, err, deleteErr, "a failed cleanup must surface, so the placement is not cleared: %v", err)
	assert.Equal(t, 1, fakeStore.Calls()["DeleteBucket"])

	cached, err := o.GetBucket(testCtx, bucketName)
	require.NoError(t, err)
	assert.Equal(t, store.StoreUUID(), cached.StoreUUID, "the store stays recorded so a later retry resumes here")
}

// TestConcurrentAddBucketInvalidInputIsCleanedUpOnEachStore: an invalid request fails create
// on every store. Delete is idempotent, so each store is cleaned up and the next one is tried.
// The aggregate stays a plain error: one store's invalid-input must not make the whole request
// look terminal.
func TestConcurrentAddBucketInvalidInputIsCleanedUpOnEachStore(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	bucketName := name("bucket-1")

	var fakeStores []*fake.ObjectDriver
	for _, storeName := range []string{name("store-a"), name("store-b")} {
		store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
		require.NoError(t, err)
		fakeStore, ok := store.(*fake.ObjectDriver)
		require.True(t, ok)
		fakeStore.SetError("CreateBucket", errors.InvalidInputError("invalid bucket name %s", bucketName))
		fakeStores = append(fakeStores, fakeStore)
	}

	_, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bucket name")
	assert.False(t, errors.IsInvalidInputError(err), "one store's error type must not leak into the aggregate: %v", err)

	createCalls, deleteCalls := 0, 0
	for _, fakeStore := range fakeStores {
		createCalls += fakeStore.Calls()["CreateBucket"]
		deleteCalls += fakeStore.Calls()["DeleteBucket"]
	}
	assert.Equal(t, 2, createCalls, "each store is asked, since cleanup succeeded")
	assert.Equal(t, 2, deleteCalls, "each failed create is followed by an idempotent delete")
}

// upsertBucketFailer wraps a persistentstore.Client and fails a chosen call number to
// UpsertBucket, delegating every other call (including every other method) unchanged. Used to
// reproduce a persist failure that happens strictly after CreateBucket already succeeded.
type upsertBucketFailer struct {
	persistentstore.Client
	failOnCall int
	calls      int
	err        error
}

func (f *upsertBucketFailer) UpsertBucket(ctx context.Context, bucket *object.Bucket) error {
	f.calls++
	if f.calls == f.failOnCall {
		return f.err
	}
	return f.Client.UpsertBucket(ctx, bucket)
}

// TestConcurrentAddBucketRetriesWhenInfoPersistFails: AddBucket calls storeClient.UpsertBucket
// twice per successful create - once before CreateBucket (to persist StoreUUID) and once after
// (to persist Info). If that second call fails, AddBucket must fail the whole request rather
// than warning and returning success: the "already completed" short-circuit trusts the cache,
// so caching Info without a confirmed persist would mean this failure is never retried, and the
// persisted record would silently lack Info forever.
func TestConcurrentAddBucketRetriesWhenInfoPersistFails(t *testing.T) {
	if strings.ToUpper(os.Getenv("TRIDENT_TEST_PERSISTENCE")) == "CRD" {
		t.Skip("checks InMemoryClient's internal state directly; not applicable under CRD persistence")
	}

	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	injected := fmt.Errorf("api server unavailable")
	o.storeClient = &upsertBucketFailer{Client: o.storeClient, failOnCall: 2, err: injected}

	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	assert.ErrorIs(t, err, injected, "a failed Info persist must fail the request, not warn and succeed")
	assert.Equal(t, 1, fakeStore.Calls()["CreateBucket"])

	// The pre-CreateBucket upsert (StoreUUID only) already succeeded; only the second one -
	// the one that would have added Info - was made to fail.
	persistedBeforeRetry, persistErr := inMemoryPersistence.GetBucket(testCtx, bucketName)
	require.NoError(t, persistErr)
	assert.Equal(t, store.StoreUUID(), persistedBeforeRetry.StoreUUID)
	assert.Empty(t, persistedBeforeRetry.Info, "Info must not be persisted since the second upsert failed")

	// Retry: CreateBucket is idempotent, so a second attempt must succeed and persist correctly.
	resumed, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	require.NotNil(t, resumed.Info.S3)
	assert.Equal(t, 2, fakeStore.Calls()["CreateBucket"], "the retry must call the driver again, not skip it as already-completed")

	persisted, err := inMemoryPersistence.GetBucket(testCtx, bucketName)
	require.NoError(t, err, "the retry must persist Info this time")
	assert.Equal(t, resumed.Info, persisted.Info)
}

// TestConcurrentAddBucketRetriesAfterTransientFailure: a failed create on the only store leaves
// the bucket cached with no store. The next call must retry it, not panic on a missing store.
func TestConcurrentAddBucketRetriesAfterTransientFailure(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	fakeStore.SetError("CreateBucket", fmt.Errorf("503 service unavailable"))
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	fakeStore.SetError("CreateBucket", nil)

	var retried *object.Bucket
	require.NotPanics(t, func() {
		retried, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	})
	require.NoError(t, err)
	assert.Equal(t, store.StoreUUID(), retried.StoreUUID)
	require.NotNil(t, retried.Info.S3)
	assert.Equal(t, 2, fakeStore.Calls()["CreateBucket"])
}

// TestConcurrentAddBucketPersistsClearedStoreName: when a failed create is cleaned up, the
// persisted TridentBucket must drop the store UUID too, not just the in-memory cache.
// Memory-persistence only, for the same reason as TestConcurrentAddBucketPersistsInfo.
func TestConcurrentAddBucketPersistsClearedStoreName(t *testing.T) {
	if strings.ToUpper(os.Getenv("TRIDENT_TEST_PERSISTENCE")) == "CRD" {
		t.Skip("checks InMemoryClient's internal state directly; not applicable under CRD persistence")
	}

	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	fakeStore.SetError("CreateBucket", fmt.Errorf("503 service unavailable"))
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	assert.Equal(t, 1, fakeStore.Calls()["DeleteBucket"], "a failed create is cleaned up by an idempotent delete")

	persisted, err := inMemoryPersistence.GetBucket(testCtx, bucketName)
	require.NoError(t, err)
	assert.Empty(t, persisted.StoreUUID, "persisted bucket must match the cache after cleanup")
}

// TestConcurrentAddBucketAllStoresFailedReportsEachCause: when no store takes the bucket, the
// error must say why for each store that actually failed, but must stay a plain retryable
// error: one store's not-found must not make the whole request look like a terminal not-found.
// A store skipped for not being Online is not a failure - it's already explained by the debug
// log AddBucket emits for it - so it must not appear in the aggregate at all.
func TestConcurrentAddBucketAllStoresFailedReportsEachCause(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	bucketName := name("bucket-1")

	fakeStores := map[string]*fake.ObjectDriver{}
	for _, storeName := range []string{name("store-a"), name("store-b"), name("store-c")} {
		store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
		require.NoError(t, err)
		fakeStore, ok := store.(*fake.ObjectDriver)
		require.True(t, ok)
		fakeStores[storeName] = fakeStore
	}
	fakeStores[name("store-a")].SetError("CreateBucket", errors.NotFoundError("tenant quota object missing"))
	fakeStores[name("store-b")].SetError("CreateBucket", fmt.Errorf("503 service unavailable"))
	fakeStores[name("store-c")].SetState(object.StoreStateDeleting)

	_, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to create bucket")
	assert.Contains(t, err.Error(), name("store-a")+": tenant quota object missing")
	assert.Contains(t, err.Error(), name("store-b")+": 503 service unavailable")
	assert.NotContains(t, err.Error(), name("store-c"), "a store skipped for not being Online is not a failure and must not appear in the aggregate")
	assert.False(t, errors.IsNotFoundError(err), "a single store's error type must not leak into the aggregate")
}

// TestConcurrentDeleteBucketNeverPlaced: a bucket every store failed to create is cached with no
// store. Deleting it must remove Trident's record without calling any driver, not fail forever
// looking for a store that was never chosen.
func TestConcurrentDeleteBucketNeverPlaced(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)

	fakeStore.SetError("CreateBucket", fmt.Errorf("503 service unavailable"))
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.Error(t, err)
	cached, err := o.GetBucket(testCtx, bucketName)
	require.NoError(t, err, "sanity check: the failed bucket must still be cached")
	require.Empty(t, cached.StoreUUID)
	deletesBefore := fakeStore.Calls()["DeleteBucket"]

	require.NoError(t, o.DeleteBucket(testCtx, bucketName))
	_, err = o.GetBucket(testCtx, bucketName)
	assert.True(t, errors.IsNotFoundError(err), "bucket must be gone from the cache: %v", err)
	assert.Equal(t, deletesBefore, fakeStore.Calls()["DeleteBucket"], "nothing was placed, so no driver delete")
	if strings.ToUpper(os.Getenv("TRIDENT_TEST_PERSISTENCE")) != "CRD" {
		_, err = inMemoryPersistence.GetBucket(testCtx, bucketName)
		assert.Error(t, err, "the persisted record must be deleted too")
	}
}

// TestConcurrentDeleteBucketNotEmptyKeepsBucket: a driver refusing to delete a non-empty bucket
// must surface as a bucket-not-empty error (COSI retries it), and the bucket must stay tracked.
func TestConcurrentDeleteBucketNotEmptyKeepsBucket(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)

	fakeStore.SetError("DeleteBucket", errors.BucketNotEmptyError("bucket %s is not empty", bucketName))
	err = o.DeleteBucket(testCtx, bucketName)
	assert.True(t, errors.IsBucketNotEmptyError(err), "not-empty must reach the caller as-is: %v", err)
	_, err = o.GetBucket(testCtx, bucketName)
	assert.NoError(t, err, "a bucket that wasn't deleted must stay tracked")

	fakeStore.SetError("DeleteBucket", nil)
	require.NoError(t, o.DeleteBucket(testCtx, bucketName))
}

// cancelAfterFirstCheckCtx reports no error on its first Err() call and context.Canceled after
// that, so a caller's first db.Lock succeeds and every later one sees a cancelled context.
type cancelAfterFirstCheckCtx struct {
	context.Context
	checks atomic.Int32
}

func (c *cancelAfterFirstCheckCtx) Err() error {
	if c.checks.Add(1) > 1 {
		return context.Canceled
	}
	return nil
}

// TestConcurrentAddBucketCancelledMidLoopReleasesLocks: a context cancelled after AddBucket's
// first lock but before the per-store lock must return an error, not panic, and must release
// the bucket and store locks so a later call for the same bucket can complete.
func TestConcurrentAddBucketCancelledMidLoopReleasesLocks(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	_, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)

	ctx := &cancelAfterFirstCheckCtx{Context: context.Background()}
	assert.NotPanics(t, func() {
		_, err = o.AddBucket(ctx, &object.Bucket{Name: bucketName}, nil)
	})
	assert.ErrorIs(t, err, context.Canceled)

	done := make(chan error, 1)
	go func() {
		_, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
		done <- err
	}()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("AddBucket hung: locks from the cancelled call were never released")
	}
}

// TestConcurrentAddBucketCompletedOnDeletingStoreIsNotRecreated: COSI periodically calls
// CreateBucket for buckets that already exist. If the bucket's store has gone Deleting (a
// refused DeleteObjectStore does that), AddBucket must return the existing bucket, not create
// it again on another store and lose track of the original.
func TestConcurrentAddBucketCompletedOnDeletingStoreIsNotRecreated(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeA, bucketName := name("store-a"), name("bucket-1")

	sa, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeA), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeA, ok := sa.(*fake.ObjectDriver)
	require.True(t, ok)
	first, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	require.Equal(t, sa.StoreUUID(), first.StoreUUID)

	err = o.DeleteObjectStore(testCtx, storeA)
	require.True(t, errors.IsConflictError(err), err)
	deletingStore, err := o.GetObjectStore(testCtx, storeA)
	require.NoError(t, err)
	require.Equal(t, object.StoreStateDeleting, deletingStore.State())

	sb, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(name("store-b")), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	fakeB, ok := sb.(*fake.ObjectDriver)
	require.True(t, ok)

	again, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)
	assert.Equal(t, sa.StoreUUID(), again.StoreUUID)
	assert.Equal(t, first.Info, again.Info)
	assert.Equal(t, 1, fakeA.Calls()["CreateBucket"])
	assert.Equal(t, 0, fakeB.Calls()["CreateBucket"], "a completed bucket must not be created again on another store")
	assert.Empty(t, fakeB.Buckets())
}

func TestConcurrentGetBucket(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName := name("store-a"), name("bucket-1")

	_, err := o.GetBucket(testCtx, bucketName)
	assert.True(t, errors.IsNotFoundError(err), err)

	_, err = addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	created, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, nil)
	require.NoError(t, err)

	got, err := o.GetBucket(testCtx, bucketName)
	require.NoError(t, err)
	assert.Equal(t, created.Name, got.Name)
	assert.Equal(t, created.StoreUUID, got.StoreUUID)
	// Info must round-trip through the cache unchanged.
	assert.Equal(t, created.Info, got.Info)
	require.NotNil(t, got.Info.S3)
	assert.NotEmpty(t, got.Info.S3.Endpoint)

	require.NoError(t, o.DeleteBucket(testCtx, bucketName))
	_, err = o.GetBucket(testCtx, bucketName)
	assert.True(t, errors.IsNotFoundError(err), err)

	require.NoError(t, o.DeleteObjectStore(testCtx, storeName))
}

func TestConcurrentObjectStoreBootstrap(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName := name("store-a")

	persistentStore, err := fake.NewObjectDriver(testCtx, fakeObjectStoreSecretConfig(), fakeObjectStoreConfig(storeName))
	require.NoError(t, err)
	require.NoError(t, persistentStore.Initialize(testCtx))
	require.NoError(t, o.storeClient.AddObjectStore(testCtx, persistentStore))

	db.Initialize()
	o.bootstrapped = false
	require.NoError(t, o.bootstrapObjectStores(testCtx))

	listed, err := o.storeClient.GetObjectStores(testCtx)
	require.NoError(t, err)
	var found object.Store
	for _, store := range listed {
		if store.Name() == storeName {
			found = store
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, persistentStore.StoreUUID(), found.StoreUUID())

	bootstrapped, err := o.GetObjectStore(testCtx, storeName)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateOnline, bootstrapped.State())

	replay, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	assert.Equal(t, persistentStore.StoreUUID(), replay.StoreUUID())
}

func TestConcurrentObjectStoreBootstrapKeepsDeletingState(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName := name("store-a")

	persistentStore, err := fake.NewObjectDriver(testCtx, fakeObjectStoreSecretConfig(), fakeObjectStoreConfig(storeName))
	require.NoError(t, err)
	require.NoError(t, persistentStore.Initialize(testCtx))
	persistentStore.SetState(object.StoreStateDeleting)
	require.NoError(t, o.storeClient.AddObjectStore(testCtx, persistentStore))

	db.Initialize()
	o.bootstrapped = false
	require.NoError(t, o.bootstrapObjectStores(testCtx))

	got, err := o.GetObjectStore(testCtx, storeName)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateDeleting, got.State())
	assert.Equal(t, persistentStore.StoreUUID(), got.StoreUUID())

	persisted, err := o.storeClient.GetObjectStore(testCtx, storeName)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateDeleting, persisted.State())
}

func TestConcurrentObjectStoreBootstrapKeepsDeletingStateWhenInitializeFails(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName := name("store-a")

	persistentStore, err := fake.NewObjectDriver(testCtx, fakeObjectStoreSecretConfig(), fakeObjectStoreConfig(storeName))
	require.NoError(t, err)
	require.NoError(t, persistentStore.Initialize(testCtx))
	persistentStore.SetState(object.StoreStateDeleting)
	persistentStore.(*fake.ObjectDriver).Password = ""
	require.NoError(t, o.storeClient.AddObjectStore(testCtx, persistentStore))

	db.Initialize()
	o.bootstrapped = false
	require.NoError(t, o.bootstrapObjectStores(testCtx))

	got, err := o.GetObjectStore(testCtx, storeName)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateDeleting, got.State())
	assert.Equal(t, persistentStore.StoreUUID(), got.StoreUUID())
}

func TestConcurrentBucketBootstrap(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName, bucketName, accessName := name("store-a"), name("bucket-1"), name("access-1")

	persistentStore, err := fake.NewObjectDriver(testCtx, fakeObjectStoreSecretConfig(), fakeObjectStoreConfig(storeName))
	require.NoError(t, err)
	require.NoError(t, persistentStore.Initialize(testCtx))
	require.NoError(t, o.storeClient.AddObjectStore(testCtx, persistentStore))

	bucket := &object.Bucket{Name: bucketName, StoreUUID: persistentStore.StoreUUID()}
	require.NoError(t, o.storeClient.UpsertBucket(testCtx, bucket))
	access := &object.BucketAccess{Name: accessName, StoreUUID: persistentStore.StoreUUID()}
	require.NoError(t, o.storeClient.UpsertBucketAccess(testCtx, access))

	db.Initialize()
	o.bootstrapped = false
	require.NoError(t, o.bootstrapObjectStores(testCtx))
	require.NoError(t, o.bootstrapBuckets(testCtx))
	require.NoError(t, o.bootstrapBucketAccesses(testCtx))

	_, results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadBucket(bucketName)))
	require.NoError(t, err)
	require.NotNil(t, results[0].Bucket.Read)
	assert.Equal(t, persistentStore.StoreUUID(), results[0].Bucket.Read.StoreUUID)
	unlocker()

	_, results, unlocker, err = db.Lock(testCtx, db.Query(db.ReadBucketAccess(accessName)))
	require.NoError(t, err)
	require.NotNil(t, results[0].BucketAccess.Read)
	assert.Equal(t, persistentStore.StoreUUID(), results[0].BucketAccess.Read.StoreUUID)
	unlocker()
}

func TestConcurrentObjectStoreKubernetesCredentials(t *testing.T) {
	o, name, clients := setupObjectStoreOrchestrator(t)
	if clients == nil {
		t.Skip("requires TRIDENT_TEST_PERSISTENCE=CRD")
	}

	storeName := name("store-k8s")
	secretName := storeName + "-secret"
	_, err := clients.K8SClient.CreateSecret(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: clients.Namespace},
		StringData: map[string]string{
			"user":     "admin",
			"password": "somepass",
		},
	})
	require.NoError(t, err)

	spec, err := json.Marshal(map[string]interface{}{
		"objectStoreDriver": "fake",
		"endpoint":          "https://example.com",
		"credentials":       map[string]string{"secretName": secretName},
	})
	require.NoError(t, err)
	tos := &netappv1.TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: storeName, Namespace: clients.Namespace},
		Spec: netappv1.TridentObjectStoreSpec{
			RawExtension: runtime.RawExtension{Raw: spec},
		},
	}
	created, err := clients.TridentClient.TridentV1().TridentObjectStores(clients.Namespace).Create(
		testCtx, tos, metav1.CreateOptions{},
	)
	require.NoError(t, err)

	secret, err := clients.K8SClient.GetSecret(secretName)
	require.NoError(t, err)
	secretConfig, err := netappv1.SecretConfig(secret.Data)
	require.NoError(t, err)

	persistentStore, err := created.Persistent(testCtx, secretConfig)
	require.NoError(t, err)
	store, err := o.AddOrUpdateObjectStore(testCtx, persistentStore)
	require.NoError(t, err)
	fakeStore, ok := store.(*fake.ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "admin", fakeStore.User)
	assert.Equal(t, "somepass", fakeStore.Password)
	assert.Equal(t, storeName, fakeStore.Name())

	adopted, err := clients.TridentClient.TridentV1().TridentObjectStores(clients.Namespace).Get(
		testCtx, storeName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.JSONEq(t, string(spec), string(adopted.Spec.Raw))
	assert.Contains(t, string(adopted.Spec.Raw), `"secretName":"`+secretName+`"`)
	assert.True(t, adopted.HasTridentFinalizers())
	assert.Equal(t, string(object.StoreStateOnline), objectStoreStatusString(t, adopted.Status.Raw, "state"))

	secret, err = clients.K8SClient.GetSecret(secretName)
	require.NoError(t, err)
	assert.Equal(t, "admin", string(secret.Data["user"]))
	assert.Equal(t, "somepass", string(secret.Data["password"]))
}

func TestConcurrentAddBucketNoStores(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)

	_, err := o.AddBucket(testCtx, &object.Bucket{Name: name("orphan")}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to create bucket")
}

func TestConcurrentAddBucketMatchesObjectStoreParameters(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeAName, storeBName := name("store-a"), name("store-b")
	bucketName := name("bucket")

	_, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeAName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	storeBConfig, err := json.Marshal(map[string]any{
		"objectStoreDriver": "fake",
		"objectStoreName":   storeBName,
		"endpoint":          "https://localhost",
		"labels":            map[string]string{"performance": "gold"},
	})
	require.NoError(t, err)
	storeB, err := addOrUpdateFakeObjectStore(o, storeBConfig, fakeObjectStoreSecretConfig())
	require.NoError(t, err)

	bucket, err := o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, map[string]string{
		"selector": "performance=gold",
	})
	require.NoError(t, err)
	require.NotNil(t, bucket)
	assert.Equal(t, storeB.StoreUUID(), bucket.StoreUUID)

	_, err = storeB.GetBucketByName(testCtx, bucketName)
	assert.NoError(t, err)
}

func TestConcurrentAddBucketNoMatchingObjectStore(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName := name("store")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)

	bucketName := name("bucket")
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketName}, map[string]string{
		"objectStoreType": "unsupported",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to create bucket")

	_, err = store.GetBucketByName(testCtx, bucketName)
	assert.True(t, errors.IsNotFoundError(err), err)
}

func TestConcurrentGrantRevokeBucketAccess(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeName := name("store-a")
	bucket1, bucket2 := name("bucket-1"), name("bucket-2")
	access1, accessMulti := name("access-1"), name("access-multi")

	store, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucket1}, nil)
	require.NoError(t, err)
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucket2}, nil)
	require.NoError(t, err)

	access, _, err := o.GrantBucketAccess(testCtx, []object.BucketAccessMode{
		{BucketName: bucket1, AccessMode: object.AccessModeReadWrite},
	}, access1)
	require.NoError(t, err)
	require.NotNil(t, access)
	assert.Equal(t, access1, access.Name)
	assert.Equal(t, store.StoreUUID(), access.StoreUUID)

	// The fake driver rejects the grant unless every requested mode names an existing
	// bucket, so a successful multi-bucket grant proves both modes reached the driver.
	multi, _, err := o.GrantBucketAccess(testCtx, []object.BucketAccessMode{
		{BucketName: bucket1, AccessMode: object.AccessModeRead},
		{BucketName: bucket2, AccessMode: object.AccessModeWrite},
	}, accessMulti)
	require.NoError(t, err)
	require.NotNil(t, multi)
	assert.Equal(t, accessMulti, multi.Name)
	assert.Equal(t, store.StoreUUID(), multi.StoreUUID)

	require.NoError(t, o.RevokeBucketAccess(testCtx, access1))
	err = o.RevokeBucketAccess(testCtx, access1)
	assert.True(t, errors.IsNotFoundError(err), err)

	require.NoError(t, o.DeleteBucket(testCtx, bucket1))
	require.NoError(t, o.DeleteBucket(testCtx, bucket2))
	require.NoError(t, o.RevokeBucketAccess(testCtx, accessMulti))
}

func TestConcurrentGrantBucketAccessCrossStoreConflict(t *testing.T) {
	o, name, _ := setupObjectStoreOrchestrator(t)
	storeAName, storeBName := name("store-a"), name("store-b")
	bucketAName, bucketBName := name("bucket-a"), name("bucket-b")

	storeA, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeAName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)
	storeB, err := addOrUpdateFakeObjectStore(o, fakeObjectStoreConfig(storeBName), fakeObjectStoreSecretConfig())
	require.NoError(t, err)

	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketAName}, nil)
	require.NoError(t, err)
	_, err = o.AddBucket(testCtx, &object.Bucket{Name: bucketBName}, nil)
	require.NoError(t, err)

	_, results, unlock, err := db.Lock(testCtx,
		db.Query(db.UpsertBucket(bucketAName, storeA.StoreUUID())),
		db.Query(db.UpsertBucket(bucketBName, storeB.StoreUUID())),
	)
	require.NoError(t, err)
	bucketA := results[0].Bucket.Read
	bucketB := results[1].Bucket.Read
	require.NotNil(t, bucketA)
	require.NotNil(t, bucketB)
	bucketA.StoreUUID = storeA.StoreUUID()
	bucketB.StoreUUID = storeB.StoreUUID()
	results[0].Bucket.Upsert(bucketA)
	results[1].Bucket.Upsert(bucketB)
	unlock()

	_, _, err = o.GrantBucketAccess(testCtx, []object.BucketAccessMode{
		{BucketName: bucketAName, AccessMode: object.AccessModeRead},
		{BucketName: bucketBName, AccessMode: object.AccessModeRead},
	}, name("access-cross"))
	assert.True(t, errors.IsConflictError(err), err)

	require.NoError(t, o.DeleteBucket(testCtx, bucketAName))
	require.NoError(t, o.DeleteBucket(testCtx, bucketBName))
}

func fakeNamedStores(t *testing.T, names ...string) []object.Store {
	t.Helper()
	stores := make([]object.Store, len(names))
	for i, name := range names {
		store, err := fake.NewObjectDriver(testCtx, nil, fakeObjectStoreConfig(name))
		require.NoError(t, err)
		stores[i] = store
	}
	return stores
}

func objectStoreStatusString(t *testing.T, raw []byte, key string) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	value, _ := doc[key].(string)
	return value
}

func objectStoreNames(stores []object.Store) []string {
	names := make([]string, len(stores))
	for i, store := range stores {
		names[i] = store.Name()
	}
	return names
}

func TestShuffleStores(t *testing.T) {
	t.Run("keeps the preferred store first", func(t *testing.T) {
		// AddBucket leaves the existing bucket's store at index 0 and shuffles stores[1:].
		const preferred = "preferred-store"
		original := []string{preferred, "store-a", "store-b", "store-c", "store-d", "store-e"}
		tailShuffled := false
		for range 100 {
			stores := fakeNamedStores(t, original...)
			shuffleStores(stores[1:])
			require.Equal(t, preferred, stores[0].Name())
			got := objectStoreNames(stores)
			assert.ElementsMatch(t, original, got)
			if !slices.Equal(original[1:], got[1:]) {
				tailShuffled = true
			}
		}
		assert.True(t, tailShuffled, "expected stores after the preferred store to be shuffled")
	})

	t.Run("shuffles all stores", func(t *testing.T) {
		// AddBucket shuffles stores[0:] when the bucket has no preferred store.
		original := []string{"store-a", "store-b", "store-c", "store-d", "store-e"}
		shuffled := false
		firstMoved := false
		for range 100 {
			stores := fakeNamedStores(t, original...)
			shuffleStores(stores[0:])
			got := objectStoreNames(stores)
			assert.ElementsMatch(t, original, got)
			if !slices.Equal(original, got) {
				shuffled = true
			}
			if got[0] != original[0] {
				firstMoved = true
			}
		}
		assert.True(t, shuffled, "expected the full store list to be shuffled")
		assert.True(t, firstMoved, "expected the first store to be eligible to move")
	})
}
