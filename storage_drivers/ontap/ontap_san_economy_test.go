// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/RoaringBitmap/roaring"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// ToStringPointer takes a string and returns a string pointer
func ToStringPointer(s string) *string {
	return &s
}

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
		log.Errorf("could not decode JSON configuration: %v", err)
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

	snapPathPattern := helper.GetSnapPathPattern("my-Bucket")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_*_snapshot_*", snapPathPattern, "Strings not equal")

	snapPathPatternForVolume := helper.GetSnapPathPatternForVolume("my-Vol")
	assert.Equal(t, "/vol/*/storagePrefix_my_Vol_snapshot_*", snapPathPatternForVolume, "Strings not equal")

	snapPath := helper.GetSnapPath("my-Bucket", "storagePrefix_my-Lun", "snap-1")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_my_Lun_snapshot_snap_1", snapPath, "Strings not equal")

	snapName1 := helper.GetSnapshotName("storagePrefix_my-Lun", "my-Snapshot")
	assert.Equal(t, "storagePrefix_my_Lun_snapshot_my_Snapshot", snapName1, "Strings not equal")

	snapName2 := helper.GetSnapshotName("my-Lun", "snapshot-123")
	assert.Equal(t, "my_Lun_snapshot_snapshot_123", snapName2, "Strings not equal")

	internalVolName := helper.GetInternalVolumeName("my-Lun")
	assert.Equal(t, "storagePrefix_my_Lun", internalVolName, "Strings not equal")

	lunPath := helper.GetLUNPath("my-Bucket", "my-Lun")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_my_Lun", lunPath, "Strings not equal")

	lunName := helper.GetInternalVolumeNameFromPath(lunPath)
	assert.Equal(t, "storagePrefix_my_Lun", lunName, "Strings not equal")

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

func newTestOntapSanEcoDriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, useREST bool, apiOverride api.OntapAPI,
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
	config.UseREST = useREST

	sanEcoDriver := &SANEconomyStorageDriver{}
	sanEcoDriver.Config = *config

	numRecords := api.DefaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		numRecords = api.MaxZapiRecords
	}

	var ontapAPI api.OntapAPI

	if apiOverride != nil {
		ontapAPI = apiOverride
	} else {
		if config.UseREST {
			ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
		} else {
			ontapAPI, _ = api.NewZAPIClientFromOntapConfig(context.TODO(), config, numRecords)
		}
	}

	sanEcoDriver.API = ontapAPI
	sanEcoDriver.telemetry = &Telemetry{
		Plugin:        sanEcoDriver.Name(),
		SVM:           sanEcoDriver.API.SVMName(),
		StoragePrefix: *sanEcoDriver.GetConfig().StoragePrefix,
		Driver:        sanEcoDriver,
	}

	return sanEcoDriver
}

