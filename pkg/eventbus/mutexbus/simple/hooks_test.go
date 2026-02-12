// Copyright 2025 NetApp, Inc. All Rights Reserved.

package simple

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetGlobalBeforeHook(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var calls []int
	hook1 := func(ctx context.Context, event string, id SubscriptionID) context.Context {
		calls = append(calls, 1)
		return ctx
	}
	hook2 := func(ctx context.Context, event string, id SubscriptionID) context.Context {
		calls = append(calls, 2)
		return ctx
	}

	// Test adding hooks
	bus.SetGlobalBeforeHook(hook1, hook2)
	assert.Len(t, bus.beforeHooks, 2)

	// Test hooks execute in order
	_, _ = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.Equal(t, []int{1, 2}, calls)

	// Test nil hook resets the hooks
	bus.SetGlobalBeforeHook(nil)
	assert.Len(t, bus.beforeHooks, 0)
}

func TestSetGlobalAfterHook(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var calls []int
	hook1 := func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		calls = append(calls, 1)
	}
	hook2 := func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		calls = append(calls, 2)
	}

	// Test adding hooks
	bus.SetGlobalAfterHook(hook1, hook2)
	assert.Len(t, bus.afterHooks, 2)

	// Test hooks execute
	_, _ = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.Equal(t, []int{1, 2}, calls)

	// Test nil hook resets the hooks
	bus.SetGlobalAfterHook(nil)
	assert.Len(t, bus.afterHooks, 0)
}

func TestSetGlobalErrorHook(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var calls []int
	hook1 := func(ctx context.Context, event string, id SubscriptionID, err error) {
		calls = append(calls, 1)
	}
	hook2 := func(ctx context.Context, event string, id SubscriptionID, err error) {
		calls = append(calls, 2)
	}

	// Test adding hooks
	bus.SetGlobalErrorHook(hook1, hook2)
	assert.Len(t, bus.errorHooks, 2)

	// Test hooks execute on error
	_, _ = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, assert.AnError
	})
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.Equal(t, []int{1, 2}, calls)

	// Test nil hook resets the hooks
	bus.SetGlobalErrorHook(nil)
	assert.Len(t, bus.errorHooks, 0)
}

func TestDisableEnableHooks(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Initially enabled with hooks config
	assert.True(t, bus.HooksEnabled())

	// Disable
	bus.DisableHooks()
	assert.False(t, bus.HooksEnabled())

	// Enable again
	bus.EnableHooks()
	assert.True(t, bus.HooksEnabled())

	// Verify pools are initialized after enable
	assert.NotNil(t, bus.beforeHookPool)
	assert.NotNil(t, bus.afterHookPool)
	assert.NotNil(t, bus.errorHookPool)

	// Multiple enable calls are safe
	bus.EnableHooks()
	assert.True(t, bus.HooksEnabled())
}

func TestEnableHooks_WithNoHooksOption(t *testing.T) {
	ctx := context.Background()
	cfg := NewConfig(WithNoHooks())
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Initially disabled
	assert.False(t, bus.HooksEnabled())
	assert.Nil(t, bus.beforeHookPool)

	// Enable hooks
	bus.EnableHooks()
	assert.True(t, bus.HooksEnabled())

	// Pools should be initialized
	assert.NotNil(t, bus.beforeHookPool)
	assert.NotNil(t, bus.afterHookPool)
	assert.NotNil(t, bus.errorHookPool)
}

func TestSetBeforeHook(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	hook := func(ctx context.Context, event string, id SubscriptionID) context.Context {
		return ctx
	}

	// Test with non-existent handler
	result := bus.SetBeforeHook(SubscriptionID(999), hook)
	assert.False(t, result)

	// Test with sync handler
	id, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	result = bus.SetBeforeHook(id, hook)
	assert.True(t, result)
	assert.Len(t, bus.syncHandlers[bus.handlers[id]].beforeHooks, 1)

	// Test with async handler
	asyncID, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAsync())
	result = bus.SetBeforeHook(asyncID, hook)
	assert.True(t, result)
	assert.Len(t, bus.asyncHandlers[bus.handlers[asyncID]].beforeHooks, 1)

	// Test nil hook resets the hooks
	result = bus.SetBeforeHook(id, nil)
	assert.True(t, result)
	assert.Len(t, bus.syncHandlers[bus.handlers[id]].beforeHooks, 0)
}

