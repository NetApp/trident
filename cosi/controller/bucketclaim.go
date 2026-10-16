/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	cosierr "github.com/netapp/trident/cosi/third_party/errors"
	cosipredicate "github.com/netapp/trident/cosi/third_party/predicate"
)

// BucketClaimReconciler reconciles a BucketClaim object
type BucketClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=bucketclaims,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=bucketclaims/status,verbs=get;update
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=bucketclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=buckets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BucketClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	claim := &cosiapi.BucketClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if kerrors.IsNotFound(err) {
			logger.V(1).Info("not reconciling nonexistent BucketClaim")
			return ctrl.Result{}, nil
		}
		// no resource to add status to or report an event for
		logger.Error(err, "failed to get BucketClaim")
		return ctrl.Result{}, err
	}

	err := r.reconcile(ctx, logger, claim)
	if err != nil {
		statusChanged := false
		if claim.Status.ReadyToUse == nil {
			claim.Status.ReadyToUse = ptr.To(false)
			statusChanged = true
		}
		message := err.Error()
		if claim.Status.Error == nil || claim.Status.Error.Time == nil ||
			claim.Status.Error.Message == nil || *claim.Status.Error.Message != message {
			claim.Status.Error = cosiapi.NewTimestampedError(time.Now(), message)
			statusChanged = true
		}
		if statusChanged {
			if updErr := r.Status().Update(ctx, claim); updErr != nil {
				logger.Error(err, "failed to update BucketClaim status after reconcile error", "updateError", updErr)
				return reconcile.Result{}, err
			}
		}

		if errors.Is(err, cosierr.NonRetryableError(nil)) {
			return reconcile.Result{}, reconcile.TerminalError(err)
		}
		return reconcile.Result{}, err
	}

	// On success, clear any errors in the status.
	if claim.Status.Error != nil && !claim.DeletionTimestamp.IsZero() {
		if claim.Status.ReadyToUse == nil {
			claim.Status.ReadyToUse = ptr.To(false)
		}
		claim.Status.Error = nil
		if err := r.Status().Update(ctx, claim); err != nil {
			logger.Error(err, "failed to update BucketClaim status after reconcile success")
			// Retry the reconcile so status can be updated eventually.
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cosiapi.BucketClaim{}).
		WithEventFilter(
			ctrlpredicate.Or( //
				// this is the only bucketclaim controller and should reconcile ALL Create/Delete/Generic events
				cosipredicate.AnyCreate(),
				cosipredicate.AnyDelete(),
				cosipredicate.AnyGeneric(),
				// opt in to desired Update events
				cosipredicate.GenerationChangedInUpdateOnly(), // reconcile spec changes
				cosipredicate.DeletionTimestampAdded(),
				cosipredicate.ProtectionFinalizerRemoved(r.Scheme), // re-add protection finalizer if removed
			),
		).
		// Bucket has no OwnerReference back to BucketClaim, so .Owns() can't be used; map Bucket
		// events to the referencing BucketClaim instead.
		Watches(
			&cosiapi.Bucket{},
			handler.EnqueueRequestsFromMapFunc(mapBucketToBucketClaim),
			builder.WithPredicates(ctrlpredicate.Or(cosipredicate.AnyCreate(), cosipredicate.AnyDelete())),
		).
		Named("bucketclaim").
		Complete(r)
}

// mapBucketToBucketClaim is a handler.MapFunc that, given a Bucket event, returns a reconcile.Request
// for the BucketClaim it references. This lets a Bucket watch re-trigger the BucketClaim waiting on it.
// bucketClaimRef.name/namespace are required on every Bucket (static or dynamic), so no lookup is needed.
func mapBucketToBucketClaim(ctx context.Context, obj client.Object) []reconcile.Request {
	bucket, ok := obj.(*cosiapi.Bucket)
	if !ok {
		ctrl.LoggerFrom(ctx).Error(nil, "mapBucketToBucketClaim received non-Bucket object", "obj", obj)
		return nil
	}

	claimRef := bucket.Spec.BucketClaimRef
	if claimRef.Name == "" || claimRef.Namespace == "" {
		// Shouldn't happen: the CRD schema requires both fields.
		ctrl.LoggerFrom(ctx).V(1).Info("Bucket has no bucketClaimRef name/namespace",
			"bucketName", bucket.Name, "claimRefName", claimRef.Name, "claimRefNamespace", claimRef.Namespace)
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: claimRef.Name, Namespace: claimRef.Namespace}},
	}
}

