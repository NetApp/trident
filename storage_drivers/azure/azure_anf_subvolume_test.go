package azure

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_azure"
	"github.com/netapp/trident/storage"
	storagefake "github.com/netapp/trident/storage/fake"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure/api"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils"
)

func newTestANFSubvolumeDriver(mockAPI api.Azure) *NASBlockStorageDriver {
	prefix := "trident-"

	config := drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: "azure-netapp-files-subvolume",
			StoragePrefix:     &prefix,
			DebugTraceFlags:   debugTraceFlags,
		},
		SubscriptionID:      SubscriptionID,
		TenantID:            TenantID,
		ClientID:            ClientID,
		ClientSecret:        ClientSecret,
		Location:            Location,
		NfsMountOptions:     "nfsvers=3",
		VolumeCreateTimeout: "30",
	}

	return &NASBlockStorageDriver{
		Config:              config,
		SDK:                 mockAPI,
		volumeCreateTimeout: 30 * time.Second,
	}
}

func newTestANFSubvolumeNewFileHelper(
	config drivers.AzureNFSStorageDriverConfig, context tridentconfig.DriverContext,
) *SubvolumeHelper {
	return &SubvolumeHelper{
		Config:         config,
		Context:        context,
		SnapshotRegexp: regexp.MustCompile(fmt.Sprintf("(?m)%v-(.+?)%v(.+)", "-", "--")),
	}
}

func newMockANFSubvolumeDriver(t *testing.T) (*mockapi.MockAzure, *NASBlockStorageDriver) {
	mockCtrl := gomock.NewController(t)

	mockAPI := mockapi.NewMockAzure(mockCtrl)

	return mockAPI, newTestANFSubvolumeDriver(mockAPI)
}

func newMockANFSubvolumeHelper() *SubvolumeHelper {
	prefix := "test-"

	config := drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: "azure-netapp-files-subvolume",
			StoragePrefix:     &prefix,
			DebugTraceFlags:   debugTraceFlags,
		},
		SubscriptionID:      SubscriptionID,
		TenantID:            TenantID,
		ClientID:            ClientID,
		ClientSecret:        ClientSecret,
		Location:            Location,
		NfsMountOptions:     "nfsvers=3",
		VolumeCreateTimeout: "30",
	}
	ctx := tridentconfig.ContextCSI
	return newTestANFSubvolumeNewFileHelper(config, ctx)
}

func TestSubvolumeGetSnapshotInternalName(t *testing.T) {
	helper := newMockANFSubvolumeHelper()
	volName := "pvc-abc1234-324abc34"
	snapName := "my-Snapshot"
	volNameLongWithoutPrefix := "myVolume"
	volNameShortWithoutPrefix := "vol"

	// Volume name has "pvc" as prefix
	result1 := helper.GetSnapshotInternalName(volName, snapName)
	assert.Equal(t, "test--my-Snapshot--abc12", result1, "invalid snapshot internal name")

	// Length of volume name is >5 and doesn't have "pvc" as prefix
	result2 := helper.GetSnapshotInternalName(volNameLongWithoutPrefix, snapName)
	assert.Equal(t, "test--my-Snapshot--myVo", result2, "invalid snapshot internal name")

	// Length of volume name is <=5 and doesn't have "pvc" as prefix
	result3 := helper.GetSnapshotInternalName(volNameShortWithoutPrefix, snapName)
	assert.Equal(t, "test--my-Snapshot--vol", result3, "invalid snapshot internal name")
}

func TestSubvolumeIsValidSnapshotInternalName(t *testing.T) {
	helper := newMockANFSubvolumeHelper()
	snapName1 := "storagePrefix--mySnap_vol1--453"
	snapName2 := "testSnap_vol1"

	// Snapshot name is valid
	result1 := helper.IsValidSnapshotInternalName(snapName1)
	assert.True(t, result1, "invalid snapshot internal name")

	// Snapshot name is invalid
	result2 := helper.IsValidSnapshotInternalName(snapName2)
	assert.False(t, result2, "valid snapshot internal name")
}

func TestSubvolumeGetSnapshotSuffixFromSnapshotInternalName(t *testing.T) {
	helper := newMockANFSubvolumeHelper()
	snapName1 := "storagePrefix--mySnap_vol1--453"
	snapName2 := "testSnap_vol"

	// Length of snapshot name is >2
	result1 := helper.GetSnapshotSuffixFromSnapshotInternalName(snapName1)
	assert.NotEmpty(t, result1, "invalid snapshot suffix")
	assert.Equal(t, "453", result1, "invalid")

	// Length of snapshot name is <=2
	result2 := helper.GetSnapshotSuffixFromSnapshotInternalName(snapName2)
	assert.Empty(t, result2, "valid snapshot suffix")
}

func TestSubvolumeDriverName(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.Name()

	assert.Equal(t, drivers.AzureNASBlockStorageDriverName, result, "driver name mismatches")
}

func TestSubvolumeBackendName_SetInConfig(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config.BackendName = "myANFSubvolumeBackend"

	result := driver.BackendName()

	assert.Equal(t, "myANFSubvolumeBackend", result, "backend name mismatches")
}

func TestSubvolumeBackendName_UseDefault(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config.BackendName = ""

	result := driver.BackendName()

	assert.Equal(t, "azurenetappfilessubvolume_1-cli", result, "backend name mismatches")
}

func TestSubvolumePoolName(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config.BackendName = "myANFSubvolumeBackend"

	result := driver.poolName("pool-1-A")

	assert.Equal(t, "myANFSubvolumeBackend_pool1A", result, "pool name mismatches")
}

func TestSubvolumeValidateVolumeName(t *testing.T) {
	tests := []struct {
		Name  string
		Valid bool
	}{
		// Invalid names
		{"", false},
		{"x2345678901234567890123456789012345678901", false},
		{"1volume", false},
		{"-volume", false},
		{"_volume", false},
		{"volume&", false},
		{"volume1-file-1234", false},
		// Valid names
		{"v", true},
		{"volume1", true},
		{"x234567890123456789012345678901234567890", true},
		{"volume-", true},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			_, driver := newMockANFSubvolumeDriver(t)

			result := driver.validateVolumeName(test.Name)

			if test.Valid {
				assert.NoError(t, result, "invalid volume name")
			} else {
				assert.Error(t, result, "valid volume name")
			}
		})
	}
}

func TestSubvolumeValidateSnapshotName(t *testing.T) {
	tests := []struct {
		Name  string
		Valid bool
	}{
		// Invalid names
		{"", false},
		{"x234567890123456789012345678901234567890123456", false},
		{"1volume", false},
		{"-volume", false},
		{"_volume", false},
		{"volume&", false},
		{"volume1--1234", false},
		// Valid names
		{"v", true},
		{"volume1", true},
		{"x23456789012345678901234567890123456789012345", true},
		{"volume-", true},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			_, driver := newMockANFSubvolumeDriver(t)

			result := driver.validateSnapshotName(test.Name)

			if test.Valid {
				assert.NoError(t, result, "invalid snapshot name")
			} else {
				assert.Error(t, result, "valid snapshot name")
			}
		})
	}
}

func TestSubvolumeValidateCreationToken(t *testing.T) {
	tests := []struct {
		Token string
		Valid bool
	}{
		// Invalid names
		{"", false},
		{"x2345678901234567890123456789012345678901234567890123456789012345", false},
		{"1volume", false},
		{"-volume", false},
		{"_volume", false},
		{"volume&", false},
		{"Volume_1-A", false},
		// Valid names
		{"v", true},
		{"volume-1", true},
		{"x234567890123456789012345678901234567890123456789012345678901234", true},
		{"volume-", true},
	}
	for _, test := range tests {
		t.Run(test.Token, func(t *testing.T) {
			_, driver := newMockANFSubvolumeDriver(t)

			result := driver.validateCreationToken(test.Token)

			if test.Valid {
				assert.NoError(t, result, "invalid creation token")
			} else {
				assert.Error(t, result, "valid creation token")
			}
		})
	}
}

func TestSubvolumeDefaultCreateTimeout(t *testing.T) {
	tests := []struct {
		Context  tridentconfig.DriverContext
		Expected time.Duration
	}{
		{tridentconfig.ContextDocker, tridentconfig.DockerCreateTimeout},
		{tridentconfig.ContextCSI, api.VolumeCreateTimeout},
		{"", api.VolumeCreateTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockANFSubvolumeDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultCreateTimeout()

			assert.Equal(t, test.Expected, result, "durations mismatched")
		})
	}
}

