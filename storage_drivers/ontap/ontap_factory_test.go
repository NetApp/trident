// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
)

const (
	invalidJSONConfig     = `{"version":1,invalid}`
	invalidProtocolConfig = `{"SANType": "invalid-protocol"}`
	basicValidConfig      = `{"version":1}`
	apiFailureConfig      = `{"version":1,"storageDriverName":"ontap-nas","managementLIF":"invalid","svm":"test"}`
)

func TestGetDriverProtocol(t *testing.T) {
	tests := []struct {
		name         string
		driverName   string
		configJSON   string
		expected     string
		expectError  bool
		errorMessage string
	}{
		{"Unmarshal error", config.OntapSANStorageDriverName, `{"SANType"}`, "", true, "failed to get pool values"},
		{"SANType as NVMe", config.OntapSANStorageDriverName, `{"SANType": "nvme"}`, sa.NVMe, false, ""},
		{"SANType as FCP", config.OntapSANStorageDriverName, `{"SANType": "fcp"}`, sa.FCP, false, ""},
		{"Empty SANType", config.OntapSANStorageDriverName, `{}`, sa.ISCSI, false, ""},
		{"Invalid SANType", config.OntapSANStorageDriverName, `{"SANType": "invalid"}`, "", true, "unsupported SAN protocol"},
		{"NAS driver no protocol", config.OntapNASStorageDriverName, `{}`, "", false, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			protocol, err := GetDriverProtocol(test.driverName, test.configJSON)
			if test.expectError {
				assert.Error(t, err)
				assert.ErrorContains(t, err, test.errorMessage)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, protocol)
			}
		})
	}
}

func TestGetSANStorageDriverBasedOnPersonality(t *testing.T) {
	ontapConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
	}
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)

	tests := []struct {
		name               string
		sanOptimized       bool
		disaggregated      bool
		driverProtocol     string
		expectedDriverType interface{}
		expectError        bool
	}{
		{
			name:               "ASA optimized and disaggregated with iSCSI",
			sanOptimized:       true,
			disaggregated:      true,
			driverProtocol:     sa.ISCSI,
			expectedDriverType: &ASAStorageDriver{},
			expectError:        false,
		},
		{
			name:               "SAN optimized but not disaggregated with iSCSI",
			sanOptimized:       true,
			disaggregated:      false,
			driverProtocol:     sa.ISCSI,
			expectedDriverType: &SANStorageDriver{},
			expectError:        false,
		},
		{
			name:               "Standard SAN with iSCSI",
			sanOptimized:       false,
			disaggregated:      false,
			driverProtocol:     sa.ISCSI,
			expectedDriverType: &SANStorageDriver{},
			expectError:        false,
		},
		{
			name:               "Standard SAN with NVMe",
			sanOptimized:       false,
			disaggregated:      false,
			driverProtocol:     sa.NVMe,
			expectedDriverType: &NVMeStorageDriver{},
			expectError:        false,
		},
		{
			name:               "Standard SAN with FCP",
			sanOptimized:       false,
			disaggregated:      false,
			driverProtocol:     sa.FCP,
			expectedDriverType: &SANStorageDriver{},
			expectError:        false,
		},
		{
			name:               "Unsupported protocol",
			sanOptimized:       false,
			disaggregated:      false,
			driverProtocol:     "unsupported",
			expectedDriverType: nil,
			expectError:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver, err := getSANStorageDriverBasedOnPersonality(
				test.sanOptimized, test.disaggregated, test.driverProtocol, &ontapConfig, mockAPI, mockAWSAPI,
			)

			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, driver)
			} else {
				assert.NoError(t, err)
				assert.IsType(t, test.expectedDriverType, driver)
			}
		})
	}
}

// TestGetStorageDriver tests the main factory function for creating storage drivers
func TestGetStorageDriver(t *testing.T) {
	tests := []struct {
		name         string
		driverName   string
		configJSON   string
		expectError  bool
		errorMessage string
	}{
		{"Invalid JSON config", config.OntapNASStorageDriverName, invalidJSONConfig, true, "invalid character"},
		{"GetDriverProtocol error", config.OntapSANStorageDriverName, invalidProtocolConfig, true, "unsupported SAN protocol"},
		{"Unsupported driver type", "unsupported-driver", basicValidConfig, true, "unknown storage driver"},
		{"API failure returns empty driver", config.OntapNASStorageDriverName, apiFailureConfig, false, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commonConfig := &drivers.CommonStorageDriverConfig{StorageDriverName: test.driverName}
			driver, err := GetStorageDriver(context.TODO(), test.configJSON, commonConfig, make(map[string]string))

			if test.expectError {
				assert.Error(t, err)
				if test.errorMessage != "" {
					assert.Contains(t, err.Error(), test.errorMessage)
				}
				assert.Nil(t, driver)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, driver)
				validateDriverType(t, test.driverName, driver)
			}
		})
	}
}

func validateDriverType(t *testing.T, driverName string, driver interface{}) {
	switch driverName {
	case config.OntapNASStorageDriverName:
		assert.IsType(t, &NASStorageDriver{}, driver)
	case config.OntapNASFlexGroupStorageDriverName:
		assert.IsType(t, &NASFlexGroupStorageDriver{}, driver)
	case config.OntapNASQtreeStorageDriverName:
		assert.IsType(t, &NASQtreeStorageDriver{}, driver)
	case config.OntapSANStorageDriverName:
		switch driver.(type) {
		case *SANStorageDriver, *ASAStorageDriver, *NVMeStorageDriver:
			// Valid SAN driver type
		default:
			t.Errorf("unexpected SAN driver type: %T", driver)
		}
	case config.OntapSANEconomyStorageDriverName:
		assert.IsType(t, &SANEconomyStorageDriver{}, driver)
	}
}

// TestGetEmptyStorageDriver tests creation of uninitialized storage driver instances
func TestGetEmptyStorageDriver(t *testing.T) {
	tests := []struct {
		name               string
		driverName         string
		driverProtocol     string
		expectedDriverType interface{}
		expectError        bool
	}{
		{"NAS driver", config.OntapNASStorageDriverName, "", &NASStorageDriver{}, false},
		{"NAS FlexGroup driver", config.OntapNASFlexGroupStorageDriverName, "", &NASFlexGroupStorageDriver{}, false},
		{"NAS Qtree driver", config.OntapNASQtreeStorageDriverName, "", &NASQtreeStorageDriver{}, false},
		{"SAN driver with iSCSI protocol", config.OntapSANStorageDriverName, sa.ISCSI, &SANStorageDriver{}, false},
		{"SAN driver with FCP protocol", config.OntapSANStorageDriverName, sa.FCP, &SANStorageDriver{}, false},
		{"SAN driver with NVMe protocol", config.OntapSANStorageDriverName, sa.NVMe, &NVMeStorageDriver{}, false},
		{"SAN Economy driver", config.OntapSANEconomyStorageDriverName, "", &SANEconomyStorageDriver{}, false},
		{"Unknown driver name", "unknown-driver", "", nil, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver, err := getEmptyStorageDriver(test.driverName, test.driverProtocol)

			if test.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown storage driver")
				assert.Nil(t, driver)
			} else {
				assert.NoError(t, err)
				assert.IsType(t, test.expectedDriverType, driver)
			}
		})
	}
}
