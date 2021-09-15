// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
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

	numRecords := api.DefaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		numRecords = api.MaxZapiRecords
	}

	var ontapAPI api.OntapAPI

	if config.UseREST == true {
		ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
	} else {
		ontapAPI, _ = api.NewZAPIClientFromOntapConfig(context.TODO(), config, numRecords)
	}

	nasfgDriver.API = ontapAPI
	nasfgDriver.telemetry = &TelemetryAbstraction{
		Plugin:        nasfgDriver.Name(),
		SVM:           nasfgDriver.GetConfig().SVM,
		StoragePrefix: *nasfgDriver.GetConfig().StoragePrefix,
		Driver:        nasfgDriver,
		done:          make(chan struct{}),
	}

	return nasfgDriver
}

func TestOntapNasFgStorageDriverConfigString(t *testing.T) {

	var ontapNasFgDriver = *newTestOntapNASFGDriver()

	excludeList := map[string]string{
		"username":                             ontapNasFgDriver.Config.Username,
		"password":                             ontapNasFgDriver.Config.Password,
		"client private key":                   "BEGIN PRIVATE KEY",
		"client private key base 64 encoding ": "QkVHSU4gUFJJVkFURSBLRVk=",
	}

	includeList := map[string]string{
		"<REDACTED>":         "<REDACTED>",
		"username":           "Username:<REDACTED>",
		"password":           "Password:<REDACTED>",
		"api":                "API:<REDACTED>",
		"client private key": "ClientPrivateKey:<REDACTED>",
	}

	for key, val := range includeList {
		assert.Contains(t, ontapNasFgDriver.String(), val,
			"ontap-nas-fg driver does not contain %v", key)
		assert.Contains(t, ontapNasFgDriver.GoString(), val,
			"ontap-nas-fg driver does not contain %v", key)
	}

	for key, val := range excludeList {
		assert.NotContains(t, ontapNasFgDriver.String(), val,
			"ontap-nas-fg driver contains %v", key)
		assert.NotContains(t, ontapNasFgDriver.GoString(), val,
			"ontap-nas-fg driver contains %v", key)
	}

}
