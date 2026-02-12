// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"time"
)

const (
	// DefaultShutdownTimeout Default shutdown timeout
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultWorkQueueName Default work queue name
	DefaultWorkQueueName = "scheduler-workqueue"

	// DefaultMaxRetries Default max retries before discarding work item
	DefaultMaxRetries = 3

	// DefaultTridentNamespace Default trident namespace
	DefaultTridentNamespace = ""

	// DefaultAssorterPeriod Default assorter period (1 minute)
	// Used when creating a periodic assorter if AssorterPeriod is not specified
	DefaultAssorterPeriod = 1 * time.Minute

	// DefaultReconciliationPeriod Default reconciliation period (5 minutes)
	// Periodic reconciliation catches any cache drift from missed events
	DefaultReconciliationPeriod = 5 * time.Minute
)

// DefaultConfig returns the default configuration for the Scheduler layer.
// It uses:
// - MaxRetries: 3
// - 30 second shutdown timeout
// - Default work queue name
// - AssorterType: Periodic
// - AssorterPeriod: 1 minute
// - ReconciliationPeriod: 5 minutes
// - TridentNamespace: ""
func DefaultConfig() *Config {
	return NewConfig()
}
