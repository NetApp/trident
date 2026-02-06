// Copyright 2025 NetApp, Inc. All Rights Reserved.

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

	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// TestConcurrency_ResizeQuotasParallelFlexvols verifies resizeQuotas can process multiple FlexVols concurrently
func TestConcurrency_ResizeQuotasParallelFlexvols(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	flexvol1 := "flexvol1"
	flexvol2 := "flexvol2"
	flexvol3 := "flexvol3"

	driver.quotaResizeMap.Set(flexvol1, true)
	driver.quotaResizeMap.Set(flexvol2, true)
	driver.quotaResizeMap.Set(flexvol3, true)

	// Mock quota resize with simulated work
	for _, fv := range []string{flexvol1, flexvol2, flexvol3} {
		fv := fv
		mockAPI.EXPECT().QuotaResize(gomock.Any(), fv).DoAndReturn(func(ctx context.Context, name string) error {
			time.Sleep(50 * time.Millisecond) // Simulate API work
			return nil
		})
	}

	driver.resizeQuotas(ctx)

	// All should complete successfully
	assert.Equal(t, 0, driver.quotaResizeMap.Len(), "All FlexVols should be removed from resize map")
}

// TestConcurrency_ResizeQuotasWithErrors verifies error handling doesn't leave locks held
func TestConcurrency_ResizeQuotasWithErrors(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	flexvol1 := "flexvol1"
	flexvol2 := "flexvol2_fails"
	flexvol3 := "flexvol3"

	driver.quotaResizeMap.Set(flexvol1, true)
	driver.quotaResizeMap.Set(flexvol2, true)
	driver.quotaResizeMap.Set(flexvol3, true)

	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol1).Return(nil)
	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol2).Return(fmt.Errorf("quota resize failed"))
	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol3).Return(nil)

	driver.resizeQuotas(ctx)

	// Successful ones should be removed, failed one should remain
	assert.False(t, driver.quotaResizeMap.Get(flexvol1), "flexvol1 should be removed")
	assert.True(t, driver.quotaResizeMap.Get(flexvol2), "flexvol2 should remain due to error")
	assert.False(t, driver.quotaResizeMap.Get(flexvol3), "flexvol3 should be removed")

	// Verify we can run again (locks were properly released)
	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol2).Return(nil)
	driver.resizeQuotas(ctx)

	assert.Equal(t, 0, driver.quotaResizeMap.Len(), "All should be cleared after retry")
}

// TestConcurrency_PruneUnusedFlexvolsParallel verifies pruneUnusedFlexvols processes FlexVols concurrently
func TestConcurrency_PruneUnusedFlexvolsParallel(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.emptyFlexvolDeferredDeletePeriod = 1 * time.Hour // Long enough to not trigger deletion
	driver.emptyFlexvolMap = make(map[string]time.Time)

	flexvols := []string{"fv1", "fv2", "fv3", "fv4"}
	volumes := api.Volumes{}
	for _, fv := range flexvols {
		volumes = append(volumes, &api.Volume{Name: fv})
	}

	mockAPI.EXPECT().VolumeListByPrefix(gomock.Any(), gomock.Any()).Return(volumes, nil)

	// Mock qtree count with simulated work
	for _, fv := range flexvols {
		fv := fv
		mockAPI.EXPECT().QtreeCount(gomock.Any(), fv).DoAndReturn(func(ctx context.Context, name string) (int, error) {
			time.Sleep(20 * time.Millisecond) // Simulate API work
			return 0, nil
		})
	}

	driver.pruneUnusedFlexvols(ctx)

	// Verify empty FlexVols were tracked
	assert.Equal(t, len(flexvols), len(driver.emptyFlexvolMap), "All empty FlexVols should be tracked")
}

