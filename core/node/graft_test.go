// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// This file's tests must not use t.Parallel(); see core_test.go for why.

// sampleGraftRequest returns a minimal, valid GraftRequest for the Block protocol.
func sampleGraftRequest(_ string) GraftRequest {
	return GraftRequest{
		Protocol: tridentconfig.Block,
		VolumeAccessInfo: models.VolumeAccessInfo{
			IscsiAccessInfo: models.IscsiAccessInfo{
				IscsiTargetIQN:    "iqn.1992-08.com.netapp:sn.test:vs.test",
				IscsiTargetPortal: "192.0.2.1:3260",
				IscsiPortals:      []string{"192.0.2.1:3260"},
				IscsiLunNumber:    0,
			},
		},
	}
}

func TestCore_Graft_EmptyVolumeID(t *testing.T) {
	core, _ := newTestCore(t)

	resp, err := core.Graft(context.Background(), "", GraftRequest{})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsInvalidInputError(err))
}

func TestCore_Graft_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	resp, err := core.Graft(context.Background(), "vol1", sampleGraftRequest("vol1"))
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestCore_Graft_VolumeLock(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found")).AnyTimes()

	core.volumeLocks.Lock("vol1")

	done := make(chan struct{})
	go func() {
		_, _ = core.Graft(context.Background(), "vol1", GraftRequest{Protocol: tridentconfig.File})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Graft proceeded before the volume lock was released")
	case <-time.After(50 * time.Millisecond):
	}

	core.volumeLocks.Unlock("vol1")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Graft did not proceed after the volume lock was released")
	}
}

func TestCore_Graft_NoTrackingInfo_ProceedsWithFreshPublishInfo(t *testing.T) {
	core, mocks := newTestCore(t)
	req := sampleGraftRequest("vol1")

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))
	mocks.ISCSI.EXPECT().GraftAttachmentRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("attachment retry boom"))

	resp, err := core.Graft(context.Background(), "vol1", req)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attachment retry boom")
}

func TestCore_Graft_ReadTrackingInfoErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	req := sampleGraftRequest("vol1")

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.New("disk on fire"))

	resp, err := core.Graft(context.Background(), "vol1", req)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestCore_Graft_TrackingInfoMismatch(t *testing.T) {
	tests := map[string]struct {
		mutateReq   func(req *GraftRequest)
		errContains string
	}{
		"lun number mismatch": {
			mutateReq:   func(req *GraftRequest) { req.IscsiLunNumber = 99 },
			errContains: "lun number mismatch",
		},
		"target IQN mismatch": {
			mutateReq:   func(req *GraftRequest) { req.IscsiTargetIQN = "iqn.other" },
			errContains: "target IQN mismatch",
		},
		"no portals specified": {
			mutateReq:   func(req *GraftRequest) { req.IscsiPortals = nil },
			errContains: "no portals specified",
		},
		"no target portal specified": {
			mutateReq:   func(req *GraftRequest) { req.IscsiTargetPortal = "" },
			errContains: "no target portal specified",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			req := sampleGraftRequest("vol1")
			tt.mutateReq(&req)

			mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
				Return(sampleTrackingInfo(ISCSI), nil)

			resp, err := core.Graft(context.Background(), "vol1", req)
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.True(t, errors.IsTerminalReconciliationError(err))
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCore_Graft_ProtocolDispatch_File(t *testing.T) {
	core, mocks := newTestCore(t)
	req := sampleGraftRequest("vol1")
	req.Protocol = tridentconfig.File

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))

	resp, err := core.Graft(context.Background(), "vol1", req)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsTerminalReconciliationError(err))
	assert.Contains(t, err.Error(), "operation not supported")
}

func TestCore_Graft_ProtocolDispatch_Default(t *testing.T) {
	core, mocks := newTestCore(t)
	req := sampleGraftRequest("vol1")
	req.Protocol = "unknown-protocol"

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))

	resp, err := core.Graft(context.Background(), "vol1", req)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsTerminalReconciliationError(err))
	assert.Contains(t, err.Error(), "operation not supported")
}

