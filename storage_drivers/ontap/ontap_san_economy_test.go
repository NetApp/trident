// Copyright 2023 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"

	roaring "github.com/RoaringBitmap/roaring/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils"
	utilserrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

var failed = errors.New("failed")

func NewTestLUNHelper(storagePrefix string, driverContext tridentconfig.DriverContext) *LUNHelper {
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "iscsi_vs",
	    "aggregate":         "aggr1",
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "%v",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}
	`, storagePrefix)
	// parse commonConfigJSON into a CommonStorageDriverConfig object
	commonConfig, err := drivers.ValidateCommonSettings(context.Background(), commonConfigJSON)
	if err != nil {
		Log().Errorf("could not decode JSON configuration: %v", err)
		return nil
	}
	config := &drivers.OntapStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig
	helper := NewLUNHelper(*config, driverContext)
	return helper
}

func TestSnapshotNames_DockerContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	snapName1 := helper.getInternalSnapshotName("snapshot-123")
	assert.Equal(t, "_snapshot_snapshot_123", snapName1, "Strings not equal")

	snapName2 := helper.getInternalSnapshotName("snapshot")
	assert.Equal(t, "_snapshot_snapshot", snapName2, "Strings not equal")

	snapName3 := helper.getInternalSnapshotName("_snapshot")
	assert.Equal(t, "_snapshot__snapshot", snapName3, "Strings not equal")

	snapName4 := helper.getInternalSnapshotName("_____snapshot")
	assert.Equal(t, "_snapshot______snapshot", snapName4, "Strings not equal")

	k8sSnapName1 := helper.getInternalSnapshotName("snapshot-0bf1ec69_da4b_11e9_bd10_000c29e763d8")
	assert.Equal(t, "_snapshot_snapshot_0bf1ec69_da4b_11e9_bd10_000c29e763d8", k8sSnapName1, "Strings not equal")
}

func TestSnapshotNames_KubernetesContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	k8sSnapName1 := helper.getInternalSnapshotName("snapshot-0bf1ec69_da4b_11e9_bd10_000c29e763d8")
	assert.Equal(t, "_snapshot_snapshot_0bf1ec69_da4b_11e9_bd10_000c29e763d8", k8sSnapName1, "Strings not equal")

	k8sSnapName2 := helper.getInternalSnapshotName("mySnap")
	assert.Equal(t, "_snapshot_mySnap", k8sSnapName2, "Strings not equal")
}

func TestHelperGetters(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	snapPathPatternForVolume := helper.GetSnapPathPatternForVolume("my-Vol")
	assert.Equal(t, "/vol/*/*my_Vol_snapshot_*", snapPathPatternForVolume, "Strings not equal")

	snapPath := helper.GetSnapPath("my-Bucket", "storagePrefix_my-Lun", "snap-1")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_my_Lun_snapshot_snap_1", snapPath, "Strings not equal")

	snapName1 := helper.GetSnapshotName("storagePrefix_my-Lun", "my-Snapshot")
	assert.Equal(t, "storagePrefix_my_Lun_snapshot_my_Snapshot", snapName1, "Strings not equal")

	snapName2 := helper.GetSnapshotName("my-Lun", "snapshot-123")
	assert.Equal(t, "my_Lun_snapshot_snapshot_123", snapName2, "Strings not equal")

	internalVolName := helper.GetInternalVolumeName("my-Lun")
	assert.Equal(t, "my_Lun", internalVolName, "Strings not equal")

	lunPath := helper.GetLUNPath("my-Bucket", "my-Lun")
	assert.Equal(t, "/vol/my_Bucket/my_Lun", lunPath, "Strings not equal")

	lunName := helper.GetInternalVolumeNameFromPath(lunPath)
	assert.Equal(t, "my_Lun", lunName, "Strings not equal")

	lunPathPatternForVolume := helper.GetLUNPathPattern("my-Vol")
	assert.Equal(t, "/vol/*/storagePrefix_my_Vol", lunPathPatternForVolume, "Strings not equal")
}

func TestValidateLUN(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	isValid := helper.IsValidSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.True(t, isValid, "boolean not true")
}

func TestGetComponents_DockerContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "mysnap", snapName, "Strings not equal")

	volName := helper.GetExternalVolumeNameFromPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")
}

func TestGetComponents_KubernetesContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_snapshot_123")
	assert.Equal(t, "snapshot_123", snapName, "Strings not equal")

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "mysnap", snapName2, "Strings not equal")

	volName := helper.GetExternalVolumeNameFromPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")
}

func TestGetComponentsNoSnapshot(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "", snapName, "Strings not equal")

	volName := helper.GetExternalVolumeNameFromPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "", snapName2, "Strings not equal")

	volName2 := helper.GetExternalVolumeNameFromPath("myBucket/storagePrefix_myLun")
	assert.NotEqual(t, "myLun", volName2, "Strings are equal")
	assert.Equal(t, "", volName2, "Strings are NOT equal")
}

func newTestOntapSanEcoDriver(t *testing.T, vserverAdminHost, vserverAdminPort, vserverAggrName string, useREST bool,
	fsxId *string, apiOverride api.OntapAPI,
) *SANEconomyStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true

	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-san-economy-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san-economy"
	config.StoragePrefix = sp("test_")
	config.UseREST = &useREST

	if fsxId != nil {
		config.AWSConfig = &drivers.AWSConfig{}
		config.AWSConfig.FSxFilesystemID = *fsxId
	}

	iscsiClient, err := iscsi.New(utils.NewOSClient(), utils.NewDevicesClient())
	assert.NoError(t, err)

	sanEcoDriver := &SANEconomyStorageDriver{
		iscsi: iscsiClient,
	}
	sanEcoDriver.Config = *config

	numRecords := api.DefaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		numRecords = api.MaxZapiRecords
	}

	var ontapAPI api.OntapAPI

	if apiOverride != nil {
		ontapAPI = apiOverride
	} else {
		if *config.UseREST {
			ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
		} else {
			ontapAPI, _ = api.NewZAPIClientFromOntapConfig(context.TODO(), config, numRecords)
		}
	}

	sanEcoDriver.API = ontapAPI
	sanEcoDriver.telemetry = &Telemetry{
		Plugin:        sanEcoDriver.Name(),
		SVM:           "SVM1",
		StoragePrefix: *sanEcoDriver.Config.StoragePrefix,
		Driver:        sanEcoDriver,
	}

	return sanEcoDriver
}

func newMockAWSOntapSanEcoDriver(t *testing.T) (*mockapi.MockOntapAPI, *mockapi.MockAWSAPI, *SANEconomyStorageDriver) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	fsxId := FSX_ID

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)

	driver := newTestOntapSanEcoDriver(t, vserverAdminHost, vserverAdminPort, vserverAggrName, false, &fsxId, mockAPI)
	driver.AWSAPI = mockAWSAPI
	return mockAPI, mockAWSAPI, driver
}

func newMockOntapSanEcoDriver(t *testing.T) (*mockapi.MockOntapAPI, *SANEconomyStorageDriver) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	driver := newTestOntapSanEcoDriver(t, vserverAdminHost, vserverAdminPort, vserverAggrName, false, nil, mockAPI)
	return mockAPI, driver
}

func TestOntapSanEcoStorageDriverConfigString(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	sanEcoDrivers := []SANEconomyStorageDriver{
		*newTestOntapSanEcoDriver(t, vserverAdminHost, vserverAdminPort, vserverAggrName, false, nil, mockAPI),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-san-economy-user",
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

	for _, sanEcoDriver := range sanEcoDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, sanEcoDriver.String(), val,
				"ontap-san-economy driver does not contain %v", key)
			assert.Contains(t, sanEcoDriver.GoString(), val,
				"ontap-san-economy driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, sanEcoDriver.String(), val,
				"ontap-san-economy driver contains %v", key)
			assert.NotContains(t, sanEcoDriver.GoString(), val,
				"ontap-san-economy driver contains %v", key)
		}
	}
}

func TestOntapSanEconomyTerminate(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	server := api.NewFakeUnstartedVserver(ctx, vserverAdminHost, vserverAggrName)
	server.StartTLS()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			t.Error("Panic in fake filer", r)
		}
	}()

	cases := [][]struct {
		igroupName         string
		igroupExistingIQNs []string
	}{
		// 2 different cluster backends - remove backend
		{
			{
				igroupName:         "igroup1",
				igroupExistingIQNs: []string{"IQN1", "IQN2"},
			},
			{
				igroupName:         "igroup2",
				igroupExistingIQNs: []string{"IQN3", "IQN4"},
			},
		},
		{
			{
				igroupName:         "igroup1",
				igroupExistingIQNs: []string{},
			},
		},
	}

	for _, testCase := range cases {

		api.FakeIgroups = map[string]map[string]struct{}{}

		var ontapSanDrivers []SANEconomyStorageDriver

		for _, driverInfo := range testCase {

			// simulate existing IQNs on the vserver
			igroupsIQNMap := map[string]struct{}{}
			for _, iqn := range driverInfo.igroupExistingIQNs {
				igroupsIQNMap[iqn] = struct{}{}
			}

			api.FakeIgroups[driverInfo.igroupName] = igroupsIQNMap

			sanEcoStorageDriver := newTestOntapSanEcoDriver(t, vserverAdminHost, port, vserverAggrName, false, nil, nil)
			sanEcoStorageDriver.Config.IgroupName = driverInfo.igroupName
			sanEcoStorageDriver.telemetry = nil
			ontapSanDrivers = append(ontapSanDrivers, *sanEcoStorageDriver)
		}

		for driverIndex, driverInfo := range testCase {
			ontapSanDrivers[driverIndex].Terminate(ctx, "")
			assert.NotContains(t, api.FakeIgroups, api.FakeIgroups[driverInfo.igroupName])
		}

	}
}

func TestOntapSanEconomyTerminate_Failed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.Config.DriverContext = tridentconfig.ContextCSI
	d.telemetry = &Telemetry{
		done: make(chan struct{}),
	}

	mockAPI.EXPECT().IgroupDestroy(ctx, gomock.Any()).Times(1).Return(fmt.Errorf("failed to destroy igroup"))

	d.Terminate(ctx, "")

	assert.False(t, d.initialized)
}

func TestEconomyGetChapInfo(t *testing.T) {
	type fields struct {
		initialized   bool
		Config        drivers.OntapStorageDriverConfig
		ips           []string
		physicalPools map[string]storage.Pool
		virtualPools  map[string]storage.Pool
	}
	type args struct {
		in0 context.Context
		in1 string
		in2 string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *models.IscsiChapInfo
	}{
		{
			name: "driverInitialized",
			fields: fields{
				initialized: true,
				Config: drivers.OntapStorageDriverConfig{
					UseCHAP:                   true,
					ChapUsername:              "foo",
					ChapInitiatorSecret:       "bar",
					ChapTargetUsername:        "baz",
					ChapTargetInitiatorSecret: "biz",
				},
				ips:           nil,
				physicalPools: nil,
				virtualPools:  nil,
			},
			args: args{
				in0: nil,
				in1: "volume",
				in2: "node",
			},
			want: &models.IscsiChapInfo{
				UseCHAP:              true,
				IscsiUsername:        "foo",
				IscsiInitiatorSecret: "bar",
				IscsiTargetUsername:  "baz",
				IscsiTargetSecret:    "biz",
			},
		},
		{
			name: "driverUninitialized",
			fields: fields{
				initialized: false,
				Config: drivers.OntapStorageDriverConfig{
					UseCHAP:                   true,
					ChapUsername:              "biz",
					ChapInitiatorSecret:       "baz",
					ChapTargetUsername:        "bar",
					ChapTargetInitiatorSecret: "foo",
				},
				ips:           nil,
				physicalPools: nil,
				virtualPools:  nil,
			},
			args: args{
				in0: nil,
				in1: "volume",
				in2: "node",
			},
			want: &models.IscsiChapInfo{
				UseCHAP:              true,
				IscsiUsername:        "biz",
				IscsiInitiatorSecret: "baz",
				IscsiTargetUsername:  "bar",
				IscsiTargetSecret:    "foo",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &SANEconomyStorageDriver{
				initialized:   tt.fields.initialized,
				Config:        tt.fields.Config,
				ips:           tt.fields.ips,
				physicalPools: tt.fields.physicalPools,
				virtualPools:  tt.fields.virtualPools,
			}
			got, err := d.GetChapInfo(tt.args.in0, tt.args.in1, tt.args.in2)
			if err != nil {
				t.Errorf("GetChapInfo(%v, %v, %v)", tt.args.in0, tt.args.in1, tt.args.in2)
			}
			assert.Equalf(t, tt.want, got, "GetChapInfo(%v, %v, %v)", tt.args.in0, tt.args.in1, tt.args.in2)
		})
	}
}

func TestGetLUNPathEconomy(t *testing.T) {
	lunPathEco := GetLUNPathEconomy("ndvp_lun_pool_ciqptQLQdpAeKPep1_QRFXEBJHCT", "ciqptQLQdpAeKPep1_cFfRTWQibX")
	assert.Equal(t, "/vol/ndvp_lun_pool_ciqptQLQdpAeKPep1_QRFXEBJHCT/ciqptQLQdpAeKPep1_cFfRTWQibX", lunPathEco,
		"Incorrect lun economy path")
}

func TestGetInternalVolumeNamenoPrefix(t *testing.T) {
	helper := NewTestLUNHelper("", tridentconfig.ContextDocker)
	internalVolName := helper.GetInternalVolumeName("my-Lun")
	assert.Equal(t, "my_Lun", internalVolName, "Incorrect lun economy path")
}

func TestGetSnapshotNameFromSnapLUNPathBlank(t *testing.T) {
	helper := NewTestLUNHelper("", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("")
	assert.Equal(t, "", snapName, "Incorrect snapshot lun name")
}

func TestGetBucketName(t *testing.T) {
	helper := NewTestLUNHelper("", tridentconfig.ContextDocker)
	snapName := helper.GetBucketName("")
	assert.Equal(t, "", snapName, "Incorrect bucket name")
}

func TestGetAPI(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	result := d.GetAPI()

	assert.True(t, reflect.DeepEqual(result, d.API), "Incorrect API returned")
}

func TestGetTelemetry(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	d.telemetry = &Telemetry{
		Plugin:        d.Name(),
		SVM:           d.Config.SVM,
		StoragePrefix: *d.Config.StoragePrefix,
		Driver:        d,
		done:          make(chan struct{}),
	}
	assert.True(t, reflect.DeepEqual(d.telemetry, d.GetTelemetry()), "Incorrect API returned")
}

func TestBackendNameUnset(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	d.Config.BackendName = ""
	d.ips = []string{"127.0.0.1"}
	assert.Equal(t, d.BackendName(), "ontapsaneco_127.0.0.1", "Incorrect bucket name")
}

func TestBackendNameSet(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	d.Config.BackendName = "ontapsaneco"
	d.ips = []string{"127.0.0.1"}

	assert.Equal(t, d.BackendName(), "ontapsaneco", "Incorrect backend name")
}

func TestDriverInitialized(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	d.initialized = true
	assert.Equal(t, d.Initialized(), d.initialized, "Incorrect initialization status")
}

func TestOntapSanEconomyTerminateCSI(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.telemetry = nil
	d.Config.DriverContext = tridentconfig.ContextCSI

	mockAPI.EXPECT().IgroupDestroy(ctx, gomock.Any()).Times(1).Return(nil)

	d.Terminate(ctx, "")
	assert.False(t, d.initialized)
}

func TestDriverValidate(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	result := d.validate(ctx)
	assert.NoError(t, result, "San Economy driver validation failed")
}

func TestDriverIgnoresDataLIF(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	d.Config.DataLIF = "foo"

	assert.NoError(t, d.validate(ctx), "driver validation succeeded: data LIF is ignored")
}

func TestDriverValidateInvalidPrefix(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	d.Config.StoragePrefix = utils.Ptr("B@D")

	assert.EqualError(t, d.validate(ctx), "storage prefix may only contain letters/digits/underscore/dash")
}

func TestDriverValidateInvalidPools(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[SpaceReserve] = "iaminvalid"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	result := d.validate(ctx)
	assert.EqualError(t, result, "storage pool validation failed: invalid spaceReserve iaminvalid in pool pool1")
}

func TestOntapSanEconomyVolumeCreate(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "none",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    "false",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	d.Config.LimitVolumeSize = "2g"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "fake-export-policy", volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
	assert.Equal(t, "false", volConfig.LUKSEncryption)
	assert.Equal(t, "xfs", volConfig.FileSystem)
}

func TestOntapSANEconomyInitializeStoragePools_NameTemplatesAndLabels(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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

	// mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().GetSVMAggregateNames(gomock.Any()).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	volume := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}
	templateData := make(map[string]interface{})
	templateData["volume"] = volume

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
			"test_newVolume",
			"test_newVolume",
			"san-eco-backend",
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
			"san-eco-backend",
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
			"san-eco-backend",
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
			"san-eco-backend",
			"Base label is not set correctly",
			"Virtual pool label is not set correctly",
			"volume name is not set correctly",
			"volume name is not set correctly",
		}, // base and virtual labels
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
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

			virtualPool := virtualPools["san-eco-backend_pool_0"]
			label, err = virtualPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, test.virtualExpected, label, test.virtualErrorMessage)

			d.CreatePrepare(ctx, &volume, virtualPool)
			assert.Equal(t, volume.InternalName, test.volumeNameVirtualExpected, test.virtualVolNameErrorMessage)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_LUNExists(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}

	tests := []struct {
		message   string
		lunExists bool
	}{
		{"error fetching info", false},
		{"lun already exists", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.lunExists {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			}

			result := d.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_NoPhysicalPool(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.virtualPools = map[string]storage.Pool{}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_InvalidSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		volumeSize string
	}{
		{"invalid"},
		{"-1002947563b"},
	}
	for _, test := range tests {
		t.Run(test.volumeSize, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				Size:       test.volumeSize,
				Encryption: "false",
				FileSystem: "xfs",
			}

			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

			result := d.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_OverSizeLimit(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		limitVolumeSize string
	}{
		{"invalid"},
		{"1GiB"},
	}
	for _, test := range tests {
		t.Run(test.limitVolumeSize, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				Size:       "2GiB",
				Encryption: "false",
				FileSystem: "xfs",
			}
			d.Config.LimitVolumeSize = test.limitVolumeSize

			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

			result := d.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_OverPoolSizeLimit_CreateNewFlexvol(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "none",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    "false",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	existingLUN := api.Lun{Size: "1073741824"}
	existingFlexvol := &api.Volume{
		Size: "1g",
		Name: "flexvol1",
	}
	newFlexvol := &api.Volume{
		Size: "1g",
		Name: "flexvol2",
	}
	volConfig := &storage.VolumeConfig{
		Size:       "2g",
		Encryption: "false",
		FileSystem: "xfs",
	}

	volAttrs := map[string]sa.Request{}
	d.Config.LimitVolumePoolSize = "2g"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return([]*api.Volume{existingFlexvol}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(existingFlexvol, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{existingLUN}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(newFlexvol, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{existingLUN}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "fake-export-policy", volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
	assert.Equal(t, "false", volConfig.LUKSEncryption)
	assert.Equal(t, "xfs", volConfig.FileSystem)
}

func TestOntapSanEconomyVolumeCreate_NotOverPoolSizeLimit_UseExistingFlexvol(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "none",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    "false",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	d.lunsPerFlexvol = 100
	existingLUN := api.Lun{
		Size: "1073741824",
		Name: "lun1",
	}
	existingFlexvol := &api.Volume{
		Size: "1g",
		Name: "flexvol1",
	}
	newFlexvol := &api.Volume{
		Size: "1g",
		Name: "flexvol2",
	}
	volConfig := &storage.VolumeConfig{
		Size:       "2g",
		Encryption: "false",
		FileSystem: "xfs",
	}

	volAttrs := map[string]sa.Request{}
	d.Config.LimitVolumePoolSize = "5g"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return([]*api.Volume{existingFlexvol}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(existingFlexvol, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{existingLUN}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(newFlexvol, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{existingLUN}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{existingLUN}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "fake-export-policy", volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
	assert.Equal(t, "false", volConfig.LUKSEncryption)
	assert.Equal(t, "xfs", volConfig.FileSystem)
}

func TestOntapSanEconomyVolumeCreate_LUNNameLimitExceeding(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
		InternalName: "thisIsATestLabelWhoseLengthShouldExceed1023Characters_AddingSomeRandomCharacters_" +
			"V88bESTQlRIWRSS40sx9ND8P9yPf0LV8jPofiqtTp2iIXgotGh83zZ1HEeFlMGxZlIcOiPdoi07cJ" +
			"bQBuHvTRNX6pHRKUXaIrjEpygM4SpaqHYdZ8O1k2meeugg7eXu4dPhqetI3Sip3W4v9QuFkh1YBaI" +
			"9sHE9w5eRxpmTv0POpCB5xAqzmN6XCkxuXKc4yfNS9PRwcTSpvkA3PcKCF3TD1TJU3NYzcChsFQgm" +
			"bAsR32cbJRdsOwx6BkHNfRCji0xSnBFUFUu1sGHfYCmzzd3OmChADIP6RwRtpnqNzvt0CU6uumBnl",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.ErrorContains(t, result, "name exceeds the limit of 254 characters")
}

func TestOntapSanEconomyVolumeCreate_InvalidSnapshotReserve(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
		SnapshotReserve: "invalid-value",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_InvalidEncryptionValue(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "invalid-value", // invalid bool value
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_InvalidFilesystem(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "nfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_BothQosPolicies(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		TieringPolicy:     "",
		QosPolicy:         "fake",
		AdaptiveQosPolicy: "fake",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
	mockAPI.EXPECT().TieringPolicyValue(ctx).Return("none")

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_NoAggregates(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.Config.LimitAggregateUsage = "invalid"
	pool1 := storage.NewStoragePool(nil, "")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_NoFlexvol(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{}, fmt.Errorf("failed to list volumes"))

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_ResizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		message      string
		destroyError bool
	}{
		{"nil", false},
		{"failed to delete volume", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil).Times(2)
			mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
			mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
			mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
			mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("error resizing volume"))
			if test.destroyError {
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(nil)
			}

			result := d.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_LUNCreateFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		message      string
		destroyError bool
	}{
		{"nil", false},
		{"failed to delete volume", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{}, nil)
			mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
			mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
			mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
			mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
			mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(fmt.Errorf("failed to create lun"))
			if test.destroyError {
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(nil)
			}

			result := d.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_TooManyLUNs(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(3).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(2).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(2).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(2).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(2).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(api.TooManyLunsError("too many luns"))
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeCreate_GetLUNFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(nil, fmt.Errorf("failed to fetch lun"))

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeCreate_LUNSetAttributeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		message   string
		errorType string
	}{
		{"nil", "None"},
		{"failed to delete volume", "Volume"},
		{"failed to delete lun", "Lun"},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{}, nil)
			mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
			mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
			mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
			mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
			mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
			mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)
			mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Times(1).Return(fmt.Errorf("failed to set attribute"))

			switch test.errorType {
			case "Lun":
				mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(fmt.Errorf(test.message))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(nil)
			case "Volume":
				mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(fmt.Errorf(test.message))
			default:
				mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(nil)
			}

			result := d.Create(ctx, volConfig, pool1, volAttrs)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeCreate_Resize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	lun := api.Lun{
		Size: "10737418240", Name: "lun_storagePrefix_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(2).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&lun, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(1073741824), nil).Times(2)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeCreate_ResizeVolumeSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	lun := api.Lun{
		Size: "10737418240", Name: "lun_storagePrefix_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&lun, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(1073741824), fmt.Errorf("failed to set size"))

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeCreate_ResizeSetSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	lun := api.Lun{
		Size: "10737418240", Name: "lun_storagePrefix_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{}, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&lun, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(1073741824), nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to set volume size"))

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeCreate_ResizeVolumeSizeFailed2(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	lun := api.Lun{
		Size: "10737418240", Name: "lun_storagePrefix_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{}, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&lun, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(1073741824), nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(1073741824), fmt.Errorf("failed to get volume size"))

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeCreate_FormatOptions(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	tempFormatOptions := strings.Join([]string{"-b 4096", "-T stride=256, stripe=16"},
		filesystem.FormatOptionsSeparator)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve:      "none",
		SnapshotPolicy:    "fake-snap-policy",
		SnapshotReserve:   "10",
		UnixPermissions:   "0755",
		ExportPolicy:      "fake-export-policy",
		SecurityStyle:     "mixed",
		Encryption:        "false",
		TieringPolicy:     "none",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    "false",
		FormatOptions:     tempFormatOptions,
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}
	d.Config.LimitVolumeSize = "2g"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Times(1).Return(api.Volumes{}, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)

	// This is the assertion of this unit test,
	// checking whether the argument FormatOptions matches with what we pass in the internal attributes.
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), tempFormatOptions).Times(1).Return(nil)

	result := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeClone(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		CloneSourceVolumeInternal: "storagePrefix_lunName",
		Size:                      "1g",
		Encryption:                "false",
		FileSystem:                "xfs",
	}
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/volumeName/storagePrefix_lunName",
		VolumeName: "volumeName",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{sourceLun}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)

	result := d.CreateClone(ctx, nil, volConfig, pool1)

	assert.Nil(t, result)
}

func TestOntapSanEconomyVolumeClone_getSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		CloneSourceVolumeInternal: "storagePrefix_lunName",
		Size:                      "1g",
		Encryption:                "false",
		FileSystem:                "xfs",
	}
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/volumeName/storagePrefix_lunName",
		VolumeName: "volumeName",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{sourceLun}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(nil, failed)

	result := d.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeClone_OverPoolSizeLimit_CheckFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		CloneSourceVolumeInternal: "storagePrefix_lunName",
		Size:                      "1g",
		Encryption:                "false",
		FileSystem:                "xfs",
	}
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/volumeName/storagePrefix_lunName",
		VolumeName: "volumeName",
	}
	d.Config.LimitVolumePoolSize = "1500MiB"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{sourceLun}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(&sourceLun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{sourceLun}, nil)

	result := d.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result)
	ok, _ := utilserrors.HasUnsupportedCapacityRangeError(result)
	assert.True(t, ok)
}

func TestOntapSanEconomyVolumeClone_OverPoolSizeLimit_CheckPassed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		CloneSourceVolumeInternal: "storagePrefix_lunName",
		Size:                      "1g",
		Encryption:                "false",
		FileSystem:                "xfs",
	}
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/volumeName/storagePrefix_lunName",
		VolumeName: "volumeName",
	}
	cloneLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/volumeName/storagePrefix_lunName2",
		VolumeName: "volumeName",
	}
	d.Config.LimitVolumePoolSize = "3g"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{sourceLun}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(&sourceLun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{sourceLun}, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return([]api.Lun{sourceLun, cloneLun}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, "volumeName", "2147483648").Times(1).Return(nil)

	result := d.CreateClone(ctx, nil, volConfig, pool1)

	assert.Nil(t, result)
}

func TestOntapSanEconomyVolumeClone_OverPoolSizeLimitCheck_VolumeInfoFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		CloneSourceVolumeInternal: "storagePrefix_lunName",
		Size:                      "1g",
		Encryption:                "false",
		FileSystem:                "xfs",
	}
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/volumeName/storagePrefix_lunName",
		VolumeName: "volumeName",
	}
	d.Config.LimitVolumePoolSize = "1500MiB"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{sourceLun}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, sourceLun.Name).Return(&sourceLun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(nil, failed)

	result := d.CreateClone(ctx, nil, volConfig, pool1)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeClone_BothQosPolicy(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[TieringPolicy] = "none"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	volConfig := &storage.VolumeConfig{
		Size:              "1g",
		Encryption:        "false",
		FileSystem:        "xfs",
		QosPolicy:         "fake",
		AdaptiveQosPolicy: "fake",
	}

	result := d.CreateClone(ctx, volConfig, volConfig, pool1)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeImport(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "xfs",
		ImportNotManaged: true,
	}
	d.flexvolNamePrefix = "test_lun_pool_"

	tests := []struct {
		name        string // Name of the test case
		volToImport string
		mocks       func(mockAPI *mockapi.MockOntapAPI)
		wantErr     assert.ErrorAssertionFunc
		testOut     string
	}{
		{
			name:        "VolumePath_error",
			volToImport: "my_LUN",
			mocks:       func(mockAPI *mockapi.MockOntapAPI) {},
			wantErr:     assert.Error,
			testOut:     "Import succeeded",
		},
		{
			name:        "VolumeInfo_Error",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("import is not implemented"))
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_Nil",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_wrongAccessType",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "random"}, nil)
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_ErroFindingLUN",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("LUN not found"))
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_ExtantLUN_Nil",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_dummyLUN_offline",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).Return(&api.Lun{State: "offline"}, nil)
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_unmanagedImport_wrongnaming",
			volToImport: "someRandom/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{State: "online", Size: "1073741824"}, nil)
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_unmanagedImport",
			volToImport: "test_lun_pool_trident_SOMERANDOM/lun0",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).
					Return(&api.Volume{Name: "test_lun_pool_trident_somerandom", AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{State: "online", Size: "1073741824"}, nil)
			},
			wantErr: assert.NoError,
			testOut: "Import succeeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)
			err := d.Import(ctx, volConfig, test.volToImport)
			if !test.wantErr(t, err, test.testOut) {
				// Stop on failure of any tests
				return
			}
		})
	}
}

func TestOntapSanEconomyVolumeImport_Managed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName: "my_vol/my_LUN",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	d.flexvolNamePrefix = "test_lun_pool_"

	tests := []struct {
		name        string // Name of the test case
		volToImport string
		mocks       func(mockAPI *mockapi.MockOntapAPI)
		wantErr     assert.ErrorAssertionFunc
		testOut     string
	}{
		{
			name:        "VolumeInfo_mangedImport_LUNRenameFail",
			volToImport: "my_vol/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{Name: "/vol/my_vol/my_LUN", State: "online", Size: "1073741824"}, nil)
				mockAPI.EXPECT().LunRename(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("LUN rename failed"))
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_mangedImport_VolumeRenameFailed",
			volToImport: "my_vol/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{Name: "/vol/my_vol/my_LUN", State: "online", Size: "1073741824"}, nil)
				mockAPI.EXPECT().LunRename(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				mockAPI.EXPECT().VolumeRename(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("volume rename failed"))
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_mangedImport_LUNNameRestoreFailed",
			volToImport: "my_vol/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{Name: "/vol/my_vol/my_LUN", State: "online", Size: "1073741824"}, nil)
				mockAPI.EXPECT().LunRename(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeRename(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("volume rename failed"))
				mockAPI.EXPECT().LunRename(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("LUN rename failed"))
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_mangedImport_igroupListFailed",
			volToImport: "my_vol/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{Name: "/vol/my_vol/my_LUN", State: "online", Size: "1073741824"}, nil)
				mockAPI.EXPECT().LunRename(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeRename(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("dummy error"))
			},
			wantErr: assert.Error,
			testOut: "Import succeeded",
		},
		{
			name:        "VolumeInfo_mangedImport_igroupListSucceeds",
			volToImport: "my_vol/my_LUN",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
				mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
					Return(&api.Lun{Name: "/vol/my_vol/my_LUN", State: "online", Size: "1073741824"}, nil)
				mockAPI.EXPECT().LunRename(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeRename(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			wantErr: assert.NoError,
			testOut: "Import succeeded",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)
			err := d.Import(ctx, volConfig, test.volToImport)
			if !test.wantErr(t, err, test.testOut) {
				// Stop on failure of any tests
				return
			}
		})
	}
}

func TestOntapSanEconomyVolumeImport_UnsupportedNameLength(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName: "my_vol" +
			"/VOLUME_NAME_BIGGER_THAN_SUPPORTED_LENGTH_wsvawgwasvsdgadsfbadsvadsfhbadvbsDFASBsdvsvsdvsdvsvs" +
			"dvsvsbsdvasdvsdvshdbvjsdvgaisuhvASDJKBVsjvglaIvbsKJAhvsJLhvbAHJLSbvcALJSvbjlv" +
			"dsvSJHcbaJScbkajhsvcljhasvfjahsvcljkASBcljaScvh;jkABCjlhavclakcvlJHASVClAKJSCvb." +
			"AKvjlHASHCvAKLHHVCAKLcvlKHvlJAvclvcklASVckASVlkAVclkASVclkSHVlkSHVlkASVclKASVcLKAHSvkSHcvLKvlkSVklhvlaksV",
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}

	mockAPI.EXPECT().VolumeInfo(gomock.Any(), gomock.Any()).Return(&api.Volume{AccessType: "rw"}, nil)
	mockAPI.EXPECT().LunGetByName(gomock.Any(), gomock.Any()).
		Return(&api.Lun{Name: "/vol/my_vol/my_LUN", State: "online", Size: "1073741824"}, nil)

	err := d.Import(ctx, volConfig, volConfig.InternalName)
	assert.Error(t, err, "Import should fail with name length exceeding limits")
}

func TestOntapSanEconomyVolumeRename(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	err := d.Rename(ctx, "volInternal", "newVolInternal")

	assert.EqualError(t, err, "rename is not implemented")
}

func TestOntapSanEconomyVolumeDestroy(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}
	flexVol := &api.Volume{
		Name:    "flexvol",
		Comment: "flexvol",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return([]string{"igroup1"}, nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup1", gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil).Times(2)
	mockAPI.EXPECT().VolumeInfo(ctx, "volumeName").Return(flexVol, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, "volumeName", "1073741824").Return(nil).Times(2)

	result := d.Destroy(ctx, volConfig)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeDestroy_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
			}

			result := d.Destroy(ctx, volConfig)

			if test.expectError {
				assert.Error(t, result)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}

func TestOntapSanEconomyVolumeDestroy_InvalidSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "invalid_size", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()

	result := d.Destroy(ctx, volConfig)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDestroy_DeleteSnapshotFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(fmt.Errorf("failed to delete lun"))

	result := d.Destroy(ctx, volConfig)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDestroy_UnmapFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}
	flexVol := &api.Volume{
		Name:    "flexvol",
		Comment: "flexvol",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "volumeName").Return(flexVol, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, "volumeName", "1073741824").Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return([]string{"igroup1"}, nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup1", gomock.Any()).Return(fmt.Errorf("failed to unmap lun"))

	result := d.Destroy(ctx, volConfig)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDestroy_LUNDestroyFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}
	flexVol := &api.Volume{
		Name:    "flexvol",
		Comment: "flexvol",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return([]string{"igroup1"}, nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup1", gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "volumeName").Return(flexVol, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, "volumeName", "1073741824").Return(nil)
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(fmt.Errorf("failed to destroy lun"))

	result := d.Destroy(ctx, volConfig)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDestroy_DockerContext(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.Config.DriverContext = tridentconfig.ContextDocker
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}
	flexVol := &api.Volume{
		Name:    "flexvol",
		Comment: "flexvol",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("node", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, "SVM1").Return([]string{"node"}, nil)
	mockAPI.EXPECT().LunMapInfo(ctx, "", gomock.Any()).Return(123, nil)
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil).Times(2)
	mockAPI.EXPECT().VolumeInfo(ctx, "volumeName").Return(flexVol, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, "volumeName", "1073741824").Return(nil).Times(2)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return([]string{"igroup1"}, nil)
	mockAPI.EXPECT().LunUnmap(ctx, "igroup1", gomock.Any()).Return(nil)

	result := d.Destroy(ctx, volConfig)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeDestroy_DockerContext_Failure(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.Config.DriverContext = tridentconfig.ContextDocker
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("", fmt.Errorf("error fetching node name"))

	result := d.Destroy(ctx, volConfig)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDestroy_DockerContext_LunMapInfoError(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.Config.DriverContext = tridentconfig.ContextDocker
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	volConfig := &storage.VolumeConfig{
		InternalName: "storagePrefix_vol1",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}
	snapLuns := []api.Lun{
		{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_vol1_snapshot_mySnap", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(snapLuns, nil).AnyTimes()
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Return("node", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, "SVM1").Return([]string{"node"}, nil)
	mockAPI.EXPECT().LunMapInfo(ctx, "", gomock.Any()).Return(0, fmt.Errorf("failed to fetch lun map info"))

	result := d.Destroy(ctx, volConfig)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDeleteBucketIfEmpty_Fsx(t *testing.T) {
	var bucketVol string = "volumeName"
	mockAPI, mockAWSAPI, d := newMockAWSOntapSanEcoDriver(t)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)

	vol := awsapi.Volume{
		State: "AVAILABLE",
	}
	volConfig := &storage.VolumeConfig{
		InternalName: bucketVol,
	}
	mockAWSAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, &vol, nil)
	mockAWSAPI.EXPECT().WaitForVolumeStates(ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).Return("", nil)
	mockAWSAPI.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
	result := d.DeleteBucketIfEmpty(ctx, bucketVol)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeDeleteBucketIfEmpty(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, "volumeName", true).Return(nil)

	result := d.DeleteBucketIfEmpty(ctx, "volumeName")

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeDeleteBucketIfEmpty_LUNFetchFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(nil, fmt.Errorf("error fetching lun"))

	result := d.DeleteBucketIfEmpty(ctx, "volumeName")

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDeleteBucketIfEmpty_LUNDestroyFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, "volumeName", true).Return(fmt.Errorf("failed to destroy lun"))

	result := d.DeleteBucketIfEmpty(ctx, "volumeName")

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeDeleteBucketIfEmpty_VolumeSetSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_vol1", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "volumeName").Return(nil, fmt.Errorf("failed to fetch volume info"))
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to resize the volume"))

	result := d.DeleteBucketIfEmpty(ctx, "volumeName")

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumePublish(t *testing.T) {
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	d.Config.SANType = sa.ISCSI

	volConfig := &storage.VolumeConfig{
		InternalName:     "lunName",
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "xfs",
		ImportNotManaged: true,
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		HostIQN:          []string{"host_iqn"},
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged:        false,
	}

	dummyLun := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "lunName", VolumeName: "volumeName"}}, nil)
	mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/volumeName/lunName")
	mockAPI.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/lunName", "formatOptions")
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/volumeName/lunName").Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"1.1.1.1"}, nil)

	result := d.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumePublishSLMError(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	d.Config.SANType = sa.ISCSI

	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		HostIQN:          []string{"host_iqn"},
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged:        false,
	}

	dummyLun := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "lunName", VolumeName: "volumeName"}}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/volumeName/lunName")
	mockAPI.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/lunName", "formatOptions")
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/volumeName/lunName").Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{}, nil)

	result := d.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumePublish_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		HostIQN:          []string{"host_iqn"},
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged:        false,
	}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"volume does not exist", false},
		{"error checking for existing volume", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
			}

			result := d.Publish(ctx, volConfig, publishInfo)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumePublish_GetISCSITargetInfoFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		HostIQN:          []string{"host_iqn"},
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged:        false,
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "lunName", VolumeName: "volumeName"}}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("", fmt.Errorf("failed to get name"))

	result := d.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeUnpublish_LegacyVolume(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)

	volConfig := &storage.VolumeConfig{
		InternalName: "lun0",
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: false},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: false},
	}

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
}

func TestOntapSanEconomyVolumeUnpublish_LunListFails(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)

	apiError := fmt.Errorf("api error")
	volumeName := "lun0"
	volConfig := &storage.VolumeConfig{
		InternalName: volumeName,
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: true},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
	}

	lunPathPattern := d.helper.GetLUNPathPattern(volumeName)
	mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(nil, apiError)

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
}

func TestOntapSanEconomyVolumeUnpublish_LUNDoesNotExist(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)
	volumeName := "lun0"
	volConfig := &storage.VolumeConfig{
		InternalName: volumeName,
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: true},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
	}

	lunPathPattern := d.helper.GetLUNPathPattern(volumeName)
	mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(nil, nil)

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
}

func TestOntapSanEconomyVolumeUnpublish_LUNMapInfoFails(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)

	apiError := fmt.Errorf("api error")
	volumeName := "lun0"
	bucketName := "bucket0"
	luns := api.Luns{
		{
			Name:       volumeName,
			VolumeName: bucketName,
		},
	}
	volConfig := &storage.VolumeConfig{
		InternalName: "lun0",
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: true},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
	}

	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := d.helper.GetLUNPath(bucketName, volumeName)
	lunPathPattern := d.helper.GetLUNPathPattern(volumeName)

	mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, apiError)

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
}

func TestOntapSanEconomyVolumeUnpublish_IgroupListLUNsMappedFails(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)

	apiError := fmt.Errorf("api error")
	volumeName := "lun0"
	bucketName := "bucket0"
	luns := api.Luns{
		{
			Name:       volumeName,
			VolumeName: bucketName,
		},
	}
	volConfig := &storage.VolumeConfig{
		InternalName: "lun0",
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: true},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
	}

	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := d.helper.GetLUNPath(bucketName, volumeName)
	lunPathPattern := d.helper.GetLUNPathPattern(volumeName)

	mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, apiError)

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
}

func TestOntapSanEconomyVolumeUnpublish_IgroupDestroyFails(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)

	apiError := fmt.Errorf("api error")
	volumeName := "lun0"
	bucketName := "bucket0"
	luns := api.Luns{
		{
			Name:       volumeName,
			VolumeName: bucketName,
		},
	}
	volConfig := &storage.VolumeConfig{
		InternalName: "lun0",
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: true},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
	}

	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := d.helper.GetLUNPath(bucketName, volumeName)
	lunPathPattern := d.helper.GetLUNPathPattern(volumeName)

	mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, nil)
	mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(apiError)

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
}

func TestOntapSanEconomyVolumeUnpublishX(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)

	volumeName := "lun0"
	bucketName := "bucket0"
	luns := api.Luns{
		{
			Name:       volumeName,
			VolumeName: bucketName,
		},
	}
	volConfig := &storage.VolumeConfig{
		InternalName: "lun0",
		AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: true},
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:         "bar",
		TridentUUID:      "1234",
		VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: true},
	}

	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := d.helper.GetLUNPath(bucketName, volumeName)
	lunPathPattern := d.helper.GetLUNPathPattern(volumeName)

	mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
	mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
	mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, nil)
	mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(nil)

	err := d.Unpublish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
}

func TestOntapSanEconomyVolumeUnpublish(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	apiError := fmt.Errorf("api error")
	volumeName := "lun0"
	bucketName := "bucket0"
	luns := api.Luns{
		{
			Name:       volumeName,
			VolumeName: bucketName,
		},
	}

	type args struct {
		publishEnforcement bool
	}

	tt := []struct {
		name       string
		args       args
		usePattern bool
		mocks      func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string)
		wantErr    assert.ErrorAssertionFunc
	}{
		{
			name: "LUNDoesNotExist",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string) {
				mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(nil, nil)
			},
			wantErr: assert.Error,
		},
		{
			name:       "LUNMapInfoFails",
			args:       args{publishEnforcement: true},
			usePattern: true,
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string) {
				mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, apiError)
			},
			wantErr: assert.Error,
		},
		{
			name: "LUNUnmapFails",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string) {
				mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(apiError)
			},
			wantErr: assert.Error,
		},
		{
			name: "IgroupListLUNsMappedFails",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string) {
				mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, apiError)
			},
			wantErr: assert.Error,
		},
		{
			name: "IgroupDestroyFails",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string) {
				mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, nil)
				mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(apiError)
			},
			wantErr: assert.Error,
		},
		{
			name: "UnpublishSucceeds",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath, lunPathPattern string) {
				mockAPI.EXPECT().LunList(ctx, lunPathPattern).Return(luns, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(0, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return(nil, nil)
				mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(nil)
			},
			wantErr: assert.NoError,
		},
	}
	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				InternalName: volumeName,
				AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			publishInfo := &models.VolumePublishInfo{
				HostName:         "bar",
				TridentUUID:      "1234",
				VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			mockCtrl := gomock.NewController(t)
			mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
			d.API = mockAPI
			d.helper = NewTestLUNHelper("", tridentconfig.ContextCSI)

			igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
			lunPath := d.helper.GetLUNPath(bucketName, volumeName)
			lunPathPattern := d.helper.GetLUNPathPattern(volumeName)

			tr.mocks(mockAPI, igroupName, lunPath, lunPathPattern)

			err := d.Unpublish(ctx, volConfig, publishInfo)
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}
		})
	}
}

func TestDriverCanSnapshot(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	d := newTestOntapSanEcoDriver(t, vserverAdminHost, vserverAdminPort, vserverAggrName, false, nil, mockAPI)

	result := d.CanSnapshot(ctx, nil, nil)

	assert.NoError(t, result)
}

func TestOntapSanEconomyGetSnapshot(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "lunName",
		VolumeName:         "volumeName",
		Name:               "lunName",
		VolumeInternalName: "volumeName",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{
		api.Lun{
			Size:       "1g",
			Name:       "volumeName_snapshot_lunName",
			VolumeName: "volumeName",
		},
	},
		nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)

	snap, err := d.GetSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, err, "Error is not nil")
	assert.NotNil(t, snap, "snapshot is nil")
}

func TestOntapSanEconomyGetSnapshots(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{
		api.Lun{
			Size:       "1073741824",
			Name:       "volumeName_snapshot_lunName",
			VolumeName: "volumeName",
		},
	},
		nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{
		api.Lun{
			Size:       "1073741824",
			Name:       "/vol/volumeName/storagePrefix_LUNName_snapshot_mySnap",
			VolumeName: "volumeName",
		},
	},
		nil)

	snaps, err := d.GetSnapshots(ctx, volConfig)

	assert.NoError(t, err)
	assert.NotNil(t, snaps, "snapshots are nil")
}

func TestOntapSanEconomyGetSnapshots_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}
	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"volume does not exist", false},
		{"error checking for existing volume", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
			}

			snaps, err := d.GetSnapshots(ctx, volConfig)

			assert.Error(t, err, "Error is not nil")
			assert.Nil(t, snaps, "snapshots are nil")
		})
	}
}

func getStructsForSnapshotCreate() (string, *storage.SnapshotConfig, string, api.Lun, api.Lun) {
	storagePrefix := "storagePrefix"

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snapshot-00c6b55c-afeb-4657-b86e-156d66c0c9fc",
		VolumeName:         "pvc-ff297a18-921a-4435-b679-c3fea351f92c",
		Name:               "snapshot-00c6b55c-afeb-4657-b86e-156d66c0c9fc",
		VolumeInternalName: "storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c",
	}

	bucketVol := "trident_lun_pool_storagePrefix_MGURDMZTKA"

	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
	}
	snapLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_00c6b55c_afeb_4657_b86e_156d66c0c9fc",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
	}
	return storagePrefix, snapConfig, bucketVol, sourceLun, snapLun
}

func TestOntapSanEconomyCreateSnapshot(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, sourceLun, snapLun := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), api.QosPolicyGroup{}).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{snapLun}, nil)

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, err, "Error is not nil")
	assert.NotNil(t, snap, "snapshots are nil")
}

func TestOntapSanEconomyCreateSnapshot_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, _, _ := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	tests := []struct {
		message     string
		expectError bool
	}{
		{"volume does not exist", false},
		{"error checking for existing volume", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
			}

			snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

			assert.Error(t, err)
			assert.Nil(t, snap)
		})
	}
}

func TestOntapSanEconomyCreateSnapshot_LUNCreateCloneFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, sourceLun, _ := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(),
		api.QosPolicyGroup{}).Return(fmt.Errorf("failed to create lun clone"))

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

	assert.Error(t, err)
	assert.Nil(t, snap)
}

func TestOntapSanEconomyCreateSnapshot_LUNGetByNameFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, sourceLun, _ := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to fetch lun"))

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

	assert.Error(t, err)
	assert.Nil(t, snap, "snapshots are nil")
}

func TestOntapSanEconomyCreateSnapshot_CreateLUNClone_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.flexvolNamePrefix = "storagePrefix"
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
	}
	d.Config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	policy := api.QosPolicyGroup{}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil)
			}

			result := d.createLUNClone(ctx, "storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c",
				"source", "snap", &d.Config, mockAPI, "storagePrefix", true, policy)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyCreateSnapshot_CreateLUNClone_LUNCreatedFromSnapshot(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.flexvolNamePrefix = "storagePrefix"
	d.Config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	policy := api.QosPolicyGroup{}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
			}

			result := d.createLUNClone(ctx, "lun_storagePrefix_vol1", "source", "snap", &d.Config, mockAPI,
				"storagePrefix", true, policy)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyCreateSnapshot_CreateLUNClone_LUNCloneCreateFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.flexvolNamePrefix = "storagePrefix"
	sourceLun := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_00c6b55c_afeb_4657_b86e_156d66c0c9fc",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
	}
	d.Config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	policy := api.QosPolicyGroup{}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(),
		api.QosPolicyGroup{}).Return(fmt.Errorf("failed to create lun clone"))

	result := d.createLUNClone(ctx,
		"storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_e753e1a5-e57b-4f94-adbc-22f7c05e2121",
		"storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c", "snapshot-00c6b55c-afeb-4657-b86e-156d66c0c9fc",
		&d.Config, mockAPI, "storagePrefix", true, policy)

	assert.Error(t, result)
}

func TestOntapSanEconomyCreateSnapshot_GetSnapshotsEconomyFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, sourceLun, _ := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(1)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), api.QosPolicyGroup{}).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to fetch lun")).Times(2)

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

	assert.Error(t, err)
	assert.Nil(t, snap, "snapshots are nil")
}

func TestOntapSanEconomyCreateSnapshot_InvalidSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, sourceLun, snapLun := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(2)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&snapLun, nil)

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

	assert.Error(t, err)
	assert.Nil(t, snap, "snapshots are nil")
}

func TestOntapSanEconomyCreateSnapshot_SnapshotNotFound(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	storagePrefix, snapConfig, _, sourceLun, snapLun := getStructsForSnapshotCreate()
	d.flexvolNamePrefix = storagePrefix

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return([]api.Lun{sourceLun}, nil).Times(4)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&snapLun, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&sourceLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), api.QosPolicyGroup{}).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)

	assert.Error(t, err)
	assert.Nil(t, snap, "snapshots are nil")
}

func getStructsForSnapshotRestore() (
	*storage.SnapshotConfig, string, api.Lun, string, string, api.Lun, string, api.Lun, string,
) {
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snapshot-00c6b55c-afeb-4657-b86e-156d66c0c9fc",
		VolumeName:         "pvc-ff297a18-921a-4435-b679-c3fea351f92c",
		Name:               "snapshot-00c6b55c-afeb-4657-b86e-156d66c0c9fc",
		VolumeInternalName: "storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c",
	}

	bucketVol := "trident_lun_pool_storagePrefix_MGURDMZTKA"

	qosPolicy := api.QosPolicyGroup{
		Name: "extreme",
		Kind: api.QosPolicyGroupKind,
	}

	lun := api.Lun{
		Size:       "1073741824",
		Name:       "storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
		Qos:        qosPolicy,
	}
	volLunPath := "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c"
	tempVolLunPath := volLunPath + "_original"

	snapLun := api.Lun{
		Size:       "1073741824",
		Name:       "storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_00c6b55c_afeb_4657_b86e_156d66c0c9fc",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
		Qos:        qosPolicy,
	}
	snapLunPath := "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_00c6b55c_afeb_4657_b86e_156d66c0c9fc"

	snapLunCopy := api.Lun{
		Size:       "1073741824",
		Name:       "storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_00c6b55c_afeb_4657_b86e_156d66c0c9fc_copy",
		VolumeName: "trident_lun_pool_storagePrefix_MGURDMZTKA",
		Qos:        qosPolicy,
	}
	snapLunCopyPath := "/vol/trident_lun_pool_storagePrefix_MGURDMZTKA/storagePrefix_pvc_ff297a18_921a_4435_b679_c3fea351f92c_snapshot_snapshot_00c6b55c_afeb_4657_b86e_156d66c0c9fc_copy"

	return snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath
}

func TestOntapSanEconomyVolumeRestoreSnapshot(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, snapLunCopyPath, volLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, tempVolLunPath).Times(1).Return([]string{}, nil)
	mockAPI.EXPECT().LunDestroy(ctx, tempVolLunPath).Times(1).Return(nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_VolLunListFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, _, _, _, _, _, _, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(nil, failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_NonexistentVolLun(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, _, _, _, _, _, _, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunListFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, _, _, _, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{lun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(nil, failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_NonexistentSnapLun(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, _, _, _, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun}, nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_DifferentFlexvols(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, snapLun, _, _, _ := getStructsForSnapshotRestore()
	snapLun.VolumeName = "otherFlexvol"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyGetFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, snapLun, _, _, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(nil, failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyExists(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{snapLunCopy}, nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, snapLunCopyPath).Times(1).Return([]string{}, nil)
	mockAPI.EXPECT().LunDestroy(ctx, snapLunCopyPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, snapLunCopyPath, volLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, tempVolLunPath).Times(1).Return([]string{}, nil)
	mockAPI.EXPECT().LunDestroy(ctx, tempVolLunPath).Times(1).Return(nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyExistsDifferentFlexvols(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, snapLun, _, snapLunCopy, _ := getStructsForSnapshotRestore()
	snapLunCopy.VolumeName = "otherFlexvol"

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{snapLunCopy}, nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyExistsUnmapFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, snapLun, _, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{snapLunCopy}, nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, snapLunCopyPath).Times(1).Return(nil, failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyExistsDestroyFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, snapLun, _, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{snapLunCopy}, nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, snapLunCopyPath).Times(1).Return([]string{}, nil)
	mockAPI.EXPECT().LunDestroy(ctx, snapLunCopyPath).Times(1).Return(failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunGetFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, _, lun, _, _, snapLun, snapLunPath, _, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(nil, failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCloneFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, _, _, snapLun, snapLunPath, snapLunCopy, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_VolLunRenameFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, _ := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyRenameFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, snapLunCopyPath, volLunPath).Times(1).Return(failed)
	mockAPI.EXPECT().LunRename(ctx, tempVolLunPath, volLunPath).Times(1).Return(nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_SnapLunCopyRenamesFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, snapLunCopyPath, volLunPath).Times(1).Return(failed)
	mockAPI.EXPECT().LunRename(ctx, tempVolLunPath, volLunPath).Times(1).Return(failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_UnmapFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, snapLunCopyPath, volLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, tempVolLunPath).Times(1).Return(nil, failed)
	mockAPI.EXPECT().LunDestroy(ctx, tempVolLunPath).Times(1).Return(nil)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeRestoreSnapshot_DestroyFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	snapConfig, bucketVol, lun, volLunPath, tempVolLunPath, snapLun, snapLunPath, snapLunCopy, snapLunCopyPath := getStructsForSnapshotRestore()

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(2).Return(api.Luns{lun, snapLun}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, snapLunPath).Times(1).Return(&snapLun, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, bucketVol, snapLun.Name, snapLunCopy.Name, snapLun.Qos).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, volLunPath, tempVolLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunRename(ctx, snapLunCopyPath, volLunPath).Times(1).Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, tempVolLunPath).Times(1).Return([]string{}, nil)
	mockAPI.EXPECT().LunDestroy(ctx, tempVolLunPath).Times(1).Return(failed)

	result := d.RestoreSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeDeleteSnapshot(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap_1",
		VolumeName:         "my_Bucket",
		Name:               "/vol/my_Bucket/storagePrefix_my_Lun",
		VolumeInternalName: "storagePrefix_my_Lun_my_Bucket",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(2).Return(api.Luns{
		api.Lun{
			Size:       "1073741824",
			Name:       "storagePrefix_my_Lun_my_Bucket_snapshot_snap_1",
			VolumeName: "my_Bucket",
		},
	},
		nil)
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Times(1)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{api.Lun{}}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)

	result := d.DeleteSnapshot(ctx, snapConfig, nil)

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeDeleteSnapshot_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap_1",
		VolumeName:         "my_Bucket",
		Name:               "/vol/my_Bucket/storagePrefix_my_Lun",
		VolumeInternalName: "storagePrefix_my_Lun_my_Bucket",
	}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
			}

			result := d.DeleteSnapshot(ctx, snapConfig, nil)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyGet(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{
		api.Lun{
			Size:       "1073741824",
			Name:       "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket",
			VolumeName: "my_Bucket",
		},
	},
		nil)

	result := d.Get(ctx, "my_Bucket")

	assert.NoError(t, result)
}

func TestOntapSanEconomyGet_ImportPath(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	testLUNPath := "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket"
	testLUN := api.Lun{
		Size:       "1073741824",
		Name:       "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket",
		VolumeName: "my_Bucket",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{testLUN}, nil)
	result := d.Get(ctx, testLUNPath)
	assert.NoError(t, result, "error in LunGetByName")

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	result = d.Get(ctx, testLUNPath)
	assert.Error(t, result, "LUN found")
}

func TestOntapSanEconomyGet_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
			}

			result := d.Get(ctx, "my_Bucket")

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyEnsureFlexvolForLUN_InvalidVolumeSize(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	opts := make(map[string]string)
	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	d.Config.LimitVolumePoolSize = "invalid"

	flexVol, newly, err := d.ensureFlexvolForLUN(ctx, &api.Volume{}, uint64(1073741824), opts, &d.Config, pool1,
		make(map[string]struct{}))

	assert.Equal(t, flexVol, "")
	assert.False(t, newly)
	assert.Error(t, err)
}

func TestOntapSanEconomyEnsureFlexvolForLUN_FlexvolFound(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	opts := make(map[string]string)
	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name: "storagePrefix_vol1",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)

	flexVol, newly, err := d.ensureFlexvolForLUN(ctx, vol, uint64(1073741824), opts, &d.Config, pool1,
		make(map[string]struct{}))

	assert.NotEqual(t, flexVol, "")
	assert.False(t, newly)
	assert.NoError(t, err)
}

func TestOntapSanEconomyEnsureFlexvolForLUN_FlexvolNotFound(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	opts := make(map[string]string)
	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name:       "",
		Aggregates: []string{"data"},
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(fmt.Errorf("failed to create volume"))

	flexVol, newly, err := d.ensureFlexvolForLUN(ctx, vol, uint64(1073741824), opts, &d.Config, pool1,
		make(map[string]struct{}))

	assert.Equal(t, flexVol, "")
	assert.False(t, newly)
	assert.Error(t, err)
}

func TestOntapSanEconomyEnsureFlexvolForLUN_NewFlexvolNotPermitted(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	configJSON := fmt.Sprintf(`
	{
	    "managementLIF":       "10.0.207.8",
	    "dataLIF":             "10.0.207.7",
	    "svm":                 "SVM1",
	    "aggregate":           "data",
	    "username":            "admin",
	    "password":            "password",
	    "storageDriverName":   "ontap-san-economy",
	    "storagePrefix":       "san-eco",
	    "version":             1,
        "denyNewVolumePools":  "true"
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	authResponse := api.IscsiInitiatorAuth{
		SVMName:  "SVM1",
		AuthType: "None",
	}
	d.telemetry = &Telemetry{
		Plugin: d.Name(),
		SVM:    "SVM1",
		Driver: d,
		done:   make(chan struct{}),
	}
	d.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	d.telemetry.TridentBackendUUID = BackendUUID
	d.telemetry.StoragePrefix = "trident_"
	hostname, _ := os.Hostname()
	message, _ := json.Marshal(d.GetTelemetry())

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(authResponse, nil)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san-economy", "1", false, "heartbeat", hostname, string(message), 1,
		"trident", 5).AnyTimes()
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	err := d.Initialize(ctx, "csi", configJSON, commonConfig, secrets, BackendUUID)

	opts := make(map[string]string)
	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 100
	vol := &api.Volume{
		Name:       "",
		Aggregates: []string{"data"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{}, nil)

	flexVol, newly, err := d.ensureFlexvolForLUN(ctx, vol, uint64(1073741824), opts, &d.Config, pool1,
		make(map[string]struct{}))

	// Expect error as no new flexvol may be created
	assert.Equal(t, "", flexVol, "Expected no Flexvol, got %s", flexVol)
	assert.False(t, newly)
	assert.Error(t, err, "Expected error when needing to create new Flexvol, got nil")
}

