// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
)

const (
	localVolumeHandle   = "svm-2:volume-b"
	remoteVolumeHandle  = "svm-1:volume-a"
	snapshotHandle      = ""
	replicationPolicy   = "MirrorAllSnapshots"
	replicationSchedule = "1min"
	localSVMName        = "svm-2"
	localFlexvolName    = "volume-b"
	remoteSVMName       = "svm-1"
	remoteFlexvolName   = "volume-a"
)

var (
	errNotReady = api.NotReadyError("operation still in progress, fail")
	errNotFound = api.NotFoundError("not found")
)

func TestPromoteMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(2).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	firstCall := mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	secondCall := mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1).After(firstCall)
	thirdCall := mockAPI.EXPECT().SnapmirrorBreak(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName, snapshotHandle).Times(1).After(secondCall)
	mockAPI.EXPECT().SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1).After(thirdCall)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_NoRemoteHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	noRemoteVolumeHandle := ""

	wait, err := promoteMirror(ctx, localVolumeHandle, noRemoteVolumeHandle, snapshotHandle, replicationPolicy,
		mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_QuiesceErrorNotReady(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1).Return(errNotReady)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "promote mirror should return a not ready error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestPromoteMirror_AbortErrorNotReady(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1).Return(errNotReady)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "promote mirror should return a not ready error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestPromoteMirror_BreakErrorNotReady(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(2).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorBreak(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName, snapshotHandle).Times(1).Return(errNotReady)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "promote mirror should return a not ready error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestPromoteMirror_ReplicationPolicySync(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	NewReplicationPolicy := "sync_mirror"

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(2).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, NewReplicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeSync}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorBreak(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		snapshotHandle).Times(1)
	mockAPI.EXPECT().SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, "snapHandle", NewReplicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_WaitForSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, localFlexvolName).Times(1).Return(api.Snapshots{
		api.Snapshot{Name: "snapshot-1", CreateTime: "1"},
	}, nil)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, "volume-a/snapshot-a", replicationPolicy,
		mockAPI)

	assert.True(t, wait, "wait should be true")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_FoundSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(2).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, localFlexvolName).Times(1).Return(api.Snapshots{
		api.Snapshot{Name: "snapshot-a", CreateTime: "1"},
	}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorBreak(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		"snapshot-a").Times(1)
	mockAPI.EXPECT().SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1)

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, "volume-a/snapshot-a", replicationPolicy,
		mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(nil, api.ApiError("snapmirror get error"))

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "snapmirror get error")
}

func TestPromoteMirror_SnapmirrorPolicyGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync},
			api.ApiError("error on snapmirror policy get"))

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "snapmirror policy get error")
}

func TestPromoteMirror_SnapshotPresentError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, localFlexvolName).Times(1).Return(nil,
		api.ApiError("snapshot present error"))

	wait, err := promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, "volume-a/snapshot-a",
		replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "snapshot present error")
}

func TestPromoteMirror_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := ""

	wait, err := promoteMirror(ctx, wrongVolumeHandle, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestPromoteMirror_InvalidRemoteVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := "pvc-a"

	wait, err := promoteMirror(ctx, localVolumeHandle, wrongVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestReleaseMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorRelease(ctx, localFlexvolName, localSVMName)

	err := releaseMirror(ctx, localVolumeHandle, mockAPI)
	assert.NoError(t, err, "release mirror should not return an error")
}

func TestReleaseMirror_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongLocalVolumeHandle := ""

	err := releaseMirror(ctx, wrongLocalVolumeHandle, mockAPI)

	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestReleaseMirror_ReleaseError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorRelease(ctx, localFlexvolName,
		localSVMName).Return(api.ApiError("error releasing snapmirror info for volume"))
	err := releaseMirror(ctx, localVolumeHandle, mockAPI)

	assert.Error(t, err, "should return an error if release fails")
}

func TestGetReplicationDetails_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{ReplicationPolicy: replicationPolicy, ReplicationSchedule: replicationSchedule}, nil)

	policy, schedule, err := getReplicationDetails(ctx, localVolumeHandle, remoteVolumeHandle, mockAPI)

	assert.Equal(t, replicationPolicy, policy, "policy should match what snapmirror returns")
	assert.Equal(t, replicationSchedule, schedule, "schedule should match what snapmirror returns")
	assert.NoError(t, err, "get replication details should not return an error")
}

