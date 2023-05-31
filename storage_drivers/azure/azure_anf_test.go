// Copyright 2021 NetApp, Inc. All Rights Reserved.

package azure

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_azure"
	"github.com/netapp/trident/storage"
	storagefake "github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/api"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func newTestANFDriver(mockAPI api.Azure) *NASStorageDriver {
	prefix := "test-"

	config := drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: "azure-netapp-files",
			StoragePrefix:     &prefix,
			DebugTraceFlags:   debugTraceFlags,
		},
		SubscriptionID:      SubscriptionID,
		TenantID:            TenantID,
		ClientID:            ClientID,
		ClientSecret:        ClientSecret,
		Location:            Location,
		NfsMountOptions:     "nfsvers=3",
		VolumeCreateTimeout: "30",
	}

	return &NASStorageDriver{
		Config:              config,
		SDK:                 mockAPI,
		volumeCreateTimeout: 30 * time.Second,
	}
}

func newMockANFDriver(t *testing.T) (*mockapi.MockAzure, *NASStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAzure(mockCtrl)

	return mockAPI, newTestANFDriver(mockAPI)
}

func TestName(t *testing.T) {
	_, driver := newMockANFDriver(t)

	result := driver.Name()

	assert.Equal(t, tridentconfig.AzureNASStorageDriverName, result, "driver name mismatch")
}

func TestBackendName_SetInConfig(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.BackendName = "myANFBackend"

	result := driver.BackendName()

	assert.Equal(t, "myANFBackend", result, "backend name mismatch")
}

func TestBackendName_UseDefault(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.BackendName = ""

	result := driver.BackendName()

	assert.Equal(t, "azurenetappfiles_1-cli", result, "backend name mismatch")
}

func TestPoolName(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.BackendName = "myANFBackend"

	result := driver.poolName("pool-1-A")

	assert.Equal(t, "myANFBackend_pool1A", result, "pool name mismatch")
}

func TestValidateVolumeName(t *testing.T) {
	tests := []struct {
		Name  string
		Valid bool
	}{
		// Invalid names
		{"", false},
		{"x2345678901234567890123456789012345678901234567890123456789012345", false},
		{"1volume", false},
		{"-volume", false},
		{"_volume", false},
		{"volume&", false},
		// Valid names
		{"v", true},
		{"volume1", true},
		{"x234567890123456789012345678901234567890123456789012345678901234", true},
		{"Volume_1-A", true},
		{"volume-", true},
		{"volume_", true},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			_, driver := newMockANFDriver(t)

			err := driver.validateVolumeName(test.Name)

			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}

func TestValidateCreationToken(t *testing.T) {
	tests := []struct {
		Token string
		Valid bool
	}{
		// Invalid names
		{"", false},
		{"x23456789012345678901234567890123456789012345678901234567890123456789012345678901", false},
		{"1volume", false},
		{"-volume", false},
		{"_volume", false},
		{"volume&", false},
		{"Volume_1-A", false},
		// Valid names
		{"v", true},
		{"volume-1", true},
		{"x2345678901234567890123456789012345678901234567890123456789012345678901234567890", true},
		{"volume-", true},
	}
	for _, test := range tests {
		t.Run(test.Token, func(t *testing.T) {
			_, driver := newMockANFDriver(t)

			err := driver.validateCreationToken(test.Token)

			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}

func TestDefaultCreateTimeout(t *testing.T) {
	tests := []struct {
		Context  tridentconfig.DriverContext
		Expected time.Duration
	}{
		{tridentconfig.ContextDocker, tridentconfig.DockerCreateTimeout},
		{tridentconfig.ContextCSI, api.VolumeCreateTimeout},
		{"", api.VolumeCreateTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockANFDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultCreateTimeout()

			assert.Equal(t, test.Expected, result, "mismatched durations")
		})
	}
}

func TestDefaultTimeout(t *testing.T) {
	tests := []struct {
		Context  tridentconfig.DriverContext
		Expected time.Duration
	}{
		{tridentconfig.ContextDocker, tridentconfig.DockerDefaultTimeout},
		{tridentconfig.ContextCSI, api.DefaultTimeout},
		{"", api.DefaultTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockANFDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultTimeout()

			assert.Equal(t, test.Expected, result, "mismatched durations")
		})
	}
}

func TestInitialize(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "volumeCreateTimeout": "600",
        "sdkTimeout": "60",
        "maxCacheAge": "300"
    }`

	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.Equal(t, "deadbeef-784c-4b35-8329-460f52a3ad50", driver.Config.ClientID)
	assert.Equal(t, "myClientSecret", driver.Config.ClientSecret)
	assert.Equal(t, "trident-", *driver.Config.StoragePrefix, "wrong storage prefix")
	assert.Equal(t, 1, len(driver.pools), "wrong number of pools")
	assert.Equal(t, BackendUUID, driver.telemetry.TridentBackendUUID, "wrong backend UUID")
	assert.Equal(t, driver.volumeCreateTimeout, 600*time.Second, "volume create timeout mismatch")
	assert.True(t, driver.Initialized(), "not initialized")
}

func TestInitialize_WithSecrets(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "volumeCreateTimeout": "600"
    }`

	secrets := map[string]string{
		"clientid":     "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientsecret": "myClientSecret",
	}

	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, secrets, BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.Equal(t, "deadbeef-784c-4b35-8329-460f52a3ad50", driver.Config.ClientID)
	assert.Equal(t, "myClientSecret", driver.Config.ClientSecret)
	assert.Equal(t, "trident-", *driver.Config.StoragePrefix, "wrong storage prefix")
	assert.Equal(t, 1, len(driver.pools), "wrong number of pools")
	assert.Equal(t, BackendUUID, driver.telemetry.TridentBackendUUID, "wrong backend UUID")
	assert.Equal(t, driver.volumeCreateTimeout, 600*time.Second, "volume create timeout mismatch")
	assert.True(t, driver.Initialized(), "not initialized")
}

func TestInitialize_WithInvalidSecrets(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "volumeCreateTimeout": "600"
    }`

	secrets := map[string]string{
		"clientID":     "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
	}

	_, driver := newMockANFDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "not initialized")
}

func TestInitialize_NoLocation(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1"
    }`

	_, driver := newMockANFDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_SDKInitError(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1"
    }`

	_, driver := newMockANFDriver(t)
	driver.SDK = nil

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_InvalidConfigJSON(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
    }`

	_, driver := newMockANFDriver(t)

	driver.Config = drivers.AzureNASStorageDriverConfig{}
	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.NotNil(t, driver.Config.CommonStorageDriverConfig, "Driver Config not set")
	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_InvalidStoragePrefix(t *testing.T) {
	prefix := "&trident"

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		StoragePrefix:     &prefix,
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "storagePrefix": "&trident",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1"
    }`

	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_InvalidVolumeCreateTimeout(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "volumeCreateTimeout": "10m"
    }`

	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_InvalidSDKTimeout(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "sdkTimeout": "30s"
    }`

	_, driver := newMockANFDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_InvalidMaxCacheAge(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "location": "fake-location",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
        "clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
        "clientSecret": "myClientSecret",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "maxCacheAge": "300s"
    }`

	_, driver := newMockANFDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialized(t *testing.T) {
	tests := []struct {
		Expected bool
	}{
		{true},
		{false},
	}
	for _, test := range tests {
		t.Run(strconv.FormatBool(test.Expected), func(t *testing.T) {
			_, driver := newMockANFDriver(t)
			driver.initialized = test.Expected

			result := driver.Initialized()

			assert.Equal(t, test.Expected, result, "mismatched initialized values")
		})
	}
}

func TestTerminate(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.initialized = true

	driver.Terminate(ctx, "")

	assert.False(t, driver.initialized, "initialized not false")
}

func TestPopulateConfigurationDefaults_NoneSet(t *testing.T) {
	config := &drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
		},
	}

	_, driver := newMockANFDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.Equal(t, "trident-", *driver.Config.StoragePrefix)
	assert.Equal(t, defaultVolumeSizeStr, driver.Config.Size)
	assert.Equal(t, defaultUnixPermissions, driver.Config.UnixPermissions)
	assert.Equal(t, defaultNfsMountOptions, driver.Config.NfsMountOptions)
	assert.Equal(t, defaultSnapshotDir, driver.Config.SnapshotDir)
	assert.Equal(t, defaultLimitVolumeSize, driver.Config.LimitVolumeSize)
	assert.Equal(t, defaultExportRule, driver.Config.ExportRule)
}

