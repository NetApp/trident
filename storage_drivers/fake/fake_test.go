// Copyright 2018 NetApp, Inc. All Rights Reserved.

package fake

import (
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage/fake"
	testutils "github.com/netapp/trident/storage_drivers/fake/test_utils"
)

// TestNewConfig tests that marshaling works properly.  This has broken
// in the past.
func TestNewConfig(t *testing.T) {
	volumes := make([]fake.Volume, 0)
	_, err := NewFakeStorageDriverConfigJSON("test", config.File,
		testutils.GenerateFakePools(2), volumes)
	if err != nil {
		t.Fatal("Unable to generate config JSON:  ", err)
	}
}
