// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/errors"
)

// cloneJobMockAPI is a mock ONTAP API that also reports clone job and split state, as the REST abstraction does.
type cloneJobMockAPI struct {
	*mockapi.MockOntapAPI
	createUUID  string
	createErr   error
	status      *api.CloneJobStatus
	statusErr   error
	statusCalls int
	split       *api.CloneSplitState
	splitErr    error
	splitReads  []*api.CloneSplitState // consumed one per read, before split is used
}

func (m *cloneJobMockAPI) VolumeCloneCreateWithUUID(_ context.Context, _, _, _ string) (string, error) {
	return m.createUUID, m.createErr
}

func (m *cloneJobMockAPI) CloneJobStatus(_ context.Context, _ string) (*api.CloneJobStatus, error) {
	m.statusCalls++
	return m.status, m.statusErr
}

func (m *cloneJobMockAPI) VolumeCloneSplitState(_ context.Context, _ string) (*api.CloneSplitState, error) {
	if len(m.splitReads) > 0 {
		next := m.splitReads[0]
		m.splitReads = m.splitReads[1:]
		return next, nil
	}
	return m.split, m.splitErr
}

const (
	cloneTestName     = "trident_pvc_clone"
	cloneTestSource   = "trident_pvc_source"
	cloneTestSnap     = "snap-from-pvc"
	cloneTestLabel    = "labels"
	cloneTestJob      = "job-1"
	cloneTestVolumeID = "6c7d1d3e-0000-4000-8000-000000000001"
)

func cloneTestConfig() *drivers.OntapStorageDriverConfig {
	return &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: tridentconfig.OntapSANStorageDriverName,
		},
	}
}

func cloneTestVolumeConfig(snapshot string) *storage.VolumeConfig {
	return &storage.VolumeConfig{
		InternalName:                cloneTestName,
		CloneSourceVolumeInternal:   cloneTestSource,
		CloneSourceSnapshotInternal: snapshot,
	}
}

func newCloneJobMock(t *testing.T) (*cloneJobMockAPI, *mockapi.MockOntapAPI) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	inner := mockapi.NewMockOntapAPI(mockCtrl)
	return &cloneJobMockAPI{MockOntapAPI: inner}, inner
}

// expectCloneFinishSteps expects the steps that follow a clone once ONTAP has created it.
func expectCloneFinishSteps(m *mockapi.MockOntapAPI) {
	m.EXPECT().VolumeWaitForStates(gomock.Any(), cloneTestName, gomock.Any(), gomock.Any(), gomock.Any()).
		Return("online", nil)
	m.EXPECT().VolumeSetComment(gomock.Any(), cloneTestName, cloneTestName, cloneTestLabel).Return(nil)
}

func timedOutCloneError() error {
	return fmt.Errorf("error creating clone; %w", &api.JobNotDoneError{JobUUID: cloneTestJob})
}

func runCloneFlexvol(
	cfg *storage.VolumeConfig, split bool, client api.OntapAPI, pending *sync.Map,
) error {
	return cloneFlexvol(
		context.Background(), cfg, cloneTestLabel, split, cloneTestConfig(), client, api.QosPolicyGroup{}, pending,
	)
}

func TestCloneFlexvol_PollTimeoutKeepsCloneAndRecordsJob(t *testing.T) {
	tests := map[string]struct {
		snapshot       string
		expectSnapshot bool
	}{
		"requested snapshot":       {snapshot: cloneTestSnap},
		"trident created snapshot": {snapshot: "", expectSnapshot: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			pending := &sync.Map{}
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(false, nil)
			if tc.expectSnapshot {
				inner.EXPECT().VolumeSnapshotCreate(gomock.Any(), gomock.Any(), cloneTestSource).Return(nil)
			}
			inner.EXPECT().VolumeCloneCreate(gomock.Any(), cloneTestName, cloneTestSource, gomock.Any(), false).
				Return(timedOutCloneError())
			// No VolumeDestroy or VolumeSnapshotDelete is expected: the clone and its snapshot are kept.

			err := runCloneFlexvol(cloneTestVolumeConfig(tc.snapshot), true, client, pending)

			assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
			rec, ok := loadPendingClone(pending, cloneTestName)
			require.True(t, ok)
			assert.Equal(t, cloneTestJob, rec.jobUUID)
			assert.Equal(t, tc.expectSnapshot, rec.createdSnapName != "")
		})
	}
}

