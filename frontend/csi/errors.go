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

type terminalReconciliationError struct {
	message string
}

func (e *terminalReconciliationError) Error() string { return e.message }

func TerminalReconciliationError(message string) error {
	return &terminalReconciliationError{message}
}

func IsTerminalReconciliationError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*terminalReconciliationError)
	return ok
}
