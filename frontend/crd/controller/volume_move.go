// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi"
	storageattribute "github.com/netapp/trident/storage_attribute"

	v1 "k8s.io/api/storage/v1"

	"github.com/netapp/trident/utils/models"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/errors"
)

// stateOutcome captures the result of processing a single TVM state.
// State handlers mutate the status in-place and return an outcome that
// tells the main handler whether to commit and/or processErr.
type stateOutcome struct {
	processErr error // non-nil = processErr with this error after committing
	skipCommit bool  // true = return immediately without committing state
}

// handleVolumeMove handles a TridentVolumeMove CR event.
func (c *TridentCrdController) handleVolumeMove(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	Logc(ctx).Debug(">>>> TridentCrdController#handleVolumeMove")
	defer Logc(ctx).Debug("<<<< TridentCrdController#handleVolumeMove")

	if ok := c.indexers.VolumeAttachmentIndexer().WaitForCacheSync(ctx); !ok {
		Logc(ctx).Warn("Failed to sync volume attachment cache. Not all volume attachments may be found.")
		return errors.ReconcileDeferredError("failed to sync volume attachment cache; requeuing...")
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key.")
		return err
	}

	tvm, err := c.validateVolumeMoveCR(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			if err = c.reconcileVolumeMove(ctx, name, nil); err != nil {
				return errors.WrapWithReconcileDeferredError(err, "failed to clear volume move info; requeuing...")
			}
			return nil
		}
		return err
	}
	status := &tvm.Status

	if err := c.reconcileAttachments(ctx, tvm.Name, status); err != nil {
		Logc(ctx).Errorf("Failed to reconcile attachments: %v", err)
		return fmt.Errorf("could not reconcile attachments: %w", err)
	}

	// Fill in the move info.
	moveInfo := &models.VolumeMoveInfo{
		State:              tvm.Status.State,
		VolumeName:         tvm.Name,
		SourceNode:         tvm.Spec.SourceNode,
		SourcePool:         tvm.Spec.SourcePool,
		TargetNode:         tvm.Spec.TargetNode,
		TargetPool:         tvm.Spec.TargetPool,
		InitialAccessInfo:  tvm.Status.InitialAccessInfo,
		TargetAccessInfo:   tvm.Status.TargetAccessInfo,
		BackendContext:     tvm.Status.BackendContext.Raw,
		DeleteAfterSuccess: tvm.Spec.GetDeleteAfterSuccess(),
	}

	outcome := c.processState(ctx, tvm, moveInfo, status)
	if outcome.skipCommit {
		return outcome.processErr
	}
	if err := c.commitState(ctx, tvm.Name, tvm.Namespace, moveInfo, status); err != nil {
		return fmt.Errorf("could not commit state: %w", err)
	}
	return outcome.processErr
}

func (c *TridentCrdController) processState(
	ctx context.Context, tvm *netappv1.TridentVolumeMove,
	moveInfo *models.VolumeMoveInfo, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	switch status.State {
	case models.VolumeMoveStatePending:
		return c.handleStatePending(ctx, moveInfo, status)
	case models.VolumeMoveStateControllerStaging:
		return c.handleStateControllerStaging(ctx, tvm, moveInfo, status)
	case models.VolumeMoveStateNodeStaging:
		return c.handleStateNodeStaging(ctx, status)
	case models.VolumeMoveStateMoving:
		return c.handleStateMoving(ctx, tvm, moveInfo, status)
	case models.VolumeMoveStateControllerUnstaging:
		return c.handleStateControllerUnstaging(ctx, tvm, moveInfo, status)
	case models.VolumeMoveStateNodeUnstaging:
		return c.handleStateNodeUnstaging(ctx, status)
	case models.VolumeMoveStateFailed:
		return c.handleStateFailed(status)
	case models.VolumeMoveStateSucceeded:
		return c.handleStateSucceeded(ctx, tvm, status)
	default:
		status.State = models.VolumeMoveStatePending
		return stateOutcome{}
	}
}