func TestCloneFlexvol_ExistingCloneWithTrackedJob(t *testing.T) {
	tests := map[string]struct {
		status      *api.CloneJobStatus
		statusErr   error
		expectCalls func(m *mockapi.MockOntapAPI)
		split       *api.CloneSplitState
		check       func(t *testing.T, err error, pending *sync.Map)
	}{
		"job still running": {
			status: &api.CloneJobStatus{State: models.JobStateRunning},
			check: func(t *testing.T, err error, pending *sync.Map) {
				assert.True(t, errors.IsVolumeCreatingError(err))
				rec, ok := loadPendingClone(pending, cloneTestName)
				require.True(t, ok)
				assert.Equal(t, cloneTestJob, rec.jobUUID)
			},
		},
		"job state cannot be read": {
			// The recorded job's state is unknown, so it must not be taken as finished.
			statusErr: fmt.Errorf("connection refused"),
			check: func(t *testing.T, err error, pending *sync.Map) {
				assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
				rec, ok := loadPendingClone(pending, cloneTestName)
				require.True(t, ok)
				assert.Equal(t, cloneTestJob, rec.jobUUID, "the record is kept")
			},
		},
		"recorded job expired": {
			statusErr:   errors.NotFoundError("job %s not found", cloneTestJob),
			split:       &api.CloneSplitState{},
			expectCalls: expectCloneFinishSteps,
			check: func(t *testing.T, err error, pending *sync.Map) {
				assert.True(t, drivers.IsVolumeExistsError(err), "got %v", err)
				_, ok := loadPendingClone(pending, cloneTestName)
				assert.False(t, ok, "the stale record is dropped")
			},
		},
		"job failed removes leftover by UUID and retries": {
			status: &api.CloneJobStatus{
				State: models.JobStateFailure, Code: "1520486", Message: "Volume busy", VolumeUUID: cloneTestVolumeID,
			},
			expectCalls: func(m *mockapi.MockOntapAPI) {
				m.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil)
			},
			check: func(t *testing.T, err error, pending *sync.Map) {
				assert.True(t, errors.IsVolumeCreatingError(err))
				assert.Contains(t, err.Error(), "1520486", "the error carries the failure code")
				assert.NotContains(t, err.Error(), cloneTestJob, "the message is the same for every attempt")
				rec, ok := loadPendingClone(pending, cloneTestName)
				require.True(t, ok)
				assert.Empty(t, rec.jobUUID, "a failed job is no longer waited on")
				assert.Equal(t, cloneTestSnap, rec.createdSnapName, "the parent snapshot is kept for the retry")
			},
		},
		"job succeeded splits an unsplit clone": {
			status: &api.CloneJobStatus{State: models.JobStateSuccess},
			split:  &api.CloneSplitState{IsFlexclone: true},
			expectCalls: func(m *mockapi.MockOntapAPI) {
				expectCloneFinishSteps(m)
				m.EXPECT().VolumeCloneSplitStart(gomock.Any(), cloneTestName).Return(nil)
			},
			check: func(t *testing.T, err error, pending *sync.Map) {
				assert.True(t, drivers.IsVolumeExistsError(err), "got %v", err)
				_, ok := loadPendingClone(pending, cloneTestName)
				assert.False(t, ok)
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			client.status, client.statusErr, client.split = tc.status, tc.statusErr, tc.split
			pending := &sync.Map{}
			storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
			if tc.expectCalls != nil {
				tc.expectCalls(inner)
			}

			err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), true, client, pending)

			tc.check(t, err, pending)
		})
	}
}

