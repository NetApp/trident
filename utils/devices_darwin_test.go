package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlushOneDevice(t *testing.T) {
	ctx := context.Background()

	result := flushOneDevice(ctx, "/test/path")
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestGetISCSIDiskSize(t *testing.T) {
	ctx := context.Background()

	result, err := getISCSIDiskSize(ctx, "/test/path")
	assert.Equal(t, result, int64(0), "received disk size")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}
