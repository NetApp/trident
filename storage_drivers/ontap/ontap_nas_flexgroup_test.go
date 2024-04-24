// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

func newTestOntapNASFlexgroupDriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, driverContext tridentconfig.DriverContext, useREST bool, fsxId *string,
) *NASFlexGroupStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-nas-flexgroup-user"
	config.Password = "password1!"
	config.StorageDriverName = tridentconfig.OntapNASFlexGroupStorageDriverName
	config.StoragePrefix = sp("test_")
	config.DriverContext = driverContext
	config.UseREST = useREST
	config.FlexGroupAggregateList = []string{"aggr1", "aggr2"}

	if fsxId != nil {
		config.AWSConfig = &drivers.AWSConfig{}
		config.AWSConfig.FSxFilesystemID = *fsxId
	}

	nasDriver := &NASFlexGroupStorageDriver{}
	nasDriver.Config = *config

	numRecords := api.DefaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		numRecords = api.MaxZapiRecords
	}

	var ontapAPI api.OntapAPI

	if config.UseREST == true {
		ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
	} else {
		ontapAPI, _ = api.NewZAPIClientFromOntapConfig(context.TODO(), config, numRecords)
	}

	nasDriver.API = ontapAPI
	nasDriver.telemetry = &Telemetry{
		Plugin:        nasDriver.Name(),
		SVM:           config.SVM,
		StoragePrefix: *nasDriver.GetConfig().StoragePrefix,
		Driver:        nasDriver,
	}

	return nasDriver
}

func newMockAWSOntapNASFlexgroupDriver(t *testing.T) (*mockapi.MockOntapAPI, *mockapi.MockAWSAPI, *NASFlexGroupStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)
	driver.AWSAPI = mockAWSAPI
	return mockAPI, mockAWSAPI, driver
}

func newMockOntapNASFlexgroupDriver(t *testing.T) (*mockapi.MockOntapAPI, *NASFlexGroupStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	fsxId := FSX_ID

	driver := newTestOntapNASFlexgroupDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, "CSI", false, &fsxId)
	driver.API = mockAPI
	return mockAPI, driver
}

func getOntapStorageDriverConfigJson(encryption, spaceReserve, QosPolicy, adaptiveQosPolicy string,
	flexGroupAggregateList []string,
) ([]byte, error) {
	ontapStorageDriverConfigDefaults := drivers.OntapStorageDriverConfigDefaults{
		CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
			Size: "429496729600",
		},
		SpaceReserve:      spaceReserve,
		SnapshotPolicy:    "snapshotPolicy",
		SnapshotReserve:   "true",
		SplitOnClone:      "true",
		UnixPermissions:   "-rwxrwxrwx",
		SnapshotDir:       "true",
		ExportPolicy:      "Always",
		SecurityStyle:     "unix",
		Encryption:        encryption,
		TieringPolicy:     "none",
		QosPolicy:         QosPolicy,
		AdaptiveQosPolicy: adaptiveQosPolicy,
	}
	ontapStorageDriverPool := []drivers.OntapStorageDriverPool{
		{
			Region:  "us_east_1",
			Zone:    "us_east_1a",
			NASType: sa.NFS,

			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
			OntapStorageDriverConfigDefaults: ontapStorageDriverConfigDefaults,
		},
	}

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	ontapStorageDriverConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		ManagementLIF:             "127.0.0.1:0",
		SVM:                       "SVM1",
		Aggregate:                 "data",
		Username:                  "dummyuser",
		Password:                  "dummypassword",
		Storage:                   ontapStorageDriverPool,
		OntapStorageDriverPool:    ontapStorageDriverPool[0],
		FlexGroupAggregateList:    flexGroupAggregateList,
	}

	return json.Marshal(ontapStorageDriverConfig)
}

func TestOntapNasFlexgroupStorageDriverInitialized(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.initialized = true
	result := driver.Initialized()
	assert.Equal(t, true, result, "Incorrect initialization status")

	driver.initialized = false
	result = driver.Initialized()
	assert.Equal(t, false, result, "Incorrect initialization status")
}

func TestOntapNasFlexgroupStorageDriverInitialize_WithTwoAuthMethods(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-flexgroup",
		"managementLIF":     "1.1.1.1:10",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword",
		"clientcertificate": "dummy-certificate",
		"clientprivatekey":  "dummy-client-private-key"
	}`
	ontapNasDriver := newTestOntapNASFlexgroupDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
		"CSI", false, nil)

	result := ontapNasDriver.Initialize(ctx, "CSI", configJSON, commonConfig,
		map[string]string{}, BackendUUID)

	assert.Error(t, result, "driver initialization succeeded even with more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapNasFlexgroupStorageDriverInitialize_WithTwoAuthMethodsWithSecrets(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-flexgroup",
		"managementLIF":     "1.1.1.1:10",
		"svm":               "SVM1",
		"aggregate":         "data"
	}`
	secrets := map[string]string{
		"username":          "dummyuser",
		"password":          "dummypassword",
		"clientprivatekey":  "dummy-client-private-key",
		"clientcertificate": "dummy-certificate",
	}
	ontapNasDriver := newTestOntapNASFlexgroupDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
		"CSI", false, nil)

	result := ontapNasDriver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets,
		BackendUUID)

	assert.Error(t, result, "driver initialization succeeded even with more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapNasFlexgroupStorageDriverInitialize_WithTwoAuthMethodsWithConfigAndSecrets(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-flexgroup",
		"managementLIF":     "1.1.1.1:10",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword"
	}`
	secrets := map[string]string{
		"clientprivatekey":  "dummy-client-private-key",
		"clientcertificate": "dummy-certificate",
	}
	ontapNasDriver := newTestOntapNASFlexgroupDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
		"CSI", false, nil)

	result := ontapNasDriver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets,
		BackendUUID)

	assert.Error(t, result, "driver initialization succeeded even with more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapNasFlexgroupStorageDriverInitialize(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-flexgroup",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword"
	}`
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	driver.telemetry = &Telemetry{
		Plugin: driver.Name(),
		SVM:    "SVM1",
		Driver: driver,
		done:   make(chan struct{}),
	}
	driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	driver.telemetry.TridentBackendUUID = BackendUUID
	hostname, _ := os.Hostname()
	message, _ := json.Marshal(driver.GetTelemetry())

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").Return([]string{"dataLIF"}, nil)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-nas-flexgroup", "1", false, "heartbeat", hostname,
		string(message), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverInitialize_StoragePool(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	driver.telemetry = &Telemetry{
		Plugin: driver.Name(),
		SVM:    "SVM1",
		Driver: driver,
		done:   make(chan struct{}),
	}
	driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	driver.telemetry.TridentBackendUUID = BackendUUID
	hostname, _ := os.Hostname()
	message, _ := json.Marshal(driver.GetTelemetry())
	tests := []struct {
		name       string
		errMessage string
	}{
		{"NoError", ""},
		{"encryptionValueNonBool", "invalid boolean value for encryption"},
		{"flexgroupAggrListFailed", "not all aggregates specified in the flexgroupAggregateList are assigned to the SVM"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SVMAggregateNames := []string{"aggr1", "aggr2"}
			SVMAggregateAttributes := map[string]string{SVMAggregateNames[0]: "hybrid", SVMAggregateNames[1]: "hdd"}
			var configJSON []byte

			mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
			mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true).AnyTimes()
			mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return(SVMAggregateNames, nil)
			mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
				SVMAggregateAttributes, nil,
			)
			mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return([]string{"dataLIF"}, nil)
			mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-nas-flexgroup", "1", false, "heartbeat", hostname,
				string(message), 1, "trident", 5).AnyTimes()
			mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid").AnyTimes()

			if test.name == "flexgroupAggrListFailed" {
				configJSON, _ = getOntapStorageDriverConfigJson("true", "volume", "none", "",
					[]string{"InvalidAggregate"})
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				assert.Error(t, result, "Aggregate validation succeeded even with not all aggregates are assigned to the SVM")
				assert.Contains(t, result.Error(), test.errMessage)
			} else if test.name == "encryptionValueNonBool" {
				configJSON, _ = getOntapStorageDriverConfigJson("none", "volume", "none", "", []string{"aggr1"})
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				assert.Error(t, result, "Encryption field validation succeeded even with pasing non boolean value")
				assert.Contains(t, result.Error(), test.errMessage)
			} else {
				configJSON, _ = getOntapStorageDriverConfigJson("true", "volume", "none", "",
					[]string{"aggr1", "aggr2"})
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				assert.NoError(t, result)
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverInitialize_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	driver.API = nil // setting driver API nil
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-flexgroup",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword"
	}`
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "Flexgroup driver initialization succeeded even with driver API is nil")
	assert.Contains(t, result.Error(), "could not create Data ONTAP API client")
}

func TestOntapNasFlexgroupStorageDriverInitialize_StoragePoolFailed(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas-flexgroup",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword"
	}`
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	tests := []struct {
		name string
		err  string
	}{
		{"AggregatesNotFound", "Failed to get aggregates"},
		{"AggregatesListEmpty", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "AggregatesListEmpty" {
				var aggrList []string
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(aggrList, nil)
			} else {
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(nil, fmt.Errorf(test.err))
			}
			mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
			result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

			assert.Error(t, result, "Flexgroup driver initialization succeeded even with no aggregates found")
		})
	}
}

