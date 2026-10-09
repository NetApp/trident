// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/utils/errors"
)

func TestUnmount_EmptyVolume_ReturnsError(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Unmount(context.Background(), "", UnmountRequest{TargetPath: "/target"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume is empty")
}

func TestUnmount_EmptyTargetPath_ReturnsError(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Unmount(context.Background(), "vol1", UnmountRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target path is empty")
}

func TestUnmount_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.Unmount(context.Background(), "vol1", UnmountRequest{TargetPath: "/target"})
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestUnmount_AcquiresVolumeLock_SerializesConcurrentCalls(t *testing.T) {
	core, mocks := newTestCore(t)
	volume := "vol1"

	core.volumeLocks.Lock(volume)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrNotExist)

	done := make(chan error, 1)
	go func() { done <- core.Unmount(context.Background(), volume, UnmountRequest{TargetPath: "/target"}) }()

	select {
	case <-done:
		t.Fatal("Unmount returned before volume lock was released")
	case <-time.After(50 * time.Millisecond):
	}

	core.volumeLocks.Unlock(volume)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Unmount did not proceed after volume lock was released")
	}
}

func TestUnmount_DelegatesToUnmountGeneric(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.Unmount(context.Background(), "vol1", UnmountRequest{TargetPath: "/target"})
	assert.NoError(t, err)
}

func TestUnmountGeneric_TargetPathGone_ReturnsNilSuccess(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrNotExist)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_StatError_Mounted_FallsBackToMountTableAndUnmounts(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrPermission)
	mocks.Mount.EXPECT().IsMounted(gomock.Any(), "", "/target", "").Return(true, nil)
	// IsLikelyNotMountPoint must not be called: it would stat the path again.
	mocks.Mount.EXPECT().Umount(gomock.Any(), "/target").Return(nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_StatError_NotMounted_SkipsUmount(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrPermission)
	mocks.Mount.EXPECT().IsMounted(gomock.Any(), "", "/target", "").Return(false, nil)
	// IsLikelyNotMountPoint must not be called: it would stat the path again.
	// No Umount call expected.
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_StatError_MountTableError_Wrapped(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrPermission)
	// IsLikelyNotMountPoint must not be called: it would stat the path again.
	mocks.Mount.EXPECT().IsMounted(gomock.Any(), "", "/target", "").Return(false, errors.New("boom"))

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to check if targetPath")
	assert.Contains(t, err.Error(), "boom")
}

func TestUnmountGeneric_IsDir_UsesIsLikelyNotMountPoint(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)
	// IsMounted must not be called when the target is a directory.

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_NotDir_UsesIsMounted(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, nil)
	mocks.Mount.EXPECT().IsMounted(gomock.Any(), "", "/target", "").Return(false, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)
	// IsLikelyNotMountPoint must not be called when the target is not a directory.

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_IsDir_MountCheckErrorIsNotExist_ReturnsTargetPathNotFound(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, os.ErrNotExist)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target path not found")
}

func TestUnmountGeneric_IsDir_MountCheckOtherError_Wrapped(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, errors.New("boom"))

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to check if targetPath")
	assert.Contains(t, err.Error(), "boom")
}

func TestUnmountGeneric_NotDir_IsMountedError_Wrapped(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, nil)
	mocks.Mount.EXPECT().IsMounted(gomock.Any(), "", "/target", "").Return(false, errors.New("boom"))

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to check if targetPath")
	assert.Contains(t, err.Error(), "boom")
}

func TestUnmountGeneric_NotMountPoint_SkipsUmount(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	// No Umount call expected.
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_IsMountPoint_CallsUmount(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, nil)
	mocks.Mount.EXPECT().Umount(gomock.Any(), "/target").Return(nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_UmountError_WrappedAndPropagated(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, nil)
	mocks.Mount.EXPECT().Umount(gomock.Any(), "/target").Return(errors.New("device busy"))
	// DeleteResourceAtPath/RemovePublishedPath must not be reached.

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to unmount volume")
	assert.Contains(t, err.Error(), "device busy")
}

func TestUnmountGeneric_DeleteResourceAtPathError_IsBestEffortAndDoesNotFailUnmount(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(errors.New("resource still busy"))
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err, "DeleteResourceAtPath errors must be logged and swallowed, not returned")
}

func TestUnmountGeneric_RemovePublishedPathNotFound_IsSwallowed(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(errors.NotFoundError("not found"))

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_RemovePublishedPathOtherError_WrappedAndReturned(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(errors.New("disk write error"))

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not remove published path")
	assert.Contains(t, err.Error(), "disk write error")
}
