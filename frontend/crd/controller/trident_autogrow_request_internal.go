// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/autogrow"
	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	// TAGRI status phases
	TagriPhasePending    = "Pending"
	TagriPhaseInProgress = "InProgress"
	TagriPhaseCompleted  = "Completed"
	TagriPhaseRejected   = "Rejected"
	TagriPhaseFailed     = "Failed"

	// Retry limits
	// MaxTagriRetries is the maximum number of attempts before considering a permanent failure
	// and marking the TAGRI as Failed. It applies to both:
	//   - PVC patch attempts: after this many failed patch attempts (e.g. RBAC, API rejection,
	//     PVC state), the TAGRI is marked Failed instead of waiting for age-based timeout.
	//   - Resize failure observations: after this many observed resize failures (e.g. CSI
	//     transient errors) while monitoring, the TAGRI is marked Failed instead of retrying further.
	// Exceeding this limit in either path causes the controller to call failTagri and set phase Failed.
	MaxTagriRetries = 5

	// Event checking limits
	// MaxEventsToCheckForResize limits how many recent events we scan when checking for resize success/failures
	MaxEventsToCheckForResize = 10

	// Event reasons
	EventReasonAutogrowTriggered = "AutogrowTriggered"
	EventReasonAutogrowSucceeded = "AutogrowSucceeded"
	EventReasonAutogrowFailed    = "AutogrowFailed"
	EventReasonAutogrowRejected  = "AutogrowRejected"
)

// handleTridentAutogrowRequestInternal is the main handler for TridentAutogrowRequestInternal CRs.
// It routes to first-time processing or monitoring mode based on the CR's current status.
func (c *TridentCrdController) handleTridentAutogrowRequestInternal(keyItem *KeyItem) error {
	if keyItem == nil {
		return errors.ReconcileDeferredError("keyItem is nil")
	}

	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event

	Logx(ctx).WithFields(LogFields{"key": key, "eventType": eventType}).Debug(">>>> handleTridentAutogrowRequestInternal")
	defer Logx(ctx).Debug("<<<< handleTridentAutogrowRequestInternal")

	// Get TAGRI from lister (Info so operators see TAGRI processing at default log level)
	Logx(ctx).WithFields(LogFields{"key": key, "eventType": eventType}).Info("Processing TAGRI")

	// Get TAGRI from lister
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// Invalid key is a permanent error, will not get fixed on retry. Thus, log and ignore.
		Logx(ctx).WithField("key", key).WithError(err).Error("Invalid key. Ignoring.")
		return nil
	}

	tagri, err := c.autogrowRequestInternalLister.TridentAutogrowRequestInternals(namespace).Get(name)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			Logx(ctx).WithField("key", key).WithError(err).Debug("TAGRI in work queue no longer exists. Ignoring.")
			return nil
		}
		// Lister error (transient) - retry with exponential backoff
		return errors.WrapWithReconcileDeferredError(err, "failed to get TAGRI from lister, will retry")
	}

	// Delete can be delivered as Update when the object gets DeletionTimestamp set; route to delete handler.
	if !tagri.ObjectMeta.DeletionTimestamp.IsZero() {
		Logx(ctx).WithFields(LogFields{
			"tagri":             tagri.Name,
			"deletionTimestamp": tagri.ObjectMeta.DeletionTimestamp,
		}).Debug("TAGRI has deletion timestamp, routing to delete handler.")
		eventType = EventDelete
	}

	// Log TAGRI details and event type
	Logx(ctx).WithFields(LogFields{
		"tagri":                       tagri.Name,
		"volume":                      tagri.Spec.Volume,
		"autogrowPolicyRefName":       tagri.Spec.AutogrowPolicyRef.Name,
		"autogrowPolicyRefGeneration": tagri.Spec.AutogrowPolicyRef.Generation,
		"observedUsedPercent":         tagri.Spec.ObservedUsedPercent,
		"observedUsedBytes":           tagri.Spec.ObservedUsedBytes,
		"observedCapacityBytes":       tagri.Spec.ObservedCapacityBytes,
		"nodeName":                    tagri.Spec.NodeName,
		"timestamp":                   tagri.Spec.Timestamp,
		"phase":                       tagri.Status.Phase,
		"message":                     tagri.Status.Message,
		"finalCapacityBytes":          tagri.Status.FinalCapacityBytes,
		"processedAt":                 tagri.Status.ProcessedAt,
		"retryCount":                  tagri.Status.RetryCount,
		"eventType":                   eventType,
	}).Debug("Handling TAGRI event")

	switch eventType {
	case EventAdd, EventForceUpdate, EventUpdate:
		return c.upsertTagriHandler(ctx, tagri)
	case EventDelete:
		return c.deleteTagriHandler(ctx, tagri)
	default:
		return nil
	}
}

func (c *TridentCrdController) upsertTagriHandler(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> upsertTagriHandler")
	defer Logx(ctx).WithField("tagri", tagri.Name).Debug("<<<< upsertTagriHandler")

	// Handle TAGRI timeout and terminal phase cleanup.
	// When handled is true, we either deleted the TAGRI or deferred (error); caller must stop and return.
	// When handled is false and err is nil, continue to phase routing (first-time or monitoring).
	handled, err := c.handleTimeoutAndTerminalPhase(ctx, tagri)
	if handled {
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to do TAGRI timeout or terminal phase based cleanup")
		}
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"phase": tagri.Status.Phase,
		}).Debug("TAGRI already handled (deleted), nothing more required")
		return nil
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to do TAGRI timeout or terminal phase based cleanup")
	}

	// Route based on current phase
	if tagri.Status.Phase == TagriPhaseInProgress {
		// MONITORING MODE
		return c.monitorTagriResize(ctx, tagri)
	}

	// FIRST-TIME PROCESSING (Pending or empty phase)
	return c.processTagriFirstTime(ctx, tagri)
}

