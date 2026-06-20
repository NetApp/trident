// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"
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

const (
	errMsgUpdateMoveState = "Could not record volume move info while TVM is in %q state: %s; requeuing.."
)

// handleVolumeMove handles a TridentVolumeMove CR event.
func (c TridentCrdController) handleVolumeMove(keyItem *KeyItem) (volMoveError error) {
	key := keyItem.key
	ctx := keyItem.ctx

	Logc(ctx).Debug(">>>> TridentCrdController#handleVolumeMove")
	defer Logc(ctx).Debug("<<<< TridentCrdController#handleVolumeMove")

	// Ensure the volume attachment indexer cache is synced
	if ok := c.indexers.VolumeAttachmentIndexer().WaitForCacheSync(ctx); !ok {
		Logc(ctx).Warn("Failed to sync volume attachment cache. Not all volume attachments may be found.")
		return errors.ReconcileDeferredError("failed to sync volume attachment cache; requeuing...")
	}

	// Convert the namespace/name string into a distinct namespace and name.
	// If this fails, no retry is likely to succeed, so return the error to forget this action.
	// We can't determine the CR, so no CR update is possible.
	// The namespace should be in the Trident namespace because it is an admin action.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key.")
		return err
	}

	// Ensure the CR is new and valid. This may return ReconcileDeferred if it should be retried.
	// Any other error returned here indicates a problem with the CR (i.e. it's not new or has been
	// deleted), so no CR update is needed.
	tvm, err := c.validateVolumeMoveCR(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			_ = c.reconcileVolumeMove(ctx, name, nil)
			return nil
		}
		return err
	}
	updateStatus := &tvm.Status

	// Reconcile the TVM CR attachments with the Kubernetes VolumeAttachments.
	// If Kubernetes VolumeAttachments are not found or the attachment is !attached,
	// TVM controllers will treat the attachment as detached and attempt to
	// detach the volume from the node.
	if err := c.reconcileAttachments(ctx, tvm.Name, updateStatus); err != nil {
		Logc(ctx).Errorf("Failed to reconcile attachments: %v", err)
		return err
	}

	// The move info is rebuilt on every handling.
	// As a result, the TVM attachments must always be reconciled.
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

	switch updateStatus.State {
	case models.VolumeMoveStatePending:
		if volMoveError := validateDeleteAfterSuccess(moveInfo.DeleteAfterSuccess); volMoveError != nil {
			updateStatus.Message = volMoveError.Error()
			updateStatus.State = models.VolumeMoveStateFailed
			break
		}

		Logc(ctx).WithField("key", key).Debug("TVM is pending, starting dry-run.")
		volMoveError = c.startVolumeMove(ctx, moveInfo, true)
		if volMoveError != nil {
			if isTerminalVolumeMoveError(volMoveError) {
				updateStatus.Message = fmt.Sprintf("volume move dry run failed with error: %s", volMoveError.Error())
				updateStatus.State = models.VolumeMoveStateFailed
				break
			}
			return errors.ReconcileDeferredError("dry-run transient error; requeuing: %s", volMoveError)
		}

		startTime := metav1.Now()
		updateStatus.StartTime = &startTime
		updateStatus.State = models.VolumeMoveStateControllerStaging

	case models.VolumeMoveStateControllerStaging:
		Logc(ctx).WithField("key", key).Debug("TVM is in controller staging state.")

		volMoveError = c.controllerStaging(ctx, moveInfo)
		if volMoveError != nil {
			if isTerminalVolumeMoveError(volMoveError) {
				updateStatus.Message = fmt.Sprintf("Volume move staging failed: %s", volMoveError.Error())
				updateStatus.State = models.VolumeMoveStateFailed
				break
			}
			return errors.ReconcileDeferredError("staging transient error; requeuing: %s", volMoveError)
		}

		if initAccessInfo := moveInfo.InitialAccessInfo; initAccessInfo != nil {
			volMoveError = c.sanitizeAccessInfo(ctx, initAccessInfo)
			if volMoveError != nil {
				updateStatus.Message = fmt.Sprintf("Could not sanitize initial access info of secrets; %s", volMoveError.Error())
				updateStatus.State = models.VolumeMoveStateFailed
				break
			}
			updateStatus.InitialAccessInfo = initAccessInfo.DeepCopy()
		}
		if targAccessInfo := moveInfo.TargetAccessInfo; targAccessInfo != nil {
			volMoveError = c.sanitizeAccessInfo(ctx, targAccessInfo)
			if volMoveError != nil {
				updateStatus.Message = fmt.Sprintf("Could not sanitize target access info of secrets; %s", volMoveError.Error())
				updateStatus.State = models.VolumeMoveStateFailed
				break
			}
			updateStatus.TargetAccessInfo = targAccessInfo.DeepCopy()
		}

		// This move is staged.
		// Gather the known attachments from K8s and populate the TVM attachments.
		tvmAttachments, err := c.populateAttachments(ctx, tvm.Name)
		if err != nil {
			updateStatus.Message = fmt.Sprintf("Failed to read VolumeAttachments; error: %s ; requeuing...", err.Error())
			volMoveError = err
			break
		}
		updateStatus.Attachments = tvmAttachments

		// No attachments = nothing to do, skip to moving.
		if len(tvm.Status.Attachments) == 0 {
			updateStatus.Message = fmt.Sprintf("No volumes attachments to modify for volume %q", tvm.Name)
			updateStatus.State = models.VolumeMoveStateMoving
			break
		}

		updateStatus.State = models.VolumeMoveStateNodeStaging

	case models.VolumeMoveStateNodeStaging:
		Logc(ctx).WithField("key", key).Debug("TVM is in node staging state.")
		Logc(ctx).Debug("Waiting for nodes to update CR status...")

		// Keep the volume move info in step with the TVM.
		volMoveError = c.reconcileVolumeMove(ctx, moveInfo.VolumeName, moveInfo)
		if volMoveError != nil {
			updateStatus.Message = fmt.Sprintf(errMsgUpdateMoveState, models.VolumeMoveStateNodeStaging, volMoveError.Error())
			break
		}

		allAttachmentsReady, err := c.attachmentsReady(ctx, models.VolumeMoveAttachmentStateBridged, updateStatus)
		if err != nil {
			updateStatus.Message = fmt.Sprintf("volume move node staging failed with error(s): %s", err.Error())
			updateStatus.State = models.VolumeMoveStateFailed
			break
		}
		if allAttachmentsReady {
			updateStatus.Message = fmt.Sprintf("All attachments are ready; moving to %q state", models.VolumeMoveStateMoving)
			updateStatus.State = models.VolumeMoveStateMoving
			break
		}

		// In this case, we need to wait for the node-side CRD controllers to reconcile and
		// update the TVM attachments.
		return errors.ReconcileDeferredError("waiting for node staging to complete; requeuing...")

	case models.VolumeMoveStateMoving:
		Logc(ctx).WithField("key", key).Debug("TVM is in moving state.")
		if !tvm.HasTridentFinalizers() {
			// Adding the finalizer, handled in the updateTridentVolumeMove function when state is moving.
			Logc(ctx).Debug("Finalizer not found; adding finalizer and requeuing...")
			break
		}
		volMoveError = c.startVolumeMove(ctx, moveInfo, false)
		updateStatus.BackendContext.Raw = moveInfo.BackendContext
		if volMoveError != nil {
			if isTerminalVolumeMoveError(volMoveError) {
				updateStatus.Message = fmt.Sprintf("Volume move failed: %s", volMoveError.Error())
				updateStatus.State = models.VolumeMoveStateFailed
				break
			}

			// Persist BackendContext progress before requeuing; the bottom-of-function
			// update is only reached via `break`, and we need to flush in-flight job
			// state so it shows up in the TVM YAML between polls.
			updateStatus.Message = "Volume move in progress"
			if updateErr := c.updateTridentVolumeMove(ctx, namespace, name, updateStatus); updateErr != nil {
				return updateErr
			}
			return errors.ReconcileDeferredError("volume move transient error %q; requeuing...", volMoveError.Error())
		}
		updateStatus.Message = "Volume move complete; unstaging controller"
		updateStatus.State = models.VolumeMoveStateControllerUnstaging

	case models.VolumeMoveStateControllerUnstaging:
		Logc(ctx).WithField("key", key).Debug("TVM is in controller unstaging state.")
		volMoveError = c.controllerUnstaging(ctx, moveInfo)
		if volMoveError != nil {
			updateStatus.Message = fmt.Sprintf("volume move unstaging failed; continuing with volume move: %s", volMoveError.Error())
		}

		// Refresh the Kubernetes attachments before proceeding.
		// This only matters for cases where there were real attachments in the TVM.
		volMoveError = c.refreshAttachments(ctx, tvm)
		if volMoveError != nil {
			if isTerminalVolumeMoveError(volMoveError) {
				updateStatus.Message = fmt.Sprintf("Volume move controller unstaging failed: %s", volMoveError.Error())
				updateStatus.State = models.VolumeMoveStateFailed
				break
			}
			updateStatus.Message = fmt.Sprintf("Could not update VolumeAttachments during volume move %s; requeuing...", volMoveError.Error())
			break
		}

		updateStatus.State = models.VolumeMoveStateNodeUnstaging

	case models.VolumeMoveStateNodeUnstaging:
		Logc(ctx).WithField("key", key).Debug("TVM is in node unstaging state.")
		Logc(ctx).Debug("Waiting for nodes to update CR status...")

		// Keep the volume move info in step with the TVM.
		volMoveError = c.reconcileVolumeMove(ctx, moveInfo.VolumeName, moveInfo)
		if volMoveError != nil {
			updateStatus.Message = fmt.Sprintf(errMsgUpdateMoveState, models.VolumeMoveStateNodeUnstaging, volMoveError.Error())
			break
		}

		allAttachmentsReady, err := c.attachmentsReady(ctx, models.VolumeMoveAttachmentStateMigrated, updateStatus)
		if err != nil {
			// Move to VolumeMoveStateFailed; the volume is moved and the worst case is that there are
			// stale paths on some host(s).
			updateStatus.Message = fmt.Sprintf("attachments may be stale; continuing with volume move: %s", updateStatus.AttachmentMessages())
			updateStatus.State = models.VolumeMoveStateSucceeded
			break
		} else if allAttachmentsReady {
			updateStatus.Message = ""
			updateStatus.State = models.VolumeMoveStateSucceeded
			break
		}

		// Wait for the node-side CRD controllers. Until the attachments are in the expected
		// state, we cannot continue. Requeue to try again.
		return errors.ReconcileDeferredError("waiting for node unstaging to complete; requeuing...")

	case models.VolumeMoveStateFailed:
		// Always set a completion time.
		// This costs 1 requeue with the current design, which is fine.
		if updateStatus.CompletionTime == nil {
			updateStatus.CompletionTime = convert.ToPtr(metav1.Now())
			break
		}

		// Do not update or requeue for terminal states.
		return nil

	case models.VolumeMoveStateSucceeded:
		if updateStatus.CompletionTime == nil {
			// Always update the completion
			updateStatus.CompletionTime = convert.ToPtr(metav1.Now())
			break
		}

		if tvm.Spec.DeleteAfterSuccess == nil {
			// DeleteAfterSuccess is not set. We are done. Return nil.
			return nil
		}

		// Check if we should delete the TVM, if it is not ready to delete,
		// shouldDeleteTVM will return a ReconcileDeferredWithDuration error.
		shouldDelete, err := shouldDeleteTVM(tvm)
		if err != nil {
			return err
		}
		if shouldDelete {
			volMoveError = c.deleteTridentVolumeMove(ctx, namespace, name)
			if volMoveError == nil {
				return nil
			}
			// Could not delete a successfully completed TVM. Requeue.
			updateStatus.Message = fmt.Sprintf(
				"Failed to automatically delete TVM %q: %s; will retry...",
				tvm.Name, volMoveError.Error(),
			)
			break
		}

	default:
		updateStatus.State = models.VolumeMoveStatePending
	}

	// As a safety net, we should always clear the move info on the volume before deleting the TVM.
	// Two situations where always clearing makes sense:
	//	1. TVM is in a terminal state - TVM is failed or succeeded.
	//	2. TVM is deleting - TVM was deleted by something (CRD controller or admin).
	if updateStatus.State.IsTerminal() {
		if err := c.reconcileVolumeMove(ctx, name, nil); err != nil {
			// Retry until we succeed.
			return errors.WrapWithReconcileDeferredError(err, "failed to clear volume move info")
		}
	}

	// Update CR with finalizers and new status
	volMoveError = c.updateTridentVolumeMove(ctx, namespace, name, updateStatus)
	if volMoveError != nil {
		return volMoveError
	}

	return volMoveError
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
	return false, errors.ReconcileDeferredWithDuration(remaining,
		"waiting %v before deleting successful TVM", remaining)
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

