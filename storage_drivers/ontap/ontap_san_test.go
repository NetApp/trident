// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

func getCommonConfig() *drivers.CommonStorageDriverConfig {
	return &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-san",
		BackendName:       "myOntapSANBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
		StoragePrefix:     convert.ToPtr("trident_"),
	}
}

func getVolumeConfig() storage.VolumeConfig {
	return storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
		Name:         "vol1",
		InternalName: "trident-pvc-1234",
	}
}

func newMockAWSOntapSANDriver(t *testing.T) (*mockapi.MockOntapAPI, *mockapi.MockAWSAPI, *SANStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	fsxId := FSX_ID
	driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, &fsxId, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}

	driver.AWSAPI = mockAWSAPI
	return mockAPI, mockAWSAPI, driver
}

func newMockOntapSANDriver(t *testing.T) (*mockapi.MockOntapAPI, *SANStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()

	driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}

	iscsiClient, err := iscsi.New()
	assert.NoError(t, err)
	driver.iscsi = iscsiClient
	driver.cloneSplitTimers = &sync.Map{}

	return mockAPI, driver
}

func TestOntapSanStorageDriverConfigString(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	ontapSanDrivers := []SANStorageDriver{
		*newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, true, nil, mockAPI),
		*newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, nil, mockAPI),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-san-user",
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

	for _, ontapSanDriver := range ontapSanDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, ontapSanDriver.String(), val, "ontap-san driver does not contain %v", key)
			assert.Contains(t, ontapSanDriver.GoString(), val, "ontap-san driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, ontapSanDriver.String(), val, "ontap-san driver contains %v", key)
			assert.NotContains(t, ontapSanDriver.GoString(), val, "ontap-san driver contains %v", key)
		}
	}
}

func newTestOntapSANDriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, useREST bool, fsxId *string, apiOverride api.OntapAPI,
) *SANStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.CommonStorageDriverConfig.DebugTraceFlags["api"] = true
	// config.Labels = map[string]string{"app": "wordpress"}
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-san-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san"
	config.StoragePrefix = sp("test_")
	config.UseREST = &useREST

	if fsxId != nil {
		config.AWSConfig = &drivers.AWSConfig{}
		config.AWSConfig.FSxFilesystemID = *fsxId
	}

	sanDriver := &SANStorageDriver{}
	sanDriver.Config = *config

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

	sanDriver.API = ontapAPI
	sanDriver.telemetry = &Telemetry{
		Plugin:        sanDriver.Name(),
		StoragePrefix: *sanDriver.Config.StoragePrefix,
		Driver:        sanDriver,
	}

	return sanDriver
}

func TestOntapSanTerminate(t *testing.T) {
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

		var ontapSanDrivers []SANStorageDriver

		for _, driverInfo := range testCase {

			// simulate existing IQNs on the vserver
			igroupsIQNMap := map[string]struct{}{}
			for _, iqn := range driverInfo.igroupExistingIQNs {
				igroupsIQNMap[iqn] = struct{}{}
			}

			api.FakeIgroups[driverInfo.igroupName] = igroupsIQNMap

			sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, port, vserverAggrName, false, nil, nil)
			sanStorageDriver.Config.IgroupName = driverInfo.igroupName
			sanStorageDriver.telemetry = nil
			ontapSanDrivers = append(ontapSanDrivers, *sanStorageDriver)
		}

		for driverIndex, driverInfo := range testCase {
			ontapSanDrivers[driverIndex].Terminate(ctx, "")
			assert.NotContains(t, api.FakeIgroups, api.FakeIgroups[driverInfo.igroupName])
		}

	}
}

func TestOntapSANStorageDriverTerminate_TelemetryFailure(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.IgroupName = "igroup1"
	driver.Config.DriverContext = tridentconfig.ContextCSI
	driver.telemetry = &Telemetry{
		Plugin:        driver.Name(),
		SVM:           "SVM1",
		StoragePrefix: *driver.Config.StoragePrefix,
		Driver:        driver,
		done:          make(chan struct{}),
	}
	driver.initialized = true

	mockAPI.EXPECT().IgroupDestroy(ctx, "igroup1").Return(fmt.Errorf("igroup not found"))

	driver.Terminate(ctx, "dummy")

	assert.False(t, driver.initialized)
}

func expectLunAndVolumeCreateSequence(ctx context.Context, mockAPI *mockapi.MockOntapAPI, fsType, luks string) {
	// expected call sequenece is:
	//   check the volume exists (should return false)
	//   create the volume
	//   create the LUN
	//   set attributes on the LUN

	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volumeName string) (bool, error) {
			return false, nil
		},
	).MaxTimes(1)

	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volume api.Volume) error {
			return nil
		},
	).MaxTimes(1)

	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, lun api.Lun) error {
			return nil
		},
	).MaxTimes(1)

	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), fsType, gomock.Any(), luks, gomock.Any()).DoAndReturn(
		func(ctx context.Context, lunPath, attribute, fstype, context, luks, formatOptions string) error {
			return nil
		},
	).MaxTimes(1)
}

func TestOntapSanVolumeCreate(t *testing.T) {
	ctx := context.Background()
	mockAPI, d := newMockOntapSANDriver(t)

	luks := "true"
	expectLunAndVolumeCreateSequence(ctx, mockAPI, "xfs", luks)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
		SkipRecoveryQueue: "true",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    luks,
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}

	err := d.Create(ctx, &volConfig, pool1, volAttrs)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "fake-export-policy", volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "true", volConfig.SkipRecoveryQueue)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
	assert.Equal(t, "true", volConfig.LUKSEncryption)
	assert.Equal(t, "xfs", volConfig.FileSystem)
}

func TestOntapSanVolumeCreate_InvalidSkipRecoveryQueue(t *testing.T) {
	ctx := context.Background()
	mockAPI, d := newMockOntapSANDriver(t)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volumeName string) (bool, error) {
			return false, nil
		},
	).MaxTimes(1)
	mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SkipRecoveryQueue: "asdf",
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}

	err := d.Create(ctx, &volConfig, pool1, volAttrs)

	assert.Error(t, err, "Expected error for invalid skipRecoveryQueue value")
}

func TestGetChapInfo(t *testing.T) {
	type fields struct {
		initialized   bool
		Config        drivers.OntapStorageDriverConfig
		ips           []string
		API           api.OntapAPI
		telemetry     *Telemetry
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
				API:           nil,
				telemetry:     nil,
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
				API:           nil,
				telemetry:     nil,
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
			d := &SANStorageDriver{
				initialized:   tt.fields.initialized,
				Config:        tt.fields.Config,
				ips:           tt.fields.ips,
				API:           tt.fields.API,
				telemetry:     tt.fields.telemetry,
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

func TestOntapSanUnpublish(t *testing.T) {
	ctx := context.Background()
	originalContext := tridentconfig.CurrentDriverContext
	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	defer func() { tridentconfig.CurrentDriverContext = originalContext }()

	type args struct {
		publishEnforcement bool
	}

	tt := []struct {
		name    string
		args    args
		mocks   func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "LegacyVolume",
			args: args{publishEnforcement: false},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName)
				mockAPI.EXPECT().IgroupDestroy(ctx, igroupName)
			},
			wantErr: assert.NoError,
		},
		{
			name: "LastLunOnIgroup",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(1, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return([]string{}, nil) // Return 0 LUNs
				mockAPI.EXPECT().IgroupDestroy(ctx,
					igroupName).Return(nil) // iGroup should be deleted
			},
			wantErr: assert.NoError,
		},
		{
			name: "NotLastLunOnIgroup",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(1, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return([]string{"/vol/v/l"}, nil) // Return 1 LUN
				// iGroup should not be deleted
			},
			wantErr: assert.NoError,
		},
		{
			name: "LunAlreadyUnmapped",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(-1, nil)          // -1 indicates not mapped
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return([]string{}, nil) // Return 0 LUNs
				mockAPI.EXPECT().IgroupDestroy(ctx,
					igroupName).Return(nil) // iGroup should be deleted
			},
			wantErr: assert.NoError,
		},
		{
			name: "LunMapInfoApiFailure",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(-1, fmt.Errorf("some api error"))
			},
			wantErr: assert.Error,
		},
		{
			name: "LunUnmapApiFailure",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(1, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(fmt.Errorf("some api error"))
			},
			wantErr: assert.Error,
		},
		{
			name: "IgroupListLUNsMappedApiFailure",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(1, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return([]string{}, fmt.Errorf("some api error"))
			},
			wantErr: assert.Error,
		},
		{
			name: "IgroupDestroyApiFailure",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				mockAPI.EXPECT().LunMapInfo(ctx, igroupName, lunPath).Return(1, nil)
				mockAPI.EXPECT().LunUnmap(ctx, igroupName, lunPath).Return(nil)
				mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName).Return([]string{}, nil)
				mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(fmt.Errorf("some api error"))
			},
			wantErr: assert.Error,
		},
		{
			name: "CurrentDriverContextNotCSI",
			args: args{publishEnforcement: true},
			mocks: func(mockAPI *mockapi.MockOntapAPI, igroupName, lunPath string) {
				tridentconfig.CurrentDriverContext = tridentconfig.ContextDocker
			},
			wantErr: assert.NoError,
		},
	}
	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				InternalName: "foo",
				AccessInfo:   models.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			publishInfo := &models.VolumePublishInfo{
				HostName:         "bar",
				TridentUUID:      "1234",
				VolumeAccessInfo: models.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
			lunPath := lunPath(volConfig.InternalName)

			mockCtrl := gomock.NewController(t)
			mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

			mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
				gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

			d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
			d.API = mockAPI

			tr.mocks(mockAPI, igroupName, lunPath)

			err := d.Unpublish(ctx, volConfig, publishInfo)
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}
		})
	}
}

func TestOntapSanVolumePublishManaged(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}
	d.Config.SANType = sa.ISCSI

	volConfig := getVolumeConfig()
	volConfig.InternalName = "lunName"

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

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().LunGetAttribute(ctx, "/vol/lunName/lun0", "formatOptions")
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/lunName/lun0").Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"1.1.1.1"}, nil)

	err := d.Publish(ctx, &volConfig, publishInfo)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanVolumePublishUnmanaged(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}
	d.Config.SANType = sa.ISCSI

	volConfig := getVolumeConfig()
	volConfig.InternalName = "lunName"

	publishInfo := &models.VolumePublishInfo{
		HostName:    "bar",
		HostIQN:     []string{"host_iqn"},
		TridentUUID: "1234",
		// VolumeAccessInfo: utils.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged: true,
	}

	dummyLun := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().LunGetAttribute(ctx, "/vol/lunName/lun0", "formatOptions")
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/lunName/lun0").Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"1.1.1.1"}, nil)

	err := d.Publish(ctx, &volConfig, publishInfo)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanVolumePublishSLMError(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}
	d.Config.SANType = sa.ISCSI

	volConfig := getVolumeConfig()
	volConfig.InternalName = "lunName"

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

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().LunGetAttribute(ctx, "/vol/lunName/lun0", "formatOptions")
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/lunName/lun0").Return(dummyLun, nil)
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{}, nil)

	err := d.Publish(ctx, &volConfig, publishInfo)
	assert.Errorf(t, err, "no reporting nodes found")
}

func TestSANStorageDriverGetBackendState(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	mockDriver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	mockDriver.API = mockAPI

	mockAPI.EXPECT().GetSVMState(ctx).Return("", fmt.Errorf("returning test error"))

	reason, changeMap := mockDriver.GetBackendState(ctx)
	assert.Equal(t, reason, StateReasonSVMUnreachable, "should be 'SVM is not reachable'")
	assert.NotNil(t, changeMap, "should not be nil")
}

func TestOntapSanVolumeCreate_LabelLengthExceeding(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)

	luks := "true"
	expectLunAndVolumeCreateSequence(ctx, mockAPI, "xfs", luks)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
		LUKSEncryption:    luks,
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

	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}

	err := driver.Create(ctx, &volConfig, pool1, volAttrs)

	assert.Error(t, err, "Error is nil")
}