// handleTimeoutAndTerminalPhase returns (handled, err). When handled is true, the caller must stop
// (either we deleted the TAGRI or deferred with error). When handled is false and err is nil, continue to
// phase routing (processTagriFirstTime or monitorTagriResize).
func (c *TridentCrdController) handleTimeoutAndTerminalPhase(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) (handled bool, err error) {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> handleTimeoutAndTerminalPhase")
	defer Logx(ctx).Debug("<<<< handleTimeoutAndTerminalPhase")

	// STEP 1: Check hard timeout (ALWAYS FIRST, applies to ALL phases)
	// This is the safety net that prevents TAGRIs from being stuck indefinitely in any phase
	hardTimeout := config.GetTagriTimeout()
	if !tagri.CreationTimestamp.IsZero() {
		age := time.Since(tagri.CreationTimestamp.Time)
		if age >= hardTimeout {
			Logx(ctx).WithFields(LogFields{
				"tagri":       tagri.Name,
				"phase":       tagri.Status.Phase,
				"age":         age.Truncate(time.Second),
				"hardTimeout": hardTimeout.Truncate(time.Second),
			}).Info("TAGRI exceeded hard timeout (age-based), deleting immediately")
			return true, c.deleteTagriNow(ctx, tagri)
		}
	}

	// STEP 2: Handle terminal phases (Completed/Rejected) — delete immediately
	if tagri.Status.Phase == TagriPhaseCompleted || tagri.Status.Phase == TagriPhaseRejected {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"phase": tagri.Status.Phase,
		}).Debug("TAGRI is in terminal phase, deleting immediately")
		return true, c.deleteTagriNow(ctx, tagri)
	}

	// STEP 3: Failed phase - wait for hard timeout, unless resize has since succeeded
	// If we previously marked Failed (e.g. after max resize retries) but resize later succeeded
	// (e.g. backend came back online), transition to Completed so we delete immediately.
	if tagri.Status.Phase == TagriPhaseFailed {
		completed, err := c.tryCompleteTagriIfResizeReachedTarget(ctx, tagri, nil, nil, "TAGRI was Failed but resize has since succeeded")
		if err != nil {
			return true, errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed after recovery")
		}
		if completed {
			return true, nil
		}
		return true, errors.ReconcileDeferredError(
			"TAGRI in Failed phase, waiting for hard timeout (%s)", hardTimeout.Truncate(time.Second))
	}

	// STEP 4: Active phases (Pending/InProgress/empty) - continue processing
	// These will be monitored and eventually timeout via hard timeout if stuck
	if tagri.Status.Phase == TagriPhasePending || tagri.Status.Phase == TagriPhaseInProgress || tagri.Status.Phase == "" {
		// Return handled=false so caller continues to processTagriFirstTime or monitorTagriResize
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"phase": tagri.Status.Phase,
		}).Debug("TAGRI in active phase, continuing to reconciliation")
		return false, nil
	}

	// STEP 5: Unknown/unexpected phase - defer processing
	// Do NOT fall through to processTagriFirstTime() which could corrupt state.
	// Return handled=true so caller stops; ReconcileDeferredError will requeue with backoff.
	Logx(ctx).WithFields(LogFields{
		"tagri": tagri.Name,
		"phase": tagri.Status.Phase,
	}).Warn("TAGRI has unknown phase; deferring reconciliation, hard timeout will clean up if stuck")
	return true, errors.ReconcileDeferredError("TAGRI has unknown phase: %s", tagri.Status.Phase)
}

