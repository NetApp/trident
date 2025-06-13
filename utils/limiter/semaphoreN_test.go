package limiter

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_limiter"
)

func TestSemaphoreN_Wait(t *testing.T) {
	limID := "tempLimiter"
	numOfGoroutines := 2

	// Test 1 (Negative Test): Expect an error when a goroutine attempts to call Wait()
	//          after all expected goroutines have successfully acquired Wait().
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	lim, _ := New(ctx, limID, TypeSemaphoreN, WithSemaphoreNSize(ctx, numOfGoroutines))

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
			inward <- struct{}{}
			<-outward
		}()
	}

	for i := 0; i < numOfGoroutines; i++ {
		<-inward
	}
	err := lim.Wait(ctx)
	assert.Error(t, err)

	for i := 0; i < numOfGoroutines; i++ {
		outward <- struct{}{}
	}
	wg.Wait()

	// Test 2 (Positive Test): Successfully acquiring the expected number of wait()
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
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
