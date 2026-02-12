// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package workerpool provides interfaces and utilities for managing goroutine pools.
package workerpool

import (
	"context"
	"fmt"

	"github.com/netapp/trident/pkg/workerpool/ants"
	"github.com/netapp/trident/pkg/workerpool/types"
	"github.com/netapp/trident/utils/errors"
)

// New creates a new Pool using the provided implementation-specific config file.
// The generic type parameter T specifies which interface the caller needs:
//   - Pool: basic pool operations only
//   - PoolWithStats: pool operations plus statistics (Running, Cap, Waiting, Stats)
//   - MultiPoolWithStats: multi-pool operations plus per-pool statistics
//
// If no config is provided, a default config is chosen based on the requested interface T.
// Currently, ants is the default implementation which supports all pool types.
// If the requested pool type doesn't support the required interface, an error is returned.
//
// Example usage:
//
//		// Default single pool
//		pool, err := workerpool.New[workerpool.Pool](ctx)
//
//	 // Default single poolWithStats
//	 pool, err := workerpool.New[workerpool.PoolWithStats](ctx)
//
//	 // Default multipool
//	 pool, err := workerpool.New[workerpool.MultiPool](ctx)
//
//	 // Default multipool multipoolWithStats
//	 pool, err := workerpool.New[workerpool.MultipoolWithStats](ctx)
//
//		// Basic pool with ants config
//		pool, err := workerpool.New[workerpool.Pool](ctx, ants.Config{NumWorkers: 10})
//
//		// Pool with stats
//		poolWithStats, err := workerpool.New[workerpool.PoolWithStats](ctx, ants.Config{
//		    NumWorkers:  10,
//		    PreAlloc:    true,
//		    NonBlocking: false,
//		})
//
//		// Multi-pool with stats
//		multiPool, err := workerpool.New[workerpool.MultiPoolWithStats](ctx, ants.MultiPoolConfig{
//		    NumPools:              4,
//		    NumWorkersPerPool:     10,
//		    LoadBalancingStrategy: ants.RoundRobin,
//		})
func New[T types.Pool](ctx context.Context, cfgs ...types.Config) (T, error) {
	var zero T

	// Determine config to use
	var cfg types.Config
	if len(cfgs) == 0 || cfgs[0] == nil {
		// Choose default config based on requested interface T
		cfg = defaultConfigFor[T]()
	} else {
		cfg = cfgs[0]
	}

	var pool any
	var err error

	switch c := cfg.(type) {
	case *ants.Config:
		pool, err = ants.NewPool(ctx, c)
	case *ants.MultiPoolConfig:
		pool, err = ants.NewMultiPool(ctx, c)
	default:
		return zero, errors.UnsupportedConfigError(fmt.Sprintf("unsupported config type %T", cfg))
	}

	if err != nil {
		return zero, err
	}

	// Type assertion to check if pool implements the requested interface
	result, ok := pool.(T)
	if !ok {
		return zero, errors.InterfaceNotSupportedError(fmt.Sprintf("%T", cfg), fmt.Sprintf("%T", zero))
	}

	return result, nil
}

// defaultConfigFor returns the default config for the requested pool interface.
// This ensures the default implementation supports the requested interface.
func defaultConfigFor[T types.Pool]() types.Config {
	var zero T

	// Check what interface T is by type switching on the zero value
	switch any(zero).(type) {
	case types.MultiPool:
		// ants.MultiPool implements MultiPool
		return ants.DefaultMultiPoolConfig()
	default:
		// For basic Pool interface, ants single pool also works
		return ants.DefaultConfig()
	}
}

// MustNew is like New but panics if pool creation fails.
func MustNew[T types.Pool](ctx context.Context, cfgs ...types.Config) T {
	pool, err := New[T](ctx, cfgs...)
	if err != nil {
		panic("workerpool: " + err.Error())
	}
	return pool
}
