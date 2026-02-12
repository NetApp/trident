// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package types defines the core interfaces and types for the EventBus system.
// This package provides a generic, type-safe event bus interface that can be
// implemented by various concrete implementations.
package types

//go:generate mockgen -destination=../../../mocks/mock_pkg/mock_eventbus/mock_eventbus.go -package=mock_eventbus github.com/netapp/trident/pkg/eventbus/types EventBus,EventBusWithOptions,EventBusWithHooks,EventBusWithMetrics,EventBusHooksOptions,EventBusMetricsOptions,EventBusHooksMetrics,EventBusHooksMetricsOptions

import (
	"context"
	"slices"
	"time"

	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
)

// SubscriptionID is a unique identifier for an event handler subscription.
// Each subscriber receives a unique ID when they subscribe to an event bus.
type SubscriptionID uint64

// Result is the return type from event subscriptions, allowing them to pass
// arbitrary data to after hooks (e.g., for logging, metrics, or chaining).
// Handlers can populate this map with key-value pairs that hooks can inspect.
// Result can be marshaled to JSON using json.Marshal() or converted to any struct
// using json.Marshal() followed by json.Unmarshal() into the target struct.
type Result map[string]any

// SubscriptionFunc is a callback function that receives context and event data.
// It returns a Result (for passing data to after hooks) and an error.
// The error can be observed by error hooks for logging, metrics.
//
// Example:
//
//		func handleUserCreated(ctx context.Context, user User) (Result, error) {
//		    if err := sendEmail(user.Email); err != nil {
//		        return nil, err
//		    }
//		    return Result{"emailSent": true}, nil
//		}
//
//	 In Trident context:
//	 func handleVolumesScheduled(ctx context.Context, event VolumesScheduled) (Result, error) {
//		// Enqueue volumes to the work queue
//		   	enqueued, skipped, err := p.EnqueueEvent(eventCtx, event.TVPNames)
//			return eventbusTypes.Result{
//				"eventID":  event.ID,
//				"enqueued": enqueued,
//				"skipped":  skipped,
//			}, nil
//		}
type SubscriptionFunc[T any] func(ctx context.Context, event T) (Result, error)

// =============================================================================
// Subscription Options
// =============================================================================

// SubscribeOption is a functional option for configuring a subscription.
// Options modify the SubscriptionOptions struct.
type SubscribeOption[T any] func(*SubscriptionOptions[T])

// SubscriptionOptions holds configuration for a subscription.
// This struct is populated by functional options passed to Subscribe().
type SubscriptionOptions[T any] struct {
	// Async determines if the handler runs asynchronously in the worker pool
	Async bool

	// Once determines if the handler should be removed after first execution
	Once bool

	// Serial determines if only one instance of this handler can execute at a time.
	// When true, concurrent calls to the same handler will be serialized (queued).
	// This is useful for handlers that must not run concurrently with themselves.
	Serial bool

	// Timeout specifies the maximum duration for handler execution.
	// If zero, the handler runs indefinitely (no timeout).
	// If positive, a timeout context is created for the handler execution.
	Timeout time.Duration

	// BeforeHooks are hooks that run before this specific handler executes
	BeforeHooks BeforeHooks[T]

	// AfterHooks are hooks that run after this specific handler completes
	AfterHooks AfterHooks[T]

	// ErrorHooks are hooks that run when this specific handler returns an error
	ErrorHooks ErrorHooks[T]
}

// =============================================================================
// Subscription Option Builders
// =============================================================================

// WithOnce creates a subscription option that removes the handler after its first execution.
// The handler will be automatically unsubscribed after processing one event.
func WithOnce[T any]() SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.Once = true
	}
}

// WithAsync creates a subscription option that runs the handler asynchronously in the worker pool.
// The Publish() call will not block waiting for this handler to complete.
func WithAsync[T any]() SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.Async = true
	}
}

// WithSerial creates a subscription option that ensures only one instance of the handler
// can execute at a time. When multiple events are published concurrently, calls to the
// same handler will be serialized (queued). This is useful for handlers that must not
// run concurrently with themselves (e.g., handlers that update shared state or external resources).
//
// Example:
//
//	bus.Subscribe(handler, bus.WithSerial())                    // sync, serialized
//	bus.Subscribe(handler, bus.WithAsync(), bus.WithSerial())   // async, but serialized per handler
func WithSerial[T any]() SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.Serial = true
	}
}

// WithTimeout creates a subscription option that sets the maximum execution time for a handler.
// If timeout is 0 or negative, the handler uses the bus-level default timeout (if set).
func WithTimeout[T any](timeout time.Duration) SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.Timeout = timeout
	}
}

// WithBefore creates a SubscribeOption that attaches before hooks to a handler.
//
// Example:
//
// bus.Subscribe(handler,
//
//	    bus.WithBefore(func(ctx context.Context, event User) context.Context {
//	        return context.WithValue(ctx, "traceID", uuid.New())
//	    }),
//	)
func WithBefore[T any](hooks ...BeforeHook[T]) SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.BeforeHooks = BeforeHooks[T](hooks).Copy()
	}
}

// WithAfter creates a SubscribeOption that attaches after hooks to a handler.
//
// Example:
//
//	bus.Subscribe(handler,
//	    bus.WithAfter(func(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
//	        metrics.RecordDuration("handler_duration", duration)
//	    }),
//	)
func WithAfter[T any](hooks ...AfterHook[T]) SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.AfterHooks = AfterHooks[T](hooks).Copy()
	}
}

// WithOnError creates a SubscribeOption that attaches error hooks to a handler.
//
// Example:
//
//	bus.Subscribe(handler,
//	    bus.WithOnError(func(ctx context.Context, event User, id SubscriptionID, err error) {
//	        log.Error("Handler failed", "error", err)
//	        alertOncall("Handler failure", err)
//	    }),
//	)
func WithOnError[T any](hooks ...ErrorHook[T]) SubscribeOption[T] {
	return func(opts *SubscriptionOptions[T]) {
		opts.ErrorHooks = ErrorHooks[T](hooks).Copy()
	}
}

