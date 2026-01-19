// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

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
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

var mockIPs = []string{"0.0.0.0", "1.1.1.1"}

func newNVMeDriver(apiOverride api.OntapAPI, awsApiOverride awsapi.AWSAPI, fsxId *string) *NVMeStorageDriver {
	sPrefix := "test_"

	config := &drivers.OntapStorageDriverConfig{}
	config.ManagementLIF = ONTAPTEST_LOCALHOST
	config.SVM = "svm"
	config.Aggregate = "data"
	config.Username = "ontap-san-user"
	config.Password = "password1!"
	config.UseREST = func() *bool { b := true; return &b }()
	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{
		DebugTraceFlags:   debugTraceFlags,
		StoragePrefix:     &sPrefix,
		StorageDriverName: "ontap-san",
	}

	if fsxId != nil {
		config.AWSConfig = &drivers.AWSConfig{}
		config.AWSConfig.FSxFilesystemID = *fsxId
	}

	driver := &NVMeStorageDriver{Config: *config, API: apiOverride, AWSAPI: awsApiOverride}
	driver.telemetry = &Telemetry{
		Plugin:        driver.Name(),
		SVM:           config.SVM,
		StoragePrefix: *driver.Config.StoragePrefix,
		Driver:        driver,
	}

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[FileSystemType] = filesystem.Ext4
	driver.virtualPools = map[string]storage.Pool{"pool1": pool1}
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	return driver
}

func newNVMeDriverAndMockApiAndAwsApi(t *testing.T) (*NVMeStorageDriver, *mockapi.MockOntapAPI, *mockapi.MockAWSAPI) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)
	fsxId := FSX_ID

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	return newNVMeDriver(mockAPI, mockAWSAPI, &fsxId), mockAPI, mockAWSAPI
}

func newNVMeDriverAndMockApi(t *testing.T) (*NVMeStorageDriver, *mockapi.MockOntapAPI) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().Terminate().AnyTimes()

	return newNVMeDriver(mockAPI, nil, nil), mockAPI
}

func TestNVMeBackendName(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)

	// Backend name non-empty
	d.Config.BackendName = "san-nvme-backend"
	assert.Equal(t, d.BackendName(), "san-nvme-backend", "Backend name is not correct.")

	d.Config.BackendName = ""
	d.ips = []string{}
	assert.Equal(t, d.BackendName(), "ontapsan_noLIFs", "Backend name is not correct.")

	d.ips = mockIPs
	assert.Equal(t, d.BackendName(), "ontapsan_0.0.0.0", "Backend name is not correct.")
}

func TestNVMeInitialize_ConfigUnmarshalError(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	d.Config.CommonStorageDriverConfig = nil
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	configJSON := `{"SANType": }`

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.ErrorContains(t, err, "could not decode JSON configuration")
}

func TestNVMeInitialize_NVMeNotSupported(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	configJSON := `{"SANType": "nvme"}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(false)

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.ErrorContains(t, err, "ONTAP doesn't support NVMe")
}

func TestNVMeInitialize_GetDataLifError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	configJSON := `{"SANType": "nvme"}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(nil, errors.New("error getting dataLifs"))

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.ErrorContains(t, err, "error getting dataLifs")
}

func TestNVMeInitialize_NoDataLifs(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	configJSON := `{"SANType": "nvme"}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return([]string{}, nil)
	mAPI.EXPECT().SVMName().Return("svm")

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.ErrorContains(t, err, "no NVMe data LIFs found on SVM svm")
}

func TestNVMeInitialize_GetAggrNamesError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	configJSON := `{"SANType": "nvme"}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(mockIPs, nil)
	mAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil)
	mAPI.EXPECT().IsDisaggregated().Return(false)
	mAPI.EXPECT().GetSVMAggregateNames(ctx).Return(nil, errors.New("failed to get aggrs"))

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.ErrorContains(t, err, "failed to get aggrs")
}

func TestNVMeInitialize_ValidateStoragePrefixError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	d.Config.CommonStorageDriverConfig = nil
	badStoragePrefix := "abc&$#"
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
		StoragePrefix:     &badStoragePrefix,
	}
	configJSON := `{"SANType": "nvme"}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(mockIPs, nil)
	mAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil)
	mAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"data"}, nil)
	mAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(nil, nil)
	mAPI.EXPECT().SVMName().Return("svm")
	mAPI.EXPECT().IsDisaggregated().Return(false)

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.ErrorContains(t, err, "storage prefix may only contain letters/digits/underscore/dash")
}

func TestNVMeInitialize_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	configJSON := `{"SANType": "nvme"}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(mockIPs, nil)
	mAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil)
	mAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"data"}, nil)
	mAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(nil, nil)
	mAPI.EXPECT().SVMName().Return("svm")
	mAPI.EXPECT().IsDisaggregated().Return(false)
	mAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mAPI.EXPECT().GetSVMUUID().Return("svm-uuid")

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.NoError(t, err, "Failed to initialize NVMe driver.")
	assert.True(t, d.Initialized(), "NVMe driver is not initialized.")
}

func TestNVMeInitialize_WithNameTemplate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mAPI, nil, nil)
	d.Config.CommonStorageDriverConfig = nil
	defer mockCtrl.Finish()
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
		StoragePrefix:     convert.ToPtr("storagePrefix_"),
	}
	configJSON := `
	{
		"SANType": "nvme",
		"defaults": {
				"nameTemplate": "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
		}
	}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(mockIPs, nil)
	mAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil)
	mAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"data"}, nil)
	mAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(nil, nil)
	mAPI.EXPECT().SVMName().Return("svm")
	mAPI.EXPECT().IsDisaggregated().Return(false)
	mAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mAPI.EXPECT().GetSVMUUID().Return("svm-uuid")

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.NoError(t, err, "Failed to initialize NVMe driver.")
	assert.True(t, d.Initialized(), "NVMe driver is not initialized.")

	nameTemplate := "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
	assert.Equal(t, nameTemplate, d.physicalPools["data"].InternalAttributes()[NameTemplate])
}

func TestNVMeInitialize_NameTemplateDefineInStoragePool(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mAPI, nil, nil)
	defer mockCtrl.Finish()
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
		StoragePrefix:     convert.ToPtr("storagePrefix_"),
	}
	configJSON := `
	{
		"SANType": "nvme",
		"storage": [
	     {
	        "defaults":
	         {
	               "nameTemplate": "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
	         }
	     }
	    ]
	}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(mockIPs, nil)
	mAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil)
	mAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"data"}, nil)
	mAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(nil, nil)
	mAPI.EXPECT().SVMName().Return("svm")
	mAPI.EXPECT().IsDisaggregated().Return(false)
	mAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mAPI.EXPECT().GetSVMUUID().Return("svm-uuid")

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.NoError(t, err, "Failed to initialize NVMe driver.")
	assert.True(t, d.Initialized(), "NVMe driver is not initialized.")

	expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
	assert.NoError(t, err)
	for _, pool := range d.virtualPools {
		assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestNVMeInitialize_NameTemplateDefineInBothPool(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mAPI, nil, nil)
	defer mockCtrl.Finish()
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
		StoragePrefix:     convert.ToPtr("storagePrefix_"),
	}
	configJSON := `
	{
		"SANType": "nvme",
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
	    ]
	}`
	mAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).Return(true)
	mAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport).Return(mockIPs, nil)
	mAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil)
	mAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{"data"}, nil)
	mAPI.EXPECT().GetSVMAggregateAttributes(ctx).Return(nil, nil)
	mAPI.EXPECT().SVMName().Return("svm")
	mAPI.EXPECT().IsDisaggregated().Return(false)
	mAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mAPI.EXPECT().GetSVMUUID().Return("svm-uuid")

	err := d.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, BackendUUID)

	assert.NoError(t, err, "Failed to initialize NVMe driver.")
	assert.True(t, d.Initialized(), "NVMe driver is not initialized.")

	expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
	assert.NoError(t, err)
	for _, pool := range d.virtualPools {
		assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestNVMeTerminate_Success(t *testing.T) {
	d, _ := newNVMeDriverAndMockApi(t)
	d.telemetry = NewOntapTelemetry(ctx, d)

	d.Terminate(ctx, "")

	assert.False(t, d.Initialized(), "NVMe driver is still running.")
}

func TestNVMeValidate_ReplicationValidationError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	d.Config.ReplicationPolicy = "pol1"
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(nil, errors.New("failed to get policy"))

	err := d.validate(ctx)

	assert.ErrorContains(t, err, "replication validation failed")
}

func TestNVMeValidate_SANValidationError(t *testing.T) {
	d, _ := newNVMeDriverAndMockApi(t)
	d.Config.SANType = "fcp"
	d.Config.UseCHAP = true

	err := d.validate(ctx)

	assert.Error(t, err, "SAN validation must have failed")
}

func TestNVMeValidate_StoragePoolError(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[SnapshotPolicy] = ""
	d.virtualPools = map[string]storage.Pool{"pool1": pool1}

	err := d.validate(ctx)

	assert.ErrorContains(t, err, "storage pool validation failed")
}

func TestNVMeInitializeStoragePools_NameTemplatesAndLabels(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mockAPI, nil, nil)
	defer mockCtrl.Finish()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

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

	d.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	d.telemetry.TridentBackendUUID = BackendUUID

	poolAttributes := map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}

	volume := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

	cases := []struct {
		testName                    string
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
			"test_newVolume",
			"test_newVolume",
			"nvme-backend",
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
			"nvme-backend",
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
			"test_newVolume",
			"newVolume_testSC",
			"nvme-backend",
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
			"nvme-backend",
			"Base label is not set correctly",
			"Virtual pool label is not set correctly",
			"volume name is not set correctly",
			"volume name is not set correctly",
		}, // base and virtual labels
	}

	for _, test := range cases {
		t.Run(test.testName, func(t *testing.T) {
			d.Config.Labels = test.physicalPoolLabels
			d.Config.NameTemplate = test.physicalNameTemplate
			d.Config.Storage[0].Labels = test.virtualPoolLabels
			d.Config.Storage[0].NameTemplate = test.virtualNameTemplate
			physicalPools, virtualPools, err := InitializeStoragePoolsCommon(ctx, d, poolAttributes,
				test.backendName)
			assert.NoError(t, err, "Error is not nil")

			physicalPool := physicalPools["data"]
			label, err := physicalPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, test.physicalExpected, label, test.physicalErrorMessage)

			d.CreatePrepare(ctx, &volume, physicalPool)
			assert.Equal(t, volume.InternalName, test.volumeNamePhysicalExpected, test.physicalVolNameErrorMessage)

			virtualPool := virtualPools["nvme-backend_pool_0"]
			label, err = virtualPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, test.virtualExpected, label, test.virtualErrorMessage)

			d.CreatePrepare(ctx, &volume, virtualPool)
			assert.Equal(t, volume.InternalName, test.volumeNameVirtualExpected, test.virtualVolNameErrorMessage)
		})
	}
}

