// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"bytes"
	"context"
	"math/rand"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/logger"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

const (
	ONTAPTEST_LOCALHOST         = "127.0.0.1"
	ONTAPTEST_SERVER_MIN_PORT   = 40000
	ONTAPTEST_SERVER_MAX_PORT   = 50000
	ONTAPTEST_VSERVER_AGGR_NAME = "data"
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

	config.ManagementLIF = ONTAPTEST_LOCALHOST
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

func TestOntapSanInitializeDriverIgroupNameCSI(t *testing.T) {
	ctx := context.Background()
	logger.Logc(ctx).Level = log.TraceLevel

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := strconv.Itoa(rand.Intn(ONTAPTEST_SERVER_MAX_PORT-ONTAPTEST_SERVER_MIN_PORT) +
		ONTAPTEST_SERVER_MIN_PORT)
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	server := newUnstartedVserver(ctx, vserverAdminHost, vserverAdminPort, vserverAggrName)
	server.StartTLS()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			log.Error("Panic in fake filer", r)
		}
	}()

	cases := [][]string{
		// default igroup names with new 2 installations
		{
			"",
			"",
		},
		// one old installation and one new
		{
			"trident",
			"",
		},
		// 2 backends with user defined igroup names
		{
			"custom1",
			"custom2",
		},
		// one old installation, one custom
		{
			"trident",
			"custom",
		},
	}

	for _, igroupNames := range cases {

		var ontapSanDrivers []SANStorageDriver
		var expectedIgroupNames []string
		var backendUUIDs []string

		for i, igroupName := range igroupNames {
			/* this call assumes:
			1. The driver API will log methods and apis
			2. CSI driver context
			3. the config's backendUUID is set to a unique string
			*/
			sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName)
			sanStorageDriver.Config.IgroupName = igroupName
			sanStorageDriver.Config.DriverContext = tridentconfig.ContextCSI
			ontapSanDrivers = append(ontapSanDrivers, *sanStorageDriver)

			backendUUIDs = append(backendUUIDs, uuid.New().String())

			expectedIgroupName := igroupName
			if expectedIgroupName == "" {
				expectedIgroupName = "trident-" + backendUUIDs[i]
			}
			expectedIgroupNames = append(expectedIgroupNames, expectedIgroupName)
		}

		igroupNameMap := map[string]struct{}{}
		for _, v := range expectedIgroupNames {
			assert.NotContains(t, igroupNameMap, v, "Igroup name not unique!")
			igroupNameMap[v] = struct{}{}
		}

		for i, ontapSanDriver := range ontapSanDrivers {

			output := captureOutput(func() {
				_ = InitializeSANDriver(ctx, ontapSanDriver.Config.DriverContext, ontapSanDriver.API,
					&ontapSanDriver.Config, func(context.Context) error { return nil }, backendUUIDs[i])
			})

			assert.Contains(t, output, "<initiator-group-name>"+expectedIgroupNames[i]+"</initiator-group-name>",
				"Logs do not contain correct igroup name")
		}
	}
}

func TestOntapSanEcoInitializeDriverIgroupNameCSI(t *testing.T) {
	ctx := context.Background()
	logger.Logc(ctx).Level = log.TraceLevel

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := strconv.Itoa(rand.Intn(ONTAPTEST_SERVER_MAX_PORT-ONTAPTEST_SERVER_MIN_PORT) +
		ONTAPTEST_SERVER_MIN_PORT)
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	server := newUnstartedVserver(ctx, vserverAdminHost, vserverAdminPort, vserverAggrName)
	server.StartTLS()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			log.Error("Panic in fake filer", r)
		}
	}()

	cases := [][]string{
		// default igroup names with new 2 installations
		{
			"",
			"",
		},
		// one old installation and one new
		{
			"trident",
			"",
		},
		// 2 backends with user defined igroup names
		{
			"custom1",
			"custom2",
		},
		// one old installation, one custom
		{
			"trident",
			"custom",
		},
	}

	for _, igroupNames := range cases {

		var ontapSanDrivers []SANEconomyStorageDriver
		var expectedIgroupNames []string
		var backendUUIDs []string

		for i, igroupName := range igroupNames {
			/* this call assumes:
			1. The driver API will log methods and apis
			2. CSI driver context
			3. the config's backendUUID is set to a unique string
			*/
			sanStorageDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName)
			sanStorageDriver.Config.IgroupName = igroupName
			sanStorageDriver.Config.DriverContext = tridentconfig.ContextCSI
			ontapSanDrivers = append(ontapSanDrivers, *sanStorageDriver)

			backendUUIDs = append(backendUUIDs, uuid.New().String())

			expectedIgroupName := igroupName
			if expectedIgroupName == "" {
				expectedIgroupName = "trident-" + backendUUIDs[i]
			}
			expectedIgroupNames = append(expectedIgroupNames, expectedIgroupName)
		}

		igroupNameMap := map[string]struct{}{}
		for _, v := range expectedIgroupNames {
			assert.NotContains(t, igroupNameMap, v, "Igroup name not unique!")
			igroupNameMap[v] = struct{}{}
		}

		for i, ontapSanDriver := range ontapSanDrivers {

			output := captureOutput(func() {
				_ = InitializeSANDriver(ctx, ontapSanDriver.Config.DriverContext, ontapSanDriver.API,
					&ontapSanDriver.Config, func(context.Context) error { return nil }, backendUUIDs[i])
			})

			assert.Contains(t, output, "<initiator-group-name>"+expectedIgroupNames[i]+"</initiator-group-name>",
				"Logs do not contain correct igroup name")
		}
	}
}

