// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"context"
	"sync"
	"sync/atomic"

	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	assorterTypes "github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	tridentclientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
)

// State represents the operational state of the Scheduler layer
type State int32

// State constants using iota
const (
	StateStopped  State = iota // Scheduler is stopped
	StateStarting              // Scheduler is starting up (caches syncing, bootstrapping)
	StateReady                 // Scheduler is ready (assorter ready, waiting to start schedule loop)
	StateRunning               // Scheduler is running (assorter schedule loop active)
	StateStopping              // Scheduler is shutting down
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

// String returns the string representation of the State
func (s *State) String() string {
	switch *s {
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

// WorkItem represents a work item in the scheduler's workqueue
// It contains the key to process and tracks retry attempts
type WorkItem struct {
	// Ctx is the context associated with this work item (from ControllerEvent)
	Ctx context.Context

	// Key is the resource identifier to process
	// For namespaced resources (TVP, TBE): "namespace/name" format
	// For cluster-scoped resources (StorageClass): just the name
	Key string

	// ObjectType indicates the type of resource (StorageClass, TridentVolumePublication, etc.)
	// Used for routing to appropriate handler
	ObjectType crdtypes.ObjectType

	// RetryCount tracks how many times this item has been retried
	// Used to limit retries and avoid infinite loops
	RetryCount int
}

// Scheduler is the top layer of the autogrow unit that:
// 1. Subscribes to controller events (SC, TVP, TBE updates)
// 2. Runs precedence checks to determine effective policies
// 3. Updates the effectivePolicyCache
// 4. Manages the assorter component which periodically publishes volumes to poll
type Scheduler struct {
	// State management
	state   State // Current state (State) - use atomic functions for access
	stateMu sync.RWMutex

	// EventBus subscription ID for controller events
	// Single subscription for all controller events (SC, TVP, TBE)
	controllerEventSubscriptionID eventbusTypes.SubscriptionID

	// Work queue for async processing with retry tracking
	workqueue workqueue.TypedRateLimitingInterface[WorkItem]

	// Assorter component (accessed via interface for extensibility)
	// Always created and owned by scheduler (no external injection)
	assorter assorterTypes.Assorter

	// VolumePublishManager for reading volume tracking info from disk
	volumePublishManager nodehelpers.VolumePublishManager

	// Informers and listers
	scLister  storagelisters.StorageClassLister
	tvpLister listerv1.TridentVolumePublicationLister

	// Trident clientset for direct API calls (bootstrap uses this to avoid lister indexer race)
	tridentClient tridentclientset.Interface

	// Autogrow cache for effective policy and volume management
	autogrowCache *agCache.AutogrowCache

	// Worker management
	stopCh chan struct{}

	// Configuration for the Scheduler layer
	config *Config
}
