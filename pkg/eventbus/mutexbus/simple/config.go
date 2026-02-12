// Copyright 2025 NetApp, Inc. All Rights Reserved.

package simple

import (
	"time"

	"github.com/brunoga/deep"

	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/eventbus/types"
	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
)

// =============================================================================
// EventBus Configuration
// =============================================================================

// Config holds configuration options for creating a mutex-based simple EventBus.
// This configuration determines the behavior and performance characteristics
// of the EventBus instance.
type Config struct {
	// WorkerPool is an optional external worker pool for async subscriptions.
	// If nil, an internal worker pool will be created with runtime.NumCPU() workers.
	// If provided, this pool is used directly.
	// The EventBus will NOT shut down an external pool on Close().
	WorkerPool workerpooltypes.Pool

	// NoHooks disables ALL hooks for this EventBus, providing maximum performance.
	// This skips:
	//   - All before/after/error hooks (both bus-level and handler-level)
	//   - Hook copying during Publish
	//   - Duration measurement
	//   - Result collection
	// Use this for high-throughput buses where observability is not needed.
	NoHooks *bool

	// DefaultTimeout specifies the default timeout for all handlers on this bus.
	// This timeout is used when a handler is subscribed without a specific timeout.
	// If a handler has its own timeout (via WithTimeout), that takes precedence.
	// If 0, handlers may run indefinitely by default (unless they specify their own timeout).
	DefaultTimeout time.Duration

	// PreAllocHandlers pre-allocates handler slices with this capacity.
	// This can reduce allocations during the first few Subscribe calls.
	// Default is 16 if not specified.
	PreAllocHandlers int

	// ShutdownTimeout specifies the maximum time to wait for:
	//   1. Pending async handlers to complete (bus.wg.Wait())
	//   2. Worker pool shutdown (if owned by this bus)
	// If 0, uses a default timeout of 30 seconds.
	// Set to a negative value to wait indefinitely.
	ShutdownTimeout time.Duration
}

// EventBusConfig is a marker method to implement the types.Config interface.
func (*Config) EventBusConfig() {}

// GetWorkerPool returns the worker pool configured for this EventBus.
// Returns nil if no external pool is configured.
func (c *Config) GetWorkerPool() workerpooltypes.Pool {
	return c.WorkerPool
}

// SetWorkerPool sets the worker pool for this EventBus configuration.
func (c *Config) SetWorkerPool(pool workerpooltypes.Pool) {
	c.WorkerPool = pool
}

// Copy returns a deep copy of this configuration.
// This allows config templates to be safely reused across multiple buses.
func (c *Config) Copy() types.Config {
	if c == nil {
		return nil
	}

	// Use deep copy to handle nested structures
	copied, err := deep.Copy(c)
	if err != nil {
		// Fallback to simple struct copy if deep copy fails
		cfgCopy := *c
		return &cfgCopy
	}
	return copied
}

// =============================================================================
// Config Options
// =============================================================================

// ConfigOption is a functional option for configuring a Config.
type ConfigOption func(*Config)

// NewConfig creates a new Config with the provided options.
// Example:
//
//	cfg := simple.NewConfig(
//	    simple.WithNoHooks(),
//	    simple.WithPreAllocHandlers(32),
//	)
//	bus, _ := eventbus.New[User](ctx, cfg)
func NewConfig(opts ...ConfigOption) *Config {
	// Start with empty config
	cfg := &Config{
		PreAllocHandlers: defaultPreAllocHandlers,
		NoHooks:          convert.ToPtr(defaultNoHooks),
		ShutdownTimeout:  defaultShutdownTimeout,
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithWorkerPoolConfig sets an external worker pool for async subscriptions.
// When an external pool is provided, the EventBus will use it directly instead
// of creating an internal pool with default settings.
// The EventBus will NOT shut down an external pool on Close().
//
// Example:
//
//	pool, _ := workerpool.New[workerpool.Pool](ctx)
//	cfg := simple.NewConfig(simple.WithWorkerPoolConfig(pool))
func WithWorkerPoolConfig(pool workerpooltypes.Pool) ConfigOption {
	return func(c *Config) {
		c.WorkerPool = pool
	}
}

// WithNoHooks disables ALL hooks for maximum performance.
//
// Example:
//
//	cfg := simple.NewConfig(simple.WithNoHooks())
func WithNoHooks() ConfigOption {
	return func(c *Config) {
		c.NoHooks = convert.ToPtr(true)
	}
}

// WithHooks enables hooks (opposite of WithNoHooks).
// Hooks are disabled by default, so this is needed if you want to enable hooks.
//
// Example:
//
//	cfg := simple.NewConfig(simple.WithHooks())
func WithHooks() ConfigOption {
	return func(c *Config) {
		c.NoHooks = convert.ToPtr(false)
	}
}

// WithDefaultTimeout sets the default timeout for all handlers on this bus.
// This timeout applies to handlers that don't specify their own timeout via WithTimeout().
// Handler-specific timeouts always take precedence over the bus default.
//
// Example:
//
//	cfg := simple.NewConfig(simple.WithDefaultTimeout(5*time.Second))
//	bus, _ := eventbus.New[User](ctx, cfg)
//	// All handlers without explicit timeout will use 5 seconds
func WithDefaultTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.DefaultTimeout = timeout
	}
}

// WithPreAllocHandlers sets the pre-allocation capacity for handler slices.
//
// Example:
//
//	cfg := simple.NewConfig(simple.WithPreAllocHandlers(32))
func WithPreAllocHandlers(capacity int) ConfigOption {
	return func(c *Config) {
		c.PreAllocHandlers = capacity
	}
}

// WithShutdownTimeout sets the maximum time to wait during Close().
// This applies to both:
//  1. Waiting for pending async handlers to complete (bus.wg.Wait())
//  2. Shutting down the worker pool (if owned by this bus)
//
// If 0 (default), uses defaultShutdownTimeout (30 seconds).
// Set to a negative value (e.g., -1) to wait indefinitely.
//
// Example:
//
//	cfg := simple.NewConfig(simple.WithShutdownTimeout(30*time.Second))
//	bus, _ := eventbus.New[User](ctx, cfg)
//	bus.Close(ctx)  // Waits max 30 seconds
func WithShutdownTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.ShutdownTimeout = timeout
	}
}
