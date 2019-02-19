// Copyright 2019 NetApp, Inc. All Rights Reserved.

package storage

import (
	"testing"
)

func assertFalse(t *testing.T, errorMessage string, booleanCondition bool) {
	if booleanCondition {
		t.Errorf(errorMessage)
	}
}

func assertTrue(t *testing.T, errorMessage string, booleanCondition bool) {
	if !booleanCondition {
		t.Errorf(errorMessage)
	}
}

func assertEqual(t *testing.T, errorMessage string, obj1, obj2 interface{}) {
	if obj1 != obj2 {
		t.Errorf("%s: '%v' != '%v'", errorMessage, obj1, obj2)
		return
	}
}

func assertAllEqual(t *testing.T, errorMessage string, objs ...interface{}) {
	allEqual := true
	for i, obj := range objs {
		if i > 0 {
			if obj != objs[i-1] {
				allEqual = false
				t.Errorf("%s:  objs[%v] '%v' != '%v' objs[%v]", errorMessage, i-1, objs[i-1], objs[i], i)
				return
			}
		}
	}

	if !allEqual {
		t.Errorf(errorMessage)
	}
}

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
		t.Logf("Running test case '%s'", testName)

		assertEqual(t, "Strings not equal", test.input.String(), test.output)
		assertTrue(t, "Predicate failed", test.predicate(test.input))
	}
}