func (c *TridentCrdController) startVolumeMove(
	ctx context.Context, moveInfo *models.VolumeMoveInfo, dryRun bool,
) (volMoveError error) {
	moveInfo.DryRun = dryRun
	volMoveError = c.orchestrator.MoveVolume(ctx, moveInfo)
	return
}

func (c *TridentCrdController) controllerStaging(
	ctx context.Context, moveInfo *models.VolumeMoveInfo,
) (volMoveError error) {
	volMoveError = c.orchestrator.StageVolumeMove(ctx, moveInfo)
	return
}

func (c *TridentCrdController) controllerUnstaging(
	ctx context.Context, moveInfo *models.VolumeMoveInfo,
) (volMoveError error) {
	volMoveError = c.orchestrator.UnstageVolumeMove(ctx, moveInfo)
	return
}

func (c *TridentCrdController) reconcileVolumeMove(
	ctx context.Context, volumeName string, moveInfo *models.VolumeMoveInfo,
) (volMoveError error) {
	volMoveError = c.orchestrator.ReconcileVolumeMove(ctx, volumeName, moveInfo)
	return
}

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

func (c *TridentCrdController) validateVolumeMoveCR(
	ctx context.Context, namespace, name string,
) (*netappv1.TridentVolumeMove, error) {
	// Get the resource with this namespace/name
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

	// Update metadata (finalizers) using the ResourceVersion from the status write.
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
		// We only override the TVM attachment if the volume attachment
		// was ripped away in Kubernetes.
		if a != nil && a.State == models.VolumeMoveAttachmentStateDetached {
			idx[a.NodeName] = a
		}
	}

	for i, a := range target {
		if a == nil {
			continue
		}
		// Don't revert node-set terminal states. If the node already
		// advanced past Detached (e.g. to Cleaned or Failed), the
		// controller's stale Detached must not overwrite it.
		if a.State == models.VolumeMoveAttachmentStateCleaned ||
			a.State == models.VolumeMoveAttachmentStateFailed {
			continue
		}
		if override, ok := idx[a.NodeName]; ok {
			target[i] = override
		}
	}
}