func TestSubvolumeDefaultTimeout(t *testing.T) {
	tests := []struct {
		Context  tridentconfig.DriverContext
		Expected time.Duration
	}{
		{tridentconfig.ContextDocker, tridentconfig.DockerDefaultTimeout},
		{tridentconfig.ContextCSI, api.DefaultTimeout},
		{"", api.DefaultTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockANFSubvolumeDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultTimeout()

			assert.Equal(t, test.Expected, result, "durations mismatched")
		})
	}
}

func getStructsForSubvolumeInitialize() (*drivers.CommonStorageDriverConfig, []*api.FileSystem) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	volumeID1 := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")
	volumeID2 := api.CreateVolumeID(SubscriptionID, "RG2", "NA2", "CP2", "testvol2")
	filesystem1 := &api.FileSystem{
		ID:                volumeID1,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv41},
		QuotaInBytes:      VolumeSizeI64,
		UnixPermissions:   defaultUnixPermissions,
	}
	filesystem2 := &api.FileSystem{
		ID:                volumeID2,
		ResourceGroup:     "RG2",
		NetAppAccount:     "NA2",
		CapacityPool:      "CP2",
		Name:              "testvol2",
		FullName:          "RG2/NA2/CP2/testvol2",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol2",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		UnixPermissions:   defaultUnixPermissions,
	}
	filesystems := []*api.FileSystem{
		filesystem1, filesystem2,
	}

	return commonConfig, filesystems
}

func TestSubvolumeInitialize(t *testing.T) {
	commonConfig, filesystems := getStructsForSubvolumeInitialize()

	configJSON := `
    {
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"volumeCreateTimeout": "600",
		"sdkTimeout": "60",
		"maxCacheAge": "300"
   }`

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)
	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.Equal(t, "deadbeef-784c-4b35-8329-460f52a3ad50", driver.Config.ClientID)
	assert.Equal(t, "myClientSecret", driver.Config.ClientSecret)
	assert.Equal(t, "trident", *driver.Config.StoragePrefix, "wrong storage prefix")
	assert.Equal(t, BackendUUID, driver.telemetry.TridentBackendUUID, "wrong backend UUID")
	assert.Equal(t, driver.volumeCreateTimeout, 600*time.Second, "volume create timeout mismatch")
	assert.True(t, driver.Initialized(), "not initialized")
}

// TestSubvolumeInitialize_SDKInitError : This method will check if we are making calls using actual SDK.
// Incase, it is not assigned with actual SDK, expectation is that it shouldn't initialize the driver and throw error.
func TestSubvolumeInitialize_SDKInitError(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
	{
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"volumeCreateTimeout": "600",
		"sdkTimeout": "60",
		"maxCacheAge": "300"
	}`

	_, driver := newMockANFSubvolumeDriver(t)
	driver.SDK = nil
	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_ErrorInitializingSDKClient_DriverConfig(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
	{
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"volumeCreateTimeout": "600",
		"sdkTimeout": "60",
		"maxCacheAge": "300"
	}`

	_, driver := newMockANFSubvolumeDriver(t)
	_, _, _ = driver.initializeStoragePools(ctx)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_MultipleFilePoolVolumesError(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
	{
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1","RG1/NA1/CP1/"],
		"volumeCreateTimeout": "600",
		"sdkTimeout": "60",
		"maxCacheAge": "300"
	}`

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(nil, errFailed).Times(1)
	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_InvalidStoragePrefix(t *testing.T) {
	commonConfig, filesystems := getStructsForSubvolumeInitialize()
	invalidPrefix := "&trident"

	commonConfig.StoragePrefix = &invalidPrefix

	configJSON := `
	{
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"volumeCreateTimeout": "600",
		"sdkTimeout": "60",
		"maxCacheAge": "300"
	}`

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)
	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_InvalidSDKTimeout(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
	{
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"volumeCreateTimeout": "600",
		"sdkTimeout": "60s",
		"maxCacheAge": "300"
    }`

	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_InvalidVolumeCreateTimeout(t *testing.T) {
	commonConfig, filesystems := getStructsForSubvolumeInitialize()

	configJSON := `
   {
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"serviceLevel": "Premium",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"virtualNetwork": "VN1",
		"subnet": "RG1/VN1/SN1",
		"volumeCreateTimeout": "10m"
   }`

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)
	mockAPI.EXPECT().Init(ctx, gomock.Any()).Return(nil).Times(1)
	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_WithInvalidSecrets(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
   {
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"serviceLevel": "Premium",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"virtualNetwork": "VN1",
		"subnet": "RG1/VN1/SN1",
		"volumeCreateTimeout": "10m"
   }`

	secrets := map[string]string{
		"client":       "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
	}

	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_InvalidConfigJSON(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
	{
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
	}`
	_, driver := newMockANFSubvolumeDriver(t)
	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeInitialize_InvalidMaxCacheAge(t *testing.T) {
	commonConfig, _ := getStructsForSubvolumeInitialize()

	configJSON := `
    {
		"version": 1,
		"storageDriverName": "azure-netapp-files-subvolume",
		"location": "fake-location",
		"subscriptionID": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
		"tenantID": "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"clientID": "deadbeef-784c-4b35-8329-460f52a3ad50",
		"clientSecret": "myClientSecret",
		"serviceLevel": "Premium",
		"debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"capacityPools": ["RG1/NA1/CP1", "RG1/NA1/CP2"],
		"filePoolVolumes": ["RG1/NA1/CP1/VOL-1"],
		"virtualNetwork": "VN1",
		"subnet": "RG1/VN1/SN1",
		"maxCacheAge": "300s"
    }`

	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{},
		BackendUUID)

	assert.Error(t, result, "initialized")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestSubvolumeTerminate(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	driver.initialized = true

	driver.Terminate(ctx, "")

	assert.False(t, driver.initialized, "initialized")
}

func getStructsForSubvolumeInitializeStoragePools() (
	*drivers.CommonStorageDriverConfig, drivers.AzureNFSStorageDriverPool, []*api.FileSystem,
) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "1234567890",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	volumeID1 := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")
	volumeID2 := api.CreateVolumeID(SubscriptionID, "RG2", "NA2", "CP2", "testvol2")
	filesystem1 := &api.FileSystem{
		ID:                volumeID1,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv41},
		QuotaInBytes:      VolumeSizeI64,
		UnixPermissions:   defaultUnixPermissions,
	}
	filesystem2 := &api.FileSystem{
		ID:                volumeID2,
		ResourceGroup:     "RG2",
		NetAppAccount:     "NA2",
		CapacityPool:      "CP2",
		Name:              "testvol2",
		FullName:          "RG2/NA2/CP2/testvol2",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol2",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		UnixPermissions:   defaultUnixPermissions,
	}
	filesystems := []*api.FileSystem{
		filesystem1,
		filesystem2,
	}

	return commonConfig, azureNFSSDPool, filesystems
}

func TestSubvolumeInitializeStoragePools_ValidateFilePoolVolumesError(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		AzureNFSStorageDriverPool: azureNFSSDPool,
	}

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(nil, errFailed).Times(1)
	driver.Config = *config
	phyPools, virtPools, err := driver.initializeStoragePools(ctx)

	assert.Error(t, err, "initialized")
	assert.Nil(t, phyPools, "physical pools are present")
	assert.Nil(t, virtPools, "virtual pools are present")
}

func TestSubvolumeInitializeStoragePools_WithMultipleProtocols(t *testing.T) {
	commonConfig, azureNFSSDPool, filesystems := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		AzureNFSStorageDriverPool: azureNFSSDPool,
	}

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)
	driver.Config = *config
	phyPools, virtPools, err := driver.initializeStoragePools(ctx)

	assert.Nil(t, err, "not initialized")
	assert.NotEmpty(t, phyPools, "physical pools are empty")
	assert.Empty(t, virtPools, "virtual pools are not empty")
}

func TestSubvolumeInitializeStoragePools_UnSupportedNFSVersion(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=5",
		AzureNFSStorageDriverPool: azureNFSSDPool,
	}

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	phyPools, virtPools, err := driver.initializeStoragePools(ctx)

	assert.NotNil(t, err, "not nil")
	assert.Empty(t, phyPools, "physical pools are not empty")
	assert.Empty(t, virtPools, "virtual pools are not empty")
}

func TestSubvolumeInitializeStoragePools_ValidateVirtualPoolInitialization(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		Location:                  "",
		NfsMountOptions:           "nfsvers=4.1",
		Storage: []drivers.AzureNFSStorageDriverPool{
			azureNFSSDPool,
		},
	}

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(nil, errFailed).Times(1)
	driver.Config = *config
	phyPools, virtPools, err := driver.initializeStoragePools(ctx)
	assert.Error(t, err, "initialized")
	assert.Nil(t, phyPools, "physical pools are present")
	assert.Nil(t, virtPools, "virtual pools are present")
}

