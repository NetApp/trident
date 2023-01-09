// Copyright 2021 NetApp, Inc. All Rights Reserved.

package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedCapacityRangeError(t *testing.T) {
	// test setup
	err := fmt.Errorf("a generic error")
	unsupportedCapacityRangeErr := UnsupportedCapacityRangeError(fmt.Errorf(
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

func TestAsInvalidJSONError(t *testing.T) {
	unmarshalTypeErr := &json.UnmarshalTypeError{
		Value:  "",
		Type:   reflect.TypeOf(fmt.Errorf("foo")),
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
	resExhaustedErr := ResourceExhaustedError(fmt.Errorf("volume limit reached"))

	tests := []struct {
		Name    string
		Err     error
		wantErr assert.BoolAssertionFunc
	}{
		{
			Name:    "NotResourceExhaustedError",
			Err:     fmt.Errorf("a generic error"),
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
