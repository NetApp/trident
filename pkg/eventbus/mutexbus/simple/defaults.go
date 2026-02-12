// Copyright 2025 NetApp, Inc. All Rights Reserved.

package simple

import (
	"runtime"
	"time"

	"github.com/netapp/trident/pkg/workerpool/ants"
)

const (
	defaultPreAllocHandlers = 16
	defaultNoHooks          = true
	defaultShutdownTimeout  = 30 * time.Second
)

var (
	// defaultNumWorkers is the default number of workers for async subscriptions (based on CPU count).
	defaultNumWorkers = runtime.NumCPU()
	// defaultWorkerPoolConfig is the default worker pool config used if no custom pool is provided.
	defaultWorkerPoolConfig = ants.DefaultConfig()
)

// DefaultConfig returns the default configuration for a simple EventBus using the functional options pattern.
// It uses the number of CPUs for worker pool size, enables pre-allocation of handlers,
// and disables hooks by default for maximum performance.
//
// Example:
//
//	cfg := simple.DefaultConfig()
//	bus, _ := simple.NewEventBus[User](ctx, cfg)
func DefaultConfig() *Config {
	return NewConfig()
}

// DefaultConfigWithHooks returns the default configuration for a simple EventBus with hooks enabled,
// using the functional options pattern.
//
// It uses the number of CPUs for worker pool size, enables pre-allocation of handlers,
// and enables hooks for observability (before, after, error hooks).
//
// Use this when you need observability, metrics, or error handling hooks at the bus or handler level.
//
// Example:
//
//	cfg := simple.DefaultConfigWithHooks()
//	bus, _ := simple.NewEventBus[User](ctx, cfg)
//	bus.SetGlobalAfterHook(metricsHook)
//	bus.Subscribe(handler, bus.WithOnError(errorHook))
func DefaultConfigWithHooks() *Config {
	return NewConfig(WithHooks())
}
