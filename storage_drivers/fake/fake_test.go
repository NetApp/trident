// Copyright 2018 NetApp, Inc. All Rights Reserved.

package fake

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestStorageDriverString(t *testing.T) {

	var fakeStorageDrivers = []StorageDriver{
		*NewFakeStorageDriverWithDebugTraceFlags(map[string]bool{"method": true, "sensitive": true}),
		*NewFakeStorageDriverWithDebugTraceFlags(map[string]bool{"method": true, "sensitive": false}),
		*NewFakeStorageDriverWithDebugTraceFlags(nil),
	}

	// key: string to include in debug logs when the sensitive flag is set to true
	// value: string to include in unit test failure error message if key is missing when expected
	sensitiveIncludeList := map[string]string{
		"fake-secret": "secret",
	}

	// key: string to exclude in debug logs when the sensitive flag is set to true
	// value: string to include in unit test failure error message if key is missing when expected
	sensitiveExcludeList := map[string]string{
		"<REDACTED>": "some information",
	}

	// key: string to include in debug logs when the sensitive flag is set to false
	// value: string to include in unit test failure error message if key is missing when expected
	externalIncludeList := map[string]string{
		"<REDACTED>":        "<REDACTED>",
		"Secret:<REDACTED>": "secret",
	}

	for _, fakeStorageDriver := range fakeStorageDrivers {
		sensitive, ok := fakeStorageDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok || (ok && !sensitive):
			for key, val := range externalIncludeList {
				assert.Contains(t, fakeStorageDriver.String(), key,
					"%s driver does not contain %v", fakeStorageDriver.Config.StorageDriverName, val)
				assert.Contains(t, fakeStorageDriver.GoString(), key,
					"%s driver does not contain %v", fakeStorageDriver.Config.StorageDriverName, val)
			}

			for key, val := range sensitiveIncludeList {
				assert.NotContains(t, fakeStorageDriver.String(), key,
					"%s driver contains %v", fakeStorageDriver.Config.StorageDriverName, val)
				assert.NotContains(t, fakeStorageDriver.GoString(), key,
					"%s driver contains %v", fakeStorageDriver.Config.StorageDriverName, val)
			}

		case ok && sensitive:
			for key, val := range sensitiveIncludeList {
				assert.Contains(t, fakeStorageDriver.String(), key,
					"%s driver does not contain %v", fakeStorageDriver.Config.StorageDriverName, val)
				assert.Contains(t, fakeStorageDriver.GoString(), key,
					"%s driver does not contain %v", fakeStorageDriver.Config.StorageDriverName, val)
			}

			for key, val := range sensitiveExcludeList {
				assert.NotContains(t, fakeStorageDriver.String(), key,
					"%s driver redacts %v", fakeStorageDriver.Config.StorageDriverName, val)
				assert.NotContains(t, fakeStorageDriver.GoString(), key,
					"%s driver redacts %v", fakeStorageDriver.Config.StorageDriverName, val)
			}
		}
	}
}
