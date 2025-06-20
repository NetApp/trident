// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netapp/trident/utils/filesystem"

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
	"github.com/netapp/trident/utils/models"
)

func getASANVMeVolumeConfig() storage.VolumeConfig {
	return storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "true",
		FileSystem:   "xfs",
		Name:         "vol1",
		InternalName: "trident-pvc-1234",
	}
}

func newMockOntapASANVMeDriver(t *testing.T) (*mockapi.MockOntapAPI, *ASANVMeStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapASANVMeDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}
	driver.Config.SANType = sa.NVMe

	return mockAPI, driver
}

func newTestOntapASANVMeDriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, apiOverride api.OntapAPI,
) *ASANVMeStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true
	// config.Labels = map[string]string{"app": "wordpress"}
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-asa-nvme-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = convert.ToPtr("test_")

	asaNvmeDriver := &ASANVMeStorageDriver{}
	asaNvmeDriver.Config = *config

	var ontapAPI api.OntapAPI

	if apiOverride != nil {
		ontapAPI = apiOverride
	} else {
		// ZAPI is not supported in ASA driver. Return Rest Client all the time.
		ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
	}

	asaNvmeDriver.API = ontapAPI
	asaNvmeDriver.telemetry = &Telemetry{
		Plugin:        asaNvmeDriver.Name(),
		StoragePrefix: *asaNvmeDriver.Config.StoragePrefix,
		Driver:        asaNvmeDriver,
	}

	return asaNvmeDriver
}

func TestGetConfigASANVMe(t *testing.T) {
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
	driver := &ASANVMeStorageDriver{
		Config: expectedConfig,
	}
	actualConfig := driver.GetConfig()
	assert.Equal(t, &expectedConfig, actualConfig, "The returned configuration should match the expected configuration")
}

func TestGetTelemetryASANVMe(t *testing.T) {
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
	driver := &ASANVMeStorageDriver{
		telemetry: expectedTelemetry,
	}
	actualTelemetry := driver.GetTelemetry()
	assert.Equal(t, expectedTelemetry, actualTelemetry, "The returned telemetry should match the expected telemetry")
}

func TestBackendNameASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
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
			expectedResult: "ontapasanvme_127.0.0.1",
		},
		{
			name:           "EmptyConfigName_NoLIFs_ReturnsNoLIFs",
			backendName:    "",
			ips:            []string{},
			expectedResult: "ontapasanvme_noLIFs",
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

func TestGetExternalConfigASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san",
			DebugTraceFlags:   map[string]bool{"method": true},
			Credentials:       map[string]string{"key": "value"},
		},
		ManagementLIF: "10.0.0.1",
		SVM:           "svm0",
		Username:      "admin",
		Password:      "password",
		UseREST:       convert.ToPtr(true),
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

func TestGetProtocolASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	actualProtocol := driver.GetProtocol(ctx)
	expectedProtocol := tridentconfig.Block
	assert.Equal(t, expectedProtocol, actualProtocol, "The returned protocol should be Block")
}

func TestStoreConfigASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-san-economy",
			DebugTraceFlags:   map[string]bool{"method": true},
			Credentials:       map[string]string{"key": "value"},
		},
		ManagementLIF: "10.0.0.1",
		SVM:           "svm0",
		Username:      "admin",
		Password:      "password",
		UseREST:       convert.ToPtr(true),
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
		ManagementLIF: "10.0.0.1",
		SVM:           "svm0",
		Username:      "admin",
		Password:      "password",
		UseREST:       convert.ToPtr(true),
	}

	// Verify that the stored configuration matches the expected configuration
	assert.Equal(t, &expectedConfig, persistentConfig.OntapConfig, "The stored configuration should match the expected configuration")
}

func TestInitializeASANVMe(t *testing.T) {
	driverContext := driverContextCSI

	var (
		mockAPI       *mockapi.MockOntapAPI
		driver        *ASANVMeStorageDriver
		commonConfig  *drivers.CommonStorageDriverConfig
		backendSecret map[string]string
		backendUUID   string
		configJSON    []byte
	)

	mockAPI, driver = newMockOntapASANVMeDriver(t)

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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{"127.0.0.1"}, nil).Times(1)
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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{"127.0.0.1"}, nil).Times(1)
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
			name: "NVMe protocol not supported",
			setupMocks: func() {
				driver.initialized = false
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(false).Times(1)
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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{"127.0.0.1"}, fmt.Errorf("some-error")).Times(1)
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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{}, nil).Times(1)
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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{"127.0.0.1"}, nil).Times(1)
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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{"127.0.0.1"}, nil).Times(1)
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
				mockAPI.EXPECT().SupportsFeature(ctx, api.NVMeProtocol).Return(true).Times(1)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{"127.0.0.1"}, nil).Times(1)
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

func TestTerminateNVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	tests := []struct {
		name          string
		driverContext tridentconfig.DriverContext
		telemetry     *Telemetry
	}{
		{
			name:          "DriverContextCSI_WithTelemetry",
			driverContext: tridentconfig.ContextCSI,
			telemetry:     &Telemetry{done: make(chan struct{})},
		},
		{
			name:          "DriverContextCSI_WithoutTelemetry",
			driverContext: tridentconfig.ContextCSI,
			telemetry:     nil,
		},
		{
			name:          "DriverContextCSI_WithoutTelemetry_IgroupDestory_Returns_error",
			driverContext: tridentconfig.ContextCSI,
			telemetry:     nil,
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

			driver.Terminate(ctx, "")

			if tt.telemetry != nil {
				assert.True(t, tt.telemetry.stopped, "Expected telemetry to be stopped")
			}
			assert.False(t, driver.initialized, "Expected driver to be not initialized")
		})
	}
}

