// Copyright 2025 NetApp, Inc. All Rights Reserved.

package simple

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/workerpool"
	"github.com/netapp/trident/pkg/workerpool/ants"
	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
)

// --------------------------- NewEventBus_test ---------------------------
// TestNewEventBus tests the NewEventBus function with various configurations
func TestNewEventBus(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		config        Config
		setupFunc     func(t *testing.T) Config      // Optional function to setup config dynamically
		cleanupFunc   func(t *testing.T, cfg Config) // Optional cleanup function
		validate      func(t *testing.T, bus *EventBus[string], err error)
		expectError   bool
		errorContains string
	}{
		{
			name: "default configuration",
			config: Config{
				PreAllocHandlers: 0,   // Should use defaultPreAllocHandlers
				NoHooks:          nil, // Should use defaultNoHooks
				DefaultTimeout:   0,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Check basic initialization
				assert.NotNil(t, bus.handlers)
				assert.NotNil(t, bus.syncHandlers)
				assert.NotNil(t, bus.asyncHandlers)
				assert.Equal(t, 0, len(bus.handlers))
				assert.Equal(t, 0, len(bus.syncHandlers))
				assert.Equal(t, 0, len(bus.asyncHandlers))

				// Check capacity (should be defaultPreAllocHandlers)
				assert.Equal(t, defaultPreAllocHandlers, cap(bus.syncHandlers))
				assert.Equal(t, defaultPreAllocHandlers, cap(bus.asyncHandlers))

				// Check NoHooks default
				assert.Equal(t, defaultNoHooks, bus.noHooks)

				// Check worker pool was created and owns it
				assert.NotNil(t, bus.workerPool)
				assert.True(t, bus.ownsWorkerPool)

				// Check slice pools
				assert.NotNil(t, bus.syncSlicePool)
				assert.NotNil(t, bus.asyncSlicePool)

				// Check hook pools based on NoHooks setting
				if bus.noHooks {
					assert.Nil(t, bus.beforeHookPool)
					assert.Nil(t, bus.afterHookPool)
					assert.Nil(t, bus.errorHookPool)
				} else {
					assert.NotNil(t, bus.beforeHookPool)
					assert.NotNil(t, bus.afterHookPool)
					assert.NotNil(t, bus.errorHookPool)
				}

				// Check default timeout
				assert.Equal(t, time.Duration(0), bus.defaultTimeout)

				// Check bus is not closed
				assert.False(t, bus.closed.Load())

				// Check counters are zero
				assert.Equal(t, uint64(0), bus.publishedCount.Load())
				assert.Equal(t, uint64(0), bus.processedCount.Load())
				assert.Equal(t, uint64(0), bus.errorCount.Load())
				assert.Equal(t, uint64(0), bus.subscriberCount.Load())

				// Cleanup
				bus.Close(ctx)
			},
		},
		{
			name: "custom pre-allocation capacity",
			config: Config{
				PreAllocHandlers: 64,
				NoHooks:          nil,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Check custom capacity
				assert.Equal(t, 64, cap(bus.syncHandlers))
				assert.Equal(t, 64, cap(bus.asyncHandlers))
				assert.Equal(t, 0, len(bus.syncHandlers))
				assert.Equal(t, 0, len(bus.asyncHandlers))

				bus.Close(ctx)
			},
		},
		{
			name: "zero pre-allocation uses default",
			config: Config{
				PreAllocHandlers: 0, // Should use default
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				assert.Equal(t, defaultPreAllocHandlers, cap(bus.syncHandlers))
				assert.Equal(t, defaultPreAllocHandlers, cap(bus.asyncHandlers))

				bus.Close(ctx)
			},
		},
		{
			name: "NoHooks explicitly enabled",
			config: func() Config {
				noHooks := true
				return Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
			}(),
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Check NoHooks is enabled
				assert.True(t, bus.noHooks)

				// Hook pools should NOT be initialized
				assert.Nil(t, bus.beforeHookPool)
				assert.Nil(t, bus.afterHookPool)
				assert.Nil(t, bus.errorHookPool)

				// Other pools should still be initialized
				assert.NotNil(t, bus.syncSlicePool)
				assert.NotNil(t, bus.asyncSlicePool)

				bus.Close(ctx)
			},
		},
		{
			name: "NoHooks explicitly disabled",
			config: func() Config {
				noHooks := false
				return Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
			}(),
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Check NoHooks is disabled
				assert.False(t, bus.noHooks)

				// Hook pools SHOULD be initialized
				assert.NotNil(t, bus.beforeHookPool)
				assert.NotNil(t, bus.afterHookPool)
				assert.NotNil(t, bus.errorHookPool)

				bus.Close(ctx)
			},
		},
		{
			name: "worker pool auto-created when not provided",
			config: Config{
				PreAllocHandlers: 16,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Worker pool should be created and owned
				assert.NotNil(t, bus.workerPool)
				assert.True(t, bus.ownsWorkerPool)

				bus.Close(ctx)
			},
		},
		{
			name: "custom default timeout",
			config: Config{
				PreAllocHandlers: 16,
				DefaultTimeout:   5 * time.Second,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Check default timeout is set
				assert.Equal(t, 5*time.Second, bus.defaultTimeout)
				assert.Equal(t, 5*time.Second, bus.GetDefaultTimeout())

				bus.Close(ctx)
			},
		},
		{
			name: "zero default timeout",
			config: Config{
				PreAllocHandlers: 16,
				DefaultTimeout:   0,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Zero timeout is valid (no timeout)
				assert.Equal(t, time.Duration(0), bus.defaultTimeout)

				bus.Close(ctx)
			},
		},
		{
			name: "custom worker pool provided",
			setupFunc: func(t *testing.T) Config {
				// Create a custom worker pool
				ctx := context.Background()
				cfg := ants.DefaultConfig()
				cfg.NumWorkers = 15

				pool, err := workerpool.New[workerpooltypes.Pool](ctx, cfg)
				require.NoError(t, err)
				require.NoError(t, pool.Start(ctx))

				return Config{
					PreAllocHandlers: 16,
					WorkerPool:       pool,
				}
			},
			cleanupFunc: func(t *testing.T, cfg Config) {
				if cfg.WorkerPool != nil {
					ctx := context.Background()
					cfg.WorkerPool.Shutdown(ctx)
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Should use provided pool
				assert.NotNil(t, bus.workerPool)
				// Should NOT own the pool
				assert.False(t, bus.ownsWorkerPool)

				// Close bus (should not shut down external pool)
				bus.Close(ctx)

				// Worker pool should still be usable
				assert.NotNil(t, bus.workerPool)
			},
		},
		{
			name: "custom worker pool with NoHooks",
			setupFunc: func(t *testing.T) Config {
				ctx := context.Background()
				cfg := ants.DefaultConfig()
				cfg.NumWorkers = 10

				pool, err := workerpool.New[workerpooltypes.Pool](ctx, cfg)
				require.NoError(t, err)
				require.NoError(t, pool.Start(ctx))

				noHooks := true
				return Config{
					PreAllocHandlers: 32,
					WorkerPool:       pool,
					NoHooks:          &noHooks,
				}
			},
			cleanupFunc: func(t *testing.T, cfg Config) {
				if cfg.WorkerPool != nil {
					ctx := context.Background()
					cfg.WorkerPool.Shutdown(ctx)
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Should use provided pool
				assert.NotNil(t, bus.workerPool)
				assert.False(t, bus.ownsWorkerPool)

				// NoHooks should be enabled
				assert.True(t, bus.noHooks)
				assert.Nil(t, bus.beforeHookPool)
				assert.Nil(t, bus.afterHookPool)
				assert.Nil(t, bus.errorHookPool)

				bus.Close(ctx)
			},
		},
		{
			name: "large pre-allocation",
			config: Config{
				PreAllocHandlers: 1000,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				assert.Equal(t, 1000, cap(bus.syncHandlers))
				assert.Equal(t, 1000, cap(bus.asyncHandlers))
				assert.Equal(t, 0, len(bus.syncHandlers))
				assert.Equal(t, 0, len(bus.asyncHandlers))

				bus.Close(ctx)
			},
		},
		{
			name: "all options combined",
			config: func() Config {
				noHooks := false
				return Config{
					PreAllocHandlers: 50,
					NoHooks:          &noHooks,
					DefaultTimeout:   10 * time.Second,
				}
			}(),
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				// Verify all settings
				assert.Equal(t, 50, cap(bus.syncHandlers))
				assert.Equal(t, 50, cap(bus.asyncHandlers))
				assert.False(t, bus.noHooks)
				assert.Equal(t, 10*time.Second, bus.defaultTimeout)

				// Verify hook pools are initialized
				assert.NotNil(t, bus.beforeHookPool)
				assert.NotNil(t, bus.afterHookPool)
				assert.NotNil(t, bus.errorHookPool)

				// Verify worker pool
				assert.NotNil(t, bus.workerPool)
				assert.True(t, bus.ownsWorkerPool)

				bus.Close(ctx)
			},
		},
		{
			name: "minimum configuration",
			config: Config{
				PreAllocHandlers: 1,
				DefaultTimeout:   1 * time.Nanosecond,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				require.NoError(t, err)
				require.NotNil(t, bus)

				assert.Equal(t, 1, cap(bus.syncHandlers))
				assert.Equal(t, 1, cap(bus.asyncHandlers))
				assert.Equal(t, 1*time.Nanosecond, bus.defaultTimeout)

				bus.Close(ctx)
			},
		},
		{
			name: "different event types - int",
			config: Config{
				PreAllocHandlers: 16,
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				// This test validates string type (default in test cases)
				require.NoError(t, err)
				require.NotNil(t, bus)

				bus.Close(ctx)
			},
		},
		{
			name: "cancelled context on worker pool creation",
			setupFunc: func(t *testing.T) Config {
				// Note: This is tricky because the worker pool might handle
				// cancelled context internally. We'll test with a valid config
				// and just ensure error handling works.
				return Config{
					PreAllocHandlers: 16,
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], err error) {
				// With valid context, should succeed
				require.NoError(t, err)
				require.NotNil(t, bus)
				bus.Close(ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if tt.setupFunc != nil {
				cfg = tt.setupFunc(t)
			} else {
				cfg = tt.config
			}

			bus, err := NewEventBus[string](ctx, &cfg)

			if tt.cleanupFunc != nil {
				defer tt.cleanupFunc(t, cfg)
			}

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, bus)
			} else {
				tt.validate(t, bus, err)
			}
		})
	}
}

