package csi

import (
	"context"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	"github.com/netapp/trident/mocks/mock_core"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mock_controller_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func setThanResetBackoff(t *testing.T, testTime time.Duration) func() {
	t.Helper()

	// We don't need to wait 10 seconds for each failure.
	reset := func(originalBackoff time.Duration) func() {
		return func() {
			volumeGroupSnapshotBackoff = originalBackoff
		}
	}(volumeGroupSnapshotBackoff)
	volumeGroupSnapshotBackoff = testTime
	return reset
}

func TestCreateVolumeGroupSnapshot_NoName(t *testing.T) {
	defer setThanResetBackoff(t, 0)()

	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	// Create a request with an empty name.
	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            "",
		SourceVolumeIds: []string{"vol-1", "vol-2"},
	}

	snapshot, err := plugin.CreateVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, snapshot)
}

func TestCreateVolumeGroupSnapshot_NoSourceVolumes(t *testing.T) {
	defer setThanResetBackoff(t, 0)()

	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc",
		SourceVolumeIds: nil,
	}

	snapshot, err := plugin.CreateVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, snapshot)
}

func TestCreateVolumeGroupSnapshot_GetGroupSnapshotConfigFails(t *testing.T) {
	defer setThanResetBackoff(t, 0)()

	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	vols := []string{"volA", "volB"}

	helper.EXPECT().GetGroupSnapshotConfigForCreate(
		gomock.Any(),
		name,
		vols,
	).Return(nil, errors.New("failure to get group snapshot config")).Times(1)

	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            name,
		SourceVolumeIds: vols,
	}

	resp, err := plugin.CreateVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateVolumeGroupSnapshot_GetGroupSnapshotFails(t *testing.T) {
	defer setThanResetBackoff(t, 0)()

	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	vols := []string{"volA", "volB"}
	groupSnapshotConfig := &storage.GroupSnapshotConfig{
		Version:     Version,
		Name:        name,
		VolumeNames: vols,
	}

	helper.EXPECT().GetGroupSnapshotConfigForCreate(
		gomock.Any(),
		name,
		vols,
	).Return(groupSnapshotConfig, nil).Times(1)
	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		groupSnapshotConfig.ID(),
	).Return(nil, errors.New("failure to get core group snapshot")).Times(1)

	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            name,
		SourceVolumeIds: vols,
	}

	resp, err := plugin.CreateVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateVolumeGroupSnapshot_CreateAndDeleteGroupSnapshotFails(t *testing.T) {
	defer setThanResetBackoff(t, 0)()

	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	vols := []string{"volA", "volB"}
	groupSnapshotConfig := &storage.GroupSnapshotConfig{
		Version:     Version,
		Name:        name,
		VolumeNames: vols,
	}

	helper.EXPECT().GetGroupSnapshotConfigForCreate(
		gomock.Any(),
		name,
		vols,
	).Return(groupSnapshotConfig, nil).Times(1)
	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		groupSnapshotConfig.ID(),
	).Return(nil, errors.NotFoundError("not found")).Times(1)
	orchestrator.EXPECT().CreateGroupSnapshot(
		gomock.Any(),
		groupSnapshotConfig,
	).Return(nil, errors.New("failure to create core group snapshot")).Times(1)
	orchestrator.EXPECT().DeleteGroupSnapshot(
		gomock.Any(),
		groupSnapshotConfig.ID(),
	).Return(errors.New("failed to cleanup failed create snapshot artifacts")).Times(1)

	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            name,
		SourceVolumeIds: vols,
	}

	resp, err := plugin.CreateVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateVolumeGroupSnapshot_Succeeds(t *testing.T) {
	defer setThanResetBackoff(t, 0)()

	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	vols := []string{"volA", "volB"}
	groupSnapshotConfig := &storage.GroupSnapshotConfig{
		Version:     Version,
		Name:        name,
		VolumeNames: vols,
	}

	// Set up a dummy external group snapshot.
	created := time.Now().Format(time.RFC3339)
	groupSnapshotExternal := &storage.GroupSnapshotExternal{}
	groupSnapshotExternal.GroupSnapshotConfig = groupSnapshotConfig
	groupSnapshotExternal.Created = created

	snaps := make([]*storage.SnapshotExternal, 0)
	for _, vol := range groupSnapshotConfig.GetVolumeNames() {
		snapName, err := storage.ConvertGroupSnapshotID(groupSnapshotConfig.ID())
		if err != nil {
			t.Fatal(err.Error())
		}

		snaps = append(snaps, &storage.SnapshotExternal{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Version:            Version,
					Name:               snapName,
					InternalName:       snapName,
					VolumeName:         vol,
					VolumeInternalName: vol,
				},
				Created: created,
				State:   storage.SnapshotStateOnline,
			},
		})
	}

	// Mock out calls.
	helper.EXPECT().GetGroupSnapshotConfigForCreate(
		gomock.Any(),
		name,
		vols,
	).Return(groupSnapshotConfig, nil).Times(1)
	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		groupSnapshotConfig.ID(),
	).Return(nil, errors.NotFoundError("not found")).Times(1)
	orchestrator.EXPECT().CreateGroupSnapshot(
		gomock.Any(),
		groupSnapshotConfig,
	).Return(groupSnapshotExternal, nil).Times(1)
	orchestrator.EXPECT().ListSnapshotsForGroup(
		gomock.Any(),
		groupSnapshotConfig.ID(),
	).Return(snaps, nil).Times(1)

	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            name,
		SourceVolumeIds: vols,
	}

	resp, err := plugin.CreateVolumeGroupSnapshot(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestGetVolumeGroupSnapshot_NoName(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	req := &csi.GetVolumeGroupSnapshotRequest{
		GroupSnapshotId: "",
	}

	resp, err := plugin.GetVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetVolumeGroupSnapshot_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	req := &csi.GetVolumeGroupSnapshotRequest{
		GroupSnapshotId: "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc",
		SnapshotIds:     nil, // CSI doesn't expect a failure if no snapshot IDs are provided.
	}

	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		req.GroupSnapshotId,
	).Return(nil, errors.NotFoundError("not found"))

	resp, err := plugin.GetVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetVolumeGroupSnapshot_GetGroupSnapshotFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	snaps := []string{"vol-1/snapshot-1qaz2wsx3edc1qaz2wsx3edc", "vol-2/snapshot-1qaz2wsx3edc1qaz2wsx3edc"}
	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		name,
	).Return(nil, errors.New("failure to get core group snapshot")).Times(1)

	req := &csi.GetVolumeGroupSnapshotRequest{
		GroupSnapshotId: name,
		SnapshotIds:     snaps,
	}

	resp, err := plugin.GetVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetVolumeGroupSnapshot_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	snapName, err := storage.ConvertGroupSnapshotID(name)
	if err != nil {
		t.Fatal(err.Error())
	}

	snaps := []string{
		"vol-1" + "/" + snapName,
		"vol-2" + "/" + snapName,
	}

	created := time.Now().Format(time.RFC3339)
	snapshots := make([]*storage.SnapshotExternal, 0)
	for _, snap := range snaps {
		volumeName, snapName, err := storage.ParseSnapshotID(snap)
		assert.NoError(t, err)
		assert.NotEmpty(t, volumeName, "volume name should not be empty")
		assert.NotEmpty(t, snapName, "snapshot name should not be empty")

		snapshots = append(snapshots, &storage.SnapshotExternal{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Version:            Version,
					Name:               snapName,
					InternalName:       snapName,
					VolumeName:         volumeName, // Extracting volume name from snapshot ID
					VolumeInternalName: volumeName,
				},
				Created: created,
				State:   storage.SnapshotStateOnline,
			},
		})
	}

	config := &storage.GroupSnapshotConfig{
		Version:     Version,
		Name:        name,
		VolumeNames: []string{"vol-1", "vol-2"},
	}

	groupSnap := &storage.GroupSnapshotExternal{}
	groupSnap.GroupSnapshotConfig = config
	groupSnap.Created = time.Now().Format(time.RFC3339)
	groupSnap.SnapshotIDs = snaps

	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		name,
	).Return(groupSnap, nil).Times(1)
	orchestrator.EXPECT().ListSnapshotsForGroup(
		gomock.Any(),
		name,
	).Return(snapshots, nil).Times(1)

	req := &csi.GetVolumeGroupSnapshotRequest{
		GroupSnapshotId: name,
		SnapshotIds:     snaps,
	}

	resp, err := plugin.GetVolumeGroupSnapshot(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteVolumeGroupSnapshot_NoName(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	// Create a request with an empty name.
	req := &csi.DeleteVolumeGroupSnapshotRequest{
		// GroupSnapshotId: "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc",
		SnapshotIds: []string{"vol-1/snapshot-1qaz2wsx3edc1qaz2wsx3edc", "vol-2/snapshot-1qaz2wsx3edc1qaz2wsx3edc"},
	}

	resp, err := plugin.DeleteVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestDeleteVolumeGroupSnapshot_NoSnapshotIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	req := &csi.DeleteVolumeGroupSnapshotRequest{
		GroupSnapshotId: "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc",
		// CSI doesn't expect a failure if no snapshot IDs are provided.
		// SnapshotIds:     []string{"vol-1/snapshot-1qaz2wsx3edc1qaz2wsx3edc", "vol-2/snapshot-1qaz2wsx3edc1qaz2wsx3edc"},
	}

	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		req.GroupSnapshotId,
	).Return(nil, errors.NotFoundError("not found")).Times(1)

	resp, err := plugin.DeleteVolumeGroupSnapshot(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteVolumeGroupSnapshot_DeleteGroupSnapshotFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	snaps := []string{"vol-1/snapshot-1qaz2wsx3edc1qaz2wsx3edc", "vol-2/snapshot-1qaz2wsx3edc1qaz2wsx3edc"}

	created := time.Now().Format(time.RFC3339)
	snapshots := make([]*storage.SnapshotExternal, 0)
	for _, snap := range snaps {
		volumeName, snapName, err := storage.ParseSnapshotID(snap)
		assert.NoError(t, err)
		assert.NotEmpty(t, volumeName, "volume name should not be empty")
		assert.NotEmpty(t, snapName, "snapshot name should not be empty")

		snapshots = append(snapshots, &storage.SnapshotExternal{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Version:            Version,
					Name:               snapName,
					InternalName:       snapName,
					VolumeName:         volumeName, // Extracting volume name from snapshot ID
					VolumeInternalName: volumeName,
				},
				Created: created,
				State:   storage.SnapshotStateOnline,
			},
		})
	}

	groupSnap := &storage.GroupSnapshotExternal{
		GroupSnapshot: storage.GroupSnapshot{
			GroupSnapshotConfig: &storage.GroupSnapshotConfig{
				Version:     Version,
				Name:        name,
				VolumeNames: []string{"vol-1", "vol-2"},
			},
			Created:     created,
			SnapshotIDs: snaps,
		},
	}

	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		name,
	).Return(groupSnap, nil).Times(1)
	orchestrator.EXPECT().DeleteGroupSnapshot(
		gomock.Any(),
		name,
	).Return(errors.New("failure to delete core group snapshot")).Times(1)

	req := &csi.DeleteVolumeGroupSnapshotRequest{
		GroupSnapshotId: name,
		SnapshotIds:     snaps,
	}

	resp, err := plugin.DeleteVolumeGroupSnapshot(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestDeleteVolumeGroupSnapshot_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	orchestrator := mock_core.NewMockOrchestrator(ctrl)
	helper := mock_controller_helpers.NewMockControllerHelper(ctrl)

	plugin := &Plugin{
		orchestrator:     orchestrator,
		controllerHelper: helper,
	}

	name := "groupsnapshot-1qaz2wsx3edc1qaz2wsx3edc"
	snapName, err := storage.ConvertGroupSnapshotID(name)
	if err != nil {
		t.Fatal(err.Error())
	}

	snaps := []string{
		"vol-1" + "/" + snapName,
		"vol-2" + "/" + snapName,
	}

	config := &storage.GroupSnapshotConfig{
		Version:     Version,
		Name:        name,
		VolumeNames: []string{"vol-1", "vol-2"},
	}

	groupSnap := &storage.GroupSnapshotExternal{}
	groupSnap.GroupSnapshotConfig = config
	groupSnap.Created = time.Now().Format(time.RFC3339)
	groupSnap.SnapshotIDs = snaps

	orchestrator.EXPECT().GetGroupSnapshot(
		gomock.Any(),
		name,
	).Return(groupSnap, nil).Times(1)
	orchestrator.EXPECT().DeleteGroupSnapshot(
		gomock.Any(),
		name,
	).Return(nil).Times(1)
	orchestrator.EXPECT().DeleteSnapshot(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil).Times(len(snaps))

	req := &csi.DeleteVolumeGroupSnapshotRequest{
		GroupSnapshotId: name,
		SnapshotIds:     snaps,
	}

	resp, err := plugin.DeleteVolumeGroupSnapshot(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteGroupedSnapshots(t *testing.T) {
	testCases := []struct {
		name                  string
		snapshotIDs           []string
		deleteSnapshotReturns error
		assertErr             assert.ErrorAssertionFunc
	}{
		{
			name:                  "Success - Delete all snapshots",
			snapshotIDs:           []string{"volume-1/snapshot-1"},
			deleteSnapshotReturns: nil,
			assertErr:             assert.NoError,
		},
		{
			name:                  "Error - Delete snapshot fails",
			snapshotIDs:           []string{"volume-1/snapshot-1"},
			deleteSnapshotReturns: errors.New("some error"),
			assertErr:             assert.Error,
		},
		{
			name:        "Error - Parse snapshot ID should not fail",
			snapshotIDs: []string{"invalid-id"},
			assertErr:   assert.NoError, // errors.New("snapshot ID  does not contain a volume name"),
		},
		{
			name:                  "Success - Delete with not found errors (ignored)",
			snapshotIDs:           []string{"volume-1/snapshot-1"},
			deleteSnapshotReturns: errors.NotFoundError("some error"),
			assertErr:             assert.NoError,
		},
		{
			name:        "Success - Empty snapshot list",
			snapshotIDs: []string{},
			assertErr:   assert.NoError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			plugin := &Plugin{
				orchestrator: mockOrchestrator,
			}

			// Mock GetVolume
			mockOrchestrator.EXPECT().
				DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.deleteSnapshotReturns).AnyTimes()

			err := plugin.deleteGroupedSnapshots(context.Background(), tc.snapshotIDs)

			tc.assertErr(t, err)
		})
	}
}