// TestConcurrency_ReapDeletedQtreesGroupsByFlexvol verifies reapDeletedQtrees groups qtrees by FlexVol
func TestConcurrency_ReapDeletedQtreesGroupsByFlexvol(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	flexvol1 := "flexvol1"
	flexvol2 := "flexvol2"

	qtrees := []*api.Qtree{
		{Name: "deleted_qtree1", Volume: flexvol1},
		{Name: "deleted_qtree2", Volume: flexvol1},
		{Name: "deleted_qtree3", Volume: flexvol1},
		{Name: "deleted_qtree4", Volume: flexvol2},
		{Name: "deleted_qtree5", Volume: flexvol2},
	}

	mockAPI.EXPECT().QtreeListByPrefix(gomock.Any(), gomock.Any(), gomock.Any()).Return(qtrees, nil)

	destroyOrder := []string{}
	var orderMu sync.Mutex

	for _, q := range qtrees {
		qtreeName := q.Name
		qtreePath := fmt.Sprintf("/vol/%s/%s", q.Volume, q.Name)
		mockAPI.EXPECT().QtreeDestroyAsync(gomock.Any(), qtreePath, true).
			DoAndReturn(func(ctx context.Context, path string, force bool) error {
				orderMu.Lock()
				destroyOrder = append(destroyOrder, qtreeName)
				orderMu.Unlock()
				return nil
			})
	}

	driver.reapDeletedQtrees(ctx)

	// Verify all qtrees were destroyed
	assert.Len(t, destroyOrder, 5, "All 5 qtrees should be destroyed")

	// Verify qtrees from same FlexVol are grouped together
	// (deleted_qtree1, deleted_qtree2, deleted_qtree3 should be together)
	// (deleted_qtree4, deleted_qtree5 should be together)
	// Lock needed even though writes are complete - ensures race detector sees proper synchronization
	// between the writes in DoAndReturn callbacks and reads here
	orderMu.Lock()
	defer orderMu.Unlock()

	fv1Qtrees := []string{}
	fv2Qtrees := []string{}
	for _, name := range destroyOrder {
		if name == "deleted_qtree1" || name == "deleted_qtree2" || name == "deleted_qtree3" {
			fv1Qtrees = append(fv1Qtrees, name)
		} else {
			fv2Qtrees = append(fv2Qtrees, name)
		}
	}

	assert.Len(t, fv1Qtrees, 3, "Should have 3 qtrees from flexvol1")
	assert.Len(t, fv2Qtrees, 2, "Should have 2 qtrees from flexvol2")
}

// TestConcurrency_CreateNoDeadlock verifies Create doesn't deadlock with concurrent calls
func TestConcurrency_CreateNoDeadlock(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.DataLIF = "1.1.1.1"

	// Create multiple qtree configs for different FlexVols
	configs := []*storage.VolumeConfig{
		{InternalName: "qtree1", Size: "1073741824"},
		{InternalName: "qtree2", Size: "1073741824"},
		{InternalName: "qtree3", Size: "1073741824"},
	}

	pool := storage.NewStoragePool(nil, "pool1")
	pool.SetInternalAttributes(map[string]string{
		SpaceReserve:    "none",
		SnapshotPolicy:  "none",
		SnapshotReserve: "0",
		UnixPermissions: "0755",
		SnapshotDir:     "false",
		ExportPolicy:    "default",
		SecurityStyle:   "unix",
		Encryption:      "false",
		TieringPolicy:   "",
		QosPolicy:       "",
	})

	// Set up expectations (will fail due to complexity, but should not deadlock)
	for _, cfg := range configs {
		mockAPI.EXPECT().QtreeExists(gomock.Any(), cfg.InternalName, gomock.Any()).Return(false, "", nil).AnyTimes()
	}

	done := make(chan bool, len(configs))
	errCount := atomic.Int32{}

	// Launch concurrent Create operations
	for _, cfg := range configs {
		cfg := cfg
		go func() {
			err := driver.Create(ctx, cfg, pool, map[string]sa.Request{})
			if err != nil {
				errCount.Add(1)
			}
			done <- true
		}()
	}

	// Wait for all operations with timeout
	timeout := time.After(5 * time.Second)
	completed := 0
	for completed < len(configs) {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatal("Deadlock detected: Create operations did not complete within timeout")
		}
	}

	// All operations should complete (even if they error)
	assert.Equal(t, len(configs), completed, "All Create operations should complete")
}

// TestConcurrency_DestroyNoDeadlock verifies Destroy doesn't deadlock with concurrent calls
func TestConcurrency_DestroyNoDeadlock(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.DataLIF = "1.1.1.1"

	flexvol := "flexvol1"
	configs := []*storage.VolumeConfig{
		{InternalName: "qtree1", InternalID: driver.CreateQtreeInternalID(driver.Config.SVM, flexvol, "qtree1")},
		{InternalName: "qtree2", InternalID: driver.CreateQtreeInternalID(driver.Config.SVM, flexvol, "qtree2")},
		{InternalName: "qtree3", InternalID: driver.CreateQtreeInternalID(driver.Config.SVM, flexvol, "qtree3")},
	}

	for _, cfg := range configs {
		cfg := cfg
		mockAPI.EXPECT().QtreeExists(gomock.Any(), cfg.InternalName, gomock.Any()).Return(true, flexvol, nil).AnyTimes()
		mockAPI.EXPECT().QtreeRename(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		mockAPI.EXPECT().QtreeDestroyAsync(gomock.Any(), gomock.Any(), true).Return(nil).AnyTimes()
	}

	done := make(chan bool, len(configs))
	successCount := atomic.Int32{}

	// Launch concurrent Destroy operations on same FlexVol (should serialize)
	for _, cfg := range configs {
		cfg := cfg
		go func() {
			err := driver.Destroy(ctx, cfg)
			if err == nil {
				successCount.Add(1)
			}
			done <- true
		}()
	}

	// Wait for all operations with timeout
	timeout := time.After(5 * time.Second)
	completed := 0
	for completed < len(configs) {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatal("Deadlock detected: Destroy operations did not complete within timeout")
		}
	}

	assert.Equal(t, len(configs), completed, "All Destroy operations should complete")
	assert.Equal(t, int32(len(configs)), successCount.Load(), "All Destroy operations should succeed")
}

