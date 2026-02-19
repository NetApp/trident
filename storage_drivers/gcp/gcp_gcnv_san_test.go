// Copyright 2025 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_gcp"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	storagefake "github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	testUUID               = "12345678-1234-1234-1234-123456789012"
	testNodeName           = "test-node-1"
	testVolumeName         = "test-volume"
	testVolumeInternalName = "test_volume"       // InternalName with hyphens transformed to underscores
	testVolumeSize         = int64(107374182400) // 100 GiB (requested size)
	testVolumeSizeStr      = "107374182400"
	// testActualVolumeSize is 101 GiB - the size actually allocated by GCNV after rounding up + 1 GiB buffer
	// to ensure kernel-visible usable space meets or exceeds requested size.
	testActualVolumeSize = int64(108447924224) // 101 GiB = 100 GiB rounded up + 1 GiB buffer
	testHostGroupID      = "test-hostgroup-id"
	testHostGroupName    = "trident-node-test-node-1-12345678-1234-1234-1234-123456789012"
	testIQN              = "iqn.1994-05.com.redhat:test-node"
	testTargetIQN        = "iqn.2024-01.com.netapp:test-target"
	testTargetPortal     = "10.0.0.1:3260"
	testLunID            = 0
	testSerialNumber     = "6c573830315d5a4a592d3446"
)

func newTestSANDriver(mockAPI api.GCNV) *SANStorageDriver {
	prefix := "test-"

	config := drivers.GCNVStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: "google-cloud-netapp-volumes-san",
			StoragePrefix:     &prefix,
			DebugTraceFlags:   debugTraceFlags,
		},
		ProjectNumber: api.ProjectNumber,
		Location:      api.Location,
		APIKey: drivers.GCPPrivateKey{
			Type:                    api.Type,
			ProjectID:               api.ProjectID,
			PrivateKeyID:            api.PrivateKeyID,
			PrivateKey:              api.PrivateKey,
			ClientEmail:             api.ClientEmail,
			ClientID:                api.ClientID,
			AuthURI:                 api.AuthURI,
			TokenURI:                api.TokenURI,
			AuthProviderX509CertURL: api.AuthProviderX509CertURL,
			ClientX509CertURL:       api.ClientX509CertURL,
		},
	}

	driver := &SANStorageDriver{
		Config:         config,
		API:            mockAPI,
		tridentUUID:    testUUID,
		lunMutex:       locks.NewGCNamedMutex(),
		hostGroupMutex: locks.NewGCNamedMutex(),
		telemetry: &Telemetry{
			Telemetry: tridentconfig.Telemetry{
				TridentVersion:     tridentconfig.OrchestratorVersion.String(),
				TridentBackendUUID: testUUID,
				Platform:           "kubernetes",
				PlatformVersion:    "",
			},
			Plugin: tridentconfig.GCNVSANStorageDriverName,
		},
	}

	// Initialize pools map
	driver.pools = make(map[string]storage.Pool)
	pool := storage.NewStoragePool(nil, "test-pool")
	pool.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool.InternalAttributes()[FileSystemType] = drivers.DefaultFileSystemType
	pool.InternalAttributes()[FormatOptions] = ""
	driver.pools["test-pool"] = pool

	return driver
}

func newMockSANDriver(t *testing.T) (*mockapi.MockGCNV, *SANStorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockGCNV(mockCtrl)

	return mockAPI, newTestSANDriver(mockAPI)
}

func getTestVolume() *api.Volume {
	return &api.Volume{
		Name:          testVolumeName,
		FullName:      fmt.Sprintf("projects/test-project/locations/us-central1/volumes/%s", testVolumeName),
		CreationToken: testVolumeName,
		SizeBytes:     testVolumeSize,
		CapacityPool:  "test-pool",
		State:         api.StateReady,
		ServiceLevel:  api.ServiceLevelFlex,
		ProtocolTypes: []string{api.ProtocolTypeISCSI},
		Labels: map[string]string{
			// The SAN driver reads these via fixGCPLabelKey (lowercase + dots/slashes -> underscores).
			"trident_netapp_io_fstype":        "ext4",
			"trident_netapp_io_formatoptions": "-F",
		},
		SerialNumber: testSerialNumber,
	}
}

func getTestHostGroup() *api.HostGroup {
	return &api.HostGroup{
		Name:        fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/%s", testHostGroupName),
		Hosts:       []string{testIQN},
		Description: fmt.Sprintf("Trident host group for node %s", testNodeName),
	}
}

func getTestISCSITargetInfo() *api.ISCSITargetInfo {
	return &api.ISCSITargetInfo{
		TargetIQN:    testTargetIQN,
		TargetPortal: testTargetPortal,
		Portals:      []string{testTargetPortal, "10.0.0.2:3260"},
		LunID:        testLunID,
	}
}

// ============================================================================
// Driver lifecycle tests
// ============================================================================

func TestSANDriver_Name(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.Name()
	assert.Equal(t, tridentconfig.GCNVSANStorageDriverName, result, "driver name mismatch")
}

func TestSANDriver_BackendName_SetInConfig(t *testing.T) {
	_, driver := newMockSANDriver(t)

	driver.Config.BackendName = "myGCNVSANBackend"

	result := driver.BackendName()
	assert.Equal(t, "myGCNVSANBackend", result, "backend name mismatch")
}

func TestSANDriver_BackendName_UseDefault(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.BackendName()
	// defaultBackendName uses strings.Replace(d.Name(), "-", "", -1) which produces "googlecloudnetappvolumessan"
	assert.Contains(t, result, "googlecloudnetappvolumessan_", "backend name should contain driver name")
}

func TestSANDriver_defaultBackendName(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.defaultBackendName()
	// The default backend name should contain the driver name (without hyphens) and an ID
	assert.Contains(t, result, "googlecloudnetappvolumessan_", "default backend name should contain driver name")
}

func TestSANDriver_defaultBackendName_ShortPrivateKeyID(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Set a short private key ID (less than 5 chars)
	driver.Config.APIKey.PrivateKeyID = "abc"

	result := driver.defaultBackendName()
	// Should use random string instead since private key ID is too short
	assert.Contains(t, result, "googlecloudnetappvolumessan_", "default backend name should contain driver name")
}

func TestSANDriver_defaultTimeout(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// SAN driver only supports CSI context (Docker is rejected in Initialize)
	result := driver.defaultTimeout()
	assert.Equal(t, api.DefaultTimeout, result, "should use API default timeout")
}

func TestSANDriver_Initialize(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `{
		"projectNumber": "123456789",
		"location": "us-central1",
		"apiKey": {
			"type": "service_account",
			"project_id": "test-project"
		}
	}`

	// Mock API expectations
	mockAPI.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockAPI.EXPECT().CapacityPoolsForStoragePools(gomock.Any()).Return([]*api.CapacityPool{
		{Name: "test-pool", Location: "us-central1"},
	}).AnyTimes()

	err := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, testUUID)
	assert.NoError(t, err, "Initialize should succeed")
	assert.True(t, driver.initialized, "driver should be marked as initialized")
}

func TestSANDriver_Initialize_InvalidContext(t *testing.T) {
	_, driver := newMockSANDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
	}

	err := driver.Initialize(ctx, tridentconfig.ContextDocker, `{}`, commonConfig, nil, testUUID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid driver context")
}

func TestSANDriver_Initialize_ConfigError_InvalidJSON(t *testing.T) {
	_, driver := newMockSANDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
	}

	err := driver.Initialize(ctx, tridentconfig.ContextCSI, `{"projectNumber":`, commonConfig, nil, testUUID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not decode JSON configuration")
}

func TestSANDriver_Initialize_ConfigError_SecretInjection(t *testing.T) {
	_, driver := newMockSANDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `{
		"projectNumber": "123456789",
		"location": "us-central1",
		"apiKey": {
			"type": "service_account",
			"project_id": "test-project"
		}
	}`

	backendSecret := map[string]string{
		"private_key_id": "id-only",
	}

	err := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, backendSecret, testUUID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not inject backend secret")
}

func TestSANDriver_Initialize_APIClientInitError(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Force Initialize to call initializeGCNVAPIClient.
	driver.API = nil

	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `{
		"projectNumber": "123456789",
		"location": "us-central1",
		"apiKey": {
			"type": "service_account",
			"project_id": "test-project",
			"private_key_id": "pkid",
			"private_key": "pk"
		}
	}`

	orig := newGCNVDriver
	defer func() { newGCNVDriver = orig }()

	newGCNVDriver = func(ctx context.Context, cfg *api.ClientConfig) (api.GCNV, error) {
		return nil, fmt.Errorf("boom")
	}

	err := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, testUUID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error validating")
	assert.Contains(t, err.Error(), "GCNV API")
}

func TestSANDriver_Initialize_ValidationError(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Use an invalid storage prefix (starts with digit) to trigger validation error
	invalidPrefix := "123-invalid-prefix"
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
		StoragePrefix:     &invalidPrefix,
	}

	configJSON := `{
		"projectNumber": "123456789",
		"location": "us-central1",
		"apiKey": {
			"type": "service_account",
			"project_id": "test-project"
		}
	}`

	err := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, nil, testUUID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error validating")
}

func TestSANDriver_Terminate(t *testing.T) {
	_, driver := newMockSANDriver(t)
	driver.initialized = true

	driver.Terminate(ctx, testUUID)

	assert.False(t, driver.initialized, "driver should be marked as not initialized")
}

// ============================================================================
// Validate tests
// ============================================================================

func TestSANDriver_validate_Success(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Set valid pool attributes
	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex // SAN requires Flex
	pool.InternalAttributes()[Size] = "100Gi"
	pool.InternalAttributes()[FileSystemType] = "ext4"

	err := driver.validate(ctx)
	assert.NoError(t, err, "validate should succeed with valid config")
}

func TestSANDriver_validate_InvalidServiceLevel(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = "invalid-level"
	pool.InternalAttributes()[Size] = "100Gi"

	err := driver.validate(ctx)
	assert.Error(t, err, "validate should fail with invalid service level")
	assert.Contains(t, err.Error(), "invalid service level")
	assert.Contains(t, err.Error(), "SAN driver requires Flex")
}

func TestSANDriver_validate_NonFlexServiceLevel_Rejected(t *testing.T) {
	// SAN driver requires Flex service level; Standard, Premium, and Extreme are not allowed
	nonFlexLevels := []string{
		api.ServiceLevelStandard,
		api.ServiceLevelPremium,
		api.ServiceLevelExtreme,
	}

	for _, level := range nonFlexLevels {
		t.Run(level, func(t *testing.T) {
			_, driver := newMockSANDriver(t)

			pool := driver.pools["test-pool"]
			pool.InternalAttributes()[ServiceLevel] = level
			pool.InternalAttributes()[Size] = "100Gi"

			err := driver.validate(ctx)
			assert.Error(t, err, "validate should fail with non-Flex service level: %s", level)
			assert.Contains(t, err.Error(), "SAN driver requires Flex")
		})
	}
}

func TestSANDriver_validate_InvalidSize(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex
	pool.InternalAttributes()[Size] = "not-a-size"

	err := driver.validate(ctx)
	assert.Error(t, err, "validate should fail with invalid size")
	assert.Contains(t, err.Error(), "invalid value for default volume size")
}

func TestSANDriver_validate_InvalidFilesystemType(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex
	pool.InternalAttributes()[Size] = "100Gi"
	pool.InternalAttributes()[FileSystemType] = "ntfs" // Invalid

	err := driver.validate(ctx)
	assert.Error(t, err, "validate should fail with invalid filesystem type")
	assert.Contains(t, err.Error(), "invalid filesystem type")
}

func TestSANDriver_validate_EmptyServiceLevel_Allowed(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = "" // Empty is allowed
	pool.InternalAttributes()[Size] = "100Gi"

	err := driver.validate(ctx)
	assert.NoError(t, err, "validate should succeed with empty service level")
}

func TestSANDriver_validate_AllServiceLevels(t *testing.T) {
	// SAN driver only allows Flex service level (or empty which defaults to Flex)
	serviceLevels := []string{
		api.ServiceLevelFlex,
		"", // Empty defaults to Flex
	}

	for _, level := range serviceLevels {
		t.Run(level, func(t *testing.T) {
			_, driver := newMockSANDriver(t)

			pool := driver.pools["test-pool"]
			pool.InternalAttributes()[ServiceLevel] = level
			pool.InternalAttributes()[Size] = "100Gi"
			pool.InternalAttributes()[FileSystemType] = "xfs"

			err := driver.validate(ctx)
			assert.NoError(t, err, "validate should succeed for service level: %s", level)
		})
	}
}

func TestSANDriver_validate_AllFilesystemTypes(t *testing.T) {
	fsTypes := []string{"ext3", "ext4", "xfs", ""}

	for _, fstype := range fsTypes {
		t.Run(fstype, func(t *testing.T) {
			_, driver := newMockSANDriver(t)

			pool := driver.pools["test-pool"]
			pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex
			pool.InternalAttributes()[Size] = "100Gi"
			pool.InternalAttributes()[FileSystemType] = fstype

			err := driver.validate(ctx)
			assert.NoError(t, err, "validate should succeed for filesystem type: %s", fstype)
		})
	}
}

