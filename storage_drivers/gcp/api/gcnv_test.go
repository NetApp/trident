// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/netapp/apiv1/netapppb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
)

func TestRegisterStoragePools(t *testing.T) {
	storagePoolMap := make(map[string]storage.Pool)

	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[serviceLevel] = ServiceLevelPremium
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[serviceLevel] = ServiceLevelStandard

	storagePoolMap[sPool1.Name()] = sPool1
	storagePoolMap[sPool2.Name()] = sPool2

	sdk := getFakeSDK()

	sdk.registerStoragePools(storagePoolMap)
}

func TestCreateBaseId(t *testing.T) {
	sdk := getFakeSDK()
	actual := sdk.createBaseID("fake-location")

	expected := "projects/123456789/locations/fake-location"

	assert.Equal(t, expected, actual, "Base IDs is not equal")
}

func TestCreateCapacityPoolId(t *testing.T) {
	sdk := getFakeSDK()
	actual := sdk.createCapacityPoolID("fake-location", "cPool1")

	expected := "projects/123456789/locations/fake-location/storagePools/cPool1"

	assert.Equal(t, expected, actual, "Capacity Pool IDs is not equal")
}

func TestParseCapacityPoolID(t *testing.T) {
	project, location, capacityPool, err := parseCapacityPoolID("projects/123456789/locations/fake-location/storagePools/myCapacityPool")
	assert.Equal(t, "123456789", project, "project not correct")
	assert.Equal(t, "fake-location", location, "location not correct")
	assert.Equal(t, "myCapacityPool", capacityPool, "capacityPool not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseCapacityPoolIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no project number key",
			"/myProject/locations/fake-location/storagePools/myCapacityPool",
		},
		{
			"no project number value",
			"projects/locations/fake-location/storagePools/myCapacityPool",
		},
		{
			"no location key",
			"projects/myProject/fake-location/storagePools/myCapacityPool",
		},
		{
			"no location value",
			"projects/myProject/locations/storagePools/myCapacityPool",
		},
		{
			"no capacity pools key",
			"projects/myProject/locations/fake-location/myCapacityPool",
		},
		{
			"no capacity pools value",
			"projects/myProject/locations/fake-location/storagePools",
		},
	}

	for _, test := range tests {

		_, _, _, err := parseCapacityPoolID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestCreateVolumeId(t *testing.T) {
	sdk := getFakeSDK()
	actual := sdk.createVolumeID("fake-location", "vol1")

	expected := "projects/123456789/locations/fake-location/volumes/vol1"

	assert.Equal(t, expected, actual, "Volume IDs is not equal")
}

func TestParseVolumeID(t *testing.T) {
	project, location, volume, err := parseVolumeID("projects/myProject/locations/fake-location/volumes/myVolume")

	assert.Equal(t, "myProject", project, "project not correct")
	assert.Equal(t, "fake-location", location, "location not correct")
	assert.Equal(t, "myVolume", volume, "volume not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseVolumeIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no project number key",
			"/myProject/locations/fake-location/volumes/myVolume",
		},
		{
			"no project number value",
			"projects/locations/fake-location/volumes/myVolume",
		},
		{
			"no location key",
			"projects/myProject/fake-location/volumes/myVolume",
		},
		{
			"no location value",
			"projects/myProject/locations/volumes/myVolume",
		},
		{
			"no volume key",
			"projects/myProject/locations/fake-location/myVolume",
		},
		{
			"no capacity pools value",
			"projects/myProject/locations/fake-location/volumes",
		},
	}

	for _, test := range tests {

		_, _, _, err := parseVolumeID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestCreateSnapshotID(t *testing.T) {
	sdk := getFakeSDK()
	actual := sdk.createSnapshotID("fake-location", "vol1", "snap1")

	expected := "projects/123456789/locations/fake-location/volumes/vol1/snapshots/snap1"

	assert.Equal(t, expected, actual, "Snapshot IDs is not equal")
}

func TestParseSnapshotID(t *testing.T) {
	project, location, volume, snapshot, err := parseSnapshotID("projects/myProject/locations/fake-location/volumes/myVolume/snapshots/mySnapshot")

	assert.Equal(t, "myProject", project, "project not correct")
	assert.Equal(t, "fake-location", location, "location not correct")
	assert.Equal(t, "myVolume", volume, "volume not correct")
	assert.Equal(t, "mySnapshot", snapshot, "snapshot not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseSnapshotIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no project number key",
			"/myProject/locations/fake-location/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no project number value",
			"projects/locations/fake-location/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no location key",
			"projects/myProject/fake-location/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no location value",
			"projects/myProject/locations/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no volume key",
			"projects/myProject/locations/fake-location/myVolume/snapshots/mySnapshot",
		},
		{
			"no volume value",
			"projects/myProject/locations/fake-location/volumes/snapshots/mySnapshot",
		},
		{
			"no snapshot key",
			"projects/myProject/locations/fake-location/volumes/myVolume/mySnapshot",
		},
		{
			"no snapshot value",
			"projects/myProject/locations/fake-location/volumes/myVolume/snapshots",
		},
	}

	for _, test := range tests {
		_, _, _, _, err := parseSnapshotID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestCreateNetworkId(t *testing.T) {
	sdk := getFakeSDK()
	actual := sdk.createNetworkID("network1")

	expected := "projects/123456789/global/networks/network1"

	assert.Equal(t, expected, actual, "Network IDs is not equal")
}

func TestParseNetworkID(t *testing.T) {
	project, network, err := parseNetworkID("projects/myProject/global/networks/myNetwork")

	assert.Equal(t, "myProject", project, "project not correct")
	assert.Equal(t, "myNetwork", network, "network not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseNetworkIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no project number key",
			"/myProject/global/networks/myNetwork",
		},
		{
			"no project number value",
			"projects/global/networks/myNetwork",
		},
		{
			"no network key",
			"projects/myProject/global/myNetwork",
		},
		{
			"no network value",
			"projects/myProject/global/networks",
		},
	}

	for _, test := range tests {

		_, _, err := parseNetworkID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestExportPolicyExportImport(t *testing.T) {
	sdk := getFakeSDK()

	rules := []ExportRule{
		{
			AllowedClients: "10.10.10.0/24",
			Nfsv3:          true,
			Nfsv4:          false,
			RuleIndex:      0,
			AccessType:     "ReadWrite",
		},
		{
			AllowedClients: "10.10.20.0/24",
			Nfsv3:          false,
			Nfsv4:          false,
			RuleIndex:      1,
			AccessType:     "ReadOnly",
		},
	}

	exportPolicy := &ExportPolicy{
		Rules: rules,
	}

	exportResult := exportPolicyExport(exportPolicy)

	assert.Equal(t, 2, len(exportResult.Rules))
	assert.Equal(t, "10.10.10.0/24", *((*exportResult).Rules)[0].AllowedClients)
	assert.Equal(t, "10.10.20.0/24", *((*exportResult).Rules)[1].AllowedClients)

	importResult := sdk.exportPolicyImport(exportResult)

	assert.Equal(t, exportPolicy, importResult)
}

func TestNewVolumeFromGCNVVolumeNilVolume(t *testing.T) {
	sdk := getFakeSDK()

	_, err := sdk.newVolumeFromGCNVVolume(ctx, nil)

	assert.Error(t, err, "error is not nil")
}

func TestNewVolumeFromGCNVVolumeWrongVolumeName(t *testing.T) {
	sdk := getFakeSDK()

	volume := &netapppb.Volume{
		Name: "projec/123456789/locations/fake-location/volumes/myVolume",
	}

	_, err := sdk.newVolumeFromGCNVVolume(ctx, volume)

	assert.Error(t, err, "volume id is not correct")
}

func TestNewVolumeFromGCNVVolumeWrongNetworkName(t *testing.T) {
	sdk := getFakeSDK()

	volume := &netapppb.Volume{
		Name:    "projects/123456789/locations/fake-location/volumes/myVolume",
		Network: "projects/123456789/global/myNetwork",
	}

	_, err := sdk.newVolumeFromGCNVVolume(ctx, volume)

	assert.Error(t, err, "network id is not correct")
}

func TestNewVolumeFromGCNVVolume(t *testing.T) {
	sdk := getFakeSDK()

	rules := []ExportRule{
		{
			AllowedClients: "10.10.10.0/24",
			Nfsv3:          true,
			Nfsv4:          false,
			RuleIndex:      0,
			AccessType:     "ReadWrite",
		},
	}

	exportPolicy := &ExportPolicy{
		Rules: rules,
	}
	newExportPolicy := exportPolicyExport(exportPolicy)

	volume := &netapppb.Volume{
		Name:              "projects/123456789/locations/fake-location/volumes/myVolume",
		ShareName:         "myVolume",
		Network:           "projects/123456789/global/networks/myNetwork",
		StoragePool:       "CP1",
		ServiceLevel:      netapppb.ServiceLevel_SERVICE_LEVEL_UNSPECIFIED,
		Protocols:         []netapppb.Protocols{netapppb.Protocols_NFSV3},
		MountOptions:      []*netapppb.MountOption{},
		CapacityGib:       100,
		UnixPermissions:   "777",
		Labels:            map[string]string{},
		SnapReserve:       0,
		State:             netapppb.Volume_STATE_UNSPECIFIED,
		SnapshotDirectory: false,
		SecurityStyle:     netapppb.SecurityStyle_UNIX,
		ExportPolicy:      newExportPolicy,
	}

	actual, _ := sdk.newVolumeFromGCNVVolume(ctx, volume)

	expected := &Volume{
		Name:                      "myVolume",
		CreationToken:             "myVolume",
		FullName:                  "projects/123456789/locations/fake-location/volumes/myVolume",
		Location:                  "fake-location",
		State:                     "Unspecified",
		CapacityPool:              "CP1",
		NetworkName:               "myNetwork",
		NetworkFullName:           "projects/123456789/global/networks/myNetwork",
		ServiceLevel:              "Premium",
		SizeBytes:                 107374182400,
		ExportPolicy:              exportPolicy,
		ProtocolTypes:             []string{"NFSv3"},
		MountTargets:              []MountTarget{},
		UnixPermissions:           "777",
		Labels:                    map[string]string{},
		SnapshotReserve:           0,
		SnapshotDirectory:         false,
		SecurityStyle:             "Unix",
		TieringPolicy:             "none",
		TieringMinimumCoolingDays: nil,
	}

	assert.Equal(t, expected, actual, "Volume is not equal")
}

func TestNewVolumeFromGCNVVolumeTieringEnabled(t *testing.T) {
	sdk := getFakeSDK()

	coolingDays := int32(30)
	tierAction := netapppb.TieringPolicy_ENABLED
	volume := &netapppb.Volume{
		Name:        "projects/123456789/locations/fake-location/volumes/myVolume",
		ShareName:   "myVolume",
		Network:     "projects/123456789/global/networks/myNetwork",
		StoragePool: "CP1",
		Protocols:   []netapppb.Protocols{netapppb.Protocols_NFSV3},
		CapacityGib: 100,
		TieringPolicy: &netapppb.TieringPolicy{
			TierAction:           &tierAction,
			CoolingThresholdDays: &coolingDays,
		},
	}

	actual, err := sdk.newVolumeFromGCNVVolume(ctx, volume)
	assert.NoError(t, err)
	assert.NotNil(t, actual)
	assert.Equal(t, drivers.TieringPolicyAuto, actual.TieringPolicy)
	if assert.NotNil(t, actual.TieringMinimumCoolingDays) {
		assert.Equal(t, coolingDays, *actual.TieringMinimumCoolingDays)
	}
}

func TestNewVolumeFromGCNVVolumeTieringPaused(t *testing.T) {
	sdk := getFakeSDK()

	coolingDays := int32(30)
	tierAction := netapppb.TieringPolicy_PAUSED
	volume := &netapppb.Volume{
		Name:        "projects/123456789/locations/fake-location/volumes/myVolume",
		ShareName:   "myVolume",
		Network:     "projects/123456789/global/networks/myNetwork",
		StoragePool: "CP1",
		Protocols:   []netapppb.Protocols{netapppb.Protocols_NFSV3},
		CapacityGib: 100,
		TieringPolicy: &netapppb.TieringPolicy{
			TierAction:           &tierAction,
			CoolingThresholdDays: &coolingDays,
		},
	}

	actual, err := sdk.newVolumeFromGCNVVolume(ctx, volume)
	assert.NoError(t, err)
	assert.NotNil(t, actual)

	// Trident models PAUSED as policy "none".
	assert.Equal(t, drivers.TieringPolicyNone, actual.TieringPolicy)
	assert.Nil(t, actual.TieringMinimumCoolingDays)
}

func TestGetMountTargetsFromVolume(t *testing.T) {
	sdk := getFakeSDK()

	volume := &netapppb.Volume{
		Name:        "projects/123456789/locations/fake-location/volumes/myVolume",
		ShareName:   "myVolume",
		Network:     "projects/123456789/global/networks/myNetwork",
		StoragePool: "CP1",
		Protocols:   []netapppb.Protocols{netapppb.Protocols_NFSV3},
		CapacityGib: 100,
		MountOptions: []*netapppb.MountOption{
			{
				Export:     "/export/path",
				ExportFull: "/users/export/path",
				Protocol:   netapppb.Protocols_NFSV3,
			},
		},
	}

	actual := sdk.getMountTargetsFromVolume(ctx, volume)

	expected := []MountTarget{
		{
			Export:     "/export/path",
			ExportPath: "/users/export/path",
			Protocol:   "NFSv3",
		},
	}

	assert.Equal(t, expected, actual, "Mount Targets are not equal")
}

func TestNewSnapshotFromGCNVSnapshotWrongSnapshotID(t *testing.T) {
	sdk := getFakeSDK()

	snapshot := &netapppb.Snapshot{
		Name: "projec/123456789/locations/fake-location/volumes/myVolume/snapshots/mySnapshot",
	}

	_, err := sdk.newSnapshotFromGCNVSnapshot(ctx, snapshot)

	assert.Error(t, err, "snapshot id is not correct")
}

func TestNewSnapshotFromGCNVSnapshot(t *testing.T) {
	sdk := getFakeSDK()

	snapshot := &netapppb.Snapshot{
		Name:        "projects/123456789/locations/fake-location/volumes/myVolume/snapshots/mySnapshot",
		Description: "mySnapshot",
		State:       netapppb.Snapshot_STATE_UNSPECIFIED,
		Labels:      map[string]string{},
		CreateTime: &timestamppb.Timestamp{
			Seconds: 123456789,
		},
	}

	actual, _ := sdk.newSnapshotFromGCNVSnapshot(ctx, snapshot)

	expected := &Snapshot{
		Name:     "mySnapshot",
		FullName: "projects/123456789/locations/fake-location/volumes/myVolume/snapshots/mySnapshot",
		Volume:   "myVolume",
		Location: "fake-location",
		State:    "Unspecified",
		Labels:   map[string]string{},
		Created:  snapshot.CreateTime.AsTime(),
	}

	assert.Equal(t, expected, actual, "Snapshot is not equal")
}

func TestServiceLevelFromCapacityPool(t *testing.T) {
	sdk := getFakeSDK()

	pool := storage.NewStoragePool(nil, "pool1")
	pool.InternalAttributes()[serviceLevel] = ServiceLevelPremium

	serviceLevel := ServiceLevelFromCapacityPool(
		sdk.sdkClient.resources.GetCapacityPools().Get("projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1"),
	)

	assert.Equal(t, ServiceLevelPremium, serviceLevel)
}

func TestServiceLevelFromCapacityPoolNil(t *testing.T) {
	serviceLevel := ServiceLevelFromCapacityPool(nil)

	assert.Equal(t, "Unspecified", serviceLevel)
}

func TestServiceLevelFromGCNVServiceLevel(t *testing.T) {
	serviceLevelPremium := ServiceLevelFromGCNVServiceLevel(netapppb.ServiceLevel_PREMIUM)
	serviceLevelExtreme := ServiceLevelFromGCNVServiceLevel(netapppb.ServiceLevel_EXTREME)
	serviceLevelStandard := ServiceLevelFromGCNVServiceLevel(netapppb.ServiceLevel_STANDARD)
	serviceLevelUnspecified := ServiceLevelFromGCNVServiceLevel(netapppb.ServiceLevel_SERVICE_LEVEL_UNSPECIFIED)
	serviceLevelFlex := ServiceLevelFromGCNVServiceLevel(netapppb.ServiceLevel_FLEX)

	assert.Equal(t, "Premium", serviceLevelPremium)
	assert.Equal(t, "Extreme", serviceLevelExtreme)
	assert.Equal(t, "Standard", serviceLevelStandard)
	assert.Equal(t, "Unspecified", serviceLevelUnspecified)
	assert.Equal(t, "Flex", serviceLevelFlex)
}

func TestGetStoragePoolStateFromGCNVState(t *testing.T) {
	readyState := StoragePoolStateFromGCNVState(netapppb.StoragePool_READY)
	unspecifiedState := StoragePoolStateFromGCNVState(netapppb.StoragePool_STATE_UNSPECIFIED)
	creatingState := StoragePoolStateFromGCNVState(netapppb.StoragePool_CREATING)
	deletingState := StoragePoolStateFromGCNVState(netapppb.StoragePool_DELETING)
	updatingState := StoragePoolStateFromGCNVState(netapppb.StoragePool_UPDATING)
	restoreState := StoragePoolStateFromGCNVState(netapppb.StoragePool_RESTORING)
	disabledState := StoragePoolStateFromGCNVState(netapppb.StoragePool_DISABLED)
	errorState := StoragePoolStateFromGCNVState(netapppb.StoragePool_ERROR)

	assert.Equal(t, "Ready", readyState)
	assert.Equal(t, "Unspecified", unspecifiedState)
	assert.Equal(t, "Creating", creatingState)
	assert.Equal(t, "Deleting", deletingState)
	assert.Equal(t, "Updating", updatingState)
	assert.Equal(t, "Restoring", restoreState)
	assert.Equal(t, "Disabled", disabledState)
	assert.Equal(t, "Error", errorState)
}

func TestVolumeStateFromGCNVState(t *testing.T) {
	readyState := VolumeStateFromGCNVState(netapppb.Volume_READY)
	unspecifiedState := VolumeStateFromGCNVState(netapppb.Volume_STATE_UNSPECIFIED)
	creatingState := VolumeStateFromGCNVState(netapppb.Volume_CREATING)
	deletingState := VolumeStateFromGCNVState(netapppb.Volume_DELETING)
	updatingState := VolumeStateFromGCNVState(netapppb.Volume_UPDATING)
	restoreState := VolumeStateFromGCNVState(netapppb.Volume_RESTORING)
	disabledState := VolumeStateFromGCNVState(netapppb.Volume_DISABLED)
	errorState := VolumeStateFromGCNVState(netapppb.Volume_ERROR)

	assert.Equal(t, "Ready", readyState)
	assert.Equal(t, "Unspecified", unspecifiedState)
	assert.Equal(t, "Creating", creatingState)
	assert.Equal(t, "Deleting", deletingState)
	assert.Equal(t, "Updating", updatingState)
	assert.Equal(t, "Restoring", restoreState)
	assert.Equal(t, "Disabled", disabledState)
	assert.Equal(t, "Error", errorState)
}

func TestVolumeSecurityStyleFromGCNVSecurityStyle(t *testing.T) {
	ntfs := VolumeSecurityStyleFromGCNVSecurityStyle(netapppb.SecurityStyle_NTFS)
	unix := VolumeSecurityStyleFromGCNVSecurityStyle(netapppb.SecurityStyle_UNIX)
	unspecified := VolumeSecurityStyleFromGCNVSecurityStyle(netapppb.SecurityStyle_SECURITY_STYLE_UNSPECIFIED)

	assert.Equal(t, "NTFS", ntfs)
	assert.Equal(t, "Unix", unix)
	assert.Equal(t, "Unspecified", unspecified)
}

func TestGCNVSecurityStyleFromVolumeSecurityStyle(t *testing.T) {
	ntfs := GCNVSecurityStyleFromVolumeSecurityStyle("NTFS")
	unix := GCNVSecurityStyleFromVolumeSecurityStyle("Unix")
	unspecified := GCNVSecurityStyleFromVolumeSecurityStyle("Unspecified")

	assert.Equal(t, netapppb.SecurityStyle_NTFS, ntfs)
	assert.Equal(t, netapppb.SecurityStyle_UNIX, unix)
	assert.Equal(t, netapppb.SecurityStyle_SECURITY_STYLE_UNSPECIFIED, unspecified)
}

func TestVolumeAccessTypeFromGCNVAccessType(t *testing.T) {
	readWrite := VolumeAccessTypeFromGCNVAccessType(netapppb.AccessType_READ_WRITE)
	readOnly := VolumeAccessTypeFromGCNVAccessType(netapppb.AccessType_READ_ONLY)
	readNone := VolumeAccessTypeFromGCNVAccessType(netapppb.AccessType_READ_NONE)
	unspecified := VolumeAccessTypeFromGCNVAccessType(netapppb.AccessType_ACCESS_TYPE_UNSPECIFIED)

	assert.Equal(t, "ReadWrite", readWrite)
	assert.Equal(t, "ReadOnly", readOnly)
	assert.Equal(t, "ReadNone", readNone)
	assert.Equal(t, "Unspecified", unspecified)
}

func TestGCNVAccessTypeFromVolumeAccessType(t *testing.T) {
	readWrite := GCNVAccessTypeFromVolumeAccessType("ReadWrite")
	readOnly := GCNVAccessTypeFromVolumeAccessType("ReadOnly")
	readNone := GCNVAccessTypeFromVolumeAccessType("ReadNone")
	unspecified := GCNVAccessTypeFromVolumeAccessType("Unspecified")

	assert.Equal(t, netapppb.AccessType_READ_WRITE, readWrite)
	assert.Equal(t, netapppb.AccessType_READ_ONLY, readOnly)
	assert.Equal(t, netapppb.AccessType_READ_NONE, readNone)
	assert.Equal(t, netapppb.AccessType_ACCESS_TYPE_UNSPECIFIED, unspecified)
}

func TestVolumeProtocolFromGCNVProtocol(t *testing.T) {
	nfs3 := VolumeProtocolFromGCNVProtocol(netapppb.Protocols_NFSV3)
	nfs4 := VolumeProtocolFromGCNVProtocol(netapppb.Protocols_NFSV4)
	smb := VolumeProtocolFromGCNVProtocol(netapppb.Protocols_SMB)
	unspecified := VolumeProtocolFromGCNVProtocol(netapppb.Protocols_PROTOCOLS_UNSPECIFIED)

	assert.Equal(t, "NFSv3", nfs3)
	assert.Equal(t, "NFSv4.1", nfs4)
	assert.Equal(t, "SMB", smb)
	assert.Equal(t, "Unknown", unspecified)
}

func TestGCNVProtocolFromVolumeProtocol(t *testing.T) {
	nfs3 := GCNVProtocolFromVolumeProtocol("NFSv3")
	nfs4 := GCNVProtocolFromVolumeProtocol("NFSv4.1")
	smb := GCNVProtocolFromVolumeProtocol("SMB")
	unspecified := GCNVProtocolFromVolumeProtocol("Unknown")

	assert.Equal(t, netapppb.Protocols_NFSV3, nfs3)
	assert.Equal(t, netapppb.Protocols_NFSV4, nfs4)
	assert.Equal(t, netapppb.Protocols_SMB, smb)
	assert.Equal(t, netapppb.Protocols_PROTOCOLS_UNSPECIFIED, unspecified)
}

func TestSnapshotStateFromGCNVState(t *testing.T) {
	readyState := SnapshotStateFromGCNVState(netapppb.Snapshot_READY)
	unspecifiedState := SnapshotStateFromGCNVState(netapppb.Snapshot_STATE_UNSPECIFIED)
	creatingState := SnapshotStateFromGCNVState(netapppb.Snapshot_CREATING)
	deletingState := SnapshotStateFromGCNVState(netapppb.Snapshot_DELETING)
	updatingState := SnapshotStateFromGCNVState(netapppb.Snapshot_UPDATING)
	disabledState := SnapshotStateFromGCNVState(netapppb.Snapshot_DISABLED)
	errorState := SnapshotStateFromGCNVState(netapppb.Snapshot_ERROR)

	assert.Equal(t, "Ready", readyState)
	assert.Equal(t, "Unspecified", unspecifiedState)
	assert.Equal(t, "Creating", creatingState)
	assert.Equal(t, "Deleting", deletingState)
	assert.Equal(t, "Updating", updatingState)
	assert.Equal(t, "Disabled", disabledState)
	assert.Equal(t, "Error", errorState)
}

func TestIsGCNVNotFoundError(t *testing.T) {
	result := IsGCNVNotFoundError(nil)
	assert.False(t, result)

	err := status.Error(codes.NotFound, "This is a test error")
	result = IsGCNVNotFoundError(err)
	assert.True(t, result)

	err = errors.New("This is a non status code  error")
	result = IsGCNVNotFoundError(err)
	assert.False(t, result)
}

func TestIsGCNVTooManyRequestsError(t *testing.T) {
	result := IsGCNVTooManyRequestsError(nil)
	assert.False(t, result)

	err := status.Error(codes.ResourceExhausted, "This is a test error")
	result = IsGCNVTooManyRequestsError(err)
	assert.True(t, result)

	err = errors.New("This is a non status code  error")
	result = IsGCNVTooManyRequestsError(err)
	assert.False(t, result)
}

func TestIsGCNVDeadlineExceededError(t *testing.T) {
	result := IsGCNVDeadlineExceededError(nil)
	assert.False(t, result)

	err := status.Error(codes.DeadlineExceeded, "This is a test error")
	result = IsGCNVDeadlineExceededError(err)
	assert.True(t, result)

	err = errors.New("This is a non status code  error")
	result = IsGCNVDeadlineExceededError(err)
	assert.False(t, result)
}

func TestIsGCNVCanceledError(t *testing.T) {
	result := IsGCNVCanceledError(nil)
	assert.False(t, result)

	err := status.Error(codes.Canceled, "This is a test error")
	result = IsGCNVCanceledError(err)
	assert.True(t, result)

	err = errors.New("This is a non status code  error")
	result = IsGCNVCanceledError(err)
	assert.False(t, result)
}

func TestCreateVolume_TieringPolicyAuto_PoolDoesNotSupportTiering(t *testing.T) {
	sdk := getFakeSDK()
	ctx := context.Background()

	poolFullName := "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1"
	pool := &CapacityPool{
		Name:            "CP1",
		FullName:        poolFullName,
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     false,
	}
	addCapacityPool(sdk, pool)

	coolingDays := int32(7)
	request := &VolumeCreateRequest{
		Name:                      "test-volume",
		CreationToken:             "test-volume",
		CapacityPool:              "CP1",
		SizeBytes:                 107374182400,
		ProtocolTypes:             []string{"NFSv3"},
		TieringPolicy:             drivers.TieringPolicyAuto,
		TieringMinimumCoolingDays: &coolingDays,
	}

	_, err := sdk.CreateVolume(ctx, request)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support auto-tiering", "should fail when pool does not support auto-tiering")
}

func TestIsGCNVTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"plain error", fmt.Errorf("some error"), false},
		{"context canceled", context.Canceled, true},
		{"wrapped context canceled", fmt.Errorf("wrap: %w", context.Canceled), true},
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped deadline exceeded", fmt.Errorf("wrap: %w", context.DeadlineExceeded), true},
		{"grpc deadline exceeded", status.Error(codes.DeadlineExceeded, "deadline"), true},
		{"grpc canceled", status.Error(codes.Canceled, "canceled"), true},
		{"grpc other code", status.Error(codes.Unavailable, "unavailable"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsGCNVTimeoutError(tc.err)
			if result != tc.expected {
				t.Fatalf("IsGCNVTimeoutError(%v) = %v, expected %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestDerefString(t *testing.T) {
	str := "test"
	strwithnil := ""

	deferString := DerefString(&str)
	deferStringNil := DerefString(&strwithnil)

	assert.Equal(t, "", deferStringNil)
	assert.Equal(t, "test", deferString)
}

func TestDerefBool(t *testing.T) {
	b := true

	deferBool := DerefBool(&b)
	deferBoolNil := DerefBool(nil)

	assert.Equal(t, false, deferBoolNil)
	assert.Equal(t, true, deferBool)
}

func TestDerefAccesstype(t *testing.T) {
	at := netapppb.AccessType_READ_WRITE

	deferAccessType := DerefAccessType(&at)
	deferAccessTypeNil := DerefAccessType(nil)

	assert.Equal(t, at, deferAccessType)
	assert.Equal(t, netapppb.AccessType(0), deferAccessTypeNil)
}

func TestIsTerminalStateError(t *testing.T) {
	errTerminalState := &TerminalStateError{
		Err: errors.New("This is a test error"),
	}

	errNil := IsTerminalStateError(nil)
	errTerminal := IsTerminalStateError(errTerminalState)

	assert.Equal(t, true, errTerminal)
	assert.Equal(t, false, errNil)
}

func TestTerminalState(t *testing.T) {
	err := errors.New("This is a test error")

	actual := TerminalState(err)
	expected := &TerminalStateError{
		Err: err,
	}

	assert.Equal(t, expected, actual, " Terminal state error is not equal")
}

func TestValidateWIPCredentialConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *drivers.GCPWIPCredential
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid_AllFieldsPresent",
			config: &drivers.GCPWIPCredential{
				Type:                           "external_account",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			expectError: false,
		},
		{
			name: "Valid_WithOptionalFields",
			config: &drivers.GCPWIPCredential{
				UniverseDomain:                 "googleapis.com",
				Type:                           "external_account",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
				QuotaProjectID: "quota-project",
			},
			expectError: false,
		},
		{
			name: "Error_MissingType",
			config: &drivers.GCPWIPCredential{
				Type:                           "",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			expectError: true,
			errorMsg:    "type",
		},
		{
			name: "Error_MissingAudience",
			config: &drivers.GCPWIPCredential{
				Type:                           "external_account",
				Audience:                       "",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			expectError: true,
			errorMsg:    "audience",
		},
		{
			name: "Error_MissingSubjectTokenType",
			config: &drivers.GCPWIPCredential{
				Type:                           "external_account",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			expectError: true,
			errorMsg:    "subject_token_type",
		},
		{
			name: "Error_MissingTokenURL",
			config: &drivers.GCPWIPCredential{
				Type:                           "external_account",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			expectError: true,
			errorMsg:    "token_url",
		},
		{
			name: "Error_NilCredentialSource",
			config: &drivers.GCPWIPCredential{
				Type:                           "external_account",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource:               nil,
			},
			expectError: true,
			errorMsg:    "credential_source",
		},
		{
			name: "Error_MissingCredentialSourceFile",
			config: &drivers.GCPWIPCredential{
				Type:                           "external_account",
				Audience:                       "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "https://sts.googleapis.com/v1/token",
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "",
				},
			},
			expectError: true,
			errorMsg:    "credential_source.file",
		},
		{
			name: "Error_MultipleFieldsMissing",
			config: &drivers.GCPWIPCredential{
				Type:                           "",
				Audience:                       "",
				SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:                       "",
				CredentialSource:               nil,
				ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@project.iam.gserviceaccount.com:generateAccessToken",
			},
			expectError: true,
			errorMsg:    "type, audience, token_url, credential_source",
		},
		{
			name: "Error_AllFieldsMissing",
			config: &drivers.GCPWIPCredential{
				Type:             "",
				Audience:         "",
				SubjectTokenType: "",
				TokenURL:         "",
				CredentialSource: nil,
			},
			expectError: true,
			errorMsg:    "type, audience, subject_token_type, token_url, service_account_impersonation_url, credential_source",
		},
		{
			name: "Error_OnlyCredentialSourceFileMissing",
			config: &drivers.GCPWIPCredential{
				Type:             "external_account",
				Audience:         "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:         "https://sts.googleapis.com/v1/token",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "",
				},
			},
			expectError: true,
		},
		{
			name: "Error_MissingServiceAccountImpersonationURL",
			config: &drivers.GCPWIPCredential{
				UniverseDomain:   "googleapis.com",
				Type:             "external_account",
				Audience:         "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
				TokenURL:         "https://sts.googleapis.com/v1/token",
				CredentialSource: &drivers.GCPWIPCredentialSource{
					File: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
				QuotaProjectID: "quota-project",
			},
			expectError: true,
			errorMsg:    "service_account_impersonation_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWIPCredentialConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err, "Expected an error but got none")
				assert.Contains(t, err.Error(), "missing required WIP credential fields")
				assert.Contains(t, err.Error(), tt.errorMsg, "Error message should contain expected missing field(s)")
			} else {
				assert.NoError(t, err, "Expected no error but got: %v", err)
			}
		})
	}
}
