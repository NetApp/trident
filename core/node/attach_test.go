// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/pkg/locks/distlock"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestAttach_EmptyVolume(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Attach(context.Background(), "", AttachRequest{PublishInfo: samplePublishInfo(ISCSI)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID is empty")
}

func TestAttach_NilPublishInfo(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.Attach(context.Background(), "test-volume", AttachRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil publishInfo")
}

func TestAttach_UnknownProtocol(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)

	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.StorageProtocol = "bogus-protocol"

	err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: publishInfo})
	require.Error(t, err)
	assert.True(t, errors.IsUnsupportedError(err))
}

func TestAttach_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestAttach_TrackingInfoWriteFailure_ShortCircuits(t *testing.T) {
	core, mocks := newTestCore(t)

	// Only the initial write is expected; a failure there must short-circuit before protocol
	// dispatch and before the deferred write.
	mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("disk full"))

	err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestAttach_WritesTrackingInfoBeforeAndAfter(t *testing.T) {
	core, mocks := newTestCore(t)

	var writes []models.VolumePublishInfo
	mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), "test-volume", gomock.Any()).
		Times(2).
		DoAndReturn(func(_ context.Context, _ string, ti *models.VolumeTrackingInfo) error {
			writes = append(writes, ti.VolumePublishInfo)
			return nil
		})
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)

	err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	require.NoError(t, err)
	assert.Len(t, writes, 2, "expected one write before dispatch and one deferred write after")
}

// TestAttach_SharedTargetPreservedInBothTrackingWrites asserts that publishInfo.SharedTarget
// survives both tracking-file writes unchanged. Prior to removing the duplicate AttachRequest.SharedTarget
// field, a caller passing req.SharedTarget=false (zero value) could overwrite publishInfo.SharedTarget.
func TestAttach_SharedTargetPreservedInBothTrackingWrites(t *testing.T) {
	for _, protocol := range []models.StorageProtocol{NFS, ISCSI, FCP} {
		t.Run(string(protocol), func(t *testing.T) {
			core, mocks := newTestCore(t)
			publishInfo := samplePublishInfo(protocol)
			publishInfo.SharedTarget = true

			var writes []models.VolumePublishInfo
			mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), "test-volume", gomock.Any()).
				Times(2).
				DoAndReturn(func(_ context.Context, _ string, ti *models.VolumeTrackingInfo) error {
					writes = append(writes, ti.VolumePublishInfo)
					return nil
				})

			switch protocol {
			case NFS:
				mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			case ISCSI:
				mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
				mocks.ISCSI.EXPECT().EnsureVolumeFormattedAndMounted(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(nil)
				mocks.ISCSI.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
			case FCP:
				mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
				mocks.FCP.EXPECT().EnsureVolumeFormattedAndMounted(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(nil)
			}

			err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: publishInfo})
			require.NoError(t, err)
			require.Len(t, writes, 2, "expected two tracking-file writes")
			assert.True(t, writes[0].SharedTarget, "initial tracking write must preserve SharedTarget=true")
			assert.True(t, writes[1].SharedTarget, "deferred tracking write must preserve SharedTarget=true")
		})
	}
}

func TestAttach_DeferredWriteFailure_SurfacesError(t *testing.T) {
	core, mocks := newTestCore(t)

	gomock.InOrder(
		mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("second write failed")),
	)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)

	err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not write tracking file")
}

func TestAttach_DeferredWriteFailure_CombinesWithAttachmentError(t *testing.T) {
	core, mocks := newTestCore(t)

	gomock.InOrder(
		mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("second write failed")),
	)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(errors.New("bad fstype"))

	err := core.Attach(context.Background(), "test-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attachment failed")
	assert.Contains(t, err.Error(), "bad fstype")
	assert.Contains(t, err.Error(), "second write failed")
}