// =============================================================================
// Hook Types
// =============================================================================

// BeforeHook is called before a handler executes.
// It can modify the context (e.g., add tracing span, inject values).
// The returned context is passed to the handler and subsequent hooks.
//
// Example:
//
//	func tracingHook(ctx context.Context, event User) context.Context {
//	    span := startSpan("handle_user_created")
//	    return context.WithValue(ctx, "span", span)
//	}
type BeforeHook[T any] func(ctx context.Context, event T, handlerID SubscriptionID) context.Context

// AfterHook is called after a handler executes (success or failure).
// It receives the result, duration, and any error returned by the handler.
// Use for metrics, logging, or cleanup operations.
//
// Example:
//
//	func metricsHook(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
//	    metrics.RecordDuration("user_created_handler", duration)
//	    if err != nil {
//	        metrics.Increment("user_created_errors")
//	    }
//	}
type AfterHook[T any] func(ctx context.Context, event T, handlerID SubscriptionID, result Result, duration time.Duration, err error)

// ErrorHook is called only when a handler returns an error.
// Use for logging, metrics, alerting, or dead-letter queue handling.
//
// Example:
//
//	func errorAlertHook(ctx context.Context, event User, id SubscriptionID, err error) {
//	    log.Error("Handler failed", "error", err, "user", event.ID)
//	    sendAlert("Handler failure", err)
//	}
type ErrorHook[T any] func(ctx context.Context, event T, handlerID SubscriptionID, err error)

// =============================================================================
// Hook Slice Types (with clone methods for thread-safe copying)
// =============================================================================

// BeforeHooks is a slice of BeforeHook with a copy method for thread-safe copying.
type BeforeHooks[T any] []BeforeHook[T]

// Copy creates a shallow copy of the hook slice (new backing array).
// Uses slices.Clone() which efficiently creates a new slice with copied elements.
// Since hooks are function pointers, shallow copy is sufficient.
func (h BeforeHooks[T]) Copy() BeforeHooks[T] {
	return slices.Clone(h)
}

// AfterHooks is a slice of AfterHook with a copy method for thread-safe copying.
type AfterHooks[T any] []AfterHook[T]

// Copy creates a shallow copy of the hook slice (new backing array).
// Uses slices.Clone() which efficiently creates a new slice with copied elements.
// Since hooks are function pointers, shallow copy is sufficient.
func (h AfterHooks[T]) Copy() AfterHooks[T] {
	return slices.Clone(h)
}

// ErrorHooks is a slice of ErrorHook with a copy method for thread-safe copying.
type ErrorHooks[T any] []ErrorHook[T]

// Copy creates a shallow copy of the hook slice (new backing array).
// Uses slices.Clone() which efficiently creates a new slice with copied elements.
// Since hooks are function pointers, shallow copy is sufficient.
func (h ErrorHooks[T]) Copy() ErrorHooks[T] {
	return slices.Clone(h)
}

// =============================================================================
// EventBus Interface
// =============================================================================

// Interface Hierarchy Overview:
//
// The EventBus system provides a composable interface hierarchy that allows
// implementations to pick and choose which features to support:
//
// Base Interface:
//   EventBus[T] - Core functionality (Subscribe, Publish, Close, WithAsync)
//
// Individual Feature Interfaces:
//   EventBusWithOptions[T]  - Adds: WithOnce, WithSerial, WithTimeout
//   EventBusWithHooks[T]    - Adds: Before/After/Error hooks (global + per-handler)
//   EventBusWithMetrics[T]  - Adds: Runtime metrics (counters, stats)
//
// Combined Feature Interfaces:
//   EventBusHooksOptions[T]      - Hooks + Options (no metrics)
//   EventBusMetricsOptions[T]    - Metrics + Options (no hooks)
//   EventBusHooksMetricsOnly[T]  - Hooks + Metrics (no options)
//   EventBusHooksMetrics[T]      - ALL features (options + hooks + metrics)
//
// This design allows:
//   - Minimal implementations: Implement only EventBus[T]
//   - Feature-specific implementations: Add only the interfaces you need
//   - Full-featured implementations: Implement EventBusHooksMetrics[T]
//   - Graceful degradation: Code can check for features via type assertion
//
// =============================================================================

