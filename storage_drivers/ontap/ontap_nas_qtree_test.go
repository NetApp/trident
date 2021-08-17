// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/v21/config"
	drivers "github.com/netapp/trident/v21/storage_drivers"
	"github.com/netapp/trident/v21/storage_drivers/ontap/api"
)

func newNASQtreeStorageDriver() *NASQtreeStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	config.ManagementLIF = ONTAPTEST_LOCALHOST
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
		ManagementLIF:           config.ManagementLIF,
		SVM:                     "SVM1",
		Username:                "client_username",
		Password:                "client_password",
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

	var qtreeDrivers = []NASQtreeStorageDriver{
		*newNASQtreeStorageDriver(),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-nas-qtree-user",
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

	for _, qtreeDriver := range qtreeDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, qtreeDriver.String(), val,
				"ontap-nas-economy driver does not contain %v", key)
			assert.Contains(t, qtreeDriver.GoString(), val,
				"ontap-nas-economy driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, qtreeDriver.String(), val,
				"ontap-nas-economy driver contains %v", key)
			assert.NotContains(t, qtreeDriver.GoString(), val,
				"ontap-nas-economy driver contains %v", key)
		}
	}
}
