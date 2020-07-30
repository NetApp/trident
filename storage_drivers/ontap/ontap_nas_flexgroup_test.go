// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newTestOntapNASFGDriver(showSensitive *bool) *NASFlexGroupStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	if showSensitive != nil {
		config.CommonStorageDriverConfig.DebugTraceFlags["sensitive"] = *showSensitive
	}

	config.ManagementLIF = "127.0.0.1"
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "ontap-nas-fg-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas-flexgroup"
	config.StoragePrefix = sp("test_")

	nasfgDriver := &NASFlexGroupStorageDriver{}
	nasfgDriver.Config = *config

	// ClientConfig holds the configuration data for Client objects
	clientConfig := api.ClientConfig{
		ManagementLIF: config.ManagementLIF,
		SVM:					"SVM1",
		Username:               "client_username",
		Password:               "client_password",
		DriverContext:           tridentconfig.DriverContext("driverContext"),
		ContextBasedZapiRecords: 100,
		DebugTraceFlags:         nil,
	}

	nasfgDriver.API = api.NewClient(clientConfig)
	nasfgDriver.Telemetry = &Telemetry{
		Plugin:        nasfgDriver.Name(),
		SVM:           nasfgDriver.GetConfig().SVM,
		StoragePrefix: *nasfgDriver.GetConfig().StoragePrefix,
		Driver:        nasfgDriver,
		done:          make(chan struct{}),
	}

	return nasfgDriver
}

func TestOntapNasFgStorageDriverConfigString(t *testing.T) {

	var ontapNasDrivers = []NASFlexGroupStorageDriver {
		*newTestOntapNASFGDriver(&[]bool{true}[0]),
		*newTestOntapNASFGDriver(&[]bool{false}[0]),
		*newTestOntapNASFGDriver(nil),
	}

	for _, nasfgDriver := range ontapNasDrivers {
		sensitive, ok := nasfgDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok || (ok && !sensitive):
			assert.Contains(t, nasfgDriver.String(), "<REDACTED>",
				"ontap-nas-fg driver did not contain <REDACTED>")
			assert.Contains(t, nasfgDriver.String(), "API:<REDACTED>",
				"ontap-nas-fg driver does not redact client API information")
			assert.Contains(t, nasfgDriver.String(), "Username:<REDACTED>",
				"ontap-nas-fg driver does not redact username")
			assert.NotContains(t, nasfgDriver.String(), "ontap-nas-fg-user",
				"ontap-nas-fgdriver contains username")
			assert.Contains(t, nasfgDriver.String(), "Password:<REDACTED>",
				"ontap-nas-fg driver does not redact password")
			assert.NotContains(t, nasfgDriver.String(), "password1!",
				"ontap-nas-fg driver contains password")
			assert.NotContains(t, nasfgDriver.String(), "client_username",
				"ontap-nas-fg driver contains username")
			assert.NotContains(t, nasfgDriver.String(), "client_password",
				"ontap-nas-fg driver contains password")

		case ok && sensitive:
			assert.Contains(t, nasfgDriver.String(), "ontap-nas-fg-user",
				"ontap-nas-fg driver does not contain username")
			assert.Contains(t, nasfgDriver.String(), "password1!",
				"ontap-nas-fg driver does not contain password")
			assert.Contains(t, nasfgDriver.String(), "client_username",
				"ontap-nas-fg driver contains client_username")
			assert.Contains(t, nasfgDriver.String(), "client_password",
				"ontap-nas-fg driver contains client_password")
		}
	}
}
