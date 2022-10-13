package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetIPAddresses(t *testing.T) {
	ctx := context.Background()
	result, err := getIPAddresses(ctx)
	assert.Nil(t, result, "got ip address")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetHostSystemInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetHostSystemInfo(ctx)
	assert.Nil(t, result, "got host system info")
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

func TestIsLikelyDir_NotPresent(t *testing.T) {
	result, err := IsLikelyDir("\\usr")
	assert.False(t, result, "directory exists")
	assert.Error(t, err, "no error")
}

func TestIsLikelyDir_Present(t *testing.T) {
	result, err := IsLikelyDir("/Users")
	assert.True(t, result, "directory doesn't exists")
	assert.NoError(t, err, "error occurred")
}

func TestGetTargetFilePath(t *testing.T) {
	ctx := context.Background()
	result := GetTargetFilePath(ctx, "/host/path1", "/test/path")
	assert.Empty(t, result, "path is not empty")
}

func TestSMBActiveOnHost(t *testing.T) {
	ctx := context.Background()
	result, err := SMBActiveOnHost(ctx)
	assert.False(t, result, "smb is active on the host")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}
