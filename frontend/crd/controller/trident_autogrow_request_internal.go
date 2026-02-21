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

// tagriIsBeingDeleted returns true if the TAGRI has a non-zero DeletionTimestamp (is being deleted).
// Safe to call with nil tagri (returns false).
func (c *TridentCrdController) tagriIsBeingDeleted(tagri *tridentv1.TridentAutogrowRequestInternal) bool {
	if tagri == nil {
		return false
	}
	return !tagri.ObjectMeta.DeletionTimestamp.IsZero()
}

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
	Logx(ctx).WithFields(LogFields{"key": key, "eventType": eventType}).Info("Processing TAGRI.")

	// Get TAGRI from lister
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// Invalid key is a permanent error, will not get fixed on retry. Thus, log and ignore.
		Logx(ctx).WithField("key", key).WithError(err).Error("Invalid key, ignoring.")
		return nil
	}

	tagri, err := c.autogrowRequestInternalLister.TridentAutogrowRequestInternals(namespace).Get(name)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			Logx(ctx).WithField("key", key).WithError(err).Debug("TAGRI in work queue no longer exists, ignoring.")
			return nil
		}
		// Lister error (transient) - retry with exponential backoff
		return errors.WrapWithReconcileDeferredError(err, "failed to get TAGRI from lister, will retry")
	}

	// Delete can be delivered as Update when the object gets DeletionTimestamp set; route to delete handler.
	if c.tagriIsBeingDeleted(tagri) {
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
	}).Debug("Handling TAGRI event.")

	switch eventType {
	case EventAdd, EventForceUpdate, EventUpdate:
		return c.upsertTagriHandler(ctx, tagri)
	case EventDelete:
		return c.deleteTagriHandler(ctx, tagri)
	default:
		// Unknown event type: ignore this enqueue.
		Logx(ctx).WithField("eventType", eventType).Debug("Unknown event type, ignoring.")
		return nil
	}
}

func (c *TridentCrdController) upsertTagriHandler(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> upsertTagriHandler")
	defer Logx(ctx).WithField("tagri", tagri.Name).Debug("<<<< upsertTagriHandler")

	// Handle TAGRI timeout and terminal phase deletion.
	// When handled is true, we either deleted the TAGRI (err=nil) or we're in a terminal/timeout path and requeuing (err!=nil).
	// Hence, no need of continuing to next step of phase routing (first-time or monitoring).
	// When handled is false and err is nil, continue to phase routing (first-time or monitoring).
	handled, err := c.handleTimeoutAndTerminalPhase(ctx, tagri)
	if handled {
		if err != nil {
			Logx(ctx).WithFields(LogFields{
				"tagri": tagri.Name,
				"phase": tagri.Status.Phase,
			}).Debug("TAGRI in terminal or timeout path, requeuing for next steps.")
			return errors.WrapWithReconcileDeferredError(err, "TAGRI handled in timeout or terminal path, requeuing for next steps")
		}
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"phase": tagri.Status.Phase,
		}).Debug("TAGRI already handled (deleted), nothing more required.")
		return nil
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to do TAGRI timeout or terminal phase based deletion")
	}

	// Route based on current phase
	if tagri.Status.Phase == TagriPhaseInProgress {
		// MONITORING MODE
		return c.monitorTagriResize(ctx, tagri)
	}

	// FIRST-TIME PROCESSING (Pending or empty phase)
	return c.processTagriFirstTime(ctx, tagri)
}

// handleTimeoutAndTerminalPhase runs timeout check first, then terminal-phase handling (Completed, Rejected, Failed).
// Returns (handled, err).
// When handled is true, the caller must stop and return as there is nothing more to do (we either deleted the TAGRI or deferred with error).
// When handled is false and err is nil, continue to next step i.e phase routing (processTagriFirstTime or monitorTagriResize).
//
// Terminal phase behavior (idempotency preserved in all cases):
//   - Rejected: hold until age-based timeout (STEP 3); operator can see status.message. Rejected conditions
//     are not re-evaluated (they are permanent for this CR); after timeout the Trident node can create a new
//     TAGRI if the underlying issue is fixed. We rely on Rejected status in the TAGRI CR to prevent further processing
//     and just wait for timeout to occur.
//   - Completed: delete in same reconciliation (STEP 2 or via markTagriCompleteAndDelete). No requeue for
//     deletion; if delete fails we requeue and next run sees Phase=Completed and only deletes (idempotent).
//   - Failed→recovery: when resize later succeeds we mark TAGRI as Complete and delete immediately as part of
//     the same reconciliation (run markTagriCompleteAndDelete).
//   - Failed: when resize has not recovered, wait for age-based timeout.
func (c *TridentCrdController) handleTimeoutAndTerminalPhase(
	ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) (handled bool, err error) {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> handleTimeoutAndTerminalPhase")
	defer Logx(ctx).Debug("<<<< handleTimeoutAndTerminalPhase")

	// STEP 1: Check age-based timeout (ALWAYS FIRST, applies to ALL phases)
	// This is the safety net that prevents TAGRIs from being stuck indefinitely in any phase.
	tagriTimeout := config.GetTagriTimeout()
	Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "tagriTimeout": tagriTimeout.Truncate(time.Second)}).Debug("TAGRI timeout (age-based).")
	if tagri.CreationTimestamp.IsZero() {
		// No creation time (e.g. test object or malformed): we cannot enforce timeout, and it won't recover.
		// Delete so the TAGRI does not float indefinitely; the node can create a new TAGRI if needed.
		Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "phase": tagri.Status.Phase}).Warn("TAGRI has zero CreationTimestamp, deleting to avoid stuck CR.")
		return true, c.deleteTagriNow(ctx, tagri)
	} else {
		age := time.Since(tagri.CreationTimestamp.Time)
		if age >= tagriTimeout {
			Logx(ctx).WithFields(LogFields{
				"tagri":        tagri.Name,
				"phase":        tagri.Status.Phase,
				"age":          age.Truncate(time.Second),
				"tagriTimeout": tagriTimeout.Truncate(time.Second),
			}).Info("TAGRI exceeded timeout, deleting immediately.")
			return true, c.deleteTagriNow(ctx, tagri)
		}
	}

	// STEP 2: Completed — mark it as handled and delete immediately
	if tagri.Status.Phase == TagriPhaseCompleted {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"phase": tagri.Status.Phase,
		}).Debug("TAGRI is in Completed phase, deleting immediately.")
		return true, c.deleteTagriNow(ctx, tagri)
	}

	// STEP 3: Rejected — hold until age-based timeout.
	// Rejected is only used for irrecoverable conditions; no reprocessing. After timeout
	// the node can create a new TAGRI if the underlying issue is fixed. Schedule next run at timeout via AddAfter.
	if tagri.Status.Phase == TagriPhaseRejected {
		timeUntilTimeout := tagriTimeout - time.Since(tagri.CreationTimestamp.Time)
		if timeUntilTimeout <= 0 {
			// Ideally we should not have come here as STEP 1 should have handled this.
			Logx(ctx).WithFields(LogFields{
				"tagri": tagri.Name, "phase": tagri.Status.Phase, "message": tagri.Status.Message,
			}).Info("TAGRI in Rejected phase at or past timeout, deleting immediately.")
			return true, c.deleteTagriNow(ctx, tagri)
		}
		Logx(ctx).WithFields(LogFields{
			"tagri":        tagri.Name,
			"phase":        tagri.Status.Phase,
			"message":      tagri.Status.Message,
			"tagriTimeout": tagriTimeout.Truncate(time.Second),
		}).Debug("TAGRI is in Rejected phase, waiting for timeout based deletion.")
		return true, errors.ReconcileDeferredWithDuration(timeUntilTimeout,
			"TAGRI rejected, waiting for timeout based deletion (%s)", tagriTimeout.Truncate(time.Second))
	}

	// STEP 4: Failed phase - wait for age-based timeout, unless resize has since succeeded
	// If we previously marked Failed (e.g. after max resize retries) but resize later succeeded
	// (e.g. backend came back online), transition to Completed so we delete immediately.
	if tagri.Status.Phase == TagriPhaseFailed {
		completed, err := c.completeAndDeleteTagriIfCapacityAtTarget(ctx, tagri, nil, nil, "TAGRI was Failed but resize has since succeeded")
		if err != nil {
			return true, errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed after recovery")
		}
		if completed {
			return true, nil
		}
		// No error; capacity not at target yet. Requeue with rate-limited backoff but cap by timeUntilTimeout
		// so tagriTimeout is strictly adhered (next run is never scheduled past timeout).
		timeUntilTimeout := tagriTimeout - time.Since(tagri.CreationTimestamp.Time)
		if timeUntilTimeout <= 0 {
			// Ideally we should not have come here as STEP 1 should have handled this.
			Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "phase": tagri.Status.Phase}).
				Info("TAGRI in Failed phase at or past timeout, deleting immediately.")
			return true, c.deleteTagriNow(ctx, tagri)
		}
		return true, errors.ReconcileDeferredWithMaxDuration(timeUntilTimeout,
			"TAGRI in Failed phase, waiting for recovery or timeout based deletion (%s)", tagriTimeout.Truncate(time.Second))
	}

	// STEP 5: Active phases (Pending/InProgress/empty) - continue processing
	// These will be monitored and eventually timeout via age-based timeout if stuck
	if tagri.Status.Phase == TagriPhasePending || tagri.Status.Phase == TagriPhaseInProgress || tagri.Status.Phase == "" {
		// Return handled=false so caller continues to phase-based routing (processTagriFirstTime or monitorTagriResize)
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"phase": tagri.Status.Phase,
		}).Debug("TAGRI in active phase, continuing to reconciliation.")
		return false, nil
	}

	// STEP 6: Unknown/unexpected phase - defer processing. No recovery path; schedule next run at timeout via AddAfter.
	timeUntilTimeout := tagriTimeout - time.Since(tagri.CreationTimestamp.Time)
	if timeUntilTimeout <= 0 {
		// Ideally we should not have come here as STEP 1 should have handled this.
		Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "phase": tagri.Status.Phase}).
			Info("TAGRI has unknown phase at or past timeout, deleting immediately.")
		return true, c.deleteTagriNow(ctx, tagri)
	}
	Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "phase": tagri.Status.Phase}).
		Warn("TAGRI has unknown phase; deferring reconciliation, age based timeout will clean up.")
	return true, errors.ReconcileDeferredWithDuration(timeUntilTimeout,
		"TAGRI has unknown phase: %s, waiting for timeout based deletion (%s)", tagri.Status.Phase, tagriTimeout.Truncate(time.Second))
}

