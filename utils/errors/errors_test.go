// Copyright 2025 NetApp, Inc. All Rights Reserved.

package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/multierr"
)

// Helper functions for testing errors
func assertErrorIs(t *testing.T, err, target error) {
	t.Helper()
	assert.True(t, Is(err, target), "expected error to be %v, got %v", target, err)
}

func assertErrorIsNot(t *testing.T, err, target error) {
	t.Helper()
	assert.False(t, Is(err, target), "expected error not to be %v", target)
}

func assertErrorAs(t *testing.T, err error, target interface{}) {
	t.Helper()
	assert.True(t, As(err, target), "expected error to be assignable to %T", target)
}

func assertErrorNotAs(t *testing.T, err error, target interface{}) {
	t.Helper()
	assert.False(t, As(err, target), "expected error not to be assignable to %T", target)
}

// createDeepWrappedError creates an error wrapped multiple levels deep for testing error wrapping behavior.
// It returns both the original error and the final wrapped error.
func createDeepWrappedError(msg string, wrapDepth int) (original, wrapped error) {
	original = New(msg)
	wrapped = original
	for i := 0; i < wrapDepth; i++ {
		wrapped = fmt.Errorf("level %d: %w", i+1, wrapped)
	}
	return original, wrapped
}

// TestStandardLibraryWrappers verifies that our error package's wrapper functions
// around the standard library behave identically to the original functions.
func TestStandardLibraryWrappers(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "New creates error with correct message",
			testFunc: func(t *testing.T) {
				err := New("test error")
				assert.Equal(t, "test error", err.Error())
			},
		},
		{
			name: "Is correctly identifies error equality",
			testFunc: func(t *testing.T) {
				err1 := New("error")
				err2 := New("different error")
				assertErrorIs(t, err1, err1)
				assertErrorIsNot(t, err1, err2)
			},
		},
		{
			name: "As correctly assigns error types",
			testFunc: func(t *testing.T) {
				var target *json.UnmarshalTypeError
				jsonErr := &json.UnmarshalTypeError{Value: "test"}
				standardErr := New("standard error")
				assertErrorAs(t, jsonErr, &target)
				assertErrorNotAs(t, standardErr, &target)
			},
		},
		{
			name: "Unwrap correctly unwraps errors",
			testFunc: func(t *testing.T) {
				originalErr := New("original error")
				wrappedErr := fmt.Errorf("wrapped: %w", originalErr)
				assert.Equal(t, originalErr, Unwrap(wrappedErr))
				assert.Nil(t, Unwrap(originalErr))
			},
		},
		{
			name: "Join combines multiple errors",
			testFunc: func(t *testing.T) {
				err1 := New("error 1")
				err2 := New("error 2")
				joinedErr := Join(err1, err2)
				assertErrorIs(t, joinedErr, err1)
				assertErrorIs(t, joinedErr, err2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// TestDeepErrorWrapping verifies that error wrapping works correctly at multiple levels
// of nesting using both our package's wrapper functions and the standard library.
func TestDeepErrorWrapping(t *testing.T) {
	tests := []struct {
		name     string
		depth    int
		message  string
		testFunc func(*testing.T, error, error)
	}{
		{
			name:    "Single level wrapping",
			depth:   1,
			message: "root error",
			testFunc: func(t *testing.T, original, wrapped error) {
				assertErrorIs(t, wrapped, original)
				assert.Equal(t, original, Unwrap(wrapped))
			},
		},
		{
			name:    "Deep wrapping (5 levels)",
			depth:   5,
			message: "deep root error",
			testFunc: func(t *testing.T, original, wrapped error) {
				// Should still be able to find original error
				assertErrorIs(t, wrapped, original)

				// Should be able to unwrap all the way down
				current := wrapped
				for i := 0; i < 5; i++ {
					current = Unwrap(current)
					assert.NotNil(t, current, "unwrap chain broken at level %d", i+1)
				}
				assert.Equal(t, original, current)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, wrapped := createDeepWrappedError(tt.message, tt.depth)
			tt.testFunc(t, original, wrapped)
		})
	}
}

// TestSimpleErrorTypes verifies that all simple error types (those without additional
// context or parameters) are created correctly and can be identified using their
// respective Is functions. These errors represent common operational states that
// don't require additional context.
func TestSimpleErrorTypes(t *testing.T) {
	tests := []struct {
		name        string
		createErr   func() error
		isErrFunc   func(error) bool
		expectedMsg string
		description string // documents the purpose/use case of this error type
	}{
		{
			name:        "NotReadyError",
			createErr:   func() error { return NotReadyError() },
			isErrFunc:   IsNotReadyError,
			description: "Used when a component or system is temporarily not ready to handle requests",
			expectedMsg: "Trident is initializing, please try again later",
		},
		{
			name:        "UnsupportedError_WithArgs",
			createErr:   func() error { return UnsupportedError("unsupported operation %s", "test") },
			isErrFunc:   IsUnsupportedError,
			expectedMsg: "unsupported operation test",
		},
		{
			name:        "UnsupportedError_NoArgs",
			createErr:   func() error { return UnsupportedError("simple message") },
			isErrFunc:   IsUnsupportedError,
			expectedMsg: "simple message",
		},
		{
			name:        "VolumeCreatingError_WithArgs",
			createErr:   func() error { return VolumeCreatingError("volume %s is creating", "vol1") },
			isErrFunc:   IsVolumeCreatingError,
			expectedMsg: "volume vol1 is creating",
		},
		{
			name:        "VolumeCreatingError_NoArgs",
			createErr:   func() error { return VolumeCreatingError("simple creating message") },
			isErrFunc:   IsVolumeCreatingError,
			expectedMsg: "simple creating message",
		},
		{
			name:        "VolumeDeletingError_WithArgs",
			createErr:   func() error { return VolumeDeletingError("volume %s is deleting", "vol1") },
			isErrFunc:   IsVolumeDeletingError,
			expectedMsg: "volume vol1 is deleting",
		},
		{
			name:        "VolumeDeletingError_NoArgs",
			createErr:   func() error { return VolumeDeletingError("simple deleting message") },
			isErrFunc:   IsVolumeDeletingError,
			expectedMsg: "simple deleting message",
		},
		{
			name:        "VolumeStateError_WithArgs",
			createErr:   func() error { return VolumeStateError("volume %s in invalid state", "vol1") },
			isErrFunc:   IsVolumeStateError,
			expectedMsg: "volume vol1 in invalid state",
		},
		{
			name:        "VolumeStateError_NoArgs",
			createErr:   func() error { return VolumeStateError("simple state message") },
			isErrFunc:   IsVolumeStateError,
			expectedMsg: "simple state message",
		},
		{
			name:        "MaxWaitExceededError_WithArgs",
			createErr:   func() error { return MaxWaitExceededError("max wait exceeded for %s", "operation") },
			isErrFunc:   IsMaxWaitExceededError,
			expectedMsg: "max wait exceeded for operation",
		},
		{
			name:        "MaxWaitExceededError_NoArgs",
			createErr:   func() error { return MaxWaitExceededError("simple wait message") },
			isErrFunc:   IsMaxWaitExceededError,
			expectedMsg: "simple wait message",
		},
		{
			name:        "InvalidInputError_WithArgs",
			createErr:   func() error { return InvalidInputError("invalid input: %s", "test") },
			isErrFunc:   IsInvalidInputError,
			expectedMsg: "invalid input: test",
		},
		{
			name:        "InvalidInputError_NoArgs",
			createErr:   func() error { return InvalidInputError("simple input message") },
			isErrFunc:   IsInvalidInputError,
			expectedMsg: "simple input message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.createErr()

			// Test error creation and message
			assert.Equal(t, tt.expectedMsg, err.Error())

			// Test detection function
			assert.True(t, tt.isErrFunc(err))
			assert.False(t, tt.isErrFunc(nil))
			assert.False(t, tt.isErrFunc(errors.New("generic error")))
		})
	}
}

// Test TempOperatorError separately due to different signature
func TestTempOperatorError(t *testing.T) {
	originalErr := errors.New("connection failed")
	err := TempOperatorError(originalErr)

	assert.True(t, IsTempOperatorError(err))
	assert.Equal(t, "temporary operator error; connection failed", err.Error())
	assert.False(t, IsTempOperatorError(nil))
	assert.False(t, IsTempOperatorError(errors.New("generic error")))
}

// Test additional simple error types with different signatures
func TestAdditionalSimpleErrorTypes(t *testing.T) {
	tests := []struct {
		name        string
		createErr   func() error
		isErrFunc   func(error) bool
		expectedMsg string
	}{
		{
			name:        "MaxLimitReachedError",
			createErr:   func() error { return MaxLimitReachedError("limit reached for %s", "volumes") },
			isErrFunc:   IsMaxLimitReachedError,
			expectedMsg: "limit reached for volumes",
		},
		{
			name:        "MaxLimitReachedError_NoArgs",
			createErr:   func() error { return MaxLimitReachedError("simple limit message") },
			isErrFunc:   IsMaxLimitReachedError,
			expectedMsg: "simple limit message",
		},
		{
			name:        "AuthError",
			createErr:   func() error { return AuthError("authentication failed for %s", "user") },
			isErrFunc:   IsAuthError,
			expectedMsg: "authentication failed for user",
		},
		{
			name:        "AuthError_NoArgs",
			createErr:   func() error { return AuthError("simple auth message") },
			isErrFunc:   IsAuthError,
			expectedMsg: "simple auth message",
		},
		{
			name:        "ISCSIDeviceFlushError",
			createErr:   func() error { return ISCSIDeviceFlushError("failed to flush device %s", "sda") },
			isErrFunc:   IsISCSIDeviceFlushError,
			expectedMsg: "failed to flush device sda",
		},
		{
			name:        "ISCSIDeviceFlushError_NoArgs",
			createErr:   func() error { return ISCSIDeviceFlushError("simple flush message") },
			isErrFunc:   IsISCSIDeviceFlushError,
			expectedMsg: "simple flush message",
		},
		{
			name:        "ISCSISameLunNumberError",
			createErr:   func() error { return ISCSISameLunNumberError("LUN %d already exists", 1) },
			isErrFunc:   IsISCSISameLunNumberError,
			expectedMsg: "LUN 1 already exists",
		},
		{
			name:        "ISCSISameLunNumberError_NoArgs",
			createErr:   func() error { return ISCSISameLunNumberError("simple LUN message") },
			isErrFunc:   IsISCSISameLunNumberError,
			expectedMsg: "simple LUN message",
		},
		{
			name:        "FCPSameLunNumberError",
			createErr:   func() error { return FCPSameLunNumberError("FCP LUN %d conflict", 2) },
			isErrFunc:   IsFCPSameLunNumberError,
			expectedMsg: "FCP LUN 2 conflict",
		},
		{
			name:        "FCPSameLunNumberError_NoArgs",
			createErr:   func() error { return FCPSameLunNumberError("simple FCP message") },
			isErrFunc:   IsFCPSameLunNumberError,
			expectedMsg: "simple FCP message",
		},
		{
			name:        "TooManyRequestsError",
			createErr:   func() error { return TooManyRequestsError("rate limit exceeded") },
			isErrFunc:   IsTooManyRequestsError,
			expectedMsg: "rate limit exceeded",
		},
		{
			name:        "TooManyRequestsError_NoArgs",
			createErr:   func() error { return WrapWithTooManyRequestsError(errors.New(""), "simple rate limit") },
			isErrFunc:   IsTooManyRequestsError,
			expectedMsg: "simple rate limit",
		},
		{
			name:        "IncorrectLUKSPassphraseError",
			createErr:   func() error { return IncorrectLUKSPassphraseError("incorrect passphrase") },
			isErrFunc:   IsIncorrectLUKSPassphraseError,
			expectedMsg: "incorrect passphrase",
		},
		{
			name:        "IncorrectLUKSPassphraseError_NoArgs",
			createErr:   func() error { return IncorrectLUKSPassphraseError("simple passphrase error") },
			isErrFunc:   IsIncorrectLUKSPassphraseError,
			expectedMsg: "simple passphrase error",
		},
		{
			name:        "NodeNotSafeToPublishForBackendError",
			createErr:   func() error { return NodeNotSafeToPublishForBackendError("node1", "ontap") },
			isErrFunc:   IsNodeNotSafeToPublishForBackendError,
			expectedMsg: "not safe to publish ontap volume to node node1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.createErr()

			// Test error creation and message
			assert.Equal(t, tt.expectedMsg, err.Error())

			// Test detection function
			assert.True(t, tt.isErrFunc(err))
			assert.False(t, tt.isErrFunc(nil))
			assert.False(t, tt.isErrFunc(errors.New("generic error")))
		})
	}
}

// Test InProgressError separately due to different signature
func TestInProgressError(t *testing.T) {
	err := InProgressError("operation in progress")
	assert.True(t, IsInProgressError(err))
	assert.Equal(t, "in progress error; operation in progress", err.Error())
	assert.False(t, IsInProgressError(nil))
	assert.False(t, IsInProgressError(errors.New("generic error")))
}

// Test TypeAssertionError separately
func TestTypeAssertionError(t *testing.T) {
	err := TypeAssertionError("string to int")
	assert.Equal(t, "could not perform assertion: string to int", err.Error())
}

// Test Unwrap methods
func TestUnwrapMethods(t *testing.T) {
	innerErr := errors.New("inner error")

	tests := []struct {
		name        string
		createErr   func(error) error
		description string
	}{
		{
			name:        "FoundError_Unwrap",
			createErr:   func(inner error) error { return WrapWithFoundError(inner, "found") },
			description: "foundError should unwrap correctly",
		},
		{
			name:        "ConnectionError_Unwrap",
			createErr:   func(inner error) error { return WrapWithConnectionError(inner, "connection") },
			description: "connectionError should unwrap correctly",
		},
		{
			name:        "ReconcileDeferredError_Unwrap",
			createErr:   func(inner error) error { return WrapWithReconcileDeferredError(inner, "deferred") },
			description: "reconcileDeferredError should unwrap correctly",
		},
		{
			name:        "ReconcileIncompleteError_Unwrap",
			createErr:   func(inner error) error { return WrapWithReconcileIncompleteError(inner, "incomplete") },
			description: "reconcileIncompleteError should unwrap correctly",
		},
		{
			name:        "ReconcileFailedError_Unwrap",
			createErr:   func(inner error) error { return WrapWithReconcileFailedError(inner, "failed") },
			description: "reconcileFailedError should unwrap correctly",
		},
		{
			name:        "NotManagedError_Unwrap",
			createErr:   func(inner error) error { return WrapWithNotManagedError(inner, "not managed") },
			description: "notManagedError should unwrap correctly",
		},
		{
			name:        "ConflictError_Unwrap",
			createErr:   func(inner error) error { return WrapWithConflictError(inner, "conflict") },
			description: "conflictError should unwrap correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrappedErr := tt.createErr(innerErr)
			unwrapped := errors.Unwrap(wrappedErr)
			assert.Equal(t, innerErr, unwrapped, tt.description)
		})
	}
}

