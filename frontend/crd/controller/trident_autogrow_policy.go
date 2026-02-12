// Copyright 2025 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/internal/autogrow"
	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// handleAutogrowPolicy reconciles TridentAutogrowPolicy CRs.
// This is the main entry point for all TAGP lifecycle events (create, update, delete).
//
// Responsibilities:
//  1. Extract key and context from keyItem
//  2. Get policy from lister (cluster-scoped)
//  3. Manage finalizers (add for new policies, check for deletion)
//  4. Validate policy spec using validateAutogrowPolicy
//  5. Sync to orchestrator cache
//  6. Route to appropriate handler based on event type
//
// Parameters:
//   - keyItem: Contains the policy key, context, and event type
//
// Returns:
//   - nil: Success
//   - ReconcileDeferredError: Temporary failure, will retry
//   - UnsupportedConfigError: Permanent validation failure, won't retry
func (c *TridentCrdController) handleAutogrowPolicy(keyItem *KeyItem) error {
	if keyItem == nil {
		return errors.ReconcileDeferredError("keyItem is nil")
	}

	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(LogFields{
		"Key":        key,
		"eventType":  eventType,
		"objectType": objectType,
	}).Trace(">>>>>> TridentCrdController#handleAutogrowPolicy")

	// TridentAutogrowPolicy is cluster-scoped (non-namespaced), so namespace will be empty
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid key.")
		return nil
	}

	// Get the CR from lister - cluster-scoped so no namespace parameter needed
	autogrowPolicy, err := c.autogrowPoliciesLister.Get(name)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			Logx(ctx).WithField("key", key).Trace("Object in work queue no longer exists.")
			return nil
		}
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	// Policy has a deletion timestamp - route to delete handler
	if !autogrowPolicy.ObjectMeta.DeletionTimestamp.IsZero() {
		Logx(ctx).WithFields(LogFields{
			"autogrowPolicyName": autogrowPolicy.Name,
			"DeletionTimestamp":  autogrowPolicy.ObjectMeta.DeletionTimestamp,
		}).Trace("AutogrowPolicy has deletion timestamp, routing to delete handler.")
		eventType = EventDelete
	}

	// Handle based on event type (Add/Update only at this point)
	switch eventType {
	case EventAdd:
		return c.syncAutogrowPolicyToOrchestrator(ctx, autogrowPolicy, true)
	case EventUpdate:
		return c.syncAutogrowPolicyToOrchestrator(ctx, autogrowPolicy, false)
	case EventDelete:
		return c.deleteAutogrowPolicy(ctx, autogrowPolicy)
	default:
		return nil
	}
}