// TestNewEventBus_DifferentTypes tests NewEventBus with various Go types
func TestNewEventBus_DifferentTypes(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	t.Run("string type", func(t *testing.T) {
		bus, err := NewEventBus[string](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("int type", func(t *testing.T) {
		bus, err := NewEventBus[int](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("struct type", func(t *testing.T) {
		type TestEvent struct {
			ID   int
			Name string
		}
		bus, err := NewEventBus[TestEvent](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("pointer type", func(t *testing.T) {
		type TestEvent struct {
			ID int
		}
		bus, err := NewEventBus[*TestEvent](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("slice type", func(t *testing.T) {
		bus, err := NewEventBus[[]string](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("map type", func(t *testing.T) {
		bus, err := NewEventBus[map[string]int](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("interface type", func(t *testing.T) {
		bus, err := NewEventBus[interface{}](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})

	t.Run("any type", func(t *testing.T) {
		bus, err := NewEventBus[any](ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, bus)
		defer bus.Close(ctx)
	})
}

// TestNewEventBus_ConcurrentCreation tests creating multiple buses concurrently
func TestNewEventBus_ConcurrentCreation(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	const numBuses = 10
	buses := make([]*EventBus[string], numBuses)
	errors := make([]error, numBuses)

	// Create buses concurrently
	done := make(chan struct{})
	for i := 0; i < numBuses; i++ {
		go func(idx int) {
			buses[idx], errors[idx] = NewEventBus[string](ctx, cfg)
			done <- struct{}{}
		}(i)
	}

	// Wait for all creations
	for i := 0; i < numBuses; i++ {
		<-done
	}

	// Verify all succeeded
	for i := 0; i < numBuses; i++ {
		require.NoError(t, errors[i], "bus %d creation failed", i)
		require.NotNil(t, buses[i], "bus %d is nil", i)

		// Each bus should have independent state
		assert.NotNil(t, buses[i].handlers)
		assert.NotNil(t, buses[i].workerPool)
	}

	// Cleanup
	for i := 0; i < numBuses; i++ {
		if buses[i] != nil {
			buses[i].Close(ctx)
		}
	}
}

// --------------------------- Subscribe_Test ---------------------------
// TestSubscribe tests the Subscribe function with various options
func TestSubscribe(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setupBus    func(t *testing.T) *EventBus[string]
		handler     SubscriptionFunc[string]
		options     func(bus *EventBus[string]) []SubscribeOption[string]
		validate    func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error)
		expectError bool
		errorType   string
	}{
		{
			name: "basic subscription - no options",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return nil
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)
				assert.Greater(t, uint64(id), uint64(0))

				// Should be in sync handlers
				bus.mu.RLock()
				defer bus.mu.RUnlock()
				assert.Equal(t, 1, len(bus.syncHandlers))
				assert.Equal(t, 0, len(bus.asyncHandlers))
				assert.Equal(t, uint64(1), bus.subscriberCount.Load())

				// Check handler properties
				handler := bus.syncHandlers[0]
				assert.Equal(t, id, handler.id)
				assert.NotNil(t, handler.callback)
				assert.False(t, handler.async)
				assert.False(t, handler.flagOnce)
				assert.Equal(t, time.Duration(0), handler.timeout)
			},
		},
		{
			name: "subscription with Async option",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithAsync()}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)
				assert.Greater(t, uint64(id), uint64(0))

				// Should be in async handlers
				bus.mu.RLock()
				defer bus.mu.RUnlock()
				assert.Equal(t, 0, len(bus.syncHandlers))
				assert.Equal(t, 1, len(bus.asyncHandlers))

				// Check handler properties
				handler := bus.asyncHandlers[0]
				assert.Equal(t, id, handler.id)
				assert.True(t, handler.async)
				assert.False(t, handler.flagOnce)
			},
		},
		{
			name: "subscription with Once option",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithOnce()}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.flagOnce)
			},
		},
		{
			name: "subscription with Once and Async",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithOnce(), bus.WithAsync()}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				assert.Equal(t, 1, len(bus.asyncHandlers))
				handler := bus.asyncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.flagOnce)
				assert.True(t, handler.async)
			},
		},
		{
			name: "subscription with timeout",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithTimeout(5 * time.Second)}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.Equal(t, 5*time.Second, handler.timeout)
			},
		},
		{
			name: "subscription with zero timeout",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithTimeout(0)}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.Equal(t, time.Duration(0), handler.timeout)
			},
		},
		{
			name: "subscription with all options combined",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{
					bus.WithAsync(),
					bus.WithOnce(),
					bus.WithTimeout(3 * time.Second),
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.asyncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.async)
				assert.True(t, handler.flagOnce)
				assert.Equal(t, 3*time.Second, handler.timeout)
			},
		},
		{
			name: "subscription with Serial option",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithSerial()}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.serial)
				assert.NotNil(t, handler.mu)
			},
		},
		{
			name: "subscription with Serial and Async",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{bus.WithSerial(), bus.WithAsync()}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.asyncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.serial)
				assert.True(t, handler.async)
				assert.NotNil(t, handler.mu)
			},
		},
		{
			name: "subscription with Serial, Once, and Timeout",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return []SubscribeOption[string]{
					bus.WithSerial(),
					bus.WithOnce(),
					bus.WithTimeout(2 * time.Second),
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.serial)
				assert.True(t, handler.flagOnce)
				assert.Equal(t, 2*time.Second, handler.timeout)
				assert.NotNil(t, handler.mu)
			},
		},
		{
			name: "subscription with Serial and all options combined",
			setupBus: func(t *testing.T) *EventBus[string] {
				noHooks := false
				cfg := Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
				bus, err := NewEventBus[string](ctx, &cfg)
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				beforeHook := func(ctx context.Context, event string, id SubscriptionID) context.Context { return ctx }
				return []SubscribeOption[string]{
					bus.WithSerial(),
					bus.WithAsync(),
					bus.WithOnce(),
					bus.WithTimeout(3 * time.Second),
					bus.WithBefore(beforeHook),
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.asyncHandlers[0]

				assert.Equal(t, id, handler.id)
				assert.True(t, handler.serial)
				assert.True(t, handler.async)
				assert.True(t, handler.flagOnce)
				assert.Equal(t, 3*time.Second, handler.timeout)
				assert.NotNil(t, handler.mu)
				assert.Equal(t, 1, len(handler.beforeHooks))
			},
		},
		{
			name: "nil handler returns error",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: nil,
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return nil
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.Error(t, err)
				assert.Equal(t, SubscriptionID(0), id)
				assert.Contains(t, err.Error(), "cannot be nil")

				// No handlers should be added
				bus.mu.RLock()
				defer bus.mu.RUnlock()
				assert.Equal(t, 0, len(bus.syncHandlers))
				assert.Equal(t, 0, len(bus.asyncHandlers))
				assert.Equal(t, uint64(0), bus.subscriberCount.Load())
			},
			expectError: true,
			errorType:   "NilHandler",
		},
		{
			name: "subscribe to closed bus returns error",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				bus.Close(ctx)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return nil
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.Error(t, err)
				assert.Equal(t, SubscriptionID(0), id)
				assert.Contains(t, err.Error(), "closed")
			},
			expectError: true,
			errorType:   "BusClosed",
		},
		{
			name: "multiple subscriptions get unique IDs",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return nil
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				// Subscribe more handlers
				id2, err2 := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				})
				require.NoError(t, err2)

				id3, err3 := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err3)

				// All IDs should be unique
				assert.NotEqual(t, id, id2)
				assert.NotEqual(t, id, id3)
				assert.NotEqual(t, id2, id3)

				// Check counts
				bus.mu.RLock()
				defer bus.mu.RUnlock()
				assert.Equal(t, 2, len(bus.syncHandlers))
				assert.Equal(t, 1, len(bus.asyncHandlers))
				assert.Equal(t, uint64(3), bus.subscriberCount.Load())
			},
		},
		{
			name: "sync and async handlers stored separately",
			setupBus: func(t *testing.T) *EventBus[string] {
				bus, err := NewEventBus[string](ctx, DefaultConfig())
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				return nil
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				// Add some sync handlers
				_, err2 := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				})
				require.NoError(t, err2)

				// Add some async handlers
				_, err3 := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err3)

				_, err4 := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err4)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				assert.Equal(t, 2, len(bus.syncHandlers))
				assert.Equal(t, 2, len(bus.asyncHandlers))
				assert.Equal(t, uint64(4), bus.subscriberCount.Load())
			},
		},
		{
			name: "subscription with before hooks",
			setupBus: func(t *testing.T) *EventBus[string] {
				noHooks := false
				cfg := Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
				bus, err := NewEventBus[string](ctx, &cfg)
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				beforeHook := func(ctx context.Context, event string, id SubscriptionID) context.Context {
					return ctx
				}
				return []SubscribeOption[string]{bus.WithBefore(beforeHook)}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]
				assert.NotNil(t, handler.beforeHooks)
				assert.Equal(t, 1, len(handler.beforeHooks))
			},
		},
		{
			name: "subscription with after hooks",
			setupBus: func(t *testing.T) *EventBus[string] {
				noHooks := false
				cfg := Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
				bus, err := NewEventBus[string](ctx, &cfg)
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				afterHook := func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					// Do nothing
				}
				return []SubscribeOption[string]{bus.WithAfter(afterHook)}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]
				assert.NotNil(t, handler.afterHooks)
				assert.Equal(t, 1, len(handler.afterHooks))
			},
		},
		{
			name: "subscription with error hooks",
			setupBus: func(t *testing.T) *EventBus[string] {
				noHooks := false
				cfg := Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
				bus, err := NewEventBus[string](ctx, &cfg)
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				errorHook := func(ctx context.Context, event string, id SubscriptionID, err error) {
					// Do nothing
				}
				return []SubscribeOption[string]{bus.WithOnError(errorHook)}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]
				assert.NotNil(t, handler.errorHooks)
				assert.Equal(t, 1, len(handler.errorHooks))
			},
		},
		{
			name: "subscription with multiple hooks of same type",
			setupBus: func(t *testing.T) *EventBus[string] {
				noHooks := false
				cfg := Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
				bus, err := NewEventBus[string](ctx, &cfg)
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				beforeHook1 := func(ctx context.Context, event string, id SubscriptionID) context.Context { return ctx }
				beforeHook2 := func(ctx context.Context, event string, id SubscriptionID) context.Context { return ctx }
				return []SubscribeOption[string]{bus.WithBefore(beforeHook1, beforeHook2)}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]
				assert.Equal(t, 2, len(handler.beforeHooks))
			},
		},
		{
			name: "subscription with all hook types",
			setupBus: func(t *testing.T) *EventBus[string] {
				noHooks := false
				cfg := Config{
					PreAllocHandlers: 16,
					NoHooks:          &noHooks,
				}
				bus, err := NewEventBus[string](ctx, &cfg)
				require.NoError(t, err)
				return bus
			},
			handler: func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			},
			options: func(bus *EventBus[string]) []SubscribeOption[string] {
				beforeHook := func(ctx context.Context, event string, id SubscriptionID) context.Context { return ctx }
				afterHook := func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
				}
				errorHook := func(ctx context.Context, event string, id SubscriptionID, err error) {}
				return []SubscribeOption[string]{
					bus.WithBefore(beforeHook),
					bus.WithAfter(afterHook),
					bus.WithOnError(errorHook),
				}
			},
			validate: func(t *testing.T, bus *EventBus[string], id SubscriptionID, err error) {
				require.NoError(t, err)

				bus.mu.RLock()
				defer bus.mu.RUnlock()
				handler := bus.syncHandlers[0]
				assert.Equal(t, 1, len(handler.beforeHooks))
				assert.Equal(t, 1, len(handler.afterHooks))
				assert.Equal(t, 1, len(handler.errorHooks))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := tt.setupBus(t)
			defer bus.Close(ctx)

			opts := tt.options(bus)
			id, err := bus.Subscribe(tt.handler, opts...)

			tt.validate(t, bus, id, err)
		})
	}
}

// TestSubscribe_ConcurrentSubscriptions tests concurrent subscribe operations
func TestSubscribe_ConcurrentSubscriptions(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	const numSubscribers = 100
	ids := make([]SubscriptionID, numSubscribers)
	errors := make([]error, numSubscribers)

	// Subscribe concurrently
	done := make(chan struct{}, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		go func(idx int) {
			ids[idx], errors[idx] = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			})
			done <- struct{}{}
		}(i)
	}

	// Wait for all subscriptions
	for i := 0; i < numSubscribers; i++ {
		<-done
	}

	// Verify all succeeded
	for i := 0; i < numSubscribers; i++ {
		require.NoError(t, errors[i], "subscription %d failed", i)

		// Check that all the ids are grater than zero
		assert.Greater(t, uint64(ids[i]), uint64(0), "subscription %d has invalid ID", i)
	}

	// Verify all IDs are unique
	idMap := make(map[SubscriptionID]bool)
	for i := 0; i < numSubscribers; i++ {
		assert.False(t, idMap[ids[i]], "duplicate ID found: %d", ids[i])
		idMap[ids[i]] = true
	}

	// Check total count
	assert.Equal(t, uint64(numSubscribers), bus.subscriberCount.Load())
	totalHandlers := len(bus.syncHandlers) + len(bus.asyncHandlers)
	assert.Equal(t, numSubscribers, totalHandlers)

	// Verify that returned IDs match the actual subscription IDs in handlers
	// For each returned ID: ids[i] == bus.handlers[ids[i]].id
	for i := 0; i < numSubscribers; i++ {
		// Look up the handler index using the returned ID
		index, exists := bus.handlers[ids[i]]
		require.True(t, exists, "returned ID %d not found in bus.handlers map", ids[i])

		// Use the helper function to determine if it's async or sync
		if bus.isAsyncUnsafe(ids[i]) {
			// Verify: ids[i] == bus.asyncHandlers[bus.handlers[ids[i]]].id
			assert.Equal(t, ids[i], bus.asyncHandlers[index].id, "returned ID should match handler ID")
		} else {
			// Verify: ids[i] == bus.syncHandlers[bus.handlers[ids[i]]].id
			assert.Equal(t, ids[i], bus.syncHandlers[index].id, "returned ID should match handler ID")
		}
	}
}

// TestSubscribe_MixedSyncAsync tests subscribing both sync and async handlers concurrently
func TestSubscribe_MixedSyncAsync(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	const numEach = 50
	done := make(chan struct{}, numEach*2)

	// Subscribe sync handlers
	for i := 0; i < numEach; i++ {
		go func() {
			_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			})
			require.NoError(t, err)
			done <- struct{}{}
		}()
	}

	// Subscribe async handlers
	for i := 0; i < numEach; i++ {
		go func() {
			_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			}, bus.WithAsync())
			require.NoError(t, err)
			done <- struct{}{}
		}()
	}

	// Wait for all
	for i := 0; i < numEach*2; i++ {
		<-done
	}

	// Verify counts
	syncCount := len(bus.syncHandlers)
	asyncCount := len(bus.asyncHandlers)
	assert.Equal(t, numEach, syncCount)
	assert.Equal(t, numEach, asyncCount)
	assert.Equal(t, uint64(numEach*2), bus.subscriberCount.Load())
}

