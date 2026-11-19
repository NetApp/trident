// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// This file's tests must not use t.Parallel(); see core_test.go for why.

func TestCore_ReconcileAttachment_EmptyVolumeID(t *testing.T) {
	core, _ := newTestCore(t)

	err := core.ReconcileAttachment(context.Background(), "", ReconcileAttachmentRequest{
		TargetIPs: []string{"192.0.2.11"},
	})

	require.Error(t, err)
	assert.True(t, errors.IsInvalidInputError(err))
}

func TestCore_ReconcileAttachment_NotReady_ReturnsErrorImmediately(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.ReconcileAttachment(context.Background(), "vol1", ReconcileAttachmentRequest{
		TargetIPs: []string{"192.0.2.11"},
	})

	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
}

func TestCore_ReconcileAttachment_NVMePrunesThenGrafts(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NVMe)
	trackingInfo.NVMeTargetIPs = []string{"192.0.2.10"}
	desired := []string{"192.0.2.10", "192.0.2.11"}

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil).Times(2)
	mocks.NVMeDriver.EXPECT().Prune(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).Return(nil)
	mocks.NVMeDriver.EXPECT().Graft(gomock.Any(), gomock.Any(), nvmeTargets(desired)).DoAndReturn(echoGraftedState)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, publishInfo *models.VolumePublishInfo) error {
			assert.Equal(t, trackingInfo.NVMeSubsystemNQN, publishInfo.NVMeSubsystemNQN)
			assert.Equal(t, desired, publishInfo.NVMeTargetIPs)
			return nil
		})

	err := core.ReconcileAttachment(context.Background(), "vol1", ReconcileAttachmentRequest{
		TargetIPs: desired,
	})

	require.NoError(t, err)
}

func TestCore_ReconcileAttachment_EmptyTargetSetPrunesWithoutGraft(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NVMe)
	trackingInfo.NVMeTargetIPs = []string{"192.0.2.10", "192.0.2.11"}

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil)
	mocks.NVMeDriver.EXPECT().Prune(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, publishInfo *models.VolumePublishInfo) error {
			assert.Empty(t, publishInfo.NVMeTargetIPs)
			return nil
		})

	err := core.ReconcileAttachment(context.Background(), "vol1", ReconcileAttachmentRequest{})

	require.NoError(t, err)
}

func TestCore_ReconcileAttachment_MissingSubsystemPrunesThenGrafts(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NVMe)
	trackingInfo.NVMeTargetIPs = []string{"192.0.2.10"}
	desired := []string{"192.0.2.11"}

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil).Times(2)
	mocks.NVMeDriver.EXPECT().Prune(gomock.Any(), gomock.Any(), nvmeTargets(desired)).Return(nil)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, publishInfo *models.VolumePublishInfo) error {
			assert.Equal(t, trackingInfo.NVMeSubsystemNQN, publishInfo.NVMeSubsystemNQN)
			assert.Equal(t, desired, publishInfo.NVMeTargetIPs)
			return nil
		})
	mocks.NVMeDriver.EXPECT().Graft(gomock.Any(), gomock.Any(), nvmeTargets(desired)).DoAndReturn(echoGraftedState)
	mocks.NodeHelper.EXPECT().UpdatePublishInfo(gomock.Any(), "vol1", gomock.Any()).Return(nil)

	err := core.ReconcileAttachment(context.Background(), "vol1", ReconcileAttachmentRequest{
		TargetIPs: desired,
	})

	require.NoError(t, err)
}

func TestCore_ReconcileAttachment_PruneErrorSkipsGraft(t *testing.T) {
	core, mocks := newTestCore(t)
	trackingInfo := sampleTrackingInfo(NVMe)

	mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").Return(trackingInfo, nil)
	mocks.NVMeDriver.EXPECT().Prune(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("subsystem lookup failed"))

	err := core.ReconcileAttachment(context.Background(), "vol1", ReconcileAttachmentRequest{
		TargetIPs: []string{"192.0.2.11"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "subsystem lookup failed")
}

func TestCore_ReconcileAttachment_IgnoresNonNVMeVolumes(t *testing.T) {
	tests := map[string]struct {
		trackingInfo *models.VolumeTrackingInfo
		trackingErr  error
	}{
		"iSCSI volume": {trackingInfo: sampleTrackingInfo(ISCSI)},
		"untracked":    {trackingErr: errors.NotFoundError("not found")},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			mocks.NodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol1").
				Return(tt.trackingInfo, tt.trackingErr).Times(2)

			err := core.ReconcileAttachment(context.Background(), "vol1", ReconcileAttachmentRequest{
				TargetIPs: []string{"192.0.2.11"},
			})

			require.NoError(t, err)
		})
	}
}
