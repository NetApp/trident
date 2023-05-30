// Copyright 2023 NetApp, Inc. All Rights Reserved.

package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.uber.org/multierr"
)

// ///////////////////////////////////////////////////////////////////////////
// Wrappers for standard library errors package
// ///////////////////////////////////////////////////////////////////////////

func New(message string) error {
	return errors.New(message)
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target any) bool {
	return errors.As(err, target)
}

func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// ///////////////////////////////////////////////////////////////////////////
// bootstrapError
// ///////////////////////////////////////////////////////////////////////////

type bootstrapError struct {
	message string
}

func (e *bootstrapError) Error() string { return e.message }

func BootstrapError(err error) error {
	return &bootstrapError{
		fmt.Sprintf("Trident initialization failed; %s", err.Error()),
	}
}

func IsBootstrapError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*bootstrapError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// foundError
// ///////////////////////////////////////////////////////////////////////////

type foundError struct {
	inner   error
	message string
}

func (e *foundError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *foundError) Unwrap() error { return e.inner }

func FoundError(message string, a ...any) error {
	return &foundError{message: fmt.Sprintf(message, a...)}
}

func WrapWithFoundError(err error, message string, a ...any) error {
	return &foundError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsFoundError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *foundError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// notFoundError
// ///////////////////////////////////////////////////////////////////////////

type notFoundError struct {
	inner   error
	message string
}

func (e *notFoundError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *notFoundError) Unwrap() error { return e.inner }

func NotFoundError(message string, a ...any) error {
	return &notFoundError{message: fmt.Sprintf(message, a...)}
}

func WrapWithNotFoundError(err error, message string, a ...any) error {
	return &notFoundError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *notFoundError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// resourceNotFoundError - To identify external not found errors
// ///////////////////////////////////////////////////////////////////////////

func IsResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// ///////////////////////////////////////////////////////////////////////////
// notReadyError
// ///////////////////////////////////////////////////////////////////////////

type notReadyError struct {
	message string
}

func (e *notReadyError) Error() string { return e.message }

func NotReadyError() error {
	return &notReadyError{
		"Trident is initializing, please try again later",
	}
}

func IsNotReadyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*notReadyError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// unsupportedError
// ///////////////////////////////////////////////////////////////////////////

type unsupportedError struct {
	message string
}

func (e *unsupportedError) Error() string { return e.message }

func UnsupportedError(message string) error {
	return &unsupportedError{message}
}

func IsUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*unsupportedError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// volumeCreatingError
// ///////////////////////////////////////////////////////////////////////////

type volumeCreatingError struct {
	message string
}

func (e *volumeCreatingError) Error() string { return e.message }

func VolumeCreatingError(message string) error {
	return &volumeCreatingError{message}
}

func IsVolumeCreatingError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeCreatingError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// volumeDeletingError
// ///////////////////////////////////////////////////////////////////////////

type volumeDeletingError struct {
	message string
}

func (e *volumeDeletingError) Error() string { return e.message }

func VolumeDeletingError(message string) error {
	return &volumeDeletingError{message}
}

func IsVolumeDeletingError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeDeletingError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// volumeStateError
// ///////////////////////////////////////////////////////////////////////////

type volumeStateError struct {
	message string
}

func (e *volumeStateError) Error() string { return e.message }

func VolumeStateError(message string) error {
	return &volumeStateError{message}
}

func IsVolumeStateError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeStateError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// timeoutError
// ///////////////////////////////////////////////////////////////////////////

type timeoutError struct {
	message string
}

func (e *timeoutError) Error() string { return e.message }

func TimeoutError(message string) error {
	return &timeoutError{message}
}

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*timeoutError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// reconcileDeferredError
// ///////////////////////////////////////////////////////////////////////////

type reconcileDeferredError struct {
	inner   error
	message string
}

func (e *reconcileDeferredError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *reconcileDeferredError) Unwrap() error {
	// Return the inner error.
	return e.inner
}

func ReconcileDeferredError(message string, a ...any) error {
	return &reconcileDeferredError{message: fmt.Sprintf(message, a...)}
}

func WrapWithReconcileDeferredError(err error, message string, a ...any) error {
	return &reconcileDeferredError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsReconcileDeferredError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *reconcileDeferredError
	return errors.As(err, &errPointer)
}

// ///////////////////////////////////////////////////////////////////////////
// reconcileIncompleteError
// ///////////////////////////////////////////////////////////////////////////

type reconcileIncompleteError struct {
	inner   error
	message string
}

func (e *reconcileIncompleteError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *reconcileIncompleteError) Unwrap() error {
	// Return the inner error.
	return e.inner
}

func ReconcileIncompleteError(message string, a ...any) error {
	return &reconcileIncompleteError{message: fmt.Sprintf(message, a...)}
}

func WrapWithReconcileIncompleteError(err error, message string, a ...any) error {
	return &reconcileIncompleteError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsReconcileIncompleteError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *reconcileIncompleteError
	return errors.As(err, &errPointer)
}

// ///////////////////////////////////////////////////////////////////////////
// reconcileFailedError
// ///////////////////////////////////////////////////////////////////////////

type reconcileFailedError struct {
	inner   error
	message string
}

func (e *reconcileFailedError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *reconcileFailedError) Unwrap() error {
	// Return the inner error.
	return e.inner
}

func ReconcileFailedError(message string, a ...any) error {
	return &reconcileFailedError{message: fmt.Sprintf(message, a...)}
}

func WrapWithReconcileFailedError(err error, message string, a ...any) error {
	return &reconcileFailedError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsReconcileFailedError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *reconcileFailedError
	return errors.As(err, &errPointer)
}

// ///////////////////////////////////////////////////////////////////////////
// unsupportedConfigError
// ///////////////////////////////////////////////////////////////////////////

type unsupportedConfigError struct {
	message string
}

func (e *unsupportedConfigError) Error() string { return e.message }

func UnsupportedConfigError(message string, a ...any) error {
	return &unsupportedConfigError{
		message: fmt.Sprintf(message, a...),
	}
}

func WrapUnsupportedConfigError(err error) error {
	if err == nil {
		return nil
	}
	return multierr.Combine(UnsupportedConfigError("unsupported config error"), err)
}

func IsUnsupportedConfigError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *unsupportedConfigError
	return errors.As(err, &errPointer)
}

// ///////////////////////////////////////////////////////////////////////////
// tempOperatorError
// ///////////////////////////////////////////////////////////////////////////

type tempOperatorError struct {
	message string
}

func (e *tempOperatorError) Error() string { return e.message }

func TempOperatorError(err error) error {
	return &tempOperatorError{
		message: fmt.Sprintf("temporary operator error; %s", err.Error()),
	}
}

func IsTempOperatorError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*tempOperatorError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// invalidInputError
// ///////////////////////////////////////////////////////////////////////////

type invalidInputError struct {
	message string
}

func (e *invalidInputError) Error() string { return e.message }

func InvalidInputError(message string) error {
	return &invalidInputError{message}
}

func IsInvalidInputError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*invalidInputError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// unsupportedCapacityRangeError
// ///////////////////////////////////////////////////////////////////////////

type unsupportedCapacityRangeError struct {
	err     error
	message string
}

func (e *unsupportedCapacityRangeError) Unwrap() error { return e.err }

func (e *unsupportedCapacityRangeError) Error() string { return e.message }

func UnsupportedCapacityRangeError(err error) error {
	return &unsupportedCapacityRangeError{
		err, fmt.Sprintf("unsupported capacity range; %s",
			err.Error()),
	}
}

func HasUnsupportedCapacityRangeError(err error) (bool, *unsupportedCapacityRangeError) {
	if err == nil {
		return false, nil
	}
	var unsupportedCapacityRangeErrorPtr *unsupportedCapacityRangeError
	ok := errors.As(err, &unsupportedCapacityRangeErrorPtr)
	return ok, unsupportedCapacityRangeErrorPtr
}

// ///////////////////////////////////////////////////////////////////////////
// maxLimitReachedError
// ///////////////////////////////////////////////////////////////////////////

type maxLimitReachedError struct {
	message string
}

func (e *maxLimitReachedError) Error() string { return e.message }

func MaxLimitReachedError(message string) error {
	return &maxLimitReachedError{message}
}

func IsMaxLimitReachedError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*maxLimitReachedError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// typeAssertionError
// ///////////////////////////////////////////////////////////////////////////

type typeAssertionError struct {
	assertion string
}

func (e *typeAssertionError) Error() string {
	return fmt.Sprintf("could not perform assertion: %s", e.assertion)
}

func TypeAssertionError(assertion string) error {
	return &typeAssertionError{assertion}
}

// ///////////////////////////////////////////////////////////////////////////
// authError
// ///////////////////////////////////////////////////////////////////////////

type authError struct {
	message string
}

func (e *authError) Error() string { return e.message }

func AuthError(message string) error {
	return &authError{message}
}

func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*authError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// iSCSIDeviceFlushError
// ///////////////////////////////////////////////////////////////////////////

type iSCSIDeviceFlushError struct {
	message string
}

func (e *iSCSIDeviceFlushError) Error() string { return e.message }

func ISCSIDeviceFlushError(message string) error {
	return &iSCSIDeviceFlushError{message}
}

func IsISCSIDeviceFlushError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*iSCSIDeviceFlushError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// tooManyRequestsError (HTTP 429)
// ///////////////////////////////////////////////////////////////////////////

type tooManyRequestsError struct {
	message string
}

func (e *tooManyRequestsError) Error() string { return e.message }

func TooManyRequestsError(message string) error {
	return &tooManyRequestsError{message}
}

func IsTooManyRequestsError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*tooManyRequestsError)
	return ok
}