func TestSANDriver_validate_InvalidSnapshotReserve_NonInteger(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex
	pool.InternalAttributes()[Size] = "100Gi"
	pool.InternalAttributes()[FileSystemType] = "ext4"
	pool.InternalAttributes()[SnapshotReserve] = "notanumber"

	err := driver.validate(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value for snapshotReserve")
}

func TestSANDriver_validate_InvalidSnapshotReserve_OutOfRange(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex
	pool.InternalAttributes()[Size] = "100Gi"
	pool.InternalAttributes()[FileSystemType] = "ext4"
	pool.InternalAttributes()[SnapshotReserve] = "95" // > 90

	err := driver.validate(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid value for snapshotReserve")
}

func TestSANDriver_validate_ValidSnapshotReserve(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	pool.InternalAttributes()[ServiceLevel] = api.ServiceLevelFlex
	pool.InternalAttributes()[Size] = "100Gi"
	pool.InternalAttributes()[FileSystemType] = "ext4"
	pool.InternalAttributes()[SnapshotReserve] = "20"

	err := driver.validate(ctx)
	assert.NoError(t, err, "validate should succeed with valid snapshotReserve")
}

// ============================================================================
// getStorageBackendPools tests
// ============================================================================

func TestSANDriver_getStorageBackendPools_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	driver.Config.ProjectNumber = "123456789"

	capacityPools := []*api.CapacityPool{
		{Name: "pool1", Location: "us-central1"},
		{Name: "pool2", Location: "us-east1"},
	}

	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return(capacityPools)

	result := driver.getStorageBackendPools(ctx)

	assert.Len(t, result, 2, "should return 2 backend pools")
	assert.Equal(t, "123456789", result[0].ProjectNumber)
	assert.Equal(t, "us-central1", result[0].Location)
	assert.Equal(t, "pool1", result[0].StoragePool)
	assert.Equal(t, "123456789", result[1].ProjectNumber)
	assert.Equal(t, "us-east1", result[1].Location)
	assert.Equal(t, "pool2", result[1].StoragePool)
}

func TestSANDriver_getStorageBackendPools_Empty(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().CapacityPoolsForStoragePools(ctx).Return([]*api.CapacityPool{})

	result := driver.getStorageBackendPools(ctx)

	assert.Len(t, result, 0, "should return empty list when no capacity pools")
}

func TestSANDriver_GetConfig_Initialized_Rename_ReconcileVolumeNodeAccess(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// GetConfig
	cfg := driver.GetConfig()
	assert.NotNil(t, cfg)

	// Initialized
	driver.initialized = true
	assert.True(t, driver.Initialized())
	driver.initialized = false
	assert.False(t, driver.Initialized())

	// Rename is a no-op for GCNV; required by import workflow.
	assert.NoError(t, driver.Rename(ctx, "old", "new"))

	// ReconcileVolumeNodeAccess is a no-op for per-node host groups (handled at backend level).
	assert.NoError(t, driver.ReconcileVolumeNodeAccess(ctx, &storage.VolumeConfig{}, nil))

	// CreateFollowup is a no-op for this driver.
	assert.NoError(t, driver.CreateFollowup(ctx, &storage.VolumeConfig{}))
}

func TestSANDriver_CreatePrepare_SetsInternalName(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := driver.pools["test-pool"]
	volConfig := &storage.VolumeConfig{Name: "My-Vol"}
	volConfig.InternalName = ""

	driver.CreatePrepare(ctx, volConfig, pool)
	assert.NotEmpty(t, volConfig.InternalName)
}

func TestSANDriver_GetStorageBackendSpecsAndPhysicalPoolNames(t *testing.T) {
	_, driver := newMockSANDriver(t)

	ctrl := gomock.NewController(t)
	backend := mockstorage.NewMockBackend(ctrl)

	backend.EXPECT().SetName(driver.BackendName()).Times(1)
	backend.EXPECT().AddStoragePool(gomock.Any()).Times(len(driver.pools))

	err := driver.GetStorageBackendSpecs(ctx, backend)
	assert.NoError(t, err)

	names := driver.GetStorageBackendPhysicalPoolNames(ctx)
	assert.Len(t, names, len(driver.pools))
}

func TestSANDriver_StoreConfig_GetExternalConfig_GetCommonConfig_GetUpdateType(t *testing.T) {
	_, driver := newMockSANDriver(t)

	b := &storage.PersistentStorageBackendConfig{}
	driver.StoreConfig(ctx, b)
	assert.NotNil(t, b.GCNVConfig)

	ext := driver.GetExternalConfig(ctx)
	assert.NotNil(t, ext)

	common := driver.GetCommonConfig(ctx)
	assert.NotNil(t, common)

	bm := driver.GetUpdateType(ctx, driver)
	assert.NotNil(t, bm)
	assert.True(t, bm.IsEmpty())
}

func TestSANDriver_GetUpdateType_NoFlaggedChanges(t *testing.T) {
	_, oldDriver := newMockSANDriver(t)
	oldDriver.volumeCreateTimeout = 1 * time.Second

	_, newDriver := newMockSANDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	assert.Equal(t, &roaring.Bitmap{}, result, "bitmap mismatch")
}

func TestSANDriver_GetUpdateType_WrongDriverType(t *testing.T) {
	oldDriver := &fake.StorageDriver{
		Config:             drivers.FakeStorageDriverConfig{},
		Volumes:            make(map[string]storagefake.Volume),
		DestroyedVolumes:   make(map[string]bool),
		Snapshots:          make(map[string]map[string]*storage.Snapshot),
		DestroyedSnapshots: make(map[string]bool),
		Secret:             "secret",
	}

	_, newDriver := newMockSANDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestSANDriver_GetUpdateType_OtherChanges(t *testing.T) {
	_, oldDriver := newMockSANDriver(t)
	prefix1 := "prefix1-"
	oldDriver.Config.StoragePrefix = &prefix1
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	_, newDriver := newMockSANDriver(t)
	prefix2 := "prefix2-"
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

func TestSANDriver_GetVolumeForImport_GetVolumeExternalWrappers(t *testing.T) {
	t.Run("GetVolumeForImport_success", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		volume := getTestVolume()
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeByNameOrID(ctx, testVolumeName).Return(volume, nil).Times(1)

		ext, err := driver.GetVolumeForImport(ctx, testVolumeName)
		assert.NoError(t, err)
		assert.NotNil(t, ext)
		// Name should have the storage prefix "test-" stripped from "test-volume" -> "volume"
		assert.Equal(t, "volume", ext.Config.Name)
		assert.Equal(t, testVolumeSizeStr, ext.Config.Size)
		assert.Equal(t, volume.FullName, ext.Config.InternalID)
		assert.Equal(t, tridentconfig.Block, ext.Config.Protocol)
		assert.Equal(t, tridentconfig.ReadWriteOnce, ext.Config.AccessMode)
		assert.Equal(t, api.ServiceLevelFlex, ext.Config.ServiceLevel)
	})

	t.Run("GetVolumeForImport_refresh_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

		_, err := driver.GetVolumeForImport(ctx, testVolumeName)
		assert.Error(t, err)
	})

	t.Run("GetVolumeForImport_volume_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeByNameOrID(ctx, testVolumeName).Return(nil, errFailed).Times(1)

		_, err := driver.GetVolumeForImport(ctx, testVolumeName)
		assert.Error(t, err)
	})

	t.Run("GetVolumeExternalWrappers_success", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		// Set up a volume with matching prefix
		volume := getTestVolume()
		volume.CreationToken = "test-myvolume" // With prefix "test-"
		volumes := &[]*api.Volume{volume}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		ch := make(chan *storage.VolumeExternalWrapper, 10)
		driver.GetVolumeExternalWrappers(ctx, ch)

		// Should receive one volume
		wrapper, ok := <-ch
		assert.True(t, ok, "should receive a volume")
		assert.NoError(t, wrapper.Error)
		assert.NotNil(t, wrapper.Volume)
		assert.Equal(t, "myvolume", wrapper.Volume.Config.Name) // Prefix stripped

		// Channel should be closed
		_, ok = <-ch
		assert.False(t, ok, "channel should be closed")
	})

	t.Run("GetVolumeExternalWrappers_filters_non_iscsi", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		// Set up a NFS volume (should be filtered out)
		nfsVolume := getTestVolume()
		nfsVolume.ProtocolTypes = []string{api.ProtocolTypeNFSv3}
		nfsVolume.CreationToken = "test-nfs-volume"
		volumes := &[]*api.Volume{nfsVolume}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		ch := make(chan *storage.VolumeExternalWrapper, 10)
		driver.GetVolumeExternalWrappers(ctx, ch)

		// Channel should be closed with no volumes
		_, ok := <-ch
		assert.False(t, ok, "channel should be closed (no iSCSI volumes)")
	})

	t.Run("GetVolumeExternalWrappers_filters_by_state", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		// Set up volumes in various states
		readyVol := getTestVolume()
		readyVol.CreationToken = "test-ready"

		deletingVol := getTestVolume()
		deletingVol.State = api.VolumeStateDeleting
		deletingVol.CreationToken = "test-deleting"

		errorVol := getTestVolume()
		errorVol.State = api.VolumeStateError
		errorVol.CreationToken = "test-error"

		volumes := &[]*api.Volume{readyVol, deletingVol, errorVol}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		ch := make(chan *storage.VolumeExternalWrapper, 10)
		driver.GetVolumeExternalWrappers(ctx, ch)

		// Should only receive the ready volume
		wrapper, ok := <-ch
		assert.True(t, ok)
		assert.Equal(t, "ready", wrapper.Volume.Config.Name)

		// Channel should be closed
		_, ok = <-ch
		assert.False(t, ok, "channel should be closed")
	})

	t.Run("GetVolumeExternalWrappers_filters_by_prefix", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		// Set up volumes with and without matching prefix
		matchingVol := getTestVolume()
		matchingVol.CreationToken = "test-matching"

		nonMatchingVol := getTestVolume()
		nonMatchingVol.CreationToken = "other-nonmatching"

		volumes := &[]*api.Volume{matchingVol, nonMatchingVol}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		ch := make(chan *storage.VolumeExternalWrapper, 10)
		driver.GetVolumeExternalWrappers(ctx, ch)

		// Should only receive the matching volume
		wrapper, ok := <-ch
		assert.True(t, ok)
		assert.Equal(t, "matching", wrapper.Volume.Config.Name)

		// Channel should be closed
		_, ok = <-ch
		assert.False(t, ok, "channel should be closed")
	})

	t.Run("GetVolumeExternalWrappers_refresh_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

		ch := make(chan *storage.VolumeExternalWrapper, 10)
		driver.GetVolumeExternalWrappers(ctx, ch)

		// Should receive an error
		wrapper, ok := <-ch
		assert.True(t, ok)
		assert.Error(t, wrapper.Error)
		assert.Nil(t, wrapper.Volume)

		// Channel should be closed
		_, ok = <-ch
		assert.False(t, ok, "channel should be closed")
	})

	t.Run("GetVolumeExternalWrappers_volumes_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

		ch := make(chan *storage.VolumeExternalWrapper, 10)
		driver.GetVolumeExternalWrappers(ctx, ch)

		// Should receive an error
		wrapper, ok := <-ch
		assert.True(t, ok)
		assert.Error(t, wrapper.Error)
		assert.Nil(t, wrapper.Volume)

		// Channel should be closed
		_, ok = <-ch
		assert.False(t, ok, "channel should be closed")
	})

	_ = roaring.Bitmap{} // keep import used even if build tags change
}

func TestSANDriver_List(t *testing.T) {
	t.Run("List_success", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		// Set up volumes with matching prefix
		vol1 := getTestVolume()
		vol1.CreationToken = "test-volume1"

		vol2 := getTestVolume()
		vol2.CreationToken = "test-volume2"

		volumes := &[]*api.Volume{vol1, vol2}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		names, err := driver.List(ctx)
		assert.NoError(t, err)
		assert.Len(t, names, 2)
		assert.Contains(t, names, "volume1")
		assert.Contains(t, names, "volume2")
	})

	t.Run("List_filters_non_iscsi", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		// Set up iSCSI and NFS volumes
		iscsiVol := getTestVolume()
		iscsiVol.CreationToken = "test-iscsi"

		nfsVol := getTestVolume()
		nfsVol.CreationToken = "test-nfs"
		nfsVol.ProtocolTypes = []string{api.ProtocolTypeNFSv3}

		volumes := &[]*api.Volume{iscsiVol, nfsVol}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		names, err := driver.List(ctx)
		assert.NoError(t, err)
		assert.Len(t, names, 1)
		assert.Contains(t, names, "iscsi")
	})

	t.Run("List_filters_by_state", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		readyVol := getTestVolume()
		readyVol.CreationToken = "test-ready"

		deletingVol := getTestVolume()
		deletingVol.CreationToken = "test-deleting"
		deletingVol.State = api.VolumeStateDeleting

		volumes := &[]*api.Volume{readyVol, deletingVol}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		names, err := driver.List(ctx)
		assert.NoError(t, err)
		assert.Len(t, names, 1)
		assert.Contains(t, names, "ready")
	})

	t.Run("List_filters_by_prefix", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		matchingVol := getTestVolume()
		matchingVol.CreationToken = "test-matching"

		nonMatchingVol := getTestVolume()
		nonMatchingVol.CreationToken = "other-nonmatching"

		volumes := &[]*api.Volume{matchingVol, nonMatchingVol}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		names, err := driver.List(ctx)
		assert.NoError(t, err)
		assert.Len(t, names, 1)
		assert.Contains(t, names, "matching")
	})

	t.Run("List_empty", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		volumes := &[]*api.Volume{}

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

		names, err := driver.List(ctx)
		assert.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("List_refresh_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

		names, err := driver.List(ctx)
		assert.Error(t, err)
		assert.Nil(t, names)
	})

	t.Run("List_volumes_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)

		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

		names, err := driver.List(ctx)
		assert.Error(t, err)
		assert.Nil(t, names)
	})
}

func TestSANDriver_getVolumeExternal(t *testing.T) {
	_, driver := newMockSANDriver(t)

	t.Run("strips_prefix", func(t *testing.T) {
		volume := getTestVolume()
		volume.CreationToken = "test-myvolume" // With prefix "test-"

		ext := driver.getVolumeExternal(volume)

		assert.Equal(t, "myvolume", ext.Config.Name)
		assert.Equal(t, "test-myvolume", ext.Config.InternalName)
	})

	t.Run("no_prefix_to_strip", func(t *testing.T) {
		volume := getTestVolume()
		volume.CreationToken = "other-volume" // No matching prefix

		ext := driver.getVolumeExternal(volume)

		assert.Equal(t, "other-volume", ext.Config.Name)
		assert.Equal(t, "other-volume", ext.Config.InternalName)
	})

	t.Run("includes_all_fields", func(t *testing.T) {
		volume := getTestVolume()
		volume.CreationToken = "test-vol"
		volume.ServiceLevel = api.ServiceLevelPremium

		ext := driver.getVolumeExternal(volume)

		assert.Equal(t, tridentconfig.OrchestratorAPIVersion, ext.Config.Version)
		assert.Equal(t, tridentconfig.Block, ext.Config.Protocol)
		assert.Equal(t, tridentconfig.ReadWriteOnce, ext.Config.AccessMode)
		assert.Equal(t, volume.FullName, ext.Config.InternalID)
		assert.Equal(t, strconv.FormatInt(volume.SizeBytes, 10), ext.Config.Size)
		assert.Equal(t, api.ServiceLevelPremium, ext.Config.ServiceLevel)
		assert.Equal(t, volume.CapacityPool, ext.Pool)
	})
}