func TestOntapSanVolume_DestroyVolumeIfNoLUN(t *testing.T) {
	ctx = context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)
	volConfig := getVolumeConfig()

	dummyLun := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		volExists     bool
		assertMessage string // This message prints when the test case fails
	}{
		{
			name: "VolumeExists_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false,
					fmt.Errorf("volume checks fail"))
			},
			wantErr:       assert.Error,
			volExists:     false,
			assertMessage: "Checking for volume succeeded.",
		},
		{
			name: "VolumeDoesNotExist",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.NoError,
			volExists:     false,
			assertMessage: "Volume existed.",
		},
		{
			name: "LUNExists",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(dummyLun, nil)
			},
			wantErr:       assert.NoError,
			volExists:     true,
			assertMessage: "LUN does not exist",
		},
		{
			name: "LUNFindError",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil, nil)
			},
			wantErr:       assert.Error,
			volExists:     false,
			assertMessage: "LUN is found.",
		},
		{
			name: "LUNDoesNotExist",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil,
					errors.NotFoundError("LUN does not exist"))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, true).Return(errors.New("destroy fail"))
			},
			wantErr:       assert.Error,
			volExists:     true,
			assertMessage: "Successful volume cleanup.",
		},
		{
			name: "LUNDoesNotExist_VolDestroy",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(nil,
					errors.NotFoundError("LUN does not exist"))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), true, true).Return(nil)
			},
			wantErr:       assert.NoError,
			volExists:     false,
			assertMessage: "Failed volume cleanup.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)
			volExists, err := driver.destroyVolumeIfNoLUN(ctx, &volConfig)
			assert.Equal(t, test.volExists, volExists, "volume exist status is not expected.")
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}

	// Mirrored configuration coverage
	volConfig.IsMirrorDestination = true
	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
	t.Run("mirrored configuration", func(t *testing.T) {
		volExists, err := driver.destroyVolumeIfNoLUN(ctx, &volConfig)
		assert.True(t, volExists, "volume does not exist")
		assert.NoError(t, err, "volume exist check return error")
	})
}

func TestOntapSanVolumeCreate_ValidationFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}
	dummyLun := &api.Lun{
		Comment:      "dummyLun",
		SerialNumber: "testSerialNumber",
	}

	type args struct {
		snapshotReserve   string
		encryption        string
		PeerVolumeHandle  string
		FileSystem        string
		QosPolicy         string
		AdaptiveQosPolicy string
		LimitVolumeSize   string
	}

	tests := []struct {
		name          string
		arg           args
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string // This message prints when the test case fails
	}{
		{
			name: "VolumeExists_Fail",
			arg: args{
				snapshotReserve:  "10",
				encryption:       "false",
				PeerVolumeHandle: "",
				FileSystem:       "xfs",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false,
					fmt.Errorf("volume checks fail"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is not present in backend",
		},
		{
			name: "Volume_Found",
			arg: args{
				snapshotReserve:  "10",
				encryption:       "false",
				PeerVolumeHandle: "",
				FileSystem:       "xfs",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunGetByName(ctx, gomock.Any()).Return(dummyLun, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is not present in backend",
		},
		{
			name: "SnapshotReserve_Fail",
			arg: args{
				snapshotReserve:  "InvalidValue",
				encryption:       "false",
				PeerVolumeHandle: "",
				FileSystem:       "xfs",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "SnapshotReserve validation passed",
		},
		{
			name: "Encryption_fail",
			arg: args{
				snapshotReserve:  "10",
				encryption:       "InvalidValue",
				PeerVolumeHandle: "",
				FileSystem:       "xfs",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Encryption validation pass",
		},
		{
			name: "PeerVolumeHandle",
			arg: args{
				snapshotReserve:  "10",
				encryption:       "false",
				PeerVolumeHandle: "svm2:vol1",
				FileSystem:       "xfs",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().GetSVMPeers(ctx).Return([]string{"InvalidSVM"}, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Get the peer storage pool and volume information",
		},
		{
			name: "CheckSupportedFilesystem",
			arg: args{
				snapshotReserve:  "10",
				encryption:       "false",
				PeerVolumeHandle: "",
				FileSystem:       "InvalidFilesystem",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			},
			wantErr:       assert.Error,
			assertMessage: "File system type is valid",
		},
		{
			name: "QosPolicy",
			arg: args{
				snapshotReserve:   "10",
				encryption:        "false",
				PeerVolumeHandle:  "",
				FileSystem:        "xfs",
				QosPolicy:         "fake-qos-policy",
				AdaptiveQosPolicy: "fake-adaptive-policy",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
			},
			wantErr:       assert.Error,
			assertMessage: "QosPolicy is valid",
		},
		{
			name: "getAggregates_fail",
			arg: args{
				snapshotReserve:   "10",
				encryption:        "false",
				PeerVolumeHandle:  "",
				FileSystem:        "xfs",
				QosPolicy:         "fake-qos-policy",
				AdaptiveQosPolicy: "",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().GetSVMAggregateSpace(ctx, "pool1").Return(nil, fmt.Errorf("aggregate not found"))
			},
			wantErr:       assert.Error,
			assertMessage: "Get the SVM aggregates",
		},
		{
			name: "InvalidVolumeSizeLimits",
			arg: args{
				snapshotReserve:   "10",
				encryption:        "false",
				PeerVolumeHandle:  "",
				FileSystem:        "xfs",
				QosPolicy:         "fake-qos-policy",
				AdaptiveQosPolicy: "",
				LimitVolumeSize:   "InvalidSize",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Volume size limit validation has passed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool1 := storage.NewStoragePool(nil, "pool1")
			pool1.SetInternalAttributes(map[string]string{
				SnapshotPolicy:    "fake-snap-policy",
				SnapshotReserve:   test.arg.snapshotReserve,
				QosPolicy:         test.arg.QosPolicy,
				AdaptiveQosPolicy: test.arg.AdaptiveQosPolicy,
			})
			driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
			volConfig.Encryption = test.arg.encryption
			volConfig.PeerVolumeHandle = test.arg.PeerVolumeHandle
			volConfig.FileSystem = test.arg.FileSystem
			driver.Config.LimitAggregateUsage = "10"
			driver.Config.CommonStorageDriverConfig.LimitVolumeSize = test.arg.LimitVolumeSize

			test.mocks(mockAPI)

			err := driver.Create(ctx, &volConfig, pool1, volAttrs)
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeCreate_GetPool(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool2 := storage.NewStoragePool(nil, "pool2")
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.virtualPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}

	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volumeName string) (bool, error) {
			return false, nil
		},
	).MaxTimes(1)

	err := driver.Create(ctx, &volConfig, pool2, volAttrs)

	assert.Error(t, err, "Storage pool is present in backend")
}

func TestOntapSanVolumeCreate_VolumeCreateFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string // This message prints when the test case fails
	}{
		{
			name: "volumeCreateFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(fmt.Errorf("volume creation failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is created",
		},
		{
			name: "volumeCreateJobFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(
					api.VolumeCreateJobExistsError("Volume creation failed"))
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(fmt.Errorf("lun creation failed"))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is created",
		},
		{
			name: "LunCreateFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(fmt.Errorf("lun creation failed"))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr:       assert.Error,
			assertMessage: "LUN is created",
		},
		{
			name: "LunCreateFail_volumeDestroyFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(fmt.Errorf("lun creation failed"))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("volume destroy failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "LUN creation failed and respective volume deleted",
		},
		{
			name: "UpdateLunAttributeFailed",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to set LUN attribute"))
				mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr:       assert.Error,
			assertMessage: "LUN attributes are updated",
		},
		{
			name: "LunCreateFail_LunDestroyFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to set LUN attribute"))
				mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(fmt.Errorf("LUN destroy failed"))
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Lun deletion failed",
		},
		{
			name: "LunCreateFail_VolumeDestroyFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
				mockAPI.EXPECT().TieringPolicyValue(ctx).Return("fake-tier-policy")
				mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to set LUN attribute"))
				mockAPI.EXPECT().LunDestroy(ctx, gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("volume destroy failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume deletion failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Create(ctx, &volConfig, pool1, volAttrs)

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeCreate_FormatOptions(t *testing.T) {
	ctx := context.Background()
	mockAPI, d := newMockOntapSANDriver(t)

	luks := "true"
	fsType := "xfs"

	// Setting up what formatOptions could look like.
	tempFormatOptions := strings.Join([]string{"-b 4096", "-T stirde=2056, stripe=16"},
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
		LUKSEncryption:    luks,
		FormatOptions:     tempFormatOptions,
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	// Setting up expected calls.
	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).Return(false, nil).MaxTimes(1)
	mockAPI.EXPECT().VolumeCreate(ctx, gomock.Any()).Return(nil).MaxTimes(1)
	mockAPI.EXPECT().LunCreate(ctx, gomock.Any()).Return(nil).MaxTimes(1)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	// This is the assertion of this unit test,
	// checking whether the argument FormatOptions matches with what we pass in the internal attributes.
	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), fsType, gomock.Any(), luks, tempFormatOptions).Return(nil).MaxTimes(1)

	volConfig := getVolumeConfig()
	volAttrs := map[string]sa.Request{}

	err := d.Create(ctx, &volConfig, pool1, volAttrs)

	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanVolumeCreate_InvalidVolumeSize(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	pool1 := storage.NewStoragePool(nil, "pool1")
	pool1.SetInternalAttributes(map[string]string{
		SnapshotPolicy:  "fake-snap-policy",
		SnapshotReserve: "10",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	tests := []struct {
		volumeSize string
	}{
		{"invalid"},
		{"-1002947563b"},
	}

	for _, test := range tests {
		t.Run(test.volumeSize, func(t *testing.T) {
			volConfig := getVolumeConfig()
			volConfig.Size = test.volumeSize
			volAttrs := map[string]sa.Request{}

			mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
				func(ctx context.Context, volumeName string) (bool, error) {
					return false, nil
				},
			).MaxTimes(1)

			err := driver.Create(ctx, &volConfig, pool1, volAttrs)
			assert.Error(t, err, "Test has passed with invalid Volume size. Expected to fail")
		})
	}
}

func TestOntapSanVolumeClone(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)
	luks := "true"
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
		LUKSEncryption:    luks,
		SplitOnClone:      "false",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	pool1.Attributes()["labels"] = sa.NewLabelOffer(map[string]string{
		"type": "clone",
	})

	driver.Config.Labels = pool1.GetLabels(ctx, "")

	volConfig := getVolumeConfig()
	volume := api.Volume{
		Name:    "vol1",
		Comment: "iscsi volume",
	}
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolume).Return(&volume, nil)
	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volumeName string) (bool, error) {
			return false, nil
		},
	).MaxTimes(1)

	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeWaitForStates(ctx, volConfig.InternalName, []string{"online"}, []string{"error"},
		maxFlexvolCloneWait).AnyTimes().Return("online", nil)
	mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := driver.CreateClone(ctx, &volConfig, &volConfig, pool1)

	assert.Empty(t, volConfig.CloneSourceVolume, "Clone source volume name is not empty")
	assert.NotEmpty(t, volConfig.CloneSourceSnapshotInternal, "Clone source snapshot internal name is empty")

	assert.NoError(t, err, "Clone creation failed. Expected no error")
}

func TestOntapSanVolumeClone_VolumeInfoFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	pool1 := storage.NewStoragePool(nil, "pool1")

	volConfig := &storage.VolumeConfig{
		Size:                      "1g",
		CloneSourceVolumeInternal: "trident-pvc-1234",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolumeInternal).Return(nil,
					fmt.Errorf("volume check fail"))
			},
			wantErr:       assert.Error,
			assertMessage: "Get the volume info from the backend",
		},
		{
			name: "VolumeInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolumeInternal).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Get the volume info from the backend",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.CreateClone(ctx, volConfig, volConfig, pool1)
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeClone_ValidationTest(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	pool1 := storage.NewStoragePool(nil, "pool1")

	type args struct {
		SplitOnClone      string
		QosPolicy         string
		AdaptiveQosPolicy string
	}

	tests := []struct {
		name          string
		arg           args
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "LunSetQosPolicyGroup_Success",
			arg: args{
				SplitOnClone: "False",
				QosPolicy:    "fake-qos-policy",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
					func(ctx context.Context, volumeName string) (bool, error) {
						return false, nil
					},
				).MaxTimes(1)
				mockAPI.EXPECT().VolumeSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeWaitForStates(ctx, getVolumeConfig().InternalName, []string{"online"}, []string{"error"},
					maxFlexvolCloneWait).AnyTimes().Return("online", nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunSetQosPolicyGroup(ctx, gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Failed to update the Qos policy on LUN",
		},
		{
			name: "LunSetQosPolicyGroup_Fail",
			arg: args{
				SplitOnClone: "False",
				QosPolicy:    "fake-qos-policy",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
					func(ctx context.Context, volumeName string) (bool, error) {
						return false, nil
					},
				).MaxTimes(1)

				mockAPI.EXPECT().VolumeSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().LunSetQosPolicyGroup(ctx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf("update QOS policy on LUN failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "Updated the Qos policy on LUN",
		},
		{
			name: "CloneFlexVolFail",
			arg: args{
				SplitOnClone: "False",
				QosPolicy:    "fake-qos-policy",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
					func(ctx context.Context, volumeName string) (bool, error) {
						return true, fmt.Errorf("failed to verify clone volume")
					},
				).MaxTimes(1)
			},
			wantErr:       assert.Error,
			assertMessage: "Validate the clone volume",
		},
		{
			name: "QosPolicy_fail",
			arg: args{
				SplitOnClone:      "False",
				QosPolicy:         "fake-qos-policy",
				AdaptiveQosPolicy: "fake-Adaptive-policy",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
			},
			wantErr:       assert.Error,
			assertMessage: "Updated the Qos policy on volume",
		},
		{
			name: "splitClone_fail",
			arg: args{
				SplitOnClone: "InvalidValue",
			},
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
			},
			wantErr:       assert.Error,
			assertMessage: "Updated the split clone value on volume",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver.Config.SplitOnClone = test.arg.SplitOnClone
			volConfig := getVolumeConfig()
			volConfig.QosPolicy = test.arg.QosPolicy
			volConfig.AdaptiveQosPolicy = test.arg.AdaptiveQosPolicy

			volume := api.Volume{}
			volume.Name = "vol1"
			mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolume).Return(&volume, nil)

			test.mocks(mockAPI)

			err := driver.CreateClone(ctx, &volConfig, &volConfig, pool1)
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeClone_LabelLengthExceeding(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	pool1 := storage.NewStoragePool(nil, "")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})
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
		"cloud":   "anf",
		longLabel: "dev-test-cluster-1",
	}

	volConfig := getVolumeConfig()

	volume := api.Volume{Name: "vol1"}
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolume).Return(&volume, nil)

	err := driver.CreateClone(ctx, &volConfig, &volConfig, nil)

	assert.Error(t, err, "Clone of a volume is created.")
}

func TestOntapSanVolumeClone_NameTemplate(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)
	luks := "true"
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
		TieringPolicy:     "none",
		QosPolicy:         "fake-qos-policy",
		AdaptiveQosPolicy: "",
		LUKSEncryption:    luks,
		SplitOnClone:      "false",
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volume := api.Volume{
		Name:    "vol1",
		Comment: "iscsi volume",
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

			volConfig := getVolumeConfig()

			mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolume).Return(&volume, nil)
			mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
				func(ctx context.Context, volumeName string) (bool, error) {
					return false, nil
				},
			).MaxTimes(1)

			mockAPI.EXPECT().VolumeSnapshotCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
			mockAPI.EXPECT().VolumeCloneCreate(ctx, gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			mockAPI.EXPECT().VolumeWaitForStates(ctx, volConfig.InternalName, []string{"online"}, []string{"error"},
				maxFlexvolCloneWait).AnyTimes().Return("online", nil)
			mockAPI.EXPECT().VolumeSetComment(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			err := driver.CreateClone(ctx, &volConfig, &volConfig, pool1)

			assert.NoError(t, err, "Clone creation failed. Expected no error")
		})
	}
}

