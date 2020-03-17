// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"testing"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/stretchr/testify/assert"
)

func newTestOntapSANConfig() *drivers.OntapStorageDriverConfig {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["trace"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api_get_volumes"] = true
	config.DebugTraceFlags = config.CommonStorageDriverConfig.DebugTraceFlags

	config.ManagementLIF = "127.0.01"
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "username"
	config.Password = "password"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = sp("test_")

	return config
}

// TestCHAP
func TestCHAP(t *testing.T) {
	config := newTestOntapSANConfig()
	config.UseCHAP = true

	var credentials *ChapCredentials
	var err error

	// start off with all 4 having random values
	credentials, err = EnsureBidrectionalChapCredentials(config)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, "unchanged ChapUsername", credentials.ChapUsername)
	assert.NotEqual(t, "unchanged ChapInitiatorSecret", credentials.ChapInitiatorSecret)
	assert.NotEqual(t, "unchanged ChapTargetUsername", credentials.ChapTargetUsername)
	assert.NotEqual(t, "unchanged ChapTargetInitiatorSecret", credentials.ChapTargetInitiatorSecret)

	// add a specific ChapUsername
	config.ChapUsername = "unchanged ChapUsername"
	credentials, err = EnsureBidrectionalChapCredentials(config)
	assert.Equal(t, nil, err)
	assert.Equal(t, "unchanged ChapUsername", credentials.ChapUsername)
	assert.NotEqual(t, "unchanged ChapInitiatorSecret", credentials.ChapInitiatorSecret)
	assert.NotEqual(t, "unchanged ChapTargetUsername", credentials.ChapTargetUsername)
	assert.NotEqual(t, "unchanged ChapTargetInitiatorSecret", credentials.ChapTargetInitiatorSecret)

	// add a specific ChapInitiatorSecret
	config.ChapUsername = "unchanged ChapUsername"
	config.ChapInitiatorSecret = "unchanged ChapInitiatorSecret"
	credentials, err = EnsureBidrectionalChapCredentials(config)
	assert.Equal(t, nil, err)
	assert.Equal(t, "unchanged ChapUsername", credentials.ChapUsername)
	assert.Equal(t, "unchanged ChapInitiatorSecret", credentials.ChapInitiatorSecret)
	assert.NotEqual(t, "unchanged ChapTargetUsername", credentials.ChapTargetUsername)
	assert.NotEqual(t, "unchanged ChapTargetInitiatorSecret", credentials.ChapTargetInitiatorSecret)

	// add a specific ChapTargetUsername
	config.ChapUsername = "unchanged ChapUsername"
	config.ChapInitiatorSecret = "unchanged ChapInitiatorSecret"
	config.ChapTargetUsername = "unchanged ChapTargetUsername"
	credentials, err = EnsureBidrectionalChapCredentials(config)
	assert.Equal(t, nil, err)
	assert.Equal(t, "unchanged ChapUsername", credentials.ChapUsername)
	assert.Equal(t, "unchanged ChapInitiatorSecret", credentials.ChapInitiatorSecret)
	assert.Equal(t, "unchanged ChapTargetUsername", credentials.ChapTargetUsername)
	assert.NotEqual(t, "unchanged ChapTargetInitiatorSecret", credentials.ChapTargetInitiatorSecret)

	// add a specific ChapTargetInitiatorSecret
	config.ChapUsername = "unchanged ChapUsername"
	config.ChapInitiatorSecret = "unchanged ChapInitiatorSecret"
	config.ChapTargetUsername = "unchanged ChapTargetUsername"
	config.ChapTargetInitiatorSecret = "unchanged ChapTargetInitiatorSecret"
	credentials, err = EnsureBidrectionalChapCredentials(config)
	assert.Equal(t, nil, err)
	assert.Equal(t, "unchanged ChapUsername", credentials.ChapUsername)
	assert.Equal(t, "unchanged ChapInitiatorSecret", credentials.ChapInitiatorSecret)
	assert.Equal(t, "unchanged ChapTargetUsername", credentials.ChapTargetUsername)
	assert.Equal(t, "unchanged ChapTargetInitiatorSecret", credentials.ChapTargetInitiatorSecret)
}

func Test_randomChapString16(t *testing.T) {
	validChars := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	for i := 0; i < 1024*8; i++ {
		s, err := randomChapString16()
		assert.Equal(t, nil, err)
		assert.NotEqual(t, "", s)
		assert.Equal(t, 16, len(s))

		// trim out all the valid characters, anything left would be an invalid character
		trimmed := ""
		for i := 0; i < len(s); i++ {
			isValid := false
			for j := 0; j < len(validChars); j++ {
				if s[i] == validChars[j] {
					isValid = true
				}
			}
			if isValid {
				continue
			}
			trimmed += string(s[i])
		}
		// make sure nothing invalid is left in the trimmed string
		assert.Equal(t, 0, len(trimmed))
	}
}
