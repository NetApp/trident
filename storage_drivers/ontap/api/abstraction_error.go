// Copyright 2024 NetApp, Inc. All Rights Reserved.

package api

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
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
	CONSISTENCY_GROUP_EXISTS_ERROR             = "53411852"
	CONSISTENCY_GROUP_SNAP_EXISTS_ERROR        = "53411921"
	NVME_SUBSYSTEM_ALREADY_EXISTS              = "72090025"
	VOLUME_BUSY_ERROR_REST                     = "524486"
	LUN_EXISTS_ERROR_REST                      = "5374242" // LUN create: a LUN already exists at the path
	LUN_CREATE_IN_PROGRESS_ERROR_REST          = "5702832" // LUN create: the LUN is still being created
	VOLUME_MOVE_INTERNAL_ERROR                 = "144182225"
)

// VolumeBusyRESTCodeRegexp matches REST errno VOLUME_BUSY_ERROR_REST when comma-heavy Messages break ExtractError.
var VolumeBusyRESTCodeRegexp = regexp.MustCompile(`(?i)\bCode:\s*` + VOLUME_BUSY_ERROR_REST + `\b`)

// IsVolumeBusyRESTError reports whether err is the ONTAP REST "Volume busy" response (code 524486) seen during
// volume-offline in a forced delete.
func IsVolumeBusyRESTError(err error) bool {
	if err == nil {
		return false
	}
	_, _, code := ExtractError(err)
	return code == VOLUME_BUSY_ERROR_REST || VolumeBusyRESTCodeRegexp.MatchString(err.Error())
}

// createTargetExistsRESTCodeRegexp matches the REST errnos ONTAP returns when a LUN or NVMe namespace create
// finds the object already there or still being created. DUPLICATE_ENTRY is the generic REST duplicate errno.
var createTargetExistsRESTCodeRegexp = regexp.MustCompile(
	`(?i)\bCode:\s*(` + LUN_EXISTS_ERROR_REST + `|` + LUN_CREATE_IN_PROGRESS_ERROR_REST + `|` +
		DUPLICATE_ENTRY + `)\b`)

// IsCreateTargetExistsRESTError reports whether err is the REST response to a LUN or NVMe namespace create
// whose target already exists or is still being created. Such a create may have been accepted by an earlier
// attempt without being visible yet, so the object has to be looked up again before anything is destroyed.
//
// The match is deliberately narrow: an error that is not recognized is treated as a hard create failure.
func IsCreateTargetExistsRESTError(err error) bool {
	if err == nil {
		return false
	}
	if createTargetExistsRESTCodeRegexp.MatchString(err.Error()) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") || strings.Contains(message, "duplicate entry")
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
// JobNotDoneError
// ///////////////////////////////////////////////////////////////////////////

// JobNotDoneError reports that an ONTAP job was still running when the caller stopped polling it. ONTAP
// keeps working on the job, so the object the job creates may still appear, or the job may still fail.
type JobNotDoneError struct {
	JobUUID string
	// Cause is the error that stopped the last status check, when polling ran out of time on an error
	// rather than on a job observed still running.
	Cause error
}

func (e *JobNotDoneError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("job %v not yet done", e.JobUUID)
	}
	return fmt.Sprintf("job %v not yet done: %v", e.JobUUID, e.Cause)
}

func (e *JobNotDoneError) Unwrap() error { return e.Cause }

// AsJobNotDoneError returns the JobNotDoneError in err's chain, if any.
func AsJobNotDoneError(err error) (*JobNotDoneError, bool) {
	var target *JobNotDoneError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
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
// snapshotReferencedByMirrorError
// ///////////////////////////////////////////////////////////////////////////

type snapshotReferencedByMirrorError struct {
	message string
}

func (e *snapshotReferencedByMirrorError) Error() string { return e.message }

func SnapshotReferencedByMirrorError(message string) error {
	return &snapshotReferencedByMirrorError{message}
}

func IsSnapshotReferencedByMirrorError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*snapshotReferencedByMirrorError)
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
