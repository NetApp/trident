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
	"slices"
	"time"

	"github.com/go-logr/logr"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	"github.com/netapp/trident/cosi/third_party/bucketaccess"
	cosierr "github.com/netapp/trident/cosi/third_party/errors"
	cosipredicate "github.com/netapp/trident/cosi/third_party/predicate"
)

// BucketAccessReconciler reconciles a BucketAccess object
type BucketAccessReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=bucketaccesses,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=bucketaccesses/status,verbs=get;update
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=bucketaccesses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BucketAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	access := &cosiapi.BucketAccess{}
	if err := r.Get(ctx, req.NamespacedName, access); err != nil {
		if kerrors.IsNotFound(err) {
			logger.V(1).Info("not reconciling nonexistent BucketAccess")
			return ctrl.Result{}, nil
		}
		// no resource to add status to or report an event for
		logger.Error(err, "failed to get BucketAccess")
		return ctrl.Result{}, err
	}

	if bucketaccess.ManagedBySidecar(access) {
		logger.V(1).Info("not reconciling BucketAccess that should be managed by sidecar")
		return ctrl.Result{}, nil
	}

	err := r.reconcile(ctx, logger, access)
	if err != nil {
		// Because the BucketAccess status is could be managed by either Sidecar or Controller,
		// indicate that this error is coming from the Controller.
		err = fmt.Errorf("COSI Controller error: %w", err)

		// Record any error as a timestamped error in the status.
		if access.Status.ReadyToUse == nil {
			access.Status.ReadyToUse = ptr.To(false)
		}
		access.Status.Error = cosiapi.NewTimestampedError(time.Now(), err.Error())
		if updErr := r.Status().Update(ctx, access); updErr != nil {
			logger.Error(err, "failed to update BucketAccess status after reconcile error", "updateError", updErr)
			// If status update fails, we must retry the error regardless of the reconcile return.
			// The reconcile needs to run again to make sure the status is eventually updated.
			return reconcile.Result{}, err
		}

		if errors.Is(err, cosierr.NonRetryableError(nil)) {
			return reconcile.Result{}, reconcile.TerminalError(err)
		}
		return reconcile.Result{}, err
	}

	// NOTE: Do not clear the error in the status on success. Success indicates 1 of 2 things:
	//   1. BucketAccess was initialized successfully, and it's now owned by the Sidecar
	//   2. BucketAccess deletion cleanup was just finished, and no status update is needed

	return reconcile.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cosiapi.BucketAccess{}).
		Named("bucketaccess").
		WithEventFilter(
			ctrlpredicate.And(
				cosipredicate.BucketAccessManagedByController(r.Scheme), // only opt in to reconciles managed by controller
				ctrlpredicate.Or(
					// when managed by controller, we should reconcile ALL Create/Delete/Generic events
					cosipredicate.AnyCreate(),
					cosipredicate.AnyDelete(),
					cosipredicate.AnyGeneric(),
					// opt in to desired update events
					cosipredicate.BucketAccessHandoffOccurred(r.Scheme), // reconcile any handoff change
					cosipredicate.DeletionTimestampAdded(),
					cosipredicate.ProtectionFinalizerRemoved(r.Scheme), // re-add protection finalizer if removed
				),
			),
		).
		Complete(r)
}

