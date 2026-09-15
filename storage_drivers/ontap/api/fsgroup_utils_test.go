// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseUnixGroupID(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int64
		expectError bool
	}{
		{name: "empty string treated as not set", input: "", expected: 0, expectError: false},
		{name: "minimum valid GID", input: "1", expected: 1, expectError: false},
		{name: "typical GID", input: "1000", expected: 1000, expectError: false},
		{name: "maximum valid GID", input: "4294967294", expected: 4294967294, expectError: false},
		{name: "zero is out of range", input: "0", expected: 0, expectError: true},
		{name: "above maximum is out of range", input: "4294967295", expected: 0, expectError: true},
		{name: "negative is out of range", input: "-1", expected: 0, expectError: true},
		{name: "non-numeric returns error", input: "abc", expected: 0, expectError: true},
		{name: "whitespace returns error", input: " 100 ", expected: 0, expectError: true},
		{name: "float returns error", input: "100.5", expected: 0, expectError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gid, err := parseUnixGroupID(test.input)
			if test.expectError {
				assert.Error(t, err, "expected an error for input %q", test.input)
				assert.Equal(t, int64(0), gid, "expected zero GID on error")
			} else {
				assert.NoError(t, err, "unexpected error for input %q", test.input)
				assert.Equal(t, test.expected, gid, "unexpected GID for input %q", test.input)
			}
		})
	}
}

func TestParseUnixGroupIDInt(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int
		expectError bool
	}{
		{name: "empty string treated as not set", input: "", expected: 0, expectError: false},
		{name: "minimum valid GID", input: "1", expected: 1, expectError: false},
		{name: "typical GID", input: "1000", expected: 1000, expectError: false},
		{name: "maximum valid GID", input: "4294967294", expected: 4294967294, expectError: false},
		{name: "zero is out of range", input: "0", expected: 0, expectError: true},
		{name: "above maximum is out of range", input: "4294967295", expected: 0, expectError: true},
		{name: "negative is out of range", input: "-1", expected: 0, expectError: true},
		{name: "non-numeric returns error", input: "abc", expected: 0, expectError: true},
		{name: "whitespace returns error", input: " 100 ", expected: 0, expectError: true},
		{name: "float returns error", input: "100.5", expected: 0, expectError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gid, err := parseUnixGroupIDInt(test.input)
			if test.expectError {
				assert.Error(t, err, "expected an error for input %q", test.input)
				assert.Equal(t, 0, gid, "expected zero GID on error")
			} else {
				assert.NoError(t, err, "unexpected error for input %q", test.input)
				assert.Equal(t, test.expected, gid, "unexpected GID for input %q", test.input)
			}
		})
	}
}