func (r *BucketClaimReconciler) reconcile(ctx context.Context, logger logr.Logger, claim *cosiapi.BucketClaim) error {
	bucketName, err := determineBucketName(claim)
	if err != nil {
		// Opinion: It is best to not apply a missing finalizer when boundBucketName is degraded
		// (err returned here). When degraded, the user needs to delete and re-create the
		// BucketClaim to fix the degradation, which requires the finalizer be absent.
		logger.Error(err, "failed to determine Bucket name for claim")
		return cosierr.NonRetryableError(err)
	}

	logger = logger.WithValues("bucketName", bucketName)

	isStaticProvisioning := claim.Spec.ExistingBucketName != ""

	if !claim.GetDeletionTimestamp().IsZero() {
		logger.Info("beginning BucketClaim deletion")
		return r.reconcileDelete(ctx, logger, claim, bucketName, isStaticProvisioning)
	}

	logger.V(1).Info("reconciling BucketClaim")

	didAdd := ctrlutil.AddFinalizer(claim, cosiapi.ProtectionFinalizer)
	if didAdd {
		if err := r.Update(ctx, claim); err != nil {
			logger.Error(err, "failed to add protection finalizer")
			return fmt.Errorf("failed to add protection finalizer: %w", err)
		}
	}

	bucket := &cosiapi.Bucket{}
	bucketNsName := types.NamespacedName{
		Name:      bucketName,
		Namespace: "", // global resource
	}
	if err := r.Get(ctx, bucketNsName, bucket); err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Error(err, "failed to determine if Bucket exists")
			return err
		}

		if claim.Status.BoundBucketName != "" {
			// Recreating the intermediate bucket could lead to data loss
			// because it may have different StorageClass configurations
			logger.Error(nil, "BucketClaim is bound to a Bucket that no longer exists")
			return cosierr.NonRetryableError(fmt.Errorf(
				"unrecoverable degradation: BucketClaim is bound to Bucket %q that no longer exists",
				claim.Status.BoundBucketName))
		}

		if isStaticProvisioning {
			// Bucket not created yet. The Bucket watch (see SetupWithManager)
			// is responsible for re-enqueuing this BucketClaim once the Bucket is created.
			logger.Info("waiting for statically-provisioned Bucket to be created")
			return nil
		}

		// Claim is not bound yet, this is normal dynamic provisioning.
		logger.Info("creating intermediate Bucket")
		bucket, err = createIntermediateBucket(ctx, logger, r.Client, claim, bucketName)
		if err != nil {
			return err
		}
	}

	isBound, err := bucketIsBoundToClaim(bucket, claim)
	if err != nil {
		logger.Error(err, "Bucket binding does not match BucketClaim")
		return cosierr.NonRetryableError(err)
	}
	if !isBound {
		if isStaticProvisioning {
			if err := ensureStaticBucketBound(ctx, logger, r.Client, bucket, claim.UID); err != nil {
				return err
			}
		} else {
			logger.Error(nil, "dynamic Bucket is malformed")
			return cosierr.NonRetryableError(fmt.Errorf("dynamic Bucket is malformed: %w", err))
		}
	}

	// Now that Bucket exists, bind the BucketClaim to it (if not already bound).
	if claim.Status.BoundBucketName == "" {
		logger.Info("binding BucketClaim to Bucket")
		if claim.Status.ReadyToUse == nil {
			claim.Status.ReadyToUse = ptr.To(false)
		}
		claim.Status.BoundBucketName = bucketName
		if err := r.Status().Update(ctx, claim); err != nil {
			logger.Error(err, "failed to bind BucketClaim to Bucket")
			return fmt.Errorf("failed to bind BucketClaim to Bucket: %w", err)
		}
	}

	if bucket.Status.BucketID == "" {
		// TODO: In the future, set up Bucket watcher to enqueue this BucketClaim when the Bucket
		// is updated. For now, return error to requeue with backoff.
		logger.Info("waiting for Bucket to be provisioned")
		return fmt.Errorf("waiting for Bucket to be provisioned")
	}

	readyToUse := ptr.Deref(bucket.Status.ReadyToUse, false)
	if readyToUse && len(bucket.Status.Protocols) == 0 {
		logger.Error(nil, "provisioned Bucket supports no protocols")
		return cosierr.NonRetryableError(fmt.Errorf("provisioned Bucket supports no protocols"))
	}

	claim.Status.ReadyToUse = bucket.Status.ReadyToUse
	claim.Status.Protocols = bucket.Status.Protocols
	claim.Status.Error = nil
	if err := r.Status().Update(ctx, claim); err != nil {
		logger.Error(err, "failed to update BucketClaim status after successful provisioning")
		return err
	}

	return nil
}

