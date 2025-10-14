// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/acp"
	tridentconfig "github.com/netapp/trident/config"
	mockacp "github.com/netapp/trident/mocks/mock_acp"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var (
	driverContextCSI     = tridentconfig.ContextCSI
	driverContextDocker  = tridentconfig.ContextDocker
	mockError            = errors.New("mock error")
	invalidStoragePrefix = convert.ToPtr("$invalid$")
	validStoragePrefix   = convert.ToPtr("trident")
	nameMoreThan64char   = "foofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoofoo"
	volInternalID        = "/svm/svm0_nas/flexvol/trident_qtree_pool_trident_GLVRJSQGLP/qtree/trident_pvc_92c02355"
	flexvol              = "trident_qtree_pool_trident_GLVRJSQGLP"
)

func newNASQtreeStorageDriver(api api.OntapAPI) *NASQtreeStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	config.ManagementLIF = ONTAPTEST_LOCALHOST
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "ontap-nas-qtree-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas-economy"
	config.StoragePrefix = validStoragePrefix
	config.LUKSEncryption = "false"

	nasqtreeDriver := &NASQtreeStorageDriver{}
	nasqtreeDriver.Config = *config
	nasqtreeDriver.qtreesPerFlexvol = defaultQtreesPerFlexvol
	nasqtreeDriver.quotaResizeMap = make(map[string]bool)
	nasqtreeDriver.cloneSplitTimers = &sync.Map{}

	nasqtreeDriver.API = api

	return nasqtreeDriver
}

func newMockOntapNasQtreeDriver(t *testing.T) (*mockapi.MockOntapAPI, *NASQtreeStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newNASQtreeStorageDriver(mockAPI)
	return mockAPI, driver
}

func newMockQtree(name, volume string) *api.Qtree {
	return &api.Qtree{
		ExportPolicy:    "exportPolicy",
		Name:            name,
		UnixPermissions: DefaultUnixPermissions,
		SecurityStyle:   DefaultSecurityStyleNFS,
		Volume:          volume,
		Vserver:         "vserver1",
	}
}

func newMockHousekeepingTask(driver *NASQtreeStorageDriver) *HousekeepingTask {
	driver.housekeepingWaitGroup = &sync.WaitGroup{}
	task := &HousekeepingTask{
		Name:         "mockTask",
		Ticker:       time.NewTicker(time.Duration(1) * time.Second),
		InitialDelay: 1 * time.Millisecond,
		Done:         make(chan struct{}),
		Tasks:        []func(ctx2 context.Context){newMockTask()},
		Driver:       driver,
	}

	return task
}

func newMockTask() func(inputCtx context.Context) {
	mockTask := func(inputCtx context.Context) {
		// Do nothing
	}
	return mockTask
}

func TestGetTelemetry_Success(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.telemetry = &Telemetry{
		Plugin:        driver.Name(),
		SVM:           "SVM1",
		StoragePrefix: *driver.Config.StoragePrefix,
		Driver:        driver,
		done:          make(chan struct{}),
		stopped:       false,
	}

	assert.True(t, reflect.DeepEqual(driver.telemetry, driver.GetTelemetry()))
}

func TestBackendName_WithSetInConfig(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.BackendName = "myNASEcoBackend"

	result := driver.BackendName()

	assert.Equal(t, "myNASEcoBackend", result, "Backend name mismatch")
}

func TestBackendName_WithDefault(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.BackendName = ""
	driver.Config.DataLIF = "[127.0.0.1:0]"

	result := driver.BackendName()

	assert.Equal(t, "ontapnaseco_127.0.0.1.0", result, "Backend name mismatch")
}

func TestInitialize_Success(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, configJSON, secrets := getStructsForInitializeDriver()

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil

	// Add various expect to mockAPI
	addCommonExpectToMockApiForInitialize(mockAPI)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, nil)

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, *configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that no error occurs and that driver is initialized
	assert.NoError(t, result, "Expected nil in initialize, got error")
	assert.True(t, driver.initialized, "Driver not initialized")
	assert.Equal(t, "trident_qtree_pool_t_e_s_t_", driver.flexvolNamePrefix, "Invalid FlexvolNamePrefix")
}

func TestInitializeWithNameTemplate_Success(t *testing.T) {
	nameTemplate := "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"

	// Get structs needed for initializing driver
	commonConfig, _, secrets := getStructsForInitializeDriver()

	// Use configJSON with the nameTemplate
	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-economy",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "aggr1",
		"defaults": {
				"nameTemplate": "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
		},
		"username":          "dummyuser",
		"password":          "dummypassword",
		"qtreesPerFlexvol":  ""
	}`

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil

	// Add various expect to mockAPI
	addCommonExpectToMockApiForInitialize(mockAPI)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, nil)

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that no error occurs and that driver is initialized
	assert.NoError(t, result, "Expected nil in initialize, got error")
	assert.True(t, driver.initialized, "Driver not initialized")
	assert.Equal(t, "trident_qtree_pool_t_e_s_t_", driver.flexvolNamePrefix, "Invalid FlexvolNamePrefix")
	for _, pool := range driver.physicalPools {
		assert.Equal(t, nameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestInitializeWith_NameTemplateDefineInStoragePool(t *testing.T) {
	expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"

	// Get structs needed for initializing driver
	commonConfig, _, secrets := getStructsForInitializeDriver()

	// Use configJSON with the nameTemplate
	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-economy",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "aggr1",
		"storage": [
	     {
	        "defaults":
	         {
				"nameTemplate": "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
	         }
	     }
	    ],
		"username":          "dummyuser",
		"password":          "dummypassword",
		"qtreesPerFlexvol":  ""
	}`

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil

	// Add various expect to mockAPI
	addCommonExpectToMockApiForInitialize(mockAPI)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, nil)

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that no error occurs and that driver is initialized
	assert.NoError(t, result, "Expected nil in initialize, got error")
	assert.True(t, driver.initialized, "Driver not initialized")
	assert.Equal(t, "trident_qtree_pool_t_e_s_t_", driver.flexvolNamePrefix, "Invalid FlexvolNamePrefix")
	for _, pool := range driver.virtualPools {
		assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestInitializeWith_NameTemplateDefineInBothPool(t *testing.T) {
	expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"

	// Get structs needed for initializing driver
	commonConfig, _, secrets := getStructsForInitializeDriver()

	// Use configJSON with the nameTemplate
	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-economy",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "aggr1",
		"defaults": {
			"nameTemplate": "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
		},
		"storage": [
	     {
	        "defaults":
	         {
				"nameTemplate": "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
	         }
	     }
	    ],
		"username":          "dummyuser",
		"password":          "dummypassword",
		"qtreesPerFlexvol":  ""
	}`

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil

	// Add various expect to mockAPI
	addCommonExpectToMockApiForInitialize(mockAPI)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, nil)

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that no error occurs and that driver is initialized
	assert.NoError(t, result, "Expected nil in initialize, got error")
	assert.True(t, driver.initialized, "Driver not initialized")
	assert.Equal(t, "trident_qtree_pool_t_e_s_t_", driver.flexvolNamePrefix, "Invalid FlexvolNamePrefix")
	for _, pool := range driver.virtualPools {
		assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestInitialize_WithInvalidDriverAPI(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, configJSON, secrets := getStructsForInitializeDriver()

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Add various expect to mockAPI
	addCommonExpectToMockApiForInitialize(mockAPI)

	// Setting API to an ivalid value.
	driver.API = nil
	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, *configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that error occurs
	assert.Error(t, result, "expected error in initialize with invalid API, got nil")
}

func TestInitialize_WithInvalidConfigJson(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, configJSON, secrets := getStructsForInitializeDriver()

	// Modify configJSON to be of invalid format
	invalidJson := strings.Replace(*configJSON, "{", "", -1)
	configJSON = &invalidJson

	// Create mock driver
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, *configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that error occurs
	assert.Error(t, result, "Expected error in initialize with invalid configJSON, got nil")
}

func TestInitialize_WithInvalidDriverContext(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, configJSON, secrets := getStructsForInitializeDriver()

	// Create mock driver
	_, driver := newMockOntapNasQtreeDriver(t)

	// Initialize
	result := driver.Initialize(ctx, "invalid-driver-context", *configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert that error occurs
	assert.Error(t, result, "Expected error in initialize with invalid driver context, got nil")
}

func TestInitialize_WithDifferentQtreePerFlexvol(t *testing.T) {
	// Create set of test cases related to qtreePerFlexvol value
	tests := []struct {
		name                    string
		configJSON              string
		expectedError           string
		expectedQtreePerFlexvol int
		additionalExpect        bool
	}{
		{
			name: "no qtreePerFlexvol set in config",
			configJSON: `
					{
						"version":           1,
						"storageDriverName": "ontap-nas-economy",
						"managementLIF":     "127.0.0.1:0",
						"svm":               "SVM1",
						"aggregate":         "aggr1",
						"username":          "dummyuser",
						"password":          "dummypassword",
						"qtreesPerFlexvol":  ""
					}`,
			expectedQtreePerFlexvol: defaultQtreesPerFlexvol,
			additionalExpect:        true,
		},
		{
			name: "valid qtreePerFlexvol as per config",
			configJSON: `
					{
						"version":           1,
						"storageDriverName": "ontap-nas-economy",
						"managementLIF":     "127.0.0.1:0",
						"svm":               "SVM1",
						"aggregate":         "aggr1",
						"username":          "dummyuser",
						"password":          "dummypassword",
						"qtreesPerFlexvol":  "120"
					}`,
			expectedQtreePerFlexvol: 120,
			additionalExpect:        true,
		},
		{
			name: "invalid value qtreePerFlexvol",
			configJSON: `
					{
						"version":           1,
						"storageDriverName": "ontap-nas-economy",
						"managementLIF":     "127.0.0.1:0",
						"svm":               "SVM1",
						"aggregate":         "aggr1",
						"username":          "dummyuser",
						"password":          "dummypassword",
						"qtreesPerFlexvol":  "invalid-value"
					}`,
			expectedError: "invalid config value for qtreesPerFlexvol",
		},
		{
			name: "qtreePerFlexVol less than minimum qtreePerFlexvol",
			configJSON: `
					{
						"version":           1,
						"storageDriverName": "ontap-nas-economy",
						"managementLIF":     "127.0.0.1:0",
						"svm":               "SVM1",
						"aggregate":         "aggr1",
						"username":          "dummyuser",
						"password":          "dummypassword",
						"qtreesPerFlexvol":  "5"
					}`,
			expectedError: fmt.Sprintf("invalid config value for qtreesPerFlexvol (minimum is %d)",
				minQtreesPerFlexvol),
		},
		{
			name: "qtreePerFlexVol more than maximum qtreePerFlexvol",
			configJSON: `
					{
						"version":           1,
						"storageDriverName": "ontap-nas-economy",
						"managementLIF":     "127.0.0.1:0",
						"svm":               "SVM1",
						"aggregate":         "aggr1",
						"username":          "dummyuser",
						"password":          "dummypassword",
						"qtreesPerFlexvol":  "400"
					}`,
			expectedError: fmt.Sprintf("invalid config value for qtreesPerFlexvol (maximum is %d)",
				maxQtreesPerFlexvol),
		},
	}

	// Create mock driver and mockAPI
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	addCommonExpectToMockApiForInitialize(mockAPI)

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			driver.Config.CommonStorageDriverConfig = nil

			// Get structs needed for initializing driver
			commonConfig, _, secrets := getStructsForInitializeDriver()

			// Add additional expect calls to mockAPI if needed
			if test.additionalExpect {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{}}, nil)
			}

			// Initialize
			result := driver.Initialize(ctx, driverContextDocker, test.configJSON,
				commonConfig, secrets, BackendUUID)

			if test.expectedError != "" {
				assert.Contains(tt, result.Error(), test.expectedError,
					"Expected error when invalid qtreePerFlexvol, got nil ")
			} else {
				assert.Equal(tt, test.expectedQtreePerFlexvol, driver.qtreesPerFlexvol,
					"Incorrect value of qtreePerFlexvol")
			}
		})
	}
}

func TestInitialize_WithNoStoragePool(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, _, secrets := getStructsForInitializeDriver()

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	addCommonExpectToMockApiForInitialize(mockAPI)

	// Provide a configJSON which has aggregate different than that returned by API
	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-economy",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword",
		"qtreesPerFlexvol":  "",
        "autoExportPolicy":  true
	}`

	expectedErrString := "could not configure storage pools"

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, configJSON,
		commonConfig, secrets, BackendUUID)

	// Assert on the expected error
	assert.Error(t, result, "Expected error in initialize with no storage pool, got nil")
	assert.Contains(t, result.Error(), expectedErrString)
}

func TestInitialize_WithNoDataLIFs(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, _, secrets := getStructsForInitializeDriver()

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().GetSVMUUID().AnyTimes().Return(uuid.New().String())
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{"aggr1"}, nil)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).AnyTimes().Return(map[string]string{}, nil)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).AnyTimes().Return([]string{}, nil)

	// Provide a configJSON
	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-economy",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"username":          "dummyuser",
		"password":          "dummypassword",
		"qtreesPerFlexvol":  "",
        "autoExportPolicy":  true
	}`

	// Initialize
	result := driver.Initialize(ctx, driverContextCSI, configJSON,
		commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "Expected error in initialize with invalid storage prefix, got nil")
}

func getStructsForInitializeDriver() (*drivers.CommonStorageDriverConfig, *string, map[string]string) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-economy",
		BackendName:       "myOntapNasEcoBackend",
		DriverContext:     driverContextCSI,
		DebugTraceFlags:   debugTraceFlags,
		StoragePrefix:     convert.ToPtr("t-e-s-t"),
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-economy",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "aggr1",
		"username":          "dummyuser",
		"password":          "dummypassword",
		"qtreesPerFlexvol":  ""
	}`

	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	return commonConfig, &configJSON, secrets
}

func addCommonExpectToMockApiForInitialize(mockAPI *mockapi.MockOntapAPI) {
	mockAPI.EXPECT().GetSVMUUID().AnyTimes().Return(uuid.New().String())
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{"aggr1"}, nil)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().GetSVMAggregateAttributes(ctx).AnyTimes().Return(map[string]string{}, nil)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).AnyTimes().Return([]string{"10.0.0.1"}, nil)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).AnyTimes().Return(map[int]string{}, nil)
	mockAPI.EXPECT().ExportRuleCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaResize(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), gomock.Any(), false, "heartbeat",
		gomock.Any(), gomock.Any(), gomock.Any(), tridentconfig.OrchestratorName, gomock.Any()).AnyTimes().Return()
}

func TestInitialized(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)

	driver.initialized = true
	assert.Equal(t, driver.Initialized(), driver.initialized, "Expected true initialized state, got false")
}

func TestTerminate_Success(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	driver.Config.AutoExportPolicy = true
	driver.telemetry = nil
	driver.initialized = true

	mockAPI.EXPECT().ExportPolicyDestroy(ctx, gomock.Any()).Return(nil)
	mockAPI.EXPECT().Terminate().AnyTimes()

	driver.Terminate(ctx, BackendUUID)

	assert.False(t, driver.initialized, "Expected driver to terminate, got initialized")
}

func TestTerminate_WithErrorInApiOperation(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.telemetry = &Telemetry{
		Plugin:        driver.Name(),
		SVM:           "SVM1",
		StoragePrefix: *driver.Config.StoragePrefix,
		Driver:        driver,
		done:          make(chan struct{}),
	}
	driver.initialized = true
	driver.housekeepingWaitGroup = &sync.WaitGroup{}
	driver.housekeepingTasks = map[string]*HousekeepingTask{"task1": newMockHousekeepingTask(driver)}

	mockAPI.EXPECT().ExportPolicyDestroy(ctx, gomock.Any()).Return(mockError)
	mockAPI.EXPECT().Terminate().AnyTimes()

	driver.Terminate(ctx, BackendUUID)

	assert.False(t, driver.initialized, "Expected driver to terminate, got initialized")
}

func TestValidate_Success(t *testing.T) {
	// Create a mock driver and mockAPI
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Provide expect for mockAPI
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return([]string{"10.0.0.0"}, nil)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, gomock.Any()).Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).Return(map[int]string{}, nil)
	mockAPI.EXPECT().ExportRuleCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	// Provide basic configuration for driver
	driver.Config = *newOntapStorageDriverConfig()
	driver.Config.LUKSEncryption = "false"
	driver.Config.StoragePrefix = validStoragePrefix

	// Validate
	result := driver.validate(ctx)

	// Assert all good
	assert.NoError(t, result, "Expected no error in validate, got error")
}

func TestValidate_WithLUKSEncryptionEnabled(t *testing.T) {
	// Create a mock driver
	_, driver := newMockOntapNasQtreeDriver(t)

	// Set the LUKS Encryption for driver to be true
	driver.Config.LUKSEncryption = "true"

	// Validate
	result := driver.validate(ctx)

	// Assert error occurs
	assert.Error(t, result, "Expected error in validate when LUKSEncryption true, got nil")
}

func TestValidate_WithInvalidStoragePrefix(t *testing.T) {
	// Create a mock driver and ensure LUKSEncryption is false
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LUKSEncryption = "false"

	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return([]string{"10.0.0.0"}, nil)

	// Set invalid value for storage prefix
	driver.Config.StoragePrefix = invalidStoragePrefix

	// Validate
	result := driver.validate(ctx)

	// Assert error occurs
	assert.Error(t, result, "Expected error in validate when StoragePrefix in invalid, got nil")
}

func TestValidate_WithInvalidStoragePool(t *testing.T) {
	// Create a mock driver and mockAPI
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return([]string{"10.0.0.0"}, nil)

	// Set invalid value for one of the attribute of storage pool
	physicalPools := map[string]storage.Pool{}
	virtualPools := map[string]storage.Pool{"test": getValidOntapNASPool()}
	driver.virtualPools = virtualPools
	driver.physicalPools = physicalPools
	driver.Config.NASType = sa.SMB
	driver.virtualPools["test"].InternalAttributes()[SpaceReserve] = "invalidValue"

	// Validate
	result := driver.validate(ctx)

	// Assert error occurs
	assert.Error(t, result, "Expected error in validate when storage pool in invalid, got nil")
}

func TestValidate_WithNoAutoExportPolicy(t *testing.T) {
	// Create a mock driver
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Ensure AutoExportPolicy is not set
	driver.Config.AutoExportPolicy = false
	driver.flexvolExportPolicy = "mock-export-policy"

	// Provide expect for mockAPI
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).Return([]string{"10.0.0.0"}, nil)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, gomock.Any()).Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).Return(map[int]string{}, nil)
	mockAPI.EXPECT().ExportRuleCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	// Validate
	result := driver.validate(ctx)

	// Assert no error occurs
	assert.NoError(t, result, "Expected no error in validate when AutoExportPolicy is false, got error")
}

func TestValidate_WithErrorInApiOperation(t *testing.T) {
	// Case 1: Error during validating driver
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).AnyTimes().Return(nil, mockError)
	result1 := driver.validate(ctx)
	assert.Error(t, result1, "Expected error when api fails to get Data LIF, got nil")

	// Case 2: Error during creating default export policy
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = false
	driver.flexvolExportPolicy = "mock-export-policy"
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, gomock.Any()).AnyTimes().Return([]string{"10.0.0.0"}, nil)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, driver.flexvolExportPolicy).AnyTimes().Return(mockError)
	result2 := driver.validate(ctx)
	assert.Error(t, result2, "Expected error when api fails to create export policy, got nil")
}

func TestCreateClone_NotSupportedWithoutRO(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	result := driver.CreateClone(ctx, nil, &storage.VolumeConfig{}, getValidOntapNASPool())
	assert.Error(t, result, "Expected error in CreateClone, got nil")
}

func TestCreateClone_Success_ROClone(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	srcVolConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		SnapshotDir:         "true",
	}

	volConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		ReadOnlyClone:       true,
	}

	flexVol := api.Volume{
		Name:        "flexvol",
		Comment:     "flexvol",
		SnapshotDir: convert.ToPtr(true),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeInfo(ctx, "trident_qtree_pool_trident_GLVRJSQGLP").Return(&flexVol, nil)

	result := driver.CreateClone(ctx, srcVolConfig, volConfig, nil)

	assert.NoError(t, result, "received error %v", result)
}

func TestCreateClone_FailureROCloneFalse(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	srcVolConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		SnapshotDir:         "true",
	}

	volConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		ReadOnlyClone:       false,
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	result := driver.CreateClone(ctx, srcVolConfig, volConfig, nil)

	assert.Error(t, result, "expected error")
}

func TestCreateClone_FailureWrongVolID(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	srcVolConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          "wrong-volume-id",
		CloneSourceSnapshot: "flexvol",
		SnapshotDir:         "true",
	}

	volConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		ReadOnlyClone:       true,
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	result := driver.CreateClone(ctx, srcVolConfig, volConfig, nil)

	assert.Error(t, result, "expected error")
}

func TestCreateClone_FailureNoVolInfo(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	srcVolConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		SnapshotDir:         "true",
	}

	volConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		ReadOnlyClone:       true,
	}

	flexVol := api.Volume{
		Name:        "flexvol",
		Comment:     "flexvol",
		SnapshotDir: convert.ToPtr(true),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeInfo(ctx, "trident_qtree_pool_trident_GLVRJSQGLP").Return(&flexVol, mockError)

	result := driver.CreateClone(ctx, srcVolConfig, volConfig, nil)

	assert.Error(t, result, "expected error")
}