// syncAutogrowPolicyToOrchestrator handles validation, orchestrator sync, and status updates.
//
// This function is the core reconciliation logic that:
//  1. Ensures finalizers are present
//  2. Validates the policy spec
//  3. Prepares config with appropriate state (Success/Failed)
//  4. Syncs to orchestrator (Add or Update)
//  5. Updates CR status based on validation and sync results
//
// State Logic:
//   - If validation fails: State = Failed, but still synced to orchestrator
//   - If sync fails: State = Failed
//   - If both succeed: State = Success
//
// Parameters:
//   - ctx: Context for logging
//   - autogrowPolicy: The TridentAutogrowPolicy CR to sync
//   - isAdd: true for new policy, false for update
//
// Returns:
//   - nil: Success
//   - ReconcileDeferredError: Temporary failure (will retry)
//   - UnsupportedConfigError: Validation failed (won't retry)
func (c *TridentCrdController) syncAutogrowPolicyToOrchestrator(ctx context.Context, autogrowPolicy *tridentv1.TridentAutogrowPolicy, isAdd bool) error {
	Logx(ctx).WithField("autogrowPolicyName", autogrowPolicy.Name).Trace(">>>>>> Syncing AutogrowPolicy to Orchestrator")

	autogrowPolicyCopy := autogrowPolicy.DeepCopy()

	// Step 1: Ensure finalizer are present
	if !autogrowPolicyCopy.HasTridentFinalizers() {
		autogrowPolicyCopy.AddTridentFinalizers()

		updatedAGPolicy, err := c.updateAutogrowPolicyCR(ctx, autogrowPolicyCopy)
		if err != nil {
			Logx(ctx).WithError(err).Error("Failed to add finalizers to AutogrowPolicy custom resource.")
			return errors.ReconcileDeferredError("failed to add finalizers to AutogrowPolicy custom resource: %v", err)
		}
		autogrowPolicyCopy = updatedAGPolicy
	}

	// Step 2: Validate the Autogrow policy spec
	validationErr := c.validateAutogrowPolicy(ctx, autogrowPolicyCopy)

	// Step 3: Prepare config with state
	autogrowPolicyConfig := &storage.AutogrowPolicyConfig{
		Name:          autogrowPolicyCopy.Name,
		UsedThreshold: autogrowPolicyCopy.Spec.UsedThreshold,
		GrowthAmount:  autogrowPolicyCopy.Spec.GrowthAmount,
		MaxSize:       autogrowPolicyCopy.Spec.MaxSize,
		State:         storage.AutogrowPolicyStateSuccess,
	}

	if validationErr != nil {
		// Still sync to orchestrator even if validation failed
		// This allows orchestrator cache to match CR state
		// Volumes won't associate with Failed policies
		autogrowPolicyConfig.State = storage.AutogrowPolicyStateFailed
	}

	// Step 4: Sync to orchestrator
	var syncErr error
	if isAdd {
		_, syncErr = c.orchestrator.AddAutogrowPolicy(ctx, autogrowPolicyConfig)
	} else {
		_, syncErr = c.orchestrator.UpdateAutogrowPolicy(ctx, autogrowPolicyConfig)
	}

	// Step 5: Update CR status (priority: syncErr > validationErr)
	if syncErr != nil {
		if !errors.IsAlreadyExistsError(syncErr) {
			autogrowPolicyCopy.Status.State = string(tridentv1.TridentAutogrowPolicyStateFailed)
			autogrowPolicyCopy.Status.Message = fmt.Sprintf("Failed to sync autogrow policy to orchestrator: %v", syncErr)

			updatedPolicy, err := c.updateAutogrowPolicyCRStatus(ctx, autogrowPolicyCopy)
			if err != nil {
				return errors.ReconcileDeferredError("failed to update CR status: %v", err)
			}
			c.recorder.Event(updatedPolicy, corev1.EventTypeWarning, "AutogrowPolicySyncFailed", "Failed to sync autogrow policy to orchestrator.")

			return errors.ReconcileDeferredError("failed to sync autogrow policy to orchestrator: %v", syncErr)
		}
	}

	if validationErr != nil {
		autogrowPolicyCopy.Status.State = string(tridentv1.TridentAutogrowPolicyStateFailed)
		autogrowPolicyCopy.Status.Message = fmt.Sprintf("Validation failed: %v", validationErr)

		updatedPolicy, err := c.updateAutogrowPolicyCRStatus(ctx, autogrowPolicyCopy)
		if err != nil {
			return errors.ReconcileDeferredError("failed to update CR status: %v", err)
		}
		c.recorder.Event(updatedPolicy, corev1.EventTypeWarning, "AutogrowPolicyValidationFailed", "Failed to validate autogrow policy.")
		return errors.WrapUnsupportedConfigError(validationErr)
	}

	// Success
	autogrowPolicyCopy.Status.State = string(tridentv1.TridentAutogrowPolicyStateSuccess)
	autogrowPolicyCopy.Status.Message = "Autogrow policy validated and ready to use"

	updatedPolicy, err := c.updateAutogrowPolicyCRStatus(ctx, autogrowPolicyCopy)
	if err != nil {
		return errors.ReconcileDeferredError("failed to update CR status: %v", err)
	}

	// Emit success event
	c.recorder.Event(updatedPolicy, corev1.EventTypeNormal, "AutogrowPolicyAccepted",
		"Autogrow policy accepted.")

	return nil
}

