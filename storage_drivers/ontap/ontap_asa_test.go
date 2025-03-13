// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	restAPIModels "github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

func getASAVolumeConfig() storage.VolumeConfig {
	return storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "true",
		FileSystem:   "xfs",
		Name:         "vol1",
		InternalName: "trident-pvc-1234",
	}
}

func newMockOntapASADriver(t *testing.T) (*mockapi.MockOntapAPI, *ASAStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapASADriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}

	return mockAPI, driver
}

func cloneTestOntapASADriver(driver *ASAStorageDriver) *ASAStorageDriver {
	clone := *driver
	return &clone
}

func newTestOntapASADriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, apiOverride api.OntapAPI,
) *ASAStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true
	// config.Labels = map[string]string{"app": "wordpress"}
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-asa-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = convert.ToPtr("test_")

	asaDriver := &ASAStorageDriver{}
	asaDriver.Config = *config

	var ontapAPI api.OntapAPI

	if apiOverride != nil {
		ontapAPI = apiOverride
	} else {
		// ZAPI is not supported in ASA driver. Return Rest Client all the time.
		ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
	}

	asaDriver.API = ontapAPI
	asaDriver.telemetry = &Telemetry{
		Plugin:        asaDriver.Name(),
		StoragePrefix: *asaDriver.Config.StoragePrefix,
		Driver:        asaDriver,
	}

	return asaDriver
}

func TestGetConfigASA(t *testing.T) {
	expectedConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san",
			DebugTraceFlags:   map[string]bool{"method": true},
		},
		ManagementLIF: "10.0.0.1",
		SVM:           "svm0",
		Username:      "admin",
		Password:      "password",
	}
	driver := &ASAStorageDriver{
		Config: expectedConfig,
	}
	actualConfig := driver.GetConfig()
	assert.Equal(t, &expectedConfig, actualConfig, "The returned configuration should match the expected configuration")
}

func TestGetTelemetryASA(t *testing.T) {
	expectedTelemetry := &Telemetry{
		Telemetry: tridentconfig.Telemetry{
			TridentVersion: tridentconfig.OrchestratorAPIVersion,
			Platform:       "linux",
		},
		Plugin:        "ontap-san",
		SVM:           "svm0",
		StoragePrefix: "trident_",
		stopped:       false,
	}
	driver := &ASAStorageDriver{
		telemetry: expectedTelemetry,
	}
	actualTelemetry := driver.GetTelemetry()
	assert.Equal(t, expectedTelemetry, actualTelemetry, "The returned telemetry should match the expected telemetry")
}

func TestBackendName(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	tests := []struct {
		name           string
		backendName    string
		ips            []string
		expectedResult string
	}{
		{
			name:           "EmptyConfigName_ReturnsOldNamingScheme",
			backendName:    "",
			ips:            []string{"127.0.0.1"},
			expectedResult: "ontapasa_127.0.0.1",
		},
		{
			name:           "EmptyConfigName_NoLIFs_ReturnsNoLIFs",
			backendName:    "",
			ips:            []string{},
			expectedResult: "ontapasa_noLIFs",
		},
		{
			name:           "ConfigNameProvided_ReturnsConfigName",
			backendName:    "customBackendName",
			ips:            []string{},
			expectedResult: "customBackendName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver.ips = tt.ips
			driver.Config.BackendName = tt.backendName
			actual := driver.BackendName()
			assert.Equal(t, tt.expectedResult, actual, "Expected backend name to match")
		})
	}
}

func TestGetExternalConfigASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san",
			DebugTraceFlags:   map[string]bool{"method": true},
			Credentials:       map[string]string{"key": "value"},
		},
		ManagementLIF:             "10.0.0.1",
		SVM:                       "svm0",
		Username:                  "admin",
		Password:                  "password",
		ClientPrivateKey:          "privateKey",
		ChapInitiatorSecret:       "chapInitiatorSecret",
		ChapTargetInitiatorSecret: "chapTargetInitiatorSecret",
		ChapTargetUsername:        "chapTargetUsername",
		ChapUsername:              "chapUsername",
		UseREST:                   convert.ToPtr(true),
	}
	driver.Config = config
	externalConfig := driver.GetExternalConfig(ctx)

	expectedConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san",
			DebugTraceFlags:   map[string]bool{"method": true},
			StoragePrefixRaw:  json.RawMessage("{}"),
			Credentials:       map[string]string{drivers.KeyName: tridentconfig.REDACTED, drivers.KeyType: tridentconfig.REDACTED},
		},
		ManagementLIF:             "10.0.0.1",
		SVM:                       "svm0",
		Username:                  tridentconfig.REDACTED,
		Password:                  tridentconfig.REDACTED,
		ClientPrivateKey:          tridentconfig.REDACTED,
		ChapInitiatorSecret:       tridentconfig.REDACTED,
		ChapTargetInitiatorSecret: tridentconfig.REDACTED,
		ChapTargetUsername:        tridentconfig.REDACTED,
		ChapUsername:              tridentconfig.REDACTED,
		UseREST:                   convert.ToPtr(true),
	}

	assert.Equal(t, expectedConfig, externalConfig, "The returned external configuration should match the expected configuration")
}

func TestGetProtocolASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	actualProtocol := driver.GetProtocol(ctx)
	expectedProtocol := tridentconfig.Block
	assert.Equal(t, expectedProtocol, actualProtocol, "The returned protocol should be Block")
}

func TestStoreConfigASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san-economy",
			DebugTraceFlags:   map[string]bool{"method": true},
			Credentials:       map[string]string{"key": "value"},
		},
		ManagementLIF:             "10.0.0.1",
		SVM:                       "svm0",
		Username:                  "admin",
		Password:                  "password",
		ClientPrivateKey:          "privateKey",
		ChapInitiatorSecret:       "chapInitiatorSecret",
		ChapTargetInitiatorSecret: "chapTargetInitiatorSecret",
		ChapTargetUsername:        "chapTargetUsername",
		ChapUsername:              "chapUsername",
		UseREST:                   convert.ToPtr(true),
	}
	driver.Config = config

	persistentConfig := &storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, persistentConfig)

	expectedConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san-economy",
			DebugTraceFlags:   map[string]bool{"method": true},
			StoragePrefixRaw:  json.RawMessage("{}"),
			Credentials:       map[string]string{"key": "value"},
		},
		ManagementLIF:             "10.0.0.1",
		SVM:                       "svm0",
		Username:                  "admin",
		Password:                  "password",
		ClientPrivateKey:          "privateKey",
		ChapInitiatorSecret:       "chapInitiatorSecret",
		ChapTargetInitiatorSecret: "chapTargetInitiatorSecret",
		ChapTargetUsername:        "chapTargetUsername",
		ChapUsername:              "chapUsername",
		UseREST:                   convert.ToPtr(true),
	}

	// Verify that the stored configuration matches the expected configuration
	assert.Equal(t, &expectedConfig, persistentConfig.OntapConfig, "The stored configuration should match the expected configuration")
}

func TestInitializeASA(t *testing.T) {
	driverContext := driverContextCSI

	var (
		mockAPI       *mockapi.MockOntapAPI
		driver        *ASAStorageDriver
		commonConfig  *drivers.CommonStorageDriverConfig
		backendSecret map[string]string
		backendUUID   string
		configJSON    []byte
	)

	mockAPI, driver = newMockOntapASADriver(t)

	initializeFunction := func() {
		driver.Config.DriverContext = driverContext
		commonConfig = driver.GetCommonConfig(ctx)
		backendSecret = map[string]string{}
		backendUUID = uuid.NewString()
		configJSON, _ = json.Marshal(driver.Config)
	}

	tests := []struct {
		name          string
		setupMocks    func()
		expectedError bool
		verify        func(*testing.T, error)
	}{
		{
			name: "CommonStorageDriverConfig not nil",
			setupMocks: func() {
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"127.0.0.1"}, nil).Times(1)
				mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(api.IscsiInitiatorAuth{AuthType: "none"}, nil).Times(1)
				mockAPI.EXPECT().GetSVMUUID().Return(uuid.NewString()).Times(1)
			},
			expectedError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during initialization")
				assert.NotNil(t, driver.telemetry, "Telemetry shouldn't be nil")
				assert.Equal(t, backendUUID, driver.telemetry.TridentBackendUUID)
				assert.NotNil(t, driver.cloneSplitTimers, "Clone split timers shouldn't be nil")
				assert.True(t, driver.Initialized(), "Expected driver to be initialized")
			},
		},
		{
			name: "CommonStorageDriverConfig is nil",
			setupMocks: func() {
				driver.initialized = false
				driver.Config.CommonStorageDriverConfig = nil
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"127.0.0.1"}, nil).Times(1)
				mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(api.IscsiInitiatorAuth{AuthType: "none"}, nil).Times(1)
				mockAPI.EXPECT().GetSVMUUID().Return(uuid.NewString()).Times(1)
			},
			expectedError: false,
			verify: func(t *testing.T, err error) {
				driver.Config.CommonStorageDriverConfig = commonConfig
				assert.NoError(t, err, "Expected no error during initialization")
				assert.True(t, driver.Initialized(), "Expected driver to be initialized")
				assert.NotNil(t, driver.telemetry, "Telemetry shouldn't be nil")
				assert.Equal(t, backendUUID, driver.telemetry.TridentBackendUUID)
			},
		},
		{
			name: "CommonStorageDriverConfig is nil and returns an error during initialization",
			setupMocks: func() {
				driver.initialized = false
				driver.Config.CommonStorageDriverConfig = nil
				backendSecret = map[string]string{
					"clientprivatekey": "not-nil",
					"username":         "ontap-asa-user",
				}
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				driver.Config.CommonStorageDriverConfig = commonConfig
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "PopulateASAConfigurationDefaults returns an error",
			setupMocks: func() {
				driver.initialized = false
				driver.Config.SplitOnClone = "not-boolean"
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				driver.Config.SplitOnClone = "false"
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "driver.API.NetInterfaceGetDataLIFs returns an error",
			setupMocks: func() {
				driver.initialized = false
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"127.0.0.1"}, fmt.Errorf("some-error")).Times(1)
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "d.ips are empty",
			setupMocks: func() {
				driver.initialized = false
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{}, nil).Times(1)
				mockAPI.EXPECT().SVMName().Return(driver.Config.SVM).Times(1)
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "InitializeASAStoragePoolsCommon returns an error",
			setupMocks: func() {
				driver.Config.SnapshotDir = "not-boolean"
				driver.initialized = false
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"127.0.0.1"}, nil).Times(1)
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				driver.Config.SnapshotDir = "false"
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "d.validate returns an error",
			setupMocks: func() {
				driver.Config.DriverContext = "docker"
				driver.initialized = false
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"127.0.0.1"}, nil).Times(1)
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				driver.Config.DriverContext = driverContext
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "d.validate returns an error and driverContext is CSI",
			setupMocks: func() {
				driver.initialized = false
				driver.Config.StoragePrefix = convert.ToPtr("#test_")
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"127.0.0.1"}, nil).Times(1)
				mockAPI.EXPECT().IgroupDestroy(ctx, driver.Config.IgroupName).Return(fmt.Errorf("api-error")).Times(1)
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				driver.Config.StoragePrefix = convert.ToPtr("test_")
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
		{
			name: "InitializeOntapDriver returns an error",
			setupMocks: func() {
				driver.initialized = false
				driver.API = nil
			},
			expectedError: true,
			verify: func(t *testing.T, err error) {
				driver.API = mockAPI
				assert.Error(t, err, "Expected error during initialization")
				assert.False(t, driver.Initialized(), "Expected driver to be not initialized")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			err := driver.Initialize(ctx, driverContext, string(configJSON), commonConfig, backendSecret, backendUUID)
			tt.verify(t, err)
		})
	}
}