func TestCreateClone_FailureSnapDirFalse(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	srcVolConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		SnapshotDir:         "false",
	}

	volConfig := &storage.VolumeConfig{
		Size:                "1g",
		Encryption:          "false",
		FileSystem:          "nfs",
		InternalID:          volInternalID,
		CloneSourceSnapshot: "flexvol",
		ReadOnlyClone:       true,
	}

	flexVol := api.Volume{
		Name:        "flexvol",
		Comment:     "flexvol",
		SnapshotDir: convert.ToPtr(false),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeInfo(ctx, "trident_qtree_pool_trident_GLVRJSQGLP").Return(&flexVol, nil)

	result := driver.CreateClone(ctx, srcVolConfig, volConfig, nil)

	assert.Error(t, result, "expected error")
}

func TestImport_NotSupported(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	result := driver.Import(ctx, &storage.VolumeConfig{}, "")
	assert.Error(t, result, "Expected error in Import, got nil")
}

func TestRename_NotSupported(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	result := driver.Rename(ctx, "", "")
	assert.Error(t, result, "Expected error in Rename, got nil")
}

func TestDestroy_Success(t *testing.T) {
	// Create a suitable volume config; have a volume name close to maxQtreeNameLength (64 char)
	svm := "SVM1"
	volName := nameMoreThan64char
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Name:         volName,
		InternalName: volNameInternal,
		InternalID:   fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", svm, volName, volNameInternal),
		Encryption:   "false",
		FileSystem:   "nfs",
	}

	// Create a default mock driver and mock api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// CASE 1: When qtree exists, assert on successful destroy
	mockAPI.EXPECT().QtreeExists(ctx, volConfig.InternalName, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QtreeRename(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().QtreeDestroyAsync(ctx, gomock.Any(), gomock.Any()).Return(nil)

	result1 := driver.Destroy(ctx, volConfig)
	assert.NoError(t, result1, "Expected no error in destroy, got error")

	// CASE 2: When qtree doesn't exist, assert on success
	mockAPI.EXPECT().QtreeExists(ctx, volConfig.InternalName, gomock.Any()).Return(false, volName, nil)

	result2 := driver.Destroy(ctx, volConfig)
	assert.NoError(t, result2, "Expected no error in destroy when qtree doesn't exist, got error")
}

func TestDestroy_WithInvalidInternalID(t *testing.T) {
	// Create a suitable volume config
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Name:         volName,
		InternalName: volNameInternal,
		InternalID:   "invalid-internalID",
		Encryption:   "false",
		FileSystem:   "nfs",
	}

	// Create a default mock driver
	_, driver := newMockOntapNasQtreeDriver(t)

	// Destroy qtree
	result := driver.Destroy(ctx, volConfig)

	// assert error
	assert.Error(t, result, "Expected error in destroy with invalid internalID, got nil")
}

func TestDestroy_WithErrorInApiOperation(t *testing.T) {
	// Create a suitable volume config
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Name:         volName,
		InternalName: volNameInternal,
		InternalID:   "",
		Encryption:   "false",
		FileSystem:   "nfs",
	}

	// CASE 1 : Error while checking qtree exists should throw error
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(false, volName, mockError)

	result1 := driver.Destroy(ctx, volConfig)
	assert.Error(t, result1, "Expected error when api failed to check qtree existence, got nil")

	// CASE 2 : Error while renaming qtree exists should throw error
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	mockAPI.EXPECT().QtreeRename(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)

	result2 := driver.Destroy(ctx, volConfig)
	assert.Error(t, result2, "Expected error when api failed to rename qtree, got nil")

	// CASE 3 : Error while destroying qtree should throw error
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	mockAPI.EXPECT().QtreeRename(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QtreeDestroyAsync(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)

	result3 := driver.Destroy(ctx, volConfig)
	assert.Error(t, result3, "Expected error when api failed to destroy qtree, got nil")

	// CASE 4 : Error while Qtree rename after error in destroying qtree should throw error
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	gomock.InOrder(
		mockAPI.EXPECT().QtreeRename(ctx, gomock.Any(), gomock.Any()).Return(nil),
		mockAPI.EXPECT().QtreeRename(ctx, gomock.Any(), gomock.Any()).Return(mockError),
	)
	mockAPI.EXPECT().QtreeDestroyAsync(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)

	result4 := driver.Destroy(ctx, volConfig)
	assert.Error(t, result4, "Expected error when api failed to rename qtree, got nil")
}

func TestPublish_Success_WithNASTypeNone(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:            "1g",
		Encryption:      "false",
		AccessInfo:      models.VolumeAccessInfo{NfsAccessInfo: models.NfsAccessInfo{NfsPath: "/testVol/testVolInternal"}},
		FileSystem:      "nfs",
		Name:            volName,
		InternalName:    volNameInternal,
		UnixPermissions: "",
		MountOptions:    "-o nfsvers=3",
	}

	// Create a mock driver driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.DataLIF = "10.0.0.0"
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)

	// Publish
	publishInfo := &models.VolumePublishInfo{}
	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NoError(t, result, "Expected no error in publish, got error")
	assert.Equal(t, fmt.Sprintf("/%s/%s", volName, volNameInternal), publishInfo.NfsPath, "NfsPath not correct")
	assert.Equal(t, driver.Config.DataLIF, publishInfo.NfsServerIP, "NfsServerIP server not correct")
	assert.Equal(t, sa.NFS, publishInfo.FilesystemType, "FilesystemType not correct")
	assert.Equal(t, volConfig.MountOptions, publishInfo.MountOptions, "MountOptions path not correct")
}

func TestPublish_Success_WithNASTypeSMB(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "cifs",
		Name:             volName,
		InternalName:     volNameInternal,
		PeerVolumeHandle: "SVM1:vol1",
		ImportNotManaged: false,
	}
	volConfig.AccessInfo.SMBPath = "/test_cifs_path"

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.NASType = sa.SMB
	driver.Config.DataLIF = "10.0.0.0"
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)

	// Publish
	publishInfo := &models.VolumePublishInfo{}
	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NoError(t, result, "Expected no error in publish, got error")
	assert.Equal(t, volConfig.AccessInfo.SMBPath, publishInfo.SMBPath, "SMB path not correct")
	assert.Equal(t, driver.Config.DataLIF, publishInfo.SMBServer, "SMB server not correct")
	assert.Equal(t, sa.SMB, publishInfo.FilesystemType, "FilesystemType not correct")
}

func TestPublish_WithDifferentInternalId(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(
		true, volName, nil)

	// Create set of test cases
	tests := []struct {
		name             string
		volConfig        *storage.VolumeConfig
		expectError      bool
		newVolInternalID string
	}{
		{
			name: "invalid internal ID in volume config",
			volConfig: &storage.VolumeConfig{
				Size:       "1g",
				Encryption: "false",
				FileSystem: "nfs",
				Name:       volName,
				InternalID: "invalid",
			},
			expectError: true,
		},
		{
			name: "empty internal ID in volume config",
			volConfig: &storage.VolumeConfig{
				Size:         "1g",
				Encryption:   "false",
				FileSystem:   "nfs",
				Name:         volName,
				InternalName: volNameInternal,
				InternalID:   "",
			},
			expectError:      false,
			newVolInternalID: fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", "SVM1", "testVol", "testVolInternal"),
		},
	}

	// run each test
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			// Publish
			result := driver.Publish(ctx, test.volConfig, &models.VolumePublishInfo{})

			if test.expectError {
				assert.Error(tt, result, "Expected error in publish, got nil")
			} else {
				assert.Equal(tt, test.newVolInternalID, test.volConfig.InternalID,
					"New InternalID of volume not correct")
			}
		})
	}
}

func TestPublish_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		Name:         volName,
		InternalName: volNameInternal,
	}

	// CASE 1 : Error while checking qtree exists should throw error
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(false, volName, mockError)

	result1 := driver.Publish(ctx, volConfig, &models.VolumePublishInfo{})
	assert.Error(t, result1, "Expected error when api failed to check qtree existence, got nil")

	// CASE 2: When Qtree does not exist
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(false, volName, nil)

	result2 := driver.Publish(ctx, volConfig, &models.VolumePublishInfo{})
	assert.Error(t, result2, "Expected error when qtree does not exist, got nil")
}

func TestPublishQtreeShare_WithDefaultPolicy_Success(t *testing.T) {
	qtreeName := "testQtree"
	flexVolName := "testFlexVol"
	nodeIP := "1.1.1.1"
	defaultPolicy := "default"

	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1", IPs: []string{nodeIP}})

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   false,
		Nodes:       nodes,
		HostName:    "node1",
	}

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, flexVolName).Return(
		&api.Qtree{ExportPolicy: defaultPolicy, Volume: flexVolName}, nil)
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).Times(0)
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), gomock.Any()).Times(0)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Times(0)
	mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	mockAPI.EXPECT().VolumeModifyExportPolicy(ctx, gomock.Any(), gomock.Any()).Times(0)

	// This handles the case where the customer originally created a backend with autoExportPolicy set to false and
	// mounted a volume and then later changed autoExportPolicy to true
	volConfig := &storage.VolumeConfig{ExportPolicy: defaultPolicy}
	driver.Config.AutoExportPolicy = true

	result := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)

	assert.Equal(t, "default", volConfig.ExportPolicy)
	assert.NoError(t, result, "Expected no error in publishQtreeShare, got error")
}

func TestPublishQtreeShare_LegacyVolumeWithEmptyPolicyInConfig_Success(t *testing.T) {
	qtreeName := "testQtree"
	flexVolName := "testFlexVol"
	nodeIP := "1.1.1.1"
	backendPolicy := getExportPolicyName(BackendUUID)

	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1", IPs: []string{nodeIP}})

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   false,
		Nodes:       nodes,
		HostName:    "node1",
	}

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, flexVolName).Return(
		&api.Qtree{ExportPolicy: backendPolicy, Volume: flexVolName}, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).Return(true, nil).Times(2)
	// Return node ip rules when asked for them
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), backendPolicy).Return(map[int]string{1: "1.1.1.1"}, nil).Times(2)
	mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, backendPolicy).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeModifyExportPolicy(ctx, flexVolName, backendPolicy).AnyTimes().Return(nil).Times(1)

	// This handles the case where the customer originally created and mounted a volume in trident version <=23.01
	// and then later upgraded to 24.10. This would have the volConfig.ExportPolicy = "".
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}
	volConfig := &storage.VolumeConfig{ExportPolicy: ""}

	result := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)

	assert.Equal(t, backendPolicy, volConfig.ExportPolicy)
	assert.NoError(t, result, "Expected no error in publishQtreeShare, got error")
}

func TestPublishQtreeShare_WithEmptyPolicy_Success(t *testing.T) {
	qtreeName := "testQtree"
	flexVolName := "testFlexVol"
	flexVolPolicy := getExportPolicyName(BackendUUID)
	nodeIP := "1.1.1.1"

	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1", IPs: []string{nodeIP}})

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   false,
		Nodes:       nodes,
		HostName:    "node1",
	}

	volConfig := &storage.VolumeConfig{ExportPolicy: getEmptyExportPolicyName("trident")}

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).AnyTimes().Return(true, nil)
	// Return an empty set of rules when asked for them
	ruleListCall1 := mockAPI.EXPECT().ExportRuleList(gomock.Any(), qtreeName).Return(make(map[int]string), nil)
	ruleListCall2 := mockAPI.EXPECT().ExportRuleList(gomock.Any(), flexVolPolicy).Return(make(map[int]string), nil)
	// Ensure that the rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), nodeIP,
		gomock.Any()).After(ruleListCall1).Return(nil)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), nodeIP,
		gomock.Any()).After(ruleListCall2).Return(nil)
	mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, qtreeName).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifyExportPolicy(ctx, flexVolName, flexVolPolicy).AnyTimes().Return(nil)

	// Ensure auto export policy is enabled and CIDRs set
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}

	result := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)

	assert.NoError(t, result, "Expected no error in publishQtreeShare, got error")
}

func TestPublishQtreeShare_WithBackendPolicy_Success(t *testing.T) {
	qtreeName := "testQtree"
	flexVolName := "testFlexVol"
	backendPolicy := getExportPolicyName(BackendUUID)
	nodeIP := "1.1.1.1"

	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1", IPs: []string{nodeIP}})

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   false,
		Nodes:       nodes,
		HostName:    "node1",
	}

	volConfig := &storage.VolumeConfig{ExportPolicy: backendPolicy}

	// CASE 1: Backend policy does not have the required node IP address
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).AnyTimes().Return(true, nil)
	// Return an empty set of rules when asked for them
	ruleListCall1 := mockAPI.EXPECT().ExportRuleList(gomock.Any(), backendPolicy).Return(make(map[int]string), nil)
	ruleListCall2 := mockAPI.EXPECT().ExportRuleList(gomock.Any(), backendPolicy).Return(make(map[int]string), nil)
	// Ensure that the rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), nodeIP,
		gomock.Any()).After(ruleListCall1).Return(nil)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), nodeIP,
		gomock.Any()).After(ruleListCall2).Return(nil)
	mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, backendPolicy).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifyExportPolicy(ctx, flexVolName, backendPolicy).AnyTimes().Return(nil)

	// Ensure auto export policy is enabled and CIDRs set
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}

	result := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)
	assert.NoError(t, result, "Expected no error in publishQtreeShare, got error")

	// CASE 2: Backend policy already have the required node IP address
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).AnyTimes().Return(true, nil)
	// Return node ip rules when asked for them
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), backendPolicy).Return(map[int]string{1: "1.1.1.1"}, nil)
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), backendPolicy).Return(map[int]string{1: "1.1.1.1"}, nil)
	mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, backendPolicy).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifyExportPolicy(ctx, flexVolName, backendPolicy).AnyTimes().Return(nil)

	// Ensure auto export policy is enabled and CIDRs set
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}

	result = driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)
	assert.NoError(t, result, "Expected no error in publishQtreeShare, got error")
}

func TestPublishQtreeShare_WithUnmanagedPublishInfo(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   true,
	}

	volConfig := &storage.VolumeConfig{ExportPolicy: "trident_empty"}

	// Create mock driver and api
	_, driver := newMockOntapNasQtreeDriver(t)

	result := driver.publishQtreeShare(ctx, volNameInternal, volName, volConfig, &publishInfo)

	assert.NoError(t, result, "Expected no error in publishQtreeShare, got error")
}

func TestPublishQtreeShare_WithErrorInApiOperation(t *testing.T) {
	// Create required info
	qtreeName := "testQtree"
	flexVolName := "testFlexVol"
	nodeIP := "1.1.1.1"

	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1", IPs: []string{nodeIP}})

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   false,
		Nodes:       nodes,
		HostName:    "node1",
	}

	volConfig := &storage.VolumeConfig{ExportPolicy: getEmptyExportPolicyName("trident")}

	// CASE 1: Error in checking if export policy exists
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).Return(false, mockError)

	result1 := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)
	assert.Error(t, result1, "Expected error when api failed to check export policy exists, got nil")

	// CASE 2: Error in modifying export policy
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}
	mockAPI.EXPECT().ExportPolicyExists(ctx, gomock.Any()).Return(true, nil)
	// Return an empty set of rules when asked for them
	ruleListCall := mockAPI.EXPECT().ExportRuleList(gomock.Any(), qtreeName).Return(make(map[int]string), nil)
	// Ensure that the rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), nodeIP,
		gomock.Any()).After(ruleListCall).Return(nil)
	mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)

	result2 := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)
	assert.Error(t, result2, "Expected error when api failed to check export policy exists, got nil")
}

func TestPublishQtreeShare_NodeNotPresentError(t *testing.T) {
	// Create required info
	qtreeName := "testQtree"
	flexVolName := "testFlexVol"

	nodes := make([]*models.Node, 0)

	publishInfo := models.VolumePublishInfo{
		BackendUUID: BackendUUID,
		Unmanaged:   false,
		Nodes:       nodes,
		HostName:    "node1",
	}

	volConfig := &storage.VolumeConfig{ExportPolicy: "trident_empty"}

	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true

	result1 := driver.publishQtreeShare(ctx, qtreeName, flexVolName, volConfig, &publishInfo)
	assert.Error(t, result1, "Expected error when node is not present in publish info, got nil")
}

func TestRestoreSnapshot_NotSupported(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	result := driver.RestoreSnapshot(ctx, &storage.SnapshotConfig{}, &storage.VolumeConfig{})
	assert.Error(t, result, "Expected error in RestoreSnapshot, got nil")
}

func TestGet_Success(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(
		true, volName, nil)

	result := driver.Get(ctx, volNameInternal)
	assert.NoError(t, result, "Expected no error in Get, got error")
}

func TestGet_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"

	// CASE 1: Error while checking for qtree existence
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(
		false, volName, mockError)

	result1 := driver.Get(ctx, volNameInternal)
	assert.Error(t, result1, "Expected error when api failed to check qtree existence, got nil")

	// CASE 2: Qtree does not exist
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(
		false, "", nil)

	result2 := driver.Get(ctx, volNameInternal)
	assert.Error(t, result2, "Expected error when qtree does not exist, got nil")
}

func TestEnsureFlexvolForQtree_Success_EligibleFlexvolFound(t *testing.T) {
	driverConfig := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
	}

	// Create mock driver and API
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Create a mock flexvol and ensure it is returned by api
	eligibleFlexvol, _ := MockGetVolumeInfo(ctx, "testVol")

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{eligibleFlexvol}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, gomock.Any()).AnyTimes().Return(0, nil)

	// Ensure flexvol for qtree
	resultFlexvol, result := driver.ensureFlexvolForQtree(
		ctx, "", "", "",
		"", false, convert.ToPtr(false), 0,
		driverConfig, "", "")

	assert.NoError(t, result, "Expected no error when eligible flexvol found, got error")
	assert.Equal(t, eligibleFlexvol.Name, resultFlexvol, "Incorrect flexvol returned")
}

func TestEnsureFlexvolForQtree_Success_NoEligibleFlexvol(t *testing.T) {
	driverConfig := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
	}

	// Create mock driver and API
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, gomock.Any()).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaOff(ctx, gomock.Any()).AnyTimes().Return(nil)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("off", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("on", nil),
	)

	resultFlexvol, result := driver.ensureFlexvolForQtree(
		ctx, "", "", "",
		"", false, convert.ToPtr(false), 0,
		driverConfig, "", "")

	// Expect no error as new flexvol is created when no eligible flexvol found
	assert.NoError(t, result, "Expected no error when no eligible flexvol found, got error")
	assert.NotNil(t, resultFlexvol, "Expected non nil flexvol, got nil")
}

func TestEnsureFlexvolForQtree_Success_NewFlexvolNotPermitted(t *testing.T) {
	// Get structs needed for initializing driver
	commonConfig, _, secrets := getStructsForInitializeDriver()

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	addCommonExpectToMockApiForInitialize(mockAPI)

	// Provide a configJSON which has aggregate different than that returned by API
	configJSON := `
	{
		"version":             1,
		"storageDriverName":   "ontap-nas-economy",
		"managementLIF":       "127.0.0.1:0",
		"svm":                 "SVM1",
		"aggregate":           "data",
		"username":            "dummyuser",
		"password":            "dummypassword",
        "denyNewVolumePools":  "true"
	}`

	// Initialize
	_ = driver.Initialize(ctx, driverContextCSI, configJSON, commonConfig, secrets, BackendUUID)

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{}, nil)

	resultFlexvol, result := driver.ensureFlexvolForQtree(
		ctx, "", "", "",
		"", false, convert.ToPtr(false), 0,
		&driver.Config, "", "")

	// Expect error as no new flexvol may be created
	assert.Equal(t, "", resultFlexvol, "Expected no Flexvol, got %s", resultFlexvol)
	assert.Error(t, result, "Expected error when needing to create new Flexvol, got nil")
}

func TestEnsureFlexvolForQtree_WithInvalidConfig(t *testing.T) {
	driverConfig := &drivers.OntapStorageDriverConfig{
		LimitVolumePoolSize: "invalid",
	}

	_, driver := newMockOntapNasQtreeDriver(t)

	_, result := driver.ensureFlexvolForQtree(
		ctx, "", "", "",
		"", false, convert.ToPtr(false), 0,
		driverConfig, "", "")

	assert.Error(t, result, "Expected error with invalid config, got nil")
}

func TestEnsureFlexvolForQtree_WithErrorInApiOperation(t *testing.T) {
	driverConfig := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
	}

	// CASE 1: Failure to get volume info
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return(
		nil, mockError)

	_, result1 := driver.ensureFlexvolForQtree(
		ctx, "", "", "",
		"", false, convert.ToPtr(false), 0,
		driverConfig, "", "")

	assert.Error(t, result1, "Expected error when api failed to list volumes, got nil")

	// CASE 2: Failure in creating new volume
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, gomock.Any()).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(mockError)

	_, result2 := driver.ensureFlexvolForQtree(
		ctx, "", "", "",
		"", false, convert.ToPtr(false), 0,
		driverConfig, "", "")

	assert.Error(t, result2, "Expected error when api failed to create volume, got nil")
}

func TestCreateFlexvolForQtree_Success_NasTypeNFS(t *testing.T) {
	// Create mock driver and API
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.NASType = sa.NFS
	addExpectForCreateFlexvolForQtree(mockAPI)

	resultFlexvol, result := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snap-policy-1",
		"", false, convert.ToPtr(false), "10", "export-policy-1",
	)

	assert.NoError(t, result, "Expected no error in create flexvol for qtree, got error")
	assert.NotNil(t, resultFlexvol, "Expected non-nil volume name, got nil")
}

