// Copyright 2025 NetApp, Inc. All Rights Reserved.

package simple

import (
	"github.com/netapp/trident/pkg/eventbus/types"
	genericsyncpool "github.com/netapp/trident/pkg/generic_syncpool"
)

// =============================================================================
// Hook Types (re-exported from types package)
// =============================================================================

// Re-export hook types and hook slice types from types package for convenience.
// Note: Hook slice types (BeforeHooks, AfterHooks, ErrorHooks) are now defined
// in the types package with DeepCopy methods for thread-safe copying.
type (
	BeforeHook[T any]  = types.BeforeHook[T]
	AfterHook[T any]   = types.AfterHook[T]
	ErrorHook[T any]   = types.ErrorHook[T]
	BeforeHooks[T any] = types.BeforeHooks[T]
	AfterHooks[T any]  = types.AfterHooks[T]
	ErrorHooks[T any]  = types.ErrorHooks[T]
)

// =============================================================================
// Global (Bus-level) Hook Registration
// =============================================================================

// SetGlobalBeforeHook registers hooks that run before EVERY handler on this bus.
// The hook can modify the context (e.g., add tracing span).
// Multiple hooks are executed in registration order.
// Nil hooks are filtered out automatically.
// To clear hooks, call with no arguments or only nil values: SetGlobalBeforeHook() or SetGlobalBeforeHook(nil)
// For handler-specific hooks, use SetBeforeHook(handlerID, hooks...) or WithBefore() at subscribe time.
func (bus *EventBus[T]) SetGlobalBeforeHook(hooks ...BeforeHook[T]) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Filter out nil hooks
	filtered := make(BeforeHooks[T], 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			filtered = append(filtered, hook)
		}
	}

	// Always set the hooks (even if empty, to allow clearing)
	bus.beforeHooks = filtered
}

// SetGlobalAfterHook registers hooks that run after EVERY handler on this bus completes.
// It receives the result, duration, and any error from the handler.
// Multiple hooks are executed in registration order.
// Nil hooks are filtered out automatically.
// To clear hooks, call with no arguments or only nil values: SetGlobalAfterHook() or SetGlobalAfterHook(nil)
// For handler-specific hooks, use SetAfterHook(handlerID, hooks...) or WithAfter() at subscribe time.
func (bus *EventBus[T]) SetGlobalAfterHook(hooks ...AfterHook[T]) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Filter out nil hooks
	filtered := make(AfterHooks[T], 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			filtered = append(filtered, hook)
		}
	}

	// Always set the hooks (even if empty, to allow clearing)
	bus.afterHooks = filtered
}

// SetGlobalErrorHook registers hooks that run when ANY handler on this bus returns an error.
// Use for logging, alerting, dead-letter queues, or retry logic.
// Multiple hooks are executed in registration order.
// Nil hooks are filtered out automatically.
// To clear hooks, call with no arguments or only nil values: SetGlobalErrorHook() or SetGlobalErrorHook(nil)
// For handler-specific hooks, use SetErrorHook(handlerID, hooks...) or WithOnError() at subscribe time.
func (bus *EventBus[T]) SetGlobalErrorHook(hooks ...ErrorHook[T]) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Filter out nil hooks
	filtered := make(ErrorHooks[T], 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			filtered = append(filtered, hook)
		}
	}

	// Always set the hooks (even if empty, to allow clearing)
	bus.errorHooks = filtered
}

// =============================================================================
// Hook System Control
// =============================================================================

// DisableHooks disables the hook system for this bus, providing better performance.
// When hooks are disabled:
//   - All hook execution is skipped (before, after, error hooks)
//   - Duration measurement is skipped
//   - Handler execution is direct with minimal overhead
//
// This method is thread-safe and can be called at runtime.
// Note: This does NOT clear existing registered hooks. If you call EnableHooks()
// later, previously registered hooks will be executed again.
//
// Example:
//
//	bus.DisableHooks()  // Disable hooks for performance
//	// ... high-throughput event processing ...
//	bus.EnableHooks() // Re-enable hooks if needed
func (bus *EventBus[T]) DisableHooks() {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	bus.noHooks = true
}

// EnableHooks re-enables the hook system for this bus.
// This initializes hook pools if they haven't been created yet.
//
// When hooks are enabled:
//   - Before/After/Error hooks are executed for each handler
//   - Duration measurements are collected
//   - Hook pools are used for memory efficiency
//
// This method is thread-safe and can be called at runtime.
//
// Example:
//
//	// Start with NoHooks for performance
//	bus := NewEventBus[Event](ctx, WithNoHooks[Event]())
//
//	// Later, enable hooks for debugging
//	bus.EnableHooks()
//	bus.SetGlobalBeforeHook(loggingHook)
func (bus *EventBus[T]) EnableHooks() {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Exit early if hooks were already enabled
	if bus.noHooks == false {
		return
	}

	// If hooks were disabled, enable them
	bus.noHooks = false

	// Initialize hook pools if they don't exist
	// This handles the case where the bus was created with WithNoHooks
	// and pools were never initialized
	if bus.beforeHookPool == nil {
		bus.beforeHookPool = genericsyncpool.NewPool(func() *beforeHookPool[T] {
			return &beforeHookPool[T]{
				hooks: make(BeforeHooks[T], 0, 4),
			}
		})
	}

	if bus.afterHookPool == nil {
		bus.afterHookPool = genericsyncpool.NewPool(func() *afterHookPool[T] {
			return &afterHookPool[T]{
				hooks: make(AfterHooks[T], 0, 4),
			}
		})
	}

	if bus.errorHookPool == nil {
		bus.errorHookPool = genericsyncpool.NewPool(func() *errorHookPool[T] {
			return &errorHookPool[T]{
				hooks: make(ErrorHooks[T], 0, 4),
			}
		})
	}
}

