// Copyright 2026 NetApp, Inc. All Rights Reserved.

package periodic

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/netapp/trident/logging"
)

// State represents the operational state of the Assorter
type State int32

const (
	StateStopped  State = iota // Assorter is stopped
	StateStarting              // Assorter is starting up (activated but loop not started)
	StateReady                 // Assorter is ready (activated, can accept volumes)
	StateRunning               // Assorter is running (loop started)
	StateStopping              // Assorter is shutting down
)

// get returns the current state (thread-safe atomic read)
func (s *State) get() State {
	return State(atomic.LoadInt32((*int32)(s)))
}

// set sets the state (thread-safe atomic write)
func (s *State) set(newState State) {
	atomic.StoreInt32((*int32)(s), int32(newState))
}

// compareAndSwap atomically compares and swaps the state if it matches the expected value
// Returns true if the swap was successful (thread-safe atomic CAS)
func (s *State) compareAndSwap(oldState, newState State) bool {
	return atomic.CompareAndSwapInt32((*int32)(s), int32(oldState), int32(newState))
}

// String returns the string representation of State
func (s State) String() string {
	switch s {
	case StateStopped:
		return "Stopped"
	case StateStarting:
		return "Starting"
	case StateReady:
		return "Ready"
	case StateRunning:
		return "Running"
	case StateStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

// Assorter is a simple periodic assorter that publishes all volumes
// at a fixed interval. This is the default implementation.
//
// Future implementations could include:
// - PriorityQueueAssorter: Adaptive polling based on usage trends
// - WeightedAssorter: Different intervals for different policy priorities
type Assorter struct {
	// State management
	state   State
	stateMu sync.RWMutex

	// Volume list management
	volumesMu sync.RWMutex
	volumes   map[string]struct{} // volumeName to be scheduled

	// Configuration (immutable copy) - contains period and publishFunc
	config *Config

	// Ticker for periodic scheduling
	ticker *time.Ticker
	stopCh chan struct{}
}

// NewAssorter creates a new periodic assorter.
func NewAssorter(ctx context.Context, cfg *Config) (*Assorter, error) {
	Logc(ctx).Debug(">>>> NewAssorter")
	defer Logc(ctx).Debug("<<<< NewAssorter")

	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Make a copy of the config to prevent external mutations
	configCopy := cfg.Copy().(*Config)

	// Validate required fields
	if configCopy.publishFunc == nil {
		return nil, fmt.Errorf("PublishFunc is required in assorter config")
	}

	period := configCopy.period
	if period <= 0 {
		period = DefaultPeriod
		Logc(ctx).WithField("period", period).Debug("Using default assorter period")
		// Update the copy with the default
		configCopy.period = period
	}

	a := &Assorter{
		state:   StateStopped,
		volumes: make(map[string]struct{}),
		config:  configCopy,
	}

	Logc(ctx).WithField("period", period).Info("Periodic assorter created")
	return a, nil
}

// Activate prepares the assorter (creates ticker and channels) but does not start the loop
// This is context-aware and will cleanup on cancellation
// Returns error if already running/ready, if activation fails, or if context is cancelled
func (a *Assorter) Activate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> assorter.Activate")
	defer Logc(ctx).Debug("<<<< assorter.Activate")

	// Check if we can transition to Starting state
	if !a.state.compareAndSwap(StateStopped, StateStarting) {
		currentState := a.state.get()
		if currentState == StateReady || currentState == StateRunning {
			Logc(ctx).Debug("Assorter already activated, skipping")
			return nil // Idempotent
		}
		return fmt.Errorf("cannot activate assorter: current state is %s", currentState)
	}

	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	Logc(ctx).WithField("period", a.config.period).Info("Activating Assorter...")

	// Create stop channel and ticker with jittered period to avoid thundering herd
	a.stopCh = make(chan struct{})
	a.ticker = time.NewTicker(a.getJitteredPeriod(ctx))

	// Transition to Ready state (activated but loop not started yet)
	a.state.set(StateReady)

	Logc(ctx).WithField("period", a.config.period).Info("Assorter activated successfully (ready for schedule loop)")
	return nil
}

// StartScheduleLoop starts the periodic scheduling loop
// This should be called after all layers are activated
// Can only be called when assorter is in Ready state
func (a *Assorter) StartScheduleLoop(ctx context.Context) error {
	Logc(ctx).Debug(">>>> assorter.StartScheduleLoop")
	defer Logc(ctx).Debug("<<<< assorter.StartScheduleLoop")

	// Check if we can transition from Ready to Running
	if !a.state.compareAndSwap(StateReady, StateRunning) {
		currentState := a.state.get()
		if currentState == StateRunning {
			Logc(ctx).Debug("Schedule loop already running, skipping")
			return nil // Idempotent
		}
		return fmt.Errorf("cannot start schedule loop: current state is %s (must be Ready)", currentState)
	}

	Logc(ctx).WithField("period", a.config.period).Info("Starting assorter schedule loop...")

	// Start periodic scheduling loop
	go a.scheduleLoop(ctx)

	Logc(ctx).WithField("period", a.config.period).Info("Assorter schedule loop started successfully")
	return nil
}

// Deactivate stops the assorter (works from both Ready and Running states)
func (a *Assorter) Deactivate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> assorter.Deactivate")
	defer Logc(ctx).Debug("<<<< assorter.Deactivate")

	// Try to transition from Running state
	if a.state.compareAndSwap(StateRunning, StateStopping) {
		// Was running, proceed with deactivation
	} else if a.state.compareAndSwap(StateReady, StateStopping) {
		// Was ready (activated but loop not started), proceed with deactivation
	} else {
		// Neither Running nor Ready
		currentState := a.state.get()
		if currentState == StateStopped {
			Logc(ctx).Debug("Assorter already stopped, skipping deactivation")
			return nil // Idempotent
		}
		return fmt.Errorf("cannot deactivate assorter: current state is %s", currentState)
	}

	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	Logc(ctx).Info("Deactivating Assorter...")

	// Stop ticker
	if a.ticker != nil {
		a.ticker.Stop()
	}

	// Stop schedule loop (safe even if loop never started)
	if a.stopCh != nil {
		close(a.stopCh)
	}

	// Transition to Stopped state
	a.state.set(StateStopped)

	Logc(ctx).Info("Assorter deactivated successfully")
	return nil
}