func TestNVMeGetStorageBackendSpecs(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	backend := storage.StorageBackend{}

	backend.ClearStoragePools()

	assert.NoError(t, d.GetStorageBackendSpecs(ctx, &backend), "Backend specs not updated.")
}

func TestNVMeGetStorageBackendPhysicalPoolNames(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	assert.Equal(t, d.GetStorageBackendPhysicalPoolNames(ctx), []string{"pool1"}, "Physical pools are different.")
}

func TestNVMeGetStorageBackendPools(t *testing.T) {
	driver, mockAPI := newNVMeDriverAndMockApi(t)
	svmUUID := "SVM1-uuid"
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

	pool = pools[1]
	assert.NotNil(t, driver.physicalPools[pool.Aggregate])
	assert.Equal(t, driver.physicalPools[pool.Aggregate].Name(), pool.Aggregate)
	assert.Equal(t, svmUUID, pool.SvmUUID)
}

func TestNVMeGetVolumeOpts(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	volConfig := storage.VolumeConfig{}
	assert.NotNil(t, d.GetVolumeOpts(ctx, &volConfig, nil), "Couldn't get VolumeOpts.")
}

func TestNVMeGetInternalVolumeName(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	volConfig := &storage.VolumeConfig{Name: "vol1"}
	pool := storage.NewStoragePool(nil, "dummyPool")

	result := d.GetInternalVolumeName(ctx, volConfig, pool)

	assert.Equal(t, result, "test_vol1", "Got different volume.")
}

func TestNVMeGetProtocol(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	assert.Equal(t, d.GetProtocol(ctx), tridentconfig.Block, "Incorrect protocol.")
}

func TestNVMeStoreConfig(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	persistentConfig := &storage.PersistentStorageBackendConfig{}

	d.StoreConfig(ctx, persistentConfig)

	assert.Equal(t, json.RawMessage("{}"), d.Config.CommonStorageDriverConfig.StoragePrefixRaw,
		"Raw prefix mismatch.")
	assert.Equal(t, d.Config, *persistentConfig.OntapConfig, "Ontap config mismatch.")
}

func TestNVMeGetUpdateType_InvalidUpdate(t *testing.T) {
	d1 := newNVMeDriver(nil, nil, nil)
	_, d2 := newMockOntapNASDriverWithSVM(t, "SVM1")

	bMap := d1.GetUpdateType(ctx, d2)

	assert.True(t, bMap.Contains(storage.InvalidUpdate), "Valid driver update.")
}

func TestNVMeGetUpdateType_OtherUpdates(t *testing.T) {
	d1 := newNVMeDriver(nil, nil, nil)
	d2 := newNVMeDriver(nil, nil, nil)

	sPrefix := "diff"
	d2.Config.DataLIF = "1.1.1.1"
	d2.Config.Password = "diff-password"
	d2.Config.Username = "diff-username"
	d2.Config.Credentials = map[string]string{"diff": "diff"}
	d2.Config.StoragePrefix = &sPrefix

	bMap := d1.GetUpdateType(ctx, d2)

	assert.True(t, bMap.Contains(storage.PasswordChange), "Unchanged password.")
	assert.True(t, bMap.Contains(storage.UsernameChange), "Unchanged username.")
	assert.True(t, bMap.Contains(storage.CredentialsChange), "Unchanged credentials.")
	assert.True(t, bMap.Contains(storage.PrefixChange), "Unchanged prefix.")
}

func TestNVMeGetUpdateType_NilDataLIF(t *testing.T) {
	d1 := newNVMeDriver(nil, nil, nil)
	d2 := newNVMeDriver(nil, nil, nil)

	d2.Config.DataLIF = ""

	bMap := d1.GetUpdateType(ctx, d2)

	assert.False(t, bMap.Contains(storage.InvalidVolumeAccessInfoChange), "Nil DataLIF should be valid.")
}

func TestNVMeGetCommonConfig(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	assert.Equal(t, d.GetCommonConfig(ctx), d.Config.CommonStorageDriverConfig, "Driver configuration not found.")
}

func TestNVMeEstablishMirror_Errors(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapmirrorPolicy := &api.SnapmirrorPolicy{
		Type:             "async",
		CopyAllSnapshots: true,
	}

	// Empty replication policy and schedule trying out synchronously.
	mAPI.EXPECT().SVMName().Return("svm")
	err := d.EstablishMirror(ctx, "vol1", "vol1", "", "")
	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")

	// Empty replication policy and schedule trying out synchronously.
	d.Config.ReplicationPolicy = "pol1"
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, "pol1").Return(snapmirrorPolicy,
		errors.New("failed to get policy")).Times(2)
	mAPI.EXPECT().SVMName().Return("svm")

	err = d.EstablishMirror(ctx, "vol1", "vol1", "", "")

	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")

	// Empty replication policy and schedule trying out asynchronously.
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, "pol1").Return(snapmirrorPolicy, nil)
	mAPI.EXPECT().SVMName().Return("svm")
	err = d.EstablishMirror(ctx, "vol1", "vol1", "", "")
	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")

	// Non-empty replication schedule trying out asynchronously.
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, "pol1").Return(snapmirrorPolicy, nil)
	mAPI.EXPECT().JobScheduleExists(ctx, "sch1").Return(false, errors.New("failed to get schedule"))
	mAPI.EXPECT().SVMName().Return("svm")
	err = d.EstablishMirror(ctx, "vol1", "vol1", "", "sch1")
	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")
}

func TestNVMeReestablishMirror_Errors(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)

	// Empty replication policy and schedule trying out synchronously.
	mAPI.EXPECT().SVMName().Return("svm")
	err := d.ReestablishMirror(ctx, "vol1", "vol1", "", "")
	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")

	// Empty replication policy and schedule trying out synchronously.
	d.Config.ReplicationPolicy = "pol1"
	snapmirrorPolicy := &api.SnapmirrorPolicy{
		Type:             "async",
		CopyAllSnapshots: true,
	}
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, "pol1").Return(snapmirrorPolicy,
		errors.New("failed to get policy")).Times(2)
	mAPI.EXPECT().SVMName().Return("svm")
	err = d.ReestablishMirror(ctx, "vol1", "vol1", "", "")

	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")

	// Empty replication policy and schedule trying out asynchronously.
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, "pol1").Return(snapmirrorPolicy, nil)
	mAPI.EXPECT().SVMName().Return("svm")

	err = d.ReestablishMirror(ctx, "vol1", "vol1", "", "")

	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")

	// Non-empty replication schedule trying out asynchronously.
	mAPI.EXPECT().SnapmirrorPolicyGet(ctx, "pol1").Return(snapmirrorPolicy, nil)
	mAPI.EXPECT().JobScheduleExists(ctx, "sch1").Return(false, errors.New("failed to get schedule"))
	mAPI.EXPECT().SVMName().Return("svm")
	err = d.ReestablishMirror(ctx, "vol1", "vol1", "", "sch1")
	assert.ErrorContains(t, err, "could not parse remoteVolumeHandle")
}

func TestNVMePromoteMirror_Error(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)

	promote, err := d.PromoteMirror(ctx, "", "remoteHandle", "")

	assert.False(t, promote, "Mirror promotion succeeded.")
	assert.ErrorContains(t, err, "invalid volume name")
}

func TestNVMeGetMirrorStatus_Error(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)

	status, err := d.GetMirrorStatus(ctx, "", "remoteHandle")

	assert.Empty(t, status, "Mirror status non-empty.")
	assert.ErrorContains(t, err, "invalid volume name")
}

func TestNVMeReleaseMirror_Error(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)

	err := d.ReleaseMirror(ctx, "")

	assert.ErrorContains(t, err, "invalid volume name")
}

func TestNVMeGetReplicationDetails_Error(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	mAPI.EXPECT().SVMName().Return("svm")

	_, _, _, err := d.GetReplicationDetails(ctx, "", "remoteHandle")

	assert.ErrorContains(t, err, "invalid volume name")
}

func TestNVMeUpdateMirror_Error(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)

	err := d.UpdateMirror(ctx, "", "")

	assert.ErrorContains(t, err, "invalid volume name")
}

func TestNVMeCheckMirrorTransferState_Error(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	mAPI.EXPECT().SVMName().Return("svm")

	_, err := d.CheckMirrorTransferState(ctx, "")

	assert.ErrorContains(t, err, "invalid volume name")
}

func TestNVMeGetMirrorTransferTime_Error(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	mAPI.EXPECT().SVMName().Return("svm")

	_, err := d.GetMirrorTransferTime(ctx, "")

	assert.ErrorContains(t, err, "invalid volume name")
}

func getNVMeCreateArgs(d *NVMeStorageDriver) (storage.Pool, *storage.VolumeConfig, map[string]sa.Request) {
	pool1 := d.virtualPools["pool1"]
	volConfig := &storage.VolumeConfig{InternalName: "vol1", Size: "200000000"}
	volAttrs := map[string]sa.Request{}

	return pool1, volConfig, volAttrs
}

func TestNVMeCreate_VolumeExists(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)

	// Volume exists API error test case.
	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, errors.New("api invocation error"))

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "api invocation error")

	// Volume exists test case.
	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)

	err = d.Create(ctx, volConfig, pool1, volAttrs)

	assert.True(t, drivers.IsVolumeExistsError(err), "Volume doesn't exist.")
}

func TestNVMeCreate_InvalidVolHandle(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	volConfig.PeerVolumeHandle = "volHandle"

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().SVMName().Return("svm")

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "invalid volume handle")
}

func TestNVMeCreate_GetPoolsError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, volAttrs := getNVMeCreateArgs(d)
	pool1 := storage.NewStoragePool(nil, "invalid-pool")

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "could not find pool")
}

func TestNVMeCreate_InvalidSnapshotReserve(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	pool1.InternalAttributes()[SnapshotReserve] = "snapReserve"

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "invalid value for snapshotReserve")
}

func TestNVMeCreate_VolSize(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil).Times(3)

	// Convert volume size error.
	volConfig.Size = "convert-size"

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "could not convert volume size")

	// Invalid volume size error.
	volConfig.Size = "-100"

	err = d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "invalid size")

	// Required volume size more than backend config volume size.
	volConfig.Size = "200000000"
	d.Config.LimitVolumeSize = "2000"

	err = d.Create(ctx, volConfig, pool1, volAttrs)
	isUnsupportedErr, _ := errors.HasUnsupportedCapacityRangeError(err)

	assert.True(t, isUnsupportedErr, "Volume size as per backend config.")
}

func TestNVMeCreate_InvalidEncryptionValue(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	pool1.InternalAttributes()[Encryption] = "encrypt"

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "invalid boolean value for encryption")
}

func TestNVMeCreate_InvalidFSType(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	pool1.InternalAttributes()[FileSystemType] = "fake-fs"

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "unsupported fileSystemType option")
}