func TestCloneFlexvol_ExistingCloneWithoutTrackedJob(t *testing.T) {
	tests := map[string]struct {
		split       bool
		splitState  *api.CloneSplitState
		splitErr    error
		waitErr     error
		expectCalls func(m *mockapi.MockOntapAPI)
		expectType  string
	}{
		"unsplit clone is split": {
			split:      true,
			splitState: &api.CloneSplitState{IsFlexclone: true},
			expectCalls: func(m *mockapi.MockOntapAPI) {
				expectCloneFinishSteps(m)
				m.EXPECT().VolumeCloneSplitStart(gomock.Any(), cloneTestName).Return(nil)
			},
			expectType: "exists",
		},
		"split already initiated": {
			split:       true,
			splitState:  &api.CloneSplitState{IsFlexclone: true, SplitInitiated: true},
			expectCalls: expectCloneFinishSteps,
			expectType:  "exists",
		},
		"clone already split": {
			split:       true,
			splitState:  &api.CloneSplitState{},
			expectCalls: expectCloneFinishSteps,
			expectType:  "exists",
		},
		"split not requested": {
			split:       false,
			expectCalls: expectCloneFinishSteps,
			expectType:  "exists",
		},
		"split state unavailable": {
			split:       true,
			splitErr:    fmt.Errorf("could not find volume"),
			expectCalls: expectCloneFinishSteps,
			expectType:  "creating",
		},
		"split start fails": {
			split:      true,
			splitState: &api.CloneSplitState{IsFlexclone: true},
			expectCalls: func(m *mockapi.MockOntapAPI) {
				expectCloneFinishSteps(m)
				m.EXPECT().VolumeCloneSplitStart(gomock.Any(), cloneTestName).
					Return(fmt.Errorf("could not find volume with name %s", cloneTestName))
			},
			expectType: "creating",
		},
		"volume not online": {
			split:   true,
			waitErr: fmt.Errorf("timed out"),
			expectCalls: func(m *mockapi.MockOntapAPI) {
				m.EXPECT().VolumeWaitForStates(gomock.Any(), cloneTestName, gomock.Any(), gomock.Any(), gomock.Any()).
					Return("offline", fmt.Errorf("timed out"))
			},
			expectType: "creating",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			client.split, client.splitErr = tc.splitState, tc.splitErr
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
			tc.expectCalls(inner)

			err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), tc.split, client, &sync.Map{})

			switch tc.expectType {
			case "exists":
				assert.True(t, drivers.IsVolumeExistsError(err), "got %v", err)
			case "creating":
				assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
			}
		})
	}
}

func TestCloneFlexvol_RetryAfterFailedJobReusesParentSnapshot(t *testing.T) {
	client, inner := newCloneJobMock(t)
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{createdSnapName: cloneTestSnap})

	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(false, nil)
	// No VolumeSnapshotCreate: the snapshot the failed attempt created is used again.
	inner.EXPECT().VolumeCloneCreate(gomock.Any(), cloneTestName, cloneTestSource, cloneTestSnap, false).Return(nil)
	expectCloneFinishSteps(inner)

	volConfig := cloneTestVolumeConfig("")
	require.NoError(t, runCloneFlexvol(volConfig, false, client, pending))

	assert.Equal(t, cloneTestSnap, volConfig.CloneSourceSnapshotInternal)
	_, ok := loadPendingClone(pending, cloneTestName)
	assert.False(t, ok)
}

func TestCloneFlexvol_ExistingCloneWithoutJobAPI(t *testing.T) {
	// An API that cannot report clone job state keeps the plain exists behavior.
	mockCtrl := gomock.NewController(t)
	inner := mockapi.NewMockOntapAPI(mockCtrl)
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)

	err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), true, inner, &sync.Map{})

	assert.True(t, drivers.IsVolumeExistsError(err))
}

func TestCleanupFailedCloneFlexVol_KeepsCloneStillCreating(t *testing.T) {
	// No VolumeDestroy or VolumeSnapshotDelete expectations: any call fails the test.
	mockCtrl := gomock.NewController(t)
	inner := mockapi.NewMockOntapAPI(mockCtrl)

	cleanupFailedCloneFlexVol(
		context.Background(), inner, errors.VolumeCreatingError("still creating"), cloneTestName, cloneTestSource,
		cloneTestSnap,
	)
}

