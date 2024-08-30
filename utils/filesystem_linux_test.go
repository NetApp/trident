// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestGetDeviceFilePath(t *testing.T) {
	ctx := context.Background()

	result, err := GetDeviceFilePath(ctx, "/test-path", "")
	assert.Equal(t, result, "/test-path", "got device file path")
	assert.NoError(t, err, "error")
}

func TestGetUnmountPath(t *testing.T) {
	ctx := context.Background()

	result, err := GetUnmountPath(ctx, &models.VolumeTrackingInfo{})
	assert.Equal(t, result, "", "got unmount path")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
