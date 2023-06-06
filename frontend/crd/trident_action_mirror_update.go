// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// handleActionMirrorUpdate handles the mirror update action and keeps the CR up to date
func (c *TridentCrdController) handleActionMirrorUpdate(keyItem *KeyItem) (updateError error) {
	Logx(keyItem.ctx).Debug(">>>> TridentCrdController#handleActionMirrorUpdate")
	defer Logx(keyItem.ctx).Debug("<<<< TridentCrdController#handleActionMirrorUpdate")

	key := keyItem.key
	ctx := keyItem.ctx

	// This action is a one time action per CRD and will ignore Update or Delete events
	if keyItem.event != EventAdd {
		return nil
	}

	// Convert the namespace/name string into a distinct namespace and name. If this fails,
	// no retry is likely to succeed, so return the error to forget this action. We can't determine the new CR,
	// so no CR update is possible.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key.")
		return err
	}

	// Ensure the CR is new and valid. This may return ReconcileDeferred if it should be retried.
	// Any other error returned here indicates a problem with the CR (i.e. it's not new or has been deleted),
	// so no CR update is needed.
	actionCR, err := c.validateActionMirrorUpdateCR(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Now that all pre-checks that don't require a CR update are done,
	// we can add a deferred function that updates the CR based on any additional errors encountered.
	defer func() {
		// If we want to retry, we won't update the CR
		if errors.IsReconcileDeferredError(updateError) {
			return
		}

		// Update CR with finalizer removal and success/failed status
		if err = c.updateActionMirrorUpdateCRComplete(ctx, namespace, name, updateError); err != nil {
			Logc(ctx).WithField("key", key).WithError(err).Error(
				"Could not update mirror update action CR with final result.")
		}
	}()

	// Detect a CR that is in progress but is not a retry from the workqueue.
	// This can only happen if Trident restarted while processing a CR, in which case we move the CR directly to Failed.
	if actionCR.InProgress() && !keyItem.isRetry {
		return fmt.Errorf("in-progress TAMU %s detected at startup, marking as Failed", actionCR.Name)
	}

	// Get TMR. All of the work of this method constitutes pre-checks and should be quick,
	// so we do this before setting the CR to in-progress. Any error other than ReconciledDeferred
	// will update the CR with a failed status and the operation will not be retried.
	tmr, updateError := c.getTMR(ctx, actionCR)
	if updateError != nil {
		return
	}

	if tmr.Status.Conditions[0].ReplicationPolicy == "Sync" {
		return fmt.Errorf("TAMU %s cannot be executed for TMR %s is a synchronous relationship", actionCR.Name,
			actionCR.Spec.TMRName)
	}
	// Check the state of the TMR to determine if it is ready to be updated or not.
	// A TMR in the process of establishing or reestablishing will be retried.
	// A TMR that is in any other state than established or reestablished will return an error and not be retried.
	localVolumeHandle := ""
	remoteVolumeHandle := ""
	localPVCName := ""
	if tmr.Status.Conditions[0].MirrorState == netappv1.MirrorStateEstablished ||
		tmr.Status.Conditions[0].MirrorState == netappv1.MirrorStateReestablished {
		localVolumeHandle = tmr.Status.Conditions[0].LocalVolumeHandle
		remoteVolumeHandle = tmr.Status.Conditions[0].RemoteVolumeHandle
		localPVCName = tmr.Status.Conditions[0].LocalPVCName
	} else {
		return fmt.Errorf("TAMU %s cannot be executed for TMR %s in %s state", actionCR.Name, actionCR.Spec.TMRName,
			tmr.Status.Conditions[0].MirrorState)
	}

	// Check if local PVC exists
	localPVC, err := c.kubeClientset.CoreV1().PersistentVolumeClaims(actionCR.Namespace).Get(
		ctx, localPVCName, metav1.GetOptions{},
	)

	// If CR already in progress, check the mirror transfer state and do not initiate another mirror update
	if actionCR.InProgress() {
		transferEndTime, err := c.orchestrator.CheckMirrorTransferState(ctx, localPVC.Spec.VolumeName)
		if errors.IsInProgressError(err) {
			return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
		}

		if err != nil {
			return err
		}

		// Confirm that the end transfer time is after the creation of the TAMU
		if transferEndTime != nil {
			if actionCR.Status.PreviousTransferTime == nil {
				return nil
			}
			if transferEndTime.After(actionCR.Status.PreviousTransferTime.Time) {
				return nil
			}
		}

		return errors.ReconcileDeferredError("mirror update retry")
	}

	// Get the last transfer time of the relationship
	previousTransferTime, err := c.orchestrator.GetMirrorTransferTime(ctx, localPVC.Spec.VolumeName)
	if err != nil {
		return fmt.Errorf("could not get previous mirror transfer time; %v", err)
	}

	// Parse snapshot name from the handle
	snapshotName := ""
	if actionCR.Spec.SnapshotHandle != "" {
		_, snapshotName, err = storage.ParseSnapshotID(actionCR.Spec.SnapshotHandle)
		if err != nil {
			return fmt.Errorf("invalid snapshot handle %s", actionCR.Spec.SnapshotHandle)
		}
	}

	// Update CR with finalizers and in-progress status
	updateError = c.updateActionMirrorUpdateCRInProgress(ctx, namespace, name, localVolumeHandle,
		remoteVolumeHandle, actionCR.Spec.SnapshotHandle, previousTransferTime)
	if updateError != nil {
		return updateError
	}

	// Invoke mirror update
	updateError = c.orchestrator.UpdateMirror(ctx, localPVC.Spec.VolumeName, snapshotName)
	if errors.IsInProgressError(updateError) {
		updateError = errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}
	return
}