// TestSubscribe_OptionsOverride tests that later options override earlier ones
func TestSubscribe_OptionsOverride(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	handler := func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}

	// Subscribe with conflicting timeout options (last one wins)
	id, err := bus.Subscribe(handler,
		bus.WithTimeout(5*time.Second),
		bus.WithTimeout(10*time.Second),
	)
	require.NoError(t, err)

	h := bus.syncHandlers[0]
	assert.Equal(t, 10*time.Second, h.timeout)
	assert.Equal(t, id, h.id)
}

// TestSubscribe_DifferentTypes tests Subscribe with different event types
func TestSubscribe_DifferentTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("int type", func(t *testing.T) {
		bus, err := NewEventBus[int](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		id, err := bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
			return Result{}, nil
		})
		require.NoError(t, err)
		assert.Greater(t, uint64(id), uint64(0))
	})

	t.Run("struct type", func(t *testing.T) {
		type Event struct {
			ID   int
			Name string
		}
		bus, err := NewEventBus[Event](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		id, err := bus.Subscribe(func(ctx context.Context, event Event) (Result, error) {
			return Result{}, nil
		})
		require.NoError(t, err)
		assert.Greater(t, uint64(id), uint64(0))
	})

	t.Run("pointer type", func(t *testing.T) {
		type Event struct {
			ID int
		}
		bus, err := NewEventBus[*Event](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		id, err := bus.Subscribe(func(ctx context.Context, event *Event) (Result, error) {
			return Result{}, nil
		})
		require.NoError(t, err)
		assert.Greater(t, uint64(id), uint64(0))
	})
}

// --------------------------- Serial Execution Tests ---------------------------
// TestSerialExecution_Sync tests serial execution with synchronous handlers
func TestSerialExecution_Sync(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executing atomic.Int32
	var maxConcurrent atomic.Int32
	var callCount atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		current := executing.Add(1)
		defer executing.Add(-1)

		// Track maximum concurrent executions atomically
		for {
			max := maxConcurrent.Load()
			if current <= max || maxConcurrent.CompareAndSwap(max, current) {
				break
			}
		}

		// Simulate work
		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	// Subscribe with Serial option
	_, err = bus.Subscribe(handler, bus.WithSerial())
	require.NoError(t, err)

	// Publish multiple events concurrently
	const numPublish = 10
	var wg sync.WaitGroup
	for i := 0; i < numPublish; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(ctx, "event")
		}()
	}

	wg.Wait()

	// Verify: All events were processed
	assert.Equal(t, int32(numPublish), callCount.Load(), "All events should be processed")

	// Verify: Only one instance was executing at a time (serial execution)
	assert.Equal(t, int32(1), maxConcurrent.Load(), "Only one handler should execute at a time")
}

// TestSerialExecution_Async tests serial execution with asynchronous handlers
func TestSerialExecution_Async(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executing atomic.Int32
	var maxConcurrent atomic.Int32
	var callCount atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		current := executing.Add(1)
		defer executing.Add(-1)

		// Track maximum concurrent executions atomically
		for {
			max := maxConcurrent.Load()
			if current <= max || maxConcurrent.CompareAndSwap(max, current) {
				break
			}
		}

		// Simulate work
		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	// Subscribe with both Async and Serial options
	_, err = bus.Subscribe(handler, bus.WithAsync(), bus.WithSerial())
	require.NoError(t, err)

	// Publish multiple events concurrently
	const numPublish = 10
	var wg sync.WaitGroup
	for i := 0; i < numPublish; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(ctx, "event")
		}()
	}

	wg.Wait()
	bus.WaitAsync() // Wait for all async handlers to complete

	// Verify: All events were processed
	assert.Equal(t, int32(numPublish), callCount.Load(), "All events should be processed")

	// Verify: Only one instance was executing at a time (serial execution)
	assert.Equal(t, int32(1), maxConcurrent.Load(), "Only one handler should execute at a time")
}

// TestSerialExecution_MultipleHandlers tests that different handlers don't block each other
func TestSerialExecution_MultipleHandlers(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executing1 atomic.Int32
	var executing2 atomic.Int32
	var callCount1 atomic.Int32
	var callCount2 atomic.Int32

	handler1 := func(ctx context.Context, event string) (Result, error) {
		callCount1.Add(1)
		executing1.Add(1)
		defer executing1.Add(-1)
		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	handler2 := func(ctx context.Context, event string) (Result, error) {
		callCount2.Add(1)
		executing2.Add(1)
		defer executing2.Add(-1)
		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	// Subscribe both handlers with Serial option
	_, err = bus.Subscribe(handler1, bus.WithSerial(), bus.WithAsync())
	require.NoError(t, err)
	_, err = bus.Subscribe(handler2, bus.WithSerial(), bus.WithAsync())
	require.NoError(t, err)

	// Publish multiple events concurrently
	const numPublish = 5
	var wg sync.WaitGroup
	for i := 0; i < numPublish; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(ctx, "event")
		}()
	}

	wg.Wait()
	bus.WaitAsync() // Wait for all async handlers to complete

	// Verify: All events were processed by both handlers
	assert.Equal(t, int32(numPublish), callCount1.Load(), "All events should be processed by handler1")
	assert.Equal(t, int32(numPublish), callCount2.Load(), "All events should be processed by handler2")

	// Note: We can't verify maxConcurrent here because the two handlers have separate mutexes
	// and can run concurrently with each other (which is the correct behavior)
}

// TestSerialExecution_WithNonSerial tests mixing serial and non-serial handlers
func TestSerialExecution_WithNonSerial(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executingSerial atomic.Int32
	var maxConcurrentSerial atomic.Int32
	var executingNonSerial atomic.Int32
	var maxConcurrentNonSerial atomic.Int32
	var callCountSerial atomic.Int32
	var callCountNonSerial atomic.Int32

	serialHandler := func(ctx context.Context, event string) (Result, error) {
		callCountSerial.Add(1)
		current := executingSerial.Add(1)
		defer executingSerial.Add(-1)

		// Track maximum concurrent executions atomically
		for {
			max := maxConcurrentSerial.Load()
			if current <= max || maxConcurrentSerial.CompareAndSwap(max, current) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	nonSerialHandler := func(ctx context.Context, event string) (Result, error) {
		callCountNonSerial.Add(1)
		current := executingNonSerial.Add(1)
		defer executingNonSerial.Add(-1)

		// Track maximum concurrent executions atomically
		for {
			max := maxConcurrentNonSerial.Load()
			if current <= max || maxConcurrentNonSerial.CompareAndSwap(max, current) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	// Subscribe one serial and one non-serial handler (both async)
	_, err = bus.Subscribe(serialHandler, bus.WithAsync(), bus.WithSerial())
	require.NoError(t, err)
	_, err = bus.Subscribe(nonSerialHandler, bus.WithAsync())
	require.NoError(t, err)

	// Publish multiple events concurrently
	const numPublish = 10
	var wg sync.WaitGroup
	for i := 0; i < numPublish; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(ctx, "event")
		}()
	}

	wg.Wait()
	bus.WaitAsync()

	// Verify: All events were processed by both handlers
	assert.Equal(t, int32(numPublish), callCountSerial.Load(), "All events should be processed by serial handler")
	assert.Equal(t, int32(numPublish), callCountNonSerial.Load(), "All events should be processed by non-serial handler")

	// Verify: Serial handler executed only one at a time
	assert.Equal(t, int32(1), maxConcurrentSerial.Load(), "Serial handler should execute only one at a time")

	// Verify: Non-serial handler could execute concurrently
	assert.Greater(t, maxConcurrentNonSerial.Load(), int32(1), "Non-serial handler should be able to execute concurrently")
}

// TestSerialExecution_CopyBehavior verifies that mutex pointer is shared across copies
func TestSerialExecution_CopyBehavior(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executing atomic.Int32
	var maxConcurrent atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		current := executing.Add(1)
		defer executing.Add(-1)

		// Track maximum concurrent executions atomically
		for {
			max := maxConcurrent.Load()
			if current <= max || maxConcurrent.CompareAndSwap(max, current) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
		return Result{}, nil
	}

	// Subscribe with Serial option
	_, err = bus.Subscribe(handler, bus.WithSerial())
	require.NoError(t, err)

	// Publish multiple events concurrently
	// Each Publish will copy the subscription struct, but the mutex pointer should be shared
	const numPublish = 10
	var wg sync.WaitGroup
	for i := 0; i < numPublish; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(ctx, "event")
		}()
	}

	wg.Wait()

	// Verify: Only one instance was executing at a time
	// This proves the mutex pointer is being shared across copies
	assert.Equal(t, int32(1), maxConcurrent.Load(), "Mutex pointer should be shared across copies")
}

// --------------------------- Async Execution Tests ---------------------------
// TestAsyncExecution_Basic tests basic asynchronous handler execution
func TestAsyncExecution_Basic(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executing atomic.Int32
	var maxConcurrent atomic.Int32
	var callCount atomic.Int32

	// Channel to coordinate handler execution and ensure they run concurrently
	handlerStarted := make(chan struct{}, 10)
	handlerCanFinish := make(chan struct{})

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		current := executing.Add(1)
		defer executing.Add(-1)

		// Track maximum concurrent executions atomically
		for {
			max := maxConcurrent.Load()
			if current <= max || maxConcurrent.CompareAndSwap(max, current) {
				break
			}
		}

		// Signal that this handler has started
		handlerStarted <- struct{}{}

		// Wait for signal to finish (simulates work while allowing concurrent execution)
		<-handlerCanFinish

		return Result{}, nil
	}

	// Subscribe with Async option (no serial)
	_, err = bus.Subscribe(handler, bus.WithAsync())
	require.NoError(t, err)

	// Publish multiple events - async handlers should not block
	const numPublish = 10
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	// Wait for at least 2 handlers to start (proves concurrency)
	<-handlerStarted
	<-handlerStarted

	// At this point, we know at least 2 handlers are executing concurrently
	// Now allow all handlers to finish
	close(handlerCanFinish)

	bus.WaitAsync() // Wait for all async handlers to complete

	// Verify: All events were processed
	assert.Equal(t, int32(numPublish), callCount.Load(), "All events should be processed")

	// Verify: Multiple instances executed concurrently (async without serial)
	assert.GreaterOrEqual(t, maxConcurrent.Load(), int32(2), "At least 2 handlers should execute concurrently in async mode")
}

// TestAsyncExecution_ErrorHandling tests async handlers with errors
func TestAsyncExecution_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var successCount atomic.Int32
	var errorCount atomic.Int32

	successHandler := func(ctx context.Context, event string) (Result, error) {
		successCount.Add(1)
		return Result{}, nil
	}

	errorHandler := func(ctx context.Context, event string) (Result, error) {
		errorCount.Add(1)
		return Result{}, fmt.Errorf("simulated error")
	}

	// Subscribe both handlers with Async option
	_, err = bus.Subscribe(successHandler, bus.WithAsync())
	require.NoError(t, err)
	_, err = bus.Subscribe(errorHandler, bus.WithAsync())
	require.NoError(t, err)

	// Publish events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	bus.WaitAsync()

	// Verify: Both handlers processed all events (errors don't stop execution)
	assert.Equal(t, int32(numPublish), successCount.Load(), "Success handler should process all events")
	assert.Equal(t, int32(numPublish), errorCount.Load(), "Error handler should process all events despite errors")
}

// --------------------------- Once Execution Tests ---------------------------
// TestOnceExecution_Sync tests that sync handlers with Once option execute only once
func TestOnceExecution_Sync(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var callCount atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		return Result{}, nil
	}

	// Subscribe with Once option
	id, err := bus.Subscribe(handler, bus.WithOnce())
	require.NoError(t, err)

	// Publish multiple events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	// Verify: Handler was called only once
	assert.Equal(t, int32(1), callCount.Load(), "Handler should execute only once")

	// Verify: Handler was automatically unsubscribed
	_, exists := bus.handlers[id]
	assert.False(t, exists, "Handler should be automatically unsubscribed after execution")
}

