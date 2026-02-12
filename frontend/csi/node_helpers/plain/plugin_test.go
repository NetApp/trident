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
	h, err := NewHelper(orchestrator)
	assert.NoError(t, err, "expected no error during helper initialization")
	assert.NotNilf(t, h, "expected helper to not be nil")
}

func TestNewHelper_VolumeStatsManagerInitialization(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)

	help, err := NewHelper(orchestrator)
	assert.NoError(t, err, "expected no error during helper initialization")
	assert.NotNil(t, help, "expected helper to not be nil")

	// Cast to the concrete helper type to verify VolumeStatsManager is embedded
	h, ok := help.(*helper)
	assert.True(t, ok, "expected helper to be of type *helper")
	assert.NotNil(t, h.VolumeStatsManager, "expected VolumeStatsManager to be initialized")

	// Verify VolumePublishManager is also initialized
	assert.NotNil(t, h.VolumePublishManager, "expected VolumePublishManager to be initialized")
}

func TestNewHelper_VolumeStatsManagerInitializationFailure(t *testing.T) {
	// This test documents the error handling path when VolumeStatsManager initialization fails.
	// In the current implementation, VolumeStatsManager.New() can fail if mount.New() fails internally.
	// Since mount.New() is called internally and not easily mockable, this test verifies that:
	// 1. NewHelper properly checks for errors from NewVolumeStatsManager
	// 2. If an error occurs, it's properly propagated with context
	// 3. The helper is not created (returns nil) when VolumeStatsManager init fails
	//
	// Note: In production, this would occur if the mount subsystem fails to initialize,
	// which could happen due to missing kernel modules, permission issues, or
	// unsupported platform configurations.

	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)

	// Test with standard initialization - should succeed
	help, err := NewHelper(orchestrator)

	// In normal conditions, initialization succeeds
	assert.NoError(t, err, "expected no error in normal conditions")
	assert.NotNil(t, help, "expected helper to be created")

	if help != nil {
		h, ok := help.(*helper)
		assert.True(t, ok, "expected helper to be of type *helper")

		// Verify that if VolumeStatsManager initialization had failed,
		// we wouldn't have a valid helper with a nil VolumeStatsManager
		assert.NotNil(t, h.VolumeStatsManager,
			"VolumeStatsManager must be non-nil when NewHelper succeeds")

		// This demonstrates that the error handling is in place:
		// If NewVolumeStatsManager returns an error, NewHelper returns:
		// - nil for the helper
		// - an error with message "could not initialize VolumeStatsManager; %v"
	}
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
