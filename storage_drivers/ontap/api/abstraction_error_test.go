// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants for error messages
const (
	testErrorMessage = "test error message"
	emptyMessage     = ""
	unicodeMessage   = "测试错误消息 error ñoño"
	specialCharsMsg  = "error with special chars: \n\t\r\"'\\"
)

var longMessage = strings.Repeat("very long error message ", 100)

// errorTestCase represents a test case for error types
type errorTestCase struct {
	name          string
	createFunc    func(string) error
	checkFunc     func(error) bool
	errorTypeName string
}

// getAllErrorTestCases returns test cases for all error types
func getAllErrorTestCases() []errorTestCase {
	return []errorTestCase{
		{
			name:          "VolumeCreateJobExistsError",
			createFunc:    VolumeCreateJobExistsError,
			checkFunc:     IsVolumeCreateJobExistsError,
			errorTypeName: "*api.volumeCreateJobExistsError",
		},
		{
			name:          "VolumeReadError",
			createFunc:    VolumeReadError,
			checkFunc:     IsVolumeReadError,
			errorTypeName: "*api.volumeReadError",
		},
		{
			name:          "VolumeIdAttributesReadError",
			createFunc:    VolumeIdAttributesReadError,
			checkFunc:     IsVolumeIdAttributesReadError,
			errorTypeName: "*api.volumeIdAttributesReadError",
		},
		{
			name:          "VolumeSpaceAttributesReadError",
			createFunc:    VolumeSpaceAttributesReadError,
			checkFunc:     IsVolumeSpaceAttributesReadError,
			errorTypeName: "*api.volumeSpaceAttributesReadError",
		},
		{
			name:          "SnapshotBusyError",
			createFunc:    SnapshotBusyError,
			checkFunc:     IsSnapshotBusyError,
			errorTypeName: "*api.snapshotBusyError",
		},
		{
			name:          "ApiError",
			createFunc:    ApiError,
			checkFunc:     IsApiError,
			errorTypeName: "*api.apiError",
		},
		{
			name:          "NotFoundError",
			createFunc:    NotFoundError,
			checkFunc:     IsNotFoundError,
			errorTypeName: "*api.notFoundError",
		},
		{
			name:          "NotReadyError",
			createFunc:    NotReadyError,
			checkFunc:     IsNotReadyError,
			errorTypeName: "*api.notReadyError",
		},
		{
			name:          "TooManyLunsError",
			createFunc:    TooManyLunsError,
			checkFunc:     IsTooManyLunsError,
			errorTypeName: "*api.tooManyLunsError",
		},
	}
}

// =============================================================================
// Error Type Tests
// These tests validate the creation, behavior, and type checking of custom error types.
// =============================================================================

// TestErrorCreationWithVariousMessages validates error creation and message preservation for all error types
func TestErrorCreationWithVariousMessages(t *testing.T) {
	errorTypes := getAllErrorTestCases()
	testMessages := []struct {
		name string
		msg  string
	}{
		{"Basic", testErrorMessage},
		{"Empty", emptyMessage},
		{"Long", longMessage},
		{"Unicode", unicodeMessage},
		{"SpecialChars", specialCharsMsg},
	}

	for _, errorType := range errorTypes {
		t.Run(errorType.name, func(t *testing.T) {
			for _, testMsg := range testMessages {
				t.Run("Message"+testMsg.name, func(t *testing.T) {
					err := errorType.createFunc(testMsg.msg)
					require.Error(t, err)
					assert.Equal(t, testMsg.msg, err.Error())
				})
			}
		})
	}
}

// TestErrorTypeChecking validates type checking functionality for all error types
func TestErrorTypeChecking(t *testing.T) {
	errorTypes := getAllErrorTestCases()

	typeCheckTests := []struct {
		name     string
		getError func(func(string) error) error
		expected bool
	}{
		{"Positive", func(createFunc func(string) error) error { return createFunc(testErrorMessage) }, true},
		{"Negative", func(createFunc func(string) error) error { return errors.New("different error") }, false},
		{"Nil", func(createFunc func(string) error) error { return nil }, false},
	}

	for _, errorType := range errorTypes {
		t.Run(errorType.name, func(t *testing.T) {
			for _, tc := range typeCheckTests {
				t.Run("TypeCheck"+tc.name, func(t *testing.T) {
					err := tc.getError(errorType.createFunc)
					assert.Equal(t, tc.expected, errorType.checkFunc(err))
				})
			}
		})
	}
}

// TestErrorTypeNamesAndInterfaces validates error type names and interface implementation
func TestErrorTypeNamesAndInterfaces(t *testing.T) {
	errorTypes := getAllErrorTestCases()

	for _, errorType := range errorTypes {
		t.Run(errorType.name, func(t *testing.T) {
			err := errorType.createFunc(testErrorMessage)
			assert.Equal(t, errorType.errorTypeName, reflect.TypeOf(err).String())
			assert.Implements(t, (*error)(nil), err)
		})
	}
}

