// Copyright 2025 NetApp, Inc. All Rights Reserved.

package simple

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brunoga/deep"
	"go.uber.org/multierr"

	"github.com/netapp/trident/pkg/eventbus/types"
	genericsyncpool "github.com/netapp/trident/pkg/generic_syncpool"
	"github.com/netapp/trident/pkg/workerpool"
	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
	"github.com/netapp/trident/utils/errors"
)

// Compile-time assertions to ensure EventBus implements all expected interfaces
var (
	_ types.EventBus[any]                    = (*EventBus[any])(nil)
	_ types.EventBusWithOptions[any]         = (*EventBus[any])(nil)
	_ types.EventBusWithHooks[any]           = (*EventBus[any])(nil)
	_ types.EventBusWithMetrics[any]         = (*EventBus[any])(nil)
	_ types.EventBusHooksMetrics[any]        = (*EventBus[any])(nil)
	_ types.EventBusMetricsOptions[any]      = (*EventBus[any])(nil)
	_ types.EventBusHooksMetricsOptions[any] = (*EventBus[any])(nil)
)

const (
	preAllocHookCapacity = 4
)

// subscriptionPool is a wrapper type for pooling handler slices
type subscriptionPool[T any] struct {
	subscriptions []subscription[T]
}

// hookWrappers are wrapper types for pooling hook slices
type beforeHookPool[T any] struct {
	hooks BeforeHooks[T]
}

type afterHookPool[T any] struct {
	hooks AfterHooks[T]
}

type errorHookPool[T any] struct {
	hooks ErrorHooks[T]
}

// Re-export types from types package for convenience and to implement the interface
type (
	SubscriptionID          = types.SubscriptionID
	Result                  = types.Result
	SubscriptionFunc[T any] = types.SubscriptionFunc[T]
	SubscribeOption[T any]  = types.SubscribeOption[T]
)

// EventBus represents a typed event bus that subscriptions can subscribe to.
//
// The struct is exported to allow type assertions and access to all implemented interfaces,
// but all fields are unexported to maintain encapsulation and prevent external modification.
//
// Handlers return errors which can be observed via hooks at two levels:
//   - Bus-level hooks: Apply to ALL subscriptions (global observability)
//   - Handler-level hooks: Apply to specific handler (fine-grained control)
//
// Example:
//
//	var UserCreated = eventbus.New[User]()
//
//	Bus-level hook: metrics for ALL subscriptions
//	UserCreated.SetGlobalAfterHook(metricsHook)
//
//	// Handler-level hooks: specific error handling for THIS handler
//	UserCreated.Subscribe(sendWelcomeEmail,
//	    UserCreated.WithOnError(func(ctx context.Context, user User, id SubscriptionID, err error) {
//	        alertOncall("welcome email failed", err)
//	    }),
//	)
//
//	// Publish
//	UserCreated.Publish(context.Background(), user)
type EventBus[T any] struct {
	mu              sync.RWMutex           // Protects handlers and slices
	closed          atomic.Bool            // Checked on Subscribe & Publish, set on Close
	noHooks         bool                   // Checked on every Publish
	defaultTimeout  time.Duration          // Default timeout for handlers without specific timeout
	shutdownTimeout time.Duration          // Maximum time to wait during Close()
	handlers        map[SubscriptionID]int // Map for O(1) lookups with slice's zero-based index
	syncHandlers    []subscription[T]      // Sync Subscribers
	asyncHandlers   []subscription[T]      // Async Subscribers

	nextID atomic.Uint64 // Atomic increment on every Subscribe

	// ------------ SyncPools ------------
	// Worker pool
	workerPool     workerpooltypes.Pool
	ownsWorkerPool bool // true if this bus created the pool and should shut it down

	// Slice pools
	syncSlicePool  *genericsyncpool.Pool[*subscriptionPool[T]]
	asyncSlicePool *genericsyncpool.Pool[*subscriptionPool[T]]

	// Hook pools
	beforeHookPool *genericsyncpool.Pool[*beforeHookPool[T]]
	afterHookPool  *genericsyncpool.Pool[*afterHookPool[T]]
	errorHookPool  *genericsyncpool.Pool[*errorHookPool[T]]
	// -----------------------------------

	// Bus-level hooks
	beforeHooks BeforeHooks[T]
	afterHooks  AfterHooks[T]
	errorHooks  ErrorHooks[T]

	// Metrics - atomic counters for lock-free access
	publishedCount  atomic.Uint64 // Total number of events published
	processedCount  atomic.Uint64 // Total number of handler invocations (successful + errors)
	errorCount      atomic.Uint64 // Total number of handler errors
	subscriberCount atomic.Uint64 // Current number of active subscribers

	wg           sync.WaitGroup // Tracks pending async jobs for THIS bus only
	shutdownOnce sync.Once      // Ensures Close is called only once
}