// processTagriFirstTime handles the first-time processing of a TAGRI:
// validates policy, calculates size, patches PVC, and starts monitoring.
func (c *TridentCrdController) processTagriFirstTime(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).WithField("tagri", tagri.Name).Debug(">>>> processTagriFirstTime")
	defer Logx(ctx).Debug("<<<< processTagriFirstTime")

	// Step 1: Validate the field observedCapacityBytes provided by node (total size host sees; required for safe growth; prevents shrinkage/data loss).
	// Done first so invalid TAGRIs are rejected before any orchestrator or API calls.
	if tagri.Spec.ObservedCapacityBytes == "" {
		if err := c.rejectTagri(ctx, tagri, nil, "observedCapacityBytes is required"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); missing observedCapacityBytes", config.GetTagriTimeout().Truncate(time.Second))
	}
	observedCapacity, parseErr := resource.ParseQuantity(tagri.Spec.ObservedCapacityBytes)
	if parseErr != nil {
		if rejectErr := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("invalid observedCapacityBytes %q: %v", tagri.Spec.ObservedCapacityBytes, parseErr)); rejectErr != nil {
			return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), parseErr)
	}
	if observedCapacity.Sign() <= 0 {
		if err := c.rejectTagri(ctx, tagri, nil, "observedCapacityBytes must be greater than 0"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); observedCapacityBytes must be greater than 0", config.GetTagriTimeout().Truncate(time.Second))
	}

	// Step 2: Get TridentVolume from core cache
	volExternal, err := c.orchestrator.GetVolume(ctx, tagri.Spec.Volume)
	if err != nil {
		if errors.IsNotFoundError(err) {
			if rejectErr := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("volume %s not found", tagri.Spec.Volume)); rejectErr != nil {
				return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
			}
			return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), err)
		}
		return errors.WrapWithReconcileDeferredError(errors.WrapWithNotFoundError(err, "volume %s not found", tagri.Spec.Volume), "failed to get volume")
	}

	// Step 3: Get PVC corresponding to the volume -
	// This is needed to check PVC current capacity and emit event on the PVC when required
	pvc, err := c.getPVCForVolume(ctx, volExternal, tagri.Spec.Volume)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			if rejectErr := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("PVC not found for volume %s", tagri.Spec.Volume)); rejectErr != nil {
				return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
			}
			return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), err)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get PVC")
	}

	// Step 4: Validate effective policy (from orchestrator cache; the policy actually applied to the volume)
	effectivePolicy := volExternal.EffectiveAutogrowPolicy
	if effectivePolicy.PolicyName == "" {
		// EffectiveAutogrowPolicy is empty in controller cache. So either there is no effective policy
		// for the volume (e.g. created without one, or policy removed), or controller cache / node
		// autogrow info is stale. We treat the Trident controller as source of truth: reject this TAGRI
		// and hold until timeout. Node and controller will eventually reflect correct state; the node
		// can then create a new TAGRI if appropriate.
		Logx(ctx).WithFields(LogFields{
			"volume":          tagri.Spec.Volume,
			"requestedPolicy": tagri.Spec.AutogrowPolicyRef.Name,
		}).Warn("TAGRI rejected, volume has no effective autogrow policy configured.")

		if err := c.rejectTagri(ctx, tagri, pvc, "no effective autogrow policy found for this volume"); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); no effective autogrow policy found for volume %s", config.GetTagriTimeout().Truncate(time.Second), tagri.Spec.Volume)
	}

	if effectivePolicy.PolicyName != tagri.Spec.AutogrowPolicyRef.Name {
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("autogrow policy mismatch: effective=%s, requested=%s",
			effectivePolicy.PolicyName, tagri.Spec.AutogrowPolicyRef.Name)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); autogrow policy mismatch: effective=%s, requested=%s", config.GetTagriTimeout().Truncate(time.Second), effectivePolicy.PolicyName, tagri.Spec.AutogrowPolicyRef.Name)
	}

	// Step 5: Validate autogrow policy existence and Success state from core cache (orchestrator).
	// Policy state in the CR may not be persisted or propagated yet when read from the lister;
	// the orchestrator's cache reflects the authoritative state after sync, so we check it first.
	orchPolicy, err := c.orchestrator.GetAutogrowPolicy(ctx, tagri.Spec.AutogrowPolicyRef.Name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// Policy not in core - may be still syncing or deleted. Retry with backoff.
			return errors.ReconcileDeferredError("autogrow policy %s not found, will retry with exponential backoff", tagri.Spec.AutogrowPolicyRef.Name)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get autogrow policy")
	}
	if !orchPolicy.State.IsSuccess() {
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("autogrow policy is not in %s state: current state is=%s", storage.AutogrowPolicyStateSuccess, orchPolicy.State)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); autogrow policy %s is not in Success state, current state is=%s", config.GetTagriTimeout().Truncate(time.Second), tagri.Spec.AutogrowPolicyRef.Name, orchPolicy.State)
	}

	// Step 6: Get autogrow policy CR from lister for generation check (CR is source of truth for metadata.generation)
	policy, err := c.autogrowPoliciesLister.Get(tagri.Spec.AutogrowPolicyRef.Name)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			// CR not in lister yet (e.g. informer lag) although orchestrator has policy; retry with backoff.
			// Idempotent: no state written yet; next run re-processes from Step 1.
			return errors.ReconcileDeferredError("autogrow policy CR %s not found in lister, will retry with exponential backoff", tagri.Spec.AutogrowPolicyRef.Name)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get autogrow policy CR from lister")
	}

	// Step 7: Validate generation (CR generation must match TAGRI's policy ref)
	if policy.Generation != tagri.Spec.AutogrowPolicyRef.Generation {
		if err := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("autogrow policy generation mismatch: current=%d, requested=%d",
			policy.Generation, tagri.Spec.AutogrowPolicyRef.Generation)); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(),
			"TAGRI rejected, waiting for timeout based deletion (%s); autogrow policy %s generation mismatch: current=%d, requested=%d",
			config.GetTagriTimeout().Truncate(time.Second),
			tagri.Spec.AutogrowPolicyRef.Name,
			policy.Generation,
			tagri.Spec.AutogrowPolicyRef.Generation)
	}

	// Step 8: Get backend resize delta (if any), then calculate final capacity in one place (policy + delta + maxSize).
	// Every TAGRI is a fresh look: use only current PVC actual size (status.capacity) for calculation.
	// If status.capacity is nil, retry with backoff: PVC is from the lister and may be stale, or binding may be in progress.
	// Idempotent: no state written; next run re-processes from Step 1. Age-based timeout cleans up if capacity never appears.
	if pvc.Status.Capacity == nil {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"pvc":   pvc.Name,
		}).Warn("PVC capacity not yet reported, will retry with backoff.")
		return errors.ReconcileDeferredError("PVC %s capacity not yet reported, will retry with backoff", pvc.Name)
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
		}).Warn("Unable to get resize delta, will retry with backoff.")
		return errors.WrapWithReconcileDeferredError(err, "unable to get resize delta, will retry with backoff")
	}

	// Subordinate volumes can only be expanded up to the size of their source (parent) volume.
	var sourceVolumeSize string
	var readableSourceSizeStr string
	if volExternal.State.IsSubordinate() {
		Logx(ctx).WithFields(LogFields{"volume": tagri.Spec.Volume}).Debug("Subordinate volume; fetching source volume size.")
		sourceVol, err := c.orchestrator.GetSubordinateSourceVolume(ctx, tagri.Spec.Volume)
		if err != nil {
			if errors.IsNotFoundError(err) {
				if rejectErr := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("source volume for subordinate volume %s not found", tagri.Spec.Volume)); rejectErr != nil {
					return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
				}
				return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), err)
			}
			return errors.WrapWithReconcileDeferredError(errors.WrapWithNotFoundError(err, "source volume for subordinate volume %s not found", tagri.Spec.Volume), "failed to get source volume for subordinate")
		}
		if sourceVol == nil || sourceVol.Config == nil || sourceVol.Config.Size == "" {
			if rejectErr := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("subordinate volume %s: source volume has no size", tagri.Spec.Volume)); rejectErr != nil {
				return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
			}
			return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); subordinate source size unknown", config.GetTagriTimeout().Truncate(time.Second))
		}
		sourceQty, parseErr := resource.ParseQuantity(sourceVol.Config.Size)
		if parseErr != nil {
			if rejectErr := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("subordinate volume %s: invalid source volume size %q", tagri.Spec.Volume, sourceVol.Config.Size)); rejectErr != nil {
				return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
			}
			return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), parseErr)
		}
		sourceVolumeSize = sourceVol.Config.Size
		readableSourceSizeStr = capacity.QuantityToHumanReadableString(sourceQty)
		Logx(ctx).WithFields(LogFields{
			"tagri":                 tagri.Name,
			"volume":                tagri.Spec.Volume,
			"sourceVolumeSize":      sourceVolumeSize,
			"readableSourceSizeStr": readableSourceSizeStr,
		}).Debug("Subordinate volume: successfully fetched source volume size.")
	}

	capacityResponse, err := c.calculateFinalCapacity(ctx, currentSize, policy, tagri.Spec.Volume, resizeDeltaBytes, sourceVolumeSize)
	if err != nil {
		// Size calculation error (e.g. current already at maxSize, stuck resize at max) — reject and hold until timeout
		if rejectErr := c.rejectTagri(ctx, tagri, pvc, fmt.Sprintf("failed to calculate final capacity: %v", err)); rejectErr != nil {
			return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
		}
		// Wrap so typed autogrow errors (e.g. AutogrowAlreadyAtMaxSizeError, AutogrowStuckResizeAtMaxSizeError) are preserved for callers/tests
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), err)
	}
	finalCapacity := capacityResponse.FinalCapacity

	// Step 9: Reject if final capacity would be below observed capacity (total size host sees); prevents volume shrinkage and data loss.
	if finalCapacity.Value() < observedCapacity.Value() {
		rejectReason := fmt.Sprintf("final capacity %d bytes would be less than observed capacity %s, may cause volume shrinkage",
			finalCapacity.Value(), tagri.Spec.ObservedCapacityBytes)
		Logx(ctx).WithFields(LogFields{
			"tagri":                 tagri.Name,
			"finalCapacityBytes":    tagri.Status.FinalCapacityBytes,
			"observedCapacityBytes": tagri.Spec.ObservedCapacityBytes,
			"finalCapacity":         finalCapacity.Value(),
			"observedCapacity":      observedCapacity.Value(),
		}).Warn("TAGRI rejected: " + rejectReason + ".")
		if rejectErr := c.rejectTagri(ctx, tagri, pvc, rejectReason); rejectErr != nil {
			return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
		}
		return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); final capacity below observed capacity", config.GetTagriTimeout().Truncate(time.Second))
	}

	// Human-readable only for display and logs; all actual operations use exact finalCapacity (bytes).
	readableCapacityStr := capacity.QuantityToHumanReadableString(finalCapacity)

	// Step 10: Check if already at or above target size
	if pvc.Status.Capacity != nil {
		currentCapacity := pvc.Status.Capacity[corev1.ResourceStorage]
		if currentCapacity.Cmp(finalCapacity) >= 0 {
			currentCapacityStr := capacity.QuantityToHumanReadableString(currentCapacity)
			Logx(ctx).WithFields(LogFields{
				"pvc":             pvc.Name,
				"currentCapacity": currentCapacityStr,
				"targetCapacity":  readableCapacityStr,
			}).Info("PVC already at or above target size, marking as completed.")

			// Success - mark as completed and delete
			return c.markTagriCompleteAndDelete(ctx, tagri, volExternal, pvc, readableCapacityStr)
		}
	}

	// Steps 11–13: TAGRI update count is at most 2 per reconciliation (for K8s API QPS).
	//   (1) Step 11: add finalizer if missing — must be before patch so we never have "PVC resized but TAGRI has no finalizer".
	//   (2) Either Step 13 (InProgress on patch success) or RetryCount (on patch failure).
	// We cannot coalesce (1) and (2) on the success path without adding finalizer after patch, which would be unsafe.
	// On patch failure we cannot coalesce (1) and (2) either: at Step 11 we don't know the patch will fail.
	if !tagri.HasTridentFinalizers() {
		tagriCopy := tagri.DeepCopy()
		tagriCopy.AddTridentFinalizers()
		tagri, err = c.updateTagriCR(ctx, tagriCopy)
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to add finalizer")
		}
	}

	// Step 12: Patch PVC with exact calculated size (bytes); display/log use human-readable. Skip if request already equals target.
	currentRequest := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if currentRequest.Cmp(finalCapacity) != 0 {
		err = c.patchPVCSize(ctx, pvc, finalCapacity)
	} else {
		Logx(ctx).WithFields(LogFields{
			"pvc":           pvc.Name,
			"requestedSize": readableCapacityStr,
		}).Debug("PVC request already at final capacity, skipping patch.")
		err = nil
	}
	if err != nil {
		// Check retry count
		if tagri.Status.RetryCount >= MaxTagriRetries {
			// PVC patch permanently failed after 5 attempts (~75s with exponential backoff)
			// This is likely a permanent issue (RBAC, API rejection, PVC state)
			// Mark as failed, emit event, and delete immediately to unblock node
			patchErr := err
			updatedTagri, failErr := c.failTagri(ctx, tagri, volExternal, pvc,
				fmt.Sprintf("failed to patch PVC %s after %d attempts: %v", pvc.Name, MaxTagriRetries, patchErr))
			if failErr != nil {
				return errors.WrapWithReconcileDeferredError(failErr, "failed to mark TAGRI as Failed; %v", patchErr)
			}
			// Delete immediately - use updated TAGRI so remove-finalizers update has current ResourceVersion (avoids 409 Conflict).
			return c.deleteTagriNow(ctx, updatedTagri)
		}

		// Increment retry and requeue with exponential backoff
		tagriCopy := tagri.DeepCopy()
		tagriCopy.Status.RetryCount++
		if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to update retry count")
		}

		return errors.WrapWithReconcileDeferredError(err, "PVC patch failed, retry %d/%d", tagriCopy.Status.RetryCount, MaxTagriRetries)
	}

	// Emit AutogrowTriggered event on PVC with details (volume, policy, target size; bump/cap when applicable).
	policyName := tagri.Spec.AutogrowPolicyRef.Name
	policyMaxSizeStr := strings.TrimSpace(policy.Spec.MaxSize)
	// Capped at source when we passed a subordinate ceiling and the applied cap equals it.
	cappedAtSourceSize := capacityResponse.CappedAtCustomCeiling
	c.emitPVCEventAutogrowTriggered(pvc, tagri.Spec.Volume, policyName, readableCapacityStr, capacityResponse.ResizeDeltaBumpBytes, capacityResponse.CappedAtPolicyMaxSize, policyMaxSizeStr, cappedAtSourceSize, readableSourceSizeStr)

	// Step 13: Update TAGRI status to InProgress (FinalCapacityBytes = value we requested for monitoring).
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseInProgress
	tagriCopy.Status.FinalCapacityBytes = fmt.Sprintf("%d", finalCapacity.Value())
	tagriCopy.Status.Message = c.buildTagriInProgressMessage(readableCapacityStr, capacityResponse.ResizeDeltaBumpBytes, capacityResponse.CappedAtPolicyMaxSize, policyMaxSizeStr, cappedAtSourceSize, readableSourceSizeStr)
	tagriCopy.Status.RetryCount = 0 // Reset for monitoring phase (used as "resize failure observed" count if resize fails)

	tagri, err = c.updateTagriStatus(ctx, tagriCopy)
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update status to InProgress")
	}
	if tagri == nil {
		// TAGRI was deleted during status update (race); nothing left to do
		return nil
	}

	// Step 14: Requeue to start monitoring. ReconcileIncompleteError = immediate requeue so we enter
	// monitorTagriResize without delay. (Ongoing "resize in progress" in monitorTagriResize uses
	// ReconcileDeferredError for rate-limited backoff to avoid hammering the API server.)
	return errors.ReconcileIncompleteError("starting resize monitoring for TAGRI %s", tagri.Name)
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
			if rejectErr := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("volume %s deleted during monitoring", tagri.Spec.Volume)); rejectErr != nil {
				return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
			}
			return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), err)
		}
		return errors.WrapWithReconcileDeferredError(errors.WrapWithNotFoundError(err, "volume %s not found during monitoring", tagri.Spec.Volume), "failed to get volume during monitoring")
	}

	// Step 2: Get PVC (fetch once and reuse)
	pvc, err := c.getPVCForVolume(ctx, volExternal, tagri.Spec.Volume)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			// PVC deleted during monitoring; mark TAGRI as Rejected (pvc is nil when NotFound)
			if rejectErr := c.rejectTagri(ctx, tagri, nil, fmt.Sprintf("PVC for volume %s deleted during monitoring", tagri.Spec.Volume)); rejectErr != nil {
				return errors.WrapWithReconcileDeferredError(rejectErr, "failed to reject TAGRI")
			}
			return errors.ReconcileDeferredWithDuration(config.GetTagriTimeout(), "TAGRI rejected, waiting for timeout based deletion (%s); %v", config.GetTagriTimeout().Truncate(time.Second), err)
		}
		return errors.WrapWithReconcileDeferredError(err, "failed to get PVC during monitoring")
	}

	// Step 3: Check PVC conditions and events for resize status (failures and success)
	// Single API call to fetch events, then check for both failure and success events
	resizeFailed, resizeSuccessful, errorMsg := c.checkPVCResizeStatus(ctx, pvc)

	// Check success first: if both success and failure events exist, success takes precedence
	// This handles cases where resize initially failed but succeeded on retry
	if resizeSuccessful {
		Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "pvc": pvc.Name}).Debug("Detected VolumeResizeSuccessful event, verifying capacity.")
		completed, err := c.completeAndDeleteTagriIfCapacityAtTarget(ctx, tagri, volExternal, pvc, "Resize completed successfully.")
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed")
		}
		if completed {
			Logx(ctx).WithFields(LogFields{
				"tagri": tagri.Name,
				"pvc":   pvc.Name,
			}).Debug("Resize completed successfully, TAGRI should be deleted by now; nothing more to do.")
			return nil // Tagri already deleted by now; nothing more to do
		}
		Logx(ctx).WithFields(LogFields{"tagri": tagri.Name, "pvc": pvc.Name}).Debug("VolumeResizeSuccessful event detected but capacity not yet updated, continuing to check capacity.")
	}

	// Check failure: we reach here when (a) most recent terminal event was VolumeResizeFailed,
	// or (b) most recent was success but capacity not yet updated (fell through from above).
	// Only in (a) do we enter the block below; in (b) resizeFailed is false and we fall through to check capacity.
	// When resizeFailed: reuse RetryCount (reset to 0 when we entered InProgress) to give transient CSI errors during resize
	// a chance to clear up before concluding the TAGRI as failed.
	// We only mark Failed after MaxTagriRetries observations; until then phase stays InProgress
	// and we keep re-checking (success event or capacity >= target will complete).
	if resizeFailed {
		// If capacity already >= target (e.g. retry succeeded and capacity updated), don't mark Failed - complete
		completed, err := c.completeAndDeleteTagriIfCapacityAtTarget(ctx, tagri, volExternal, pvc,
			"Volume resize failure event detected, but PVC capacity already at target, marking as completed.")
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed")
		}
		if completed {
			Logx(ctx).WithFields(LogFields{
				"tagri": tagri.Name,
				"pvc":   pvc.Name,
			}).Debug("Resize completed successfully, TAGRI should be deleted by now; nothing more to do.")
			return nil // Tagri already deleted by now; nothing more to do
		}

		// Increment RetryCount (same field as patch retries; in InProgress it means "resize failure observed" count) and persist
		tagriCopy := tagri.DeepCopy()
		tagriCopy.Status.RetryCount++
		if tagriCopy.Status.RetryCount < MaxTagriRetries {
			if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to update retry count (resize failure observed)")
			}
			Logx(ctx).WithFields(LogFields{
				"tagri":         tagri.Name,
				"pvc":           pvc.Name,
				"retryCount":    tagriCopy.Status.RetryCount,
				"maxRetryCount": MaxTagriRetries,
				"errorMsg":      errorMsg,
			}).Warn("Resize failure observed, will retry before concluding failed.")
			return errors.ReconcileDeferredError(
				"Resize failure observed, will retry before concluding failed; count %d/%d; %v",
				tagriCopy.Status.RetryCount, MaxTagriRetries, errorMsg)
		}

		// RetryCount reached max - conclude failed. Keep CR until timeout so operators can see what went wrong
		// and so Failed->recovery can run (CSI resize may still succeed before timeout).
		// Pass tagriCopy (with RetryCount already incremented to MaxTagriRetries) so the Failed status shows the final count.
		Logx(ctx).WithFields(LogFields{
			"tagri":         tagri.Name,
			"pvc":           pvc.Name,
			"maxRetryCount": MaxTagriRetries,
			"errorMsg":      errorMsg,
		}).Warn("CSI resize failed after max retry count, marking TAGRI as Failed.")
		if _, failErr := c.failTagri(ctx, tagriCopy, volExternal, pvc, fmt.Sprintf("CSI resize failure: %s", errorMsg)); failErr != nil {
			return errors.WrapWithReconcileDeferredError(failErr, "failed to mark TAGRI as Failed")
		}
		return errors.ReconcileDeferredError("TAGRI marked Failed, requeuing for recovery check or timeout based deletion; %v", errorMsg)
	}

	// Step 4: Check PVC capacity
	if pvc.Status.Capacity == nil {
		// Use ReconcileDeferredError to avoid hammering API server with immediate requeues
		return errors.ReconcileDeferredError("PVC %s capacity not yet reported, will retry with backoff", pvc.Name)
	}

	// Check if resize has reached target (reuse same helper as success/failure event paths)
	completed, err := c.completeAndDeleteTagriIfCapacityAtTarget(ctx, tagri, volExternal, pvc, "Resize completed successfully.")
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to mark TAGRI as completed")
	}
	if completed {
		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"pvc":   pvc.Name,
		}).Debug("Resize completed successfully, TAGRI should be deleted by now; nothing more to do.")
		return nil // Tagri already deleted by now; nothing more to do
	}

	targetCapacity, err := resource.ParseQuantity(tagri.Status.FinalCapacityBytes)
	if err != nil {
		// Invalid capacity - unlikely to happen in monitoring stage. Mark Failed and keep until timeout so operators can see status.
		if _, failErr := c.failTagri(ctx, tagri, volExternal, pvc, fmt.Sprintf("invalid target capacity: %v", err)); failErr != nil {
			return errors.WrapWithReconcileDeferredError(failErr, "failed to mark TAGRI as Failed")
		}
		return errors.WrapWithReconcileDeferredError(err, "TAGRI marked Failed, waiting for timeout based deletion")
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
	}).Debug("Resize still in progress, continuing to monitor.")

	return errors.ReconcileDeferredError("resize in progress: %s/%s",
		currentCapacity.String(), targetCapacity.String())
}

