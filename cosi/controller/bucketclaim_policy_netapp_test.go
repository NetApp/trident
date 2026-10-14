// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file is NETAPP LOCAL. It is not vendored from upstream and is absent from
// cosi/upstream.manifest, so resync.sh ignores it.
//
// It pins how a Bucket acquires its deletionPolicy, which became load-bearing when Trident
// implemented deprovisioning: the delete path in cosi/sidecar/bucket_delete.go honours the
// recorded policy faithfully, so a Bucket carrying the wrong one leaks or destroys storage
// with no other signal that anything went wrong.
//
// It also serves as the disproof of a bug that design-docs/cosi/07-deletion-semantics.md
// listed as a GA gate: "a Bucket created while its BucketClass is missing gets Retain and is
// never corrected once the class appears". No Bucket is created in that window at all, so
// there is nothing to correct. See TestBucketPolicyFromLateBucketClass.

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	cositest "github.com/netapp/trident/cosi/third_party/test"
)

// TestBucketPolicyFromLateBucketClass reconciles a BucketClaim whose BucketClass does not exist
// yet, then creates the class and reconciles again.
//
// The reported failure was that the intermediate Bucket is created during the gap with a
// defaulted Retain and never revised. What actually happens is that the reconcile errors and
// creates nothing, so the Bucket is first built after the class exists and takes its policy
// from it.
func TestBucketPolicyFromLateBucketClass(t *testing.T) {
	claim := &cosiapi.BucketClaim{
		ObjectMeta: meta.ObjectMeta{Name: "my-bucket", Namespace: "my-ns", UID: types.UID("qwerty")},
		Spec: cosiapi.BucketClaimSpec{
			BucketClassName: "late-class",
			Protocols:       []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
		},
	}

	deps := cositest.MustBootstrap(t, claim)
	ctx := deps.ContextWithLogger

	r := BucketClaimReconciler{Client: deps.Client, Scheme: deps.Client.Scheme()}
	claimNsName := types.NamespacedName{Name: "my-bucket", Namespace: "my-ns"}
	bucketNsName := types.NamespacedName{Name: "bc-qwerty"}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: claimNsName})
	require.Error(t, err, "a missing BucketClass must fail the reconcile so it is retried")

	assert.Error(t, r.Get(ctx, bucketNsName, &cosiapi.Bucket{}),
		"no Bucket may be created while the policy is unknown; this is what makes the reported downgrade impossible")

	class := &cosiapi.BucketClass{
		ObjectMeta: meta.ObjectMeta{Name: "late-class"},
		Spec: cosiapi.BucketClassSpec{
			DriverName:     "cosi.s3.corp.net",
			DeletionPolicy: cosiapi.BucketDeletionPolicyDelete,
		},
	}
	require.NoError(t, deps.Client.Create(ctx, class))

	// Errors because the Bucket is not provisioned yet, which is a later stage than the one
	// under test; the Bucket itself has been created by this point.
	_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: claimNsName})

	bucket := &cosiapi.Bucket{}
	require.NoError(t, r.Get(ctx, bucketNsName, bucket))
	assert.Equal(t, cosiapi.BucketDeletionPolicyDelete, bucket.Spec.DeletionPolicy,
		"the Bucket must carry the policy the user asked for")
}

// TestBucketPolicyIgnoresLaterBucketClassEdit records that a Bucket captures its policy when it
// is created and never re-reads the class, exactly as a PersistentVolume captures reclaimPolicy
// from its StorageClass at bind time.
//
// Note the fake client permits an edit that a real API server does not: BucketClass.spec is
// immutable, confirmed live on 2026-08-31 with "BucketClass spec is immutable". So this scenario
// cannot arise in a cluster at all, which is a second independent reason the reported
// deletion-policy downgrade is impossible. The property is still worth pinning, because the
// delete path trusts the policy recorded on the Bucket.
//
// An admin who needs to change the policy of an existing bucket edits the Bucket itself, whose
// deletionPolicy is deliberately mutable for this purpose.
func TestBucketPolicyIgnoresLaterBucketClassEdit(t *testing.T) {
	claim := &cosiapi.BucketClaim{
		ObjectMeta: meta.ObjectMeta{Name: "my-bucket", Namespace: "my-ns", UID: types.UID("qwerty")},
		Spec: cosiapi.BucketClaimSpec{
			BucketClassName: "some-class",
			Protocols:       []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
		},
	}
	class := &cosiapi.BucketClass{
		ObjectMeta: meta.ObjectMeta{Name: "some-class"},
		Spec: cosiapi.BucketClassSpec{
			DriverName:     "cosi.s3.corp.net",
			DeletionPolicy: cosiapi.BucketDeletionPolicyRetain,
		},
	}

	deps := cositest.MustBootstrap(t, claim, class)
	ctx := deps.ContextWithLogger

	r := BucketClaimReconciler{Client: deps.Client, Scheme: deps.Client.Scheme()}
	claimNsName := types.NamespacedName{Name: "my-bucket", Namespace: "my-ns"}
	bucketNsName := types.NamespacedName{Name: "bc-qwerty"}

	_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: claimNsName})

	bucket := &cosiapi.Bucket{}
	require.NoError(t, r.Get(ctx, bucketNsName, bucket))
	require.Equal(t, cosiapi.BucketDeletionPolicyRetain, bucket.Spec.DeletionPolicy)

	class.Spec.DeletionPolicy = cosiapi.BucketDeletionPolicyDelete
	require.NoError(t, deps.Client.Update(ctx, class))

	_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: claimNsName})

	require.NoError(t, r.Get(ctx, bucketNsName, bucket))
	assert.Equal(t, cosiapi.BucketDeletionPolicyRetain, bucket.Spec.DeletionPolicy,
		"an existing Bucket keeps the policy it was created with")
}
