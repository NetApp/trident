// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/pkg/locks/distlock"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
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

	err := core.detachISCSIVolumeRetry(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
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

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
	assert.NoError(t, err)
}

const staleDMLunSerial = "yocwC+Ws3R1K"

var (
	staleDMOwnedUUID       = "mpath-3600a0980" + hex.EncodeToString([]byte(staleDMLunSerial))
	staleDMOtherVolumeUUID = "mpath-3600a0980" + hex.EncodeToString([]byte("81OcA?XLUkD6"))
)

// staleDMTrackingInfo models a retry where the LUN serial no longer resolves, so the detach falls back to the
// tracking file's /dev/dm-9, whose minor may since have been reused by another device.
func staleDMTrackingInfo(mocks *testMocks, luksEncrypted bool) *models.VolumeTrackingInfo {
	ti := sampleTrackingInfo(ISCSI)
	ti.IscsiLunSerial = staleDMLunSerial
	ti.DevicePath = "/dev/dm-9"
	if luksEncrypted {
		ti.LUKSEncryption = "true"
	}
	mocks.Devices.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), hex.EncodeToString([]byte(staleDMLunSerial))).
		Return("", errors.NotFoundError("not found"))
	return ti
}

// expectISCSIDetachWithDevicesGone sets up a detach with a live session but no SCSI devices left for the LUN.
func expectISCSIDetachWithDevicesGone(mocks *testMocks, iscsiMock *mock_iscsi.MockIscsiReconcileUtils) {
	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
	mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil, nil)
	mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), "/dev/dm-9", staleDMLunSerial).
		Return(errors.New("device still has 1 slave(s)"))
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
}

func TestDetachISCSIVolume_StaleDM_FlushOnlyIfOwnedByLUN(t *testing.T) {
	tests := map[string]struct {
		sessions    bool
		uuid        string
		uuidErr     error
		expectFlush bool
	}{
		"dm minor reused by another device is not flushed": {
			sessions: true, uuid: "part1-" + staleDMOwnedUUID,
		},
		"dm already removed is not flushed": {
			sessions: true, uuidErr: errors.NotFoundError("multipath device 'dm-9' UUID not found"),
		},
		"dm still owned by LUN is flushed": {
			sessions: true, uuid: staleDMOwnedUUID, expectFlush: true,
		},
		"no sessions, dm minor reused is not flushed": {
			uuid: "LVM-abc123",
		},
		"no sessions, dm still owned by LUN is flushed": {
			uuid: staleDMOwnedUUID, expectFlush: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			iscsiMock := withMockedIscsiUtils(t)
			ti := staleDMTrackingInfo(mocks, false)

			if tc.sessions {
				expectISCSIDetachWithDevicesGone(mocks, iscsiMock)
			} else {
				iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{})
				mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
			}
			mocks.Devices.EXPECT().GetMultipathDeviceUUID("/dev/dm-9").Return(tc.uuid, tc.uuidErr)
			if tc.expectFlush {
				mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), "/dev/dm-9",
					gomock.Any(), gomock.Any()).Return(nil)
			}

			err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
			assert.NoError(t, err)
		})
	}
}

func TestDetachISCSIVolume_StaleDM_LUKSClosedOnlyIfOwnedByLUN(t *testing.T) {
	tests := map[string]struct {
		sessions bool
		uuid     string
		owned    bool
	}{
		"LUKS device on dm minor reused by another volume is not closed": {
			sessions: true, uuid: staleDMOtherVolumeUUID,
		},
		"LUKS device on dm still owned by LUN is closed": {
			sessions: true, uuid: staleDMOwnedUUID, owned: true,
		},
		"no sessions, LUKS device on reused dm minor is not closed": {
			uuid: staleDMOtherVolumeUUID,
		},
		"no sessions, LUKS device on dm still owned by LUN is closed": {
			uuid: staleDMOwnedUUID, owned: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			iscsiMock := withMockedIscsiUtils(t)
			ti := staleDMTrackingInfo(mocks, true)

			// Read once before the LUKS lookup and once before the final flush.
			mocks.Devices.EXPECT().GetMultipathDeviceUUID("/dev/dm-9").Return(tc.uuid, nil).Times(2)
			luksPath := ""
			if tc.owned {
				luksPath = "/dev/mapper/luks-test"
				mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice("/dev/dm-9").Return(luksPath, nil)
				mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), "/dev/dm-9",
					gomock.Any(), gomock.Any()).Return(nil)
			}
			if tc.sessions {
				expectISCSIDetachWithDevicesGone(mocks, iscsiMock)
				if tc.owned {
					mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), luksPath).Return(nil)
					mocks.Devices.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), luksPath).Return(nil)
				}
			} else {
				iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{})
				mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), luksPath).Return(nil)
				mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
			}

			err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
			assert.NoError(t, err)
		})
	}
}

