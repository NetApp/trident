// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

const (
	remoteVolumeHandle  = "svm-1:volume-a"
	snapshotHandle      = ""
	replicationPolicy   = "MirrorAllSnapshots"
	replicationSchedule = "1min"
	localSVMName        = "svm-2"
	localFlexvolName    = "volume-b"
	remoteSVMName       = "svm-1"
	remoteFlexvolName   = "volume-a"
	localSVMUUID        = "f714bf7b-9357-11ed-961b-005056b36ae8"
	transferTime        = "2023-06-01T14:15:36Z"
)

var (
	errNotReady        = api.NotReadyError("operation still in progress, fail")
	errNotFound        = api.NotFoundError("not found")
	endTransferTime, _ = time.Parse(utils.TimestampFormat, transferTime)
)

func TestPromoteMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_NoRemoteHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	noRemoteVolumeHandle := ""

	wait, err := promoteMirror(ctx, localFlexvolName, noRemoteVolumeHandle, snapshotHandle, replicationPolicy,
		mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_QuiesceErrorNotReady(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1).Return(errNotReady)

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "promote mirror should return a not ready error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestPromoteMirror_AbortErrorNotReady(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)
	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1).Return(errNotReady)

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "promote mirror should return a not ready error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestPromoteMirror_BreakErrorNotReady(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "promote mirror should return a not ready error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestPromoteMirror_ReplicationPolicySync(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	NewReplicationPolicy := "sync_mirror"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, "snapHandle", NewReplicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_WaitForSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)

	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		"snapshot-a", localFlexvolName).Return(
		api.Snapshot{
			CreateTime: "1",
			Name:       "snapshot-1",
		},
		nil)

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, "volume-a/snapshot-a", replicationPolicy,
		mockAPI)

	assert.True(t, wait, "wait should be true")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_FoundSnapshot(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(2).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)

	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		"snapshot-a", localFlexvolName).Return(
		api.Snapshot{
			CreateTime: "1",
			Name:       "snapshot-a",
		},
		nil)

	mockAPI.EXPECT().SnapmirrorQuiesce(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
		remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorAbort(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1)
	mockAPI.EXPECT().SnapmirrorBreak(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		"snapshot-a").Times(1)
	mockAPI.EXPECT().SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1)

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, "volume-a/snapshot-a", replicationPolicy,
		mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.NoError(t, err, "promote mirror should not return an error")
}

func TestPromoteMirror_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(nil, api.ApiError("snapmirror get error"))

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "snapmirror get error")
}

func TestPromoteMirror_SnapmirrorPolicyGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync},
			api.ApiError("error on snapmirror policy get"))

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "snapmirror policy get error")
}

func TestPromoteMirror_SnapshotPresentError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{State: api.SnapmirrorStateSnapmirrored}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, replicationPolicy).Times(1).
		Return(&api.SnapmirrorPolicy{Type: api.SnapmirrorPolicyZAPITypeAsync}, nil)

	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		"snapshot-a", localFlexvolName).Return(
		api.Snapshot{},
		api.ApiError("snapshot present error"))

	wait, err := promoteMirror(ctx, localFlexvolName, remoteVolumeHandle, "volume-a/snapshot-a",
		replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "snapshot present error")
}

func TestPromoteMirror_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeName := ""

	wait, err := promoteMirror(ctx, wrongVolumeName, remoteVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestPromoteMirror_InvalidRemoteVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := "pvc-a"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	wait, err := promoteMirror(ctx, localFlexvolName, wrongVolumeHandle, snapshotHandle, replicationPolicy, mockAPI)

	assert.False(t, wait, "wait should be false")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestReleaseMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorRelease(ctx, localFlexvolName, localSVMName)

	err := releaseMirror(ctx, localFlexvolName, mockAPI)
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

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorRelease(ctx, localFlexvolName,
		localSVMName).Return(api.ApiError("error releasing snapmirror info for volume"))
	err := releaseMirror(ctx, localFlexvolName, mockAPI)

	assert.Error(t, err, "should return an error if release fails")
}

func TestGetReplicationDetails_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(&api.Snapmirror{ReplicationPolicy: replicationPolicy, ReplicationSchedule: replicationSchedule}, nil)

	policy, schedule, SVMName, err := getReplicationDetails(ctx, localFlexvolName, remoteVolumeHandle, mockAPI)

	assert.Equal(t, replicationPolicy, policy, "policy should match what snapmirror returns")
	assert.Equal(t, replicationSchedule, schedule, "schedule should match what snapmirror returns")
	assert.Equal(t, SVMName, localSVMName, "SVM name should match")
	assert.NoError(t, err, "get replication details should not return an error")
}

