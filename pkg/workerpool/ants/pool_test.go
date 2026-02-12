// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Pool Creation Tests (NewPool)
// ============================================================================

func TestNewPool(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		config      *Config
		wantErr     bool
		errContains string
		validate    func(t *testing.T, p *Pool)
	}{
		{
			name: "valid config with default values",
			config: &Config{
				NumWorkers: 10,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Pool) {
				assert.NotNil(t, p.pool)
				assert.Equal(t, 10, p.Cap())
				assert.False(t, p.IsStarted())
				assert.False(t, p.IsClosed())
			},
		},
		{
			name: "valid config with PreAlloc enabled",
			config: &Config{
				NumWorkers: 5,
				PreAlloc:   true,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Pool) {
				assert.NotNil(t, p.pool)
				assert.Equal(t, 5, p.Cap())
			},
		},
		{
			name: "valid config with NonBlocking enabled",
			config: &Config{
				NumWorkers:  5,
				NonBlocking: true,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Pool) {
				assert.NotNil(t, p.pool)
				assert.Equal(t, 5, p.Cap())
			},
		},
		{
			name: "valid config with all options",
			config: &Config{
				NumWorkers:  8,
				PreAlloc:    true,
				NonBlocking: true,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Pool) {
				assert.NotNil(t, p.pool)
				assert.Equal(t, 8, p.Cap())
			},
		},
		{
			name: "single worker pool",
			config: &Config{
				NumWorkers: 1,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Pool) {
				assert.Equal(t, 1, p.Cap())
			},
		},
		{
			name: "large worker pool",
			config: &Config{
				NumWorkers: 1000,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Pool) {
				assert.Equal(t, 1000, p.Cap())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPool(ctx, tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, pool)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, pool)
			defer pool.Shutdown(ctx)

			if tt.validate != nil {
				tt.validate(t, pool)
			}
		})
	}
}

// ============================================================================
// Pool Start Tests
// ============================================================================

func TestPool_Start(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		setup    func(t *testing.T) *Pool
		validate func(t *testing.T, p *Pool, err error)
	}{
		{
			name: "start sets IsStarted to true",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				return p
			},
			validate: func(t *testing.T, p *Pool, err error) {
				assert.NoError(t, err)
				assert.True(t, p.IsStarted())
			},
		},
		{
			name: "start can be called multiple times",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx) // First call
				return p
			},
			validate: func(t *testing.T, p *Pool, err error) {
				assert.NoError(t, err)
				assert.True(t, p.IsStarted())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.setup(t)
			defer pool.Shutdown(ctx)

			err := pool.Start(ctx)
			tt.validate(t, pool, err)
		})
	}
}

// ============================================================================
// Pool Submit Tests
// ============================================================================

func TestPool_Submit(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		config      *Config
		setupPool   func(t *testing.T, p *Pool)
		task        func(executed *atomic.Bool)
		wantErr     bool
		wantErrType error
		checkExec   bool // whether to verify task execution
	}{
		{
			name:   "submit task executes successfully",
			config: &Config{NumWorkers: 5},
			setupPool: func(t *testing.T, p *Pool) {
				_ = p.Start(ctx)
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:   false,
			checkExec: true,
		},
		{
			name:   "submit to closed pool returns error",
			config: &Config{NumWorkers: 5},
			setupPool: func(t *testing.T, p *Pool) {
				_ = p.Start(ctx)
				_ = p.Shutdown(ctx)
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:     true,
			wantErrType: ants.ErrPoolClosed,
			checkExec:   false,
		},
		{
			name:   "submit to non-blocking pool when busy returns error",
			config: &Config{NumWorkers: 1, NonBlocking: true},
			setupPool: func(t *testing.T, p *Pool) {
				_ = p.Start(ctx)
				// Submit a blocking task to make pool busy
				_ = p.Submit(ctx, func() {
					time.Sleep(100 * time.Millisecond)
				})
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:   true, // Pool is busy
			checkExec: false,
		},
		{
			name:   "submit multiple tasks",
			config: &Config{NumWorkers: 5},
			setupPool: func(t *testing.T, p *Pool) {
				_ = p.Start(ctx)
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:   false,
			checkExec: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPool(ctx, tt.config)
			assert.NoError(t, err)
			defer pool.Shutdown(ctx)

			if tt.setupPool != nil {
				tt.setupPool(t, pool)
			}

			var executed atomic.Bool
			var taskToSubmit func()
			if tt.task != nil {
				taskToSubmit = func() {
					tt.task(&executed)
				}
			}

			err = pool.Submit(ctx, taskToSubmit)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrType != nil {
					assert.ErrorIs(t, err, tt.wantErrType)
				}
				return
			}

			assert.NoError(t, err)

			if tt.checkExec {
				// Wait for task to execute
				time.Sleep(50 * time.Millisecond)
				assert.True(t, executed.Load(), "task should have been executed")
			}
		})
	}
}

func TestPool_Submit_Concurrent(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, &Config{NumWorkers: 10})
	assert.NoError(t, err)
	defer pool.Shutdown(ctx)

	_ = pool.Start(ctx)

	const numTasks = 100
	var counter atomic.Int32
	var wg sync.WaitGroup

	wg.Add(numTasks)
	for i := 0; i < numTasks; i++ {
		err := pool.Submit(ctx, func() {
			defer wg.Done()
			counter.Add(1)
		})
		assert.NoError(t, err)
	}

	wg.Wait()
	assert.Equal(t, int32(numTasks), counter.Load())
}

// ============================================================================
// Pool Shutdown Tests
// ============================================================================