type subscription[T any] struct {
	id       SubscriptionID      // Unique ID for this subscription
	callback SubscriptionFunc[T] // The handler function
	async    bool                // If true, run in worker pool (non-blocking)
	timeout  time.Duration       // If > 0, handler execution timeout; if 0, run indefinitely

	serial bool // If true, only one instance can execute at a time
	// Mutex pointer for serializing handler execution when serial=true
	// This is a pointer so that when we copy the subscription during Publish,
	// all copies share the same mutex instance
	mu *sync.Mutex

	flagOnce bool // If true, handler executes only once using atomic CAS
	// Atomic flag for Once handlers - when flagOnce=true, this is set atomically
	// on first execution. Subsequent publish calls will see this and skip execution.
	// This is a pointer so that when we copy the subscription during Publish,
	// all copies share the same atomic flag instance (similar to mu).
	executed *atomic.Bool

	// Handler-level hooks (specific to this handler)
	// These are slices that can be set with SubscriptionOptions
	beforeHooks BeforeHooks[T]
	afterHooks  AfterHooks[T]
	errorHooks  ErrorHooks[T]
}

// NewEventBus creates an EventBus using a Config struct.
// The returned EventBus is a concrete type with unexported fields for encapsulation,
// but can be used with any interface it implements (EventBus, EventBusWithHooks, etc.).
//
//	 Example:
//
//		cfg := simple.NewConfig(
//			simple.WithNoHooks(),
//			simple.WithDefaultTimeout(5*time.Second),
//		)
//		bus, err := simple.NewEventBus[User](ctx, cfg)
func NewEventBus[T any](ctx context.Context, cfg *Config) (*EventBus[T], error) {
	// Set pre-alloc capacity if specified
	preAllocCap := cfg.PreAllocHandlers
	if preAllocCap == 0 {
		preAllocCap = defaultPreAllocHandlers
	}

	// Set shutdown timeout - use default if not specified
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = defaultShutdownTimeout
	}

	bus := &EventBus[T]{
		handlers:        make(map[SubscriptionID]int),
		syncHandlers:    make([]subscription[T], 0, preAllocCap),
		asyncHandlers:   make([]subscription[T], 0, preAllocCap),
		defaultTimeout:  cfg.DefaultTimeout,
		shutdownTimeout: shutdownTimeout,
	}

	if cfg.NoHooks != nil {
		bus.noHooks = *cfg.NoHooks
	} else {
		bus.noHooks = defaultNoHooks
	}

	// Create worker pool if no custom pool provided
	// Else use the provided pool directly
	if cfg.WorkerPool == nil {
		// Create a copy of the default worker pool config and modify it
		defaultConfig, err := deep.Copy(defaultWorkerPoolConfig)
		if err != nil {
			return nil, err
		}
		defaultConfig.NumWorkers = defaultNumWorkers

		pool, err := workerpool.New[workerpooltypes.Pool](ctx, defaultConfig)

		if err != nil {
			return nil, err
		}
		if err := pool.Start(ctx); err != nil {
			return nil, err
		}
		bus.workerPool = pool
		bus.ownsWorkerPool = true
	} else {
		bus.workerPool = cfg.WorkerPool
		bus.ownsWorkerPool = false
	}

	// Create slice pools
	bus.syncSlicePool = genericsyncpool.NewPool(func() *subscriptionPool[T] {
		return &subscriptionPool[T]{
			subscriptions: make([]subscription[T], 0, preAllocCap),
		}
	})

	bus.asyncSlicePool = genericsyncpool.NewPool(func() *subscriptionPool[T] {
		return &subscriptionPool[T]{
			subscriptions: make([]subscription[T], 0, preAllocCap),
		}
	})

	// Initialize hook pools if hooks are enabled
	if !bus.noHooks {
		bus.beforeHookPool = genericsyncpool.NewPool(func() *beforeHookPool[T] {
			return &beforeHookPool[T]{
				hooks: make(BeforeHooks[T], 0, preAllocHookCapacity),
			}
		})

		bus.afterHookPool = genericsyncpool.NewPool(func() *afterHookPool[T] {
			return &afterHookPool[T]{
				hooks: make(AfterHooks[T], 0, preAllocHookCapacity),
			}
		})

		bus.errorHookPool = genericsyncpool.NewPool(func() *errorHookPool[T] {
			return &errorHookPool[T]{
				hooks: make(ErrorHooks[T], 0, preAllocHookCapacity),
			}
		})
	}

	return bus, nil
}

