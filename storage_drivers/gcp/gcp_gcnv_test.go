// Copyright 2025 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_gcp"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	storagefake "github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/gcp/gcnvapi"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var (
	ctx                  = context.Background()
	errFailed            = errors.New("failed")
	debugTraceFlags      = map[string]bool{"method": true, "api": true, "discovery": true}
	DefaultVolumeSize, _ = strconv.ParseInt(defaultVolumeSizeStr, 10, 64)
)

func newTestGCNVDriver(mockAPI gcnvapi.GCNV) *NASStorageDriver {
	prefix := "test-"

	config := drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: "google-cloud-netapp-volumes",
			StoragePrefix:     &prefix,
			DebugTraceFlags:   debugTraceFlags,
		},
		ProjectNumber: gcnvapi.ProjectNumber,
		Location:      gcnvapi.Location,
		APIKey: drivers.GCPPrivateKey{
			Type:                    gcnvapi.Type,
			ProjectID:               gcnvapi.ProjectID,
			PrivateKeyID:            gcnvapi.PrivateKeyID,
			PrivateKey:              gcnvapi.PrivateKey,
			ClientEmail:             gcnvapi.ClientEmail,
			ClientID:                gcnvapi.ClientID,
			AuthURI:                 gcnvapi.AuthURI,
			TokenURI:                gcnvapi.TokenURI,
			AuthProviderX509CertURL: gcnvapi.AuthProviderX509CertURL,
			ClientX509CertURL:       gcnvapi.ClientX509CertURL,
		},
		VolumeCreateTimeout: "10",
	}

	return &NASStorageDriver{
		Config:              config,
		API:                 mockAPI,
		volumeCreateTimeout: 10 * time.Second,
	}
}

func newMockGCNVDriver(t *testing.T) (*mockapi.MockGCNV, *NASStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockGCNV(mockCtrl)

	return mockAPI, newTestGCNVDriver(mockAPI)
}

func TestName(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	result := driver.Name()
	assert.Equal(t, tridentconfig.GCNVNASStorageDriverName, result, "driver name mismatch")
}

func TestBackendName_SetInConfig(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "myGCNVBackend"

	result := driver.BackendName()
	assert.Equal(t, "myGCNVBackend", result, "backend name mismatch")
}

func TestBackendName_UseDefault(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = ""

	result := driver.BackendName()
	assert.Equal(t, "googlecloudnetappvolumes_12345", result, "backend name mismatch")
}

func TestDefaultBackendName_WorkloadIdentity(t *testing.T) {
	_, driver := newMockGCNVDriver(t)
	driver.Config.APIKey = drivers.GCPPrivateKey{}
	result := driver.BackendName()
	assert.NotNil(t, result, "received nil for the backend name")
}

func TestPoolName(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "myGCNVBackend"

	result := driver.poolName("pool-1-A")
	assert.Equal(t, "myGCNVBackend_pool1A", result, "pool name mismatch")
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
		{"Volume_1-A", false},
		{"volume-", false},
		{"volume_", false},
		// Valid names
		{"v", true},
		{"volume1", true},
		{"x23456789012345678901234567890123456789012345678901234567890123", true},
		{"volume-1-a", true},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			_, driver := newMockGCNVDriver(t)

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
	}
	for _, test := range tests {
		t.Run(test.Token, func(t *testing.T) {
			_, driver := newMockGCNVDriver(t)

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
		{tridentconfig.ContextCSI, gcnvapi.VolumeCreateTimeout},
		{"", gcnvapi.VolumeCreateTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockGCNVDriver(t)
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
		{tridentconfig.ContextCSI, gcnvapi.DefaultTimeout},
		{"", gcnvapi.DefaultTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockGCNVDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultTimeout()
			assert.Equal(t, test.Expected, result, "mismatched durations")
		})
	}
}

func TestGCNVInitialize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	capacityPool := &gcnvapi.CapacityPool{
		Name:            "CP1",
		FullName:        "CP-premium-pool",
		Location:        gcnvapi.Location,
		ServiceLevel:    gcnvapi.ServiceLevelPremium,
		State:           gcnvapi.StateReady,
		NetworkName:     gcnvapi.NetworkName,
		NetworkFullName: gcnvapi.NetworkFullName,
	}

	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.Equal(t, "trident-", *driver.Config.StoragePrefix, "wrong storage prefix")
	assert.Equal(t, 1, len(driver.pools), "wrong number of pools")
	assert.Equal(t, gcnvapi.BackendUUID, driver.telemetry.TridentBackendUUID, "wrong backend UUID")
	assert.Equal(t, driver.volumeCreateTimeout, 10*time.Second, "volume create timeout mismatch")
	assert.True(t, driver.Initialized(), "not initialized")
}

func TestGCNVInitialize_WithSecrets(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	secrets := map[string]string{
		"private_key_id": gcnvapi.PrivateKeyID,
		"private_key":    gcnvapi.PrivateKey,
	}

	capacityPool := &gcnvapi.CapacityPool{
		Name:            "CP1",
		FullName:        "CP-premium-pool",
		Location:        gcnvapi.Location,
		ServiceLevel:    gcnvapi.ServiceLevelPremium,
		State:           gcnvapi.StateReady,
		NetworkName:     gcnvapi.NetworkName,
		NetworkFullName: gcnvapi.NetworkFullName,
	}

	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		secrets, gcnvapi.BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.True(t, driver.Initialized(), "not initialized")
}

