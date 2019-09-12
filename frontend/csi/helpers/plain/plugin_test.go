// Copyright 2019 NetApp, Inc. All Rights Reserved.

package plain

import (
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/testutils"
)

func TestCombineAccessModes(t *testing.T) {
	var accessModesTests = []struct {
		accessModes []config.AccessMode
		expected    config.AccessMode
	}{
		{[]config.AccessMode{config.ModeAny, config.ModeAny}, config.ModeAny},
		{[]config.AccessMode{config.ModeAny, config.ReadWriteOnce}, config.ReadWriteOnce},
		{[]config.AccessMode{config.ModeAny, config.ReadOnlyMany}, config.ReadOnlyMany},
		{[]config.AccessMode{config.ModeAny, config.ReadWriteMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteOnce, config.ModeAny}, config.ReadWriteOnce},
		{[]config.AccessMode{config.ReadWriteOnce, config.ReadWriteOnce}, config.ReadWriteOnce},
		{[]config.AccessMode{config.ReadWriteOnce, config.ReadOnlyMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteOnce, config.ReadWriteMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ModeAny}, config.ReadOnlyMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ReadWriteOnce}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ReadOnlyMany}, config.ReadOnlyMany},
		{[]config.AccessMode{config.ReadOnlyMany, config.ReadWriteMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ModeAny}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ReadWriteOnce}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ReadOnlyMany}, config.ReadWriteMany},
		{[]config.AccessMode{config.ReadWriteMany, config.ReadWriteMany}, config.ReadWriteMany},
	}

	for _, tc := range accessModesTests {
		accessMode := combineAccessModes(tc.accessModes)
		testutils.AssertEqual(t, "Access Modes not combining as expected!", tc.expected, accessMode)
	}
}
