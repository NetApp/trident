// Copyright 2026 NetApp, Inc. All Rights Reserved.

//go:generate mockgen -destination=../../../../../mocks/mock_frontend/mock_autogrow/mock_scheduler/mock_assorter/mock_assorter.go github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types Assorter

package types

import (
	"context"
	"time"
)

// PublishFunc is the callback function signature for publishing scheduled volumes.
// The assorter calls this function when it has volumes ready to be polled.
// The scheduler provides this function, which typically publishes to the VolumesScheduled EventBus.
//
// The assorter generates the context (with source="assorter") and passes it to publishFunc.
//
// IMPORTANT CONTRACT: The volumeNames slice is owned by the assorter and will be reused.
// The callback MUST NOT store or retain a reference to this slice.
// If the callback needs to keep the data, it MUST make a copy.
// The callback must complete synchronously before returning.
type PublishFunc func(ctx context.Context, volumeNames []string) error

// Assorter is the interface that all assorter implementations must satisfy.
// An assorter is responsible for scheduling volumes for polling by publishing
// VolumesScheduled events to the eventbus.
//
// Different implementations can use different scheduling strategies:
// - Periodic: Fixed interval for all volumes
// - PriorityQueue: Adaptive scheduling based on usage patterns
// - Weighted: Different intervals based on policy priority
type Assorter interface {
	// Activate prepares the assorter (creates ticker, channels) but does not start the loop
	// This allows all layers to be activated before starting the scheduling loop
	Activate(ctx context.Context) error

	// StartScheduleLoop starts the periodic scheduling loop
	// This should be called after all layers are activated
	StartScheduleLoop(ctx context.Context) error

	// Deactivate stops the assorter gracefully
	Deactivate(ctx context.Context) error

	// AddVolume adds a volume to the scheduling list
	AddVolume(ctx context.Context, volumeName string) error

	// RemoveVolume removes a volume from the scheduling list
	RemoveVolume(ctx context.Context, volumeName string) error

	// UpdateVolumePeriodicity changes the polling frequency for a volume
	// (may be a no-op for simple periodic assorters)
	UpdateVolumePeriodicity(ctx context.Context, volumeName string, period time.Duration) error

	// IsRunning returns true if the assorter is currently active
	IsRunning() bool

	// GetVolumeCount returns the number of volumes currently being scheduled
	GetVolumeCount() int
}

// Config is the interface that all assorter configuration types must implement.
// This allows the generic factory to work with different assorter types.
type Config interface {
	// AssorterConfig is a marker method to identify assorter configs
	AssorterConfig()

	// Copy returns a deep copy of the configuration
	Copy() Config
}