func TestOntapNasFlexgroupStorageDriverInitialize_ValidationFailed(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	tests := []struct {
		name       string
		errMesaage string
	}{
		{"SupportsFeatureFailed", "ONTAP version does not support FlexGroups"},
		{"NASDriverValidationFailed", "driver validation failed"},
		{"storagePrefixValidationFailed", "storage prefix may only contain letters/digits/underscore/dash and must begin with letter"},
		{"storagePoolValidation", "invalid spaceReserve"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()

			if test.name == "SupportsFeatureFailed" {
				configJSON, _ := getOntapStorageDriverConfigJson("true", "volume", "", "none", []string{
					"aggr1", "aggr2",
				})
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(false) // feature not supported
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
					map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
				)
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				assert.Error(t, result,
					"FlexGroup driver initialization succeeded even with ONTAP version does not support FlexGroups")
				assert.Contains(t, result.Error(), test.errMesaage)
			} else if test.name == "NASDriverValidationFailed" {
				configJSON, _ := getOntapStorageDriverConfigJson("true", "volume", "", "none", []string{
					"aggr1", "aggr2",
				})
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
					map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
				)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").Return(nil,
					fmt.Errorf("failed to get data LIFs")) // failed to get network interface
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				assert.Error(t, result, "FlexGroup driver initialization succeeded even with failed to get data lifs")
				assert.Contains(t, result.Error(), test.errMesaage)
			} else if test.name == "storagePrefixValidationFailed" {
				configJSON, _ := getOntapStorageDriverConfigJson("true", "volume", "", "none", []string{
					"aggr1", "aggr2",
				})
				storagePrefix := *driver.Config.StoragePrefix
				*driver.Config.StoragePrefix = "Test@&" // invalid storage pool prefix
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
					map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
				)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").Return([]string{"dataLIF"}, nil)
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				*driver.Config.StoragePrefix = storagePrefix
				assert.Error(t, result, "FlexGroup driver initialization succeeded even with invalid storage pool prefix")
				assert.Contains(t, result.Error(), test.errMesaage)
			} else if test.name == "storagePoolValidation" {
				configJSON, _ := getOntapStorageDriverConfigJson("true", "invalidSpaceReserve", "", "none", []string{
					"aggr1", "aggr2",
				})
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
					map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
				)
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").Return([]string{"dataLIF"}, nil)
				result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)

				assert.Error(t, result, "FlexGroup driver initialization succeeded even with invalid spaceReserve")
				assert.Contains(t, result.Error(), test.errMesaage)
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverInitialize_GetSVMAggregateNamesFailed(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON, _ := getOntapStorageDriverConfigJson("true", "volume", "", "none",
		[]string{"aggr1", "aggr2"})
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).AnyTimes().Return(true)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return([]string{"dataLIF"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return(nil, fmt.Errorf("no aggregates found"))
	result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)
	assert.Error(t, result, "FlexGroup driver initialization succeeded even with no aggregates present on SVM")
	assert.Contains(t, result.Error(), "no aggregates found")
}

func TestOntapNasFlexgroupStorageDriverInitialize_GetSVMAggregateNameEmptyList(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas-flexgroup",
		BackendName:       "myOntapNasFlexgroupBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON, _ := getOntapStorageDriverConfigJson("true", "volume", "", "none",
		[]string{"aggr1", "aggr2"})

	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).AnyTimes().Return(true)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "nfs").AnyTimes().Return([]string{"dataLIF"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	var aggrList []string
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return(aggrList, nil)
	result := driver.Initialize(ctx, "CSI", string(configJSON), commonConfig, secrets, BackendUUID)
	assert.Error(t, result, "FlexGroup driver initialization succeeded even with no aggregates present on SVM")
	assert.Contains(t, result.Error(), "no assigned aggregates found")
}

func TestOntapNasFlexgroupStorageDriverTerminate(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	tests := []struct {
		name string
		err  error
	}{
		{"TerminateSuccess", nil},
		{"TerminateFailed", fmt.Errorf("policy not found")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver.Config.AutoExportPolicy = true
			driver.telemetry = nil
			driver.initialized = true

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().ExportPolicyDestroy(ctx, "trident-dummy").Return(test.err)

			driver.Terminate(ctx, "dummy")

			assert.False(t, driver.initialized)
		})
	}
}

func TestOntapNasFlexgroupStorageDriverTerminate_TelemetryFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.AutoExportPolicy = true
	driver.telemetry = &Telemetry{
		Plugin:        driver.Name(),
		SVM:           "SVM1",
		StoragePrefix: *driver.GetConfig().StoragePrefix,
		Driver:        driver,
		done:          make(chan struct{}),
	}
	driver.initialized = true

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().ExportPolicyDestroy(ctx, "trident-dummy").Return(fmt.Errorf("policy not found"))

	driver.Terminate(ctx, "dummy")

	assert.False(t, driver.initialized)
}

func TestOntapNasFlexgroupStorageDriverConfigString(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	ontapNasFlexgroupDriver := *driver

	excludeList := map[string]string{
		"username":                             ontapNasFlexgroupDriver.Config.Username,
		"password":                             ontapNasFlexgroupDriver.Config.Password,
		"client private key":                   "BEGIN PRIVATE KEY",
		"client private key base 64 encoding ": "QkVHSU4gUFJJVkFURSBLRVk=",
	}

	includeList := map[string]string{
		"<REDACTED>":         "<REDACTED>",
		"username":           "Username:<REDACTED>",
		"password":           "Password:<REDACTED>",
		"api":                "API:<REDACTED>",
		"client private key": "ClientPrivateKey:<REDACTED>",
	}

	for key, val := range includeList {
		assert.Contains(t, ontapNasFlexgroupDriver.String(), val,
			"ontap-nas-flexgroup driver does not contain %v", key)
		assert.Contains(t, ontapNasFlexgroupDriver.GoString(), val,
			"ontap-nas-flexgroup driver does not contain %v", key)
	}

	for key, val := range excludeList {
		assert.NotContains(t, ontapNasFlexgroupDriver.String(), val,
			"ontap-nas-flexgroup driver contains %v", key)
		assert.NotContains(t, ontapNasFlexgroupDriver.GoString(), val,
			"ontap-nas-flexgroup driver contains %v", key)
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
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
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().FlexgroupCreate(ctx, gomock.Any()).Return(nil)
	mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(nil)

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "trident-"+BackendUUID, volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
}

func TestNASFlexGroupStorageDriverGetBackendState(t *testing.T) {
	mockApi, mockDriver := newMockOntapNASFlexgroupDriver(t)

	// set fake values
	mockDriver.physicalPool = storage.NewStoragePool(nil, "pool1")
	mockApi.EXPECT().GetSVMState(ctx).Return("", fmt.Errorf("returning test error"))

	reason, changeMap := mockDriver.GetBackendState(ctx)
	assert.Equal(t, reason, StateReasonSVMUnreachable, "should be 'SVM is not reachable'")
	assert.NotNil(t, changeMap, "should not be nil")
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
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
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	driver.timeout = 1 * time.Second
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().FlexgroupCreate(ctx, gomock.Any()).AnyTimes().Return(fmt.Errorf("volume creation failed"))

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result, "FlexGroup creation succeeded even with backend cannot create volume")
	assert.Contains(t, result.Error(), "volume creation failed")
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_SnapshotDisabled(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
	sb.SetBackendUUID(BackendUUID)
	pool1 := storage.NewStoragePool(sb, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		SnapshotDir:       "false",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
	})
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().FlexgroupCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().FlexgroupModifySnapshotDirectoryAccess(ctx, "vol1", false).Return(
		fmt.Errorf("failed to disable snapshot directory access"))

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result,
		"FlexGroup creation succeeded even with backend cannot disable snapshot directory access")
	assert.Contains(t, result.Error(), "failed to disable snapshot directory access")
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_MountFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
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
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().FlexgroupCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
	mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(fmt.Errorf("volume mount failed"))

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result, "FlexGroup creation succeeded even with backend cannot mount the volume")
	assert.Contains(t, result.Error(), "error mounting volume vol1 to junction: /vol1")
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_LabelLengthExceeding(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
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
	pool1.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"cloud": "anf",
		"thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
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
			"lthQFQfHVgPpUZdzZMjXry": "dev-test-cluster-1",
	})
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)

	result := driver.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result, "FlexGroup creation succeeded even with label length limit exceeds")
	assert.Contains(t, result.Error(), "label length 1160 exceeds the character limit of 1023 characters")
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_SMBShareCreatefail(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
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
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	driver.Config.NASType = sa.SMB
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		name         string
		smbShareName string
		errMessage   string
	}{
		{"UserSpecifiedShareName", "vol1", "Failed to create SMB share"},
		{"UserDoesNotSpecifiedShareName", "", "Failed to create SMB share"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver.Config.SMBShare = test.smbShareName
			mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
			mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
			mockAPI.EXPECT().FlexgroupCreate(ctx, gomock.Any()).AnyTimes().Return(nil)
			mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(nil)

			if test.smbShareName != "" {
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/").Return(fmt.Errorf(test.errMessage))
			} else {
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/vol1").Return(fmt.Errorf(test.errMessage))
			}
			result := driver.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result, "SMB Share creation succeeded even with backend failed to create share")
			assert.Contains(t, result.Error(), "Failed to create SMB share")
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_FlexgroupExistsCheckFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("fakesvm")

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
		SnapshotDir:     "true",
	})
	driver.virtualPools = map[string]storage.Pool{"pool1": pool1}

	volAttrs := map[string]sa.Request{}

	tests := []struct {
		name            string
		flexgroupExists bool
		errMessage      string
	}{
		{"flexgroupAlreadyExists", true, "volume exists"},
		{"felxgroupGetFailed", false, "error checking for existing volume"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "flexgroupAlreadyExists" {
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(test.flexgroupExists, nil)
				result := driver.Create(ctx, volConfig, pool1, volAttrs)

				assert.Error(t, result, "Flexgroup volume exists")
				assert.Contains(t, result.Error(), "volume vol1 already exists")
			} else {
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(test.flexgroupExists, fmt.Errorf(test.errMessage))
				result := driver.Create(ctx, volConfig, pool1, volAttrs)

				assert.Error(t, result, "Flexgroup volume exists")
				assert.Contains(t, result.Error(), "error checking for existing FlexGroup")
			}
		})
	}
}

