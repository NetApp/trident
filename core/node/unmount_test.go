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
	"github.com/netapp/trident/utils/models"
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

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), volume).Return(nil, errors.New("read failed"))
	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, errors.New("stat failed"))

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
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat failed")
	case <-time.After(2 * time.Second):
		t.Fatal("Unmount did not proceed after volume lock was released")
	}
}

func TestUnmount_DelegatesToUnmountGeneric(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(sampleTrackingInfo(ISCSI), nil)
	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.Unmount(context.Background(), "vol1", UnmountRequest{TargetPath: "/target"})
	assert.NoError(t, err)
}

func TestUnmount_NoTrackingInfo_FallsBackToUnmountGeneric(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(nil, errors.NotFoundError("not found"))
	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(errors.NotFoundError("not found"))

	err := core.Unmount(context.Background(), "vol1", UnmountRequest{TargetPath: "/target"})
	assert.NoError(t, err)
}

func TestUnmount_NVMe_UnmountsThroughDriver(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(sampleTrackingInfo(NVMe), nil)
	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, nil)
	mocks.NVMeDriver.EXPECT().Unmount(gomock.Any(), "vol1", &models.VolumeUnmountSpec{MountPoint: "/target"}).Return(nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)
	// The generic mount checks must not be used for NVMe.

	err := core.Unmount(context.Background(), "vol1", UnmountRequest{TargetPath: "/target"})
	assert.NoError(t, err)
}

func TestUnmountVolume_DispatchesByProtocol(t *testing.T) {
	t.Run("NVMe uses the driver", func(t *testing.T) {
		core, mocks := newTestCore(t)
		mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
		mocks.NVMeDriver.EXPECT().Unmount(gomock.Any(), "vol1", gomock.Any()).Return(nil)
		mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
		mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

		assert.NoError(t, core.unmountVolume(context.Background(), "vol1", "/target", NVMe))
	})
	t.Run("other protocols use the generic path", func(t *testing.T) {
		core, mocks := newTestCore(t)
		mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
		mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
		mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
		mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

		assert.NoError(t, core.unmountVolume(context.Background(), "vol1", "/target", FCP))
	})
}

func TestUnmountNVMeVolume_TargetPathGone_ReturnsNilSuccess(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrNotExist)
	// The driver, DeleteResourceAtPath and RemovePublishedPath must not be reached.

	err := core.unmountVolume(context.Background(), "vol1", "/target", NVMe)
	assert.NoError(t, err)
}

func TestUnmountNVMeVolume_IsLikelyDirOtherError_Wrapped(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, errors.New("permission denied"))

	err := core.unmountVolume(context.Background(), "vol1", "/target", NVMe)
	require.Error(t, err)
	assert.True(t, errors.IsInternalError(err))
	assert.Contains(t, err.Error(), "could not check if the target path")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestUnmountNVMeVolume_DriverError_ReturnedWithoutDeletingTargetOrPublishedPath(t *testing.T) {
	core, mocks := newTestCore(t)
	unmountErr := errors.InvalidInputError("unable to unmount volume; device busy")

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.NVMeDriver.EXPECT().Unmount(gomock.Any(), "vol1", gomock.Any()).Return(unmountErr)
	// DeleteResourceAtPath/RemovePublishedPath must not be reached.

	err := core.unmountVolume(context.Background(), "vol1", "/target", NVMe)
	assert.Equal(t, unmountErr, err)
}

func TestUnmountNVMeVolume_DeleteResourceAtPathError_IsBestEffort(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
	mocks.NVMeDriver.EXPECT().Unmount(gomock.Any(), "vol1", gomock.Any()).Return(nil)
	mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(errors.New("resource still busy"))
	mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.unmountVolume(context.Background(), "vol1", "/target", NVMe)
	assert.NoError(t, err)
}

func TestUnmountNVMeVolume_RemovePublishedPathErrors(t *testing.T) {
	t.Run("not found is swallowed", func(t *testing.T) {
		core, mocks := newTestCore(t)
		mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
		mocks.NVMeDriver.EXPECT().Unmount(gomock.Any(), "vol1", gomock.Any()).Return(nil)
		mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
		mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(errors.NotFoundError("not found"))

		assert.NoError(t, core.unmountVolume(context.Background(), "vol1", "/target", NVMe))
	})
	t.Run("other error is an internal error", func(t *testing.T) {
		core, mocks := newTestCore(t)
		mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(true, nil)
		mocks.NVMeDriver.EXPECT().Unmount(gomock.Any(), "vol1", gomock.Any()).Return(nil)
		mocks.OsUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), "/target").Return(nil)
		mocks.NodeHelper.EXPECT().RemovePublishedPath(gomock.Any(), "vol1", "/target").Return(errors.New("disk write error"))

		err := core.unmountVolume(context.Background(), "vol1", "/target", NVMe)
		require.Error(t, err)
		assert.True(t, errors.IsInternalError(err))
		assert.Contains(t, err.Error(), "could not remove published path")
	})
}

func TestUnmountGeneric_TargetPathGone_ReturnsNilSuccess(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, os.ErrNotExist)

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	assert.NoError(t, err)
}

func TestUnmountGeneric_IsLikelyDirOtherError_Wrapped(t *testing.T) {
	core, mocks := newTestCore(t)

	mocks.OsUtils.EXPECT().IsLikelyDir("/target").Return(false, errors.New("permission denied"))

	err := core.unmountGeneric(context.Background(), "vol1", "/target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if the target path")
	assert.Contains(t, err.Error(), "permission denied")
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
