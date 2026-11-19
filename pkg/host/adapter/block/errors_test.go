// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDeviceNotFoundError(t *testing.T) {
	target := &DeviceNotFoundError{dev: "sda", err: fmt.Errorf("gone")}
	wrapped := fmt.Errorf("outer: %w", target)
	unrelated := fmt.Errorf("something else")

	assert.True(t, IsDeviceNotFoundError(target))
	assert.True(t, IsDeviceNotFoundError(wrapped))
	assert.False(t, IsDeviceNotFoundError(unrelated))
	assert.False(t, IsDeviceNotFoundError(nil))
}

func TestIsDeviceFlushError(t *testing.T) {
	target := &DeviceFlushError{dev: "/dev/sda", err: fmt.Errorf("ioctl")}
	wrapped := fmt.Errorf("outer: %w", target)
	unrelated := fmt.Errorf("something else")

	assert.True(t, IsDeviceFlushError(target))
	assert.True(t, IsDeviceFlushError(wrapped))
	assert.False(t, IsDeviceFlushError(unrelated))
	assert.False(t, IsDeviceFlushError(nil))
}

func TestDeviceNotFoundError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("root cause")
	err := &DeviceNotFoundError{dev: "sda", err: inner}
	assert.Equal(t, inner, err.Unwrap())
}

func TestDeviceFlushError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("ioctl failed")
	err := &DeviceFlushError{dev: "/dev/sda", err: inner}
	assert.Equal(t, inner, err.Unwrap())
}