// AddVolume adds a volume to the scheduling list
// Can be called when assorter is Ready (during scheduler bootstrap) or Running
func (a *Assorter) AddVolume(ctx context.Context, volumeName string) error {
	Logc(ctx).WithField("volumeName", volumeName).Debug("Adding volume to assorter")

	// Check if assorter is ready or running (allow adding during bootstrap or normal operation)
	if !a.IsReady() && !a.IsRunning() {
		currentState := a.state.get()
		Logc(ctx).WithFields(LogFields{
			"volumeName":   volumeName,
			"currentState": currentState.String(),
		}).Warn("Cannot add volume: assorter not ready or running")
		return fmt.Errorf("cannot add volume: assorter is in %s state (must be Ready or Running)", currentState.String())
	}

	a.volumesMu.Lock()
	defer a.volumesMu.Unlock()

	// Check if volume already exists
	if _, exists := a.volumes[volumeName]; exists {
		Logc(ctx).WithField("volumeName", volumeName).Debug("Volume already exists in assorter")
		return fmt.Errorf("volume %q already exists in assorter", volumeName)
	}

	// Add volume to the list
	a.volumes[volumeName] = struct{}{}

	Logc(ctx).WithFields(LogFields{
		"volumeName":   volumeName,
		"totalVolumes": len(a.volumes),
	}).Debug("Volume added to assorter")

	return nil
}

// RemoveVolume removes a volume from the scheduling list
// Can be called when assorter is Ready or Running
func (a *Assorter) RemoveVolume(ctx context.Context, volumeName string) error {
	Logc(ctx).WithField("volumeName", volumeName).Debug("Removing volume from assorter")

	// Check if assorter is in a valid state to remove volumes
	// Allow removal during Ready (bootstrap cleanup) or Running (normal operation)
	if !a.IsReady() && !a.IsRunning() {
		currentState := a.state.get()
		Logc(ctx).WithFields(LogFields{
			"volumeName":   volumeName,
			"currentState": currentState.String(),
		}).Warn("Cannot remove volume: assorter not ready, running, or stopping")
		return fmt.Errorf("cannot remove volume: assorter is in %s state (must be Ready or Running)", currentState.String())
	}

	a.volumesMu.Lock()
	defer a.volumesMu.Unlock()

	// Check if volume exists
	if _, exists := a.volumes[volumeName]; !exists {
		Logc(ctx).WithField("volumeName", volumeName).Debug("Volume does not exist in assorter")
		return fmt.Errorf("volume %q does not exist in assorter", volumeName)
	}

	// Remove volume from the list
	delete(a.volumes, volumeName)

	Logc(ctx).WithFields(LogFields{
		"volumeName":   volumeName,
		"totalVolumes": len(a.volumes),
	}).Debug("Volume removed from assorter")

	return nil
}