func TestValidateASANVMe(t *testing.T) {
	type testCase struct {
		name          string
		setupDriver   func(driver *ASANVMeStorageDriver)
		expectedError bool
	}

	_, driver := newMockOntapASANVMeDriver(t)

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
			setupDriver: func(driver *ASANVMeStorageDriver) {
				// No additional setup needed for this case
			},
			expectedError: false,
		},
		{
			name: "validateSanDriver fails",
			setupDriver: func(driver *ASANVMeStorageDriver) {
				driver.Config.SANType = sa.FCP
				driver.Config.UseCHAP = true
			},
			expectedError: true,
		},
		{
			name: "ValidateStoragePrefix fails",
			setupDriver: func(driver *ASANVMeStorageDriver) {
				storagePrefix := "/s"
				driver.Config.StoragePrefix = &storagePrefix
			},
			expectedError: true,
		},
		{
			name: "ValidateASAStoragePool fails",
			setupDriver: func(driver *ASANVMeStorageDriver) {
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

func TestCreateASANVMe(t *testing.T) {
	var (
		volConfig     *storage.VolumeConfig
		storagePool   *storage.StoragePool
		volAttrs      map[string]sa.Request
		nvmeNamespace api.NVMeNamespace
	)

	volumeName := "testSU"
	storagePrefix := "myprefix_"
	internalName := fmt.Sprintf("%s%s", storagePrefix, volumeName)
	labels := fmt.Sprintf(`{"provisioning":{"template":"%s"}}`, volumeName)
	formatOptions := strings.Join([]string{"-b 4096", "-T stirde=2056, stripe=16"},
		filesystem.FormatOptionsSeparator)
	mockUUID := uuid.New().String()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	expectedInternalID := driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, internalName)
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
		Size:              capacityBytes,
	}

	initializedFunction := func() {
		driver.Config.DriverContext = tridentconfig.ContextCSI
		driver.Config.StoragePrefix = &storagePrefix
		driver.Config.LimitVolumeSize = "2G"

		volConfig = &storage.VolumeConfig{
			InternalName: internalName,
			Name:         volumeName,
			Size:         "1G",
		}

		nsComment := map[string]string{
			nsAttributeFSType:    "ext4",
			nsAttributeLUKS:      "false",
			nsAttributeDriverCtx: string(tridentconfig.ContextCSI),
			FormatOptions:        formatOptions,
			nsLabels:             "{\"provisioning\":{\"template\":\"testSU\"}}",
		}

		commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, nsComment, nsMaxCommentLength)

		nvmeNamespace = api.NVMeNamespace{
			Name:      internalName,
			Size:      "1073741824",
			OsType:    "linux",
			BlockSize: defaultNamespaceBlockSize,
			Comment:   commentStr,
			QosPolicy: api.QosPolicyGroup{
				Name: "fake-qos-policy",
				Kind: api.QosPolicyGroupKind,
			},
		}

		storagePool = storage.NewStoragePool(nil, "pool1")
		storagePool.SetInternalAttributes(internalAttributesMap)
		storagePool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
			"template": `{{.volume.Name}}`,
		})
		storagePool.InternalAttributes()[FormatOptions] = formatOptions
		storagePool.InternalAttributes()[SkipRecoveryQueue] = "true"
		driver.physicalPools = map[string]storage.Pool{"pool1": storagePool}
		volAttrs = map[string]sa.Request{}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive case - NVMe Namespace created successfully",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().NVMeNamespaceCreate(ctx, nvmeNamespace).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, internalName).DoAndReturn(
					func(ctx context.Context, name string) (*api.NVMeNamespace, error) {
						nvmeNamespace.UUID = mockUUID
						return &nvmeNamespace, nil
					}).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Should not be an error")
				// Assert whether volconfig gets proper updated with the values that were sent via storage pool.
				assert.Equal(t, mockUUID, volConfig.AccessInfo.NVMeNamespaceUUID, "Namespace UUID should match")
				assert.Equal(t, expectedInternalID, volConfig.InternalID, "Internal ID should match volume name")
				assert.Equal(t, internalName, volConfig.InternalName, "Internal Name should match volume name")
				assert.Equal(t, storagePool.InternalAttributes()[SpaceReserve], volConfig.SpaceReserve, "SpaceReserve does not match")
				assert.Equal(t, storagePool.InternalAttributes()[SnapshotPolicy], volConfig.SnapshotPolicy, "SnapshotPolicy does not match")
				assert.Equal(t, storagePool.InternalAttributes()[Encryption], volConfig.Encryption, "Encryption does not match")
				assert.Equal(t, storagePool.InternalAttributes()[QosPolicy], volConfig.QosPolicy, "QoSPolicy does not match")
				assert.Equal(t, storagePool.InternalAttributes()[LUKSEncryption], volConfig.LUKSEncryption, "LUKSEncryption does not match")
				assert.Equal(t, storagePool.InternalAttributes()[FileSystemType], volConfig.FileSystem, "FileSystem does not match")
				assert.Equal(t, formatOptions, volConfig.FormatOptions, "FormatOptions do not match")
				assert.Equal(t, "true", volConfig.SkipRecoveryQueue, "SkipRecoveryQueue does not match")
			},
		},
		{
			name: "Positive case - If size passed is zero, taking default size which was set in storage pool",
			setupMocks: func() {
				volConfig = &storage.VolumeConfig{
					InternalName: internalName,
					Name:         volumeName,
					Size:         "0",
				}
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				// Set size as that of storage pool which is expected by mockAPI and reset later
				nvmeNamespace.Size = storagePool.InternalAttributes()[Size]
				defer func() { nvmeNamespace.Size = "1073741824" }()
				mockAPI.EXPECT().NVMeNamespaceCreate(ctx, nvmeNamespace).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, internalName).DoAndReturn(
					func(ctx context.Context, name string) (*api.NVMeNamespace, error) {
						nvmeNamespace.UUID = mockUUID
						return &nvmeNamespace, nil
					}).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Should not be an error")
				assert.Equal(t, storagePool.InternalAttributes()[Size], volConfig.Size, "Size does not match")
			},
		},
		{
			name: "Positive case - Verify labels are set correctly",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().NVMeNamespaceCreate(ctx, nvmeNamespace).DoAndReturn(
					func(ctx context.Context, ns api.NVMeNamespace) error {
						expectedLabels, _ := ConstructLabelsFromConfigs(
							ctx, storagePool, volConfig, driver.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
						assert.Equal(t, expectedLabels, labels, "Labels should match the expected value")
						return nil
					}).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, internalName).Return(&nvmeNamespace, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Should not be an error")
			},
		},
		{
			name: "Negative case - NVMe Namespace already exists",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(true, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Should be an error")
				assert.IsType(t, drivers.NewVolumeExistsError(internalName), err)
			},
		},
		{
			name: "Negative case - Error checking NVMe Namespace existence",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(
					false, fmt.Errorf("error checking namespace existence")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Should be an error")
				assert.Contains(t, err.Error(), "failure checking for existence of NVMe namespace", "error message does not match")
			},
		},
		{
			name: "Negative case - QOSPolicyGroup creation returns an error",
			setupMocks: func() {
				internalAttributesMap[QosPolicy] = "some-qos-policy"
				internalAttributesMap[AdaptiveQosPolicy] = "some-adaptive-policy"
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(true, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[QosPolicy] = "fake-qos-policy"
				delete(internalAttributesMap, AdaptiveQosPolicy)
				assert.Error(t, err, "Should be an error")
			},
		},
		{
			name: "Negative case - Error creating NVMe namespace",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake").Times(1)
				mockAPI.EXPECT().NVMeNamespaceCreate(ctx, nvmeNamespace).Return(fmt.Errorf("API error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Should be an error")
				assert.Contains(t, err.Error(), "API error")
			},
		},
		{
			name: "Negative case - Space allocation set to false",
			setupMocks: func() {
				internalAttributesMap[SpaceAllocation] = "false"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[LUKSEncryption] = "false"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("luksEncryption must be set to %s", DefaultLuksEncryption))
			},
		},
		{
			name: "Negative case - skipRecoveryQueue is not a valid boolean",
			setupMocks: func() {
				internalAttributesMap[SkipRecoveryQueue] = "invalid-boolean"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when skipRecoveryQueue is not a valid boolean")
				assert.Contains(t, err.Error(), "invalid boolean value for skipRecoveryQueue")
			},
		},
		{
			name: "Negative case - Capacity given is invalid",
			setupMocks: func() {
				volConfig.Size = "invalid-size"
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.Size = "1G"
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "could not convert size")
			},
		},
		{
			name: "Negative case - driver volume size limit is invalid",
			setupMocks: func() {
				volConfig.Size = "1G"
				driver.Config.LimitVolumeSize = "invalid-size"
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				volConfig.Size = "1G"
				assert.Error(t, err, "Expected error when limitVolumeSize is invalid")
				assert.Contains(t, err.Error(), "error parsing limitVolumeSize", "Expected error parsing limitVolumeSize")
			},
		},
		{
			name: "Negative case - Unsupported file system",
			setupMocks: func() {
				internalAttributesMap[FileSystemType] = "invalid-filesystem"
				storagePool.SetInternalAttributes(internalAttributesMap)
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, internalName).Return(false, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				internalAttributesMap[FileSystemType] = "ext4"
				volConfig.Size = "1G"
				assert.Error(t, err, "expected error when unsupported file system is passed")
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

func TestCreateCloneASANVMe(t *testing.T) {
	var (
		cloneVolConfig  storage.VolumeConfig
		storagePool     *storage.StoragePool
		sourceNS        *api.NVMeNamespace
		nsCommentString string
	)

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	initializeFunction := func() {
		cloneVolConfig = getASANVMeVolumeConfig()
		cloneVolConfig.InternalName = "testVol"
		cloneVolConfig.CloneSourceVolumeInternal = "sourceVol"
		cloneVolConfig.CloneSourceSnapshotInternal = "sourceSnap"
		cloneVolConfig.QosPolicy = "mixed"

		storagePool = storage.NewStoragePool(nil, "pool1")
		storagePool.SetInternalAttributes(map[string]string{
			SplitOnClone: "false",
		})

		// Set how should be the expected comment to be set on the namespace
		nsComment := map[string]string{
			nsAttributeFSType:    cloneVolConfig.FileSystem,
			nsAttributeLUKS:      cloneVolConfig.LUKSEncryption,
			nsAttributeDriverCtx: string(driver.Config.DriverContext),
			FormatOptions:        cloneVolConfig.FormatOptions,
			nsLabels:             "",
		}
		nsCommentString, _ = driver.createNVMeNamespaceCommentString(ctx, nsComment, nsMaxCommentLength)

		sourceNS = &api.NVMeNamespace{
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
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
				mockAPI.EXPECT().StorageUnitExists(ctx, cloneVolConfig.InternalName).Return(
					false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(
					ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
					cloneVolConfig.CloneSourceSnapshotInternal).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetQosPolicyGroup(ctx, cloneVolConfig.InternalName, expectedQOSPolicy).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, cloneVolConfig.InternalName).Return(sourceNS, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, cloneVolConfig.InternalName, nsCommentString).Return(nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
				mockAPI.EXPECT().StorageUnitExists(ctx, cloneVolConfig.InternalName).Return(
					false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(
					ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
					cloneVolConfig.CloneSourceSnapshotInternal).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneSplitStart(ctx, cloneVolConfig.InternalName).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetQosPolicyGroup(ctx, cloneVolConfig.InternalName, expectedQOSPolicy).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, cloneVolConfig.InternalName).Return(sourceNS, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, cloneVolConfig.InternalName, nsCommentString).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				storagePool.SetInternalAttributes(map[string]string{
					SplitOnClone: "false",
				})
				assert.NoError(t, err, "Expected no error during clone creation")
			},
		},
		{
			name: "Negative - NVMeNamespaceGetByName returns an error",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, fmt.Errorf("error-api")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - NVMeNamespaceGetByName cannot find a namespace",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(nil, nil).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - Error setting qosPolicyGroup",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
				mockAPI.EXPECT().StorageUnitExists(ctx, cloneVolConfig.InternalName).Return(
					false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(
					ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
					cloneVolConfig.CloneSourceSnapshotInternal).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, cloneVolConfig.InternalName, nsCommentString).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetQosPolicyGroup(ctx, cloneVolConfig.InternalName, expectedQOSPolicy).Return(
					fmt.Errorf("api-error")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected an error during clone creation")
			},
		},
		{
			name: "Negative - Error setting namespace comment",
			setupMocks: func() {
				longLabel := "thisIsATestLABEL"
				driver.Config.SplitOnClone = "false"
				driver.Config.Labels = map[string]string{
					longLabel: "dev-test-cluster-1",
				}
				storagePool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
				})
				// Set how should be the expected comment to be set on the namespace
				expectedLabel := "{\"provisioning\":{\"thisIsATestLABEL\":\"dev-test-cluster-1\"}}"
				nsComment := map[string]string{
					nsAttributeFSType:    cloneVolConfig.FileSystem,
					nsAttributeLUKS:      cloneVolConfig.LUKSEncryption,
					nsAttributeDriverCtx: string(driver.Config.DriverContext),
					FormatOptions:        cloneVolConfig.FormatOptions,
					nsLabels:             expectedLabel,
				}
				nsCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, nsComment, nsMaxCommentLength)

				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
				mockAPI.EXPECT().StorageUnitExists(ctx, cloneVolConfig.InternalName).Return(
					false, nil).Times(1)
				mockAPI.EXPECT().StorageUnitCloneCreate(
					ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
					cloneVolConfig.CloneSourceSnapshotInternal).Return(nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, cloneVolConfig.InternalName, nsCommentString).Return(fmt.Errorf("api-error")).Times(1)
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
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
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

				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
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

func TestCreateCloneASANVMe_NameTemplate(t *testing.T) {
	var (
		mockAPI        *mockapi.MockOntapAPI
		driver         *ASANVMeStorageDriver
		cloneVolConfig storage.VolumeConfig
		storagePool    *storage.StoragePool
		sourceNS       *api.NVMeNamespace
	)

	mockAPI, driver = newMockOntapASANVMeDriver(t)

	initializeFunction := func() {
		cloneVolConfig = getASANVMeVolumeConfig()
		cloneVolConfig.CloneSourceVolumeInternal = "sourceVol"
		cloneVolConfig.CloneSourceSnapshotInternal = "sourceSnap"

		sourceNS = &api.NVMeNamespace{
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
			mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, sourceNS.Name).Return(sourceNS, nil).Times(1)
			mockAPI.EXPECT().StorageUnitExists(ctx, cloneVolConfig.InternalName).Return(
				false, nil).Times(1)
			mockAPI.EXPECT().StorageUnitCloneCreate(
				ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
				cloneVolConfig.CloneSourceSnapshotInternal).Return(nil).Times(1)
			mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, cloneVolConfig.InternalName).Return(sourceNS, nil).Times(1)

			// Set how should be the expected comment to be set on the namespace
			nsComment := map[string]string{
				nsAttributeFSType:    cloneVolConfig.FileSystem,
				nsAttributeLUKS:      cloneVolConfig.LUKSEncryption,
				nsAttributeDriverCtx: string(driver.Config.DriverContext),
				FormatOptions:        cloneVolConfig.FormatOptions,
				nsLabels:             test.expectedLabel,
			}
			nsCommentString, err := driver.createNVMeNamespaceCommentString(ctx, nsComment, nsMaxCommentLength)
			mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, cloneVolConfig.InternalName, nsCommentString).Return(nil).Times(1)

			driver.Config.Labels = map[string]string{
				test.labelKey: test.labelValue,
			}
			storagePool.SetAttributes(map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
			})

			err = driver.CreateClone(ctx, nil, &cloneVolConfig, storagePool)

			assert.NoError(t, err, "Clone creation failed. Expected no error")
		})
	}
}