// processTagriFirstTime handles the first-time processing of a TAGRI:
// validates policy, calculates size, patches PVC, and starts monitoring.
func (c *TridentCrdController) processTagriFirstTime(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> processTagriFirstTime")
	defer Logx(ctx).Debug("<<<< processTagriFirstTime")

	// Step 1: Validate mandatory observed capacity (total size host sees; required for safe growth; prevents shrinkage/data loss).
	// Done first so invalid TAGRIs are rejected before any orchestrator or API calls.
	if tagri.Spec.ObservedCapacityBytes == "" {
		if err := c.rejectTagri(ctx, tagri, nil, "observedCapacityBytes is required"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (missing observedCapacityBytes), requeuing for deletion")
	}
	observedCapacity, parseErr := resource.ParseQuantity(tagri.Spec.ObservedCapacityBytes)
	if parseErr != nil {
		if err := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("invalid observedCapacityBytes %q: %v", tagri.Spec.ObservedCapacityBytes, parseErr)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (invalid observedCapacityBytes), requeuing for deletion")
	}
	if observedCapacity.Sign() <= 0 {
		if err := c.rejectTagri(ctx, tagri, nil, "observedCapacityBytes must be greater than 0"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (observedCapacityBytes must be positive), requeuing for deletion")
	}

	// Step 2: Get TridentVolume from core cache
	volExternal, err := c.orchestrator.GetVolume(ctx, tagri.Spec.Volume)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// Volume not found in core cache - this is a permanent validation error, reject and requeue for immediate deletion
			if err := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("Volume %s not found", tagri.Spec.Volume)); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
			}
			return errors.ReconcileIncompleteError("TAGRI rejected (Volume %s not found), requeuing for deletion", tagri.Spec.Volume)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get volume")
	}

	// Step 3: Get PVC corresponding to the volume -
	// This is needed to check PVC current capacity and emit event on the PVC when required
	pvc, err := c.getPVCForVolume(ctx, volExternal, tagri.Spec.Volume)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			// PVC not found - this is a permanent validation error, reject and requeue for immediate deletion
			if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("PVC not found for volume %s", tagri.Spec.Volume)); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
			}
			return errors.ReconcileIncompleteError("TAGRI rejected (PVC not found), requeuing for deletion")
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get PVC")
	}

	// Step 4: Validate effective policy
	effectivePolicy := volExternal.EffectiveAutogrowPolicy
	if effectivePolicy.PolicyName == "" {
		// EffectiveAutogrowPolicy is empty - this indicates:
		// 1. Volume was created without autogrow policy, OR
		// 2. Autogrow policy was removed/unset after volume creation, OR
		// 3. Race condition: autogrow policy not yet propagated to volume config
		//
		// Since node created TAGRI with policy reference, this is likely a configuration error
		// or the autogrow policy was removed after node observed it. Reject the request.
		Logx(ctx).WithFields(LogFields{
			"volume":          tagri.Spec.Volume,
			"requestedPolicy": tagri.Spec.AutogrowPolicyRef.Name,
		}).Warn("Volume has no effective autogrow policy configured, rejecting TAGRI")

		if err := c.rejectTagri(ctx, tagri, pvc, "No effective autogrow policy found for this volume"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (no effective autogrow policy found for this volume), requeuing for deletion")
	}

	if effectivePolicy.PolicyName != tagri.Spec.AutogrowPolicyRef.Name {
		// Autogrow policy mismatch - validation error, reject and requeue for immediate deletion
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("Autogrow policy mismatch: effective=%s, requested=%s",
			effectivePolicy.PolicyName, tagri.Spec.AutogrowPolicyRef.Name)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError(fmt.Sprintf("TAGRI rejected (autogrow policy mismatch: effective=%s, requested=%s), requeuing for deletion",
			effectivePolicy.PolicyName, tagri.Spec.AutogrowPolicyRef.Name))
	}

	// Step 5: Validate autogrow policy existence and Success state from core cache (orchestrator).
	// Policy state in the CR may not be persisted or propagated yet when read from the lister;
	// the orchestrator's cache reflects the authoritative state after sync, so we check it first.
	orchPolicy, err := c.orchestrator.GetAutogrowPolicy(ctx, tagri.Spec.AutogrowPolicyRef.Name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// Policy not in core - may be still syncing or deleted. Retry with backoff.
			return errors.ReconcileDeferredError("Autogrow policy %s not found, will retry with exponential backoff", tagri.Spec.AutogrowPolicyRef.Name)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get autogrow policy")
	}
	if !orchPolicy.State.IsSuccess() {
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("Autogrow policy is not in %s state: current state is=%s", storage.AutogrowPolicyStateSuccess, orchPolicy.State)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError(fmt.Sprintf("TAGRI rejected (autogrow policy is not in success state, current state is=%s), requeuing for deletion", orchPolicy.State))
	}

	// Step 6: Get autogrow policy CR from lister for generation check (CR is source of truth for metadata.generation)
	policy, err := c.autogrowPoliciesLister.Get(tagri.Spec.AutogrowPolicyRef.Name)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			// CR not in lister yet (e.g. informer lag) although orchestrator has policy; retry.
			return errors.ReconcileDeferredError("Autogrow policy CR %s not found in lister, will retry with exponential backoff", tagri.Spec.AutogrowPolicyRef.Name)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get autogrow policy CR from lister")
	}

	// Step 7: Validate generation (CR generation must match TAGRI's policy ref)
	if policy.Generation != tagri.Spec.AutogrowPolicyRef.Generation {
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("Autogrow policy generation mismatch: current=%d, requested=%d",
			policy.Generation, tagri.Spec.AutogrowPolicyRef.Generation)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError(
			fmt.Sprintf("TAGRI rejected (autogrow policy generation mismatch: current=%d, requested=%d), requeuing for deletion", policy.Generation, tagri.Spec.AutogrowPolicyRef.Generation))
	}

	// Step 8: Get backend resize delta (if any), then calculate final capacity in one place (policy + delta + maxSize).
	// Every TAGRI is a fresh look: use only current PVC actual size (status.capacity) for calculation.
	// If status.capacity is nil (e.g. PVC not bound), reject outright — we need a real current size to grow from.
	if pvc.Status.Capacity == nil {
		if err := c.rejectTagri(ctx, tagri, pvc, "PVC capacity not yet reported (PVC may not be bound); cannot calculate final capacity"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (PVC capacity not reported), requeuing for deletion")
	}
	currentSize := pvc.Status.Capacity[corev1.ResourceStorage]
	resizeDeltaBytes, err := c.orchestrator.GetResizeDeltaForBackend(ctx, volExternal.BackendUUID)
	if err != nil {
		// Unable to get resize delta (e.g. backend not ready); retry with backoff to avoid hammering.
		// Workqueue will requeue with exponential backoff; it does not permanently give up.
		Logx(ctx).WithFields(LogFields{
			"volume":      tagri.Spec.Volume,
			"backendUUID": volExternal.BackendUUID,
			"error":       err,
		}).Warn("Unable to get resize delta; will retry with backoff")
		return errors.ReconcileDeferredError("unable to get resize delta; will retry with backoff: %v", err)
	}
	finalCapacity, err := c.calculateFinalCapacity(ctx, currentSize, policy, tagri.Spec.Volume, resizeDeltaBytes)
	if err != nil {
		// Size calculation error - validation error, reject and requeue for immediate deletion
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("Failed to calculate final capacity: %v", err)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (final capacity calculation error), requeuing for deletion")
	}

	// Step 9: Reject if final capacity would be below observed capacity (total size host sees); prevents volume shrinkage and data loss.
	if finalCapacity.Value() < observedCapacity.Value() {
		msg := fmt.Sprintf("Rejected: final capacity %d bytes would be less than observed capacity %d bytes (%s), may cause volume shrinkage and data loss",
			finalCapacity.Value(), observedCapacity.Value(), tagri.Spec.ObservedCapacityBytes)
		Logx(ctx).WithFields(LogFields{
			"tagri":                 tagri.Name,
			"finalCapacityBytes":    tagri.Status.FinalCapacityBytes,
			"observedCapacityBytes": tagri.Spec.ObservedCapacityBytes,
			"finalCapacity":         finalCapacity.Value(),
			"observedCapacity":      observedCapacity.Value(),
		}).Warn(msg)
		if err := c.rejectTagri(ctx, tagri, pvc, msg); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileIncompleteError("TAGRI rejected (final capacity below observed capacity), requeuing for deletion")
	}

	// Human-readable only for display and logs; all actual operations use exact finalCapacity (bytes).
	readableCapacityStr := capacity.QuantityToHumanReadableString(finalCapacity)

	// Step 10: Check if already at or above target size
	if pvc.Status.Capacity != nil {
		currentCapacity := pvc.Status.Capacity[corev1.ResourceStorage]
		if currentCapacity.Cmp(finalCapacity) >= 0 {
			currentCapacityStr := capacity.QuantityToHumanReadableString(currentCapacity)
			Logx(ctx).WithFields(LogFields{
				"pvc":                  pvc.Name,
				"currentCapacity":      currentCapacityStr,
				"currentCapacityBytes": currentCapacity.Value(),
				"targetCapacity":       readableCapacityStr,
				"targetCapacityBytes":  finalCapacity.Value(),
			}).Info("PVC already at or above target size, marking as completed")

			// Success - mark as completed and requeue for immediate deletion (human-readable for display only)
			if err := c.completeTagriSuccess(ctx, tagri, volExternal, pvc, readableCapacityStr); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as Completed")
			}
			return errors.ReconcileIncompleteError("TAGRI completed (already at target size), requeuing for deletion")
		}
	}

	// Step 11: Add finalizer
	if !tagri.HasTridentFinalizers() {
		tagriCopy := tagri.DeepCopy()
		tagriCopy.AddTridentFinalizers()
		tagri, err = c.updateTagriCR(ctx, tagriCopy)
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to add finalizer")
		}
	}

	// Step 12: Patch PVC with exact calculated size (bytes); display/log use human-readable
	err = c.patchPVCSize(ctx, pvc, finalCapacity)
	if err != nil {
		// Check retry count
		if tagri.Status.RetryCount >= MaxTagriRetries {
			// PVC patch permanently failed after 5 attempts (~75s with exponential backoff)
			// This is likely a permanent issue (RBAC, API rejection, PVC state)
			// Mark as failed, emit event, and delete immediately to unblock node
			if err := c.failTagri(ctx, tagri, volExternal, pvc,
				fmt.Sprintf("Failed to patch PVC after %d attempts: %v", MaxTagriRetries, err)); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as Failed")
			}
			// Delete immediately - no need to wait for age-based timeout
			return c.deleteTagriNow(ctx, tagri)
		}

		// Increment retry and requeue with exponential backoff
		tagriCopy := tagri.DeepCopy()
		tagriCopy.Status.RetryCount++
		if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to update retry count")
		}

		return errors.ReconcileDeferredError("PVC patch failed, retry %d/%d: %v",
			tagriCopy.Status.RetryCount, MaxTagriRetries, err)
	}

	// Step 13: Update TAGRI status to InProgress (FinalCapacityBytes = numeric bytes; Message = human-readable for display)
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseInProgress
	tagriCopy.Status.FinalCapacityBytes = fmt.Sprintf("%d", finalCapacity.Value())
	tagriCopy.Status.Message = fmt.Sprintf("PVC patched, monitoring resize progress (target: %s)", readableCapacityStr)
	tagriCopy.Status.RetryCount = 0 // Reset for monitoring phase (used as "resize failure observed" count if resize fails)

	tagri, err = c.updateTagriStatus(ctx, tagriCopy)
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update status to InProgress")
	}
	if tagri == nil {
		// TAGRI was deleted during status update (race); nothing left to do
		return nil
	}

	// TotalAutogrowAttempted is incremented only when TAGRI reaches a terminal state (Rejected, Failed, or Completed).
	// RetryCount and TAGRI phase/status capture in-progress retries; tvol attempted = number of requests concluded.

	// Step 14: Return ReconcileIncompleteError to start monitoring
	return errors.ReconcileIncompleteError("Starting resize monitoring for TAGRI %s", tagri.Name)
}

