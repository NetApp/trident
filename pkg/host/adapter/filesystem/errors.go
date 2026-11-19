// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"errors"
)

type NotMountedError struct {
	err error
}

func (e *NotMountedError) Error() string {
	return "target is not mounted"
}
func (e *NotMountedError) Unwrap() error { return e.err }

func IsNotMountedError(err error) bool {
	var target *NotMountedError
	return errors.As(err, &target)
}

type TargetBusyError struct {
	err error
}

func (e *TargetBusyError) Error() string {
	return "target is busy"
}
func (e *TargetBusyError) Unwrap() error { return e.err }

func IsTargetBusyError(err error) bool {
	var target *TargetBusyError
	return errors.As(err, &target)
}