// validateAutogrowPolicy validates the policy spec according to design requirements.
//
// Validation Rules:
//  1. UsedThreshold (required):
//     - Must be a percentage: "1%" to "99%" (e.g., "80%")
//  2. GrowthAmount (optional, defaults to "10%"):
//     - Percentage: "1%" or higher (e.g., "10%", "50%")
//     - Absolute: valid resource.Quantity > 0
//  3. MaxSize (optional):
//     - Must be valid resource.Quantity
//
// Parameters:
//   - ctx: Context for logging
//   - policy: The TridentAutogrowPolicy to validate
//
// Returns:
//   - nil: Validation passed
//   - error: Descriptive validation error
func (c *TridentCrdController) validateAutogrowPolicy(
	ctx context.Context, policy *tridentv1.TridentAutogrowPolicy,
) error {
	Logx(ctx).WithField("autogrowPolicyName", policy.Name).Trace(">>>>>> Validating AutogrowPolicy")
	defer Logx(ctx).WithField("autogrowPolicyName", policy.Name).Trace("<<<<<< Validating AutogrowPolicy")
	// Delegate to the reusable validation function in internal/autogrow package
	_, err := autogrow.ValidateAutogrowPolicySpec(
		policy.Spec.UsedThreshold,
		policy.Spec.GrowthAmount,
		policy.Spec.MaxSize,
	)

	return err
}

// deleteAutogrowPolicy handles deletion of TridentAutogrowPolicy CRs.
//
// Implementation matches backend deletion pattern:
//  1. Attempts deletion in orchestrator (soft-delete if volumes exist)
//  2. If policy not found: removes finalizer (deletion complete)
//  3. If policy soft-deleted (has volumes):
//     - Updates CR status.State to TridentAutogrowPolicyStateDeleting
//     - Updates status with list of blocking volumes
//     - Keeps finalizer and returns ReconcileDeferredError (will retry)
//  4. When volumes are disassociated, policy auto-deleted from orchestrator
//  5. Next reconciliation detects NotFoundError and removes finalizer
//
// Parameters:
//   - ctx: Context for logging
//   - policy: The TridentAutogrowPolicy being deleted
//
// Returns:
//   - nil: Finalizer removed (deletion complete)
//   - ReconcileDeferredError: Deletion in progress (policy in Deleting state with volumes)
func (c *TridentCrdController) deleteAutogrowPolicy(
	ctx context.Context, policy *tridentv1.TridentAutogrowPolicy,
) error {
	Logx(ctx).WithFields(LogFields{
		"autogrowPolicyName": policy.Name,
		"deletionTimestamp":  policy.ObjectMeta.DeletionTimestamp,
	}).Trace(">>>>>> Deleting AutogrowPolicy")

	// Try to delete from orchestrator (it will check for volume usage)
	err := c.orchestrator.DeleteAutogrowPolicy(ctx, policy.Name)
	if err != nil {
		if errors.IsAutogrowPolicyNotFoundError(err) {
			// Policy already deleted from orchestrator, safe to remove finalizer
			Logx(ctx).Info("Policy not found in orchestrator, removing finalizer.")
			return c.removeFinalizers(ctx, policy, false)
		}
		// Other errors - defer and retry
		return errors.ReconcileDeferredError("failed to delete policy from orchestrator: %v", err)
	}

	// Deletion call succeeded - now check if Autogrow policy is fully deleted or soft-deleted
	orchAGPolicy, getErr := c.orchestrator.GetAutogrowPolicy(ctx, policy.Name)
	if getErr != nil {
		if errors.IsAutogrowPolicyNotFoundError(getErr) {
			// Policy fully deleted (had no volumes), remove finalizer
			Logx(ctx).Info("Policy fully deleted from orchestrator, removing finalizer.")
			return c.removeFinalizers(ctx, policy, false)
		}
		// Error checking policy state
		return errors.ReconcileDeferredError("failed to verify policy state: %v", getErr)
	}

	// Policy still exists in orchestrator - check if it has volumes
	if orchAGPolicy.VolumeCount > 0 {
		// Policy is soft-deleted with volumes, update CR status and keep finalizer
		Logx(ctx).WithFields(LogFields{
			"autogrowPolicyName": policy.Name,
			"volumeCount":        orchAGPolicy.VolumeCount,
		}).Info("Policy in Deleting state with volumes, updating CR status.")

		policyCopy := policy.DeepCopy()
		policyCopy.Status.State = string(tridentv1.TridentAutogrowPolicyStateDeleting)
		policyCopy.Status.Message = fmt.Sprintf(
			"Policy is in use by %d volume(s), waiting for volumes to be disassociated.",
			orchAGPolicy.VolumeCount)

		updatedPolicy, statusErr := c.updateAutogrowPolicyCRStatus(ctx, policyCopy)
		if statusErr != nil {
			return errors.ReconcileDeferredError("failed to update status: %v", statusErr)
		}

		c.recorder.Event(updatedPolicy, corev1.EventTypeNormal, "AutogrowPolicyDeleting",
			fmt.Sprintf("Waiting for %d volume(s) to be disassociated", orchAGPolicy.VolumeCount))

		// Keep finalizer, retry later
		return errors.ReconcileDeferredError("policy still in use by %d volume(s)", orchAGPolicy.VolumeCount)
	}

	// Policy exists with 0 volumes - orchestrator should have deleted it but didn't
	// This shouldn't normally happen, but retry to trigger cleanup
	Logx(ctx).Warn("Policy exists with 0 volumes but wasn't deleted, will retry.")
	return errors.ReconcileDeferredError("policy exists with 0 volumes, retrying deletion")
}

