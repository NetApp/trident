// Copyright 2022 NetApp, Inc. All Rights Reserved.

package plain

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/frontend/csi"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	mockOrchestrator "github.com/netapp/trident/mocks/mock_core"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/utils/models"
)

var volumePublishManagerError = fmt.Errorf("volume tracking error")

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestDeactivate(t *testing.T) {
	helper := &helper{}
	err := helper.Deactivate()
	assert.NoError(t, err)
}

func TestGetName(t *testing.T) {
	helper := &helper{}
	assert.Equal(t, nodehelpers.PlainCSIHelper, helper.GetName())
}

func TestAddPublishedPath_FailsToReadTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables.
	ctx := context.TODO()
	volumeID := "1234567890"
	pathToAdd := "path/to/remove"

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(nil, volumePublishManagerError)

	// Inject the VolumePublishManager.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
	}
	err := helper.AddPublishedPath(ctx, volumeID, pathToAdd)
	assert.Error(t, err, "expected error")
}

func TestAddPublishedPath_FailsToWriteTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables.
	ctx := context.TODO()
	volumeID := "1234567890"
	pathToAdd := "path/to/remove"
	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo:      models.VolumePublishInfo{},
		VolumeTrackingInfoPath: "",
		StagingTargetPath:      "",
		PublishedPaths: map[string]struct{}{
			pathToAdd: {},
		},
	}

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(volTrackingInfo, nil)
	mockVolumePublishManager.EXPECT().WriteTrackingInfo(ctx, volumeID,
		volTrackingInfo).Return(volumePublishManagerError)

	// Inject the VolumePublishManager and add an empty entry for the volumeID.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
	}

	err := helper.AddPublishedPath(ctx, volumeID, pathToAdd)
	assert.Error(t, err, "expected error")
}

func TestAddPublishedPath_Succeeds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.TODO()
	volumeID := "1234567890"
	pathToAdd := "path/to/add"
	volTrackingInfo := &models.VolumeTrackingInfo{
		PublishedPaths: map[string]struct{}{},
	}

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(volTrackingInfo, nil)
	mockVolumePublishManager.EXPECT().WriteTrackingInfo(ctx, volumeID, volTrackingInfo).Return(nil)

	// Inject the VolumePublishManager and add a published path.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
	}

	err := helper.AddPublishedPath(ctx, volumeID, pathToAdd)
	assert.NoError(t, err, "expected no error")
	assert.Contains(t, volTrackingInfo.PublishedPaths, pathToAdd)
}

func TestRemovePublishedPath_FailsToReadTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.TODO()
	volumeID := "1234567890"
	pathToRemove := "path/to/remove"

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(nil, volumePublishManagerError)

	// Inject the VolumePublishManager.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
	}
	err := helper.RemovePublishedPath(ctx, volumeID, pathToRemove)
	assert.Error(t, err, "expected error")
}

func TestRemovePublishedPath_FailsToWriteTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.TODO()
	volumeID := "1234567890"
	pathToRemove := "path/to/remove"
	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo:      models.VolumePublishInfo{},
		VolumeTrackingInfoPath: "",
		StagingTargetPath:      "",
		PublishedPaths: map[string]struct{}{
			pathToRemove: {},
		},
	}

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(volTrackingInfo, nil)
	mockVolumePublishManager.EXPECT().WriteTrackingInfo(ctx, volumeID,
		volTrackingInfo).Return(volumePublishManagerError)

	// Inject the VolumePublishManager and add a published path.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
	}

	err := helper.RemovePublishedPath(ctx, volumeID, pathToRemove)
	assert.Error(t, err, "expected error")
}

func TestRemovePublishedPath_Succeeds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.TODO()
	volumeID := "1234567890"
	pathToRemove := "path/to/remove"
	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo:      models.VolumePublishInfo{},
		VolumeTrackingInfoPath: "",
		StagingTargetPath:      "",
		PublishedPaths: map[string]struct{}{
			pathToRemove: {},
		},
	}

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(volTrackingInfo, nil)
	mockVolumePublishManager.EXPECT().WriteTrackingInfo(ctx, volumeID, volTrackingInfo).Return(nil)

	// Inject the VolumePublishManager and add a published path.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
	}

	err := helper.RemovePublishedPath(ctx, volumeID, pathToRemove)
	assert.NoError(t, err, "expected no error")
	assert.NotContains(t, volTrackingInfo.PublishedPaths, pathToRemove)
}

func TestNewHelper(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	h, _ := NewHelper(orchestrator)
	assert.NotNilf(t, h, "expected helper to not be nil")
}

func TestActivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	h, _ := NewHelper(orchestrator)
	err := h.Activate()
	assert.NoError(t, err)
}

func TestVersion(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	h, _ := NewHelper(orchestrator)
	assert.Equal(t, csi.Version, h.Version())
}