func TestOntapSanEconomyGetStorageBackendSpecs(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	d.ips = []string{"127.0.0.1"}
	backend := storage.StorageBackend{}

	result := d.GetStorageBackendSpecs(ctx, &backend)

	assert.Nil(t, result)
}

func TestOntapSanEconomyGetStorageBackendPhysicalPoolNames(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	poolNames := d.GetStorageBackendPhysicalPoolNames(ctx)

	assert.Equal(t, "pool1", poolNames[0], "Pool names are not equal")
}

func TestOntapSanEconomyGetStorageBackendPools(t *testing.T) {
	mockAPI, driver := newMockOntapSanEcoDriver(t)
	svmUUID := "SVM1-uuid"
	flexVolPrefix := fmt.Sprintf("trident_lun_pool_%s_", *driver.Config.StoragePrefix)
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

func TestOntapSanEconomyGetInternalVolumeName(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	d.Config.StoragePrefix = utils.Ptr("storagePrefix_")
	volConfig := &storage.VolumeConfig{Name: "my-Lun"}
	pool := storage.NewStoragePool(nil, "dummyPool")

	internalVolName := d.GetInternalVolumeName(ctx, volConfig, pool)

	assert.Equal(t, "storagePrefix_my_Lun", internalVolName, "Strings not equal")
}

func TestOntapSanEconomyGetProtocol(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	protocol := d.GetProtocol(ctx)

	assert.Equal(t, protocol, tridentconfig.Block, "Protocols not equal")
}

func TestOntapSanEconomyStoreConfig(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	persistentConfig := &storage.PersistentStorageBackendConfig{}

	d.StoreConfig(ctx, persistentConfig)

	assert.Equal(t, json.RawMessage("{}"), d.Config.CommonStorageDriverConfig.StoragePrefixRaw,
		"raw prefix mismatch")
	assert.Equal(t, d.Config, *persistentConfig.OntapConfig, "ontap config mismatch")
}

func TestGetVolumeForImport(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx,
		gomock.Any()).Times(1).Return(&api.Lun{
		Size:       "1073741824",
		Name:       "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket",
		VolumeName: "my_Bucket",
	},
		nil)

	result, resultErr := d.GetVolumeForImport(ctx, "my_Bucket/my_Lun")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "my_Lun_my_Bucket", result.Config.Name)
	assert.Equal(t, "storagePrefix_my_Lun_my_Bucket", result.Config.InternalName)
}