func TestOntapSanEcoStorageDriverConfigString(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDrivers := []SANEconomyStorageDriver{
		*newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI),
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

func TestOntapSanEconomyReconcileNodeAccess(t *testing.T) {
	ctx := context.Background()

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
		nodes              []*utils.Node
		igroupFinalIQNs    []string
	}{
		// Add a backend
		{
			{
				igroupName:         "igroup1",
				igroupExistingIQNs: []string{},
				nodes: []*utils.Node{
					{
						Name: "node1",
						IQN:  "IQN1",
					},
					{
						Name: "node2",
						IQN:  "IQN2",
					},
				},
				igroupFinalIQNs: []string{"IQN1", "IQN2"},
			},
		},
		// 2 same cluster backends/ nodes unchanged - both current
		{
			{
				igroupName:         "igroup1",
				igroupExistingIQNs: []string{"IQN1", "IQN2"},
				nodes: []*utils.Node{
					{
						Name: "node1",
						IQN:  "IQN1",
					},
					{
						Name: "node2",
						IQN:  "IQN2",
					},
				},
				igroupFinalIQNs: []string{"IQN1", "IQN2"},
			},
			{
				igroupName:         "igroup2",
				igroupExistingIQNs: []string{"IQN3", "IQN4"},
				nodes: []*utils.Node{
					{
						Name: "node3",
						IQN:  "IQN3",
					},
					{
						Name: "node4",
						IQN:  "IQN4",
					},
				},
				igroupFinalIQNs: []string{"IQN3", "IQN4"},
			},
		},
		// 2 different cluster backends - add node
		{
			{
				igroupName:         "igroup1",
				igroupExistingIQNs: []string{"IQN1"},
				nodes: []*utils.Node{
					{
						Name: "node1",
						IQN:  "IQN1",
					},
					{
						Name: "node2",
						IQN:  "IQN2",
					},
				},
				igroupFinalIQNs: []string{"IQN1", "IQN2"},
			},
			{
				igroupName:         "igroup2",
				igroupExistingIQNs: []string{"IQN3", "IQN4"},
				nodes: []*utils.Node{
					{
						Name: "node3",
						IQN:  "IQN3",
					},
					{
						Name: "node4",
						IQN:  "IQN4",
					},
				},
				igroupFinalIQNs: []string{"IQN3", "IQN4"},
			},
		},
		// 2 different cluster backends - remove node
		{
			{
				igroupName:         "igroup1",
				igroupExistingIQNs: []string{"IQN1", "IQN2"},
				nodes: []*utils.Node{
					{
						Name: "node1",
						IQN:  "IQN1",
					},
				},
				igroupFinalIQNs: []string{"IQN1"},
			},
			{
				igroupName:         "igroup2",
				igroupExistingIQNs: []string{"IQN3", "IQN4"},
				nodes: []*utils.Node{
					{
						Name: "node3",
						IQN:  "IQN3",
					},
					{
						Name: "node4",
						IQN:  "IQN4",
					},
				},
				igroupFinalIQNs: []string{"IQN3", "IQN4"},
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

			sanEcoStorageDriver := newTestOntapSanEcoDriver(vserverAdminHost, port, vserverAggrName, false, nil)
			sanEcoStorageDriver.Config.IgroupName = driverInfo.igroupName
			ontapSanDrivers = append(ontapSanDrivers, *sanEcoStorageDriver)
		}

		for driverIndex, driverInfo := range testCase {
			ontapSanDrivers[driverIndex].ReconcileNodeAccess(ctx, driverInfo.nodes,
				uuid.New().String())
		}

		for _, driverInfo := range testCase {

			assert.Equal(t, len(driverInfo.igroupFinalIQNs), len(api.FakeIgroups[driverInfo.igroupName]))

			for _, iqn := range driverInfo.igroupFinalIQNs {
				assert.Contains(t, api.FakeIgroups[driverInfo.igroupName], iqn)
			}
		}
	}
}

func TestOntapSanEconomyTerminate(t *testing.T) {
	ctx := context.Background()

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

			sanEcoStorageDriver := newTestOntapSanEcoDriver(vserverAdminHost, port, vserverAggrName, false, nil)
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
		want   *utils.IscsiChapInfo
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
			}, want: &utils.IscsiChapInfo{UseCHAP: true, IscsiUsername: "foo", IscsiInitiatorSecret: "bar", IscsiTargetUsername: "baz", IscsiTargetSecret: "biz"},
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
			}, want: &utils.IscsiChapInfo{UseCHAP: true, IscsiUsername: "biz", IscsiInitiatorSecret: "baz", IscsiTargetUsername: "bar", IscsiTargetSecret: "foo"},
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
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, true, mockAPI)

	assert.True(t, reflect.DeepEqual(sanEcoDriver.GetAPI(), sanEcoDriver.API), "Incorrect API returned")
}

func TestGetTelemetry(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, true, mockAPI)
	sanEcoDriver.telemetry = &Telemetry{
		Plugin:        sanEcoDriver.Name(),
		SVM:           sanEcoDriver.GetConfig().SVM,
		StoragePrefix: *sanEcoDriver.GetConfig().StoragePrefix,
		Driver:        sanEcoDriver,
		done:          make(chan struct{}),
	}
	assert.True(t, reflect.DeepEqual(sanEcoDriver.telemetry, sanEcoDriver.GetTelemetry()), "Incorrect API returned")
}

func TestBackendNameUnset(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)

	sanEcoDriver.Config.BackendName = ""
	sanEcoDriver.ips = []string{"127.0.0.1"}
	assert.Equal(t, sanEcoDriver.BackendName(), "ontapsaneco_127.0.0.1", "Incorrect bucket name")
}

func TestBackendNameSet(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)

	sanEcoDriver.Config.BackendName = "ontapsaneco"
	sanEcoDriver.ips = []string{"127.0.0.1"}
	assert.Equal(t, sanEcoDriver.BackendName(), "ontapsaneco", "Incorrect backend name")
}

func TestDriverInitialized(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)

	sanEcoDriver.initialized = true
	assert.Equal(t, sanEcoDriver.Initialized(), sanEcoDriver.initialized, "Incorrect initialization status")

	sanEcoDriver.initialized = false
	assert.Equal(t, sanEcoDriver.Initialized(), sanEcoDriver.initialized, "Incorrect initialization status")
}

func TestOntapSanEconomyTerminateCSI(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.telemetry = nil
	d.API = mockAPI

	d.Config.DriverContext = tridentconfig.ContextCSI

	mockAPI.EXPECT().IgroupDestroy(ctx, gomock.Any()).Times(1).Return(nil)

	d.Terminate(ctx, "")
	assert.False(t, d.initialized)
}

