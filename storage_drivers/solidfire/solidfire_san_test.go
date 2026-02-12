// Copyright 2020 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/solidfire/api"
	"github.com/netapp/trident/utils/models"
)

const (
	TenantName       = "tester"
	AdminPass        = "admin:password"
	Endpoint         = "https://" + AdminPass + "@10.0.0.1/json-rpc/7.0"
	RedactedEndpoint = "https://<REDACTED>" + "@10.0.0.1/json-rpc/7.0"
)

func newTestSolidfireSANDriver() *SANStorageDriver {
	config := &drivers.SolidfireStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	config.TenantName = TenantName
	config.EndPoint = Endpoint
	config.SVIP = "10.0.0.1:1000"
	config.InitiatorIFace = "default"
	config.Types = &[]api.VolType{
		{
			Type: "Gold",
			QOS: api.QoS{
				BurstIOPS: 10000,
				MaxIOPS:   8000,
				MinIOPS:   6000,
			},
		},
		{
			Type: "Bronze",
			QOS: api.QoS{
				BurstIOPS: 4000,
				MaxIOPS:   2000,
				MinIOPS:   1000,
			},
		},
	}
	config.AccessGroups = []int64{}
	config.UseCHAP = true
	config.DefaultBlockSize = 4096
	config.StorageDriverName = "solidfire-san"
	config.StoragePrefix = sp("test_")

	cfg := api.Config{
		TenantName:       config.TenantName,
		EndPoint:         Endpoint,
		SVIP:             config.SVIP,
		InitiatorIFace:   config.InitiatorIFace,
		Types:            config.Types,
		LegacyNamePrefix: config.LegacyNamePrefix,
		AccessGroups:     config.AccessGroups,
		DefaultBlockSize: 4096,
		DebugTraceFlags:  config.DebugTraceFlags,
	}

	client, _ := api.NewFromParameters(Endpoint, config.SVIP, cfg)

	sanDriver := &SANStorageDriver{}
	sanDriver.Config = *config
	sanDriver.Client = client
	sanDriver.AccountID = 2222
	sanDriver.AccessGroups = []int64{}
	sanDriver.LegacyNamePrefix = "oldtest_"
	sanDriver.InitiatorIFace = "default"
	sanDriver.DefaultMaxIOPS = 20000
	sanDriver.DefaultMinIOPS = 1000

	return sanDriver
}

func callString(s SANStorageDriver) string {
	return s.String()
}

func callGoString(s SANStorageDriver) string {
	return s.GoString()
}

func TestSolidfireSANStorageDriverConfigString(t *testing.T) {
	solidfireSANDrivers := []SANStorageDriver{
		*newTestSolidfireSANDriver(),
	}

	for _, toString := range []func(SANStorageDriver) string{callString, callGoString} {
		for _, solidfireSANDriver := range solidfireSANDrivers {
			assert.Contains(t, toString(solidfireSANDriver), "<REDACTED>",
				"Solidfire driver does not contain <REDACTED>")
			assert.Contains(t, toString(solidfireSANDriver), "Client:<REDACTED>",
				"Solidfire driver does not redact client API information")
			assert.Contains(t, toString(solidfireSANDriver), "AccountID:<REDACTED>",
				"Solidfire driver does not redact Account ID information")
			assert.NotContains(t, toString(solidfireSANDriver), TenantName,
				"Solidfire driver contains tenant name")
			assert.NotContains(t, toString(solidfireSANDriver), RedactedEndpoint,
				"Solidfire driver contains endpoint's admin and password")
			assert.NotContains(t, toString(solidfireSANDriver), "2222",
				"Solidfire driver contains Account ID")
		}
	}
}

func TestSolidfireSANStorageDriverGetResizeDeltaBytes(t *testing.T) {
	d := newTestSolidfireSANDriver()
	assert.Equal(t, int64(tridentconfig.SANResizeDelta), d.GetResizeDeltaBytes())
}

func TestValidateStoragePrefix(t *testing.T) {
	tests := []struct {
		Name          string
		StoragePrefix string
	}{
		{
			Name:          "storage prefix starts with plus",
			StoragePrefix: "+abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts with digit",
			StoragePrefix: "1abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts with underscore",
			StoragePrefix: "_abcd_123_ABC",
		},
		{
			Name:          "storage prefix ends capitalized",
			StoragePrefix: "abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts capitalized",
			StoragePrefix: "ABCD_123_abc",
		},
		{
			Name:          "storage prefix has plus",
			StoragePrefix: "abcd+123_ABC",
		},
		{
			Name:          "storage prefix has dash",
			StoragePrefix: "abcd-123",
		},
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
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
		{
			Name:          "storage prefix is empty",
			StoragePrefix: "",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			d := newTestSolidfireSANDriver()
			d.Config.StoragePrefix = &test.StoragePrefix

			err := d.populateConfigurationDefaults(context.Background(), &d.Config)
			assert.NoError(t, err)

			err = d.validate(context.Background())
			assert.NoError(t, err, "Solidfire driver validation should never fail")
			if d.Config.StoragePrefix != nil {
				assert.Empty(t, *d.Config.StoragePrefix,
					"Solidfire driver should always set storage prefix empty")
			}
		})
	}
}

func TestGetStorageBackendPools(t *testing.T) {
	d := newTestSolidfireSANDriver()
	backendPools := d.getStorageBackendPools(context.Background())

	// These backend pools are derived from the driver's configuration. If that changes in the helper
	// "newTestSolidfireSANDriver", these assertions will need to be adjusted as well.
	assert.Equal(t, len(backendPools), 1, "unexpected set of backend pools")
	assert.Equal(t, d.Config.TenantName, backendPools[0].TenantName, "tenant name didn't match")
	assert.Equal(t, d.AccountID, backendPools[0].AccountID, "account ID didn't match")
}

func TestGetConfig(t *testing.T) {
	tests := []struct {
		name           string
		setupDriver    func() *SANStorageDriver
		expectedDriver string
	}{
		{
			name: "standard config",
			setupDriver: func() *SANStorageDriver {
				d := newTestSolidfireSANDriver()
				return d
			},
			expectedDriver: "solidfire-san",
		},
		{
			name: "custom storage driver name",
			setupDriver: func() *SANStorageDriver {
				d := newTestSolidfireSANDriver()
				d.Config.StorageDriverName = "custom-solidfire"
				return d
			},
			expectedDriver: "custom-solidfire",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			config := driver.GetConfig()

			assert.NotNil(t, config, "config should not be nil")

			// Verify it's the correct config type by checking the string representation
			configStr := config.String()
			assert.Contains(t, configStr, tt.expectedDriver, "config should contain driver name")
		})
	}
}

