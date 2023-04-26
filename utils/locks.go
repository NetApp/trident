// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"sync"
	"sync/atomic"

	. "github.com/netapp/trident/logging"
)

// sync.Map is like a Go map[interface{}]interface{} but is safe for concurrent use by multiple
// goroutines without additional locking or coordination.
var sharedLocks, waitQueue sync.Map

// getLock returns a mutex with the specified ID.  If the lock does not exist, one is created.
// This method uses sync.Map primitive (concurrency safe map) to defend against race conditions where multiple
// callers try to get a lock at the same time.
func getLock(ctx context.Context, lockID string) *sync.Mutex {
	newLock := &sync.Mutex{}
	storedLock, loaded := sharedLocks.LoadOrStore(lockID, newLock)
	if !loaded {
		Logc(ctx).WithField("lock", lockID).Debug("Created shared lock.")
	}
	return storedLock.(*sync.Mutex)
}

// Lock acquires a mutex with the specified ID.  The mutex does not need to exist before
// calling this method.  The semantics of this method are intentionally identical to sync.Mutex.Lock().
func Lock(ctx context.Context, lockContext, lockID string) {
	IncrementQueueSize(lockID)
	Logc(ctx).WithField("lock", lockID).Debugf("Attempting to acquire shared lock (%s); %d position in the queue.",
		lockContext, WaitQueueSize(lockID))

	getLock(ctx, lockID).Lock()

	DecrementQueueSize(lockID)
	Logc(ctx).WithField("lock", lockID).Debugf("Acquired shared lock (%s).", lockContext)
}

// Unlock releases a mutex with the specified ID.  The semantics of this method are intentionally
// identical to sync.Mutex.Unlock().
func Unlock(ctx context.Context, lockContext, lockID string) {
	getLock(ctx, lockID).Unlock()
	Logc(ctx).WithField("lock", lockID).Debugf("Released shared lock (%s).", lockContext)
}

// IncrementQueueSize increments the wait queue size by 1
func IncrementQueueSize(lockID string) {
	currentWait, ok := waitQueue.Load(lockID)
	if !ok {
		val := uint32(1)
		valPtr := &val
		waitQueue.Store(lockID, valPtr)
	} else {
		if ptr, ok := currentWait.(*uint32); ok {
			atomic.AddUint32(ptr, 1)
		}
	}
}

// DecrementQueueSize decrements the wait queue size by 1
func DecrementQueueSize(lockID string) {
	currentWait, ok := waitQueue.Load(lockID)
	if ok {
		if ptr, ok := currentWait.(*uint32); ok {
			if *ptr > 0 {
				atomic.AddUint32(ptr, ^uint32(0))
			}
		}
	}
}

// WaitQueueSize returns the wait queue size
func WaitQueueSize(lockID string) uint32 {
	currentWait, ok := waitQueue.Load(lockID)
	if !ok {
		return 0
	}

	return atomic.LoadUint32(currentWait.(*uint32))
}