// A name lookup can trail a volume ONTAP is still building, so a recorded job decides, not the missing name.
func TestCloneFlexvol_RecordedJobWhenNameNotVisible(t *testing.T) {
	tests := map[string]struct {
		status       *api.CloneJobStatus
		statusErr    error
		expectCalls  func(m *mockapi.MockOntapAPI)
		expectCreate bool
		expectType   string
	}{
		"job still running": {
			status:     &api.CloneJobStatus{State: models.JobStateRunning},
			expectType: "creating",
		},
		"job finished but the volume is not visible yet": {
			status:     &api.CloneJobStatus{State: models.JobStateSuccess},
			expectType: "creating",
		},
		"job cannot be read": {
			statusErr:  fmt.Errorf("connection refused"),
			expectType: "creating",
		},
		"job failed removes the leftover and retries": {
			status: &api.CloneJobStatus{State: models.JobStateFailure, Code: "1520486", VolumeUUID: cloneTestVolumeID},
			expectCalls: func(m *mockapi.MockOntapAPI) {
				m.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil)
			},
			expectType: "creating",
		},
		"job expired reuses the parent snapshot and creates again": {
			statusErr: errors.NotFoundError("job %s not found", cloneTestJob),
			expectCalls: func(m *mockapi.MockOntapAPI) {
				m.EXPECT().VolumeCloneCreate(gomock.Any(), cloneTestName, cloneTestSource, cloneTestSnap, false).Return(nil)
				expectCloneFinishSteps(m)
			},
			expectCreate: true,
			expectType:   "ok",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			client.status, client.statusErr = tc.status, tc.statusErr
			pending := &sync.Map{}
			storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(false, nil)
			// No other call is expected for a running job: no second create, no snapshot delete.
			if tc.expectCalls != nil {
				tc.expectCalls(inner)
			}

			err := runCloneFlexvol(cloneTestVolumeConfig(""), false, client, pending)

			switch tc.expectType {
			case "creating":
				assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
				_, ok := loadPendingClone(pending, cloneTestName)
				assert.True(t, ok, "the record is kept")
			case "ok":
				assert.NoError(t, err)
				_, ok := loadPendingClone(pending, cloneTestName)
				assert.False(t, ok)
			}
		})
	}
}

// Without a job lookup (ZAPI) a retry cannot tell a clone whose split never started from a finished one, so a
// split that fails must fail the create and remove the clone, as it always has.
func TestCloneFlexvol_SplitFailureWithoutJobAPIRemovesClone(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	inner := mockapi.NewMockOntapAPI(mockCtrl)
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(false, nil)
	inner.EXPECT().VolumeCloneCreate(gomock.Any(), cloneTestName, cloneTestSource, cloneTestSnap, false).Return(nil)
	expectCloneFinishSteps(inner)
	inner.EXPECT().VolumeCloneSplitStart(gomock.Any(), cloneTestName).Return(fmt.Errorf("split failed"))
	inner.EXPECT().VolumeDestroy(gomock.Any(), cloneTestName, true, true).Return(nil)

	err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), true, inner, &sync.Map{})

	require.Error(t, err)
	assert.False(t, errors.IsVolumeCreatingError(err), "got %v", err)
}

func TestCloneFlexvol_SplitFailureWithJobAPIIsRetried(t *testing.T) {
	client, inner := newCloneJobMock(t)
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(false, nil)
	inner.EXPECT().VolumeCloneCreate(gomock.Any(), cloneTestName, cloneTestSource, cloneTestSnap, false).Return(nil)
	expectCloneFinishSteps(inner)
	// No VolumeDestroy: the finished clone stays for the retry to split.
	inner.EXPECT().VolumeCloneSplitStart(gomock.Any(), cloneTestName).Return(fmt.Errorf("split failed"))

	err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), true, client, &sync.Map{})

	assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
}