func TestRenameASANVMe(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	nsInfo := &api.NVMeNamespace{
		UUID: uuid.NewString(),
		Name: "existingName",
	}

	testCases := []struct {
		name      string
		newName   string
		mockError error
		mocks     func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error)
		expectErr bool
	}{
		{
			name:      "valid rename",
			newName:   "newNamespace",
			mockError: nil,
			mocks: func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, name).Return(nsInfo, err).Times(1)
				mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsInfo.UUID, newName).Return(err).Times(1)
			},
			expectErr: false,
		},
		{
			name:    "error while fetching namespace",
			newName: "newNamespace",
			mocks: func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, name).Return(nsInfo, err).Times(1)
			},
			mockError: fmt.Errorf("error fetching existing namespace"),
			expectErr: true,
		},
		{
			name:    "rename error",
			newName: "newNamespace",
			mocks: func(mockAPI *mockapi.MockOntapAPI, name, newName string, err error) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, name).Return(nsInfo, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsInfo.UUID, newName).Return(err).Times(1)
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

func TestDestroyASANVMe(t *testing.T) {
	var volConfig storage.VolumeConfig

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	initializeFunction := func() {
		driver.Config.DriverContext = tridentconfig.ContextCSI
		volConfig = storage.VolumeConfig{
			InternalName: "testNS",
		}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive - Namespace exists and is destroyed successfully",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().NVMeNamespaceDelete(ctx, volConfig.InternalName).Return(nil)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during LUN destruction")
			},
		},
		{
			name: "Positive - Namespace does not exist",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error when LUN does not exist")
			},
		},
		{
			name: "Positive - Context is Docker, Namespace exists and is destroyed successfully",
			setupMocks: func() {
				driver.Config.DriverContext = tridentconfig.ContextDocker

				mockAPI.EXPECT().NVMeNamespaceExists(ctx, volConfig.InternalName).Return(true, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceDelete(ctx, volConfig.InternalName).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during LUN destruction")
			},
		},
		{
			name: "Negative - Error checking for existing Namespace",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, volConfig.InternalName).Return(false, fmt.Errorf("error checking Namespace"))
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "error checking for existing ASA NVMe namespace")
			},
		},
		{
			name: "Negative - Error destroying Namespace",
			setupMocks: func() {
				mockAPI.EXPECT().NVMeNamespaceExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().NVMeNamespaceDelete(ctx, volConfig.InternalName).Return(fmt.Errorf("error destroying Namespace"))
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "error destroying ASA NVMe namespace")
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

func TestPublishASANVMe(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
		FormatOptions: strings.Join([]string{"-b 4096", "-T stirde=2056, stripe=16"},
			filesystem.FormatOptionsSeparator),
	}

	flexVol := &api.Volume{
		AccessType: VolTypeRW,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:    "fakeHostName",
		TridentUUID: "fakeUUID",
	}

	subsystem := &api.NVMeSubsystem{
		Name: "fakeSubsysName",
		NQN:  "fakeNQN",
		UUID: "fakeUUID",
	}

	// Create a comment generated from Trident to be present on existing namespace
	existingNsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		FormatOptions:        volConfig.FormatOptions,
		nsLabels:             "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}
	existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

	nvmeNamespace := api.NVMeNamespace{
		UUID:      uuid.NewString(),
		Name:      volConfig.Name,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   existingCommentString,
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	// case: error getting volume Info
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(
		flexVol, fmt.Errorf("error getting volume")).Times(1)

	err := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error during publish, got none")

	// case: DP type flexvol
	flexVol.AccessType = VolTypeDP
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error for a DP volume, got none")

	// case: Docker context: positive case
	flexVol.AccessType = VolTypeRW
	driver.Config.DriverContext = tridentconfig.ContextDocker
	publishInfo.HostNQN = "fakeHostNQN"
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, volConfig.InternalName).Return(&nvmeNamespace, nil).Times(1)
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, gomock.Any(), gomock.Any()).Return(subsystem, nil).Times(1)
	mockAPI.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mockAPI.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error when running in Docker context, got one")

	// case: Docker context: negative case - not able to fetch nvme namespace
	flexVol.AccessType = VolTypeRW
	driver.Config.DriverContext = tridentconfig.ContextDocker
	publishInfo.HostNQN = "fakeHostNQN"
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, volConfig.InternalName).Return(nil, errors.New("mock error")).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when running in Docker context, got none")

	// case: Docker context: negative case - empty NVMe namespace
	flexVol.AccessType = VolTypeRW
	driver.Config.DriverContext = tridentconfig.ContextDocker
	publishInfo.HostNQN = "fakeHostNQN"
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, volConfig.InternalName).Return(nil, nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when nil NVMe namespace is returned in Docker context, got none")

	// case: Docker context: negative case - failed to parse NVMe namespace comment
	flexVol.AccessType = VolTypeRW
	driver.Config.DriverContext = tridentconfig.ContextDocker
	publishInfo.HostNQN = "fakeHostNQN"
	nvmeNamespace.Comment = "{\"invalid json with no nsAttribute as first level key\"}"
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, volConfig.InternalName).Return(&nvmeNamespace, nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when NVMe namespace comment is invalid in Docker context, got none")

	// case: Error creating subsystem in CSI Context
	driver.Config.DriverContext = tridentconfig.ContextCSI
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, gomock.Any(), gomock.Any()).Return(
		subsystem, fmt.Errorf("Error creating subsystem")).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when creating subsystem, got none")

	// case: Host NQN not found in publish Info
	flexVol.AccessType = VolTypeRW
	publishInfo.HostNQN = ""
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when host NQN is empty, got none")

	// case: Error while adding host nqn to subsystem
	publishInfo.HostNQN = "fakeHostNQN"
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, gomock.Any(), gomock.Any()).Return(subsystem, nil).Times(1)
	mockAPI.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(fmt.Errorf("Error adding host nqnq to subsystem")).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when adding host NQN to subsystem, got none")

	// case: Error returned by NVMeEnsureNamespaceMapped
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, gomock.Any(), gomock.Any()).Return(subsystem, nil).Times(1)
	mockAPI.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mockAPI.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(
		fmt.Errorf("Error returned by NVMeEnsureNamespaceMapped")).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when NVMeEnsureNamespaceMapped fails, got none")

	// case: Success
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, gomock.Any(), gomock.Any()).Return(subsystem, nil).Times(1)
	mockAPI.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mockAPI.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error when adding host NQN to subsystem, got one")

	// case: Success for RAW volume
	volConfig.FileSystem = filesystem.Raw
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mockAPI.EXPECT().NVMeSubsystemCreate(ctx, "s_fakeInternalName", "s_fakeInternalName").Return(subsystem, nil).Times(1)
	mockAPI.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mockAPI.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

	err = driver.Publish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error when adding host NQN to subsystem, got one")
}

