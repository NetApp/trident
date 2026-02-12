// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"time"
)

// Config holds the configuration for the Scheduler layer
type Config struct {
	// ShutdownTimeout is the maximum duration to wait for graceful shutdown
	ShutdownTimeout time.Duration

	// WorkQueueName is the name of the work queue for tracking and debugging
	WorkQueueName string

	// MaxRetries is the maximum number of times to retry processing a failed work item
	MaxRetries int

	// AssorterType specifies which assorter implementation to use (e.g., Periodic)
	AssorterType AssorterType

	// AssorterPeriod is the interval at which the assorter schedules volumes for polling
	AssorterPeriod time.Duration

	// ReconciliationPeriod is the interval at which the scheduler reconciles cache with API server
	// This catches any drift from missed events or informer issues
	ReconciliationPeriod time.Duration

	// TridentNamespace is the namespace where Trident is installed
	TridentNamespace string
}

// AssorterType specifies the type of assorter implementation to use
type AssorterType int

const (
	// AssorterTypePeriodic creates a simple periodic assorter that polls all volumes at a fixed interval
	AssorterTypePeriodic AssorterType = iota
)

// String returns the string representation of AssorterType
func (a AssorterType) String() string {
	switch a {
	case AssorterTypePeriodic:
		return "Periodic"
	default:
		return "Unknown"
	}
}

// ConfigOption is a functional option for configuring the Scheduler
type ConfigOption func(*Config)

// WithShutdownTimeout sets the shutdown timeout
func WithShutdownTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.ShutdownTimeout = timeout
	}
}

// WithWorkQueueName sets the work queue name
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

// WithTridentNamespace sets the Trident namespace
func WithTridentNamespace(namespace string) ConfigOption {
	return func(c *Config) {
		c.TridentNamespace = namespace
	}
}

// WithAssorterType sets the assorter type
func WithAssorterType(assorterType AssorterType) ConfigOption {
	return func(c *Config) {
		c.AssorterType = assorterType
	}
}

// WithAssorterPeriod sets the assorter period
func WithAssorterPeriod(period time.Duration) ConfigOption {
	return func(c *Config) {
		c.AssorterPeriod = period
	}
}

// WithReconciliationPeriod sets the reconciliation period
func WithReconciliationPeriod(period time.Duration) ConfigOption {
	return func(c *Config) {
		c.ReconciliationPeriod = period
	}
}

// Copy returns a deep copy of this configuration.
// This allows config to be safely reused across multiple schedulers.
func (c *Config) Copy() *Config {
	if c == nil {
		return nil
	}

	// Copy each field explicitly
	return &Config{
		ShutdownTimeout:      c.ShutdownTimeout,
		WorkQueueName:        c.WorkQueueName,
		MaxRetries:           c.MaxRetries,
		AssorterPeriod:       c.AssorterPeriod,
		ReconciliationPeriod: c.ReconciliationPeriod,
		TridentNamespace:     c.TridentNamespace,
		AssorterType:         c.AssorterType,
	}
}

// NewConfig creates a new Config with default values and applies the provided options.

func NewConfig(opts ...ConfigOption) *Config {
	// Start with defaults
	config := &Config{
		ShutdownTimeout:      DefaultShutdownTimeout,
		WorkQueueName:        DefaultWorkQueueName,
		MaxRetries:           DefaultMaxRetries,
		TridentNamespace:     DefaultTridentNamespace,
		AssorterType:         AssorterTypePeriodic,
		AssorterPeriod:       DefaultAssorterPeriod,
		ReconciliationPeriod: DefaultReconciliationPeriod,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return config
}