// ////////////////////////////////////////////////////////////////////////////
// incorrectLUKSPassphrase
// ////////////////////////////////////////////////////////////////////////////

type incorrectLUKSPassphrase struct {
	message string
}

func (e *incorrectLUKSPassphrase) Error() string { return e.message }

func IncorrectLUKSPassphraseError(message string) error {
	return &incorrectLUKSPassphrase{message}
}

func IsIncorrectLUKSPassphraseError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*incorrectLUKSPassphrase)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// invalidJSONError (if could not unmarshal JSON for any non-retryable reason)
// ///////////////////////////////////////////////////////////////////////////

type invalidJSONError struct {
	message string
}

func (e *invalidJSONError) Error() string { return e.message }

func InvalidJSONError(message string) error {
	return &invalidJSONError{message}
}

func IsInvalidJSONError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*invalidJSONError)
	return ok
}

// AsInvalidJSONError returns an InvalidJSONError, true if the error means the data cannot be unmarshaled, or the
// original error, false if it does not meet those conditions.
func AsInvalidJSONError(err error) (error, bool) {
	var syntaxErr *json.SyntaxError
	var jsonErr *json.UnmarshalTypeError

	if err == nil {
		return err, false
	}

	isJsonErr := errors.As(err, &jsonErr)
	jErr, ok := err.(*json.UnmarshalTypeError)
	// If a json.UnmarshalTypeError has a Type field that is nil, calling Error() on it will cause a nil pointer panic.
	isNilTypeErr := ok && jErr.Type == nil

	isSyntaxErr := errors.As(err, &syntaxErr)
	isEOFErr := errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
	asInvalidJSON := isJsonErr || isEOFErr || isSyntaxErr

	// IsInvalidJSONError checks for nil.
	if IsInvalidJSONError(err) {
		return err, true
	}

	if asInvalidJSON {
		msg := "is nil-typed json.UnmarshalTypeError"
		if !isNilTypeErr {
			msg = err.Error()
		}
		return InvalidJSONError(msg), true
	}

	return err, false
}