func TestSubvolumeValidate_StoragePrefix(t *testing.T) {
	tests := []struct {
		Name          string
		StoragePrefix string
		Valid         bool
	}{
		// Invalid storage prefixes
		{
			Name:          "storage prefix may only contain letters and hyphens",
			StoragePrefix: "abcd123456789",
		},
		{
			Name:          "storage prefix length is greater than 10",
			StoragePrefix: "abcdefghijkl",
		},
		{
			Name:          "storage prefix does not contain --",
			StoragePrefix: "abcd--ef",
		},
		{
			Name:          "storage prefix does not end with -",
			StoragePrefix: "abcdef-",
		},
		// Valid storage prefixes
		{
			Name:          "storage prefix length is lesser than 10",
			StoragePrefix: "abcde",
			Valid:         true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			config := &drivers.AzureNFSStorageDriverConfig{
				CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
					StoragePrefix: &test.StoragePrefix,
				},
			}

			_, driver := newMockANFSubvolumeDriver(t)
			driver.Config = *config

			result := driver.validate(ctx)

			if test.Valid {
				assert.NoError(t, result, "storage prefix should be valid")
			} else {
				assert.Error(t, result, "storage prefix should be invalid")
			}
		})
	}
}

func TestSubvolumeValidate_MountOptionsError(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	prefix := "test"
	commonConfig.StoragePrefix = &prefix

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "ro",
		AzureNFSStorageDriverPool: azureNFSSDPool,
		Storage: []drivers.AzureNFSStorageDriverPool{
			azureNFSSDPool,
		},
	}

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	result := driver.validate(ctx)

	assert.Error(t, result, "validated configuration")
}

func TestSubvolumeValidate_InvalidVolumeSizeError(t *testing.T) {
	commonConfig, azureNFSSDPool, filesystems := getStructsForSubvolumeInitializeStoragePools()

	prefix := "test"
	commonConfig.StoragePrefix = &prefix

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		AzureNFSStorageDriverPool: azureNFSSDPool,
	}

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	tempPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "test-pool2")
	tempPool[pool.Name()] = pool
	driver.virtualPools = tempPool

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)
	_, _, _ = driver.initializeStoragePools(ctx)

	result := driver.validate(ctx)
	assert.Error(t, result, "validated configuration")
}

func getStructsForSubvolumeCreate() (
	*drivers.AzureNFSStorageDriverConfig, []*api.FileSystem, *storage.VolumeConfig,
	*api.Subvolume, *api.SubvolumeCreateRequest,
) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID1 := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")
	volumeID2 := api.CreateVolumeID(SubscriptionID, "RG2", "NA2", "CP2", "testvol2")

	filesystem1 := &api.FileSystem{
		ID:                volumeID1,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv41},
		QuotaInBytes:      VolumeSizeI64,
		UnixPermissions:   defaultUnixPermissions,
	}
	filesystem2 := &api.FileSystem{
		ID:                volumeID2,
		ResourceGroup:     "RG2",
		NetAppAccount:     "NA2",
		CapacityPool:      "CP2",
		Name:              "testvol2",
		FullName:          "RG2/NA2/CP2/testvol2",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol2",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		UnixPermissions:   defaultUnixPermissions,
	}
	filesystems := []*api.FileSystem{
		filesystem1, filesystem2,
	}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testsubvol1",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID1,
	}

	subVolumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1", "testsubvol1")
	subVolume := &api.Subvolume{
		ID:            subVolumeID,
		ResourceGroup: "RG1",
		NetAppAccount: "NA1",
		CapacityPool:  "CP1",
		Volume:        "testvol1",
		Name:          "testsubvol1",
		FullName:      "RG1/NA1/CP1/testvol1/testsubvol1",
		Type:          "READ-ONLY",
		Size:          SubvolumeSizeI64,
	}

	subvolumeCreateRequest := &api.SubvolumeCreateRequest{
		CreationToken: "trident-testsubvol1",
		Volume:        "RG1/NA1/CP1/testvol1",
		Size:          SubvolumeSizeI64,
	}

	return config, filesystems, volConfig, subVolume, subvolumeCreateRequest
}

func TestSubvolumeCreate(t *testing.T) {
	config, filesystems, volConfig, subVolume, subvolumeCreateRequest := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, subVolume,
		nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Equal(t, subVolume.ID, volConfig.InternalID, "internal ID not set on volConfig")
	assert.NoError(t, result, "create subvolume failed")
}