func TestGCNVInitialize_WithInvalidSecrets_WrongKeyName(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	secrets := map[string]string{
		"private_key_id": gcnvapi.PrivateKeyID,
		"private-key":    gcnvapi.PrivateKey,
	}

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		secrets, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_WithInvalidSecrets_WrongKeyID(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	secrets := map[string]string{
		"private-key_id": gcnvapi.PrivateKeyID,
		"private_key":    gcnvapi.PrivateKey,
	}

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		secrets, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidConfigJSON(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
    }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.NotNil(t, driver.Config.CommonStorageDriverConfig, "Driver Config not set")
	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidConfigValue(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "volumeCreateTimeout": "yes"
    }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.NotNil(t, driver.Config.CommonStorageDriverConfig, "Driver Config not set")
	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_APISetToNil(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	driver.API = nil

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidSDKTimeout(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40s"
    }`

	driver.API = nil

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidMaxCacheAge(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300s"
    }`

	driver.API = nil

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidStoragePrefix(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	storagePrefix := "&trident"

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		StoragePrefix:     &storagePrefix,
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidServiceLevel(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "ultra-premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300"
    }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidExportRule(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "exportRule": "10.10.10.10/128"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidSnapshotDir(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "snapshotDir": "true$"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidSnapshotReserve(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "snapshotReserve": "true"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidSnapshotReserve_GreaterThan90(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "snapshotReserve": "91"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidSnapshotReserve_LesserThan0(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "snapshotReserve": "-1"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidUnixPermissions(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "unixPermissions": "999"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestGCNVInitialize_InvalidPoolAttribute_InvalidDefaultVolumeSize(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "google-cloud-netapp-volumes",
        "location": "fake-location",
        "serviceLevel": "premium",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
	    "storagePool": ["premium-test-pool"],
        "sdkTimeout": "40",
        "maxCacheAge": "300",
        "storage": [{ "defaults": { "size": "true"}}]
       }`

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig,
		map[string]string{}, gcnvapi.BackendUUID)

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
			_, driver := newMockGCNVDriver(t)
			driver.initialized = test.Expected

			result := driver.Initialized()

			assert.Equal(t, test.Expected, result, "mismatched initialized values")
		})
	}
}

func TestGCNVTerminate(t *testing.T) {
	_, driver := newMockGCNVDriver(t)
	driver.initialized = true

	driver.Terminate(ctx, "")
	assert.False(t, driver.initialized, "initialized not false")
}

func TestPopulateConfigurationDefaults_NoneSet(t *testing.T) {
	config := &drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
		},
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config = *config

	result := driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.NoError(t, result, " error occurred")
	assert.Equal(t, "trident-", *driver.Config.StoragePrefix, "storage prefix mismatch")
	assert.Equal(t, drivers.DefaultVolumeSize, driver.Config.Size, "size mismatch")
	assert.Equal(t, gcnvapi.ServiceLevelStandard, driver.Config.ServiceLevel, "service level mismatch")
	assert.Equal(t, defaultStorageClass, driver.Config.StorageClass, "storage class mismatch")
	assert.Equal(t, "", driver.Config.NFSMountOptions, "NFS mount options mismatch")
	assert.Equal(t, "", driver.Config.SnapshotDir, "snapshot dir mismatch")
	assert.Equal(t, defaultSnapshotReserve, driver.Config.SnapshotReserve, "snapshot reserve mismatch")
	assert.Equal(t, defaultUnixPermissions, driver.Config.UnixPermissions, "unix permissions mismatch")
	assert.Equal(t, defaultLimitVolumeSize, driver.Config.LimitVolumeSize, "limit volume size mismatch")
	assert.Equal(t, defaultExportRule, driver.Config.ExportRule, "export rule mismatch")
	assert.Equal(t, sa.NFS, driver.Config.NASType, "NAS type mismatch")
}

func TestPopulateConfigurationDefaults_AllSet(t *testing.T) {
	prefix := "myPrefix"

	config := &drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			StoragePrefix:   &prefix,
			LimitVolumeSize: "123456789000",
		},
		NFSMountOptions:     "nfsvers=4.1",
		VolumeCreateTimeout: "30",
		GCNVNASStorageDriverPool: drivers.GCNVNASStorageDriverPool{
			GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				UnixPermissions: "0700",
				SnapshotDir:     "true",
				SnapshotReserve: "1456898458",
				ExportRule:      "1.1.1.1/32",
			},
			StorageClass: "software",
			ServiceLevel: "premium",
			NASType:      "smb",
		},
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config = *config

	result := driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.NoError(t, result, " error occurred")
	assert.Equal(t, "myPrefix", *driver.Config.StoragePrefix, "storage prefix mismatch")
	assert.Equal(t, "1234567890", driver.Config.Size, "size mismatch")
	assert.Equal(t, "premium", driver.Config.ServiceLevel, "service level mismatch")
	assert.Equal(t, "software", driver.Config.StorageClass, "storage class mismatch")
	assert.Equal(t, "nfsvers=4.1", driver.Config.NFSMountOptions, "NFS mount options mismatch")
	assert.Equal(t, "30", driver.Config.VolumeCreateTimeout, "NFS mount options mismatch")
	assert.Equal(t, "true", driver.Config.SnapshotDir, "snapshot dir mismatch")
	assert.Equal(t, "1456898458", driver.Config.SnapshotReserve, "snapshot reserve mismatch")
	assert.Equal(t, "0700", driver.Config.UnixPermissions, "unix permissions mismatch")
	assert.Equal(t, "123456789000", driver.Config.LimitVolumeSize, "limit volume size mismatch")
	assert.Equal(t, "1.1.1.1/32", driver.Config.ExportRule, "export rule mismatch")
	assert.Equal(t, sa.NFS, driver.Config.NASType, "NAS type mismatch")
}

func TestPopulateConfigurationDefaults_InvalidSnapshotDir(t *testing.T) {
	config := &drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
		},
		GCNVNASStorageDriverPool: drivers.GCNVNASStorageDriverPool{
			GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
				SnapshotDir: "true$",
			},
		},
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config = *config

	result := driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.Error(t, result, " no error")
}

func TestInitializeStoragePools(t *testing.T) {
	supportedTopologies := []map[string]string{
		{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "us-east-1c"},
	}

	config := &drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     "myGCNVBackend",
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		NFSMountOptions: "nfsvers=4.1",
		GCNVNASStorageDriverPool: drivers.GCNVNASStorageDriverPool{
			GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				UnixPermissions: "0777",
				SnapshotDir:     "false",
				SnapshotReserve: "1456898458",
				ExportRule:      "1.1.1.1/32",
			},
			ServiceLevel: "Standard",
			Region:       "region1",
			Zone:         "zone1",
		},
		Storage: []drivers.GCNVNASStorageDriverPool{
			{
				GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
					CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
						Size: "123456789000",
					},
					UnixPermissions: "0700",
					ExportRule:      "2.2.2.2/32",
				},
				ServiceLevel:        gcnvapi.ServiceLevelExtreme,
				Region:              "region2",
				Zone:                "zone2",
				SupportedTopologies: supportedTopologies,
				NASType:             "nfs",
				StoragePools:        []string{"Pool1"},
				Network:             "test-network1",
			},
			{
				GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
					UnixPermissions: "0770",
					SnapshotDir:     "false",
					SnapshotReserve: "1456898458",
				},
				SupportedTopologies: supportedTopologies,
			},
		},
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config = *config

	// Pool 1
	pool0 := storage.NewStoragePool(nil, "myGCNVBackend_pool_0")
	pool0.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool0.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Encryption] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool0.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool0.Attributes()[sa.NASType] = sa.NewStringOffer("nfs")

	pool0.Attributes()[sa.Region] = sa.NewStringOffer("region2")
	pool0.Attributes()[sa.Zone] = sa.NewStringOffer("zone2")

	pool0.InternalAttributes()[Size] = "123456789000"
	pool0.InternalAttributes()[UnixPermissions] = "0700"
	pool0.InternalAttributes()[ServiceLevel] = gcnvapi.ServiceLevelExtreme
	pool0.InternalAttributes()[SnapshotDir] = "false"
	pool0.InternalAttributes()[SnapshotReserve] = "1456898458"
	pool0.InternalAttributes()[ExportRule] = "2.2.2.2/32"

	pool0.InternalAttributes()[Network] = "test-network1"
	pool0.InternalAttributes()[CapacityPools] = "Pool1"

	pool0.SetSupportedTopologies(supportedTopologies)

	// Pool 2
	pool1 := storage.NewStoragePool(nil, "myGCNVBackend_pool_1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool1.Attributes()[sa.NASType] = sa.NewStringOffer("nfs")

	pool1.Attributes()[sa.Region] = sa.NewStringOffer("region1")
	pool1.Attributes()[sa.Zone] = sa.NewStringOffer("zone1")

	pool1.InternalAttributes()[Size] = "1234567890"
	pool1.InternalAttributes()[UnixPermissions] = "0770"
	pool1.InternalAttributes()[ServiceLevel] = gcnvapi.ServiceLevelStandard
	pool1.InternalAttributes()[SnapshotDir] = "false"
	pool1.InternalAttributes()[SnapshotReserve] = "1456898458"
	pool1.InternalAttributes()[ExportRule] = "1.1.1.1/32"

	pool1.InternalAttributes()[Network] = ""
	pool1.InternalAttributes()[CapacityPools] = ""

	pool1.SetSupportedTopologies(supportedTopologies)

	expectedPools := map[string]storage.Pool{
		"myGCNVBackend_pool_0": pool0,
		"myGCNVBackend_pool_1": pool1,
	}

	result := driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	assert.NoError(t, result, " error occurred")
	assert.Equal(t, expectedPools, driver.pools, "pools do not match")
}

func TestInitializeStoragePools_InvalidSnapshotDir(t *testing.T) {
	config := &drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     "myGCNVBackend",
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		NFSMountOptions: "nfsvers=4.1",
		GCNVNASStorageDriverPool: drivers.GCNVNASStorageDriverPool{
			GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				UnixPermissions: "0777",
				SnapshotDir:     "true$",
				SnapshotReserve: "1456898458",
				ExportRule:      "1.1.1.1/32",
			},
			ServiceLevel: "Standard",
			Region:       "region1",
			Zone:         "zone1",
		},
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config = *config

	result := driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	assert.Error(t, result, "no error")
}

func TestInitializeStoragePools_VirtualPool_InvalidSnapshotDir(t *testing.T) {
	config := &drivers.GCNVNASStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     "myGCNVBackend",
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		NFSMountOptions: "nfsvers=4.1",
		GCNVNASStorageDriverPool: drivers.GCNVNASStorageDriverPool{
			GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				SnapshotReserve: "1456898458",
			},
			ServiceLevel: "Standard",
			Region:       "region1",
			Zone:         "zone1",
		},
		Storage: []drivers.GCNVNASStorageDriverPool{
			{
				GCNVNASStorageDriverConfigDefaults: drivers.GCNVNASStorageDriverConfigDefaults{
					CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
						Size: "123456789000",
					},
					UnixPermissions: "0700",
					ExportRule:      "2.2.2.2/32",
					SnapshotDir:     "true$",
				},
				ServiceLevel: gcnvapi.ServiceLevelExtreme,
				NASType:      "nfs",
				StoragePools: []string{"Pool1"},
				Network:      "test-network1",
			},
		},
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config = *config

	result := driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	assert.NoError(t, result, "error occurred")
}

func getStructsForCreateNFSVolume(ctx context.Context, driver *NASStorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *gcnvapi.CapacityPool, *gcnvapi.Volume, *gcnvapi.VolumeCreateRequest,
) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          true,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	capacityPool := &gcnvapi.CapacityPool{
		Name:            "CP1",
		FullName:        "CP-premium-pool",
		Location:        gcnvapi.Location,
		ServiceLevel:    gcnvapi.ServiceLevelPremium,
		State:           gcnvapi.StateReady,
		NetworkName:     gcnvapi.NetworkName,
		NetworkFullName: gcnvapi.NetworkFullName,
	}

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            nil,
		SnapshotReserve:   0,
		SnapshotDirectory: true,
		SecurityStyle:     "Unix",
	}

	createRequest := &gcnvapi.VolumeCreateRequest{
		Name:            "testvol1",
		CreationToken:   "trident-testvol1",
		CapacityPool:    "CP1",
		SizeBytes:       gcnvapi.VolumeSizeI64,
		ExportPolicy:    exportPolicy,
		ProtocolTypes:   []string{gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41},
		UnixPermissions: "0755",
		Labels: map[string]string{
			"backend_uuid":     gcnvapi.BackendUUID,
			"platform":         "",
			"platform_version": "",
			"plugin":           "google-cloud-netapp-volumes",
			"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
		},
		SnapshotReserve:   nil,
		SnapshotDirectory: true,
		SecurityStyle:     "Unix",
	}

	return volConfig, capacityPool, volume, createRequest
}

func getMultipleCapacityPoolsForCreateVolume() []*gcnvapi.CapacityPool {
	return []*gcnvapi.CapacityPool{
		{
			Name:            "CP1",
			FullName:        "CP-premium-pool",
			Location:        gcnvapi.Location,
			ServiceLevel:    gcnvapi.ServiceLevelPremium,
			State:           gcnvapi.StateReady,
			NetworkName:     gcnvapi.NetworkName,
			NetworkFullName: gcnvapi.NetworkFullName,
		},
		{
			Name:            "CP2",
			FullName:        "CP-premium-pool",
			Location:        gcnvapi.Location,
			ServiceLevel:    gcnvapi.ServiceLevelPremium,
			State:           gcnvapi.StateReady,
			NetworkName:     gcnvapi.NetworkName,
			NetworkFullName: gcnvapi.NetworkFullName,
		},
		{
			Name:            "CP3",
			FullName:        "CP-premium-pool",
			Location:        gcnvapi.Location,
			ServiceLevel:    gcnvapi.ServiceLevelPremium,
			State:           gcnvapi.StateReady,
			NetworkName:     gcnvapi.NetworkName,
			NetworkFullName: gcnvapi.NetworkFullName,
		},
	}
}

func getFlexServiceCapacityPoolForCreateVolume() []*gcnvapi.CapacityPool {
	return []*gcnvapi.CapacityPool{
		{
			Name:            "CP1",
			FullName:        "CP-flex-pool",
			Location:        gcnvapi.Location,
			ServiceLevel:    gcnvapi.ServiceLevelFlex,
			State:           gcnvapi.StateReady,
			NetworkName:     gcnvapi.NetworkName,
			NetworkFullName: gcnvapi.NetworkFullName,
			Zone:            "asia-east1-c",
		},
	}
}

func TestCreate_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.SizeBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, gcnvapi.ServiceLevelPremium, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_MultipleCapacityPools_FirstSucceeds(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	capacityPools := getMultipleCapacityPoolsForCreateVolume()
	createRequest.CapacityPool = capacityPools[0].Name

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return(capacityPools).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, capacityPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return(capacityPools).Times(1)

	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.SizeBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, gcnvapi.ServiceLevelPremium, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_MultipleCapacityPools_SecondSucceeds(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	capacityPools := getMultipleCapacityPoolsForCreateVolume()

	createRequest1 := *createRequest
	createRequest1.CapacityPool = capacityPools[0].Name

	createRequest2 := *createRequest
	createRequest2.CapacityPool = capacityPools[1].Name

	volume.CapacityPool = capacityPools[1].Name

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return(capacityPools).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, capacityPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return(capacityPools).Times(1)

	mockAPI.EXPECT().CreateVolume(ctx, &createRequest1).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest2).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.SizeBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, gcnvapi.ServiceLevelPremium, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolume_MultipleCapacityPools_NoneSucceeds(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	capacityPools := getMultipleCapacityPoolsForCreateVolume()

	createRequest1 := *createRequest
	createRequest1.CapacityPool = capacityPools[0].Name

	createRequest2 := *createRequest
	createRequest2.CapacityPool = capacityPools[1].Name

	createRequest3 := *createRequest
	createRequest3.CapacityPool = capacityPools[2].Name

	volume.CapacityPool = capacityPools[1].Name

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return(capacityPools).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, capacityPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return(capacityPools).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest1).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest2).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, &createRequest3).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestGCNVCreate_InvalidVolumeName(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Name = "111111"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidCreationToken(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.InternalName = "111111"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NoStoragePool(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, nil, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NonexistentStoragePool(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]
	driver.pools = nil

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_VolumeExistsCreating(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volume.State = gcnvapi.VolumeStateCreating

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, errors.VolumeCreatingError(""), result, "not VolumeCreatingError")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volume.State = gcnvapi.VolumeStateReady

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, drivers.NewVolumeExistsError(""), result, "not VolumeExistsError")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_InvalidSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "invalid"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NegativeSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "-200Gi"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_ZeroSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "0"
	createRequest.UnixPermissions = "0777"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.SizeBytes, DefaultVolumeSize, "request size mismatch")
	assert.Equal(t, volConfig.Size, defaultVolumeSizeStr, "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_ServiceLevelFlex(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelFlex
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "0"
	createRequest.UnixPermissions = "0777"

	createRequest.SizeBytes = int64(1073741824)
	volume.SizeBytes = int64(1073741824)

	capacityPool.ServiceLevel = gcnvapi.ServiceLevelFlex
	volume.ServiceLevel = gcnvapi.ServiceLevelFlex

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelFlex).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.SizeBytes, int64(1073741824), "request size mismatch")
	assert.Equal(t, volConfig.Size, "1073741824", "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_BelowAbsoluteMinimumSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "1k"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)
	fmt.Println("------", result)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_AboveMaximumSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.LimitVolumeSize = "100Gi"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = "101Gi"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestGCNVCreate_InvalidSnapshotDir(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.SnapshotDir = "invalid"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)
	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestGCNVCreate_SnapshotReserve(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.SnapshotReserve = "invalid"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)
	assert.Error(t, result, "create failed")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestGCNVCreate_InvalidSnapshotReserve(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.SnapshotReserve = "50"

	volume.SizeBytes = int64(214748364800)
	volume.SnapshotReserve = int64(50)

	createRequest.SizeBytes = int64(214748364800)
	createRequest.SnapshotReserve = &volume.SnapshotReserve

	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, gcnvapi.ServiceLevelPremium, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_MountOptions(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.MountOptions = "nfsvers=3"
	createRequest.UnixPermissions = "0777"
	createRequest.ProtocolTypes = []string{gcnvapi.ProtocolTypeNFSv3}
	createRequest.ExportPolicy.Rules[0].Nfsv4 = false
	createRequest.SnapshotDirectory = false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.SizeBytes, DefaultVolumeSize, "request size mismatch")
	assert.Equal(t, volConfig.Size, defaultVolumeSizeStr, "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_MountOptions_NFSv4(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	volConfig.MountOptions = "nfsvers=4"
	volume.ProtocolTypes = []string{gcnvapi.ProtocolTypeNFSv41}
	createRequest.ProtocolTypes = []string{gcnvapi.ProtocolTypeNFSv41}
	createRequest.UnixPermissions = "0777"
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].Nfsv4 = true

	volume.ExportPolicy.Rules[0].Nfsv3 = false
	volume.ExportPolicy.Rules[0].Nfsv4 = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.SizeBytes, DefaultVolumeSize, "request size mismatch")
	assert.Equal(t, volConfig.Size, defaultVolumeSizeStr, "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_MountOptions_BothEnabled(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	createRequest.UnixPermissions = "0777"
	createRequest.ProtocolTypes = []string{gcnvapi.ProtocolTypeNFSv3, gcnvapi.ProtocolTypeNFSv41}
	createRequest.ExportPolicy.Rules[0].Nfsv4 = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.SizeBytes, DefaultVolumeSize, "request size mismatch")
	assert.Equal(t, volConfig.Size, defaultVolumeSizeStr, "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_InvalidMountOptions(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.MountOptions = "nfsvers=5"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_VolConfigMountOptionsNFSv3(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.MountOptions = "nfsvers=3"
	volConfig.SnapshotDir = "false"

	createRequest.UnixPermissions = "0777"
	createRequest.ProtocolTypes = []string{gcnvapi.ProtocolTypeNFSv3}
	createRequest.ExportPolicy.Rules[0].Nfsv4 = false
	createRequest.SnapshotDirectory = false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolume_VolConfigMountOptions(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.MountOptions = "nfsvers=4.1"

	createRequest.UnixPermissions = "0777"
	createRequest.ProtocolTypes = []string{gcnvapi.ProtocolTypeNFSv41}
	createRequest.ExportPolicy.Rules[0].Nfsv3 = false
	createRequest.ExportPolicy.Rules[0].Nfsv4 = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolume_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, _, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_BelowGCNVMinimumSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.Size = strconv.FormatUint(MinimumGCNVVolumeSizeBytesHW-1, 10)

	createRequest.UnixPermissions = "0777"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, uint64(createRequest.SizeBytes), MinimumGCNVVolumeSizeBytesHW, "request size mismatch")
	assert.Equal(t, volConfig.Size, strconv.FormatUint(MinimumGCNVVolumeSizeBytesHW, 10), "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolumeWithPoolLabels(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	driver.Config.Labels = map[string]string{
		"backend_uuid":     gcnvapi.BackendUUID,
		"platform":         "",
		"platform_version": "",
		"plugin":           "google-cloud-netapp-volumes",
		"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
	}

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.SizeBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, gcnvapi.ServiceLevelPremium, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_NFSVolumeWithPoolLabels_NoMatchingCapacityPool(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.Config.Labels = map[string]string{
		"label1":  "value1",
		"label2":  "value2",
		"label3":  "value3",
		"label4":  "value4",
		"label5":  "value5",
		"label6":  "value6",
		"label7":  "value7",
		"label8":  "value8",
		"label9":  "value9",
		"label10": "value10",
		"label11": "value11",
		"label12": "value12",
		"label13": "value13",
		"label14": "value14",
		"label15": "value15",
		"label16": "value16",
		"label17": "value17",
		"label18": "value18",
		"label19": "value19",
		"label20": "value20",
		"label21": "value21",
		"label22": "value22",
		"label23": "value23",
		"label24": "value24",
		"label25": "value25",
		"label26": "value26",
		"label27": "value27",
		"label28": "value28",
		"label29": "value29",
		"label30": "value30",
		"label31": "value31",
		"label32": "value32",
		"label33": "value33",
		"label34": "value34",
		"label35": "value35",
		"label36": "value36",
		"label37": "value37",
		"label38": "value38",
		"label39": "value39",
		"label40": "value40",
		"label41": "value41",
		"label42": "value42",
		"label43": "value43",
		"label44": "value44",
		"label45": "value45",
		"label46": "value46",
		"label47": "value47",
		"label48": "value48",
		"label49": "value49",
		"label50": "value50",
		"label51": "value51",
		"label52": "value52",
		"label53": "value53",
		"label54": "value54",
		"label55": "value55",
		"label56": "value56",
		"label57": "value57",
		"label58": "value58",
		"label59": "value59",
		"label60": "value60",
	}

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, _, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{}).Times(1)

	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{}).Times(1)
	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_NFSVolume_ZoneSelectionSucceeds(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelFlex
	driver.Config.NASType = "nfs"
	driver.Config.SupportedTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-c"},
	}

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.RequisiteTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-a"},
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-c"},
	}
	volConfig.PreferredTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-c"},
	}
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	capacityPools := getFlexServiceCapacityPoolForCreateVolume()
	createRequest.CapacityPool = capacityPools[0].Name

	volConfig.Size = "0"

	createRequest.SizeBytes = int64(1073741824)
	volume.SizeBytes = int64(1073741824)

	volume.ServiceLevel = gcnvapi.ServiceLevelFlex

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelFlex).Return(capacityPools).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, capacityPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return(capacityPools).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.SizeBytes, int64(1073741824), "request size mismatch")
	assert.Equal(t, volConfig.Size, "1073741824", "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolume_ZoneSelectionFails(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelFlex
	driver.Config.NASType = "nfs"
	driver.Config.SupportedTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-c"},
	}

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, _, volume, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	volConfig.RequisiteTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-a"},
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-b"},
	}
	volConfig.PreferredTopologies = []map[string]string{
		{"topology.kubernetes.io/region": "asia-east1", "topology.kubernetes.io/zone": "asia-east1-a"},
	}
	createRequest.UnixPermissions = "0777"
	volume.UnixPermissions = "0777"

	capacityPools := getFlexServiceCapacityPoolForCreateVolume()
	createRequest.CapacityPool = capacityPools[0].Name

	volConfig.Size = "0"

	createRequest.SizeBytes = int64(1073741824)
	volume.SizeBytes = int64(1073741824)

	volume.ServiceLevel = gcnvapi.ServiceLevelFlex

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelFlex).Return(capacityPools).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, capacityPools, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{}).Times(1)
	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func getStructsForCreateSMBVolume(ctx context.Context, driver *NASStorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *gcnvapi.CapacityPool, *gcnvapi.Volume, *gcnvapi.VolumeCreateRequest,
) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	capacityPool := &gcnvapi.CapacityPool{
		Name:            "CP1",
		FullName:        "CP-premium-pool",
		Location:        gcnvapi.Location,
		ServiceLevel:    gcnvapi.ServiceLevelPremium,
		State:           gcnvapi.StateReady,
		NetworkName:     gcnvapi.NetworkName,
		NetworkFullName: gcnvapi.NetworkFullName,
	}

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeSMB},
		MountTargets:      nil,
		Labels:            nil,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
	}

	createRequest := &gcnvapi.VolumeCreateRequest{
		Name:          "testvol1",
		CreationToken: "trident-testvol1",
		CapacityPool:  "CP1",
		SizeBytes:     gcnvapi.VolumeSizeI64,
		ProtocolTypes: []string{gcnvapi.ProtocolTypeSMB},
		Labels: map[string]string{
			"backend_uuid":     gcnvapi.BackendUUID,
			"platform":         "",
			"platform_version": "",
			"plugin":           "google-cloud-netapp-volumes",
			"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
		},
		SnapshotReserve:   nil,
		SnapshotDirectory: true,
	}

	return volConfig, capacityPool, volume, createRequest
}

func TestCreate_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateSMBVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
	assert.Equal(t, strconv.FormatInt(createRequest.SizeBytes, 10), volConfig.Size, "request size mismatch")
	assert.Equal(t, gcnvapi.ServiceLevelPremium, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "0777", volConfig.UnixPermissions)
}

func TestCreate_SMBVolume_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, _, createRequest := getStructsForCreateSMBVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_SMBVolume_BelowGCNVMinimumSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, createRequest := getStructsForCreateSMBVolume(ctx, driver, storagePool)
	volConfig.Size = strconv.FormatUint(MinimumGCNVVolumeSizeBytesHW-1, 10)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)

	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, uint64(createRequest.SizeBytes), MinimumGCNVVolumeSizeBytesHW, "request size mismatch")
	assert.Equal(t, volConfig.Size, strconv.FormatUint(MinimumGCNVVolumeSizeBytesHW, 10), "config size mismatch")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreate_NFSVolumeOnSMBPool_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, _ := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	_, _, _, createRequest := getStructsForCreateSMBVolume(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NotEqual(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type matches")
	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestCreate_SMBVolumeOnNFSPool_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	volConfig, capacityPool, volume, _ := getStructsForCreateSMBVolume(ctx, driver, storagePool)
	_, _, _, createRequest := getStructsForCreateNFSVolume(ctx, driver, storagePool)
	createRequest.UnixPermissions = "0777"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, storagePool,
		gcnvapi.ServiceLevelPremium).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)

	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, []*gcnvapi.CapacityPool{capacityPool}, volConfig.RequisiteTopologies, volConfig.PreferredTopologies).Return([]*gcnvapi.CapacityPool{capacityPool}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NotEqual(t, createRequest.ProtocolTypes, volume.ProtocolTypes, "protocol type matches")
	assert.Error(t, result, "create did not fail")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func getStructsForCreateClone(ctx context.Context, driver *NASStorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *storage.VolumeConfig, *gcnvapi.VolumeCreateRequest, *gcnvapi.Volume, *gcnvapi.Volume, *gcnvapi.Snapshot,
) {
	snapshotReserve := int64(0)

	sourceVolConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	cloneVolConfig := &storage.VolumeConfig{
		Version:                   "1",
		Name:                      "testvol2",
		InternalName:              "trident-testvol2",
		CloneSourceVolume:         "testvol1",
		CloneSourceVolumeInternal: "trident-testvol1",
	}

	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	createRequest := &gcnvapi.VolumeCreateRequest{
		Name:            cloneVolConfig.Name,
		CreationToken:   cloneVolConfig.InternalName,
		CapacityPool:    "CP1",
		SizeBytes:       gcnvapi.VolumeSizeI64,
		ExportPolicy:    exportPolicy,
		ProtocolTypes:   []string{gcnvapi.ProtocolTypeNFSv3},
		UnixPermissions: "0755",
		Labels: map[string]string{
			"backend_uuid":     gcnvapi.BackendUUID,
			"platform":         "",
			"platform_version": "",
			"plugin":           "google-cloud-netapp-volumes",
			"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
		},
		SnapshotReserve:   &snapshotReserve,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	sourceVolume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            nil,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	cloneVolume := &gcnvapi.Volume{
		Name:              "testvol2",
		CreationToken:     "trident-testvol2",
		FullName:          gcnvapi.FullVolumeName + "testvol2",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            nil,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	snapshot := &gcnvapi.Snapshot{
		Name:     "snap1",
		Volume:   "testvol1",
		Location: gcnvapi.Location,
		State:    gcnvapi.StateReady,
		Created:  time.Now(),
		Labels: map[string]string{
			"backend_uuid":     gcnvapi.BackendUUID,
			"platform":         "",
			"platform_version": "",
			"plugin":           "google-cloud-netapp-volumes",
			"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
		},
	}

	return sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume, snapshot
}

func TestCreateClone_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]
	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), gomock.Any()).Return(snapshot, nil).Times(1)

	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
	assert.Equal(t, cloneVolume.FullName, cloneVolConfig.InternalID, "internal ID not set on volConfig")
}

func TestCreateClone_Snapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolume.SnapshotDirectory = false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "snap1").Return(snapshot, nil).Times(1)

	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Nil(t, result, "create failed")
	assert.NoError(t, result, "error occurred")
}

func TestCreateClone_CreateSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	cloneVolConfig.CloneSourceSnapshotInternal = ""
	sourceVolConfig.CloneSourceSnapshotInternal = "snap1"

	sourceVolume.SnapshotDirectory = false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	// snapshot name is a timestamp, therefore it cannot be known beforehand, using Any
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), gomock.Any()).Return(snapshot, nil).Times(1)

	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	assert.NotEqual(t, sourceVolConfig.CloneSourceSnapshotInternal, cloneVolConfig.CloneSourceSnapshotInternal)

	err = driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, err)
	assert.Equal(t, sourceVolConfig.CloneSourceSnapshotInternal, cloneVolConfig.CloneSourceSnapshotInternal)
}

func TestCreateClone_ROClone(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolume.SnapshotDirectory = true
	cloneVolConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "snap1").Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Nil(t, result, "create failed")
	assert.NoError(t, result, "error occurred")
}

func TestCreateClone_ROCloneFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolume.SnapshotDirectory = false
	cloneVolConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "snap1").Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NotNil(t, result, "create succeeded")
	assert.Error(t, result, "expected error")
}

func TestCreateClone_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_InvalidVolumeName(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.Name = "1testvol"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_InvalidCreationToken(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.InternalName = "1testvol"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_NonexistentSourceVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_VolumeExistsCreating(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, cloneVolume, _ := getStructsForCreateClone(ctx, driver, storagePool)

	cloneVolume.State = gcnvapi.VolumeStateCreating

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(true, cloneVolume, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.IsType(t, errors.VolumeCreatingError(""), result, "not VolumeExistsError")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, cloneVolume, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(true, cloneVolume, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.IsType(t, drivers.NewVolumeExistsError(""), result, "not VolumeExistsError")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotNotFound(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, _ := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "snap1").Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotNotReady(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, snapshot := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	snapshot.State = gcnvapi.SnapshotStateError

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "snap1").Return(snapshot, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotCreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), gomock.Any()).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_SnapshotRefetchFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, _, sourceVolume, _, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), gomock.Any()).Return(snapshot, nil).Times(1)

	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, _, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)

	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), gomock.Any()).Return(snapshot, nil).Times(1)

	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", cloneVolConfig.InternalID, "internal ID set on volConfig")
}

func TestCreateClone_AboveMaxLabelCount(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.Labels = map[string]string{
		"label1":  "value1",
		"label2":  "value2",
		"label3":  "value3",
		"label4":  "value4",
		"label5":  "value5",
		"label6":  "value6",
		"label7":  "value7",
		"label8":  "value8",
		"label9":  "value9",
		"label10": "value10",
		"label11": "value11",
		"label12": "value12",
		"label13": "value13",
		"label14": "value14",
		"label15": "value15",
		"label16": "value16",
		"label17": "value17",
		"label18": "value18",
		"label19": "value19",
		"label20": "value20",
		"label21": "value21",
		"label22": "value22",
		"label23": "value23",
		"label24": "value24",
		"label25": "value25",
		"label26": "value26",
		"label27": "value27",
		"label28": "value28",
		"label29": "value29",
		"label30": "value30",
		"label31": "value31",
		"label32": "value32",
		"label33": "value33",
		"label34": "value34",
		"label35": "value35",
		"label36": "value36",
		"label37": "value37",
		"label38": "value38",
		"label39": "value39",
		"label40": "value40",
		"label41": "value41",
		"label42": "value42",
		"label43": "value43",
		"label44": "value44",
		"label45": "value45",
		"label46": "value46",
		"label47": "value47",
		"label48": "value48",
		"label49": "value49",
		"label50": "value50",
		"label51": "value51",
		"label52": "value52",
		"label53": "value53",
		"label54": "value54",
		"label55": "value55",
		"label56": "value56",
		"label57": "value57",
		"label58": "value58",
		"label59": "value59",
		"label60": "value60",
	}

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	storagePool := driver.pools["gcnv_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume, snapshot := getStructsForCreateClone(ctx, driver, storagePool)

	createRequest.Labels = driver.Config.Labels
	sourceVolume.Labels = driver.Config.Labels

	cloneVolConfig.CloneSourceSnapshotInternal = "snap1"
	sourceVolume.SnapshotDirectory = false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "snap1").Return(snapshot, nil).Times(1)

	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Nil(t, result, "create failed")
	assert.NoError(t, result, "error occurred")
}

func getStructsForImport(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	OriginalVolume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	return volConfig, OriginalVolume
}

func getStructsForNFSImport(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	OriginalVolume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	return volConfig, OriginalVolume
}

func getStructsForSMBImport(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	OriginalVolume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeSMB},
		MountTargets:      nil,
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
	}

	return volConfig, OriginalVolume
}

func TestImport_Managed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)
	var snapshotDirAccess bool
	originalName := "importMe"
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess, nil).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, originalVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalVolume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedVolumeFullName(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)
	var snapshotDirAccess bool
	originalFullName := "projects/123456789/locations/fake-location/volumes/testvol1"
	expectedUnixPermissions := "0770"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalFullName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess, nil).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, originalVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalFullName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalVolume.Name, volConfig.Name, "internal name mismatch")
	assert.Equal(t, originalFullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedWithSnapshotDir(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	volConfig.SnapshotDir = "true"
	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)

	originalName := "importMe"
	expectedUnixPermissions := "0770"
	snapshotDirAccess := true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess, nil).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, originalVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalVolume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedWithSnapshotDirFalse(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	volConfig.SnapshotDir = "false"
	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)

	originalName := "importMe"
	expectedUnixPermissions := "0770"
	snapshotDirAccess := false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess, nil).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, originalVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalVolume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_ManagedWithInvalidSnapshotDirValue(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	volConfig.SnapshotDir = "xxxffa"
	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_SMB_Managed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForSMBImport(ctx, driver)

	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)
	var snapshotDirAccess bool
	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, nil, &snapshotDirAccess, nil).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, originalVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalVolume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_SMB_Failed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForSMBImport(ctx, driver)

	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)
	var snapshotDirAccess bool
	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, nil, &snapshotDirAccess, nil).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_DualProtocolVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForSMBImport(ctx, driver)

	// Dual-protocol volume has ProtocolTypes as [NFSv3, CIFS]
	originalVolume.ProtocolTypes = append(originalVolume.ProtocolTypes, "NFSv3")

	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_CapacityPoolError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "import succeeded")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_NotManaged(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	volConfig.ImportNotManaged = true
	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, originalVolume.FullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_NotManagedVolumeFullName(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	volConfig.ImportNotManaged = true
	originalFullName := "projects/123456789/locations/fake-location/volumes/testvol1"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalFullName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalFullName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalVolume.Name, volConfig.Name, "internal name mismatch")
	assert.Equal(t, originalFullName, volConfig.InternalID, "internal ID not set on volConfig")
}

func TestImport_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForNFSImport(ctx, driver)
	volConfig.ImportNotManaged = true

	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_NotFound(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForNFSImport(ctx, driver)

	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(nil, errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_DuplicateVolumes(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForNFSImport(ctx, driver)

	originalName := "importMe"

	// Mock API to return an error indicating duplicate volumes
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(nil, errors.New("found multiple volumes with the same name in the given location")).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Contains(t, result.Error(), "found multiple volumes with the same name in the given location", "unexpected error message")
}

func TestImport_InvalidUnixPermissions(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "8888"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_ModifyVolumeFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)
	originalName := "importMe"
	expectedUnixPermissions := "0770"
	snapshotDirAccess := false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess, nil).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	expectedLabels := driver.updateTelemetryLabels(ctx, originalVolume)
	originalName := "importMe"
	expectedUnixPermissions := "0770"
	snapshotDirAccess := false

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	mockAPI.EXPECT().ModifyVolume(ctx, originalVolume, expectedLabels, &expectedUnixPermissions, &snapshotDirAccess, nil).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, originalVolume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return("", errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestImport_BackendVolumeMismatch(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs3"
	driver.Config.UnixPermissions = "0770"

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, originalVolume := getStructsForNFSImport(ctx, driver)

	originalName := "importMe"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, originalName).Return(originalVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, originalVolume).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "trident-testvol1", volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "", volConfig.InternalID, "internal ID set on volConfig")
}

func TestGCNVRename(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	result := driver.Rename(ctx, "oldName", "newName")

	assert.Nil(t, result, "not nil")
}

func TestGetTelemetryLabels(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	result := driver.getTelemetryLabels(ctx)

	assert.NotNil(t, result, "received nil")
	assert.Equal(t, result["backend_uuid"], gcnvapi.BackendUUID, "backend UUID mismatch")
}

func TestUpdateTelemetryLabels(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	result := driver.updateTelemetryLabels(ctx, &gcnvapi.Volume{})

	assert.NotNil(t, result)
	assert.Equal(t, result["backend_uuid"], gcnvapi.BackendUUID, "backend UUID mismatch")
}

func TestWaitForVolumeCreate_Ready(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "trident-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.StateReady,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateReady, nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, volume)

	assert.Nil(t, result, "incorrect state")
}

func TestWaitForVolumeCreate_Creating(t *testing.T) {
	for _, state := range []string{gcnvapi.VolumeStateUnspecified, gcnvapi.VolumeStateCreating} {

		mockAPI, driver := newMockGCNVDriver(t)

		volume := &gcnvapi.Volume{
			Name:          "testvol1",
			CreationToken: "trident-testvol1",
			FullName:      gcnvapi.FullVolumeName + "testvol1",
			State:         gcnvapi.VolumeStateCreating,
		}

		mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
			driver.volumeCreateTimeout).Return(state, errFailed).Times(1)

		result := driver.waitForVolumeCreate(ctx, volume)

		assert.Error(t, result, "expected error")
		assert.IsType(t, errors.VolumeCreatingError(""), result, "not VolumeCreatingError")
	}
}

func TestWaitForVolumeCreate_DeletingSucceeded(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "trident-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.VolumeStateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateDeleting, errFailed).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleting, nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, volume)

	assert.Nil(t, result, "creation failed")
}

func TestWaitForVolumeCreate_DeletingFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "trident-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.VolumeStateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateDeleting, errFailed).Times(1)

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleting, errFailed).Times(1)

	result := driver.waitForVolumeCreate(ctx, volume)

	assert.Error(t, result, "expected error")
}

func TestWaitForVolumeCreate_ErrorDeleteSucceeded(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "trident-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.VolumeStateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateError, errFailed).Times(1)

	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)

	result := driver.waitForVolumeCreate(ctx, volume)

	assert.Nil(t, result, "creation failed")
}

func TestWaitForVolumeCreate_ErrorDeleteFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "trident-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.VolumeStateCreating,
	}

	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateError, errFailed).Times(1)

	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(errFailed).Times(1)

	result := driver.waitForVolumeCreate(ctx, volume)

	assert.Error(t, result, "expected error")
}

func TestWaitForVolumeCreate_OtherStates(t *testing.T) {
	for _, state := range []string{gcnvapi.VolumeStateUpdating, gcnvapi.VolumeStateRestoring, gcnvapi.VolumeStateDisabled} {

		mockAPI, driver := newMockGCNVDriver(t)

		volume := &gcnvapi.Volume{
			Name:          "testvol1",
			CreationToken: "trident-testvol1",
			FullName:      gcnvapi.FullVolumeName + "testvol1",
			State:         gcnvapi.VolumeStateCreating,
		}

		mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{gcnvapi.VolumeStateError},
			driver.volumeCreateTimeout).Return(state, errFailed).Times(1)

		result := driver.waitForVolumeCreate(ctx, volume)

		assert.Nil(t, result, "creation failed")
	}
}

func getStructsForDestroyNFSVolume(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	return volConfig, volume
}

func getStructsForDestroyDeleteSnapshot(ctx context.Context, driver *NASStorageDriver) (
	volumeConfig *storage.VolumeConfig, volume *gcnvapi.Volume, cloneVolumeConfig *storage.VolumeConfig,
	cloneVolume *gcnvapi.Volume, snapshot *gcnvapi.Snapshot,
) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	labels := driver.getTelemetryLabels(ctx)

	volumeConfig = &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	cloneVolumeConfig = &storage.VolumeConfig{
		Version:                     "1",
		Name:                        "clonetestvol",
		InternalName:                "trident-clonetestvol",
		Size:                        gcnvapi.VolumeSizeStr,
		CloneSourceSnapshotInternal: "snap",
		CloneSourceVolume:           volumeConfig.Name,
		CloneSourceVolumeInternal:   volumeConfig.InternalName,
	}

	volume = &gcnvapi.Volume{
		Name:              volumeConfig.Name,
		CreationToken:     volumeConfig.InternalName,
		FullName:          gcnvapi.FullVolumeName + volumeConfig.Name,
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	cloneVolume = &gcnvapi.Volume{
		Name:              cloneVolumeConfig.Name,
		CreationToken:     cloneVolumeConfig.InternalName,
		FullName:          gcnvapi.FullVolumeName + cloneVolumeConfig.Name,
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      nil,
		UnixPermissions:   "0755",
		Labels:            labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	snapshot = &gcnvapi.Snapshot{
		Name:     "snap1",
		Volume:   volume.Name,
		Location: gcnvapi.Location,
		State:    gcnvapi.StateReady,
		Created:  time.Now(),
		Labels: map[string]string{
			"backend_uuid":     gcnvapi.BackendUUID,
			"platform":         "",
			"platform_version": "",
			"plugin":           "google-cloud-netapp-volumes",
			"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
		},
	}

	return
}

func getStructsForDestroySMBVolume(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeSMB},
		MountTargets:      nil,
		Labels:            nil,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
	}

	return volConfig, volume
}

func TestDestroy_NFSVolume_Docker(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextDocker

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_NFSVolume_CSI(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_DeleteSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	sourceVolumeConfig, sourceVolume, cloneVolConfig, cloneVolume, snapshot := getStructsForDestroyDeleteSnapshot(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(true, cloneVolume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, cloneVolume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, cloneVolume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleted, nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, sourceVolumeConfig.InternalName).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, cloneVolConfig.CloneSourceSnapshotInternal).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, snapshot, gomock.Any()).Return(nil).Times(1)

	err := driver.Destroy(ctx, cloneVolConfig)

	assert.Nil(t, err)
}

func TestDestroy_DeleteSnapshot_VolDeleteError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	_, _, cloneVolConfig, _, _ := getStructsForDestroyDeleteSnapshot(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig).Return(false, nil, errors.NotFoundError("volume not found")).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, gomock.Any()).Return(nil).Times(0)

	err := driver.Destroy(ctx, cloneVolConfig)

	assert.NotNil(t, err)
}

func Test_AutomaticDeleteSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	sourceVolumeConfig, sourceVolume, cloneVolConfig, _, snapshot := getStructsForDestroyDeleteSnapshot(ctx, driver)

	mockAPI.EXPECT().VolumeByName(ctx, sourceVolumeConfig.InternalName).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, cloneVolConfig.CloneSourceSnapshotInternal).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, snapshot, gomock.Any()).Return(nil).Times(1)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func Test_AutomaticDeleteSnapshot_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	_, _, cloneVolConfig, _, _ := getStructsForDestroyDeleteSnapshot(ctx, driver)

	cloneVolConfig.CloneSourceSnapshotInternal = ""

	mockAPI.EXPECT().VolumeByName(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), cloneVolConfig.CloneSourceSnapshotInternal).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func Test_AutomaticDeleteSnapshot_SelectedSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	_, _, cloneVolConfig, _, _ := getStructsForDestroyDeleteSnapshot(ctx, driver)

	cloneVolConfig.CloneSourceSnapshot = cloneVolConfig.CloneSourceSnapshotInternal

	mockAPI.EXPECT().VolumeByName(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), cloneVolConfig.CloneSourceSnapshotInternal).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func Test_AutomaticDeleteSnapshot_VolDeleteError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	_, _, cloneVolConfig, _, _ := getStructsForDestroyDeleteSnapshot(ctx, driver)

	mockAPI.EXPECT().VolumeByName(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), cloneVolConfig.CloneSourceSnapshotInternal).Times(0)
	mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	driver.deleteAutomaticSnapshot(ctx, errors.New("volume delete error"), cloneVolConfig)
}

func Test_AutomaticDeleteSnapshot_VolByNameError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	sourceVolumeConfig, _, cloneVolConfig, _, _ := getStructsForDestroyDeleteSnapshot(ctx, driver)

	tests := []struct {
		name              string
		volumeByNameError error
	}{
		{
			name:              "Fails when volume not found for name",
			volumeByNameError: errors.NotFoundError("not found"),
		},
		{
			name:              "Fails when volume for name returns error other than not found",
			volumeByNameError: errors.New("other error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().VolumeByName(ctx, sourceVolumeConfig.InternalName).Return(nil, test.volumeByNameError).Times(1)
			mockAPI.EXPECT().SnapshotForVolume(ctx, gomock.Any(), cloneVolConfig.CloneSourceSnapshotInternal).Times(0)
			mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
		})
	}
}

func Test_AutomaticDeleteSnapshot_SnapshotForVolumeError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	sourceVolumeConfig, sourceVolume, cloneVolConfig, _, _ := getStructsForDestroyDeleteSnapshot(ctx, driver)

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
			mockAPI.EXPECT().VolumeByName(ctx, sourceVolumeConfig.InternalName).Return(sourceVolume, nil).Times(1)
			mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, cloneVolConfig.CloneSourceSnapshotInternal).Return(nil, test.snapshotForVolumeError).Times(1)
			mockAPI.EXPECT().DeleteSnapshot(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
		})
	}
}

func Test_AutomaticDeleteSnapshot_DeleteSnapshotError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	sourceVolumeConfig, sourceVolume, cloneVolConfig, _, snapshot := getStructsForDestroyDeleteSnapshot(ctx, driver)

	mockAPI.EXPECT().VolumeByName(ctx, sourceVolumeConfig.InternalName).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, cloneVolConfig.CloneSourceSnapshotInternal).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, snapshot, gomock.Any()).Return(errors.New("delete failed")).Times(1)

	driver.deleteAutomaticSnapshot(ctx, nil, cloneVolConfig)
}

func TestDestroy_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_AlreadyDeleted(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_StillDeletingDeleted_Docker(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextDocker

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	volume.State = gcnvapi.VolumeStateDeleting

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDestroy_StillDeleting_Docker(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextDocker

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	volume.State = gcnvapi.VolumeStateDeleting

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateDeleting, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_StillDeleting_CSI(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	volume.State = gcnvapi.VolumeStateDeleting

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.volumeCreateTimeout).Return(gcnvapi.VolumeStateDeleting, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextCSI

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_VolumeWaitFailed_Docker(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextDocker

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleting, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
}

func TestDestroy_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.DriverContext = tridentconfig.ContextDocker

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroySMBVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateDeleted, []string{gcnvapi.VolumeStateError},
		driver.defaultTimeout()).Return(gcnvapi.VolumeStateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "not nil")
}

func getStructsForPublishNFSVolume(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume, *models.VolumePublishInfo) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	mountTargets := []gcnvapi.MountTarget{
		{
			Export:     "trident-testvol1",
			ExportPath: "1.1.1.1:/trident-testvol1",
			Protocol:   "NFSv3",
		},
	}

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      mountTargets,
		UnixPermissions:   "0755",
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	publishInfo := &models.VolumePublishInfo{}

	return volConfig, volume, publishInfo
}

func getStructsForPublishSMBVolume(ctx context.Context, driver *NASStorageDriver) (*storage.VolumeConfig, *gcnvapi.Volume, *models.VolumePublishInfo) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	mountTargets := []gcnvapi.MountTarget{
		{
			Export:     "trident-testvol1",
			ExportPath: "\\\\tri-abcd.trident.com\\trident-testvol1",
			Protocol:   "SMB",
		},
	}

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeSMB},
		MountTargets:      mountTargets,
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
	}

	publishInfo := &models.VolumePublishInfo{}

	return volConfig, volume, publishInfo
}

func TestPublish_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.MountOptions = "nfsvers=3"
	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)
	assert.Nil(t, result, "not nil")
	assert.Equal(t, "/trident-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "1.1.1.1", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=3", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_ROClone_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.MountOptions = "nfsvers=3"
	publishInfo.NfsPath = "/trident-testvol1/.snapshot/" + gcnvapi.SnapshotUUID
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, volConfig.CloneSourceVolumeInternal).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")

	// TODO : Verify RO clone implementation.

	// assert.Equal(t, "/trident-testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", publishInfo.NfsPath,
	//	"NFS path mismatch")
	assert.Equal(t, "1.1.1.1", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=3", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishSMBVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, "\\trident-testvol1", publishInfo.SMBPath, "NFS path mismatch")
	assert.Equal(t, "tri-abcd.trident.com", publishInfo.SMBServer, "NFS server IP mismatch")
	assert.Equal(t, "smb", publishInfo.FilesystemType, "filesystem type mismatch")
}

func TestPublish_MountOptions(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath
	volConfig.MountOptions = "nfsvers=4.1"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, "/trident-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "1.1.1.1", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "nfsvers=4.1", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_MountOptions_Error(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	publishInfo.NfsPath = volConfig.AccessInfo.NfsPath
	volConfig.MountOptions = "nfsvers=4.4"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_MountOptions_ParseNFSExportPathError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volume.MountTargets[0].ExportPath = "1234:testvol1"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_MountOptions_ParseSMBExportPathError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volume.MountTargets[0].ExportPath = "1234:testvol1"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", publishInfo.SMBPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.SMBServer, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
}

func TestPublish_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _, publishInfo := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_ROClone_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	publishInfo.NfsPath = "/trident-testvol1/.snapshot/" + gcnvapi.SnapshotUUID
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, volConfig.CloneSourceVolumeInternal).Return(nil, errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "/trident-testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", publishInfo.NfsPath, "NFS path mismatch")
	assert.NotEqual(t, "", publishInfo.NfsPath, "NFS path is empty")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_NoMountTargets(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, publishInfo := getStructsForPublishNFSVolume(ctx, driver)
	volume.MountTargets = nil

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", publishInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "", publishInfo.MountOptions, "mount options mismatch")
}

func getStructsForCreateSnapshot(ctx context.Context, driver *NASStorageDriver, snapTime time.Time) (
	*storage.VolumeConfig, *gcnvapi.Volume, *storage.SnapshotConfig, *gcnvapi.Snapshot,
) {
	exportPolicy := &gcnvapi.ExportPolicy{
		Rules: []gcnvapi.ExportRule{
			{
				AllowedClients: "0.0.0.0/0",
				SMB:            false,
				Nfsv3:          true,
				Nfsv4:          false,
				RuleIndex:      1,
				AccessType:     gcnvapi.AccessTypeReadWrite,
			},
		},
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         gcnvapi.VolumeSizeStr,
	}

	Labels := driver.getTelemetryLabels(ctx)

	mountTargets := []gcnvapi.MountTarget{
		{
			Export:     "trident-testvol1",
			ExportPath: "1.1.1.1:/trident-testvol1",
			Protocol:   "NFSv3",
		},
	}

	volume := &gcnvapi.Volume{
		Name:              "testvol1",
		CreationToken:     "trident-testvol1",
		FullName:          gcnvapi.FullVolumeName + "testvol1",
		Location:          gcnvapi.Location,
		State:             gcnvapi.StateReady,
		CapacityPool:      "CP1",
		NetworkName:       gcnvapi.NetworkName,
		NetworkFullName:   gcnvapi.NetworkFullName,
		ServiceLevel:      gcnvapi.ServiceLevelPremium,
		SizeBytes:         gcnvapi.VolumeSizeI64,
		ExportPolicy:      exportPolicy,
		ProtocolTypes:     []string{gcnvapi.ProtocolTypeNFSv3},
		MountTargets:      mountTargets,
		UnixPermissions:   "0755",
		Labels:            Labels,
		SnapshotReserve:   0,
		SnapshotDirectory: false,
		SecurityStyle:     "Unix",
	}

	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "testvol1",
		VolumeInternalName: "trident-testvol1",
	}

	snapshot := &gcnvapi.Snapshot{
		Name:     "snap1",
		Volume:   "testvol1",
		Location: gcnvapi.Location,
		State:    gcnvapi.StateReady,
		Created:  time.Now(),
		Labels: map[string]string{
			"backend_uuid":     gcnvapi.BackendUUID,
			"platform":         "",
			"platform_version": "",
			"plugin":           "google-cloud-netapp-volumes",
			"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
		},
	}

	return volConfig, volume, snapConfig, snapshot
}

func TestGCNVCanSnapshot(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	result := driver.CanSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestGetSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, resultErr, "not nil")
	assert.Equal(t, snapConfig, result.Config, "snapshot mismatch")
	assert.Equal(t, int64(0), result.SizeBytes, "snapshot mismatch")
	assert.Equal(t, storage.SnapshotStateOnline, result.State, "snapshot mismatch")
}

func TestGetSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGetSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGetSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NoError(t, resultErr, "error occurred")
}

func TestGetSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errors.NotFoundError("not found")).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NoError(t, resultErr, "error occurred")
}

func TestGetSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGetSnapshot_SnapshotNotReady(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshot.State = gcnvapi.SnapshotStateError

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.NoError(t, resultErr, "expected no error")
}

func getSnapshotsForList(driver *NASStorageDriver, snapTime time.Time) *[]*gcnvapi.Snapshot {
	return &[]*gcnvapi.Snapshot{
		{
			Name:     "snap1",
			Volume:   "testvol1",
			Location: gcnvapi.Location,
			State:    gcnvapi.StateReady,
			Created:  time.Now(),
			Labels: map[string]string{
				"backend_uuid":     gcnvapi.BackendUUID,
				"platform":         "",
				"platform_version": "",
				"plugin":           "google-cloud-netapp-volumes",
				"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
			},
		},
		{
			Name:     "snap2",
			Volume:   "testvol1",
			Location: gcnvapi.Location,
			State:    gcnvapi.StateReady,
			Created:  time.Now(),
			Labels: map[string]string{
				"backend_uuid":     gcnvapi.BackendUUID,
				"platform":         "",
				"platform_version": "",
				"plugin":           "google-cloud-netapp-volumes",
				"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
			},
		},
		{
			Name:     "snap3",
			Volume:   "testvol1",
			Location: gcnvapi.Location,
			State:    gcnvapi.SnapshotStateError,
			Created:  time.Now(),
			Labels: map[string]string{
				"backend_uuid":     gcnvapi.BackendUUID,
				"platform":         "",
				"platform_version": "",
				"plugin":           "google-cloud-netapp-volumes",
				"version":          driver.fixGCPLabelValue(driver.telemetry.TridentVersion),
			},
		},
	}
}

func TestGCNVGetSnapshots(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshots := getSnapshotsForList(driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotsForVolume(ctx, volume).Return(snapshots, nil).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	expectedSnapshot0 := &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			Version:            tridentconfig.OrchestratorAPIVersion,
			Name:               "snap1",
			InternalName:       "snap1",
			VolumeName:         "testvol1",
			VolumeInternalName: "trident-testvol1",
		},
		Created:   snapTime.UTC().Format(convert.TimestampFormat),
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
		Created:   snapTime.UTC().Format(convert.TimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.NotNil(t, result, "received nil")
	assert.NoError(t, resultErr, "error occurred")

	assert.Len(t, result, 2)
	assert.Equal(t, result[0], expectedSnapshot0, "snapshot 0 mismatch")
	assert.Equal(t, result[1], expectedSnapshot1, "snapshot 1 mismatch")
}

func TestGetSnapshots_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGetSnapshots_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGetSnapshots_GetSnapshotsFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, _, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotsForVolume(ctx, volume).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGCNVCreateSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errors.NotFoundError("not found")).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, volume, snapConfig.InternalName, gcnvapi.DefaultTimeout).Return(snapshot, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	expectedSnapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapTime.UTC().Format(convert.TimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.NoError(t, resultErr, "error occurred")
	assert.Equal(t, expectedSnapshot, result, "snapshot mismatch")
}

func TestCreateSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestCreateSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestCreateSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestCreateSnapshot_SnapshotExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGCNVCreateSnapshot_SnapshotExistsNotReady(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshot.State = gcnvapi.SnapshotStateError

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGCNVCreateSnapshot_SnapshotExistsReady(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	expectedSnapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapTime.UTC().Format(convert.TimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.NoError(t, resultErr, "error occurred")
	assert.Equal(t, expectedSnapshot, result, "snapshot mismatch")
}

func TestGCNVCreateSnapshot_SnapshotCreateFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errors.NotFoundError("not found")).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, volume, "snap1", gomock.Any()).Return(nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "expected error")
}

func TestGCNVRestoreSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, volume, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{
		gcnvapi.VolumeStateError,
		gcnvapi.VolumeStateDeleting, gcnvapi.VolumeStateDeleted,
	}, gcnvapi.DefaultSDKTimeout).Return("", nil).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestRestoreSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestRestoreSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestRestoreSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestRestoreSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestRestoreSnapshot_SnapshotRestoreFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, volume, snapshot).Return(errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestRestoreSnapshot_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, volume, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, gcnvapi.VolumeStateReady, []string{
		gcnvapi.VolumeStateError, gcnvapi.VolumeStateDeleting, gcnvapi.VolumeStateDeleted,
	}, gcnvapi.DefaultSDKTimeout).Return("", errFailed).Times(1)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestDeleteSnapshot_SnapshotReady(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, volume, snapshot, gomock.Any()).Return(nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDeleteSnapshot_SnapshotError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshot.State = gcnvapi.SnapshotStateError

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, volume, snapshot, gomock.Any()).Return(nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestDeleteSnapshot_SnapshotOtherState(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)
	snapshot.State = gcnvapi.SnapshotStateDeleting

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestDeleteSnapshot_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestDeleteSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestDeleteSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, _, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot exists")
}

func TestDeleteSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errors.NotFoundError("not found")).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot exists")
}

func TestDeleteSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, _ := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func TestDeleteSnapshot_SnapshotDeleteFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)
	snapTime := time.Now()

	volConfig, volume, snapConfig, snapshot := getStructsForCreateSnapshot(ctx, driver, snapTime)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, snapConfig.InternalName).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, volume, snapshot, gomock.Any()).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
}

func getVolumesForList() *[]*gcnvapi.Volume {
	return &[]*gcnvapi.Volume{
		{
			State:         gcnvapi.VolumeStateReady,
			CreationToken: "myPrefix-testvol1",
		},
		{
			State:         gcnvapi.VolumeStateReady,
			CreationToken: "myPrefix-testvol2",
		},
		{
			State:         gcnvapi.VolumeStateDeleting,
			CreationToken: "myPrefix-testvol3",
		},
		{
			State:         gcnvapi.VolumeStateError,
			CreationToken: "myPrefix-testvol5",
		},
		{
			State:         gcnvapi.VolumeStateReady,
			CreationToken: "testvol6",
		},
	}
}

func TestGCNVList(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

	list, result := driver.List(ctx)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, list, []string{"testvol1", "testvol2"}, "list not equal")
}

func TestList_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	list, result := driver.List(ctx)

	assert.Error(t, result, "expected error")
	assert.Nil(t, list, "list not nil")
}

func TestList_ListFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

	list, result := driver.List(ctx)

	assert.Error(t, result, "expected error")
	assert.Nil(t, list, "list not nil")
}

func TestList_ListNone(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(&[]*gcnvapi.Volume{}, nil).Times(1)

	list, result := driver.List(ctx)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, []string{}, list, "list not equal")
}

func TestGCNVGet(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := &gcnvapi.Volume{}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "volume1").Return(volumes, nil).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.Nil(t, result, "not nil")
}

func TestGet_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.Error(t, result, "expected error")
}

func TestGet_NotFound(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "volume1").Return(nil, errFailed).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.Error(t, result, "expected error")
}

func TestResize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	volConfig.InternalID = ""
	newSize := uint64(gcnvapi.VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().ResizeVolume(ctx, volume, int64(newSize)).Return(nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, strconv.FormatUint(newSize, 10), volConfig.Size, "size mismatch")
}

func TestResize_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	newSize := uint64(gcnvapi.VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "expected error")
}

func TestResize_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _ := getStructsForDestroyNFSVolume(ctx, driver)

	newSize := uint64(gcnvapi.VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "expected error")
}

func TestResize_VolumeNotReady(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	volume.State = gcnvapi.SnapshotStateError
	newSize := uint64(gcnvapi.VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "expected error")
}

func TestGCNVResize_NoSizeChange(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(gcnvapi.VolumeSizeI64)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, gcnvapi.VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_ShrinkingVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(gcnvapi.VolumeSizeI64 / 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "expected error")
	assert.Equal(t, gcnvapi.VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_AboveMaximumSize(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)
	driver.Config.LimitVolumeSize = strconv.FormatInt(gcnvapi.VolumeSizeI64+1, 10)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(gcnvapi.VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "expected error")
	assert.Equal(t, gcnvapi.VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_VolumeResizeFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume := getStructsForDestroyNFSVolume(ctx, driver)
	newSize := uint64(gcnvapi.VolumeSizeI64 * 2)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().ResizeVolume(ctx, volume, int64(newSize)).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "expected error")
	assert.Equal(t, gcnvapi.VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestGCNVGetStorageBackendSpecs(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	err := driver.populateConfigurationDefaults(ctx, &driver.Config)
	assert.NoError(t, err, "error occurred")

	driver.initializeStoragePools(ctx)
	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	backend := &storage.StorageBackend{}
	backend.ClearStoragePools()

	result := driver.GetStorageBackendSpecs(ctx, backend)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, "googlecloudnetappvolumes_12345", backend.Name(), "backend name mismatch")
	for _, pool := range driver.pools {
		assert.Equal(t, backend, pool.Backend(), "pool-backend mismatch")
		p, ok := backend.StoragePools().Load("googlecloudnetappvolumes_12345_pool")
		assert.True(t, ok)
		assert.Equal(t, pool, p.(storage.Pool), "backend-pool mismatch")
	}
}

func TestGCNVCreatePrepare(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["gcnv_pool"]
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	driver.CreatePrepare(ctx, volConfig, storagePool)

	assert.Equal(t, "myPrefix-testvol1", volConfig.InternalName)
}

func TestGCNVGetStorageBackendPhysicalPoolNames(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	result := driver.GetStorageBackendPhysicalPoolNames(ctx)

	assert.Equal(t, []string{}, result, "physical pool names mismatch")
}

func TestGetInternalVolumeName_PassthroughStore(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["gcnv_pool"]
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	result := driver.GetInternalVolumeName(ctx, volConfig, storagePool)

	assert.Equal(t, "myPrefix-testvol1", result, "internal name mismatch")
}

func TestGetInternalVolumeName_CSI(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["gcnv_pool"]
	volConfig := &storage.VolumeConfig{Name: "pvc-5e522901-b891-41d8-9e83-5496d2e62e71"}

	result := driver.GetInternalVolumeName(ctx, volConfig, storagePool)

	assert.Equal(t, "pvc-5e522901-b891-41d8-9e83-5496d2e62e71", result, "internal name mismatch")
}

func TestGetInternalVolumeName_NonCSI(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	storagePool := driver.pools["gcnv_pool"]
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	result := driver.GetInternalVolumeName(ctx, volConfig, storagePool)

	gcnvRegex := regexp.MustCompile(`^gcnv-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	assert.True(t, gcnvRegex.MatchString(result), "internal name mismatch")
}

func TestCreateFollowup_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Contains(t, (volume.MountTargets)[0].ExportPath, volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "/"+volume.CreationToken, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "nfs", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_ROClone_NFSVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.CloneSourceSnapshot = gcnvapi.SnapshotUUID
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, volConfig.CloneSourceVolumeInternal).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	// TODO : Verify RO clone implementation.

	assert.Nil(t, result, "not nil")
	assert.Contains(t, (volume.MountTargets)[0].ExportPath, volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	// assert.Equal(t, "/testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", volConfig.AccessInfo.NfsPath,
	//	"NFS path mismatch")
	assert.Equal(t, "nfs", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_SMBVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "smb"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishSMBVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Contains(t, (volume.MountTargets)[0].ExportPath, volConfig.AccessInfo.SMBServer, "SMB server mismatch")
	assert.Equal(t, "\\"+volume.CreationToken, volConfig.AccessInfo.SMBPath, "SMB path mismatch")
	assert.Equal(t, "smb", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _, _ := getStructsForPublishNFSVolume(ctx, driver)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_ROClone_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, _, _ := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.CloneSourceSnapshot = gcnvapi.SnapshotUUID
	volConfig.ReadOnlyClone = true

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, volConfig.CloneSourceVolumeInternal).Return(nil, errFailed).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_VolumeNotAvailable(t *testing.T) {
	nonAvailableStates := []string{
		gcnvapi.VolumeStateError, gcnvapi.VolumeStateDeleted, gcnvapi.VolumeStateDeleting, gcnvapi.VolumeStateCreating,
		gcnvapi.VolumeStateDisabled, gcnvapi.VolumeStateRestoring, gcnvapi.VolumeStateUnspecified, gcnvapi.VolumeStateUpdating,
	}

	for _, state := range nonAvailableStates {
		mockAPI, driver := newMockGCNVDriver(t)

		driver.Config.BackendName = "gcnv"
		driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
		driver.Config.NASType = "nfs"

		driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

		volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)
		volume.State = state

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

		result := driver.CreateFollowup(ctx, volConfig)

		assert.Error(t, result, "expected error")
		assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
		assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
		assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
	}
}

func TestCreateFollowup_NoMountTargets(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)
	volume.MountTargets = nil

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_MountOptions_ParseNFSExportPathError(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium
	driver.Config.NASType = "nfs"

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)
	volume.MountTargets[0].ExportPath = "1234:testvol1"

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestGCNVGetProtocol(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	result := driver.GetProtocol(ctx)

	assert.Equal(t, tridentconfig.File, result)
}

func TestGCNVStoreConfig(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "google-cloud-netapp-volumes",
		BackendName:       "myGCNVBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	_, driver := newMockGCNVDriver(t)
	driver.Config.CommonStorageDriverConfig = commonConfig

	persistentConfig := &storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, persistentConfig)

	assert.Equal(t, json.RawMessage("{}"), driver.Config.CommonStorageDriverConfig.StoragePrefixRaw,
		"raw prefix mismatch")
	assert.Equal(t, driver.Config, *persistentConfig.GCNVConfig, "gcnv config mismatch")
}

func TestGCNVGetVolumeForImport(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "myPrefix-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.StateReady,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, "testvol1").Return(volume, nil).Times(1)

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "testvol1", result.Config.Name)
	assert.Equal(t, "myPrefix-testvol1", result.Config.InternalName)
}

func TestGCNVGetVolumeForImport_VolumeNameWithPrefix(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volume := &gcnvapi.Volume{
		Name:          "myPrefix-testvol1",
		CreationToken: "myPrefix-testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.StateReady,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, "testvol1").Return(volume, nil).Times(1)

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "testvol1", result.Config.Name)
	assert.Equal(t, "myPrefix-testvol1", result.Config.InternalName)
}

func TestGetVolumeForImport_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "error expected")
}

func TestGetVolumeForImport_GetFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByNameOrID(ctx, "testvol1").Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetVolumeForImport(ctx, "testvol1")

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "error expected")
}

func TestGCNVGetExternalConfig(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	result := driver.GetExternalConfig(ctx)

	assert.NotNil(t, result, "received nil")
}

func TestGCNVGetVolumeExternalWrappers(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(*volumes))

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumesExternal := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			volumesExternal = append(volumesExternal, wrapper.Volume)
		}
	}

	assert.Len(t, volumesExternal, 2, "wrong number of volumes")
}

func TestGetVolumeExternalWrappers_DiscoveryFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(*volumes))

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	var result error
	for wrapper := range channel {
		if wrapper.Error != nil {
			result = wrapper.Error
			break
		}
	}

	assert.Error(t, result, "expected error")
}

