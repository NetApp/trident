// Copyright 2020 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	sfapi "github.com/netapp/trident/storage_drivers/solidfire/api"
	"github.com/netapp/trident/utils/errors"
)

func TestGetDriverConfigByName(t *testing.T) {
	tests := []struct {
		driverName           string
		expectedDriverConfig DriverConfig
		errorExpected        bool
	}{
		{
			driverName:           config.OntapNASStorageDriverName,
			expectedDriverConfig: &OntapStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.OntapNASQtreeStorageDriverName,
			expectedDriverConfig: &OntapStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.OntapSANEconomyStorageDriverName,
			expectedDriverConfig: &OntapStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.OntapNASFlexGroupStorageDriverName,
			expectedDriverConfig: &OntapStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.SolidfireSANStorageDriverName,
			expectedDriverConfig: &SolidfireStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.AzureNASStorageDriverName,
			expectedDriverConfig: &AzureNASStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.GCNVNASStorageDriverName,
			expectedDriverConfig: &GCNVNASStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           config.FakeStorageDriverName,
			expectedDriverConfig: &FakeStorageDriverConfig{},
			errorExpected:        false,
		},
		{
			driverName:           "invalid driver",
			expectedDriverConfig: nil,
			errorExpected:        true,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s", test.driverName), func(t *testing.T) {
			driverConfig, err := GetDriverConfigByName(test.driverName)
			assert.Equal(t, test.expectedDriverConfig, driverConfig)
			if test.errorExpected {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestCommonStorageDriverConfig_String(t *testing.T) {
	user := "user"
	pass := "pass"
	config := &CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "common",
		BackendName:       "common",
		Debug:             true,
		DebugTraceFlags:   map[string]bool{},
		DisableDelete:     true,
		StoragePrefixRaw:  json.RawMessage("trident"),
		StoragePrefix:     func(s string) *string { return &s }("trident"),
		Credentials: map[string]string{
			"Username": user,
			"Password": pass,
		},
	}
	configString := config.String()
	assert.NotContains(t, configString, user)
	assert.NotContains(t, configString, pass)
}

func TestCommonStorageDriverConfig_SetBackendName(t *testing.T) {
	backendName := "common-driver"
	config := &CommonStorageDriverConfig{
		BackendName: "",
	}
	config.SetBackendName(backendName)
	assert.Equal(t, config.BackendName, backendName)
}

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
	config.ClientPrivateKey = "client-private-key"
	return config
}

func TestOntapStorageDriverConfig_String(t *testing.T) {
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

func TestOntapStorageDriverConfig_GoString(t *testing.T) {
	config := newTestOntapStorageDriverConfig(map[string]bool{"method": true})
	configGoString := config.GoString()
	assert.NotContains(t, configGoString, config.Username)
	assert.NotContains(t, configGoString, config.Password)
	assert.NotContains(t, configGoString, config.ChapUsername)
	assert.NotContains(t, configGoString, config.ChapInitiatorSecret)
	assert.NotContains(t, configGoString, config.ChapTargetUsername)
	assert.NotContains(t, configGoString, config.ChapTargetInitiatorSecret)
	assert.NotContains(t, configGoString, config.ClientPrivateKey)
}

func TestOntapStorageDriverConfig_InjectSecrets(t *testing.T) {
	// configMap for holding a config struct data as a map.
	var configMap map[string]interface{}

	// UseChap defaults to false
	config := &OntapStorageDriverConfig{
		Username:         "Username",
		Password:         "Password",
		ClientPrivateKey: "ClientPrivateKey",
	}
	secretMap := map[string]string{
		"clientprivatekey":          "clientPrivateKey",
		"username":                  "username",
		"password":                  "password",
		"chapusername":              "chapUsername",
		"chapinitiatorsecret":       "chapInitiatorSecret",
		"chaptargetusername":        "chapTargetUsername",
		"chaptargetinitiatorsecret": "chapTargetInitiatorSecret",
	}
	assert.NoError(t, config.InjectSecrets(secretMap))

	// Convert the current config to a map.
	data, _ := json.Marshal(&config)
	json.Unmarshal(data, &configMap)

	// Look through each field in secretMap. If it exists in the config, the value
	// should have the same value of that key in secretMap.
	for key, expectedValue := range secretMap {
		if actualInterface, ok := configMap[key]; ok {
			assert.Equal(t, expectedValue, actualInterface)
		}
	}

	// UseChap == true, so the "chap" fields in the secret map should also exist.
	config = &OntapStorageDriverConfig{
		UseCHAP: true,
	}

	// ordered list of keys to ensure we always hit all coverage blocks in InjectSecrets.
	secretList := []string{
		"chapusername",
		"chapinitiatorsecret",
		"chaptargetusername",
		"chaptargetinitiatorSecret",
		"", // This accounts for an off by 1 error in the loop below.
	}
	forceFailuresMap := make(map[string]string, 0)

	// Loop through the secrets we need in the config and inject a map containing each
	// expected secret 1 at a time to force a failure path.
	for _, secret := range secretList {
		assert.Error(t, config.InjectSecrets(forceFailuresMap))
		forceFailuresMap[secret] = secret
	}
}

func TestOntapStorageDriverConfig_ExtractSecrets(t *testing.T) {
	fieldMap := map[string]string{
		"ClientPrivateKey":          "clientPrivateKey",
		"Username":                  "username",
		"Password":                  "password",
		"ChapUsername":              "chapUsername",
		"ChapInitiatorSecret":       "chapInitiatorSecret",
		"ChapTargetUsername":        "chapTargetUsername",
		"ChapTargetInitiatorSecret": "chapTargetInitiatorSecret",
	}

	// This behavior will need to be repeated.
	assertions := func(secretMap map[string]string) {
		for key, expectedValue := range fieldMap {
			if actualValue, ok := secretMap[key]; ok {
				assert.Equal(t, expectedValue, actualValue)
			}
		}
	}

	config := OntapStorageDriverConfig{
		Username:         fieldMap["Username"],
		Password:         fieldMap["Password"],
		ClientPrivateKey: fieldMap["ClientPrivateKey"],
	}
	secretMap := config.ExtractSecrets()
	assertions(secretMap)

	// UseChap == true
	config = OntapStorageDriverConfig{
		Username:                  fieldMap["Username"],
		Password:                  fieldMap["Password"],
		ClientPrivateKey:          fieldMap["ClientPrivateKey"],
		UseCHAP:                   true,
		ChapUsername:              fieldMap["ChapUsername"],
		ChapInitiatorSecret:       fieldMap["ChapInitiatorSecret"],
		ChapTargetUsername:        fieldMap["ChapTargetUsername"],
		ChapTargetInitiatorSecret: fieldMap["ChapTargetInitiatorSecret"],
	}
	secretMap = config.ExtractSecrets()
	assertions(secretMap)
}

func TestOntapStorageDriverConfig_ResetSecrets(t *testing.T) {
	secretName := "keep-it-secret-keep-it-safe"
	fieldMap := map[string]string{
		"ClientPrivateKey":          "clientPrivateKey",
		"Username":                  "username",
		"Password":                  "password",
		"ChapUsername":              "chapUsername",
		"ChapInitiatorSecret":       "chapInitiatorSecret",
		"ChapTargetUsername":        "chapTargetUsername",
		"ChapTargetInitiatorSecret": "chapTargetInitiatorSecret",
	}

	// This behavior will need to be repeated.
	assertions := func(config *OntapStorageDriverConfig) {
		// Convert the current config to a map.
		var configMap map[string]interface{}
		data, _ := json.Marshal(&config)
		json.Unmarshal(data, &configMap)

		// Look through each field in fieldMap. If it exists in the config, the value
		// should be empty
		for key := range fieldMap {
			if actualInterface, ok := configMap[key]; ok {
				assert.Equal(t, "", actualInterface)
			}
		}
	}

	config := &OntapStorageDriverConfig{
		Username:         secretName,
		Password:         "",
		ClientPrivateKey: secretName,
	}
	config.ResetSecrets()
	assertions(config)

	// UseChap == true
	config = &OntapStorageDriverConfig{
		Username:                  secretName,
		Password:                  secretName,
		ClientPrivateKey:          secretName,
		UseCHAP:                   true,
		ChapUsername:              secretName,
		ChapInitiatorSecret:       secretName,
		ChapTargetUsername:        secretName,
		ChapTargetInitiatorSecret: secretName,
	}
	config.ResetSecrets()
	assertions(config)
}

func TestOntapStorageDriverConfig_HideSensitiveWithSecretName(t *testing.T) {
	secretName := "keep-it-secret-keep-it-safe"
	fieldMap := map[string]string{
		"ClientPrivateKey":          "clientPrivateKey",
		"Username":                  "username",
		"Password":                  "password",
		"ChapUsername":              "chapUsername",
		"ChapInitiatorSecret":       "chapInitiatorSecret",
		"ChapTargetUsername":        "chapTargetUsername",
		"ChapTargetInitiatorSecret": "chapTargetInitiatorSecret",
	}

	// This behavior will need to be repeated.
	assertions := func(config *OntapStorageDriverConfig) {
		// Convert the current config to a map.
		var configMap map[string]interface{}
		data, _ := json.Marshal(&config)
		json.Unmarshal(data, &configMap)

		// Look through each field in fieldMap. If it exists in the config, the value
		// should be obfuscated to be the value of secretName
		for key := range fieldMap {
			if actualInterface, ok := configMap[key]; ok {
				assert.Equal(t, secretName, actualInterface)
			}
		}
	}

	config := &OntapStorageDriverConfig{
		Username:         fieldMap["Username"],
		Password:         fieldMap["Password"],
		ClientPrivateKey: fieldMap["ClientPrivateKey"],
	}
	config.HideSensitiveWithSecretName(secretName)
	assertions(config)

	// UseChap == true
	config = &OntapStorageDriverConfig{
		Username:                  fieldMap["Username"],
		Password:                  fieldMap["Password"],
		ClientPrivateKey:          fieldMap["ClientPrivateKey"],
		UseCHAP:                   true,
		ChapUsername:              fieldMap["ChapUsername"],
		ChapInitiatorSecret:       fieldMap["ChapInitiatorSecret"],
		ChapTargetUsername:        fieldMap["ChapTargetUsername"],
		ChapTargetInitiatorSecret: fieldMap["ChapTargetInitiatorSecret"],
	}
	config.HideSensitiveWithSecretName(secretName)
	assertions(config)
}

func TestOntapStorageDriverConfig_GetAndHideSensitive(t *testing.T) {
	secretName := "keep-it-secret-keep-it-safe"
	fieldMap := map[string]string{
		"ClientPrivateKey":          "clientPrivateKey",
		"Username":                  "username",
		"Password":                  "password",
		"ChapUsername":              "chapUsername",
		"ChapInitiatorSecret":       "chapInitiatorSecret",
		"ChapTargetUsername":        "chapTargetUsername",
		"ChapTargetInitiatorSecret": "chapTargetInitiatorSecret",
	}

	// This behavior will need to be repeated.
	assertions := func(config *OntapStorageDriverConfig, secretName string, secretMap map[string]string) {
		// All fields in the secret map should exist in the secretMap
		for key, expectedValue := range fieldMap {
			if actualValue, ok := secretMap[key]; ok {
				assert.Equal(t, expectedValue, actualValue)
			}
		}

		// After calling GetAndHideSensitive, secrets on the config will be obfuscated.
		// Convert the current config to a map.
		var configMap map[string]interface{}
		data, _ := json.Marshal(&config)
		json.Unmarshal(data, &configMap)

		// Look through each field in fieldMap. If it exists in the config, the value
		// should be obfuscated to be the value of secretName
		for key := range fieldMap {
			if actualInterface, ok := configMap[key]; ok {
				assert.Equal(t, secretName, actualInterface)
			}
		}
	}

	config := &OntapStorageDriverConfig{
		Username:         fieldMap["Username"],
		Password:         fieldMap["Password"],
		ClientPrivateKey: fieldMap["ClientPrivateKey"],
	}
	secretMap := config.GetAndHideSensitive(secretName)
	assertions(config, secretName, secretMap)

	// UseChap == true
	config = &OntapStorageDriverConfig{
		Username:                  fieldMap["Username"],
		Password:                  fieldMap["Password"],
		ClientPrivateKey:          fieldMap["ClientPrivateKey"],
		UseCHAP:                   true,
		ChapUsername:              fieldMap["ChapUsername"],
		ChapInitiatorSecret:       fieldMap["ChapInitiatorSecret"],
		ChapTargetUsername:        fieldMap["ChapTargetUsername"],
		ChapTargetInitiatorSecret: fieldMap["ChapTargetInitiatorSecret"],
	}
	config.GetAndHideSensitive(secretName)
	assertions(config, secretName, secretMap)
}

func TestOntapStorageDriverConfig_CheckForCRDControllerForbiddenAttributes(t *testing.T) {
	fieldMap := map[string]string{
		"ClientPrivateKey":          "clientPrivateKey",
		"Username":                  "username",
		"Password":                  "password",
		"ChapUsername":              "chapUsername",
		"ChapInitiatorSecret":       "chapInitiatorSecret",
		"ChapTargetUsername":        "chapTargetUsername",
		"ChapTargetInitiatorSecret": "chapTargetInitiatorSecret",
	}

	// This behavior will need to be repeated.
	assertions := func(secretMap map[string]string, forbiddenList []string) {
		// All fields in the secret map should exist in the forbiddenList
		for key := range secretMap {
			assert.Contains(t, forbiddenList, key)
		}
	}

	config := &OntapStorageDriverConfig{
		Username:         fieldMap["Username"],
		Password:         fieldMap["Password"],
		ClientPrivateKey: fieldMap["ClientPrivateKey"],
	}
	secretMap := config.ExtractSecrets()
	forbiddenList := config.CheckForCRDControllerForbiddenAttributes()
	assertions(secretMap, forbiddenList)

	// UseChap == true
	config = &OntapStorageDriverConfig{
		Username:                  fieldMap["Username"],
		Password:                  fieldMap["Password"],
		ClientPrivateKey:          fieldMap["ClientPrivateKey"],
		UseCHAP:                   true,
		ChapUsername:              fieldMap["ChapUsername"],
		ChapInitiatorSecret:       fieldMap["ChapInitiatorSecret"],
		ChapTargetUsername:        fieldMap["ChapTargetUsername"],
		ChapTargetInitiatorSecret: fieldMap["ChapTargetInitiatorSecret"],
	}
	secretMap = config.ExtractSecrets()
	forbiddenList = config.CheckForCRDControllerForbiddenAttributes()
	assertions(secretMap, forbiddenList)
}

func TestOntapStorageDriverConfig_SpecOnlyValidation(t *testing.T) {
	fieldMap := map[string]string{
		"ClientPrivateKey":          "clientPrivateKey",
		"Username":                  "username",
		"Password":                  "password",
		"ChapUsername":              "chapUsername",
		"ChapInitiatorSecret":       "chapInitiatorSecret",
		"ChapTargetUsername":        "chapTargetUsername",
		"ChapTargetInitiatorSecret": "chapTargetInitiatorSecret",
	}

	// This behavior will need to be repeated.
	assertions := func(expectToFail bool, error error) {
		if expectToFail {
			assert.Error(t, error)
		} else {
			assert.NoError(t, error)
		}
	}

	// Config has secrets but no common credentials map.
	config := OntapStorageDriverConfig{
		Username:         fieldMap["Username"],
		Password:         fieldMap["Password"],
		ClientPrivateKey: fieldMap["ClientPrivateKey"],
	}
	err := config.SpecOnlyValidation()
	assertions(true, err)

	// Config has no Credentials or forbiddenSecrets
	config = OntapStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{},
		},
	}
	err = config.SpecOnlyValidation()
	assertions(true, err)

	// Config only has credentials.
	config = OntapStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
	}
	err = config.SpecOnlyValidation()
	assertions(false, err)
}