// TestDetachISCSIVolume_FinalFlushFollowsPrepareDeviceForRemoval covers PrepareDeviceForRemoval running against
// the LUN's dm-9: it returns the device when the flush still needs to happen, otherwise it already flushed it.
func TestDetachISCSIVolume_FinalFlushFollowsPrepareDeviceForRemoval(t *testing.T) {
	tests := map[string]struct {
		unmappedDevice string
	}{
		"successful first flush is not repeated":     {unmappedDevice: ""},
		"deferred flush runs on the unmapped device": {unmappedDevice: "/dev/dm-9"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			iscsiMock := withMockedIscsiUtils(t)
			ti := sampleTrackingInfo(ISCSI)
			ti.IscsiLunSerial = staleDMLunSerial
			deviceInfo := &models.ScsiDeviceInfo{MultipathDevice: "dm-9"}

			mocks.Devices.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any()).Return("dm-9", nil)
			iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{6: 3})
			mocks.ISCSI.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
				Return(deviceInfo, nil)
			mocks.ISCSI.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
			mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, gomock.Any(), nil, false, false).
				Return(tc.unmappedDevice, nil)
			mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), "/dev/dm-9", staleDMLunSerial).
				Return(errors.New("device still has 1 slave(s)"))
			mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
			mocks.ISCSI.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
			if tc.unmappedDevice != "" {
				mocks.Devices.EXPECT().GetMultipathDeviceUUID(tc.unmappedDevice).Return(staleDMOwnedUUID, nil)
			}
			mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), tc.unmappedDevice,
				gomock.Any(), gomock.Any()).Return(nil)
			mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

			err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
			assert.NoError(t, err)
		})
	}
}

func TestMultipathDeviceBelongsToLUN(t *testing.T) {
	tests := map[string]struct {
		devicePath string
		lunSerial  string
		uuid       string
		uuidErr    error
		readsUUID  bool
		expected   bool
	}{
		"no serial cannot be verified":         {devicePath: "/dev/dm-9", expected: true},
		"mapper path cannot be verified":       {devicePath: "/dev/mapper/3600a0980", lunSerial: staleDMLunSerial, expected: true},
		"empty path cannot be verified":        {devicePath: "", lunSerial: staleDMLunSerial, expected: true},
		"device owned by LUN":                  {devicePath: "/dev/dm-9", lunSerial: staleDMLunSerial, uuid: staleDMOwnedUUID, readsUUID: true, expected: true},
		"bare dm name owned by LUN":            {devicePath: "dm-9", lunSerial: staleDMLunSerial, uuid: staleDMOwnedUUID, readsUUID: true, expected: true},
		"device no longer exists":              {devicePath: "/dev/dm-9", lunSerial: staleDMLunSerial, uuidErr: errors.NotFoundError("gone"), readsUUID: true, expected: false},
		"minor reused by LVM":                  {devicePath: "/dev/dm-9", lunSerial: staleDMLunSerial, uuid: "LVM-abc123", readsUUID: true, expected: false},
		"minor reused by kpartx partition map": {devicePath: "/dev/dm-9", lunSerial: staleDMLunSerial, uuid: "part1-" + staleDMOwnedUUID, readsUUID: true, expected: false},
		"minor reused by another LUN's map":    {devicePath: "/dev/dm-9", lunSerial: staleDMLunSerial, uuid: staleDMOtherVolumeUUID, readsUUID: true, expected: false},
		"UUID read error is assumed to match":  {devicePath: "/dev/dm-9", lunSerial: staleDMLunSerial, uuidErr: errors.New("permission denied"), readsUUID: true, expected: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			if tc.readsUUID {
				mocks.Devices.EXPECT().GetMultipathDeviceUUID(tc.devicePath).Return(tc.uuid, tc.uuidErr)
			}

			assert.Equal(t, tc.expected, core.multipathDeviceBelongsToLUN(context.Background(), tc.devicePath, tc.lunSerial))
		})
	}
}

