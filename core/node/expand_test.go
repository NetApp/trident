// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

func TestExpand_EmptyVolume(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Expand(context.Background(), "", ExpandRequest{MountPath: "/mnt/path", RequiredBytes: 1024})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume is empty")
}

func TestExpand_EmptyMountPath(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: "", RequiredBytes: 1024, Secrets: nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume path is empty")
}

func TestExpand_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: "/mnt/path", RequiredBytes: 1024, Secrets: nil})
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestExpand_ReadTrackingInfoNotFound(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(nil, errors.NotFoundError("nope"))

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: "/mnt/path", RequiredBytes: 1024, Secrets: nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to find tracking file for volume: test-vol")
	assert.True(t, errors.IsNotFoundError(err))
}

func TestExpand_ReadTrackingInfoOtherErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(nil, errors.New("disk error"))

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: "/mnt/path", RequiredBytes: 1024, Secrets: nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk error")
}

func TestExpand_MountPathMismatchStillProceeds(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NFS)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)

	// The tracking info's staged path is "/var/lib/trident/staging/test-volume" (per samplePublishInfo),
	// so passing a different mountPath must merely log a warning, not fail the call.
	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: "/some/other/path", RequiredBytes: 1024, Secrets: nil})
	assert.NoError(t, err)
}

func TestExpand_ProtocolNFS_NoOp(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NFS)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 1024, Secrets: nil})
	assert.NoError(t, err)
}

func TestExpand_ProtocolSMB_NoOp(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(SMB)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 1024, Secrets: nil})
	assert.NoError(t, err)
}

func TestExpand_ProtocolISCSI_Delegates(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(ISCSI)
	trackingInfo.VolumePublishInfo.FilesystemType = filesystem.Raw
	trackingInfo.VolumePublishInfo.DevicePath = ""
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)
	mocks.ISCSI.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), int64(2048)).Return(nil)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 2048, Secrets: nil})
	assert.NoError(t, err)
}

func TestExpand_ProtocolFCP_Delegates(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(FCP)
	trackingInfo.VolumePublishInfo.FilesystemType = filesystem.Raw
	trackingInfo.VolumePublishInfo.DevicePath = ""
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)
	mocks.FCP.EXPECT().IsAlreadyAttached(gomock.Any(), 0, trackingInfo.VolumePublishInfo.FCTargetWWNN).Return(true)
	mocks.FCP.EXPECT().RescanDevices(gomock.Any(), trackingInfo.VolumePublishInfo.FCTargetWWNN, int32(0), int64(2048)).Return(nil)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 2048, Secrets: nil})
	assert.NoError(t, err)
}

func TestExpand_ProtocolNVMe_Delegates(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NVMe)
	trackingInfo.VolumePublishInfo.FilesystemType = filesystem.Raw
	trackingInfo.VolumePublishInfo.DevicePath = ""
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 2048, Secrets: nil})
	assert.NoError(t, err)
}

func TestExpand_UnknownProtocol_Error(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NFS)
	trackingInfo.VolumePublishInfo.StorageProtocol = models.StorageProtocol("bogus")
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil)

	err := core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 1024, Secrets: nil})
	require.Error(t, err)
	assert.True(t, errors.IsPreconditionError(err))
}

func TestExpand_VolumeLockReleasedAfterCall(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NFS)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "test-vol").Return(trackingInfo, nil).Times(2)

	// If Expand failed to release the per-volume lock, the second call would deadlock.
	done := make(chan error, 1)
	require.NoError(t, core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 1024, Secrets: nil}))
	go func() {
		done <- core.Expand(context.Background(), "test-vol", ExpandRequest{MountPath: trackingInfo.GlobalMount, RequiredBytes: 1024, Secrets: nil})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("second Expand call deadlocked; volume lock was not released")
	}
}