func TestTerminate(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	tests := []struct {
		name          string
		driverContext tridentconfig.DriverContext
		telemetry     *Telemetry
		setupMocks    func()
	}{
		{
			name:          "DriverContextCSI_WithTelemetry",
			driverContext: tridentconfig.ContextCSI,
			telemetry:     &Telemetry{done: make(chan struct{})},
			setupMocks: func() {
				mockAPI.EXPECT().IgroupDestroy(ctx, driver.Config.IgroupName).Return(nil).Times(1)
			},
		},
		{
			name:          "DriverContextCSI_WithoutTelemetry",
			driverContext: tridentconfig.ContextCSI,
			telemetry:     nil,
			setupMocks: func() {
				mockAPI.EXPECT().IgroupDestroy(ctx, driver.Config.IgroupName).Return(nil).Times(1)
			},
		},
		{
			name:          "DriverContextCSI_WithoutTelemetry_IgroupDestory_Returns_error",
			driverContext: tridentconfig.ContextCSI,
			telemetry:     nil,
			setupMocks: func() {
				mockAPI.EXPECT().IgroupDestroy(ctx, driver.Config.IgroupName).Return(fmt.Errorf("api-error")).Times(1)
			},
		},
		{
			name:          "DriverContextDocker_WithTelemetry",
			driverContext: tridentconfig.ContextDocker,
			telemetry:     &Telemetry{done: make(chan struct{})},
		},
		{
			name:          "DriverContextDocker_WithoutTelemetry",
			driverContext: tridentconfig.ContextDocker,
			telemetry:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver.Config.DriverContext = tt.driverContext
			driver.telemetry = tt.telemetry

			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			driver.Terminate(ctx, "")

			if tt.telemetry != nil {
				assert.True(t, tt.telemetry.stopped, "Expected telemetry to be stopped")
			}
			assert.False(t, driver.initialized, "Expected driver to be not initialized")
		})
	}
}

func TestValidateASA(t *testing.T) {
	type testCase struct {
		name          string
		setupDriver   func(driver *ASAStorageDriver)
		expectedError bool
	}

	_, driver := newMockOntapASADriver(t)

	// Initialize the driver and mock API
	initializeDriver := func() {
		storagePrefix := "trident_"
		driver.Config.CommonStorageDriverConfig.StoragePrefix = &storagePrefix
		driver.ips = []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}
		driver.Config.AutoExportPolicy = true
		driver.Config.UseCHAP = true
		driver.Config.DataLIF = "1.1.1.1"

		_ = PopulateASAConfigurationDefaults(ctx, &driver.Config)
		driver.virtualPools = map[string]storage.Pool{}
		driver.physicalPools = map[string]storage.Pool{}
	}

	testCases := []testCase{
		{
			name: "Positive Case",
			setupDriver: func(driver *ASAStorageDriver) {
				// No additional setup needed for this case
			},
			expectedError: false,
		},
		{
			name: "validateSanDriver fails",
			setupDriver: func(driver *ASAStorageDriver) {
				driver.Config.SANType = sa.FCP
				driver.Config.UseCHAP = true
			},
			expectedError: true,
		},
		{
			name: "ValidateStoragePrefix fails",
			setupDriver: func(driver *ASAStorageDriver) {
				storagePrefix := "/s"
				driver.Config.StoragePrefix = &storagePrefix
			},
			expectedError: true,
		},
		{
			name: "ValidateASAStoragePool fails",
			setupDriver: func(driver *ASAStorageDriver) {
				driver.Config.NASType = sa.NFS
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initializeDriver()
			tc.setupDriver(driver)

			err := driver.validate(ctx)

			if tc.expectedError {
				assert.Error(t, err, "Expected an error during validation")
			} else {
				assert.NoError(t, err, "Expected no error during validation")
			}
		})
	}
}

func TestCreateASA(t *testing.T) {
	var (
		volConfig   *storage.VolumeConfig
		storagePool *storage.StoragePool
		volAttrs    map[string]sa.Request
	)

	volumeName := "testSU"
	labels := fmt.Sprintf(`{"provisioning":{"template":"%s"}}`, volumeName)

	mockAPI, driver := newMockOntapASADriver(t)

	capacityBytes, _ := capacity.ToBytes("2G")
	internalAttributesMap := map[string]string{
		SpaceAllocation:   "true",
		SpaceReserve:      "none",
		SnapshotPolicy:    "none",
		Encryption:        "true",
		SkipRecoveryQueue: "true",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    "false",
		FileSystemType:    "ext4",
		FormatOptions: strings.Join([]string{"-b 4096", "-T stirde=2056, stripe=16"},
			filesystem.FormatOptionsSeparator),
		Size: capacityBytes,
	}

	initializedFunction := func() {
		driver.Config.DriverContext = tridentconfig.ContextCSI
		volConfig = &storage.VolumeConfig{
			InternalName: volumeName,
			Name:         volumeName,
			Size:         "1G",
		}

		storagePool = storage.NewStoragePool(nil, "pool1")
		storagePool.SetInternalAttributes(internalAttributesMap)
		storagePool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
			"template": `{{.volume.Name}}`,
		})
		driver.physicalPools = map[string]storage.Pool{"pool1": storagePool}
		volAttrs = map[string]sa.Request{}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive case - LUN created successfully",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetComment(ctx, volumeName, labels).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetAttribute(ctx, volumeName, LUNAttributeFSType, storagePool.InternalAttributes()[FileSystemType], string(driver.Config.DriverContext), storagePool.InternalAttributes()[LUKSEncryption], storagePool.InternalAttributes()[FormatOptions]).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Should not be an error")
				// Assert whether volconfig gets proper updated with the values that were sent via storage pool.
				assert.Equal(t, storagePool.InternalAttributes()[SpaceReserve], volConfig.SpaceReserve)
				assert.Equal(t, storagePool.InternalAttributes()[SnapshotPolicy], volConfig.SnapshotPolicy)
				assert.Equal(t, storagePool.InternalAttributes()[Encryption], volConfig.Encryption)
				assert.Equal(t, storagePool.InternalAttributes()[QosPolicy], volConfig.QosPolicy)
				assert.Equal(t, storagePool.InternalAttributes()[LUKSEncryption], volConfig.LUKSEncryption)
				assert.Equal(t, storagePool.InternalAttributes()[FileSystemType], volConfig.FileSystem)
			},
		},
		{
			name: "Positive case - If size passed is zero, taking default size which was set in storage pool",
			setupMocks: func() {
				volConfig = &storage.VolumeConfig{
					InternalName: volumeName,
					Name:         volumeName,
					Size:         "0",
				}
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetComment(ctx, volumeName, labels).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetAttribute(ctx, volumeName, LUNAttributeFSType, storagePool.InternalAttributes()[FileSystemType], string(driver.Config.DriverContext), storagePool.InternalAttributes()[LUKSEncryption], storagePool.InternalAttributes()[FormatOptions]).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Should not be an error")
				assert.Equal(t, storagePool.InternalAttributes()[Size], volConfig.Size)
			},
		},
		{
			name: "Positive case - Verify labels are set correctly",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetComment(ctx, volumeName, labels).DoAndReturn(
					func(ctx context.Context, name, labels string) error {
						expectedLabels, _ := ConstructLabelsFromConfigs(ctx, storagePool, volConfig, driver.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
						assert.Equal(t, expectedLabels, labels, "Labels should match the expected value")
						return nil
					}).Times(1)
				mockAPI.EXPECT().LunSetAttribute(ctx, volumeName, LUNAttributeFSType, storagePool.InternalAttributes()[FileSystemType], string(driver.Config.DriverContext), storagePool.InternalAttributes()[LUKSEncryption], storagePool.InternalAttributes()[FormatOptions]).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Should not be an error")
			},
		},
		{
			name: "Negative case - LUN already exists",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(true, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Should be an error")
				assert.IsType(t, drivers.NewVolumeExistsError("testLUN"), err)
			},
		},
		{
			name: "Negative case - Error checking LUN existence",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, fmt.Errorf("error checking lun existence")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Should be an error")
				assert.Contains(t, err.Error(), "failure checking for existence of LUN")
			},
		},
		{
			name: "Negative case - QOSPolicyGroup creation returns an error",
			setupMocks: func() {
				internalAttributesMap[QosPolicy] = "some-qos-policy"
				internalAttributesMap[AdaptiveQosPolicy] = "some-adaptive-policy"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(true, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[QosPolicy] = "fake-qos-policy"
				delete(internalAttributesMap, AdaptiveQosPolicy)
				assert.Error(t, err, "Should be an error")
			},
		},
		{
			name: "Negative case - Error creating LUN",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(fmt.Errorf("API error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Should be an error")
				assert.Contains(t, err.Error(), "API error")
			},
		},
		{
			name: "Negative case - Error setting LUN comment",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetComment(ctx, volumeName, labels).Return(fmt.Errorf("API error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "error setting labels on the LUN")
			},
		},
		{
			name: "Negative case - Error setting LUN attribute",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetComment(ctx, volumeName, labels).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetAttribute(ctx, volumeName, LUNAttributeFSType,
					storagePool.InternalAttributes()[FileSystemType],
					string(driver.Config.DriverContext),
					storagePool.InternalAttributes()[LUKSEncryption],
					storagePool.InternalAttributes()[FormatOptions]).
					Return(fmt.Errorf("api-error")).Times(1)
				mockAPI.EXPECT().LunDestroy(ctx, volumeName).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Negative case - Space allocation set to false",
			setupMocks: func() {
				internalAttributesMap[SpaceAllocation] = "false"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[SpaceAllocation] = "true"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("spaceAllocation must be set to %s", DefaultSpaceAllocation))
			},
		},
		{
			name: "Negative case - Space Reserve not set to none",
			setupMocks: func() {
				volConfig.SpaceReserve = "volume"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.SpaceReserve = "none"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("spaceReserve must be set to %s", DefaultSpaceReserve))
			},
		},
		{
			name: "Negative case - Snapshot-policy not set to none",
			setupMocks: func() {
				volConfig.SnapshotPolicy = "not-none"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.SnapshotPolicy = "none"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("snapshotPolicy must be set to %s", DefaultSnapshotPolicy))
			},
		},
		{
			name: "Negative case - Snapshot-reserve is set",
			setupMocks: func() {
				volConfig.SnapshotReserve = "10"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.SnapshotReserve = ""
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "snapshotReserve must not be set")
			},
		},
		{
			name: "Negative case - Unix permission is set",
			setupMocks: func() {
				volConfig.UnixPermissions = "rwxrwxrwx"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.UnixPermissions = ""
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unixPermissions must not be set")
			},
		},
		{
			name: "Negative case - Export policy is set",
			setupMocks: func() {
				volConfig.ExportPolicy = "some-policy"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.ExportPolicy = ""
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "exportPolicy must not be set")
			},
		},
		{
			name: "Negative case - Security style is set",
			setupMocks: func() {
				volConfig.SecurityStyle = "mixed"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.ExportPolicy = ""
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "securityStyle must not be set")
			},
		},
		{
			name: "Negative case - Encryption is not set to default",
			setupMocks: func() {
				volConfig.Encryption = "false"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.ExportPolicy = DefaultASAEncryption
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("encryption must be set to %s", DefaultASAEncryption))
			},
		},
		{
			name: "Negative case - Tiering policy is set",
			setupMocks: func() {
				internalAttributesMap[TieringPolicy] = "some-tieringPolicy"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[TieringPolicy] = ""
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "tieringPolicy must not be set")
			},
		},
		{
			name: "Negative case - LUKSEncryption must be set to false",
			setupMocks: func() {
				internalAttributesMap[LUKSEncryption] = "true"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[LUKSEncryption] = "false"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("luksEncryption must be set to %s", DefaultLuksEncryption))
			},
		},
		{
			name: "Negative case - Capacity given is invalid",
			setupMocks: func() {
				volConfig.Size = "invalid-size"
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.Size = "1G"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "could not convert size")
			},
		},
		{
			name: "Negative case - Unsupported file system",
			setupMocks: func() {
				internalAttributesMap[FileSystemType] = "invalid-filesystem"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().LunExists(ctx, volumeName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[FileSystemType] = "ext4"
				volConfig.Size = "1G"
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializedFunction()
			tt.setupMocks()
			err := driver.Create(ctx, volConfig, storagePool, volAttrs)
			tt.verify(t, err)
		})
	}
}

