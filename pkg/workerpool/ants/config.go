// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"time"

	"github.com/brunoga/deep"
	"github.com/panjf2000/ants/v2"

	"github.com/netapp/trident/pkg/workerpool/types"
)

// ============================================================================
// Pool Configuration
// ============================================================================

// Config holds configuration options for creating an ants worker pool.
type Config struct {
	// NumWorkers is the number of worker goroutines.
	NumWorkers int

	// PreAlloc pre-allocates workers on pool creation for better performance.
	PreAlloc bool

	// NonBlocking makes Submit return immediately with an error if the pool is busy.
	NonBlocking bool

	// ExpiryDuration is the period for cleaning up expired workers.
	// Workers that have been idle for longer than this duration will be purged.
	ExpiryDuration time.Duration

	// DisablePurge prevents the pool from purging idle workers.
	DisablePurge bool
}

// PoolConfig is a marker method to implement the workerpool.Config interface.
func (*Config) PoolConfig() {}

// Copy returns a deep copy of this configuration.
func (c *Config) Copy() types.Config {
	if c == nil {
		return nil
	}

	// Use deep copy to handle nested structures
	copied, err := deep.Copy(c)
	if err != nil {
		// Fallback to simple struct copy if deep copy fails
		cfgCopy := *c
		return &cfgCopy
	}
	return copied
}

// ============================================================================
// Config Functional Options
// ============================================================================

// ConfigOption is a functional option for configuring a worker pool.
type ConfigOption func(*Config)

// WithNumWorkers sets the number of worker goroutines.
func WithNumWorkers(n int) ConfigOption {
	return func(c *Config) {
		c.NumWorkers = n
	}
}

// WithPreAlloc enables pre-allocation of workers on pool creation.
func WithPreAlloc(preAlloc bool) ConfigOption {
	return func(c *Config) {
		c.PreAlloc = preAlloc
	}
}

// WithNonBlocking enables non-blocking mode for Submit operations.
func WithNonBlocking(nonBlocking bool) ConfigOption {
	return func(c *Config) {
		c.NonBlocking = nonBlocking
	}
}

// WithExpiryDuration sets the period for cleaning up expired workers.
func WithExpiryDuration(d time.Duration) ConfigOption {
	return func(c *Config) {
		c.ExpiryDuration = d
	}
}

// WithDisablePurge disables purging of idle workers.
func WithDisablePurge(disable bool) ConfigOption {
	return func(c *Config) {
		c.DisablePurge = disable
	}
}

// NewConfig creates a new Config with the provided options.
func NewConfig(opts ...ConfigOption) *Config {
	// Setting up the necessary defaults
	cfg := &Config{
		NumWorkers:     defaultNumWorkers,
		PreAlloc:       true,
		ExpiryDuration: defaultExpiryDuration,
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// ============================================================================
// MultiPool Configuration
// ============================================================================

// LoadBalancingStrategy represents the load-balancing algorithm for MultiPool.
type LoadBalancingStrategy = ants.LoadBalancingStrategy

// Load balancing strategies supported by MultiPool.
const (
	RoundRobin LoadBalancingStrategy = ants.RoundRobin
	LeastTasks LoadBalancingStrategy = ants.LeastTasks
)

// MultiPoolConfig holds configuration options for creating an ants multiPool.
// A multiPool consists of multiple underlying pools with load balancing,
// providing better performance through reduced lock contention.
type MultiPoolConfig struct {
	// NumPools is the number of underlying pools to create.
	NumPools int

	// NumWorkersPerPool is the number of workers in each underlying multiPool.
	NumWorkersPerPool int

	// LoadBalancingStrategy determines how tasks are distributed across pools.
	LoadBalancingStrategy LoadBalancingStrategy

	// PreAlloc pre-allocates workers on multiPool creation for better performance.
	PreAlloc bool

	// NonBlocking makes Submit return immediately with an error if pools are busy.
	NonBlocking bool

	// ExpiryDuration is the period for cleaning up expired workers.
	// Workers that have been idle for longer than this duration will be purged.
	ExpiryDuration time.Duration

	// DisablePurge prevents the multiPool from purging idle workers.
	DisablePurge bool
}

// PoolConfig is a marker method to implement the workerpool.Config interface.
func (*MultiPoolConfig) PoolConfig() {}

func (m *MultiPoolConfig) Copy() types.Config {
	if m == nil {
		return nil
	}

	// Use deep copy to handle nested structures
	copied, err := deep.Copy(m)
	if err != nil {
		// Fallback to simple struct copy if deep copy fails
		cfgCopy := *m
		return &cfgCopy
	}
	return copied
}

// ============================================================================
// MultiPoolConfig Functional Options
// ============================================================================

// MultiPoolConfigOption is a functional option for configuring a multipool.
type MultiPoolConfigOption func(*MultiPoolConfig)

// WithNumPools sets the number of underlying pools to create.
func WithNumPools(n int) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.NumPools = n
	}
}

// WithNumWorkersPerPool sets the number of workers in each underlying pool.
func WithNumWorkersPerPool(n int) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.NumWorkersPerPool = n
	}
}

// WithLoadBalancingStrategy sets the load-balancing algorithm for MultiPool.
func WithLoadBalancingStrategy(strategy LoadBalancingStrategy) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.LoadBalancingStrategy = strategy
	}
}

// WithMultiPoolPreAlloc enables pre-allocation of workers on multipool creation.
func WithMultiPoolPreAlloc(preAlloc bool) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.PreAlloc = preAlloc
	}
}

// WithMultiPoolNonBlocking enables non-blocking mode for Submit operations.
func WithMultiPoolNonBlocking(nonBlocking bool) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.NonBlocking = nonBlocking
	}
}

// WithMultiPoolExpiryDuration sets the period for cleaning up expired workers.
func WithMultiPoolExpiryDuration(d time.Duration) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.ExpiryDuration = d
	}
}

// WithMultiPoolDisablePurge disables purging of idle workers.
func WithMultiPoolDisablePurge(disable bool) MultiPoolConfigOption {
	return func(c *MultiPoolConfig) {
		c.DisablePurge = disable
	}
}

// NewMultiPoolConfig creates a new MultiPoolConfig with the provided options.
func NewMultiPoolConfig(opts ...MultiPoolConfigOption) *MultiPoolConfig {
	// Setting up the necessary defaults
	cfg := &MultiPoolConfig{
		NumPools:              defaultNumWorkers,
		NumWorkersPerPool:     defaultNumWorkers,
		LoadBalancingStrategy: RoundRobin,
		PreAlloc:              true,
		ExpiryDuration:        defaultExpiryDuration,
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