func TestCore_Graft_Block_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	t.Cleanup(func() {
		publishedISCSISessions = models.NewISCSISessions()
	})

	req := sampleGraftRequest("vol1")

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))
	attachInfo := &models.AttachmentInfo{
		VolumePublishInfo: &models.VolumePublishInfo{
			VolumeAccessInfo: req.VolumeAccessInfo,
		},
	}
	mocks.ISCSI.EXPECT().GraftAttachmentRetry(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(attachInfo, nil)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).Return(nil)
	mocks.ISCSI.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), "vol1", "", models.NotInvalid)

	resp, err := core.Graft(context.Background(), "vol1", req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "vol1", resp.VolumeName)
	assert.Equal(t, tridentconfig.Block, resp.Protocol)
}

func TestGraftISCSIAttachment_NilPublishInfo(t *testing.T) {
	core, _ := newTestCore(t)
	req := sampleGraftRequest("vol1")

	resp, err := core.graftISCSIAttachment(context.Background(), "vol1", req, nil)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsTerminalReconciliationError(err))
	assert.Contains(t, err.Error(), "publish info is nil")
}

func TestGraftISCSIAttachment_CHAPDecryptFailure(t *testing.T) {
	core, _ := newTestCore(t, WithAESKey([]byte("0123456789abcdef")))
	req := sampleGraftRequest("vol1")
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.UseCHAP = true
	publishInfo.IscsiUsername = "!!!not-valid-base64!!!"

	resp, err := core.graftISCSIAttachment(context.Background(), "vol1", req, publishInfo)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error decrypting iscsi username")
}

func TestGraftISCSIAttachment_GraftAttachmentRetryErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	req := sampleGraftRequest("vol1")
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().GraftAttachmentRetry(gomock.Any(), publishInfo, gomock.Any()).
		Return(nil, errors.New("attachment retry failed"))

	resp, err := core.graftISCSIAttachment(context.Background(), "vol1", req, publishInfo)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attachment retry failed")
}

func TestGraftISCSIAttachment_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	t.Cleanup(func() {
		publishedISCSISessions = models.NewISCSISessions()
	})

	req := sampleGraftRequest("vol1")
	publishInfo := samplePublishInfo(ISCSI)

	newAccessInfo := models.VolumeAccessInfo{
		IscsiAccessInfo: models.IscsiAccessInfo{
			IscsiTargetIQN:    "iqn.1992-08.com.netapp:sn.test:vs.test",
			IscsiTargetPortal: "192.0.2.2:3260",
			IscsiPortals:      []string{"192.0.2.2:3260"},
		},
	}
	attachInfo := &models.AttachmentInfo{
		VolumePublishInfo: &models.VolumePublishInfo{VolumeAccessInfo: newAccessInfo},
	}
	mocks.ISCSI.EXPECT().GraftAttachmentRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(attachInfo, nil)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).Return(nil)
	mocks.ISCSI.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), "vol1", "", models.NotInvalid)

	resp, err := core.graftISCSIAttachment(context.Background(), "vol1", req, publishInfo)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "vol1", resp.VolumeName)
	assert.Equal(t, tridentconfig.Block, resp.Protocol)
	assert.Equal(t, newAccessInfo, resp.VolumeAccessInfo)
}

func TestGraftISCSIAttachment_UpdatePublishInfoErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	req := sampleGraftRequest("vol1")
	publishInfo := samplePublishInfo(ISCSI)

	attachInfo := &models.AttachmentInfo{
		VolumePublishInfo: &models.VolumePublishInfo{VolumeAccessInfo: req.VolumeAccessInfo},
	}
	mocks.ISCSI.EXPECT().GraftAttachmentRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(attachInfo, nil)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).
		Return(errors.New("could not write tracking file"))

	resp, err := core.graftISCSIAttachment(context.Background(), "vol1", req, publishInfo)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not write tracking file")
}

func TestDecryptCHAPAccessInfo_NilAccessInfo(t *testing.T) {
	core, _ := newTestCore(t)
	assert.NoError(t, core.decryptCHAPAccessInfo(context.Background(), nil))
}

func TestDecryptCHAPAccessInfo_NoCHAP_NoOp(t *testing.T) {
	core, _ := newTestCore(t)
	accessInfo := &models.VolumeAccessInfo{
		IscsiAccessInfo: models.IscsiAccessInfo{
			IscsiChapInfo: models.IscsiChapInfo{UseCHAP: false, IscsiUsername: "!!!garbage!!!"},
		},
	}

	err := core.decryptCHAPAccessInfo(context.Background(), accessInfo)
	require.NoError(t, err)
	// Untouched: no decryption should have been attempted.
	assert.Equal(t, "!!!garbage!!!", accessInfo.IscsiUsername)
}

