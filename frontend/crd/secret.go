// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
)

// updateSecretHandler takes a Kubernetes secret resource and converts it
// into a namespace/name string which is then put onto the work queue.
// This method should *not* be passed resources of any type other than Secrets.
func (c *TridentCrdController) updateSecretHandler(old, new interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Trace("TridentCrdController#updateSecretHandler")

	newSecret := new.(*corev1.Secret)
	oldSecret := old.(*corev1.Secret)

	// Ignore metadata and status only updates
	if oldSecret != nil && newSecret != nil {
		if newSecret.GetGeneration() == oldSecret.GetGeneration() && newSecret.GetGeneration() != 0 {
			Logx(ctx).Trace("No change in the generation, nothing to do.")
			return
		}
	}

	if oldSecret != nil && newSecret != nil {
		if newSecret.ResourceVersion == oldSecret.ResourceVersion {
			Logx(ctx).Trace("No change in the resource version, nothing to do.")
			return
		}
	}

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
		objectType: ObjectTypeSecret,
	}

	c.workqueue.Add(keyItem)
}

func (c *TridentCrdController) handleSecret(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(LogFields{
		"eventType":  eventType,
		"objectType": objectType,
	}).Trace("TridentCrdController#handleSecret")

	if eventType != EventUpdate {
		Logx(ctx).Error("Wrong secret event triggered a handleSecret.")
		return nil
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, secretName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).Error("Invalid key.")
		return nil
	}

	// Get the backend configs that matches the secret namespace and secretName, i.e. the key
	backendConfigs, err := c.getBackendConfigsWithSecret(ctx, namespace, secretName)
	if err != nil {
		if utils.IsNotFoundError(err) {
			Logx(ctx).Warnf("No backend config is associated with the secret update")
		} else {
			Logx(ctx).Errorf("unable to identify a backend config associated with the secret update")
		}
	}

	// Add these backend configs back to the queue and run update on these backends
	for _, backendConfig := range backendConfigs {
		Logx(ctx).WithFields(LogFields{
			"backendConfig.Name":      backendConfig.Name,
			"backendConfig.Namespace": backendConfig.Namespace,
		}).Info("Running update on backend due to secret update.")

		newKey := backendConfig.Namespace + "/" + backendConfig.Name
		newCtx := context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

		keyItem := KeyItem{
			key:        newKey,
			event:      EventUpdate,
			ctx:        newCtx,
			objectType: ObjectTypeTridentBackendConfig,
		}
		c.workqueue.Add(keyItem)
	}
	return nil
}
