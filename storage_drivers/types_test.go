// Copyright 2020 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
)

func newTestOntapStorageDriverConfig(debugTraceFlags map[string]bool) *OntapStorageDriverConfig {
	config := &OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = debugTraceFlags
	config.ManagementLIF = "127.0.0.1"
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "ontap-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap"
	config.StoragePrefix = sp("test_")
	config.ChapUsername = "ontap-chap-user"
	config.ChapInitiatorSecret = "ontap-chap-secret"
	config.ChapTargetInitiatorSecret = "ontap-chap-target-secret"
	config.ChapTargetUsername = "ontap-chap-target-user"
	return config
}

func TestOntapStorageDriverConfigString(t *testing.T) {
	ontapStorageDriverConfigs := []OntapStorageDriverConfig{
		*newTestOntapStorageDriverConfig(map[string]bool{"method": true}),
		*newTestOntapStorageDriverConfig(nil),
	}

	// key: string to include in debug logs when the sensitive flag is set to true
	// value: string to include in unit test failure error message if key is missing when expected
	sensitiveIncludeList := map[string]string{
		"ontap-user":               "username",
		"password1!":               "password",
		"ontap-chap-user":          "chap username",
		"ontap-chap-secret":        "chap initiator secret name",
		"ontap-chap-target-secret": "chap target secret",
		"ontap-chap-target-user":   "chap target username",
	}

	// key: string to include in debug logs when the sensitive flag is set to false
	// value: string to include in unit test failure error message if key is missing when expected
	externalIncludeList := map[string]string{
		"<REDACTED>":                           "<REDACTED>",
		"Username:<REDACTED>":                  "username",
		"Password:<REDACTED>":                  "password",
		"ChapUsername:<REDACTED>":              "chap username",
		"ChapInitiatorSecret:<REDACTED>":       "chap initiator secret",
		"ChapTargetUsername:<REDACTED>":        "chap target username",
		"ChapTargetInitiatorSecret:<REDACTED>": "chap target initiator secret",
		"ClientPrivateKey:<REDACTED>":          "client private key",
	}

	for _, ontapStorageDriverConfig := range ontapStorageDriverConfigs {
		for key, val := range externalIncludeList {
			assert.Contains(t, ontapStorageDriverConfig.String(), key,
				"%s driver does not contain %v", ontapStorageDriverConfig.StorageDriverName, val)
			assert.Contains(t, ontapStorageDriverConfig.GoString(), key,
				"%s driver does not contain %v", ontapStorageDriverConfig.StorageDriverName, val)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, ontapStorageDriverConfig.String(), key,
				"%s driver contains %v", ontapStorageDriverConfig.StorageDriverName, val)
			assert.NotContains(t, ontapStorageDriverConfig.GoString(), key,
				"%s driver contains %v", ontapStorageDriverConfig.StorageDriverName, val)
		}
	}
}

func newTestStorageDriverConfig(debugTraceFlags map[string]bool) *FakeStorageDriverConfig {
	fakeConfig := &FakeStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Version:           ConfigVersion,
			StorageDriverName: FakeStorageDriverName,
			DebugTraceFlags:   debugTraceFlags,
		},
		Protocol:     config.File,
		InstanceName: "fake-instance",
		Username:     "fake-user",
		Password:     "fake-password",
	}

	return fakeConfig
}

func TestStorageDriverConfigString(t *testing.T) {
	fakeStorageDriverConfigs := []FakeStorageDriverConfig{
		*newTestStorageDriverConfig(map[string]bool{"method": true}),
		*newTestStorageDriverConfig(nil),
	}

	// key: string to include in debug logs when the sensitive flag is set to true
	// value: string to include in unit test failure error message if key is missing when expected
	sensitiveIncludeList := map[string]string{
		"fake-user":     "username",
		"fake-password": "password",
	}

	// key: string to include in debug logs when the sensitive flag is set to false
	// value: string to include in unit test failure error message if key is missing when expected
	externalIncludeList := map[string]string{
		"<REDACTED>":          "<REDACTED>",
		"Username:<REDACTED>": "username",
		"Password:<REDACTED>": "password",
	}

	for _, fakeStorageDriverConfig := range fakeStorageDriverConfigs {
		for key, val := range externalIncludeList {
			assert.Contains(t, fakeStorageDriverConfig.String(), key,
				"%s driver config does not contain %v", fakeStorageDriverConfig.StorageDriverName, val)
			assert.Contains(t, fakeStorageDriverConfig.GoString(), key,
				"%s driver config does not contain %v", fakeStorageDriverConfig.StorageDriverName, val)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, fakeStorageDriverConfig.String(), key,
				"%s driver config contains %v", fakeStorageDriverConfig.StorageDriverName, val)
			assert.NotContains(t, fakeStorageDriverConfig.GoString(), key,
				"%s driver config contains %v", fakeStorageDriverConfig.StorageDriverName, val)
		}
	}
}

func TestGetCredentialNameAndType(t *testing.T) {
	type CredentialNameAndType struct {
		Values map[string]string
		Name   string
		Type   string
		Error  error
	}

	inputs := []CredentialNameAndType{
		{
			map[string]string{"name": "secret1", "type": "secret"},
			"secret1",
			"secret",
			nil,
		},
		{
			map[string]string{"name": "secret1"},
			"secret1",
			"secret",
			nil,
		},
		{
			map[string]string{"type": "secret"},
			"",
			"",
			fmt.Errorf("credentials field is missing 'name' attribute"),
		},
		{
			map[string]string{"name": "", "type": "KMIP"},
			"",
			"",
			fmt.Errorf("credentials field does not support type '%s'", "KMIP"),
		},
		{
			map[string]string{"name": "", "type": "secret", "randomKey": "randomValue"},
			"",
			"",
			fmt.Errorf("credentials field contains invalid fields '%v' attribute", []string{"randomKey"}),
		},
		{
			map[string]string{},
			"",
			"",
			nil,
		},
		{
			nil,
			"",
			"",
			nil,
		},
	}

	for _, input := range inputs {
		secretName, secretType, err := getCredentialNameAndType(input.Values)
		assert.Equal(t, secretName, input.Name)
		assert.Equal(t, secretType, input.Type)
		assert.Equal(t, err, input.Error)
	}
}
