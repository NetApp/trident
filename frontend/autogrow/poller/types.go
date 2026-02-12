// Copyright 2026 NetApp, Inc. All Rights Reserved.

package poller

import (
	"context"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	poolTypes "github.com/netapp/trident/pkg/workerpool/types"
)

// State represents the operational state of the Poller layer
type State int32

// State constants using iota
const (
	StateStopped  State = iota // Poller is stopped
	StateStarting              // Poller is starting up
	StateRunning               // Poller is running
	StateStopping              // Poller is shutting down
)

// String returns the string representation of the State
func (s *State) String() string {
	switch *s {
	case StateStopped:
		return "Stopped"
	case StateStarting:
		return "Starting"
	case StateRunning:
		return "Running"
	case StateStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

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

// WorkItem represents a work item in the poller's workqueue
// It contains the TVP name to poll and tracks retry attempts
type WorkItem struct {
	// Ctx is the context associated with this work item (from VolumesScheduled event)
	Ctx context.Context

	// TVPName is the TridentVolumePublication name to poll (volumeID.nodeID format)
	TVPName string

	// RetryCount tracks how many times this item has been retried
	// Used to limit retries and avoid infinite loops
	RetryCount int
}

// Poller is the middle layer of the Autogrow unit that polls volumes for usage
// and publishes VolumeThresholdBreached events to the event bus
type Poller struct {
	// State management
	state   State // Current state (State) - use atomic functions for access
	stateMu sync.RWMutex

	// EventBus subscription ID for VolumesScheduled events
	subscriptionID eventbusTypes.SubscriptionID

	// Work queue for async volume polling with retry tracking
	workqueue workqueue.TypedRateLimitingInterface[WorkItem]

	// Lister for Autogrow policies
	agpLister listerv1.TridentAutogrowPolicyLister

	// Lister for TridentVolumePublications (needed to lookup TVP by name and extract volumeID)
	tvpLister listerv1.TridentVolumePublicationLister

	// Autogrow cache for effective policy lookups
	autogrowCache *agCache.AutogrowCache

	// VolumeStatsProvider for fetching volume usage statistics
	volumeStatsProvider nodehelpers.VolumeStatsManager

	// Worker pool for processing polling work items
	workerPool    poolTypes.Pool
	ownWorkerPool bool // true if we created the pool, false if it was provided

	// Worker management
	stopCh chan struct{}

	// Configuration for the Poller layer
	config *Config
}