// validateActionMirrorUpdateCR returns an error if the TAMU CR is invalid, is not new,
// or is not in the in progress state.
func (c *TridentCrdController) validateActionMirrorUpdateCR(ctx context.Context, namespace,
	name string,
) (*netappv1.TridentActionMirrorUpdate, error) {
	// Get the resource with the namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentActionMirrorUpdates(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Mirror update action in work queue no longer exists.")
		return nil, err
	}
	if err != nil {
		return nil, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	if !actionCR.IsNew() && !actionCR.InProgress() {
		return nil, fmt.Errorf("mirror update action %s/%s is not new or in progress", namespace, name)
	}

	return actionCR, nil
}

// updateActionMirrorUpdateCRInProgress updates the action CR to an in progress state and adds finalizers
func (c *TridentCrdController) updateActionMirrorUpdateCRInProgress(ctx context.Context, namespace,
	name, localVolumeHandle, remoteVolumeHandle, snapshotHandle string, previousTransferTime *time.Time,
) error {
	// Get the resource with the namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentActionMirrorUpdates(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Mirror update action in work queue no longer exists.")
		return err
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	if !actionCR.HasTridentFinalizers() {
		actionCR.AddTridentFinalizers()
	}
	actionCR.Status.State = netappv1.TridentActionStateInProgress
	actionCR.Status.Message = ""
	actionCR.Status.LocalVolumeHandle = localVolumeHandle
	actionCR.Status.RemoteVolumeHandle = remoteVolumeHandle

	if previousTransferTime != nil {
		transferTime := metav1.NewTime(*previousTransferTime)
		actionCR.Status.PreviousTransferTime = &transferTime
	}

	if snapshotHandle != "" {
		actionCR.Status.SnapshotHandle = snapshotHandle
	}

	_, err = c.crdClientset.TridentV1().TridentActionMirrorUpdates(namespace).Update(ctx, actionCR, updateOpts)
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	return nil
}

// updateActionMirrorUpdateCRComplete updates the action CR to either succeeded or failed and removes finalizers
func (c *TridentCrdController) updateActionMirrorUpdateCRComplete(ctx context.Context, namespace, name string,
	updateError error,
) error {
	// Get the resource with the namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentActionMirrorUpdates(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Mirror update action in work queue no longer exists.")
		return nil
	}
	if err != nil {
		return err
	}

	if actionCR.HasTridentFinalizers() {
		actionCR.RemoveTridentFinalizers()
	}

	if updateError == nil {
		actionCR.Status.State = netappv1.TridentActionStateSucceeded
		actionCR.Status.Message = ""
	} else {
		actionCR.Status.State = netappv1.TridentActionStateFailed
		actionCR.Status.Message = updateError.Error()
	}

	completionTime := metav1.Now()
	actionCR.Status.CompletionTime = &completionTime

	_, err = c.crdClientset.TridentV1().TridentActionMirrorUpdates(namespace).Update(ctx, actionCR, updateOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Mirror update in work queue no longer exists.")
		return nil
	}
	return err
}

// getTMR returns the TMR referenced in the action CR
func (c *TridentCrdController) getTMR(
	ctx context.Context, tridentActionMirrorUpdate *netappv1.TridentActionMirrorUpdate,
) (*netappv1.TridentMirrorRelationship, error) {
	k8sTMR, err := c.crdClientset.TridentV1().TridentMirrorRelationships(tridentActionMirrorUpdate.Namespace).Get(
		ctx, tridentActionMirrorUpdate.Spec.TMRName, metav1.GetOptions{})
	if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
		message := fmt.Sprintf(
			"TridentMirrorRelationship '%v' for TridentActionMirrorUpdate '%v' does not yet exist.",
			tridentActionMirrorUpdate.Spec.TMRName, tridentActionMirrorUpdate.Name,
		)
		Logx(ctx).Debug(message)
		// If TMR does not yet exist, do not update the TridentActionMirrorUpdate, no need to retry
		return nil, errors.ReconcileFailedError(message)
	} else if err != nil {
		return nil, errors.WrapWithReconcileFailedError(err, "reconcile failed")
	}
	if k8sTMR == nil {
		return nil, errors.ReconcileDeferredError("could not get TridentMirrorRelationship %s",
			tridentActionMirrorUpdate.Spec.TMRName)
	}
	return k8sTMR, nil
}
