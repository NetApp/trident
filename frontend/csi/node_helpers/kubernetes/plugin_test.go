// Copyright 2022 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	mockOrchestrator "github.com/netapp/trident/mocks/mock_core"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

var (
	volumePublishManagerError = fmt.Errorf("volume tracking error")
	kubernetesHelper          = nodehelpers.KubernetesHelper
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestNewHelper(t *testing.T) {
	aPath := "/var/lib/kubelet"
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	help, err := NewHelper(orchestrator, aPath, false)
	h, ok := help.(*helper)
	if !ok {
		t.Fatal("Could not cast helper to a NodeHelper!")
	}
	assert.Contains(t, h.podsPath, aPath, "value of the kubelet dir aPath is unexpected")
	assert.NoError(t, err, "expected nil error!")
}

func TestActivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	mockVolPubMgr := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)
	h := newValidHelper(orchestrator, "pvc-123", mockVolPubMgr)

	mockVolPubMgr.EXPECT().GetVolumeTrackingFiles().Return([]os.FileInfo{}, errors.New("foo"))
	err := h.Activate()
	assert.Error(t, err, "expected error during activate if reconcile fails")

	mockVolPubMgr.EXPECT().GetVolumeTrackingFiles().Return([]os.FileInfo{}, nil)
	err = h.Activate()
	assert.NoError(t, err, "did not expect error if reconcile is successful")
}

func TestDeactivate(t *testing.T) {
	helper := &helper{}
	err := helper.Deactivate()
	assert.NoError(t, err)
}

func TestGetName(t *testing.T) {
	helper := &helper{}
	assert.Equal(t, kubernetesHelper, helper.GetName())
}

func TestVersion(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	h, _ := NewHelper(orchestrator, "", false)
	assert.Equal(t, "unknown", h.Version())
}

func TestReconcileVolumeTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)
	ctx := context.Background()
	volId := "pvc-123"
	h := newValidHelper(orchestrator, volId, mockVolumePublishManager)

	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	_, _ = osFs.Create(volId)
	fInfo, _ := osFs.Stat(volId)

	mockVolumePublishManager.EXPECT().GetVolumeTrackingFiles().Return([]os.FileInfo{}, errors.New("foo"))
	err := h.reconcileVolumePublishInfo(ctx)
	assert.Error(t, err, "expected error if volume tracking files couldn't be retrieved")

	mockVolumePublishManager.EXPECT().GetVolumeTrackingFiles().Return([]os.FileInfo{fInfo}, nil)
	err = h.reconcileVolumePublishInfo(ctx)
	assert.Error(t, err, "expected error if published paths discovery fails")

	_ = osFs.Mkdir("/pods", 0o777)
	mockVolumePublishManager.EXPECT().GetVolumeTrackingFiles().Return([]os.FileInfo{fInfo}, nil)
	mockVolumePublishManager.EXPECT().UpgradeVolumeTrackingFile(ctx, volId, make(map[string]struct{}), gomock.Any()).
		Return(false, errors.New("foo"))
	err = h.reconcileVolumePublishInfo(ctx)
	assert.Error(t, err, "expected error if reconcile file fails")
	assert.Equal(t, "foo", err.Error(), "expected the error that the mock returned")

	mockVolumePublishManager.EXPECT().GetVolumeTrackingFiles().Return([]os.FileInfo{}, nil)
	err = h.reconcileVolumePublishInfo(ctx)
	assert.NoError(t, err, "expected no error if reconcile succeeds with no files found")
}

func TestReconcileVolumeTrackingInfoFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)
	ctx := context.Background()
	volId := "pvc-123"
	fName := volId + ".json"
	paths := map[string]struct{}{}

	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	validH := newValidHelper(orchestrator, volId, mockVolumePublishManager)

	mockVolumePublishManager.EXPECT().UpgradeVolumeTrackingFile(ctx, volId, paths,
		nil).Return(false, nil)
	mockVolumePublishManager.EXPECT().ValidateTrackingFile(ctx, volId).Return(false, nil)

	err := validH.reconcileVolumePublishInfoFile(ctx, fName, nil)
	assert.NoError(t, err, "did not expect an error during reconcile")

	mockVolumePublishManager.EXPECT().UpgradeVolumeTrackingFile(ctx, volId, paths,
		nil).Return(false, errors.New("foo"))
	err = validH.reconcileVolumePublishInfoFile(ctx, fName, nil)
	assert.Error(t, err, "expected error if upgrade to tracking file occurred")

	mockVolumePublishManager.EXPECT().UpgradeVolumeTrackingFile(gomock.Any(), volId, paths,
		nil).Return(false, nil)
	mockVolumePublishManager.EXPECT().ValidateTrackingFile(gomock.Any(), volId).Return(true, nil)
	mockVolumePublishManager.EXPECT().DeleteTrackingInfo(gomock.Any(), volId).Return(errors.New("foo error"))

	err = validH.reconcileVolumePublishInfoFile(ctx, fName, nil)
	assert.Error(t, err, "expected error if file delete failed")

	mockVolumePublishManager.EXPECT().UpgradeVolumeTrackingFile(ctx, volId, paths,
		nil).Return(false, nil)
	mockVolumePublishManager.EXPECT().ValidateTrackingFile(ctx, volId).Return(false, errors.New("foo"))

	err = validH.reconcileVolumePublishInfoFile(ctx, fName, nil)
	assert.Error(t, err, "expected error if validate tracking file error occurred")
}