func TestGroupControllerGetCapabilities(t *testing.T) {
	testCases := []struct {
		name     string
		gcsCap   []*csi.GroupControllerServiceCapability
		expected *csi.GroupControllerGetCapabilitiesResponse
	}{
		{
			name: "Success - Return capabilities",
			gcsCap: []*csi.GroupControllerServiceCapability{
				{
					Type: &csi.GroupControllerServiceCapability_Rpc{
						Rpc: &csi.GroupControllerServiceCapability_RPC{
							Type: csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
						},
					},
				},
			},
			expected: &csi.GroupControllerGetCapabilitiesResponse{
				Capabilities: []*csi.GroupControllerServiceCapability{
					{
						Type: &csi.GroupControllerServiceCapability_Rpc{
							Rpc: &csi.GroupControllerServiceCapability_RPC{
								Type: csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				gcsCap: tc.gcsCap,
			}

			resp, err := plugin.GroupControllerGetCapabilities(context.Background(), &csi.GroupControllerGetCapabilitiesRequest{})

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tc.expected, resp)
		})
	}
}

func TestCreateVolumeGroupSnapshot(t *testing.T) {
	defer setThanResetBackoff(t, 0)()
	snapshotConfig := &storage.SnapshotConfig{
		Name:       "snap1",
		VolumeName: "vol-1",
	}
	snapshot := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     "offline",
		Config:    snapshotConfig,
	}
	snapshotConfig2 := &storage.SnapshotConfig{
		Name:       "snap1",
		VolumeName: "vol-2",
	}
	snapshot2 := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     "offline",
		Config:    snapshotConfig2,
	}
	testCases := []struct {
		name                         string
		req                          *csi.CreateVolumeGroupSnapshotRequest
		listSnapshotsForGroupReturns error
		listSnapshot                 []*storage.SnapshotExternal
		mockControllerHelpers        func() controllerhelpers.ControllerHelper
		expErrCode                   codes.Code
	}{
		{
			name: "CreateVolumeGroupSnapshot Error - group snapshot was invalid",
			req: &csi.CreateVolumeGroupSnapshotRequest{
				Name:            "snapshot-1",
				SourceVolumeIds: []string{"vol-1", "vol-2"},
			},
			mockControllerHelpers: func() controllerhelpers.ControllerHelper {
				helper := mock_controller_helpers.NewMockControllerHelper(gomock.NewController(t))
				helper.EXPECT().GetGroupSnapshotConfigForCreate(gomock.Any(), gomock.Any(), gomock.Any()).Return(&storage.GroupSnapshotConfig{}, nil)
				return helper
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "CreateVolumeGroupSnapshot Error - Could not list snapshots for group snapshot",
			req: &csi.CreateVolumeGroupSnapshotRequest{
				Name:            "snapshot-1",
				SourceVolumeIds: []string{"vol-1", "vol-2"},
			},
			mockControllerHelpers: func() controllerhelpers.ControllerHelper {
				helper := mock_controller_helpers.NewMockControllerHelper(gomock.NewController(t))
				helper.EXPECT().GetGroupSnapshotConfigForCreate(gomock.Any(), gomock.Any(), gomock.Any()).Return(&storage.GroupSnapshotConfig{
					Version:     Version,
					Name:        "grp1",
					VolumeNames: []string{"v1", "v2"},
				}, nil)
				return helper
			},
			listSnapshotsForGroupReturns: errors.New(""),
			expErrCode:                   codes.Unknown,
		},
		{
			name: "CreateVolumeGroupSnapshot Error - Group snapshot exists but is incompatible",
			req: &csi.CreateVolumeGroupSnapshotRequest{
				Name:            "snapshot-1",
				SourceVolumeIds: []string{"vol-1", "vol-2"},
			},
			mockControllerHelpers: func() controllerhelpers.ControllerHelper {
				helper := mock_controller_helpers.NewMockControllerHelper(gomock.NewController(t))
				helper.EXPECT().GetGroupSnapshotConfigForCreate(gomock.Any(), gomock.Any(), gomock.Any()).Return(&storage.GroupSnapshotConfig{
					Version:     Version,
					Name:        "grp1",
					VolumeNames: []string{"v1", "v2"},
				}, nil)
				return helper
			},
			listSnapshotsForGroupReturns: nil,
			expErrCode:                   codes.Internal,
		},
		{
			name: "CreateVolumeGroupSnapshot Error - could not convert group snapshot to CSI group snapshot",
			req: &csi.CreateVolumeGroupSnapshotRequest{
				Name:            "snapshot-1",
				SourceVolumeIds: []string{"vol-1", "vol-2"},
			},
			mockControllerHelpers: func() controllerhelpers.ControllerHelper {
				helper := mock_controller_helpers.NewMockControllerHelper(gomock.NewController(t))
				helper.EXPECT().GetGroupSnapshotConfigForCreate(gomock.Any(), gomock.Any(), gomock.Any()).Return(&storage.GroupSnapshotConfig{
					Version:     Version,
					Name:        "grp1",
					VolumeNames: []string{"vol-1", "vol-2"},
				}, nil)
				return helper
			},
			listSnapshot:                 []*storage.SnapshotExternal{snapshot.ConstructExternal(), snapshot2.ConstructExternal()},
			listSnapshotsForGroupReturns: nil,
			expErrCode:                   codes.Internal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			plugin := &Plugin{
				orchestrator: mockOrchestrator,
			}

			if tc.mockControllerHelpers != nil {
				plugin.controllerHelper = tc.mockControllerHelpers()
			}
			// Mock GetVolume
			mockOrchestrator.EXPECT().
				GetGroupSnapshot(gomock.Any(), gomock.Any()).Return(&storage.GroupSnapshotExternal{GroupSnapshot: storage.GroupSnapshot{GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "snap"}}}, nil).AnyTimes()
			mockOrchestrator.EXPECT().
				ListSnapshotsForGroup(gomock.Any(), gomock.Any()).Return(tc.listSnapshot, tc.listSnapshotsForGroupReturns).AnyTimes()

			_, err := plugin.CreateVolumeGroupSnapshot(context.Background(), tc.req)

			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestGetCSIGroupSnapshotFromTridentGroupSnapshot(t *testing.T) {
	snapshotConfig := &storage.SnapshotConfig{
		Name:       "snap1",
		VolumeName: "vol-1",
	}

	// Create snapshots for different test cases
	onlineSnapshot := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     storage.SnapshotStateOnline,
		Config:    snapshotConfig,
	}

	offlineSnapshot := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     storage.SnapshotStateCreating,
		Config:    snapshotConfig,
	}

	invalidTimeSnapshot := storage.Snapshot{
		Created:   "invalid-time",
		SizeBytes: 1024,
		State:     storage.SnapshotStateOnline,
		Config:    snapshotConfig,
	}

	testCases := []struct {
		name           string
		groupSnapshot  *storage.GroupSnapshotExternal
		snapshots      []*storage.SnapshotExternal
		expectedError  codes.Code
		expectedResult bool
	}{
		{
			name:          "Error - Group snapshot is nil",
			groupSnapshot: nil,
			snapshots:     []*storage.SnapshotExternal{},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Group snapshot validation fails",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name: "snap",
						// Missing VolumeNames to trigger validation failure
					},
					Created: "2023-05-15T17:04:09Z",
				},
			},
			snapshots:     []*storage.SnapshotExternal{},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Invalid group creation time",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name:        "snap",
						VolumeNames: []string{"vol-1"},
					},
					Created: "invalid-time",
				},
			},
			snapshots:     []*storage.SnapshotExternal{},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Snapshot not ready",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name:        "snap",
						VolumeNames: []string{"vol-1"},
					},
					Created: "2023-05-15T17:04:09Z",
				},
			},
			snapshots:     []*storage.SnapshotExternal{offlineSnapshot.ConstructExternal()},
			expectedError: codes.Internal,
		},
		{
			name: "Success - Valid group snapshot with ready snapshots",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name:        "snap",
						VolumeNames: []string{"vol-1"},
					},
					Created: "2023-05-15T17:04:09Z",
				},
			},
			snapshots:      []*storage.SnapshotExternal{onlineSnapshot.ConstructExternal()},
			expectedResult: true,
		},
		{
			name: "Success - Invalid snapshot time uses group time",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name:        "snap",
						VolumeNames: []string{"vol-1"},
					},
					Created: "2023-05-15T17:04:09Z",
				},
			},
			snapshots:      []*storage.SnapshotExternal{invalidTimeSnapshot.ConstructExternal()},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}

			result, err := plugin.getCSIGroupSnapshotFromTridentGroupSnapshot(
				context.Background(), tc.groupSnapshot, tc.snapshots)

			if tc.expectedError != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expectedError, status.Code())
				assert.Nil(t, result)
				return
			}

			if tc.expectedResult {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.groupSnapshot.ID(), result.GroupSnapshotId)
				assert.Len(t, result.Snapshots, len(tc.snapshots))
				assert.True(t, result.ReadyToUse)
			}
		})
	}
}