func TestOntapSanVolumeClone_NameTemplateLabelLengthExceeding(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	pool1 := storage.NewStoragePool(nil, "")
	pool1.SetInternalAttributes(map[string]string{
		"tieringPolicy": "none",
	})
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
		"cloud":   "anf",
		longLabel: "dev-test-cluster-1",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})

	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.Config.NameTemplate = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"

	volConfig := getVolumeConfig()

	volume := api.Volume{Name: "vol1"}
	mockAPI.EXPECT().VolumeInfo(ctx, volConfig.CloneSourceVolume).Return(&volume, nil)

	err := driver.CreateClone(ctx, &volConfig, &volConfig, nil)

	assert.Error(t, err, "Clone of a volume is created.")

	// Storage pool is not unset

	pool1.SetName("pool1")
	volume = api.Volume{
		AccessType: "rw",
		Comment:    "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&volume, nil)

	err = driver.CreateClone(ctx, &volConfig, &volConfig, pool1)

	assert.Error(t, err, "Clone of a volume is created.")
}

func TestOntapSanVolumeImport(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.Config.Labels = map[string]string{
		"cloud": "san",
		"label": "dev-test-cluster-1",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	originalVolumeName := "originalVolume"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
		Comment:    "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}
	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
	}

	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)
	mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
		"/vol/"+originalVolumeName+"/lun0").Return(nil)
	mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
	mockAPI.EXPECT().VolumeSetComment(ctx, volConfig.InternalName, originalVolumeName,
		"{\"provisioning\":{\"cloud\":\"san\",\"label\":\"dev-test-cluster-1\"}}").Return(nil)
	mockAPI.EXPECT().LunListIgroupsMapped(ctx, "/vol/trident-pvc-1234/lun0").Return(nil, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.NoError(t, err, "Error in Volume import, expected no error")
}

func TestOntapSanVolumeImport_VolumeInfoFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	originalVolumeName := "vol1"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil,
					fmt.Errorf("volume check fail"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "VolumeInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "LunInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				volume := api.Volume{
					Name:    originalVolumeName,
					Comment: "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
				}

				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(
					nil, fmt.Errorf("lun not found"))
			},
			wantErr:       assert.Error,
			assertMessage: "LUN is present in backend",
		},
		{
			name: "LunInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				volume := api.Volume{
					Name:    originalVolumeName,
					Comment: "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
				}

				mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
				mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(nil, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "LUN is present in backend",
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

func TestOntapSanVolumeImport_RenameFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	originalVolumeName := "vol1"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		Comment: "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}
	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "LunRename",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
					"/vol/"+originalVolumeName+"/lun0").Return(fmt.Errorf("LUN rename failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "Renamed the LUN",
		},
		{
			name: "VolumeRename",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
					"/vol/"+originalVolumeName+"/lun0").Return(nil)
				mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, volConfig.InternalName).Return(
					fmt.Errorf("volume rename failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "Renamed the Volume",
		},
		{
			name: "LunOnline",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				lun.State = "offline"
			},
			wantErr:       assert.Error,
			assertMessage: "LUN is online",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
			mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)

			test.mocks(mockAPI)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeImport_VolumeUpdateFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	originalVolumeName := "vol1"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		Comment: "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}
	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeSetComment_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)
				mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
					"/vol/"+originalVolumeName+"/lun0").Return(nil)
				mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, volConfig.InternalName, originalVolumeName,
					"").Return(fmt.Errorf("volume comment update failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "comment updated on volume",
		},
		{
			name: "LunListIgroupsMapped_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)
				mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
					"/vol/"+originalVolumeName+"/lun0").Return(nil)
				mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
				mockAPI.EXPECT().VolumeSetComment(ctx, volConfig.InternalName, originalVolumeName,
					"").Return(nil)
				mockAPI.EXPECT().LunListIgroupsMapped(ctx, "/vol/trident-pvc-1234/lun0").Return(
					nil, fmt.Errorf("LUN igroup mapping failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "LUN is mapped to igroup",
		},
		{
			name: "AcessTypeReadOnly",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				volume.AccessType = "ro"
			},
			wantErr:       assert.Error,
			assertMessage: "Volume access type is read/write",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)

			test.mocks(mockAPI)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeImport_ImportNotManaged(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	originalVolumeName := "vol1"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = true

	volume := api.Volume{
		Comment: "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}
	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
	}

	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Volume is imported, expected to fail")
}

func TestOntapSanVolumeImport_NameTemplateLabel(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	pool1 := storage.NewStoragePool(nil, "pool1")

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"

	originalVolumeName := "originalVolume"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
		Comment:    "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}
	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
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

	for i, test := range tests {
		t.Run(fmt.Sprintf("Unix Permissions: %d", i), func(t *testing.T) {
			driver.Config.Labels = map[string]string{
				test.labelKey: test.labelValue,
			}
			pool1.SetAttributes(map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
			})
			driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

			mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
			mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)
			mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
				"/vol/"+originalVolumeName+"/lun0").Return(nil)
			mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)
			mockAPI.EXPECT().VolumeSetComment(ctx, volConfig.InternalName, originalVolumeName,
				test.expectedLabel).Return(nil)
			mockAPI.EXPECT().LunListIgroupsMapped(ctx, "/vol/trident-pvc-1234/lun0").Return(
				nil, nil)

			err := driver.Import(ctx, &volConfig, originalVolumeName)

			assert.NoError(t, err, "Volume import fail")
		})
	}
}

func TestOntapSanVolumeImport_NameTemplateLabelLengthExceeding(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
		"cloud":   "san",
		longLabel: "dev-test-cluster-1",
	}

	pool1.InternalAttributes()[NameTemplate] = "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume." +
		"RequestName}}"
	pool1.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(driver.Config.Labels),
	})
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}

	originalVolumeName := "originalVolume"
	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	volume := api.Volume{
		AccessType: "rw",
		Comment:    "{\"provisioning\":{\"cloud\":\"anf\",\"clusterName\":\"dev-test-cluster-1\"}}",
	}
	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
	}

	mockAPI.EXPECT().VolumeInfo(ctx, originalVolumeName).Return(&volume, nil)
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)
	mockAPI.EXPECT().LunRename(ctx, "/vol/"+originalVolumeName+"/lun1",
		"/vol/"+originalVolumeName+"/lun0").Return(nil)
	mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, volConfig.InternalName).Return(nil)

	err := driver.Import(ctx, &volConfig, originalVolumeName)

	assert.Error(t, err, "Volume imported, expected an error")
}

func TestOntapSanVolumeRename(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	originalVolumeName := "vol1"
	newVolumeName := "vol2"

	mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, newVolumeName).Return(nil)

	err := driver.Rename(ctx, originalVolumeName, newVolumeName)

	assert.NoError(t, err, "Volume rename failed")
}

func TestOntapSanVolumeRename_fail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	originalVolumeName := "vol1"
	newVolumeName := "vol2"

	mockAPI.EXPECT().VolumeRename(ctx, originalVolumeName, newVolumeName).Return(
		fmt.Errorf("failed to rename volume"))

	err := driver.Rename(ctx, originalVolumeName, newVolumeName)

	assert.Error(t, err, "Renamed the volume, expected to fail")
}

