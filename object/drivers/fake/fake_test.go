// Copyright 2026 NetApp, Inc. All Rights Reserved.

package fake

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/object"
	"github.com/netapp/trident/utils/errors"
)

func newTestDriver(t *testing.T) *ObjectDriver {
	t.Helper()
	store, err := NewObjectDriver(
		context.Background(), []byte(`{"user":"alice","password":"secret"}`),
		[]byte(`{"objectStoreName":"fake","endpoint":"https://localhost"}`),
	)
	require.NoError(t, err)
	d := store.(*ObjectDriver)
	require.NoError(t, d.Initialize(context.Background()))
	return d
}

func TestCreateDeleteBucketRoundTrip(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	bucket := &object.Bucket{Name: "my-bucket"}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err)
	got, err := d.GetBucketByName(ctx, "my-bucket")
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", got.Name)
	assert.Equal(t, "trident-my-bucket", got.InternalID)
	assert.Equal(t, d.StoreUUID(), got.StoreUUID)
	assert.Contains(t, d.Buckets(), "trident-my-bucket")

	require.NoError(t, d.DeleteBucket(ctx, "my-bucket"))
	_, err = d.GetBucketByName(ctx, "my-bucket")
	assert.True(t, errors.IsNotFoundError(err))
	assert.NotContains(t, d.Buckets(), "trident-my-bucket")
}

// TestCreateBucketDuplicate: a repeat CreateBucket for the same name must succeed and return
// the same Info as the original, not error.
func TestCreateBucketDuplicate(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	first := &object.Bucket{Name: "dup"}
	err := d.CreateBucket(ctx, first)
	require.NoError(t, err)
	require.NotNil(t, first.Info.S3)

	second := &object.Bucket{Name: "dup"}
	err = d.CreateBucket(ctx, second)
	require.NoError(t, err)
	assert.Equal(t, first.Info, second.Info)

	assert.Equal(t, 2, d.Calls()["CreateBucket"])
}

func TestCreateBucketInjectedError(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	injected := errors.AlreadyExistsError("bucket conflicts with an unrelated resource")
	d.SetError("CreateBucket", injected)

	err := d.CreateBucket(ctx, &object.Bucket{Name: "conflict"})
	assert.ErrorIs(t, err, injected)
	assert.Equal(t, 1, d.Calls()["CreateBucket"])
	// The injected error must short-circuit before the bucket is stored.
	assert.NotContains(t, d.Buckets(), "trident-conflict")

	d.SetError("CreateBucket", nil)
	err = d.CreateBucket(ctx, &object.Bucket{Name: "conflict"})
	require.NoError(t, err)
	assert.Equal(t, 2, d.Calls()["CreateBucket"])
}

func TestDeleteBucketMissing(t *testing.T) {
	d := newTestDriver(t)
	assert.NoError(t, d.DeleteBucket(context.Background(), "missing"))
}

func TestGrantRevokeBucketAccess(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	err := d.CreateBucket(ctx, &object.Bucket{Name: "cred-bucket"})
	require.NoError(t, err)

	access := &object.BucketAccess{Name: "access-1"}
	modes := []object.BucketAccessMode{
		{BucketName: "cred-bucket", AccessMode: object.AccessModeReadWrite},
	}
	_, err = d.GrantBucketAccess(ctx, modes, access)
	require.NoError(t, err)
	assert.Contains(t, d.Accesses(), "trident-access-1")

	require.NoError(t, d.RevokeBucketAccess(ctx, access))
	assert.NotContains(t, d.Accesses(), "trident-access-1")
}

func TestGrantAccessMissingBucket(t *testing.T) {
	d := newTestDriver(t)
	_, err := d.GrantBucketAccess(context.Background(),
		[]object.BucketAccessMode{{BucketName: "missing", AccessMode: object.AccessModeRead}},
		&object.BucketAccess{Name: "access-missing"},
	)
	assert.True(t, errors.IsNotFoundError(err))
}