func TestGetVolumeExternalWrappers_ListFailed(t *testing.T) {
	mockAPI, driver := newMockGCNVDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(*volumes))

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
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

	assert.Error(t, result, "expected error")
}

func TestStringAndGoString(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	stringFunc := func(d *NASStorageDriver) string { return d.String() }
	goStringFunc := func(d *NASStorageDriver) string { return d.GoString() }

	for _, toString := range []func(*NASStorageDriver) string{stringFunc, goStringFunc} {
		assert.Contains(t, toString(driver), "<REDACTED>", "GCNV driver does not contain <REDACTED>")
		assert.Contains(t, toString(driver), "ProjectNumber:<REDACTED>", "GCNV driver does not redact ProjectNumber")
		assert.Contains(t, toString(driver), "APIKey:<REDACTED>", "GCNV driver does not redact APIKey")
		assert.Contains(t, toString(driver), "Credentials:map[string]string{\"name\":\"<REDACTED>\", \"type\":\"<REDACTED>\"}", "GCNV driver does not redact Credentials")
		assert.NotContains(t, toString(driver), gcnvapi.ProjectNumber, "GCNV driver contains ProjectNumber")
		assert.NotContains(t, toString(driver), driver.Config.APIKey, "GCNV driver contains APIKey")
		assert.NotContains(t, toString(driver), driver.Config.Credentials, "GCNV driver contains Credentials")
	}
}

