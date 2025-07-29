// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalidTrackingFileError(t *testing.T) {
	msg := "invalid tracking file"
	err := InvalidTrackingFileError(msg)

	// Check type and message
	typed, ok := err.(*invalidTrackingFileError)
	assert.True(t, ok)
	assert.Equal(t, msg, typed.Error())

	// IsInvalidTrackingFileError should be true for this error
	assert.True(t, IsInvalidTrackingFileError(err))

	// Should be false for nil
	assert.False(t, IsInvalidTrackingFileError(nil))

	// Should be false for unrelated error
	assert.False(t, IsInvalidTrackingFileError(errors.New("other error")))

	// Should be false for terminalReconciliationError
	terr := TerminalReconciliationError("terminal")
	assert.False(t, IsInvalidTrackingFileError(terr))
}

func TestTerminalReconciliationError(t *testing.T) {
	msg := "terminal reconciliation error"
	err := TerminalReconciliationError(msg)

	// Check type and message
	typed, ok := err.(*terminalReconciliationError)
	assert.True(t, ok)
	assert.Equal(t, msg, typed.Error())

	// IsTerminalReconciliationError should be true for this error
	assert.True(t, IsTerminalReconciliationError(err))

	// Should be false for nil
	assert.False(t, IsTerminalReconciliationError(nil))

	// Should be false for unrelated error
	assert.False(t, IsTerminalReconciliationError(errors.New("other error")))

	// Should be false for invalidTrackingFileError
	itfErr := InvalidTrackingFileError("invalid")
	assert.False(t, IsTerminalReconciliationError(itfErr))
}
