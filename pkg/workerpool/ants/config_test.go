// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Config Tests
// ============================================================================

func TestConfig_PoolConfig(t *testing.T) {
	cfg := &Config{}
	cfg.PoolConfig() // Should not panic
}

func TestConfig_Copy(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "nil config",
			cfg:  nil,
		},
		{
			name: "empty config",
			cfg:  &Config{},
		},
		{
			name: "full config",
			cfg: &Config{
				NumWorkers:     10,
				PreAlloc:       true,
				NonBlocking:    true,
				ExpiryDuration: 5 * time.Second,
				DisablePurge:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.cfg.Copy()

			if tt.cfg == nil {
				assert.Nil(t, copied)
				return
			}

			assert.NotNil(t, copied)
			copiedCfg := copied.(*Config)

			// Verify values match
			assert.Equal(t, tt.cfg.NumWorkers, copiedCfg.NumWorkers)
			assert.Equal(t, tt.cfg.PreAlloc, copiedCfg.PreAlloc)
			assert.Equal(t, tt.cfg.NonBlocking, copiedCfg.NonBlocking)
			assert.Equal(t, tt.cfg.ExpiryDuration, copiedCfg.ExpiryDuration)
			assert.Equal(t, tt.cfg.DisablePurge, copiedCfg.DisablePurge)

			// Verify it's a different instance
			assert.NotSame(t, tt.cfg, copiedCfg)
		})
	}
}

func TestWithNumWorkers(t *testing.T) {
	cfg := &Config{}
	opt := WithNumWorkers(5)
	opt(cfg)
	assert.Equal(t, 5, cfg.NumWorkers)
}

func TestWithPreAlloc(t *testing.T) {
	tests := []struct {
		name     string
		preAlloc bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			opt := WithPreAlloc(tt.preAlloc)
			opt(cfg)
			assert.Equal(t, tt.preAlloc, cfg.PreAlloc)
		})
	}
}

func TestWithNonBlocking(t *testing.T) {
	tests := []struct {
		name        string
		nonBlocking bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			opt := WithNonBlocking(tt.nonBlocking)
			opt(cfg)
			assert.Equal(t, tt.nonBlocking, cfg.NonBlocking)
		})
	}
}

func TestWithExpiryDuration(t *testing.T) {
	duration := 10 * time.Second
	cfg := &Config{}
	opt := WithExpiryDuration(duration)
	opt(cfg)
	assert.Equal(t, duration, cfg.ExpiryDuration)
}

func TestWithDisablePurge(t *testing.T) {
	tests := []struct {
		name    string
		disable bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			opt := WithDisablePurge(tt.disable)
			opt(cfg)
			assert.Equal(t, tt.disable, cfg.DisablePurge)
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name string
		opts []ConfigOption
		want *Config
	}{
		{
			name: "no options",
			opts: nil,
			want: &Config{
				NumWorkers:     defaultNumWorkers,
				PreAlloc:       true,
				ExpiryDuration: defaultExpiryDuration},
		},
		{
			name: "single option",
			opts: []ConfigOption{WithNumWorkers(3)},
			want: &Config{
				NumWorkers:     3,
				PreAlloc:       true,
				ExpiryDuration: defaultExpiryDuration,
			},
		},
		{
			name: "multiple options",
			opts: []ConfigOption{
				WithNumWorkers(5),
				WithPreAlloc(true),
				WithNonBlocking(true),
				WithExpiryDuration(10 * time.Second),
				WithDisablePurge(true),
			},
			want: &Config{
				NumWorkers:     5,
				PreAlloc:       true,
				NonBlocking:    true,
				ExpiryDuration: 10 * time.Second,
				DisablePurge:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewConfig(tt.opts...)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
// MultiPoolConfig Tests
// ============================================================================

func TestMultiPoolConfig_PoolConfig(t *testing.T) {
	cfg := &MultiPoolConfig{}
	cfg.PoolConfig() // Should not panic
}

func TestMultiPoolConfig_Copy(t *testing.T) {
	tests := []struct {
		name string
		cfg  *MultiPoolConfig
	}{
		{
			name: "nil config",
			cfg:  nil,
		},
		{
			name: "empty config",
			cfg:  &MultiPoolConfig{},
		},
		{
			name: "full config",
			cfg: &MultiPoolConfig{
				NumPools:              4,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: RoundRobin,
				PreAlloc:              true,
				NonBlocking:           true,
				ExpiryDuration:        5 * time.Second,
				DisablePurge:          true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.cfg.Copy()

			if tt.cfg == nil {
				assert.Nil(t, copied)
				return
			}

			assert.NotNil(t, copied)
			copiedCfg := copied.(*MultiPoolConfig)

			// Verify values match
			assert.Equal(t, tt.cfg.NumPools, copiedCfg.NumPools)
			assert.Equal(t, tt.cfg.NumWorkersPerPool, copiedCfg.NumWorkersPerPool)
			assert.Equal(t, tt.cfg.LoadBalancingStrategy, copiedCfg.LoadBalancingStrategy)
			assert.Equal(t, tt.cfg.PreAlloc, copiedCfg.PreAlloc)
			assert.Equal(t, tt.cfg.NonBlocking, copiedCfg.NonBlocking)
			assert.Equal(t, tt.cfg.ExpiryDuration, copiedCfg.ExpiryDuration)
			assert.Equal(t, tt.cfg.DisablePurge, copiedCfg.DisablePurge)

			// Verify it's a different instance
			assert.NotSame(t, tt.cfg, copiedCfg)
		})
	}
}

func TestWithNumPools(t *testing.T) {
	cfg := &MultiPoolConfig{}
	opt := WithNumPools(4)
	opt(cfg)
	assert.Equal(t, 4, cfg.NumPools)
}

func TestWithNumWorkersPerPool(t *testing.T) {
	cfg := &MultiPoolConfig{}
	opt := WithNumWorkersPerPool(8)
	opt(cfg)
	assert.Equal(t, 8, cfg.NumWorkersPerPool)
}

func TestWithLoadBalancingStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy LoadBalancingStrategy
	}{
		{"RoundRobin", RoundRobin},
		{"LeastTasks", LeastTasks},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MultiPoolConfig{}
			opt := WithLoadBalancingStrategy(tt.strategy)
			opt(cfg)
			assert.Equal(t, tt.strategy, cfg.LoadBalancingStrategy)
		})
	}
}