func TestSANDriver_GetSnapshotAndGetSnapshots(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	vol := getTestVolume()
	snapName := "snap1"
	snapCfg := &storage.SnapshotConfig{InternalName: snapName, VolumeInternalName: testVolumeName}
	volCfg := &storage.VolumeConfig{InternalName: testVolumeName}

	t.Run("snapshot_not_found_returns_nil_nil", func(t *testing.T) {
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeExists(ctx, volCfg).Return(true, vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, snapName).Return(nil, errors.NotFoundError("not found")).Times(1)

		s, err := driver.GetSnapshot(ctx, snapCfg, volCfg)
		assert.NoError(t, err)
		assert.Nil(t, s)
	})

	t.Run("snapshot_ready_returns_snapshot", func(t *testing.T) {
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeExists(ctx, volCfg).Return(true, vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, snapName).Return(&api.Snapshot{
			Name:    snapName,
			State:   api.SnapshotStateReady,
			Created: time.Unix(0, 0),
		}, nil).Times(1)

		s, err := driver.GetSnapshot(ctx, snapCfg, volCfg)
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.Equal(t, storage.SnapshotStateOnline, s.State)
	})

	t.Run("get_snapshots_lists_all", func(t *testing.T) {
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotsForVolume(ctx, vol).Return(&[]*api.Snapshot{
			{Name: "s1", State: api.SnapshotStateReady, Created: time.Unix(0, 0)},
			{Name: "s2", State: api.SnapshotStateReady, Created: time.Unix(0, 0)},
		}, nil).Times(1)

		snaps, err := driver.GetSnapshots(ctx, volCfg)
		assert.NoError(t, err)
		assert.Len(t, snaps, 2)
	})
}

func TestSANDriver_RestoreSnapshot_NoErrorPath(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	vol := getTestVolume()
	snapName := "snap1"
	snapCfg := &storage.SnapshotConfig{InternalName: snapName, VolumeInternalName: testVolumeName}
	volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
	snap := &api.Snapshot{Name: snapName, State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volCfg).Return(vol, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, vol, snapName).Return(snap, nil).Times(1)
	mockAPI.EXPECT().RestoreSnapshot(ctx, vol, snap).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, vol, api.VolumeStateReady, gomock.Any(), api.DefaultSDKTimeout).Return(api.VolumeStateReady, nil).Times(1)

	assert.NoError(t, driver.RestoreSnapshot(ctx, snapCfg, volCfg))
}

func TestSANDriver_SnapshotOps_ErrorPaths(t *testing.T) {
	t.Run("GetSnapshot_refresh_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)
		_, err := driver.GetSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			&storage.VolumeConfig{InternalName: testVolumeName},
		)
		assert.Error(t, err)
	})

	t.Run("GetSnapshot_volume_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeExists(ctx, volCfg).Return(false, nil, errFailed).Times(1)
		_, err := driver.GetSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.Error(t, err)
	})

	t.Run("GetSnapshot_snapshot_api_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		vol := getTestVolume()
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeExists(ctx, volCfg).Return(true, vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, "s1").Return(nil, errFailed).Times(1)
		_, err := driver.GetSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.Error(t, err)
	})

	t.Run("GetSnapshot_not_ready_returns_nil_nil", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		vol := getTestVolume()
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().VolumeExists(ctx, volCfg).Return(true, vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, "s1").Return(&api.Snapshot{
			Name:    "s1",
			State:   api.SnapshotStateCreating,
			Created: time.Unix(0, 0),
		}, nil).Times(1)
		s, err := driver.GetSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.NoError(t, err)
		assert.Nil(t, s)
	})

	t.Run("GetSnapshots_refresh_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)
		_, err := driver.GetSnapshots(ctx, &storage.VolumeConfig{InternalName: testVolumeName})
		assert.Error(t, err)
	})

	t.Run("GetSnapshots_volume_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(nil, errFailed).Times(1)
		_, err := driver.GetSnapshots(ctx, volCfg)
		assert.Error(t, err)
	})

	t.Run("GetSnapshots_list_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		vol := getTestVolume()
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotsForVolume(ctx, vol).Return(nil, errFailed).Times(1)
		_, err := driver.GetSnapshots(ctx, volCfg)
		assert.Error(t, err)
	})

	t.Run("RestoreSnapshot_refresh_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)
		err := driver.RestoreSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			&storage.VolumeConfig{InternalName: testVolumeName},
		)
		assert.Error(t, err)
	})

	t.Run("RestoreSnapshot_volume_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(nil, errFailed).Times(1)
		err := driver.RestoreSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.Error(t, err)
	})

	t.Run("RestoreSnapshot_snapshot_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		vol := getTestVolume()
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, "s1").Return(nil, errFailed).Times(1)
		err := driver.RestoreSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.Error(t, err)
	})

	t.Run("RestoreSnapshot_restore_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		vol := getTestVolume()
		snap := &api.Snapshot{Name: "s1", State: api.SnapshotStateReady}
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, "s1").Return(snap, nil).Times(1)
		mockAPI.EXPECT().RestoreSnapshot(ctx, vol, snap).Return(errFailed).Times(1)
		err := driver.RestoreSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.Error(t, err)
	})

	t.Run("RestoreSnapshot_wait_error", func(t *testing.T) {
		mockAPI, driver := newMockSANDriver(t)
		vol := getTestVolume()
		snap := &api.Snapshot{Name: "s1", State: api.SnapshotStateReady}
		volCfg := &storage.VolumeConfig{InternalName: testVolumeName}
		mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
		mockAPI.EXPECT().Volume(ctx, volCfg).Return(vol, nil).Times(1)
		mockAPI.EXPECT().SnapshotForVolume(ctx, vol, "s1").Return(snap, nil).Times(1)
		mockAPI.EXPECT().RestoreSnapshot(ctx, vol, snap).Return(nil).Times(1)
		mockAPI.EXPECT().WaitForVolumeState(ctx, vol, api.VolumeStateReady, gomock.Any(), api.DefaultSDKTimeout).Return(api.VolumeStateReady, errFailed).Times(1)
		err := driver.RestoreSnapshot(ctx,
			&storage.SnapshotConfig{InternalName: "s1", VolumeInternalName: testVolumeName},
			volCfg,
		)
		assert.Error(t, err)
	})
}

func TestSANDriver_InitializeGCNVAPIClient_InvalidTimeouts(t *testing.T) {
	_, driver := newMockSANDriver(t)

	driver.Config.SDKTimeout = "not-a-number"
	_, err := driver.initializeGCNVAPIClient(ctx, &driver.Config)
	assert.Error(t, err)

	driver.Config.SDKTimeout = ""
	driver.Config.MaxCacheAge = "not-a-number"
	_, err = driver.initializeGCNVAPIClient(ctx, &driver.Config)
	assert.Error(t, err)
}

func TestSANDriver_InitializeGCNVAPIClient_Success_UsesDefaultsAndOverrides(t *testing.T) {
	_, driver := newMockSANDriver(t)

	orig := newGCNVDriver
	defer func() { newGCNVDriver = orig }()

	mockCtrl := gomock.NewController(t)
	mockClient := mockapi.NewMockGCNV(mockCtrl)

	// Exercise both default and override parsing paths.
	driver.Config.SDKTimeout = "10"
	driver.Config.MaxCacheAge = "15"

	newGCNVDriver = func(ctx context.Context, cfg *api.ClientConfig) (api.GCNV, error) {
		assert.Equal(t, driver.Config.ProjectNumber, cfg.ProjectNumber)
		assert.Equal(t, driver.Config.Location, cfg.Location)
		assert.Equal(t, 10*time.Second, cfg.SDKTimeout)
		assert.Equal(t, 15*time.Second, cfg.MaxCacheAge)
		return mockClient, nil
	}

	// initializeGCNVAPIClient also calls Init on the created client
	mockClient.EXPECT().Init(ctx, driver.pools).Return(nil).Times(1)

	client, err := driver.initializeGCNVAPIClient(ctx, &driver.Config)
	assert.NoError(t, err)
	assert.Equal(t, mockClient, client)
}

func TestSANDriver_InitializeGCNVAPIClient_Success_UsesDefaults(t *testing.T) {
	_, driver := newMockSANDriver(t)

	orig := newGCNVDriver
	defer func() { newGCNVDriver = orig }()

	mockCtrl := gomock.NewController(t)
	mockClient := mockapi.NewMockGCNV(mockCtrl)

	// Exercise the default paths (no config overrides).
	driver.Config.SDKTimeout = ""
	driver.Config.MaxCacheAge = ""

	newGCNVDriver = func(ctx context.Context, cfg *api.ClientConfig) (api.GCNV, error) {
		assert.Equal(t, api.DefaultSDKTimeout, cfg.SDKTimeout)
		assert.Equal(t, api.DefaultMaxCacheAge, cfg.MaxCacheAge)
		return mockClient, nil
	}

	// initializeGCNVAPIClient also calls Init on the created client
	mockClient.EXPECT().Init(ctx, driver.pools).Return(nil).Times(1)

	client, err := driver.initializeGCNVAPIClient(ctx, &driver.Config)
	assert.NoError(t, err)
	assert.Equal(t, mockClient, client)
}

func TestSANDriver_InitializeGCNVConfig_SecretInjectionSuccess(t *testing.T) {
	_, driver := newMockSANDriver(t)

	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `{
		"projectNumber": "123456789",
		"location": "us-central1",
		"apiKey": {
			"type": "service_account",
			"project_id": "test-project",
			"private_key_id": "",
			"private_key": ""
		}
	}`

	backendSecret := map[string]string{
		"private_key":    "test-private-key",
		"private_key_id": "test-private-key-id",
	}

	config, err := driver.initializeGCNVConfig(ctx, configJSON, commonConfig, backendSecret)
	assert.NoError(t, err)
	assert.Equal(t, "test-private-key", config.APIKey.PrivateKey)
	assert.Equal(t, "test-private-key-id", config.APIKey.PrivateKeyID)
}

func TestSANDriver_InitializeStoragePools_NoVPools(t *testing.T) {
	_, driver := newMockSANDriver(t)

	driver.Config.BackendName = "backend1"
	driver.Config.Region = "r1"
	driver.Config.Zone = "z1"
	driver.Config.Size = "100G"
	driver.Config.ServiceLevel = "flex" // SAN requires Flex
	driver.Config.Network = "net1"
	driver.Config.StoragePools = []string{"cp1", "cp2"}

	driver.initializeStoragePools(ctx)

	assert.Len(t, driver.pools, 1)
	for _, pool := range driver.pools {
		assert.Equal(t, "100G", pool.InternalAttributes()[Size])
		assert.Equal(t, "Flex", pool.InternalAttributes()[ServiceLevel]) // Normalized to title case
		assert.Equal(t, "net1", pool.InternalAttributes()[Network])
		assert.Equal(t, "cp1,cp2", pool.InternalAttributes()[CapacityPools])
	}
}

func TestSANDriver_InitializeStoragePools_WithVPools(t *testing.T) {
	_, driver := newMockSANDriver(t)

	driver.Config.BackendName = "backend2"
	driver.Config.Region = "r1"
	driver.Config.Zone = "z1"
	driver.Config.Size = "100G"
	driver.Config.ServiceLevel = "flex" // SAN requires Flex
	driver.Config.Network = "net1"
	driver.Config.StoragePools = []string{"cp1"}
	driver.Config.SupportedTopologies = []map[string]string{{"kubernetes.io/hostname": "nodeA"}}
	driver.Config.FileSystemType = "ext4" // driver-level default
	driver.Config.FormatOptions = "-E nodiscard"

	driver.Config.Storage = []drivers.GCNVStorageDriverPool{
		{
			Labels:              map[string]string{"a": "b"},
			Region:              "r2", // override
			Zone:                "z2", // override
			StoragePools:        []string{"cp2", "cp3"},
			ServiceLevel:        "flex", // SAN requires Flex (even per-pool)
			Network:             "net2",
			SupportedTopologies: []map[string]string{{"topology.kubernetes.io/zone": "z2"}},
			GCNVStorageDriverConfigDefaults: drivers.GCNVStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "200G",
				},
				FileSystemType: "xfs",            // override for this pool
				FormatOptions:  "-f -i size=512", // override for this pool
			},
		},
		{}, // uses defaults
	}

	driver.initializeStoragePools(ctx)

	assert.Len(t, driver.pools, 2)

	p0, ok := driver.pools["backend2_pool_0"]
	assert.True(t, ok)
	assert.Equal(t, "200G", p0.InternalAttributes()[Size])
	assert.Equal(t, "Flex", p0.InternalAttributes()[ServiceLevel]) // Normalized to title case
	assert.Equal(t, "net2", p0.InternalAttributes()[Network])
	assert.Equal(t, "cp2,cp3", p0.InternalAttributes()[CapacityPools])
	assert.Equal(t, "xfs", p0.InternalAttributes()[FileSystemType], "Pool 0 should use per-pool FileSystemType")
	assert.Equal(t, "-f -i size=512", p0.InternalAttributes()[FormatOptions], "Pool 0 should use per-pool FormatOptions")

	p1, ok := driver.pools["backend2_pool_1"]
	assert.True(t, ok)
	assert.Equal(t, "100G", p1.InternalAttributes()[Size])
	assert.Equal(t, "Flex", p1.InternalAttributes()[ServiceLevel]) // Normalized to title case
	assert.Equal(t, "net1", p1.InternalAttributes()[Network])
	assert.Equal(t, "cp1", p1.InternalAttributes()[CapacityPools])
	assert.Equal(t, "ext4", p1.InternalAttributes()[FileSystemType], "Pool 1 should use driver-level FileSystemType")
	assert.Equal(t, "-E nodiscard", p1.InternalAttributes()[FormatOptions], "Pool 1 should use driver-level FormatOptions")
}

// ============================================================================
// Volume creation tests
// ============================================================================

