// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	. "github.com/netapp/trident/logging"
)

const (
	testNamespace = "trident"
	testLockID    = "pvc-abc123"
	testHostA     = "node-a"
	testHostB     = "node-b"
)

func TestMain(m *testing.M) {
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

// newLock returns a LeaseLock wired to a fresh fake clientset.
func newLock(t *testing.T, hostID string) (*LeaseLock, *fake.Clientset) {
	t.Helper()
	cs := fake.NewSimpleClientset()
	client := cs.CoordinationV1().Leases(testNamespace)
	lock := NewLeaseLock(client, testNamespace, hostID, testLockID)
	return lock, cs
}

// seedLease pre-populates the fake store with a Lease whose holder is set to holderID.
// Pass "" to seed a holder-less Lease.
func seedLease(t *testing.T, cs *fake.Clientset, holderID string) {
	t.Helper()
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLockID,
			Namespace: testNamespace,
		},
	}
	if holderID != "" {
		lease.Spec.HolderIdentity = &holderID
	}
	_, err := cs.CoordinationV1().Leases(testNamespace).Create(
		context.Background(), lease, metav1.CreateOptions{},
	)
	require.NoError(t, err)
}

// injectGetError prepends a reactor that returns err on all GET lease calls.
func injectGetError(cs *fake.Clientset, err error) {
	cs.Fake.PrependReactor("get", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

// injectCreateError prepends a reactor that returns err on all CREATE lease calls.
func injectCreateError(cs *fake.Clientset, err error) {
	cs.Fake.PrependReactor("create", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

// injectDeleteError prepends a reactor that returns err on all DELETE lease calls.
func injectDeleteError(cs *fake.Clientset, err error) {
	cs.Fake.PrependReactor("delete", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

func TestWithLock_NilFn(t *testing.T) {
	lock, _ := newLock(t, testHostA)
	err := lock.WithLock(context.Background(), nil)
	assert.ErrorIs(t, err, ErrLockInvalidArgument)
}

func TestWithLock_HappyPath(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	called := false

	err := lock.WithLock(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "critical section should have been called")

	// Lease should be deleted after success.
	leases, listErr := cs.CoordinationV1().Leases(testNamespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, listErr)
	assert.Empty(t, leases.Items, "lease should be deleted after successful WithLock")
}

func TestWithLock_CriticalSectionError_LeaseRetained(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	fnErr := errors.New("something went wrong in the critical section")

	err := lock.WithLock(context.Background(), func(ctx context.Context) error {
		return fnErr
	})

	assert.ErrorIs(t, err, fnErr)

	// Lease must still exist — sticky fence.
	leases, listErr := cs.CoordinationV1().Leases(testNamespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, listErr)
	require.Len(t, leases.Items, 1, "lease should be retained after critical section failure")
	assert.Equal(t, testHostA, *leases.Items[0].Spec.HolderIdentity)
}

func TestAcquire_LeaseNotFound_Creates(t *testing.T) {
	lock, cs := newLock(t, testHostA)

	err := lock.acquire(context.Background())
	require.NoError(t, err)

	leases, _ := cs.CoordinationV1().Leases(testNamespace).List(context.Background(), metav1.ListOptions{})
	require.Len(t, leases.Items, 1)
	assert.Equal(t, testHostA, *leases.Items[0].Spec.HolderIdentity)
}

func TestAcquire_CreateConflict_ErrLockConflict(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	// Inject AlreadyExists on Create to simulate a race where another node created first.
	injectCreateError(cs, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "leases"}, testLockID))

	err := lock.acquire(context.Background())
	assert.ErrorIs(t, err, ErrLockAcquireConflict)
}

func TestAcquire_CreateAPIError(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	injectCreateError(cs, fmt.Errorf("api server unavailable"))

	err := lock.acquire(context.Background())
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrLockAcquireConflict)
}

func TestAcquire_AlreadyHolder_NoUpdate(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	seedLease(t, cs, testHostA)

	err := lock.acquire(context.Background())
	require.NoError(t, err)

	// Create-first: we attempt a Create (conflict), then Get to confirm we're the holder.
	// The important invariant is that no Update is issued — the lease is never overwritten.
	newActions := cs.Fake.Actions()[len(cs.Fake.Actions())-2:]
	verbs := make([]string, 0, len(newActions))
	for _, a := range newActions {
		verbs = append(verbs, a.GetVerb())
		assert.NotEqual(t, "update", a.GetVerb(), "re-entry must not update the existing lease")
	}
	assert.Contains(t, verbs, "create", "expected a create attempt")
	assert.Contains(t, verbs, "get", "expected a get to inspect the holder")
}

func TestAcquire_AnotherHolder_ErrLockConflict(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	seedLease(t, cs, testHostB) // testHostB holds the lease.

	err := lock.acquire(context.Background())
	assert.ErrorIs(t, err, ErrLockAcquireConflict)
}

func TestAcquire_FreeHolder_ErrLockConflict(t *testing.T) {
	lock, cs := newLock(t, testHostA)
	seedLease(t, cs, "") // Holder-less lease — should never happen but must be treated as conflict.

	err := lock.acquire(context.Background())
	assert.ErrorIs(t, err, ErrLockAcquireConflict)
}

func TestAcquire_GetAPIError(t *testing.T) {
	// Create-first: Get is only called when Create returns AlreadyExists.
	// Seed a lease so Create conflicts, then inject a Get error to simulate
	// an etcd timeout during holder inspection.
	lock, cs := newLock(t, testHostA)
	seedLease(t, cs, testHostB) // ensures Create → AlreadyExists → triggers Get
	injectGetError(cs, fmt.Errorf("etcd timeout"))

	err := lock.acquire(context.Background())
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrLockAcquireConflict)
}

func TestAcquire_NilContext(t *testing.T) {
	lock, _ := newLock(t, testHostA)
	//nolint:staticcheck
	err := lock.acquire(nil)
	assert.Error(t, err)
}

func TestAcquire_ConflictThenSuccess(t *testing.T) {
	// If acquire fails due to conflict, a subsequent WithLock call on the same
	// instance must still be able to proceed once the conflicting lease is removed.
	lock, cs := newLock(t, testHostA)
	seedLease(t, cs, testHostB)

	// First attempt — blocked by testHostB's lease.
	err := lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	assert.ErrorIs(t, err, ErrLockAcquireConflict)

	// Remove the conflicting lease so the retry can succeed.
	_ = cs.CoordinationV1().Leases(testNamespace).Delete(
		context.Background(), testLockID, metav1.DeleteOptions{},
	)

	// Second attempt — must succeed now that the lease is free.
	err = lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	assert.NoError(t, err, "WithLock must succeed once the conflicting lease is removed")
}

func TestRelease_DeleteUsesUIDPrecondition(t *testing.T) {
	// release() must delete by UID so it cannot accidentally remove a replacement lease
	// created after operator recovery.
	lock, cs := newLock(t, testHostA)

	// Inject a known UID into the Create response so lock.leaseUID is non-empty.
	const wantUID = types.UID("test-uid-abc123")
	cs.Fake.PrependReactor("create", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		obj := action.(k8stesting.CreateAction).GetObject().(*coordinationv1.Lease)
		obj.UID = wantUID
		return true, obj, nil
	})

	var capturedUID string
	cs.Fake.PrependReactor("delete", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		da := action.(k8stesting.DeleteAction)
		if p := da.GetDeleteOptions().Preconditions; p != nil && p.UID != nil {
			capturedUID = string(*p.UID)
		}
		return false, nil, nil // let the default reactor complete the delete
	})

	err := lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	require.NoError(t, err)

	assert.Equal(t, string(wantUID), capturedUID, "Delete must carry a UID precondition matching the acquired lease UID")
}