func TestAttach_VolumeLockSerializesSameVolume(t *testing.T) {
	core, mocks := newTestCore(t)

	started := make(chan struct{})
	release := make(chan struct{})
	var callCount int
	var mu sync.Mutex

	mocks.NodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, _ string) error {
			mu.Lock()
			callCount++
			first := callCount == 1
			mu.Unlock()
			if first {
				close(started)
				<-release
			}
			return nil
		},
	)

	done := make(chan error, 2)
	go func() {
		done <- core.Attach(context.Background(), "same-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first Attach never started")
	}

	go func() {
		done <- core.Attach(context.Background(), "same-volume", AttachRequest{PublishInfo: samplePublishInfo(NFS)})
	}()

	// The second call must be blocked on the volume lock while the first is in flight.
	select {
	case <-done:
		t.Fatal("second Attach completed before the first released the volume lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Attach calls did not both complete")
		}
	}
}

func TestAttachNFSVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), "ext4").Return(nil)

	err := core.attachNFSVolume(context.Background(), samplePublishInfo(NFS))
	assert.NoError(t, err)
}

func TestAttachNFSVolume_IsCompatibleError(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(errors.New("incompatible fs"))

	err := core.attachNFSVolume(context.Background(), samplePublishInfo(NFS))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible fs")
}

func TestAttachSMBVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)
	publishInfo.SMBADUser = "domain\\user"
	publishInfo.SMBADPass = "secret"

	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), publishInfo.FilesystemType).Return(nil)
	mocks.Mount.EXPECT().AttachSMBVolume(
		gomock.Any(), "test-volume", publishInfo.GlobalMount, publishInfo.SMBADUser, publishInfo.SMBADPass, publishInfo,
	).Return(nil)

	err := core.attachSMBVolume(context.Background(), "test-volume", publishInfo)
	assert.NoError(t, err)
}

func TestAttachSMBVolume_IsCompatibleError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(errors.New("incompatible fs"))

	err := core.attachSMBVolume(context.Background(), "test-volume", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible fs")
}

func TestAttachSMBVolume_AttachError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(SMB)
	mocks.Mount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
	mocks.Mount.EXPECT().AttachSMBVolume(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("smb mount failed"))

	err := core.attachSMBVolume(context.Background(), "test-volume", publishInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "smb mount failed")
}

func TestEnsureAttachISCSIVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, AttachISCSIVolumeTimeoutShort).
		Return(int64(0), nil)

	mpathSize, err := core.ensureAttachISCSIVolume(context.Background(), "test-volume", publishInfo, AttachISCSIVolumeTimeoutShort)
	require.NoError(t, err)
	assert.Equal(t, int64(0), mpathSize)
}

func TestEnsureAttachISCSIVolume_NonAuthErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
		Return(int64(0), errors.New("login timed out"))

	_, err := core.ensureAttachISCSIVolume(context.Background(), "test-volume", publishInfo, AttachISCSIVolumeTimeoutShort)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login timed out")
}

func TestEnsureAttachISCSIVolume_AuthErrorTriggersChapRetrySuccess(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	chapInfo := &models.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "chapuser",
		IscsiInitiatorSecret: "chapsecret",
	}

	gomock.InOrder(
		mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
			Return(int64(0), errors.AuthError("auth failed")),
		mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
			Return(int64(1073741824), nil),
	)
	mocks.Controller.MockChapClient.EXPECT().GetChap(gomock.Any(), "test-volume", "test-node").
		Return(chapInfo, nil)

	mpathSize, err := core.ensureAttachISCSIVolume(context.Background(), "test-volume", publishInfo, AttachISCSIVolumeTimeoutShort)
	require.NoError(t, err)
	assert.Equal(t, int64(1073741824), mpathSize)
	assert.Equal(t, *chapInfo, publishInfo.IscsiChapInfo)
}

func TestEnsureAttachISCSIVolume_AuthErrorChapLookupFails(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
		Return(int64(0), errors.AuthError("auth failed"))
	mocks.Controller.MockChapClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("controller unreachable"))

	_, err := core.ensureAttachISCSIVolume(context.Background(), "test-volume", publishInfo, AttachISCSIVolumeTimeoutShort)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not retrieve CHAP credentials")
}