func TestExpandISCSIVolume_CapturePreExpandBaselineErrorPropagates(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = "unsupported-fs"

	err := core.expandISCSIVolume(context.Background(), "test-vol", pi, 1024, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported fileSystemType option")
}

func TestExpandISCSIVolume_ExpandVolumeErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = ""
	mocks.ISCSI.EXPECT().ExpandVolume(gomock.Any(), pi, int64(1024)).Return(errors.New("expand failed"))

	err := core.expandISCSIVolume(context.Background(), "test-vol", pi, 1024, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expand failed")
}

func TestExpandISCSIVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = ""
	mocks.ISCSI.EXPECT().ExpandVolume(gomock.Any(), pi, int64(1024)).Return(nil)

	err := core.expandISCSIVolume(context.Background(), "test-vol", pi, 1024, nil)
	assert.NoError(t, err)
}

func TestExpandFCPVolume_NotAttached(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(FCP)
	mocks.FCP.EXPECT().IsAlreadyAttached(gomock.Any(), int(pi.FCPLunNumber), pi.FCTargetWWNN).Return(false)

	err := core.expandFCPVolume(context.Background(), "test-vol", pi, 1024, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not attached")
}

func TestExpandFCPVolume_RescanErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(FCP)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = ""
	mocks.FCP.EXPECT().IsAlreadyAttached(gomock.Any(), int(pi.FCPLunNumber), pi.FCTargetWWNN).Return(true)
	mocks.FCP.EXPECT().RescanDevices(gomock.Any(), pi.FCTargetWWNN, pi.FCPLunNumber, int64(1024)).Return(errors.New("rescan failed"))

	err := core.expandFCPVolume(context.Background(), "test-vol", pi, 1024, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rescan failed")
}

func TestExpandFCPVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(FCP)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = ""
	mocks.FCP.EXPECT().IsAlreadyAttached(gomock.Any(), int(pi.FCPLunNumber), pi.FCTargetWWNN).Return(true)
	mocks.FCP.EXPECT().RescanDevices(gomock.Any(), pi.FCTargetWWNN, pi.FCPLunNumber, int64(1024)).Return(nil)

	err := core.expandFCPVolume(context.Background(), "test-vol", pi, 1024, nil)
	assert.NoError(t, err)
}

