// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

func testMountReq(targetPath string, readOnly bool, secrets map[string]string) MountRequest {
	return MountRequest{
		TargetPath: targetPath,
		ReadOnly:   readOnly,
		Secrets:    secrets,
	}
}

func TestMount_EmptyVolume_ReturnsError(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Mount(context.Background(), "", testMountReq("/target", false, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume is empty")
}

func TestMount_EmptyTargetPath_ReturnsError(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Mount(context.Background(), "vol1", testMountReq("", false, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target path is empty")
}

func TestMount_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.Mount(context.Background(), "vol1", testMountReq("/target", false, nil))
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestMount_AcquiresVolumeLock_SerializesConcurrentCalls(t *testing.T) {
	core, mocks := newTestCore(t)
	volume := "vol1"

	core.volumeLocks.Lock(volume)

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), volume).Return(nil, errors.New("read failed"))

	done := make(chan error, 1)
	go func() { done <- core.Mount(context.Background(), volume, testMountReq("/target", false, nil)) }()

	select {
	case <-done:
		t.Fatal("Mount returned before volume lock was released")
	case <-time.After(50 * time.Millisecond):
	}

	core.volumeLocks.Unlock(volume)

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read failed")
	case <-time.After(2 * time.Second):
		t.Fatal("Mount did not proceed after volume lock was released")
	}
}

func TestMount_ReadTrackingInfoError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(nil, errors.New("tracking file missing"))

	err := core.Mount(context.Background(), "vol1", testMountReq("/target", false, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tracking file missing")
}

func TestMount_TrackingNotFound_ReturnsPreconditionError(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("missing tracking file"))

	err := core.Mount(context.Background(), "vol1", testMountReq("/target", false, nil))
	require.Error(t, err)
	assert.True(t, errors.IsPreconditionError(err))
}

func TestMount_SingleNodeSingleWriterAlreadyPublishedElsewhere_ReturnsPreconditionError(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(ISCSI)
	trackingInfo.PublishedPaths = map[string]struct{}{"/other-target": {}}
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil)

	err := core.Mount(context.Background(), "vol1", MountRequest{
		TargetPath:             "/target",
		SingleNodeSingleWriter: true,
	})
	require.Error(t, err)
	assert.True(t, errors.IsPreconditionError(err))
	assert.Contains(t, err.Error(), snswAlreadyPublishedElsewhereMsg)
}

func TestIsVolumePublishedElsewhere(t *testing.T) {
	trackingInfo := &models.VolumeTrackingInfo{
		PublishedPaths: map[string]struct{}{"/path/a": {}},
	}
	assert.False(t, isVolumePublishedElsewhere(trackingInfo, "/path/a"))
	assert.True(t, isVolumePublishedElsewhere(trackingInfo, "/path/b"))
	assert.False(t, isVolumePublishedElsewhere(&models.VolumeTrackingInfo{}, "/path/b"))
}

func TestMount_ReadOnly_AppendsRoToMountOptions(t *testing.T) {
	core, mocks := newTestCore(t)

	trackingInfo := sampleTrackingInfo(NFS)
	trackingInfo.VolumePublishInfo.MountOptions = "rw"

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil)
	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)

	var captured *models.VolumePublishInfo
	mocks.Mount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), "/target", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, pi *models.VolumePublishInfo) error {
			captured = pi
			return nil
		})
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.Mount(context.Background(), "vol1", testMountReq("/target", true, nil))
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Contains(t, captured.MountOptions, "ro")
	assert.Contains(t, captured.MountOptions, "rw")
}

func TestMount_DispatchesToProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol models.StorageProtocol
		setup    func(mocks *testMocks)
	}{
		{
			name:     "NFS",
			protocol: NFS,
			setup: func(mocks *testMocks) {
				mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
				mocks.Mount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name:     "SMB",
			protocol: SMB,
			setup: func(mocks *testMocks) {
				mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
				mocks.Mount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name:     "FCP",
			protocol: FCP,
			setup: func(mocks *testMocks) {
				mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
				mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name:     "ISCSI",
			protocol: ISCSI,
			setup: func(mocks *testMocks) {
				mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
				mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name:     "NVMe",
			protocol: NVMe,
			setup: func(mocks *testMocks) {
				mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
				mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			trackingInfo := sampleTrackingInfo(tc.protocol)
			mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil)
			tc.setup(mocks)

			err := core.Mount(context.Background(), "vol1", testMountReq("/target", false, nil))
			assert.NoError(t, err)
		})
	}
}

func TestMount_UnknownProtocol_ReturnsError(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NFS)
	trackingInfo.VolumePublishInfo.StorageProtocol = models.StorageProtocol("bogus")
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil)

	err := core.Mount(context.Background(), "vol1", testMountReq("/target", false, nil))
	require.Error(t, err)
	assert.True(t, errors.IsPreconditionError(err))
}

