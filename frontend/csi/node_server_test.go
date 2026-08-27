// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
		labels, err := plugin.getTopologyLabels(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", labels["topology.kubernetes.io/region"])
	})

	t.Run("caches the K8s fallback after the first successful lookup", func(t *testing.T) {
		topologyLabels = nil
		ctrl := gomock.NewController(t)
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
			Return(map[string]string{"zone": "z1"}, nil).Times(1)

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
		first, err := plugin.getTopologyLabels(context.Background())
		require.NoError(t, err)
		second, err := plugin.getTopologyLabels(context.Background())
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})

	t.Run("no controller helper and no cache returns nil", func(t *testing.T) {
		topologyLabels = nil
		plugin := &Plugin{nodeName: "node-a"}
		labels, err := plugin.getTopologyLabels(context.Background())
		require.NoError(t, err) // no topology source to consult is not a failed lookup
		assert.Nil(t, labels)
	})

	t.Run("retries transient lookup failures", func(t *testing.T) {
		topologyLabels = nil
		stubImmediateTopologyLabelsBackoff(t, 2)

		ctrl := gomock.NewController(t)
		expected := map[string]string{K8sTopologyRegionLabel: "us-west-2"}
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		gomock.InOrder(
			mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
				Return(nil, fmt.Errorf("temporary api error")),
			mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
				Return(nil, fmt.Errorf("temporary api error")),
			mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
				Return(expected, nil),
		)

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
		labels, err := plugin.getTopologyLabels(context.Background())
		require.NoError(t, err)
		assert.Equal(t, expected, labels)
	})

	t.Run("exhausted failures report unknown labels and do not initialize the cache", func(t *testing.T) {
		topologyLabels = nil
		stubImmediateTopologyLabelsBackoff(t, 2)

		ctrl := gomock.NewController(t)
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
			Return(nil, fmt.Errorf("temporary api error")).Times(3) // first attempt plus two retries

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
		labels, err := plugin.getTopologyLabels(context.Background())
		assert.Error(t, err)
		assert.Nil(t, labels)
		assert.Nil(t, topologyLabels)
	})

	t.Run("lookup failure falls back to labels another caller already cached", func(t *testing.T) {
		cached := map[string]string{K8sTopologyRegionLabel: "us-west-2"}
		topologyLabels = nil
		stubImmediateTopologyLabelsBackoff(t, 1)

		ctrl := gomock.NewController(t)
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		// Stands in for a second caller winning the lookup while this one is still retrying.
		mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
			DoAndReturn(func(context.Context, string) (map[string]string, error) {
				topologyLabelsLock.Lock()
				defer topologyLabelsLock.Unlock()
				topologyLabels = map[string]string{K8sTopologyRegionLabel: "us-west-2"}
				return nil, fmt.Errorf("temporary api error")
			}).Times(2) // first attempt plus one retry

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
		labels, err := plugin.getTopologyLabels(context.Background())
		require.NoError(t, err) // the labels are known, so reporting them is correct
		assert.Equal(t, cached, labels)
	})

	t.Run("permanent lookup failures are not retried", func(t *testing.T) {
		permanentErrors := map[string]error{
			"forbidden":    apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "node-a", fmt.Errorf("no RBAC")),
			"unauthorized": apierrors.NewUnauthorized("no credentials"),
			"not found":    apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, "node-a"),
		}

		for name, permanentErr := range permanentErrors {
			t.Run(name, func(t *testing.T) {
				topologyLabels = nil
				stubImmediateTopologyLabelsBackoff(t, 5)

				ctrl := gomock.NewController(t)
				mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
				mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
					Return(nil, permanentErr).Times(1) // retrying a misconfiguration only delays NodeGetInfo

				plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper}
				labels, err := plugin.getTopologyLabels(context.Background())
				assert.Error(t, err)
				assert.Nil(t, labels)
				assert.Nil(t, topologyLabels)
			})
		}
	})
}

// stubImmediateTopologyLabelsBackoff swaps in a zero-interval retry policy capped at maxRetries, so
// retry-path tests are decided by mocked call counts rather than by how long the backoff sleeps.
func stubImmediateTopologyLabelsBackoff(t *testing.T, maxRetries uint64) {
	t.Helper()

	productionBackoff := makeTopologyLabelsBackoff
	makeTopologyLabelsBackoff = func() backoff.BackOff {
		return backoff.WithMaxRetries(&backoff.ZeroBackOff{}, maxRetries)
	}

	t.Cleanup(func() {
		makeTopologyLabelsBackoff = productionBackoff
		topologyLabels = nil
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
	assert.Contains(t, err.Error(), "failed to stage volume")
	assert.Contains(t, err.Error(), "Trident is initializing")
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
		assert.Contains(t, err.Error(), "failed to unstage volume")
		assert.Contains(t, err.Error(), "Trident is initializing")
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
		assert.Contains(t, err.Error(), "failed to publish volume")
		assert.Contains(t, err.Error(), "Trident is initializing")
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
		assert.Contains(t, err.Error(), "failed to unpublish volume")
		assert.Contains(t, err.Error(), "Trident is initializing")
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
		assert.Contains(t, err.Error(), "failed to expand volume")
		assert.Contains(t, err.Error(), "Trident is initializing")
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
	t.Run("reports the topology labels read from the node", func(t *testing.T) {
		topologyLabels = nil
		t.Cleanup(func() { topologyLabels = nil })

		ctrl := gomock.NewController(t)
		mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
		mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
			Return(map[string]string{K8sTopologyRegionLabel: "us-east-1"}, nil)

		plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper, nodeOrchestrator: newReadyNodeCore(t)}
		resp, err := plugin.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
		require.NoError(t, err)
		assert.Equal(t, "node-a", resp.NodeId)
		require.NotNil(t, resp.AccessibleTopology)
		assert.Equal(t, map[string]string{K8sTopologyRegionLabel: "us-east-1"}, resp.AccessibleTopology.Segments)
	})

	t.Run("no controller helper reports no topology", func(t *testing.T) {
		topologyLabels = nil
		t.Cleanup(func() { topologyLabels = nil })

		plugin := &Plugin{nodeName: "node-a", nodeOrchestrator: newReadyNodeCore(t)}
		resp, err := plugin.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
		require.NoError(t, err)
		assert.Equal(t, "node-a", resp.NodeId)
		assert.NotNil(t, resp.AccessibleTopology)
	})

	// The registrar treats an error as retryable but would publish a CSINode with empty topologyKeys
	// if we answered OK, so an unreadable node must fail the call rather than report no topology.
	unknownLabelErrors := map[string]error{
		"exhausted retries": fmt.Errorf("temporary api error"),
		"forbidden":         apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "node-a", fmt.Errorf("no RBAC")),
	}

	for name, lookupErr := range unknownLabelErrors {
		t.Run("unavailable when labels are unknown: "+name, func(t *testing.T) {
			topologyLabels = nil
			stubImmediateTopologyLabelsBackoff(t, 1)

			ctrl := gomock.NewController(t)
			mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
			mockHelper.EXPECT().GetNodeTopologyLabels(gomock.Any(), "node-a").
				Return(nil, lookupErr).AnyTimes()

			plugin := &Plugin{nodeName: "node-a", controllerHelper: mockHelper, nodeOrchestrator: newReadyNodeCore(t)}
			resp, err := plugin.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
			assert.Nil(t, resp)
			require.Error(t, err)
			assert.Equal(t, codes.Unavailable, status.Code(err))
			assert.Nil(t, topologyLabels)
		})
	}
}
