// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestNewOsClient(t *testing.T) {
	client, err := newOsSpecificClient()
	assert.NotNil(t, client)
	assert.NoError(t, err)
}

func TestDarwinClient_IsLikelyNotMountPoint(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.IsLikelyNotMountPoint(ctx, "test-mountpoint")
	assert.False(t, result, "is likely a mountpoint")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestDarwinClient_IsMounted(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.IsMounted(ctx, "source-dev", "test-mountpoint", "test-mountoptions")
	assert.False(t, result, "device is mounted")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestDarwinClient_MountNFSPath(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.MountNFSPath(ctx, "/export/path", "/mount/path", "test-options")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_MountSMBPath(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.MountSMBPath(ctx, "\\export\\path", "\\mount\\path", "test-user", "password")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_UmountSMBPath(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.UmountSMBPath(ctx, "", "test-target")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_WindowsBindMount(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.WindowsBindMount(ctx, "test-source", "test-target", []string{"test-val1", "test-val2"})
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_IsNFSShareMounted(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.IsNFSShareMounted(ctx, "\\export\\path", "\\mount\\path")
	assert.False(t, result, "nfs share is mounted")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestDarwinClient_MountDevice(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.MountDevice(ctx, "\\device\\path", "\\mount\\path", "", false)
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_Umount(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.Umount(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_RemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.RemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_UmountAndRemoveTemporaryMountPoint(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.UmountAndRemoveTemporaryMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_UmountAndRemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.UmountAndRemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_RemountDevice(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.RemountDevice(ctx, "\\mount\\path", "")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestDarwinClient_GetHostMountInfo(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.GetHostMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestDarwinClient_GetSelfMountInfo(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.GetSelfMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestDarwinClient_IsCompatible(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	err = client.IsCompatible(ctx, "nfs")
	assert.Error(t, err, "no error")
}

func TestDarwinClient_PVMountpointMappings(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.PVMountpointMappings(ctx)
	assert.NotNil(t, result, "did not get mount point mappings")
	assert.Error(t, err, "no error")
}

func TestDarwinClient_ListProcMounts(t *testing.T) {
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.ListProcMounts("")
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestDarwinClient_ListProcMountinfo(t *testing.T) {
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result, err := client.ListProcMountinfo()
	assert.Nil(t, result)
	assert.Error(t, err)
}