// updateAutogrowPolicyCR updates the TridentAutogrowPolicy CR.
// Used for updating both spec (finalizers) and status in a single operation.
//
// Parameters:
//   - ctx: Context for logging
//   - policy: The TridentAutogrowPolicy to update
//
// Returns:
//   - *TridentAutogrowPolicy: Updated policy
//   - error: If update fails
func (c *TridentCrdController) updateAutogrowPolicyCR(
	ctx context.Context, policy *tridentv1.TridentAutogrowPolicy,
) (*tridentv1.TridentAutogrowPolicy, error) {
	result, err := c.crdClientset.TridentV1().TridentAutogrowPolicies().Update(ctx, policy, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"autogrowPolicyName": policy.Name,
			"err":                err,
		}).Error("Could not update AutogrowPolicy CR.")
	}
	return result, err
}

// updateAutogrowPolicyCRStatus updates the TridentAutogrowPolicy CR status subresource.
// This must be used for any changes to policy.Status fields (State, Message).
// Status is a special subresource in Kubernetes that requires UpdateStatus() method.
//
// Parameters:
//   - ctx: Context for logging
//   - policy: The TridentAutogrowPolicy with updated status
//
// Returns:
//   - *TridentAutogrowPolicy: Updated policy
//   - error: If update fails
func (c *TridentCrdController) updateAutogrowPolicyCRStatus(
	ctx context.Context, policy *tridentv1.TridentAutogrowPolicy,
) (*tridentv1.TridentAutogrowPolicy, error) {
	result, err := c.crdClientset.TridentV1().TridentAutogrowPolicies().UpdateStatus(ctx, policy, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"autogrowPolicyName": policy.Name,
			"err":                err,
		}).Error("Could not update AutogrowPolicy CR status.")
	}
	return result, err
}

// removeAutogrowPolicyFinalizers removes Trident's finalizers from TridentAutogrowPolicy CRs
func (c *TridentCrdController) removeAutogrowPolicyFinalizers(
	ctx context.Context, policy *tridentv1.TridentAutogrowPolicy,
) error {
	Logx(ctx).WithFields(LogFields{
		"autogrowPolicy.ResourceVersion":              policy.ResourceVersion,
		"autogrowPolicy.ObjectMeta.DeletionTimestamp": policy.ObjectMeta.DeletionTimestamp,
	}).Trace(">>>>>> removeAutogrowPolicyFinalizers")

	if policy.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		policyCopy := policy.DeepCopy()
		policyCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentAutogrowPolicies().Update(ctx, policyCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return err
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return nil
}
