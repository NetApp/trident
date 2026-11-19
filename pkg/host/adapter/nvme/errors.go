// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"errors"
	"fmt"
)

// Error is a runtime currency for nvme adapter execution errors on the host.
// It contains the exit code and output of the nvme command.
// It is a leaf error type that should not be unwrapped.
type Error struct {
	bin  string
	cmd  string
	code int
	out  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %s exit code %d: output %s", e.bin, e.cmd, e.code, e.out)
}
func (e *Error) ExitCode() int  { return e.code }
func (e *Error) Output() string { return e.out }
func (e *Error) Binary() string { return e.bin }
func (e *Error) Cmd() string    { return e.cmd }

// SubsystemNotFoundError means the host has no paths to the subsystem: either no subsystem with that
// NQN is connected, or it is connected with no controllers, or the sysfs subsystem directory does
// not exist because no NVMe fabrics transport has been used on the host.
type SubsystemNotFoundError struct {
	nqn string
}

func (e *SubsystemNotFoundError) Error() string {
	return fmt.Sprintf("NVMe subsystem %s not found", e.nqn)
}

func IsSubsystemNotFoundError(err error) bool {
	var target *SubsystemNotFoundError
	return errors.As(err, &target)
}
