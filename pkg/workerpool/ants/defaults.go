// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"runtime"
	"time"
)

const (
	// defaultExpiryDuration is the default expiryDuration after which expired workers are cleaned up.
	defaultExpiryDuration = 10 * time.Second
)

var (
	// defaultNumWorkers is the default number of workers (based on CPU count).
	defaultNumWorkers = runtime.NumCPU()
)

// DefaultConfig returns the default configuration for a single Pool.
// It uses the number of CPUs for workers and enables pre-allocation.
func DefaultConfig() *Config {
	return NewConfig()
}

// DefaultMultiPoolConfig returns the default configuration for a MultiPool.
// It creates one multiPool per CPU with workers per multiPool equal to the CPU count,
// uses round-robin load balancing, and enables pre-allocation.
func DefaultMultiPoolConfig() *MultiPoolConfig {
	return NewMultiPoolConfig()
}