func TestDriverValidate(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)

	assert.Nil(t, sanEcoDriver.validate(ctx), "San Economy driver validation failed")
}

func TestDriverIgnoresDataLIF(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)
	sanEcoDriver.Config.DataLIF = "foo"

	assert.NoError(t, sanEcoDriver.validate(ctx), "driver validation succeeded: data LIF is ignored")
}

func TestDriverValidateInvalidPrefix(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)
	sanEcoDriver.Config.StoragePrefix = ToStringPointer("B@D")

	assert.EqualError(t, sanEcoDriver.validate(ctx), "storage prefix may only contain letters/digits/underscore/dash")
}

func TestDriverValidateInvalidPools(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	sanEcoDriver := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SpaceReserve: "iaminvalid",
	})
	sanEcoDriver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	assert.EqualError(t, sanEcoDriver.validate(ctx),
		"storage pool validation failed: invalid spaceReserve iaminvalid in pool pool1")
}

func TestOntapSanEconomyVolumeCreate(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})
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
	mockAPI.EXPECT().VolumeDisableSnapshotDirectoryAccess(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Times(1).Return(nil)

	err := d.Create(ctx, volConfig, pool1, volAttrs)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyVolumeClone(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "lunName", VolumeName: "volumeName"}}, nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)

	err := d.CreateClone(ctx, volConfig, volConfig, pool1)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyVolumeImport(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}

	err := d.Import(ctx, volConfig, "volInternal")
	assert.EqualError(t, err, "import is not implemented")
}

func TestOntapSanEconomyVolumeRename(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	err := d.Rename(ctx, "volInternal", "newVolInternal")
	assert.EqualError(t, err, "rename is not implemented")
}

func TestOntapSanEconomyVolumeDestroy(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)

	err := d.Destroy(ctx, volConfig)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyVolumeDeleteBucketIfEmpty(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeDestroy(ctx, "volumeName", true)

	err := d.DeleteBucketIfEmpty(ctx, "volumeName")
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyVolumePublish(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}

	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	publishInfo := &utils.VolumePublishInfo{
		HostName:         "bar",
		HostIQN:          []string{"host_iqn"},
		TridentUUID:      "1234",
		VolumeAccessInfo: utils.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged:        false,
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "lunName", VolumeName: "volumeName"}}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetComment(ctx, "/vol/volumeName/storagePrefix_lunName")
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"1.1.1.1"}, nil)

	err := d.Publish(ctx, volConfig, publishInfo)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyVolumePublishSLMError(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}

	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	publishInfo := &utils.VolumePublishInfo{
		HostName:         "bar",
		HostIQN:          []string{"host_iqn"},
		TridentUUID:      "1234",
		VolumeAccessInfo: utils.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged:        false,
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "lunName", VolumeName: "volumeName"}}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetComment(ctx, "/vol/volumeName/storagePrefix_lunName")
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{}, nil)

	err := d.Publish(ctx, volConfig, publishInfo)
	assert.Errorf(t, err, "no reporting data LIFs found")
}

func TestDriverCanSnapshot(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI)

	err := d.CanSnapshot(ctx, nil, nil)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyGetSnapshot(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "lunName",
		VolumeName:         "volumeName",
		Name:               "lunName",
		VolumeInternalName: "volumeName",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1g", Name: "volumeName_snapshot_lunName", VolumeName: "volumeName"}},
		nil)
	mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824"}, nil)

	snap, err := d.GetSnapshot(ctx, snapConfig, nil)
	assert.Nil(t, err, "Error is not nil")
	assert.NotNil(t, snap, "snapshot is nil")
}

func TestOntapSanEconomyGetSnapshots(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.ips = []string{"127.0.0.1"}

	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1073741824", Name: "volumeName_snapshot_lunName", VolumeName: "volumeName"}},
		nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/volumeName/storagePrefix_lunName_snapshot_mySnap", VolumeName: "volumeName"}},
		nil)

	snaps, err := d.GetSnapshots(ctx, volConfig)
	assert.Nil(t, err, "Error is not nil")
	assert.NotNil(t, snaps, "snapshots are nil")
}

func TestOntapSanEconomyCreateSnapshot(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)
	d.flexvolNamePrefix = "storagePrefix"

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap_1",
		VolumeName:         "my_Bucket",
		Name:               "/vol/my_Bucket/storagePrefix_my_Lun",
		VolumeInternalName: "storagePrefix_my_Lun_my_Bucket",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(2).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"}},
		nil)
	mockAPI.EXPECT().LunGetByName(ctx,
		gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"},
		nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(2).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"}},
		nil)
	mockAPI.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_snapshot_snap_1", VolumeName: "my_Bucket"}},
		nil)

	snap, err := d.CreateSnapshot(ctx, snapConfig, nil)
	assert.Nil(t, err, "Error is not nil")
	assert.NotNil(t, snap, "snapshots are nil")
}