func TestSetAfterHook(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	hook := func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
	}

	// Test with non-existent handler
	result := bus.SetAfterHook(SubscriptionID(999), hook)
	assert.False(t, result)

	// Test with sync handler
	id, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	result = bus.SetAfterHook(id, hook)
	assert.True(t, result)
	assert.Len(t, bus.syncHandlers[bus.handlers[id]].afterHooks, 1)

	// Test with async handler
	asyncID, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAsync())
	result = bus.SetAfterHook(asyncID, hook)
	assert.True(t, result)
	assert.Len(t, bus.asyncHandlers[bus.handlers[asyncID]].afterHooks, 1)

	// Test nil hook resets the hooks
	result = bus.SetAfterHook(id, nil)
	assert.True(t, result)
	assert.Len(t, bus.syncHandlers[bus.handlers[id]].afterHooks, 0)
}

func TestSetErrorHook(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	hook := func(ctx context.Context, event string, id SubscriptionID, err error) {}

	// Test with non-existent handler
	result := bus.SetErrorHook(SubscriptionID(999), hook)
	assert.False(t, result)

	// Test with sync handler
	id, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	result = bus.SetErrorHook(id, hook)
	assert.True(t, result)
	assert.Len(t, bus.syncHandlers[bus.handlers[id]].errorHooks, 1)

	// Test with async handler
	asyncID, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAsync())
	result = bus.SetErrorHook(asyncID, hook)
	assert.True(t, result)
	assert.Len(t, bus.asyncHandlers[bus.handlers[asyncID]].errorHooks, 1)

	// Test nil hook resets the hooks
	result = bus.SetErrorHook(id, nil)
	assert.True(t, result)
	assert.Len(t, bus.syncHandlers[bus.handlers[id]].errorHooks, 0)
}

func TestWithBeforeOption(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var called bool
	hook := func(ctx context.Context, event string, id SubscriptionID) context.Context {
		called = true
		return ctx
	}

	// Subscribe with before hook
	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithBefore(hook))

	require.NoError(t, err)
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.True(t, called)
}

func TestWithAfterOption(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var called bool
	hook := func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		called = true
	}

	// Subscribe with after hook
	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAfter(hook))

	require.NoError(t, err)
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.True(t, called)
}

func TestWithOnErrorOption(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var called bool
	hook := func(ctx context.Context, event string, id SubscriptionID, err error) {
		called = true
	}

	// Subscribe with error hook
	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, assert.AnError
	}, bus.WithOnError(hook))

	require.NoError(t, err)
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.True(t, called)
}

func TestHooksDisabled_NoExecution(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var beforeCalled, afterCalled, errorCalled bool

	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		beforeCalled = true
		return ctx
	})
	bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		afterCalled = true
	})
	bus.SetGlobalErrorHook(func(ctx context.Context, event string, id SubscriptionID, err error) {
		errorCalled = true
	})

	// Disable hooks
	bus.DisableHooks()

	// Subscribe and publish
	_, _ = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, assert.AnError
	})
	bus.Publish(ctx, "test")
	bus.WaitAsync()

	// Hooks should not be called
	assert.False(t, beforeCalled)
	assert.False(t, afterCalled)
	assert.False(t, errorCalled)
}

func TestMultipleHooks_Combined(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var globalBefore, globalAfter, handlerBefore, handlerAfter bool

	// Set global hooks
	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		globalBefore = true
		return ctx
	})
	bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		globalAfter = true
	})

	// Subscribe with handler-level hooks
	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithBefore(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		handlerBefore = true
		return ctx
	}), bus.WithAfter(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		handlerAfter = true
	}))

	require.NoError(t, err)
	bus.Publish(ctx, "test")
	bus.WaitAsync()

	// Both global and handler-level hooks should execute
	assert.True(t, globalBefore)
	assert.True(t, globalAfter)
	assert.True(t, handlerBefore)
	assert.True(t, handlerAfter)
}

func TestSetHooks_AfterSubscription(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfigWithHooks())
	require.NoError(t, err)
	defer bus.Close(ctx)

	var called bool
	hook := func(ctx context.Context, event string, id SubscriptionID) context.Context {
		called = true
		return ctx
	}

	// Subscribe first
	id, _ := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})

	// Add hook after subscription
	result := bus.SetBeforeHook(id, hook)
	assert.True(t, result)

	// Verify hook executes
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.True(t, called)
}
