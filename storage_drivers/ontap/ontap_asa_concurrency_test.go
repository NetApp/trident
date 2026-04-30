// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils/models"
)

// ============================================================================
// ASA SAN: Concurrent Unpublish serialization on same igroup
// Verifies that igroupMutex serializes Unpublish calls sharing an igroup
// ============================================================================

func TestASASAN_ConcurrentUnpublish_SameIgroup(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapASADriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.ISCSI

	defer func(prev tridentconfig.DriverContext) {
		tridentconfig.CurrentDriverContext = prev
	}(tridentconfig.CurrentDriverContext)
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	const numGoroutines = 5
	igroupName := "testHost-testUUID"

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	// LunMapInfo returns valid LUN ID for every call
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, gomock.Any()).Return(1, nil).AnyTimes()

	// LunUnmap tracks concurrency on the shared igroup
	mockAPI.EXPECT().LunUnmap(ctx, igroupName, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ string) error {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			return nil
		},
	).AnyTimes()

	// After unmap, each goroutine checks if igroup has remaining LUNs. Return non-empty
	// to avoid concurrent igroup destruction contention in this test.
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(
		[]string{"some-remaining-lun"}, nil,
	).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(volID int) {
			defer wg.Done()
			volConfig := &storage.VolumeConfig{
				InternalName: fmt.Sprintf("vol%d", volID),
			}
			volConfig.AccessInfo.IscsiIgroup = igroupName
			publishInfo := &models.VolumePublishInfo{
				HostName:    "testHost",
				TridentUUID: "testUUID",
			}
			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), successCount.Load(),
		"all Unpublish calls should succeed")
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"igroup operations should be serialized (max concurrent should be 1)")
}

// ============================================================================
// ASA SAN: Concurrent Unpublish on different igroups can run in parallel
// ============================================================================

func TestASASAN_ConcurrentUnpublish_DifferentIgroups(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapASADriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.ISCSI

	defer func(prev tridentconfig.DriverContext) {
		tridentconfig.CurrentDriverContext = prev
	}(tridentconfig.CurrentDriverContext)
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	const numGoroutines = 5
	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(1, nil).AnyTimes()
	mockAPI.EXPECT().LunUnmap(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ string) error {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			return nil
		},
	).AnyTimes()

	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, gomock.Any()).Return(
		[]string{"remaining-lun"}, nil,
	).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(volID int) {
			defer wg.Done()
			igroupName := fmt.Sprintf("host%d-uuid%d", volID, volID)
			volConfig := &storage.VolumeConfig{
				InternalName: fmt.Sprintf("vol%d", volID),
			}
			volConfig.AccessInfo.IscsiIgroup = igroupName
			publishInfo := &models.VolumePublishInfo{
				HostName:    fmt.Sprintf("host%d", volID),
				TridentUUID: fmt.Sprintf("uuid%d", volID),
			}
			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), successCount.Load(),
		"all Unpublish calls should succeed")
	assert.Greater(t, maxConcurrent.Load(), int32(1),
		"different-igroup operations should run in parallel (max concurrent: %d)", maxConcurrent.Load())
}

// ============================================================================
// ASA NVMe: Concurrent Unpublish serialization on same subsystem
// Verifies that lockNamespaceAndSubsystem serializes Unpublish calls sharing
// a subsystem UUID
// ============================================================================

func TestASANVMe_ConcurrentUnpublish_SameSubsystem(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().Terminate().AnyTimes()

	driver := newTestOntapASANVMeDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.NVMe

	const numGoroutines = 5
	subsystemUUID := "shared-subsystem-uuid"

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	// RemoveHostFromSubsystem path: NVMeGetHostsOfSubsystem returns empty (no hosts, nothing to remove)
	mockAPI.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).DoAndReturn(
		func(_ context.Context, _ string) ([]*api.NvmeSubsystemHost, error) {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			return []*api.NvmeSubsystemHost{}, nil
		},
	).AnyTimes()

	// UnmapNamespaceFromSubsystem path: namespace is not mapped (nothing to unmap)
	mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, gomock.Any()).Return(false, nil).AnyTimes()

	// deleteSubsystemIfEmpty path: subsystem still has namespaces
	mockAPI.EXPECT().NVMeSubsystemGetNamespaceCount(ctx, subsystemUUID).Return(int64(1), nil).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(nsID int) {
			defer wg.Done()
			nsUUID := fmt.Sprintf("namespace-uuid-%d", nsID)
			volConfig := &storage.VolumeConfig{
				InternalName: fmt.Sprintf("vol%d", nsID),
			}
			volConfig.AccessInfo.NVMeSubsystemUUID = subsystemUUID
			volConfig.AccessInfo.NVMeNamespaceUUID = nsUUID
			publishInfo := &models.VolumePublishInfo{
				HostNQN: fmt.Sprintf("nqn.host-%d", nsID),
			}
			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), successCount.Load(),
		"all Unpublish calls should succeed")
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"same-subsystem operations should be serialized (max concurrent should be 1)")
}

