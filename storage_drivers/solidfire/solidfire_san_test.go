// Copyright 2020 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/solidfire/api"
)

const (
	TenantName = "tester"
	AdminPass  = "admin:password"
	Endpoint   = "https://" + AdminPass + "@10.0.0.1/json-rpc/7.0"
)

func newTestSolidfireSANDriver(showSensitive *bool) *SANStorageDriver {
	config := &drivers.SolidfireStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	if showSensitive != nil {
		config.CommonStorageDriverConfig.DebugTraceFlags["sensitive"] = *showSensitive
	}

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

func TestSolidfireSANStorageDriverConfigString(t *testing.T) {

	var solidfireSANDrivers = []SANStorageDriver{
		*newTestSolidfireSANDriver(&[]bool{true}[0]),
		*newTestSolidfireSANDriver(&[]bool{false}[0]),
		*newTestSolidfireSANDriver(nil),
	}

	for _, solidfireSANDriver := range solidfireSANDrivers {
		sensitive, ok := solidfireSANDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok:
			assert.Contains(t, solidfireSANDriver.String(), "<REDACTED>",
				"Solidfire driver does not contain <REDACTED>")
			assert.Contains(t, solidfireSANDriver.String(), "Client:<REDACTED>",
				"Solidfire driver does not redact client API information")
			assert.Contains(t, solidfireSANDriver.String(), "AccountID:<REDACTED>",
				"Solidfire driver does not redact Account ID information")
			assert.NotContains(t, solidfireSANDriver.String(), TenantName,
				"Solidfire driver contains tenant name")
			assert.NotContains(t, solidfireSANDriver.String(), AdminPass,
				"Solidfire driver contains endpoint's admin and password")
			assert.NotContains(t, solidfireSANDriver.String(), "2222",
				"Solidfire driver contains Account ID")
		case ok && sensitive:
			assert.Contains(t, solidfireSANDriver.String(), TenantName,
				"Solidfire driver does not contain tenant name")
			assert.Contains(t, solidfireSANDriver.String(), AdminPass,
				"Solidfire driver does not contain endpoint's admin and password")
			assert.Contains(t, solidfireSANDriver.String(), "2222",
				"Solidfire driver does not contain Account ID")
		case ok && !sensitive:
			assert.Contains(t, solidfireSANDriver.String(), "<REDACTED>",
				"Solidfire driver does not contain <REDACTED>")
			assert.Contains(t, solidfireSANDriver.String(), "Client:<REDACTED>",
				"Solidfire driver does not redact client API information")
			assert.Contains(t, solidfireSANDriver.String(), "AccountID:<REDACTED>",
				"Solidfire driver does not redact Account ID information")
			assert.NotContains(t, solidfireSANDriver.String(), TenantName,
				"Solidfire driver contains tenant name")
			assert.NotContains(t, solidfireSANDriver.String(), AdminPass,
				"Solidfire driver contains endpoint's admin and password")
			assert.NotContains(t, solidfireSANDriver.String(), "2222",
				"Solidfire driver contains Account ID")
		}
	}
}