func TestGetVolumeExternal_MultipleAggregates(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	lun := api.Lun{
		Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap",
		VolumeName: "myLun",
	}
	vol := api.Volume{
		Aggregates: []string{"aggr1", "aggr2"},
	}

	volExt := d.getVolumeExternal(&lun, &vol)

	assert.NotNil(t, volExt)
}

func TestGetVolumeForImport_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	result, err := d.GetVolumeForImport(ctx, "my_Lun")

	assert.Error(t, err)
	assert.Nil(t, result)

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to get volume"))
	result, err = d.GetVolumeForImport(ctx, "my_bucket/my_LUN")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetVolumeForImport_VolumeInfoFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(nil, fmt.Errorf("failed to get volume info"))

	result, err := d.GetVolumeForImport(ctx, "my_Bucket/my_Lun")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetVolumeForImport_LunGetByNameFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{Aggregates: []string{"data"}}, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to get lun by name"))

	result, err := d.GetVolumeForImport(ctx, "my_Bucket/my_Lun")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetVolumeExternalWrappers(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{}}, nil).Times(1)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{api.Lun{}}, nil)

	d.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}

	assert.Len(t, volumes, 1, "wrong number of volumes")
}

func TestGetVolumeExternalWrappers_Failed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to list volume by prefix"))

	d.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			assert.Error(t, wrapper.Error)
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}
}

