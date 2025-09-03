// Copyright 2025 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
	testutils "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils/models"
)

const (
	testPoolName              = "pool-0"
	testBackendName           = "fake_backend"
	testVolumeName            = "test_volume"
	testVolumeInternalName    = "test_volume_internal"
	testVolumeSize            = "1Gi"
	testTxnVolumeName         = "test_txn_volume"
	testTxnVolumeInternalName = "test_txn_volume_internal"
	testStorageClassName      = "test_sc"
	testNodeName              = "test_node"
	testNodeIQN               = "iqn.test"
	testNodeIP1               = "192.168.1.1"
	testNodeIP2               = "192.168.1.2"
	testVolumePublicationName = "test_volume_pub"
	testSnapshotName          = "test_snapshot"
	testSnapshotInternalName  = "test_snapshot_internal"
	testGroupSnapshotName     = "test_group_snapshot"
	testVolume1Name           = "test_volume1"
	testVolume2Name           = "test_volume2"
	testSnapshot1ID           = "snapshot1"
	testSnapshot2ID           = "snapshot2"
	testProvisioningType      = "thin"
	memoryStoreVersion        = "memory"
	alreadyExistsError        = "already exists"
	nonexistentName           = "nonexistent"
	testVersionString         = "test_version"
	testAPIVersionString      = "test_api"
	anySecretName             = "any_secret_name"
	updatedIQN                = "updated_iqn"
)

// Helper function to create a context
func testCtx() context.Context {
	return context.Background()
}

// Test helper functions
func getTestBackend() *storage.StorageBackend {
	fakeConfig := drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           drivers.ConfigVersion,
			StorageDriverName: config.FakeStorageDriverName,
		},
		Protocol:     config.File,
		Pools:        testutils.GenerateFakePools(2),
		InstanceName: testBackendName,
	}

	fakeStorageDriver := fakedriver.NewFakeStorageDriver(testCtx(), fakeConfig)
	fakeBackend, _ := storage.NewStorageBackend(testCtx(), fakeStorageDriver)
	return fakeBackend
}

func getTestVolume(backend *storage.StorageBackend) *storage.Volume {
	volumeConfig := &storage.VolumeConfig{
		Name:         testVolumeName,
		InternalName: testVolumeInternalName,
		Size:         testVolumeSize,
	}

	p, _ := backend.StoragePools().Load(testPoolName)
	return storage.NewVolume(volumeConfig, backend.BackendUUID(), p.(storage.Pool).Name(), false,
		storage.VolumeStateOnline)
}

func getTestStorageClass() *storageclass.StorageClass {
	scConfig := &storageclass.Config{
		Version: config.OrchestratorAPIVersion,
		Name:    testStorageClassName,
		Attributes: map[string]storageattribute.Request{
			storageattribute.IOPS:             storageattribute.NewIntRequest(40),
			storageattribute.Snapshots:        storageattribute.NewBoolRequest(true),
			storageattribute.ProvisioningType: storageattribute.NewStringRequest(testProvisioningType),
		},
	}
	return storageclass.New(scConfig)
}

func getTestVolumeTransaction(op storage.VolumeOperation) *storage.VolumeTransaction {
	volumeConfig := &storage.VolumeConfig{
		Name:         testTxnVolumeName,
		InternalName: testTxnVolumeInternalName,
	}

	return &storage.VolumeTransaction{
		Config: volumeConfig,
		Op:     op,
	}
}

func getTestNode() *models.Node {
	return &models.Node{
		Name:    testNodeName,
		IQN:     testNodeIQN,
		IPs:     []string{testNodeIP1, testNodeIP2},
		Deleted: false,
	}
}

func getTestVolumePublication() *models.VolumePublication {
	return &models.VolumePublication{
		Name:       testVolumePublicationName,
		NodeName:   testNodeName,
		VolumeName: testVolumeName,
		ReadOnly:   false,
		AccessMode: 1,
	}
}

func getTestSnapshot() *storage.Snapshot {
	snapConfig := &storage.SnapshotConfig{
		Version:            config.OrchestratorAPIVersion,
		Name:               testSnapshotName,
		InternalName:       testSnapshotInternalName,
		VolumeName:         testVolumeName,
		VolumeInternalName: testVolumeInternalName,
	}
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   time.Now().UTC().Format(time.RFC3339),
		SizeBytes: 1000000000,
		State:     storage.SnapshotStateOnline,
	}
}