func TestOntapSanEconomyRestoreSnapshot(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap_1",
		VolumeName:         "my_Bucket",
		Name:               "/vol/my_Bucket/storagePrefix_my_Lun",
		VolumeInternalName: "storagePrefix_my_Lun_my_Bucket",
	}

	err := d.RestoreSnapshot(ctx, snapConfig, nil)
	assert.EqualError(t, err, fmt.Sprintf("restoring snapshots is not supported by backend type %s", d.Name()))
}

func TestOntapSanEconomyVolumeDeleteSnapshot(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap_1",
		VolumeName:         "my_Bucket",
		Name:               "/vol/my_Bucket/storagePrefix_my_Lun",
		VolumeInternalName: "storagePrefix_my_Lun_my_Bucket",
	}
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(2).Return(api.Luns{api.Lun{Size: "1073741824", Name: "storagePrefix_my_Lun_my_Bucket_snapshot_snap_1", VolumeName: "my_Bucket"}},
		nil)
	mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Times(1)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{api.Lun{}}, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)

	err := d.DeleteSnapshot(ctx, snapConfig, nil)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyGet(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"}},
		nil)

	err := d.Get(ctx, "my_Bucket")
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyGetStorageBackendSpecs(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.ips = []string{"127.0.0.1"}
	d.API = mockAPI

	backend := storage.StorageBackend{}

	err := d.GetStorageBackendSpecs(ctx, &backend)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanEconomyGetStorageBackendPhysicalPoolNames(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	pool1 := storage.NewStoragePool(nil, "pool1")
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	poolNames := d.GetStorageBackendPhysicalPoolNames(ctx)
	assert.Equal(t, "pool1", poolNames[0], "Pool names are not equal")
}

func TestOntapSanEconomyGetInternalVolumeName(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.Config.StoragePrefix = ToStringPointer("storagePrefix_")

	internalVolName := d.GetInternalVolumeName(ctx, "my-Lun")
	assert.Equal(t, "storagePrefix_my_Lun", internalVolName, "Strings not equal")
}

func TestOntapSanEconomyGetProtocol(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	protocol := d.GetProtocol(ctx)
	assert.Equal(t, protocol, tridentconfig.Block, "Protocols not equal")
}

func TestOntapSanEconomyStoreConfig(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

	persistentConfig := &storage.PersistentStorageBackendConfig{}

	d.StoreConfig(ctx, persistentConfig)

	assert.Equal(t, json.RawMessage("{}"), d.Config.CommonStorageDriverConfig.StoragePrefixRaw,
		"raw prefix mismatch")
	assert.Equal(t, d.Config, *persistentConfig.OntapConfig, "ontap config mismatch")
}

func TestGetVolumeExternal(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Times(1).Return(api.Luns{}, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunGetByName(ctx,
		gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"},
		nil)

	result, resultErr := d.GetVolumeExternal(ctx, "my_Lun")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "my_Lun_my_Bucket", result.Config.Name)
	assert.Equal(t, "storagePrefix_my_Lun_my_Bucket", result.Config.InternalName)
}

func TestGetVolumeExternalWrappers(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
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

func TestGetUpdateType_OtherChanges(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	oldDriver := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	oldDriver.API = mockAPI
	prefix1 := "storagePrefix_"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	newDriver := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	oldDriver.API = mockAPI
	prefix2 := "storagePREFIX_"

	newDriver.Config.StoragePrefix = &prefix2
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestOntapSanEconomyVolumeResize(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSanEcoDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	volConfig := &storage.VolumeConfig{
		Size:       "1073741824",
		Encryption: "false",
		FileSystem: "xfs",
	}

	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(2).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"}},
		nil)
	mockAPI.EXPECT().LunGetByName(ctx,
		gomock.Any()).Times(1).Return(&api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"},
		nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{}, nil)
	mockAPI.EXPECT().LunList(ctx,
		gomock.Any()).Times(1).Return(api.Luns{api.Lun{Size: "1073741824", Name: "/vol/my_Bucket/storagePrefix_my_Lun_my_Bucket", VolumeName: "my_Bucket"}},
		nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{Aggregates: []string{"data"}}, nil)
	mockAPI.EXPECT().SupportsFeature(ctx, api.LunGeometrySkip).Times(1)
	mockAPI.EXPECT().LunGetGeometry(ctx, gomock.Any()).Times(1).Return(uint64(2147483648), nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockAPI.EXPECT().LunSetSize(ctx, gomock.Any(), "2147483648").Return(uint64(2147483648), nil)

	err := d.Resize(ctx, volConfig, uint64(2147483648))
	assert.Nil(t, err, "Error is not nil")
}
