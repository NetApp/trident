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
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/locks"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// ============================================================================
// Phase 2: deleteBucketIfEmpty Tests - With Simple Mocks
// These tests verify concurrent bucket deletion operations
// ============================================================================

// TestSANEco_DeleteBucket_ConcurrentSameBucket verifies that concurrent
// delete operations on the same bucket are properly serialized
func TestSANEco_DeleteBucket_ConcurrentSameBucket(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := &SANEconomyStorageDriver{
		API:          mockAPI,
		flexvolLocks: locks.NewGCNamedMutex(),
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: make(map[string]bool),
			},
		},
	}

	bucketName := "bucket1"
	const numGoroutines = 10

	// Track how many times VolumeDestroy is called
	var destroyCalls atomic.Int32
	var lunListCalls atomic.Int32

	// Mock: LunList behavior changes after first deletion
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, pattern string) (api.Luns, error) {
			callNum := lunListCalls.Add(1)
			// First call: bucket exists but is empty
			if callNum == 1 {
				return api.Luns{}, nil
			}
			// After first deletion, bucket no longer exists - return error
			return nil, fmt.Errorf("volume not found")
		},
	).AnyTimes()

	// Mock: VolumeDestroy should only be called once
	mockAPI.EXPECT().VolumeDestroy(ctx, bucketName, true, true).DoAndReturn(
		func(ctx context.Context, name string, force1 bool, force2 bool) error {
			destroyCalls.Add(1)
			return nil
		},
	).MaxTimes(1) // Critical: should only be called once despite concurrent attempts

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errorCount atomic.Int32

	// Launch concurrent delete attempts on the same bucket
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := driver.deleteBucketIfEmpty(ctx, bucketName)
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Verify VolumeDestroy was called exactly once (protected by lock)
	assert.Equal(t, int32(1), destroyCalls.Load(),
		"VolumeDestroy should be called exactly once despite %d concurrent attempts", numGoroutines)

	// Verify only one goroutine succeeded, others got errors
	assert.Equal(t, int32(1), successCount.Load(),
		"Only one goroutine should succeed")
	assert.Equal(t, int32(numGoroutines-1), errorCount.Load(),
		"Other %d goroutines should fail after bucket is deleted", numGoroutines-1)

	// At least one goroutine should have succeeded
	assert.GreaterOrEqual(t, successCount.Load(), int32(1),
		"At least one delete operation should succeed")
}

// TestSANEco_DeleteBucket_ConcurrentDifferentBuckets verifies that
// delete operations on different buckets can proceed in parallel
func TestSANEco_DeleteBucket_ConcurrentDifferentBuckets(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := &SANEconomyStorageDriver{
		API:          mockAPI,
		flexvolLocks: locks.NewGCNamedMutex(),
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: make(map[string]bool),
			},
		},
	}

	const numBuckets = 5
	var destroyCalls atomic.Int32
	var concurrentOps atomic.Int32
	var maxConcurrent atomic.Int32

	// Mock: Each bucket appears empty
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil).AnyTimes()

	// Mock: VolumeDestroy tracks concurrent operations to prove parallelism
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, true).DoAndReturn(
		func(ctx context.Context, name string, force1 bool, force2 bool) error {
			current := concurrentOps.Add(1)
			// Update max if this is higher
			for {
				oldMax := maxConcurrent.Load()
				if current <= oldMax || maxConcurrent.CompareAndSwap(oldMax, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond) // Simulate work
			concurrentOps.Add(-1)
			destroyCalls.Add(1)
			return nil
		},
	).Times(numBuckets)

	var wg sync.WaitGroup

	// Launch concurrent deletes on different buckets
	for i := 0; i < numBuckets; i++ {
		wg.Add(1)
		go func(bucketID int) {
			defer wg.Done()
			bucketName := fmt.Sprintf("bucket_%d", bucketID)
			err := driver.deleteBucketIfEmpty(ctx, bucketName)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all buckets were deleted
	assert.Equal(t, int32(numBuckets), destroyCalls.Load(),
		"All %d buckets should be deleted", numBuckets)

	// Verify parallelism: multiple operations ran concurrently
	assert.Greater(t, maxConcurrent.Load(), int32(1),
		"Multiple delete operations should run in parallel (max concurrent: %d)", maxConcurrent.Load())
}

// TestSANEco_DeleteBucket_MixedEmptyAndNonEmpty verifies behavior when
// some buckets are empty and others have LUNs
func TestSANEco_DeleteBucket_MixedEmptyAndNonEmpty(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := &SANEconomyStorageDriver{
		API:          mockAPI,
		flexvolLocks: locks.NewGCNamedMutex(),
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: make(map[string]bool),
			},
		},
	}

	emptyBucket := "empty_bucket"
	nonEmptyBucket := "nonempty_bucket"

	// Mock: Empty bucket returns no LUNs
	mockAPI.EXPECT().LunList(ctx, fmt.Sprintf("/vol/%s/*", emptyBucket)).Return(
		api.Luns{}, nil,
	).AnyTimes()

	// Mock: Non-empty bucket returns LUNs
	mockAPI.EXPECT().LunList(ctx, fmt.Sprintf("/vol/%s/*", nonEmptyBucket)).Return(
		api.Luns{
			{Name: "/vol/nonempty_bucket/lun1", Size: "1073741824"},
		}, nil,
	).AnyTimes()

	// Mock: Only empty bucket should be deleted
	mockAPI.EXPECT().VolumeDestroy(ctx, emptyBucket, true, true).Return(nil).Times(1)

	// Mock: Non-empty bucket should be resized, not deleted
	mockAPI.EXPECT().VolumeInfo(ctx, nonEmptyBucket).Return(
		&api.Volume{Name: nonEmptyBucket, Size: "10737418240"}, nil,
	).AnyTimes()
	mockAPI.EXPECT().VolumeSetSize(ctx, nonEmptyBucket, gomock.Any()).Return(nil).Times(1)

	var wg sync.WaitGroup

	// Concurrent operations: delete empty, resize non-empty
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := driver.deleteBucketIfEmpty(ctx, emptyBucket)
		assert.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		err := driver.deleteBucketIfEmpty(ctx, nonEmptyBucket)
		assert.NoError(t, err)
	}()

	wg.Wait()
}

