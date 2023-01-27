package version

import "fmt"

// ///////////////////////////////////////////////////////////////////////////
// unsupportedKubernetesVersionError
// ///////////////////////////////////////////////////////////////////////////

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
