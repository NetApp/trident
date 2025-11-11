// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/acp"
	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func (c *TridentCrdController) handleActionSnapshotRestore(keyItem *KeyItem) (restoreError error) {
	Logc(keyItem.ctx).Debug(">>>> TridentCrdController#runActionSnapshotRestore")
	defer Logc(keyItem.ctx).Debug("<<<< TridentCrdController#runActionSnapshotRestore")

	key := keyItem.key
	ctx := keyItem.ctx

	// This one-shot action runs on Add and does not need Update or Delete
	if keyItem.event != EventAdd {
		return nil
	}

	// Convert the namespace/name string into a distinct namespace and name.  If this fails, no
	// retry is likely to succeed, so return the error to forget this action.  We can't determine
	// the CR, so no CR update is possible.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key.")
		return err
	}

	// Ensure the CR is new and valid.  This may return ReconcileDeferred if it should be retried.
	// Any other error returned here indicates a problem with the CR (i.e. it's not new or has been
	// deleted), so no CR update is needed.
	actionCR, err := c.validateActionSnapshotRestoreCR(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Now that all prechecks that don't require a CR update are done, we can add a deferred
	// function that updates the CR based on any additional errors encountered.
	defer func() {
		// If we want to be retried, we won't update the CR.
		if errors.IsReconcileDeferredError(restoreError) {
			return
		}

		// Update CR with finalizer removal and success/failed status
		if err = c.updateActionSnapshotRestoreCRComplete(ctx, namespace, name, restoreError); err != nil {
			Logc(ctx).WithField("key", key).WithError(err).Error(
				"Could not update snapshot restore action CR with final result.")
		}
	}()

	// Check if this feature is supported by reaching out to trident-acp via API.
	if err = acp.API().IsFeatureEnabled(ctx, acp.FeatureSnapshotRestore); err != nil {
		Logc(ctx).WithField("key", key).WithError(err).Error("Could not complete snapshot restore.")
		return err
	}

	// Detect a CR that is in progress but is not a retry from the workqueue.  This can only happen
	// if Trident restarted while processing a CR, in which case we move the CR directly to Failed.
	if actionCR.InProgress() && !keyItem.isRetry {
		return fmt.Errorf("in-progress TridentActionSnapshotRestore %s detected at startup, marking as Failed",
			keyItem.key)
	}

	// Get PV and VSC corresponding to PVC and VS.  All of the work of this method constitutes pre-checks
	// and should be quick, so we do this before setting the CR to in-progress.  Any error other than
	// ReconcileDeferred (not applicable here) will update the CR with a failed status and the operation
	// will not be retried.
	tridentVolume, tridentSnapshot, restoreError := c.getKubernetesObjectsForActionSnapshotRestore(ctx, actionCR)
	if restoreError != nil {
		return restoreError
	}

	// Update CR with finalizers and in-progress status
	restoreError = c.updateActionSnapshotRestoreCRInProgress(ctx, namespace, name)
	if restoreError != nil {
		return restoreError
	}

	// Invoke snapshot restore
	restoreError = c.orchestrator.RestoreSnapshot(ctx, tridentVolume, tridentSnapshot)
	if errors.IsInProgressError(restoreError) {
		restoreError = errors.WrapWithReconcileDeferredError(restoreError, "reconcile deferred")
	}

	return restoreError
}

func (c *TridentCrdController) validateActionSnapshotRestoreCR(
	ctx context.Context, namespace, name string,
) (*netappv1.TridentActionSnapshotRestore, error) {
	// Get the resource with this namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentActionSnapshotRestores(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Snapshot restore action in work queue no longer exists.")
		return nil, err
	}
	if err != nil {
		return nil, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	if !actionCR.IsNew() && !actionCR.InProgress() {
		return nil, fmt.Errorf("snapshot restore action %s/%s is not new or in progress", namespace, name)
	}

	return actionCR, nil
}

func (c *TridentCrdController) updateActionSnapshotRestoreCRInProgress(
	ctx context.Context, namespace, name string,
) error {
	// Get the resource with this namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentActionSnapshotRestores(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Snapshot restore action in work queue no longer exists.")
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
	startTime := metav1.Now()
	actionCR.Status.StartTime = &startTime

	_, err = c.crdClientset.TridentV1().TridentActionSnapshotRestores(namespace).Update(ctx, actionCR, updateOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Snapshot restore action in work queue no longer exists.")
		return err
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	return nil
}

func (c *TridentCrdController) updateActionSnapshotRestoreCRComplete(
	ctx context.Context, namespace, name string, restoreError error,
) error {
	// Get the resource with this namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentActionSnapshotRestores(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Snapshot restore action in work queue no longer exists.")
		return nil
	}
	if err != nil {
		return err
	}

	if actionCR.HasTridentFinalizers() {
		actionCR.RemoveTridentFinalizers()
	}

	if restoreError == nil {
		actionCR.Status.State = netappv1.TridentActionStateSucceeded
		actionCR.Status.Message = ""
	} else {
		actionCR.Status.State = netappv1.TridentActionStateFailed
		actionCR.Status.Message = restoreError.Error()
	}

	completionTime := metav1.Now()
	actionCR.Status.CompletionTime = &completionTime

	_, err = c.crdClientset.TridentV1().TridentActionSnapshotRestores(namespace).Update(ctx, actionCR, updateOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Snapshot restore action in work queue no longer exists.")
		return nil
	}
	return err
}