// commitState is the guaranteed epilogue that persists the volume config
// projection and the TVM CR status. It runs after every state handler unless
// the handler explicitly sets skipCommit.
func (c *TridentCrdController) commitState(
	ctx context.Context, name, namespace string,
	moveInfo *models.VolumeMoveInfo, status *netappv1.TridentVolumeMoveStatus,
) error {
	moveInfo.State = status.State
	var mi *models.VolumeMoveInfo
	if !status.State.IsTerminal() {
		mi = moveInfo
	}
	if err := c.reconcileVolumeMove(ctx, name, mi); err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to update volume move info")
	}
	return c.updateTridentVolumeMove(ctx, namespace, name, status)
}

// TVM State handlers
// Each handler mutates status in-place and returns a stateOutcome.

// handleStatePending attempts a dry-run if supported and transitions the TVM to ControllerStaging.
func (c *TridentCrdController) handleStatePending(
	ctx context.Context,
	moveInfo *models.VolumeMoveInfo, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	if err := validateDeleteAfterSuccess(moveInfo.DeleteAfterSuccess); err != nil {
		status.Message = err.Error()
		status.State = models.VolumeMoveStateFailed
		return stateOutcome{}
	}

	Logc(ctx).Debug("TVM is pending, starting dry-run.")
	if err := c.startVolumeMove(ctx, moveInfo, true); err != nil {
		if isTerminalVolumeMoveError(err) {
			status.Message = fmt.Sprintf("volume move dry run failed with error: %s", err.Error())
			status.State = models.VolumeMoveStateFailed
			return stateOutcome{}
		}
		return stateOutcome{processErr: errors.ReconcileDeferredError("dry-run transient error; requeuing: %s", err)}
	}

	// NOTE: attachments are intentionally seeded in ControllerStaging, not here.
	// The first TVM -> TVOL projection should land just after Pending but before
	// handling any logic for ControllerStaging.
	//
	// Any publish that raced before MoveInfo existed still owns a VolumeAttachment
	// captured by the snapshot, and any publish after is move aware via MoveInfo.
	status.StartTime = convert.ToPtr(metav1.Now())
	status.State = models.VolumeMoveStateControllerStaging
	return stateOutcome{}
}

func (c *TridentCrdController) handleStateControllerStaging(
	ctx context.Context, tvm *netappv1.TridentVolumeMove,
	moveInfo *models.VolumeMoveInfo, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	Logc(ctx).Debug("TVM is in controller staging state.")

	if err := c.populateAttachments(ctx, moveInfo.VolumeName, status); err != nil {
		return stateOutcome{processErr: errors.ReconcileDeferredError("could not populate attachments; requeuing: %s", err)}
	}

	if err := c.controllerStaging(ctx, moveInfo); err != nil {
		if isTerminalVolumeMoveError(err) {
			status.Message = fmt.Sprintf("Volume move staging failed: %s", err.Error())
			status.State = models.VolumeMoveStateFailed
			return stateOutcome{}
		}
		return stateOutcome{processErr: errors.ReconcileDeferredError("staging transient error; requeuing: %s", err)}
	}

	if initAccessInfo := moveInfo.InitialAccessInfo; initAccessInfo != nil {
		if err := c.sanitizeAccessInfo(ctx, initAccessInfo); err != nil {
			status.Message = fmt.Sprintf("Could not sanitize initial access info of secrets; %s", err.Error())
			status.State = models.VolumeMoveStateFailed
			return stateOutcome{}
		}
		status.InitialAccessInfo = initAccessInfo.DeepCopy()
	}
	if targAccessInfo := moveInfo.TargetAccessInfo; targAccessInfo != nil {
		if err := c.sanitizeAccessInfo(ctx, targAccessInfo); err != nil {
			status.Message = fmt.Sprintf("Could not sanitize target access info of secrets; %s", err.Error())
			status.State = models.VolumeMoveStateFailed
			return stateOutcome{}
		}
		status.TargetAccessInfo = targAccessInfo.DeepCopy()
	}

	if len(tvm.Status.Attachments) == 0 {
		status.Message = fmt.Sprintf("No volume attachments to modify for volume %q", tvm.Name)
		status.State = models.VolumeMoveStateMoving
		return stateOutcome{}
	}

	status.State = models.VolumeMoveStateNodeStaging
	return stateOutcome{}
}