// Subscribe adds an event handler and returns its unique ID.
// By default, subscriptions are synchronous and persistent(stays in-memory).
// Use options to modify behavior:
//   - bus.WithAsync(): run in worker pool (non-blocking)
//   - bus.WithOnce(): remove handler after first execution
//   - bus.WithTimeout(duration): set maximum handler execution time
//   - bus.WithSerial(): serialize handler executions (no concurrent runs of same handler)
//   - bus.WithBefore(hooks)/WithAfter(hooks)/WithOnError(hooks): attach hooks
//
// Returns ErrBusClosed if the bus has been closed, or ErrNilHandler if fn is nil.
//
// Examples:
//
//	bus.Subscribe(handler)                                           							// sync, persistent, no timeout
//	bus.Subscribe(handler, bus.WithAsync())                          							// async concurrent
//	bus.Subscribe(handler, bus.WithOnce())                           							// sync, one-time
//	bus.Subscribe(handler, bus.WithOnce(), bus.WithAsync())          							// async, one-time
//	bus.Subscribe(handler, bus.WithSerial())                         							// sync, serialized
//	bus.Subscribe(handler, bus.WithAsync(), bus.WithSerial())        							// async, serialized
//	bus.Subscribe(handler, bus.WithOnce(), bus.WithSerial())           							// sync, one-time, serialized
//	bus.Subscribe(handler, bus.WithTimeout(5*time.Second))           							// sync with 5s timeout
//	bus.Subscribe(handler, bus.WithAsync(), bus.WithTimeout(3*time.Second))  	                // async with 3s timeout
//	bus.Subscribe(handler, bus.WithBefore(hooks), bus.WithAfter(hooks), bus.WithOnError(hooks)) // with hooks
func (bus *EventBus[T]) Subscribe(fn SubscriptionFunc[T], opts ...SubscribeOption[T]) (SubscriptionID, error) {
	if bus.closed.Load() {
		return 0, errors.BusClosedError()
	}

	if fn == nil {
		return 0, errors.NilHandlerError()
	}

	// Getting the id for the handler
	id := SubscriptionID(bus.nextID.Add(1))

	// Build subscription options from functional options
	subOpts := &types.SubscriptionOptions[T]{}
	for _, opt := range opts {
		opt(subOpts)
	}

	// Create handler from options
	handler := &subscription[T]{
		id:       id,
		callback: fn,
		async:    subOpts.Async,
		flagOnce: subOpts.Once,
		serial:   subOpts.Serial,
		timeout:  subOpts.Timeout,
	}

	// Copy hooks from options
	if len(subOpts.BeforeHooks) > 0 {
		handler.beforeHooks = subOpts.BeforeHooks
	}
	if len(subOpts.AfterHooks) > 0 {
		handler.afterHooks = subOpts.AfterHooks
	}
	if len(subOpts.ErrorHooks) > 0 {
		handler.errorHooks = subOpts.ErrorHooks
	}

	// Create mutex for serial handlers
	// The mutex pointer is stored in the handler and will be garbage collected
	// when the handler is removed
	if handler.serial {
		handler.mu = &sync.Mutex{}
	}

	// Create atomic bool for Once handlers
	// The pointer is shared across all copies of this subscription made during Publish,
	// ensuring all copies see the same executed state
	if handler.flagOnce {
		handler.executed = &atomic.Bool{}
	}

	bus.mu.Lock()
	// Store handler in appropriate slice and save metadata with index in handlers map
	if handler.async {
		bus.asyncHandlers = append(bus.asyncHandlers, *handler)
		bus.handlers[id] = len(bus.asyncHandlers) - 1 // Storing zero-based index
	} else {
		bus.syncHandlers = append(bus.syncHandlers, *handler)
		bus.handlers[id] = len(bus.syncHandlers) - 1 // Storing zero-based index
	}
	bus.mu.Unlock()

	// Increment subscriber count
	bus.subscriberCount.Add(1)

	return id, nil
}

// WithAsync returns an option that makes handlers run asynchronously in the worker pool.
// Because this is a method on EventBus[T], type is inferred and no type parameter is needed!
//
// Example:
//
//	bus.Subscribe(handler, bus.WithAsync())
func (bus *EventBus[T]) WithAsync() SubscribeOption[T] {
	return types.WithAsync[T]()
}

// WithOnce returns an option that removes handlers after their first execution.
// Because this is a method on EventBus[T], type is inferred and no type parameter is needed!
//
// Example:
//
//	bus.Subscribe(handler, bus.WithOnce())
func (bus *EventBus[T]) WithOnce() SubscribeOption[T] {
	return types.WithOnce[T]()
}

// WithSerial returns an option that ensures only one instance of the handler
// can execute at a time. When multiple events are published concurrently, calls to the
// same handler will be serialized (queued). This is useful for handlers that must not
// run concurrently with themselves.
// Because this is a method on EventBus[T], type is inferred and no type parameter is needed!
//
// Example:
//
//	bus.Subscribe(handler, bus.WithSerial())                    // sync, serialized
//	bus.Subscribe(handler, bus.WithAsync(), bus.WithSerial())   // async, but serialized
func (bus *EventBus[T]) WithSerial() SubscribeOption[T] {
	return types.WithSerial[T]()
}

