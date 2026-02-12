// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package eventbus provides a factory function for creating EventBus instances
// The actual implementations live in subpackages like mutexbus/simple.
package eventbus

import (
	"context"
	"fmt"

	"github.com/netapp/trident/pkg/eventbus/mutexbus/simple"
	"github.com/netapp/trident/pkg/eventbus/types"
	"github.com/netapp/trident/utils/errors"
)

var (
	// defaultConfigEventBusHooksMetricsOptions is the default config for types.EventBusHooksMetricsOptions[T] interface.
	// Complete feature set: hooks enabled, metrics tracked, options supported.
	defaultConfigEventBusHooksMetricsOptions = simple.DefaultConfigWithHooks()

	// defaultConfigEventBusHooksMetrics is the default config for types.EventBusHooksMetrics[T] interface.
	// Hooks enabled, metrics tracked
	defaultConfigEventBusHooksMetrics = simple.DefaultConfigWithHooks()

	// defaultConfigEventBusHooksOptions is the default config for types.EventBusHooksOptions[T] interface.
	// Hooks enabled, options supported
	defaultConfigEventBusHooksOptions = simple.DefaultConfigWithHooks()

	// defaultConfigEventBusMetricsOptions is the default config for types.EventBusMetricsOptions[T] interface.
	// Options supported, metrics always tracked
	defaultConfigEventBusMetricsOptions = simple.DefaultConfig()

	// defaultConfigEventBusWithHooks is the default config for types.EventBusWithHooks[T] interface.
	// Hooks enabled for observability
	defaultConfigEventBusWithHooks = simple.DefaultConfigWithHooks()

	// defaultConfigEventBusWithMetrics is the default config for types.EventBusWithMetrics[T] interface.
	// Metrics are tracked
	defaultConfigEventBusWithMetrics = simple.DefaultConfig()

	// defaultConfigEventBusWithOptions is the default config for types.EventBusWithOptions[T] interface.
	// Options are supported
	defaultConfigEventBusWithOptions = simple.DefaultConfig()

	// defaultConfigEventBus is the default config for basic types.EventBus[T] interface.
	defaultConfigEventBus = simple.DefaultConfig()
)

// defaultConfigFor returns the default config for the requested EventBus interface.
// This ensures the default implementation matches the requested interface capabilities.
// Each interface type has its own default config variable for clarity and potential
// future customization per interface type.
func defaultConfigFor[T any, K types.EventBus[T]]() (types.Config, error) {
	var zero K

	// Check what interface K is and return the appropriate default config copy
	// Order matters: check most specific interfaces first, then fall back to less specific
	switch any(zero).(type) {
	case types.EventBusHooksMetricsOptions[T]:
		// User requested complete interface with all features (hooks + metrics + options)
		return defaultConfigEventBusHooksMetricsOptions.Copy(), nil
	case types.EventBusHooksMetrics[T]:
		// User requested hooks + metrics (no options)
		return defaultConfigEventBusHooksMetrics.Copy(), nil
	case types.EventBusHooksOptions[T]:
		// User requested hooks + options (without metrics)
		return defaultConfigEventBusHooksOptions.Copy(), nil
	case types.EventBusMetricsOptions[T]:
		// User requested metrics + options (no hooks)
		return defaultConfigEventBusMetricsOptions.Copy(), nil
	case types.EventBusWithHooks[T]:
		// User requested hooks interface
		return defaultConfigEventBusWithHooks.Copy(), nil
	case types.EventBusWithMetrics[T]:
		// User requested metrics interface
		return defaultConfigEventBusWithMetrics.Copy(), nil
	case types.EventBusWithOptions[T]:
		// User requested options interface
		return defaultConfigEventBusWithOptions.Copy(), nil
	case types.EventBus[T]:
		// User requested basic EventBus interface
		return defaultConfigEventBus.Copy(), nil
	default:
		return nil, errors.InterfaceNotSupportedError(
			fmt.Sprintf("%T", zero),
			"types.EventBus[T], types.EventBusWithOptions[T], types.EventBusWithHooks[T], "+
				"types.EventBusWithMetrics[T], types.EventBusHooksOptions[T], types.EventBusMetricsOptions[T], "+
				"types.EventBusHooksMetrics[T], or types.EventBusHooksMetricsOptions[T]",
		)
	}
}

// New creates a new EventBus using the provided implementation-specific config.
// The generic type parameter T specifies the event type that the bus will handle.
// The generic type parameter K constrains the return type to types that implement EventBus[T].
//
// If no config is provided, a shared default config is used (defaultConfig).
// Currently, only the simple (mutex-based) implementation is supported.
//
// The config type determines which implementation is used:
//   - *simple.Config: mutex-based EventBus with configurable worker pool
//   - future: other implementations (lock-free, channel-based, etc.)
//
// Example usage:
//
//	// Default EventBus (returns interface)
//	bus, err := eventbus.New[User, types.EventBus[User]](ctx)
//
//	// Explicit concrete type (returns *simple.EventBus[User])
//	bus, err := eventbus.New[User, *simple.EventBus[User]](ctx, simpleConfig)
//
//	// EventBus with custom config
//	cfg := simple.NewConfig(
//	    simple.WithNoHooks(),
//	    simple.WithDefaultTimeout(5*time.Second),
//	)
//	bus, err := eventbus.New[User, types.EventBus[User]](ctx, cfg)
//
//	// EventBus with custom worker pool
//	pool, _ := workerpool.New[workerpool.Pool](ctx)
//	cfg := simple.NewConfig(simple.WithWorkerPoolConfig(pool))
//	bus, err := eventbus.New[User, types.EventBus[User]](ctx, cfg)
func New[T any, K types.EventBus[T]](ctx context.Context, cfgs ...types.Config) (K, error) {
	var zero K

	// Determine config to use
	var cfg types.Config
	var err error
	if len(cfgs) == 0 || cfgs[0] == nil {
		// Choose default config based on requested interface K
		cfg, err = defaultConfigFor[T, K]()
		if err != nil {
			return zero, err
		}
	} else {
		cfg = cfgs[0]
	}

	// Create EventBus based on config type
	var bus any

	switch c := cfg.(type) {
	case *simple.Config:
		bus, err = simple.NewEventBus[T](ctx, c)
	default:
		return zero, errors.UnsupportedConfigError(fmt.Sprintf("unsupported config type %T", cfg))
	}

	if err != nil {
		return zero, err
	}

	// Validate that the created bus can be assigned to K
	// This provides runtime type safety while K provides compile-time constraints
	result, ok := bus.(K)
	if !ok {
		return zero, errors.InterfaceNotSupportedError(fmt.Sprintf("%T", cfg), fmt.Sprintf("%T", zero))
	}
	return result, nil
}