func (c *TridentCrdController) handleStateNodeStaging(
	ctx context.Context, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	Logc(ctx).Debug("TVM is in node staging state. Waiting for nodes to update CR status...")

	allReady, err := c.attachmentsReady(ctx, models.VolumeMoveAttachmentStateBridged, status)
	if err != nil {
		status.Message = fmt.Sprintf("volume move node staging failed with error(s): %s", err.Error())
		status.State = models.VolumeMoveStateFailed
		return stateOutcome{}
	}
	if allReady {
		status.Message = fmt.Sprintf("All attachments are ready; moving to %q state", models.VolumeMoveStateMoving)
		status.State = models.VolumeMoveStateMoving
		return stateOutcome{}
	}

	return stateOutcome{processErr: errors.ReconcileDeferredError("waiting for node staging to complete; requeuing...")}
}

func (c *TridentCrdController) handleStateMoving(
	ctx context.Context, tvm *netappv1.TridentVolumeMove,
	moveInfo *models.VolumeMoveInfo, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	Logc(ctx).Debug("TVM is in moving state.")

	if !tvm.HasTridentFinalizers() {
		Logc(ctx).Debug("Finalizer not found; adding finalizer and requeuing...")
		return stateOutcome{}
	}

	err := c.startVolumeMove(ctx, moveInfo, false)
	status.BackendContext.Raw = moveInfo.BackendContext

	if err != nil {
		if isTerminalVolumeMoveError(err) {
			status.Message = fmt.Sprintf("Volume move failed: %s", err.Error())
			status.State = models.VolumeMoveStateFailed
			return stateOutcome{}
		}
		status.Message = "Volume move in progress"
		return stateOutcome{processErr: errors.ReconcileDeferredError("volume move transient error %q; requeuing...", err.Error())}
	}

	status.Message = "Volume move complete; unstaging controller"
	status.State = models.VolumeMoveStateControllerUnstaging
	return stateOutcome{}
}

func (c *TridentCrdController) handleStateControllerUnstaging(
	ctx context.Context, tvm *netappv1.TridentVolumeMove,
	moveInfo *models.VolumeMoveInfo, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	Logc(ctx).Debug("TVM is in controller unstaging state.")

	if err := c.controllerUnstaging(ctx, moveInfo); err != nil {
		status.Message = fmt.Sprintf("volume move unstaging failed; continuing with volume move: %s", err.Error())
	}

	if err := c.refreshAttachments(ctx, tvm); err != nil {
		if isTerminalVolumeMoveError(err) {
			status.Message = fmt.Sprintf("Volume move controller unstaging failed: %s", err.Error())
			status.State = models.VolumeMoveStateFailed
			return stateOutcome{}
		}
		status.Message = fmt.Sprintf("Could not update VolumeAttachments during volume move %s; requeuing...", err.Error())
		// Return a deferred error so the workqueue applies rate-limited backoff
		// instead of hot-looping on the status-update event.
		return stateOutcome{processErr: errors.ReconcileDeferredError("%s", status.Message)}
	}

	status.State = models.VolumeMoveStateNodeUnstaging
	return stateOutcome{}
}

