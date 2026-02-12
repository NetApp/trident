// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"

	"github.com/netapp/trident/pkg/workerpool/types"
)

// Compile-time checks to ensure types implement required interfaces.
var (
	_ types.Pool   = (*Pool)(nil)
	_ types.Config = (*Config)(nil)
)

// Pool wraps an ants.Pool.
type Pool struct {
	pool    *ants.Pool
	started atomic.Bool
	closed  atomic.Bool
}

// NewPool creates a new Pool using the ants library.
func NewPool(ctx context.Context, cfg *Config) (*Pool, error) {
	opts := []ants.Option{
		ants.WithPreAlloc(cfg.PreAlloc),
		ants.WithNonblocking(cfg.NonBlocking),
		ants.WithDisablePurge(cfg.DisablePurge),
	}

	// Only set expiry duration if explicitly configured
	if cfg.ExpiryDuration > 0 {
		opts = append(opts, ants.WithExpiryDuration(cfg.ExpiryDuration))
	}

	pool, err := ants.NewPool(cfg.NumWorkers, opts...)
	if err != nil {
		return nil, err
	}

	return &Pool{pool: pool}, nil
}

// Start initializes and starts the worker pool.
// For ants pool, this marks the pool as started since it's ready after New().
func (p *Pool) Start(ctx context.Context) error {
	p.started.Store(true)
	return nil
}

// IsStarted returns true if the pool has been started.
func (p *Pool) IsStarted() bool {
	return p.started.Load()
}

// Submit submits a task to the pool for execution.
func (p *Pool) Submit(ctx context.Context, task func()) error {
	if p.closed.Load() {
		return ants.ErrPoolClosed
	}

	return p.pool.Submit(task)
}

// Shutdown gracefully shuts down the pool without a timeout.
// It stops accepting new tasks and waits indefinitely for running tasks to complete.
// Use ShutdownWithTimeout if you need time-bounded shutdown behavior.
func (p *Pool) Shutdown(ctx context.Context) error {
	if p.closed.Swap(true) {
		return nil
	}

	p.pool.Release()
	return nil
}

// ShutdownWithTimeout gracefully shuts down the pool with a timeout.
// It stops accepting new tasks and waits for running tasks to complete.
// Returns an error if the timeout expires before shutdown completes.
func (p *Pool) ShutdownWithTimeout(timeout time.Duration) error {
	if p.closed.Swap(true) {
		return nil
	}

	return p.pool.ReleaseTimeout(timeout)
}

// Running returns the number of currently executing workers.
func (p *Pool) Running() int {
	return p.pool.Running()
}

// Cap returns the capacity of the pool.
func (p *Pool) Cap() int {
	return p.pool.Cap()
}

// Free returns the number of available (idle) workers.
func (p *Pool) Free() int {
	return p.pool.Free()
}

// IsClosed returns true if the pool has been shut down.
func (p *Pool) IsClosed() bool {
	return p.closed.Load()
}

// Waiting returns the number of tasks waiting in the queue.
func (p *Pool) Waiting() int {
	return p.pool.Waiting()
}

// Stats returns all pool statistics at once.
func (p *Pool) Stats() types.PoolStats {
	return types.PoolStats{
		Capacity: p.pool.Cap(),
		Running:  p.pool.Running(),
		Free:     p.pool.Free(),
		Waiting:  p.pool.Waiting(),
	}
}