func TestSubvolumeCreate_InvalidVolumeName(t *testing.T) {
	config, filesystems, volConfig, _, _ := getStructsForSubvolumeCreate()

	volConfig.Name = "1234-abcd"

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreate_InvalidCreationToken(t *testing.T) {
	config, filesystems, volConfig, _, _ := getStructsForSubvolumeCreate()

	volConfig.InternalName = "1234-abcd"

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreate_ErrorSubvolumeExists1(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume,
		nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreate_ErrorSubvolumeExists2(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	subVolume.ProvisioningState = api.StateCreating

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume,
		nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreate_ErrorSubvolumeExists3(t *testing.T) {
	config, filesystems, volConfig, _, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil,
		errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreate_ErrorSubvolumeInvalidVolumeSize1(t *testing.T) {
	config, filesystems, volConfig, _, _ := getStructsForSubvolumeCreate()

	volConfig.Size = "invalid"
	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil, nil).Times(1)
	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreate_ErrorSubvolumeInvalidVolumeSize2(t *testing.T) {
	config, filesystems, volConfig, _, _ := getStructsForSubvolumeCreate()

	volConfig.Size = "-1M"
	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreateVolume_ZeroSize(t *testing.T) {
	config, filesystems, volConfig, subVolume, subvolumeCreateRequest := getStructsForSubvolumeCreate()

	volConfig.Size = "0"

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, subVolume,
		nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create subvolume failed")
}

func TestSubvolumeCreateVolume_ZeroSize_InvalidStoragePoolSize(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	volConfig.Size = "0"

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]
	storagePool.InternalAttributes()[Size] = "invalid"

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, subVolume,
		nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreateVolume_ZeroSize_ParseError(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	volConfig.Size = "0"

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]
	storagePool.InternalAttributes()[Size] = "-1M"

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, subVolume,
		nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreateVolume_BelowSubvolumeMinimumSize(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	volConfig.Size = strconv.FormatUint(MinimumSubvolumeSizeBytes-1, 10)

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, subVolume,
		nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)
	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreateVolume_AboveMaximumSize(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	config.LimitVolumeSize = strconv.FormatUint(MinimumSubvolumeSizeBytes, 10)
	volConfig.Size = strconv.FormatUint(MinimumSubvolumeSizeBytes+10, 10)

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, subVolume,
		nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)
	assert.Error(t, result, "created subvolume")
}

func TestSubvolumeCreateVolume_Error(t *testing.T) {
	config, filesystems, volConfig, _, subvolumeCreateRequest := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().ValidateFilePoolVolumes(ctx, gomock.Any()).Return(filesystems, nil).Times(1)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	_, virtualPool, _ := driver.initializeStoragePools(ctx)
	storagePool := virtualPool["myANFSubvolumeBackend_pool_0"]

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil,
		nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(nil, errFailed).Times(1)
	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "created subvolume")
}

func getStructsForSubvolumeCreateClone() (
	*drivers.AzureNFSStorageDriverConfig, *storage.VolumeConfig, *storage.VolumeConfig,
	*api.Subvolume, *api.Subvolume, *api.SubvolumeCreateRequest,
) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID1 := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-pvc-b99a6221-2635-49fc-bfab-b0cab18c24d1-file-0")
	volumeID2 := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-pvc-c883baf4-9742-49a3-85d6-6dd1d5514826-file-0")
	sourceVolConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "pvc-b99a6221-2635-49fc-bfab-b0cab18c24d1",
		InternalName: "trident-pvc-b99a6221-2635-49fc-bfab-b0cab18c24d1-file-0",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID1,
	}

	volConfig := &storage.VolumeConfig{
		Version:                   "1",
		Name:                      "pvc-c883baf4-9742-49a3-85d6-6dd1d5514826",
		InternalName:              "trident-pvc-c883baf4-9742-49a3-85d6-6dd1d5514826-file-0",
		Size:                      SubvolumeSizeStr,
		InternalID:                volumeID2,
		CloneSourceVolume:         "pvc-b99a6221-2635-49fc-bfab-b0cab18c24d1",
		CloneSourceVolumeInternal: "trident-pvc-b99a6221-2635-49fc-bfab-b0cab18c24d1-file-0",
		CloneSourceSnapshot:       "testSnap",
	}
	subVolumeID1 := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-testSnap--b99a6")
	subVolume1 := &api.Subvolume{
		ID:                subVolumeID1,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testVol1",
		Name:              "trident-testSnap--b99a6",
		FullName:          "RG1/NA1/CP1/testVol1/trident-testSnap--b99a6",
		ProvisioningState: api.StateAvailable,
	}

	subVolumeID2 := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-pvc-c883baf4-9742-49a3-85d6-6dd1d5514826-file-0")
	subVolume2 := &api.Subvolume{
		ID:            subVolumeID2,
		ResourceGroup: "RG1",
		NetAppAccount: "NA1",
		CapacityPool:  "CP1",
		Volume:        "testVol1",
		Name:          "trident-pvc-c883baf4-9742-49a3-85d6-6dd1d5514826-file-0",
		FullName:      "RG1/NA1/CP1/testVol1/trident-pvc-c883baf4-9742-49a3-85d6-6dd1d5514826-file-0",
	}

	subvolumeCreateRequest := &api.SubvolumeCreateRequest{
		CreationToken: "trident-pvc-c883baf4-9742-49a3-85d6-6dd1d5514826-file-0",
		Volume:        "RG1/NA1/CP1/testVol1",
		Parent:        "trident-testSnap--b99a6",
	}

	return config, sourceVolConfig, volConfig, subVolume1, subVolume2, subvolumeCreateRequest
}

func TestSubvolumeCreateClone(t *testing.T) {
	config, sourceVolConfig, volConfig, subVolume1, subVolume2, subvolumeCreateRequest := getStructsForSubvolumeCreateClone()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeByID(ctx, subVolume1.ID, false).Return(subVolume1, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(subVolume2, nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume2, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)
	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Nil(t, result, "created clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorInvalidVolumeName(t *testing.T) {
	config, sourceVolConfig, volConfig, _, _, _ := getStructsForSubvolumeCreateClone()

	volConfig.Name = ""

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorInvalidCreationToken(t *testing.T) {
	config, sourceVolConfig, volConfig, _, _, _ := getStructsForSubvolumeCreateClone()

	volConfig.InternalName = ""

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorInvalidSourceVolumeID(t *testing.T) {
	config, sourceVolConfig, volConfig, _, _, _ := getStructsForSubvolumeCreateClone()

	sourceVolConfig.InternalID = ""

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorSourceVolumeNotFound(t *testing.T) {
	config, sourceVolConfig, volConfig, subVolume1, _, _ := getStructsForSubvolumeCreateClone()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeByID(ctx, subVolume1.ID, false).Return(nil, errFailed).Times(1)
	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorSourceVolumeDoesNotExists(t *testing.T) {
	config, sourceVolConfig, volConfig, subVolume1, _, _ := getStructsForSubvolumeCreateClone()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeByID(ctx, subVolume1.ID, false).Return(subVolume1, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil,
		errFailed).Times(1)
	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorSourceVolumeAlreadyExists(t *testing.T) {
	config, sourceVolConfig, volConfig, subVolume1, _, _ := getStructsForSubvolumeCreateClone()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeByID(ctx, subVolume1.ID, false).Return(subVolume1, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume1,
		nil).Times(1)
	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorSourceVolumeAlreadyExistsButInCreatingState(t *testing.T) {
	config, sourceVolConfig, volConfig, subVolume1, _, _ := getStructsForSubvolumeCreateClone()
	subVolume1.ProvisioningState = api.StateCreating

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeByID(ctx, subVolume1.ID, false).Return(subVolume1, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume1,
		nil).Times(1)
	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func TestSubvolumeCreateClone_ErrorUnableToCreateSubvolume(t *testing.T) {
	config, sourceVolConfig, volConfig, subVolume1, _, subvolumeCreateRequest := getStructsForSubvolumeCreateClone()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeByID(ctx, subVolume1.ID, false).Return(subVolume1, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(nil, errFailed).Times(1)
	result := driver.CreateClone(ctx, sourceVolConfig, volConfig, nil)

	assert.Error(t, result, "failed to create clone of subvolume")
}

func getStructsForSubvolumeImport() (
	*drivers.AzureNFSStorageDriverConfig, *storage.VolumeConfig, *api.Subvolume,
) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testsubvol1",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID,
	}

	subVolumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1", "trident-testsubvol1")
	subVolume := &api.Subvolume{
		ID:            subVolumeID,
		ResourceGroup: "RG1",
		NetAppAccount: "NA1",
		CapacityPool:  "CP1",
		Volume:        "testvol1",
		Name:          "trident-testsubvol1",
		FullName:      "RG1/NA1/CP1/testvol1/trident-testsubvol1",
		Type:          "READ-ONLY",
		Size:          SubvolumeSizeI64,
	}

	return config, volConfig, subVolume
}

func TestSubvolumeImport(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeImport()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "trident-testsubvol1"

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, originalName, driver.getAllFilePoolVolumes(), true).Return(subVolume,
		nil).Times(1)
	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "unable to import subvolume")
}

func TestSubvolumeImport_SubvolumeIsSnapshot(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeImport()

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "test--mySnap--subvol1"

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	result := driver.Import(ctx, volConfig, originalName)
	assert.Error(t, result, "imported subvolume")
}

func TestSubvolumeImport_InvalidCreationToken(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeImport()

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "1234"

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "imported subvolume")
}

func TestSubvolumeImport_NonexistentSubvolume(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeImport()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "trident-testsubvol1"
	volConfig.InternalName = "test-invalid"

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, originalName, driver.getAllFilePoolVolumes(), true).Return(nil,
		errFailed).Times(1)
	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "imported subvolume")
}

func TestSubvolumeImport_BelowSubvolumeMinimumSize(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeImport()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "trident-testsubvol1"
	subVolume.Size = SubvolumeSizeI64 - 1

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, originalName, driver.getAllFilePoolVolumes(), true).Return(subVolume,
		nil).Times(1)
	result := driver.Import(ctx, volConfig, originalName)

	assert.Error(t, result, "imported subvolume")
}

func TestSubvolumeRename(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.Rename(ctx, " oldname", "newname")

	assert.Nil(t, result, "Unable to Rename")
}

func getStructsForWaitForSubvolumeCreate() (*drivers.AzureNFSStorageDriverConfig, *api.Subvolume) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	subVolumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1", "trident-testsubvol1")
	subVolume := &api.Subvolume{
		ID:            subVolumeID,
		ResourceGroup: "RG1",
		NetAppAccount: "NA1",
		CapacityPool:  "CP1",
		Volume:        "testvol1",
		Name:          "trident-testsubvol1",
		FullName:      "RG1/NA1/CP1/testvol1/trident-testsubvol1",
		Type:          "READ-ONLY",
		Size:          SubvolumeSizeI64,
	}

	return config, subVolume
}

func TestSubvolumeWaitForSubvolumeCreate_Creating(t *testing.T) {
	config, subVolume := getStructsForWaitForSubvolumeCreate()

	for _, state := range []string{api.StateAccepted, api.StateCreating} {

		mockAPI, driver := newMockANFSubvolumeDriver(t)
		driver.Config = *config

		driver.populateConfigurationDefaults(ctx, &driver.Config)
		subVolume.ProvisioningState = api.StateCreating

		mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
			driver.volumeCreateTimeout).Return(state, errFailed).Times(1)

		result := driver.waitForSubvolumeCreate(ctx, subVolume)
		assert.Error(t, result, "subvolume creation is complete")
	}
}

func TestSubvolumeWaitForSubvolumeCreate_DeletingNotCompleted(t *testing.T) {
	config, subVolume := getStructsForWaitForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	subVolume.ProvisioningState = api.StateCreating

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleting, errFailed).Times(1)

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, nil).Times(1)

	result := driver.waitForSubvolumeCreate(ctx, subVolume)
	assert.Nil(t, result, "subvolume creation is complete")
}

func TestSubvolumeWaitForSubvolumeCreate_DeletingCompleted(t *testing.T) {
	config, subVolume := getStructsForWaitForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	subVolume.ProvisioningState = api.StateCreating

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleting, errFailed).Times(1)

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, errFailed).Times(1)

	result := driver.waitForSubvolumeCreate(ctx, subVolume)
	assert.Nil(t, result, "subvolume creation is complete")
}

func TestSubvolumeWaitForSubvolumeCreate_ErrorDelete(t *testing.T) {
	config, subVolume := getStructsForWaitForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	subVolume.ProvisioningState = api.StateCreating

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateError, errFailed).Times(1)

	mockAPI.EXPECT().DeleteSubvolume(ctx, subVolume).Return(nil).Times(1)

	result := driver.waitForSubvolumeCreate(ctx, subVolume)
	assert.Nil(t, result, "subvolume creation is complete")
}

