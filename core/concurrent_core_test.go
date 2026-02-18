// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/brunoga/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	db "github.com/netapp/trident/core/concurrent_cache"
	"github.com/netapp/trident/logging"
	mockpersistentstore "github.com/netapp/trident/mocks/mock_persistent_store"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
	testutils "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var (
	inMemoryPersistence *persistentstore.InMemoryClient
	testCtx             = context.Background()
	expiredCtx, _       = context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	failed              = errors.New("failed")
)

func init() {
	inMemoryPersistence = persistentstore.NewInMemoryClient()
}

func persistenceCleanup(t *testing.T, o *ConcurrentTridentOrchestrator) {
	t.Helper()
	// We do not want to cleanup for MockStoreClient
	_, ok := o.storeClient.(*persistentstore.InMemoryClient)
	if !ok {
		return
	}

	err := o.storeClient.DeleteBackends(testCtx)
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up backends: ", err)
	}

	err = o.storeClient.DeleteVolumes(testCtx)
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up volumes: ", err)
	}

	// Clear the InMemoryClient state so that it looks like we're
	// bootstrapping afresh next time.
	if err = inMemoryPersistence.Stop(); err != nil {
		t.Fatalf("Unable to stop in memory client for orchestrator: %v", o)
	}

	config.CurrentDriverContext = ""
}

func getConcurrentOrchestrator() *ConcurrentTridentOrchestrator {
	orchestrator, _ := NewConcurrentTridentOrchestrator(inMemoryPersistence)
	concurrentOrchestrator := orchestrator.(*ConcurrentTridentOrchestrator)
	concurrentOrchestrator.bootstrapped = true
	concurrentOrchestrator.bootstrapError = nil
	return concurrentOrchestrator
}

func makeSyncMapFromMap[K comparable, V any](m map[K]V) *sync.Map {
	syncMap := &sync.Map{}
	for k, v := range m {
		syncMap.Store(k, v)
	}
	return syncMap
}

func addBackendsToCache(t *testing.T, backends ...storage.Backend) {
	t.Helper()
	for _, backend := range backends {
		results, unlocker, err := db.Lock(context.Background(), db.Query(db.UpsertBackend(backend.BackendUUID(), "",
			backend.Name())))
		require.NoError(t, err)
		results[0].Backend.Upsert(backend)
		unlocker()
	}
}

func removeBackendFromCache(t *testing.T, backendUUID string) {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.DeleteBackend(backendUUID)))
	defer unlocker()
	require.NoError(t, err)
	results[0].Backend.Delete()
}

func addStorageClassesToCache(t *testing.T, storageClasses ...*storageclass.StorageClass) {
	t.Helper()
	for _, sc := range storageClasses {
		results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertStorageClass(sc.GetName())))
		require.NoError(t, err)
		results[0].StorageClass.Upsert(sc)
		unlocker()
	}
}

func addSnapshotsToCache(t *testing.T, snapshots ...*storage.Snapshot) {
	t.Helper()
	for _, snapshot := range snapshots {
		results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertSnapshot(snapshot.Config.VolumeName, snapshot.Config.ID())))
		require.NoError(t, err)
		results[0].Snapshot.Upsert(snapshot)
		unlocker()
	}
}

func addVolumePublicationsToCache(t *testing.T, publications ...*models.VolumePublication) {
	t.Helper()
	for _, publication := range publications {
		results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertVolumePublication(publication.VolumeName, publication.NodeName)))
		require.NoError(t, err)
		results[0].VolumePublication.Upsert(publication)
		unlocker()
	}
}

func getBackendByUuidFromCache(t *testing.T, backendUuid string) storage.Backend {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadBackend(backendUuid)))
	defer unlocker()
	require.NoError(t, err)
	backend := results[0].Backend.Read
	return backend
}

func getSnapshotByIDFromCache(t *testing.T, snapshotId string) *storage.Snapshot {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadSnapshot(snapshotId)))
	defer unlocker()
	require.NoError(t, err)
	snapshot := results[0].Snapshot.Read
	return snapshot
}

func getBackendByNameFromCache(t *testing.T, backendName string) storage.Backend {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadBackendByName(backendName)))
	defer unlocker()
	require.NoError(t, err)
	backend := results[0].Backend.Read
	return backend
}

func getStorageClassByNameFromCache(t *testing.T, scName string) *storageclass.StorageClass {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadStorageClass(scName)))
	defer unlocker()
	require.NoError(t, err)
	sc := results[0].StorageClass.Read
	return sc
}

func getVolumeByNameFromCache(t *testing.T, volumeName string) *storage.Volume {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadVolume(volumeName)))
	defer unlocker()
	require.NoError(t, err)
	volume := results[0].Volume.Read
	return volume
}

func getSubVolumeByNameFromCache(t *testing.T, volumeName string) *storage.Volume {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadSubordinateVolume(volumeName)))
	defer unlocker()
	require.NoError(t, err)
	volume := results[0].SubordinateVolume.Read
	return volume
}

func addBackendsToPersistence(t *testing.T, o *ConcurrentTridentOrchestrator, backends ...storage.Backend) {
	t.Helper()
	for _, backend := range backends {
		err := o.storeClient.AddBackend(testCtx, backend)
		require.NoError(t, err)
	}
}

func addVolumesToCache(t *testing.T, vols ...*storage.Volume) {
	t.Helper()
	for _, vol := range vols {
		results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertVolume(vol.Config.Name, vol.BackendUUID)))
		require.NoError(t, err)
		results[0].Volume.Upsert(vol)
		unlocker()
	}
}

func addVolumesToPersistence(t *testing.T, o *ConcurrentTridentOrchestrator, vols ...*storage.Volume) {
	t.Helper()
	for _, vol := range vols {
		err := o.storeClient.AddVolume(testCtx, vol)
		require.NoError(t, err)
	}
}

func getNodeByNameFromCache(t *testing.T, nodeName string) *models.Node {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadNode(nodeName)))
	defer unlocker()
	require.NoError(t, err)
	node := results[0].Node.Read
	return node
}

func addNodesToCache(t *testing.T, nodes ...*models.Node) {
	t.Helper()
	for _, node := range nodes {
		results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertNode(node.Name)))
		require.NoError(t, err)
		results[0].Node.Upsert(node)
		unlocker()
	}
}

func getVolumePublicationByIDFromCache(t *testing.T, volumeID, nodeID string) *models.VolumePublication {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadVolumePublication(volumeID, nodeID)))
	defer unlocker()
	require.NoError(t, err)
	vp := results[0].VolumePublication.Read
	return vp
}

func addVolumePublicationsToPersistence(t *testing.T, o *ConcurrentTridentOrchestrator, publications ...*models.VolumePublication) {
	t.Helper()
	for _, publication := range publications {
		err := o.storeClient.AddVolumePublication(testCtx, publication)
		require.NoError(t, err)
	}
}

// create function for adding subordinate volumes
func addSubordinateVolumesToCache(t *testing.T, subVols ...*storage.Volume) {
	t.Helper()
	for _, subVol := range subVols {
		results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertSubordinateVolume(subVol.Config.Name, subVol.Config.ShareSourceVolume)))
		require.NoError(t, err)
		results[0].SubordinateVolume.Upsert(subVol)
		unlocker()
	}
}

func getSubordinateVolumeByNameFromCache(t *testing.T, volumeId string) *storage.Volume {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadSubordinateVolume(volumeId)))
	defer unlocker()
	require.NoError(t, err)
	volume := results[0].SubordinateVolume.Read
	return volume
}

func removeVolumeFromCache(t *testing.T, volumeName string) {
	t.Helper()
	results, unlocker, err := db.Lock(testCtx, db.Query(db.DeleteVolume(volumeName)))
	defer unlocker()
	require.NoError(t, err)
	results[0].Volume.Delete()
}

func getFakeStorageDriverConfig(name string) drivers.FakeStorageDriverConfig {
	fakeConfig := drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           drivers.ConfigVersion,
			StorageDriverName: config.FakeStorageDriverName,
			Flags:             make(map[string]string),
		},
		Protocol:     config.File,
		Pools:        testutils.GenerateFakePools(2),
		InstanceName: name,
		VolumeAccess: "1.0.0.1",
	}
	return fakeConfig
}

func getFakeBackend(name, uuid string, driver storage.Driver) *storage.StorageBackend {
	fakeConfig := getFakeStorageDriverConfig(name)
	if driver == nil {
		driver = fakedriver.NewFakeStorageDriver(testCtx, fakeConfig)
	}
	fakeBackend, _ := storage.NewStorageBackend(testCtx, driver)
	fakeBackend.SetName(name)
	fakeBackend.SetBackendUUID(uuid)
	fakeBackend.SetState(storage.Online)
	return fakeBackend
}

func getFakeBackendWithConfig(
	name, uuid string, driver storage.Driver, fakeConfig drivers.FakeStorageDriverConfig,
) *storage.StorageBackend {
	if driver == nil {
		driver = fakedriver.NewFakeStorageDriver(testCtx, fakeConfig)
	}
	fakeBackend, _ := storage.NewStorageBackend(testCtx, driver)
	fakeBackend.SetName(name)
	fakeBackend.SetBackendUUID(uuid)
	fakeBackend.SetState(storage.Online)
	return fakeBackend
}

func getMockBackend(mockCtrl *gomock.Controller, name, uuid string) *mockstorage.MockBackend {
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	mockBackend.EXPECT().Name().Return(name).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(uuid).AnyTimes()
	mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("ontap-nas").AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().Online().Return(true).AnyTimes()
	mockBackend.EXPECT().SmartCopy().Return(mockBackend).AnyTimes()
	mockBackend.EXPECT().GetUniqueKey().Return(name).AnyTimes()
	mockBackend.EXPECT().ConstructPersistent(gomock.Any()).Return(&storage.BackendPersistent{Name: name, BackendUUID: uuid}).AnyTimes()

	return mockBackend
}

func getMockBackendWithMap(mockCtrl *gomock.Controller, attributes map[string]string) *mockstorage.MockBackend {
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	if name, ok := attributes["name"]; ok {
		mockBackend.EXPECT().Name().Return(name).AnyTimes()
	}
	if uuid, ok := attributes["uuid"]; ok {
		mockBackend.EXPECT().BackendUUID().Return(uuid).AnyTimes()
	}
	if protocol, ok := attributes["protocol"]; ok {
		mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.Protocol(protocol)).AnyTimes()
	}
	if driverName, ok := attributes["driverName"]; ok {
		mockBackend.EXPECT().GetDriverName().Return(driverName).AnyTimes()
	}
	if state, ok := attributes["state"]; ok {
		mockBackend.EXPECT().State().Return(storage.BackendState(state)).AnyTimes()
		mockBackend.EXPECT().Online().Return(true).AnyTimes()
	}
	if online, ok := attributes["online"]; ok {
		mockBackend.EXPECT().Online().Return(online == "true").AnyTimes()
	}

	mockBackend.EXPECT().SmartCopy().Return(mockBackend).AnyTimes()

	mockBackend.EXPECT().ConstructPersistent(gomock.Any()).Return(&storage.BackendPersistent{
		Name:        attributes["name"],
		BackendUUID: attributes["uuid"],
	}).AnyTimes()

	return mockBackend
}

func getFakeStorageClass(name string) *storageclass.StorageClass {
	scConfig := &storageclass.Config{
		Version: "1",
		Name:    name,
	}
	return storageclass.New(scConfig)
}

func getFakeVolume(name, backendUUID string) *storage.Volume {
	fakeVolume := &storage.Volume{
		Config:      &storage.VolumeConfig{InternalName: name, Name: name},
		BackendUUID: backendUUID,
	}
	return fakeVolume
}

func getFakeNode(name string) *models.Node {
	fakeNode := &models.Node{
		Name: name,
	}
	return fakeNode
}

func getFakeVolumePublication(volumeName, nodeName string) *models.VolumePublication {
	fakeVolumePublication := &models.VolumePublication{
		VolumeName: volumeName,
		NodeName:   nodeName,
	}
	return fakeVolumePublication
}

// Bootstrap tests

func TestBootstrapConcurrentCore(t *testing.T) {
	version := &config.PersistentStateVersion{
		PublicationsSynced:     true,
		OrchestratorAPIVersion: "1",
		PersistentStoreVersion: string(persistentstore.CRDV1Store),
	}

	backend1 := &storage.BackendPersistent{
		Version: "1",
		Config: storage.PersistentStorageBackendConfig{
			FakeStorageDriverConfig: &drivers.FakeStorageDriverConfig{
				CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
					Version:           1,
					StorageDriverName: "fake",
					BackendName:       "fake1",
				},
				Protocol: config.File,
				Pools:    testutils.GenerateFakePools(1),
			},
		},
		Name:        "fake1",
		BackendUUID: "backend-uuid1",
		Online:      true,
		State:       storage.Online,
	}
	backends := []*storage.BackendPersistent{backend1}

	scConfig := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
	}
	sc := storageclass.New(scConfig)
	storageClasses := []*storageclass.Persistent{sc.ConstructPersistent()}

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "vol1",
		InternalName: "vol1",
	}
	volume := storage.Volume{
		Config: volConfig,
		State:  storage.VolumeStateOnline,
	}
	volumes := []*storage.VolumeExternal{volume.ConstructExternal()}

	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "vol1",
		VolumeInternalName: "vol1",
	}
	snapshot := &storage.Snapshot{
		Config: snapConfig,
		State:  storage.SnapshotStateOnline,
	}
	snapshotID := snapshot.ID()
	snapshots := []*storage.SnapshotPersistent{snapshot.ConstructPersistent()}

	pub := &models.VolumePublication{
		NodeName:   "node1",
		VolumeName: "vol1",
	}
	pubs := []*models.VolumePublication{pub}

	node := &models.Node{
		Name: "node1",
	}
	nodes := []*models.Node{node}

	tests := []struct {
		name         string
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(o *ConcurrentTridentOrchestrator)
	}{
		{
			name: "GetVersionFailure",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				o.bootstrapped = false
				o.bootstrapError = failed

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(nil, failed).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsBootstrapError(err))
			},
			verifyResult: func(o *ConcurrentTridentOrchestrator) {
				assert.False(t, o.bootstrapped)
				assert.NotNil(t, o.bootstrapError)
			},
		},
		{
			name: "SuccessDifferentAPIVersion",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				o.uuid = ""
				o.bootstrapped = false
				o.bootstrapError = failed

				newVersion := &config.PersistentStateVersion{
					PublicationsSynced:     true,
					OrchestratorAPIVersion: "0",
					PersistentStoreVersion: string(persistentstore.CRDV1Store),
				}

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(newVersion, nil).Times(1)
				mockStoreClient.EXPECT().GetType().Return(persistentstore.CRDV1Store).Times(1)
				mockStoreClient.EXPECT().SetVersion(gomock.Any(), version).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetTridentUUID(gomock.Any()).Return("trident-uuid", nil).AnyTimes()
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
				mockStoreClient.EXPECT().GetAutogrowPolicies(gomock.Any()).Return([]*storage.AutogrowPolicyPersistent{}, nil).AnyTimes()
				mockStoreClient.EXPECT().GetStorageClasses(gomock.Any()).Return(storageClasses, nil).AnyTimes()
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
				mockStoreClient.EXPECT().GetSnapshots(gomock.Any()).Return(snapshots, nil).AnyTimes()
				mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return(pubs, nil).AnyTimes()
				mockStoreClient.EXPECT().GetNodes(gomock.Any()).Return(nodes, nil).AnyTimes()
				mockStoreClient.EXPECT().IsBackendDeleting(gomock.Any(), gomock.Any()).Return(false).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(o *ConcurrentTridentOrchestrator) {
				assert.Equal(t, "trident-uuid", o.uuid)
				assert.True(t, o.bootstrapped)
				assert.Nil(t, o.bootstrapError)
			},
		},
		{
			name: "SetVersionFailure",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				o.bootstrapped = false
				o.bootstrapError = failed

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(version, nil).Times(1)
				mockStoreClient.EXPECT().GetType().Return(persistentstore.CRDV1Store).Times(1)
				mockStoreClient.EXPECT().SetVersion(gomock.Any(), version).Return(failed).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsBootstrapError(err))
			},
			verifyResult: func(o *ConcurrentTridentOrchestrator) {
				assert.False(t, o.bootstrapped)
				assert.NotNil(t, o.bootstrapError)
			},
		},
		{
			name: "TridentUUIDFailure",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				o.bootstrapped = false
				o.bootstrapError = failed

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(version, nil).Times(1)
				mockStoreClient.EXPECT().GetType().Return(persistentstore.CRDV1Store).Times(1)
				mockStoreClient.EXPECT().SetVersion(gomock.Any(), version).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetTridentUUID(gomock.Any()).Return("", failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsBootstrapError(err))
			},
			verifyResult: func(o *ConcurrentTridentOrchestrator) {
				assert.False(t, o.bootstrapped)
				assert.NotNil(t, o.bootstrapError)
			},
		},
		{
			name: "PersistenceFailure",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				o.bootstrapped = false
				o.bootstrapError = failed

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(version, nil).Times(1)
				mockStoreClient.EXPECT().GetType().Return(persistentstore.CRDV1Store).Times(1)
				mockStoreClient.EXPECT().SetVersion(gomock.Any(), version).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetTridentUUID(gomock.Any()).Return("trident-uuid", nil).AnyTimes()
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
				mockStoreClient.EXPECT().GetAutogrowPolicies(gomock.Any()).Return([]*storage.AutogrowPolicyPersistent{}, nil).AnyTimes()
				mockStoreClient.EXPECT().GetStorageClasses(gomock.Any()).Return(nil, persistentstore.NewPersistentStoreError("failed", "key")).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsBootstrapError(err))
			},
			verifyResult: func(o *ConcurrentTridentOrchestrator) {
				assert.False(t, o.bootstrapped)
				assert.NotNil(t, o.bootstrapError)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				o.uuid = ""
				o.bootstrapped = false
				o.bootstrapError = failed

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(version, nil).Times(1)
				mockStoreClient.EXPECT().GetType().Return(persistentstore.CRDV1Store).Times(1)
				mockStoreClient.EXPECT().SetVersion(gomock.Any(), version).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetTridentUUID(gomock.Any()).Return("trident-uuid", nil).AnyTimes()
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
				mockStoreClient.EXPECT().GetAutogrowPolicies(gomock.Any()).Return([]*storage.AutogrowPolicyPersistent{}, nil).AnyTimes()
				mockStoreClient.EXPECT().GetStorageClasses(gomock.Any()).Return(storageClasses, nil).AnyTimes()
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
				mockStoreClient.EXPECT().GetSnapshots(gomock.Any()).Return(snapshots, nil).AnyTimes()
				mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return(pubs, nil).AnyTimes()
				mockStoreClient.EXPECT().GetNodes(gomock.Any()).Return(nodes, nil).AnyTimes()
				mockStoreClient.EXPECT().IsBackendDeleting(gomock.Any(), gomock.Any()).Return(false).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(o *ConcurrentTridentOrchestrator) {
				assert.Equal(t, "trident-uuid", o.uuid)
				assert.True(t, o.bootstrapped)
				assert.Nil(t, o.bootstrapError)

				backendResult := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, backendResult)

				scResult := getStorageClassByNameFromCache(t, "sc1")
				assert.NotNil(t, scResult)

				volumeResult := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volumeResult)

				snapshotResult := getSnapshotByIDFromCache(t, snapshotID)
				assert.NotNil(t, snapshotResult)

				pubResult := getVolumePublicationByIDFromCache(t, "vol1", "node1")
				assert.NotNil(t, pubResult)

				nodeResult := getNodeByNameFromCache(t, "node1")
				assert.NotNil(t, nodeResult)

				expectedSC1PoolMap := map[string][]string{"fake1": {"pool-0"}}
				assert.Equal(t, expectedSC1PoolMap, o.scPoolMap.BackendPoolMapForStorageClass(testCtx, "sc1"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.Bootstrap(false)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapBackendsConcurrentCore(t *testing.T) {
	backend := &storage.BackendPersistent{
		Version: "1",
		Config: storage.PersistentStorageBackendConfig{
			FakeStorageDriverConfig: &drivers.FakeStorageDriverConfig{
				CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
					Version:           1,
					StorageDriverName: "fake",
					BackendName:       "fake1",
				},
				Protocol: config.File,
				Pools:    testutils.GenerateFakePools(1),
			},
		},
		Name:        "fake1",
		BackendUUID: "backend-uuid1",
		Online:      true,
		State:       storage.Online,
	}
	backends := []*storage.BackendPersistent{backend}

	tests := []struct {
		name        string
		ctx         context.Context
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "StoreError",
			ctx:  testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, result)
			},
		},
		{
			name: "Success",
			ctx:  testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, result)
				assert.Equal(t, storage.Online, result.State())
			},
		},
		{
			name: "SuccessDeleting",
			ctx:  testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				backendsCopy := []*storage.BackendPersistent{deep.MustCopy(backends[0])}
				backendsCopy[0].State = storage.Deleting
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backendsCopy, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, result)
				assert.Equal(t, storage.Deleting, result.State())
			},
		},
		{
			name: "LockError",
			ctx:  expiredCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, result)
			},
		},
		{
			name: "MarshalError",
			ctx:  testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				noConfigBackend := &storage.BackendPersistent{Name: "backend1", BackendUUID: "backend-uuid1"}

				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return([]*storage.BackendPersistent{noConfigBackend}, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, result)
			},
		},
		{
			name: "ValidateError",
			ctx:  testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				unknownBackend := &storage.BackendPersistent{
					Version: "1",
					Config: storage.PersistentStorageBackendConfig{
						FakeStorageDriverConfig: &drivers.FakeStorageDriverConfig{
							CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
								Version:           1,
								StorageDriverName: "unknown",
								BackendName:       "fake1",
							},
						},
					},
				}

				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return([]*storage.BackendPersistent{unknownBackend}, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, result)
			},
		},
		{
			name: "ValidateErrorDocker",
			ctx:  testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextDocker
				unknownBackend := &storage.BackendPersistent{
					Version: "1",
					Config: storage.PersistentStorageBackendConfig{
						FakeStorageDriverConfig: &drivers.FakeStorageDriverConfig{
							CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
								Version:           1,
								StorageDriverName: "unknown",
								BackendName:       "fake1",
							},
						},
					},
				}

				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return([]*storage.BackendPersistent{unknownBackend}, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapBackends(tt.ctx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapStorageClassesConcurrentCore(t *testing.T) {
	scConfig := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
	}
	sc := storageclass.New(scConfig)

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "StoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetStorageClasses(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getStorageClassByNameFromCache(t, "sc1")
				assert.Nil(t, result)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				storageClasses := []*storageclass.Persistent{sc.ConstructPersistent()}
				mockStoreClient.EXPECT().GetStorageClasses(gomock.Any()).Return(storageClasses, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getStorageClassByNameFromCache(t, "sc1")
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapStorageClasses(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapVolumesConcurrentCore(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "vol1",
		InternalName: "vol1",
	}
	volume := storage.Volume{
		Config: volConfig,
		State:  storage.VolumeStateOnline,
	}

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "StoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, result)
			},
		},
		{
			name: "SuccessMissingBackend",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				volumes := []*storage.VolumeExternal{volume.ConstructExternal()}
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, result)
				assert.Equal(t, storage.VolumeStateMissingBackend, result.State)
			},
		},
		{
			name: "SuccessSubordinate",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				volumes := []*storage.VolumeExternal{volume.ConstructExternal()}
				volumes[0].State = storage.VolumeStateSubordinate
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getSubordinateVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, result)
				assert.Equal(t, storage.VolumeStateSubordinate, result.State)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().Volumes().Return(&sync.Map{}).AnyTimes()
				mockBackend.EXPECT().Driver().Return(&fakedriver.StorageDriver{}).AnyTimes()
				mockBackend.EXPECT().HealVolumePublishEnforcement(gomock.Any(), gomock.Any()).Return(false)

				addBackendsToCache(t, mockBackend)

				volumes := []*storage.VolumeExternal{volume.ConstructExternal()}
				volumes[0].BackendUUID = "backend-uuid"
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, result)
				assert.Equal(t, storage.VolumeStateOnline, result.State)
			},
		},
		{
			name: "Success_AutogrowStatusPreserved",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().Volumes().Return(&sync.Map{}).AnyTimes()
				mockBackend.EXPECT().Driver().Return(&fakedriver.StorageDriver{}).AnyTimes()
				mockBackend.EXPECT().HealVolumePublishEnforcement(gomock.Any(), gomock.Any()).Return(false)

				addBackendsToCache(t, mockBackend)

				volumes := []*storage.VolumeExternal{volume.ConstructExternal()}
				volumes[0].BackendUUID = "backend-uuid"
				now := time.Now()
				volumes[0].AutogrowStatus = &models.VolumeAutogrowStatus{
					LastAutogrowPolicyUsed:  "policy1",
					LastAutogrowAttemptedAt: &now,
					TotalAutogrowAttempted:  1,
					TotalSuccessfulAutogrow: 1,
				}
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				require.NotNil(t, result)
				require.NotNil(t, result.AutogrowStatus)
				assert.Equal(t, "policy1", result.AutogrowStatus.LastAutogrowPolicyUsed)
				assert.Equal(t, 1, result.AutogrowStatus.TotalAutogrowAttempted)
				assert.Equal(t, 1, result.AutogrowStatus.TotalSuccessfulAutogrow)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapVolumes(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapSnapshotsConcurrentCore(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "vol1",
		InternalName: "vol1",
	}
	volume := &storage.Volume{
		Config:      volConfig,
		BackendUUID: "backend-uuid",
		State:       storage.VolumeStateOnline,
	}

	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "vol1",
		VolumeInternalName: "vol1",
	}
	snapshot := &storage.Snapshot{
		Config: snapConfig,
		State:  storage.SnapshotStateOnline,
	}
	snapshotID := snapshot.ID()

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "StoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetSnapshots(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)

				result := getSnapshotByIDFromCache(t, snapshotID)
				assert.Nil(t, result)
			},
		},
		{
			name: "SuccessMissingVolume",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				snapshots := []*storage.SnapshotPersistent{snapshot.ConstructPersistent()}
				mockStoreClient.EXPECT().GetSnapshots(gomock.Any()).Return(snapshots, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getSnapshotByIDFromCache(t, snapshotID)
				assert.NotNil(t, result)
				assert.Equal(t, storage.SnapshotStateMissingVolume, result.State)
			},
		},
		{
			name: "SuccessMissingBackend",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				addVolumesToCache(t, volume)

				snapshots := []*storage.SnapshotPersistent{snapshot.ConstructPersistent()}
				mockStoreClient.EXPECT().GetSnapshots(gomock.Any()).Return(snapshots, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getSnapshotByIDFromCache(t, snapshotID)
				assert.NotNil(t, result)
				assert.Equal(t, storage.SnapshotStateMissingBackend, result.State)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				fakeDriver := fakedriver.NewFakeStorageDriverWithDebugTraceFlags(map[string]bool{})
				mockBackend.EXPECT().Driver().Return(fakeDriver).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, volume)

				snapshots := []*storage.SnapshotPersistent{snapshot.ConstructPersistent()}
				mockStoreClient.EXPECT().GetSnapshots(gomock.Any()).Return(snapshots, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, result)
				assert.Equal(t, storage.VolumeStateOnline, result.State)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapSnapshots(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapVolumeTransactionsConcurrentCore(t *testing.T) {
	txn := &storage.VolumeTransaction{
		Op: storage.VolumeCreating,
		VolumeCreatingConfig: &storage.VolumeCreatingConfig{
			VolumeConfig: storage.VolumeConfig{
				Name: "vol1",
			},
			BackendUUID: "backend-uuid1",
		},
	}

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "StoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{txn}, nil).AnyTimes()
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), txn).Return(nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapVolTxns(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapVolumePublicationsConcurrentCore(t *testing.T) {
	pub := &models.VolumePublication{
		NodeName:   "node1",
		VolumeName: "vol1",
	}

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "NonCSI",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextDocker

				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "StoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return([]*models.VolumePublication{pub}, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumePublicationByIDFromCache(t, "vol1", "node1")
				assert.NotNil(t, result)
			},
		},
		{
			name: "SuccessWithVPSync",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				// Create a volume publication that needs syncing
				vpToSync := &models.VolumePublication{
					NodeName:       "node1",
					VolumeName:     "vol1",
					AutogrowPolicy: "", // Empty, needs to be synced from volume
				}

				// Create corresponding volume with autogrow policy
				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
				}
				addVolumesToCache(t, vol)

				// VP sync happens asynchronously, so we allow UpdateVolumePublication to be called
				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return([]*models.VolumePublication{vpToSync}, nil)
				mockStoreClient.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumePublicationByIDFromCache(t, "vol1", "node1")
				assert.NotNil(t, result)
			},
		},
		{
			name: "VolumeNotFoundForVP",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				// Create VP but don't add corresponding volume to cache
				vpOrphan := &models.VolumePublication{
					NodeName:   "node1",
					VolumeName: "nonexistent-vol",
				}

				mockStoreClient.EXPECT().GetVolumePublications(gomock.Any()).Return([]*models.VolumePublication{vpOrphan}, nil)
			},
			verifyError: func(err error) {
				// Should still succeed - orphan VPs are just skipped during sync
				assert.NoError(t, err)

				result := getVolumePublicationByIDFromCache(t, "nonexistent-vol", "node1")
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapVolumePublications(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapNodesConcurrentCore(t *testing.T) {
	node := &models.Node{
		Name: "node1",
	}

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "NonCSI",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextDocker

				mockStoreClient.EXPECT().GetNodes(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "StoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				mockStoreClient.EXPECT().GetNodes(gomock.Any()).Return(nil, failed).AnyTimes()
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				nodes := []*models.Node{node}

				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(false).AnyTimes()
				mockBackend.EXPECT().ReconcileNodeAccess(gomock.Any(), nodes, gomock.Any()).Return(nil).AnyTimes()
				mockBackend.EXPECT().IsNodeAccessUpToDate().Return(false).AnyTimes()
				mockBackend.EXPECT().SetNodeAccessUpToDate().AnyTimes()

				addBackendsToCache(t, mockBackend)

				mockStoreClient.EXPECT().GetNodes(gomock.Any()).Return(nodes, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getNodeByNameFromCache(t, "node1")
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapNodes(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestBootstrapSubordinateVolumesConcurrentCore(t *testing.T) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "vol1",
		InternalName: "vol1",
	}
	volume := &storage.Volume{
		Config:      volConfig,
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subvolumeConfig := &storage.VolumeConfig{
		Name:              "subvol1",
		InternalName:      "subvol1",
		Size:              "1G",
		ShareSourceVolume: "vol1",
		StorageClass:      "gold",
	}
	subVolume := &storage.Volume{
		Config:      subvolumeConfig,
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				volumes := []*storage.VolumeExternal{volume.ConstructExternal()}
				volumes[0].State = storage.VolumeStateSubordinate
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()

				addVolumesToCache(t, volume)
				addSubordinateVolumesToCache(t, subVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, result)

				_, ok := result.Config.SubordinateVolumes["subvol1"]
				assert.True(t, ok)
			},
		},
		{
			name: "SuccessSourceVolumeMissing",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				volumes := []*storage.VolumeExternal{volume.ConstructExternal()}
				volumes[0].State = storage.VolumeStateSubordinate
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()

				addSubordinateVolumesToCache(t, subVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapSubordinateVolumes(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

// Backend tests

func TestValidateAndCreateBackendFromConfig_SetsConcurrentFlag(t *testing.T) {
	db.Initialize()
	o := getConcurrentOrchestrator()

	fakeConfig := getFakeStorageDriverConfig("test-backend")
	configJSON, err := fakeConfig.Marshal()
	require.NoError(t, err)

	backend, err := o.validateAndCreateBackendFromConfig(testCtx, string(configJSON), "", "")

	require.NoError(t, err)
	require.NotNil(t, backend)

	// Verify the concurrent flag was set in the backend's common config
	commonConfig := backend.Driver().GetCommonConfig(testCtx)
	require.NotNil(t, commonConfig.Flags)
	assert.Equal(t, "true", commonConfig.Flags[FlagConcurrent])

	persistenceCleanup(t, o)
}

func TestGetBackendConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		backendName  string
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result *storage.BackendExternal)
	}{
		{
			name:        "Success",
			backendName: "testBackend1",
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend1 := getFakeBackend("testBackend1", "uuid1", nil)
				backend2 := getFakeBackend("testBackend2", "uuid2", nil)
				addBackendsToCache(t, backend1, backend2)
				addBackendsToPersistence(t, o, backend1, backend2)
			},
			verifyError: func(err error) {
				require.NoError(t, err)
			},
			verifyResult: func(result *storage.BackendExternal) {
				assert.Equal(t, "testBackend1", result.Name)
			},
		},
		{
			name:        "NotFound",
			backendName: "testBackend2",
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend testBackend2 was not found")
			},
			verifyResult: func(result *storage.BackendExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:        "BootstrapError",
			backendName: "testBackend",
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				o.bootstrapError = errors.New("bootstrap error")
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(result *storage.BackendExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.GetBackend(testCtx, tt.backendName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetBackendByBackendUUIDConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		backendUUID  string
		verifyError  func(err error)
		verifyResult func(result *storage.BackendExternal)
	}{
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend1 := getFakeBackend("testBackend1", "uuid1", nil)
				backend2 := getFakeBackend("testBackend2", "uuid2", nil)
				addBackendsToCache(t, backend1, backend2)
				addBackendsToPersistence(t, o, backend1, backend2)
			},
			backendUUID: "uuid1",
			verifyError: func(err error) {
				require.NoError(t, err)
			},
			verifyResult: func(result *storage.BackendExternal) {
				assert.Equal(t, "testBackend1", result.Name)
			},
		},
		{
			name:         "NotFound",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			backendUUID: "uuid2",
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend with UUID uuid2 was not found")
			},
			verifyResult: func(result *storage.BackendExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No backends added for this case
			},
			backendUUID: "uuid",
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(result *storage.BackendExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.GetBackendByBackendUUID(testCtx, tt.backendUUID)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListBackendsConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storage.BackendExternal)
	}{
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend1 := getFakeBackend("testBackend1", "uuid1", nil)
				backend2 := getFakeBackend("testBackend2", "uuid2", nil)
				addBackendsToCache(t, backend1, backend2)
				addBackendsToPersistence(t, o, backend1, backend2)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.BackendExternal) {
				require.Len(t, result, 2)
				expectedNames := []string{"testBackend1", "testBackend2"}
				actualNames := []string{}
				for _, backend := range result {
					actualNames = append(actualNames, backend.Name)
				}
				assert.ElementsMatch(t, expectedNames, actualNames)
			},
		},
		{
			name:         "Empty",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No backends added
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.BackendExternal) {
				assert.Empty(t, result)
			},
		},
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No backends added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storage.BackendExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.ListBackends(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteBackendConcurrentCore(t *testing.T) {
	tests := []struct {
		name                string
		backendNameToDelete string
		setupMocks          func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError         func(err error)
		verifyResult        func()
	}{
		{
			name:                "Success",
			backendNameToDelete: "testBackend",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
				addBackendsToPersistence(t, o, fakeBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func() {
				assert.Nil(t, getBackendByUuidFromCache(t, "uuid"))
			},
		},
		{
			name:                "DeleteAndAddAgain",
			backendNameToDelete: "testBackend",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
				addBackendsToPersistence(t, o, fakeBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func() {
				assert.Nil(t, getBackendByUuidFromCache(t, "uuid"))
				// Add the backend again to the cache. should not give any errors
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
			},
		},
		{
			name:                "NotFound",
			backendNameToDelete: "testBackend2",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
				addBackendsToPersistence(t, o, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend testBackend2 was not found")
			},
			verifyResult: func() {
				assert.NotNil(t, getBackendByUuidFromCache(t, "uuid"))
			},
		},
		{
			name:                "BootstrapError",
			backendNameToDelete: "testBackend",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				o.bootstrapError = errors.New("bootstrap error")
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func() {
				assert.NotNil(t, getBackendByUuidFromCache(t, "uuid"))
			},
		},
		{
			name:                "BackendInUse",
			backendNameToDelete: "testBackend",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "uuid")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(true).AnyTimes()
				mockBackend.EXPECT().ConfigRef().Return("").AnyTimes()
				mockBackend.EXPECT().SetState(storage.Deleting).Times(1)
				mockBackend.EXPECT().SetOnline(false).Times(1)
				mockBackend.EXPECT().Terminate(gomock.Any()).Times(0)
				addBackendsToCache(t, mockBackend)
				addBackendsToPersistence(t, o, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func() {
				assert.NotNil(t, getBackendByUuidFromCache(t, "uuid"))
			},
		},
		{
			name:                "UsingTridentctl",
			backendNameToDelete: "testBackend",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "uuid")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().ConfigRef().Return("tridentBackendConfig").AnyTimes()
				addBackendsToCache(t, mockBackend)
				addBackendsToPersistence(t, o, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "cannot delete backend 'testBackend' created using "+
					"TridentBackendConfig CR; delete the TridentBackendConfig CR first")
			},
			verifyResult: func() {
				assert.NotNil(t, getBackendByUuidFromCache(t, "uuid"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			o := getConcurrentOrchestrator()
			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.DeleteBackend(testCtx, tt.backendNameToDelete)
			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult()
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteBackendByBackendUUIDConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		backendName    string
		backendUUID    string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
	}{
		{
			name:        "Success",
			backendName: "testBackend1",
			backendUUID: "backend-uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().HasVolumes().Return(false).Times(1)

				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockBackend.EXPECT().ConfigRef().Return("").Times(1)
				mockBackend.EXPECT().Terminate(gomock.Any()).Times(1)
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend).Return(nil).Times(1)

				o.storeClient = mockStoreClient

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:           "BootstrapError",
			backendName:    "testBackend1",
			backendUUID:    "backend-uuid1",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
		},
		{
			name:        "BackendNotFound",
			backendName: "nonExistentBackend",
			backendUUID: "nonexistent-backend-uuid",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No backend added to cache
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonExistentBackend was not found")
			},
		},
		{
			name:        "BackendHasVolumes",
			backendName: "testBackend1",
			backendUUID: "backend-uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().ConfigRef().Return("").Times(1)
				mockBackend.EXPECT().HasVolumes().Return(true).Times(1)
				mockBackend.EXPECT().SetOnline(false).Times(1)
				mockBackend.EXPECT().SetState(storage.Deleting).Times(1)
				addBackendsToCache(t, mockBackend)

				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), mockBackend).Return(nil).Times(1)
				o.storeClient = mockStoreClient
			},
			verifyError: func(err error) {
			},
		},
		{
			name:        "DeleteFromStoreError",
			backendName: "testBackend1",
			backendUUID: "backend-uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().HasVolumes().Return(false).Times(1)

				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockBackend.EXPECT().ConfigRef().Return("").Times(1)
				mockBackend.EXPECT().Terminate(gomock.Any()).Times(1)
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend).Return(errors.New("persistence error")).Times(1)

				o.storeClient = mockStoreClient

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "persistence error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.DeleteBackendByBackendUUID(testCtx, tt.backendName, tt.backendUUID)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func Test_UpdateBackendConcurrentCore(t *testing.T) {
	existingBackendName := "fake-backend"
	existingBackendUuid := "fake-backend-uuid"

	tests := []struct {
		name             string
		newBackendConfig map[string]interface{}
		contextValue     string
		callingConfigRef string
		setupMocks       func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError      func(err error)
		verifyResult     func(backend *storage.BackendExternal)
	}{
		{
			name:             "BootstrapError",
			newBackendConfig: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				o.bootstrapped = false
				o.bootstrapError = errors.New("bootstrap error")

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name: "BackendConfigRefError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				// Use case where Backend ConfigRef is non-empty
				results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertBackend(existingBackendUuid, "", "")))
				require.NoError(t, err)
				oldBackend := results[0].Backend.Read
				require.NotNil(t, oldBackend)
				oldBackend.SetConfigRef("test")
				results[0].Backend.Upsert(oldBackend)
				unlocker()
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "cannot update backend 'fake-backend' created using "+
					"TridentBackendConfig CR; please update the TridentBackendConfig CR")
			},
		},
		{
			name: "InvalidCallingConfig",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
			},
			callingConfigRef: "test",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend 'fake-backend' update initiated using an invalid "+
					"configRef, it is associated with configRef '' and not 'test'")
			},
		},
		{
			name: "BadCredentials",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"username":          "",
				"protocol":          config.File,
			},
			contextValue:     logging.ContextSourceCRD,
			callingConfigRef: "test",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "unsupported config error; input is missing the credentials field")
			},
		},
		{
			name: "UpdateStoragePrefixError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"storagePrefix":     "new-prefix",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "updating the storage prefix isn't currently supported")
			},
		},
		{
			name: "BackendRenameSuccess",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "new",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				mockStoreClient.EXPECT().ReplaceBackendAndUpdateVolumes(gomock.Any(), fakeBackend, gomock.Any()).
					Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				assert.Equal(t, "new", backend.Name)
			},
		},
		{
			name: "BackendRenameWithNameAlreadyExistsError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "new",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				// Add a backend with the same name as newBackend to get this error
				dupBackend := getFakeBackend("new", "new-uuid2", nil)
				addBackendsToCache(t, dupBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "key new for Backend already exists")
			},
		},
		{
			name: "BackendCRRenameError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "new",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().ReplaceBackendAndUpdateVolumes(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("CR rename error"))

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "CR rename error")
			},
		},
		{
			name: "InvalidUpdateError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.Block,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid backend update")
			},
		},
		{
			name: "UpdateError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).
					Return(errors.New("error updating backend"))

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "error updating backend")
			},
		},
		{
			name: "UpdateVolumeAccessError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"volumeAccess":      "1.1.1.1",
				"protocol":          config.File,
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "updating the data plane IP address isn't currently supported")
			},
		},
		{
			name: "UpdateNonOrphanVolumeError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(),
					gomock.Any()).Return(errors.New("error updating non-orphan volume"))

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: existingBackendUuid, Orphaned: false,
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateNonOrphanVolumeSuccess",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(),
					gomock.Any()).Return(nil).Times(1)

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: existingBackendUuid, Orphaned: false,
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				backendInCache := getBackendByUuidFromCache(t, backend.BackendUUID)
				volCount := 0
				backendInCache.Volumes().Range(func(_, _ interface{}) bool {
					volCount++
					return true
				})
				require.Equal(t, 1, volCount)
				vol, ok := backendInCache.Volumes().Load("vol1")
				assert.True(t, ok)
				assert.True(t, vol.(*storage.Volume).Orphaned)
			},
		},
		{
			name: "BackendUpdateSuccess",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
				"debugTraceFlags":   map[string]bool{"api": true, "method": true}, // turn on debug flags
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				fakeConfig, ok := backend.Config.(drivers.FakeStorageDriverConfig)
				if !ok {
					assert.Fail(t, "failed to cast backend config to FakeStorageDriverConfig")
				}
				// verify debug flags are updated
				assert.True(t, fakeConfig.DebugTraceFlags["api"])
				assert.True(t, fakeConfig.DebugTraceFlags["method"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var newBackendConfigJSON []byte
			var err error

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(o, mockStoreClient)
			}

			c := context.WithValue(ctx(), logging.ContextKeyRequestSource, tt.contextValue)
			newBackendConfigJSON, err = json.Marshal(tt.newBackendConfig)
			if err != nil {
				t.Fatal("failed to unmarshal newBackendConfig", err)
			}

			backend, err := o.UpdateBackend(c, existingBackendName, string(newBackendConfigJSON), tt.callingConfigRef)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(backend)
			}
		})
	}
}

