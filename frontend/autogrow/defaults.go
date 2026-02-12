// Copyright 2026 NetApp, Inc. All Rights Reserved.

package autogrow

import (
	"runtime"
)

const (
	// DefaultWorkerPoolPreAlloc determines if goroutines are pre-allocated in the orchestrator's worker pool
	DefaultWorkerPoolPreAlloc = true

	// DefaultWorkerPoolNonBlocking determines if pool submission should block when full in the orchestrator's worker pool
	DefaultWorkerPoolNonBlocking = false
)

var (
	// DefaultWorkerPoolSize is the default number of workers in the orchestrator's worker pool
	// Uses runtime.NumCPU() as the orchestrator's worker pool handles eventbus operations
	DefaultWorkerPoolSize = runtime.NumCPU()
)
