// Copyright 2018 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"
	"time"
)

var ctx = context.Background

func TestLockCreated(t *testing.T) {
	Lock(ctx(), "testContext", "myLock")
	defer Unlock(ctx(), "testContext", "myLock")

	if _, ok := sharedLocks.lockMap["myLock"]; !ok {
		t.Error("Expected lock myLock to exist.")
	}

	if _, ok := sharedLocks.lockMap["myLock2"]; ok {
		t.Error("Did not expect lock myLock2 to exist.")
	}
}

func TestLockReused(t *testing.T) {
	Lock(ctx(), "testContext", "reuseLock")
	Unlock(ctx(), "testContext", "reuseLock")

	lock1 := sharedLocks.lockMap["reuseLock"]

	Lock(ctx(), "testContext", "reuseLock")
	Unlock(ctx(), "testContext", "reuseLock")

	lock2 := sharedLocks.lockMap["reuseLock"]

	if lock1 != lock2 {
		t.Error("Expected locks to match.")
	}
}

func acquire1(m1, r chan string) {
	for i := 0; i < 3; i++ {
		op := <-m1
		switch op {
		case "lock":
			Lock(ctx(), "testContext1", "behaviorLock")
		case "unlock":
			Unlock(ctx(), "testContext1", "behaviorLock")
		case "done":
			close(m1)
			r <- "done1"
			return
		}
	}
}

func acquire2(m2, r chan string) {
	for i := 0; i < 3; i++ {
		op := <-m2
		switch op {
		case "lock":
			Lock(ctx(), "testContext2", "behaviorLock")
		case "unlock":
			Unlock(ctx(), "testContext2", "behaviorLock")
		case "done":
			close(m2)
			r <- "done2"
			return
		}
	}
}

func snooze() {
	time.Sleep(1 * time.Millisecond)
}

func TestLockBehavior(t *testing.T) {
	r := make(chan string, 2)
	m1 := make(chan string, 3)
	m2 := make(chan string, 3)
	go acquire1(m1, r)
	go acquire2(m2, r)

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