func TestEnsureAttachISCSIVolume_AuthErrorRetryStillFails(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	gomock.InOrder(
		mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
			Return(int64(0), errors.AuthError("auth failed")),
		mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
			Return(int64(0), errors.New("still failing after CHAP retry")),
	)
	mocks.Controller.MockChapClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&models.IscsiChapInfo{UseCHAP: true}, nil)

	_, err := core.ensureAttachISCSIVolume(context.Background(), "test-volume", publishInfo, AttachISCSIVolumeTimeoutShort)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still failing after CHAP retry")
}

func TestAttachISCSIVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, AttachISCSIVolumeTimeoutShort).
		Return(int64(0), nil)
	mocks.ISCSI.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), publishInfo.InternalID, "", publishInfo, false, false,
	).Return(nil)
	mocks.ISCSI.EXPECT().AddSession(
		gomock.Any(), gomock.Any(), publishInfo, "test-volume", "", models.NotInvalid,
	)

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, nil, distlock.NewNoopLock())
	assert.NoError(t, err)
}

func TestAttachISCSIVolume_AttachError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("attach failed"))

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, nil, distlock.NewNoopLock())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attach failed")
}

func TestAttachISCSIVolume_FormatMountError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), nil)
	mocks.ISCSI.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("mkfs failed"))

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, nil, distlock.NewNoopLock())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkfs failed")
}

func TestAttachISCSIVolume_GratuitousResizeOnMpathSize(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.FilesystemType = "raw" // skip GetFilesystemSize in capturePreExpandSizeBaseline
	publishInfo.DevicePath = ""        // skip GetDiskSize in capturePreExpandSizeBaseline

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).
		Return(int64(2147483648), nil)
	mocks.ISCSI.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil)
	mocks.ISCSI.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
	// The gratuitous resize's own failure must not fail Attach overall (best-effort, logged as a warning).
	mocks.ISCSI.EXPECT().ExpandVolume(gomock.Any(), publishInfo, int64(2147483648)).
		Return(errors.New("resize failed"))

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, nil, distlock.NewNoopLock())
	assert.NoError(t, err)
}

func TestAttachISCSIVolume_LUKSPassphraseRotation(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "false" // avoid exercising real cryptsetup boundary via c.cmd/c.dev

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
	mocks.ISCSI.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false, false,
	).Return(nil)
	mocks.ISCSI.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	// LUKSEncryption "false" means convert.ToBool is false, so ensureLUKSVolumePassphrase must
	// not be invoked and no controller/luks calls should occur.
	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, map[string]string{}, distlock.NewNoopLock())
	assert.NoError(t, err)
}

// TestAttachISCSIVolume_LUKSLockDeleteFailed verifies that ErrLockDeleteFailed from the first
// WithLock call (format/map) is translated to an orchestrator InternalError so the distlock
// package does not leak into the CSI transport layer.
func TestAttachISCSIVolume_LUKSLockDeleteFailed(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)

	locker := lockerFunc(func(_ context.Context, _ func(context.Context) error) error {
		return distlock.ErrLockDeleteFailed
	})
	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, nil, locker)
	require.Error(t, err)
	assert.True(t, errors.IsInternalError(err), "expected InternalError, got: %v", err)
}

// TestAttachISCSIVolume_LUKSLockAcquireConflict verifies that ErrLockAcquireConflict from the
// first WithLock call is translated to a VolumeStateError (→ codes.Aborted) so the CO retries.
func TestAttachISCSIVolume_LUKSLockAcquireConflict(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)

	locker := lockerFunc(func(_ context.Context, _ func(context.Context) error) error {
		return distlock.ErrLockAcquireConflict
	})
	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, nil, locker)
	require.Error(t, err)
	assert.True(t, errors.IsVolumeStateError(err), "expected VolumeStateError, got: %v", err)
}