func TestGetVolumeExternalWrappers_NoVolumes(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, nil)

	d.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}

	assert.Len(t, volumes, 0, "wrong number of volumes")
}

func TestGetVolumeExternalWrappers_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to fetch lun"))
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{}}, nil).Times(1)

	d.GetVolumeExternalWrappers(ctx, channel)
}

func TestGetVolumeExternalWrappers_VolumeNotAttachedWithLUN(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	channel := make(chan *storage.VolumeExternalWrapper, 1)

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{Name: "vol1"}}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{api.Lun{VolumeName: "vol2"}}, nil)

	d.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}

	assert.Len(t, volumes, 0)
}

func TestGetVolumeExternalWrappers_VolumeAttachedWithLUN(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	channel := make(chan *storage.VolumeExternalWrapper, 1)
	luns := []api.Lun{
		{Size: "1g", Name: "storagePrefix_my_Lun_my_Bucket_snapshot_snap", VolumeName: "my_Bucket"},
	}

	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{Name: "my_Bucket"}},
		nil).Times(1)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(luns, nil)

	d.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}

	assert.Len(t, volumes, 0)
}

func TestGetUpdateType_OtherChanges(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	oldDriver := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	oldDriver.API = mockAPI
	prefix1 := "storagePrefix_"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}
	oldDriver.Config.DataLIF = "10.0.2.11"
	oldDriver.Config.Username = "oldUser"
	oldDriver.Config.Password = "oldPassword"

	newDriver := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	oldDriver.API = mockAPI
	prefix2 := "storagePREFIX_"

	newDriver.Config.StoragePrefix = &prefix2
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}
	newDriver.Config.DataLIF = "10.0.2.10"
	newDriver.Config.Username = "newUser"
	newDriver.Config.Password = "newPassword"

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.UsernameChange)
	expectedBitmap.Add(storage.PasswordChange)
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestGetUpdateType_NilDataLIF(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	oldDriver := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	oldDriver.Config.DataLIF = "10.0.2.11"

	newDriver := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	newDriver.Config.DataLIF = "10.0.2.10"

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.False(t, result.Contains(storage.InvalidVolumeAccessInfoChange), "Nil DataLIF should be valid.")
}

