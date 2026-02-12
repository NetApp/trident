// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
)

// handleTridentBackends handles the business logic for TridentBackend events
func (c *TridentNodeCrdController) handleTridentBackends(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key")
		return err
	}

	if namespace == "" {
		Logc(ctx).WithField("key", key).Error("Invalid key; no namespace present.")
		return errors.NewBadRequest(fmt.Sprintf("invalid key, no namespace present: %v", key))
	}

	Logc(ctx).WithFields(LogFields{
		"TridentBackend": name,
		"Namespace":      namespace,
		"Event":          keyItem.event,
	}).Debug("Processing TridentBackend event")

	// Get the TridentBackend object
	backend, err := c.tridentBackendLister.TridentBackends(namespace).Get(name)
	if err != nil {
		if keyItem.event == EventDelete {
			// For delete events, we still want to notify autogrow even if object is gone
			// Send the full key (namespace/name)
			c.autogrowOrchestrator.HandleTBEEvent(ctx, EventDelete, key)

			Logc(ctx).WithFields(LogFields{
				"TridentBackend": name,
				"Namespace":      namespace,
				"Event":          keyItem.event,
			}).Debug("TridentBackend already deleted")
			return nil
		}
		Logc(ctx).WithFields(LogFields{
			"TridentBackend": name,
			"Namespace":      namespace,
			"Event":          keyItem.event,
		}).WithError(err).Error("Failed to get TridentBackend")
		return err
	}

	fields := LogFields{
		"TridentBackend": name,
		"Namespace":      namespace,
		"BackendUUID":    backend.BackendUUID,
		"Event":          keyItem.event,
	}
	Logc(ctx).WithFields(fields).Debug("Processing TridentBackend event")

	// Let autogrow controller handle this event (publishes to ControllerEventBus)
	// Send the full key (namespace/name) so scheduler has namespace info
	c.autogrowOrchestrator.HandleTBEEvent(ctx, keyItem.event, key)

	Logc(ctx).WithFields(fields).Info("Successfully processed TridentBackend event")

	return nil
}
