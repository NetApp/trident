// Copyright 2022 NetApp, Inc. All Rights Reserved.

package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/mocks/mock_storage"
	drivers "github.com/netapp/trident/storage_drivers"
)

var ctx = context.Background()

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

// TestInitializeRecovery intentionally passes a bogus config to
// NewStorageBackendForConfig to test its ability to recover.
func TestInitializeRecovery(t *testing.T) {
	empty := ""
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-nas",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
		},
		// These should be bogus yet valid connection parameters
		ManagementLIF: "127.0.0.1",
		DataLIF:       "127.0.0.1",
		IgroupName:    "nonexistent",
		SVM:           "nonexistent",
		Username:      "none",
		Password:      "none",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal ONTAP config:  ", err)
	}

	commonConfig, configInJSON, err := ValidateCommonSettings(context.Background(), string(marshaledJSON))
	if err != nil {
		t.Error("Failed to validate settings for configuration.")
	}

	_, err = NewStorageBackendForConfig(context.Background(), configInJSON, "fakeConfigRef", uuid.New().String(),
		commonConfig, nil)
	if err == nil {
		t.Error("Failed to get error for incorrect configuration.")
	}
}

func TestNewStorageBackendForConfig(t *testing.T) {
	backendUUID := uuid.New().String()
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "fake",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
			Credentials: map[string]string{
				"name": "secret1",
				"type": "secret",
			},
		},
		Username: "",
		Password: "",
	}
	marshaledJSON, err := json.Marshal(config)
	assert.Nil(t, err)

	commonConfig, configInJSON, err := ValidateCommonSettings(ctx, string(marshaledJSON))
	assert.Nil(t, err)

	storageBackend, err := NewStorageBackendForConfig(ctx, configInJSON, "", backendUUID, commonConfig,
		nil)
	assert.Nil(t, err)

	assert.Equal(t, storageBackend.Driver().Name(), "fake")
	assert.Equal(t, storageBackend.BackendUUID(), backendUUID)
	assert.Equal(t, storageBackend.ConfigRef(), "")
}

func TestNewStorageBackendForConfig_UnknownDriver(t *testing.T) {
	backendUUID := uuid.New().String()
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "unknown",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
			Credentials: map[string]string{
				"name": "secret1",
				"type": "secret",
			},
		},
		Username: "",
		Password: "",
	}
	marshaledJSON, err := json.Marshal(config)
	assert.Nil(t, err)

	commonConfig, configInJSON, err := ValidateCommonSettings(ctx, string(marshaledJSON))
	assert.Nil(t, err)

	storageBackend, err := NewStorageBackendForConfig(ctx, configInJSON, "", backendUUID, commonConfig,
		nil)
	assert.NotNil(t, err)
	assert.Nil(t, storageBackend)
}

func TestNewStorageBackendForConfig_Panic(t *testing.T) {
	assert.Panics(t, func() { NewStorageBackendForConfig(nil, "", "", "", nil, nil) })
}

func TestCreateNewStorageBackend_WithMockDriver(t *testing.T) {
	dummyBackendName := "dummy-backend"

	// create a mock controller and a mock driver.
	mockCtrl := gomock.NewController(t)
	mockDriver := mock_storage.NewMockDriver(mockCtrl)
	mockDriver.EXPECT().GetStorageBackendSpecs(ctx, gomock.Any()).Return(fmt.Errorf("couldn't get backend specs"))
	mockDriver.EXPECT().BackendName().Return(dummyBackendName)
	mockDriver.EXPECT().Name().Return(dummyBackendName).Times(2)

	stb, err := CreateNewStorageBackend(ctx, mockDriver)

	assert.Error(t, err, "expected error")
	assert.NotNil(t, stb, "expected non-nil storage backend")
	assert.Equal(t, "failed", string(stb.State()), "expected failed backend")
}