func TestGetUpdateType_Failure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	oldDriver := newTestOntapNASDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, "CSI", false, nil)
	newDriver := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestOntapSanEconomyVolumeResize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(2).Return(api.Luns{
		api.Lun{
			Size:       "1073741824",
			Name:       "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket",
			VolumeName: "my_Bucket",
		},
	},
		nil)
	mockAPI.EXPECT().LunGetByName(ctx,
		gomock.Any()).Times(1).Return(&api.Lun{
		Size:       "1073741824",
		Name:       "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket",
		VolumeName: "my_Bucket",
	},
		nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{
		api.Lun{
			Size:       "1073741824",
			Name:       "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket",
			VolumeName: "my_Bucket",
		},
	},
		nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{Aggregates: []string{"data"}}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunSetSize(ctx, gomock.Any(), "2147483648").Return(uint64(2147483648), nil)

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Nil(t, result)
}

func TestOntapSanEconomyVolumeResize_LUNExists(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}

	tests := []struct {
		message string
		isError bool
	}{
		{"volume does not exist", false},
		{"error checking for existing volume", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.isError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{}, nil)
			}

			result := d.Resize(ctx, volConfig, uint64(2147483648))

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyVolumeResize_InvalidLUNSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	luns := []api.Lun{
		{
			Size:       "invalid", // invalid uint64 value
			Name:       "lun_vol1",
			VolumeName: "volumeName",
		},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_GetSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_vol1", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to fetch lun"))

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_VolumeInfoFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to get volume info")).Times(2)

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_SizeError(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}

	tests := []struct {
		message       string
		requestedSize uint64
		currentSize   string
		expectError   bool
	}{
		{"same size", 1073741824, "1073741824", false},
		{"requested size is less than current size", 1073741824, "2147483648", true},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			lun := api.Lun{
				Size: test.currentSize, Name: "lun_vol1",
				VolumeName: "volumeName",
			}
			luns := make([]api.Lun, 0)
			luns = append(luns, lun)

			mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
			mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
			mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{}, nil)

			result := d.Resize(ctx, volConfig, test.requestedSize)

			if test.expectError {
				assert.Error(t, result)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}

