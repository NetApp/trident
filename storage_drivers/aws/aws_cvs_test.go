// Copyright 2020 NetApp, Inc. All Rights Reserved.

package aws

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/aws/api"
	"github.com/netapp/trident/utils"
)

const (
	APIKey    = "-----BEGIN PRIVATE KEY-----AAAABBBCCCCDDDDDDEEEEEEFFFF----END PRIVATE KEY-----"
	APIURL    = "https://randomapi.aws.com"
	SecretKey = "-----BEGIN PRIVATE KEY-----GGGGHHHHIIIIIJJJJJJKKKKLLL----END PRIVATE KEY-----"
	ProxyURL  = "https://randomproxy.aws.com"
)

func newTestAWSDriver(showSensitive *bool) *NFSStorageDriver {
	config := &drivers.AWSNFSStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	if showSensitive != nil {
		config.CommonStorageDriverConfig.DebugTraceFlags["sensitive"] = *showSensitive
	}

	config.APIURL = APIURL
	config.APIKey = APIKey
	config.APIRegion = "us-central-10"
	config.SecretKey = SecretKey
	config.ProxyURL = ProxyURL
	config.NfsMountOptions = "nfsvers=3"
	config.VolumeCreateTimeout = "30"
	config.StorageDriverName = "aws-cvs"
	config.StoragePrefix = sp("test_")

	API := api.NewDriver(api.ClientConfig{
		APIURL:          config.APIURL,
		APIKey:          config.APIKey,
		SecretKey:       config.SecretKey,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	AWSDriver := &NFSStorageDriver{}
	AWSDriver.Config = *config
	AWSDriver.API = API
	AWSDriver.apiVersion = utils.MustParseSemantic("20.5.1")
	AWSDriver.sdeVersion = utils.MustParseSemantic("20.6.2")
	AWSDriver.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	AWSDriver.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{12}$`)
	AWSDriver.apiRegions = []string{"us-central-9", "us-central-10"}

	return AWSDriver
}

func callString(s NFSStorageDriver) string {
	return s.String()
}

func callGoString(s NFSStorageDriver) string {
	return s.GoString()
}

func TestAWSStorageDriverConfigString(t *testing.T) {

	var AWSDrivers = []NFSStorageDriver{
		*newTestAWSDriver(&[]bool{true}[0]),
		*newTestAWSDriver(&[]bool{false}[0]),
		*newTestAWSDriver(nil),
	}

	for _, toString := range []func(NFSStorageDriver) string{callString, callGoString} {

		for _, AWSDriver := range AWSDrivers {
			sensitive, ok := AWSDriver.Config.DebugTraceFlags["sensitive"]

			switch {

			case !ok:
				assert.Contains(t, toString(AWSDriver), "<REDACTED>",
					"AWS driver does not contain <REDACTED>")
				assert.Contains(t, toString(AWSDriver), "API:<REDACTED>",
					"AWS driver does not redact API information")
				assert.Contains(t, toString(AWSDriver), "APIURL:<REDACTED>",
					"AWS driver does not redact API URL")
				assert.Contains(t, toString(AWSDriver), "APIKey:<REDACTED>",
					"AWS driver does not redact APIKey")
				assert.Contains(t, toString(AWSDriver), "SecretKey:<REDACTED>",
					"AWS driver does not redact Secret Key")
				assert.NotContains(t, toString(AWSDriver), APIURL,
					"AWS driver contains API URL")
				assert.NotContains(t, toString(AWSDriver), APIKey,
					"AWS driver contains API Key ")
				assert.NotContains(t, toString(AWSDriver), SecretKey,
					"AWS driver contains Secret Key")
			case ok && sensitive:
				assert.Contains(t, toString(AWSDriver), APIURL,
					"AWS driver does not contains API URL")
				assert.Contains(t, toString(AWSDriver), APIKey,
					"AWS driver does not contains API Key")
				assert.Contains(t, toString(AWSDriver), SecretKey,
					"AWS driver does not contains Secret Key")
			case ok && !sensitive:
				assert.Contains(t, toString(AWSDriver), "<REDACTED>",
					"AWS driver does not contain <REDACTED>")
				assert.Contains(t, toString(AWSDriver), "API:<REDACTED>",
					"AWS driver does not redact API information")
				assert.Contains(t, toString(AWSDriver), "APIURL:<REDACTED>",
					"AWS driver does not redact API URL")
				assert.Contains(t, toString(AWSDriver), "APIKey:<REDACTED>",
					"AWS driver does not redact APIKey")
				assert.Contains(t, toString(AWSDriver), "SecretKey:<REDACTED>",
					"AWS driver does not redact Secret Key")
				assert.NotContains(t, toString(AWSDriver), APIURL,
					"AWS driver contains API URL")
				assert.NotContains(t, toString(AWSDriver), APIKey,
					"AWS driver contains API Key ")
				assert.NotContains(t, toString(AWSDriver), SecretKey,
					"AWS driver contains Secret Key")
			}
		}
	}
}
