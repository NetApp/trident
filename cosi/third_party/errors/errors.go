/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// COSI internal error types.
package errors

import "errors"

// NonRetryableError is an error that should not be retried but still treated as a reconcile error.
func NonRetryableError(wrapped error) error {
	return &nonRetryableError{err: wrapped}
}

type nonRetryableError struct {
	err error
}

func (te *nonRetryableError) Error() string {
	return te.err.Error()
}

// This function will return nil if te.err is nil.
func (te *nonRetryableError) Unwrap() error {
	return te.err
}

func (te *nonRetryableError) Is(target error) bool {
	tp := &nonRetryableError{}
	return errors.As(target, &tp)
}
