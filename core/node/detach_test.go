// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

// withMockedFcpUtils swaps the package-level fcpUtils var for a mock for the duration of a test.
func withMockedFcpUtils(t *testing.T) *mock_fcp.MockFcpReconcileUtils {
	origFcpUtils := fcpUtils
	mock := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
	fcpUtils = mock
	t.Cleanup(func() { fcpUtils = origFcpUtils })
	return mock
}

// withMockedIscsiUtils swaps the package-level iscsiUtils var for a mock for the duration of a test.
func withMockedIscsiUtils(t *testing.T) *mock_iscsi.MockIscsiReconcileUtils {
	origIscsiUtils := iscsiUtils
	mock := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
	iscsiUtils = mock
	t.Cleanup(func() { iscsiUtils = origIscsiUtils })
	return mock
}

// clearNVMeFlushRetryMap ensures NVMeNamespacesFlushRetry (a package-level global) starts and
// ends each test clean, since it's shared across the whole test binary.
func clearNVMeFlushRetryMap(t *testing.T) {
	t.Cleanup(func() {
		for k := range NVMeNamespacesFlushRetry {
			delete(NVMeNamespacesFlushRetry, k)
		}
	})
}

func TestDetach_EmptyVolume(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Detach(context.Background(), "", DetachRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume is empty")
}

func TestDetach_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.Detach(context.Background(), "test-volume", DetachRequest{})
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestDetach_VolumeLockSerializesSameVolume(t *testing.T) {
	core, mocks := newTestCore(t)

	started := make(chan struct{})
	release := make(chan struct{})
	first := true

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "same-volume").Times(2).DoAndReturn(
		func(context.Context, string) (*models.VolumeTrackingInfo, error) {
			if first {
				first = false
				close(started)
				<-release
			}
			return nil, errors.NotFoundError("not found")
		},
	)

	done := make(chan error, 2)
	go func() { done <- core.Detach(context.Background(), "same-volume", DetachRequest{}) }()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first Detach never started")
	}

	go func() { done <- core.Detach(context.Background(), "same-volume", DetachRequest{}) }()

	select {
	case <-done:
		t.Fatal("second Detach completed before the first released the volume lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Detach calls did not both complete")
		}
	}
}

func TestDetachInternal_TrackingInfoNotFound_ReturnsNil(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-volume").
		Return(nil, errors.NotFoundError("tracking file gone"))

	err := core.detach(context.Background(), "test-volume", false)
	assert.NoError(t, err)
}

func TestDetachInternal_InvalidJSONTrackingFile_ReturnsError(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-volume").
		Return(nil, errors.InvalidJSONError("bad json"))

	err := core.detach(context.Background(), "test-volume", false)
	require.Error(t, err)
	assert.True(t, errors.IsInternalError(err))
}

func TestDetachInternal_OtherReadError_ReturnsError(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-volume").
		Return(nil, errors.New("disk read error"))

	err := core.detach(context.Background(), "test-volume", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk read error")
}

func TestDetachInternal_EmptyVolume(t *testing.T) {
	core, _ := newTestCore(t)
	err := core.detach(context.Background(), "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume is empty")
}

func TestDetachInternal_UnknownProtocol_ReturnsError(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.StorageProtocol = "bogus"
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-volume").Return(ti, nil)

	err := core.detach(context.Background(), "test-volume", false)
	require.Error(t, err)
	assert.True(t, errors.IsPreconditionError(err))
}

func TestDetachInternal_DispatchesNFS(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(NFS)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-volume").Return(ti, nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detach(context.Background(), "test-volume", false)
	assert.NoError(t, err)
}

func TestDetachNFSVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachNFSVolume(context.Background(), "test-volume")
	assert.NoError(t, err)
}

func TestDetachNFSVolume_DeleteError(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").
		Return(errors.New("delete failed"))

	err := core.detachNFSVolume(context.Background(), "test-volume")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestDetachSMBVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(SMB)

	mocks.Filesystem.EXPECT().GetUnmountPath(gomock.Any(), ti).Return(`\\server\share`, nil)
	mocks.Mount.EXPECT().UmountSMBPath(gomock.Any(), `\\server\share`, ti.GlobalMount).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachSMBVolume(context.Background(), "test-volume", ti)
	assert.NoError(t, err)
}

