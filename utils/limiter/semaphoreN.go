package limiter

import (
	"context"
	"errors"
	"fmt"

	. "github.com/netapp/trident/logging"
)

const defaultBufferSize = 10

type SemaphoreN struct {
	id     string
	tokens chan struct{}
}

// Creates a default semaphore, use options to customize it.
func newSemaphoreN(id string) *SemaphoreN {
	return &SemaphoreN{
		id:     id,
		tokens: make(chan struct{}, defaultBufferSize),
	}
}

func (s *SemaphoreN) Wait(ctx context.Context) error {
	Logc(ctx).WithField("ID", s.id).Debug(">>>> SemaphoreN.Wait")
	defer Logc(ctx).WithField("ID", s.id).Debug("<<<< SemaphoreN.Wait")

	select {
	case s.tokens <- struct{}{}:
		Logc(ctx).WithField("ID", s.id).Debug("SemaphoreN lock acquired successfully.")
		return nil
	case <-ctx.Done():
		return errors.New("context has been cancelled")
	}
}

// Release should always be called after Wait, otherwise it may lead to deadlock.
func (s *SemaphoreN) Release(ctx context.Context) {
	Logc(ctx).WithField("ID", s.id).Debug(">>>> SemaphoreN.Release")
	defer Logc(ctx).WithField("ID", s.id).Debug("<<<< SemaphoreN.Release")

	select {
	case <-s.tokens:
		Logc(ctx).WithField("ID", s.id).Debug("SemaphoreN lock released successfully.")
		return
	default:
		// Just to be on the safe side if release is called before wait.
		Logc(ctx).WithField("ID", s.id).Warn("Release() was called before wait().")
		return
	}
}

func WithSemaphoreNSize(ctx context.Context, bufferSize int) LimiterOption {
	return func(lim Limiter) error {
		s, ok := lim.(*SemaphoreN)
		if !ok {
			return fmt.Errorf("wrong limter type passed: %T, WithSemaphoreNSize option is intended for SemaphoreN", lim)
		}
		s.tokens = make(chan struct{}, bufferSize)
		Logc(ctx).WithField("ID", s.id).Debug("WithSemaphoreNSize LimiterOption was successfully applied.")
		return nil
	}
}