func TestGetReplicationDetails_EmptyRemoteHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	emptyVolumeHandle := ""

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	policy, schedule, SVMName, err := getReplicationDetails(ctx, localFlexvolName, emptyVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Equal(t, SVMName, localSVMName, "SVM name should match")
	assert.NoError(t, err, "get replication details should not return an error")
}

func TestGetReplicationDetails_InvalidLocalVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := ""

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	policy, schedule, SVMName, err := getReplicationDetails(ctx, wrongVolumeHandle, remoteVolumeHandle,
		mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Equal(t, SVMName, localSVMName, "SVM name should match")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestGetReplicationDetails_InvalidRemoteVolumeHandle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	wrongVolumeHandle := "pvc-a"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	policy, schedule, SVMName, err := getReplicationDetails(ctx, localFlexvolName, wrongVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Equal(t, SVMName, localSVMName, "SVM name should match")
	assert.Error(t, err, "should return an error if cannot parse volume handle")
}

func TestGetReplicationDetails_SnapmirrorGetNotFoundError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(nil, errNotFound)

	policy, schedule, SVMName, err := getReplicationDetails(ctx, localFlexvolName, remoteVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Equal(t, SVMName, localSVMName, "SVM name should match")
	assert.Error(t, err, "snapmirror not found error")
}

func TestGetReplicationDetails_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).Times(1).
		Return(nil, api.ApiError("snapmirror get error"))

	policy, schedule, SVMName, err := getReplicationDetails(ctx, localFlexvolName, remoteVolumeHandle, mockAPI)

	assert.Empty(t, policy, "policy should be empty")
	assert.Empty(t, schedule, "schedule should be empty")
	assert.Equal(t, SVMName, localSVMName, "SVM name should match")
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

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	err := establishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

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

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	err := establishMirror(ctx, localFlexvolName, wrongVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_VolumeInfoError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(nil, api.ApiError("volume info error"))

	err := establishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_NotDPVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: false}, nil)

	err := establishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().VolumeInfo(ctx, localFlexvolName).Return(&api.Volume{DPVolume: true}, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, api.ApiError("snapmirror get error"))

	err := establishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should not return an error")
}

func TestEstablishMirror_SnapmirrorInitializeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	err := establishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
}

func TestEstablishMirror_StillUninitialized(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	err := establishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "establish mirror should return an error")
	assert.True(t, api.IsNotReadyError(err), "not NotReadyError")
}

func TestReestablishMirror_NoErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	err := reestablishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
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

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	err := reestablishMirror(ctx, localFlexvolName, wrongVolumeHandle, replicationPolicy, replicationSchedule, mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, api.ApiError("snapmirror get error"))

	err := reestablishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_SnapmirrorEstablished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{
			State: api.SnapmirrorStateSnapmirrored, RelationshipStatus: api.SnapmirrorStatusIdle,
			LastTransferType: "sync",
		}, nil)

	err := reestablishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.NoError(t, err, "reestablish mirror should not return an error")
}

func TestReestablishMirror_ResyncError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(nil, errNotFound)
	mockAPI.EXPECT().SnapmirrorCreate(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule).Return(nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(&api.Snapmirror{State: api.SnapmirrorStateUninitialized, RelationshipStatus: api.SnapmirrorStatusIdle},
			nil)
	mockAPI.EXPECT().SnapmirrorResync(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName).
		Return(api.ApiError("resync error"))

	err := reestablishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestReestablishMirror_ReconcileIncompleteError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	err := reestablishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
	assert.True(t, errors.IsReconcileIncompleteError(err), "not reconcile incomplete error")
}

func TestReestablishMirror_SnapmirrorNotHealthy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return(localSVMName)
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

	err := reestablishMirror(ctx, localFlexvolName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		mockAPI)

	assert.Error(t, err, "reestablish mirror should return an error")
}

func TestMirrorUpdate_InProgressError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	snapName := "snapshot-123"
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SnapmirrorUpdate(ctx, localInternalVolumeName, snapName).Times(1)

	err := mirrorUpdate(ctx, localInternalVolumeName, snapName, mockAPI)

	assert.True(t, errors.IsInProgressError(err), "mirror update should be in progress")
}