func TestMountNFSVolume_AlreadyMounted_NoOp(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NFS)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, nil)
	// No AttachNFSVolume or AddPublishedPath expected.

	err := core.mountNFSVolume(context.Background(), "vol1", "/target", publishInfo)
	assert.NoError(t, err)
}

func TestMountNFSVolume_NotMountPoint_AttachesAndTracksPublishedPath(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NFS)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().AttachNFSVolume(gomock.Any(), publishInfo.InternalID, "/target", publishInfo).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountNFSVolume(context.Background(), "vol1", "/target", publishInfo)
	assert.NoError(t, err)
}

func TestMountNFSVolume_TargetPathDoesNotExist_CreatesAndAttaches(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NFS)
	targetPath := filepath.Join(t.TempDir(), "target")

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), targetPath).Return(false, os.ErrNotExist)
	mocks.Mount.EXPECT().AttachNFSVolume(gomock.Any(), publishInfo.InternalID, targetPath, publishInfo).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", targetPath).Return(nil)

	err := core.mountNFSVolume(context.Background(), "vol1", targetPath, publishInfo)
	require.NoError(t, err)

	info, statErr := os.Stat(targetPath)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestMountNFSVolume_IsLikelyNotMountPointError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NFS)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, errors.New("stat failed"))

	err := core.mountNFSVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat failed")
}

func TestMountNFSVolume_AttachError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NFS)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), "/target", gomock.Any()).Return(errors.New("attach failed"))

	err := core.mountNFSVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attach failed")
}

func TestMountNFSVolume_AddPublishedPathError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NFS)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), "/target", gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(errors.New("write failed"))

	err := core.mountNFSVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestMountSMBVolume_EmptyGlobalMount_ReturnsError(t *testing.T) {
	core, _ := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)
	publishInfo.GlobalMount = ""

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staging target not available")
}

func TestMountSMBVolume_AlreadyMounted_NoOp(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, nil)

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	assert.NoError(t, err)
}

func TestMountSMBVolume_NotMounted_BindMount(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().WindowsBindMount(gomock.Any(), publishInfo.GlobalMount, "/target", []string{"bind"}).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	assert.NoError(t, err)
}

func TestMountSMBVolume_ReadOnly_AppendsRoOption(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)
	publishInfo.MountOptions = "ro"

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().WindowsBindMount(gomock.Any(), publishInfo.GlobalMount, "/target", []string{"bind", "ro"}).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	assert.NoError(t, err)
}

func TestMountSMBVolume_TargetPathDoesNotExist_CreatesAndMounts(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)
	targetPath := filepath.Join(t.TempDir(), "target")

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), targetPath).Return(false, os.ErrNotExist)
	mocks.Mount.EXPECT().WindowsBindMount(gomock.Any(), publishInfo.GlobalMount, targetPath, []string{"bind"}).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", targetPath).Return(nil)

	err := core.mountSMBVolume(context.Background(), "vol1", targetPath, publishInfo)
	require.NoError(t, err)

	info, statErr := os.Stat(targetPath)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestMountSMBVolume_IsLikelyNotMountPointError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(false, errors.New("stat failed"))

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat failed")
}

func TestMountSMBVolume_WindowsBindMountError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), "/target", gomock.Any()).Return(errors.New("bind mount failed"))

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind mount failed")
}

