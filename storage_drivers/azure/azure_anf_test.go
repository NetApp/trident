// Copyright 2023 NetApp, Inc. All Rights Reserved.

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
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/acp"
	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockacp "github.com/netapp/trident/mocks/mock_acp"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_azure"
	"github.com/netapp/trident/storage"
	storagefake "github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/api"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
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
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

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
        "maxCacheAge": "300",
        "kerberos": "sec-krb5"
    }`

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

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

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

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

func TestInitialize_NoTenantID_NOClientID(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	envVariables := map[string]string{
		"AZURE_CLIENT_ID":            "deadbeef-784c-4b35-8329-460f52a3ad50",
		"AZURE_TENANT_ID":            "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"AZURE_FEDERATED_TOKEN_FILE": "/test/file/path",
		"AZURE_AUTHORITY_HOST":       "https://msft.com/",
	}

	// Set required environment variables for testing
	for key, value := range envVariables {
		_ = os.Setenv(key, value)
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1",
        "location": "fake-location"
    }`

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.True(t, driver.Initialized(), "not initialized")

	// Unset all the environment variables
	for key := range envVariables {
		_ = os.Unsetenv(key)
	}
}

func TestInitialize_NOClientID_NOClientSecret(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configFile, _ := os.Getwd()

	envVariable := map[string]string{
		"AZURE_CREDENTIAL_FILE": configFile + "azure.json",
	}

	// Set required environment variable for testing
	for key, value := range envVariable {
		_ = os.Setenv(key, value)
	}

	configFileContent := `
	{
	  "cloud": "AzurePublicCloud",
	  "tenantId": "deadbeef-784c-4b35-8329-460f52a3ad50",
	  "subscriptionId": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
	  "aadClientId": "test-msi",
	  "aadClientSecret": "test-msi",
	  "resourceGroup": "RG1",
	  "location": "fake-location",
	  "useManagedIdentityExtension": true,
	  "userAssignedIdentityID": "deadbeef-173f-4bf4-b5b8-7cba6f53a227"
	}`

	_ = os.WriteFile(envVariable["AZURE_CREDENTIAL_FILE"], []byte(configFileContent), os.ModePerm)

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
        "serviceLevel": "Premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
	    "virtualNetwork": "VN1",
	    "subnet": "RG1/VN1/SN1"
    }`

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.True(t, driver.Initialized(), "not initialized")

	// Remove the file
	_ = os.Remove(envVariable["AZURE_CREDENTIAL_FILE"])

	// Unset environment variable
	for key := range envVariable {
		_ = os.Unsetenv(key)
	}
}

func TestInitialize_NOClientID_NOClientSecret_Error_ReadingFile(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configFile, _ := os.Getwd()

	envVariable := map[string]string{
		"AZURE_CREDENTIAL_FILE": configFile + "azure.json",
	}

	// Set required environment variable for testing
	for key, value := range envVariable {
		_ = os.Setenv(key, value)
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "azure-netapp-files",
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

	// Unset environment variable
	for key := range envVariable {
		_ = os.Unsetenv(key)
	}
}

func TestInitialize_NOClientID_NOClientSecret_Error_JSONUnmarshal(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files",
		BackendName:       "myANFBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configFile, _ := os.Getwd()

	envVariable := map[string]string{
		"AZURE_CREDENTIAL_FILE": configFile + "azure.json",
	}

	// Set required environment variable for testing
	for key, value := range envVariable {
		_ = os.Setenv(key, value)
	}

	configFileContent := `
	{
	  "cloud": "AzurePublicCloud",
	  "tenantId": "deadbeef-784c-4b35-8329-460f52a3ad50",
	  "subscriptionId": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
	  "aadClientId": "test-msi",
	  "aadClientSecret": "test-msi",
	  "resourceGroup": "RG1",
	  "location": "fake-location",
	  "useManagedIdentityExtension": true,
	  "userAssignedIdentityID" = "deadbeef-173f-4bf4-b5b8-7cba6f53a227"
	}`

	_ = os.WriteFile(envVariable["AZURE_CREDENTIAL_FILE"], []byte(configFileContent), os.ModePerm)

	configJSON := `
   {
		"version": 1,
       "storageDriverName": "azure-netapp-files",
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

	// Remove the file
	_ = os.Remove(envVariable["AZURE_CREDENTIAL_FILE"])

	// Unset environment variable
	for key := range envVariable {
		_ = os.Unsetenv(key)
	}
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

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI, driver := newMockANFDriver(t)

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

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

func TestInitialize_FailsToGetBackendPools(t *testing.T) {
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
	// Have to at least one CapacityPool for ANF backends; backend pool discovery will force initialize to fail
	// if no capacity pools can be found.
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{}).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
	assert.Empty(t, driver.Config.BackendPools, "backend pools were not empty")
}

func TestInitialize_withEncryptionKeys(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

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
        "maxCacheAge": "300",
        "kerberos": "sec-krb5",
		"customerEncryptionKeys": {"netappAccount1": "key1", "netappAccount2": "key2"}
    }`

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

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

func TestInitialize_withEncryptionKeysStandardNetworkFeatures(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

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
        "maxCacheAge": "300",
        "kerberos": "sec-krb5",
		"customerEncryptionKeys": {"netappAccount1": "key1", "netappAccount2": "key2"},
		"networkFeatures": "Standard"
    }`

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

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

func TestInitialize_FailureWithEncryptionKeysBasicNetworkFeatures(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

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
        "maxCacheAge": "300",
        "kerberos": "sec-krb5",
		"customerEncryptionKeys": {"netappAccount1": "key1", "netappAccount2": "key2"},
		"networkFeatures": "Basic"
    }`

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize should fail with Basic networkFeature")
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

func TestPopulateConfigurationDefaults_SnapshotDir(t *testing.T) {
	config := &drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
		},
	}

	_, driver := newMockANFDriver(t)
	driver.Config = *config

	tests := []struct {
		name                string
		nfsMountOption      string
		inputSnapshotDir    string
		expectedSnapshotDir string
	}{
		{"Default snapshotDir with no nfsMountOption", "", "", ""},

		{"Default snapshotDir with nfsMountOption as NFSv3", "nfsvers=3", "", ""},
		{"Explicit snapshotDir with nfsMountOption as NFSv3", "nfsvers=3", "TRUE", "true"},
		{"Invalid snapshotDir with nfsMountOption as NFSv3", "nfsvers=3", "TrUe", "TrUe"},

		{"Default snapshotDir with nfsMountOption as NFSv4", "nfsvers=4", "", ""},
		{"Explicit snapshotDir with nfsMountOption as NFSv4", "nfsvers=4", "FALSE", "false"},
		{"Invalid snapshotDir with nfsMountOption as NFSv4", "nfsvers=4", "TrUe", "TrUe"},

		{"Default snapshotDir with nfsMountOption as NFSv4.1", "nfsvers=4.1", "", ""},
		{"Explicit snapshotDir with nfsMountOption as NFSv4.1", "nfsvers=4.1", "FALSE", "false"},
		{"Invalid snapshotDir with nfsMountOption as NFSv4", "nfsvers=4.1", "TrUe", "TrUe"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver.Config.NfsMountOptions = test.nfsMountOption
			driver.Config.SnapshotDir = test.inputSnapshotDir
			driver.populateConfigurationDefaults(ctx, &driver.Config)
			assert.Equal(t, test.expectedSnapshotDir, driver.Config.SnapshotDir)
		})
	}
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
				SnapshotDir:     "TRUE",
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
	pool.InternalAttributes()[Kerberos] = ""

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
				Kerberos:            "sec=krb5i",
			},
			{
				AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{
					UnixPermissions: "0770",
					SnapshotDir:     "FALSE",
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
	pool0.InternalAttributes()[Kerberos] = "sec=krb5i"

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
	pool1.InternalAttributes()[Kerberos] = ""

	pool1.SetSupportedTopologies(supportedTopologies)

	expectedPools := map[string]storage.Pool{
		"myANFBackend_pool_0": pool0,
		"myANFBackend_pool_1": pool1,
	}

	assert.Equal(t, expectedPools, driver.pools, "pools do not match")
}

func TestInitializeStoragePools_SnapshotDir(t *testing.T) {
	// Test with multiple vpools each with different snapshotDir value
	config := &drivers.AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     "myANFBackend",
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		Storage: []drivers.AzureNASStorageDriverPool{
			{
				Region: "us_east_1",
				Zone:   "us_east_1a",
			},
			{
				AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{SnapshotDir: "False"},
			},
			{
				AzureNASStorageDriverConfigDefaults: drivers.AzureNASStorageDriverConfigDefaults{SnapshotDir: "Invalid"},
			},
		},
	}

	_, driver := newMockANFDriver(t)
	driver.Config = *config
	driver.Config.SnapshotDir = "TRUE"

	driver.initializeStoragePools(ctx)

	// Case1: Ensure snapshotDir is picked from driver config and is lower case
	virtualPool1 := driver.pools["myANFBackend_pool_0"]
	assert.Equal(t, "true", virtualPool1.InternalAttributes()[SnapshotDir])

	// Case2: Ensure snapshotDir is picked from virtual pool and is lower case
	virtualPool2 := driver.pools["myANFBackend_pool_1"]
	assert.Equal(t, "false", virtualPool2.InternalAttributes()[SnapshotDir])

	// Case3: Ensure snapshotDir is picked from virtual pool and is same as passed in case of invalid value
	virtualPool3 := driver.pools["myANFBackend_pool_2"]
	assert.Equal(t, "Invalid", virtualPool3.InternalAttributes()[SnapshotDir])
}

func TestInitialize_KerberosDisabledInPool(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

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
        "maxCacheAge": "300",
        "kerberos": "sec-krb5"
    }`

	// Have to at least one CapacityPool for ANF backends.
	pool := &api.CapacityPool{
		Name:          "CP1",
		Location:      "fake-location",
		NetAppAccount: "NA1",
		ResourceGroup: "RG1",
	}

	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(errFailed).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{pool}).Times(1)

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

