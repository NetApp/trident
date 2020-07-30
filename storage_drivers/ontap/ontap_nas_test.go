// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newTestOntapNASDriver(showSensitive *bool) *NASStorageDriver {
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
	config.Username = "ontap-nas-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas"
	config.StoragePrefix = sp("test_")

	nasDriver := &NASStorageDriver{}
	nasDriver.Config = *config

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

	nasDriver.API = api.NewClient(clientConfig)
	nasDriver.Telemetry = &Telemetry{
		Plugin:        nasDriver.Name(),
		SVM:           nasDriver.GetConfig().SVM,
		StoragePrefix: *nasDriver.GetConfig().StoragePrefix,
		Driver:        nasDriver,
		done:          make(chan struct{}),
	}

	return nasDriver
}

func TestOntapNasStorageDriverConfigString(t *testing.T) {

	var ontapNasDrivers = []NASStorageDriver {
		*newTestOntapNASDriver(&[]bool{true}[0]),
		*newTestOntapNASDriver(&[]bool{false}[0]),
		*newTestOntapNASDriver(nil),
	}

	for _, ontapNasDriver := range ontapNasDrivers {
		sensitive, ok := ontapNasDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok || (ok && !sensitive):
			assert.Contains(t, ontapNasDriver.String(), "<REDACTED>",
				"ontap-nas driver did not contain <REDACTED>")
			assert.Contains(t, ontapNasDriver.String(), "API:<REDACTED>",
				"ontap-nas driver does not redact client API information")
			assert.Contains(t, ontapNasDriver.String(), "Username:<REDACTED>",
				"ontap-nas driver does not redact username")
			assert.NotContains(t, ontapNasDriver.String(), "ontap-nas-user",
				"ontap-nas driver contains username")
			assert.Contains(t, ontapNasDriver.String(), "Password:<REDACTED>",
				"ontap-nas driver does not redact password")
			assert.NotContains(t, ontapNasDriver.String(), "password1!",
				"ontap-nas driver contains password")
			assert.NotContains(t, ontapNasDriver.String(), "client_username",
				"ontap-nas driver contains username")
			assert.NotContains(t, ontapNasDriver.String(), "client_password",
				"ontap-nas driver contains password")

		case ok && sensitive:
			assert.Contains(t, ontapNasDriver.String(), "ontap-nas-user",
				"ontap-nas driver does not contain username")
			assert.Contains(t, ontapNasDriver.String(), "password1!",
				"ontap-nas driver does not contain password")
			assert.Contains(t, ontapNasDriver.String(), "client_username",
				"ontap-nas driver contains client_username")
			assert.Contains(t, ontapNasDriver.String(), "client_password",
				"ontap-nas driver contains client_password")
		}
	}
}
