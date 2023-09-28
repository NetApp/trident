// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// handleTridentBackendConfig compares the actual state with the desired, and attempts to converge the two.
// It then updates the Status block of the TridentBackendConfig resource with the current status of the resource.
func (c *TridentCrdController) handleTridentBackendConfig(keyItem *KeyItem) error {
	if keyItem == nil {
		return errors.ReconcileDeferredError("keyItem item is nil")
	}

	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(LogFields{
		"Key":        key,
		"eventType":  eventType,
		"objectType": objectType,
	}).Trace("TridentCrdController#handleTridentBackendConfig")

	var backendConfig *tridentv1.TridentBackendConfig

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid key.")
		return nil
	}

	// Get the CR with this namespace/name
	backendConfig, err = c.backendConfigsLister.TridentBackendConfigs(namespace).Get(name)
	if err != nil {
		// The resource may no longer exist, in which case we stop processing.
		if k8sapierrors.IsNotFound(err) {
			Logx(ctx).WithField("key", key).Trace("Object in work queue no longer exists.")
			return nil
		}
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	// Ensure backendconfig is not deleting, then ensure it has a finalizer
	if backendConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		backendConfigCopy := backendConfig.DeepCopy()

		if !backendConfigCopy.HasTridentFinalizers() {
			Logx(ctx).WithField("backendConfig.Name", backendConfigCopy.Name).Debugf("Adding finalizer.")
			backendConfigCopy.AddTridentFinalizers()

			if backendConfig, err = c.updateTridentBackendConfigCR(ctx, backendConfigCopy); err != nil {
				return multierr.Combine(errors.ReconcileDeferredError("error setting finalizer"), err)
			}
		}
	} else {
		Logx(ctx).WithFields(LogFields{
			"backendConfig.Name":                   backendConfig.Name,
			"backend.ObjectMeta.DeletionTimestamp": backendConfig.ObjectMeta.DeletionTimestamp,
		}).Trace("TridentCrdController#handleTridentBackendConfig CR is being deleted, not updating.")
		eventType = EventDelete
	}

	// Check to see if the backend config deletion was initialized in any of the previous reconcileBackendConfig loops
	if backendConfig.Status.Phase == string(tridentv1.PhaseDeleting) {
		Logx(ctx).WithFields(LogFields{
			"backendConfig.Name":                         backendConfig.Name,
			"backendConfig.ObjectMeta.DeletionTimestamp": backendConfig.ObjectMeta.DeletionTimestamp,
		}).Debugf("TridentCrdController# CR has %s phase.", string(tridentv1.PhaseDeleting))
		eventType = EventDelete
	}

	// Ensure we have a valid Spec, and fields are properly set
	if err = backendConfig.Validate(); err != nil {
		backendInfo := tridentv1.TridentBackendConfigBackendInfo{
			BackendName: backendConfig.Status.BackendInfo.BackendName,
			BackendUUID: backendConfig.Status.BackendInfo.BackendUUID,
		}

		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to process backend: %v", err),
			BackendInfo:         backendInfo,
			Phase:               backendConfig.Status.Phase,
			DeletionPolicy:      backendConfig.Status.DeletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		var statusErr error
		if _, statusErr = c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Failed to process backend.",
			corev1.EventTypeWarning); statusErr != nil {
			err = errors.ReconcileDeferredError(
				"validation error: %v, Also encountered error while updating the status: %v", err, statusErr,
			)
		}

		return errors.WrapUnsupportedConfigError(err)
	}

	// Retrieve the deletion policy
	deletionPolicy, err := backendConfig.Spec.GetDeletionPolicy()
	if err != nil {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to retrieve deletion policy: %v", err),
			BackendInfo:         backendConfig.Status.BackendInfo,
			Phase:               backendConfig.Status.Phase,
			DeletionPolicy:      backendConfig.Status.DeletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
			"Failed to retrieve deletion policy.", corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf("errror during deletion policy retrieval: %v, "+
				"Also encountered error while updating the status: %v", err, statusErr)
		}

		return errors.ReconcileDeferredError("encountered error while retrieving the deletion policy :%v", err)
	}

	Logx(ctx).WithFields(LogFields{
		"Spec": backendConfig.Spec.ToString(),
	}).Trace("TridentBackendConfig Spec is valid.")

	phase := tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)

	if eventType == EventDelete {
		phase = tridentv1.PhaseDeleting
	} else if eventType == EventForceUpdate {
		// could also be Lost or Unknown, aim is to run an update
		phase = tridentv1.PhaseBound
	}

	switch phase {
	case tridentv1.PhaseUnbound:
		// We add a new backend or bind to an existing one
		err = c.addBackendConfig(ctx, backendConfig, deletionPolicy)
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
		}
	case tridentv1.PhaseBound, tridentv1.PhaseLost, tridentv1.PhaseUnknown:
		// We update the CR
		err = c.updateBackendConfig(ctx, backendConfig, deletionPolicy)
		if err != nil {
			if errors.IsUnsupportedConfigError(err) {
				return err
			}
			return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
		}
	case tridentv1.PhaseDeleting:
		// We delete the CR
		err = c.deleteBackendConfig(ctx, backendConfig, deletionPolicy)
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
		}
	default:
		// This should never be the case
		return errors.UnsupportedConfigError("backend config has an unsupported phase: '%v'", phase)
	}

	return nil
}

