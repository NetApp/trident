// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockhelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
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

func generateVolumePublicationFromCSIPublishRequest(req *csi.ControllerPublishVolumeRequest) *models.VolumePublication {
	vp := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(req.GetVolumeId(), req.GetNodeId()),
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

func generateFakeNode(nodeID string) *models.Node {
	fakeNode := &models.Node{
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
	expectedPublishContext := map[string]string{
		"filesystemType": "",
		"formatOptions":  "",
		"mountOptions":   "",
		"protocol":       "",
	}
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
		"formatOptions":          "",
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
	volumePublishInfo := &models.VolumePublishInfo{
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
		"formatOptions":     "",
		"nvmeNamespaceUUID": "",
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

func TestControllerListSnapshots_CoreListsSnapshots(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshotConfig := &storage.SnapshotConfig{
		Name:       snapshotID,
		VolumeName: volumeID,
	}
	snapshot := storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	mockOrchestrator.EXPECT().ListSnapshots(
		gomock.Any(),
	).Return([]*storage.SnapshotExternal{snapshot.ConstructExternal()}, nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(volume.ConstructExternal(), nil)

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: ""}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp.Entries)
}

func TestControllerListSnapshots_CoreListsSnapshotsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshotConfig := &storage.SnapshotConfig{
		Name:       snapshotID,
		VolumeName: volumeID,
	}
	snapshot := storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	mockOrchestrator.EXPECT().ListSnapshotsForVolume(
		gomock.Any(), volumeID,
	).Return([]*storage.SnapshotExternal{snapshot.ConstructExternal()}, nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(volume.ConstructExternal(), nil)

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: volumeID}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp.Entries)
}

func TestControllerListSnapshots_CoreFailsToFindSnapshotsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"

	mockOrchestrator.EXPECT().ListSnapshotsForVolume(
		gomock.Any(), volumeID,
	).Return(nil, errors.NotFoundError("snapshots not found"))

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: volumeID}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.Entries)
	assert.Empty(t, resp.NextToken)
}

func TestControllerListSnapshots_CoreFailsToListsSnapshotsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"

	mockOrchestrator.EXPECT().ListSnapshotsForVolume(
		gomock.Any(), volumeID,
	).Return(nil, fmt.Errorf("core error"))

	// No snapshot ID or source volume ID will force Trident to list all snapshots.
	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: "", SourceVolumeId: volumeID}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestControllerListSnapshots_SnapshotImport_FailsWithInvalidID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapshotID := "snap-content-01"

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s--%s", volumeID, snapshotID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)

	// CSI spec calls for empty return and error if snapshot is not found.
	assert.Nil(t, resp.Entries)
	assert.NoError(t, err)
}

func TestControllerListSnapshots_SnapshotImport_FailsWithInvalidIDComponents(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap_content-01!"

	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(false)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)

	// CSI spec calls for empty return and error if snapshot is not found.
	assert.Nil(t, resp.Entries)
	assert.NoError(t, err)
}

func TestControllerListSnapshots_SnapshotImport_FailsToGetSnapshots(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"

	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, fmt.Errorf("core error"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_FailsToGetSnapshotConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"

	// If a snapshotID isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.NotFoundError("snapshotID not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, fmt.Errorf("helper err"))

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_CoreFailsToImportSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapshotID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:       snapshotID,
		VolumeName: volumeID,
	}

	// If a snapshotID isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapshotID,
	).Return(nil, errors.NotFoundError("snapshotID not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapshotID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapshotID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(nil, fmt.Errorf("core error"))

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapshotID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_CoreFailsToFindParentVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:         snapContentID,
		VolumeName:   volumeID,
		InternalName: "snap.2023-05-23_175116",
	}
	snapshot := &storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// If a snapshot isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.NotFoundError("snapshot not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapContentID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(snapshot.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(nil, fmt.Errorf("core error"))

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.Nil(t, resp.Entries)
	assert.Error(t, err)

	// Get the CSI status code from the returned error.
	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Internal, status.Code())
}

func TestControllerListSnapshots_SnapshotImport_CoreFailsToFindSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	snapContentID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:         snapContentID,
		VolumeName:   volumeID,
		InternalName: "snap.2023-05-23_175116",
	}
	snapshot := &storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}
	notFoundErr := errors.NotFoundError("snapshot not found")

	// If a snapshot isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(gomock.Any(), volumeID, snapContentID).Return(nil, notFoundErr)
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapContentID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(snapshot.ConstructExternal(), notFoundErr)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)

	// CSI spec calls for empty return and error if snapshot is not found.
	assert.Nil(t, resp.Entries)
	assert.NoError(t, err)
}