func getMultipleCapacityPoolsForCreateVolume() []*api.CapacityPool {
	return []*api.CapacityPool{
		{
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			Name:              "CP1",
			FullName:          "RG1/NA1/CP1",
			Location:          Location,
			ServiceLevel:      api.ServiceLevelUltra,
			ProvisioningState: api.StateAvailable,
			QosType:           "Auto",
		},
		{
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			Name:              "CP2",
			FullName:          "RG1/NA1/CP2",
			Location:          Location,
			ServiceLevel:      api.ServiceLevelUltra,
			ProvisioningState: api.StateAvailable,
			QosType:           "Auto",
		},
		{
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			Name:              "CP3",
			FullName:          "RG1/NA1/CP3",
			Location:          Location,
			ServiceLevel:      api.ServiceLevelUltra,
			ProvisioningState: api.StateAvailable,
			QosType:           "Auto",
		},
	}
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_MultipleCapacityPools_FirstSucceeds(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	capacityPools := getMultipleCapacityPoolsForCreateVolume()

	createRequest.ResourceGroup = capacityPools[0].ResourceGroup
	createRequest.NetAppAccount = capacityPools[0].NetAppAccount
	createRequest.CapacityPool = capacityPools[0].Name
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPools).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_MultipleCapacityPools_SecondSucceeds(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	capacityPools := getMultipleCapacityPoolsForCreateVolume()

	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	createRequest1 := *createRequest
	createRequest1.ResourceGroup = capacityPools[0].ResourceGroup
	createRequest1.NetAppAccount = capacityPools[0].NetAppAccount
	createRequest1.CapacityPool = capacityPools[0].Name
	createRequest2 := *createRequest
	createRequest2.ResourceGroup = capacityPools[1].ResourceGroup
	createRequest2.NetAppAccount = capacityPools[1].NetAppAccount
	createRequest2.CapacityPool = capacityPools[1].Name

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.ResourceGroup = capacityPools[1].ResourceGroup
	filesystem.NetAppAccount = capacityPools[1].NetAppAccount
	filesystem.CapacityPool = capacityPools[1].Name

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPools).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest1).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest2).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_MultipleCapacityPools_NoneSucceeds(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	capacityPools := getMultipleCapacityPoolsForCreateVolume()

	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	createRequest1 := *createRequest
	createRequest1.ResourceGroup = capacityPools[0].ResourceGroup
	createRequest1.NetAppAccount = capacityPools[0].NetAppAccount
	createRequest1.CapacityPool = capacityPools[0].Name
	createRequest2 := *createRequest
	createRequest2.ResourceGroup = capacityPools[1].ResourceGroup
	createRequest2.NetAppAccount = capacityPools[1].NetAppAccount
	createRequest2.CapacityPool = capacityPools[1].Name
	createRequest3 := *createRequest
	createRequest3.ResourceGroup = capacityPools[2].ResourceGroup
	createRequest3.NetAppAccount = capacityPools[2].NetAppAccount
	createRequest3.CapacityPool = capacityPools[2].Name

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.ResourceGroup = capacityPools[1].ResourceGroup
	filesystem.NetAppAccount = capacityPools[1].NetAppAccount
	filesystem.CapacityPool = capacityPools[1].Name

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return(capacityPools).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest1).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest2).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest3).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_Kerberos_type5(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.NfsMountOptions = ""
	driver.Config.Kerberos = "sec=krb5"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.KerberosEnabled = true
	createRequest.ExportPolicy.Rules[0].Kerberos5ReadWrite = true
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].UnixReadWrite = false
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.SnapshotDirectory = true
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.KerberosEnabled = true
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}

	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_Kerberos_type5_FailsEntitlementCheck(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.NfsMountOptions = ""
	driver.Config.Kerberos = "sec=krb5"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.KerberosEnabled = true
	createRequest.ExportPolicy.Rules[0].Kerberos5ReadWrite = true
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].UnixReadWrite = false
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.KerberosEnabled = true
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NFSVolume_Kerberos_type5I(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5i"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.KerberosEnabled = true
	createRequest.ExportPolicy.Rules[0].Kerberos5IReadWrite = true
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].UnixReadWrite = false
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.KerberosEnabled = true
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_Kerberos_type5I_FailsEntitlementCheck(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5i"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.KerberosEnabled = true
	createRequest.ExportPolicy.Rules[0].Kerberos5IReadWrite = true
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].UnixReadWrite = false
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.KerberosEnabled = true
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NFSVolume_Kerberos_type5P(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5p"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.KerberosEnabled = true
	createRequest.ExportPolicy.Rules[0].Kerberos5PReadWrite = true
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].UnixReadWrite = false
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.KerberosEnabled = true
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_Kerberos_type5P_FailsEntitlementCheck(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5p"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.KerberosEnabled = true
	createRequest.ExportPolicy.Rules[0].Kerberos5PReadWrite = true
	createRequest.ExportPolicy.Rules[0].Nfsv41 = true
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].UnixReadWrite = false
	createRequest.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.KerberosEnabled = true
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NFSVolume_Kerberos_type5P_failure(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5P"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "unsupported kerberos type: sec=krb5P", "did not fail")
	assert.NotNil(t, result, "expected nil")
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

func TestCreate_VolumeExistsResourceExhaustedError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	filesystem.ProvisioningState = api.StateError
	cpLowSpaceError := errors.ResourceExhaustedError(
		errors.New("PoolSizeTooSmall: Unable to complete the operation. " +
			"The capacity pool size of 4398046511104 bytes is too small for the " +
			"combined volume size of 6597069766656 bytes of the capacity pool."))

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateError, cpLowSpaceError).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, errors.ResourceExhaustedError(errors.New("")), result, "not ResourceExhaustedError")
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
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

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

	assert.Error(t, result, "create did not fail")
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

	assert.Error(t, result, "create did not fail")
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

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

	assert.Error(t, result, "create did not fail")
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

	assert.Error(t, result, "create did not fail")
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
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
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

	assert.Error(t, result, "create did not fail")
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

	volConfig, _, subnet, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{}).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
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

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
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

	assert.Error(t, result, "create did not fail")
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

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
	createRequest.SnapshotDirectory = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, "true", volConfig.SnapshotDir)
}