func TestDetachSMBVolume_GetUnmountPathError_SkipsUnmountAndDelete(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(SMB)

	mocks.Filesystem.EXPECT().GetUnmountPath(gomock.Any(), ti).Return("", errors.New("bad mapping"))

	err := core.detachSMBVolume(context.Background(), "test-volume", ti)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad mapping")
}

func TestDetachSMBVolume_UnmountErrorStillDeletesTrackingInfo(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(SMB)

	mocks.Filesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return(`\\server\share`, nil)
	mocks.Mount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("unmount failed"))
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachSMBVolume(context.Background(), "test-volume", ti)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmount failed")
}

func TestDetachSMBVolume_DeleteTrackingInfoErrorTakesPrecedence(t *testing.T) {
	core, mocks := newTestCore(t)
	ti := sampleTrackingInfo(SMB)

	mocks.Filesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return(`\\server\share`, nil)
	mocks.Mount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("unmount failed"))
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).
		Return(errors.New("delete failed"))

	err := core.detachSMBVolume(context.Background(), "test-volume", ti)
	require.Error(t, err)
	// detachSMBVolume returns the DeleteTrackingInfo error, masking the unmount error.
	assert.Contains(t, err.Error(), "delete failed")
	assert.NotContains(t, err.Error(), "unmount failed")
}

func TestDetachFCPVolume_GetDevicesForLUNError(t *testing.T) {
	core, _ := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), publishInfo.FCTargetWWNN).
		Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(int(publishInfo.FCPLunNumber), gomock.Any()).
		Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return(nil, errors.New("sysfs read failed"))

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get devices for LUN")
}

func TestDetachFCPVolume_NoDevicesFound_CleansUpTrackingOnly(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(
		gomock.Any(), publishInfo.DevicePath, uint64(removeMultipathDeviceMappingRetries), removeMultipathDeviceMappingRetryDelay,
	).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachFCPVolume_NoDevicesFound_LUKSTeardownNonLegacyPath(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.LUKSEncryption = "true"
	publishInfo.DevicePath = "/dev/mapper/mpatha" // no "luks" substring -> non-legacy branch

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil)
	mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice(publishInfo.DevicePath).
		Return("/dev/mapper/luks-uuid", nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(
		gomock.Any(), publishInfo.DevicePath, gomock.Any(), gomock.Any(),
	).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachFCPVolume_GetDeviceInfoError(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), int(publishInfo.FCPLunNumber),
		publishInfo.FCTargetWWNN, false).Return(nil, errors.New("scan failed"))

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get device info")
}

func TestDetachFCPVolume_DeviceInfoNil_ReturnsNil(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil, nil)

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachFCPVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.FCP.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, publishInfo, nil, false, false).
		Return("/dev/mapper/mpatha", nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), publishInfo.GlobalMount).Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(
		gomock.Any(), "/dev/mapper/mpatha", uint64(removeMultipathDeviceMappingRetries), removeMultipathDeviceMappingRetryDelay,
	).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachFCPVolume_PrepareDeviceForRemovalError_ReturnsErr(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.FCP.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, publishInfo, nil, false, false).
		Return("", errors.New("device busy"))

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device busy")
}

func TestDetachFCPVolume_SameLunNumberError_RetriesWithAllTrackingFiles(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	gomock.InOrder(
		mocks.FCP.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, publishInfo, nil, false, false).
			Return("", errors.FCPSameLunNumberError("same LUN number on host")),
		mocks.FCP.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, publishInfo, gomock.Any(), false, false).
			Return("/dev/mapper/mpatha", nil),
	)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachFCPVolume_UnsafeDetachBypassesPrepareError(t *testing.T) {
	core, mocks := newTestCore(t, WithUnsafeDetach(true))
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.FCP.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, publishInfo, nil, true, false).
		Return("", errors.New("device busy but ignored"))
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	// unmappedMpathDevice stays "" on error, so the ghost/removal call target is "".
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), "", gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachFCPVolume_UmountTempMountPointError(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{{"host": 1}})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{"/sys/block/sda"})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda"}, nil)
	mocks.FCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.FCP.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("/dev/mapper/mpatha", nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).
		Return(errors.New("umount failed"))

	err := core.detachFCPVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove temporary directory")
}