func TestCreateCloneASA(t *testing.T) {
	var (
		cloneVolConfig storage.VolumeConfig
		storagePool    *storage.StoragePool
		sourceLun      *api.Lun
	)

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		cloneVolConfig = getASAVolumeConfig()
		cloneVolConfig.InternalName = "testVol"
		cloneVolConfig.CloneSourceVolumeInternal = "sourceVol"
		cloneVolConfig.CloneSourceSnapshotInternal = "sourceSnap"
		cloneVolConfig.QosPolicy = "mixed"

		storagePool = storage.NewStoragePool(nil, "pool1")
		storagePool.SetInternalAttributes(map[string]string{
			SplitOnClone: "false",
		})

		sourceLun = &api.Lun{
			Name: "sourceVol",
		}
	}

	expectedQOSPolicy := api.QosPolicyGroup{
		Name: "mixed",
		Kind: api.QosPolicyGroupKind,
	}

	tests := []struct {
		name             string
		setupMocks       func()
		verify           func(*testing.T, error)
		storagePoolIsNil bool
	}{
		{
			name: "Positive - Runs without any error",
			setupMocks: func() {
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
				mockAPI.EXPECT().LunExists(ctx, cloneVolConfig.InternalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal, cloneVolConfig.CloneSourceSnapshotInternal, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetQosPolicyGroup(ctx, cloneVolConfig.InternalName, expectedQOSPolicy).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during clone creation")
			},
		},
		{
			name: "Positive - SplitOnClone is enabled",
			setupMocks: func() {
				storagePool.SetInternalAttributes(map[string]string{
					SplitOnClone: "true",
				})
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
				mockAPI.EXPECT().LunExists(ctx, cloneVolConfig.InternalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal, cloneVolConfig.CloneSourceSnapshotInternal, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneSplitStart(ctx, cloneVolConfig.InternalName, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetQosPolicyGroup(ctx, cloneVolConfig.InternalName, expectedQOSPolicy).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				storagePool.SetInternalAttributes(map[string]string{
					SplitOnClone: "false",
				})
				assert.NoError(t, err, "Expected no error during clone creation")
			},
		},
		{
			name: "Negative - LunGetByName returns an error",
			setupMocks: func() {
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, fmt.Errorf("error-api")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - LunGetByName cannot find a LUN",
			setupMocks: func() {
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(nil, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - Wrong value of splitOnClone",
			setupMocks: func() {
				storagePool.SetInternalAttributes(map[string]string{
					SplitOnClone: "invalid",
				})
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - Error setting qosPolicyGroup",
			setupMocks: func() {
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
				mockAPI.EXPECT().LunExists(ctx, cloneVolConfig.InternalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal, cloneVolConfig.CloneSourceSnapshotInternal, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetQosPolicyGroup(ctx, cloneVolConfig.InternalName, expectedQOSPolicy).Return(fmt.Errorf("api-error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - Error setting lun comment",
			setupMocks: func() {
				longLabel := "thisIsATestLABEL"
				driver.Config.SplitOnClone = "false"
				driver.Config.Labels = map[string]string{
					longLabel: "dev-test-cluster-1",
				}
				storagePool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
				})
				expectedLabel := "{\"provisioning\":{\"thisIsATestLABEL\":\"dev-test-cluster-1\"}}"
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
				mockAPI.EXPECT().LunExists(ctx, cloneVolConfig.InternalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal, cloneVolConfig.CloneSourceSnapshotInternal, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().LunSetComment(ctx, cloneVolConfig.InternalName, expectedLabel).Return(fmt.Errorf("api-error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Clone of a volume shouldn't be created.")
			},
		},
		{
			name: "Negative - Label Length Exceeding",
			setupMocks: func() {
				longLabel := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
					"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ" +
					"bQBuHvTRNX6pHRKUXaIrjEpygM4SpaqHYdZ8O1k2meeugg7eXu4dPhqetI3Sip3W4v9QuFkh1YBaI" +
					"9sHE9w5eRxpmTv0POpCB5xAqzmN6XCkxuXKc4yfNS9PRwcTSpvkA3PcKCF3TD1TJU3NYzcChsFQgm" +
					"bAsR32cbJRdsOwx6BkHNfRCji0xSnBFUFUu1sGHfYCmzzd3OmChADIP6RwRtpnqNzvt0CU6uumBnl" +
					"Lc5U7mBI1Ndmqhn0BBSh588thKOQcpD4bvnSBYU788tBeVxQtE8KkdUgKl8574eWldqWDiALwoiCS" +
					"Ae2GuZzwG4ACw2uHdIkjb6FEwapSKCEogr4yWFAVCYPp2pA37Mj88QWN82BEpyoTV6BRAOsubNPfT" +
					"N94X0qCcVaQp4L5bA4SPTQu0ag20a2k9LmVsocy5y11U3ewpzVGtENJmxyuyyAbxOFOkDxKLRMhgs" +
					"uJMhhplD894tkEcPoiFhdsYZbBZ4MOBF6KkuBF5aqMrQbOCFt2vvTN843nRhomVMpY01SNuUeb5mh" +
					"UN53wsqqHSGoYb1eUBDlTUDLFcCcNacxfsILqmthnrD1B5u85jRm1SfkFfuIDOgaaTM9UhxNQ1U6M" +
					"mBaRYBkuGtTScoVTXyF4lij2sj1WWrKb7qWlaUUjxHiaxgLovPWErldCXXkNFsHgc7UYLQLF4j6lO" +
					"I1QdTAyrtCcSxRwdkjBxj8mQy1HblHnaaBwP7Nax9FvIvxpeqyD6s3X1vfFNGAMuRsc9DKmPDfxjh" +
					"qGzRQawFEbbURWij9xleKsUr0yCjukyKsxuaOlwbXnoFh4V3wtidrwrNXieFD608EANwvCp7u2S8Q" +
					"px99T4O87AdQGa5cAX8Ccojd9tENOmQRmOAwVEuFtuogos96TFlq0YHyfESDTB2TWayIuGJvgTIpX" +
					"lthQFQfHVgPpUZdzZMjXry"
				driver.Config.SplitOnClone = "false"
				driver.Config.Labels = map[string]string{
					longLabel: "dev-test-cluster-1",
				}
				storagePool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
				})
				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Clone of a volume shouldn't be created.")
			},
		},
		{
			name: "Negative - Name Template Label Length Exceeding",
			setupMocks: func() {
				longLabel := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
					"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ" +
					"bQBuHvTRNX6pHRKUXaIrjEpygM4SpaqHYdZ8O1k2meeugg7eXu4dPhqetI3Sip3W4v9QuFkh1YBaI" +
					"9sHE9w5eRxpmTv0POpCB5xAqzmN6XCkxuXKc4yfNS9PRwcTSpvkA3PcKCF3TD1TJU3NYzcChsFQgm" +
					"bAsR32cbJRdsOwx6BkHNfRCji0xSnBFUFUu1sGHfYCmzzd3OmChADIP6RwRtpnqNzvt0CU6uumBnl" +
					"Lc5U7mBI1Ndmqhn0BBSh588thKOQcpD4bvnSBYU788tBeVxQtE8KkdUgKl8574eWldqWDiALwoiCS" +
					"Ae2GuZzwG4ACw2uHdIkjb6FEwapSKCEogr4yWFAVCYPp2pA37Mj88QWN82BEpyoTV6BRAOsubNPfT" +
					"N94X0qCcVaQp4L5bA4SPTQu0ag20a2k9LmVsocy5y11U3ewpzVGtENJmxyuyyAbxOFOkDxKLRMhgs" +
					"uJMhhplD894tkEcPoiFhdsYZbBZ4MOBF6KkuBF5aqMrQbOCFt2vvTN843nRhomVMpY01SNuUeb5mh" +
					"UN53wsqqHSGoYb1eUBDlTUDLFcCcNacxfsILqmthnrD1B5u85jRm1SfkFfuIDOgaaTM9UhxNQ1U6M" +
					"mBaRYBkuGtTScoVTXyF4lij2sj1WWrKb7qWlaUUjxHiaxgLovPWErldCXXkNFsHgc7UYLQLF4j6lO" +
					"I1QdTAyrtCcSxRwdkjBxj8mQy1HblHnaaBwP7Nax9FvIvxpeqyD6s3X1vfFNGAMuRsc9DKmPDfxjh" +
					"qGzRQawFEbbURWij9xleKsUr0yCjukyKsxuaOlwbXnoFh4V3wtidrwrNXieFD608EANwvCp7u2S8Q" +
					"px99T4O87AdQGa5cAX8Ccojd9tENOmQRmOAwVEuFtuogos96TFlq0YHyfESDTB2TWayIuGJvgTIpX" +
					"lthQFQfHVgPpUZdzZMjXry"
				driver.Config.SplitOnClone = "false"
				driver.Config.Labels = map[string]string{
					longLabel: "dev-test-cluster-1",
				}

				storagePool.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
					"RequestName}}"
				storagePool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
				})

				driver.Config.NameTemplate = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"

				mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Clone of a volume shouldn't be created.")
			},
			storagePoolIsNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			var err error
			if tt.storagePoolIsNil {
				err = driver.CreateClone(ctx, nil, &cloneVolConfig, nil)
			} else {
				err = driver.CreateClone(ctx, nil, &cloneVolConfig, storagePool)
			}
			tt.verify(t, err)
		})
	}
}

func TestCreateCloneASA_NameTemplate(t *testing.T) {
	var (
		mockAPI        *mockapi.MockOntapAPI
		driver         *ASAStorageDriver
		cloneVolConfig storage.VolumeConfig
		storagePool    *storage.StoragePool
		sourceLun      *api.Lun
	)

	mockAPI, driver = newMockOntapASADriver(t)

	initializeFunction := func() {
		cloneVolConfig = getASAVolumeConfig()
		cloneVolConfig.CloneSourceVolumeInternal = "sourceVol"
		cloneVolConfig.CloneSourceSnapshotInternal = "sourceSnap"

		sourceLun = &api.Lun{
			Name: "sourceVol",
		}

		storagePool = storage.NewStoragePool(nil, "pool1")
		storagePool.SetInternalAttributes(map[string]string{
			SplitOnClone: "false",
		})
		driver.physicalPools = map[string]storage.Pool{"pool1": storagePool}
	}
	tests := []struct {
		labelTestName string
		labelKey      string
		labelValue    string
		expectedLabel string
	}{
		{
			"label", "nameTemplate", "dev-test-cluster-1",
			"{\"provisioning\":{\"nameTemplate\":\"dev-test-cluster-1\"}}",
		},
		{
			"emptyLabelKey", "", "dev-test-cluster-1",
			"{\"provisioning\":{\"\":\"dev-test-cluster-1\"}}",
		},
		{
			"emptyLabelValue", "nameTemplate", "",
			"{\"provisioning\":{\"nameTemplate\":\"\"}}",
		},
		{
			"labelValueSpecialCharacter", "nameTemplate^%^^\\u0026\\u0026^%_________", "dev-test-cluster-1%^%",
			"{\"provisioning\":{\"nameTemplate^%^^\\\\u0026\\\\u0026^%_________\":\"dev-test-cluster-1%^%\"}}",
		},
	}

	for _, test := range tests {
		t.Run(test.labelTestName, func(t *testing.T) {
			initializeFunction()
			mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(sourceLun, nil).Times(1)
			mockAPI.EXPECT().LunExists(ctx, cloneVolConfig.InternalName).Return(false, nil).Times(1)
			mockAPI.EXPECT().StorageUnitCloneCreate(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal, cloneVolConfig.CloneSourceSnapshotInternal, ASATypeLun).Return(nil).Times(1)
			mockAPI.EXPECT().LunSetComment(gomock.Any(), cloneVolConfig.InternalName, test.expectedLabel).Return(nil).Times(1)

			driver.Config.Labels = map[string]string{
				test.labelKey: test.labelValue,
			}
			storagePool.SetAttributes(map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
			})

			err := driver.CreateClone(ctx, nil, &cloneVolConfig, storagePool)

			assert.NoError(t, err, "Clone creation failed. Expected no error")
		})
	}
}

func TestRenameASA(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	testCases := []struct {
		name      string
		newName   string
		mockError error
		mocks     func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error)
		expectErr bool
	}{
		{
			name:      "valid rename",
			newName:   "newLUN",
			mockError: nil,
			mocks: func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error) {
				mockAPI.EXPECT().LunRename(ctx, name, newName).Return(err).Times(1)
			},
			expectErr: false,
		},
		{
			name:    "rename error",
			newName: "newLUN", mocks: func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error) {
				mockAPI.EXPECT().LunRename(ctx, name, newName).Return(err).Times(1)
			},
			mockError: fmt.Errorf("rename error"),
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mocks(mockAPI, tc.name, tc.newName, tc.mockError)

			err := driver.Rename(ctx, tc.name, tc.newName)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDestroyASA(t *testing.T) {
	var volConfig storage.VolumeConfig

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		driver.Config.DriverContext = tridentconfig.ContextCSI
		volConfig = storage.VolumeConfig{
			InternalName: "testLUN",
		}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive - LUN exists and is destroyed successfully",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunDestroy(ctx, volConfig.InternalName).Return(nil)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during LUN destruction")
			},
		},
		{
			name: "Positive - LUN does not exist",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error when LUN does not exist")
			},
		},
		{
			name: "Positive - Context is Docker, LUN exists and is destroyed successfully",
			setupMocks: func() {
				driver.Config.DriverContext = tridentconfig.ContextDocker

				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("nodeName", nil).Times(1)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, driver.Config.SVM).Return([]string{"iscsiInterfaces"}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, driver.Config.IgroupName, volConfig.InternalName).Return(1, nil).Times(1)
				mockAPI.EXPECT().LunExists(ctx, volConfig.InternalName).Return(true, nil).Times(1)
				mockAPI.EXPECT().LunDestroy(ctx, volConfig.InternalName).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during LUN destruction")
			},
		},
		{
			name: "Negative - Error checking for existing LUN",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volConfig.InternalName).Return(false, fmt.Errorf("error checking LUN"))
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "error checking for existing LUN")
			},
		},
		{
			name: "Negative - Error destroying LUN",
			setupMocks: func() {
				mockAPI.EXPECT().LunExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunDestroy(ctx, volConfig.InternalName).Return(fmt.Errorf("error destroying LUN"))
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "error destroying LUN")
			},
		},
		{
			name: "Negative - Context is Docker, LUN exists and LUNMapInfo returns an error",
			setupMocks: func() {
				driver.Config.DriverContext = tridentconfig.ContextDocker

				mockAPI.EXPECT().LunExists(ctx, volConfig.InternalName).Return(true, nil).Times(1)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("nodeName", nil).Times(1)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, driver.Config.SVM).Return([]string{"iscsiInterfaces"}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, driver.Config.IgroupName, volConfig.InternalName).Return(1, fmt.Errorf("error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error.")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			err := driver.Destroy(ctx, &volConfig)
			tt.verify(t, err)
		})
	}
}

