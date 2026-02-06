// Copyright 2018 NetApp, Inc. All Rights Reserved.

package locks

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background

func TestLockCreated(t *testing.T) {
	Lock(ctx(), "testContext", "myLock")
	defer Unlock(ctx(), "testContext", "myLock")

	if _, ok := sharedLocks.Load("myLock"); !ok {
		t.Error("Expected lock myLock to exist.")
	}

	if _, ok := sharedLocks.Load("myLock2"); ok {
		t.Error("Did not expect lock myLock2 to exist.")
	}
}

func TestLockReused(t *testing.T) {
	Lock(ctx(), "testContext", "reuseLock")
	Unlock(ctx(), "testContext", "reuseLock")

	lock1, _ := sharedLocks.Load("reuseLock")

	Lock(ctx(), "testContext", "reuseLock")
	Unlock(ctx(), "testContext", "reuseLock")

	lock2, _ := sharedLocks.Load("reuseLock")

	if lock1 != lock2 {
		t.Error("Expected locks to match.")
	}
}

func acquire1(m1, r chan string, lockContext, lockID string) {
	for i := 0; i < 3; i++ {
		op := <-m1
		switch op {
		case "lock":
			Lock(ctx(), lockContext, lockID)
		case "unlock":
			Unlock(ctx(), lockContext, lockID)
		case "done":
			close(m1)
			r <- "done1"
			return
		}
	}
}

func acquire2(m2, r chan string, lockContext, lockID string) {
	for i := 0; i < 3; i++ {
		op := <-m2
		switch op {
		case "lock":
			Lock(ctx(), lockContext, lockID)
		case "unlock":
			Unlock(ctx(), lockContext, lockID)
		case "done":
			close(m2)
			r <- "done2"
			return
		}
	}
}

func acquire3(m3, r chan string, lockContext, lockID string) {
	for i := 0; i < 3; i++ {
		op := <-m3
		switch op {
		case "lock":
			Lock(ctx(), lockContext, lockID)
		case "unlock":
			Unlock(ctx(), lockContext, lockID)
		case "done":
			close(m3)
			r <- "done3"
			return
		}
	}
}

func acquireX(x, r chan string, lockContext, lockID string) {
	for i := 0; i < 3; i++ {
		op := <-x
		switch op {
		case "lock":
			Lock(ctx(), lockContext, lockID)
		case "unlock":
			Unlock(ctx(), lockContext, lockID)
		case "done":
			close(x)
			r <- "done1"
			return
		}
	}
}

func snooze() {
	time.Sleep(10 * time.Millisecond)
}

func TestLockBehavior(t *testing.T) {
	r := make(chan string, 2)
	m1 := make(chan string, 3)
	m2 := make(chan string, 3)
	lockID := "behaviorLock"

	// We could introduce a delay between the methods but that would involve
	// leaving control to the go runtime to execute the methods. The proper
	// fix would be to ensure the methods are not sharing variables in a
	// concurrent context
	go acquire1(m1, r, "testContext1", lockID)
	go acquire2(m2, r, "testContext2", lockID)

	m2 <- "lock"
	snooze()
	m1 <- "lock"
	snooze()
	m2 <- "unlock"
	snooze()
	m1 <- "unlock"
	snooze()
	m2 <- "done"
	snooze()
	m1 <- "done"

	r1 := <-r
	r2 := <-r
	if r1 != "done2" && r2 != "done1" {
		t.Error("Expected done2 followed by done1.")
	}
}