func (c *TridentCrdController) addBackendConfig(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, deletionPolicy string,
) error {
	Logx(ctx).WithFields(LogFields{
		"backendConfig.Name": backendConfig.Name,
	}).Trace("TridentCrdController#addBackendConfig")

	rawJSONData := backendConfig.Spec.Raw
	Logx(ctx).WithFields(LogFields{
		"backendConfig.Name": backendConfig.Name,
		"backendConfig.UID":  backendConfig.UID,
	}).Trace("Adding backend in core.")

	backendDetails, err := c.orchestrator.AddBackend(ctx, string(rawJSONData), string(backendConfig.UID))
	if err == nil {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Backend '%v' created", backendDetails.Name),
			BackendInfo:         c.getBackendInfo(ctx, backendDetails.Name, backendDetails.BackendUUID),
			Phase:               string(tridentv1.PhaseBound),
			DeletionPolicy:      deletionPolicy,
			LastOperationStatus: OperationStatusSuccess,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Backend created.",
			corev1.EventTypeNormal); statusErr != nil {
			err = fmt.Errorf("encountered an error while updating the status: %v", statusErr)
		}
	} else if !errors.IsReconcileDeferredError(err) {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to create backend: %v", err),
			BackendInfo:         c.getBackendInfo(ctx, "", ""),
			Phase:               string(tridentv1.PhaseUnbound),
			DeletionPolicy:      deletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Failed to create backend.",
			corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf("failed to create backend: %v; Also encountered error while updating the status: %v", err,
				statusErr)
		}
	}

	return err
}