func TestGetReplicationDetails_EmptyRemoteHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	emptyVolumeHandle := ""

	policy, schedule, err := getReplicationDetails(ctx, localVolumeHandle, emptyVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.NoError(t, err, "get replication details should not return an error")
}

func TestGetReplicationDetails_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := ""

	policy, schedule, err := getReplicationDetails(ctx, wrongVolumeHandle, remoteVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestGetReplicationDetails_InvalidRemoteVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := "pvc-a"

	policy, schedule, err := getReplicationDetails(ctx, localVolumeHandle, wrongVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestGetReplicationDetails_SnapmirrorGetNotFoundError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(nil, errNotFound)

	policy, schedule, err := getReplicationDetails(ctx, localVolumeHandle, remoteVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Error(t, err, "snapmirror not found error")
}

func TestGetReplicationDetails_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(nil, api.ApiError("snapmirror get error"))

	policy, schedule, err := getReplicationDetails(ctx, localVolumeHandle, remoteVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Error(t, err, "snapmirror get error")
}

func TestValidateReplicationConfig_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync, CopyAllSnapshots: true}, nil)
	mockAPI.EXPECT().JobScheduleExists(ctx, replicationSchedule).Return(true, nil)

	err := validateReplicationConfig(ctx, replicationPolicy, replicationSchedule, mockAPI)

	assert.NoError(t, err, "validate replication config should not return an error")
}

func TestValidateReplicationConfig_ReplicationPolicyError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)

	err := validateReplicationConfig(ctx, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "validate replication config should return an error")
}

func TestValidateReplicationConfig_ReplicationPolicyTypeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).
		Return(&api.SnapmirrorPolicy{Type: "invalid"}, nil)

	err := validateReplicationConfig(ctx, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "validate replication config should return an error")
}

func TestValidateReplicationConfig_ReplicationScheduleError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeSync}, nil)
	mockAPI.EXPECT().JobScheduleExists(ctx, replicationSchedule).
		Return(false, api.ApiError("replication schedule error"))

	err := validateReplicationConfig(ctx, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "validate replication config should return an error")
}

func TestEstablishMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: true}, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorInitialize(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)

	err := establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.NoError(t, err, "establish mirror should not return an error")
}

func TestEstablishMirror_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := ""

	err := establishMirror(ctx, wrongVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_InvalidRemoteVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := "pvc-a"

	err := establishMirror(ctx, localVolumeHandle, wrongVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_VolumeInfoError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(nil, api.ApiError("volume info error"))

	err := establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_NotDPVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: false}, nil)

	err := establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: true}, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, api.ApiError("snapmirror get error"))

	err := establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should not return an error")
}

func TestEstablishMirror_SnapmirrorInitializeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: true}, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorInitialize(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Return(api.ApiError("snapmirror initialize error"))

	err := establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_StillUninitialized(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: true}, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorInitialize(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized}, nil)

	err := establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestReestablishMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorResync(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored, IsHealthy: true}, nil)

	err := reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.NoError(t, err, "reestablish mirror should not return an error")
}

func TestReestablishMirror_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := ""

	err := reestablishMirror(ctx, wrongVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_InvalidRemoteVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := "pvc-a"

	err := reestablishMirror(ctx, localVolumeHandle, wrongVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, api.ApiError("snapmirror get error"))

	err := reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_SnapmirrorEstablished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{
			State: api.SnapmirrorStateSnapmirrored, RelationshipStatus: api.SnapmirrorStatusIdle,
			LastTransferType: "sync",
		}, nil)

	err := reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.NoError(t, err, "reestablish mirror should not return an error")
}

func TestReestablishMirror_ResyncError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorResync(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(api.ApiError("resync error"))

	err := reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_ReconcileIncompleteError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorResync(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)

	err := reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
	assert.True(t, utils.IsReconcileIncompleteError(err), "not reconcile incomplete error")
}

func TestReestablishMirror_SnapmirrorNotHealthy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorResync(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{
			State: api.SnapmirrorStateSnapmirrored, IsHealthy: false,
			UnhealthyReason: "Not healthy",
		}, nil)
	mockAPI.EXPECT().SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)

	err := reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}