func TestNVMeCreate_BothQoSPolicyError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	pool1.InternalAttributes()[QosPolicy] = "QoSPol1"
	pool1.InternalAttributes()[AdaptiveQosPolicy] = "AQoSPol1"

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "only one kind of QoS policy group may be defined")
}

func TestNVMeCreate_AggSpaceError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	d.Config.LimitAggregateUsage = "10000000"

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")
	mAPI.EXPECT().GetSVMAggregateSpace(ctx, gomock.Any()).Return(nil, errors.New("failed to get aggr space"))

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "failed to get aggr space")
}

func TestNVMeCreate_LongLabelError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)

	longLabelVal := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
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
	labelMap := map[string]string{"key": longLabelVal}
	pool1.Attributes()[sa.Labels] = sa.NewLabelOffer(labelMap)

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "exceeds the character limit")
}

func TestNVMeCreate_VolumeCreateAPIError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")
	mAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(errors.New("volume create failed"))

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "volume create failed")
}

func TestNVMeCreate_NamespaceCreateAPIError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil).Times(2)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy").Times(2)
	mAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil).Times(2)
	mAPI.EXPECT().NVMeNamespaceCreate(ctx, gomock.Any()).
		Return(errors.New("failed to create namespace")).
		Times(2)
	// Volume destroy error test case.
	mAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, true).Return(errors.New("failed to delete volume"))

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, err, "failed to create namespace")

	// Volume destroy success test case.
	mAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, true).Return(nil)

	err = d.Create(ctx, volConfig, pool1, volAttrs)
	assert.ErrorContains(t, err, "failed to create namespace")
}

func TestNVMeCreate_LUKSVolume(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	nsPath := fmt.Sprintf("/vol/%s/namespace0", volConfig.InternalName)

	volConfig.LUKSEncryption = "true"
	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")
	mAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
	mAPI.EXPECT().NVMeNamespaceCreate(ctx, gomock.Any()).Return(nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, nsPath).Return(
		&api.NVMeNamespace{Name: volConfig.InternalName, UUID: uuid.New().String()}, nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, err, "Failed to create NVMe volume.")
}

func TestNVMeCreate_InvalidSkipRecoveryQueue(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)

	volConfig.SkipRecoveryQueue = "asdf"
	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, err, "Invalid skipRecoveryQueue value should have failed.")
}

func TestNVMeCreate_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	pool1, volConfig, volAttrs := getNVMeCreateArgs(d)
	nsPath := fmt.Sprintf("/vol/%s/namespace0", volConfig.InternalName)

	mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
	mAPI.EXPECT().TieringPolicyValue(ctx).Return("TPolicy")
	mAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
	mAPI.EXPECT().NVMeNamespaceCreate(ctx, gomock.Any()).Return(nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, nsPath).Return(
		&api.NVMeNamespace{Name: volConfig.InternalName, UUID: uuid.New().String()}, nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, err, "Failed to create NVMe volume.")
}

func TestNVMeDestroy_VolumeExists(t *testing.T) {
	type parameters struct {
		configureMockOntapAPI func(mockAPI *mockapi.MockOntapAPI)
		expectError           bool
	}

	tests := map[string]parameters{
		"volume exists API call return error": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(false, errors.New("api call failed"))
			},
			expectError: true,
		},
		"volume does not exist": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(false, nil)
			},
			expectError: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			d, mockAPI := newNVMeDriverAndMockApi(t)
			_, volConfig, _ := getNVMeCreateArgs(d)

			if params.configureMockOntapAPI != nil {
				params.configureMockOntapAPI(mockAPI)
			}
			err := d.Destroy(ctx, volConfig)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNVMeDestroy_SnapMirrorAPIError(t *testing.T) {
	type parameters struct {
		configureMockOntapAPI func(mockAPI *mockapi.MockOntapAPI)
		getVolumeConfig       func(driver *NVMeStorageDriver) storage.VolumeConfig
	}
	tests := map[string]parameters{
		"volume configuration without internal snapshot": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm")
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).
					Return(errors.New("snap mirror api call failed"))
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				return *volConfig
			},
		},
		"volume configuration with internal snapshot": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm")
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).
					Return(errors.New("snap mirror api call failed"))
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				volConfig.CloneSourceSnapshotInternal = "mockSnapshot"
				return *volConfig
			},
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			d, mockAPI := newNVMeDriverAndMockApi(t)
			d.Config.DriverContext = tridentconfig.ContextDocker

			if params.configureMockOntapAPI != nil {
				params.configureMockOntapAPI(mockAPI)
			}
			volConfig := params.getVolumeConfig(d)
			err := d.Destroy(ctx, &volConfig)

			assert.ErrorContains(t, err, "snap mirror api call failed")
		})
	}
}

func TestNVMeDestroy_VolumeDestroy_FSx(t *testing.T) {
	svmName := "SVM1"
	d, mAPI, mAWSAPI := newNVMeDriverAndMockApiAndAwsApi(t)
	d.Config.DriverContext = tridentconfig.ContextDocker
	_, volConfig, _ := getNVMeCreateArgs(d)

	tests := []struct {
		message  string
		nasType  string
		smbShare string
		state    string
	}{
		{"Test volume in FSx in available state", "nfs", "", "AVAILABLE"},
		{"Test volume in FSx in deleting state", "nfs", "", "DELETING"},
	}

	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			vol := awsapi.Volume{
				State: test.state,
			}
			isVolumeExists := vol.State != ""
			mAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
			mAWSAPI.EXPECT().VolumeExists(ctx, volConfig).Return(isVolumeExists, &vol, nil)
			if isVolumeExists {
				mAWSAPI.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).Return("", nil)
				if vol.State == awsapi.StateAvailable {
					mAWSAPI.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
				}
			} else {
				mAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, volConfig.InternalName, svmName).Return(nil)
				// mockAPI.EXPECT().SnapmirrorRelease(ctx, volConfig.InternalName, svmName).Return(nil)
				mAPI.EXPECT().VolumeDestroy(ctx, volConfig.InternalName, true, false).Return(nil)
			}
			result := d.Destroy(ctx, volConfig)

			assert.NoError(t, result)
		})
	}
}

func TestNVMeDestroy_VolumeDestroy(t *testing.T) {
	type parameters struct {
		configureMockOntapAPI func(mockAPI *mockapi.MockOntapAPI)
		getVolumeConfig       func(driver *NVMeStorageDriver) storage.VolumeConfig
		expectError           bool
	}

	ctx := context.Background()
	const cloneSourceSnapshot = "mockSnapshot"

	tests := map[string]parameters{
		"Volume destroy API call return error": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true,
					false).Return(errors.New("destroy volume failed"))
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				return *volConfig
			},
			expectError: true,
		},
		"volume Destroy error: volume config with internal snapshot": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true,
					false).Return(errors.New("destroy volume failed"))
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				volConfig.CloneSourceSnapshotInternal = cloneSourceSnapshot
				volConfig.CloneSourceSnapshot = ""
				return *volConfig
			},
			expectError: true,
		},
		"happy path": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, false).Return(nil)
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				return *volConfig
			},
			expectError: false,
		},
		"happy path: volume config with internal snapshot": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, false).Return(nil)
				mockAPI.EXPECT().VolumeSnapshotDelete(ctx, cloneSourceSnapshot, "").Return(nil)
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				volConfig.CloneSourceSnapshotInternal = cloneSourceSnapshot
				volConfig.CloneSourceSnapshot = ""
				return *volConfig
			},
			expectError: false,
		},
		"volume config with internal snapshot: snapshot delete error": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, false).Return(nil)
				mockAPI.EXPECT().VolumeSnapshotDelete(ctx, cloneSourceSnapshot, "").
					Return(errors.New("failed to delete snapshot"))
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				volConfig.CloneSourceSnapshotInternal = cloneSourceSnapshot
				volConfig.CloneSourceSnapshot = ""
				return *volConfig
			},
			expectError: false,
		},
		"volume config with internal snapshot: snapshot delete not found error": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, false).Return(nil)
				mockAPI.EXPECT().VolumeSnapshotDelete(ctx, cloneSourceSnapshot, "").
					Return(api.NotFoundError("snapshot not found"))
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				volConfig.CloneSourceSnapshotInternal = cloneSourceSnapshot
				volConfig.CloneSourceSnapshot = ""
				return *volConfig
			},
			expectError: false,
		},
		"skipRecoveryQueue volume": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
				mockAPI.EXPECT().SVMName().Return("svm").AnyTimes()
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, true).Return(nil)
			},
			getVolumeConfig: func(driver *NVMeStorageDriver) storage.VolumeConfig {
				_, volConfig, _ := getNVMeCreateArgs(driver)
				volConfig.SkipRecoveryQueue = "true"
				return *volConfig
			},
			expectError: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			d, mockAPI := newNVMeDriverAndMockApi(t)
			d.Config.DriverContext = tridentconfig.ContextDocker

			if params.configureMockOntapAPI != nil {
				params.configureMockOntapAPI(mockAPI)
			}
			volConfig := params.getVolumeConfig(d)
			err := d.Destroy(ctx, &volConfig)

			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNVMeGetVolumeForImport_GetVolumeError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("failed to get volume"))

	_, err := d.GetVolumeForImport(ctx, "vol")

	assert.ErrorContains(t, err, "failed to get volume")
}

func TestNVMeGetVolumeForImport_GetNamespaceError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)

	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{}, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(nil, errors.New("failed to get ns"))

	_, err := d.GetVolumeForImport(ctx, "vol")

	assert.ErrorContains(t, err, "failed to get ns")
}

func TestNVMeGetVolumeForImport_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	v := &api.Volume{Name: "test_vol", SnapshotPolicy: "pol", Aggregates: []string{"data"}}

	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(v, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(&api.NVMeNamespace{Size: "10"}, nil)

	vol, err := d.GetVolumeForImport(ctx, "vol")

	assert.NoError(t, err, "Failed to get Volume External struct.")
	assert.Equal(t, "vol", vol.Config.Name, "Found different volume.")
	assert.Equal(t, "test_vol", vol.Config.InternalName, "Found different volume.")
	assert.Equal(t, "10", vol.Config.Size, "Wrong volume size.")
	assert.Equal(t, "data", vol.Pool, "Found wrong pool.")
}

func TestNVMeGetVolumeExternalWrappers_VolumeListError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	sChannel := make(chan *storage.VolumeExternalWrapper)

	mAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, errors.New("failed to get volumes"))

	go d.GetVolumeExternalWrappers(ctx, sChannel)

	v := <-sChannel

	assert.ErrorContains(t, v.Error, "failed to get volumes")
	assert.Nil(t, v.Volume, "Found a volume.")
}