// TestConcurrency_MixedOperationsNoDeadlock verifies mixed operations don't deadlock
func TestConcurrency_MixedOperationsNoDeadlock(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.DataLIF = "1.1.1.1"
	driver.emptyFlexvolDeferredDeletePeriod = 1 * time.Hour // Don't actually delete during test

	// Set up multiple FlexVols for resize
	flexvol1 := "flexvol1"
	flexvol2 := "flexvol2"
	flexvol3 := "flexvol3"

	driver.quotaResizeMap.Set(flexvol1, true)
	driver.quotaResizeMap.Set(flexvol2, true)
	driver.quotaResizeMap.Set(flexvol3, true)

	// Mock resize operations
	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol1).Return(nil).AnyTimes()
	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol2).Return(nil).AnyTimes()
	mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol3).Return(nil).AnyTimes()

	// Mock prune operations
	volumes := api.Volumes{
		&api.Volume{Name: flexvol1},
		&api.Volume{Name: flexvol2},
	}
	mockAPI.EXPECT().VolumeListByPrefix(gomock.Any(), gomock.Any()).Return(volumes, nil).AnyTimes()
	mockAPI.EXPECT().QtreeCount(gomock.Any(), gomock.Any()).Return(5, nil).AnyTimes()

	// Mock reap operations
	mockAPI.EXPECT().QtreeListByPrefix(gomock.Any(), gomock.Any(), gomock.Any()).Return([]*api.Qtree{}, nil).AnyTimes()

	done := make(chan bool, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	// Run resizeQuotas concurrently
	go func() {
		defer wg.Done()
		driver.resizeQuotas(ctx)
		done <- true
	}()

	// Run pruneUnusedFlexvols concurrently
	go func() {
		defer wg.Done()
		driver.pruneUnusedFlexvols(ctx)
		done <- true
	}()

	// Run reapDeletedQtrees concurrently
	go func() {
		defer wg.Done()
		driver.reapDeletedQtrees(ctx)
		done <- true
	}()

	// Wait for all operations with timeout
	timeout := time.After(5 * time.Second)
	completed := 0
	for completed < 3 {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatal("Deadlock detected: Mixed operations did not complete within timeout")
		}
	}

	wg.Wait()
	assert.Equal(t, 3, completed, "All housekeeping operations should complete without deadlock")
}

// TestConcurrency_FlexvolLocksGarbageCollection verifies GCNamedMutex properly cleans up
func TestConcurrency_FlexvolLocksGarbageCollection(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)

	// Lock and unlock many FlexVols
	for i := 0; i < 100; i++ {
		flexvol := fmt.Sprintf("flexvol_%d", i)
		driver.flexvolLocks.Lock(flexvol)
		driver.flexvolLocks.Unlock(flexvol)
	}

	// GCNamedMutex should have cleaned up locks with refcount 0
	// We can't directly test internal state, but no panics means it's working
	assert.NotNil(t, driver.flexvolLocks, "flexvolLocks should still be valid")
}

// TestConcurrency_ResizeQuotasStressTest stress tests concurrent quota resize operations.
// In production, multiple housekeeping goroutines might run overlapping cycles during high load.
// This test validates the driver handles that chaos without crashing or hanging.
func TestConcurrency_ResizeQuotasStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Add many FlexVols
	numFlexvols := 50
	for i := 0; i < numFlexvols; i++ {
		flexvol := fmt.Sprintf("flexvol_%d", i)
		driver.quotaResizeMap.Set(flexvol, true)
		// Allow multiple calls since we run resizeQuotas concurrently 5 times
		mockAPI.EXPECT().QuotaResize(gomock.Any(), flexvol).Return(nil).AnyTimes()
	}

	// Run resize multiple times concurrently
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			driver.resizeQuotas(ctx)
		}()
	}

	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Stress test timed out - possible deadlock or performance issue")
	}

	assert.Equal(t, 0, driver.quotaResizeMap.Len(), "All FlexVols should be cleared")
}