func TestSANDriver_Create_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	internalName := "test_volume"
	expectedFullName := fmt.Sprintf("projects/test-project/locations/us-central1/volumes/%s", internalName)

	volConfig := &storage.VolumeConfig{
		Name:          testVolumeName,
		InternalName:  internalName,
		Size:          testVolumeSizeStr,
		FileSystem:    "ext4",
		FormatOptions: "-F",
	}

	pool := driver.pools["test-pool"]

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, pool, "", drivers.TieringPolicyNone).Return([]*api.CapacityPool{{Name: "test-pool"}}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return([]*api.CapacityPool{{Name: "test-pool"}}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
			// Verify request - Name uses InternalName (with underscores)
			assert.Equal(t, internalName, req.Name)
			// SizeBytes should be testActualVolumeSize (101 GiB) due to +1 GiB buffer for GCNV metadata overhead
			assert.Equal(t, testActualVolumeSize, req.SizeBytes)
			assert.Equal(t, api.OSTypeLinux, req.OSType)
			assert.Contains(t, req.ProtocolTypes, api.ProtocolTypeISCSI)
			// Labels may have modified keys due to fixGCPLabelKey (dots/slashes -> underscores, lowercase)
			// Check for fstype label (key will be transformed)
			foundFstype := false
			for k, v := range req.Labels {
				if strings.Contains(k, "fstype") && v == "ext4" {
					foundFstype = true
					break
				}
			}
			assert.True(t, foundFstype, "should contain fstype label with value ext4")
			// Check for formatOptions label (key will be transformed, value also lowercased)
			foundFormatOpts := false
			for k, v := range req.Labels {
				if strings.Contains(k, "formatoptions") && (v == "-f" || v == "-F") {
					foundFormatOpts = true
					break
				}
			}
			assert.True(t, foundFormatOpts, "should contain formatOptions label with value -F (may be transformed to -f)")

			created := getTestVolume()
			created.Name = internalName
			created.CreationToken = internalName
			created.FullName = expectedFullName
			return created, nil
		},
	)
	// Wait for volume to become ready
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.NoError(t, err, "Create should succeed")
	assert.Equal(t, expectedFullName, volConfig.InternalID, "InternalID should be set to the full GCP ID")
}

func TestSANDriver_Create_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	internalName := "test_volume"

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: internalName,
		Size:         testVolumeSizeStr,
		FileSystem:   "ext4",
	}

	pool := driver.pools["test-pool"]

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, getTestVolume(), nil).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err, "Create should fail when volume exists")
	assert.Contains(t, err.Error(), "already exists")
}

func TestSANDriver_Create_InvalidVolumeName(t *testing.T) {
	// Note: Validation was removed to match original behavior before validation was added.
	// This test is kept for future use if validation is re-added.
	t.Skip("Validation removed - test skipped")
}

func TestSANDriver_Create_InvalidCreationToken(t *testing.T) {
	// Note: Validation was removed to match original behavior before validation was added.
	// This test is kept for future use if validation is re-added.
	t.Skip("Validation removed - test skipped")
}

func TestSANDriver_Create_InvalidSize(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeInternalName,
		Size:         "invalid",
		FileSystem:   "ext4",
	}

	pool := driver.pools["test-pool"]

	// Mock expectations - check if volume exists first
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err, "Create should fail with invalid size")
	assert.Contains(t, err.Error(), "invalid volume size")
}

func TestSANDriver_Create_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: testVolumeSizeStr}
	pool := driver.pools["test-pool"]

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_Create_VolumeCreatingState(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: testVolumeSizeStr}
	pool := driver.pools["test-pool"]

	existing := &api.Volume{Name: testVolumeInternalName, State: api.VolumeStateCreating}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, existing, nil).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume state is still")
}

func TestSANDriver_Create_SizeTooLarge(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	// Use a size that exceeds GCNV's 128 TiB maximum (128 TiB = 140737488355328 bytes)
	// This will pass CheckVolumeSizeLimits but fail in calculateActualSizeBytes
	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: "140737488355328"}
	pool := driver.pools["test-pool"]

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds GCNV maximum")
}

func TestSANDriver_Create_NoCapacityPools(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: testVolumeSizeStr}
	pool := driver.pools["test-pool"]

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, pool, "", drivers.TieringPolicyNone).Return([]*api.CapacityPool{}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return([]*api.CapacityPool{}).Times(1)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no GCNV storage pools found")
}

func TestSANDriver_Create_MultipleCapacityPools_FirstFailsSecondSucceeds(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: testVolumeSizeStr}
	pool := driver.pools["test-pool"]

	c1 := &api.CapacityPool{Name: "cp1"}
	c2 := &api.CapacityPool{Name: "cp2"}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, pool, "", drivers.TieringPolicyNone).Return([]*api.CapacityPool{c1, c2}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*api.CapacityPool{c1, c2}).Times(1)

	gomock.InOrder(
		mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
				assert.Equal(t, "cp1", req.CapacityPool)
				return nil, errFailed
			},
		),
		mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
				assert.Equal(t, "cp2", req.CapacityPool)
				return getTestVolume(), nil
			},
		),
		// Wait for volume to become ready
		mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil),
	)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.NoError(t, err)
}

func TestSANDriver_Create_AllCapacityPoolsFail(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: testVolumeSizeStr}
	pool := driver.pools["test-pool"]

	c1 := &api.CapacityPool{Name: "cp1"}
	c2 := &api.CapacityPool{Name: "cp2"}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, pool, "", drivers.TieringPolicyNone).Return([]*api.CapacityPool{c1, c2}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*api.CapacityPool{c1, c2}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).Return(nil, errFailed).Times(2)

	err := driver.Create(ctx, volConfig, pool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not create LUN in any capacity pool")
}

func TestSANDriver_Create_BreaksWhenLabelCountExceeded(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{Name: testVolumeName, InternalName: testVolumeInternalName, Size: testVolumeSizeStr}

	mockCtrl := gomock.NewController(t)
	mockPool := mockstorage.NewMockPool(mockCtrl)

	labels := make(map[string]string)
	for i := 0; i < api.MaxLabelCount+10; i++ {
		labels[fmt.Sprintf("key%d", i)] = "v"
	}

	mockPool.EXPECT().InternalAttributes().Return(map[string]string{ServiceLevel: "", FileSystemType: drivers.DefaultFileSystemType, FormatOptions: ""}).AnyTimes()
	mockPool.EXPECT().Name().Return("mock-pool").AnyTimes()
	mockPool.EXPECT().GetLabels(ctx, "").Return(labels).Times(1)

	// Add mock pool to driver.pools so validation passes
	driver.pools["mock-pool"] = mockPool

	c1 := &api.CapacityPool{Name: "cp1"}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CapacityPoolsForStoragePool(ctx, mockPool, "", drivers.TieringPolicyNone).Return([]*api.CapacityPool{c1}).Times(1)
	mockAPI.EXPECT().FilterCapacityPoolsOnTopology(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*api.CapacityPool{c1}).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
			assert.Greater(t, len(req.Labels), api.MaxLabelCount)
			return getTestVolume(), nil
		},
	).Times(1)
	// Wait for volume to become ready
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil).Times(1)

	err := driver.Create(ctx, volConfig, mockPool, nil)
	assert.NoError(t, err)
}

// ============================================================================
// populateConfigurationDefaults tests
// ============================================================================

func TestSANDriver_populateConfigurationDefaults_AllDefaults(t *testing.T) {
	_, driver := newMockSANDriver(t)

	config := &drivers.GCNVStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
			DebugTraceFlags:   debugTraceFlags,
		},
	}

	err := driver.populateConfigurationDefaults(ctx, config)

	assert.NoError(t, err)
	assert.NotNil(t, config.StoragePrefix)
	assert.Equal(t, drivers.DefaultVolumeSize, config.Size)
	assert.Equal(t, api.ServiceLevelFlex, config.ServiceLevel)
	assert.Equal(t, defaultLimitVolumeSize, config.LimitVolumeSize)
	assert.Equal(t, drivers.DefaultFileSystemType, config.FileSystemType)
	assert.Equal(t, DefaultExt4FormatOptions, config.FormatOptions)
	assert.Equal(t, driver.defaultCreateTimeout(), driver.volumeCreateTimeout)
}

func TestSANDriver_populateConfigurationDefaults_PreservesExistingValues(t *testing.T) {
	_, driver := newMockSANDriver(t)

	prefix := "custom-"
	config := &drivers.GCNVStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
			StoragePrefix:     &prefix,
			DebugTraceFlags:   debugTraceFlags,
			LimitVolumeSize:   "500Gi",
		},
		GCNVStorageDriverPool: drivers.GCNVStorageDriverPool{
			ServiceLevel: api.ServiceLevelFlex, // SAN requires Flex
			GCNVStorageDriverConfigDefaults: drivers.GCNVStorageDriverConfigDefaults{
				FileSystemType: "xfs",
				FormatOptions:  "-f",
			},
		},
	}
	config.Size = "100Gi"

	err := driver.populateConfigurationDefaults(ctx, config)

	assert.NoError(t, err)
	assert.Equal(t, "custom-", *config.StoragePrefix)
	assert.Equal(t, "100Gi", config.Size)
	assert.Equal(t, api.ServiceLevelFlex, config.ServiceLevel)
	assert.Equal(t, "500Gi", config.LimitVolumeSize)
	assert.Equal(t, "xfs", config.FileSystemType)
	assert.Equal(t, "-f", config.FormatOptions)
}

func TestSANDriver_populateConfigurationDefaults_VolumeCreateTimeout(t *testing.T) {
	_, driver := newMockSANDriver(t)

	config := &drivers.GCNVStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
			DebugTraceFlags:   debugTraceFlags,
		},
		VolumeCreateTimeout: "300",
	}

	err := driver.populateConfigurationDefaults(ctx, config)

	assert.NoError(t, err)
	assert.Equal(t, 300*time.Second, driver.volumeCreateTimeout)
}

func TestSANDriver_populateConfigurationDefaults_InvalidVolumeCreateTimeout(t *testing.T) {
	_, driver := newMockSANDriver(t)

	config := &drivers.GCNVStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
			DebugTraceFlags:   debugTraceFlags,
		},
		VolumeCreateTimeout: "not-a-number",
	}

	err := driver.populateConfigurationDefaults(ctx, config)

	assert.Error(t, err)
}

func TestSANDriver_populateConfigurationDefaults_FormatOptionsForFilesystems(t *testing.T) {
	tests := []struct {
		name           string
		fileSystemType string
		expectedFormat string
	}{
		{"ext3", "ext3", DefaultExt3FormatOptions},
		{"ext4", "ext4", DefaultExt4FormatOptions},
		{"xfs", "xfs", DefaultXfsFormatOptions},
		{"empty defaults to ext4", "", DefaultExt4FormatOptions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, driver := newMockSANDriver(t)

			config := &drivers.GCNVStorageDriverConfig{
				CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
					StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
					DebugTraceFlags:   debugTraceFlags,
				},
				GCNVStorageDriverPool: drivers.GCNVStorageDriverPool{
					GCNVStorageDriverConfigDefaults: drivers.GCNVStorageDriverConfigDefaults{
						FileSystemType: tt.fileSystemType,
					},
				},
			}

			err := driver.populateConfigurationDefaults(ctx, config)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFormat, config.FormatOptions)
			if tt.fileSystemType == "" {
				assert.Equal(t, drivers.DefaultFileSystemType, config.FileSystemType)
			} else {
				assert.Equal(t, tt.fileSystemType, config.FileSystemType)
			}
		})
	}
}

func TestSANDriver_populateConfigurationDefaults_ServiceLevelFlex(t *testing.T) {
	_, driver := newMockSANDriver(t)

	config := &drivers.GCNVStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: tridentconfig.GCNVSANStorageDriverName,
			DebugTraceFlags:   debugTraceFlags,
		},
	}

	err := driver.populateConfigurationDefaults(ctx, config)

	assert.NoError(t, err)
	// SAN driver should default to Flex service level (required for block volumes)
	assert.Equal(t, api.ServiceLevelFlex, config.ServiceLevel)
}

// ============================================================================
// waitForVolumeCreate tests
// ============================================================================

func TestSANDriver_waitForVolumeCreate_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Volume reaches Ready state successfully
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return(api.VolumeStateReady, nil).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.NoError(t, err)
}

func TestSANDriver_waitForVolumeCreate_VolumeCreating(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Timeout while volume is still in Creating state - should return VolumeCreatingError
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return(api.VolumeStateCreating, errFailed).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.Error(t, err)
	assert.True(t, errors.IsVolumeCreatingError(err), "should return VolumeCreatingError")
}

func TestSANDriver_waitForVolumeCreate_VolumeUnspecified(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Timeout while volume is in Unspecified state - should return VolumeCreatingError
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return(api.VolumeStateUnspecified, errFailed).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.Error(t, err)
	assert.True(t, errors.IsVolumeCreatingError(err), "should return VolumeCreatingError")
}

func TestSANDriver_waitForVolumeCreate_VolumeDeleting_WaitSuccess(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Volume is in Deleting state, wait for deletion to complete successfully
	gomock.InOrder(
		mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
			Return(api.VolumeStateDeleting, errFailed),
		mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateDeleted, gomock.Any(), gomock.Any()).
			Return(api.VolumeStateDeleted, nil),
	)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.NoError(t, err)
}

func TestSANDriver_waitForVolumeCreate_VolumeDeleting_WaitFails(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Volume is in Deleting state, wait for deletion fails
	gomock.InOrder(
		mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
			Return(api.VolumeStateDeleting, errFailed),
		mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateDeleted, gomock.Any(), gomock.Any()).
			Return(api.VolumeStateDeleting, errFailed),
	)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.Error(t, err)
}

func TestSANDriver_waitForVolumeCreate_VolumeError_DeleteSuccess(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Volume reached Error state, delete succeeds
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return(api.VolumeStateError, errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.NoError(t, err)
}

func TestSANDriver_waitForVolumeCreate_VolumeError_DeleteFails(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Volume reached Error state, delete fails
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return(api.VolumeStateError, errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(errFailed).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.Error(t, err)
}

func TestSANDriver_waitForVolumeCreate_EmptyState(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Empty state means volume could not be found or API error
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return("", errFailed).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.Error(t, err)
	assert.Equal(t, errFailed, err)
}

func TestSANDriver_waitForVolumeCreate_UnexpectedState(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Unexpected state - should log and return nil (fall through)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady, gomock.Any(), gomock.Any()).
		Return("SomeUnexpectedState", errFailed).Times(1)

	err := driver.waitForVolumeCreate(ctx, volume)
	assert.NoError(t, err)
}

// ============================================================================
// Volume deletion tests
// ============================================================================

func TestSANDriver_Destroy_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	volume := getTestVolume()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateDeleted,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return(api.VolumeStateDeleted, nil)

	err := driver.Destroy(ctx, volConfig)
	assert.NoError(t, err, "Destroy should succeed")
}

func TestSANDriver_Destroy_VolumeNotFound(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, nil)

	err := driver.Destroy(ctx, volConfig)
	assert.NoError(t, err, "Destroy should succeed when volume not found")
}

