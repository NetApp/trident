// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// Shared exponential backoff defaults for ONTAP read-after-write waits (see newOntapBackOff).
const (
	waitForOntapMultiplier    = 2
	waitForOntapRandomization = 0.2
)

var (
	// ONTAP read-after-write retry tuning (overridden in unit tests for speed).
	waitForOntapInitialInterval = 100 * time.Millisecond
	waitForOntapMaxInterval     = 2 * time.Second
	waitForOntapMaxElapsed      = 30 * time.Second

	// waitForVolumeDeleteMaxElapsed bounds how long the ONTAP layer works on removing one volume: retrying a
	// destroy ONTAP rejects as busy, and waiting for an accepted destroy to take effect. A Flexvol delete is
	// asynchronous and takes far longer than a read-after-write lookup, so it gets its own budget; it matches
	// the window OntapAPIREST.VolumeDestroy already allows itself to retry a busy volume.
	waitForVolumeDeleteMaxElapsed = 60 * time.Second
)

// ConfigureWaitForOntapBackoffForTests shortens read-after-write retry timing for unit tests.
// Call from TestMain in this package and in packages that exercise WaitFor* helpers.
func ConfigureWaitForOntapBackoffForTests() {
	waitForOntapInitialInterval = 10 * time.Millisecond
	waitForOntapMaxInterval = 50 * time.Millisecond
	waitForOntapMaxElapsed = 30 * time.Second
	// Test-only delete-wait budget: with the 10ms/50ms backoff above, 500ms still allows several
	// retry cycles while keeping timeout-path unit tests from waiting the production 60s window.
	waitForVolumeDeleteMaxElapsed = 500 * time.Millisecond
}

// LunGetter is the minimal surface needed for WaitForLunToExist. OntapAPI satisfies it.
type LunGetter interface {
	LunGetByName(ctx context.Context, name string) (*Lun, error)
}

// NVMeNamespaceGetter is the minimal surface needed for WaitForNVMeNamespaceToExist.
type NVMeNamespaceGetter interface {
	NVMeNamespaceGetByName(context.Context, string) (*NVMeNamespace, error)
}

// NVMeNamespaceSizeGetter is the minimal surface needed for WaitForNVMeNamespaceSize.
type NVMeNamespaceSizeGetter interface {
	NVMeNamespaceGetSize(context.Context, string) (int, error)
}

// VolumeExistenceChecker is the minimal surface needed for WaitForVolumeToBeDeleted. OntapAPI satisfies it.
type VolumeExistenceChecker interface {
	VolumeExists(ctx context.Context, volumeName string) (bool, error)
}

// FlexgroupExistenceChecker is the minimal surface needed for WaitForFlexgroupToBeDeleted.
type FlexgroupExistenceChecker interface {
	FlexgroupExists(ctx context.Context, volumeName string) (bool, error)
}

// newOntapBackOff returns exponential backoff using the waitForOntap* vars, with
// MaxElapsedTime shortened when ctx carries a sooner deadline.
func newOntapBackOff(ctx context.Context) *backoff.ExponentialBackOff {
	return newOntapBackOffWithBudget(ctx, waitForOntapMaxElapsed)
}

// newOntapBackOffWithBudget returns exponential backoff using the waitForOntap* intervals and the given
// overall budget, shortened when ctx carries a sooner deadline.
func newOntapBackOffWithBudget(ctx context.Context, maxElapsed time.Duration) *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = waitForOntapInitialInterval
	bo.MaxInterval = waitForOntapMaxInterval
	bo.Multiplier = waitForOntapMultiplier
	bo.RandomizationFactor = waitForOntapRandomization
	bo.MaxElapsedTime = maxElapsed
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < bo.MaxElapsedTime {
			bo.MaxElapsedTime = remaining
		}
	}
	return bo
}

// errWaitInterrupted formats errors when a WaitFor* call stops because ctx was cancelled,
// timed out, or newOntapBackOff exhausted its budget. RetryNotify unwraps PermanentError, so
// non-NotFound failures are returned as-is; only NotFound after budget exhaustion is wrapped.
func errWaitInterrupted(ctx context.Context, resourceDesc, path string, retryErr error) error {
	if stderrors.Is(retryErr, context.Canceled) || stderrors.Is(retryErr, context.DeadlineExceeded) {
		return fmt.Errorf("waiting for %s %s interrupted: %w", resourceDesc, path, retryErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("waiting for %s %s interrupted: %w", resourceDesc, path, ctxErr)
	}
	if !errors.IsNotFoundError(retryErr) {
		return retryErr
	}
	return fmt.Errorf("timed out waiting for %s %s: %w", resourceDesc, path, retryErr)
}

// WaitForLunToExist calls LunGetByName until the LUN at lunPath exists, ctx is done, or
// newOntapBackOff exhausts its budget. Only errors.IsNotFoundError results are retried (e.g. ONTAP
// REST read-after-write where the LUN collection is briefly empty right after create). Other errors
// fail immediately.
func WaitForLunToExist(ctx context.Context, o LunGetter, lunPath string) (*Lun, error) {
	var found *Lun
	operation := func() error {
		lun, err := o.LunGetByName(ctx, lunPath)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return err
			}
			return backoff.Permanent(err)
		}
		if lun == nil {
			// Do not retry: a missing result without NotFoundError is unexpected API behavior.
			// Use a non-NotFound error so errWaitInterrupted does not report this as a timeout.
			return backoff.Permanent(fmt.Errorf("unexpected empty result looking up LUN %s", lunPath))
		}
		found = lun
		return nil
	}
	notify := func(err error, d time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"lunPath":   lunPath,
			"increment": d,
		}).Debug("LUN not visible yet, retrying.")
	}
	bo := newOntapBackOff(ctx)
	if err := backoff.RetryNotify(operation, backoff.WithContext(bo, ctx), notify); err != nil {
		return nil, errWaitInterrupted(ctx, "LUN", lunPath, err)
	}
	return found, nil
}

