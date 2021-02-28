// Copyright 2020 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils"
)

const (
	ProjectNumber     = "123456"
	PrivateKeyId      = "12345678987654321"
	PrivateKey        = "-----BEGIN PRIVATE KEY-----AAAABBBCCCCDDDDDDEEEEEEFFFF----END PRIVATE KEY-----"
	ClientEmail       = "random@random.com"
	ClientID          = "98765432123456789"
	ClientX509CertURL = "https://random.com/x509Cert"
)

func newTestGCPDriver() *NFSStorageDriver {
	config := &drivers.GCPNFSStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	APIKey := drivers.GCPPrivateKey{
		Type:                    "random_account",
		ProjectID:               "random_project",
		PrivateKeyID:            PrivateKeyId,
		PrivateKey:              PrivateKey,
		ClientEmail:             ClientEmail,
		ClientID:                ClientID,
		AuthURI:                 "https://random.com/o/oauth2/auth",
		TokenURI:                "https://random.com/token",
		AuthProviderX509CertURL: "https://random.com/certs",
		ClientX509CertURL:       ClientX509CertURL,
	}

	config.ProjectNumber = ProjectNumber
	config.APIKey = APIKey
	config.Region = "us-central-10"
	config.ProxyURL = "https://random.com"
	config.NfsMountOptions = "nfsvers=3"
	config.VolumeCreateTimeout = "30"
	config.StorageDriverName = "gcp-cvs"
	config.StoragePrefix = sp("test_")

	API := api.NewDriver(api.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		APIKey:          config.APIKey,
		APIRegion:       config.APIRegion,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	GCPDriver := &NFSStorageDriver{}
	GCPDriver.Config = *config
	GCPDriver.API = API
	GCPDriver.apiVersion = utils.MustParseSemantic("20.5.1")
	GCPDriver.sdeVersion = utils.MustParseSemantic("20.6.2")
	GCPDriver.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	GCPDriver.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{12}$`)
	GCPDriver.apiRegions = []string{"us-central-9", "us-central-10"}

	return GCPDriver
}

func callString(s NFSStorageDriver) string {
	return s.String()
}

func callGoString(s NFSStorageDriver) string {
	return s.GoString()
}

func TestGCPStorageDriverConfigString(t *testing.T) {

	var GCPDrivers = []NFSStorageDriver{
		*newTestGCPDriver(),
	}

	for _, toString := range []func(NFSStorageDriver) string{callString, callGoString} {

		for _, GCPDriver := range GCPDrivers {
			assert.Contains(t, toString(GCPDriver), "<REDACTED>",
				"GCP driver does not contain <REDACTED>")
			assert.Contains(t, toString(GCPDriver), "API:<REDACTED>",
				"GCP driver does not redact API information")
			assert.Contains(t, toString(GCPDriver), "ProjectNumber:<REDACTED>",
				"GCP driver does not redact Project Number")
			assert.Contains(t, toString(GCPDriver), "APIKey:<REDACTED>",
				"GCP driver does not redact APIKey")
			assert.NotContains(t, toString(GCPDriver), ProjectNumber,
				"GCP driver contains project number")
			assert.NotContains(t, toString(GCPDriver), PrivateKeyId,
				"GCP driver contains Private Key Id")
			assert.NotContains(t, toString(GCPDriver), PrivateKey,
				"GCP driver contains Private Key")
			assert.NotContains(t, toString(GCPDriver), ClientEmail,
				"GCP driver contains Client Email")
			assert.NotContains(t, toString(GCPDriver), ClientID,
				"GCP driver contains Client ID")
			assert.NotContains(t, toString(GCPDriver), ClientX509CertURL,
				"GCP driver contains Client X509 Cert URL")
		}
	}
}

func TestMakeNetworkPath(t *testing.T) {

	driver := newTestGCPDriver()

	// Without shared VPC host project
	driver.Config = drivers.GCPNFSStorageDriverConfig{
		ProjectNumber: "737253775480",
	}
	assert.Equal(t,
		"projects/737253775480/global/networks/myNetwork",
		driver.makeNetworkPath("myNetwork"))

	// With shared VPC host project
	driver.Config = drivers.GCPNFSStorageDriverConfig{
		ProjectNumber:     "737253775480",
		HostProjectNumber: "527303026223",
	}
	assert.Equal(t,
		"projects/527303026223/global/networks/myNetwork",
		driver.makeNetworkPath("myNetwork"))
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
		//Valid storage prefixes
		{
			Name:          "storage prefix has dash",
			StoragePrefix: "abcd-123",
			Valid:         true,
		},
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
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
