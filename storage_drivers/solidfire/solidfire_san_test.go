// Copyright 2020 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/solidfire/api"
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