func TestCreateFlexvolForQtree_Success_NasTypeSMB(t *testing.T) {
	// Create mock driver and API
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.NASType = sa.SMB
	addExpectForCreateFlexvolForQtree(mockAPI)

	resultFlexvol, result := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snap-policy-1",
		"", false, convert.ToPtr(false), "10", "export-policy-1",
	)

	assert.NoError(t, result, "Expected no error in create flexvol for qtree, got error")
	assert.NotNil(t, resultFlexvol, "Expected non-nil volume name, got nil")
}

func TestCreateFlexvolForQtree_WithInvalidSnapshotReserve(t *testing.T) {
	// Create mock driver and API
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	resultFlexvol, result := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snap-policy-1",
		"", false, convert.ToPtr(false), "invalid", "export-policy-1",
	)

	assert.Error(t, result, "Expected error when invalid snapshot reserve, got error")
	assert.Emptyf(t, resultFlexvol, "Expected empty volume name, got non-empty")
}

func TestCreateFlexvolForQtree_WithErrorInApiOperation(t *testing.T) {
	// CASE 1: Error in creating volume
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(mockError)
	resultFlexvol1, result1 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result1, "Expected error, got nil")
	assert.Emptyf(t, resultFlexvol1, "Expected empty volume name, got non-empty")

	// CASE 2: Error in disabling snapshot directory
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(mockError)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(nil)

	resultFlexvol2, result2 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result2, "Expected error when api failed to disable snapshot directory, got nil")
	assert.Emptyf(t, resultFlexvol2, "Expected empty volume name, got non-empty")

	// CASE 3: Error in destroying volume after error in disabling snapshot directory
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(mockError)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(mockError)

	resultFlexvol3, result3 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result3, "Expected error when api failed to destroy volume, got nil")
	assert.Emptyf(t, resultFlexvol3, "Expected empty volume name, got non-empty")

	// CASE 4: Error in mounting volume
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(nil)

	resultFlexvol4, result4 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result4, "Expected error when api failed to mount volume, got nil")
	assert.Emptyf(t, resultFlexvol4, "Expected empty volume name, got non-empty")

	// CASE 5: Error in destroying volume after error in mounting volume
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(mockError)

	resultFlexvol5, result5 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result5, "Expected error when api failed to destroy volume, got nil")
	assert.Emptyf(t, resultFlexvol5, "Expected empty volume name, got non-empty")

	// CASE 6: Error in adding quota entry
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).AnyTimes().Return(mockError)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(nil)

	resultFlexvol6, result6 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result6, "Expected error when api failed to add quota entry, got nil")
	assert.Emptyf(t, resultFlexvol6, "Expected empty volume name, got non-empty")

	// CASE 7: Error in destorying volume after error in adding quota entry
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).AnyTimes().Return(mockError)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(mockError)

	resultFlexvol7, result7 := driver.createFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", false, convert.ToPtr(false), "10", "export-policy-1")

	assert.Error(t, result7, "Expected error when api failed to destroy volume, got nil")
	assert.Emptyf(t, resultFlexvol7, "Expected empty volume name, got non-empty")
}

func addExpectForCreateFlexvolForQtree(mockAPI *mockapi.MockOntapAPI) {
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, gomock.Any(), false).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).AnyTimes().Return(nil)
	mockAPI.EXPECT().VolumeMount(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().Return(nil)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("off", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("on", nil),
	)
}

func TestFindFlexvolForQtree_Success_ZeroEligibleVolume(t *testing.T) {
	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test_"

	isEncrypt := convert.ToPtr(false)
	volAttrs := &api.Volume{
		Aggregates:      []string{"aggr1"},
		Encrypt:         isEncrypt,
		Name:            "test_*",
		SnapshotDir:     convert.ToPtr(false),
		SnapshotPolicy:  "snapshotPolicy",
		SpaceReserve:    "none",
		SnapshotReserve: 10,
		TieringPolicy:   "",
	}

	// Ensure 0 volumes are returned by api
	mockAPI.EXPECT().VolumeListByAttrs(ctx, volAttrs).AnyTimes().Return(api.Volumes{}, nil)

	resultFlexvol, result := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "10", false, isEncrypt,
		false, 0, 0,
	)

	assert.Emptyf(t, resultFlexvol, "Expected empty flexvol, got non-empty")
	assert.NoError(t, result, "Expected no error, got error")
}

func TestFindFlexvolForQtree_Success_OneEligibleVolume(t *testing.T) {
	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test_"

	isEncrypt := convert.ToPtr(false)
	volAttrs := &api.Volume{
		Aggregates:      []string{"aggr1"},
		Encrypt:         isEncrypt,
		Name:            "test_*",
		SnapshotDir:     convert.ToPtr(false),
		SnapshotPolicy:  "snapshotPolicy",
		SpaceReserve:    "none",
		SnapshotReserve: 10,
		TieringPolicy:   "",
	}

	eligibleVolume := &api.Volume{
		Name: "test_flexvol",
	}

	// Ensure 1 volume is returned by api
	mockAPI.EXPECT().VolumeListByAttrs(ctx, volAttrs).AnyTimes().Return(api.Volumes{eligibleVolume}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, eligibleVolume.Name).AnyTimes().Return(0, nil)

	resultFlexvol, result := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "10", false, isEncrypt,
		false, 0, 0,
	)

	assert.Equal(t, eligibleVolume.Name, resultFlexvol, "Expected one eligible flexvol, got empty")
	assert.NoError(t, result, "Expected no error, got error")
}

func TestFindFlexvolForQtree_Success_MultipleEligibleVolume(t *testing.T) {
	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test_"

	isEncrypt := convert.ToPtr(false)
	volAttrs := &api.Volume{
		Aggregates:      []string{"aggr1"},
		Encrypt:         isEncrypt,
		Name:            "test_*",
		SnapshotDir:     convert.ToPtr(false),
		SnapshotPolicy:  "snapshotPolicy",
		SpaceReserve:    "none",
		SnapshotReserve: 10,
		TieringPolicy:   "",
	}

	eligibleVolume1 := &api.Volume{Name: "test_flexvol1"}
	eligibleVolume2 := &api.Volume{Name: "test_flexvol2"}

	// Ensure multiple volumes are returned by api
	mockAPI.EXPECT().VolumeListByAttrs(ctx, volAttrs).AnyTimes().Return(api.Volumes{eligibleVolume1, eligibleVolume2},
		nil)
	mockAPI.EXPECT().QtreeCount(ctx, eligibleVolume1.Name).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().QtreeCount(ctx, eligibleVolume2.Name).AnyTimes().Return(0, nil)

	resultFlexvol, result := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "10", false, isEncrypt,
		false, 0, 0,
	)

	// Returned flexvol should be one of the eligible volumes
	ok := resultFlexvol == eligibleVolume1.Name || resultFlexvol == eligibleVolume2.Name

	assert.True(t, ok, "Expected one of the random flexvol from eligible volumes, got different")
	assert.NoError(t, result, "Expected no error, got error")
}

func TestFindFlexvolForQtree_Success_VolumeWithSizeMoreThanLimit(t *testing.T) {
	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test_"

	isEncrypt := convert.ToPtr(false)
	volAttrs := &api.Volume{
		Aggregates:      []string{"aggr1"},
		Encrypt:         isEncrypt,
		Name:            "test_*",
		SnapshotDir:     convert.ToPtr(false),
		SnapshotPolicy:  "snapshotPolicy",
		SpaceReserve:    "none",
		SnapshotReserve: 10,
		TieringPolicy:   "",
	}

	eligibleVolume1 := &api.Volume{Name: "test_flexvol1"}
	eligibleVolume2 := &api.Volume{Name: "test_flexvol2"}

	// Ensure volume is returned by api
	mockAPI.EXPECT().VolumeListByAttrs(ctx, volAttrs).AnyTimes().Return(api.Volumes{eligibleVolume1, eligibleVolume2},
		nil)
	mockAPI.EXPECT().QtreeCount(ctx, eligibleVolume1.Name).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().QtreeCount(ctx, eligibleVolume2.Name).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, eligibleVolume1.Name).AnyTimes().Return(volAttrs, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, eligibleVolume2.Name).AnyTimes().Return(nil, mockError)
	mockAPI.EXPECT().QuotaEntryList(ctx, eligibleVolume1.Name).AnyTimes().
		Return(api.QuotaEntries{&api.QuotaEntry{Target: "", DiskLimitBytes: 1 * 1024 * 1024 * 1024}}, nil)

	resultFlexvol, result := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "10", false, isEncrypt,
		true, 1073741824, 10,
	)

	// Assert that no volume is returned
	assert.Emptyf(t, resultFlexvol, "Expected empty flexvol, got non-empty")
	assert.NoError(t, result, "Expected no error, got error")
}

func TestFindFlexvolForQtree_WithInvalidSnapshotReserve(t *testing.T) {
	// Create mock driver
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test_"

	_, result := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "invalid", false, convert.ToPtr(false),
		true, 1073741824, 10,
	)

	// Assert error occurred
	assert.Error(t, result, "Expected error when invalid snapshot reserve, got no error")
}

func TestFindFlexvolForQtree_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"

	// CASE 1: Error in getting volume list
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(nil, mockError)

	resultFlexvol1, result1 := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "10", false, convert.ToPtr(false),
		false, 0, 0,
	)

	assert.Error(t, result1, "Expected error, got nil")
	assert.Emptyf(t, resultFlexvol1, "Expected empty flexvol, got non-empty")

	// CASE 2: Error in getting qtree count
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{volInfo}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, volName).AnyTimes().Return(0, mockError)

	resultFlexvol2, result2 := driver.findFlexvolForQtree(
		ctx, "aggr1", "none", "snapshotPolicy",
		"", "10", false, convert.ToPtr(false),
		false, 0, 0,
	)

	assert.Error(t, result2, "Expected error, got nil")
	assert.Emptyf(t, resultFlexvol2, "Expected empty flexvol, got non-empty")
}

func TestGetOptimalSizeForFlexvol_Success(t *testing.T) {
	// Create flexvol
	volName := "testvol"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.SnapshotReserve = 1
	volInfo.SnapshotSpaceUsed = 2000

	// Provide some quota entries which are present in flexvol
	quotaEntries := api.QuotaEntries{
		&api.QuotaEntry{Target: "", DiskLimitBytes: 10000},
		&api.QuotaEntry{Target: "", DiskLimitBytes: 10000},
	}

	// Space for new qtree
	newQtreeSizeInBytes := uint64(10000)

	// Create new mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, gomock.Any()).AnyTimes().Return(quotaEntries, nil)

	resultSize, result := driver.getOptimalSizeForFlexvol(ctx, volName, newQtreeSizeInBytes)

	assert.NoError(t, result, "Expected no error, got error")
	assert.NotEmptyf(t, resultSize, "Expected non-zero size, got nil")
}

func TestGetOptimalSizeForFlexvol_WithErrorInApiOperation(t *testing.T) {
	// CASE 1: Error in getting volume info
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).AnyTimes().Return(nil, mockError)
	_, result1 := driver.getOptimalSizeForFlexvol(ctx, "volName", uint64(10000))

	assert.Error(t, result1, "Expected error, got nil")

	// CASE 2: Error in getting hard disk limit
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).AnyTimes().Return(nil, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, gomock.Any()).AnyTimes().Return(nil, mockError)

	_, result2 := driver.getOptimalSizeForFlexvol(ctx, "volName", uint64(10000))

	assert.Error(t, result2, "Expected error, got nil")
}

func TestAddDefaultQuotaForFlexvol_Success(t *testing.T) {
	volName := "testvol"

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), volName, gomock.Any(), gomock.Any()).
		AnyTimes().Return(nil)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, volName).Return("off", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, volName).Return("on", nil),
	)

	result := driver.addDefaultQuotaForFlexvol(ctx, volName)

	assert.NoError(t, result, "Expected no error, got error")
}

func TestAddDefaultQuotaForFlexvol_WithErrorInApiOperation(t *testing.T) {
	volName := "testvol"

	// CASE 1: Error in quota set entry
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), volName, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)

	result1 := driver.addDefaultQuotaForFlexvol(ctx, volName)
	assert.Error(t, result1, "Expected error when api failed to set quota entry, got nil")

	// CASE 2: Error in disabling quota
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), volName, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).AnyTimes().Return("on", nil)
	mockAPI.EXPECT().QuotaOff(ctx, volName).AnyTimes().Return(mockError)

	result2 := driver.addDefaultQuotaForFlexvol(ctx, volName)
	assert.Nil(t, result2, "Expected no error when api failed to disable quota, got nil")

	// CASE 3: Error in enabling quota
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaSetEntry(ctx, gomock.Any(), volName, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).AnyTimes().Return("off", nil)
	mockAPI.EXPECT().QuotaOn(ctx, volName).AnyTimes().Return(mockError)

	result3 := driver.addDefaultQuotaForFlexvol(ctx, volName)
	assert.Nil(t, result3, "Expected no error when api failed to enable quota, got nil")
}

func TestSetQuotaForQtree_Success(t *testing.T) {
	volName := "testvol"
	sizeKB := uint64(10000)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaSetEntry(ctx, "", volName, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	result := driver.setQuotaForQtree(ctx, "", volName, sizeKB)
	assert.NoError(t, result, "Expected no error, got error")
	assert.True(t, driver.quotaResizeMap[volName])
}

func TestSetQuotaForQtree_WithErrorInApiOperation(t *testing.T) {
	volName := "testvol"
	sizeKB := uint64(10000)

	// CASE: Error in quota set entry
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaSetEntry(ctx, "", volName, gomock.Any(), gomock.Any()).AnyTimes().Return(mockError)

	result := driver.setQuotaForQtree(ctx, "", volName, sizeKB)
	assert.Error(t, result, "Expected error, got nil")
	assert.False(t, driver.quotaResizeMap[volName])
}

func TestDisableQuotas_Success(t *testing.T) {
	volName := "testvol"

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("on", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("off", nil),
	)
	mockAPI.EXPECT().QuotaOff(ctx, volName).AnyTimes().Return(nil)

	result := driver.disableQuotas(ctx, volName, true)

	assert.NoError(t, result, "Expected no error, got error")
}

func TestDisableQuotas_WithCorruptStatus(t *testing.T) {
	volName := "testvol"

	// CASE 1: Corrupt status at very first time
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaStatus(ctx, volName).AnyTimes().Return("corrupt", nil)

	result1 := driver.disableQuotas(ctx, volName, true)
	assert.Error(t, result1, "Expected error when quota status is corrupt, got nil")

	// CASE 2: Corrupt status during wait period
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, volName).Return("on", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, volName).Return("corrupt", nil),
	)
	mockAPI.EXPECT().QuotaOff(ctx, volName).AnyTimes().Return(nil)

	result2 := driver.disableQuotas(ctx, volName, true)
	assert.Error(t, result2, "Expected error when quota status is corrupt, got no error")
}

func TestDisableQuotas_WithErrorInApiOperation(t *testing.T) {
	volName := "testvol"

	// CASE 1: Error when getting quota status
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).AnyTimes().Return("", mockError)

	result1 := driver.disableQuotas(ctx, volName, true)
	assert.Error(t, result1, "Expected error when api failed to get quota status, got nil")

	// CASE 2: Error when setting quota off
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).AnyTimes().Return("on", nil)
	mockAPI.EXPECT().QuotaOff(ctx, volName).AnyTimes().Return(mockError)

	result2 := driver.disableQuotas(ctx, volName, true)
	assert.Error(t, result2, "Expected error when api failed to disable quota, got nil")

	// CASE 3: Error in getting quota status during wait period
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("on", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("", mockError),
	)
	mockAPI.EXPECT().QuotaOff(ctx, volName).Return(nil)

	result3 := driver.disableQuotas(ctx, volName, true)
	assert.Error(t, result3, "Expected error when api failed to fetch quota status, got nil")
}

func TestEnableQuotas_Success(t *testing.T) {
	volName := "testvol"

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("off", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("on", nil),
	)
	mockAPI.EXPECT().QuotaOn(ctx, volName).AnyTimes().Return(nil)

	result := driver.enableQuotas(ctx, volName, true)

	assert.NoError(t, result, "Expected no error when enabling quota, got error")
}

func TestEnableQuotas_WithCorruptStatus(t *testing.T) {
	volName := "testvol"

	// CASE 1: Corrupt status at very first time
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("corrupt", nil)

	result1 := driver.enableQuotas(ctx, volName, true)
	assert.Error(t, result1, "Expected error when quota status is corrupt, got nil")

	// CASE 2: Corrupt status during wait period
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("off", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("corrupt", nil),
	)
	mockAPI.EXPECT().QuotaOn(ctx, volName).AnyTimes().Return(nil)

	result2 := driver.enableQuotas(ctx, volName, true)
	assert.Error(t, result2, "Expected error when quota status is corrupt, got nil")
}

func TestEnableQuotas_WithErrorInApiOperation(t *testing.T) {
	volName := "testvol"

	// CASE 1: Error when getting quota status
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaStatus(ctx, volName).AnyTimes().Return("", mockError)

	result1 := driver.enableQuotas(ctx, volName, true)
	assert.Error(t, result1, "Expected error when api failed to get quota status, got nil")

	// CASE 2: Error when setting quota on
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaStatus(ctx, volName).AnyTimes().Return("off", nil)
	mockAPI.EXPECT().QuotaOn(ctx, volName).AnyTimes().Return(mockError)

	result2 := driver.enableQuotas(ctx, volName, true)
	assert.Error(t, result2, "Expected error when api failed to enable quota, got nil")

	// CASE 3: Error in getting quota status during wait period
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	gomock.InOrder(
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("off", nil),
		mockAPI.EXPECT().QuotaStatus(ctx, gomock.Any()).Return("", mockError),
	)
	mockAPI.EXPECT().QuotaOn(ctx, volName).Return(nil)

	result3 := driver.enableQuotas(ctx, volName, true)
	assert.Error(t, result3, "Expected error when api failed to get quota status, got nil")
}

func TestQueueAllFlexvolsForQuotaResize_Success(t *testing.T) {
	volName1 := "testVol1"
	volName2 := "testVol2"
	volInfo1, _ := MockGetVolumeInfo(ctx, volName1)
	volInfo2, _ := MockGetVolumeInfo(ctx, volName2)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{volInfo1, volInfo2}, nil)

	driver.queueAllFlexvolsForQuotaResize(ctx)

	assert.True(t, driver.quotaResizeMap[volName1],
		fmt.Sprintf("Expected true for %s in driver's quotaResizeMap, got false", volName1))
	assert.True(t, driver.quotaResizeMap[volName2],
		fmt.Sprintf("Expected true for %s in driver's quotaResizeMap, got false", volName2))
}

func TestQueueAllFlexvolsForQuotaResize_WithErrorInApiOperation(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(nil, mockError)

	driver.queueAllFlexvolsForQuotaResize(ctx)

	// No volumes should be added in driver quotaResizeMap
	assert.Equal(t, 0, len(driver.quotaResizeMap))
}

func TestResizeQuotas_Success(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Add flexvols to driver's quotaResizeMap. Add some requiring resize and some do not
	driver.quotaResizeMap["vol1"] = true
	driver.quotaResizeMap["vol2"] = true
	driver.quotaResizeMap["vol3"] = true
	driver.quotaResizeMap["vol4"] = false

	// Ensure for one of the flexvol quota resize fails with notFoundError and one fails due to other error
	mockAPI.EXPECT().QuotaResize(ctx, "vol1").AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaResize(ctx, "vol2").AnyTimes().Return(errors.NotFoundError("mock not found error"))
	mockAPI.EXPECT().QuotaResize(ctx, "vol3").AnyTimes().Return(mockError)

	// Resize quota
	driver.resizeQuotas(ctx)

	// Assert that where resize is successful or the flexvol is not found, the entry is removed from quotaResizeMap
	assert.NotContains(t, driver.quotaResizeMap, "vol1", "Expected successful resizeQuota, but failed")
	assert.NotContains(t, driver.quotaResizeMap, "vol2", "Not found flexvol should be removed from quotaResize map")
	assert.Contains(t, driver.quotaResizeMap, "vol3",
		"Flexvol for which quotaResize failed due should be retained for next try")
	assert.Contains(t, driver.quotaResizeMap, "vol4",
		"Flexvol not requiring quotaResize should be retained for next try")
}

func TestGetTotalHardDiskLimitQuota_Success(t *testing.T) {
	volName := "testVol"

	// Create set of quota entries
	quotaEntries := api.QuotaEntries{
		&api.QuotaEntry{Target: "", DiskLimitBytes: 10000},
		&api.QuotaEntry{Target: "", DiskLimitBytes: 10000},
	}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(quotaEntries, nil)

	resultSize, result := driver.getTotalHardDiskLimitQuota(ctx, volName)

	assert.Equal(t, uint64(20000), resultSize, "TotalHardDiskLimitQuota incorrect")
	assert.NoError(t, result, "Expected no error while calculating TotalHardDiskLimitQuota, but go error")
}

func TestGetTotalHardDiskLimitQuota_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(nil, mockError)

	resultSize, result := driver.getTotalHardDiskLimitQuota(ctx, volName)

	assert.Error(t, result, "Expected error when api fails, got nil")
	assert.Equal(t, uint64(0), resultSize, "Non-zero TotalHardDiskLimitQuota returned in case of error")
}

