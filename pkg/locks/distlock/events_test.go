// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type recordedEvent struct {
	lease     *coordinationv1.Lease
	eventType string
	reason    string
	message   string
}

type fakeEventRecorder struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (f *fakeEventRecorder) Event(_ context.Context, lease *coordinationv1.Lease, eventType, reason, message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, recordedEvent{
		lease:     lease,
		eventType: eventType,
		reason:    reason,
		message:   message,
	})
}

func (f *fakeEventRecorder) reasons() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.events))
	for _, e := range f.events {
		out = append(out, e.reason)
	}
	return out
}

func (f *fakeEventRecorder) byReason(reason string) []recordedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedEvent
	for _, e := range f.events {
		if e.reason == reason {
			out = append(out, e)
		}
	}
	return out
}

func newLockWithRecorder(t *testing.T, hostID string, recorder *fakeEventRecorder) (*LeaseLock, *fake.Clientset) {
	t.Helper()
	cs := fake.NewSimpleClientset()
	client := cs.CoordinationV1().Leases(testNamespace)
	lock := NewLeaseLock(
		client, testNamespace, hostID, testLockID,
		WithEventRecorder(recorder),
	)
	return lock, cs
}

func TestWithLock_Events_HappyPath_Silent(t *testing.T) {
	recorder := &fakeEventRecorder{}
	lock, _ := newLockWithRecorder(t, testHostA, recorder)

	err := lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	require.NoError(t, err)
	assert.Empty(t, recorder.reasons())
}

func TestWithLock_Events_CriticalSectionFailure_Retains(t *testing.T) {
	recorder := &fakeEventRecorder{}
	lock, _ := newLockWithRecorder(t, testHostA, recorder)

	err := lock.WithLock(context.Background(), func(ctx context.Context) error {
		return errors.New("cryptsetup failed")
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCriticalSectionFailed))
	assert.Equal(t, []string{ReasonLeaseRetained}, recorder.reasons())

	retained := recorder.byReason(ReasonLeaseRetained)
	require.Len(t, retained, 1)
	assert.Equal(t, EventTypeWarning, retained[0].eventType)
	assert.Contains(t, retained[0].message, "cryptsetup failed")
	assert.Equal(t, testLockID, retained[0].lease.Name)
	assert.Equal(t, testNamespace, retained[0].lease.Namespace)
	assert.Equal(t, "Lease", retained[0].lease.Kind)
}

func TestWithLock_Events_Conflict_Silent(t *testing.T) {
	recorder := &fakeEventRecorder{}
	lockA, cs := newLockWithRecorder(t, testHostA, recorder)
	require.Error(t, lockA.WithLock(context.Background(), func(ctx context.Context) error {
		return errors.New("sticky")
	}))

	lockB := NewLeaseLock(
		cs.CoordinationV1().Leases(testNamespace), testNamespace, testHostB, testLockID,
		WithEventRecorder(recorder),
	)
	err := lockB.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockAcquireConflict))
	// Only the retain from lockA; conflict itself is silent.
	assert.Equal(t, []string{ReasonLeaseRetained}, recorder.reasons())
}

func TestWithLock_Events_ReleaseFailed(t *testing.T) {
	recorder := &fakeEventRecorder{}
	lock, cs := newLockWithRecorder(t, testHostA, recorder)
	injectDeleteError(cs, fmt.Errorf("delete boom"))

	err := lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockDeleteFailed))
	assert.Equal(t, []string{ReasonLeaseReleaseFailed}, recorder.reasons())

	failed := recorder.byReason(ReasonLeaseReleaseFailed)
	require.Len(t, failed, 1)
	assert.Equal(t, EventTypeWarning, failed[0].eventType)
	assert.Contains(t, failed[0].message, "delete boom")
}

func TestWithLock_Events_NilRecorder_NoPanic(t *testing.T) {
	cs := fake.NewSimpleClientset()
	lock := NewLeaseLock(cs.CoordinationV1().Leases(testNamespace), testNamespace, testHostA, testLockID)
	assert.NotPanics(t, func() {
		err := lock.WithLock(context.Background(), func(ctx context.Context) error {
			return errors.New("boom")
		})
		require.Error(t, err)
	})
}
