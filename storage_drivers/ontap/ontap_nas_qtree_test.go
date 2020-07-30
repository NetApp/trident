// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newNASQtreeStorageDriver(showSensitive *bool) *NASQtreeStorageDriver {
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
	config.Username = "ontap-nas-qtree-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas-economy"
	config.StoragePrefix = sp("test_")

	nasqtreeDriver := &NASQtreeStorageDriver{}
	nasqtreeDriver.Config = *config

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

	nasqtreeDriver.API = api.NewClient(clientConfig)
	nasqtreeDriver.Telemetry = &Telemetry{
		Plugin:        nasqtreeDriver.Name(),
		SVM:           nasqtreeDriver.GetConfig().SVM,
		StoragePrefix: *nasqtreeDriver.GetConfig().StoragePrefix,
		Driver:        nasqtreeDriver,
		done:          make(chan struct{}),
	}

	return nasqtreeDriver
}

func TestOntapNasQtreeStorageDriverConfigString(t *testing.T) {

	var qtreeDrivers = []NASQtreeStorageDriver {
		*newNASQtreeStorageDriver(&[]bool{true}[0]),
		*newNASQtreeStorageDriver(&[]bool{false}[0]),
		*newNASQtreeStorageDriver(nil),
	}

	for _, qtreeDriver := range qtreeDrivers {
		sensitive, ok := qtreeDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok || (ok && !sensitive):
			assert.Contains(t, qtreeDriver.String(), "<REDACTED>",
				"ontap-nas-qtree driver did not contain <REDACTED>")
			assert.Contains(t, qtreeDriver.String(), "API:<REDACTED>",
				"ontap-nas-qtree driver does not redact client API information")
			assert.Contains(t, qtreeDriver.String(), "Username:<REDACTED>",
				"ontap-nas-qtree driver does not redact username")
			assert.NotContains(t, qtreeDriver.String(), "ontap-nas-qtree-user",
				"ontap-nas-qtree driver contains username")
			assert.Contains(t, qtreeDriver.String(), "Password:<REDACTED>",
				"ontap-nas-qtree driver does not redact password")
			assert.NotContains(t, qtreeDriver.String(), "password1!",
				"ontap-nas-qtree driver contains password")
			assert.NotContains(t, qtreeDriver.String(), "client_username",
				"ontap-nas-qtree driver contains username")
			assert.NotContains(t, qtreeDriver.String(), "client_password",
				"ontap-nas-qtree driver contains password")

		case ok && sensitive:
			assert.Contains(t, qtreeDriver.String(), "ontap-nas-qtree-user",
				"ontap-qtree driver does not contain username")
			assert.Contains(t, qtreeDriver.String(), "password1!",
				"ontap-qtree driver does not contain password")
			assert.Contains(t, qtreeDriver.String(), "client_username",
				"ontap-nas-qtree driver contains client_username")
			assert.Contains(t, qtreeDriver.String(), "client_password",
				"ontap-nas-qtree driver contains client_password")
		}
	}
}