func Test_UpdateBackendByBackendUUIDConcurrentCore(t *testing.T) {
	existingBackendName := "fake-backend"
	existingBackendUuid := "fake-backend-uuid"

	tests := []struct {
		name             string
		newBackendConfig map[string]interface{}
		contextValue     string
		callingConfigRef string
		setupMocks       func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError      func(err error)
		verifyResult     func(backend *storage.BackendExternal)
	}{
		{
			name:             "BootstrapError",
			newBackendConfig: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				o.bootstrapped = false
				o.bootstrapError = errors.New("bootstrap error")

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name: "BackendNotFound",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// Do not add existingBackend to cache
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend fake-backend was not found")
			},
		},
		{
			name: "BackendConfigRefError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				// Use case where Backend ConfigRef is non-empty
				results, unlocker, err := db.Lock(testCtx, db.Query(db.UpsertBackend(existingBackendUuid, "", "")))
				require.NoError(t, err)
				oldBackend := results[0].Backend.Read
				require.NotNil(t, oldBackend)
				oldBackend.SetConfigRef("test")
				results[0].Backend.Upsert(oldBackend)
				unlocker()
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "cannot update backend 'fake-backend' created using "+
					"TridentBackendConfig CR; please update the TridentBackendConfig CR")
			},
		},
		{
			name: "InvalidCallingConfig",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
			},
			callingConfigRef: "test",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend 'fake-backend' update initiated using an invalid "+
					"configRef, it is associated with configRef '' and not 'test'")
			},
		},
		{
			name: "BadCredentials",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"username":          "",
				"protocol":          config.File,
			},
			contextValue:     logging.ContextSourceCRD,
			callingConfigRef: "test",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "unsupported config error; input is missing the credentials field")
			},
		},
		{
			name: "UpdateStoragePrefixError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"storagePrefix":     "new-prefix",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "updating the storage prefix isn't currently supported")
			},
		},
		{
			name: "BackendRenameSuccess",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "new",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				mockStoreClient.EXPECT().ReplaceBackendAndUpdateVolumes(gomock.Any(), fakeBackend, gomock.Any()).
					Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				assert.Equal(t, "new", backend.Name)
			},
		},
		{
			name: "BackendRenameWithNameAlreadyExistsError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "new",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				// Add a backend with the same name as newBackend to get this error
				dupBackend := getFakeBackend("new", "new-uuid2", nil)
				addBackendsToCache(t, dupBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "key new for Backend already exists")
			},
		},
		{
			name: "BackendCRRenameError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "new",
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().ReplaceBackendAndUpdateVolumes(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("CR rename error"))

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "CR rename error")
			},
		},
		{
			name: "InvalidUpdateError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.Block,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid backend update")
			},
		},
		{
			name: "UpdateError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).
					Return(errors.New("error updating backend"))

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "error updating backend")
			},
		},
		{
			name: "UpdateVolumeAccessError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"volumeAccess":      "1.1.1.1",
				"protocol":          config.File,
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "updating the data plane IP address isn't currently supported")
			},
		},
		{
			name: "UpdateNonOrphanVolumeError",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(),
					gomock.Any()).Return(errors.New("error updating non-orphan volume"))

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: existingBackendUuid, Orphaned: false,
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateNonOrphanVolumeSuccess",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(),
					gomock.Any()).Return(nil).Times(1)

				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: existingBackendUuid, Orphaned: false,
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				backendInCache := getBackendByUuidFromCache(t, backend.BackendUUID)
				volCount := 0
				backendInCache.Volumes().Range(func(_, _ interface{}) bool {
					volCount++
					return true
				})
				require.Equal(t, 1, volCount)
				vol, ok := backendInCache.Volumes().Load("vol1")
				assert.True(t, ok)
				assert.True(t, vol.(*storage.Volume).Orphaned)
			},
		},
		{
			name: "BackendUpdateSuccess",
			newBackendConfig: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       existingBackendName,
				"protocol":          config.File,
				"volumeAccess":      "1.0.0.1",
				"debugTraceFlags":   map[string]bool{"api": true, "method": true}, // turn on debug flags
			},
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend(existingBackendName, existingBackendUuid, nil)
				addBackendsToCache(t, fakeBackend)

				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				fakeConfig, ok := backend.Config.(drivers.FakeStorageDriverConfig)
				if !ok {
					assert.Fail(t, "failed to cast backend config to FakeStorageDriverConfig")
				}
				// verify debug flags are updated
				assert.True(t, fakeConfig.DebugTraceFlags["api"])
				assert.True(t, fakeConfig.DebugTraceFlags["method"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var newBackendConfigJSON []byte
			var err error

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(o, mockStoreClient)
			}

			c := context.WithValue(ctx(), logging.ContextKeyRequestSource, tt.contextValue)
			newBackendConfigJSON, err = json.Marshal(tt.newBackendConfig)
			if err != nil {
				t.Fatal("failed to unmarshal newBackendConfig", err)
			}

			backend, err := o.UpdateBackendByBackendUUID(c, existingBackendName, string(newBackendConfigJSON), existingBackendUuid, tt.callingConfigRef)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(backend)
			}
		})
	}
}

func TestImportVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeConfig *storage.VolumeConfig
		bootstrapErr error
		setupMocks   func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller,
			mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name: "Success_ImportManaged",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalBackendVolume",
				ImportNotManaged:   false,
				StorageClass:       "gold",
				Size:               "100",
				VolumeMode:         config.Filesystem,
				AccessMode:         config.ReadWriteMany,
				Protocol:           config.File,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				originalBackendVol := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "originalBackendVolume",
						Size:         "100",
					},
				}

				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")
				fakePool1.AddStorageClass("gold")
				fakeConfig := getFakeStorageDriverConfig("fakeDriver")
				fakeDriver := fakedriver.NewFakeStorageDriver(testCtx, fakeConfig)
				fakeDriver.Volumes[originalBackendVol.Config.InternalName] = fake.Volume{
					Name: "originalBackendVolume", SizeBytes: 100,
				}
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", fakeDriver)
				fakeBackend.AddStoragePool(fakePool1)
				fakePool1.SetBackend(fakeBackend)

				addStorageClassesToCache(t, fakeStorageClass)
				addBackendsToCache(t, fakeBackend)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "testVolume", result.Config.Name)
				assert.Equal(t, "testVolume", result.Config.InternalName)

				// verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				assert.NotNil(t, volume)
				assert.Equal(t, "100", volume.Config.Size)
			},
		},
		{
			name: "Success_ImportNotManaged",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalBackendVolume",
				ImportNotManaged:   true,
				StorageClass:       "gold",
				Size:               "100",
				VolumeMode:         config.Filesystem,
				AccessMode:         config.ReadWriteMany,
				Protocol:           config.File,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				originalBackendVol := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "originalBackendVolume",
						Size:         "100",
					},
				}
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalBackendVolume").Return(originalBackendVol, nil).Times(1)
				mockBackend.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(
					&storage.Volume{
						Config: &storage.VolumeConfig{
							Name:         "testVolume",
							InternalName: "originalBackendVolume",
						},
					}, nil).Times(1)
				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				fakePool1.SetBackend(mockBackend)
				fakePool1.AddStorageClass("gold")

				addStorageClassesToCache(t, fakeStorageClass)
				addBackendsToCache(t, mockBackend)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "testVolume", result.Config.Name)
				assert.Equal(t, "originalBackendVolume", result.Config.InternalName)

				// verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name: "BootstrapError",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
			},
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "VolumeAlreadyExists",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				// Add a volume to the cache
				existingVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", InternalName: "testVolume"},
					BackendUUID: "backend-uuid1",
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, existingVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume testVolume already exists")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BackendNotFound",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend backend-uuid1 not found")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "UnknownStorageClass",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				addBackendsToCache(t, mockBackend)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "unknown storage class: gold")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BackendStorageVolumeNotFound",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(nil, errors.New("volume get error")).Times(1)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume originalVolume was not found: volume get error")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "RequestedSizeParseError",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100Gi",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalBackendVolume",
							Size:         "100",
						},
					}, nil).Times(1)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not determine requested size to import:")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "ActualSizeParseError",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalBackendVolume",
							Size:         "100Gi",
						},
					}, nil).Times(1)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not determine actual size of the volume being imported:")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "RequestedSizeGreaterThanActualSize",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalBackendVolume",
							Size:         "50",
						},
					}, nil).Times(1)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "unsupported capacity range; requested size is more than actual size")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "PVAlreadyExistsForVolume",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				// Add another volume to the cache with the same backend volume
				anotherVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "anotherVolume", InternalName: "originalVolume"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, anotherVolume)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalVolume",
							Size:         "100",
						},
					}, nil).Times(1)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "PV originalVolume already exists for volume anotherVolume")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BackendStorageClassMismatch",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")
				fakePool1.AddStorageClass("silver") // Mismatched storage class
				fakePool1.SetBackend(mockBackend)

				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						return &sync.Map{}
					}(),
				).Times(1)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalVolume",
							Size:         "100",
						},
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "storageClass gold does not match any storage pools for backend testBackend1")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "InvalidMode",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				fakePool1.AddStorageClass("gold")
				fakePool1.SetBackend(mockBackend)

				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalVolume",
							Size:         "100",
						},
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid volume mode (), access mode () or protocol ()")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BackendProtocolMismatch",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
				Protocol:           config.Block,
				AccessMode:         config.ReadWriteMany,
				VolumeMode:         config.RawBlock,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				fakePool1.AddStorageClass("gold")
				fakePool1.SetBackend(mockBackend)

				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").
					Return(&storage.VolumeExternal{
						Config: &storage.VolumeConfig{
							InternalName: "originalVolume",
							Size:         "100",
						},
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "requested volume mode (Block), access mode (ReadWriteMany), protocol (block) are incompatible with the backend testBackend1")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "NoBackendSpecified",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "",
				ImportOriginalName: "originalVolume",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "no backend specified for import")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "OriginalNameNotSpecified",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "original name not specified")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "AddVolumeTransactionError",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(
					errors.New("transaction error")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "failed to add volume transaction")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "ImportVolumeError_CleanupSucceeds",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
				VolumeMode:         config.Filesystem,
				AccessMode:         config.ReadWriteMany,
				Protocol:           config.File,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				originalBackendVol := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "originalVolume",
						Size:         "100",
					},
				}
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").Return(originalBackendVol, nil).Times(1)
				mockBackend.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(
					nil, errors.New("volume import error in backend")).Times(1)

				volConfig.InternalName = "testVolume" // setting this because of trident managed import

				// Make sure the cleanup routine calls volume rename and remove cached volume
				mockBackend.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").Return(nil).Times(1)
				mockBackend.EXPECT().RemoveCachedVolume("testVolume").Times(1)

				fakePool1.SetBackend(mockBackend)
				fakePool1.AddStorageClass("gold")

				addStorageClassesToCache(t, fakeStorageClass)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "failed to import volume originalVolume on backend backend-uuid1: volume import error in backend")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "ImportVolumeError_CleanupFailure",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
				VolumeMode:         config.Filesystem,
				AccessMode:         config.ReadWriteMany,
				Protocol:           config.File,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				originalBackendVol := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "originalVolume",
						Size:         "100",
					},
				}
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").Return(originalBackendVol, nil).Times(1)
				mockBackend.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(
					nil, errors.New("volume import error in backend")).Times(1)

				volConfig.InternalName = "testVolume" // setting this because of trident managed import

				// Make sure the cleanup routine calls volume rename and fails
				mockBackend.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").
					Return(errors.New("failed to rename volume")).Times(1)
				mockBackend.EXPECT().RemoveCachedVolume("testVolume").Times(1)

				fakePool1.SetBackend(mockBackend)
				fakePool1.AddStorageClass("gold")

				addStorageClassesToCache(t, fakeStorageClass)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "failed to import volume originalVolume on backend backend-uuid1: volume import error in backend")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "AddVolumeToStoreError_CleanupSuccess",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
				VolumeMode:         config.Filesystem,
				AccessMode:         config.ReadWriteMany,
				Protocol:           config.File,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				originalBackendVol := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "originalVolume",
						Size:         "100",
					},
				}
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").Return(originalBackendVol, nil).Times(1)
				mockBackend.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(
					&storage.Volume{
						Config: &storage.VolumeConfig{
							Name:         "testVolume",
							InternalName: "testVolume",
						},
					}, nil).Times(1)

				volConfig.InternalName = "testVolume" // setting this because of trident managed import

				// Make sure the cleanup routine calls volume rename and remove cached volume
				mockBackend.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").Return(nil).Times(1)
				mockBackend.EXPECT().RemoveCachedVolume("testVolume").Times(1)

				fakePool1.SetBackend(mockBackend)
				fakePool1.AddStorageClass("gold")

				addStorageClassesToCache(t, fakeStorageClass)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(errors.New("failed to add volume to persistence")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "failed to persist imported volume data: failed to add volume to persistence")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "HandlePreviouslyFailedImportTxn_Success",
			volumeConfig: &storage.VolumeConfig{
				Name:               "testVolume",
				ImportBackendUUID:  "backend-uuid1",
				ImportOriginalName: "originalVolume",
				StorageClass:       "gold",
				Size:               "100",
				VolumeMode:         config.Filesystem,
				AccessMode:         config.ReadWriteMany,
				Protocol:           config.File,
			},
			bootstrapErr: nil,
			setupMocks: func(volConfig *storage.VolumeConfig, mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				originalBackendVol := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "originalVolume",
						Size:         "100",
					},
				}
				existingVolTxn := &storage.VolumeTransaction{
					Op: storage.ImportVolume,
					Config: &storage.VolumeConfig{
						Name:               "testVolume",
						InternalName:       "testVolume",
						ImportOriginalName: "originalVolume",
						ImportBackendUUID:  "backend-uuid1",
					},
				}
				fakeStorageClass := getFakeStorageClass("gold")
				fakePool1 := storage.NewStoragePool(nil, "pool1")

				mockBackend1 := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")
				mockBackend3 := getMockBackend(mockCtrl, "testBackend3", "backend-uuid3")
				mockBackend4 := getMockBackend(mockCtrl, "testBackend4", "backend-uuid4")

				mockBackend1.EXPECT().GetVolumeForImport(gomock.Any(), "originalVolume").Return(originalBackendVol, nil).Times(1)
				mockBackend1.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(
					&storage.Volume{
						Config: &storage.VolumeConfig{
							Name:         "testVolume",
							InternalName: "testVolume",
						},
					}, nil).Times(1)

				volConfig.InternalName = "testVolume" // setting this because of trident managed import

				mockBackend1.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").Return(nil).Times(1)

				fakePool1.SetBackend(mockBackend1)
				fakePool1.AddStorageClass("gold")

				addStorageClassesToCache(t, fakeStorageClass)
				addBackendsToCache(t, mockBackend2, mockBackend3, mockBackend4, mockBackend1)

				mockBackend1.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				mockBackend2.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				mockBackend3.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				mockBackend4.EXPECT().StoragePools().Return(
					func() *sync.Map {
						m := sync.Map{}
						m.Store("pool1", fakePool1)
						return &m
					}(),
				).Times(1)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(existingVolTxn, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Nil(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Equal(t, result.Config.Name, "testVolume")

				// verify the volume is in the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				assert.NotNil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(tt.volumeConfig, mockCtrl, mockStoreClient, o)
			}

			result, err := o.ImportVolume(testCtx, tt.volumeConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestAddBackendConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		configJSON   string
		configRef    string
		setupMocks   func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError  func(err error)
		verifyResult func(backend *storage.BackendExternal)
	}{
		{
			name:       "NewBackendSuccess",
			configJSON: `{"backendName": "backened1", "storageDriverName": "fake", "version": 1, "protocol": "file"}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(backend *storage.BackendExternal) {
				assert.Equal(t, "backened1", backend.Name)
				assert.Equal(t, config.File, backend.Protocol)

				assert.NotNil(t, getBackendByNameFromCache(t, "backened1"))
			},
		},
		{
			name:       "MissingBackendName",
			configJSON: `{"storageDriverName": "fake", "version": 1, "protocol": "file"}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend name cannot be empty")
			},
		},
		{
			name:       "InvalidConfigFormat",
			configJSON: `{"backendName": "backened1",`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid config format")
			},
		},
		{
			name:       "MissingStorageDriverName",
			configJSON: `{"backendName": "backened1", "version": 1, "protocol": "file"}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "missing storage driver name in configuration file")
			},
		},
		{
			name:       "EmptyCredentialName",
			configJSON: `{"backendName": "backened1", "version": 1, "storageDriverName": "fake", "protocol": "file", credentials: {"type": "secret", "name": ""}}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "credentials `name` field cannot be empty")
			},
		},
		{
			name:       "CredentialNotFound",
			configJSON: `{"backendName": "backened1", "version": 1, "storageDriverName": "fake", "protocol": "file", credentials: {"type": "secret", "name": "cred1"}}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().GetBackendSecret(gomock.Any(), "cred1").Return(nil, nil)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend credentials not found")
			},
		},
		{
			name:       "AlreadyExistsUpdateWorkflow",
			configJSON: `{"backendName": "existingBackend", "storageDriverName": "fake", "version": 1, "protocol": "file", "volumeAccess": "1.0.0.1"}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				addBackendsToCache(t, getFakeBackend("existingBackend", "uuid1", nil))
				mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Times(0)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:       "PersistentStoreFailure",
			configJSON: `{"backendName": "existingBackend", "storageDriverName": "fake", "version": 1, "protocol": "file", "volumeAccess": "1.0.0.1"}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(errors.New("persistent store error"))
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "persistent store error")
			},
		},
		// add test cases for  handling bootstrap error
		{
			name:       "BootstrapError",
			configJSON: `{"backendName": "backened1", "storageDriverName": "fake", "version": 1, "protocol": "file"}`,
			configRef:  "",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient) {
				o.bootstrapError = errors.New("bootstrap error")
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(backend *storage.BackendExternal) {
				assert.Nil(t, backend)
				// Additionally verify the backend is not added to the cache
				assert.Nil(t, getBackendByNameFromCache(t, "backened1"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(t, o, mockStoreClient)
			}

			backend, err := o.AddBackend(context.TODO(), tt.configJSON, tt.configRef)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(backend)
			}
		})
	}
}

// TestUpdateBackendStateConcurrentCore covers public UpdateBackendState API.
func TestUpdateBackendStateConcurrentCore(t *testing.T) {
	tests := []struct {
		name             string
		backendName      string
		backendState     string
		userBackendState string
		ctx              context.Context
		setup            func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mockCtrl *gomock.Controller)
		verify           func(t *testing.T, o *ConcurrentTridentOrchestrator, ext *storage.BackendExternal, err error)
	}{
		{
			name:             "BootstrapError",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				o.bootstrapError = errors.New("bootstrap error")

				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:             "LockError",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              expiredCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:             "StoreError",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(failed)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:             "BothStatesSet",
			backendName:      "b1",
			backendState:     "failed",
			userBackendState: "offline",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:        "NeitherStateSet",
			backendName: "b1",
			ctx:         testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:         "BackendNotFound",
			backendName:  "missing",
			backendState: "failed",
			ctx:          testCtx,
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
				assert.True(t, errors.IsNotFoundError(err), "expected not found error")
			},
		},
		{
			name:         "InvalidBackendState",
			backendName:  "b1",
			backendState: "online",
			ctx:          testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:         "SuccessBackendFailed",
			backendName:  "b1",
			backendState: "failed",
			ctx:          testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, o *ConcurrentTridentOrchestrator, ext *storage.BackendExternal, err error) {
				require.NoError(t, err)
				require.NotNil(t, ext)
				assert.Equal(t, storage.Failed, ext.State)
			},
		},
		{
			name:             "SuccessUserBackendSuspended",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, o *ConcurrentTridentOrchestrator, ext *storage.BackendExternal, err error) {
				require.NoError(t, err)
				require.NotNil(t, ext)
				// Validate cached backend user state.
				b := getBackendByNameFromCache(t, "b1")
				require.NotNil(t, b)
				assert.Equal(t, storage.UserSuspended, b.UserState())
			},
		},
		{
			name:             "UserBackendInvalid",
			backendName:      "b1",
			userBackendState: "bogus-state",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				backend := getFakeBackend("b1", "uuid-b1", nil)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:             "BackendUserStateConfiguredConfigRefSet",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				fakeConfig := getFakeStorageDriverConfig("b1")
				fakeConfig.UserState = string(storage.UserSuspended)
				backend := getFakeBackendWithConfig("b1", "uuid-b1", nil, fakeConfig)
				backend.SetConfigRef("fake-ref")

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, _ *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
		{
			name:             "BackendUserStateConfiguredConfigRefNotSet",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				fakeConfig := getFakeStorageDriverConfig("b1")
				fakeConfig.UserState = string(storage.UserSuspended)
				backend := getFakeBackendWithConfig("b1", "uuid-b1", nil, fakeConfig)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, ext *storage.BackendExternal, err error) {
				require.NoError(t, err)
				require.NotNil(t, ext)

				// Validate cached backend user state.
				b := getBackendByNameFromCache(t, "b1")
				require.NotNil(t, b)
				assert.Equal(t, storage.UserSuspended, b.UserState())
			},
		},
		{
			name:             "BackendUserStateIdempotent",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				fakeConfig := getFakeStorageDriverConfig("b1")
				fakeConfig.UserState = string(storage.UserSuspended)
				backend := getFakeBackendWithConfig("b1", "uuid-b1", nil, fakeConfig)
				backend.SetUserState(storage.UserSuspended)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, ext *storage.BackendExternal, err error) {
				require.NoError(t, err)
				require.NotNil(t, ext)

				// Validate cached backend user state.
				b := getBackendByNameFromCache(t, "b1")
				require.NotNil(t, b)
				assert.Equal(t, storage.UserSuspended, b.UserState())
			},
		},
		{
			name:             "BackendUserStateBackendDeleting",
			backendName:      "b1",
			userBackendState: "suspended",
			ctx:              testCtx,
			setup: func(t *testing.T, o *ConcurrentTridentOrchestrator, mockStoreClient *mockpersistentstore.MockStoreClient, mc *gomock.Controller) {
				fakeConfig := getFakeStorageDriverConfig("b1")
				fakeConfig.UserState = string(storage.UserSuspended)
				backend := getFakeBackendWithConfig("b1", "uuid-b1", nil, fakeConfig)
				backend.SetState(storage.Deleting)

				mockStoreClient.EXPECT().AddBackend(gomock.Any(), backend).Return(nil)

				addBackendsToCache(t, backend)
				addBackendsToPersistence(t, o, backend)
			},
			verify: func(t *testing.T, _ *ConcurrentTridentOrchestrator, ext *storage.BackendExternal, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()
			config.CurrentDriverContext = config.ContextCSI

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setup != nil {
				tt.setup(t, o, mockStoreClient, mockCtrl)
			}

			ext, err := o.UpdateBackendState(tt.ctx, tt.backendName, tt.backendState, tt.userBackendState)
			if tt.verify != nil {
				tt.verify(t, o, ext, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

// Volume tests

func TestAddVolumeConcurrentCore(t *testing.T) {
	volumeConfig := &storage.VolumeConfig{
		Name:              "vol1",
		InternalName:      "vol1",
		Size:              "1G",
		Protocol:          config.File,
		StorageClass:      "gold",
		ShareSourceVolume: "",
		VolumeMode:        config.Filesystem,
		AccessMode:        config.ReadWriteMany,
		AccessInfo: models.VolumeAccessInfo{
			NfsAccessInfo: models.NfsAccessInfo{
				NfsPath: "10.10.10.10/path",
			},
		},
	}
	fakePool1 := storage.NewStoragePool(nil, "pool1")
	fakePool2 := storage.NewStoragePool(nil, "pool2")
	fakeStorageClass := getFakeStorageClass("gold")

	otherVolume := &storage.Volume{
		Config:      volumeConfig,
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	tests := []struct {
		name         string
		volumeConfig *storage.VolumeConfig
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name:         "SuccessOnePool",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), fakePool1)
				mockBackend.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					&storage.Volume{
						Config:      &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
						BackendUUID: "backend-uuid1",
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "vol1", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name: "BootstrapError",
			volumeConfig: &storage.VolumeConfig{
				Name: "vol1", InternalName: "vol1",
			},
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "VolumeAlreadyExists",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)
				addVolumesToCache(t, otherVolume)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume remains in the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "VolumeIsSubordinate",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)
				addSubordinateVolumesToCache(t, otherVolume)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name: "InvalidProtocol",
			volumeConfig: func() *storage.VolumeConfig {
				c := *volumeConfig
				c.VolumeMode = ""
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "FailedAddTransaction",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "NoAvailableBackendsBeforeFiltering",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name: "NoAvailableBackendsAfterFiltering",
			volumeConfig: func() *storage.VolumeConfig {
				c := *volumeConfig
				c.Protocol = config.Block
				c.AccessMode = config.ReadWriteOnce
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name: "MirroringNotSupported",
			volumeConfig: func() *storage.VolumeConfig {
				c := *volumeConfig
				c.IsMirrorDestination = true
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()
				mockBackend.EXPECT().CanMirror().Return(false).AnyTimes()

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "FailedTransactionUpdate",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1, "pool2": fakePool2})).AnyTimes()
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "FailedCreateThenSuccess",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1, "pool2": fakePool2})).AnyTimes()
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)
				mockBackend.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, failed)
				mockBackend.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					&storage.Volume{
						Config:      &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
						BackendUUID: "backend-uuid1",
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "VolumeCreatingErrorThenSuccess",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1, "pool2": fakePool2})).AnyTimes()
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				mockBackend.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, errors.VolumeCreatingError("creating"))
				mockBackend.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					&storage.Volume{
						Config:      &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
						BackendUUID: "backend-uuid1",
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.AddVolume(testCtx, tt.volumeConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestAddVolumeMultiBackendConcurrentCore(t *testing.T) {
	volumeConfig := &storage.VolumeConfig{
		Name:              "vol1",
		InternalName:      "vol1",
		Size:              "1G",
		Protocol:          config.File,
		StorageClass:      "gold",
		ShareSourceVolume: "",
		VolumeMode:        config.Filesystem,
		AccessMode:        config.ReadWriteMany,
		AccessInfo: models.VolumeAccessInfo{
			NfsAccessInfo: models.NfsAccessInfo{
				NfsPath: "10.10.10.10/path",
			},
		},
	}
	fakePool1a := storage.NewStoragePool(nil, "pool1a")
	fakePool1b := storage.NewStoragePool(nil, "pool1b")
	fakePool2a := storage.NewStoragePool(nil, "pool2a")
	fakePool2b := storage.NewStoragePool(nil, "pool2b")

	fakeStorageClass := getFakeStorageClass("gold")

	tests := []struct {
		name         string
		volumeConfig *storage.VolumeConfig
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name:         "SuccessFirstTry",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend1").AnyTimes()
				mockBackend1.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1a": fakePool1a, "pool1b": fakePool1b})).AnyTimes()
				mockBackend1.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				mockBackend1.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					&storage.Volume{
						Config:      &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
						BackendUUID: "backend-uuid1",
					}, nil).AnyTimes()

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()
				mockBackend2.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool2a": fakePool2a, "pool2b": fakePool2b})).AnyTimes()
				mockBackend2.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				mockBackend2.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					&storage.Volume{
						Config:      &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
						BackendUUID: "backend-uuid1",
					}, nil).AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1a.SetBackend(mockBackend1)
				fakePool1b.SetBackend(mockBackend1)
				fakePool2a.SetBackend(mockBackend2)
				fakePool2b.SetBackend(mockBackend2)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "vol1", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "SuccessAfterBackendIneligibleError",
			volumeConfig: volumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend1").AnyTimes()
				mockBackend1.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1a": fakePool1a, "pool1b": fakePool1b})).AnyTimes()
				mockBackend1.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				mockBackend1.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					nil, drivers.NewBackendIneligibleError("vol1", []error{failed}, []string{"pool1a", "pool1b"})).AnyTimes()

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()
				mockBackend2.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool2a": fakePool2a, "pool2b": fakePool2b})).AnyTimes()
				mockBackend2.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				mockBackend2.EXPECT().AddVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(
					&storage.Volume{
						Config:      &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
						BackendUUID: "backend-uuid1",
					}, nil).AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addStorageClassesToCache(t, fakeStorageClass)

				fakePool1a.SetBackend(mockBackend1)
				fakePool1b.SetBackend(mockBackend1)
				fakePool2a.SetBackend(mockBackend2)
				fakePool2b.SetBackend(mockBackend2)
				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "vol1", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.AddVolume(testCtx, tt.volumeConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestAddSubordinateVolumeConcurrentCore(t *testing.T) {
	volumeConfig := &storage.VolumeConfig{
		Name:              "vol1",
		InternalName:      "vol1",
		Size:              "1G",
		Protocol:          config.File,
		StorageClass:      "gold",
		ShareSourceVolume: "",
		VolumeMode:        config.Filesystem,
		AccessMode:        config.ReadWriteMany,
		AccessInfo: models.VolumeAccessInfo{
			NfsAccessInfo: models.NfsAccessInfo{
				NfsPath: "10.10.10.10/path",
			},
		},
	}

	subvolumeConfig := &storage.VolumeConfig{
		Name:              "subvol1",
		InternalName:      "subvol1",
		Size:              "1G",
		ShareSourceVolume: "vol1",
		StorageClass:      "gold",
	}

	parentVolume := &storage.Volume{
		Config:      volumeConfig,
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	tests := []struct {
		name         string
		volumeConfig *storage.VolumeConfig
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name:         "AddSubordinateVolume",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// addSubordinateVolume will be called, so you can mock it if needed
				// For this example, we just add a backend to avoid nil deref
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, parentVolume)

				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "subvol1", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "NonSubordinateVolumeAlreadyExists",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolumeConfig := &storage.VolumeConfig{
					Name:              "subvol1",
					InternalName:      "subvol1",
					Size:              "1G",
					Protocol:          config.File,
					StorageClass:      "gold",
					ShareSourceVolume: "",
					VolumeMode:        config.Filesystem,
					AccessMode:        config.ReadWriteMany,
					AccessInfo: models.VolumeAccessInfo{
						NfsAccessInfo: models.NfsAccessInfo{
							NfsPath: "10.10.10.10/path",
						},
					},
				}

				otherVolume := &storage.Volume{
					Config:      otherVolumeConfig,
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)
			},
		},
		{
			name:         "SubordinateVolumeAlreadyExists",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherSubvolume := &storage.Volume{
					Config:      subvolumeConfig,
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateSubordinate,
				}

				addBackendsToCache(t, mockBackend)
				addSubordinateVolumesToCache(t, otherSubvolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume remains in the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name: "InvalidSize",
			volumeConfig: func() *storage.VolumeConfig {
				c := *subvolumeConfig
				c.Size = "asdf"
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name: "CloneSourceVolume",
			volumeConfig: func() *storage.VolumeConfig {
				c := *subvolumeConfig
				c.CloneSourceVolume = "vol1"
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name: "IsMirrorDestination",
			volumeConfig: func() *storage.VolumeConfig {
				c := *subvolumeConfig
				c.IsMirrorDestination = true
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name: "ImportOriginalName",
			volumeConfig: func() *storage.VolumeConfig {
				c := *subvolumeConfig
				c.ImportOriginalName = "vol1"
				return &c
			}(),
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "NoSourceVolume",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceImportNotManaged",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config: func() *storage.VolumeConfig {
						c := *volumeConfig
						c.ImportNotManaged = true
						return &c
					}(),
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "StorageClassMismatch",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config: func() *storage.VolumeConfig {
						c := *volumeConfig
						c.StorageClass = "silver"
						return &c
					}(),
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceOrphaned",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config:      volumeConfig,
					BackendUUID: "backend-uuid1",
					Orphaned:    true,
					State:       storage.VolumeStateMissingBackend,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceNotOnline",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config:      volumeConfig,
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "NoSourceVolumeBackend",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config:      volumeConfig,
					BackendUUID: "",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceVolumeNotNFS",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config: func() *storage.VolumeConfig {
						c := *volumeConfig
						c.AccessInfo.NfsPath = ""
						return &c
					}(),
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceVolumeInvalidSize",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config: func() *storage.VolumeConfig {
						c := *volumeConfig
						c.Size = "invalid"
						return &c
					}(),
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceVolumeTooSmall",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				otherVolume := &storage.Volume{
					Config: func() *storage.VolumeConfig {
						c := *volumeConfig
						c.Size = "500Mi"
						return &c
					}(),
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, otherVolume)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "AddSubordinateVolumeFailed",
			volumeConfig: subvolumeConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// addSubordinateVolume will be called, so you can mock it if needed
				// For this example, we just add a backend to avoid nil deref
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, parentVolume)

				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.AddVolume(testCtx, tt.volumeConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestUpdateVolumeLUKSPassphraseNamesConcurrentCore(t *testing.T) {
	tests := []struct {
		name            string
		volumeName      string
		passphraseNames *[]string
		bootstrapErr    error
		setupMocks      func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError     func(t *testing.T, err error)
	}{
		{
			name:            "BootstrapError",
			volumeName:      "vol1",
			passphraseNames: &[]string{"key1"},
			bootstrapErr:    errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:            "Success",
			volumeName:      "vol1",
			passphraseNames: &[]string{"key1", "key2"},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:            "VolumeNotFound",
			volumeName:      "nonexistent",
			passphraseNames: &[]string{"key1"},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "volume nonexistent not found")
			},
		},
		{
			name:            "NilPassphraseNames",
			volumeName:      "vol1",
			passphraseNames: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				addVolumesToCache(t, vol)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:            "PersistenceError",
			volumeName:      "vol1",
			passphraseNames: &[]string{"key1"},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(errors.New("persistence error")).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.UpdateVolumeLUKSPassphraseNames(testCtx, tt.volumeName, tt.passphraseNames)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestUpdateVolumeAutogrowStatusConcurrentCore(t *testing.T) {
	now := time.Now()
	statusSuccess := &models.VolumeAutogrowStatus{
		LastAutogrowPolicyUsed:   "policy1",
		LastAutogrowAttemptedAt:  &now,
		LastProposedSize:         "100Gi",
		TotalAutogrowAttempted:   1,
		TotalSuccessfulAutogrow:  1,
		LastSuccessfulAutogrowAt: &now,
		LastSuccessfulSize:       "100Gi",
		LastError:                "",
	}

	tests := []struct {
		name          string
		volumeName    string
		status        *models.VolumeAutogrowStatus
		bootstrapErr  error
		useExpiredCtx bool
		setupMocks    func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError   func(t *testing.T, err error)
	}{
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			status:       statusSuccess,
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks:   nil,
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "bootstrap error")
			},
		},
		{
			name:       "Success_MainVolume",
			volumeName: "vol1",
			status:     statusSuccess,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, v *storage.Volume) error {
						assert.Equal(t, "vol1", v.Config.Name)
						require.NotNil(t, v.AutogrowStatus)
						assert.Equal(t, "policy1", v.AutogrowStatus.LastAutogrowPolicyUsed)
						assert.Equal(t, 1, v.AutogrowStatus.TotalAutogrowAttempted)
						return nil
					}).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:       "Success_SubordinateVolume",
			volumeName: "subvol1",
			status:     statusSuccess,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:      "subvol1",
						Name:              "subvol1",
						ShareSourceVolume: "vol1",
					},
					BackendUUID: "backend-uuid1",
				}
				addSubordinateVolumesToCache(t, subVol)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, v *storage.Volume) error {
						assert.Equal(t, "subvol1", v.Config.Name)
						require.NotNil(t, v.AutogrowStatus)
						assert.Equal(t, "policy1", v.AutogrowStatus.LastAutogrowPolicyUsed)
						return nil
					}).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent",
			status:     statusSuccess,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No volumes in cache
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "was not found")
			},
		},
		{
			name:       "PersistenceError_MainVolume",
			volumeName: "vol1",
			status:     statusSuccess,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(errors.New("persistence error")).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "persistence error")
			},
		},
		{
			name:       "PersistenceError_SubordinateVolume",
			volumeName: "subvol1",
			status:     statusSuccess,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:      "subvol1",
						Name:              "subvol1",
						ShareSourceVolume: "vol1",
					},
					BackendUUID: "backend-uuid1",
				}
				addSubordinateVolumesToCache(t, subVol)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(errors.New("store update failed")).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "store update failed")
			},
		},
		{
			name:       "NilStatus_MainVolume",
			volumeName: "vol1",
			status:     nil,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, v *storage.Volume) error {
						assert.Nil(t, v.AutogrowStatus)
						return nil
					}).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:          "ContextCancelled",
			volumeName:    "vol1",
			status:        statusSuccess,
			useExpiredCtx: true,
			setupMocks: func(t *testing.T, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
				// No UpdateVolume expectation - expired context may cause Lock to fail or short-circuit
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(t, mockStoreClient, o)
			}

			ctx := testCtx
			if tt.useExpiredCtx {
				ctx = expiredCtx
			}
			err := o.UpdateVolumeAutogrowStatus(ctx, tt.volumeName, tt.status)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestCloneVolumeConcurrentCore(t *testing.T) {
	sourceVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName: "sourceVolume",
			Name:         "sourceVolume",
			Size:         "1073741824",
			VolumeMode:   config.Filesystem,
		},
		BackendUUID: "backend-uuid",
	}

	sourceSnapshot := &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			Name:                "sourceSnapshot",
			InternalName:        "sourceSnapshot",
			VolumeName:          "sourceVolume",
			VolumeInternalName:  "sourceVolume",
			LUKSPassphraseNames: []string{"passphrase1"},
		},
	}

	cloneVolumeConfig := &storage.VolumeConfig{
		InternalName:              "cloneVolume",
		Name:                      "cloneVolume",
		Size:                      "1073741824",
		VolumeMode:                config.Filesystem,
		CloneSourceVolume:         "sourceVolume",
		CloneSourceVolumeInternal: "sourceVolume",
	}

	cloneVolume := &storage.Volume{
		Config:      cloneVolumeConfig,
		BackendUUID: "backend-uuid",
	}

	fakePool1 := storage.NewStoragePool(nil, "pool1")

	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		volumeConfig *storage.VolumeConfig
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
			},
			volumeConfig: &storage.VolumeConfig{
				Name:              "cloneVol",
				VolumeMode:        config.Filesystem,
				CloneSourceVolume: "sourceVol",
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "cloneVolume", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "VolumeAlreadyExists",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume, cloneVolume)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume remains in the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "VolumeAlreadyExistsAsSubordinate",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)
				addSubordinateVolumesToCache(t, cloneVolume)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume remains in the cache
				volume := getSubordinateVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "SourceVolumeExistsAsSubordinate",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addSubordinateVolumesToCache(t, sourceVolume)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SourceVolumeNotFound",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "BackendNotFound",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SuccessNoCloneSizeSpecified", // Docker
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				VolumeMode:                config.Filesystem,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "cloneVolume", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "InvalidCloneSourceSize",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				invalidSourceVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName: "sourceVolume",
						Name:         "sourceVolume",
						Size:         "1G",
						VolumeMode:   config.Filesystem,
					},
					BackendUUID: "backend-uuid",
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, invalidSourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "InvalidCloneSize",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				Size:                      "1G",
				VolumeMode:                config.Filesystem,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "CloneSizeTooLarge",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				Size:                      "1073741825", // 1GB + 1 byte
				VolumeMode:                config.Filesystem,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SuccessOrphanedSourceVolume",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil).Times(1)

				orphanedSourceVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName: "sourceVolume",
						Name:         "sourceVolume",
						Size:         "1073741824",
						VolumeMode:   config.Filesystem,
					},
					BackendUUID: "backend-uuid",
					Orphaned:    true,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, orphanedSourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "cloneVolume", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "VolumeModeMismatch",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				Size:                      "1073741824",
				VolumeMode:                config.RawBlock,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "MissingCloneSourceSnapshot",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:                "cloneVolume",
				Name:                        "cloneVolume",
				Size:                        "1073741824",
				VolumeMode:                  config.Filesystem,
				CloneSourceVolume:           "sourceVolume",
				CloneSourceVolumeInternal:   "sourceVolume",
				CloneSourceSnapshot:         "sourceSnapshot",
				CloneSourceSnapshotInternal: "sourceSnapshot",
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SuccessSourceSnapshot",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				Size:                      "1073741824",
				VolumeMode:                config.Filesystem,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
				CloneSourceSnapshot:       "sourceSnapshot",
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "cloneVolume", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "UnmanagedSourceVolumeWithoutSnapshot",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				unmanagedSourceVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:     "sourceVolume",
						Name:             "sourceVolume",
						Size:             "1073741824",
						VolumeMode:       config.Filesystem,
						ImportNotManaged: true,
					},
					BackendUUID: "backend-uuid",
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, unmanagedSourceVolume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "SuccessSourceVirtualPoolKnown",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), fakePool1)
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), fakePool1, false).Return(cloneVolume, nil).Times(1)

				sourceVolumeWithPool := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:     "sourceVolume",
						Name:             "sourceVolume",
						Size:             "1073741824",
						VolumeMode:       config.Filesystem,
						ImportNotManaged: true,
					},
					BackendUUID: "backend-uuid",
					Pool:        "pool1",
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolumeWithPool)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				Size:                      "1073741824",
				VolumeMode:                config.Filesystem,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
				CloneSourceSnapshot:       "sourceSnapshot",
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "cloneVolume", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "VolumeCreatingErrorThenSuccess",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, errors.VolumeCreatingError("creating"))
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "cloneVolume", result.Config.Name)

				// Additionally verify the volume is added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.NotNil(t, volume)
			},
		},
		{
			name:         "CloneVolumeFailed",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, failed)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "PersistenceFailed",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil)
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "PersistenceFailedCleanupFailed",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().CreatePrepare(gomock.Any(), gomock.Any(), gomock.Any())
				mockBackend.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(cloneVolume, nil)
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(failed)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolume)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			volumeConfig: cloneVolumeConfig,
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)

				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "CloneVolumeFromDifferentStorageClassFailed",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")

				sourceVolumeDifferentStorageClass := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName: "sourceVolume",
						Name:         "sourceVolume",
						Size:         "1073741824",
						VolumeMode:   config.Filesystem,
						StorageClass: "test-storage-class",
					},
					BackendUUID: "backend-uuid",
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, sourceVolumeDifferentStorageClass)
				addSnapshotsToCache(t, sourceSnapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			volumeConfig: &storage.VolumeConfig{
				InternalName:              "cloneVolume",
				Name:                      "cloneVolume",
				Size:                      "1073741824",
				VolumeMode:                config.Filesystem,
				CloneSourceVolume:         "sourceVolume",
				CloneSourceVolumeInternal: "sourceVolume",
				StorageClass:              "test-different-storage-class",
			},
			verifyError: func(err error) {
				assert.True(t, errors.IsMismatchedStorageClassError(err))
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.Nil(t, result)
				// Additionally verify the volume is not added to the cache
				volume := getVolumeByNameFromCache(t, "cloneVolume")
				assert.Nil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.CloneVolume(testCtx, tt.volumeConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteVolumeConcurrentCore(t *testing.T) {
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subVol := &storage.Volume{
		Config:      &storage.VolumeConfig{InternalName: "subvol1", Name: "subvol1", ShareSourceVolume: "vol1"},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	tests := []struct {
		name         string
		volumeName   string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(t *testing.T, err error)
	}{
		{
			name:       "Success",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks:   nil,
			verifyError: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(t *testing.T, err error) {
				assert.True(t, errors.IsNotFoundError(err))
			},
		},
		{
			name:       "OrphanedVolumeWarns",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				orphanedVol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					Orphaned:    true,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, orphanedVol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), orphanedVol).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "SoftDeleteWithSnapshots",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				snap := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snap1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snap)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "SoftDeleteWithSubordinateVolumes",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "SoftDeleteUpdateFailed",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "MissingBackend",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "MissingBackendPersistenceFailed",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "VolumeDeleteFailed",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "VolumeNotManagedPersistenceFailed",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(errors.NotManagedError("not managed")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "VolumeNotManaged",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(errors.NotManagedError("not managed")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "PersistenceFailed",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "DeletingBackendDeleted",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Deleting),
					"driverName": "ontap-nas",
				})
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(false).AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().Terminate(gomock.Any()).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "DeletingBackendDeleteFailedWarning",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Deleting),
					"driverName": "ontap-nas",
				})
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(false).AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "DeletingBackendHasVolumes",
			volumeName: "vol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Deleting),
					"driverName": "ontap-nas",
				})
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(true).AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.DeleteVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteSubordinateVolumeConcurrentCore(t *testing.T) {
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:       "vol1",
			Name:               "vol1",
			SubordinateVolumes: map[string]interface{}{"subvol1": nil},
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subVol := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subvol1",
			Name:              "subvol1",
			ShareSourceVolume: "vol1",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	subVol2 := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subvol2",
			Name:              "subvol2",
			ShareSourceVolume: "vol1",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	tests := []struct {
		name         string
		volumeName   string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(t *testing.T, err error)
	}{
		{
			name:       "Success",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "PersistenceFailed",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)

				// Additionally verify the volume is not removed from the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.NotNil(t, volume)
			},
		},
		{
			name:       "NoSourceVolume",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the volume is removed from the cache
				volume := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "DeletingSourceVolumeDeleted",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				deletingVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:       "vol1",
						Name:               "vol1",
						SubordinateVolumes: map[string]interface{}{"subvol1": nil},
					},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, deletingVol)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the source volume is removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.Nil(t, volume)

				// Additionally verify the volume is removed from the cache
				volume = getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "DeletingSourceVolumePreservedBySubordinate",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				deletingVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:       "vol1",
						Name:               "vol1",
						SubordinateVolumes: map[string]interface{}{"subvol1": nil, "subvol2": nil},
					},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, deletingVol)
				addSubordinateVolumesToCache(t, subVol, subVol2)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the source volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)

				// Additionally verify the volume is removed from the cache
				volume = getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "DeletingSourceVolumePreservedBySnapshot",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				deletingVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:       "vol1",
						Name:               "vol1",
						SubordinateVolumes: map[string]interface{}{"subvol1": nil},
					},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				snap := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snap1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, deletingVol)
				addSnapshotsToCache(t, snap)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the source volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)

				// Additionally verify the volume is removed from the cache
				volume = getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
		{
			name:       "DeletingSourceVolumePreservedBySnapshotUpdateFailed",
			volumeName: "subvol1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				deletingVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:       "vol1",
						Name:               "vol1",
						SubordinateVolumes: map[string]interface{}{"subvol1": nil},
					},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				snap := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snap1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, deletingVol)
				addSnapshotsToCache(t, snap)
				addSubordinateVolumesToCache(t, subVol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), subVol).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)

				// Additionally verify the source volume is not removed from the cache
				volume := getVolumeByNameFromCache(t, "vol1")
				assert.NotNil(t, volume)

				// Additionally verify the volume is removed from the cache
				volume = getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.Nil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.DeleteVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetVolumeForImportConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeID     string
		backendName  string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator, mockCtrl *gomock.Controller)
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name:        "Success",
			volumeID:    "vol1",
			backendName: "backend1",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "backend1", "backend-uuid1")
				expectedVol := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "vol1"}}
				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "vol1").Return(expectedVol, nil).Times(1)
				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "vol1", result.Config.Name)
			},
		},
		{
			name:        "BackendNotFound",
			volumeID:    "vol1",
			backendName: "nonexistent-backend",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockCtrl *gomock.Controller) {
				// No backends added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonexistent-backend was not found")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BootstrapError",
			volumeID:     "vol1",
			backendName:  "backend1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockCtrl *gomock.Controller) {
				// No backends added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:        "BackendReturnsError",
			volumeID:    "vol1",
			backendName: "backend1",
			setupMocks: func(o *ConcurrentTridentOrchestrator, mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "backend1", "backend-uuid1")
				mockBackend.EXPECT().GetVolumeForImport(gomock.Any(), "vol1").Return(nil, errors.New("backend error")).Times(1)
				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend error")
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o, mockCtrl)
			}

			result, err := o.GetVolumeForImport(testCtx, tt.volumeID, tt.backendName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListVolumesConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storage.VolumeExternal)
	}{
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend := getFakeBackend("testBackend1", "backend-uuid1", nil)
				vol1 := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				subVol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "subvol1", Name: "subvol1", ShareSourceVolume: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, backend)
				addVolumesToCache(t, vol1)
				addSubordinateVolumesToCache(t, subVol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.VolumeExternal) {
				require.Len(t, result, 2)
				expectedNames := []string{"subvol1", "vol1"}
				actualNames := []string{}
				for _, v := range result {
					actualNames = append(actualNames, v.Config.Name)
				}
				assert.ElementsMatch(t, expectedNames, actualNames)
			},
		},
		{
			name:         "Empty",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No volumes or subordinate volumes added
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.VolumeExternal) {
				assert.Empty(t, result)
			},
		},
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.ListVolumes(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		bootstrapErr error
		setupMocks   func()
		verifyError  func(err error)
		verifyResult func(result *storage.VolumeExternal)
	}{
		{
			name:         "Success",
			volumeName:   "vol1",
			bootstrapErr: nil,
			setupMocks: func() {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "vol1", result.Config.Name)
			},
		},
		{
			name:         "SubordinateVolume",
			volumeName:   "subvol1",
			bootstrapErr: nil,
			setupMocks: func() {
				subVol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "subvol1", Name: "subvol1", ShareSourceVolume: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addSubordinateVolumesToCache(t, subVol)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "subvol1", result.Config.Name)
			},
		},
		{
			name:         "NotFound",
			volumeName:   "nonexistent",
			bootstrapErr: nil,
			setupMocks:   func() {},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsNotFoundError(err))
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks:   func() {},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			result, err := o.GetVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListSubordinateVolumesConcurrentCore(t *testing.T) {
	vol1 := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:       "vol1",
			Name:               "vol1",
			SubordinateVolumes: map[string]interface{}{"subVol1a": true, "subVol1b": true},
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subVol1a := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol1a",
			Name:              "subVol1a",
			ShareSourceVolume: "vol1",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	subVol1b := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol1b",
			Name:              "subVol1b",
			ShareSourceVolume: "vol1",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	vol2 := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:       "vol2",
			Name:               "vol2",
			SubordinateVolumes: map[string]interface{}{"subVol2a": true, "subVol2b": true},
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subVol2a := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol2a",
			Name:              "subVol2a",
			ShareSourceVolume: "vol2",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	subVol2b := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol2b",
			Name:              "subVol2b",
			ShareSourceVolume: "vol2",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	tests := []struct {
		name             string
		sourceVolumeName string
		bootstrapErr     error
		setupMocks       func(mockCtrl *gomock.Controller)
		verifyError      func(t *testing.T, err error)
		verifyResult     func(t *testing.T, result []*storage.VolumeExternal)
	}{
		{
			name:             "BootstrapError",
			sourceVolumeName: "",
			bootstrapErr:     errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller) {
				// No setup needed
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(t *testing.T, result []*storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:             "ListAllSubordinateVolumes",
			sourceVolumeName: "",
			bootstrapErr:     nil,
			setupMocks: func(mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol1, vol2)
				addSubordinateVolumesToCache(t, subVol1a, subVol1b, subVol2a, subVol2b)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(t *testing.T, result []*storage.VolumeExternal) {
				require.Len(t, result, 4)
				assert.ElementsMatch(t,
					[]string{"subVol1a", "subVol1b", "subVol2a", "subVol2b"},
					[]string{result[0].Config.Name, result[1].Config.Name, result[2].Config.Name, result[3].Config.Name},
				)
			},
		},
		{
			name:             "ListSubordinateVolumesBySource",
			sourceVolumeName: "vol1",
			bootstrapErr:     nil,
			setupMocks: func(mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol1, vol2)
				addSubordinateVolumesToCache(t, subVol1a, subVol1b, subVol2a, subVol2b)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(t *testing.T, result []*storage.VolumeExternal) {
				require.Len(t, result, 2)
				assert.ElementsMatch(t,
					[]string{"subVol1a", "subVol1b"},
					[]string{result[0].Config.Name, result[1].Config.Name},
				)
			},
		},
		{
			name:             "SourceVolumeNotFound",
			sourceVolumeName: "nonexistentVolume",
			bootstrapErr:     nil,
			setupMocks: func(mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol1, vol2)
				addSubordinateVolumesToCache(t, subVol1a, subVol1b, subVol2a, subVol2b)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NotNil(t, err)
			},
			verifyResult: func(t *testing.T, result []*storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()
			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl)
			}

			result, err := o.ListSubordinateVolumes(testCtx, tt.sourceVolumeName)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(t, result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetSubordinateSourceVolumeConcurrentCore(t *testing.T) {
	vol1 := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:       "vol1",
			Name:               "vol1",
			SubordinateVolumes: map[string]interface{}{"subVol1a": true, "subVol1b": true},
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subVol1a := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol1a",
			Name:              "subVol1a",
			ShareSourceVolume: "vol1",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	subVol1b := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol1b",
			Name:              "subVol1b",
			ShareSourceVolume: "vol1",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	vol2 := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:       "vol2",
			Name:               "vol2",
			SubordinateVolumes: map[string]interface{}{"subVol2a": true, "subVol2b": true},
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	subVol2a := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol2a",
			Name:              "subVol2a",
			ShareSourceVolume: "vol2",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	subVol2b := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:      "subVol2b",
			Name:              "subVol2b",
			ShareSourceVolume: "vol2",
		},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateSubordinate,
	}

	tests := []struct {
		name         string
		volumeName   string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller)
		verifyError  func(t *testing.T, err error)
		verifyResult func(t *testing.T, result *storage.VolumeExternal)
	}{
		{
			name:         "BootstrapError",
			volumeName:   "",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller) {
				// No setup needed
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
			verifyResult: func(t *testing.T, result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:       "Success",
			volumeName: "subVol2b",
			setupMocks: func(mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol1, vol2)
				addSubordinateVolumesToCache(t, subVol1a, subVol1b, subVol2a, subVol2b)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(t *testing.T, result *storage.VolumeExternal) {
				assert.NotNil(t, result)
				assert.Equal(t, "vol2", result.Config.Name)
			},
		},
		{
			name:       "SubordinateVolumeNotFound",
			volumeName: "subVol3a",
			setupMocks: func(mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol1, vol2)
				addSubordinateVolumesToCache(t, subVol1a, subVol1b, subVol2a, subVol2b)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
			verifyResult: func(t *testing.T, result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:       "SourceVolumeNotFound",
			volumeName: "subVol3",
			setupMocks: func(mockCtrl *gomock.Controller) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				subVol3 := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:      "subVol3",
						Name:              "subVol3",
						ShareSourceVolume: "vol3",
					},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateSubordinate,
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol1, vol2)
				addSubordinateVolumesToCache(t, subVol1a, subVol1b, subVol2a, subVol2b, subVol3)
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
			verifyResult: func(t *testing.T, result *storage.VolumeExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()
			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl)
			}

			result, err := o.GetSubordinateSourceVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(t, result)
			}

			persistenceCleanup(t, o)
		})
	}
}

// Snapshots tests

func TestListSnapshotsConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storage.SnapshotExternal)
	}{
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend := getFakeBackend("testBackend1", "backend-uuid1", nil)
				vol1 := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				vol2 := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol2", Name: "vol2"},
					BackendUUID: "backend-uuid1",
				}
				snapshot1 := &storage.Snapshot{Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"}}
				snapshot2 := &storage.Snapshot{Config: &storage.SnapshotConfig{Name: "snapshot2", VolumeName: "vol2"}}

				addBackendsToCache(t, backend)
				addVolumesToCache(t, vol1, vol2)
				addSnapshotsToCache(t, snapshot1, snapshot2)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				require.Len(t, result, 2)
				expectedNames := []string{"snapshot1", "snapshot2"}
				actualNames := []string{}
				for _, snapshot := range result {
					actualNames = append(actualNames, snapshot.Config.Name)
				}
				assert.ElementsMatch(t, expectedNames, actualNames)
			},
		},
		{
			name:         "Empty",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Empty(t, result)
			},
		},
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.ListSnapshots(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListSnapshotsByNameConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		snapshotName string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storage.SnapshotExternal)
	}{
		{
			name:         "Success",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend := getFakeBackend("testBackend1", "backend-uuid1", nil)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"}}

				addBackendsToCache(t, backend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				require.Len(t, result, 1)
				assert.Equal(t, "snapshot1", result[0].Config.Name)
			},
		},
		{
			name:         "Empty",
			snapshotName: "nonexistentSnapshot",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Empty(t, result)
			},
		},
		{
			name:         "BootstrapError",
			snapshotName: "snapshot1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.ListSnapshotsByName(testCtx, tt.snapshotName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListSnapshotsForVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		bootstrapErr error
		setupMocks   func(o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storage.SnapshotExternal)
	}{
		{
			name:         "Success",
			volumeName:   "vol1",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				backend := getFakeBackend("testBackend1", "backend-uuid1", nil)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"}}

				addBackendsToCache(t, backend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				require.Len(t, result, 1)
				assert.Equal(t, "snapshot1", result[0].Config.Name)
			},
		},
		{
			name:         "VolumeNotFound",
			volumeName:   "nonexistentVolume",
			bootstrapErr: nil,
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume nonexistentVolume not found")
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(o)
			}

			result, err := o.ListSnapshotsForVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestReadSnapshotsForVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storage.SnapshotExternal)
	}{
		{
			name:         "Success",
			volumeName:   "vol1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"}}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetSnapshots(gomock.Any(), vol.Config).Return([]*storage.Snapshot{snapshot}, nil).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				require.Len(t, result, 1)
				assert.Equal(t, "snapshot1", result[0].Config.Name)
			},
		},
		{
			name:         "VolumeNotFound",
			volumeName:   "nonexistentVolume",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume nonexistentVolume not found")
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BackendNotFound",
			volumeName:   "vol1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "nonexistent-backend-uuid",
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonexistent-backend-uuid not found")
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.ReadSnapshotsForVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		snapshotName string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError  func(err error)
		verifyResult func(result *storage.SnapshotExternal)
	}{
		{
			name:         "Success",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				backend := getFakeBackend("testBackend1", "backend-uuid1", nil)
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}

				addBackendsToCache(t, backend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)
			},
		},
		{
			name:         "SnapshotNotFound",
			volumeName:   "vol1",
			snapshotName: "nonexistentSnapshot",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "snapshot nonexistentSnapshot was not found")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "FetchAndUpdateSnapshot_ToOnline",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateCreating,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				onlineSnapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}
				mockBackend.EXPECT().GetSnapshot(gomock.Any(), snapshot.Config, vol.Config).
					Return(onlineSnapshot, nil).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				// Success on saving the snapshot to persistence
				mockStoreClient.EXPECT().UpdateSnapshot(gomock.Any(), onlineSnapshot).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)
				assert.Equal(t, storage.SnapshotStateOnline, result.State)
			},
		},
		{
			name:         "FetchAndUpdateSnapshot_NoStatusChange",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateCreating,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetSnapshot(gomock.Any(), snapshot.Config, vol.Config).
					Return(snapshot, nil).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)
				assert.Equal(t, storage.SnapshotStateCreating, result.State)
			},
		},
		{
			name:         "FetchAndUpdateSnapshot_BackendError",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateCreating,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetSnapshot(gomock.Any(), snapshot.Config, vol.Config).
					Return(nil, errors.New("ontap error")).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "ontap error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "FetchAndUpdateSnapshot_PersistenceError",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateCreating,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				onlineSnapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}
				mockBackend.EXPECT().GetSnapshot(gomock.Any(), snapshot.Config, vol.Config).
					Return(onlineSnapshot, nil).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				// Error on saving the snapshot to persistence
				mockStoreClient.EXPECT().UpdateSnapshot(gomock.Any(), onlineSnapshot).Return(
					errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "persistence error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
				// Make sure snapshot in the cache is not updated
				snapshotID := storage.MakeSnapshotID("vol1", "snapshot1")
				snapshot := getSnapshotByIDFromCache(t, snapshotID)
				assert.Equal(t, storage.SnapshotStateCreating, snapshot.State)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			result, err := o.GetSnapshot(testCtx, tt.volumeName, tt.snapshotName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		snapshotName string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError  func(err error)
	}{
		{
			name:         "Success",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().DeleteSnapshot(gomock.Any(), snapshot.Config, vol.Config).Return(nil).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				// Additionally verify the snapshot is removed from the cache
				snapshot := getSnapshotByIDFromCache(t, storage.MakeSnapshotID("vol1", "snapshot1"))
				assert.Nil(t, snapshot)
			},
		},
		{
			name:         "SnapshotNotFound",
			volumeName:   "vol1",
			snapshotName: "nonexistentSnapshot",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "snapshot nonexistentSnapshot not found on volume vol1")
			},
		},
		{
			name:         "VolumeNotFound",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				// Remove volume from cache to simulate volume not found error.
				removeVolumeFromCache(t, vol.Config.Name)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume vol1 not found")
			},
		},
		{
			name:         "SnapshotInMissingVolumeState",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateMissingVolume,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Remove volume from cache to simulate volume not found error.
				removeVolumeFromCache(t, vol.Config.Name)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Nil(t, err)

				// Additionally verify the snapshot is removed from the cache
				snapshot := getSnapshotByIDFromCache(t, storage.MakeSnapshotID("vol1", "snapshot1"))
				assert.Nil(t, snapshot)
			},
		},
		{
			name:         "BackendNotFound",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Remove backend from cache to simulate volume not found error.
				removeBackendFromCache(t, "backend-uuid1")
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend backend-uuid1 not found")
			},
		},
		{
			name:         "SnapshotInBackendMissingState",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateMissingBackend,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Remove backend from cache to simulate volume not found error.
				removeBackendFromCache(t, "backend-uuid1")

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Nil(t, err)

				// Additionally verify the snapshot is removed from the cache
				snapshot := getSnapshotByIDFromCache(t, storage.MakeSnapshotID("vol1", "snapshot1"))
				assert.Nil(t, snapshot)
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No snapshots added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
		},
		{
			name:         "SnapshotInUse",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}

				readonlyVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						InternalName:        "readonlyVol",
						Name:                "readonlyVol",
						ReadOnlyClone:       true,
						CloneSourceSnapshot: "snapshot1",
					},
					BackendUUID: "backend-uuid1",
				}

				addVolumesToCache(t, vol, readonlyVol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "unable to delete snapshot snapshot1 as it is a source for read-only clone readonlyVol")
			},
		},
		{
			name:         "HandlePreviouslyFailedTxn_Success",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					State:  storage.SnapshotStateOnline,
				}

				existingTxn := &storage.VolumeTransaction{
					Op: storage.DeleteSnapshot,

					SnapshotConfig: &storage.SnapshotConfig{
						Name:       "snapshot1",
						VolumeName: "vol1",
					},
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().DeleteSnapshot(gomock.Any(), snapshot.Config, vol.Config).Return(nil).
					Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(existingTxn, nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "rejecting the deleteSnapshot transaction after successful completion of a preexisting deleteSnapshot transaction")

				// Additionally verify the snapshot is removed from the cache
				snapshot := getSnapshotByIDFromCache(t, storage.MakeSnapshotID("vol1", "snapshot1"))
				assert.Nil(t, snapshot)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			err := o.DeleteSnapshot(testCtx, tt.volumeName, tt.snapshotName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestCreateSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		snapshotConfig *storage.SnapshotConfig
		bootstrapErr   error
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError    func(err error)
		verifyResult   func(result *storage.SnapshotExternal)
	}{
		{
			name: "Success",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&storage.Snapshot{
						Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddSnapshot(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)
			},
		},
		{
			name: "VolumeNotFound",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "nonexistentVolume",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "source volume nonexistentVolume not found")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "VolumeNotFound_SubVolumeFound",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "subVol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				subVol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "subVol1", Name: "subVol1", ShareSourceVolume: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
				addSubordinateVolumesToCache(t, subVol)
				// Remove the volume so that we hit the vol not found but subVol found case
				removeVolumeFromCache(t, vol.Config.Name)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "creating snapshot is not allowed on subordinate volume subVol1")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "VolumeDeleting",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "source volume vol1 is deleting")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BackendNotFound",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "nonexistent-backend-uuid",
				}
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonexistent-backend-uuid for the source volume not found")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BootstrapError",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "SnapshotAlreadyExists",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "snapshot vol1/snapshot1 already exists")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "CanSnapshotError",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					errors.New("snapshots cannot be taken")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "snapshots cannot be taken", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "AddVolumeTransactionError",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(
					errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "persistence error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "CreateSnapshotError",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					nil, errors.New("ontap error")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "failed to create snapshot snapshot1 for volume vol1 on backend testBackend1: ontap error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "CreateSnapshot_MaxLimitReachedError",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					nil, errors.MaxLimitReachedError("max limit error")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsMaxLimitReachedError(err))
				assert.Equal(t, "failed to create snapshot snapshot1 for volume vol1 on backend testBackend1: max limit error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "AddSnapshotPersistenceError",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&storage.Snapshot{
						Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					}, nil).Times(1)
				mockBackend.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().AddSnapshot(gomock.Any(), gomock.Any()).Return(
					errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "persistence error", err.Error())
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "HandlePreviouslyFailedTxn_NoSnapshotInCache_Success",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				existingTxn := &storage.VolumeTransaction{
					Op: storage.AddSnapshot,
					Config: &storage.VolumeConfig{
						Name:         "testVolume",
						InternalName: "testVolume",
					},
					SnapshotConfig: &storage.SnapshotConfig{
						Name:       "snapshot1",
						VolumeName: "vol1",
					},
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().DeleteSnapshot(gomock.Any(), existingTxn.SnapshotConfig, existingTxn.Config).Return(nil).Times(1)
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&storage.Snapshot{
						Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(existingTxn, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				mockStoreClient.EXPECT().AddSnapshot(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)
			},
		},
		{
			name: "HandlePreviouslyFailedTxn_SnapshotInCache_Success",
			snapshotConfig: &storage.SnapshotConfig{
				Name:       "snapshot1",
				VolumeName: "vol1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}

				existingTxn := &storage.VolumeTransaction{
					Op: storage.AddSnapshot,
					Config: &storage.VolumeConfig{
						Name:         "testVolume",
						InternalName: "testVolume",
					},
					SnapshotConfig: &storage.SnapshotConfig{
						Name:       "snapshot1",
						VolumeName: "vol1",
					},
				}

				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().DeleteSnapshot(gomock.Any(), existingTxn.SnapshotConfig, gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&storage.Snapshot{
						Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					}, nil).Times(1)

				// Simulate the snapshot already exists in the cache
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(existingTxn, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				mockStoreClient.EXPECT().AddSnapshot(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			result, err := o.CreateSnapshot(testCtx, tt.snapshotConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestCreateSnapshotConcurrentCore_ConfigComplete(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	// Re-initialize the concurrent cache for each test
	db.Initialize()

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	o := getConcurrentOrchestrator()
	o.storeClient = mockStoreClient

	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName:        "vol1",
			Name:                "vol1",
			LUKSPassphraseNames: []string{"pass1", "pass2"},
		},
		BackendUUID: "backend-uuid1",
	}

	mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
	mockBackend.EXPECT().CanSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockBackend.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&storage.Snapshot{
			Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
		}, nil).Times(1)

	addBackendsToCache(t, mockBackend)
	addVolumesToCache(t, vol)

	mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
	mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, txn *storage.VolumeTransaction) error {
		assert.NotNil(t, txn.SnapshotConfig)
		assert.Equal(t, "snapshot1", txn.SnapshotConfig.Name)
		assert.Equal(t, "vol1", txn.SnapshotConfig.VolumeInternalName)
		assert.ElementsMatch(t, []string{"pass2", "pass1"}, txn.SnapshotConfig.LUKSPassphraseNames)
		return nil
	}).Times(1)
	mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStoreClient.EXPECT().AddSnapshot(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	snapshotConfig := &storage.SnapshotConfig{
		Name:       "snapshot1",
		VolumeName: "vol1",
	}

	_, err := o.CreateSnapshot(testCtx, snapshotConfig)
	assert.NoError(t, err)

	persistenceCleanup(t, o)
}

func TestRestoreSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		snapshotName string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError  func(err error)
	}{
		{
			name:         "Success",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().RestoreSnapshot(gomock.Any(), snapshot.Config, vol.Config).Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:         "VolumeNotFound",
			volumeName:   "nonexistentVolume",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume nonexistentVolume not found")
			},
		},
		{
			name:         "SnapshotNotFound",
			volumeName:   "vol1",
			snapshotName: "nonexistentSnapshot",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "snapshot nonexistentSnapshot not found on volume vol1")
			},
		},
		{
			name:         "BackendNotFound",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "nonexistent-backend-uuid",
				}
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonexistent-backend-uuid not found")
			},
		},
		{
			name:         "VolumeAttached",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}
				publication := &models.VolumePublication{
					VolumeName: "vol1",
					NodeName:   "node1",
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
				addVolumePublicationsToCache(t, publication)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "cannot restore attached volume to snapshot")
			},
		},
		{
			name:         "RestoreSnapshotError",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().RestoreSnapshot(gomock.Any(), snapshot.Config, vol.Config).
					Return(errors.New("ontap restore error")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "ontap restore error")
			},
		},
		{
			name:         "BootstrapError",
			volumeName:   "vol1",
			snapshotName: "snapshot1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			err := o.RestoreSnapshot(testCtx, tt.volumeName, tt.snapshotName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestImportSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		snapshotConfig *storage.SnapshotConfig
		bootstrapErr   error
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError    func(err error)
		verifyResult   func(result *storage.SnapshotExternal)
	}{
		{
			name: "Success",
			snapshotConfig: &storage.SnapshotConfig{
				Name:         "snapshot1",
				VolumeName:   "vol1",
				InternalName: "internalSnapshot1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&storage.Snapshot{
						Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
					}, nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().AddSnapshot(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "snapshot1", result.Config.Name)

				// Additionally verify the snapshot is added to the cache
				snapshot := getSnapshotByIDFromCache(t, storage.MakeSnapshotID(
					"vol1", "snapshot1"))
				assert.NotNil(t, snapshot)
			},
		},
		{
			name: "VolumeNotFound",
			snapshotConfig: &storage.SnapshotConfig{
				Name:         "snapshot1",
				VolumeName:   "nonexistentVolume",
				InternalName: "internalSnapshot1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No volumes added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume nonexistentVolume was not found")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "BackendNotFound",
			snapshotConfig: &storage.SnapshotConfig{
				Name:         "snapshot1",
				VolumeName:   "vol1",
				InternalName: "internalSnapshot1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "nonexistent-backend-uuid",
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonexistent-backend-uuid for volume vol1 not found")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "SnapshotAlreadyExists",
			snapshotConfig: &storage.SnapshotConfig{
				Name:         "snapshot1",
				VolumeName:   "vol1",
				InternalName: "internalSnapshot1",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
				}
				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "snapshot vol1/snapshot1 already exists")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
		{
			name: "InvalidInput",
			snapshotConfig: &storage.SnapshotConfig{
				Name:         "snapshot1",
				VolumeName:   "vol1",
				InternalName: "",
			},
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "snapshot internal name not found")
			},
			verifyResult: func(result *storage.SnapshotExternal) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			result, err := o.ImportSnapshot(testCtx, tt.snapshotConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestAddStorageClassConcurrentCore(t *testing.T) {
	scConfig := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
	}
	sc := storageclass.New(scConfig)

	fakePool1 := storage.NewStoragePool(nil, "pool1")

	tests := []struct {
		name         string
		scConfig     *storageclass.Config
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyResult func(result *storageclass.External, o *ConcurrentTridentOrchestrator)
		verifyError  func(t *testing.T, err error)
	}{
		{
			name:         "Success",
			scConfig:     scConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				fakePool1.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)

				mockStoreClient.EXPECT().AddStorageClass(gomock.Any(), sc).Return(nil).Times(1)
			},
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.NotNil(t, result)

				// Additionally verify the storage class is added to the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.NotNil(t, newSC)

				// Assert the pool map was updated
				poolMap := o.GetStorageClassPoolMap()
				assert.NotNil(t, poolMap)

				backendPoolMap := poolMap.BackendPoolMapForStorageClass(testCtx, "sc1")
				assert.NotNil(t, backendPoolMap)

				poolNames := backendPoolMap["testBackend"]
				assert.True(t, collection.StringInSlice("pool1", poolNames))
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:         "BootstrapError",
			scConfig:     scConfig,
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks:   nil,
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, result)

				// Additionally verify the storage class is not added to the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.Nil(t, newSC)
			},
			verifyError: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:         "StorageClassAlreadyExists",
			scConfig:     scConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				fakePool1.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc)
			},
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, result)

				// Additionally verify the storage class remains in the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.NotNil(t, newSC)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NotNil(t, err)
			},
		},
		{
			name:         "StoreError",
			scConfig:     scConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{"pool1": fakePool1})).AnyTimes()

				fakePool1.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)

				mockStoreClient.EXPECT().AddStorageClass(gomock.Any(), sc).Return(failed).Times(1)
			},
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, result)

				// Additionally verify the storage class is not added to the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.Nil(t, newSC)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NotNil(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.AddStorageClass(testCtx, tt.scConfig)

			if tt.verifyResult != nil {
				tt.verifyResult(result, o)
			}

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestUpdateStorageClassConcurrentCore(t *testing.T) {
	scConfig := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
		Pools:   map[string][]string{"testBackend": {"pool1"}},
	}
	sc := storageclass.New(scConfig)

	scConfigUpdated := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
		Pools:   map[string][]string{"testBackend": {"pool2"}},
	}

	fakePool1 := storage.NewStoragePool(nil, "pool1")
	fakePool2 := storage.NewStoragePool(nil, "pool2")

	tests := []struct {
		name         string
		scConfig     *storageclass.Config
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyResult func(result *storageclass.External, o *ConcurrentTridentOrchestrator)
		verifyError  func(t *testing.T, err error)
	}{
		{
			name:         "Success",
			scConfig:     scConfigUpdated,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().UpdateStorageClass(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.NotNil(t, result)

				// Additionally verify the storage class is added to the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.NotNil(t, newSC)

				// Assert the pool map was updated
				poolMap := o.GetStorageClassPoolMap()
				assert.NotNil(t, poolMap)

				backendPoolMap := poolMap.BackendPoolMapForStorageClass(testCtx, "sc1")
				assert.NotNil(t, backendPoolMap)

				poolNames := backendPoolMap["testBackend"]
				assert.False(t, collection.StringInSlice("pool1", poolNames))
				assert.True(t, collection.StringInSlice("pool2", poolNames))
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:         "BootstrapError",
			scConfig:     scConfigUpdated,
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks:   nil,
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, result)

				// Additionally verify the storage class is not added to the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.Nil(t, newSC)
			},
			verifyError: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:         "StorageClassNotFound",
			scConfig:     scConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
			},
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, result)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NotNil(t, err)
			},
		},
		{
			name:         "StoreError",
			scConfig:     scConfig,
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().UpdateStorageClass(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyResult: func(result *storageclass.External, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, result)

				// Additionally verify the storage class remains in the cache
				newSC := getStorageClassByNameFromCache(t, "sc1")
				assert.NotNil(t, newSC)

				// Assert the pool map was not updated
				poolMap := o.GetStorageClassPoolMap()
				assert.NotNil(t, poolMap)

				backendPoolMap := poolMap.BackendPoolMapForStorageClass(testCtx, "sc1")
				assert.NotNil(t, backendPoolMap)

				poolNames := backendPoolMap["testBackend"]
				assert.True(t, collection.StringInSlice("pool1", poolNames))
				assert.False(t, collection.StringInSlice("pool2", poolNames))
			},
			verifyError: func(t *testing.T, err error) {
				assert.NotNil(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.UpdateStorageClass(testCtx, tt.scConfig)

			if tt.verifyResult != nil {
				tt.verifyResult(result, o)
			}

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteStorageClassConcurrentCore(t *testing.T) {
	scConfig := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
		Pools:   map[string][]string{"testBackend": {"pool1"}},
	}
	sc := storageclass.New(scConfig)

	fakePool1 := storage.NewStoragePool(nil, "pool1")
	fakePool2 := storage.NewStoragePool(nil, "pool2")

	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(t *testing.T, err error, o *ConcurrentTridentOrchestrator)
	}{
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().DeleteStorageClass(gomock.Any(), sc).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error, o *ConcurrentTridentOrchestrator) {
				assert.NoError(t, err)

				// Assert the pool map no longer contains the storage class
				poolMap := o.GetStorageClassPoolMap()
				assert.NotNil(t, poolMap)

				backendPoolMap := poolMap.BackendPoolMapForStorageClass(testCtx, "sc1")
				assert.Nil(t, backendPoolMap)
			},
		},
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks:   nil,
			verifyError: func(t *testing.T, err error, o *ConcurrentTridentOrchestrator) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:         "StorageClassNotFound",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(t *testing.T, err error, o *ConcurrentTridentOrchestrator) {
				assert.NotNil(t, err)
			},
		},
		{
			name:         "StoreError",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().DeleteStorageClass(gomock.Any(), sc).Return(failed).Times(1)
			},
			verifyError: func(t *testing.T, err error, o *ConcurrentTridentOrchestrator) {
				assert.NotNil(t, err)

				// Assert the pool map was not updated
				poolMap := o.GetStorageClassPoolMap()
				assert.NotNil(t, poolMap)

				backendPoolMap := poolMap.BackendPoolMapForStorageClass(testCtx, "sc1")
				assert.NotNil(t, backendPoolMap)

				poolNames := backendPoolMap["testBackend"]
				assert.True(t, collection.StringInSlice("pool1", poolNames))
				assert.False(t, collection.StringInSlice("pool2", poolNames))
			},
		},
		{
			name:         "DeleteStorageClass with autogrow policy - volumes updated successfully",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// Create SC with autogrow policy
				scConfigWithPolicy := &storageclass.Config{
					Version:        "1",
					Name:           "sc1",
					Pools:          map[string][]string{"testBackend": {"pool1"}},
					AutogrowPolicy: "policy1",
				}
				scWithPolicy := storageclass.New(scConfigWithPolicy)
				addStorageClassesToCache(t, scWithPolicy)

				// Add autogrow policy
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				// Add backend
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
				})).AnyTimes()
				fakePool1.SetBackend(mockBackend)
				addBackendsToCache(t, mockBackend)

				// Add volume using this SC
				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend-uuid1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().DeleteStorageClass(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error, o *ConcurrentTridentOrchestrator) {
				assert.NoError(t, err)

				// Volume should have been updated to empty policy
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.ReadVolume("vol1")))
				defer unlocker()
				vol := results[0].Volume.Read
				assert.NotNil(t, vol)

				// Volume should now have empty policy (SC was deleted, no fallback)
				assert.Equal(t, "", vol.EffectiveAGPolicy.PolicyName)
				assert.Equal(t, models.AutogrowPolicyReasonNotConfigured, vol.EffectiveAGPolicy.Reason)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.DeleteStorageClass(testCtx, "sc1")

			if tt.verifyError != nil {
				tt.verifyError(t, err, o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetStorageClassConcurrentCore(t *testing.T) {
	scConfig := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
		Pools:   map[string][]string{"testBackend": {"pool1"}},
	}
	sc := storageclass.New(scConfig)

	fakePool1 := storage.NewStoragePool(nil, "pool1")
	fakePool2 := storage.NewStoragePool(nil, "pool2")

	tests := []struct {
		name         string
		scName       string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result *storageclass.External)
	}{
		{
			name:         "Success",
			scName:       "sc1",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc)

				o.RebuildStorageClassPoolMap(testCtx)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storageclass.External) {
				require.NotNil(t, result)
				assert.Equal(t, "sc1", result.Config.Name)
			},
		},
		{
			name:         "BootstrapError",
			scName:       "sc1",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result *storageclass.External) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "NotFound",
			scName:       "nonexistent",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsNotFoundError(err))
			},
			verifyResult: func(result *storageclass.External) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.GetStorageClass(testCtx, tt.scName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListStorageClassesConcurrentCore(t *testing.T) {
	sc1Config := &storageclass.Config{
		Version: "1",
		Name:    "sc1",
		Pools:   map[string][]string{"testBackend": {"pool1"}},
	}
	sc1 := storageclass.New(sc1Config)

	sc2Config := &storageclass.Config{
		Version: "1",
		Name:    "sc2",
		Pools:   map[string][]string{"testBackend": {"pool2"}},
	}
	sc2 := storageclass.New(sc2Config)

	fakePool1 := storage.NewStoragePool(nil, "pool1")
	fakePool2 := storage.NewStoragePool(nil, "pool2")

	tests := []struct {
		name         string
		bootstrapErr error
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError  func(err error)
		verifyResult func(result []*storageclass.External)
	}{
		{
			name:         "Success",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(makeSyncMapFromMap(map[string]storage.Pool{
					"pool1": fakePool1,
					"pool2": fakePool2,
				})).AnyTimes()

				fakePool1.SetBackend(mockBackend)
				fakePool2.SetBackend(mockBackend)

				addBackendsToCache(t, mockBackend)
				addStorageClassesToCache(t, sc1, sc2)

				o.RebuildStorageClassPoolMap(testCtx)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storageclass.External) {
				require.Len(t, result, 2)
				expectedNames := []string{"sc1", "sc2"}
				actualNames := []string{}
				for _, sc := range result {
					actualNames = append(actualNames, sc.Config.Name)
				}
				assert.ElementsMatch(t, expectedNames, actualNames)
			},
		},
		{
			name:         "BootstrapError",
			bootstrapErr: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Equal(t, "bootstrap error", err.Error())
			},
			verifyResult: func(result []*storageclass.External) {
				assert.Nil(t, result)
			},
		},
		{
			name:         "Empty",
			bootstrapErr: nil,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No volumes or subordinate volumes added
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result []*storageclass.External) {
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			o.bootstrapError = tt.bootstrapErr

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.ListStorageClasses(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestAddNodeConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*testing.T, *gomock.Controller) persistentstore.Client
		newNode        *models.Node
		expectedNode   *models.Node
		verifyBehavior func(*ConcurrentTridentOrchestrator)
		verifyError    func(*testing.T, error)
	}{
		{
			name: "Success with new node",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			newNode: &models.Node{
				Name: "node1",
				HostInfo: &models.HostSystem{
					Services: []string{"iscsi", "nfs"},
				},
			},
			expectedNode: &models.Node{
				Name: "node1",
				HostInfo: &models.HostSystem{
					Services: []string{"iscsi", "nfs"},
				},
				LogLevel: "debug",
			},
			verifyBehavior: func(o *ConcurrentTridentOrchestrator) {
				assert.WithinDuration(t, time.Now(), o.getLastNodeRegistrationTime(), 10*time.Second)
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
		},
		{
			name: "Success with existing node and backends",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")

				// Setup backend expectations
				mockBackend1.EXPECT().BackendUUID().Return("uuid1").AnyTimes()
				mockBackend1.EXPECT().InvalidateNodeAccess().AnyTimes()
				mockBackend2.EXPECT().BackendUUID().Return("uuid2").AnyTimes()
				mockBackend2.EXPECT().InvalidateNodeAccess().AnyTimes()

				// Add backend to cache
				addBackendsToCache(t, mockBackend1, mockBackend2)

				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertNode("node1")))
				defer unlocker()

				results[0].Node.Upsert(&models.Node{
					Name: "node1",
					HostInfo: &models.HostSystem{
						Services: []string{"iscsi", "nfs"},
					},
					PublicationState: models.NodeCleanable,
				})
				return mockStoreClient
			},
			newNode: &models.Node{
				Name: "node1",
			},
			expectedNode: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeCleanable,
				LogLevel:         "debug",
			},
			verifyBehavior: func(o *ConcurrentTridentOrchestrator) {
				assert.WithinDuration(t, time.Now(), o.getLastNodeRegistrationTime(), 10*time.Second)
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure in store",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(),
					gomock.Any()).Return(fmt.Errorf("persistent store error")).Times(1)

				return mockStoreClient
			},
			newNode: &models.Node{
				Name: "node1",
				HostInfo: &models.HostSystem{
					Services: []string{"iscsi", "nfs"},
				},
			},
			expectedNode: nil,
			verifyBehavior: func(o *ConcurrentTridentOrchestrator) {
				assert.Equal(t, time.Time{}, o.getLastNodeRegistrationTime())
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.ErrorContains(t, err, "persistent store error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			db.Initialize()

			mockStoreClient := tt.setup(t, mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient
			o.lastNodeRegistrationTime = time.Time{}

			err := o.AddNode(testCtx, tt.newNode, func(_, _, _ string) {})

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(o)
			}
			tt.verifyError(t, err)

			results, unlocker, _ := db.Lock(testCtx, db.Query(db.ReadNode(tt.newNode.Name)))
			defer unlocker()

			assert.NotNil(t, results)
			assert.Equal(t, results[0].Node.Read, tt.expectedNode)
		})
	}
}

func TestUpdateNodeConcurrentCore(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*testing.T, *gomock.Controller) persistentstore.Client
		verifyError   func(*testing.T, error)
		flags         *models.NodePublicationStateFlags
		node          *models.Node
		expectedState models.NodePublicationState
	}{
		{
			name: "Failure when node does not exist",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Times(0)
				return mockStoreClient
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.Equal(t, err, errors.NotFoundError("node node1 was not found"))
			},
		},
		{
			// a "clean" flag set will only transition to cleanable, not clean
			name: "invalid transition dirty->clean",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(true),
				AdministratorReady: convert.ToPtr(true),
				ProvisionerReady:   convert.ToPtr(true),
			},
			node: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeDirty,
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			expectedState: models.NodeCleanable,
		},
		{
			name: "clean->dirty",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(false),
				AdministratorReady: convert.ToPtr(false),
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			node: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeClean,
			},
			expectedState: models.NodeDirty,
		},
		{
			name: "cleanable->clean",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(true),
				AdministratorReady: convert.ToPtr(true),
				ProvisionerReady:   convert.ToPtr(true),
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			node: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeCleanable,
			},
			expectedState: models.NodeClean,
		},
		{
			name: "cleanable->dirty",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(false),
				AdministratorReady: convert.ToPtr(false),
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			node: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeCleanable,
			},
			expectedState: models.NodeDirty,
		},
		{
			name: "dirty->cleanable",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(true),
				AdministratorReady: convert.ToPtr(true),
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			node: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeDirty,
			},
			expectedState: models.NodeCleanable,
		},
		{
			name: "invalid transition clean->cleanable",
			setup: func(t *testing.T, mockCtrl *gomock.Controller) persistentstore.Client {
				t.Helper()
				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				return mockStoreClient
			},
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(true),
				AdministratorReady: convert.ToPtr(true),
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			node: &models.Node{
				Name:             "node1",
				PublicationState: models.NodeClean,
			},
			expectedState: models.NodeClean,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			db.Initialize()
			mockStoreClient := tt.setup(t, mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.node != nil {
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertNode(tt.node.Name)))
				results[0].Node.Upsert(tt.node)
				unlocker()
			}

			err := o.UpdateNode(context.Background(), "node1", tt.flags)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}
			if tt.expectedState != "" {
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.ReadNode("node1")))
				defer unlocker()
				assert.NotNil(t, results)
				assert.Equal(t, tt.expectedState, results[0].Node.Read.PublicationState)
			}
		})
	}
}

func TestGetNodeConcurrentCore(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*testing.T)
		verifyError  func(*testing.T, error)
		expectedNode *models.NodeExternal
	}{
		{
			name: "success",
			setup: func(t *testing.T) {
				t.Helper()
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertNode("node1")))
				defer unlocker()
				results[0].Node.Upsert(getFakeNode("node1"))
			},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.NoError(t, err)
			},
			expectedNode: getFakeNode("node1").ConstructExternal(),
		},
		{
			name:  "node not found",
			setup: func(t *testing.T) {},
			verifyError: func(t *testing.T, err error) {
				t.Helper()
				assert.Error(t, err)
				assert.ErrorContains(t, err, "node node1 was not found")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			tt.setup(t)
			o := getConcurrentOrchestrator()
			node, err := o.GetNode(testCtx, "node1")
			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}
			assert.Equal(t, tt.expectedNode, node)
		})
	}
}

func TestListNodesConcurrentCore(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*testing.T)
		expectedNodes []*models.NodeExternal
	}{
		{
			name: "list multiple nodes",
			setup: func(t *testing.T) {
				// maps are always iterated randomly
				for name := range map[string]struct{}{"node1": {}, "node2": {}} {
					results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertNode(name)))
					results[0].Node.Upsert(getFakeNode(name))
					unlocker()
				}
			},
			expectedNodes: []*models.NodeExternal{
				getFakeNode("node1").ConstructExternal(),
				getFakeNode("node2").ConstructExternal(),
			},
		},
		{
			name: "list no nodes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			if tt.setup != nil {
				tt.setup(t)
			}
			o := getConcurrentOrchestrator()
			nodes, err := o.ListNodes(testCtx)
			assert.NoError(t, err)
			for _, expectedNode := range tt.expectedNodes {
				assert.Contains(t, nodes, expectedNode)
			}
		})
	}
}

func TestDeleteNodeConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		nodeName       string
		bootstrapError error
		ctx            context.Context
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyBehavior func(t *testing.T, o *ConcurrentTridentOrchestrator)
	}{
		{
			name:           "Success",
			nodeName:       "testNode",
			bootstrapError: nil,
			ctx:            testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				node := &models.Node{Name: "testNode"}
				addNodesToCache(t, node)

				// Add backends to cache
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")
				addBackendsToCache(t, mockBackend1, mockBackend2)

				// Setup backend expectations
				mockBackend1.EXPECT().BackendUUID().Return("uuid1").AnyTimes()
				mockBackend1.EXPECT().InvalidateNodeAccess().Times(1)
				mockBackend1.EXPECT().IsNodeAccessUpToDate().Return(true).Times(1)
				mockBackend2.EXPECT().BackendUUID().Return("uuid2").AnyTimes()
				mockBackend2.EXPECT().InvalidateNodeAccess().Times(1)
				mockBackend2.EXPECT().IsNodeAccessUpToDate().Return(false).Times(1)
				mockBackend2.EXPECT().CanEnablePublishEnforcement().Return(true).Times(1)
				mockBackend2.EXPECT().Volumes().Return(&sync.Map{}).Times(1)
				mockBackend2.EXPECT().ReconcileNodeAccess(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockBackend2.EXPECT().SetNodeAccessUpToDate().Times(1)

				mockStoreClient.EXPECT().DeleteNode(gomock.Any(), node).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify node is removed from cache
				nodes, err := o.ListNodes(context.Background())
				assert.NoError(t, err)
				assert.Empty(t, nodes)
			},
		},
		{
			name:           "SuccessNodeHasPublications",
			nodeName:       "testNode",
			bootstrapError: nil,
			ctx:            testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				node := &models.Node{Name: "testNode"}
				addNodesToCache(t, node)

				// Add backends to cache
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")
				addBackendsToCache(t, mockBackend1, mockBackend2)

				// Add publication to cache
				fakePublication := getFakeVolumePublication("vol1", "testNode")
				addVolumePublicationsToCache(t, fakePublication)

				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify node is still present and marked as deleted
				nodes, err := o.ListNodes(context.Background())
				assert.NoError(t, err)
				assert.NotEmpty(t, nodes)
				assert.True(t, *nodes[0].Deleted)
			},
		},
		{
			name:           "SuccessNodeHasPublicationsUpdateFailed",
			nodeName:       "testNode",
			bootstrapError: nil,
			ctx:            testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				node := &models.Node{Name: "testNode"}
				addNodesToCache(t, node)

				// Add backends to cache
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")
				addBackendsToCache(t, mockBackend1, mockBackend2)

				// Add publication to cache
				fakePublication := getFakeVolumePublication("vol1", "testNode")
				addVolumePublicationsToCache(t, fakePublication)

				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(failed).Times(1)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify node is still present and unchanged
				nodes, err := o.ListNodes(context.Background())
				assert.NoError(t, err)
				assert.NotEmpty(t, nodes)
				assert.False(t, *nodes[0].Deleted)
			},
		},
		{
			name:           "NodeNotFound",
			nodeName:       "nonExistentNode",
			bootstrapError: nil,
			ctx:            testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No node added to cache

				// Add backends to cache
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")
				addBackendsToCache(t, mockBackend1, mockBackend2)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "node nonExistentNode was not found")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No behavior to verify
			},
		},
		{
			name:           "DeleteNodeError",
			nodeName:       "testNode",
			bootstrapError: nil,
			ctx:            testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				node := &models.Node{Name: "testNode"}
				addNodesToCache(t, node)

				// Add backends to cache
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")
				addBackendsToCache(t, mockBackend1, mockBackend2)

				mockStoreClient.EXPECT().DeleteNode(gomock.Any(), node).Return(errors.New("delete error")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "failed to delete node testNode in store")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Node should still exist in cache
				nodes, err := o.ListNodes(context.Background())
				assert.NoError(t, err)
				assert.Len(t, nodes, 1)
			},
		},
		{
			name:           "BootstrapError",
			nodeName:       "testNode",
			bootstrapError: errors.New("bootstrap error"),
			ctx:            testCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No behavior to verify
			},
		},
		{
			name:           "LockError",
			nodeName:       "testNode",
			bootstrapError: nil,
			ctx:            expiredCtx,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "context deadline exceeded")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No behavior to verify
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI

			mockCtrl := gomock.NewController(t)

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.DeleteNode(tt.ctx, tt.nodeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t, o)
			}
		})
	}
}

func TestPeriodicallyReconcileNodeAccessOnBackendsConcurrentCore(t *testing.T) {
	tests := []struct {
		name                           string
		reconcilePeriod                time.Duration
		lastNodeRegistrationTimeOffset time.Duration
		setupMocks                     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
	}{
		{
			name:                           "LoopStartsAndStopsCorrectly",
			reconcilePeriod:                NodeAccessReconcilePeriod,
			lastNodeRegistrationTimeOffset: 0,
		},
		{
			name:                           "NotTimeToReconcileYet",
			reconcilePeriod:                50 * time.Millisecond,
			lastNodeRegistrationTimeOffset: 0,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")

				// Add backend to cache
				addBackendsToCache(t, mockBackend)
			},
		},
		{
			name:                           "LoopReconcilesBackend",
			reconcilePeriod:                50 * time.Millisecond,
			lastNodeRegistrationTimeOffset: -2 * NodeRegistrationCooldownPeriod,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")

				// Setup backend expectations
				mockBackend.EXPECT().BackendUUID().Return("uuid1").AnyTimes()
				mockBackend.EXPECT().IsNodeAccessUpToDate().Return(true).AnyTimes()

				// Add backend to cache
				addBackendsToCache(t, mockBackend)
			},
		},
		{
			name:                           "LoopReconcilesBackends",
			reconcilePeriod:                50 * time.Millisecond,
			lastNodeRegistrationTimeOffset: -2 * NodeRegistrationCooldownPeriod,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")

				// Setup backend expectations
				mockBackend1.EXPECT().BackendUUID().Return("uuid1").AnyTimes()
				mockBackend1.EXPECT().IsNodeAccessUpToDate().Return(true).AnyTimes()
				mockBackend2.EXPECT().BackendUUID().Return("uuid2").AnyTimes()
				mockBackend2.EXPECT().IsNodeAccessUpToDate().Return(false).AnyTimes()
				mockBackend2.EXPECT().CanEnablePublishEnforcement().Return(true).AnyTimes()
				mockBackend2.EXPECT().Volumes().Return(&sync.Map{}).AnyTimes()
				mockBackend2.EXPECT().ReconcileNodeAccess(gomock.Any(), gomock.Any(), gomock.Any()).Return(failed).AnyTimes()

				// Add backend to cache
				addBackendsToCache(t, mockBackend1, mockBackend2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient
			o.nodeAccessReconcilePeriod = tt.reconcilePeriod
			o.lastNodeRegistrationTime = time.Now().Add(tt.lastNodeRegistrationTimeOffset)

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			// Start loop
			go o.PeriodicallyReconcileNodeAccessOnBackends()
			for o.stopNodeAccessLoop == nil {
				// Wait for loop to initialize
				time.Sleep(10 * time.Millisecond)
			}

			assert.NotNil(t, o.stopNodeAccessLoop, "loop channel should be initialized")

			time.Sleep(500 * time.Millisecond) // Wait for loop to do some work

			// Stop loop
			o.Stop()
		})
	}
}

func TestPeriodicallyReconcileBackendStateConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		pollInterval   time.Duration
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyBehavior func(t *testing.T, o *ConcurrentTridentOrchestrator)
	}{
		{
			name:         "ZeroPollIntervalDoesNotStartLoop",
			pollInterval: 0,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No setup needed for zero interval test
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				assert.Nil(t, o.stopReconcileBackendLoop, "reconcile backend loop should not have started")
			},
		},
		{
			name:         "NormalPollIntervalStartsLoopBootstrapError",
			pollInterval: 50 * time.Millisecond,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(false).AnyTimes()
				mockBackend.EXPECT().Name().Return("backend1").AnyTimes()

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				// Set bootstrap error to trigger reconciliation
				o.bootstrapError = failed
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// The loop should be running (stopReconcileBackendLoop should not be nil)
				assert.NotNil(t, o.stopReconcileBackendLoop, "reconcile backend loop should have started")
			},
		},
		{
			name:         "ReconcileWithNoBackends",
			pollInterval: 50 * time.Millisecond,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// No backends added to cache
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// The loop should still be running even with no backends
				assert.NotNil(t, o.stopReconcileBackendLoop, "reconcile backend loop should have started")
			},
		},
		{
			name:         "ReconcileWithMultipleBackends",
			pollInterval: 50 * time.Millisecond,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "backend1", "uuid1")
				mockBackend2 := getMockBackend(mockCtrl, "backend2", "uuid2")

				// Setup backend expectations
				mockBackend1.EXPECT().CanGetState().Return(false).AnyTimes()
				mockBackend2.EXPECT().CanGetState().Return(false).AnyTimes()
				mockBackend1.EXPECT().Name().Return("backend1").AnyTimes()
				mockBackend2.EXPECT().Name().Return("backend2").AnyTimes()

				// Add backends to cache
				addBackendsToCache(t, mockBackend1, mockBackend2)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// The loop should be running
				assert.NotNil(t, o.stopReconcileBackendLoop, "reconcile backend loop should have started")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			go o.PeriodicallyReconcileBackendState(tt.pollInterval)
			if tt.pollInterval > 0 {
				for o.stopReconcileBackendLoop == nil {
					// Wait for loop to initialize
					time.Sleep(10 * time.Millisecond)
				}
			}

			time.Sleep(500 * time.Millisecond) // Wait for loop to do some work

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t, o)
			}

			// Clean up
			o.Stop()
		})
	}
}

func TestReconcileBackendStateConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		backendUUID string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend
		verifyError func(t *testing.T, err error)
	}{
		{
			name:        "BackendNotFound",
			backendUUID: "nonexistent",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				// No backend added to cache
				return nil
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.True(t, errors.IsNotFoundError(err))
			},
		},
		{
			name:        "BackendNotSupported",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(false).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "ReconcileHealthyBackendNoChange",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("", roaring.New()).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "ReconcileBackendNoPools",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")
				bitset := roaring.New()
				bitset.Add(storage.BackendStatePoolsChange) // Simulate a change in pool state

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("no aggregates", bitset).Times(2)
				mockBackend.EXPECT().UpdateBackendState(gomock.Any(), "no aggregates").Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "ReconcileBackendNoPoolsProblemResolves",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")
				bitset := roaring.New()
				bitset.Add(storage.BackendStatePoolsChange) // Simulate a change in pool state

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("no aggregates", bitset).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("", roaring.New()).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "ReconcileBackendStateReasonChange",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")
				bitset := roaring.New()
				bitset.Add(storage.BackendStatePoolsChange) // Simulate a change in pool state
				bitset.Add(storage.BackendStateReasonChange)

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("no aggregates", bitset).Times(2)
				mockBackend.EXPECT().UpdateBackendState(gomock.Any(), "no aggregates").Times(1)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), mockBackend).Return(nil).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "ReconcileBackendStateReasonChangeStoreError",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")
				bitset := roaring.New()
				bitset.Add(storage.BackendStatePoolsChange) // Simulate a change in pool state
				bitset.Add(storage.BackendStateReasonChange)

				// Setup backend expectations
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("no aggregates", bitset).Times(2)
				mockBackend.EXPECT().UpdateBackendState(gomock.Any(), "no aggregates").Times(1)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), mockBackend).Return(failed).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:        "ReconcileBackendPoolsChangeConfigError",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "backend1", "uuid1")
				bitset := roaring.New()
				bitset.Add(storage.BackendStatePoolsChange) // Simulate a change in pool state

				// Setup backend expectations for offline backend
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("", bitset).Times(2)
				mockBackend.EXPECT().UpdateBackendState(gomock.Any(), "").Times(1)
				mockBackend.EXPECT().MarshalDriverConfig().Return(nil, failed).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:        "ReconcileBackendPoolsChange",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "backend1",
					"uuid":       "uuid1",
					"state":      string(storage.Online),
					"driverName": "fake",
				})
				bitset := roaring.New()
				bitset.Add(storage.BackendStatePoolsChange) // Simulate a change in pool state

				fakeBackend := getFakeBackend("backend1", "uuid1", nil)
				fakeConfigBytes, _ := fakeBackend.MarshalDriverConfig()

				// Setup backend expectations for offline backend
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("", bitset).Times(2)
				mockBackend.EXPECT().UpdateBackendState(gomock.Any(), "").Times(1)
				mockBackend.EXPECT().MarshalDriverConfig().Return(fakeConfigBytes, nil).Times(1)
				mockBackend.EXPECT().ConfigRef().Return("").AnyTimes()
				mockBackend.EXPECT().Name().Return("backend1").AnyTimes()
				mockBackend.EXPECT().BackendUUID().Return("uuid1").AnyTimes()
				mockBackend.EXPECT().Driver().Return(fakeBackend.Driver()).AnyTimes()
				mockBackend.EXPECT().UserState().Return(storage.UserNormal).AnyTimes()
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend.EXPECT().Online().Return(true).AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(true).AnyTimes()
				mockBackend.EXPECT().SmartCopy().Return(mockBackend).AnyTimes()
				mockBackend.EXPECT().GetUniqueKey().Return("backend1").AnyTimes()
				mockBackend.EXPECT().ConstructPersistent(gomock.Any()).Return(&storage.BackendPersistent{Name: "backend1", BackendUUID: "uuid1"}).AnyTimes()
				mockBackend.EXPECT().Terminate(gomock.Any()).Times(1)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "ReconcileBackendVersionChangeStoreError",
			backendUUID: "uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "backend1",
					"uuid":       "uuid1",
					"state":      string(storage.Online),
					"driverName": "fake",
				})
				bitset := roaring.New()
				bitset.Add(storage.BackendStateAPIVersionChange) // Simulate a change in pool state

				fakeBackend := getFakeBackend("backend1", "uuid1", nil)
				fakeConfigBytes, _ := fakeBackend.MarshalDriverConfig()

				// Setup backend expectations for offline backend
				mockBackend.EXPECT().CanGetState().Return(true).Times(1)
				mockBackend.EXPECT().GetBackendState(gomock.Any()).Return("", bitset).Times(2)
				mockBackend.EXPECT().UpdateBackendState(gomock.Any(), "").Times(1)
				mockBackend.EXPECT().MarshalDriverConfig().Return(fakeConfigBytes, nil).Times(1)
				mockBackend.EXPECT().ConfigRef().Return("").AnyTimes()
				mockBackend.EXPECT().Name().Return("backend1").AnyTimes()
				mockBackend.EXPECT().BackendUUID().Return("uuid1").AnyTimes()
				mockBackend.EXPECT().Driver().Return(fakeBackend.Driver()).AnyTimes()
				mockBackend.EXPECT().UserState().Return(storage.UserNormal).AnyTimes()
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend.EXPECT().Online().Return(true).AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(true).AnyTimes()
				mockBackend.EXPECT().SmartCopy().Return(mockBackend).AnyTimes()
				mockBackend.EXPECT().GetUniqueKey().Return("backend1").AnyTimes()
				mockBackend.EXPECT().ConstructPersistent(gomock.Any()).Return(&storage.BackendPersistent{Name: "backend1", BackendUUID: "uuid1"}).AnyTimes()
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(failed).Times(1)

				// Add backend to cache
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			tt.setupMocks(mockCtrl, mockStoreClient, o)

			err := o.reconcileBackendState(testCtx, tt.backendUUID)

			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestReconcileVolumePublicationsConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		bootstrapError error
		verifyError    func(err error)
	}{
		{
			name:           "Success_WithOrphanedPublication",
			bootstrapError: nil,
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:           "BootstrapError",
			bootstrapError: errors.New("bootstrap error"),
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			err := o.ReconcileVolumePublications(context.Background(), nil)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestGetVolumePublicationConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		volumeName     string
		nodeName       string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(pub *models.VolumePublication)
	}{
		{
			name:           "Success",
			volumeName:     "testVolume",
			nodeName:       "testNode",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				pub := &models.VolumePublication{
					Name:       models.GenerateVolumePublishName("testVolume", "testNode"),
					VolumeName: "testVolume",
					NodeName:   "testNode",
				}
				addVolumePublicationsToCache(t, pub)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pub *models.VolumePublication) {
				assert.NotNil(t, pub)
				assert.Equal(t, "testVolume", pub.VolumeName)
				assert.Equal(t, "testNode", pub.NodeName)
			},
		},
		{
			name:           "NotFound",
			volumeName:     "nonExistentVolume",
			nodeName:       "testNode",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No publication added
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume publication")
				assert.ErrorContains(t, err, "not found")
			},
			verifyResult: func(pub *models.VolumePublication) {
				assert.Nil(t, pub)
			},
		},
		{
			name:           "BootstrapError",
			volumeName:     "testVolume",
			nodeName:       "testNode",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(pub *models.VolumePublication) {
				assert.Nil(t, pub)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.GetVolumePublication(testCtx, tt.volumeName, tt.nodeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}
		})
	}
}

func TestListVolumePublicationsConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(pubs []*models.VolumePublicationExternal)
	}{
		{
			name:           "Success",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				pub1 := &models.VolumePublication{
					Name:       "pub1",
					VolumeName: "volume1",
					NodeName:   "node1",
				}
				pub2 := &models.VolumePublication{
					Name:       "pub2",
					VolumeName: "volume2",
					NodeName:   "node2",
				}
				addVolumePublicationsToCache(t, pub1, pub2)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Len(t, pubs, 2)
			},
		},
		{
			name:           "EmptyList",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No publications added
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Empty(t, pubs)
			},
		},
		{
			name:           "BootstrapError",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Nil(t, pubs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.ListVolumePublications(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}
		})
	}
}

func TestListVolumePublicationsForVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		volumeName     string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(pubs []*models.VolumePublicationExternal)
	}{
		{
			name:           "Success",
			volumeName:     "testVolume",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				pub1 := &models.VolumePublication{
					Name:       "pub1",
					VolumeName: "testVolume",
					NodeName:   "node1",
				}
				pub2 := &models.VolumePublication{
					Name:       "pub2",
					VolumeName: "testVolume",
					NodeName:   "node2",
				}
				pub3 := &models.VolumePublication{
					Name:       "pub3",
					VolumeName: "otherVolume",
					NodeName:   "node3",
				}
				addVolumePublicationsToCache(t, pub1, pub2, pub3)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Len(t, pubs, 2)
				for _, pub := range pubs {
					assert.Equal(t, "testVolume", pub.VolumeName)
				}
			},
		},
		{
			name:           "NoPublicationsForVolume",
			volumeName:     "testVolume",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				pub := &models.VolumePublication{
					Name:       "pub1",
					VolumeName: "otherVolume",
					NodeName:   "node1",
				}
				addVolumePublicationsToCache(t, pub)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Empty(t, pubs)
			},
		},
		{
			name:           "BootstrapError",
			volumeName:     "testVolume",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Nil(t, pubs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.ListVolumePublicationsForVolume(testCtx, tt.volumeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestListVolumePublicationsForNodeConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		nodeName       string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(pubs []*models.VolumePublicationExternal)
	}{
		{
			name:           "Success",
			nodeName:       "testNode",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				pub1 := &models.VolumePublication{
					Name:       "pub1",
					VolumeName: "volume1",
					NodeName:   "testNode",
				}
				pub2 := &models.VolumePublication{
					Name:       "pub2",
					VolumeName: "volume2",
					NodeName:   "testNode",
				}
				pub3 := &models.VolumePublication{
					Name:       "pub3",
					VolumeName: "volume3",
					NodeName:   "otherNode",
				}
				addVolumePublicationsToCache(t, pub1, pub2, pub3)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Len(t, pubs, 2)
				for _, pub := range pubs {
					assert.Equal(t, "testNode", pub.NodeName)
				}
			},
		},
		{
			name:           "NoPublicationsForNode",
			nodeName:       "testNode",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				pub := &models.VolumePublication{
					Name:       "pub1",
					VolumeName: "volume1",
					NodeName:   "otherNode",
				}
				addVolumePublicationsToCache(t, pub)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Empty(t, pubs)
			},
		},
		{
			name:           "BootstrapError",
			nodeName:       "testNode",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(pubs []*models.VolumePublicationExternal) {
				assert.Nil(t, pubs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.ListVolumePublicationsForNode(testCtx, tt.nodeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}
		})
	}
}

func TestPublishVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		volumeName  string
		nodeName    string
		setupMocks  func(*testing.T, *gomock.Controller, *ConcurrentTridentOrchestrator)
		verifyError func(*testing.T, error)
	}{
		{
			name:       "Success",
			volumeName: "testVolume",
			nodeName:   "testNode",
			setupMocks: func(t *testing.T, mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				driver := mockstorage.NewMockDriver(mockCtrl)
				driver.EXPECT().Name().Return(config.FakeStorageDriverName).AnyTimes()
				driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				fakeNode := getFakeNode("testNode")
				addNodesToCache(t, fakeNode)
				fakeBackend := getFakeBackend("testBackend", "uuid", driver)
				addBackendsToCache(t, fakeBackend)
				fakeVolume := getFakeVolume("testVolume", "uuid")
				addVolumesToCache(t, fakeVolume)
				addVolumesToPersistence(t, o, fakeVolume)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:       "NotFound",
			volumeName: "nonExistentVolume",
			nodeName:   "testNode",
			setupMocks: func(t *testing.T, _ *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeNode := getFakeNode("testNode")
				addNodesToCache(t, fakeNode)
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
			},
			verifyError: func(t *testing.T, err error) {
				assert.True(t, errors.IsNotFoundError(err))
				assert.ErrorContains(t, err, "volume nonExistentVolume was not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			mockCtrl := gomock.NewController(t)
			db.Initialize()

			o := getConcurrentOrchestrator()
			if tt.setupMocks != nil {
				tt.setupMocks(t, mockCtrl, o)
			}

			err := o.PublishVolume(testCtx, tt.volumeName, &models.VolumePublishInfo{HostName: tt.nodeName})
			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

// TestPublishVolume_VerifyVolumePublicationFields tests that VolumePublication is created with all fields
// including the new StorageClass, BackendUUID, and Pool fields from the commit
func TestPublishVolume_VerifyVolumePublicationFields(t *testing.T) {
	config.CurrentDriverContext = config.ContextCSI
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	db.Initialize()
	o := getConcurrentOrchestrator()

	// Setup test data
	volumeName := "test-volume"
	nodeName := "test-node"
	backendUUID := "backend-uuid-123"
	poolName := "aggr1"
	storageClassName := "gold"

	// Create fake volume with all fields set
	fakeVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName: volumeName,
			Name:         volumeName,
			StorageClass: storageClassName,
		},
		BackendUUID: backendUUID,
		Pool:        poolName,
	}

	// Setup mocks
	driver := mockstorage.NewMockDriver(mockCtrl)
	driver.EXPECT().Name().Return(config.FakeStorageDriverName).AnyTimes()
	driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
	driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
	driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	fakeNode := getFakeNode(nodeName)
	addNodesToCache(t, fakeNode)
	fakeBackend := getFakeBackend("testBackend", backendUUID, driver)
	addBackendsToCache(t, fakeBackend)
	addVolumesToCache(t, fakeVolume)
	addVolumesToPersistence(t, o, fakeVolume)

	// Execute PublishVolume
	publishInfo := &models.VolumePublishInfo{HostName: nodeName}
	err := o.PublishVolume(testCtx, volumeName, publishInfo)
	assert.NoError(t, err, "PublishVolume should succeed")

	// Verify VolumePublication was created with all new fields
	pub, err := o.GetVolumePublication(testCtx, volumeName, nodeName)
	assert.NoError(t, err, "GetVolumePublication should succeed")
	assert.NotNil(t, pub, "VolumePublication should exist")

	// Verify the new fields are set correctly
	assert.Equal(t, storageClassName, pub.StorageClass, "StorageClass should match volume's storage class")
	assert.Equal(t, backendUUID, pub.BackendUUID, "BackendUUID should match backend UUID")
	assert.Equal(t, poolName, pub.Pool, "Pool should match volume's pool")

	// Also verify existing fields still work
	assert.Equal(t, volumeName, pub.VolumeName, "VolumeName should match")
	assert.Equal(t, nodeName, pub.NodeName, "NodeName should match")

	persistenceCleanup(t, o)
}

// TestPublishVolume_VerifyVolumePublicationFields_EmptyValues tests VolumePublication creation with empty new fields
func TestPublishVolume_VerifyVolumePublicationFields_EmptyValues(t *testing.T) {
	config.CurrentDriverContext = config.ContextCSI
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	db.Initialize()
	o := getConcurrentOrchestrator()

	// Setup test data with empty values for new fields
	volumeName := "test-volume"
	nodeName := "test-node"
	backendUUID := "backend-uuid-456"

	// Create fake volume with empty StorageClass and Pool
	fakeVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName: volumeName,
			Name:         volumeName,
			StorageClass: "", // Empty storage class
		},
		BackendUUID: backendUUID,
		Pool:        "", // Empty pool
	}

	// Setup mocks
	driver := mockstorage.NewMockDriver(mockCtrl)
	driver.EXPECT().Name().Return(config.FakeStorageDriverName).AnyTimes()
	driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
	driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
	driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	fakeNode := getFakeNode(nodeName)
	addNodesToCache(t, fakeNode)
	fakeBackend := getFakeBackend("testBackend", backendUUID, driver)
	addBackendsToCache(t, fakeBackend)
	addVolumesToCache(t, fakeVolume)
	addVolumesToPersistence(t, o, fakeVolume)

	// Execute PublishVolume
	publishInfo := &models.VolumePublishInfo{HostName: nodeName}
	err := o.PublishVolume(testCtx, volumeName, publishInfo)
	assert.NoError(t, err, "PublishVolume should succeed")

	// Verify VolumePublication was created with empty fields preserved
	pub, err := o.GetVolumePublication(testCtx, volumeName, nodeName)
	assert.NoError(t, err, "GetVolumePublication should succeed")
	assert.NotNil(t, pub, "VolumePublication should exist")

	// Verify the new fields are empty as expected
	assert.Equal(t, "", pub.StorageClass, "StorageClass should be empty")
	assert.Equal(t, backendUUID, pub.BackendUUID, "BackendUUID should still be set")
	assert.Equal(t, "", pub.Pool, "Pool should be empty")

	persistenceCleanup(t, o)
}

// TestPublishVolumeConcurrentCore_PublicationAlreadyExists tests that PublishVolume
// handles AlreadyExistsError gracefully when the volume publication already exists
// (e.g., during Kubernetes retries of ControllerPublishVolume).
func TestPublishVolumeConcurrentCore_PublicationAlreadyExists(t *testing.T) {
	config.CurrentDriverContext = config.ContextCSI
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	db.Initialize()

	// Create mock store client that returns AlreadyExistsError for AddVolumePublication
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Create the orchestrator with mock store client
	o := getConcurrentOrchestrator()
	o.storeClient = mockStoreClient

	// Setup driver mock
	driver := mockstorage.NewMockDriver(mockCtrl)
	driver.EXPECT().Name().Return(config.FakeStorageDriverName).AnyTimes()
	driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
	driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
	driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Setup cache with required resources
	fakeNode := getFakeNode("testNode")
	addNodesToCache(t, fakeNode)
	fakeBackend := getFakeBackend("testBackend", "uuid", driver)
	addBackendsToCache(t, fakeBackend)
	fakeVolume := getFakeVolume("testVolume", "uuid")
	addVolumesToCache(t, fakeVolume)

	// Mock store client expectations
	// AddVolumePublication returns AlreadyExistsError (simulating Kubernetes retry scenario)
	alreadyExistsErr := persistentstore.NewAlreadyExistsError("TridentVolumePublication", "testVolume.testNode")
	mockStoreClient.EXPECT().AddVolumePublication(gomock.Any(), gomock.Any()).Return(alreadyExistsErr)
	// UpdateVolume should still be called after handling AlreadyExistsError
	mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil)

	// Call PublishVolume - should succeed despite AlreadyExistsError
	err := o.PublishVolume(testCtx, "testVolume", &models.VolumePublishInfo{HostName: "testNode"})

	// Verify no error is returned (AlreadyExistsError is handled gracefully)
	assert.NoError(t, err, "PublishVolume should succeed even when publication already exists")
}

func TestUnpublishVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		volumeName  string
		nodeName    string
		setupMocks  func(*testing.T, *ConcurrentTridentOrchestrator)
		verifyError func(*testing.T, error)
	}{
		{
			name:       "Success",
			volumeName: "testVolume",
			nodeName:   "testNode",
			setupMocks: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				fakeNode := getFakeNode("testNode")
				addNodesToCache(t, fakeNode)
				fakeBackend := getFakeBackend("testBackend", "uuid", nil)
				addBackendsToCache(t, fakeBackend)
				fakeVolume := getFakeVolume("testVolume", "uuid")
				addVolumesToCache(t, fakeVolume)
				addVolumesToPersistence(t, o, fakeVolume)
				fakeVolumePublication := getFakeVolumePublication("testVolume", "testNode")
				addVolumePublicationsToCache(t, fakeVolumePublication)
				addVolumePublicationsToPersistence(t, o, fakeVolumePublication)
			},
			verifyError: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()

			o := getConcurrentOrchestrator()
			if tt.setupMocks != nil {
				tt.setupMocks(t, o)
			}

			err := o.UnpublishVolume(testCtx, tt.volumeName, tt.nodeName)
			if tt.verifyError != nil {
				tt.verifyError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestResizeVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		volumeName     string
		newSize        string
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError    func(err error)
		verifyBehavior func(t *testing.T, o *ConcurrentTridentOrchestrator)
	}{
		{
			name:       "Success",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the volume is updated in the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "107374182400", volume.Config.Size)
			},
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonExistentVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume nonExistentVolume not found")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No additional behavior to verify
			},
		},
		{
			name:       "BackendNotFound",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "nonexistent-backend-uuid",
				}
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "unable to find backend nonexistent-backend-uuid during volume resize")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No additional behavior to verify
			},
		},
		{
			name:       "VolumeDeleting",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateDeleting,
				}

				addVolumesToCache(t, volume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume testVolume is deleting")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No additional behavior to verify
			},
		},
		{
			name:       "OrphanedVolumeResizeError",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
					Orphaned:    true,
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().ResizeVolume(gomock.Any(), volume.Config, "100Gi").Return(errors.New("volume resize failed")).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume resize failed")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// No additional behavior to verify
			},
		},
		{
			name:       "UpdatePersistenceError",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().ResizeVolume(gomock.Any(), volume.Config, "100Gi").Return(nil).Times(1)

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "persistence error")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the volume is not updated in the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
		{
			name:       "AddVolumeTransactionError",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
				}
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "persistence error")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the volume is not updated in the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
		{
			name:       "DeleteVolumeTransactionError",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
				}
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)

				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "persistence error")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// The volume cache should still be updated with the new value
				volume := getVolumeByNameFromCache(t, "testVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "107374182400", volume.Config.Size)
			},
		},
		{
			name:       "HandlePreviouslyFailedResizeTxn_Success",
			volumeName: "testVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "50Gi"},
					BackendUUID: "backend-uuid1",
				}
				existingVolTxn := &storage.VolumeTransaction{
					Op:     storage.ResizeVolume,
					Config: &storage.VolumeConfig{Name: "testVolume", Size: "100Gi"},
				}

				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)

				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), gomock.Any()).Return(existingVolTxn, nil).Times(1)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				mockStoreClient.EXPECT().AddVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the volume is updated in the cache
				volume := getVolumeByNameFromCache(t, "testVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "107374182400", volume.Config.Size)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			err := o.ResizeVolume(testCtx, tt.volumeName, tt.newSize)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t, o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestResizeSubordinateVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		volumeName     string
		newSize        string
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyError    func(err error)
		verifyBehavior func(t *testing.T, o *ConcurrentTridentOrchestrator)
	}{
		{
			name:       "Success",
			volumeName: "testSubVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "100Gi"},
					BackendUUID: "backend-uuid1",
				}
				subVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testSubVolume", Size: "50Gi", ShareSourceVolume: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)
				addSubordinateVolumesToCache(t, subVolume)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the volume is updated in the cache
				volume := getSubVolumeByNameFromCache(t, "testSubVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "107374182400", volume.Config.Size)
			},
		},
		{
			name:       "SourceVolumeNotFound",
			volumeName: "testSubVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				subVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testSubVolume", Size: "50Gi", ShareSourceVolume: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addSubordinateVolumesToCache(t, subVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "source volume testVolume for subordinate volume testSubVolume not found")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the sub volume is not updated in the cache
				volume := getSubVolumeByNameFromCache(t, "testSubVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
		{
			name:       "NewSizeConversionError",
			volumeName: "testSubVolume",
			newSize:    "garbageSize123Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "100Gi"},
					BackendUUID: "backend-uuid1",
				}
				subVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testSubVolume", Size: "50Gi", ShareSourceVolume: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)
				addSubordinateVolumesToCache(t, subVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not convert volume size garbageSize123Gi: invalid size value 'garbagesize123': strconv.ParseInt: parsing \"garbagesize123\": invalid syntax")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the sub volume is not updated in the cache
				volume := getSubVolumeByNameFromCache(t, "testSubVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
		{
			name:       "SrcVolSizeConversionError",
			volumeName: "testSubVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "garbabeSize123Gi"},
					BackendUUID: "backend-uuid1",
				}
				subVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testSubVolume", Size: "50Gi", ShareSourceVolume: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)
				addSubordinateVolumesToCache(t, subVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not convert source volume size garbabeSize123Gi: invalid size value 'garbabesize123': strconv.ParseInt: parsing \"garbabesize123\": invalid syntax")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the sub volume is not updated in the cache
				volume := getSubVolumeByNameFromCache(t, "testSubVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
		{
			name:       "LargeResizeRequest",
			volumeName: "testSubVolume",
			newSize:    "1200Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "100Gi"},
					BackendUUID: "backend-uuid1",
				}
				subVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testSubVolume", Size: "50Gi", ShareSourceVolume: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)
				addSubordinateVolumesToCache(t, subVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "subordinate volume testSubVolume may not be larger than source volume testVolume")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the sub volume is not updated in the cache
				volume := getSubVolumeByNameFromCache(t, "testSubVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
		{
			name:       "UpdatePersistenceError",
			volumeName: "testSubVolume",
			newSize:    "100Gi",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				fakeBackend := getFakeBackend("testBackend", "backend-uuid1", nil)
				volume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testVolume", Size: "100Gi"},
					BackendUUID: "backend-uuid1",
				}
				subVolume := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "testSubVolume", Size: "50Gi", ShareSourceVolume: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addBackendsToCache(t, fakeBackend)
				addVolumesToCache(t, volume)
				addSubordinateVolumesToCache(t, subVolume)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(errors.New("persistence error")).Times(1)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "persistence error")
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify the sub volume is not updated in the cache
				volume := getSubVolumeByNameFromCache(t, "testSubVolume")
				require.NotNil(t, volume)
				assert.Equal(t, "50Gi", volume.Config.Size)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			err := o.ResizeVolume(testCtx, tt.volumeName, tt.newSize)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t, o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetCHAPConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		volumeName     string
		nodeName       string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(chap *models.IscsiChapInfo)
	}{
		{
			name:       "Success",
			volumeName: "testVolume",
			nodeName:   "testNode",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().GetChapInfo(gomock.Any(), "testVolume", "testNode").Return(&models.IscsiChapInfo{
					UseCHAP:              false,
					IscsiUsername:        "someUser",
					IscsiInitiatorSecret: "someSecret",
					IscsiTargetUsername:  "someTargetUser",
					IscsiTargetSecret:    "someTargetSecret",
				}, nil)

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "testVolume", Name: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(chap *models.IscsiChapInfo) {
				assert.Equal(t, chap.IscsiInitiatorSecret, "someSecret")
				assert.Equal(t, chap.IscsiUsername, "someUser")
				assert.Equal(t, chap.IscsiTargetSecret, "someTargetSecret")
				assert.Equal(t, chap.IscsiTargetUsername, "someTargetUser")
			},
		},
		{
			name:           "VolumeNotFound",
			volumeName:     "nonExistentVolume",
			nodeName:       "testNode",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "testVolume", Name: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "volume nonExistentVolume was not found")
			},
			verifyResult: func(chap *models.IscsiChapInfo) {
				assert.Nil(t, chap)
			},
		},
		{
			name:           "BackendNotFound",
			volumeName:     "testVolume",
			nodeName:       "testNode",
			bootstrapError: nil,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "testVolume", Name: "testVolume"},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend backend-uuid1 not found for volume testVolume")
			},
			verifyResult: func(chap *models.IscsiChapInfo) {
				assert.Nil(t, chap)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.GetCHAP(testCtx, tt.volumeName, tt.nodeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestRemoveBackendConfigRef(t *testing.T) {
	tests := []struct {
		name        string
		backendUUID string
		configRef   string
		setupMocks  func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name:        "Success",
			backendUUID: "backend-uuid1",
			configRef:   "config-ref1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().ConfigRef().Return("config-ref1").Times(1)
				mockBackend.EXPECT().SetConfigRef("").Times(1)

				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), mockBackend).Return(nil).Times(1)

				o.storeClient = mockStoreClient

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "BackendNotFound",
			backendUUID: "nonexistent-backend-uuid",
			configRef:   "config-ref1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No backend added to cache
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend with UUID 'nonexistent-backend-uuid' not found")
			},
		},
		{
			name:        "ConfigRefMismatch",
			backendUUID: "backend-uuid1",
			configRef:   "wrong-config-ref",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().ConfigRef().Return("config-ref1").Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "TridentBackendConfig with UID 'wrong-config-ref' cannot request removal of configRef 'config-ref1' for backend with UUID 'backend-uuid1'")
			},
		},
		{
			name:        "UpdateBackendError",
			backendUUID: "backend-uuid1",
			configRef:   "config-ref1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().ConfigRef().Return("config-ref1").Times(1)
				mockBackend.EXPECT().SetConfigRef("").Times(1)

				mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), mockBackend).Return(errors.New("update error")).Times(1)

				o.storeClient = mockStoreClient

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "failed to remove configRef 'config-ref1'")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.RemoveBackendConfigRef(testCtx, tt.backendUUID, tt.configRef)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestCheckForBackendNameChange(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		backendName string
		want        string
		wantErr     bool
	}{
		{
			name:        "No change in backend name",
			configJSON:  `{"version":1,"backendName":"ontap1"}`,
			backendName: "ontap1",
			want:        "",
			wantErr:     false,
		},
		{
			name:        "Backend name changed",
			configJSON:  `{"version":1,"backendName":"ontap2"}`,
			backendName: "ontap1",
			want:        "ontap2",
			wantErr:     false,
		},
		{
			name:        "No backend name in config",
			configJSON:  `{"version":1}`,
			backendName: "ontap1",
			want:        "",
			wantErr:     false,
		},
		{
			name:        "Invalid JSON",
			configJSON:  `{invalid`,
			backendName: "ontap1",
			want:        "",
			wantErr:     true,
		},
	}

	o := &ConcurrentTridentOrchestrator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := o.checkForBackendNameChange(tt.configJSON, tt.backendName)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestCleanupDeletingBackendsConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient)
		verifyBehavior func(t *testing.T, o *ConcurrentTridentOrchestrator)
	}{
		{
			name: "Success_CleanupDeletingBackendsWithoutVolumes",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend1 := getFakeBackend("testBackend1", "backend-uuid1", nil)
				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				// Backend1 is deleting and has no volumes
				mockBackend1.SetState(storage.Deleting)
				mockBackend1.SetOnline(true)

				// Backend2 is online, should not be cleaned up
				mockBackend2.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend2.EXPECT().HasVolumes().Return(false).Times(1)
				mockStoreClient.EXPECT().IsBackendDeleting(gomock.Any(), mockBackend2).Return(false).Times(1)

				addBackendsToCache(t, mockBackend1, mockBackend2)

				// Expect deletion of backend1
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend1).Return(nil).Times(1)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify backend1 was removed from cache
				backend := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, backend)

				// Verify backend2 is still in cache
				backend2 := getBackendByUuidFromCache(t, "backend-uuid2")
				assert.NotNil(t, backend2)
			},
		},
		{
			name: "Success_CleanupBackendMarkedDeletingByStore",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				// Backend is online but store client says it's deleting
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(false).Times(1)
				mockBackend.EXPECT().Terminate(gomock.Any()).Times(1)

				addBackendsToCache(t, mockBackend)

				mockStoreClient.EXPECT().IsBackendDeleting(gomock.Any(), mockBackend).Return(true).Times(1)
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend).Return(nil).Times(1)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify backend was removed from cache
				backend := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.Nil(t, backend)
			},
		},
		{
			name: "SkipFailedBackends",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend := getFakeBackend("testBackend1", "backend-uuid1", nil)

				// Backend is failed - should be skipped
				mockBackend.SetState(storage.Failed)

				addBackendsToCache(t, mockBackend)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify backend is still in cache (not cleaned up)
				backend := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, backend)
			},
		},
		{
			name: "SkipBackendsWithVolumes",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend := mockstorage.NewMockBackend(mockCtrl)

				// Backend is deleting but has volumes - should be skipped
				mockBackend.EXPECT().Name().Return("testBackend1").AnyTimes()
				mockBackend.EXPECT().BackendUUID().Return("backend-uuid1").AnyTimes()
				mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
				mockBackend.EXPECT().GetDriverName().Return("ontap-nas").AnyTimes()
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend.EXPECT().Online().Return(true).AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(true).AnyTimes()
				mockBackend.EXPECT().SmartCopy().Return(mockBackend).AnyTimes()
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend1").AnyTimes()
				mockBackend.EXPECT().ConstructPersistent(gomock.Any()).Return(&storage.BackendPersistent{Name: "testBackend1", BackendUUID: "backend-uuid1"}).AnyTimes()

				addBackendsToCache(t, mockBackend)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify backend is still in cache (not cleaned up)
				backend := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, backend)
			},
		},
		{
			name: "SkipOnlineBackendsNotMarkedDeletingByStore",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				// Backend is online and store doesn't mark it as deleting
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(false).Times(1)

				addBackendsToCache(t, mockBackend)

				mockStoreClient.EXPECT().IsBackendDeleting(gomock.Any(), mockBackend).Return(false).Times(1)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify backend is still in cache (not cleaned up)
				backend := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, backend)
			},
		},
		{
			name: "DeleteBackendStoreError_ContinueWithOthers",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend1 := getFakeBackend("testBackend1", "backend-uuid1", nil)
				mockBackend2 := getFakeBackend("testBackend2", "backend-uuid2", nil)

				// Both backends are deleting and have no volumes
				mockBackend1.SetState(storage.Deleting)
				mockBackend2.SetState(storage.Deleting)

				addBackendsToCache(t, mockBackend1, mockBackend2)

				// First backend deletion fails, second succeeds
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend1).Return(errors.New("store error")).Times(1)
				mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend2).Return(nil).Times(1)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify first backend is still in cache (deletion failed)
				backend1 := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, backend1)

				// Verify second backend was removed from cache
				backend2 := getBackendByUuidFromCache(t, "backend-uuid2")
				assert.Nil(t, backend2)
			},
		},
		{
			name: "NoBackendsToCleanup",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				// Backend is online and has volumes
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
				mockBackend.EXPECT().HasVolumes().Return(true).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify backend is still in cache
				backend := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, backend)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient)
			}

			o.cleanupDeletingBackends(testCtx)

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t, o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestValidateBackendUpdateConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller) (storage.Backend, storage.Backend)
		verifyError func(err error)
	}{
		{
			name: "Success_SameDriverType",
			setupMocks: func(mockCtrl *gomock.Controller) (storage.Backend, storage.Backend) {
				oldBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				newBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				return oldBackend, newBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Error_DifferentDriverTypes",
			setupMocks: func(mockCtrl *gomock.Controller) (storage.Backend, storage.Backend) {
				oldBackend := mockstorage.NewMockBackend(mockCtrl)
				oldBackend.EXPECT().Name().Return("testBackend1").AnyTimes()
				oldBackend.EXPECT().BackendUUID().Return("backend-uuid1").AnyTimes()
				oldBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
				oldBackend.EXPECT().GetDriverName().Return("ontap-nas").AnyTimes()

				newBackend := mockstorage.NewMockBackend(mockCtrl)
				newBackend.EXPECT().Name().Return("testBackend1").AnyTimes()
				newBackend.EXPECT().BackendUUID().Return("backend-uuid1").AnyTimes()
				newBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
				newBackend.EXPECT().GetDriverName().Return("ontap-san").AnyTimes()

				return oldBackend, newBackend
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "cannot update the backend as the old backend is of type ontap-nas and the new backend is of type ontap-san")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			o := getConcurrentOrchestrator()

			oldBackend, newBackend := tt.setupMocks(mockCtrl)

			err := o.validateBackendUpdate(oldBackend, newBackend)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestUpdateBackendVolumesConcurrentCore(t *testing.T) {
	volConfig1 := &storage.VolumeConfig{
		Name:         "testVolume1",
		InternalName: "testVolume1",
	}
	volConfig2 := &storage.VolumeConfig{
		Name:         "testVolume2",
		InternalName: "testVolume2",
	}

	tests := []struct {
		name           string
		setupMocks     func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend
		verifyError    func(err error)
		verifyBehavior func(t *testing.T, o *ConcurrentTridentOrchestrator)
	}{
		{
			name: "Success_VolumeBecomesOrphaned",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockDriver := mockstorage.NewMockDriver(mockCtrl)

				// Setup volume that exists in cache but not on backend (should become orphaned)
				vol := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    false,
				}

				addVolumesToCache(t, vol)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().Driver().Return(mockDriver).Times(1)
				mockDriver.EXPECT().Get(gomock.Any(), volConfig1).Return(errors.New("volume not found")).Times(1)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, volume *storage.Volume) error {
					assert.True(t, volume.Orphaned)
					return nil
				}).Times(1)

				mockBackend.EXPECT().Volumes().Return(&sync.Map{}).Times(1)

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify volume is marked as orphaned in cache
				vol := getVolumeByNameFromCache(t, "testVolume1")
				assert.NotNil(t, vol)
				assert.True(t, vol.Orphaned)
			},
		},
		{
			name: "Success_OrphanedVolumeRecovered",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockDriver := mockstorage.NewMockDriver(mockCtrl)

				// Setup orphaned volume that now exists on backend (should be recovered)
				vol := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    true,
				}

				addVolumesToCache(t, vol)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().Driver().Return(mockDriver).Times(1)
				mockDriver.EXPECT().Get(gomock.Any(), volConfig1).Return(nil).Times(1)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, volume *storage.Volume) error {
					assert.False(t, volume.Orphaned)
					return nil
				}).Times(1)

				mockBackend.EXPECT().Volumes().Return(&sync.Map{}).Times(1)

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify volume is no longer orphaned in cache
				vol := getVolumeByNameFromCache(t, "testVolume1")
				assert.NotNil(t, vol)
				assert.False(t, vol.Orphaned)
			},
		},
		{
			name: "Success_NoStateChange",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockDriver := mockstorage.NewMockDriver(mockCtrl)

				// Setup volume that exists and is not orphaned (no change needed)
				vol := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    false,
				}

				addVolumesToCache(t, vol)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().Driver().Return(mockDriver).Times(1)
				mockDriver.EXPECT().Get(gomock.Any(), volConfig1).Return(nil).Times(1)

				// No UpdateVolume call expected since state doesn't change
				mockBackend.EXPECT().Volumes().Return(&sync.Map{}).Times(1)

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Verify volume state unchanged
				vol := getVolumeByNameFromCache(t, "testVolume1")
				assert.NotNil(t, vol)
				assert.False(t, vol.Orphaned)
			},
		},
		{
			name: "Success_MultipleVolumes",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockDriver := mockstorage.NewMockDriver(mockCtrl)

				// Setup multiple volumes with different states
				vol1 := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    false,
				}
				vol2 := &storage.Volume{
					Config:      volConfig2,
					BackendUUID: "backend-uuid1",
					Orphaned:    true,
				}

				addVolumesToCache(t, vol1, vol2)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().Driver().Return(mockDriver).Times(2)
				// Vol1 not found on backend (becomes orphaned)
				mockDriver.EXPECT().Get(gomock.Any(), volConfig1).Return(errors.New("not found")).Times(1)
				// Vol2 found on backend (recovered)
				mockDriver.EXPECT().Get(gomock.Any(), volConfig2).Return(nil).Times(1)

				// Expect two update calls
				gomock.InOrder(
					mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, volume *storage.Volume) error {
						if volume.Config.Name == "testVolume1" {
							assert.True(t, volume.Orphaned)
						}
						return nil
					}).Times(1),
					mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, volume *storage.Volume) error {
						if volume.Config.Name == "testVolume2" {
							assert.False(t, volume.Orphaned)
						}
						return nil
					}).Times(1),
				)

				mockBackend.EXPECT().Volumes().Return(&sync.Map{}).Times(2)

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				vol1 := getVolumeByNameFromCache(t, "testVolume1")
				assert.NotNil(t, vol1)
				assert.True(t, vol1.Orphaned)

				vol2 := getVolumeByNameFromCache(t, "testVolume2")
				assert.NotNil(t, vol2)
				assert.False(t, vol2.Orphaned)
			},
		},
		{
			name: "Success_BackendNotFound",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				vol := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    false,
				}

				addVolumesToCache(t, vol)
				// Don't add backend to cache - it will be not found after lock

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Function should return nil without error (best effort)
			},
		},
		{
			name: "Success_VolumeDeletedAfterQuery",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				vol := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    false,
				}

				addVolumesToCache(t, vol)
				addBackendsToCache(t, mockBackend)

				// Simulate volume being deleted after initial query by removing it from cache
				removeVolumeFromCache(t, "testVolume1")

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Should handle gracefully when volume is nil
			},
		},
		{
			name: "Success_UpdateVolumeStoreError",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockDriver := mockstorage.NewMockDriver(mockCtrl)

				vol := &storage.Volume{
					Config:      volConfig1,
					BackendUUID: "backend-uuid1",
					Orphaned:    false,
				}

				addVolumesToCache(t, vol)
				addBackendsToCache(t, mockBackend)

				mockBackend.EXPECT().Driver().Return(mockDriver).Times(1)
				mockDriver.EXPECT().Get(gomock.Any(), volConfig1).Return(errors.New("not found")).Times(1)

				// Simulate store update failure
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(errors.New("store error")).Times(1)

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err) // Function continues on store errors
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				vol := getVolumeByNameFromCache(t, "testVolume1")
				assert.NotNil(t, vol)
				// volume is not updated in cache, should still be not orphaned
				assert.False(t, vol.Orphaned)
			},
		},
		{
			name: "Success_NoVolumesInBackend",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) storage.Backend {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")

				// Add backend but no volumes
				addBackendsToCache(t, mockBackend)

				return mockBackend
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyBehavior: func(t *testing.T, o *ConcurrentTridentOrchestrator) {
				// Should handle case with no volumes gracefully
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			var backend storage.Backend
			if tt.setupMocks != nil {
				backend = tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.updateBackendVolumes(testCtx, backend)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t, o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestHandleFailedTransactionAddVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.AddVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteVolumeFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.AddVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success_NoVolume_MultipleBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.AddVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_NoVolume_MultipleBackends_DeleteVolumeFails",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.AddVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(failed).Times(1)

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success_NoVolume_NoBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.AddVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteVolumeTransactionFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.AddVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionDeleteVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.DeleteVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_DeleteVolumeFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.DeleteVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(failed)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NoVolume_MultipleBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.DeleteVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NoVolume_NoBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.DeleteVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteVolumeTransactionFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
				Op:     storage.DeleteVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), vol).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionAddSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.AddSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteSnapshotFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.AddSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success_NoSnapshot_MultipleBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.AddSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_NoSnapshot_MultipleBackends_DeleteSnapshotFails",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.AddSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(failed).Times(1)

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Success_NoSnapshot_NoBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.AddSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteVolumeTransactionFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.AddSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionDeleteSnapshotConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.DeleteSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_DeleteSnapshotFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.DeleteSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(failed).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NoSnapshot_MultipleBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.DeleteSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NoSnapshot_NoBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.DeleteSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteVolumeTransactionFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				SnapshotConfig: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				Op:             storage.DeleteSnapshot,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				snapshot := &storage.Snapshot{
					Config: &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "vol1"},
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)
				addSnapshotsToCache(t, snapshot)

				mockStoreClient.EXPECT().DeleteSnapshot(gomock.Any(), snapshot).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionResizeVolumeConcurrentCore(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1", Size: "2Gi"},
				Op:     storage.ResizeVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), "2Gi").Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1", Size: "1Gi"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_ResizeVolumeFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1", Size: "2Gi"},
				Op:     storage.ResizeVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), "2Gi").Return(failed).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1", Size: "1Gi"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NoVolume_MultipleBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1", Size: "2Gi"},
				Op:     storage.ResizeVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend1",
					"uuid":       "backend-uuid1",
					"state":      string(storage.Online),
					"driverName": "ontap-nas",
				})
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				mockBackend2 := getMockBackendWithMap(mockCtrl, map[string]string{
					"name":       "testBackend2",
					"uuid":       "backend-uuid2",
					"state":      string(storage.Failed),
					"driverName": "ontap-nas",
				})
				mockBackend2.EXPECT().GetUniqueKey().Return("testBackend2").AnyTimes()

				addBackendsToCache(t, mockBackend1, mockBackend2)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NoSnapshot_NoBackends",
			ctx:  expiredCtx,
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1", Size: "2Gi"},
				Op:     storage.ResizeVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteVolumeTransactionFails",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn: &storage.VolumeTransaction{
				Config: &storage.VolumeConfig{Name: "vol1", InternalName: "vol1", Size: "2Gi"},
				Op:     storage.ResizeVolume,
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), "2Gi").Return(nil).Times(1)

				mockBackend2 := getMockBackend(mockCtrl, "testBackend2", "backend-uuid2")

				vol := &storage.Volume{
					Config:      &storage.VolumeConfig{InternalName: "vol1", Name: "vol1", Size: "1Gi"},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateOnline,
				}

				addBackendsToCache(t, mockBackend1, mockBackend2)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionImportVolumeNotManagedConcurrentCore(t *testing.T) {
	txn := &storage.VolumeTransaction{
		Config: &storage.VolumeConfig{
			Name:               "vol1",
			InternalName:       "vol1",
			Size:               "2Gi",
			ImportOriginalName: "importMe",
			ImportBackendUUID:  "backend-uuid1",
			ImportNotManaged:   true,
		},
		Op: storage.ImportVolume,
	}

	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{InternalName: "importMe", Name: "vol1", Size: "1Gi"},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success_NotManaged_VolumeInCache",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_NotManaged_VolumeNotInCache",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend1)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_NotManaged_VolumeInCache_DeleteVolumeStoreError",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Failure_NotManaged_VolumeInCache_DeleteTransactionStoreError",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionImportVolumeManagedConcurrentCore(t *testing.T) {
	txn := &storage.VolumeTransaction{
		Config: &storage.VolumeConfig{
			Name:               "vol1",
			InternalName:       "vol1",
			Size:               "2Gi",
			ImportOriginalName: "importMe",
			ImportBackendUUID:  "backend-uuid1",
			ImportNotManaged:   false,
		},
		Op: storage.ImportVolume,
	}

	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{InternalName: "importMe", Name: "vol1", Size: "1Gi"},
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success_Managed_VolumeInCache_Rename",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RenameVolume(gomock.Any(), txn.Config, txn.Config.ImportOriginalName).Return(nil)

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_Managed_VolumeInCache_Rename_NoBackend",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_Managed_VolumeInCache_RenameFailed",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RenameVolume(gomock.Any(), txn.Config, txn.Config.ImportOriginalName).Return(failed)

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_Managed_VolumeNotInCache",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RenameVolume(gomock.Any(), txn.Config, txn.Config.ImportOriginalName).Return(failed)

				addBackendsToCache(t, mockBackend1)

				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_Managed_VolumeInCache_DeleteVolumeStoreError",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "Failure_Managed_VolumeInCache_DeleteTransactionStoreError",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend1 := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend1.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()
				mockBackend1.EXPECT().RenameVolume(gomock.Any(), txn.Config, txn.Config.ImportOriginalName).Return(nil)

				addBackendsToCache(t, mockBackend1)
				addVolumesToCache(t, vol)

				mockStoreClient.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestHandleFailedTransactionVolumeCreatingConcurrentCore(t *testing.T) {
	txn := &storage.VolumeTransaction{
		Config:               &storage.VolumeConfig{Name: "vol1", InternalName: "vol1"},
		VolumeCreatingConfig: &storage.VolumeCreatingConfig{Pool: "pool1", BackendUUID: "backend-uuid1"},
		Op:                   storage.VolumeCreating,
	}

	tests := []struct {
		name        string
		ctx         context.Context
		txn         *storage.VolumeTransaction
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "Success",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(nil)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Failure_DeleteTransactionStoreError",
			ctx:  expiredCtx, // Ensure we delete a transaction even if context is expired
			txn:  txn,
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), gomock.Any()).Return(failed)
			},
			verifyError: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.handleFailedTransaction(tt.ctx, tt.txn)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
		})
	}
}

func TestGetVolumeTransactionConcurrentCore(t *testing.T) {
	volTxn := &storage.VolumeTransaction{
		Config: &storage.VolumeConfig{Name: "vol1"},
		Op:     storage.AddVolume,
	}

	tests := []struct {
		name       string
		setupMocks func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		expectTxn  *storage.VolumeTransaction
		expectErr  bool
	}{
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), volTxn).Return(volTxn, nil)
			},
			expectTxn: volTxn,
			expectErr: false,
		},
		{
			name: "PersistenceFailure",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetVolumeTransaction(gomock.Any(), volTxn).Return(nil, failed)
			},
			expectTxn: nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.GetVolumeTransaction(testCtx, volTxn)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectTxn, result)
			}
		})
	}
}

func TestDeleteVolumeTransactionConcurrentCore(t *testing.T) {
	volTxn := &storage.VolumeTransaction{
		Config: &storage.VolumeConfig{Name: "vol1"},
		Op:     storage.AddVolume,
	}

	tests := []struct {
		name       string
		setupMocks func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		expectErr  bool
	}{
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), volTxn).Return(nil)
			},
			expectErr: false,
		},
		{
			name: "PersistenceFailure",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(gomock.Any(), volTxn).Return(failed)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.DeleteVolumeTransaction(testCtx, volTxn)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReleaseMirrorConcurrentCore(t *testing.T) {
	tests := []struct {
		name                    string
		backendUUID             string
		pvcVolumeName           string
		localInternalVolumeName string
		bootstrapError          error
		setupMocks              func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError             func(err error)
	}{
		{
			name:                    "Success",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReleaseMirror(gomock.Any(), "testLocalVolume").
					Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "BootstrapError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			bootstrapError:          errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:                    "BackendNotSupportMirroring",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"
				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
		},
		{
			name:                    "ReleaseMirrorError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReleaseMirror(gomock.Any(), "testLocalVolume").
					Return(errors.New("release mirror failed")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "release mirror failed")
			},
		},
		{
			name:                    "BackendNotFound",
			backendUUID:             "nonExistentBackend",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeVolume := getFakeVolume("testPVCVolume", "nonExistentBackend-Uuid1")

				addVolumesToCache(t, fakeVolume)
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				// The error will come from the db.Lock operation when backend is not found
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.ReleaseMirror(testCtx, tt.backendUUID, tt.pvcVolumeName, tt.localInternalVolumeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestCanBackendMirrorConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		backendUUID    string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(canMirror bool)
	}{
		{
			name:        "Success_BackendCanMirror",
			backendUUID: "backend-uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(canMirror bool) {
				assert.True(t, canMirror)
			},
		},
		{
			name:        "Success_BackendCannotMirror",
			backendUUID: "backend-uuid1",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(canMirror bool) {
				assert.False(t, canMirror)
			},
		},
		{
			name:           "BootstrapError",
			backendUUID:    "backend-uuid1",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(canMirror bool) {
				assert.False(t, canMirror)
			},
		},
		{
			name:        "BackendNotFound",
			backendUUID: "nonExistentBackend",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend with UUID nonExistentBackend was not found")
			},
			verifyResult: func(canMirror bool) {
				assert.False(t, canMirror)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.CanBackendMirror(testCtx, tt.backendUUID)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetMirrorStatusConcurrentCore(t *testing.T) {
	tests := []struct {
		name                    string
		backendUUID             string
		localInternalVolumeName string
		remoteVolumeHandle      string
		bootstrapError          error
		setupMocks              func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError             func(err error)
		verifyResult            func(status string)
	}{
		{
			name:                    "Success",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetMirrorStatus(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("established", nil).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(status string) {
				assert.Equal(t, "established", status)
			},
		},
		{
			name:                    "BootstrapError",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			bootstrapError:          errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(status string) {
				assert.Empty(t, status)
			},
		},
		{
			name:                    "BackendNotFound",
			backendUUID:             "nonExistentBackend",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend with UUID nonExistentBackend was not found")
			},
			verifyResult: func(status string) {
				assert.Empty(t, status)
			},
		},
		{
			name:                    "BackendNotSupportMirroring",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
			verifyResult: func(status string) {
				assert.Empty(t, status)
			},
		},
		{
			name:                    "GetMirrorStatusError",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetMirrorStatus(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("", errors.New("mirror status retrieval failed")).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "mirror status retrieval failed")
			},
			verifyResult: func(status string) {
				assert.Empty(t, status)
			},
		},
		{
			name:                    "VariousMirrorStatuses",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetMirrorStatus(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("replicating", nil).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(status string) {
				assert.Equal(t, "replicating", status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.GetMirrorStatus(testCtx, tt.backendUUID, tt.localInternalVolumeName, tt.remoteVolumeHandle)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestPromoteMirrorConcurrentCore(t *testing.T) {
	tests := []struct {
		name                    string
		backendUUID             string
		pvcVolumeName           string
		localInternalVolumeName string
		remoteVolumeHandle      string
		snapshotHandle          string
		bootstrapError          error
		setupMocks              func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError             func(err error)
		verifyResult            func(promoted bool)
	}{
		{
			name:                    "Success",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().PromoteMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume", "testSnapshot").
					Return(true, nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(promoted bool) {
				assert.True(t, promoted)
			},
		},
		{
			name:                    "BootstrapError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "testSnapshot",
			bootstrapError:          errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(promoted bool) {
				assert.False(t, promoted)
			},
		},
		{
			name:                    "BackendNotSupportMirroring",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"
				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
			verifyResult: func(promoted bool) {
				assert.False(t, promoted)
			},
		},
		{
			name:                    "PromoteMirrorError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().PromoteMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume", "testSnapshot").
					Return(false, errors.New("promote mirror failed")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "promote mirror failed")
			},
			verifyResult: func(promoted bool) {
				assert.False(t, promoted)
			},
		},
		{
			name:                    "BackendNotFound",
			backendUUID:             "nonExistentBackend",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeVolume := getFakeVolume("testPVCVolume", "nonExistentBackend-Uuid1")

				addVolumesToCache(t, fakeVolume)
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				// The error will come from the db.Lock operation when backend is not found
				assert.ErrorContains(t, err, "backend with UUID nonExistentBackend was not found")
			},
			verifyResult: func(promoted bool) {
				assert.False(t, promoted)
			},
		},
		{
			name:                    "PromoteMirrorReturnsFalse",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().PromoteMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume", "testSnapshot").
					Return(false, nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(promoted bool) {
				assert.False(t, promoted)
			},
		},
		{
			name:                    "EmptySnapshotHandle",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			snapshotHandle:          "",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().PromoteMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume", "").
					Return(true, nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(promoted bool) {
				assert.True(t, promoted)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.PromoteMirror(testCtx, tt.backendUUID, tt.pvcVolumeName,
				tt.localInternalVolumeName, tt.remoteVolumeHandle, tt.snapshotHandle)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestReestablishMirrorConcurrentCore(t *testing.T) {
	tests := []struct {
		name                    string
		backendUUID             string
		pvcVolumeName           string
		localInternalVolumeName string
		remoteVolumeHandle      string
		replicationPolicy       string
		replicationSchedule     string
		bootstrapError          error
		setupMocks              func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError             func(err error)
	}{
		{
			name:                    "Success",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReestablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "5minutely").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "BootstrapError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			bootstrapError:          errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:                    "BackendNotFound",
			backendUUID:             "nonExistentBackend",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeVolume := getFakeVolume("testPVCVolume", "nonExistentBackend-Uuid1")
				addVolumesToCache(t, fakeVolume)
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend with UUID nonExistentBackend was not found")
			},
		},
		{
			name:                    "BackendNotSupportMirroring",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"
				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
		},
		{
			name:                    "ReestablishMirrorError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReestablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "5minutely").Return(errors.New("reestablish mirror failed")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "reestablish mirror failed")
			},
		},
		{
			name:                    "EmptyReplicationPolicy",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReestablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"", "5minutely").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "EmptyReplicationSchedule",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReestablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "DifferentReplicationPolicies",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorLatest",
			replicationSchedule:     "hourly",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().ReestablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorLatest", "hourly").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.ReestablishMirror(testCtx, tt.backendUUID, tt.pvcVolumeName,
				tt.localInternalVolumeName, tt.remoteVolumeHandle,
				tt.replicationPolicy, tt.replicationSchedule)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestEstablishMirrorConcurrentCore(t *testing.T) {
	tests := []struct {
		name                    string
		backendUUID             string
		pvcVolumeName           string
		localInternalVolumeName string
		remoteVolumeHandle      string
		replicationPolicy       string
		replicationSchedule     string
		bootstrapError          error
		setupMocks              func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError             func(err error)
	}{
		{
			name:                    "Success",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "5minutely").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "BootstrapError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			bootstrapError:          errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:                    "BackendNotSupportMirroring",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"
				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
		},
		{
			name:                    "EstablishMirrorError",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "5minutely").Return(errors.New("establish mirror failed")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "establish mirror failed")
			},
		},
		{
			name:                    "BackendNotFound",
			backendUUID:             "nonExistentBackend",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeVolume := getFakeVolume("testPVCVolume", "nonExistentBackend-Uuid1")

				addVolumesToCache(t, fakeVolume)
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				// The error will come from the db.Lock operation when backend is not found
				assert.ErrorContains(t, err, "backend with UUID nonExistentBackend was not found")
			},
		},
		{
			name:                    "EmptyReplicationPolicy",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"", "5minutely").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "EmptyReplicationSchedule",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "").Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:                    "InvalidReplicationSchedule",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "invalid-schedule",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "testLocalVolume", "testRemoteVolume",
					"MirrorAllSnapshots", "invalid-schedule").Return(errors.New("invalid replication schedule")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid replication schedule")
			},
		},
		{
			name:                    "EmptyLocalInternalVolumeName",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "",
			remoteVolumeHandle:      "testRemoteVolume",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "", "testRemoteVolume",
					"MirrorAllSnapshots", "5minutely").Return(errors.New("invalid local volume name")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid local volume name")
			},
		},
		{
			name:                    "EmptyRemoteVolumeHandle",
			backendUUID:             "backend-uuid1",
			pvcVolumeName:           "testPVCVolume",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "",
			replicationPolicy:       "MirrorAllSnapshots",
			replicationSchedule:     "5minutely",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().EstablishMirror(gomock.Any(), "testLocalVolume", "",
					"MirrorAllSnapshots", "5minutely").Return(errors.New("invalid remote volume handle")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid remote volume handle")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.EstablishMirror(testCtx, tt.backendUUID, tt.pvcVolumeName,
				tt.localInternalVolumeName, tt.remoteVolumeHandle,
				tt.replicationPolicy, tt.replicationSchedule)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestGetReplicationDetailsConcurrentCore(t *testing.T) {
	tests := []struct {
		name                    string
		backendUUID             string
		localInternalVolumeName string
		remoteVolumeHandle      string
		bootstrapError          error
		setupMocks              func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError             func(err error)
		verifyResult            func(replicationPolicy, replicationSchedule, remoteVolumeState string)
	}{
		{
			name:                    "Success",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("MirrorAllSnapshots", "5minutely", "established", nil).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Equal(t, "MirrorAllSnapshots", replicationPolicy)
				assert.Equal(t, "5minutely", replicationSchedule)
				assert.Equal(t, "established", remoteVolumeState)
			},
		},
		{
			name:                    "BootstrapError",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			bootstrapError:          errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
		{
			name:                    "BackendNotFound",
			backendUUID:             "nonExistentBackend",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend with UUID nonExistentBackend was not found")
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
		{
			name:                    "BackendNotSupportMirroring",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
		{
			name:                    "GetReplicationDetailsError",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("", "", "", errors.New("get replication details failed")).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "get replication details failed")
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
		{
			name:                    "EmptyLocalInternalVolumeName",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "", "testRemoteVolume").
					Return("", "", "", errors.New("invalid local volume name")).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid local volume name")
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
		{
			name:                    "EmptyRemoteVolumeHandle",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "testLocalVolume", "").
					Return("", "", "", errors.New("invalid remote volume handle")).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "invalid remote volume handle")
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
		{
			name:                    "DifferentReplicationPolicies",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("MirrorLatest", "hourly", "replicating", nil).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Equal(t, "MirrorLatest", replicationPolicy)
				assert.Equal(t, "hourly", replicationSchedule)
				assert.Equal(t, "replicating", remoteVolumeState)
			},
		},
		{
			name:                    "VariousRemoteVolumeStates",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("MirrorAllSnapshots", "10minutely", "broken", nil).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Equal(t, "MirrorAllSnapshots", replicationPolicy)
				assert.Equal(t, "10minutely", replicationSchedule)
				assert.Equal(t, "broken", remoteVolumeState)
			},
		},
		{
			name:                    "EmptyReplicationDetails",
			backendUUID:             "backend-uuid1",
			localInternalVolumeName: "testLocalVolume",
			remoteVolumeHandle:      "testRemoteVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().GetReplicationDetails(gomock.Any(), "testLocalVolume", "testRemoteVolume").
					Return("", "", "", nil).Times(1)

				addBackendsToCache(t, mockBackend)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(replicationPolicy, replicationSchedule, remoteVolumeState string) {
				assert.Empty(t, replicationPolicy)
				assert.Empty(t, replicationSchedule)
				assert.Empty(t, remoteVolumeState)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			replicationPolicy, replicationSchedule, remoteVolumeState, err := o.GetReplicationDetails(testCtx,
				tt.backendUUID, tt.localInternalVolumeName, tt.remoteVolumeHandle)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(replicationPolicy, replicationSchedule, remoteVolumeState)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestUpdateMirrorConcurrentCore(t *testing.T) {
	tests := []struct {
		name           string
		pvcVolumeName  string
		snapshotName   string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
	}{
		{
			name:          "Success",
			pvcVolumeName: "testPVCVolume",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().UpdateMirror(gomock.Any(), "testPVCVolume", "testSnapshot").
					Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:           "BootstrapError",
			pvcVolumeName:  "testPVCVolume",
			snapshotName:   "testSnapshot",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
		},
		{
			name:          "VolumeNotFound",
			pvcVolumeName: "nonExistentVolume",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// Don't add any volume to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not find volume 'nonExistentVolume' in Trident")
			},
		},
		{
			name:          "BackendNotSupportMirroring",
			pvcVolumeName: "testPVCVolume",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"
				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
		},
		{
			name:          "UpdateMirrorError",
			pvcVolumeName: "testPVCVolume",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().UpdateMirror(gomock.Any(), "testPVCVolume", "testSnapshot").
					Return(errors.New("update mirror failed")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "update mirror failed")
			},
		},
		{
			name:          "BackendNotFound",
			pvcVolumeName: "testPVCVolume",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "nonExistentBackend"

				addVolumesToCache(t, fakeVolume)
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				// The error will come from the db.Lock operation when backend is not found
				assert.ErrorContains(t, err, "backend nonExistentBackend not found")
			},
		},
		{
			name:          "EmptySnapshotName",
			pvcVolumeName: "testPVCVolume",
			snapshotName:  "",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().UpdateMirror(gomock.Any(), "testPVCVolume", "").
					Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:          "EmptyPVCVolumeName",
			pvcVolumeName: "",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed as it should fail at getVolume
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not find volume '' in Trident")
			},
		},
		{
			name:          "DifferentInternalVolumeName",
			pvcVolumeName: "testPVCVolume",
			snapshotName:  "testSnapshot",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().UpdateMirror(gomock.Any(), "internal-testPVCVolume", "testSnapshot").
					Return(nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"
				fakeVolume.Config.InternalName = "internal-testPVCVolume"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.UpdateMirror(testCtx, tt.pvcVolumeName, tt.snapshotName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestCheckMirrorTransferStateConcurrentCore(t *testing.T) {
	transferTime := time.Now()

	tests := []struct {
		name           string
		pvcVolumeName  string
		bootstrapError error
		setupMocks     func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator)
		verifyError    func(err error)
		verifyResult   func(transferTime *time.Time)
	}{
		{
			name:          "Success",
			pvcVolumeName: "testPVCVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().CheckMirrorTransferState(gomock.Any(), "testPVCVolume").
					Return(&transferTime, nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *time.Time) {
				assert.NotNil(t, result)
				assert.Equal(t, transferTime, *result)
			},
		},
		{
			name:           "BootstrapError",
			pvcVolumeName:  "testPVCVolume",
			bootstrapError: errors.New("bootstrap error"),
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "bootstrap error")
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
		{
			name:          "VolumeNotFound",
			pvcVolumeName: "nonExistentVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// Don't add any volume to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not find volume 'nonExistentVolume' in Trident")
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
		{
			name:          "BackendNotFound",
			pvcVolumeName: "testPVCVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "nonExistentBackend"

				addVolumesToCache(t, fakeVolume)
				// Don't add any backend to simulate not found scenario
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend nonExistentBackend not found")
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
		{
			name:          "BackendNotSupportMirroring",
			pvcVolumeName: "testPVCVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(false)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "backend does not support mirroring")
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
		{
			name:          "CheckMirrorTransferStateError",
			pvcVolumeName: "testPVCVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().CheckMirrorTransferState(gomock.Any(), "testPVCVolume").
					Return(nil, errors.New("check transfer state failed")).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "check transfer state failed")
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
		{
			name:          "NilTransferTime",
			pvcVolumeName: "testPVCVolume",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend1", "backend-uuid1")
				mockBackend.EXPECT().CanMirror().Return(true)
				mockBackend.EXPECT().CheckMirrorTransferState(gomock.Any(), "testPVCVolume").
					Return(nil, nil).Times(1)

				fakeVolume := getFakeVolume("testPVCVolume", "volume-uuid1")
				fakeVolume.BackendUUID = "backend-uuid1"

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, fakeVolume)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
		{
			name:          "EmptyPVCVolumeName",
			pvcVolumeName: "",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed as it should fail at getVolume
			},
			verifyError: func(err error) {
				assert.ErrorContains(t, err, "could not find volume '' in Trident")
			},
			verifyResult: func(result *time.Time) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()
			o.bootstrapError = tt.bootstrapError

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			result, err := o.CheckMirrorTransferState(testCtx, tt.pvcVolumeName)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestUnpublishVolumeWithNVMeNamespaceUUIDs(t *testing.T) {
	db.Initialize()
	o := getConcurrentOrchestrator()

	// Create node with NQN
	fakeNode := &models.Node{
		Name: "testNode",
		NQN:  "nqn.2014-08.org.nvmexpress:uuid:test-host-nqn",
	}
	addNodesToCache(t, fakeNode)

	// Create backend
	fakeBackend := getFakeBackend("testBackend", "uuid", nil)
	addBackendsToCache(t, fakeBackend)

	// Create NVMe volume with namespace UUID
	nvmeVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName: "nvmeVolume",
			Name:         "nvmeVolume",
			AccessInfo: models.VolumeAccessInfo{
				NVMeAccessInfo: models.NVMeAccessInfo{
					NVMeNamespaceUUID: "ns-uuid-1",
				},
			},
		},
		BackendUUID: "uuid",
	}
	addVolumesToCache(t, nvmeVolume)
	addVolumesToPersistence(t, o, nvmeVolume)

	// Create another NVMe volume published to same node
	otherNVMeVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			InternalName: "otherVolume",
			Name:         "otherVolume",
			AccessInfo: models.VolumeAccessInfo{
				NVMeAccessInfo: models.NVMeAccessInfo{
					NVMeNamespaceUUID: "ns-uuid-2",
				},
			},
		},
		BackendUUID: "uuid",
	}
	addVolumesToCache(t, otherNVMeVolume)

	// Create publications
	pub1 := &models.VolumePublication{
		VolumeName: "nvmeVolume",
		NodeName:   "testNode",
	}
	pub2 := &models.VolumePublication{
		VolumeName: "otherVolume",
		NodeName:   "testNode",
	}
	addVolumePublicationsToCache(t, pub1, pub2)
	addVolumePublicationsToPersistence(t, o, pub1)

	// Unpublish first volume - should collect namespace UUIDs
	err := o.UnpublishVolume(testCtx, "nvmeVolume", "testNode")
	assert.NoError(t, err)

	persistenceCleanup(t, o)
}

func TestPublishVolumeAutogrowIneligible(t *testing.T) {
	tests := []struct {
		name                       string
		volumeName                 string
		nodeName                   string
		expectedAutogrowIneligible bool
		setupMocks                 func(*testing.T, *gomock.Controller, *ConcurrentTridentOrchestrator)
	}{
		{
			name:                       "UnmanagedImport",
			volumeName:                 "unmanagedVolume",
			nodeName:                   "testNode",
			expectedAutogrowIneligible: true,
			setupMocks: func(t *testing.T, mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				driver := mockstorage.NewMockDriver(mockCtrl)
				driver.EXPECT().Name().Return(config.FakeStorageDriverName).AnyTimes()
				driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				fakeNode := getFakeNode("testNode")
				addNodesToCache(t, fakeNode)
				fakeBackend := getFakeBackend("testBackend", "uuid", driver)
				addBackendsToCache(t, fakeBackend)
				fakeVolume := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:             "unmanagedVolume",
						InternalName:     "unmanagedVolume",
						ImportNotManaged: true,
					},
					BackendUUID: "uuid",
				}
				addVolumesToCache(t, fakeVolume)
				addVolumesToPersistence(t, o, fakeVolume)
			},
		},
		{
			name:                       "NormalVolume",
			volumeName:                 "normalVolume",
			nodeName:                   "testNode",
			expectedAutogrowIneligible: false,
			setupMocks: func(t *testing.T, mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				driver := mockstorage.NewMockDriver(mockCtrl)
				driver.EXPECT().Name().Return(config.FakeStorageDriverName).AnyTimes()
				driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				fakeNode := getFakeNode("testNode")
				addNodesToCache(t, fakeNode)
				fakeBackend := getFakeBackend("testBackend", "uuid", driver)
				addBackendsToCache(t, fakeBackend)
				fakeVolume := getFakeVolume("normalVolume", "uuid")
				addVolumesToCache(t, fakeVolume)
				addVolumesToPersistence(t, o, fakeVolume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			mockCtrl := gomock.NewController(t)
			db.Initialize()

			o := getConcurrentOrchestrator()
			if tt.setupMocks != nil {
				tt.setupMocks(t, mockCtrl, o)
			}

			err := o.PublishVolume(testCtx, tt.volumeName, &models.VolumePublishInfo{HostName: tt.nodeName})
			assert.NoError(t, err, "PublishVolume should succeed for %s", tt.name)

			// Verify VolumePublication was created
			pub, err := o.GetVolumePublication(testCtx, tt.volumeName, tt.nodeName)
			assert.NoError(t, err, "GetVolumePublication should succeed")
			assert.NotNil(t, pub, "VolumePublication should exist")

			// Verify AutogrowIneligible field
			assert.Equal(t, tt.expectedAutogrowIneligible, pub.AutogrowIneligible,
				"AutogrowIneligible should match expected value")

			persistenceCleanup(t, o)
		})
	}
}

// ============================================================================
// Autogrow Policy Tests
// ============================================================================

func TestConcurrent_BootstrapAutogrowPolicies(t *testing.T) {
	tests := []struct {
		name              string
		driverContext     config.DriverContext
		setupMocks        func(*gomock.Controller, *ConcurrentTridentOrchestrator)
		verifyError       func(error)
		verifyPolicyCount int
	}{
		{
			name:          "Non-CSI mode - should skip",
			driverContext: config.ContextDocker,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// No setup needed - should return early
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyPolicyCount: 0,
		},
		{
			name:          "CSI mode - no policies in store",
			driverContext: config.ContextCSI,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().GetAutogrowPolicies(gomock.Any()).Return([]*storage.AutogrowPolicyPersistent{}, nil)
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyPolicyCount: 0,
		},
		{
			name:          "CSI mode - error getting policies from store",
			driverContext: config.ContextCSI,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().GetAutogrowPolicies(gomock.Any()).Return(nil, fmt.Errorf("store error"))
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "store error")
			},
			verifyPolicyCount: 0,
		},
		{
			name:          "CSI mode - successfully bootstrap single policy",
			driverContext: config.ContextCSI,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				policies := []*storage.AutogrowPolicyPersistent{
					{
						Name:          "policy1",
						UsedThreshold: "80",
						GrowthAmount:  "20",
						MaxSize:       "1000Gi",
						State:         storage.AutogrowPolicyStateSuccess,
					},
				}
				mockStore.EXPECT().GetAutogrowPolicies(gomock.Any()).Return(policies, nil)
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyPolicyCount: 1,
		},
		{
			name:          "CSI mode - successfully bootstrap multiple policies",
			driverContext: config.ContextCSI,
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				policies := []*storage.AutogrowPolicyPersistent{
					{
						Name:          "policy1",
						UsedThreshold: "80",
						GrowthAmount:  "20",
						MaxSize:       "1000Gi",
						State:         storage.AutogrowPolicyStateSuccess,
					},
					{
						Name:          "policy2",
						UsedThreshold: "90",
						GrowthAmount:  "30",
						MaxSize:       "2000Gi",
						State:         storage.AutogrowPolicyStateFailed,
					},
					{
						Name:          "policy3",
						UsedThreshold: "75",
						GrowthAmount:  "25",
						MaxSize:       "1500Gi",
						State:         storage.AutogrowPolicyStateDeleting,
					},
				}
				mockStore.EXPECT().GetAutogrowPolicies(gomock.Any()).Return(policies, nil)
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyPolicyCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Re-initialize the concurrent cache for each test
			db.Initialize()

			o := getConcurrentOrchestrator()

			// Set driver context
			oldContext := config.CurrentDriverContext
			config.CurrentDriverContext = tt.driverContext
			defer func() { config.CurrentDriverContext = oldContext }()

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			err := o.bootstrapAutogrowPolicies(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			// Verify policy count in cache
			if tt.verifyPolicyCount > 0 {
				results, unlocker, lockErr := db.Lock(testCtx, db.Query(db.ListAutogrowPolicies()))
				assert.NoError(t, lockErr)
				assert.Len(t, results[0].AutogrowPolicies, tt.verifyPolicyCount)
				unlocker()
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_ResolveEffectiveAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name               string
		volumeConfig       *storage.VolumeConfig
		setupCache         func()
		expectedPolicyName string
		expectedReason     models.AutogrowPolicyReason
		expectError        bool
		errorCheck         func(error) bool
	}{
		{
			name: "No StorageClass, no volume autogrow policy - not configured",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "",
				RequestedAutogrowPolicy: "",
			},
			setupCache:         func() {},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonNotConfigured,
			expectError:        false,
		},
		{
			name: "Volume autogrow policy is 'none' - disabled",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "none",
			},
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy1",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()
			},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonDisabled,
			expectError:        false,
		},
		{
			name: "StorageClass autogrow policy is 'NONE' (case insensitive) - disabled",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "",
			},
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "NONE",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()
			},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonDisabled,
			expectError:        false,
		},
		{
			name: "Volume autogrow policy specifies policy - policy exists in Success state",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "policy1",
			},
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()
			},
			expectedPolicyName: "policy1",
			expectedReason:     models.AutogrowPolicyReasonActive,
			expectError:        false,
		},
		{
			name: "Volume autogrow policy specifies policy - policy does not exist",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "nonexistent-policy",
			},
			setupCache:         func() {},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonNotFound,
			expectError:        true,
			errorCheck:         errors.IsAutogrowPolicyNotFoundError,
		},
		{
			name: "Volume autogrow policy specifies policy - policy in Failed state",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "policy-failed",
			},
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy-failed", "80", "20", "1000Gi", storage.AutogrowPolicyStateFailed)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-failed")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()
			},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonUnusable,
			expectError:        true,
			errorCheck:         errors.IsAutogrowPolicyNotUsableError,
		},
		{
			name: "Volume autogrow policy specifies policy - policy in Deleting state",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "policy-deleting",
			},
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy-deleting", "80", "20", "1000Gi", storage.AutogrowPolicyStateDeleting)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-deleting")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()
			},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonUnusable,
			expectError:        true,
			errorCheck:         errors.IsAutogrowPolicyNotUsableError,
		},
		{
			name: "No volume autogrow policy, StorageClass has autogrow policy - policy exists",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "",
			},
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy2",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policy := storage.NewAutogrowPolicy("policy2", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy2")))
				results2[0].AutogrowPolicy.Upsert(policy)
				unlocker2()
			},
			expectedPolicyName: "policy2",
			expectedReason:     models.AutogrowPolicyReasonActive,
			expectError:        false,
		},
		{
			name: "Volume autogrow policy overrides StorageClass autogrow policy",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "sc1",
				RequestedAutogrowPolicy: "policy-vol",
			},
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy-sc",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				volPolicy := storage.NewAutogrowPolicy("policy-vol", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-vol")))
				results2[0].AutogrowPolicy.Upsert(volPolicy)
				unlocker2()

				scPolicy := storage.NewAutogrowPolicy("policy-sc", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results3, unlocker3, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-sc")))
				results3[0].AutogrowPolicy.Upsert(scPolicy)
				unlocker3()
			},
			expectedPolicyName: "policy-vol",
			expectedReason:     models.AutogrowPolicyReasonActive,
			expectError:        false,
		},
		{
			name: "StorageClass not found - no policy",
			volumeConfig: &storage.VolumeConfig{
				Name:                    "vol1",
				StorageClass:            "nonexistent-sc",
				RequestedAutogrowPolicy: "",
			},
			setupCache:         func() {},
			expectedPolicyName: "",
			expectedReason:     models.AutogrowPolicyReasonNotConfigured,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()

			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			result, err := o.resolveEffectiveAutogrowPolicy(testCtx, tt.volumeConfig)

			assert.Equal(t, tt.expectedPolicyName, result.PolicyName, "PolicyName mismatch")
			assert.Equal(t, tt.expectedReason, result.Reason, "Reason mismatch")

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorCheck != nil {
					assert.True(t, tt.errorCheck(err), "Error type mismatch")
				}
			} else {
				assert.NoError(t, err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_InvalidateVolumesForPolicy(t *testing.T) {
	tests := []struct {
		name            string
		policyName      string
		setupCache      func()
		expectedInvalid []string
	}{
		{
			name:       "No volumes using the policy",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name: "vol1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotConfigured,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedInvalid: []string{},
		},
		{
			name:       "Single volume using the policy - should be invalidated",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name: "vol1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedInvalid: []string{"vol1"},
		},
		{
			name:       "Multiple volumes using the policy - all should be invalidated",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name: "vol1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				vol2 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name: "vol2",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				vol3 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name: "vol3",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy2",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1, vol2, vol3)
			},
			expectedInvalid: []string{"vol1", "vol2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()

			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			o.invalidateVolumesForPolicy(testCtx, tt.policyName)

			for _, volName := range tt.expectedInvalid {
				results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadVolume(volName)))
				assert.NoError(t, err)
				vol := results[0].Volume.Read
				assert.NotNil(t, vol)
				assert.Equal(t, "", vol.EffectiveAGPolicy.PolicyName, "Volume should be invalidated")
				assert.Equal(t, models.AutogrowPolicyReasonUnusable, vol.EffectiveAGPolicy.Reason, "Reason should be Unusable")
				unlocker()
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_ReevaluateVolumesForPolicy(t *testing.T) {
	tests := []struct {
		name              string
		policyName        string
		setupCache        func()
		expectedUpdated   []string
		expectedUnchanged []string
	}{
		{
			name:       "No volumes need reevaluation",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated:   []string{},
			expectedUnchanged: []string{"vol1"},
		},
		{
			name:       "Volume with matching autogrow policy should be updated to use policy",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonUnusable,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated:   []string{"vol1"},
			expectedUnchanged: []string{},
		},
		{
			name:       "Volume with no volume-level autogrow policy but inherits from SC should be updated",
			policyName: "policy1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy1",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results2[0].AutogrowPolicy.Upsert(policy)
				unlocker2()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonUnusable,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated:   []string{"vol1"},
			expectedUnchanged: []string{},
		},
		{
			name:       "Multiple volumes - some updated, some unchanged",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonUnusable,
					},
				}
				vol2 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol2",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				vol3 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol3",
						RequestedAutogrowPolicy: "none",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonDisabled,
					},
				}
				vol4 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol4",
						RequestedAutogrowPolicy: "policy2",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotFound,
					},
				}
				addVolumesToCache(t, vol1, vol2, vol3, vol4)
			},
			expectedUpdated:   []string{"vol1"},
			expectedUnchanged: []string{"vol2", "vol3", "vol4"},
		},
		{
			name:       "Failed policy - volumes get reason updated to Unusable",
			policyName: "policy-failed",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy-failed", "80", "20", "1000Gi", storage.AutogrowPolicyStateFailed)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-failed")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						RequestedAutogrowPolicy: "policy-failed",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotFound,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated:   []string{"vol1"},
			expectedUnchanged: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()

			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			o.reevaluateVolumesForPolicy(testCtx, tt.policyName)

			// Get policy to check its state
			policyResults, policyUnlocker, _ := db.Lock(testCtx, db.Query(db.ReadAutogrowPolicy(tt.policyName)))
			policy := policyResults[0].AutogrowPolicy.Read
			policyUnlocker()

			for _, volName := range tt.expectedUpdated {
				results, unlocker, err := db.Lock(testCtx, db.Query(db.ReadVolume(volName)))
				assert.NoError(t, err)
				vol := results[0].Volume.Read
				assert.NotNil(t, vol)

				// For Failed/Deleting policies, volumes should have empty PolicyName and Unusable reason
				// For Success policies, volumes should have the policy name and Active reason
				if policy != nil && !policy.State().IsSuccess() {
					assert.Equal(t, "", vol.EffectiveAGPolicy.PolicyName, "Volume should have empty policy name for Failed policy")
					assert.Equal(t, models.AutogrowPolicyReasonUnusable, vol.EffectiveAGPolicy.Reason, "Reason should be Unusable for Failed policy")
				} else {
					assert.Equal(t, tt.policyName, vol.EffectiveAGPolicy.PolicyName, "Volume should use the policy")
					assert.Equal(t, models.AutogrowPolicyReasonActive, vol.EffectiveAGPolicy.Reason, "Reason should be Active")
				}
				unlocker()
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_UpdateStorageClassAutogrowPolicyInternal(t *testing.T) {
	tests := []struct {
		name            string
		scName          string
		setupCache      func()
		expectedUpdated []string
		expectError     bool
	}{
		{
			name:   "No volumes using the StorageClass",
			scName: "sc1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy1",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results2[0].AutogrowPolicy.Upsert(policy)
				unlocker2()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)
			},
			expectedUpdated: []string{},
			expectError:     false,
		},
		{
			name:   "Volumes using SC without volume-level autogrow policy - should be updated",
			scName: "sc1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy-new",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policyNew := storage.NewAutogrowPolicy("policy-new", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-new")))
				results2[0].AutogrowPolicy.Upsert(policyNew)
				unlocker2()

				policyOld := storage.NewAutogrowPolicy("policy-old", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results3, unlocker3, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-old")))
				results3[0].AutogrowPolicy.Upsert(policyOld)
				unlocker3()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-old",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated: []string{"vol1"},
			expectError:     false,
		},
		{
			name:   "Volumes with volume-level autogrow policy should NOT be updated",
			scName: "sc1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy-sc",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policySC := storage.NewAutogrowPolicy("policy-sc", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-sc")))
				results2[0].AutogrowPolicy.Upsert(policySC)
				unlocker2()

				policyVol := storage.NewAutogrowPolicy("policy-vol", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results3, unlocker3, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-vol")))
				results3[0].AutogrowPolicy.Upsert(policyVol)
				unlocker3()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "policy-vol",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-vol",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated: []string{},
			expectError:     false,
		},
		{
			name:   "Multiple volumes - only those without volume-level autogrow policy are updated",
			scName: "sc1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy-new",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policyNew := storage.NewAutogrowPolicy("policy-new", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-new")))
				results2[0].AutogrowPolicy.Upsert(policyNew)
				unlocker2()

				policyOld := storage.NewAutogrowPolicy("policy-old", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results3, unlocker3, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-old")))
				results3[0].AutogrowPolicy.Upsert(policyOld)
				unlocker3()

				policyVol := storage.NewAutogrowPolicy("policy-vol", "75", "25", "1500Gi", storage.AutogrowPolicyStateSuccess)
				results4, unlocker4, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-vol")))
				results4[0].AutogrowPolicy.Upsert(policyVol)
				unlocker4()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-old",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				vol2 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol2",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "policy-vol",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-vol",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				vol3 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol3",
						StorageClass:            "sc2",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-old",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1, vol2, vol3)
			},
			expectedUpdated: []string{"vol1"},
			expectError:     false,
		},
		{
			name:   "Volume already using correct policy - no update needed",
			scName: "sc1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy1",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results2[0].AutogrowPolicy.Upsert(policy)
				unlocker2()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated: []string{},
			expectError:     false,
		},
		{
			name:   "Policy resolution error - should return error but skip volumes",
			scName: "sc1",
			setupCache: func() {
				// SC with non-existent autogrow policy
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "nonexistent-policy",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-old",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)
			},
			expectedUpdated: []string{},
			expectError:     true, // Should return AutogrowPolicyNotFoundError
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()

			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			err := o.upsertStorageClassAutogrowPolicyInternal(testCtx, tt.scName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			for _, volName := range tt.expectedUpdated {
				results, unlocker, lockErr := db.Lock(testCtx, db.Query(db.ReadVolume(volName)))
				assert.NoError(t, lockErr)
				vol := results[0].Volume.Read
				assert.NotNil(t, vol)
				assert.Equal(t, "policy-new", vol.EffectiveAGPolicy.PolicyName, "Volume should use new SC policy")
				assert.Equal(t, models.AutogrowPolicyReasonActive, vol.EffectiveAGPolicy.Reason)
				unlocker()
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_AddAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name          string
		policyName    string
		policyConfig  *storage.AutogrowPolicyConfig
		setupCache    func()
		expectError   bool
		verifyVolumes []string // volumes that should be associated after add
	}{
		{
			name:       "Add new policy in Success state - no existing volumes",
			policyName: "policy1",
			policyConfig: &storage.AutogrowPolicyConfig{
				Name:          "policy1",
				UsedThreshold: "80",
				GrowthAmount:  "20",
				MaxSize:       "1000Gi",
				State:         storage.AutogrowPolicyStateSuccess,
			},
			setupCache: func() {
				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)
			},
			expectError:   false,
			verifyVolumes: []string{},
		},
		{
			name:       "Add policy in Failed state - volumes re-evaluated but not associated",
			policyName: "policy-failed",
			policyConfig: &storage.AutogrowPolicyConfig{
				Name:          "policy-failed",
				UsedThreshold: "80",
				GrowthAmount:  "20",
				MaxSize:       "1000Gi",
				State:         storage.AutogrowPolicyStateFailed,
			},
			setupCache: func() {
				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				// Volume references the policy that will be added in Failed state
				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "policy-failed",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotFound,
					},
				}
				addVolumesToCache(t, vol)
			},
			expectError:   false,
			verifyVolumes: []string{}, // No volumes should be associated with Failed policy
		},
		{
			name:       "Add policy that already exists - should fail",
			policyName: "policy1",
			policyConfig: &storage.AutogrowPolicyConfig{
				Name:          "policy1",
				UsedThreshold: "80",
				GrowthAmount:  "20",
				MaxSize:       "1000Gi",
				State:         storage.AutogrowPolicyStateSuccess,
			},
			setupCache: func() {
				// Policy already exists
				existingPolicy := storage.NewAutogrowPolicy("policy1", "70", "30", "500Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(existingPolicy)
				unlocker()
			},
			expectError:   true,
			verifyVolumes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			result, err := o.AddAutogrowPolicy(testCtx, tt.policyConfig)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.policyName, result.Name)

				// Verify associated volumes
				if tt.verifyVolumes != nil {
					assert.ElementsMatch(t, tt.verifyVolumes, result.Volumes)
				}

				// For Failed policy test, verify volume reason was updated
				if tt.policyConfig.State == storage.AutogrowPolicyStateFailed {
					volResults, volUnlocker, volErr := db.Lock(testCtx, db.Query(db.ReadVolume("vol1")))
					assert.NoError(t, volErr)
					vol := volResults[0].Volume.Read
					assert.NotNil(t, vol)
					assert.Equal(t, "", vol.EffectiveAGPolicy.PolicyName)
					assert.Equal(t, models.AutogrowPolicyReasonUnusable, vol.EffectiveAGPolicy.Reason)
					volUnlocker()
				}
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_DeleteAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name        string
		policyName  string
		setupCache  func()
		expectError bool
		verifySoft  bool // true if should be soft-deleted (Deleting state)
	}{
		{
			name:       "Delete policy with no volumes - hard delete",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()
			},
			expectError: false,
			verifySoft:  false,
		},
		{
			name:       "Delete policy with volumes - soft delete (set to Deleting)",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol)
			},
			expectError: false,
			verifySoft:  true,
		},
		{
			name:        "Delete non-existent policy - should fail",
			policyName:  "nonexistent",
			setupCache:  func() {},
			expectError: true,
			verifySoft:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			err := o.DeleteAutogrowPolicy(testCtx, tt.policyName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify policy state
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.InconsistentReadAutogrowPolicy(tt.policyName)))
				policy := results[0].AutogrowPolicy.Read
				unlocker()

				if tt.verifySoft {
					// Soft delete - policy should still exist but in Deleting state
					assert.NotNil(t, policy)
					assert.Equal(t, storage.AutogrowPolicyStateDeleting, policy.State())
				} else {
					// Hard delete - policy should be gone
					assert.Nil(t, policy)
				}
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_UpdateVolumeAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name                    string
		volumeName              string
		requestedAutogrowPolicy string
		setupCache              func(*mockpersistentstore.MockStoreClient)
		expectError             bool
		expectedEffectivePolicy string
	}{
		{
			name:                    "Update volume to use specific policy",
			volumeName:              "vol1",
			requestedAutogrowPolicy: "policy-new",
			setupCache: func(mockStore *mockpersistentstore.MockStoreClient) {
				policy := storage.NewAutogrowPolicy("policy-new", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-new")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotConfigured,
					},
				}
				addVolumesToCache(t, vol)

				mockStore.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			expectError:             false,
			expectedEffectivePolicy: "policy-new",
		},
		{
			name:                    "Update volume to use non-existent policy - should return error",
			volumeName:              "vol1",
			requestedAutogrowPolicy: "nonexistent",
			setupCache: func(mockStore *mockpersistentstore.MockStoreClient) {
				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "vol1",
						StorageClass:            "sc1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotConfigured,
					},
				}
				addVolumesToCache(t, vol)

				mockStore.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			expectError:             true,
			expectedEffectivePolicy: "", // Policy not found, should be empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()
			o := getConcurrentOrchestrator()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o.storeClient = mockStoreClient

			if tt.setupCache != nil {
				tt.setupCache(mockStoreClient)
			}

			err := o.UpdateVolumeAutogrowPolicy(testCtx, tt.volumeName, tt.requestedAutogrowPolicy)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify volume was updated
			results, unlocker, _ := db.Lock(testCtx, db.Query(db.ReadVolume(tt.volumeName)))
			vol := results[0].Volume.Read
			unlocker()

			assert.NotNil(t, vol)
			assert.Equal(t, tt.requestedAutogrowPolicy, vol.Config.RequestedAutogrowPolicy)
			assert.Equal(t, tt.expectedEffectivePolicy, vol.EffectiveAGPolicy.PolicyName)

			persistenceCleanup(t, o)
		})
	}
}

// ============================================================================
// VP Propagation Tests
// ============================================================================

func TestConcurrent_syncVolumePublication(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(*gomock.Controller, *ConcurrentTridentOrchestrator)
		verifyError func(error)
	}{
		{
			name: "Success",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				vp := &models.VolumePublication{
					Name:       "pvc-123-node1",
					VolumeName: "pvc-123",
					NodeName:   "node1",
				}
				addVolumePublicationsToCache(t, vp)

				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(nil)
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "VPNotFound",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				// Don't add VP to cache - simulating VP deletion
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Times(0)
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				// Should return nil (successful skip) since VP was deleted - this is not an error condition
				assert.NoError(t, err)
			},
		},
		{
			name: "StoreUpdateFails",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) {
				vp := &models.VolumePublication{
					Name:       "pvc-123-node1",
					VolumeName: "pvc-123",
					NodeName:   "node1",
				}
				addVolumePublicationsToCache(t, vp)

				storeErr := errors.New("store update failed")
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(storeErr)
				o.storeClient = mockStore
			},
			verifyError: func(err error) {
				assert.Error(t, err)
				assert.ErrorContains(t, err, "store update failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()
			o := getConcurrentOrchestrator()

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, o)
			}

			vp := &models.VolumePublication{
				Name:       "pvc-123-node1",
				VolumeName: "pvc-123",
				NodeName:   "node1",
			}

			err := o.syncVolumePublication(testCtx, vp)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_SyncVolumePublications(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(*gomock.Controller, *ConcurrentTridentOrchestrator) []*models.VolumePublication
		timeout    time.Duration
	}{
		{
			name: "EmptyList",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) []*models.VolumePublication {
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				o.storeClient = mockStore
				return []*models.VolumePublication{}
			},
			timeout: 1 * time.Second,
		},
		{
			name: "SingleVP",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) []*models.VolumePublication {
				vp := &models.VolumePublication{
					Name:       "pvc-123-node1",
					VolumeName: "pvc-123",
					NodeName:   "node1",
				}
				addVolumePublicationsToCache(t, vp)

				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(nil)
				o.storeClient = mockStore

				return []*models.VolumePublication{vp}
			},
			timeout: 5 * time.Second,
		},
		{
			name: "MultipleVPs",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) []*models.VolumePublication {
				vp1 := &models.VolumePublication{
					Name:       "pvc-123-node1",
					VolumeName: "pvc-123",
					NodeName:   "node1",
				}
				vp2 := &models.VolumePublication{
					Name:       "pvc-456-node2",
					VolumeName: "pvc-456",
					NodeName:   "node2",
				}
				vp3 := &models.VolumePublication{
					Name:       "pvc-789-node3",
					VolumeName: "pvc-789",
					NodeName:   "node3",
				}
				addVolumePublicationsToCache(t, vp1, vp2, vp3)

				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(nil).Times(3)
				o.storeClient = mockStore

				return []*models.VolumePublication{vp1, vp2, vp3}
			},
			timeout: 10 * time.Second,
		},
		{
			name: "PartialFailure",
			setupMocks: func(mockCtrl *gomock.Controller, o *ConcurrentTridentOrchestrator) []*models.VolumePublication {
				vp1 := &models.VolumePublication{
					Name:       "pvc-123-node1",
					VolumeName: "pvc-123",
					NodeName:   "node1",
				}
				vp2 := &models.VolumePublication{
					Name:       "pvc-456-node2",
					VolumeName: "pvc-456",
					NodeName:   "node2",
				}
				addVolumePublicationsToCache(t, vp1, vp2)

				storeErr := errors.New("store update failed")
				mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)
				// First VP succeeds
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				// Second VP fails continuously
				mockStore.EXPECT().UpdateVolumePublication(gomock.Any(), gomock.Any()).Return(storeErr).AnyTimes()
				o.storeClient = mockStore

				return []*models.VolumePublication{vp1, vp2}
			},
			timeout: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()
			o := getConcurrentOrchestrator()

			var vpList []*models.VolumePublication
			if tt.setupMocks != nil {
				vpList = tt.setupMocks(mockCtrl, o)
			}

			// Run the function with timeout for tests that may retry indefinitely
			done := make(chan bool, 1)
			go func() {
				o.SyncVolumePublications(testCtx, vpList)
				done <- true
			}()

			select {
			case <-done:
				// Completed successfully
			case <-time.After(tt.timeout):
				// For PartialFailure test, timeout is expected as it retries indefinitely
				if tt.name == "PartialFailure" {
					t.Log("SyncVolumePublications still retrying failed VP as expected")
				} else {
					t.Fatalf("Test timed out after %v", tt.timeout)
				}
			}

			persistenceCleanup(t, o)
		})
	}
}

// ============================================================================
// Tests for subordinate volume support in autogrow policy operations
// ============================================================================

func TestBootstrapVolumes_SubordinateAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "SubordinateVolume_AutogrowPolicyResolved_CSI",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				config.CurrentDriverContext = config.ContextCSI

				// Add autogrow policy to cache
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				// Add backend
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid")
				mockBackend.EXPECT().Driver().Return(&fakedriver.StorageDriver{}).AnyTimes()
				mockBackend.EXPECT().HealVolumePublishEnforcement(gomock.Any(), gomock.Any()).Return(false).AnyTimes()
				addBackendsToCache(t, mockBackend)

				// Return subordinate volume from store
				subVolConfig := &storage.VolumeConfig{
					Version:                 "1",
					Name:                    "subvol1",
					InternalName:            "subvol1",
					ShareSourceVolume:       "vol1",
					RequestedAutogrowPolicy: "policy1",
				}
				subVolume := storage.Volume{
					Config:      subVolConfig,
					BackendUUID: "backend-uuid",
					State:       storage.VolumeStateSubordinate,
				}
				volumes := []*storage.VolumeExternal{subVolume.ConstructExternal()}
				mockStoreClient.EXPECT().GetVolumes(gomock.Any()).Return(volumes, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getSubordinateVolumeByNameFromCache(t, "subvol1")
				require.NotNil(t, result)
				assert.Equal(t, storage.VolumeStateSubordinate, result.State)
				// Verify autogrow policy was resolved
				assert.Equal(t, "policy1", result.EffectiveAGPolicy.PolicyName)
				assert.Equal(t, models.AutogrowPolicyReasonActive, result.EffectiveAGPolicy.Reason)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapVolumes(testCtx)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestAddSubordinateVolume_AutogrowPolicyResolution(t *testing.T) {
	volumeConfig := &storage.VolumeConfig{
		Name:              "vol1",
		InternalName:      "vol1",
		Size:              "1G",
		Protocol:          config.File,
		StorageClass:      "gold",
		ShareSourceVolume: "",
		VolumeMode:        config.Filesystem,
		AccessMode:        config.ReadWriteMany,
		AccessInfo: models.VolumeAccessInfo{
			NfsAccessInfo: models.NfsAccessInfo{
				NfsPath: "10.10.10.10/path",
			},
		},
	}

	parentVolume := &storage.Volume{
		Config:      volumeConfig,
		BackendUUID: "backend-uuid1",
		State:       storage.VolumeStateOnline,
	}

	tests := []struct {
		name         string
		subVolConfig *storage.VolumeConfig
		setupMocks   func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyResult func(result *storage.VolumeExternal)
		verifyError  func(err error)
	}{
		{
			name: "SubordinateVolume_WithAutogrowPolicy",
			subVolConfig: &storage.VolumeConfig{
				Name:                    "subvol1",
				InternalName:            "subvol1",
				Size:                    "1G",
				ShareSourceVolume:       "vol1",
				StorageClass:            "gold",
				RequestedAutogrowPolicy: "policy1",
			},
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().GetUniqueKey().Return("testBackend").AnyTimes()

				// Add autogrow policy
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				addBackendsToCache(t, mockBackend)
				addVolumesToCache(t, parentVolume)

				mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(err error) {
				assert.NoError(t, err)
			},
			verifyResult: func(result *storage.VolumeExternal) {
				require.NotNil(t, result)
				assert.Equal(t, "subvol1", result.Config.Name)

				// Verify autogrow policy was resolved for the subordinate volume
				subVol := getSubordinateVolumeByNameFromCache(t, "subvol1")
				require.NotNil(t, subVol)
				assert.Equal(t, "policy1", subVol.EffectiveAGPolicy.PolicyName)
				assert.Equal(t, models.AutogrowPolicyReasonActive, subVol.EffectiveAGPolicy.Reason)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			result, err := o.AddVolume(testCtx, tt.subVolConfig)

			if tt.verifyError != nil {
				tt.verifyError(err)
			}
			if tt.verifyResult != nil {
				tt.verifyResult(result)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestDeleteStorageClass_SubordinateVolumes_AutogrowPolicy(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(t *testing.T, err error, o *ConcurrentTridentOrchestrator)
	}{
		{
			name: "DeleteStorageClass_SubordinateVolume_AutogrowPolicyUpdated",
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				// Create SC with autogrow policy
				scConfigWithPolicy := &storageclass.Config{
					Version:        "1",
					Name:           "sc1",
					AutogrowPolicy: "policy1",
				}
				scWithPolicy := storageclass.New(scConfigWithPolicy)
				addStorageClassesToCache(t, scWithPolicy)

				// Add autogrow policy
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				// Add backend
				mockBackend := getMockBackend(mockCtrl, "testBackend", "backend-uuid1")
				mockBackend.EXPECT().StoragePools().Return(&sync.Map{}).AnyTimes()
				addBackendsToCache(t, mockBackend)

				// Add parent volume
				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:         "vol1",
						StorageClass: "sc1",
					},
					BackendUUID: "backend-uuid1",
				}
				addVolumesToCache(t, vol)

				// Add subordinate volume using this SC with inherited autogrow policy
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "subvol1",
						StorageClass:            "sc1",
						ShareSourceVolume:       "vol1",
						RequestedAutogrowPolicy: "", // Inherits from SC
					},
					BackendUUID: "backend-uuid1",
					State:       storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addSubordinateVolumesToCache(t, subVol)

				o.RebuildStorageClassPoolMap(testCtx)

				mockStoreClient.EXPECT().DeleteStorageClass(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			verifyError: func(t *testing.T, err error, o *ConcurrentTridentOrchestrator) {
				assert.NoError(t, err)

				// Subordinate volume should have been updated to empty policy
				subVol := getSubordinateVolumeByNameFromCache(t, "subvol1")
				assert.NotNil(t, subVol)
				assert.Equal(t, "", subVol.EffectiveAGPolicy.PolicyName)
				assert.Equal(t, models.AutogrowPolicyReasonNotConfigured, subVol.EffectiveAGPolicy.Reason)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.DeleteStorageClass(testCtx, "sc1")

			if tt.verifyError != nil {
				tt.verifyError(t, err, o)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_UpdateVolumeAutogrowPolicy_SubordinateVolume(t *testing.T) {
	tests := []struct {
		name                    string
		volumeName              string
		requestedAutogrowPolicy string
		setupCache              func(*mockpersistentstore.MockStoreClient)
		expectError             bool
		expectedEffectivePolicy string
	}{
		{
			name:                    "Update subordinate volume to use specific policy",
			volumeName:              "subvol1",
			requestedAutogrowPolicy: "policy-new",
			setupCache: func(mockStore *mockpersistentstore.MockStoreClient) {
				policy := storage.NewAutogrowPolicy("policy-new", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-new")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				// Add parent volume
				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:         "vol1",
						StorageClass: "sc1",
					},
					BackendUUID: "backend1",
				}
				addVolumesToCache(t, vol)

				// Add subordinate volume
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "subvol1",
						StorageClass:            "sc1",
						ShareSourceVolume:       "vol1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					State:       storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonNotConfigured,
					},
				}
				addSubordinateVolumesToCache(t, subVol)

				mockStore.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
			expectError:             false,
			expectedEffectivePolicy: "policy-new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			db.Initialize()
			o := getConcurrentOrchestrator()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o.storeClient = mockStoreClient

			if tt.setupCache != nil {
				tt.setupCache(mockStoreClient)
			}

			err := o.UpdateVolumeAutogrowPolicy(testCtx, tt.volumeName, tt.requestedAutogrowPolicy)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify subordinate volume was updated
			results, unlocker, _ := db.Lock(testCtx, db.Query(db.ReadSubordinateVolume(tt.volumeName)))
			subVol := results[0].SubordinateVolume.Read
			unlocker()

			assert.NotNil(t, subVol)
			assert.Equal(t, tt.requestedAutogrowPolicy, subVol.Config.RequestedAutogrowPolicy)
			assert.Equal(t, tt.expectedEffectivePolicy, subVol.EffectiveAGPolicy.PolicyName)

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_InvalidateVolumesForPolicy_SubordinateVolumes(t *testing.T) {
	tests := []struct {
		name                   string
		policyName             string
		setupCache             func()
		expectedInvalidVolumes []string
		expectedInvalidSubVols []string
	}{
		{
			name:       "Subordinate volume using policy is invalidated",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				// Regular volume using policy1
				vol1 := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name: "vol1",
					},
					BackendUUID: "backend1",
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addVolumesToCache(t, vol1)

				// Subordinate volume using policy1
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:              "subvol1",
						ShareSourceVolume: "vol1",
					},
					BackendUUID: "backend1",
					State:       storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy1",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addSubordinateVolumesToCache(t, subVol)
			},
			expectedInvalidVolumes: []string{"vol1"},
			expectedInvalidSubVols: []string{"subvol1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			o.invalidateVolumesForPolicy(testCtx, tt.policyName)

			// Verify regular volumes
			for _, volName := range tt.expectedInvalidVolumes {
				vol := getVolumeByNameFromCache(t, volName)
				require.NotNil(t, vol)
				assert.Equal(t, "", vol.EffectiveAGPolicy.PolicyName, "Volume %s should be invalidated", volName)
				assert.Equal(t, models.AutogrowPolicyReasonUnusable, vol.EffectiveAGPolicy.Reason)
			}

			// Verify subordinate volumes
			for _, subVolName := range tt.expectedInvalidSubVols {
				subVol := getSubordinateVolumeByNameFromCache(t, subVolName)
				require.NotNil(t, subVol)
				assert.Equal(t, "", subVol.EffectiveAGPolicy.PolicyName, "SubVolume %s should be invalidated", subVolName)
				assert.Equal(t, models.AutogrowPolicyReasonUnusable, subVol.EffectiveAGPolicy.Reason)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_ReevaluateVolumesForPolicy_SubordinateVolumes(t *testing.T) {
	tests := []struct {
		name       string
		policyName string
		setupCache func()
		verify     func(t *testing.T)
	}{
		{
			name:       "Subordinate volume re-evaluated when policy transitions to Success",
			policyName: "policy1",
			setupCache: func() {
				policy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy1")))
				results[0].AutogrowPolicy.Upsert(policy)
				unlocker()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				vol1 := &storage.Volume{
					Config:      &storage.VolumeConfig{Name: "vol1"},
					BackendUUID: "backend1",
				}
				addVolumesToCache(t, vol1)

				// Subordinate volume that was previously unusable
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "subvol1",
						ShareSourceVolume:       "vol1",
						RequestedAutogrowPolicy: "policy1",
					},
					BackendUUID: "backend1",
					State:       storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "",
						Reason:     models.AutogrowPolicyReasonUnusable,
					},
				}
				addSubordinateVolumesToCache(t, subVol)
			},
			verify: func(t *testing.T) {
				subVol := getSubordinateVolumeByNameFromCache(t, "subvol1")
				require.NotNil(t, subVol)
				assert.Equal(t, "policy1", subVol.EffectiveAGPolicy.PolicyName)
				assert.Equal(t, models.AutogrowPolicyReasonActive, subVol.EffectiveAGPolicy.Reason)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			o.reevaluateVolumesForPolicy(testCtx, tt.policyName)

			if tt.verify != nil {
				tt.verify(t)
			}

			persistenceCleanup(t, o)
		})
	}
}

func TestConcurrent_UpsertStorageClassAutogrowPolicyInternal_SubordinateVolumes(t *testing.T) {
	tests := []struct {
		name            string
		scName          string
		setupCache      func()
		expectedUpdated []string // subordinate volume names that should be updated
		expectError     bool
	}{
		{
			name:   "Subordinate volumes using SC without volume-level AGP are updated",
			scName: "sc1",
			setupCache: func() {
				scConfig := &storageclass.Config{
					Name:           "sc1",
					AutogrowPolicy: "policy-new",
				}
				sc := storageclass.New(scConfig)
				results, unlocker, _ := db.Lock(testCtx, db.Query(db.UpsertStorageClass("sc1")))
				results[0].StorageClass.Upsert(sc)
				unlocker()

				policyNew := storage.NewAutogrowPolicy("policy-new", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess)
				results2, unlocker2, _ := db.Lock(testCtx, db.Query(db.UpsertAutogrowPolicy("policy-new")))
				results2[0].AutogrowPolicy.Upsert(policyNew)
				unlocker2()

				backend := getFakeBackend("backend1", "backend1", nil)
				addBackendsToCache(t, backend)

				// Parent volume
				vol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:         "vol1",
						StorageClass: "sc1",
					},
					BackendUUID: "backend1",
				}
				addVolumesToCache(t, vol)

				// Subordinate volume inheriting from SC (no volume-level AGP)
				subVol := &storage.Volume{
					Config: &storage.VolumeConfig{
						Name:                    "subvol1",
						StorageClass:            "sc1",
						ShareSourceVolume:       "vol1",
						RequestedAutogrowPolicy: "",
					},
					BackendUUID: "backend1",
					State:       storage.VolumeStateSubordinate,
					EffectiveAGPolicy: models.EffectiveAutogrowPolicyInfo{
						PolicyName: "policy-old",
						Reason:     models.AutogrowPolicyReasonActive,
					},
				}
				addSubordinateVolumesToCache(t, subVol)
			},
			expectedUpdated: []string{"subvol1"},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Initialize()
			o := getConcurrentOrchestrator()

			if tt.setupCache != nil {
				tt.setupCache()
			}

			err := o.upsertStorageClassAutogrowPolicyInternal(testCtx, tt.scName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			for _, subVolName := range tt.expectedUpdated {
				subVol := getSubordinateVolumeByNameFromCache(t, subVolName)
				require.NotNil(t, subVol)
				assert.Equal(t, "policy-new", subVol.EffectiveAGPolicy.PolicyName,
					"Subordinate volume %s should use new SC policy", subVolName)
				assert.Equal(t, models.AutogrowPolicyReasonActive, subVol.EffectiveAGPolicy.Reason)
			}

			persistenceCleanup(t, o)
		})
	}
}