func (c *TridentCrdController) handleStateNodeUnstaging(
	ctx context.Context, status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	Logc(ctx).Debug("TVM is in node unstaging state. Waiting for nodes to update CR status...")

	allReady, err := c.attachmentsReady(ctx, models.VolumeMoveAttachmentStateMigrated, status)
	if err != nil {
		status.Message = fmt.Sprintf("attachments may be stale; continuing with volume move: %s", status.AttachmentMessages())
		status.State = models.VolumeMoveStateSucceeded
		return stateOutcome{}
	}
	if allReady {
		status.Message = ""
		status.State = models.VolumeMoveStateSucceeded
		return stateOutcome{}
	}

	return stateOutcome{processErr: errors.ReconcileDeferredError("waiting for node unstaging to complete; requeuing...")}
}

func (c *TridentCrdController) handleStateFailed(
	status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	if status.CompletionTime == nil {
		status.CompletionTime = convert.ToPtr(metav1.Now())
		return stateOutcome{}
	}
	return stateOutcome{skipCommit: true}
}

func (c *TridentCrdController) handleStateSucceeded(
	ctx context.Context, tvm *netappv1.TridentVolumeMove,
	status *netappv1.TridentVolumeMoveStatus,
) stateOutcome {
	if status.CompletionTime == nil {
		status.CompletionTime = convert.ToPtr(metav1.Now())
		return stateOutcome{}
	}

	if tvm.Spec.DeleteAfterSuccess == nil {
		return stateOutcome{skipCommit: true}
	}

	shouldDelete, err := shouldDeleteTVM(tvm)
	if err != nil {
		return stateOutcome{skipCommit: true, processErr: err}
	}
	if shouldDelete {
		if err := c.deleteTridentVolumeMove(ctx, tvm.Namespace, tvm.Name); err != nil {
			status.Message = fmt.Sprintf("Failed to automatically delete TVM %q: %s; will retry...", tvm.Name, err.Error())
			// Return a deferred error so the workqueue applies rate-limited
			// backoff instead of hot-looping on the status-update event.
			return stateOutcome{processErr: errors.ReconcileDeferredError("%s", status.Message)}
		}
		return stateOutcome{skipCommit: true}
	}

	return stateOutcome{skipCommit: true}
}

// Orchestrator wrappers

func (c *TridentCrdController) startVolumeMove(
	ctx context.Context, moveInfo *models.VolumeMoveInfo, dryRun bool,
) error {
	moveInfo.DryRun = dryRun
	return c.orchestrator.MoveVolume(ctx, moveInfo)
}

func (c *TridentCrdController) controllerStaging(
	ctx context.Context, moveInfo *models.VolumeMoveInfo,
) error {
	return c.orchestrator.StageVolumeMove(ctx, moveInfo)
}

func (c *TridentCrdController) controllerUnstaging(
	ctx context.Context, moveInfo *models.VolumeMoveInfo,
) error {
	return c.orchestrator.UnstageVolumeMove(ctx, moveInfo)
}

func (c *TridentCrdController) reconcileVolumeMove(
	ctx context.Context, volumeName string, moveInfo *models.VolumeMoveInfo,
) error {
	return c.orchestrator.ReconcileVolumeMove(ctx, volumeName, moveInfo)
}

// Validation and helpers

func validateDeleteAfterSuccess(d *time.Duration) error {
	if d == nil {
		return nil
	}
	if *d < 0 {
		return errors.InvalidInputError(
			"deleteAfterSuccess must be zero or a positive duration (for example \"0s\" or \"10m\"); got %v",
			d,
		)
	}
	return nil
}

func shouldDeleteTVM(tvm *netappv1.TridentVolumeMove) (bool, error) {
	if tvm == nil || tvm.Status.State != models.VolumeMoveStateSucceeded {
		return false, nil
	}
	if tvm.Status.CompletionTime == nil || tvm.Spec.DeleteAfterSuccess == nil {
		return false, nil
	}
	deleteAfter := tvm.Spec.DeleteAfterSuccess.Duration
	if deleteAfter <= 0 {
		return true, nil
	}
	remaining := deleteAfter - time.Since(tvm.Status.CompletionTime.Time)
	if remaining <= 0 {
		return true, nil
	}
	return false, errors.ReconcileDeferredWithDuration(remaining, "waiting %v before deleting TVM", remaining)
}

