// Copyright 2020 NetApp, Inc. All Rights Reserved.

package eseries

import (
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/eseries/api"
)

const (
	Username      = "tester"
	Password      = "password"
	PasswordArray = "Passwords"
)

func newTestEseriesSANDriver(showSensitive *bool) *SANStorageDriver {
	config := &drivers.ESeriesStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	if showSensitive != nil {
		config.CommonStorageDriverConfig.DebugTraceFlags["sensitive"] = *showSensitive
	}

	config.Username = Username
	config.Password = Password
	config.PasswordArray = PasswordArray
	config.WebProxyHostname = "10.0.0.1"
	config.WebProxyPort = "2222"
	config.WebProxyUseHTTP = false
	config.WebProxyVerifyTLS = false
	config.ControllerA = "10.0.0.2"
	config.ControllerB = "10.0.0.3"
	config.HostDataIP = "10.0.0.4"
	config.StorageDriverName = "eseries-san"
	config.StoragePrefix = sp("test_")

	telemetry := make(map[string]string)
	telemetry["version"] = "20.07.0"
	telemetry["plugin"] = "eseries"
	telemetry["storagePrefix"] = *config.StoragePrefix

	API := api.NewAPIClient(api.ClientConfig{
		WebProxyHostname:      config.WebProxyHostname,
		WebProxyPort:          config.WebProxyPort,
		WebProxyUseHTTP:       config.WebProxyUseHTTP,
		WebProxyVerifyTLS:     config.WebProxyVerifyTLS,
		Username:              config.Username,
		Password:              config.Password,
		ControllerA:           config.ControllerA,
		ControllerB:           config.ControllerB,
		PasswordArray:         config.PasswordArray,
		PoolNameSearchPattern: config.PoolNameSearchPattern,
		HostDataIP:            config.HostDataIP,
		Protocol:              "iscsi",
		AccessGroup:           config.AccessGroup,
		HostType:              config.HostType,
		DriverName:            "eseries-iscsi",
		Telemetry:             telemetry,
	})

	sanDriver := &SANStorageDriver{}
	sanDriver.Config = *config
	sanDriver.API = API

	return sanDriver
}

func TestEseriesSANStorageDriverConfigString(t *testing.T) {

	var EseriesSANDrivers = []SANStorageDriver{
		*newTestEseriesSANDriver(&[]bool{true}[0]),
		*newTestEseriesSANDriver(&[]bool{false}[0]),
		*newTestEseriesSANDriver(nil),
	}

	for _, EseriesSANDriver := range EseriesSANDrivers {
		sensitive, ok := EseriesSANDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok:
			assert.Contains(t, EseriesSANDriver.String(), "<REDACTED>",
				"Eseries driver does not contain <REDACTED>")
			assert.Contains(t, EseriesSANDriver.String(), "API:<REDACTED>",
				"Eseries driver does not redact API information")
			assert.NotContains(t, EseriesSANDriver.String(), Username,
				"Eseries driver contains  username")
			assert.NotContains(t, EseriesSANDriver.String(), Password,
				"Eseries driver contains password")
			assert.NotContains(t, EseriesSANDriver.String(), PasswordArray,
				"Eseries driver contains password array")
		case ok && sensitive:
			assert.Contains(t, EseriesSANDriver.String(), Username,
				"Eseries driver does not contain username")
			assert.Contains(t, EseriesSANDriver.String(), Password,
				"Eseries driver does not contain password")
			assert.Contains(t, EseriesSANDriver.String(), PasswordArray,
				"Eseries driver does not contain password array")
		case ok && !sensitive:
			assert.Contains(t, EseriesSANDriver.String(), "<REDACTED>",
				"Eseries driver does not contain <REDACTED>")
			assert.Contains(t, EseriesSANDriver.String(), "API:<REDACTED>",
				"Eseries driver does not redact API information")
			assert.NotContains(t, EseriesSANDriver.String(), Username,
				"Eseries driver contains  username")
			assert.NotContains(t, EseriesSANDriver.String(), Password,
				"Eseries driver contains password")
			assert.NotContains(t, EseriesSANDriver.String(), PasswordArray,
				"Eseries driver contains password array")
		}
	}
}
