// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

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
	message string
}

func (e *foundError) Error() string { return e.message }

func FoundError(message string) error {
	return &foundError{message}
}

func IsFoundError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*foundError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// notFoundError
// ///////////////////////////////////////////////////////////////////////////

type notFoundError struct {
	message string
}

func (e *notFoundError) Error() string { return e.message }

func NotFoundError(message string) error {
	return &notFoundError{message}
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*notFoundError)
	return ok
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
	message string
}

func (e *reconcileDeferredError) Error() string { return e.message }

func ReconcileDeferredError(err error) error {
	return &reconcileDeferredError{
		message: err.Error(),
	}
}

func IsReconcileDeferredError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*reconcileDeferredError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// reconcileIncompleteError
// ///////////////////////////////////////////////////////////////////////////

type reconcileIncompleteError struct {
	message string
}

func (e *reconcileIncompleteError) Error() string { return e.message }

func ReconcileIncompleteError() error {
	return &reconcileIncompleteError{
		message: "reconcile incomplete",
	}
}

func ConvertToReconcileIncompleteError(err error) error {
	return &reconcileIncompleteError{
		message: err.Error(),
	}
}

func IsReconcileIncompleteError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*reconcileIncompleteError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// reconcileFailedError
// ///////////////////////////////////////////////////////////////////////////

type reconcileFailedError struct {
	message string
}

func (e *reconcileFailedError) Error() string { return e.message }

func ReconcileFailedError(err error) error {
	return &reconcileFailedError{
		message: fmt.Sprintf("reconcile failed; %s", err.Error()),
	}
}

func IsReconcileFailedError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*reconcileFailedError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// unsupportedConfigError
// ///////////////////////////////////////////////////////////////////////////

type unsupportedConfigError struct {
	message string
}

func (e *unsupportedConfigError) Error() string { return e.message }

func UnsupportedConfigError(err error) error {
	return &unsupportedConfigError{
		message: fmt.Sprintf("unsupported configuration; %s", err.Error()),
	}
}

func IsUnsupportedConfigError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*unsupportedConfigError)
	return ok
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

/////////////////////////////////////////////////////////////////////////////
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

// ////////////////////////////////////////////////////////////////////////////
// invalidJSONError (if could not unmarshal JSON for any non-retryable reason)
// ////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// resourceExhaustedError
/////////////////////////////////////////////////////////////////////////////

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