func (r *BucketAccessReconciler) reconcile(
	ctx context.Context, logger logr.Logger, access *cosiapi.BucketAccess,
) error {
	if !access.GetDeletionTimestamp().IsZero() {
		logger.Info("beginning BucketAccess deletion")
		return r.reconcileDelete(ctx, logger, access)
	}

	initialized, err := bucketaccess.SidecarRequirementsPresent(&access.Status)
	if err != nil {
		logger.Error(err, "processed a degraded BucketAccess")
		return cosierr.NonRetryableError(fmt.Errorf("processed a degraded BucketAccess: %w", err))
	}
	if initialized {
		// BucketAccessClass info should only be copied to the BucketAccess status once, upon
		// initial provisioning. After the info is copied, make no attempt to fill in any missing or
		// lost info because we don't know whether the current Class is compatible with the info
		// from the existing (old) Class info. If we reach this condition, something is systemically
		// wrong. Sidecar should have ownership, but we determined otherwise, and the Sidecar will
		// likely also determine us to be the owner.
		logger.Error(nil, "processed a BucketAccess that should be managed by COSI Sidecar")
		return cosierr.NonRetryableError(fmt.Errorf("processed a BucketAccess that should be managed by COSI Sidecar"))
	}

	logger = logger.WithValues("bucketAccessClassName", access.Spec.BucketAccessClassName)

	logger.V(1).Info("initializing BucketAccess")

	didAdd := ctrlutil.AddFinalizer(access, cosiapi.ProtectionFinalizer)
	if didAdd {
		if err := r.Update(ctx, access); err != nil {
			logger.Error(err, "failed to add protection finalizer")
			return fmt.Errorf("failed to add protection finalizer: %w", err)
		}
	}

	if err := validateAllBucketClaimReferencesUnique(access.Spec.BucketClaims); err != nil {
		logger.Error(err, "bucketClaims references are not unique")
		return err
	}

	claimsByName, err := getAllBucketClaims(ctx, r.Client, access.Namespace, access.Spec.BucketClaims)
	if err != nil {
		logger.Error(err, "failed to get all referenced BucketClaims")
		return err
	}

	// Mark as many referenced BucketClaims as possible as soon as possible in the reconcile.
	// This ensures that BucketClaims are marked to protect their data from deletion quickly.
	if err := markAllBucketClaimsAsAccessed(ctx, r.Client, claimsByName); err != nil {
		logger.Error(err, "failed to mark all referenced BucketClaims")
		return err
	}

	// HARD STOP IF ALREADY INITIALIZED
	// There may be cases where it is important to reprocess a BucketAccess to resolve a bug or
	// repair BucketClaim access marking, but the full initialization SHOULD NOT be repeated to
	// avoid changing status fields that directly affect the Sidecar's RPC call parameters.
	if len(access.Status.AccessedBuckets) > 0 {
		logger.Info("reprocessed a BucketAccess that is already initialized")
		return nil
	}

	class := &cosiapi.BucketAccessClass{}
	classNsName := types.NamespacedName{
		Name:      access.Spec.BucketAccessClassName,
		Namespace: "", // global resource
	}
	if err := r.Get(ctx, classNsName, class); err != nil {
		if kerrors.IsNotFound(err) {
			// TODO: for now, return an error and allow the controller to exponential backoff
			// until the access class exists. in the future, optimize this by adding a
			// access class reconciler that enqueues requests for BucketAccesses that reference the
			// class and aren't yet passed to the sidecar.
			logger.Error(err, "BucketAccessClass not found")
			return err
		}
		logger.Error(err, "failed to get BucketAccessClass")
		return err
	}

	if err := ValidateAccessAgainstClass(&class.Spec, &access.Spec); err != nil {
		logger.Error(err, "invalid spec against BucketAccessClass requirements")
		return cosierr.NonRetryableError(err)
	}

	bucketsByClaimName, err := getBucketsForClaims(ctx, r.Client, claimsByName)
	if err != nil {
		logger.Error(err, "failed to get Buckets for referenced BucketClaims")
		return err
	}

	blockers := cannotAccessBucketClaims(claimsByName, access.Spec)
	if len(blockers) > 0 {
		logger.Error(nil, "access cannot be provisioned for one or more BucketClaims", "blockers", blockers)
		return cosierr.NonRetryableError(
			fmt.Errorf("access cannot be provisioned for one or more BucketClaims: %v", blockers))
	}

	waitlist := waitingOnBucketClaims(claimsByName, bucketsByClaimName)
	if len(waitlist) > 0 {
		logger.Error(nil, "waiting for prerequisites before provisioning access", "waitlist", waitlist)
		// TODO: for now, return an error and allow the controller to exponential backoff until we
		// are done waiting on the resources. in the future, optimize this by adding a bucketclaim
		// reconciler that enqueues requests for BucketClaims when they finish provisioning.
		return fmt.Errorf("waiting for prerequisites before provisioning access: %v", waitlist)
	}

	accessedBuckets, err := compileAccessedBuckets(access.Spec.BucketClaims, bucketsByClaimName)
	if err != nil {
		logger.Error(err, "waiting for BucketClaims to finish provisioning")
		return fmt.Errorf("waiting for BucketClaims to finish provisioning: %w", err)
	}

	// After this status update, resource management should be handed off to the Sidecar
	if access.Status.ReadyToUse == nil {
		access.Status.ReadyToUse = ptr.To(false)
	}
	access.Status.AccessedBuckets = accessedBuckets
	access.Status.DriverName = class.Spec.DriverName
	access.Status.AuthenticationType = class.Spec.AuthenticationType
	access.Status.Parameters = class.Spec.Parameters
	access.Status.Error = nil
	if err := r.Status().Update(ctx, access); err != nil {
		logger.Error(err, "failed to update BucketClaim status after successful initialization")
		return err
	}

	return nil
}