func (c *TridentCrdController) updateBackendConfig(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, deletionPolicy string,
) error {
	logFields := LogFields{
		"backendConfig.Name": backendConfig.Name,
		"backendConfig.UID":  backendConfig.UID,
		"backendName":        backendConfig.Status.BackendInfo.BackendName,
		"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
	}

	Logx(ctx).WithFields(logFields).Trace("TridentCrdController#updateBackendConfig")

	var phase tridentv1.TridentBackendConfigPhase
	var backend *tridentv1.TridentBackend
	var err error

	if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
		BackendInfo.BackendUUID); err != nil {
		Logx(ctx).WithFields(logFields).Errorf("Unable to identify if the backend exists or not; %v", err)
		err = fmt.Errorf("unable to identify if the backend exists or not; %v", err)

		phase = tridentv1.PhaseUnknown
	} else if backend == nil {
		Logx(ctx).WithFields(logFields).Errorf("Could not find backend during update.")
		err = errors.UnsupportedConfigError("could not find backend during update")

		phase = tridentv1.PhaseLost
	} else {
		Logx(ctx).WithFields(logFields).Trace("Updating backend in core.")
		rawJSONData := backendConfig.Spec.Raw

		var backendDetails *storage.BackendExternal
		backendDetails, err = c.orchestrator.UpdateBackendByBackendUUID(ctx,
			backendConfig.Status.BackendInfo.BackendName, string(rawJSONData),
			backendConfig.Status.BackendInfo.BackendUUID, string(backendConfig.UID))
		if err != nil {
			phase = tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)

			if errors.IsNotFoundError(err) {
				Logx(ctx).WithFields(LogFields{
					"backendConfig.Name": backendConfig.Name,
				}).Error("Could not find backend during update.")

				phase = tridentv1.PhaseLost
				err = fmt.Errorf("could not find backend during update; %v", err)
			}
		} else {
			newStatus := tridentv1.TridentBackendConfigStatus{
				Message:             fmt.Sprintf("Backend '%v' updated", backendDetails.Name),
				BackendInfo:         c.getBackendInfo(ctx, backendDetails.Name, backendDetails.BackendUUID),
				Phase:               string(tridentv1.PhaseBound),
				DeletionPolicy:      deletionPolicy,
				LastOperationStatus: OperationStatusSuccess,
			}

			if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Backend updated.",
				corev1.EventTypeNormal); statusErr != nil {
				return fmt.Errorf("encountered an error while updating the status: %v", statusErr)
			}
		}
	}

	if err != nil {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to apply the backend update; %v", err),
			BackendInfo:         backendConfig.Status.BackendInfo,
			Phase:               string(phase),
			DeletionPolicy:      backendConfig.Status.DeletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
			"Failed to apply the backend update.", corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf(
				"failed to update backend: %v; Also encountered error while updating the status: %v", err, statusErr)
		}
	}

	return err
}

func (c *TridentCrdController) deleteBackendConfig(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, deletionPolicy string,
) error {
	logFields := LogFields{
		"backendConfig.Name": backendConfig.Name,
		"backendName":        backendConfig.Status.BackendInfo.BackendName,
		"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		"phase":              backendConfig.Status.Phase,
		"deletionPolicy":     deletionPolicy,
		"deletionTimeStamp":  backendConfig.ObjectMeta.DeletionTimestamp,
	}

	Logx(ctx).WithFields(logFields).Trace("TridentCrdController#deleteBackendConfig")

	if backendConfig.Status.Phase == string(tridentv1.PhaseUnbound) {
		// Originally we were doing the same for the phase=lost but it does not remove configRef
		// from the in-memory object, thus making it hard to remove it using tridentctl
		Logx(ctx).Debugf("Deleting the CR with the status '%v'", backendConfig.Status.Phase)
	} else {
		Logx(ctx).WithFields(logFields).Trace("Attempting to delete backend.")

		if deletionPolicy == tridentv1.BackendDeletionPolicyDelete {

			message, phase, err := c.deleteBackendConfigUsingPolicyDelete(ctx, backendConfig, logFields)
			if err != nil {
				lastOperationStatus := OperationStatusFailed
				if phase == tridentv1.PhaseDeleting {
					lastOperationStatus = OperationStatusSuccess
				}

				newStatus := tridentv1.TridentBackendConfigStatus{
					Message:             message,
					BackendInfo:         backendConfig.Status.BackendInfo,
					Phase:               string(phase),
					DeletionPolicy:      deletionPolicy,
					LastOperationStatus: lastOperationStatus,
				}

				if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
					"Failed to delete backend.", corev1.EventTypeWarning); statusErr != nil {
					return fmt.Errorf("backend err: %v and status update err: %v", err, statusErr)
				}

				return err
			}

			Logx(ctx).Debugf("Backend '%v' deleted.", backendConfig.Status.BackendInfo.BackendName)
		} else if deletionPolicy == tridentv1.BackendDeletionPolicyRetain {

			message, phase, err := c.deleteBackendConfigUsingPolicyRetain(ctx, backendConfig, logFields)
			if err != nil {
				newStatus := tridentv1.TridentBackendConfigStatus{
					Message:             message,
					BackendInfo:         backendConfig.Status.BackendInfo,
					Phase:               string(phase),
					DeletionPolicy:      deletionPolicy,
					LastOperationStatus: OperationStatusFailed,
				}

				if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
					"Failed to remove configRef from backend.", corev1.EventTypeWarning); statusErr != nil {
					return fmt.Errorf("backend err: %v and status update err: %v", err, statusErr)
				}

				return err
			}
		}
	}

	Logx(ctx).Debugf("Removing TridentBackendConfig '%v' finalizers.", backendConfig.Name)
	return c.removeFinalizers(ctx, backendConfig, false)
}