func TestOntapSanInitializeDriverIgroupNameDocker(t *testing.T) {
	ctx := context.Background()
	logger.Logc(ctx).Level = log.TraceLevel

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := strconv.Itoa(rand.Intn(ONTAPTEST_SERVER_MAX_PORT-ONTAPTEST_SERVER_MIN_PORT) +
		ONTAPTEST_SERVER_MIN_PORT)
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	server := newUnstartedVserver(ctx, vserverAdminHost, vserverAdminPort, vserverAggrName)
	server.StartTLS()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			log.Error("Panic in fake filer", r)
		}
	}()

	cases := [][]string{
		// default igroup names with new 2 installations
		{
			"",
			"",
		},
		// one old installation and one new
		{
			drivers.GetDefaultIgroupName(tridentconfig.ContextDocker),
			"",
		},
		// 2 backends with user defined igroup names
		{
			"custom1",
			"custom2",
		},
		// one old installation, one custom
		{
			drivers.GetDefaultIgroupName(tridentconfig.ContextDocker),
			"custom",
		},
	}

	for _, igroupNames := range cases {

		var ontapSanDrivers []SANStorageDriver
		var expectedIgroupNames []string
		var backendUUIDs []string

		for _, igroupName := range igroupNames {
			/* this call assumes:
			1. The driver API will log methods and apis
			2. CSI driver context
			3. the config's backendUUID is set to a unique string
			*/
			sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName)
			sanStorageDriver.Config.IgroupName = igroupName
			sanStorageDriver.Config.DriverContext = tridentconfig.ContextDocker
			ontapSanDrivers = append(ontapSanDrivers, *sanStorageDriver)

			backendUUIDs = append(backendUUIDs, uuid.New().String())

			expectedIgroupName := igroupName
			if expectedIgroupName == "" {
				expectedIgroupName = "netappdvp"
			}
			expectedIgroupNames = append(expectedIgroupNames, expectedIgroupName)
		}

		for i, ontapSanDriver := range ontapSanDrivers {

			output := captureOutput(func() {
				_ = InitializeSANDriver(ctx, ontapSanDriver.Config.DriverContext, ontapSanDriver.API,
					&ontapSanDriver.Config, func(context.Context) error { return nil }, backendUUIDs[i])
			})

			assert.Contains(t, output, "<initiator-group-name>"+expectedIgroupNames[i]+"</initiator-group-name>",
				"Logs do not contain correct igroup name")
		}
	}
}

