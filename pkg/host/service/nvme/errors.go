// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import "errors"

// ErrTransportNotLoaded is returned by PreChecks when nvme-cli works but the NVMe/TCP transport is not loaded:
// NVMe is not active on the host, which is not a discovery failure.
var ErrTransportNotLoaded = errors.New("NVMe driver is not loaded on the host")

// FlushOptions selects how EnsureFlush treats the host.
type FlushOptions struct {
	// Force skips the flush.
	Force bool
	// IgnoreErrors logs a flush that cannot run or fails, instead of returning it.
	IgnoreErrors bool
}

// FlushError means the device could not be flushed: the subsystem has no live path, or the flush failed.
type FlushError struct {
	msg string
	err error // The adapter error, nil when the flush was not attempted.
}

func newFlushError(msg string, err error) *FlushError { return &FlushError{msg: msg, err: err} }

// NewFlushError returns a FlushError for callers that fake the service, such as the core's tests.
func NewFlushError(msg string, err error) *FlushError { return newFlushError(msg, err) }

func (e *FlushError) Error() string { return e.msg }
func (e *FlushError) Unwrap() error { return e.err }

func IsFlushError(err error) bool {
	var target *FlushError
	return errors.As(err, &target)
}