func TestControllerListSnapshots_SnapshotImport_Succeeds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapContentID := "snap-content-01"
	snapshotConfig := &storage.SnapshotConfig{
		Name:         snapContentID,
		VolumeName:   volumeID,
		InternalName: "snap.2023-05-23_175116",
	}
	snapshot := &storage.Snapshot{
		Config:    snapshotConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// If a snapshot isn't found, ListSnapshots will try to import it.
	mockOrchestrator.EXPECT().GetSnapshot(
		gomock.Any(), volumeID, snapContentID,
	).Return(nil, errors.NotFoundError("snapshot not found"))
	mockHelper.EXPECT().IsValidResourceName(volumeID).Return(true)
	mockHelper.EXPECT().IsValidResourceName(snapContentID).Return(true)
	mockHelper.EXPECT().GetSnapshotConfigForImport(gomock.Any(), volumeID, snapContentID).Return(snapshotConfig, nil)
	mockOrchestrator.EXPECT().ImportSnapshot(gomock.Any(), snapshotConfig).Return(snapshot.ConstructExternal(), nil)
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), volumeID).Return(volume.ConstructExternal(), nil)

	fakeReq := &csi.ListSnapshotsRequest{SnapshotId: fmt.Sprintf("%s/%s", volumeID, snapContentID)}
	resp, err := controllerServer.ListSnapshots(ctx, fakeReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp.Entries)

	csiSnapshot := resp.Entries[0]
	assert.NotNil(t, csiSnapshot)
	assert.Equal(t, snapshot.ID(), csiSnapshot.GetSnapshot().GetSnapshotId())
}

func TestControllerGetListSnapshots_FailsWhenRequestIsInvalid(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	var snapshots []*storage.SnapshotExternal

	fakeReq := &csi.ListSnapshotsRequest{MaxEntries: -1}
	snapshotResp, err := controllerServer.getListSnapshots(ctx, fakeReq, snapshots)
	assert.Error(t, err)

	status, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, status.Code())
	assert.Nil(t, snapshotResp)
}

func TestControllerGetListSnapshots_AddsSnapshotToCSISnapshotResponse(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshots := []*storage.SnapshotExternal{
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       snapshotID,
					VolumeName: volumeID,
				},
			},
		},
	}

	mockOrchestrator.EXPECT().GetVolume(
		gomock.Any(), volumeID,
	).Return(volume.ConstructExternal(), nil).Times(len(snapshots))

	fakeReq := &csi.ListSnapshotsRequest{MaxEntries: int32(len(snapshots) + 1), StartingToken: snapshots[0].ID()}
	listSnapshotResp, err := controllerServer.getListSnapshots(ctx, fakeReq, snapshots)
	assert.NoError(t, err)
	assert.NotNil(t, listSnapshotResp)

	entries := listSnapshotResp.GetEntries()
	assert.NotEmpty(t, entries)

	csiSnapshot := entries[0].GetSnapshot()
	assert.NotNil(t, csiSnapshot)
	assert.Equal(t, snapshots[0].ID(), csiSnapshot.GetSnapshotId())
}

func TestControllerGetListSnapshots_DoesNotExceedMaxEntries(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
	controllerServer := generateController(mockOrchestrator, mockHelper)

	// Set up variables for mocks.
	volumeID := "pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278"
	volume := &storage.Volume{Config: &storage.VolumeConfig{Size: "1Gi"}}
	snapshotID := "snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj"
	snapshots := []*storage.SnapshotExternal{
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       snapshotID,
					VolumeName: volumeID,
				},
			},
		},
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       snapshotID,
					VolumeName: volumeID,
				},
			},
		},
	}

	mockOrchestrator.EXPECT().GetVolume(
		gomock.Any(), volumeID,
	).Return(volume.ConstructExternal(), nil).Times(len(snapshots))

	fakeReq := &csi.ListSnapshotsRequest{MaxEntries: 0, StartingToken: ""}
	listSnapshotResp, err := controllerServer.getListSnapshots(ctx, fakeReq, snapshots)
	assert.NoError(t, err)
	assert.NotNil(t, listSnapshotResp)

	entries := listSnapshotResp.GetEntries()
	assert.NotEmpty(t, entries)
	assert.NotEqual(t, math.MaxInt16, len(entries))
}