func TestSANDriver_Destroy_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.Destroy(ctx, &storage.VolumeConfig{InternalName: testVolumeName})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_Destroy_VolumeExistsError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	err := driver.Destroy(ctx, volConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking for existing volume")
}

func TestSANDriver_Destroy_DeleteError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(errFailed).Times(1)

	err := driver.Destroy(ctx, volConfig)
	assert.Error(t, err)
}

func TestSANDriver_Destroy_VolumeDeleting_RetryWaitsForState(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volume.State = api.VolumeStateDeleting

	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateDeleted,
		[]string{api.VolumeStateError}, driver.volumeCreateTimeout).Return(api.VolumeStateDeleted, nil)

	err := driver.Destroy(ctx, volConfig)
	assert.NoError(t, err, "Destroy should wait for deleting volume")
}

func TestSANDriver_Destroy_WaitForDeletionError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateDeleted,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return("", errFailed)

	err := driver.Destroy(ctx, volConfig)
	assert.Error(t, err)
}

// ============================================================================
// deleteAutomaticSnapshot tests
// ============================================================================

func TestSANDriver_deleteAutomaticSnapshot_NoAutomaticSnapshot(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// CloneSourceSnapshot is set, so no automatic snapshot was created
	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "user-snapshot",
		CloneSourceSnapshotInternal: "user-snapshot",
	}

	// Should return immediately without any API calls
	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_VolDeleteError(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Automatic snapshot exists (CloneSourceSnapshot empty but CloneSourceSnapshotInternal set)
	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "", // Empty means it was auto-created
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	// Pass a deletion error - should skip cleanup
	driver.deleteAutomaticSnapshot(ctx, errFailed, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_SourceVolumeNotFound(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(nil, errors.NotFoundError("not found"))

	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_SourceVolumeLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(nil, errFailed)

	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_SnapshotNotFound(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	sourceVolume := getTestVolume()
	sourceVolume.Name = "source-vol"

	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(sourceVolume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "auto-snap").Return(nil, errors.NotFoundError("not found"))

	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_SnapshotLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	sourceVolume := getTestVolume()
	sourceVolume.Name = "source-vol"

	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(sourceVolume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "auto-snap").Return(nil, errFailed)

	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	sourceVolume := getTestVolume()
	sourceVolume.Name = "source-vol"
	snapshot := &api.Snapshot{
		Name:     "auto-snap",
		FullName: "projects/test-project/locations/us-central1/volumes/source-vol/snapshots/auto-snap",
		Volume:   sourceVolume.Name,
		State:    api.SnapshotStateReady,
	}

	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(sourceVolume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "auto-snap").Return(snapshot, nil)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, snapshot, api.DefaultTimeout).Return(nil)

	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_deleteAutomaticSnapshot_DeleteFails(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	sourceVolume := getTestVolume()
	sourceVolume.Name = "source-vol"
	snapshot := &api.Snapshot{
		Name:     "auto-snap",
		FullName: "projects/test-project/locations/us-central1/volumes/source-vol/snapshots/auto-snap",
		Volume:   sourceVolume.Name,
		State:    api.SnapshotStateReady,
	}

	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(sourceVolume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "auto-snap").Return(snapshot, nil)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, snapshot, api.DefaultTimeout).Return(errFailed)

	// Should log error but not return it
	driver.deleteAutomaticSnapshot(ctx, nil, volConfig)
}

func TestSANDriver_Destroy_WithAutomaticSnapshot_CleansUp(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	// Volume config has an automatic snapshot (CloneSourceSnapshot empty but CloneSourceSnapshotInternal set)
	volConfig := &storage.VolumeConfig{
		Name:                        testVolumeName,
		InternalName:                testVolumeName,
		CloneSourceSnapshot:         "",
		CloneSourceSnapshotInternal: "auto-snap",
		CloneSourceVolumeInternal:   "source-vol",
	}

	volume := getTestVolume()
	sourceVolume := getTestVolume()
	sourceVolume.Name = "source-vol"
	snapshot := &api.Snapshot{
		Name:     "auto-snap",
		FullName: "projects/test-project/locations/us-central1/volumes/source-vol/snapshots/auto-snap",
		Volume:   sourceVolume.Name,
		State:    api.SnapshotStateReady,
	}

	// Mock expectations for Destroy
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateDeleted,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return(api.VolumeStateDeleted, nil)

	// Mock expectations for deleteAutomaticSnapshot (called via defer)
	mockAPI.EXPECT().VolumeByName(ctx, "source-vol").Return(sourceVolume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "auto-snap").Return(snapshot, nil)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, snapshot, api.DefaultTimeout).Return(nil)

	err := driver.Destroy(ctx, volConfig)
	assert.NoError(t, err, "Destroy should succeed and clean up automatic snapshot")
}

// ============================================================================
// Publish tests (CRITICAL - per-node host group logic)
// ============================================================================

func TestSANDriver_Publish_Success_NewHostGroup(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(nil, errors.NotFoundError("not found"))
	mockAPI.EXPECT().CreateHostGroup(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.HostGroupCreateRequest) (*api.HostGroup, error) {
			assert.Equal(t, testHostGroupName, req.ResourceID)
			assert.Equal(t, []string{testIQN}, req.Hosts)
			return hostGroup, nil
		},
	)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{}, nil)
	mockAPI.EXPECT().AddHostGroupToVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Publish should succeed")

	// Verify publishInfo populated correctly
	// GCNV doesn't expose LUN IDs via API, so 0 is used as placeholder; node discovers actual LUN by serial
	assert.Equal(t, int32(0), publishInfo.IscsiLunNumber)
	// Publish now decodes the hex serial number returned by GCNV into ASCII to match VPD page 0x80.
	assert.Equal(t, "lW801]ZJY-4F", publishInfo.IscsiLunSerial)
	assert.Equal(t, testTargetPortal, publishInfo.IscsiTargetPortal)
	assert.Equal(t, testTargetIQN, publishInfo.IscsiTargetIQN)
	assert.Equal(t, "iscsi", publishInfo.SANType)
	assert.False(t, publishInfo.UseCHAP)
	assert.False(t, publishInfo.SharedTarget)
	assert.Equal(t, testHostGroupName, publishInfo.IscsiIgroup)
	assert.Equal(t, "ext4", publishInfo.FilesystemType)
	assert.Equal(t, "-F", publishInfo.FormatOptions)
}

func TestSANDriver_Publish_Success_ExistingHostGroup(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	// Mock expectations - host group already exists
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{}, nil)
	mockAPI.EXPECT().AddHostGroupToVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Publish should succeed with existing host group")
}

func TestSANDriver_Publish_ExistingHostGroup_AddsMissingIQN(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()
	// HostGroup exists but is missing the node IQN.
	hostGroup.Hosts = []string{"iqn.1994-05.com.redhat:some-other-node"}
	targetInfo := getTestISCSITargetInfo()

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().AddInitiatorsToHostGroup(ctx, hostGroup, []string{testIQN}).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{}, nil)
	mockAPI.EXPECT().AddHostGroupToVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err)
}

func TestSANDriver_Publish_InvalidIQNFormat_ReturnsError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{"not-an-iqn"},
	}

	volume := getTestVolume()

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IQN format")
}

func TestSANDriver_Publish_SerialNotHex_FailsFast(t *testing.T) {
	// Changed behavior: Publish now fails fast if the volume serial number
	// cannot be hex-decoded, since findLUNBySerial requires a valid decoded serial.
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()
	volume.SerialNumber = "not-hex"
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{}, nil)
	mockAPI.EXPECT().AddHostGroupToVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err, "Publish should fail when serial is not hex-decodable")
	assert.Contains(t, err.Error(), "could not decode GCNV serial number")
}

func TestSANDriver_Publish_Success_AlreadyMapped(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	// Mock expectations - already mapped
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{hostGroup.Name}, nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Publish should succeed when already mapped")
}

func TestSANDriver_Publish_XFS_AddNouuidOption(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()
	// Use transformed key (GCP labels have dots/slashes replaced with underscores)
	volume.Labels["trident_netapp_io_fstype"] = "xfs"
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{hostGroup.Name}, nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Publish should succeed")
	assert.Equal(t, "nouuid", publishInfo.MountOptions, "XFS should have nouuid mount option")
}

func TestSANDriver_Publish_XFS_AppendNouuidOption(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: false,
		HostIQN:   []string{testIQN},
	}
	publishInfo.MountOptions = "rw,relatime"

	volume := getTestVolume()
	// Use transformed key (GCP labels have dots/slashes replaced with underscores)
	volume.Labels["trident_netapp_io_fstype"] = "xfs"
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{hostGroup.Name}, nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Publish should succeed")
	assert.Equal(t, "rw,relatime,nouuid", publishInfo.MountOptions, "XFS should append nouuid to existing options")
}

func TestSANDriver_Publish_LocalhostRejected(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:  testNodeName,
		Localhost: true, // Docker mode - should be rejected
		HostIQN:   []string{testIQN},
	}

	volume := getTestVolume()

	// Mock expectations - Publish should fail fast when Localhost=true
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.Error(t, err, "Publish should fail when Localhost=true (Docker mode not supported)")
	assert.Contains(t, err.Error(), "Docker mode", "Error should mention Docker mode rejection")
	assert.Contains(t, err.Error(), "not supported", "Error should indicate mode is not supported")
}

func TestSANDriver_Publish_NoHostIQN_GeneratesSynthetic(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName: testNodeName,
		HostIQN:  []string{}, // Empty IQN - should generate synthetic IQN
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()
	targetInfo := getTestISCSITargetInfo()

	// Mock expectations - Publish should generate synthetic IQN and proceed
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(nil, errors.NotFoundError("not found"))
	mockAPI.EXPECT().CreateHostGroup(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.HostGroupCreateRequest) (*api.HostGroup, error) {
			// Verify synthetic IQN was generated (starts with iqn.1994-05.com.netapp:trident-node-)
			assert.Len(t, req.Hosts, 1, "Should have one IQN")
			assert.True(t, strings.HasPrefix(req.Hosts[0], "iqn.1994-05.com.netapp:trident-node-"),
				"Synthetic IQN should have expected prefix")
			return hostGroup, nil
		},
	)
	mockAPI.EXPECT().VolumeMappedHostGroups(ctx, volume.FullName).Return([]string{}, nil)
	mockAPI.EXPECT().AddHostGroupToVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().ISCSITargetInfo(ctx, volume).Return(targetInfo, nil)

	err := driver.Publish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Publish should succeed with synthetic IQN")
}

// ============================================================================
// Unpublish tests (CRITICAL - cleanup logic)
// ============================================================================

func TestSANDriver_Unpublish_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName: testNodeName,
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().RemoveHostGroupFromVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().HostGroupVolumes(ctx, hostGroup.Name).Return([]string{}, nil)
	mockAPI.EXPECT().DeleteHostGroup(ctx, hostGroup).Return(nil)

	err := driver.Unpublish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Unpublish should succeed")
}

func TestSANDriver_Unpublish_HostGroupStillInUse(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName: testNodeName,
	}

	volume := getTestVolume()
	hostGroup := getTestHostGroup()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(hostGroup, nil)
	mockAPI.EXPECT().RemoveHostGroupFromVolume(ctx, volume.FullName, hostGroup.Name).Return(nil)
	mockAPI.EXPECT().HostGroupVolumes(ctx, hostGroup.Name).Return([]string{"other-volume"}, nil)
	// DeleteHostGroup should NOT be called

	err := driver.Unpublish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Unpublish should succeed even when host group still in use")
}

func TestSANDriver_Unpublish_VolumeNotFound(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName: testNodeName,
	}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errors.NotFoundError("not found"))

	err := driver.Unpublish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Unpublish should succeed when volume not found")
}

func TestSANDriver_Unpublish_HostGroupNotFound(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
	}

	publishInfo := &models.VolumePublishInfo{
		HostName: testNodeName,
	}

	volume := getTestVolume()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().HostGroupByName(ctx, testHostGroupName).Return(nil, errors.NotFoundError("not found"))

	err := driver.Unpublish(ctx, volConfig, publishInfo)
	assert.NoError(t, err, "Unpublish should succeed when host group not found")
}

// ============================================================================
// ReconcileNodeAccess tests (CRITICAL - node lifecycle)
// ============================================================================

func TestSANDriver_ReconcileNodeAccess_NoHostGroupsNoOp(t *testing.T) {
	// ReconcileNodeAccess no longer creates host groups - Publish is the sole creator.
	// With no existing host groups, ReconcileNodeAccess is essentially a no-op.
	mockAPI, driver := newMockSANDriver(t)

	nodes := []*models.Node{
		{Name: "node-1", IQN: "iqn.xxx.node-1"},
		{Name: "node-2", IQN: "iqn.xxx.node-2"},
	}

	// Mock expectations - no existing host groups, nothing to update or delete
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().HostGroups(ctx).Return([]*api.HostGroup{}, nil)

	err := driver.ReconcileNodeAccess(ctx, nodes, "", "")
	assert.NoError(t, err, "ReconcileNodeAccess should succeed")
}

func TestSANDriver_ReconcileNodeAccess_UpdateExistingHostGroups(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	nodes := []*models.Node{
		{Name: "node-1", IQN: "iqn.xxx.node-1-new"},
	}

	existingHostGroup := &api.HostGroup{
		Name:  fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/trident-node-node-1-%s", testUUID),
		Hosts: []string{"iqn.xxx.node-1-old"},
	}

	// Mock expectations - ReconcileNodeAccess updates IQNs on existing host groups
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().HostGroups(ctx).Return([]*api.HostGroup{existingHostGroup}, nil)
	mockAPI.EXPECT().UpdateHostGroup(ctx, existingHostGroup, []string{"iqn.xxx.node-1-new"}).Return(nil)

	err := driver.ReconcileNodeAccess(ctx, nodes, "", "")
	assert.NoError(t, err, "ReconcileNodeAccess should succeed")
}