func TestSubvolumeWaitForSubvolumeCreate_ErrorDeleteFailed(t *testing.T) {
	config, subVolume := getStructsForWaitForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	subVolume.ProvisioningState = api.StateCreating

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateError, errFailed).Times(1)

	mockAPI.EXPECT().DeleteSubvolume(ctx, subVolume).Return(errFailed).Times(1)

	result := driver.waitForSubvolumeCreate(ctx, subVolume)
	assert.Nil(t, result, "subvolume creation is complete")
}

func TestSubvolumeWaitForSubvolumeCreate_OtherStates(t *testing.T) {
	config, subVolume := getStructsForWaitForSubvolumeCreate()

	for _, state := range []string{api.StateMoving, "unknown"} {
		mockAPI, driver := newMockANFSubvolumeDriver(t)
		driver.Config = *config

		driver.populateConfigurationDefaults(ctx, &driver.Config)
		subVolume.ProvisioningState = api.StateCreating

		mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
			driver.volumeCreateTimeout).Return(state, errFailed).Times(1)

		result := driver.waitForSubvolumeCreate(ctx, subVolume)
		assert.Nil(t, result, "subvolume creation is complete")
	}
}

func getStructsForSubvolumeDestroy() (*drivers.AzureNFSStorageDriverConfig, *storage.VolumeConfig, *api.Subvolume) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID,
	}

	subVolumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1", "trident-testsubvol1")
	subVolume := &api.Subvolume{
		ID:            subVolumeID,
		ResourceGroup: "RG1",
		NetAppAccount: "NA1",
		CapacityPool:  "CP1",
		Volume:        "testvol1",
		Name:          "trident-testsubvol1",
		FullName:      "RG1/NA1/CP1/testvol1/trident-testsubvol1",
		Type:          "READ-ONLY",
		Size:          SubvolumeSizeI64,
	}

	return config, volConfig, subVolume
}

func TestSubvolumeDestroy_InternalIDIsNull(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	volConfig.InternalID = ""

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume,
		nil).Times(1)

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, nil).Times(1)

	mockAPI.EXPECT().DeleteSubvolume(ctx, subVolume).Return(nil).Times(1)
	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, " subvolume not destroyed")
}

func TestSubvolumeDestroy_InternalIDIsNull_DeleteSubvolumeError(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	volConfig.InternalID = ""

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume,
		nil).Times(1)

	mockAPI.EXPECT().DeleteSubvolume(ctx, subVolume).Return(errFailed).Times(1)
	result := driver.Destroy(ctx, volConfig)

	assert.Error(t, result, "subvolume destroyed")
}

func TestSubvolumeDestroy_InternalIDIsNotNull(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	volConfig.InternalID = api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1", "trident-testsubvol1")

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	extantSubvolume := &api.Subvolume{
		ID:            volConfig.InternalID,
		ResourceGroup: subVolume.ResourceGroup,
		NetAppAccount: subVolume.NetAppAccount,
		CapacityPool:  subVolume.CapacityPool,
		Volume:        subVolume.Volume,
		Name:          volConfig.InternalName,
	}

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().DeleteSubvolume(ctx, extantSubvolume).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Error(t, result, "subvolume destroyed")
}

func TestSubvolumeDestroy_SubvolumeExistsCheckFailed(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	volConfig.InternalID = ""

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil,
		errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)
	assert.Error(t, result, "subvolume exists")
}

func TestSubvolumeDestroy_SubvolumeDeleted(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	volConfig.InternalID = ""

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(false, nil,
		nil).Times(1)

	result := driver.Destroy(ctx, volConfig)
	assert.Nil(t, result, "subvolume not destroyed")
}

func TestSubvolumeDestroy_SubvolumeStillDeleting(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	volConfig.InternalID = ""
	subVolume.ProvisioningState = api.StateDeleting

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeExists(ctx, volConfig, driver.getAllFilePoolVolumes()).Return(true, subVolume,
		nil).Times(1)

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateDeleted, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateDeleted, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)
	assert.Nil(t, result, "subvolume not destroyed")
}

func TestSubvolumeDestroy_ErrorParsingVolumeConfig(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeDestroy()

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	result := driver.Destroy(ctx, volConfig)

	assert.Error(t, result, "subvolume destroyed")
}

func getStructsForSubvolumePublish() (
	*drivers.AzureNFSStorageDriverConfig, *storage.VolumeConfig, *api.FileSystem, *utils.VolumePublishInfo,
) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID := api.CreateVolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testvol1")

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testsubvol1",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID,
	}

	mountTargets := []api.MountTarget{
		{
			MountTargetID: "mountTargetID",
			FileSystemID:  "filesystemID",
			IPAddress:     "1.1.1.1",
			SmbServerFqdn: "",
		},
	}

	filesystem := &api.FileSystem{
		ID:                volumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Name:              "testvol1",
		FullName:          "RG1/NA1/CP1/testvol1",
		Location:          Location,
		ProvisioningState: api.StateAvailable,
		CreationToken:     "trident-testvol1",
		ProtocolTypes:     []string{api.ProtocolTypeNFSv3},
		QuotaInBytes:      VolumeSizeI64,
		ServiceLevel:      api.ServiceLevelUltra,
		SnapshotDirectory: true,
		UnixPermissions:   defaultUnixPermissions,
		MountTargets:      mountTargets,
	}

	publishInfo := &utils.VolumePublishInfo{}

	return config, volConfig, filesystem, publishInfo
}

func TestSubvolumePublish(t *testing.T) {
	config, volConfig, filesystem, publishInfo := getStructsForSubvolumePublish()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(filesystem, nil).Times(1)
	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "subvolume not published")
}

func TestSubvolumePublish_ErrorFindingParentVolume(t *testing.T) {
	config, volConfig, _, publishInfo := getStructsForSubvolumePublish()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(nil, errFailed).Times(1)
	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, "subvolume published")
}

func TestSubvolumePublish_MountTargetsZero(t *testing.T) {
	config, volConfig, filesystem, publishInfo := getStructsForSubvolumePublish()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	filesystem.MountTargets = []api.MountTarget{}

	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(filesystem, nil).Times(1)
	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Error(t, result, " subvolume published")
}

func TestSubvolumePublish_MountOptionAndFileSystemIsNotEmpty(t *testing.T) {
	config, volConfig, filesystem, publishInfo := getStructsForSubvolumePublish()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	volConfig.MountOptions = "test-mount"
	volConfig.FileSystem = "xfs"

	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(filesystem, nil).Times(1)
	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "subvolume not published")
}

func TestSubvolumeCanSanpshot(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.CanSnapshot(ctx, nil, nil)

	assert.Nil(t, result, "snapshot cannot be taken")
}

func getStructsForSubvolumeCreateSnapshot() (
	*drivers.AzureNFSStorageDriverConfig, *storage.VolumeConfig,
	*api.Subvolume, *api.SubvolumeCreateRequest, *storage.SnapshotConfig,
) {
	prefix := "trident"
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		StoragePrefix:     &prefix,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		SubscriptionID:            SubscriptionID,
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f-file-0")
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f",
		InternalName: "trident-pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f-file-0",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID,
	}

	subVolumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1", "trident-testSnap--ce20c")
	subVolume := &api.Subvolume{
		ID:                subVolumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testVol1",
		Name:              "trident-testSnap--ce20c",
		FullName:          "RG1/NA1/CP1/testVol1/trident-testSnap--ce20c",
		ProvisioningState: api.StateAvailable,
	}

	subvolumeCreateRequest := &api.SubvolumeCreateRequest{
		CreationToken: "trident-testSnap--ce20c",
		Volume:        "RG1/NA1/CP1/testVol1",
		Parent:        "trident-pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f-file-0",
	}

	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "testSnap",
		InternalName:       "testSnap",
		VolumeName:         "pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f",
		VolumeInternalName: "trident-pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f-file-0",
	}

	return config, volConfig, subVolume, subvolumeCreateRequest, snapConfig
}

func TestSubvolumeCreateSnapshot(t *testing.T) {
	config, volConfig, subVolume, subvolumeCreateRequest, snapConfig := getStructsForSubvolumeCreateSnapshot()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeExistsByID(ctx, subVolume.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateAvailable, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "snaspshot not created")
	assert.NoError(t, resultErr, "error")
}

