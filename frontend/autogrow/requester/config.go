// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

import (
	"time"

	poolTypes "github.com/netapp/trident/pkg/workerpool/types"
)

// Config holds the configuration for the Requester layer
type Config struct {
	// Optional: Provide an existing worker pool, otherwise one will be created with default settings
	WorkerPool poolTypes.Pool

	// ShutdownTimeout is how long to wait for workers to drain during shutdown
	ShutdownTimeout time.Duration

	// WorkQueueName for identification in metrics
	WorkQueueName string

	// MaxRetries is the maximum number of times a work item will be retried
	MaxRetries int

	// TridentNamespace is where TAGRI CRs are created
	TridentNamespace string
}

// ConfigOption is a function that modifies a Config
type ConfigOption func(*Config)

// NewConfig creates a new Config with default values and applies the provided options.
func NewConfig(opts ...ConfigOption) *Config {
	config := &Config{
		ShutdownTimeout:  DefaultShutdownTimeout,
		WorkQueueName:    DefaultWorkQueueName,
		MaxRetries:       DefaultMaxRetries,
		TridentNamespace: DefaultTridentNamespace,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return config
}

// WithWorkerPool sets an existing worker pool to use
func WithWorkerPool(pool poolTypes.Pool) ConfigOption {
	return func(c *Config) {
		c.WorkerPool = pool
	}
}

// WithShutdownTimeout sets the shutdown timeout duration
func WithShutdownTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.ShutdownTimeout = timeout
	}
}

// WithWorkQueueName sets the work queue name for metrics
func WithWorkQueueName(name string) ConfigOption {
	return func(c *Config) {
		c.WorkQueueName = name
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(retries int) ConfigOption {
	return func(c *Config) {
		c.MaxRetries = retries
	}
}

// WithTridentNamespace sets the namespace where TAGRI CRs are created
func WithTridentNamespace(namespace string) ConfigOption {
	return func(c *Config) {
		c.TridentNamespace = namespace
	}
}

// Copy returns a deep copy of this configuration.
// This allows config to be safely reused across multiple requesters.
// Note: WorkerPool field is NOT deep copied (shared reference is intentional).
func (c *Config) Copy() *Config {
	if c == nil {
		return nil
	}

	// Create a new Config with copied values
	newConfig := &Config{
		WorkerPool:       c.WorkerPool, // Share the pool reference (intentional)
		ShutdownTimeout:  c.ShutdownTimeout,
		WorkQueueName:    c.WorkQueueName,
		MaxRetries:       c.MaxRetries,
		TridentNamespace: c.TridentNamespace,
	}

	return newConfig
}