// WithTimeout returns an option that sets the maximum execution time for a handler.
// If the handler does not complete within the specified duration, it will be canceled.
// If timeout is 0 or negative, the handler uses the bus-level default timeout (if set).
// Because this is a method on EventBus[T], type is inferred and no type parameter is needed!
//
// Example:
//
//	bus.Subscribe(handler, bus.WithTimeout(5*time.Second))
//	bus.Subscribe(handler, bus.WithTimeout(500*time.Millisecond), bus.WithAsync())
func (bus *EventBus[T]) WithTimeout(timeout time.Duration) SubscribeOption[T] {
	return types.WithTimeout[T](timeout)
}

// SetDefaultTimeout sets or updates the default timeout for all handlers on this bus.
// This timeout applies to handlers that don't specify their own timeout.
// Handler-specific timeouts always take precedence over the bus default.
// This method is thread-safe and can be called at any time.
//
// Example:
//
//	bus.SetDefaultTimeout(5*time.Second)  // All handlers without timeout now use 5s
//	bus.SetDefaultTimeout(0)              // Disable default timeout
func (bus *EventBus[T]) SetDefaultTimeout(timeout time.Duration) {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.defaultTimeout = timeout
}

// GetDefaultTimeout returns the current default timeout for this bus.
// Returns 0 if no default timeout is set.
func (bus *EventBus[T]) GetDefaultTimeout() time.Duration {
	bus.mu.RLock()
	defer bus.mu.RUnlock()
	return bus.defaultTimeout
}

// Publish publishes the event to all subscribed subscriptions.
// Returns immediately if the bus has been closed via Close.
func (bus *EventBus[T]) Publish(ctx context.Context, event T) {
	// Check if bus is closed
	if bus.closed.Load() {
		return
	}

	// Increment published count
	bus.publishedCount.Add(1)

	// Fast path: NoHooks enabled for entire bus
	// Skip ALL hook-related operations for maximum performance
	if bus.noHooks {
		bus.publishNoHooks(ctx, event)
		return
	}

	// Normal path: Full hook support
	bus.publishWithHooks(ctx, event)
}

// publishNoHooks is the fast path when hooks are disabled for the entire bus.
// This skips:
//   - All hook copying
//   - All hook pool operations
//   - All hook execution
//
// Handlers are executed directly with minimal overhead.
func (bus *EventBus[T]) publishNoHooks(ctx context.Context, event T) {
	bus.mu.RLock()
	nSync := len(bus.syncHandlers)
	nAsync := len(bus.asyncHandlers)

	// We don't have any handler to process, return early
	if nSync == 0 && nAsync == 0 {
		bus.mu.RUnlock()
		return
	}

	// Get sync handler slice from pool
	var syncWrapper *subscriptionPool[T]
	if nSync > 0 {
		syncWrapper = bus.syncSlicePool.Get()
		if cap(syncWrapper.subscriptions) < nSync {
			syncWrapper.subscriptions = make([]subscription[T], nSync)
		} else {
			syncWrapper.subscriptions = syncWrapper.subscriptions[:nSync]
		}
		copy(syncWrapper.subscriptions, bus.syncHandlers)
	}

	// Get async handler slice from pool
	var asyncWrapper *subscriptionPool[T]
	if nAsync > 0 {
		asyncWrapper = bus.asyncSlicePool.Get()
		if cap(asyncWrapper.subscriptions) < nAsync {
			asyncWrapper.subscriptions = make([]subscription[T], nAsync)
		} else {
			asyncWrapper.subscriptions = asyncWrapper.subscriptions[:nAsync]
		}
		copy(asyncWrapper.subscriptions, bus.asyncHandlers)
	}

	bus.mu.RUnlock()

	// Execute async subscriptions (non-blocking in goroutine)
	if nAsync > 0 {
		// IMPORTANT: Add to WaitGroup BEFORE spawning goroutine to prevent race
		// with WaitAsync() being called immediately after Publish() returns
		bus.wg.Add(1)
		// Execute in a go-routine to avoid blocking the publish call
		// But because of function closure, variable will escape to heap
		go func() {
			defer bus.wg.Done()
			bus.executeAsyncHandlersNoHooks(ctx, event, asyncWrapper.subscriptions)
			// Reset and return to pool
			asyncWrapper.subscriptions = asyncWrapper.subscriptions[:0]
			bus.asyncSlicePool.Put(asyncWrapper)
		}()
	}

	// Execute sync subscriptions (blocking) - NO HOOKS
	if nSync > 0 {
		bus.executeSyncHandlersNoHooks(ctx, event, syncWrapper.subscriptions)
		// Reset and return to pool
		syncWrapper.subscriptions = syncWrapper.subscriptions[:0]
		bus.syncSlicePool.Put(syncWrapper)
	}
}