// TestErrorTypeDifferentiation ensures that different error types are not confused with each other.
func TestErrorTypeDifferentiation(t *testing.T) {
	testCases := getAllErrorTestCases()

	t.Run("ComprehensiveErrorTypeDifferentiation", func(t *testing.T) {
		// Create instances of all error types
		errorInstances := make([]error, len(testCases))
		for i, tc := range testCases {
			errorInstances[i] = tc.createFunc(testErrorMessage)
		}

		// Test that each error instance only matches its own type checker
		for i, err := range errorInstances {
			for j, tc := range testCases {
				if i == j {
					// Should match its own type
					assert.True(t, tc.checkFunc(err),
						"Error type %s should be recognized by its own checker", tc.name)
				} else {
					// Should not match other types
					assert.False(t, tc.checkFunc(err),
						"Error type %s should not be recognized by %s checker",
						testCases[i].name, tc.name)
				}
			}
		}
	})

	t.Run("CrossTypePointerComparison", func(t *testing.T) {
		// Test that same error instance always matches itself
		err1 := VolumeReadError(testErrorMessage)
		err2 := VolumeReadError(testErrorMessage)

		// Same type but different instances
		assert.True(t, IsVolumeReadError(err1))
		assert.True(t, IsVolumeReadError(err2))

		// Different instances are not equal (pointer comparison)
		// Note: Go error types with same message are still different instances
		assert.NotSame(t, err1, err2, "Different error instances should not be the same pointer")

		// But same instance is equal to itself
		assert.Same(t, err1, err1, "Same error instance should be identical to itself")
		assert.Same(t, err2, err2, "Same error instance should be identical to itself")
	})
}

// TestErrorConstants verifies that error constants are defined correctly and have expected values.
func TestErrorConstants(t *testing.T) {
	constantTests := []struct {
		name     string
		actual   string
		expected string
	}{
		{"ENTRY_DOESNT_EXIST", ENTRY_DOESNT_EXIST, "4"},
		{"DUPLICATE_ENTRY", DUPLICATE_ENTRY, "1"},
		{"INVALID_ENTRY", INVALID_ENTRY, "655446"},
		{"DP_VOLUME_NOT_INITIALIZED", DP_VOLUME_NOT_INITIALIZED, "917536"},
		{"SNAPMIRROR_TRANSFER_IN_PROGRESS", SNAPMIRROR_TRANSFER_IN_PROGRESS, "13303812"},
		{"SNAPMIRROR_TRANSFER_IN_PROGRESS_BROKEN_OFF", SNAPMIRROR_TRANSFER_IN_PROGRESS_BROKEN_OFF, "13303808"},
		{"SNAPMIRROR_MODIFICATION_IN_PROGRESS", SNAPMIRROR_MODIFICATION_IN_PROGRESS, "13303822"},
		{"LUN_MAP_EXIST_ERROR", LUN_MAP_EXIST_ERROR, "5374922"},
		{"FLEXGROUP_VOLUME_SIZE_ERROR_REST", FLEXGROUP_VOLUME_SIZE_ERROR_REST, "917534"},
		{"EXPORT_POLICY_NOT_FOUND", EXPORT_POLICY_NOT_FOUND, "1703954"},
		{"EXPORT_POLICY_RULE_EXISTS", EXPORT_POLICY_RULE_EXISTS, "1704070"},
		{"CONSISTENCY_GROUP_SNAP_EXISTS_ERROR", CONSISTENCY_GROUP_SNAP_EXISTS_ERROR, "53411921"},
	}

	for _, tc := range constantTests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.actual, "Constant %s should have value %s", tc.name, tc.expected)
			assert.NotEmpty(t, tc.actual, "Constant %s should not be empty", tc.name)

			// Verify constant is numeric
			for _, char := range tc.actual {
				assert.True(t, char >= '0' && char <= '9',
					"Constant '%s' should only contain numeric characters, found '%c'", tc.actual, char)
			}
		})
	}
}

// TestErrorWrapping tests the behavior of wrapped errors, including compatibility with errors.Is.
func TestErrorWrapping(t *testing.T) {
	errorTypes := getAllErrorTestCases()

	// Table-driven test cases for different wrapping scenarios
	wrappingTests := []struct {
		name         string
		wrapFunc     func(error) error
		shouldMatch  bool
		testErrorsIs bool
		description  string
	}{
		{
			name:         "BasicWrapping",
			wrapFunc:     func(err error) error { return errors.New("wrapped: " + err.Error()) },
			shouldMatch:  false,
			testErrorsIs: false,
			description:  "errors.New() creates new unrelated error",
		},
		{
			name:         "FmtErrorf",
			wrapFunc:     func(err error) error { return fmt.Errorf("context: %w", err) },
			shouldMatch:  false,
			testErrorsIs: true,
			description:  "fmt.Errorf with %w creates proper wrapping",
		},
		{
			name: "MultiLevel",
			wrapFunc: func(err error) error {
				l1 := fmt.Errorf("level 1: %w", err)
				l2 := fmt.Errorf("level 2: %w", l1)
				return fmt.Errorf("level 3: %w", l2)
			},
			shouldMatch:  false,
			testErrorsIs: true,
			description:  "Multiple levels of wrapping with %w",
		},
	}

	// Test each wrapping scenario against all error types
	for _, errorType := range errorTypes {
		t.Run(errorType.name, func(t *testing.T) {
			for _, wrapTest := range wrappingTests {
				t.Run(wrapTest.name, func(t *testing.T) {
					originalErr := errorType.createFunc(testErrorMessage)
					wrappedErr := wrapTest.wrapFunc(originalErr)

					// Type checker should not recognize wrapped errors
					assert.Equal(t, wrapTest.shouldMatch, errorType.checkFunc(wrappedErr),
						"Wrapped error type checking failed for %s: %s", wrapTest.name, wrapTest.description)

					// Original error should still be recognized
					assert.True(t, errorType.checkFunc(originalErr),
						"Original error should still be recognized by its type checker")

					// Test errors.Is if applicable
					if wrapTest.testErrorsIs {
						assert.True(t, errors.Is(wrappedErr, originalErr),
							"errors.Is should find original error in wrapped error for %s", wrapTest.name)
					}
				})
			}
		})
	}
}

