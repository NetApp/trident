// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package assorter provides interfaces and utilities for managing volume scheduling.
package assorter

import (
	"context"
	"fmt"

	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter/periodic"
	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types"
	"github.com/netapp/trident/utils/errors"
)

// New creates a new Assorter using the provided implementation-specific config.
// The generic type parameter T specifies which interface the caller needs:
//   - Assorter: basic assorter operations
//
// If no config is provided, a default config is chosen based on the requested interface T.
// Currently, periodic is the default implementation.
// If the requested assorter type doesn't support the required interface, an error is returned.
//
// Example usage:
//
//	// Default periodic assorter
//	assorter, err := assorter.New[types.Assorter](ctx)
//
//	// Periodic assorter with custom config using options pattern
//	config := periodic.NewConfig(
//	    periodic.WithPeriod(2 * time.Minute),
//	    periodic.WithPublishFunc(myPublishFunc),
//	)
//	assorter, err := assorter.New[types.Assorter](ctx, config)
func New[T types.Assorter](ctx context.Context, cfgs ...types.Config) (T, error) {
	var zero T

	// Determine config to use
	var cfg types.Config
	if len(cfgs) == 0 || cfgs[0] == nil {
		// Choose default config based on requested interface T
		cfg = defaultConfigFor[T]()
	} else {
		cfg = cfgs[0]
	}

	var assorter any
	var err error

	switch c := cfg.(type) {
	case *periodic.Config:
		assorter, err = periodic.NewAssorter(ctx, c)
	default:
		return zero, errors.UnsupportedConfigError(fmt.Sprintf("unsupported config type %T", cfg))
	}

	if err != nil {
		return zero, err
	}

	// Type assertion to check if assorter implements the requested interface
	result, ok := assorter.(T)
	if !ok {
		return zero, errors.InterfaceNotSupportedError(fmt.Sprintf("%T", cfg), fmt.Sprintf("%T", zero))
	}

	return result, nil
}

// defaultConfigFor returns the default config for the requested assorter interface.
// This ensures the default implementation supports the requested interface.
func defaultConfigFor[T types.Assorter]() types.Config {
	// For basic Assorter interface, periodic assorter is the default
	return periodic.DefaultConfig()
}

// MustNew is like New but panics if assorter creation fails.
func MustNew[T types.Assorter](ctx context.Context, cfgs ...types.Config) T {
	assorter, err := New[T](ctx, cfgs...)
	if err != nil {
		panic("assorter: " + err.Error())
	}
	return assorter
}
