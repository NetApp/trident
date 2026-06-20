// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

type invalidTrackingFileError struct {
	message string
}

func (e *invalidTrackingFileError) Error() string { return e.message }

func InvalidTrackingFileError(message string) error {
	return &invalidTrackingFileError{message}
}

func IsInvalidTrackingFileError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*invalidTrackingFileError)
	return ok
}