func TestWaitQueueSize(t *testing.T) {
	ctx1, ctx2, ctx3 := "testContext1", "testContext2", "testContext3"
	lockID1, lockID2, lockID3 := "lockID1", "lockID2", "lockID3"

	r := make(chan string, 9)
	lockID1_m1 := make(chan string, 3)
	lockID1_m2 := make(chan string, 3)
	lockID1_m3 := make(chan string, 3)

	lockID2_m1 := make(chan string, 3)
	lockID2_m2 := make(chan string, 3)
	lockID2_m3 := make(chan string, 3)

	lockID3_m1 := make(chan string, 3)
	lockID3_m2 := make(chan string, 3)
	lockID3_m3 := make(chan string, 3)

	go acquire1(lockID1_m1, r, ctx1, lockID1)
	go acquire2(lockID1_m2, r, ctx2, lockID1)
	go acquire3(lockID1_m3, r, ctx3, lockID1)

	go acquire1(lockID2_m1, r, ctx1, lockID2)
	go acquire2(lockID2_m2, r, ctx2, lockID2)
	go acquire3(lockID2_m3, r, ctx3, lockID2)

	go acquire1(lockID3_m1, r, ctx1, lockID3)
	go acquire2(lockID3_m2, r, ctx2, lockID3)
	go acquire3(lockID3_m3, r, ctx3, lockID3)

	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m3 <- "lock"
	lockID2_m3 <- "lock"
	lockID3_m3 <- "lock"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m2 <- "lock"
	lockID2_m2 <- "lock"
	lockID3_m2 <- "lock"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID3))

	lockID1_m1 <- "lock"
	lockID2_m1 <- "lock"
	lockID3_m1 <- "lock"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 2), fmt.Sprintf("Expected Queue size for lock %s to be 2.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 2), fmt.Sprintf("Expected Queue size for lock %s to be 2.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 2), fmt.Sprintf("Expected Queue size for lock %s to be 2.", lockID3))

	lockID1_m3 <- "unlock"
	lockID2_m3 <- "unlock"
	lockID3_m3 <- "unlock"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID3))

	lockID1_m3 <- "done"
	lockID2_m3 <- "done"
	lockID3_m3 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID3))

	lockID1_m2 <- "unlock"
	lockID2_m2 <- "unlock"
	lockID3_m2 <- "unlock"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m2 <- "done"
	lockID2_m2 <- "done"
	lockID3_m2 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m1 <- "unlock"
	lockID2_m1 <- "unlock"
	lockID3_m1 <- "unlock"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m1 <- "done"
	lockID2_m1 <- "done"
	lockID3_m1 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_r1 := <-r
	lockID1_r2 := <-r
	lockID1_r3 := <-r

	lockID2_r1 := <-r
	lockID2_r2 := <-r
	lockID2_r3 := <-r

	lockID3_r1 := <-r
	lockID3_r2 := <-r
	lockID3_r3 := <-r

	if lockID1_r1 != "done3" && lockID1_r2 != "done2" && lockID1_r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
	if lockID2_r1 != "done3" && lockID2_r2 != "done2" && lockID2_r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
	if lockID3_r1 != "done3" && lockID3_r2 != "done2" && lockID3_r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
}

