// Copyright 2024 NetApp, Inc. All Rights Reserved.
package mock_exec

import "fmt"

// ExitErrorStub is a stub for exec.ExitError.
type MockExitError struct {
	code    int
	Message string
}

func NewMockExitError(code int, message string) *MockExitError {
	return &MockExitError{
		code:    code,
		Message: message,
	}
}

// Error implements the error interface.
func (e *MockExitError) Error() string {
	return fmt.Sprintf("exit code %v", e.code)
}

// ExitCode implements the ExitError interface.
func (e *MockExitError) ExitCode() int {
	return e.code
}
