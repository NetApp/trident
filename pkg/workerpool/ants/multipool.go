// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"

	"github.com/netapp/trident/pkg/workerpool/types"
)

const (
	indefiniteTime = 365 * 24 * time.Hour
)

// Compile-time checks to ensure types implement required interfaces.
var (
	_ types.Pool   = (*MultiPool)(nil)
	_ types.Config = (*MultiPoolConfig)(nil)
)

// MultiPool wraps an ants.MultiPool.
type MultiPool struct {
	multiPool *ants.MultiPool
	started   atomic.Bool
	closed    atomic.Bool
}

// NewMultiPool creates a new MultiPool using the ants library.
func NewMultiPool(ctx context.Context, cfg *MultiPoolConfig) (*MultiPool, error) {
	opts := []ants.Option{
		ants.WithPreAlloc(cfg.PreAlloc),
		ants.WithNonblocking(cfg.NonBlocking),
		ants.WithDisablePurge(cfg.DisablePurge),
	}

	// Only set expiry duration if explicitly configured
	if cfg.ExpiryDuration > 0 {
		opts = append(opts, ants.WithExpiryDuration(cfg.ExpiryDuration))
	}

	multipool, err := ants.NewMultiPool(
		cfg.NumPools,
		cfg.NumWorkersPerPool,
		cfg.LoadBalancingStrategy,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return &MultiPool{multiPool: multipool}, nil
}

// Start initializes and starts the multiPool.
// For ants multiPool, this marks the multiPool as started since it's ready after New().
func (mp *MultiPool) Start(ctx context.Context) error {
	mp.started.Store(true)
	return nil
}

// IsStarted returns true if the multiPool has been started.
func (mp *MultiPool) IsStarted() bool {
	return mp.started.Load()
}

// Submit submits a task to one of the pools using the load-balancing strategy.
func (mp *MultiPool) Submit(ctx context.Context, task func()) error {
	if mp.closed.Load() {
		return ants.ErrPoolClosed
	}

	return mp.multiPool.Submit(task)
}

// Shutdown gracefully shuts down the multiPool without a timeout.
// It stops accepting new tasks and waits indefinitely for running tasks to complete.
// Use ShutdownWithTimeout if you need time-bounded shutdown behavior.
// Note: ants.MultiPool only supports timeout-based release, so this uses a very large timeout.
func (mp *MultiPool) Shutdown(ctx context.Context) error {
	if mp.closed.Swap(true) {
		return nil
	}

	// Use a very large timeout to simulate indefinite wait
	// This is a limitation of ants.MultiPool which only supports ReleaseTimeout
	err := mp.multiPool.ReleaseTimeout(indefiniteTime)
	if err != nil {
	} else {
	}
	return err
}

// ShutdownWithTimeout gracefully shuts down the multiPool with a timeout.
// It stops accepting new tasks and waits for running tasks to complete.
// Returns an error if the timeout expires before shutdown completes.
func (mp *MultiPool) ShutdownWithTimeout(timeout time.Duration) error {
	if mp.closed.Swap(true) {
		return nil
	}

	return mp.multiPool.ReleaseTimeout(timeout)
}

// Running returns the number of currently executing workers across all pools.
func (mp *MultiPool) Running() int {
	return mp.multiPool.Running()
}

// Cap returns the total capacity of the multiPool.
func (mp *MultiPool) Cap() int {
	return mp.multiPool.Cap()
}

// Free returns the number of available (idle) workers across all pools.
func (mp *MultiPool) Free() int {
	return mp.multiPool.Free()
}

// IsClosed returns true if the multiPool has been shut down.
func (mp *MultiPool) IsClosed() bool {
	return mp.closed.Load()
}

// Waiting returns the number of tasks waiting in the queue across all pools.
func (mp *MultiPool) Waiting() int {
	return mp.multiPool.Waiting()
}

// Stats returns all multiPool statistics at once.
func (mp *MultiPool) Stats() types.PoolStats {
	return types.PoolStats{
		Capacity: mp.multiPool.Cap(),
		Running:  mp.multiPool.Running(),
		Free:     mp.multiPool.Free(),
		Waiting:  mp.multiPool.Waiting(),
	}
}

// RunningByIndex returns the number of currently executing workers in a specific multiPool.
func (mp *MultiPool) RunningByIndex(idx int) (int, error) {
	return mp.multiPool.RunningByIndex(idx)
}

// FreeByIndex returns the number of available workers in a specific multiPool.
func (mp *MultiPool) FreeByIndex(idx int) (int, error) {
	return mp.multiPool.FreeByIndex(idx)
}

// WaitingByIndex returns the number of tasks waiting in a specific multiPool.
func (mp *MultiPool) WaitingByIndex(idx int) (int, error) {
	return mp.multiPool.WaitingByIndex(idx)
}
