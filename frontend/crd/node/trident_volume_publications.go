// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
)

// handleTridentVolumePublications handles the business logic for TridentVolumePublication events
func (c *TridentNodeCrdController) handleTridentVolumePublications(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key")
		return err
	}

	if namespace == "" {
		Logc(ctx).WithField("key", key).Error("`Invalid key; no namespace present.`")
		return errors.NewBadRequest(fmt.Sprintf("invalid key, no namespace present: %v", key))
	}

	// Get the TridentVolumePublication object to validate nodeID
	tvp, err := c.tridentVolumePublicationLister.TridentVolumePublications(namespace).Get(name)
	if err != nil {
		if keyItem.event == EventDelete {
			// For delete events, we still want to notify scheduler even if object is gone
			// Send just the name (volumeID.nodeID), not the full key
			c.autogrowOrchestrator.HandleTVPEvent(ctx, EventDelete, name)

			Logc(ctx).WithFields(LogFields{
				"TridentVolumePublication": name,
				"Namespace":                namespace,
				"Event":                    keyItem.event,
			}).Debug("TridentVolumePublication already deleted")
			return nil
		}
		Logc(ctx).WithFields(LogFields{
			"TridentVolumePublication": name,
			"Namespace":                namespace,
			"Event":                    keyItem.event,
		}).WithError(err).Error("Failed to get TridentVolumePublication")
		return err
	}

	// Validate that this TridentVolumePublication belongs to this node
	// With label selector, this should never happen, but adding as safety check
	if tvp.NodeID != c.nodeName {
		Logc(ctx).WithFields(LogFields{
			"TridentVolumePublication": name,
			"nodeID":                   tvp.NodeID,
			"currentNode":              c.nodeName,
			"Event":                    keyItem.event,
		}).Warn("Label selector failed: received TridentVolumePublication for different node - this indicates label selector is not working properly")
		// This shouldn't happen with proper label selector, but handle gracefully
		return nil
	}

	fields := LogFields{
		"TridentVolumePublication": name,
		"Namespace":                namespace,
		"nodeID":                   tvp.NodeID,
		"volumeID":                 tvp.VolumeID,
		"Event":                    keyItem.event,
	}
	Logc(ctx).WithFields(fields).Debug("Processing TridentVolumePublication for this node")

	// Let autogrow controller handle this event (publishes to ControllerEventBus)
	// Send not just the name (volumeID.nodeID), but the full key
	c.autogrowOrchestrator.HandleTVPEvent(ctx, keyItem.event, key)

	Logc(ctx).WithFields(fields).Info("Successfully processed TridentVolumePublication event")

	return nil
}
