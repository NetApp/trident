// Copyright 2025 NetApp, Inc. All Rights Reserved.

package fake

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	testutils "github.com/netapp/trident/storage_drivers/fake/test_utils"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

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
	fakeStorageDrivers := []StorageDriver{
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
			map[string]string{"base-key": "base-value"},
			nil,
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value"}}`, "fake",
			"Base label is not set correctly", "Base label is not set correctly",
		}, // base label only
		{
			nil,
			map[string]string{"virtual-key": "virtual-value"},
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
		physicalPools := map[string]*fake.StoragePool{
			"fake-backend_pool_0": {
				Bytes: 50 * 1024 * 1024 * 1024,
				Attrs: map[string]sa.Offer{
					sa.IOPS:             sa.NewIntOffer(1000, 10000),
					sa.Snapshots:        sa.NewBoolOffer(true),
					sa.ProvisioningType: sa.NewStringOffer("thin", "thick"),
					"uniqueOptions":     sa.NewStringOffer("foo", "bar", "baz"),
				},
			},
		}

		fakePool := drivers.FakeStorageDriverPool{
			Labels: c.physicalPoolLabels,
			Region: "us_east_1",
			Zone:   "us_east_1a",
		}

		virtualPools := drivers.FakeStorageDriverPool{
			Labels: c.virtualPoolLabels,
			Region: "us_east_1",
			Zone:   "us_east_1a",
		}

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

func TestGetGroupSnapshotTarget(t *testing.T) {
	driver := *NewFakeStorageDriverWithDebugTraceFlags(map[string]bool{"method": true})
	volConfig := []*storage.VolumeConfig{
		{Name: "vol1", InternalName: "vol1"},
		{Name: "vol2", InternalName: "vol2"},
	}

	// Add volumes to the driver
	driver.Volumes["vol1"] = fake.Volume{Name: "vol1"}
	driver.Volumes["vol2"] = fake.Volume{Name: "vol2"}

	targetInfo, err := driver.GetGroupSnapshotTarget(context.TODO(), volConfig)
	assert.NoError(t, err, "GetGroupSnapshotTarget should not return an error")
	assert.NotNil(t, targetInfo, "TargetInfo should not be nil")
	assert.Equal(t, "fake", targetInfo.GetStorageType(), "TargetType should be 'fake'")
	assert.Equal(t, "fake-instance", targetInfo.GetStorageUUID(), "TargetUUID should match the instance name")
	assert.Len(t, targetInfo.GetVolumes(), 2, "There should be 2 target volumes")
}

func TestCreateGroupSnapshot(t *testing.T) {
	driver := *NewFakeStorageDriverWithDebugTraceFlags(map[string]bool{"method": true})
	vol1 := fake.Volume{Name: "vol1", SizeBytes: 1024}
	vol2 := fake.Volume{Name: "vol2", SizeBytes: 2048}
	driver.Volumes["vol1"] = vol1
	driver.Volumes["vol2"] = vol2

	groupSnapshotConfig := &storage.GroupSnapshotConfig{
		Name:        "groupsnapshot-12345",
		VolumeNames: []string{"vol1", "vol2"},
	}
	groupSnapshotTarget := &storage.GroupSnapshotTargetInfo{
		StorageType: "fake",
		StorageUUID: "fake-instance",
		StorageVolumes: map[string]map[string]*storage.VolumeConfig{
			"internalVol1": {
				"vol1": {
					Name:         "vol1",
					InternalName: "internalVol1",
				},
			},
			"internalVol2": {
				"vol2": {
					Name:         "vol2",
					InternalName: "internalVol2",
				},
			},
		},
	}

	err := driver.CreateGroupSnapshot(context.TODO(), groupSnapshotConfig, groupSnapshotTarget)
	assert.NoError(t, err, "CreateGroupSnapshot should not return an error")
}

func TestFakeStorageDriver_Import_Managed(t *testing.T) {
	driver := *NewFakeStorageDriverWithDebugTraceFlags(nil)
	originalName := "originalVolume"

	// Create a fake volume to import
	fakeVolume := fake.Volume{
		Name:      originalName,
		SizeBytes: 1073741824, // 1GB
	}
	driver.Volumes[originalName] = fakeVolume

	volConfig := &storage.VolumeConfig{
		Name:             "importedVolume",
		InternalName:     "trident-imported-volume",
		Size:             "1GB",
		ImportNotManaged: false,
	}

	err := driver.Import(context.Background(), volConfig, originalName)

	assert.NoError(t, err, "Import should not return an error")
	assert.Equal(t, "1073741824", volConfig.Size, "Volume size should be updated")

	// Verify the volume was renamed (moved in the map)
	_, exists := driver.Volumes[volConfig.InternalName]
	assert.True(t, exists, "Volume should exist with new internal name")

	_, originalExists := driver.Volumes[originalName]
	assert.False(t, originalExists, "Original volume should no longer exist")
}

func TestFakeStorageDriver_Import_NoRename(t *testing.T) {
	driver := *NewFakeStorageDriverWithDebugTraceFlags(nil)
	originalName := "originalVolume"

	// Create a fake volume to import
	fakeVolume := fake.Volume{
		Name:      originalName,
		SizeBytes: 1073741824, // 1GB
	}
	driver.Volumes[originalName] = fakeVolume

	volConfig := &storage.VolumeConfig{
		Name:             "importedVolume",
		InternalName:     "trident-imported-volume",
		Size:             "1GB",
		ImportNotManaged: false,
		ImportNoRename:   true, // Enable --no-rename flag
	}

	err := driver.Import(context.Background(), volConfig, originalName)

	assert.NoError(t, err, "Import with --no-rename should not return an error")
	assert.Equal(t, "1073741824", volConfig.Size, "Volume size should be updated")

	// With --no-rename, the volume should NOT be renamed (should stay with original name)
	_, exists := driver.Volumes[originalName]
	assert.True(t, exists, "Original volume should still exist with original name")

	_, newExists := driver.Volumes[volConfig.InternalName]
	assert.False(t, newExists, "Volume should NOT exist with new internal name")
}

func TestFakeStorageDriver_Import_NotManaged(t *testing.T) {
	driver := *NewFakeStorageDriverWithDebugTraceFlags(nil)
	originalName := "originalVolume"

	// Create a fake volume to import
	fakeVolume := fake.Volume{
		Name:      originalName,
		SizeBytes: 1073741824, // 1GB
	}
	driver.Volumes[originalName] = fakeVolume

	volConfig := &storage.VolumeConfig{
		Name:             "importedVolume",
		InternalName:     "trident-imported-volume",
		Size:             "1GB",
		ImportNotManaged: true, // Unmanaged import
	}

	err := driver.Import(context.Background(), volConfig, originalName)

	assert.NoError(t, err, "Unmanaged import should not return an error")
	assert.Equal(t, "1073741824", volConfig.Size, "Volume size should be updated")

	// With unmanaged import, the volume should NOT be renamed
	_, exists := driver.Volumes[originalName]
	assert.True(t, exists, "Original volume should still exist with original name")

	_, newExists := driver.Volumes[volConfig.InternalName]
	assert.False(t, newExists, "Volume should NOT exist with new internal name")
}
