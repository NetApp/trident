// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMountNFSPath(t *testing.T) {
	ctx := context.Background()
	result := mountNFSPath(ctx, "/export/path", "/mount/path", "test-options")
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestIsNFSShareMounted(t *testing.T) {
	ctx := context.Background()
	result, err := IsNFSShareMounted(ctx, "\\export\\path", "\\mount\\path")
	assert.False(t, result, "nfs share is mounted")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}

func TestMountDevice(t *testing.T) {
	ctx := context.Background()
	result := MountDevice(ctx, "\\device\\path", "\\mount\\path", "", false)
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestRemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	result := RemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestUmountAndRemoveTemporaryMountPoint(t *testing.T) {
	ctx := context.Background()
	result := UmountAndRemoveTemporaryMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestUmountAndRemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	result := UmountAndRemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestRemountDevice(t *testing.T) {
	ctx := context.Background()
	result := RemountDevice(ctx, "\\mount\\path", "")
	assert.Error(t, result, "no error")
	assert.True(t, IsUnsupportedError(result), "not UnsupportedError")
}

func TestGetHostMountInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetHostMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetSelfMountInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetSelfMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}

func TestIsCompatible_NFSProtocol(t *testing.T) {
	ctx := context.Background()
	err := IsCompatible(ctx, "nfs")
	assert.Error(t, err, "no error")
}

func TestIsCompatible_SMBProtocol(t *testing.T) {
	ctx := context.Background()
	err := IsCompatible(ctx, "smb")
	assert.NoError(t, err, "error")
}
