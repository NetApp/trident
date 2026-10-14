// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file is NETAPP LOCAL. It is not vendored from upstream and is absent from
// cosi/upstream.manifest, so resync.sh ignores it. It covers cosi/sidecar/bucket_delete.go.
//
// Upstream has no deletion tests because upstream has no deletion: its stub removed the
// protection finalizer and returned an error, and nothing in the vendored bucket_test.go
// exercises that path. So all coverage of Bucket deletion lives here.

package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	cositest "github.com/netapp/trident/cosi/third_party/test"
)

// deleteCapableServer adds DriverDeleteBucket to the vendored fake, which does not implement it
// because upstream never had a deletion path to test.
//
// Embedding rather than editing cosi/third_party/test/rpc.go keeps that file byte-identical to
// upstream: a method on the outer type shadows the promoted one, so this needs no divergence.
type deleteCapableServer struct {
	cositest.FakeProvisionerServer

	deleteFunc func(context.Context, *cosiproto.DriverDeleteBucketRequest) (*cosiproto.DriverDeleteBucketResponse, error)
}

func (s *deleteCapableServer) DriverDeleteBucket(
	ctx context.Context, req *cosiproto.DriverDeleteBucketRequest,
) (*cosiproto.DriverDeleteBucketResponse, error) {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, req)
	}
	panic("deleteFunc not set in deleteCapableServer")
}

// deleteFixture is a Bucket under a running fake driver, ready to have deletion reconciled.
type deleteFixture struct {
	deps       *cositest.Dependencies
	reconciler BucketReconciler
	nsName     types.NamespacedName

	// deleteReqs records every DriverDeleteBucket the reconciler made. Written on the gRPC
	// server goroutine and read after Reconcile returns, so the RPC round trip orders them.
	deleteReqs []*cosiproto.DriverDeleteBucketRequest

	// deleteErr is what the fake driver returns. Set before reconciling to simulate failure.
	deleteErr error
}

const testDriverName = "cosi.s3.corp.net"

// newDeleteFixture provisions a Bucket the way the reconciler would, then marks it for deletion,
// leaving it exactly as the reconciler finds it on the delete pass.
//
// Provisioning for real rather than hand-building the object matters: it is what puts the
// protection finalizer on and records status.bucketID, and a Delete on an object with no
// finalizer would vanish immediately instead of getting a deletion timestamp.
func newDeleteFixture(t *testing.T, policy cosiapi.BucketDeletionPolicy) *deleteFixture {
	t.Helper()

	f := &deleteFixture{}

	server := &deleteCapableServer{
		FakeProvisionerServer: cositest.FakeProvisionerServer{
			CreateBucketFunc: func(
				_ context.Context, req *cosiproto.DriverCreateBucketRequest,
			) (*cosiproto.DriverCreateBucketResponse, error) {
				return &cosiproto.DriverCreateBucketResponse{
					BucketId: "cosi-" + req.Name,
					Protocols: &cosiproto.ObjectProtocolAndBucketInfo{
						S3: &cosiproto.S3BucketInfo{
							Endpoint:        "s3.corp.net",
							BucketId:        "corp-cosi-" + req.Name,
							Region:          "us-east-1",
							AddressingStyle: &cosiproto.S3AddressingStyle{Style: cosiproto.S3AddressingStyle_PATH},
						},
					},
				}, nil
			},
		},
		deleteFunc: func(
			_ context.Context, req *cosiproto.DriverDeleteBucketRequest,
		) (*cosiproto.DriverDeleteBucketResponse, error) {
			f.deleteReqs = append(f.deleteReqs, req)
			if f.deleteErr != nil {
				return nil, f.deleteErr
			}
			return &cosiproto.DriverDeleteBucketResponse{}, nil
		},
	}

	rpcClient := mustServe(t, server)

	bucket := &cosiapi.Bucket{
		ObjectMeta: meta.ObjectMeta{Name: "bc-qwerty"},
		Spec: cosiapi.BucketSpec{
			DriverName:     testDriverName,
			DeletionPolicy: policy,
			Protocols:      []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
			Parameters:     map[string]string{"maxSize": "10Gi"},
			BucketClaimRef: cosiapi.BucketClaimReference{
				Name:      "my-bucket",
				Namespace: "my-ns",
				UID:       "qwerty",
			},
		},
	}

	f.deps = cositest.MustBootstrap(t, bucket)
	f.nsName = cositest.NsName(bucket)
	f.reconciler = BucketReconciler{
		Client: f.deps.Client,
		Scheme: f.deps.Client.Scheme(),
		DriverInfo: DriverInfo{
			Name:               testDriverName,
			SupportedProtocols: []cosiproto.ObjectProtocol_Type{cosiproto.ObjectProtocol_S3},
			ProvisionerClient:  rpcClient,
		},
	}

	ctx := f.deps.ContextWithLogger
	_, err := f.reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: f.nsName})
	require.NoError(t, err, "provisioning must succeed before deletion can be tested")

	provisioned := f.get(t)
	require.Contains(t, provisioned.GetFinalizers(), cosiapi.ProtectionFinalizer)
	require.Equal(t, "cosi-bc-qwerty", provisioned.Status.BucketID)

	require.NoError(t, f.deps.Client.Delete(ctx, provisioned.DeepCopy()))

	return f
}

