// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func newTestOntapNASFlexgroupDriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, driverContext tridentconfig.DriverContext, useREST bool,
) *NASFlexGroupStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-nas-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas-flexgroup"
	config.StoragePrefix = sp("test_")
	config.DriverContext = driverContext
	config.UseREST = useREST

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

func newMockOntapNASFlexgroupDriver(t *testing.T) (*mockapi.MockOntapAPI, *NASFlexGroupStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	driver := newTestOntapNASFlexgroupDriver(vserverAdminHost, vserverAdminPort, vserverAggrName, "CSI", false)
	driver.API = mockAPI
	return mockAPI, driver
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
		Size:             "1g",
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

	// mockAPI.EXPECT().SVMName().AnyTimes().Return("fakesvm")
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
