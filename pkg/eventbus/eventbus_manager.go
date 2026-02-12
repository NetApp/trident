// Copyright 2025 NetApp, Inc. All Rights Reserved.

package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.uber.org/multierr"

	"github.com/netapp/trident/pkg/eventbus/mutexbus/simple"
	"github.com/netapp/trident/pkg/eventbus/types"
	"github.com/netapp/trident/pkg/workerpool"
	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
)

// =============================================================================
// EventManager
// =============================================================================

// managedBus is an interface for closing an EventBus.
type managedBus interface {
	Close(ctx context.Context) error
}

// EventManager manages a shared worker pool and all EventBuses created from it.
// It provides a centralized way to create EventBuses and shut them all down.
//
// Example:
//
//		// Create manager with custom configuration using options
//		mgr, err := eventbus.NewManager(ctx,
//		    eventbus.WithBusConfigTemplate(simple.NewConfig(simple.WithNoHooks())),
//		)
//		if err != nil {
//		    log.Fatal(err)
//		}
//		defer mgr.Shutdown(context.Background())
//
//		// Create typed event buses
//	 In trident context
//	 VolumesScheduledEventBus, err = eventbus.NewBus[
//	 VolumesScheduled, EventBusMetricsOptions[VolumesScheduled]](ctx, mgr)
//
//	 General
//	 userCreated, _ := eventbus.NewBus[User, types.EventBus[User]](ctx, mgr)
//		orderPlaced, _ := eventbus.NewBus[Order, types.EventBus[Order]](ctx, mgr)
//
//		// Subscribe and publish as usual
//		userCreated.Subscribe(handler)
//		userCreated.Publish(ctx, user)
type EventManager struct {
	pool              workerpooltypes.Pool
	buses             []managedBus
	mu                sync.Mutex
	closed            atomic.Bool
	ownsPool          bool         // true if manager created the pool, false if provided externally
	busConfigTemplate types.Config // optional config template for all buses created by this manager
}

// =============================================================================
// EventManager Options
// =============================================================================

// ManagerOption is a functional option for configuring an EventManager.
type ManagerOption func(*EventManager)

// WithWorkerPool sets an external worker pool for the manager to use.
// When this option is provided, the manager will NOT create or shut down the pool.
// The caller retains ownership and responsibility for pool lifecycle.
//
// Example:
//
//	pool, _ := workerpool.New[workerpool.Pool](ctx)
//	pool.Start(ctx)
//	mgr, _ := eventbus.NewManager(ctx, eventbus.WithWorkerPool(pool))
func WithWorkerPool(pool workerpooltypes.Pool) ManagerOption {
	return func(m *EventManager) {
		m.pool = pool
		m.ownsPool = false // External pool - caller owns it
	}
}

// WithBusConfigTemplate sets a template configuration for all buses created by this manager.
// Each bus created via NewBus() will receive a deep copy of this config (unless
// overridden in the NewBus call).
//
// Example:
//
//	busTemplate := simple.NewConfig(
//	    simple.WithNoHooks(),
//	    simple.WithDefaultTimeout(5*time.Second),
//	)
//	mgr, _ := eventbus.NewManager(ctx, eventbus.WithBusConfigTemplate(busTemplate))
//	// All buses will use the template by default
//	bus1, _ := eventbus.NewBus[User, types.EventBus[User]](ctx, mgr)
func WithBusConfigTemplate(busConfig types.Config) ManagerOption {
	return func(m *EventManager) {
		m.busConfigTemplate = busConfig
	}
}

// =============================================================================
// EventManager Constructor and Methods
// =============================================================================

// NewManager creates a new EventManager with its own worker pool.
// The pool is started automatically and ready for use.
// The manager takes ownership and will shut down the pool on Shutdown().
//
// Examples:
//
//	// Manager with default settings
//	mgr, _ := eventbus.NewManager(ctx)
//
//	// Manager with custom bus template
//	busTemplate := simple.NewConfig(simple.WithNoHooks())
//	mgr, _ := eventbus.NewManager(ctx,
//	    eventbus.WithBusConfigTemplate(busTemplate),
//	)
//
//	// Manager with external pool (won't create its own)
//	pool, _ := workerpool.New[workerpool.Pool](ctx)
//	pool.Start(ctx)
//	mgr, _ := eventbus.NewManager(ctx, eventbus.WithWorkerPool(pool))
func NewManager(ctx context.Context, opts ...ManagerOption) (*EventManager, error) {
	// Create manager with defaults
	mgr := &EventManager{
		buses:    make([]managedBus, 0),
		ownsPool: true, // By default, the manager owns the pool
	}

	// Apply options
	for _, opt := range opts {
		opt(mgr)
	}

	// If external pool was provided via options, we're done
	if mgr.pool != nil {
		return mgr, nil
	}

	// Create internal pool with the default configuration
	pool, err := workerpool.New[workerpooltypes.Pool](ctx)
	if err != nil {
		return nil, err
	}

	// Start the pool
	if err := pool.Start(ctx); err != nil {
		// Cleanup: shutdown the pool before returning error
		_ = pool.Shutdown(ctx)
		return nil, err
	}

	mgr.pool = pool
	return mgr, nil
}