// monitorTagriResize monitors the PVC resize progress for a TAGRI in InProgress phase.
func (c *TridentCrdController) monitorTagriResize(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> monitorTagriResize")
	defer Logx(ctx).Debug("<<<< monitorTagriResize")

	// Step 1: Get TridentVolume (fetch once and reuse)
	volExternal, err := c.orchestrator.GetVolume(ctx, tagri.Spec.Volume)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// Volume deleted during monitoring; mark TAGRI as Rejected
			if err := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("Volume %s deleted during monitoring", tagri.Spec.Volume)); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
			}
			return errors.ReconcileIncompleteError("TAGRI rejected (volume deleted), requeuing for deletion")
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get volume during monitoring")
	}

	// Step 2: Get PVC (fetch once and reuse)
	pvc, err := c.getPVCForVolume(ctx, volExternal, tagri.Spec.Volume)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			// PVC deleted during monitoring; mark TAGRI as Rejected
			if err := c.rejectTagri(ctx, tagri, nil, "PVC deleted during monitoring"); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
			}
			return errors.ReconcileIncompleteError("TAGRI rejected (PVC deleted), requeuing for deletion")
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get PVC during monitoring")
	}

	// Step 3: Check PVC conditions and events for resize status (failures and success)
	// Single API call to fetch events, then check for both failure and success events
	resizeFailed, resizeSuccessful, errorMsg := c.checkPVCResizeStatus(ctx, pvc)

	// Check success first: if both success and failure events exist, success takes precedence
	// This handles cases where resize initially failed but succeeded on retry
	if resizeSuccessful {
		Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "pvc": pvc.Name}).Debug("Detected VolumeResizeSuccessful event, verifying capacity")
		completed, err := c.tryCompleteTagriIfResizeReachedTarget(ctx, tagri, volExternal, pvc, "Resize completed successfully.")
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed")
		}
		if completed {
			return errors.ReconcileIncompleteError("TAGRI marked as completed, waiting for terminal phase based deletion")
		}
		Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "pvc": pvc.Name}).Debug("VolumeResizeSuccessful event detected but capacity not yet updated, continuing to check capacity")
	}

	// Check failure: we reach here when (a) most recent terminal event was VolumeResizeFailed,
	// or (b) most recent was success but capacity not yet updated (fell through from above).
	// Only in (a) do we enter the block below; in (b) resizeFailed is false and we fall through to check capacity.
	// When resizeFailed: reuse RetryCount (reset to 0 when we entered InProgress) to give transient CSI errors during resize
	// a chance to clear up before concluding the TAGRI as failed.
	// We only mark Failed after MaxTagriPatchRetries observations; until then phase stays InProgress
	// and we keep re-checking (success event or capacity >= target will complete).
	if resizeFailed {
		// If capacity already >= target (e.g. retry succeeded and capacity updated), don't mark Failed - complete
		completed, err := c.tryCompleteTagriIfResizeReachedTarget(ctx, tagri, volExternal, pvc,
			"Resize completed (capacity at target despite failure event), marking as completed")
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed")
		}
		if completed {
			return errors.ReconcileIncompleteError("TAGRI marked as completed, waiting for terminal phase based deletion")
		}

		// Increment RetryCount (same field as patch retries; in InProgress it means "resize failure observed" count) and persist
		tagriCopy := tagri.DeepCopy()
		tagriCopy.Status.RetryCount++
		if tagriCopy.Status.RetryCount < MaxTagriRetries {
			if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to update retry count (resize failure observed)")
			}
			Logx(ctx).WithFields(LogFields{
				"tagri":    tagri.Name,
				"pvc":      pvc.Name,
				"count":    tagriCopy.Status.RetryCount,
				"max":      MaxTagriRetries,
				"errorMsg": errorMsg,
			}).Warn("Resize failure observed (potentially transient), will retry before concluding failed")
			return errors.ReconcileDeferredError("Resize failure observed (potentially transient), count %d/%d - will retry before concluding failed",
				tagriCopy.Status.RetryCount, MaxTagriRetries)
		}

		// RetryCount reached max - conclude failed
		Logx(ctx).WithFields(LogFields{
			"tagri":    tagri.Name,
			"pvc":      pvc.Name,
			"errorMsg": errorMsg,
		}).Warn("CSI resize failed after max retry count, marking TAGRI as Failed")
		if err := c.failTagri(ctx, tagri, volExternal, pvc, fmt.Sprintf("CSI resize failure: %s", errorMsg)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as Failed")
		}
		return errors.ReconcileIncompleteError("TAGRI marked as Failed, waiting for age-based timeout before deletion")
	}

	// Step 4: Check PVC capacity
	if pvc.Status.Capacity == nil {
		// Use ReconcileDeferredError to avoid hammering API server with immediate requeues
		return errors.ReconcileDeferredError("PVC %s capacity not yet reported, will retry with backoff", pvc.Name)
	}

	// Check if resize has reached target (reuse same helper as success/failure event paths)
	completed, err := c.tryCompleteTagriIfResizeReachedTarget(ctx, tagri, volExternal, pvc, "Resize completed successfully.")
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed")
	}
	if completed {
		return errors.ReconcileIncompleteError("TAGRI marked as completed, waiting for terminal phase based deletion")
	}

	targetCapacity, err := resource.ParseQuantity(tagri.Status.FinalCapacityBytes)
	if err != nil {
		// Invalid capacity - unlikely to happen in monitoring stage. Nonetheless, mark TAGRI as Failed.
		if err := c.failTagri(ctx, tagri, volExternal, pvc, fmt.Sprintf("Invalid target capacity: %v", err)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as Failed")
		}
		return errors.ReconcileIncompleteError("TAGRI marked as Failed (invalid capacity), requeuing for age-based timeout deletion")
	}

	currentCapacity := pvc.Status.Capacity[corev1.ResourceStorage]
	currentCapacityStr := capacity.QuantityToHumanReadableString(currentCapacity)
	targetCapacityStr := capacity.QuantityToHumanReadableString(targetCapacity)

	// Still in progress, continue monitoring
	// Use ReconcileDeferredError instead of ReconcileIncompleteError to avoid hammering API server
	// Workqueue will retry with exponential backoff (2s, 4s, 8s...), reducing API load
	Logx(ctx).WithFields(LogFields{
		"pvc":                  pvc.Name,
		"currentCapacity":      currentCapacityStr,
		"currentCapacityBytes": currentCapacity.Value(),
		"targetCapacity":       targetCapacityStr,
		"targetCapacityBytes":  targetCapacity.Value(),
	}).Debug("Resize still in progress, continuing to monitor")

	return errors.ReconcileDeferredError("Resize in progress: %s/%s",
		currentCapacity.String(), targetCapacity.String())
}

