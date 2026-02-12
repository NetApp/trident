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
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MultiPool Creation Tests (NewMultiPool)
// ============================================================================

func TestNewMultiPool(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		config      *MultiPoolConfig
		wantErr     bool
		errContains string
		validate    func(t *testing.T, mp *MultiPool)
	}{
		{
			name: "valid config with RoundRobin",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.NotNil(t, mp.multiPool)
				assert.Equal(t, 40, mp.Cap()) // 4 pools * 10 workers
				assert.False(t, mp.IsStarted())
				assert.False(t, mp.IsClosed())
			},
		},
		{
			name: "valid config with LeastTasks",
			config: &MultiPoolConfig{
				NumPools:              2,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: LeastTasks,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.NotNil(t, mp.multiPool)
				assert.Equal(t, 10, mp.Cap()) // 2 pools * 5 workers
			},
		},
		{
			name: "valid config with PreAlloc enabled",
			config: &MultiPoolConfig{
				NumPools:              3,
				NumWorkersPerPool:     4,
				LoadBalancingStrategy: RoundRobin,
				PreAlloc:              true,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.NotNil(t, mp.multiPool)
				assert.Equal(t, 12, mp.Cap())
			},
		},
		{
			name: "valid config with NonBlocking enabled",
			config: &MultiPoolConfig{
				NumPools:              2,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: RoundRobin,
				NonBlocking:           true,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.NotNil(t, mp.multiPool)
				assert.Equal(t, 10, mp.Cap())
			},
		},
		{
			name: "valid config with all options",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     8,
				LoadBalancingStrategy: LeastTasks,
				PreAlloc:              true,
				NonBlocking:           true,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.NotNil(t, mp.multiPool)
				assert.Equal(t, 32, mp.Cap())
			},
		},
		{
			name: "invalid config with zero pools",
			config: &MultiPoolConfig{
				NumPools:              0,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			wantErr: true,
		},
		{
			name: "invalid config with negative pools",
			config: &MultiPoolConfig{
				NumPools:              -1,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			wantErr: true,
		},
		{
			name: "single multipool",
			config: &MultiPoolConfig{
				NumPools:              1,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.Equal(t, 10, mp.Cap())
			},
		},
		{
			name: "many pools with few workers each",
			config: &MultiPoolConfig{
				NumPools:              10,
				NumWorkersPerPool:     2,
				LoadBalancingStrategy: RoundRobin,
			},
			wantErr: false,
			validate: func(t *testing.T, mp *MultiPool) {
				assert.Equal(t, 20, mp.Cap())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, err := NewMultiPool(ctx, tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, mp)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, mp)
			defer mp.Shutdown(ctx)

			if tt.validate != nil {
				tt.validate(t, mp)
			}
		})
	}
}

// ============================================================================
// MultiPool Start Tests
// ============================================================================

func TestMultiPool_Start(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		setup    func(t *testing.T) *MultiPool
		validate func(t *testing.T, mp *MultiPool, err error)
	}{
		{
			name: "start sets IsStarted to true",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				return mp
			},
			validate: func(t *testing.T, mp *MultiPool, err error) {
				assert.NoError(t, err)
				assert.True(t, mp.IsStarted())
			},
		},
		{
			name: "start can be called multiple times",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx) // First call
				return mp
			},
			validate: func(t *testing.T, mp *MultiPool, err error) {
				assert.NoError(t, err)
				assert.True(t, mp.IsStarted())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := tt.setup(t)
			defer mp.Shutdown(ctx)

			err := mp.Start(ctx)
			tt.validate(t, mp, err)
		})
	}
}

// ============================================================================
// MultiPool Submit Tests
// ============================================================================