func (c *TridentCrdController) validateVolumeMoveCR(
	ctx context.Context, namespace, name string,
) (*netappv1.TridentVolumeMove, error) {
	actionCR, err := c.crdClientset.TridentV1().TridentVolumeMoves(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("TVM in work queue no longer exists.")
		return nil, errors.NotFoundError(fmt.Sprintf("TVM %q in work queue no longer exists", name))
	}
	if err != nil {
		return nil, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}
	return actionCR, nil
}

func (c *TridentCrdController) deleteTridentVolumeMove(ctx context.Context, namespace, name string) error {
	fields := LogFields{
		"name":      name,
		"namespace": namespace,
	}
	Logc(ctx).WithFields(fields).Info("Deleting successful TridentVolumeMove CR.")

	err := c.crdClientset.TridentV1().TridentVolumeMoves(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		Logc(ctx).WithFields(fields).Debug("TVM already deleted.")
		return nil
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "failed to delete TVM")
	}
	return nil
}

// TVM CR update

func (c *TridentCrdController) updateTridentVolumeMove(
	ctx context.Context, namespace, name string, statusUpdate *netappv1.TridentVolumeMoveStatus,
) error {
	fields := LogFields{
		"name":      name,
		"namespace": namespace,
		"tvmStatus": statusUpdate,
	}

	tvm, err := c.crdClientset.TridentV1().TridentVolumeMoves(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).WithFields(fields).Debug("TVM in work queue no longer exists.")
		return err
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	// Write the status subresource FIRST so that critical fields (e.g. Context)
	// are persisted before the metadata Update triggers a watch/re-enqueue.
	//
	// Nodes concurrently update individual Attachments[i].{State, Message} via
	// their own UpdateStatus calls. The fresh API read above reflects those
	// changes; the handler's statusUpdate carries a stale snapshot that would
	// revert them and create a livelock (TVM stuck in NodeStaging/NodeUnstaging).
	//
	// Use the fresh attachments whenever they exist. The only time they are
	// empty is the very first write during ControllerStaging, where statusUpdate
	// is populating the attachment list for the first time.
	//
	// After restoring fresh attachments, apply any Detached states that the
	// controller set in reconcileAttachments. The controller owns the
	// Pending→Detached transition (K8s VolumeAttachment disappeared); nodes
	// own all other attachment state transitions.
	freshAttachments := tvm.Status.Attachments
	tvm.Status = *statusUpdate
	if len(freshAttachments) > 0 {
		tvm.Status.Attachments = freshAttachments
		applyControllerAttachmentOverrides(statusUpdate.Attachments, tvm.Status.Attachments)
	}

	tvm, err = c.crdClientset.TridentV1().TridentVolumeMoves(namespace).UpdateStatus(ctx, tvm, updateOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).WithFields(fields).Debug("TVM in work queue no longer exists.")
		return err
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	if statusUpdate.State == models.VolumeMoveStateFailed || statusUpdate.State == models.VolumeMoveStateSucceeded {
		if tvm.HasTridentFinalizers() {
			tvm.RemoveTridentFinalizers()
		}
	} else if statusUpdate.State == models.VolumeMoveStateMoving && !tvm.HasTridentFinalizers() {
		tvm.AddTridentFinalizers()
	}

	_, err = c.crdClientset.TridentV1().TridentVolumeMoves(namespace).Update(ctx, tvm, updateOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).WithFields(fields).Debug("TVM in work queue no longer exists.")
		return err
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	return nil
}

// Attachment helpers