func (r *BucketClaimReconciler) reconcileDelete(
	ctx context.Context, logger logr.Logger,
	claim *cosiapi.BucketClaim,
	bucketName string,
	isStaticProvisioning bool,
) error {
	claim.Status.ReadyToUse = ptr.To(false)
	claim.Status.Error = nil // previous error is no longer relevant
	if err := r.Status().Update(ctx, claim); err != nil {
		logger.Error(err, "failed to update BucketClaim status before deletion")
		return fmt.Errorf("failed to update BucketClaim status before deletion: %w", err)
	}

	bucket := &cosiapi.Bucket{}
	bucketNsName := types.NamespacedName{
		Name:      bucketName,
		Namespace: "", // global resource
	}
	if err := r.Get(ctx, bucketNsName, bucket); err != nil {
		if kerrors.IsNotFound(err) {
			// Bucket doesn't exist
			logger.Info("removing finalizer from BucketClaim with deleted or nonexistent Bucket")
			return r.removeClaimFinalizer(ctx, logger, claim)
		} else {
			logger.Error(err, "failed to determine if Bucket exists")
			return err
		}
	}

	isBound, err := bucketIsBoundToClaim(bucket, claim)
	if err != nil {
		if isStaticProvisioning {
			// BucketClaim was made with a reference to a Bucket already bound to another claim.
			// Allow the claim to delete so the user can try again.
			logger.Info(
				"removing finalizer from static BucketClaim which references a Bucket already bound to a different BucketClaim") // nolint:lll
			return r.removeClaimFinalizer(ctx, logger, claim)
		}
		// It might be safe to delete the claim, but we can't be sure. It is safest to require the
		// admin to decide what to do, to ensure no system info is lost in the worst case.
		errMsg := "administrator must resolve unexpected error: dynamic Bucket does not reference this BucketClaim"
		logger.Error(err, errMsg)
		return cosierr.NonRetryableError(fmt.Errorf("%s: %w", errMsg, err))
	}

	if !isBound { // implies static provisioning because dynamic buckets are bound at initial creation
		logger.Info("removing finalizer from static BucketClaim which references an unbound Bucket")
		return r.removeClaimFinalizer(ctx, logger, claim)
	}

	// NETAPP LOCAL: upstream hardcodes BucketDeletionPolicyRetain here regardless of the
	// bucket's actual policy, so every log line below claims "Retain" even on the Delete path.
	logger = logger.WithValues("bucketDeletionPolicy", bucket.Spec.DeletionPolicy)

	switch bucket.Spec.DeletionPolicy {
	case cosiapi.BucketDeletionPolicyRetain:
		if err := r.applyBucketClaimIsDeletingAnnotation(ctx, logger, bucket); err != nil {
			return err
		}
		return r.removeClaimFinalizer(ctx, logger, claim)

	case cosiapi.BucketDeletionPolicyDelete:
		if !bucket.DeletionTimestamp.IsZero() {
			logger.Info("still waiting for Bucket to be deleted")
			// TODO: return nil when Bucket watcher is set up
			return fmt.Errorf("still waiting for Bucket to be deleted")
		}

		if err := r.applyBucketClaimIsDeletingAnnotation(ctx, logger, bucket); err != nil {
			return err
		}

		if err := r.Delete(ctx, bucket); err != nil {
			logger.Error(err, "failed to delete Bucket")
			return fmt.Errorf("failed to delete Bucket: %w", err)
		}

		logger.Info("waiting for Bucket to be deleted")
		// TODO: return nil when Bucket watcher is set up
		return fmt.Errorf("waiting for Bucket to be deleted")
		// once Bucket is deleted, a future reconcile will remove the BucketClaim finalizer

	default:
		logger.Error(nil, "unknown Bucket deletion policy", "deletionPolicy", bucket.Spec.DeletionPolicy)
		return cosierr.NonRetryableError(fmt.Errorf("unknown Bucket deletion policy %q", bucket.Spec.DeletionPolicy))
	}
}