func TestRelease_DeleteConflict_IsOK(t *testing.T) {
	// A 409 Conflict on Delete means the lease was replaced (UID mismatch after operator
	// recovery). Our lease is already gone; this must be treated as success, not an error.
	lock, cs := newLock(t, testHostA)
	cs.Fake.PrependReactor("delete", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "leases"}, testLockID, fmt.Errorf("uid mismatch"))
	})

	err := lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	assert.NoError(t, err, "409 Conflict on delete must be treated as success (UID mismatch = lease replaced)")
}

func TestRelease_DeleteError_ReturnsError(t *testing.T) {
	// WithLock should surface a Delete failure so the caller knows the lease lingers.
	lock, cs := newLock(t, testHostA)
	injectDeleteError(cs, fmt.Errorf("api server unavailable"))

	err := lock.WithLock(context.Background(), func(ctx context.Context) error {
		return nil // critical section succeeds; only the Delete fails.
	})
	assert.Error(t, err, "delete failure should surface as an error from WithLock")
}

func TestRelease_DeleteNotFound_IsOK(t *testing.T) {
	// If the Lease is externally deleted between acquire and release
	// (e.g. operator intervention), the 404 on Delete must not surface as an error.
	lock, cs := newLock(t, testHostA)
	cs.Fake.PrependReactor("delete", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "leases"}, testLockID)
	})

	err := lock.WithLock(context.Background(), func(ctx context.Context) error { return nil })
	assert.NoError(t, err, "404 on delete should be treated as a no-op, not an error")
}

func TestWithLock_ReEntry_StickyLease(t *testing.T) {
	// Simulate: first call fails, leaving a sticky lease.
	// Second call from the same host must succeed (re-entry).
	lock, _ := newLock(t, testHostA)
	fnErr := errors.New("transient failure")

	// First call — critical section fails.
	_ = lock.WithLock(context.Background(), func(ctx context.Context) error {
		return fnErr
	})

	// Second call — same host should re-enter without ErrLockConflict.
	err := lock.WithLock(context.Background(), func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err, "same host must be able to re-enter after a sticky lease")
}

func TestWithLock_CrossNode_StickyLeaseBlocks(t *testing.T) {
	// After host A fails and leaves a sticky lease, host B must see ErrLockConflict.
	lockA, cs := newLock(t, testHostA)
	fnErr := errors.New("transient failure")

	// Host A leaves a sticky lease.
	_ = lockA.WithLock(context.Background(), func(ctx context.Context) error {
		return fnErr
	})

	// Host B attempts to acquire — must be blocked.
	lockB := NewLeaseLock(cs.CoordinationV1().Leases(testNamespace), testNamespace, testHostB, testLockID)
	err := lockB.WithLock(context.Background(), func(ctx context.Context) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrLockAcquireConflict, "host B must not acquire while host A holds the sticky lease")
}
