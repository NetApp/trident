// Copyright 2024 NetApp, Inc. All Rights Reserved.

package api

import (
	"errors"
	"regexp"
	"strings"

	"github.com/netapp/trident/pkg/collection"
)

// ///////////////////////////////////////////////////////////////////////////
// REST error codes
// ///////////////////////////////////////////////////////////////////////////
const (
	ENTRY_DOESNT_EXIST                         = "4"
	DUPLICATE_ENTRY                            = "1"
	INVALID_ENTRY                              = "655446"
	DP_VOLUME_NOT_INITIALIZED                  = "917536"
	SNAPMIRROR_TRANSFER_IN_PROGRESS            = "13303812"
	SNAPMIRROR_TRANSFER_IN_PROGRESS_BROKEN_OFF = "13303808" // Transition to broken_off state failed. Reason:Another transfer is in progress
	SNAPMIRROR_MODIFICATION_IN_PROGRESS        = "13303822"
	LUN_MAP_EXIST_ERROR                        = "5374922"
	FLEXGROUP_VOLUME_SIZE_ERROR_REST           = "917534"
	EXPORT_POLICY_NOT_FOUND                    = "1703954"
	EXPORT_POLICY_RULE_EXISTS                  = "1704070"
	CONSISTENCY_GROUP_SNAP_EXISTS_ERROR        = "53411921"
	NVME_SUBSYSTEM_ALREADY_EXISTS              = "72090025"
	VOLUME_BUSY_ERROR_REST                     = "524486"
	VOLUME_CREATE_IN_PROGRESS_ERROR_REST       = "13107405"
	LUN_ALREADY_EXISTS_ERROR_REST              = "5374242"
	LUN_CREATE_IN_PROGRESS_ERROR_REST          = "5702832"
	LUN_DUPLICATE_NAME_ERROR_REST              = "5440688"
	LUN_CREATED_PROPERTIES_UNSET_ERROR_REST    = "5374863"
	LUN_CREATED_PROPERTIES_UNREADABLE_REST     = "5374886"
)

// lunCreateConflictRESTCodes are the LUN create responses that mean the LUN exists on the array: it was
// already there, another request is making it, or this request made it but could not finish reporting on
// it. None of them warrant tearing the Flexvol down, because a retry can reconcile what is there.
var lunCreateConflictRESTCodes = []string{
	LUN_ALREADY_EXISTS_ERROR_REST,
	LUN_CREATE_IN_PROGRESS_ERROR_REST,
	LUN_DUPLICATE_NAME_ERROR_REST,
	LUN_CREATED_PROPERTIES_UNSET_ERROR_REST,
	LUN_CREATED_PROPERTIES_UNREADABLE_REST,
}

// VolumeBusyRESTCodeRegexp matches REST errno VOLUME_BUSY_ERROR_REST when comma-heavy messages break ExtractError.
var VolumeBusyRESTCodeRegexp = regexp.MustCompile(`(?i)\bCode:\s*` + VOLUME_BUSY_ERROR_REST + `\b`)

// VolumeCreateInProgressRESTCodeRegexp matches the REST job error returned for a competing volume create.
var VolumeCreateInProgressRESTCodeRegexp = regexp.MustCompile(
	`(?i)\bCode:\s*` + VOLUME_CREATE_IN_PROGRESS_ERROR_REST + `\b`,
)

var lunCreateConflictRESTCodeRegexp = regexp.MustCompile(
	`(?i)\bCode:\s*(?:` + strings.Join(lunCreateConflictRESTCodes, "|") + `)\b`,
)

// IsVolumeBusyRESTError reports whether err means another ONTAP volume job is still active.
func IsVolumeBusyRESTError(err error) bool {
	if err == nil {
		return false
	}
	_, _, code := ExtractError(err)
	return code == VOLUME_BUSY_ERROR_REST || code == VOLUME_CREATE_IN_PROGRESS_ERROR_REST ||
		VolumeBusyRESTCodeRegexp.MatchString(err.Error()) ||
		VolumeCreateInProgressRESTCodeRegexp.MatchString(err.Error())
}

// IsLUNCreateConflictRESTError reports whether ONTAP says the LUN the create asked for is already on the
// array, so the caller can reconcile it on a retry instead of destroying the Flexvol underneath it.
func IsLUNCreateConflictRESTError(err error) bool {
	if err == nil {
		return false
	}
	_, _, code := ExtractError(err)
	return collection.ContainsString(lunCreateConflictRESTCodes, code) ||
		lunCreateConflictRESTCodeRegexp.MatchString(err.Error())
}

// IsVolumeBusyError reports whether ONTAP rejected an operation because another volume job is still active.
func IsVolumeBusyError(err error) bool {
	return err != nil &&
		(IsVolumeBusyRESTError(err) || strings.Contains(strings.ToLower(err.Error()), "volume is busy"))
}

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
	var target *volumeCreateJobExistsError
	return errors.As(err, &target)
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
