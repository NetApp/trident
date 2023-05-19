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
	mockhelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

var ctx = context.Background()

func generateController(
	mockOrchestrator *mockcore.MockOrchestrator, mockHelper *mockhelpers.MockControllerHelper,
) *Plugin {
	controllerServer := &Plugin{
		orchestrator:     mockOrchestrator,
		name:             Provisioner,
		nodeName:         "foo",
		version:          tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:         "bar",
		role:             CSIController,
		controllerHelper: mockHelper,
		opCache:          sync.Map{},
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

func generateVolumePublicationFromCSIPublishRequest(req *csi.ControllerPublishVolumeRequest) *utils.VolumePublication {
	vp := &utils.VolumePublication{
		Name:       utils.GenerateVolumePublishName(req.GetVolumeId(), req.GetNodeId()),
		VolumeName: req.GetVolumeId(),
		NodeName:   req.GetNodeId(),
		ReadOnly:   req.GetReadonly(),
	}
	if req.VolumeCapability != nil {
		if req.VolumeCapability.GetAccessMode() != nil {
			vp.AccessMode = int32(req.VolumeCapability.GetAccessMode().GetMode())
		}
	}
	return vp
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
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeNode := generateFakeNode(req.NodeId)

	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(gomock.Any(), req.NodeId).Return(fakeNode.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().PublishVolume(gomock.Any(), req.VolumeId, gomock.Any()).Return(nil)

	publishResponse, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.Nilf(t, err, "unexpected error publishing volume; %v", err)

	publishContext := publishResponse.PublishContext
	expectedPublishContext := map[string]string{"filesystemType": "", "mountOptions": "", "protocol": ""}
	assert.Equal(t, expectedPublishContext, publishContext)
}

func TestControllerPublishVolume_iSCSIProtocol(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeNode := generateFakeNode(req.NodeId)
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeVolumeExternal.Config.Protocol = tridentconfig.Block

	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(gomock.Any(), req.NodeId).Return(fakeNode.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().PublishVolume(gomock.Any(), req.VolumeId, gomock.Any()).Return(nil)

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
		"SANType":                sa.ISCSI,
		"sharedTarget":           "false",
		"useCHAP":                "false",
	}
	assert.Equal(t, expectedPublishContext, publishContext)
}

func TestControllerPublishVolume_NVMeProtocol(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakePublishVolumeRequest()
	fakeNode := generateFakeNode(req.NodeId)
	fakeVolumeExternal := generateFakeVolumeExternal(req.VolumeId)
	fakeVolumeExternal.Config.Protocol = tridentconfig.Block
	volumePublishInfo := &utils.VolumePublishInfo{
		SANType: sa.NVMe,
	}

	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), req.VolumeId).Return(fakeVolumeExternal, nil)
	mockOrchestrator.EXPECT().GetNode(gomock.Any(), req.NodeId).Return(fakeNode.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().PublishVolume(gomock.Any(), req.VolumeId, gomock.Any()).SetArg(2, *volumePublishInfo).Return(nil)

	publishResponse, err := controllerServer.ControllerPublishVolume(ctx, req)
	assert.Nilf(t, err, "unexpected error publishing volume; %v", err)
	publishContext := publishResponse.PublishContext

	expectedPublishContext := map[string]string{
		"filesystemType":    "",
		"LUKSEncryption":    "",
		"mountOptions":      "",
		"nvmeNamespacePath": "",
		"nvmeSubsystemNqn":  "",
		"protocol":          "block",
		"SANType":           sa.NVMe,
		"sharedTarget":      "false",
		"nvmeTargetIPs":     "",
	}
	assert.Equal(t, expectedPublishContext, publishContext)
}

func TestControllerUnpublishVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakeUnpublishVolumeRequest()

	mockHelper.EXPECT().GetNodePublicationState(gomock.Any(), req.NodeId).Return(nil, nil).Times(1)
	mockOrchestrator.EXPECT().UpdateNode(gomock.Any(), req.NodeId, nil).Return(nil).Times(1)
	mockOrchestrator.EXPECT().UnpublishVolume(gomock.Any(), req.VolumeId, req.NodeId).Return(nil)

	_, err := controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Nil(t, err, "unexpected error unpublishing volume")
}

func TestControllerUnpublishVolume_NotFoundErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked orchestrator
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Create a mocked helper
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	// Create an instance of ControllerServer for this test
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Create fake objects for this test
	req := generateFakeUnpublishVolumeRequest()
	nodeErr := errors.New("")
	notFoundErr := errors.NotFoundError("")

	// Simulate an error during fetching node state.
	mockHelper.EXPECT().GetNodePublicationState(gomock.Any(), req.NodeId).Return(nil, nodeErr).Times(1)

	_, err := controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Error(t, err, "expected error fetching node state")

	// Simulate an error during updating node state.
	mockHelper.EXPECT().GetNodePublicationState(gomock.Any(), req.NodeId).Return(nil, nil).Times(1)
	mockOrchestrator.EXPECT().UpdateNode(gomock.Any(), req.NodeId, nil).Return(nodeErr).Times(1)

	_, err = controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Error(t, err, "expected error updating node state")

	// Simulate an error during volume unpublishing; not found errors are ignored for
	// GetNodePublicationState and UpdateNode.
	mockHelper.EXPECT().GetNodePublicationState(gomock.Any(), req.NodeId).Return(nil, notFoundErr).Times(1)
	mockOrchestrator.EXPECT().UpdateNode(gomock.Any(), req.NodeId, nil).Return(notFoundErr).Times(1)
	mockOrchestrator.EXPECT().UnpublishVolume(gomock.Any(), req.VolumeId, req.NodeId).Return(errors.NotFoundError("not found"))

	_, err = controllerServer.ControllerUnpublishVolume(ctx, req)
	assert.Nil(t, err, "unexpected error unpublishing volume")
}