func TestUnpublishASANVMe(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:    "fakeHostName",
		TridentUUID: "fakeUUID",
	}

	// case 1: NVMeEnsureNamespaceUnmapped returned error
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeUUID"
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	mockAPI.EXPECT().NVMeEnsureNamespaceUnmapped(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(false, fmt.Errorf("NVMeEnsureNamespaceUnmapped returned error"))

	err := driver.Unpublish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when NVMeEnsureNamespaceUnmapped fails, got none")

	// case 2: Success
	volConfig.AccessInfo.PublishEnforcement = true
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeUUID"
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	mockAPI.EXPECT().NVMeEnsureNamespaceUnmapped(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

	err = driver.Unpublish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error during unpublish, got one")
}

func TestGetSnapshotNVMe(t *testing.T) {
	var (
		snapConfig storage.SnapshotConfig
		snapshot   *api.Snapshot
	)

	mockAPI, driver := newMockOntapASANVMeDriver(t)

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
				mockAPI.EXPECT().StorageUnitSnapshotInfo(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(snapshot, nil).Times(1)
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
				mockAPI.EXPECT().StorageUnitSnapshotInfo(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil, errors.NotFoundError("snapshot not found")).Times(1)
			},
			verify: func(t *testing.T, snap *storage.Snapshot, err error) {
				assert.NoError(t, err, "Expected no error when snapshot does not exist")
				assert.Nil(t, snap, "Expected no snapshot to be returned")
			},
		},
		{
			name: "Negative - Error retrieving snapshot",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotInfo(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil, fmt.Errorf("error")).Times(1)
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

func TestGetSnapshotsNVMe(t *testing.T) {
	var (
		volConfig storage.VolumeConfig
		snapshots *api.Snapshots
	)

	mockAPI, driver := newMockOntapASANVMeDriver(t)

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
				mockAPI.EXPECT().StorageUnitSnapshotList(
					ctx, volConfig.InternalName).Return(snapshots, nil).Times(1)
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
				mockAPI.EXPECT().StorageUnitSnapshotList(
					ctx, volConfig.InternalName).Return(nil, nil).Times(1)
			},
			verify: func(t *testing.T, snaps []*storage.Snapshot, err error) {
				assert.Error(t, err, "Expected error when no snapshots found")
				assert.Nil(t, snaps, "Expected no snapshots to be returned")
			},
		},
		{
			name: "Negative - Error retrieving snapshots",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotList(
					ctx, volConfig.InternalName,
				).Return(nil, fmt.Errorf("error")).Times(1)
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

func TestCreateSnapshotNVMe(t *testing.T) {
	type testCase struct {
		name             string
		setupMock        func(mockAPI *mockapi.MockOntapAPI)
		expectedSnapshot *storage.Snapshot
		expectedError    error
	}

	mockAPI, driver := newMockOntapASANVMeDriver(t)

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
				mockAPI.EXPECT().StorageUnitSnapshotCreate(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitSnapshotInfo(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(&api.Snapshot{CreateTime: createTime.String()}, nil).Times(1)
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
				mockAPI.EXPECT().StorageUnitSnapshotCreate(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(fmt.Errorf("error creating snapshot")).Times(1)
			},
			expectedSnapshot: nil,
			expectedError:    fmt.Errorf("error creating snapshot"),
		},
		{
			name: "Error retrieving snapshot info",
			setupMock: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().StorageUnitSnapshotCreate(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitSnapshotInfo(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil, fmt.Errorf("error retrieving snapshot info")).Times(1)
			},
			expectedSnapshot: nil,
			expectedError:    fmt.Errorf("error retrieving snapshot info"),
		},
		{
			name: "No snapshot created",
			setupMock: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().StorageUnitSnapshotCreate(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil).Times(1)
				mockAPI.EXPECT().StorageUnitSnapshotInfo(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil, nil).Times(1)
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

func TestRestoreSnapshotNVMe(t *testing.T) {
	var snapConfig storage.SnapshotConfig

	mockAPI, driver := newMockOntapASANVMeDriver(t)

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
				mockAPI.EXPECT().StorageUnitSnapshotRestore(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during snapshot restore")
			},
		},
		{
			name: "Negative - Error restoring snapshot",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotRestore(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(fmt.Errorf("error")).Times(1)
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

func TestDeleteSnapshotNVMe(t *testing.T) {
	var (
		snapConfig storage.SnapshotConfig
		volConfig  storage.VolumeConfig
	)

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	initializeFunction := func() {
		snapConfig = storage.SnapshotConfig{
			Version:            "1",
			Name:               "testSnap",
			InternalName:       "testSnap",
			VolumeName:         "testVol",
			VolumeInternalName: "testVol",
		}

		driver.cloneSplitTimers = &sync.Map{}
	}

	tests := []struct {
		name       string
		setupMocks func()
		verify     func(*testing.T, error)
	}{
		{
			name: "Positive - Runs without any error",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotDelete(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(nil).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "Expected no error during snapshot deletion")
			},
		},
		{
			name: "Negative - Snapshot busy error",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotDelete(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(api.SnapshotBusyError("snapshot busy")).Times(1)
			},
			verify: func(t *testing.T, err error) {
				assert.Error(t, err, "Expected error when snapshot is busy")
			},
		},
		{
			name: "Negative - General error",
			setupMocks: func() {
				mockAPI.EXPECT().StorageUnitSnapshotDelete(
					ctx, snapConfig.InternalName, snapConfig.VolumeInternalName,
				).Return(fmt.Errorf("general error")).Times(1)
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

func TestGetNVMe(t *testing.T) {
	var (
		mockAPI *mockapi.MockOntapAPI
		driver  *ASANVMeStorageDriver
	)

	initializeFunction := func() {
		mockAPI, driver = newMockOntapASANVMeDriver(t)
	}

	t.Run("Positive - Test1 - Namespace exists", func(t *testing.T) {
		initializeFunction()

		mockAPI.EXPECT().NVMeNamespaceExists(ctx, "testNS").Return(true, nil).Times(1)

		err := driver.Get(ctx, "testNS")
		assert.NoError(t, err, "Expected no error when Namespace exists")
	})

	t.Run("Negative - Test1 - Namespace does not exist", func(t *testing.T) {
		initializeFunction()

		mockAPI.EXPECT().NVMeNamespaceExists(ctx, "testNS").Return(false, nil).Times(1)

		err := driver.Get(ctx, "testNS")
		assert.Error(t, err, "Expected error when Namespace does not exist")
		assert.Contains(t, err.Error(), "NVMe namespace testNS does not exist", "Expected not found error message")
	})

	t.Run("Negative - Test2 - Error checking Namespace existence", func(t *testing.T) {
		initializeFunction()

		mockAPI.EXPECT().NVMeNamespaceExists(ctx, "testNS").Return(false, fmt.Errorf("error checking Namespace")).Times(1)

		err := driver.Get(ctx, "testNS")
		assert.Error(t, err, "Expected error when checking Namespace existence fails")
		assert.Contains(t, err.Error(), "error checking for existing ASA NVMe namespace", "Expected error message")
	})
}

func TestGetVolumeExternalASANVMe(t *testing.T) {
	type testCase struct {
		name          string
		nvmeNamespace *api.NVMeNamespace
		volume        *api.Volume
		storagePrefix string
		expected      *storage.VolumeExternal
	}

	_, driver := newMockOntapASANVMeDriver(t)

	tests := []testCase{
		{
			name: "Volume with storage prefix",
			nvmeNamespace: &api.NVMeNamespace{
				Name: "prefix_testVol",
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
					InternalID:     driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, "prefix_testVol"),
					Size:           "100GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					SnapshotDir:    "false",
					AccessMode:     tridentconfig.ReadWriteOnce,
					AccessInfo:     models.VolumeAccessInfo{},
				},
				Pool: "aggr1",
			},
		},
		{
			name: "Volume without storage prefix",
			nvmeNamespace: &api.NVMeNamespace{
				Name: "testVol",
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
					InternalID:     driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, "testVol"),
					Size:           "200GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					SnapshotDir:    "false",
					AccessMode:     tridentconfig.ReadWriteOnce,
					AccessInfo:     models.VolumeAccessInfo{},
				},
				Pool: "aggr2",
			},
		},
		{
			name: "Volume with empty aggregates",
			nvmeNamespace: &api.NVMeNamespace{
				Name: "testVol2",
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
					InternalID:     driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, "testVol2"),
					Size:           "300GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					SnapshotDir:    "false",
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
			actual := driver.getVolumeExternal(tt.nvmeNamespace, tt.volume)
			assert.Equal(t, tt.expected, actual, "Should have been equal")
		})
	}
}

func TestGetUpdateTypeASANVMe(t *testing.T) {
	type testCase struct {
		name       string
		invalid    bool
		driverOrig storage.Driver
		expected   *roaring.Bitmap
	}

	var driver storage.Driver
	_, driver = newMockOntapASANVMeDriver(t)

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
				_, drivOrig := newMockOntapASANVMeDriver(t)
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
				_, drivOrig := newMockOntapASANVMeDriver(t)
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
				_, drivOrig := newMockOntapASANVMeDriver(t)
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
				_, drivOrig := newMockOntapASANVMeDriver(t)
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
				_, drivOrig := newMockOntapASANVMeDriver(t)
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

func TestGetVolumeExternalWrappersASANVMe(t *testing.T) {
	type testCase struct {
		name               string
		setupMocks         func(*mockapi.MockOntapAPI)
		storagePrefix      string
		expectedVolumes    []*storage.VolumeExternalWrapper
		expectedErrorCount int
	}

	mockAPI, driver := newMockOntapASANVMeDriver(t)
	driver.Config.StoragePrefix = convert.ToPtr("prefix_")

	tests := []testCase{
		{
			name: "Volumes and Namespaces retrieved successfully",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return([]*api.Volume{
					{Name: "prefix_vol1"},
					{Name: "prefix_vol2"},
				}, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceList(ctx, *driver.Config.StoragePrefix+"*").Return(
					api.NVMeNamespaces{
						&api.NVMeNamespace{Name: "prefix_vol1", VolumeName: "prefix_vol1", Size: "100GiB"},
						&api.NVMeNamespace{Name: "prefix_vol2", VolumeName: "prefix_vol2", Size: "200GiB"},
					}, nil).Times(1)
			},
			expectedVolumes: []*storage.VolumeExternalWrapper{
				{Volume: &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Version:      tridentconfig.OrchestratorAPIVersion,
						Name:         "vol1",
						InternalName: "prefix_vol1",
						InternalID:   driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, "prefix_vol1"),
						Size:         "100GiB",
						Protocol:     tridentconfig.Block,
						AccessMode:   tridentconfig.ReadWriteOnce,
						SnapshotDir:  "false",
					},
					Pool: drivers.UnsetPool,
				}},
				{Volume: &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Version:      tridentconfig.OrchestratorAPIVersion,
						Name:         "vol2",
						InternalName: "prefix_vol2",
						InternalID:   driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, "prefix_vol2"),
						Size:         "200GiB",
						Protocol:     tridentconfig.Block,
						AccessMode:   tridentconfig.ReadWriteOnce,
						SnapshotDir:  "false",
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
			name: "Error retrieving Namespaces",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return([]*api.Volume{
					{Name: "prefix_vol1"},
				}, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceList(ctx, *driver.Config.StoragePrefix+"*").Return(
					nil, fmt.Errorf("Namespace error")).Times(1)
			},
			storagePrefix:      "prefix_",
			expectedVolumes:    []*storage.VolumeExternalWrapper{{Volume: nil, Error: fmt.Errorf("Namespace error")}},
			expectedErrorCount: 1,
		},
		{
			name: "Flexvol not found for Namespace",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, *driver.Config.StoragePrefix).Return([]*api.Volume{
					{Name: "prefix_vol1"},
				}, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceList(ctx, *driver.Config.StoragePrefix+"*").Return(
					api.NVMeNamespaces{&api.NVMeNamespace{Name: "prefix_vol2", VolumeName: "prefix_vol2", Size: "100GiB"}}, nil).Times(1)
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

func TestGetVolumeForImportASANVMe(t *testing.T) {
	type testCase struct {
		name           string
		volumeID       string
		setupMocks     func(*mockapi.MockOntapAPI)
		expectedVolume *storage.VolumeExternal
		expectedError  error
	}

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	tests := []testCase{
		{
			name:     "Volume and Namespace retrieved successfully",
			volumeID: "testVol",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, "testVol").Return(&api.Volume{
					Name:           "testVol",
					SnapshotPolicy: "default",
					Aggregates:     []string{"aggr1"},
				}, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "testVol").Return(&api.NVMeNamespace{
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
					InternalID:     driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, "testVol"),
					Size:           "100GiB",
					Protocol:       tridentconfig.Block,
					SnapshotPolicy: "default",
					SnapshotDir:    "false",
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
			name:     "Error retrieving Namespace",
			volumeID: "testVol",
			setupMocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, "testVol").Return(&api.Volume{
					Name:           "testVol",
					SnapshotPolicy: "default",
					Aggregates:     []string{"aggr1"},
				}, nil).Times(1)
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "testVol").Return(nil, fmt.Errorf("Namespace error")).Times(1)
			},
			expectedVolume: nil,
			expectedError:  fmt.Errorf("Namespace error"),
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

func TestCreatePrepareASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
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

func TestCreatePrepareASANVMe_NilPool(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}

	volConfig.ImportNotManaged = false
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	driver.Config.NameTemplate = `{{.volume.Name}}_{{.volume.Namespace}}_{{.volume.StorageClass}}`

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	driver.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, driver, driver.getStoragePoolAttributes(ctx), driver.BackendName())

	driver.CreatePrepare(ctx, &volConfig, nil)

	assert.Equal(t, "newVolume_testNamespace_testSC", volConfig.InternalName, "Incorrect volume internal name.")
}

func TestCreatePrepareASANVMe_NilPool_TemplateNotContainVolumeName(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := storage.VolumeConfig{Name: "pvc-1234567", Namespace: "testNamespace", StorageClass: "testSC"}

	volConfig.ImportNotManaged = false
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	driver.Config.NameTemplate = `{{.volume.Namespace}}_{{.volume.StorageClass}}_{{slice .volume.Name 4 9}}`

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	driver.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, driver, driver.getStoragePoolAttributes(ctx), driver.BackendName())

	driver.CreatePrepare(ctx, &volConfig, nil)

	assert.Equal(t, "testNamespace_testSC_12345", volConfig.InternalName, "Incorrect volume internal name.")
}

func TestCreateFollowupASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	volConfig := getASANVMeVolumeConfig()
	err := driver.CreateFollowup(ctx, &volConfig)
	assert.NoError(t, err, "There shouldn't be any error")
}

func TestReconcileVolumeNodeAccessASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	err := driver.ReconcileVolumeNodeAccess(ctx, nil, nil)
	assert.NoError(t, err, "There shouldn't be any error")
}

func TestGoStringASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

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

func TestReconcileNodeAccessASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	backendUUID := "1234"
	tridentUUID := "4321"

	nodesInUse := []*models.Node{{Name: "node2"}}

	err := driver.ReconcileNodeAccess(ctx, nodesInUse, backendUUID, tridentUUID)

	assert.NoError(t, err)
}

func TestASANVMeStorageDriver_CanEnablePublishEnforcement(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	canEnable := driver.CanEnablePublishEnforcement()
	assert.True(t, canEnable, "The CanEnablePublishEnforcement method should return true")
}

func TestCanSnapshotASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	err := driver.CanSnapshot(ctx, nil, nil)
	assert.Nil(t, err)
}

func TestGetStorageBackendSpecsASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	backend := &storage.StorageBackend{}
	backend.SetOnline(true)
	backend.ClearStoragePools()
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
	actualPhysicalPoolsName := backend.GetPhysicalPoolNames(ctx)

	sort.Strings(expectedPhysicalPoolsName)
	sort.Strings(actualPhysicalPoolsName)

	assert.Equal(t, expectedPhysicalPoolsName, actualPhysicalPoolsName)
}

func TestGetStorageBackendPhysicalPoolNamesASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

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

func TestGetBackendStateASANVMe(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)
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

func TestEnablePublishEnforcementASANVMe(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	volConfig := getASANVMeVolumeConfig()
	vol := storage.Volume{Config: &volConfig}

	driver.EnablePublishEnforcement(ctx, &vol)

	assert.True(t, vol.Config.AccessInfo.PublishEnforcement, "Incorrect publish enforcement value.")
}

func TestOntapASANVMeStorageDriver_Resize_Success(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := getASANVMeVolumeConfig()
	nsUUID := uuid.New()
	ns := &api.NVMeNamespace{UUID: nsUUID.String(), Name: "trident-pvc-1234", Size: "1073741824"}
	requestedSize := "2147483648" // 2GB
	requestedSizeBytes, _ := convert.ToPositiveInt64(requestedSize)

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(ns, nil)
	mockAPI.EXPECT().NVMeNamespaceSetSize(ctx, nsUUID.String(), requestedSizeBytes).Return(nil)

	err := driver.Resize(ctx, &volConfig, uint64(requestedSizeBytes)) // 2GB

	assert.NoError(t, err, "expected no error during resize, got one")
	assert.Equal(t, requestedSize, volConfig.Size, "actual size does not match the requested size")
}