func TestPublishASA(t *testing.T) {
	var (
		volConfig   storage.VolumeConfig
		publishInfo models.VolumePublishInfo
		lun         *api.Lun
		flexVol     *api.Volume
	)

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		volConfig = getASAVolumeConfig()
		volConfig.InternalName = "testVol"

		driver.Config.IgroupName = "testIgroup"

		publishInfo = models.VolumePublishInfo{
			HostName:    "testHost",
			TridentUUID: "testUUID",
			Nodes:       []*models.Node{{Name: "testHost"}},
		}

		flexVol = &api.Volume{
			Name:       "testVol",
			AccessType: "rw",
		}

		lun = &api.Lun{
			SerialNumber: uuid.NewString(),
		}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive - Runs without any error",
			setupMocks: func() {
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("nodeName", nil).Times(1)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, driver.Config.SVM).Return([]string{"iscsiInterfaces"}, nil).Times(1)
				mockAPI.EXPECT().LunGetFSType(ctx, volConfig.InternalName).Return("ext4", nil).Times(1)
				mockAPI.EXPECT().LunGetAttribute(ctx, volConfig.InternalName, "formatOptions").Return("formatOptions", nil).Times(1)
				mockAPI.EXPECT().LunGetByName(ctx, volConfig.InternalName).Return(lun, nil).Times(1)
				mockAPI.EXPECT().EnsureLunMapped(ctx, driver.Config.IgroupName, volConfig.InternalName).Return(1123, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during volume publish")
				assert.Equal(t, volConfig.AccessInfo, publishInfo.VolumeAccessInfo)
			},
		},
		{
			name: "Positive - Runs without any error and context set is csi",
			setupMocks: func() {
				tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
				mockAPI.EXPECT().IgroupCreate(ctx, getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID), driver.Config.SANType, "linux").DoAndReturn(func(arg0, arg1, arg2, arg3 any) error {
					tridentconfig.CurrentDriverContext = ""
					return nil
				}).Times(1)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("nodeName", nil).Times(1)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, driver.Config.SVM).Return([]string{"iscsiInterfaces"}, nil).Times(1)
				mockAPI.EXPECT().LunGetFSType(ctx, volConfig.InternalName).Return("lunFSType", nil).Times(1)
				mockAPI.EXPECT().LunGetAttribute(ctx, volConfig.InternalName, "formatOptions").Return("formatOptions", nil).Times(1)
				mockAPI.EXPECT().LunGetByName(ctx, volConfig.InternalName).Return(lun, nil).Times(1)
				mockAPI.EXPECT().EnsureLunMapped(ctx, getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID), volConfig.InternalName).Return(1123, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during volume publish")
				assert.Equal(t, volConfig.AccessInfo, publishInfo.VolumeAccessInfo)
			},
		},
		{
			name: "Negative - Volume not found",
			setupMocks: func() {
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(nil, fmt.Errorf("volume not found"))
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when volume is not found")
			},
		},
		{
			name: "Negative - Volume access type not rw",
			setupMocks: func() {
				flexVol.AccessType = "ro"
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when volume access type is not rw")
			},
		},
		{
			name: "Negative - Error getting target info",
			setupMocks: func() {
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("nodeName", fmt.Errorf("error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when getting target info fails")
			},
		},
		{
			name: "Negative - Error while Publishing",
			setupMocks: func() {
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("nodeName", nil).Times(1)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, driver.Config.SVM).Return([]string{"iscsiInterfaces"}, nil).Times(1)
				mockAPI.EXPECT().LunGetFSType(ctx, volConfig.InternalName).Return("lunFSType", nil).Times(1)
				mockAPI.EXPECT().LunGetAttribute(ctx, volConfig.InternalName, "formatOptions").Return("formatOptions", nil).Times(1)
				mockAPI.EXPECT().LunGetByName(ctx, volConfig.InternalName).Return(nil, fmt.Errorf("error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when getting target info fails")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			err := driver.Publish(ctx, &volConfig, &publishInfo)
			tt.verify(t, err)
		})
	}
}