func TestOntapNasStorageFlexgroupDriverVolumeCreate_GetSVMAggregateNamesFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakevol",
	}
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
		SnapshotDir:     "true",
	})
	driver.virtualPools = map[string]storage.Pool{"pool1": pool1}
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		name       string
		errMessage string
	}{
		{"AggregatesListEmpty", ""},
		{"AggregatesNotFound", "Failed to get aggregates"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("fakesvm")
			mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)

			if test.name == "AggregatesListEmpty" {
				var aggrList []string
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(aggrList, nil)
			} else {
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(nil, fmt.Errorf(test.errMessage))
			}
			driver.Config.FlexGroupAggregateList = nil
			result := driver.Create(ctx, volConfig, pool1, volAttrs)
			assert.Error(t, result, "Get the SVM aggregates")
		})
	}
}

func TestOntapNasStorageFlexgroupDriverVolumeCreate_StoragePoolFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakevol",
	}
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
		SnapshotDir:     "true",
	})

	volAttrs := map[string]sa.Request{}

	tests := []struct {
		name       string
		errMessage string
	}{
		{"InvalidValueSnapshotDir", "invalid boolean value for snapshotDir"},
		{"InvalidValueEncryption", "invalid boolean value for encryption"},
		{"InvalidValueSnapshotReserve", "invalid value for snapshotReserve"},
		{"InvalidQoSPolicy", "only one kind of QoS policy group may be defined"},
		{"InvalidValueSize", "could not convert volume size nonInt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("fakesvm")
			mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
			mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)

			driver.Config.FlexGroupAggregateList = nil

			if test.name == "InvalidValueSnapshotDir" {
				pool2 := storage.NewStoragePool(nil, "pool2")
				pool2.SetInternalAttributes(map[string]string{
					"tieringPolicy": "none",
					SnapshotDir:     "fake", // invalid SnapshotDir
				})

				result := driver.Create(ctx, volConfig, pool2, volAttrs)
				assert.Error(t, result, "Flexgroup created even with invalid SnapshotDir")
				assert.Contains(t, result.Error(), test.errMessage)
			} else if test.name == "InvalidValueEncryption" {
				volConfig.Encryption = "invalid" // invalid bool value
				result := driver.Create(ctx, volConfig, pool1, volAttrs)
				volConfig.Encryption = "true" // added valid value after the test execution

				assert.Error(t, result, "Flexgroup created even with invalid Encryption")
				assert.Contains(t, result.Error(), test.errMessage)
			} else if test.name == "InvalidValueSnapshotReserve" {
				pool2 := storage.NewStoragePool(nil, "pool2")
				pool2.SetInternalAttributes(map[string]string{
					"tieringPolicy": "none",
					SnapshotDir:     "true",
					SnapshotReserve: "fake",
				})

				result := driver.Create(ctx, volConfig, pool2, volAttrs)
				assert.Error(t, result, "Flexgroup created even with invalid SnapshotReserve")
				assert.Contains(t, result.Error(), test.errMessage)
			} else if test.name == "InvalidQoSPolicy" {
				pool2 := storage.NewStoragePool(nil, "pool2")
				pool2.SetInternalAttributes(map[string]string{
					TieringPolicy:     "none",
					SnapshotDir:       "true",
					QosPolicy:         "fake",
					AdaptiveQosPolicy: "fake",
				})

				result := driver.Create(ctx, volConfig, pool2, volAttrs)
				assert.Error(t, result)
				assert.Contains(t, result.Error(), test.errMessage)
			} else if test.name == "InvalidValueSize" {
				volConfig.Size = "nonInt" // invalid volume size
				result := driver.Create(ctx, volConfig, pool1, volAttrs)
				assert.Error(t, result)
				assert.Contains(t, result.Error(), test.errMessage)
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeCreate_FlexgroupSize(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("fakesvm")

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
		SnapshotDir:     "true",
	})
	driver.physicalPool = pool1

	tests := []struct {
		volumeSize string
		expectErr  bool
	}{
		{"invalid", true},
		{"19m", false},
		{"-1002947563b", true},
	}
	for _, test := range tests {
		t.Run(test.volumeSize, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				Size:             test.volumeSize,
				Encryption:       "false",
				FileSystem:       "nfs",
				InternalName:     "vol1",
				PeerVolumeHandle: "fakesvm:vol1",
			}

			mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)
			mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
			if !test.expectErr {
				mockAPI.EXPECT().FlexgroupCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(nil)

			}

			result := driver.Create(ctx, volConfig, pool1, map[string]sa.Request{})

			if test.expectErr {
				assert.Error(t, result, "expected error, got nil")
			} else {
				assert.NoError(t, result, "expected no error, got error")
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeDestroy_FSx(t *testing.T) {
	svmName := "SVM1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	mockAPI, mockAWSAPI, driver := newMockAWSOntapNASFlexgroupDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Name:         volName,
		InternalName: volNameInternal,
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	assert.NotNil(t, mockAPI)

	tests := []struct {
		message  string
		nasType  string
		smbShare string
		state    string
	}{
		{"Test NFS volume in FSx in available state", "nfs", "", "AVAILABLE"},
		{"Test NFS volume in FSx in deleting state", "nfs", "", "DELETING"},
		{"Test NFS volume does not exist in FSx", "nfs", "", ""},
		{"Test SMB volume does not exist in FSx", "smb", volConfig.InternalName, ""},
	}

	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			vol := awsapi.Volume{
				State: test.state,
			}
			isVolumeExists := vol.State != ""
			mockAWSAPI.EXPECT().VolumeExists(ctx, volConfig).Return(isVolumeExists, &vol, nil)
			if isVolumeExists {
				mockAWSAPI.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).Return("", nil)
				if vol.State == awsapi.StateAvailable {
					mockAWSAPI.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
				}
			} else {
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().FlexgroupDestroy(ctx, volConfig.InternalName, true).Return(nil)
				if test.nasType == sa.SMB {
					if test.smbShare == "" {
						mockAPI.EXPECT().SMBShareExists(ctx, volConfig.InternalName).Return(true, nil)
						mockAPI.EXPECT().SMBShareDestroy(ctx, volConfig.InternalName).Return(nil)
					}
					driver.Config.NASType = sa.SMB
					driver.Config.SMBShare = test.smbShare
				}
			}

			result := driver.Destroy(ctx, volConfig)

			assert.NoError(t, result)
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeDestroy(t *testing.T) {
	svmName := "SVM1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Name:         volName,
		InternalName: volNameInternal,
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	assert.NotNil(t, mockAPI)

	tests := []struct {
		message  string
		nasType  string
		smbShare string
	}{
		{"Test NFS volume", "nfs", ""},
		{"Test SMB volume", "smb", ""},
		{"Test SMB volume with share", "smb", volConfig.InternalName},
	}

	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
			mockAPI.EXPECT().FlexgroupDestroy(ctx, volConfig.InternalName, true).Return(nil)
			if test.nasType == sa.SMB {
				if test.smbShare == "" {
					mockAPI.EXPECT().SMBShareExists(ctx, volConfig.InternalName).Return(true, nil)
					mockAPI.EXPECT().SMBShareDestroy(ctx, volConfig.InternalName).Return(nil)
				}
				driver.Config.NASType = sa.SMB
				driver.Config.SMBShare = test.smbShare
			}
			result := driver.Destroy(ctx, volConfig)

			assert.NoError(t, result)
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeDestroy_Fail(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	svmName := "SVM1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Name:         volName,
		InternalName: volNameInternal,
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
	mockAPI.EXPECT().FlexgroupDestroy(ctx, volConfig.InternalName, true).Return(fmt.Errorf("cannot delete volume"))

	result := driver.Destroy(ctx, volConfig)

	assert.Error(t, result, "Flexgroup destroyed")
	assert.Contains(t, result.Error(), "cannot delete volume")
}

func TestOntapNasFlexgroupStorageDriverSMBShareDestroy_VolumeNotFound(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	svmName := "SVM1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Name:         volName,
		InternalName: volNameInternal,
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	driver.Config.NASType = sa.SMB

	tests := []struct {
		name       string
		errMessage string
	}{
		{"SMBShareServerError", "backend Server does not respond"},
		{"SMBShareNotFound", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
			mockAPI.EXPECT().FlexgroupDestroy(ctx, volConfig.InternalName, true).Return(nil)
			if test.name == "SMBShareServerError" {
				mockAPI.EXPECT().SMBShareExists(ctx, volNameInternal).Return(false,
					fmt.Errorf(test.errMessage))
				result := driver.Destroy(ctx, volConfig)
				assert.Error(t, result, "SMB Share created")
				assert.Contains(t, result.Error(), test.errMessage)
			} else {
				mockAPI.EXPECT().SMBShareExists(ctx, volNameInternal).Return(false, nil)
				result := driver.Destroy(ctx, volConfig)
				assert.NoError(t, result)
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverSMBDestroy_Fail(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	svmName := "SVM1"
	volName := "testVol"
	volNameInternal := volName + "Internal"
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Name:         volName,
		InternalName: volNameInternal,
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	driver.Config.NASType = sa.SMB

	mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
	mockAPI.EXPECT().FlexgroupDestroy(ctx, volConfig.InternalName, true).Return(nil)
	mockAPI.EXPECT().SMBShareExists(ctx, volNameInternal).Return(true, nil)
	mockAPI.EXPECT().SMBShareDestroy(ctx, volNameInternal).Return(fmt.Errorf("cannot delete SMB share"))

	result := driver.Destroy(ctx, volConfig)

	assert.Error(t, result, "SMB share destroyed")
	assert.Contains(t, result.Error(), "cannot delete SMB share")
}

func TestOntapNasFlexgroupStorageDriverVolumeClone(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "fakesvm:vol1",
	}

	sb := &storage.StorageBackend{}
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
		SplitOnClone:      "false",
	})
	driver.physicalPool = pool1
	driver.Config.AutoExportPolicy = true
	volume := api.Volume{}
	volume.Comment = "ontap"

	// the first lookup should fail, the second should succeed
	gomock.InOrder(
		mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil),
		mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil),
	)

	mockAPI.EXPECT().FlexgroupInfo(ctx, volConfig.CloneSourceVolume).Return(&volume, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(nil)

	result := driver.CreateClone(ctx, volConfig, volConfig, pool1)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeClone_StoragePoolNil(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:       "400g",
		Encryption: "false",
		FileSystem: "nfs",
	}

	flexgroup := api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, volConfig.CloneSourceVolume).Return(&flexgroup, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)

	result := driver.CreateClone(ctx, nil, volConfig, nil)

	assert.Error(t, result, "Clone created with invalid storage pool")
}

func TestOntapNasFlexgroupStorageDriverVolumeClone_CreateCloneFailed(t *testing.T) {
	tests := []struct {
		name       string
		errMessage string
	}{
		{"FlexgroupInfoFailed", "volume not found"},
		{"FlexgroupInfoNil", ""}, // backend return empty list
		{"SupportsFeatureFailed", ""},
		{"FlexgroupVolumeExists", ""}, // Volume is present with clone name
		{"FlexgroupVolumeExistsFailed", "failed to get volume"},
		{"FlexgroupSnapshotCreateFailed", "failed to create snapshot"},
		{"FlexgroupSetCommentFailed", "failed to set comment on flexgroup volume"},
		{"FlexgroupMountFailed", "failed to mount flexgroup volume"},
		{"FlexgroupSetQosPolicyGroupNameFailed", "failed to set QosPolicy on flexgroup volume"},
		{"FlexgroupCloneSplitStartFailed", "error in splitting clone"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

			pool1 := storage.NewStoragePool(nil, "pool1")
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
				SplitOnClone:      "false",
			})
			driver.physicalPool = pool1
			driver.Config.AutoExportPolicy = true

			volume := api.Volume{}
			volume.Comment = "ontap"

			volConfig := &storage.VolumeConfig{
				Size:         "400g",
				Encryption:   "false",
				FileSystem:   "nfs",
				Name:         "sourceVol",
				InternalName: "sourceVol-internal",
			}

			cloneConfig := &storage.VolumeConfig{
				Size:                      "400g",
				Encryption:                "false",
				FileSystem:                "nfs",
				CloneSourceVolume:         volConfig.Name,
				CloneSourceVolumeInternal: volConfig.InternalName,
				CloneSourceSnapshot:       "",
				Name:                      "clone1",
				InternalName:              "clone1-internal",
			}

			flexgroup := api.Volume{
				Name:    "flexgroup",
				Comment: "flexgroup",
			}

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

			if test.name == "FlexgroupInfoFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(nil, fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Original Flexgroup volume is found")

			} else if test.name == "FlexgroupInfoNil" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(nil, nil)

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Original Flexgroup volume is found")

			} else if test.name == "SupportsFeatureFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(false)

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Clone feature supported")
				assert.Contains(t, result.Error(), "the ONTAPI version does not support FlexGroup cloning")

			} else if test.name == "FlexgroupVolumeExists" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().FlexgroupExists(ctx, gomock.Any()).AnyTimes().Return(true, nil)

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Clone created despite clone volume present")
				assert.Contains(t, result.Error(), "volume "+cloneConfig.InternalName+" already exists")

			} else if test.name == "FlexgroupVolumeExistsFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true,
					fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Clone created despite error in checking for existing volume")
				assert.Contains(t, result.Error(), test.errMessage)

			} else if test.name == "FlexgroupSnapshotCreateFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)

				assert.Error(t, result, "Clone created despite failed to create snapshot")
				assert.Contains(t, result.Error(), "failed to create snapshot")

			} else if test.name == "FlexgroupSetCommentFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)

				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)

				assert.Error(t, result, "Clone created even with failed to set comment on flexgroup volume")
				assert.Contains(t, result.Error(), "failed to set comment on flexgroup volume")

			} else if test.name == "FlexgroupMountFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)

				assert.Error(t, result, "Clone volume mounted")
				assert.Contains(t, result.Error(), test.errMessage)

			} else if test.name == "FlexgroupSetQosPolicyGroupNameFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetQosPolicyGroupName(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf(test.errMessage))

				cloneConfig.QosPolicy = "fake-qos-policy"
				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)
				assert.Error(t, result, "QosPolicy set on flexgroup volume")
				assert.Contains(t, result.Error(), test.errMessage)

			} else if test.name == "FlexgroupCloneSplitStartFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupCloneSplitStart(ctx, gomock.Any()).Return(fmt.Errorf(
					test.errMessage))

				cloneConfig.SplitOnClone = "true"
				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)
				assert.Error(t, result, "Clone created while error in splitting clone")
				assert.Contains(t, result.Error(), "error splitting clone: error in splitting clone")
			}
		})
	}
}