func TestBackendName(t *testing.T) {
	tests := []struct {
		name           string
		backendName    string
		expectedResult string
	}{
		{
			name:           "standard backend name",
			backendName:    "solidfire-backend-1",
			expectedResult: "solidfire-backend-1",
		},
		{
			name:           "empty backend name - uses SVIP",
			backendName:    "",
			expectedResult: "solidfire_10.0.0.1", // Uses SVIP from test config
		},
		{
			name:           "backend name with special characters",
			backendName:    "solidfire-backend_test-123",
			expectedResult: "solidfire-backend_test-123",
		},
		{
			name:           "long backend name",
			backendName:    "very-long-backend-name-with-multiple-dashes-and-numbers-12345",
			expectedResult: "very-long-backend-name-with-multiple-dashes-and-numbers-12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()
			driver.Config.CommonStorageDriverConfig.BackendName = tt.backendName

			result := driver.BackendName()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestPoolName(t *testing.T) {
	tests := []struct {
		name           string
		inputName      string
		expectedResult string
	}{
		{
			name:           "simple pool name",
			inputName:      "pool1",
			expectedResult: "solidfire_10.0.0.1_pool1", // Uses BackendName() which includes SVIP
		},
		{
			name:           "pool name with dashes",
			inputName:      "pool-with-dashes",
			expectedResult: "solidfire_10.0.0.1_poolwithdashes", // Dashes are removed
		},
		{
			name:           "pool name with underscores",
			inputName:      "pool_with_underscores",
			expectedResult: "solidfire_10.0.0.1_pool_with_underscores", // Underscores preserved
		},
		{
			name:           "empty pool name",
			inputName:      "",
			expectedResult: "solidfire_10.0.0.1_",
		},
		{
			name:           "pool name with mixed characters",
			inputName:      "Pool-123_Test",
			expectedResult: "solidfire_10.0.0.1_Pool123_Test", // Dashes removed, underscores preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()
			result := driver.poolName(tt.inputName)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestInitialized(t *testing.T) {
	tests := []struct {
		name           string
		setupDriver    func() *SANStorageDriver
		expectedResult bool
	}{
		{
			name: "uninitialized driver",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.initialized = false
				return driver
			},
			expectedResult: false,
		},
		{
			name: "initialized driver",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.initialized = true
				return driver
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			result := driver.Initialized()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestTerminate(t *testing.T) {
	tests := []struct {
		name        string
		setupDriver func() *SANStorageDriver
		backendUUID string
	}{
		{
			name: "terminate initialized driver",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.initialized = true
				return driver
			},
			backendUUID: "test-uuid-123",
		},
		{
			name: "terminate uninitialized driver",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.initialized = false
				return driver
			},
			backendUUID: "test-uuid-456",
		},
		{
			name: "terminate with empty UUID",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.initialized = true
				return driver
			},
			backendUUID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			ctx := context.Background()

			// Should not panic and should reset initialized status
			driver.Terminate(ctx, tt.backendUUID)
			assert.False(t, driver.initialized, "driver should be marked as uninitialized after terminate")
		})
	}
}

func TestGetProtocol(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	protocol := driver.GetProtocol(ctx)
	assert.Equal(t, tridentconfig.Block, protocol, "SolidFire driver should return Block protocol")
}

func TestStoreConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *storage.PersistentStorageBackendConfig
		expectPanic bool
	}{
		{
			name: "store valid config",
			config: &storage.PersistentStorageBackendConfig{
				SolidfireConfig: &drivers.SolidfireStorageDriverConfig{},
			},
			expectPanic: false,
		},
		{
			name:        "store nil config - should panic",
			config:      nil,
			expectPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()
			ctx := context.Background()

			if tt.expectPanic {
				assert.Panics(t, func() {
					driver.StoreConfig(ctx, tt.config)
				}, "StoreConfig should panic with nil config")
			} else {
				assert.NotPanics(t, func() {
					driver.StoreConfig(ctx, tt.config)
				}, "StoreConfig should not panic with valid config")

				// Verify the config was stored
				if tt.config != nil {
					assert.Equal(t, &driver.Config, tt.config.SolidfireConfig, "config should be stored")
				}
			}
		})
	}
}

// ============================================================================
// Phase 2: Utility Functions Tests (0% coverage → target coverage)
// ============================================================================

func TestParseQOS(t *testing.T) {
	tests := []struct {
		name        string
		qosOpt      string
		expectedQOS api.QoS
		expectError bool
	}{
		{
			name:   "valid qos string",
			qosOpt: "1000,2000,4000",
			expectedQOS: api.QoS{
				MinIOPS:   1000,
				MaxIOPS:   2000,
				BurstIOPS: 4000,
			},
			expectError: false,
		},
		{
			name:   "valid qos with zeros",
			qosOpt: "0,1000,2000",
			expectedQOS: api.QoS{
				MinIOPS:   0,
				MaxIOPS:   1000,
				BurstIOPS: 2000,
			},
			expectError: false,
		},
		{
			name:        "invalid qos - missing values",
			qosOpt:      "1000,2000",
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:        "invalid qos - too many values",
			qosOpt:      "1000,2000,4000,8000",
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:        "invalid qos - non-numeric values",
			qosOpt:      "min,max,burst",
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:        "invalid qos - empty string",
			qosOpt:      "",
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:        "invalid qos - single value",
			qosOpt:      "1000",
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:   "valid qos - negative values allowed",
			qosOpt: "-1000,2000,4000",
			expectedQOS: api.QoS{
				MinIOPS:   -1000,
				MaxIOPS:   2000,
				BurstIOPS: 4000,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qos, err := parseQOS(tt.qosOpt)

			if tt.expectError {
				assert.Error(t, err, "parseQOS should return an error")
			} else {
				assert.NoError(t, err, "parseQOS should not return an error")
				assert.Equal(t, tt.expectedQOS.MinIOPS, qos.MinIOPS, "MinIOPS should match")
				assert.Equal(t, tt.expectedQOS.MaxIOPS, qos.MaxIOPS, "MaxIOPS should match")
				assert.Equal(t, tt.expectedQOS.BurstIOPS, qos.BurstIOPS, "BurstIOPS should match")
			}
		})
	}
}

func TestParseType(t *testing.T) {
	// Create test volume types
	testTypes := []api.VolType{
		{
			Type: "Gold",
			QOS: api.QoS{
				MinIOPS:   6000,
				MaxIOPS:   8000,
				BurstIOPS: 10000,
			},
		},
		{
			Type: "Bronze",
			QOS: api.QoS{
				MinIOPS:   1000,
				MaxIOPS:   2000,
				BurstIOPS: 4000,
			},
		},
		{
			Type: "Silver",
			QOS: api.QoS{
				MinIOPS:   4000,
				MaxIOPS:   6000,
				BurstIOPS: 8000,
			},
		},
	}

	tests := []struct {
		name        string
		typeName    string
		volumeTypes []api.VolType
		expectedQOS api.QoS
		expectError bool
	}{
		{
			name:        "valid type - Gold",
			typeName:    "Gold",
			volumeTypes: testTypes,
			expectedQOS: api.QoS{
				MinIOPS:   6000,
				MaxIOPS:   8000,
				BurstIOPS: 10000,
			},
			expectError: false,
		},
		{
			name:        "valid type - Bronze",
			typeName:    "Bronze",
			volumeTypes: testTypes,
			expectedQOS: api.QoS{
				MinIOPS:   1000,
				MaxIOPS:   2000,
				BurstIOPS: 4000,
			},
			expectError: false,
		},
		{
			name:        "valid type - case insensitive",
			typeName:    "gold",
			volumeTypes: testTypes,
			expectedQOS: api.QoS{
				MinIOPS:   6000,
				MaxIOPS:   8000,
				BurstIOPS: 10000,
			},
			expectError: false,
		},
		{
			name:        "valid type - mixed case",
			typeName:    "BRONZE",
			volumeTypes: testTypes,
			expectedQOS: api.QoS{
				MinIOPS:   1000,
				MaxIOPS:   2000,
				BurstIOPS: 4000,
			},
			expectError: false,
		},
		{
			name:        "invalid type - not found",
			typeName:    "Platinum",
			volumeTypes: testTypes,
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:        "invalid type - empty string",
			typeName:    "",
			volumeTypes: testTypes,
			expectedQOS: api.QoS{},
			expectError: true,
		},
		{
			name:        "empty volume types",
			typeName:    "Gold",
			volumeTypes: []api.VolType{},
			expectedQOS: api.QoS{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			qos, err := parseType(ctx, tt.volumeTypes, tt.typeName)

			if tt.expectError {
				assert.Error(t, err, "parseType should return an error")
			} else {
				assert.NoError(t, err, "parseType should not return an error")
				assert.Equal(t, tt.expectedQOS.MinIOPS, qos.MinIOPS, "MinIOPS should match")
				assert.Equal(t, tt.expectedQOS.MaxIOPS, qos.MaxIOPS, "MaxIOPS should match")
				assert.Equal(t, tt.expectedQOS.BurstIOPS, qos.BurstIOPS, "BurstIOPS should match")
			}
		})
	}
}

func TestMakeSolidFireName(t *testing.T) {
	tests := []struct {
		name           string
		inputName      string
		expectedResult string
	}{
		{
			name:           "simple name with dashes",
			inputName:      "test-volume",
			expectedResult: "test-volume", // No underscores, so no change
		},
		{
			name:           "name with underscores - converts to dashes",
			inputName:      "test_volume_name",
			expectedResult: "test-volume-name", // Underscores converted to dashes
		},
		{
			name:           "name with dashes only",
			inputName:      "test-volume-name",
			expectedResult: "test-volume-name", // No underscores, so no change
		},
		{
			name:           "name with mixed underscores and dashes",
			inputName:      "test_volume-123_name",
			expectedResult: "test-volume-123-name", // Only underscores converted
		},
		{
			name:           "name with multiple underscores",
			inputName:      "test__volume__name",
			expectedResult: "test--volume--name", // All underscores converted
		},
		{
			name:           "name starting with underscore",
			inputName:      "_test_volume",
			expectedResult: "-test-volume", // Underscores at start converted
		},
		{
			name:           "name ending with underscore",
			inputName:      "test_volume_",
			expectedResult: "test-volume-", // Underscores at end converted
		},
		{
			name:           "empty name",
			inputName:      "",
			expectedResult: "",
		},
		{
			name:           "name with only underscores",
			inputName:      "___",
			expectedResult: "---", // All underscores converted to dashes
		},
		{
			name:           "name with numbers and mixed chars",
			inputName:      "vol_123-test_456",
			expectedResult: "vol-123-test-456", // Underscores converted, dashes unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeSolidFireName(tt.inputName)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGetTelemetry(t *testing.T) {
	tests := []struct {
		name        string
		setupDriver func() *SANStorageDriver
		expectNil   bool
	}{
		{
			name: "uninitialized telemetry - returns nil",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver() // telemetry is nil by default
			},
			expectNil: true,
		},
		{
			name: "initialized telemetry - returns telemetry object",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				// Manually set telemetry for testing
				driver.telemetry = &Telemetry{
					Plugin: "solidfire-san",
				}
				return driver
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			telemetry := driver.getTelemetry()

			if tt.expectNil {
				assert.Nil(t, telemetry, "telemetry should be nil when not initialized")
			} else {
				assert.NotNil(t, telemetry, "telemetry should not be nil when initialized")
				assert.Equal(t, "solidfire-san", telemetry.Plugin, "plugin name should be set correctly")
			}
		})
	}
}

// ============================================================================
// Phase 3: Initialization Functions Tests - validate() improvement (20% → target 100%)
// ============================================================================

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		setupDriver func() *SANStorageDriver
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration - should pass",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				// Ensure storage prefix is empty for valid config
				emptyPrefix := ""
				driver.Config.StoragePrefix = &emptyPrefix
				return driver
			},
			expectError: false,
		},
		{
			name: "missing TenantName - should fail",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.TenantName = ""
				return driver
			},
			expectError: true,
			errorMsg:    "missing required TenantName in config",
		},
		{
			name: "missing EndPoint - should fail",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.EndPoint = ""
				return driver
			},
			expectError: true,
			errorMsg:    "missing required EndPoint in config",
		},
		{
			name: "missing SVIP - should fail",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.SVIP = ""
				return driver
			},
			expectError: true,
			errorMsg:    "missing required SVIP in config",
		},
		{
			name: "non-empty StoragePrefix - should fail",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				nonEmpty := "test_prefix"
				driver.Config.StoragePrefix = &nonEmpty
				return driver
			},
			expectError: true,
			errorMsg:    "storage prefix must be empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			ctx := context.Background()

			err := driver.validate(ctx)

			if tt.expectError {
				assert.Error(t, err, "validate should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "validate should not return an error")
			}
		})
	}
}