// TestCreate_NFSVolume_WithSnapDirMountOptionCombinations tests expected snapshotDir and mountOption values
// based on various combinations for input snapshotDir values and mount options
func TestCreate_NFSVolume_WithSnapDirMountOptionCombinations(t *testing.T) {
	/*
		Each test provides input SnapshotDir + MountOption combinations; mock request objects parameters and expected snapshotDir and mountOption value
		Each test is to test different combinations for snapshotDir, mountOptions at various places: backend config, storage class and volume config
		Note: SnapshotDir is currently only supported to be given at backend level and volume config level and not storage class.
		Note: MountOption is currently only supported to be given at backend and at storage class level and not volume config level.

		Below are the cases.
		- : means no explicit value given; NA - means currently it is not supported to be given at that level
		Preference of value pickup is: volConfig >  storageClass > backend
		NFSv4 and NFSv4.1 should behave the same
		Output expected: default mountOption = NFSv3
						 default snapDir for NFSv3     = false; if no explicit value is provided
						 default snapDir for NFSv4/4.1 = true; if no explicit value is provided


		// No explicit option provided

		| Case   1   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |      NA      |   -      | false    |
		|MountOption |  -       |      -       |   NA     | nfsv3    |
		|----------- |----------|--------------|----------|----------|

		// Explicit snapDir option at various levels

		| Case   2   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |     NA       |  true    | true     |
		|MountOption |  -       |      -       |   NA     | nfsv3    |
		|----------- |----------|--------------|----------|----------|

		| Case   3   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |     NA       |  false   | false    |
		|MountOption |  -       |      -       |   NA     | nfsv3    |
		|----------- |----------|--------------|----------|----------|

		| Case   4   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  true    |     NA       |   -      | true     |
		|MountOption |  -       |      -       |   NA     | nfsv3    |
		|----------- |----------|--------------|----------|----------|

		| Case   5   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  false   |     NA       |   -      | false    |
		|MountOption |  -       |      -       |   NA     | nfsv3    |
		|----------- |----------|--------------|----------|----------|


		// Mount option in Backend Config with different snapDir values

		| Case   6   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |      NA      |   -      | true     |
		|MountOption |nfsvers=4 |      -       |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   7   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  true    |      NA      |   -      | true     |
		|MountOption |nfsvers=4 |      -       |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   8   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  false   |      NA      |   -      | false    |
		|MountOption |nfsvers=4 |      -       |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|


		// Mount option in Storage Class with different snapDir values

		| Case   9   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |      NA      |   -      | true     |
		|MountOption |  -       |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   10  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |      NA      |   false  | false    |
		|MountOption |  -       |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   11  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  false   |      NA      |   -      | false    |
		|MountOption |  -       |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   12  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  true    |      NA      |   false  | false    |
		|MountOption |  -       |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|


		// Mount option in both Backend Config and Storage Class with different snapDir values

		| Case   13  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     |  -       |      NA      |   -      | true     |
		|MountOption |nfsvers=3 |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   14  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     | false    |      NA      |   -      | false    |
		|MountOption |nfsvers=3 |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case   15  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     | true     |      NA      |   -      | true     |
		|MountOption |nfsvers=3 |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case  16   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     | -        |      NA      |  false   | false    |
		|MountOption |nfsvers=3 |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case  17   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     | -        |      NA      |   true   | true     |
		|MountOption |nfsvers=3 |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|


		// All possible options given

		| Case   18  | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     | true     |      NA      |  false   | false    |
		|MountOption |nfsvers=3 |   nfsvers=4  |   NA     | nfsv4    |
		|----------- |----------|--------------|----------|----------|

		| Case  19   | Backend  | StorageClass | VolConfig| Output
		|----------- |----------|--------------|----------|----------|
		|SnapDir     | false    |      NA      |   true   | true     |
		|MountOption |nfsvers=4 |   nfsvers=3  |   NA     | nfsv3    |
		|----------- |----------|--------------|----------|----------|

	*/

	tests := []struct {
		name                          string
		snapDirInBackend              string
		snapDirInVolConfig            string
		mountOptInBackend             string
		mountOptInPhyisicalPool       string
		mockRequestSnapshotDir        bool
		mockRequestProtocols          []string
		mockRequestExportPolicyNfsv3  bool
		mockRequestExportPolicyNfsv41 bool
		mockResponseSnapshotDir       bool
		mockResponseProtocols         []string
		expectedSnapshotDir           bool
		expectedProtocols             []string
	}{
		{
			name:                          "1. No explicit values provided for either SnapshotDir or MountOption",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv3},
			mockRequestExportPolicyNfsv3:  true,
			mockRequestExportPolicyNfsv41: false,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv3},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv3},
		},
		{
			name:                          "2. SnapshotDir = true in VolConfig",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "true",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv3},
			mockRequestExportPolicyNfsv3:  true,
			mockRequestExportPolicyNfsv41: false,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv3},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv3},
		},
		{
			name:                          "3. SnapshotDir = false in VolConfig",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "false",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv3},
			mockRequestExportPolicyNfsv3:  true,
			mockRequestExportPolicyNfsv41: false,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv3},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv3},
		},

		{
			name:                          "4. SnapshotDir = true in Backend",
			snapDirInBackend:              "true",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv3},
			mockRequestExportPolicyNfsv3:  true,
			mockRequestExportPolicyNfsv41: false,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv3},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv3},
		},
		{
			name:                          "5. SnapshotDir = false in Backend",
			snapDirInBackend:              "false",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv3},
			mockRequestExportPolicyNfsv3:  true,
			mockRequestExportPolicyNfsv41: false,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv3},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv3},
		},
		{
			name:                          "6. MountOption = nfsv4 in Backend + SnapshotDir = nil in Backend",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "nfsvers=4.1",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "7. MountOption = nfsv4 in Backend + SnapshotDir = true in Backend",
			snapDirInBackend:              "true",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "nfsvers=4.1",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "8. MountOption = nfsv4 in Backend + SnapshotDir = false in Backend",
			snapDirInBackend:              "false",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "nfsvers=4.1",
			mountOptInPhyisicalPool:       "",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "9. MountOption = nfsv4 in Storage Class + SnapshotDir = nil everywhere",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "10. MountOption = nfsv4 in Storage Class + SnapshotDir = false in VolConfig",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "false",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "11. MountOption = nfsv4 in Storage Class + SnapshotDir = false in Backend",
			snapDirInBackend:              "false",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "12. MountOption = nfsv4 in Storage Class + SnapshotDir = true in Backend, false in VolConfig",
			snapDirInBackend:              "true",
			snapDirInVolConfig:            "false",
			mountOptInBackend:             "",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "13. MountOption = nfsv3 in Backend and nfsv4 in Storage Class + SnapshotDir = nil",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "nfsvers=3",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "14. MountOption = nfsv3 in Backend and nfsv4 in Storage Class + SnapshotDir = false in Backend",
			snapDirInBackend:              "false",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "nfsvers=3",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "15. MountOption = nfsv3 in Backend and nfsv4 in Storage Class + SnapshotDir = true in Backend",
			snapDirInBackend:              "true",
			snapDirInVolConfig:            "",
			mountOptInBackend:             "nfsvers=3",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "16. MountOption = nfsv3 in Backend and nfsv4 in Storage Class + SnapshotDir = false in VolConfig",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "false",
			mountOptInBackend:             "nfsvers=3",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "17. MountOption = nfsv3 in Backend and nfsv4 in Storage Class + SnapshotDir = true in VolConfig",
			snapDirInBackend:              "",
			snapDirInVolConfig:            "true",
			mountOptInBackend:             "nfsvers=3",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "18. MountOption = nfsv3 in Backend and nfsv4 in Storage Class + SnapshotDir = true in Backend and false in VolConfig",
			snapDirInBackend:              "true",
			snapDirInVolConfig:            "false",
			mountOptInBackend:             "nfsvers=3",
			mountOptInPhyisicalPool:       "nfsvers=4.1",
			mockRequestSnapshotDir:        false,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv41},
			mockRequestExportPolicyNfsv3:  false,
			mockRequestExportPolicyNfsv41: true,
			mockResponseSnapshotDir:       false,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv41},
			expectedSnapshotDir:           false,
			expectedProtocols:             []string{api.ProtocolTypeNFSv41},
		},
		{
			name:                          "19. MountOption = nfsv4 in Backend and nfsv3 in Storage Class + SnapshotDir = false in Backend and true in VolConfig",
			snapDirInBackend:              "false",
			snapDirInVolConfig:            "true",
			mountOptInBackend:             "nfsvers=4",
			mountOptInPhyisicalPool:       "nfsvers=3",
			mockRequestSnapshotDir:        true,
			mockRequestProtocols:          []string{api.ProtocolTypeNFSv3},
			mockRequestExportPolicyNfsv3:  true,
			mockRequestExportPolicyNfsv41: false,
			mockResponseSnapshotDir:       true,
			mockResponseProtocols:         []string{api.ProtocolTypeNFSv3},
			expectedSnapshotDir:           true,
			expectedProtocols:             []string{api.ProtocolTypeNFSv3},
		},
	}

	for _, test := range tests {
		mockAPI, driver := newMockANFDriver(t)
		driver.Config.BackendName = "anf"
		driver.Config.ServiceLevel = api.ServiceLevelUltra
		driver.Config.NASType = "nfs"

		// Set test values in backend config
		driver.Config.SnapshotDir = test.snapDirInBackend
		driver.Config.NfsMountOptions = test.mountOptInBackend
		driver.populateConfigurationDefaults(ctx, &driver.Config)

		// Set test values in storage pool if not already set by populateConfigurationDefaults
		driver.initializeStoragePools(ctx)
		storagePool := driver.pools["anf_pool"]

		// Initialize telemetry
		driver.initializeTelemetry(ctx, BackendUUID)

		// Set test values in volume config; note that volConfig inherits mount option from storage class, so set from there
		volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver,
			storagePool)
		volConfig.SnapshotDir = test.snapDirInVolConfig
		volConfig.MountOptions = test.mountOptInPhyisicalPool

		// Set suitable values for mock requests and set mock expects
		createRequest.ProtocolTypes = test.mockRequestProtocols
		createRequest.ExportPolicy.Rules[0].Nfsv3 = test.mockRequestExportPolicyNfsv3
		createRequest.ExportPolicy.Rules[0].Nfsv41 = test.mockRequestExportPolicyNfsv41
		createRequest.SnapshotDirectory = test.mockRequestSnapshotDir
		filesystem.ProtocolTypes = test.mockResponseProtocols
		filesystem.SnapshotDirectory = test.mockResponseSnapshotDir

		mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil)
		mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil)
		mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(false)
		mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet)
		mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
			api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool})
		mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil)
		mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
			driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil)

		// Create volume
		result := driver.Create(ctx, volConfig, storagePool, nil)

		// Assert on suitable snapshotDir values
		assert.NoError(t, result, "create failed")
		assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
		assert.Equal(t, test.expectedSnapshotDir, filesystem.SnapshotDirectory)
		assert.Equal(t, test.expectedProtocols, filesystem.ProtocolTypes)
	}
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, uint64(createRequest.QuotaInBytes), MinimumANFVolumeSizeBytes, "request size mismatch")
	assert.Equal(t, volConfig.Size, strconv.FormatUint(MinimumANFVolumeSizeBytes, 10), "config size mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolume_ZoneSelectionSucceeds(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.SupportedTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
	}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.RequisiteTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "2"},
	}
	volConfig.PreferredTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
	}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard
	createRequest.Zone = "1"
	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_ZoneSelectionFails(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.SupportedTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "3"},
	}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.RequisiteTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "2"},
	}
	volConfig.PreferredTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
	}
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard
	createRequest.Zone = "1"
	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
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

