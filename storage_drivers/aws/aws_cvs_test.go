// Copyright 2020 NetApp, Inc. All Rights Reserved.

package aws

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/v21/storage_drivers"
	"github.com/netapp/trident/v21/storage_drivers/aws/api"
	"github.com/netapp/trident/v21/utils"
)

const (
	APIKey    = "-----BEGIN PRIVATE KEY-----AAAABBBCCCCDDDDDDEEEEEEFFFF----END PRIVATE KEY-----"
	APIURL    = "https://randomapi.aws.com"
	SecretKey = "-----BEGIN PRIVATE KEY-----GGGGHHHHIIIIIJJJJJJKKKKLLL----END PRIVATE KEY-----"
	ProxyURL  = "https://randomproxy.aws.com"
)

func newTestAWSDriver() *NFSStorageDriver {
	config := &drivers.AWSNFSStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

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
		*newTestAWSDriver(),
	}

	for _, toString := range []func(NFSStorageDriver) string{callString, callGoString} {
		for _, AWSDriver := range AWSDrivers {
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

func TestValidateStoragePrefix(t *testing.T) {
	tests := []struct {
		Name          string
		StoragePrefix string
		Valid         bool
	}{
		//Invalid storage prefixes
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
			Name:          "storage prefix is single digit",
			StoragePrefix: "1",
		},
		// Valid storage prefixes
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
			Valid:         true,
		},
		{
			Name:          "storage prefix is single underscore",
			StoragePrefix: "_",
			Valid:         true,
		},
		{
			Name:          "storage prefix is single colon",
			StoragePrefix: ":",
		},
		{
			Name:          "storage prefix is single dash",
			StoragePrefix: "-",
			Valid:         true,
		},
		{
			Name:          "storage prefix is empty",
			StoragePrefix: "",
			Valid:         true,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			err := validateStoragePrefix(test.StoragePrefix)
			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}