func (r *BucketClaimReconciler) removeClaimFinalizer(
	ctx context.Context, logger logr.Logger, claim *cosiapi.BucketClaim,
) error {
	ctrlutil.RemoveFinalizer(claim, cosiapi.ProtectionFinalizer)
	if err := r.Update(ctx, claim); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return fmt.Errorf("failed to remove finalizer: %w", err)
	}
	return nil
}

func (r *BucketClaimReconciler) applyBucketClaimIsDeletingAnnotation(
	ctx context.Context, logger logr.Logger, bucket *cosiapi.Bucket,
) error {
	if bucket.Annotations == nil {
		bucket.Annotations = map[string]string{}
	}
	bucket.Annotations[cosiapi.BucketClaimBeingDeletedAnnotation] = ""
	if err := r.Update(ctx, bucket); err != nil {
		logger.Error(err, "failed to annotate Bucket to indicate BucketClaim is being deleted")
		return fmt.Errorf("failed to annotate Bucket to indicate BucketClaim is being deleted: %w", err)
	}
	return nil
}

// Determine the bucket name that should go with the claim. No errors can be retried.
func determineBucketName(claim *cosiapi.BucketClaim) (string, error) {
	name := ""

	if claim.Spec.ExistingBucketName != "" {
		// Case: Static provisioning
		name = claim.Spec.ExistingBucketName
	} else {
		// Case: Dynamic provisioning
		name = "bc-" + string(claim.UID) // DO NOT CHANGE UNLESS ABSOLUTELY NECESSARY
		// ^ boundBucketName could become the source of truth to technically allow changing this.
		// However, keeping this consistent will make it possible to recover from loss of binding info
		// due to unexpected system issues without having to perform deeper system inspection.
	}

	if name == "" { // catch developer error
		return "", fmt.Errorf("internal error: determined bucket name is empty")
	}

	// Bound name should match whatever was determined above. Divergence shouldn't happen normally.
	// In case of a disaster that lost original objects, the user may re-create them, possibly with
	// mistakes. In such a case, COSI can't be certain which name is correct.
	if claim.Status.BoundBucketName != "" && claim.Status.BoundBucketName != name {
		return "", fmt.Errorf("unrecoverable degradation: boundBucketName %q does not match determined name %q",
			claim.Status.BoundBucketName, name)
	}

	return name, nil
}

func createIntermediateBucket(
	ctx context.Context,
	logger logr.Logger,
	client client.Client,
	claim *cosiapi.BucketClaim,
	bucketName string,
) (*cosiapi.Bucket, error) {
	className := claim.Spec.BucketClassName
	if className == "" {
		logger.Error(nil, "BucketClaim cannot have empty bucketClassName")
		return nil, cosierr.NonRetryableError(fmt.Errorf("BucketClaim cannot have empty bucketClassName"))
	}

	logger = logger.WithValues("bucketClassName", className)

	class := &cosiapi.BucketClass{}
	classNsName := types.NamespacedName{
		Name:      className,
		Namespace: "", // global resource
	}
	if err := client.Get(ctx, classNsName, class); err != nil {
		if kerrors.IsNotFound(err) {
			// TODO: for now, return an error and allow the controller to exponential backoff
			// until the BucketClass exists. in the future, optimize this by adding a
			// BucketClass reconciler that enqueues requests for BucketClaims that reference the
			// class and don't yet have a bound Bucket.
			logger.Error(err, "BucketClass not found")
			return nil, err
		}
		logger.Error(err, "failed to get BucketClass")
		return nil, err
	}

	logger.V(1).Info("using BucketClass for intermediate Bucket")

	bucket := generateIntermediateBucket(claim, class, bucketName)

	if err := client.Create(ctx, bucket); err != nil {
		if kerrors.IsAlreadyExists(err) {
			// Unlikely race condition. Error to allow the next reconcile to attempt to recover.
			logger.Error(err, "intermediate Bucket already exists")
			return nil, err
		}
		logger.Error(err, "failed to create intermediate Bucket")
		return nil, err
	}

	return bucket, nil
}

