// Copyright 2018 NetApp, Inc. All Rights Reserved.

package fake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/v21/config"
	"github.com/netapp/trident/v21/storage/fake"
	sa "github.com/netapp/trident/v21/storage_attribute"
	drivers "github.com/netapp/trident/v21/storage_drivers"
	testutils "github.com/netapp/trident/v21/storage_drivers/fake/test_utils"
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
		*NewFakeStorageDriverWithDebugTraceFlags(map[string]bool{"method": true}),
		*NewFakeStorageDriverWithDebugTraceFlags(nil),
	}

	// key: string to include in debug logs when the sensitive flag is set to true
	// value: string to include in unit test failure error message if key is missing when expected
	sensitiveIncludeList := map[string]string{
		"fake-secret": "secret",
	}

	// key: string to include in debug logs when the sensitive flag is set to false
	// value: string to include in unit test failure error message if key is missing when expected
	externalIncludeList := map[string]string{
		"<REDACTED>":        "<REDACTED>",
		"Secret:<REDACTED>": "secret",
	}

	for _, fakeStorageDriver := range fakeStorageDrivers {
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
	}
}

func TestInitializeStoragePoolsLabels(t *testing.T) {

	ctx := context.Background()
	cases := []struct {
		physicalPoolLabels   map[string]string
		virtualPoolLabels    map[string]string
		physicalExpected     string
		virtualExpected      string
		backendName          string
		physicalErrorMessage string
		virtualErrorMessage  string
	}{
		{
			nil, nil, "", "", "fake",
			"Label is not empty", "Label is not empty",
		}, // no labels
		{
			map[string]string{"base-key": "base-value"}, nil,
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value"}}`, "fake",
			"Base label is not set correctly", "Base label is not set correctly",
		}, // base label only
		{
			nil, map[string]string{"virtual-key": "virtual-value"},
			"",
			`{"provisioning":{"virtual-key":"virtual-value"}}`, "fake",
			"Base label is not empty", "Virtual pool label is not set correctly",
		}, // virtual label only
		{
			map[string]string{"base-key": "base-value"},
			map[string]string{"virtual-key": "virtual-value"},
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value","virtual-key":"virtual-value"}}`,
			"fake",
			"Base label is not set correctly", "Virtual pool label is not set correctly",
		}, // base and virtual labels
	}

	for _, c := range cases {
		physicalPools := map[string]*fake.StoragePool{"fake-backend_pool_0": {
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin", "thick"),
				"uniqueOptions":     sa.NewStringOffer("foo", "bar", "baz"),
			},
		}}

		fakePool := drivers.FakeStorageDriverPool{
			Labels: c.physicalPoolLabels,
			Region: "us_east_1",
			Zone:   "us_east_1a"}

		virtualPools := drivers.FakeStorageDriverPool{
			Labels: c.virtualPoolLabels,
			Region: "us_east_1",
			Zone:   "us_east_1a"}

		d, _ := NewFakeStorageDriverWithPools(ctx, physicalPools, fakePool,
			[]drivers.FakeStorageDriverPool{virtualPools})

		physicalPool := d.physicalPools["fake-backend_pool_0"]
		label, err := physicalPool.GetLabelsJSON(ctx, "provisioning", 1023)
		assert.Nil(t, err, "Error is not nil")
		assert.Equal(t, c.physicalExpected, label, c.physicalErrorMessage)

		virtualPool := d.virtualPools["fake_us_east_1_pool_0"]
		label, err = virtualPool.GetLabelsJSON(ctx, "provisioning", 1023)
		assert.Nil(t, err, "Error is not nil")
		assert.Equal(t, c.virtualExpected, label, c.virtualErrorMessage)
	}
}