// Test UnsupportedCapacityRangeError Unwrap and edge cases
func TestUnsupportedCapacityRangeErrorUnwrap(t *testing.T) {
	innerErr := errors.New("capacity error")
	err := UnsupportedCapacityRangeError(innerErr)

	// Test unwrap
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, innerErr, unwrapped)

	// Test edge case in HasUnsupportedCapacityRangeError
	_, targetErr := HasUnsupportedCapacityRangeError(nil)
	assert.Nil(t, targetErr)
}

// Test ResourceExhaustedError Unwrap and edge cases
func TestResourceExhaustedErrorUnwrap(t *testing.T) {
	innerErr := errors.New("resource limit")
	err := ResourceExhaustedError(innerErr)

	// Test unwrap
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, innerErr, unwrapped)

	// Test edge case in HasResourceExhaustedError
	_, targetErr := HasResourceExhaustedError(nil)
	assert.Nil(t, targetErr)
}

// Test IsResourceNotFoundError
func TestIsResourceNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"not found lowercase", errors.New("resource not found"), true},
		{"not found uppercase", errors.New("Resource NOT FOUND"), true},
		{"not found mixed case", errors.New("Item Not Found"), true},
		{"different error", errors.New("connection failed"), false},
		{"empty error", errors.New(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsResourceNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUnsupportedCapacityRangeError(t *testing.T) {
	// test setup
	err := errors.New("a generic error")
	unsupportedCapacityRangeErr := UnsupportedCapacityRangeError(errors.New(
		"error wrapped within UnsupportedCapacityRange"))
	wrappedError := fmt.Errorf("wrapping unsupportedCapacityRange; %w", unsupportedCapacityRangeErr)

	// test exec
	t.Run("should not identify an UnsupportedCapacityRangeError", func(t *testing.T) {
		ok, _ := HasUnsupportedCapacityRangeError(err)
		assert.Equal(t, false, ok)
	})

	t.Run("should identify an UnsupportedCapacityRangeError", func(t *testing.T) {
		ok, _ := HasUnsupportedCapacityRangeError(unsupportedCapacityRangeErr)
		assert.Equal(t, true, ok)
	})

	t.Run("should identify an UnsupportedCapacityRangeError within a wrapped error", func(t *testing.T) {
		ok, _ := HasUnsupportedCapacityRangeError(wrappedError)
		assert.Equal(t, true, ok)
	})
}

// Unified test for wrapping error types that follow similar patterns
func TestWrappingErrorTypes(t *testing.T) {
	genericErr := errors.New("a generic error")

	tests := []struct {
		name        string
		createErr   func(string, ...any) error
		wrapWithErr func(error, string, ...any) error
		isErrFunc   func(error) bool
		errorType   string
	}{
		{
			name:        "NotFoundError",
			createErr:   NotFoundError,
			wrapWithErr: WrapWithNotFoundError,
			isErrFunc:   IsNotFoundError,
			errorType:   "not found",
		},
		{
			name:        "FoundError",
			createErr:   FoundError,
			wrapWithErr: WrapWithFoundError,
			isErrFunc:   IsFoundError,
			errorType:   "found",
		},
		{
			name:        "NotManagedError",
			createErr:   NotManagedError,
			wrapWithErr: WrapWithNotManagedError,
			isErrFunc:   IsNotManagedError,
			errorType:   "not managed",
		},
		{
			name:        "ConnectionError",
			createErr:   ConnectionError,
			wrapWithErr: WrapWithConnectionError,
			isErrFunc:   IsConnectionError,
			errorType:   "connection",
		},
		{
			name:        "ReconcileDeferredError",
			createErr:   ReconcileDeferredError,
			wrapWithErr: WrapWithReconcileDeferredError,
			isErrFunc:   IsReconcileDeferredError,
			errorType:   "reconcile deferred",
		},
		{
			name:        "ReconcileIncompleteError",
			createErr:   ReconcileIncompleteError,
			wrapWithErr: WrapWithReconcileIncompleteError,
			isErrFunc:   IsReconcileIncompleteError,
			errorType:   "reconcile incomplete",
		},
		{
			name:        "ReconcileFailedError",
			createErr:   ReconcileFailedError,
			wrapWithErr: WrapWithReconcileFailedError,
			isErrFunc:   IsReconcileFailedError,
			errorType:   "reconcile failed",
		},
		{
			name:        "ConflictError",
			createErr:   ConflictError,
			wrapWithErr: WrapWithConflictError,
			isErrFunc:   IsConflictError,
			errorType:   "conflict",
		},
		{
			name:        "AlreadyExistsError",
			createErr:   AlreadyExistsError,
			wrapWithErr: WrapWithAlreadyExistsError,
			isErrFunc:   IsAlreadyExistsError,
			errorType:   "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test basic error creation
			err := tt.createErr("test error with formatting %s, %s", "foo", "bar")
			assert.True(t, strings.Contains(err.Error(), "foo"))
			assert.True(t, strings.Contains(err.Error(), "bar"))
			assert.True(t, tt.isErrFunc(err))

			// Test with generic error (should be false)
			assert.False(t, tt.isErrFunc(genericErr))

			// Test with nil (should be false)
			assert.False(t, tt.isErrFunc(nil))

			// Test empty error
			err = tt.createErr("")
			assert.True(t, tt.isErrFunc(err))

			// Test wrapping with non-nil error and message
			err = tt.wrapWithErr(errors.New("inner error"), tt.errorType)
			assert.True(t, tt.isErrFunc(err))
			expectedMsg := fmt.Sprintf("%s; inner error", tt.errorType)
			assert.Equal(t, expectedMsg, err.Error())

			// Test wrapping with nil error
			err = tt.wrapWithErr(nil, tt.errorType)
			assert.Equal(t, tt.errorType, err.Error())

			// Test wrapping with empty message
			err = tt.wrapWithErr(errors.New("inner error"), "")
			assert.True(t, tt.isErrFunc(err))
			assert.Equal(t, "inner error", err.Error())

			// Test wrapping with empty error and message
			err = tt.wrapWithErr(errors.New(""), tt.errorType)
			assert.True(t, tt.isErrFunc(err))
			assert.Equal(t, tt.errorType, err.Error())

			// Test wrapped error detection
			wrappedErr := fmt.Errorf("custom message: %w", tt.createErr(""))
			assert.True(t, tt.isErrFunc(wrappedErr))

			// Test multi-level wrapping
			deepErr := fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", tt.createErr("")))
			assert.True(t, tt.isErrFunc(deepErr))
		})
	}
}

func TestAsInvalidJSONError(t *testing.T) {
	unmarshalTypeErr := &json.UnmarshalTypeError{
		Value:  "",
		Type:   reflect.TypeOf(errors.New("foo")),
		Offset: 0,
		Struct: "",
		Field:  "",
	}
	nilTypeUnmarshalTypeErr := &json.UnmarshalTypeError{}
	syntaxErr := &json.SyntaxError{}
	unexpectedEOF := io.ErrUnexpectedEOF
	anEOF := io.EOF
	invalidJSON := &invalidJSONError{}
	notFound := &notFoundError{}

	unexpectedMsg := "Expected provided error to be usable as an InvalidJSONError!"
	unexpectedInvalidJson := "Did not expect the provided error to be usable as an InvalidJSONError"

	type Args struct {
		Provided      error
		Expected      bool
		UnexpectedStr string
		TestName      string
	}

	testCases := []Args{
		{nil, false, unexpectedInvalidJson, "Nil error"},
		{unmarshalTypeErr, true, unexpectedMsg, "JSON UnmarshalTypeError"},
		{nilTypeUnmarshalTypeErr, true, unexpectedMsg, "Nil typed JSON UnmarshalTypeError"},
		{syntaxErr, true, unexpectedMsg, "JSON SyntaxError"},
		{unexpectedEOF, true, unexpectedMsg, "Unexpected EOF Error"},
		{anEOF, true, unexpectedMsg, "EOF Error"},
		{invalidJSON, true, unexpectedMsg, "Already InvalidJSONError"},
		{notFound, false, unexpectedInvalidJson, "NotFoundError"},
	}

	for _, args := range testCases {
		t.Run(args.TestName, func(t *testing.T) {
			_, isInvalidErr := AsInvalidJSONError(args.Provided)
			assert.Equal(t, args.Expected, isInvalidErr, args.UnexpectedStr)
		})
	}
}

// Test constructor functions
func TestConstructorCoverage(t *testing.T) {
	tests := []struct {
		name        string
		createErr   func() error
		isErrFunc   func(error) bool
		expectedMsg string
	}{
		// InvalidJSONError tests
		{
			name:        "InvalidJSONError_WithArgs",
			createErr:   func() error { return InvalidJSONError("JSON error with %s", "details") },
			isErrFunc:   IsInvalidJSONError,
			expectedMsg: "JSON error with details",
		},
		{
			name:        "InvalidJSONError_NoArgs",
			createErr:   func() error { return InvalidJSONError("simple JSON error") },
			isErrFunc:   IsInvalidJSONError,
			expectedMsg: "simple JSON error",
		},
		// MismatchedStorageClassError tests
		{
			name:        "MismatchedStorageClassError_WithArgs",
			createErr:   func() error { return MismatchedStorageClassError("Storage class error with %s", "details") },
			isErrFunc:   IsMismatchedStorageClassError,
			expectedMsg: "Storage class error with details",
		},
		{
			name:        "MismatchedStorageClassError_NoArgs",
			createErr:   func() error { return MismatchedStorageClassError("simple storage class error") },
			isErrFunc:   IsMismatchedStorageClassError,
			expectedMsg: "simple storage class error",
		},
		// Additional constructor tests
		{
			name:        "TooManyRequestsError_NoArgs",
			createErr:   func() error { return TooManyRequestsError("rate limit") },
			isErrFunc:   IsTooManyRequestsError,
			expectedMsg: "rate limit",
		},
		{
			name:        "IncorrectLUKSPassphraseError_NoArgs",
			createErr:   func() error { return IncorrectLUKSPassphraseError("wrong passphrase") },
			isErrFunc:   IsIncorrectLUKSPassphraseError,
			expectedMsg: "wrong passphrase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.createErr()
			assert.Equal(t, tt.expectedMsg, err.Error())
			assert.True(t, tt.isErrFunc(err))
			assert.False(t, tt.isErrFunc(nil))
		})
	}

	// Test direct Error() methods and nil checks
	jsonErr := &invalidJSONError{message: "direct test"}
	assert.Equal(t, "direct test", jsonErr.Error())
	assert.False(t, IsInvalidJSONError(nil))

	scErr := &mismatchedStorageClassError{message: "direct test"}
	assert.Equal(t, "direct test", scErr.Error())
}