// TestDetachISCSIVolume_NoHostSessionMap_LUKS_LockDeleteFailed covers the early-exit path at
// line 480 of detach.go: the device is already gone (no host session map), the LUKS device is
// closed successfully, but the locker cannot delete the lease (ErrLockDeleteFailed). The function
// must propagate the error so the CO can retry.
func TestDetachISCSIVolume_NoHostSessionMap_LUKS_LockDeleteFailed(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.LUKSEncryption = "true"
	ti.DevicePath = "/dev/mapper/mpatha"
	ti.IscsiLunSerial = ""

	iscsiMock.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(map[int]int{})
	mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice(ti.DevicePath).Return("/dev/mapper/luks-uuid", nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	// RemoveMultipathDeviceMappingWithRetries and DeleteTrackingInfo are NOT expected: the error
	// must short-circuit before we reach them.

	locker := lockerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return distlock.ErrLockDeleteFailed
	})
	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, locker)
	require.Error(t, err)
	assert.True(t, errors.IsInternalError(err), "expected InternalError, got: %v", err)
}

// TestDetachISCSIVolume_LUKS_LockDeleteFailed covers path at line 538 of detach.go: the main
// detach path finds a device, closes the LUKS device successfully, but the locker cannot delete
// the lease. The function must return InternalError (translated from ErrLockDeleteFailed) for CO retry.
func TestDetachISCSIVolume_LUKS_LockDeleteFailed(t *testing.T) {
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

	locker := lockerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return distlock.ErrLockDeleteFailed
	})
	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, locker)
	require.Error(t, err)
	assert.True(t, errors.IsInternalError(err), "expected InternalError, got: %v", err)
}

// TestDetachISCSIVolume_LUKS_LockAcquireConflict verifies that ErrLockAcquireConflict at the
// second close site (line ~538) is translated to VolumeStateError so the CO can retry.
func TestDetachISCSIVolume_LUKS_LockAcquireConflict(t *testing.T) {
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
	// The lock is acquired by another host; EnsureLUKSDeviceClosedWithMaxWaitLimit is never called.

	locker := lockerFunc(func(_ context.Context, _ func(context.Context) error) error {
		return distlock.ErrLockAcquireConflict
	})
	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, locker)
	require.Error(t, err)
	assert.True(t, errors.IsVolumeStateError(err), "expected VolumeStateError, got: %v", err)
}

// TestDetachISCSIVolume_LUKS_MaxWaitExceeded_ContinuesCleanup covers the default case at line 544
// of detach.go: when EnsureLUKSDeviceClosedWithMaxWaitLimit returns a MaxWaitExceededError, the
// detach must continue rather than returning an error, leaving cleanup to subsequent calls.
func TestDetachISCSIVolume_LUKS_MaxWaitExceeded_ContinuesCleanup(t *testing.T) {
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
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), "/dev/mapper/luks-uuid").
		Return(errors.MaxWaitExceededError("timed out waiting for LUKS close"))
	mocks.ISCSI.EXPECT().PrepareDeviceForRemoval(gomock.Any(), deviceInfo, gomock.Any(), nil, false, false).
		Return(deviceInfo.MultipathDevice, nil)
	mocks.Devices.EXPECT().RemoveGhostMultipathDevice(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any()).Return(nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	mocks.Devices.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), deviceInfo.MultipathDevice, gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
	mocks.ISCSI.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, distlock.NewNoopLock())
	assert.NoError(t, err)
}