// ============================================================================
// ASA NVMe: Concurrent Unpublish on different subsystems can run in parallel
// ============================================================================

func TestASANVMe_ConcurrentUnpublish_DifferentSubsystems(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().Terminate().AnyTimes()

	driver := newTestOntapASANVMeDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.NVMe

	const numGoroutines = 5
	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	mockAPI.EXPECT().NVMeGetHostsOfSubsystem(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string) ([]*api.NvmeSubsystemHost, error) {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			return []*api.NvmeSubsystemHost{}, nil
		},
	).AnyTimes()

	mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	mockAPI.EXPECT().NVMeSubsystemGetNamespaceCount(ctx, gomock.Any()).Return(int64(1), nil).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			volConfig := &storage.VolumeConfig{
				InternalName: fmt.Sprintf("vol%d", id),
			}
			volConfig.AccessInfo.NVMeSubsystemUUID = fmt.Sprintf("subsystem-uuid-%d", id)
			volConfig.AccessInfo.NVMeNamespaceUUID = fmt.Sprintf("namespace-uuid-%d", id)
			publishInfo := &models.VolumePublishInfo{
				HostNQN: fmt.Sprintf("nqn.host-%d", id),
			}
			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), successCount.Load(),
		"all Unpublish calls should succeed")
	assert.Greater(t, maxConcurrent.Load(), int32(1),
		"different-subsystem operations should run in parallel (max concurrent: %d)", maxConcurrent.Load())
}

// ============================================================================
// ASA NVMe: Concurrent Publish serialization on same subsystem
// Verifies that lockNamespaceAndSubsystem serializes Publish calls sharing
// a subsystem
// ============================================================================

func TestASANVMe_ConcurrentPublish_SameSubsystem(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().Terminate().AnyTimes()

	driver := newTestOntapASANVMeDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.NVMe

	defer func(prev tridentconfig.DriverContext) {
		tridentconfig.CurrentDriverContext = prev
	}(tridentconfig.CurrentDriverContext)
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	const numGoroutines = 5
	subsystemUUID := "shared-subsystem-uuid"
	subsystemNQN := "nqn.shared-subsystem"

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	// isFlexvolRW check
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(
		&api.Volume{AccessType: "rw"}, nil,
	).AnyTimes()

	// NVMeSubsystemCreate returns the shared subsystem
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, gomock.Any(), gomock.Any()).Return(
		&api.NVMeSubsystem{UUID: subsystemUUID, NQN: subsystemNQN}, nil,
	).AnyTimes()

	// NVMeAddHostToSubsystem tracks concurrency on the shared subsystem
	mockAPI.EXPECT().NVMeAddHostToSubsystem(ctx, gomock.Any(), subsystemUUID).DoAndReturn(
		func(_ context.Context, _ string, _ string) error {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			return nil
		},
	).AnyTimes()

	// NVMeEnsureNamespaceMapped succeeds
	mockAPI.EXPECT().NVMeEnsureNamespaceMapped(ctx, subsystemUUID, gomock.Any()).Return(nil).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nsUUID := fmt.Sprintf("namespace-uuid-%d", id)
			volConfig := &storage.VolumeConfig{
				InternalName: fmt.Sprintf("vol%d", id),
				Name:         fmt.Sprintf("pvc-%d", id),
				FileSystem:   "xfs",
			}
			volConfig.AccessInfo.NVMeNamespaceUUID = nsUUID
			publishInfo := &models.VolumePublishInfo{
				HostName:    "testHost",
				TridentUUID: "testUUID",
				HostNQN:     fmt.Sprintf("nqn.host-%d", id),
			}
			err := driver.Publish(ctx, volConfig, publishInfo)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), successCount.Load(),
		"all Publish calls should succeed")
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"same-subsystem Publish calls should be serialized (max concurrent should be 1)")
}