func TestResourceExhaustedError(t *testing.T) {
	resExhaustedErr := ResourceExhaustedError(errors.New("volume limit reached"))

	tests := []struct {
		Name    string
		Err     error
		wantErr assert.BoolAssertionFunc
	}{
		{
			Name:    "NotResourceExhaustedError",
			Err:     errors.New("a generic error"),
			wantErr: assert.False,
		},
		{
			Name:    "ResourceExhaustedError",
			Err:     resExhaustedErr,
			wantErr: assert.True,
		},
		{
			Name:    "WrappedResourceExhaustedError",
			Err:     fmt.Errorf("wrapping resourceExhaustedError; %w", resExhaustedErr),
			wantErr: assert.True,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			ok, _ := HasResourceExhaustedError(tt.Err)
			tt.wantErr(t, ok, "Unexpected error")
		})
	}
}

// Test specific error types with unique behavior
func TestSpecialErrorTypes(t *testing.T) {
	// Test UnsupportedConfigError with multierr support
	t.Run("UnsupportedConfigError", func(t *testing.T) {
		err := UnsupportedConfigError("error with formatting %s, %s", "foo", "bar")
		assert.True(t, strings.Contains("error with formatting foo, bar", err.Error()))
		assert.True(t, IsUnsupportedConfigError(err))
		assert.False(t, IsUnsupportedConfigError(nil))
		assert.False(t, IsUnsupportedConfigError(errors.New("generic")))

		// Multierr tests
		err = multierr.Combine(
			errors.New("not unsupported config err"),
			UnsupportedConfigError("is unsupported config error"),
		)
		assert.True(t, IsUnsupportedConfigError(err))

		err = multierr.Combine(
			errors.New("not unsupported config err"),
			errors.New("not unsupported config err"),
		)
		assert.False(t, IsUnsupportedConfigError(err))

		err = WrapUnsupportedConfigError(errors.New("not unsupported config err"))
		assert.True(t, IsUnsupportedConfigError(err))

		err = WrapUnsupportedConfigError(nil)
		assert.Nil(t, err)
	})

	// Test UnlicensedError with multierr support
	t.Run("UnlicensedError", func(t *testing.T) {
		err := UnlicensedError("error with formatting %s, %s", "foo", "bar")
		assert.True(t, strings.Contains("error with formatting foo, bar", err.Error()))
		assert.True(t, IsUnlicensedError(err))
		assert.False(t, IsUnlicensedError(nil))
		assert.False(t, IsUnlicensedError(errors.New("generic")))

		// Multierr tests
		err = multierr.Combine(
			errors.New("not unlicensed err"),
			UnlicensedError("is unlicensed error"),
		)
		assert.True(t, IsUnlicensedError(err))

		err = multierr.Combine(
			errors.New("not unlicensed err"),
			errors.New("not unlicensed err"),
		)
		assert.False(t, IsUnlicensedError(err))

		err = WrapUnlicensedError(errors.New("not unlicensed err"))
		assert.True(t, IsUnlicensedError(err))

		err = WrapUnlicensedError(nil)
		assert.Nil(t, err)
	})

	// Test UsageStatsUnavailableError
	t.Run("UsageStatsUnavailableError", func(t *testing.T) {
		err := UsageStatsUnavailableError("error with formatting %s, %s", "foo", "bar")
		assert.Contains(t, err.Error(), "error with formatting foo, bar")
		assert.True(t, IsUsageStatsUnavailableError(err))
		assert.False(t, IsUsageStatsUnavailableError(nil))
		assert.False(t, IsUsageStatsUnavailableError(errors.New("generic")))

		// Test without formatting
		err = UsageStatsUnavailableError("simple message")
		assert.Equal(t, "simple message", err.Error())
		assert.True(t, IsUsageStatsUnavailableError(err))
	})

	// Test FormatError
	t.Run("FormatError", func(t *testing.T) {
		err := FormatError(errors.New("formatting error"))
		assert.True(t, IsFormatError(err))
		assert.Equal(t, "Formatting failed; formatting error", err.Error())
		assert.False(t, IsFormatError(errors.New("generic")))

		// Test wrapped format error
		wrappedErr := fmt.Errorf("wrapping formatError; %w", err)
		assert.True(t, IsFormatError(wrappedErr))
	})

	// Test UsageStatsUnavailableError
	t.Run("UsageStatsUnavailableError", func(t *testing.T) {
		err := UsageStatsUnavailableError("error with formatting %s, %s", "foo", "bar")
		assert.Contains(t, err.Error(), "error with formatting foo, bar")
		assert.True(t, IsUsageStatsUnavailableError(err))
		assert.False(t, IsUsageStatsUnavailableError(nil))
		assert.False(t, IsUsageStatsUnavailableError(errors.New("generic")))

		// Test without formatting
		err = UsageStatsUnavailableError("simple message")
		assert.Equal(t, "simple message", err.Error())
		assert.True(t, IsUsageStatsUnavailableError(err))
	})
}