func TestAttachISCSIVolume_LUKSFormatErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
	// EnsureVolumeFormattedAndMounted / AddSession must never be reached: the LUKS format step
	// fails first because no passphrase was supplied in secrets.

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, map[string]string{}, distlock.NewNoopLock())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LUKS passphrase cannot be empty")
}

func TestEnsureAttachFCPVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)

	mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, AttachFCPVolumeTimeoutShort).
		Return(int64(0), nil)

	mpathSize, err := core.ensureAttachFCPVolume(context.Background(), publishInfo, AttachFCPVolumeTimeoutShort)
	require.NoError(t, err)
	assert.Equal(t, int64(0), mpathSize)
}

func TestEnsureAttachFCPVolume_Error(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)

	mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("fcp login failed"))

	_, err := core.ensureAttachFCPVolume(context.Background(), publishInfo, AttachFCPVolumeTimeoutShort)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fcp login failed")
	// FCP has no CHAP concept, so no controller calls should ever be made here.
}

func TestAttachFCPVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)

	mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).Return(int64(0), nil)
	mocks.FCP.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), publishInfo.InternalID, "", publishInfo, false, false,
	).Return(nil)

	err := core.attachFCPVolume(context.Background(), "test-volume", publishInfo, nil)
	assert.NoError(t, err)
}

func TestAttachFCPVolume_AttachError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)

	mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), errors.New("fcp attach failed"))

	err := core.attachFCPVolume(context.Background(), "test-volume", publishInfo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fcp attach failed")
}

func TestAttachFCPVolume_FormatMountError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)

	mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
	mocks.FCP.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("mkfs failed"))

	err := core.attachFCPVolume(context.Background(), "test-volume", publishInfo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkfs failed")
}

func TestAttachFCPVolume_GratuitousResizeFailureIsSwallowed(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(FCP)
	publishInfo.FilesystemType = "raw"
	publishInfo.DevicePath = ""

	mocks.FCP.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(1048576), nil)
	mocks.FCP.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil)
	mocks.FCP.EXPECT().IsAlreadyAttached(gomock.Any(), int(publishInfo.FCPLunNumber), publishInfo.FCTargetWWNN).
		Return(false)

	err := core.attachFCPVolume(context.Background(), "test-volume", publishInfo, nil)
	assert.NoError(t, err)
}

func TestAttachNVMeVolume_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).Return(nil)
	mocks.NVMe.EXPECT().EnsureCryptsetupFormattedAndMappedOnHost(
		gomock.Any(), publishInfo.InternalID, publishInfo, gomock.Nil(),
	).Return(false, false, nil)
	mocks.NVMe.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), publishInfo.InternalID, "", publishInfo, false, false,
	).Return(nil)
	mocks.NVMe.EXPECT().AddPublishedNVMeSession(gomock.Any(), publishInfo)

	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, nil)
	assert.NoError(t, err)
}

func TestAttachNVMeVolume_AttachError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("nvme connect failed"))

	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nvme connect failed")
}

func TestAttachNVMeVolume_CryptsetupError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NVMe.EXPECT().EnsureCryptsetupFormattedAndMappedOnHost(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(false, false, errors.New("cryptsetup failed"))

	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cryptsetup failed")
}

func TestAttachNVMeVolume_FormatMountError(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)

	mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NVMe.EXPECT().EnsureCryptsetupFormattedAndMappedOnHost(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(false, false, nil)
	mocks.NVMe.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("mkfs failed"))

	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkfs failed")
}

func TestAttachNVMeVolume_LUKSBranchSkippedWhenDisabled(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "false"

	mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NVMe.EXPECT().EnsureCryptsetupFormattedAndMappedOnHost(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(false, false, nil)
	mocks.NVMe.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil)
	mocks.NVMe.EXPECT().AddPublishedNVMeSession(gomock.Any(), publishInfo)

	// No controller.GetChap or luks calls expected since LUKSEncryption is false.
	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, nil)
	assert.NoError(t, err)
}
