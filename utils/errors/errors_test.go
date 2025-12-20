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
