// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cryptsetup

import (
	"errors"
	"fmt"
)

// Error is a runtime currency for cryptsetup adapter execution errors on the host.
// It contains the exit code and output of the cryptsetup command.
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

type IncorrectPassphraseError struct {
	err error
}

func (e *IncorrectPassphraseError) Error() string {
	return "incorrect LUKS passphrase"
}
func (e *IncorrectPassphraseError) Unwrap() error { return e.err }

func IsIncorrectPassphraseError(err error) bool {
	var target *IncorrectPassphraseError
	return errors.As(err, &target)
}

type OOMError struct {
	err error
}

func (e *OOMError) Error() string {
	return "cryptsetup oom failure"
}
func (e *OOMError) Unwrap() error { return e.err }

func IsOOMError(err error) bool {
	var target *OOMError
	return errors.As(err, &target)
}

type DeviceNotActiveError struct {
	err error
}

func (e *DeviceNotActiveError) Error() string {
	return "LUKS mapper is not active"
}
func (e *DeviceNotActiveError) Unwrap() error { return e.err }

func IsDeviceNotActiveError(err error) bool {
	var target *DeviceNotActiveError
	return errors.As(err, &target)
}

type BusyDeviceError struct {
	err error
}

func (e *BusyDeviceError) Error() string { return "LUKS device is busy" }
func (e *BusyDeviceError) Unwrap() error { return e.err }

func IsBusyDeviceError(err error) bool {
	var target *BusyDeviceError
	return errors.As(err, &target)
}

type BadParameterError struct {
	err error
}

func (e *BadParameterError) Error() string { return "cryptsetup invoked with bad parameters" }
func (e *BadParameterError) Unwrap() error { return e.err }

func IsBadParameterError(err error) bool {
	var target *BadParameterError
	return errors.As(err, &target)
}

type NotLUKSDeviceError struct {
	err error
}

func (e *NotLUKSDeviceError) Error() string {
	return "device is not LUKS formatted"
}

func (e *NotLUKSDeviceError) Unwrap() error { return e.err }

func IsNotLUKSDeviceError(err error) bool {
	var target *NotLUKSDeviceError
	return errors.As(err, &target)
}
