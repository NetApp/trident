package limiter

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_limiter"
)

func TestSemaphoreN_Wait(t *testing.T) {
	limID := "tempLimiter"
	numOfGoroutines := 2

	// Test 1 (negative): a Wait on a full semaphore must block until its context is
	// cancelled, then return an error — not succeed and not fail instantly.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	lim, _ := New(ctx, limID, TypeSemaphoreN, WithSemaphoreNSize(ctx, numOfGoroutines))

	// outward/inward let the test know both slots are held before we call Wait again.
	// Holders park on <-outward so they keep their tokens until we explicitly release.
	outward := make(chan struct{}, numOfGoroutines)
	inward := make(chan struct{}, numOfGoroutines)

	var wg sync.WaitGroup
	wg.Add(numOfGoroutines)

	for i := 0; i < numOfGoroutines; i++ {
		go func() {
			defer wg.Done()
			err := lim.Wait(ctx)
			defer lim.Release(ctx)
			assert.NoError(t, err)
			inward <- struct{}{} // signal: this goroutine holds a slot
			<-outward            // block until test releases holders
		}()
	}

	for i := 0; i < numOfGoroutines; i++ {
		<-inward // wait until semaphore size (2) is fully acquired
	}

	// negCtx bounds the whole negative case so a stuck Wait fails in seconds, not at CI timeout.
	const negativeCaseTimeout = 5 * time.Second
	negCtx, negCancel := context.WithTimeout(context.Background(), negativeCaseTimeout)
	defer negCancel()

	// waitCtx has no deadline of its own; the test cancels it once blocking is verified.
	waitCtx, waitCancel := context.WithCancel(negCtx)
	defer waitCancel()

	// Third caller: should block on the full semaphore until waitCtx is cancelled.
	waitDone := make(chan error, 1)
	var waiterRunning atomic.Bool
	go func() {
		waiterRunning.Store(true)
		waitDone <- lim.Wait(waitCtx)
	}()

	// Ensure the waiter goroutine has started before we inspect waitDone.
	for !waiterRunning.Load() {
		select {
		case <-negCtx.Done():
			t.Fatal("timed out waiting for waiter goroutine to start")
		default:
			runtime.Gosched()
		}
	}

	// Non-blocking check: if Wait already returned, the semaphore did not block as expected.
	select {
	case err := <-waitDone:
		t.Fatalf("Wait returned before cancel while semaphore full: %v", err)
	case <-negCtx.Done():
		t.Fatal("timed out before cancel while waiting for Wait to block")
	default:
	}

	waitCancel() // unblock Wait via ctx.Done(); should not acquire a token

	select {
	case err := <-waitDone:
		assert.Error(t, err)
	case <-negCtx.Done():
		t.Fatal("Wait did not return after context cancel")
	}

	// Release holders so Test 2 can reuse the same limiter.
	for i := 0; i < numOfGoroutines; i++ {
		outward <- struct{}{}
	}
	wg.Wait()

	// Test 2 (positive): after releases, the expected number of Wait calls succeed.
	for i := 0; i < numOfGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := lim.Wait(ctx)
			defer lim.Release(ctx)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestSemaphoreN_Release(t *testing.T) {
	limID := "tempLimiter"
	numOfGoroutines := 2

	// Test 1 (Positive Test): Successfully releasing the acquired wait.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	lim, _ := New(ctx, limID, TypeSemaphoreN, WithSemaphoreNSize(ctx, numOfGoroutines))

	var wg sync.WaitGroup
	for i := 0; i < numOfGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := lim.Wait(ctx)
			defer lim.Release(ctx)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// Test 2 (Negative Test): Trying release before wait
	lim.Release(ctx)
}

func TestWithSemaphoreNSize(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockLimiter := mock_limiter.NewMockLimiter(ctrl)
	limID := "tempLimiter"

	tests := []struct {
		name        string
		limiter     Limiter
		bufferSize  int
		expectError bool
	}{
		{
			name:        "Valid SemaphoreN",
			limiter:     newSemaphoreN(limID),
			bufferSize:  20,
			expectError: false,
		},
		{
			name:        "Invalid Limiter Type",
			limiter:     mockLimiter,
			bufferSize:  20,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			option := WithSemaphoreNSize(ctx(), test.bufferSize)
			err := option(test.limiter)
			if test.expectError {
				assert.Error(t, err)
				assert.Equal(t, fmt.Sprintf("wrong limter type passed: %T, WithSemaphoreNSize option is intended for SemaphoreN", test.limiter), err.Error())
			} else {
				assert.NoError(t, err)
				s, ok := test.limiter.(*SemaphoreN)
				assert.True(t, ok)
				assert.Equal(t, test.bufferSize, cap(s.tokens))
			}
		})
	}
}
