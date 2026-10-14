// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/object"
	objectfake "github.com/netapp/trident/object/drivers/fake"
	"github.com/netapp/trident/object/drivers/storagegrid"
)

func TestTridentObjectStoreConversion(t *testing.T) {
	store, err := objectfake.NewObjectDriver(context.Background(), []byte(`{"user":"alice","password":"secret"}`), []byte(
		`{"objectStoreName":"store-a","endpoint":"https://localhost","uuid":"uuid-a","state":"online"}`,
	))
	require.NoError(t, err)

	tos, err := NewTridentObjectStore(store)
	require.NoError(t, err)
	storeJSON, err := json.Marshal(store)
	require.NoError(t, err)
	assert.JSONEq(t, string(storeJSON), string(tos.Spec.Raw))
	assert.JSONEq(t, string(storeJSON), string(tos.Status.Raw))
	assert.Contains(t, string(tos.Spec.Raw), `"objectStoreDriver":"fake"`)
	assert.NotContains(t, string(tos.Spec.Raw), "alice")
	assert.NotContains(t, string(tos.Spec.Raw), "secret")
	assert.Equal(t, string(object.StoreStateOnline), statusString(t, tos.Status.Raw, "state"))
	assert.Equal(t, "uuid-a", statusString(t, tos.Status.Raw, "uuid"))

	persistent, err := tos.Persistent(context.Background())
	require.NoError(t, err)
	assert.Equal(t, store.Name(), persistent.Name())
	assert.Equal(t, store.StoreUUID(), persistent.StoreUUID())
	assert.Equal(t, store.State(), persistent.State())
}

func TestTridentObjectStoreStorageGridConversion(t *testing.T) {
	secret := []byte(`{"username":"root","password":"secret"}`)
	store, err := storagegrid.NewObjectDriver(context.Background(), secret, []byte(`{
		"objectStoreName":"sg",
		"managementEndpoint":"https://grid.example",
		"s3Endpoint":"https://s3.grid.example",
		"accountId":"12345678901234567890",
		"labels":{"performance":"gold"},
		"uuid":"uuid-sg",
		"state":"online"
	}`))
	require.NoError(t, err)

	tos, err := NewTridentObjectStore(store)
	require.NoError(t, err)
	storeJSON, err := json.Marshal(store)
	require.NoError(t, err)
	assert.JSONEq(t, string(storeJSON), string(tos.Spec.Raw))
	assert.JSONEq(t, string(storeJSON), string(tos.Status.Raw))
	assert.Contains(t, string(tos.Spec.Raw), `"objectStoreDriver":"storage-grid"`)
	assert.Contains(t, string(tos.Spec.Raw), `"accountId":"12345678901234567890"`)
	assert.NotContains(t, string(tos.Spec.Raw), "secret")
	assert.Equal(t, string(object.StoreStateOnline), statusString(t, tos.Status.Raw, "state"))
	assert.Equal(t, "uuid-sg", statusString(t, tos.Status.Raw, "uuid"))

	persistent, err := tos.Persistent(context.Background(), secret)
	require.NoError(t, err)
	assert.Equal(t, store.Name(), persistent.Name())
	assert.Equal(t, store.StoreUUID(), persistent.StoreUUID())
	assert.Equal(t, store.State(), persistent.State())
	assert.True(t, persistent.MatchParams(map[string]string{"selector": "performance=gold"}))
	grid, ok := persistent.(*storagegrid.ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "root", grid.Username)
	assert.Equal(t, "secret", grid.Password)
	assert.Equal(t, "12345678901234567890", grid.AccountID)
}

func TestTridentObjectStorePersistentPrefersSpecOverStatus(t *testing.T) {
	tos := &TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a"},
		Spec: TridentObjectStoreSpec{RawExtension: runtime.RawExtension{Raw: []byte(`{
			"objectStoreDriver":"fake",
			"endpoint":"https://spec.example",
			"objectStoreName":"ignored"
		}`)}},
		Status: TridentObjectStoreStatus{RawExtension: runtime.RawExtension{Raw: []byte(`{
			"objectStoreDriver":"fake",
			"endpoint":"https://stale.example",
			"uuid":"uuid-status",
			"state":"deleting"
		}`)}},
	}

	persistent, err := tos.Persistent(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "store-a", persistent.Name())
	assert.Equal(t, "uuid-status", persistent.StoreUUID())
	assert.Equal(t, object.StoreStateDeleting, persistent.State())
	fakeStore, ok := persistent.(*objectfake.ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "https://spec.example", fakeStore.Endpoint)
}

func TestTridentObjectStorePersistentUsesStatusDriverWhenSpecHasNone(t *testing.T) {
	tos := &TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a"},
		Status: TridentObjectStoreStatus{RawExtension: runtime.RawExtension{Raw: []byte(`{
			"objectStoreDriver":"fake",
			"endpoint":"https://status.example",
			"uuid":"uuid-status"
		}`)}},
	}

	persistent, err := tos.Persistent(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fake", persistent.DriverName())
	assert.Equal(t, "store-a", persistent.Name())
	assert.Equal(t, "uuid-status", persistent.StoreUUID())
}

