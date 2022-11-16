// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDeviceFilePath(t *testing.T) {
	ctx := context.Background()

	result, err := GetDeviceFilePath(ctx, "/test-path", "")
	assert.Equal(t, result, "/test-path", "got device file path")
	assert.NoError(t, err, "error")
}

func TestGetUnmountPath(t *testing.T) {
	ctx := context.Background()

	result, err := GetUnmountPath(ctx, &VolumeTrackingInfo{})
	assert.Equal(t, result, "", "got unmount path")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}