func TestSetCommonConfigIgnoresZeroValues(t *testing.T) {
	d := newTestDriver(t)
	originalName := d.Name()
	originalUUID := d.StoreUUID()
	originalState := d.State()
	require.NotEmpty(t, originalName)
	require.NotEmpty(t, originalUUID)
	// Initialize does not set state. Core marks the store online after it succeeds.
	assert.Empty(t, originalState)

	d.SetCommonConfig(object.CommonConfig{})
	assert.Equal(t, originalName, d.Name())
	assert.Equal(t, originalUUID, d.StoreUUID())
	assert.Equal(t, originalState, d.State())

	d.SetCommonConfig(object.CommonConfig{StoreName: "renamed"})
	assert.Equal(t, "renamed", d.Name())
	assert.Equal(t, originalUUID, d.StoreUUID())
	assert.Equal(t, originalState, d.State())

	d.SetCommonConfig(object.CommonConfig{UUID: "uuid-set"})
	assert.Equal(t, "renamed", d.Name())
	assert.Equal(t, "uuid-set", d.StoreUUID())
	assert.Equal(t, originalState, d.State())

	d.SetCommonConfig(object.CommonConfig{State: object.StoreStateDeleting})
	assert.Equal(t, "renamed", d.Name())
	assert.Equal(t, "uuid-set", d.StoreUUID())
	assert.Equal(t, object.StoreStateDeleting, d.State())
}

func TestStoreIdentity(t *testing.T) {
	d := newTestDriver(t)
	assert.Equal(t, "fake", d.Name())
	assert.Equal(t, "fake", d.DriverName())
	assert.NotEmpty(t, d.StoreUUID())
	assert.Equal(t, "fake", d.GetUniqueKey())
	assert.Empty(t, d.State())
	assert.NotSame(t, d, d.SmartCopy())

	d.SetState(object.StoreStateDeleting)
	assert.Equal(t, object.StoreStateDeleting, d.State())

	require.NoError(t, d.Terminate(context.Background()))
	assert.False(t, d.Initialized())
}

func TestSecretsSeparatedFromSerialization(t *testing.T) {
	ctx := context.Background()
	store, err := NewObjectDriver(ctx, []byte(`{"user":"alice","password":"s3cret"}`), []byte(`{
		"objectStoreName":"secret-store",
		"endpoint":"https://s3.example"
	}`))
	require.NoError(t, err)
	d := store.(*ObjectDriver)

	assert.Equal(t, "alice", d.User)
	assert.Equal(t, "s3cret", d.Password)
	assert.Equal(t, map[string]string{
		"user":     "alice",
		"password": "s3cret",
	}, d.Secrets())

	serialized, err := json.Marshal(d)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "alice")
	assert.NotContains(t, string(serialized), "s3cret")
	assert.Contains(t, string(serialized), "secret-store")
	assert.Contains(t, string(serialized), "https://s3.example")
	assert.Contains(t, string(serialized), `"state"`)
}

func TestStateRoundTripsInJSON(t *testing.T) {
	d := newTestDriver(t)
	d.SetState(object.StoreStateDeleting)

	serialized, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(serialized), `"state":"deleting"`)
	assert.Contains(t, string(serialized), `"uuid":"`)

	roundTrip, err := NewObjectDriver(context.Background(), []byte(`{"user":"alice","password":"secret"}`), serialized)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateDeleting, roundTrip.State())
	assert.Equal(t, d.StoreUUID(), roundTrip.StoreUUID())
}

func TestNewObjectDriverAppliesConfigsInOrder(t *testing.T) {
	store, err := NewObjectDriver(context.Background(), nil,
		[]byte(`{"objectStoreName":"status-name","endpoint":"https://status.example","uuid":"uuid-status","state":"deleting"}`),
		nil,
		[]byte(`{"endpoint":"https://spec.example","labels":{"tier":"gold"}}`),
		[]byte(`{"objectStoreName":"cr-name"}`),
	)
	require.NoError(t, err)
	d := store.(*ObjectDriver)

	assert.Equal(t, "cr-name", d.Name())
	assert.Equal(t, "https://spec.example", d.Endpoint)
	assert.Equal(t, "uuid-status", d.StoreUUID())
	assert.Equal(t, object.StoreStateDeleting, d.State())
	assert.Equal(t, map[string]string{"tier": "gold"}, d.Labels)
	assert.Equal(t, "fake", d.ObjectStoreDriver)
}