func TestMultiPool_Submit(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		config      *MultiPoolConfig
		setupPool   func(t *testing.T, mp *MultiPool)
		task        func(executed *atomic.Bool)
		wantErr     bool
		wantErrType error
		checkExec   bool
	}{
		{
			name: "submit task executes successfully",
			config: &MultiPoolConfig{
				NumPools:              2,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: RoundRobin,
			},
			setupPool: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:   false,
			checkExec: true,
		},
		{
			name: "submit to closed multipool returns error",
			config: &MultiPoolConfig{
				NumPools:              2,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: RoundRobin,
			},
			setupPool: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
				_ = mp.Shutdown(ctx)
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:     true,
			wantErrType: ants.ErrPoolClosed,
			checkExec:   false,
		},
		{
			name: "submit with LeastTasks strategy",
			config: &MultiPoolConfig{
				NumPools:              3,
				NumWorkersPerPool:     4,
				LoadBalancingStrategy: LeastTasks,
			},
			setupPool: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			task: func(executed *atomic.Bool) {
				executed.Store(true)
			},
			wantErr:   false,
			checkExec: true,
		},
		{
			name: "submit multiple tasks across pools",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: RoundRobin,
			},
			setupPool: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
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
			mp, err := NewMultiPool(ctx, tt.config)
			require.NoError(t, err)
			defer mp.Shutdown(ctx)

			if tt.setupPool != nil {
				tt.setupPool(t, mp)
			}

			var executed atomic.Bool
			var taskToSubmit func()
			if tt.task != nil {
				taskToSubmit = func() {
					tt.task(&executed)
				}
			}

			err = mp.Submit(ctx, taskToSubmit)

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

func TestMultiPool_Submit_Concurrent(t *testing.T) {
	ctx := context.Background()
	mp, err := NewMultiPool(ctx, &MultiPoolConfig{
		NumPools:              4,
		NumWorkersPerPool:     10,
		LoadBalancingStrategy: RoundRobin,
	})
	require.NoError(t, err)
	defer mp.Shutdown(ctx)

	_ = mp.Start(ctx)

	const numTasks = 200
	var counter atomic.Int32
	var wg sync.WaitGroup

	wg.Add(numTasks)
	for i := 0; i < numTasks; i++ {
		err := mp.Submit(ctx, func() {
			defer wg.Done()
			counter.Add(1)
		})
		require.NoError(t, err)
	}

	wg.Wait()
	assert.Equal(t, int32(numTasks), counter.Load())
}

func TestMultiPool_Submit_LoadBalancing(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		strategy LoadBalancingStrategy
	}{
		{
			name:     "RoundRobin distributes tasks",
			strategy: RoundRobin,
		},
		{
			name:     "LeastTasks distributes tasks",
			strategy: LeastTasks,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, err := NewMultiPool(ctx, &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: tt.strategy,
			})
			require.NoError(t, err)
			defer mp.Shutdown(ctx)

			_ = mp.Start(ctx)

			const numTasks = 50
			var counter atomic.Int32
			var wg sync.WaitGroup

			wg.Add(numTasks)
			for i := 0; i < numTasks; i++ {
				err := mp.Submit(ctx, func() {
					defer wg.Done()
					counter.Add(1)
				})
				require.NoError(t, err)
			}

			wg.Wait()
			assert.Equal(t, int32(numTasks), counter.Load())
		})
	}
}

// ============================================================================
// MultiPool Shutdown Tests
// ============================================================================

func TestMultiPool_Shutdown(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		setup    func(t *testing.T) *MultiPool
		validate func(t *testing.T, mp *MultiPool, err error)
	}{
		{
			name: "shutdown sets IsClosed to true",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				return mp
			},
			validate: func(t *testing.T, mp *MultiPool, err error) {
				assert.NoError(t, err)
				assert.True(t, mp.IsClosed())
			},
		},
		{
			name: "shutdown is idempotent",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				_ = mp.Shutdown(ctx) // First shutdown
				return mp
			},
			validate: func(t *testing.T, mp *MultiPool, err error) {
				assert.NoError(t, err) // Second shutdown should succeed
				assert.True(t, mp.IsClosed())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := tt.setup(t)
			err := mp.Shutdown(ctx)
			tt.validate(t, mp, err)
		})
	}
}

// ============================================================================
// MultiPool ShutdownWithTimeout Tests
// ============================================================================

func TestMultiPool_ShutdownWithTimeout(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		setup     func(t *testing.T) *MultiPool
		timeout   time.Duration
		wantErr   bool
		checkErr  func(t *testing.T, err error)
		checkPool func(t *testing.T, mp *MultiPool)
	}{
		{
			name: "shutdown with sufficient timeout succeeds",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				return mp
			},
			timeout: 5 * time.Second,
			wantErr: false,
			checkPool: func(t *testing.T, mp *MultiPool) {
				assert.True(t, mp.IsClosed())
			},
		},
		{
			name: "shutdown with very short timeout",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				return mp
			},
			timeout: 100 * time.Millisecond,
			wantErr: false,
			checkPool: func(t *testing.T, mp *MultiPool) {
				assert.True(t, mp.IsClosed())
			},
		},
		{
			name: "shutdown is idempotent with timeout",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				_ = mp.ShutdownWithTimeout(5 * time.Second) // First shutdown
				return mp
			},
			timeout: 5 * time.Second,
			wantErr: false,
			checkPool: func(t *testing.T, mp *MultiPool) {
				assert.True(t, mp.IsClosed())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := tt.setup(t)

			err := mp.ShutdownWithTimeout(tt.timeout)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.checkPool != nil {
				tt.checkPool(t, mp)
			}
		})
	}
}

// ============================================================================
// MultiPool Stats Tests
// ============================================================================

