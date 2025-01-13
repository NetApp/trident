// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils/errors"
)

type (
	AppStatus    string
	ResourceType string
)

// updateTMRHandler is the update handler for the TridentMirrorRelationship watcher
// validates that the update of the TMR is allowed and if so, adds it to the workqueue
func (c *TridentCrdController) updateTMRHandler(old, new interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Debug("TridentCrdController#updateTMRHandler")
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		Logx(ctx).Error(err)
		return
	}

	newRelationship := new.(*netappv1.TridentMirrorRelationship)
	oldRelationship := old.(*netappv1.TridentMirrorRelationship)

	currentState := ""
	logFields := LogFields{}

	if newRelationship != nil {
		if len(newRelationship.Status.Conditions) > 0 {
			currentState = newRelationship.Status.Conditions[0].MirrorState
		}

		logFields = LogFields{"TridentMirrorRelationship": newRelationship.Name}
		Logx(ctx).WithFields(logFields).Tracef("Checking if update is needed %v", currentState)

		// Ensure localPVCName and remoteVolumeHandle (if already set) are not being changed
		if oldRelationship != nil && len(newRelationship.Status.Conditions) > 0 && len(newRelationship.Spec.VolumeMappings) > 0 {
			oldPVCName := newRelationship.Status.Conditions[0].LocalPVCName
			newPVCName := newRelationship.Spec.VolumeMappings[0].LocalPVCName
			if oldPVCName != "" && oldPVCName != newPVCName {
				Logx(ctx).WithFields(logFields).WithFields(LogFields{
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
				Logx(ctx).WithFields(logFields).WithFields(LogFields{
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

		// Ensure replicationPolicy and replicationSchedule (if already set) are not being changed
		// Ignore changes if relationship is transitioning to promoted (or deleted)
		conditionCopy, err := c.validateTMRUpdate(ctx, oldRelationship, newRelationship)
		if err != nil {
			_, err = c.updateTMRStatus(ctx, newRelationship, conditionCopy)
			if err != nil {
				Logx(ctx).WithFields(logFields).WithError(err).Error("Unable to update TMR status.")
			}
			return
		}

		if !newRelationship.ObjectMeta.DeletionTimestamp.IsZero() {
			c.addEventToWorkqueue(key, EventUpdate, ctx, newRelationship.GetKind())
			return
		} else if len(newRelationship.Status.Conditions) == 0 || collection.ContainsString(
			netappv1.GetTransitioningMirrorStatusStates(), newRelationship.Status.Conditions[0].MirrorState,
		) {
			// TMR is currently transitioning actual states
			c.addEventToWorkqueue(key, EventUpdate, ctx, newRelationship.GetKind())
			return
		} else if oldRelationship.Generation == newRelationship.Generation {
			// User has not changed the TMR
			return
		}
	}

	c.updateCRHandler(old, new)
}

// updateTMRConditionLocalFields returns a new TridentMirrorRelationshipCondition object with the specified fields set
func updateTMRConditionLocalFields(
	statusCondition *netappv1.TridentMirrorRelationshipCondition,
	localPVCName,
	remoteVolumeHandle string,
) (*netappv1.TridentMirrorRelationshipCondition, error) {
	conditionCopy := statusCondition.DeepCopy()
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
	ctx context.Context,
	tmr *netappv1.TridentMirrorRelationship,
) (*netappv1.TridentMirrorRelationship, error) {
	logFields := LogFields{"TridentMirrorRelationship": tmr.Name}

	// Update phase of the tmrCR
	Logx(ctx).WithFields(logFields).Trace("Updating the TridentMirrorRelationship CR")

	newTMR, err := c.crdClientset.TridentV1().TridentMirrorRelationships(tmr.Namespace).Update(ctx, tmr, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(logFields).Errorf("could not update TridentMirrorRelationship CR; %v", err)
	}

	return newTMR, err
}

// handleTridentMirrorRelationship ensures we move to the desired state and the desired state is maintained
func (c *TridentCrdController) handleTridentMirrorRelationship(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	logFields := LogFields{"TridentMirrorRelationship": keyItem.key}
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
		if k8sapierrors.IsNotFound(err) {
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
		Logx(ctx).WithFields(logFields).Tracef("Adding finalizer.")
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
			return errors.ReconcileIncompleteError("deleting TridentMirrorRelationship")
		}

		Logx(ctx).WithFields(logFields).Tracef("Removing TridentMirrorRelationship finalizers.")
		return c.removeFinalizers(ctx, mirrorRCopy, false)
	}

	validMR, reason := mirrorRCopy.IsValid()
	if validMR {
		Logx(ctx).WithFields(logFields).Debug("Valid TridentMirrorRelationship provided.")
		statusCondition, err = c.handleIndividualVolumeMapping(
			ctx, mirrorRCopy, mirrorRCopy.Spec.VolumeMappings[0], statusCondition,
		)
		if err != nil {
			if api.IsNotReadyError(err) {
				return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
			}
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
	if collection.ContainsString(netappv1.GetTransitioningMirrorStatusStates(), statusCondition.MirrorState) {
		err = errors.ReconcileIncompleteError(
			"TridentMirrorRelationship %v in state %v", relationship.Name, statusCondition.MirrorState,
		)
	}
	return err
}

func (c *TridentCrdController) getCurrentMirrorState(
	ctx context.Context,
	desiredMirrorState,
	backendUUID,
	localInternalVolumeName,
	remoteVolumeHandle string,
) (string, error) {
	currentMirrorState, err := c.orchestrator.GetMirrorStatus(ctx, backendUUID, localInternalVolumeName, remoteVolumeHandle)
	if err != nil {
		// Unsupported backends are always "promoted"
		if errors.IsUnsupportedError(err) {
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
	relCopy := relationship.DeepCopy()
	relCopy.Spec.MirrorState = netappv1.MirrorStatePromoted
	// We do not want to wait for a snapshot to appear if we are deleting
	volumeMapping.PromotedSnapshotHandle = ""

	// If the mirror is currently broken, we are safe to delete the TMR and release the relationship metadata
	if currentCondition.MirrorState == "" || currentCondition.MirrorState == netappv1.MirrorStatePromoted {
		relCopy.Spec.MirrorState = netappv1.MirrorStateReleased
	}

	status, err := c.handleIndividualVolumeMapping(ctx, relCopy, volumeMapping, currentCondition)
	if err != nil {
		// If any of the snapmirror operations fail, retry
		if api.IsNotReadyError(err) {
			return false, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
		}

		// If the underlying volume does not exist, we are safe to delete the TMR
		if errors.IsReconcileDeferredError(err) {
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
	logFields := LogFields{"TridentMirrorRelationship": relationship.Name}
	// Clear the status condition message
	statusCondition := currentCondition.DeepCopy()
	statusCondition.Message = ""
	localPVCName := volumeMapping.LocalPVCName

	// Check if local PVC exists
	localPVC, err := c.kubeClientset.CoreV1().PersistentVolumeClaims(relationship.Namespace).Get(
		ctx, localPVCName, metav1.GetOptions{},
	)
	statusErr, ok := err.(*k8sapierrors.StatusError)
	if (ok && statusErr.Status().Reason == metav1.StatusReasonNotFound) || localPVC == nil {
		message := fmt.Sprintf("Local PVC for TridentMirrorRelationship does not yet exist.")
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Trace(message)
		// If PVC does not yet exist, do not update the TMR and retry later
		return nil, errors.ReconcileDeferredError(message)
	} else if err != nil {
		return nil, err
	}

	// Check if local PVC is bound to a PV
	localPV, _ := c.kubeClientset.CoreV1().PersistentVolumes().Get(ctx, localPVC.Spec.VolumeName, metav1.GetOptions{})
	if localPV == nil || localPV.Spec.CSI == nil || localPV.Spec.CSI.VolumeAttributes == nil {
		message := fmt.Sprintf("PV for local PVC for TridentMirrorRelationship does not yet exist.")
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Trace(message)
		return nil, errors.ReconcileDeferredError(message)
	}
	// Check if PV has internal name set
	if localPV.Spec.CSI.VolumeAttributes["internalName"] == "" {
		message := fmt.Sprintf(
			"PV for local PVC for TridentMirrorRelationship does not yet have an internal volume name set.")
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Trace(message)
		return nil, errors.ReconcileDeferredError(message)
	}

	existingVolume, err := c.orchestrator.GetVolume(ctx, localPV.Spec.CSI.VolumeHandle)
	if err != nil {
		statusCondition.MirrorState = netappv1.MirrorStateFailed
		Logx(ctx).WithFields(logFields).Errorf("Failed to apply TridentMirrorRelationship update.")

		if errors.IsNotFoundError(err) {
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
		return updateTMRConditionLocalFields(statusCondition, localPVCName, volumeMapping.RemoteVolumeHandle)
	}

	// If we not trying to promote the mirror then we should verify the backend supports mirroring
	if relationship.Spec.MirrorState != netappv1.MirrorStatePromoted {
		if mirrorCapable, err := c.orchestrator.CanBackendMirror(ctx, existingVolume.BackendUUID); err != nil {
			statusCondition.MirrorState = netappv1.MirrorStateFailed
			statusCondition.Message = "Error checking if localPVC's backend can support mirroring"
			Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).WithError(err).Error(statusCondition.Message)
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message)
			return updateTMRConditionLocalFields(statusCondition, localPVCName, volumeMapping.RemoteVolumeHandle)
		} else if !mirrorCapable {
			statusCondition.MirrorState = netappv1.MirrorStateInvalid
			statusCondition.Message = "localPVC's backend does not support mirroring"
			Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Warn(statusCondition.Message)
			c.recorder.Eventf(
				relationship, corev1.EventTypeWarning, netappv1.MirrorStateInvalid, statusCondition.Message)
			return updateTMRConditionLocalFields(statusCondition, localPVCName, volumeMapping.RemoteVolumeHandle)
		}
	}

	localInternalVolumeName := existingVolume.Config.InternalName
	remoteVolumeHandle := volumeMapping.RemoteVolumeHandle

	desiredMirrorState := relationship.Spec.MirrorState
	currentMirrorState, _ := c.getCurrentMirrorState(
		ctx, desiredMirrorState, existingVolume.BackendUUID, localInternalVolumeName, remoteVolumeHandle,
	)

	// Release any previous snapmirror relationship
	if relationship.Spec.MirrorState == netappv1.MirrorStateReleased {
		statusCondition.Message = "Releasing snapmirror metadata"
		if err := c.orchestrator.ReleaseMirror(ctx, existingVolume.BackendUUID, localInternalVolumeName); err != nil {
			Logx(ctx).WithError(err).Error("Error releasing snapmirror")
		}

		return updateTMRConditionLocalFields(
			statusCondition, localPVCName, volumeMapping.RemoteVolumeHandle)
	}

	// If we are not already at our desired state on the backend
	if currentMirrorState != desiredMirrorState {
		// Ensure we finish the current operation before changing what we are doing
		if collection.ContainsString(netappv1.GetTransitioningMirrorStatusStates(), currentMirrorState) {
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
			Logx(ctx).WithFields(logFields).Debugf("Attempting to establish mirror")

			// Pass in the replicationPolicy and replicationSchedule to establish mirror
			// Initialize snapmirror relationship
			err = c.orchestrator.EstablishMirror(
				ctx, existingVolume.BackendUUID, localInternalVolumeName, remoteVolumeHandle,
				relationship.Spec.ReplicationPolicy, relationship.Spec.ReplicationSchedule)
			if err != nil && !api.IsNotReadyError(err) {
				currentMirrorState = netappv1.MirrorStateFailed
				statusCondition.Message = "Could not establish mirror"
				Logx(ctx).WithFields(logFields).WithError(err).Error(statusCondition.Message)
				c.recorder.Eventf(
					relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message,
				)
			} else if api.IsNotReadyError(err) {
				update, _ := updateTMRConditionLocalFields(statusCondition, localPVCName,
					volumeMapping.RemoteVolumeHandle)
				return update, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
			} else {
				// If we performed an action, get new mirror state
				currentMirrorState, _ = c.getCurrentMirrorState(
					ctx, desiredMirrorState, existingVolume.BackendUUID, localInternalVolumeName, remoteVolumeHandle,
				)
			}
		} else if desiredMirrorState == netappv1.MirrorStateReestablished &&
			remoteVolumeHandle != "" &&
			currentMirrorState != netappv1.MirrorStateReestablished {
			Logx(ctx).WithFields(logFields).Debugf("Attempting to reestablish mirror")
			// Resync snapmirror relationship
			err = c.orchestrator.ReestablishMirror(
				ctx, existingVolume.BackendUUID, localInternalVolumeName, remoteVolumeHandle,
				relationship.Spec.ReplicationPolicy, relationship.Spec.ReplicationSchedule)
			if err != nil {
				currentMirrorState = netappv1.MirrorStateFailed
				statusCondition.Message = "Could not reestablish mirror"
				Logx(ctx).WithFields(logFields).WithError(err).Error(statusCondition.Message)
				c.recorder.Eventf(
					relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message,
				)
			} else {
				// If we performed an action, get new mirror state
				currentMirrorState, _ = c.getCurrentMirrorState(
					ctx, desiredMirrorState, existingVolume.BackendUUID, localInternalVolumeName, remoteVolumeHandle,
				)
			}
		} else if desiredMirrorState == netappv1.MirrorStatePromoted &&
			currentMirrorState != netappv1.MirrorStatePromoted {
			Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Debugf(
				"Attempting to promote volume for mirror",
			)
			waitingForSnapshot, err := c.orchestrator.PromoteMirror(
				ctx, existingVolume.BackendUUID, localInternalVolumeName, remoteVolumeHandle,
				volumeMapping.PromotedSnapshotHandle,
			)
			if err != nil && !api.IsNotReadyError(err) {
				currentMirrorState = netappv1.MirrorStateFailed
				statusCondition.Message = "Could not promote mirror"
				Logx(ctx).WithFields(logFields).WithError(err).Error(statusCondition.Message)
				c.recorder.Eventf(
					relationship, corev1.EventTypeWarning, netappv1.MirrorStateFailed, statusCondition.Message,
				)
			} else if api.IsNotReadyError(err) {
				update, _ := updateTMRConditionLocalFields(statusCondition, localPVCName,
					volumeMapping.RemoteVolumeHandle)
				return update, err
			}

			if waitingForSnapshot {
				statusCondition.Message = fmt.Sprintf("Waiting for snapshot %v", volumeMapping.PromotedSnapshotHandle)
			}
		}
	}

	statusCondition.MirrorState = currentMirrorState

	statusCondition = c.updateTMRConditionReplicationSettings(ctx, statusCondition, existingVolume, localInternalVolumeName,
		remoteVolumeHandle)

	if errors.IsUnsupportedError(err) {
		statusCondition.MirrorState = netappv1.MirrorStateInvalid
		statusCondition.Message = err.Error()
		Logx(ctx).WithFields(logFields).WithField("PVC", localPVCName).Error(err)
		c.recorder.Eventf(relationship, corev1.EventTypeWarning, netappv1.MirrorStateInvalid, statusCondition.Message)
		return updateTMRConditionLocalFields(statusCondition, localPVCName, volumeMapping.RemoteVolumeHandle)
	}

	return updateTMRConditionLocalFields(
		statusCondition, localPVCName, volumeMapping.RemoteVolumeHandle)
}

func (c *TridentCrdController) updateTMRConditionReplicationSettings(
	ctx context.Context,
	statusCondition *netappv1.TridentMirrorRelationshipCondition,
	volume *storage.VolumeExternal,
	localInternalVolumeName,
	remoteVolumeHandle string,
) *netappv1.TridentMirrorRelationshipCondition {
	conditionCopy := statusCondition.DeepCopy()
	// Get the replication policy and schedule
	policy, schedule, SVMName, err := c.orchestrator.GetReplicationDetails(ctx, volume.BackendUUID,
		localInternalVolumeName, remoteVolumeHandle)
	if err != nil {
		Logx(ctx).Errorf("Error getting replication details: %v", err)
	} else {
		conditionCopy.ReplicationPolicy = policy
		conditionCopy.ReplicationSchedule = schedule
	}
	conditionCopy.LocalVolumeHandle = SVMName + ":" + localInternalVolumeName

	return conditionCopy
}

// validateTMRUpdate checks to see if there are changes in the replication policy or schedule and will return
// a new condition and error if the TMR update is invalid
func (c *TridentCrdController) validateTMRUpdate(
	ctx context.Context, oldRelationship,
	newRelationship *netappv1.TridentMirrorRelationship,
) (*netappv1.TridentMirrorRelationshipCondition, error) {
	logFields := LogFields{}
	logFields = LogFields{"TridentMirrorRelationship": newRelationship.Name}

	// Ignore changes if relationship is going to promoted (or deleted)
	if oldRelationship != nil && len(newRelationship.Status.Conditions) > 0 && newRelationship.Spec.
		MirrorState != netappv1.MirrorStatePromoted {
		oldReplicationPolicyStatus := newRelationship.Status.Conditions[0].ReplicationPolicy
		oldReplicationPolicySpec := oldRelationship.Spec.ReplicationPolicy
		newReplicationPolicySpec := newRelationship.Spec.ReplicationPolicy

		// Ensure replicationPolicy (if already set) is not being changed
		if oldReplicationPolicyStatus != "" {
			// Old replication policy in status must be equal to new policy to allow change
			// And old replication policy in spec must be equal to the new policy in spec to allow change
			if oldReplicationPolicyStatus != newReplicationPolicySpec &&
				newReplicationPolicySpec != oldReplicationPolicySpec {
				Logx(ctx).WithFields(logFields).WithFields(LogFields{
					"oldReplicationPolicySpec":   oldReplicationPolicySpec,
					"newReplicationPolicySpec":   newReplicationPolicySpec,
					"oldReplicationPolicyStatus": oldReplicationPolicyStatus,
				}).Error("replication policy cannot be changed, must delete TMR to change policy.")
				conditionCopy := newRelationship.Status.Conditions[0].DeepCopy()
				conditionCopy.MirrorState = netappv1.MirrorStateFailed
				conditionCopy.Message = fmt.Sprintf("replication policy may not be changed from %v",
					oldReplicationPolicyStatus)

				return conditionCopy, fmt.Errorf(
					"replication policy cannot be changed, must delete TMR to change policy.")
			}
		}

		// Ensure replicationSchedule (if already set) is not being changed
		oldReplicationScheduleStatus := newRelationship.Status.Conditions[0].ReplicationSchedule
		oldReplicationScheduleSpec := oldRelationship.Spec.ReplicationSchedule
		newReplicationScheduleSpec := newRelationship.Spec.ReplicationSchedule
		if oldReplicationScheduleStatus != "" {
			// Old replication schedule in status must be equal to new schedule to allow change
			// And old replication schedule in spec must be equal to the new schedule in spec to allow change
			if oldReplicationScheduleStatus != newReplicationScheduleSpec &&
				newReplicationScheduleSpec != oldReplicationScheduleSpec {
				Logx(ctx).WithFields(logFields).WithFields(LogFields{
					"oldReplicationScheduleSpec":   oldReplicationScheduleSpec,
					"newReplicationScheduleSpec":   newReplicationScheduleSpec,
					"oldReplicationScheduleStatus": oldReplicationScheduleStatus,
				}).Error("replication schedule cannot be changed, must delete TMR to change schedule.")
				conditionCopy := newRelationship.Status.Conditions[0].DeepCopy()
				conditionCopy.MirrorState = netappv1.MirrorStateFailed
				conditionCopy.Message = fmt.Sprintf("replication schedule may not be changed from %v",
					oldReplicationScheduleStatus)

				return conditionCopy, fmt.Errorf(
					"replication schedule cannot be changed, must delete TMR to change schedule.")
			}
		}
	}
	return nil, nil
}