// calculateFinalCapacity delegates to autogrow.CalculateFinalCapacity (policy + optional resize delta + maxSize).
// When customCeilingSize is non-empty (e.g. subordinate volume cap at source size), effective max is min(policy maxSize, floor(customCeilingSize)).
// Builds FinalCapacityRequest, logs request and response, returns FinalCapacityResponse.
func (c *TridentCrdController) calculateFinalCapacity(
	ctx context.Context,
	currentSize resource.Quantity,
	policy *tridentv1.TridentAutogrowPolicy,
	volumeName string,
	resizeDeltaBytes int64,
	customCeilingSize string,
) (autogrow.FinalCapacityResponse, error) {
	Logx(ctx).Debug(">>>> calculateFinalCapacity")
	defer Logx(ctx).Debug("<<<< calculateFinalCapacity")

	req := autogrow.FinalCapacityRequest{
		CurrentSize:       currentSize,
		Policy:            policy,
		ResizeDeltaBytes:  resizeDeltaBytes,
		CustomCeilingSize: customCeilingSize,
	}
	requestLog := LogFields{
		"volume":           volumeName,
		"currentSize":      currentSize.String(),
		"currentSizeBytes": currentSize.Value(),
		"resizeDeltaBytes": resizeDeltaBytes,
	}
	if customCeilingSize != "" {
		requestLog["customCeilingSize"] = customCeilingSize
	}
	if policy != nil {
		requestLog["autogrowPolicyName"] = policy.Name
		requestLog["autogrowPolicyGrowthAmount"] = policy.Spec.GrowthAmount
		requestLog["autogrowPolicyMaxSize"] = policy.Spec.MaxSize
	}
	Logx(ctx).WithFields(requestLog).Debug("Final capacity request.")

	response, err := autogrow.CalculateFinalCapacity(req)
	if err != nil {
		Logx(ctx).WithError(err).Error("Failed to calculate final capacity.")
		return autogrow.FinalCapacityResponse{}, err
	}

	Logx(ctx).WithFields(LogFields{
		"volume":                   volumeName,
		"policyBasedCapacity":      capacity.QuantityToHumanReadableString(response.PolicyBasedCapacity),
		"policyBasedCapacityBytes": response.PolicyBasedCapacity.Value(),
		"finalCapacity":            capacity.QuantityToHumanReadableString(response.FinalCapacity),
		"finalCapacityBytes":       response.FinalCapacity.Value(),
		"resizeDeltaBumpBytes":     response.ResizeDeltaBumpBytes,
		"cappedAtPolicyMaxSize":    response.CappedAtPolicyMaxSize,
		"cappedAtCustomCeiling":    response.CappedAtCustomCeiling,
		"cappedAtBytes":            response.CappedAtBytes,
	}).Debug("Final capacity response.")

	return response, nil
}