// applyControllerAttachmentOverrides propagates controller-owned attachment
// state transitions into the fresh attachment slice. Today the only such
// transition is Detached (set by reconcileAttachments when a K8s
// VolumeAttachment disappears). All other attachment states are node-owned.
func applyControllerAttachmentOverrides(
	source []*netappv1.TridentVolumeMoveAttachmentStatus,
	target []*netappv1.TridentVolumeMoveAttachmentStatus,
) {
	idx := make(map[string]*netappv1.TridentVolumeMoveAttachmentStatus, len(source))
	for _, a := range source {
		if a != nil && a.State == models.VolumeMoveAttachmentStateDetached {
			idx[a.NodeName] = a
		}
	}

	for i, a := range target {
		if a == nil {
			continue
		}
		if a.State == models.VolumeMoveAttachmentStateCleaned ||
			a.State == models.VolumeMoveAttachmentStateFailed {
			continue
		}
		if override, ok := idx[a.NodeName]; ok {
			target[i] = override
		}
	}
}

// populateAttachments seeds the immutable TVM attachment set from the Kubernetes
// VolumeAttachments that exist for volumeName at the moment the move begins.
//
// Membership is anchored on VolumeAttachment.Spec (PersistentVolumeName +
// NodeName), which Kubernetes sets at creation and never mutates. We deliberately
// do NOT gate on Status.Attached (patched asynchronously by the external-attacher
// after the publish RPC returns) nor on the existence of a Trident
// VolumePublication (persisted only after the backend confirms the mapping). The
// VA is the earliest, most complete signal that a node has - or is about to have -
// a connection established using the source access info.
//
// Late VolumeAttachments created after this snapshot are intentionally ignored: a
// publish that observes the in-flight move self-heals to the target portals in the
// driver, so those nodes need no bridging.
func (c *TridentCrdController) populateAttachments(
	ctx context.Context, volumeName string, status *netappv1.TridentVolumeMoveStatus,
) error {
	// The TVM attachment membership is immutable once the TVM attachments are populated.
	// Never repopulate TVM attachments once they have been populated.
	if len(status.Attachments) != 0 {
		Logc(ctx).WithFields(LogFields{
			"volumeName":     volumeName,
			"tvmAttachments": status.Attachments,
		}).Trace("TVM attachments already populated; continuing with volume move.")
		return nil
	}

	// Bypass the indexer and read straight from Kubernetes here.
	vaList, err := c.kubeClientset.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("could not list volume attachments for %q: %w", volumeName, err)
	}

	nodes := make(map[string]struct{}, len(vaList.Items))
	for _, va := range vaList.Items {
		source := va.Spec.Source.PersistentVolumeName
		if source == nil || *source != volumeName {
			continue
		}
		if va.Spec.NodeName == "" {
			continue
		}
		nodes[va.Spec.NodeName] = struct{}{}
	}

	// Sort for deterministic attachment ordering.
	nodeNames := make([]string, 0, len(nodes))
	for nodeName := range nodes {
		nodeNames = append(nodeNames, nodeName)
	}
	sort.Strings(nodeNames)

	for _, nodeName := range nodeNames {
		Logc(ctx).WithField("node", nodeName).Debug("Seeding TVM attachment from K8s VolumeAttachment.")
		status.Attachments = append(status.Attachments, &netappv1.TridentVolumeMoveAttachmentStatus{
			State:    models.VolumeMoveAttachmentStatePending,
			NodeName: nodeName,
		})
	}

	return nil
}