func TestOntapSanEconomyVolumeResize_WithInvalidVolumeSizeLimit(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.Config.LimitVolumeSize = "invalid-value" // invalid int value
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil).Times(2)

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_OverSizeLimit(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.Config.LimitVolumeSize = "2GB"
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil).Times(2)

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_OverPoolSizeLimit_OneLUN(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.Config.LimitVolumePoolSize = "2g"
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size:       "1073741824",
		Name:       "lun_vol1",
		VolumeName: "volumeName",
	}
	flexvol := &api.Volume{
		Aggregates: []string{"aggr"},
		Size:       "1073741824",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(flexvol, nil).Times(2)

	result := d.Resize(ctx, volConfig, uint64(3221225472))

	assert.Error(t, result)
	ok, _ := utilserrors.HasUnsupportedCapacityRangeError(result)
	assert.True(t, ok)
}

func TestOntapSanEconomyVolumeResize_OverPoolSizeLimit_MultipleLUNs(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.Config.LimitVolumePoolSize = "5g"
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun1 := api.Lun{
		Size:       "1073741824",
		Name:       "lun1",
		VolumeName: "volumeName",
	}
	lun2 := api.Lun{
		Size:       "3221225472",
		Name:       "lun2",
		VolumeName: "volumeName",
	}
	flexvol := &api.Volume{
		Aggregates: []string{"aggr"},
		Size:       "1073741824",
	}
	luns := []api.Lun{lun1, lun2}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun1, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(flexvol, nil).Times(2)

	result := d.Resize(ctx, volConfig, uint64(3221225472))

	assert.Error(t, result)
	ok, _ := utilserrors.HasUnsupportedCapacityRangeError(result)
	assert.True(t, ok)
}