func TestMirrorUpdate_NoVolName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	snapName := "snapshot-123"
	localInternalVolumeName := ""

	err := mirrorUpdate(ctx, localInternalVolumeName, snapName, mockAPI)

	assert.False(t, errors.IsInProgressError(err), "invalid volume name expected")
}

func TestMirrorUpdate_UpdateError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	snapName := "snapshot-123"
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SnapmirrorUpdate(ctx, localInternalVolumeName, snapName).Times(1).Return(fmt.Errorf("failed"))

	err := mirrorUpdate(ctx, localInternalVolumeName, snapName, mockAPI)

	assert.False(t, errors.IsInProgressError(err), "mirror update failed")
}

func TestCheckMirrorTransferState_SucceededIdle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			RelationshipStatus: api.SnapmirrorStatusIdle,
			EndTransferTime:    &endTransferTime,
			IsHealthy:          true,
		}, nil)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.NoError(t, err, "transfer status is finished")
	assert.True(t, endTransferTime.Equal(*endTime), "transfer time should return")
}

func TestCheckMirrorTransferState_SucceededSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			RelationshipStatus: api.SnapmirrorStatusSuccess,
			EndTransferTime:    &endTransferTime,
			IsHealthy:          true,
		}, nil)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.NoError(t, err, "transfer status is finished")
	assert.True(t, endTransferTime.Equal(*endTime), "transfer time should return")
}

func TestCheckMirrorTransferState_NotHealthy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			RelationshipStatus: api.SnapmirrorStatusSuccess,
			EndTransferTime:    &endTransferTime,
			IsHealthy:          false,
		}, nil)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.Error(t, err, "transfer status should error")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestCheckMirrorTransferState_NoVolName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := ""

	mockAPI.EXPECT().SVMName().Return(localSVMName)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.Error(t, err, "invalid volume name")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestCheckMirrorTransferState_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(nil, fmt.Errorf("failed"))

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.Error(t, err, "snapmirror get failed")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestCheckMirrorTransferState_Transferring(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			RelationshipStatus: api.SnapmirrorStatusTransferring,
			EndTransferTime:    nil,
		}, nil)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.True(t, errors.IsInProgressError(err), "transfer status is in progress")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestCheckMirrorTransferState_TransferFailed(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			RelationshipStatus: api.SnapmirrorStatusFailed,
			UnhealthyReason:    "transfer failed",
			EndTransferTime:    nil,
		}, nil)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.False(t, errors.IsInProgressError(err), "transfer status is not in progress, failed")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestCheckMirrorTransferState_DefaultError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			RelationshipStatus: api.SnapmirrorStatusQueued,
		}, nil)

	endTime, err := checkMirrorTransferState(ctx, localInternalVolumeName, mockAPI)

	assert.True(t, errors.IsInProgressError(err), "transfer status not expected")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestGetMirrorTransferTime_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			EndTransferTime: &endTransferTime,
		}, nil)

	endTime, err := getMirrorTransferTime(ctx, localInternalVolumeName, mockAPI)

	assert.NoError(t, err, "getMirrorTransferTime should not have error")
	assert.True(t, endTransferTime.Equal(*endTime), "transfer time should return")
}

func TestGetMirrorTransferTime_NoVolName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := ""

	mockAPI.EXPECT().SVMName().Return(localSVMName)

	endTime, err := getMirrorTransferTime(ctx, localInternalVolumeName, mockAPI)

	assert.Error(t, err, "invalid volume name")
	assert.Nil(t, endTime, "transfer time should not return")
}

func TestGetMirrorTransferTime_NoTransferTime(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(&api.Snapmirror{
			EndTransferTime: nil,
		}, nil)

	endTime, err := getMirrorTransferTime(ctx, localInternalVolumeName, mockAPI)

	assert.NoError(t, err, "getMirrorTransferTime should not have error")
	assert.Nil(t, endTime, "transfer time should be nil")
}

func TestGetMirrorTransferTime_SnapmirrorGetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	ctx := context.Background()
	localInternalVolumeName := "pvc_123"

	mockAPI.EXPECT().SVMName().Return(localSVMName)
	mockAPI.EXPECT().SnapmirrorGet(ctx, localInternalVolumeName, localSVMName, "", "").
		Return(nil, fmt.Errorf("failed"))

	endTime, err := getMirrorTransferTime(ctx, localInternalVolumeName, mockAPI)

	assert.Error(t, err, "snapmirror get failed")
	assert.Nil(t, endTime, "transfer time should not return")
}
