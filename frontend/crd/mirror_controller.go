// Copyright 2021 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logger"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils"
)

type AppStatus string
type ResourceType string

// addMirrorRelationship is the add handler for the TridentMirrorRelationship watcher.
// This is called for every TMR when Trident starts
func (c *TridentCrdController) addMirrorRelationship(obj interface{}) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventAdd))

	Logx(ctx).Debug("TridentCrdController#addMirrorRelationship")

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentMirrorRelationship,
	}
	// Only add it if it's new or needs cleanup for deletion
	newRelationship := obj.(*netappv1.TridentMirrorRelationship)
	if len(newRelationship.Status.Conditions) == 0 || int(newRelationship.Generation) != newRelationship.Status.Conditions[0].ObservedGeneration || !newRelationship.ObjectMeta.DeletionTimestamp.IsZero() {
		c.workqueue.Add(keyItem)
	}
}

// updateMirrorRelationship is the update handler for the TridentMirrorRelationship watcher.
func (c *TridentCrdController) updateMirrorRelationship(old, new interface{}) {

	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Debug("TridentCrdController#updateMirrorRelationship")
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventUpdate,
		ctx:        ctx,
		objectType: ObjectTypeTridentMirrorRelationship,
	}

	newRelationship := new.(*netappv1.TridentMirrorRelationship)
	oldRelationship := old.(*netappv1.TridentMirrorRelationship)
	currentState := ""
	needsUpdate := false
	logFields := log.Fields{}

	if len(newRelationship.Status.Conditions) > 0 {
		currentState = newRelationship.Status.Conditions[0].MirrorState
	}

	if newRelationship != nil {
		logFields = log.Fields{"TridentMirrorRelationship": newRelationship.Name}
		Logx(ctx).WithFields(logFields).Debugf("Checking if update is needed %v", currentState)

		// Ensure localPVCName and remoteVolumeHandle (if already set) are not being changed
		if oldRelationship != nil && len(newRelationship.Status.Conditions) > 0 && len(newRelationship.Spec.VolumeMappings) > 0 {
			oldPVCName := newRelationship.Status.Conditions[0].LocalPVCName
			newPVCName := newRelationship.Spec.VolumeMappings[0].LocalPVCName
			if oldPVCName != "" && oldPVCName != newPVCName {
				Logx(ctx).WithFields(logFields).WithFields(log.Fields{
					"oldLocalPVCName": oldPVCName,
					"newLocalPVCName": newPVCName,
				}).Error("localPVCName cannot be changed.")
				conditionCopy := newRelationship.Status.Conditions[0].DeepCopy()
				conditionCopy.MirrorState = netappv1.MirrorStateFailed
				conditionCopy.Message = fmt.Sprintf("localPVCName may not be changed from %v", oldPVCName)
				_, err = c.updateTMRStatus(ctx, newRelationship, conditionCopy)
				if err != nil {
					Logx(ctx).WithFields(logFields).WithError(err).Error("Unable to update TMR status.")
				}
				return
			}
			oldRemoteHandle := newRelationship.Status.Conditions[0].RemoteVolumeHandle
			newRemoteHandle := newRelationship.Spec.VolumeMappings[0].RemoteVolumeHandle
			if oldRemoteHandle != "" && newRemoteHandle != oldRemoteHandle {
				Logx(ctx).WithFields(logFields).WithFields(log.Fields{
					"oldRemoteVolumeHandle": oldRemoteHandle,
					"newRemoteVolumeHandle": newRemoteHandle,
				}).Error("remoteVolumeHandle cannot be changed once set.")
				condition := newRelationship.Status.Conditions[0].DeepCopy()
				condition.MirrorState = netappv1.MirrorStateFailed
				condition.Message = fmt.Sprintf("remoteVolumeHandle may not be changed from %v",
					condition.RemoteVolumeHandle)
				_, err = c.updateTMRStatus(ctx, newRelationship, condition)
				if err != nil {
					Logx(ctx).WithFields(logFields).WithError(err).Error("Unable to update TMR status.")
				}
				return
			}
		}

		if oldRelationship == nil {
			needsUpdate = true
		} else if !newRelationship.ObjectMeta.DeletionTimestamp.IsZero() {
			// TMR is pending deletion
			needsUpdate = true
		} else if len(newRelationship.Status.Conditions) == 0 {
			// TMR has not been updated before
			needsUpdate = true
		} else if oldRelationship.Generation != newRelationship.Generation {
			// User has changed the TMR
			needsUpdate = true
		} else if newRelationship.Status.Conditions[0].ObservedGeneration != int(newRelationship.Generation) {
			// User has changed the TMR
			needsUpdate = true
		} else if utils.SliceContainsString(
			netappv1.GetTransitioningMirrorStatusStates(), newRelationship.Status.Conditions[0].MirrorState,
		) {
			// TMR is currently transitioning actual states
			needsUpdate = true
		}
	}

	if needsUpdate {
		c.workqueue.Add(keyItem)
	} else {
		Logx(ctx).WithFields(logFields).Debugf("No required update for TridentMirrorRelationship")
	}
}