// publishWithHooks is the normal path with full hook support.
func (bus *EventBus[T]) publishWithHooks(ctx context.Context, event T) {
	bus.mu.RLock()
	nSync := len(bus.syncHandlers)
	nAsync := len(bus.asyncHandlers)

	// We don't have any handler to process, return early
	if nSync == 0 && nAsync == 0 {
		bus.mu.RUnlock()
		return
	}

	// Get sync handler slice from pool
	var syncWrapper *subscriptionPool[T]
	if nSync > 0 {
		syncWrapper = bus.syncSlicePool.Get()
		if cap(syncWrapper.subscriptions) < nSync {
			syncWrapper.subscriptions = make([]subscription[T], nSync)
		} else {
			syncWrapper.subscriptions = syncWrapper.subscriptions[:nSync]
		}
		copy(syncWrapper.subscriptions, bus.syncHandlers)
	}

	// Get async handler slice from pool
	var asyncWrapper *subscriptionPool[T]
	if nAsync > 0 {
		asyncWrapper = bus.asyncSlicePool.Get()
		if cap(asyncWrapper.subscriptions) < nAsync {
			asyncWrapper.subscriptions = make([]subscription[T], nAsync)
		} else {
			asyncWrapper.subscriptions = asyncWrapper.subscriptions[:nAsync]
		}
		copy(asyncWrapper.subscriptions, bus.asyncHandlers)
	}

	// Get hook wrappers from pools and copy bus-level hooks
	busBeforePool := bus.beforeHookPool.Get()
	busAfterPool := bus.afterHookPool.Get()
	busErrorPool := bus.errorHookPool.Get()

	// Ensure capacity and copy hooks
	if cap(busBeforePool.hooks) < len(bus.beforeHooks) {
		busBeforePool.hooks = make(BeforeHooks[T], len(bus.beforeHooks))
	} else {
		busBeforePool.hooks = busBeforePool.hooks[:len(bus.beforeHooks)]
	}
	copy(busBeforePool.hooks, bus.beforeHooks)

	if cap(busAfterPool.hooks) < len(bus.afterHooks) {
		busAfterPool.hooks = make(AfterHooks[T], len(bus.afterHooks))
	} else {
		busAfterPool.hooks = busAfterPool.hooks[:len(bus.afterHooks)]
	}
	copy(busAfterPool.hooks, bus.afterHooks)

	if cap(busErrorPool.hooks) < len(bus.errorHooks) {
		busErrorPool.hooks = make(ErrorHooks[T], len(bus.errorHooks))
	} else {
		busErrorPool.hooks = busErrorPool.hooks[:len(bus.errorHooks)]
	}
	copy(busErrorPool.hooks, bus.errorHooks)

	busBeforeHooks := busBeforePool.hooks
	busAfterHooks := busAfterPool.hooks
	busErrorHooks := busErrorPool.hooks

	bus.mu.RUnlock()

	// Execute async subscriptions (non-blocking in goroutine)
	if nAsync > 0 {
		// IMPORTANT: Add to WaitGroup BEFORE spawning goroutine to prevent race
		// with WaitAsync() being called immediately after Publish() returns
		bus.wg.Add(1)
		go func() {
			defer bus.wg.Done()
			bus.executeAsyncHandlers(ctx, event, asyncWrapper.subscriptions, busBeforeHooks, busAfterHooks, busErrorHooks)
			asyncWrapper.subscriptions = asyncWrapper.subscriptions[:0]
			bus.asyncSlicePool.Put(asyncWrapper)
		}()
	}

	// Execute sync subscriptions (blocking)
	if nSync > 0 {
		bus.executeSyncHandlers(ctx, event, syncWrapper.subscriptions, busBeforeHooks, busAfterHooks, busErrorHooks)
		syncWrapper.subscriptions = syncWrapper.subscriptions[:0]
		bus.syncSlicePool.Put(syncWrapper)
	}

	// Clear hook references and return to pools
	busBeforePool.hooks = busBeforePool.hooks[:0]
	bus.beforeHookPool.Put(busBeforePool)

	busAfterPool.hooks = busAfterPool.hooks[:0]
	bus.afterHookPool.Put(busAfterPool)

	busErrorPool.hooks = busErrorPool.hooks[:0]
	bus.errorHookPool.Put(busErrorPool)
}

