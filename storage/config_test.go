// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage

import (
	"fmt"
	"testing"

	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
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
			expected: fmt.Sprintf("%s", name),
		},
		{
			prefix:   nil,
			expected: fmt.Sprintf("%s-%s", config.OrchestratorName, name),
		},
	} {
		c := dvp.CommonStorageDriverConfig{
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