// HooksEnabled returns true if the hook system is enabled for this bus.
// This can be used to check the current hook state.
//
// Example:
//
//	if bus.HooksEnabled() {
//	    fmt.Println("Running with observability hooks")
//	} else {
//	    fmt.Println("Running in high-performance mode")
//	}
func (bus *EventBus[T]) HooksEnabled() bool {
	bus.mu.RLock()
	defer bus.mu.RUnlock()
	return !bus.noHooks
}

// =============================================================================
// Handler-level Hook Registration (by SubscriptionID)
// =============================================================================

// SetBeforeHook adds before hooks to a specific handler identified by SubscriptionID.
// Returns false if the handler doesn't exist.
// Multiple hooks are executed in registration order.
// Nil hooks are filtered out automatically.
// To clear hooks, call with no arguments or only nil values: SetBeforeHook(id) or SetBeforeHook(id, nil)
func (bus *EventBus[T]) SetBeforeHook(id SubscriptionID, hooks ...BeforeHook[T]) bool {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	index, exists := bus.handlers[id]
	if !exists {
		return false
	}

	async := bus.isAsyncUnsafe(id)

	// Filter out nil hooks
	filtered := make(BeforeHooks[T], 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			filtered = append(filtered, hook)
		}
	}

	// Always set the hooks (even if empty, to allow clearing)
	if async {
		bus.asyncHandlers[index].beforeHooks = filtered
	} else {
		bus.syncHandlers[index].beforeHooks = filtered
	}
	return true
}

// SetAfterHook adds after hooks to a specific handler identified by SubscriptionID.
// Returns false if the handler doesn't exist.
// Multiple hooks are executed in registration order.
// Nil hooks are filtered out automatically.
// To clear hooks, call with no arguments or only nil values: SetAfterHook(id) or SetAfterHook(id, nil)
func (bus *EventBus[T]) SetAfterHook(id SubscriptionID, hooks ...AfterHook[T]) bool {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	index, exists := bus.handlers[id]
	if !exists {
		return false
	}

	async := bus.isAsyncUnsafe(id)

	// Filter out nil hooks
	filtered := make(AfterHooks[T], 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			filtered = append(filtered, hook)
		}
	}

	// Always set the hooks (even if empty, to allow clearing)
	if async {
		bus.asyncHandlers[index].afterHooks = filtered
	} else {
		bus.syncHandlers[index].afterHooks = filtered
	}
	return true
}

// SetErrorHook adds error hooks to a specific handler identified by SubscriptionID.
// Returns false if the handler doesn't exist.
// Multiple hooks are executed in registration order.
// Nil hooks are filtered out automatically.
// To clear hooks, call with no arguments or only nil values: SetErrorHook(id) or SetErrorHook(id, nil)
func (bus *EventBus[T]) SetErrorHook(id SubscriptionID, hooks ...ErrorHook[T]) bool {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	index, exists := bus.handlers[id]
	if !exists {
		return false
	}

	async := bus.isAsyncUnsafe(id)

	// Filter out nil hooks
	filtered := make(ErrorHooks[T], 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			filtered = append(filtered, hook)
		}
	}

	// Always set the hooks (even if empty, to allow clearing)
	if async {
		bus.asyncHandlers[index].errorHooks = filtered
	} else {
		bus.syncHandlers[index].errorHooks = filtered
	}
	return true
}

// =============================================================================
// Hook Option Factories
// =============================================================================

// WithBefore creates a SubscribeOption that attaches before hooks to a handler.
// Because this is a method on EventBus[T], the type T is already known!
//
//	bus.Subscribe(handler, bus.WithBefore(tracingHook))
//
// Example:
//
//	bus.Subscribe(handler,
//	    bus.WithBefore(func(ctx context.Context, event User) context.Context {
//	        return context.WithValue(ctx, "traceID", uuid.New())
//	    }),
//	)
func (bus *EventBus[T]) WithBefore(hooks ...BeforeHook[T]) SubscribeOption[T] {
	return types.WithBefore[T](hooks...)
}

// WithAfter creates a SubscribeOption that attaches after hooks to a handler.
// Because this is a method on EventBus[T], the type T is already known!
//
//	bus.Subscribe(handler, bus.WithAfter(metricsHook))
//
// Example:
//
//	bus.Subscribe(handler,
//	    bus.WithAfter(func(ctx context.Context, event User, id SubscriptionID, result Result, duration time.Duration, err error) {
//	        metrics.RecordDuration("handler_duration", duration)
//	    }),
//	)
func (bus *EventBus[T]) WithAfter(hooks ...AfterHook[T]) SubscribeOption[T] {
	return types.WithAfter[T](hooks...)
}

// WithOnError creates a SubscribeOption that attaches error hooks to a handler.
// Because this is a method on EventBus[T], the type T is already known!
//
//	bus.Subscribe(handler, bus.WithOnError(alertHook))
//
// Example:
//
//	bus.Subscribe(handler,
//	    bus.WithOnError(func(ctx context.Context, event User, id SubscriptionID, err error) {
//	        log.Error("Handler failed", "error", err)
//	        alertOncall("Handler failure", err)
//	    }),
//	)
func (bus *EventBus[T]) WithOnError(hooks ...ErrorHook[T]) SubscribeOption[T] {
	return types.WithOnError[T](hooks...)
}
