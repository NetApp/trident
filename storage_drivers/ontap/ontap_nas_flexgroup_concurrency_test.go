// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

func newFlexgroupPublishInfo(backendUUID string) *models.VolumePublishInfo {
	return &models.VolumePublishInfo{
		BackendUUID: backendUUID,
		HostName:    "node1",
		Nodes: []*models.Node{
			{Name: "node1", IPs: []string{"1.1.1.1"}},
		},
	}
}

func TestNASFlexGroupTerminateWaitsForExportPolicyReconcile(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.telemetry = nil
	driver.initialized = true

	policyName := getExportPolicyName("backend-uuid")
	reconcileStarted := make(chan struct{})
	releaseReconcile := make(chan struct{})
	terminateStarted := make(chan struct{})

	mockAPI.EXPECT().ExportPolicyCreate(gomock.Any(), policyName).Return(nil)
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), policyName).DoAndReturn(
		func(context.Context, string) (map[int]string, error) {
			close(reconcileStarted)
			<-releaseReconcile
			return map[int]string{}, nil
		},
	)
	mockAPI.EXPECT().ExportPolicyExists(gomock.Any(), policyName).DoAndReturn(
		func(context.Context, string) (bool, error) {
			close(terminateStarted)
			return true, nil
		},
	)
	mockAPI.EXPECT().ExportPolicyDestroy(gomock.Any(), policyName).Return(nil)
	mockAPI.EXPECT().Terminate()

	reconcileDone := make(chan error, 1)
	go func() {
		reconcileDone <- driver.ReconcileNodeAccess(context.Background(), nil, "backend-uuid", "")
	}()

	select {
	case <-reconcileStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for export policy reconciliation to start")
	}

	terminateDone := make(chan struct{})
	go func() {
		driver.Terminate(context.Background(), "backend-uuid")
		close(terminateDone)
	}()

	select {
	case <-terminateStarted:
		t.Fatal("Terminate deleted the export policy while reconciliation held its lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseReconcile)
	require.NoError(t, <-reconcileDone)

	select {
	case <-terminateStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Terminate to acquire the export policy lock")
	}

	select {
	case <-terminateDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Terminate to complete")
	}

	assert.False(t, driver.initialized)
}

func TestNASFlexGroupPublishWaitsForExportPolicyReconcile(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}

	policyName := getExportPolicyName("backend-uuid")
	reconcileStarted := make(chan struct{})
	releaseReconcile := make(chan struct{})
	publishPolicyCheckStarted := make(chan struct{})

	mockAPI.EXPECT().ExportPolicyCreate(gomock.Any(), policyName).Return(nil)
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), policyName).DoAndReturn(
		func(context.Context, string) (map[int]string, error) {
			close(reconcileStarted)
			<-releaseReconcile
			return map[int]string{}, nil
		},
	)
	mockAPI.EXPECT().ExportPolicyExists(gomock.Any(), policyName).DoAndReturn(
		func(context.Context, string) (bool, error) {
			close(publishPolicyCheckStarted)
			return true, nil
		},
	)
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), policyName).Return(map[int]string{}, nil)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), policyName, "1.1.1.1", gomock.Any()).Return(nil)
	mockAPI.EXPECT().FlexgroupModifyExportPolicy(gomock.Any(), "flexgroup", policyName).Return(nil)

	reconcileDone := make(chan error, 1)
	go func() {
		reconcileDone <- driver.ReconcileNodeAccess(context.Background(), nil, "backend-uuid", "")
	}()

	select {
	case <-reconcileStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for export policy reconciliation to start")
	}

	publishDone := make(chan error, 1)
	go func() {
		publishDone <- driver.Publish(
			context.Background(),
			&storage.VolumeConfig{InternalName: "flexgroup"},
			newFlexgroupPublishInfo("backend-uuid"),
		)
	}()

	select {
	case <-publishPolicyCheckStarted:
		t.Fatal("Publish checked the export policy while reconciliation held its lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseReconcile)
	require.NoError(t, <-reconcileDone)

	select {
	case <-publishPolicyCheckStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Publish to acquire the export policy lock")
	}

	require.NoError(t, <-publishDone)
}

func TestNASFlexGroupConcurrentPublishSameBackendPolicy(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}

	policyName := getExportPolicyName("backend-uuid")
	volumes := []string{"flexgroup-a", "flexgroup-b"}

	var current, maxConcurrent atomic.Int32
	mockAPI.EXPECT().ExportPolicyExists(gomock.Any(), policyName).Return(true, nil).Times(len(volumes))
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), policyName).DoAndReturn(
		func(context.Context, string) (map[int]string, error) {
			cur := current.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			current.Add(-1)
			return map[int]string{}, nil
		},
	).Times(len(volumes))
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), policyName, "1.1.1.1", gomock.Any()).Return(nil).AnyTimes()
	for _, volume := range volumes {
		mockAPI.EXPECT().FlexgroupModifyExportPolicy(gomock.Any(), volume, policyName).Return(nil)
	}

	var wg sync.WaitGroup
	errs := make([]error, len(volumes))
	for i, volume := range volumes {
		wg.Add(1)
		go func(i int, volume string) {
			defer wg.Done()
			errs[i] = driver.Publish(
				context.Background(),
				&storage.VolumeConfig{InternalName: volume},
				newFlexgroupPublishInfo("backend-uuid"),
			)
		}(i, volume)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "publish %d failed", i)
	}
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"publishes against the same backend policy should serialize")
}