// TestErrorChaining tests specific error chaining scenarios
func TestErrorChaining(t *testing.T) {
	t.Run("CombinedErrorChaining", func(t *testing.T) {
		err1 := VolumeReadError("first error")
		err2 := NotFoundError("second error")
		chainedErr := fmt.Errorf("combined errors: %w and also %s", err1, err2.Error())

		// Chained error should not match specific type checkers
		assert.False(t, IsVolumeReadError(chainedErr),
			"Chained error should not be recognized as VolumeReadError")
		assert.False(t, IsNotFoundError(chainedErr),
			"Chained error should not be recognized as NotFoundError")

		// But errors.Is should find the wrapped error
		assert.True(t, errors.Is(chainedErr, err1),
			"errors.Is should find the wrapped VolumeReadError in chained error")
	})

	t.Run("NestedErrorChaining", func(t *testing.T) {
		baseErr := ApiError("base error")
		level1 := fmt.Errorf("level 1: %w", baseErr)
		level2 := fmt.Errorf("level 2: %w", level1)
		level3 := fmt.Errorf("level 3: %w", level2)

		// Deep nesting should still allow errors.Is to find the original
		assert.True(t, errors.Is(level3, baseErr),
			"errors.Is should find base error through multiple wrapping levels")
		assert.False(t, IsApiError(level3),
			"Type checker should not recognize deeply wrapped error")
	})
}

// TestErrorBehaviorEdgeCases covers edge cases and unusual scenarios for error handling.
func TestErrorBehaviorEdgeCases(t *testing.T) {
	errorTypes := getAllErrorTestCases()

	// Test nil error handling for all types
	t.Run("NilErrorHandling", func(t *testing.T) {
		for _, errorType := range errorTypes {
			assert.False(t, errorType.checkFunc(nil), "%s type checker should return false for nil", errorType.name)
		}
	})

	// Test type assertion safety
	t.Run("TypeAssertionSafety", func(t *testing.T) {
		var err error = VolumeReadError(testErrorMessage)
		assert.NotPanics(t, func() { IsVolumeCreateJobExistsError(err) })
	})

	// Test message preservation with various message types
	t.Run("ErrorMessagePreservation", func(t *testing.T) {
		testMessages := []string{"", "simple message", longMessage, unicodeMessage, specialCharsMsg, "message with\nnewlines\tand\ttabs"}

		for _, errorType := range errorTypes {
			for _, msg := range testMessages {
				err := errorType.createFunc(msg)
				assert.Equal(t, msg, err.Error(), "Error message should be preserved exactly for %s", errorType.name)
			}
		}
	})
}

// TestErrorConcurrency tests concurrent error creation and type checking for thread safety.
func TestErrorConcurrency(t *testing.T) {
	t.Run("ConcurrentErrorCreation", func(t *testing.T) {
		const numGoroutines, numIterations = 100, 10
		var wg sync.WaitGroup

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numIterations; j++ {
					msg := fmt.Sprintf("concurrent error %d-%d", id, j)
					err := VolumeReadError(msg)
					assert.True(t, IsVolumeReadError(err))
					assert.Equal(t, msg, err.Error())
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("ConcurrentTypeChecking", func(t *testing.T) {
		const numGoroutines, numIterations = 50, 100
		errorTypes := getAllErrorTestCases()
		var wg sync.WaitGroup

		// Pre-create errors for testing
		testErrors := make([]error, len(errorTypes))
		for i, errorType := range errorTypes {
			testErrors[i] = errorType.createFunc(fmt.Sprintf("test error %d", i))
		}

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numIterations; j++ {
					// Test each error against each type checker
					for errIdx, err := range testErrors {
						for checkerIdx, errorType := range errorTypes {
							expected := errIdx == checkerIdx
							result := errorType.checkFunc(err)
							assert.Equal(t, expected, result,
								"Concurrent type check failed: error %d vs checker %d", errIdx, checkerIdx)
						}
					}
				}
			}(i)
		}

		wg.Wait()
	})
}