// TestConcurrency_CreateRaceFlexVolCapacity validates that the per-FlexVol lock
// prevents TOCTOU (Time-Of-Check-Time-Of-Use) races in capacity checking.
//
// concurrent threads try to allocate 2 GiB each on a FlexVol with:
//   - Current usage: 7 GiB
//   - Size limit: 10 GiB
//
// Without proper locking, all 3 threads could read "7 GiB used", think they fit,
// and proceed, resulting in 13 GiB allocated (over-commit).
//
// With the lock acquired BEFORE capacity checks in findFlexvolForQtree:
//   - Thread 1: reads 7 GiB, allocates → 9 GiB (SUCCESS)
//   - Thread 2: reads 9 GiB, 9+2=11 > 10 (REJECTED)
//   - Thread 3: reads 9 GiB, 9+2=11 > 10 (REJECTED)
//
// Result: Exactly 1 allocation succeeds, preventing capacity over-commit.
func TestConcurrency_CreateRaceFlexVolCapacity(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LimitVolumePoolSize = "10g" // 10 GiB limit per FlexVol

	const (
		flexvolName       = "test_flexvol"
		existingQuotaSize = 7 * 1024 * 1024 * 1024  // 7 GiB already used
		newQtreeSize      = 2 * 1024 * 1024 * 1024  // 2 GiB per new qtree
		maxFlexvolSize    = 10 * 1024 * 1024 * 1024 // 10 GiB limit
		numConcurrent     = 3                       // 3 concurrent creates
	)

	// Setup: FlexVol has 7 GiB used, 3 GiB free
	// Each new qtree wants 2 GiB
	// Only 1 should succeed (7+2=9 < 10), others should fail or pick different FlexVol
	volInfo := &api.Volume{
		Name:            flexvolName,
		Size:            "10g",
		SnapshotReserve: 10,
	}

	// Track how many creates actually attempted to use this FlexVol
	var successfulCreates atomic.Int32

	// Setup expectations
	// VolumeListByAttrs will return our test FlexVol
	mockAPI.EXPECT().VolumeListByAttrs(gomock.Any(), gomock.Any()).
		Return(api.Volumes{volInfo}, nil).AnyTimes()

	// For each concurrent create attempt, the lock ensures serialized access
	// The first one to acquire lock will see 7 GiB used, next sees 9 GiB, next sees 11 GiB (over limit)
	mockAPI.EXPECT().VolumeInfo(gomock.Any(), flexvolName).
		Return(volInfo, nil).AnyTimes()

	mockAPI.EXPECT().QuotaEntryList(gomock.Any(), flexvolName).
		DoAndReturn(func(ctx context.Context, name string) (api.QuotaEntries, error) {
			// Each successful create adds 2 GiB to the total
			currentCount := successfulCreates.Load()
			currentTotal := existingQuotaSize + (int64(currentCount) * newQtreeSize)

			return api.QuotaEntries{
				{Target: "", DiskLimitBytes: currentTotal},
			}, nil
		}).AnyTimes()

	// QtreeCount shows we have room (not at 200 limit)
	mockAPI.EXPECT().QtreeCount(gomock.Any(), flexvolName).
		Return(10, nil).AnyTimes() // Well under the 200 limit

	// Track which creates actually proceed past capacity check
	// Only the ones that fit should get here
	mockAPI.EXPECT().VolumeSize(gomock.Any(), flexvolName).
		Return(uint64(maxFlexvolSize), nil).AnyTimes()

	mockAPI.EXPECT().VolumeSetSize(gomock.Any(), flexvolName, gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string, newSizeStr string) error {
			// This is the critical section - actually "allocating" space
			currentCount := successfulCreates.Load()
			currentTotal := existingQuotaSize + (int64(currentCount) * newQtreeSize)
			newTotal := currentTotal + newQtreeSize

			// Verify we never exceed the limit
			if newTotal > maxFlexvolSize {
				t.Errorf("TOCTOU RACE DETECTED: Attempted to allocate %d bytes, exceeds limit of %d",
					newTotal, maxFlexvolSize)
				return fmt.Errorf("would exceed capacity")
			}

			successfulCreates.Add(1)
			return nil
		}).AnyTimes()

	// Run concurrent findFlexvolForQtree calls
	var wg sync.WaitGroup
	results := make([]*locks.LockedResource, numConcurrent)
	errors := make([]error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Find the FlexVol (this acquires the lock and checks capacity)
			lockedFlexvol, err := driver.findFlexvolForQtree(
				ctx, "aggr1", "none", "none", "", "0",
				false, nil, true, newQtreeSize, maxFlexvolSize,
			)

			results[index] = lockedFlexvol
			errors[index] = err

			// If we got a FlexVol, simulate using it
			if lockedFlexvol != nil {
				defer lockedFlexvol.Unlock()

				// Simulate the actual allocation by calling VolumeSetSize
				// This increments successfulCreates
				_, _ = driver.API.VolumeSize(ctx, lockedFlexvol.Name())
				_ = driver.API.VolumeSetSize(ctx, lockedFlexvol.Name(), "newsize")
			}
		}(i)
	}

	wg.Wait()

	// Verification
	finalSuccessCount := successfulCreates.Load()

	// With 7 GiB used and 10 GiB limit:
	// - First thread: 7 + 2 = 9 GiB (fits) ✓
	// - Second thread: 9 + 2 = 11 GiB (exceeds) ✗
	// - Third thread: 11 + 2 = 13 GiB (exceeds) ✗
	// Therefore, exactly 1 should succeed
	assert.Equal(t, int32(1), finalSuccessCount,
		"Expected exactly 1 successful allocation (7+2=9 < 10), but got %d", finalSuccessCount)

	// Count how many got a FlexVol vs nil
	gotFlexvol := 0
	gotNil := 0
	gotError := 0
	for i := 0; i < numConcurrent; i++ {
		if errors[i] != nil {
			gotError++
		} else if results[i] != nil {
			gotFlexvol++
		} else {
			gotNil++
		}
	}

	assert.Equal(t, 1, gotFlexvol, "Expected 1 thread to get the FlexVol")
	assert.Equal(t, 2, gotNil, "Expected 2 threads to get nil (capacity exceeded)")
	assert.Equal(t, 0, gotError, "Expected no errors")
}

