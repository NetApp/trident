// Copyright 2018 NetApp, Inc. All Rights Reserved.

package locks

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background

func testLockID(t *testing.T, suffix string) string {
	return t.Name() + "/" + suffix
}

func TestLockReused(t *testing.T) {
	lockID := testLockID(t, "reuse")

	Lock(ctx(), "testContext", lockID)
	Unlock(ctx(), "testContext", lockID)

	Lock(ctx(), "testContext", lockID)
	Unlock(ctx(), "testContext", lockID)
}

func TestLockUnlockSerialization(t *testing.T) {
	lockID := testLockID(t, "serialization")

	const numGoroutines = 50
	var counter int64
	var wg sync.WaitGroup

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Lock(ctx(), "testContext", lockID)
			defer Unlock(ctx(), "testContext", lockID)

			oldValue := atomic.LoadInt64(&counter)
			atomic.StoreInt64(&counter, oldValue+1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(numGoroutines), counter)
}

func TestUnlockWithoutPriorLock(t *testing.T) {
	lockID := testLockID(t, "unlock-no-prior-lock")

	require.NotPanics(t, func() {
		Unlock(ctx(), "testContext", lockID)
	})
}

func TestLockUnlockConcurrentFirstUse(t *testing.T) {
	const goroutines = 32
	lockID := testLockID(t, "concurrent-first-use")

	ready := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var counter int64

	wg.Add(1)
	go func() {
		defer wg.Done()
		Lock(ctx(), "holder", lockID)
		close(ready)
		<-release
		atomic.AddInt64(&counter, 1)
		Unlock(ctx(), "holder", lockID)
	}()
	<-ready

	for range goroutines - 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Lock(ctx(), "waiter", lockID)
			atomic.AddInt64(&counter, 1)
			Unlock(ctx(), "waiter", lockID)
		}()
	}

	close(release)
	wg.Wait()
	assert.Equal(t, int64(goroutines), counter)
}

// TestGCNamedMutex_NoConcurrentHoldersDuringUnlock stresses Unlock/RUnlock so a
// second goroutine cannot enter the critical section while the prior holder is
// still releasing the resource mutex (regression for GC map deletion ordering).
func TestGCNamedMutex_NoConcurrentHoldersDuringUnlock(t *testing.T) {
	g := NewGCNamedMutex()
	name := testLockID(t, "unlock-window")

	const workers = 8
	const iterations = 10000

	var holders atomic.Int32
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				g.Lock(name)
				if holders.Add(1) != 1 {
					t.Errorf("multiple goroutines hold %q concurrently during unlock window (holders=%d)", name, holders.Load())
				}
				for range 50 {
					runtime.Gosched()
				}
				holders.Add(-1)
				g.Unlock(name)
			}
		}()
	}

	wg.Wait()
}

// TestGCNamedMutex_RUnlockBlocksNewLockUntilReleased verifies a writer cannot
// acquire the same name until the last reader's RUnlock completes, including map GC.
func TestGCNamedMutex_RUnlockBlocksNewLockUntilReleased(t *testing.T) {
	g := NewGCNamedMutex()
	name := testLockID(t, "runlock-blocks-write-lock")

	for range 2000 {
		g.RLock(name)

		waiterBlocked := make(chan struct{})
		waiterAcquired := make(chan struct{})
		go func() {
			close(waiterBlocked)
			g.Lock(name)
			close(waiterAcquired)
			g.Unlock(name)
		}()

		<-waiterBlocked
		for range 100 {
			runtime.Gosched()
		}

		select {
		case <-waiterAcquired:
			t.Fatal("writer acquired lock before reader called RUnlock")
		default:
		}

		g.RUnlock(name)

		select {
		case <-waiterAcquired:
		case <-time.After(time.Second):
			t.Fatal("writer did not acquire lock after RUnlock")
		}
	}
}

// TestGCNamedMutex_UnlockBlocksNewLockUntilReleased verifies a waiter cannot
// acquire the same name until Unlock completes, including map GC.
func TestGCNamedMutex_UnlockBlocksNewLockUntilReleased(t *testing.T) {
	g := NewGCNamedMutex()
	name := testLockID(t, "unlock-blocks-new-lock")

	for range 2000 {
		g.Lock(name)

		waiterBlocked := make(chan struct{})
		waiterAcquired := make(chan struct{})
		go func() {
			close(waiterBlocked)
			g.Lock(name)
			close(waiterAcquired)
			g.Unlock(name)
		}()

		<-waiterBlocked
		for range 100 {
			runtime.Gosched()
		}

		select {
		case <-waiterAcquired:
			t.Fatal("waiter acquired lock before holder called Unlock")
		default:
		}

		g.Unlock(name)

		select {
		case <-waiterAcquired:
		case <-time.After(time.Second):
			t.Fatal("waiter did not acquire lock after Unlock")
		}
	}
}

