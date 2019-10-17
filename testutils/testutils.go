// Copyright 2019 NetApp, Inc. All Rights Reserved.

// Package testutils provides some basic test routines that were proliferating
package testutils

import "testing"

// AssertFalse produces an error if the passed-in condition is not false
func AssertFalse(t *testing.T, errorMessage string, booleanCondition bool) {
	if booleanCondition {
		t.Errorf(errorMessage)
	}
}

// AssertFalse produces an error if the passed-in condition is false
func AssertTrue(t *testing.T, errorMessage string, booleanCondition bool) {
	if !booleanCondition {
		t.Errorf(errorMessage)
	}
}

// AssertFalse produces an error if the passed-in objects differ
func AssertEqual(t *testing.T, errorMessage string, obj1, obj2 interface{}) {
	if obj1 != obj2 {
		t.Errorf("%s: '%v' != '%v'", errorMessage, obj1, obj2)
		return
	}
}

// AssertNotEqual produces an error if the passed-in objects are equal
func AssertNotEqual(t *testing.T, errorMessage string, obj1, obj2 interface{}) {
	if obj1 == obj2 {
		t.Errorf("%s: '%v' == '%v'", errorMessage, obj1, obj2)
		return
	}
}

// AssertFalse produces an error if a deep inspection of the passed-in objects differ
func AssertAllEqual(t *testing.T, errorMessage string, objs ...interface{}) {
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
