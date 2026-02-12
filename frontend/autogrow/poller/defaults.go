// Copyright 2026 NetApp, Inc. All Rights Reserved.

package poller

import (
	"runtime"
	"time"
)

const (
	// DefaultWorkQueueName is the default name for the poller work queue
	DefaultWorkQueueName = "autogrow-poller"

	// DefaultWorkerPoolPreAlloc determines if goroutines are pre-allocated
	DefaultWorkerPoolPreAlloc = true

	// DefaultWorkerPoolNonBlocking determines if pool submission should block when full
	DefaultWorkerPoolNonBlocking = false

	// DefaultMaxRetries is the maximum number of times a work item will be retried
	DefaultMaxRetries = 3

	// DefaultShutdownTimeout is how long to wait for workers to drain during shutdown
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultTridentNamespace is the default namespace where Trident resources are located
	DefaultTridentNamespace = ""

	// Workqueue rate limiting configuration
	// These constants define the rate limiter behavior combining exponential backoff with bucket rate limiting
	// Poller can be aggressive as it does local CSI operations (volume stats) and in-memory eventbus publishes
	// It does NOT directly hit the kube-apiserver (only reads from informer cache)

	// WorkqueueBaseDelay is the initial delay for exponential backoff on retries
	// Items will be retried after this delay on first failure
	WorkqueueBaseDelay = 5 * time.Millisecond

	// WorkqueueMaxDelay is the maximum delay for exponential backoff
	// Even with exponential backoff, delay will not exceed this value
	WorkqueueMaxDelay = 1000 * time.Second

	// WorkqueueBucketQPS defines the queries per second for the bucket rate limiter
	// Higher than requester since poller doesn't hit kube-apiserver directly
	WorkqueueBucketQPS = 50

	// WorkqueueBucketBurst defines the burst capacity for the bucket rate limiter
	// Allows temporary bursts - can handle many volumes at once since operations are local
	WorkqueueBucketBurst = 500
)

var (
	// DefaultWorkerPoolSize is the default number of workers in the pool
	// Calculated dynamically based on both burst capacity and available CPUs
	// Poller can be more aggressive since it doesn't hit kube-apiserver
	// Formula: max(CPU*4, burst/10) - using 4x CPU since operations are local (volume stats + eventbus)
	DefaultWorkerPoolSize = calculateWorkerPoolSize()
)

// calculateWorkerPoolSize determines optimal worker pool size based on system resources and rate limits
// Poller does local CSI operations (volume stats) and in-memory eventbus publishes, not API calls
func calculateWorkerPoolSize() int {
	// Use 4x CPU for poller since it's doing local I/O, not remote API calls
	cpuBased := runtime.NumCPU() * 4
	// 10% of burst capacity (500/10 = 50 workers at burst)
	burstBased := WorkqueueBucketBurst / 10

	// Return the maximum to ensure adequate workers in all scenarios
	if cpuBased > burstBased {
		return cpuBased
	}
	return burstBased
}

// DefaultConfig returns the default configuration for the Poller layer.
// It uses:
// - MaxRetries: 3
// - 30 second shutdown timeout
// - Default work queue name and namespace
func DefaultConfig() *Config {
	return NewConfig()
}
