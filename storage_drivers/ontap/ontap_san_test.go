// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
)

func TestOntapSanStorageDriverConfigString(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	ontapSanDrivers := []SANStorageDriver{
		*newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, true, mockAPI),
		*newTestOntapSANDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, false, mockAPI),
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
	vserverAdminHost, vserverAdminPort, vserverAggrName string, useREST bool, apiOverride api.OntapAPI,
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
	config.UseREST = useREST

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
		if config.UseREST {
			ontapAPI, _ = api.NewRestClientFromOntapConfig(context.TODO(), config)
		} else {
			ontapAPI, _ = api.NewZAPIClientFromOntapConfig(context.TODO(), config, numRecords)
		}
	}

	sanDriver.API = ontapAPI
	sanDriver.telemetry = &Telemetry{
		Plugin:        sanDriver.Name(),
		SVM:           ontapAPI.SVMName(),
		StoragePrefix: *sanDriver.GetConfig().StoragePrefix,
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

			sanStorageDriver := newTestOntapSANDriver(vserverAdminHost, port, vserverAggrName, false, nil)
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

	mockAPI.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), fsType, gomock.Any(), luks).DoAndReturn(
		func(ctx context.Context, lunPath, attribute, fstype, context, luks string) error {
			return nil
		},
	).MaxTimes(1)
}

func TestOntapSanVolumeCreate(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	luks := "true"
	expectLunAndVolumeCreateSequence(ctx, mockAPI, "xfs", luks)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI

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
	})
	d.physicalPools = map[string]storage.Pool{"pool1": pool1}

	volConfig := &storage.VolumeConfig{
		Size:       "1g",
		Encryption: "false",
		FileSystem: "xfs",
	}
	volAttrs := map[string]sa.Request{}

	err := d.Create(ctx, volConfig, pool1, volAttrs)

	assert.Nil(t, err, "Error is not nil")
	assert.Equal(t, "none", volConfig.SpaceReserve)
	assert.Equal(t, "fake-snap-policy", volConfig.SnapshotPolicy)
	assert.Equal(t, "10", volConfig.SnapshotReserve)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "false", volConfig.SnapshotDir)
	assert.Equal(t, "fake-export-policy", volConfig.ExportPolicy)
	assert.Equal(t, "mixed", volConfig.SecurityStyle)
	assert.Equal(t, "false", volConfig.Encryption)
	assert.Equal(t, "fake-qos-policy", volConfig.QosPolicy)
	assert.Equal(t, "", volConfig.AdaptiveQosPolicy)
	assert.Equal(t, "true", volConfig.LUKSEncryption)
	assert.Equal(t, "xfs", volConfig.FileSystem)
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
			want: &utils.IscsiChapInfo{
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
			want: &utils.IscsiChapInfo{
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
	}
	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			volConfig := &storage.VolumeConfig{
				InternalName: "foo",
				AccessInfo:   utils.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			publishInfo := &utils.VolumePublishInfo{
				HostName:         "bar",
				TridentUUID:      "1234",
				VolumeAccessInfo: utils.VolumeAccessInfo{PublishEnforcement: tr.args.publishEnforcement},
			}

			igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
			lunPath := lunPath(volConfig.InternalName)

			mockCtrl := gomock.NewController(t)
			mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

			mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

			d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
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

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
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

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"1.1.1.1"}, nil)

	err := d.Publish(ctx, volConfig, publishInfo)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanVolumePublishUnmanaged(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
	d.ips = []string{"127.0.0.1"}

	volConfig := &storage.VolumeConfig{
		InternalName: "lunName",
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	}

	publishInfo := &utils.VolumePublishInfo{
		HostName:    "bar",
		HostIQN:     []string{"host_iqn"},
		TridentUUID: "1234",
		// VolumeAccessInfo: utils.VolumeAccessInfo{PublishEnforcement: true},
		Unmanaged: true,
	}

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"1.1.1.1"}, nil)

	err := d.Publish(ctx, volConfig, publishInfo)
	assert.Nil(t, err, "Error is not nil")
}

func TestOntapSanVolumePublishSLMError(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")

	d := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockAPI)
	d.API = mockAPI
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

	mockAPI.EXPECT().VolumeInfo(ctx, gomock.Any()).Times(1).Return(&api.Volume{AccessType: VolTypeRW}, nil)
	mockAPI.EXPECT().IscsiNodeGetNameRequest(ctx).Times(1).Return("node1", nil)
	mockAPI.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return([]string{"iscsi_if"}, nil).Times(1)
	mockAPI.EXPECT().LunGetFSType(ctx, "/vol/lunName/lun0")
	mockAPI.EXPECT().EnsureIgroupAdded(ctx, gomock.Any(), gomock.Any()).Times(1)
	mockAPI.EXPECT().EnsureLunMapped(ctx, gomock.Any(), gomock.Any()).Times(1).Return(1, nil)
	mockAPI.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{"node1"}, nil)
	mockAPI.EXPECT().GetSLMDataLifs(ctx, gomock.Any(), gomock.Any()).Times(1).Return([]string{}, nil)

	err := d.Publish(ctx, volConfig, publishInfo)
	assert.Errorf(t, err, "no reporting nodes found")
}

func TestSANStorageDriverGetBackendState(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockApi := mockapi.NewMockOntapAPI(mockCtrl)

	mockApi.EXPECT().SVMName().AnyTimes().Return("SVM1")

	mockDriver := newTestOntapSANDriver(ONTAPTEST_LOCALHOST, "0", ONTAPTEST_VSERVER_AGGR_NAME, true, mockApi)
	mockDriver.API = mockApi

	mockApi.EXPECT().GetSVMState(ctx).Return("", fmt.Errorf("returning test error"))

	reason, changeMap := mockDriver.GetBackendState(ctx)
	assert.Equal(t, reason, StateReasonSVMUnreachable, "should be 'SVM is not reachable'")
	assert.NotNil(t, changeMap, "should not be nil")
}
