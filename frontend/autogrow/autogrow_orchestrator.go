// Copyright 2026 NetApp, Inc. All Rights Reserved.

package autogrow

//go:generate mockgen -destination=../../mocks/mock_frontend/mock_autogrow/mock_orchestrator.go github.com/netapp/trident/frontend/autogrow AutogrowOrchestrator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	storagelisters "k8s.io/client-go/listers/storage/v1"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	"github.com/netapp/trident/frontend/autogrow/controller"
	"github.com/netapp/trident/frontend/autogrow/poller"
	"github.com/netapp/trident/frontend/autogrow/requester"
	"github.com/netapp/trident/frontend/autogrow/scheduler"
	agTypes "github.com/netapp/trident/frontend/autogrow/types"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	tridentclientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	"github.com/netapp/trident/pkg/workerpool"
	"github.com/netapp/trident/pkg/workerpool/ants"
	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
	"github.com/netapp/trident/utils/errors"
)

// AutogrowOrchestrator defines the interface for the Autogrow orchestrator
// This interface is used for testing and dependency injection
type AutogrowOrchestrator interface {
	// Activate starts the Autogrow feature and all its layers
	Activate(ctx context.Context) error

	// Deactivate stops the Autogrow feature and all its layers
	Deactivate(ctx context.Context) error

	// IsRunning returns true if Autogrow is currently running
	IsRunning() bool

	// GetState returns the current operational state
	GetState() State

	// GetCache returns the AutogrowCache instance
	GetCache() *agCache.AutogrowCache

	// HandleStorageClassEvent processes StorageClass CRD events
	HandleStorageClassEvent(ctx context.Context, eventType EventType, name string)

	// HandleTVPEvent processes TridentVolumePublication CRD events
	HandleTVPEvent(ctx context.Context, eventType EventType, key string)

	// HandleTBEEvent processes TridentBackend CRD events
	HandleTBEEvent(ctx context.Context, eventType EventType, key string)
}

// EventType is an alias for CRD event types
type EventType = crdtypes.EventType

// State represents the operational state of the Autogrow feature
type State int32