func TestAddPublishedPath_FailsToReadTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables.
	ctx := context.Background()
	volumeID := "1234567890"
	pathToAdd := "path/to/add"

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
	ctx := context.Background()
	volumeID := "1234567890"
	pathToAdd := "path/to/remove"
	volTrackingInfo := &utils.VolumeTrackingInfo{
		VolumePublishInfo:      utils.VolumePublishInfo{},
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
		publishedPaths: map[string]map[string]struct{}{
			volumeID: {},
		},
	}

	assert.NotContains(t, helper.publishedPaths[volumeID], pathToAdd)
	err := helper.AddPublishedPath(ctx, volumeID, pathToAdd)
	assert.Error(t, err, "expected error")
	assert.NotContains(t, helper.publishedPaths[volumeID], pathToAdd)
}

func TestAddPublishedPath_Succeeds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.Background()
	volumeID := "1234567890"
	pathToAdd := "path/to/add"
	volTrackingInfo := &utils.VolumeTrackingInfo{
		PublishedPaths: map[string]struct{}{},
	}

	// Mock out the expected calls.
	mockVolumePublishManager.EXPECT().ReadTrackingInfo(ctx, volumeID).Return(volTrackingInfo, nil)
	mockVolumePublishManager.EXPECT().WriteTrackingInfo(ctx, volumeID, volTrackingInfo).Return(nil)

	// Inject the VolumePublishManager and add a published path.
	helper := &helper{
		VolumePublishManager: mockVolumePublishManager,
		publishedPaths: map[string]map[string]struct{}{
			volumeID: {},
		},
	}

	assert.NotContains(t, volTrackingInfo.PublishedPaths, pathToAdd)
	assert.NotContains(t, helper.publishedPaths[volumeID], pathToAdd)
	err := helper.AddPublishedPath(ctx, volumeID, pathToAdd)
	assert.NoError(t, err, "expected no error")
	assert.Contains(t, helper.publishedPaths[volumeID], pathToAdd)
	assert.Contains(t, volTrackingInfo.PublishedPaths, pathToAdd)
}

func TestRemovePublishedPath_FailsToReadTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.Background()
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
	ctx := context.Background()
	volumeID := "1234567890"
	pathToRemove := "path/to/remove"
	volTrackingInfo := &utils.VolumeTrackingInfo{
		VolumePublishInfo:      utils.VolumePublishInfo{},
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
		publishedPaths: map[string]map[string]struct{}{
			volumeID: {
				pathToRemove: {},
			},
		},
	}

	assert.Contains(t, helper.publishedPaths[volumeID], pathToRemove)
	err := helper.RemovePublishedPath(ctx, volumeID, pathToRemove)
	assert.Error(t, err, "expected error")
	assert.Contains(t, helper.publishedPaths[volumeID], pathToRemove)
}

func TestRemovePublishedPath_Succeeds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVolumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	// Setup test variables and mocks.
	ctx := context.Background()
	volumeID := "1234567890"
	pathToRemove := "path/to/remove"
	volTrackingInfo := &utils.VolumeTrackingInfo{
		VolumePublishInfo:      utils.VolumePublishInfo{},
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
		publishedPaths: map[string]map[string]struct{}{
			volumeID: {
				pathToRemove: {},
			},
		},
	}

	assert.Contains(t, helper.publishedPaths[volumeID], pathToRemove)
	assert.Contains(t, volTrackingInfo.PublishedPaths, pathToRemove)
	err := helper.RemovePublishedPath(ctx, volumeID, pathToRemove)
	assert.NoError(t, err, "expected no error")
	assert.NotContains(t, helper.publishedPaths[volumeID], pathToRemove)
	assert.NotContains(t, volTrackingInfo.PublishedPaths, pathToRemove)
}

func TestDiscoverPVCsToPublishedPaths(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	help, _ := NewHelper(orchestrator, "/var/lib/kubelet", false)
	h, ok := help.(*helper)
	if !ok {
		t.Fatal("Could not cast helper to a NodeHelper!")
	}

	podUUIDPath := "/var/lib/kubelet/pods/pod-uuid/" + volumesFilesystemPath
	volName := "pvc-123"
	_ = osFs.MkdirAll(podUUIDPath, 0o777)
	_, _ = osFs.Create(podUUIDPath + volName)

	result, err := h.discoverPVCsToPublishedPathsFilesystemVolumes(context.Background())
	expectedPublishedPath := filepath.Join(podUUIDPath, volName, "mount")
	_, ok = result[volName][expectedPublishedPath]
	assert.NoError(t, err)
	assert.True(t, ok, "expected published path not found in map!")
}