// populateAttachments populates the attachments on the TVM in the controller staging state.
// This should always happen exactly once per TVM and should always read directly from the
// Kubernetes API to gain an accurate picture of what VolumeAttachments exist during a TVM.
//
// Rules for populating attachments:
//  1. Attachments must have the attached=true state (CSI ControllerPublishVolume must have happened).
//  2. Attachments must not be duplicated
//
// The TVM attachments follow these rules throughout the TVM lifecycle:
//  1. Immutable membership - once populated, TVM attachments are neither evicted nor added.
//  2. Mutable state - Live attachments in K8s can be destroyed during a TVM.
//     In that scenario, the TVM attachment should move to a Detached state.
func (c *TridentCrdController) populateAttachments(
	ctx context.Context, pvName string,
) ([]*netappv1.TridentVolumeMoveAttachmentStatus, error) {
	if pvName == "" {
		return nil, errors.New("empty volume name")
	}

	vaList, err := c.kubeClientset.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if vaList == nil {
		return nil, nil
	}

	tvmAttachments := make([]*netappv1.TridentVolumeMoveAttachmentStatus, 0)
	for _, va := range vaList.Items {
		if !va.Status.Attached {
			continue
		}
		if va.Spec.Source.PersistentVolumeName == nil || *va.Spec.Source.PersistentVolumeName != pvName {
			continue
		}

		tvmAttachments = append(tvmAttachments, &netappv1.TridentVolumeMoveAttachmentStatus{
			State:    models.VolumeMoveAttachmentStatePending,
			NodeName: va.Spec.NodeName,
			Message:  "",
		})
	}
	return tvmAttachments, nil
}