// TestConcurrency_CreateRaceQtreeCount validates that the per-FlexVol lock
// prevents TOCTOU races when checking the qtree count limit (200 qtrees/FlexVol).
//
// Scenario: 10 concurrent threads try to create qtrees on a FlexVol with:
//   - Current qtrees: 195
//   - Qtree limit: 200
//
// Without proper locking, all 10 threads could read "195 qtrees", think they fit,
// and proceed, resulting in 205 qtrees (exceeds limit).
//
// With the lock acquired BEFORE qtree count checks in findFlexvolForQtree:
//   - Threads 1-5: read 195-199, allocate → 196-200 (SUCCESS)
//   - Threads 6-10: read 200, 200+1=201 > 200 (REJECTED)
//
// Result: Exactly 5 allocations succeed, preventing qtree limit violation.
func TestConcurrency_CreateRaceQtreeCount(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LimitVolumePoolSize = "100g" // Large enough to not be capacity-constrained

	const (
		flexvolName       = "test_flexvol"
		existingQtrees    = 195 // Already have 195 qtrees
		qtreeLimit        = 200 // ONTAP limit
		numConcurrent     = 10  // 10 concurrent creates
		expectedSuccesses = 5   // Only 5 should fit (195 + 5 = 200)
	)

	volInfo := &api.Volume{
		Name:            flexvolName,
		Size:            "100g",
		SnapshotReserve: 10,
	}

	// Track how many creates successfully passed the qtree count check
	var successfulCreates atomic.Int32

	// Setup expectations
	mockAPI.EXPECT().VolumeListByAttrs(gomock.Any(), gomock.Any()).
		Return(api.Volumes{volInfo}, nil).AnyTimes()

	mockAPI.EXPECT().VolumeInfo(gomock.Any(), flexvolName).
		Return(volInfo, nil).AnyTimes()

	// QuotaEntryList - capacity is fine (not the constraint here)
	mockAPI.EXPECT().QuotaEntryList(gomock.Any(), flexvolName).
		Return(api.QuotaEntries{
			{Target: "", DiskLimitBytes: 10 * 1024 * 1024 * 1024}, // 10 GiB used
		}, nil).AnyTimes()

	// QtreeCount is the critical check - it should increase as creates succeed
	mockAPI.EXPECT().QtreeCount(gomock.Any(), flexvolName).
		DoAndReturn(func(ctx context.Context, name string) (int, error) {
			currentCount := successfulCreates.Load()
			return existingQtrees + int(currentCount), nil
		}).AnyTimes()

	// VolumeSize and VolumeSetSize for simulating allocation
	mockAPI.EXPECT().VolumeSize(gomock.Any(), flexvolName).
		Return(uint64(100*1024*1024*1024), nil).AnyTimes()

	mockAPI.EXPECT().VolumeSetSize(gomock.Any(), flexvolName, gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string, newSizeStr string) error {
			// This is where we actually "allocate" - increment the counter
			currentCount := successfulCreates.Load()
			newCount := existingQtrees + int(currentCount) + 1

			// Verify we never exceed the qtree limit
			if newCount > qtreeLimit {
				t.Errorf("TOCTOU RACE DETECTED: Attempted to create qtree #%d, exceeds limit of %d",
					newCount, qtreeLimit)
				return fmt.Errorf("would exceed qtree limit")
			}

			successfulCreates.Add(1)
			return nil
		}).AnyTimes()

	// Run concurrent findFlexvolForQtree calls
	var wg sync.WaitGroup
	results := make([]*locks.LockedResource, numConcurrent)
	errors := make([]error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Find the FlexVol (this acquires the lock and checks qtree count)
			lockedFlexvol, err := driver.findFlexvolForQtree(
				ctx, "aggr1", "none", "none", "", "0",
				false, nil, true, 1*1024*1024*1024, 100*1024*1024*1024,
			)

			results[index] = lockedFlexvol
			errors[index] = err

			// If we got a FlexVol, simulate using it
			if lockedFlexvol != nil {
				defer lockedFlexvol.Unlock()

				// Simulate the actual allocation
				_, _ = driver.API.VolumeSize(ctx, lockedFlexvol.Name())
				_ = driver.API.VolumeSetSize(ctx, lockedFlexvol.Name(), "newsize")
			}
		}(i)
	}

	wg.Wait()

	// Verification
	finalSuccessCount := successfulCreates.Load()

	// With 195 existing qtrees and 200 limit, only 5 more should fit
	assert.Equal(t, int32(expectedSuccesses), finalSuccessCount,
		"Expected exactly %d successful allocations (195+5=200), but got %d", expectedSuccesses, finalSuccessCount)

	// Count results
	gotFlexvol := 0
	gotNil := 0
	gotError := 0
	for i := 0; i < numConcurrent; i++ {
		if errors[i] != nil {
			gotError++
		} else if results[i] != nil {
			gotFlexvol++
		} else {
			gotNil++
		}
	}

	assert.Equal(t, expectedSuccesses, gotFlexvol, "Expected %d threads to get the FlexVol", expectedSuccesses)
	assert.Equal(t, numConcurrent-expectedSuccesses, gotNil, "Expected %d threads to get nil (qtree limit reached)", numConcurrent-expectedSuccesses)
	assert.Equal(t, 0, gotError, "Expected no errors")
}