func TestPopulateConfigurationDefaults_AllSet(t *testing.T) {
	prefix := "myPrefix"

	config := &drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			StoragePrefix:   &prefix,
			LimitVolumeSize: "123456789000",
		},
		NfsMountOptions: "nfsvers=4.1",
		AzureNASStorageDriverPool: drivers.AzureNASStorageDriverPool{
			AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				UnixPermissions: "0700",
				SnapshotDir:     "true",
				ExportRule:      "1.1.1.1/32",
			},
		},
	}

	_, driver := newMockANFDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.Equal(t, "myPrefix", *driver.Config.StoragePrefix)
	assert.Equal(t, "1234567890", driver.Config.Size)
	assert.Equal(t, "0700", driver.Config.UnixPermissions)
	assert.Equal(t, "nfsvers=4.1", driver.Config.NfsMountOptions)
	assert.Equal(t, "true", driver.Config.SnapshotDir)
	assert.Equal(t, "123456789000", driver.Config.LimitVolumeSize)
	assert.Equal(t, "1.1.1.1/32", driver.Config.ExportRule)
}

func TestInitializeStoragePools_NoVirtualPools(t *testing.T) {
	supportedTopologies := []map[string]string{
		{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "us-east-1c"},
	}

	config := &drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     "myANFBackend",
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		NfsMountOptions: "nfsvers=4.1",
		AzureNASStorageDriverPool: drivers.AzureNASStorageDriverPool{
			AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				UnixPermissions: "0700",
				SnapshotDir:     "true",
				ExportRule:      "1.1.1.1/32",
			},
			VirtualNetwork:      "VN1",
			Subnet:              "SN1",
			NetworkFeatures:     api.NetworkFeaturesStandard,
			ResourceGroups:      []string{"RG1", "RG2"},
			NetappAccounts:      []string{"NA1", "NA2"},
			CapacityPools:       []string{"CP1", "CP2"},
			ServiceLevel:        api.ServiceLevelUltra,
			Region:              "region1",
			Zone:                "zone1",
			SupportedTopologies: supportedTopologies,
			NASType:             "nfs",
		},
	}

	_, driver := newMockANFDriver(t)
	driver.Config = *config

	driver.initializeStoragePools(ctx)

	pool := storage.NewStoragePool(nil, "myANFBackend_pool")
	pool.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool.Attributes()[sa.Region] = sa.NewStringOffer("region1")
	pool.Attributes()[sa.Zone] = sa.NewStringOffer("zone1")
	pool.Attributes()[sa.NASType] = sa.NewStringOffer("nfs")

	pool.InternalAttributes()[Size] = "1234567890"
	pool.InternalAttributes()[UnixPermissions] = "0700"
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelUltra
	pool.InternalAttributes()[SnapshotDir] = "true"
	pool.InternalAttributes()[ExportRule] = "1.1.1.1/32"
	pool.InternalAttributes()[VirtualNetwork] = "VN1"
	pool.InternalAttributes()[Subnet] = "SN1"
	pool.InternalAttributes()[NetworkFeatures] = api.NetworkFeaturesStandard
	pool.InternalAttributes()[ResourceGroups] = "RG1,RG2"
	pool.InternalAttributes()[NetappAccounts] = "NA1,NA2"
	pool.InternalAttributes()[CapacityPools] = "CP1,CP2"

	pool.SetSupportedTopologies(supportedTopologies)

	expectedPools := map[string]storage.Pool{"myANFBackend_pool": pool}

	assert.Equal(t, expectedPools, driver.pools, "pools do not match")
}

