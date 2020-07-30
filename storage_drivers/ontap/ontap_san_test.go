// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newTestOntapSANDriver(showSensitive *bool) *SANStorageDriver {
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
	config.Username = "ontap-san-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = sp("test_")

	sanDriver := &SANStorageDriver{}
	sanDriver.Config = *config

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

	sanDriver.API = api.NewClient(clientConfig)
	sanDriver.Telemetry = &Telemetry{
		Plugin:        sanDriver.Name(),
		SVM:           sanDriver.GetConfig().SVM,
		StoragePrefix: *sanDriver.GetConfig().StoragePrefix,
		Driver:        sanDriver,
		done:          make(chan struct{}),
	}

	return sanDriver
}

func TestOntapSanStorageDriverConfigString(t *testing.T) {

	var ontapsanDrivers = []SANStorageDriver {
		*newTestOntapSANDriver(&[]bool{true}[0]),
		*newTestOntapSANDriver(&[]bool{false}[0]),
		*newTestOntapSANDriver(nil),
	}

	for _, ontapSanDriver := range ontapsanDrivers {
		sensitive, ok := ontapSanDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok || (ok && !sensitive):
			assert.Contains(t, ontapSanDriver.String(), "<REDACTED>",
				"ontap-san driver did not contain <REDACTED>")
			assert.Contains(t, ontapSanDriver.String(), "API:<REDACTED>",
				"ontap-san driver does not redact client API information")
			assert.Contains(t, ontapSanDriver.String(), "Username:<REDACTED>",
				"ontap-san driver does not redact username")
			assert.NotContains(t, ontapSanDriver.String(), "ontap-san-user",
				"ontap-san driver contains username")
			assert.Contains(t, ontapSanDriver.String(), "Password:<REDACTED>",
				"ontap-san driver does not redact password")
			assert.NotContains(t, ontapSanDriver.String(), "password1!",
				"ontap-san driver contains password")
			assert.NotContains(t, ontapSanDriver.String(), "client_username",
				"ontap-san driver contains username")
			assert.NotContains(t, ontapSanDriver.String(), "client_password",
				"ontap-san driver contains password")

		case ok && sensitive:
			assert.Contains(t, ontapSanDriver.String(), "ontap-san-user",
				"ontap-san driver does not contain username")
			assert.Contains(t, ontapSanDriver.String(), "password1!",
				"ontap-san driver does not contain password")
			assert.Contains(t, ontapSanDriver.String(), "client_username",
				"ontap-san driver contains client_username")
			assert.Contains(t, ontapSanDriver.String(), "client_password",
				"ontap-san driver contains client_password")
		}
	}
}