// TestDetachISCSIVolume_LUKS_PostBlockDeviceRemoval_LockDeleteFailed covers path at line 653 of
// detach.go: the LUKS device lingered past block device removal, EnsureLUKSDeviceClosed succeeds,
// but the locker cannot delete the lease. The function must return ErrLockDeleteFailed.
//
// SharedTarget=true with TargetHasMountedDevice returning true suppresses the logout path so the
// test only needs to set up expectations for the LUKS-specific calls and cleanup.
func TestDetachISCSIVolume_LUKS_PostBlockDeviceRemoval_LockDeleteFailed(t *testing.T) {
	core, mocks := newTestCore(t)
	iscsiMock := withMockedIscsiUtils(t)
	ti := sampleTrackingInfo(ISCSI)
	ti.LUKSEncryption = "true"
	ti.DevicePath = "/dev/mapper/mpatha"
	ti.SharedTarget = true // suppresses logout to keep mock setup tractable
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
	// SharedTarget + TargetHasMountedDevice=true → logout=false, skip all RemovePortals/Logout calls.
	mocks.ISCSI.EXPECT().TargetHasMountedDevice(gomock.Any(), ti.IscsiTargetIQN).Return(true, nil)
	mocks.Mount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Devices.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), "/dev/mapper/luks-uuid").Return(nil)
	// RemoveMultipathDeviceMappingWithRetries and DeleteTrackingInfo are NOT expected: the error propagates.

	// A locker that succeeds on the first WithLock call and returns ErrLockDeleteFailed on the second.
	callCount := 0
	locker := lockerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		callCount++
		if err := fn(ctx); err != nil {
			return err
		}
		if callCount >= 2 {
			return distlock.ErrLockDeleteFailed
		}
		return nil
	})

	err := core.detachISCSIVolume(context.Background(), "test-volume", ti, false, locker)
	require.Error(t, err)
	assert.True(t, errors.IsInternalError(err), "expected InternalError, got: %v", err)
}

const nvmeTestCryptID = "CRYPT-LUKS2-x"

// nvmeTrackingInfo is the tracking info of a staged NVMe volume whose device stack is recorded, so detach
// reads it from the file instead of discovering it. A LUKS volume has the crypt layer on top of the namespace.
func nvmeTrackingInfo(publishInfo *models.VolumePublishInfo) *models.VolumeTrackingInfo {
	trackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		PublishedPaths:    map[string]struct{}{},
	}
	trackingInfo.BlockAttachInfo.Devices = models.BlockDeviceStack{
		{ID: publishInfo.NVMeNamespaceUUID, Type: models.BlockDeviceDisk},
	}
	if publishInfo.UseLUKS() {
		trackingInfo.BlockAttachInfo.Devices = append(trackingInfo.BlockAttachInfo.Devices,
			models.BlockDeviceInfo{ID: nvmeTestCryptID, Type: models.BlockDeviceCrypt, Base: publishInfo.NVMeNamespaceUUID})
	}
	return trackingInfo
}

func detachNVMe(core *Core, mocks *testMocks, publishInfo *models.VolumePublishInfo, force bool) error {
	return detachNVMeTracked(core, mocks, nvmeTrackingInfo(publishInfo), force)
}

func detachNVMeTracked(core *Core, mocks *testMocks, trackingInfo *models.VolumeTrackingInfo, force bool) error {
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-volume").Return(trackingInfo, nil)
	return core.detach(context.Background(), "test-volume", force)
}

// expectNVMeDetachCleanup expects the tracking info deletion that follows a successful driver detach.
func expectNVMeDetachCleanup(mocks *testMocks, publishInfo *models.VolumePublishInfo) {
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)
}