// Consolidate simple error tests
func TestRemainingSimpleErrors(t *testing.T) {
	// Test MismatchedStorageClassError
	t.Run("MismatchedStorageClassError", func(t *testing.T) {
		err := MismatchedStorageClassError("clone volume test-clone from source volume test-source with different storage classes is not allowed")
		assert.True(t, strings.Contains("clone volume test-clone from source volume test-source with different storage classes is not allowed", err.Error()))
		assert.True(t, IsMismatchedStorageClassError(err))
		assert.False(t, IsMismatchedStorageClassError(errors.New("generic")))
		assert.False(t, IsMismatchedStorageClassError(nil))

		// Test no-args path
		err = MismatchedStorageClassError("simple message")
		assert.True(t, IsMismatchedStorageClassError(err))
		assert.Equal(t, "simple message", err.Error())
	})

	// Test BootstrapError
	t.Run("BootstrapError", func(t *testing.T) {
		originalErr := errors.New("database connection failed")
		err := BootstrapError(originalErr)
		assert.True(t, IsBootstrapError(err))
		assert.Equal(t, "Trident initialization failed; database connection failed", err.Error())
		assert.False(t, IsBootstrapError(errors.New("generic")))
		assert.False(t, IsBootstrapError(nil))
	})

	// Test TimeoutError
	t.Run("TimeoutError", func(t *testing.T) {
		err := TimeoutError("timeout error with formatting %s, %s", "foo", "bar")
		assert.True(t, strings.Contains("timeout error with formatting foo, bar", err.Error()))
		assert.True(t, IsTimeoutError(err))
		assert.False(t, IsTimeoutError(errors.New("generic")))
		assert.False(t, IsTimeoutError(nil))

		// Test without formatting
		err = TimeoutError("simple timeout")
		assert.True(t, IsTimeoutError(err))
		assert.Equal(t, "simple timeout", err.Error())
	})
}

