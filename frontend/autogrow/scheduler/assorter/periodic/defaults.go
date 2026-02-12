// Copyright 2026 NetApp, Inc. All Rights Reserved.

package periodic

import (
	"time"
)

const (
	// DefaultPeriod is the default interval for periodic scheduling.
	DefaultPeriod = 1 * time.Minute
)

// DefaultConfig returns a new Config with default values.
// Note: PublishFunc must be set by the caller - it cannot have a default value.
func DefaultConfig() *Config {
	return NewConfig() // Uses the NewConfig function which applies defaults
}