func TestOntapSanVolumeDestroy_FSx(t *testing.T) {
	type parameters struct {
		configureOntapAPI func(api *mockapi.MockOntapAPI)
		ConfigureAwsAPI   func(api *mockapi.MockAWSAPI)
		volumeConfig      storage.VolumeConfig
		assertError       assert.ErrorAssertionFunc
	}

	const recoveryQueueNamePostfix = 1234

	mockAPI, mockAWSAPI, driver := newMockAWSOntapSANDriver(t)
	volConfig := getVolumeConfig()

	skipRecoveryQueueVolumeConfig := getVolumeConfig()
	skipRecoveryQueueVolumeConfig.SkipRecoveryQueue = "true"

	tests := map[string]parameters{
		"FSx volume in available state": {
			ConfigureAwsAPI: func(api *mockapi.MockAWSAPI) {
				vol := awsapi.Volume{
					State: awsapi.StateAvailable,
				}
				api.EXPECT().VolumeExists(ctx, &volConfig).Return(true, &vol, nil)
				api.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).
					Return("", nil)
				api.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
			},
			volumeConfig: volConfig,
			assertError:  assert.NoError,
		},
		"FSx volume in deleting state": {
			ConfigureAwsAPI: func(api *mockapi.MockAWSAPI) {
				vol := awsapi.Volume{
					State: awsapi.StateDeleting,
				}
				api.EXPECT().VolumeExists(ctx, &volConfig).Return(true, &vol, nil)
				api.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).Return("", nil)
			},
			volumeConfig: volConfig,
			assertError:  assert.NoError,
		},
		"SkipRecoveryQueue FSx Fexvol volume: error purging the recovery queue": {
			configureOntapAPI: func(a *mockapi.MockOntapAPI) {
				recoveryQueueVolumeName := fmt.Sprintf("%s_%d", skipRecoveryQueueVolumeConfig.InternalName,
					recoveryQueueNamePostfix)
				a.EXPECT().VolumeRecoveryQueueGetName(ctx, skipRecoveryQueueVolumeConfig.InternalName).Return(recoveryQueueVolumeName, nil)
				a.EXPECT().VolumeRecoveryQueuePurge(ctx, recoveryQueueVolumeName).Return(assert.AnError)
			},
			ConfigureAwsAPI: func(api *mockapi.MockAWSAPI) {
				vol := awsapi.Volume{
					State: awsapi.StateAvailable,
				}
				api.EXPECT().VolumeExists(ctx, &skipRecoveryQueueVolumeConfig).Return(true, &vol, nil)
				api.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).
					Return("", nil)
				api.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
			},
			volumeConfig: skipRecoveryQueueVolumeConfig,
			assertError:  assert.NoError,
		},
		"SkipRecoveryQueue FSx Fexvol volume: happy path": {
			configureOntapAPI: func(a *mockapi.MockOntapAPI) {
				recoveryQueueVolumeName := fmt.Sprintf("%s_%d", skipRecoveryQueueVolumeConfig.InternalName,
					recoveryQueueNamePostfix)
				a.EXPECT().VolumeRecoveryQueueGetName(ctx, skipRecoveryQueueVolumeConfig.InternalName).Return(recoveryQueueVolumeName, nil)
				a.EXPECT().VolumeRecoveryQueuePurge(ctx, recoveryQueueVolumeName).Return(nil)
			},
			ConfigureAwsAPI: func(api *mockapi.MockAWSAPI) {
				vol := awsapi.Volume{
					State: awsapi.StateAvailable,
				}
				api.EXPECT().VolumeExists(ctx, &skipRecoveryQueueVolumeConfig).Return(true, &vol, nil)
				api.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).
					Return("", nil)
				api.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
			},
			volumeConfig: skipRecoveryQueueVolumeConfig,
			assertError:  assert.NoError,
		},
		"SkipRecoveryQueue FSx Fexvol volume: error getting recovery queue volumeName": {
			configureOntapAPI: func(a *mockapi.MockOntapAPI) {
				a.EXPECT().VolumeRecoveryQueueGetName(ctx, skipRecoveryQueueVolumeConfig.InternalName).
					Return("", assert.AnError)
			},
			ConfigureAwsAPI: func(api *mockapi.MockAWSAPI) {
				vol := awsapi.Volume{
					State: awsapi.StateAvailable,
				}
				api.EXPECT().VolumeExists(ctx, &skipRecoveryQueueVolumeConfig).Return(true, &vol, nil)
				api.EXPECT().WaitForVolumeStates(
					ctx, &vol, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout).Return("", nil)
				api.EXPECT().DeleteVolume(ctx, &vol).Return(nil)
			},
			volumeConfig: skipRecoveryQueueVolumeConfig,
			assertError:  assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			if params.configureOntapAPI != nil {
				params.configureOntapAPI(mockAPI)
			}
			if params.ConfigureAwsAPI != nil {
				params.ConfigureAwsAPI(mockAWSAPI)
			}

			err := driver.Destroy(ctx, &params.volumeConfig)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestOntapSanVolumeDestroy(t *testing.T) {
	type parameters struct {
		configureOntapAPI func(mockAPI *mockapi.MockOntapAPI)
		volumeConfig      storage.VolumeConfig
	}

	const svmName = "SVM1"
	const nodeName = "node1"
	const iscsiInterface = "iscsi_if"
	const cloneSourceSnapshotInternal = "20240717T102157Z"

	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false

	volConfigWithInternalSnapshot := getVolumeConfig()
	volConfigWithInternalSnapshot.ImportNotManaged = false
	volConfigWithInternalSnapshot.CloneSourceSnapshotInternal = cloneSourceSnapshotInternal

	volConfigSkipRecoveryQueue := getVolumeConfig()
	volConfig.ImportNotManaged = false
	volConfigSkipRecoveryQueue.SkipRecoveryQueue = "true"

	tests := map[string]parameters{
		"happy path": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return(nodeName, nil)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), false).Return(nil)
			},
			volumeConfig: volConfig,
		},
		"source volume with internal snapshot": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return(nodeName, nil)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeSnapshotDelete(ctx, cloneSourceSnapshotInternal, "").Return(nil).Times(1)
			},
			volumeConfig: volConfigWithInternalSnapshot,
		},
		"SkipRecoveryQueue volume": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return(nodeName, nil)
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), true).Return(nil)
			},
			volumeConfig: volConfigSkipRecoveryQueue,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mockAPI, driver := newMockOntapSANDriver(t)

			if params.configureOntapAPI != nil {
				params.configureOntapAPI(mockAPI)
			}

			driver.Config.DriverContext = tridentconfig.ContextDocker
			driver.Config.SANType = sa.ISCSI

			err := driver.Destroy(ctx, &params.volumeConfig)
			assert.NoError(t, err, "Volume destroy failed")
		})
	}
}

func TestOntapSanVolumeDestroy_fail(t *testing.T) {
	type parameters struct {
		configureOntapAPI func(mockAPI *mockapi.MockOntapAPI)
		volConfig         storage.VolumeConfig
		expectedError     error
	}

	const svmName = "SVM1"
	const nodeName = "node1"
	const iscsiInterface = "iscsi_if"
	const cloneSourceSnapshotInternal = "20240717T102157Z"

	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = true

	volConfigWithInternalSnapshot := getVolumeConfig()
	volConfigWithInternalSnapshot.ImportNotManaged = true
	volConfigWithInternalSnapshot.CloneSourceSnapshotInternal = cloneSourceSnapshotInternal

	tests := map[string]parameters{
		"GetISCSITargetInfo_fail": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface},
					fmt.Errorf("failed to get target iscsi info")).Times(1)
			},
			volConfig:     volConfig,
			expectedError: fmt.Errorf("could not get SVM iSCSI node name: failed to get target iscsi info"),
		},
		"GetISCSITargetInfo_fail: source volume has an internal snapshot": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface},
					fmt.Errorf("failed to get target iscsi info")).Times(1)
			},
			volConfig:     volConfigWithInternalSnapshot,
			expectedError: fmt.Errorf("could not get SVM iSCSI node name: failed to get target iscsi info"),
		},
		"LunMapInfo_fail": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0,
					fmt.Errorf("LUN map to iscsi device failed"))
			},
			volConfig: volConfig,
			expectedError: fmt.Errorf(
				"error reading LUN maps for volume %v: LUN map to iscsi device failed", volConfig.InternalName,
			),
		},
		"LunMapInfo_fail: source volume has an internal snapshot": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0,
					fmt.Errorf("LUN map to iscsi device failed"))
			},
			volConfig: volConfigWithInternalSnapshot,
			expectedError: fmt.Errorf(
				"error reading LUN maps for volume %v: LUN map to iscsi device failed", volConfig.InternalName,
			),
		},
		"SnapmirrorDeleteViaDestination_fail": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(
					fmt.Errorf("snapmirror delete failed"))
			},
			volConfig:     volConfig,
			expectedError: fmt.Errorf("snapmirror delete failed"),
		},
		"SnapmirrorDeleteViaDestination_fail: source volume has an internal snapshot": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(
					fmt.Errorf("snapmirror delete failed"))
			},
			volConfig:     volConfigWithInternalSnapshot,
			expectedError: fmt.Errorf("snapmirror delete failed"),
		},
		"VolumeDestroy_fail": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("volume destroy failed"))
			},
			volConfig:     volConfig,
			expectedError: fmt.Errorf("error destroying volume %v: volume destroy failed", volConfig.InternalName),
		},
		"VolumeDestroy_fail: source volume has an internal snapshot": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("volume destroy failed"))
			},
			volConfig:     volConfigWithInternalSnapshot,
			expectedError: fmt.Errorf("error destroying volume %v: volume destroy failed", volConfig.InternalName),
		},
		"internal volume cleanup failure: internal snapshot not found": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeSnapshotDelete(ctx, cloneSourceSnapshotInternal, "").Return(api.NotFoundError("snapshot not found")).Times(1)
			},
			volConfig:     volConfigWithInternalSnapshot,
			expectedError: nil,
		},
		"internal volume cleanup failure: internal snapshot deletion error": {
			configureOntapAPI: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{iscsiInterface}, nil).Times(1)
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, nil)
				mockAPI.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(0, nil)
				mockAPI.EXPECT().SnapmirrorDeleteViaDestination(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().SnapmirrorRelease(ctx, gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeDestroy(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockAPI.EXPECT().VolumeSnapshotDelete(ctx, cloneSourceSnapshotInternal, "").Return(fmt.Errorf("snapshot not found")).Times(1)
			},
			volConfig:     volConfigWithInternalSnapshot,
			expectedError: nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			mockAPI, driver := newMockOntapSANDriver(t)
			mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)
			mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return(nodeName, nil)

			driver.Config.DriverContext = tridentconfig.ContextDocker
			driver.Config.SANType = sa.ISCSI

			if params.configureOntapAPI != nil {
				params.configureOntapAPI(mockAPI)
			}

			err := driver.Destroy(ctx, &params.volConfig)
			assert.Equal(t, params.expectedError, err)
		})
	}
}

func TestOntapSanVolumeDestroy_VolumeExistsFail(t *testing.T) {
	const svmName = "SVM1"
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return(svmName)

	driver.Config.DriverContext = tridentconfig.ContextDocker

	volConfig := getVolumeConfig()

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeExists_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false,
					fmt.Errorf("volume check fail"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "VolumeExists_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Volume is present in backend",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Destroy(ctx, &volConfig)
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeSnapshot(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()

	snapshotConfig := &storage.SnapshotConfig{
		Name:               "snap_vol1",
		InternalName:       "trident-pvc-1234_snap",
		VolumeName:         "vol1",
		VolumeInternalName: "trident-pvc-1234",
	}

	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volumeName string) (bool, error) {
			return true, nil
		},
	).MaxTimes(1)

	mockAPI.EXPECT().LunSize(ctx, gomock.Any()).Return(1073741824, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, snapshotConfig.InternalName,
		snapshotConfig.VolumeInternalName).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		snapshotConfig.InternalName, snapshotConfig.VolumeInternalName).Return(
		api.Snapshot{
			CreateTime: "",
			Name:       snapshotConfig.InternalName,
		},
		nil)

	snap, err := driver.CreateSnapshot(ctx, snapshotConfig, &volConfig)

	assert.Equal(t, snap.Config.InternalName, snapshotConfig.InternalName)
	assert.Equal(t, snap.Config.VolumeInternalName, snapshotConfig.VolumeInternalName)
	assert.Equal(t, snap.SizeBytes, int64(1073741824))
	assert.Equal(t, snap.State, storage.SnapshotStateOnline)
	assert.NoError(t, err, "Snapshot creation failed")
}

func TestOntapSanVolumeSnapshot_SnapshotNotFound(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()

	snapshotConfig := &storage.SnapshotConfig{
		Name:               "snap_vol1",
		InternalName:       "trident-pvc-1234_snap",
		VolumeName:         "vol1",
		VolumeInternalName: "trident-pvc-1234",
	}

	mockAPI.EXPECT().VolumeExists(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, volumeName string) (bool, error) {
			return true, nil
		},
	).MaxTimes(1)

	mockAPI.EXPECT().LunSize(ctx, gomock.Any()).Return(1073741824, nil)
	mockAPI.EXPECT().VolumeSnapshotCreate(ctx, snapshotConfig.InternalName,
		snapshotConfig.VolumeInternalName).Return(nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		snapshotConfig.InternalName, snapshotConfig.VolumeInternalName).Return(
		api.Snapshot{
			CreateTime: "",
			Name:       snapshotConfig.InternalName,
		},
		errors.NotFoundError("snapshot %v not found for volume %v", snapshotConfig.InternalName,
			snapshotConfig.VolumeInternalName))

	_, err := driver.CreateSnapshot(ctx, snapshotConfig, &volConfig)

	assert.Error(t, err, "Snapshot created")
}