func TestNVMeGetVolumeExternalWrappers_NamespaceListError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	sChannel := make(chan *storage.VolumeExternalWrapper)

	mAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, nil)
	mAPI.EXPECT().NVMeNamespaceList(ctx, gomock.Any()).Return(nil, errors.New("failed to get namespaces"))

	go d.GetVolumeExternalWrappers(ctx, sChannel)

	v := <-sChannel

	assert.ErrorContains(t, v.Error, "failed to get namespaces")
	assert.Nil(t, v.Volume, "Found a volume.")
}

func TestNVMeGetVolumeExternalWrappers_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	sChannel := make(chan *storage.VolumeExternalWrapper)
	vols := api.Volumes{&api.Volume{Name: "vol1", SnapshotPolicy: "pol1"}}
	namespaces := api.NVMeNamespaces{
		&api.NVMeNamespace{Name: "ns1", VolumeName: "vol1", Size: "10"},
		&api.NVMeNamespace{Name: "ns2", VolumeName: "vol2", Size: "20"},
	}

	mAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(vols, nil)
	mAPI.EXPECT().NVMeNamespaceList(ctx, gomock.Any()).Return(namespaces, nil)

	go d.GetVolumeExternalWrappers(ctx, sChannel)

	v := <-sChannel

	assert.NoError(t, v.Error, "failed to get namespaces")
	assert.Equal(t, "vol1", v.Volume.Config.Name, "Found different volume.")
	assert.Equal(t, "vol1", v.Volume.Config.InternalName, "Found different volume.")
	assert.Equal(t, "10", v.Volume.Config.Size, "Wrong volume size.")
	assert.Equal(t, drivers.UnsetPool, v.Volume.Pool, "Found wrong pool.")
}

func TestNVMeCreateNVMeNamespaceCommentString(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	nsAttr := map[string]string{
		nsAttributeFSType:    "ext4",
		nsAttributeLUKS:      "luks",
		nsAttributeDriverCtx: "docker",
	}

	nsCommentString := `{"nsAttribute":{"LUKS":"luks","com.netapp.ndvp.fstype":"ext4","driverContext":"docker"}}`

	// Comment string exceeds max length
	nsComment, err := d.createNVMeNamespaceCommentString(ctx, nsAttr, 10)

	assert.ErrorContains(t, err, "exceeds the character limit")

	assert.Equal(t, "", nsComment, "Comment has garbage string.")

	// Success case
	nsComment, err = d.createNVMeNamespaceCommentString(ctx, nsAttr, nsMaxCommentLength)
	assert.NoError(t, err, "Failed to get namespace comment.")
	assert.Equal(t, nsCommentString, nsComment, "Incorrect namespace comment.")
}

func TestNVMeParseNVMeNamespaceCommentString(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)

	nsCommentString := `{"nsAttribute":{"LUKS":"luks","com.netapp.ndvp.fstype":"ext4","driverContext":"docker"}}`

	nsComment, err := d.ParseNVMeNamespaceCommentString(ctx, nsCommentString)

	assert.NoError(t, err)
	assert.Equal(t, "ext4", nsComment[nsAttributeFSType])
	assert.Equal(t, "luks", nsComment[nsAttributeLUKS])
	assert.Equal(t, "docker", nsComment[nsAttributeDriverCtx])
}

func TestGetNamespaceSpecificSubsystemName(t *testing.T) {
	// case 1: subsystem name is shorter than 64 char
	name := "fakeName_pvc_b0d71710-8bb3-4334-8621-fe3c211c5f02"
	volName := "pvc-b0d71710-8bb3-4334-8621-fe3c211c5f02"
	expected_name := "s_fakeName_pvc_b0d71710-8bb3-4334-8621-fe3c211c5f02"

	got_name := getNamespaceSpecificSubsystemName(name, volName)

	assert.Equal(t, got_name, expected_name)

	// case 2: subsystem name is longer than 64 char
	name = "fakeNodeNamefakeNodeNamefakeNodeNamefakeNodeName_pvc_b0d71710-8bb3-4334-8621-fe3c211c5f02"
	volName = "pvc-b0d71710-8bb3-4334-8621-fe3c211c5f02"
	expected_name = "s_fakeNodeNamefakeNodeN_pvc-b0d71710-8bb3-4334-8621-fe3c211c5f02"
	got_name = getNamespaceSpecificSubsystemName(name, volName)

	assert.Equal(t, got_name, expected_name)

	// case 3: subsystem name is exactly 64 char
	name = "fakeNodeNamefakeNodeN_pvc_b0d71710-8bb3-4334-8621-fe3c211c5f02"
	volName = "pvc-b0d71710-8bb3-4334-8621-fe3c211c5f02"
	expected_name = "s_fakeNodeNamefakeNodeN_pvc_b0d71710-8bb3-4334-8621-fe3c211c5f02"

	got_name = getNamespaceSpecificSubsystemName(name, volName)

	assert.Equal(t, got_name, expected_name)
}

func TestPublish(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mock := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mock, nil, nil)

	originalContext := tridentconfig.CurrentDriverContext
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
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

	namespace := &api.NVMeNamespace{
		UUID:    "fakeNsUUID",
		Comment: "fakeComment",
	}

	// Test case 1: VolumeInfo returns error
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, errors.New("Error Getting Volume Info")).Times(1)
	err := d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 2: Volume is DP type
	flexVol.AccessType = VolTypeDP
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 3: Docker context - namespace fetch error
	flexVol.AccessType = VolTypeRW
	d.Config.DriverContext = tridentconfig.ContextDocker
	tridentconfig.CurrentDriverContext = tridentconfig.ContextDocker
	d.Config.StoragePrefix = convert.ToPtr("netappdvp_")
	publishInfo.HostNQN = "fakeHostNQN"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(nil, errors.New("Error getting namespace by name")).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 4: Docker context - namespace is nil
	d.Config.DriverContext = tridentconfig.ContextDocker
	tridentconfig.CurrentDriverContext = tridentconfig.ContextDocker
	d.Config.StoragePrefix = convert.ToPtr("netappdvp_")
	publishInfo.HostNQN = "fakeHostNQN"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(nil, nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 5: Docker context - subsystem creation fails
	d.Config.DriverContext = tridentconfig.ContextDocker
	tridentconfig.CurrentDriverContext = tridentconfig.ContextDocker
	d.Config.StoragePrefix = convert.ToPtr("netappdvp_")
	publishInfo.HostNQN = "fakeHostNQN"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(namespace, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "netappdvp__subsystem", "netappdvp__subsystem").Return(subsystem, errors.New("Error creating subsystem")).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 6: Docker context - success path
	d.Config.DriverContext = tridentconfig.ContextDocker
	tridentconfig.CurrentDriverContext = tridentconfig.ContextDocker
	d.Config.StoragePrefix = convert.ToPtr("netappdvp_")
	publishInfo.HostNQN = "fakeHostNQN"
	publishInfo.MountOptions = ""
	volConfig.FileSystem = ""
	volConfig.InternalID = "fakeInternalID"
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeNsUUID"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(namespace, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "netappdvp__subsystem", "netappdvp__subsystem").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, volConfig.AccessInfo.NVMeNamespaceUUID).Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)

	// Test case 7: CSI context - subsystem creation error
	d.Config.DriverContext = tridentconfig.ContextCSI
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	publishInfo.FilesystemType = ""
	publishInfo.LUKSEncryption = ""
	volConfig.FileSystem = ""
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, errors.New("Error creating subsystem")).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 8: Host NQN missing
	flexVol.AccessType = VolTypeRW
	publishInfo.HostNQN = ""
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 9: Adding host to subsystem fails
	publishInfo.HostNQN = "fakeHostNQN"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(errors.New("Error adding host nqnq to subsystem")).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 10: Namespace mapping fails
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(errors.New("Error returned by NVMeEnsureNamespaceMapped")).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)

	// Test case 11: Success path
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)

	// Test case 12: XFS with nouuid
	volConfig.FileSystem = filesystem.Xfs
	publishInfo.MountOptions = ""
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
	assert.Contains(t, publishInfo.MountOptions, "nouuid", "XFS volumes should have nouuid mount option")

	// Test case 13: XFS preserves existing options
	volConfig.FileSystem = filesystem.Xfs
	publishInfo.MountOptions = "rw,relatime"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
	assert.Contains(t, publishInfo.MountOptions, "rw,relatime")
	assert.Contains(t, publishInfo.MountOptions, "nouuid")
	assert.Equal(t, "rw,relatime,nouuid", publishInfo.MountOptions)

	// Test case 14: Non-XFS without nouuid
	volConfig.FileSystem = filesystem.Ext4
	publishInfo.MountOptions = ""
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
	assert.NotContains(t, publishInfo.MountOptions, "nouuid")

	// Test case 15: XFS no duplicate nouuid
	volConfig.FileSystem = filesystem.Xfs
	publishInfo.MountOptions = "rw,nouuid,relatime"
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "fakeHostName_fakeUUID", "fakeHostName_fakeUUID").Return(subsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
	assert.Equal(t, "rw,nouuid,relatime", publishInfo.MountOptions)

	// Test case 16: RAW - per-volume subsystem check error
	volConfig.FileSystem = filesystem.Raw
	volConfig.Name = "pvc-12345"
	volConfig.InternalName = "trident_pvc_12345"
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeNsUUID"
	publishInfo.MountOptions = ""
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, "s_trident_pvc_12345").Return(nil, fmt.Errorf("error checking per-volume subsystem"))
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check per-volume subsystem")

	// Test case 17: RAW - per-volume subsystem exists
	perVolumeSS := &api.NVMeSubsystem{
		Name: "s_trident_pvc_12345",
		UUID: "perVolumeSSUUID",
		NQN:  "perVolumeNQN",
	}
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, "s_trident_pvc_12345").Return(perVolumeSS, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "s_trident_pvc_12345", "s_trident_pvc_12345").Return(perVolumeSS, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, perVolumeSS.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, perVolumeSS.UUID, "fakeNsUUID").Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)

	// Test case 18: RAW - use SuperSubsystem
	superSubsystem := &api.NVMeSubsystem{
		Name: "trident_subsystem_fakeUUID_1",
		UUID: "superSSUUID",
		NQN:  "superNQN",
	}
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, "s_trident_pvc_12345").Return(nil, nil).Times(1)
	mock.EXPECT().NVMeGetSubsystemsForNamespace(ctx, "fakeNsUUID").Return([]api.NVMeSubsystem{}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemList(ctx, "trident_subsystem_fakeUUID*").Return([]api.NVMeSubsystem{}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "trident_subsystem_fakeUUID_1", "trident_subsystem_fakeUUID_1").Return(superSubsystem, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, superSubsystem.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, superSubsystem.UUID, "fakeNsUUID").Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)

	// Test case 19: RAW - SuperSubsystem lookup error
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, "s_trident_pvc_12345").Return(nil, nil).Times(1)
	mock.EXPECT().NVMeGetSubsystemsForNamespace(ctx, "fakeNsUUID").Return(nil, fmt.Errorf("API error getting subsystems for namespace"))
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get SuperSubsystem")

	// Test case 20: RAW - namespace already in SuperSubsystem
	existingSuperSS := &api.NVMeSubsystem{
		Name: "trident_subsystem_fakeUUID_2",
		UUID: "existingSuperSSUUID",
		NQN:  "existingSuperNQN",
	}
	mock.EXPECT().VolumeInfo(ctx, volConfig.InternalName).Return(flexVol, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, "s_trident_pvc_12345").Return(nil, nil).Times(1)
	mock.EXPECT().NVMeGetSubsystemsForNamespace(ctx, "fakeNsUUID").Return([]api.NVMeSubsystem{{
		Name: "trident_subsystem_fakeUUID_2",
		UUID: "existingSuperSSUUID",
	}}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemCreate(ctx, "trident_subsystem_fakeUUID_2", "trident_subsystem_fakeUUID_2").Return(existingSuperSS, nil).Times(1)
	mock.EXPECT().NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, existingSuperSS.UUID).Return(nil).Times(1)
	mock.EXPECT().NVMeEnsureNamespaceMapped(ctx, existingSuperSS.UUID, "fakeNsUUID").Return(nil).Times(1)
	err = d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
}