func TestPruneUnusedFlexvols_WithExpiredFlexvolWithZeroQtree(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	volName := "testVol"
	vol, _ := MockGetVolumeInfo(ctx, volName)
	flexvolPrefix := "test"
	driver.flexvolNamePrefix = flexvolPrefix

	mockAPI.EXPECT().VolumeListByPrefix(ctx, flexvolPrefix).AnyTimes().Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, volName).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, volName, true, true).AnyTimes().Return(nil)

	// Populate driver's emptyFlexvolMap with mock volume and initialEmptyTime as much before now
	driver.emptyFlexvolMap = map[string]time.Time{
		volName:    time.Now().Add(-10 * time.Hour),
		"testVol2": time.Now().Add(-10 * time.Hour),
	}
	// Set emptyFlexvolDeferredDeletePeriod with smaller time interval to ensure it expires
	driver.emptyFlexvolDeferredDeletePeriod = 0

	// Run prune method
	driver.pruneUnusedFlexvols(ctx)

	// Ensure that flexvol is successfully deleted and removed from map
	assert.NotContains(t, driver.emptyFlexvolMap, volName, "Expected successful prune of flexvol, but failed")
}

func TestPruneUnusedFlexvols_WithUnexpiredFlexvolWithZeroQtree(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	volName := "testVol"
	vol, _ := MockGetVolumeInfo(ctx, volName)
	volNotInEmptyFlexvolMapName := "testVol2"
	volNotInEmptyFlexvolMap, _ := MockGetVolumeInfo(ctx, volNotInEmptyFlexvolMapName)
	flexvolPrefix := "test"
	driver.flexvolNamePrefix = flexvolPrefix

	mockAPI.EXPECT().VolumeListByPrefix(ctx, flexvolPrefix).AnyTimes().Return(api.Volumes{vol, volNotInEmptyFlexvolMap},
		nil)
	mockAPI.EXPECT().QtreeCount(ctx, volName).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().QtreeCount(ctx, volNotInEmptyFlexvolMapName).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, volName, true, true).AnyTimes().Return(nil)

	// Populate driver's emptyFlexvolMap with mock volume
	driver.emptyFlexvolMap = map[string]time.Time{volName: time.Now()}
	// Set emptyFlexvolDeferredDeletePeriod with larger time interval to ensure it does not expire
	driver.emptyFlexvolDeferredDeletePeriod = 1 * time.Hour

	// Run prune method
	driver.pruneUnusedFlexvols(ctx)

	// Ensure that flexvol is not deleted and not removed from map
	assert.Contains(t, driver.emptyFlexvolMap, volName, "Expected no prune of flexvol, but pruned")
}

func TestPruneUnusedFlexvols_WithFlexvolWithNonZeroQtree(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	volName := "testVol"
	vol, _ := MockGetVolumeInfo(ctx, volName)
	flexvolPrefix := "test"
	driver.flexvolNamePrefix = flexvolPrefix

	mockAPI.EXPECT().VolumeListByPrefix(ctx, flexvolPrefix).AnyTimes().Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, volName).AnyTimes().Return(5, nil)

	// Populate driver's emptyFlexvolMap with mock volume
	driver.emptyFlexvolMap = map[string]time.Time{volName: time.Now()}

	// Run prune method
	driver.pruneUnusedFlexvols(ctx)

	// Ensure that flexvol is removed from map
	assert.NotContains(t, driver.emptyFlexvolMap, volName, "Expected flexvol to be removed from map, but present")
}

func TestPruneUnusedFlexvols_WithErrorInApiOperation(t *testing.T) {
	volName := "testvol"
	vol, _ := MockGetVolumeInfo(ctx, volName)
	flexvolPrefix := "test"

	// CASE 1: Error when getting volume list by prefix
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = flexvolPrefix
	mockAPI.EXPECT().VolumeListByPrefix(ctx, flexvolPrefix).AnyTimes().Return(api.Volumes{}, mockError)

	// Populate driver's emptyFlexvolMap with mock volume
	driver.emptyFlexvolMap = map[string]time.Time{volName: time.Now()}

	// Run prune method
	driver.pruneUnusedFlexvols(ctx)

	// Ensure that the mock volume in the map is retained
	assert.Contains(t, driver.emptyFlexvolMap, volName, "Expected flexvol to be retained in map, but deleted")

	// CASE 2: Error in getting qtree count: logs the warning and deletes the flexvol from map
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = flexvolPrefix
	mockAPI.EXPECT().VolumeListByPrefix(ctx, flexvolPrefix).AnyTimes().Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, gomock.Any()).AnyTimes().Return(0, mockError)

	driver.emptyFlexvolMap = map[string]time.Time{volName: time.Now()}
	driver.pruneUnusedFlexvols(ctx)

	// Ensure that the mock volume in the map is removed
	assert.NotContains(t, driver.emptyFlexvolMap, volName, "Expected flexvol to be removed in map, but retained")

	// CASE 3: Error in destroying volume: should retain flexvol in map
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = flexvolPrefix
	mockAPI.EXPECT().VolumeListByPrefix(ctx, flexvolPrefix).AnyTimes().Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, volName).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, volName, true, true).AnyTimes().Return(mockError)

	// Populate driver's emptyFlexvolMap with mock volume
	driver.emptyFlexvolMap = map[string]time.Time{volName: time.Now()}

	// Run prune method
	driver.pruneUnusedFlexvols(ctx)

	// Ensure that the mock volume in the map is retained
	assert.Contains(t, driver.emptyFlexvolMap, volName, "Expected flexvol to be retained in map, but deleted")
}

func TestReapDeletedQtrees_Success(t *testing.T) {
	qtrees := api.Qtrees{newMockQtree("qtree1", "testVol")}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), driver.flexvolNamePrefix).AnyTimes().Return(qtrees, nil)
	mockAPI.EXPECT().QtreeDestroyAsync(ctx, gomock.Any(), true).AnyTimes().Return(nil)

	driver.reapDeletedQtrees(ctx)

	// Nothing to assert
}

func TestReapDeletedQtrees_WithErrorInApiOperation(t *testing.T) {
	qtrees := api.Qtrees{newMockQtree("qtree1", "testVol")}

	// Case 1: Error while fetching qtree list
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), driver.flexvolNamePrefix).AnyTimes().Return(nil, mockError)
	driver.reapDeletedQtrees(ctx)

	// Case 2: Error while destroying qtree
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), driver.flexvolNamePrefix).AnyTimes().Return(qtrees, nil)
	mockAPI.EXPECT().QtreeDestroyAsync(ctx, gomock.Any(), true).AnyTimes().Return(mockError)
	driver.reapDeletedQtrees(ctx)

	// Nothing to assert
}

func TestGetInternalVolumeName_WithDifferentInput(t *testing.T) {
	storagePrefix := validStoragePrefix

	// Create set of test cases
	tests := []struct {
		name         string
		inputName    string
		expectedName string
	}{
		{
			name:         "name with disallowed ONTAP characters should be modified",
			inputName:    "foo-foo__foo.foo",
			expectedName: fmt.Sprintf("%s_%s", *storagePrefix, "foo_foo_foo_foo"),
		}, {
			name:         "name with more than 64 characters should be modified",
			inputName:    nameMoreThan64char,
			expectedName: strings.Replace(*storagePrefix, "_", "", -1),
		},
	}

	// Run each test
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			// Create a default mock driver
			_, driver := newMockOntapNasQtreeDriver(t)

			// Set config
			driver.Config.StoragePrefix = storagePrefix
			driver.Config.DriverContext = driverContextCSI
			volConfig := &storage.VolumeConfig{Name: test.inputName}
			pool := storage.NewStoragePool(nil, "dummyPool")

			// Get internal name
			result := driver.GetInternalVolumeName(ctx, volConfig, pool)

			// Check if actual name contains expected name. Cannot assert on equal as new uuid is generated
			// everytime in case the length of name is more than 64 characters, thus use assert on contains
			assert.True(tt, strings.Contains(result, test.expectedName))
		})
	}
}

func TestInitialize_QtreeStoragePoolsNameTemplatesAndLabels(t *testing.T) {
	mockAPI, d := newMockOntapNasQtreeDriver(t)
	flexVolPrefix := fmt.Sprintf("trident_qtree_pool_%s_", *d.Config.StoragePrefix)
	d.flexvolNamePrefix = flexVolPrefix

	volume := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	d.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
			Labels: map[string]string{"lable": `{{.volume.Name}}`},
			OntapStorageDriverConfigDefaults: drivers.OntapStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					NameTemplate: `{{.volume.Name}}`,
				},
			},
		},
	}

	poolAttributes := map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().GetSVMAggregateNames(gomock.Any()).AnyTimes().Return([]string{"aggr1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{"aggr1": "vmdisk"}, nil,
	)

	cases := []struct {
		name                        string
		physicalPoolLabels          map[string]string
		virtualPoolLabels           map[string]string
		physicalNameTemplate        string
		virtualNameTemplate         string
		physicalExpected            string
		virtualExpected             string
		volumeNamePhysicalExpected  string
		volumeNameVirtualExpected   string
		backendName                 string
		physicalErrorMessage        string
		virtualErrorMessage         string
		physicalVolNameErrorMessage string
		virtualVolNameErrorMessage  string
	}{
		{
			"no name templates and labels",
			nil,
			nil,
			"",
			"",
			"",
			"",
			"trident_newVolume",
			"trident_newVolume",
			"nas-eco-backend",
			"Label is not empty",
			"Label is not empty",
			"",
			"",
		}, // no name templates and labels
		{
			"base name templates and label only",
			map[string]string{"base-key": `{{.volume.Name}}_{{.volume.Namespace}}`},
			nil,
			`{{.volume.Name}}_{{.volume.Namespace}}`,
			"",
			`{"provisioning":{"base-key":"newVolume_testNamespace"}}`,
			`{"provisioning":{"base-key":"newVolume_testNamespace"}}`,
			"newVolume_testNamespace",
			"newVolume_testNamespace",
			"nas-eco-backend",
			"Base label is not set correctly",
			"Base label is not set correctly",
			"volume name is not set correctly",
			"volume name is not derived correctly",
		}, // base name templates and label only
		{
			"virtual name templates and label only",
			nil,
			map[string]string{"virtual-key": `{{.volume.Name}}_{{.volume.StorageClass}}`},
			"",
			`{{.volume.Name}}_{{.volume.StorageClass}}`,
			"",
			`{"provisioning":{"virtual-key":"newVolume_testSC"}}`,
			"trident_newVolume",
			"newVolume_testSC",
			"nas-eco-backend",
			"Base label is not empty",
			"Virtual pool label is not set correctly",
			"volume name is not set correctly",
			"volume name is not set correctly",
		}, // virtual name templates and label only
		{
			"base and virtual labels",
			map[string]string{"base-key": `{{.volume.Name}}_{{.volume.Namespace}}`},
			map[string]string{"virtual-key": `{{.volume.Name}}_{{.volume.StorageClass}}`},
			`{{.volume.Name}}_{{.volume.Namespace}}`,
			`{{.volume.Name}}_{{.volume.StorageClass}}`,
			`{"provisioning":{"base-key":"newVolume_testNamespace"}}`,
			`{"provisioning":{"base-key":"newVolume_testNamespace","virtual-key":"newVolume_testSC"}}`,
			"newVolume_testNamespace",
			"newVolume_testSC",
			"nas-eco-backend",
			"Base label is not set correctly",
			"Virtual pool label is not set correctly",
			"volume name is not set correctly",
			"volume name is not set correctly",
		}, // base and virtual labels
		{
			"invalid physicalNameTemplate name",
			map[string]string{"base-key": `{{.volume.Name}}_{{.volume.Namespace}}`},
			map[string]string{"virtual-key": `{{.volume.Name}}_{{.volume.StorageClass}}`},
			`{{.volume.Name}}_{{.volume.InvalidField}}`,
			`{{.volume.Name}}_{{.volume.StorageClass}}`,
			`{"provisioning":{"base-key":"newVolume_testNamespace"}}`,
			`{"provisioning":{"base-key":"newVolume_testNamespace","virtual-key":"newVolume_testSC"}}`,
			"trident_newVolume",
			"newVolume_testSC",
			"nas-eco-backend",
			"Base label is not set correctly",
			"Virtual pool label is not set correctly",
			"volume name is not set correctly",
			"volume name is not set correctly",
		}, // invalid physicalNameTemplate name
	}

	for _, test := range cases {
		t.Run(test.name, func(tt *testing.T) {
			d.Config.Labels = test.physicalPoolLabels
			d.Config.NameTemplate = test.physicalNameTemplate
			d.Config.Storage[0].Labels = test.virtualPoolLabels
			d.Config.Storage[0].NameTemplate = test.virtualNameTemplate
			physicalPools, virtualPools, err := InitializeStoragePoolsCommon(ctx, d, poolAttributes,
				test.backendName)
			assert.NoError(t, err, "Error is not nil")

			physicalPool := physicalPools["aggr1"]
			label, err := physicalPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, test.physicalExpected, label, test.physicalErrorMessage)

			d.CreatePrepare(ctx, &volume, physicalPool)
			assert.Equal(t, volume.InternalName, test.volumeNamePhysicalExpected, test.physicalVolNameErrorMessage)

			virtualPool := virtualPools["nas-eco-backend_pool_0"]
			label, err = virtualPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, test.virtualExpected, label, test.virtualErrorMessage)

			d.CreatePrepare(ctx, &volume, virtualPool)
			assert.Equal(t, volume.InternalName, test.volumeNameVirtualExpected, test.virtualVolNameErrorMessage)
		})
	}
}

func TestGetInternalVolumeName_WithPassthroughStore(t *testing.T) {
	defer ScopedTridentConfigUsingPassthroughStore(true)()
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.StoragePrefix = validStoragePrefix
	volName := "vol"
	volConfig := &storage.VolumeConfig{Name: volName}
	pool := storage.NewStoragePool(nil, "dummyPool")

	result := driver.GetInternalVolumeName(ctx, volConfig, pool)

	assert.Equal(t, *validStoragePrefix+volName, result, "Internal volume name incorrect")
}

func TestGetInternalVolumeName(t *testing.T) {
	ctx := context.Background()
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.StoragePrefix = validStoragePrefix
	// Test-1 UsingPassthroughStore == true
	tridentconfig.UsingPassthroughStore = true
	name := "pvc_123456789"

	expected := "tridentpvc_123456789"
	volConfig := &storage.VolumeConfig{Name: name}
	pool := storage.NewStoragePool(nil, "dummyPool")

	out := driver.GetInternalVolumeName(ctx, volConfig, pool)

	assert.Equal(t, expected, out)

	// Test-2 UsingPassthroughStore == false
	tridentconfig.UsingPassthroughStore = false
	expected = "trident_pvc_123456789"

	out = driver.GetInternalVolumeName(ctx, volConfig, pool)

	assert.Equal(t, expected, out)

	// Test-3 UsingPassthroughStore == false and valid name template
	tridentconfig.UsingPassthroughStore = false
	expected = "pool_dev_test_cluster_1"
	pool = getValidOntapNASPool()
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-4 UsingPassthroughStore == false and invalid name template
	tridentconfig.UsingPassthroughStore = false
	// driver.GetInternalVolumeName returns an internal volume name if nameTemplate generation fails.
	expected = "trident_pvc_123456789"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			// "Namespac" is an invalid field
			NameTemplate: "pool_{{.labels.Cluster}}_{{.volume.Namespac}}_{{.volume.RequestName}}",
		},
	)
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-5 UsingPassthroughStore == false and valid name template with multiple underscore
	tridentconfig.UsingPassthroughStore = false
	expected = "pool"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_________{{.labels.Cluster}}_______{{.volume.Namespace}}______{{.volume." +
				"RequestName}}_____",
		},
	)
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-6 UsingPassthroughStore == false and valid name template with special character
	tridentconfig.UsingPassthroughStore = false
	expected = "pool"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool$%^&*{{.labels.Cluster}}_______{{.volume.Namespace}}______{{.volume." +
				"RequestName}}_____",
		},
	)
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-7 UsingPassthroughStore == false and valid name template with label exist in pool
	tridentconfig.UsingPassthroughStore = false
	// "dev_test_cluster_1" is the pool label set in function getValidOntapNASPool
	expected = "pool_dev_test_cluster_1"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.labels.clusterName}}_{{.volume.Namespace}}_{{.volume.RequestName}}",
		},
	)
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-8 UsingPassthroughStore == false and valid name template with volume namespace
	tridentconfig.UsingPassthroughStore = false
	volConfig.Namespace = "trident"
	expected = "pool_dev_test_cluster_1_trident"
	pool = getValidOntapNASPool()
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-9 UsingPassthroughStore == false and valid name template with volume requestName
	tridentconfig.UsingPassthroughStore = false
	volConfig.Namespace = "trident"
	volConfig.RequestName = "volumeRequestName"
	expected = "pool_dev_test_cluster_1_trident_volumeRequestName"
	pool = getValidOntapNASPool()
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)

	// Test-10 invalid name template
	expected = "trident_pvc_123456789"
	pool = getValidOntapNASPool()
	pool.SetInternalAttributes(
		map[string]string{
			NameTemplate: "pool_{{.labels.clusterName}}_{{.volume.Namespace}}_{{.volume.RequestName",
		},
	)
	out = driver.GetInternalVolumeName(ctx, volConfig, pool)
	assert.Equal(t, expected, out)
}

func TestCreatePrepare_Success(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.StoragePrefix = validStoragePrefix
	volConfig := &storage.VolumeConfig{Name: "testVol"}
	pool := storage.NewStoragePool(nil, "dummyPool")

	// CASE 1: With UsingPassthroughStore, expectedInternalName should be StoragePrefix + VolumeName
	defer ScopedTridentConfigUsingPassthroughStore(true)()
	expectedInternalName1 := "tridenttestVol"
	driver.CreatePrepare(ctx, volConfig, pool)
	assert.Equal(t, expectedInternalName1, volConfig.InternalName, "Incorrect volume InternalName set by CreatePrepare")

	// CASE 2: Without UsingPassthroughStore, expectedInternalName should be StoragePrefix-VolumeName with - replaced by _
	defer ScopedTridentConfigUsingPassthroughStore(false)()
	expectedInternalName2 := "trident_testVol"
	driver.CreatePrepare(ctx, volConfig, pool)
	assert.Equal(t, expectedInternalName2, volConfig.InternalName, "Incorrect volume InternalName set by CreatePrepare")
}

func TestCreatePrepare_NilPool(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	flexVolPrefix := fmt.Sprintf("trident_qtree_pool_%s_", *driver.Config.StoragePrefix)
	driver.flexvolNamePrefix = flexVolPrefix

	volume := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	driver.Config.StoragePrefix = validStoragePrefix
	driver.Config.NameTemplate = `{{.volume.Name}}_{{.volume.Namespace}}`

	poolAttributes := map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(driver.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().GetSVMAggregateNames(gomock.Any()).AnyTimes().Return([]string{"aggr1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{"aggr1": "vmdisk"}, nil,
	)

	var err error
	driver.physicalPools, driver.virtualPools, err = InitializeStoragePoolsCommon(ctx, driver, poolAttributes,
		"testBackend")
	assert.NoError(t, err, "Error is not nil")

	driver.CreatePrepare(ctx, &volume, nil)
	assert.Equal(t, "newVolume_testNamespace", volume.InternalName, "Incorrect volume InternalName set by CreatePrepare")
}

func TestCreatePrepare_NilPool_templateNotContainVolumeName(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	flexVolPrefix := fmt.Sprintf("trident_qtree_pool_%s_", *driver.Config.StoragePrefix)
	driver.flexvolNamePrefix = flexVolPrefix

	volume := storage.VolumeConfig{Name: "pvc-1234567", Namespace: "testNamespace", StorageClass: "testSC"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	driver.Config.StoragePrefix = validStoragePrefix
	driver.Config.NameTemplate = `{{.volume.Namespace}}_{{slice .volume.Name 4 9}}`

	poolAttributes := map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(driver.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().GetSVMAggregateNames(gomock.Any()).AnyTimes().Return([]string{"aggr1"}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{"aggr1": "vmdisk"}, nil,
	)

	var err error
	driver.physicalPools, driver.virtualPools, err = InitializeStoragePoolsCommon(ctx, driver, poolAttributes,
		"testBackend")
	assert.NoError(t, err, "Error is not nil")

	driver.CreatePrepare(ctx, &volume, nil)
	assert.Equal(t, "testNamespace_12345", volume.InternalName, "Incorrect volume InternalName set by CreatePrepare")
}

func ScopedTridentConfigUsingPassthroughStore(newValue bool) func() {
	oldValue := tridentconfig.UsingPassthroughStore
	tridentconfig.UsingPassthroughStore = newValue
	return func() {
		tridentconfig.UsingPassthroughStore = oldValue
	}
}

func TestCreateFollowup_Success_WithNASTypeNone(t *testing.T) {
	svm := "svm1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
		InternalID:   "",
	}

	// Set driver config nas type default and other config values
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SVM = svm
	driver.Config.NASType = ""
	driver.Config.DataLIF = "10.0.0.0"
	driver.Config.NfsMountOptions = "-o nfsvers=3"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)

	// Create followup
	result := driver.CreateFollowup(ctx, volConfig)

	// Assert on no error and suitable values for volconfig
	assert.NoError(t, result, "Expected no error in CreateFollowup, got error")
	assert.Equal(t, fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", svm, volName, volNameInternal), volConfig.InternalID,
		"Incorrect volume InternalID")
	assert.Equal(t, driver.Config.DataLIF, volConfig.AccessInfo.NfsServerIP, "Incorrect NfsServerIP")
	assert.Equal(t, fmt.Sprintf("/%s/%s", volName, volConfig.InternalName), volConfig.AccessInfo.NfsPath,
		"Incorrect NfsPath")
	assert.Equal(t, strings.TrimPrefix(driver.Config.NfsMountOptions, "-o "), volConfig.AccessInfo.MountOptions,
		"Incorrect MountOptions")
}

func TestCreateFollowup_Success_WithNASTypeSMB(t *testing.T) {
	svm := "svm1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}

	// Set driver config nas type as SMB and other config values
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SVM = svm
	driver.Config.NASType = sa.SMB
	driver.Config.DataLIF = "10.0.0.0"
	driver.Config.SMBShare = "vol1"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)

	// Create followup
	result := driver.CreateFollowup(ctx, volConfig)

	// Assert on no error and suitable values for volconfig
	assert.NoError(t, result, "Expected no error in CreateFollowup, got error")
	assert.Equal(t, driver.Config.DataLIF, volConfig.AccessInfo.SMBServer, "Incorrect SMBServer")
	expectedSMBPath := tridentconfig.WindowsPathSeparator + driver.Config.SMBShare +
		tridentconfig.WindowsPathSeparator + volName + tridentconfig.WindowsPathSeparator +
		volNameInternal
	assert.Equal(t, expectedSMBPath, volConfig.AccessInfo.SMBPath, "Incorrect SMBPath")
	assert.Equal(t, sa.SMB, volConfig.FileSystem, "Incorrect FileSystem")
}

func TestCreateFollowup_Success_WithROClone(t *testing.T) {
	svm := "svm1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:                      volName,
		InternalName:              volNameInternal,
		InternalID:                "",
		ReadOnlyClone:             true,
		CloneSourceVolumeInternal: volNameInternal,
	}

	// Set driver config nas type default and other config values
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SVM = svm
	driver.Config.NASType = ""
	driver.Config.DataLIF = "10.0.0.0"
	driver.Config.NfsMountOptions = "-o nfsvers=3"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)

	// Create followup
	result := driver.CreateFollowup(ctx, volConfig)

	// Assert on no error and suitable values for volconfig
	assert.NoError(t, result, "Expected no error in CreateFollowup, got error")
	assert.Equal(t, fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", svm, volName, volNameInternal), volConfig.InternalID,
		"Incorrect volume InternalID")
	assert.Equal(t, driver.Config.DataLIF, volConfig.AccessInfo.NfsServerIP, "Incorrect NfsServerIP")
	assert.Equal(t, fmt.Sprintf("/%s/%s/.snapshot/", volName, volNameInternal), volConfig.AccessInfo.NfsPath,
		"Incorrect NfsPath")
	assert.Equal(t, strings.TrimPrefix(driver.Config.NfsMountOptions, "-o "), volConfig.AccessInfo.MountOptions,
		"Incorrect MountOptions")
}