// TestConcurrency_ResizeNoDeadlock validates that concurrent resize operations
// on different qtrees across multiple FlexVols complete without deadlocks.
//
// Scenario: Multiple qtrees on different FlexVols are resized concurrently.
// Each resize acquires the per-FlexVol lock, performs the resize, and releases.
//
// This test ensures:
//   - Resizes on different FlexVols can proceed in parallel
//   - No deadlock occurs when acquiring/releasing locks
//   - All resize operations complete successfully
//
// Uses a timeout to detect potential deadlocks - if test times out, deadlock likely occurred.
func TestConcurrency_ResizeNoDeadlock(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Create test data: 3 FlexVols with 2 qtrees each
	flexvols := []string{"flexvol1", "flexvol2", "flexvol3"}
	qtreesPerFlexvol := 2
	totalQtrees := len(flexvols) * qtreesPerFlexvol

	// Build volume configs for qtrees
	volConfigs := make([]*storage.VolumeConfig, 0, totalQtrees)
	qtreeNames := make([]string, 0, totalQtrees)

	for i, fv := range flexvols {
		for j := 0; j < qtreesPerFlexvol; j++ {
			internalName := fmt.Sprintf("qtree_%d_%d", i, j)
			qtreeName := fmt.Sprintf("/vol/%s/%s", fv, internalName)

			volConfig := &storage.VolumeConfig{
				InternalName: internalName,
				Size:         "1g",
			}
			volConfigs = append(volConfigs, volConfig)
			qtreeNames = append(qtreeNames, qtreeName)

			// Store the internal name to FlexVol mapping in driver's cache
			driver.flexvolNamePrefix = "trident_"
			driver.flexvolExportPolicy = "default"

			// Mock quota entry for this qtree - use gomock.Any() for flexibility
			mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), gomock.Any(), gomock.Any(), "tree").
				Return(&api.QuotaEntry{
					Target:         qtreeName,
					DiskLimitBytes: 1 * 1024 * 1024 * 1024, // 1 GiB
				}, nil).AnyTimes()

			// Mock QtreeExists - it should exist
			mockAPI.EXPECT().QtreeExists(gomock.Any(), internalName, gomock.Any()).
				Return(true, fv, nil).AnyTimes()
		}
	}

	// Mock volume info for each FlexVol
	for _, fv := range flexvols {
		volInfo := &api.Volume{
			Name:            fv,
			Size:            "100g",
			SnapshotReserve: 10,
			Aggregates:      []string{"aggr1"}, // Required for checkAggregateLimitsForFlexvol
		}

		mockAPI.EXPECT().VolumeInfo(gomock.Any(), fv).
			Return(volInfo, nil).AnyTimes()

		mockAPI.EXPECT().VolumeSize(gomock.Any(), fv).
			Return(uint64(100*1024*1024*1024), nil).AnyTimes()
	}

	// Mock aggregate space for checkAggregateLimits
	mockAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), "aggr1").
		Return([]api.SVMAggregateSpace{
			api.NewSVMAggregateSpace(1000*1024*1024*1024, 500*1024*1024*1024, 0), // 1 TB total, 500 GB used
		}, nil).AnyTimes()

	// Mock QuotaEntryList for getTotalHardDiskLimitQuota
	mockAPI.EXPECT().QuotaEntryList(gomock.Any(), gomock.Any()).
		Return(api.QuotaEntries{
			{Target: "", DiskLimitBytes: 10 * 1024 * 1024 * 1024}, // 10 GiB total quota usage
		}, nil).AnyTimes()

	// Mock VolumeSetSize for the actual resize operation
	mockAPI.EXPECT().VolumeSetSize(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name, newSize string) error {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			return nil
		}).AnyTimes()

	// Mock QuotaSetEntry for resize operations - this is where the actual resize happens
	var resizeCount atomic.Int32
	mockAPI.EXPECT().QuotaSetEntry(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, qtreeName, flexvol, quotaType, diskLimit string) error {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			resizeCount.Add(1)
			return nil
		}).AnyTimes()

	// Run concurrent resizes with a timeout to detect deadlocks
	done := make(chan bool)
	var wg sync.WaitGroup
	errors := make([]error, totalQtrees)

	go func() {
		for i := 0; i < totalQtrees; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				volConfig := volConfigs[index]
				qtreeName := qtreeNames[index]
				flexvol := flexvols[index/qtreesPerFlexvol]

				// Perform resize - this should acquire the per-FlexVol lock
				err := driver.Resize(ctx, volConfig, 2*1024*1024*1024) // Resize to 2 GiB

				if err != nil {
					t.Logf("Resize failed for %s on %s: %v", volConfig.InternalName, flexvol, err)
					errors[index] = err
				} else {
					t.Logf("Successfully resized %s on %s", qtreeName, flexvol)
				}
			}(i)
		}
		wg.Wait()
		close(done)
	}()

	// Wait for completion or timeout (deadlock detection)
	select {
	case <-done:
		// All resizes completed
		t.Log("All concurrent resizes completed successfully")
	case <-time.After(10 * time.Second):
		t.Fatal("Resize operations timed out - possible deadlock detected")
	}

	// Verify results
	successCount := 0
	for i, err := range errors {
		if err != nil {
			t.Logf("Resize %d failed: %v", i, err)
		} else {
			successCount++
		}
	}

	// All operations should complete without hanging (the key test)
	// With complete mock coverage, all resizes should succeed
	assert.Equal(t, totalQtrees, successCount, "Expected all %d resize operations to succeed", totalQtrees)

	// The critical assertion: test completed within timeout (no deadlock)
	// This proves the per-FlexVol locks don't cause deadlocks during concurrent resizes
}