func TestGetStorageDriver(t *testing.T) {
	tests := []struct {
		StorageDriverName string
		DriverProtocol    string
		Valid             bool
	}{
		{"fake", "", true},
		{"solidfire-san", "", true},
		{"azure-netapp-files", "", true},
		{"gcp-cvs", "", true},
		{"unknown", "", false},
	}

	for _, test := range tests {
		t.Run(test.StorageDriverName+"-"+test.DriverProtocol, func(t *testing.T) {
			driver, err := GetStorageDriver(test.StorageDriverName)
			if test.Valid {
				assert.Nil(t, err)
				assert.NotNil(t, driver)
			} else {
				assert.NotNil(t, err)
				assert.Nil(t, driver)
			}
		})
	}
}

func TestSpecOnlyValidation(t *testing.T) {
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "fake",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
			Credentials: map[string]string{
				"name": "secret1",
				"type": "secret",
			},
		},
		Username: "",
		Password: "",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal config: ", err)
	}

	commonConfig, configInJSON, err := ValidateCommonSettings(ctx, string(marshaledJSON))
	if err != nil {
		t.Fatal("Failed to validate settings for invalid configuration: ", err)
	}

	err2 := SpecOnlyValidation(ctx, commonConfig, configInJSON)
	assert.Nil(t, err2)
}

func TestSpecOnlyValidation_UnknownDriver(t *testing.T) {
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "unknown",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
			Credentials: map[string]string{
				"name": "secret1",
				"type": "secret",
			},
		},
		Username: "",
		Password: "",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal config: ", err)
	}

	commonConfig, configInJSON, err := ValidateCommonSettings(ctx, string(marshaledJSON))
	if err != nil {
		t.Fatal("Failed to validate settings for invalid configuration: ", err)
	}

	err2 := SpecOnlyValidation(ctx, commonConfig, configInJSON)
	assert.NotNil(t, err2, "Should fail with unknown driver name")
}

func TestSpecOnlyValidation_InvalidYaml(t *testing.T) {
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "fake",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
			Credentials: map[string]string{
				"name": "secret1",
				"type": "secret",
			},
		},
		Username: "",
		Password: "",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal config: ", err)
	}

	commonConfig, _, err := ValidateCommonSettings(ctx, string(marshaledJSON))
	if err != nil {
		t.Fatal("Failed to validate settings for invalid configuration: ", err)
	}

	// Makes the marshalled json invalid
	marshaledJSON = append(marshaledJSON, byte(0))
	configJSONBytes, err := yaml.YAMLToJSON([]byte(marshaledJSON))

	err2 := SpecOnlyValidation(ctx, commonConfig, string(configJSONBytes))
	assert.NotNil(t, err2, "Fails to unmarshal the json config")
}

func TestValidateCommonSettings_InvalidJson(t *testing.T) {
	configJSON := "}"

	_, _, err := ValidateCommonSettings(ctx, configJSON)
	assert.NotNil(t, err, "Invalid JSON")
}

func TestValidateCommonSettings_ValidationFailed(t *testing.T) {
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:          1,
			StoragePrefixRaw: json.RawMessage("{}"),
			StoragePrefix:    &empty,
			Credentials: map[string]string{
				"name": "secret1",
				"type": "secret",
			},
		},
		Username: "",
		Password: "",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal config: ", err)
	}

	_, _, err = ValidateCommonSettings(ctx, string(marshaledJSON))
	assert.NotNil(t, err, "Storage driver name missing")
}

func TestParseBackendName(t *testing.T) {
	tests := []struct {
		name         string
		configJSON   string
		expectedName string
		expectError  bool
	}{
		{
			name:         "ValidJSON_ReturnsBackendName",
			configJSON:   `{"backendName": "test-backend"}`,
			expectedName: "test-backend",
			expectError:  false,
		},
		{
			name:         "InvalidYAML_ReturnsError",
			configJSON:   "invalid-yaml",
			expectedName: "",
			expectError:  true,
		},
		{
			name:         "EmptyJSON_ReturnsError",
			configJSON:   "",
			expectedName: "",
			expectError:  true,
		},
		{
			name:         "MissingBackendName_ReturnsEmpty",
			configJSON:   `{"someOtherField": "value"}`,
			expectedName: "",
			expectError:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backendName, err := ParseBackendName(test.configJSON)
			if test.expectError {
				assert.NotNil(t, err)
				assert.Empty(t, backendName)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, test.expectedName, backendName)
			}
		})
	}
}
