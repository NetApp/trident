// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

import (
	"context"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	tridentclientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	poolTypes "github.com/netapp/trident/pkg/workerpool/types"
)

// State RequesterState represents the operational state of the Requester layer
type State int32

// State constants using iota
const (
	StateStopped  State = iota // Requester is stopped
	StateStarting              // Requester is starting up
	StateRunning               // Requester is running
	StateStopping              // Requester is shutting down
)

// String returns the string representation of the RequesterState
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

// WorkItem represents a work item in the requester's workqueue
// It contains the TVP name to process and tracks retry attempts
type WorkItem struct {
	// Ctx is the context associated with this work item (from VolumeThresholdBreached event)
	Ctx context.Context

	// TVPName is the TridentVolumePublication name (volumeID.nodeID format)
	TVPName string

	// RetryCount tracks how many times this item has been retried
	// Used to limit retries and avoid infinite loops
	RetryCount int
}

// Requester is the bottom most layer of the autogrow unit that creates autogrow request internal CRs
type Requester struct {
	// State management
	state   State // Current state (State) - use atomic functions for access
	stateMu sync.RWMutex

	// EventBus subscription ID
	subscriptionID eventbusTypes.SubscriptionID

	// Work queue for async processing with retry tracking
	workqueue workqueue.TypedRateLimitingInterface[WorkItem]

	// Latest events map - stores most recent event per TVP
	// This allows us to process only the latest event when multiple events arrive
	latestEventsMu sync.RWMutex
	latestEvents   map[string]autogrowTypes.VolumeThresholdBreached // TVPName -> latest event

	// Kubernetes clients
	tridentClient tridentclientset.Interface

	// Lister fetches agp information from the sharedInformer's cache
	agpLister listerv1.TridentAutogrowPolicyLister

	// Lister fetches TVP information from the sharedInformer's cache
	tvpLister listerv1.TridentVolumePublicationLister

	// Autogrow cache for effective policy lookups
	autogrowCache *agCache.AutogrowCache

	// Worker pool for processing work items
	workerPool    poolTypes.Pool
	ownWorkerPool bool // true if we created the pool, false if it was provided

	// Worker management
	stopCh chan struct{}

	// Configuration for the Requester layer
	config *Config
}