func getTestGroupSnapshot() *storage.GroupSnapshot {
	return &storage.GroupSnapshot{
		GroupSnapshotConfig: &storage.GroupSnapshotConfig{
			Name:        testGroupSnapshotName,
			VolumeNames: []string{testVolume1Name, testVolume2Name},
		},
		SnapshotIDs: []string{testSnapshot1ID, testSnapshot2ID},
		Created:     time.Now().UTC().Format(time.RFC3339),
	}
}

// Test client initialization and basic operations
func TestInMemoryClient_BasicOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "NewInMemoryClient",
			assertion: func(t *testing.T, client *InMemoryClient) {
				assert.NotNil(t, client)
				assert.NotNil(t, client.backends)
				assert.NotNil(t, client.volumes)
				assert.NotNil(t, client.storageClasses)
				assert.NotNil(t, client.volumeTxns)
				assert.NotNil(t, client.volumePublications)
				assert.NotNil(t, client.nodes)
				assert.NotNil(t, client.snapshots)
				assert.NotNil(t, client.version)
				assert.NotEmpty(t, client.uuid)
				assert.Equal(t, memoryStoreVersion, client.version.PersistentStoreVersion)
				assert.Equal(t, config.OrchestratorAPIVersion, client.version.OrchestratorAPIVersion)
			},
		},
		{
			name: "GetType",
			assertion: func(t *testing.T, client *InMemoryClient) {
				assert.Equal(t, MemoryStore, client.GetType())
			},
		},
		{
			name: "GetConfig",
			assertion: func(t *testing.T, client *InMemoryClient) {
				config := client.GetConfig()
				assert.NotNil(t, config)
			},
		},
		{
			name: "GetTridentUUID",
			assertion: func(t *testing.T, client *InMemoryClient) {
				uuid, err := client.GetTridentUUID(testCtx())
				assert.NoError(t, err)
				assert.NotEmpty(t, uuid)
				assert.Equal(t, client.uuid, uuid)
			},
		},
		{
			name: "GetVersion",
			assertion: func(t *testing.T, client *InMemoryClient) {
				version, err := client.GetVersion(testCtx())
				assert.NoError(t, err)
				assert.NotNil(t, version)
				assert.Equal(t, memoryStoreVersion, version.PersistentStoreVersion)
				assert.Equal(t, config.OrchestratorAPIVersion, version.OrchestratorAPIVersion)
			},
		},
		{
			name: "SetVersion",
			assertion: func(t *testing.T, client *InMemoryClient) {
				newVersion := &config.PersistentStateVersion{
					PersistentStoreVersion: testVersionString,
					OrchestratorAPIVersion: testAPIVersionString,
				}
				err := client.SetVersion(testCtx(), newVersion)
				assert.NoError(t, err)
				// InMemoryClient doesn't actually update version in SetVersion - it's a no-op
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

func TestInMemoryClient_Stop(t *testing.T) {
	client := NewInMemoryClient()
	backend := getTestBackend()

	// Add some data
	err := client.AddBackend(testCtx(), backend)
	require.NoError(t, err)

	// Stop should clear all data
	err = client.Stop()
	assert.NoError(t, err)

	// Verify data is cleared
	assert.Empty(t, client.backends)
	assert.Equal(t, 0, client.backendsAdded)
	assert.Empty(t, client.volumes)
	assert.Equal(t, 0, client.volumesAdded)
	assert.Empty(t, client.storageClasses)
	assert.Equal(t, 0, client.storageClassesAdded)
	assert.Empty(t, client.volumeTxns)
	assert.Equal(t, 0, client.volumeTxnsAdded)
	assert.Empty(t, client.volumePublications)
	assert.Equal(t, 0, client.volumePublicationsAdded)
	assert.Empty(t, client.nodes)
	assert.Equal(t, 0, client.nodesAdded)
	assert.Empty(t, client.snapshots)
	assert.Equal(t, 0, client.snapshotsAdded)
}

// Backend tests
func TestInMemoryClient_BackendOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddBackend_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				err := client.AddBackend(testCtx(), backend)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.backendsAdded)
				assert.Len(t, client.backends, 1)
				assert.Contains(t, client.backends, backend.Name())
			},
		},
		{
			name: "AddBackend_Duplicate",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				// Add backend first time
				err := client.AddBackend(testCtx(), backend)
				require.NoError(t, err)
				// Try to add same backend again
				err = client.AddBackend(testCtx(), backend)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), alreadyExistsError)
				assert.Equal(t, 1, client.backendsAdded) // Should not increment
			},
		},
		{
			name: "GetBackend_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				err := client.AddBackend(testCtx(), backend)
				require.NoError(t, err)
				retrieved, err := client.GetBackend(testCtx(), backend.Name())
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, backend.Name(), retrieved.Name)
			},
		},
		{
			name: "GetBackend_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetBackend(testCtx(), nonexistentName)
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetBackendSecret",
			assertion: func(t *testing.T, client *InMemoryClient) {
				secret, err := client.GetBackendSecret(testCtx(), anySecretName)
				assert.NoError(t, err)
				assert.Nil(t, secret)
			},
		},
		{
			name: "UpdateBackend_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				// Add backend
				err := client.AddBackend(testCtx(), backend)
				require.NoError(t, err)

				// Create a slightly different backend for update
				updatedBackend := getTestBackend()
				updatedBackend.SetState(storage.Online)

				// Update backend
				err = client.UpdateBackend(testCtx(), updatedBackend)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateBackend_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				err := client.UpdateBackend(testCtx(), backend)
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteBackend",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				// Add backend
				err := client.AddBackend(testCtx(), backend)
				require.NoError(t, err)
				// Delete backend
				err = client.DeleteBackend(testCtx(), backend)
				assert.NoError(t, err)
				assert.NotContains(t, client.backends, backend.Name())
			},
		},
		{
			name: "IsBackendDeleting",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				// InMemoryClient always returns false for IsBackendDeleting
				isDeleting := client.IsBackendDeleting(testCtx(), backend)
				assert.False(t, isDeleting)
			},
		},
		{
			name: "ReplaceBackendAndUpdateVolumes",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend1 := getTestBackend()
				backend2 := getTestBackend()
				err := client.ReplaceBackendAndUpdateVolumes(testCtx(), backend1, backend2)
				assert.Error(t, err)
				// Should return NotSupported error
			},
		},
		{
			name: "GetBackends_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backends, err := client.GetBackends(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, backends)
			},
		},
		{
			name: "GetBackends_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				err := client.AddBackend(testCtx(), backend)
				require.NoError(t, err)
				backends, err := client.GetBackends(testCtx())
				assert.NoError(t, err)
				assert.Len(t, backends, 1)
				assert.Equal(t, backend.Name(), backends[0].Name)
			},
		},
		{
			name: "DeleteBackends_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				err := client.DeleteBackends(testCtx())
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteBackends_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				err := client.AddBackend(testCtx(), backend)
				require.NoError(t, err)
				err = client.DeleteBackends(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, client.backends)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Volume tests
func TestInMemoryClient_VolumeOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddVolume_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.volumesAdded)
				assert.Len(t, client.volumes, 1)
				assert.Contains(t, client.volumes, volume.Config.Name)
			},
		},
		{
			name: "AddVolume_Duplicate",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				require.NoError(t, err)
				err = client.AddVolume(testCtx(), volume)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "already exists")
			},
		},
		{
			name: "GetVolume_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				require.NoError(t, err)
				retrieved, err := client.GetVolume(testCtx(), volume.Config.Name)
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, volume.Config.Name, retrieved.Config.Name)
			},
		},
		{
			name: "GetVolume_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetVolume(testCtx(), "nonexistent")
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "UpdateVolume_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				require.NoError(t, err)

				// Create a slightly different volume for update
				updatedVolume := getTestVolume(backend)
				updatedVolume.State = storage.VolumeStateDeleting

				err = client.UpdateVolume(testCtx(), updatedVolume)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateVolume_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.UpdateVolume(testCtx(), volume)
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteVolume",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				require.NoError(t, err)
				err = client.DeleteVolume(testCtx(), volume)
				assert.NoError(t, err)
				assert.NotContains(t, client.volumes, volume.Config.Name)
			},
		},
		{
			name: "GetVolumes_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				volumes, err := client.GetVolumes(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, volumes)
			},
		},
		{
			name: "GetVolumes_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				require.NoError(t, err)
				volumes, err := client.GetVolumes(testCtx())
				assert.NoError(t, err)
				assert.Len(t, volumes, 1)
				assert.Equal(t, volume.Config.Name, volumes[0].Config.Name)
			},
		},
		{
			name: "DeleteVolumes_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				err := client.DeleteVolumes(testCtx())
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteVolumes_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				backend := getTestBackend()
				volume := getTestVolume(backend)
				err := client.AddVolume(testCtx(), volume)
				require.NoError(t, err)
				err = client.DeleteVolumes(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, client.volumes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Volume Transaction tests
func TestInMemoryClient_VolumeTransactionOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddVolumeTransaction",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				err := client.AddVolumeTransaction(testCtx(), txn)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.volumeTxnsAdded)
				assert.Len(t, client.volumeTxns, 1)
				assert.Contains(t, client.volumeTxns, txn.Name())
			},
		},
		{
			name: "AddVolumeTransaction_Overwrite",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				// Add transaction
				err := client.AddVolumeTransaction(testCtx(), txn)
				require.NoError(t, err)
				// Add same transaction again (should overwrite)
				err = client.AddVolumeTransaction(testCtx(), txn)
				assert.NoError(t, err)
				assert.Equal(t, 2, client.volumeTxnsAdded) // Counter increments
				assert.Len(t, client.volumeTxns, 1)        // But map size stays same
			},
		},
		{
			name: "GetVolumeTransactions_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txns, err := client.GetVolumeTransactions(testCtx())
				assert.Error(t, err)
				assert.Nil(t, txns)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetVolumeTransactions_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				err := client.AddVolumeTransaction(testCtx(), txn)
				require.NoError(t, err)
				txns, err := client.GetVolumeTransactions(testCtx())
				assert.NoError(t, err)
				assert.Len(t, txns, 1)
				assert.Equal(t, txn.Name(), txns[0].Name())
			},
		},
		{
			name: "UpdateVolumeTransaction",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				// UpdateVolumeTransaction doesn't check if transaction exists
				err := client.UpdateVolumeTransaction(testCtx(), txn)
				assert.NoError(t, err)
				assert.Contains(t, client.volumeTxns, txn.Name())
			},
		},
		{
			name: "GetVolumeTransaction_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				err := client.AddVolumeTransaction(testCtx(), txn)
				require.NoError(t, err)
				retrieved, err := client.GetVolumeTransaction(testCtx(), txn)
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, txn.Name(), retrieved.Name())
			},
		},
		{
			name: "GetVolumeTransaction_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				retrieved, err := client.GetVolumeTransaction(testCtx(), txn)
				assert.NoError(t, err)
				assert.Nil(t, retrieved)
			},
		},
		{
			name: "DeleteVolumeTransaction",
			assertion: func(t *testing.T, client *InMemoryClient) {
				txn := getTestVolumeTransaction(storage.AddVolume)
				err := client.AddVolumeTransaction(testCtx(), txn)
				require.NoError(t, err)
				err = client.DeleteVolumeTransaction(testCtx(), txn)
				assert.NoError(t, err)
				assert.NotContains(t, client.volumeTxns, txn.Name())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Storage Class tests
func TestInMemoryClient_StorageClassOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddStorageClass_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.AddStorageClass(testCtx(), sc)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.storageClassesAdded)
				assert.Len(t, client.storageClasses, 1)
				assert.Contains(t, client.storageClasses, sc.GetName())
			},
		},
		{
			name: "AddStorageClass_Duplicate",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.AddStorageClass(testCtx(), sc)
				require.NoError(t, err)
				err = client.AddStorageClass(testCtx(), sc)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "already exists")
			},
		},
		{
			name: "UpdateStorageClass_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.AddStorageClass(testCtx(), sc)
				require.NoError(t, err)
				err = client.UpdateStorageClass(testCtx(), sc)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateStorageClass_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.UpdateStorageClass(testCtx(), sc)
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetStorageClass_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.AddStorageClass(testCtx(), sc)
				require.NoError(t, err)
				retrieved, err := client.GetStorageClass(testCtx(), sc.GetName())
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, sc.GetName(), retrieved.GetName())
			},
		},
		{
			name: "GetStorageClass_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetStorageClass(testCtx(), "nonexistent")
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetStorageClasses_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				classes, err := client.GetStorageClasses(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, classes)
			},
		},
		{
			name: "GetStorageClasses_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.AddStorageClass(testCtx(), sc)
				require.NoError(t, err)
				classes, err := client.GetStorageClasses(testCtx())
				assert.NoError(t, err)
				assert.Len(t, classes, 1)
				assert.Equal(t, sc.GetName(), classes[0].GetName())
			},
		},
		{
			name: "DeleteStorageClass",
			assertion: func(t *testing.T, client *InMemoryClient) {
				sc := getTestStorageClass()
				err := client.AddStorageClass(testCtx(), sc)
				require.NoError(t, err)
				err = client.DeleteStorageClass(testCtx(), sc)
				assert.NoError(t, err)
				assert.NotContains(t, client.storageClasses, sc.GetName())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Node tests
func TestInMemoryClient_NodeOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddOrUpdateNode_Add",
			assertion: func(t *testing.T, client *InMemoryClient) {
				node := getTestNode()
				err := client.AddOrUpdateNode(testCtx(), node)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.nodesAdded)
				assert.Len(t, client.nodes, 1)
				assert.Contains(t, client.nodes, node.Name)
			},
		},
		{
			name: "AddOrUpdateNode_Update",
			assertion: func(t *testing.T, client *InMemoryClient) {
				node := getTestNode()
				// Add node
				err := client.AddOrUpdateNode(testCtx(), node)
				require.NoError(t, err)
				// Update same node
				node.IQN = "updated_iqn"
				err = client.AddOrUpdateNode(testCtx(), node)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.nodesAdded) // Should not increment on update
				assert.Len(t, client.nodes, 1)
				retrieved := client.nodes[node.Name]
				assert.Equal(t, "updated_iqn", retrieved.IQN)
			},
		},
		{
			name: "GetNode_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				node := getTestNode()
				err := client.AddOrUpdateNode(testCtx(), node)
				require.NoError(t, err)
				retrieved, err := client.GetNode(testCtx(), node.Name)
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, node.Name, retrieved.Name)
			},
		},
		{
			name: "GetNode_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetNode(testCtx(), "nonexistent")
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetNodes_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				nodes, err := client.GetNodes(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, nodes)
			},
		},
		{
			name: "GetNodes_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				node := getTestNode()
				err := client.AddOrUpdateNode(testCtx(), node)
				require.NoError(t, err)
				nodes, err := client.GetNodes(testCtx())
				assert.NoError(t, err)
				assert.Len(t, nodes, 1)
				assert.Equal(t, node.Name, nodes[0].Name)
			},
		},
		{
			name: "DeleteNode",
			assertion: func(t *testing.T, client *InMemoryClient) {
				node := getTestNode()
				err := client.AddOrUpdateNode(testCtx(), node)
				require.NoError(t, err)
				err = client.DeleteNode(testCtx(), node)
				assert.NoError(t, err)
				assert.NotContains(t, client.nodes, node.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Volume Publication tests
func TestInMemoryClient_VolumePublicationOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddVolumePublication_New",
			assertion: func(t *testing.T, client *InMemoryClient) {
				vp := getTestVolumePublication()
				err := client.AddVolumePublication(testCtx(), vp)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.volumePublicationsAdded)
				assert.Len(t, client.volumePublications, 1)
				assert.Contains(t, client.volumePublications, vp.Name)
			},
		},
		{
			name: "AddVolumePublication_Existing",
			assertion: func(t *testing.T, client *InMemoryClient) {
				vp := getTestVolumePublication()
				// Add first time
				err := client.AddVolumePublication(testCtx(), vp)
				require.NoError(t, err)
				// Add same publication again (should not increment counter)
				err = client.AddVolumePublication(testCtx(), vp)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.volumePublicationsAdded) // Should not increment
				assert.Len(t, client.volumePublications, 1)
			},
		},
		{
			name: "UpdateVolumePublication",
			assertion: func(t *testing.T, client *InMemoryClient) {
				vp := getTestVolumePublication()
				err := client.UpdateVolumePublication(testCtx(), vp)
				assert.NoError(t, err)
				// UpdateVolumePublication calls AddVolumePublication internally
				assert.Contains(t, client.volumePublications, vp.Name)
			},
		},
		{
			name: "GetVolumePublication_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				vp := getTestVolumePublication()
				err := client.AddVolumePublication(testCtx(), vp)
				require.NoError(t, err)
				retrieved, err := client.GetVolumePublication(testCtx(), vp.Name)
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, vp.Name, retrieved.Name)
			},
		},
		{
			name: "GetVolumePublication_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetVolumePublication(testCtx(), "nonexistent")
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetVolumePublications_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				pubs, err := client.GetVolumePublications(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, pubs)
			},
		},
		{
			name: "GetVolumePublications_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				vp := getTestVolumePublication()
				err := client.AddVolumePublication(testCtx(), vp)
				require.NoError(t, err)
				pubs, err := client.GetVolumePublications(testCtx())
				assert.NoError(t, err)
				assert.Len(t, pubs, 1)
				assert.Equal(t, vp.Name, pubs[0].Name)
			},
		},
		{
			name: "DeleteVolumePublication",
			assertion: func(t *testing.T, client *InMemoryClient) {
				vp := getTestVolumePublication()
				err := client.AddVolumePublication(testCtx(), vp)
				require.NoError(t, err)
				err = client.DeleteVolumePublication(testCtx(), vp)
				assert.NoError(t, err)
				assert.NotContains(t, client.volumePublications, vp.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Snapshot tests
func TestInMemoryClient_SnapshotOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddSnapshot",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.AddSnapshot(testCtx(), snapshot)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.snapshotsAdded)
				assert.Len(t, client.snapshots, 1)
				assert.Contains(t, client.snapshots, snapshot.ID())
			},
		},
		{
			name: "GetSnapshot_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.AddSnapshot(testCtx(), snapshot)
				require.NoError(t, err)
				retrieved, err := client.GetSnapshot(testCtx(), snapshot.Config.VolumeName, snapshot.Config.Name)
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, snapshot.Config.Name, retrieved.Config.Name)
			},
		},
		{
			name: "GetSnapshot_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetSnapshot(testCtx(), "nonexistent_volume", "nonexistent_snapshot")
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetSnapshots_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshots, err := client.GetSnapshots(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, snapshots)
			},
		},
		{
			name: "GetSnapshots_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.AddSnapshot(testCtx(), snapshot)
				require.NoError(t, err)
				snapshots, err := client.GetSnapshots(testCtx())
				assert.NoError(t, err)
				assert.Len(t, snapshots, 1)
				assert.Equal(t, snapshot.Config.Name, snapshots[0].Config.Name)
			},
		},
		{
			name: "UpdateSnapshot_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.AddSnapshot(testCtx(), snapshot)
				require.NoError(t, err)
				err = client.UpdateSnapshot(testCtx(), snapshot)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateSnapshot_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.UpdateSnapshot(testCtx(), snapshot)
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteSnapshot",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.AddSnapshot(testCtx(), snapshot)
				require.NoError(t, err)
				err = client.DeleteSnapshot(testCtx(), snapshot)
				assert.NoError(t, err)
				assert.NotContains(t, client.snapshots, snapshot.ID())
				assert.Equal(t, 0, client.snapshotsAdded) // Counter decrements
			},
		},
		{
			name: "DeleteSnapshots_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				err := client.DeleteSnapshots(testCtx())
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteSnapshots_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				snapshot := getTestSnapshot()
				err := client.AddSnapshot(testCtx(), snapshot)
				require.NoError(t, err)
				err = client.DeleteSnapshots(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, client.snapshots)
				assert.Equal(t, 0, client.snapshotsAdded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Group Snapshot tests
func TestInMemoryClient_GroupSnapshotOperations(t *testing.T) {
	tests := []struct {
		name      string
		assertion func(t *testing.T, client *InMemoryClient)
	}{
		{
			name: "AddGroupSnapshot_New",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.groupSnapshotsAdded)
				assert.Len(t, client.groupSnapshots, 1)
				persistent := groupSnapshot.ConstructPersistent()
				assert.Contains(t, client.groupSnapshots, persistent.ID())
			},
		},
		{
			name: "AddGroupSnapshot_Existing",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				// Add first time
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				require.NoError(t, err)
				// Add same group snapshot again (should not increment counter)
				err = client.AddGroupSnapshot(testCtx(), groupSnapshot)
				assert.NoError(t, err)
				assert.Equal(t, 1, client.groupSnapshotsAdded) // Should not increment
				assert.Len(t, client.groupSnapshots, 1)
			},
		},
		{
			name: "GetGroupSnapshot_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				require.NoError(t, err)
				persistent := groupSnapshot.ConstructPersistent()
				retrieved, err := client.GetGroupSnapshot(testCtx(), persistent.ID())
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, persistent.ID(), retrieved.ID())
			},
		},
		{
			name: "GetGroupSnapshot_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				retrieved, err := client.GetGroupSnapshot(testCtx(), "nonexistent")
				assert.Error(t, err)
				assert.Nil(t, retrieved)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "GetGroupSnapshots_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshots, err := client.GetGroupSnapshots(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, groupSnapshots)
			},
		},
		{
			name: "GetGroupSnapshots_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				require.NoError(t, err)
				groupSnapshots, err := client.GetGroupSnapshots(testCtx())
				assert.NoError(t, err)
				assert.Len(t, groupSnapshots, 1)
				persistent := groupSnapshot.ConstructPersistent()
				assert.Equal(t, persistent.ID(), groupSnapshots[0].ID())
			},
		},
		{
			name: "UpdateGroupSnapshot_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				require.NoError(t, err)
				err = client.UpdateGroupSnapshot(testCtx(), groupSnapshot)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateGroupSnapshot_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.UpdateGroupSnapshot(testCtx(), groupSnapshot)
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteGroupSnapshot_Success",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				require.NoError(t, err)
				err = client.DeleteGroupSnapshot(testCtx(), groupSnapshot)
				assert.NoError(t, err)
				persistent := groupSnapshot.ConstructPersistent()
				assert.NotContains(t, client.groupSnapshots, persistent.ID())
				assert.Equal(t, 0, client.groupSnapshotsAdded) // Counter decrements
			},
		},
		{
			name: "DeleteGroupSnapshot_NotFound",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.DeleteGroupSnapshot(testCtx(), groupSnapshot)
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteGroupSnapshots_Empty",
			assertion: func(t *testing.T, client *InMemoryClient) {
				err := client.DeleteGroupSnapshots(testCtx())
				assert.Error(t, err)
				assert.True(t, MatchKeyNotFoundErr(err))
			},
		},
		{
			name: "DeleteGroupSnapshots_WithData",
			assertion: func(t *testing.T, client *InMemoryClient) {
				groupSnapshot := getTestGroupSnapshot()
				err := client.AddGroupSnapshot(testCtx(), groupSnapshot)
				require.NoError(t, err)
				err = client.DeleteGroupSnapshots(testCtx())
				assert.NoError(t, err)
				assert.Empty(t, client.groupSnapshots)
				assert.Equal(t, 0, client.groupSnapshotsAdded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewInMemoryClient()
			tt.assertion(t, client)
		})
	}
}

