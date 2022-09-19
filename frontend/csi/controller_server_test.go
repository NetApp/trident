// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
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
		Name:    nodeID,
		Deleted: false,
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

	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Return(nil)

	publishResponse, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.Nilf(t, err, "unexpected error publishing volume; %v", err)

	publishContext := publishResponse.PublishContext
	expectedPublishContext := map[string]string{"mountOptions": "", "protocol": ""}
	assert.Equal(t, expectedPublishContext, publishContext)
}

func TestControllerPublishVolume_BlockProtocol(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeNode := generateFakeNode(req.NodeId)
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeVolumeExternal.Config.Protocol = tridentconfig.Block

	mockOrchestrator.EXPECT().GetVolume(ctx, req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(ctx, req.NodeId).Return(fakeNode, nil)
	mockOrchestrator.EXPECT().PublishVolume(ctx, req.VolumeId, gomock.Any()).Return(nil)

	publishResponse, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.Nilf(t, err, "unexpected error publishing volume; %v", err)
	publishContext := publishResponse.PublishContext
	expectedPublishContext := map[string]string{
		"filesystemType":         "",
		"iscsiIgroup":            "",
		"iscsiInterface":         "",
		"iscsiLunNumber":         "0",
		"iscsiLunSerial":         "",
		"iscsiTargetIqn":         "",
		"iscsiTargetPortalCount": "1",
		"LUKSEncryption":         "",
		"mountOptions":           "",
		"p1":                     "",
		"protocol":               "block",
		"sharedTarget":           "false",
		"useCHAP":                "false",
	}
	assert.Equal(t, expectedPublishContext, publishContext)
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

	mockOrchestrator.EXPECT().UnpublishVolume(ctx, req.VolumeId, gomock.Any()).Return(nil)

	_, err := controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Nil(t, err, "unexpected error unpublishing volume")
}

func TestControllerUnpublishVolume_NotFoundErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockHybridPlugin(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakeUnpublishVolumeRequest()

	// Simulate an error during volume unpublishing
	mockOrchestrator.EXPECT().UnpublishVolume(ctx, req.VolumeId, gomock.Any()).Return(utils.NotFoundError("not found"))

	_, err := controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Nil(t, err, "unexpected error unpublishing volume")
}
