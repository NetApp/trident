// Copyright 2019 NetApp, Inc. All Rights Reserved.

package storage

import (
	"testing"
)

func TestVolumeState(t *testing.T) {

	tests := map[string]struct {
		input     VolumeState
		output    string
		predicate func(VolumeState) bool
	}{
		"Unknown state (bad)": {
			input:  "",
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Unknown state": {
			input:  VolumeStateUnknown,
			output: "unknown",
			predicate: func(input VolumeState) bool {
				return input.IsUnknown()
			},
		},
		"Online state": {
			input:  VolumeStateOnline,
			output: "online",
			predicate: func(input VolumeState) bool {
				return input.IsOnline()
			},
		},
		"Deleting state": {
			input:  VolumeStateDeleting,
			output: "deleting",
			predicate: func(input VolumeState) bool {
				return input.IsDeleting()
			},
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)

		assertEqual(t, "Strings not equal", test.input.String(), test.output)
		assertTrue(t, "Predicate failed", test.predicate(test.input))
	}
}