// reconcileAttachments reconciles the TVM attachments with the Kubernetes VolumeAttachments.
//
// The TVM attachments follow these rules throughout the TVM lifecycle:
//  1. Immutable membership - once populated, TVM attachments are neither evicted nor added.
//  2. Mutable state - Live attachments in K8s can be destroyed during a TVM.
//     In that scenario, the TVM attachment should move to a Detached state.
func (c *TridentCrdController) reconcileAttachments(
	ctx context.Context, pvName string, status *netappv1.TridentVolumeMoveStatus,
) error {
	if pvName == "" {
		return errors.New("empty PV name")
	}
	if status == nil {
		return errors.New("TVM status is nil")
	}
	if len(status.Attachments) == 0 {
		return nil
	}

	vaList, err := c.indexers.VolumeAttachmentIndexer().GetCachedVolumeAttachmentsByVolume(ctx, pvName)
	if err != nil {
		return err
	}

	nodeAttachments := make(map[string]*v1.VolumeAttachment)
	for _, k8sAttachment := range vaList {
		if k8sAttachment == nil {
			continue
		}
		if k8sAttachment.Spec.NodeName == "" {
			continue
		}
		nodeAttachments[k8sAttachment.Spec.NodeName] = k8sAttachment.DeepCopy()
	}

	for i, tvmAttachment := range status.Attachments {
		if tvmAttachment == nil {
			continue
		}
		switch tvmAttachment.State {
		case models.VolumeMoveAttachmentStateFailed,
			models.VolumeMoveAttachmentStateDetached,
			models.VolumeMoveAttachmentStateCleaned:
			continue
		}

		if _, ok := nodeAttachments[tvmAttachment.NodeName]; !ok {
			detachedState := models.VolumeMoveAttachmentStateDetached
			Logc(ctx).WithFields(LogFields{
				"nodeName":      tvmAttachment.NodeName,
				"previousState": tvmAttachment.State,
				"newState":      detachedState,
			}).Debugf("Attachment in K8s no longer exists; moving TVM attachment to %q state.", detachedState)

			status.Attachments[i] = &netappv1.TridentVolumeMoveAttachmentStatus{
				State:    models.VolumeMoveAttachmentStateDetached,
				NodeName: tvmAttachment.NodeName,
				Message:  "Kubernetes VolumeAttachment was removed; issuing TVM attachment for cleanup",
			}
		}
	}

	return nil
}

// refreshAttachments heals Kubernetes VolumeAttachments by replacing the
// protocol-specific volume attachment metadata that changes during a
// volume move. This ensures an attachment that is reattached by Kubelet
// (without going through CSI ControllerPublishVolume) again will have
// the new models.VolumeAccessInfo.
func (c *TridentCrdController) refreshAttachments(
	ctx context.Context, tvm *netappv1.TridentVolumeMove,
) error {
	if tvm == nil {
		return errors.New("nil TVM")
	}
	if tvm.Name == "" {
		return errors.New("TVM name is empty")
	}
	if len(tvm.Status.Attachments) == 0 {
		return nil
	}
	if tvm.Status.TargetAccessInfo == nil {
		return errors.New("TVM Status.TargetAccessInfo is nil")
	}
	targetAccessInfo := tvm.Status.TargetAccessInfo.DeepCopy()

	tvmAttachMap := make(map[string]*netappv1.TridentVolumeMoveAttachmentStatus, len(tvm.Status.Attachments))
	for _, va := range tvm.Status.Attachments {
		if va == nil || va.NodeName == "" {
			continue
		}
		tvmAttachMap[va.NodeName] = va
	}

	k8sAttachments, err := c.indexers.VolumeAttachmentIndexer().GetCachedVolumeAttachmentsByVolume(ctx, tvm.Name)
	if err != nil {
		return err
	}
	if len(k8sAttachments) == 0 {
		return nil
	}

	k8sAttachMap := make(map[string]*v1.VolumeAttachment)
	for _, a := range k8sAttachments {
		k8sNodeName := a.Spec.NodeName
		tvmAttachment, ok := tvmAttachMap[k8sNodeName]
		if !ok {
			continue
		}
		if !a.Status.Attached {
			continue
		}
		if len(a.Status.AttachmentMetadata) == 0 {
			continue
		}

		switch tvmAttachment.State {
		case models.VolumeMoveAttachmentStateFailed,
			models.VolumeMoveAttachmentStateDetached,
			models.VolumeMoveAttachmentStateCleaned:
			continue
		}

		k8sAttachMap[k8sNodeName] = a
	}

	for _, k8sAttachment := range k8sAttachMap {
		publishInfo := make(map[string]string)
		for k, v := range k8sAttachment.Status.AttachmentMetadata {
			publishInfo[k] = v
		}

		protocol, ok := publishInfo["protocol"]
		if !ok {
			continue
		}
		targetAccessInfoCopy := targetAccessInfo.DeepCopy()

		switch tridentconfig.Protocol(protocol) {
		case tridentconfig.File:
			Logc(ctx).Debug("Volume protocol is not supported with volume move.")
			continue
		case tridentconfig.Block:
			sanType, ok := publishInfo["SANType"]
			if !ok {
				Logc(ctx).Debugf("Empty SAN type %q is not supported with volume move.", sanType)
				continue
			}

			switch sanType {
			case storageattribute.NVMe, storageattribute.FCP:
				Logc(ctx).Debugf("Volume protocol type %q is not supported with volume move.", sanType)
				continue
			case storageattribute.ISCSI:
				if err := csi.UpsertIscsiTargetPortals(ctx, publishInfo, targetAccessInfoCopy); err != nil {
					return err
				}
				csi.SetEncryptedCHAPPublishInfo(publishInfo, targetAccessInfoCopy)
			default:
				Logc(ctx).Debugf("Volume SAN type %q is not supported with volume move.", sanType)
				continue
			}
		default:
			Logc(ctx).Debugf("Volume protocol %q is not supported with volume move.", protocol)
			continue
		}

		k8sAttachment.Status.AttachmentMetadata = publishInfo
		k8sAttachment, err = c.kubeClientset.StorageV1().VolumeAttachments().UpdateStatus(ctx, k8sAttachment, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				Logc(ctx).WithError(err).Warn("VolumeAttachment could not be found; continuing...")
				continue
			}
			Logc(ctx).WithError(err).Error("VolumeAttachment could not be updated.")
			return err
		}
	}

	return nil
}

