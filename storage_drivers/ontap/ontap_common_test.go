// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
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

	config.ManagementLIF = "127.0.0.1"
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "username"
	config.Password = "password"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = sp("test_")

	return config
}

func newIscsiInitiatorGetDefaultAuthResponse(authType string) *azgo.IscsiInitiatorGetDefaultAuthResponse {
	result := azgo.NewIscsiInitiatorGetDefaultAuthResponse()
	result.Result.SetAuthType(authType)
	return result
}

func newIscsiInitiatorGetDefaultAuthResponseCHAP(authType, userName, outboundUsername string) *azgo.IscsiInitiatorGetDefaultAuthResponse {
	result := azgo.NewIscsiInitiatorGetDefaultAuthResponse()
	result.Result.SetAuthType(authType)
	result.Result.SetUserName(userName)
	result.Result.SetOutboundUserName(outboundUsername)
	return result
}

// TestCHAP1 tests that all 4 CHAP credential fields are required
func TestCHAP1(t *testing.T) {
	config := newTestOntapSANConfig()
	config.UseCHAP = true

	var credentials *ChapCredentials
	var err error

	defaultAuthNone := newIscsiInitiatorGetDefaultAuthResponse("none")

	// start off missing all 4 values and validate that it generates an error
	_, err = ValidateBidrectionalChapCredentials(defaultAuthNone, config)
	assert.EqualError(t, err, "missing value for required field(s) [ChapUsername ChapInitiatorSecret ChapTargetUsername ChapTargetInitiatorSecret]")

	// add a specific ChapUsername
	config.ChapUsername = "unchanged ChapUsername"
	_, err = ValidateBidrectionalChapCredentials(defaultAuthNone, config)
	assert.EqualError(t, err, "missing value for required field(s) [ChapInitiatorSecret ChapTargetUsername ChapTargetInitiatorSecret]")

	// add a specific ChapInitiatorSecret
	config.ChapUsername = "unchanged ChapUsername"
	config.ChapInitiatorSecret = "unchanged ChapInitiatorSecret"
	_, err = ValidateBidrectionalChapCredentials(defaultAuthNone, config)
	assert.EqualError(t, err, "missing value for required field(s) [ChapTargetUsername ChapTargetInitiatorSecret]")

	// add a specific ChapTargetUsername
	config.ChapUsername = "unchanged ChapUsername"
	config.ChapInitiatorSecret = "unchanged ChapInitiatorSecret"
	config.ChapTargetUsername = "unchanged ChapTargetUsername"
	_, err = ValidateBidrectionalChapCredentials(defaultAuthNone, config)
	assert.EqualError(t, err, "missing value for required field(s) [ChapTargetInitiatorSecret]")

	// add a specific ChapTargetInitiatorSecret
	config.ChapUsername = "unchanged ChapUsername"
	config.ChapInitiatorSecret = "unchanged ChapInitiatorSecret"
	config.ChapTargetUsername = "unchanged ChapTargetUsername"
	config.ChapTargetInitiatorSecret = "unchanged ChapTargetInitiatorSecret"
	credentials, err = ValidateBidrectionalChapCredentials(defaultAuthNone, config)
	assert.Equal(t, nil, err)
	assert.Equal(t, "unchanged ChapUsername", credentials.ChapUsername)
	assert.Equal(t, "unchanged ChapInitiatorSecret", credentials.ChapInitiatorSecret)
	assert.Equal(t, "unchanged ChapTargetUsername", credentials.ChapTargetUsername)
	assert.Equal(t, "unchanged ChapTargetInitiatorSecret", credentials.ChapTargetInitiatorSecret)
}

// TestCHAP2 tests that we honor the auth type deny
func TestCHAP2(t *testing.T) {
	config := newTestOntapSANConfig()
	config.UseCHAP = true

	var err error

	defaultAuthDeny := newIscsiInitiatorGetDefaultAuthResponse("deny")

	// error if auth type is deny
	_, err = ValidateBidrectionalChapCredentials(defaultAuthDeny, config)
	assert.EqualError(t, err, "default initiator's auth type is deny")
}

// TestCHAP3 tests that we error on unexpected values
func TestCHAP3(t *testing.T) {
	config := newTestOntapSANConfig()
	config.UseCHAP = true

	var err error

	// error if auth type is an unexpected value
	_, err = ValidateBidrectionalChapCredentials(nil, config)
	assert.EqualError(t, err, "error checking default initiator's auth type: response is nil")

	// error if auth type is an unexpected value
	defaultAuthUnsupported := newIscsiInitiatorGetDefaultAuthResponse("unsupported")

	_, err = ValidateBidrectionalChapCredentials(defaultAuthUnsupported, config)
	assert.EqualError(t, err, "default initiator's auth type is unsupported")

	// error if auth type is an unexpected value
	defaultAuthUnsupported = newIscsiInitiatorGetDefaultAuthResponse("")

	_, err = ValidateBidrectionalChapCredentials(defaultAuthUnsupported, config)
	assert.EqualError(t, err, "default initiator's auth type is unsupported")
}

