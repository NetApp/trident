// Copyright 2023 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"

	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

func (c *TridentCrdController) updateTridentBackendHandler(old, new interface{}) {
	// When a CR has a finalizer those come as update events and not deletes,
	// Do not handle any other update events other than backend deletion
	// otherwise it may result in continuous reconcile loops.
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	if err := c.removeFinalizers(ctx, new, false); err != nil {
		Logx(ctx).WithError(err).Error("Error removing finalizers")
	}
}

// deleteTridentBackendHandler takes a TridentBackend resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than TridentBackend.
func (c *TridentCrdController) deleteTridentBackendHandler(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

	Logx(ctx).Trace("TridentCrdController#deleteTridentBackendHandler")

	var key, namespace string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, _, err = cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid backend key.")
		return
	}

	// Get the BackendInfo.BackendUUID of the object and set it as the key
	backendUUID := obj.(*tridentv1.TridentBackend).BackendUUID

	// Later we are going to rely on the backendUUID to find an associated backend config
	// Relying on backendUUID will allow easy identification of the backend config, name won't
	// as it is of type `tbe-xyz` format, and not stored in the config.
	newKey := namespace + "/" + backendUUID

	keyItem := KeyItem{
		key:        newKey,
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	c.workqueue.Add(keyItem)
}

func (c *TridentCrdController) handleTridentBackend(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(LogFields{
		"Key":        key,
		"eventType":  eventType,
		"objectType": objectType,
	}).Trace("TridentCrdController#handleTridentBackend")

	if eventType != EventDelete {
		Logx(ctx).Error("Wrong backend event triggered a handleTridentBackend.")
		return nil
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid key.")
		return nil
	}

	// Get the backend config that matches the backendUUID, i.e. the key
	backendConfig, err := c.getBackendConfigWithBackendUUID(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logx(ctx).Warnf("No backend config is associated with the backendUUID '%v'.", name)
		} else {
			Logx(ctx).Errorf("unable to identify a backend config associated with the backendUUID '%v'.", name)
		}

		return nil
	}

	var newEventType EventType
	if backendConfig.Status.Phase == string(tridentv1.PhaseUnbound) {
		// Ideally this should not be the case
		Logx(ctx).Tracef("Backend Config '%v' has %v phase, nothing to do", backendConfig.Name,
			tridentv1.PhaseUnbound)
		return nil
	} else if backendConfig.Status.Phase == string(tridentv1.PhaseLost) {
		// Nothing to do here
		Logx(ctx).Tracef("Backend Config '%v' is already in a %v phase, nothing to do", backendConfig.Name,
			tridentv1.PhaseLost)
		return nil
	} else if backendConfig.Status.Phase == string(tridentv1.PhaseDeleting) {
		Logx(ctx).Tracef("Backend Config '%v' is already in a deleting phase, ensuring its deletion",
			backendConfig.Name)
		newEventType = EventDelete
	} else {
		Logx(ctx).Tracef("Backend Config '%v' has %v phase, running an update to set it to %v",
			backendConfig.Name, backendConfig.Status.Phase, tridentv1.PhaseLost)
		newEventType = EventForceUpdate
	}

	newKey := backendConfig.Namespace + "/" + backendConfig.Name
	newCtx := context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	newKeyItem := KeyItem{
		key:        newKey,
		event:      newEventType,
		ctx:        newCtx,
		objectType: ObjectTypeTridentBackendConfig,
	}
	c.workqueue.Add(newKeyItem)

	return nil
}