// executeHandlerNoHooks executes a single handler WITHOUT any hooks (fast path).
// This is the common handler for both sync and async cases when hooks are disabled.
func (bus *EventBus[T]) executeHandlerNoHooks(
	ctx context.Context,
	event T,
	handler subscription[T],
) {
	// For Once handlers, check if already executed using atomic CAS
	// If this handler was already executed by another concurrent Publish call,
	// skip execution (lock-free approach)
	var shouldUnsubscribe bool
	if handler.flagOnce {
		// CompareAndSwap atomically checks if value is false and sets it to true
		// Returns true if swap succeeded (we won the race), false if already true
		if !handler.executed.CompareAndSwap(false, true) {
			// Another goroutine already executed this handler, skip
			return
		}
		// We won the race - proceed with execution and unsubscribe after
		shouldUnsubscribe = true
	}

	// Serialize execution if Serial flag is set and mutex pointer exists
	// This ensures only one instance of this handler executes at a time
	// The mutex is a pointer, so all copies of the subscription share the same lock
	if handler.serial && handler.mu != nil {
		handler.mu.Lock()
		defer handler.mu.Unlock()
	}

	// Apply timeout: handler-specific timeout takes precedence, then bus default
	var cancel context.CancelFunc
	timeout := handler.timeout
	if timeout == 0 && bus.defaultTimeout > 0 {
		timeout = bus.defaultTimeout
	}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Direct execution - no hooks
	_, err := handler.callback(ctx, event)

	// Track metrics
	bus.processedCount.Add(1)
	if err != nil {
		bus.errorCount.Add(1)
	}

	// If this was a Once handler and we executed it, unsubscribe it now
	if shouldUnsubscribe {
		bus.Unsubscribe(handler.id)
	}
}

// executeSyncHandlersNoHooks executes synchronous handlers WITHOUT any hooks (fast path)
// Note: Once handlers are already removed before this function is called
func (bus *EventBus[T]) executeSyncHandlersNoHooks(
	ctx context.Context,
	event T,
	handlers []subscription[T],
) {
	for _, h := range handlers {
		bus.executeHandlerNoHooks(ctx, event, h)
	}
}

// executeAsyncHandlersNoHooks executes asynchronous handlers WITHOUT any hooks (fast path)
// Bus lock shouldn't be held while calling this.
// Note: Once handlers are already removed before this function is called
func (bus *EventBus[T]) executeAsyncHandlersNoHooks(
	ctx context.Context,
	event T,
	handlers []subscription[T],
) {
	// Cannot return early, otherwise we'll be putting the slice back into the pool
	// which might still be in use.
	var currentWg sync.WaitGroup

	for _, h := range handlers {
		h := h // Capture loop variable for goroutine

		currentWg.Add(1)
		// As we're submitting to worker pool, all the variables used inside will escape to heap.
		_ = bus.workerPool.Submit(ctx, func() {
			defer currentWg.Done()

			bus.executeHandlerNoHooks(ctx, event, h)
		})
	}

	currentWg.Wait()
}

// executeHandler runs a single handler with before/after/error hooks.
// Handler-level hooks are read directly from the cloned handler struct.
// This is safe because Publish clones all handlers and hooks before releasing the lock.
// executeHandler runs a single handler with before/after/error hooks.
// Hooks are executed in this order:
//  1. Bus-level before hooks
//  2. Handler-level before hooks
//  3. Handler execution
//  4. Handler-level error hooks (if error)
//  5. Bus-level error hooks (if error)
//  6. Handler-level after hooks
//  7. Bus-level after hooks
func (bus *EventBus[T]) executeHandler(
	ctx context.Context,
	event T,
	handler subscription[T], // VALUE, not pointer
	busBeforeHooks BeforeHooks[T],
	busAfterHooks AfterHooks[T],
	busErrorHooks ErrorHooks[T],
) {
	// For Once handlers, check if already executed using atomic CAS
	// If this handler was already executed by another concurrent Publish call,
	// skip execution (lock-free approach)
	var shouldUnsubscribe bool
	if handler.flagOnce {
		// CompareAndSwap atomically checks if value is false and sets it to true
		// Returns true if swap succeeded (we won the race), false if already true
		if !handler.executed.CompareAndSwap(false, true) {
			// Another goroutine already executed this handler, skip
			return
		}
		// We won the race - proceed with execution and unsubscribe after
		shouldUnsubscribe = true
	}

	// Serialize execution if Serial flag is set and mutex pointer exists
	// This ensures only one instance of this handler executes at a time
	// Lock is acquired before hooks to ensure the entire execution (including hooks) is serialized
	// The mutex is a pointer, so all copies of the subscription share the same lock
	if handler.serial && handler.mu != nil {
		handler.mu.Lock()
		defer handler.mu.Unlock()
	}

	// Run bus-level before hooks (can modify context)
	for _, hook := range busBeforeHooks {
		if hook != nil {
			ctx = hook(ctx, event, handler.id)
		}
	}

	// Run handler-level before hooks (can modify context)
	for _, hook := range handler.beforeHooks {
		if hook != nil {
			ctx = hook(ctx, event, handler.id)
		}
	}

	// Apply timeout: handler-specific timeout takes precedence, then bus default
	var cancel context.CancelFunc
	timeout := handler.timeout
	if timeout == 0 && bus.defaultTimeout > 0 {
		timeout = bus.defaultTimeout
	}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Execute handler and measure duration
	start := time.Now()
	result, err := handler.callback(ctx, event)
	duration := time.Since(start)

	// Track metrics
	bus.processedCount.Add(1)
	if err != nil {
		bus.errorCount.Add(1)
	}

	// Run error hooks if handler failed (handler-level first, then bus-level)
	if err != nil {
		for _, hook := range handler.errorHooks {
			if hook != nil {
				hook(ctx, event, handler.id, err)
			}
		}
		for _, hook := range busErrorHooks {
			if hook != nil {
				hook(ctx, event, handler.id, err)
			}
		}
	}

	// Run after hooks (handler-level first, then bus-level)
	for _, hook := range handler.afterHooks {
		if hook != nil {
			hook(ctx, event, handler.id, result, duration, err)
		}
	}
	for _, hook := range busAfterHooks {
		if hook != nil {
			hook(ctx, event, handler.id, result, duration, err)
		}
	}

	// If this was a Once handler and we executed it, unsubscribe it now
	if shouldUnsubscribe {
		bus.Unsubscribe(handler.id)
	}
}

