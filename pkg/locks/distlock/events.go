// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import (
	"context"

	coordinationv1 "k8s.io/api/coordination/v1"
)

// Event type strings mirror Kubernetes EventTypeNormal / EventTypeWarning.
const (
	EventTypeNormal  = "Normal" // retained for EventRecorder API completeness
	EventTypeWarning = "Warning"
)

// Stable event reasons for lease lifecycle.
// Happy-path acquire/re-entry/release and acquire conflicts are not evented;
// only sticky-fence retain and release-failed emit Warnings.
const (
	ReasonLeaseRetained      = "LeaseRetained"
	ReasonLeaseReleaseFailed = "LeaseReleaseFailed"
)

// EventRecorder records Kubernetes (or CO) events for lease lifecycle transitions.
// Implementations must post onto the given Lease object (or a best-effort reference
// when the Lease is gone). LeaseLock treats a nil recorder as a no-op.
type EventRecorder interface {
	Event(ctx context.Context, lease *coordinationv1.Lease, eventType, reason, message string)
}

// WithEventRecorder attaches an optional event sink to a LeaseLock.
func WithEventRecorder(recorder EventRecorder) LeaseLockOpts {
	return func(l *LeaseLock) {
		l.eventRecorder = recorder
	}
}