func TestOntapSanVolumeSnapshotRestore(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()

	snapshotConfig := &storage.SnapshotConfig{
		Name:               "snap_vol1",
		InternalName:       "trident-pvc-1234_snap",
		VolumeName:         "vol1",
		VolumeInternalName: "trident-pvc-1234",
	}

	mockAPI.EXPECT().SnapshotRestoreVolume(ctx, snapshotConfig.InternalName,
		snapshotConfig.VolumeInternalName).Return(nil)

	err := driver.RestoreSnapshot(ctx, snapshotConfig, &volConfig)

	assert.NoError(t, err, "Snapshot restore failed")
}

func TestOntapSanVolumeSnapshotDelete(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()

	snapshotConfig := &storage.SnapshotConfig{
		Name:               "snap_vol1",
		InternalName:       "trident-pvc-1234_snap",
		VolumeName:         "vol1",
		VolumeInternalName: "trident-pvc-1234",
	}

	mockAPI.EXPECT().VolumeSnapshotDelete(ctx, snapshotConfig.InternalName,
		snapshotConfig.VolumeInternalName).Return(nil)

	err := driver.DeleteSnapshot(ctx, snapshotConfig, &volConfig)

	assert.NoError(t, err, "Snapshot delete failed")
}

func TestOntapSanVolumeSnapshotDelete_fail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()

	snapshotConfig := &storage.SnapshotConfig{
		Name:               "snap_vol1",
		InternalName:       "trident-pvc-1234_snap",
		VolumeName:         "vol1",
		VolumeInternalName: "trident-pvc-1234",
	}

	snapBusyError := api.SnapshotBusyError("Snapshot is in use")
	mockAPI.EXPECT().VolumeSnapshotDelete(ctx, snapshotConfig.InternalName,
		snapshotConfig.VolumeInternalName).Return(snapBusyError)
	mockAPI.EXPECT().VolumeListBySnapshotParent(ctx, snapshotConfig.InternalName,
		snapshotConfig.VolumeInternalName).Return(nil, nil)

	// Use DefaultCloneSplitDelay to set time to past. It is defaulted to 10 seconds.
	driver.cloneSplitTimers.Store(snapshotConfig.ID(), time.Now().Add(-10*time.Second))
	err := driver.DeleteSnapshot(ctx, snapshotConfig, &volConfig)

	assert.Error(t, err, "Snapshot destroyed, expected an error")
}

func TestOntapSanVolumeSnapshotGet(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	snapshotConfig := &storage.SnapshotConfig{
		InternalName: "trident-pvc-1234_snap",
	}

	mockAPI.EXPECT().VolumeExists(ctx, snapshotConfig.InternalName).Return(true, nil)

	err := driver.Get(ctx, snapshotConfig.InternalName)

	assert.NoError(t, err, "Failed to get the snapshot")
}

func TestOntapSanVolumeSnapshotGet_fail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	snapshotConfig := &storage.SnapshotConfig{
		InternalName: "trident-pvc-1234_snap",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeExists_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, snapshotConfig.InternalName).Return(false,
					fmt.Errorf("volume check fail"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "VolumeExists_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, snapshotConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Get(ctx, snapshotConfig.InternalName)
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSanVolumeGetStorageBackendSpecs(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	pool1 := storage.NewStoragePool(nil, "pool1")
	driver.physicalPools = map[string]storage.Pool{"pool1": pool1}
	driver.virtualPools = map[string]storage.Pool{"pool1": pool1}
	driver.ips = []string{"1.2.3.1", "1.2.3.2", "1.2.3.3"}

	backend, _ := storage.NewStorageBackend(ctx, driver)
	err := driver.GetStorageBackendSpecs(ctx, backend)
	assert.NoError(t, err, "Failed to get the storage backend specification")
}

func TestOntapSanStorageDriverGetStorageBackendPools(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
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

func TestOntapSanVolumeGetInternalVolumeName(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()
	driver.Config.CommonStorageDriverConfig = commonConfig
	volConfig := &storage.VolumeConfig{Name: "vol1"}
	pool := storage.NewStoragePool(nil, "dummyPool")

	internalName := driver.GetInternalVolumeName(ctx, volConfig, pool)

	assert.Equal(t, "trident_vol1", internalName)
}

func TestOntapSanVolumeCreatePrepare(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()
	pool := storage.NewStoragePool(nil, "dummyPool")

	driver.CreatePrepare(ctx, &volConfig, pool)
}

func TestOntapSanVolumeCreatePrepare_NilPool(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.Config.NameTemplate = `{{.volume.Name}}_{{.volume.Namespace}}_{{.volume.StorageClass}}`
	volConfig := storage.VolumeConfig{Name: "newVolume", Namespace: "testNamespace", StorageClass: "testSC"}

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	driver.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, driver,
		driver.getStoragePoolAttributes(ctx), driver.BackendName())

	driver.CreatePrepare(ctx, &volConfig, nil)
	assert.Equal(t, volConfig.InternalName, "newVolume_testNamespace_testSC", "volume name is not set correctly")
}

func TestOntapSanVolumeCreatePrepare_NilPool_templateNotContainVolumeName(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	driver.API = mockAPI
	driver.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.Config.NameTemplate = `{{.volume.Namespace}}_{{.volume.StorageClass}}_{{slice .volume.Name 4 9}}`
	volConfig := storage.VolumeConfig{Name: "pvc-1234567", Namespace: "testNamespace", StorageClass: "testSC"}

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	driver.physicalPools, _, _ = InitializeStoragePoolsCommon(ctx, driver,
		driver.getStoragePoolAttributes(ctx), driver.BackendName())

	driver.CreatePrepare(ctx, &volConfig, nil)
	assert.Equal(t, volConfig.InternalName, "testNamespace_testSC_12345", "volume name is not set correctly")
}

func TestOntapSanVolumeCreateFollowup(t *testing.T) {
	ctx := context.Background()
	_, driver := newMockOntapSANDriver(t)

	volConfig := getVolumeConfig()

	_ = driver.CreateFollowup(ctx, &volConfig)
}

func TestOntapSanVolumeGetVolumeForImport(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	storagePrefix := "trident-"
	driver.Config.StoragePrefix = &storagePrefix

	originalVolumeName := "trident-vol1"
	volume := api.Volume{
		Aggregates:     []string{"svm1"},
		Name:           "trident-vol1",
		SnapshotPolicy: "none",
		AccessType:     VolTypeRW,
	}

	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + originalVolumeName + "/lun1",
		State: "online",
	}

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&volume, nil)
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(&lun, nil)

	volumeExternal, err := driver.GetVolumeForImport(ctx, "trident-vol1")

	assert.NoError(t, err, "Failed to get the external volume")
	assert.Equal(t, "svm1", volumeExternal.Pool)
	assert.Equal(t, "vol1", volumeExternal.Config.Name)
	assert.Equal(t, "trident-vol1", volumeExternal.Config.InternalName)
	assert.Equal(t, "1g", volumeExternal.Config.Size)
	assert.Equal(t, tridentconfig.Block, volumeExternal.Config.Protocol)
	assert.Equal(t, "none", volumeExternal.Config.SnapshotPolicy)
}

func TestOntapSanVolumeGetVolumeForImport_Fail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	storagePrefix := "trident-"
	driver.Config.StoragePrefix = &storagePrefix

	originalVolumeName := "vol1"
	volume := api.Volume{
		Aggregates:     []string{"svm1"},
		Name:           "vol1",
		SnapshotPolicy: "none",
		AccessType:     VolTypeRW,
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(
					nil, fmt.Errorf("failed to verify volume"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "LunGetByName_fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&volume, nil)
				mockAPI.EXPECT().LunGetByName(ctx, "/vol/"+originalVolumeName+"/*").Return(
					nil, fmt.Errorf("failed to get lun with specified name"))
			},
			wantErr:       assert.Error,
			assertMessage: "LUN is present in backend",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			_, err := driver.GetVolumeForImport(ctx, "vol1")
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSANStorageDriverGetVolumeExternalWrappers(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	channel := make(chan *storage.VolumeExternalWrapper, 1)
	*driver.Config.StoragePrefix = "Test_"

	volume := api.Volume{
		Aggregates:     []string{"svm1"},
		Name:           "vol1",
		SnapshotPolicy: "none",
		AccessType:     VolTypeRW,
	}

	lun := api.Lun{
		Size:  "1g",
		Name:  "/vol/" + "vol1" + "/lun1",
		State: "online",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&volume}, nil).Times(1)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{lun}, nil).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)
	assert.Equal(t, 0, len(channel))
}

func TestOntapSANStorageDriverGetVolumeExternalWrappers_1(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	channel := make(chan *storage.VolumeExternalWrapper, 1)
	*driver.Config.StoragePrefix = "Test_"

	volume := api.Volume{
		Aggregates:     []string{"svm1"},
		Name:           "vol1",
		SnapshotPolicy: "none",
		AccessType:     VolTypeRW,
	}

	lun := api.Lun{
		Size:       "1g",
		Name:       "/vol/" + "vol1" + "/lun1",
		State:      "online",
		VolumeName: "vol1",
	}

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&volume}, nil).Times(1)
	mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(api.Luns{lun}, nil).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)
	assert.Equal(t, 1, len(channel))
}

func TestOntapSANStorageDriverGetVolumeExternalWrappers_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	*driver.Config.StoragePrefix = "Test_"

	tests := []struct {
		name    string
		mocks   func(mockAPI *mockapi.MockOntapAPI)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "VolumeListByPrefix_fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(nil, fmt.Errorf("no volume found"))
			},
			wantErr: assert.Error,
		},
		{
			name: "LunList_fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeListByPrefix(ctx, gomock.Any()).Return(api.Volumes{&api.Volume{}}, nil).Times(1)
				mockAPI.EXPECT().LunList(ctx, gomock.Any()).Return(nil, fmt.Errorf("LUN not found"))
			},
			wantErr: assert.Error,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := make(chan *storage.VolumeExternalWrapper, 1)
			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
			test.mocks(mockAPI)

			driver.GetVolumeExternalWrappers(ctx, channel)

			assert.Equal(t, 1, len(channel))
		})
	}
}

func TestOntapSANStorageDriverGetUpdateType(t *testing.T) {
	mockAPI, oldDriver := newMockOntapSANDriver(t)

	oldDriver.API = mockAPI
	prefix1 := "test_"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Username = "user1"
	oldDriver.Config.Password = "password1"
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}
	oldDriver.Config.DataLIF = "1.2.3.1"

	newDriver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)

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
	newDriver.Config.DataLIF = "1.2.3.2"

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.UsernameChange)
	expectedBitmap.Add(storage.PasswordChange)
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestOntapSANStorageDriverGetUpdateType_NilDataLIF(t *testing.T) {
	mockAPI, oldDriver := newMockOntapSANDriver(t)
	oldDriver.Config.DataLIF = "1.2.3.1"

	newDriver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	newDriver.Config.DataLIF = ""

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.False(t, result.Contains(storage.InvalidVolumeAccessInfoChange), "Nil DataLIF should be valid.")
}

func TestOntapSANStorageDriverGetUpdateType_Failure(t *testing.T) {
	mockAPI, _ := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	oldDriver := newTestOntapSanEcoDriver(t, ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, false, nil, mockAPI)
	oldDriver.API = mockAPI
	prefix1 := "test_"
	oldDriver.Config.StoragePrefix = &prefix1

	// Created a SAN driver
	newDriver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)

	newDriver.API = mockAPI
	prefix2 := "storage_"
	newDriver.Config.StoragePrefix = &prefix2

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestOntapSANStorageDriverResize(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		Aggregates:     aggr,
		SnapshotPolicy: "none",
	}
	volConfig := getVolumeConfig()

	mockAPI.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1073741824), nil)
	mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)
	mockAPI.EXPECT().VolumeSetSize(ctx, "trident-pvc-1234",
		"2362232012").Return(nil) // LUNMetadataBufferMultiplier * 1.1
	mockAPI.EXPECT().LunSetSize(ctx, "/vol/trident-pvc-1234/lun0", "2147483648").Return(uint64(214748364), nil)

	err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

	assert.NoError(t, err, "Volume resize failed")
}