// UpdateVolumePeriodicity changes the polling periodicity for a volume
// This is a placeholder for future priority queue-based assorters
// For the periodic assorter, this is a no-op since all volumes use the same period
// Can be called when assorter is Ready or Running
func (a *Assorter) UpdateVolumePeriodicity(
	ctx context.Context,
	volumeName string,
	period time.Duration,
) error {
	Logc(ctx).WithFields(LogFields{
		"volumeName": volumeName,
		"period":     period,
	}).Debug("UpdateVolumePeriodicity called (no-op for periodic assorter)")

	// Check if assorter is ready or running
	if !a.IsReady() && !a.IsRunning() {
		currentState := a.state.get()
		Logc(ctx).WithFields(LogFields{
			"volumeName":   volumeName,
			"currentState": currentState.String(),
		}).Warn("Cannot update volume periodicity: assorter not ready or running")
		return fmt.Errorf("cannot update volume periodicity: assorter is in %s state (must be Ready or Running)", currentState.String())
	}

	// This is a placeholder for future implementations
	// The periodic assorter doesn't support per-volume periodicity
	return nil
}

// IsReady returns true if the assorter is ready (activated but loop not started)
func (a *Assorter) IsReady() bool {
	return a.state.get() == StateReady
}

// IsRunning returns true if the assorter is running
func (a *Assorter) IsRunning() bool {
	return a.state.get() == StateRunning
}

// GetVolumeCount returns the number of volumes currently scheduled
func (a *Assorter) GetVolumeCount() int {
	a.volumesMu.RLock()
	defer a.volumesMu.RUnlock()
	return len(a.volumes)
}

// scheduleLoop is the main scheduling loop that runs periodically
func (a *Assorter) scheduleLoop(ctx context.Context) {
	Logc(ctx).WithField("period", a.config.period).Debug(">>>> assorter.scheduleLoop started")
	defer Logc(ctx).Debug("<<<< assorter.scheduleLoop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-a.ticker.C:
			// Periodic tick - publish all volumes
			if err := a.publishVolumes(ctx); err != nil {
				Logc(ctx).WithError(err).Warn("Failed to publish volumes")
			}

			// Reset ticker with jittered period to avoid thundering herd
			a.ticker.Reset(a.getJitteredPeriod(ctx))
		}
	}
}

// getJitteredPeriod returns the assorter period with added jitter.
// It introduces randomness (jitter) between schedule periods to avoid a thundering herd on the poller.
// The jitter is calculated as a random duration up to 20% of the base period.
func (a *Assorter) getJitteredPeriod(ctx context.Context) time.Duration {
	basePeriod := a.config.period

	// Calculate maximum jitter as 20% of the base period
	maxJitter := basePeriod / 5

	jitter := time.Duration(0)
	if maxJitter > 0 {
		if n, err := rand.Int(rand.Reader, big.NewInt(int64(maxJitter))); err == nil {
			jitter = time.Duration(n.Int64())
		} else {
			Logc(ctx).WithError(err).Debug("Failed to generate random jitter, using zero jitter")
		}
	}

	jitteredPeriod := basePeriod + jitter
	Logc(ctx).WithFields(LogFields{
		"basePeriod":     basePeriod,
		"jitter":         jitter,
		"jitteredPeriod": jitteredPeriod,
	}).Debug("Calculated jittered assorter period")

	return jitteredPeriod
}

// publishVolumes publishes all volumes in the scheduling list to the VolumesScheduled eventbus
func (a *Assorter) publishVolumes(ctx context.Context) error {
	Logc(ctx).Debug(">>>> assorter.publishVolumes")
	defer Logc(ctx).Debug("<<<< assorter.publishVolumes")

	// Check if assorter is still running before publishing
	if !a.IsRunning() {
		Logc(ctx).Debug("Assorter not running, skipping publish")
		return nil
	}

	// Ensure publish func is initialized
	if a.config.publishFunc == nil {
		Logc(ctx).Error("PublishFunc not initialized - this is a programming error")
		return fmt.Errorf("PublishFunc not initialized")
	}

	// Get snapshot of current volumes
	a.volumesMu.RLock()
	volumeCount := len(a.volumes)

	// Early exit if no volumes
	if volumeCount == 0 {
		a.volumesMu.RUnlock()
		Logc(ctx).Debug("No volumes to publish")
		return nil
	}

	// Create slice and copy volume names
	volumeNames := make([]string, volumeCount)
	i := 0
	for volumeName := range a.volumes {
		volumeNames[i] = volumeName
		i++
	}
	a.volumesMu.RUnlock()

	// Create a new context with source="assorter" since the assorter is generating this event
	// Use GenerateRequestContext similar to CRD controller pattern
	publishCtx := GenerateRequestContext(nil, "", "scheduler.assorter", WorkflowNone, LogLayerAutogrow)

	// Pass slice directly to callback (callback must not retain it!)
	// This avoids an extra allocation and copy operation
	if err := a.config.publishFunc(publishCtx, volumeNames); err != nil {
		Logc(ctx).WithError(err).WithField("volumeCount", volumeCount).
			Error("Failed to publish volumes via callback")
		return err
	}

	Logc(ctx).WithField("volumeCount", volumeCount).
		Info("Published volumes via callback")

	return nil
}
