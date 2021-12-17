// Copyright 2021 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockhelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_helpers"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

var ctx = context.Background()

func generateController(mockOrchestrator *mockcore.MockOrchestrator, mockHelper *mockhelpers.MockHybridPlugin) *Plugin {
	controllerServer := &Plugin{
		orchestrator: mockOrchestrator,
		name:         Provisioner,
		nodeName:     "foo",
		version:      tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:     "bar",
		role:         CSIController,
		helper:       mockHelper,
		opCache:      sync.Map{},
	}
	return controllerServer
}

func generateFakePublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	req := &csi.ControllerPublishVolumeRequest{
		VolumeId: "volumeID",
		NodeId:   "nodeId",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	return req
}

func generateFakeUnpublishVolumeRequest() *csi.ControllerUnpublishVolumeRequest {
	req := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "volumeID",
		NodeId:   "nodeId",
	}
	return req
}

func generateFakeNode(nodeID string) *utils.Node {
	fakeNode := &utils.Node{
		Name: nodeID,
	}
	return fakeNode
}

func generateFakeVolumeExternal(volumeID string) *storage.VolumeExternal {
	fakeVolumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name: volumeID,
		},
		Backend:     "backend",
		BackendUUID: "backendUUID",
		Pool:        "pool",
		Orphaned:    false,
		State:       storage.VolumeStateOnline,
	}
	return fakeVolumeExternal
}

func TestControllerPublishVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	// Mimic not finding the volume publication
	mockOrchestrator.EXPECT().GetVolumePublication(ctx, req.VolumeId, req.NodeId).Return(nil,
		utils.NotFoundError("not found"))
	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Return(nil)
	// Verify we record the volume publication
	mockOrchestrator.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(nil)

	_, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.Nilf(t, err, "unexpected error publishing volume; %v", err)
}

// TestControllerPublishVolumeExistsWithSameOptions verifies behavior when a publich call comes in for the same
// volume/node with the same publish options
func TestControllerPublishVolumeExistsWithSameOptions(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	expectedPublication := generateVolumePublicationFromCSIPublishRequest(req)
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	// Mimic finding the volume publication
	mockOrchestrator.EXPECT().GetVolumePublication(ctx, req.VolumeId, req.NodeId).Return(expectedPublication, nil)
	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Return(nil)
	// Verify we don't rerecord the volume publication
	mockOrchestrator.EXPECT().AddVolumePublication(ctx, gomock.Any()).Times(0)

	_, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.Nil(t, err, "unexpected error publishing volume")
}

// TestControllerPublishVolumeExistsWithDifferentOptions verifies behavior when a publich call comes in for the same
// volume/node with different publish options
func TestControllerPublishVolumeExistsWithDifferentOptions(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	foundPublication := generateVolumePublicationFromCSIPublishRequest(req)
	foundPublication.ReadOnly = !foundPublication.ReadOnly
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	// Mimic finding the volume publication
	mockOrchestrator.EXPECT().GetVolumePublication(ctx, req.VolumeId, req.NodeId).Return(foundPublication, nil)
	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	// Verify we don't publish the volume
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Times(0)
	// Verify we don't rerecord the volume publication
	mockOrchestrator.EXPECT().AddVolumePublication(ctx, gomock.Any()).Times(0)

	_, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.NotNil(t, err, "unexpected success publishing volume")
}

func TestControllerPublishVolumePublishingError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	// Mimic not finding the volume publication
	mockOrchestrator.EXPECT().GetVolumePublication(ctx, req.VolumeId, req.NodeId).Return(nil,
		utils.NotFoundError("not found"))
	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	// Simulate an error during volume publishing
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Return(fmt.Errorf("some error"))
	// Verify we still record the volume publication if publishing fails
	mockOrchestrator.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(nil)

	_, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.NotNil(t, err, "unexpected success publishing volume")
}

func TestControllerPublishVolumeTVPError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	// Mimic not finding the volume publication
	mockOrchestrator.EXPECT().GetVolumePublication(ctx, req.VolumeId, req.NodeId).Return(nil,
		utils.NotFoundError("not found"))
	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	// Verify we do not publish the volume
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Times(0)
	// Simulate an error during recording of volume publication
	mockOrchestrator.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(fmt.Errorf("some error"))

	_, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.NotNil(t, err, "unexpected success publishing volume")
}

func TestControllerUnpublishVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakeUnpublishVolumeRequest()
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	mockOrchestrator.EXPECT().UnpublishVolume(ctx, req.VolumeId, gomock.Any()).Return(nil)
	// Verify we remove the volume publication on a successful volume unpublish
	mockOrchestrator.EXPECT().DeleteVolumePublication(ctx, req.VolumeId, req.NodeId).Return(nil)

	_, err := controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Nil(t, err, "unexpected error unpublishing volume")
}

func TestControllerUnpublishVolumeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakeUnpublishVolumeRequest()
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	// Simulate an error during volume unpublishing
	mockOrchestrator.EXPECT().UnpublishVolume(ctx, req.VolumeId, gomock.Any()).Return(fmt.Errorf("some error"))
	// Verify we do not remove the volume publication if unpublishing fails
	mockOrchestrator.EXPECT().DeleteVolumePublication(ctx, gomock.Any(), gomock.Any()).Times(0)

	_, err := controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.NotNil(t, err, "unexpected success unpublishing volume")
}