// executeSyncHandlers executes synchronous handlers (blocking)
// Note: Once handlers are already removed before this function is called
func (bus *EventBus[T]) executeSyncHandlers(
	ctx context.Context,
	event T,
	handlers []subscription[T],
	busBeforeHooks BeforeHooks[T],
	busAfterHooks AfterHooks[T],
	busErrorHooks ErrorHooks[T],
) {
	for _, h := range handlers {
		bus.executeHandler(ctx, event, h, busBeforeHooks, busAfterHooks, busErrorHooks)
	}
}

// executeAsyncHandlers executes asynchronous handlers (non-blocking in worker pool)
// Note: Once handlers are already removed before this function is called
func (bus *EventBus[T]) executeAsyncHandlers(
	ctx context.Context,
	event T,
	handlers []subscription[T],
	busBeforeHooks BeforeHooks[T],
	busAfterHooks AfterHooks[T],
	busErrorHooks ErrorHooks[T],
) {
	var currentWg sync.WaitGroup

	for _, h := range handlers {
		h := h // Capture loop variable for goroutine

		currentWg.Add(1)
		// Here also we might need something to avoid function closure leak to heap.
		_ = bus.workerPool.Submit(ctx, func() {
			defer currentWg.Done()
			bus.executeHandler(ctx, event, h, busBeforeHooks, busAfterHooks, busErrorHooks)
		})
	}

	currentWg.Wait()
}

// Unsubscribe removes a handler by its ID in O(1) time
func (bus *EventBus[T]) Unsubscribe(id SubscriptionID) bool {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	return bus.unsubscribeUnsafe(bus.isAsyncUnsafe(id), id)
}

// unsubscribeUnsafe removes a handler by its ID in O(1) time (caller must hold lock)
func (bus *EventBus[T]) unsubscribeUnsafe(isAsync bool, id SubscriptionID) bool {
	index, exists := bus.handlers[id]
	if !exists {
		return true
	}

	// O(1) removal using stored index
	if isAsync {
		lastIdx := len(bus.asyncHandlers) - 1
		if index != lastIdx {
			// Move last element to the removed position
			bus.asyncHandlers[index] = bus.asyncHandlers[lastIdx]
			// Update the moved handler's metadata with new index
			movedHandlerID := bus.asyncHandlers[index].id
			bus.handlers[movedHandlerID] = index
		}
		bus.asyncHandlers = bus.asyncHandlers[:lastIdx]
	} else {
		lastIdx := len(bus.syncHandlers) - 1
		if index != lastIdx {
			// Move last element to the removed position
			bus.syncHandlers[index] = bus.syncHandlers[lastIdx]
			// Update the moved handler's metadata with new index
			movedHandlerID := bus.syncHandlers[index].id
			bus.handlers[movedHandlerID] = index
		}
		bus.syncHandlers = bus.syncHandlers[:lastIdx]
	}

	delete(bus.handlers, id)

	// Decrement subscriber count
	bus.subscriberCount.Add(^uint64(0))

	return true
}

// UnsubscribeAll removes all subscriptions
func (bus *EventBus[T]) UnsubscribeAll() {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Reset subscriber count to zero
	bus.subscriberCount.Store(0)

	bus.handlers = make(map[SubscriptionID]int)
	bus.syncHandlers = bus.syncHandlers[:0]
	bus.asyncHandlers = bus.asyncHandlers[:0]
}

// HasSubscription returns true if there are any subscribed subscriptions
func (bus *EventBus[T]) HasSubscription() bool {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	return len(bus.syncHandlers) > 0 || len(bus.asyncHandlers) > 0
}

// WaitAsync blocks until all async subscriptions OF THIS BUS complete.
// This only waits for this bus's jobs, not the entire shared worker pool.
func (bus *EventBus[T]) WaitAsync() {
	bus.wg.Wait()
}