func TestCreate_CMEKVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"
	driver.Config.CustomerEncryptionKeys = map[string]string{"NA1": "k1"}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	createRequest.KeyVaultEndpointID = api.CreateKeyVaultEndpoint(driver.Config.SubscriptionID,
		createRequest.ResourceGroup, driver.Config.CustomerEncryptionKeys[createRequest.NetAppAccount])

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_CMEKVolumeNilNetworking(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = ""
	driver.Config.NASType = "nfs"
	driver.Config.CustomerEncryptionKeys = map[string]string{"NA1": "k1"}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, capacityPool, subnet, createRequest, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	createRequest.NetworkFeatures = api.NetworkFeaturesStandard
	filesystem.UnixPermissions = "0777"
	filesystem.NetworkFeatures = api.NetworkFeaturesStandard

	createRequest.KeyVaultEndpointID = api.CreateKeyVaultEndpoint(driver.Config.SubscriptionID,
		createRequest.ResourceGroup, driver.Config.CustomerEncryptionKeys[createRequest.NetAppAccount])

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().HasFeature(api.FeatureUnixPermissions).Return(true).Times(1)
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.QuotaInBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, api.ServiceLevelUltra, volConfig.ServiceLevel)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NotEqual(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type matches")
	assert.Error(t, result, "create did not fail")
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
	mockAPI.EXPECT().RandomSubnetForStoragePool(ctx, storagePool).Return(subnet).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		api.ServiceLevelUltra).Return([]*api.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NotEqual(t, createRequest.ProtocolTypes, filesystem.ProtocolTypes, "protocol type matches")
	assert.Error(t, result, "create did not fail")
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
		SnapshotID:        SnapshotName,
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
		ID:                SnapshotName,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testvol1",
		Name:              "snap1",
		FullName:          "RG1/NA1/CP1/testvol1/snap1",
		Location:          Location,
		Created:           time.Now(),
		SnapshotID:        SnapshotID,
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
	sourceVolConfig.SnapshotDir = "false"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotState(ctx, snapshot, sourceFilesystem, api.StateAvailable, []string{api.StateError},
		api.SnapshotTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, cloneVolConfig.CloneSourceSnapshotInternal, snapshot.Name,
		"expected snapshot name to be set in CloneSourceSnapshotInternal ")
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
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolConfig.SnapshotDir = "false"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_CMEK(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"
	driver.Config.CustomerEncryptionKeys = map[string]string{"NA1": "k1"}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolConfig.SnapshotDir = "false"
	sourceFilesystem.NetworkFeatures = api.NetworkFeaturesStandard

	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_CMEKNilNetworking(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"
	driver.Config.CustomerEncryptionKeys = map[string]string{"NA1": "k1"}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolConfig.SnapshotDir = "false"
	sourceFilesystem.NetworkFeatures = ""

	createRequest.NetworkFeatures = api.NetworkFeaturesStandard

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_CMEKBasicNetworking(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"
	driver.Config.CustomerEncryptionKeys = map[string]string{"NA1": "k1"}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolConfig.SnapshotDir = "false"
	sourceFilesystem.NetworkFeatures = ""
	sourceFilesystem.NetworkFeatures = api.NetworkFeaturesBasic

	createRequest.NetworkFeatures = api.NetworkFeaturesBasic

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_ROClone(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceFilesystem.SnapshotDirectory = true
	cloneVolConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Nil(t, result, "create failed")
	assert.NoError(t, result, "error occurred")
}

func TestCreateClone_ROCloneFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolConfig.SnapshotDir = "false"
	cloneVolConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NotNil(t, result, "create succeeded")
	assert.Error(t, result, "expected error")
}

func TestCreateClone_Kerberos_Enabled_ACP_Disabled(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, _, _ := getStructsForCreateClone(ctx,
		driver, storagePool)
	sourceVolConfig.SnapshotDir = "false"
	sourceFilesystem.KerberosEnabled = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
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

func TestCreateClone_VolumeExistsResourceExhaustedError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceFilesystem, cloneFilesystem, _ := getStructsForCreateClone(ctx, driver,
		storagePool)
	cloneFilesystem.ProvisioningState = api.StateError
	cpLowSpaceError := errors.ResourceExhaustedError(
		errors.New("PoolSizeTooSmall: Unable to complete the operation. " +
			"The capacity pool size of 4398046511104 bytes is too small for the " +
			"combined volume size of 6597069766656 bytes of the capacity pool."))

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(true, cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateError, cpLowSpaceError).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, cloneFilesystem).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.IsType(t, errors.ResourceExhaustedError(errors.New("")), result, "not ResourceExhaustedError")
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
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

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
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"

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
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
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
	sourceVolConfig.SnapshotDir = "false"

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

func TestCreateClone_ZoneSelectionSucceeds(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NASType = "nfs"
	driver.Config.SupportedTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "2"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "3"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "4"},
	}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceFilesystem, cloneFilesystem, snapshot := getStructsForCreateClone(ctx,
		driver, storagePool)
	sourceVolConfig.RequisiteTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "3"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "4"},
	}
	sourceVolConfig.PreferredTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "3"},
	}
	sourceVolConfig.Zone = "3"
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	cloneVolConfig.RequisiteTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "2"},
	}
	cloneVolConfig.PreferredTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "eastus", "topology.kubernetes.io/zone": "1"},
	}
	sourceVolConfig.SnapshotDir = "false"
	createRequest.Zone = "3"
	sourceFilesystem.Zones = []string{"3"}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceFilesystem, nil).Times(1)
	mockAPI.EXPECT().VolumeExistsByID(ctx, cloneFilesystem.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceFilesystem, "snap1").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneFilesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneFilesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneFilesystem.ID, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func getStructsForImport(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *api.FileSystem) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	apiExportRule := api.ExportRule{
		AllowedClients:      defaultExportRule,
		Cifs:                false,
		Nfsv3:               false,
		Nfsv41:              false,
		RuleIndex:           1,
		UnixReadOnly:        false,
		UnixReadWrite:       false,
		Kerberos5ReadWrite:  false,
		Kerberos5PReadWrite: false,
		Kerberos5IReadWrite: false,
	}

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
		ExportPolicy:      api.ExportPolicy{Rules: []api.ExportRule{apiExportRule}},
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
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              false,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedWithKerberos5(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	originalName := "importMe"
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              true,
		Kerberos5ReadWrite:  true,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.KerberosEnabled = true
	originalFilesystem.ExportPolicy.Rules[0].Nfsv41 = true
	originalFilesystem.ExportPolicy.Rules[0].Kerberos5ReadWrite = true

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.True(t, originalFilesystem.ExportPolicy.Rules[0].Kerberos5ReadWrite, "kerberos protocol type mismatch")
}

func TestImport_ManagedWithKerberos5_ACP_Disabled(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.KerberosEnabled = true
	originalFilesystem.ExportPolicy.Rules[0].Nfsv41 = true
	originalFilesystem.ExportPolicy.Rules[0].Kerberos5ReadWrite = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
}