// deleteMirrorRelationship is the delete handler for the TridentMirrorRelationship watcher.
func (c *TridentCrdController) deleteMirrorRelationship(obj interface{}) {

	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

	Logx(ctx).Debug("TridentCrdController#deleteMirrorRelationship")

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentMirrorRelationship,
	}

	c.workqueue.Add(keyItem)
}

// updateTMRConditionLocalFields returns a new TridentMirrorRelationshipCondition object with the specified fields set
func updateTMRConditionLocalFields(
	statusCondition *netappv1.TridentMirrorRelationshipCondition,
	localVolumeHandle,
	localPVCName,
	remoteVolumeHandle string,
) (*netappv1.TridentMirrorRelationshipCondition, error) {

	conditionCopy := statusCondition.DeepCopy()
	if localVolumeHandle != "" {
		conditionCopy.LocalVolumeHandle = localVolumeHandle
	}
	conditionCopy.LocalPVCName = localPVCName
	conditionCopy.RemoteVolumeHandle = remoteVolumeHandle
	return conditionCopy, nil
}

// updateTMRStatus updates the TridentMirrorRelationship.status.conditions fields on the specified
// TridentMirrorRelationship resource using the kubernetes api
func (c *TridentCrdController) updateTMRStatus(
	ctx context.Context,
	relationship *netappv1.TridentMirrorRelationship,
	statusCondition *netappv1.TridentMirrorRelationshipCondition,
) (*netappv1.TridentMirrorRelationship, error) {

	// Create new status
	mirrorRCopy := relationship.DeepCopy()
	statusCondition.ObservedGeneration = int(mirrorRCopy.Generation)

	currentTime := time.Now()
	statusCondition.LastTransitionTime = currentTime.Format(time.RFC3339)
	if len(mirrorRCopy.Status.Conditions) == 0 {
		mirrorRCopy.Status.Conditions = append(mirrorRCopy.Status.Conditions, statusCondition)
	} else {
		mirrorRCopy.Status.Conditions[0] = statusCondition
	}

	return c.crdClientset.TridentV1().TridentMirrorRelationships(mirrorRCopy.Namespace).UpdateStatus(
		ctx, mirrorRCopy, updateOpts,
	)
}

// updateTMRCR updates the TridentMirrorRelationshipCR
func (c *TridentCrdController) updateTMRCR(
	ctx context.Context, tmr *netappv1.TridentMirrorRelationship,
) (*netappv1.TridentMirrorRelationship, error) {

	logFields := log.Fields{"TridentMirrorRelationship": tmr.Name}

	// Update phase of the tmrCR
	Logx(ctx).WithFields(logFields).Debug("Updating the TridentMirrorRelationship CR")

	newTMR, err := c.crdClientset.TridentV1().TridentMirrorRelationships(tmr.Namespace).Update(ctx, tmr, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(logFields).Errorf("could not update TridentMirrorRelationship CR; %v", err)
	}

	return newTMR, err
}

func (c *TridentCrdController) reconcileTMR(keyItem *KeyItem) error {

	Logx(keyItem.ctx).Debug("TridentCrdController#reconcileTMR")
	logFields := log.Fields{"TridentMirrorRelationship": keyItem.key}

	// Pass the namespace/name string of the resource to be synced.
	if err := c.handleTridentMirrorRelationship(keyItem); err != nil {

		if utils.IsReconcileDeferredError(err) {
			c.workqueue.AddRateLimited(*keyItem)
			errMessage := fmt.Sprintf(
				"deferred syncing TridentMirrorRelationship, requeuing; %v", err.Error(),
			)
			Logx(keyItem.ctx).WithFields(logFields).Info(errMessage)
			return err
		} else if utils.IsReconcileIncompleteError(err) {
			c.workqueue.Add(*keyItem)
			errMessage := fmt.Sprintf(
				"syncing TridentMirrorRelationship in progress, requeuing; %v", err.Error(),
			)
			Logx(keyItem.ctx).WithFields(logFields).Info(errMessage)

			return err
		} else {
			errMessage := fmt.Sprintf(
				"error syncing TridentMirrorRelationship, requeuing; %v", err.Error(),
			)
			Logx(keyItem.ctx).WithFields(logFields).Error(errMessage)

			return fmt.Errorf(errMessage)
		}
	}

	return nil
}