/*
func TestOntapNasFlexgroupStorageDriverVolumeClone_CreateCloneFailed(t *testing.T) {
	tests := []struct {
		name       string
		errMessage string
	}{
		{"FlexgroupInfoFailed", "volume not found"},
		{"FlexgroupInfoNil", ""}, // backend return empty list
		{"SupportsFeatureFailed", ""},
		{"FlexgroupVolumeExists", ""}, // Volume is present with clone name
		{"FlexgroupVolumeExistsFailed", "failed to get volume"},
		{"FlexgroupSnapshotCreateFailed", "failed to create snapshot"},
		{"FlexgroupSetCommentFailed", "failed to set comment on flexgroup volume"},
		{"FlexgroupMountFailed", "failed to mount flexgroup volume"},
		{"FlexgroupSetQosPolicyGroupNameFailed", "failed to set QosPolicy on flexgroup volume"},
		{"FlexgroupCloneSplitStartFailed", "error in splitting clone"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

			pool1 := storage.NewStoragePool(nil, "pool1")
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
				SplitOnClone:      "false",
			})
			driver.physicalPool = pool1
			driver.Config.AutoExportPolicy = true

			volume := api.Volume{}
			volume.Comment = "ontap"

			volConfig := &storage.VolumeConfig{
				Size:         "400g",
				Encryption:   "false",
				FileSystem:   "nfs",
				Name:         "sourceVol",
				InternalName: "sourceVol-internal",
			}

			cloneConfig := &storage.VolumeConfig{
				Size:                      "400g",
				Encryption:                "false",
				FileSystem:                "nfs",
				CloneSourceVolume:         volConfig.Name,
				CloneSourceVolumeInternal: volConfig.InternalName,
				CloneSourceSnapshot:       "",
				Name:                      "clone1",
				InternalName:              "clone1-internal",
			}

			flexgroup := api.Volume{
				Name:    "flexgroup",
				Comment: "flexgroup",
			}

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

			if test.name == "FlexgroupInfoFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(nil, fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Original Flexgroup volume is found")

			} else if test.name == "FlexgroupInfoNil" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(nil, nil)

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Original Flexgroup volume is found")

			} else if test.name == "SupportsFeatureFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(false)

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Clone feature supported")
				assert.Contains(t, result.Error(), "the ONTAPI version does not support FlexGroup cloning")

			} else if test.name == "FlexgroupVolumeExists" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().FlexgroupExists(ctx, gomock.Any()).AnyTimes().Return(true, nil)

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Clone created despite clone volume present")
				assert.Contains(t, result.Error(), "volume "+cloneConfig.InternalName+" already exists")

			} else if test.name == "FlexgroupVolumeExistsFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, nil, cloneConfig, pool1)

				assert.Error(t, result, "Clone created despite error in checking for existing volume")
				assert.Contains(t, result.Error(), test.errMessage)

			} else if test.name == "FlexgroupSnapshotCreateFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)

				assert.Error(t, result, "Clone created despite failed to create snapshot")
				assert.Contains(t, result.Error(), "failed to create snapshot")

			} else if test.name == "FlexgroupSetCommentFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)

				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)

				assert.Error(t, result, "Clone created even with failed to set comment on flexgroup volume")
				assert.Contains(t, result.Error(), "failed to set comment on flexgroup volume")

			} else if test.name == "FlexgroupMountFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)

				assert.Error(t, result, "Clone volume mounted")
				assert.Contains(t, result.Error(), test.errMessage)

			} else if test.name == "FlexgroupSetQosPolicyGroupNameFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetQosPolicyGroupName(ctx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf(test.errMessage))

				cloneConfig.QosPolicy = "fake-qos-policy"
				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)
				assert.Error(t, result, "QosPolicy set on flexgroup volume")
				assert.Contains(t, result.Error(), test.errMessage)

			} else if test.name == "FlexgroupCloneSplitStartFailed" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, cloneConfig.CloneSourceVolumeInternal).Return(&flexgroup, nil)
				mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
				// the first lookup should fail, the second should succeed
				gomock.InOrder(
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(false, nil),
					mockAPI.EXPECT().FlexgroupExists(ctx, cloneConfig.InternalName).Return(true, nil),
				)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().FlexgroupCloneSplitStart(ctx, gomock.Any()).Return(fmt.Errorf(
					test.errMessage))

				cloneConfig.SplitOnClone = "true"
				result := driver.CreateClone(ctx, volConfig, cloneConfig, pool1)
				assert.Error(t, result, "Clone created while error in splitting clone")
				assert.Contains(t, result.Error(), "error splitting clone: error in splitting clone")
			}
		})
	}
}
*/