// ============================================================================
// Phase 3: CloneLUN Tests - Concurrent Clone Operations
// These tests verify concurrent LUN cloning operations with locking
// ============================================================================

// TestSANEco_CloneLUN_ConcurrentSameSource verifies that multiple concurrent
// clones from the same source LUN in the same bucket work correctly
func TestSANEco_CloneLUN_ConcurrentSameSource(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := &SANEconomyStorageDriver{
		API:          mockAPI,
		flexvolLocks: locks.NewGCNamedMutex(),
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: make(map[string]bool),
			},
		},
	}

	bucketName := "bucket1"
	sourcePath := "/vol/bucket1/lun_source"
	const numClones = 5

	var cloneCalls atomic.Int32

	// Mock: LunCloneCreate should be called once per clone
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketName, sourcePath, gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, volName, source, clone string, qos api.QosPolicyGroup) error {
			cloneCalls.Add(1)
			return nil
		},
	).Times(numClones)

	// Mock: VolumeInfo for resize checks (called after each clone)
	mockAPI.EXPECT().VolumeInfo(ctx, bucketName).Return(&api.Volume{
		Size:       "1073741824", // 1GB
		Aggregates: []string{"aggr1"},
	}, nil).AnyTimes()

	// Mock: VolumeSetSize may be called for resize after clones
	mockAPI.EXPECT().VolumeSetSize(ctx, bucketName, gomock.Any()).Return(nil).AnyTimes()

	// Mock: LunList to determine bucket usage
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil).AnyTimes()

	var wg sync.WaitGroup
	var successCount atomic.Int32

	// Launch concurrent clones from same source
	for i := 0; i < numClones; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cloneName := fmt.Sprintf("clone_%d", id)
			_, err := driver.cloneLUNFromSnapshot(ctx, bucketName, sourcePath, cloneName, api.QosPolicyGroup{})
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Verify all clones succeeded
	assert.Equal(t, int32(numClones), successCount.Load(),
		"All %d clones should succeed", numClones)
	assert.Equal(t, int32(numClones), cloneCalls.Load(),
		"LunCloneCreate should be called %d times", numClones)
}