func TestSubvolumeCreateSnapshot_ErrorParsingSubvolumeID(t *testing.T) {
	config, volConfig, _, _, snapConfig := getStructsForSubvolumeCreateSnapshot()
	volConfig.InternalID = ""

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot created")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeCreateSnapshot_ErrorSubvolumeDoesNotExist(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeExistsByID(ctx, subVolume.ID).Return(false, nil, errFailed).Times(1)
	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot created")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeCreateSnapshot_ErrorCreatingSnapshot(t *testing.T) {
	config, volConfig, subVolume, subvolumeCreateRequest, snapConfig := getStructsForSubvolumeCreateSnapshot()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeExistsByID(ctx, subVolume.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot created")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeCreateSnapshot_InvalidSnapshotName(t *testing.T) {
	config, volConfig, _, _, snapConfig := getStructsForSubvolumeCreateSnapshot()
	snapConfig.Name = "1snap"

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot created")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeCreateSnapshot_InvalidCreationToken(t *testing.T) {
	config, volConfig, _, _, snapConfig := getStructsForSubvolumeCreateSnapshot()
	snapConfig.VolumeName = "_snap"

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot created")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeCreateSnapshot_ErrorSubvolumeState(t *testing.T) {
	config, volConfig, subVolume, subvolumeCreateRequest, snapConfig := getStructsForSubvolumeCreateSnapshot()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().SubvolumeExistsByID(ctx, subVolume.ID).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSubvolume(ctx, subvolumeCreateRequest).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.volumeCreateTimeout).Return(api.StateCreating, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot created")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeGetSnapshot(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	volConfig.InternalID = api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-testSnap--ce20c")

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(true, subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(true, subVolume, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "unable to get snapshot")
	assert.NoError(t, resultErr, "error")
}

func TestSubvolumeGetSnapshot_ErrorCheckingForExistingSnapshot(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	volConfig.InternalID = api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-testSnap--ce20c")

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(true, subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(false, subVolume, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "got snapshot")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeGetSnapshot_ErrorSnapshotDoesNotExist(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	volConfig.InternalID = api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-testSnap--ce20c")

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(true, subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(false, nil, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "got snapshot")
	assert.NoError(t, resultErr, "error")
}

func TestSubvolumeGetSnapshot_ErrorSnapshotStateIsNotAvailable(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	volConfig.InternalID = api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-testSnap--ce20c")
	subVolume.ProvisioningState = api.StateCreating

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(true, subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(true, subVolume, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "got snapshot")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeGetSnapshot_ErrorSubvolumeDoesNotExist(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(false, subVolume, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "got snapshot")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeGetSnapshot_ErrorSubvolumeNotFound(t *testing.T) {
	config, volConfig, _, _, snapConfig := getStructsForSubvolumeCreateSnapshot()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().SubvolumeExistsByID(ctx, volConfig.InternalID).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "got snapshot")
	assert.Error(t, resultErr, "no error")
}

func getStructsForSubvolumeGetSnapshots() (
	*drivers.AzureNFSStorageDriverConfig, *storage.VolumeConfig, *api.Subvolume, *[]*api.Subvolume,
) {
	prefix := "trident"
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		StoragePrefix:     &prefix,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		SubscriptionID:            SubscriptionID,
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	volumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1",
		"trident-pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f-file-0")
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f",
		InternalName: "trident-pvc-ce20c6cf-0a75-4b27-b9bd-3f53bf520f4f-file-0",
		Size:         SubvolumeSizeStr,
		InternalID:   volumeID,
	}

	subVolumeID := api.CreateSubvolumeID(SubscriptionID, "RG1", "NA1", "CP1", "testVol1", "trident-testSnap--ce20c")
	subVolume := &api.Subvolume{
		ID:                subVolumeID,
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		CapacityPool:      "CP1",
		Volume:            "testVol1",
		Name:              "trident-testSnap--ce20c",
		FullName:          "RG1/NA1/CP1/testVol1/trident-testSnap--ce20c",
		ProvisioningState: api.StateAvailable,
	}

	subVolumes := &[]*api.Subvolume{
		{
			ID:                subVolumeID,
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testVol1",
			Name:              "trident--testSnap--ce20c",
			FullName:          "RG1/NA1/CP1/testVol1/trident--testSnap--ce20c",
			ProvisioningState: api.StateAvailable,
		},
		{
			ID:                subVolumeID,
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testVol1",
			Name:              "trident-testSnap--b99a6",
			FullName:          "RG1/NA1/CP1/testVol1/trident-testSnap--b99a6",
			ProvisioningState: api.StateAvailable,
		},
		{
			ID:                subVolumeID,
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testVol1",
			Name:              "testSnap--b99a6",
			FullName:          "RG1/NA1/CP1/testVol1/testSnap--b99a6",
			ProvisioningState: api.StateAvailable,
		},
		{
			ID:                subVolumeID,
			ResourceGroup:     "RG1",
			NetAppAccount:     "NA1",
			CapacityPool:      "CP1",
			Volume:            "testVol1",
			Name:              "trident--testSnap--ce20d",
			FullName:          "RG1/NA1/CP1/testVol1/trident--testSnap--ce20d",
			ProvisioningState: api.StateAvailable,
		},
	}

	return config, volConfig, subVolume, subVolumes
}

func TestSubvolumeGetSnapshots(t *testing.T) {
	config, volConfig, subVolume, subVolumes := getStructsForSubvolumeGetSnapshots()

	vol := []string{
		api.CreateVolumeFullName(subVolume.ResourceGroup,
			subVolume.NetAppAccount, subVolume.CapacityPool, subVolume.Volume),
	}

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"
	driver.Config.StoragePrefix = &prefix

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().Subvolumes(ctx, vol).Return(subVolumes, nil).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.NotNil(t, result, "unable to get snapshots")
	assert.NoError(t, resultErr, "error")
}

func TestSubvolumeGetSnapshots_ErrorSubvolumeDoesNotExist(t *testing.T) {
	config, volConfig, _, _ := getStructsForSubvolumeGetSnapshots()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "got snapshots")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeGetSnapshots_SubvolumesExist(t *testing.T) {
	config, volConfig, subVolume, subVolumes := getStructsForSubvolumeGetSnapshots()

	prefix := "trident"
	config.StoragePrefix = &prefix

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	vol := []string{
		api.CreateVolumeFullName(subVolume.ResourceGroup,
			subVolume.NetAppAccount, subVolume.CapacityPool, subVolume.Volume),
	}

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().Subvolumes(ctx, vol).Return(subVolumes, nil).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.NotNil(t, result, "unable to get snapshots")
	assert.NoError(t, resultErr, "error")
}

func TestSubvolumeGetSnapshots_ErrorSubvolumesDoNotExist(t *testing.T) {
	config, volConfig, subVolume, _ := getStructsForSubvolumeGetSnapshots()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	vol := []string{
		api.CreateVolumeFullName(subVolume.ResourceGroup,
			subVolume.NetAppAccount, subVolume.CapacityPool, subVolume.Volume),
	}

	driver.helper = newMockANFSubvolumeHelper()
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().Subvolumes(ctx, vol).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.Nil(t, result, "got snapshots")
	assert.Error(t, resultErr, "no error")
}

func TestSubvolumeRestoreSnapshot(t *testing.T) {
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "testvol1",
		VolumeInternalName: "trident-testvol1",
	}

	_, driver := newMockANFSubvolumeDriver(t)

	result := driver.RestoreSnapshot(ctx, snapConfig, nil)

	assert.Error(t, result, "restored snapshot")
}

func TestSubvolumeDeleteSnapshot(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()
	subVolume.ProvisioningState = ""
	subVolume.FullName = ""

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().DeleteSubvolume(ctx, subVolume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateDeleted, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateDeleted, nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "deleted snapshot")
}

func TestSubvolumeDeleteSnapshot_ErrorParsingSubvolumeID(t *testing.T) {
	config, volConfig, _, _, snapConfig := getStructsForSubvolumeCreateSnapshot()
	volConfig.InternalID = ""

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)
	assert.Error(t, result, "deleted snapshot")
}

func TestSubvolumeDeleteSnapshot_ErrorDeletingSubvolume(t *testing.T) {
	config, volConfig, subVolume, _, snapConfig := getStructsForSubvolumeCreateSnapshot()
	subVolume.ProvisioningState = ""
	subVolume.FullName = ""

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	prefix := "trident"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.helper = newMockANFSubvolumeHelper()
	driver.helper.Config.StoragePrefix = &prefix

	mockAPI.EXPECT().DeleteSubvolume(ctx, subVolume).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)
	assert.Error(t, result, "deleted snapshot")
}

func TestSubvolumeGet(t *testing.T) {
	mockAPI, driver := newMockANFSubvolumeDriver(t)
	name := "subvol1"

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, name, driver.getAllFilePoolVolumes(), false).Return(nil,
		nil).Times(1)
	result := driver.Get(ctx, name)

	assert.Nil(t, result, "unable to get subvolume")
}

func TestSubvolumeGet_Error(t *testing.T) {
	mockAPI, driver := newMockANFSubvolumeDriver(t)
	name := "subvol1"

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, name, driver.getAllFilePoolVolumes(), false).Return(nil,
		errFailed).Times(1)
	result := driver.Get(ctx, name)

	assert.Error(t, result, "got subvolume")
}