func TestImport_ManagedWithKerberos5I(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5i"

	originalName := "importMe"
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              true,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: true,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.KerberosEnabled = true
	originalFilesystem.ExportPolicy.Rules[0].Nfsv41 = true
	originalFilesystem.ExportPolicy.Rules[0].Kerberos5IReadWrite = true

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.True(t, originalFilesystem.ExportPolicy.Rules[0].Kerberos5IReadWrite, "kerberos protocol type mismatch")
}

func TestImport_ManagedWithKerberos5P(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5p"

	originalName := "importMe"
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              true,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: true,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.KerberosEnabled = true
	originalFilesystem.ExportPolicy.Rules[0].Nfsv41 = true
	originalFilesystem.ExportPolicy.Rules[0].Kerberos5PReadWrite = true

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.True(t, originalFilesystem.ExportPolicy.Rules[0].Kerberos5PReadWrite, "kerberos protocol type mismatch")
}

func TestImport_ManagedWithKerberos_IncorrectProtocolType(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	originalName := "importMe"
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              true,
		Kerberos5ReadWrite:  true,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.KerberosEnabled = true
	originalFilesystem.ExportPolicy.Rules[0].Nfsv41 = true
	originalFilesystem.ExportPolicy.Rules[0].Kerberos5ReadWrite = false
	originalFilesystem.ExportPolicy.Rules[0].Kerberos5IReadWrite = true

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).AnyTimes()

	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess,
		&exportRule).Return(errors.New("Could not import volume, volume modify failed.")).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.False(t, originalFilesystem.ExportPolicy.Rules[0].Kerberos5ReadWrite, "kerberos protocol type mismatch")
}

func TestImport_ManagedWithKerberosDisabled(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	originalName := "importMe"

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)
	originalFilesystem.KerberosEnabled = false

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	mockACP.EXPECT().IsFeatureEnabled(ctx, acp.FeatureInflightEncryption).Return(nil).AnyTimes()

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.False(t, originalFilesystem.KerberosEnabled, "kerberosEnabled flag is set")
}

func TestImport_ManagedWithKerberosNotSetInConfig(t *testing.T) {
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
	originalFilesystem.KerberosEnabled = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.True(t, originalFilesystem.KerberosEnabled, "kerberosEnabled flag is set")
}

func TestImport_ManagedWithSnapshotDir(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	exportRule := api.ExportRule{
		Nfsv41:              false,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	volConfig.SnapshotDir = "true"
	snapshotDirAccess := true

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedWithSnapshotDirFalse(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.UnixPermissions = "0770"
	driver.Config.NASType = "nfs"

	originalName := "importMe"

	exportRule := api.ExportRule{
		Nfsv41:              false,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	volConfig.SnapshotDir = "false"
	snapshotDirAccess := false

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalFilesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedWithInvalidSnapshotDirValue(t *testing.T) {
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

	volConfig.SnapshotDir = "xxxffa"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.NotNil(t, result, "received nil")
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
	var snapshotDirAccess bool

	exportRule := api.ExportRule{}

	volConfig, originalFilesystem := getStructsForSMBImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		nil, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

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
	var snapshotDirAccess bool

	exportRule := api.ExportRule{}

	volConfig, originalFilesystem := getStructsForSMBImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		nil, &snapshotDirAccess, &exportRule).Return(errors.New("unix permissions not applicable for SMB")).Times(1)

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

	assert.Error(t, result, "import did not fail")
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
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              false,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

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
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return(api.StateAvailable, nil).Times(1)

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
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              false,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(errFailed).Times(1)

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
	var snapshotDirAccess bool

	exportRule := api.ExportRule{
		Nfsv41:              false,
		Kerberos5ReadWrite:  false,
		Kerberos5IReadWrite: false,
		Kerberos5PReadWrite: false,
	}

	volConfig, originalFilesystem := getStructsForImport(ctx, driver)

	expectedLabels := map[string]string{
		drivers.TridentLabelTag: driver.getTelemetryLabels(ctx),
	}
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, originalName).Return(originalFilesystem, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalFilesystem).Return(nil).Times(1)
	mockAPI.EXPECT().ModifyVolume(ctx, originalFilesystem, expectedLabels,
		&expectedUnixPermissions, &snapshotDirAccess, &exportRule).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, originalFilesystem, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout(), api.Import).Return("", errFailed).Times(1)

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

	assert.Error(t, result, "import did not fail")
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
		driver.volumeCreateTimeout, api.Create).Return(api.StateAvailable, nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem, api.Create)

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
			driver.volumeCreateTimeout, api.Create).Return(state, errFailed).Times(1)

		result := driver.waitForVolumeCreate(ctx, filesystem, api.Create)

		assert.Error(t, result, "expected error")
		assert.IsType(t, errors.VolumeCreatingError(""), result, "not VolumeCreatingError")
	}
}

func TestWaitForVolumeCreate_DeletingDelete(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Delete).Return(api.StateDeleting, errFailed).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem, api.Delete)

	assert.NotNil(t, result, "not nil")
}

func TestWaitForVolumeCreate_ErrorDelete(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Create).Return(api.StateError, errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem, api.Create)

	assert.NotNil(t, result, "nil error")
}

func TestWaitForVolumeCreate_ErrorDeleteFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	filesystem := &api.FileSystem{
		Name:              "testvol1",
		CreationToken:     "netapp-testvol1",
		ProvisioningState: api.StateCreating,
	}

	// todo: check why should an error state not return an error
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout, api.Delete).Return(api.StateError, errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(errFailed).Times(1)

	result := driver.waitForVolumeCreate(ctx, filesystem, api.Delete)

	assert.NotNil(t, result, "nil error")
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
			driver.volumeCreateTimeout, api.Create).Return(state, errFailed).Times(1)

		result := driver.waitForVolumeCreate(ctx, filesystem, api.Create)

		assert.NotNil(t, result, "nil error")
	}
}

func getStructsForDestroyNFSVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem, *api.FileSystem, *api.Snapshot) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")
	srcVolumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "srcvol1")

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

	srcFilesystem := &api.FileSystem{
		ID:                srcVolumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testsrcvol1",
		FullName:          "RG1/NA1/CP1/testsrcvol1",
		Location:          Location,
		ExportPolicy:      exportPolicy,
		Labels:            make(map[string]string),
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testsrcvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
		NetworkFeatures:   api.NetworkFeaturesStandard,
	}

	snapshot := &api.Snapshot{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "srcvol1",
		Name:              "snap1",
		FullName:          "RG1/NA1/CP1/srcvol1/snap1",
		Location:          Location,
		Created:           time.Now(),
		SnapshotID:        SnapshotID,
		ProvisioningState: api.StateAvailable,
	}

	return volConfig, filesystem, srcFilesystem, snapshot
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

func TestDestroy_NFSVolume_Docker(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextDocker

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout(), api.Delete).Return(api.StateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_NFSVolume_CSI(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextCSI

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(0)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_Clone_DeleteAutomaticSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextCSI

	volConfig, filesystem, srcFilesystem, snapshot := getStructsForDestroyNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = "srcvol1"
	volConfig.CloneSourceSnapshotInternal = "snap1"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout(), api.Delete).Return(api.StateDeleted, nil).Times(1)
	mockAPI.EXPECT().VolumeByID(ctx, srcFilesystem.ID).Return(srcFilesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, srcFilesystem, volConfig.CloneSourceSnapshotInternal).Return(snapshot,
		nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, srcFilesystem, snapshot).Return(nil).Times(1)

	err := driver.Destroy(ctx, volConfig)

	assert.Nil(t, err, "not nil")
}

func TestDestroy_Clone_DeleteAutomaticSnapshot_VolDeletingState(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextCSI

	volConfig, filesystem, srcFilesystem, snapshot := getStructsForDestroyNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = "srcvol1"
	volConfig.CloneSourceSnapshotInternal = "snap1"
	filesystem.ProvisioningState = api.StateDeleting

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout(), api.Delete).Return(api.StateDeleted, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().VolumeByID(ctx, srcFilesystem.ID).Return(srcFilesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, srcFilesystem, volConfig.CloneSourceSnapshotInternal).Return(snapshot,
		nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, srcFilesystem, snapshot).Return(nil).Times(1)

	err := driver.Destroy(ctx, volConfig)

	assert.Nil(t, err, "not nil")
}

func TestDestroy_Clone_DeleteAutomaticSnapshot_VolDeleteError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextCSI

	volConfig, _, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = "srcvol1"
	volConfig.CloneSourceSnapshotInternal = "snap1"

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil,
		errors.NotFoundError("volume not found")).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any()).Times(0)

	err := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, err)
}