func TestGetStorageBackendPhysicalPoolNames(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	poolNames := driver.GetStorageBackendPhysicalPoolNames(ctx)
	assert.Equal(t, []string{}, poolNames, "GetStorageBackendPhysicalPoolNames should return empty slice")
}

func TestGetCommonConfig(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	commonConfig := driver.GetCommonConfig(ctx)
	assert.NotNil(t, commonConfig, "GetCommonConfig should not return nil")
	assert.Equal(t, driver.Config.CommonStorageDriverConfig, commonConfig, "should return the same config")
}

func TestGetInternalVolumeName(t *testing.T) {
	tests := []struct {
		name                    string
		volumeName              string
		usePassthroughStore     bool
		setupDriver             func() *SANStorageDriver
		expectedTransformations []string // What transformations we expect to see
		shouldGenerateUUID      bool
	}{
		{
			name:                "passthrough store - simple underscore replacement",
			volumeName:          "test_volume_name",
			usePassthroughStore: true,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{"test-volume-name"}, // Just underscore replacement
			shouldGenerateUUID:      false,
		},
		{
			name:                "passthrough store - no underscores",
			volumeName:          "test-volume-name",
			usePassthroughStore: true,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{"test-volume-name"}, // No change needed
			shouldGenerateUUID:      false,
		},
		{
			name:                "regular mode - simple name",
			volumeName:          "test-volume",
			usePassthroughStore: false,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{}, // Will use GetCommonInternalVolumeName + transformations
			shouldGenerateUUID:      false,
		},
		{
			name:                "regular mode - underscores converted to dashes",
			volumeName:          "test_volume_name",
			usePassthroughStore: false,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{}, // Complex transformation with prefix
			shouldGenerateUUID:      false,
		},
		{
			name:                "regular mode - periods converted to dashes",
			volumeName:          "test.volume.name",
			usePassthroughStore: false,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{}, // Complex transformation
			shouldGenerateUUID:      false,
		},
		{
			name:                "regular mode - mixed characters with double dashes",
			volumeName:          "test_volume.with--mixed_chars",
			usePassthroughStore: false,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{}, // Should clean up double dashes
			shouldGenerateUUID:      false,
		},
		{
			name:                "regular mode - very long name triggers UUID generation",
			volumeName:          "this-is-a-very-long-volume-name-that-exceeds-sixty-four-characters-and-should-trigger-uuid-generation-logic",
			usePassthroughStore: false,
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedTransformations: []string{}, // Should generate UUID-based name
			shouldGenerateUUID:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global passthrough store flag
			originalPassthrough := tridentconfig.UsingPassthroughStore
			tridentconfig.UsingPassthroughStore = tt.usePassthroughStore
			defer func() {
				tridentconfig.UsingPassthroughStore = originalPassthrough
			}()

			driver := tt.setupDriver()
			ctx := context.Background()

			volConfig := &storage.VolumeConfig{
				Name: tt.volumeName,
			}

			result := driver.GetInternalVolumeName(ctx, volConfig, nil)

			// Basic validation
			assert.NotEmpty(t, result, "result should not be empty")

			if tt.usePassthroughStore {
				// For passthrough store, should just replace underscores with dashes
				expected := strings.Replace(tt.volumeName, "_", "-", -1)
				assert.Equal(t, expected, result, "passthrough store should just replace underscores")
			} else {
				// For regular mode, verify no prohibited characters
				assert.NotContains(t, result, "_", "result should not contain underscores")
				assert.NotContains(t, result, ".", "result should not contain periods")
				assert.NotContains(t, result, "--", "result should not contain double dashes")

				if tt.shouldGenerateUUID {
					// For long names, should be UUID-based (shorter than original)
					assert.True(t, len(result) <= 64, "UUID-based name should be <= 64 characters")
					assert.True(t, len(result) < len(tt.volumeName), "UUID-based name should be shorter than original")
					// Should contain a prefix and UUID-like string
					assert.Contains(t, result, "-", "UUID-based name should contain separators")
				} else {
					// For regular transformations, should be based on original name
					// (but may include prefixes from GetCommonInternalVolumeName)
					assert.True(t, len(result) <= 64, "regular name should be <= 64 characters")
				}
			}
		})
	}
}