func TestDetachFCPVolumeRetry_SucceedsWithoutRetry(t *testing.T) {
	core, mocks := newTestCore(t)
	fcpMock := withMockedFcpUtils(t)
	publishInfo := samplePublishInfo(FCP)

	fcpMock.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return([]map[string]int{})
	fcpMock.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return([]string{})
	fcpMock.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachFCPVolumeRetry(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolumeRetry_SucceedsWithoutRetry(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), ti.IscsiTargetIQN).Return(map[int]int{})
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(
		gomock.Any(), ti.DevicePath, uint64(removeMultipathDeviceMappingRetries), removeMultipathDeviceMappingRetryDelay,
	).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachISCSIVolumeRetry(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_NoHostSessionMap_CleanupOnly(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), ti.IscsiTargetIQN).Return(map[int]int{})
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(
		gomock.Any(), ti.DevicePath, uint64(removeMultipathDeviceMappingRetries), removeMultipathDeviceMappingRetryDelay,
	).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_NoHostSessionMap_LUKSTeardownNonLegacyPath(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.LUKSEncryption = "true"
	ti.DevicePath = "/dev/mapper/mpatha"
	ti.IscsiLunSerial = "" // skip serial-based device path resolution

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{})
	mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice(ti.DevicePath).Return("/dev/mapper/luks-uuid", nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), ti.DevicePath, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_LunSerialResolvesDevicePath(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.IscsiLunSerial = "abc123"
	ti.DevicePath = "/dev/sdz" // stale; should be replaced by the serial-resolved multipath device

	mocks.Devices.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any()).Return("mpatha", nil)
	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{})
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(
		gomock.Any(), "/dev/mpatha", gomock.Any(), gomock.Any(),
	).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
	assert.Equal(t, "/dev/mpatha", ti.VolumePublishInfo.DevicePath)
}

func TestDetachISCSIVolume_GetDeviceInfoError(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), int(ti.IscsiLunNumber), ti.IscsiTargetIQN, false).
		Return(nil, errors.New("scan failed"))

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get device info")
}

func TestDetachISCSIVolume_DeviceInfoNil_ContinuesCleanup(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil, nil)
	mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), &ti.VolumePublishInfo, gomock.Any())
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), ti.DevicePath, ti.IscsiLunSerial).Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), ti.DevicePath, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
	// Not a shared target by default, so logout always proceeds.
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), ti.IscsiTargetIQN, ti.IscsiTargetPortal).Return(nil)
	for _, portal := range ti.IscsiPortals {
		mocks.ISCSI.EXPECT().Logout(gomock.Any(), ti.IscsiTargetIQN, portal).Return(nil)
	}

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

// detachISCSISuccessMocks sets up all the expectations for a non-shared-target, always-logout
// success path with a real device found, shared by several tests below.
func detachISCSISuccessMocks(t *testing.T, mocks *testMocks, iscsiMock *mock_iscsi.MockIscsiReconcileUtils, ti *models.VolumeTrackingInfo, deviceInfo *models.ScsiDeviceInfo) {
	t.Helper()
	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, &ti.VolumePublishInfo, nil, false, false).
		Return(deviceInfo.MultipathDevice, nil)
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any()).Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
}