func TestUnpublishASA(t *testing.T) {
	lunID := 1234

	type testCase struct {
		name          string
		setupMocks    func(*mockapi.MockOntapAPI)
		driverContext tridentconfig.DriverContext
		expectedError bool
		verify        func(*testing.T, error)
	}
	mockAPI, driver := newMockOntapASADriver(t)

	defer func(currentDriverContext tridentconfig.DriverContext) {
		tridentconfig.CurrentDriverContext = currentDriverContext
	}(tridentconfig.CurrentDriverContext)

	initializeFunction := func() (storage.VolumeConfig, models.VolumePublishInfo) {
		volConfig := getASAVolumeConfig()
		volConfig.InternalName = "testVol"
		publishInfo := models.VolumePublishInfo{
			HostName:    "testHost",
			TridentUUID: "testUUID",
		}
		volConfig.AccessInfo.IscsiIgroup = fmt.Sprintf(publishInfo.HostName + "-" + publishInfo.TridentUUID)
		return volConfig, publishInfo
	}

	tests := []testCase{
		{
			name: "Runs without any error",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunMapInfo(ctx, "testHost-testUUID", "testVol").Return(lunID, nil).Times(1)
				mockAPI.EXPECT().LunUnmap(ctx, "testHost-testUUID", "testVol").Return(nil).Times(1)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, "testHost-testUUID").Return([]string{}, nil).Times(1)
				mockAPI.EXPECT().IgroupDestroy(ctx, "testHost-testUUID").Return(nil).Times(1)
			},
			driverContext: tridentconfig.ContextCSI,
			expectedError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during volume unpublish")
			},
		},
		{
			name:          "Runs without any error when context is not CSI",
			setupMocks:    func(mockAPI *mockapi.MockOntapAPI) {},
			driverContext: tridentconfig.ContextDocker,
			expectedError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during volume unpublish when context is not CSI")
			},
		},
		{
			name: "Error unmapping LUN from igroup",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunMapInfo(ctx, "testHost-testUUID", "testVol").Return(0, fmt.Errorf("error")).Times(1)
			},
			driverContext: tridentconfig.ContextCSI,
			expectedError: true,
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when unmapping LUN from igroup fails")
			},
		},
		{
			name: "Error removing igroup",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunMapInfo(ctx, "testHost-testUUID", "testVol").Return(lunID, nil).Times(1)
				mockAPI.EXPECT().LunUnmap(ctx, "testHost-testUUID", "testVol").Return(nil).Times(1)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, "testHost-testUUID").Return([]string{}, fmt.Errorf("error")).Times(1)
			},
			driverContext: tridentconfig.ContextCSI,
			expectedError: true,
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when removing igroup fails")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volConfig, publishInfo := initializeFunction()
			tridentconfig.CurrentDriverContext = tt.driverContext
			tt.setupMocks(mockAPI)
			err := driver.Unpublish(ctx, &volConfig, &publishInfo)
			tt.verify(t, err)
		})
	}
}

func TestGetSnapshot(t *testing.T) {
	var (
		snapConfig storage.SnapshotConfig
		snapshot   *api.Snapshot
	)

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		snapConfig = storage.SnapshotConfig{
			Version:            "1",
			Name:               "testSnap",
			InternalName:       "testSnap",
			VolumeName:         "testVol",
			VolumeInternalName: "testVol",
		}

		snapshot = &api.Snapshot{
			CreateTime: time.Now().Format(time.RFC3339),
		}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, *storage.Snapshot, error)
	}{
		{
			name: "Positive - Snapshot exists",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(snapshot, nil).Times(1)
			},
			verify: func(t *testing.T, snap *storage.Snapshot, err error) {
				assert.NoError(t, err, "Expected no error when snapshot exists")
				assert.NotNil(t, snap, "Expected snapshot to be returned")
				assert.Equal(t, snapConfig.InternalName, snap.Config.InternalName, "Expected snapshot name to match")
			},
		},
		{
			name: "Negative - Snapshot does not exist",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil, errors.NotFoundError("snapshot not found")).Times(1)
			},
			verify: func(t *testing.T, snap *storage.Snapshot, err error) {
				assert.NoError(t, err, "Expected no error when snapshot does not exist")
				assert.Nil(t, snap, "Expected no snapshot to be returned")
			},
		},
		{
			name: "Negative - Error retrieving snapshot",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil, fmt.Errorf("error")).Times(1)
			},
			verify: func(t *testing.T, snap *storage.Snapshot, err error) {
				assert.Error(t, err, "Expected error when retrieving snapshot fails")
				assert.Nil(t, snap, "Expected no snapshot to be returned")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			snap, err := driver.GetSnapshot(ctx, &snapConfig, nil)
			tt.verify(t, snap, err)
		})
	}
}

func TestGetSnapshots(t *testing.T) {
	var (
		volConfig storage.VolumeConfig
		snapshots *api.Snapshots
	)

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		volConfig = getASAVolumeConfig()
		volConfig.InternalName = "testVol"
		snapshots = &api.Snapshots{
			{
				Name:       "snap1",
				CreateTime: time.Now().Format(time.RFC3339),
			},
			{
				Name:       "snap2",
				CreateTime: time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
		}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, []*storage.Snapshot, error)
	}{
		{
			name: "Positive - Snapshots exist",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotList(ctx, volConfig.InternalName, ASATypeLun).Return(snapshots, nil).Times(1)
			},
			verify: func(t *testing.T, snaps []*storage.Snapshot, err error) {
				assert.NoError(t, err, "Expected no error when snapshots exist")
				assert.NotNil(t, snaps, "Expected snapshots to be returned")
				assert.Equal(t, len(snaps), len(*snapshots), "Expected number of snapshots to match")
			},
		},
		{
			name: "Negative - No snapshots found",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotList(ctx, volConfig.InternalName, ASATypeLun).Return(nil, nil).Times(1)
			},
			verify: func(t *testing.T, snaps []*storage.Snapshot, err error) {
				assert.Error(t, err, "Expected error when no snapshots found")
				assert.Nil(t, snaps, "Expected no snapshots to be returned")
			},
		},
		{
			name: "Negative - Error retrieving snapshots",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotList(ctx, volConfig.InternalName, ASATypeLun).Return(nil, fmt.Errorf("error")).Times(1)
			},
			verify: func(t *testing.T, snaps []*storage.Snapshot, err error) {
				assert.Error(t, err, "Expected error when retrieving snapshots fails")
				assert.Nil(t, snaps, "Expected no snapshots to be returned")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			snaps, err := driver.GetSnapshots(ctx, &volConfig)
			tt.verify(t, snaps, err)
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
	type testCase struct {
		name             string
		setupMock        func(mockAPI *mockapi.MockOntapAPI)
		expectedSnapshot *storage.Snapshot
		expectedError    error
	}

	mockAPI, driver := newMockOntapASADriver(t)
	snapConfig := storage.SnapshotConfig{
		Version:            "1",
		Name:               "testSnap",
		InternalName:       "testSnap",
		VolumeName:         "testVol",
		VolumeInternalName: "testVolInternalName",
	}
	createTime := time.Now()

	testCases := []testCase{
		{
			name: "Successfully create snapshot",
			setupMock: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().StorageUnitSnapshotCreate(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(&api.Snapshot{
					CreateTime: createTime.String(),
				}, nil).Times(1)
			},
			expectedSnapshot: &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Version:            "1",
					Name:               snapConfig.Name,
					InternalName:       snapConfig.InternalName,
					VolumeName:         snapConfig.VolumeName,
					VolumeInternalName: snapConfig.VolumeInternalName,
				},
				Created: createTime.String(),
				State:   storage.SnapshotStateOnline,
			},
			expectedError: nil,
		},
		{
			name: "Error creating snapshot",
			setupMock: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().StorageUnitSnapshotCreate(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(fmt.Errorf("error creating snapshot")).Times(1)
			},
			expectedSnapshot: nil,
			expectedError:    fmt.Errorf("error creating snapshot"),
		},
		{
			name: "Error retrieving snapshot info",
			setupMock: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().StorageUnitSnapshotCreate(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil, fmt.Errorf("error retrieving snapshot info")).Times(1)
			},
			expectedSnapshot: nil,
			expectedError:    fmt.Errorf("error retrieving snapshot info"),
		},
		{
			name: "No snapshot created",
			setupMock: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().StorageUnitSnapshotCreate(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitSnapshotInfo(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil, nil).Times(1)
			},
			expectedSnapshot: nil,
			expectedError:    fmt.Errorf("no snapshot with name %v could be created for volume %v", snapConfig.InternalName, snapConfig.VolumeInternalName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			tc.setupMock(mockAPI)

			snapshot, err := driver.CreateSnapshot(ctx, &snapConfig, nil)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedSnapshot, snapshot)
		})
	}
}