// EventBus represents a typed event bus that handlers can subscribe to.
// Unlike topic-based event buses, each EventBus is a standalone variable that can be
// easily navigated in IDEs (Find Usages, Go to Definition, etc.).
//
// The interface is generic over type T, ensuring compile-time type safety.
// Handlers are executed either synchronously (blocking) or asynchronously (non-blocking)
// based on subscription options.
//
// Thread Safety:
//   - All methods are safe for concurrent use
//   - Publish can be called while Subscribe/Unsubscribe are running
//   - Multiple goroutines can Publish simultaneously
//
// Example Usage:
//
//	// Create a typed event bus
//	bus, _ := simple.New[User](ctx, simple.DefaultConfig())
//
//	// Basic usage (EventBus interface)
//	id, _ := bus.Subscribe(sendWelcomeEmail, bus.WithAsync())
//	bus.Publish(ctx, user)
//
//	// Check for full feature support
//	if fullBus, ok := bus.(EventBusHooksMetrics[User]); ok {
//	    // All features available: options + hooks + metrics
//	    fullBus.Subscribe(handler,
//	        fullBus.WithSerial(),
//	        fullBus.WithTimeout(5*time.Second),
//	        fullBus.WithAfter(metricsHook),
//	    )
//	    metrics := fullBus.Metrics()
//	    log.Info("Full-featured bus", "subscribers", metrics.SubscriberCount)
//	}
//
//	// Or check for specific feature combinations
//	if hooksWithOpts, ok := bus.(EventBusHooksOptions[User]); ok {
//	    // Hooks + Options (no metrics)
//	    hooksWithOpts.Subscribe(handler,
//	        hooksWithOpts.WithSerial(),
//	        hooksWithOpts.WithBefore(tracingHook),
//	    )
//	}
//
//	if metricsWithOpts, ok := bus.(EventBusMetricsOptions[User]); ok {
//	    // Metrics + Options (no hooks)
//	    metricsWithOpts.Subscribe(handler, metricsWithOpts.WithSerial())
//	    metrics := metricsWithOpts.Metrics()
//	    log.Info("Metrics", "processed", metrics.ProcessedCount)
//	}
//
//	if hooksMetrics, ok := bus.(EventBusHooksMetricsOnly[User]); ok {
//	    // Hooks + Metrics (no options)
//	    hooksMetrics.Subscribe(handler, hooksMetrics.WithAfter(logHook))
//	    hooksMetrics.SetGlobalErrorHook(alertHook)
//	    metrics := hooksMetrics.Metrics()
//	}
//
//	// Or check for individual features
//	if busWithOptions, ok := bus.(EventBusWithOptions[User]); ok {
//	    busWithOptions.Subscribe(handler, busWithOptions.WithSerial())
//	}
//
//	if busWithMetrics, ok := bus.(EventBusWithMetrics[User]); ok {
//	    metrics := busWithMetrics.Metrics()
//	    log.Info("Bus metrics", "subscribers", metrics.SubscriberCount)
//	}
//
//	if busWithHooks, ok := bus.(EventBusWithHooks[User]); ok {
//	    busWithHooks.SetErrorHook(id, alertOncall)
//	}
//
//	// Cleanup
//	bus.Close(ctx)
type EventBus[T any] interface {
	// =============================================================================
	// Subscription Management
	// =============================================================================

	// Subscribe adds an event handler and returns its unique ID.
	// By default, subscriptions are synchronous and persistent.
	// Use options to modify behavior:
	//   - bus.WithAsync(): run in worker pool (non-blocking)
	//
	// Additional options (if the bus implements EventBusWithOptions):
	//   - bus.WithOnce(): remove handler after first execution
	//   - bus.WithSerial(): ensure only one instance executes at a time
	//   - bus.WithTimeout(): adds context with deadline, which will be passed down to the handler
	//
	// Hook options (if the bus implements EventBusWithHooks):
	//   - bus.WithBefore()/WithAfter()/WithOnError(): attach hooks respectively
	//
	// Returns an error if the bus has been closed, or if fn is nil.
	//
	// Examples:
	//
	//	bus.Subscribe(handler)                                   // sync, persistent
	//	bus.Subscribe(handler, bus.WithAsync())                  // async
	//
	//	// With EventBusWithOptions:
	//	bus.Subscribe(handler, bus.WithOnce())                   // one-time
	//	bus.Subscribe(handler, bus.WithSerial())                 // serialized execution
	//	bus.Subscribe(handler, bus.WithTimeout(5*time.Second))   // with timeout
	//	bus.Subscribe(handler, bus.WithOnce(), bus.WithAsync())  // combine
	Subscribe(fn SubscriptionFunc[T], opts ...SubscribeOption[T]) (SubscriptionID, error)

	// WithAsync returns an option that makes handlers run asynchronously in the worker pool.
	//
	// Implementation Note: Should delegate to types.WithAsync[T]()
	//
	// Example:
	//
	//	bus.Subscribe(handler, bus.WithAsync())
	WithAsync() SubscribeOption[T]

	// Unsubscribe removes a handler by its ID.
	// Returns true if the handler was found and removed, false otherwise.
	// This is an idempotent operation - calling it multiple times is safe.
	//
	// Time Complexity: O(1)
	//
	// Example:
	//
	//	id, _ := bus.Subscribe(handler)
	//	// ... later ...
	//	bus.Unsubscribe(id)
	Unsubscribe(id SubscriptionID) bool

	// UnsubscribeAll removes all subscriptions from the bus.
	// This is useful for cleanup or testing scenarios.
	//
	// Example:
	//
	//	bus.UnsubscribeAll()
	UnsubscribeAll()

	// HasSubscription returns true if there are any subscribed handlers.
	// This can be used to avoid publishing events when no one is listening.
	//
	// Example:
	//
	//	if bus.HasSubscription() {
	//	    bus.Publish(ctx, event)
	//	}
	HasSubscription() bool

	// =============================================================================
	// Event Publishing
	// =============================================================================

	// Publish publishes the event to all subscribed handlers.
	// Synchronous handlers are executed in the current goroutine (blocking).
	// Asynchronous handlers are executed in the worker pool (non-blocking).
	//
	// Returns immediately if the bus has been closed via Close.
	//
	// If the bus supports hooks (implements EventBusWithHooks), hook execution order is:
	//  1. Bus-level before hooks
	//  2. Handler-level before hooks
	//  3. Handler execution
	//  4. Error hooks (if error occurred)
	//  5. Handler-level after hooks
	//  6. Bus-level after hooks
	//
	// Example:
	//
	//	bus.Publish(ctx, User{ID: 123, Name: "Alice"})
	Publish(ctx context.Context, event T)

	// =============================================================================
	// Lifecycle Management
	// =============================================================================

	// WaitAsync blocks until all async handlers OF THIS BUS complete.
	// This only waits for this bus's jobs, not the entire shared worker pool.
	//
	// Example:
	//
	//	bus.Publish(ctx, event)
	//	bus.WaitAsync()  // Wait for all async handlers to complete
	WaitAsync()

	// Close gracefully shuts down the EventBus.
	// It waits for all pending async handlers to complete, respecting the context timeout.
	// After Close returns, the bus is marked as closed and will reject new Publish and Subscribe calls.
	// Returns nil if all handlers completed, or ctx.Err() if the context was cancelled.
	//
	// Note: This only waits for this bus's jobs, not the entire worker pool if shared.
	//
	// Example:
	//
	//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//	defer cancel()
	//	if err := bus.Close(ctx); err != nil {
	//	    log.Error("Failed to close bus", "error", err)
	//	}
	//
	// Defer pattern:
	//
	//	defer bus.Close(context.Background())
	Close(ctx context.Context) error

	// IsClosed returns true if the EventBus has been closed.
	//
	// Example:
	//
	//	if bus.IsClosed() {
	//	    log.Warn("Bus is closed, cannot publish")
	//	}
	IsClosed() bool

	// =============================================================================
	// Monitoring & Statistics
	// =============================================================================

	// GetWorkerPoolStats returns statistics about the worker pool.
	// Returns (capacity, running, waiting) where:
	//   - capacity: max number of workers
	//   - running: number of currently executing workers
	//   - waiting: number of tasks in queue
	// Returns (0, 0, 0) if the pool doesn't support stats.
	//
	// Example:
	//
	//	cap, running, waiting := bus.GetWorkerPoolStats()
	//	log.Info("Pool stats", "capacity", cap, "running", running, "waiting", waiting)
	GetWorkerPoolStats() (capacity, running, waiting int)
}