func TestInitializeStoragePools_VirtualPools(t *testing.T) {
	supportedTopologies := []map[string]string{
		{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "us-east-1c"},
	}

	config := &drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     "myANFBackend",
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		NfsMountOptions: "nfsvers=4.1",
		AzureNASStorageDriverPool: drivers.AzureNASStorageDriverPool{
			AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				UnixPermissions: "0777",
				SnapshotDir:     "true",
				ExportRule:      "1.1.1.1/32",
			},
			VirtualNetwork: "VN1",
			Subnet:         "SN1",
			ServiceLevel:   "Standard",
			Region:         "region1",
			Zone:           "zone1",
		},
		Storage: []drivers.AzureNASStorageDriverPool{
			{
				AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{
					CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
						Size: "123456789000",
					},
					UnixPermissions: "0700",
					ExportRule:      "2.2.2.2/32",
				},
				VirtualNetwork:      "VN1",
				Subnet:              "SN1",
				NetworkFeatures:     api.NetworkFeaturesBasic,
				ResourceGroups:      []string{"RG1", "RG2"},
				NetappAccounts:      []string{"NA1", "NA2"},
				CapacityPools:       []string{"CP1"},
				ServiceLevel:        api.ServiceLevelUltra,
				Region:              "region2",
				Zone:                "zone2",
				SupportedTopologies: supportedTopologies,
				NASType:             "nfs",
			},
			{
				AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{
					UnixPermissions: "0770",
					SnapshotDir:     "false",
				},
				VirtualNetwork:      "VN1",
				Subnet:              "SN2",
				NetworkFeatures:     "",
				ResourceGroups:      []string{"RG1", "RG2"},
				NetappAccounts:      []string{"NA1", "NA2"},
				CapacityPools:       []string{"CP2"},
				SupportedTopologies: supportedTopologies,
			},
		},
	}

	_, driver := newMockANFDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	pool0 := storage.NewStoragePool(nil, "myANFBackend_pool_0")
	pool0.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool0.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool0.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool0.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool0.Attributes()[sa.Region] = sa.NewStringOffer("region2")
	pool0.Attributes()[sa.Zone] = sa.NewStringOffer("zone2")
	pool0.Attributes()[sa.NASType] = sa.NewStringOffer("nfs")

	pool0.InternalAttributes()[Size] = "123456789000"
	pool0.InternalAttributes()[UnixPermissions] = "0700"
	pool0.InternalAttributes()[ServiceLevel] = api.ServiceLevelUltra
	pool0.InternalAttributes()[SnapshotDir] = "true"
	pool0.InternalAttributes()[ExportRule] = "2.2.2.2/32"
	pool0.InternalAttributes()[VirtualNetwork] = "VN1"
	pool0.InternalAttributes()[Subnet] = "SN1"
	pool0.InternalAttributes()[NetworkFeatures] = api.NetworkFeaturesBasic
	pool0.InternalAttributes()[ResourceGroups] = "RG1,RG2"
	pool0.InternalAttributes()[NetappAccounts] = "NA1,NA2"
	pool0.InternalAttributes()[CapacityPools] = "CP1"

	pool0.SetSupportedTopologies(supportedTopologies)

	pool1 := storage.NewStoragePool(nil, "myANFBackend_pool_1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool1.Attributes()[sa.Region] = sa.NewStringOffer("region1")
	pool1.Attributes()[sa.Zone] = sa.NewStringOffer("zone1")
	pool1.Attributes()[sa.NASType] = sa.NewStringOffer("nfs")

	pool1.InternalAttributes()[Size] = "1234567890"
	pool1.InternalAttributes()[UnixPermissions] = "0770"
	pool1.InternalAttributes()[ServiceLevel] = "Standard"
	pool1.InternalAttributes()[SnapshotDir] = "false"
	pool1.InternalAttributes()[ExportRule] = "1.1.1.1/32"
	pool1.InternalAttributes()[VirtualNetwork] = "VN1"
	pool1.InternalAttributes()[Subnet] = "SN2"
	pool1.InternalAttributes()[NetworkFeatures] = ""
	pool1.InternalAttributes()[ResourceGroups] = "RG1,RG2"
	pool1.InternalAttributes()[NetappAccounts] = "NA1,NA2"
	pool1.InternalAttributes()[CapacityPools] = "CP2"

	pool1.SetSupportedTopologies(supportedTopologies)

	expectedPools := map[string]storage.Pool{
		"myANFBackend_pool_0": pool0,
		"myANFBackend_pool_1": pool1,
	}

	assert.Equal(t, expectedPools, driver.pools, "pools do not match")
}

