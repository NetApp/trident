// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/object"
	objectfake "github.com/netapp/trident/object/drivers/fake"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentlisters "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

func newObjectStoreTestController(t *testing.T) (*TridentCrdController, *mockcore.MockOrchestrator) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller, err := newTridentCrdControllerImpl(
		orchestrator,
		"trident",
		GetTestKubernetesClientset(),
		GetTestSnapshotClientset(),
		GetTestCrdClientset(),
		nil,
		nil,
	)
	require.NoError(t, err)
	return controller, orchestrator
}

func testObjectStore(name, secretName string, withFinalizer bool) *tridentv1.TridentObjectStore {
	objectStore := &tridentv1.TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "trident",
		},
		Spec: tridentv1.TridentObjectStoreSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(`{
				"objectStoreDriver":"fake",
				"endpoint":"example.com",
				"credentials":{"secretName":"` + secretName + `"}
			}`)},
		},
	}
	if withFinalizer {
		objectStore.Finalizers = []string{tridentv1.TridentFinalizerName}
	}
	return objectStore
}

func TestHandleTridentObjectStorePassesSecretSeparately(t *testing.T) {
	controller, orchestrator := newObjectStoreTestController(t)

	objectStore := testObjectStore("sg-tenant1", "sg-tenant1-secret", true)
	objectStoreIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	require.NoError(t, objectStoreIndexer.Add(objectStore))
	controller.objectStoresLister = tridentlisters.NewTridentObjectStoreLister(objectStoreIndexer)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sg-tenant1-secret", Namespace: "trident"},
		Data: map[string][]byte{
			"user":     []byte("admin"),
			"password": []byte("somepass"),
		},
	}
	secretIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	require.NoError(t, secretIndexer.Add(secret))
	controller.secretsLister = corev1listers.NewSecretLister(secretIndexer)

	orchestrator.EXPECT().AddOrUpdateObjectStore(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, store object.Store) (object.Store, error) {
			require.NotNil(t, store)
			assert.Equal(t, "fake", store.DriverName())
			assert.Equal(t, "sg-tenant1", store.Name())
			fakeStore, ok := store.(*objectfake.ObjectDriver)
			require.True(t, ok)
			assert.Equal(t, "example.com", fakeStore.Endpoint)
			assert.Equal(t, "admin", fakeStore.User)
			assert.Equal(t, "somepass", fakeStore.Password)

			serialized, err := json.Marshal(store)
			require.NoError(t, err)
			assert.NotContains(t, string(serialized), "admin")
			assert.NotContains(t, string(serialized), "somepass")
			assert.NotContains(t, string(serialized), "sg-tenant1-secret")
			return store, nil
		},
	)

	err := controller.handleTridentObjectStore(&KeyItem{
		key:        "trident/sg-tenant1",
		event:      EventAdd,
		ctx:        context.Background(),
		objectType: ObjectTypeTridentObjectStore,
	})
	require.NoError(t, err)
}

func TestHandleTridentObjectStoreRequeuesWhenSecretMissing(t *testing.T) {
	for _, event := range []EventType{EventAdd, EventUpdate} {
		t.Run(string(event), func(t *testing.T) {
			controller, _ := newObjectStoreTestController(t)

			objectStore := testObjectStore("sg-tenant1", "sg-tenant1-secret", true)
			objectStoreIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			require.NoError(t, objectStoreIndexer.Add(objectStore))
			controller.objectStoresLister = tridentlisters.NewTridentObjectStoreLister(objectStoreIndexer)
			controller.secretsLister = corev1listers.NewSecretLister(
				cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{}),
			)

			err := controller.handleTridentObjectStore(&KeyItem{
				key:        "trident/sg-tenant1",
				event:      event,
				ctx:        context.Background(),
				objectType: ObjectTypeTridentObjectStore,
			})
			require.Error(t, err)
			assert.True(t, errors.IsReconcileDeferredError(err), "missing secret should requeue")
			assert.False(t, errors.IsUnsupportedConfigError(err), "missing secret is not a permanent config error")
		})
	}
}