func TestExpandNVMeVolume_CapturePreExpandBaselineErrorPropagates(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(NVMe)
	pi.FilesystemType = "unsupported-fs"

	err := core.expandNVMeVolume(context.Background(), "test-vol", pi, 1024, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported fileSystemType option")
}

func TestExpandNVMeVolume_Success(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(NVMe)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = ""

	// No rescan step for NVMe: no NVMe/Devices/Filesystem mock calls expected at all, since
	// Raw+empty-DevicePath skips every size lookup in capturePreExpandSizeBaseline and
	// expandFilesystemAndLUKS.
	err := core.expandNVMeVolume(context.Background(), "test-vol", pi, 1024, nil)
	assert.NoError(t, err)
}

func TestCapturePreExpandSizeBaseline_UnsupportedFilesystem(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = "zfs"

	_, _, err := core.capturePreExpandSizeBaseline(context.Background(), pi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported fileSystemType option: zfs")
}

func TestCapturePreExpandSizeBaseline_RawSkipsFilesystemSizeLookup(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = "/dev/sdz"
	mocks.Devices.EXPECT().GetDiskSize(gomock.Any(), "/dev/sdz").Return(int64(4096), nil)
	// No Filesystem.GetFilesystemSize expectation: a call would fail the test.

	devSize, fsSize, err := core.capturePreExpandSizeBaseline(context.Background(), pi)
	require.NoError(t, err)
	assert.Equal(t, int64(4096), devSize)
	assert.Equal(t, int64(0), fsSize)
}

func TestCapturePreExpandSizeBaseline_NonRawCallsFilesystemSize(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = ""
	mocks.Filesystem.EXPECT().GetFilesystemSize(gomock.Any(), pi.GlobalMount).Return(int64(1024), nil)

	devSize, fsSize, err := core.capturePreExpandSizeBaseline(context.Background(), pi)
	require.NoError(t, err)
	assert.Equal(t, int64(0), devSize)
	assert.Equal(t, int64(1024), fsSize)
}

func TestCapturePreExpandSizeBaseline_FilesystemSizeErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = ""
	mocks.Filesystem.EXPECT().GetFilesystemSize(gomock.Any(), pi.GlobalMount).Return(int64(0), errors.New("stat failed"))

	_, _, err := core.capturePreExpandSizeBaseline(context.Background(), pi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat failed")
}

func TestCapturePreExpandSizeBaseline_EmptyDevicePathSkipsDiskSize(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = ""
	mocks.Filesystem.EXPECT().GetFilesystemSize(gomock.Any(), pi.GlobalMount).Return(int64(1024), nil)
	// No Devices.GetDiskSize expectation: a call would fail the test.

	devSize, _, err := core.capturePreExpandSizeBaseline(context.Background(), pi)
	require.NoError(t, err)
	assert.Equal(t, int64(0), devSize)
}

func TestCapturePreExpandSizeBaseline_DiskSizeErrorSwallowed(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = "/dev/sdz"
	mocks.Devices.EXPECT().GetDiskSize(gomock.Any(), "/dev/sdz").Return(int64(0), errors.New("io error"))

	devSize, _, err := core.capturePreExpandSizeBaseline(context.Background(), pi)
	require.NoError(t, err, "GetDiskSize errors must be swallowed (logged), not returned")
	assert.Equal(t, int64(0), devSize)
}

func TestExpandFilesystemAndLUKS_UnsupportedFilesystem(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = "zfs"

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, nil, 0, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported fileSystemType option: zfs")
}

func TestExpandFilesystemAndLUKS_LUKSMissingPassphrase(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.LUKSEncryption = "true"
	pi.DevicePath = "/dev/mapper/luks-abc"

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, nil, 0, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no passphrase provided")
}

func TestExpandFilesystemAndLUKS_LUKSEmptyPassphrase(t *testing.T) {
	core, _ := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.LUKSEncryption = "true"
	pi.DevicePath = "/dev/mapper/luks-abc"
	secrets := map[string]string{"luks-passphrase": ""}

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, secrets, 0, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty passphrase provided")
}

func TestExpandFilesystemAndLUKS_LUKSLegacyDevicePathSkipsMultipathResolution(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.LUKSEncryption = "true"
	pi.DevicePath = "/dev/mapper/luks-abc"
	secrets := map[string]string{"luks-passphrase": "secret"}
	// No Devices.GetLUKSDeviceForMultipathDevice expectation: a call would fail the test, since
	// IsLegacyDevicePath("/dev/mapper/luks-abc") is true.

	// Resize's cryptsetup invocation is: ctx, "cryptsetup", timeout, logOutput (4 fixed params),
	// then passphrase, "resize", devicePath (3 variadic args) - 7 matchers total. Only exercised
	// on linux; harmless if never called (darwin/windows).
	mocks.Command.EXPECT().
		ExecuteWithTimeoutAndInput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte{}, errors.New("cryptsetup resize failed")).
		AnyTimes()

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, secrets, 0, 0)
	require.Error(t, err)
}

func TestExpandFilesystemAndLUKS_LUKSMultipathResolutionErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.LUKSEncryption = "true"
	pi.DevicePath = "/dev/mapper/mpatha"
	secrets := map[string]string{"luks-passphrase": "secret"}
	mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice("/dev/mapper/mpatha").Return("", errors.New("no mapping found"))

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, secrets, 0, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no mapping found")
}

func TestExpandFilesystemAndLUKS_LUKSMultipathResolutionSuccess_ResizeFailurePropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.LUKSEncryption = "true"
	pi.DevicePath = "/dev/mapper/mpatha"
	secrets := map[string]string{"luks-passphrase": "secret"}
	mocks.Devices.EXPECT().GetLUKSDeviceForMultipathDevice("/dev/mapper/mpatha").Return("/dev/mapper/luks-abc", nil)

	// Multipath resolution succeeds, but the subsequent luks.Device.Resize call fails, and that
	// failure should propagate. Only exercised on linux; harmless if never called (darwin/windows).
	mocks.Command.EXPECT().
		ExecuteWithTimeoutAndInput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte{}, errors.New("cryptsetup resize failed")).
		AnyTimes()

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, secrets, 0, 0)
	require.Error(t, err)
}