// handleTridentMirrorRelationship ensures we move to the desired state and the desired state is maintained
func (c *TridentCrdController) handleTridentMirrorRelationship(keyItem *KeyItem) error {

	key := keyItem.key
	ctx := keyItem.ctx

	logFields := log.Fields{"TridentMirrorRelationship": keyItem.key}
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid key.")
		return nil
	}

	// Get the resource with this namespace/name
	relationship, err := c.mirrorLister.TridentMirrorRelationships(namespace).Get(name)
	if err != nil {
		// The resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			Logx(ctx).WithField("key", key).Debug("Object in work queue no longer exists.")
			return nil
		}

		return err
	}

	mirrorRCopy := relationship.DeepCopy()
	statusCondition := &netappv1.TridentMirrorRelationshipCondition{
		LocalPVCName: relationship.Spec.VolumeMappings[0].LocalPVCName,
	}
	if len(mirrorRCopy.Status.Conditions) > 0 {
		statusCondition = mirrorRCopy.Status.Conditions[0]
	}
	mirrorRCopy.Status.Conditions = []*netappv1.TridentMirrorRelationshipCondition{statusCondition}
	// Ensure TMR is not deleting, then ensure it has a finalizer
	if mirrorRCopy.ObjectMeta.DeletionTimestamp.IsZero() && !mirrorRCopy.HasTridentFinalizers() {
		Logx(ctx).WithFields(logFields).Debugf("Adding finalizer.")
		mirrorRCopy.AddTridentFinalizers()

		if mirrorRCopy, err = c.updateTMRCR(ctx, mirrorRCopy); err != nil {
			return fmt.Errorf("error setting finalizer; %v", err)
		}
	} else if !mirrorRCopy.ObjectMeta.DeletionTimestamp.IsZero() {
		Logx(ctx).WithFields(logFields).WithField(
			"DeletionTimestamp", mirrorRCopy.ObjectMeta.DeletionTimestamp).Debug(
			"TridentCrdController#handleTridentMirrorRelationship CR is being deleted.")

		deleted, err := c.ensureMirrorReadyForDeletion(ctx, mirrorRCopy, mirrorRCopy.Spec.VolumeMappings[0],
			statusCondition)
		if err != nil {
			return err
		} else if !deleted {
			return utils.ConvertToReconcileIncompleteError(fmt.Errorf("deleting TridentMirrorRelationship"))
		}

		Logx(ctx).WithFields(logFields).Debugf("Removing TridentMirrorRelationship finalizers.")
		return c.removeFinalizers(ctx, mirrorRCopy, false)
	}

	validMR, reason := mirrorRCopy.IsValid()
	if validMR {
		Logx(ctx).WithFields(logFields).Debug("Valid TridentMirrorRelationship provided.")
		statusCondition, err = c.handleIndividualVolumeMapping(
			ctx, mirrorRCopy, mirrorRCopy.Spec.VolumeMappings[0], statusCondition,
		)
		if err != nil {
			return err
		}
	} else {
		Logx(ctx).WithFields(logFields).WithField("reason", reason).Debug("Invalid TridentMirrorRelationship provided.")
		c.recorder.Eventf(mirrorRCopy, corev1.EventTypeWarning, netappv1.MirrorStateInvalid, reason)

		if len(mirrorRCopy.Status.Conditions) > 0 {
			// For now, we only allow a single volumeMapping which should map to a single status condition
			statusCondition = mirrorRCopy.Status.Conditions[0]
		} else {
			statusCondition = &netappv1.TridentMirrorRelationshipCondition{LocalPVCName: mirrorRCopy.Spec.VolumeMappings[0].LocalPVCName}
		}
		statusCondition.MirrorState = netappv1.MirrorStateInvalid
		statusCondition.Message = reason
		statusCondition.LocalPVCName = mirrorRCopy.Spec.VolumeMappings[0].LocalPVCName
	}
	// Here we ensure we have the latest TMR before updating the status, adding the finalizer
	// would make our copy stale
	relationship, _ = c.mirrorLister.TridentMirrorRelationships(namespace).Get(name)
	var originalState string
	if len(relationship.Status.Conditions) > 0 {
		originalState = relationship.Status.Conditions[0].MirrorState
	}

	// Only if the actual state changed at all
	if originalState == "" || originalState != statusCondition.MirrorState {
		mirrorRCopy, updateErr := c.updateTMRStatus(ctx, relationship, statusCondition)
		if updateErr != nil {
			Logx(ctx).WithFields(logFields).Error(updateErr)
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning, statusCondition.MirrorState,
				"Could not update TridentMirrorRelationship",
			)
			return fmt.Errorf("could not update TridentMirrorRelationship status; %v", updateErr)
		} else if relationship.Spec.MirrorState == statusCondition.MirrorState {
			Logx(ctx).WithFields(logFields).Debugf(
				"Desired state of %v reached for TridentMirrorRelationship %v",
				statusCondition.MirrorState, mirrorRCopy.Name,
			)
			c.recorder.Eventf(
				relationship, corev1.EventTypeNormal, statusCondition.MirrorState,
				"Desired state reached",
			)
		}
	}
	if utils.SliceContainsString(netappv1.GetTransitioningMirrorStatusStates(), statusCondition.MirrorState) {
		err = utils.ConvertToReconcileIncompleteError(
			fmt.Errorf(
				"TridentMirrorRelationship %v in state %v", relationship.Name, statusCondition.MirrorState,
			),
		)
	}
	return err
}

