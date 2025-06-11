package csi

import (
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_core"
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
			t.Fatalf(err.Error())
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
		t.Fatalf(err.Error())
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
		t.Fatalf(err.Error())
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