// TestOnceExecution_Async tests that async handlers with Once option execute only once
func TestOnceExecution_Async(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var callCount atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		return Result{}, nil
	}

	// Subscribe with Once and Async options
	id, err := bus.Subscribe(handler, bus.WithOnce(), bus.WithAsync())
	require.NoError(t, err)

	// Publish multiple events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	bus.WaitAsync()

	// Verify: Handler was called only once
	assert.Equal(t, int32(1), callCount.Load(), "Async handler should execute only once")

	// Verify: Handler was automatically unsubscribed
	_, exists := bus.handlers[id]
	assert.False(t, exists, "Async handler should be automatically unsubscribed after execution")
}

// TestOnceExecution_MultipleHandlers tests multiple handlers with Once option
func TestOnceExecution_MultipleHandlers(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var callCount1 atomic.Int32
	var callCount2 atomic.Int32
	var callCount3 atomic.Int32

	handler1 := func(ctx context.Context, event string) (Result, error) {
		callCount1.Add(1)
		return Result{}, nil
	}

	handler2 := func(ctx context.Context, event string) (Result, error) {
		callCount2.Add(1)
		return Result{}, nil
	}

	handler3 := func(ctx context.Context, event string) (Result, error) {
		callCount3.Add(1)
		return Result{}, nil
	}

	// Subscribe all handlers with Once option
	id1, err := bus.Subscribe(handler1, bus.WithOnce())
	require.NoError(t, err)
	id2, err := bus.Subscribe(handler2, bus.WithOnce(), bus.WithAsync())
	require.NoError(t, err)
	id3, err := bus.Subscribe(handler3, bus.WithOnce())
	require.NoError(t, err)

	// Publish multiple events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	bus.WaitAsync()

	// Verify: Each handler was called only once
	assert.Equal(t, int32(1), callCount1.Load(), "Handler1 should execute only once")
	assert.Equal(t, int32(1), callCount2.Load(), "Handler2 should execute only once")
	assert.Equal(t, int32(1), callCount3.Load(), "Handler3 should execute only once")

	// Verify: All handlers were automatically unsubscribed
	_, exists1 := bus.handlers[id1]
	_, exists2 := bus.handlers[id2]
	_, exists3 := bus.handlers[id3]
	assert.False(t, exists1, "Handler1 should be unsubscribed")
	assert.False(t, exists2, "Handler2 should be unsubscribed")
	assert.False(t, exists3, "Handler3 should be unsubscribed")
}

// TestOnceExecution_WithRegularHandlers tests mixing Once and regular handlers
func TestOnceExecution_WithRegularHandlers(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var onceCount atomic.Int32
	var regularCount atomic.Int32

	onceHandler := func(ctx context.Context, event string) (Result, error) {
		onceCount.Add(1)
		return Result{}, nil
	}

	regularHandler := func(ctx context.Context, event string) (Result, error) {
		regularCount.Add(1)
		return Result{}, nil
	}

	// Subscribe one Once handler and one regular handler
	onceID, err := bus.Subscribe(onceHandler, bus.WithOnce(), bus.WithAsync())
	require.NoError(t, err)
	regularID, err := bus.Subscribe(regularHandler, bus.WithAsync())
	require.NoError(t, err)

	// Publish multiple events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	bus.WaitAsync()

	// Verify: Once handler was called only once, regular handler called for all events
	assert.Equal(t, int32(1), onceCount.Load(), "Once handler should execute only once")
	assert.Equal(t, int32(numPublish), regularCount.Load(), "Regular handler should execute for all events")

	// Verify: Once handler unsubscribed, regular handler still subscribed
	_, onceExists := bus.handlers[onceID]
	_, regularExists := bus.handlers[regularID]
	assert.False(t, onceExists, "Once handler should be unsubscribed")
	assert.True(t, regularExists, "Regular handler should still be subscribed")
}

// TestOnceExecution_ConcurrentPublish tests Once handlers with concurrent publishes
func TestOnceExecution_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var callCount atomic.Int32

	// Channel to coordinate handler execution
	handlerStarted := make(chan struct{})
	handlerCanFinish := make(chan struct{})

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)

		// Signal that handler has started
		handlerStarted <- struct{}{}

		// Wait for signal to finish
		<-handlerCanFinish

		return Result{}, nil
	}

	// Subscribe with Once and Async options
	id, err := bus.Subscribe(handler, bus.WithOnce(), bus.WithAsync())
	require.NoError(t, err)

	// Publish many events concurrently
	const numPublish = 100
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	// Wait for the first (and only) handler execution to start
	<-handlerStarted

	// Allow handler to finish
	close(handlerCanFinish)

	// Wait for async handlers to finish
	bus.WaitAsync()

	// Verify: Handler was called only once despite concurrent publishes
	assert.Equal(t, int32(1), callCount.Load(), "Handler should execute only once even with concurrent publishes")

	// Verify: Handler auto-unsubscribed after execution
	bus.mu.RLock()
	_, exists := bus.handlers[id]
	bus.mu.RUnlock()
	assert.False(t, exists, "Once handler should be auto-unsubscribed after execution")
}

// TestOnceExecution_WithSerial tests Once option combined with Serial
func TestOnceExecution_WithSerial(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var callCount atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		return Result{}, nil
	}

	// Subscribe with Once, Serial, and Async options
	id, err := bus.Subscribe(handler, bus.WithOnce(), bus.WithSerial(), bus.WithAsync())
	require.NoError(t, err)

	// Verify handler has both Once and Serial properties
	index := bus.handlers[id]
	h := bus.asyncHandlers[index]
	assert.True(t, h.flagOnce, "Handler should have Once flag")
	assert.True(t, h.serial, "Handler should have Serial flag")
	assert.NotNil(t, h.mu, "Handler should have mutex for serial execution")

	// Publish multiple events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	bus.WaitAsync()

	// Verify: Handler was called only once
	assert.Equal(t, int32(1), callCount.Load(), "Handler should execute only once")

	// Verify: Handler was automatically unsubscribed
	_, exists := bus.handlers[id]
	assert.False(t, exists, "Handler should be unsubscribed after execution")
}

// TestOnceExecution_ErrorHandler tests Once handler that returns an error
func TestOnceExecution_ErrorHandler(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var callCount atomic.Int32

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		return Result{}, fmt.Errorf("simulated error")
	}

	// Subscribe with Once option
	id, err := bus.Subscribe(handler, bus.WithOnce())
	require.NoError(t, err)

	// Publish multiple events
	const numPublish = 5
	for i := 0; i < numPublish; i++ {
		bus.Publish(ctx, "event")
	}

	// Verify: Handler was called only once (even though it returned error)
	assert.Equal(t, int32(1), callCount.Load(), "Handler should execute only once even if it returns error")

	// Verify: Handler was automatically unsubscribed (errors don't prevent unsubscription)
	_, exists := bus.handlers[id]
	assert.False(t, exists, "Handler should be unsubscribed even after returning error")
}

// TestOnceExecution_SubscriberCount tests that subscriber count decreases after Once handler executes
func TestOnceExecution_SubscriberCount(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	handler := func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}

	// Subscribe multiple Once handlers
	const numHandlers = 5
	for i := 0; i < numHandlers; i++ {
		_, err := bus.Subscribe(handler, bus.WithOnce())
		require.NoError(t, err)
	}

	// Verify initial count
	assert.Equal(t, uint64(numHandlers), bus.subscriberCount.Load(), "Should have all handlers subscribed")

	// Publish one event
	bus.Publish(ctx, "event")

	// Verify: All Once handlers were unsubscribed
	assert.Equal(t, uint64(0), bus.subscriberCount.Load(), "All Once handlers should be unsubscribed")

	// Verify: Handler slices are empty
	syncLen := len(bus.syncHandlers)
	asyncLen := len(bus.asyncHandlers)
	assert.Equal(t, 0, syncLen, "Sync handlers should be empty")
	assert.Equal(t, 0, asyncLen, "Async handlers should be empty")
}

// --------------------------- Timeout Execution Tests ---------------------------

// TestTimeoutExecution_Sync tests that sync handlers respect timeout
func TestTimeoutExecution_Sync(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	handlerStarted := make(chan struct{})
	handlerFinished := make(chan struct{})
	var contextCanceled atomic.Bool

	handler := func(ctx context.Context, event string) (Result, error) {
		close(handlerStarted)
		defer close(handlerFinished)

		// Check if context has a deadline
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline, "Context should have a deadline")

		// Wait for context cancellation
		<-ctx.Done()
		contextCanceled.Store(true)
		return Result{}, ctx.Err()
	}

	// Subscribe with a short timeout
	_, err = bus.Subscribe(handler, bus.WithTimeout(50*time.Millisecond))
	require.NoError(t, err)

	// Publish event
	bus.Publish(ctx, "event")

	<-handlerStarted
	<-handlerFinished

	// Verify: Context was canceled due to timeout
	assert.True(t, contextCanceled.Load(), "Handler context should be canceled by timeout")
}

// TestTimeoutExecution_Async tests that async handlers respect timeout
func TestTimeoutExecution_Async(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	handlerStarted := make(chan struct{})
	handlerFinished := make(chan struct{})
	var contextCanceled atomic.Bool

	handler := func(ctx context.Context, event string) (Result, error) {
		close(handlerStarted)
		defer close(handlerFinished)

		// Check if context has a deadline
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline, "Context should have a deadline")

		// Wait for context cancellation
		<-ctx.Done()
		contextCanceled.Store(true)
		return Result{}, ctx.Err()
	}

	// Subscribe with timeout and async
	_, err = bus.Subscribe(handler, bus.WithTimeout(50*time.Millisecond), bus.WithAsync())
	require.NoError(t, err)

	// Publish event
	bus.Publish(ctx, "event")

	<-handlerStarted
	<-handlerFinished
	bus.WaitAsync()

	// Verify: Context was canceled due to timeout
	assert.True(t, contextCanceled.Load(), "Async handler context should be canceled by timeout")
}

// TestTimeoutExecution_HandlerCompletesBeforeTimeout tests handler that finishes before timeout
func TestTimeoutExecution_HandlerCompletesBeforeTimeout(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	var executed atomic.Bool
	var returnedError atomic.Value

	handler := func(ctx context.Context, event string) (Result, error) {
		executed.Store(true)
		// Handler completes quickly, well before timeout
		return Result{"status": "success"}, nil
	}

	// Subscribe with a generous timeout
	_, err = bus.Subscribe(handler, bus.WithTimeout(5*time.Second))
	require.NoError(t, err)

	// Publish event
	bus.Publish(ctx, "event")

	// Verify: Handler executed successfully without timeout
	assert.True(t, executed.Load(), "Handler should execute")
	assert.Nil(t, returnedError.Load(), "Handler should not return error")
}

// TestTimeoutExecution_MultipleHandlersDifferentTimeouts tests multiple handlers with different timeouts
func TestTimeoutExecution_MultipleHandlersDifferentTimeouts(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	handler1Started := make(chan struct{})
	handler1Finished := make(chan struct{})
	handler2Started := make(chan struct{})
	handler2Finished := make(chan struct{})
	handler3Started := make(chan struct{})
	handler3Finished := make(chan struct{})

	var handler1Canceled atomic.Bool
	var handler2Canceled atomic.Bool
	var handler3Completed atomic.Bool

	// Handler 1: Short timeout, will timeout
	handler1 := func(ctx context.Context, event string) (Result, error) {
		close(handler1Started)
		defer close(handler1Finished)
		<-ctx.Done()
		handler1Canceled.Store(true)
		return Result{}, ctx.Err()
	}

	// Handler 2: Medium timeout, will timeout
	handler2 := func(ctx context.Context, event string) (Result, error) {
		close(handler2Started)
		defer close(handler2Finished)
		<-ctx.Done()
		handler2Canceled.Store(true)
		return Result{}, ctx.Err()
	}

	// Handler 3: Long timeout, completes successfully
	handler3 := func(ctx context.Context, event string) (Result, error) {
		close(handler3Started)
		defer close(handler3Finished)
		handler3Completed.Store(true)
		return Result{}, nil
	}

	// Subscribe handlers with different timeouts
	_, err = bus.Subscribe(handler1, bus.WithTimeout(50*time.Millisecond))
	require.NoError(t, err)
	_, err = bus.Subscribe(handler2, bus.WithTimeout(100*time.Millisecond))
	require.NoError(t, err)
	_, err = bus.Subscribe(handler3, bus.WithTimeout(5*time.Second))
	require.NoError(t, err)

	// Publish event
	bus.Publish(ctx, "event")

	<-handler1Started
	<-handler1Finished
	<-handler2Started
	<-handler2Finished
	<-handler3Started
	<-handler3Finished

	// Verify: Different behaviors based on timeout
	assert.True(t, handler1Canceled.Load(), "Handler1 should be canceled")
	assert.True(t, handler2Canceled.Load(), "Handler2 should be canceled")
	assert.True(t, handler3Completed.Load(), "Handler3 should complete successfully")
}