func TestRestoreSnapshot(t *testing.T) {
	var snapConfig storage.SnapshotConfig

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		snapConfig = storage.SnapshotConfig{
			Version:            "1",
			Name:               "testSnap",
			InternalName:       "testSnap",
			VolumeName:         "testVol",
			VolumeInternalName: "testVol",
		}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive - Runs without any error",
			setupMocks: func() {
				mockAPI.EXPECT().SnapshotRestoreVolume(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during snapshot restore")
			},
		},
		{
			name: "Negative - Error restoring snapshot",
			setupMocks: func() {
				mockAPI.EXPECT().SnapshotRestoreVolume(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName).Return(fmt.Errorf("error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error during snapshot restore")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			err := driver.RestoreSnapshot(ctx, &snapConfig, nil)
			tt.verify(t, err)
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	var (
		snapConfig storage.SnapshotConfig
		volConfig  storage.VolumeConfig
	)

	mockAPI, driver := newMockOntapASADriver(t)

	initializeFunction := func() {
		snapConfig = storage.SnapshotConfig{
			Version:            "1",
			Name:               "testSnap",
			InternalName:       "testSnap",
			VolumeName:         "testVol",
			VolumeInternalName: "testVol",
		}

		driver.cloneSplitTimers = make(map[string]time.Time)
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive - Runs without any error",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during snapshot deletion")
			},
		},
		{
			name: "Negative - Snapshot busy error",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(api.SnapshotBusyError("snapshot busy")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when snapshot is busy")
			},
		},
		{
			name: "Negative - General error",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName, ASATypeLun).Return(fmt.Errorf("general error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected general error during snapshot deletion")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializeFunction()
			tt.setupMocks()
			err := driver.DeleteSnapshot(ctx, &snapConfig, &volConfig)
			tt.verify(t, err)
		})
	}
}

func TestGet(t *testing.T) {
	var (
		mockAPI *mockapi.MockOntapAPI
		driver  *ASAStorageDriver
	)

	initializeFunction := func() {
		mockAPI, driver = newMockOntapASADriver(t)
	}

	t.Run("Positive - Test1 - LUN exists", func(t *testing.T) {
		initializeFunction()

		mockAPI.EXPECT().LunExists(ctx, "testLUN").Return(true, nil).Times(1)

		err := driver.Get(ctx, "testLUN")
		assert.NoError(t, err, "Expected no error when LUN exists")
	})

	t.Run("Negative - Test1 - LUN does not exist", func(t *testing.T) {
		initializeFunction()

		mockAPI.EXPECT().LunExists(ctx, "testLUN").Return(false, nil).Times(1)

		err := driver.Get(ctx, "testLUN")
		assert.Error(t, err, "Expected error when LUN does not exist")
		assert.Contains(t, err.Error(), "LUN testLUN does not exist", "Expected not found error message")
	})

	t.Run("Negative - Test2 - Error checking LUN existence", func(t *testing.T) {
		initializeFunction()

		mockAPI.EXPECT().LunExists(ctx, "testLUN").Return(false, fmt.Errorf("error checking LUN")).Times(1)

		err := driver.Get(ctx, "testLUN")
		assert.Error(t, err, "Expected error when checking LUN existence fails")
		assert.Contains(t, err.Error(), "error checking for existing LUN", "Expected error message")
	})
}

func TestGetVolumeExternal(t *testing.T) {
	type testCase struct {
		name          string
		lun           *api.Lun
		volume        *api.Volume
		storagePrefix string
		expected      *storage.VolumeExternal
	}

	_, driver := newMockOntapASADriver(t)

	tests := []testCase{
		{
			name: "Volume with storage prefix",
			lun: &api.Lun{
				Size: "100GiB",
			},
			volume: &api.Volume{
				Name:           "prefix_testVol",
				SnapshotPolicy: "default",
				Aggregates:     []string{"aggr1"},
			},
			storagePrefix: "prefix_",
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:        tridentconfig.OrchestratorAPIVersion,
					Name:           "testVol",
					InternalName:   "prefix_testVol",
					Size:           "100GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					AccessMode:     tridentconfig.ReadWriteOnce,
					AccessInfo:     models.VolumeAccessInfo{},
				},
				Pool: "aggr1",
			},
		},
		{
			name: "Volume without storage prefix",
			lun: &api.Lun{
				Size: "200GiB",
			},
			volume: &api.Volume{
				Name:           "testVol",
				SnapshotPolicy: "default",
				Aggregates:     []string{"aggr2"},
			},
			storagePrefix: "",
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:        tridentconfig.OrchestratorAPIVersion,
					Name:           "testVol",
					InternalName:   "testVol",
					Size:           "200GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					AccessMode:     tridentconfig.ReadWriteOnce,
					AccessInfo:     models.VolumeAccessInfo{},
				},
				Pool: "aggr2",
			},
		},
		{
			name: "Volume with empty aggregates",
			lun: &api.Lun{
				Size: "300GiB",
			},
			volume: &api.Volume{
				Name:           "prefix_testVol2",
				SnapshotPolicy: "default",
				Aggregates:     []string{},
			},
			storagePrefix: "prefix_",
			expected: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:        tridentconfig.OrchestratorAPIVersion,
					Name:           "testVol2",
					InternalName:   "prefix_testVol2",
					Size:           "300GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					AccessMode:     tridentconfig.ReadWriteOnce,
					AccessInfo:     models.VolumeAccessInfo{},
				},
				Pool: drivers.UnsetPool,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver.Config.StoragePrefix = &tt.storagePrefix
			actual := driver.getVolumeExternal(tt.lun, tt.volume)
			assert.Equal(t, tt.expected, actual, "Should have been equal")
		})
	}
}

func TestGetUpdateType(t *testing.T) {
	type testCase struct {
		name       string
		invalid    bool
		driverOrig storage.Driver
		expected   *roaring.Bitmap
	}

	var driver storage.Driver
	_, driver = newMockOntapASADriver(t)

	tests := []testCase{
		{
			name:    "Invalid driver type",
			invalid: true,
			expected: func() *roaring.Bitmap {
				rB := roaring.NewBitmap()
				rB.Add(storage.InvalidUpdate)
				return rB
			}(),
		},
		{
			name: "Password change",
			driverOrig: func() storage.Driver {
				_, drivOrig := newMockOntapASADriver(t)
				drivOrig.Config.Password = "other-password"
				return drivOrig
			}(),
			expected: func() *roaring.Bitmap {
				rB := roaring.NewBitmap()
				rB.Add(storage.PasswordChange)
				return rB
			}(),
		},
		{
			name: "Username change",
			driverOrig: func() storage.Driver {
				_, drivOrig := newMockOntapASADriver(t)
				drivOrig.Config.Username = "other-username"
				return drivOrig
			}(),
			expected: func() *roaring.Bitmap {
				rB := roaring.NewBitmap()
				rB.Add(storage.UsernameChange)
				return rB
			}(),
		},
		{
			name: "Credentials change",
			driverOrig: func() storage.Driver {
				_, drivOrig := newMockOntapASADriver(t)
				drivOrig.Config.Credentials = map[string]string{"key": "oldValue"}
				return drivOrig
			}(),
			expected: func() *roaring.Bitmap {
				rB := roaring.NewBitmap()
				rB.Add(storage.CredentialsChange)
				return rB
			}(),
		},
		{
			name: "Storage prefix change",
			driverOrig: func() storage.Driver {
				_, drivOrig := newMockOntapASADriver(t)
				drivOrig.Config.StoragePrefix = convert.ToPtr("oldPrefix")
				return drivOrig
			}(),
			expected: func() *roaring.Bitmap {
				rB := roaring.NewBitmap()
				rB.Add(storage.PrefixChange)
				return rB
			}(),
		},
		{
			name: "No change",
			driverOrig: func() storage.Driver {
				_, drivOrig := newMockOntapASADriver(t)
				return drivOrig
			}(),
			expected: roaring.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actual *roaring.Bitmap
			if tt.invalid {
				actual = driver.GetUpdateType(context.Background(), &SANStorageDriver{})
			} else {
				actual = driver.GetUpdateType(context.Background(), tt.driverOrig)
			}
			assert.True(t, tt.expected.Equals(actual), "Expected and actual bitmaps should be equal")
		})
	}
}

func TestGetVolumeExternalWrappersASA(t *testing.T) {
	type testCase struct {
		name               string
		setupMocks         func(*mockapi.MockOntapAPI)
		storagePrefix      string
		expectedVolumes    []*storage.VolumeExternalWrapper
		expectedErrorCount int
	}

	mockAPI, driver := newMockOntapASADriver(t)
	driver.Config.StoragePrefix = convert.ToPtr("prefix_")

	tests := []testCase{
		{
			name: "Volumes and LUNs retrieved successfully",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return([]*api.Volume{
					{Name: "prefix_vol1"},
					{Name: "prefix_vol2"},
				}, nil).Times(1)
				mockAPI.EXPECT().LunList(ctx, lunPath(*driver.Config.StoragePrefix+"*")).Return([]api.Lun{
					{Name: "/vol/prefix_vol1/lun0", VolumeName: "prefix_vol1", Size: "100GiB"},
					{Name: "/vol/prefix_vol2/lun0", VolumeName: "prefix_vol2", Size: "200GiB"},
				}, nil).Times(1)
			},
			expectedVolumes: []*storage.VolumeExternalWrapper{
				{Volume: &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Version:      tridentconfig.OrchestratorAPIVersion,
						Name:         "vol1",
						InternalName: "prefix_vol1",
						Size:         "100GiB",
						Protocol:     tridentconfig.Block,
						AccessMode:   tridentconfig.ReadWriteOnce,
					},
					Pool: drivers.UnsetPool,
				}},
				{Volume: &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Version:      tridentconfig.OrchestratorAPIVersion,
						Name:         "vol2",
						InternalName: "prefix_vol2",
						Size:         "200GiB",
						Protocol:     tridentconfig.Block,
						AccessMode:   tridentconfig.ReadWriteOnce,
					},
					Pool: drivers.UnsetPool,
				}},
			},
			expectedErrorCount: 0,
		},
		{
			name: "Error retrieving volumes",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return(nil, fmt.Errorf("volume error")).Times(1)
			},
			storagePrefix:      "prefix_",
			expectedVolumes:    []*storage.VolumeExternalWrapper{{Volume: nil, Error: fmt.Errorf("volume error")}},
			expectedErrorCount: 1,
		},
		{
			name: "Error retrieving LUNs",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return([]*api.Volume{
					{Name: "prefix_vol1"},
				}, nil).Times(1)
				mockAPI.EXPECT().LunList(ctx, lunPath(*driver.Config.StoragePrefix+"*")).Return(nil, fmt.Errorf("LUN error")).Times(1)
			},
			storagePrefix:      "prefix_",
			expectedVolumes:    []*storage.VolumeExternalWrapper{{Volume: nil, Error: fmt.Errorf("LUN error")}},
			expectedErrorCount: 1,
		},
		{
			name: "Flexvol not found for LUN",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return([]*api.Volume{
					{Name: "prefix_vol1"},
				}, nil).Times(1)
				mockAPI.EXPECT().LunList(ctx, lunPath(*driver.Config.StoragePrefix+"*")).Return([]api.Lun{
					{Name: "/vol/prefix_vol2/lun0", VolumeName: "prefix_vol2", Size: "100GiB"},
				}, nil).Times(1)
			},
			storagePrefix:      "prefix_",
			expectedVolumes:    []*storage.VolumeExternalWrapper{},
			expectedErrorCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the mock expectations for the test case
			tt.setupMocks(mockAPI)

			// Create a channel to receive the volume external wrappers
			channel := make(chan *storage.VolumeExternalWrapper)

			// Run the GetVolumeExternalWrappers method in a separate goroutine
			go driver.GetVolumeExternalWrappers(ctx, channel)

			// Collect the results from the channel
			var results []*storage.VolumeExternalWrapper
			for wrapper := range channel {
				results = append(results, wrapper)
			}

			// Verify the results
			var errorCount int
			for _, result := range results {
				if result.Error != nil {
					errorCount++
				}
			}
			assert.Equal(t, tt.expectedErrorCount, errorCount)
			for i, expected := range tt.expectedVolumes {
				if i < len(results) {
					assert.Equal(t, expected.Volume, results[i].Volume)
					assert.Equal(t, expected.Error, results[i].Error)
				}
			}
		})
	}
}

