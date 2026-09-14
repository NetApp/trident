// Copyright 2026 NetApp, Inc. All Rights Reserved.

package distlock

import (
	"context"
	"errors"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	coordv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/util/retry"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
)

var (
	_ Locker = &LeaseLock{}

	ErrLockInvalidArgument   = errors.New("invalid argument for lock")
	ErrLockDeleteFailed      = errors.New("lock could not be deleted")
	ErrLockAcquireConflict   = errors.New("lock held by another host")
	ErrCriticalSectionFailed = errors.New("critical section failed")
)

// LeaseLock is a fail-fast, fencing-token lock backed by a Kubernetes Lease.
//
// Acquire semantics:
//   - Lease absent: create it with our hostID.
//   - Lease held by us: idempotent re-entry (no write); safe after a crash-and-restart.
//   - Anything else (another holder, unexpectedly free): ErrLockAcquireConflict immediately.
//     The caller propagates this as a retryable error; the CO retries the RPC.
//
// Release semantics:
//   - Success (fn returns nil): delete the Lease so the next operation starts clean.
//   - Failure (fn returns non-nil): leave the Lease with our holderIdentity set.
//     The Lease is a sticky fencing token - until cleared externally, preventing silent
//     re-acquisition after a partial or failed cryptsetup operation.
//
// LeaseLock instances are not safe for use between concurrent routines. Each instance must
// be used by a single goroutine for the duration of one WithLock call. Use NewLeaseLock to
// create a fresh instance per operation.
type LeaseLock struct {
	lockID    string
	hostID    string
	namespace string
	leaseUID  string
	client    coordv1client.LeaseInterface
}

type LeaseLockOpts func(*LeaseLock)

// NewLeaseLock constructs a Locker backed by a single named Kubernetes Lease.
// leaseClient is a real client-go typed client (clientset.CoordinationV1().Leases(namespace));
// namespace/lockID identify the Lease object; hostID is this process's holder identity.
func NewLeaseLock(
	client coordv1client.LeaseInterface,
	namespace, hostID, lockID string,
	opts ...LeaseLockOpts,
) *LeaseLock {
	leaseLock := &LeaseLock{
		lockID:    lockID,
		hostID:    hostID,
		namespace: namespace,
		client:    client,
	}

	for _, opt := range opts {
		opt(leaseLock)
	}
	return leaseLock
}

// WithLock executes fn under the LeaseLock.
//   - If another node holds the lease: returns ErrLockAcquireConflict immediately (no waiting).
//   - On success (fn returns nil): deletes the Lease.
//   - On failure (fn returns non-nil): leaves holderIdentity set (sticky lease).
//     The same host may re-enter on a subsequent call; other hosts get ErrLockAcquireConflict.
func (l *LeaseLock) WithLock(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrLockInvalidArgument)
	}
	if fn == nil {
		return fmt.Errorf("%w: nil critical section", ErrLockInvalidArgument)
	}

	if acquireErr := l.acquire(ctx); acquireErr != nil {
		return acquireErr
	}

	if fnErr := fn(ctx); fnErr != nil {
		Logc(ctx).WithError(fnErr).Warnf(
			"Failed to execute critical section while holding lease: %s uid: %s. "+
				"Retaining lease ownership for subsequent retries on host %s. "+
				"If the current holder cannot make progress, manual device "+
				"inspection and Lease removal may be required.",
			l.lockID, l.leaseUID, l.hostID,
		)
		return fmt.Errorf("%w: %w", ErrCriticalSectionFailed, fnErr)
	}
	return l.release(ctx)
}

func (l *LeaseLock) acquire(ctx context.Context) error {
	if ctx == nil {
		return errors.New("supplied context cannot be nil")
	}

	holderID := l.hostID
	acquireTime := metav1.NowMicro()
	leaseConfig := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      l.lockID,
			Namespace: l.namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: new(holderID),
			AcquireTime:    new(acquireTime),
		},
	}

	// Create is the claim: the API server's uniqueness constraint means exactly one
	// caller can create the Lease, so a successful Create is an exclusive acquire.
	lease, createErr := l.client.Create(ctx, leaseConfig, metav1.CreateOptions{})
	if createErr == nil {
		l.leaseUID = string(lease.UID)
		return nil
	}
	if !apierrors.IsAlreadyExists(createErr) {
		return fmt.Errorf("failed to create lease %s for host %s: %w", l.lockID, l.hostID, createErr)
	}

	// AlreadyExists means the Lease exists, so the only remaining question is whether
	// the existing holder is us (re-entry) or another host (conflict).
	var getErr error
	lease, getErr = l.client.Get(ctx, l.lockID, metav1.GetOptions{})
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			// Released between our Create conflict and this Get; another host was
			// holding it moments ago. Report a conflict so the caller retries cleanly.
			return fmt.Errorf("%w: lease released by other host", ErrLockAcquireConflict)
		}
		return fmt.Errorf("failed to inspect existing lease %s: %w", l.lockID, getErr)
	}

	if holder := convert.ToVal(lease.Spec.HolderIdentity); holder != l.hostID {
		return fmt.Errorf("%w: holder=%q", ErrLockAcquireConflict, holder)
	}
	l.leaseUID = string(lease.UID)

	return nil
}

func (l *LeaseLock) release(ctx context.Context) error {
	backoff := wait.Backoff{
		Steps:    3,
		Duration: 200 * time.Millisecond,
		Factor:   2.0,
	}
	predicate := func(err error) bool {
		if apierrors.IsNotFound(err) {
			return false // lease already gone — treat as success, stop retrying
		}
		Logc(ctx).WithError(err).Debugf("Failed to delete lease %s; retrying.", l.lockID)
		return true
	}
	fn := func() error {
		err := l.client.Delete(ctx, l.lockID, metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{
				UID: new(types.UID(l.leaseUID)),
			},
		})
		if apierrors.IsConflict(err) {
			return nil // UID mismatch between the leases - our lease was stale; consider this success.
		}
		return err
	}

	if deleteErr := retry.OnError(backoff, predicate, fn); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
		Logc(ctx).WithFields(LogFields{
			"leaseName": l.lockID,
			"holderID":  l.hostID,
			"leaseUID":  l.leaseUID,
		}).WithError(deleteErr).Error("Could not delete lease.")
		return fmt.Errorf("%w: lease=%s host=%s: %v", ErrLockDeleteFailed, l.lockID, l.hostID, deleteErr)
	}

	return nil
}