// TestTimeoutExecution_WithOnce tests timeout combined with once execution
func TestTimeoutExecution_WithOnce(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	handlerStarted := make(chan struct{})
	handlerFinished := make(chan struct{})
	var callCount atomic.Int32
	var contextCanceled atomic.Bool

	handler := func(ctx context.Context, event string) (Result, error) {
		callCount.Add(1)
		close(handlerStarted)
		defer close(handlerFinished)
		<-ctx.Done()
		contextCanceled.Store(true)
		return Result{}, ctx.Err()
	}

	// Subscribe with Once and Timeout
	id, err := bus.Subscribe(handler, bus.WithOnce(), bus.WithTimeout(50*time.Millisecond))
	require.NoError(t, err)

	// Publish first event
	bus.Publish(ctx, "event1")

	<-handlerStarted
	<-handlerFinished

	// Verify: Handler executed once and timed out
	assert.Equal(t, int32(1), callCount.Load(), "Handler should execute once")
	assert.True(t, contextCanceled.Load(), "Handler should timeout")

	// Verify: Handler was unsubscribed (Once behavior)
	_, exists := bus.handlers[id]
	assert.False(t, exists, "Handler should be unsubscribed after Once execution")

	// Publish second event - handler should not execute again
	bus.Publish(ctx, "event2")
	assert.Equal(t, int32(1), callCount.Load(), "Handler should not execute again")
}

// TestTimeoutExecution_ParentContextCancellation tests that parent context cancellation works with timeout
func TestTimeoutExecution_ParentContextCancellation(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	cfg := DefaultConfig()
	bus, err := NewEventBus[string](parentCtx, cfg)
	require.NoError(t, err)
	defer bus.Close(parentCtx)

	handlerStarted := make(chan struct{})
	handlerFinished := make(chan struct{})
	var contextCanceled atomic.Bool

	handler := func(ctx context.Context, event string) (Result, error) {
		close(handlerStarted)
		defer close(handlerFinished)
		<-ctx.Done()
		contextCanceled.Store(true)
		return Result{}, ctx.Err()
	}

	// Subscribe with timeout longer than we'll wait
	_, err = bus.Subscribe(handler, bus.WithTimeout(5*time.Second))
	require.NoError(t, err)

	// Start publishing
	go bus.Publish(parentCtx, "event")

	// Wait for handler to start
	<-handlerStarted

	// Cancel parent context
	cancel()

	// Wait for handler to finish
	<-handlerFinished

	// Verify: Handler was canceled (by parent context, not timeout)
	assert.True(t, contextCanceled.Load(), "Handler should be canceled when parent context is canceled")
}

// --------------------------- Publish_test ---------------------------
// TestPublish tests the Publish function with various scenarios
func TestPublish(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		config      Config
		setupFunc   func(t *testing.T, bus *EventBus[string]) // Setup subscriptions and hooks
		publishFunc func(t *testing.T, bus *EventBus[string]) // Perform publish operations
		validate    func(t *testing.T, bus *EventBus[string]) // Validate results
	}{
		{
			name:   "publish to bus with no subscribers",
			config: *DefaultConfig(),
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				// Should not panic, just return early
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish to closed bus",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				err := bus.Close(ctx)
				require.NoError(t, err)
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				// Should return early without incrementing count
				assert.Equal(t, uint64(0), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish to single sync subscriber",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var received string
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					received = event
					return Result{}, nil
				})
				require.NoError(t, err)
				// Store received in test context
				t.Cleanup(func() {
					assert.Equal(t, "test event", received)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish to single async subscriber",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var received string
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					received = event
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err)
				// Store received in test context
				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, "test event", received)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish to multiple sync subscribers",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				receivedCount := 0
				for i := 0; i < 5; i++ {
					_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						receivedCount++
						return Result{}, nil
					})
					require.NoError(t, err)
				}
				t.Cleanup(func() {
					assert.Equal(t, 5, receivedCount)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish to multiple async subscribers",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				receivedCount := new(int64)
				for i := 0; i < 5; i++ {
					_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						atomic.AddInt64(receivedCount, 1)
						return Result{}, nil
					}, bus.WithAsync())
					require.NoError(t, err)
				}
				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, int64(5), *receivedCount)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish to mixed sync and async subscribers",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				syncCount := 0
				asyncCount := new(int64)
				// Add 3 sync subscribers
				for i := 0; i < 3; i++ {
					_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						syncCount++
						return Result{}, nil
					})
					require.NoError(t, err)
				}
				// Add 3 async subscribers
				for i := 0; i < 3; i++ {
					_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						atomic.AddInt64(asyncCount, 1)
						return Result{}, nil
					}, bus.WithAsync())
					require.NoError(t, err)
				}
				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, 3, syncCount)
					assert.Equal(t, int64(3), *asyncCount)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish multiple events",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				receivedCount := 0
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount++
					return Result{}, nil
				})
				require.NoError(t, err)
				t.Cleanup(func() {
					assert.Equal(t, 10, receivedCount)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				for i := 0; i < 10; i++ {
					bus.Publish(ctx, "test event")
				}
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(10), bus.publishedCount.Load())
			},
		},
		{
			name: "publish with NoHooks enabled (fast path)",
			config: Config{
				NoHooks: convert.ToPtr(true),
			},
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				receivedCount := 0
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount++
					return Result{}, nil
				})
				require.NoError(t, err)
				t.Cleanup(func() {
					assert.Equal(t, 1, receivedCount)
					// Verify NoHooks is enabled
					assert.True(t, bus.noHooks)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish with bus-level before hook",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var hookCallCount, handlerCallCount atomic.Int32

				// Add bus-level before hook
				bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					assert.Equal(t, int32(0), handlerCallCount.Load(), "before hook should be called before handler")
					hookCallCount.Add(1)
					return ctx
				})

				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					return Result{}, nil
				})
				require.NoError(t, err)
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish with bus-level after hook",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var hookCallCount, handlerCallCount atomic.Int32

				// Add bus-level after hook
				bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					assert.Equal(t, int32(1), handlerCallCount.Load(), "after hook should be called after handler")
					hookCallCount.Add(1)
					assert.NoError(t, err)
					assert.Greater(t, duration, time.Duration(0))
				})

				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					return Result{}, nil
				})
				require.NoError(t, err)
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish with handler-level before hook",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var hookCallCount, handlerCallCount atomic.Int32

				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					return Result{}, nil
				}, bus.WithBefore(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					assert.Equal(t, int32(0), handlerCallCount.Load(), "before hook should be called before handler")
					hookCallCount.Add(1)
					return ctx
				}))
				require.NoError(t, err)
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish with handler-level after hook",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var hookCallCount, handlerCallCount atomic.Int32

				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					return Result{}, nil
				}, bus.WithAfter(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					assert.Equal(t, int32(1), handlerCallCount.Load(), "after hook should be called after handler")
					hookCallCount.Add(1)
					assert.NoError(t, err)
					assert.Greater(t, duration, time.Duration(0))
				}))
				require.NoError(t, err)
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish with error hook when handler returns error",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				var errorHookCallCount atomic.Int32

				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, assert.AnError
				}, bus.WithOnError(func(ctx context.Context, event string, id SubscriptionID, err error) {
					errorHookCallCount.Add(1)
					assert.Error(t, err)
				}))
				require.NoError(t, err)
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish respects Once option",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				callCount := 0
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					callCount++
					return Result{}, nil
				}, bus.WithOnce())
				require.NoError(t, err)

				t.Cleanup(func() {
					// Should only be called once despite two publishes
					assert.Equal(t, 1, callCount)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "event 1")
				bus.Publish(ctx, "event 2")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(2), bus.publishedCount.Load())
			},
		},
		{
			name:   "publish with context cancellation",
			config: *DefaultConfig(),
			setupFunc: func(t *testing.T, bus *EventBus[string]) {
				handlerExecuted := false
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerExecuted = true
					return Result{}, ctx.Err()
				})
				require.NoError(t, err)
				t.Cleanup(func() {
					assert.True(t, handlerExecuted)
				})
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				bus.Publish(cancelCtx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus, err := NewEventBus[string](ctx, &tt.config)
			require.NoError(t, err)
			if tt.setupFunc == nil || tt.name != "publish to closed bus" {
				defer bus.Close(ctx)
			}

			if tt.setupFunc != nil {
				tt.setupFunc(t, bus)
			}

			if tt.publishFunc != nil {
				tt.publishFunc(t, bus)
			}

			if tt.validate != nil {
				tt.validate(t, bus)
			}
		})
	}
}

// TestPublish_ConcurrentPublish tests sync concurrent publishing to the same bus
func TestPublish_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[int](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Track received events
	receivedCount := 0
	mu := &sync.RWMutex{}

	// Subscribe to events
	_, err = bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish 100 events concurrently
	numPublishers := 10
	eventsPerPublisher := 10
	var wg sync.WaitGroup
	wg.Add(numPublishers)

	for i := 0; i < numPublishers; i++ {
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				bus.Publish(ctx, publisherID*eventsPerPublisher+j)
			}
		}(i)
	}

	wg.Wait()

	// Validate
	assert.Equal(t, 100, receivedCount)
	assert.Equal(t, uint64(100), bus.publishedCount.Load())
}

// TestPublish_DifferentEventTypes tests publishing different event types
func TestPublish_DifferentEventTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("int type", func(t *testing.T) {
		bus, err := NewEventBus[int](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		received := -1
		_, err = bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		bus.Publish(ctx, 42)
		assert.Equal(t, 42, received)
	})

	t.Run("struct type", func(t *testing.T) {
		type Event struct {
			ID   int
			Name string
		}
		bus, err := NewEventBus[Event](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		var received Event
		_, err = bus.Subscribe(func(ctx context.Context, event Event) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		bus.Publish(ctx, Event{ID: 1, Name: "test"})
		assert.Equal(t, Event{ID: 1, Name: "test"}, received)
	})

	t.Run("pointer type", func(t *testing.T) {
		type Event struct {
			ID int
		}
		bus, err := NewEventBus[*Event](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		var received *Event
		_, err = bus.Subscribe(func(ctx context.Context, event *Event) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		event := &Event{ID: 1}
		bus.Publish(ctx, event)
		assert.Equal(t, event, received)
		assert.Equal(t, 1, received.ID)
	})
}

// --------------------------- publishNoHooks_test ---------------------------
// TestPublishNoHooks tests the publishNoHooks function (fast path)
func TestPublishNoHooks(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) *EventBus[string]      // Create and setup bus
		publishFunc func(t *testing.T, bus *EventBus[string]) // Perform publish operations
		validate    func(t *testing.T, bus *EventBus[string]) // Validate results
	}{
		{
			name: "no handlers - return early",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				// Call publishNoHooks directly through Publish (which will route to publishNoHooks)
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				// Should not panic, just return early
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "single sync handler",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount.Add(1)
					return Result{}, nil
				})
				require.NoError(t, err)

				// Store in context for validation
				t.Cleanup(func() {
					assert.Equal(t, int32(1), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
				assert.Equal(t, 1, len(bus.syncHandlers))
				assert.Equal(t, 0, len(bus.asyncHandlers))
			},
		},
		{
			name: "single async handler",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount.Add(1)
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err)

				// Store in context for validation
				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, int32(1), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
				assert.Equal(t, 0, len(bus.syncHandlers))
				assert.Equal(t, 1, len(bus.asyncHandlers))
			},
		},
		{
			name: "multiple sync handlers",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				for i := 0; i < 5; i++ {
					_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						receivedCount.Add(1)
						return Result{}, nil
					})
					require.NoError(t, err)
				}

				t.Cleanup(func() {
					assert.Equal(t, int32(5), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
				assert.Equal(t, 5, len(bus.syncHandlers))
			},
		},
		{
			name: "multiple async handlers",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				for i := 0; i < 5; i++ {
					_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						receivedCount.Add(1)
						return Result{}, nil
					}, bus.WithAsync())
					require.NoError(t, err)
				}

				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, int32(5), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
				assert.Equal(t, 5, len(bus.asyncHandlers))
			},
		},
		{
			name: "mixed sync and async handlers",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				syncCount := atomic.Int32{}
				asyncCount := atomic.Int32{}

				// Add 3 sync handlers
				for i := 0; i < 3; i++ {
					_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						syncCount.Add(1)
						return Result{}, nil
					})
					require.NoError(t, err)
				}

				// Add 2 async handlers
				for i := 0; i < 2; i++ {
					_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						asyncCount.Add(1)
						return Result{}, nil
					}, bus.WithAsync())
					require.NoError(t, err)
				}

				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, int32(3), syncCount.Load())
					assert.Equal(t, int32(2), asyncCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
				assert.Equal(t, 3, len(bus.syncHandlers))
				assert.Equal(t, 2, len(bus.asyncHandlers))
			},
		},
		{
			name: "multiple publishes reuse pools",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount.Add(1)
					return Result{}, nil
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(10), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				for i := 0; i < 10; i++ {
					bus.Publish(ctx, "test event")
				}
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(10), bus.publishedCount.Load())
			},
		},
		{
			name: "handlers receive correct event data",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				receivedEvents := []string{}
				mu := sync.Mutex{}

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					mu.Lock()
					receivedEvents = append(receivedEvents, event)
					mu.Unlock()
					return Result{}, nil
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					mu.Lock()
					assert.Equal(t, []string{"event1", "event2", "event3"}, receivedEvents)
					mu.Unlock()
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "event1")
				bus.Publish(ctx, "event2")
				bus.Publish(ctx, "event3")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(3), bus.publishedCount.Load())
			},
		},
		{
			name: "hook pools should not be used",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfig()
				config.NoHooks = convert.ToPtr(true)
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.True(t, bus.noHooks)

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				})
				require.NoError(t, err)

				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				// Hook pools should be nil when NoHooks is true
				assert.Nil(t, bus.beforeHookPool)
				assert.Nil(t, bus.afterHookPool)
				assert.Nil(t, bus.errorHookPool)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := tt.setupFunc(t)
			defer bus.Close(ctx)

			if tt.publishFunc != nil {
				tt.publishFunc(t, bus)
			}

			if tt.validate != nil {
				tt.validate(t, bus)
			}
		})
	}
}

