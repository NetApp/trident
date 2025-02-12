// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	sa "github.com/netapp/trident/storage_attribute"
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
