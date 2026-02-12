// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package types provides shared types for the workerpool package.
package types

//go:generate mockgen -destination=../../../mocks/mock_pkg/mock_workerpool/mock_workerpool.go -package=mock_workerpool github.com/netapp/trident/pkg/workerpool/types Pool,MultiPool,Config

import (
	"context"
	"time"
)

// PoolStats contains runtime statistics for a worker pool.
type PoolStats struct {
	Capacity int // Maximum number of workers
	Running  int // Number of currently executing workers
	Free     int // Number of available (idle) workers
	Waiting  int // Number of tasks waiting in the queue
}

// ============================================================================
// Pool Interface
// ============================================================================

// Pool defines the base interface for a worker pool.
// Implementations must be safe for concurrent use.
type Pool interface {
	// Start initializes and starts the worker pool.
	// Must be called before submitting tasks.
	Start(ctx context.Context) error

	// Submit submits a task to the pool for execution.
	// Returns an error if the pool is closed or the task cannot be submitted.
	Submit(ctx context.Context, task func()) error

	// Shutdown gracefully shuts down the pool.
	// It stops accepting new tasks and waits indefinitely for running tasks to complete.
	// For time-bounded shutdown, use ShutdownWithTimeout instead.
	Shutdown(ctx context.Context) error

	// ShutdownWithTimeout gracefully shuts down the pool with a timeout.
	// It stops accepting new tasks and waits for running tasks to complete.
	// Returns an error if the timeout expires before shutdown completes.
	ShutdownWithTimeout(timeout time.Duration) error

	// IsStarted returns true if the pool has been started.
	IsStarted() bool

	// IsClosed returns true if the pool has been shut down.
	IsClosed() bool

	// Running returns the number of currently executing workers.
	Running() int

	// Cap returns the capacity (maximum number of workers) of the pool.
	Cap() int

	// Free returns the number of available (idle) workers.
	// Returns -1 if the pool is closed or doesn't support this metric.
	Free() int

	// Waiting returns the number of tasks waiting in the queue.
	Waiting() int

	// Stats returns all pool statistics at once.
	Stats() PoolStats
}

// ============================================================================
// MultiPool Interface
// ============================================================================

// MultiPool defines the base interface for a  multipool.
// Implementations must be safe for concurrent use.
// This is for multi-pool implementations that consist of multiple underlying pools
// with load balancing.
type MultiPool interface {
	Pool

	// RunningByIndex returns the number of currently executing workers in a specific pool.
	// Returns an error if the index is out of bounds.
	RunningByIndex(idx int) (int, error)

	// FreeByIndex returns the number of available workers in a specific pool.
	// Returns an error if the index is out of bounds.
	FreeByIndex(idx int) (int, error)

	// WaitingByIndex returns the number of tasks waiting in a specific pool.
	// Returns an error if the index is out of bounds.
	WaitingByIndex(idx int) (int, error)
}

// ============================================================================
// Pool Config Interface
// ============================================================================

// Config is an interface that all pool configurations must implement.
// Each pool implementation defines its own config type.
type Config interface {
	// PoolConfig is a marker method to ensure only valid configs are used.
	PoolConfig()
	// Copy returns a deep copy of this configuration.
	Copy() Config
}