// The failure message repeats for the same failure so the provisioner's repeated-failure accounting can count it:
// it names neither the job nor ONTAP's job-numbered message.
func TestCloneFlexvol_FailedJobMessageIsStable(t *testing.T) {
	failures := map[string]*api.CloneJobStatus{
		"job-1": {State: models.JobStateFailure, Code: "1520486", Message: "[Job 2967] Job failed: Volume busy", VolumeUUID: cloneTestVolumeID},
		"job-2": {State: models.JobStateFailure, Code: "1520486", Message: "[Job 3012] Job failed: Volume busy", VolumeUUID: cloneTestVolumeID},
	}
	var messages []string
	for _, jobUUID := range []string{"job-1", "job-2"} {
		status := failures[jobUUID]
		client, inner := newCloneJobMock(t)
		client.status = status
		pending := &sync.Map{}
		storePendingClone(pending, cloneTestName, pendingClone{jobUUID: jobUUID, createdSnapName: cloneTestSnap})
		inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
		inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil)

		err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)

		require.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
		messages = append(messages, err.Error())
	}
	assert.Equal(t, messages[0], messages[1])
	assert.NotContains(t, messages[0], "job-1")
	assert.NotContains(t, messages[0], "2967")
}

// While the leftover of a failed job cannot be removed, the record keeps the job so a retry does not take the
// leftover for a finished clone if ONTAP expires the job.
func TestCloneFlexvol_FailedJobKeptWhileLeftoverRemains(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.status = &api.CloneJobStatus{State: models.JobStateFailure, Code: "1520486", VolumeUUID: cloneTestVolumeID}
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
	inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).
		Return(errors.New("busy"))

	err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)

	assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
	rec, ok := loadPendingClone(pending, cloneTestName)
	require.True(t, ok)
	assert.True(t, rec.cleanupPending)
}

// A leftover whose delete failed is still deleted on the retry after ONTAP expires the failed job, and it is
// never reported as a finished clone.
func TestCloneFlexvol_LeftoverDeletedAfterFailedJobExpires(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.status = &api.CloneJobStatus{State: models.JobStateFailure, Code: "1520486", VolumeUUID: cloneTestVolumeID}
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil).Times(2)
	gomock.InOrder(
		inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).
			Return(errors.New("busy")),
		// The retry deletes by the UUID the record kept, without looking the volume up by name.
		inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil),
	)

	first := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)
	require.True(t, errors.IsVolumeCreatingError(first), "got %v", first)

	// ONTAP no longer holds the failed job.
	client.status, client.statusErr = nil, errors.NotFoundError("job expired")
	second := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)

	assert.True(t, errors.IsVolumeCreatingError(second), "got %v", second)
	assert.False(t, drivers.IsVolumeExistsError(second), "the leftover must not be reported as a finished clone")
	rec, ok := loadPendingClone(pending, cloneTestName)
	require.True(t, ok)
	assert.False(t, rec.cleanupPending)
	assert.Equal(t, cloneTestSnap, rec.createdSnapName)
}

// On an existing clone, a setup step that fails is retried instead of reported as a failed create.
func TestCloneFlexvol_ExistingCloneSetupFailureIsRetried(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.status = &api.CloneJobStatus{State: models.JobStateSuccess, VolumeUUID: cloneTestVolumeID}
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob})
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
	inner.EXPECT().VolumeWaitForStates(gomock.Any(), cloneTestName, gomock.Any(), gomock.Any(), gomock.Any()).
		Return("online", nil)
	inner.EXPECT().VolumeSetComment(gomock.Any(), cloneTestName, cloneTestName, cloneTestLabel).
		Return(errors.New("transient"))

	err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)

	assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
}

// When the failed job names no volume, the leftover's UUID is read from the volume, never deleted by name. A
// name that is not visible yet may still be a volume ONTAP holds, so the cleanup stays pending until the
// visibility wait has passed; after that a volume that is not there is done.
func TestCloneFlexvol_FailedJobWithoutVolumeUUID(t *testing.T) {
	tests := map[string]struct {
		info          *api.Volume
		infoErr       error
		wait          time.Duration // zero: the visibility wait has already passed
		expectDestroy bool
		wantPending   bool
	}{
		"not visible within the wait (not found)":  {infoErr: errors.NotFoundError("no volume"), wait: time.Hour, wantPending: true},
		"not visible within the wait (read error)": {infoErr: api.VolumeReadError("could not find volume"), wait: time.Hour, wantPending: true},
		"gone after the wait (not found)":          {infoErr: errors.NotFoundError("no volume")},
		"gone after the wait (read error)":         {infoErr: api.VolumeReadError("could not find volume")},
		"leftover is deleted by its UUID":          {info: &api.Volume{Name: cloneTestName, UUID: cloneTestVolumeID}, wait: time.Hour, expectDestroy: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			saved := cloneLeftoverVisibilityWait
			cloneLeftoverVisibilityWait = tc.wait
			t.Cleanup(func() { cloneLeftoverVisibilityWait = saved })

			client, inner := newCloneJobMock(t)
			client.status = &api.CloneJobStatus{State: models.JobStateFailure, Code: "1520486"}
			pending := &sync.Map{}
			storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
			inner.EXPECT().VolumeInfo(gomock.Any(), cloneTestName).Return(tc.info, tc.infoErr)
			if tc.expectDestroy {
				inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil)
			}
			// No VolumeDestroy by name is expected.

			err := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)

			assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
			rec, ok := loadPendingClone(pending, cloneTestName)
			require.True(t, ok)
			assert.Empty(t, rec.jobUUID)
			assert.Equal(t, tc.wantPending, rec.cleanupPending)
			assert.Equal(t, cloneTestSnap, rec.createdSnapName)
		})
	}
}