func (c *TridentCrdController) getCurrentMirrorState(
	ctx context.Context, desiredMirrorState, backendUUID, localVolumeHandle, remoteVolumeHandle string,
) (string, error) {

	currentMirrorState, err := c.orchestrator.GetMirrorStatus(ctx, backendUUID, localVolumeHandle, remoteVolumeHandle)
	if err != nil {
		// Unsupported backends are always "promoted"
		if utils.IsUnsupportedError(err) {
			return netappv1.MirrorStatePromoted, nil
		}
		return "", err
	}
	if desiredMirrorState == netappv1.MirrorStateReestablished {
		if currentMirrorState == netappv1.MirrorStateEstablishing {
			return netappv1.MirrorStateReestablishing, nil
		} else if currentMirrorState == netappv1.MirrorStateEstablished {
			return netappv1.MirrorStateReestablished, nil
		}
	} else if desiredMirrorState == netappv1.MirrorStatePromoted &&
		(currentMirrorState == "" || currentMirrorState == netappv1.MirrorStatePromoted) {
		currentMirrorState = netappv1.MirrorStatePromoted
	} else if desiredMirrorState == netappv1.MirrorStatePromoted && currentMirrorState != netappv1.MirrorStatePromoted {
		currentMirrorState = netappv1.MirrorStatePromoting
	}
	return currentMirrorState, err
}

// ensureMirrorReadyForDeletion forces the mirror relationship to be broken by promoting the TMR
// returns true if the TMR is ready to be deleted, false if there is more work to do
func (c *TridentCrdController) ensureMirrorReadyForDeletion(
	ctx context.Context,
	relationship *netappv1.TridentMirrorRelationship,
	volumeMapping *netappv1.TridentMirrorRelationshipVolumeMapping,
	currentCondition *netappv1.TridentMirrorRelationshipCondition,
) (bool, error) {

	// If the mirror is currently broken, we are safe to delete the TMR
	if currentCondition.MirrorState == "" || currentCondition.MirrorState == netappv1.MirrorStatePromoted {
		return true, nil
	}

	relCopy := relationship.DeepCopy()
	relCopy.Spec.MirrorState = netappv1.MirrorStatePromoted
	// We do not want to wait for a snapshot to appear if we are deleting
	volumeMapping.LatestSnapshotHandle = ""
	status, err := c.handleIndividualVolumeMapping(ctx, relCopy, volumeMapping, currentCondition)
	if err != nil {
		// If the underlying volume does not exist, we are safe to delete the TMR
		if utils.IsReconcileDeferredError(err) {
			return true, nil
		}
		return false, err
	}
	return status.MirrorState == netappv1.MirrorStatePromoted || status.MirrorState == "", nil
}