// =============================================================================
// Options Support Interface
// =============================================================================

// EventBusWithOptions is an interface that indicates an EventBus implementation
// supports advanced subscription options: Once, Serial, and Timeout.
//
// Not all EventBus implementations need to support these options. For example:
//   - A simple implementation might not support Serial (per-handler serialization)
//   - A high-performance implementation might skip Timeout to avoid context overhead
//   - A minimal implementation might only support basic Async mode
//
// Implementations that satisfy this interface provide:
//   - WithOnce(): One-time handler execution
//   - WithSerial(): Per-handler serialization (prevent concurrent execution)
//   - WithTimeout(): Per-handler execution timeouts
//
// These options can be combined with WithAsync() from the base EventBus interface.
//
// Example usage (type assertion):
//
//	if busWithOpts, ok := bus.(EventBusWithOptions[User]); ok {
//	    // This implementation supports advanced options
//	    busWithOpts.Subscribe(handler,
//	        busWithOpts.WithSerial(),
//	        busWithOpts.WithTimeout(5*time.Second),
//	    )
//	} else {
//	    // Fallback: use basic subscription
//	    bus.Subscribe(handler, bus.WithAsync())
//	}
//
// Example usage (type switch):
//
//	switch b := bus.(type) {
//	case EventBusWithOptions[User]:
//	    b.Subscribe(handler, b.WithOnce(), b.WithSerial())
//	default:
//	    // Use basic EventBus without advanced options
//	    bus.Subscribe(handler)
//	}
type EventBusWithOptions[T any] interface {
	EventBus[T]

	// =============================================================================
	// Advanced Subscription Options
	// =============================================================================
	//
	// Implementation Note:
	// These methods should be implemented by delegating to the corresponding functions
	// in this package (types.WithOnce[T](), types.WithSerial[T](), types.WithTimeout[T]()).
	// This avoids code duplication across different EventBus implementations.
	//
	// Example implementation:
	//
	//	func (bus *EventBus[T]) WithOnce() SubscribeOption[T] {
	//	    return types.WithOnce[T]()
	//	}
	//
	// This pattern provides:
	//   - Clean API for users: bus.WithOnce() (type inferred, no type parameters)
	//   - Shared implementation: types.WithOnce[T]() (reusable across implementations)
	//   - No code duplication: all implementations call the same underlying function

	// WithOnce returns an option that removes handlers after their first execution.
	// The handler will be automatically unsubscribed after processing one event.
	//
	// Implementation Note: Should delegate to types.WithOnce[T]()
	//
	// Example:
	//
	//	bus.Subscribe(handler, bus.WithOnce())
	//	bus.Subscribe(handler, bus.WithOnce(), bus.WithAsync())  // one-time, async
	WithOnce() SubscribeOption[T]

	// WithSerial returns an option that ensures only one instance of the handler
	// can execute at a time. When multiple events are published concurrently, calls to the
	// same handler will be serialized (queued). This is useful for handlers that must not
	// run concurrently with themselves (e.g., handlers that update shared state or external resources).
	//
	// Implementation Note: Should delegate to types.WithSerial[T]()
	//
	// Example:
	//
	//	bus.Subscribe(handler, bus.WithSerial())                    // sync, serialized
	//	bus.Subscribe(handler, bus.WithAsync(), bus.WithSerial())   // async, but serialized per handler
	WithSerial() SubscribeOption[T]

	// WithTimeout returns an option that sets the maximum execution time for a handler.
	// The timeout creates a context deadline that handlers should respect. The EventBus
	// does not forcibly terminate handlers; it is the handler's responsibility to honor
	// the context deadline by checking for cancellation signals.
	// If timeout is 0 or negative, the handler uses the bus-level default timeout (if set).
	//
	// Implementation Note: Should delegate to types.WithTimeout[T](timeout)
	//
	// Example:
	//
	//	bus.Subscribe(handler, bus.WithTimeout(5*time.Second))
	//	bus.Subscribe(handler, bus.WithTimeout(500*time.Millisecond), bus.WithAsync())
	WithTimeout(timeout time.Duration) SubscribeOption[T]

	// =============================================================================
	// Bus-Level Timeout Management
	// =============================================================================

	// SetDefaultTimeout sets or updates the default timeout for all handlers on this bus.
	// This timeout applies to handlers that don't specify their own timeout via WithTimeout.
	// Handler-specific timeouts always take precedence over the bus default.
	// This method should be thread-safe and can be called at any time.
	//
	// Setting timeout to 0 disables the default timeout.
	//
	// Example:
	//
	//	bus.SetDefaultTimeout(5*time.Second)  // All handlers without timeout now use 5s
	//	bus.SetDefaultTimeout(0)              // Disable default timeout
	SetDefaultTimeout(timeout time.Duration)

	// GetDefaultTimeout returns the current default timeout for this bus.
	// Returns 0 if no default timeout is set.
	//
	// Example:
	//
	//	timeout := bus.GetDefaultTimeout()
	//	if timeout > 0 {
	//	    fmt.Printf("Default timeout: %v\n", timeout)
	//	}
	GetDefaultTimeout() time.Duration
}