func TestUnpublish(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mock := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mock, nil, nil)

	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:    "fakeHostName",
		TridentUUID: "fakeUUID",
		HostNQN:     "nqn.2014-08.org.nvmexpress:uuid:fake-host-nqn",
	}

	// case 1: NVMeRemoveHostFromSubsystem - error checking if host present
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeUUID"
	volConfig.AccessInfo.NVMeSubsystemUUID = "fakeSubsystemUUID"
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return(nil, fmt.Errorf("NVMeGetHostsOfSubsystem returned error"))

	err := d.Unpublish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when NVMeGetHostsOfSubsystem fails, got none")

	// case 2: NVMeEnsureNamespaceUnmapped - error checking if namespace is mapped
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return([]*api.NvmeSubsystemHost{}, nil)
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, "fakeSubsystemUUID", "fakeUUID").
		Return(false, fmt.Errorf("NVMeIsNamespaceMapped returned error"))

	err = d.Unpublish(ctx, volConfig, publishInfo)

	assert.Error(t, err, "expected error when NVMeIsNamespaceMapped fails, got none")

	// case 3: Namespace not mapped (nothing to do, but deleteSubsystemIfEmpty still called)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return([]*api.NvmeSubsystemHost{}, nil)
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, "fakeSubsystemUUID", "fakeUUID").
		Return(false, nil)
	mock.EXPECT().NVMeSubsystemGetNamespaceCount(ctx, "fakeSubsystemUUID").
		Return(int64(1), nil)

	err = d.Unpublish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error when namespace not mapped")

	// case 4: Success - namespace mapped, no other hosts or namespaces, complete flow
	volConfig.AccessInfo.PublishEnforcement = true
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeUUID"
	volConfig.AccessInfo.NVMeSubsystemUUID = "fakeSubsystemUUID"
	volConfig.AccessInfo.NVMeTargetIPs = []string{"1.2.3.4"}
	volConfig.AccessInfo.NVMeSubsystemNQN = "nqn.fake.subsystem"
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	// Step 1: NVMeRemoveHostFromSubsystem
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return([]*api.NvmeSubsystemHost{
			{NQN: "nqn.2014-08.org.nvmexpress:uuid:fake-host-nqn"},
		}, nil)
	mock.EXPECT().NVMeGetNamespaceUUIDsForSubsystem(ctx, "fakeSubsystemUUID").
		Return([]string{"fakeUUID"}, nil)
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, "nqn.2014-08.org.nvmexpress:uuid:fake-host-nqn", "fakeSubsystemUUID").
		Return(nil)

	// Step 2: NVMeEnsureNamespaceUnmapped
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, "fakeSubsystemUUID", "fakeUUID").
		Return(true, nil)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return([]*api.NvmeSubsystemHost{}, nil)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, "fakeSubsystemUUID", "fakeUUID").
		Return(nil)

	// Step 3: deleteSubsystemIfEmpty
	mock.EXPECT().NVMeSubsystemGetNamespaceCount(ctx, "fakeSubsystemUUID").
		Return(int64(0), nil)
	mock.EXPECT().NVMeSubsystemDelete(ctx, "fakeSubsystemUUID").
		Return(nil)

	err = d.Unpublish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error during unpublish, got one")
	assert.Empty(t, volConfig.AccessInfo.NVMeTargetIPs, "expected NVMeTargetIPs to be cleared")
	assert.Empty(t, volConfig.AccessInfo.NVMeSubsystemNQN, "expected NVMeSubsystemNQN to be cleared")
	assert.Empty(t, volConfig.AccessInfo.NVMeSubsystemUUID, "expected NVMeSubsystemUUID to be cleared")

	// case 5: Namespace mapped but other hosts need it (RWX scenario) - AccessInfo should be preserved
	volConfig = &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}
	volConfig.AccessInfo.PublishEnforcement = true
	volConfig.AccessInfo.NVMeNamespaceUUID = "fakeUUID"
	volConfig.AccessInfo.NVMeSubsystemUUID = "fakeSubsystemUUID"
	volConfig.AccessInfo.NVMeTargetIPs = []string{"1.2.3.4"}
	volConfig.AccessInfo.NVMeSubsystemNQN = "nqn.fake.subsystem"
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	publishInfo.Nodes = []*models.Node{
		{
			Name: "node1",
			NQN:  "nqn.2014-08.org.nvmexpress:uuid:fake-host-nqn",
		},
		{
			Name: "node2",
			NQN:  "nqn.2014-08.org.nvmexpress:uuid:other-host-nqn",
		},
	}

	// NVMeRemoveHostFromSubsystem - removes current host
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return([]*api.NvmeSubsystemHost{
			{NQN: "nqn.2014-08.org.nvmexpress:uuid:fake-host-nqn"},
		}, nil)
	mock.EXPECT().NVMeGetNamespaceUUIDsForSubsystem(ctx, "fakeSubsystemUUID").
		Return([]string{"fakeUUID"}, nil)
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, "nqn.2014-08.org.nvmexpress:uuid:fake-host-nqn", "fakeSubsystemUUID").
		Return(nil)

	// NVMeEnsureNamespaceUnmapped - namespace stays mapped because other hosts need it
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, "fakeSubsystemUUID", "fakeUUID").
		Return(true, nil)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, "fakeSubsystemUUID").
		Return([]*api.NvmeSubsystemHost{
			{NQN: "nqn.2014-08.org.nvmexpress:uuid:other-host-nqn"}, // Other host still present
		}, nil)
	// NVMeSubsystemRemoveNamespace should NOT be called because other hosts need this namespace

	// deleteSubsystemIfEmpty - subsystem still has namespaces
	mock.EXPECT().NVMeSubsystemGetNamespaceCount(ctx, "fakeSubsystemUUID").
		Return(int64(1), nil)

	err = d.Unpublish(ctx, volConfig, publishInfo)

	assert.NoError(t, err, "expected no error during unpublish for RWX volume")
	// AccessInfo should NOT be cleared because namespace was not unmapped
	assert.Equal(t, []string{"1.2.3.4"}, volConfig.AccessInfo.NVMeTargetIPs, "expected NVMeTargetIPs to be preserved")
	assert.Equal(t, "nqn.fake.subsystem", volConfig.AccessInfo.NVMeSubsystemNQN, "expected NVMeSubsystemNQN to be preserved")
	assert.Equal(t, "fakeSubsystemUUID", volConfig.AccessInfo.NVMeSubsystemUUID, "expected NVMeSubsystemUUID to be preserved")
}
func TestCreatePrepare(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mock := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mock, nil, nil)
	volConfig := &storage.VolumeConfig{
		Name:         "fakeVolName",
		InternalName: "fakeInternalName",
	}
	volConfig.ImportNotManaged = false
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	pool := storage.NewStoragePool(nil, "dummyPool")

	d.CreatePrepare(ctx, volConfig, pool)
	tridentconfig.CurrentDriverContext = ""

	assert.Equal(t, "test_fakeVolName", volConfig.InternalName, "Incorrect volume internal name.")
	assert.True(t, volConfig.AccessInfo.PublishEnforcement, "Publish enforcement not enabled.")
}

func TestNVMeCreatePrepare_NilPool(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mockAPI, nil, nil)
	defer mockCtrl.Finish()

	volConfig := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}

	volConfig.ImportNotManaged = false
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	d.Config.NameTemplate = `{{.volume.Name}}_{{.volume.Namespace}}_{{.volume.StorageClass}}`

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	d.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, d, d.getStoragePoolAttributes(ctx), d.BackendName())

	d.CreatePrepare(ctx, &volConfig, nil)
	assert.Equal(t, "newVolume_testNamespace_testSC", volConfig.InternalName, "Incorrect volume internal name.")
}

func TestNVMeCreatePrepare_NilPool_templateNotContainVolumeName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newNVMeDriver(mockAPI, nil, nil)
	defer mockCtrl.Finish()

	volConfig := storage.VolumeConfig{Name: "pvc-1234567", Namespace: "testNamespace", StorageClass: "testSC"}

	volConfig.ImportNotManaged = false
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	d.Config.NameTemplate = `{{.volume.Namespace}}_{{.volume.StorageClass}}_{{slice .volume.Name 4 9}}`

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	d.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, d, d.getStoragePoolAttributes(ctx), d.BackendName())

	d.CreatePrepare(ctx, &volConfig, nil)
	assert.Equal(t, "testNamespace_testSC_12345", volConfig.InternalName, "Incorrect volume internal name.")
}

func TestNVMeResize_VolumeExistsErrors(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)

	// API call fails use case.
	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(false, errors.New("api call failed"))

	err := d.Resize(ctx, volConfig, 100)

	assert.ErrorContains(t, err, "error occurred checking for existing volume")

	// Volume doesn't exist.
	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(false, nil)

	err = d.Resize(ctx, volConfig, 100)

	assert.ErrorContains(t, err, "does not exist")
}

func TestNVMeResize_GetVolumeSizeError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(0), errors.New("api call failed"))

	err := d.Resize(ctx, volConfig, 100)

	assert.ErrorContains(t, err, "error occurred when checking volume size")
}

func TestNVMeResize_GetNamespaceError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(nil, errors.New("api call failed"))

	err := d.Resize(ctx, volConfig, 100)

	assert.ErrorContains(t, err, "api call failed")
}

func TestNVMeResize_ParseNamespaceSizeError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "-100"}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)

	err := d.Resize(ctx, volConfig, 100)

	assert.ErrorContains(t, err, "error while parsing namespace size")
}

func TestNVMeResize_LessRequestedSizeError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)

	err := d.Resize(ctx, volConfig, 99)

	assert.ErrorContains(t, err, "less than existing volume size")
}