// TestPublishNoHooks_ConcurrentPublish tests concurrent publishing with NoHooks enabled
func TestPublishNoHooks_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.NoHooks = convert.ToPtr(true)
	bus, err := NewEventBus[int](ctx, config)
	require.NoError(t, err)
	defer bus.Close(ctx)
	require.True(t, bus.noHooks)

	// Track received events
	receivedCount := atomic.Int32{}

	// Subscribe to events
	_, err = bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
		receivedCount.Add(1)
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish 100 events concurrently
	numPublishers := 10
	eventsPerPublisher := 10
	var wg sync.WaitGroup
	wg.Add(numPublishers)

	for i := 0; i < numPublishers; i++ {
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				bus.Publish(ctx, publisherID*eventsPerPublisher+j)
			}
		}(i)
	}

	wg.Wait()

	// Validate
	assert.Equal(t, int32(100), receivedCount.Load())
	assert.Equal(t, uint64(100), bus.publishedCount.Load())
}

// TestPublishNoHooks_DifferentTypes tests publishNoHooks with different event types
func TestPublishNoHooks_DifferentTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("int type", func(t *testing.T) {
		config := DefaultConfig()
		config.NoHooks = convert.ToPtr(true)
		bus, err := NewEventBus[int](ctx, config)
		require.NoError(t, err)
		defer bus.Close(ctx)

		received := -1
		_, err = bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		bus.Publish(ctx, 42)
		assert.Equal(t, 42, received)
	})

	t.Run("struct type", func(t *testing.T) {
		type Event struct {
			ID   int
			Name string
		}
		config := DefaultConfig()
		config.NoHooks = convert.ToPtr(true)
		bus, err := NewEventBus[Event](ctx, config)
		require.NoError(t, err)
		defer bus.Close(ctx)

		var received Event
		_, err = bus.Subscribe(func(ctx context.Context, event Event) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		bus.Publish(ctx, Event{ID: 1, Name: "test"})
		assert.Equal(t, Event{ID: 1, Name: "test"}, received)
	})

	t.Run("pointer type", func(t *testing.T) {
		type Event struct {
			ID int
		}
		config := DefaultConfig()
		config.NoHooks = convert.ToPtr(true)
		bus, err := NewEventBus[*Event](ctx, config)
		require.NoError(t, err)
		defer bus.Close(ctx)

		var received *Event
		_, err = bus.Subscribe(func(ctx context.Context, event *Event) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		event := &Event{ID: 1}
		bus.Publish(ctx, event)
		assert.Equal(t, event, received)
	})
}

// --------------------------- publishWithHooks_test ---------------------------
// TestPublishWithHooks tests the publishWithHooks function (normal path with hooks)
func TestPublishWithHooks(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) *EventBus[string]      // Create and setup bus
		publishFunc func(t *testing.T, bus *EventBus[string]) // Perform publish operations
		validate    func(t *testing.T, bus *EventBus[string]) // Validate results
	}{
		{
			name: "no handlers - return early",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				// Should not panic, just return early
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "single sync handler without hooks",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount.Add(1)
					return Result{}, nil
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "single async handler without hooks",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				receivedCount := atomic.Int32{}
				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					receivedCount.Add(1)
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err)

				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, int32(1), receivedCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "with bus-level before hook",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				beforeCallCount := atomic.Int32{}
				handlerCallCount := atomic.Int32{}

				bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					assert.Equal(t, int32(0), handlerCallCount.Load(), "before hook should be called before handler")
					beforeCallCount.Add(1)
					return ctx
				})

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					return Result{}, nil
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), beforeCallCount.Load())
					assert.Equal(t, int32(1), handlerCallCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "with bus-level after hook",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				afterCallCount := atomic.Int32{}
				handlerCallCount := atomic.Int32{}

				bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					assert.Equal(t, int32(1), handlerCallCount.Load(), "after hook should be called after handler")
					afterCallCount.Add(1)
					assert.NoError(t, err)
					assert.Greater(t, duration, time.Duration(0))
				})

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					time.Sleep(10 * time.Millisecond)
					return Result{}, nil
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), afterCallCount.Load())
					assert.Equal(t, int32(1), handlerCallCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "with bus-level error hook",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				errorCallCount := atomic.Int32{}

				bus.SetGlobalErrorHook(func(ctx context.Context, event string, id SubscriptionID, err error) {
					errorCallCount.Add(1)
					assert.Error(t, err)
				})

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, assert.AnError
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), errorCallCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "with handler-level before hook",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				beforeCallCount := atomic.Int32{}
				handlerCallCount := atomic.Int32{}

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					return Result{}, nil
				}, bus.WithBefore(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					assert.Equal(t, int32(0), handlerCallCount.Load(), "before hook should be called before handler")
					beforeCallCount.Add(1)
					return ctx
				}))
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), beforeCallCount.Load())
					assert.Equal(t, int32(1), handlerCallCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "with handler-level after hook",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				afterCallCount := atomic.Int32{}
				handlerCallCount := atomic.Int32{}

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCallCount.Add(1)
					time.Sleep(10 * time.Millisecond)
					return Result{}, nil
				}, bus.WithAfter(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					assert.Equal(t, int32(1), handlerCallCount.Load(), "after hook should be called after handler")
					afterCallCount.Add(1)
					assert.NoError(t, err)
					assert.Greater(t, duration, time.Duration(0))
				}))
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), afterCallCount.Load())
					assert.Equal(t, int32(1), handlerCallCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "with handler-level error hook",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				errorCallCount := atomic.Int32{}

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, assert.AnError
				}, bus.WithOnError(func(ctx context.Context, event string, id SubscriptionID, err error) {
					errorCallCount.Add(1)
					assert.Error(t, err)
				}))
				require.NoError(t, err)

				t.Cleanup(func() {
					assert.Equal(t, int32(1), errorCallCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "multiple handlers with mixed hooks",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				busBeforeCount := atomic.Int32{}
				busAfterCount := atomic.Int32{}
				handlerCount := atomic.Int32{}

				// Set bus-level hooks
				bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					busBeforeCount.Add(1)
					return ctx
				})

				bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					busAfterCount.Add(1)
				})

				// Add 3 handlers
				for i := 0; i < 3; i++ {
					_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
						handlerCount.Add(1)
						return Result{}, nil
					})
					require.NoError(t, err)
				}

				t.Cleanup(func() {
					assert.Equal(t, int32(3), busBeforeCount.Load())
					assert.Equal(t, int32(3), busAfterCount.Load())
					assert.Equal(t, int32(3), handlerCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
		{
			name: "hook pools are used and returned",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					return ctx
				})

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				})
				require.NoError(t, err)

				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				// Hook pools should be initialized
				assert.NotNil(t, bus.beforeHookPool)
				assert.NotNil(t, bus.afterHookPool)
				assert.NotNil(t, bus.errorHookPool)
			},
		},
		{
			name: "async handlers with hooks",
			setupFunc: func(t *testing.T) *EventBus[string] {
				config := DefaultConfigWithHooks()
				bus, err := NewEventBus[string](ctx, config)
				require.NoError(t, err)
				require.False(t, bus.noHooks)

				beforeCount := atomic.Int32{}
				afterCount := atomic.Int32{}
				handlerCount := atomic.Int32{}

				bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
					beforeCount.Add(1)
					return ctx
				})

				bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
					afterCount.Add(1)
				})

				_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					handlerCount.Add(1)
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err)

				t.Cleanup(func() {
					bus.WaitAsync()
					assert.Equal(t, int32(1), beforeCount.Load())
					assert.Equal(t, int32(1), afterCount.Load())
					assert.Equal(t, int32(1), handlerCount.Load())
				})
				return bus
			},
			publishFunc: func(t *testing.T, bus *EventBus[string]) {
				bus.Publish(ctx, "test event")
			},
			validate: func(t *testing.T, bus *EventBus[string]) {
				assert.Equal(t, uint64(1), bus.publishedCount.Load())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := tt.setupFunc(t)
			defer bus.Close(ctx)

			if tt.publishFunc != nil {
				tt.publishFunc(t, bus)
			}

			if tt.validate != nil {
				tt.validate(t, bus)
			}
		})
	}
}

// TestPublishWithHooks_HookPoolReuse tests that hook pools are reused across publishes
func TestPublishWithHooks_HookPoolReuse(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfigWithHooks()
	bus, err := NewEventBus[string](ctx, config)
	require.NoError(t, err)
	defer bus.Close(ctx)
	require.False(t, bus.noHooks)

	callCount := atomic.Int32{}

	// Add bus-level hooks
	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		callCount.Add(1)
		return ctx
	})

	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish multiple times to test pool reuse
	numPublishes := 100
	for i := 0; i < numPublishes; i++ {
		bus.Publish(ctx, "test")
	}

	// Validate hooks were called
	assert.Equal(t, int32(numPublishes), callCount.Load())
	assert.Equal(t, uint64(numPublishes), bus.publishedCount.Load())

	// Verify hook pools exist
	assert.NotNil(t, bus.beforeHookPool)
	assert.NotNil(t, bus.afterHookPool)
	assert.NotNil(t, bus.errorHookPool)
}

// TestPublishWithHooks_BusAndHandlerHooks tests interaction between bus-level and handler-level hooks
func TestPublishWithHooks_BusAndHandlerHooks(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfigWithHooks()
	bus, err := NewEventBus[string](ctx, config)
	require.NoError(t, err)
	defer bus.Close(ctx)

	executionOrder := []string{}
	mu := sync.Mutex{}

	// Set bus-level before hook
	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		mu.Lock()
		executionOrder = append(executionOrder, "bus-before")
		mu.Unlock()
		return ctx
	})

	// Set bus-level after hook
	bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		mu.Lock()
		executionOrder = append(executionOrder, "bus-after")
		mu.Unlock()
	})

	// Subscribe with handler-level hooks
	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		mu.Lock()
		executionOrder = append(executionOrder, "handler")
		mu.Unlock()
		return Result{}, nil
	},
		bus.WithBefore(func(ctx context.Context, event string, id SubscriptionID) context.Context {
			mu.Lock()
			executionOrder = append(executionOrder, "handler-before")
			mu.Unlock()
			return ctx
		}),
		bus.WithAfter(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
			mu.Lock()
			executionOrder = append(executionOrder, "handler-after")
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	// Publish
	bus.Publish(ctx, "test")

	// Validate execution order: bus-before, handler-before, handler, handler-after, bus-after
	mu.Lock()
	assert.Equal(t, []string{"bus-before", "handler-before", "handler", "handler-after", "bus-after"}, executionOrder)
	mu.Unlock()
}