// TestMountSMBVolume_AddPublishedPathError_Propagates is a regression test: mountSMBVolume must
// record targetPath via AddPublishedPath like every other protocol, since NodeGetVolumeStats,
// reconciliation, and unpublish bookkeeping all rely on PublishedPaths being accurate.
func TestMountSMBVolume_AddPublishedPathError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)

	mocks.Mount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), "/target").Return(true, nil)
	mocks.Mount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), "/target", gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(errors.New("write failed"))

	err := core.mountSMBVolume(context.Background(), "vol1", "/target", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

// TestMountFCPVolume_LUKS_LegacyDevicePath_ErrorsResolvingUnderlyingDevice covers the legacy
// LUKS device-path branch (luks.NewDeviceFromMappingPath): resolving the underlying device
// backing a legacy "/dev/mapper/luks-*" path is expected to fail in this unit-test environment
// (unsupported on darwin/windows; on linux the mocked Command returns an error), and that
// failure must propagate out of mountFCPVolume without ever reaching mountDeviceAtTargetPath.
func TestMountFCPVolume_LUKS_LegacyDevicePath_ErrorsResolvingUnderlyingDevice(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.LUKSEncryption = "true"
	publishInfo.DevicePath = "/dev/mapper/luks-test-volume"

	// Only exercised on linux; harmless if never called (darwin/windows).
	mocks.Command.EXPECT().
		ExecuteWithTimeoutAndInput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte{}, errors.New("cryptsetup status failed")).
		AnyTimes()

	err := core.mountFCPVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	require.Error(t, err)
}

func TestMountFCPVolume_LUKS_NonLegacyDevicePath_MountsMappedDevice(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.LUKSEncryption = "true"
	publishInfo.InternalID = "pvc-test"
	// Non-legacy (multipath) device path, and no secrets so ensureLUKSVolumePassphrase
	// short-circuits before ever touching CheckPassphrase/RotatePassphrase.
	publishInfo.DevicePath = "/dev/mapper/mpatha"

	expectedMappedPath := "/dev/mapper/luks-pvc-test"
	mocks.Mount.EXPECT().MountDevice(gomock.Any(), expectedMappedPath, "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountFCPVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	assert.NoError(t, err)
}

func TestMountFCPVolume_NonLUKS_MountsRawDevicePath(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.LUKSEncryption = "false"
	publishInfo.DevicePath = "/dev/sdz"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), "/dev/sdz", "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountFCPVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	assert.NoError(t, err)
}

func TestMountNVMeVolume_LUKS_MountsMappedDevice(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"
	publishInfo.InternalID = "pvc-nvme"
	publishInfo.DevicePath = "/dev/nvme0n1"

	expectedMappedPath := "/dev/mapper/luks-pvc-nvme"
	mocks.Mount.EXPECT().MountDevice(gomock.Any(), expectedMappedPath, "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountNVMeVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	assert.NoError(t, err)
}

func TestMountNVMeVolume_NonLUKS_MountsRawDevicePath(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "false"
	publishInfo.DevicePath = "/dev/nvme0n1"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), "/dev/nvme0n1", "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountNVMeVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	assert.NoError(t, err)
}

func TestMountDeviceAtTargetPath_RawBlock_BindMountsWithIsRawBlockTrue(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = filesystem.Raw
	publishInfo.MountOptions = "rw"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), "/dev/sdz", "/target", "rw,bind", gomock.Eq(true)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountDeviceAtTargetPath(context.Background(), "vol1", "/target", "/dev/sdz", publishInfo)
	require.NoError(t, err)
	assert.Equal(t, "rw,bind", publishInfo.MountOptions)
}

func TestMountDeviceAtTargetPath_RawBlock_NoExistingOptions_SetsBind(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = filesystem.Raw
	publishInfo.MountOptions = ""

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), "/dev/sdz", "/target", "bind", gomock.Eq(true)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountDeviceAtTargetPath(context.Background(), "vol1", "/target", "/dev/sdz", publishInfo)
	require.NoError(t, err)
	assert.Equal(t, "bind", publishInfo.MountOptions)
}

func TestMountDeviceAtTargetPath_RawBlock_MountDeviceError_WrappedAndPropagated(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = filesystem.Raw

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(true)).
		Return(errors.New("bind failed"))

	err := core.mountDeviceAtTargetPath(context.Background(), "vol1", "/target", "/dev/sdz", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to bind mount raw device")
	assert.Contains(t, err.Error(), "bind failed")
}