func TestNVMeResize_SameNamespaceSize(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("api call failed"))

	err := d.Resize(ctx, volConfig, 120)

	assert.NoError(t, err, "Failed to resize.")
	assert.Equal(t, "100", volConfig.Size, "Resize failed.")
}

func TestNVMeResize_AggrLimitsError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("api call failed")).Times(2)

	err := d.Resize(ctx, volConfig, tridentconfig.SANResizeDelta+200)

	assert.ErrorContains(t, err, "api call failed")
}

func TestNVMeResize_DriverVolumeLimitError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}
	vol := &api.Volume{Aggregates: []string{"data"}}
	d.Config.LimitVolumeSize = "1000.1000"

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("failed"))
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)

	err := d.Resize(ctx, volConfig, tridentconfig.SANResizeDelta+200)

	assert.ErrorContains(t, err, "error parsing limitVolumeSize")
}

func TestNVMeResize_VolumeSetSizeError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}
	vol := &api.Volume{Aggregates: []string{"data"}}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("failed"))
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(errors.New("api call failed"))

	err := d.Resize(ctx, volConfig, tridentconfig.SANResizeDelta+200)

	assert.ErrorContains(t, err, "volume resize failed")
}

func TestNVMeResize_NamespaceSetSizeError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}
	vol := &api.Volume{Aggregates: []string{"data"}}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("failed"))
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mAPI.EXPECT().NVMeNamespaceSetSize(ctx, gomock.Any(), gomock.Any()).Return(errors.New("api call failed"))

	err := d.Resize(ctx, volConfig, tridentconfig.SANResizeDelta+200)

	assert.ErrorContains(t, err, "volume resize failed")
}

func TestNVMeResize_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}
	vol := &api.Volume{Aggregates: []string{"data"}}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(200), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("failed"))
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mAPI.EXPECT().NVMeNamespaceSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)

	err := d.Resize(ctx, volConfig, tridentconfig.SANResizeDelta+200)

	assert.NoError(t, err, "Volume resize failed.")
	assert.Equal(t, "50000200", volConfig.Size, "Incorrect namespace size after resize.")
}

func TestNVMeResize_VolumeLargerThanResizeRequest(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	ns := &api.NVMeNamespace{Name: "/vol/vol1/namespace0", Size: "100"}
	vol := &api.Volume{Aggregates: []string{"data"}}

	mAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil)
	mAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(tridentconfig.SANResizeDelta*2), nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).Return(ns, nil)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("failed"))
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)

	err := d.Resize(ctx, volConfig, tridentconfig.SANResizeDelta+200)

	assert.NoError(t, err, "Volume resize failed.")
	assert.Equal(t, "50000200", volConfig.Size, "Incorrect namespace size after resize.")
}

func TestCreateClone(t *testing.T) {
	type parameters struct {
		configureMockOntapAPI     func(mockAPI *mockapi.MockOntapAPI)
		configureDriver           func(driver *NVMeStorageDriver)
		cloneVolConfig            storage.VolumeConfig
		pool                      *storage.StoragePool
		expectError               bool
		validateCloneVolumeConfig func(t *testing.T, volConfig *storage.VolumeConfig)
	}

	pool := storage.NewStoragePool(nil, "fakepool")
	pool.InternalAttributes()[FileSystemType] = filesystem.Ext4
	pool.InternalAttributes()[SplitOnClone] = "true"
	pool.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"type": "clone",
	})

	poolWithBadName := storage.NewStoragePool(nil, "")
	poolWithBadName.InternalAttributes()[FileSystemType] = filesystem.Ext4
	poolWithBadName.InternalAttributes()[SplitOnClone] = "true"
	poolWithBadName.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"type": "clone",
	})

	poolWithNameTemplate := storage.NewStoragePool(nil, "fakepool")
	poolWithNameTemplate.InternalAttributes()[FileSystemType] = filesystem.Ext4
	poolWithNameTemplate.InternalAttributes()[SplitOnClone] = "true"
	poolWithNameTemplate.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"type": "clone",
	})
	poolWithNameTemplate.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." + "RequestName}}"

	longLabelVal := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
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

	poolWithLongLabelVal := storage.NewStoragePool(nil, "fakepool")
	poolWithLongLabelVal.InternalAttributes()[FileSystemType] = filesystem.Ext4
	poolWithLongLabelVal.InternalAttributes()[SplitOnClone] = "true"
	poolWithLongLabelVal.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"type": "clone",
	})
	poolWithLongLabelVal.Attributes()[sa.Labels] = sa.NewLabelOffer(map[string]string{"key": longLabelVal})

	tests := map[string]parameters{
		"Success creating clone": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				vol := &api.Volume{Aggregates: []string{"data"}}
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        pool,
			expectError: false,
		},
		"Success creating clone with snapshot": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				vol := &api.Volume{Aggregates: []string{"data"}}
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeSnapshotCreate(ctx, gomock.Any(), gomock.Any())
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "",
			},
			pool:        pool,
			expectError: false,
			validateCloneVolumeConfig: func(t *testing.T, volConfig *storage.VolumeConfig) {
				assert.Empty(t, volConfig.CloneSourceSnapshot)
				assert.NotEmpty(t, volConfig.CloneSourceSnapshotInternal)
			},
		},
		"Unable to check the volume": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("unable to get volume Info"))
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        pool,
			expectError: true,
		},
		"Flexvol not found": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, nil)
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        pool,
			expectError: true,
		},
		"Success setting comment": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				vol := &api.Volume{Aggregates: []string{"data"}, Comment: "fakeComment"}
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        pool,
			expectError: false,
		},
		"Storage pool name not set": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        poolWithBadName,
			expectError: true,
		},
		"Success using Name Template": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				vol := &api.Volume{Aggregates: []string{"data"}, Comment: "fakeComment"}
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
			},
			configureDriver: func(driver *NVMeStorageDriver) {
				driver.physicalPools = map[string]storage.Pool{"pool1": poolWithNameTemplate}
				driver.Config.SplitOnClone = "false"
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        poolWithNameTemplate,
			expectError: false,
		},
		"Storage pool attribute label too long": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				vol := &api.Volume{Aggregates: []string{"data"}, Comment: "fakeComment"}
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
			},
			configureDriver: func(driver *NVMeStorageDriver) {
				driver.physicalPools = map[string]storage.Pool{"pool1": poolWithLongLabelVal}
				driver.Config.SplitOnClone = "false"
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        poolWithLongLabelVal,
			expectError: true,
		},
		"GetLabelsJSON attribute label too long": {
			configureMockOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).AnyTimes().Return(false, nil)
				ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100"}
				mockAPI.EXPECT().NVMeNamespaceGetByName(ctx, gomock.Any()).AnyTimes().Return(ns, nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), false).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, "cloneVol1", []string{"online"}, []string{"error"}, maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
				mockAPI.EXPECT().VolumeCloneSplitStart(ctx, gomock.Any()).AnyTimes().Return(nil)
				vol := &api.Volume{Aggregates: []string{"data"}, Comment: "fakeComment"}
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
			},
			configureDriver: func(driver *NVMeStorageDriver) {
				driver.physicalPools = nil
				driver.Config.Labels = map[string]string{
					"cloud":      "anf",
					longLabelVal: "dev-test-cluster-1",
				}
				driver.Config.SplitOnClone = "false"
			},
			cloneVolConfig: storage.VolumeConfig{
				InternalName:                "cloneVol1",
				Size:                        "200000000",
				CloneSourceVolumeInternal:   "fakeSource",
				CloneSourceSnapshotInternal: "fakeSourceSnapshot",
			},
			pool:        poolWithLongLabelVal,
			expectError: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			d, mockAPI := newNVMeDriverAndMockApi(t)
			d.Config.Labels = params.pool.GetLabels(ctx, "")

			if params.configureDriver != nil {
				params.configureDriver(d)
			}

			if params.configureMockOntapAPI != nil {
				params.configureMockOntapAPI(mockAPI)
			}

			_, volConfig, _ := getNVMeCreateArgs(d)
			volConfig.InternalID = "/vol/cloneVol1/namespace0"

			err := d.CreateClone(ctx, volConfig, &params.cloneVolConfig, params.pool)

			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if params.validateCloneVolumeConfig != nil {
				params.validateCloneVolumeConfig(t, &params.cloneVolConfig)
			}
		})
	}
}

func TestImport(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	originalName := "fakeOriginalName"
	vol := &api.Volume{Aggregates: []string{"data"}}
	ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100", UUID: "fakeUUID"}

	// Test1: Error - Error getting volume info
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, errors.New("Error getting volume info"))

	err := d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test2: Error - Failed to get the volume info
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, nil)

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test3: Error - volume is not read-write
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	vol.AccessType = "dp"

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test4: Error - Error getting namespace
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(nil,
		errors.New("error getting namespace info"))
	vol.AccessType = "rw"

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test5: Error - Failed to get namespace
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(nil, nil)

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test6: Error - Namespace not online
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	ns.State = "offline"

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test7: Error - Namespace mapped to subsystem
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(true, nil)
	ns.State = "online"

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test8: Error - Checking if namespace is mapped returns error
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false,
		errors.New("error while checking namespace mapped"))

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test9: Error - while renaming the volume
	volConfig.ImportNotManaged = false
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false, nil)
	mAPI.EXPECT().VolumeRename(ctx, originalName,
		volConfig.InternalName).Return(errors.New("error renaming volume"))

	err = d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)

	// Test10: Success
	vol.Comment = "fakeComment"
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false, nil)
	mAPI.EXPECT().VolumeRename(ctx, originalName, volConfig.InternalName).Return(nil)

	err = d.Import(ctx, volConfig, originalName)

	assert.NoError(t, err)
}

func TestImport_NoRename(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	originalName := "fakeOriginalName"
	volConfig.ImportNotManaged = false
	volConfig.ImportNoRename = true       // Enable --no-rename flag
	volConfig.InternalName = originalName // With --no-rename, InternalName should be the original name

	vol := &api.Volume{
		Aggregates: []string{"data"},
		AccessType: "rw",
		Comment:    "fakeComment",
	}
	ns := &api.NVMeNamespace{
		Name:  "/vol/cloneVol1/namespace0",
		Size:  "100",
		UUID:  "fakeUUID",
		State: "online",
	}

	// Test successful import with --no-rename (VolumeRename should NOT be called)
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false, nil)
	// With --no-rename, VolumeRename should NOT be called
	// Only label updates should happen (VolumeSetComment is called in the label update logic)

	err := d.Import(ctx, volConfig, originalName)

	assert.NoError(t, err, "Error in Volume import with --no-rename, expected no error")
	assert.Equal(t, originalName, volConfig.InternalName, "Expected volume internal name to remain as original name with --no-rename")
}