// buildTagriInProgressMessage returns the TAGRI status message for InProgress phase: "PVC patched..." with bump/cap detail and values when applicable.
// When cappedAtSourceSize is true and sourceSizeStr is set, message indicates capping at source volume size (subordinate volumes).
func (c *TridentCrdController) buildTagriInProgressMessage(targetSizeStr string, resizeDeltaBumpBytes int64, cappedAtMaxSize bool, policyMaxSizeStr string, cappedAtSourceSize bool, sourceSizeStr string) string {
	const monitoring = "; monitoring resize progress (target: %s)"
	if cappedAtSourceSize && sourceSizeStr != "" {
		if resizeDeltaBumpBytes > 0 {
			bumpStr := capacity.BytesToDecimalSizeString(resizeDeltaBumpBytes)
			return fmt.Sprintf("PVC patched after bumping by resize delta of %s and capping till source volume size (%s)"+monitoring, bumpStr, sourceSizeStr, targetSizeStr)
		}
		return fmt.Sprintf("PVC patched after capping till source volume size (%s)"+monitoring, sourceSizeStr, targetSizeStr)
	}
	if resizeDeltaBumpBytes > 0 && cappedAtMaxSize {
		bumpStr := capacity.BytesToDecimalSizeString(resizeDeltaBumpBytes)
		maxStr := policyMaxSizeStr
		if maxStr == "" {
			maxStr = "policy max"
		}
		return fmt.Sprintf("PVC patched after bumping by resize delta of %s and capping till max size (%s)"+monitoring, bumpStr, maxStr, targetSizeStr)
	}
	if resizeDeltaBumpBytes > 0 {
		bumpStr := capacity.BytesToDecimalSizeString(resizeDeltaBumpBytes)
		return fmt.Sprintf("PVC patched after bumping by resize delta of %s"+monitoring, bumpStr, targetSizeStr)
	}
	if cappedAtMaxSize {
		maxStr := policyMaxSizeStr
		if maxStr == "" {
			maxStr = "policy max"
		}
		return fmt.Sprintf("PVC patched after capping till max size (%s)"+monitoring, maxStr, targetSizeStr)
	}
	return fmt.Sprintf("PVC patched"+monitoring, targetSizeStr)
}