// newTestSolidfireStorageDriverConfig creates a SolidfireStorageDriverConfig for testing.
func newTestSolidfireStorageDriverConfig() *SolidfireStorageDriverConfig {
	config := &SolidfireStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &CommonStorageDriverConfig{
		Credentials: map[string]string{
			"username": "",
			"password": "",
		},
	}
	config.CommonStorageDriverConfig.DebugTraceFlags = map[string]bool{"method": true}
	config.TenantName = "tenant"
	config.EndPoint = "0987654321567890"
	config.SVIP = "65432123456"
	config.InitiatorIFace = "65432123456"
	config.Types = &[]sfapi.VolType{}
	config.LegacyNamePrefix = "nfs-mount-options"
	config.AccessGroups = []int64{}
	config.UseCHAP = true
	config.DefaultBlockSize = int64(100)
	config.StoragePrefix = sp("solidfire_")
	return config
}

func TestSolidfireStorageDriverConfig_String(t *testing.T) {
	config := newTestSolidfireStorageDriverConfig()
	tenantName := config.TenantName
	endPoint := config.EndPoint
	configGoString := config.String()
	assert.NotContains(t, configGoString, tenantName)
	assert.NotContains(t, configGoString, endPoint)
}

func TestSolidfireStorageDriverConfig_GoString(t *testing.T) {
	config := newTestSolidfireStorageDriverConfig()
	tenantName := config.TenantName
	endPoint := config.EndPoint
	configGoString := config.GoString()
	assert.NotContains(t, configGoString, tenantName)
	assert.NotContains(t, configGoString, endPoint)
}