func TestOntapSANStorageDriverResize_SameSize(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		Aggregates:     aggr,
		SnapshotPolicy: "none",
	}
	volConfig := getVolumeConfig()

	mockAPI.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1181116006), nil)
	mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)

	err := driver.Resize(ctx, &volConfig, 1073741824) // 2GB

	assert.NoError(t, err, "able to update volume size smaller than actual size")
}

func TestOntapSANStorageDriverResize_VolumeLargerThanResizeRequest(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		Aggregates:     aggr,
		SnapshotPolicy: "none",
	}
	volConfig := getVolumeConfig()

	mockAPI.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(2684354560), nil) // 2.5 Gi
	mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)  // 1 Gi
	mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)
	mockAPI.EXPECT().LunSetSize(ctx, "/vol/trident-pvc-1234/lun0", "2147483648").Return(uint64(214748364), nil)

	err := driver.Resize(ctx, &volConfig, 2147483648) // 2Gi

	assert.NoError(t, err, "Volume resize failed")
}

func TestOntapSANStorageDriverResize_UpdateSmallerSize(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")
	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		Aggregates:     aggr,
		SnapshotPolicy: "none",
	}
	volConfig := getVolumeConfig()

	mockAPI.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
	mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1181116006), nil)
	mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)

	err := driver.Resize(ctx, &volConfig, 107374182)

	assert.Error(t, err, "Update the size of a volume")
}

func TestOntapSANStorageDriverResize_VolumeExistsFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)

	volConfig := getVolumeConfig()

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeExists_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false,
					fmt.Errorf("volume check fail"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "VolumeExists_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
				mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1073741824), nil)
				mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(
					nil, fmt.Errorf("failed to get volume"))
			},
			wantErr:       assert.Error,
			assertMessage: "Volume is present in backend",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSANStorageDriverResize_VolumeSizeFail(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	aggr := make([]string, 0)
	aggr = append(aggr, "aggr1")

	volConfig := getVolumeConfig()

	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		Aggregates:     aggr,
		SnapshotPolicy: "none",
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "GetVolumeSizeFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(
					uint64(0), fmt.Errorf("error occurred while checking volume size"))
			},
			wantErr:       assert.Error,
			assertMessage: "Get the volume size",
		},
		{
			name: "GetLUNSizeFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1073741824), nil)
				mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(0,
					fmt.Errorf("error occurred while checking LUN size"))
			},
			wantErr:       assert.Error,
			assertMessage: "Get the LUN size",
		},
		{
			name: "VolumeSetSizeFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1073741824), nil)
				mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)
				mockAPI.EXPECT().VolumeSetSize(ctx, "trident-pvc-1234",
					"2362232012").Return(fmt.Errorf("error occurred while updating volume size"))
			},
			wantErr:       assert.Error,
			assertMessage: "Updated the volume size",
		},
		{
			name: "LunSetSizeFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1073741824), nil)
				mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)
				mockAPI.EXPECT().VolumeSetSize(ctx, "trident-pvc-1234", "2362232012").Return(nil)
				mockAPI.EXPECT().LunSetSize(ctx, "/vol/trident-pvc-1234/lun0", "2147483648").Return(uint64(0),
					fmt.Errorf("error occurred while updating LUn size"))
			},
			wantErr:       assert.Error,
			assertMessage: "Updated the volume size",
		},
		{
			name: "limitVolumeSizeFail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				driver.Config.CommonStorageDriverConfig.LimitVolumeSize = "InvalidLimitSize"
				mockAPI.EXPECT().VolumeSize(ctx, "trident-pvc-1234").Return(uint64(1073741824), nil)
				mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, "trident-pvc-1234").AnyTimes().Return(&volume, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Updated the volume size limit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAPI.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
			test.mocks(mockAPI)

			err := driver.Resize(ctx, &volConfig, 2147483648) // 2GB

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSANStorageDriverReconcileNodeAccess(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)
	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1"})

	igroupName := "igroup1"
	mockAPI.EXPECT().IgroupList(ctx).Return([]string{igroupName}, nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName)
	mockAPI.EXPECT().IgroupDestroy(ctx, igroupName)

	err := driver.ReconcileNodeAccess(ctx, nodes, "1234", "")

	assert.NoError(t, err, "Node reconcile failed")
}

func TestOntapSANStorageDriverReconcileNodeAccess_fail(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)
	nodes := make([]*models.Node, 0)
	nodes = append(nodes, &models.Node{Name: "node1"})

	igroupName := "igroup1"
	mockAPI.EXPECT().IgroupList(ctx).Return([]string{igroupName}, nil)
	mockAPI.EXPECT().IgroupListLUNsMapped(ctx, igroupName)
	mockAPI.EXPECT().IgroupDestroy(ctx, igroupName).Return(fmt.Errorf("failed to delete Igroup"))

	err := driver.ReconcileNodeAccess(ctx, nodes, "1234", "")

	assert.Error(t, err, "Node reconciled")
}

func TestOntapSanVolumePublishisFlexvolRW(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.ips = []string{"127.0.0.1"}
	driver.Config.DriverContext = tridentconfig.ContextCSI
	driver.Config.SANType = sa.ISCSI

	volConfig := getVolumeConfig()
	volConfig.InternalName = "lunName"

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

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "VolumeInfo_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(
					nil, fmt.Errorf("failed to get volume"))
			},
			wantErr:       assert.Error,
			assertMessage: "Get the volume from backend",
		},
		{
			name: "VolumeInfo_NotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeLS}, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Get the volume from backend",
		},
		{
			name: "GetISCSITargetInfo_fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
				mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return(
					"node1", fmt.Errorf("node not found"))
			},
			wantErr:       assert.Error,
			assertMessage: "Get the reporting node info",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.Publish(ctx, &volConfig, publishInfo)
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}

	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().LunGetAttribute(ctx, "/vol/lunName/lun0", "formatOptions")
	mockAPI.EXPECT().LunGetByName(ctx, "/vol/lunName/lun0").Return(dummyLun, nil)

	err := driver.Publish(ctx, &volConfig, publishInfo)
	assert.Errorf(t, err, "no reporting nodes found")
}

func TestOntapSANStorageDriverInitialize_WithNameTemplate(t *testing.T) {
	{
		ctx := context.Background()

		mockCtrl := gomock.NewController(t)
		mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

		driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
		driver.API = mockAPI
		driver.Config.CommonStorageDriverConfig = nil
		driver.ips = []string{"127.0.0.1"}

		defer mockCtrl.Finish()

		mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
			gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
		mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

		commonConfig := getCommonConfig()

		configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"defaults": {
				"nameTemplate": "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}"
		},
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

		iscsiInitiatorAuth := api.IscsiInitiatorAuth{
			SVMName:                "SVM1",
			ChapUser:               "dummyuser",
			ChapPassphrase:         "dummypassword",
			ChapOutboundUser:       "",
			ChapOutboundPassphrase: "",
			Initiator:              "iqn",
			AuthType:               "none",
		}

		driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
		driver.telemetry.TridentBackendUUID = BackendUUID
		message, _ := json.Marshal(driver.GetTelemetry())

		mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
		mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
		mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
			map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
		)
		mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "iscsi", "linux").Return(nil)
		mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(iscsiInitiatorAuth, nil)
		mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
		mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
			string(message), 1, "trident", 5).AnyTimes()
		mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

		result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)
		assert.NoError(t, result, "Ontap SAN storage driver initialization failed")

		nameTemplate := "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"
		assert.Equal(t, nameTemplate, driver.physicalPools["data"].InternalAttributes()[NameTemplate])
	}
}

func TestOntapSANStorageDriverInitialize_NameTemplateDefineInStoragePool(t *testing.T) {
	{
		ctx := context.Background()

		mockCtrl := gomock.NewController(t)
		mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

		driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
		driver.API = mockAPI
		driver.ips = []string{"127.0.0.1"}

		defer mockCtrl.Finish()

		mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
			gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
		mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

		commonConfig := getCommonConfig()

		configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
		"managementLIF":     "127.0.0.1:0",
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

		iscsiInitiatorAuth := api.IscsiInitiatorAuth{
			SVMName:                "SVM1",
			ChapUser:               "dummyuser",
			ChapPassphrase:         "dummypassword",
			ChapOutboundUser:       "",
			ChapOutboundPassphrase: "",
			Initiator:              "iqn",
			AuthType:               "none",
		}

		driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
		driver.telemetry.TridentBackendUUID = BackendUUID
		message, _ := json.Marshal(driver.GetTelemetry())

		mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
		mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
		mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
			map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
		)
		mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "iscsi", "linux").Return(nil)
		mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(iscsiInitiatorAuth, nil)
		mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
		mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
			string(message), 1, "trident", 5).AnyTimes()
		mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

		result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)
		assert.NoError(t, result, "Ontap SAN storage driver initialization failed")

		expectedNameTemplate := "pool_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"

		assert.NoError(t, result)
		for _, pool := range driver.virtualPools {
			assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
		}
	}
}

func TestOntapSANStorageDriverInitialize_NameTemplateDefineInBothPool(t *testing.T) {
	{
		ctx := context.Background()

		mockCtrl := gomock.NewController(t)
		mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

		driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
		driver.API = mockAPI
		driver.Config.CommonStorageDriverConfig = nil
		driver.ips = []string{"127.0.0.1"}

		defer mockCtrl.Finish()

		mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
			gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
		mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

		commonConfig := getCommonConfig()

		configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
		"managementLIF":     "127.0.0.1:0",
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

		iscsiInitiatorAuth := api.IscsiInitiatorAuth{
			SVMName:                "SVM1",
			ChapUser:               "dummyuser",
			ChapPassphrase:         "dummypassword",
			ChapOutboundUser:       "",
			ChapOutboundPassphrase: "",
			Initiator:              "iqn",
			AuthType:               "none",
		}

		driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
		driver.telemetry.TridentBackendUUID = BackendUUID
		message, _ := json.Marshal(driver.GetTelemetry())

		mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
		mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
		mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
			map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
		)
		mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "iscsi", "linux").Return(nil)
		mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(iscsiInitiatorAuth, nil)
		mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
		mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
			string(message), 1, "trident", 5).AnyTimes()
		mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

		result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)
		assert.NoError(t, result, "Ontap SAN storage driver initialization failed")

		expectedNameTemplate := "{{.config.StorageDriverName}}_{{.labels.Cluster}}_{{.volume.Namespace}}_{{.volume.RequestName}}_{{slice .volume.Name 4 9}}"

		assert.NoError(t, result)
		for _, pool := range driver.physicalPools {
			assert.Equal(t, expectedNameTemplate, pool.InternalAttributes()[NameTemplate])
		}
	}
}

func TestOntapSANStorageDriverInitialize_WithTwoAuthMethodsWithSecrets(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
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
	sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, nil, nil)
	sanStorageDriver.Config.CommonStorageDriverConfig = nil

	result := sanStorageDriver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets,
		BackendUUID)

	assert.Error(t, result, "driver initialization succeeded even with more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapSANStorageDriverInitialize_WithTwoAuthMethodsWithConfigAndSecrets(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
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
	sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, nil, nil)
	sanStorageDriver.Config.CommonStorageDriverConfig = nil

	result := sanStorageDriver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets,
		BackendUUID)

	assert.Error(t, result, "driver initialization succeeded even with more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapSanStorageDriverInitialize(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
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

	iscsiInitiatorAuth := api.IscsiInitiatorAuth{
		SVMName:                "SVM1",
		ChapUser:               "dummyuser",
		ChapPassphrase:         "dummypassword",
		ChapOutboundUser:       "",
		ChapOutboundPassphrase: "",
		Initiator:              "iqn",
		AuthType:               "none",
	}

	driver.telemetry.TridentVersion = tridentconfig.OrchestratorVersion.String()
	driver.telemetry.TridentBackendUUID = BackendUUID
	message, _ := json.Marshal(driver.GetTelemetry())

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "iscsi", "linux").Return(nil)
	mockAPI.EXPECT().IscsiInitiatorGetDefaultAuth(ctx).Return(iscsiInitiatorAuth, nil)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
		string(message), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

	assert.NoError(t, result, "Ontap SAN storage driver initialization failed")
}