func TestDetachISCSIVolume_NotSharedTarget_AlwaysLogsOut(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.SharedTarget = false
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	detachISCSISuccessMocks(t, mocks, iscsiMock, ti, deviceInfo)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), ti.IscsiTargetIQN, ti.IscsiTargetPortal).Return(nil)
	for _, portal := range ti.IscsiPortals {
		mocks.ISCSI.EXPECT().Logout(gomock.Any(), ti.IscsiTargetIQN, portal).Return(nil)
	}

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_SharedTarget_HasMountedDevice_NoLogout(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.SharedTarget = true
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	detachISCSISuccessMocks(t, mocks, iscsiMock, ti, deviceInfo)
	mocks.ISCSI.EXPECT().TargetHasMountedDevice(gomock.Any(), ti.IscsiTargetIQN).Return(true, nil)
	// No RemovePortalsFromSession/Logout calls expected: shared target has other mounted devices.

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_SharedTarget_MountCheckError_NoLogout(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.SharedTarget = true
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	detachISCSISuccessMocks(t, mocks, iscsiMock, ti, deviceInfo)
	mocks.ISCSI.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).
		Return(false, errors.New("mount check failed"))

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_SharedTarget_NotSafeToLogout_NoLogout(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.SharedTarget = true
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	detachISCSISuccessMocks(t, mocks, iscsiMock, ti, deviceInfo)
	mocks.ISCSI.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
	mocks.ISCSI.EXPECT().SafeToLogOut(gomock.Any(), 6, 3).Return(false)

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_SharedTarget_SafeToLogout_LogsOutAllPortals(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.SharedTarget = true
	ti.IscsiPortals = []string{"192.0.2.1:3260", "192.0.2.2:3260"}
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	detachISCSISuccessMocks(t, mocks, iscsiMock, ti, deviceInfo)
	mocks.ISCSI.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
	mocks.ISCSI.EXPECT().SafeToLogOut(gomock.Any(), 6, 3).Return(true)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), ti.IscsiTargetIQN, ti.IscsiTargetPortal).Return(nil)
	for _, portal := range ti.IscsiPortals {
		mocks.ISCSI.EXPECT().Logout(gomock.Any(), ti.IscsiTargetIQN, portal).Return(nil)
	}

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_PrepareDeviceForRemovalError_SameLunRetry(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
	gomock.InOrder(
		mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, gomock.Any(), nil, false, false).
			Return("", errors.ISCSISameLunNumberError("same LUN number")),
		mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, gomock.Any(), gomock.Any(), false, false).
			Return(deviceInfo.MultipathDevice, nil),
	)
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any()).Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_UnsafeDetachBypassesError(t *testing.T) {
	core, mocks := newTestCore(t, WithUnsafeDetach(true))
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, gomock.Any(), nil, true, false).
		Return("", errors.New("device busy but ignored"))
	// mpathDevicePath falls back to publishInfo.DevicePath (ti.DevicePath) when prepErr is
	// ignored and unmappedMpathDevice is empty.
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), ti.DevicePath, gomock.Any()).Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), ti.DevicePath, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

func TestDetachISCSIVolume_LUKSTeardownWithDeviceFound(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.LUKSEncryption = "true"
	ti.DevicePath = "/dev/mapper/mpatha"
	deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "/dev/mapper/mpatha"}

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(deviceInfo, nil)
	mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice(ti.DevicePath).Return("/dev/mapper/luks-uuid", nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, gomock.Any(), nil, false, false).
		Return(deviceInfo.MultipathDevice, nil)
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any()).Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false)
	assert.NoError(t, err)
}

// fakeDetachNVMeSubsystem hand-implements the slice of nvme.NVMeSubsystemInterface that
// detachNVMeVolume actually calls (rather than using the real *nvme.NVMeSubsystem), so these
// tests are not at the mercy of nvme_linux.go/nvme_darwin.go's platform-gated (GOOS-specific)
// real device-discovery and flush logic - e.g. on non-Linux dev machines,
// nvme.NVMeSubsystem.GetNVMeDeviceAt/NVMeDevice.FlushNVMeDevice unconditionally return
// errors.UnsupportedError, which would make every "device found" scenario untestable there. This
// also sidesteps nvme.NVMeDevice's unexported `command` field, which only real GOOS-specific
// scanning code can populate, so a *nvme.NVMeDevice built via struct literal from this package
// would panic if FlushDevice ever reached the real command execution path. Any interface method
// not overridden here is intentionally left nil-embedded (panics if called), matching the
// pattern already used by utils_test.go's fakeNVMeSubsystem.
type fakeDetachNVMeSubsystem struct {
	nvme.NVMeSubsystemInterface
	getDeviceErr    error
	disconnectErr   error
	disconnectCalls int
}

func (f *fakeDetachNVMeSubsystem) Disconnect(context.Context) error {
	f.disconnectCalls++
	return f.disconnectErr
}