// ============================================================================
// ASA SAN: Concurrent Import + Unpublish serialization on same LUN
// Verifies that lunMutex in Import serializes with lunMutex in Unpublish
// ============================================================================

func TestASASAN_ConcurrentImportAndUnpublish_SameLUN(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapASADriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.ISCSI

	defer func(prev tridentconfig.DriverContext) {
		tridentconfig.CurrentDriverContext = prev
	}(tridentconfig.CurrentDriverContext)
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	lunName := "testLUN"
	igroupName := "testHost-testUUID"

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	trackConcurrency := func() {
		cur := currentConcurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		currentConcurrent.Add(-1)
	}

	// Import path mocks
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(
		&api.Lun{Name: lunName, State: "online", Size: "1073741824"}, nil,
	).AnyTimes()
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(
		&api.Volume{AccessType: "rw"}, nil,
	).AnyTimes()
	mockAPI.EXPECT().LunRename(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAPI.EXPECT().LunSetComment(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return(
		[]string{"old-igroup"}, nil,
	).AnyTimes()
	mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(1, nil).AnyTimes()

	// LunUnmap is invoked from inside both critical sections:
	//   - Import path: LunUnmapAllIgroups -> LunUnmap per igroup (under lunMutex)
	//   - Unpublish path: LunUnmapIgroup -> LunUnmap (under lunMutex)
	// Tracking concurrency here is the only way to detect cross-operation
	// interleaving between Import and Unpublish on the same LUN.
	mockAPI.EXPECT().LunUnmap(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string) error {
			trackConcurrency()
			return nil
		},
	).AnyTimes()

	// Unpublish path mocks
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(
		[]string{"remaining-lun"}, nil,
	).AnyTimes()

	var wg sync.WaitGroup
	var importSuccess atomic.Int32
	var unpublishSuccess atomic.Int32

	const iterations = 3

	for i := 0; i < iterations; i++ {
		// Import goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			volConfig := getASAVolumeConfig()
			volConfig.InternalName = lunName
			err := driver.Import(ctx, &volConfig, "originalLUN")
			if err == nil {
				importSuccess.Add(1)
			}
		}()

		// Unpublish goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			volConfig := &storage.VolumeConfig{
				InternalName: lunName,
			}
			volConfig.AccessInfo.IscsiIgroup = igroupName
			publishInfo := &models.VolumePublishInfo{
				HostName:    "testHost",
				TridentUUID: "testUUID",
			}
			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if err == nil {
				unpublishSuccess.Add(1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(iterations), importSuccess.Load(), "all Import calls should succeed")
	assert.Equal(t, int32(iterations), unpublishSuccess.Load(), "all Unpublish calls should succeed")
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"Import and Unpublish on same LUN should be serialized via lunMutex")
}

// ============================================================================
// ASA NVMe: Concurrent Import serialization via namespaceMutex
// Verifies that Import acquires namespaceMutex
// ============================================================================

func TestASANVMe_ConcurrentImport_SameNamespace(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().Terminate().AnyTimes()

	driver := newTestOntapASANVMeDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.NVMe

	nsUUID := "shared-ns-uuid"
	const numGoroutines = 5

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	// Namespace lookup
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(
		&api.NVMeNamespace{
			Name:  "original-ns",
			UUID:  nsUUID,
			State: "online",
			Size:  "1073741824",
		}, nil,
	).AnyTimes()

	// VolumeInfo
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(
		&api.Volume{AccessType: "rw"}, nil,
	).AnyTimes()

	// NVMeIsNamespaceMapped: track concurrency within the locked section
	mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUUID).DoAndReturn(
		func(_ context.Context, _ string, _ string) (bool, error) {
			cur := currentConcurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			currentConcurrent.Add(-1)
			return false, nil
		},
	).AnyTimes()

	// Rename + SetComment (Import may generate a new comment from volConfig fields)
	mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUUID, gomock.Any()).Return(nil).AnyTimes()
	mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			volConfig := getASANVMeVolumeConfig()
			volConfig.InternalName = fmt.Sprintf("trident-pvc-%d", id)
			err := driver.Import(ctx, &volConfig, "original-ns")
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(numGoroutines), successCount.Load(),
		"all Import calls should succeed")
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"same-namespace Import calls should be serialized via namespaceMutex")
}
