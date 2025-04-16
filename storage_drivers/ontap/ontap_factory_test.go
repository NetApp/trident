// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
)

func TestGetDriverProtocol(t *testing.T) {
	driverName := config.OntapSANStorageDriverName

	// Unmarshal error
	_, err := GetDriverProtocol(driverName, `{"SANType"}`)
	assert.ErrorContains(t, err, "failed to get pool values")

	// SANType as NVMe
	SANType, err := GetDriverProtocol(driverName, `{"SANType": "nvme"}`)
	assert.Equal(t, SANType, sa.NVMe, "Incorrect protocol type.")
	assert.NoError(t, err, "Failed to get protocol type.")

	// SANType as FCP
	SANType, err = GetDriverProtocol(driverName, `{"SANType": "fcp"}`)
	assert.Equal(t, SANType, sa.FCP, "Incorrect protocol type.")
	assert.NoError(t, err, "Failed to get protocol type.")

	// Empty SANType
	SANType, err = GetDriverProtocol(driverName, `{}`)
	assert.Equal(t, sa.ISCSI, SANType, "Incorrect protocol type.")
	assert.NoError(t, err, "Failed to get protocol type.")

	// Incorrect SANType
	SANType, err = GetDriverProtocol(driverName, `{"SANType": "invalid"}`)
	assert.ErrorContains(t, err, "unsupported SAN protocol")

	// Different driver returns no protocol
	_, err = GetDriverProtocol(config.OntapNASStorageDriverName, `{}`)
	assert.NoError(t, err, "Failed to get protocol type.")
}

func TestGetSANStorageDriverBasedOnPersonality(t *testing.T) {
	ontapConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
	}
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)

	tests := []struct {
		sanOptimized       bool
		disaggregated      bool
		driverProtocol     string
		expectedDriverType interface{}
		expectError        bool
	}{
		{true, true, sa.ISCSI, &ASAStorageDriver{}, false},
		{true, false, sa.ISCSI, &SANStorageDriver{}, false},
		{false, false, sa.ISCSI, &SANStorageDriver{}, false},
		{false, false, sa.NVMe, &NVMeStorageDriver{}, false},
		{false, false, sa.FCP, &SANStorageDriver{}, false},
		{false, false, "unsupported", nil, true},
	}

	for _, test := range tests {
		driver, err := getSANStorageDriverBasedOnPersonality(
			test.sanOptimized, test.disaggregated, test.driverProtocol, &ontapConfig, mockAPI, mockAWSAPI,
		)

		if test.expectError {
			assert.Error(t, err, "expected error but got none")
		} else {
			assert.NoError(t, err, "expected no error but got one")
			assert.IsType(t, test.expectedDriverType, driver, "storage driver type does not match")
		}
	}
}