func TestOntapSanStorageDriverInitialize_WithFC(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"sanType":           "fcp",
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
	message, _ := json.Marshal(driver.GetTelemetry())

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().IgroupCreate(ctx, gomock.Any(), "fcp", "linux").Return(nil)
	mockAPI.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, "fcp").Return([]string{"10:20:30:40:50:60:70:80"}, nil)
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
		string(message), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().GetSVMUUID().Return("SVM1-uuid")

	result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

	assert.NoError(t, result, "Ontap SAN storage driver initialization failed")
}

func TestOntapSANStorageDriverInitialize_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	driver.API = nil // setting driver API nil

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
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

	assert.Error(t, result, "Ontap SAN driver initialization succeeded even with driver API is nil")
	assert.Contains(t, result.Error(), "could not create Data ONTAP API client")
}

func TestOntapSanStorageDriverInitialize_NetInterfaceGetDataLIFsFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
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
	message, _ := json.Marshal(driver.GetTelemetry())

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").
		Return(nil, fmt.Errorf("error in getting datalifs"))
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
		string(message), 1, "trident", 5).AnyTimes()

	result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "Ontap SAN driver initialized")
}

func TestOntapSanStorageDriverInitialize_FcpInterfaceGetFail(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
		"managementLIF":     "127.0.0.1:0",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"sanType":           "fcp",
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
	message, _ := json.Marshal(driver.GetTelemetry())

	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, "fcp").Return(nil, fmt.Errorf("error in getting datalifs"))
	mockAPI.EXPECT().EmsAutosupportLog(ctx, "ontap-san", "1", false, "heartbeat", "hostname",
		string(message), 1, "trident", 5).AnyTimes()

	result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "Ontap SAN driver initialized")
}

func TestOntapSANStorageDriverInitialize_StoragePoolFailed(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-san",
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
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "AggregatesNotFound",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return(nil, fmt.Errorf("aggregates not found"))
			},
			wantErr:       assert.NoError,
			assertMessage: "Get the ONTAP SAN storage pool aggregates",
		},
		{
			name: "AggregatesListEmpty",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
				mockAPI.EXPECT().GetSVMAggregateNames(ctx).Return([]string{}, nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Get the ONTAP SAN storage pool aggregates",
		},
		{
			name: "AggregatesIpListEmpty",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{}, nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Get the ips of ONTAP SAN storage pool aggregates",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver.Config.CommonStorageDriverConfig = nil
			test.mocks(mockAPI)
			mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()

			result := driver.Initialize(ctx, "CSI", configJSON, commonConfig, secrets, BackendUUID)

			assert.Error(t, result, test.assertMessage)
		})
	}
}

func TestOntapSANStorageDriverInitializeStoragePools_NameTemplatesAndLabels(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	defer mockCtrl.Finish()

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
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

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().GetSVMAggregateNames(gomock.Any()).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

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
			"san-backend",
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
			"san-backend",
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
			"san-backend",
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
			"san-backend",
			"Base label is not set correctly",
			"Virtual pool label is not set correctly",
			"volume name is not set correctly",
			"volume name is not set correctly",
		}, // base and virtual labels
	}

	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			d.Config.Labels = c.physicalPoolLabels
			d.Config.NameTemplate = c.physicalNameTemplate
			d.Config.Storage[0].Labels = c.virtualPoolLabels
			d.Config.Storage[0].NameTemplate = c.virtualNameTemplate
			physicalPools, virtualPools, err := InitializeStoragePoolsCommon(ctx, d, poolAttributes,
				c.backendName)
			assert.NoError(t, err, "Error is not nil")

			physicalPool := physicalPools["data"]
			label, err := physicalPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, c.physicalExpected, label, c.physicalErrorMessage)

			d.CreatePrepare(ctx, &volume, physicalPool)
			assert.Equal(t, volume.InternalName, c.volumeNamePhysicalExpected, c.physicalVolNameErrorMessage)

			virtualPool := virtualPools["san-backend_pool_0"]
			label, err = virtualPool.GetTemplatizedLabelsJSON(ctx, "provisioning", 1023, templateData)
			assert.NoError(t, err, "Error is not nil")
			assert.Equal(t, c.virtualExpected, label, c.virtualErrorMessage)

			d.CreatePrepare(ctx, &volume, virtualPool)
			assert.Equal(t, volume.InternalName, c.volumeNameVirtualExpected, c.virtualVolNameErrorMessage)
		})
	}
}

func getOntapSANStorageDriverConfigJson() ([]byte, error) {
	commonConfig := getCommonConfig()
	storagePrefix := "Test_"
	commonConfig.StoragePrefix = &storagePrefix

	ontapStorageDriverConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		ManagementLIF:             "127.0.0.1:0",
		SVM:                       "SVM1",
		Aggregate:                 "data",
		Username:                  "dummyuser",
		Password:                  "dummypassword",
		Storage:                   nil,
		ReplicationPolicy:         "fake-rep-policy",
		IgroupName:                "igroup1",
	}

	return json.Marshal(ontapStorageDriverConfig)
}

func TestOntapSANStorageDriverInitialize_ValidationFailed(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	driver.Config.CommonStorageDriverConfig = nil
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	secrets := map[string]string{
		"clientcertificate": "dummy-certificate",
	}

	configJSON, _ := getOntapSANStorageDriverConfigJson()
	mockAPI.EXPECT().IsSVMDRCapable(ctx).Return(true, nil).AnyTimes()
	mockAPI.EXPECT().SupportsFeature(ctx, gomock.Any()).AnyTimes().Return(true) // feature not supported
	mockAPI.EXPECT().GetSVMAggregateNames(ctx).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)
	mockAPI.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1"}, nil)
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(nil,
		fmt.Errorf("failed to validate replication policy"))
	mockAPI.EXPECT().IgroupDestroy(ctx, "igroup1").Return(fmt.Errorf("igroup not found"))

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, string(configJSON), commonConfig, secrets, BackendUUID)

	assert.Error(t, result,
		"SAN driver initialization succeeded even with ONTAP version does not support")
	assert.Contains(t, result.Error(), "failed to validate replication policy")
}

func TestOntapSANStorageDriverInitialized(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.initialized = true
	result := driver.Initialized()
	assert.Equal(t, true, result, "Incorrect initialization status")

	driver.initialized = false
	result = driver.Initialized()
	assert.Equal(t, false, result, "Incorrect initialization status")
}

func TestOntapSantorageDriverVolumeCanSnapshot(t *testing.T) {
	_, driver := newMockOntapSANDriver(t)
	result := driver.CanSnapshot(ctx, nil, nil)
	assert.NoError(t, result, "failed to check that snapshot is supported")
}

func TestOntapSANStorageDriverGetCommonConfig(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig

	config := driver.GetCommonConfig(ctx)

	assert.Equal(t, commonConfig.StorageDriverName, config.StorageDriverName)
	assert.Equal(t, commonConfig.BackendName, config.BackendName)
}

func TestOntapSANStorageDriverEstablishMirror(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig
	driver.Config.ReplicationSchedule = "none"
	driver.Config.ReplicationPolicy = "fake-rep-policy"

	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		SnapshotPolicy: "none",
		DPVolume:       true,
	}

	snapmirrorPolicy := api.SnapmirrorPolicy{
		Type: api.SnapmirrorPolicyZAPITypeAsync, CopyAllSnapshots: true,
	}
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(&snapmirrorPolicy, nil)
	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&volume, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)

	err := driver.EstablishMirror(ctx, "trident-pvc-1234", "svm1:vol1",
		"", "")

	assert.NoError(t, err, "Failed to established the snapshot mirror")
}

func TestOntapSANStorageDriverEstablishMirror_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig
	driver.Config.ReplicationSchedule = "none"
	driver.Config.ReplicationPolicy = "fake-rep-policy"

	volume := api.Volume{
		Name:           "vol1",
		Comment:        "",
		SnapshotPolicy: "none",
		DPVolume:       true,
	}

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "ReplicationPolicyValidation_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Times(2).Return(nil,
					fmt.Errorf("snap mirror fail"))
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&volume, nil)
				mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Validate the replication policy for establish mirror",
		},
		{
			name: "validateReplicationSchedule_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				snapmirrorPolicy := api.SnapmirrorPolicy{
					Type: api.SnapmirrorPolicyZAPITypeAsync, CopyAllSnapshots: true,
				}
				mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(&snapmirrorPolicy, nil)
				mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Return(&volume, nil)
				mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)
				mockAPI.EXPECT().JobScheduleExists(ctx, "none").Return(false, fmt.Errorf("job failed"))
			},
			wantErr:       assert.NoError,
			assertMessage: "Validate the replication schedule for establish mirror",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.EstablishMirror(ctx, "trident-pvc-1234", "svm1:vol1", "", "none")
			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSANStorageDriverReestablishMirror(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig
	driver.Config.ReplicationSchedule = "none"
	driver.Config.ReplicationPolicy = "fake-rep-policy"

	snapmirrorPolicy := api.SnapmirrorPolicy{
		Type: api.SnapmirrorPolicyZAPITypeAsync, CopyAllSnapshots: true,
	}
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(&snapmirrorPolicy, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)

	err := driver.ReestablishMirror(ctx, "trident-pvc-1234", "svm1:vol1", "", "")

	assert.NoError(t, err, "Failed to reestablished the snapshot mirror")
}

func TestOntapSANStorageDriverReestablishMirror_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig
	driver.Config.ReplicationSchedule = "none"
	driver.Config.ReplicationPolicy = "fake-rep-policy"

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "ReplicationPolicyValidation_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Times(2).Return(
					nil, fmt.Errorf("snap mirror fail"))
				mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Validate the replication policy for reestablish mirror",
		},
		{
			name: "validateReplicationSchedule_Fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				snapmirrorPolicy := api.SnapmirrorPolicy{
					Type: api.SnapmirrorPolicyZAPITypeAsync, CopyAllSnapshots: true,
				}
				mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(&snapmirrorPolicy, nil)
				mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)
				mockAPI.EXPECT().JobScheduleExists(ctx, "none").Return(false, fmt.Errorf("job failed"))
			},
			wantErr:       assert.NoError,
			assertMessage: "Validate the replication schedule for establish mirror",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.ReestablishMirror(ctx, "trident-pvc-1234", "svm1:vol1", "", "none")

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSANStorageDriverPromoteMirror(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.Config.ReplicationSchedule = "none"
	driver.Config.ReplicationPolicy = "fake-rep-policy"

	snapmirrorPolicy := api.SnapmirrorPolicy{
		Type: api.SnapmirrorPolicyZAPITypeAsync, CopyAllSnapshots: true,
	}
	mockAPI.EXPECT().SnapmirrorPolicyGet(ctx, gomock.Any()).Return(&snapmirrorPolicy, nil)
	mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		"snapshot-a", "volume-a").Return(
		api.Snapshot{Name: "snapshot-1", CreateTime: "1"},
		nil)

	mirror, err := driver.PromoteMirror(ctx, "volume-a",
		"svm1:vol1", "volume-a/snapshot-a")

	assert.NoError(t, err, "Failed to promote the snapshot mirror")
	assert.Equal(t, true, mirror)
}

func TestOntapSANStorageDriverGetMirrorStatus(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver.Config.ReplicationSchedule = "none"
	driver.Config.ReplicationPolicy = "fake-rep-policy"

	mockAPI.EXPECT().SnapmirrorGet(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&api.Snapmirror{State: api.SnapmirrorStateSynchronizing}, nil)

	mirror, err := driver.GetMirrorStatus(ctx, "volume-a", "svm1:vol1")

	assert.NoError(t, err, "Failed to get the snapshot mirror status")
	assert.Equal(t, "establishing", mirror)
}

func TestOntapSANStorageDriverReleaseMirror(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	mockAPI.EXPECT().SnapmirrorRelease(ctx, "volume-a", "SVM1")

	err := driver.ReleaseMirror(ctx, "volume-a")

	assert.NoError(t, err, "release mirror should not return an error")
}

func TestOntapSANStorageDriverGetReplicationDetails(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	ctx := context.Background()

	mockAPI.EXPECT().SVMName().Return("SVM1")
	mockAPI.EXPECT().SnapmirrorGet(ctx, "volume-a", "SVM1", "volume-a", "svm-1").Times(1).
		Return(&api.Snapmirror{ReplicationPolicy: "MirrorAllSnapshots", ReplicationSchedule: "1min"}, nil)

	policy, schedule, SVMName, err := driver.GetReplicationDetails(ctx, "volume-a", "svm-1:volume-a")

	assert.Equal(t, "MirrorAllSnapshots", policy, "policy should match what snapmirror returns")
	assert.Equal(t, "1min", schedule, "schedule should match what snapmirror returns")
	assert.Equal(t, "SVM1", SVMName, "SVM name should match")
	assert.NoError(t, err, "get replication details should not return an error")
}

