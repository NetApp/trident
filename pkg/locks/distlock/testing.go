// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import "context"

var _ Locker = &ErrorAfterLocker{}

// ErrorAfterLocker is a test helper that executes the critical section and then returns a
// pre-configured error, simulating the case where the lock release fails (e.g. ErrLockDeleteFailed).
type ErrorAfterLocker struct {
	Err error
}

// NewErrorAfterLocker returns an ErrorAfterLocker that runs the critical section and then returns err.
func NewErrorAfterLocker(err error) *ErrorAfterLocker {
	return &ErrorAfterLocker{Err: err}
}

func (d *ErrorAfterLocker) WithLock(ctx context.Context, fn func(context.Context) error) error {
	if fnErr := fn(ctx); fnErr != nil {
		return fnErr
	}
	return d.Err
}