// GetNVMeDevice/GetNVMeDeviceAt always report "no device found" (nil, getDeviceErr): every
// detachNVMeVolume test below therefore exercises the nvmeDev==nil branches, which is sufficient
// to cover subsystem lookup, LUKS teardown, and disconnect-decision logic without needing a real,
// working *nvme.NVMeDevice (see the flush-retry-map note in this file's summary for why the flush
// branch itself is out of reach here).
func (f *fakeDetachNVMeSubsystem) GetNVMeDevice(context.Context, string) (*nvme.NVMeDevice, error) {
	return nil, f.getDeviceErr
}

func (f *fakeDetachNVMeSubsystem) GetNVMeDeviceAt(context.Context, string) (*nvme.NVMeDevice, error) {
	return nil, f.getDeviceErr
}

var _ nvme.NVMeSubsystemInterface = (*fakeDetachNVMeSubsystem)(nil)

func TestDetachNVMeVolume_NoDeviceFound_Success(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	subsystem := &fakeDetachNVMeSubsystem{}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), publishInfo.NVMeSubsystemNQN, publishInfo.NVMeNamespaceUUID).
		Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), publishInfo.NVMeSubsystemNQN).Return(subsystem)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), publishInfo.GlobalMount).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, subsystem.disconnectCalls, "numNs==0 by default, so Disconnect must be invoked")
}

func TestDetachNVMeVolume_GetNVMeDeviceNonNotFoundError(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	subsystem := &fakeDetachNVMeSubsystem{getDeviceErr: errors.New("nvme-cli failed")}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get NVMe device")
}

func TestDetachNVMeVolume_GetNVMeDeviceNotFoundError_ContinuesCleanup(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	subsystem := &fakeDetachNVMeSubsystem{getDeviceErr: errors.NotFoundError("no device")}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachNVMeVolume_LUKSTeardown_NoDeviceFound(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"
	subsystem := &fakeDetachNVMeSubsystem{}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Devices.EXPECT().GetLUKSDevicePathForDevicePath(gomock.Any(), publishInfo.DevicePath).
		Return("/dev/mapper/luks-uuid", nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachNVMeVolume_LUKSDevicePathLookupError(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"
	subsystem := &fakeDetachNVMeSubsystem{}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Devices.EXPECT().GetLUKSDevicePathForDevicePath(gomock.Any(), publishInfo.DevicePath).
		Return("", errors.New("lookup failed"))

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lookup failed")
}

func TestDetachNVMeVolume_UmountTempMountPointError(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	subsystem := &fakeDetachNVMeSubsystem{}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).
		Return(errors.New("umount failed"))

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove temporary directory")
}

func TestDetachNVMeVolume_DeleteTrackingInfoError(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	subsystem := &fakeDetachNVMeSubsystem{}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(errors.New("delete failed"))

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestDetachNVMeVolume_DisconnectSkippedWhenNamespacesRemain(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	// Pre-populate the package-level published session map with a namespace still attached to
	// this subsystem, so disconnectNVMeSubsystemIfNeeded's numNs>0 check skips Disconnect().
	publishedNVMeSessions.AddNVMeSession(*nvme.NewNVMeSubsystem(publishInfo.NVMeSubsystemNQN, mocks.Command, afero.NewMemMapFs()), nil)
	publishedNVMeSessions.AddNamespaceToSession(publishInfo.NVMeSubsystemNQN, "some-other-namespace-uuid")
	t.Cleanup(func() { publishedNVMeSessions.RemoveNVMeSession(publishInfo.NVMeSubsystemNQN) })

	subsystem := &fakeDetachNVMeSubsystem{}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
	assert.Equal(t, 0, subsystem.disconnectCalls, "a remaining namespace must prevent disconnect")
}

func TestDetachNVMeVolume_DisconnectErrorIsNonFatal(t *testing.T) {
	clearNVMeFlushRetryMap(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	subsystem := &fakeDetachNVMeSubsystem{disconnectErr: errors.New("disconnect failed")}

	mocks.NVMe.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
	mocks.NVMe.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(subsystem)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	// disconnectNVMeSubsystemIfNeeded's error is logged and swallowed; detach must still proceed
	// with the rest of cleanup and return success.
	err := core.detachNVMeVolume(context.Background(), "test-volume", publishInfo, false)
	assert.NoError(t, err)
}
