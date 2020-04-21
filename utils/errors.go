package utils

import (
	"fmt"
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
