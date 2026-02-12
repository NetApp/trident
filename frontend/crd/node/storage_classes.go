// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"fmt"

	. "github.com/netapp/trident/logging"
)

// handleStorageClasses handles the business logic for StorageClass events
func (c *TridentNodeCrdController) handleStorageClasses(keyItem *KeyItem) (returnErr error) {
	key := keyItem.key
	ctx := keyItem.ctx

	// Recover from panics (e.g., type assertion failures in the lister)
	defer func() {
		if r := recover(); r != nil {
			returnErr = fmt.Errorf("unexpected object type or panic in handleStorageClasses: %v", r)
		}
	}()

	// StorageClass is cluster-scoped, so the key is just the name (no namespace prefix)
	name := key

	Logc(ctx).WithFields(LogFields{
		"StorageClass": name,
		"Event":        keyItem.event,
	}).Debug("Processing StorageClass event")

	// Get the StorageClass object using the lister
	sc, err := c.storageClassLister.Get(name)
	if err != nil {
		if keyItem.event == EventDelete {
			// For delete events, we still want to notify scheduler even if object is gone
			c.autogrowOrchestrator.HandleStorageClassEvent(ctx, EventDelete, name)

			Logc(ctx).WithFields(LogFields{
				"StorageClass": name,
				"Event":        keyItem.event,
			}).Debug("StorageClass already deleted")
			return nil
		}
		Logc(ctx).WithFields(LogFields{
			"StorageClass": name,
			"Event":        keyItem.event,
		}).WithError(err).Error("Failed to get StorageClass")
		return err
	}

	// Let autogrow controller handle this event (publishes to ControllerEventBus)
	c.autogrowOrchestrator.HandleStorageClassEvent(ctx, keyItem.event, name)

	Logc(ctx).WithFields(LogFields{
		"StorageClass": name,
		"Provisioner":  sc.Provisioner,
		"Event":        keyItem.event,
	}).Info("StorageClass event processed")

	return nil
}
