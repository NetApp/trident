// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newTestOntapNASFGDriver() *NASFlexGroupStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	config.ManagementLIF = ONTAPTEST_LOCALHOST
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
		ManagementLIF:           config.ManagementLIF,
		SVM:                     "SVM1",
		Username:                "client_username",
		Password:                "client_password",
		DriverContext:           tridentconfig.DriverContext("driverContext"),
		ContextBasedZapiRecords: 100,
		DebugTraceFlags:         nil,
	}

	nasfgDriver.API = api.NewClient(clientConfig)
	nasfgDriver.telemetry = &Telemetry{
		Plugin:        nasfgDriver.Name(),
		SVM:           nasfgDriver.GetConfig().SVM,
		StoragePrefix: *nasfgDriver.GetConfig().StoragePrefix,
		Driver:        nasfgDriver,
		done:          make(chan struct{}),
	}

	return nasfgDriver
}

func TestOntapNasFgStorageDriverConfigString(t *testing.T) {

	var ontapNasFgDrivers = []NASFlexGroupStorageDriver{
		*newTestOntapNASFGDriver(),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-nas-fg-user",
		"password":        "password1!",
		"client username": "client_username",
		"client password": "client_password",
	}

	externalIncludeList := map[string]string{
		"<REDACTED>":                   "<REDACTED>",
		"username":                     "Username:<REDACTED>",
		"password":                     "Password:<REDACTED>",
		"api":                          "API:<REDACTED>",
		"chap username":                "ChapUsername:<REDACTED>",
		"chap initiator secret":        "ChapInitiatorSecret:<REDACTED>",
		"chap target username":         "ChapTargetUsername:<REDACTED>",
		"chap target initiator secret": "ChapTargetInitiatorSecret:<REDACTED>",
		"client private key":           "ClientPrivateKey:<REDACTED>",
	}

	for _, ontapNasFgDriver := range ontapNasFgDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, ontapNasFgDriver.String(), val,
				"ontap-nas-fg driver does not contain %v", key)
			assert.Contains(t, ontapNasFgDriver.GoString(), val,
				"ontap-nas-fg driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, ontapNasFgDriver.String(), val,
				"ontap-nas-fg driver contains %v", key)
			assert.NotContains(t, ontapNasFgDriver.GoString(), val,
				"ontap-nas-fg driver contains %v", key)
		}
	}
}
