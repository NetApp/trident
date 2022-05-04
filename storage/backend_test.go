// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackendState(t *testing.T) {
	tests := map[string]struct {
		input     BackendState
		output    string
		predicate func(BackendState) bool
	}{
		"Unknown state (bad)": {
			input:  "",
			output: "unknown",
			predicate: func(input BackendState) bool {
				return input.IsUnknown()
			},
		},
		"Unknown state": {
			input:  Unknown,
			output: "unknown",
			predicate: func(input BackendState) bool {
				return input.IsUnknown()
			},
		},
		"Online state": {
			input:  Online,
			output: "online",
			predicate: func(input BackendState) bool {
				return input.IsOnline()
			},
		},
		"Offline state": {
			input:  Offline,
			output: "offline",
			predicate: func(input BackendState) bool {
				return input.IsOffline()
			},
		},
		"Deleting state": {
			input:  Deleting,
			output: "deleting",
			predicate: func(input BackendState) bool {
				return input.IsDeleting()
			},
		},
		"Failed state": {
			input:  Failed,
			output: "failed",
			predicate: func(input BackendState) bool {
				return input.IsFailed()
			},
		},
	}
	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				assert.Equal(t, test.input.String(), test.output, "Strings not equal")
				assert.True(t, test.predicate(test.input), "Predicate failed")
			},
		)
	}
}