func (c *TridentCrdController) deleteBackendConfigUsingPolicyDelete(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, logFields map[string]interface{},
) (string, tridentv1.TridentBackendConfigPhase, error) {
	var phase tridentv1.TridentBackendConfigPhase
	var backend *tridentv1.TridentBackend
	var message string
	var err error

	// Deletion Logic:
	// 1. First check if the backend exists, if not delete the TBC CR
	// 2. If it does, check if it is in a deleting state, if it is then do not re-run deletion logic in the core.
	// 3. If it is not in a deleting state, then run the deletion logic in the core.
	// 4. After running the deletion logic in the core check again if the backend still exists,
	//    if not delete the TBC CR.
	// 5. If it still exists, then fail and set `Status.phase=Deleting`.
	if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
		BackendInfo.BackendUUID); err != nil {

		message = fmt.Sprintf("Unable to identify if the backend is deleted or not; %v", err)
		phase = tridentv1.PhaseUnknown

		Logx(ctx).WithFields(logFields).Errorf(message)
		err = fmt.Errorf("unable to identify if the backend '%v' is deleted or not; %v",
			backendConfig.Status.BackendInfo.BackendName, err)
	} else if backend == nil {
		Logx(ctx).WithFields(logFields).Trace("Backend not found, proceeding with the TridentBackendConfig deletion.")

		// In the lost case ensure the backend is deleted with deletionPolicy `delete`
		if backendConfig.Status.Phase == string(tridentv1.PhaseLost) {
			Logx(ctx).WithFields(logFields).Debugf("Attempting to remove in-memory backend object.")
			if deleteErr := c.orchestrator.DeleteBackendByBackendUUID(ctx, backendConfig.Status.BackendInfo.BackendName,
				backendConfig.Status.BackendInfo.BackendUUID); deleteErr != nil {
				Logx(ctx).WithFields(logFields).Warnf("unable to delete backend: %v: %v",
					backendConfig.Status.BackendInfo.BackendName, deleteErr)
			}
		}

		// Attempt to remove configRef for all the cases because this tbc will be gone
		Logx(ctx).WithFields(logFields).Debugf("Attempting to remove configRef from in-memory backend object.")
		_ = c.orchestrator.RemoveBackendConfigRef(ctx, backendConfig.Status.BackendInfo.BackendUUID,
			string(backendConfig.UID))
	} else if backend.State == string(storage.Deleting) {
		message = "Backend is in a deleting state, cannot proceed with the TridentBackendConfig deletion. "
		phase = tridentv1.PhaseDeleting

		Logx(ctx).WithFields(logFields).Errorf(message + "Re-adding this work item back to the queue.")
		err = fmt.Errorf("backend is in a deleting state, cannot proceed with the TridentBackendConfig deletion")
	} else {
		Logx(ctx).WithFields(logFields).Trace("Backend is present and not in a deleting state, " +
			"proceeding with the backend deletion.")

		if err = c.orchestrator.DeleteBackendByBackendUUID(ctx, backendConfig.Status.BackendInfo.BackendName,
			backendConfig.Status.
				BackendInfo.BackendUUID); err != nil {

			phase = tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)
			err = fmt.Errorf("unable to delete backend '%v'; %v", backendConfig.Status.BackendInfo.BackendName, err)

			if !errors.IsNotFoundError(err) {
				message = fmt.Sprintf("Unable to delete backend; %v", err)
				Logx(ctx).WithFields(logFields).Errorf(message)
			} else {
				// In the next reconcile loop the above condition `backend == nil` should be true if backend
				// is not present in-memory as well as the tbe CR
				message = "Could not find backend during deletion."
				Logx(ctx).WithFields(logFields).Trace(message)
			}
		} else {

			// Wait 2 seconds before checking again backend is gone or not.
			time.Sleep(2 * time.Second)

			// Ensure backend does not exist
			if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
				BackendInfo.BackendUUID); err != nil {
				message = fmt.Sprintf("Unable to ensure backend deletion.; %v", err)
				phase = tridentv1.PhaseUnknown

				Logx(ctx).WithFields(logFields).Errorf("Unable to ensure backend deletion; %v", err)
				err = fmt.Errorf("unable to ensure backend '%v' deletion; %v",
					backendConfig.Status.BackendInfo.BackendName,
					err)
			} else if backend != nil {
				message = "Backend still present after a deletion attempt"
				phase = tridentv1.PhaseDeleting

				Logx(ctx).WithFields(logFields).Errorf("Backend still present after a deletion attempt. " +
					"Re-adding this work item back to the queue to try again.")
				err = fmt.Errorf("backend '%v' still present after a deletion attempt",
					backendConfig.Status.BackendInfo.BackendName)
			} else {
				Logx(ctx).WithFields(logFields).Trace("Backend deleted.")
			}
		}
	}

	return message, phase, err
}