func TestOntapSanEcoInitializeDriverIgroupNameDocker(t *testing.T) {
	ctx := context.Background()
	logger.Logc(ctx).Level = log.TraceLevel

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := strconv.Itoa(rand.Intn(ONTAPTEST_SERVER_MAX_PORT-ONTAPTEST_SERVER_MIN_PORT) +
		ONTAPTEST_SERVER_MIN_PORT)
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	server := newUnstartedVserver(ctx, vserverAdminHost, vserverAdminPort, vserverAggrName)
	server.StartTLS()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			log.Error("Panic in fake filer", r)
		}
	}()

	cases := [][]string{
		// default igroup names with new 2 installations
		{
			"",
			"",
		},
		// one old installation and one new
		{
			drivers.GetDefaultIgroupName(tridentconfig.ContextDocker),
			"",
		},
		// 2 backends with user defined igroup names
		{
			"custom1",
			"custom2",
		},
		// one old installation, one custom
		{
			drivers.GetDefaultIgroupName(tridentconfig.ContextDocker),
			"custom",
		},
	}

	for _, igroupNames := range cases {

		var ontapSanDrivers []SANEconomyStorageDriver
		var expectedIgroupNames []string
		var backendUUIDs []string

		for _, igroupName := range igroupNames {
			/* this call assumes:
			1. The driver API will log methods and apis
			2. CSI driver context
			3. the config's backendUUID is set to a unique string
			*/
			sanStorageDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName)
			sanStorageDriver.Config.IgroupName = igroupName
			sanStorageDriver.Config.DriverContext = tridentconfig.ContextDocker
			ontapSanDrivers = append(ontapSanDrivers, *sanStorageDriver)

			backendUUIDs = append(backendUUIDs, uuid.New().String())

			expectedIgroupName := igroupName
			if expectedIgroupName == "" {
				expectedIgroupName = "netappdvp"
			}
			expectedIgroupNames = append(expectedIgroupNames, expectedIgroupName)
		}

		for i, ontapSanDriver := range ontapSanDrivers {

			output := captureOutput(func() {
				_ = InitializeSANDriver(ctx, ontapSanDriver.Config.DriverContext, ontapSanDriver.API,
					&ontapSanDriver.Config, func(context.Context) error { return nil }, backendUUIDs[i])
			})

			assert.Contains(t, output, "<initiator-group-name>"+expectedIgroupNames[i]+"</initiator-group-name>",
				"Logs do not contain correct igroup name")
		}
	}
}

func TestOntapSanGetDefaultIgroupName(t *testing.T) {
	ctx := context.Background()
	logger.Logc(ctx).Level = log.TraceLevel

	cases := []struct {
		driverContext      tridentconfig.DriverContext
		backendUUID        string
		expectedIgroupName string
	}{
		{
			tridentconfig.ContextCSI,
			"UNIQUE-CSI-UUID",
			"trident-UNIQUE-CSI-UUID",
		},
		{
			tridentconfig.ContextKubernetes,
			"UNIQUE-KUBERNETES-UUID",
			"trident",
		},
		{
			tridentconfig.ContextDocker,
			"UNIQUE-DOCKER-UUID",
			"netappdvp",
		},
	}

	for _, c := range cases {
		actualIgroupName := getDefaultIgroupName(c.driverContext, c.backendUUID)
		assert.Equal(t, c.expectedIgroupName, actualIgroupName, "Unexpected igroupName")

	}

}

func TestGetExternalConfigRedactSecrets(t *testing.T) {

	var cases = []struct {
		Name           string
		originalConfig drivers.OntapStorageDriverConfig
		externalConfig drivers.OntapStorageDriverConfig
		errorMessage   string
	}{
		{
			Name: "CHAP credentials provided",
			originalConfig: drivers.OntapStorageDriverConfig{
				Username:                  "test-username",
				Password:                  "test-password",
				ClientPrivateKey:          "test-client-private-key",
				ChapInitiatorSecret:       "test-chap-initiator-secret",
				ChapTargetInitiatorSecret: "test-chap-target-initiator-secret",
				ChapTargetUsername:        "test-chap-target-username",
				ChapUsername:              "test-chap-username",
			},
			externalConfig: drivers.OntapStorageDriverConfig{
				Username:                  "<REDACTED>",
				Password:                  "<REDACTED>",
				ClientPrivateKey:          "<REDACTED>",
				ChapInitiatorSecret:       "<REDACTED>",
				ChapTargetInitiatorSecret: "<REDACTED>",
				ChapTargetUsername:        "<REDACTED>",
				ChapUsername:              "<REDACTED>",
			},
			errorMessage: "sensitive information not redacted correctly",
		},
		{
			Name: "CHAP credentials not provided",
			originalConfig: drivers.OntapStorageDriverConfig{
				Username:                  "",
				Password:                  "",
				ClientPrivateKey:          "",
				ChapInitiatorSecret:       "",
				ChapTargetInitiatorSecret: "",
				ChapTargetUsername:        "",
				ChapUsername:              "",
			},
			externalConfig: drivers.OntapStorageDriverConfig{
				Username:                  "<REDACTED>",
				Password:                  "<REDACTED>",
				ClientPrivateKey:          "<REDACTED>",
				ChapInitiatorSecret:       "<REDACTED>",
				ChapTargetInitiatorSecret: "<REDACTED>",
				ChapTargetUsername:        "<REDACTED>",
				ChapUsername:              "<REDACTED>",
			},
			errorMessage: "sensitive information not redacted correctly",
		},
	}

	for _, c := range cases {
		c := c // capture range variable
		t.Run(c.Name, func(t *testing.T) {

			externalConfig := getExternalConfig(context.TODO(), c.originalConfig)

			assert.Equal(t, c.externalConfig, externalConfig, c.errorMessage)

		})
	}
}
