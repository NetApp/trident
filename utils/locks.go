package utils

import (
	"context"
	"sync"

	. "github.com/netapp/trident/v21/logger"
)

type locks struct {
	lockMap    map[string]*sync.Mutex
	createLock *sync.Mutex
}

var sharedLocks *locks

// init initializes the shared locks struct exactly once per runtime.
func init() {
	sharedLocks = &locks{
		lockMap:    map[string]*sync.Mutex{},
		createLock: &sync.Mutex{},
	}
}

// getLock returns a mutex with the specified ID.  If the lock does not exist, one is created.
// This method uses the check-lock-check pattern to defend against race conditions where multiple
// callers try to get a non-existent lock at the same time.
func getLock(ctx context.Context, lockID string) *sync.Mutex {

	var lock *sync.Mutex
	var ok bool

	if lock, ok = sharedLocks.lockMap[lockID]; !ok {

		sharedLocks.createLock.Lock()
		defer sharedLocks.createLock.Unlock()

		if lock, ok = sharedLocks.lockMap[lockID]; !ok {
			lock = &sync.Mutex{}
			sharedLocks.lockMap[lockID] = lock
			Logc(ctx).WithField("lock", lockID).Debug("Created shared lock.")
		}
	}

	return lock
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