func TestDecryptCHAPAccessInfo_DecryptFailures(t *testing.T) {
	key := []byte("0123456789abcdef")

	encrypt := func(t *testing.T, plaintext string) string {
		t.Helper()
		ciphertext, err := crypto.EncryptStringWithAES(plaintext, key)
		require.NoError(t, err)
		return ciphertext
	}

	const garbage = "!!!not-valid-base64!!!"

	tests := map[string]struct {
		buildAccessInfo func(t *testing.T) *models.VolumeAccessInfo
		errContains     string
	}{
		"initiator username fails": {
			buildAccessInfo: func(t *testing.T) *models.VolumeAccessInfo {
				return &models.VolumeAccessInfo{IscsiAccessInfo: models.IscsiAccessInfo{
					IscsiChapInfo: models.IscsiChapInfo{UseCHAP: true, IscsiUsername: garbage},
				}}
			},
			errContains: "error decrypting iscsi username",
		},
		"initiator secret fails": {
			buildAccessInfo: func(t *testing.T) *models.VolumeAccessInfo {
				return &models.VolumeAccessInfo{IscsiAccessInfo: models.IscsiAccessInfo{
					IscsiChapInfo: models.IscsiChapInfo{
						UseCHAP:              true,
						IscsiUsername:        encrypt(t, "user"),
						IscsiInitiatorSecret: garbage,
					},
				}}
			},
			errContains: "error decrypting initiator secret",
		},
		"target username fails": {
			buildAccessInfo: func(t *testing.T) *models.VolumeAccessInfo {
				return &models.VolumeAccessInfo{IscsiAccessInfo: models.IscsiAccessInfo{
					IscsiChapInfo: models.IscsiChapInfo{
						UseCHAP:              true,
						IscsiUsername:        encrypt(t, "user"),
						IscsiInitiatorSecret: encrypt(t, "secret"),
						IscsiTargetUsername:  garbage,
					},
				}}
			},
			errContains: "error decrypting target username",
		},
		"target secret fails": {
			buildAccessInfo: func(t *testing.T) *models.VolumeAccessInfo {
				return &models.VolumeAccessInfo{IscsiAccessInfo: models.IscsiAccessInfo{
					IscsiChapInfo: models.IscsiChapInfo{
						UseCHAP:              true,
						IscsiUsername:        encrypt(t, "user"),
						IscsiInitiatorSecret: encrypt(t, "secret"),
						IscsiTargetUsername:  encrypt(t, "targetUser"),
						IscsiTargetSecret:    garbage,
					},
				}}
			},
			errContains: "error decrypting target secret",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			core, _ := newTestCore(t, WithAESKey(key))
			accessInfo := tt.buildAccessInfo(t)

			err := core.decryptCHAPAccessInfo(context.Background(), accessInfo)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestDecryptCHAPAccessInfo_Success(t *testing.T) {
	key := []byte("0123456789abcdef")
	core, _ := newTestCore(t, WithAESKey(key))

	encrypt := func(plaintext string) string {
		ciphertext, err := crypto.EncryptStringWithAES(plaintext, key)
		require.NoError(t, err)
		return ciphertext
	}

	accessInfo := &models.VolumeAccessInfo{IscsiAccessInfo: models.IscsiAccessInfo{
		IscsiChapInfo: models.IscsiChapInfo{
			UseCHAP:              true,
			IscsiUsername:        encrypt("initiatorUser"),
			IscsiInitiatorSecret: encrypt("initiatorSecret"),
			IscsiTargetUsername:  encrypt("targetUser"),
			IscsiTargetSecret:    encrypt("targetSecret"),
		},
	}}

	err := core.decryptCHAPAccessInfo(context.Background(), accessInfo)
	require.NoError(t, err)
	assert.Equal(t, "initiatorUser", accessInfo.IscsiUsername)
	assert.Equal(t, "initiatorSecret", accessInfo.IscsiInitiatorSecret)
	assert.Equal(t, "targetUser", accessInfo.IscsiTargetUsername)
	assert.Equal(t, "targetSecret", accessInfo.IscsiTargetSecret)
}
