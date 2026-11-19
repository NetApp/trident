// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"errors"
	"fmt"
)

const (
	// fsck exit codes (bitmask — values can be OR'd together).
	ExitFsckNoErrors            = 0 // no filesystem errors
	ExitFsckCorrected           = 1 // filesystem errors corrected
	ExitFsckCorrectedNeedReboot = 2 // filesystem errors corrected; reboot required
	ExitFsckUncorrected         = 4 // uncorrectable errors; mounting risks data corruption

	// blkid exit codes.
	ExitBlkidSuccess = 0 // device has a recognizable signature
	ExitBlkidNoMatch = 2 // device has no recognizable signature
)

type DeviceNotFoundError struct {
	dev string
	err error
}

func (e *DeviceNotFoundError) Error() string {
	return fmt.Sprintf("device %s not found: %v", e.dev, e.err)
}
func (e *DeviceNotFoundError) Unwrap() error { return e.err }

func IsDeviceNotFoundError(err error) bool {
	var target *DeviceNotFoundError
	return errors.As(err, &target)
}

type DeviceFlushError struct {
	dev string
	err error
}

func (e *DeviceFlushError) Error() string {
	return fmt.Sprintf("failed to flush device %s: %v", e.dev, e.err)
}
func (e *DeviceFlushError) Unwrap() error { return e.err }

func IsDeviceFlushError(err error) bool {
	var target *DeviceFlushError
	return errors.As(err, &target)
}

// NoFilesystemSignatureError is returned by Type when blkid finds no
// recognizable signature on the device.
type NoFilesystemSignatureError struct {
	err error
}

func IsNoFilesystemSignatureError(err error) bool {
	var target *NoFilesystemSignatureError
	return errors.As(err, &target)
}

func (e *NoFilesystemSignatureError) Error() string { return "device has no filesystem signature" }
func (e *NoFilesystemSignatureError) Unwrap() error { return e.err }

// FsckFilesystemCorrectedError is returned by Repair when fsck reports
// correctable errors (exit codes 1 or 3).
type FsckFilesystemCorrectedError struct {
	err  error
	code int
}

func IsFsckCorrectedError(err error) bool {
	var target *FsckFilesystemCorrectedError
	return errors.As(err, &target)
}

func (e *FsckFilesystemCorrectedError) Error() string {
	return fmt.Sprintf("fsck reported correctable issues (exit code %d)", e.code)
}
func (e *FsckFilesystemCorrectedError) Unwrap() error { return e.err }

func isFsckCorrectedExitCode(code int) bool {
	correctedMask := ExitFsckCorrected | ExitFsckCorrectedNeedReboot
	return code&^correctedMask == 0 && code != ExitFsckNoErrors
}
