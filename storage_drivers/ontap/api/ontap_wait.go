// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
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
)

// ConfigureWaitForOntapBackoffForTests shortens read-after-write retry timing for unit tests.
// Call from TestMain in packages that exercise WaitFor* helpers through production code.
func ConfigureWaitForOntapBackoffForTests() {
	waitForOntapInitialInterval = 10 * time.Millisecond
	waitForOntapMaxInterval = 50 * time.Millisecond
	waitForOntapMaxElapsed = 30 * time.Second
}

// VolumeInfoGetter reads a volume by name.
type VolumeInfoGetter func(context.Context, string) (*Volume, error)

// WaitForJunctionPath retries a volume lookup until ONTAP reports a usable junction path.
// If expectedPath is empty, any non-empty junction path is accepted. Retry budget matches
// WaitForNVMeNamespaceToExist via newOntapBackOff (~30s unless ctx expires sooner).
func WaitForJunctionPath(
	ctx context.Context, getter VolumeInfoGetter, volumeName, expectedPath string,
) (*Volume, error) {
	var volume *Volume

	checkJunctionPath := func() error {
		var err error
		volume, err = getter(ctx, volumeName)
		if err != nil {
			getErr := fmt.Errorf("get volume %s while waiting for junction path: %w", volumeName, err)
			if errors.IsNotFoundError(err) ||
				stderrors.Is(err, context.Canceled) ||
				stderrors.Is(err, context.DeadlineExceeded) {
				return backoff.Permanent(getErr)
			}
			return getErr
		}
		if volume == nil {
			return backoff.Permanent(errors.NotFoundError("volume %s was not found", volumeName))
		}
		junctionPath := strings.Trim(volume.JunctionPath, "/")
		if junctionPath == "" {
			return fmt.Errorf("volume %s has an empty junction path", volumeName)
		}
		if expectedPath != "" && junctionPath != strings.Trim(expectedPath, "/") {
			return fmt.Errorf(
				"volume %s has junction path %q, expected %q", volumeName, volume.JunctionPath, expectedPath,
			)
		}
		return nil
	}
	notify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"volume":    volumeName,
			"increment": duration,
		}).WithError(err).Debug("Junction path is not ready, retrying.")
	}

	bo := newOntapBackOff(ctx)
	if err := backoff.RetryNotify(checkJunctionPath, backoff.WithContext(bo, ctx), notify); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("waiting for volume %s junction path interrupted: %w", volumeName, ctxErr)
		}
		if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("waiting for volume %s junction path interrupted: %w", volumeName, err)
		}
		if errors.IsNotFoundError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("timed out waiting for volume %s junction path: %w", volumeName, err)
	}

	return volume, nil
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

// newOntapBackOff returns exponential backoff using the waitForOntap* vars, with
// MaxElapsedTime shortened when ctx carries a sooner deadline.
func newOntapBackOff(ctx context.Context) *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = waitForOntapInitialInterval
	bo.MaxInterval = waitForOntapMaxInterval
	bo.Multiplier = waitForOntapMultiplier
	bo.RandomizationFactor = waitForOntapRandomization
	bo.MaxElapsedTime = waitForOntapMaxElapsed
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
			return backoff.Permanent(errors.NotFoundError("LUN %s not found", lunPath))
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
			return backoff.Permanent(errors.NotFoundError("namespace %s not found", nsPath))
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
