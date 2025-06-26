// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

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
		results, unlocker, err := db.Lock(db.Query(db.UpsertBackend(backend.BackendUUID(), "", backend.Name())))
		require.NoError(t, err)
		results[0].Backend.Upsert(backend)
		unlocker()
	}
}

func removeBackendFromCache(t *testing.T, backendUUID string) {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.DeleteBackend(backendUUID)))
	defer unlocker()
	require.NoError(t, err)
	results[0].Backend.Delete()
}

func addStorageClassesToCache(t *testing.T, storageClasses ...*storageclass.StorageClass) {
	t.Helper()
	for _, sc := range storageClasses {
		results, unlocker, err := db.Lock(db.Query(db.UpsertStorageClass(sc.GetName())))
		require.NoError(t, err)
		results[0].StorageClass.Upsert(sc)
		unlocker()
	}
}

func addSnapshotsToCache(t *testing.T, snapshots ...*storage.Snapshot) {
	t.Helper()
	for _, snapshot := range snapshots {
		results, unlocker, err := db.Lock(db.Query(db.UpsertSnapshot(snapshot.Config.VolumeName, snapshot.Config.ID())))
		require.NoError(t, err)
		results[0].Snapshot.Upsert(snapshot)
		unlocker()
	}
}

func addVolumePublicationsToCache(t *testing.T, publications ...*models.VolumePublication) {
	t.Helper()
	for _, publication := range publications {
		results, unlocker, err := db.Lock(db.Query(db.UpsertVolumePublication(publication.VolumeName, publication.NodeName)))
		require.NoError(t, err)
		results[0].VolumePublication.Upsert(publication)
		unlocker()
	}
}

func getBackendByUuidFromCache(t *testing.T, backendUuid string) storage.Backend {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadBackend(backendUuid)))
	defer unlocker()
	require.NoError(t, err)
	backend := results[0].Backend.Read
	return backend
}

func getSnapshotByIDFromCache(t *testing.T, snapshotId string) *storage.Snapshot {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadSnapshot(snapshotId)))
	defer unlocker()
	require.NoError(t, err)
	snapshot := results[0].Snapshot.Read
	return snapshot
}

func getBackendByNameFromCache(t *testing.T, backendName string) storage.Backend {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadBackendByName(backendName)))
	defer unlocker()
	require.NoError(t, err)
	backend := results[0].Backend.Read
	return backend
}

func getStorageClassByNameFromCache(t *testing.T, scName string) *storageclass.StorageClass {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadStorageClass(scName)))
	defer unlocker()
	require.NoError(t, err)
	sc := results[0].StorageClass.Read
	return sc
}

func getVolumeByNameFromCache(t *testing.T, volumeName string) *storage.Volume {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadVolume(volumeName)))
	defer unlocker()
	require.NoError(t, err)
	volume := results[0].Volume.Read
	return volume
}

func getSubVolumeByNameFromCache(t *testing.T, volumeName string) *storage.Volume {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadSubordinateVolume(volumeName)))
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
		results, unlocker, err := db.Lock(db.Query(db.UpsertVolume(vol.Config.Name, vol.BackendUUID)))
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
	results, unlocker, err := db.Lock(db.Query(db.ReadNode(nodeName)))
	defer unlocker()
	require.NoError(t, err)
	node := results[0].Node.Read
	return node
}

func addNodesToCache(t *testing.T, nodes ...*models.Node) {
	t.Helper()
	for _, node := range nodes {
		results, unlocker, err := db.Lock(db.Query(db.UpsertNode(node.Name)))
		require.NoError(t, err)
		results[0].Node.Upsert(node)
		unlocker()
	}
}

func getVolumePublicationByIDFromCache(t *testing.T, volumeID, nodeID string) *models.VolumePublication {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadVolumePublication(volumeID, nodeID)))
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
		results, unlocker, err := db.Lock(db.Query(db.UpsertSubordinateVolume(subVol.Config.Name, subVol.Config.ShareSourceVolume)))
		require.NoError(t, err)
		results[0].SubordinateVolume.Upsert(subVol)
		unlocker()
	}
}

func getSubordinateVolumeByNameFromCache(t *testing.T, volumeId string) *storage.Volume {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.ReadSubordinateVolume(volumeId)))
	defer unlocker()
	require.NoError(t, err)
	volume := results[0].SubordinateVolume.Read
	return volume
}

func removeVolumeFromCache(t *testing.T, volumeName string) {
	t.Helper()
	results, unlocker, err := db.Lock(db.Query(db.DeleteVolume(volumeName)))
	defer unlocker()
	require.NoError(t, err)
	results[0].Volume.Delete()
}

func getFakeStorageDriverConfig(name string) drivers.FakeStorageDriverConfig {
	fakeConfig := drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           drivers.ConfigVersion,
			StorageDriverName: config.FakeStorageDriverName,
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
				o.bootstrapped = false
				o.bootstrapError = failed

				mockStoreClient.EXPECT().GetVersion(gomock.Any()).Return(version, nil).Times(1)
				mockStoreClient.EXPECT().GetType().Return(persistentstore.CRDV1Store).Times(1)
				mockStoreClient.EXPECT().SetVersion(gomock.Any(), version).Return(nil).Times(1)
				mockStoreClient.EXPECT().GetTridentUUID(gomock.Any()).Return("trident-uuid", nil).AnyTimes()
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
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
			defer mockCtrl.Finish()

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
		setupMocks  func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator)
		verifyError func(err error)
	}{
		{
			name: "StoreError",
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
			setupMocks: func(mockCtrl *gomock.Controller, mockStoreClient *mockpersistentstore.MockStoreClient, o *ConcurrentTridentOrchestrator) {
				mockStoreClient.EXPECT().GetBackends(gomock.Any()).Return(backends, nil).AnyTimes()
			},
			verifyError: func(err error) {
				assert.NoError(t, err)

				result := getBackendByUuidFromCache(t, "backend-uuid1")
				assert.NotNil(t, result)
			},
		},
		{
			name: "MarshalError",
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
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			o := getConcurrentOrchestrator()
			o.storeClient = mockStoreClient

			if tt.setupMocks != nil {
				tt.setupMocks(mockCtrl, mockStoreClient, o)
			}

			err := o.bootstrapBackends(testCtx)

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
				results, unlocker, err := db.Lock(db.Query(db.UpsertBackend(existingBackendUuid, "", "")))
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
					"name":  "testBackend",
					"uuid":  "backend-uuid1",
					"state": string(storage.Deleting),
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
					"name":  "testBackend",
					"uuid":  "backend-uuid1",
					"state": string(storage.Deleting),
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
					"name":  "testBackend",
					"uuid":  "backend-uuid1",
					"state": string(storage.Deleting),
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
			bootstrapErr: fmt.Errorf("bootstrap error"),
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
			bootstrapErr: fmt.Errorf("bootstrap error"),
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
			bootstrapErr: fmt.Errorf("bootstrap error"),
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
			bootstrapErr: fmt.Errorf("bootstrap error"),
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
			bootstrapErr: fmt.Errorf("bootstrap error"),
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
			defer mockCtrl.Finish()

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
				driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().CreateFollowup(gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				driver.EXPECT().ReconcileNodeAccess(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

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
				mockBackend2.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").Return(errors.New("vol not found")).AnyTimes()
				mockBackend3.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").Return(errors.New("vol not found")).AnyTimes()
				mockBackend4.EXPECT().RenameVolume(gomock.Any(), gomock.Any(), "originalVolume").Return(errors.New("vol not found")).AnyTimes()

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
