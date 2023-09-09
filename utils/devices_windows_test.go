// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestFlushOneDevice(t *testing.T) {
	ctx := context.Background()

	result := flushOneDevice(ctx, "/test/path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestGetISCSIDiskSize(t *testing.T) {
	ctx := context.Background()

	result, err := getISCSIDiskSize(ctx, "/test/path")
	assert.Equal(t, result, int64(0), "received disk size")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