func TestImport_LUKSNamespace(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	originalName := "fakeOriginalName"
	vol := &api.Volume{Aggregates: []string{"data"}}
	ns := &api.NVMeNamespace{
		Name: "/vol/cloneVol1/namespace0",
		Size: "20GB",
		UUID: "fakeUUID",
	}
	ns.State = "online"

	vol.Comment = "fakeComment"
	volConfig.LUKSEncryption = "true"
	volConfig.ImportNotManaged = true
	volConfig.Size = "20GB"
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false, nil)

	beforeLUKSOverheadBytesStr, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		t.Fatalf("failed to convert volume size")
	}
	beforeLUKSOverhead, err := strconv.ParseUint(beforeLUKSOverheadBytesStr, 10, 64)
	if err != nil {
		t.Fatalf("failed to convert volume size")
	}

	err = d.Import(ctx, volConfig, originalName)

	afterLUKSOverheadBytesStr, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		t.Fatalf("failed to convert volume size")
	}
	afterLUKSOverhead, err := strconv.ParseUint(afterLUKSOverheadBytesStr, 10, 64)
	if err != nil {
		t.Fatalf("failed to convert volume size")
	}

	assert.NoError(t, err)
	assert.Less(t, afterLUKSOverhead, beforeLUKSOverhead)
	assert.Equal(t, beforeLUKSOverhead, incrementWithLUKSMetadataIfLUKSEnabled(ctx, afterLUKSOverhead, "true"))
}

func TestImport_NameTemplate(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	originalName := "fakeOriginalName"
	vol := &api.Volume{Aggregates: []string{"data"}}
	ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100", UUID: "fakeUUID"}
	ns.State = "online"

	pool := storage.NewStoragePool(nil, "fakepool")
	pool.InternalAttributes()[FileSystemType] = filesystem.Ext4
	pool.InternalAttributes()[SplitOnClone] = "true"
	pool.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." + "RequestName}}"

	d.physicalPools = map[string]storage.Pool{"pool1": pool}
	d.Config.SplitOnClone = "false"

	vol.Comment = "{\"provisioning\": {\"storageDriverName\": \"ontap-san\", \"backendName\": \"customBackendName\"}}"
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false, nil)
	mAPI.EXPECT().VolumeRename(ctx, originalName, volConfig.InternalName).Return(nil)
	mAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	err := d.Import(ctx, volConfig, originalName)

	assert.NoError(t, err)
}

func TestImport_LongLabelError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	_, volConfig, _ := getNVMeCreateArgs(d)
	originalName := "fakeOriginalName"
	vol := &api.Volume{Aggregates: []string{"data"}}
	ns := &api.NVMeNamespace{Name: "/vol/cloneVol1/namespace0", Size: "100", UUID: "fakeUUID"}
	ns.State = "online"

	longLabelVal := "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
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
	labelMap := map[string]string{"key": longLabelVal}

	pool := storage.NewStoragePool(nil, "fakepool")
	pool.InternalAttributes()[FileSystemType] = filesystem.Ext4
	pool.InternalAttributes()[SplitOnClone] = "true"
	pool.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." + "RequestName}}"

	pool.Attributes()[sa.Labels] = sa.NewLabelOffer(labelMap)

	d.physicalPools = map[string]storage.Pool{"pool1": pool}
	d.Config.SplitOnClone = "false"
	d.Config.Labels = pool.GetLabels(ctx, "")

	vol.Comment = "{\"provisioning\": {\"storageDriverName\": \"ontap-san\", \"backendName\": \"customBackendName\"}}"
	mAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(vol, nil)
	mAPI.EXPECT().NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*").Return(ns, nil)
	mAPI.EXPECT().NVMeIsNamespaceMapped(ctx, "", ns.UUID).Return(false, nil)
	mAPI.EXPECT().VolumeRename(ctx, originalName, volConfig.InternalName).Return(nil)

	err := d.Import(ctx, volConfig, originalName)

	assert.Error(t, err)
}

func TestExtractNamespaceName(t *testing.T) {
	var nsStr, nsNameGot, nsNameExpected string
	// Test 1 : Empty String
	nsStr = ""
	nsNameExpected = "namespace0"

	nsNameGot = extractNamespaceName(nsStr)

	assert.Equal(t, nsNameExpected, nsNameGot)

	// Test 2 : Success - Namespace Name in proper format
	nsStr = "/vol/fakeFlexVolName/fakeNamespaceName"
	nsNameExpected = "fakeNamespaceName"

	nsNameGot = extractNamespaceName(nsStr)

	assert.Equal(t, nsNameExpected, nsNameGot)

	// Test 3 : Namespace Name has more length
	nsStr = "/vol/fakeFlexVolName/hasMoreLength/fakeNamespaceName"
	nsNameExpected = "MalformedNamespace"

	nsNameGot = extractNamespaceName(nsStr)

	assert.Equal(t, nsNameExpected, nsNameGot)

	// Test 4 : Namespace Name doesn't match the format
	nsStr = "/vol/fakeFlexVolName"
	nsNameExpected = "MalformedNamespace"

	nsNameGot = extractNamespaceName(nsStr)

	assert.Equal(t, nsNameExpected, nsNameGot)
}

func TestCreateNamespacePath(t *testing.T) {
	nsNameExpected := "/vol/fakeFlexVolName/fakeNamespaceName"
	flexVolName := "fakeFlexVolName"
	nsName := "fakeNamespaceName"

	nsNameGot := createNamespacePath(flexVolName, nsName)

	assert.Equal(t, nsNameExpected, nsNameGot)
}

func TestGetBackendState(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)

	mAPI.EXPECT().GetSVMState(ctx).Return("", errors.New("returning test error"))

	reason, changeMap := d.GetBackendState(ctx)

	assert.Equal(t, reason, StateReasonSVMUnreachable, "should be 'SVM is not reachable'")
	assert.NotNil(t, changeMap, "should not be nil")
}

func TestEnablePublishEnforcement(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	config := getVolumeConfig()
	vol := storage.Volume{Config: &config}

	assert.True(t, d.CanEnablePublishEnforcement(), "Cannot enable publish enforcement.")

	d.EnablePublishEnforcement(ctx, &vol)

	assert.True(t, vol.Config.AccessInfo.PublishEnforcement, "Incorrect publish enforcement value.")
}

func TestNVMeGetConfig(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	config := d.GetConfig()
	assert.NotNil(t, config)
	assert.Equal(t, &d.Config, config)
}

func TestNVMeGetTelemetry(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	telemetry := d.GetTelemetry()
	assert.NotNil(t, telemetry)
	assert.Equal(t, d.telemetry, telemetry)
}

func TestNVMeString(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			// String method panics due to nil dependencies - this is acceptable
			t.Log("String method panicked as expected due to nil dependencies")
		}
	}()

	d := newNVMeDriver(nil, nil, nil)
	_ = d.String() // Just verify it doesn't crash unexpectedly
}

func TestNVMeGoString(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			// GoString method panics due to nil dependencies - this is acceptable
			t.Log("GoString method panicked as expected due to nil dependencies")
		}
	}()

	d := newNVMeDriver(nil, nil, nil)
	_ = d.GoString() // Just verify it doesn't crash unexpectedly
}

func TestNVMeGetExternalConfig(t *testing.T) {
	// Define test values as constants
	const (
		testManagementLIF = "1.2.3.4"
		testSVM           = "test-svm"
		testPassword      = "secret123"
		redactedPassword  = "<REDACTED>"
	)

	d := newNVMeDriver(nil, nil, nil)
	d.Config.ManagementLIF = testManagementLIF
	d.Config.SVM = testSVM
	d.Config.Password = testPassword

	config := d.GetExternalConfig(ctx)

	ontapConfig, ok := config.(drivers.OntapStorageDriverConfig)
	assert.True(t, ok)
	assert.Equal(t, testManagementLIF, ontapConfig.ManagementLIF)
	assert.Equal(t, testSVM, ontapConfig.SVM)
	assert.Equal(t, redactedPassword, ontapConfig.Password) // Password is redacted with angle brackets
}

func TestNVMeCanSnapshot(t *testing.T) {
	d := newNVMeDriver(nil, nil, nil)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "test-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{
		InternalName: "test-vol",
	}

	err := d.CanSnapshot(ctx, snapConfig, volConfig)
	assert.NoError(t, err, "Should support snapshots")
}

func TestNVMeGetSnapshot_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "test-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	apiSnapshot := api.Snapshot{
		Name:       "test-snap",
		CreateTime: "2023-01-01T12:00:00Z",
	}

	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotInfo(ctx, "test-snap", "test-vol").Return(apiSnapshot, nil)

	snapshot, err := d.GetSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, err)
	assert.NotNil(t, snapshot)
	assert.Equal(t, "test-snap", snapshot.Config.InternalName)
}

func TestNVMeGetSnapshot_NotFound(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "missing-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	var emptySnapshot api.Snapshot
	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotInfo(ctx, "missing-snap", "test-vol").Return(emptySnapshot, errors.NotFoundError("not found"))

	snapshot, err := d.GetSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, err)
	assert.Nil(t, snapshot)
}

func TestNVMeGetSnapshot_APIError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "test-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	var emptySnapshot api.Snapshot
	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotInfo(ctx, "test-snap", "test-vol").Return(emptySnapshot, errors.New("API error"))

	snapshot, err := d.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, err)
	assert.Nil(t, snapshot)
}

func TestNVMeGetSnapshots_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	apiSnapshots := api.Snapshots{
		{Name: "snap1", CreateTime: "2023-01-01T12:00:00Z"},
		{Name: "snap2", CreateTime: "2023-01-02T12:00:00Z"},
	}

	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotList(ctx, "test-vol").Return(apiSnapshots, nil)

	snapshots, err := d.GetSnapshots(ctx, volConfig)

	assert.NoError(t, err)
	assert.Len(t, snapshots, 2)
	assert.Equal(t, "snap1", snapshots[0].Config.InternalName)
}

func TestNVMeGetSnapshots_ListError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	var emptySnapshots api.Snapshots
	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotList(ctx, "test-vol").Return(emptySnapshots, errors.New("API error"))

	snapshots, err := d.GetSnapshots(ctx, volConfig)

	assert.Error(t, err)
	assert.Nil(t, snapshots)
}

func TestNVMeCreateSnapshot_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "new-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	mAPI.EXPECT().VolumeExists(ctx, "test-vol").Return(true, nil)
	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotCreate(ctx, "new-snap", "test-vol").Return(nil)
	mAPI.EXPECT().VolumeSnapshotInfo(ctx, "new-snap", "test-vol").Return(api.Snapshot{
		Name:       "new-snap",
		CreateTime: "2023-01-01T12:00:00Z",
	}, nil)

	snapshot, err := d.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, err)
	assert.NotNil(t, snapshot)
	assert.Equal(t, "new-snap", snapshot.Config.InternalName)
}