func TestNewObjectDriverFromSecretConfig(t *testing.T) {
	ctx := context.Background()
	store, err := NewObjectDriver(ctx, []byte(`{"user":"admin","password":"somepass"}`), []byte(`{
		"objectStoreDriver":"fake",
		"objectStoreName":"sg-tenant1",
		"endpoint":"example.com"
	}`))
	require.NoError(t, err)
	d := store.(*ObjectDriver)

	assert.Equal(t, "sg-tenant1", d.Name())
	assert.Equal(t, "example.com", d.Endpoint)
	assert.Equal(t, "admin", d.User)
	assert.Equal(t, "somepass", d.Password)
	assert.NotContains(t, mustMarshal(t, d), "admin")
	assert.NotContains(t, mustMarshal(t, d), "somepass")
}

func mustMarshal(t *testing.T, value any) string {
	t.Helper()
	serialized, err := json.Marshal(value)
	require.NoError(t, err)
	return string(serialized)
}

func TestMatchParamsSelector(t *testing.T) {
	d := &ObjectDriver{
		ObjectStoreName: "store-b",
		Labels: map[string]string{
			"performance": "gold",
			"protection":  "minimal",
			"cost":        "100",
		},
	}
	tests := []struct {
		name   string
		params map[string]string
		want   bool
	}{
		{name: "nil params", params: nil, want: true},
		{name: "empty params", params: map[string]string{}, want: true},
		{name: "equal match", params: map[string]string{"selector": "performance=gold"}, want: true},
		{name: "equal mismatch", params: map[string]string{"selector": "performance=silver"}, want: false},
		{name: "not equal match", params: map[string]string{"selector": "protection != full"}, want: true},
		{name: "not equal mismatch", params: map[string]string{"selector": "protection != minimal"}, want: false},
		{name: "in match", params: map[string]string{"selector": "performance in (gold, silver)"}, want: true},
		{name: "in mismatch", params: map[string]string{"selector": "performance in (silver, bronze)"}, want: false},
		{name: "notin match", params: map[string]string{"selector": "performance notin (silver, bronze)"}, want: true},
		{name: "notin mismatch", params: map[string]string{"selector": "performance notin (gold, silver)"}, want: false},
		{name: "exists match", params: map[string]string{"selector": "protection"}, want: true},
		{name: "exists mismatch", params: map[string]string{"selector": "cloud"}, want: false},
		{name: "not exists match", params: map[string]string{"selector": "!cloud"}, want: true},
		{name: "not exists mismatch", params: map[string]string{"selector": "!protection"}, want: false},
		{
			name:   "compound match",
			params: map[string]string{"selector": "performance=gold; protection!=full; !cloud"},
			want:   true,
		},
		{
			name:   "compound mismatch",
			params: map[string]string{"selector": "performance=gold; protection=full"},
			want:   false,
		},
		{name: "numeric label via selector", params: map[string]string{"selector": "cost=100"}, want: true},
		{name: "invalid selector", params: map[string]string{"selector": "performance in gold"}, want: false},
		{name: "empty selector", params: map[string]string{"selector": ""}, want: false},
		{name: "objectStoreType match", params: map[string]string{"objectStoreType": "fake"}, want: true},
		{name: "objectStoreType mismatch", params: map[string]string{"objectStoreType": "unsupported"}, want: false},
		{name: "unknown key rejects the store", params: map[string]string{"objectStoreName": "store-a"}, want: false},
		{
			name: "selector and objectStoreType",
			params: map[string]string{
				"objectStoreType": "fake",
				"selector":        "performance=gold",
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, d.MatchParams(test.params))
		})
	}
}
