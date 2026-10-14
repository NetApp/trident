// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file drives Provisioner against a real core.Orchestrator, not the
// mock_core.MockOrchestrator provisioner_test.go uses. It proves the whole path COSI's
// reconcilers depend on actually works: Provisioner -> core.Orchestrator -> concurrent_cache
// locking -> object.Store, using object/drivers/fake as the backend.
//
// This does not need cosi/ or a live Kubernetes cluster - unlike
// cosi/frontend_integration_test.go, which already covers the vendored reconcilers against
// cosi/provisioner/fake. Together the two tests cover the full stack in two independently
// runnable halves, split at the same seam the rest of this design keeps cosi/ out of core.

package objectprovisioner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/core"
	db "github.com/netapp/trident/core/concurrent_cache"
	"github.com/netapp/trident/cosi/provisioner"
	objectfactory "github.com/netapp/trident/object/factory"
	persistentstore "github.com/netapp/trident/persistent_store"
)

// newTestOrchestrator returns a real, bootstrapped orchestrator over a fresh in-memory store,
// with one fake object store already registered.
//
// db.Initialize resets concurrent_cache's package-level state, which every orchestrator
// instance shares - without it, "store-a" from one test's orchestrator would collide with the
// next test's, since they are not actually isolated from each other.
func newTestOrchestrator(t *testing.T) core.Orchestrator {
	t.Helper()
	db.Initialize()

	orchestrator, err := core.NewConcurrentTridentOrchestrator(persistentstore.NewInMemoryClient())
	require.NoError(t, err)
	require.NoError(t, orchestrator.Bootstrap(false))

	config, err := json.Marshal(map[string]string{
		"objectStoreDriver": "fake",
		"objectStoreName":   "store-a",
		"endpoint":          "https://s3.example.com",
	})
	require.NoError(t, err)
	secretConfig, err := json.Marshal(map[string]string{
		"user":     "alice",
		"password": "secret",
	})
	require.NoError(t, err)
	store, err := objectfactory.CreateStore(context.Background(), "fake", secretConfig, config)
	require.NoError(t, err)
	_, err = orchestrator.AddOrUpdateObjectStore(context.Background(), store)
	require.NoError(t, err)

	return orchestrator
}

func TestIntegrationBucketLifecycle(t *testing.T) {
	orchestrator := newTestOrchestrator(t)
	p := New(orchestrator)
	ctx := context.Background()

	info, err := p.DriverInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, DriverName, info.Name)

	created, err := p.CreateBucket(ctx, &provisioner.CreateBucketRequest{Name: "my-bucket"})
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", created.BucketID)
	require.NotNil(t, created.Info.S3)
	assert.Equal(t, "https://s3.example.com", created.Info.S3.Endpoint)

	got, err := p.GetBucket(ctx, &provisioner.GetBucketRequest{BucketID: "my-bucket"})
	require.NoError(t, err)
	assert.Equal(t, created, got)

	access, err := p.GrantAccess(ctx, &provisioner.GrantAccessRequest{
		AccountName: "my-account",
		Buckets:     []provisioner.AccessedBucket{{BucketID: "my-bucket", AccessMode: provisioner.AccessModeReadWrite}},
	})
	require.NoError(t, err)
	assert.Equal(t, "my-account", access.AccountID)
	require.Len(t, access.Buckets, 1)
	assert.Equal(t, "my-bucket", access.Buckets[0].BucketID)
	require.NotNil(t, access.Credentials.S3)
	assert.NotEmpty(t, access.Credentials.S3.AccessKeyID)

	require.NoError(t, p.RevokeAccess(ctx, &provisioner.RevokeAccessRequest{AccountID: "my-account"}))
	assert.NoError(t, p.RevokeAccess(ctx, &provisioner.RevokeAccessRequest{AccountID: "my-account"}),
		"RevokeAccess must be idempotent")

	require.NoError(t, p.DeleteBucket(ctx, &provisioner.DeleteBucketRequest{BucketID: "my-bucket"}))
	assert.NoError(t, p.DeleteBucket(ctx, &provisioner.DeleteBucketRequest{BucketID: "my-bucket"}),
		"DeleteBucket must be idempotent")

	_, err = p.GetBucket(ctx, &provisioner.GetBucketRequest{BucketID: "my-bucket"})
	assert.ErrorIs(t, err, provisioner.ErrNotFound)
}

func TestIntegrationCreateBucketOnMissingStoreName(t *testing.T) {
	orchestrator := newTestOrchestrator(t)
	p := New(orchestrator)
	ctx := context.Background()

	// No objectStoreName parameter: AddBucket's own selection picks the only configured store.
	created, err := p.CreateBucket(ctx, &provisioner.CreateBucketRequest{Name: "auto-placed"})
	require.NoError(t, err)
	assert.Equal(t, "auto-placed", created.BucketID)
}

func TestIntegrationGetBucketNotFound(t *testing.T) {
	orchestrator := newTestOrchestrator(t)
	p := New(orchestrator)

	_, err := p.GetBucket(context.Background(), &provisioner.GetBucketRequest{BucketID: "never-created"})
	assert.ErrorIs(t, err, provisioner.ErrNotFound)
}