// ============================================================================
// LockedResource and GCNamedMutex Tests
// These tests verify the named lock and LockedResource wrapper functionality
// ============================================================================

// TestLockedResource_BasicLockUnlock verifies basic lock functionality
func TestLockedResource_BasicLockUnlock(t *testing.T) {
	mutex := NewGCNamedMutex()

	lockedResource := mutex.LockWithGuard("test-resource")
	require.NotNil(t, lockedResource)
	assert.Equal(t, "test-resource", lockedResource.Name())

	lockedResource.Unlock()
	lockedResource.Unlock() // Double unlock should be safe

	lockedResource2 := mutex.LockWithGuard("test-resource")
	require.NotNil(t, lockedResource2)
	lockedResource2.Unlock()
}

// TestLockedResource_SerializationSameResource verifies that operations
// on the same resource are properly serialized by the lock
func TestLockedResource_SerializationSameResource(t *testing.T) {
	mutex := NewGCNamedMutex()

	const numGoroutines = 50
	const resourceName = "shared-resource"
	var counter int64
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lockedResource := mutex.LockWithGuard(resourceName)
			defer lockedResource.Unlock()

			oldValue := atomic.LoadInt64(&counter)
			newValue := oldValue + 1
			atomic.StoreInt64(&counter, newValue)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(numGoroutines), counter, "All increments should be serialized")
}

// TestLockedResource_ParallelDifferentResources verifies that operations
// on different resources can proceed in parallel
func TestLockedResource_ParallelDifferentResources(t *testing.T) {
	mutex := NewGCNamedMutex()

	const numResources = 10
	const opsPerResource = 10
	counters := make([]int64, numResources)
	var wg sync.WaitGroup

	for resourceID := 0; resourceID < numResources; resourceID++ {
		for op := 0; op < opsPerResource; op++ {
			wg.Add(1)
			go func(resID int) {
				defer wg.Done()
				resourceName := fmt.Sprintf("resource-%d", resID)
				lockedResource := mutex.LockWithGuard(resourceName)
				defer lockedResource.Unlock()

				oldValue := atomic.LoadInt64(&counters[resID])
				newValue := oldValue + 1
				atomic.StoreInt64(&counters[resID], newValue)
			}(resourceID)
		}
	}

	wg.Wait()

	for i := 0; i < numResources; i++ {
		assert.Equal(t, int64(opsPerResource), counters[i],
			"Resource %d should have %d operations", i, opsPerResource)
	}
}

// TestLockedResource_GarbageCollection verifies that locks are properly cleaned up
func TestLockedResource_GarbageCollection(t *testing.T) {
	mutex := NewGCNamedMutex()

	const numLocks = 1000
	for i := 0; i < numLocks; i++ {
		resourceName := fmt.Sprintf("resource-%d", i)
		lockedResource := mutex.LockWithGuard(resourceName)
		lockedResource.Unlock()
	}

	lockedResource := mutex.LockWithGuard("test-after-gc")
	require.NotNil(t, lockedResource)
	lockedResource.Unlock()

	var wg sync.WaitGroup
	var counter int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lockedResource := mutex.LockWithGuard("test-after-gc")
			defer lockedResource.Unlock()
			atomic.AddInt64(&counter, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(10), counter)
}

// TestLockedResource_PanicRecovery verifies that locks are released even on panic
func TestLockedResource_PanicRecovery(t *testing.T) {
	mutex := NewGCNamedMutex()

	resourceName := "test-panic-resource"

	func() {
		defer func() {
			recover()
		}()
		lockedResource := mutex.LockWithGuard(resourceName)
		defer lockedResource.Unlock()
		panic("test panic")
	}()

	lockedResource := mutex.LockWithGuard(resourceName)
	require.NotNil(t, lockedResource)
	lockedResource.Unlock()
}

// TestLockedResource_NoDeadlock verifies operations complete without deadlock
func TestLockedResource_NoDeadlock(t *testing.T) {
	mutex := NewGCNamedMutex()

	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for res := 0; res < 5; res++ {
			for op := 0; op < 20; op++ {
				wg.Add(1)
				go func(resourceID int) {
					defer wg.Done()
					resourceName := fmt.Sprintf("resource-%d", resourceID)
					lockedResource := mutex.LockWithGuard(resourceName)
					defer lockedResource.Unlock()
					var sum int64
					for i := 0; i < 1000; i++ {
						sum += int64(i)
					}
				}(res)
			}
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout - possible deadlock")
	}
}