func TestOntapNasSFlexgrouptorageDriverVolumeClone_BothQosPolicy(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})

	driver.Config.SplitOnClone = "false"

	volConfig := &storage.VolumeConfig{
		Size:                        "400g",
		Encryption:                  "false",
		FileSystem:                  "nfs",
		CloneSourceSnapshotInternal: "flexgroup",
		QosPolicy:                   "fake",
		AdaptiveQosPolicy:           "fake",
	}

	flexgroup := api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, volConfig.CloneSourceVolume).Return(&flexgroup, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)

	result := driver.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result, "Flexgroup clone created despite both AdaptiveQosPolicy and QosPolicy define")
	assert.Contains(t, result.Error(), "only one kind of QoS policy group may be defined")
}

func TestOntapNasFlexgroupStorageDriverVolumeClone_LabelLengthExceeding(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	pool1 := storage.NewStoragePool(nil, "")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})
	driver.Config.SplitOnClone = "false"

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
	driver.Config.Labels = map[string]string{
		"cloud":   "anf",
		longLabel: "dev-test-cluster-1",
	}

	volConfig := &storage.VolumeConfig{
		Size:                        "400g",
		Encryption:                  "false",
		FileSystem:                  "nfs",
		CloneSourceSnapshotInternal: "flexgroup",
	}
	flexgroup := api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, volConfig.CloneSourceVolume).Return(&flexgroup, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)

	result := driver.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result, "FlexGroup clone creation succeeded despite label length limit exceeds")
	assert.Contains(t, result.Error(), "label length 1160 exceeds the character limit of 1023 characters")
}

func TestOntapNasFlexgroupStorageDriverVolumeClone_CreateFail(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})

	driver.Config.SplitOnClone = "false"
	driver.Config.NASType = sa.SMB

	volConfig := &storage.VolumeConfig{
		Size:                        "400g",
		Encryption:                  "false",
		FileSystem:                  "nfs",
		CloneSourceSnapshotInternal: "flexgroup",
	}

	flexgroup := api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, volConfig.CloneSourceVolume).Return(&flexgroup, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mockAPI.EXPECT().FlexgroupExists(ctx, "").Return(false, nil)
	mockAPI.EXPECT().VolumeCloneCreate(ctx, volConfig.InternalName, volConfig.CloneSourceVolumeInternal,
		volConfig.CloneSourceSnapshotInternal, true).Return(fmt.Errorf("create clone fail"))

	result := driver.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result, "Flexgroup clone created")
	assert.Contains(t, result.Error(), "create clone fail")
}

func TestOntapNasFlexgroupStorageDriverVolumeClone_SMBShareCreateFail(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})
	driver.Config.SplitOnClone = "false"
	driver.Config.NASType = sa.SMB

	volConfig := &storage.VolumeConfig{
		Size:                        "400g",
		Encryption:                  "false",
		FileSystem:                  "nfs",
		CloneSourceSnapshotInternal: "flexgroup",
	}

	flexgroup := api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, volConfig.CloneSourceVolume).Return(&flexgroup, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	// the first lookup should fail, the second should succeed
	gomock.InOrder(
		mockAPI.EXPECT().FlexgroupExists(ctx, "").Return(false, nil),
		mockAPI.EXPECT().FlexgroupExists(ctx, "").Return(true, nil),
	)
	mockAPI.EXPECT().VolumeCloneCreate(ctx, volConfig.InternalName, volConfig.CloneSourceVolumeInternal,
		volConfig.CloneSourceSnapshotInternal, true).Return(nil)
	mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().FlexgroupMount(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().SMBShareExists(ctx, volConfig.InternalName).Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, volConfig.InternalName, "/"+volConfig.InternalName).Return(
		fmt.Errorf("cannot create volume"))

	result := driver.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result, "SMB Share created")
	assert.Contains(t, result.Error(), "cannot create volume")
}

func TestOntapNasFlexgroupStorageDriverVolumeImport(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol2",
		PeerVolumeHandle: "fakesvm:vol1",
		ImportNotManaged: false,
		UnixPermissions:  DefaultUnixPermissions,
	}
	flexgroup := &api.Volume{
		Name:       "flexgroup",
		Comment:    "flexgroup",
		AccessType: "rw",
	}

	tests := []struct {
		name    string
		NasType string
	}{
		{"importNFSVolume", "nfs"},
		{"importSMBVolume", "smb"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup, nil)
			mockAPI.EXPECT().FlexgroupModifyUnixPermissions(ctx, gomock.Any(), gomock.Any(), DefaultUnixPermissions).Return(nil)

			if test.NasType == sa.SMB {
				driver.Config.NASType = sa.SMB
			}
			result := driver.Import(ctx, volConfig, "vol1")
			assert.NoError(t, result)
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeImport_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol2",
		PeerVolumeHandle: "SVM1:vol1",
		ImportNotManaged: true,
		UnixPermissions:  DefaultUnixPermissions,
	}

	flexgroup := &api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	tests := []struct {
		name       string
		errMessage string
	}{
		{"FlexgroupInfoError", "Failed to get flexgroup"},
		{"FlexgroupInfoIDError", "Failed to get flexgroup"},
		{"FlexgroupInfoAccessTypeError", "access type is not RW"},
		{"FlexgroupInfoReturnEmptylist", ""},
		{"FlexgroupInfoInvalidAccessType", ""},
		{"FlexgroupImportNotManaged", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "FlexgroupInfoError" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(nil,
					api.VolumeReadError(test.errMessage))
				result := driver.Import(ctx, volConfig, "vol1")
				assert.Error(t, result)
			} else if test.name == "FlexgroupInfoIDError" {
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup,
					api.VolumeIdAttributesReadError(test.errMessage))
				result := driver.Import(ctx, volConfig, "vol1")
				assert.Error(t, result)
			} else if test.name == "FlexgroupInfoAccessTypeError" {
				flexgroup.AccessType = "rw"
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup,
					api.VolumeSpaceAttributesReadError(test.errMessage))
				volConfig.ImportNotManaged = false

				result := driver.Import(ctx, volConfig, "vol1")
				assert.Error(t, result)
			} else if test.name == "FlexgroupInfoReturnEmptylist" {
				flexgroup.AccessType = "rw"
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(nil, nil)

				result := driver.Import(ctx, volConfig, "vol1")
				assert.Error(t, result)
			} else if test.name == "FlexgroupInfoInvalidAccessType" {
				flexgroup.AccessType = "invalidAccessType"
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup, nil)

				result := driver.Import(ctx, volConfig, "vol1")
				flexgroup.AccessType = "rw"
				assert.Error(t, result)
			} else if test.name == "FlexgroupImportNotManaged" {
				flexgroup.AccessType = "rw"
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup, nil)
				volConfig.ImportNotManaged = true

				result := driver.Import(ctx, volConfig, "vol1")
				assert.Error(t, result, "junction path is not set for volume vol1")
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeImport_ModifyComment(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol2",
		PeerVolumeHandle: "SVM1:vol1",
		ImportNotManaged: false,
		UnixPermissions:  DefaultUnixPermissions,
	}
	flexgroup := &api.Volume{
		Name: "flexgroup",
		Comment: "{\"provisioning\": {\"storageDriverName\": \"ontap-nas-flexgroup\", " +
			"\"backendName\": \"customBackendName\"}}",
		AccessType: "rw",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup, nil)
	mockAPI.EXPECT().FlexgroupSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(
		fmt.Errorf("error modifying comment"))

	result := driver.Import(ctx, volConfig, "vol1")

	assert.Error(t, result, "Flexgroup imported even with backend failed to set comment")
}