func generateIntermediateBucket(
	claim *cosiapi.BucketClaim, class *cosiapi.BucketClass, bucketName string,
) *cosiapi.Bucket {
	return &cosiapi.Bucket{
		ObjectMeta: meta.ObjectMeta{
			Name: bucketName,
			// Do not pre-apply protection finalizer here. Sidecar is responsible for the finalizer.
			// If Sidecar (driver) isn't running or driver name is incorrect, user needs to be able
			// to delete the claim, and COSI needs to delete the intermediate Bucket which hasn't
			// had any backend resources created for the Bucket.
			Finalizers: []string{ /* PURPOSEFULLY EMPTY */ },
		},
		Spec: cosiapi.BucketSpec{
			DriverName:     class.Spec.DriverName,
			DeletionPolicy: class.Spec.DeletionPolicy,
			Parameters:     class.Spec.Parameters,
			Protocols:      claim.Spec.Protocols,
			BucketClaimRef: cosiapi.BucketClaimReference{
				Name:      claim.Name,
				Namespace: claim.Namespace,
				UID:       claim.UID,
			},
		},
		Status: cosiapi.BucketStatus{},
	}
}

func bucketIsBoundToClaim(bucket *cosiapi.Bucket, claim *cosiapi.BucketClaim) (bool, error) {
	errs := []error{}

	claimRef := bucket.Spec.BucketClaimRef

	if claimRef.Namespace != claim.Namespace {
		//nolint:staticcheck // ST1005: okay to capitalize resource kind
		errs = append(errs, fmt.Errorf("Bucket claim ref namespace %q does not match BucketClaim namespace %q",
			claimRef.Namespace, claim.Namespace))
	}

	if claimRef.Name != claim.Name {
		//nolint:staticcheck // ST1005: okay to capitalize resource kind
		errs = append(errs, fmt.Errorf("Bucket claim ref name %q does not match BucketClaim name %q",
			claimRef.Name, claim.Name))
	}

	if string(claimRef.UID) == "" { // bucket is not bound
		if len(errs) > 0 {
			return false, fmt.Errorf("unbound Bucket does not match BucketClaim: %w", errors.Join(errs...))
		}
		return false, nil
	}

	if claimRef.UID != claim.UID {
		//nolint:staticcheck // ST1005: okay to capitalize resource kind
		errs = append(errs, fmt.Errorf("Bucket claim ref UID %q does not match BucketClaim UID %q",
			claimRef.UID, claim.UID))
	}

	if len(errs) > 0 {
		return false, fmt.Errorf("bound Bucket does not match BucketClaim: %w", errors.Join(errs...))
	}
	return true, nil
}

func ensureStaticBucketBound(
	ctx context.Context,
	logger logr.Logger,
	client client.Client,
	bucket *cosiapi.Bucket,
	claimUID types.UID,
) error {
	claimRef := &bucket.Spec.BucketClaimRef

	// already bound to this BucketClaim
	if claimRef.UID == claimUID {
		return nil
	}

	// means the Bucket was once bound to a different BucketClaim
	// COSI explicitly (for data integrity and security) does not allow re-binding Buckets
	if claimRef.UID != "" && claimRef.UID != claimUID {
		logger.Error(nil, "Bucket claim ref UID does not match BucketClaim UID")
		//nolint:staticcheck // ST1005: okay to capitalize resource kind
		return cosierr.NonRetryableError(fmt.Errorf(
			"Bucket claim ref UID %q does not match BucketClaim UID %q",
			claimRef.UID, claimUID))
	}

	// safe to bind the Bucket to this BucketClaim
	logger.Info("binding statically-provisioned Bucket to BucketClaim", "claimUID", claimUID)
	bucket.Spec.BucketClaimRef.UID = claimUID
	if err := client.Update(ctx, bucket); err != nil {
		logger.Error(err, "failed to set bucketClaimRef.UID on Bucket")
		return fmt.Errorf("failed to set bucketClaimRef.UID on Bucket: %w", err)
	}
	return nil
}
