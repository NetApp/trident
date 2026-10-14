// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file is NETAPP LOCAL, absent from cosi/upstream.manifest, so resync.sh ignores it.
//
// Upstream's Bucket reconciler has no deletion logic: it removes the protection finalizer and
// errors, orphaning the backend bucket. This file implements deletion instead, kept separate so
// the vendored bucket.go diverges by only a single call site. Semantics are specified in
// design-docs/cosi/07-deletion-semantics.md.

package reconciler

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"google.golang.org/grpc/status"
	"k8s.io/utils/ptr"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	cosierr "github.com/netapp/trident/cosi/third_party/errors"
)

// reconcileDelete deprovisions a Bucket that has a deletion timestamp, then releases it.
//
// The finalizer comes off only after the driver confirms the backend bucket is gone; every
// failure path below returns without touching it, so a failed deprovision stays visible instead
// of leaking silently.
func (r *BucketReconciler) reconcileDelete(
	ctx context.Context, logger logr.Logger, bucket *cosiapi.Bucket,
) error {
	logger.V(1).Info("beginning Bucket deletion cleanup")

	bucket.Status.ReadyToUse = ptr.To(false)
	bucket.Status.Error = nil
	if err := r.Status().Update(ctx, bucket); err != nil {
		logger.Error(err, "failed to update Bucket status before deletion")
		return fmt.Errorf("failed to update Bucket status before deletion: %w", err)
	}

	if err := r.deprovisionBucket(ctx, logger, bucket); err != nil {
		return err
	}

	ctrlutil.RemoveFinalizer(bucket, cosiapi.ProtectionFinalizer)
	if err := r.Update(ctx, bucket); err != nil {
		logger.Error(err, "failed to remove protection finalizer")
		return fmt.Errorf("failed to remove protection finalizer: %w", err)
	}

	return nil
}

// deprovisionBucket removes the backend bucket per the deletion policy. Returning nil means
// there is nothing left to deprovision, not that a bucket was deleted.
func (r *BucketReconciler) deprovisionBucket(
	ctx context.Context, logger logr.Logger, bucket *cosiapi.Bucket,
) error {
	switch bucket.Spec.DeletionPolicy {
	case cosiapi.BucketDeletionPolicyRetain:
		// An admin can delete a Retain bucket directly, so the policy is checked here rather
		// than inferred from the deletion timestamp.
		logger.Info("not deprovisioning backend bucket; deletionPolicy is Retain",
			"bucketID", bucket.Status.BucketID)
		return nil

	case cosiapi.BucketDeletionPolicyDelete:

	default:
		// deletionPolicy is enum-constrained to Retain or Delete; reaching this means the object
		// bypassed validation. Refuse rather than guess between leaking and destroying data.
		logger.Error(nil, "unknown Bucket deletion policy", "deletionPolicy", bucket.Spec.DeletionPolicy)
		return cosierr.NonRetryableError(
			fmt.Errorf("unknown Bucket deletion policy %q", bucket.Spec.DeletionPolicy))
	}

	if bucket.Status.BucketID == "" {
		// No recorded bucketID means provisioning never completed, so there is nothing on the
		// backend to remove. Mirrors the guard in the BucketAccess path.
		logger.Info("not deprovisioning backend bucket with no recorded bucketID")
		return nil
	}

	logger.Info("calling driver to delete bucket", "bucketID", bucket.Status.BucketID)

	return driverDeleteBucket(ctx, logger, r.DriverInfo.ProvisionerClient, bucket)
}

// driverDeleteBucket calls the driver, translating a transport error into a retry decision. The
// proto requires OK for an already-deleted bucket, which is what makes retrying safe here.
func driverDeleteBucket(
	ctx context.Context,
	logger logr.Logger,
	rpcClient cosiproto.ProvisionerClient,
	bucket *cosiapi.Bucket,
) error {
	_, err := rpcClient.DriverDeleteBucket(ctx,
		&cosiproto.DriverDeleteBucketRequest{
			BucketId:   bucket.Status.BucketID,
			Parameters: bucket.Spec.Parameters,
		},
	)
	if err != nil {
		logger.Error(err, "DriverDeleteBucket error")
		if rpcErrorIsRetryable(status.Code(err)) {
			return err
		}
		// Terminal for InvalidArgument, AlreadyExists, Unimplemented; everything else retries,
		// which is what a non-empty bucket needs. The finalizer stays either way, so an admin
		// can patch deletionPolicy to Retain if the backend bucket should survive.
		return cosierr.NonRetryableError(err)
	}

	return nil
}
