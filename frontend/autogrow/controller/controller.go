// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/netapp/trident/frontend/autogrow/cache"
	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/eventbus"
)

// NewController creates a new Controller layer
func NewController(ctx context.Context, cache *cache.AutogrowCache) (*Controller, error) {
	Logc(ctx).Debug("Creating Controller layer")

	c := &Controller{
		cache: cache,
	}

	Logc(ctx).Info("Controller layer created successfully")
	return c, nil
}

// Activate starts the controller layer
// Currently a no-op as the controller is stateless
func (c *Controller) Activate(ctx context.Context) error {
	Logc(ctx).Debug("Controller layer activate (no-op)")
	// No-op: Controller is stateless
	// Future: Could initialize metrics, event buffers, rate limiters, etc.
	return nil
}

// Deactivate stops the controller layer
// Currently a no-op as the controller is stateless
func (c *Controller) Deactivate(ctx context.Context) error {
	Logc(ctx).Debug("Controller layer deactivate (no-op)")
	// No-op: Controller is stateless
	// Future: Could flush buffers, stop metrics collection, etc.
	return nil
}

// HandleStorageClassEvent handles a StorageClass event for autogrow
// This publishes the event to the ControllerEventBus for consumption by the Scheduler
func (c *Controller) HandleStorageClassEvent(ctx context.Context, eventType crdtypes.EventType, name string) {
	Logc(ctx).WithFields(LogFields{
		"eventType": eventType,
		"name":      name,
	}).Debug(">>>> autogrow.controller.HandleStorageClassEvent")
	defer Logc(ctx).WithFields(LogFields{
		"eventType": eventType,
		"name":      name,
	}).Debug("<<<< autogrow.controller.HandleStorageClassEvent")

	timestamp := time.Now()
	controllerEvent := autogrowTypes.ControllerEvent{
		Ctx:        ctx,
		ID:         generateControllerEventID(timestamp, string(crdtypes.ObjectTypeStorageClass), string(eventType), name),
		ObjectType: crdtypes.ObjectTypeStorageClass,
		EventType:  eventType,
		Key:        name,
		Timestamp:  timestamp,
	}

	if eventbus.ControllerEventBus != nil {
		eventbus.ControllerEventBus.Publish(ctx, controllerEvent)
		Logc(ctx).WithFields(LogFields{
			"objectType": crdtypes.ObjectTypeStorageClass,
			"eventType":  eventType,
			"key":        name,
		}).Debug("Published StorageClass event to ControllerEventBus")
	} else {
		Logc(ctx).Warn("ControllerEventBus not initialized, skipping StorageClass event publish")
	}
}

// HandleTVPEvent handles a TridentVolumePublication event for autogrow
// This publishes the event to the ControllerEventBus for consumption by the Scheduler
func (c *Controller) HandleTVPEvent(ctx context.Context, eventType crdtypes.EventType, key string) {
	Logc(ctx).WithFields(LogFields{
		"eventType": eventType,
		"key":       key,
	}).Debug(">>>> autogrow.controller.HandleTVPEvent")
	defer Logc(ctx).WithFields(LogFields{
		"eventType": eventType,
		"key":       key,
	}).Debug("<<<< autogrow.controller.HandleTVPEvent")

	timestamp := time.Now()
	controllerEvent := autogrowTypes.ControllerEvent{
		Ctx:        ctx,
		ID:         generateControllerEventID(timestamp, string(crdtypes.ObjectTypeTridentVolumePublication), string(eventType), key),
		ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
		EventType:  eventType,
		Key:        key,
		Timestamp:  timestamp,
	}

	if eventbus.ControllerEventBus != nil {
		eventbus.ControllerEventBus.Publish(ctx, controllerEvent)
		Logc(ctx).WithFields(LogFields{
			"objectType": crdtypes.ObjectTypeTridentVolumePublication,
			"eventType":  eventType,
			"key":        key,
		}).Debug("Published TVP event to ControllerEventBus")
	} else {
		Logc(ctx).Warn("ControllerEventBus not initialized, skipping TVP event publish")
	}
}

// HandleTBEEvent handles a TridentBackend event for autogrow
// This publishes the event to the ControllerEventBus for consumption by the Scheduler
func (c *Controller) HandleTBEEvent(ctx context.Context, eventType crdtypes.EventType, key string) {
	Logc(ctx).WithFields(LogFields{
		"eventType": eventType,
		"key":       key,
	}).Debug(">>>> autogrow.controller.HandleTBEEvent")
	defer Logc(ctx).WithFields(LogFields{
		"eventType": eventType,
		"key":       key,
	}).Debug("<<<< autogrow.controller.HandleTBEEvent")

	timestamp := time.Now()
	controllerEvent := autogrowTypes.ControllerEvent{
		Ctx:        ctx,
		ID:         generateControllerEventID(timestamp, string(crdtypes.ObjectTypeTridentBackend), string(eventType), key),
		ObjectType: crdtypes.ObjectTypeTridentBackend,
		EventType:  eventType,
		Key:        key,
		Timestamp:  timestamp,
	}

	if eventbus.ControllerEventBus != nil {
		eventbus.ControllerEventBus.Publish(ctx, controllerEvent)
		Logc(ctx).WithFields(LogFields{
			"objectType": crdtypes.ObjectTypeTridentBackend,
			"eventType":  eventType,
			"key":        key,
		}).Debug("Published TBE event to ControllerEventBus")
	} else {
		Logc(ctx).Warn("ControllerEventBus not initialized, skipping TBE event publish")
	}
}

// generateControllerEventID creates a unique event ID for controller events
func generateControllerEventID(timestamp time.Time, objectType, eventType, key string) uint64 {
	// Create composite string: timestamp + objectType + eventType + key
	composite := fmt.Sprintf("%d:%s:%s:%s", timestamp.UnixNano(), objectType, eventType, key)

	// Hash the composite string using SHA256
	hash := sha256.Sum256([]byte(composite))

	// Convert first 8 bytes of hash to uint64
	return binary.BigEndian.Uint64(hash[:8])
}
