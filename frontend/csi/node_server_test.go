// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	mock_controller_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	mock_node_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/pkg/locks"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestAttemptLock(t *testing.T) {
	ok := attemptLock(context.Background(), "TestAttemptLock", "lock-id-1", csiNodeLockTimeout)
	assert.True(t, ok, "expected to acquire an uncontended lock well within the timeout")
	locks.Unlock(context.Background(), "TestAttemptLock", "lock-id-1")
}

func TestPlugin_GetTopologyLabels(t *testing.T) {
	t.Run("gets labels from K8s discovery via controllerHelper", func(t *testing.T) {
		topologyLabels = nil // reset package-level fallback cache between tests
		ctrl := gomock.NewController(t)
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
			Return(map[string]string{"topology.kubernetes.io/region": "us-east-1"}, nil)

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
		labels := plugin.getTopologyLabels(context.Background())
		assert.Equal(t, "us-east-1", labels["topology.kubernetes.io/region"])
	})

	t.Run("caches the K8s fallback after the first successful lookup", func(t *testing.T) {
		topologyLabels = nil
		ctrl := gomock.NewController(t)
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
			Return(map[string]string{"zone": "z1"}, nil).Times(1)

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
		first := plugin.getTopologyLabels(context.Background())
		second := plugin.getTopologyLabels(context.Background())
		assert.Equal(t, first, second)
	})

	t.Run("no controller helper and no cache returns nil", func(t *testing.T) {
		topologyLabels = nil
		plugin := &Plugin{nodeName: "node-a"}
		assert.Nil(t, plugin.getTopologyLabels(context.Background()))
	})
}

func TestPlugin_NodeStageVolume_Validation(t *testing.T) {
	plugin := &Plugin{}

	testCases := []struct {
		name string
		req  *csi.NodeStageVolumeRequest
	}{
		{name: "nil request", req: nil},
		{name: "empty volume ID", req: &csi.NodeStageVolumeRequest{StagingTargetPath: "/tmp/x"}},
		{name: "empty target path", req: &csi.NodeStageVolumeRequest{VolumeId: "vol-1"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.NodeStageVolume(context.Background(), tc.req)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestPlugin_NodeStageVolume_ProtocolParsingErrors(t *testing.T) {
	plugin := &Plugin{}

	testCases := []struct {
		name           string
		publishContext map[string]string
	}{
		{
			name: "unknown protocol",
			publishContext: map[string]string{
				"protocol": "bogus",
			},
		},
		{
			name: "unsupported SAN type",
			publishContext: map[string]string{
				"protocol": string(tridentconfig.Block),
				"SANType":  "bogus",
			},
		},
		{
			name: "invalid iSCSI LUN number",
			publishContext: map[string]string{
				"protocol":       string(tridentconfig.Block),
				"SANType":        sa.ISCSI,
				"iscsiLunNumber": "not-a-number",
			},
		},
		{
			name: "missing volume capability for block stage",
			publishContext: map[string]string{
				"protocol":       string(tridentconfig.Block),
				"SANType":        sa.ISCSI,
				"filesystemType": "ext4",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &csi.NodeStageVolumeRequest{
				VolumeId:          "vol-1",
				StagingTargetPath: "/tmp/staging",
				PublishContext:    tc.publishContext,
			}
			_, err := plugin.NodeStageVolume(context.Background(), req)
			require.Error(t, err)
			assert.NotEqual(t, codes.OK, status.Code(err))
			if tc.name == "missing volume capability for block stage" {
				assert.Equal(t, codes.InvalidArgument, status.Code(err))
			}
		})
	}
}

func TestPlugin_NodeStageVolume_DelegatesToNodeOrchestrator(t *testing.T) {
	plugin := &Plugin{nodeOrchestrator: newNotReadyNodeCore()}

	req := &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-1",
		StagingTargetPath: "/tmp/staging",
		PublishContext: map[string]string{
			"protocol": string(tridentconfig.File),
		},
	}
	_, err := plugin.NodeStageVolume(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestPlugin_NodeUnstageVolume(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = plugin.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{VolumeId: "vol-1"})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("delegates to node orchestrator", func(t *testing.T) {
		plugin := &Plugin{nodeOrchestrator: newNotReadyNodeCore()}

		_, err := plugin.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
			VolumeId: "vol-1", StagingTargetPath: "/tmp/staging",
		})
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})
}

func TestPlugin_NodePublishVolume(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = plugin.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{VolumeId: "vol-1"})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("missing volume capability", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
			VolumeId:   "vol-1",
			TargetPath: "/tmp/target",
			PublishContext: map[string]string{
				"protocol": string(tridentconfig.Block),
			},
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("unknown protocol", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
			VolumeId:   "vol-1",
			TargetPath: "/tmp/target",
			VolumeCapability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("delegates to node orchestrator", func(t *testing.T) {
		plugin := &Plugin{nodeOrchestrator: newNotReadyNodeCore()}

		_, err := plugin.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
			VolumeId:   "vol-1",
			TargetPath: "/tmp/target",
			VolumeCapability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			PublishContext: map[string]string{
				"protocol": string(tridentconfig.Block),
			},
		})
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})
}

func TestHasSingleNodeSingleWriterAccessMode(t *testing.T) {
	req := &csi.NodePublishVolumeRequest{
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			},
		},
	}
	assert.True(t, hasSingleNodeSingleWriterAccessMode(req))
	assert.False(t, hasSingleNodeSingleWriterAccessMode(&csi.NodePublishVolumeRequest{}))
}