const (
	StateStopped  State = iota // Autogrow is stopped
	StateStarting              // Autogrow is starting up
	StateRunning               // Autogrow is running
	StateStopping              // Autogrow is shutting down
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
	case StateRunning:
		return "Running"
	case StateStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

// Autogrow is the top-level orchestrator for the Autogrow feature
// It manages four layers: Controller, Scheduler, Poller, and Requester
type Autogrow struct {
	// State management
	state   State
	stateMu sync.RWMutex

	// AutogrowCache - shared across all layers
	cache *agCache.AutogrowCache

	// Four layers (using interface to avoid import cycles)
	controller *controller.Controller // Controller layer (lightweight, handles CRD events)
	scheduler  *scheduler.Scheduler   // Scheduler layer (precedence checks, assorter)
	poller     *poller.Poller         // Poller layer (polls volume usage)
	requester  *requester.Requester   // Requester layer (handles growth requests)

	// Worker pool for async task execution (shared by eventbus, scheduler, requester)
	workerPool workerpooltypes.Pool

	// Eventbus Manager
	eventbusManager *eventbus.EventManager

	// Autogrow period from command line (passed to scheduler and poller)
	autogrowPeriod time.Duration
}

// New creates a new Autogrow instance with all dependencies
// and initializes all layers with default configurations.
// This is the recommended way to create an Autogrow instance.
//
// It will:
// 1. Create AutogrowCache
// 2. Create Controller layer with defaults
// 3. Create Scheduler layer with defaults
// 4. Create Poller layer with defaults
// 5. Create Requester layer with defaults
func New(
	ctx context.Context,
	tridentClient tridentclientset.Interface,
	agpLister listerv1.TridentAutogrowPolicyLister,
	scLister storagelisters.StorageClassLister,
	tvpLister listerv1.TridentVolumePublicationLister,
	nodeHelper nodehelpers.NodeHelper,
	tridentNamespace string,
	autogrowPeriod time.Duration,
	schedulerConfig *scheduler.Config,
	pollerConfig *poller.Config,
	requesterConfig *requester.Config,
) (*Autogrow, error) {
	Logc(ctx).Debug(">>>> Autogrow.New")
	defer Logc(ctx).Debug("<<<< Autogrow.New")

	// Declare variables
	var (
		err             error
		controllerLayer *controller.Controller
		schedulerLayer  *scheduler.Scheduler
		pollerLayer     *poller.Poller
		requesterLayer  *requester.Requester
	)

	// 1. Create AutogrowCache
	Logc(ctx).Info("Creating AutogrowCache")
	agCache := agCache.NewAutogrowCache()
	Logc(ctx).Info("Created AutogrowCache")
	// Now we'll create all the layers

	// 2. Create Controller layer (lightweight, no config needed)
	Logc(ctx).Info("Creating Controller layer with defaults")
	controllerLayer, err = controller.NewController(ctx, agCache)
	if err != nil {
		return nil, fmt.Errorf("failed to create controller layer: %w", err)
	}
	Logc(ctx).Info("Created Controller layer with defaults")

	// 3. Create Scheduler layer with config using autogrowPeriod
	Logc(ctx).Info("Creating Scheduler layer with Autogrow period from command line")
	// Create scheduler config with the Autogrow period and namespace
	if schedulerConfig == nil {
		schedulerConfig = scheduler.NewConfig(
			scheduler.WithAssorterPeriod(autogrowPeriod),
			scheduler.WithTridentNamespace(tridentNamespace),
		)
	} else {
		// If config provided, ensure namespace and period are set
		if schedulerConfig.TridentNamespace == "" {
			schedulerConfig.TridentNamespace = tridentNamespace
		}
		if schedulerConfig.AssorterPeriod == 0 {
			schedulerConfig.AssorterPeriod = autogrowPeriod
		}
	}
	schedulerLayer, err = scheduler.NewScheduler(
		ctx,
		scLister,
		tvpLister,
		tridentClient,
		agCache,
		nodeHelper,
		schedulerConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler layer: %w", err)
	}
	Logc(ctx).Info("Created Scheduler layer with defaults")

	// 4. Create Poller layer with config using autogrowPeriod
	Logc(ctx).Info("Creating Poller layer with Autogrow period from command line")
	// Create poller config with Autogrow period and namespace
	if pollerConfig == nil {
		pollerConfig = poller.NewConfig(
			poller.WithAutogrowPeriod(autogrowPeriod),
			poller.WithTridentNamespace(tridentNamespace),
		)
	} else {
		// If config provided, ensure namespace and Autogrow period are set
		if pollerConfig.TridentNamespace == "" {
			pollerConfig.TridentNamespace = tridentNamespace
		}
		if pollerConfig.AutogrowPeriod == nil {
			pollerConfig.AutogrowPeriod = &autogrowPeriod
		}
	}
	pollerLayer, err = poller.NewPoller(
		ctx,
		agpLister,
		tvpLister,
		agCache,
		nodeHelper,
		pollerConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create poller layer: %w", err)
	}
	Logc(ctx).Info("Created Poller layer with defaults")

	// 5. Create Requester layer with default config
	Logc(ctx).Info("Creating Requester layer with defaults")
	// Ensure requester config has the Trident namespace set
	if requesterConfig == nil {
		requesterConfig = requester.NewConfig(requester.WithTridentNamespace(tridentNamespace))
	} else if requesterConfig.TridentNamespace == "" {
		requesterConfig.TridentNamespace = tridentNamespace
	}
	requesterLayer, err = requester.NewRequester(
		ctx,
		tridentClient,
		agpLister,
		tvpLister,
		agCache,
		requesterConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create requester layer: %w", err)
	}
	Logc(ctx).Info("Created Requester layer with defaults")

	ag := &Autogrow{
		state:          StateStopped,
		cache:          agCache,
		controller:     controllerLayer,
		scheduler:      schedulerLayer,
		poller:         pollerLayer,
		requester:      requesterLayer,
		autogrowPeriod: autogrowPeriod,
		// workerPool and eventbusManager are created in Activate(), not here
	}

	Logc(ctx).Info("Autogrow feature initialized successfully")
	return ag, nil
}

// Activate creates resources and starts all Autogrow layers in the correct order:
// 0. Worker Pool (created fresh each time)
// 0.5. Eventbus Manager and Eventbuses (created fresh each time)
// 1. Controller
// 2. Scheduler
// 3. Poller
// 4. Requester
// 5. Scheduler Schedule Loop
func (ag *Autogrow) Activate(ctx context.Context) error {
	// Check if we can transition to Starting state
	if !ag.state.compareAndSwap(StateStopped, StateStarting) {
		currentState := ag.state.get()
		if currentState == StateRunning {
			Logc(ctx).Debug("Autogrow already running, skipping activation")
			return nil // Idempotent
		}
		// Return StateError with state and message - caller will decide what to do
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot activate Autogrow: current state is %s", currentState.String()))
	}

	ag.stateMu.Lock()
	defer ag.stateMu.Unlock()

	Logc(ctx).Info(">>>> Activating Autogrow feature")
	defer Logc(ctx).Info("<<<< Autogrow feature activation complete")

	// Activate layers in order:

	// 0. Create Worker Pool (must be first, as eventbuses use it)
	Logc(ctx).Info("Creating worker pool")
	// TODO (@pshashan): After benchmark testing change the num of workers actually required.
	workerPool, err := workerpool.New[workerpooltypes.Pool](
		ctx,
		ants.NewConfig(
			ants.WithNumWorkers(DefaultWorkerPoolSize),
			ants.WithPreAlloc(DefaultWorkerPoolPreAlloc),
			ants.WithNonBlocking(DefaultWorkerPoolNonBlocking),
		),
	)
	if err != nil {
		ag.state.set(StateStopped)
		return fmt.Errorf("failed to create worker pool: %w", err)
	}
	ag.workerPool = workerPool
	Logc(ctx).WithFields(LogFields{
		"size":        DefaultWorkerPoolSize,
		"preAlloc":    DefaultWorkerPoolPreAlloc,
		"nonBlocking": DefaultWorkerPoolNonBlocking,
	}).Info("Worker pool created")

	// Start the worker pool
	if err := ag.workerPool.Start(ctx); err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to start worker pool: %w", err)
	}
	Logc(ctx).Info("Worker pool started")

	// 0.5. Create eventbus manager and eventbuses
	Logc(ctx).Info("Creating eventbus manager")
	eventbusManager, err := eventbus.NewManager(
		ctx,
		eventbus.WithWorkerPool(ag.workerPool),
	)
	if err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to create eventbus manager: %w", err)
	}
	ag.eventbusManager = eventbusManager
	Logc(ctx).Info("Eventbus manager created")

	// Create eventbuses
	Logc(ctx).Info("Creating Autogrow eventbuses")

	eventbus.VolumeThresholdBreachedEventBus, err = eventbus.NewBus[
		agTypes.VolumeThresholdBreached,
		eventbusTypes.EventBusMetricsOptions[agTypes.VolumeThresholdBreached],
	](ctx, ag.eventbusManager)
	if err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to create VolumeThresholdBreached eventbus: %w", err)
	}

	eventbus.VolumesScheduledEventBus, err = eventbus.NewBus[
		agTypes.VolumesScheduled,
		eventbusTypes.EventBusMetricsOptions[agTypes.VolumesScheduled],
	](ctx, ag.eventbusManager)
	if err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to create VolumesScheduled eventbus: %w", err)
	}

	eventbus.ControllerEventBus, err = eventbus.NewBus[
		agTypes.ControllerEvent,
		eventbusTypes.EventBusMetricsOptions[agTypes.ControllerEvent],
	](ctx, ag.eventbusManager)
	if err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to create ControllerEvent eventbus: %w", err)
	}

	Logc(ctx).Info("Created Autogrow eventbuses")

	// 1. Activate Controller (lightweight, currently a no-op)
	if err := ag.controller.Activate(ctx); err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to activate controller: %w", err)
	}
	Logc(ctx).Info("Controller layer activated")

	// 2. Activate Scheduler
	if err := ag.scheduler.Activate(ctx); err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to activate scheduler: %w", err)
	}
	Logc(ctx).Info("Scheduler layer activated")

	// 3. Activate Poller (poller will create its own worker pool with Autogrow period settings)
	if err := ag.poller.Activate(ctx); err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to activate poller: %w", err)
	}
	Logc(ctx).Info("Poller layer activated")

	// 4. Activate Requester
	if err := ag.requester.Activate(ctx); err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to activate requester: %w", err)
	}
	Logc(ctx).Info("Requester layer activated")

	// 5. Start the scheduler's schedule loop (includes assorter and reconciliation)
	if err := ag.scheduler.StartScheduleLoop(ctx); err != nil {
		ag.cleanup(ctx)
		return fmt.Errorf("failed to start scheduler schedule loop: %w", err)
	}
	Logc(ctx).Info("Scheduler schedule loop started")

	// All layers activated successfully
	ag.state.set(StateRunning)
	Logc(ctx).Info("Autogrow feature is now running")
	return nil
}