// calculateFinalCapacity delegates to autogrow.CalculateFinalCapacity (policy + optional resize delta + maxSize).
func (c *TridentCrdController) calculateFinalCapacity(
	ctx context.Context,
	currentSize resource.Quantity,
	policy *tridentv1.TridentAutogrowPolicy,
	volumeName string,
	resizeDeltaBytes int64,
) (resource.Quantity, error) {
	Logx(ctx).Debug(">>>> calculateFinalCapacity")
	defer Logx(ctx).Debug("<<<< calculateFinalCapacity")
	finalCapacity, err := autogrow.CalculateFinalCapacity(currentSize, policy, resizeDeltaBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to calculate final capacity for volume %s: %w", volumeName, err)
	}

	Logx(ctx).WithFields(LogFields{
		"volume":             volumeName,
		"currentSize":        currentSize.String(),
		"currentSizeBytes":   currentSize.Value(),
		"growthAmount":       policy.Spec.GrowthAmount,
		"maxSize":            policy.Spec.MaxSize,
		"finalCapacity":      capacity.QuantityToHumanReadableString(finalCapacity),
		"finalCapacityBytes": finalCapacity.Value(),
	}).Debug("Calculated final capacity")

	return finalCapacity, nil
}

// getPVCForVolume returns the PVC corresponding to the given volume (PV name). It resolves PVC only from
// the PV binding: either via the CSI getter (cache-first) or via the Kubernetes API (PV.Spec.ClaimRef).
// Volume config (namespace/requestName) is not used, as it can be stale when a PV is rebound to a different PVC (e.g. KubeVirt).
// On error the workqueue will retry; eventually age-based timeout or rejection applies.
func (c *TridentCrdController) getPVCForVolume(ctx context.Context, volExternal *storage.VolumeExternal, volumeName string) (*corev1.PersistentVolumeClaim, error) {
	Logx(ctx).Debug(">>>> getPVCForVolume")
	defer Logx(ctx).Debug("<<<< getPVCForVolume")
	if volExternal == nil {
		return nil, fmt.Errorf("invalid volume")
	}
	if volumeName == "" {
		return nil, fmt.Errorf("invalid volume name")
	}

	getter := c.getPVCGetter(ctx)
	if getter != nil {
		pvc, err := getter.GetPVCForPV(ctx, volumeName)
		if err == nil {
			return pvc, nil
		}
		Logx(ctx).WithError(err).WithField("volumeName", volumeName).Debug("Failed to resolve PVC from PV, returning error for retry")
		return nil, err
	}

	// Resolve PVC from PV via Kubernetes API: get PV, then get PVC from Spec.ClaimRef (current binding).
	pv, err := c.kubeClientset.CoreV1().PersistentVolumes().Get(ctx, volumeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if pv.Spec.ClaimRef == nil {
		return nil, fmt.Errorf("PV %s has no claimRef (not bound)", volumeName)
	}
	namespace := pv.Spec.ClaimRef.Namespace
	name := pv.Spec.ClaimRef.Name
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("PV %s claimRef missing namespace or name", volumeName)
	}
	pvc, err := c.kubeClientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

// patchPVCSize patches the PVC's spec.resources.requests.storage with the exact new size (bytes).
// Logs show both human-readable and bytes for observability.
func (c *TridentCrdController) patchPVCSize(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
	Logx(ctx).Debug(">>>> patchPVCSize")
	defer Logx(ctx).Debug("<<<< patchPVCSize")
	pvcCopy := pvc.DeepCopy()
	pvcCopy.Spec.Resources.Requests[corev1.ResourceStorage] = newSize
	readableSizeStr := capacity.QuantityToHumanReadableString(newSize)

	_, err := c.kubeClientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Update(ctx, pvcCopy, metav1.UpdateOptions{})
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"pvc":          pvc.Name,
			"newSize":      readableSizeStr,
			"newSizeBytes": newSize.Value(),
			"error":        err,
		}).Error("Failed to patch PVC size")
		return err
	}

	Logx(ctx).WithFields(LogFields{
		"pvc":          pvc.Name,
		"newSize":      readableSizeStr,
		"newSizeBytes": newSize.Value(),
	}).Info("Successfully patched PVC size")

	return nil
}