func TestValidate_InvalidServiceLevel(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.ServiceLevel = "invalid"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidExportRule(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.ExportRule = "1.2.3.4.5"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidSnapshotDir(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.SnapshotDir = "yes"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_ValidUnixPermissions(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.UnixPermissions = "0777"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.NoError(t, result, "validate failed")
}

func TestValidate_InvalidUnixPermissions(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.UnixPermissions = "777"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidSize(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.Size = "abcde"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidLabel(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.Labels = map[string]string{
		"key1": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key2": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key3": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
	}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidNetworkFeatures(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.Config.NetworkFeatures = "invalid"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func getStructsForCreateNFSVolume(ctx context.Context, driver *NASStorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *api.CapacityPool, *api.Subnet, *api.FilesystemCreateRequest, *api.FileSystem,
) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
	}

	capacityPool := &api.CapacityPool{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		Name:              "CP1",
		FullName:          "RG1/NA1/CP1",
		Location:          Location,
		ServiceLevel:      api.ServiceLevelUltra,
		ProvisioningState: api.StateAvailable,
		QosType:           "Auto",
	}

	subnet := &api.Subnet{
		ID:             api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1"),
		ResourceGroup:  "RG2",
		VirtualNetwork: "VN1",
		Name:           "SN1",
		FullName:       "RG1/VN1/SN1",
		Location:       Location,
	}

	snapshotDir, _ := strconv.ParseBool(defaultSnapshotDir)

	apiExportRule := api.ExportRule{
		AllowedClients: defaultExportRule,
		Cifs:           false,
		Nfsv3:          true,
		Nfsv41:         false,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := api.ExportPolicy{
		Rules: []api.ExportRule{apiExportRule},
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	poolLabels, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
	labels[storage.ProvisioningLabelTag] = poolLabels

	createRequest := &api.FilesystemCreateRequest{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              volConfig.Name,
		SubnetID:          subnet.ID,
		CreationToken:     volConfig.InternalName,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		SnapshotDirectory: snapshotDir,
		UnixPermissions:   defaultUnixPermissions,
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		SnapshotDirectory: snapshotDir,
		SubnetID:          subnet.ID,
		UnixPermissions:   defaultUnixPermissions,
	}

	return volConfig, capacityPool, subnet, createRequest, filesystem
}

func TestCreate_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidVolumeName(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Name = "1testvol"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidCreationToken(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.InternalName = "1testvol"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NoStoragePool(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, nil, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NonexistentStoragePool(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]
	driver.pools = nil

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_VolumeExistsCreating(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	filesystem.ProvisioningState = api.StateCreating

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, errors.VolumeCreatingError(""), result, "not VolumeCreatingError")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	filesystem.ProvisioningState = api.StateAvailable

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, drivers.NewVolumeExistsError(""), result, "not VolumeExistsError")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "invalid"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NegativeSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "-1M"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_ZeroSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "0"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.QuotaInBytes, DefaultVolumeSize, "request size mismatch")
	assert.Equal(t, volConfig.Size, defaultVolumeSizeStr, "config size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_BelowAbsoluteMinimumSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "1k"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_AboveMaximumSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.LimitVolumeSize = "100Gi"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "200Gi"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidSnapshotDir(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.SnapshotDir = "invalid"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidLabel(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]
	storagePool.Attributes()[sa.Labels] = sa.NewLabelOffer(map[string]string{
		"key1": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key2": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key3": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
	})

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NoCapacityPool(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool, api.ServiceLevelUltra).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NoSubnet(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_InvalidMountOptions(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.MountOptions = "nfsvers=5"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_DefaultMountOptions(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"
	driver.Config.NfsMountOptions = ""

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, volConfig.Size, strconv.FormatInt(createRequest.QuotaInBytes, 10), "request size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolume_VolConfigMountOptions(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.MountOptions = "nfsvers=4"
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolume_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_BelowANFMinimumSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = strconv.FormatUint(MinimumANFVolumeSizeBytes-1, 10)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, uint64(createRequest.QuotaInBytes), MinimumANFVolumeSizeBytes, "request size mismatch")
	assert.Equal(t, volConfig.Size, strconv.FormatUint(MinimumANFVolumeSizeBytes, 10), "config size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func getStructsForCreateSMBVolume(ctx context.Context, driver *NASStorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *api.CapacityPool, *api.Subnet, *api.FilesystemCreateRequest, *api.FileSystem,
) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
	}

	capacityPool := &api.CapacityPool{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		Name:              "CP1",
		FullName:          "RG1/NA1/CP1",
		Location:          Location,
		ServiceLevel:      api.ServiceLevelUltra,
		ProvisioningState: api.StateAvailable,
		QosType:           "Auto",
	}

	subnet := &api.Subnet{
		ID:             api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1"),
		ResourceGroup:  "RG2",
		VirtualNetwork: "VN1",
		Name:           "SN1",
		FullName:       "RG1/VN1/SN1",
		Location:       Location,
	}

	snapshotDir, _ := strconv.ParseBool(defaultSnapshotDir)

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	poolLabels, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
	labels[storage.ProvisioningLabelTag] = poolLabels

	createRequest := &api.FilesystemCreateRequest{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              volConfig.Name,
		SubnetID:          subnet.ID,
		CreationToken:     volConfig.InternalName,
		Labels:            labels,
		ProtocolTypes:     []string{api.ProtocolTypeCIFS},
		QuotaInBytes:      VolumeSizeI64,
		SnapshotDirectory: snapshotDir,
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeCIFS},
		QuotaInBytes:      VolumeSizeI64,
		SnapshotDirectory: snapshotDir,
		SubnetID:          subnet.ID,
	}

	return volConfig, capacityPool, subnet, createRequest, filesystem
}

func TestCreate_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "smb"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateSMBVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "", volConfig.UnixPermissions)
}

func TestCreate_SMBVolume_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "smb"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, _ := getStructsForCreateSMBVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_SMBVolume_BelowANFMinimumSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "smb"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateSMBVolume(ctx, driver, storagePool)
	volConfig.Size = strconv.FormatUint(MinimumANFVolumeSizeBytes-1, 10)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, uint64(createRequest.QuotaInBytes), MinimumANFVolumeSizeBytes, "request size mismatch")
	assert.Equal(t, volConfig.Size, strconv.FormatUint(MinimumANFVolumeSizeBytes, 10), "config size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolumeOnSMBPool_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "smb"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	_, _, _, createRequest, _ := getStructsForCreateSMBVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NotEqual(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type matches")
	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_SMBVolumeOnNFSPool_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, _, filesystem := getStructsForCreateSMBVolume(ctx, driver, storagePool)
	_, _, _, createRequest, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomCapacityPoolForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPool).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NotEqual(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type matches")
	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func getStructsForCreateClone(ctx context.Context, driver *NASStorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *storage.VolumeConfig, *api.FilesystemCreateRequest, *api.FileSystem, *api.FileSystem,
	*api.Snapshot,
) {
	sourceVolumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")
	cloneVolumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol2")
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")

	sourceVolConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   sourceVolumeID,
	}

	cloneVolConfig := &storage.VolumeConfig{
		Version:                   "1",
		Name:                      "testvol2",
		InternalName:              "trident-testvol2",
		CloneSourceVolume:         "testvol1",
		CloneSourceVolumeInternal: "trident-testvol1",
	}

	snapshotDir, _ := strconv.ParseBool(defaultSnapshotDir)

	apiExportRule := api.ExportRule{
		AllowedClients: defaultExportRule,
		Cifs:           false,
		Nfsv3:          true,
		Nfsv41:         false,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := api.ExportPolicy{
		Rules: []api.ExportRule{apiExportRule},
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	poolLabels, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
	labels[storage.ProvisioningLabelTag] = poolLabels

	createRequest := &api.FilesystemCreateRequest{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              cloneVolConfig.Name,
		SubnetID:          subnetID,
		CreationToken:     cloneVolConfig.InternalName,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		SnapshotDirectory: snapshotDir,
		SnapshotID:        SnapshotUUID,
		UnixPermissions:   defaultUnixPermissions,
	}

	sourceFilesystem := &api.FileSystem{
		ID:                sourceVolumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		SnapshotDirectory: snapshotDir,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
	}

	cloneFilesystem := &api.FileSystem{
		ID:                cloneVolumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol2",
		FullName:          "RG1/NA1/CP1/testvol2",
		Location:          Location,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol2",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: snapshotDir,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
	}

	snapshot := &api.Snapshot{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testvol1",
		Name:              "snap1",
		FullName:          "RG1/NA1/CP1/testvol1/snap1",
		Location:          Location,
		Created:           time.Now(),
		SnapshotID:        SnapshotUUID,
		ProvisioningState: api.StateAvailable,
	}

	return sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot
}

func TestCreateClone_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesBasic
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	createRequest.NetworkFeatures = api.NetworkFeaturesBasic
	sourceFilesystem.NetworkFeatures = api.NetworkFeaturesBasic
	cloneFilesystem.NetworkFeatures = api.NetworkFeaturesBasic

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, sourceFilesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_Snapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshot = "snap1"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_InvalidVolumeName(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.Name = "1testvol"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_InvalidCreationToken(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.InternalName = "1testvol"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_NonexistentSourceVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, _ := getStructsForCreateClone(ctx, driver,
		storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_VolumeExistsCreating(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, _ := getStructsForCreateClone(ctx, driver,
		storagePool)
	cloneFilesystem.ProvisioningState = api.StateCreating

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(true, cloneFilesystem, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.IsType(t,
		errors.VolumeCreatingError(""), result, "not VolumeCreatingError")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, _ := getStructsForCreateClone(ctx, driver,
		storagePool)
	cloneFilesystem.ProvisioningState = api.StateAvailable

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(true, cloneFilesystem, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.IsType(t, drivers.NewVolumeExistsError(""), result, "not VolumeExistsError")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotNotFound(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, _ := getStructsForCreateClone(ctx, driver,
		storagePool)
	cloneVolConfig.CloneSourceSnapshot = "snap1"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotNotAvailable(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshot = "snap1"
	snapshot.ProvisioningState = api.StateError

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotCreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, _ := getStructsForCreateClone(ctx, driver,
		storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotWaitFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, sourceFilesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotRefetchFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, sourceFilesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, gomock.Any()).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_InvalidLabel(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.Labels = map[string]string{
		"key1": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key2": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key3": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
	}

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, sourceFilesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, sourceFilesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func getStructsForImport(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *api.FileSystem) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	originalFilesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "importMe",
		FullName:          "RG1/NA1/CP1/importMe",
		Location:          Location,
		Labels:            make(map[string]string),
		ProvisioningState: api.StateAvailable,
		CreationToken:     "importMe",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
	}

	return volConfig, originalFilesystem
}

func getStructsForSMBImport(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *api.FileSystem) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	originalFilesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "importMe",
		FullName:          "RG1/NA1/CP1/importMe",
		Location:          Location,
		Labels:            make(map[string]string),
		ProvisioningState: api.StateAvailable,
		CreationToken:     "importMe",
		ProtocolTypes:     []string{api.ProtocolTypeCIFS},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
	}

	return volConfig, originalFilesystem
}

func TestImport_Managed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_SMB_Managed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "smb"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForSMBImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		nil).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_SMB_Failed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "smb"
	driver.Config.UnixPermissions = "0770"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForSMBImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		nil).Return(errors.New("unix permissions not applicable for SMB")).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import failed")
}

func TestImport_DualProtocolVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	// Dual-protocol volume has ProtocolTypes as [NFSv3, CIFS]
	originalFilesystem.ProtocolTypes = append(originalFilesystem.ProtocolTypes, "CIFS")

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import failed")
}

func TestImport_ManagedWithLabels(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = ""
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.UnixPermissions = "0700"
	originalFilesystem.Labels = map[string]string{
		storage.ProvisioningLabelTag: `{"provisioning":{"cloud":"anf","clusterName":"dev-test-cluster-1"}}`,
	}

	expectedLabels := map[string]string{
		storage.ProvisioningLabelTag: "",
		drivers.TridentLabelTag:      driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0700"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_NotManaged(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	volConfig.ImportNotManaged = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"

	originalName := "importMe"

	volConfig, _ := getStructsForImport(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_NotFound(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"

	originalName := "importMe"

	volConfig, _ := getStructsForImport(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(nil, errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_InvalidCapacityPool(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_InvalidUnixPermissions(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "8888"
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_ModifyVolumeFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout()).Return("", errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_BackendVolumeMismatch(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForSMBImport(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import should fail")
}

func TestRename(t *testing.T) {
	_, driver := newMockANFDriver(t)

	result := driver.Rename(ctx, "oldName", "newName")

	assert.Nil(t, result, "not nil")
}

func TestGetTelemetryLabels(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	result := driver.getTelemetryLabels(ctx)

	assert.True(t, strings.HasPrefix(result, `{"trident":{`))
}

func TestUpdateTelemetryLabels(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	result := driver.updateTelemetryLabels(ctx, &api.FileSystem{})

	assert.NotNil(t, result)
	assert.True(t, strings.HasPrefix(result[drivers.TridentLabelTag], `{"trident":{`))
}

func TestWaitForVolumeCreate_Available(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem)

	assert.Nil(t, result)
}

func TestWaitForVolumeCreate_Creating(t *testing.T) {
	for _, state := range []string{api.StateAccepted, api.StateCreating} {

		mockAPI, driver := newMockANFDriver(t)

		filesystem := &api.FileSystem{
			Name:              "testvol1",
			CreationToken:     "netapp-testvol1",
			ProvisioningState: api.StateCreating,
		}

		mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
			driver.volumeCreateTimeout).Return(state, errFailed).Times(1)

		result := driver.waitForVolumeCreate(ctx, filesystem)

		assert.Error(t, result, "expected error")
		assert.IsType(t, errors.VolumeCreatingError(""), result, "not VolumeCreatingError")
	}
}

func TestWaitForVolumeCreate_DeletingDeleteFinished(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleting, errFailed).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem)

	assert.Nil(t, result, "not nil")
}

func TestWaitForVolumeCreate_DeletingDeleteNotFinished(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleting, errFailed).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleting, errFailed).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem)

	assert.Nil(t, result, "not nil")
}

func TestWaitForVolumeCreate_ErrorDelete(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateError, errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem)

	assert.Nil(t, result, "not nil")
}

func TestWaitForVolumeCreate_ErrorDeleteFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateError, errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(errFailed).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem)

	assert.Nil(t, result, "not nil")
}

func TestWaitForVolumeCreate_OtherStates(t *testing.T) {
	for _, state := range []string{api.StateMoving, "unknown"} {

		mockAPI, driver := newMockANFDriver(t)

		filesystem := &api.FileSystem{
			Name:              "testvol1",
			CreationToken:     "netapp-testvol1",
			ProvisioningState: api.StateCreating,
		}

		mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
			driver.volumeCreateTimeout).Return(state, errFailed).Times(1)

		result := driver.waitForVolumeCreate(ctx, filesystem)

		assert.Nil(t, result, "not nil")
	}
}

func getStructsForDestroyNFSVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	apiExportRule := api.ExportRule{
		AllowedClients: defaultExportRule,
		Cifs:           false,
		Nfsv3:          true,
		Nfsv41:         false,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := api.ExportPolicy{
		Rules: []api.ExportRule{apiExportRule},
	}

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ExportPolicy:      exportPolicy,
		Labels:            make(map[string]string),
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
		NetworkFeatures:   api.NetworkFeaturesStandard,
	}

	return volConfig, filesystem
}

func getStructsForDestroySMBVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		Labels:            make(map[string]string),
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeCIFS},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		NetworkFeatures:   api.NetworkFeaturesStandard,
	}

	return volConfig, filesystem
}

func TestDestroy_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_AlreadyDeleted(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_StillDeletingDeleted(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	filesystem.ProvisioningState = api.StateDeleting

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_StillDeleting(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	filesystem.ProvisioningState = api.StateDeleting

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleting, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleting, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroySMBVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func getStructsForPublishNFSVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem, *utils.VolumePublishInfo) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	mountTargets := []api.MountTarget{
		{
			MountTargetID: "mountTargetID",
			FileSystemID:  "filesystemID",
			IPAddress:     "1.1.1.1",
			SmbServerFqdn: "",
		},
	}

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		Labels:            labels,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
		MountTargets:      mountTargets,
	}

	publishInfo := &utils.VolumePublishInfo{}

	return volConfig, filesystem, publishInfo
}

func getStructsForPublishSMBVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem, *utils.VolumePublishInfo) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	mountTargets := []api.MountTarget{
		{
			MountTargetID: "mountTargetID",
			FileSystemID:  "filesystemID",
			IPAddress:     "1.1.1.1",
			SmbServerFqdn: "trident-1234.trident.com",
		},
	}

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		Labels:            labels,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeCIFS},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		MountTargets:      mountTargets,
	}

	publishInfo := &utils.VolumePublishInfo{}

	return volConfig, filesystem, publishInfo
}