// A leftover first seen by name keeps its UUID on the record, so the retry after a failed delete is addressed
// by it and does not depend on the name lookup again.
func TestCloneFlexvol_LeftoverUUIDReadByNameIsKept(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.status = &api.CloneJobStatus{State: models.JobStateFailure, Code: "1520486"}
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil).Times(2)
	inner.EXPECT().VolumeInfo(gomock.Any(), cloneTestName).
		Return(&api.Volume{Name: cloneTestName, UUID: cloneTestVolumeID}, nil).Times(1)
	gomock.InOrder(
		inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).
			Return(errors.New("busy")),
		inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil),
	)

	first := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)
	require.True(t, errors.IsVolumeCreatingError(first), "got %v", first)
	rec, ok := loadPendingClone(pending, cloneTestName)
	require.True(t, ok)
	assert.True(t, rec.cleanupPending)
	assert.Equal(t, cloneTestVolumeID, rec.leftoverUUID)

	// The failed job has expired. The second attempt has only the record.
	client.status, client.statusErr = nil, errors.NotFoundError("job expired")
	second := runCloneFlexvol(cloneTestVolumeConfig(cloneTestSnap), false, client, pending)

	assert.True(t, errors.IsVolumeCreatingError(second), "got %v", second)
	rec, ok = loadPendingClone(pending, cloneTestName)
	require.True(t, ok)
	assert.False(t, rec.cleanupPending)
	assert.Empty(t, rec.leftoverUUID, "a finished cleanup must not leave a UUID for a later failure to reuse")
}

// A retry that arrives with a config that does not name the parent snapshot gets the snapshot Trident created for
// the clone back, so deleting the clone later deletes the snapshot too.
func TestCloneFlexvol_ExistingCloneRestoresRecordedSnapshot(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.status = &api.CloneJobStatus{State: models.JobStateSuccess, VolumeUUID: cloneTestVolumeID}
	client.split = &api.CloneSplitState{}
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
	expectCloneFinishSteps(inner)
	volConfig := cloneTestVolumeConfig("")

	err := runCloneFlexvol(volConfig, true, client, pending)

	assert.True(t, drivers.IsVolumeExistsError(err), "got %v", err)
	assert.Equal(t, cloneTestSnap, volConfig.CloneSourceSnapshotInternal)
}

