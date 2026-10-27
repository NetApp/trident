// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-openapi/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/cluster"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	tridenterrors "github.com/netapp/trident/utils/errors"
)

func TestVolumeCloneCreate_PollTimeoutKeepsJobNotDoneError(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	timeout := &api.JobNotDoneError{JobUUID: "job-1"}
	rsi.EXPECT().VolumeCloneCreateAsync(ctx, "clone", "source", "snap").Return(timeout)

	err := oapi.VolumeCloneCreate(ctx, "clone", "source", "snap", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "job job-1 not yet done")
	jobErr, ok := api.AsJobNotDoneError(err)
	require.True(t, ok, "the typed error survives the abstraction's wrapping")
	assert.Equal(t, "job-1", jobErr.JobUUID)
}

func TestAsJobNotDoneError(t *testing.T) {
	_, ok := api.AsJobNotDoneError(nil)
	assert.False(t, ok)
	_, ok = api.AsJobNotDoneError(errors.New("other"))
	assert.False(t, ok)
}

func TestCloneJobStatus(t *testing.T) {
	const volumeUUID = "066f230c-c4d1-11f1-9aea-00a0b8899761"

	tests := map[string]struct {
		payload  *models.Job
		getErr   error
		expected *api.CloneJobStatus
		wantErr  bool
	}{
		"failed job reports code and the volume UUID": {
			payload: &models.Job{
				State:       convert.ToPtr(models.JobStateFailure),
				Code:        convert.ToPtr(int64(1520486)),
				Message:     convert.ToPtr("[Job 2967] Job failed: Volume busy"),
				Description: convert.ToPtr("POST /api/storage/volumes/" + volumeUUID),
			},
			expected: &api.CloneJobStatus{State: models.JobStateFailure, Code: "1520486",
				Message: "[Job 2967] Job failed: Volume busy", VolumeUUID: volumeUUID,
			},
		},
		"running job": {
			payload:  &models.Job{State: convert.ToPtr(models.JobStateRunning)},
			expected: &api.CloneJobStatus{State: models.JobStateRunning},
		},
		"description without a volume path": {
			payload: &models.Job{
				State:       convert.ToPtr(models.JobStateSuccess),
				Description: convert.ToPtr("Create trident_pvc_x"),
			},
			expected: &api.CloneJobStatus{State: models.JobStateSuccess},
		},
		"description with a path that is not a UUID": {
			payload: &models.Job{
				State:       convert.ToPtr(models.JobStateSuccess),
				Description: convert.ToPtr("POST /api/storage/volumes/not-a-uuid"),
			},
			expected: &api.CloneJobStatus{State: models.JobStateSuccess},
		},
		"description naming a snapshot path still yields the volume UUID": {
			payload: &models.Job{
				State:       convert.ToPtr(models.JobStateSuccess),
				Description: convert.ToPtr("DELETE /api/storage/volumes/" + volumeUUID + "/snapshots/other"),
			},
			expected: &api.CloneJobStatus{State: models.JobStateSuccess, VolumeUUID: volumeUUID},
		},
		"missing state": {payload: &models.Job{}, wantErr: true},
		"lookup error":  {getErr: fmt.Errorf("boom"), wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			oapi, rsi := newMockOntapAPIREST(t)
			var result *cluster.JobGetOK
			if tc.payload != nil {
				result = &cluster.JobGetOK{Payload: tc.payload}
			}
			rsi.EXPECT().JobGet(ctx, "job-1", []string{"state", "code", "message", "description"}).
				Return(result, tc.getErr)

			status, err := oapi.CloneJobStatus(ctx, "job-1")

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, status)
		})
	}
}

func TestCloneJobStatus_States(t *testing.T) {
	assert.True(t, (&api.CloneJobStatus{State: models.JobStateSuccess}).Succeeded())
	assert.True(t, (&api.CloneJobStatus{State: models.JobStateFailure}).Failed())
}

func TestVolumeCloneSplitState(t *testing.T) {
	tests := map[string]struct {
		volume   *models.Volume
		getErr   error
		expected *api.CloneSplitState
		notFound bool
		wantErr  bool
	}{
		"unsplit flexclone": {
			volume: &models.Volume{Clone: &models.VolumeInlineClone{IsFlexclone: convert.ToPtr(true)}},
			expected: &api.CloneSplitState{
				IsFlexclone: true,
			},
		},
		"parent snapshot reported": {
			volume: &models.Volume{Clone: &models.VolumeInlineClone{
				IsFlexclone:    convert.ToPtr(true),
				ParentSnapshot: &models.SnapshotReference{Name: convert.ToPtr("snapshot-auto")},
			}},
			expected: &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-auto"},
		},
		"split requested": {
			volume: &models.Volume{Clone: &models.VolumeInlineClone{
				IsFlexclone: convert.ToPtr(true), SplitInitiated: convert.ToPtr(true),
			}},
			expected: &api.CloneSplitState{IsFlexclone: true, SplitInitiated: true},
		},
		"no clone state reported": {volume: &models.Volume{}, wantErr: true},
		"is_flexclone not reported": {
			volume:  &models.Volume{Clone: &models.VolumeInlineClone{SplitInitiated: convert.ToPtr(true)}},
			wantErr: true,
		},
		"split finished": {
			volume:   &models.Volume{Clone: &models.VolumeInlineClone{IsFlexclone: convert.ToPtr(false)}},
			expected: &api.CloneSplitState{},
		},
		"not found":    {notFound: true, wantErr: true},
		"lookup error": {getErr: fmt.Errorf("boom"), wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			oapi, rsi := newMockOntapAPIREST(t)
			rsi.EXPECT().VolumeGetByName(
				ctx, "vol", []string{"clone.is_flexclone", "clone.split_initiated", "clone.parent_snapshot.name"},
			).Return(tc.volume, tc.getErr)

			state, err := oapi.VolumeCloneSplitState(ctx, "vol")

			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tc.notFound, tridenterrors.IsNotFoundError(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, state)
		})
	}
}

func TestCloneJobStatus_ExpiredJobIsNotFound(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)
	rsi.EXPECT().JobGet(ctx, "job-1", []string{"state", "code", "message", "description"}).
		Return(nil, runtime.NewAPIError("GET job", errors.New("not found"), 404))

	_, err := oapi.CloneJobStatus(ctx, "job-1")

	assert.True(t, tridenterrors.IsNotFoundError(err), "got %v", err)
}