func (r *BucketAccessReconciler) reconcileDelete(
	ctx context.Context, logger logr.Logger, access *cosiapi.BucketAccess,
) error {
	claimsByName, err := getAllBucketClaims(ctx, r.Client, access.Namespace, access.Spec.BucketClaims)
	if err != nil {
		logger.Error(err, "failed to get all referenced BucketClaims during deletion")
		return err
	}

	accessList := cosiapi.BucketAccessList{}
	if err := r.List(ctx, &accessList, &client.ListOptions{Namespace: access.Namespace}); err != nil {
		logger.Error(err, "failed to list BucketAccesses during deletion")
		return err
	}

	// Removing a mark on a BucketClaim is not a guarantee that there are no referencing
	// BucketAccesses. A user may create a referencing BucketAccess at any time during or after this
	// reconcile. To avoid races, the BucketClaim controller should check that the claim is unmarked
	// and check again for referencing accesses before proceeding with its own deletion.
	unmarked, err := unmarkUnaccessedBucketClaims(ctx, r.Client, claimsByName, accessList.Items, access.Name)
	if len(unmarked) > 0 {
		logger.Info("removed mark from BucketClaims having no referencing BucketAccesses",
			"removedMarkFromClaims", unmarked)
	}
	if err != nil {
		logger.Error(err, "failed to unmark unaccessed BucketClaims during deletion")
		return err
	}

	ctrlutil.RemoveFinalizer(access, cosiapi.ProtectionFinalizer)
	if err := r.Update(ctx, access); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return nil
}

// Validate that all bucketClaims references are to unique claims.
// This should be prohibited by schema rules, but this is critical for the BucketAccess reconcile
// logic's expectations, so double check.
func validateAllBucketClaimReferencesUnique(claimAccesses []cosiapi.BucketClaimAccess) error {
	claimCounts := make(map[string]int, len(claimAccesses))
	doErr := false
	for _, ref := range claimAccesses {
		count, ok := claimCounts[ref.BucketClaimName]
		if !ok {
			claimCounts[ref.BucketClaimName] = 1
		} else {
			claimCounts[ref.BucketClaimName] = count + 1
			doErr = true
		}
	}
	if doErr {
		return cosierr.NonRetryableError(
			fmt.Errorf("one or more BucketClaims are referenced multiple times, with reference counts: %v",
				claimCounts))
	}

	return nil
}

