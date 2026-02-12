// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

import (
	"time"
)

const (
	// DefaultWorkQueueName is the default name for the requester work queue
	DefaultWorkQueueName = "autogrow-requester"

	// DefaultTridentNamespace is the default namespace for TAGRI CRs
	DefaultTridentNamespace = ""

	// DefaultWorkerPoolPreAlloc determines if goroutines are pre-allocated
	DefaultWorkerPoolPreAlloc = true

	// DefaultWorkerPoolNonBlocking determines if pool submission should block when full
	DefaultWorkerPoolNonBlocking = false

	// DefaultShutdownTimeout is how long to wait for workers to drain during shutdown
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultMaxRetries is the maximum number of times a work item will be retried
	DefaultMaxRetries = 3

	// Workqueue rate limiting configuration
	// These constants define the rate limiter behavior combining exponential backoff with bucket rate limiting
	// Requester must be conservative as it creates TAGRI CRs via kube-apiserver API calls
	// We need to be mindful of not overwhelming the API server
	// Values are set to 10% of tridentClientSet client-go rate limiter (100 QPS, 200 burst)

	// WorkqueueBaseDelay is the initial delay for exponential backoff on retries
	// Items will be retried after this delay on first failure
	WorkqueueBaseDelay = 5 * time.Millisecond

	// WorkqueueMaxDelay is the maximum delay for exponential backoff
	// Even with exponential backoff, delay will not exceed this value
	WorkqueueMaxDelay = 1000 * time.Second

	// WorkqueueBucketQPS defines the queries per second for the bucket rate limiter
	// Set to 10% of tridentClientSet client-go QPS limit (10% of 100 = 10 QPS)
	WorkqueueBucketQPS = 10

	// WorkqueueBucketBurst defines the burst capacity for the bucket rate limiter
	// Set to 10% of tridentClientSet client-go burst limit (10% of 200 = 20 burst)
	WorkqueueBucketBurst = 20
)

var (
	// defaultWorkerPoolSize is the default number of workers in the pool
	// Set equal to WorkqueueBucketQPS to match the rate limiting capacity
	// Requester makes kube-apiserver API calls (TAGRI creation) and must respect
	// TridentClientSet rate limits (100 QPS, 200 burst)
	// Setting workers = QPS ensures we can handle the configured throughput without overwhelming the API server
	defaultWorkerPoolSize = WorkqueueBucketQPS
)

// DefaultConfig returns the default configuration for the Requester layer.
// It uses:
// - MaxRetries: 3
// - 30 second shutdown timeout
// - Default work queue name
// - Default trident namespace
func DefaultConfig() *Config {
	return NewConfig()
}
