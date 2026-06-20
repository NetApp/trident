// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"errors"
	"testing"

	errors2 "github.com/netapp/trident/utils/errors"

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
	terr := errors2.TerminalReconciliationError("terminal")
	assert.False(t, IsInvalidTrackingFileError(terr))
}