func TestMountDeviceAtTargetPath_RegularFilesystem_MountsWithIsRawBlockFalse(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = "ext4"
	publishInfo.MountOptions = "rw"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), "/dev/sdz", "/target", "rw", gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountDeviceAtTargetPath(context.Background(), "vol1", "/target", "/dev/sdz", publishInfo)
	assert.NoError(t, err)
}

func TestMountDeviceAtTargetPath_RegularFilesystem_MountDeviceError_WrappedAndPropagated(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = "ext4"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).
		Return(errors.New("mount failed"))

	err := core.mountDeviceAtTargetPath(context.Background(), "vol1", "/target", "/dev/sdz", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to mount device")
	assert.Contains(t, err.Error(), "mount failed")
}

func TestMountDeviceAtTargetPath_AddPublishedPathError_Propagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = "ext4"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(errors.New("tracking write failed"))

	err := core.mountDeviceAtTargetPath(context.Background(), "vol1", "/target", "/dev/sdz", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tracking write failed")
}

func TestMountISCSIVolume_LUKS_LegacyDevicePath_ErrorsResolvingUnderlyingDevice(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"
	publishInfo.DevicePath = "/dev/mapper/luks-test-volume"

	mocks.Command.EXPECT().
		ExecuteWithTimeoutAndInput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte{}, errors.New("cryptsetup status failed")).
		AnyTimes()

	err := core.mountISCSIVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	require.Error(t, err)
}

func TestMountISCSIVolume_LUKS_NonLegacyDevicePath_MountsMappedDevice(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"
	publishInfo.InternalID = "pvc-iscsi"
	publishInfo.DevicePath = "/dev/dm-3"

	expectedMappedPath := "/dev/mapper/luks-pvc-iscsi"
	mocks.Mount.EXPECT().MountDevice(gomock.Any(), expectedMappedPath, "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountISCSIVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	assert.NoError(t, err)
}

func TestMountISCSIVolume_NonLUKS_MountsMultipathDevicePath(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "false"
	publishInfo.DevicePath = "/dev/dm-3"

	mocks.Mount.EXPECT().MountDevice(gomock.Any(), "/dev/dm-3", "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountISCSIVolume(context.Background(), "vol1", "/target", publishInfo, nil)
	assert.NoError(t, err)
}

// TestMountISCSIVolume_LUKS_UsesSecretsParameterNotPublishInfoSecrets is a regression test for a
// bug where mountISCSIVolume read publishInfo.Secrets instead of the secrets parameter Mount()
// received from the caller. publishInfo.Secrets is tagged json:"-" and never survives a round
// trip through the tracking file, so reading it always produces an empty passphrase; if that
// regression reappears, ensureLUKSVolumePassphrase will short-circuit with "LUKS passphrase
// cannot be empty" without ever attempting to verify a passphrase, which this test disallows.
func TestMountISCSIVolume_LUKS_UsesSecretsParameterNotPublishInfoSecrets(t *testing.T) {
	hook := logtest.NewGlobal()
	defer hook.Reset()

	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"
	publishInfo.DevicePath = "/dev/dm-3"
	publishInfo.Secrets = nil // never populated on a real, deserialized tracking file

	secrets := map[string]string{"luks-passphrase": "current-secret", "luks-passphrase-name": "A"}

	// CheckPassphrase's cryptsetup invocation is: ctx, "cryptsetup", timeout, logOutput, stdin
	// (5 fixed params), then "open", device, luksDeviceName, "--type", "luks2", "--test-passphrase"
	// (6 variadic args) - 11 matchers total.
	mocks.Command.EXPECT().
		ExecuteWithTimeoutAndInput(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).
		Return([]byte{}, errors.New("cryptsetup unavailable in test")).
		AnyTimes()
	mocks.Mount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), "/target", gomock.Any(), gomock.Eq(false)).Return(nil)
	mocks.NodeHelper.EXPECT().AddPublishedPath(gomock.Any(), "vol1", "/target").Return(nil)

	err := core.mountISCSIVolume(context.Background(), "vol1", "/target", publishInfo, secrets)
	assert.NoError(t, err) // ensureLUKSVolumePassphrase failures are logged, not propagated

	for _, entry := range hook.AllEntries() {
		assert.NotContains(t, entry.Message, "LUKS passphrase cannot be empty")
	}
}