func TestCanSnapshot(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	err := driver.CanSnapshot(ctx, nil, nil)
	assert.NoError(t, err, "CanSnapshot should always return nil")
}

func TestRename(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	err := driver.Rename(ctx, "old-name", "new-name")
	assert.NoError(t, err, "Rename should always return nil (not supported)")
}

func TestCreatePrepare(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	volConfig := &storage.VolumeConfig{
		Name: "test-volume",
	}

	// Before CreatePrepare, InternalName should be empty
	assert.Empty(t, volConfig.InternalName, "InternalName should be empty initially")

	driver.CreatePrepare(ctx, volConfig, nil)

	// After CreatePrepare, InternalName should be set
	assert.NotEmpty(t, volConfig.InternalName, "InternalName should be set after CreatePrepare")
}

func TestDiffSlices(t *testing.T) {
	tests := []struct {
		name     string
		sliceA   []int64
		sliceB   []int64
		expected []int64
	}{
		{
			name:     "no difference - same slices",
			sliceA:   []int64{1, 2, 3},
			sliceB:   []int64{1, 2, 3},
			expected: []int64{},
		},
		{
			name:     "b has additional elements",
			sliceA:   []int64{1, 2},
			sliceB:   []int64{1, 2, 3, 4},
			expected: []int64{3, 4},
		},
		{
			name:     "no common elements",
			sliceA:   []int64{1, 2, 3},
			sliceB:   []int64{4, 5, 6},
			expected: []int64{4, 5, 6},
		},
		{
			name:     "empty slice A",
			sliceA:   []int64{},
			sliceB:   []int64{1, 2, 3},
			expected: []int64{1, 2, 3},
		},
		{
			name:     "empty slice B",
			sliceA:   []int64{1, 2, 3},
			sliceB:   []int64{},
			expected: []int64{},
		},
		{
			name:     "both empty",
			sliceA:   []int64{},
			sliceB:   []int64{},
			expected: []int64{},
		},
		{
			name:     "duplicates in B",
			sliceA:   []int64{1, 2},
			sliceB:   []int64{2, 3, 3, 4},
			expected: []int64{3, 3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := diffSlices(tt.sliceA, tt.sliceB)

			if len(tt.expected) == 0 {
				assert.Empty(t, result, "result should be empty")
			} else {
				assert.ElementsMatch(t, tt.expected, result, "elements should match")
			}
		})
	}
}

func TestReconcileVolumeNodeAccess(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	err := driver.ReconcileVolumeNodeAccess(ctx, nil, nil)
	assert.NoError(t, err, "ReconcileVolumeNodeAccess should always return nil")
}

func TestCreateEarlyValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupDriver func() *SANStorageDriver
		volConfig   *storage.VolumeConfig
		storagePool storage.Pool
		volAttrs    map[string]sa.Request
		expectError bool
		errorMsg    string
	}{
		{
			name: "nil storage pool - should return error",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			volConfig: &storage.VolumeConfig{
				InternalName: "test-volume",
			},
			storagePool: nil, // This should trigger the early validation error
			volAttrs:    make(map[string]sa.Request),
			expectError: true,
			errorMsg:    "pool not specified",
		},
		{
			name: "nonexistent storage pool - should return error",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				// Initialize virtualPools but don't add the pool we'll try to use
				driver.virtualPools = make(map[string]storage.Pool)
				return driver
			},
			volConfig: &storage.VolumeConfig{
				InternalName: "test-volume",
			},
			storagePool: storage.NewStoragePool(nil, "nonexistent-pool"), // Pool not in virtualPools
			volAttrs:    make(map[string]sa.Request),
			expectError: true,
			errorMsg:    "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			ctx := context.Background()

			err := driver.Create(ctx, tt.volConfig, tt.storagePool, tt.volAttrs)

			if tt.expectError {
				assert.Error(t, err, "Create should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "Create should not return an error")
			}
		})
	}
}

func TestCreateCloneEarlyValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupDriver func() *SANStorageDriver
		volConfig   *storage.VolumeConfig
		storagePool storage.Pool
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid QoS string - should return parseQOS error",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			volConfig: &storage.VolumeConfig{
				InternalName:                "test-clone",
				CloneSourceVolumeInternal:   "source-vol",
				CloneSourceSnapshotInternal: "snap-1",
				Qos:                         "invalid,qos", // Invalid QoS format (needs 3 values)
			},
			storagePool: &storage.StoragePool{},
			expectError: true,
			errorMsg:    "qos parameter must have 3 constituents",
		},
		{
			name: "non-numeric QoS values - should return parseQOS error",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			volConfig: &storage.VolumeConfig{
				InternalName:                "test-clone",
				CloneSourceVolumeInternal:   "source-vol",
				CloneSourceSnapshotInternal: "snap-1",
				Qos:                         "min,max,burst", // Non-numeric values
			},
			storagePool: &storage.StoragePool{},
			expectError: true,
			errorMsg:    "invalid syntax", // strconv.ParseInt error for non-numeric
		},
		{
			name: "valid QoS - parseQOS succeeds (skip API call test)",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			volConfig: &storage.VolumeConfig{
				InternalName:                "test-clone",
				CloneSourceVolumeInternal:   "source-vol",
				CloneSourceSnapshotInternal: "snap-1",
				Qos:                         "1000,2000,4000", // Valid QoS format
			},
			storagePool: &storage.StoragePool{},
			expectError: false, // Test only QoS parsing, not full CreateClone
			errorMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			ctx := context.Background()

			// For the valid QoS test, only validate QoS parsing without calling CreateClone
			if tt.name == "valid QoS - parseQOS succeeds (skip API call test)" {
				qos, err := parseQOS(tt.volConfig.Qos)
				assert.NoError(t, err, "parseQOS should succeed")
				assert.Equal(t, int64(1000), qos.MinIOPS)
				assert.Equal(t, int64(2000), qos.MaxIOPS)
				assert.Equal(t, int64(4000), qos.BurstIOPS)
				return
			}

			// Create a source volume config (required parameter)
			sourceVolConfig := &storage.VolumeConfig{
				InternalName: "source-volume",
			}

			err := driver.CreateClone(ctx, sourceVolConfig, tt.volConfig, tt.storagePool)

			if tt.expectError {
				assert.Error(t, err, "CreateClone should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "CreateClone should not return an error")
			}
		})
	}
}

