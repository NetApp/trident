// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"sync"

	. "github.com/netapp/trident/logger"
)

// sync.Map is like a Go map[interface{}]interface{} but is safe for concurrent use by multiple
// goroutines without additional locking or coordination.
var sharedLocks sync.Map

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
	Logc(ctx).WithField("lock", lockID).Debugf("Attempting to acquire shared lock (%s).", lockContext)
	getLock(ctx, lockID).Lock()
	Logc(ctx).WithField("lock", lockID).Debugf("Acquired shared lock (%s).", lockContext)
}

// Unlock releases a mutex with the specified ID.  The semantics of this method are intentionally
// identical to sync.Mutex.Unlock().
func Unlock(ctx context.Context, lockContext, lockID string) {
	getLock(ctx, lockID).Unlock()
	Logc(ctx).WithField("lock", lockID).Debugf("Released shared lock (%s).", lockContext)
}