func TestDestroy_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_AlreadyDeleted(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_StillDeletingDeleted_Docker(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextDocker

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
	filesystem.ProvisioningState = api.StateDeleting

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		tridentconfig.DockerDefaultTimeout, api.Delete).Return(api.StateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_StillDeleting_Docker(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextDocker

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
	filesystem.ProvisioningState = api.StateDeleting

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		tridentconfig.DockerDefaultTimeout, api.Delete).Return(api.StateDeleting, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_StillDeleting_CSI(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
	filesystem.ProvisioningState = api.StateDeleting

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_VolumeWaitFailed_Docker(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextDocker

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, filesystem).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, filesystem, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout(), api.Delete).Return(api.StateDeleting, errFailed).Times(1)

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

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func getStructsForDeleteAutomaticSnapshot(ctx context.Context, driver *NASStorageDriver) (
	srcVolume *api.FileSystem, cloneVolumeConfig *storage.VolumeConfig,
	cloneVolume *api.FileSystem, snapshot *api.Snapshot,
) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")
	srcVolumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testsrcvol1")

	cloneVolumeConfig = &storage.VolumeConfig{
		Version:                     "1",
		Name:                        "testvol1",
		InternalName:                "trident-testvol1",
		Size:                        VolumeSizeStr,
		InternalID:                  volumeID,
		CloneSourceSnapshotInternal: "snap1",
		CloneSourceVolume:           "testsrcvol1",
		CloneSourceVolumeInternal:   "testsrcvol1",
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

	cloneVolume = &api.FileSystem{
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

	srcVolume = &api.FileSystem{
		ID:                srcVolumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testsrcvol1",
		FullName:          "RG1/NA1/CP1/testsrcvol1",
		Location:          Location,
		ExportPolicy:      exportPolicy,
		Labels:            make(map[string]string),
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testsrcvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		SubnetID:          subnetID,
		UnixPermissions:   defaultUnixPermissions,
		NetworkFeatures:   api.NetworkFeaturesStandard,
	}

	snapshot = &api.Snapshot{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testsrcvol1",
		Name:              "snap1",
		FullName:          "RG1/NA1/CP1/testsrcvol1/snap1",
		Location:          Location,
		Created:           time.Now(),
		SnapshotID:        SnapshotID,
		ProvisioningState: api.StateAvailable,
	}

	return srcVolume, cloneVolumeConfig, cloneVolume, snapshot
}

func Test_DeleteAutomaticSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	srcVol, cloneVolConfig, _, snapshot := getStructsForDeleteAutomaticSnapshot(ctx, driver)

	mockAPI.EXPECT().VolumeByID(ctx, srcVol.ID).Return(srcVol, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, srcVol, cloneVolConfig.CloneSourceSnapshotInternal).Return(snapshot,
		nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, srcVol, snapshot).Return(nil).Times(1)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func Test_DeleteAutomaticSnapshot_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	_, cloneVolConfig, _, _ := getStructsForDeleteAutomaticSnapshot(ctx, driver)
	cloneVolConfig.CloneSourceSnapshotInternal = ""

	mockAPI.EXPECT().VolumeByID(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), gomock.Any()).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any()).Times(0)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func Test_DeleteAutomaticSnapshot_ExistingSrcSnapshot(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	_, cloneVolConfig, _, _ := getStructsForDeleteAutomaticSnapshot(ctx, driver)
	cloneVolConfig.CloneSourceSnapshot = cloneVolConfig.CloneSourceSnapshotInternal

	mockAPI.EXPECT().VolumeByID(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), gomock.Any()).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any()).Times(0)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func Test_DeleteAutomaticSnapshot_VolDeleteError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	_, cloneVolConfig, _, _ := getStructsForDeleteAutomaticSnapshot(ctx, driver)

	mockAPI.EXPECT().VolumeByID(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), gomock.Any()).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any()).Times(0)

	driver.deleteAutomaticSnapshot(ctx, errors.New("volume delete error"), cloneVolConfig)
}

func Test_DeleteAutomaticSnapshot_VolByIDError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	srcVol, cloneVolConfig, _, _ := getStructsForDeleteAutomaticSnapshot(ctx, driver)

	tests := []struct {
		name            string
		volumeByIDError error
	}{
		{
			name:            "Fails when volume not found for ID",
			volumeByIDError: errors.NotFoundError("not found"),
		},
		{
			name:            "Fails when volume for ID returns error other than not found",
			volumeByIDError: errors.New("other error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().VolumeByID(ctx, srcVol.ID).Return(nil,
				test.volumeByIDError).Times(1)
			mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), cloneVolConfig.CloneSourceSnapshotInternal).Times(0)
			mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any()).Times(0)

			driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
		})
	}
}

func Test_DeleteAutomaticSnapshot_SnapshotForVolumeError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	srcVol, cloneVolConfig, _, _ := getStructsForDeleteAutomaticSnapshot(ctx, driver)

	tests := []struct {
		name                   string
		snapshotForVolumeError error
	}{
		{
			name:                   "Fails when snapshot for volume not found",
			snapshotForVolumeError: errors.NotFoundError("not found"),
		},
		{
			name:                   "Fails when snapshot for volume returns error other than not found",
			snapshotForVolumeError: errors.New("other error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().VolumeByID(ctx, srcVol.ID).Return(srcVol,
				nil).Times(1)
			mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(),
				cloneVolConfig.CloneSourceSnapshotInternal).Return(nil, test.snapshotForVolumeError).Times(1)
			mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any()).Times(0)

			driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
		})
	}
}