func TestCreateFollowup_WithInvalidInternalID(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
		InternalID:   "invalid",
	}
	_, driver := newMockOntapNasQtreeDriver(t)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "Expected error when invalid InternalID, got nil")
}

func TestCreateFollowup_WithQtreeDoesNotExist(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}

	// Create mock driver and api
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(false, "", nil)

	// Create followup
	result := driver.CreateFollowup(ctx, volConfig)

	// Expect error
	assert.Error(t, result, "Expected error when qtree does not exists, got nil")
}

func TestCreateFollowup_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}

	// CASE: Error while checking if qtree exists
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(false, "", mockError)

	// Create followup
	result := driver.CreateFollowup(ctx, volConfig)

	// Expect error
	assert.Error(t, result, "Expected error when api failed to check qtree existence, got nil")
}

func TestGetProtocol_Success(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)

	protocol := driver.GetProtocol(ctx)
	assert.Equal(t, protocol, tridentconfig.File, "Incorrect protocol")
}

func TestStoreConfig_Success(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	ontapConfig := newOntapStorageDriverConfig()
	ontapConfig.StorageDriverName = "ontap-nas-economy"
	backendConfig := &storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, backendConfig)

	assert.Equal(t, &driver.Config, backendConfig.OntapConfig, "Incorrect backend config")
}

func TestGetVolumeForImport_Success(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"

	volName := "testVol"
	volNameInternal := *driver.Config.StoragePrefix + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1", "aggr2"}
	quotaEntry := &api.QuotaEntry{
		Target:         "",
		DiskLimitBytes: 1073741824,
	}

	mockAPI.EXPECT().QtreeGetByName(ctx, volName, driver.flexvolNamePrefix).AnyTimes().Return(
		newMockQtree(volNameInternal, volName), nil)
	mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).AnyTimes().Return(quotaEntry, nil)

	volumeExternal, result := driver.GetVolumeForImport(ctx, volName)

	// Expect no error and external volume attributes are set
	assert.NoError(t, result, "Expected no error in getting external volume, got error")
	assert.Equal(t, "Internal", volumeExternal.Config.Name, "Volume name doesn't match")
	assert.Equal(t, volNameInternal, volumeExternal.Config.InternalName, "Volume internal name doesn't match")
	assert.Equal(t, strconv.FormatInt(quotaEntry.DiskLimitBytes, 10), volumeExternal.Config.Size,
		"Volume size incorrect")
	assert.Equal(t, volInfo.SnapshotPolicy, volumeExternal.Config.SnapshotPolicy,
		"Volume snapshot policy doesn't match")
	assert.Equal(t, tridentconfig.ReadWriteMany, volumeExternal.Config.AccessMode, "Volume accessMode not correct")
	assert.Equal(t, volInfo.Aggregates[0], volumeExternal.Pool, "Volume pool not correct")
}

func TestGetVolumeForImport_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)

	// CASE 1: Error in getting qtree by name
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().QtreeGetByName(ctx, volName, driver.flexvolNamePrefix).AnyTimes().Return(nil, mockError)

	volumeExternal1, result1 := driver.GetVolumeForImport(ctx, volName)

	assert.Error(t, result1, "Expected error when api failed to get qtree, got nil")
	assert.Nil(t, volumeExternal1, "Expected nil volume when error occurs, got non-nil")

	// CASE 2: Error in getting volume info
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().QtreeGetByName(ctx, volName, driver.flexvolNamePrefix).AnyTimes().Return(
		newMockQtree(volNameInternal, volName), nil)
	mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, mockError)

	volumeExternal2, result2 := driver.GetVolumeForImport(ctx, volName)

	assert.Error(t, result2, "Expected error when api failed to get volume info, got nil")
	assert.Nil(t, volumeExternal2, "Expected nil volume when error occurs, got non-nil")

	// CASE 3: Error in getting quota entry
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().QtreeGetByName(ctx, volName, driver.flexvolNamePrefix).AnyTimes().Return(
		newMockQtree(volNameInternal, volName), nil)
	mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, mockError)

	volumeExternal3, result3 := driver.GetVolumeForImport(ctx, volName)

	assert.Error(t, result3, "Expected error when api failed to get quota entry, got nil")
	assert.Nil(t, volumeExternal3, "Expected nil volume when error occurs, got non-nil")
}

func TestGetVolumeExternalWrappers_Success(t *testing.T) {
	volName1 := "vol1"
	volInfo1, _ := MockGetVolumeInfo(ctx, volName1)
	volInfo1.Aggregates = []string{"aggr1"}

	volName2 := "vol2"
	volInfo2, _ := MockGetVolumeInfo(ctx, volName2)
	volInfo2.Aggregates = []string{"aggr1"}

	qtrees := api.Qtrees{
		newMockQtree("", ""),                             // Qtree with no name should be excluded
		newMockQtree(deletedQtreeNamePrefix+"qtree", ""), // Qtree which is deleted should be excluded
		newMockQtree("qtree1", volName1),                 // This should be converted into VolumeExternal
		newMockQtree("qtree2", volName2),                 // Qtree for which no quota is found should be excluded
		newMockQtree("qtree3", "vol3"),                   // Qtree attached to different volume should be excluded
	}
	quotaEntries := api.QuotaEntries{&api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{volInfo1, volInfo2}, nil)
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(qtrees, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, gomock.Any()).AnyTimes().Return(quotaEntries, nil)

	// Create a mock channel
	testChan := make(chan *storage.VolumeExternalWrapper, 10)

	// Execute the function
	driver.GetVolumeExternalWrappers(ctx, testChan)

	// Assert that only 1 VolumeExternal exists
	assert.Equal(t, 1, len(testChan), "Expected single entry in channel, got %d", len(testChan))

	// Read from channel
	val, _ := <-testChan

	// Expect no error
	assert.NotNil(t, val.Volume, "Volume External not found in the channel")
	assert.Nil(t, val.Error, "Error occured when getting volume external, expected nil")
}

func TestGetVolumeExternalWrappers_WithZeroFlexvol(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{}, nil)

	testChan := make(chan *storage.VolumeExternalWrapper, 10)
	driver.GetVolumeExternalWrappers(ctx, testChan)

	assert.Equal(t, 0, len(testChan), "Expected no entry in channel, got %d", len(testChan))
}

func TestGetVolumeExternalWrappers_WithZeroQtree(t *testing.T) {
	volInfo, _ := MockGetVolumeInfo(ctx, "testvol")

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{volInfo}, nil)
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(api.Qtrees{}, nil)

	testChan := make(chan *storage.VolumeExternalWrapper, 10)
	driver.GetVolumeExternalWrappers(ctx, testChan)

	assert.Equal(t, 0, len(testChan), "Expected no entry in channel, got %d", len(testChan))
}

func TestGetVolumeExternalWrappers_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"

	// CASE 1: Error in getting volume list
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.flexvolNamePrefix = "test"
	mockAPI.EXPECT().VolumeListByPrefix(ctx, driver.flexvolNamePrefix).AnyTimes().Return(nil, mockError)

	testChan1 := make(chan *storage.VolumeExternalWrapper, 10)
	driver.GetVolumeExternalWrappers(ctx, testChan1)

	val1, _ := <-testChan1
	assert.Error(t, val1.Error, "Expected error when api failed to get volume list, got nil")

	// CASE 2: Error in getting qtree list
	mockAPI, driver = newMockOntapNasQtreeDriver(t)

	driver.flexvolNamePrefix = "test"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{volInfo}, nil)
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil, mockError)

	testChan2 := make(chan *storage.VolumeExternalWrapper, 10)
	driver.GetVolumeExternalWrappers(ctx, testChan2)

	val2, _ := <-testChan2
	assert.Error(t, val2.Error, "Expected error when api failed to get volume list, got nil")

	// CASE 3: Error in getting quota entry
	mockAPI, driver = newMockOntapNasQtreeDriver(t)

	driver.flexvolNamePrefix = "test"
	qtrees := api.Qtrees{newMockQtree("qtree1", volName)}

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).AnyTimes().Return(api.Volumes{volInfo}, nil)
	mockAPI.EXPECT().QtreeListByPrefix(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(qtrees, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, gomock.Any()).AnyTimes().Return(nil, mockError)

	testChan3 := make(chan *storage.VolumeExternalWrapper, 10)
	driver.GetVolumeExternalWrappers(ctx, testChan3)

	val3, _ := <-testChan3
	assert.Error(t, val3.Error, "Expected error when api failed to get quota entry, got nil")
}

func TestConvertDiskLimitToBytes(t *testing.T) {
	var input int64 = 5
	var expected int64 = 5 * 1024

	result := convertDiskLimitToBytes(input)

	assert.Equal(t, expected, result, "Conversion of disk limit to bytes is incorrect")
}

func TestGetUpdateType_Success(t *testing.T) {
	// Create and initialize old driver
	_, oldDriver := newMockOntapNasQtreeDriver(t)
	oldDriver.Config.StoragePrefix = convert.ToPtr("test_")
	oldDriver.Config.Username = "user1"
	oldDriver.Config.Password = "password1"
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	// Create a new driver
	_, newDriver := newMockOntapNasQtreeDriver(t)
	newDriver.Config.StoragePrefix = convert.ToPtr("storage_")
	newDriver.Config.Username = "user2"
	newDriver.Config.Password = "password2"
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.UsernameChange)
	expectedBitmap.Add(storage.PasswordChange)
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "Bitmap mismatch")
}

func TestGetUpdateType_InvalidOldDriver(t *testing.T) {
	var oldDriver storage.Driver = nil
	_, newDriver := newMockOntapNasQtreeDriver(t)
	result := newDriver.GetUpdateType(ctx, oldDriver)
	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	assert.Equal(t, expectedBitmap, result, "Bitmap mismatch")
}

func TestHousekeepingTask_Start(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	task := newMockHousekeepingTask(driver)
	task.Start(ctx)
	defer close(task.Done)
	assert.False(t, task.stopped)
}

func TestHousekeepingTask_Stop(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	task := newMockHousekeepingTask(driver)
	task.Stop(ctx)
	assert.True(t, task.stopped)
}

func TestHousekeepingTask_Run(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	task := newMockHousekeepingTask(driver)
	task.run(ctx, time.Time{})
	assert.False(t, task.stopped)
}

func TestNewPruneTask_WithDefaultValues(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.QtreePruneFlexvolsPeriod = ""
	driver.Config.EmptyFlexvolDeferredDeletePeriod = ""

	var tasks []func(context.Context)
	result := *NewPruneTask(ctx, driver, tasks)
	assert.Equal(t, pruneTask, result.Name)
	assert.Equal(t, HousekeepingStartupDelay, result.InitialDelay)
	assert.Equal(t, len(make(chan struct{})), len(result.Done))
	assert.Equal(t, tasks, result.Tasks)
	assert.Equal(t, driver, result.Driver)

	expectedEmptyFlexvolDeferredDeletePeriod := defaultEmptyFlexvolDeferredDeletePeriod
	assert.Equal(t, expectedEmptyFlexvolDeferredDeletePeriod, driver.emptyFlexvolDeferredDeletePeriod,
		"emptyFlexvolDeferredDeletePeriod does not match")
}

func TestNewPruneTask_WithValidConfigValues(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.QtreePruneFlexvolsPeriod = "100"
	driver.Config.EmptyFlexvolDeferredDeletePeriod = "100"

	expectedSeconds, _ := strconv.ParseInt(driver.Config.EmptyFlexvolDeferredDeletePeriod, 10, 64)
	expectedFlexvolDeferredDeletePeriod := time.Duration(expectedSeconds) * time.Second

	var tasks []func(context.Context)
	result := *NewPruneTask(ctx, driver, tasks)
	assert.Equal(t, pruneTask, result.Name)
	assert.Equal(t, HousekeepingStartupDelay, result.InitialDelay)
	assert.Equal(t, len(make(chan struct{})), len(result.Done))
	assert.Equal(t, tasks, result.Tasks)
	assert.Equal(t, driver, result.Driver)

	assert.Equal(t, expectedFlexvolDeferredDeletePeriod, driver.emptyFlexvolDeferredDeletePeriod,
		"emptyFlexvolDeferredDeletePeriod does not match")
}

func TestNewPruneTask_WithInValidConfigValues(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.QtreePruneFlexvolsPeriod = "invalid"
	driver.Config.EmptyFlexvolDeferredDeletePeriod = "invalid"

	var tasks []func(context.Context)
	result := *NewPruneTask(ctx, driver, tasks)

	// Invalid config values are merely logged. A default Housekeeping Task should be returned
	assert.Equal(t, pruneTask, result.Name)
	assert.Equal(t, HousekeepingStartupDelay, result.InitialDelay)
	assert.Equal(t, len(make(chan struct{})), len(result.Done))
	assert.Equal(t, tasks, result.Tasks)
	assert.Equal(t, driver, result.Driver)

	expectedFlexvolDeferredDeletePeriod := defaultEmptyFlexvolDeferredDeletePeriod
	assert.Equal(t, expectedFlexvolDeferredDeletePeriod, driver.emptyFlexvolDeferredDeletePeriod,
		"emptyFlexvolDeferredDeletePeriod does not match")
}

func TestNewResizeTask_WithDefaultValues(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.QtreeQuotaResizePeriod = ""

	var tasks []func(context.Context)
	result := *NewResizeTask(ctx, driver, tasks)
	assert.Equal(t, resizeTask, result.Name)
	assert.Equal(t, HousekeepingStartupDelay, result.InitialDelay)
	assert.Equal(t, len(make(chan struct{})), len(result.Done))
	assert.Equal(t, tasks, result.Tasks)
	assert.Equal(t, driver, result.Driver)
}

func TestNewResizeTask_WithValidConfigValues(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.QtreeQuotaResizePeriod = "100"

	var tasks []func(context.Context)
	result := *NewResizeTask(ctx, driver, tasks)
	assert.Equal(t, resizeTask, result.Name)
	assert.Equal(t, HousekeepingStartupDelay, result.InitialDelay)
	assert.Equal(t, len(make(chan struct{})), len(result.Done))
	assert.Equal(t, tasks, result.Tasks)
	assert.Equal(t, driver, result.Driver)
}

func TestNewResizeTask_WithInValidConfigValues(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.QtreeQuotaResizePeriod = "invalid"

	var tasks []func(context.Context)
	result := *NewResizeTask(ctx, driver, tasks)

	// Invalid config values are only logged. A default Housekeeping Task is returned
	assert.Equal(t, resizeTask, result.Name)
	assert.Equal(t, HousekeepingStartupDelay, result.InitialDelay)
	assert.Equal(t, len(make(chan struct{})), len(result.Done))
	assert.Equal(t, tasks, result.Tasks)
	assert.Equal(t, driver, result.Driver)
}

func TestResize_WithDifferentSizeRequest(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volInfo.SpaceReserve = "none"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}

	// Create set of test cases
	tests := []struct {
		name              string
		resizeTo          int
		attachExpectToApi func(*mockapi.MockOntapAPI)
		expectError       bool
	}{
		{
			name:     "resize request same as current size",
			resizeTo: 1 * 1024 * 1024 * 1024, // 1GB,
			attachExpectToApi: func(mockAPI *mockapi.MockOntapAPI) {
				quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1 * 1024 * 1024 * 1024}
				mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
				mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal,
					gomock.Any()).AnyTimes().Return(quotaEntry, nil)
			},
			expectError: false,
		}, {
			name:     "resize request more than current size",
			resizeTo: 10 * 1024 * 1024 * 1024, // 10GB,
			attachExpectToApi: func(mockAPI *mockapi.MockOntapAPI) {
				quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1 * 1024 * 1024 * 1024}
				mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
				mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal,
					gomock.Any()).AnyTimes().Return(quotaEntry, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
				mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().QuotaSetEntry(ctx, volNameInternal, volName, gomock.Any(),
					gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(api.QuotaEntries{quotaEntry}, nil)
			},
			expectError: false,
		}, {
			name:     "resize request less than current size",
			resizeTo: 1 * 1024 * 1024 * 1024, // 1GB,
			attachExpectToApi: func(mockAPI *mockapi.MockOntapAPI) {
				quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 10 * 1024 * 1024 * 1024}
				mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
				mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal,
					gomock.Any()).AnyTimes().Return(quotaEntry, nil)
			},
			expectError: true,
		},
	}

	// Run each test
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			// Create mock API and driver and attach expect
			mockAPI, driver := newMockOntapNasQtreeDriver(t)
			test.attachExpectToApi(mockAPI)

			// Resize
			result := driver.Resize(ctx, volConfig, uint64(test.resizeTo))

			// Assert based on expected error
			if test.expectError {
				assert.Error(t, result, "Expected error when %s, but got no error", test.name)
			} else {
				assert.NoError(t, result, "Expected no error when %s, but got error", test.name)
			}
		})
	}
}

func TestResize_WithInvalidInternalID(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
		InternalID:   "invalid",
	}
	resizeToInBytes := 10737418240 // 10g

	_, driver := newMockOntapNasQtreeDriver(t)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result, "Expected error when volume has invalid Internal ID, but got no error")
}

func TestResize_WithQtreeDoesNotExist(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(false, "", nil)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result, "Expected error when qtree does not exist, but got no error")
}

func TestResize_WithInvalidVolumeSizeLimit(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig.LimitVolumeSize = "invalid"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).AnyTimes().Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result, "Expected error when invalid volume size limit, but got no error")
}

func TestResize_OverSizeLimit(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig.LimitVolumeSize = "9GiB"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).AnyTimes().Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result, "Expected error with insufficient volume size limit, but got no error")
}

func TestResize_OverPoolSizeLimit_CheckPassed(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volInfo.Size = "1181116006"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LimitVolumePoolSize = "20GiB"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).AnyTimes().Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, volNameInternal, volName, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(api.QuotaEntries{quotaEntry}, nil)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))

	assert.NoError(t, result, "Expected no error")
}

func TestResize_OverPoolSizeLimit_MultipleQtrees(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volInfo.Size = "10737418240"
	volInfo.SnapshotReserve = 0
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry1 := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}
	quotaEntry2 := &api.QuotaEntry{Target: "/vol/vol1/qtree2", DiskLimitBytes: 3221225472}
	quotaEntries := api.QuotaEntries{quotaEntry1, quotaEntry2}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LimitVolumePoolSize = "11GiB"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).Return(quotaEntry1, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).Return(volInfo, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).Return(volInfo, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).Return(quotaEntries, nil)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))

	assert.Error(t, result, "Expected error")
	ok, _ := errors.HasUnsupportedCapacityRangeError(result)
	assert.True(t, ok)
}