func (c *TridentCrdController) handleIndividualVolumeMapping(
	ctx context.Context,
	relationship *netappv1.TridentMirrorRelationship,
	volumeMapping *netappv1.TridentMirrorRelationshipVolumeMapping,
	currentCondition *netappv1.TridentMirrorRelationshipCondition,
) (*netappv1.TridentMirrorRelationshipCondition, error) {

	logFields := log.Fields{"TridentMirrorRelationship": relationship.Name}
	// Clear the status condition message
	statusCondition := currentCondition.DeepCopy()
	statusCondition.Message = ""
	localPVCName := volumeMapping.LocalPVCName

	// Check if local PVC exists
	localPVC, err := c.kubeClientset.CoreV1().PersistentVolumeClaims(relationship.Namespace).Get(
		ctx, localPVCName, metav1.GetOptions{},
	)
	statusErr, ok := err.(*errors.StatusError)
	if (ok && statusErr.Status().Reason == metav1.StatusReasonNotFound) || localPVC == nil {
		message := fmt.Sprintf("Local PVC for TridentMirrorRelationship does not yet exist.")
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debug(message)
		// If PVC does not yet exist, do not update the TMR and retry later
		return nil, utils.ReconcileDeferredError(fmt.Errorf(message))
	} else if err != nil {
		return nil, err
	}

	// Check if local PVC is bound to a PV
	localPV, _ := c.kubeClientset.CoreV1().PersistentVolumes().Get(ctx, localPVC.Spec.VolumeName, metav1.GetOptions{})
	if localPV == nil || localPV.Spec.CSI == nil || localPV.Spec.CSI.VolumeAttributes == nil {
		message := fmt.Sprintf("PV for local PVC for TridentMirrorRelationship does not yet exist.")
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debug(message)
		return nil, utils.ReconcileDeferredError(fmt.Errorf(message))
	}
	// Check if PV has internal name set
	if localPV.Spec.CSI.VolumeAttributes["internalName"] == "" {
		message := fmt.Sprintf(
			"PV for local PVC for TridentMirrorRelationship does not yet have an internal volume name set.")
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debug(message)
		return nil, utils.ReconcileDeferredError(fmt.Errorf(message))
	}

	existingVolume, err := c.orchestrator.GetVolume(ctx, localPV.Spec.CSI.VolumeHandle)
	if err != nil {
		statusCondition.MirrorState = netappv1.MirrorStateFailed
		Logx(ctx).WithFields(logFields).Debugf("Failed to apply TridentMirrorRelationship update.")

		if utils.IsNotFoundError(err) {
			Logx(ctx).WithError(err).WithFields(logFields).WithField(
				"VolumeHandle", localPV.Spec.CSI.VolumeHandle).Errorf("Volume does not exist on backend")
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning,
				netappv1.MirrorStateFailed,
				"Failed to apply TridentMirrorRelationship update: Volume does not exist on backend",
			)
		} else {
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning,
				netappv1.MirrorStateFailed, "Failed to apply TridentMirrorRelationship update",
			)
		}
	}
	if existingVolume == nil {
		statusCondition.MirrorState = netappv1.MirrorStateFailed
		statusCondition.Message = fmt.Sprintf(
			"Could not find volume at volume handle: %v", localPV.Spec.CSI.VolumeHandle,
		)
		Logx(ctx).WithFields(logFields).Debugf(statusCondition.Message)
		c.recorder.Eventf(relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message)
		return updateTMRConditionLocalFields(statusCondition, "", localPVCName, volumeMapping.RemoteVolumeHandle)
	}

	// If we not trying to promote the mirror then we should verify the backend supports mirroring
	if relationship.Spec.MirrorState != netappv1.MirrorStatePromoted {
		if mirrorCapable, err := c.orchestrator.CanBackendMirror(ctx, existingVolume.BackendUUID); err != nil {
			statusCondition.MirrorState = netappv1.MirrorStateFailed
			statusCondition.Message = "Error checking if localPVC's backend can support mirroring"
			Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).WithError(err).Error(statusCondition.Message)
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message)
			return updateTMRConditionLocalFields(statusCondition, "", localPVCName, volumeMapping.RemoteVolumeHandle)
		} else if !mirrorCapable {
			statusCondition.MirrorState = netappv1.MirrorStateInvalid
			statusCondition.Message = "localPVC's backend does not support mirroring"
			Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debug(statusCondition.Message)
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning, netappv1.MirrorStateInvalid, statusCondition.Message)
			return updateTMRConditionLocalFields(statusCondition, "", localPVCName, volumeMapping.RemoteVolumeHandle)
		}
	}

	localVolumeHandle := existingVolume.Config.MirrorHandle
	remoteVolumeHandle := volumeMapping.RemoteVolumeHandle

	desiredMirrorState := relationship.Spec.MirrorState
	currentMirrorState, _ := c.getCurrentMirrorState(
		ctx, desiredMirrorState, existingVolume.BackendUUID, localVolumeHandle, remoteVolumeHandle,
	)

	// If we are not already at our desired state on the backend
	if currentMirrorState != desiredMirrorState {
		// Ensure we finish the current operation before changing what we are doing
		if utils.SliceContainsString(netappv1.GetTransitioningMirrorStatusStates(), currentMirrorState) {
			switch currentMirrorState {
			case netappv1.MirrorStateEstablishing:
				desiredMirrorState = netappv1.MirrorStateEstablished
			case netappv1.MirrorStateReestablishing:
				desiredMirrorState = netappv1.MirrorStateReestablished
			case netappv1.MirrorStatePromoting:
				desiredMirrorState = netappv1.MirrorStatePromoted
			}
		}

		if desiredMirrorState == netappv1.MirrorStateEstablished &&
			remoteVolumeHandle != "" &&
			currentMirrorState != netappv1.MirrorStateEstablished {
			Logc(ctx).WithFields(logFields).Debugf("Attempting to establish mirror")
			// Initialize snapmirror relationship
			err = c.orchestrator.EstablishMirror(
				ctx, existingVolume.BackendUUID, localVolumeHandle, remoteVolumeHandle,
			)
			if err != nil {
				currentMirrorState = netappv1.MirrorStateFailed
				statusCondition.Message = "Could not establish mirror"
				Logx(ctx).WithFields(logFields).WithError(err).Info(statusCondition.Message)
				c.recorder.Eventf(
					relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message,
				)
			} else {
				// If we performed an action, get new mirror state
				currentMirrorState, _ = c.getCurrentMirrorState(
					ctx, desiredMirrorState, existingVolume.BackendUUID, localVolumeHandle, remoteVolumeHandle,
				)
			}
		} else if desiredMirrorState == netappv1.MirrorStateReestablished &&
			remoteVolumeHandle != "" &&
			currentMirrorState != netappv1.MirrorStateReestablished {
			Logc(ctx).WithFields(logFields).Debugf("Attempting to reestablish mirror")
			// Resync snapmirror relationship
			err = c.orchestrator.ReestablishMirror(
				ctx, existingVolume.BackendUUID, localVolumeHandle, remoteVolumeHandle,
			)
			if err != nil {
				currentMirrorState = netappv1.MirrorStateFailed
				statusCondition.Message = "Could not reestablish mirror"
				Logx(ctx).WithFields(logFields).WithError(err).Info(statusCondition.Message)
				c.recorder.Eventf(
					relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message,
				)
			} else {
				// If we performed an action, get new mirror state
				currentMirrorState, _ = c.getCurrentMirrorState(
					ctx, desiredMirrorState, existingVolume.BackendUUID, localVolumeHandle, remoteVolumeHandle,
				)
			}
		} else if desiredMirrorState == netappv1.MirrorStatePromoted &&
			currentMirrorState != netappv1.MirrorStatePromoted {
			Logc(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debugf(
				"Attempting to promote volume for mirror",
			)
			waitingForSnapshot, err := c.orchestrator.PromoteMirror(
				ctx, existingVolume.BackendUUID, localVolumeHandle, remoteVolumeHandle,
				volumeMapping.LatestSnapshotHandle,
			)
			if err != nil {
				currentMirrorState = netappv1.MirrorStateFailed
				statusCondition.Message = "Could not promote mirror"
				Logx(ctx).WithFields(logFields).WithError(err).Info(statusCondition.Message)
				c.recorder.Eventf(
					relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message,
				)
			}
			if waitingForSnapshot {
				statusCondition.Message = fmt.Sprintf("Waiting for snapshot %v", volumeMapping.LatestSnapshotHandle)
			}
		}
	}

	statusCondition.MirrorState = currentMirrorState

	if utils.IsUnsupportedError(err) {
		statusCondition.MirrorState = netappv1.MirrorStateInvalid
		statusCondition.Message = err.Error()
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debug(err)
		c.recorder.Eventf(relationship, corev1.EventTypeWarning, netappv1.MirrorStateInvalid, statusCondition.Message)
		return updateTMRConditionLocalFields(statusCondition, "", localPVCName, volumeMapping.RemoteVolumeHandle)
	}

	return updateTMRConditionLocalFields(
		statusCondition, localVolumeHandle, localPVCName, volumeMapping.RemoteVolumeHandle)

}
