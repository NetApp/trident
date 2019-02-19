// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
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

func TestGetError(t *testing.T) {
	e := GetError(nil, nil)

	assertEqual(t, "Strings not equal",
		"failed", e.(ZapiError).Status())

	assertEqual(t, "Strings not equal",
		azgo.EINTERNALERROR, e.(ZapiError).Code())

	assertEqual(t, "Strings not equal",
		"unexpcted nil ZAPI result", e.(ZapiError).Reason())
}