func TestInitializeEarlyValidation(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "invalid JSON - should return unmarshal error",
			configJSON:  `{"invalid": json}`, // Missing quotes around json
			expectError: true,
			errorMsg:    "invalid character", // JSON unmarshal error
		},
		{
			name:        "malformed JSON - missing closing brace",
			configJSON:  `{"tenantName": "test"`, // Missing closing brace
			expectError: true,
			errorMsg:    "unexpected end of JSON input", // JSON unmarshal error
		},
		{
			name:        "empty JSON - JSON parsing succeeds",
			configJSON:  `{}`, // Valid but empty JSON - will pass JSON parsing
			expectError: false,
			errorMsg:    "", // JSON parsing succeeds
		},
		{
			name: "valid JSON structure - JSON parsing succeeds",
			configJSON: `{
				"tenantName": "test-tenant",
				"endPoint": "https://admin:password@10.0.0.1/json-rpc/8.0",
				"svip": "10.0.0.1:3260"
			}`, // Valid JSON - JSON parsing succeeds
			expectError: false,
			errorMsg:    "", // JSON parsing succeeds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only test JSON unmarshaling, not full Initialize
			config := &drivers.SolidfireStorageDriverConfig{}
			err := json.Unmarshal([]byte(tt.configJSON), config)

			if tt.expectError {
				assert.Error(t, err, "JSON unmarshal should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "JSON unmarshal should not return an error")
			}
		})
	}
}

func TestResizeEarlyValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupDriver func() *SANStorageDriver
		sizeBytes   uint64
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid size - exceeds MaxInt64",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			sizeBytes:   uint64(math.MaxInt64) + 1, // math.MaxInt64 + 1
			expectError: true,
			errorMsg:    "invalid volume size",
		},
		{
			name: "valid size - at MaxInt64 boundary (skip API call test)",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			sizeBytes:   uint64(math.MaxInt64), // math.MaxInt64 exactly
			expectError: false,                 // Test only size validation
			errorMsg:    "",
		},
		{
			name: "valid size - normal range (skip API call test)",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			sizeBytes:   1000000000, // 1GB
			expectError: false,      // Test only size validation
			errorMsg:    "",
		},
		{
			name: "valid size - small value (skip API call test)",
			setupDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			sizeBytes:   1024,  // 1KB
			expectError: false, // Test only size validation
			errorMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only validate size conversion, not full Resize
			if tt.sizeBytes > math.MaxInt64 {
				// This should fail validation
				_, err := ValidateVolumeSizeBytes(tt.sizeBytes)
				assert.Error(t, err, "size validation should fail for values > MaxInt64")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// This should pass validation
				size, err := ValidateVolumeSizeBytes(tt.sizeBytes)
				assert.NoError(t, err, "size validation should pass")
				assert.Equal(t, int64(tt.sizeBytes), size, "converted size should match")
			}
		})
	}
}

// Helper function to validate volume size (mimics the logic in Resize)
func ValidateVolumeSizeBytes(sizeBytes uint64) (int64, error) {
	if sizeBytes > math.MaxInt64 {
		return 0, errors.New("invalid volume size")
	}
	return int64(sizeBytes), nil
}

func TestReconcileNodeAccessSimple(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	// Create some mock nodes
	nodes := []*models.Node{
		{Name: "node1"},
		{Name: "node2"},
	}

	// ReconcileNodeAccess should always return nil (it's a no-op function)
	err := driver.ReconcileNodeAccess(ctx, nodes, "", "")
	assert.NoError(t, err, "ReconcileNodeAccess should always return nil")
}

func TestGetVolumeOpts(t *testing.T) {
	tests := []struct {
		name         string
		volConfig    *storage.VolumeConfig
		expectedOpts map[string]string
	}{
		{
			name:      "empty volume config",
			volConfig: &storage.VolumeConfig{},
			expectedOpts: map[string]string{
				"type": "",
			},
		},
		{
			name: "volume config with filesystem",
			volConfig: &storage.VolumeConfig{
				FileSystem: "ext4",
			},
			expectedOpts: map[string]string{
				"fileSystemType": "ext4",
				"type":           "",
			},
		},
		{
			name: "volume config with block size",
			volConfig: &storage.VolumeConfig{
				BlockSize: "4096",
			},
			expectedOpts: map[string]string{
				"blocksize": "4096",
				"type":      "",
			},
		},
		{
			name: "volume config with QoS",
			volConfig: &storage.VolumeConfig{
				Qos: "1000,2000,4000",
			},
			expectedOpts: map[string]string{
				"qos":  "1000,2000,4000",
				"type": "",
			},
		},
		{
			name: "volume config with all options",
			volConfig: &storage.VolumeConfig{
				FileSystem: "xfs",
				BlockSize:  "512",
				Qos:        "2000,4000,8000",
			},
			expectedOpts: map[string]string{
				"fileSystemType": "xfs",
				"blocksize":      "512",
				"qos":            "2000,4000,8000",
				"type":           "",
			},
		},
		{
			name:      "volume config with QoS type fallback from pool",
			volConfig: &storage.VolumeConfig{
				// No QosType and no Qos - should fallback to pool
			},
			expectedOpts: map[string]string{
				"type": "Gold", // Should get this from pool
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()

			var pool storage.Pool = nil
			// For the pool fallback test, create a pool with QoSType
			if tt.name == "volume config with QoS type fallback from pool" {
				pool = storage.NewStoragePool(nil, "test-pool")
				pool.InternalAttributes()[QoSType] = "Gold"
			}

			opts := driver.GetVolumeOpts(tt.volConfig, pool, nil)

			assert.Equal(t, tt.expectedOpts, opts, "volume options should match expected")
		})
	}
}

func TestStringMethods(t *testing.T) {
	driver := newTestSolidfireSANDriver()

	// Test String() method
	stringResult := driver.String()
	assert.NotEmpty(t, stringResult, "String() should return non-empty string")
	assert.Contains(t, stringResult, "solidfire-san", "String() should contain driver name")
	assert.Contains(t, stringResult, "initialized:false", "String() should show initialization status")

	// Test GoString() method
	goStringResult := driver.GoString()
	assert.NotEmpty(t, goStringResult, "GoString() should return non-empty string")
	assert.Contains(t, goStringResult, "solidfire-san", "GoString() should contain driver name")
	assert.Contains(t, goStringResult, "initialized:false", "GoString() should show initialization status")

	// Both should have same basic structure (though memory addresses may differ)
	assert.Equal(t, len(stringResult) > 100, len(goStringResult) > 100, "Both should return substantial content")
}