func TestPool_Shutdown(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		setup    func(t *testing.T) *Pool
		validate func(t *testing.T, p *Pool, err error)
	}{
		{
			name: "shutdown sets IsClosed to true",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				return p
			},
			validate: func(t *testing.T, p *Pool, err error) {
				assert.NoError(t, err)
				assert.True(t, p.IsClosed())
			},
		},
		{
			name: "shutdown is idempotent",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				_ = p.Shutdown(ctx) // First shutdown
				return p
			},
			validate: func(t *testing.T, p *Pool, err error) {
				assert.NoError(t, err) // Second shutdown should succeed
				assert.True(t, p.IsClosed())
			},
		},
		{
			name: "shutdown waits for running tasks",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)

				var taskCompleted atomic.Bool
				_ = p.Submit(ctx, func() {
					time.Sleep(50 * time.Millisecond)
					taskCompleted.Store(true)
				})

				// Give task time to start
				time.Sleep(10 * time.Millisecond)

				// Store reference for validation
				return p
			},
			validate: func(t *testing.T, p *Pool, err error) {
				assert.NoError(t, err)
				assert.True(t, p.IsClosed())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.setup(t)
			err := pool.Shutdown(ctx)
			tt.validate(t, pool, err)
		})
	}
}

// ============================================================================
// Pool ShutdownWithTimeout Tests
// ============================================================================

func TestPool_ShutdownWithTimeout(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		setup     func(t *testing.T) *Pool
		timeout   time.Duration
		wantErr   bool
		checkErr  func(t *testing.T, err error)
		checkPool func(t *testing.T, p *Pool)
	}{
		{
			name: "shutdown with sufficient timeout succeeds",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				return p
			},
			timeout: 5 * time.Second,
			wantErr: false,
			checkPool: func(t *testing.T, p *Pool) {
				assert.True(t, p.IsClosed())
			},
		},
		{
			name: "shutdown with very short timeout",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				return p
			},
			timeout: 100 * time.Millisecond,
			wantErr: false,
			checkPool: func(t *testing.T, p *Pool) {
				assert.True(t, p.IsClosed())
			},
		},
		{
			name: "shutdown is idempotent with timeout",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				_ = p.ShutdownWithTimeout(5 * time.Second) // First shutdown
				return p
			},
			timeout: 5 * time.Second,
			wantErr: false,
			checkPool: func(t *testing.T, p *Pool) {
				assert.True(t, p.IsClosed())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.setup(t)

			err := pool.ShutdownWithTimeout(tt.timeout)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.checkPool != nil {
				tt.checkPool(t, pool)
			}
		})
	}
}

// ============================================================================
// Pool Stats Tests
// ============================================================================

func TestPool_Stats(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		config   *Config
		setup    func(t *testing.T, p *Pool)
		validate func(t *testing.T, p *Pool)
	}{
		{
			name:   "stats on idle pool",
			config: &Config{NumWorkers: 10, PreAlloc: true},
			setup: func(t *testing.T, p *Pool) {
				_ = p.Start(ctx)
			},
			validate: func(t *testing.T, p *Pool) {
				stats := p.Stats()
				assert.Equal(t, 10, stats.Capacity)
				assert.Equal(t, 0, stats.Running)
				assert.Equal(t, 0, stats.Waiting)
				assert.Equal(t, 10, stats.Free)

				// Individual methods should match
				assert.Equal(t, stats.Capacity, p.Cap())
				assert.Equal(t, stats.Running, p.Running())
				assert.Equal(t, stats.Waiting, p.Waiting())
				assert.Equal(t, stats.Free, p.Free())
			},
		},
		{
			name:   "stats with running tasks",
			config: &Config{NumWorkers: 5},
			setup: func(t *testing.T, p *Pool) {
				_ = p.Start(ctx)
				// Submit blocking tasks
				for i := 0; i < 3; i++ {
					_ = p.Submit(ctx, func() {
						time.Sleep(100 * time.Millisecond)
					})
				}
				// Give tasks time to start
				time.Sleep(20 * time.Millisecond)
			},
			validate: func(t *testing.T, p *Pool) {
				stats := p.Stats()
				assert.Equal(t, 5, stats.Capacity)
				assert.GreaterOrEqual(t, stats.Running, 1, "should have running tasks")
				assert.Equal(t, 5, stats.Free+stats.Running, "free + running should equal capacity")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPool(ctx, tt.config)
			assert.NoError(t, err)
			defer pool.Shutdown(ctx)

			if tt.setup != nil {
				tt.setup(t, pool)
			}

			tt.validate(t, pool)
		})
	}
}

// ============================================================================
// Pool State Tests
// ============================================================================

func TestPool_State(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		setup       func(t *testing.T) *Pool
		wantStarted bool
		wantClosed  bool
	}{
		{
			name: "new pool is not started and not closed",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				return p
			},
			wantStarted: false,
			wantClosed:  false,
		},
		{
			name: "started pool is started and not closed",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				return p
			},
			wantStarted: true,
			wantClosed:  false,
		},
		{
			name: "shutdown pool is closed",
			setup: func(t *testing.T) *Pool {
				p, err := NewPool(ctx, &Config{NumWorkers: 5})
				assert.NoError(t, err)
				_ = p.Start(ctx)
				_ = p.Shutdown(ctx)
				return p
			},
			wantStarted: true,
			wantClosed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.setup(t)
			if !tt.wantClosed {
				defer pool.Shutdown(ctx)
			}

			assert.Equal(t, tt.wantStarted, pool.IsStarted())
			assert.Equal(t, tt.wantClosed, pool.IsClosed())
		})
	}
}
