package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHostSystemInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetHostSystemInfo(ctx)
	assert.Nil(t, result, "host system information is populated")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}

func TestNFSActiveOnHost(t *testing.T) {
	ctx := context.Background()
	result, err := NFSActiveOnHost(ctx)
	assert.False(t, result, "nfs is present on the host")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetTargetFilePath(t *testing.T) {
	ctx := context.Background()
	result := GetTargetFilePath(ctx, "/host/path1", "/test/path")
	assert.NotEmpty(t, result, "path is empty")
}

func TestGetIPAddresses(t *testing.T) {
	ctx := context.Background()
	result, err := getIPAddresses(ctx)
	assert.Nil(t, result, "got ip address")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}