func TestGetVolumeForImportASA(t *testing.T) {
	type testCase struct {
		name           string
		volumeID       string
		setupMocks     func(*mockapi.MockOntapAPI)
		expectedVolume *storage.VolumeExternal
		expectedError  error
	}

	mockAPI, driver := newMockOntapASADriver(t)

	tests := []testCase{
		{
			name:     "Volume and LUN retrieved successfully",
			volumeID: "testVol",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, "testVol").Return(&api.Volume{
					Name:           "testVol",
					SnapshotPolicy: "default",
					Aggregates:     []string{"aggr1"},
				}, nil).Times(1)
				mockAPI.EXPECT().LunGetByName(ctx, "testVol").Return(&api.Lun{
					Name:       "testVol",
					VolumeName: "testVol",
					Size:       "100GiB",
				}, nil).Times(1)
			},
			expectedVolume: &storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Version:        tridentconfig.OrchestratorAPIVersion,
					Name:           "testVol",
					InternalName:   "testVol",
					Size:           "100GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					AccessMode:     tridentconfig.ReadWriteOnce,
				},
				Pool: "aggr1",
			},
			expectedError: nil,
		},
		{
			name:     "Error retrieving volume",
			volumeID: "testVol",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, "testVol").Return(nil, fmt.Errorf("volume error")).Times(1)
			},
			expectedVolume: nil,
			expectedError:  fmt.Errorf("volume error"),
		},
		{
			name:     "Error retrieving LUN",
			volumeID: "testVol",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, "testVol").Return(&api.Volume{
					Name:           "testVol",
					SnapshotPolicy: "default",
					Aggregates:     []string{"aggr1"},
				}, nil).Times(1)
				mockAPI.EXPECT().LunGetByName(ctx, "testVol").Return(nil, fmt.Errorf("LUN error")).Times(1)
			},
			expectedVolume: nil,
			expectedError:  fmt.Errorf("LUN error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the mock expectations for the test case
			tt.setupMocks(mockAPI)

			// Run the GetVolumeForImport method
			actualVolume, actualError := driver.GetVolumeForImport(ctx, tt.volumeID)

			// Verify the results
			assert.Equal(t, tt.expectedVolume, actualVolume)
			assert.Equal(t, tt.expectedError, actualError)
		})
	}
}

func TestCreatePrepareASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}
	volConfig.ImportNotManaged = false
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	pool := storage.NewStoragePool(nil, "dummyPool")

	driver.CreatePrepare(ctx, volConfig, pool)
	tridentconfig.CurrentDriverContext = ""

	assert.Equal(t, "test_fakeVolName", volConfig.InternalName, "Incorrect volume internal name.")
	assert.True(t, volConfig.AccessInfo.PublishEnforcement, "Publish enforcement not enabled.")
}

func TestCreateFollowupASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	volConfig := getASAVolumeConfig()
	err := driver.CreateFollowup(ctx, &volConfig)
	assert.NoError(t, err, "There shouldn't be any error")
}

func TestReconcileVolumeNodeAccess(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	err := driver.ReconcileVolumeNodeAccess(ctx, nil, nil)
	assert.NoError(t, err, "There shouldn't be any error")
}

func TestGetChapInfoASA(t *testing.T) {
	driver := &ASAStorageDriver{
		Config: drivers.OntapStorageDriverConfig{
			UseCHAP:                   true,
			ChapUsername:              "testUser",
			ChapInitiatorSecret:       "initiatorSecret",
			ChapTargetUsername:        "targetUser",
			ChapTargetInitiatorSecret: "targetSecret",
		},
	}

	expectedChapInfo := &models.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "testUser",
		IscsiInitiatorSecret: "initiatorSecret",
		IscsiTargetUsername:  "targetUser",
		IscsiTargetSecret:    "targetSecret",
	}

	chapInfo, err := driver.GetChapInfo(context.Background(), "", "")

	assert.NoError(t, err, "Expected no error from GetChapInfo")
	assert.Equal(t, expectedChapInfo, chapInfo, "Expected CHAP info to match")
}

func TestGoStringASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san-economy",
			DebugTraceFlags:   map[string]bool{"method": true},
			Credentials:       map[string]string{"key": "value"},
		},
		ManagementLIF:             "10.0.0.1",
		SVM:                       "svm0",
		Username:                  "admin",
		Password:                  "password",
		ClientPrivateKey:          "privateKey",
		ChapInitiatorSecret:       "chapInitiatorSecret",
		ChapTargetInitiatorSecret: "chapTargetInitiatorSecret",
		ChapTargetUsername:        "chapTargetUsername",
		ChapUsername:              "chapUsername",
		UseREST:                   convert.ToPtr(true),
	}

	driver.Config = config

	actualGoString := driver.GoString()

	expectedRedactedFields := map[string]string{
		"Username":                  tridentconfig.REDACTED,
		"Password":                  tridentconfig.REDACTED,
		"ClientPrivateKey":          tridentconfig.REDACTED,
		"ChapInitiatorSecret":       tridentconfig.REDACTED,
		"ChapTargetInitiatorSecret": tridentconfig.REDACTED,
		"ChapTargetUsername":        tridentconfig.REDACTED,
		"ChapUsername":              tridentconfig.REDACTED,
		"Credentials":               `map[string]string{"name":"<REDACTED>", "type":"<REDACTED>"}`,
	}

	for field, redactedValue := range expectedRedactedFields {
		assert.Contains(t, actualGoString, field+":"+redactedValue, "The field "+field+" should be redacted")
	}
	assert.Contains(t, actualGoString, "ManagementLIF:"+`"`+config.ManagementLIF+`"`, "The ManagementLIF should be present")
	assert.Contains(t, actualGoString, "SVM:"+`"`+config.SVM+`"`, "The SVM should be present")
}

func TestReconcileNodeAccessASA(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	backendUUID := "1234"
	tridentUUID := "4321"

	// Test reconcile destroys unused igroups
	existingIgroups := []string{"netappdvp", "node1-" + tridentUUID, "node2-" + tridentUUID, "trident-" + backendUUID}
	nodesInUse := []*models.Node{{Name: "node2"}}
	mockAPI.EXPECT().IgroupList(ctx).Return(existingIgroups, nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, existingIgroups[1])
	mockAPI.EXPECT().IgroupDestroy(ctx, existingIgroups[1])
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, existingIgroups[3])
	mockAPI.EXPECT().IgroupDestroy(ctx, existingIgroups[3])

	err := driver.ReconcileNodeAccess(ctx, nodesInUse, backendUUID, tridentUUID)
	assert.NoError(t, err)
}

func TestASAStorageDriver_CanEnablePublishEnforcement(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	canEnable := driver.CanEnablePublishEnforcement()
	assert.True(t, canEnable, "The CanEnablePublishEnforcement method should return true")
}

func TestCanSnapshotASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)
	err := driver.CanSnapshot(ctx, nil, nil)
	assert.Nil(t, err)
}

func TestGetStorageBackendSpecsASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)

	backend := &storage.StorageBackend{}
	backend.SetOnline(true)
	backend.SetStorage(make(map[string]storage.Pool))
	backend.SetDriver(driver)

	pool1 := storage.NewStoragePool(nil, "dummyPool1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	pool2 := storage.NewStoragePool(nil, "dummyPool2")
	pool2.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool2.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool2.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	physicalPools := map[string]storage.Pool{"dummyPool1": pool1, "dummyPool2": pool2}
	driver.physicalPools = physicalPools

	err := driver.GetStorageBackendSpecs(ctx, backend)

	assert.NoError(t, err)
	assert.Equal(t, backend.Name(), driver.BackendName(), "Should be equal")

	expectedPhysicalPoolsName := []string{"dummyPool1", "dummyPool2"}
	assert.Equal(t, expectedPhysicalPoolsName, backend.GetPhysicalPoolNames(ctx))
}

func TestGetStorageBackendPhysicalPoolNamesASA(t *testing.T) {
	_, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "dummyPool1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	pool2 := storage.NewStoragePool(nil, "dummyPool2")
	pool2.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool2.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool2.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool2.Attributes()[sa.Replication] = sa.NewBoolOffer(false)

	physicalPools := map[string]storage.Pool{"dummyPool1": pool1, "dummyPool2": pool2}
	driver.physicalPools = physicalPools

	expectedPhysicalPoolsName := []string{"dummyPool1", "dummyPool2"}

	actualPhysicalPoolsName := driver.GetStorageBackendPhysicalPoolNames(ctx)

	sort.Strings(expectedPhysicalPoolsName)
	sort.Strings(actualPhysicalPoolsName)

	assert.Equal(t, expectedPhysicalPoolsName, actualPhysicalPoolsName, "Should be equal")
}