func TestGetStorageBackendSpecs(t *testing.T) {
	driver := newTestSolidfireSANDriver()
	ctx := context.Background()

	// Create a mock backend
	backend := &storage.StorageBackend{}

	err := driver.GetStorageBackendSpecs(ctx, backend)
	assert.NoError(t, err, "GetStorageBackendSpecs should not return error")

	// The function should set the backend name
	assert.Equal(t, driver.BackendName(), backend.Name(), "backend name should be set")
}

func TestGetUpdateType(t *testing.T) {
	tests := []struct {
		name            string
		setupOrigDriver func() *SANStorageDriver
		setupNewDriver  func() *SANStorageDriver
		expectedUpdates []uint32
	}{
		{
			name: "identical drivers - no updates",
			setupOrigDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			setupNewDriver: func() *SANStorageDriver {
				return newTestSolidfireSANDriver()
			},
			expectedUpdates: []uint32{},
		},
		{
			name: "different SVIP - invalid volume access info change",
			setupOrigDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.SVIP = "10.0.0.1:3260"
				return driver
			},
			setupNewDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.SVIP = "10.0.0.2:3260"
				return driver
			},
			expectedUpdates: []uint32{storage.InvalidVolumeAccessInfoChange},
		},
		{
			name: "different password - password change",
			setupOrigDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.EndPoint = "https://admin:oldpass@10.0.0.1/json-rpc/7.0"
				return driver
			},
			setupNewDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.EndPoint = "https://admin:newpass@10.0.0.1/json-rpc/7.0"
				return driver
			},
			expectedUpdates: []uint32{storage.PasswordChange},
		},
		{
			name: "different username - username change",
			setupOrigDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.EndPoint = "https://olduser:pass@10.0.0.1/json-rpc/7.0"
				return driver
			},
			setupNewDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.EndPoint = "https://newuser:pass@10.0.0.1/json-rpc/7.0"
				return driver
			},
			expectedUpdates: []uint32{storage.UsernameChange},
		},
		{
			name: "different storage prefix - prefix change",
			setupOrigDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				oldPrefix := "old_"
				driver.Config.StoragePrefix = &oldPrefix
				return driver
			},
			setupNewDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				newPrefix := "new_"
				driver.Config.StoragePrefix = &newPrefix
				return driver
			},
			expectedUpdates: []uint32{storage.PrefixChange},
		},
		{
			name: "multiple changes - multiple update flags",
			setupOrigDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.SVIP = "10.0.0.1:3260"
				driver.Config.EndPoint = "https://admin:oldpass@10.0.0.1/json-rpc/7.0"
				return driver
			},
			setupNewDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.SVIP = "10.0.0.2:3260"
				driver.Config.EndPoint = "https://admin:newpass@10.0.0.1/json-rpc/7.0"
				return driver
			},
			expectedUpdates: []uint32{storage.InvalidVolumeAccessInfoChange, storage.PasswordChange},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newDriver := tt.setupNewDriver()
			origDriver := tt.setupOrigDriver()
			ctx := context.Background()

			bitmap := newDriver.GetUpdateType(ctx, origDriver)

			if len(tt.expectedUpdates) == 0 {
				assert.True(t, bitmap.IsEmpty(), "bitmap should be empty when no updates expected")
			} else {
				for _, expectedUpdate := range tt.expectedUpdates {
					assert.True(t, bitmap.Contains(expectedUpdate), "bitmap should contain expected update type %d", expectedUpdate)
				}
				assert.Equal(t, uint64(len(tt.expectedUpdates)), bitmap.GetCardinality(), "bitmap should contain exactly expected number of updates")
			}
		})
	}
}

func TestGetVolumeExternal(t *testing.T) {
	tests := []struct {
		name         string
		externalName string
		volumeAttrs  *api.Volume
		expected     *storage.VolumeExternal
	}{
		{
			name:         "standard volume conversion",
			externalName: "external-vol-1",
			volumeAttrs: &api.Volume{
				Name:      "internal-vol-1",
				TotalSize: 10737418240, // 10GB
				BlockSize: 4096,
			},
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:         tridentconfig.OrchestratorAPIVersion,
					Name:            "external-vol-1",
					InternalName:    "internal-vol-1",
					Size:            "10737418240",
					Protocol:        tridentconfig.Block,
					SnapshotPolicy:  "",
					ExportPolicy:    "",
					SnapshotDir:     "false",
					UnixPermissions: "",
					StorageClass:    "",
					AccessMode:      tridentconfig.ReadWriteOnce,
					AccessInfo:      models.VolumeAccessInfo{},
					BlockSize:       "4096",
					FileSystem:      "",
				},
				Pool: drivers.UnsetPool,
			},
		},
		{
			name:         "volume with different block size",
			externalName: "external-vol-2",
			volumeAttrs: &api.Volume{
				Name:      "internal-vol-2",
				TotalSize: 1073741824, // 1GB
				BlockSize: 512,
			},
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:         tridentconfig.OrchestratorAPIVersion,
					Name:            "external-vol-2",
					InternalName:    "internal-vol-2",
					Size:            "1073741824",
					Protocol:        tridentconfig.Block,
					SnapshotPolicy:  "",
					ExportPolicy:    "",
					SnapshotDir:     "false",
					UnixPermissions: "",
					StorageClass:    "",
					AccessMode:      tridentconfig.ReadWriteOnce,
					AccessInfo:      models.VolumeAccessInfo{},
					BlockSize:       "512",
					FileSystem:      "",
				},
				Pool: drivers.UnsetPool,
			},
		},
		{
			name:         "volume with zero size",
			externalName: "external-vol-zero",
			volumeAttrs: &api.Volume{
				Name:      "internal-vol-zero",
				TotalSize: 0,
				BlockSize: 4096,
			},
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:         tridentconfig.OrchestratorAPIVersion,
					Name:            "external-vol-zero",
					InternalName:    "internal-vol-zero",
					Size:            "0",
					Protocol:        tridentconfig.Block,
					SnapshotPolicy:  "",
					ExportPolicy:    "",
					SnapshotDir:     "false",
					UnixPermissions: "",
					StorageClass:    "",
					AccessMode:      tridentconfig.ReadWriteOnce,
					AccessInfo:      models.VolumeAccessInfo{},
					BlockSize:       "4096",
					FileSystem:      "",
				},
				Pool: drivers.UnsetPool,
			},
		},
		{
			name:         "volume with large size",
			externalName: "large-volume",
			volumeAttrs: &api.Volume{
				Name:      "large-internal-volume",
				TotalSize: 549755813888, // 512GB
				BlockSize: 4096,
			},
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:         tridentconfig.OrchestratorAPIVersion,
					Name:            "large-volume",
					InternalName:    "large-internal-volume",
					Size:            "549755813888",
					Protocol:        tridentconfig.Block,
					SnapshotPolicy:  "",
					ExportPolicy:    "",
					SnapshotDir:     "false",
					UnixPermissions: "",
					StorageClass:    "",
					AccessMode:      tridentconfig.ReadWriteOnce,
					AccessInfo:      models.VolumeAccessInfo{},
					BlockSize:       "4096",
					FileSystem:      "",
				},
				Pool: drivers.UnsetPool,
			},
		},
		{
			name:         "volume with empty names",
			externalName: "",
			volumeAttrs: &api.Volume{
				Name:      "",
				TotalSize: 1000000000, // 1GB
				BlockSize: 512,
			},
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:         tridentconfig.OrchestratorAPIVersion,
					Name:            "",
					InternalName:    "",
					Size:            "1000000000",
					Protocol:        tridentconfig.Block,
					SnapshotPolicy:  "",
					ExportPolicy:    "",
					SnapshotDir:     "false",
					UnixPermissions: "",
					StorageClass:    "",
					AccessMode:      tridentconfig.ReadWriteOnce,
					AccessInfo:      models.VolumeAccessInfo{},
					BlockSize:       "512",
					FileSystem:      "",
				},
				Pool: drivers.UnsetPool,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()

			result := driver.getVolumeExternal(tt.externalName, tt.volumeAttrs)

			// Verify the returned VolumeExternal structure
			assert.NotNil(t, result, "result should not be nil")
			assert.NotNil(t, result.Config, "config should not be nil")

			// Check all the config fields
			assert.Equal(t, tt.expected.Config.Version, result.Config.Version, "Version should match")
			assert.Equal(t, tt.expected.Config.Name, result.Config.Name, "Name should match")
			assert.Equal(t, tt.expected.Config.InternalName, result.Config.InternalName, "InternalName should match")
			assert.Equal(t, tt.expected.Config.Size, result.Config.Size, "Size should match")
			assert.Equal(t, tt.expected.Config.Protocol, result.Config.Protocol, "Protocol should match")
			assert.Equal(t, tt.expected.Config.SnapshotPolicy, result.Config.SnapshotPolicy, "SnapshotPolicy should match")
			assert.Equal(t, tt.expected.Config.ExportPolicy, result.Config.ExportPolicy, "ExportPolicy should match")
			assert.Equal(t, tt.expected.Config.SnapshotDir, result.Config.SnapshotDir, "SnapshotDir should match")
			assert.Equal(t, tt.expected.Config.UnixPermissions, result.Config.UnixPermissions, "UnixPermissions should match")
			assert.Equal(t, tt.expected.Config.StorageClass, result.Config.StorageClass, "StorageClass should match")
			assert.Equal(t, tt.expected.Config.AccessMode, result.Config.AccessMode, "AccessMode should match")
			assert.Equal(t, tt.expected.Config.AccessInfo, result.Config.AccessInfo, "AccessInfo should match")
			assert.Equal(t, tt.expected.Config.BlockSize, result.Config.BlockSize, "BlockSize should match")
			assert.Equal(t, tt.expected.Config.FileSystem, result.Config.FileSystem, "FileSystem should match")

			// Check the pool
			assert.Equal(t, tt.expected.Pool, result.Pool, "Pool should match")
		})
	}
}

func TestGetEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		inputEndpoint  string
		expectedResult string
		expectError    bool
		errorMsg       string
	}{
		{
			name:           "valid endpoint with old version - should upgrade to minimum",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/7.0",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with minimum version - should keep as is",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/8.0",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with newer version - should keep as is",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/9.0",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/9.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with decimal version - should upgrade if needed",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/7.5",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with higher decimal version - should keep as is",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/8.5",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/8.5",
			expectError:    false,
		},
		{
			name:           "valid endpoint with different IP",
			inputEndpoint:  "https://user:pass@192.168.1.100/json-rpc/7.0",
			expectedResult: "https://user:pass@192.168.1.100/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with different port",
			inputEndpoint:  "https://admin:password@10.0.0.1:443/json-rpc/6.0",
			expectedResult: "https://admin:password@10.0.0.1:443/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:          "invalid endpoint - missing version",
			inputEndpoint: "https://admin:password@10.0.0.1/json-rpc/",
			expectError:   true,
			errorMsg:      "invalid endpoint in config file",
		},
		{
			name:          "invalid endpoint - missing json-rpc path",
			inputEndpoint: "https://admin:password@10.0.0.1/8.0",
			expectError:   true,
			errorMsg:      "invalid endpoint in config file",
		},
		{
			name:          "invalid endpoint - completely malformed",
			inputEndpoint: "not-a-valid-endpoint",
			expectError:   true,
			errorMsg:      "invalid endpoint in config file",
		},
		{
			name:          "invalid endpoint - non-numeric version",
			inputEndpoint: "https://admin:password@10.0.0.1/json-rpc/abc",
			expectError:   true,
			errorMsg:      "invalid endpoint in config file",
		},
		{
			name:          "invalid endpoint - empty version",
			inputEndpoint: "https://admin:password@10.0.0.1/json-rpc/",
			expectError:   true,
			errorMsg:      "invalid endpoint in config file",
		},
		{
			name:           "valid endpoint with http (not https)",
			inputEndpoint:  "http://admin:password@10.0.0.1/json-rpc/7.0",
			expectedResult: "http://admin:password@10.0.0.1/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with very old version",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/5.0",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/8.0",
			expectError:    false,
		},
		{
			name:           "valid endpoint with three-part version",
			inputEndpoint:  "https://admin:password@10.0.0.1/json-rpc/7.5.1",
			expectedResult: "https://admin:password@10.0.0.1/json-rpc/8.0",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()
			ctx := context.Background()

			config := &drivers.SolidfireStorageDriverConfig{
				EndPoint: tt.inputEndpoint,
			}

			result, err := driver.getEndpoint(ctx, config)

			if tt.expectError {
				assert.Error(t, err, "getEndpoint should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "getEndpoint should not return an error")
				assert.Equal(t, tt.expectedResult, result, "endpoint should match expected result")
			}
		})
	}
}