// =============================================================================
// Hook Support Interface
// =============================================================================

// EventBusWithHooks is a marker interface that indicates an EventBus implementation
// supports the hook system (before, after, and error hooks).
//
// Implementations that satisfy this interface provide:
//   - Global (bus-level) hooks that apply to all handlers
//   - Handler-level hooks that apply to specific subscriptions
//   - Runtime hook enable/disable control via DisableHooks/EnableHooks
//
// Not all EventBus implementations need to support hooks. For example, a high-performance
// implementation might choose to skip hooks entirely for better throughput.
//
// Example usage (type assertion):
//
//	if busWithHooks, ok := bus.(EventBusWithHooks[User]); ok {
//	    // This implementation supports hooks
//	    busWithHooks.SetGlobalAfterHook(metricsHook)
//	} else {
//	    // Fallback: this implementation doesn't support hooks
//	    log.Warn("Bus doesn't support hooks, metrics disabled")
//	}
//
// Example usage (type switch):
//
//	switch b := bus.(type) {
//	case EventBusWithHooks[User]:
//	    b.SetGlobalBeforeHook(tracingHook)
//	    b.SetGlobalErrorHook(alertHook)
//	default:
//	    log.Info("Using hook-free bus for maximum performance")
//	}
type EventBusWithHooks[T any] interface {
	EventBus[T]

	// =============================================================================
	// Global (Bus-Level) Hooks
	// =============================================================================

	// SetGlobalBeforeHook sets bus-level before hooks that apply to ALL handlers.
	// These hooks are called before any handler executes, allowing you to modify
	// the context (e.g., add tracing spans, inject request IDs, start timers).
	//
	// The hooks are called in the order they are provided. Each hook receives the
	// context returned by the previous hook, forming a chain.
	//
	// Nil hooks are filtered out automatically.
	// To clear hooks, call with no arguments or only nil values: SetGlobalBeforeHook() or SetGlobalBeforeHook(nil)
	//
	// Calling this method replaces any previously set global before hooks.
	//
	// Example:
	//
	//	bus.SetGlobalBeforeHook(
	//	    func(ctx context.Context, event User) context.Context {
	//	        // Add trace span for all handlers
	//	        span := startSpan("user_event_handler")
	//	        return context.WithValue(ctx, "span", span)
	//	    },
	//	    func(ctx context.Context, event User) context.Context {
	//	        // Add request ID
	//	        return context.WithValue(ctx, "requestID", uuid.New())
	//	    },
	//	)
	SetGlobalBeforeHook(hooks ...BeforeHook[T])

	// SetGlobalAfterHook sets bus-level after hooks that apply to ALL handlers.
	// These hooks are called after any handler completes (whether successful or not),
	// making them ideal for metrics, logging, cleanup, or resource release.
	//
	// After hooks receive the handler's result, execution duration, and any error
	// that occurred. They are called in the order provided.
	//
	// Nil hooks are filtered out automatically.
	// To clear hooks, call with no arguments or only nil values: SetGlobalAfterHook() or SetGlobalAfterHook(nil)
	//
	// Calling this method replaces any previously set global after hooks.
	//
	// Example:
	//
	//	bus.SetGlobalAfterHook(
	//	    func(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
	//	        // Record metrics for all handlers
	//	        metrics.RecordDuration("user_handler_duration", duration)
	//	        if err != nil {
	//	            metrics.Increment("user_handler_errors")
	//	        }
	//	    },
	//	    func(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
	//	        // Log completion
	//	        log.Info("Handler completed", "id", id, "duration", duration, "error", err)
	//	    },
	//	)
	SetGlobalAfterHook(hooks ...AfterHook[T])

	// SetGlobalErrorHook sets bus-level error hooks that apply to ALL handlers.
	// These hooks are called only when a handler returns an error, making them
	// ideal for error logging, alerting, dead-letter queues, or retry logic.
	//
	// Error hooks are called in the order provided. They receive the event data,
	// handler ID, and the error that occurred.
	//
	// Nil hooks are filtered out automatically.
	// To clear hooks, call with no arguments or only nil values: SetGlobalErrorHook() or SetGlobalErrorHook(nil)
	//
	// Calling this method replaces any previously set global error hooks.
	//
	// Example:
	//
	//	bus.SetGlobalErrorHook(
	//	    func(ctx context.Context, event User, id SubscriptionID, err error) {
	//	        // Log all handler errors
	//	        log.Error("Handler failed", "id", id, "error", err, "user", event.ID)
	//	    },
	//	    func(ctx context.Context, event User, id SubscriptionID, err error) {
	//	        // Alert on-call if critical
	//	        if isCritical(err) {
	//	            alertOncall("Critical handler failure", err)
	//	        }
	//	    },
	//	)
	SetGlobalErrorHook(hooks ...ErrorHook[T])

	// =============================================================================
	// Handler-Specific Hooks
	// =============================================================================

	// SetBeforeHook sets before hooks for a specific handler identified by its subscription ID.
	// These hooks only apply to the specified handler, allowing fine-grained control.
	//
	// Handler-level hooks are called after global before hooks but before the handler executes.
	// The hooks are called in the order provided.
	//
	// Nil hooks are filtered out automatically.
	// To clear hooks, call with no arguments or only nil values: SetBeforeHook(id) or SetBeforeHook(id, nil)
	//
	// Returns true if the handler was found and hooks were set, false if the handler
	// doesn't exist (e.g., already unsubscribed).
	//
	// Calling this method replaces any previously set before hooks for this handler.
	//
	// Example:
	//
	//	id, _ := bus.Subscribe(sendWelcomeEmail)
	//	bus.SetBeforeHook(id,
	//	    func(ctx context.Context, event User) context.Context {
	//	        // Add handler-specific context
	//	        return context.WithValue(ctx, "emailType", "welcome")
	//	    },
	//	)
	SetBeforeHook(id SubscriptionID, hooks ...BeforeHook[T]) bool

	// SetAfterHook sets after hooks for a specific handler identified by its subscription ID.
	// These hooks only apply to the specified handler, allowing fine-grained observability.
	//
	// Handler-level after hooks are called after the handler completes but before
	// global after hooks. The hooks are called in the order provided.
	//
	// Nil hooks are filtered out automatically.
	// To clear hooks, call with no arguments or only nil values: SetAfterHook(id) or SetAfterHook(id, nil)
	//
	// Returns true if the handler was found and hooks were set, false if the handler
	// doesn't exist (e.g., already unsubscribed).
	//
	// Calling this method replaces any previously set after hooks for this handler.
	//
	// Example:
	//
	//	id, _ := bus.Subscribe(sendWelcomeEmail)
	//	bus.SetAfterHook(id,
	//	    func(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
	//	        // Track email send metrics
	//	        if err == nil && result["emailSent"] == true {
	//	            metrics.Increment("welcome_emails_sent")
	//	        }
	//	    },
	//	)
	SetAfterHook(id SubscriptionID, hooks ...AfterHook[T]) bool

	// SetErrorHook sets error hooks for a specific handler identified by its subscription ID.
	// These hooks only apply to the specified handler when it returns an error.
	//
	// Handler-level error hooks are called after global error hooks.
	// The hooks are called in the order provided.
	//
	// Returns true if the handler was found and hooks were set, false if the handler
	// doesn't exist (e.g., already unsubscribed).
	//
	// Nil hooks are filtered out automatically.
	// To clear hooks, call with no arguments or only nil values: SetErrorHook(id) or SetErrorHook(id, nil)
	//
	// Calling this method replaces any previously set error hooks for this handler.
	//
	// Example:
	//
	//	id, _ := bus.Subscribe(sendWelcomeEmail)
	//	bus.SetErrorHook(id,
	//	    func(ctx context.Context, event User, id SubscriptionID, err error) {
	//	        // Retry logic for this specific handler
	//	        if isRetryable(err) {
	//	            retryQueue.Add(event, err)
	//	        }
	//	    },
	//	    func(ctx context.Context, event User, id SubscriptionID, err error) {
	//	        // Alert for this critical handler
	//	        alertOncall("Welcome email failed", err, event)
	//	    },
	//	)
	SetErrorHook(id SubscriptionID, hooks ...ErrorHook[T]) bool

	// =============================================================================
	// Hook Control
	// =============================================================================

	// DisableHooks temporarily disables ALL hooks (both global and handler-specific)
	// without removing them.
	//
	// Call EnableHooks() to re-enable hooks. Hooks should be disabled by default.
	//
	// This operation is thread-safe and affects all subsequent Publish calls.
	//
	// Example:
	//
	//	bus.DisableHooks()
	//	// Run performance test without hook overhead
	//	runPerfTest()
	//	bus.EnableHooks()
	DisableHooks()

	// EnableHooks re-enables hooks after they were disabled with DisableHooks().
	// If hooks are already enabled, this is a no-op.
	//
	// This operation is thread-safe and affects all subsequent Publish calls.
	//
	// Example:
	//
	//	bus.DisableHooks()
	//	defer bus.EnableHooks()  // Ensure hooks are re-enabled
	//	// ... do work without hooks ...
	EnableHooks()

	// HooksEnabled returns true if hooks are currently enabled, false otherwise.
	// This can be used to check hook status before performing hook-dependent operations.
	//
	// Example:
	//
	//	if bus.HooksEnabled() {
	//	    log.Info("Running with observability hooks enabled")
	//	} else {
	//	    log.Warn("Hooks disabled - no metrics or logging active")
	//	}
	HooksEnabled() bool

	// =============================================================================
	// Hook Option Factories
	// =============================================================================
	//
	// Implementation Note:
	// These methods should be implemented by delegating to the corresponding functions
	// in this package (types.WithBefore[T](), types.WithAfter[T](), types.WithOnError[T]()).
	// This provides clean API with type inference while reusing shared implementation.
	//
	// Example implementation:
	//
	//	func (bus *EventBus[T]) WithBefore(hooks ...BeforeHook[T]) SubscribeOption[T] {
	//	    return types.WithBefore[T](hooks...)
	//	}

	// WithBefore creates a SubscribeOption that attaches before hooks to a handler.
	// Because this is a method on EventBus[T], the type T is already known,
	// so you don't need to specify it!
	//
	// Implementation Note: Should delegate to types.WithBefore[T](hooks...)
	//
	// Example:
	//
	//	bus.Subscribe(handler,
	//	    bus.WithBefore(func(ctx context.Context, event User) context.Context {
	//	        return context.WithValue(ctx, "traceID", uuid.New())
	//	    }),
	//	)
	WithBefore(hooks ...BeforeHook[T]) SubscribeOption[T]

	// WithAfter creates a SubscribeOption that attaches after hooks to a handler.
	// Because this is a method on EventBus[T], the type T is already known,
	// so you don't need to specify it!
	//
	// Implementation Note: Should delegate to types.WithAfter[T](hooks...)
	// Example:
	//
	//	bus.Subscribe(handler,
	//	    bus.WithAfter(func(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
	//	        metrics.RecordDuration("handler_duration", duration)
	//	    }),
	//	)
	WithAfter(hooks ...AfterHook[T]) SubscribeOption[T]

	// WithOnError creates a SubscribeOption that attaches error hooks to a handler.
	// Because this is a method on EventBus[T], the type T is already known,
	// so you don't need to specify it!
	//
	// Implementation Note: Should delegate to types.WithOnError[T](hooks...)
	// Example:
	//
	//	bus.Subscribe(handler,
	//	    bus.WithOnError(func(ctx context.Context, event User, id SubscriptionID, err error) {
	//	        log.Error("Handler failed", "error", err, "user", event.ID)
	//	        alertOncall("Handler failure", err)
	//	    }),
	//	)
	WithOnError(hooks ...ErrorHook[T]) SubscribeOption[T]
}