func TestResize_OverPoolSizeLimit_OneQtree(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volInfo.Size = "1181116006"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LimitVolumePoolSize = "9GiB"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).Return(volInfo, nil)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))

	assert.Error(t, result, "Expected error")
	ok, _ := errors.HasUnsupportedCapacityRangeError(result)
	assert.True(t, ok)
}

func TestResize_OverPoolSizeLimitCheck_VolumeInfoFailed(t *testing.T) {
	volName := "vol1"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volInfo.Size = "1181116006"
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.LimitVolumePoolSize = "20GiB"

	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).AnyTimes().Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).AnyTimes().Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).Return(volInfo, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(nil, failed)

	result := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))

	assert.Error(t, result, "Expected error")
}

func TestResize_WithErrorInApiOperation(t *testing.T) {
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	volConfig := &storage.VolumeConfig{
		Name:         volName,
		InternalName: volNameInternal,
	}
	resizeToInBytes := 10737418240 // 10g

	quotaEntry := &api.QuotaEntry{Target: "", DiskLimitBytes: 1073741824}
	quotaEntries := api.QuotaEntries{quotaEntry}

	// CASE 1: Error in checking for qtree existence
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(false, "", mockError)

	result1 := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result1, "Expected error when api failed to check qtree existence, but got no error")

	// CASE 2: Error in getting quota entry
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).Return(nil, mockError)

	result2 := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result2, "Expected error when api failed to get quota entry, but got no error")

	// CASE 3: Error in getting volume info
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).Return(nil, mockError)

	result3 := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result3, "Expected error when api failed to get volume info, but got no error")

	// CASE 4: Error in resizing flexvol
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(quotaEntries, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(mockError)

	result4 := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result4, "Expected error when api failed to resize volume, but got no error")

	// CASE 5: Error in setting Quota Entry
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().QtreeExists(ctx, volNameInternal, gomock.Any()).Return(true, volName, nil)
	mockAPI.EXPECT().QuotaGetEntry(ctx, volName, volNameInternal, gomock.Any()).Return(quotaEntry, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(quotaEntries, nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, volNameInternal, volName, gomock.Any(), gomock.Any()).AnyTimes().
		Return(mockError)

	result5 := driver.Resize(ctx, volConfig, uint64(resizeToInBytes))
	assert.Error(t, result5, "Expected error when api failed to resize volume, but got no error")
}

func TestResizeFlexvol_Success_WithOptimalSize(t *testing.T) {
	volName := "vol1"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}

	resizeToInBytes := uint64(10737418240) // 10g

	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}
	quotaEntries := api.QuotaEntries{quotaEntry}

	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(quotaEntries, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(nil)

	result := driver.resizeFlexvol(ctx, volName, resizeToInBytes)

	assert.NoError(t, result, "Flexvol resize failed")
}

func TestResizeFlexvol_Success_WithoutOptimalSize(t *testing.T) {
	volName := "vol1"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}

	resizeToInBytes := uint64(10737418240) // 10g

	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(nil, mockError)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(nil)

	result := driver.resizeFlexvol(ctx, volName, resizeToInBytes)

	assert.NoError(t, result, "Flexvol resize failed")
}

func TestResizeFlexvol_FallbackPath_VolumeSetSizeSuccess(t *testing.T) {
	volName := "vol1"
	ctx := context.Background()
	resizeToInBytes := uint64(10737418240) // 10g

	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	// Simulate getOptimalSizeForFlexvol returning an error (simulate error in VolumeInfo)
	mockAPI.EXPECT().VolumeInfo(ctx, volName).Return(nil, errors.New("mock error"))
	// Expect VolumeSetSize to be called with "+" and the requested size, and succeed
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, "+"+strconv.FormatUint(resizeToInBytes, 10)).Return(nil).Times(1)

	err := driver.resizeFlexvol(ctx, volName, resizeToInBytes)
	assert.NoError(t, err, "Expected resizeFlexvol to return nil when VolumeSetSize succeeds in fallback path")
}

func TestResizeFlexvol_WithErrorInApiOperation(t *testing.T) {
	volName := "vol1"
	volInfo, _ := MockGetVolumeInfo(ctx, volName)
	volInfo.Aggregates = []string{"aggr1"}
	resizeToInBytes := uint64(10737418240) // 10g
	quotaEntry := &api.QuotaEntry{Target: "/vol/vol1/qtree1", DiskLimitBytes: 1073741824}
	quotaEntries := api.QuotaEntries{quotaEntry}

	// CASE 1: Error in setting volume size when optimal size for flexvol found
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(quotaEntries, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(mockError)

	result1 := driver.resizeFlexvol(ctx, volName, resizeToInBytes)

	assert.Error(t, result1, "Expected error when api failed to set size for flexvol, got nil")

	// CASE 2: Error in setting volume size when optimal size for flexvol not found
	mockAPI, driver = newMockOntapNasQtreeDriver(t)

	mockAPI.EXPECT().VolumeInfo(ctx, volName).AnyTimes().Return(volInfo, mockError)
	mockAPI.EXPECT().QuotaEntryList(ctx, volName).AnyTimes().Return(quotaEntries, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, volName, gomock.Any()).AnyTimes().Return(mockError)

	result2 := driver.resizeFlexvol(ctx, volName, resizeToInBytes)

	assert.Error(t, result2, "Expected error when api failed to set size for flexvol, got nil")
}

func TestReconcileNodeAccess(t *testing.T) {
	backendPolicy := getExportPolicyName(BackendUUID)

	// CASE 1: No qtrees mounted on any node, no existing rule(s) in backend-policy
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}
	volConfigs := make([]*storage.VolumeConfig, 0)
	volToNodePublications := make(map[string][]*models.VolumePublication)
	nodes := make([]*models.Node, 0)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).AnyTimes().Return(nil, nil)

	err := driver.ReconcileNodeAccess(ctx, nodes, BackendUUID, "")
	assert.NoError(t, err, "Reconcile node access failed")

	// CASE 2: No qtrees mounted on any node, 1 stale rule in backend-policy
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}
	volConfigs = make([]*storage.VolumeConfig, 0)
	volToNodePublications = make(map[string][]*models.VolumePublication)
	nodes = make([]*models.Node, 0)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).AnyTimes().Return(map[int]string{1: "1.1.1.1"}, nil)
	mockAPI.EXPECT().ExportRuleDestroy(ctx, gomock.Any(), 1).Return(nil)

	err = driver.ReconcileNodeAccess(ctx, nodes, BackendUUID, "")
	assert.NoError(t, err, "Reconcile node access failed")

	// CASE 4: 1 qtree mounted on 1 node, no IP change in backend policy
	nodes = []*models.Node{
		{
			Name: "node-1",
			IPs:  []string{"1.1.1.1"},
		},
	}
	volConfigs = make([]*storage.VolumeConfig, 0)
	volToNodePublications = make(map[string][]*models.VolumePublication)
	volConfig := &storage.VolumeConfig{
		Name:         "vol-1",
		InternalName: "qtree-1",
		InternalID:   "/svm/svm-1/flexvol/flex-vol-1/qtree/qtree-1",
		ExportPolicy: backendPolicy,
	}
	volToNodePublications["vol-1"] = []*models.VolumePublication{
		{
			Name:       "vp-1",
			NodeName:   "node-1",
			VolumeName: "vol-1",
			ReadOnly:   false,
			AccessMode: 0,
		},
	}
	volConfigs = append(volConfigs, volConfig)
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}
	mockAPI.EXPECT().ExportPolicyCreate(ctx, backendPolicy).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).AnyTimes().Return(map[int]string{1: "1.1.1.1"}, nil)

	err = driver.ReconcileNodeAccess(ctx, nodes, BackendUUID, "")
	assert.NoError(t, err, "Reconcile node access failed")

	// CASE 5: 1 qtree mounted on 1 node, CIDR change affecting IP in backend policy
	nodes = []*models.Node{
		{
			Name: "node-1",
			IPs:  []string{"1.1.1.1"},
		},
	}
	volConfigs = make([]*storage.VolumeConfig, 0)
	volToNodePublications = make(map[string][]*models.VolumePublication)
	volConfig = &storage.VolumeConfig{
		Name:         "vol-1",
		InternalName: "qtree-1",
		InternalID:   "/svm/svm-1/flexvol/flex-vol-1/qtree/qtree-1",
		ExportPolicy: backendPolicy,
	}
	volToNodePublications["vol-1"] = []*models.VolumePublication{
		{
			Name:       "vp-1",
			NodeName:   "node-1",
			VolumeName: "vol-1",
			ReadOnly:   false,
			AccessMode: 0,
		},
	}
	volConfigs = append(volConfigs, volConfig)
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"2.2.2.2/32"}
	mockAPI.EXPECT().ExportPolicyCreate(ctx, backendPolicy).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, gomock.Any()).AnyTimes().Return(map[int]string{1: "1.1.1.1"}, nil)
	mockAPI.EXPECT().ExportRuleDestroy(ctx, backendPolicy, 1)

	err = driver.ReconcileNodeAccess(ctx, nodes, BackendUUID, "")
	assert.NoError(t, err, "Reconcile node access failed")

	// CASE 6: 1 qtree mounted on 1 node, IP missing in backend policy
	nodes = []*models.Node{
		{
			Name: "node-1",
			IPs:  []string{"1.1.1.1"},
		},
	}
	volConfigs = make([]*storage.VolumeConfig, 0)
	volToNodePublications = make(map[string][]*models.VolumePublication)
	volConfig = &storage.VolumeConfig{
		Name:         "vol-1",
		InternalName: "qtree-1",
		InternalID:   "/svm/svm-1/flexvol/flex-vol-1/qtree/qtree-1",
		ExportPolicy: "qtree-1",
	}
	volToNodePublications["vol-1"] = []*models.VolumePublication{
		{
			Name:       "vp-1",
			NodeName:   "node-1",
			VolumeName: "vol-1",
			ReadOnly:   false,
			AccessMode: 0,
		},
	}
	volConfigs = append(volConfigs, volConfig)
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"0.0.0.0/0"}
	mockAPI.EXPECT().ExportPolicyCreate(ctx, backendPolicy).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, backendPolicy).AnyTimes().Return(nil, nil)
	mockAPI.EXPECT().ExportRuleCreate(ctx, backendPolicy, "1.1.1.1", gomock.Any()).AnyTimes().Return(nil)

	err = driver.ReconcileNodeAccess(ctx, nodes, BackendUUID, "")
	assert.NoError(t, err, "Reconcile node access failed")

	// CASE 7: 1 qtree mounted on 1 node, CIDR change affecting IP in backend policy
	nodes = []*models.Node{
		{
			Name: "node-1",
			IPs:  []string{"1.1.1.1"},
		},
	}
	volConfigs = make([]*storage.VolumeConfig, 0)
	volToNodePublications = make(map[string][]*models.VolumePublication)
	volConfig = &storage.VolumeConfig{
		Name:         "vol-1",
		InternalName: "qtree-1",
		InternalID:   "/svm/svm-1/flexvol/flex-vol-1/qtree/qtree-1",
		ExportPolicy: "qtree-1",
	}
	volToNodePublications["vol-1"] = []*models.VolumePublication{
		{
			Name:       "vp-1",
			NodeName:   "node-1",
			VolumeName: "vol-1",
			ReadOnly:   false,
			AccessMode: 0,
		},
	}
	volConfigs = append(volConfigs, volConfig)
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"2.2.2.2/32"}
	mockAPI.EXPECT().ExportPolicyCreate(ctx, backendPolicy).AnyTimes().Return(nil)
	mockAPI.EXPECT().ExportRuleList(ctx, backendPolicy).AnyTimes().Return(map[int]string{1: "1.1.1.1"}, nil)
	mockAPI.EXPECT().ExportRuleDestroy(ctx, backendPolicy, 1)

	err = driver.ReconcileNodeAccess(ctx, nodes, BackendUUID, "")
	assert.NoError(t, err, "Reconcile node access failed")
}

func TestEnsureSMBShare_Success_WithSMBShareInConfig(t *testing.T) {
	volName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm2:vol1",
		SecureSMBEnabled: false,
	}
	driver.Config.SMBShare = volName

	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(nil)

	result := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)

	assert.NoError(t, result, "SMB Create failed")
}

func TestEnsureSMBShare_Success_WithoutSMBShareInConfig(t *testing.T) {
	volName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm2:vol1",
		SecureSMBEnabled: false,
	}

	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(nil)

	result := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)

	assert.NoError(t, result, "SMB creation failed")
}

func TestEnsureSMBShare_Success_SecureSMBEnabled(t *testing.T) {
	volName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm2:vol1",
		SMBShareACL:      map[string]string{"user": "full_control"},
		SecureSMBEnabled: true,
	}
	driver.Config.SMBShare = volName

	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().SMBShareAccessControlDelete(ctx, "vol1", smbShareDeleteACL).Return(nil)
	mockAPI.EXPECT().SMBShareAccessControlCreate(ctx, "vol1", volConfig.SMBShareACL).Return(nil)

	result := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)

	assert.NoError(t, result, "SMB Create failed")
}

func TestEnsureSMBShare_WithErrorInApiOperation(t *testing.T) {
	volName := "vol1"

	// CASE 1: Error in checking for SMB share when share name present in config
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = volName
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm2:vol1",
		SecureSMBEnabled: false,
	}

	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, mockError)

	result1 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result1, "Expected SMB creation to fail when api failed to check for SMB share, but got no error")

	// CASE 2: Error in creatingSMB share when share name present in config
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = volName
	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(mockError)

	result2 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result2, "Expected SMB creation to fail when api failed to check for SMB share, but got no error")

	// CASE 3: Error in checking for SMB share when share name not present in config
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, mockError)

	result3 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result3, "Expected SMB creation to fail when api failed to check for SMB share, but got no error")

	// CASE 4: Error in creatingSMB share when share name not present in config
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(mockError)

	result4 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result4, "Expected SMB creation to fail when api failed to check for SMB share, but got no error")
}

func TestEnsureSMBShare_WithErrorInApiOperationSecureSMBEnabled(t *testing.T) {
	volName := "vol1"

	// CASE 1: Error in checking for SMB share
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm2:vol1",
		SMBShareACL:      map[string]string{"us": "full_control"},
		SecureSMBEnabled: true,
	}

	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, mockError)

	result1 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result1, "Expected SMB creation to fail when api failed to check for SMB share, but got no error")

	// CASE 2: Error in creatingSMB share
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(mockError)

	result2 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result2, "Expected SMB creation to fail when api failed to check for SMB share, but got no error")

	// CASE 3: Error in deleting SMB Share Access Control
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().SMBShareAccessControlDelete(ctx, "vol1", smbShareDeleteACL).AnyTimes().Return(mockError)

	result3 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result3, "Expected SMB Share Access Control Deletion to fail when api failed to delete for SMB"+
		" Share Access Control, but got no error")

	// CASE 4: Error in creating SMB Share Access Control
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	mockAPI.EXPECT().SMBShareExists(ctx, volName).AnyTimes().Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volName, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().SMBShareAccessControlDelete(ctx, "vol1", smbShareDeleteACL).Return(nil)
	mockAPI.EXPECT().SMBShareAccessControlCreate(ctx, "vol1", volConfig.SMBShareACL).AnyTimes().Return(mockError)

	result4 := driver.EnsureSMBShare(ctx, volName, "/"+volName, volConfig.SMBShareACL, volConfig.SecureSMBEnabled)
	assert.Error(t, result4, "Expected SMB Share Access Control Creation to fail when api failed to create "+
		" for SMB share Access Control, but got no error")
}

func TestDestroySMBShare_Success_ShareNotInConfig(t *testing.T) {
	shareName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = "otherShare"

	// SMB share exists and is destroyed successfully
	mockAPI.EXPECT().SMBShareExists(ctx, shareName).Return(true, nil)
	mockAPI.EXPECT().SMBShareDestroy(ctx, shareName).Return(nil)

	err := driver.DestroySMBShare(ctx, shareName)
	assert.NoError(t, err, "Expected no error when destroying SMB share")
}

func TestDestroySMBShare_Success_ShareDoesNotExist(t *testing.T) {
	shareName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = "otherShare"

	// SMB share does not exist
	mockAPI.EXPECT().SMBShareExists(ctx, shareName).Return(false, nil)

	err := driver.DestroySMBShare(ctx, shareName)
	assert.NoError(t, err, "Expected no error when SMB share does not exist")
}

func TestDestroySMBShare_SkipIfShareInConfig(t *testing.T) {
	shareName := "vol1"
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = shareName

	// Should skip deletion if share matches config
	err := driver.DestroySMBShare(ctx, shareName)
	assert.NoError(t, err, "Expected no error when SMB share matches config")
}

func TestDestroySMBShare_ErrorOnExistsCheck(t *testing.T) {
	shareName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = "otherShare"

	mockAPI.EXPECT().SMBShareExists(ctx, shareName).Return(false, mockError)

	err := driver.DestroySMBShare(ctx, shareName)
	assert.Error(t, err, "Expected error when SMBShareExists fails")
}

func TestDestroySMBShare_ErrorOnDestroy(t *testing.T) {
	shareName := "vol1"
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.SMBShare = "otherShare"

	mockAPI.EXPECT().SMBShareExists(ctx, shareName).Return(true, nil)
	mockAPI.EXPECT().SMBShareDestroy(ctx, shareName).Return(mockError)

	err := driver.DestroySMBShare(ctx, shareName)
	assert.Error(t, err, "Expected error when SMBShareDestroy fails")
}

