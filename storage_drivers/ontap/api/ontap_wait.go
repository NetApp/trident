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

const (
	waitForLunInitialInterval = 100 * time.Millisecond
	waitForLunMaxInterval     = 2 * time.Second
	waitForLunMaxElapsed      = 30 * time.Second
	waitForLunMultiplier      = 2
	waitForLunRandomization   = 0.2
)

// LunGetter is the minimal surface needed for WaitForLunToExist. OntapAPI satisfies it.
type LunGetter interface {
	LunGetByName(ctx context.Context, name string) (*Lun, error)
}

// WaitForLunToExist calls LunGetByName until the LUN at lunPath is visible, ctx is done, or attempts
// exceed the backoff budget. Only errors.IsNotFoundError results are retried (e.g. ONTAP REST
// read-after-write where the LUN collection is briefly empty right after create). Other errors fail
// immediately.
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
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = waitForLunInitialInterval
	bo.MaxInterval = waitForLunMaxInterval
	bo.Multiplier = waitForLunMultiplier
	bo.RandomizationFactor = waitForLunRandomization
	bo.MaxElapsedTime = waitForLunMaxElapsed
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < bo.MaxElapsedTime {
			bo.MaxElapsedTime = remaining
		}
	}
	if err := backoff.RetryNotify(operation, backoff.WithContext(bo, ctx), notify); err != nil {
		var perm *backoff.PermanentError
		if stderrors.As(err, &perm) {
			return nil, perm.Err
		}
		if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("waiting for LUN %s interrupted: %w", lunPath, err)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("waiting for LUN %s interrupted: %w", lunPath, ctxErr)
		}
		return nil, fmt.Errorf("timed out waiting for LUN %s: %w", lunPath, err)
	}
	return found, nil
}