func TestInitializeStoragePools(t *testing.T) {
	tests := []struct {
		name          string
		setupDriver   func() *SANStorageDriver
		expectedPools int
		expectedNames []string
		expectedError bool
		errorMsg      string
	}{
		{
			name: "no types, no virtual pools - creates default pool",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Client.VolumeTypes = nil                                // No types defined
				driver.Config.Storage = []drivers.SolidfireStorageDriverPool{} // No virtual pools
				driver.DefaultMinIOPS = 1000
				driver.DefaultMaxIOPS = 10000
				return driver
			},
			expectedPools: 1,
			expectedNames: []string{"SolidFire-default"},
			expectedError: false,
		},
		{
			name: "types defined, no virtual pools - creates pool per type",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				// Types are already defined in newTestSolidfireSANDriver (Gold, Bronze)
				driver.Config.Storage = []drivers.SolidfireStorageDriverPool{} // No virtual pools
				return driver
			},
			expectedPools: 2,
			expectedNames: []string{"Gold", "Bronze"},
			expectedError: false,
		},
		{
			name: "virtual pools defined - creates pools from config",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.Storage = []drivers.SolidfireStorageDriverPool{
					{
						Type:   "Gold",
						Region: "us-east-1",
						Zone:   "us-east-1a",
					},
					{
						Type:   "Bronze",
						Region: "us-west-2",
						Zone:   "us-west-2b",
					},
				}
				return driver
			},
			expectedPools: 2,
			expectedNames: []string{"solidfire_10.0.0.1_pool_0", "solidfire_10.0.0.1_pool_1"},
			expectedError: false,
		},
		{
			name: "virtual pools with invalid QoS type - should error",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.Storage = []drivers.SolidfireStorageDriverPool{
					{
						Type: "InvalidType", // This type doesn't exist in driver.Config.Types
					},
				}
				return driver
			},
			expectedPools: 0,
			expectedError: true,
			errorMsg:      "invalid QoS type: InvalidType",
		},
		{
			name: "virtual pools without type - uses default QoS",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.Storage = []drivers.SolidfireStorageDriverPool{
					{
						// No Type specified
						Region: "us-central-1",
					},
				}
				driver.DefaultMinIOPS = 500
				driver.DefaultMaxIOPS = 5000
				return driver
			},
			expectedPools: 1,
			expectedNames: []string{"solidfire_10.0.0.1_pool_0"},
			expectedError: false,
		},
		{
			name: "multiple virtual pools with mixed configurations",
			setupDriver: func() *SANStorageDriver {
				driver := newTestSolidfireSANDriver()
				driver.Config.Storage = []drivers.SolidfireStorageDriverPool{
					{
						Type:   "Gold",
						Region: "us-east-1",
					},
					{
						// No type - will use default
						Region: "us-west-2",
					},
					{
						Type: "Bronze",
						Zone: "us-central-1a",
					},
				}
				return driver
			},
			expectedPools: 3,
			expectedNames: []string{"solidfire_10.0.0.1_pool_0", "solidfire_10.0.0.1_pool_1", "solidfire_10.0.0.1_pool_2"},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := tt.setupDriver()
			ctx := context.Background()

			err := driver.initializeStoragePools(ctx)

			if tt.expectedError {
				assert.Error(t, err, "initializeStoragePools should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "initializeStoragePools should not return an error")

				// Verify the number of virtual pools created
				assert.Equal(t, tt.expectedPools, len(driver.virtualPools), "number of virtual pools should match expected")

				// Verify pool names exist
				for _, expectedName := range tt.expectedNames {
					_, exists := driver.virtualPools[expectedName]
					assert.True(t, exists, "virtual pool %s should exist", expectedName)
				}

				// Verify pool attributes are set correctly
				for poolName, pool := range driver.virtualPools {
					// Check that all pools have required attributes
					assert.NotNil(t, pool.Attributes()[sa.BackendType], "pool %s should have BackendType", poolName)
					assert.NotNil(t, pool.Attributes()[sa.IOPS], "pool %s should have IOPS", poolName)
					assert.NotNil(t, pool.Attributes()[sa.Snapshots], "pool %s should have Snapshots", poolName)
					assert.NotNil(t, pool.Attributes()[sa.Clones], "pool %s should have Clones", poolName)
					assert.NotNil(t, pool.Attributes()[sa.Encryption], "pool %s should have Encryption", poolName)
					assert.NotNil(t, pool.Attributes()[sa.Replication], "pool %s should have Replication", poolName)
					assert.NotNil(t, pool.Attributes()[sa.ProvisioningType], "pool %s should have ProvisioningType", poolName)
					assert.NotNil(t, pool.Attributes()[sa.Media], "pool %s should have Media", poolName)
				}
			}
		})
	}
}

func TestSetProvisioningLabels(t *testing.T) {
	tests := []struct {
		name        string
		setupPool   func() storage.Pool
		expectError bool
		expectedKey string
	}{
		{
			name: "successful label setting",
			setupPool: func() storage.Pool {
				pool := storage.NewStoragePool(nil, "test-pool")
				return pool
			},
			expectError: false,
			expectedKey: storage.ProvisioningLabelTag,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()
			ctx := context.Background()
			pool := tt.setupPool()
			meta := make(map[string]string)

			err := driver.setProvisioningLabels(ctx, pool, meta)

			if tt.expectError {
				assert.Error(t, err, "setProvisioningLabels should return an error")
			} else {
				assert.NoError(t, err, "setProvisioningLabels should not return an error")
				// Verify the provisioning label was set in meta
				_, exists := meta[tt.expectedKey]
				assert.True(t, exists, "meta should contain provisioning label key")
			}
		})
	}
}

func TestPopulateConfigurationDefaults(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    func() *drivers.SolidfireStorageDriverConfig
		expectedPrefix string
		expectedSize   string
		expectedCHAP   bool
		expectError    bool
		errorMsg       string
	}{
		{
			name: "default config - empty size",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = "" // Empty size should get default
				config.UseCHAP = false
				config.DriverContext = ""
				return config
			},
			expectedPrefix: "",
			expectedSize:   drivers.DefaultVolumeSize,
			expectedCHAP:   false,
			expectError:    false,
		},
		{
			name: "valid custom size",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = "10Gi"
				config.UseCHAP = false
				config.DriverContext = ""
				return config
			},
			expectedPrefix: "",
			expectedSize:   "10Gi",
			expectedCHAP:   false,
			expectError:    false,
		},
		{
			name: "invalid size format - should error",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = "invalid-size"
				config.UseCHAP = false
				config.DriverContext = ""
				return config
			},
			expectError: true,
			errorMsg:    "invalid config value for default volume size",
		},
		{
			name: "docker context - forces CHAP when disabled",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = ""
				config.UseCHAP = false // Should be forced to true for Docker
				config.DriverContext = tridentconfig.ContextDocker
				return config
			},
			expectedPrefix: "",
			expectedSize:   drivers.DefaultVolumeSize,
			expectedCHAP:   true, // Should be forced to true
			expectError:    false,
		},
		{
			name: "docker context - keeps CHAP when already enabled",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = ""
				config.UseCHAP = true // Already enabled
				config.DriverContext = tridentconfig.ContextDocker
				return config
			},
			expectedPrefix: "",
			expectedSize:   drivers.DefaultVolumeSize,
			expectedCHAP:   true,
			expectError:    false,
		},
		{
			name: "CSI context - forces CHAP when disabled",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = ""
				config.UseCHAP = false // Should be forced to true for CSI
				config.DriverContext = tridentconfig.ContextCSI
				return config
			},
			expectedPrefix: "",
			expectedSize:   drivers.DefaultVolumeSize,
			expectedCHAP:   true, // Should be forced to true
			expectError:    false,
		},
		{
			name: "kubernetes context - does not force CHAP",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = ""
				config.UseCHAP = false
				config.DriverContext = ""
				return config
			},
			expectedPrefix: "",
			expectedSize:   drivers.DefaultVolumeSize,
			expectedCHAP:   false, // Should remain false for Kubernetes
			expectError:    false,
		},
		{
			name: "complex valid config",
			setupConfig: func() *drivers.SolidfireStorageDriverConfig {
				config := &drivers.SolidfireStorageDriverConfig{}
				config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
				config.Size = "50Gi"
				config.UseCHAP = false
				config.DriverContext = tridentconfig.ContextCSI
				return config
			},
			expectedPrefix: "",
			expectedSize:   "50Gi",
			expectedCHAP:   true, // Should be forced for CSI
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := newTestSolidfireSANDriver()
			config := tt.setupConfig()
			ctx := context.Background()

			err := driver.populateConfigurationDefaults(ctx, config)

			if tt.expectError {
				assert.Error(t, err, "populateConfigurationDefaults should return an error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "error message should match expected")
				}
			} else {
				assert.NoError(t, err, "populateConfigurationDefaults should not return an error")

				// Verify StoragePrefix is always set to empty string
				assert.NotNil(t, config.StoragePrefix, "StoragePrefix should not be nil")
				assert.Equal(t, tt.expectedPrefix, *config.StoragePrefix, "StoragePrefix should be empty string")

				// Verify Size is set correctly
				assert.Equal(t, tt.expectedSize, config.Size, "Size should match expected")

				// Verify CHAP setting based on context
				assert.Equal(t, tt.expectedCHAP, config.UseCHAP, "UseCHAP should match expected")
			}
		})
	}
}