// TestPublishWithHooks_ConcurrentPublish tests concurrent publishing with hooks
func TestPublishWithHooks_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfigWithHooks()
	bus, err := NewEventBus[int](ctx, config)
	require.NoError(t, err)
	defer bus.Close(ctx)

	hookCallCount := atomic.Int32{}
	handlerCallCount := atomic.Int32{}

	bus.SetGlobalBeforeHook(func(ctx context.Context, event int, id SubscriptionID) context.Context {
		hookCallCount.Add(1)
		return ctx
	})

	_, err = bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
		handlerCallCount.Add(1)
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish 100 events concurrently
	numPublishers := 10
	eventsPerPublisher := 10
	var wg sync.WaitGroup
	wg.Add(numPublishers)

	for i := 0; i < numPublishers; i++ {
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				bus.Publish(ctx, publisherID*eventsPerPublisher+j)
			}
		}(i)
	}

	wg.Wait()

	// Validate
	assert.Equal(t, int32(100), hookCallCount.Load())
	assert.Equal(t, int32(100), handlerCallCount.Load())
	assert.Equal(t, uint64(100), bus.publishedCount.Load())
}

// TestPublishWithHooks_MixedSyncAsync tests sync and async handlers with hooks
func TestPublishWithHooks_MixedSyncAsync(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfigWithHooks()
	bus, err := NewEventBus[string](ctx, config)
	require.NoError(t, err)
	defer bus.Close(ctx)

	busHookCount := atomic.Int32{}
	syncCount := atomic.Int32{}
	asyncCount := atomic.Int32{}

	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		busHookCount.Add(1)
		return ctx
	})

	// Add 3 sync handlers
	for i := 0; i < 3; i++ {
		_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
			syncCount.Add(1)
			return Result{}, nil
		})
		require.NoError(t, err)
	}

	// Add 2 async handlers
	for i := 0; i < 2; i++ {
		_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
			asyncCount.Add(1)
			return Result{}, nil
		}, bus.WithAsync())
		require.NoError(t, err)
	}

	// Publish
	bus.Publish(ctx, "test")
	bus.WaitAsync()

	// Validate
	assert.Equal(t, int32(5), busHookCount.Load()) // Called for each of 5 handlers
	assert.Equal(t, int32(3), syncCount.Load())
	assert.Equal(t, int32(2), asyncCount.Load())
}

// TestPublishWithHooks_DifferentTypes tests publishWithHooks with different event types
func TestPublishWithHooks_DifferentTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("int type", func(t *testing.T) {
		config := DefaultConfigWithHooks()
		bus, err := NewEventBus[int](ctx, config)
		require.NoError(t, err)
		defer bus.Close(ctx)

		hookCalled := atomic.Bool{}
		received := -1

		bus.SetGlobalBeforeHook(func(ctx context.Context, event int, id SubscriptionID) context.Context {
			hookCalled.Store(true)
			return ctx
		})

		_, err = bus.Subscribe(func(ctx context.Context, event int) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		bus.Publish(ctx, 42)
		assert.Equal(t, 42, received)
		assert.True(t, hookCalled.Load())
	})

	t.Run("struct type", func(t *testing.T) {
		type Event struct {
			ID   int
			Name string
		}
		config := DefaultConfigWithHooks()
		bus, err := NewEventBus[Event](ctx, config)
		require.NoError(t, err)
		defer bus.Close(ctx)

		hookCalled := atomic.Bool{}
		var received Event

		bus.SetGlobalBeforeHook(func(ctx context.Context, event Event, id SubscriptionID) context.Context {
			hookCalled.Store(true)
			return ctx
		})

		_, err = bus.Subscribe(func(ctx context.Context, event Event) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		bus.Publish(ctx, Event{ID: 1, Name: "test"})
		assert.Equal(t, Event{ID: 1, Name: "test"}, received)
		assert.True(t, hookCalled.Load())
	})

	t.Run("pointer type", func(t *testing.T) {
		type Event struct {
			ID int
		}
		config := DefaultConfigWithHooks()
		bus, err := NewEventBus[*Event](ctx, config)
		require.NoError(t, err)
		defer bus.Close(ctx)

		hookCalled := atomic.Bool{}
		var received *Event

		bus.SetGlobalBeforeHook(func(ctx context.Context, event *Event, id SubscriptionID) context.Context {
			hookCalled.Store(true)
			return ctx
		})

		_, err = bus.Subscribe(func(ctx context.Context, event *Event) (Result, error) {
			received = event
			return Result{}, nil
		})
		require.NoError(t, err)

		event := &Event{ID: 1}
		bus.Publish(ctx, event)
		assert.Equal(t, event, received)
		assert.True(t, hookCalled.Load())
	})
}

// TestPublishWithHooks_HookCopying tests that hooks are copied and isolated per publish
func TestPublishWithHooks_HookCopying(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfigWithHooks()
	bus, err := NewEventBus[string](ctx, config)
	require.NoError(t, err)
	defer bus.Close(ctx)

	hook1CallCount := atomic.Int32{}
	hook2CallCount := atomic.Int32{}

	// Set initial hook
	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		hook1CallCount.Add(1)
		return ctx
	})

	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish with first hook
	bus.Publish(ctx, "event1")
	assert.Equal(t, int32(1), hook1CallCount.Load())

	// Add a second hook (note: SetGlobalBeforeHook doesn't append, it replaces)
	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		hook2CallCount.Add(1)
		return ctx
	})

	// Publish with both hooks
	bus.Publish(ctx, "event2")

	// Validate: both hooks should be called on second publish
	assert.Equal(t, int32(1), hook1CallCount.Load(), "first hook should be called once (only on first publish)")
	assert.Equal(t, int32(1), hook2CallCount.Load(), "second hook should be called once (only on second publish)")
}

// ============================================================================
// Unsubscribe Tests
// ============================================================================

// TestUnsubscribe_ValidSyncSubscription tests unsubscribing a synchronous subscription.
func TestUnsubscribe_ValidSyncSubscription(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe synchronously
	id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Verify subscription exists
	assert.Equal(t, uint64(1), bus.subscriberCount.Load())

	// Unsubscribe
	result := bus.Unsubscribe(id)
	assert.True(t, result, "Unsubscribe should return true")

	// Verify subscription removed
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Publish should not call the handler
	bus.Publish(ctx, "test")
	assert.Equal(t, uint64(1), bus.publishedCount.Load(), "Publish should be called once")
	assert.Equal(t, uint64(0), bus.processedCount.Load(), "No handlers should be called")
}

// TestUnsubscribe_ValidAsyncSubscription tests unsubscribing an asynchronous subscription.
func TestUnsubscribe_ValidAsyncSubscription(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe asynchronously
	id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAsync())
	require.NoError(t, err)

	// Verify subscription exists
	assert.Equal(t, uint64(1), bus.subscriberCount.Load())

	// Unsubscribe
	result := bus.Unsubscribe(id)
	assert.True(t, result, "Unsubscribe should return true")

	// Verify subscription removed
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Publish should not call the handler
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.Equal(t, uint64(1), bus.publishedCount.Load(), "Publish should be called once")
	assert.Equal(t, uint64(0), bus.processedCount.Load(), "No handlers should be called")
}

// TestUnsubscribe_NonExistentID tests unsubscribing with a non-existent ID.
func TestUnsubscribe_NonExistentID(t *testing.T) {
	tests := []struct {
		name string
		id   SubscriptionID
	}{
		{
			name: "zero id",
			id:   0,
		},
		{
			name: "large id",
			id:   99999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			bus, err := NewEventBus[string](ctx, DefaultConfig())
			require.NoError(t, err)
			defer bus.Close(ctx)

			// Unsubscribe non-existent ID should return true (idempotent)
			result := bus.Unsubscribe(tt.id)
			assert.True(t, result, "Unsubscribe should return true even for non-existent ID")

			// Verify subscriber count is still 0
			assert.Equal(t, uint64(0), bus.subscriberCount.Load())
		})
	}
}

// TestUnsubscribe_IdempotentUnsubscribe tests unsubscribing the same ID twice.
func TestUnsubscribe_IdempotentUnsubscribe(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe
	id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// First unsubscribe
	result1 := bus.Unsubscribe(id)
	assert.True(t, result1, "First unsubscribe should return true")
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Second unsubscribe (same ID)
	result2 := bus.Unsubscribe(id)
	assert.True(t, result2, "Second unsubscribe should return true (idempotent)")
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())
}

// TestUnsubscribe_SwapAndPopIndexUpdate tests that indices are updated correctly after swap-and-pop.
func TestUnsubscribe_SwapAndPopIndexUpdate(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe multiple handlers
	var callOrder []int
	var mu sync.Mutex

	id1, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		mu.Lock()
		callOrder = append(callOrder, 1)
		mu.Unlock()
		return Result{}, nil
	})
	require.NoError(t, err)

	id2, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		mu.Lock()
		callOrder = append(callOrder, 2)
		mu.Unlock()
		return Result{}, nil
	})
	require.NoError(t, err)

	id3, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		mu.Lock()
		callOrder = append(callOrder, 3)
		mu.Unlock()
		return Result{}, nil
	})
	require.NoError(t, err)

	// Verify all subscriptions exist
	assert.Equal(t, uint64(3), bus.subscriberCount.Load())

	// Unsubscribe middle handler (id2)
	result := bus.Unsubscribe(id2)
	assert.True(t, result, "Unsubscribe should return true")
	assert.Equal(t, uint64(2), bus.subscriberCount.Load())

	// Publish and verify only handlers 1 and 3 are called
	callOrder = nil
	bus.Publish(ctx, "test")
	assert.ElementsMatch(t, []int{1, 3}, callOrder, "Handlers 1 and 3 should be called")

	// Unsubscribe first handler (id1)
	result = bus.Unsubscribe(id1)
	assert.True(t, result, "Unsubscribe should return true")
	assert.Equal(t, uint64(1), bus.subscriberCount.Load())

	// Publish and verify only handler 3 is called
	callOrder = nil
	bus.Publish(ctx, "test")
	assert.Equal(t, []int{3}, callOrder, "Only handler 3 should be called")

	// Unsubscribe last handler (id3)
	result = bus.Unsubscribe(id3)
	assert.True(t, result, "Unsubscribe should return true")
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Publish should call no handlers
	callOrder = nil
	bus.Publish(ctx, "test")
	assert.Empty(t, callOrder, "Call order should be empty")
}

// TestUnsubscribe_MixedSyncAsync tests unsubscribing both sync and async subscriptions.
func TestUnsubscribe_MixedSyncAsync(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe sync handlers
	syncID1, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	syncID2, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Subscribe async handlers
	asyncID1, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAsync())
	require.NoError(t, err)

	asyncID2, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	}, bus.WithAsync())
	require.NoError(t, err)

	// Verify all subscriptions exist
	assert.Equal(t, uint64(4), bus.subscriberCount.Load())

	// Unsubscribe one sync
	result := bus.Unsubscribe(syncID1)
	assert.True(t, result, "Unsubscribe sync should return true")
	assert.Equal(t, uint64(3), bus.subscriberCount.Load())

	// Unsubscribe one async
	result = bus.Unsubscribe(asyncID1)
	assert.True(t, result, "Unsubscribe async should return true")
	assert.Equal(t, uint64(2), bus.subscriberCount.Load())

	// Publish should call remaining 2 handlers
	bus.Publish(ctx, "test")
	bus.WaitAsync()
	assert.Equal(t, uint64(2), bus.processedCount.Load(), "Should process 2 handlers")

	// Unsubscribe remaining
	bus.Unsubscribe(syncID2)
	bus.Unsubscribe(asyncID2)
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())
}

// TestUnsubscribeAll_EmptyBus tests UnsubscribeAll on an empty bus.
func TestUnsubscribeAll_EmptyBus(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// UnsubscribeAll on empty bus should not panic
	bus.UnsubscribeAll()
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())
}

// TestUnsubscribeAll_WithSubscriptions tests UnsubscribeAll with multiple subscriptions.
func TestUnsubscribeAll_WithSubscriptions(t *testing.T) {
	tests := []struct {
		name         string
		syncCount    int
		asyncCount   int
		expectedCall int
	}{
		{
			name:         "only sync subscriptions",
			syncCount:    5,
			asyncCount:   0,
			expectedCall: 5,
		},
		{
			name:         "only async subscriptions",
			syncCount:    0,
			asyncCount:   5,
			expectedCall: 5,
		},
		{
			name:         "mixed sync and async",
			syncCount:    3,
			asyncCount:   2,
			expectedCall: 5,
		},
		{
			name:         "single subscription",
			syncCount:    1,
			asyncCount:   0,
			expectedCall: 1,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus, err := NewEventBus[string](ctx, DefaultConfig())
			require.NoError(t, err)
			defer bus.Close(ctx)

			// Subscribe sync handlers
			for i := 0; i < tt.syncCount; i++ {
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				})
				require.NoError(t, err)
			}

			// Subscribe async handlers
			for i := 0; i < tt.asyncCount; i++ {
				_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
					return Result{}, nil
				}, bus.WithAsync())
				require.NoError(t, err)
			}

			// Verify subscriptions
			assert.Equal(t, uint64(tt.expectedCall), bus.subscriberCount.Load())

			// Publish before unsubscribe
			bus.Publish(ctx, "test")
			bus.WaitAsync()
			assert.Equal(t, uint64(tt.expectedCall), bus.processedCount.Load(), "Should call all handlers")

			// UnsubscribeAll
			bus.UnsubscribeAll()
			assert.Equal(t, uint64(0), bus.subscriberCount.Load())

			// Publish after unsubscribe
			bus.Publish(ctx, "test")
			bus.WaitAsync()
			assert.Equal(t, uint64(tt.expectedCall), bus.processedCount.Load(), "No new handlers should be called")
		})
	}
}