func TestPublish_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, "/trident-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "1.1.1.1", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=3", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "smb"

	volConfig, filesystem, publishInfo := getStructsForPublishSMBVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, "\\trident-testvol1", publishInfo.SMBPath, "SMB path mismatch")
	assert.Equal(t, "trident-1234.trident.com", publishInfo.SMBServer, "SMB server mismatch")
	assert.Equal(t, "smb", publishInfo.FilesystemType, "filesystem type mismatch")
}

func TestPublish_MountOptions(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.InternalID = ""
	volConfig.MountOptions = "nfsvers=4.1"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, "/trident-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "1.1.1.1", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=4.1", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_NoMountTargets(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	filesystem.MountTargets = nil

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func getStructsForCreateSnapshot(ctx context.Context, driver *NASStorageDriver, snapTime time.Time) (
	*storage.VolumeConfig, *api.FileSystem, *storage.SnapshotConfig, *api.Snapshot,
) {
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		Labels:            labels,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
	}

	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "testvol1",
		VolumeInternalName: "trident-testvol1",
	}

	snapshot := &api.Snapshot{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testvol1",
		Name:              "snap1",
		FullName:          "RG1/NA1/CP1/testvol1/snap1",
		Location:          Location,
		Created:           snapTime,
		SnapshotID:        SnapshotUUID,
		ProvisioningState: api.StateAvailable,
	}

	return volConfig, filesystem, snapConfig, snapshot
}