// WaitForNVMeNamespaceToExist calls NVMeNamespaceGetByName until the namespace at nsPath exists,
// ctx is done, or newOntapBackOff exhausts its budget. Only NotFound errors are retried. When
// retryOnEmptyResult is true, a nil namespace with no error is retried (post-create eventual consistency).
func WaitForNVMeNamespaceToExist(
	ctx context.Context,
	o NVMeNamespaceGetter,
	nsPath string,
	retryOnEmptyResult bool,
) (*NVMeNamespace, error) {
	var found *NVMeNamespace
	operation := func() error {
		ns, err := o.NVMeNamespaceGetByName(ctx, nsPath)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return err
			}
			return backoff.Permanent(err)
		}
		if ns == nil {
			if retryOnEmptyResult {
				return errors.NotFoundError("namespace %s not found", nsPath)
			}
			// Use a non-NotFound error so errWaitInterrupted does not report this as a timeout.
			return backoff.Permanent(fmt.Errorf("unexpected empty result looking up NVMe namespace %s", nsPath))
		}
		found = ns
		return nil
	}
	notify := func(err error, d time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"namespace": nsPath,
			"increment": d,
		}).Trace("Namespace not yet visible, retrying.")
	}
	bo := newOntapBackOff(ctx)
	if err := backoff.RetryNotify(operation, backoff.WithContext(bo, ctx), notify); err != nil {
		return nil, errWaitInterrupted(ctx, "NVMe namespace", nsPath, err)
	}
	return found, nil
}

// WaitForVolumeToBeDeleted calls VolumeExists until the volume is gone, ctx is done, or the delete budget
// is exhausted. ONTAP removes a Flexvol asynchronously, so a VolumeDestroy that returned successfully only
// means the delete was accepted; callers that intend to recreate the volume have to know it is really gone.
//
// A NotFound error counts as gone. Any other read failure is retried rather than treated as absence: while a
// volume is being deleted, reads of it are exactly what fails intermittently, and reporting a volume as
// deleted when the read failed would let a caller recreate a name ONTAP still holds.
func WaitForVolumeToBeDeleted(ctx context.Context, o VolumeExistenceChecker, volumeName string) error {
	return waitForVolumeToBeDeleted(ctx, volumeName, o.VolumeExists)
}

// WaitForFlexgroupToBeDeleted calls FlexgroupExists until the FlexGroup is gone, ctx is done, or the delete
// budget is exhausted. REST volume existence checks are style-specific, so FlexGroups must not use the
// Flexvol-only VolumeExists path.
func WaitForFlexgroupToBeDeleted(ctx context.Context, o FlexgroupExistenceChecker, volumeName string) error {
	return waitForVolumeToBeDeleted(ctx, volumeName, o.FlexgroupExists)
}

func waitForVolumeToBeDeleted(
	ctx context.Context, volumeName string, volumeExists func(context.Context, string) (bool, error),
) error {
	operation := func() error {
		exists, err := volumeExists(ctx, volumeName)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return nil
			}
			return err
		}
		if exists {
			return errors.FoundError("volume %s still exists", volumeName)
		}
		return nil
	}
	notify := func(err error, d time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"volume":    volumeName,
			"increment": d,
			"error":     err,
		}).Debug("Volume not deleted yet, retrying.")
	}
	bo := newOntapBackOffWithBudget(ctx, waitForVolumeDeleteMaxElapsed)
	if err := backoff.RetryNotify(operation, backoff.WithContext(bo, ctx), notify); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("waiting for volume %s to be deleted interrupted: %w", volumeName, ctxErr)
		}
		return fmt.Errorf("timed out waiting for volume %s to be deleted: %w", volumeName, err)
	}
	return nil
}

// WaitForNVMeNamespaceSize calls NVMeNamespaceGetSize until a size is returned, ctx is done, or
// newOntapBackOff exhausts its budget. Only NotFound errors are retried.
func WaitForNVMeNamespaceSize(ctx context.Context, o NVMeNamespaceSizeGetter, nsPath string) (int, error) {
	var size int
	operation := func() error {
		got, err := o.NVMeNamespaceGetSize(ctx, nsPath)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return err
			}
			return backoff.Permanent(err)
		}
		size = got
		return nil
	}
	notify := func(err error, d time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"namespace": nsPath,
			"increment": d,
		}).Trace("Namespace size not yet visible, retrying.")
	}
	bo := newOntapBackOff(ctx)
	if err := backoff.RetryNotify(operation, backoff.WithContext(bo, ctx), notify); err != nil {
		return 0, errWaitInterrupted(ctx, "NVMe namespace size", nsPath, err)
	}
	return size, nil
}