// Deactivate stops all Autogrow layers and cleans up resources in reverse order:
// 1. Requester
// 2. Poller
// 3. Scheduler
// 4. Controller
// 5. Eventbus Manager (shuts down all eventbuses)
// 6. Worker Pool (shut down explicitly as we own it)
// 7. Autogrow Cache (clears all cached data)
func (ag *Autogrow) Deactivate(ctx context.Context) error {
	// Check if we can transition to Stopping state
	if !ag.state.compareAndSwap(StateRunning, StateStopping) {
		currentState := ag.state.get()
		if currentState == StateStopped {
			Logc(ctx).Debug("Autogrow already stopped, skipping deactivation")
			return nil // Idempotent
		}
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot deactivate Autogrow: current state is %s", currentState.String()))
	}

	ag.stateMu.Lock()
	defer ag.stateMu.Unlock()

	Logc(ctx).Info(">>>> Deactivating Autogrow feature")
	defer Logc(ctx).Info("<<<< Autogrow feature deactivation complete")

	// Cleanup (handles all layer deactivation and resource cleanup)
	ag.cleanup(ctx)

	Logc(ctx).Info("Autogrow feature stopped")
	return nil
}

// cleanup performs final cleanup operations
// This function is safe to call multiple times and handles nil checks internally
func (ag *Autogrow) cleanup(ctx context.Context) {
	// Deactivate layers in reverse order:

	// 1. Deactivate Requester
	if ag.requester != nil {
		if err := ag.requester.Deactivate(ctx); err != nil {
			Logc(ctx).WithError(err).Error("Failed to deactivate requester")
			// Continue deactivation even if requester fails
		} else {
			Logc(ctx).Info("Requester layer deactivated")
		}
	}

	// 2. Deactivate Poller
	if ag.poller != nil {
		if err := ag.poller.Deactivate(ctx); err != nil {
			Logc(ctx).WithError(err).Error("Failed to deactivate poller")
			// Continue deactivation even if poller fails
		} else {
			Logc(ctx).Info("Poller layer deactivated")
		}
	}

	// 3. Deactivate Scheduler
	if ag.scheduler != nil {
		if err := ag.scheduler.Deactivate(ctx); err != nil {
			Logc(ctx).WithError(err).Error("Failed to deactivate scheduler")
			// Continue deactivation even if scheduler fails
		} else {
			Logc(ctx).Info("Scheduler layer deactivated")
		}
	}

	// 4. Deactivate Controller (lightweight, currently a no-op)
	if ag.controller != nil {
		if err := ag.controller.Deactivate(ctx); err != nil {
			Logc(ctx).WithError(err).Error("Failed to deactivate controller")
			// Continue deactivation even if controller fails
		} else {
			Logc(ctx).Info("Controller layer deactivated")
		}
	}

	// 5. Shutdown eventbus manager (which shuts down all registered buses)
	if ag.eventbusManager != nil {
		Logc(ctx).Info("Shutting down eventbus manager")
		if err := ag.eventbusManager.Shutdown(ctx); err != nil {
			Logc(ctx).WithError(err).Error("Failed to shutdown eventbus manager")
			// Continue deactivation even if eventbus manager fails
		} else {
			Logc(ctx).Info("Eventbus manager shut down successfully")
		}

		// Clear global eventbus references to prevent accidental usage after shutdown
		eventbus.VolumeThresholdBreachedEventBus = nil
		eventbus.VolumesScheduledEventBus = nil
		eventbus.ControllerEventBus = nil
		Logc(ctx).Debug("Global eventbus references cleared")

		// Clear the eventbus manager reference
		ag.eventbusManager = nil
	}

	// 6. Shutdown worker pool (eventbus manager doesn't own it, so we must shut it down)
	if ag.workerPool != nil {
		Logc(ctx).Info("Shutting down worker pool")
		if err := ag.workerPool.Shutdown(ctx); err != nil {
			Logc(ctx).WithError(err).Error("Failed to shutdown worker pool")
			// Continue deactivation even if worker pool fails
		} else {
			Logc(ctx).Info("Worker pool shut down successfully")
		}

		// Clear the worker pool reference
		ag.workerPool = nil
	}

	// 7. Clear the Autogrow cache
	if ag.cache != nil {
		Logc(ctx).Info("Clearing Autogrow cache")
		ag.cache.Clear()
		Logc(ctx).Info("Autogrow cache cleared")
	}

	// 8. Reset state to Stopped
	ag.state.set(StateStopped)
}

