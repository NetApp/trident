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

func TestNotFoundError(t *testing.T) {
	err := NotFoundError("not found error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("not found error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsNotFoundError(err))

	assert.False(t, IsNotFoundError(nil))

	err = NotFoundError("")
	assert.True(t, IsNotFoundError(err))

	// Test wrapping
	err = WrapWithNotFoundError(errors.New("not a not found err"), "not found")
	assert.True(t, IsNotFoundError(err))
	assert.Equal(t, "not found; not a not found err", err.Error())

	err = WrapWithNotFoundError(nil, "not found")
	assert.Equal(t, "not found", err.Error())

	err = WrapWithNotFoundError(errors.New("not a not found err"), "")
	assert.True(t, IsNotFoundError(err))
	assert.Equal(t, "not a not found err", err.Error())

	err = WrapWithNotFoundError(errors.New(""), "not found")
	assert.True(t, IsNotFoundError(err))
	assert.Equal(t, "not found", err.Error())

	err = NotFoundError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsNotFoundError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = NotFoundError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsNotFoundError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestFoundError(t *testing.T) {
	err := FoundError("found error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("found error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsFoundError(err))

	assert.False(t, IsFoundError(nil))

	err = FoundError("")
	assert.True(t, IsFoundError(err))

	// Test wrapping
	err = WrapWithFoundError(errors.New("not a found err"), "found")
	assert.True(t, IsFoundError(err))
	assert.Equal(t, "found; not a found err", err.Error())

	err = WrapWithFoundError(nil, "found")
	assert.Equal(t, "found", err.Error())

	err = WrapWithFoundError(errors.New("not a found err"), "")
	assert.True(t, IsFoundError(err))
	assert.Equal(t, "not a found err", err.Error())

	err = WrapWithFoundError(errors.New(""), "found")
	assert.True(t, IsFoundError(err))
	assert.Equal(t, "found", err.Error())

	err = FoundError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsFoundError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = FoundError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsFoundError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestNotManagedError(t *testing.T) {
	err := NotManagedError("not managed error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("not managed error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsNotManagedError(err))

	assert.False(t, IsNotManagedError(nil))

	err = NotManagedError("")
	assert.True(t, IsNotManagedError(err))

	// Test wrapping
	err = WrapWithNotManagedError(errors.New("not a not managed err"), "not managed")
	assert.True(t, IsNotManagedError(err))
	assert.Equal(t, "not managed; not a not managed err", err.Error())

	err = WrapWithNotManagedError(nil, "not managed")
	assert.Equal(t, "not managed", err.Error())

	err = WrapWithNotManagedError(errors.New("not a not managed err"), "")
	assert.True(t, IsNotManagedError(err))
	assert.Equal(t, "not a not managed err", err.Error())

	err = WrapWithNotManagedError(errors.New(""), "not managed")
	assert.True(t, IsNotManagedError(err))
	assert.Equal(t, "not managed", err.Error())

	err = NotManagedError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsNotManagedError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = NotManagedError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsNotManagedError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestConnectionError(t *testing.T) {
	err := ConnectionError("connection error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("connection error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsConnectionError(err))

	assert.False(t, IsConnectionError(nil))

	err = ConnectionError("")
	assert.True(t, IsConnectionError(err))

	// Test wrapping
	err = WrapWithConnectionError(errors.New("not a connection err"), "not found")
	assert.True(t, IsConnectionError(err))
	assert.Equal(t, "not found; not a connection err", err.Error())

	err = WrapWithConnectionError(nil, "connection")
	assert.Equal(t, "connection", err.Error())

	err = WrapWithConnectionError(errors.New("not a connection err"), "")
	assert.True(t, IsConnectionError(err))
	assert.Equal(t, "not a connection err", err.Error())

	err = WrapWithConnectionError(errors.New(""), "connection")
	assert.True(t, IsConnectionError(err))
	assert.Equal(t, "connection", err.Error())

	err = ConnectionError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsConnectionError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = ConnectionError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsConnectionError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
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

func TestReconcileDeferredError(t *testing.T) {
	err := ReconcileDeferredError("deferred error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("deferred error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsReconcileDeferredError(err))

	assert.False(t, IsReconcileDeferredError(nil))

	err = ReconcileDeferredError("")
	assert.True(t, IsReconcileDeferredError(err))

	// Test wrapping
	err = WrapWithReconcileDeferredError(errors.New("not reconcile deferred err"), "reconcile deferred")
	assert.True(t, IsReconcileDeferredError(err))
	assert.Equal(t, "reconcile deferred; not reconcile deferred err", err.Error())

	err = WrapWithReconcileDeferredError(nil, "reconcile deferred")
	assert.Equal(t, "reconcile deferred", err.Error())

	err = WrapWithReconcileDeferredError(errors.New("not reconcile deferred err"), "")
	assert.True(t, IsReconcileDeferredError(err))
	assert.Equal(t, "not reconcile deferred err", err.Error())

	err = WrapWithReconcileDeferredError(errors.New(""), "reconcile deferred")
	assert.True(t, IsReconcileDeferredError(err))
	assert.Equal(t, "reconcile deferred", err.Error())

	err = ReconcileDeferredError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsReconcileDeferredError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = ReconcileDeferredError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsReconcileDeferredError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestReconcileIncompleteError(t *testing.T) {
	err := ReconcileIncompleteError("deferred error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("deferred error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsReconcileIncompleteError(err))

	assert.False(t, IsReconcileIncompleteError(nil))

	err = ReconcileIncompleteError("")
	assert.True(t, IsReconcileIncompleteError(err))

	// Test wrapping
	err = WrapWithReconcileIncompleteError(errors.New("not reconcile deferred err"), "reconcile deferred")
	assert.True(t, IsReconcileIncompleteError(err))
	assert.Equal(t, "reconcile deferred; not reconcile deferred err", err.Error())

	err = WrapWithReconcileIncompleteError(nil, "reconcile deferred")
	assert.Equal(t, "reconcile deferred", err.Error())

	err = WrapWithReconcileIncompleteError(errors.New("not reconcile deferred err"), "")
	assert.True(t, IsReconcileIncompleteError(err))
	assert.Equal(t, "not reconcile deferred err", err.Error())

	err = WrapWithReconcileIncompleteError(errors.New(""), "reconcile deferred")
	assert.True(t, IsReconcileIncompleteError(err))
	assert.Equal(t, "reconcile deferred", err.Error())

	err = ReconcileIncompleteError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsReconcileIncompleteError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = ReconcileIncompleteError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsReconcileIncompleteError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestUnsupportedConfigError(t *testing.T) {
	err := UnsupportedConfigError("error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsUnsupportedConfigError(err))

	assert.False(t, IsUnsupportedConfigError(nil))

	err = UnsupportedConfigError("")
	assert.True(t, IsUnsupportedConfigError(err))

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
}

func TestUnlicensedError(t *testing.T) {
	err := UnlicensedError("error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsUnlicensedError(err))

	assert.False(t, IsUnlicensedError(nil))

	err = UnlicensedError("")
	assert.True(t, IsUnlicensedError(err))

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
}

func TestReconcileFailedError(t *testing.T) {
	err := ReconcileFailedError("deferred error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("deferred error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsReconcileFailedError(err))

	assert.False(t, IsReconcileFailedError(nil))

	err = ReconcileFailedError("")
	assert.True(t, IsReconcileFailedError(err))

	// Test wrapping
	err = WrapWithReconcileFailedError(errors.New("not reconcile deferred err"), "reconcile deferred")
	assert.True(t, IsReconcileFailedError(err))
	assert.Equal(t, "reconcile deferred; not reconcile deferred err", err.Error())

	err = WrapWithReconcileFailedError(nil, "reconcile deferred")
	assert.Equal(t, "reconcile deferred", err.Error())

	err = WrapWithReconcileFailedError(errors.New("not reconcile deferred err"), "")
	assert.True(t, IsReconcileFailedError(err))
	assert.Equal(t, "not reconcile deferred err", err.Error())

	err = WrapWithReconcileFailedError(errors.New(""), "reconcile deferred")
	assert.True(t, IsReconcileFailedError(err))
	assert.Equal(t, "reconcile deferred", err.Error())

	err = ReconcileFailedError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsReconcileFailedError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = ReconcileFailedError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsReconcileFailedError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestFormatError(t *testing.T) {
	err := FormatError(errors.New("formatting error"))
	assert.True(t, IsFormatError(err))
	assert.Equal(t, "Formatting failed; formatting error", err.Error())
}

func TestIsFormatError(t *testing.T) {
	formatErr := &formatError{
		message: "format error",
	}

	tests := []struct {
		Name    string
		Err     error
		wantErr assert.BoolAssertionFunc
	}{
		{
			Name:    "NotFormatError",
			Err:     errors.New("a generic error"),
			wantErr: assert.False,
		},
		{
			Name:    "FormatError",
			Err:     formatErr,
			wantErr: assert.True,
		},
		{
			Name:    "WrappedFormatError",
			Err:     fmt.Errorf("wrapping formatError; %w", formatErr),
			wantErr: assert.True,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			tt.wantErr(t, IsFormatError(tt.Err), "Unexpected error")
		})
	}
}

func TestConflictError(t *testing.T) {
	err := ConflictError("conflict error with formatting %s, %s", "foo", "bar")
	assert.True(t, strings.Contains("conflict error with formatting foo, bar", err.Error()))

	err = errors.New("a generic error")
	assert.False(t, IsConflictError(err))

	assert.False(t, IsConflictError(nil))

	err = ConflictError("")
	assert.True(t, IsConflictError(err))

	// Test wrapping
	err = WrapWithConflictError(errors.New("not conflict err"), "conflict err")
	assert.True(t, IsConflictError(err))
	assert.Equal(t, "conflict err; not conflict err", err.Error())

	err = WrapWithConflictError(nil, "conflict err")
	assert.Equal(t, "conflict err", err.Error())

	err = WrapWithConflictError(errors.New("not reconcile deferred err"), "")
	assert.True(t, IsConflictError(err))
	assert.Equal(t, "not reconcile deferred err", err.Error())

	err = WrapWithConflictError(errors.New(""), "conflict err")
	assert.True(t, IsConflictError(err))
	assert.Equal(t, "conflict err", err.Error())

	err = ConflictError("")
	err = fmt.Errorf("custom message: %w", err)
	assert.True(t, IsConflictError(err))
	assert.Equal(t, "custom message: ", err.Error())

	// wrap multi levels deep
	err = ConflictError("")
	err = fmt.Errorf("outer; %w", fmt.Errorf("inner; %w", err))
	assert.True(t, IsConflictError(err))
	assert.Equal(t, "outer; inner; ", err.Error())
}

func TestMismatchedStorageClassError(t *testing.T) {
	err := MismatchedStorageClassError("clone volume test-clone from source volume test-source with different storage classes is not allowed")
	assert.True(t, strings.Contains("clone volume test-clone from source volume test-source with different storage classes is not allowed", err.Error()))

	err = errors.New("clone volume test-clone from source volume test-source with different storage classes is not allowed")
	assert.False(t, IsMismatchedStorageClassError(err))

	assert.False(t, IsMismatchedStorageClassError(nil))

	err = MismatchedStorageClassError("")
	assert.True(t, IsMismatchedStorageClassError(err))

	err = MismatchedStorageClassError("clone volume test-clone from source volume test-source with different storage classes is not allowed")
	assert.True(t, IsMismatchedStorageClassError(err))
	assert.Equal(t, "clone volume test-clone from source volume test-source with different storage classes is not allowed", err.Error())
}
