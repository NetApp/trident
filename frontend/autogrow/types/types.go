package types

import (
	"context"
	"time"

	crdtypes "github.com/netapp/trident/frontend/crd/types"
)

// VolumesScheduled is published by the Scheduler (assorter) layer periodically
// to trigger volume polling.
type VolumesScheduled struct {
	// Ctx is the context associated with this event
	Ctx context.Context

	// ID is a unique identifier for this event (for deduplication)
	ID uint64

	// TVPNames is the list of TridentVolumePublication names to poll
	// Format: volumeID.nodeID (e.g., pvc-123.node1)
	TVPNames []string

	// Timestamp when the event was generated
	Timestamp time.Time
}

// VolumeThresholdBreached is published by the Poller layer when a volume
// crosses its usage threshold and needs to be grown.
type VolumeThresholdBreached struct {
	// Ctx is the context associated with this event
	Ctx context.Context

	// ID is a unique identifier for this event (for deduplication)
	ID uint64

	// TVPName is the TridentVolumePublication name (volumeID.nodeID format)
	TVPName string

	// TAGPName is the name of the TridentAutogrowPolicy against which the threshold was breached
	TAGPName string

	// CurrentTotalSize is the current volume size in bytes
	CurrentTotalSize int64

	// UsedSize is the current used space in bytes
	UsedSize int64

	// UsedPercentage is the current used percentage of the volume
	UsedPercentage float32

	// Timestamp when the threshold was breached
	Timestamp time.Time
}

// ControllerEvent represents an event from the controller layer
// This encapsulates StorageClass, TVP, and TBE events
// Uses crdtypes.EventType and crdtypes.ObjectType directly for type safety
type ControllerEvent struct {
	// Ctx is the context associated with this event
	Ctx context.Context

	// ID is a unique identifier for this event (for deduplication)
	ID uint64

	// ObjectType indicates the type of resource (StorageClass, TridentVolumePublication, TridentBackend)
	// Uses crdtypes.ObjectType for type safety
	ObjectType crdtypes.ObjectType

	// EventType indicates what happened (add, update, delete)
	// Uses crdtypes.EventType for type safety
	EventType crdtypes.EventType

	// Key is the resource identifier
	// For namespaced resources (TVP, TBE): "namespace/name" format
	// For cluster-scoped resources (StorageClass): just the name
	Key string

	// Timestamp when the event occurred
	Timestamp time.Time
}
