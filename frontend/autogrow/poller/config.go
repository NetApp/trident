// Copyright 2026 NetApp, Inc. All Rights Reserved.

package poller

import (
	"time"

	poolTypes "github.com/netapp/trident/pkg/workerpool/types"
)

// Config holds the configuration for the Poller layer
type Config struct {
	// Optional: Provide an existing worker pool, otherwise one will be created with default settings
	WorkerPool poolTypes.Pool

	// AutogrowPeriod is the Autogrow scheduling period from the command line.
	// If set, the poller will configure its worker pool expiry duration accordingly
	// to prevent goroutines from being destroyed between polling cycles.
	// Use a pointer to distinguish between "not set" (nil) and "set to zero" (valid but unusual).
	AutogrowPeriod *time.Duration

	// MaxRetries is the maximum number of times a work item will be retried
	MaxRetries int

	// ShutdownTimeout is how long to wait for workers to drain during shutdown
	ShutdownTimeout time.Duration

	// WorkQueueName for identification in metrics
	WorkQueueName string

	// TridentNamespace is the namespace where Trident resources are located
	TridentNamespace string
}

// ConfigOption is a function that modifies a Config
type ConfigOption func(*Config)

// NewConfig creates a new Config with default values and applies the provided options.
func NewConfig(opts ...ConfigOption) *Config {
	config := &Config{
		MaxRetries:       DefaultMaxRetries,
		ShutdownTimeout:  DefaultShutdownTimeout,
		WorkQueueName:    DefaultWorkQueueName,
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

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(retries int) ConfigOption {
	return func(c *Config) {
		c.MaxRetries = retries
	}
}

// WithShutdownTimeout sets the timeout for graceful shutdown
func WithShutdownTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.ShutdownTimeout = timeout
	}
}

// WithWorkQueueName sets the name for the work queue
func WithWorkQueueName(name string) ConfigOption {
	return func(c *Config) {
		c.WorkQueueName = name
	}
}

// WithTridentNamespace sets the Trident namespace
func WithTridentNamespace(namespace string) ConfigOption {
	return func(c *Config) {
		c.TridentNamespace = namespace
	}
}

// WithAutogrowPeriod sets the Autogrow period for worker pool expiry configuration
func WithAutogrowPeriod(period time.Duration) ConfigOption {
	return func(c *Config) {
		c.AutogrowPeriod = &period
	}
}

// Copy returns a deep copy of this configuration.
// This allows config to be safely reused across multiple pollers.
// Note: WorkerPool field is NOT deep copied (shared reference is intentional).
func (c *Config) Copy() *Config {
	if c == nil {
		return nil
	}

	// Create a new Config with copied values
	newConfig := &Config{
		WorkerPool:       c.WorkerPool, // Share the pool reference (intentional)
		MaxRetries:       c.MaxRetries,
		ShutdownTimeout:  c.ShutdownTimeout,
		WorkQueueName:    c.WorkQueueName,
		TridentNamespace: c.TridentNamespace,
	}

	// Deep copy AutogrowPeriod pointer
	if c.AutogrowPeriod != nil {
		period := *c.AutogrowPeriod
		newConfig.AutogrowPeriod = &period
	}

	return newConfig
}