func TestSubvolumeResize_SubvolumeNotFound(t *testing.T) {
	config, volConfig, _ := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := uint64(SubvolumeSizeI64 + 30)

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(nil, errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, newSize)

	assert.Error(t, result, "resized subvolume")
}

func TestSubvolumeResize_SubvolumeFound(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := SubvolumeSizeI64 * 2
	subVolume.ProvisioningState = api.StateAvailable

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)

	mockAPI.EXPECT().ResizeSubvolume(ctx, subVolume, newSize).Return(nil).Times(1)

	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout()).Return(api.StateAvailable, nil).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Nil(t, result, "unable to resize subvolume")
}

func TestSubvolumeResize_SubvolumeFound_StateError(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := SubvolumeSizeI64 * 2
	subVolume.ProvisioningState = api.StateCreating

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Error(t, result, "resized subvolume")
}

func TestSubvolumeResize_SubvolumeSize_NoChange(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := SubvolumeSizeI64
	subVolume.ProvisioningState = api.StateAvailable

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Nil(t, result, "unable to resize subvolume")
}

func TestSubvolumeResize_SubvolumeShrink(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := SubvolumeSizeI64 / 2
	subVolume.ProvisioningState = api.StateAvailable

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Error(t, result, "resized subvolume")
}

func TestSubvolumeResize_SubvolumeSize_AboveMaximum(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	config.LimitVolumeSize = strconv.FormatUint(MinimumSubvolumeSizeBytes, 10)
	newSize := SubvolumeSizeI64 + 10
	subVolume.ProvisioningState = api.StateAvailable

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Error(t, result, "resized subvolume")
}

func TestSubvolumeResize_Error(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := SubvolumeSizeI64 + 10
	subVolume.ProvisioningState = api.StateAvailable

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().ResizeSubvolume(ctx, subVolume, newSize).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Error(t, result, "resized subvolume")
}

func TestSubvolumeResize_SubvolumeStateError(t *testing.T) {
	config, volConfig, subVolume := getStructsForSubvolumeDestroy()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	newSize := SubvolumeSizeI64 + 10
	subVolume.ProvisioningState = api.StateAvailable

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().Subvolume(ctx, volConfig, true).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().ResizeSubvolume(ctx, subVolume, newSize).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForSubvolumeState(ctx, subVolume, api.StateAvailable, []string{api.StateError},
		driver.defaultTimeout()).Return("", errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, uint64(newSize))

	assert.Error(t, result, "resized subvolume")
}

func TestSubvolumeGetStorageBackendSpecs_VirtualPoolDoesNotExist(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		AzureNFSStorageDriverPool: azureNFSSDPool,
		Storage: []drivers.AzureNFSStorageDriverPool{
			azureNFSSDPool,
		},
	}

	_, driver := newMockANFSubvolumeDriver(t)

	backend := &storage.StorageBackend{}
	backend.SetName(driver.BackendName())

	driver.Config = *config
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	physicalPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "test-pool")
	physicalPool[pool.Name()] = pool
	driver.physicalPools = physicalPool

	backend.SetStorage(physicalPool)

	for _, pool := range driver.physicalPools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	for _, pool := range driver.virtualPools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	result := driver.GetStorageBackendSpecs(ctx, backend)
	assert.Nil(t, result, "unable to get storage backend spec")
}

func TestSubvolumeGetStorageBackendSpecs_VirtualPoolExists(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		AzureNFSStorageDriverPool: azureNFSSDPool,
		Storage: []drivers.AzureNFSStorageDriverPool{
			azureNFSSDPool,
		},
	}

	_, driver := newMockANFSubvolumeDriver(t)

	backend := &storage.StorageBackend{}
	backend.SetName(driver.BackendName())

	driver.Config = *config
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	virtualPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "test-pool")
	virtualPool[pool.Name()] = pool
	driver.virtualPools = virtualPool

	backend.SetStorage(virtualPool)

	for _, pool := range driver.physicalPools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	for _, pool := range driver.virtualPools {
		pool.SetBackend(backend)
		backend.AddStoragePool(pool)
	}

	result := driver.GetStorageBackendSpecs(ctx, backend)
	assert.Nil(t, result, "unable to get storage backend spec")
}

func TestSubvolumeCreatePrepare(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	driver.CreatePrepare(ctx, volConfig)
}

func TestSubvolumeGetStorageBackendPhysicalPoolNames(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)

	tempPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "test-pool")
	tempPool[pool.Name()] = pool

	driver.physicalPools = tempPool

	result := driver.GetStorageBackendPhysicalPoolNames(ctx)

	assert.NotNil(t, result, "unable to get physical pool names")
}

func TestSubvolumeGetInternalVolumeName(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	tridentconfig.UsingPassthroughStore = true
	result := driver.GetInternalVolumeName(ctx, "testvol1")

	assert.Equal(t, "trident-testvol1", result, "internal name mismatch")
}

func TestSubvolumeCreateFollowUp(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(filesystems[0], nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Error(t, result, "found mount targets")
}

func TestSubvolumeCreateFollowUp_SubvolumeNotFound(t *testing.T) {
	config, _, volConfig, _, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(nil, errFailed).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)
	assert.Error(t, result, "subvolume found")
}

func TestSubvolumeCreateFollowUp_ParentVolumeNotFound(t *testing.T) {
	config, _, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)

	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(nil, errFailed).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)
	assert.Error(t, result, "parent volume found")
}

func TestSubvolumeCreateFollowUp_StateError(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	subVolume.ProvisioningState = api.StateError

	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(filesystems[0], nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)
	assert.Error(t, result, "parent volume found")
}

func TestSubvolumeCreateFollowUp_MountTarget(t *testing.T) {
	config, filesystems, volConfig, subVolume, _ := getStructsForSubvolumeCreate()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	filesystems[0].MountTargets = []api.MountTarget{
		{
			MountTargetID: "mountTargetID",
			FileSystemID:  "filesystemID",
			IPAddress:     "1.1.1.1",
			SmbServerFqdn: "",
		},
	}

	mockAPI.EXPECT().Subvolume(ctx, volConfig, false).Return(subVolume, nil).Times(1)
	mockAPI.EXPECT().SubvolumeParentVolume(ctx, volConfig).Return(filesystems[0], nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)
	assert.NoError(t, result, " encountered error")
}

func TestSubvolumeGetProtocol(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	result := driver.GetProtocol(ctx)

	assert.Equal(t, tridentconfig.BlockOnFile, result, "protocol mismatch")
}

func TestSubvolumeStoreConfig(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	_, driver := newMockANFSubvolumeDriver(t)
	driver.Config.CommonStorageDriverConfig = commonConfig

	persistentConfig := &storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, persistentConfig)
}

func TestSubvolumeGetExternalConfig(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	result := driver.GetExternalConfig(ctx)
	assert.NotNil(t, result, "unable to get the config")
}

func TestSubvolumeGetVolumeExternal(t *testing.T) {
	config, _, subVolume := getStructsForSubvolumeImport()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "trident-testsubvol1"

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, originalName, driver.getAllFilePoolVolumes(), true).Return(subVolume,
		nil).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, originalName)

	assert.NotNil(t, result, "unable to get container volume")
	assert.NoError(t, resultErr, "error")
}

func TestSubvolumeGetVolumeExternal_Error(t *testing.T) {
	config, _, _ := getStructsForSubvolumeImport()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	originalName := "trident-testsubvol1"

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	mockAPI.EXPECT().SubvolumeByCreationToken(ctx, originalName, driver.getAllFilePoolVolumes(), true).Return(nil,
		errFailed).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, originalName)

	assert.Nil(t, result, "not nil")
	assert.Error(t, resultErr, "no error")
}

func getStructsForSubvolumes() (*drivers.AzureNFSStorageDriverConfig, *[]*api.Subvolume) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "azure-netapp-files-subvolume",
		BackendName:       "myANFSubvolumeBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	azureNFSSDPool := drivers.AzureNFSStorageDriverPool{
		Labels:         map[string]string{"key1": "val1"},
		Region:         "region1",
		Zone:           "zone1",
		ServiceLevel:   api.ServiceLevelUltra,
		VirtualNetwork: "VN1",
		Subnet:         "RG1/VN1/SN1",
		SupportedTopologies: []map[string]string{
			{
				"topology.kubernetes.io/region": "europe-west1",
				"topology.kubernetes.io/zone":   "us-east-1c",
			},
		},
		ResourceGroups:  []string{"RG1", "RG2"},
		NetappAccounts:  []string{"NA1", "NA2"},
		CapacityPools:   []string{"RG1/NA1/CP1", "RG1/NA1/CP2"},
		FilePoolVolumes: []string{"RG1/NA1/CP1/VOL-1"},
		AzureNFSStorageDriverConfigDefaults: drivers.AzureNFSStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "20971520",
			},
			UnixPermissions: "0700",
			SnapshotDir:     "true",
			ExportRule:      "1.1.1.1/32",
		},
	}

	azureNFSStorage := []drivers.AzureNFSStorageDriverPool{azureNFSSDPool}

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		Storage:                   azureNFSStorage,
	}

	subVolumes := &[]*api.Subvolume{
		{
			ProvisioningState: api.StateAvailable,
			Name:              "test--mySnap--subvol1",
		},
		{
			ProvisioningState: api.StateAvailable,
			Name:              "test-subvol2",
		},
		{
			ProvisioningState: api.StateDeleting,
			Name:              "test-subvol3",
		},
		{
			ProvisioningState: api.StateDeleted,
			Name:              "test-subvol4",
		},
		{
			ProvisioningState: api.StateError,
			Name:              "test-subvol5",
		},
		{
			ProvisioningState: api.StateAvailable,
			Name:              "subvol6",
		},
	}

	return config, subVolumes
}