func TestValidateGroupSnapshot(t *testing.T) {
	snapshotConfig := &storage.SnapshotConfig{
		Name:               "snap1",
		InternalName:       "internal_snap1",
		VolumeName:         "vol-1",
		VolumeInternalName: "internal_vol_1",
	}

	invalidSnapshotConfig := &storage.SnapshotConfig{
		Name:       "", // Invalid: empty name
		VolumeName: "vol-1",
	}

	// Create snapshots for different test cases
	onlineSnapshot := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     storage.SnapshotStateOnline,
		Config:    snapshotConfig,
	}

	offlineSnapshot := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     storage.SnapshotStateCreating,
		Config:    snapshotConfig,
	}

	invalidSnapshot := storage.Snapshot{
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
		State:     storage.SnapshotStateOnline,
		Config:    invalidSnapshotConfig,
	}

	testCases := []struct {
		name          string
		groupSnapshot *storage.GroupSnapshotExternal
		snapshots     []*storage.SnapshotExternal
		volumeNames   []string
		expectedError codes.Code
	}{
		{
			name:          "Error - Group snapshot is nil",
			groupSnapshot: nil,
			snapshots:     []*storage.SnapshotExternal{(&onlineSnapshot).ConstructExternal()},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.Internal,
		},
		{
			name: "Error - No snapshots provided",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots:     []*storage.SnapshotExternal{},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.Internal,
		},
		{
			name: "Error - No volume names provided",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots:     []*storage.SnapshotExternal{(&onlineSnapshot).ConstructExternal()},
			volumeNames:   []string{},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Parse snapshot ID fails",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				// Create snapshot with invalid ID format
				(&storage.Snapshot{
					Created:   "2023-05-15T17:04:09Z",
					SizeBytes: 1024,
					State:     storage.SnapshotStateOnline,
					Config: &storage.SnapshotConfig{
						Name:               "invalid-snapshot-id",
						InternalName:       "internal_invalid",
						VolumeName:         "",
						VolumeInternalName: "internal_vol_1",
					},
				}).ConstructExternal(),
			},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Duplicate volume in group",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				(&storage.Snapshot{
					Created:   "2023-05-15T17:04:09Z",
					SizeBytes: 1024,
					State:     storage.SnapshotStateOnline,
					Config:    &storage.SnapshotConfig{Name: "snap-1", InternalName: "internal_snap1", VolumeName: "vol-1"},
				}).ConstructExternal(),
				(&storage.Snapshot{
					Created:   "2023-05-15T17:04:09Z",
					SizeBytes: 1024,
					State:     storage.SnapshotStateOnline,
					Config:    &storage.SnapshotConfig{Name: "snap-2", InternalName: "internal_snap2", VolumeName: "vol-1"},
				}).ConstructExternal(),
			},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.AlreadyExists,
		},
		{
			name: "Error - Volume count mismatch",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				(&storage.Snapshot{
					Created:   "2023-05-15T17:04:09Z",
					SizeBytes: 1024,
					State:     storage.SnapshotStateOnline,
					Config:    &storage.SnapshotConfig{Name: "snap-1", InternalName: "internal_snap1", VolumeName: "vol-1"},
				}).ConstructExternal(),
			},
			volumeNames:   []string{"vol-1", "vol-2"},
			expectedError: codes.AlreadyExists,
		},
		{
			name: "Error - Missing volume in group",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				(&storage.Snapshot{
					Created:   "2023-05-15T17:04:09Z",
					SizeBytes: 1024,
					State:     storage.SnapshotStateOnline,
					Config:    &storage.SnapshotConfig{Name: "snap-1", InternalName: "internal_snap1", VolumeName: "vol-1"},
				}).ConstructExternal(),
			},
			volumeNames:   []string{"vol-2"},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Invalid constituent snapshot",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				(&invalidSnapshot).ConstructExternal(),
			},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.Internal,
		},
		{
			name: "Error - Snapshot not ready",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				(&offlineSnapshot).ConstructExternal(),
			},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.Internal,
		},
		{
			name: "Success - Valid group snapshot",
			groupSnapshot: &storage.GroupSnapshotExternal{
				GroupSnapshot: storage.GroupSnapshot{
					GroupSnapshotConfig: &storage.GroupSnapshotConfig{Name: "group-1"},
				},
			},
			snapshots: []*storage.SnapshotExternal{
				(&onlineSnapshot).ConstructExternal(),
			},
			volumeNames:   []string{"vol-1"},
			expectedError: codes.OK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}

			err := plugin.validateGroupSnapshot(context.Background(), tc.groupSnapshot, tc.snapshots, tc.volumeNames)

			if tc.expectedError != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expectedError, status.Code())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
