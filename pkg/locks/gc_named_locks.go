package locks

import "sync"

// LockedResource is a wrapper that holds a named lock and its unlock function.
// It provides idempotent unlock behavior - safe to call Unlock() multiple times.
// Note: Go has no destructors, so the caller must explicitly call Unlock() (typically via defer).
type LockedResource struct {
	name   string
	unlock func()
}

// Name returns the resource name that is locked.
func (lr *LockedResource) Name() string {
	return lr.name
}

// Guard is the unlock handle returned by GCNamedMutex.LockWithGuard.
type Guard interface {
	Name() string
	Unlock()
}

// NamedLocker provides garbage-collected named write locks. *GCNamedMutex is the
// production implementation; tests may inject gomock.MockNamedLocker to assert lock scope.
//
//go:generate mockgen -destination=../../mocks/mock_pkg/mock_locks/mock_named_locker.go github.com/netapp/trident/pkg/locks NamedLocker
type NamedLocker interface {
	LockWithGuard(name string) Guard
}

// Compile-time check that *LockedResource implements Guard.
var _ Guard = (*LockedResource)(nil)

// Compile-time check that *GCNamedMutex implements NamedLocker.
var _ NamedLocker = (*GCNamedMutex)(nil)

// Unlock releases the lock on this resource.
// Safe to call multiple times (subsequent calls are no-ops).
func (lr *LockedResource) Unlock() {
	if lr.unlock != nil {
		lr.unlock()
		lr.unlock = nil
	}
}

// GCNamedMutex provides garbage-collected named RW mutexes.
type GCNamedMutex struct {
	mutexes map[string]*gcMutex
	m       *sync.Mutex
}

type gcMutex struct {
	m sync.RWMutex
	c int
}

func NewGCNamedMutex() *GCNamedMutex {
	return &GCNamedMutex{
		mutexes: make(map[string]*gcMutex),
		m:       &sync.Mutex{},
	}
}

func (g *GCNamedMutex) Lock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		resourceMutex = &gcMutex{}
		g.mutexes[name] = resourceMutex
	}
	resourceMutex.c++
	g.m.Unlock()

	resourceMutex.m.Lock()
}

func (g *GCNamedMutex) Unlock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		g.m.Unlock()
		return
	}
	resourceMutex.c--
	if resourceMutex.c == 0 {
		delete(g.mutexes, name)
	}
	g.m.Unlock()

	resourceMutex.m.Unlock()
}

func (g *GCNamedMutex) RLock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		resourceMutex = &gcMutex{}
		g.mutexes[name] = resourceMutex
	}
	resourceMutex.c++
	g.m.Unlock()

	resourceMutex.m.RLock()
}

func (g *GCNamedMutex) RUnlock(name string) {
	g.m.Lock()
	resourceMutex, ok := g.mutexes[name]
	if !ok {
		g.m.Unlock()
		return
	}
	resourceMutex.c--
	if resourceMutex.c == 0 {
		delete(g.mutexes, name)
	}
	g.m.Unlock()

	resourceMutex.m.RUnlock()
}

// LockWithGuard acquires a write lock and returns a wrapper for convenient unlock handling.
// The caller is responsible for calling Unlock() - typically via defer.
// Usage:
//
//	locked := mutex.LockWithGuard("resourceName")
//	defer locked.Unlock()
func (g *GCNamedMutex) LockWithGuard(name string) Guard {
	g.Lock(name)
	return &LockedResource{
		name:   name,
		unlock: func() { g.Unlock(name) },
	}
}

// RLockWithGuard acquires a read lock and returns a wrapper for convenient unlock handling.
// The caller is responsible for calling Unlock() - typically via defer.
// Usage:
//
//	locked := mutex.RLockWithGuard("resourceName")
//	defer locked.Unlock()
func (g *GCNamedMutex) RLockWithGuard(name string) Guard {
	g.RLock(name)
	return &LockedResource{
		name:   name,
		unlock: func() { g.RUnlock(name) },
	}
}