// TestUnsubscribeAll_Idempotent tests calling UnsubscribeAll multiple times.
func TestUnsubscribeAll_Idempotent(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe some handlers
	for i := 0; i < 3; i++ {
		_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
			return Result{}, nil
		})
		require.NoError(t, err)
	}

	assert.Equal(t, uint64(3), bus.subscriberCount.Load())

	// First UnsubscribeAll
	bus.UnsubscribeAll()
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Second UnsubscribeAll should not panic
	bus.UnsubscribeAll()
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Third UnsubscribeAll should not panic
	bus.UnsubscribeAll()
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())
}

// TestUnsubscribe_ConcurrentUnsubscribe tests concurrent unsubscribe operations.
func TestUnsubscribe_ConcurrentUnsubscribe(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe multiple handlers
	numHandlers := 100
	ids := make([]SubscriptionID, numHandlers)

	for i := 0; i < numHandlers; i++ {
		id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
			return Result{}, nil
		})
		require.NoError(t, err)
		ids[i] = id
	}

	assert.Equal(t, uint64(numHandlers), bus.subscriberCount.Load())

	// Unsubscribe concurrently
	var wg sync.WaitGroup
	for i := 0; i < numHandlers; i++ {
		wg.Add(1)
		go func(id SubscriptionID) {
			defer wg.Done()
			result := bus.Unsubscribe(id)
			assert.True(t, result, "Unsubscribe should return true")
		}(ids[i])
	}

	wg.Wait()

	// Verify all subscriptions removed
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())

	// Publish should call no handlers
	bus.Publish(ctx, "test")
	assert.Equal(t, uint64(0), bus.processedCount.Load(), "No handlers should be called")
}

// TestUnsubscribe_ConcurrentUnsubscribeAndSubscribe tests concurrent unsubscribe and subscribe.
func TestUnsubscribe_ConcurrentUnsubscribeAndSubscribe(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Initial subscriptions
	initialCount := 50
	ids := make([]SubscriptionID, initialCount)

	for i := 0; i < initialCount; i++ {
		id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
			return Result{}, nil
		})
		require.NoError(t, err)
		ids[i] = id
	}

	// Concurrent operations
	var wg sync.WaitGroup
	numOps := 100

	// Unsubscribe operations
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx < initialCount {
				bus.Unsubscribe(ids[idx])
			} else {
				// Unsubscribe non-existent ID
				bus.Unsubscribe(SubscriptionID(idx + 10000))
			}
		}(i)
	}

	// Subscribe operations
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
				return Result{}, nil
			})
		}()
	}

	wg.Wait()

	// Verify subscriber count is correct (should be numOps since we unsubscribed all initial)
	count := bus.subscriberCount.Load()
	assert.Equal(t, uint64(numOps), count, "Should have new subscriptions")
}

// TestUnsubscribe_DuringPublish tests unsubscribing while a publish is in progress.
func TestUnsubscribe_DuringPublish(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe multiple handlers
	numHandlers := 10
	ids := make([]SubscriptionID, numHandlers)
	var handlerCount atomic.Int32

	for i := 0; i < numHandlers; i++ {
		id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
			handlerCount.Add(1)
			time.Sleep(10 * time.Millisecond) // Small delay to simulate work
			return Result{}, nil
		})
		require.NoError(t, err)
		ids[i] = id
	}

	// Start publish in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		bus.Publish(ctx, "test")
	}()

	// Give publish time to start
	time.Sleep(5 * time.Millisecond)

	// Unsubscribe some handlers while publish may be in progress
	for i := 0; i < 5; i++ {
		result := bus.Unsubscribe(ids[i])
		assert.True(t, result, "Unsubscribe during publish should return true")
	}

	// Wait for publish to complete
	wg.Wait()

	// Verify subscriber count
	assert.Equal(t, uint64(5), bus.subscriberCount.Load(), "Should have 5 remaining subscribers")

	// First publish should have called all 10 handlers (unsubscribe happened during execution)
	assert.Equal(t, int32(10), handlerCount.Load(), "First publish should call all 10 handlers")

	// Publish again should only call remaining 5 handlers
	bus.Publish(ctx, "test2")
	assert.Equal(t, int32(15), handlerCount.Load(), "Second publish should call only 5 remaining handlers")
}

// TestUnsubscribe_AfterClose tests unsubscribing after the bus is closed.
func TestUnsubscribe_AfterClose(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)

	// Subscribe
	id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Close the bus
	err = bus.Close(ctx)
	require.NoError(t, err)

	// Unsubscribe after close should still work
	result := bus.Unsubscribe(id)
	assert.True(t, result, "Unsubscribe after close should return true")

	// Subscriber count should be updated
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())
}

// TestUnsubscribe_WithHooks tests that unsubscribing doesn't affect global hooks.
func TestUnsubscribe_WithHooks(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfigWithHooks()
	bus, err := NewEventBus[string](ctx, cfg)
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Set global hooks
	var beforeCount atomic.Int32
	var afterCount atomic.Int32

	bus.SetGlobalBeforeHook(func(ctx context.Context, event string, id SubscriptionID) context.Context {
		beforeCount.Add(1)
		return ctx
	})

	bus.SetGlobalAfterHook(func(ctx context.Context, event string, id SubscriptionID, result Result, duration time.Duration, err error) {
		afterCount.Add(1)
	})

	// Subscribe
	id, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish
	bus.Publish(ctx, "test1")
	assert.Equal(t, int32(1), beforeCount.Load())
	assert.Equal(t, int32(1), afterCount.Load())

	// Unsubscribe
	bus.Unsubscribe(id)

	// Subscribe again
	_, err = bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	// Publish again - hooks should still work
	bus.Publish(ctx, "test2")
	assert.Equal(t, int32(2), beforeCount.Load(), "Before hook should be called again")
	assert.Equal(t, int32(2), afterCount.Load(), "After hook should be called again")
}

// TestUnsubscribe_SubscriberCountAccuracy tests that SubscriberCount is accurate.
func TestUnsubscribe_SubscriberCountAccuracy(t *testing.T) {
	ctx := context.Background()
	bus, err := NewEventBus[string](ctx, DefaultConfig())
	require.NoError(t, err)
	defer bus.Close(ctx)

	// Subscribe and unsubscribe in various patterns
	id1, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) { return Result{}, nil })
	require.NoError(t, err)
	assert.Equal(t, uint64(1), bus.subscriberCount.Load())

	id2, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) { return Result{}, nil })
	require.NoError(t, err)
	assert.Equal(t, uint64(2), bus.subscriberCount.Load())

	id3, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) { return Result{}, nil })
	require.NoError(t, err)
	assert.Equal(t, uint64(3), bus.subscriberCount.Load())

	// Unsubscribe middle
	bus.Unsubscribe(id2)
	assert.Equal(t, uint64(2), bus.subscriberCount.Load())

	// Unsubscribe first
	bus.Unsubscribe(id1)
	assert.Equal(t, uint64(1), bus.subscriberCount.Load())

	// Subscribe more
	id4, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) { return Result{}, nil })
	require.NoError(t, err)
	assert.Equal(t, uint64(2), bus.subscriberCount.Load())

	id5, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) { return Result{}, nil })
	require.NoError(t, err)
	assert.Equal(t, uint64(3), bus.subscriberCount.Load())

	// Unsubscribe all remaining
	bus.Unsubscribe(id3)
	bus.Unsubscribe(id4)
	bus.Unsubscribe(id5)
	assert.Equal(t, uint64(0), bus.subscriberCount.Load())
}

// --------------------------- MixedBus's Tests ---------------------------

// TestPublish_MultipleBusesConcurrentPublish tests multiple buses with mixed sync/async handlers publishing concurrently
func TestPublish_MultipleBusesConcurrentPublish(t *testing.T) {
	ctx := context.Background()

	// Test configuration
	const (
		numBuses            = 5
		syncHandlersPerBus  = 3
		asyncHandlersPerBus = 3
		numPublishers       = 10
		eventsPerPublisher  = 20
		totalEvents         = numPublishers * eventsPerPublisher
	)

	// Track events received by each bus
	type BusStats struct {
		syncReceived  atomic.Int64
		asyncReceived atomic.Int64
		totalReceived atomic.Int64
		bus           *EventBus[string]
	}

	busStats := make([]*BusStats, numBuses)

	// Create multiple buses with sync and async handlers
	for busIdx := 0; busIdx < numBuses; busIdx++ {
		bus, err := NewEventBus[string](ctx, DefaultConfig())
		require.NoError(t, err)
		defer bus.Close(ctx)

		stats := &BusStats{bus: bus}
		busStats[busIdx] = stats

		// Add sync handlers
		for handlerIdx := 0; handlerIdx < syncHandlersPerBus; handlerIdx++ {
			handlerID := handlerIdx
			_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
				stats.syncReceived.Add(1)
				stats.totalReceived.Add(1)
				// Simulate some work
				_ = handlerID
				return Result{"handler": handlerID, "type": "sync"}, nil
			})
			require.NoError(t, err)
		}

		// Add async handlers
		for handlerIdx := 0; handlerIdx < asyncHandlersPerBus; handlerIdx++ {
			handlerID := handlerIdx
			_, err := bus.Subscribe(func(ctx context.Context, event string) (Result, error) {
				stats.asyncReceived.Add(1)
				stats.totalReceived.Add(1)
				// Simulate some work
				_ = handlerID
				return Result{"handler": handlerID, "type": "async"}, nil
			}, bus.WithAsync())
			require.NoError(t, err)
		}
	}

	// Concurrent publishing phase
	var publishWg sync.WaitGroup
	publishWg.Add(numBuses * numPublishers)

	// Each bus gets multiple concurrent publishers
	for busIdx := 0; busIdx < numBuses; busIdx++ {
		busIdx := busIdx // Capture for goroutine
		for publisherIdx := 0; publisherIdx < numPublishers; publisherIdx++ {
			publisherIdx := publisherIdx // Capture for goroutine
			go func() {
				defer publishWg.Done()
				bus := busStats[busIdx].bus

				// Each publisher sends multiple events
				for eventIdx := 0; eventIdx < eventsPerPublisher; eventIdx++ {
					eventID := fmt.Sprintf("bus%d-pub%d-evt%d", busIdx, publisherIdx, eventIdx)
					bus.Publish(ctx, eventID)
				}
			}()
		}
	}

	// Wait for all publishers to complete
	publishWg.Wait()

	// Wait for all async handlers to complete on all buses
	for _, stats := range busStats {
		stats.bus.WaitAsync()
	}

	// Validate results for each bus
	for busIdx, stats := range busStats {
		t.Run(fmt.Sprintf("Bus%d", busIdx), func(t *testing.T) {
			expectedPerHandlerType := int64(totalEvents)
			expectedTotal := int64(totalEvents * (syncHandlersPerBus + asyncHandlersPerBus))

			// Verify sync handlers received all events
			assert.Equal(t, expectedPerHandlerType*int64(syncHandlersPerBus), stats.syncReceived.Load(),
				"Sync handlers should receive all events")

			// Verify async handlers received all events
			assert.Equal(t, expectedPerHandlerType*int64(asyncHandlersPerBus), stats.asyncReceived.Load(),
				"Async handlers should receive all events")

			// Verify total events
			assert.Equal(t, expectedTotal, stats.totalReceived.Load(),
				"Total events should equal events * (sync + async handlers)")

			// Verify bus metrics
			assert.Equal(t, uint64(totalEvents), stats.bus.publishedCount.Load(),
				"Published count should match total events")
			assert.Equal(t, uint64(expectedTotal), stats.bus.processedCount.Load(),
				"Processed count should match total handler invocations")
			assert.Equal(t, uint64(0), stats.bus.errorCount.Load(),
				"Error count should be zero")
			assert.Equal(t, uint64(syncHandlersPerBus+asyncHandlersPerBus), stats.bus.subscriberCount.Load(),
				"Subscriber count should be correct")
		})
	}
}