func TestDiscoverPVCsToPublishedPaths_ReadDirFails(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	help, _ := NewHelper(orchestrator, "/var/lib/kubelet", false)
	h, ok := help.(*helper)
	if !ok {
		t.Fatal("Could not case helper to a NodeHelper!")
	}

	podUUIDPath := "/var/lib/kubelet/pods/pod-uuid/" + volumesFilesystemPath
	volName := "pvc-123"
	_ = osFs.MkdirAll(podUUIDPath, 0o777)
	_, _ = osFs.Create(podUUIDPath + volName)

	// invalid path
	h.podsPath = "/*"
	res, err := h.discoverPVCsToPublishedPathsFilesystemVolumes(context.Background())
	assert.Error(t, err)
	assert.Nil(t, res, "expected nil map!")

	// remove directory, then make a file with the same name
	_ = osFs.RemoveAll(podUUIDPath)
	_, _ = osFs.Create(podUUIDPath)
	h.podsPath = "/var/lib/kubelet/pods"
	res, err = h.discoverPVCsToPublishedPathsFilesystemVolumes(context.Background())
	assert.Error(t, err)
	assert.Empty(t, res)
}

func TestDiscoverPVCsToPublishedPathsRawDevices(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	help, _ := NewHelper(orchestrator, "/var/lib/kubelet", false)
	h, ok := help.(*helper)
	if !ok {
		t.Fatal("Could not cast helper to a NodeHelper!")
	}

	podUUIDPathBase := "/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/"
	volName1 := "pvc-123"
	podUUID1 := "123-456"
	podUUIDPath1 := podUUIDPathBase + volName1
	_ = osFs.MkdirAll(podUUIDPath1, 0o777)
	_, _ = osFs.Create(podUUIDPath1 + "/" + podUUID1)

	volName2 := "pvc-123"
	podUUID2 := "123-456"
	podUUIDPath2 := podUUIDPathBase + volName2
	_ = osFs.MkdirAll(podUUIDPath2, 0o777)
	_, _ = osFs.Create(podUUIDPath2 + "/" + podUUID2)

	mapping := make(map[string]map[string]struct{})

	err := h.discoverPVCsToPublishedPathsRawDevices(context.Background(), mapping)
	expectedPublishedPath := filepath.Join(podUUIDPath1, podUUID1)
	_, ok = mapping[volName1][expectedPublishedPath]
	assert.NoError(t, err)
	assert.True(t, ok, "expected published path not found in map!")

	expectedPublishedPath = filepath.Join(podUUIDPath2, podUUID2)
	_, ok = mapping[volName1][expectedPublishedPath]
	assert.NoError(t, err)
	assert.True(t, ok, "expected published path not found in map!")
}

func TestDiscoverPVCsToPublishedPathsRawDevices_EmptyMap(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockOrchestrator.NewMockOrchestrator(mockCtrl)
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	help, _ := NewHelper(orchestrator, "/var/lib/kubelet", false)
	h, ok := help.(*helper)
	if !ok {
		t.Fatal("Could not cast helper to a NodeHelper!")
	}

	podUUIDPathBase := "/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/"
	volName1 := "pvc-123"
	podUUID1 := "123-456"
	podUUIDPath1 := podUUIDPathBase + volName1
	_ = osFs.MkdirAll(podUUIDPath1, 0o777)
	_, _ = osFs.Create(podUUIDPath1 + "/" + podUUID1)

	volName2 := "pvc-123"
	podUUID2 := "123-456"
	podUUIDPath2 := podUUIDPathBase + volName2
	_ = osFs.MkdirAll(podUUIDPath2, 0o777)
	_, _ = osFs.Create(podUUIDPath2 + "/" + podUUID2)

	mapping := make(map[string]map[string]struct{})

	// invalid path
	h.kubeConfigPath = "/abc/something"
	err := h.discoverPVCsToPublishedPathsRawDevices(context.Background(), mapping)
	assert.Nil(t, err)
	assert.True(t, len(mapping) == 0, "expected empty map!")
}

func newValidHelper(
	orchestrator *mockOrchestrator.MockOrchestrator, volId string, mockPubMgr *mockNodeHelpers.MockVolumePublishManager,
) *helper {
	return &helper{
		orchestrator: orchestrator,
		podsPath:     "/pods",
		publishedPaths: map[string]map[string]struct{}{
			volId: {},
		},
		VolumePublishManager: mockPubMgr,
	}
}