// IsRunning returns true if Autogrow is currently running
func (ag *Autogrow) IsRunning() bool {
	return ag.state.get() == StateRunning
}

// GetState returns the state of Autogrow Orchestrator
func (ag *Autogrow) GetState() State {
	return ag.state.get()
}

// GetCache returns the AutogrowCache (useful for testing/debugging)
func (ag *Autogrow) GetCache() *agCache.AutogrowCache {
	return ag.cache
}

// ========================================
// Translation functions for node CRD handlers
// These delegate to the controller layer
// ========================================

// HandleStorageClassEvent translates StorageClass events from node handlers to the controller
func (ag *Autogrow) HandleStorageClassEvent(ctx context.Context, eventType EventType, name string) {
	currentState := ag.state.String()

	// Check if Autogrow is running before processing events
	if !ag.IsRunning() {
		Logc(ctx).WithFields(LogFields{
			"eventType": eventType,
			"name":      name,
			"state":     currentState,
		}).Debug("Autogrow not running, ignoring StorageClass event")
		return
	}

	if ag.controller != nil {
		ag.controller.HandleStorageClassEvent(ctx, eventType, name)
	} else {
		Logc(ctx).Warn("Autogrow controller not initialized, cannot handle StorageClass event")
	}
}

// HandleTVPEvent translates TridentVolumePublication events from node handlers to the controller
func (ag *Autogrow) HandleTVPEvent(ctx context.Context, eventType EventType, key string) {
	currentState := ag.state.String()

	// Check if Autogrow is running before processing events
	if !ag.IsRunning() {
		Logc(ctx).WithFields(LogFields{
			"eventType": eventType,
			"key":       key,
			"state":     currentState,
		}).Debug("Autogrow not running, ignoring TVP event")
		return
	}

	if ag.controller != nil {
		ag.controller.HandleTVPEvent(ctx, eventType, key)
	} else {
		Logc(ctx).Warn("Autogrow controller not initialized, cannot handle TVP event")
	}
}

// HandleTBEEvent translates TridentBackend events from node handlers to the controller
func (ag *Autogrow) HandleTBEEvent(ctx context.Context, eventType EventType, key string) {
	currentState := ag.state.String()

	// Check if Autogrow is running before processing events
	if !ag.IsRunning() {
		Logc(ctx).WithFields(LogFields{
			"eventType": eventType,
			"key":       key,
			"state":     currentState,
		}).Debug("Autogrow not running, ignoring TBE event")
		return
	}

	if ag.controller != nil {
		ag.controller.HandleTBEEvent(ctx, eventType, key)
	} else {
		Logc(ctx).Warn("Autogrow controller not initialized, cannot handle TBE event")
	}
}
