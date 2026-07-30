// Copyright 2025 NetApp, Inc. All Rights Reserved.

package locks

import (
	"context"

	"github.com/netapp/trident/logging"
)

// sharedLocks provides garbage-collected named locks for package-level Lock/Unlock.
var sharedLocks = NewGCNamedMutex()

// Lock acquires a mutex with the specified ID. The mutex does not need to exist
// before calling this method. Semantics match sync.Mutex.Lock().
//
// ctx is used for logging only; acquisition is not cancelled when ctx is done.
func Lock(ctx context.Context, lockContext, lockID string) {
	logging.Logc(ctx).WithField("lockContext", lockContext).Debugf(
		"Attempting to acquire shared lock (%s).", lockID)
	sharedLocks.Lock(lockID)
	logging.Logc(ctx).WithField("lockContext", lockContext).Debugf("Acquired shared lock (%s).", lockID)
}

// Unlock releases a mutex with the specified ID. Semantics match sync.Mutex.Unlock().
func Unlock(ctx context.Context, lockContext, lockID string) {
	sharedLocks.Unlock(lockID)
	logging.Logc(ctx).WithField("lockContext", lockContext).Debugf("Released shared lock (%s).", lockID)
}