// =============================================================================
// Hooks + Options Combined Interface
// =============================================================================

// EventBusHooksOptions is an interface that combines hook support with advanced options.
// This indicates an implementation supports both the hook system AND the Once/Serial/Timeout options.
//
// Not all EventBus implementations with hooks need to support options. For example:
//   - A hooks-only implementation might skip Serial/Timeout for simplicity
//   - A minimal observable bus might only provide hooks without advanced options
//
// Implementations that satisfy this interface provide:
//   - All hook capabilities (before/after/error hooks, global and handler-specific)
//   - Advanced subscription options (Once, Serial, Timeout)
//
// Example usage (type assertion):
//
//	if busWithBoth, ok := bus.(EventBusHooksOptions[User]); ok {
//	    // This implementation supports both hooks AND options
//	    busWithBoth.Subscribe(handler,
//	        busWithBoth.WithSerial(),
//	        busWithBoth.WithTimeout(5*time.Second),
//	        busWithBoth.WithAfter(metricsHook),
//	    )
//	} else if busWithHooks, ok := bus.(EventBusWithHooks[User]); ok {
//	    // Fallback: hooks only, no advanced options
//	    busWithHooks.Subscribe(handler, busWithHooks.WithAfter(metricsHook))
//	}
//
// Example usage (type switch):
//
//	switch b := bus.(type) {
//	case EventBusHooksOptions[User]:
//	    // Full observability with advanced options
//	    b.Subscribe(handler, b.WithSerial(), b.WithBefore(tracingHook))
//	case EventBusWithHooks[User]:
//	    // Hooks only
//	    b.Subscribe(handler, b.WithBefore(tracingHook))
//	default:
//	    // Basic bus
//	    bus.Subscribe(handler)
//	}
type EventBusHooksOptions[T any] interface {
	EventBusWithHooks[T]
	EventBusWithOptions[T]
}

