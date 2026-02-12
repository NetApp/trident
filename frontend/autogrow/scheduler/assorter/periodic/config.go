// Copyright 2026 NetApp, Inc. All Rights Reserved.

package periodic

import (
	"time"

	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types"
)

// Compile-time check to ensure Config implements types.Config.
var _ types.Config = (*Config)(nil)

// Config holds the configuration for the periodic assorter.
type Config struct {
	period      time.Duration
	publishFunc types.PublishFunc
}

// ConfigOption is a functional option for configuring the periodic assorter.
type ConfigOption func(*Config)

// WithPeriod sets the polling period for the assorter.
func WithPeriod(period time.Duration) ConfigOption {
	return func(c *Config) {
		c.period = period
	}
}

// WithPublishFunc sets the publish callback function.
func WithPublishFunc(publishFunc types.PublishFunc) ConfigOption {
	return func(c *Config) {
		c.publishFunc = publishFunc
	}
}

// NewConfig creates a new Config with default values and applies the provided options.
// Default values:
// - Period: DefaultPeriod (1 minute)
// - PublishFunc: nil (must be provided via WithPublishFunc)
func NewConfig(opts ...ConfigOption) *Config {
	// Start with defaults
	config := &Config{
		period: DefaultPeriod,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return config
}

// AssorterConfig is a marker method to satisfy the types.Config interface.
func (c *Config) AssorterConfig() {}

// Copy returns a deep copy of this configuration.
func (c *Config) Copy() types.Config {
	if c == nil {
		return nil
	}
	return &Config{
		period:      c.period,
		publishFunc: c.publishFunc, // Function pointers are safe to copy
	}
}