func TestMustRetryError(t *testing.T) {
	// Base inner error for wrapping
	inner := errors.New("base transport error")

	tests := []struct {
		name        string
		makeErr     func() error
		wantMsg     string
		wantIsRetry assert.BoolAssertionFunc
		checkUnwrap bool
	}{
		{
			name:        "MustRetryError_NoArgs",
			makeErr:     func() error { return MustRetryError("rate limited") },
			wantMsg:     "rate limited",
			wantIsRetry: assert.True,
			checkUnwrap: false,
		},
		{
			name:        "MustRetryError_WithArgs",
			makeErr:     func() error { return MustRetryError("retry after %s", "backoff") },
			wantMsg:     "retry after backoff",
			wantIsRetry: assert.True,
			checkUnwrap: false,
		},
		{
			name:        "WrapWithMustRetryError_WithInnerAndMessage",
			makeErr:     func() error { return WrapWithMustRetryError(inner, "received HTTP 429 Too Many Requests") },
			wantMsg:     "received HTTP 429 Too Many Requests; base transport error",
			wantIsRetry: assert.True,
			checkUnwrap: true,
		},
		{
			name:        "WrapWithMustRetryError_WithInnerEmptyMessage_UsesInner",
			makeErr:     func() error { return WrapWithMustRetryError(inner, "") },
			wantMsg:     "base transport error",
			wantIsRetry: assert.True,
			checkUnwrap: true,
		},
		{
			name:        "WrapMustRetryError_NilInner_UsesMessage",
			makeErr:     func() error { return WrapWithMustRetryError(nil, "retry later") },
			wantMsg:     "retry later",
			wantIsRetry: assert.True,
			checkUnwrap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.makeErr()
			// message formatting
			assert.Equal(t, tt.wantMsg, err.Error())
			// IsMustRetryError detection
			tt.wantIsRetry(t, IsMustRetryError(err))
			// negative cases
			assert.False(t, IsMustRetryError(nil))
			assert.False(t, IsMustRetryError(errors.New("generic")))

			if tt.checkUnwrap {
				// Unwrap should return the inner error for wrapped cases
				un := Unwrap(err)
				// For nil inner, unwrap should be nil
				if tt.name == "WrapMustRetryError_NilInner_UsesMessage" {
					assert.Nil(t, un)
				} else {
					assert.Equal(t, inner, un)
				}
			}
		})
	}

	// Multi-level wrapping should still be detected
	wrapped := fmt.Errorf("outer: %w", MustRetryError("retry after backoff"))
	assert.True(t, IsMustRetryError(wrapped))
}

func TestInterfaceNotSupportedError(t *testing.T) {
	err := InterfaceNotSupportedError("ants.Config", "MultiPoolWithStats")

	assert.True(t, IsInterfaceNotSupportedError(err))
	assert.Equal(t, `requested type "ants.Config" does not support interface MultiPoolWithStats`, err.Error())
	assert.False(t, IsInterfaceNotSupportedError(nil))
	assert.False(t, IsInterfaceNotSupportedError(errors.New("generic error")))
}