func Test_DeleteAutomaticSnapshot_DeleteSnapshotError(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	srcVol, cloneVolConfig, _, snapshot := getStructsForDeleteAutomaticSnapshot(ctx, driver)

	mockAPI.EXPECT().VolumeByID(ctx, srcVol.ID).Return(srcVol, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, srcVol, cloneVolConfig.CloneSourceSnapshotInternal).Return(snapshot,
		nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, srcVol, snapshot).Return(errors.New("delete failed")).Times(1)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func getStructsForPublishNFSVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem, *models.VolumePublishInfo) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
		AccessInfo: models.VolumeAccessInfo{
			NfsAccessInfo: models.NfsAccessInfo{
				NfsPath: "/trident-testvol1",
			},
		},
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	mountTargets := []api.MountTarget{
		{
			MountTargetID: "mountTargetID",
			FileSystemID:  "filesystemID",
			IPAddress:     "1.1.1.1",
			ServerFqdn:    "",
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

	publishInfo := &models.VolumePublishInfo{}

	return volConfig, filesystem, publishInfo
}

func getStructsForPublishSMBVolume(
	ctx context.Context, driver *NASStorageDriver,
) (*storage.VolumeConfig, *api.FileSystem, *models.VolumePublishInfo) {
	subnetID := api.CreateSubnetID(SubscriptionID, "RG2", "VN1", "SN1")
	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "importMe")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
		InternalID:   volumeID,
		AccessInfo: models.VolumeAccessInfo{
			SMBAccessInfo: models.SMBAccessInfo{
				SMBPath: "\\trident-testvol1",
			},
		},
	}

	labels := make(map[string]string)
	labels[drivers.TridentLabelTag] = driver.getTelemetryLabels(ctx)
	labels[storage.ProvisioningLabelTag] = ""

	mountTargets := []api.MountTarget{
		{
			MountTargetID: "mountTargetID",
			FileSystemID:  "filesystemID",
			IPAddress:     "1.1.1.1",
			ServerFqdn:    "trident-1234.trident.com",
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

	publishInfo := &models.VolumePublishInfo{}

	return volConfig, filesystem, publishInfo
}

func TestPublish_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath

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

func TestPublish_NFSVolume_Kerberos_Type5(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	filesystem.MountTargets[0].ServerFqdn = "trident-1234.trident.com"
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	filesystem.KerberosEnabled = true

	volConfig.MountOptions = "nfsvers=4.1"

	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, "/trident-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "trident-1234.trident.com", publishInfo.NfsServerIP, "server fqdn mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=4.1", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_NFSVolume_Kerberos_Type5_NotSet(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	filesystem.MountTargets[0].ServerFqdn = "trident-1234.trident.com"
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	filesystem.KerberosEnabled = false

	volConfig.MountOptions = "nfsvers=4.1"

	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.NotEqual(t, "trident-1234.trident.com", publishInfo.NfsServerIP, "server fqdn mismatch")
}

func TestPublish_ROClone_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"

	volConfig, filesystem, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.AccessInfo.NfsPath = "/trident-testvol1/.snapshot/" + SnapshotID
	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal).Return(filesystem, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.NoError(t, result, "error occurred")
	assert.Equal(t, filesystem.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, "/trident-testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", publishInfo.NfsPath,
		"NFS path mismatch")
	assert.Equal(t, "1.1.1.1", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=3", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "smb"

	volConfig, filesystem, publishInfo := getStructsForPublishSMBVolume(ctx, driver)
	publishInfo.SMBPath = volConfig.AccessInfo.SMBPath

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

func TestPublish_ROClone_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.AccessInfo.NfsPath = "/trident-testvol1/.snapshot/" + SnapshotID
	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal).Return(nil, errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NotNil(t, result, "got nil")
	assert.Error(t, result, "expected error")
	assert.NotEqual(t, "", publishInfo.NfsPath, "NFS path is empty")
	assert.Equal(t, "/trident-testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", publishInfo.NfsPath,
		"NFS path mismatch")
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
		SnapshotID:        SnapshotID,
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
			SnapshotID:        SnapshotID,
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
			SnapshotID:        SnapshotID,
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
		Created:   snapTime.UTC().Format(utils.TimestampFormat),
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
		Created:   snapTime.UTC().Format(utils.TimestampFormat),
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
		Created:   snapTime.UTC().Format(utils.TimestampFormat),
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
		[]string{api.StateError, api.StateDeleting, api.StateDeleted}, api.DefaultSDKTimeout, api.Restore).
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
		api.DefaultSDKTimeout, api.Restore).Return("", errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDeleteSnapshot_Docker(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextDocker

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

func TestDeleteSnapshot_CSI(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	snapTime := time.Now()
	volConfig, filesystem, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, filesystem, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, filesystem, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, filesystem, snapshot).Return(nil).Times(1)

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

func TestDeleteSnapshot_SnapshotWaitFailed_Docker(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.DriverContext = tridentconfig.ContextDocker

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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

	volConfig, filesystem, _, _ := getStructsForDestroyNFSVolume(ctx, driver)
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

func TestOntapSanStorageDriverGetStorageBackendPools(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.Config.SubscriptionID = "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b"
	resourceGroup := "RG1"
	account := "NA1"
	location := "fake-location"
	pools := []*api.CapacityPool{
		{
			Name:          "CP1",
			Location:      location,
			NetAppAccount: account,
			ResourceGroup: resourceGroup,
		},
		{
			Name:          "CP2",
			Location:      location,
			NetAppAccount: account,
			ResourceGroup: resourceGroup,
		},
	}

	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return(pools)

	backendPools := driver.getStorageBackendPools(ctx)
	assert.Equal(t, len(pools), len(backendPools))

	// expected is an api.CapacityPools which contains all information present in the actual ANFStorageBackendPool
	// (except for the subscription ID).
	expected, actual := pools[0], backendPools[0]
	assert.NotNil(t, expected)
	assert.NotNil(t, actual)
	assert.Equal(t, driver.Config.SubscriptionID, actual.SubscriptionID)
	assert.Equal(t, expected.ResourceGroup, actual.ResourceGroup)
	assert.Equal(t, expected.NetAppAccount, actual.NetappAccount)
	assert.Equal(t, expected.Location, actual.Location)
	assert.Equal(t, expected.Name, actual.CapacityPool)

	expected, actual = pools[1], backendPools[1]
	assert.NotNil(t, expected)
	assert.NotNil(t, actual)
	assert.Equal(t, driver.Config.SubscriptionID, actual.SubscriptionID)
	assert.Equal(t, expected.ResourceGroup, actual.ResourceGroup)
	assert.Equal(t, expected.NetAppAccount, actual.NetappAccount)
	assert.Equal(t, expected.Location, actual.Location)
	assert.Equal(t, expected.Name, actual.CapacityPool)
}

func TestCreatePrepare(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["anf_pool"]
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	driver.CreatePrepare(ctx, volConfig, storagePool)

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

	storagePool := driver.pools["anf_pool"]
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	result := driver.GetInternalVolumeName(ctx, volConfig, storagePool)

	assert.Equal(t, "myPrefix-testvol1", result, "internal name mismatch")
}

func TestGetInternalVolumeName_CSI(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["anf_pool"]
	volConfig := &storage.VolumeConfig{Name: "pvc-5e522901-b891-41d8-9e83-5496d2e62e71"}

	result := driver.GetInternalVolumeName(ctx, volConfig, storagePool)

	assert.Equal(t, "pvc-5e522901-b891-41d8-9e83-5496d2e62e71", result, "internal name mismatch")
}

func TestGetInternalVolumeName_NonCSI(t *testing.T) {
	_, driver := newMockANFDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["anf_pool"]
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	result := driver.GetInternalVolumeName(ctx, volConfig, storagePool)

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

func TestCreateFollowup_NFSVolume_Kerberos_Type5(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)

	filesystem.MountTargets[0].ServerFqdn = "trident-1234.trident.com"
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	filesystem.KerberosEnabled = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, (filesystem.MountTargets)[0].ServerFqdn, volConfig.AccessInfo.NfsServerIP,
		"server fqdn mismatch")
	assert.Equal(t, "/"+filesystem.CreationToken, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "nfs", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_NFSVolume_Kerberos_Type5_NotSet(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"
	driver.Config.Kerberos = "sec=krb5"

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)

	filesystem.MountTargets[0].ServerFqdn = "trident-1234.trident.com"
	filesystem.ProtocolTypes = []string{api.ProtocolTypeNFSv41}
	filesystem.KerberosEnabled = false

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NotEqual(t, (filesystem.MountTargets)[0].ServerFqdn, volConfig.AccessInfo.NfsServerIP,
		"server fqdn mismatch")
}

func TestCreateFollowup_ROClone_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "nfs"

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.CloneSourceSnapshot = SnapshotID
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NoError(t, result, "error occurred")
	assert.Equal(t, (filesystem.MountTargets)[0].IPAddress, volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "/testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", volConfig.AccessInfo.NfsPath,
		"NFS path mismatch")
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
	assert.Equal(t, (filesystem.MountTargets)[0].ServerFqdn, volConfig.AccessInfo.SMBServer, "SMB server mismatch")
	assert.Equal(t, "\\"+filesystem.CreationToken, volConfig.AccessInfo.SMBPath, "SMB path mismatch")
	assert.Equal(t, "smb", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_ROClone_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	driver.Config.NASType = "smb"

	volConfig, filesystem, _ := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.CloneSourceSnapshot = SnapshotID
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal).Return(filesystem, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NoError(t, result, "error occurred")
	assert.Equal(t, (filesystem.MountTargets)[0].ServerFqdn, volConfig.AccessInfo.SMBServer, "SMB server mismatch")
	assert.Equal(t, "\\testvol1\\~snapshot\\deadbeef-5c0d-4afa-8cd8-afa3fba5665c", volConfig.AccessInfo.SMBPath,
		"SMB path mismatch")
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
	assert.NotEqual(t, "", volConfig.AccessInfo.NfsPath, "NFS path is empty")
	assert.Equal(t, "/"+volConfig.InternalName, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
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
	assert.NotEqual(t, "", volConfig.AccessInfo.NfsPath, "NFS path is empty")
	assert.Equal(t, "/"+volConfig.InternalName, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_ROClone_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)

	volConfig, _, _ := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, volConfig.CloneSourceVolumeInternal).Return(nil,
		errors.NotFoundError("not found")).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NotNil(t, result, "received nil")
	assert.Error(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.NotEqual(t, "", volConfig.AccessInfo.NfsPath, "NFS path is empty")
	assert.Equal(t, "/"+volConfig.InternalName, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
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
		assert.NotEqual(t, "", volConfig.AccessInfo.NfsPath, "NFS path is empty")
		assert.Equal(t, "/"+volConfig.InternalName, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
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
	assert.NotEqual(t, "", volConfig.AccessInfo.NfsPath, "NFS path is empty")
	assert.Equal(t, "/"+volConfig.InternalName, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
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

func TestGetVolumeForImport(t *testing.T) {
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

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "testvol1", result.Config.Name)
	assert.Equal(t, "myPrefix-testvol1", result.Config.InternalName)
}

func TestGetVolumeForImport_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "error expected")
}

func TestGetVolumeForImport_GetFailed(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByCreationToken(ctx, "testvol1").Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

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

func TestValidateNetworkFeatures(t *testing.T) {
	tests := []struct {
		name            string
		networkFeatures string
		encryptionKeys  map[string]string
		expectedError   bool
	}{
		{
			name:            "standard network feature and encryption keys",
			networkFeatures: api.NetworkFeaturesStandard,
			encryptionKeys:  map[string]string{"key1": "value1", "key2": "value2"},
			expectedError:   false,
		},
		{
			name:            "no network feature and encryption keys",
			networkFeatures: "",
			encryptionKeys:  map[string]string{"key1": "value1", "key2": "value2"},
			expectedError:   false,
		},
		{
			name:            "basic network feature and valid encryption keys",
			networkFeatures: api.NetworkFeaturesBasic,
			encryptionKeys:  map[string]string{"key1": "value1", "key2": "value2"},
			expectedError:   true,
		},
		{
			name:            "standard network feature and no encryption keys",
			networkFeatures: api.NetworkFeaturesStandard,
			encryptionKeys:  map[string]string{},
			expectedError:   false,
		},
		{
			name:            "no network feature and no encryption keys",
			networkFeatures: "",
			encryptionKeys:  map[string]string{},
			expectedError:   false,
		},
		{
			name:            "basic networking and no encryption keys",
			networkFeatures: api.NetworkFeaturesBasic,
			encryptionKeys:  map[string]string{},
			expectedError:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateNetworkFeatures(test.networkFeatures, test.encryptionKeys)
			if test.expectedError {
				assert.Error(t, err, "expected error because of invalid combination")
			} else {
				assert.NoError(t, err, "unexpected error for a valid combination")
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

func TestConstructVolumeAccessPath(t *testing.T) {
	_, driver := newMockANFDriver(t)
	driver.initializeTelemetry(ctx, BackendUUID)
	volConfig, fileSystem, _ := getStructsForPublishNFSVolume(ctx, driver)
	tests := []struct {
		Protocol string
		Valid    bool
	}{
		// Valid protocols
		{
			Protocol: "nfs",
			Valid:    true,
		},
		{
			Protocol: "smb",
			Valid:    true,
		},

		// Invalid protocols
		{
			Protocol: "abc",
			Valid:    false,
		},
		{
			Protocol: "",
			Valid:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.Protocol, func(t *testing.T) {
			result := constructVolumeAccessPath(volConfig, fileSystem, test.Protocol)
			if test.Valid {
				assert.NotEmpty(t, result, "access path should not be empty")
			} else {
				assert.Empty(t, result, "access path should be empty")
			}
		})
	}
}

func TestUpdate_Success(t *testing.T) {
	defer acp.SetAPI(acp.API())

	// Create mocks
	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Set up driver and populate defaults
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	// Set right expects
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).AnyTimes()
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).AnyTimes()
	mockAPI.EXPECT().ModifyVolume(ctx, filesystem, gomock.Any(), gomock.Any(), utils.Ptr(true),
		gomock.Any()).Return(nil).AnyTimes()

	updateInfo := &models.VolumeUpdateInfo{SnapshotDirectory: "TRUE"}
	allVolumes := map[string]*storage.Volume{
		volConfig.Name: {
			Config:      volConfig,
			BackendUUID: BackendUUID,
			Pool:        "anf_pool",
			Orphaned:    false,
			State:       "Online",
		},
	}

	result, err := driver.Update(ctx, volConfig, updateInfo, allVolumes)

	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result))
	for _, v := range result {
		assert.Equal(t, "true", v.Config.SnapshotDir)
	}
}

func TestUpdate_NilVolumeUpdateInfo(t *testing.T) {
	_, driver := newMockANFDriver(t)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	allVolumes := map[string]*storage.Volume{
		volConfig.InternalName: {
			Config:      volConfig,
			BackendUUID: BackendUUID,
			Pool:        "anf_pool",
			Orphaned:    false,
			State:       "Online",
		},
	}

	result, err := driver.Update(ctx, volConfig, nil, allVolumes)

	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.True(t, errors.IsInvalidInputError(err))
}

func TestUpdate_ErrorInAPIOperation(t *testing.T) {
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockError := errors.New("mock error")

	// CASE 1: Error in refreshing volume
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(mockError).AnyTimes()

	updateInfo := &models.VolumeUpdateInfo{SnapshotDirectory: "TRUE"}
	allVolumes := map[string]*storage.Volume{
		volConfig.InternalName: {
			Config:      volConfig,
			BackendUUID: BackendUUID,
			Pool:        "anf_pool",
			Orphaned:    false,
			State:       "Online",
		},
	}

	result, err := driver.Update(ctx, volConfig, updateInfo, allVolumes)

	assert.Nil(t, result)
	assert.NotNil(t, err)

	// CASE 2: Error in getting volume
	mockAPI, driver = newMockANFDriver(t)
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).AnyTimes()
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errors.New("error in getting volume")).AnyTimes()

	result, err = driver.Update(ctx, volConfig, updateInfo, allVolumes)

	assert.Nil(t, result)
	assert.NotNil(t, err)
}

func TestUpdate_VolumeStateNotAvailable(t *testing.T) {
	mockAPI, driver := newMockANFDriver(t)

	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	filesystem.ProvisioningState = api.StateError

	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).AnyTimes()
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).AnyTimes()

	updateInfo := &models.VolumeUpdateInfo{SnapshotDirectory: "TRUE"}
	allVolumes := map[string]*storage.Volume{
		volConfig.InternalName: {
			Config:      volConfig,
			BackendUUID: BackendUUID,
			Pool:        "anf_pool",
			Orphaned:    false,
			State:       "Online",
		},
	}

	result, err := driver.Update(ctx, volConfig, updateInfo, allVolumes)

	assert.Nil(t, result)
	assert.NotNil(t, err)
}

func TestUpdateSnapshotDirectory_DifferentSnapshotDir(t *testing.T) {
	defer acp.SetAPI(acp.API())

	tests := []struct {
		name          string
		inputSnapDir  string
		outputSnapDir string
		expectErr     bool
	}{
		{
			name:          "SnapshotDir to true",
			inputSnapDir:  "TRUE",
			outputSnapDir: "true",
		},
		{
			name:          "SnapshotDir to false",
			inputSnapDir:  "False",
			outputSnapDir: "false",
		},
		{
			name:         "Invalid SnapshotDir",
			inputSnapDir: "tRUe",
			expectErr:    true,
		},
	}

	// Create mocks
	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Set up driver and populate defaults
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	// Set right expects
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).AnyTimes()
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).AnyTimes()
	mockAPI.EXPECT().ModifyVolume(ctx, filesystem, gomock.Any(), gomock.Any(), utils.Ptr(true),
		gomock.Any()).Return(nil).AnyTimes()

	allVolumes := map[string]*storage.Volume{
		volConfig.InternalName: {
			Config:      volConfig,
			BackendUUID: BackendUUID,
			Pool:        "anf_pool",
			Orphaned:    false,
			State:       "Online",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			result, err := driver.updateSnapshotDirectory(
				ctx, volConfig.InternalName, filesystem, test.inputSnapDir, allVolumes)

			if test.expectErr {
				assert.Nil(tt, result)
				assert.NotNil(tt, err)
				assert.True(tt, errors.IsInvalidInputError(err))
			} else {
				assert.Nil(t, err)
				assert.NotNil(tt, result)
				assert.Equal(t, 1, len(result))
				for _, v := range result {
					assert.Equal(t, test.outputSnapDir, v.Config.SnapshotDir)
				}
			}
		})
	}
}

