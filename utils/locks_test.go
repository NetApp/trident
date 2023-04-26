// Copyright 2018 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
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
	lockID := "waitLock"

	r := make(chan string, 3)
	m1 := make(chan string, 3)
	m2 := make(chan string, 3)
	m3 := make(chan string, 3)

	go acquire1(m1, r, ctx1, lockID)
	go acquire2(m2, r, ctx2, lockID)
	go acquire3(m3, r, ctx3, lockID)

	assert.True(t, WaitQueueSize(lockID) == 0, "Expected Queue size to be 0.")
	m3 <- "lock"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 0, "Expected Queue size to be 0.")
	m2 <- "lock"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 1, "Expected Queue size to be 1.")
	m1 <- "lock"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 2, "Expected Queue size to be 2.")
	m3 <- "unlock"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 1, "Expected Queue size to be 1.")
	m3 <- "done"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 1, "Expected Queue size to be 1.")
	m2 <- "unlock"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 0, "Expected Queue size to be 0.")
	m2 <- "done"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 0, "Expected Queue size to be 0.")
	m1 <- "unlock"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 0, "Expected Queue size to be 0.")
	m1 <- "done"
	snooze()
	assert.True(t, WaitQueueSize(lockID) == 0, "Expected Queue size to be 0.")

	r1 := <-r
	r2 := <-r
	r3 := <-r

	if r1 != "done3" && r2 != "done2" && r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
}

func TestWaitQueueSize2(t *testing.T) {
	ctx1, ctx2, ctx3 := "testContext1", "testContext2", "testContext3"
	lockID := "waitLock"

	r := make(chan string, 3)
	m1 := make(chan string, 3)
	m2 := make(chan string, 3)
	m3 := make(chan string, 3)

	go acquire1(m1, r, ctx1, lockID)
	go acquire2(m2, r, ctx2, lockID)
	go acquire3(m3, r, ctx3, lockID)

	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 0.")
	m3 <- "lock"
	m2 <- "lock"

	assert.True(t, waitUntilHelper(lockID, 1), "Expected Queue size to be 2.")

	Unlock(ctx(), ctx3, lockID)
	snooze()
	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 1.")

	m1 <- "lock"

	m3 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID, 1), "Expected Queue size to be 1.")

	Unlock(ctx(), ctx2, lockID)
	snooze()
	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 0.")

	m2 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 0.")

	Unlock(ctx(), ctx1, lockID)
	snooze()
	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 0.")

	m1 <- "done"
	snooze()
	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 0.")

	r1 := <-r
	r2 := <-r
	r3 := <-r

	if r1 != "done3" && r2 != "done2" && r3 != "done1" {
		t.Error("Expected done3 followed by done2 followed by done1")
	}
}

func TestWaitQueueSize3(t *testing.T) {
	const total = uint32(10)
	r := make(chan string, total)
	var allChan [total]chan string
	lockID := "waitLock"

	for i := 1; i <= int(total); i++ {
		allChan[i-1] = make(chan string)
	}

	assert.True(t, waitUntilHelper(lockID, 0), "Expected Queue size to be 0.")

	for i := 0; i < int(total); i++ {
		allChan[i] = make(chan string, 3)

		go acquireX(allChan[i], r, "textContext"+strconv.Itoa(i+1), lockID)
		allChan[i] <- "lock"

		snooze()
		assert.True(t, waitUntilHelper(lockID, uint32(i)),
			fmt.Sprintf("Expected Queue size to be %v", i))
	}

	assert.True(t, waitUntilHelper(lockID, total-1), fmt.Sprintf("Expected Queue size to be %v.",
		total-1))

	for i := 0; i < int(total); i++ {
		expectedCount := total - (uint32(i) + 2)
		if total < (uint32(i) + 2) {
			expectedCount = 0
		}

		Unlock(ctx(), "textContext"+strconv.Itoa(i+1), lockID)
		snooze()

		assert.True(t, waitUntilHelper(lockID, expectedCount),
			fmt.Sprintf("Expected Queue size to be %v", expectedCount))

		allChan[i] <- "done"

		snooze()
		assert.True(t, waitUntilHelper(lockID, expectedCount),
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