func TestOntapASANVMeStorageDriver_Resize_SizeMoreThanIntMax(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	volConfig := getASANVMeVolumeConfig()
	var requestedSize uint64
	requestedSize = math.MaxInt64 + 100

	err := driver.Resize(ctx, &volConfig, requestedSize)

	assert.Error(t, err, "Expected error when resizing to size greater than int64 max")
}

func TestOntapASANVMeStorageDriver_Resize_InvalidCurrentSize(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := getASANVMeVolumeConfig()
	nsUUID := uuid.New()
	ns := &api.NVMeNamespace{UUID: nsUUID.String(), Name: "trident-pvc-1234", Size: "invalid-size"}

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(ns, nil)

	err := driver.Resize(ctx, &volConfig, 1073741824) // 1GB

	assert.Error(t, err, "Expected error when namespace current size is invalid")
	assert.ErrorContains(t, err, "error while parsing ASA NVMe namespace size",
		"Expected specific error message for invalid size")
}

func TestOntapASANVMeStorageDriver_Resize_LesserSizeThanCurrent(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := getASANVMeVolumeConfig()
	nsUUID := uuid.New()
	ns := &api.NVMeNamespace{UUID: nsUUID.String(), Name: "trident-pvc-1234", Size: "2147483648"}

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(ns, nil)

	err := driver.Resize(ctx, &volConfig, 1073741824) // 1GB

	assert.Error(t, err, "Expected error when resizing to lesser size than current")
}

func TestOntapASANVMeStorageDriver_Resize_DriverVolumeLimitError(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := getASANVMeVolumeConfig()
	nsUUID := uuid.New()
	ns := &api.NVMeNamespace{UUID: nsUUID.String(), Name: "trident-pvc-1234", Size: "1073741824"}

	// Case: Invalid limitVolumeSize on driver
	driver.Config.LimitVolumeSize = "1000.1000"

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(ns, nil)

	err := driver.Resize(ctx, &volConfig, 2147483648) // 1GB

	assert.ErrorContains(t, err, "error parsing limitVolumeSize")

	// Case: requestedSize is more than limitVolumeSize
	driver.Config.LimitVolumeSize = "1073741824"

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(ns, nil)

	err = driver.Resize(ctx, &volConfig, 2147483648) // 1GB

	capacityErr, _ := errors.HasUnsupportedCapacityRangeError(err)

	assert.True(t, capacityErr, "expected unsupported capacity error")
}

func TestOntapASANVMeStorageDriver_Resize_APIErrors(t *testing.T) {
	mockAPI, driver := newMockOntapASANVMeDriver(t)

	volConfig := getASANVMeVolumeConfig()
	nsUUID := uuid.New()
	ns := &api.NVMeNamespace{UUID: nsUUID.String(), Name: "trident-pvc-1234", Size: "2147483648"}
	requestedSize := "2147483648" // 2GB
	requestedSizeBytes, _ := convert.ToPositiveInt64(requestedSize)

	// Case: Failure while getting NVMe namespace
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(nil, fmt.Errorf("mock error"))
	err := driver.Resize(ctx, &volConfig, uint64(requestedSizeBytes)) // 2GB

	assert.Error(t, err, "Expected error when fetching NVMe namespace")

	// Case: Namespace does not exist
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(nil, nil)

	err = driver.Resize(ctx, &volConfig, uint64(requestedSizeBytes)) // 2GB

	assert.Error(t, err, "Expected error when NVMe namespace does not exist")

	// Case: Failure while resizing namespace
	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, "trident-pvc-1234").Return(ns, nil)
	mockAPI.EXPECT().NVMeNamespaceSetSize(
		ctx, nsUUID.String(), requestedSizeBytes).Return(fmt.Errorf("error getting namespace size"))

	err = driver.Resize(ctx, &volConfig, uint64(requestedSizeBytes)) // 2GB

	assert.Error(t, err, "Expected error when setting NVMe namespace size")
}

func TestImportASANVMe_Managed_Success(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

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

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}

	nsUuid := uuid.New()

	// Create a comment generated from Trident to be present on existing namespace
	existingNsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		nsLabels:             "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   existingCommentString,
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	// Generate the comment to be set
	nsCommentToBeSet := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		FormatOptions:        volConfig.FormatOptions,
		nsLabels:             "{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}",
	}

	nsCommentToBeSetString, _ := driver.createNVMeNamespaceCommentString(ctx, nsCommentToBeSet, nsMaxCommentLength)

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(false, nil)
	mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUuid.String(), volConfig.InternalName).Return(nil)
	mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, volConfig.InternalName, nsCommentToBeSetString).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Expected no error in managed import, but got error")
	assert.Equal(t, "1g", volConfig.Size, "Expected volume config to be updated with actual Namespace size")
	assert.Equal(t, driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, nvmeNamespace.Name), volConfig.InternalID,
		"InternalID not set as expected")
	assert.Equal(t, nvmeNamespace.UUID, volConfig.AccessInfo.NVMeNamespaceUUID,
		"NVMe namespace UUID not set as expected")
}

func TestImportASANVMe_Managed_ExistingComments(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}

	existingNsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		FormatOptions:        volConfig.FormatOptions,
	}

	tests := []struct {
		name               string
		getExistingComment func() string
		setCommentExpected bool
	}{
		{
			name: "MetadataLabels_WithCustomBaseLabel",
			getExistingComment: func() string {
				label := "{\"custom-comment\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}"
				existingNsComment[nsLabels] = label
				existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

				return existingCommentString
			},
			setCommentExpected: false,
		},
		{
			name: "NoMetadataLabel_OnlyBaseLabel",
			getExistingComment: func() string {
				return "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}"
			},
			setCommentExpected: false,
		},
		{
			name: "ChangedMetadataLabelKey",
			getExistingComment: func() string {
				label := "{\"custom-comment\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}"
				existingNsComment[nsLabels] = label
				existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)
				updatedCommentString := strings.Replace(existingCommentString, `"nsAttribute"`, `"myCustomKey"`, 1)

				return updatedCommentString
			},
			setCommentExpected: false,
		},
		{
			name: "TridentSetLabel",
			getExistingComment: func() string {
				label := "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}"
				existingNsComment[nsLabels] = label
				existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

				return existingCommentString
			},
			setCommentExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nsUuid := uuid.New()
			nvmeNamespace := api.NVMeNamespace{
				UUID:      nsUuid.String(),
				Name:      originalVolumeName,
				State:     "online",
				Size:      "1g",
				OsType:    "linux",
				BlockSize: defaultNamespaceBlockSize,
				Comment:   test.getExistingComment(),
				QosPolicy: api.QosPolicyGroup{
					Name: "fake-qos-policy",
					Kind: api.QosPolicyGroupKind,
				},
			}

			// Set mock expects
			mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
			mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(false, nil)
			mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUuid.String(), volConfig.InternalName).Return(nil)

			if test.setCommentExpected {
				// Generate the comment to be set
				nsCommentToBeSet := map[string]string{
					nsAttributeFSType:    volConfig.FileSystem,
					nsAttributeLUKS:      volConfig.LUKSEncryption,
					nsAttributeDriverCtx: string(driver.Config.DriverContext),
					FormatOptions:        volConfig.FormatOptions,
					nsLabels:             "{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}",
				}

				nsCommentToBeSetString, _ := driver.createNVMeNamespaceCommentString(ctx, nsCommentToBeSet, nsMaxCommentLength)

				mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, volConfig.InternalName, nsCommentToBeSetString).Return(nil)
			}

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			assert.NoError(t, err, "Expected no error in managed import, but got error")
			assert.Equal(t, "1g", volConfig.Size, "Expected volume config to be updated with actual Namespace size")
			assert.Equal(t, driver.CreateASANVMeNamespaceInternalID(driver.Config.SVM, nvmeNamespace.Name), volConfig.InternalID,
				"InternalID not set as expected")
			assert.Equal(t, nvmeNamespace.UUID, volConfig.AccessInfo.NVMeNamespaceUUID,
				"NVMe namespace UUID not set as expected")
		})
	}
}