// =============================================================================
// Metrics Support Interface
// =============================================================================

// =============================================================================
// Metrics Types
// =============================================================================

// BusMetrics represents metrics for an EventBus instance.
// These counters track the lifecycle and health of event processing.
type BusMetrics struct {
	// PublishedCount is the total number of events published to this bus
	PublishedCount uint64

	// SubscriberCount is the current number of active subscribers
	SubscriberCount uint64

	// ProcessedCount is the total number of handler invocations (successful + errors)
	ProcessedCount uint64

	// ErrorCount is the total number of handler errors
	ErrorCount uint64
}

// EventBusWithMetrics is an interface that indicates an EventBus implementation
// provides runtime metrics and observability data.
//
// Implementations that satisfy this interface expose counters for:
//   - Total events published
//   - Current subscriber count
//   - Total handlers executed (successful + failed)
//   - Total handler errors
//
// Not all EventBus implementations need to provide metrics. Metrics add a small
// overhead (atomic counter increments).
//
// Example usage (type assertion):
//
//	if busWithMetrics, ok := bus.(EventBusWithMetrics[User]); ok {
//	    metrics := busWithMetrics.Metrics()
//	    log.Info("Bus metrics",
//	        "published", metrics.PublishedCount,
//	        "subscribers", metrics.SubscriberCount,
//	        "processed", metrics.ProcessedCount,
//	        "errors", metrics.ErrorCount,
//	    )
//	}
type EventBusWithMetrics[T any] interface {
	EventBus[T]

	// Metrics returns a snapshot of the current metrics for this bus.
	// The returned BusMetrics struct contains atomic counter values captured
	// at the moment this method is called.
	//
	// This operation is thread-safe and lock-free, using atomic loads.
	//
	// Example:
	//
	//	metrics := bus.Metrics()
	//	errorRate := float64(metrics.ErrorCount) / float64(metrics.ProcessedCount)
	//	if errorRate > 0.05 {
	//	    log.Warn("High error rate detected", "rate", errorRate)
	//	}
	Metrics() BusMetrics

	// =============================================================================
	// Individual Metric Getters
	// =============================================================================

	// PublishedCount returns the total number of events published to this bus.
	// This count includes all Publish() calls since the bus was created.
	//
	// This operation is thread-safe and lock-free, using atomic loads.
	//
	// Example:
	//
	//	count := bus.PublishedCount()
	//	log.Info("Total events published", "count", count)
	PublishedCount() uint64

	// SubscriberCount returns the current number of active subscribers.
	// This count reflects the number of handlers currently registered on the bus.
	//
	// This operation is thread-safe and lock-free, using atomic loads.
	//
	// Example:
	//
	//	count := bus.SubscriberCount()
	//	if count == 0 {
	//	    log.Warn("No subscribers registered")
	//	}
	SubscriberCount() uint64

	// ProcessedCount returns the total number of handler invocations.
	// This includes both successful executions and those that returned errors.
	//
	// This operation is thread-safe and lock-free, using atomic loads.
	//
	// Example:
	//
	//	count := bus.ProcessedCount()
	//	log.Info("Total handlers executed", "count", count)
	ProcessedCount() uint64

	// ErrorCount returns the total number of handler errors.
	// This count includes all handlers that returned non-nil errors.
	//
	// This operation is thread-safe and lock-free, using atomic loads.
	//
	// Example:
	//
	//	errors := bus.ErrorCount()
	//	processed := bus.ProcessedCount()
	//	if processed > 0 {
	//	    errorRate := float64(errors) / float64(processed)
	//	    log.Info("Error rate", "rate", errorRate)
	//	}
	ErrorCount() uint64
}