func TestSubvolumeGetVolumeExternalWrappers(t *testing.T) {
	config, subVolumesList := getStructsForSubvolumes()

	storagePrefix := "test-"
	config.StoragePrefix = &storagePrefix

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config
	driver.helper = newMockANFSubvolumeHelper()

	channel := make(chan *storage.VolumeExternalWrapper, len(*subVolumesList))

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().Subvolumes(ctx, driver.getAllFilePoolVolumes()).Return(subVolumesList, nil).Times(1)
	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the subvolumes from the channel
	subVolumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			subVolumes = append(subVolumes, wrapper.Volume)
		}
	}

	assert.Len(t, subVolumes, 1, "wrong number of subvolumes")
}

func TestSubvolumeGetVolumeExternalWrappers_Error(t *testing.T) {
	config, subVolumesList := getStructsForSubvolumes()

	mockAPI, driver := newMockANFSubvolumeDriver(t)
	driver.Config = *config

	channel := make(chan *storage.VolumeExternalWrapper, len(*subVolumesList))

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	mockAPI.EXPECT().Subvolumes(ctx, driver.getAllFilePoolVolumes()).Return(nil, errFailed).Times(1)
	driver.GetVolumeExternalWrappers(ctx, channel)

	var result error
	for wrapper := range channel {
		if wrapper.Error != nil {
			result = wrapper.Error
			break
		}
	}

	assert.NotNil(t, result, "nil")
}

func TestSubvolumeString(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	stringFunc := func(d *NASBlockStorageDriver) string { return d.String() }

	for _, toString := range []func(*NASBlockStorageDriver) string{stringFunc} {
		assert.Contains(t, toString(driver), "<REDACTED>",
			"ANF Subvolume driver does not contain <REDACTED>")
		assert.Contains(t, toString(driver), "SDK:<REDACTED>",
			"ANF Subvolume driver does not redact SDK information")
		assert.Contains(t, toString(driver), "SubscriptionID:<REDACTED>",
			"ANF Subvolume driver does not redact API URL")
		assert.Contains(t, toString(driver), "TenantID:<REDACTED>",
			"ANF Subvolume driver does not redact Tenant ID")
		assert.Contains(t, toString(driver), "ClientID:<REDACTED>",
			"ANF Subvolume Subvolume driver does not redact Client ID")
		assert.Contains(t, toString(driver), "ClientSecret:<REDACTED>",
			"ANF Subvolume driver does not redact Client Secret")
		assert.NotContains(t, toString(driver), SubscriptionID,
			"ANF Subvolume driver contains Subscription ID")
		assert.NotContains(t, toString(driver), TenantID,
			"ANF Subvolume driver contains Tenant ID")
		assert.NotContains(t, toString(driver), ClientID,
			"ANF Subvolume driver contains Client ID")
		assert.NotContains(t, toString(driver), ClientSecret,
			"ANF Subvolume driver contains Client Secret")
	}
}

func TestSubvolumeGoString(t *testing.T) {
	_, driver := newMockANFSubvolumeDriver(t)
	goStringFunc := func(d *NASBlockStorageDriver) string { return d.GoString() }

	for _, toString := range []func(*NASBlockStorageDriver) string{goStringFunc} {
		assert.Contains(t, toString(driver), "<REDACTED>",
			"ANF Subvolume driver does not contain <REDACTED>")
		assert.Contains(t, toString(driver), "SDK:<REDACTED>",
			"ANF Subvolume driver does not redact SDK information")
		assert.Contains(t, toString(driver), "SubscriptionID:<REDACTED>",
			"ANF Subvolume driver does not redact API URL")
		assert.Contains(t, toString(driver), "TenantID:<REDACTED>",
			"ANF Subvolume driver does not redact Tenant ID")
		assert.Contains(t, toString(driver), "ClientID:<REDACTED>",
			"ANF Subvolume driver does not redact Client ID")
		assert.Contains(t, toString(driver), "ClientSecret:<REDACTED>",
			"ANF Subvolume driver does not redact Client Secret")
		assert.NotContains(t, toString(driver), SubscriptionID,
			"ANF Subvolume driver contains Subscription ID")
		assert.NotContains(t, toString(driver), TenantID,
			"ANF Subvolume driver contains Tenant ID")
		assert.NotContains(t, toString(driver), ClientID,
			"ANF Subvolume driver contains Client ID")
		assert.NotContains(t, toString(driver), ClientSecret,
			"ANF Subvolume driver contains Client Secret")
	}
}

func TestSubvolumeGetUpdateType(t *testing.T) {
	_, oldDriver := newMockANFSubvolumeDriver(t)
	oldDriver.volumeCreateTimeout = 1 * time.Second

	_, newDriver := newMockANFSubvolumeDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestSubvolumeGetUpdateType_InvalidUpdate(t *testing.T) {
	oldDriver := &fake.StorageDriver{
		Config:             drivers.FakeStorageDriverConfig{},
		Volumes:            make(map[string]storagefake.Volume),
		DestroyedVolumes:   make(map[string]bool),
		Snapshots:          make(map[string]map[string]*storage.Snapshot),
		DestroyedSnapshots: make(map[string]bool),
		Secret:             "secret",
	}

	_, newDriver := newMockANFSubvolumeDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestSubvolumeGetUpdateType_PrefixChange(t *testing.T) {
	_, oldDriver := newMockANFSubvolumeDriver(t)
	prefix1 := "prefix1-"
	oldDriver.Config.StoragePrefix = &prefix1

	_, newDriver := newMockANFSubvolumeDriver(t)
	prefix2 := "prefix2-"
	newDriver.Config.StoragePrefix = &prefix2

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.PrefixChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestSubvolumeGetUpdateType_CredentialsChange(t *testing.T) {
	_, oldDriver := newMockANFSubvolumeDriver(t)
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	_, newDriver := newMockANFSubvolumeDriver(t)
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestSubvolumeReconcileNodeAccess(t *testing.T) {
	node1 := &utils.Node{
		Name: "node-1",
	}
	node2 := &utils.Node{
		Name: "node-2",
	}

	nodes := []*utils.Node{
		node1, node2,
	}

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAzure(mockCtrl)

	driver := *newTestANFSubvolumeDriver(mockAPI)

	result := driver.ReconcileNodeAccess(ctx, nodes, "")

	assert.Nil(t, result, "not nil")
}

func TestSubvolumeGetCommonConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAzure(mockCtrl)

	driver := *newTestANFSubvolumeDriver(mockAPI)
	driver.Config.BackendName = "myANFSubvolumeBackend"
	driver.Config.ServiceLevel = api.ServiceLevelUltra

	result := driver.GetCommonConfig(ctx)
	assert.Equal(t, driver.Config.CommonStorageDriverConfig, result, "common config mismatch")
}

func TestSubvolumegetAllFilePoolVolumes_VirtualPools(t *testing.T) {
	commonConfig, azureNFSSDPool, _ := getStructsForSubvolumeInitializeStoragePools()

	config := &drivers.AzureNFSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		NfsMountOptions:           "nfsvers=4.1",
		AzureNFSStorageDriverPool: azureNFSSDPool,
		Storage: []drivers.AzureNFSStorageDriverPool{
			azureNFSSDPool,
		},
	}

	_, driver := newMockANFSubvolumeDriver(t)

	backend := &storage.StorageBackend{}
	backend.SetName(driver.BackendName())

	driver.Config = *config
	driver.populateConfigurationDefaults(ctx, &driver.Config)

	virtualPool := make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "test-pool")
	virtualPool[pool.Name()] = pool
	driver.virtualPools = virtualPool

	backend.SetStorage(virtualPool)

	result := driver.getAllFilePoolVolumes()
	assert.NotNil(t, result, "unable to get file pool volumes")
}