func TestImportASANVMe_UnManaged_Success(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		AccessType: "rw",
	}

	nsUuid := uuid.New()
	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(false, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Expected no error in unmanaged import, but got error")
	assert.Equal(t, "1g", volConfig.Size, "Expected volume config to be updated with actual Namespace size")
}

func TestImportASANVMe_VolumeNotRW(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		AccessType: "ro",
	}

	nsUuid := uuid.New()
	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when volume is not RW, but got none")
}

func TestImportASANVMe_NamespaceNotOnline(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.FileSystem = ""
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		AccessType: "ro",
	}

	nsUuid := uuid.New()
	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "offline",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when NVMe namespace is not online, but got none")
}

func TestImportASANVMe_DifferentLabels(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"app":   "my-db-app",
		"label": "gold",
	}

	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}

	// Create a comment generated from Trident to be present on existing namespace
	existingNsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		nsLabels:             "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

	nsUuid := uuid.New()
	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   existingCommentString,
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
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

			// Generate the comment to be set
			nsCommentToBeSet := map[string]string{
				nsAttributeFSType:    volConfig.FileSystem,
				nsAttributeLUKS:      volConfig.LUKSEncryption,
				nsAttributeDriverCtx: string(driver.Config.DriverContext),
				FormatOptions:        volConfig.FormatOptions,
				nsLabels:             test.expectedLabel,
			}
			nsCommentToBeSetString, _ := driver.createNVMeNamespaceCommentString(ctx, nsCommentToBeSet, nsMaxCommentLength)

			mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
			mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(false, nil)
			mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUuid.String(), volConfig.InternalName).Return(nil)
			mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, volConfig.InternalName, nsCommentToBeSetString).Return(nil)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			assert.NoError(t, err, "Expected no error in NVMe namespace import, but got error")
		})
	}
}

func TestImportASANVMe_LabelLengthExceeding(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

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

	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// originalVolumeName will be same as NVMe namespace name in ASA driver
	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
	}

	// Create a comment generated from Trident to be present on existing namespace
	existingNsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		FormatOptions:        volConfig.FormatOptions,
		nsLabels:             "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

	nsUuid := uuid.New()
	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   existingCommentString,
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(false, nil)
	mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUuid.String(), volConfig.InternalName).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Expected error when label length is too long during import, but got none")
}

func TestImportASANVMe_APIErrors(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapASANVMeDriver(t)

	originalVolumeName := "namespace1"
	volConfig := getASANVMeVolumeConfig()
	volConfig.ImportNotManaged = false

	// Create a comment generated from Trident to be present on existing namespace
	existingNsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(driver.Config.DriverContext),
		FormatOptions:        volConfig.FormatOptions,
		nsLabels:             "{\"provisioning\":{\"app\":\"my-gateway-app\",\"label\":\"silver\"}}",
	}

	existingCommentString, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)

	nsUuid := uuid.New()
	nvmeNamespace := api.NVMeNamespace{
		UUID:      nsUuid.String(),
		Name:      originalVolumeName,
		State:     "online",
		Size:      "1g",
		OsType:    "linux",
		BlockSize: defaultNamespaceBlockSize,
		Comment:   existingCommentString,
		QosPolicy: api.QosPolicyGroup{
			Name: "fake-qos-policy",
			Kind: api.QosPolicyGroupKind,
		},
	}

	volume := api.Volume{
		AccessType: "rw",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "NVMeNamespaceGetByName_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(
					nil, fmt.Errorf("error while fetching Namespace"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error while fetching NVMe namespace, got nil.",
		},
		{
			name: "NVMeNamespaceGetByName_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error when NVMe namespace is nil, but got none.",
		},
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil,
					fmt.Errorf("error while fetching volume"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error while fetching volume, got nil.",
		},
		{
			name: "VolumeInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error when Volume info is nil, but got none.",
		},
		{
			name: "NVMeIsNamespaceMapped_Error",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(
					false, fmt.Errorf("error while checking if namespace is mapped"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected error when checking for namespace mapping fails, but got none.",
		},
		{
			name: "NVMeNamespaceRename_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(
					false, nil)
				mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUuid.String(), volConfig.InternalName).Return(
					fmt.Errorf("error while renaming NVMe namespace"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected NVMe namespace  rename to fail, but it succeeded",
		},
		{
			name: "NVMeNamespaceSetComment_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.Labels = map[string]string{
					"app":   "my-db-app",
					"label": "gold",
				}

				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, originalVolumeName).Return(&nvmeNamespace, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", nsUuid.String()).Return(
					false, nil)
				mockAPI.EXPECT().NVMeNamespaceRename(ctx, nsUuid.String(), volConfig.InternalName).Return(nil)
				// Generate the comment to be set
				nsCommentToBeSet := map[string]string{
					nsAttributeFSType:    volConfig.FileSystem,
					nsAttributeLUKS:      volConfig.LUKSEncryption,
					nsAttributeDriverCtx: string(driver.Config.DriverContext),
					FormatOptions:        volConfig.FormatOptions,
					nsLabels:             "{\"provisioning\":{\"app\":\"my-db-app\",\"label\":\"gold\"}}",
				}

				nsCommentToBeSetString, _ := driver.createNVMeNamespaceCommentString(ctx, nsCommentToBeSet, nsMaxCommentLength)

				mockAPI.EXPECT().NVMeNamespaceSetComment(ctx, volConfig.InternalName, nsCommentToBeSetString).Return(
					fmt.Errorf("error while setting NVMe namespace comment"))
			},
			wantErr:       assert.Error,
			assertMessage: "Expected NVMe namespace set comment to fail, but it succeeded",
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

func TestOntapASANVMeStorageDriver_CreateASANVMeNamespaceInternalID(t *testing.T) {
	svm := "svm_test"
	name := "namespace_test"
	expectedID := "/svm/svm_test/namespace/namespace_test"

	_, driver := newMockOntapASANVMeDriver(t)
	actualID := driver.CreateASANVMeNamespaceInternalID(svm, name)

	assert.Equal(t, expectedID, actualID, "The namespace internal ID does not match")
}

func TestOntapASANVMeStorageDriver_CreateNVMeNamespaceCommentString(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)
	nsAttr := map[string]string{
		nsAttributeFSType:    "ext4",
		nsAttributeLUKS:      "luks",
		nsAttributeDriverCtx: "docker",
	}

	// Case: Invalid comment json
	nsCommentString := `{"nsAttribute":{"LUKS":"not a valid json object"}`

	nsComment, err := driver.createNVMeNamespaceCommentString(ctx, nsAttr, 10)

	assert.Error(t, err, "Expected error when comment is invalid json format")
	assert.Equal(t, "", nsComment, "Expected comment to be empty.")

	// Case: comment string exceeds max length
	nsCommentString = `{"nsAttribute":{"LUKS":"luks","com.netapp.ndvp.fstype":"ext4","driverContext":"docker"}}`

	// Comment string exceeds max length
	nsComment, err = driver.createNVMeNamespaceCommentString(ctx, nsAttr, 10)

	assert.ErrorContains(t, err, "exceeds the character limit")
	assert.Equal(t, "", nsComment, "Comment has garbage string.")

	// Case: Success case
	nsComment, err = driver.createNVMeNamespaceCommentString(ctx, nsAttr, nsMaxCommentLength)

	assert.NoError(t, err, "Failed to get namespace comment.")
	assert.Equal(t, nsCommentString, nsComment, "Incorrect namespace comment.")
}

func TestOntapASANVMeStorageDriver_ParseNVMeNamespaceCommentString(t *testing.T) {
	_, driver := newMockOntapASANVMeDriver(t)

	nsCommentString := `{"nsAttribute":{"LUKS":"luks","com.netapp.ndvp.fstype":"ext4","formatOptions":"-E nodiscard","driverContext":"docker"}}`

	nsComment, err := driver.ParseNVMeNamespaceCommentString(ctx, nsCommentString)

	assert.NoError(t, err)
	assert.Equal(t, "ext4", nsComment[nsAttributeFSType])
	assert.Equal(t, "luks", nsComment[nsAttributeLUKS])
	assert.Equal(t, "docker", nsComment[nsAttributeDriverCtx])
	assert.Equal(t, "-E nodiscard", nsComment[FormatOptions])
}

