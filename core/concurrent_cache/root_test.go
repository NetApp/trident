package concurrent_cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLockCache tests that a Lock call with the LockCache subquery holds the root resource
// and blocks any other Lock call until it is released.
func TestLockCache(t *testing.T) {
	initCaches()

	// Goroutine 1: acquire Lock with LockCache() and hold it
	lockHeld := make(chan struct{})
	releaseRequested := make(chan struct{})
	var unlockFirst func()
	var firstErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, unlock, err := Lock(context.Background(), Query(LockCache()))
		firstErr = err
		unlockFirst = unlock
		close(lockHeld)
		<-releaseRequested
		if unlockFirst != nil {
			unlockFirst()
		}
	}()

	// Wait until the first Lock has been acquired
	<-lockHeld
	assert.NoError(t, firstErr, "first Lock(LockCache()) should succeed")

	// Goroutine 2: attempt Lock with another query; it must block while the first holds LockCache
	secondDone := make(chan struct{})
	var secondErr error
	go func() {
		_, _, secondErr = Lock(context.Background(), Query(ReadBackend("backend1"), ReadVolume("volume1")))
		close(secondDone)
	}()

	// Give the second goroutine a moment to reach the Lock call and block
	time.Sleep(50 * time.Millisecond)
	select {
	case <-secondDone:
		assert.FailNow(t, "second Lock completed while first still holds LockCache; LockCache should block other Lock calls")
	default:
		// Second Lock is still blocked, as expected
	}

	// Release the first lock; second Lock should now complete
	close(releaseRequested)
	wg.Wait()

	// Wait for second Lock to finish (with a timeout)
	select {
	case <-secondDone:
		assert.NoError(t, secondErr, "second Lock should succeed after first releases")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "second Lock did not complete after first released LockCache")
	}
}