func (c *TridentCrdController) deleteBackendConfigUsingPolicyRetain(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, logFields map[string]interface{},
) (string, tridentv1.TridentBackendConfigPhase, error) {
	var phase tridentv1.TridentBackendConfigPhase
	var backend *tridentv1.TridentBackend
	var message string
	var err error

	// Deletion Logic:
	// 1. First check if the backend exists, if not delete the TBC CR
	// 2. If it does, check if it has a configRef field set, if not delete the TBC CR
	// 3. If the configRef field is set, then run the remove configRef logic in the core.
	if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
		BackendInfo.BackendUUID); err != nil {

		message = fmt.Sprintf("Unable to identify if the backend exists or not; %v", err)
		phase = tridentv1.PhaseUnknown

		Logx(ctx).WithFields(logFields).Errorf(message)
		err = fmt.Errorf("unable to identify if the backend '%v' exists or not; %v",
			backendConfig.Status.BackendInfo.BackendName, err)
	} else if backend == nil {
		Logx(ctx).WithFields(logFields).Trace("Backend not found, " +
			"proceeding with the TridentBackendConfig deletion.")

		Logx(ctx).WithFields(logFields).Debugf("Attempting to remove configRef from in-memory backend object.")
		_ = c.orchestrator.RemoveBackendConfigRef(ctx, backendConfig.Status.BackendInfo.BackendUUID,
			string(backendConfig.UID))
	} else if backend.ConfigRef == "" {
		Logx(ctx).WithFields(logFields).Trace("Backend found " +
			"but does not contain a configRef, proceeding with the TridentBackendConfig deletion.")
	} else {
		if err = c.orchestrator.RemoveBackendConfigRef(ctx, backendConfig.Status.BackendInfo.BackendUUID,
			string(backendConfig.UID)); err != nil {

			phase = tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)
			err = fmt.Errorf("failed to remove configRef from the backend '%v'; %v",
				backendConfig.Status.BackendInfo.BackendName, err)

			if !errors.IsNotFoundError(err) {
				message = fmt.Sprintf("Failed to remove configRef from the backend; %v", err)
				Logx(ctx).WithFields(logFields).Errorf(message)
			} else {
				// In the next reconcile loop the above condition `backend == nil` should be true if backend
				// is not present in-memory as well as the tbe CR
				message = "Could not find backend during configRef removal."
				Logx(ctx).WithFields(logFields).Trace(message)
			}
		} else {
			Logx(ctx).WithFields(logFields).Trace("ConfigRef removed from the backend; backend not deleted.")
		}
	}

	return message, phase, err
}

// getBackendConfigWithBackendUUID identifies if the backend config referencing a given backendUUID exists or not
func (c *TridentCrdController) getBackendConfigWithBackendUUID(
	ctx context.Context, namespace, backendUUID string,
) (*tridentv1.TridentBackendConfig, error) {
	// Get list of all the backend configs
	backendConfigList, err := c.crdClientset.TridentV1().TridentBackendConfigs(namespace).List(ctx, listOpts)
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"backendUUID": backendUUID,
			"namespace":   namespace,
			"err":         err,
		}).Errorf("Error listing Trident backendConfig configs.")
		return nil, fmt.Errorf("error listing Trident backendConfig configs: %v", err)
	}

	for _, backendConfig := range backendConfigList.Items {
		if backendConfig.Status.BackendInfo.BackendUUID == backendUUID {
			return backendConfig, nil
		}
	}

	return nil, errors.NotFoundError("backend config not found")
}