func TestOntapNasFlexgroupStorageDriverVolumeImport_UnixPermissions(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol2",
		PeerVolumeHandle: "SVM1:vol1",
		ImportNotManaged: false,
		UnixPermissions:  "",
	}
	flexgroup := &api.Volume{
		Name:       "flexgroup",
		AccessType: "rw",
	}

	tests := []struct {
		nasYype         string
		securityStyle   string
		unixPermissions string
	}{
		{nasYype: sa.SMB, securityStyle: "mixed", unixPermissions: DefaultUnixPermissions},
		{nasYype: sa.SMB, securityStyle: "ntfs", unixPermissions: ""},
		{nasYype: sa.NFS, securityStyle: "mixed", unixPermissions: DefaultUnixPermissions},
		{nasYype: sa.NFS, securityStyle: "unix", unixPermissions: DefaultUnixPermissions},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("Unix Permissions: %d", i), func(t *testing.T) {
			driver.Config.NASType = test.nasYype
			driver.Config.SecurityStyle = test.securityStyle

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup, nil)
			mockAPI.EXPECT().FlexgroupModifyUnixPermissions(ctx, gomock.Any(),
				gomock.Any(), "").Return(fmt.Errorf("error modifying unix permissions"))

			result := driver.Import(ctx, volConfig, "vol1")
			assert.Error(t, result, "Flexgroup imported even with backend failed to modify unix permissions")
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeImport_SMBShareCreateFail(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol2",
		PeerVolumeHandle: "fakesvm:vol1",
		ImportNotManaged: false,
		UnixPermissions:  DefaultUnixPermissions,
	}
	driver.Config.NASType = sa.SMB
	flexgroup := &api.Volume{
		Name:         "flexgroup",
		Comment:      "flexgroup",
		JunctionPath: "/flexgroup",
		AccessType:   "rw",
	}
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(flexgroup, nil)
	mockAPI.EXPECT().FlexgroupModifyUnixPermissions(ctx, gomock.Any(),
		gomock.Any(), DefaultUnixPermissions).Return(nil)
	mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
	mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/vol1").Return(fmt.Errorf("error creating SMB share"))

	result := driver.Import(ctx, volConfig, "vol1")
	assert.Error(t, result, "Flexgroup imported even with SMB share creation failed")
}

func TestOntapNasFlexgroupStorageDriverVolumePublish_NASType_None(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "nfs",
		InternalName:     "vol1",
		PeerVolumeHandle: "SVM1:vol1",
		ImportNotManaged: false,
		UnixPermissions:  "",
		MountOptions:     "-o nfsvers=3",
	}
	volConfig.AccessInfo.NfsPath = "/nfs"

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	result := driver.Publish(ctx, volConfig, &utils.VolumePublishInfo{})

	assert.NoError(t, result)
}

func TestOntapNasSFlexgrouptorageDriverVolumePublish_NASType_SMB(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.NASType = "smb"

	volConfig := &storage.VolumeConfig{
		Size:             "400g",
		Encryption:       "false",
		FileSystem:       "cifs",
		InternalName:     "vol1",
		PeerVolumeHandle: "SVM1:vol1",
		ImportNotManaged: false,
	}
	volConfig.AccessInfo.SMBPath = "/test_cifs_path"

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	result := driver.Publish(ctx, volConfig, &utils.VolumePublishInfo{})

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeRename(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	result := driver.Rename(ctx, "volInternal", "newVolInternal")
	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeCanSnapshot(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	result := driver.CanSnapshot(ctx, nil, nil)
	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeGetSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
	mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(snapshots, nil)

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, snap)
	assert.NoError(t, err)
}

func TestOntapNasFlexgroupStorageDriverVolumeGetSnapshot_VolumeSizeFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	tests := []struct {
		TestName                 string
		getVolumeSizeFailedError error
	}{
		{"VolumeNotFound", errors.NotFoundError("failed to get flexgroup size")},
		{"Failed to get volume", fmt.Errorf("failed to get flexgroup size")},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf(test.TestName), func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(0, test.getVolumeSizeFailedError)

			snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

			assert.Nil(t, snap)
			assert.Error(t, err, "FlexgroupUsedSize execute successfully")
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeGetSnapshot_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
	mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(nil, fmt.Errorf("no snapshots found"))

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap)
	assert.Error(t, err, "GetSnapshot FlexgroupSnapshotList execute successfullly")
}

func TestOntapNasFlexgroupStorageDriverVolumeGetSnapshot_NoError(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(0, nil)
	mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(nil, nil)

	snap, err := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, snap)
	assert.NoError(t, err)
}

func TestOntapNasFlexgroupStorageDriverVolumeGetSnapshots(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
	mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(snapshots, nil)

	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.NotNil(t, snap)
	assert.NoError(t, err)
}

func TestOntapNasFlexgroupStorageDriverVolumeGetSnapshots_VolumeSizeFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(0, fmt.Errorf("failed to get volume size"))

	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, snap)
	assert.Error(t, err, "GetSnapshots FlexgroupUsedSize execute successfully")
}

func TestConstructOntapNASFlexgroupSMBVolumePath2StorageDriverVolumeGetSnapshots_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
	mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(nil, fmt.Errorf("no snapshots found"))

	snap, err := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, snap)
	assert.Error(t, err, "GetSnapshots FlexgroupSnapshotList execute successfullly")
}

func TestOntapNasFlexgroupStorageDriverVolumeCreateSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
	mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, "snap1", "vol1").Return(nil)
	mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(snapshots, nil)

	snap, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, snap)
	assert.NoError(t, err)
}

func TestOntapNasFlexgroupStorageDriverVolumeCreateSnapshot_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "snap1",
	})

	tests := []struct {
		name    string
		message string
	}{
		{"FlexgroupExistsFailed", "Failed to get Flexgroup volume"},
		{"FlexgroupExists", ""},
		{"FlexgroupUsedSizeFailed", "Failed to get Flexgroup volume size"},
		{"FlexgroupSnapshotCreateFailed", "Failed to create snapshot"},
		{"FlexgroupSnapshotListFailed", "Failed to get snapshots"},
		{"FlexgroupSnapshotListEmpty", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "FlexgroupExistsFailed" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false,
					fmt.Errorf(test.message))

				_, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
				assert.Error(t, err, "Flexgroup volume exists")
			} else if test.name == "FlexgroupExists" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)

				_, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
				assert.Error(t, err, "Flexgroup volume exists")
			} else if test.name == "FlexgroupUsedSizeFailed" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
				mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1,
					fmt.Errorf(test.message))

				_, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
				assert.Error(t, err, "CreateSnapshot FlexgroupUsedSize successfully executed")
			} else if test.name == "FlexgroupSnapshotCreateFailed" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
				mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, "snap1", "vol1").Return(fmt.Errorf(test.message))

				_, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
				assert.Error(t, err, "CreateSnapshot FlexgroupSnapshotCreate successfully executed")
			} else if test.name == "FlexgroupSnapshotListFailed" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
				mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, "snap1", "vol1").Return(nil)
				mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(nil, fmt.Errorf(test.message))

				_, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
				assert.Error(t, err, "CreateSnapshot FlexgroupSnapshotList successfully executed")
			} else if test.name == "FlexgroupSnapshotListEmpty" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
				mockAPI.EXPECT().FlexgroupUsedSize(ctx, "vol1").Return(1, nil)
				mockAPI.EXPECT().FlexgroupSnapshotCreate(ctx, "snap1", "vol1").Return(nil)
				mockAPI.EXPECT().FlexgroupSnapshotList(ctx, "vol1").Return(nil, nil)

				_, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
				assert.Error(t, err, "CreateSnapshot FlexgroupSnapshotList is not empty")
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverVolumeRestoreSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().SnapshotRestoreFlexgroup(ctx, "snap1", "vol1").Return(nil)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeRestoreSnapshot_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().SnapshotRestoreFlexgroup(ctx, "snap1", "vol1").Return(fmt.Errorf("failed to restore volume"))

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "SnapshotRestoreFlexgroup successfully executed")
}

func TestOntapNasFlexgroupStorageDriverVolumeDeleteSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupSnapshotDelete(ctx, "snap1", "vol1").Return(nil)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeDeleteSnapshot_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	childVols := make([]string, 0)
	childVols = append(childVols, "vol1")

	mockAPI.EXPECT().FlexgroupSnapshotDelete(ctx, "snap1", "vol1").Return(api.SnapshotBusyError("snapshot is busy"))
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, "snap1", "vol1").Return(childVols, nil)
	mockAPI.EXPECT().FlexgroupCloneSplitStart(ctx, "vol1").Return(nil)

	driver.cloneSplitTimers = make(map[string]time.Time)
	// Use DefaultCloneSplitDelay to set time to past. It is defaulted to 10 seconds.
	driver.cloneSplitTimers[snapConfig.ID()] = time.Now().Add(-10 * time.Second)
	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "FlexgroupSnapshotDelete sucessfully executed")
}

func TestOntapNasFlexgroupStorageDriverVolumeGet(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)

	result := driver.Get(ctx, "vol1")

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverVolumeGet_Error(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, fmt.Errorf("error checking for existing volume"))

	result := driver.Get(ctx, "vol1")

	assert.Error(t, result, "Flexgroup volume exists")
	assert.Contains(t, result.Error(), "error checking for existing volume")
}

func TestOntapNasFlexgroupStorageDriverVolumeGet_DoesNotExist(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)

	result := driver.Get(ctx, "vol1")

	assert.Error(t, result, "Flexgroup volume exists")
}