// An existing clone whose volume reached the abort state will never come online. The create returns no volume, so
// the clone is deleted here, by UUID, and the retry recreates it from the parent snapshot the record keeps.
func TestCloneFlexvol_ExistingCloneInTerminalStateIsRemovedAndRecreated(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.status = &api.CloneJobStatus{State: models.JobStateSuccess, VolumeUUID: cloneTestVolumeID}
	pending := &sync.Map{}
	storePendingClone(pending, cloneTestName, pendingClone{jobUUID: cloneTestJob, createdSnapName: cloneTestSnap})
	gomock.InOrder(
		inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil),
		inner.EXPECT().VolumeWaitForStates(gomock.Any(), cloneTestName, gomock.Any(), gomock.Any(), gomock.Any()).
			Return("error", api.TerminalState(fmt.Errorf("volume reached state error"))),
		inner.EXPECT().VolumeInfo(gomock.Any(), cloneTestName).
			Return(&api.Volume{Name: cloneTestName, UUID: cloneTestVolumeID}, nil),
		inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil),
		// The retry finds no volume and clones again from the snapshot the first attempt created.
		inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(false, nil),
		inner.EXPECT().VolumeCloneCreate(gomock.Any(), cloneTestName, cloneTestSource, cloneTestSnap, false).Return(nil),
	)
	expectCloneFinishSteps(inner)

	first := runCloneFlexvol(cloneTestVolumeConfig(""), false, client, pending)

	require.True(t, errors.IsVolumeCreatingError(first), "got %v", first)
	rec, ok := loadPendingClone(pending, cloneTestName)
	require.True(t, ok)
	assert.False(t, rec.cleanupPending)
	assert.Empty(t, rec.jobUUID)
	assert.Equal(t, cloneTestSnap, rec.createdSnapName)

	volConfig := cloneTestVolumeConfig("")
	require.NoError(t, runCloneFlexvol(volConfig, false, client, pending))
	assert.Equal(t, cloneTestSnap, volConfig.CloneSourceSnapshotInternal)
	_, ok = loadPendingClone(pending, cloneTestName)
	assert.False(t, ok)
}

// An aborted clone whose delete fails stays on record as pending cleanup, so it is never taken for a finished
// clone, and it keeps the parent snapshot recovered from the volume when no record held it.
func TestCloneFlexvol_ExistingCloneInTerminalStateKeepsCleanupPending(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.split = &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-auto"}
	pending := &sync.Map{}
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
	inner.EXPECT().VolumeWaitForStates(gomock.Any(), cloneTestName, gomock.Any(), gomock.Any(), gomock.Any()).
		Return("error", api.TerminalState(fmt.Errorf("volume reached state error")))
	inner.EXPECT().VolumeInfo(gomock.Any(), cloneTestName).
		Return(&api.Volume{Name: cloneTestName, UUID: cloneTestVolumeID}, nil)
	inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).
		Return(errors.New("busy"))

	err := runCloneFlexvol(cloneTestVolumeConfig(""), false, client, pending)

	assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
	assert.False(t, drivers.IsVolumeExistsError(err), "got %v", err)
	rec, ok := loadPendingClone(pending, cloneTestName)
	require.True(t, ok)
	assert.True(t, rec.cleanupPending)
	assert.Equal(t, "snapshot-auto", rec.createdSnapName)
}

// An aborted clone found without a record keeps the automatic parent snapshot its config already names, so the
// retry reuses it instead of creating another. A snapshot the request named is not Trident's to reuse.
func TestCloneFlexvol_ExistingCloneInTerminalStateRecordsOnlyAutomaticSnapshot(t *testing.T) {
	tests := map[string]struct {
		externalSnapshot string
		expected         string
	}{
		"automatic snapshot named by the config": {expected: "snapshot-auto"},
		"snapshot named by the request":          {externalSnapshot: "user-snapshot", expected: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			pending := &sync.Map{}
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
			inner.EXPECT().VolumeWaitForStates(gomock.Any(), cloneTestName, gomock.Any(), gomock.Any(), gomock.Any()).
				Return("error", api.TerminalState(fmt.Errorf("volume reached state error")))
			inner.EXPECT().VolumeInfo(gomock.Any(), cloneTestName).
				Return(&api.Volume{Name: cloneTestName, UUID: cloneTestVolumeID}, nil)
			inner.EXPECT().VolumeDestroyByUUID(gomock.Any(), cloneTestVolumeID, cloneTestName, true, true).Return(nil)
			volConfig := cloneTestVolumeConfig("snapshot-auto")
			volConfig.CloneSourceSnapshot = tc.externalSnapshot

			err := runCloneFlexvol(volConfig, false, client, pending)

			assert.True(t, errors.IsVolumeCreatingError(err), "got %v", err)
			rec, ok := loadPendingClone(pending, cloneTestName)
			require.True(t, ok)
			assert.Equal(t, tc.expected, rec.createdSnapName)
		})
	}
}