// mustServe starts the fake provisioner and returns a client wired to it.
func mustServe(t *testing.T, server cosiproto.ProvisionerServer) cosiproto.ProvisionerClient {
	t.Helper()

	cleanup, serve, sock, err := cositest.RpcServer(nil, server)
	t.Cleanup(cleanup)
	require.NoError(t, err)
	go serve()

	conn, err := cositest.RpcClientConn(sock)
	require.NoError(t, err)

	return cosiproto.NewProvisionerClient(conn)
}

// reconcile runs the delete pass.
func (f *deleteFixture) reconcile(t *testing.T) error {
	t.Helper()

	_, err := f.reconciler.Reconcile(f.deps.ContextWithLogger, ctrl.Request{NamespacedName: f.nsName})

	return err
}

// get returns the Bucket, requiring that it still exists.
func (f *deleteFixture) get(t *testing.T) *cosiapi.Bucket {
	t.Helper()

	bucket := &cosiapi.Bucket{}
	require.NoError(t, f.reconciler.Get(f.deps.ContextWithLogger, f.nsName, bucket))

	return bucket
}

// gone reports whether the Bucket has been reaped, which the API server does once the last
// finalizer comes off an object with a deletion timestamp.
func (f *deleteFixture) gone(t *testing.T) bool {
	t.Helper()

	err := f.reconciler.Get(f.deps.ContextWithLogger, f.nsName, &cosiapi.Bucket{})

	return err != nil
}

// TestBucketDeletion_DeletePolicy is the happy path: the driver is asked to remove the backend
// bucket, and only then is the object released.
func TestBucketDeletion_DeletePolicy(t *testing.T) {
	f := newDeleteFixture(t, cosiapi.BucketDeletionPolicyDelete)

	require.NoError(t, f.reconcile(t))

	require.Len(t, f.deleteReqs, 1)
	assert.Equal(t, "cosi-bc-qwerty", f.deleteReqs[0].BucketId,
		"the driver must be given the recorded bucketID, not the object name")
	assert.Equal(t, map[string]string{"maxSize": "10Gi"}, f.deleteReqs[0].Parameters,
		"parameters travel with the delete so a driver can route it")

	assert.True(t, f.gone(t), "the finalizer should have come off, letting the object be reaped")
}

// TestBucketDeletion_RetainPolicy checks Retain keeps the backend bucket. The controller never
// deletes a Retain bucket, but an admin can, so the policy is checked rather than inferred from
// the deletion timestamp.
func TestBucketDeletion_RetainPolicy(t *testing.T) {
	f := newDeleteFixture(t, cosiapi.BucketDeletionPolicyRetain)

	require.NoError(t, f.reconcile(t))

	assert.Empty(t, f.deleteReqs, "Retain must not reach the driver")
	assert.True(t, f.gone(t), "the object is still released; only the backend bucket survives")
}

// TestBucketDeletion_DriverFailureKeepsFinalizer is the regression guard for the leak this whole
// file exists to fix.
//
// Upstream removed the finalizer and then errored, so the Bucket disappeared while the backend
// bucket stayed, with nothing left in the cluster naming it. The finalizer must survive every
// failure, retryable or not: a stuck object is visible and recoverable, a leaked bucket is
// neither.
func TestBucketDeletion_DriverFailureKeepsFinalizer(t *testing.T) {
	tests := []struct {
		name         string
		code         codes.Code
		wantErr      bool
		wantTerminal bool
	}{
		{
			name:    "retryable failure",
			code:    codes.Unavailable,
			wantErr: true,
		},
		{
			// The non-empty bucket case, and the reason it is not terminal. A driver reporting
			// BucketNotEmpty gets retried with backoff, so emptying the bucket lets the pending
			// deletion complete on its own rather than needing the object to be unwedged.
			name:    "non-empty bucket is retryable",
			code:    codes.FailedPrecondition,
			wantErr: true,
		},
		{
			name:         "terminal failure",
			code:         codes.InvalidArgument,
			wantErr:      true,
			wantTerminal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDeleteFixture(t, cosiapi.BucketDeletionPolicyDelete)
			f.deleteErr = grpcstatus.Error(tt.code, "driver said no")

			err := f.reconcile(t)

			require.Error(t, err)
			require.Len(t, f.deleteReqs, 1)

			// A terminal error stops the reconciler retrying. That is intended for a failure no
			// retry can fix, and is safe only because the finalizer stays put.
			assert.Equal(t, tt.wantTerminal, errors.Is(err, reconcile.TerminalError(nil)))

			assert.False(t, f.gone(t), "the object must not be reaped after a failed deprovision")
			bucket := f.get(t)
			assert.Contains(t, bucket.GetFinalizers(), cosiapi.ProtectionFinalizer,
				"removing the finalizer here is exactly the upstream leak")
			assert.NotNil(t, bucket.Status.Error, "the failure must be visible on the object")
			assert.False(t, *bucket.Status.ReadyToUse)
		})
	}
}

