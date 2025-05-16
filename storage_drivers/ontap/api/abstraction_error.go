// Copyright 2024 NetApp, Inc. All Rights Reserved.

package api

// ///////////////////////////////////////////////////////////////////////////
// REST error codes
// ///////////////////////////////////////////////////////////////////////////
const (
	ENTRY_DOESNT_EXIST                         = "4"
	DUPLICATE_ENTRY                            = "1"
	DP_VOLUME_NOT_INITIALIZED                  = "917536"
	SNAPMIRROR_TRANSFER_IN_PROGRESS            = "13303812"
	SNAPMIRROR_TRANSFER_IN_PROGRESS_BROKEN_OFF = "13303808" // Transition to broken_off state failed. Reason:Another transfer is in progress
	SNAPMIRROR_MODIFICATION_IN_PROGRESS        = "13303822"
	LUN_MAP_EXIST_ERROR                        = "5374922"
	FLEXGROUP_VOLUME_SIZE_ERROR_REST           = "917534"
	EXPORT_POLICY_NOT_FOUND                    = "1703954"
	EXPORT_POLICY_RULE_EXISTS                  = "1704070"
)

// ///////////////////////////////////////////////////////////////////////////
// volumeCreateJobExistsError
// ///////////////////////////////////////////////////////////////////////////
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

// ///////////////////////////////////////////////////////////////////////////
// volumeReadError
// ///////////////////////////////////////////////////////////////////////////

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

// ///////////////////////////////////////////////////////////////////////////
// volumeIdAttributesReadError
// ///////////////////////////////////////////////////////////////////////////

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

// ///////////////////////////////////////////////////////////////////////////
// volumeSpaceAttributesReadError
// ///////////////////////////////////////////////////////////////////////////

type volumeSpaceAttributesReadError struct {
	message string
}

func (e *volumeSpaceAttributesReadError) Error() string { return e.message }

func VolumeSpaceAttributesReadError(message string) error {
	return &volumeSpaceAttributesReadError{message}
}

func IsVolumeSpaceAttributesReadError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeSpaceAttributesReadError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// snapshotBusyError
// ///////////////////////////////////////////////////////////////////////////

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

// ///////////////////////////////////////////////////////////////////////////
// ApiError
// ///////////////////////////////////////////////////////////////////////////

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
// notReadyError
// ///////////////////////////////////////////////////////////////////////////
type notReadyError struct {
	message string
}

func (e *notReadyError) Error() string { return e.message }

func NotReadyError(message string) error {
	return &notReadyError{message}
}

func IsNotReadyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*notReadyError)
	return ok
}

// ///////////////////////////////////////////////////////////////////////////
// tooManyLunsError
// ///////////////////////////////////////////////////////////////////////////
type tooManyLunsError struct {
	message string
}

func (e *tooManyLunsError) Error() string { return e.message }

func TooManyLunsError(message string) error {
	return &tooManyLunsError{message}
}

func IsTooManyLunsError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*tooManyLunsError)
	return ok
}