// A clone that does not report its parent snapshot while the retry begins reports it by the time the split is
// requested, and the name is taken before the split makes it unrecoverable.
func TestCloneFlexvol_ParentSnapshotReportedAfterCloneIsOnline(t *testing.T) {
	client, inner := newCloneJobMock(t)
	client.splitReads = []*api.CloneSplitState{
		{IsFlexclone: true}, // read before the wait: metadata is incomplete
		{IsFlexclone: true, ParentSnapshot: "snapshot-auto"}, // read before the split
	}
	inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
	expectCloneFinishSteps(inner)
	inner.EXPECT().VolumeCloneSplitStart(gomock.Any(), cloneTestName).Return(nil)
	volConfig := cloneTestVolumeConfig("")

	err := runCloneFlexvol(volConfig, true, client, &sync.Map{})

	assert.True(t, drivers.IsVolumeExistsError(err), "got %v", err)
	assert.Equal(t, "snapshot-auto", volConfig.CloneSourceSnapshotInternal)
}

// A retry for a clone whose request named no snapshot gets the automatic parent snapshot back from the clone
// itself when no record holds it, before the split is requested.
func TestCloneFlexvol_ExistingCloneRecoversParentSnapshotFromVolume(t *testing.T) {
	tests := map[string]struct {
		requestSnapshot string
		split           *api.CloneSplitState
		splitErr        error
		expected        string
	}{
		"recovered from the clone": {
			split:    &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-auto"},
			expected: "snapshot-auto",
		},
		"clone already split reports no parent": {
			split:    &api.CloneSplitState{},
			expected: "",
		},
		"clone state cannot be read": {
			splitErr: fmt.Errorf("boom"),
			expected: "",
		},
		"request named a snapshot": {
			requestSnapshot: cloneTestSnap,
			split:           &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-other"},
			expected:        cloneTestSnap,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			client.split, client.splitErr = tc.split, tc.splitErr
			inner.EXPECT().VolumeExists(gomock.Any(), cloneTestName).Return(true, nil)
			expectCloneFinishSteps(inner)
			volConfig := cloneTestVolumeConfig(tc.requestSnapshot)

			// Not splitting keeps the split-state read out of the finish steps, so only the recovery reads it.
			err := runCloneFlexvol(volConfig, false, client, &sync.Map{})

			assert.True(t, drivers.IsVolumeExistsError(err), "got %v", err)
			assert.Equal(t, tc.expected, volConfig.CloneSourceSnapshotInternal)
		})
	}
}

func TestRecoverAutomaticSnapshot(t *testing.T) {
	automatic := func() *storage.VolumeConfig {
		return &storage.VolumeConfig{InternalName: cloneTestName, CloneSourceVolumeInternal: cloneTestSource}
	}
	tests := map[string]struct {
		config   func() *storage.VolumeConfig
		split    *api.CloneSplitState
		plainAPI bool
		expected string
	}{
		"automatic snapshot recovered": {
			config: automatic, split: &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-auto"},
			expected: "snapshot-auto",
		},
		"name already known": {
			config: func() *storage.VolumeConfig {
				c := automatic()
				c.CloneSourceSnapshotInternal = "snapshot-known"
				return c
			},
			split: &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-other"}, expected: "snapshot-known",
		},
		"cloned from a named snapshot": {
			config: func() *storage.VolumeConfig {
				c := automatic()
				c.CloneSourceSnapshot = "user-snapshot"
				return c
			},
			split: &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-other"}, expected: "",
		},
		"not a clone": {
			config: func() *storage.VolumeConfig { return &storage.VolumeConfig{InternalName: cloneTestName} },
			split:  &api.CloneSplitState{IsFlexclone: true, ParentSnapshot: "snapshot-other"}, expected: "",
		},
		"clone already split":            {config: automatic, split: &api.CloneSplitState{}, expected: ""},
		"client cannot read clone state": {config: automatic, plainAPI: true, expected: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, inner := newCloneJobMock(t)
			client.split = tc.split
			var target api.OntapAPI = client
			if tc.plainAPI {
				target = inner
			}
			config := tc.config()

			recoverAutomaticSnapshot(context.Background(), target, config)

			assert.Equal(t, tc.expected, config.CloneSourceSnapshotInternal)
		})
	}
}