// emitPVCEventAutogrowTriggered emits a Normal event on the PVC when the PVC has been successfully patched.
// Message includes volume, effective policy, target size; when applicable, the minimum resize delta bump
// (resizeDeltaBumpBytes), capping at policy maxSize (policyMaxSizeStr), and/or capping at source volume size (subordinate).
func (c *TridentCrdController) emitPVCEventAutogrowTriggered(
	pvc *corev1.PersistentVolumeClaim,
	volumeName, policyName, targetSizeStr string,
	resizeDeltaBumpBytes int64,
	cappedAtMaxSize bool,
	policyMaxSizeStr string,
	cappedAtSourceSize bool,
	sourceSizeStr string,
) {
	if pvc == nil {
		return
	}
	// Base message aligned with other autogrow events (e.g. AutogrowRejected: "volume %s with effective autogrow policy %s").
	msg := fmt.Sprintf("Autogrow triggered for volume %s with effective autogrow policy %s: expanding to %s", volumeName, policyName, targetSizeStr)
	var suffixes []string
	if resizeDeltaBumpBytes > 0 {
		bumpStr := capacity.BytesToDecimalSizeString(resizeDeltaBumpBytes)
		suffixes = append(suffixes, fmt.Sprintf("final size bumped by minimum resize delta %s required by backend", bumpStr))
	}
	if cappedAtSourceSize && sourceSizeStr != "" {
		suffixes = append(suffixes, fmt.Sprintf("capped at source volume size %s; subordinate volume cannot exceed source", sourceSizeStr))
	}
	if cappedAtMaxSize && !cappedAtSourceSize {
		if policyMaxSizeStr != "" {
			suffixes = append(suffixes, fmt.Sprintf("capped at policy maxSize %s; volume will not autogrow beyond this size", policyMaxSizeStr))
		} else {
			suffixes = append(suffixes, "capped at policy maxSize; volume will not autogrow beyond this size")
		}
	}
	if len(suffixes) > 0 {
		msg += " (" + strings.Join(suffixes, "; ") + ")"
	}
	c.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonAutogrowTriggered, msg)
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
		Logx(ctx).WithError(err).WithField("volumeName", volumeName).Debug("Failed to resolve PVC from PV, returning error for retry.")
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
		}).Error("Failed to patch PVC size.")
		return err
	}

	Logx(ctx).WithFields(LogFields{
		"pvc":          pvc.Name,
		"newSize":      readableSizeStr,
		"newSizeBytes": newSize.Value(),
	}).Info("Successfully patched PVC size.")

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

	// Step 2: Fetch PVC events once (single API call). Limit response size to avoid large payloads.
	events, err := c.kubeClientset.CoreV1().Events(pvc.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=PersistentVolumeClaim", pvc.Name),
		Limit:         int64(MaxEventsToCheckForResize), // we only inspect up to this many events after sort
	})
	if err != nil {
		Logx(ctx).WithError(err).Warn("Failed to get PVC events for resize status check.")
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
			}).Warn("Detected most recent PVC resize failure event.")
			return true, false, event.Message
		}
		if event.Reason == "VolumeResizeSuccessful" || event.Reason == "FileSystemResizeSuccessful" {
			Logx(ctx).WithFields(LogFields{
				"pvc":     pvc.Name,
				"reason":  event.Reason,
				"message": event.Message,
			}).Debug("Detected most recent PVC resize success event.")
			return false, true, ""
		}
	}

	return false, false, ""
}