// Get all BucketClaims that this BucketAccess references.
// If any claims don't exist, assume they don't exist YET; mark them nil in the resulting map
// without treating nonexistence as an error.
// When no error is returned, the output map MUST have an entry for every given BucketClaimAccess.
func getAllBucketClaims(
	ctx context.Context, client client.Client, namespace string, claimAccesses []cosiapi.BucketClaimAccess,
) (map[string]*cosiapi.BucketClaim, error) {
	claims := make(map[string]*cosiapi.BucketClaim, len(claimAccesses))
	errs := []error{}

	for _, ref := range claimAccesses {
		c := cosiapi.BucketClaim{}
		nsName := types.NamespacedName{
			Namespace: namespace,
			Name:      ref.BucketClaimName,
		}
		err := client.Get(ctx, nsName, &c)
		if kerrors.IsNotFound(err) {
			// BucketClaim doesn't exist
			claims[ref.BucketClaimName] = nil
		} else if err != nil {
			// Unspecified API server error that probably resolves after exponential backoff
			errs = append(errs, err)
		} else {
			// No error
			claims[ref.BucketClaimName] = &c
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("could not get one or more BucketClaims: %w", errors.Join(errs...))
	}

	return claims, nil
}

// Mark all (non-nil) BucketClaims as having a BucketAccess reference.
func markAllBucketClaimsAsAccessed(
	ctx context.Context,
	client client.Client,
	claimsByName map[string]*cosiapi.BucketClaim,
) error {
	errs := []error{}
	for _, claim := range claimsByName {
		if claim == nil {
			continue
		}

		if claim.Annotations == nil {
			claim.Annotations = map[string]string{}
		}
		if _, ok := claim.Annotations[cosiapi.HasBucketAccessReferencesAnnotation]; ok {
			continue // already present
		}
		// Race condition: this will still attempt to apply the annotation even when the deletion
		// timestamp is set. This may interrupt an in-progress BucketClaim deletion before the point
		// of no return, preserving data, or it may be too late. The BucketClaim deletion logic must
		// handle the unexpected appearance of this annotation at any point.
		claim.Annotations[cosiapi.HasBucketAccessReferencesAnnotation] = ""
		if err := client.Update(ctx, claim); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to mark one or more BucketClaims as having a BucketAccess reference: %w",
			errors.Join(errs...))
	}

	return nil
}

// Unmark all (non-nil) BucketClaims that aren't referenced by any other BucketAccesses.
// Optionally ignore a reference from a specific BucketAccess.
// Returns list of BucketClaims that were unmarked, which is accurate even when an error is returned.
func unmarkUnaccessedBucketClaims(
	ctx context.Context,
	client client.Client,
	claimsByName map[string]*cosiapi.BucketClaim,
	accesses []cosiapi.BucketAccess,
	ignoreReferenceFromAccessName string, // optional
) ([]string, error) {
	errs := []error{}
	unmarked := []string{}
	for _, claim := range claimsByName {
		if claim == nil {
			continue
		}

		if claim.Annotations == nil {
			continue // no annotations means not marked
		}
		if _, ok := claim.Annotations[cosiapi.HasBucketAccessReferencesAnnotation]; !ok {
			continue // not marked
		}

		if hasBucketAccessReferences(claim.Name, accesses, ignoreReferenceFromAccessName) {
			continue // should not unmark
		}

		unmarked = append(unmarked, claim.Name)
		delete(claim.Annotations, cosiapi.HasBucketAccessReferencesAnnotation)
		if err := client.Update(ctx, claim); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return unmarked, fmt.Errorf("failed to unmark one or more BucketClaims without referencing BucketAccesses: %w",
			errors.Join(errs...))
	}

	return unmarked, nil
}

// Return true if one or more BucketAccesses reference the named BucketClaim.
// Optionally ignore a reference from a specific BucketAccess.
func hasBucketAccessReferences(
	claimName string,
	accesses []cosiapi.BucketAccess,
	ignoreReferenceFromAccessName string, // optional
) bool {
	for _, access := range accesses {
		if access.Name == ignoreReferenceFromAccessName {
			continue
		}
		for _, ref := range access.Spec.BucketClaims {
			if ref.BucketClaimName == claimName {
				return true
			}
		}
	}
	return false
}

