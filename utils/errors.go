package utils

import (
	"fmt"
	"strings"
)

/////////////////////////////////////////////////////////////////////////////
// bootstrapError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// foundError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// notFoundError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// resourceNotFoundError - To identify external not found errors
/////////////////////////////////////////////////////////////////////////////

func IsResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

/////////////////////////////////////////////////////////////////////////////
// notReadyError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// unsupportedError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// volumeCreatingError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// volumeDeletingError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// timeoutError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// unsupportedKubernetesVersionError
/////////////////////////////////////////////////////////////////////////////

type unsupportedKubernetesVersionError struct {
	message string
}

func (e *unsupportedKubernetesVersionError) Error() string { return e.message }

func UnsupportedKubernetesVersionError(err error) error {
	return &unsupportedKubernetesVersionError{
		message: fmt.Sprintf("unsupported Kubernetes version; %s", err.Error()),
	}
}

func IsUnsupportedKubernetesVersionError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*unsupportedKubernetesVersionError)
	return ok
}

/////////////////////////////////////////////////////////////////////////////
// reconcileDeferredError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// reconcileIncompleteError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// reconcileFailedError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// unsupportedConfigError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// tempOperatorError
/////////////////////////////////////////////////////////////////////////////

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

/////////////////////////////////////////////////////////////////////////////
// invalidInputError
/////////////////////////////////////////////////////////////////////////////

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
