// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

/////////////////////////////////////////////////////////////////////////////
// volumeCreateJobExistsError
/////////////////////////////////////////////////////////////////////////////
type volumeCreateJobExistsError struct {
	message string
}

func (e *volumeCreateJobExistsError) Error() string { return e.message }

func VolumeCreateJobExistsError(message string) error {
	return &volumeCreateJobExistsError{message}
}

func IsVolumeCreateJobExistsError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeCreateJobExistsError)
	return ok
}

/////////////////////////////////////////////////////////////////////////////
// volumeReadError
/////////////////////////////////////////////////////////////////////////////

type volumeReadError struct {
	message string
}

func (e *volumeReadError) Error() string { return e.message }

func VolumeReadError(message string) error {
	return &volumeReadError{message}
}

func IsVolumeReadError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeReadError)
	return ok
}

/////////////////////////////////////////////////////////////////////////////
// volumeIdAttributesReadError
/////////////////////////////////////////////////////////////////////////////

type volumeIdAttributesReadError struct {
	message string
}

func (e *volumeIdAttributesReadError) Error() string { return e.message }

func VolumeIdAttributesReadError(message string) error {
	return &volumeIdAttributesReadError{message}
}

func IsVolumeIdAttributesReadError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeIdAttributesReadError)
	return ok
}

/////////////////////////////////////////////////////////////////////////////
// volumeSpaceAttributesReadError
/////////////////////////////////////////////////////////////////////////////

type volumeSpaceAttributesReadError struct {
	message string
}

func (e *volumeSpaceAttributesReadError) Error() string { return e.message }

func VolumeSpaceAttributesReadError(message string) error {
	return &volumeIdAttributesReadError{message}
}

func IsVolumeSpaceAttributesReadError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeSpaceAttributesReadError)
	return ok
}

/////////////////////////////////////////////////////////////////////////////
// snapshotBusyError
/////////////////////////////////////////////////////////////////////////////

type snapshotBusyError struct {
	message string
}

func (e *snapshotBusyError) Error() string { return e.message }

func SnapshotBusyError(message string) error {
	return &snapshotBusyError{message}
}

func IsSnapshotBusyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*snapshotBusyError)
	return ok
}

/////////////////////////////////////////////////////////////////////////////
// ApiError
/////////////////////////////////////////////////////////////////////////////

type apiError struct {
	message string
}

func (e *apiError) Error() string { return e.message }

func ApiError(message string) error {
	return &apiError{message}
}

func IsApiError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*apiError)
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
// snapmirrorTransferInProgress
/////////////////////////////////////////////////////////////////////////////
type snapmirrorTransferInProgress struct {
	message string
}

func (e *snapmirrorTransferInProgress) Error() string { return e.message }

func SnapmirrorTransferInProgress(message string) error {
	return &snapmirrorTransferInProgress{message}
}

func IsSnapmirrorTransferInProgress(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*snapmirrorTransferInProgress)
	return ok
}