// =============================================================================
// Metrics + Options Combined Interface
// =============================================================================

// EventBusMetricsOptions is an interface that combines metrics support with advanced options.
// This indicates an implementation provides runtime metrics AND supports Once/Serial/Timeout options.
//
// Not all EventBus implementations with metrics need to support options. For example:
//   - A metrics-only implementation might skip Serial/Timeout for simplicity
//   - A monitoring-focused bus might only track metrics without advanced subscription features
//
// Implementations that satisfy this interface provide:
//   - Runtime metrics (published count, subscriber count, processed count, error count)
//   - Advanced subscription options (Once, Serial, Timeout)
//
// Example usage (type assertion):
//
//	if busWithBoth, ok := bus.(EventBusMetricsOptions[User]); ok {
//	    // This implementation supports both metrics AND options
//	    busWithBoth.Subscribe(handler,
//	        busWithBoth.WithSerial(),
//	        busWithBoth.WithTimeout(5*time.Second),
//	    )
//	    metrics := busWithBoth.Metrics()
//	    log.Info("Bus stats", "subscribers", metrics.SubscriberCount)
//	} else if busWithMetrics, ok := bus.(EventBusWithMetrics[User]); ok {
//	    // Fallback: metrics only, no advanced options
//	    metrics := busWithMetrics.Metrics()
//	    log.Info("Basic stats", "subscribers", metrics.SubscriberCount)
//	}
//
// Example usage (type switch):
//
//	switch b := bus.(type) {
//	case EventBusMetricsOptions[User]:
//	    // Metrics with advanced options
//	    b.Subscribe(handler, b.WithSerial())
//	    logMetrics(b.Metrics())
//	case EventBusWithMetrics[User]:
//	    // Metrics only
//	    logMetrics(b.Metrics())
//	default:
//	    // Basic bus
//	    bus.Subscribe(handler)
//	}
type EventBusMetricsOptions[T any] interface {
	EventBusWithMetrics[T]
	EventBusWithOptions[T]
}

// =============================================================================
// Complete EventBus Interfaces
// =============================================================================

// EventBusHooksMetricsOptions is the ultimate comprehensive interface that combines ALL EventBus
// capabilities: base functionality, options, hooks, and metrics.
//
// This interface is useful when you want to:
//   - Ensure an implementation provides all features
//   - Write code that requires options, hooks, and metrics together
//   - Type-assert for full-featured, production-ready bus implementations
//
// Most production-ready EventBus implementations should satisfy this interface
// to provide maximum observability, flexibility, and control.
//
// This interface composes:
//   - EventBus[T]: Core subscription, publishing, and lifecycle methods
//   - EventBusWithOptions[T]: Once, Serial, Timeout options
//   - EventBusWithHooks[T]: Before/After/Error hooks (global and per-handler)
//   - EventBusWithMetrics[T]: Runtime metrics and counters
//
// Example usage:
//
//	if fullBus, ok := bus.(EventBusHooksMetrics[User]); ok {
//	    // All features available
//	    fullBus.Subscribe(handler,
//	        fullBus.WithSerial(),              // from Options
//	        fullBus.WithTimeout(5*time.Second), // from Options
//	        fullBus.WithAfter(metricsHook),    // from Hooks
//	    )
//
//	    // Configure global hooks
//	    fullBus.SetGlobalBeforeHook(tracingHook)
//
//	    // Check metrics
//	    metrics := fullBus.Metrics()
//	    log.Info("Full-featured bus",
//	        "subscribers", metrics.SubscriberCount,
//	        "processed", metrics.ProcessedCount,
//	    )
//	}
type EventBusHooksMetricsOptions[T any] interface {
	EventBus[T]
	EventBusWithOptions[T]
	EventBusWithHooks[T]
	EventBusWithMetrics[T]
}

// EventBusHooksMetrics combines hooks and metrics WITHOUT the advanced options.
// This is useful for observable implementations that don't need Once/Serial/Timeout complexity.
//
// Use this when you want:
//   - Full observability (hooks + metrics)
//   - Simpler implementation (no Serial/Timeout overhead)
//   - Production monitoring without advanced subscription features
//
// Example usage:
//
//	if obsBus, ok := bus.(EventBusHooksMetricsOnly[User]); ok {
//	    // Hooks and metrics, but no advanced options
//	    obsBus.Subscribe(handler,
//	        obsBus.WithAsync(),              // from EventBus
//	        obsBus.WithAfter(metricsHook),   // from Hooks
//	    )
//	    obsBus.SetGlobalErrorHook(alertHook)
//	    metrics := obsBus.Metrics()
//	    log.Info("Observable bus", "errors", metrics.ErrorCount)
//	}
type EventBusHooksMetrics[T any] interface {
	EventBus[T]
	EventBusWithHooks[T]
	EventBusWithMetrics[T]
}

// =============================================================================
// EventBus Config Interface
// =============================================================================

// Config is an interface that all EventBus configurations must implement.
// Each EventBus implementation defines its own config type.
// This allows the factory function to determine which implementation to use
// based on the config type provided.
//
// Example implementations:
//   - simple.Config: Configuration for mutex-based simple EventBus
//   - future implementations could add: lockfree.Config, channel.Config, etc.
type Config interface {
	// EventBusConfig is a marker method to ensure only valid configs are used.
	EventBusConfig()

	// GetWorkerPool returns the worker pool configured for this EventBus.
	// Returns nil if no external pool is configured.
	GetWorkerPool() workerpooltypes.Pool

	// SetWorkerPool sets the worker pool for this EventBus configuration.
	// This allows the EventManager to inject its shared pool.
	SetWorkerPool(pool workerpooltypes.Pool)

	// Copy returns a deep copy of this configuration.
	// This allows config templates to be safely reused across multiple buses.
	Copy() Config
}
