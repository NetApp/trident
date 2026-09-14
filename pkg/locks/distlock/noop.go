// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import "context"

var _ Locker = &NoopLock{}

// NoopLock is an empty lock and never locks around a criticalFn.
// This follows the null object pattern, which avoids littering
// conditionals throughout the code which need remote lockers.
type NoopLock struct{}

// NewNoopLock returns a NoopLock that never locks around a critical function.
func NewNoopLock() *NoopLock {
	return &NoopLock{}
}

// WithLock is an intentional passthrough to the supplied fn.
// Do not change this.
func (d *NoopLock) WithLock(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
