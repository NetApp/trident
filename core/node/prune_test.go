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
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// This file's tests must not use t.Parallel(); see core_test.go for why.

// samplePruneRequest returns a minimal, valid PruneRequest for the Block protocol.
func samplePruneRequest(_ string) PruneRequest {
	return PruneRequest{
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

func TestCore_Prune_EmptyVolumeID(t *testing.T) {
	core, _ := newTestCore(t)

	resp, err := core.Prune(context.Background(), "", PruneRequest{})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsInvalidInputError(err))
}

func TestCore_Prune_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	resp, err := core.Prune(context.Background(), "vol1", samplePruneRequest("vol1"))
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestCore_Prune_VolumeLock(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found")).AnyTimes()

	core.volumeLocks.Lock("vol1")

	done := make(chan struct{})
	go func() {
		_, _ = core.Prune(context.Background(), "vol1", PruneRequest{Protocol: tridentconfig.File})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Prune proceeded before the volume lock was released")
	case <-time.After(50 * time.Millisecond):
	}

	core.volumeLocks.Unlock("vol1")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Prune did not proceed after the volume lock was released")
	}
}

func TestCore_Prune_ReadTrackingInfoNotFound_ToleratedNotAnError(t *testing.T) {
	core, mocks := newTestCore(t)
	t.Cleanup(func() {
		publishedISCSISessions = models.NewISCSISessions()
	})
	req := samplePruneRequest("vol1")

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))
	attachInfo := &models.AttachmentInfo{
		VolumePublishInfo: &models.VolumePublishInfo{VolumeAccessInfo: req.VolumeAccessInfo},
	}
	mocks.ISCSI.EXPECT().PruneAttachmentRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(attachInfo, nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())

	resp, err := core.Prune(context.Background(), "vol1", req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestCore_Prune_ProtocolDispatch_File(t *testing.T) {
	core, mocks := newTestCore(t)
	req := samplePruneRequest("vol1")
	req.Protocol = tridentconfig.File

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))

	resp, err := core.Prune(context.Background(), "vol1", req)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsTerminalReconciliationError(err))
	assert.Contains(t, err.Error(), "operation not supported")
}

func TestCore_Prune_ProtocolDispatch_Default(t *testing.T) {
	core, mocks := newTestCore(t)
	req := samplePruneRequest("vol1")
	req.Protocol = "unknown-protocol"

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(nil, errors.NotFoundError("not found"))

	resp, err := core.Prune(context.Background(), "vol1", req)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsTerminalReconciliationError(err))
	assert.Contains(t, err.Error(), "operation not supported")
}

func TestCore_Prune_Block_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	t.Cleanup(func() {
		publishedISCSISessions = models.NewISCSISessions()
	})
	req := samplePruneRequest("vol1")

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
		Return(sampleTrackingInfo(ISCSI), nil)
	attachInfo := &models.AttachmentInfo{
		VolumePublishInfo: &models.VolumePublishInfo{VolumeAccessInfo: req.VolumeAccessInfo},
	}
	mocks.ISCSI.EXPECT().PruneAttachmentRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(attachInfo, nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())

	resp, err := core.Prune(context.Background(), "vol1", req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "vol1", resp.VolumeName)
	assert.Equal(t, tridentconfig.Block, resp.Protocol)
	assert.Equal(t, req.VolumeAccessInfo, resp.VolumeAccessInfo)
}

func TestPruneISCSIAttachment_NilPublishInfo(t *testing.T) {
	core, _ := newTestCore(t)
	req := samplePruneRequest("vol1")

	resp, err := core.pruneISCSIAttachment(context.Background(), "vol1", req, nil)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.IsTerminalReconciliationError(err))
	assert.Contains(t, err.Error(), "publish info is nil")
}

func TestPruneISCSIAttachment_TrackingInfoMismatch(t *testing.T) {
	tests := map[string]struct {
		mutateReq   func(req *PruneRequest)
		errContains string
	}{
		"lun number mismatch": {
			mutateReq:   func(req *PruneRequest) { req.IscsiLunNumber = 99 },
			errContains: "lun number mismatch",
		},
		"target IQN mismatch": {
			mutateReq:   func(req *PruneRequest) { req.IscsiTargetIQN = "iqn.other" },
			errContains: "target IQN mismatch",
		},
		"no portals specified": {
			mutateReq:   func(req *PruneRequest) { req.IscsiPortals = nil },
			errContains: "no portals specified",
		},
		"no target portal specified": {
			mutateReq:   func(req *PruneRequest) { req.IscsiTargetPortal = "" },
			errContains: "no target portal specified",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			core, _ := newTestCore(t)
			req := samplePruneRequest("vol1")
			publishInfo := samplePublishInfo(ISCSI)
			tt.mutateReq(&req)

			resp, err := core.pruneISCSIAttachment(context.Background(), "vol1", req, publishInfo)
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.True(t, errors.IsTerminalReconciliationError(err))
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestPruneISCSIAttachment_PruneAttachmentRetryErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	req := samplePruneRequest("vol1")
	publishInfo := samplePublishInfo(ISCSI)

	mocks.ISCSI.EXPECT().PruneAttachmentRetry(gomock.Any(), publishInfo, gomock.Any()).
		Return(nil, errors.New("prune retry failed"))

	resp, err := core.pruneISCSIAttachment(context.Background(), "vol1", req, publishInfo)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prune retry failed")
}

func TestPruneISCSIAttachment_Success(t *testing.T) {
	core, mocks := newTestCore(t)
	t.Cleanup(func() {
		publishedISCSISessions = models.NewISCSISessions()
	})

	req := samplePruneRequest("vol1")
	publishInfo := samplePublishInfo(ISCSI)

	attachInfo := &models.AttachmentInfo{
		VolumePublishInfo: &models.VolumePublishInfo{VolumeAccessInfo: publishInfo.VolumeAccessInfo},
	}
	mocks.ISCSI.EXPECT().PruneAttachmentRetry(gomock.Any(), publishInfo, gomock.Any()).Return(attachInfo, nil)
	mocks.ISCSI.EXPECT().RemovePortalsFromSession(gomock.Any(), attachInfo.VolumePublishInfo, gomock.Any())

	resp, err := core.pruneISCSIAttachment(context.Background(), "vol1", req, publishInfo)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "vol1", resp.VolumeName)
	assert.Equal(t, tridentconfig.Block, resp.Protocol)
	assert.Equal(t, req.VolumeAccessInfo, resp.VolumeAccessInfo)
}