func (c *TridentCrdController) getKubernetesObjectsForActionSnapshotRestore(
	ctx context.Context, actionCR *netappv1.TridentActionSnapshotRestore,
) (tridentVolume, tridentSnapshot string, err error) {
	// Get PVC
	pvc, err := c.kubeClientset.CoreV1().PersistentVolumeClaims(actionCR.Namespace).Get(
		ctx, actionCR.Spec.PVCName, getOpts)
	if err != nil {
		return tridentVolume, tridentSnapshot, err
	}

	// Ensure PVC is bound
	if pvc.Status.Phase != v1.ClaimBound {
		err = fmt.Errorf("PVC %s/%s is not bound to a PV", pvc.Namespace, pvc.Name)
		return tridentVolume, tridentSnapshot, err
	}

	// Get the PV to which the PVC is bound and validate its status
	pv, err := c.kubeClientset.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, getOpts)
	if err != nil {
		return tridentVolume, tridentSnapshot, err
	}

	// Ensure PV is bound to the PVC
	if pv.Status.Phase != v1.VolumeBound || pv.Spec.ClaimRef == nil ||
		pv.Spec.ClaimRef.Namespace != pvc.Namespace || pv.Spec.ClaimRef.Name != pvc.Name {
		err = fmt.Errorf("PV %s is not bound to PVC %s/%s", pv.Name, pvc.Namespace, pvc.Name)
		return tridentVolume, tridentSnapshot, err
	}

	// Get VolumeSnapshot
	vs, err := c.snapshotClientSet.SnapshotV1().VolumeSnapshots(actionCR.Namespace).Get(
		ctx, actionCR.Spec.VolumeSnapshotName, getOpts)
	if err != nil {
		return tridentVolume, tridentSnapshot, err
	}

	// Ensure VS is bound and validate its status
	if vs.Status.BoundVolumeSnapshotContentName == nil || vs.Status.CreationTime == nil ||
		vs.Status.CreationTime.IsZero() || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		err = fmt.Errorf("volume snapshot %s/%s is not bound and ready to use", vs.Namespace, vs.Name)
		return tridentVolume, tridentSnapshot, err
	}

	// Get VolumeSnapshotContent
	vsc, err := c.snapshotClientSet.SnapshotV1().VolumeSnapshotContents().Get(
		ctx, *vs.Status.BoundVolumeSnapshotContentName, getOpts)
	if err != nil {
		return tridentVolume, tridentSnapshot, err
	}

	// Ensure VSC is bound to the VS and is ready to use and has a valid handle
	if vsc.Spec.VolumeSnapshotRef.Name != vs.Name || vsc.Spec.VolumeSnapshotRef.Namespace != vs.Namespace {
		err = fmt.Errorf("volume snapshot content %s is not bound to snapshot %s/%s", vsc.Name, vs.Namespace, vs.Name)
		return tridentVolume, tridentSnapshot, err
	}
	if vsc.Status.ReadyToUse == nil || !*vsc.Status.ReadyToUse {
		err = fmt.Errorf("volume snapshot content %s is not ready to use", vsc.Name)
		return tridentVolume, tridentSnapshot, err
	}
	if vsc.Status.SnapshotHandle == nil {
		err = fmt.Errorf("volume snapshot content %s does not have a snapshot handle", vsc.Name)
		return tridentVolume, tridentSnapshot, err
	}
	tridentVolume, tridentSnapshot, err = storage.ParseSnapshotID(*vsc.Status.SnapshotHandle)
	if err != nil {
		err = fmt.Errorf("volume snapshot content %s does not have a valid snapshot handle", vsc.Name)
		return tridentVolume, tridentSnapshot, err
	}

	// Ensure the VS is the most recent one on the PVC
	snapshotList, err := c.snapshotClientSet.SnapshotV1().VolumeSnapshots(actionCR.Namespace).List(ctx, listOpts)
	if err != nil {
		return tridentVolume, tridentSnapshot, err
	}

	for _, snapshot := range snapshotList.Items {

		// Skip the one we're restoring
		if snapshot.Namespace == vs.Namespace && snapshot.Name == vs.Name {
			continue
		}

		if snapshot.Spec.Source.PersistentVolumeClaimName != nil &&
			*snapshot.Spec.Source.PersistentVolumeClaimName == pvc.Name &&
			snapshot.Status.CreationTime != nil &&
			!snapshot.Status.CreationTime.IsZero() &&
			snapshot.Status.CreationTime.After(vs.Status.CreationTime.Time) {
			err = fmt.Errorf("volume snapshot %s is not the newest snapshot of PVC %s/%s",
				vs.Name, pvc.Namespace, pvc.Name)
			return tridentVolume, tridentSnapshot, err
		}
	}

	return tridentVolume, tridentSnapshot, err
}
