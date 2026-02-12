// Copyright 2025 NetApp, Inc. All Rights Reserved.

package workerpool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/pkg/workerpool/ants"
	"github.com/netapp/trident/pkg/workerpool/types"
)

func TestNew(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		config  types.Config
		wantErr bool
	}{
		{
			name:    "Pool with default config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "Pool with ants.Config",
			config:  &ants.Config{NumWorkers: 5},
			wantErr: false,
		},
		{
			name: "Pool with full ants.Config",
			config: &ants.Config{
				NumWorkers:     10,
				PreAlloc:       true,
				NonBlocking:    false,
				ExpiryDuration: 5 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pool types.Pool
			var err error

			if tt.config == nil {
				pool, err = New[types.Pool](ctx)
			} else {
				pool, err = New[types.Pool](ctx, tt.config)
			}

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, pool)
			defer pool.Shutdown(ctx)

			// Verify pool is functional
			err = pool.Start(ctx)
			assert.NoError(t, err)
			assert.True(t, pool.IsStarted())
		})
	}
}

func TestNewMultiPool(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		config  types.Config
		wantErr bool
	}{
		{
			name:    "Pool with default config",
			config:  ants.DefaultMultiPoolConfig(),
			wantErr: false,
		},
		{
			name: "Pool with ants.Config",
			config: &ants.MultiPoolConfig{NumPools: 5,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: ants.LeastTasks,
			},
			wantErr: false,
		},
		{
			name:    "Pool with wrong config",
			config:  ants.DefaultConfig(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pool types.Pool
			var err error

			if tt.config == nil {
				pool, err = New[types.MultiPool](ctx)
			} else {
				pool, err = New[types.MultiPool](ctx, tt.config)
			}

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, pool)
			defer pool.Shutdown(ctx)

			// Verify pool is functional
			err = pool.Start(ctx)
			assert.NoError(t, err)
			assert.True(t, pool.IsStarted())
		})
	}
}

func TestMustNew(t *testing.T) {
	ctx := context.Background()
	t.Run("MustNew succeeds with valid config", func(t *testing.T) {
		assert.NotPanics(t, func() {
			pool := MustNew[types.Pool](ctx, &ants.Config{NumWorkers: 5})
			defer pool.Shutdown(ctx)
			assert.NotNil(t, pool)
		})
	})

	t.Run("MustNew succeeds with default config", func(t *testing.T) {
		assert.NotPanics(t, func() {
			pool := MustNew[types.Pool](ctx)
			defer pool.Shutdown(ctx)
			assert.NotNil(t, pool)
		})
	})

	t.Run("MustNew panic with wrong config", func(t *testing.T) {
		assert.Panics(t, func() {
			pool := MustNew[types.MultiPool](ctx, ants.DefaultConfig())
			defer pool.Shutdown(ctx)
			assert.Nil(t, pool)
		})
	})
}