// reconcileAttachments reconciles the TVM attachments with the Kubernetes VolumeAttachments.
//
// The TVM attachments follow these rules throughout the TVM lifecycle:
//  1. Immutable membership - once populated, TVM attachments are neither evicted nor added.
//  2. Mutable state - Live attachments in K8s can be destroyed during a TVM.
//     In that scenario, the TVM attachment should move to a Detached state.
//
// As a result, this function modifies the TVM attachments in-place
// and intentionally does not return a modified set of attachments.
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

	// Here we can rely on the indexer as the reconcile
	// does not remove TVM attachments.
	vaList, err := c.indexers.VolumeAttachmentIndexer().GetCachedVolumeAttachmentsByVolume(ctx, pvName)
	if err != nil {
		return err
	}

	// Build a filtered index of K8s attachments by node.
	nodeAttachments := make(map[string]*v1.VolumeAttachment)
	for _, k8sAttachment := range vaList {
		if k8sAttachment == nil {
			continue
		}
		if k8sAttachment.Spec.NodeName == "" {
			continue
		}
		if !k8sAttachment.Status.Attached {
			continue
		}
		nodeAttachments[k8sAttachment.Spec.NodeName] = k8sAttachment.DeepCopy()
	}

	// Reconcile the TVM attachments with the Kubernetes attachments.
	// TVM is the immutable set, the Kubernetes attachments help us
	// decide if we should move to a detached state or not.
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

		// TVM attachment wasn't found in K8s.
		// That means it was deleted. Set it to Detached.
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
//
// Without this, Kubelet could reattach a volume to the same node with
// stale attachment metadata / publish context and fail forever.
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
		// No known attachments are OK; continue in the handler.
		return nil
	}
	if tvm.Status.TargetAccessInfo == nil {
		return errors.New("TVM Status.TargetAccessInfo is nil")
	}
	targetAccessInfo := tvm.Status.TargetAccessInfo.DeepCopy()

	// Build an index of TVM attachments by node name.
	tvmAttachments := tvm.Status.Attachments
	tvmAttachMap := make(map[string]*netappv1.TridentVolumeMoveAttachmentStatus, len(tvmAttachments))
	for _, va := range tvmAttachments {
		tvmAttachMap[va.NodeName] = va
	}

	// Build an index of K8s attachments by node name.
	// Here we can rely on the indexer as the reconcile
	// does not remove TVM attachments.
	k8sAttachments, err := c.indexers.VolumeAttachmentIndexer().GetCachedVolumeAttachmentsByVolume(ctx, tvm.Name)
	if err != nil {
		return err
	}
	if len(k8sAttachments) == 0 {
		// No attachments. Return early.
		return nil
	}

	// Reorganize the attachments into an index of nodeName -> attachment.
	// Filter the attachments by the following conditions:
	//	1. The node of the attachment is not known to the TVM.
	//	2. The attachment has "attached" set to false (CSI ControllerPublishVolume has not happened).
	//  3. The attachment has no attachment metadata.
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
		// Construct a new publish info from the attachment metadata.
		publishInfo := make(map[string]string)
		for k, v := range k8sAttachment.Status.AttachmentMetadata {
			publishInfo[k] = v
		}

		protocol, ok := publishInfo["protocol"]
		if !ok {
			continue
		}
		targetAccessInfoCopy := targetAccessInfo.DeepCopy()

		// Update only the relevant publish info for each protocol on each volume attachment.
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
				// Unset the existing portals on the publish info metadata in the K8s attachment.
				if err := csi.UpsertIscsiTargetPortals(ctx, publishInfo, targetAccessInfoCopy); err != nil {
					return err
				}
				// The CHAP credentials should already be encrypted in the TVM attachments at this point.
				// Replace them in the publish context.
				csi.SetEncryptedCHAPPublishInfo(publishInfo, targetAccessInfoCopy)
			default:
				Logc(ctx).Debugf("Volume SAN type %q is not supported with volume move.", sanType)
				continue
			}
		default:
			Logc(ctx).Debugf("Volume protocol %q is not supported with volume move.", protocol)
			continue
		}

		// Heal the volume attachment by updating the VolumeAttachment status with the new publish info.
		k8sAttachment.Status.AttachmentMetadata = publishInfo
		k8sAttachment, err = c.kubeClientset.StorageV1().VolumeAttachments().UpdateStatus(ctx, k8sAttachment, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				Logc(ctx).WithError(err).Warn("VolumeAttachment could not be found; continuing...")
				continue
			}
			// Volume attachment update failed for other reasons. Valid failure case.
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
		// Failed states are reserved for terminal failures.
		// At this point, the attachment can never move to a ready state.
		// Moving forward with the vol move will not be safe, so fail.
		return false, fmt.Errorf("attachment update on node %q failed; %s", node, msg)
	default:
		// This could be the case where an attachment is in a previous state
		// still transitioning to the next one.
		return false, nil
	}
}

