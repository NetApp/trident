// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/google/uuid"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

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

func newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName string) *SANEconomyStorageDriver {
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

	sanEcoDriver := &SANEconomyStorageDriver{}
	sanEcoDriver.Config = *config

	// ClientConfig holds the configuration data for Client objects
	clientConfig := api.ClientConfig{
		ManagementLIF:           config.ManagementLIF,
		Username:                "client_username",
		Password:                "client_password",
		DriverContext:           tridentconfig.ContextCSI,
		ContextBasedZapiRecords: 100,
		DebugTraceFlags:         config.CommonStorageDriverConfig.DebugTraceFlags,
	}

	sanEcoDriver.API = api.NewClient(clientConfig, "SVM1")
	sanEcoDriver.telemetry = &Telemetry{
		Plugin:        sanEcoDriver.Name(),
		SVM:           sanEcoDriver.API.SVMName(),
		StoragePrefix: *sanEcoDriver.GetConfig().StoragePrefix,
		Driver:        sanEcoDriver,
		done:          make(chan struct{}),
	}

	return sanEcoDriver
}

func TestOntapSanEcoStorageDriverConfigString(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	sanEcoDrivers := []SANEconomyStorageDriver{
		*newTestOntapSanEcoDriver(vserverAdminHost, vserverAdminPort, vserverAggrName),
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

			sanEcoStorageDriver := newTestOntapSanEcoDriver(vserverAdminHost, port, vserverAggrName)
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

			sanEcoStorageDriver := newTestOntapSanEcoDriver(vserverAdminHost, port, vserverAggrName)
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
			name: "driverInitialized", fields: fields{
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
			}, args: args{
				in0: nil,
				in1: "volume",
				in2: "node",
			}, want: &utils.IscsiChapInfo{
				UseCHAP:              true,
				IscsiUsername:        "foo",
				IscsiInitiatorSecret: "bar",
				IscsiTargetUsername:  "baz",
				IscsiTargetSecret:    "biz",
			},
		},
		{
			name: "driverUninitialized", fields: fields{
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
			}, args: args{
				in0: nil,
				in1: "volume",
				in2: "node",
			}, want: &utils.IscsiChapInfo{
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