func TestOntapSANStorageDriverUpdateMirror(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	ctx := context.Background()

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "SnapmirrorUpdate_success",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SnapmirrorUpdate(ctx, "trident-pvc-1234", "trident-pvc-1234-snap").Return(nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Snap mirror update failed",
		},
		{
			name: "SnapmirrorUpdate_fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SnapmirrorUpdate(ctx, "trident-pvc-1234", "trident-pvc-1234-snap").Return(
					fmt.Errorf("snapmirror update failed"))
			},
			wantErr:       assert.Error,
			assertMessage: "Snap mirror updated",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			err := driver.UpdateMirror(ctx, "trident-pvc-1234", "trident-pvc-1234-snap")

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
		})
	}
}

func TestOntapSANStorageDriverCheckMirrorTransferState(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	ctx := context.Background()

	transferTime := "2023-06-01T10:15:36-04:00"
	transferFormat := "2006-01-02T15:04:05-07:00"
	endTime, _ := time.Parse(transferFormat, transferTime)
	endTransferTime = endTime.UTC()

	tests := []struct {
		name          string
		mocks         func(mockAPI *mockapi.MockOntapAPI)
		wantErr       assert.ErrorAssertionFunc
		assertMessage string
	}{
		{
			name: "CheckMirrorTransferState_success",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SVMName().Return("SVM1")
				mockAPI.EXPECT().SnapmirrorGet(ctx, "trident-pvc-1234", "SVM1", "", "").
					Return(&api.Snapmirror{
						RelationshipStatus: api.SnapmirrorStatusSuccess,
						IsHealthy:          true,
						EndTransferTime:    &endTransferTime,
					}, nil)
			},
			wantErr:       assert.NoError,
			assertMessage: "Mirror transfer state is failed",
		},
		{
			name: "CheckMirrorTransferState_fail",
			mocks: func(mockAPI *mockapi.MockOntapAPI) {
				mockAPI.EXPECT().SVMName().Return("SVM1")
				mockAPI.EXPECT().SnapmirrorGet(ctx, "trident-pvc-1234", "SVM1", "", "").
					Return(&api.Snapmirror{
						RelationshipStatus: api.SnapmirrorStatusFailed,
						UnhealthyReason:    "transfer failed",
						EndTransferTime:    nil,
					}, nil)
			},
			wantErr:       assert.Error,
			assertMessage: "Mirror transfer state is success",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mocks(mockAPI)

			endTransferTime, err := driver.CheckMirrorTransferState(ctx, "trident-pvc-1234")

			if !test.wantErr(t, err, test.assertMessage) {
				return
			}
			if endTransferTime != nil {
				assert.Equal(t, endTransferTime.String(), endTransferTime.String())
			}
		})
	}
}

func TestOntapSANStorageDriverEnablePublishEnforcement(t *testing.T) {
	ctx := context.Background()
	mockAPI, driver := newMockOntapSANDriver(t)

	volName := "trident_pvc_63a8ea3d_4213_4753_8b38_2da69c178ed0"
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

	assert.NoError(t, err, "Failed to enable publish enforcement")
	assert.True(t, volume.Config.AccessInfo.PublishEnforcement)
	assert.Equal(t, int32(-1), volume.Config.AccessInfo.IscsiAccessInfo.IscsiLunNumber)
}

func TestOntapSANStorageDriverCanEnablePublishEnforcement(t *testing.T) {
	_, driver := newMockOntapSANDriver(t)
	enforcement := driver.CanEnablePublishEnforcement()

	assert.Equal(t, true, enforcement)
}

func TestOntapSANStorageDriverGetChapInfo(t *testing.T) {
	ctx := context.Background()
	_, driver := newMockOntapSANDriver(t)

	driver.Config = drivers.OntapStorageDriverConfig{
		UseCHAP:                   true,
		ChapUsername:              "foo",
		ChapInitiatorSecret:       "bar",
		ChapTargetUsername:        "baz",
		ChapTargetInitiatorSecret: "biz",
	}

	iscsiChapInfo, err := driver.GetChapInfo(ctx, "", "")

	assert.NoError(t, err, "Failed to get the chap info")
	assert.Equal(t, driver.Config.UseCHAP, iscsiChapInfo.UseCHAP)
	assert.Equal(t, driver.Config.ChapUsername, iscsiChapInfo.IscsiUsername)
	assert.Equal(t, driver.Config.ChapInitiatorSecret, iscsiChapInfo.IscsiInitiatorSecret)
	assert.Equal(t, driver.Config.ChapTargetUsername, iscsiChapInfo.IscsiTargetUsername)
	assert.Equal(t, driver.Config.ChapTargetInitiatorSecret, iscsiChapInfo.IscsiTargetSecret)
}

func TestOntapSANStorageDriverGetProtocol(t *testing.T) {
	ctx := context.Background()
	_, driver := newMockOntapSANDriver(t)
	protocol := driver.GetProtocol(ctx)

	assert.Equal(t, tridentconfig.Block, protocol)
}

func TestOntapSANStorageDriverStoreConfig(t *testing.T) {
	ctx := context.Background()
	_, driver := newMockOntapSANDriver(t)
	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig
	persistentStorageBackendConfig := storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, &persistentStorageBackendConfig)

	assert.Equal(t, driver.Config.StorageDriverName, persistentStorageBackendConfig.OntapConfig.StorageDriverName)
}

func TestOntapSanVolumeGetSnapshot(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	snapshotConfig := &storage.SnapshotConfig{
		InternalName:       "trident-pvc-1234_snap",
		VolumeInternalName: "trident-pvc-1234",
		VolumeName:         "vol1",
	}

	volConfig := getVolumeConfig()

	mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
	mockAPI.EXPECT().VolumeSnapshotInfo(ctx,
		snapshotConfig.InternalName, snapshotConfig.VolumeInternalName).Return(
		api.Snapshot{
			CreateTime: "time",
			Name:       snapshotConfig.InternalName,
		},
		nil)

	snapshot, err := driver.GetSnapshot(ctx, snapshotConfig, &volConfig)

	assert.NoError(t, err, "Failed to get a snaphot")
	assert.Equal(t, "vol1", snapshot.Config.VolumeName)
	assert.Equal(t, "trident-pvc-1234_snap", snapshot.Config.InternalName)
}

func TestOntapSanVolumeGetSnapshots(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()

	snapshots := api.Snapshots{}
	snapshots = append(snapshots, api.Snapshot{
		CreateTime: "time",
		Name:       "trident-pvc-1234_snap",
	})

	mockAPI.EXPECT().LunSize(ctx, "/vol/trident-pvc-1234/lun0").Return(1073741824, nil)
	mockAPI.EXPECT().VolumeSnapshotList(ctx, "trident-pvc-1234").Return(snapshots, nil)

	snapshot, err := driver.GetSnapshots(ctx, &volConfig)

	assert.NoError(t, err, "Failed to get a snaphot list")
	assert.Equal(t, "vol1", snapshot[0].Config.VolumeName)
	assert.Equal(t, "trident-pvc-1234_snap", snapshot[0].Config.InternalName)
}

func TestOntapSanVolumeCreatePrepare_EnablePublishEnforcement(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	volConfig := getVolumeConfig()
	volConfig.ImportNotManaged = false
	pool := storage.NewStoragePool(nil, "dummyPool")

	commonConfig := getCommonConfig()

	driver.Config.CommonStorageDriverConfig = commonConfig

	tridentconfig.CurrentDriverContext = tridentconfig.ContextCSI

	driver.CreatePrepare(ctx, &volConfig, pool)
	assert.Equal(t, true, volConfig.AccessInfo.PublishEnforcement)
}

func TestOntapSanVolumeValidate(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	storagePrefix := "trident_"
	commonConfig := getCommonConfig()
	commonConfig.StoragePrefix = &storagePrefix

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	driver.Config = config
	driver.ips = ips

	driver.Config.CommonStorageDriverConfig = commonConfig

	err := driver.validate(ctx)
	assert.NoError(t, err, "Failed to validate ONTAP SAN driver configuration")
}

func TestOntapSanVolumeValidate_ValidateSANDriver(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().EmsAutosupportLog(ctx, gomock.Any(), "1", false, "heartbeat",
		gomock.Any(), gomock.Any(), 1, "trident", 5).AnyTimes()
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	driver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, nil, mockAPI)
	driver.API = mockAPI

	storagePrefix := "trident&#"
	commonConfig := getCommonConfig()
	commonConfig.StoragePrefix = &storagePrefix
	commonConfig.DriverContext = tridentconfig.ContextDocker

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	driver.Config = config
	driver.ips = ips

	driver.Config.CommonStorageDriverConfig = commonConfig

	err := driver.validate(ctx)
	assert.Error(t, err, "Validate the ONTAP SAN driver")
}

func TestOntapSanVolumeValidate_ValidateStoragePrefix(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	storagePrefix := "trident&#"
	commonConfig := getCommonConfig()
	commonConfig.StoragePrefix = &storagePrefix

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	driver.Config = config
	driver.ips = ips

	driver.Config.CommonStorageDriverConfig = commonConfig

	err := driver.validate(ctx)
	assert.Error(t, err, "Validate the ONTAP SAN storage prefix")
}

func TestOntapSanVolumeValidate_ValidateStoragePools(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	storagePrefix := "trident_"
	commonConfig := getCommonConfig()
	commonConfig.StoragePrefix = &storagePrefix

	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "::1", "127.0.0.1"}

	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AutoExportPolicy:          true,
		UseCHAP:                   true,
		DataLIF:                   "1.1.1.1",
	}

	driver.Config = config
	driver.ips = ips

	physicalPools := map[string]storage.Pool{}
	virtualPools := map[string]storage.Pool{"test": getValidOntapNASPool()}
	driver.virtualPools = virtualPools
	driver.physicalPools = physicalPools
	driver.virtualPools["test"].InternalAttributes()[SecurityStyle] = "invalidValue"

	driver.Config.CommonStorageDriverConfig = commonConfig

	err := driver.validate(ctx)
	assert.Error(t, err, "Unexpected error")
}

func TestOntapSanStorageDriverVolumeRestoreSnapshot(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)
	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SnapshotRestoreVolume(ctx, "snap1", "vol1").Return(nil)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.NoError(t, result)
}

func TestOntapSanStorageDriverVolumeRestoreSnapshot_Failure(t *testing.T) {
	mockAPI, driver := newMockOntapSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "nfs",
		InternalName: "vol1",
	}

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	mockAPI.EXPECT().SnapshotRestoreVolume(ctx, "snap1", "vol1").Return(fmt.Errorf("failed to restore volume"))

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result)
}

func TestOntapSanVolumeGroupSnapshot(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
	snapInfoResult := api.Snapshot{CreateTime: "1"}

	mockAPI.EXPECT().ConsistencyGroupSnapshot(ctx, snapName, gomock.InAnyOrder(storageVolNames)).Return(nil).Times(1)

	mockAPI.EXPECT().VolumeSnapshotInfo(ctx, snapName, gomock.Any()).Return(snapInfoResult, nil).Times(2)
	mockAPI.EXPECT().LunSize(ctx, gomock.Any()).Return(1073741824, nil).Times(2)

	groupSnapshot, snapshots, err := driver.CreateGroupSnapshot(ctx, groupSnapshotConfig, targetInfo)

	assert.Equal(t, groupSnapshot.ID(), groupSnapshotConfig.ID())
	assert.Equal(t, groupSnapshot.GetVolumeNames(), groupSnapshotConfig.GetVolumeNames())

	for _, snap := range snapshots {
		assert.Contains(t, groupSnapshot.GetSnapshotIDs(), snap.ID())
	}

	assert.NoError(t, err, "Group snapshot creation failed")
}

func TestOntapSanVolumeGroupTarget(t *testing.T) {
	ctx := context.Background()

	mockAPI, driver := newMockOntapSANDriver(t)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

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