// attachmentsReady looks through all attachments on the TVM status and checks
// if all attachments are at the desired state.
// If any attachment has terminal errors, it returns an error.
func (c *TridentCrdController) attachmentsReady(
	ctx context.Context, desiredState models.VolumeMoveAttachmentState, status *netappv1.TridentVolumeMoveStatus,
) (bool, error) {
	// No attachments; good to continue moving.
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

// sanitizeAccessInfo attempts to encrypt, redact, and ultimately sanitize any
// secrets that may be within the supplied access info in-place.
//
// If any secrets fail sanitization, this should stop the entire volume move.
func (c *TridentCrdController) sanitizeAccessInfo(
	ctx context.Context, accessInfo *models.VolumeAccessInfo,
) error {
	return csi.EncryptCHAPAccessInfo(ctx, accessInfo)
}

// isTerminalVolumeMoveError returns true for errors that cannot be resolved by retrying.
// The TVM should transition to Failed immediately for these. Any error not matched here
// is treated as transient and the reconcile is requeued — mirroring the node side's
// isTerminalError logic in TridentNodeCrdController.
func isTerminalVolumeMoveError(err error) bool {
	return errors.IsInvalidInputError(err) ||
		errors.IsNotFoundError(err) ||
		errors.IsVolumeStateError(err) ||
		errors.IsAuthError(err) ||
		errors.IsTerminalReconciliationError(err)
}