func TestTridentObjectStorePersistentRequiresDriver(t *testing.T) {
	tos := &TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a"},
		Spec:       TridentObjectStoreSpec{RawExtension: runtime.RawExtension{Raw: []byte(`{"endpoint":"https://x"}`)}},
	}

	_, err := tos.Persistent(context.Background())
	require.ErrorContains(t, err, "objectStoreDriver is required")
}

func TestTridentObjectStorePersistentUsesSecretConfig(t *testing.T) {
	tos := &TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a"},
		Spec: TridentObjectStoreSpec{RawExtension: runtime.RawExtension{Raw: []byte(`{
			"objectStoreDriver":"fake",
			"endpoint":"https://spec.example",
			"credentials":{"secretName":"store-a-secret"}
		}`)}},
		Status: TridentObjectStoreStatus{RawExtension: runtime.RawExtension{Raw: []byte(`{
			"objectStoreDriver":"fake",
			"endpoint":"https://spec.example",
			"uuid":"uuid-status",
			"state":"online"
		}`)}},
	}

	persistent, err := tos.Persistent(
		context.Background(), []byte(`{"user":"alice","password":"secret"}`),
	)
	require.NoError(t, err)
	fakeStore, ok := persistent.(*objectfake.ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "alice", fakeStore.User)
	assert.Equal(t, "secret", fakeStore.Password)
	assert.NotContains(t, mustMarshal(t, fakeStore), "store-a-secret")
}

func TestTridentObjectStoreApplyDoesNotMutateSpec(t *testing.T) {
	specJSON := []byte(`{"objectStoreDriver":"fake","endpoint":"https://user.example","credentials":{"secretName":"user-secret"}}`)
	tos := &TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a"},
		Spec:       TridentObjectStoreSpec{RawExtension: runtime.RawExtension{Raw: specJSON}},
	}
	store, err := objectfake.NewObjectDriver(context.Background(), []byte(`{"user":"alice","password":"secret"}`), []byte(
		`{"objectStoreName":"store-a","endpoint":"https://driver.example","uuid":"uuid-b"}`,
	))
	require.NoError(t, err)
	store.SetState(object.StoreStateOnline)

	require.NoError(t, tos.Apply(store))
	assert.JSONEq(t, string(specJSON), string(tos.Spec.Raw))
	assert.JSONEq(t, mustMarshal(t, store), string(tos.Status.Raw))
	assert.NotContains(t, string(tos.Status.Raw), `"secretName"`)
	assert.NotContains(t, string(tos.Status.Raw), "alice")
	assert.Equal(t, "uuid-b", statusString(t, tos.Status.Raw, "uuid"))
	assert.Equal(t, string(object.StoreStateOnline), statusString(t, tos.Status.Raw, "state"))
}

func TestTridentObjectStoreApplyRejectsNameMismatch(t *testing.T) {
	tos := &TridentObjectStore{ObjectMeta: metav1.ObjectMeta{Name: "store-a"}}
	store, err := objectfake.NewObjectDriver(context.Background(), nil, []byte(
		`{"objectStoreName":"store-b","endpoint":"https://driver.example"}`,
	))
	require.NoError(t, err)

	assert.ErrorIs(t, tos.Apply(store), ErrNamesDontMatch)
}

func TestTridentObjectStoreSpecSecretConfig(t *testing.T) {
	spec := TridentObjectStoreSpec{RawExtension: runtime.RawExtension{Raw: []byte(`{
		"objectStoreDriver":"fake",
		"endpoint":"https://example.com",
		"credentials":{"secretName":"store-secret"}
	}`)}}

	secretName, err := spec.GetSecretName()
	require.NoError(t, err)
	assert.Equal(t, "store-secret", secretName)
	driverName, err := spec.GetDriverName()
	require.NoError(t, err)
	assert.Equal(t, "fake", driverName)

	secretConfig, err := SecretConfig(map[string][]byte{
		"user":     []byte("admin"),
		"password": []byte("somepass"),
	})
	require.NoError(t, err)
	var actualSecret map[string]interface{}
	require.NoError(t, json.Unmarshal(secretConfig, &actualSecret))
	assert.Equal(t, "admin", actualSecret["user"])
	assert.Equal(t, "somepass", actualSecret["password"])
}

func TestTridentObjectStoreSpecRejectsInvalidCredentialsRef(t *testing.T) {
	spec := TridentObjectStoreSpec{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"credentials":{"name":"wrong-field"}}`)},
	}

	_, err := spec.GetSecretName()
	require.ErrorContains(t, err, "credentials.secretName is required")
}

func TestTridentBucketConversions(t *testing.T) {
	bucket := &object.Bucket{Name: "bucket-a", StoreName: "store-a"}
	tb, err := NewTridentBucket(bucket)
	require.NoError(t, err)
	assert.Equal(t, bucket, tb.Persistent())

	access := &object.BucketAccess{
		Name:      "access-a",
		StoreName: "store-a",
	}
	tba, err := NewTridentBucketAccess(access)
	require.NoError(t, err)
	persisted, err := tba.Persistent()
	require.NoError(t, err)
	assert.Equal(t, access, persisted)
}

func mustMarshal(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func statusString(t *testing.T, raw []byte, key string) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	value, _ := doc[key].(string)
	return value
}