func TestSolidfireStorageDriverConfig_InjectSecrets(t *testing.T) {
	tests := []struct {
		secretMap   map[string]string
		errorExists bool
	}{
		{
			secretMap: map[string]string{
				"endpoint": "test",
			},
			errorExists: false,
		},
		{
			secretMap: map[string]string{
				"end_point": "test",
			},
			errorExists: true,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			config := SolidfireStorageDriverConfig{}
			err := config.InjectSecrets(test.secretMap)
			if test.errorExists {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSolidfireStorageDriverConfig_ExtractSecrets(t *testing.T) {
	endPoint := "/endpoint"
	config := SolidfireStorageDriverConfig{
		EndPoint: endPoint,
	}

	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["EndPoint"]; ok {
		assert.Equal(t, endPoint, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestSolidfireStorageDriverConfig_ResetSecrets(t *testing.T) {
	endPoint := "/endpoint"
	config := SolidfireStorageDriverConfig{
		EndPoint: endPoint,
	}

	config.ResetSecrets()
	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["EndPoint"]; ok {
		assert.NotEqual(t, endPoint, val)
		assert.Equal(t, "", val)
	} else {
		t.Fatal("KVP is invalid")
	}
}

func TestSolidfireStorageDriverConfig_HideSensitiveWithSecretName(t *testing.T) {
	endPoint := "/endpoint"
	config := SolidfireStorageDriverConfig{
		EndPoint: endPoint,
	}

	obfuscatedSecret := "<REDACTED>"
	config.HideSensitiveWithSecretName(obfuscatedSecret)
	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["EndPoint"]; ok {
		assert.NotEqual(t, endPoint, val)
		assert.Equal(t, obfuscatedSecret, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestSolidfireStorageDriverConfig_GetAndHideSensitive(t *testing.T) {
	endPoint := "/endpoint"
	config := SolidfireStorageDriverConfig{
		EndPoint: endPoint,
	}

	obfuscatedSecret := "<REDACTED>"
	secretMap := config.GetAndHideSensitive(obfuscatedSecret)

	if val, ok := secretMap["EndPoint"]; ok {
		assert.Equal(t, endPoint, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestSolidfireStorageDriverConfig_CheckForCRDControllerForbiddenAttributes(t *testing.T) {
	endPoint := "/endpoint"
	config := SolidfireStorageDriverConfig{
		EndPoint: endPoint,
	}

	forbiddenKeys := config.CheckForCRDControllerForbiddenAttributes()
	assert.Contains(t, forbiddenKeys, "EndPoint")
}

func TestSolidfireStorageDriverConfig_SpecOnlyValidation(t *testing.T) {
	endPoint := "/endpoint"
	config := SolidfireStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
	}
	err := config.SpecOnlyValidation()
	assert.NoError(t, err)

	config = SolidfireStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
		EndPoint: endPoint,
	}
	err = config.SpecOnlyValidation()
	assert.Error(t, err)

	config = SolidfireStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{},
		},
	}
	err = config.SpecOnlyValidation()
	assert.Error(t, err)
}

// newTestAzureNASStorageDriverConfig creates a AzureNASStorageDriverConfig for testing.
func newTestAzureNASStorageDriverConfig() *AzureNASStorageDriverConfig {
	config := &AzureNASStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &CommonStorageDriverConfig{
		Credentials: map[string]string{
			"username": "",
			"password": "",
		},
	}
	config.CommonStorageDriverConfig.DebugTraceFlags = map[string]bool{"method": true}
	config.SubscriptionID = "1234567890987654321234567890"
	config.TenantID = "0987654321567890"
	config.ClientID = "65432123456"
	config.ClientSecret = "65432123456"
	config.Location = "east-us"
	config.NfsMountOptions = "nfs-mount-options"
	config.VolumeCreateTimeout = "100"
	config.SDKTimeout = "10"
	config.MaxCacheAge = "1000"
	config.StorageDriverName = "anf"
	config.StoragePrefix = sp("anf_")
	return config
}

func TestAzureNASStorageDriverConfig_String(t *testing.T) {
	config := newTestAzureNASStorageDriverConfig()
	subscriptionID := config.SubscriptionID
	tenantID := config.TenantID
	clientID := config.ClientID
	clientSecret := config.ClientSecret
	configString := config.String()
	assert.NotContains(t, configString, subscriptionID)
	assert.NotContains(t, configString, tenantID)
	assert.NotContains(t, configString, clientID)
	assert.NotContains(t, configString, clientSecret)
}

func TestAzureNASStorageDriverConfig_GoString(t *testing.T) {
	config := newTestAzureNASStorageDriverConfig()
	subscriptionID := config.SubscriptionID
	tenantID := config.TenantID
	clientID := config.ClientID
	clientSecret := config.ClientSecret
	configGoString := config.GoString()
	assert.NotContains(t, configGoString, subscriptionID)
	assert.NotContains(t, configGoString, tenantID)
	assert.NotContains(t, configGoString, clientID)
	assert.NotContains(t, configGoString, clientSecret)
}

func TestAzureNASStorageDriverConfig_InjectSecrets(t *testing.T) {
	tests := []struct {
		secretMap   map[string]string
		errorExists bool
	}{
		{
			secretMap: map[string]string{
				"clientid":     "test",
				"clientsecret": "test",
			},
			errorExists: false,
		},
		{
			secretMap: map[string]string{
				"client_id":    "test",
				"clientsecret": "test",
			},
			errorExists: true,
		},
		{
			secretMap: map[string]string{
				"clientid":      "test",
				"client_secret": "test",
			},
			errorExists: true,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			config := AzureNASStorageDriverConfig{}
			err := config.InjectSecrets(test.secretMap)
			if test.errorExists {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAzureNASStorageDriverConfig_ExtractSecrets(t *testing.T) {
	privateKey := "1234567890987654321"
	privateKeyID := "1234567890987654321"
	config := AzureNASStorageDriverConfig{
		ClientID:     privateKey,
		ClientSecret: privateKeyID,
	}

	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["ClientID"]; ok {
		assert.Equal(t, privateKey, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}

	if val, ok := secretMap["ClientSecret"]; ok {
		assert.Equal(t, privateKeyID, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestAzureNASStorageDriverConfig_ResetSecrets(t *testing.T) {
	privateKey := "1234567890987654321"
	privateKeyID := "1234567890987654321"
	config := AzureNASStorageDriverConfig{
		ClientID:     privateKey,
		ClientSecret: privateKeyID,
	}

	config.ResetSecrets()
	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["ClientID"]; ok {
		assert.NotEqual(t, privateKey, val)
		assert.Equal(t, "", val)
	} else {
		t.Fatal("KVP is invalid")
	}

	if val, ok := secretMap["ClientSecret"]; ok {
		assert.NotEqual(t, privateKeyID, val)
		assert.Equal(t, "", val)
	} else {
		t.Fatal("KVP is invalid")
	}
}

func TestAzureNASStorageDriverConfig_HideSensitiveWithSecretName(t *testing.T) {
	privateKey := "1234567890987654321"
	privateKeyID := "1234567890987654321"
	config := AzureNASStorageDriverConfig{
		ClientID:     privateKey,
		ClientSecret: privateKeyID,
	}

	obfuscatedSecret := "<REDACTED>"
	config.HideSensitiveWithSecretName(obfuscatedSecret)
	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["ClientID"]; ok {
		assert.NotEqual(t, privateKey, val)
		assert.Equal(t, obfuscatedSecret, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}

	if val, ok := secretMap["ClientSecret"]; ok {
		assert.NotEqual(t, privateKeyID, val)
		assert.Equal(t, obfuscatedSecret, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestAzureNASStorageDriverConfig_GetAndHideSensitive(t *testing.T) {
	privateKey := "1234567890987654321"
	privateKeyID := "1234567890987654321"
	config := AzureNASStorageDriverConfig{
		ClientID:     privateKey,
		ClientSecret: privateKeyID,
	}

	obfuscatedSecret := "<REDACTED>"
	secretMap := config.GetAndHideSensitive(obfuscatedSecret)

	if val, ok := secretMap["ClientID"]; ok {
		assert.Equal(t, privateKey, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}

	if val, ok := secretMap["ClientSecret"]; ok {
		assert.Equal(t, privateKeyID, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestAzureNASStorageDriverConfig_CheckForCRDControllerForbiddenAttributes(t *testing.T) {
	config := AzureNASStorageDriverConfig{
		ClientID:     "1234567890987654321",
		ClientSecret: "1234567890987654321",
	}

	forbiddenKeys := config.CheckForCRDControllerForbiddenAttributes()
	assert.Contains(t, forbiddenKeys, "ClientID")
	assert.Contains(t, forbiddenKeys, "ClientSecret")
}

func TestAzureNASStorageDriverConfig_SpecOnlyValidation(t *testing.T) {
	config := AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
	}
	err := config.SpecOnlyValidation()
	assert.NoError(t, err)

	config = AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
		ClientID:     "1234567890987654321",
		ClientSecret: "1234567890987654321",
	}
	err = config.SpecOnlyValidation()
	assert.Error(t, err)

	config = AzureNASStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{},
		},
	}
	err = config.SpecOnlyValidation()
	assert.Nil(t, err)
}

// newTestStorageDriverConfig creates a FakeStorageDriverConfig for testing.
func newTestFakeStorageDriverConfig() *FakeStorageDriverConfig {
	fakeConfig := &FakeStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Version:           ConfigVersion,
			StorageDriverName: config.FakeStorageDriverName,
			DebugTraceFlags:   map[string]bool{},
		},
		Protocol:     config.File,
		InstanceName: "fake-instance",
		Username:     "fake-user",
		Password:     "fake-password",
	}

	return fakeConfig
}

func TestFakeStorageDriverConfig_String(t *testing.T) {
	config := newTestFakeStorageDriverConfig()
	user := config.Username
	pass := config.Password
	configString := config.String()
	assert.NotContains(t, configString, user)
	assert.NotContains(t, configString, pass)
}

func TestFakeStorageDriverConfig_GoString(t *testing.T) {
	config := newTestFakeStorageDriverConfig()
	user := config.Username
	pass := config.Password
	configGoString := config.GoString()
	assert.NotContains(t, configGoString, user)
	assert.NotContains(t, configGoString, pass)
}

func TestFakeStorageDriverConfig_InjectSecrets(t *testing.T) {
	config := newTestFakeStorageDriverConfig()
	err := config.InjectSecrets(map[string]string{
		"kubeconfig": "kubeconfig-01",
	})
	assert.NoError(t, err)
}

func TestFakeStorageDriverConfig_ExtractSecrets(t *testing.T) {
	user := "user"
	pass := "pass"
	config := FakeStorageDriverConfig{
		Username: user,
		Password: pass,
	}

	secretMap := config.ExtractSecrets()
	if val, ok := secretMap["Username"]; ok {
		assert.Equal(t, user, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}

	if val, ok := secretMap["Password"]; ok {
		assert.Equal(t, pass, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestFakeStorageDriverConfig_ResetSecrets(t *testing.T) {
	user := "user"
	pass := "pass"
	config := FakeStorageDriverConfig{
		Username: user,
		Password: pass,
	}

	config.ResetSecrets()
	secretMap := config.ExtractSecrets()

	if val, ok := secretMap["Username"]; ok {
		assert.NotEqual(t, user, val)
		assert.Equal(t, "", val)
	} else {
		t.Fatal("KVP is invalid")
	}

	if val, ok := secretMap["Password"]; ok {
		assert.NotEqual(t, pass, val)
		assert.Equal(t, "", val)
	} else {
		t.Fatal("KVP is invalid")
	}
}

func TestFakeStorageDriverConfig_HideSensitiveWithSecretName(t *testing.T) {
	user := "user"
	pass := "pass"
	config := FakeStorageDriverConfig{
		Username: user,
		Password: pass,
	}

	secretMap := config.ExtractSecrets()
	obfuscatedSecret := "<REDACTED>"
	config.HideSensitiveWithSecretName(obfuscatedSecret)

	if val, ok := secretMap["Username"]; ok {
		assert.Equal(t, user, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}

	if val, ok := secretMap["Password"]; ok {
		assert.Equal(t, pass, val)
	} else {
		t.Fatal("KVP does not exist in secret map")
	}
}

func TestFakeStorageDriverConfig_GetAndHideSensitive(t *testing.T) {
	config := FakeStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
	}
	secretMap := config.GetAndHideSensitive("anything")

	// No secrets to hide for the fake driver!
	assert.Empty(t, secretMap)
}

func TestFakeStorageDriverConfig_CheckForCRDControllerForbiddenAttributes(t *testing.T) {
	user := "user"
	pass := "pass"
	config := FakeStorageDriverConfig{
		Username: user,
		Password: pass,
	}

	forbiddenKeys := config.CheckForCRDControllerForbiddenAttributes()
	assert.Contains(t, forbiddenKeys, "Username")
	assert.Contains(t, forbiddenKeys, "Password")
}

func TestFakeStorageDriverConfig_SpecOnlyValidation(t *testing.T) {
	user := "user"
	pass := "pass"
	config := FakeStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
	}

	err := config.SpecOnlyValidation()
	assert.NoError(t, err)

	config = FakeStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{
				"Username": "user",
				"Password": "pass",
			},
		},
		Username: user,
		Password: pass,
	}
	err = config.SpecOnlyValidation()
	assert.Error(t, err)

	config = FakeStorageDriverConfig{
		CommonStorageDriverConfig: &CommonStorageDriverConfig{
			Credentials: map[string]string{},
		},
	}
	err = config.SpecOnlyValidation()
	assert.Error(t, err)
}

func TestBackendIneligibleError(t *testing.T) {
	// NewBackendIneligibleError
	volumeName := "volume"
	errList := []error{
		errors.New("one"),
		errors.New("two"),
	}
	ineligiblePhysicalPoolNames := []string{
		"pool_one",
		"pool_two",
	}
	backendIneligibleError := NewBackendIneligibleError(volumeName, errList, ineligiblePhysicalPoolNames)

	// Error
	for _, err := range errList {
		assert.Contains(t, backendIneligibleError.Error(), err.Error())
	}

	// IsBackendIneligibleError
	var err error
	assert.False(t, IsBackendIneligibleError(err))

	err = errors.New("one")
	assert.False(t, IsBackendIneligibleError(err))

	backendIneligibleError = NewBackendIneligibleError(volumeName, errList, ineligiblePhysicalPoolNames)
	assert.True(t, IsBackendIneligibleError(backendIneligibleError))

	// GetIneligiblePhysicalPoolNames
	err, ineligiblePhysicalPools := GetIneligiblePhysicalPoolNames(backendIneligibleError)
	assert.EqualValues(t, ineligiblePhysicalPoolNames, ineligiblePhysicalPools)
	assert.NoError(t, err)

	err, ineligiblePhysicalPools = GetIneligiblePhysicalPoolNames(errors.New(""))
	assert.Empty(t, ineligiblePhysicalPools)
	assert.Error(t, err)
}

func TestVolumeExistsError(t *testing.T) {
	// Error
	msg := "test"
	volumeExistsErr := NewVolumeExistsError(msg)
	volumeExistsErrMsg := volumeExistsErr.Error()
	expectedErrMsg := fmt.Sprintf("volume %s already exists", msg)
	assert.Equal(t, expectedErrMsg, volumeExistsErrMsg)

	// IsVolumeExistsError
	tests := []struct {
		inputError  error
		errorExists bool
	}{
		{
			inputError:  NewVolumeExistsError("error"),
			errorExists: true,
		},
		{
			inputError:  errors.New("error"),
			errorExists: false,
		},
		{
			inputError:  nil,
			errorExists: false,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			isVolumeExistsError := IsVolumeExistsError(test.inputError)
			assert.Equal(t, test.errorExists, isVolumeExistsError)
		})
	}
}

func TestInjectionError(t *testing.T) {
	injectionError := injectionError("test")
	assert.NotNil(t, injectionError)
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

func TestCheckMapContainsAttributes(t *testing.T) {
	tests := []struct {
		forbiddenMap map[string]string
		expectedList []string
	}{
		{
			forbiddenMap: map[string]string{
				"one":   "value",
				"two":   "value",
				"three": "",
			},
			expectedList: []string{
				"one",
				"two",
			},
		},
		{
			forbiddenMap: map[string]string{
				"key": "",
			},
			expectedList: nil,
		},
		{
			forbiddenMap: nil,
			expectedList: nil,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			forbiddenList := checkMapContainsAttributes(test.forbiddenMap)
			sort.Strings(test.expectedList)
			sort.Strings(forbiddenList)
			assert.EqualValues(t, test.expectedList, forbiddenList)
		})
	}
}
