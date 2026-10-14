// Copyright 2026 NetApp, Inc. All Rights Reserved.

package provisioner

import "errors"

// The COSI reconcilers decide whether to retry, or mark an object permanently failed, from the
// gRPC status code a driver returns. A backend here returns ordinary Go errors; package adapter
// maps each sentinel below to the code the reconcilers expect. An unwrapped error maps to a
// retryable internal error, so wrap one of these whenever the outcome is known.
var (
	// ErrNotFound reports that the bucket or account does not exist on the
	// backend. Delete and revoke should not return it, since those are
	// idempotent and an absent object means success.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists reports that an object with the requested identity
	// exists but is incompatible with the request, such as a bucket of the same
	// name provisioned with different parameters. Terminal: the reconciler
	// stops rather than retrying, because a retry cannot resolve it.
	ErrAlreadyExists = errors.New("already exists with incompatible properties")

	// ErrInvalidArgument reports a malformed or unsatisfiable request, such as
	// a required parameter that is missing or a protocol the bucket cannot
	// serve. Terminal.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrUnsupported reports that the backend does not implement the operation
	// at all. Terminal.
	ErrUnsupported = errors.New("unsupported")

	// ErrPermissionDenied reports that the backend refused the operation for
	// authorization reasons. Retryable, on the assumption that a permission
	// grant may land shortly.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrUnauthenticated reports that the backend rejected Trident's
	// credentials. Retryable, since a credential refresh may fix it.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrBucketNotEmpty reports a delete refused because the bucket still holds
	// objects. Retryable: the bucket may be emptied, at which point the delete
	// succeeds without user action.
	ErrBucketNotEmpty = errors.New("bucket is not empty")

	// ErrMultiBucketAccessUnsupported reports that GrantAccess was asked to grant one account
	// access to a set of buckets the backend cannot service together, e.g. buckets that span
	// backends. Per the COSI spec, this is the one case DriverGrantBucketAccess must answer with
	// OUT_OF_RANGE rather than InvalidArgument. Terminal: the bucket set is a property of the
	// BucketAccess the reconciler will not retry with a different one.
	//
	// TODO(reederc42): decide whether provisioner sentinel errors like this belong in their own
	// package, or in utils/errors alongside TerminalReconciliationError.
	ErrMultiBucketAccessUnsupported = errors.New("backend does not support multi-bucket access")
)