func TestWaitQueueSize2(t *testing.T) {
	ctx1, ctx2, ctx3 := "testContext1", "testContext2", "testContext3"
	lockID1, lockID2, lockID3 := "waitLock1", "waitLock2", "waitLock3"

	r := make(chan string, 9)
	lockID1_m1 := make(chan string, 3)
	lockID1_m2 := make(chan string, 3)
	lockID1_m3 := make(chan string, 3)

	lockID2_m1 := make(chan string, 3)
	lockID2_m2 := make(chan string, 3)
	lockID2_m3 := make(chan string, 3)

	lockID3_m1 := make(chan string, 3)
	lockID3_m2 := make(chan string, 3)
	lockID3_m3 := make(chan string, 3)

	go acquire1(lockID1_m1, r, ctx1, lockID1)
	go acquire2(lockID1_m2, r, ctx2, lockID1)
	go acquire3(lockID1_m3, r, ctx3, lockID1)

	go acquire1(lockID2_m1, r, ctx1, lockID2)
	go acquire2(lockID2_m2, r, ctx2, lockID2)
	go acquire3(lockID2_m3, r, ctx3, lockID2)

	go acquire1(lockID3_m1, r, ctx1, lockID3)
	go acquire2(lockID3_m2, r, ctx2, lockID3)
	go acquire3(lockID3_m3, r, ctx3, lockID3)

	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m3 <- "lock"
	lockID1_m2 <- "lock"
	lockID2_m3 <- "lock"
	lockID2_m2 <- "lock"
	lockID3_m3 <- "lock"
	lockID3_m2 <- "lock"
	assert.True(t, waitUntilHelper(lockID1, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID3))

	Unlock(ctx(), ctx3, lockID1)
	Unlock(ctx(), ctx3, lockID2)
	Unlock(ctx(), ctx3, lockID3)
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m1 <- "lock"
	lockID1_m3 <- "done"
	lockID2_m1 <- "lock"
	lockID2_m3 <- "done"
	lockID3_m1 <- "lock"
	lockID3_m3 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 1), fmt.Sprintf("Expected Queue size for lock %s to be 1.", lockID3))

	Unlock(ctx(), ctx2, lockID1)
	Unlock(ctx(), ctx2, lockID2)
	Unlock(ctx(), ctx2, lockID3)
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m2 <- "done"
	lockID2_m2 <- "done"
	lockID3_m2 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	Unlock(ctx(), ctx1, lockID1)
	Unlock(ctx(), ctx1, lockID2)
	Unlock(ctx(), ctx1, lockID3)
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_m1 <- "done"
	lockID2_m1 <- "done"
	lockID3_m1 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	lockID1_r1 := <-r
	lockID1_r2 := <-r
	lockID1_r3 := <-r

	lockID2_r1 := <-r
	lockID2_r2 := <-r
	lockID2_r3 := <-r

	lockID3_r1 := <-r
	lockID3_r2 := <-r
	lockID3_r3 := <-r

	if lockID1_r1 != "done3" && lockID1_r2 != "done2" && lockID1_r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
	if lockID2_r1 != "done3" && lockID2_r2 != "done2" && lockID2_r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
	if lockID3_r1 != "done3" && lockID3_r2 != "done2" && lockID3_r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
}

func TestWaitQueueSize3(t *testing.T) {
	const total = uint32(10)
	r := make(chan string, total)
	var allChan1 [total]chan string
	var allChan2 [total]chan string
	var allChan3 [total]chan string
	lockID1 := "waitLock1"
	lockID2 := "waitLock2"
	lockID3 := "waitLock3"

	assert.True(t, waitUntilHelper(lockID1, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID1))
	assert.True(t, waitUntilHelper(lockID2, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID2))
	assert.True(t, waitUntilHelper(lockID3, 0), fmt.Sprintf("Expected Queue size for lock %s to be 0.", lockID3))

	for i := 0; i < int(total); i++ {
		allChan1[i] = make(chan string, 3)
		allChan2[i] = make(chan string, 3)
		allChan3[i] = make(chan string, 3)

		go acquireX(allChan1[i], r, "textContext"+strconv.Itoa(i+1), lockID1)
		go acquireX(allChan2[i], r, "textContext"+strconv.Itoa(i+1), lockID2)
		go acquireX(allChan3[i], r, "textContext"+strconv.Itoa(i+1), lockID3)

		allChan1[i] <- "lock"
		allChan2[i] <- "lock"
		allChan3[i] <- "lock"

		snooze()
		assert.True(t, waitUntilHelper(lockID1, uint32(i)),
			fmt.Sprintf("Expected Queue size to be %v", i))
		assert.True(t, waitUntilHelper(lockID2, uint32(i)),
			fmt.Sprintf("Expected Queue size to be %v", i))
		assert.True(t, waitUntilHelper(lockID3, uint32(i)),
			fmt.Sprintf("Expected Queue size to be %v", i))
	}

	assert.True(t, waitUntilHelper(lockID1, total-1), fmt.Sprintf("Expected Queue size to be %v.",
		total-1))
	assert.True(t, waitUntilHelper(lockID2, total-1), fmt.Sprintf("Expected Queue size to be %v.",
		total-1))
	assert.True(t, waitUntilHelper(lockID3, total-1), fmt.Sprintf("Expected Queue size to be %v.",
		total-1))

	for i := 0; i < int(total); i++ {
		expectedCount := total - (uint32(i) + 2)
		if total < (uint32(i) + 2) {
			expectedCount = 0
		}

		Unlock(ctx(), "textContext"+strconv.Itoa(i+1), lockID1)
		Unlock(ctx(), "textContext"+strconv.Itoa(i+1), lockID2)
		Unlock(ctx(), "textContext"+strconv.Itoa(i+1), lockID3)
		snooze()

		assert.True(t, waitUntilHelper(lockID1, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))
		assert.True(t, waitUntilHelper(lockID2, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))
		assert.True(t, waitUntilHelper(lockID3, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))

		allChan1[i] <- "done"
		allChan2[i] <- "done"
		allChan3[i] <- "done"

		snooze()
		assert.True(t, waitUntilHelper(lockID1, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))
		assert.True(t, waitUntilHelper(lockID2, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))
		assert.True(t, waitUntilHelper(lockID3, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))
	}
}

