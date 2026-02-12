// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ants

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, runtime.NumCPU(), cfg.NumWorkers)
	assert.True(t, cfg.PreAlloc)
	assert.Equal(t, 10*time.Second, cfg.ExpiryDuration)
}

func TestDefaultMultiPoolConfig(t *testing.T) {
	cfg := DefaultMultiPoolConfig()

	assert.Equal(t, runtime.NumCPU(), cfg.NumPools)
	assert.Equal(t, runtime.NumCPU(), cfg.NumWorkersPerPool)
	assert.Equal(t, RoundRobin, cfg.LoadBalancingStrategy)
	assert.True(t, cfg.PreAlloc)
	assert.Equal(t, 10*time.Second, cfg.ExpiryDuration)
}
