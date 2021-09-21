// Copyright 2021 NetApp, Inc. All Rights Reserved.

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedCapacityRangeError(t *testing.T) {
	// test setup
	err := fmt.Errorf("a generic error")
	unsupportedCapacityRangeErr := UnsupportedCapacityRangeError(fmt.Errorf(
		"error wrapped within UnsupportedCapacityRange"))
	wrappedError := fmt.Errorf("wrapping unsupportedCapacityRange; %w", unsupportedCapacityRangeErr)

	// test exec
	t.Run("should not identify an UnsupportedCapacityRangeError", func (t *testing.T) {
		ok, _ := HasUnsupportedCapacityRangeError(err)
		assert.Equal(t, false, ok)
	})

	t.Run("should identify an UnsupportedCapacityRangeError", func (t *testing.T) {
		ok, _ := HasUnsupportedCapacityRangeError(unsupportedCapacityRangeErr)
		assert.Equal(t, true, ok)
	})

	t.Run("should identify an UnsupportedCapacityRangeError within a wrapped error", func (t *testing.T) {
		ok, _ := HasUnsupportedCapacityRangeError(wrappedError)
		assert.Equal(t, true, ok)
	})
}