func TestOntapSanEconomyVolumeResize_VolumeSetSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to set volume size"))

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_LunSetSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(3)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunSetSize(ctx, gomock.Any(), "2147483648").Return(uint64(2147483648),
		fmt.Errorf("failed to set lun size"))

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.Error(t, result)
}

func TestOntapSanEconomyVolumeResize_FlexvolBiggerThanLUN(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(4)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockAPI.EXPECT().LunSetSize(ctx, gomock.Any(), gomock.Any()).Return(uint64(16106127360), nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(2147483648), nil)

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeResize_VolumeSizeFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(4)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mockAPI.EXPECT().LunSetSize(ctx, gomock.Any(), gomock.Any()).Return(uint64(16106127360), nil)
	mockAPI.EXPECT().VolumeSize(ctx, gomock.Any()).Return(uint64(2147483648), fmt.Errorf("failed to get volume size"))

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.NoError(t, result)
}

func TestOntapSanEconomyVolumeResize_VolumeSetSizeFailed2(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}
	lun := api.Lun{
		Size: "1073741824", Name: "lun_vol1",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(4)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&api.Volume{Aggregates: []string{"aggr"}}, nil).Times(2)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunSetSize(ctx, gomock.Any(), gomock.Any()).Return(uint64(16106127360), nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to set volume size"))

	result := d.Resize(ctx, volConfig, uint64(2147483648))

	assert.NoError(t, result)
}

func TestOntapSanEconomyInitialize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "SVM1",
	    "aggregate":         "data",
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	authResponse := api.IscsiInitiatorAuth{
		SVMName:  "SVM1",
		AuthType: "None",
	}
	d.telemetry = &Telemetry{
		Plugin: d.Name(),
		SVM:    "SVM1",
		Driver: d,
		done:   make(chan struct{}),
	}
	d.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	d.telemetry.TridentBackendUUID = BackendUUID
	d.telemetry.StoragePrefix = "trident_"
	hostname, _ := os.Hostname()
	message, _ := json.Marshal(d.GetTelemetry())

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(authResponse, nil)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san-economy", "1", false, "heartbeat", hostname, string(message), 1,
		"trident", 5).AnyTimes()
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)

	assert.NoError(t, result)
}

func TestOntapSanEconomyInitialize_WithNameTemplate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "SVM1",
	    "aggregate":         "data",
		"defaults": {
				"nameTemplate": "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
		},
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	authResponse := api.IscsiInitiatorAuth{
		SVMName:  "SVM1",
		AuthType: "None",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(authResponse, nil)
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)
	assert.NoError(t, result)

	nameTemplate := "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
	assert.Equal(t, nameTemplate, d.physicalPools["data"].InternalAttributes()[NameTemplate])
}