func TestStateError(t *testing.T) {
	tests := []struct {
		name        string
		state       string
		message     string
		expectedMsg string
		testIsFunc  bool
	}{
		{
			name:        "StateError with state and message",
			state:       "Starting",
			message:     "cannot activate: current state is Starting",
			expectedMsg: "state: Starting; cannot activate: current state is Starting",
			testIsFunc:  true,
		},
		{
			name:        "StateError with state only (empty message)",
			state:       "Stopping",
			message:     "",
			expectedMsg: "state: Stopping",
			testIsFunc:  true,
		},
		{
			name:        "StateError with Running state and message",
			state:       "Running",
			message:     "autogrow is already running",
			expectedMsg: "state: Running; autogrow is already running",
			testIsFunc:  true,
		},
		{
			name:        "StateError with Stopped state and message",
			state:       "Stopped",
			message:     "autogrow is already stopped",
			expectedMsg: "state: Stopped; autogrow is already stopped",
			testIsFunc:  true,
		},
		{
			name:        "StateError with empty state and message",
			state:       "",
			message:     "unknown state error",
			expectedMsg: "state: ; unknown state error",
			testIsFunc:  true,
		},
		{
			name:        "StateError with empty state and empty message",
			state:       "",
			message:     "",
			expectedMsg: "state: ",
			testIsFunc:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewStateError(tt.state, tt.message)

			// Verify error message format
			assert.Equal(t, tt.expectedMsg, err.Error())

			if tt.testIsFunc {
				assert.True(t, IsStateError(err), "IsStateError should detect this error")
			}

			// Verify we can extract state and message via type assertion
			var stateErr *StateError
			if assert.True(t, errors.As(err, &stateErr)) {
				assert.Equal(t, tt.state, stateErr.State, "State field should match")
				assert.Equal(t, tt.message, stateErr.Message, "Message field should match")
			}
		})
	}

	t.Run("IsStateError_NilError", func(t *testing.T) {
		assert.False(t, IsStateError(nil))
	})

	t.Run("IsStateError_DifferentErrorType", func(t *testing.T) {
		assert.False(t, IsStateError(errors.New("generic error")))
		assert.False(t, IsStateError(NotFoundError("not found")))
		assert.False(t, IsStateError(InvalidInputError("invalid input")))
	})

	// Test wrapped error detection
	t.Run("IsStateError_WrappedError", func(t *testing.T) {
		originalErr := NewStateError("Starting", "activation in progress")
		wrappedErr := fmt.Errorf("operation failed: %w", originalErr)

		assert.True(t, IsStateError(wrappedErr), "Should detect wrapped state error")

		// Should be able to extract the state error from wrapped error
		var stateErr *StateError
		if assert.True(t, errors.As(wrappedErr, &stateErr)) {
			assert.Equal(t, "Starting", stateErr.State)
			assert.Equal(t, "activation in progress", stateErr.Message)
		}
	})

	// Test multi-level wrapping
	t.Run("IsStateError_DeepWrapping", func(t *testing.T) {
		originalErr := NewStateError("Stopping", "deactivation in progress")
		wrappedOnce := fmt.Errorf("level 1: %w", originalErr)
		wrappedTwice := fmt.Errorf("level 2: %w", wrappedOnce)
		wrappedThrice := fmt.Errorf("level 3: %w", wrappedTwice)

		assert.True(t, IsStateError(wrappedThrice), "Should detect deeply wrapped state error")

		// Should still be able to extract state error fields
		var stateErr *StateError
		if assert.True(t, errors.As(wrappedThrice, &stateErr)) {
			assert.Equal(t, "Stopping", stateErr.State)
			assert.Equal(t, "deactivation in progress", stateErr.Message)
		}
	})

	// Test direct struct creation (verify public fields are accessible)
	t.Run("StateError_DirectStructCreation", func(t *testing.T) {
		err := &StateError{
			State:   "CustomState",
			Message: "custom message",
		}

		assert.Equal(t, "state: CustomState; custom message", err.Error())
		assert.True(t, IsStateError(err))
		assert.Equal(t, "CustomState", err.State)
		assert.Equal(t, "custom message", err.Message)
	})

	// Test state extraction from wrapped error (common use case in orchestrator)
	t.Run("StateError_ExtractStateFromWrappedError", func(t *testing.T) {
		originalErr := NewStateError("Running", "cannot activate: already running")
		wrappedErr := fmt.Errorf("autogrow activation failed: %w", originalErr)

		// Caller pattern: check if it's a state error, then extract state
		if IsStateError(wrappedErr) {
			var stateErr *StateError
			if errors.As(wrappedErr, &stateErr) {
				// This is the pattern used in trident_autogrow_policies.go
				switch stateErr.State {
				case "Starting", "Stopping":
					// Transient states - would requeue
					assert.Contains(t, []string{"Starting", "Stopping"}, stateErr.State)
				case "Running":
					// Already running - this is the case we're testing
					assert.Equal(t, "Running", stateErr.State)
				case "Stopped":
					// Already stopped
					assert.Equal(t, "Stopped", stateErr.State)
				default:
					t.Errorf("Unexpected state: %s", stateErr.State)
				}
			}
		} else {
			t.Error("Expected wrapped error to be detected as StateError")
		}
	})

	// Test all state transitions from autogrow orchestrator
	t.Run("StateError_AllOrchestatorStates", func(t *testing.T) {
		states := []struct {
			state       string
			message     string
			isTransient bool
		}{
			{"Stopped", "autogrow is not running", false},
			{"Starting", "activation in progress", true},
			{"Running", "autogrow is already running", false},
			{"Stopping", "deactivation in progress", true},
		}

		for _, s := range states {
			t.Run(s.state, func(t *testing.T) {
				err := NewStateError(s.state, s.message)
				assert.True(t, IsStateError(err))

				var stateErr *StateError
				if assert.True(t, errors.As(err, &stateErr)) {
					assert.Equal(t, s.state, stateErr.State)
					assert.Equal(t, s.message, stateErr.Message)

					// Verify transient state logic
					isTransient := (stateErr.State == "Starting" || stateErr.State == "Stopping")
					assert.Equal(t, s.isTransient, isTransient,
						"State %s transient classification mismatch", s.state)
				}
			})
		}
	})
}

func TestKeyError(t *testing.T) {
	// Test KeyNotFoundError creation
	t.Run("KeyNotFoundError_StringKey", func(t *testing.T) {
		err := KeyNotFoundError("mykey")
		assert.Error(t, err)
		assert.Equal(t, "key mykey does not exist in cache", err.Error())
		assert.True(t, IsKeyError(err))
	})

	t.Run("KeyNotFoundError_IntKey", func(t *testing.T) {
		err := KeyNotFoundError(42)
		assert.Error(t, err)
		assert.Equal(t, "key 42 does not exist in cache", err.Error())
		assert.True(t, IsKeyError(err))
	})

	// Test Key() method
	t.Run("KeyError_KeyExtraction", func(t *testing.T) {
		err := KeyNotFoundError("test-key")
		var keyErr *keyError
		if assert.True(t, errors.As(err, &keyErr)) {
			assert.Equal(t, "test-key", keyErr.Key())
		}
	})

	// Test negative cases
	t.Run("IsKeyError_Nil", func(t *testing.T) {
		assert.False(t, IsKeyError(nil))
	})

	t.Run("IsKeyError_DifferentErrorType", func(t *testing.T) {
		assert.False(t, IsKeyError(errors.New("generic error")))
		assert.False(t, IsKeyError(NotFoundError("not found")))
	})

	// Test wrapped error detection
	t.Run("IsKeyError_WrappedError", func(t *testing.T) {
		originalErr := KeyNotFoundError("wrapped-key")
		wrappedErr := fmt.Errorf("cache operation failed: %w", originalErr)

		assert.True(t, IsKeyError(wrappedErr))

		var keyErr *keyError
		if assert.True(t, errors.As(wrappedErr, &keyErr)) {
			assert.Equal(t, "wrapped-key", keyErr.Key())
		}
	})
}