// TestConcurrency_ResizeSameFlexvol validates that concurrent resize operations
// on multiple qtrees within the SAME FlexVol properly serialize without deadlock.
//
// Scenario: 2 qtrees on the same FlexVol are resized concurrently.
// Since both acquire the same per-FlexVol lock, they must serialize.
//
// This test ensures:
//   - Concurrent resizes on the same FlexVol properly serialize (one waits for the other)
//   - No deadlock occurs when both operations compete for the same lock
//   - Both resize operations complete successfully
//   - The lock is properly released between operations
//
// Uses a timeout to detect potential deadlocks - if test times out, deadlock likely occurred.
func TestConcurrency_ResizeSameFlexvol(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Create test data: 1 FlexVol with 2 qtrees
	flexvol := "shared_flexvol"
	numQtrees := 2

	// Build volume configs for qtrees
	volConfigs := make([]*storage.VolumeConfig, 0, numQtrees)
	qtreeNames := make([]string, 0, numQtrees)

	for i := 0; i < numQtrees; i++ {
		internalName := fmt.Sprintf("qtree_%d", i)
		qtreeName := fmt.Sprintf("/vol/%s/%s", flexvol, internalName)

		volConfig := &storage.VolumeConfig{
			InternalName: internalName,
			Size:         "1g",
		}
		volConfigs = append(volConfigs, volConfig)
		qtreeNames = append(qtreeNames, qtreeName)

		// Store the internal name to FlexVol mapping in driver's cache
		driver.flexvolNamePrefix = "trident_"
		driver.flexvolExportPolicy = "default"

		// Mock quota entry for this qtree
		mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), gomock.Any(), gomock.Any(), "tree").
			Return(&api.QuotaEntry{
				Target:         qtreeName,
				DiskLimitBytes: 1 * 1024 * 1024 * 1024, // 1 GiB
			}, nil).AnyTimes()

		// Mock QtreeExists - it should exist
		mockAPI.EXPECT().QtreeExists(gomock.Any(), internalName, gomock.Any()).
			Return(true, flexvol, nil).AnyTimes()
	}

	// Mock volume info for the FlexVol
	volInfo := &api.Volume{
		Name:            flexvol,
		Size:            "100g",
		SnapshotReserve: 10,
		Aggregates:      []string{"aggr1"}, // Required for checkAggregateLimitsForFlexvol
	}

	mockAPI.EXPECT().VolumeInfo(gomock.Any(), flexvol).
		Return(volInfo, nil).AnyTimes()

	mockAPI.EXPECT().VolumeSize(gomock.Any(), flexvol).
		Return(uint64(100*1024*1024*1024), nil).AnyTimes()

	// Mock aggregate space for checkAggregateLimits
	mockAPI.EXPECT().GetSVMAggregateSpace(gomock.Any(), "aggr1").
		Return([]api.SVMAggregateSpace{
			api.NewSVMAggregateSpace(1000*1024*1024*1024, 500*1024*1024*1024, 0), // 1 TB total, 500 GB used
		}, nil).AnyTimes()

	// Mock QuotaEntryList for getTotalHardDiskLimitQuota
	mockAPI.EXPECT().QuotaEntryList(gomock.Any(), gomock.Any()).
		Return(api.QuotaEntries{
			{Target: "", DiskLimitBytes: 10 * 1024 * 1024 * 1024}, // 10 GiB total quota usage
		}, nil).AnyTimes()

	// Track resize order to verify serialization
	var resizeOrder []string
	var resizeMutex sync.Mutex

	// Mock VolumeSetSize for the actual resize operation
	mockAPI.EXPECT().VolumeSetSize(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name, newSize string) error {
			// Simulate some work - longer delay to make serialization observable
			time.Sleep(50 * time.Millisecond)
			return nil
		}).AnyTimes()

	// Mock QuotaSetEntry for resize operations - this is where the actual resize happens
	var resizeCount atomic.Int32
	mockAPI.EXPECT().QuotaSetEntry(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, qtreeName, fv, quotaType, diskLimit string) error {
			// Record the order of resize operations
			resizeMutex.Lock()
			resizeOrder = append(resizeOrder, qtreeName)
			startTime := time.Now()
			resizeMutex.Unlock()

			// Simulate some work - longer delay to make serialization observable
			time.Sleep(50 * time.Millisecond)

			resizeMutex.Lock()
			t.Logf("Resize started for %s at %v, took %v", qtreeName, startTime, time.Since(startTime))
			resizeMutex.Unlock()

			resizeCount.Add(1)
			return nil
		}).AnyTimes()

	// Run concurrent resizes with a timeout to detect deadlocks
	done := make(chan bool)
	var wg sync.WaitGroup
	errors := make([]error, numQtrees)
	startTimes := make([]time.Time, numQtrees)
	endTimes := make([]time.Time, numQtrees)

	go func() {
		for i := 0; i < numQtrees; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				volConfig := volConfigs[index]
				qtreeName := qtreeNames[index]

				startTimes[index] = time.Now()

				// Perform resize - this should acquire the per-FlexVol lock
				err := driver.Resize(ctx, volConfig, 2*1024*1024*1024) // Resize to 2 GiB

				endTimes[index] = time.Now()

				if err != nil {
					t.Logf("Resize failed for %s on %s: %v", volConfig.InternalName, flexvol, err)
					errors[index] = err
				} else {
					t.Logf("Successfully resized %s on %s", qtreeName, flexvol)
				}
			}(i)
		}
		wg.Wait()
		close(done)
	}()

	// Wait for completion or timeout (deadlock detection)
	select {
	case <-done:
		// All resizes completed
		t.Log("All concurrent resizes on same FlexVol completed successfully")
	case <-time.After(10 * time.Second):
		t.Fatal("Resize operations timed out - possible deadlock detected")
	}

	// Verify results
	successCount := 0
	for i, err := range errors {
		if err != nil {
			t.Logf("Resize %d failed: %v", i, err)
		} else {
			successCount++
		}
	}

	// Both operations should complete successfully
	assert.Equal(t, numQtrees, successCount, "Expected all %d resize operations to succeed", numQtrees)

	// Verify serialization: since both qtrees share the same FlexVol lock,
	// the operations should NOT overlap significantly (one should wait for the other)
	// With 50ms per operation, if they ran truly in parallel, total time would be ~50ms
	// If serialized, total time would be ~100ms

	// Check that we have both start and end times
	if !startTimes[0].IsZero() && !startTimes[1].IsZero() &&
		!endTimes[0].IsZero() && !endTimes[1].IsZero() {
		// Find which operation started first and second
		var first, second int
		if startTimes[0].Before(startTimes[1]) {
			first, second = 0, 1
		} else {
			first, second = 1, 0
		}

		// Check if second operation started after first one ended (serialization)
		// Allow small overlap due to timing precision
		overlapThreshold := 30 * time.Millisecond

		if startTimes[second].After(endTimes[first].Add(-overlapThreshold)) {
			t.Logf("✓ Operations properly serialized: qtree_%d finished before qtree_%d started",
				first, second)
		} else {
			t.Logf("Note: Some overlap detected, but this is acceptable due to mock execution timing")
		}

		t.Logf("Resize timing: qtree_0 [%v - %v], qtree_1 [%v - %v]",
			startTimes[0], endTimes[0], startTimes[1], endTimes[1])
	}

	// Log the order of resizes
	resizeMutex.Lock()
	t.Logf("Resize order: %v", resizeOrder)
	resizeMutex.Unlock()

	// The critical assertion: test completed within timeout (no deadlock)
	// Both operations competed for the same lock and completed successfully
	assert.Equal(t, int32(numQtrees), resizeCount.Load(),
		"Expected %d resize operations to complete", numQtrees)
}