func TestOntapNasFlexgroupStorageDriverGetStorageBackendSpecs(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	backend := storage.StorageBackend{}

	physicalPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "pool1")
	physicalPool[pool.Name()] = pool
	driver.physicalPool = pool

	backend.SetStorage(physicalPool)

	result1 := driver.GetStorageBackendSpecs(ctx, &backend)

	assert.NoError(t, result1)

	virtualPool := make(map[string]storage.Pool)
	virtualPool[pool.Name()] = pool
	driver.virtualPools = virtualPool

	result2 := driver.GetStorageBackendSpecs(ctx, &backend)

	assert.NoError(t, result2)
}

func TestOntapNasFlexgroupStorageDriverGetStorageBackendPhysicalPoolNames(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	physicalPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "pool1")
	physicalPool[pool.Name()] = pool
	driver.physicalPool = pool

	poolNames := driver.GetStorageBackendPhysicalPoolNames(ctx)

	assert.Equal(t, "pool1", poolNames[0], "Pool names are not equal")
}

func TestOntapNasFlexgroupStorageDriverGetStorageBackendPools(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	svmUUID := "SVM1-uuid"
	pool := storage.NewStoragePool(nil, "pool1")
	driver.physicalPool = pool
	mockAPI.EXPECT().GetSVMUUID().Return(svmUUID)

	pools := driver.getStorageBackendPools(ctx)
	backendPool := pools[0]
	assert.NotEmpty(t, pools)
	assert.Equal(t, svmUUID, backendPool.SvmUUID)
}

func TestOntapNasFlexgroupStorageDriverGetInternalVolumeName(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.StoragePrefix = utils.Ptr("storagePrefix_")

	volName := driver.GetInternalVolumeName(ctx, "vol1")

	assert.Equal(t, "storagePrefix_vol1", volName, "Strings not equal")
}

func TestOntapNasFlexgroupStorageDriverGetProtocol(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)

	protocol := driver.GetProtocol(ctx)
	assert.Equal(t, protocol, tridentconfig.File, "Protocols not equal")
}

func TestOntapNasFlexgroupStorageDriverGetVolumeExternalWrappers(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{}}, nil).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)
}