func TestValueError(t *testing.T) {
	// Test ValueNotFoundError creation with key
	t.Run("ValueNotFoundError_WithKey", func(t *testing.T) {
		err := ValueNotFoundError("myvalue", "mykey")
		assert.Error(t, err)
		assert.Equal(t, "for key mykey, value myvalue does not exist in cache", err.Error())
		assert.True(t, IsValueError(err))
	})

	// Test ValueNotFoundError creation without key
	t.Run("ValueNotFoundError_WithoutKey", func(t *testing.T) {
		err := ValueNotFoundError("myvalue")
		assert.Error(t, err)
		assert.Equal(t, "value myvalue does not exist in cache", err.Error())
		assert.True(t, IsValueError(err))
	})

	// Test with mixed types
	t.Run("ValueNotFoundError_MixedTypes", func(t *testing.T) {
		err := ValueNotFoundError(123, "key1")
		assert.Error(t, err)
		assert.Equal(t, "for key key1, value 123 does not exist in cache", err.Error())
		assert.True(t, IsValueError(err))
	})

	// Test Key() and Value() methods with key provided
	t.Run("ValueError_KeyValueExtraction_WithKey", func(t *testing.T) {
		err := ValueNotFoundError("test-value", "test-key")
		var valErr *valueError
		if assert.True(t, errors.As(err, &valErr)) {
			assert.Equal(t, "test-key", valErr.Key())
			assert.Equal(t, "test-value", valErr.Value())
		}
	})

	// Test Key() and Value() methods without key
	t.Run("ValueError_KeyValueExtraction_WithoutKey", func(t *testing.T) {
		err := ValueNotFoundError("test-value")
		var valErr *valueError
		if assert.True(t, errors.As(err, &valErr)) {
			assert.Equal(t, "", valErr.Key(), "Key should be empty when not provided")
			assert.Equal(t, "test-value", valErr.Value())
		}
	})

	// Test negative cases
	t.Run("IsValueError_Nil", func(t *testing.T) {
		assert.False(t, IsValueError(nil))
	})

	t.Run("IsValueError_DifferentErrorType", func(t *testing.T) {
		assert.False(t, IsValueError(errors.New("generic error")))
		assert.False(t, IsValueError(NotFoundError("not found")))
		assert.False(t, IsValueError(KeyNotFoundError("key")))
	})

	// Test wrapped error detection
	t.Run("IsValueError_WrappedError", func(t *testing.T) {
		originalErr := ValueNotFoundError("wrapped-value", "wrapped-key")
		wrappedErr := fmt.Errorf("cache delete failed: %w", originalErr)

		assert.True(t, IsValueError(wrappedErr))

		var valErr *valueError
		if assert.True(t, errors.As(wrappedErr, &valErr)) {
			assert.Equal(t, "wrapped-key", valErr.Key())
			assert.Equal(t, "wrapped-value", valErr.Value())
		}
	})

	// Test multi-level wrapping
	t.Run("IsValueError_DeepWrapping", func(t *testing.T) {
		originalErr := ValueNotFoundError("deep-value", "deep-key")
		wrappedOnce := fmt.Errorf("level 1: %w", originalErr)
		wrappedTwice := fmt.Errorf("level 2: %w", wrappedOnce)

		assert.True(t, IsValueError(wrappedTwice))

		var valErr *valueError
		if assert.True(t, errors.As(wrappedTwice, &valErr)) {
			assert.Equal(t, "deep-key", valErr.Key())
			assert.Equal(t, "deep-value", valErr.Value())
		}
	})

	// Test with nil key (should be treated as no key)
	t.Run("ValueNotFoundError_NilKey", func(t *testing.T) {
		err := ValueNotFoundError("myvalue", nil)
		assert.Error(t, err)
		assert.Equal(t, "value myvalue does not exist in cache", err.Error())

		var valErr *valueError
		if assert.True(t, errors.As(err, &valErr)) {
			assert.Equal(t, "", valErr.Key(), "Key should be empty when nil is provided")
			assert.Equal(t, "myvalue", valErr.Value())
		}
	})
}

func TestZeroValueError(t *testing.T) {
	tests := []struct {
		name          string
		makeErr       func() error
		wantMsg       string
		wantIsZeroVal assert.BoolAssertionFunc
	}{
		{
			name:          "ZeroValueError_NoArgs",
			makeErr:       func() error { return ZeroValueError("key must not be zero value") },
			wantMsg:       "key must not be zero value",
			wantIsZeroVal: assert.True,
		},
		{
			name:          "ZeroValueError_WithArgs",
			makeErr:       func() error { return ZeroValueError("field %s must not be zero value", "userName") },
			wantMsg:       "field userName must not be zero value",
			wantIsZeroVal: assert.True,
		},
		{
			name:          "ZeroValueError_MultipleArgs",
			makeErr:       func() error { return ZeroValueError("%s field at index %d is zero value", "ID", 5) },
			wantMsg:       "ID field at index 5 is zero value",
			wantIsZeroVal: assert.True,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.makeErr()
			// message formatting
			assert.Equal(t, tt.wantMsg, err.Error())
			// IsZeroValueError detection
			tt.wantIsZeroVal(t, IsZeroValueError(err))
			// negative cases
			assert.False(t, IsZeroValueError(nil))
			assert.False(t, IsZeroValueError(errors.New("generic")))
		})
	}

	// Multi-level wrapping should still be detected
	wrapped := fmt.Errorf("outer: %w", ZeroValueError("key must not be zero value"))
	assert.True(t, IsZeroValueError(wrapped))
}

func TestBusClosedError(t *testing.T) {
	err := BusClosedError()

	// Test that IsBusClosedError correctly identifies the error
	assert.True(t, IsBusClosedError(err))
	assert.Equal(t, "eventbus: bus is closed", err.Error())

	// Test that nil is not identified as BusClosedError
	assert.False(t, IsBusClosedError(nil))

	// Test that generic errors are not identified as BusClosedError
	assert.False(t, IsBusClosedError(errors.New("generic error")))

	// Test wrapped BusClosedError is still detected
	wrappedErr := fmt.Errorf("operation failed: %w", err)
	assert.True(t, IsBusClosedError(wrappedErr))

	// Test that errors.As works correctly
	var target *busClosedError
	assert.True(t, errors.As(err, &target))
	assert.Equal(t, "eventbus: bus is closed", target.Error())
}

func TestNilHandlerError(t *testing.T) {
	err := NilHandlerError()

	// Test that IsNilHandlerError correctly identifies the error
	assert.True(t, IsNilHandlerError(err))
	assert.Equal(t, "eventbus: handler cannot be nil", err.Error())

	// Test that nil is not identified as NilHandlerError
	assert.False(t, IsNilHandlerError(nil))

	// Test that generic errors are not identified as NilHandlerError
	assert.False(t, IsNilHandlerError(errors.New("generic error")))

	// Test wrapped NilHandlerError is still detected
	wrappedErr := fmt.Errorf("subscription failed: %w", err)
	assert.True(t, IsNilHandlerError(wrappedErr))

	// Test that errors.As works correctly
	var target *nilHandlerError
	assert.True(t, errors.As(err, &target))
	assert.Equal(t, "eventbus: handler cannot be nil", target.Error())
}