func TestUpdateSnapshotDirectory_Failure(t *testing.T) {
	defer acp.SetAPI(acp.API())

	// Create mocks
	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockANFDriver(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Set up driver and populate defaults
	driver.Config.BackendName = "anf"
	driver.Config.ServiceLevel = api.ServiceLevelUltra
	driver.Config.NetworkFeatures = api.NetworkFeaturesStandard
	driver.Config.NASType = "nfs"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, BackendUUID)

	storagePool := driver.pools["anf_pool"]

	volConfig, _, _, _, filesystem := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	// CASE: Error while modifying volume
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).AnyTimes()
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).AnyTimes()
	mockAPI.EXPECT().ModifyVolume(
		ctx, filesystem, gomock.Any(), gomock.Any(), utils.Ptr(true), gomock.Any()).Return(
		errors.New("mock error")).AnyTimes()

	allVolumes := map[string]*storage.Volume{
		volConfig.InternalName: {
			Config:      volConfig,
			BackendUUID: BackendUUID,
			Pool:        "anf_pool",
			Orphaned:    false,
			State:       "Online",
		},
	}

	result, err := driver.updateSnapshotDirectory(
		ctx, volConfig.InternalName, filesystem, "TRUE", allVolumes)

	assert.Nil(t, result)
	assert.NotNil(t, err)

	// CASE: Volume not found in backend cache
	mockAPI, driver = newMockANFDriver(t)
	mockAPI.EXPECT().RefreshAzureResources(ctx).Return(nil).AnyTimes()
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(filesystem, nil).AnyTimes()
	mockAPI.EXPECT().ModifyVolume(
		ctx, filesystem, gomock.Any(), gomock.Any(), utils.Ptr(true), gomock.Any()).Return(nil).AnyTimes()

	allVolumes = map[string]*storage.Volume{}

	filesystem.SnapshotDirectory = false
	result, err = driver.updateSnapshotDirectory(
		ctx, volConfig.InternalName, filesystem, "TRUE", allVolumes)

	assert.Nil(t, result)
	assert.NotNil(t, err)
}