func TestGetUpdateType_NoFlaggedChanges(t *testing.T) {
	_, oldDriver := newMockGCNVDriver(t)
	oldDriver.volumeCreateTimeout = 1 * time.Second

	_, newDriver := newMockGCNVDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.Equal(t, &roaring.Bitmap{}, result, "bitmap mismatch")
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

	_, newDriver := newMockGCNVDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestGetUpdateType_OtherChanges(t *testing.T) {
	_, oldDriver := newMockGCNVDriver(t)
	prefix1 := "prefix1-"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	_, newDriver := newMockGCNVDriver(t)
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

func TestGCNVReconcileNodeAccess(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	err := driver.ReconcileNodeAccess(ctx, nil, "", "")

	assert.Nil(t, err, "not nil")
}

func TestGCNVGetCommonConfig(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.Config.BackendName = "gcnv"
	driver.Config.ServiceLevel = gcnvapi.ServiceLevelPremium

	result := driver.GetCommonConfig(ctx)

	assert.Equal(t, driver.Config.CommonStorageDriverConfig, result, "common config mismatch")
}

func TestGCNVValidateStoragePrefix(t *testing.T) {
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
			err := validateGCNVStoragePrefix(test.StoragePrefix)
			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}

func TestConstructVolumeAccessPath(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)
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
			result := constructVolumeAccessPath(volConfig, volume, test.Protocol)
			if test.Valid {
				assert.NotEmpty(t, result, "access path should not be empty")
			} else {
				assert.Empty(t, result, "access path should be empty")
			}
		})
	}
}