func TestCreateNSCommentBasedOnSourceNS(t *testing.T) {
	ctx := context.Background()

	_, driver := newMockOntapASANVMeDriver(t)
	driver.Config.DriverContext = driverContextCSI

	volConfig := &storage.VolumeConfig{
		Name:           "testNamespace",
		InternalName:   "testNamespace",
		Size:           "1Gi",
		FileSystem:     "ext4",
		FormatOptions:  "-E nodiscard",
		LUKSEncryption: "true",
	}

	pool := storage.NewStoragePool(nil, "fakepool")
	unsetPool := storage.NewStoragePool(nil, drivers.UnsetPool)

	// Define test cases
	tests := []struct {
		name                 string
		getSourceNsComment   func() string
		storagePool          storage.Pool
		getDriverLabelConfig func() map[string]string
		getExpectedResult    func() string
		expectedError        bool
	}{
		{
			name: "No comment set on source Namespace",
			getSourceNsComment: func() string {
				return ""
			},
			storagePool: pool,
			getDriverLabelConfig: func() map[string]string {
				return map[string]string{
					"provisioning": "cluster=my-cluster",
				}
			},
			getExpectedResult: func() string {
				// New set of comment should be created taking values from volConfig and driver config
				labels, _ := ConstructLabelsFromConfigs(ctx, pool, volConfig,
					driver.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
				newNsComment := map[string]string{
					nsAttributeFSType:    volConfig.FileSystem,
					nsAttributeLUKS:      volConfig.LUKSEncryption,
					nsAttributeDriverCtx: string(driver.Config.DriverContext),
					FormatOptions:        volConfig.FormatOptions,
					nsLabels:             labels,
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, newNsComment, nsMaxCommentLength)
				return commentStr
			},
			expectedError: false,
		},
		{
			name: "Trident set comment on source Namespace",
			getSourceNsComment: func() string {
				existingNsComment := map[string]string{
					nsAttributeFSType:    "xfs",
					nsAttributeLUKS:      "false",
					nsAttributeDriverCtx: "docker",
					FormatOptions:        "--trim",
					nsLabels:             "{\"provisioning\":{\"template\":\"testSU\"}}",
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)
				return commentStr
			},
			storagePool: unsetPool,
			getDriverLabelConfig: func() map[string]string {
				return map[string]string{
					"provisioning": "cluster=my-cluster",
				}
			},
			getExpectedResult: func() string {
				storagePoolTemp := ConstructPoolForLabels(driver.Config.NameTemplate, driver.Config.Labels)
				labels, _ := ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
					driver.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
				newNsComment := map[string]string{
					nsAttributeFSType:    volConfig.FileSystem,
					nsAttributeLUKS:      volConfig.LUKSEncryption,
					nsAttributeDriverCtx: string(driver.Config.DriverContext),
					FormatOptions:        volConfig.FormatOptions,
					nsLabels:             labels,
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, newNsComment, nsMaxCommentLength)
				return commentStr
			},
			expectedError: false,
		},
		{
			name: "Comment not set by Trident on source Namespace",
			getSourceNsComment: func() string {
				return "some custom comment not set by Trident"
			},
			storagePool: unsetPool,
			getDriverLabelConfig: func() map[string]string {
				return map[string]string{
					"provisioning": "cluster=my-cluster",
				}
			},
			getExpectedResult: func() string {
				return ""
			},
			expectedError: false,
		},
		{
			name: "Comment set by Trident, but modified on source Namespace",
			getSourceNsComment: func() string {
				existingNsComment := map[string]string{
					nsAttributeFSType:    "xfs",
					nsAttributeLUKS:      "false",
					nsAttributeDriverCtx: "docker",
					FormatOptions:        "--trim",
					nsLabels:             "{\"provisioning-key-not-present\":{\"template\":\"testSU\"}}",
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)
				return commentStr
			},
			storagePool: nil,
			getDriverLabelConfig: func() map[string]string {
				return map[string]string{
					"provisioning": "cluster=my-cluster",
				}
			},
			getExpectedResult: func() string {
				// Though comment on source namespace seems to be set by Trident, the "provisioning" key is modified
				// Hence, we can't overwrite it.
				return ""
			},
			expectedError: false,
		},
		{
			name: "Error constructing base label, label length more than max length",
			getSourceNsComment: func() string {
				existingNsComment := map[string]string{
					nsAttributeFSType:    "xfs",
					nsAttributeLUKS:      "false",
					nsAttributeDriverCtx: "docker",
					FormatOptions:        "--trim",
					nsLabels:             "{\"provisioning\":{\"template\":\"testSU\"}}",
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)
				return commentStr
			},
			storagePool: nil,
			getDriverLabelConfig: func() map[string]string {
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

				return map[string]string{
					"provisioning": longLabel,
				}
			},
			getExpectedResult: func() string {
				return ""
			},
			expectedError: true,
		},
		{
			name: "Error constructing comment with metadata",
			getSourceNsComment: func() string {
				existingNsComment := map[string]string{
					nsAttributeFSType:    "xfs",
					nsAttributeLUKS:      "false",
					nsAttributeDriverCtx: "docker",
					FormatOptions:        "--trim",
					nsLabels:             "{\"provisioning\":{\"template\":\"testSU\"}}",
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, existingNsComment, nsMaxCommentLength)
				return commentStr
			},
			storagePool: nil,
			getDriverLabelConfig: func() map[string]string {
				// Label length is less than max length of 254 character;
				// but after combining metadata, it will exceed 254 character
				longLabelLessThan254Char := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
					"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ"

				return map[string]string{
					"provisioning": longLabelLessThan254Char,
				}
			},
			getExpectedResult: func() string {
				return ""
			},
			expectedError: true,
		},
	}

	// Run test cases
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Perform some initialisations
			driver.Config.Labels = test.getDriverLabelConfig()

			sourceNs := &api.NVMeNamespace{
				Comment: test.getSourceNsComment(),
			}

			// Get the namespace comment
			result, err := driver.createNSCommentBasedOnSourceNS(
				ctx, volConfig, sourceNs, test.storagePool)

			// Check for correctness
			if test.expectedError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "expected no error but got one")
				assert.Equal(t, test.getExpectedResult(), result, "expected result does not match")
			}
		})
	}
}

func TestGenerateNSCommentWithMetadata(t *testing.T) {
	ctx := context.Background()

	_, driver := newMockOntapASANVMeDriver(t)
	driver.Config.DriverContext = driverContextCSI

	volConfig := &storage.VolumeConfig{
		Name:           "testNamespace",
		InternalName:   "testNamespace",
		Size:           "1Gi",
		FileSystem:     "ext3",
		FormatOptions:  "-E nodiscard",
		LUKSEncryption: "true",
	}

	fakePool := storage.NewStoragePool(nil, "fakepool")
	fakePool.InternalAttributes()[FileSystemType] = filesystem.Ext4
	fakePool.InternalAttributes()[SplitOnClone] = "true"
	fakePool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cluster": "my-cluster-a",
	})

	// Define test cases
	tests := []struct {
		name                 string
		storagePool          *storage.StoragePool
		getDriverLabelConfig func() map[string]string
		getExpectedResult    func() string
		expectedError        bool
	}{
		{
			name:        "Positive case",
			storagePool: fakePool,
			getDriverLabelConfig: func() map[string]string {
				return map[string]string{
					"provisioning": "cluster=my-cluster",
				}
			},
			getExpectedResult: func() string {
				labels, _ := ConstructLabelsFromConfigs(ctx, fakePool, volConfig,
					driver.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
				newNsComment := map[string]string{
					nsAttributeFSType:    volConfig.FileSystem,
					nsAttributeLUKS:      volConfig.LUKSEncryption,
					nsAttributeDriverCtx: string(driver.Config.DriverContext),
					FormatOptions:        volConfig.FormatOptions,
					nsLabels:             labels,
				}

				commentStr, _ := driver.createNVMeNamespaceCommentString(ctx, newNsComment, nsMaxCommentLength)
				return commentStr
			},
			expectedError: false,
		},
		{
			name:        "Label length more than max length",
			storagePool: storage.NewStoragePool(nil, drivers.UnsetPool),
			getDriverLabelConfig: func() map[string]string {
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

				return map[string]string{
					"provisioning": longLabel,
				}
			},
			getExpectedResult: func() string {
				return ""
			},
			expectedError: true,
		},
		{
			name:        "Error constructing comment with metadata",
			storagePool: storage.NewStoragePool(nil, drivers.UnsetPool),
			getDriverLabelConfig: func() map[string]string {
				// Label length is less than max length of 254 character;
				// but after combining metadata, it will exceed 254 character
				longLabelLessThan254Char := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
					"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ"

				return map[string]string{
					"provisioning": longLabelLessThan254Char,
				}
			},
			getExpectedResult: func() string {
				return ""
			},
			expectedError: true,
		},
	}

	// Run test cases
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Perform some initialisations
			driver.Config.Labels = test.getDriverLabelConfig()

			// Get the namespace comment
			result, err := driver.createNSCommentWithMetadata(
				ctx, volConfig, test.storagePool)

			// Check for correctness
			if test.expectedError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "expected no error but got one")
				assert.Equal(t, test.getExpectedResult(), result, "expected result does not match")
			}
		})
	}
}