func TestMultiPool_Stats(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		config   *MultiPoolConfig
		setup    func(t *testing.T, mp *MultiPool)
		validate func(t *testing.T, mp *MultiPool)
	}{
		{
			name: "stats on idle multipool",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
				PreAlloc:              true,
			},
			setup: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			validate: func(t *testing.T, mp *MultiPool) {
				stats := mp.Stats()
				assert.Equal(t, 40, stats.Capacity)
				assert.Equal(t, 0, stats.Running)
				assert.Equal(t, 0, stats.Waiting)
				assert.Equal(t, 40, stats.Free)

				// Individual methods should match
				assert.Equal(t, stats.Capacity, mp.Cap())
				assert.Equal(t, stats.Running, mp.Running())
				assert.Equal(t, stats.Waiting, mp.Waiting())
				assert.Equal(t, stats.Free, mp.Free())
			},
		},
		{
			name: "stats with running tasks",
			config: &MultiPoolConfig{
				NumPools:              2,
				NumWorkersPerPool:     5,
				LoadBalancingStrategy: RoundRobin,
			},
			setup: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
				// Submit blocking tasks
				for i := 0; i < 4; i++ {
					_ = mp.Submit(ctx, func() {
						time.Sleep(100 * time.Millisecond)
					})
				}
				// Give tasks time to start
				time.Sleep(20 * time.Millisecond)
			},
			validate: func(t *testing.T, mp *MultiPool) {
				stats := mp.Stats()
				assert.Equal(t, 10, stats.Capacity)
				assert.GreaterOrEqual(t, stats.Running, 1, "should have running tasks")
				assert.Equal(t, 10, stats.Free+stats.Running, "free + running should equal capacity")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, err := NewMultiPool(ctx, tt.config)
			require.NoError(t, err)
			defer mp.Shutdown(ctx)

			if tt.setup != nil {
				tt.setup(t, mp)
			}

			tt.validate(t, mp)
		})
	}
}

// ============================================================================
// MultiPool ByIndex Stats Tests
// ============================================================================

func TestMultiPool_StatsByIndex(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		config   *MultiPoolConfig
		idx      int
		setup    func(t *testing.T, mp *MultiPool)
		wantErr  bool
		validate func(t *testing.T, running, free, waiting int)
	}{
		{
			name: "valid index returns stats",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
				PreAlloc:              true,
			},
			idx: 0,
			setup: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			wantErr: false,
			validate: func(t *testing.T, running, free, waiting int) {
				assert.Equal(t, 0, running)
				assert.Equal(t, 10, free)
				assert.Equal(t, 0, waiting)
			},
		},
		{
			name: "last valid index returns stats",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			idx: 3, // Last multiPool (0-indexed)
			setup: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			wantErr: false,
			validate: func(t *testing.T, running, free, waiting int) {
				assert.GreaterOrEqual(t, free, 0)
			},
		},
		{
			name: "invalid index (out of bounds) returns error",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			idx: 4, // Out of bounds
			setup: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			wantErr: true,
		},
		{
			name: "negative index returns error",
			config: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
			},
			idx: -1,
			setup: func(t *testing.T, mp *MultiPool) {
				_ = mp.Start(ctx)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, err := NewMultiPool(ctx, tt.config)
			require.NoError(t, err)
			defer mp.Shutdown(ctx)

			if tt.setup != nil {
				tt.setup(t, mp)
			}

			running, runErr := mp.RunningByIndex(tt.idx)
			free, freeErr := mp.FreeByIndex(tt.idx)
			waiting, waitErr := mp.WaitingByIndex(tt.idx)

			if tt.wantErr {
				assert.Error(t, runErr)
				assert.Error(t, freeErr)
				assert.Error(t, waitErr)
				return
			}

			assert.NoError(t, runErr)
			assert.NoError(t, freeErr)
			assert.NoError(t, waitErr)

			if tt.validate != nil {
				tt.validate(t, running, free, waiting)
			}
		})
	}
}

// ============================================================================
// MultiPool State Tests
// ============================================================================

func TestMultiPool_State(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		setup       func(t *testing.T) *MultiPool
		wantStarted bool
		wantClosed  bool
	}{
		{
			name: "new multipool is not started and not closed",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				return mp
			},
			wantStarted: false,
			wantClosed:  false,
		},
		{
			name: "started multipool is started and not closed",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				return mp
			},
			wantStarted: true,
			wantClosed:  false,
		},
		{
			name: "shutdown multipool is closed",
			setup: func(t *testing.T) *MultiPool {
				mp, err := NewMultiPool(ctx, &MultiPoolConfig{
					NumPools:              2,
					NumWorkersPerPool:     5,
					LoadBalancingStrategy: RoundRobin,
				})
				require.NoError(t, err)
				_ = mp.Start(ctx)
				_ = mp.Shutdown(ctx)
				return mp
			},
			wantStarted: true,
			wantClosed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := tt.setup(t)
			if !tt.wantClosed {
				defer mp.Shutdown(ctx)
			}

			assert.Equal(t, tt.wantStarted, mp.IsStarted())
			assert.Equal(t, tt.wantClosed, mp.IsClosed())
		})
	}
}