// rejectTagri marks a TAGRI as Rejected due to validation or permanent conditions.
// Rejected is only used when the condition is irrecoverable for this TAGRI (no retry will fix it);
// the node can create a new TAGRI after the underlying issue is fixed. All current rejection paths are
// permanent: invalid/missing spec (observedCapacityBytes), volume/PVC not found or deleted, no effective
// policy or policy mismatch/generation/not Success, final capacity calculation error (e.g. already at max),
// or final capacity below observed (would shrink). Transient failures use ReconcileDeferredError and retry.
// It adds a finalizer when rejecting so the CR is held until timeout-based deletion (same as Failed).
// Observability: TridentVolume + K8s Events.
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
	}).Info("Rejecting TAGRI.")

	// Update TAGRI status to Rejected and add finalizer if missing (so CR is held until timeout, like Failed).
	// Only after this succeeds do we update tvol and emit event.
	// When the CRD has a status subresource, Update() does not persist status; we must use UpdateStatus() for phase/message.
	// So when adding a finalizer we do updateTagriCR (persists finalizer), then updateTagriStatus (persists Rejected).
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseRejected
	tagriCopy.Status.Message = reason
	now := metav1.NewTime(time.Now())
	tagriCopy.Status.ProcessedAt = &now
	addedFinalizer := !tagri.HasTridentFinalizers()
	if addedFinalizer {
		tagriCopy.AddTridentFinalizers()
		updated, err := c.updateTagriCR(ctx, tagriCopy)
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to add finalizer when rejecting TAGRI")
		}
		// Status is not persisted by Update() when CRD has status subresource; persist it explicitly.
		// Use updated (current ResourceVersion) and set status for UpdateStatus call.
		statusCopy := updated.DeepCopy()
		statusCopy.Status.Phase = TagriPhaseRejected
		statusCopy.Status.Message = reason
		statusCopy.Status.ProcessedAt = &now
		if _, err := c.updateTagriStatus(ctx, statusCopy); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status to Rejected")
		}
	} else {
		if _, err := c.updateTagriStatus(ctx, tagriCopy); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status")
		}
	}

	// Update TridentVolume (best effort)
	if volExternal, err := c.orchestrator.GetVolume(ctx, tagri.Spec.Volume); err == nil {
		err = c.updateVolumeAutogrowStatusAttempt(ctx, volExternal, "", reason, tagri)
		if err != nil {
			Logx(ctx).WithError(err).Warn("Failed to update TridentVolume autogrow status (best effort), ignoring error to avoid blocking TAGRI lifecycle.")
		}
	}

	// Emit event on PVC for observability (include policy name; reason may already mention it in some cases)
	if pvc != nil {
		policyName := tagri.Spec.AutogrowPolicyRef.Name
		c.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonAutogrowRejected,
			fmt.Sprintf("Autogrow request rejected for volume %s with effective autogrow policy %s: %s", tagri.Spec.Volume, policyName, reason))
	}

	return nil
}

