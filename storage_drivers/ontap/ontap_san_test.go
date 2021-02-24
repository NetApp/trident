// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newTestOntapSANDriver() *SANStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

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
		ManagementLIF:           config.ManagementLIF,
		SVM:                     "SVM1",
		Username:                "client_username",
		Password:                "client_password",
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

	var ontapSanDrivers = []SANStorageDriver{
		*newTestOntapSANDriver(),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-san-user",
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

	for _, ontapSanDriver := range ontapSanDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, ontapSanDriver.String(), val,
				"ontap-san driver does not contain %v", key)
			assert.Contains(t, ontapSanDriver.GoString(), val,
				"ontap-san driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, ontapSanDriver.String(), val,
				"ontap-san driver contains %v", key)
			assert.NotContains(t, ontapSanDriver.GoString(), val,
				"ontap-san driver contains %v", key)
		}
	}
}