// Integration tests with multiple operations

func TestInMemoryClient_MultipleOperations(t *testing.T) {
	client := NewInMemoryClient()

	// Add backend
	backend := getTestBackend()
	err := client.AddBackend(testCtx(), backend)
	require.NoError(t, err)

	// Add volume
	volume := getTestVolume(backend)
	err = client.AddVolume(testCtx(), volume)
	require.NoError(t, err)

	// Add storage class
	sc := getTestStorageClass()
	err = client.AddStorageClass(testCtx(), sc)
	require.NoError(t, err)

	// Add node
	node := getTestNode()
	err = client.AddOrUpdateNode(testCtx(), node)
	require.NoError(t, err)

	// Verify all data exists
	backends, err := client.GetBackends(testCtx())
	assert.NoError(t, err)
	assert.Len(t, backends, 1)

	volumes, err := client.GetVolumes(testCtx())
	assert.NoError(t, err)
	assert.Len(t, volumes, 1)

	classes, err := client.GetStorageClasses(testCtx())
	assert.NoError(t, err)
	assert.Len(t, classes, 1)

	nodes, err := client.GetNodes(testCtx())
	assert.NoError(t, err)
	assert.Len(t, nodes, 1)

	// Stop client should clear all data
	err = client.Stop()
	assert.NoError(t, err)

	// Verify all data is cleared
	backends, err = client.GetBackends(testCtx())
	assert.NoError(t, err)
	assert.Empty(t, backends)

	volumes, err = client.GetVolumes(testCtx())
	assert.NoError(t, err)
	assert.Empty(t, volumes)

	classes, err = client.GetStorageClasses(testCtx())
	assert.NoError(t, err)
	assert.Empty(t, classes)

	nodes, err = client.GetNodes(testCtx())
	assert.NoError(t, err)
	assert.Empty(t, nodes)
}
