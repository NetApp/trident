// Copyright 2020 NetApp, Inc. All Rights Reserved.

package azure

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/sdk"
)

const (
	SubscriptionID = "1-subid-23456789876454321"
	TenantID = "1-tenantid-23456789876454321"
	ClientID = "1-clientid-23456789876454321"
	ClientSecret = "client-secret-23456789876454321"
)

func newTestANFDriver(showSensitive *bool) *NFSStorageDriver {
	config := &drivers.AzureNFSStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	if showSensitive != nil {
		config.CommonStorageDriverConfig.DebugTraceFlags["sensitive"] = *showSensitive
	}

	config.SubscriptionID = SubscriptionID
	config.TenantID = TenantID
	config.ClientID = ClientID
	config.ClientSecret = ClientSecret
	config.NfsMountOptions = "nfsvers=3"
	config.VolumeCreateTimeout = "30"
	config.StorageDriverName = "ANF-cvs"
	config.StoragePrefix = sp("test_")

	SDK := sdk.NewDriver(sdk.ClientConfig{
		SubscriptionID:  config.SubscriptionID,
		TenantID:        config.TenantID,
		ClientID:        config.ClientID,
		ClientSecret:    config.ClientSecret,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	ANFDriver := &NFSStorageDriver{}
	ANFDriver.Config = *config
	ANFDriver.SDK = SDK
	ANFDriver.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	ANFDriver.volumeCreateTimeout = 30 * time.Second

	return ANFDriver
}

func callString(s NFSStorageDriver) string {
	return s.String()
}

func callGoString(s NFSStorageDriver) string {
	return s.GoString()
}

func TestANFStorageDriverConfigString(t *testing.T) {

	var ANFDrivers = []NFSStorageDriver{
		*newTestANFDriver(&[]bool{true}[0]),
		*newTestANFDriver(&[]bool{false}[0]),
		*newTestANFDriver(nil),
	}

	for _, toString := range []func(NFSStorageDriver) string{callString, callGoString} {

		for _, ANFDriver := range ANFDrivers {
			sensitive, ok := ANFDriver.Config.DebugTraceFlags["sensitive"]

			switch {

			case !ok:
				assert.Contains(t, toString(ANFDriver), "<REDACTED>",
					"ANF driver does not contain <REDACTED>")
				assert.Contains(t, toString(ANFDriver), "SDK:<REDACTED>",
					"ANF driver does not redact SDK information")
				assert.Contains(t, toString(ANFDriver), "SubscriptionID:<REDACTED>",
					"ANF driver does not redact API URL")
				assert.Contains(t, toString(ANFDriver), "TenantID:<REDACTED>",
					"ANF driver does not redact Tenant ID")
				assert.Contains(t, toString(ANFDriver), "ClientID:<REDACTED>",
					"ANF driver does not redact Client ID")
				assert.Contains(t, toString(ANFDriver), "ClientSecret:<REDACTED>",
					"ANF driver does not redact Client Secret")
				assert.NotContains(t, toString(ANFDriver), SubscriptionID,
					"ANF driver contains Subscription ID")
				assert.NotContains(t, toString(ANFDriver), TenantID,
					"ANF driver contains Tenant ID")
				assert.NotContains(t, toString(ANFDriver), ClientID,
					"ANF driver contains Client ID")
				assert.NotContains(t, toString(ANFDriver), ClientSecret,
					"ANF driver contains Client Secret")
			case ok && sensitive:
				assert.Contains(t, toString(ANFDriver), SubscriptionID,
					"ANF driver does not contains Subscription ID")
				assert.Contains(t, toString(ANFDriver), TenantID,
					"ANF driver does not contains Tenant ID")
				assert.Contains(t, toString(ANFDriver), ClientID,
					"ANF driver does not contains Client ID")
				assert.Contains(t, toString(ANFDriver), ClientSecret,
					"ANF driver does not contains Client Secret")
			case ok && !sensitive:
				assert.Contains(t, toString(ANFDriver), "<REDACTED>",
					"ANF driver does not contain <REDACTED>")
				assert.Contains(t, toString(ANFDriver), "SDK:<REDACTED>",
					"ANF driver does not redact SDK information")
				assert.Contains(t, toString(ANFDriver), "SubscriptionID:<REDACTED>",
					"ANF driver does not redact API URL")
				assert.Contains(t, toString(ANFDriver), "TenantID:<REDACTED>",
					"ANF driver does not redact Tenant ID")
				assert.Contains(t, toString(ANFDriver), "ClientID:<REDACTED>",
					"ANF driver does not redact Client ID")
				assert.Contains(t, toString(ANFDriver), "ClientSecret:<REDACTED>",
					"ANF driver does not redact Client Secret")
				assert.NotContains(t, toString(ANFDriver), SubscriptionID,
					"ANF driver contains Subscription ID")
				assert.NotContains(t, toString(ANFDriver), TenantID,
					"ANF driver contains Tenant ID")
				assert.NotContains(t, toString(ANFDriver), ClientID,
					"ANF driver contains Client ID")
				assert.NotContains(t, toString(ANFDriver), ClientSecret,
					"ANF driver contains Client Secret")
			}
		}
	}
}