func (c *TridentCrdController) attachmentReady(
	_ context.Context, desiredState models.VolumeMoveAttachmentState, attachment *netappv1.TridentVolumeMoveAttachmentStatus,
) (bool, error) {
	if attachment == nil {
		return false, fmt.Errorf("attachment status is nil")
	}

	node, msg := attachment.NodeName, attachment.Message
	switch attachment.State {
	case desiredState, models.VolumeMoveAttachmentStateDetached, models.VolumeMoveAttachmentStateCleaned:
		return true, nil
	case models.VolumeMoveAttachmentStateFailed:
		return false, fmt.Errorf("attachment update on node %q failed; %s", node, msg)
	default:
		return false, nil
	}
}

// attachmentsReady checks whether all TVM attachments have reached the desired state.
func (c *TridentCrdController) attachmentsReady(
	ctx context.Context, desiredState models.VolumeMoveAttachmentState, status *netappv1.TridentVolumeMoveStatus,
) (bool, error) {
	if len(status.Attachments) == 0 {
		return true, nil
	}
	if desiredState == "" {
		return false, fmt.Errorf("desired attachment status is empty")
	}

	allReady := true
	var errs error
	for _, attachment := range status.Attachments {
		ready, err := c.attachmentReady(ctx, desiredState, attachment)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		if !ready {
			allReady = false
		}
	}
	return allReady, errs
}

// sanitizeAccessInfo encrypts any secrets within the supplied access info in-place.
func (c *TridentCrdController) sanitizeAccessInfo(
	ctx context.Context, accessInfo *models.VolumeAccessInfo,
) error {
	return csi.EncryptCHAPAccessInfo(ctx, accessInfo)
}

// isTerminalVolumeMoveError returns true for errors that cannot be resolved by retrying.
func isTerminalVolumeMoveError(err error) bool {
	return errors.IsInvalidInputError(err) ||
		errors.IsNotFoundError(err) ||
		errors.IsVolumeStateError(err) ||
		errors.IsAuthError(err) ||
		errors.IsTerminalReconciliationError(err)
}