func TestSANDriver_ReconcileNodeAccess_DeleteOrphanedHostGroups(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	nodes := []*models.Node{
		{Name: "node-1", IQN: "iqn.xxx.node-1"},
	}

	existingHostGroups := []*api.HostGroup{
		{
			Name:  fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/trident-node-node-1-%s", testUUID),
			Hosts: []string{"iqn.xxx.node-1"},
		},
		{
			Name:  fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/trident-node-node-2-%s", testUUID),
			Hosts: []string{"iqn.xxx.node-2"},
		},
	}

	// Mock expectations - orphaned host group (node-2) with no volumes gets deleted
	// Note: HostGroupVolumes is called twice - once without lock and once with lock (TOCTOU protection)
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().HostGroups(ctx).Return(existingHostGroups, nil)
	mockAPI.EXPECT().HostGroupVolumes(ctx, existingHostGroups[1].Name).Return([]string{}, nil).Times(2)
	mockAPI.EXPECT().DeleteHostGroup(ctx, existingHostGroups[1]).Return(nil)

	err := driver.ReconcileNodeAccess(ctx, nodes, "", "")
	assert.NoError(t, err, "ReconcileNodeAccess should succeed")
}

func TestSANDriver_ReconcileNodeAccess_KeepOrphanedHostGroupsWithVolumes(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	nodes := []*models.Node{
		{Name: "node-1", IQN: "iqn.xxx.node-1"},
	}

	existingHostGroups := []*api.HostGroup{
		{
			Name:  fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/trident-node-node-1-%s", testUUID),
			Hosts: []string{"iqn.xxx.node-1"},
		},
		{
			Name:  fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/trident-node-node-2-%s", testUUID),
			Hosts: []string{"iqn.xxx.node-2"},
		},
	}

	// Mock expectations - orphaned host group still has volumes, so it's NOT deleted
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().HostGroups(ctx).Return(existingHostGroups, nil)
	mockAPI.EXPECT().HostGroupVolumes(ctx, existingHostGroups[1].Name).Return([]string{"some-volume"}, nil)
	// DeleteHostGroup should NOT be called since host group still has volumes

	err := driver.ReconcileNodeAccess(ctx, nodes, "", "")
	assert.NoError(t, err, "ReconcileNodeAccess should succeed")
}

// ============================================================================
// Snapshot tests
// ============================================================================

func TestSANDriver_CreateSnapshot_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "test-snap",
		VolumeInternalName: testVolumeName,
	}

	volConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	volume := getTestVolume()
	snapshot := &api.Snapshot{
		Name:    "test-snap",
		Created: time.Now(),
		State:   api.SnapshotStateReady,
	}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	// First check if snapshot exists (for idempotency)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(nil, errors.NotFoundError("not found"))
	// Create the snapshot
	mockAPI.EXPECT().CreateSnapshot(ctx, volume, "test-snap", gomock.Any()).Return(snapshot, nil)

	result, err := driver.CreateSnapshot(ctx, snapConfig, volConfig)
	assert.NoError(t, err, "CreateSnapshot should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, storage.SnapshotStateOnline, result.State)
}

func TestSANDriver_CreateSnapshot_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	_, err := driver.CreateSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		&storage.VolumeConfig{InternalName: testVolumeName},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_CreateSnapshot_VolumeError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	_, err := driver.CreateSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking for existing volume")
}

func TestSANDriver_CreateSnapshot_SnapshotLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(nil, errFailed).Times(1)

	_, err := driver.CreateSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking for existing snapshot")
}

func TestSANDriver_CreateSnapshot_CreateError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(nil, errors.NotFoundError("not found")).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, volume, "test-snap", gomock.Any()).Return(nil, errFailed).Times(1)

	_, err := driver.CreateSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not create snapshot")
}

func TestSANDriver_CreateSnapshot_AlreadyExistsReady(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	snapshot := &api.Snapshot{Name: "test-snap", Created: time.Now(), State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(snapshot, nil).Times(1)

	result, err := driver.CreateSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, storage.SnapshotStateOnline, result.State)
}

func TestSANDriver_CreateSnapshot_AlreadyExistsNotReady(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	snapshot := &api.Snapshot{Name: "test-snap", Created: time.Now(), State: "Creating"}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(snapshot, nil).Times(1)

	_, err := driver.CreateSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists but is")
}

func TestSANDriver_DeleteSnapshot_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	snapConfig := &storage.SnapshotConfig{
		InternalName:       "test-snap",
		VolumeInternalName: testVolumeName,
	}

	volConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	volume := getTestVolume()
	snapshot := &api.Snapshot{
		Name: "test-snap",
	}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(snapshot, nil)
	mockAPI.EXPECT().DeleteSnapshot(ctx, volume, snapshot, gomock.Any()).Return(nil)

	err := driver.DeleteSnapshot(ctx, snapConfig, volConfig)
	assert.NoError(t, err, "DeleteSnapshot should succeed")
}

func TestSANDriver_DeleteSnapshot_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.DeleteSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		&storage.VolumeConfig{InternalName: testVolumeName},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_DeleteSnapshot_VolumeError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(false, nil, errFailed).Times(1)

	err := driver.DeleteSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking for existing volume")
}

func TestSANDriver_DeleteSnapshot_AlreadyDeleted(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(nil, errors.NotFoundError("not found")).Times(1)

	err := driver.DeleteSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.NoError(t, err)
}

func TestSANDriver_DeleteSnapshot_SnapshotLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(nil, errFailed).Times(1)

	err := driver.DeleteSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
}

func TestSANDriver_DeleteSnapshot_DeleteError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	snapshot := &api.Snapshot{Name: "test-snap"}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, volConfig).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, volume, "test-snap").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, volume, snapshot, gomock.Any()).Return(errFailed).Times(1)

	err := driver.DeleteSnapshot(ctx,
		&storage.SnapshotConfig{InternalName: "test-snap", VolumeInternalName: testVolumeName},
		volConfig,
	)
	assert.Error(t, err)
}

// ============================================================================
// Clone tests
// ============================================================================

func TestSANDriver_CreateClone_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
		FileSystem:                  "ext4",
		FormatOptions:               "-F",
	}

	sourceVolume := getTestVolume()
	snapshot := &api.Snapshot{
		Name:     "test-snap",
		FullName: "projects/12345/locations/us-central1/volumes/test-volume/snapshots/test-snap",
		State:    api.SnapshotStateReady,
	}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "test-snap").Return(snapshot, nil)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
			assert.Equal(t, "test_clone", req.Name)
			assert.Equal(t, snapshot.FullName, req.SnapshotID)
			assert.Contains(t, req.ProtocolTypes, api.ProtocolTypeISCSI)
			// Labels should be sanitized (dots and slashes replaced with underscores, lowercased)
			assert.Equal(t, "ext4", req.Labels["trident_netapp_io_fstype"])
			assert.Equal(t, "-f", req.Labels["trident_netapp_io_formatoptions"])
			return getTestVolume(), nil
		},
	)
	// Wait for clone to become ready
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.NoError(t, err, "CreateClone should succeed")
	assert.Equal(t, getTestVolume().FullName, cloneConfig.InternalID, "InternalID should be set to clone's FullName")
}

func TestSANDriver_CreateClone_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_CreateClone_CloneExistsCreating(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	existing := &api.Volume{Name: "test-clone", State: api.VolumeStateCreating}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(true, existing, nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume state is still")
}

func TestSANDriver_CreateClone_CloneExistsReady(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	existing := &api.Volume{Name: "test-clone", State: api.VolumeStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(true, existing, nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestSANDriver_CreateClone_SourceVolumeError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(nil, errFailed).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find source volume")
}

func TestSANDriver_CreateClone_SnapshotLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	sourceVolume := getTestVolume()

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "test-snap").Return(nil, errFailed).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find snapshot")
}

func TestSANDriver_CreateClone_SnapshotNotReady(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	sourceVolume := getTestVolume()
	snapshot := &api.Snapshot{Name: "test-snap", FullName: "snap-full", State: "Creating"}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "test-snap").Return(snapshot, nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source snapshot state is")
}

func TestSANDriver_CreateClone_NoSnapshot_CreateIntermediateSnapshotFlow(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                      "test-clone",
		InternalName:              "test_clone",
		CloneSourceVolumeInternal: testVolumeName,
		CloneSourceSnapshot:       "",
		Size:                      testVolumeSizeStr,
		FileSystem:                "ext4",
	}

	sourceVolume := getTestVolume()

	var snapName string
	created := &api.Snapshot{Name: "snap-created", FullName: "snap-full", State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), api.SnapshotTimeout).DoAndReturn(
		func(ctx context.Context, vol *api.Volume, name string, _ time.Duration) (*api.Snapshot, error) {
			snapName = name
			return created, nil
		},
	).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).DoAndReturn(
		func(ctx context.Context, vol *api.Volume, name string) (*api.Snapshot, error) {
			assert.Equal(t, snapName, name)
			return created, nil
		},
	).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
			assert.Equal(t, "snap-full", req.SnapshotID)
			return getTestVolume(), nil
		},
	).Times(1)
	// Wait for clone to become ready
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, created, gomock.Any()).Return(nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.NoError(t, err)
	assert.Equal(t, "snap-created", cloneConfig.CloneSourceSnapshotInternal)
	assert.Equal(t, getTestVolume().FullName, cloneConfig.InternalID)
}

func TestSANDriver_CreateClone_InvalidSize(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        "bad",
	}

	sourceVolume := getTestVolume()
	snapshot := &api.Snapshot{Name: "test-snap", FullName: "snap-full", State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "test-snap").Return(snapshot, nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid volume size")
}

func TestSANDriver_CreateClone_CreateLUNError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	sourceVolume := getTestVolume()
	snapshot := &api.Snapshot{Name: "test-snap", FullName: "snap-full", State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "test-snap").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).Return(nil, errFailed).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not create clone")
}

func TestSANDriver_CreateClone_LargerThanSource(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	// Request a clone larger than source volume (200GB vs 100GB source)
	largerSize := strconv.FormatInt(int64(200*1024*1024*1024), 10)
	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        largerSize,
		FileSystem:                  "ext4",
	}

	sourceVolume := getTestVolume()
	snapshot := &api.Snapshot{
		Name:     "test-snap",
		FullName: "snap-full",
		State:    api.SnapshotStateReady,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, "test-snap").Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *api.VolumeCreateRequest) (*api.Volume, error) {
			// The clone size should be calculated with the buffer
			assert.Greater(t, req.SizeBytes, int64(200*1024*1024*1024))
			return getTestVolume(), nil
		},
	).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.NoError(t, err)
}

func TestSANDriver_CreateClone_IntermediateSnapshotCleanupOnCreateFailure(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                      "test-clone",
		InternalName:              "test_clone",
		CloneSourceVolumeInternal: testVolumeName,
		CloneSourceSnapshot:       "", // No snapshot, triggers intermediate snapshot creation
		Size:                      testVolumeSizeStr,
		FileSystem:                "ext4",
	}

	sourceVolume := getTestVolume()
	intermediateSnap := &api.Snapshot{Name: "snap-intermediate", FullName: "snap-full", State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), api.SnapshotTimeout).Return(intermediateSnap, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(intermediateSnap, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).Return(nil, errFailed).Times(1)
	// Intermediate snapshot should be cleaned up on failure
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, intermediateSnap, gomock.Any()).Return(nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not create clone")
}

func TestSANDriver_CreateClone_IntermediateSnapshotCleanupOnWaitFailure(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                      "test-clone",
		InternalName:              "test_clone",
		CloneSourceVolumeInternal: testVolumeName,
		CloneSourceSnapshot:       "", // No snapshot, triggers intermediate snapshot creation
		Size:                      testVolumeSizeStr,
		FileSystem:                "ext4",
	}

	sourceVolume := getTestVolume()
	intermediateSnap := &api.Snapshot{Name: "snap-intermediate", FullName: "snap-full", State: api.SnapshotStateReady}
	createdClone := getTestVolume()

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), api.SnapshotTimeout).Return(intermediateSnap, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(intermediateSnap, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).Return(createdClone, nil).Times(1)
	// Volume wait fails with empty state (volume not found or API error), which returns the error
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return("", errFailed).Times(1)
	// Intermediate snapshot should be cleaned up on failure
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, intermediateSnap, gomock.Any()).Return(nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
}

func TestSANDriver_CreateClone_IntermediateSnapshotCleanupErrorLogged(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                      "test-clone",
		InternalName:              "test_clone",
		CloneSourceVolumeInternal: testVolumeName,
		CloneSourceSnapshot:       "", // No snapshot, triggers intermediate snapshot creation
		Size:                      testVolumeSizeStr,
		FileSystem:                "ext4",
	}

	sourceVolume := getTestVolume()
	intermediateSnap := &api.Snapshot{Name: "snap-intermediate", FullName: "snap-full", State: api.SnapshotStateReady}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, sourceVolConfig).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, sourceVolume, gomock.Any(), api.SnapshotTimeout).Return(intermediateSnap, nil).Times(1)
	mockAPI.EXPECT().SnapshotForVolume(ctx, sourceVolume, gomock.Any()).Return(intermediateSnap, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, gomock.Any()).Return(getTestVolume(), nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, gomock.Any(), api.VolumeStateReady, gomock.Any(), gomock.Any()).Return(api.VolumeStateReady, nil).Times(1)
	// DeleteSnapshot fails but clone should still succeed (error is logged, not returned)
	mockAPI.EXPECT().DeleteSnapshot(ctx, sourceVolume, intermediateSnap, gomock.Any()).Return(errFailed).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.NoError(t, err, "Clone should succeed even if intermediate snapshot cleanup fails")
}

func TestSANDriver_CreateClone_VolumeNameValidationError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "", // Invalid empty name
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
}

func TestSANDriver_CreateClone_CreationTokenValidationError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "", // Invalid empty internal name
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
}

func TestSANDriver_CreateClone_VolumeExistsCheckError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	sourceVolConfig := &storage.VolumeConfig{
		InternalName: testVolumeName,
	}

	cloneConfig := &storage.VolumeConfig{
		Name:                        "test-clone",
		InternalName:                "test_clone",
		CloneSourceVolumeInternal:   testVolumeName,
		CloneSourceSnapshotInternal: "test-snap",
		Size:                        testVolumeSizeStr,
	}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeExists(ctx, cloneConfig).Return(false, nil, errFailed).Times(1)

	err := driver.CreateClone(ctx, sourceVolConfig, cloneConfig, driver.pools["test-pool"])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking for existing volume")
}

// ============================================================================
// Helper method tests
// ============================================================================

func TestSANDriver_GetNodeSpecificHostGroupName(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.getNodeSpecificHostGroupName("test-node")
	expected := fmt.Sprintf("trident-node-test-node-%s", testUUID)

	assert.Equal(t, expected, result)
}

func TestSANDriver_GetNodeSpecificHostGroupName_HashesWhenTooLong(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Ensure we exceed the 63-char limit to take the hashing branch.
	longNode := strings.Repeat("a", 200)
	result := driver.getNodeSpecificHostGroupName(longNode)

	assert.LessOrEqual(t, len(result), 63)
	assert.True(t, strings.HasPrefix(result, "trident-node-t"))
	assert.False(t, strings.HasSuffix(result, "-"))
}