// GetWorkerPoolStats returns statistics about the worker pool.
// Returns (capacity, running, waiting) - capacity is the max workers,
// running is the number of currently executing workers,
// waiting is the number of tasks in queue.
// Returns (0, 0, 0) if the pool doesn't support stats.
func (bus *EventBus[T]) GetWorkerPoolStats() (capacity, running, waiting int) {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	if bus.workerPool == nil {
		return 0, 0, 0
	}

	// We don't need type assertions as we are initiating the bus with Pool
	return bus.workerPool.Cap(), bus.workerPool.Running(), bus.workerPool.Waiting()
}

// Metrics returns the current metrics for this EventBus instance.
// All counters are atomic and can be safely read concurrently.
//
// Metrics include:
//   - PublishedCount: Total events published to this bus
//   - SubscriberCount: Current number of active subscribers
//   - ProcessedCount: Total handler invocations (successful + errors)
//   - ErrorCount: Total handler errors
//
// Example:
//
//	metrics := bus.Metrics()
//	log.Info("Bus stats",
//	    "published", metrics.PublishedCount,
//	    "subscribers", metrics.SubscriberCount,
//	    "processed", metrics.ProcessedCount,
//	    "errors", metrics.ErrorCount,
//	)
func (bus *EventBus[T]) Metrics() types.BusMetrics {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	return types.BusMetrics{
		PublishedCount:  bus.publishedCount.Load(),
		SubscriberCount: bus.subscriberCount.Load(),
		ProcessedCount:  bus.processedCount.Load(),
		ErrorCount:      bus.errorCount.Load(),
	}
}

// =============================================================================
// Individual Metric Getters
// =============================================================================

// PublishedCount returns the total number of events published to this bus.
func (bus *EventBus[T]) PublishedCount() uint64 {
	return bus.publishedCount.Load()
}

// SubscriberCount returns the current number of active subscribers.
func (bus *EventBus[T]) SubscriberCount() uint64 {
	return bus.subscriberCount.Load()
}

// ProcessedCount returns the total number of handler invocations.
func (bus *EventBus[T]) ProcessedCount() uint64 {
	return bus.processedCount.Load()
}

// ErrorCount returns the total number of handler errors.
func (bus *EventBus[T]) ErrorCount() uint64 {
	return bus.errorCount.Load()
}

// =============================================================================
// Lifecycle Management
// =============================================================================

// IsClosed returns true if the EventBus has been closed
func (bus *EventBus[T]) IsClosed() bool {
	return bus.closed.Load()
}

// Close gracefully shuts down the EventBus.
// It waits for all pending async subscriptions OF THIS BUS to complete.
// After closing, the bus is marked as closed and will reject new Publish calls.
// Returns nil if all subscriptions completed, or ctx.Err() if the context was canceled.
func (bus *EventBus[T]) Close(ctx context.Context) (closeErr error) {
	bus.shutdownOnce.Do(func() {
		bus.closed.Store(true)

		// Determine shutdown timeout
		timeout := bus.shutdownTimeout
		if timeout <= 0 {
			timeout = defaultShutdownTimeout
		}

		// Wait for THIS bus's pending async work to complete
		done := make(chan struct{})
		go func() {
			bus.wg.Wait()
			close(done)
		}()

		// Use timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		select {
		case <-done:
			// All async work completed
			closeErr = nil
		case <-timeoutCtx.Done():
			// Timeout or context cancellation
			closeErr = timeoutCtx.Err()
		}

		// Shut down the worker pool if this bus owns it
		if bus.ownsWorkerPool && bus.workerPool != nil {
			if err := bus.workerPool.ShutdownWithTimeout(timeout); err != nil {
				closeErr = multierr.Combine(closeErr, err)
			}
		}
	})

	return
}

// =============================================================================
// Helper functions
// =============================================================================

// isAsyncUnsafe determines whether a subscription is asynchronous based on its ID.
// This method is used internally during unsubscribe operations to identify
// which handler slice (syncHandlers or asyncHandlers) contains the subscription.
//
// The method performs a fast lookup using the handlers map to get the index,
// then verifies if that index points to the subscription in the asyncHandlers slice
// by checking both the index bounds and matching the subscription ID.
//
// Returns:
//   - true: if the subscription exists and is in the asyncHandlers slice
//   - false: if the subscription doesn't exist, or is in the syncHandlers slice
//
// Note: This method assumes the caller holds the appropriate lock (mu).
// It's called from unsubscribeUnsafe which is invoked with the lock held.
func (bus *EventBus[T]) isAsyncUnsafe(id SubscriptionID) (isAsync bool) {
	// Get the stored index for this subscription ID
	tempIndex, exists := bus.handlers[id]

	if !exists {
		return false
	}

	// Check if the index points to this subscription in the asyncHandlers slice
	// We verify both the index is within bounds AND the ID matches to ensure correctness
	if tempIndex < len(bus.asyncHandlers) && bus.asyncHandlers[tempIndex].id == id {
		return true
	}

	return false
}