// checkPVCResizeStatus checks if the PVC resize has failed or succeeded by examining PVC conditions and events.
// Makes a single API call to fetch events, then checks for both failure and success indicators.
// Returns (resizeFailed bool, resizeSuccessful bool, errorMessage string).
// Does not categorize failures - all resize failures should mark TAGRI as Failed and wait for timeout.
func (c *TridentCrdController) checkPVCResizeStatus(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (bool, bool, string) {
	Logx(ctx).Debug(">>>> checkPVCResizeStatus")
	defer Logx(ctx).Debug("<<<< checkPVCResizeStatus")
	// Step 1: Check PVC status conditions for terminal resize failure states
	// Kubernetes sets these conditions when CSI driver reports resize failures
	for _, condition := range pvc.Status.Conditions {
		// Check for resize failure conditions (available in newer K8s versions)
		if condition.Type == corev1.PersistentVolumeClaimResizing && condition.Status == corev1.ConditionFalse {
			// Resizing condition is False indicates resize has failed
			return true, false, fmt.Sprintf("PVC condition %s is False: %s", condition.Type, condition.Message)
		}

		// Check for file system resize failure
		if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending && condition.Status == corev1.ConditionFalse {
			// FileSystemResizePending is False indicates filesystem resize has failed
			return true, false, fmt.Sprintf("PVC condition %s is False: %s", condition.Type, condition.Message)
		}
	}

	// Step 2: Fetch PVC events once (single API call)
	events, err := c.kubeClientset.CoreV1().Events(pvc.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=PersistentVolumeClaim", pvc.Name),
	})
	if err != nil {
		Logx(ctx).WithError(err).Warn("Failed to get PVC events for resize status check")
		return false, false, ""
	}

	// Step 3: Sort by LastTimestamp descending (most recent first) so we use only the latest resize outcome.
	// Events can include older resizes (previous autogrow or manual resize); the most recent terminal
	// event (VolumeResizeFailed or VolumeResizeSuccessful) reflects the current attempt.
	items := events.Items
	sort.Slice(items, func(i, j int) bool {
		ti, tj := items[i].LastTimestamp.Time, items[j].LastTimestamp.Time
		return ti.After(tj)
	})

	// Step 4: Take the first (most recent) terminal event; ignore older success/failure from other resizes.
	for i := 0; i < len(items) && i < MaxEventsToCheckForResize; i++ {
		event := items[i]
		if event.Reason == "VolumeResizeFailed" {
			Logx(ctx).WithFields(LogFields{
				"pvc":     pvc.Name,
				"reason":  event.Reason,
				"message": event.Message,
			}).Warn("Detected most recent PVC resize failure event")
			return true, false, event.Message
		}
		if event.Reason == "VolumeResizeSuccessful" || event.Reason == "FileSystemResizeSuccessful" {
			Logx(ctx).WithFields(LogFields{
				"pvc":     pvc.Name,
				"reason":  event.Reason,
				"message": event.Message,
			}).Debug("Detected most recent PVC resize success event")
			return false, true, ""
		}
	}

	return false, false, ""
}

// rejectTagri marks a TAGRI as Rejected due to validation errors.
// The TAGRI will be deleted immediately by handleTimeoutAndTerminalPhase() on next reconcile.
// Observability is provided via TridentVolume autogrow status and K8s Events.
func (c *TridentCrdController) rejectTagri(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	pvc *corev1.PersistentVolumeClaim,
	reason string) error {
	Logx(ctx).Debug(">>>> rejectTagri")
	defer Logx(ctx).Debug("<<<< rejectTagri")
	Logx(ctx).WithFields(LogFields{
		"tagri":  tagri.Name,
		"reason": reason,
	}).Info("Rejecting TAGRI")

	// Update TAGRI status to Rejected first. Only after this succeeds do we update tvol and emit event,
	// so that retries (e.g. on conflict) do not double-increment TotalAutogrowAttempted.
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseRejected
	tagriCopy.Status.Message = reason
	now := metav1.NewTime(time.Now())
	tagriCopy.Status.ProcessedAt = &now
	if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status")
	}

	// Update TridentVolume (best effort)
	if volExternal, err := c.orchestrator.GetVolume(ctx, tagri.Spec.Volume); err == nil {
		err = c.updateVolumeAutogrowStatusAttempt(ctx, volExternal, "", reason, tagri)
		if err != nil {
			Logx(ctx).WithError(err).Warn("Failed to update TridentVolume autogrow status (best effort); ignoring error to avoid blocking TAGRI lifecycle")
		}
	}

	// Emit event on PVC for observability
	if pvc != nil {
		c.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonAutogrowRejected,
			fmt.Sprintf("Autogrow request rejected for volume %s: %s", tagri.Spec.Volume, reason))
	}

	return nil
}

// tryCompleteTagriIfResizeReachedTarget checks if the PVC's current capacity has reached the TAGRI's
// target (FinalCapacityBytes). If volExternal and pvc are nil, they are fetched using tagri.Spec.Volume.
// If capacity >= target, the TAGRI is marked Completed and (true, nil) or (true, err) is returned.
// Otherwise (false, nil) is returned. Used by handleTimeoutAndTerminalPhase (Failed recovery) and
// by monitorTagriResize for all success paths.
func (c *TridentCrdController) tryCompleteTagriIfResizeReachedTarget(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	volExternal *storage.VolumeExternal,
	pvc *corev1.PersistentVolumeClaim,
	logContext string,
) (completed bool, err error) {
	Logx(ctx).Debug(">>>> tryCompleteTagriIfResizeReachedTarget")
	defer Logx(ctx).Debug("<<<< tryCompleteTagriIfResizeReachedTarget")
	if tagri.Status.FinalCapacityBytes == "" {
		return false, nil
	}
	if volExternal == nil || pvc == nil {
		var volErr error
		volExternal, volErr = c.orchestrator.GetVolume(ctx, tagri.Spec.Volume)
		if volErr != nil {
			return false, nil
		}
		pvc, volErr = c.getPVCForVolume(ctx, volExternal, tagri.Spec.Volume)
		if volErr != nil || pvc == nil || pvc.Status.Capacity == nil {
			return false, nil
		}
	}
	targetCapacity, parseErr := resource.ParseQuantity(tagri.Status.FinalCapacityBytes)
	if parseErr != nil {
		return false, nil
	}
	currentCapacity := pvc.Status.Capacity[corev1.ResourceStorage]
	if currentCapacity.Cmp(targetCapacity) < 0 {
		return false, nil
	}
	if logContext != "" {
		Logx(ctx).WithFields(LogFields{
			"tagri":           tagri.Name,
			"pvc":             pvc.Name,
			"currentCapacity": currentCapacity.String(),
			"targetCapacity":  targetCapacity.String(),
		}).Info(logContext)
	}
	targetCapacityStr := capacity.QuantityToHumanReadableString(targetCapacity)
	if err := c.completeTagriSuccess(ctx, tagri, volExternal, pvc, targetCapacityStr); err != nil {
		return true, err
	}
	return true, nil
}