func TestSANDriver_GetNodeSpecificHostGroupName_NormalizesInvalidChars(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.getNodeSpecificHostGroupName("NODE__A@B")

	// Lowercase, underscores -> hyphens, invalid chars -> hyphens, no consecutive hyphens.
	assert.True(t, strings.HasPrefix(result, "trident-node-node-a-b-"))
	assert.False(t, strings.Contains(result, "--"))
	assert.False(t, strings.HasSuffix(result, "-"))
}

func TestSANDriver_GetNodeSpecificHostGroupName_NodeNameStartsWithDigit(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.getNodeSpecificHostGroupName("1node")

	// Still valid because the overall name starts with the fixed "trident-node-" prefix.
	assert.True(t, strings.HasPrefix(result, "trident-node-"))
	assert.False(t, strings.HasSuffix(result, "-"))
	assert.LessOrEqual(t, len(result), 63)
}

func TestSANDriver_GetNodeSpecificHostGroupName_Invariants(t *testing.T) {
	_, driver := newMockSANDriver(t)

	cases := []string{
		"",                       // empty node name
		"---",                    // only invalid-ish separators
		"1node",                  // starts with digit
		"NODE__A@B",              // invalid chars + underscores
		strings.Repeat("a", 200), // forces hash path
	}

	for _, nodeName := range cases {
		t.Run(nodeName, func(t *testing.T) {
			result := driver.getNodeSpecificHostGroupName(nodeName)

			// Must meet GCNV host group ID constraints.
			assert.NotEmpty(t, result)
			assert.LessOrEqual(t, len(result), 63)
			assert.True(t, result[0] >= 'a' && result[0] <= 'z')
			assert.False(t, strings.HasSuffix(result, "-"))

			// Only lowercase letters, digits, and hyphens.
			for _, r := range result {
				ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
				assert.True(t, ok, "unexpected char %q in %q", r, result)
			}
		})
	}
}

func TestSANDriver_ExtractNodeNameFromHostGroupName(t *testing.T) {
	_, driver := newMockSANDriver(t)

	hostGroupName := fmt.Sprintf("trident-node-test-node-%s", testUUID)
	result := driver.extractNodeNameFromHostGroupName(hostGroupName)

	assert.Equal(t, "test-node", result)

	// Test with full path
	fullPath := fmt.Sprintf("projects/test-project/locations/us-central1/hostGroups/trident-node-node-1-%s", testUUID)
	result2 := driver.extractNodeNameFromHostGroupName(fullPath)
	assert.Equal(t, "node-1", result2)
}

func TestSANDriver_ExtractNodeNameFromHostGroupName_Invalid(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.extractNodeNameFromHostGroupName("invalid-name")
	assert.Empty(t, result)
}

func TestSANDriver_ExtractNodeNameFromHostGroupName_UUIDMismatch(t *testing.T) {
	_, driver := newMockSANDriver(t)

	hostGroupName := "trident-node-test-node-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	assert.Empty(t, driver.extractNodeNameFromHostGroupName(hostGroupName))
}

func TestSANDriver_ExtractNodeNameFromHostGroupName_TooShort(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// No nodeName + hyphen before UUID, so after stripping prefix the remaining string length is <= uuidLen.
	hostGroupName := fmt.Sprintf("trident-node-%s", testUUID)
	assert.Empty(t, driver.extractNodeNameFromHostGroupName(hostGroupName))
}

func TestSANDriver_FixGCPLabelKey(t *testing.T) {
	_, driver := newMockSANDriver(t)

	// Empty is invalid.
	_, ok := driver.fixGCPLabelKey("")
	assert.False(t, ok)

	// Must start with a lowercase letter.
	_, ok = driver.fixGCPLabelKey("1abc")
	assert.False(t, ok)

	// Replace disallowed chars and truncate.
	key, ok := driver.fixGCPLabelKey("Trident.NetApp.IO/This-Is-A-Very-Long-Key-That-Should-Be-Truncated-Because-It-Is-Way-Too-Long")
	assert.True(t, ok)
	assert.LessOrEqual(t, len(key), api.MaxLabelLength)
	assert.True(t, strings.HasPrefix(key, "trident_netapp_io"))
}

// ============================================================================
// Additional driver interface tests
// ============================================================================

func TestSANDriver_Resize_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeName,
		Size:         testVolumeSizeStr,
	}

	newSizeBytes := uint64(214748364800) // 200 GiB
	volume := getTestVolume()

	// Mock expectations - expect +1 GiB over-provisioning for GCNV metadata overhead.
	// 200 GiB requested + 1 GiB overhead = 201 GiB = 215822106624 bytes
	expectedResizeBytes := int64(215822106624) // 201 GiB
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil)
	mockAPI.EXPECT().ResizeVolume(ctx, volume, expectedResizeBytes).Return(nil)

	err := driver.Resize(ctx, volConfig, newSizeBytes)
	assert.NoError(t, err, "Resize should succeed")
	// volConfig.Size should reflect actual GCNV allocation (201 GiB), not requested size
	assert.Equal(t, strconv.FormatUint(uint64(expectedResizeBytes), 10), volConfig.Size)
}

func TestSANDriver_Resize_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.Resize(ctx, &storage.VolumeConfig{InternalName: testVolumeName}, 1024)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_Resize_VolumeLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(nil, errFailed).Times(1)

	err := driver.Resize(ctx, volConfig, 1024)
	assert.Error(t, err)
}

func TestSANDriver_Resize_SizeTooLarge(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	// Request a size that exceeds GCNV's 128 TiB maximum (128 TiB = 140737488355328 bytes)
	// We need to request more than (128 TiB - 2 GiB) to trigger the max size error
	tooBig := uint64(140737488355328) // 128 TiB exactly, which after +1 GiB buffer exceeds max
	err := driver.Resize(ctx, volConfig, tooBig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds GCNV maximum")
}

func TestSANDriver_Resize_ResizeError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	newSizeBytes := uint64(214748364800) // 200 GiB
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName, Size: testVolumeSizeStr}

	// Mock expectations - expect +1 GiB over-provisioning for GCNV metadata overhead.
	// 200 GiB requested + 1 GiB overhead = 201 GiB = 215822106624 bytes
	expectedResizeBytes := int64(215822106624) // 201 GiB
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)
	mockAPI.EXPECT().ResizeVolume(ctx, volume, expectedResizeBytes).Return(errFailed).Times(1)

	err := driver.Resize(ctx, volConfig, newSizeBytes)
	assert.Error(t, err)
}

func TestSANDriver_Resize_VolumeNotReady(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volume.State = api.VolumeStateCreating // Not ready
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	err := driver.Resize(ctx, volConfig, uint64(214748364800))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "state is Creating, not Ready")
}

func TestSANDriver_Resize_ShrinkPrevented(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	volume.SizeBytes = int64(214748364800) // 200 GiB existing
	volConfig := &storage.VolumeConfig{InternalName: testVolumeName}

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	// Try to resize to smaller (100 GiB requested -> 101 GiB actual, which is less than 200 GiB)
	err := driver.Resize(ctx, volConfig, uint64(testVolumeSize))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is less than existing volume size")
}

func TestSANDriver_Resize_AlreadySameSize(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()
	// Requesting 99 GiB (106300440576 bytes):
	//   - ceil(99 GiB) = 99 GiB
	//   - + 1 GiB buffer = 100 GiB = 107374182400 bytes
	// Set volume to exact expected actual size
	requestSize := uint64(106300440576)   // ~99 GiB
	expectedActual := int64(107374182400) // 100 GiB (rounded up + 1 GiB buffer)
	volume.SizeBytes = expectedActual

	volConfig := &storage.VolumeConfig{InternalName: testVolumeName, Size: "106300440576"}
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, volConfig).Return(volume, nil).Times(1)

	err := driver.Resize(ctx, volConfig, requestSize)
	assert.NoError(t, err, "Resize should succeed when already at target size")
}

func TestSANDriver_ValidateVolumeName(t *testing.T) {
	_, driver := newMockSANDriver(t)

	tests := []struct {
		name  string
		valid bool
	}{
		{"valid", true},
		{"a", true},
		{"valid-name", true},
		{"valid123", true},
		{"a1b2c3", true},
		{"", false},               // Empty
		{"UPPERCASE", false},      // Must be lowercase
		{"has_underscore", false}, // Underscores not allowed in volume name
		{"123invalid", false},     // Must start with letter
		{"-invalid", false},       // Must start with letter
		{"has space", false},      // No spaces
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := driver.validateVolumeName(tt.name)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestSANDriver_ValidateCreationToken(t *testing.T) {
	_, driver := newMockSANDriver(t)

	tests := []struct {
		token string
		valid bool
	}{
		{"valid", true},
		{"a", true},
		{"valid_token", true}, // Underscores allowed in creation token
		{"valid123", true},
		{"a1b2c3", true},
		{"", false},           // Empty
		{"UPPERCASE", false},  // Must be lowercase
		{"has-hyphen", false}, // Hyphens not allowed in creation token
		{"123invalid", false}, // Must start with letter
		{"_invalid", false},   // Must start with letter
		{"has space", false},  // No spaces
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			err := driver.validateCreationToken(tt.token)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestSANDriver_Get_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volume := getTestVolume()

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, testVolumeName).Return(volume, nil)

	err := driver.Get(ctx, &storage.VolumeConfig{InternalName: testVolumeName})
	assert.NoError(t, err, "Get should succeed")
}

func TestSANDriver_Get_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.Get(ctx, &storage.VolumeConfig{InternalName: testVolumeName})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_Get_VolumeLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, testVolumeName).Return(nil, errFailed).Times(1)

	err := driver.Get(ctx, &storage.VolumeConfig{InternalName: testVolumeName})
	assert.Error(t, err)
}

func TestSANDriver_GetProtocol(t *testing.T) {
	_, driver := newMockSANDriver(t)

	result := driver.GetProtocol(ctx)
	assert.Equal(t, tridentconfig.Block, result)
}

func TestSANDriver_GetInternalVolumeName(t *testing.T) {
	_, driver := newMockSANDriver(t)

	pool := storage.NewStoragePool(nil, "test-pool")

	orig := tridentconfig.UsingPassthroughStore
	defer func() { tridentconfig.UsingPassthroughStore = orig }()

	t.Run("passthrough_store", func(t *testing.T) {
		tridentconfig.UsingPassthroughStore = true
		volConfig := &storage.VolumeConfig{Name: "myVol"}
		result := driver.GetInternalVolumeName(ctx, volConfig, pool)
		assert.Equal(t, *driver.Config.StoragePrefix+"myVol", result)
	})

	t.Run("csi_name_hyphens_replaced", func(t *testing.T) {
		tridentconfig.UsingPassthroughStore = false
		// CSI names match pvc-<uuid> pattern, hyphens replaced with underscores for GCP compliance
		csiName := "pvc-f668b586-a327-49e8-8d2c-b2748343024a"
		expectedName := "pvc_f668b586_a327_49e8_8d2c_b2748343024a"
		volConfig := &storage.VolumeConfig{Name: csiName}
		result := driver.GetInternalVolumeName(ctx, volConfig, pool)
		assert.Equal(t, expectedName, result)
		assert.NotContains(t, result, "-", "Result should not contain hyphens")
	})

	t.Run("non_csi_name_generates_uuid", func(t *testing.T) {
		tridentconfig.UsingPassthroughStore = false
		// Non-CSI names get a generated gcnv_<uuid> name with underscores
		volConfig := &storage.VolumeConfig{Name: "my-custom-volume"}
		result := driver.GetInternalVolumeName(ctx, volConfig, pool)
		assert.True(t, strings.HasPrefix(result, "gcnv_"), "Non-CSI names should get gcnv_ prefix")
		assert.Len(t, result, 5+36, "Result should be gcnv_ (5) + uuid with underscores (36)")
		assert.NotContains(t, result, "-", "Result should not contain hyphens")
	})
}

func TestSANDriver_Import_Success(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: false, // Managed import - will update labels
	}

	volume := getTestVolume()
	// Import requires the volume to have BlockDevices to be considered an iSCSI volume
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	mockAPI.EXPECT().UpdateSANVolume(ctx, volume, gomock.Any()).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return(api.VolumeStateReady, nil)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.NoError(t, err, "Import should succeed")
	assert.Equal(t, testVolumeSizeStr, volConfig.Size)
	assert.Equal(t, volume.FullName, volConfig.InternalID, "InternalID should be set to the full GCP ID")
}

func TestSANDriver_Import_RefreshError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(errFailed).Times(1)

	err := driver.Import(ctx, &storage.VolumeConfig{InternalName: "new-name"}, "original-name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not refresh GCNV resources")
}

func TestSANDriver_Import_VolumeLookupError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(nil, errFailed).Times(1)

	err := driver.Import(ctx, &storage.VolumeConfig{InternalName: "new-name"}, "original-name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find volume to import")
}

func TestSANDriver_Import_NotBlockVolume(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName: "new-name",
	}

	volume := getTestVolume()
	volume.ProtocolTypes = []string{"NFSv3"} // Not a block volume (no ISCSI protocol)

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.Error(t, err, "Import should fail for non-block volume")
	assert.Contains(t, err.Error(), "not an iSCSI volume")
}

func TestSANDriver_Import_ISCSIProtocolWithoutBlockDevices(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: false, // Managed import - will update labels
	}

	volume := getTestVolume()
	volume.BlockDevices = nil
	volume.ProtocolTypes = []string{api.ProtocolTypeISCSI}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	mockAPI.EXPECT().UpdateSANVolume(ctx, volume, gomock.Any()).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return(api.VolumeStateReady, nil)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.NoError(t, err, "Import should succeed for iSCSI volume indicated by protocol type")
	assert.Equal(t, testVolumeSizeStr, volConfig.Size)
	assert.Equal(t, volume.FullName, volConfig.InternalID, "InternalID should be set to the full GCP ID")
}

