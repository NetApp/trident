// Copyright 2020 NetApp, Inc. All Rights Reserved.

package eseries

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/eseries/api"
)

const (
	Username      = "tester"
	Password      = "password"
	PasswordArray = "passwords"
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

func newTestEseriesSANDriverAPI(config *drivers.ESeriesStorageDriverConfig) *api.Client {

	telemetry := make(map[string]string)
	telemetry["version"] = "21.01.0"
	telemetry["plugin"] = "eseries"
	telemetry["storagePrefix"] = *config.StoragePrefix

	API := api.NewAPIClient(context.Background(), api.ClientConfig{
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
		DebugTraceFlags:       config.DebugTraceFlags,
		Protocol:              "iscsi",
		AccessGroup:           config.AccessGroup,
		HostType:              config.HostType,
		DriverName:            "eseries-iscsi",
		Telemetry:             telemetry,
	})

	return API
}

func callString(s SANStorageDriver) string {
	return s.String()
}

func callGoString(s SANStorageDriver) string {
	return s.GoString()
}

func TestEseriesSANStorageDriverConfigString(t *testing.T) {

	var eseriesSANDrivers = []SANStorageDriver{
		*newTestEseriesSANDriver(map[string]bool{
			"method":    true,
			"sensitive": true,
		}),
		*newTestEseriesSANDriver(map[string]bool{
			"method":    true,
			"sensitive": false,
		}),
		*newTestEseriesSANDriver(map[string]bool{}),
	}

	for _, toString := range []func(SANStorageDriver) string{callString, callGoString} {

		for _, eseriesSANDriver := range eseriesSANDrivers {
			sensitive, ok := eseriesSANDriver.Config.DebugTraceFlags["sensitive"]
			eseriesSANDriver.API = newTestEseriesSANDriverAPI(&eseriesSANDriver.Config)

			switch {

			case !ok:
				assert.Contains(t, toString(eseriesSANDriver), "<REDACTED>",
					"Eseries driver does not contain <REDACTED>")
				assert.Contains(t, toString(eseriesSANDriver), "API:<REDACTED>",
					"Eseries driver does not redact API information")
				assert.NotContains(t, toString(eseriesSANDriver), Username,
					"Eseries driver contains  username")
				assert.NotContains(t, toString(eseriesSANDriver), Password,
					"Eseries driver contains password")
				assert.NotContains(t, toString(eseriesSANDriver), PasswordArray,
					"Eseries driver contains password array")
			case ok && sensitive:
				assert.Contains(t, toString(eseriesSANDriver), Username,
					"Eseries driver does not contain username")
				assert.Contains(t, toString(eseriesSANDriver), Password,
					"Eseries driver does not contain password")
				assert.Contains(t, toString(eseriesSANDriver), PasswordArray,
					"Eseries driver does not contain password array")
			case ok && !sensitive:
				assert.Contains(t, toString(eseriesSANDriver), "<REDACTED>",
					"Eseries driver does not contain <REDACTED>")
				assert.Contains(t, toString(eseriesSANDriver), "API:<REDACTED>",
					"Eseries driver does not redact API information")
				assert.NotContains(t, toString(eseriesSANDriver), Username,
					"Eseries driver contains  username")
				assert.NotContains(t, toString(eseriesSANDriver), Password,
					"Eseries driver contains password")
				assert.NotContains(t, toString(eseriesSANDriver), PasswordArray,
					"Eseries driver contains password array")
			}
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

	sensitiveInfo, _ := json.Marshal("Body: {\"controllerAddresses\": [\"10.10.10.1\", \"10.10.10.2\"], " +
		"\"password\": \"RaNd0M\"}")

	var eseriesSANDrivers = []SANStorageDriver{
		*newTestEseriesSANDriver(map[string]bool{
			"method":    true,
			"sensitive": true,
			"api":       true,
		}),
		*newTestEseriesSANDriver(map[string]bool{
			"method":    true,
			"sensitive": false,
			"api":       true,
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

	for _, eseriesSANDriver := range eseriesSANDrivers {
		api := eseriesSANDriver.Config.DebugTraceFlags["api"]
		sensitive := eseriesSANDriver.Config.DebugTraceFlags["sensitive"]
		server.Config.TLSConfig = &tls.Config{
			InsecureSkipVerify: !eseriesSANDriver.Config.WebProxyVerifyTLS,
		}

		eseriesSANDriver.Config.WebProxyPort = port
		eseriesSANDriver.API = newTestEseriesSANDriverAPI(&eseriesSANDriver.Config)

		output := captureOutput(func() {
			if _, _, err := eseriesSANDriver.API.InvokeAPI(context.Background(), sensitiveInfo, "", ""); err != nil {
				t.Fatal(err)
			}
		})

		switch {
		case api && !sensitive:
			assert.NotContains(t, output, "RaNd0M", "Logs contain sensitive information")
			assert.Contains(t, output, "<suppressed>", "Logs do not suppress sensitive information")

		case !api:
			assert.Empty(t, output)

		case api && sensitive:
			assert.Contains(t, output, "RaNd0M", "Logs do not print sensitive information")
			assert.NotContains(t, output, "<suppressed>", "Logs suppress sensitive information")
		}
	}
}

func TestValidateStoragePrefix(t *testing.T) {
	tests := []struct {
		Name          string
		StoragePrefix string
	}{
		{
			Name:          "storage prefix starts with plus",
			StoragePrefix: "+abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts with digit",
			StoragePrefix: "1abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts with underscore",
			StoragePrefix: "_abcd_123_ABC",
		},
		{
			Name:          "storage prefix ends capitalized",
			StoragePrefix: "abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts capitalized",
			StoragePrefix: "ABCD_123_abc",
		},
		{
			Name:          "storage prefix has plus",
			StoragePrefix: "abcd+123_ABC",
		},
		{
			Name:          "storage prefix has dash",
			StoragePrefix: "abcd-123",
		},
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
		},
		{
			Name:          "storage prefix is single digit",
			StoragePrefix: "1",
		},
		{
			Name:          "storage prefix is single underscore",
			StoragePrefix: "_",
		},
		{
			Name:          "storage prefix is single colon",
			StoragePrefix: ":",
		},
		{
			Name:          "storage prefix is single dash",
			StoragePrefix: "-",
		},
		{
			Name:          "storage prefix is empty",
			StoragePrefix: "",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			d := newTestEseriesSANDriver(nil)
			d.Config.StoragePrefix = &test.StoragePrefix

			err := d.populateConfigurationDefaults(context.Background(), &d.Config)
			assert.NoError(t, err)

			err = d.validate(context.Background())
			assert.NoError(t, err, "eseries validation should not fail")
			assert.NotNil(t, d.Config.StoragePrefix, "eseries storage prefix should not be nil")
			assert.Equal(t, *d.Config.StoragePrefix, test.StoragePrefix,
				"eseries storage prefix should be equal to configured prefix")
		})
	}
}