// completeAndDeleteTagriIfCapacityAtTarget checks if the PVC's current capacity has reached the TAGRI's
// target (FinalCapacityBytes). If volExternal and pvc are nil, they are fetched using tagri.Spec.Volume.
// If capacity >= target, the TAGRI is marked Completed and deleted via markTagriCompleteAndDelete;
// returns (true, nil) or (true, err). Otherwise returns (false, nil).
func (c *TridentCrdController) completeAndDeleteTagriIfCapacityAtTarget(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	volExternal *storage.VolumeExternal,
	pvc *corev1.PersistentVolumeClaim,
	logContext string,
) (completed bool, err error) {
	Logx(ctx).Debug(">>>> completeTagriIfCapacityAtTarget")
	defer Logx(ctx).Debug("<<<< completeTagriIfCapacityAtTarget")
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
		}).Debug(logContext)
	}
	targetCapacityStr := capacity.QuantityToHumanReadableString(targetCapacity)
	if err := c.markTagriCompleteAndDelete(ctx, tagri, volExternal, pvc, targetCapacityStr); err != nil {
		return true, err
	}
	return true, nil
}

// failTagri marks a TAGRI as Failed.
// The TAGRI will be deleted by handleTimeoutAndTerminalPhase() after the age-based timeout expires.
// This gives operators time to investigate and fix issues before the next TAGRI is created.
// Observability is provided via TridentVolume autogrow status and K8s Events on the PVC.
// Returns the updated TAGRI (with current ResourceVersion) so callers that immediately delete
// (e.g. deleteTagriNow) use a fresh object and avoid 409 Conflict on the next update.
func (c *TridentCrdController) failTagri(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	volExternal *storage.VolumeExternal,
	pvc *corev1.PersistentVolumeClaim,
	reason string,
) (*tridentv1.TridentAutogrowRequestInternal, error) {
	Logx(ctx).Debug(">>>> failTagri")
	defer Logx(ctx).Debug("<<<< failTagri")
	Logx(ctx).WithFields(LogFields{
		"tagri":  tagri.Name,
		"reason": reason,
	}).Warn("Failing TAGRI.")

	// Update TAGRI status to Failed first. Only after this succeeds do we update tvol and emit event,
	// so that retries (e.g. on conflict) do not double-increment TotalAutogrowAttempted.
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseFailed
	tagriCopy.Status.Message = reason
	now := metav1.NewTime(time.Now())
	tagriCopy.Status.ProcessedAt = &now
	updated, err := c.updateTagriStatus(ctx, tagriCopy)
	if err != nil {
		return nil, errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status")
	}
	// Use updated TAGRI when available so callers get current ResourceVersion; otherwise use original (e.g. if CR was deleted during update).
	if updated == nil {
		updated = tagri
	}

	// Update TridentVolume (best effort)
	if volExternal != nil {
		if err := c.updateVolumeAutogrowStatusAttempt(ctx, volExternal, "", reason, tagri); err != nil {
			Logx(ctx).WithError(err).Warn("Failed to update TridentVolume autogrow status (best effort), ignoring error to avoid blocking TAGRI lifecycle.")
		}
	}

	// Emit event on PVC for observability
	if pvc != nil {
		policyName := tagri.Spec.AutogrowPolicyRef.Name
		c.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonAutogrowFailed,
			fmt.Sprintf("Autogrow request failed for volume %s with effective autogrow policy %s: %s", tagri.Spec.Volume, policyName, reason))
	}

	return updated, nil
}