// TestBucketDeletion_RetryAfterFailureSucceeds checks the object is not wedged by a transient
// failure: once the driver recovers, the next reconcile finishes the job.
//
// This is the half of idempotency that matters in practice. The proto requires the driver to
// return OK for an already-deleted bucket, so a retry after a partial failure completes rather
// than erroring on not-found.
func TestBucketDeletion_RetryAfterFailureSucceeds(t *testing.T) {
	f := newDeleteFixture(t, cosiapi.BucketDeletionPolicyDelete)

	f.deleteErr = grpcstatus.Error(codes.Unavailable, "backend briefly unreachable")
	require.Error(t, f.reconcile(t))
	require.False(t, f.gone(t))

	f.deleteErr = nil
	require.NoError(t, f.reconcile(t))

	assert.Len(t, f.deleteReqs, 2, "the driver is called again rather than the failure being cached")
	assert.True(t, f.gone(t))
}

// TestBucketDeletion_UnknownPolicyRefuses covers a Bucket whose deletionPolicy is neither Retain
// nor Delete.
//
// The API server enforces the enum, so this models an object that somehow evaded validation. The
// point is that the ambiguity is resolved by refusing: the two available guesses are "destroy the
// customer's data" and "leak it", and neither is a safe default.
func TestBucketDeletion_UnknownPolicyRefuses(t *testing.T) {
	rpcClient := mustServe(t, &deleteCapableServer{}) // any driver call panics

	bucket := &cosiapi.Bucket{
		ObjectMeta: meta.ObjectMeta{
			Name:       "bc-badpolicy",
			Finalizers: []string{cosiapi.ProtectionFinalizer},
		},
		Spec: cosiapi.BucketSpec{
			DriverName:     testDriverName,
			DeletionPolicy: "",
			Protocols:      []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
			BucketClaimRef: cosiapi.BucketClaimReference{
				Name: "my-bucket", Namespace: "my-ns", UID: "qwerty",
			},
		},
	}

	deps := cositest.MustBootstrap(t, bucket)
	ctx := deps.ContextWithLogger
	nsName := cositest.NsName(bucket)

	r := BucketReconciler{
		Client: deps.Client,
		Scheme: deps.Client.Scheme(),
		DriverInfo: DriverInfo{
			Name:               testDriverName,
			SupportedProtocols: []cosiproto.ObjectProtocol_Type{cosiproto.ObjectProtocol_S3},
			ProvisionerClient:  rpcClient,
		},
	}

	require.NoError(t, deps.Client.Delete(ctx, bucket.DeepCopy()))

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nsName})
	require.Error(t, err)
	assert.True(t, errors.Is(err, reconcile.TerminalError(nil)), "no retry can resolve an invalid policy")

	got := &cosiapi.Bucket{}
	require.NoError(t, r.Get(ctx, nsName, got), "the object must survive so the state is visible")
	assert.Contains(t, got.GetFinalizers(), cosiapi.ProtectionFinalizer)
}