func TestCanSnapshot(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	result := driver.CanSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestGetSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, resultErr, "not nil")
	assert.Equal(t, snapConfig, result.Config, "snapshot mismatch")
	assert.Equal(t, int64(0), result.SizeBytes, "snapshot mismatch")
	assert.Equal(t, storage.SnapshotStateOnline, result.State, "snapshot mismatch")
}

func TestGetSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestGetSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestGetSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Nil(t, resultErr, "not nil")
}

func TestGetSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(nil,
		errors.NotFoundError("not found")).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Nil(t, resultErr, "not nil")
}

func TestGetSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestGetSnapshot_SnapshotNotAvailable(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshot.ProvisioningState = api.StateError

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func getSnapshotsForList(snapTime time.Time) *[]*api.Snapshot {
	return &[]*api.Snapshot{
		{
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testvol1",
			Name:              "snap1",
			FullName:          "RG1/NA1/CP1/testvol1/snap1",
			Location:          Location,
			Created:           snapTime,
			SnapshotID:        SnapshotUUID,
			ProvisioningState: api.StateAvailable,
		},
		{
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testvol1",
			Name:              "snap2",
			FullName:          "RG1/NA1/CP1/testvol1/snap2",
			Location:          Location,
			Created:           snapTime,
			SnapshotID:        SnapshotUUID,
			ProvisioningState: api.StateAvailable,
		},
		{
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testvol1",
			Name:              "snap3",
			FullName:          "RG1/NA1/CP1/testvol1/snap3",
			Location:          Location,
			Created:           time.Now(),
			SnapshotID:        "",
			ProvisioningState: api.StateError,
		},
	}
}

func TestGetSnapshots(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshots := getSnapshotsForList(snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotsForVolume(ctx, filesystem).Return(snapshots, nil).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	expectedSnapshot0 := &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			Version:            tridentconfig.OrchestratorAPIVersion,
			Name:               "snap1",
			InternalName:       "snap1",
			VolumeName:         "testvol1",
			VolumeInternalName: "trident-testvol1",
		},
		Created:   snapTime.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	expectedSnapshot1 := &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			Version:            tridentconfig.OrchestratorAPIVersion,
			Name:               "snap2",
			InternalName:       "snap2",
			VolumeName:         "testvol1",
			VolumeInternalName: "trident-testvol1",
		},
		Created:   snapTime.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.Nil(t, resultErr, "not nil")
	assert.Len(t, result, 2)

	assert.Equal(t, result[0], expectedSnapshot0, "snapshot 0 mismatch")
	assert.Equal(t, result[1], expectedSnapshot1, "snapshot 1 mismatch")
}

func TestGetSnapshots_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestGetSnapshots_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestGetSnapshots_GetSnapshotsFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotsForVolume(ctx, filesystem).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestCreateSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, filesystem, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, filesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	expectedSnapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapTime.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.Nil(t, resultErr, "not nil")
	assert.Equal(t, expectedSnapshot, result)
}

func TestCreateSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestCreateSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestCreateSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestCreateSnapshot_SnapshotCreateFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, filesystem, "snap1").Return(nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestCreateSnapshot_SnapshotWaitFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, filesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, filesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "expected error")
}

func TestRestoreSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, filesystem, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable,
		[]string{api.StateError, api.StateDeleting, api.StateDeleted}, api.DefaultSDKTimeout).
		Return(api.StateAvailable, nil).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestRestoreSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestRestoreSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestRestoreSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestRestoreSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestRestoreSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(nil, errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestRestoreSnapshot_SnapshotRestoreFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, filesystem, snapshot).Return(errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestRestoreSnapshot_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, filesystem, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable,
		[]string{api.StateError, api.StateDeleting, api.StateDeleted},
		api.DefaultSDKTimeout).Return("", errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDeleteSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, filesystem, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, filesystem, api.StateDeleted, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDeleteSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDeleteSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDeleteSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDeleteSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDeleteSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(nil, errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDeleteSnapshot_SnapshotDeleteFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, filesystem, snapshot).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDeleteSnapshot_SnapshotWaitFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, filesystem, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, filesystem, api.StateDeleted, []string{api.StateError},
		api.SnapshotTimeout).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func getVolumesForList() *[]*api.FileSystem {
	return &[]*api.FileSystem{
		{
			ProvisioningState: api.StateAvailable,
			CreationToken:     "myPrefix-testvol1",
		},
		{
			ProvisioningState: api.StateAvailable,
			CreationToken:     "myPrefix-testvol2",
		},
		{
			ProvisioningState: api.StateDeleting,
			CreationToken:     "myPrefix-testvol3",
		},
		{
			ProvisioningState: api.StateDeleted,
			CreationToken:     "myPrefix-testvol4",
		},
		{
			ProvisioningState: api.StateError,
			CreationToken:     "myPrefix-testvol5",
		},
		{
			ProvisioningState: api.StateAvailable,
			CreationToken:     "testvol6",
		},
	}
}

func TestList(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

	list, result := driver.List(ctx)

	assert.Nil(t, result, "expected no error")
	assert.Equal(t, list, []string{"testvol1", "testvol2"}, "list not nil")
}

func TestList_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Times(0)

	list, result := driver.List(ctx)

	assert.Error(t, result, "expected error")
	assert.Nil(t, list, "list not nil")
}

func TestList_ListFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

	list, result := driver.List(ctx)

	assert.Error(t, result, "expected error")
	assert.Nil(t, list, "list not nil")
}

func TestList_ListNone(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(&[]*api.FileSystem{}, nil).Times(1)

	list, result := driver.List(ctx)

	assert.Nil(t, result, "expected nil")
	assert.Equal(t, []string{}, list, "list not empty")
}

func TestGet(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, "volume1").Return(filesystem, nil).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.NoError(t, result, "expected no error")
}

func TestGet_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, "volume1").Times(0)

	result := driver.Get(ctx, "volume1")

	assert.Error(t, result, "expected error")
}

func TestGet_NotFound(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, "volume1").Return(nil, errFailed).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.Error(t, result, "expected error")
}