// TestSANEco_CloneLUN_ConcurrentDifferentBuckets verifies that clones in
// different buckets can proceed in parallel without blocking each other
func TestSANEco_CloneLUN_ConcurrentDifferentBuckets(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := &SANEconomyStorageDriver{
		API:          mockAPI,
		flexvolLocks: locks.NewGCNamedMutex(),
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: make(map[string]bool),
			},
		},
	}

	const numBuckets = 5
	const delayMs = 10

	var concurrentOps atomic.Int32
	var maxConcurrent atomic.Int32

	// Mock: Track concurrent operations to prove parallelism
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, volName, source, clone string, qos api.QosPolicyGroup) error {
			current := concurrentOps.Add(1)
			// Update max if this is higher
			for {
				oldMax := maxConcurrent.Load()
				if current <= oldMax || maxConcurrent.CompareAndSwap(oldMax, current) {
					break
				}
			}
			time.Sleep(delayMs * time.Millisecond)
			concurrentOps.Add(-1)
			return nil
		},
	).Times(numBuckets)

	// Mock: VolumeInfo for each bucket
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{
		Size:       "1073741824",
		Aggregates: []string{"aggr1"},
	}, nil).AnyTimes()

	// Mock: Resize operations
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil).AnyTimes()

	var wg sync.WaitGroup

	// Launch clones in different buckets
	for i := 0; i < numBuckets; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			bucketName := fmt.Sprintf("bucket_%d", id)
			sourcePath := fmt.Sprintf("/vol/%s/lun_source", bucketName)
			cloneName := fmt.Sprintf("clone_%d", id)
			_, err := driver.cloneLUNFromSnapshot(ctx, bucketName, sourcePath, cloneName, api.QosPolicyGroup{})
			require.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify parallelism: multiple operations ran concurrently
	assert.Greater(t, maxConcurrent.Load(), int32(1),
		"Multiple clone operations should run in parallel (max concurrent: %d)", maxConcurrent.Load())
}

// TestSANEco_CloneLUN_ConcurrentSameBucket verifies that multiple clones
// in the same bucket are properly serialized by the FlexVol lock
func TestSANEco_CloneLUN_ConcurrentSameBucket(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := &SANEconomyStorageDriver{
		API:          mockAPI,
		flexvolLocks: locks.NewGCNamedMutex(),
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				DebugTraceFlags: make(map[string]bool),
			},
		},
	}

	bucketName := "bucket1"
	const numClones = 5
	const delayMs = 10

	var cloneCalls atomic.Int32
	var concurrentOps atomic.Int32
	var maxConcurrent atomic.Int32

	// Mock: Track concurrent operations
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketName, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, volName, source, clone string, qos api.QosPolicyGroup) error {
			current := concurrentOps.Add(1)
			// Update max if this is higher
			for {
				oldMax := maxConcurrent.Load()
				if current <= oldMax || maxConcurrent.CompareAndSwap(oldMax, current) {
					break
				}
			}
			time.Sleep(delayMs * time.Millisecond)
			concurrentOps.Add(-1)
			cloneCalls.Add(1)
			return nil
		},
	).Times(numClones)

	// Mock: VolumeInfo for resize
	mockAPI.EXPECT().VolumeInfo(ctx, bucketName).Return(&api.Volume{
		Size:       "1073741824",
		Aggregates: []string{"aggr1"},
	}, nil).AnyTimes()

	// Mock: Resize operations
	mockAPI.EXPECT().VolumeSetSize(ctx, bucketName, gomock.Any()).Return(nil).AnyTimes()
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil).AnyTimes()

	var wg sync.WaitGroup
	start := time.Now()

	// Launch multiple clones in the same bucket
	for i := 0; i < numClones; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sourcePath := fmt.Sprintf("/vol/%s/lun_source_%d", bucketName, id)
			cloneName := fmt.Sprintf("clone_%d", id)
			_, err := driver.cloneLUNFromSnapshot(ctx, bucketName, sourcePath, cloneName, api.QosPolicyGroup{})
			require.NoError(t, err)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Verify serialization: operations should be serialized (max 1 concurrent)
	assert.Equal(t, int32(1), maxConcurrent.Load(),
		"Clones in same bucket should be serialized (max concurrent: %d)", maxConcurrent.Load())

	// Verify all clones completed
	assert.Equal(t, int32(numClones), cloneCalls.Load(),
		"All %d clones should complete", numClones)

	// Verify execution took longer than single operation (serialized, not parallel)
	minExpected := time.Duration((numClones-1)*delayMs) * time.Millisecond
	assert.GreaterOrEqual(t, elapsed, minExpected,
		"Serialized clones should take at least %v (took %v)", minExpected, elapsed)
}