// failTagri marks a TAGRI as Failed.
// The TAGRI will be deleted by handleTimeoutAndTerminalPhase() after the age-based timeout expires.
// This gives operators time to investigate and fix issues before the next TAGRI is created.
// Observability is provided via TridentVolume autogrow status and K8s Events on the PVC.
func (c *TridentCrdController) failTagri(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	volExternal *storage.VolumeExternal,
	pvc *corev1.PersistentVolumeClaim,
	reason string,
) error {
	Logx(ctx).Debug(">>>> failTagri")
	defer Logx(ctx).Debug("<<<< failTagri")
	Logx(ctx).WithFields(LogFields{
		"tagri":  tagri.Name,
		"reason": reason,
	}).Warn("Failing TAGRI")

	// Update TAGRI status to Failed first. Only after this succeeds do we update tvol and emit event,
	// so that retries (e.g. on conflict) do not double-increment TotalAutogrowAttempted.
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseFailed
	tagriCopy.Status.Message = reason
	now := metav1.NewTime(time.Now())
	tagriCopy.Status.ProcessedAt = &now
	if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status")
	}

	// Update TridentVolume (best effort)
	if volExternal != nil {
		if err := c.updateVolumeAutogrowStatusAttempt(ctx, volExternal, "", reason, tagri); err != nil {
			Logx(ctx).WithError(err).Warn("Failed to update TridentVolume autogrow status (best effort); ignoring error to avoid blocking TAGRI lifecycle")
		}
	}

	// Emit event on PVC for observability
	if pvc != nil {
		c.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonAutogrowFailed,
			fmt.Sprintf("Autogrow request failed for volume %s: %s", tagri.Spec.Volume, reason))
	}

	return nil
}

// completeTagriSuccess marks a TAGRI as Completed.
// The TAGRI will be deleted immediately by handleTimeoutAndTerminalPhase() on next reconcile.
// Observability is provided via TridentVolume autogrow status and K8s Events on the PVC.
// We persist TAGRI status first so that on conflict/requeue we don't double-count tvol TotalSuccessfulAutogrow/TotalAutogrowAttempted.
func (c *TridentCrdController) completeTagriSuccess(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	volExternal *storage.VolumeExternal,
	pvc *corev1.PersistentVolumeClaim,
	finalSize string,
) error {
	Logx(ctx).Debug(">>>> completeTagriSuccess")
	defer Logx(ctx).Debug("<<<< completeTagriSuccess")
	Logx(ctx).WithFields(LogFields{
		"tagri":     tagri.Name,
		"finalSize": finalSize,
	}).Info("TAGRI completed successfully")

	// Update TAGRI status to Completed first. Only after this succeeds do we update tvol and emit event,
	// so that retries (e.g. on conflict) do not double-increment TotalSuccessfulAutogrow/TotalAutogrowAttempted.
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseCompleted
	tagriCopy.Status.Message = fmt.Sprintf("Volume successfully auto-grown to %s", finalSize)
	now := metav1.NewTime(time.Now())
	tagriCopy.Status.ProcessedAt = &now
	if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status")
	}

	// Update TridentVolume with success (best effort) - only after TAGRI is persisted
	if volExternal != nil {
		if err := c.updateVolumeAutogrowStatusSuccess(ctx, volExternal, finalSize, tagri); err != nil {
			Logx(ctx).WithError(err).Warn("Failed to update TridentVolume success status (best effort); ignoring error to avoid blocking TAGRI lifecycle")
		}
	}

	// Emit success event on PVC for observability
	if pvc != nil {
		c.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonAutogrowSucceeded,
			fmt.Sprintf("Autogrow request succeeded: volume %s expanded to %s", tagri.Spec.Volume, finalSize))
	}

	return nil
}

// updateTagriStatus updates the status of a TAGRI CR.
func (c *TridentCrdController) updateTagriStatus(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
) (*tridentv1.TridentAutogrowRequestInternal, error) {
	Logx(ctx).Debug(">>>> updateTagriStatus")
	defer Logx(ctx).Debug("<<<< updateTagriStatus")
	// Check if TAGRI is being deleted - skip update to avoid race condition
	if !tagri.ObjectMeta.DeletionTimestamp.IsZero() {
		Logx(ctx).WithFields(LogFields{
			"tagri":             tagri.Name,
			"deletionTimestamp": tagri.ObjectMeta.DeletionTimestamp,
		}).Debug("TAGRI is being deleted, skipping status update to avoid race condition")
		return tagri, nil
	}

	updated, err := c.crdClientset.TridentV1().TridentAutogrowRequestInternals(tagri.Namespace).UpdateStatus(
		ctx, tagri, metav1.UpdateOptions{})
	if err != nil {
		// Handle race condition where TAGRI was deleted during update
		if k8sapierrors.IsConflict(err) && strings.Contains(err.Error(), "UID in object meta:") {
			Logx(ctx).WithFields(LogFields{
				"tagri": tagri.Name,
			}).Debug("TAGRI was deleted during status update (race condition), ignoring")
			return nil, nil
		}

		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"error": err,
		}).Error("Failed to update TAGRI status")
		return nil, err
	}

	Logx(ctx).WithFields(LogFields{
		"tagri": tagri.Name,
		"phase": tagri.Status.Phase,
	}).Debug("Updated TAGRI status")

	return updated, nil
}