func TestOntapNasQtreeStorageDriverConfigString(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	qtreeDrivers := []NASQtreeStorageDriver{
		*newNASQtreeStorageDriver(mockAPI),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-nas-qtree-user",
		"password":        "password1!",
		"client username": "client_username",
		"client password": "client_password",
	}

	externalIncludeList := map[string]string{
		"<REDACTED>":                   "<REDACTED>",
		"username":                     "Username:<REDACTED>",
		"password":                     "Password:<REDACTED>",
		"api":                          "API:<REDACTED>",
		"chap username":                "ChapUsername:<REDACTED>",
		"chap initiator secret":        "ChapInitiatorSecret:<REDACTED>",
		"chap target username":         "ChapTargetUsername:<REDACTED>",
		"chap target initiator secret": "ChapTargetInitiatorSecret:<REDACTED>",
		"client private key":           "ClientPrivateKey:<REDACTED>",
	}

	for _, qtreeDriver := range qtreeDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, qtreeDriver.String(), val, "ontap-nas-economy driver does not contain %v", key)
			assert.Contains(t, qtreeDriver.GoString(), val, "ontap-nas-economy driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, qtreeDriver.String(), val, "ontap-nas-economy driver contains %v", key)
			assert.NotContains(t, qtreeDriver.GoString(), val, "ontap-nas-economy driver contains %v", key)
		}
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_NoRulesSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"
	rules := []string{"0.0.0.0/0", "::/0"}

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return an empty set of rules when asked for them
	ruleListCall := mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(make(map[int]string), nil)
	// Ensure that the default rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[0],
		gomock.Any()).After(ruleListCall).Return(nil)
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[1],
		gomock.Any()).After(ruleListCall).Return(nil)

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(ctx); err != nil {
		t.Error(err)
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_RulesExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"
	fakeRules := map[int]string{
		0: "foo",
		1: "bar",
		2: "baz",
	}

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return the fake rules when asked
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(fakeRules, nil)

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(ctx); err != nil {
		t.Error(err)
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_ErrorGettingRules(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return an error when asked for export rules
	mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(nil, errors.New("foobar"))

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(ctx); err == nil {
		t.Error("Error was not propagated")
	}
}

func TestNASQtreeStorageDriver_ensureDefaultExportPolicyRule_ErrorCreatingRules(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakeExportPolicy := "foobar"
	rules := []string{"0.0.0.0/0", "::/0"}

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	// Return an empty set of rules when asked for them
	ruleListCall := mockAPI.EXPECT().ExportRuleList(gomock.Any(), fakeExportPolicy).Return(make(map[int]string), nil)
	// Ensure that the default rules are created after getting an empty list of rules
	mockAPI.EXPECT().ExportRuleCreate(gomock.Any(), gomock.Any(), rules[0], gomock.Any()).After(ruleListCall).Return(
		errors.New("foobar"),
	)

	qtreeDriver := newNASQtreeStorageDriver(mockAPI)
	qtreeDriver.flexvolExportPolicy = fakeExportPolicy

	if err := qtreeDriver.ensureDefaultExportPolicyRule(ctx); err == nil {
		t.Error("Error was not propagated")
	}
}

func TestGetStorageBackendSpecs_Success(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	backend := storage.StorageBackend{}

	result := driver.GetStorageBackendSpecs(ctx, &backend)

	assert.NoError(t, result, "Expected no error, got error")
}

func TestOntapNasQtreeStorageDriverGetStorageBackendPools(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	svmUUID := "SVM1-uuid"
	flexVolPrefix := fmt.Sprintf("trident_qtree_pool_%s_", *driver.Config.StoragePrefix)
	driver.flexvolNamePrefix = flexVolPrefix
	driver.physicalPools = map[string]storage.Pool{
		"pool1": storage.NewStoragePool(nil, "pool1"),
		"pool2": storage.NewStoragePool(nil, "pool2"),
	}
	mockAPI.EXPECT().GetSVMUUID().Return(svmUUID)

	pools := driver.getStorageBackendPools(ctx)

	assert.NotEmpty(t, pools)
	assert.Equal(t, len(driver.physicalPools), len(pools))

	pool := pools[0]
	assert.NotNil(t, driver.physicalPools[pool.Aggregate])
	assert.Equal(t, driver.physicalPools[pool.Aggregate].Name(), pool.Aggregate)
	assert.Equal(t, svmUUID, pool.SvmUUID)
	assert.Equal(t, flexVolPrefix, pool.FlexVolPrefix)

	pool = pools[1]
	assert.NotNil(t, driver.physicalPools[pool.Aggregate])
	assert.Equal(t, driver.physicalPools[pool.Aggregate].Name(), pool.Aggregate)
	assert.Equal(t, svmUUID, pool.SvmUUID)
	assert.Equal(t, flexVolPrefix, pool.FlexVolPrefix)
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_1Gi(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	quotaEntry := &api.QuotaEntry{
		Target:         "",
		DiskLimitBytes: 1024 ^ 3,
	}
	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := uint64(quotaEntry.DiskLimitBytes)

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(quotaEntry, nil)
	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(ctx, qtreeName, flexvolName)

	assert.Nil(t, err, fmt.Sprintf("Unexpected err, %v", err))
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_1Ki(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	quotaEntry := &api.QuotaEntry{
		Target:         "",
		DiskLimitBytes: 1024,
	}
	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := uint64(quotaEntry.DiskLimitBytes)

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(quotaEntry, nil)
	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(ctx, qtreeName, flexvolName)

	assert.Nil(t, err, fmt.Sprintf("Unexpected err, %v", err))
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_NoLimit(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	quotaEntry := &api.QuotaEntry{
		Target:         "",
		DiskLimitBytes: -1,
	}
	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := uint64(0)

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(quotaEntry, nil)
	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(ctx, qtreeName, flexvolName)

	assert.Nil(t, err, fmt.Sprintf("Unexpected err, %v", err))
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestNASQtreeStorageDriver_getQuotaDiskLimitSize_Error(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	qtreeName := "foo"
	flexvolName := "bar"
	expectedLimit := uint64(0)

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().QuotaGetEntry(gomock.Any(), flexvolName, qtreeName, "tree").Return(nil, errors.New("error"))
	driver := newNASQtreeStorageDriver(mockAPI)

	limit, err := driver.getQuotaDiskLimitSize(ctx, qtreeName, flexvolName)

	assert.NotNil(t, err, "Unexpected success")
	assert.Equal(t, expectedLimit, limit, "Unexpected return value")
}

func TestCreateQtreeInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	svm := "fakeSVM"
	flexvol := "fakeFlexVol"
	qtree := "fakeQtree"
	testString := fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", svm, flexvol, qtree)
	str := driver.CreateQtreeInternalID(svm, flexvol, qtree)
	assert.Equal(t, testString, str)
}

func TestParseInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	testString := "/svm/fakeSVM/flexvol/fakeFlexvol/qtree/fakeQtree"
	svm, flexvol, qtree, err := driver.ParseQtreeInternalID(testString)
	assert.NoError(t, err, "unexpected error found while parsing InternalId")
	assert.Equal(t, svm, "fakeSVM")
	assert.Equal(t, flexvol, "fakeFlexvol")
	assert.Equal(t, qtree, "fakeQtree")
}

func TestParseInternalIdWithMissingSVM(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	testString := "/flexvol/fakeFlexvol/qtree/fakeQtree"
	_, _, _, err := driver.ParseQtreeInternalID(testString)
	assert.Error(t, err, "expected an error when SVM Name is missing")
}

func TestSetVolumePatternWithInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := "/svm/fakeSVM/flexvol/fakeFlexvol/qtree/fakeQtree"
	internalName := "fakeQtree"
	volumePrefix := "fakePrefix"
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(ctx, internalID, internalName,
		volumePrefix)
	assert.NoError(t, err, "unexpected error found while setting setting volume pattern")
	assert.Equal(t, volumePattern, "fakeFlexvol")
	assert.Equal(t, name, "fakeQtree")
}

func TestSetVolumePatternWithMisformedInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := "/flexvol/fakeFlexvol/qtree/fakeQtree"
	internalName := "fakeQtree"
	volumePrefix := "fakePrefix"
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(ctx, internalID, internalName,
		volumePrefix)
	assert.Error(t, err, "expected an error when InternalID is misformed")
	assert.Equal(t, volumePattern, "")
	assert.Equal(t, name, "")
}

func TestSetVolumePatternWithoutInternalID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := ""
	internalName := "fakeQtree"
	volumePrefix := "fakePrefix"
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(ctx, internalID, internalName,
		volumePrefix)
	assert.NoError(t, err, "unexpected error found while setting setting volume pattern")
	assert.Equal(t, volumePattern, "fakePrefix*")
	assert.Equal(t, name, "fakeQtree")
}

func TestSetVolumePatternWithNoInternalIDAndNoPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	driver := newNASQtreeStorageDriver(mockAPI)
	internalID := ""
	internalName := "fakeQtree"
	volumePrefix := ""
	volumePattern, name, err := driver.SetVolumePatternToFindQtree(ctx, internalID, internalName,
		volumePrefix)
	assert.NoError(t, err, "unexpected error found while setting setting volume pattern")
	assert.Equal(t, volumePattern, "*")
	assert.Equal(t, name, "fakeQtree")
}

func TestNASQtreeStorageDriver_VolumeCreate(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	qtreeName := "qtree1"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         qtreeName,
		InternalName: qtreeName,
	}
	flexvolName := "flexvol1"
	flexvol := &api.Volume{Name: flexvolName}
	sizeBytes := 1073741824
	sizeBytesStr := strconv.FormatUint(uint64(sizeBytes), 10)
	sizeKB := 1048576
	sizeKBStr := strconv.FormatUint(uint64(sizeKB), 10)

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		SnapshotDir:       "true",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	driver.Config.NASType = sa.SMB
	driver.Config.LimitVolumeSize = "2g"
	volAttrs := map[string]sa.Request{}

	emptyPolicy := fmt.Sprintf("%sempty", *validStoragePrefix)

	addCommonExpectForQtreeCreate(mockAPI, flexvol, flexvolName, sizeBytesStr)
	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, "*").Return(false, "", nil)
	mockAPI.EXPECT().QtreeCreate(ctx, qtreeName, flexvolName, "0755", emptyPolicy,
		"mixed", "fake-qos-policy").Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, qtreeName, flexvolName, "tree", sizeKBStr).Return(nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, flexvolName, sizeBytesStr).AnyTimes().Return(nil)
	mockAPI.EXPECT().SMBShareExists(ctx, flexvolName).AnyTimes().Return(true, nil)
	mockAPI.EXPECT().ExportPolicyCreate(ctx, emptyPolicy).Times(1).Return(nil)

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
	assert.Equal(t, "/svm/SVM1/flexvol/flexvol1/qtree/qtree1", volConfig.InternalID)
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, emptyPolicy, volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
}

func addCommonExpectForQtreeCreate(
	mockAPI *mockapi.MockOntapAPI, flexvol *api.Volume,
	flexvolName, sizeBytesStr string,
) {
	mockAPI.EXPECT().TieringPolicyValue(ctx).AnyTimes().Return("snapshot-only")
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return([]*api.Volume{flexvol}, nil)
	mockAPI.EXPECT().QtreeCount(ctx, flexvolName).AnyTimes().Return(0, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, flexvolName).AnyTimes().Return(flexvol, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, flexvolName, sizeBytesStr).AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaEntryList(ctx, flexvolName).AnyTimes().Return(api.QuotaEntries{}, nil)
}

func TestCreate_WithInvalidInternalID(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         "qtree1",
		InternalName: "qtree1",
		InternalID:   "invalid",
	}

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	result := driver.Create(ctx, volConfig, pool1, volAttrs)
	assert.Error(t, result, "Expected error in volume create when qtree exists, got nil")
}

func TestCreate_WithVirtualPoolAndNoMatchingPhysicalPool(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	qtreeName := "qtree1"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         qtreeName,
		InternalName: qtreeName,
	}

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool2 := storage.NewStoragePool(sb, "pool2")
	driver.virtualPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, "*").Return(false, "", nil)

	result := driver.Create(ctx, volConfig, pool2, volAttrs)
	assert.Error(t, result, "Expected error when virtual pool with no matching physical pool is requested, got nil")
}

func TestCreate_WithQtreeAlreadyExists(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         "qtree1",
		InternalName: "qtree1",
	}

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().QtreeExists(ctx, "qtree1", "*").Return(true, "", nil)
	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result, "Expected error in volume create when qtree exists, got nil")
}

func TestCreate_WithInvalidConfig(t *testing.T) {
	// Create attributes common to all test cases
	volName := "vol1"
	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	commonStoragePool := storage.NewStoragePool(sb, "pool1")
	volAttrs := map[string]sa.Request{}

	// Create set of test cases
	tests := []struct {
		name        string
		volConfig   *storage.VolumeConfig
		storagePool func() *storage.StoragePool
	}{
		{
			name: "with invalid volume size",
			volConfig: &storage.VolumeConfig{
				Size:         "invalid",
				FileSystem:   "nfs",
				Name:         volName,
				InternalName: volName,
			},
		}, {
			name: "with volume size less than MinimumVolumeSizeBytes",
			volConfig: &storage.VolumeConfig{
				Size:         "10M",
				FileSystem:   "nfs",
				Name:         volName,
				InternalName: volName,
			},
		}, {
			name: "with name length more than maxQtreeNameLength",
			volConfig: &storage.VolumeConfig{
				Size:         "1g",
				FileSystem:   "nfs",
				Name:         nameMoreThan64char,
				InternalName: nameMoreThan64char,
			},
		}, {
			name: "with invalid snapshot directory",
			volConfig: &storage.VolumeConfig{
				Size:         "1g",
				FileSystem:   "nfs",
				Name:         volName,
				InternalName: volName,
			},
			storagePool: func() *storage.StoragePool {
				sb := storage.NewTestStorageBackend()
				sb.SetBackendUUID(BackendUUID)
				pool := storage.NewStoragePool(sb, "pool1")
				pool.InternalAttributes()["snapshotDir"] = "invalid"
				return pool
			},
		}, {
			name: "with invalid encryption",
			volConfig: &storage.VolumeConfig{
				Size:         "1g",
				FileSystem:   "nfs",
				Name:         volName,
				InternalName: volName,
			},
			storagePool: func() *storage.StoragePool {
				sb := storage.NewTestStorageBackend()
				sb.SetBackendUUID(BackendUUID)
				pool := storage.NewStoragePool(sb, "pool1")
				pool.InternalAttributes()["snapshotDir"] = "true"
				pool.InternalAttributes()["encryption"] = "123"
				return pool
			},
		},
	}

	// Run each test
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockAPI, driver := newMockOntapNasQtreeDriver(t)
			// Set common config of driver
			driver.physicalPools = map[string]storage.Pool{"pool1": commonStoragePool}
			driver.Config.AutoExportPolicy = true

			// Add expect to API
			mockAPI.EXPECT().QtreeExists(ctx, test.volConfig.Name, "*").Return(false, "", nil)

			// Add storage pool based on test case
			storagePool := commonStoragePool
			if test.storagePool != nil {
				storagePool = test.storagePool()
			}

			// Call create
			result := driver.Create(ctx, test.volConfig, storagePool, volAttrs)

			assert.Error(t, result, "Expected error in volume create %s, got nil", test.name)
		})
	}
}

func TestCreate_OverSizeLimit(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	qtreeName := "qtree1"
	volConfig := &storage.VolumeConfig{
		Size:         "200g",
		FileSystem:   "nfs",
		Name:         qtreeName,
		InternalName: qtreeName,
	}

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		SnapshotDir:       "true",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	driver.Config.NASType = sa.SMB
	driver.Config.LimitVolumeSize = "100g"
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, "*").Return(false, "", nil)

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestCreate_OverPoolSizeLimit(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	qtreeName := "qtree1"
	volConfig := &storage.VolumeConfig{
		Size:         "5g",
		FileSystem:   "nfs",
		Name:         qtreeName,
		InternalName: qtreeName,
	}
	flexvolName := "flexvol1"
	flexvol := &api.Volume{Name: flexvolName}
	sizeBytes := 1073741824
	sizeBytesStr := strconv.FormatUint(uint64(sizeBytes), 10)

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		SnapshotDir:       "true",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	driver.Config.NASType = sa.SMB
	driver.Config.LimitVolumePoolSize = "2g"
	volAttrs := map[string]sa.Request{}

	emptyPolicy := fmt.Sprintf("%sempty", *validStoragePrefix)

	addCommonExpectForQtreeCreate(mockAPI, flexvol, flexvolName, sizeBytesStr)

	mockAPI.EXPECT().ExportPolicyCreate(ctx, emptyPolicy).Times(1).Return(nil)
	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, "*").Return(false, "", nil)

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result, "Expected error in volume create when ineligible backend, got nil")
}

func TestCreate_WithIneligibleBackend(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         "qtree1",
		InternalName: "qtree1",
	}
	sizeBytes := 1073741824
	sizeBytesStr := "+" + strconv.FormatUint(uint64(sizeBytes), 10)
	flexvolName := "flexvol1"
	flexvol := &api.Volume{Name: flexvolName}
	volAttrs := map[string]sa.Request{}
	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool1.InternalAttributes()["snapshotDir"] = "false"
	emptyPolicy := fmt.Sprintf("%sempty", *validStoragePrefix)

	// CASE 1: Physical pool for which error in getting aggregate space
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true
	driver.Config.LimitAggregateUsage = "50%"

	mockAPI.EXPECT().ExportPolicyCreate(ctx, emptyPolicy).Times(1).Return(nil)
	mockAPI.EXPECT().QtreeExists(ctx, "qtree1", "*").Return(false, "", nil)
	mockAPI.EXPECT().TieringPolicyValue(ctx).AnyTimes().Return("snapshot-only")
	mockAPI.EXPECT().GetSVMAggregateSpace(ctx, "pool1").Return(nil, mockError)

	result1 := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result1, "Expected error in volume create when ineligible backend, got nil")

	// CASE 2: Physical pool for which error in getting volume list
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true

	mockAPI.EXPECT().ExportPolicyCreate(ctx, emptyPolicy).Times(1).Return(nil)
	mockAPI.EXPECT().QtreeExists(ctx, "qtree1", "*").Return(false, "", nil)
	mockAPI.EXPECT().TieringPolicyValue(ctx).AnyTimes().Return("snapshot-only")
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return(nil, mockError)

	result2 := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result2, "Expected error in volume create when ineligible backend, got nil")

	// CASE 3: Physical pool for which error in setting volume size
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.AutoExportPolicy = true

	mockAPI.EXPECT().ExportPolicyCreate(ctx, emptyPolicy).Times(1).Return(nil)
	mockAPI.EXPECT().QtreeExists(ctx, "qtree1", "*").Return(false, "", nil)
	mockAPI.EXPECT().TieringPolicyValue(ctx).AnyTimes().Return("snapshot-only")
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).AnyTimes().Return([]*api.Volume{flexvol}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, flexvolName).AnyTimes().Return(nil, mockError)
	mockAPI.EXPECT().VolumeSetSize(ctx, flexvolName, sizeBytesStr).AnyTimes().Return(mockError)
	mockAPI.EXPECT().QtreeCount(ctx, flexvolName).AnyTimes().Return(0, nil)

	result3 := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result3, "Expected error in volume create when ineligible backend, got nil")
}

func TestCreate_WithErrorInApiOperation(t *testing.T) {
	qtreeName := "qtree1"
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		FileSystem:   "nfs",
		Name:         qtreeName,
		InternalName: qtreeName,
	}

	flexvolName := "flexvol1"
	flexvol := &api.Volume{Name: flexvolName}
	sizeBytes := 1073741824
	sizeBytesStr := strconv.FormatUint(uint64(sizeBytes), 10)
	sizeKB := 1048576
	sizeKBStr := strconv.FormatUint(uint64(sizeKB), 10)

	sb := storage.NewTestStorageBackend()
	sb.SetBackendUUID(BackendUUID)
	pool1 := getValidOntapNASPool()
	pool1.SetName("pool1")
	volAttrs := map[string]sa.Request{}

	// Case 1: Error while checking qtree existence
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, "*").Return(false, "", mockError)

	result1 := driver.Create(ctx, volConfig, pool1, volAttrs)
	assert.Error(t, result1, "Expected error when api fails to check qtree existence, got nil")

	// Case 2: Error in creating qtree
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	addCommonExpectForQtreeCreate(mockAPI, flexvol, flexvolName, sizeBytesStr)
	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, "*").Return(false, "", nil)
	mockAPI.EXPECT().QtreeCreate(ctx, qtreeName, flexvolName, "777", "default",
		"unix", "").AnyTimes().Return(mockError)

	result2 := driver.Create(ctx, volConfig, pool1, volAttrs)
	assert.Error(t, result2, "Expected error when api fails to create qtree, got nil")

	// Case 3: Error in setting quota for qtree
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, gomock.Any()).Return(false, "", nil)
	addCommonExpectForQtreeCreate(mockAPI, flexvol, flexvolName, sizeBytesStr)
	mockAPI.EXPECT().QtreeCreate(ctx, qtreeName, flexvolName, "777", "default",
		"unix", "").AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, qtreeName, flexvolName, "tree", sizeKBStr).Return(mockError)

	result3 := driver.Create(ctx, volConfig, pool1, volAttrs)
	assert.Error(t, result3, "Expected error when api fails to set quota for qtree, got nil")

	// Case 4: Error in ensuring SMB share
	mockAPI, driver = newMockOntapNasQtreeDriver(t)
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.NASType = sa.SMB
	mockAPI.EXPECT().QtreeExists(ctx, qtreeName, gomock.Any()).Return(false, "", nil)
	addCommonExpectForQtreeCreate(mockAPI, flexvol, flexvolName, sizeBytesStr)
	mockAPI.EXPECT().QtreeCreate(ctx, qtreeName, flexvolName, "777", "default",
		"unix", "").AnyTimes().Return(nil)
	mockAPI.EXPECT().QuotaSetEntry(ctx, qtreeName, flexvolName, "tree", sizeKBStr).Return(nil)
	mockAPI.EXPECT().SMBShareExists(ctx, flexvolName).Return(false, mockError)

	result4 := driver.Create(ctx, volConfig, pool1, volAttrs)
	assert.Error(t, result4, "Expected error when api fails to set quota for qtree, got nil")
}

func TestNASQtreeStorageDriverGetBackendState(t *testing.T) {
	mockApi, mockDriver := newMockOntapNasQtreeDriver(t)

	mockApi.EXPECT().GetSVMState(ctx).Return("", errors.New("returning test error"))

	reason, changeMap := mockDriver.GetBackendState(ctx)
	assert.Equal(t, reason, StateReasonSVMUnreachable, "should be 'SVM is not reachable'")
	assert.NotNil(t, changeMap, "should not be nil")
}

func TestGetCommonConfig_Success(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	driver.Config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}

	result := driver.GetCommonConfig(ctx)

	assert.NotNil(t, result, "Expected not nil config, got nil")
}

func TestCanSnapshot_Succsss(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
		SnapshotDir:  "true",
	}
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}
	result := driver.CanSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, result, "error occurred")
	assert.Nil(t, result, "result not nil")
}

func TestCanSnapshot_NoSnapshotDir(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
		SnapshotDir:  "true",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}
	volConfig.SnapshotDir = "false"
	result := driver.CanSnapshot(ctx, snapConfig, volConfig)
	assert.Error(t, result, "expecting an error")
	assert.NotNil(t, result, "result is nil")
}

func TestCanSnapshot_InvalidSnapshotDir(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
		SnapshotDir:  "true",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}
	volConfig.SnapshotDir = "wrongValue"
	result := driver.CanSnapshot(ctx, snapConfig, volConfig)
	assert.Error(t, result, "expecting an error")
	assert.NotNil(t, result, "result is nil")
}

func TestCreateSnapshot_Success(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeExists(ctx, flexvol).Return(true, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, "snap1", flexvol).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, flexvol).Return(
		api.Snapshot{
			CreateTime: "time",
			Name:       "snap1",
		},
		nil,
	)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, snap, "result is nil")
	assert.NoError(t, err, "error occurred")
}

func TestCreateSnapshot_FailureErrorCheckingVolume(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().VolumeExists(ctx, flexvol).Return(true, mockError)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestCreateSnapshot_FailureNoVolumeExists(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().VolumeExists(ctx, flexvol).Return(false, nil)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestCreateSnapshot_FailureSnapshotCreateFailed(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeExists(ctx, flexvol).Return(true, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, "snap1", flexvol).Return(mockError)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestCreateSnapshot_FailureSnapshotInfoFailed(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeExists(ctx, flexvol).Return(true, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, "snap1", flexvol).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, flexvol).Return(
		api.Snapshot{
			CreateTime: "time",
			Name:       "snap1",
		},
		mockError,
	)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestCreateSnapshot_FailureNoSnapshots(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeExists(ctx, flexvol).Return(true, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, "snap1", flexvol).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, flexvol).Return(
		api.Snapshot{},
		mockError,
	)
	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestCreateSnapshot_FailureWrongVolumeID(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   "wrong-id",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestGetSnapshot_Success(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, flexvol).Return(
		api.Snapshot{
			CreateTime: "time",
			Name:       "snap1",
		},
		nil,
	)

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, snap, "result is nil")
	assert.NoError(t, err, "error occurred")
}

func TestGetSnapshot_FailureNoSnapshotReturned(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, flexvol).Return(
		api.Snapshot{},
		errors.NotFoundError("snapshot %v not found for volume %v", snapConfig.InternalName,
			snapConfig.VolumeInternalName))

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.NoError(t, err, "error occurred")
}

func TestGetSnapshot_FailureErrorFetchingSnapshots(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapConfig.InternalName, flexvol).Return(
		api.Snapshot{},
		mockError)

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestGetSnapshot_FailureWrongVolumeID(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   "wrong-internal-id",
	}
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestGetSnapshots_Success(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, flexvol).Return(snapshots, nil)

	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.NotNil(t, snap, "result is nil")
	assert.NoError(t, err, "error occurred")
}

