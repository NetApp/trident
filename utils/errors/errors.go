// Copyright 2025 NetApp, Inc. All Rights Reserved.

package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.uber.org/multierr"
)

const (
	IsNegativeErrorMsg string = `must be greater than or equal to 0`
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

func Join(errs ...error) error {
	return errors.Join(errs...)
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
	if len(a) == 0 {
		return &foundError{message: message}
	}
	return &foundError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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
	if len(a) == 0 {
		return &notFoundError{message: message}
	}
	return &notFoundError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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
// alreadyExistsError
// ///////////////////////////////////////////////////////////////////////////

type alreadyExistsError struct {
	inner   error
	message string
}

func (e *alreadyExistsError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *alreadyExistsError) Unwrap() error { return e.inner }

func AlreadyExistsError(message string, a ...any) error {
	if len(a) == 0 {
		return &alreadyExistsError{message: message}
	}
	return &alreadyExistsError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func WrapWithAlreadyExistsError(err error, message string, a ...any) error {
	return &alreadyExistsError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *alreadyExistsError
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

func UnsupportedError(message string, a ...any) error {
	if len(a) == 0 {
		return &unsupportedError{message: message}
	}
	return &unsupportedError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

func VolumeCreatingError(message string, a ...any) error {
	if len(a) == 0 {
		return &volumeCreatingError{message: message}
	}
	return &volumeCreatingError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

func VolumeDeletingError(message string, a ...any) error {
	if len(a) == 0 {
		return &volumeDeletingError{message: message}
	}
	return &volumeDeletingError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

func VolumeStateError(message string, a ...any) error {
	if len(a) == 0 {
		return &volumeStateError{message: message}
	}
	return &volumeStateError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsVolumeStateError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeStateError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// connectionError
// ///////////////////////////////////////////////////////////////////////////

type connectionError struct {
	inner   error
	message string
}

func (e *connectionError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *connectionError) Unwrap() error {
	// Return the inner error.
	return e.inner
}

func ConnectionError(message string, a ...any) error {
	if len(a) == 0 {
		return &connectionError{message: message}
	}
	return &connectionError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func WrapWithConnectionError(err error, message string, a ...any) error {
	return &connectionError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *connectionError
	return errors.As(err, &errPointer)
}

// ///////////////////////////////////////////////////////////////////////////
// timeoutError
// ///////////////////////////////////////////////////////////////////////////

type timeoutError struct {
	message string
}

func (e *timeoutError) Error() string { return e.message }

func TimeoutError(message string, a ...any) error {
	if len(a) == 0 {
		return &timeoutError{message: message}
	}
	return &timeoutError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*timeoutError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// maxWaitExceededError
// ///////////////////////////////////////////////////////////////////////////

type maxWaitExceededError struct {
	message string
}

func (e *maxWaitExceededError) Error() string { return e.message }

func MaxWaitExceededError(message string, a ...any) error {
	if len(a) == 0 {
		return &maxWaitExceededError{message: message}
	}
	return &maxWaitExceededError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsMaxWaitExceededError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*maxWaitExceededError)
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
	if len(a) == 0 {
		return &reconcileDeferredError{message: message}
	}
	return &reconcileDeferredError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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
	if len(a) == 0 {
		return &reconcileIncompleteError{message: message}
	}
	return &reconcileIncompleteError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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
	if len(a) == 0 {
		return &reconcileFailedError{message: message}
	}
	return &reconcileFailedError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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
	if len(a) == 0 {
		return &unsupportedConfigError{message: message}
	}
	return &unsupportedConfigError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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
// usageStatsUnavailableError
// ///////////////////////////////////////////////////////////////////////////

type usageStatsUnavailableError struct {
	message string
}

func (e *usageStatsUnavailableError) Error() string { return e.message }

func UsageStatsUnavailableError(message string, a ...any) error {
	if len(a) == 0 {
		return &usageStatsUnavailableError{message: message}
	}
	return &usageStatsUnavailableError{message: fmt.Sprintf(message, a...)}
}

func IsUsageStatsUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *usageStatsUnavailableError
	return errors.As(err, &errPointer)
}

// ///////////////////////////////////////////////////////////////////////////
// unlicensedError
// ///////////////////////////////////////////////////////////////////////////

type unlicensedError struct {
	message string
}

func (e *unlicensedError) Error() string { return e.message }

func UnlicensedError(message string, a ...any) error {
	if len(a) == 0 {
		return &unlicensedError{message: message}
	}
	return &unlicensedError{
		message: fmt.Sprintf(fmt.Sprintf("%s", message), a...),
	}
}

func WrapUnlicensedError(err error) error {
	if err == nil {
		return nil
	}
	return multierr.Combine(UnlicensedError("unlicensed"), err)
}

func IsUnlicensedError(err error) bool {
	if err == nil {
		return false
	}
	var errPointer *unlicensedError
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

func InvalidInputError(message string, a ...any) error {
	if len(a) == 0 {
		return &invalidInputError{message: message}
	}
	return &invalidInputError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsInvalidInputError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*invalidInputError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// mismatchedStorageClassError
// ///////////////////////////////////////////////////////////////////////////

type mismatchedStorageClassError struct {
	message string
}

func (e *mismatchedStorageClassError) Error() string {
	return e.message
}

func MismatchedStorageClassError(message string, a ...any) error {
	if len(a) == 0 {
		return &mismatchedStorageClassError{message: message}
	}
	return &mismatchedStorageClassError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsMismatchedStorageClassError(err error) bool {
	if err == nil {
		return false
	}

	var errPointer *mismatchedStorageClassError
	return errors.As(err, &errPointer)
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

func MaxLimitReachedError(message string, a ...any) error {
	if len(a) == 0 {
		return &maxLimitReachedError{message: message}
	}
	return &maxLimitReachedError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

func AuthError(message string, a ...any) error {
	if len(a) == 0 {
		return &authError{message: message}
	}
	return &authError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

func ISCSIDeviceFlushError(message string, a ...any) error {
	if len(a) == 0 {
		return &iSCSIDeviceFlushError{message: message}
	}
	return &iSCSIDeviceFlushError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsISCSIDeviceFlushError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*iSCSIDeviceFlushError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// iSCSISameLunNumberError
// ///////////////////////////////////////////////////////////////////////////

type iSCSISameLunNumberError struct {
	message string
}

func (e *iSCSISameLunNumberError) Error() string { return e.message }

func ISCSISameLunNumberError(message string, a ...any) error {
	if len(a) == 0 {
		return &iSCSISameLunNumberError{message: message}
	}
	return &iSCSISameLunNumberError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsISCSISameLunNumberError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*iSCSISameLunNumberError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// fcpSameLunNumberError
// ///////////////////////////////////////////////////////////////////////////

type fcpSameLunNumberError struct {
	message string
}

func (e *fcpSameLunNumberError) Error() string { return e.message }

func FCPSameLunNumberError(message string, a ...any) error {
	if len(a) == 0 {
		return &fcpSameLunNumberError{message: message}
	}
	return &fcpSameLunNumberError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func IsFCPSameLunNumberError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*fcpSameLunNumberError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// tooManyRequestsError (HTTP 429)
// ///////////////////////////////////////////////////////////////////////////

type tooManyRequestsError struct {
	inner   error
	message string
}

func (e *tooManyRequestsError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *tooManyRequestsError) Unwrap() error { return e.inner }

func TooManyRequestsError(message string, a ...any) error {
	if len(a) == 0 {
		return &tooManyRequestsError{message: message}
	}
	return &tooManyRequestsError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func WrapWithTooManyRequestsError(err error, message string, a ...any) error {
	return &tooManyRequestsError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
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

func IncorrectLUKSPassphraseError(message string, a ...any) error {
	if len(a) == 0 {
		return &incorrectLUKSPassphrase{message: message}
	}
	return &incorrectLUKSPassphrase{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

func InvalidJSONError(message string, a ...any) error {
	if len(a) == 0 {
		return &invalidJSONError{message: message}
	}
	return &invalidJSONError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
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

// ///////////////////////////////////////////////////////////////////////////
// notManagedError
// ///////////////////////////////////////////////////////////////////////////

type notManagedError struct {
	inner   error
	message string
}

func (e *notManagedError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *notManagedError) Unwrap() error { return e.inner }

func NotManagedError(message string, a ...any) error {
	if len(a) == 0 {
		return &notManagedError{message: message}
	}
	return &notManagedError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func WrapWithNotManagedError(err error, message string, a ...any) error {
	return &notManagedError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsNotManagedError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *notManagedError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// formatError
// ///////////////////////////////////////////////////////////////////////////

type formatError struct {
	message string
}

func (f *formatError) Error() string { return f.message }

func FormatError(err error) error {
	return &formatError{
		fmt.Sprintf("Formatting failed; %s", err.Error()),
	}
}

func IsFormatError(err error) bool {
	var f *formatError
	return errors.As(err, &f)
}

// ///////////////////////////////////////////////////////////////////////////
// conflictError
// ///////////////////////////////////////////////////////////////////////////

type conflictError struct {
	inner   error
	message string
}

func (e *conflictError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *conflictError) Unwrap() error { return e.inner }

// ConflictError should be used when modifying a resource is disallowed due to
// interconnectedness or dependencies, such as when a snapshot is part of a group of snapshots.
func ConflictError(message string, a ...any) error {
	if len(a) == 0 {
		return &conflictError{message: message}
	}
	return &conflictError{message: fmt.Sprintf(fmt.Sprintf("%s", message), a...)}
}

func WrapWithConflictError(err error, message string, a ...any) error {
	return &conflictError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsConflictError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *conflictError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// mustRetryError
// ///////////////////////////////////////////////////////////////////////////

type mustRetryError struct {
	inner   error
	message string
}

func (e *mustRetryError) Error() string {
	if e.inner == nil || e.inner.Error() == "" {
		return e.message
	} else if e.message == "" {
		return e.inner.Error()
	}
	return fmt.Sprintf("%v; %v", e.message, e.inner.Error())
}

func (e *mustRetryError) Unwrap() error { return e.inner }

// MustRetryError is a custom error type to indicate that an operation should be retried.
func MustRetryError(message string, a ...any) error {
	if len(a) == 0 {
		return &mustRetryError{message: message}
	}
	return &mustRetryError{message: fmt.Sprintf(message, a...)}
}

func WrapWithMustRetryError(err error, message string, a ...any) error {
	return &mustRetryError{
		inner:   err,
		message: fmt.Sprintf(message, a...),
	}
}

func IsMustRetryError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *mustRetryError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// interfaceNotSupportedError
// ///////////////////////////////////////////////////////////////////////////

// interfaceNotSupportedError is returned when a requested type doesn't support the requested interface.
type interfaceNotSupportedError struct {
	message string
}

func (e *interfaceNotSupportedError) Error() string { return e.message }

// InterfaceNotSupportedError creates a new error when a requested type doesn't support the requested interface.
func InterfaceNotSupportedError(requestedType, interfaceName string) error {
	return &interfaceNotSupportedError{
		message: fmt.Sprintf("requested type %q does not support interface %s",
			requestedType, interfaceName),
	}
}

// IsInterfaceNotSupportedError returns true if err is an interfaceNotSupportedError.
func IsInterfaceNotSupportedError(err error) bool {
	if err == nil {
		return false
	}
	var interfaceNotSupportedError *interfaceNotSupportedError
	ok := errors.As(err, &interfaceNotSupportedError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// zeroValueError
// ///////////////////////////////////////////////////////////////////////////

type zeroValueError struct {
	message string
}

func (e *zeroValueError) Error() string { return e.message }

// ZeroValueError creates a new error when a value is a zero value.
func ZeroValueError(message string, a ...any) error {
	if len(a) == 0 {
		return &zeroValueError{message: message}
	}
	return &zeroValueError{message: fmt.Sprintf(message, a...)}
}

// IsZeroValueError returns true if the error is a zero value error.
func IsZeroValueError(err error) bool {
	if err == nil {
		return false
	}
	var zeroValueError *zeroValueError
	ok := errors.As(err, &zeroValueError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// busClosedError
// ///////////////////////////////////////////////////////////////////////////

// busClosedError indicates an operation was attempted on a closed EventBus.
type busClosedError struct {
	message string
}

func (e *busClosedError) Error() string { return e.message }

// BusClosedError creates a new bus closed error.
func BusClosedError() error {
	return &busClosedError{
		message: "eventbus: bus is closed",
	}
}

// IsBusClosedError returns true if the error is a bus closed error.
func IsBusClosedError(err error) bool {
	if err == nil {
		return false
	}
	var busClosedError *busClosedError
	ok := errors.As(err, &busClosedError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// nilHandlerError
// ///////////////////////////////////////////////////////////////////////////

// nilHandlerError indicates a nil handler was passed to Subscribe.
type nilHandlerError struct {
	message string
}

func (e *nilHandlerError) Error() string { return e.message }

// NilHandlerError creates a new nil handler error.
func NilHandlerError() error {
	return &nilHandlerError{
		message: "eventbus: handler cannot be nil",
	}
}

// IsNilHandlerError returns true if the error is a nil handler error.
func IsNilHandlerError(err error) bool {
	if err == nil {
		return false
	}
	var nilHandlerError *nilHandlerError
	ok := errors.As(err, &nilHandlerError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// StateError - error that contains state and optional message
// ///////////////////////////////////////////////////////////////////////////

type StateError struct {
	State   string // The state (e.g., "Starting", "Stopping", "Running", "Stopped")
	Message string // Optional message
}

func (e *StateError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("state: %s; %s", e.State, e.Message)
	}
	return fmt.Sprintf("state: %s", e.State)
}

// NewStateError creates a new error with state and optional message
func NewStateError(state string, message string) error {
	return &StateError{State: state, Message: message}
}

// IsStateError returns true if the error is a state error
func IsStateError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *StateError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// autogrowPolicyInUseError
// ///////////////////////////////////////////////////////////////////////////

type autogrowPolicyInUseError struct {
	message string
}

func (e *autogrowPolicyInUseError) Error() string { return e.message }

// AutogrowPolicyInUseError creates an error when policy cannot be deleted because volumes are using it
func AutogrowPolicyInUseError(policyName string, volumeCount int, volumes []string) error {
	return &autogrowPolicyInUseError{
		message: fmt.Sprintf("cannot delete policy %s: in use by %d volume(s): %v",
			policyName, volumeCount, volumes),
	}
}

func IsAutogrowPolicyInUseError(err error) bool {
	if err == nil {
		return false
	}

	var policyInUseErr *autogrowPolicyInUseError
	return errors.As(err, &policyInUseErr)
}

// ///////////////////////////////////////////////////////////////////////////
// autogrowPolicyNotUsableError
// ///////////////////////////////////////////////////////////////////////////
type autogrowPolicyNotUsableError struct {
	message string
}

func (e *autogrowPolicyNotUsableError) Error() string { return e.message }

// AutogrowPolicyNotUsableError when policy exists but is in Failed/Deleting state
func AutogrowPolicyNotUsableError(policyName string, state string) error {
	return &autogrowPolicyNotUsableError{
		message: fmt.Sprintf("autogrow policy '%s' exists but is in '%s' state and cannot be used", policyName, state),
	}
}

func IsAutogrowPolicyNotUsableError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *autogrowPolicyNotUsableError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// keyError - error for operations involving keys
// ///////////////////////////////////////////////////////////////////////////

type keyError struct {
	key     string // String representation of the key
	message string // Error message
}

func (e *keyError) Error() string {
	return e.message
}

// Key returns the key that caused the error
func (e *keyError) Key() string {
	return e.key
}

// KeyNotFoundError creates an error when a key is not found
func KeyNotFoundError(key any) error {
	return &keyError{
		key:     fmt.Sprintf("%v", key),
		message: fmt.Sprintf("key %v does not exist in cache", key),
	}
}

// IsKeyError returns true if the error is a key error
func IsKeyError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *keyError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// valueError - error for operations involving values
// ///////////////////////////////////////////////////////////////////////////

type valueError struct {
	key     string // String representation of the key (optional)
	value   string // String representation of the value
	message string // Error message
}

func (e *valueError) Error() string {
	return e.message
}

// Key returns the key that caused the error (may be empty if no key was provided)
func (e *valueError) Key() string {
	return e.key
}

// Value returns the value that caused the error
func (e *valueError) Value() string {
	return e.value
}

// ValueNotFoundError creates an error when a value is not found
// If key is provided, the error message includes the key context
// Usage: ValueNotFoundError(value) or ValueNotFoundError(value, key)
func ValueNotFoundError(value any, key ...any) error {
	var keyStr string
	var msg string

	if len(key) > 0 && key[0] != nil {
		keyStr = fmt.Sprintf("%v", key[0])
		msg = fmt.Sprintf("for key %v, value %v does not exist in cache", key[0], value)
	} else {
		msg = fmt.Sprintf("value %v does not exist in cache", value)
	}

	return &valueError{
		key:     keyStr,
		value:   fmt.Sprintf("%v", value),
		message: msg,
	}
}

// IsValueError returns true if the error is a value error
func IsValueError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *valueError
	return errors.As(err, &errPtr)
}

// ///////////////////////////////////////////////////////////////////////////
// autogrowPolicyNotFoundError
// ///////////////////////////////////////////////////////////////////////////

type autogrowPolicyNotFoundError struct {
	message string
}

func (e *autogrowPolicyNotFoundError) Error() string { return e.message }

// AutogrowPolicyNotFoundError creates an error when a referenced Autogrow policy does not exist
func AutogrowPolicyNotFoundError(policyName string) error {
	return &autogrowPolicyNotFoundError{
		message: fmt.Sprintf("autogrow policy '%s' not found", policyName),
	}
}

func IsAutogrowPolicyNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var errPtr *autogrowPolicyNotFoundError
	return errors.As(err, &errPtr)
}