// Register adds an EventBus to the manager's tracking list for lifecycle management.
// This allows the manager to coordinate shutdown of all registered buses.
// Returns false if the manager has been shut down (bus not registered).
//
// Use this when you create a bus manually and want the manager to handle its lifecycle:
//
//	cfg := simple.NewConfig(simple.WithNoHooks())
//	cfg.SetWorkerPool(mgr.Pool())
//	bus, _ := simple.NewEventBus[User](ctx, cfg)
//	if !mgr.Register(bus) {
//	    // Manager is closed, handle accordingly
//	}
//
// Note: When using NewBus, registration happens automatically.
func (m *EventManager) Register(bus managedBus) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.IsClosed() {
		return false
	}

	m.buses = append(m.buses, bus)
	return true
}

// Pool returns the underlying worker pool.
// Returns nil if the manager has been shut down or if the pool is not yet initialized.
// Callers should check for nil before using the returned pool.
func (m *EventManager) Pool() workerpooltypes.Pool {
	if m.IsClosed() {
		return nil
	}
	return m.pool
}

// Shutdown gracefully shuts down all EventBuses and optionally the worker pool.
// It waits for all pending async subscriptions to complete before returning.
// The context can be used to set a timeout.
//
// If the manager created its own pool (via NewManager), the pool will be shut down.
// If the manager was given an external pool (via NewManagerWithPool), the pool is
// NOT shut down - the caller retains ownership and responsibility.
//
// Returns a combined error if any buses or the pool fail to shut down.
// Uses multierr to collect all errors encountered during shutdown.
func (m *EventManager) Shutdown(ctx context.Context) (shutdownErr error) {
	if m.closed.Swap(true) {
		return nil // Already closed
	}

	m.mu.Lock()
	buses := make([]managedBus, len(m.buses))
	copy(buses, m.buses)
	// Clear the buses slice to release references and prevent memory leaks
	m.buses = m.buses[:0]
	m.mu.Unlock()

	// Close all buses (this waits for their async subscriptions)
	// Collect all errors using multierr
	for _, bus := range buses {
		if err := bus.Close(ctx); err != nil {
			shutdownErr = multierr.Append(shutdownErr, err)
		}
	}

	// Only shutdown the pool if we own it
	if m.ownsPool {
		if err := m.pool.Shutdown(ctx); err != nil {
			shutdownErr = multierr.Append(shutdownErr, err)
		}
	}

	return
}

// IsClosed returns true if the manager has been shut down.
func (m *EventManager) IsClosed() bool {
	return m.closed.Load()
}

// =============================================================================
// NewBus - Create EventBus from Manager
// =============================================================================

// NewBus creates a new EventBus managed by this EventManager.
// The bus shares the manager's worker pool for async subscriptions.
// The generic type parameter K constrains the return type to types that implement EventBus[T].
//
// Config resolution follows a three-level fallback strategy:
//  1. If config is provided as parameter, use it
//  2. If manager has a config template, use a copy of it
//  3. Otherwise, use the shared default config
//
// The manager's worker pool will be automatically set in the config if not already set.
//
// Examples:
//
//	// Uses manager's config template (or default if not set), returns interface
//	mgr, _ := eventbus.NewManager(ctx)
//	userCreated, _ := eventbus.NewBus[User, types.EventBus[User]](ctx, mgr)
//
//	// Request concrete type
//	userBus, _ := eventbus.NewBus[User, *simple.EventBus[User]](ctx, mgr)
//
//	// Explicit config overrides manager's template
//	cfg := simple.NewConfig(simple.WithNoHooks())
//	orderPlaced, _ := eventbus.NewBus[Order, types.EventBus[Order]](ctx, mgr, cfg)
//
//	// Manager with config template - all buses use it by default
//	template := simple.NewConfig(simple.WithNoHooks())
//	mgr, _ := eventbus.NewManager(ctx, eventbus.WithBusConfigTemplate(template))
//	bus1, _ := eventbus.NewBus[User, types.EventBus[User]](ctx, mgr)    // Uses template
//	bus2, _ := eventbus.NewBus[Order, types.EventBus[Order]](ctx, mgr)   // Uses template
func NewBus[T any, K types.EventBus[T]](ctx context.Context, mgr *EventManager, cfgs ...types.Config) (K, error) {
	var zero K

	// Check if manager is closed before creating bus
	if mgr.IsClosed() {
		return zero, fmt.Errorf("cannot create bus: manager is closed")
	}

	// Get or create config with three-level fallback
	var cfg types.Config

	if len(cfgs) > 0 && cfgs[0] != nil {
		// Level 1: User explicitly provided a config - respect it as-is
		// Don't inject manager's pool; user may want bus to create its own internal pool
		cfg = cfgs[0]
	} else if mgr.busConfigTemplate != nil {
		// Level 2: Use manager's config template (deep copy it)
		// Inject manager's pool if not already set
		cfg = mgr.busConfigTemplate.Copy()
		if cfg.GetWorkerPool() == nil {
			cfg.SetWorkerPool(mgr.pool)
		}
	} else {
		// Level 3: No config provided and no template
		// Create a minimal config with the manager's worker pool
		cfg = simple.NewConfig(simple.WithWorkerPoolConfig(mgr.pool))
	}

	// Create the bus using the main factory function
	bus, err := New[T, K](ctx, cfg)
	if err != nil {
		return zero, err
	}

	// Register with manager for lifecycle management
	if !mgr.Register(bus) {
		// Manager was closed between our check and registration
		// Close the newly created bus to clean up resources
		_ = bus.Close(ctx)
		return zero, fmt.Errorf("cannot register bus: manager is closed")
	}

	return bus, nil
}