func TestExpandFilesystemAndLUKS_RawFilesystemSkipsExpandFilesystemOnNode(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Raw
	pi.DevicePath = "/dev/sdz"
	mocks.Devices.EXPECT().GetDiskSize(gomock.Any(), "/dev/sdz").Return(int64(2048), nil)
	// No Filesystem.ExpandFilesystemOnNode expectation: a call would fail the test.

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 1024, nil, 1024, 0)
	assert.NoError(t, err)
}

func TestExpandFilesystemAndLUKS_EmptyDevicePathSkipsPostExpandSizeCheck(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = ""
	// No Devices.GetDiskSize expectation: a call would fail the test.
	mocks.Filesystem.EXPECT().
		ExpandFilesystemOnNode(gomock.Any(), pi, "", pi.GlobalMount, filesystem.Ext4, pi.MountOptions, int64(2048)).
		Return(int64(4096), nil)

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 2048, nil, 0, 1024)
	assert.NoError(t, err)
}

func TestExpandFilesystemAndLUKS_ExpandFilesystemOnNodeErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = ""
	mocks.Filesystem.EXPECT().
		ExpandFilesystemOnNode(gomock.Any(), pi, "", pi.GlobalMount, filesystem.Ext4, pi.MountOptions, int64(2048)).
		Return(int64(0), errors.New("resize2fs failed"))

	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 2048, nil, 0, 1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resize2fs failed")
}

func TestExpandFilesystemAndLUKS_FilesystemDidNotGrowError(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = "/dev/sdz"
	preExpandDeviceSizeBytes := int64(100)
	preExpandFilesystemSize := int64(500)
	mocks.Devices.EXPECT().GetDiskSize(gomock.Any(), "/dev/sdz").Return(int64(200), nil) // device grew (200 > 100)
	mocks.Filesystem.EXPECT().
		ExpandFilesystemOnNode(gomock.Any(), pi, pi.DevicePath, pi.GlobalMount, filesystem.Ext4, pi.MountOptions, int64(2048)).
		Return(int64(500), nil) // filesystem did NOT grow (500 <= 500)

	err := core.expandFilesystemAndLUKS(
		context.Background(), "test-vol", pi, 2048, nil, preExpandDeviceSizeBytes, preExpandFilesystemSize)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filesystem size did not grow")
}

func TestExpandFilesystemAndLUKS_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = "/dev/sdz"
	preExpandDeviceSizeBytes := int64(100)
	preExpandFilesystemSize := int64(500)
	mocks.Devices.EXPECT().GetDiskSize(gomock.Any(), "/dev/sdz").Return(int64(200), nil)
	mocks.Filesystem.EXPECT().
		ExpandFilesystemOnNode(gomock.Any(), pi, pi.DevicePath, pi.GlobalMount, filesystem.Ext4, pi.MountOptions, int64(2048)).
		Return(int64(900), nil) // filesystem grew (900 > 500)

	err := core.expandFilesystemAndLUKS(
		context.Background(), "test-vol", pi, 2048, nil, preExpandDeviceSizeBytes, preExpandFilesystemSize)
	assert.NoError(t, err)
}

func TestExpandFilesystemAndLUKS_PostExpandDiskSizeErrorSwallowed(t *testing.T) {
	core, mocks := newTestCore(t)
	pi := samplePublishInfo(ISCSI)
	pi.FilesystemType = filesystem.Ext4
	pi.DevicePath = "/dev/sdz"
	mocks.Devices.EXPECT().GetDiskSize(gomock.Any(), "/dev/sdz").Return(int64(0), errors.New("io error"))
	mocks.Filesystem.EXPECT().
		ExpandFilesystemOnNode(gomock.Any(), pi, pi.DevicePath, pi.GlobalMount, filesystem.Ext4, pi.MountOptions, int64(2048)).
		Return(int64(900), nil)

	// devicesGrew is computed from postExpandDeviceSizeBytes, which stays 0 when GetDiskSize
	// errors; devicesGrew ends up false, so the growth-verification error path never triggers.
	err := core.expandFilesystemAndLUKS(context.Background(), "test-vol", pi, 2048, nil, 100, 500)
	assert.NoError(t, err)
}
