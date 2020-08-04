// Copyright 2020 NetApp, Inc. All Rights Reserved.

package eseries

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/eseries/api"
)

const (
	Username      = "tester"
	Password      = "password"
	PasswordArray = "Passwords"
)

func newTestEseriesSANDriver(debugTraceFlags map[string]bool) *SANStorageDriver {
	config := &drivers.ESeriesStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = debugTraceFlags

	config.Username = Username
	config.Password = Password
	config.PasswordArray = PasswordArray
	config.WebProxyHostname = "127.0.0.1"
	config.WebProxyPort = "0"
	config.WebProxyUseHTTP = true
	config.WebProxyVerifyTLS = false
	config.ControllerA = "10.0.0.2"
	config.ControllerB = "10.0.0.3"
	config.HostDataIP = "10.0.0.4"
	config.StorageDriverName = "eseries-san"
	config.StoragePrefix = sp("test_")

	sanDriver := &SANStorageDriver{}
	sanDriver.Config = *config

	return sanDriver
}

func newTestEseriesSANDriverAPI(config *drivers.ESeriesStorageDriverConfig) *api.Client{

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
		DebugTraceFlags: 	   config.DebugTraceFlags,
		Protocol:              "iscsi",
		AccessGroup:           config.AccessGroup,
		HostType:              config.HostType,
		DriverName:            "eseries-iscsi",
		Telemetry:             telemetry,
	})

	return API
}

func TestEseriesSANStorageDriverConfigString(t *testing.T) {

	var EseriesSANDrivers = []SANStorageDriver{
		*newTestEseriesSANDriver(map[string]bool{
			"method": true,
			"sensitive": true,
		}),
		*newTestEseriesSANDriver(map[string]bool{
			"method": true,
			"sensitive": false,
		}),
		*newTestEseriesSANDriver(map[string]bool{}),
	}

	for _, EseriesSANDriver := range EseriesSANDrivers {
		sensitive, ok := EseriesSANDriver.Config.DebugTraceFlags["sensitive"]
		EseriesSANDriver.API = newTestEseriesSANDriverAPI(&EseriesSANDriver.Config)

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

func captureOutput(f func()) string {
	var buf bytes.Buffer
	startingLevel := log.GetLevel()
	defer log.SetLevel(startingLevel)
	defer log.SetOutput(os.Stdout)
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)
	f()
	return buf.String()
}

func TestEseriesSANStorageDriverInvokeAPI(t *testing.T) {

	sensitiveInfo, _ := json.Marshal("Body: {\"controllerAddresses\": [\"10.193.156.28\", \"10.193.156.29\"], " +
		"\"password\": \"Netapp123\"}")

	var EseriesSANDrivers = []SANStorageDriver{
		*newTestEseriesSANDriver(map[string]bool{
			"method": true,
			"sensitive": true,
			"api": true,
		}),
		*newTestEseriesSANDriver(map[string]bool{
			"method": true,
			"sensitive": false,
			"api": true,
		 }),
		 *newTestEseriesSANDriver(map[string]bool{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/devmgr/v2/storage-systems/", func(w http.ResponseWriter, r *http.Request) {
	})

	server := httptest.NewUnstartedServer(mux)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	server.Listener = listener
	server.Start()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			log.Error("Panic in TestEseriesSANStorageDriverInvokeAPI", r)
		}
	}()

	defer server.Close()

	for _, EseriesSANDriver := range EseriesSANDrivers {
		api, _ := EseriesSANDriver.Config.DebugTraceFlags["api"]
		sensitive, _ := EseriesSANDriver.Config.DebugTraceFlags["sensitive"]
		server.Config.TLSConfig = &tls.Config{
			InsecureSkipVerify: !EseriesSANDriver.Config.WebProxyVerifyTLS,
		}

		EseriesSANDriver.Config.WebProxyPort = port
		EseriesSANDriver.API = newTestEseriesSANDriverAPI(&EseriesSANDriver.Config)

		output := captureOutput(func() {
			EseriesSANDriver.API.InvokeAPI(sensitiveInfo, "", "")
		})

		switch {

		case api && !sensitive:

			assert.NotContains(t, output, "Netapp123", "Logs contain sensitive information")
			assert.Contains(t, output, "<suppressed>", "Logs do not suppress sensitive information")

		case !api:

			assert.Empty(t, output)

		case api && sensitive:

			assert.Contains(t, output, "Netapp123", "Logs do not print sensitive information")
			assert.NotContains(t, output, "<suppressed>", "Logs suppress sensitive information")
		}
	}
}