// markTagriCompleteAndDelete marks the TAGRI as Completed and deletes it in the same reconciliation.
//
// The order of steps below is important and must be followed:
//  1. Remove finalizers (updateTagriCR) then persist Phase=Completed (updateTagriStatus). When the CRD has a status
//     subresource, Update() does not persist status, so we do both. If either fails, return — do not run steps 2–4.
//  2. Update TridentVolume autogrow status (best effort).
//  3. Emit success event on the PVC.
//  4. Delete the TAGRI (deleteTagriCROnly; no need to remove finalizers as they were removed in step 1).
//
// Idempotency: Step 1 ensures that if step 4 fails and the item is requeued, the next run sees Phase=Completed
// and only runs delete in handleTimeoutAndTerminalPhase — no second tvol update or PVC event.
// Observability: logging, TridentVolume autogrow status, and K8s Events on the PVC.
func (c *TridentCrdController) markTagriCompleteAndDelete(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
	volExternal *storage.VolumeExternal,
	pvc *corev1.PersistentVolumeClaim,
	finalSize string,
) error {
	Logx(ctx).Debug(">>>> markTagriCompleteAndDelete")
	defer Logx(ctx).Debug("<<<< markTagriCompleteAndDelete")

	// If TAGRI is already being deleted, skip all steps; treat work as done (no double tvol/event, no redundant delete).
	if c.tagriIsBeingDeleted(tagri) {
		Logx(ctx).WithFields(LogFields{
			"tagri":             tagri.Name,
			"deletionTimestamp": tagri.ObjectMeta.DeletionTimestamp,
		}).Debug("TAGRI is being deleted, skipping complete-and-delete steps; work considered done.")
		return nil
	}

	Logx(ctx).WithFields(LogFields{
		"tagri":     tagri.Name,
		"finalSize": finalSize,
	}).Info("TAGRI completed successfully.")

	// Remove finalizers first; status is not persisted by Update() when CRD has status subresource, so we persist it explicitly after.
	tagriCopy := tagri.DeepCopy()
	tagriCopy.Status.Phase = TagriPhaseCompleted
	tagriCopy.Status.Message = fmt.Sprintf("Volume successfully auto-grown to %s", finalSize)
	now := metav1.NewTime(time.Now())
	tagriCopy.Status.ProcessedAt = &now
	tagriCopy.RemoveTridentFinalizers()
	updated, err := c.updateTagriCR(ctx, tagriCopy)
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to remove finalizer when marking TAGRI completed")
	}
	// Persist status (Phase=Completed, Message, ProcessedAt) via UpdateStatus; use updated for current ResourceVersion.
	statusCopy := updated.DeepCopy()
	statusCopy.Status.Phase = TagriPhaseCompleted
	statusCopy.Status.Message = tagriCopy.Status.Message
	statusCopy.Status.ProcessedAt = &now
	if _, err := c.updateTagriStatus(ctx, statusCopy); err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update TAGRI status to Completed")
	}

	// Update TridentVolume with success (best effort)
	if volExternal != nil {
		if err := c.updateVolumeAutogrowStatusSuccess(ctx, volExternal, finalSize, tagri); err != nil {
			Logx(ctx).WithError(err).Warn("Failed to update TridentVolume success status (best effort), ignoring error to avoid blocking TAGRI lifecycle.")
		}
	}

	// Emit success event on PVC
	if pvc != nil {
		policyName := tagri.Spec.AutogrowPolicyRef.Name
		c.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonAutogrowSucceeded,
			fmt.Sprintf("Volume %s with effective policy %s autogrown to %s", tagri.Spec.Volume, policyName, finalSize))
	}

	// Finalizers already removed above; just delete.
	return c.deleteTagriCROnly(ctx, tagri.Namespace, tagri.Name)
}

// updateTagriStatus updates the status of a TAGRI CR.
func (c *TridentCrdController) updateTagriStatus(
	ctx context.Context,
	tagri *tridentv1.TridentAutogrowRequestInternal,
) (*tridentv1.TridentAutogrowRequestInternal, error) {
	Logx(ctx).Debug(">>>> updateTagriStatus")
	defer Logx(ctx).Debug("<<<< updateTagriStatus")
	// Check if TAGRI is being deleted - skip update to avoid race condition
	if c.tagriIsBeingDeleted(tagri) {
		Logx(ctx).WithFields(LogFields{
			"tagri":             tagri.Name,
			"deletionTimestamp": tagri.ObjectMeta.DeletionTimestamp,
		}).Debug("TAGRI is being deleted, skipping status update to avoid race condition.")
		return tagri, nil
	}

	updated, err := c.crdClientset.TridentV1().TridentAutogrowRequestInternals(tagri.Namespace).UpdateStatus(
		ctx, tagri, metav1.UpdateOptions{})
	if err != nil {
		// Handle race condition where TAGRI was deleted during update
		if k8sapierrors.IsConflict(err) && strings.Contains(err.Error(), "UID in object meta:") {
			Logx(ctx).WithFields(LogFields{
				"tagri": tagri.Name,
			}).Debug("TAGRI was deleted during status update (race condition), ignoring.")
			return nil, nil
		}

		Logx(ctx).WithFields(LogFields{
			"tagri": tagri.Name,
			"error": err,
		}).Error("Failed to update TAGRI status.")
		return nil, err
	}

	Logx(ctx).WithFields(LogFields{
		"tagri": tagri.Name,
		"phase": tagri.Status.Phase,
	}).Debug("Updated TAGRI status.")

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
		}).Error("Failed to update TAGRI CR.")
		return nil, err
	}

	return updated, nil
}

// deleteTagriCROnly performs the Delete API call only. Used when finalizers were already removed (e.g. in markTagriCompleteAndDelete).
func (c *TridentCrdController) deleteTagriCROnly(ctx context.Context, namespace, name string) error {
	Logx(ctx).WithFields(LogFields{"tagri": name, "namespace": namespace}).Debug(">>>> deleteTagriCROnly")
	defer Logx(ctx).WithFields(LogFields{"tagri": name, "namespace": namespace}).Debug("<<<< deleteTagriCROnly")
	err := c.crdClientset.TridentV1().TridentAutogrowRequestInternals(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8sapierrors.IsNotFound(err) {
		Logx(ctx).WithFields(LogFields{"tagri": name, "error": err}).Error("Failed to delete TAGRI.")
		return errors.WrapWithReconcileDeferredError(err, "failed to delete TAGRI")
	}
	if err == nil {
		Logx(ctx).WithField("tagri", name).Info("TAGRI deleted successfully.")
	}
	return nil
}

// deleteTagriNow removes finalizers (if any) and deletes the TAGRI CR.
// On 409 Conflict (object modified), returns ReconcileIncompleteError so the item is requeued
// immediately and the next reconciliation retries delete with fresh state from the lister.
func (c *TridentCrdController) deleteTagriNow(ctx context.Context, tagri *tridentv1.TridentAutogrowRequestInternal) error {
	Logx(ctx).Debug(">>>> deleteTagriNow")
	defer Logx(ctx).Debug("<<<< deleteTagriNow")

	if tagri.HasTridentFinalizers() {
		tagriCopy := tagri.DeepCopy()
		tagriCopy.RemoveTridentFinalizers()
		if _, err := c.updateTagriCR(ctx, tagriCopy); err != nil {
			if k8sapierrors.IsNotFound(err) {
				Logx(ctx).WithField("tagri", tagri.Name).Debug("TAGRI already deleted.")
				return nil
			}
			if k8sapierrors.IsConflict(err) {
				return errors.WrapWithReconcileIncompleteError(err, "TAGRI object modified, requeuing to retry delete")
			}
			return errors.WrapWithReconcileDeferredError(err, "failed to remove finalizers")
		}
	}
	return c.deleteTagriCROnly(ctx, tagri.Namespace, tagri.Name)
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
		Logx(ctx).WithError(err).Warnf("Failed to update volume autogrow status; %s.", logMsg)
		return err
	}

	logFields["volume"] = volumeName
	Logx(ctx).WithFields(logFields).Debugf("Updated volume autogrow status; %s.", logMsg)
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
	totalAttempts := 1
	if volExternal.AutogrowStatus != nil {
		totalAttempts = volExternal.AutogrowStatus.TotalAutogrowAttempted + 1
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
			"totalAttempts": totalAttempts,
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
	totalSuccesses, totalAttempts := 1, 1
	if volExternal.AutogrowStatus != nil {
		totalSuccesses = volExternal.AutogrowStatus.TotalSuccessfulAutogrow + 1
		totalAttempts = volExternal.AutogrowStatus.TotalAutogrowAttempted + 1
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
			"totalSuccesses": totalSuccesses,
			"totalAttempts":  totalAttempts,
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
		}).Error("Failed to delete TAGRI via delete event handler, will retry.")
		return err
	}

	return nil
}