func TestAutogrowPolicyInUseError(t *testing.T) {
	tests := []struct {
		name        string
		policyName  string
		volumeCount int
		volumes     []string
		expectedMsg string
		testIsFunc  bool
	}{
		{
			name:        "Single volume using policy",
			policyName:  "gold-policy",
			volumeCount: 1,
			volumes:     []string{"vol1"},
			expectedMsg: "cannot delete policy gold-policy: in use by 1 volume(s): [vol1]",
			testIsFunc:  true,
		},
		{
			name:        "Multiple volumes using policy",
			policyName:  "silver-policy",
			volumeCount: 3,
			volumes:     []string{"vol1", "vol2", "vol3"},
			expectedMsg: "cannot delete policy silver-policy: in use by 3 volume(s): [vol1 vol2 vol3]",
			testIsFunc:  true,
		},
		{
			name:        "No volumes (count zero)",
			policyName:  "bronze-policy",
			volumeCount: 0,
			volumes:     []string{},
			expectedMsg: "cannot delete policy bronze-policy: in use by 0 volume(s): []",
			testIsFunc:  true,
		},
		{
			name:        "Empty policy name",
			policyName:  "",
			volumeCount: 1,
			volumes:     []string{"vol1"},
			expectedMsg: "cannot delete policy : in use by 1 volume(s): [vol1]",
			testIsFunc:  true,
		},
		{
			name:        "Nil volumes slice",
			policyName:  "test-policy",
			volumeCount: 0,
			volumes:     nil,
			expectedMsg: "cannot delete policy test-policy: in use by 0 volume(s): []",
			testIsFunc:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AutogrowPolicyInUseError(tt.policyName, tt.volumeCount, tt.volumes)

			assert.Equal(t, tt.expectedMsg, err.Error())

			if tt.testIsFunc {
				assert.True(t, IsAutogrowPolicyInUseError(err), "IsAutogrowPolicyInUseError should detect this error")
			}
		})
	}

	// Test negative cases for IsAutogrowPolicyInUseError
	t.Run("IsAutogrowPolicyInUseError_NilError", func(t *testing.T) {
		assert.False(t, IsAutogrowPolicyInUseError(nil))
	})

	t.Run("IsAutogrowPolicyInUseError_DifferentErrorType", func(t *testing.T) {
		assert.False(t, IsAutogrowPolicyInUseError(errors.New("generic error")))
		assert.False(t, IsAutogrowPolicyInUseError(NotFoundError("not found")))
	})

	// Test wrapped error detection
	t.Run("IsAutogrowPolicyInUseError_WrappedError", func(t *testing.T) {
		originalErr := AutogrowPolicyInUseError("test-policy", 2, []string{"vol1", "vol2"})
		wrappedErr := fmt.Errorf("operation failed: %w", originalErr)

		assert.True(t, IsAutogrowPolicyInUseError(wrappedErr), "Should detect wrapped autogrow policy in use error")
	})
}

func TestAutogrowPolicyNotUsableError(t *testing.T) {
	tests := []struct {
		name        string
		policyName  string
		state       string
		expectedMsg string
		testIsFunc  bool
	}{
		{
			name:        "Policy in Failed state",
			policyName:  "gold-policy",
			state:       "Failed",
			expectedMsg: "autogrow policy 'gold-policy' exists but is in 'Failed' state and cannot be used",
			testIsFunc:  true,
		},
		{
			name:        "Policy in Deleting state",
			policyName:  "silver-policy",
			state:       "Deleting",
			expectedMsg: "autogrow policy 'silver-policy' exists but is in 'Deleting' state and cannot be used",
			testIsFunc:  true,
		},
		{
			name:        "Empty policy name",
			policyName:  "",
			state:       "Failed",
			expectedMsg: "autogrow policy '' exists but is in 'Failed' state and cannot be used",
			testIsFunc:  true,
		},
		{
			name:        "Empty state",
			policyName:  "test-policy",
			state:       "",
			expectedMsg: "autogrow policy 'test-policy' exists but is in '' state and cannot be used",
			testIsFunc:  true,
		},
		{
			name:        "Custom state value",
			policyName:  "custom-policy",
			state:       "CustomState",
			expectedMsg: "autogrow policy 'custom-policy' exists but is in 'CustomState' state and cannot be used",
			testIsFunc:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AutogrowPolicyNotUsableError(tt.policyName, tt.state)

			// Verify error message
			assert.Equal(t, tt.expectedMsg, err.Error())

			// Verify IsAutogrowPolicyNotUsableError detects it
			if tt.testIsFunc {
				assert.True(t, IsAutogrowPolicyNotUsableError(err), "IsAutogrowPolicyNotUsableError should detect this error")
			}
		})
	}

	// Test negative cases for IsAutogrowPolicyNotUsableError
	t.Run("IsAutogrowPolicyNotUsableError_NilError", func(t *testing.T) {
		assert.False(t, IsAutogrowPolicyNotUsableError(nil))
	})

	t.Run("IsAutogrowPolicyNotUsableError_DifferentErrorType", func(t *testing.T) {
		assert.False(t, IsAutogrowPolicyNotUsableError(errors.New("generic error")))
		assert.False(t, IsAutogrowPolicyNotUsableError(NotFoundError("not found")))
		assert.False(t, IsAutogrowPolicyNotUsableError(AutogrowPolicyInUseError("policy", 1, []string{"vol1"})))
	})

	// Test wrapped error detection
	t.Run("IsAutogrowPolicyNotUsableError_WrappedError", func(t *testing.T) {
		originalErr := AutogrowPolicyNotUsableError("test-policy", "Failed")
		wrappedErr := fmt.Errorf("validation failed: %w", originalErr)

		assert.True(t, IsAutogrowPolicyNotUsableError(wrappedErr), "Should detect wrapped autogrow policy not usable error")
	})
}

func TestAutogrowPolicyNotFoundError(t *testing.T) {
	tests := []struct {
		name           string
		policyName     string
		expectedSubstr string
	}{
		{
			name:           "Simple policy name",
			policyName:     "test-policy",
			expectedSubstr: "autogrow policy 'test-policy' not found",
		},
		{
			name:           "Policy with special characters",
			policyName:     "my-policy-123",
			expectedSubstr: "autogrow policy 'my-policy-123' not found",
		},
		{
			name:           "Empty policy name",
			policyName:     "",
			expectedSubstr: "autogrow policy '' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AutogrowPolicyNotFoundError(tt.policyName)

			// Verify error message contains expected content
			assert.Contains(t, err.Error(), tt.expectedSubstr)

			// Verify IsAutogrowPolicyNotFoundError detects it
			assert.True(t, IsAutogrowPolicyNotFoundError(err), "IsAutogrowPolicyNotFoundError should detect this error")
		})
	}

	// Test negative cases for IsAutogrowPolicyNotFoundError
	t.Run("IsAutogrowPolicyNotFoundError_NilError", func(t *testing.T) {
		assert.False(t, IsAutogrowPolicyNotFoundError(nil))
	})

	t.Run("IsAutogrowPolicyNotFoundError_DifferentErrorType", func(t *testing.T) {
		assert.False(t, IsAutogrowPolicyNotFoundError(errors.New("generic error")))
		assert.False(t, IsAutogrowPolicyNotFoundError(NotFoundError("not found")))
		assert.False(t, IsAutogrowPolicyNotFoundError(AutogrowPolicyInUseError("policy", 1, []string{"vol1"})))
		assert.False(t, IsAutogrowPolicyNotFoundError(AutogrowPolicyNotUsableError("policy", "Failed")))
	})

	// Test wrapped error detection
	t.Run("IsAutogrowPolicyNotFoundError_WrappedError", func(t *testing.T) {
		originalErr := AutogrowPolicyNotFoundError("test-policy")
		wrappedErr := fmt.Errorf("lookup failed: %w", originalErr)

		assert.True(t, IsAutogrowPolicyNotFoundError(wrappedErr), "Should detect wrapped autogrow policy not found error")
	})
}