// TestCHAP4 tests that CHAP credentials match existing SVM CHAP usernames
func TestCHAP4(t *testing.T) {
	config := newTestOntapSANConfig()
	config.UseCHAP = true

	var err error

	defaultAuthCHAP := newIscsiInitiatorGetDefaultAuthResponseCHAP("CHAP", "", "")

	// start off missing all 4 values and validate that it generates an error
	_, err = ValidateBidrectionalChapCredentials(defaultAuthCHAP, config)
	assert.EqualError(t, err, "missing value for required field(s) [ChapUsername ChapInitiatorSecret ChapTargetUsername ChapTargetInitiatorSecret]")

	// add a specific ChapTargetInitiatorSecret
	config.ChapUsername = "aChapUsername"
	config.ChapInitiatorSecret = "aChapInitiatorSecret"
	config.ChapTargetUsername = "aChapTargetUsername"
	config.ChapTargetInitiatorSecret = "aChapTargetInitiatorSecret"
	_, err = ValidateBidrectionalChapCredentials(defaultAuthCHAP, config)
	assert.EqualError(t, err, "provided CHAP usernames do not match default initiator's usernames")

	defaultAuthCHAP = newIscsiInitiatorGetDefaultAuthResponseCHAP("CHAP", "aChapUsername", "aChapTargetUsername")
	_, err = ValidateBidrectionalChapCredentials(defaultAuthCHAP, config)
	assert.Equal(t, nil, err)
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

func TestValidateStoragePrefix(t *testing.T) {

	var storagePrefixTests = []struct {
		storagePrefix   string
		expected        bool
		expectedEconomy bool
	}{
		{"+abcd_123_ABC", false, false},
		{"1abcd_123_ABC", false, true},
		{"_abcd_123_ABC", true, true},
		{"abcd_123_ABC", true, true},
		{"ABCD_123_abc", true, true},
		{"abcd+123_ABC", false, false},
		{"abcd-123", true, true},
		{"abc.", true, true},
		{"a", true, true},
		{"1", false, true},
		{"_", true, true},
		{"-", true, true},
		{":", false, false},
		{".", true, true},
		{"", true, true},
	}

	for _, spt := range storagePrefixTests {

		isValid := ValidateStoragePrefix(spt.storagePrefix) == nil
		assert.Equal(t, spt.expected, isValid)

		isValid = ValidateStoragePrefixEconomy(spt.storagePrefix) == nil
		assert.Equal(t, spt.expectedEconomy, isValid)
	}
}

func TestOntapCalculateOptimalFlexVolSize(t *testing.T) {
	tests := []struct {
		name                      string
		flexvol                   string
		newQtreeSize              uint64
		totalDiskLimit            uint64
		percentageSnapshotReserve int
		sizeUsedBySnapshots       int
		expectedFlexvolSize       uint64
	}{
		{
			name:                      "3 gb snapshot add 1 gb",
			newQtreeSize:              1073741824,
			totalDiskLimit:            1073741824,
			percentageSnapshotReserve: 0,
			sizeUsedBySnapshots:       3158216704,
			expectedFlexvolSize:       5305700352,
		},
		{
			name:                      "0 gb snapshot add 4 gb",
			newQtreeSize:              1073741824,
			totalDiskLimit:            3221225472,
			percentageSnapshotReserve: 0,
			sizeUsedBySnapshots:       0,
			expectedFlexvolSize:       4294967296,
		},
		{
			name:                      "1 gb snapshot add 1 gb 20 snap reserve",
			newQtreeSize:              1073741824,
			totalDiskLimit:            1073741824,
			percentageSnapshotReserve: 20,
			sizeUsedBySnapshots:       983871488,
			expectedFlexvolSize:       3131355136,
		},
		{
			name:                      "1 gb snapshot add 7 gb 20 snap reserve",
			newQtreeSize:              7516192768,
			totalDiskLimit:            1073741824,
			percentageSnapshotReserve: 20,
			sizeUsedBySnapshots:       1022541824,
			expectedFlexvolSize:       10737418240,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			volAttrs := &azgo.VolumeAttributesType{
				VolumeSpaceAttributesPtr: &azgo.VolumeSpaceAttributesType{
					PercentageSnapshotReservePtr: &test.percentageSnapshotReserve,
					SizeUsedBySnapshotsPtr:       &test.sizeUsedBySnapshots,
				},
			}
			newFlexvolSize := calculateOptimalSizeForFlexvol(context.Background(), test.flexvol, volAttrs,
				test.newQtreeSize, test.totalDiskLimit)
			assert.Equal(t, test.expectedFlexvolSize, newFlexvolSize)
		})
	}
}