// Return an error if the BucketAccess doesn't meet BucketAccessClass requirements.
func ValidateAccessAgainstClass(
	class *cosiapi.BucketAccessClassSpec,
	access *cosiapi.BucketAccessSpec,
) error {
	errs := []error{}

	needServiceAccount := class.AuthenticationType == cosiapi.BucketAccessAuthenticationTypeServiceAccount
	if needServiceAccount && access.ServiceAccountName == "" {
		errs = append(errs, fmt.Errorf("serviceAccountName must be specified"))
	}

	if !class.SupportsMultiBucketAccess() && len(access.BucketClaims) > 1 {
		errs = append(errs, fmt.Errorf("multi-bucket access is disallowed"))
	}

	for _, claimRef := range access.BucketClaims {
		if slices.Contains(class.DisallowedBucketAccessModes, claimRef.AccessMode) {
			errs = append(errs,
				fmt.Errorf("accessMode %q requested for BucketClaim %q is disallowed",
					claimRef.AccessMode, claimRef.BucketClaimName),
			)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("one or more features are disallowed by the BucketAccessClass: %w", errors.Join(errs...))
	}
	return nil
}

// Get all Buckets from the referenced BucketClaims.
// If any BucketClaims don't have a bound Bucket, assume the Bucket hasn't been created YET;
// mark them nil in the resulting map without treating it as an error.
// When no error is returned, the output map MUST have an entry for every given BucketClaim.
func getBucketsForClaims(
	ctx context.Context,
	client client.Client,
	claimsByName map[string]*cosiapi.BucketClaim,
) (map[string]*cosiapi.Bucket, error) {
	bucketsByClaimName := make(map[string]*cosiapi.Bucket, len(claimsByName))
	errs := []error{}

	for claimName, claim := range claimsByName {
		if claim == nil {
			// BucketClaim doesn't exist (yet), so can't have a Bucket
			bucketsByClaimName[claimName] = nil
			continue
		}
		if claim.Status.BoundBucketName == "" {
			// BucketClaim's Bucket hasn't been created (yet)
			bucketsByClaimName[claimName] = nil
			continue
		}

		bucket := cosiapi.Bucket{}
		nsName := types.NamespacedName{
			Namespace: "",
			Name:      claim.Status.BoundBucketName,
		}
		if err := client.Get(ctx, nsName, &bucket); err != nil {
			if kerrors.IsNotFound(err) {
				errs = append(errs, cosierr.NonRetryableError(
					fmt.Errorf("unrecoverable degradation: Bucket %q for BucketClaim %q has gone missing: %w",
						claim.Status.BoundBucketName, claimName, err)))
				continue
			}
			errs = append(errs, err) // should resolve after exponential backoff
			continue
		}

		bucketsByClaimName[claimName] = &bucket
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("could not get one or more Buckets: %w", errors.Join(errs...))
	}

	if len(bucketsByClaimName) != len(claimsByName) {
		// Should never happen, but double check because the 1:1 requirement is critical.
		return nil, fmt.Errorf("did not get one or more Buckets, but no errors observed")
	}

	return bucketsByClaimName, nil
}

// Ensure that all BucketClaims can request the access to be provisioned without known errors.
// Return a list of messages that explain what is blocking provisioning.
func cannotAccessBucketClaims(
	claimsByName map[string]*cosiapi.BucketClaim,
	spec cosiapi.BucketAccessSpec,
) []string {
	blockers := []string{}
	for name, claim := range claimsByName {
		if claim == nil {
			continue
		}
		if !claim.DeletionTimestamp.IsZero() {
			// The BucketClaim might not delete while this BucketAccess exists.
			// The BucketAccess can't proceed for the in-deletion BucketClaim.
			// Because this is a data safety race, rely on the user to resolve it as they desire.
			// This race is probably rare in the real world, so going to excessive lengths to
			// resolve it in COSI seems like premature optimization.
			blockers = append(blockers,
				fmt.Sprintf("stuck: data integrity for deleting BucketClaim %q is not guaranteed", name))
		}
		if len(claim.Status.Protocols) > 0 && !slices.Contains(claim.Status.Protocols, spec.Protocol) {
			blockers = append(blockers,
				fmt.Sprintf("BucketClaim %q does not support protocol %q", name, spec.Protocol))
		}
	}
	return blockers
}

// Ensure that all BucketClaims are provisioned enough to continue with access initialization.
// Return a list of messages that explain what needs to be waited on.
func waitingOnBucketClaims(
	claimsByName map[string]*cosiapi.BucketClaim,
	bucketsByClaimName map[string]*cosiapi.Bucket,
) []string {
	waitMsgs := []string{}
	for claimName, claim := range claimsByName {
		if claim == nil {
			// Could be a typo in a BucketClaim reference, or the BucketClaim might not exist yet.
			waitMsgs = append(waitMsgs, fmt.Sprintf("waiting for BucketClaim %q to exist", claimName))
			continue
		}

		// For static provisioning, this could indicate a misconfiguration somewhere.
		if claim.Status.BoundBucketName == "" {
			waitMsgs = append(waitMsgs, fmt.Sprintf("waiting for BucketClaim %q to be bound to a Bucket", claimName))
			continue
		}

		bucket, ok := bucketsByClaimName[claimName]
		if !ok {
			// Should never happen because getBucketsForClaims() guarantees 1:1 mapping
			waitMsgs = append(waitMsgs, fmt.Sprintf("internal error: missing Bucket %q for BucketClaim %q",
				claim.Status.BoundBucketName, claimName))
			continue
		}

		if bucket == nil {
			// Should never trigger because getBucketsForClaims() treats missing Buckets as errors,
			// but check for nil here to avoid any chance of nil pointer dereference below.
			waitMsgs = append(waitMsgs, fmt.Sprintf("waiting for Bucket %q for BucketClaim %q to exist",
				claim.Status.BoundBucketName, claimName))
			continue
		}

		// BucketIDs are a requirement for status.accessedBuckets, so ensure all Buckets have one.
		if bucket.Status.BucketID == "" {
			waitMsgs = append(waitMsgs,
				fmt.Sprintf("waiting for Bucket %q for BucketClaim %q to finish provisioning",
					claim.Status.BoundBucketName, claimName))
			continue
		}

		// The real thing we are waiting on is for the BucketClaim to be provisioned.
		// Beyond assumption/type checking, the checks above provide more detailed feedback to help
		// users debug issues more easily.
		if !ptr.Deref(claim.Status.ReadyToUse, false) {
			waitMsgs = append(waitMsgs, fmt.Sprintf("waiting for BucketClaim %q to finish provisioning Bucket %q",
				claimName, claim.Status.BoundBucketName))
			continue
		}
	}
	return waitMsgs
}

// Compile the accessedBuckets status list for the BucketAccess.
func compileAccessedBuckets(
	claimAccesses []cosiapi.BucketClaimAccess,
	bucketsByClaimName map[string]*cosiapi.Bucket,
) (
	[]cosiapi.AccessedBucket,
	error,
) {
	accessedBuckets := make([]cosiapi.AccessedBucket, len(claimAccesses))
	missingIds := []string{}

	// It will be helpful for human readability if the ordering of AccessedBuckets in the status
	// matches the ordering of BucketClaims in the spec.
	for i, ref := range claimAccesses {
		bucket, ok := bucketsByClaimName[ref.BucketClaimName]
		if !ok || bucket == nil {
			// Unexpected during runtime because getAllBucketClaims() requires that all input
			// BucketAccessClaims must be represented in the claimsByName map.
			return nil, fmt.Errorf("internal error: missing Bucket for BucketClaim %q", ref.BucketClaimName)
		}

		if bucket.Status.BucketID == "" {
			missingIds = append(missingIds, ref.BucketClaimName)
			continue
		}

		accessedBuckets[i] = cosiapi.AccessedBucket{
			BucketName:      bucket.Name,
			BucketID:        bucket.Status.BucketID,
			BucketClaimName: ref.BucketClaimName,
		}
	}

	if len(missingIds) > 0 {
		// Should never happen because waitingOnBucketClaims() checks for this condition.
		return nil, fmt.Errorf("one or more Buckets are missing IDs: %v", missingIds)
	}

	return accessedBuckets, nil
}