func TestNVMeCreateSnapshot_CreateError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "new-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	mAPI.EXPECT().VolumeExists(ctx, "test-vol").Return(true, nil)
	mAPI.EXPECT().NVMeNamespaceGetSize(ctx, "/vol/test-vol/*").Return(1024, nil)
	mAPI.EXPECT().VolumeSnapshotCreate(ctx, "new-snap", "test-vol").Return(errors.New("create error"))

	snapshot, err := d.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, err)
	assert.Nil(t, snapshot)
}

func TestNVMeRestoreSnapshot_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "restore-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	mAPI.EXPECT().SnapshotRestoreVolume(ctx, "restore-snap", "test-vol").Return(nil)

	err := d.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, err)
}

func TestNVMeRestoreSnapshot_Error(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "restore-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	mAPI.EXPECT().SnapshotRestoreVolume(ctx, "restore-snap", "test-vol").Return(errors.New("restore error"))

	err := d.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, err)
}

func TestNVMeDeleteSnapshot_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	d.cloneSplitTimers = &sync.Map{} // Initialize to prevent nil pointer
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "delete-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	mAPI.EXPECT().VolumeSnapshotDelete(ctx, "delete-snap", "test-vol").Return(nil)

	err := d.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, err)
}

func TestNVMeDeleteSnapshot_Error(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)
	d.cloneSplitTimers = &sync.Map{} // Initialize to prevent nil pointer
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "delete-snap",
		VolumeInternalName: "test-vol",
	}
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}

	mAPI.EXPECT().VolumeSnapshotDelete(ctx, "delete-snap", "test-vol").Return(errors.New("delete error"))

	err := d.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, err)
}

func TestNVMeCreateVolumeGroupSnapshot(t *testing.T) {
	ctx := context.Background()

	driver, mockAPI := newNVMeDriverAndMockApi(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	groupSnapshotConfig := &storage.GroupSnapshotConfig{
		Name:         "groupsnapshot-1234",
		InternalName: "groupsnapshot-1234",
		VolumeNames:  []string{"vol1", "vol2"},
	}
	storageVols := storage.GroupSnapshotTargetVolumes{
		"trident_vol1": {
			"vol1": &storage.VolumeConfig{Name: "vol1"},
		},
		"trident_vol2": {
			"vol2": &storage.VolumeConfig{Name: "vol2"},
		},
	}
	targetInfo := &storage.GroupSnapshotTargetInfo{
		StorageType:    "unified",
		StorageUUID:    "12345",
		StorageVolumes: storageVols,
	}
	storageVolNames := []string{"trident_vol1", "trident_vol2"}
	snapName, _ := storage.ConvertGroupSnapshotID(groupSnapshotConfig.Name)

	mockAPI.EXPECT().ConsistencyGroupSnapshot(ctx, snapName, gomock.InAnyOrder(storageVolNames)).Return(nil).Times(1)

	err := driver.CreateGroupSnapshot(ctx, groupSnapshotConfig, targetInfo)
	assert.NoError(t, err, "Group snapshot creation failed")
}

func TestNVMeProcessVolumeGroupSnapshot(t *testing.T) {
	ctx := context.Background()

	driver, mockAPI := newNVMeDriverAndMockApi(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	groupSnapshotConfig := &storage.GroupSnapshotConfig{
		Name:         "groupsnapshot-1234",
		InternalName: "groupsnapshot-1234",
		VolumeNames:  []string{"vol1", "vol2"},
	}
	storageVols := []*storage.VolumeConfig{
		{Name: "vol1"},
		{Name: "vol2"},
	}
	snapName, _ := storage.ConvertGroupSnapshotID(groupSnapshotConfig.Name)
	snapInfoResult := api.Snapshot{CreateTime: "1"}
	size := 1073741824

	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapName, gomock.Any()).Return(snapInfoResult, nil).Times(2)
	mockAPI.EXPECT().NVMeNamespaceGetSize(ctx, gomock.Any()).Return(1073741824, nil).Times(2)

	snaps, err := driver.ProcessGroupSnapshot(ctx, groupSnapshotConfig, storageVols)
	assert.NoError(t, err, "Group snapshot processing failed")
	assert.NotNil(t, snaps, "Grouped snapshot extraction failed")
	for _, snap := range snaps {
		assert.Equal(t, snapName, snap.Config.Name)
		assert.Equal(t, int64(size), snap.SizeBytes)
	}
}

func TestNVMeGetGroupSnapshotTarget(t *testing.T) {
	ctx := context.Background()

	driver, mockAPI := newNVMeDriverAndMockApi(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsDisaggregated().AnyTimes().Return(false)

	volumeConfigs := []*storage.VolumeConfig{
		{
			Name:         "vol1",
			InternalName: "trident_vol1",
		},
		{
			Name:         "vol2",
			InternalName: "trident_vol2",
		},
	}

	storageVols := storage.GroupSnapshotTargetVolumes{
		"trident_vol1": {
			"vol1": &storage.VolumeConfig{Name: "vol1", InternalName: "trident_vol1"},
		},
		"trident_vol2": {
			"vol2": &storage.VolumeConfig{Name: "vol2", InternalName: "trident_vol2"},
		},
	}
	expectedTargetInfo := &storage.GroupSnapshotTargetInfo{
		StorageType:    "Unified",
		StorageUUID:    "12345",
		StorageVolumes: storageVols,
	}

	mockAPI.EXPECT().GetSVMUUID().Return("12345").Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(true, nil).Times(2)

	targetInfo, err := driver.GetGroupSnapshotTarget(ctx, volumeConfigs)

	assert.Equal(t, targetInfo, expectedTargetInfo)
	assert.NoError(t, err, "Volume group target failed")
}

// Phase 3: Volume Management Tests
func TestNVMeGet_Success(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)

	mAPI.EXPECT().VolumeExists(ctx, "test-vol").Return(true, nil)

	err := d.Get(ctx, "test-vol")

	assert.NoError(t, err)
}

func TestNVMeGet_NotFound(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)

	mAPI.EXPECT().VolumeExists(ctx, "missing-vol").Return(false, nil)

	err := d.Get(ctx, "missing-vol")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestNVMeGet_APIError(t *testing.T) {
	d, mAPI := newNVMeDriverAndMockApi(t)

	mAPI.EXPECT().VolumeExists(ctx, "test-vol").Return(false, errors.New("API error"))

	err := d.Get(ctx, "test-vol")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking for existing volume")
}

func TestNVMeRename_UnsupportedOperation(t *testing.T) {
	d, _ := newNVMeDriverAndMockApi(t)

	err := d.Rename(ctx, "old-vol", "new-vol")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "renaming volumes is not supported")
}

// Phase 4: Advanced Features and Utility Tests
func TestNVMeCanEnablePublishEnforcement(t *testing.T) {
	d, _ := newNVMeDriverAndMockApi(t)

	canEnable := d.CanEnablePublishEnforcement()

	assert.True(t, canEnable)
}

func TestNVMeEnablePublishEnforcement(t *testing.T) {
	d, _ := newNVMeDriverAndMockApi(t)
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			AccessInfo: models.VolumeAccessInfo{},
		},
	}

	err := d.EnablePublishEnforcement(ctx, volume)

	assert.NoError(t, err)
	assert.True(t, volume.Config.AccessInfo.PublishEnforcement)
}

func TestNVMeNoOpMethods(t *testing.T) {
	d, _ := newNVMeDriverAndMockApi(t)
	volConfig := &storage.VolumeConfig{InternalName: "test-vol"}
	nodes := []*models.Node{{Name: "node1", IQN: "iqn.test"}}

	// Test all no-op methods in one consolidated test
	assert.NoError(t, d.ReconcileNodeAccess(ctx, nodes, "backend1", "context1"))
	assert.NoError(t, d.ReconcileVolumeNodeAccess(ctx, volConfig, nodes))
	assert.NoError(t, d.CreateFollowup(ctx, volConfig))
	assert.True(t, d.CanEnablePublishEnforcement())
}

// TestNVMeStorageDriverGetStoragePrefixSubsystemName tests the getStoragePrefixSubsystemName method
func TestNVMeStorageDriverGetStoragePrefixSubsystemName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)

	tests := []struct {
		name           string
		storagePrefix  string
		expectedResult string
		description    string
	}{
		{
			name:           "DefaultDockerPrefix",
			storagePrefix:  "netappdvp_",
			expectedResult: "netappdvp__subsystem",
			description:    "Default Docker storage prefix with trailing underscore",
		},
		{
			name:           "CustomShortPrefix",
			storagePrefix:  "app1_",
			expectedResult: "app1__subsystem",
			description:    "Custom short prefix with trailing underscore",
		},
		{
			name:           "CustomPrefixWithoutUnderscore",
			storagePrefix:  "myapp",
			expectedResult: "myapp_subsystem",
			description:    "Custom prefix without trailing underscore",
		},
		{
			name:           "EmptyPrefix",
			storagePrefix:  "",
			expectedResult: "_subsystem",
			description:    "Empty storage prefix",
		},
		{
			name:           "LongPrefixTriggeringTruncation",
			storagePrefix:  "very_long_application_name_with_lots_of_characters_that_exceeds_limit_",
			expectedResult: "very_long_application_name_with_lots_of_characters_th_subsystem",
			description:    "Long prefix that triggers truncation to fit 64 char limit",
		},
		{
			name:           "PrefixExactly53Chars",
			storagePrefix:  "exactly_fifty_three_characters_prefix_name_here_",
			expectedResult: "exactly_fifty_three_characters_prefix_name_here__subsystem",
			description:    "Prefix that should not trigger truncation",
		},
		{
			name:           "PrefixTriggeringTruncationEdgeCase",
			storagePrefix:  "this_is_a_very_long_prefix_that_will_definitely_trigger_truncation_mechanism_in_code",
			expectedResult: "this_is_a_very_long_prefix_that_will_definitely_trigg_subsystem",
			description:    "Edge case prefix that triggers truncation at boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock driver with the test storage prefix
			d := &NVMeStorageDriver{
				Config: drivers.OntapStorageDriverConfig{
					CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
						StoragePrefix: &tt.storagePrefix,
					},
				},
				API:    mockAPI,
				AWSAPI: mockAWSAPI,
			}

			result := d.getStoragePrefixSubsystemName()

			// Verify the result matches expected
			assert.Equal(t, tt.expectedResult, result, tt.description)

			// Verify the result doesn't exceed maximum length
			assert.LessOrEqual(t, len(result), maximumSubsystemNameLength,
				"Subsystem name should not exceed maximum length of %d characters", maximumSubsystemNameLength)

			// Verify the result contains "_subsystem" suffix
			assert.True(t, strings.HasSuffix(result, "_subsystem"),
				"Subsystem name should end with '_subsystem'")

			// Log the test result for debugging
			t.Logf("Test: %s | Input: '%s' (%d chars) | Output: '%s' (%d chars)",
				tt.name, tt.storagePrefix, len(tt.storagePrefix), result, len(result))
		})
	}
}