func TestGetBackendStateASA(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)
	dataLIFs := []string{"1.2.3.4"}
	derivedPools := []string{ONTAPTEST_VSERVER_AGGR_NAME}

	pool1 := storage.NewStoragePool(nil, ONTAPTEST_VSERVER_AGGR_NAME)
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer("dummyBackend")
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	physicalPools := map[string]storage.Pool{ONTAPTEST_VSERVER_AGGR_NAME: pool1}
	driver.physicalPools = physicalPools

	mockAPI.EXPECT().GetSVMState(ctx).Return(restAPIModels.SvmStateRunning, nil).AnyTimes().Times(1)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(derivedPools, nil).AnyTimes()
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return(dataLIFs, nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, true).Return("9.14.1", nil).Times(1)
	mockAPI.EXPECT().APIVersion(ctx, false).Return("9.14.1", nil).Times(1)

	state, code := driver.GetBackendState(ctx)
	assert.False(t, code.Contains(storage.BackendStateReasonChange), "Should not be reason change")
	assert.False(t, code.Contains(storage.BackendStateAPIVersionChange), "Should not be API version change")
	assert.False(t, code.Contains(storage.BackendStatePoolsChange), "Should be no pool change")
	assert.Equal(t, "", state, "Reason should be empty")
}

func TestEnablePublishEnforcementASA(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volName := "trid_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	internalVolName := "pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         volName,
			InternalName: internalVolName,
			AccessInfo: models.VolumeAccessInfo{
				PublishEnforcement: false,
				IscsiAccessInfo: models.IscsiAccessInfo{
					IscsiLunNumber: 1,
				},
			},
			ImportNotManaged: false,
		},
	}
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return(nil, nil)

	err := driver.EnablePublishEnforcement(ctx, volume)
	assert.NoError(t, err)
	assert.True(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.Equal(t, int32(-1), volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestOntapASAStorageDriver_Resize_Success(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volConfig := getASAVolumeConfig()

	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(1073741824, nil)
	mockAPI.EXPECT().LunSetSize(ctx, "trident-pvc-1234", "2147483648").Return(uint64(214748364), nil)

	err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.NoError(t, err, "Volume resize failed")
}

func TestOntapASAStorageDriver_Resize_LesserSizeThanCurrent(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volConfig := getASAVolumeConfig()

	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(2147483648, nil) // 2GB

	err := driver.Resize(ctx, &volConfig, 1073741824) // 1GB

	assert.Error(t, err, "Expected error when resizing to lesser size than current")
}

func TestOntapASAStorageDriver_Resize_APIErrors(t *testing.T) {
	mockAPI, driver := newMockOntapASADriver(t)

	volConfig := getASAVolumeConfig()

	// Case: Failure while checking if LUN exists
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(false, fmt.Errorf("error checking LUN existence"))

	err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when checking LUN existence")

	// Case: LUN does not exist
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(false, nil)

	err = driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when LUN does not exist")

	// Case: Failure while getting LUN size
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(0, fmt.Errorf("error getting LUN size"))

	err = driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when getting LUN size")

	// Case: Failure while resizing LUN
	mockAPI.EXPECT().LunExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().LunSize(ctx, "trident-pvc-1234").Return(1073741824, nil)
	mockAPI.EXPECT().LunSetSize(ctx, "trident-pvc-1234", "2147483648").Return(uint64(0), fmt.Errorf("error resizing LUN"))

	err = driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.Error(t, err, "Expected error when resizing LUN")
}

func TestOntapASAStorageDriver_Import_Managed_Success(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "2g",
		Name:    "lun1",
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}
	igroups := []string{"igroup1", "igroup2"}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
	mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
		"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(
		igroups, nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup1", volConfig.InternalName).Return(nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup2", volConfig.InternalName).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Expected no error in managed import, but got error")
	assert.Equal(t, "2g", volConfig.Size, "Expected volume config to be updated with actual LUN size")
}

func TestOntapASAStorageDriver_Import_UnManaged_Success(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Name:       originalVolumeName,
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "2g",
		Name:    "lun1",
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Expected no error in unmanaged import, but got error")
	assert.Equal(t, "2g", volConfig.Size, "Expected volume config to be updated with actual LUN size")
}

func TestOntapASAStorageDriver_Import_VolumeNotRW(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Name:       originalVolumeName,
		AccessType: "ro",
	}
	lun := api.Lun{
		Size:    "1g",
		Name:    "lun1",
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when volume is not RW, but got none")
}

func TestOntapASAStorageDriver_Import_LunNotOnline(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as LUN name in ASA driver
	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Name:       originalVolumeName,
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "1g",
		Name:    "lun1",
		State:   "offline",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when LUN is not online, but got none")
}

func TestOntapASAStorageDriver_NameTemplateLabel(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"

	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "2g",
		Name:    originalVolumeName,
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	tests := []struct {
		labelTestName string
		labelKey      string
		labelValue    string
		expectedLabel string
	}{
		{
			"label", "nameTemplate", "dev-test-cluster-1",
			"{\"provisioning\":{\"nameTemplate\":\"dev-test-cluster-1\"}}",
		},
		{
			"emptyLabelKey", "", "dev-test-cluster-1",
			"{\"provisioning\":{\"\":\"dev-test-cluster-1\"}}",
		},
		{
			"emptyLabelValue", "nameTemplate", "",
			"{\"provisioning\":{\"nameTemplate\":\"\"}}",
		},
		{
			"labelValueSpecialCharacter", "nameTemplate^%^^\\u0026\\u0026^%_________", "dev-test-cluster-1%^%",
			"{\"provisioning\":{\"nameTemplate^%^^\\\\u0026\\\\u0026^%_________\":\"dev-test-cluster-1%^%\"}}",
		},
	}

	for _, test := range tests {
		t.Run(test.labelTestName, func(t *testing.T) {
			driver.Config.Labels = map[string]string{
				test.labelKey: test.labelValue,
			}
			pool1.SetAttributes(map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
			})
			driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

			mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
			mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
			mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
				test.expectedLabel).Return(nil)
			mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(nil, nil)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			assert.NoError(t, err, "Volume import fail")
		})
	}
}

func TestOntapASAStorageDriver_NameTemplateLabelLengthExceeding(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	longLabel := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
		"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ" +
		"bQBuHvTRNX6pHRKUXaIrjEpygM4SpaqHYdZ8O1k2meeugg7eXu4dPhqetI3Sip3W4v9QuFkh1YBaI" +
		"9sHE9w5eRxpmTv0POpCB5xAqzmN6XCkxuXKc4yfNS9PRwcTSpvkA3PcKCF3TD1TJU3NYzcChsFQgm" +
		"bAsR32cbJRdsOwx6BkHNfRCji0xSnBFUFUu1sGHfYCmzzd3OmChADIP6RwRtpnqNzvt0CU6uumBnl" +
		"Lc5U7mBI1Ndmqhn0BBSh588thKOQcpD4bvnSBYU788tBeVxQtE8KkdUgKl8574eWldqWDiALwoiCS" +
		"Ae2GuZzwG4ACw2uHdIkjb6FEwapSKCEogr4yWFAVCYPp2pA37Mj88QWN82BEpyoTV6BRAOsubNPfT" +
		"N94X0qCcVaQp4L5bA4SPTQu0ag20a2k9LmVsocy5y11U3ewpzVGtENJmxyuyyAbxOFOkDxKLRMhgs" +
		"uJMhhplD894tkEcPoiFhdsYZbBZ4MOBF6KkuBF5aqMrQbOCFt2vvTN843nRhomVMpY01SNuUeb5mh" +
		"UN53wsqqHSGoYb1eUBDlTUDLFcCcNacxfsILqmthnrD1B5u85jRm1SfkFfuIDOgaaTM9UhxNQ1U6M" +
		"mBaRYBkuGtTScoVTXyF4lij2sj1WWrKb7qWlaUUjxHiaxgLovPWErldCXXkNFsHgc7UYLQLF4j6lO" +
		"I1QdTAyrtCcSxRwdkjBxj8mQy1HblHnaaBwP7Nax9FvIvxpeqyD6s3X1vfFNGAMuRsc9DKmPDfxjh" +
		"qGzRQawFEbbURWij9xleKsUr0yCjukyKsxuaOlwbXnoFh4V3wtidrwrNXieFD608EANwvCp7u2S8Q" +
		"px99T4O87AdQGa5cAX8Ccojd9tENOmQRmOAwVEuFtuogos96TFlq0YHyfESDTB2TWayIuGJvgTIpX" +
		"lthQFQfHVgPpUZdzZMjXry"

	pool1 := storage.NewStoragePool(nil, "pool1")

	driver.Config.Labels = map[string]string{
		"app":     "my-db-app",
		longLabel: "gold",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}
	lun := api.Lun{
		Size:    "1g",
		Name:    originalVolumeName,
		State:   "online",
		Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when label length is too long during import, but got none")
}

func TestOntapASAStorageDriver_APIErrors(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASADriver(t)

	originalVolumeName := "lun1"
	volConfig := getASAVolumeConfig()
	volConfig.ImportNotManaged = false

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "LunInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(
					nil, fmt.Errorf("error while fetching LUN"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error while fetching LUN, got nil.",
		},
		{
			name: "LunInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN info to be nil, got non-nil.",
		},
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil,
					fmt.Errorf("error while fetching volume"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error while fetching volume, got nil.",
		},
		{
			name: "VolumeInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Expected Volume info to be nil, got non-nil.",
		},
		{
			name: "LunRename_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(
					fmt.Errorf("error while renaming LUN"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN rename to fail, but it succeeded",
		},
		{
			name: "LunSetComment_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
					"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(
					fmt.Errorf("error while setting LUN comment"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN set comment to fail, but it succeeded",
		},
		{
			name: "LunListIgroup_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
					"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(
					nil, fmt.Errorf("error while listing igroups of LUN"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN list igroup to fail, but it succeeded",
		},
		{
			name: "LunUnmap_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}
				volume := api.Volume{
					Name:       originalVolumeName,
					AccessType: "rw",
				}
				lun := api.Lun{
					Size:    "1g",
					Name:    "lun1",
					State:   "online",
					Comment: "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
				}
				igroups := []string{"igroup1", "igroup2"}
				mockAPI.EXPECT().LunGetByName(ctx, originalVolumeName).Return(&lun, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().LunSetComment(ctx, volConfig.InternalName,
					"{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}").Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(ctx, volConfig.InternalName).Return(igroups, nil)
				mockAPI.EXPECT().LunUnmap(ctx, gomock.Any(), volConfig.InternalName).Return(
					fmt.Errorf("error while unmaping igroup of LUN")).AnyTimes()
			},
			wantErr:       assert.Error,
			assertMessage: "Expected LUN unmap igroup to fail, but it succeeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}