func TestPlugin_NodeUnpublishVolume(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = plugin.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{VolumeId: "vol-1"})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("delegates to node orchestrator", func(t *testing.T) {
		plugin := &Plugin{nodeOrchestrator: newNotReadyNodeCore()}

		_, err := plugin.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
			VolumeId: "vol-1", TargetPath: "/tmp/target",
		})
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})
}

func TestPlugin_NodeExpandVolume(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = plugin.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{VolumeId: "vol-1"})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("delegates to node orchestrator", func(t *testing.T) {
		plugin := &Plugin{nodeOrchestrator: newNotReadyNodeCore()}

		_, err := plugin.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
			VolumeId: "vol-1", VolumePath: "/tmp/target",
		})
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})
}

func TestPlugin_NodeGetVolumeStats(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		plugin := &Plugin{}
		_, err := plugin.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = plugin.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{VolumeId: "vol-1"})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("volume path not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockHelper := mock_node_helpers.NewMockNodeHelper(ctrl)
		mockHelper.EXPECT().VerifyVolumePath(gomock.Any(), "/tmp/vol").Return(errors.NotFoundError("no such path"))

		plugin := &Plugin{nodeHelper: mockHelper}
		_, err := plugin.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
			VolumeId: "vol-1", VolumePath: "/tmp/vol",
		})
		require.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("raw block volume stats", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockHelper := mock_node_helpers.NewMockNodeHelper(ctrl)
		trackingInfo := &models.VolumeTrackingInfo{}
		mockHelper.EXPECT().VerifyVolumePath(gomock.Any(), "/tmp/vol").Return(nil)
		mockHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol-1").Return(trackingInfo, nil)
		mockHelper.EXPECT().IsRawBlockVolume(trackingInfo).Return(true)
		mockHelper.EXPECT().GetBlockDeviceStatsByID(gomock.Any(), "vol-1", trackingInfo).
			Return(&nodehelpers.VolumeStats{Total: 100, Used: 40, Available: 60}, nil)

		plugin := &Plugin{nodeHelper: mockHelper}
		resp, err := plugin.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
			VolumeId: "vol-1", VolumePath: "/tmp/vol",
		})
		require.NoError(t, err)
		require.Len(t, resp.Usage, 1)
		assert.Equal(t, int64(100), resp.Usage[0].Total)
	})

	t.Run("filesystem volume stats include inodes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockHelper := mock_node_helpers.NewMockNodeHelper(ctrl)
		trackingInfo := &models.VolumeTrackingInfo{}
		mockHelper.EXPECT().VerifyVolumePath(gomock.Any(), "/tmp/vol").Return(nil)
		mockHelper.EXPECT().ReadTrackingInfo(gomock.Any(), "vol-1").Return(trackingInfo, nil)
		mockHelper.EXPECT().IsRawBlockVolume(trackingInfo).Return(false)
		mockHelper.EXPECT().GetFilesystemStatsByID(gomock.Any(), "/tmp/vol").
			Return(&nodehelpers.VolumeStats{Total: 100, Used: 40, Available: 60}, nil)

		plugin := &Plugin{nodeHelper: mockHelper}
		resp, err := plugin.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
			VolumeId: "vol-1", VolumePath: "/tmp/vol",
		})
		require.NoError(t, err)
		require.Len(t, resp.Usage, 2)
	})
}

func TestPlugin_NodeGetCapabilities(t *testing.T) {
	plugin := &Plugin{nsCap: []*csi.NodeServiceCapability{
		NewNodeServiceCapability(csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME),
	}}
	resp, err := plugin.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Capabilities, 1)
}

func TestPlugin_NodeGetInfo(t *testing.T) {
	plugin := &Plugin{nodeName: "node-a", nodeOrchestrator: newReadyNodeCore(t)}
	resp, err := plugin.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, "node-a", resp.NodeId)
	assert.NotNil(t, resp.AccessibleTopology)
}