// getBackendConfigsWithSecret identifies the backend configs referencing a given secret (if any)
func (c *TridentCrdController) getBackendConfigsWithSecret(
	ctx context.Context, namespace, secret string,
) ([]*tridentv1.TridentBackendConfig, error) {
	var backendConfigs []*tridentv1.TridentBackendConfig

	// Get list of all the backend configs
	backendConfigList, err := c.crdClientset.TridentV1().TridentBackendConfigs(namespace).List(ctx, listOpts)
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"err": err,
		}).Errorf("Error listing Trident backendConfig configs.")
		return backendConfigs, fmt.Errorf("error listing Trident backendConfig configs: %v", err)
	}

	for _, backendConfig := range backendConfigList.Items {
		if secretName, err := backendConfig.Spec.GetSecretName(); err == nil && secretName != "" {
			if secretName == secret {
				backendConfigs = append(backendConfigs, backendConfig)
			}
		}
	}

	if len(backendConfigs) > 0 {
		return backendConfigs, nil
	}

	return backendConfigs, errors.NotFoundError("no backend config with the matching secret found")
}

// updateLogAndStatus updates the event logs and status of a TridentOrchestrator CR (if required)
func (c *TridentCrdController) updateTbcEventAndStatus(
	ctx context.Context, tbcCR *tridentv1.TridentBackendConfig,
	newStatus tridentv1.TridentBackendConfigStatus, debugMessage, eventType string,
) (tbcCRNew *tridentv1.TridentBackendConfig, err error) {
	var logEvent bool

	if tbcCRNew, logEvent, err = c.updateTridentBackendConfigCRStatus(ctx, tbcCR, newStatus, debugMessage); err != nil {
		return
	}

	// Log event only when phase has been updated or a event type  warning has occurred
	if logEvent || eventType == corev1.EventTypeWarning {
		c.recorder.Event(tbcCR, eventType, newStatus.LastOperationStatus, newStatus.Message)
	}

	return
}

// updateTridentBackendConfigCRStatus updates the status of a CR if required
func (c *TridentCrdController) updateTridentBackendConfigCRStatus(
	ctx context.Context,
	tbcCR *tridentv1.TridentBackendConfig, newStatusDetails tridentv1.TridentBackendConfigStatus, debugMessage string,
) (*tridentv1.TridentBackendConfig, bool, error) {
	logFields := LogFields{"TridentBackendConfigCR": tbcCR.Name}

	// Update phase of the tbcCR
	Logx(ctx).WithFields(logFields).Trace(debugMessage)

	if reflect.DeepEqual(tbcCR.Status, newStatusDetails) {
		Logx(ctx).WithFields(logFields).Trace("New status is same as the old phase, no status update needed.")

		return tbcCR, false, nil
	}

	crClone := tbcCR.DeepCopy()
	crClone.Status = newStatusDetails

	// client-go does not provide r.Status().Patch which would have been ideal, something to do if we switch to using
	// controller-runtime.
	newTbcCR, err := c.crdClientset.TridentV1().TridentBackendConfigs(tbcCR.Namespace).UpdateStatus(
		ctx, crClone, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(logFields).Errorf("Could not update status of the CR; %v", err)
		// If this is due to CR deletion, ensure it is handled properly for the CRs moving from unbound to bound phase.
		if newTbcCR = c.checkAndHandleNewlyBoundCRDeletion(ctx, tbcCR, newStatusDetails); newTbcCR != nil {
			err = nil
		}
	}

	return newTbcCR, true, err
}