func TestSANDriver_Import_WithTelemetryLabels_ManagedVolume(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: false, // Managed import - should update labels
	}

	volume := getTestVolume()
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}
	// Start with some existing labels
	volume.Labels = map[string]string{
		"existing_label": "existing_value",
	}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	mockAPI.EXPECT().UpdateSANVolume(ctx, volume, gomock.Any()).DoAndReturn(
		func(ctx context.Context, vol *api.Volume, req *api.VolumeUpdateRequest) (*api.Volume, error) {
			// Verify telemetry labels are present
			assert.Contains(t, req.Labels, "version", "version label should be present")
			assert.Contains(t, req.Labels, "backend_uuid", "backend_uuid label should be present")
			assert.Contains(t, req.Labels, "platform", "platform label should be present")
			assert.Contains(t, req.Labels, "platform_version", "platform_version label should be present")
			assert.Contains(t, req.Labels, "plugin", "plugin label should be present")
			// Verify existing labels are preserved
			assert.Contains(t, req.Labels, "existing_label", "existing labels should be preserved")
			assert.Equal(t, "existing_value", req.Labels["existing_label"])
			// Verify field mask includes labels
			assert.Contains(t, req.FieldMask, "labels", "field mask should include labels")
			return vol, nil
		},
	).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return(api.VolumeStateReady, nil)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.NoError(t, err, "Import should succeed with label update")
	assert.Equal(t, testVolumeSizeStr, volConfig.Size)
	assert.Equal(t, volume.FullName, volConfig.InternalID, "InternalID should be set to the full GCP ID")
}

func TestSANDriver_Import_WithTelemetryLabels_UnmanagedVolume(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: true, // Unmanaged import - should NOT update labels
	}

	volume := getTestVolume()
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	// UpdateVolume should NOT be called for unmanaged imports
	mockAPI.EXPECT().UpdateSANVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.NoError(t, err, "Import should succeed without label update for unmanaged volume")
	assert.Equal(t, testVolumeSizeStr, volConfig.Size)
	assert.Equal(t, volume.FullName, volConfig.InternalID, "InternalID should be set to the full GCP ID")
}

func TestSANDriver_Import_LabelUpdateFailure(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: false, // Managed import - should update labels
	}

	volume := getTestVolume()
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	mockAPI.EXPECT().UpdateSANVolume(ctx, volume, gomock.Any()).Return(nil, errFailed).Times(1)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.Error(t, err, "Import should fail when label update fails")
	assert.Contains(t, err.Error(), "volume label update failed")
}

func TestSANDriver_Import_WithTelemetryLabels_NilLabels(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: false, // Managed import - should update labels
	}

	volume := getTestVolume()
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}
	volume.Labels = nil // Volume has no labels initially

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	mockAPI.EXPECT().UpdateSANVolume(ctx, volume, gomock.Any()).DoAndReturn(
		func(ctx context.Context, vol *api.Volume, req *api.VolumeUpdateRequest) (*api.Volume, error) {
			// Verify telemetry labels are present even when starting with nil labels
			assert.Contains(t, req.Labels, "version", "version label should be present")
			assert.Contains(t, req.Labels, "backend_uuid", "backend_uuid label should be present")
			assert.Contains(t, req.Labels, "platform", "platform label should be present")
			assert.Contains(t, req.Labels, "platform_version", "platform_version label should be present")
			assert.Contains(t, req.Labels, "plugin", "plugin label should be present")
			return vol, nil
		},
	).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return(api.VolumeStateReady, nil)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.NoError(t, err, "Import should succeed and create labels when volume has nil labels")
	assert.Equal(t, testVolumeSizeStr, volConfig.Size)
	assert.Equal(t, volume.FullName, volConfig.InternalID, "InternalID should be set to the full GCP ID")
}

func TestSANDriver_Import_VolumeNotReady(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName: "new-name",
	}

	volume := getTestVolume()
	volume.State = api.VolumeStateCreating
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.Error(t, err, "Import should fail for volume not in ready state")
	assert.Contains(t, err.Error(), "is not available")
}

func TestSANDriver_Import_EnsureCapacityPoolError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName: "new-name",
	}

	volume := getTestVolume()
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(errFailed)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.Error(t, err, "Import should fail when capacity pool validation fails")
}

func TestSANDriver_Import_WaitForStateError(t *testing.T) {
	mockAPI, driver := newMockSANDriver(t)

	volConfig := &storage.VolumeConfig{
		InternalName:     "new-name",
		ImportNotManaged: false,
	}

	volume := getTestVolume()
	volume.BlockDevices = []api.BlockDevice{{Name: "block0"}}

	// Mock expectations
	mockAPI.EXPECT().RefreshGCNVResources(ctx).Return(nil).Times(1)
	mockAPI.EXPECT().VolumeByName(ctx, "original-name").Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(ctx, volume).Return(nil)
	mockAPI.EXPECT().UpdateSANVolume(ctx, volume, gomock.Any()).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeState(ctx, volume, api.VolumeStateReady,
		[]string{api.VolumeStateError}, api.DefaultTimeout).Return("", errFailed)

	err := driver.Import(ctx, volConfig, "original-name")
	assert.Error(t, err, "Import should fail when wait for state fails")
}

func TestSANDriver_CanSnapshot(t *testing.T) {
	_, driver := newMockSANDriver(t)

	err := driver.CanSnapshot(ctx, nil, nil)
	assert.NoError(t, err)
}

func TestCalculateActualSizeBytes(t *testing.T) {
	GiB := api.GiBBytes // 1073741824

	tests := []struct {
		name           string
		requestedBytes int64
		expectedBytes  int64
		expectError    bool
		errorContains  string
	}{
		{
			name:           "Exact 1 GiB rounds up and adds 1 GiB buffer",
			requestedBytes: 1 * GiB,
			expectedBytes:  2 * GiB, // 1 GiB (already whole) + 1 GiB buffer = 2 GiB
			expectError:    false,
		},
		{
			name:           "Exact 100 GiB rounds up and adds 1 GiB buffer",
			requestedBytes: 100 * GiB,
			expectedBytes:  101 * GiB, // 100 GiB (already whole) + 1 GiB buffer = 101 GiB
			expectError:    false,
		},
		{
			name:           "Fractional 2.5 GiB rounds up to 3 GiB then adds 1 GiB buffer",
			requestedBytes: 2*GiB + GiB/2, // 2.5 GiB
			expectedBytes:  4 * GiB,       // rounds up to 3 GiB + 1 GiB buffer = 4 GiB
			expectError:    false,
		},
		{
			name:           "1 byte over 2 GiB rounds up to 3 GiB then adds 1 GiB buffer",
			requestedBytes: 2*GiB + 1,
			expectedBytes:  4 * GiB, // rounds up to 3 GiB + 1 GiB buffer = 4 GiB
			expectError:    false,
		},
		{
			name:           "Very small size (1 byte) rounds up to 1 GiB then adds 1 GiB buffer",
			requestedBytes: 1,
			expectedBytes:  2 * GiB, // rounds up to 1 GiB + 1 GiB buffer = 2 GiB
			expectError:    false,
		},
		{
			name:           "Size exceeding GCNV 128 TiB limit returns error",
			requestedBytes: MaxGCNVVolumeSizeBytes, // 128 TiB exactly - would need +1-2 GiB, exceeds limit
			expectError:    true,
			errorContains:  "exceeds GCNV maximum",
		},
		{
			name:           "Size just under the safe limit succeeds",
			requestedBytes: MaxGCNVVolumeSizeBytes - 2*GiB, // Maximum safe request (128 TiB - 2 GiB = 131070 GiB)
			expectedBytes:  (131070 + 1) * GiB,             // 131070 GiB rounds to itself + 1 GiB buffer = 131071 GiB
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualBytes, err := calculateActualSizeBytes(tt.requestedBytes)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBytes, actualBytes,
					"Expected %d bytes (%d GiB), got %d bytes (%d GiB)",
					tt.expectedBytes, tt.expectedBytes/GiB, actualBytes, actualBytes/GiB)
			}
		})
	}
}

func TestMaxGCNVVolumeSizeBytes_Constant(t *testing.T) {
	// Verify the constant is correct: 128 TiB = 128 * 1024 * 1 GiB
	expected := int64(128) * 1024 * api.GiBBytes
	assert.Equal(t, expected, MaxGCNVVolumeSizeBytes, "MaxGCNVVolumeSizeBytes should be 128 TiB")

	// Verify in human-readable terms
	TiB := int64(1024) * api.GiBBytes
	assert.Equal(t, int64(128)*TiB, MaxGCNVVolumeSizeBytes, "MaxGCNVVolumeSizeBytes should be 128 TiB")
}

func TestSANStorageDriver_String(t *testing.T) {
	// Create a driver with sensitive credentials
	driver := &SANStorageDriver{
		Config: drivers.GCNVStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "google-cloud-netapp-volumes-san",
			},
			ProjectNumber: "123456789",
			Location:      "us-east4",
			APIKey: drivers.GCPPrivateKey{
				Type:         "service_account",
				ProjectID:    "my-project",
				PrivateKeyID: "key-id-12345",
				PrivateKey:   "-----BEGIN RSA PRIVATE KEY-----\nSECRET\n-----END RSA PRIVATE KEY-----",
				ClientEmail:  "test@my-project.iam.gserviceaccount.com",
				ClientID:     "123456789",
			},
		},
	}

	// Call String() - should return redacted output
	result := driver.String()

	// Verify sensitive data is NOT in the output
	assert.NotContains(t, result, "SECRET", "Private key should be redacted")
	assert.NotContains(t, result, "-----BEGIN RSA PRIVATE KEY-----", "Private key should be redacted")
	assert.NotContains(t, result, "key-id-12345", "Private key ID should be redacted")

	// Verify REDACTED markers are present
	assert.Contains(t, result, "REDACTED", "Output should contain REDACTED markers")

	// Verify non-sensitive data may still be present (driver name, location, etc.)
	// The output format depends on convert.ToStringRedacted implementation
	assert.NotEmpty(t, result, "String() should return non-empty output")
}

func TestSANStorageDriver_GoString(t *testing.T) {
	// Create a driver with sensitive credentials
	driver := &SANStorageDriver{
		Config: drivers.GCNVStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "google-cloud-netapp-volumes-san",
			},
			ProjectNumber: "123456789",
			Location:      "us-east4",
			APIKey: drivers.GCPPrivateKey{
				Type:         "service_account",
				ProjectID:    "my-project",
				PrivateKeyID: "key-id-67890",
				PrivateKey:   "-----BEGIN RSA PRIVATE KEY-----\nTOPSECRET\n-----END RSA PRIVATE KEY-----",
				ClientEmail:  "test@my-project.iam.gserviceaccount.com",
				ClientID:     "987654321",
			},
		},
	}

	// Call GoString() - should return same as String()
	goStringResult := driver.GoString()
	stringResult := driver.String()

	// GoString should return the same as String
	assert.Equal(t, stringResult, goStringResult, "GoString() should return the same as String()")

	// Verify sensitive data is NOT in the output
	assert.NotContains(t, goStringResult, "TOPSECRET", "Private key should be redacted")
	assert.NotContains(t, goStringResult, "key-id-67890", "Private key ID should be redacted")
}

func TestSANStorageDriver_String_WithNilConfig(t *testing.T) {
	// Create a minimal driver
	driver := &SANStorageDriver{
		Config: drivers.GCNVStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "google-cloud-netapp-volumes-san",
			},
		},
	}

	// Should not panic with minimal config
	assert.NotPanics(t, func() {
		result := driver.String()
		assert.NotEmpty(t, result)
	})

	assert.NotPanics(t, func() {
		result := driver.GoString()
		assert.NotEmpty(t, result)
	})
}

func TestSANStorageDriver_Create_NilPool(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockGCNV(mockCtrl)

	driver := &SANStorageDriver{
		initialized: true,
		Config: drivers.GCNVStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "google-cloud-netapp-volumes-san",
			},
		},
		API:   mockAPI,
		pools: make(map[string]storage.Pool),
	}

	volConfig := &storage.VolumeConfig{
		Name:         "test-volume",
		InternalName: "test_volume",
		Size:         "1073741824",
	}

	// Mock API call that happens before pool validation
	mockAPI.EXPECT().RefreshGCNVResources(gomock.Any()).Return(nil)

	// Call Create with nil pool
	err := driver.Create(context.Background(), volConfig, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pool not specified")
}

func TestSANStorageDriver_Create_PoolNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockGCNV(mockCtrl)

	driver := &SANStorageDriver{
		initialized: true,
		Config: drivers.GCNVStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "google-cloud-netapp-volumes-san",
			},
		},
		API:   mockAPI,
		pools: make(map[string]storage.Pool), // Empty pools map
	}

	volConfig := &storage.VolumeConfig{
		Name:         "test-volume",
		InternalName: "test_volume",
		Size:         "1073741824",
	}

	// Create a mock pool that is NOT in driver.pools
	unknownPool := storage.NewStoragePool(nil, "unknown_pool")

	// Mock API call that happens before pool validation
	mockAPI.EXPECT().RefreshGCNVResources(gomock.Any()).Return(nil)

	// Call Create with pool not in driver.pools
	err := driver.Create(context.Background(), volConfig, unknownPool, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pool unknown_pool does not exist")
}

func TestSANStorageDriver_Import_PreservesInternalName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAPI := mockapi.NewMockGCNV(mockCtrl)

	driver := &SANStorageDriver{
		initialized: true,
		Config: drivers.GCNVStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "google-cloud-netapp-volumes-san",
			},
		},
		API: mockAPI,
	}

	originalName := "existing-volume"
	volume := &api.Volume{
		Name:          originalName,
		FullName:      "projects/test-project/locations/us-central1/volumes/existing-volume",
		CreationToken: originalName,
		SizeBytes:     testVolumeSize,
		CapacityPool:  "test-pool",
		State:         api.VolumeStateReady,
		ProtocolTypes: []string{api.ProtocolTypeISCSI},
	}

	volConfig := &storage.VolumeConfig{
		Name:             "new-volume",
		InternalName:     "new_volume",
		ImportNotManaged: true,
	}

	mockAPI.EXPECT().RefreshGCNVResources(gomock.Any()).Return(nil)
	mockAPI.EXPECT().VolumeByName(gomock.Any(), originalName).Return(volume, nil)
	mockAPI.EXPECT().EnsureVolumeInValidCapacityPool(gomock.Any(), volume).Return(nil)

	err := driver.Import(context.Background(), volConfig, originalName)

	assert.NoError(t, err)
	assert.Equal(t, originalName, volConfig.InternalName, "internal name should match original volume name")
	assert.Equal(t, volume.FullName, volConfig.InternalID, "internal ID should be set to full volume name")
	assert.Equal(t, strconv.FormatUint(uint64(volume.SizeBytes), 10), volConfig.Size)
}