func TestWithMultiPoolPreAlloc(t *testing.T) {
	tests := []struct {
		name     string
		preAlloc bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MultiPoolConfig{}
			opt := WithMultiPoolPreAlloc(tt.preAlloc)
			opt(cfg)
			assert.Equal(t, tt.preAlloc, cfg.PreAlloc)
		})
	}
}

func TestWithMultiPoolNonBlocking(t *testing.T) {
	tests := []struct {
		name        string
		nonBlocking bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MultiPoolConfig{}
			opt := WithMultiPoolNonBlocking(tt.nonBlocking)
			opt(cfg)
			assert.Equal(t, tt.nonBlocking, cfg.NonBlocking)
		})
	}
}

func TestWithMultiPoolExpiryDuration(t *testing.T) {
	duration := 15 * time.Second
	cfg := &MultiPoolConfig{}
	opt := WithMultiPoolExpiryDuration(duration)
	opt(cfg)
	assert.Equal(t, duration, cfg.ExpiryDuration)
}

func TestWithMultiPoolDisablePurge(t *testing.T) {
	tests := []struct {
		name    string
		disable bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MultiPoolConfig{}
			opt := WithMultiPoolDisablePurge(tt.disable)
			opt(cfg)
			assert.Equal(t, tt.disable, cfg.DisablePurge)
		})
	}
}

func TestNewMultiPoolConfig(t *testing.T) {
	tests := []struct {
		name string
		opts []MultiPoolConfigOption
		want *MultiPoolConfig
	}{
		{
			name: "no options",
			opts: nil,
			want: &MultiPoolConfig{
				NumPools:              defaultNumWorkers,
				NumWorkersPerPool:     defaultNumWorkers,
				LoadBalancingStrategy: RoundRobin,
				PreAlloc:              true,
				ExpiryDuration:        defaultExpiryDuration,
			},
		},
		{
			name: "single option",
			opts: []MultiPoolConfigOption{WithNumPools(2)},
			want: &MultiPoolConfig{
				NumPools:              2,
				NumWorkersPerPool:     defaultNumWorkers,
				LoadBalancingStrategy: RoundRobin,
				PreAlloc:              true,
				ExpiryDuration:        defaultExpiryDuration,
			},
		},
		{
			name: "multiple options",
			opts: []MultiPoolConfigOption{
				WithNumPools(3),
				WithNumWorkersPerPool(10),
				WithLoadBalancingStrategy(LeastTasks),
				WithMultiPoolPreAlloc(true),
				WithMultiPoolNonBlocking(true),
				WithMultiPoolExpiryDuration(20 * time.Second),
				WithMultiPoolDisablePurge(true),
			},
			want: &MultiPoolConfig{
				NumPools:              3,
				NumWorkersPerPool:     10,
				LoadBalancingStrategy: LeastTasks,
				PreAlloc:              true,
				NonBlocking:           true,
				ExpiryDuration:        20 * time.Second,
				DisablePurge:          true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewMultiPoolConfig(tt.opts...)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
// Load Balancing Strategy Constants Tests
// ============================================================================

func TestLoadBalancingStrategyConstants(t *testing.T) {
	// Verify that constants are defined and accessible
	assert.NotNil(t, RoundRobin)
	assert.NotNil(t, LeastTasks)

	// Verify they are different values
	assert.NotEqual(t, RoundRobin, LeastTasks)
}