// checkAndHandleNewlyBoundCRDeletion handles a corner cases where tbc is delete immediately after creation
// which may result in tbc-only deletion in next reconcile without the tbe or in-memory backend deletion
func (c *TridentCrdController) checkAndHandleNewlyBoundCRDeletion(
	ctx context.Context,
	tbcCR *tridentv1.TridentBackendConfig, newStatusDetails tridentv1.TridentBackendConfigStatus,
) *tridentv1.TridentBackendConfig {
	logFields := LogFields{"TridentBackendConfigCR": tbcCR.Name}
	var newTbcCR *tridentv1.TridentBackendConfig

	// Need to handle a scenario where a new TridentBackendConfig gets deleted immediately after creation.
	// When new tbc is created it results in a new tbe creation or binding but tbc however runs into a
	// failure to tbc CR deletion.
	// In this scenario the subsequent reconcile needs to have the knowledge of the backend name,
	// backendUUID to ensure proper deletion of tbe and in-memory backend.
	if tbcCR.Status.Phase == string(tridentv1.PhaseUnbound) && newStatusDetails.Phase == string(tridentv1.
		PhaseBound) {
		// Get the updated copy of the tbc CR and check if it is deleting
		updatedTbcCR, err := c.backendConfigsLister.TridentBackendConfigs(tbcCR.Namespace).Get(tbcCR.Name)
		if err != nil {
			Logx(ctx).WithFields(logFields).Errorf("encountered an error while getting the latest CR update: %v", err)
			return nil
		}

		var updateStatus bool
		if !updatedTbcCR.ObjectMeta.DeletionTimestamp.IsZero() {
			// tbc CR has been marked for deletion
			Logx(ctx).WithFields(logFields).Debugf("This CR is deleting, re-attempting to update the status.")
			updateStatus = true
		} else {
			Logx(ctx).WithFields(logFields).Debugf("CR is not deleting.")
		}

		if updateStatus {
			crClone := updatedTbcCR.DeepCopy()
			crClone.Status = newStatusDetails
			newTbcCR, err = c.crdClientset.TridentV1().TridentBackendConfigs(tbcCR.Namespace).UpdateStatus(
				ctx, crClone, updateOpts)
			if err != nil {
				Logx(ctx).WithFields(logFields).Errorf("another attempt to update the status failed: %v; "+
					"backend (name: %v, backendUUID: %v) requires manual intervention", err,
					newStatusDetails.BackendInfo.BackendName, newStatusDetails.BackendInfo.BackendUUID)
			} else {
				Logx(ctx).WithFields(logFields).Debugf("Status updated.")
			}
		}
	}

	return newTbcCR
}

// updateTridentBackendConfigCR updates the TridentBackendConfigCR
func (c *TridentCrdController) updateTridentBackendConfigCR(
	ctx context.Context, tbcCR *tridentv1.TridentBackendConfig,
) (*tridentv1.TridentBackendConfig, error) {
	logFields := LogFields{"TridentBackendConfigCR": tbcCR.Name}

	// Update phase of the tbcCR
	Logx(ctx).WithFields(logFields).Trace("Updating the TridentBackendConfig CR")

	newTbcCR, err := c.crdClientset.TridentV1().TridentBackendConfigs(tbcCR.Namespace).Update(ctx, tbcCR, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(logFields).Errorf("could not update TridentBackendConfig CR; %v", err)
	}

	return newTbcCR, err
}

// removeBackendConfigFinalizers removes TridentConfig's finalizers from TridentBackendconfig CRs
func (c *TridentCrdController) removeBackendConfigFinalizers(
	ctx context.Context, tbc *tridentv1.TridentBackendConfig,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"tbc.ResourceVersion":              tbc.ResourceVersion,
		"tbc.ObjectMeta.DeletionTimestamp": tbc.ObjectMeta.DeletionTimestamp,
	}).Trace("removeBackendConfigFinalizers")

	if tbc.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		backendConfigCopy := tbc.DeepCopy()
		backendConfigCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentBackendConfigs(tbc.Namespace).Update(ctx, backendConfigCopy,
			updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// getBackendInfo creates TridentBackendConfigBackendInfo object based on the provided information
func (c *TridentCrdController) getBackendInfo(
	_ context.Context, backendName, backendUUID string,
) tridentv1.TridentBackendConfigBackendInfo {
	return tridentv1.TridentBackendConfigBackendInfo{
		BackendName: backendName,
		BackendUUID: backendUUID,
	}
}