func TestOntapSanEconomyInitialize_NameTemplateDefineInStoragePool(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "SVM1",
	    "aggregate":         "data",
		"storage": [
	     {
	        "defaults":
	         {
	               "nameTemplate": "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
	         }
	     }
	    ],
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	authResponse := api.IscsiInitiatorAuth{
		SVMName:  "SVM1",
		AuthType: "None",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(authResponse, nil)
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)
	assert.NoError(t, result)

	expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
	for _, pool := range d.virtualPools {
		assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestOntapSanEconomyInitialize_NameTemplateDefineInBothPool(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "SVM1",
	    "aggregate":         "data",
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
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	authResponse := api.IscsiInitiatorAuth{
		SVMName:  "SVM1",
		AuthType: "None",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(authResponse, nil)
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)
	assert.NoError(t, result)

	expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
	for _, pool := range d.virtualPools {
		assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
	}
}

func TestOntapSanEconomyInitialize_InvalidConfig(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`{invalid-json}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result)
}

func TestOntapSanEconomyInitialize_NoDataLIFs(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "iscsi_vs",
	    "aggregate":         "data",
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			if test.expectError {
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return(nil, nil)
			}
			result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyInitialize_NumOfLUNs(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}
	authResponse := api.IscsiInitiatorAuth{
		SVMName:  "SVM1",
		AuthType: "None",
	}
	hostname, _ := os.Hostname()

	tests := []struct {
		numOfLUNs   string
		expectError bool
	}{
		{"NaN", true},
		{"100", false},
		{"40", true},
		{"500", true},
	}
	for _, test := range tests {
		t.Run(test.numOfLUNs, func(t *testing.T) {
			commonConfigJSON := fmt.Sprintf(`
			{
			    "managementLIF":     "10.0.207.8",
			    "dataLIF":           "10.0.207.7",
			    "svm":               "iscsi_vs",
			    "aggregate":         "data",
			    "username":          "admin",
			    "password":          "password",
			    "storageDriverName": "ontap-san-economy",
			    "storagePrefix":     "san-eco",
			    "debugTraceFlags":   {"method": true, "api": true},
			    "version":1,
				"lunsPerFlexvol":    "%v"
			}`, test.numOfLUNs)

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
			mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
			mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
				map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
			)
			mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san-economy", "1", false, "heartbeat", hostname,
				gomock.Any(), 1,
				"trident", 5).AnyTimes()
			if !test.expectError {
				mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(authResponse, nil)
				mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid").AnyTimes()
			}

			result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)

			if test.expectError {
				assert.Error(t, result)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}

func TestOntapSanEconomyInitialize_OtherContext(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "iscsi_vs",
	    "aggregate":         "data",
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	tests := []struct {
		driverContext string
		expectError   bool
	}{
		{"docker", true},
		{"invalid", true},
	}
	for _, test := range tests {
		t.Run(test.driverContext, func(t *testing.T) {
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
			mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
			mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
				map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
			)

			result := d.Initialize(ctx, tridentconfig.DriverContext(test.driverContext), commonConfigJSON, commonConfig,
				secrets, BackendUUID)

			assert.Error(t, result)
		})
	}
}

func TestOntapSanEconomyInitialize_NoSVMAggregates(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san-economy",
		BackendName:       "myOntapSanEcoBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	commonConfigJSON := fmt.Sprintf(`
	{
	    "managementLIF":     "10.0.207.8",
	    "dataLIF":           "10.0.207.7",
	    "svm":               "iscsi_vs",
	    "aggregate":         "data",
	    "username":          "admin",
	    "password":          "password",
	    "storageDriverName": "ontap-san-economy",
	    "storagePrefix":     "san-eco",
	    "debugTraceFlags":   {"method": true, "api": true},
	    "version":1
	}`)
	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"10.0.207.7"}, nil)
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return(nil, fmt.Errorf("error getting svm aggregate names"))

	result := d.Initialize(ctx, "csi", commonConfigJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result)
}

func TestOntapSanEconomyInitialize_GetFlexvolForLUN(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name: "storagePrefix_vol1",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(2)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(vol, nil)

	flexVol, err := d.getFlexvolForLUN(ctx, vol, uint64(1073741824), true, uint64(2147483648),
		make(map[string]struct{}))

	assert.NoError(t, err)
	assert.NotEqual(t, "", flexVol)
}

func TestOntapSanEconomyInitialize_GetFlexvolForLUN_InvalidLUNPath(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name: "storagePref_vol1",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "myBucket/storagePref_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)

	flexVol, err := d.getFlexvolForLUN(ctx, vol, uint64(1073741824), false, uint64(2147483648),
		make(map[string]struct{}))

	assert.NoError(t, err)
	assert.Equal(t, "", flexVol)
}

func TestOntapSanEconomyInitialize_GetFlexvolForLUN_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name: "storagePrefix_vol1",
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf("error fetching luns"))

	flexVol, err := d.getFlexvolForLUN(ctx, vol, uint64(1073741824), false, uint64(2147483648),
		make(map[string]struct{}))

	assert.Error(t, err)
	assert.Equal(t, "", flexVol)
}

func TestOntapSanEconomyInitialize_GetFlexvolForLUN_LimitFlexvolSize_Failed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name: "storagePrefix_vol1",
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(nil, fmt.Errorf("failed to get volume"))

	flexVol, err := d.getFlexvolForLUN(ctx, vol, uint64(1073741824), true, uint64(2147483648),
		make(map[string]struct{}))

	assert.NoError(t, err)
	assert.Equal(t, "", flexVol)
}

func TestOntapSanEconomyInitialize_GetFlexvolForLUN_LargeSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol := &api.Volume{
		Name: "storagePrefix_vol1",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(vol, nil)

	flexVol, err := d.getFlexvolForLUN(ctx, vol, uint64(1073741824), true, uint64(1073741824),
		make(map[string]struct{}))

	assert.NoError(t, err)
	assert.Equal(t, "", flexVol)
}

func TestOntapSanEconomyInitialize_GetFlexvolForLUN_IgnoredVols(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.lunsPerFlexvol = 1
	vol1 := &api.Volume{
		Name: "storagePrefix_vol1",
	}
	vol2 := &api.Volume{
		Name: "storagePrefix_vol2",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}

	mockAPI.EXPECT().VolumeListByAttrs(ctx, gomock.Any()).Return(api.Volumes{vol1, vol2}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil).Times(4)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(vol1, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(vol2, nil)

	flexVol, err := d.getFlexvolForLUN(ctx, vol1, uint64(1073741824), true, uint64(2147483648),
		make(map[string]struct{}))

	assert.NoError(t, err)
	assert.NotEqual(t, "", flexVol)
}

func TestOntapSanEconomyInitialize_CreateFlexvolForLUN_InvalidSnapshotReserve(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	vol := &api.Volume{
		Name: "storagePrefix_vol1",
	}
	opts := make(map[string]string)
	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.InternalAttributes()[SnapshotReserve] = "invalid"
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volName, err := d.createFlexvolForLUN(ctx, vol, opts, pool1)

	assert.Error(t, err)
	assert.Equal(t, "", volName)
}

func TestOntapSanEconomyInitialize_CreateFlexvolForLUN_Failed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	vol := &api.Volume{
		Name:       "storagePrefix_vol1",
		Aggregates: []string{"data"},
	}
	opts := make(map[string]string)
	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(fmt.Errorf("failed to create volume"))

	volName, err := d.createFlexvolForLUN(ctx, vol, opts, pool1)

	assert.Error(t, err)
	assert.Equal(t, "", volName)
}

// func TestOntapSanEconomyInitialize_CreateFlexvolForLUN_VolumeModifySnapshotDirectoryAccessFailed(t *testing.T) {
//	mockAPI, d := newMockOntapSanEcoDriver(t)
//	vol := &api.Volume{
//		Name:       "storagePrefix_vol1",
//		Aggregates: []string{"data"},
//	}
//	opts := make(map[string]string)
//	pool1 := storage.NewStoragePool(nil, "pool1")
//	d.physicalPools = map[string]storage.Pool{"pool1": pool1}
//
//	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
//	mockAPI.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx,
//		gomock.Any(), false).Return(fmt.Errorf("failed to disable snapshot directory access"))
//	mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true).Return(fmt.Errorf("failed to destroy volume"))
//
//	volName, err := d.createFlexvolForLUN(ctx, vol, opts, pool1)
//
//	assert.Error(t, err)
//	assert.Equal(t, "", volName)
// }

func TestOntapSanEconomyGetSnapshotEconomy_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "lunName",
		VolumeName:         "volumeName",
		Name:               "lunName",
		VolumeInternalName: "volumeName",
	}

	tests := []struct {
		message     string
		expectError bool
	}{
		{"error fetching info", true},
		{"no luns found", false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if test.expectError {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf(test.message))
			} else {
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, nil)
			}

			snap, err := d.getSnapshotEconomy(ctx, snapConfig, &d.Config)

			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, snap)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, snap)
			}
		})
	}
}

func TestOntapSanEconomyGetSnapshotEconomy_LunGetByNameFailed(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "lunName",
		VolumeName:         "volumeName",
		Name:               "lunName",
		VolumeInternalName: "volumeName",
	}
	luns := []api.Lun{
		{Size: "1073741824", Name: "lun_storagePrefix_volumeName_snapshot_lunName", VolumeName: "volumeName"},
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil, fmt.Errorf("failed to get lun by name"))

	snap, err := d.getSnapshotEconomy(ctx, snapConfig, &d.Config)

	assert.Error(t, err)
	assert.Nil(t, snap)
}

func TestOntapSanEconomyGetSnapshotEconomy_InvalidSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "lunName",
		VolumeName:         "volumeName",
		Name:               "lunName",
		VolumeInternalName: "volumeName",
	}
	lun := api.Lun{
		Size: "invalid", Name: "lun_storagePrefix_volumeName_snapshot_lunName",
		VolumeName: "volumeName",
	}
	luns := []api.Lun{lun}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(&lun, nil)

	snap, err := d.getSnapshotEconomy(ctx, snapConfig, &d.Config)

	assert.Error(t, err)
	assert.Nil(t, snap)
}

func TestOntapSanEconomyGetLUNSize(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)

	tests := []struct {
		message string
		size    string
	}{
		{"zero size", "0"},
		{"invalid", "invalid"},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: test.size}, nil)

			size, err := d.getLUNSize(ctx, "lun", "vol1")

			assert.Equal(t, uint64(0), size)
			assert.Error(t, err)
		})
	}
}

func TestOntapSanEconomyCreatePrepare(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
		InternalName: "vol1",
	}
	pool := storage.NewStoragePool(nil, "dummyPool")

	d.CreatePrepare(ctx, volConfig, pool)

	assert.NotEqual(t, "", volConfig.InternalName)
}

func TestOntapSanEconomyVolumeCreateFollowup(t *testing.T) {
	ctx := context.Background()
	_, driver := newMockOntapSanEcoDriver(t)
	volConfig := &storage.VolumeConfig{
		Name:         "testVolume",
		InternalName: "trident_lun_pool_trident_1234",
	}
	driver.CreateFollowup(ctx, volConfig)
}

func TestOntapSanEconomyCreatePrepare_StoragePoolUnset(t *testing.T) {
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	_, d := newMockOntapSanEcoDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:             "1g",
		Encryption:       "false",
		FileSystem:       "xfs",
		InternalName:     "vol1",
		ImportNotManaged: false,
	}
	pool := storage.NewStoragePool(nil, "")
	pool.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." + "RequestName}}"

	d.physicalPools = map[string]storage.Pool{"pool1": pool}

	d.CreatePrepare(ctx, volConfig, pool)

	assert.NotEqual(t, "", volConfig.InternalName)
}

func TestOntapSanEconomyCreatePrepare_NilPool(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()

	d.Config.NameTemplate = `{{.volume.Name}}_{{.volume.Namespace}}_{{.volume.StorageClass}}`
	volConfig := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	d.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, d, d.getStoragePoolAttributes(), d.BackendName())

	d.CreatePrepare(ctx, &volConfig, nil)
	assert.Equal(t, volConfig.InternalName, "newVolume_testNamespace_testSC", "volume name is not set correctly")
}

func TestOntapSanEconomyCreatePrepare_NilPool_templateNotContainVolumeName(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()

	d.Config.NameTemplate = `{{.volume.Namespace}}_{{.volume.StorageClass}}_{{slice .volume.Name 4 9}}`
	volConfig := storage.VolumeConfig{Name: "pvc-1234567", Namespace: "testNamespace", StorageClass: "testSC"}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	d.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, d, d.getStoragePoolAttributes(), d.BackendName())

	d.CreatePrepare(ctx, &volConfig, nil)
	assert.Equal(t, volConfig.InternalName, "testNamespace_testSC_12345", "volume name is not set correctly")
}

func TestOntapSanEconomyGetCommonConfig(t *testing.T) {
	_, d := newMockOntapSanEcoDriver(t)

	config := d.GetCommonConfig(ctx)

	assert.NotNil(t, config)
}

func TestOntapSanEconomyEnablePublishEnforcement_FailsCheckingIfLUNExists(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
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
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf("ontap api error"))

	err := d.EnablePublishEnforcement(ctx, volume)
	assert.Error(t, err)
	assert.False(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.NotEqual(t, -1, volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestOntapSanEconomyEnablePublishEnforcement_LUNDoesNotExist(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
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

	// Add LUNs that do not include the volume.
	luns := []api.Lun{
		{Size: "1073741824", Name: "/vol/myBucket/storagePrefix_vol1_snapshot_mySnap", VolumeName: "myLun"},
	}
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)

	err := d.EnablePublishEnforcement(ctx, volume)
	assert.Error(t, err)
	assert.False(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.NotEqual(t, -1, volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestOntapSanEconomyEnablePublishEnforcement_FailsToUnmapAllIgroups(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
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

	// Add LUNs that do include the volume.
	luns := []api.Lun{
		{
			Size:       "1073741824",
			Name:       fmt.Sprintf("/vol/myBucket/storagePrefix_vol1_%s", internalVolName),
			VolumeName: volName,
		},
	}
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return(nil, fmt.Errorf("ontap api error"))

	err := d.EnablePublishEnforcement(ctx, volume)
	assert.Error(t, err)
	assert.False(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.NotEqual(t, -1, volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestOntapSanEconomyEnablePublishEnforcement_EnablesAccessControl(t *testing.T) {
	mockAPI, d := newMockOntapSanEcoDriver(t)
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	volName := "websterj_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
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

	// Add LUNs that do include the volume.
	luns := []api.Lun{
		{
			Size:       "1073741824",
			Name:       fmt.Sprintf("/vol/myBucket/storagePrefix_vol1_%s", internalVolName),
			VolumeName: volName,
		},
	}
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(luns, nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, gomock.Any()).Return(nil, nil)

	err := d.EnablePublishEnforcement(ctx, volume)
	assert.NoError(t, err)
	assert.True(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.Equal(t, int32(-1), volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestSANEconomyStorageDriverGetBackendState(t *testing.T) {
	mockApi, mockDriver := newMockOntapSanEcoDriver(t)

	mockApi.EXPECT().GetSVMState(ctx).Return("", fmt.Errorf("returning test error"))

	reason, changeMap := mockDriver.GetBackendState(ctx)
	assert.Equal(t, reason, StateReasonSVMUnreachable, "should be 'SVM is not reachable'")
	assert.NotNil(t, changeMap, "should not be nil")
}