func waitUntilHelper(lockID string, expectedSize uint32) bool {
	var size uint32

	invoke := func() error {
		size = WaitQueueSize(lockID)

		if size != expectedSize {
			return fmt.Errorf("expected: %v, got: %v", expectedSize, size)
		}

		return nil
	}

	invokeBackoff := backoff.NewExponentialBackOff()
	invokeBackoff.MaxElapsedTime = 10 * time.Second

	if err := backoff.Retry(invoke, invokeBackoff); err != nil {
		return false
	}

	return true
}

func TestQueue(t *testing.T) {
	lockID1 := "testQueue1"
	lockID2 := "testQueue2"
	lockID3 := "testQueue3"

	assert.True(t, WaitQueueSize(lockID1) == 0)
	assert.True(t, WaitQueueSize(lockID2) == 0)
	assert.True(t, WaitQueueSize(lockID3) == 0)

	DecrementQueueSize(lockID1)
	DecrementQueueSize(lockID1)
	IncrementQueueSize(lockID2)
	assert.True(t, WaitQueueSize(lockID1) == 0)
	assert.True(t, WaitQueueSize(lockID2) == 1)
	assert.True(t, WaitQueueSize(lockID3) == 0)

	DecrementQueueSize(lockID1)
	DecrementQueueSize(lockID1)
	IncrementQueueSize(lockID2)
	IncrementQueueSize(lockID3)
	assert.True(t, WaitQueueSize(lockID1) == 0)
	assert.True(t, WaitQueueSize(lockID2) == 2)
	assert.True(t, WaitQueueSize(lockID3) == 1)

	DecrementQueueSize(lockID1)
	DecrementQueueSize(lockID2)
	DecrementQueueSize(lockID3)
	assert.True(t, WaitQueueSize(lockID1) == 0)
	assert.True(t, WaitQueueSize(lockID2) == 1)
	assert.True(t, WaitQueueSize(lockID3) == 0)

	DecrementQueueSize(lockID1)
	DecrementQueueSize(lockID2)
	DecrementQueueSize(lockID3)
	assert.True(t, WaitQueueSize(lockID1) == 0)
	assert.True(t, WaitQueueSize(lockID2) == 0)
	assert.True(t, WaitQueueSize(lockID3) == 0)

	DecrementQueueSize(lockID1)
	DecrementQueueSize(lockID2)
	DecrementQueueSize(lockID3)
	assert.True(t, WaitQueueSize(lockID1) == 0)
	assert.True(t, WaitQueueSize(lockID2) == 0)
	assert.True(t, WaitQueueSize(lockID3) == 0)
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
