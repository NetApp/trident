// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestParseUnixGroupID(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedGID int
		expectedSet bool
		expectError bool
	}{
		{name: "Empty means not set", input: "", expectedGID: 0, expectedSet: false, expectError: false},
		{name: "Zero (root group)", input: "0", expectedGID: 0, expectedSet: true, expectError: false},
		{name: "Typical GID", input: "1234", expectedGID: 1234, expectedSet: true, expectError: false},
		{name: "Maximum GID", input: "2147483647", expectedGID: MaxUnixGroupID, expectedSet: true, expectError: false},
		{name: "Above maximum", input: "2147483648", expectError: true},
		{name: "Negative", input: "-1", expectError: true},
		{name: "Non-numeric", input: "abc", expectError: true},
		{name: "Trailing space", input: "12 ", expectError: true},
		{name: "Float", input: "12.0", expectError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gid, set, err := ParseUnixGroupID(test.input)
			if test.expectError {
				assert.Error(t, err, "expected an error for input %q", test.input)
				assert.True(t, errors.IsInvalidInputError(err), "expected an InvalidInputError")
				assert.False(t, set, "set should be false on error")
				return
			}
			assert.NoError(t, err, "did not expect an error for input %q", test.input)
			assert.Equal(t, test.expectedSet, set, "unexpected set flag")
			assert.Equal(t, test.expectedGID, gid, "unexpected GID")
		})
	}
}