func TestDetachNVMeVolume_Success_Order(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	gomock.InOrder(
		mocks.NVMeDriver.EXPECT().Detach(gomock.Any(), "test-volume", gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, state *models.VolumeAttachState, spec *models.VolumeDetachSpec) error {
				assert.Equal(t, publishInfo.NVMeSubsystemNQN, state.NVMeSubsystemNQN)
				assert.Equal(t, publishInfo.NVMeNamespaceUUID, state.NVMeNamespaceUUID)
				assert.Equal(t, publishInfo.GlobalMount, state.GlobalMount, "for the temporary expand mount")
				assert.Equal(t, &models.VolumeDetachSpec{InternalID: publishInfo.InternalID}, spec)
				return nil
			}),
		mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil),
	)

	err := detachNVMe(core, mocks, publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachNVMeVolume_SpecCarriesForceAndUnsafeDetach(t *testing.T) {
	core, mocks := newTestCore(t, WithUnsafeDetach(true))
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMeDriver.EXPECT().Detach(
		gomock.Any(), "test-volume", gomock.Any(), &models.VolumeDetachSpec{Force: true, IgnoreFlushErrors: true},
	).Return(nil)
	expectNVMeDetachCleanup(mocks, publishInfo)

	assert.NoError(t, detachNVMe(core, mocks, publishInfo, true))
}

// A driver failure to find the namespace, other than not found, aborts the detach.
func TestDetachNVMeVolume_DriverDetachError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMeDriver.EXPECT().Detach(gomock.Any(), "test-volume", gomock.Any(), gomock.Any()).
		Return(errors.New("failed to get NVMe device: nvme-cli failed"))

	err := detachNVMe(core, mocks, publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get NVMe device")
}

func TestDetachNVMeVolume_TrackingInfoWithoutDeviceStack(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	// A tracking file with no device stack reaches the driver unchanged; the driver resolves it from the host.
	mocks.NVMeDriver.EXPECT().Detach(gomock.Any(), "test-volume", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, state *models.VolumeAttachState, _ *models.VolumeDetachSpec) error {
			assert.Empty(t, state.Devices)
			assert.Equal(t, publishInfo.NVMeNamespaceUUID, state.NVMeNamespaceUUID)
			return nil
		})
	expectNVMeDetachCleanup(mocks, publishInfo)

	trackingInfo := &models.VolumeTrackingInfo{VolumePublishInfo: *publishInfo}
	err := detachNVMeTracked(core, mocks, trackingInfo, false)
	assert.NoError(t, err)
}

func TestDetachNVMeVolume_LUKSTeardown(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"
	publishInfo.InternalID = "trident_pvc_1"

	mocks.NVMeDriver.EXPECT().Detach(gomock.Any(), "test-volume", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, state *models.VolumeAttachState, spec *models.VolumeDetachSpec) error {
			assert.True(t, state.Encryption.IsLUKS())
			crypt, ok := state.BlockAttachInfo.Devices.Find(models.BlockDeviceCrypt)
			require.True(t, ok)
			assert.Equal(t, nvmeTestCryptID, crypt.ID)
			assert.Equal(t, &models.VolumeDetachSpec{InternalID: "trident_pvc_1"}, spec)
			return nil
		})
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(nil)

	err := detachNVMe(core, mocks, publishInfo, false)
	assert.NoError(t, err)
}

func TestDetachNVMeVolume_DeleteTrackingInfoError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMeDriver.EXPECT().Detach(gomock.Any(), "test-volume", gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), "test-volume").Return(errors.New("delete failed"))

	err := detachNVMe(core, mocks, publishInfo, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestDetachNVMeVolume_DetachMaxWaitExceededReturnsEarly(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	// DeleteTrackingInfo is not expected: the early return skips it and kubelet retries.
	mocks.NVMeDriver.EXPECT().Detach(gomock.Any(), "test-volume", gomock.Any(), gomock.Any()).
		Return(errors.MaxWaitExceededError("request waited too long for the lock"))

	err := detachNVMe(core, mocks, publishInfo, false)

	require.Error(t, err)
	assert.True(t, errors.IsMaxWaitExceededError(err))
}