func TestGetSnapshots_SuccessDockerContext(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	driver.Config.DriverContext = tridentconfig.ContextDocker
	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.NoError(t, err, "error occurred")
}

func TestGetSnapshots_FailureWrongVolumeID(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   "wrong-internal-id",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestGetSnapshots_FailureSnapshotListErr(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, flexvol).Return(snapshots, mockError)

	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, snap, "result not nil")
	assert.Error(t, err, "expecting an error")
}

func TestDeleteSnapshot_Success(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().VolumeSnapshotDelete(ctx, "snap1", flexvol).Return(nil)
	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "result not nil")
	assert.NoError(t, result, "error occurred")
}

func TestDeleteSnapshot_FailureSnapshotBusy(t *testing.T) {
	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	childVols := make([]string, 0)
	childVols = append(childVols, flexvol)

	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	mockAPI.EXPECT().VolumeSnapshotDelete(ctx, "snap1", flexvol).Return(api.SnapshotBusyError("snapshot is busy"))
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, "snap1", flexvol).Return(childVols, nil)
	mockAPI.EXPECT().VolumeCloneSplitStart(ctx, flexvol).Return(nil)

	driver.cloneSplitTimers = &sync.Map{}
	// Use DefaultCloneSplitDelay to set time to past. It is defaulted to 10 seconds.
	driver.cloneSplitTimers.Store(snapConfig.ID(), time.Now().Add(-10*time.Second))
	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "result is nil")
	assert.Error(t, result, "expecting an error")
}

func TestDeleteSnapshot_FailureWrongVolumeID(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)
	childVols := make([]string, 0)
	childVols = append(childVols, flexvol)

	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   "wrong-id",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "result is nil")
	assert.Error(t, result, "expecting an error")
}

func TestNASQtreeStorageDriver_RestoreSnapshot(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: flexvol,
		InternalID:   volInternalID,
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: flexvol,
	}

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result)
}

func TestNASQtreeStorageDriver_GetQtreesInPool(t *testing.T) {
	_, driver := newMockOntapNasQtreeDriver(t)

	internalID1 := "/vol/trident_pvc_b4010eee_8b95_42c2_b7fb_9f6cd79f06df/namespace0"
	internalID2 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f854"
	internalID3 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f877"
	internalID4 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCQT/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f843"

	mockVol1 := getMockVolume("pvc-b4010eee-8b95-42c2-b7fb-9f6cd79f06df", internalID1)
	mockVol2 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f854", internalID2)
	mockVol3 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f877", internalID3)
	mockVol4 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f843", internalID4)

	allVolumes := map[string]*storage.Volume{
		"pvc-b4010eee-8b95-42c2-b7fb-9f6cd79f06df": mockVol1,
		"pvc-99138d85-6259-4830-ada0-30e45e21f854": mockVol2,
		"pvc-99138d85-6259-4830-ada0-30e45e21f877": mockVol3,
		"pvc-99138d85-6259-4830-ada0-30e45e21f843": mockVol4,
	}

	tests := []struct {
		name         string
		qPoolName    string
		expectedVols int
	}{
		{"no-vol-in qtreepool", "trident_qtree_pool_trident_XHPULXSABC", 0},
		{"vol-in-qtreepool", "trident_qtree_pool_trident_XHPULXSCYE", 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			result := driver.getQtreesInPool(test.qPoolName, allVolumes)
			assert.Equal(tt, test.expectedVols, len(result))
		})
	}
}

func TestNASQtreeStorageDriver_UpdateVolume_Success(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)

	internalID1 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f854"
	internalID2 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f877"
	internalID3 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCQT/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f843"

	mockVol1 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f854", internalID1)
	mockVol2 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f877", internalID2)
	mockVol3 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f843", internalID3)

	mockVol1.Config.SnapshotDir = "true"
	mockVol2.Config.SnapshotDir = "true"
	mockVol3.Config.SnapshotDir = "true"

	allVolumes := map[string]*storage.Volume{
		"pvc-99138d85-6259-4830-ada0-30e45e21f854": mockVol1,
		"pvc-99138d85-6259-4830-ada0-30e45e21f877": mockVol2,
		"pvc-99138d85-6259-4830-ada0-30e45e21f843": mockVol3,
	}

	updateInfo := &models.VolumeUpdateInfo{
		SnapshotDirectory: "false",
		PoolLevel:         true,
	}

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil)

	mockAPI.EXPECT().QtreeExists(gomock.Any(), "trident_pvc_99138d85_6259_4830_ada0_30e45e21f854", gomock.Any()).Return(true, "trident_qtree_pool_trident_XHPULXSCYE", nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(gomock.Any(), "trident_qtree_pool_trident_XHPULXSCYE", false).Return(nil)

	result, resultErr := driver.Update(ctx, mockVol1.Config, updateInfo, allVolumes)

	assert.NoError(t, resultErr)
	assert.NotNil(t, result)
	for _, v := range result {
		assert.True(t, strings.HasPrefix(v.Config.InternalID, "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree"))
		assert.Equal(t, updateInfo.SnapshotDirectory, v.Config.SnapshotDir)
	}
}

func TestNASQtreeStorageDriver_UpdateVolume_Failure(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	fakeErr := errors.New("fake error")

	internalID1 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f854"
	internalID2 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f877"
	internalID3 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCQT/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f843"

	mockVol1 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f854", internalID1)
	mockVol2 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f877", internalID2)
	mockVol3 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f843", internalID3)

	mockVol1.Config.SnapshotDir = "true"
	mockVol2.Config.SnapshotDir = "true"
	mockVol3.Config.SnapshotDir = "true"

	allVolumes := map[string]*storage.Volume{
		"pvc-99138d85-6259-4830-ada0-30e45e21f854": mockVol1,
		"pvc-99138d85-6259-4830-ada0-30e45e21f877": mockVol2,
		"pvc-99138d85-6259-4830-ada0-30e45e21f843": mockVol3,
	}

	updateInfo := &models.VolumeUpdateInfo{
		SnapshotDirectory: "false",
		PoolLevel:         true,
	}

	// Mock out any expected calls on the ACP API.
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()

	// CASE 1: No volume update info provided
	result, resultErr := driver.Update(ctx, mockVol1.Config, nil, allVolumes)

	assert.Error(t, resultErr)
	assert.True(t, errors.IsInvalidInputError(resultErr))
	assert.Nil(t, result)

	// CASE 2: Invalid Internal ID
	mockVol1.Config.InternalID = "invalid"

	result, resultErr = driver.Update(ctx, mockVol1.Config, updateInfo, allVolumes)

	assert.Error(t, resultErr)
	assert.Nil(t, result)

	// CASE 3: Error in checking if qtree exists
	mockVol1.Config.InternalID = internalID1
	mockAPI.EXPECT().QtreeExists(gomock.Any(), "trident_pvc_99138d85_6259_4830_ada0_30e45e21f854", gomock.Any()).Return(false, "", fakeErr)

	result, resultErr = driver.Update(ctx, mockVol1.Config, updateInfo, allVolumes)

	assert.Error(t, resultErr)
	assert.Equal(t, fakeErr.Error(), resultErr.Error())
	assert.Nil(t, result)

	// CASE 4: Qtree does not exist
	mockVol1.Config.InternalID = internalID1
	mockAPI.EXPECT().QtreeExists(gomock.Any(), "trident_pvc_99138d85_6259_4830_ada0_30e45e21f854", gomock.Any()).Return(false, "", nil)

	result, resultErr = driver.Update(ctx, mockVol1.Config, updateInfo, allVolumes)

	assert.Error(t, resultErr)
	assert.Equal(t, "volume trident_pvc_99138d85_6259_4830_ada0_30e45e21f854 does not exist", resultErr.Error())
	assert.Nil(t, result)

	// CASE 5: Error while modifying snapshot directory
	mockVol1.Config.InternalID = internalID1
	mockAPI.EXPECT().QtreeExists(gomock.Any(), "trident_pvc_99138d85_6259_4830_ada0_30e45e21f854", gomock.Any()).Return(true, "trident_qtree_pool_trident_XHPULXSCYE", nil)
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(gomock.Any(), "trident_qtree_pool_trident_XHPULXSCYE", false).Return(fakeErr)

	result, resultErr = driver.Update(ctx, mockVol1.Config, updateInfo, allVolumes)

	assert.Error(t, resultErr)
	assert.Equal(t, fakeErr.Error(), resultErr.Error())
	assert.Nil(t, result)
}

func TestNASQtreeStorageDriver_UpdateSnapshotDirectory_Success(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	internalID1 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f854"
	internalID2 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f877"
	internalID3 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCQT/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f843"

	mockVol1 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f854", internalID1)
	mockVol2 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f877", internalID2)
	mockVol3 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f843", internalID3)

	mockVol1.Config.SnapshotDir = "true"
	mockVol2.Config.SnapshotDir = "true"
	mockVol3.Config.SnapshotDir = "true"

	allVolumes := map[string]*storage.Volume{
		"pvc-99138d85-6259-4830-ada0-30e45e21f854": mockVol1,
		"pvc-99138d85-6259-4830-ada0-30e45e21f877": mockVol2,
		"pvc-99138d85-6259-4830-ada0-30e45e21f843": mockVol3,
	}

	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(gomock.Any(), "trident_qtree_pool_trident_XHPULXSCYE", false).Return(nil)

	result, resultErr := driver.updateSnapshotDirectory(ctx, mockVol1.Config, "false", true, "trident_qtree_pool_trident_XHPULXSCYE", allVolumes)

	assert.NoError(t, resultErr)
	assert.NotNil(t, result)
	for _, v := range result {
		assert.True(t, strings.HasPrefix(v.Config.InternalID, "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree"))
		assert.Equal(t, "false", v.Config.SnapshotDir)
	}
}

func TestNASQtreeStorageDriver_UpdateSnapshotDirectory_Failure(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockAPI, driver := newMockOntapNasQtreeDriver(t)
	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	fakeErr := errors.New("fake error")

	internalID1 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f854"
	internalID2 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCYE/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f877"
	internalID3 := "/svm/iscsi0/flexvol/trident_qtree_pool_trident_XHPULXSCQT/qtree/trident_pvc_99138d85_6259_4830_ada0_30e45e21f843"

	mockVol1 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f854", internalID1)
	mockVol2 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f877", internalID2)
	mockVol3 := getMockVolume("pvc-99138d85-6259-4830-ada0-30e45e21f843", internalID3)

	mockVol1.Config.SnapshotDir = "true"
	mockVol2.Config.SnapshotDir = "true"
	mockVol3.Config.SnapshotDir = "true"

	allVolumes := map[string]*storage.Volume{
		"pvc-99138d85-6259-4830-ada0-30e45e21f854": mockVol1,
		"pvc-99138d85-6259-4830-ada0-30e45e21f877": mockVol2,
		"pvc-99138d85-6259-4830-ada0-30e45e21f843": mockVol3,
	}

	// CASE: Invalid snapshot dir value
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(nil).AnyTimes()

	result, resultErr := driver.updateSnapshotDirectory(ctx, mockVol1.Config, "invalid", true, "", allVolumes)

	assert.Error(t, resultErr)
	assert.True(t, errors.IsInvalidInputError(resultErr))
	assert.Nil(t, result)

	// CASE: Pool level value is false
	result, resultErr = driver.updateSnapshotDirectory(ctx, mockVol1.Config, "false", false, "", allVolumes)

	assert.Error(t, resultErr)
	assert.True(t, errors.IsInvalidInputError(resultErr))
	assert.Equal(t, fmt.Sprintf("pool level must be set to true for updating snapshot directory of %v volume", driver.Config.StorageDriverName), resultErr.Error())
	assert.Nil(t, result)

	// CASE: Error while modifying snapshot directory
	mockVol1.Config.InternalID = internalID1
	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(gomock.Any(), "trident_qtree_pool_trident_XHPULXSCYE", false).Return(fakeErr)

	result, resultErr = driver.updateSnapshotDirectory(
		ctx, mockVol1.Config, "false", true,
		"trident_qtree_pool_trident_XHPULXSCYE", allVolumes)

	assert.Error(t, resultErr)
	assert.Equal(t, fakeErr.Error(), resultErr.Error())
	assert.Nil(t, result)
}

func TestOntapNasEcoUnpublish(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	type args struct {
		publishEnforcement bool
		autoExportPolicy   bool
	}

	tt := []struct {
		name    string
		args    args
		mocks   func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string)
		wantErr assert.ErrorAssertionFunc
	}{
		// mockAPI EXPECT calls are in order of being called.
		{
			// The qtree trident_pvc_123 is published to a node with IP addresses 1.1.1.1 and 2.2.2.2
			// This qtree is expected to have the export policy "trident_pvc_123" with the above rules.
			// After unpublish, the qtree is expected to be set to the empty export policy with no rules and
			// delete the previous export policy after their rules have been deleted.
			name: "qtreeWithOneMount",
			args: args{publishEnforcement: false, autoExportPolicy: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: qtreeName, Volume: flexVolName}, nil)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName).
					Return(map[int]string{1: "1.1.1.1", 2: "2.2.2.2"}, nil)
				mockAPI.EXPECT().ExportRuleDestroy(ctx, qtreeName, gomock.Any()).Times(2)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName)
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty")
				mockAPI.EXPECT().ExportPolicyDestroy(ctx, qtreeName)
			},
			wantErr: assert.NoError,
		},
		{
			name: "emptyPolicyDoesNotExist",
			args: args{publishEnforcement: false, autoExportPolicy: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: qtreeName, Volume: flexVolName}, nil)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName).
					Return(map[int]string{1: "1.1.1.1", 2: "2.2.2.2"}, nil)
				mockAPI.EXPECT().ExportRuleDestroy(ctx, qtreeName, gomock.Any()).Times(2)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName)
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty").
					Return(errors.NotFoundError("export policy not found"))
				mockAPI.EXPECT().ExportPolicyCreate(ctx, "tridentempty")
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty")
				mockAPI.EXPECT().ExportPolicyDestroy(ctx, qtreeName)
			},
			wantErr: assert.NoError,
		},
		{
			// The qtree trident_pvc_123 is published to two nodes,
			// node1 has IP addresses 1.1.1.1 and 2.2.2.2, node2 has IP addresses 4.4.4.4 and 5.5.5.5.
			// This qtree is expected to have the export policy "trident_pvc_123" with all the above rules.
			// After unpublish from node1, qtree trident_pvc_123 is expected to only have the IP addresses from node2.
			name: "qtreeWithTwoMounts",
			args: args{publishEnforcement: false, autoExportPolicy: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: qtreeName, Volume: flexVolName}, nil)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName).
					Return(map[int]string{1: "1.1.1.1", 2: "2.2.2.2", 4: "4.4.4.4", 5: "5.5.5.5"}, nil)
				mockAPI.EXPECT().ExportRuleDestroy(ctx, qtreeName, gomock.Any()).Times(2)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName).Return(map[int]string{4: "4.4.4.4", 5: "5.5.5.5"}, nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "qtreeDoesNotExist",
			args: args{publishEnforcement: false, autoExportPolicy: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(nil, errors.New("qtree does not exist"))
			},
			wantErr: assert.Error,
		},
		{
			name: "qtreeExistError",
			args: args{publishEnforcement: false, autoExportPolicy: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(nil, errors.New("some api error"))
			},
			wantErr: assert.Error,
		},
		{
			name: "exportRuleListError",
			args: args{publishEnforcement: false, autoExportPolicy: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: qtreeName, Volume: flexVolName}, nil)
				mockAPI.EXPECT().ExportRuleList(ctx, qtreeName).Return(nil, errors.New("some api error"))
			},
			wantErr: assert.Error,
		},
	}

	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				InternalName: "trident_pvc_123",
				ExportPolicy: "trident_pvc_123",
				AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			publishInfo := &models.VolumePublishInfo{
				HostName:         "node1",
				BackendUUID:      "1234",
				HostIP:           []string{"1.1.1.1", "2.2.2.2"},
				VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			mockAPI, driver := newMockOntapNasQtreeDriver(t)
			driver.Config.AutoExportPolicy = tr.args.autoExportPolicy

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
			tr.mocks(mockAPI, volConfig.InternalName, "flexVolName")

			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}
		})
	}
}

func TestOntapNasEcoLegacyUnpublish(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	type args struct {
		autoExportPolicy bool
		exportPolicy     string
		nodeNum          int
	}

	tt := []struct {
		name    string
		args    args
		mocks   func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName, backendPolicy string)
		wantErr assert.ErrorAssertionFunc
	}{
		// mockAPI EXPECT calls are in order of being called.
		{
			// This qtree has the backend based export policy "trident-1234" and only has a single publication.
			// After unpublish, the qtree is expected to be set to the empty export policy with no rules because it
			// is no longer in use by any pods on any node.
			name: "legacyQtreeWithOneMount",
			args: args{autoExportPolicy: true, nodeNum: 0, exportPolicy: "trident-1234"},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName, backendPolicy string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: backendPolicy, Volume: flexVolName}, nil)
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty")
			},
			wantErr: assert.NoError,
		},
		{
			// This qtree has the backend based export policy "trident-1234" but its volConfig.ExportPolicy="" because
			// this qtree vol was created using trident version <= 23.01. During Unpublish, the correct export policy
			// should be querired from backend and used.
			// After unpublish, the qtree is expected to be set to the empty export policy with no rules because it
			// is no longer in use by any pods on any node.
			name: "legacyQtreeWithEmptyExportPolicyInVolConfig",
			args: args{autoExportPolicy: true, nodeNum: 0, exportPolicy: ""},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName, backendPolicy string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: backendPolicy, Volume: flexVolName}, nil)
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty")
			},
			wantErr: assert.NoError,
		},
		{
			// This qtree has the backend based export policy "trident-1234" and only has a single publication.
			// After unpublish, the qtree is expected to be set to the empty export policy with no rules because it
			// is no longer in use by any pods on any node.
			name: "emptyPolicyDoesNotExist",
			args: args{autoExportPolicy: true, nodeNum: 0, exportPolicy: "trident-1234"},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName, backendPolicy string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: backendPolicy, Volume: flexVolName}, nil)
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty").
					Return(errors.NotFoundError("export policy not found"))
				mockAPI.EXPECT().ExportPolicyCreate(ctx, "tridentempty")
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty")
			},
			wantErr: assert.NoError,
		},
		{
			// This qtree has the backend based export policy "trident-1234" and has more than one publication.
			// After unpublish, the qtree is expected to continue to use the backend based export policy.
			name: "legacyQtreeWithMultipleMounts",
			args: args{autoExportPolicy: true, nodeNum: 2, exportPolicy: "trident-1234"},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName, backendPolicy string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: backendPolicy, Volume: flexVolName}, nil)
			},
			wantErr: assert.NoError,
		},
		{
			// This qtree is expected to have the export policy "default",
			// this is the case if a customer updates their backend to use autoExportPolicy=true after it was
			// originally false.
			// After unpublish, the qtree is expected to be set to the empty export policy.
			name: "legacyQtreeDefaultExportPolicy",
			args: args{autoExportPolicy: true, nodeNum: 0, exportPolicy: "default"},
			mocks: func(mockAPI *mockapi.MockOntapAPI, qtreeName, flexVolName, backendPolicy string) {
				mockAPI.EXPECT().QtreeGetByName(ctx, qtreeName, gomock.Any()).
					Return(&api.Qtree{ExportPolicy: "default", Volume: flexVolName}, nil).Times(1)
				mockAPI.EXPECT().QtreeModifyExportPolicy(ctx, qtreeName, flexVolName, "tridentempty").Times(1)
			},
			wantErr: assert.NoError,
		},
	}

	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				InternalName: "trident_pvc_123",
			}

			publishInfo := &models.VolumePublishInfo{
				HostName:    "node1",
				BackendUUID: "1234",
				HostIP:      []string{"1.1.1.1", "2.2.2.2"},
				Nodes:       make([]*models.Node, tr.args.nodeNum),
			}

			mockAPI, driver := newMockOntapNasQtreeDriver(t)
			driver.Config.AutoExportPolicy = tr.args.autoExportPolicy
			volConfig.ExportPolicy = tr.args.exportPolicy

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
			tr.mocks(mockAPI, volConfig.InternalName, "flexVolName", getExportPolicyName(publishInfo.BackendUUID))

			err := driver.Unpublish(ctx, volConfig, publishInfo)
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}
		})
	}
}

func getMockVolume(name, internalID string) *storage.Volume {
	return &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:       "v1",
			Name:          name,
			InternalName:  name,
			Size:          "10Mi",
			AccessInfo:    models.VolumeAccessInfo{},
			ReadOnlyClone: false,
			InternalID:    internalID,
		},
		BackendUUID: BackendUUID,
		Pool:        "",
		Orphaned:    false,
		State:       "",
	}
}
