// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import "context"

// Locker executes a critical section under a distributed lock.
// Runtime factors help decide whether this Locker is a no-op or backed by a real mechanism.
type Locker interface {
	WithLock(ctx context.Context, fn func(context.Context) error) error
}