// updateTagriCR updates the TAGRI CR itself (for finalizers, etc.).
func (c *TridentCrdController) updateTagriCR(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
) (*tridentv1.TridentAutogrowRequestInternal, error) {
	Logx(ctx).Debug(">>>> updateTagriCR")
	defer Logx(ctx).Debug("<<<< updateTagriCR")
	updated, err := c.crdClientset.TridentV1().TridentAutogrowRequestInternals(tagri.Namespace).Update(
		ctx, tagri, metav1.UpdateOptions{})
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"error": err,
		}).Error("Failed to update TAGRI CR")
		return nil, err
	}

	return updated, nil
}

// deleteTagriNow removes finalizers and deletes the TAGRI CR immediately.
// Use this when you want to unblock nodes from creating new TAGRIs.
func (c *TridentCrdController) deleteTagriNow(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).Debug(">>>> deleteTagriNow")
	defer Logx(ctx).Debug("<<<< deleteTagriNow")
	// Remove finalizers
	if tagri.HasTridentFinalizers() {
		tagriCopy := tagri.DeepCopy()
		tagriCopy.RemoveTridentFinalizers()
		if _, err := c.updateTagriCR(ctx, tagriCopy); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to remove finalizers")
		}
	}

	// Delete CR
	err := c.crdClientset.TridentV1().TridentAutogrowRequestInternals(tagri.Namespace).Delete(
		ctx, tagri.Name, metav1.DeleteOptions{})
	if err != nil && !k8sapierrors.IsNotFound(err) {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"error": err,
		}).Error("Failed to delete TAGRI")
		return errors.WrapWithReconcileDeferredError(err, "failed to delete TAGRI")
	}

	Logx(ctx).WithField("tagri", tagri.Name).Info("TAGRI deleted immediately")
	return nil
}

// updateVolumeAutogrowStatus updates the volume's autogrow status via the orchestrator, both in-memory and persistence layer.
func (c *TridentCrdController) updateVolumeAutogrowStatus(
	ctx context.Context,
	volExternal *storage.VolumeExternal,
	updateFunc func(*models.VolumeAutogrowStatus),
	logMsg string,
	logFields LogFields,
) error {
	Logx(ctx).Debug(">>>> updateVolumeAutogrowStatus")
	defer Logx(ctx).Debug("<<<< updateVolumeAutogrowStatus")
	if volExternal == nil || volExternal.Config == nil {
		return fmt.Errorf("invalid volume")
	}

	volumeName := volExternal.Config.Name
	status := volExternal.AutogrowStatus
	if status == nil {
		status = &models.VolumeAutogrowStatus{}
	}
	updateFunc(status)

	if err := c.orchestrator.UpdateVolumeAutogrowStatus(ctx, volumeName, status); err != nil {
		Logx(ctx).WithError(err).Warnf("Failed to update volume autogrow status (%s)", logMsg)
		return err
	}

	logFields["volume"] = volumeName
	Logx(ctx).WithFields(logFields).Debugf("Updated volume autogrow status (%s)", logMsg)
	return nil
}

// updateVolumeAutogrowStatusAttempt updates the volume's autogrow status when a TAGRI reaches a terminal
// state (Rejected or Failed). TotalAutogrowAttempted is incremented only on terminal state, not when
// entering InProgress; RetryCount and TAGRI phase capture in-progress retries. Best-effort: callers
// should ignore errors to avoid blocking TAGRI lifecycle on observability updates.
func (c *TridentCrdController) updateVolumeAutogrowStatusAttempt(
	ctx context.Context,
	volExternal *storage.VolumeExternal,
	proposedSize string,
	errorMsg string,
	tagri *tridentv1.TridentAutogrowRequestInternal,
) error {
	Logx(ctx).Debug(">>>> updateVolumeAutogrowStatusAttempt")
	defer Logx(ctx).Debug("<<<< updateVolumeAutogrowStatusAttempt")
	if tagri == nil {
		return nil
	}
	return c.updateVolumeAutogrowStatus(ctx, volExternal,
		func(status *models.VolumeAutogrowStatus) {
			now := time.Now()
			status.LastAutogrowPolicyUsed = tagri.Spec.AutogrowPolicyRef.Name
			status.LastAutogrowAttemptedAt = &now
			status.LastProposedSize = proposedSize
			status.LastError = errorMsg
			status.TotalAutogrowAttempted++
		},
		"attempt",
		LogFields{
			"proposedSize":  proposedSize,
			"totalAttempts": 0, // Will be set by helper after update
		},
	)
}

// updateVolumeAutogrowStatusSuccess updates the volume's autogrow status after successful resize.
// This is best-effort: callers should ignore errors to avoid blocking TAGRI completion on observability updates.
// Ensures fast success path so nodes are unblocked quickly to create new TAGRIs if needed.
func (c *TridentCrdController) updateVolumeAutogrowStatusSuccess(
	ctx context.Context,
	volExternal *storage.VolumeExternal,
	finalSize string,
	tagri *tridentv1.TridentAutogrowRequestInternal,
) error {
	Logx(ctx).Debug(">>>> updateVolumeAutogrowStatusSuccess")
	defer Logx(ctx).Debug("<<<< updateVolumeAutogrowStatusSuccess")
	if tagri == nil {
		return nil
	}
	return c.updateVolumeAutogrowStatus(ctx, volExternal,
		func(status *models.VolumeAutogrowStatus) {
			now := time.Now()
			status.LastAutogrowPolicyUsed = tagri.Spec.AutogrowPolicyRef.Name
			status.LastSuccessfulAutogrowAt = &now
			status.LastSuccessfulSize = finalSize
			status.LastError = ""
			status.TotalSuccessfulAutogrow++
			status.TotalAutogrowAttempted++
		},
		"success",
		LogFields{
			"finalSize":      finalSize,
			"totalSuccesses": 0, // Will be set by helper after update
		},
	)
}

func (c *TridentCrdController) deleteTagriHandler(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "eventType": EventDelete}).Debug(">>>> deleteTagriHandler")
	defer Logx(ctx).Debug("<<<< deleteTagriHandler")

	// When K8s sends us a delete event, we should clean up finalizers and let the CR be deleted
	if err := c.deleteTagriNow(ctx, tagri); err != nil {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"error": err,
		}).Error("Failed to delete TAGRI via delete event handler, will retry")
		return err
	}

	return nil
}
