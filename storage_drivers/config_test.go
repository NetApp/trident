// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"fmt"
	"testing"

	"github.com/netapp/trident/v21/config"
)

func TestGetCommonInternalVolumeName(t *testing.T) {
	const name = "volume"
	for _, test := range []struct {
		prefix   *string
		expected string
	}{
		{
			prefix:   &[]string{"specific"}[0],
			expected: fmt.Sprintf("specific-%s", name),
		},
		{
			prefix:   &[]string{""}[0],
			expected: name,
		},
		{
			prefix:   nil,
			expected: fmt.Sprintf("%s-%s", config.OrchestratorName, name),
		},
	} {
		c := CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "fake",
			StoragePrefix:     test.prefix,
		}
		got := GetCommonInternalVolumeName(&c, name)
		if test.expected != got {
			t.Errorf("Mismatch between volume names.  Expected %s, got %s",
				test.expected, got)
		}
	}
}
