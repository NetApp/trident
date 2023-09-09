// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestIsLikelyNotMountPoint(t *testing.T) {
	ctx := context.Background()
	result, err := IsLikelyNotMountPoint(ctx, "test-mountpoint")
	assert.False(t, result, "is likely a mountpoint")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestIsMounted(t *testing.T) {
	ctx := context.Background()
	result, err := IsMounted(ctx, "source-dev", "test-mountpoint", "test-mountoptions")
	assert.False(t, result, "device is mounted")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestMountNFSPath(t *testing.T) {
	ctx := context.Background()
	result := mountNFSPath(ctx, "/export/path", "/mount/path", "test-options")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestMountSMBPath(t *testing.T) {
	ctx := context.Background()
	result := mountSMBPath(ctx, "\\export\\path", "\\mount\\path", "test-user", "password")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestUmountSMBPath(t *testing.T) {
	ctx := context.Background()
	result := UmountSMBPath(ctx, "", "test-target")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestWindowsBindMount(t *testing.T) {
	ctx := context.Background()
	result := WindowsBindMount(ctx, "test-source", "test-target", []string{"test-val1", "test-val2"})
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestIsNFSShareMounted(t *testing.T) {
	ctx := context.Background()
	result, err := IsNFSShareMounted(ctx, "\\export\\path", "\\mount\\path")
	assert.False(t, result, "nfs share is mounted")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestMountDevice(t *testing.T) {
	ctx := context.Background()
	result := MountDevice(ctx, "\\device\\path", "\\mount\\path", "", false)
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestUmount(t *testing.T) {
	ctx := context.Background()
	result := Umount(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestRemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	result := RemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestUmountAndRemoveTemporaryMountPoint(t *testing.T) {
	ctx := context.Background()
	result := UmountAndRemoveTemporaryMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestUmountAndRemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	result := UmountAndRemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestRemountDevice(t *testing.T) {
	ctx := context.Background()
	result := RemountDevice(ctx, "\\mount\\path", "")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestGetHostMountInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetHostMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetSelfMountInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetSelfMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestIsCompatible(t *testing.T) {
	ctx := context.Background()
	err := IsCompatible(ctx, "nfs")
	assert.Error(t, err, "no error")
}