func TestConstructVolumeAccessPath_NFSVolume_ROClone(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishNFSVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.CloneSourceSnapshot = gcnvapi.SnapshotUUID
	volConfig.ReadOnlyClone = true

	result := constructVolumeAccessPath(volConfig, volume, "nfs")

	assert.NotNil(t, result, "received nil")
	assert.Equal(t, "/testvol1/.snapshot/deadbeef-5c0d-4afa-8cd8-afa3fba5665c", result, "volume access path mismatch")
}

func TestConstructVolumeAccessPath_SMBVolume_ROClone(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	driver.initializeTelemetry(ctx, gcnvapi.BackendUUID)

	volConfig, volume, _ := getStructsForPublishSMBVolume(ctx, driver)
	volConfig.CloneSourceVolumeInternal = volConfig.Name
	volConfig.CloneSourceSnapshot = gcnvapi.SnapshotUUID
	volConfig.ReadOnlyClone = true

	result := constructVolumeAccessPath(volConfig, volume, "smb")

	assert.NotNil(t, result, "received nil")
	assert.Equal(t, "\\testvol1\\~snapshot\\deadbeef-5c0d-4afa-8cd8-afa3fba5665c", result, "volume access path mismatch")
}

func TestNFSExportComponentsForProtocol_ProtocolIsEmpty(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.StateReady,
	}

	server, share, resultErr := driver.nfsExportComponentsForProtocol(volume, "")

	assert.Error(t, resultErr, "expected error")
	assert.Equal(t, "", server, "server value is populated")
	assert.Equal(t, "", share, "share name is populated")
}

func TestNFSExportComponentsForProtocol_IncorrectProtocol(t *testing.T) {
	_, driver := newMockGCNVDriver(t)

	volume := &gcnvapi.Volume{
		Name:          "testvol1",
		CreationToken: "testvol1",
		FullName:      gcnvapi.FullVolumeName + "testvol1",
		State:         gcnvapi.StateReady,
	}

	server, share, resultErr := driver.nfsExportComponentsForProtocol(volume, "SMB")

	assert.Error(t, resultErr, "expected error")
	assert.Equal(t, "", server, "server value is populated")
	assert.Equal(t, "", share, "share name is populated")
}
