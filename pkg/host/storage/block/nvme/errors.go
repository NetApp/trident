// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import "errors"

// LUKSPassphraseError reports that Attach could not verify or rotate the LUKS passphrase. It is the last attach
// step, so the device stack is already set up and formatted (and mounted, if a mount point was given), and the
// caller must treat the volume as in use on this node.
type LUKSPassphraseError struct {
	err error
}

// NewLUKSPassphraseError wraps err as a *LUKSPassphraseError, for callers that fake the Driver, such as the
// core's tests.
func NewLUKSPassphraseError(err error) *LUKSPassphraseError { return &LUKSPassphraseError{err: err} }

func (e *LUKSPassphraseError) Error() string { return e.err.Error() }
func (e *LUKSPassphraseError) Unwrap() error { return e.err }

// IsLUKSPassphraseError reports whether err, or an error it wraps, is a *LUKSPassphraseError.
func IsLUKSPassphraseError(err error) bool {
	var target *LUKSPassphraseError
	return errors.As(err, &target)
}