func TestHandleTridentObjectStoreDeletion(t *testing.T) {
	tests := []struct {
		name              string
		deleteErr         error
		expectErr         bool
		expectFinalizer   bool
		objectStoreExists bool
	}{
		{
			name:              "store unknown to core releases finalizer",
			objectStoreExists: true,
		},
		{
			name:              "CR already removed by core",
			objectStoreExists: false,
		},
		{
			name:              "store with buckets keeps finalizer",
			deleteErr:         errors.ConflictError("object store sg-tenant1 has buckets"),
			expectErr:         true,
			expectFinalizer:   true,
			objectStoreExists: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller, orchestrator := newObjectStoreTestController(t)

			objectStore := testObjectStore("sg-tenant1", "sg-tenant1-secret", true)
			now := metav1.Now()
			objectStore.DeletionTimestamp = &now
			objectStoreIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			require.NoError(t, objectStoreIndexer.Add(objectStore))
			controller.objectStoresLister = tridentlisters.NewTridentObjectStoreLister(objectStoreIndexer)

			tosClient := controller.crdClientset.TridentV1().TridentObjectStores("trident")
			if test.objectStoreExists {
				_, err := tosClient.Create(context.Background(), objectStore, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			orchestrator.EXPECT().DeleteObjectStore(gomock.Any(), "sg-tenant1").Return(test.deleteErr)

			err := controller.handleTridentObjectStore(&KeyItem{
				key:        "trident/sg-tenant1",
				event:      EventUpdate,
				ctx:        context.Background(),
				objectType: ObjectTypeTridentObjectStore,
			})
			if test.expectErr {
				require.Error(t, err)
				assert.True(t, errors.IsReconcileDeferredError(err))
			} else {
				require.NoError(t, err)
			}

			latest, err := tosClient.Get(context.Background(), "sg-tenant1", metav1.GetOptions{})
			if test.expectFinalizer {
				require.NoError(t, err)
				assert.True(t, latest.HasTridentFinalizers())
			} else {
				assert.True(t, k8sapierrors.IsNotFound(err), "CR should be gone once its finalizer is released")
			}
		})
	}
}

func TestHandleTridentObjectStoreDeletionRemovesCreatedSecret(t *testing.T) {
	controller, orchestrator := newObjectStoreTestController(t)

	objectStore := testObjectStore("sg-tenant1", "sg-tenant1-creds", true)
	now := metav1.Now()
	objectStore.DeletionTimestamp = &now
	objectStoreIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	require.NoError(t, objectStoreIndexer.Add(objectStore))
	controller.objectStoresLister = tridentlisters.NewTridentObjectStoreLister(objectStoreIndexer)

	tosClient := controller.crdClientset.TridentV1().TridentObjectStores("trident")
	_, err := tosClient.Create(context.Background(), objectStore, metav1.CreateOptions{})
	require.NoError(t, err)

	secrets := controller.kubeClientset.CoreV1().Secrets("trident")
	_, err = secrets.Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sg-tenant1-creds", Namespace: "trident"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = secrets.Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-managed-secret", Namespace: "trident"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	orchestrator.EXPECT().DeleteObjectStore(gomock.Any(), "sg-tenant1").Return(nil)

	err = controller.handleTridentObjectStore(&KeyItem{
		key:        "trident/sg-tenant1",
		event:      EventUpdate,
		ctx:        context.Background(),
		objectType: ObjectTypeTridentObjectStore,
	})
	require.NoError(t, err)

	_, err = secrets.Get(context.Background(), "sg-tenant1-creds", metav1.GetOptions{})
	assert.True(t, k8sapierrors.IsNotFound(err))
	_, err = secrets.Get(context.Background(), "user-managed-secret", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestHandleTridentObjectStoreRequeuesWhenSecretNameCannotBeRead(t *testing.T) {
	for _, event := range []EventType{EventAdd, EventUpdate} {
		t.Run(string(event), func(t *testing.T) {
			controller, _ := newObjectStoreTestController(t)

			objectStore := testObjectStore("sg-tenant1", "sg-tenant1-secret", true)
			objectStore.Spec.Raw = []byte(`{"objectStoreDriver":"fake","credentials":{"name":"wrong-field"}}`)
			objectStoreIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			require.NoError(t, objectStoreIndexer.Add(objectStore))
			controller.objectStoresLister = tridentlisters.NewTridentObjectStoreLister(objectStoreIndexer)

			err := controller.handleTridentObjectStore(&KeyItem{
				key:        "trident/sg-tenant1",
				event:      event,
				ctx:        context.Background(),
				objectType: ObjectTypeTridentObjectStore,
			})
			require.Error(t, err)
			assert.True(t, errors.IsReconcileDeferredError(err), "secret name failure should requeue")
			assert.False(t, errors.IsUnsupportedConfigError(err), "secret name failure is not a permanent config error")
			assert.ErrorContains(t, err, "credentials.secretName is required")
		})
	}
}
