// Copyright 2026 NetApp, Inc. All Rights Reserved.

package objectprovisioner

import (
	"fmt"

	"github.com/netapp/trident/cosi/provisioner"
	"github.com/netapp/trident/utils/errors"
)

// translateError maps a core.Orchestrator error onto the provisioner sentinel the COSI
// reconcilers branch on. An unmatched error becomes a retryable codes.Internal further out, in
// cosi/adapter.
func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.IsNotFoundError(err):
		return fmt.Errorf("%w: %s", provisioner.ErrNotFound, err)
	case errors.IsAlreadyExistsError(err):
		return fmt.Errorf("%w: %s", provisioner.ErrAlreadyExists, err)
	case errors.IsInvalidInputError(err):
		return fmt.Errorf("%w: %s", provisioner.ErrInvalidArgument, err)
	case errors.IsUnsupportedError(err):
		return fmt.Errorf("%w: %s", provisioner.ErrUnsupported, err)
	case errors.IsPermissionDeniedError(err):
		return fmt.Errorf("%w: %s", provisioner.ErrPermissionDenied, err)
	case errors.IsBucketNotEmptyError(err):
		return fmt.Errorf("%w: %s", provisioner.ErrBucketNotEmpty, err)
	default:
		return err
	}
}

// translateGrantAccessError adds one mapping on top of translateError: GrantBucketAccess's only
// ConflictError case is a cross-store grant, which is exactly what
// ErrMultiBucketAccessUnsupported means. Kept out of translateError since ConflictError is used
// for unrelated things elsewhere in core.
func translateGrantAccessError(err error) error {
	if errors.IsConflictError(err) {
		return fmt.Errorf("%w: %s", provisioner.ErrMultiBucketAccessUnsupported, err)
	}
	return translateError(err)
}