func TestResize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	volConfig.InternalID = ""
	newSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().ResizeVolume(ctx, filesystem, int64(newSize)).Return(nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, strconv.FormatUint(newSize, 10), volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_VolumeNotAvailable(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	filesystem.ProvisioningState = api.StateError
	newSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_NoSizeChange(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(VolumeSizeI64)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_ShrinkingVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(VolumeSizeI64 / 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_AboveMaximumSize(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	driver.Config.LimitVolumeSize = strconv.FormatInt(VolumeSizeI64+1, 10)
	newSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestResize_VolumeResizeFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().ResizeVolume(ctx, filesystem, int64(newSize)).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestGetStorageBackendSpecs(t *testing.T) {
	_, driver := newMockANFDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	backend := &storage.StorageBackend{}
	backend.SetStorage(make(map[string]storage.Pool))

	result := driver.GetStorageBackendSpecs(ctx, backend)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, "azurenetappfiles_1-cli", backend.Name(), "backend name mismatch")
	for _, pool := range driver.pools {
		assert.Equal(t, backend, pool.Backend(), "pool-backend mismatch")
		assert.Equal(t, pool, backend.Storage()["azurenetappfiles_1-cli_pool"], "backend-pool mismatch")
	}
}

func TestCreatePrepare(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	driver.CreatePrepare(ctx, volConfig)

	assert.Equal(t, "myPrefix-testvol1", volConfig.InternalName)
}

func TestGetStorageBackendPhysicalPoolNames(t *testing.T) {
	_, driver := newMockANFDriver(t)

	result := driver.GetStorageBackendPhysicalPoolNames(ctx)

	assert.Equal(t, []string{}, result, "physical pool names mismatch")
}

func TestGetInternalVolumeName_PassthroughStore(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "testvol1")

	assert.Equal(t, "myPrefix-testvol1", result, "internal name mismatch")
}

func TestGetInternalVolumeName_CSI(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "pvc-5e522901-b891-41d8-9e83-5496d2e62e71")

	assert.Equal(t, "pvc-5e522901-b891-41d8-9e83-5496d2e62e71", result, "internal name mismatch")
}

func TestGetInternalVolumeName_NonCSI(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "testvol1")

	anfRegex := regexp.MustCompile(`^anf-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	assert.True(t, anfRegex.MatchString(result), "internal name mismatch")
}

func TestCreateFollowup_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, (filesystem.MountTargets)[0].IPAddress, volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "/"+filesystem.CreationToken, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "nfs", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "smb"

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, (filesystem.MountTargets)[0].SmbServerFqdn, volConfig.AccessInfo.SMBServer, "SMB server mismatch")
	assert.Equal(t, "\\"+filesystem.CreationToken, volConfig.AccessInfo.SMBPath, "SMB path mismatch")
	assert.Equal(t, "smb", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_VolumeNotAvailable(t *testing.T) {
	nonAvailableStates := []string{
		api.StateAccepted, api.StateCreating, api.StateDeleting, api.StateDeleted,
		api.StateMoving, api.StateError, api.StateReverting,
	}

	for _, state := range nonAvailableStates {

		mockAPI, driver := newMockANFDriver(t)
		driver.initializeTelemetry(ctx, BackendUUID)

		volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)
		filesystem.ProvisioningState = state

		mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

		result := driver.CreateFollowup(ctx, volConfig)

		assert.NotNil(t, result, "expected error")
		assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
		assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
		assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
	}
}

func TestCreateFollowup_NoMountTargets(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)
	filesystem.MountTargets = nil

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestGetProtocol(t *testing.T) {
	_, driver := newMockANFDriver(t)

	result := driver.GetProtocol(ctx)

	assert.Equal(t, tridentconfig.File, result)
}

func TestStoreConfig(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	_, driver := newMockANFDriver(t)
	driver.Config.CommonStorageDriverConfig = commonConfig

	persistentConfig := &storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, persistentConfig)

	assert.Equal(t, json.RawMessage("{}"), driver.Config.CommonStorageDriverConfig.StoragePrefixRaw,
		"raw prefix mismatch")
	assert.Equal(t, driver.Config, *persistentConfig.AzureConfig, "azure config mismatch")
}

func TestGetVolumeExternal(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "myPrefix-testvol1",
		ProvisioningState: api.StateAvailable,
	}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, "testvol1").Return(filesystem, nil).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, "testvol1")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "testvol1", result.Config.Name)
	assert.Equal(t, "myPrefix-testvol1", result.Config.InternalName)
}

func TestGetVolumeExternal_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, "testvol1")

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "error expected")
}

func TestGetVolumeExternal_GetFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, "testvol1").Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, "testvol1")

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "error expected")
}

func TestGetVolumeExternalWrappers(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	filesystems := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(*filesystems))

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(filesystems, nil).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}

	assert.Len(t, volumes, 2, "wrong number of volumes")
}

func TestGetVolumeExternalWrappers_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	filesystems := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(*filesystems))

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	var result error
	for wrapper := range channel {
		if wrapper.Error != nil {
			result = wrapper.Error
			break
		}
	}

	assert.NotNil(t, result, "expected error")
}

func TestGetVolumeExternalWrappers_ListFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	filesystems := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(*filesystems))

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	var result error
	for wrapper := range channel {
		if wrapper.Error != nil {
			result = wrapper.Error
			break
		}
	}

	assert.NotNil(t, result, "expected error")
}

func TestStringAndGoString(t *testing.T) {
	_, driver := newMockANFDriver(t)

	stringFunc := func(d *NASStorageDriver) string { return d.String() }
	goStringFunc := func(d *NASStorageDriver) string { return d.GoString() }

	for _, toString := range []func(*NASStorageDriver) string{stringFunc, goStringFunc} {

		assert.Contains(t, toString(driver), "<REDACTED>",
			"ANF driver does not contain <REDACTED>")
		assert.Contains(t, toString(driver), "SDK:<REDACTED>",
			"ANF driver does not redact SDK information")
		assert.Contains(t, toString(driver), "SubscriptionID:<REDACTED>",
			"ANF driver does not redact API URL")
		assert.Contains(t, toString(driver), "TenantID:<REDACTED>",
			"ANF driver does not redact Tenant ID")
		assert.Contains(t, toString(driver), "ClientID:<REDACTED>",
			"ANF driver does not redact Client ID")
		assert.Contains(t, toString(driver), "ClientSecret:<REDACTED>",
			"ANF driver does not redact Client Secret")
		assert.NotContains(t, toString(driver), SubscriptionID,
			"ANF driver contains Subscription ID")
		assert.NotContains(t, toString(driver), TenantID,
			"ANF driver contains Tenant ID")
		assert.NotContains(t, toString(driver), ClientID,
			"ANF driver contains Client ID")
		assert.NotContains(t, toString(driver), ClientSecret,
			"ANF driver contains Client Secret")
	}
}

func TestGetUpdateType_NoFlaggedChanges(t *testing.T) {
	_, oldDriver := newMockANFDriver(t)
	oldDriver.volumeCreateTimeout = 1 * time.Second

	_, newDriver := newMockANFDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestGetUpdateType_WrongDriverType(t *testing.T) {
	oldDriver := &fake.StorageDriver{
		Config:             drivers.FakeStorageDriverConfig{},
		Volumes:            make(map[string]storagefake.Volume),
		DestroyedVolumes:   make(map[string]bool),
		Snapshots:          make(map[string]map[string]*storage.Snapshot),
		DestroyedSnapshots: make(map[string]bool),
		Secret:             "secret",
	}

	_, newDriver := newMockANFDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestGetUpdateType_OtherChanges(t *testing.T) {
	_, oldDriver := newMockANFDriver(t)
	prefix1 := "prefix1-"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	_, newDriver := newMockANFDriver(t)
	prefix2 := "prefix2-"
	newDriver.Config.StoragePrefix = &prefix2
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestReconcileNodeAccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAzure(mockCtrl)

	driver := *newTestANFDriver(mockAPI)

	result := driver.ReconcileNodeAccess(ctx, nil, "", "")

	assert.Nil(t, result, "not nil")
}

func TestValidateStoragePrefix(t *testing.T) {
	tests := []struct {
		Name          string
		StoragePrefix string
		Valid         bool
	}{
		// Invalid storage prefixes
		{
			Name:          "storage prefix starts with plus",
			StoragePrefix: "+abcd-ABC",
		},
		{
			Name:          "storage prefix starts with digit",
			StoragePrefix: "1abcd-ABC",
		},
		{
			Name:          "storage prefix starts with underscore",
			StoragePrefix: "_abcd-ABC",
		},
		{
			Name:          "storage prefix contains digits",
			StoragePrefix: "abcd-123-ABC",
		},
		{
			Name:          "storage prefix contains underscore",
			StoragePrefix: "ABCD_abc",
		},
		{
			Name:          "storage prefix has plus",
			StoragePrefix: "abcd+ABC",
		},
		{
			Name:          "storage prefix is single digit",
			StoragePrefix: "1",
		},
		{
			Name:          "storage prefix is single underscore",
			StoragePrefix: "_",
		},
		{
			Name:          "storage prefix is single colon",
			StoragePrefix: ":",
		},
		{
			Name:          "storage prefix is single dash",
			StoragePrefix: "-",
		},
		// Valid storage prefixes
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
			Valid:         true,
		},
		{
			Name:          "storage prefix has only letters and dash",
			StoragePrefix: "abcd-efgh",
			Valid:         true,
		},
		{
			Name:          "storage prefix ends with dash",
			StoragePrefix: "abcd-efgh-",
			Valid:         true,
		},
		{
			Name:          "storage prefix has capital letters",
			StoragePrefix: "ABCD",
			Valid:         true,
		},
		{
			Name:          "storage prefix has letters and capital letters",
			StoragePrefix: "abcd-EFGH",
			Valid:         true,
		},
		{
			Name:          "storage prefix is empty",
			StoragePrefix: "",
			Valid:         true,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			err := validateStoragePrefix(test.StoragePrefix)
			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}

func TestGetCommonConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAzure(mockCtrl)

	driver := *newTestANFDriver(mockAPI)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	result := driver.GetCommonConfig(ctx)

	assert.Equal(t, driver.Config.CommonStorageDriverConfig, result, "common config mismatch")
}