func TestOntapNasFlexgroupStorageDriverGetVolumeExternalWrappers_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupListByPrefix(ctx, gomock.Any()).Return(nil, fmt.Errorf("no volume found"))

	driver.GetVolumeExternalWrappers(ctx, channel)

	assert.Equal(t, 1, len(channel))
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_NASType_None(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	flexgroup := api.Volume{
		Name:         "flexgroup",
		Comment:      "flexgroup",
		JunctionPath: "",
		AccessType:   "rw",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
	mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(nil)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_NASType_SMB(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	driver.Config.NASType = "smb"

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "cifs",
		InternalName: "vol1",
	}

	flexgroup := api.Volume{
		Name:         "flexgroup",
		Comment:      "flexgroup",
		JunctionPath: "",
		AccessType:   "rw",
	}

	tests := []struct {
		message   string
		isSuccess bool
	}{
		{"SMB share created successfully", false},
		{"SMB share creation fail", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
			mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(nil)
			if test.isSuccess {
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/vol1").Return(nil)

				result := driver.CreateFollowup(ctx, volConfig)
				assert.NoError(t, result)
			} else {
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/vol1").Return(fmt.Errorf("SMB share creation failed"))

				result := driver.CreateFollowup(ctx, volConfig)
				assert.Error(t, result, "SMB Share Created")
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_VolumeInfoFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}
	tests := []struct {
		name      string
		isSuccess bool
	}{
		{"VolumeInfoFailure", false},
		{"VolumeInfoEmpty", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "VolumeInfoFailure" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(nil, fmt.Errorf("could not find volume"))

				result := driver.CreateFollowup(ctx, volConfig)

				assert.Error(t, result, "Get the flexgroup volume info")
			} else if test.name == "VolumeInfoEmpty" {
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().FlexgroupInfo(ctx, gomock.Any()).Return(nil, nil)

				result := driver.CreateFollowup(ctx, volConfig)

				assert.Error(t, result, "Get the flexgroup volume info")
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_VolumeMountFailure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	flexgroup := api.Volume{
		Name:         "flexgroup",
		Comment:      "flexgroup",
		JunctionPath: "",
		AccessType:   "dp",
		DPVolume:     true,
	}

	tests := []struct {
		name    string
		message string
	}{
		{"MountNFSVolumeFailed", "nfs volume mount failed"},
		{"MountVolumeAPIError", "mount volume failed"},
		{"MountSMBVolumeFailed", "smb volume mount failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
			if test.name == "MountVolumeAPIError" {
				mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(api.ApiError(test.message))

				result := driver.CreateFollowup(ctx, volConfig)
				assert.Error(t, result, "Flexgroup volume mounted")
			} else if test.name == "MountNFSVolumeFailed" {
				mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(fmt.Errorf(test.message))

				result := driver.CreateFollowup(ctx, volConfig)
				assert.Error(t, result, "Flexgroup volume mounted")
			} else if test.name == "MountSMBVolumeFailed" {
				driver.Config.NASType = sa.SMB
				mockAPI.EXPECT().FlexgroupMount(ctx, "vol1", "/vol1").Return(fmt.Errorf(test.message))

				result := driver.CreateFollowup(ctx, volConfig)
				assert.Error(t, result, "Flexgroup volume mounted")
			}
		})
	}
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_WithJunctionPath_NASType_None(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	flexgroup := api.Volume{
		Name:         "flexgroup",
		Comment:      "flexgroup",
		JunctionPath: "/vol1",
		AccessType:   "rw",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_WithJunctionPath_NASType_SMB(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.NASType = "smb"

	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "cifs",
		InternalName: "vol1",
	}

	flexgroup := api.Volume{
		Name:         "flexgroup",
		Comment:      "flexgroup",
		JunctionPath: "\\vol1",
		AccessType:   "rw",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverCreateFollowup_GetStoragePoolAttributes(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)

	poolAttr := driver.getStoragePoolAttributes()

	assert.NotNil(t, poolAttr)
	assert.Equal(t, driver.Name(), poolAttr[BackendType].ToString())
	assert.Equal(t, "true", poolAttr[Snapshots].ToString())
	assert.Equal(t, "true", poolAttr[Clones].ToString())
	assert.Equal(t, "true", poolAttr[Encryption].ToString())
	assert.Equal(t, "false", poolAttr[Replication].ToString())
	assert.Equal(t, "thick,thin", poolAttr[ProvisioningType].ToString())
}

func TestOntapNasFlexgroupStorageDriverCreatePrepare(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}
	driver.CreatePrepare(ctx, volConfig)
}

func TestOntapNasFlexgroupStorageDriverGetUpdateType(t *testing.T) {
	mockAPI, oldDriver := newMockOntapNASFlexgroupDriver(t)

	oldDriver.API = mockAPI
	prefix1 := "test_"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Username = "user1"
	oldDriver.Config.Password = "password1"
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	newDriver := newTestOntapNASFlexgroupDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME,
		"CSI", false, nil)
	newDriver.API = mockAPI
	prefix2 := "storage_"
	newDriver.Config.StoragePrefix = &prefix2
	newDriver.Config.Username = "user2"
	newDriver.Config.Password = "password2"
	newDriver.Config.StoragePrefix = &prefix2
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.UsernameChange)
	expectedBitmap.Add(storage.PasswordChange)
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestOntapNasFlexgroupStorageDriverGetUpdateType_Failure(t *testing.T) {
	mockAPI, _ := newMockOntapNASFlexgroupDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	oldDriver := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, false, nil, mockAPI)
	oldDriver.API = mockAPI
	prefix1 := "test_"
	oldDriver.Config.StoragePrefix = &prefix1

	// Created a SAN driver
	newDriver := newTestOntapNASFlexgroupDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME,
		"CSI", false, nil)
	newDriver.API = mockAPI
	prefix2 := "storage_"
	newDriver.Config.StoragePrefix = &prefix2

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestOntapNasFlexgroupStorageDriverResize(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	flexgroup := api.Volume{
		Name:       "flexgroup",
		Comment:    "flexgroup",
		Aggregates: aggr,
	}
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupSize(ctx, "vol1").Return(uint64(429496729600), nil)
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
	mockAPI.EXPECT().FlexgroupSetSize(ctx, "vol1", "536870912000").Return(nil)

	result := driver.Resize(ctx, volConfig, 536870912000) // 500GB

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverResize_VolumeDoesNotExist(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(false, nil)

	result := driver.Resize(ctx, volConfig, 429496729600) // 500GB

	assert.Error(t, result, "Flexgroup volume exists")
}

func TestOntapNasFlexgroupStorageDriverResize_SameSize(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupSize(ctx, "vol1").Return(uint64(429496729600), nil)

	result := driver.Resize(ctx, volConfig, 429496729600) // 1GB

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverResize_NoVolumeInfo(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupSize(ctx, "vol1").Return(uint64(429496729600), nil)
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(nil, fmt.Errorf("error fetching volume info"))
	mockAPI.EXPECT().FlexgroupSetSize(ctx, "vol1", "536870912000").Return(nil)

	result := driver.Resize(ctx, volConfig, 536870912000) // 500GB

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverResize_WithAggregate(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	flexgroup := api.Volume{
		Name:       "flexgroup",
		Comment:    "flexgroup",
		Aggregates: aggr,
	}
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupSize(ctx, "vol1").Return(uint64(429496729600), nil)
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
	mockAPI.EXPECT().FlexgroupSetSize(ctx, "vol1", "536870912000").Return(nil)

	result := driver.Resize(ctx, volConfig, 536870912000) // 500GB

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverResize_FakeLimitVolumeSize(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	// Added fake LimitVolumeSize value
	driver.Config.CommonStorageDriverConfig.LimitVolumeSize = "fake"
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	flexgroup := api.Volume{
		Name:       "flexgroup",
		Comment:    "flexgroup",
		Aggregates: aggr,
	}
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupSize(ctx, "vol1").Return(uint64(429496729600), nil)
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
	mockAPI.EXPECT().FlexgroupSetSize(ctx, "vol1", "536870912000").Return(nil)

	result := driver.Resize(ctx, volConfig, 536870912000) // 500GB

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverResize_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	flexgroup := api.Volume{
		Name:       "flexgroup",
		Comment:    "flexgroup",
		Aggregates: aggr,
	}
	volConfig := &storage.VolumeConfig{
		Size:         "400g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	mockAPI.EXPECT().FlexgroupExists(ctx, "vol1").Return(true, nil)
	mockAPI.EXPECT().FlexgroupSize(ctx, "vol1").Return(uint64(429496729600), nil)
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)
	mockAPI.EXPECT().FlexgroupSetSize(ctx, "vol1", "536870912000").Return(fmt.Errorf("cannot resize to specified size"))

	result := driver.Resize(ctx, volConfig, 536870912000) // 500GB

	assert.Error(t, result, "Flexgroup volume resized")
}

func TestOntapNasFlexgroupStorageDriverStoreConfig(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	ontapConf := newOntapStorageDriverConfig()
	ontapConf.StorageDriverName = "ontap-nas-flexgroup"
	backendConfig := &storage.PersistentStorageBackendConfig{
		OntapConfig: ontapConf,
	}

	driver.StoreConfig(ctx, backendConfig)

	assert.Equal(t, &driver.Config, backendConfig.OntapConfig)
}

func TestOntapNasFlexgroupStorageDriverReconcileNodeAccess(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	nodes := make([]*utils.Node, 0)
	nodes = append(nodes, &utils.Node{Name: "node1"})

	result := driver.ReconcileNodeAccess(ctx, nodes, "1234", "")

	assert.NoError(t, result)
}

func TestOntapNasFlexgroupStorageDriverGetVolumeExternal(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	flexgroup := api.Volume{
		Name:    "flexgroup",
		Comment: "flexgroup",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(&flexgroup, nil)

	volExt, err := driver.GetVolumeExternal(ctx, "vol1")

	assert.NotNil(t, volExt)
	assert.NoError(t, err)
}

func TestOntapNasFlexgroupStorageDriverGetVolumeExternal_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().FlexgroupInfo(ctx, "vol1").Return(nil, fmt.Errorf("error fetching volume info"))

	volExt, err := driver.GetVolumeExternal(ctx, "vol1")

	assert.Nil(t, volExt)
	assert.Error(t, err, "Get the flexgroup volume info")
}

func TestOntapNasFlexgroupStorageDriverGetZAPIVolumeExternal(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)

	var volumeName azgo.VolumeNameType
	volumeName = "test_externalVolume"

	volumeIdAttributes := azgo.VolumeIdAttributesType{
		NamePtr: &volumeName,
	}

	permission := "777"
	volumeSecurityAttributes := azgo.VolumeSecurityAttributesType{
		VolumeSecurityUnixAttributesPtr: &azgo.VolumeSecurityUnixAttributesType{
			PermissionsPtr: &permission,
		},
	}

	size := 100000000
	volumeSpaceAttributes := azgo.VolumeSpaceAttributesType{
		SizePtr: &size,
	}

	snapshotPolicy := "snapshotPolicy"
	volumeSnapshotAttributes := azgo.VolumeSnapshotAttributesType{
		SnapshotPolicyPtr: &snapshotPolicy,
	}
	volumeExportAttributes := azgo.VolumeExportAttributesType{
		PolicyPtr: &snapshotPolicy,
	}

	volumeAttributes := azgo.VolumeAttributesType{
		VolumeIdAttributesPtr:       &volumeIdAttributes,
		VolumeSecurityAttributesPtr: &volumeSecurityAttributes,
		VolumeSpaceAttributesPtr:    &volumeSpaceAttributes,
		VolumeSnapshotAttributesPtr: &volumeSnapshotAttributes,
		VolumeExportAttributesPtr:   &volumeExportAttributes,
	}

	volumeConfig := driver.getVolumeExternal(&volumeAttributes)
	config := *volumeConfig.Config
	assert.Equal(t, "externalVolume", config.Name)
	assert.Equal(t, "test_externalVolume", config.InternalName)
	assert.Equal(t, "snapshotPolicy", config.SnapshotPolicy)
	assert.Equal(t, "snapshotPolicy", config.ExportPolicy)
	assert.Equal(t, "777", config.UnixPermissions)
	assert.Equal(t, "100000000", config.Size)
}

func TestOntapNasFlexgroupStorageDriverGetCommonConfig(t *testing.T) {
	_, driver := newMockOntapNASFlexgroupDriver(t)
	driver.Config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}

	result := driver.GetCommonConfig(ctx)

	assert.NotNil(t, result)
}

func TestOntapNasFlexgroupStorageDriverEnsureSMBShare(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)

	driver.Config.SMBShare = "smbShare"
	tests := []struct {
		name       string
		errMessage string
	}{
		{"SMBShareCreateWithGivenName", ""},
		{"SMBShareCreateWithGivenName_Failed", "SMB share create failed"},
		{"SMBShareCreateWithGivenName_shareExists", "SMB share get failed"},
		{"SMBShareCreateWithVolumeName", ""},
		{"SMBShareCreateWithVolumeName_Failed", "SMB share create failed"},
		{"SMBShareCreateWithVolumeName_shareExists", "SMB share get failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "SMBShareCreateWithGivenName" {
				mockAPI.EXPECT().SMBShareExists(ctx, "smbShare").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "smbShare", "/").Return(nil)

				result := driver.EnsureSMBShare(ctx, driver.Config.SMBShare, "/"+driver.Config.SMBShare)
				assert.NoError(t, result, "")
			} else if test.name == "SMBShareCreateWithGivenName_Failed" {
				mockAPI.EXPECT().SMBShareExists(ctx, "smbShare").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "smbShare", "/").Return(fmt.Errorf(test.errMessage))

				result := driver.EnsureSMBShare(ctx, driver.Config.SMBShare, "/"+driver.Config.SMBShare)
				assert.Error(t, result, "SMB share created")
			} else if test.name == "SMBShareCreateWithGivenName_shareExists" {
				mockAPI.EXPECT().SMBShareExists(ctx, "smbShare").Return(false, fmt.Errorf(test.errMessage))

				result := driver.EnsureSMBShare(ctx, driver.Config.SMBShare, "/"+driver.Config.SMBShare)
				assert.Error(t, result, "SMB share exists")
			} else if test.name == "SMBShareCreateWithVolumeName" {
				driver.Config.SMBShare = ""
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/vol1").Return(nil)

				result := driver.EnsureSMBShare(ctx, "vol1", "/vol1")
				assert.NoError(t, result, "")
			} else if test.name == "SMBShareCreateWithVolumeName_Failed" {
				driver.Config.SMBShare = ""
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, nil)
				mockAPI.EXPECT().SMBShareCreate(ctx, "vol1", "/vol1").Return(fmt.Errorf(test.errMessage))

				result := driver.EnsureSMBShare(ctx, "vol1", "/vol1")
				assert.Error(t, result, "SMB share created")
			} else if test.name == "SMBShareCreateWithVolumeName_shareExists" {
				driver.Config.SMBShare = ""
				mockAPI.EXPECT().SMBShareExists(ctx, "vol1").Return(false, fmt.Errorf(test.errMessage))

				result := driver.EnsureSMBShare(ctx, "vol1", "/vol1")
				assert.Error(t, result, "SMB share exists")
			}
		})
	}
}

func TestOntapNasFlexgroupGetVserverAggrMediaType(t *testing.T) {
	mockAPI, driver := newMockOntapNASFlexgroupDriver(t)
	aggrList := map[string]string{"aggr1": "vmdisk"}

	tests := []struct {
		name       string
		errMessage string
	}{
		{"Aggregate", ""},
		{"AggregateNameNotMatched", ""},
		{"GetSVMAggregateAttributes_Failed", "Failed to get SVM aggregate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "AggregateNameNotMatched" {
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).Return(aggrList, nil)

				_, err := driver.getVserverAggrMediaType(ctx, []string{"aggr3"})
				assert.NoError(t, err)
			} else if test.name == "GetSVMAggregateAttributes_Failed" {
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).Return(
					nil, fmt.Errorf(test.errMessage))

				_, err := driver.getVserverAggrMediaType(ctx, []string{"aggr2"})
				assert.Error(t, err, "Get the SVM aggregate")
			} else if test.name == "Aggregate" {
				mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).Return(aggrList, nil)

				_, err := driver.getVserverAggrMediaType(ctx, []string{"aggr1"})
				assert.NoError(t, err, "Failed to get SVM aggregate")
			}
		})
	}
}