// TestBucketDeletion_StaticProvisioningHonoursDelete pins a data-destruction path that is easy to
// overlook: a Bucket adopted via spec.existingBucketID, carrying deletionPolicy Delete, is
// deprovisioned on delete even though Trident never created the backend bucket.
//
// This is deliberate and matches Kubernetes convention -- a pre-provisioned PersistentVolume with
// reclaimPolicy Delete removes its backing volume too -- but it means an admin who adopts an
// existing bucket with the wrong policy loses it. Pinned rather than left implicit.
func TestBucketDeletion_StaticProvisioningHonoursDelete(t *testing.T) {
	var deleteReqs []*cosiproto.DriverDeleteBucketRequest

	server := &deleteCapableServer{
		FakeProvisionerServer: cositest.FakeProvisionerServer{
			GetBucketFunc: func(
				_ context.Context, req *cosiproto.DriverGetBucketRequest,
			) (*cosiproto.DriverGetBucketResponse, error) {
				return &cosiproto.DriverGetBucketResponse{
					BucketId: req.BucketId,
					Protocols: &cosiproto.ObjectProtocolAndBucketInfo{
						S3: &cosiproto.S3BucketInfo{
							Endpoint:        "s3.corp.net",
							BucketId:        req.BucketId,
							Region:          "us-east-1",
							AddressingStyle: &cosiproto.S3AddressingStyle{Style: cosiproto.S3AddressingStyle_PATH},
						},
					},
				}, nil
			},
		},
		deleteFunc: func(
			_ context.Context, req *cosiproto.DriverDeleteBucketRequest,
		) (*cosiproto.DriverDeleteBucketResponse, error) {
			deleteReqs = append(deleteReqs, req)
			return &cosiproto.DriverDeleteBucketResponse{}, nil
		},
	}

	rpcClient := mustServe(t, server)

	bucket := &cosiapi.Bucket{
		ObjectMeta: meta.ObjectMeta{Name: "bc-adopted"},
		Spec: cosiapi.BucketSpec{
			DriverName:       testDriverName,
			DeletionPolicy:   cosiapi.BucketDeletionPolicyDelete,
			ExistingBucketID: "pre-existing-bucket",
			Protocols:        []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
			BucketClaimRef: cosiapi.BucketClaimReference{
				Name: "my-bucket", Namespace: "my-ns", UID: "qwerty",
			},
		},
	}

	deps := cositest.MustBootstrap(t, bucket)
	ctx := deps.ContextWithLogger
	nsName := cositest.NsName(bucket)

	r := BucketReconciler{
		Client: deps.Client,
		Scheme: deps.Client.Scheme(),
		DriverInfo: DriverInfo{
			Name:               testDriverName,
			SupportedProtocols: []cosiproto.ObjectProtocol_Type{cosiproto.ObjectProtocol_S3},
			ProvisionerClient:  rpcClient,
		},
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nsName})
	require.NoError(t, err)

	adopted := &cosiapi.Bucket{}
	require.NoError(t, r.Get(ctx, nsName, adopted))
	require.Equal(t, "pre-existing-bucket", adopted.Status.BucketID)

	require.NoError(t, deps.Client.Delete(ctx, adopted.DeepCopy()))

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: nsName})
	require.NoError(t, err)

	require.Len(t, deleteReqs, 1, "an adopted bucket under Delete is still deprovisioned")
	assert.Equal(t, "pre-existing-bucket", deleteReqs[0].BucketId)
}

// TestBucketDeletion_NoBucketIDSkipsDriver covers a Bucket whose provisioning never completed.
// There is nothing on the backend to remove, and calling the driver with an empty ID would be
// rejected as an invalid argument, wedging the object behind its finalizer forever.
func TestBucketDeletion_NoBucketIDSkipsDriver(t *testing.T) {
	server := &deleteCapableServer{} // any driver call panics

	rpcClient := mustServe(t, server)

	bucket := &cosiapi.Bucket{
		ObjectMeta: meta.ObjectMeta{
			Name:       "bc-unprovisioned",
			Finalizers: []string{cosiapi.ProtectionFinalizer},
		},
		Spec: cosiapi.BucketSpec{
			DriverName:     testDriverName,
			DeletionPolicy: cosiapi.BucketDeletionPolicyDelete,
			Protocols:      []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
			BucketClaimRef: cosiapi.BucketClaimReference{
				Name:      "my-bucket",
				Namespace: "my-ns",
				UID:       "qwerty",
			},
		},
	}

	deps := cositest.MustBootstrap(t, bucket)
	ctx := deps.ContextWithLogger
	nsName := cositest.NsName(bucket)

	r := BucketReconciler{
		Client: deps.Client,
		Scheme: deps.Client.Scheme(),
		DriverInfo: DriverInfo{
			Name:               testDriverName,
			SupportedProtocols: []cosiproto.ObjectProtocol_Type{cosiproto.ObjectProtocol_S3},
			ProvisionerClient:  rpcClient,
		},
	}

	require.NoError(t, deps.Client.Delete(ctx, bucket.DeepCopy()))

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: nsName})
	require.NoError(t, err)

	assert.Error(t, r.Get(ctx, nsName, &cosiapi.Bucket{}),
		"an unprovisioned Bucket should be released without a driver call")
}