// ///////////////////////////////////////////////////////////////////////////
// nodeNotSafeToPublishForBackend
// ///////////////////////////////////////////////////////////////////////////

type nodeNotSafeToPublishForBackend struct {
	node        string
	backendType string
}

func (e *nodeNotSafeToPublishForBackend) Error() string {
	return fmt.Sprintf("not safe to publish %s volume to node %s", e.backendType, e.node)
}

func NodeNotSafeToPublishForBackendError(node, backendType string) error {
	return &nodeNotSafeToPublishForBackend{node, backendType}
}

func IsNodeNotSafeToPublishForBackendError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*nodeNotSafeToPublishForBackend)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// resourceExhaustedError
// ///////////////////////////////////////////////////////////////////////////

type resourceExhaustedError struct {
	err     error
	message string
}

func (e *resourceExhaustedError) Unwrap() error { return e.err }

func (e *resourceExhaustedError) Error() string { return e.message }

func ResourceExhaustedError(err error) error {
	return &resourceExhaustedError{err, fmt.Sprintf("insufficient resources; %v", err)}
}

func HasResourceExhaustedError(err error) (bool, *resourceExhaustedError) {
	if err == nil {
		return false, nil
	}
	var resourceExhaustedErrorPtr *resourceExhaustedError
	ok := errors.As(err, &resourceExhaustedErrorPtr)
	return ok, resourceExhaustedErrorPtr
}

// ///////////////////////////////////////////////////////////////////////////
// inProgressError
// ///////////////////////////////////////////////////////////////////////////

type inProgressError struct {
	message string
}

func (e *inProgressError) Error() string { return e.message }

func InProgressError(str string) error {
	return &inProgressError{
		message: fmt.Sprintf("in progress error; %s", str),
	}
}

func IsInProgressError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*inProgressError)
	return ok
}
