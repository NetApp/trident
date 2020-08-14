// Copyright 2020 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/utils"
)

type mockBackend struct {
	name        string
	backendUUID string
	volumes     map[string]*storage.Volume
	protocol    config.Protocol
	// Store non-volume specific access info here
	accessInfo utils.VolumeAccessInfo
}

func GetFakeInternalName(name string) string {
	return strings.ToUpper(name)
}

func newMockBackend(protocol config.Protocol) *mockBackend {
	return &mockBackend{
		volumes:  make(map[string]*storage.Volume),
		protocol: protocol,
	}
}

// MockOrchestrator is a struct that implements the Orchestrator interface for
// use in testing frontends.  Although it retains the appearance of correct
// functionality for this purpose, all functions are effectively nops.
// Note:  Many of the getter methods are copied verbatim from
// TridentOrchestrator, since their functionality is not inherently interesting
// or testable.
type MockOrchestrator struct {
	//backends           map[string]*storage.Backend
	backendsByUUID map[string]*storage.Backend
	//mockBackends       map[string]*mockBackend
	mockBackendsByUUID map[string]*mockBackend
	storageClasses     map[string]*storageclass.StorageClass
	volumes            map[string]*storage.Volume
	nodes              map[string]*utils.Node
	mutex              *sync.Mutex
}

func (m *MockOrchestrator) Bootstrap() error {
	return nil
}

func (m *MockOrchestrator) AddFrontend(f frontend.Plugin) {
	// NOP for the time being, since users of MockOrchestrator don't need this
}

func (m *MockOrchestrator) GetFrontend(ctx context.Context, name string) (frontend.Plugin, error) {
	return nil, nil
}

func (m *MockOrchestrator) GetVersion(context.Context) (string, error) {
	return config.OrchestratorVersion.String(), nil
}

// TODO:  Add extra methods to add backends without needing to provide a valid,
// stringified JSON config.
func (m *MockOrchestrator) AddBackend(ctx context.Context, configJSON string) (*storage.BackendExternal, error) {
	// We need to do this to determine if the backend is NFS or not.
	backend := &storage.Backend{
		Name:        fmt.Sprintf("mock-%d", len(m.backendsByUUID)),
		BackendUUID: uuid.New().String(),
		Driver:      nil,
		Online:      true,
		State:       storage.Online,
		Storage:     make(map[string]*storage.Pool),
	}
	mock := newMockBackend(backend.GetProtocol())
	m.mutex.Lock()
	defer m.mutex.Unlock()
	//m.backends[backend.Name] = backend
	//m.mockBackends[backend.Name] = mock
	m.backendsByUUID[backend.BackendUUID] = backend
	m.mockBackendsByUUID[backend.BackendUUID] = mock
	return backend.ConstructExternal(), nil
}

// Convenience method for test harnesses to avoid having to create a
// backend config JSON.
func (m *MockOrchestrator) addMockBackend(
	name string, protocol config.Protocol,
) *storage.Backend {
	mock := newMockBackend(protocol)
	backend := &storage.Backend{
		Name:        name,
		BackendUUID: uuid.New().String(),
		Driver:      nil,
		Online:      true,
		State:       storage.Online,
		Storage:     make(map[string]*storage.Pool),
	}
	mock.name = name
	mock.backendUUID = backend.BackendUUID
	//m.backends[backend.Name] = backend
	//m.mockBackends[backend.Name] = mock
	m.backendsByUUID[backend.BackendUUID] = backend
	m.mockBackendsByUUID[backend.BackendUUID] = mock
	return backend
}

func (m *MockOrchestrator) AddMockONTAPNFSBackend(name, lif string) *storage.BackendExternal {
	backend := m.addMockBackend(name, config.File)
	backend.Driver = &ontap.NASStorageDriver{
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "ontap-nas",
			},
			DataLIF: lif,
		},
	}
	mock := m.mockBackendsByUUID[backend.BackendUUID]
	mock.accessInfo.NfsServerIP = lif
	return backend.ConstructExternal()
}

func (m *MockOrchestrator) AddMockONTAPSANBackend(name, lif string) *storage.BackendExternal {
	backend := m.addMockBackend(name, config.Block)
	backend.Driver = &ontap.SANStorageDriver{
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "ontap-san",
			},
			DataLIF: lif,
		},
	}
	// add any iscsi specific bits you need here...
	//mock := m.mockBackendsByUUID[backend.BackendUUID]
	//mock.accessInfo.IscsiUsername = "user"
	return backend.ConstructExternal()
}

func (m *MockOrchestrator) AddFakeBackend(backend *storage.Backend) *storage.BackendExternal {
	mock := &mockBackend{
		name:        backend.Name,
		backendUUID: backend.BackendUUID,
		volumes:     make(map[string]*storage.Volume),
		protocol:    backend.GetProtocol(),
	}
	m.backendsByUUID[backend.BackendUUID] = backend
	m.mockBackendsByUUID[backend.BackendUUID] = mock
	return backend.ConstructExternal()
}

func (m *MockOrchestrator) AddMockFakeSANBackend(name string) *storage.BackendExternal {
	backend := m.addMockBackend(name, config.Block)
	backend.Driver = &fake.StorageDriver{
		Config: drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "fake",
			},
			Protocol: config.Block,
		},
	}
	//mock := m.mockBackendsByUUID[backend.BackendUUID]
	return backend.ConstructExternal()
}

func (m *MockOrchestrator) AddMockFakeNASBackend(name string) *storage.BackendExternal {
	backend := m.addMockBackend(name, config.Block)
	backend.Driver = &fake.StorageDriver{
		Config: drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "fake",
			},
			Protocol: config.File,
		},
	}
	//mock := m.mockBackendsByUUID[backend.BackendUUID]
	return backend.ConstructExternal()
}

//TODO:  Add other mock backends here as necessary.

// UpdateBackend updates an existing backend
func (m *MockOrchestrator) UpdateBackend(ctx context.Context, backendName, configJSON string) (storageBackendExternal *storage.BackendExternal, err error) {
	backend, err := m.GetBackend(ctx, backendName)
	if err != nil {
		return nil, err
	}
	return m.UpdateBackendByBackendUUID(ctx, backendName, configJSON, backend.BackendUUID)
}

// UpdateBackendByBackendUUID updates an existing backend
func (m *MockOrchestrator) UpdateBackendByBackendUUID(ctx context.Context, backendName, configJSON, backendUUID string) (storageBackendExternal *storage.BackendExternal, err error) {

	originalBackend, found := m.backendsByUUID[backendUUID]
	if !found {
		m.dumpKnownBackends()
		return nil, utils.NotFoundError(fmt.Sprintf("backend name:%v uuid:%v was not found", backendName, backendUUID))
	}

	newBackend, err := factory.NewStorageBackendForConfig(configJSON)
	if err != nil {
		return nil, err
	}

	originalBackend.Terminate()
	m.backendsByUUID[backendName] = newBackend
	return newBackend.ConstructExternal(), nil
}

// UpdateBackendState updates an existing backend
func (m *MockOrchestrator) UpdateBackendState(ctx context.Context, backendName, backendState string) (storageBackendExternal *storage.BackendExternal, err error) {
	//TODO
	return nil, fmt.Errorf("operation not currently supported")
}

func (m *MockOrchestrator) dumpKnownBackends() {
	log.Debug(">>>MockOrchestrator#dumpKnownBackends")
	defer log.Debug("<<<MockOrchestrator#dumpKnownBackends")

	log.WithFields(log.Fields{
		"len(m.backendsByUUID)":     len(m.backendsByUUID),
		"len(m.mockBackendsByUUID)": len(m.mockBackendsByUUID),
	}).Debug("MockOrchestrator#dumpKnownBackends spinning through backends")
	for _, backend := range m.backendsByUUID {
		log.WithFields(log.Fields{
			"backend.Name":        backend.Name,
			"backend.BackendUUID": backend.BackendUUID,
		}).Debug("MockOrchestrator#getBackendByName found")
	}
}

func (m *MockOrchestrator) getBackendByName(backendName string) (*storage.Backend, error) {
	log.WithFields(log.Fields{"backendName": backendName}).Debug(">>>MockOrchestrator#getBackendByName")
	defer log.WithFields(log.Fields{"backendName": backendName}).Debug("<<<MockOrchestrator#getBackendByName")

	log.WithFields(log.Fields{
		"len(m.backendsByUUID)":     len(m.backendsByUUID),
		"len(m.mockBackendsByUUID)": len(m.mockBackendsByUUID),
	}).Debug("MockOrchestrator#getBackendByName spinning through backends")
	for _, backend := range m.backendsByUUID {
		log.WithFields(log.Fields{
			"backendName":  backendName,
			"backend.Name": backend.Name,
		}).Debug("MockOrchestrator#getBackendByName checking")
		if backend.Name == backendName {
			log.Debug("MockOrchestrator#getBackendByName returning")
			return backend, nil
		}
	}

	log.Debug("MockOrchestrator#getBackendByName giving up, not found")
	return nil, utils.NotFoundError("not found")
}

func (m *MockOrchestrator) GetBackend(ctx context.Context, backendName string) (*storage.BackendExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	//b, found := m.backends[backendName]
	b, err := m.getBackendByName(backendName)
	if err != nil {
		return nil, err
	}

	return b.ConstructExternal(), nil
}

func (m *MockOrchestrator) GetBackendByBackendUUID(ctx context.Context, backendUUID string) (*storage.BackendExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	//b, found := m.backends[backendUUID]
	b, found := m.backendsByUUID[backendUUID]
	if !found {
		return nil, utils.NotFoundError("not found")
	}
	return b.ConstructExternal(), nil
}

func (m *MockOrchestrator) ListBackends(context.Context) ([]*storage.BackendExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	// backends := make([]*storage.BackendExternal, 0, len(m.backends))
	// for _, b := range m.backends {
	backends := make([]*storage.BackendExternal, 0, len(m.backendsByUUID))
	for _, b := range m.backendsByUUID {
		backends = append(backends, b.ConstructExternal())
	}
	return backends, nil
}

func (m *MockOrchestrator) DeleteBackend(ctx context.Context, backend string) error {
	// Implement this if it becomes necessary to test.
	return nil
}

func (m *MockOrchestrator) DeleteBackendByBackendUUID(ctx context.Context, backendName, backendUUID string) error {
	// Implement this if it becomes necessary to test.
	return nil
}

func (m *MockOrchestrator) AddVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error) {
	var mockBackends map[string]*mockBackend

	// Don't bother with actually getting the backends from the storage class;
	// to test that logic, use an instance of the real orchestrator.  Perform
	// a sanity check on the storage class, though, to catch odd behavior,
	// like passing in something not intended.
	if _, ok := m.storageClasses[volumeConfig.StorageClass]; !ok {
		return nil, fmt.Errorf("storage class %s not found for volume %s",
			volumeConfig.StorageClass, volumeConfig.Name)
	}

	rand.Seed(time.Now().UnixNano())
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if volumeConfig.Protocol == config.ProtocolAny {
		//mockBackends = m.mockBackends
		mockBackends = m.mockBackendsByUUID
	} else {
		mockBackends = make(map[string]*mockBackend)
		//for name, b := range m.mockBackends {
		for _, b := range m.mockBackendsByUUID {
			name := b.name
			log.WithFields(
				log.Fields{"backendName": name, "protocol": b.protocol},
			).Infof("Checking backend for protocol %s", volumeConfig.Protocol)
			if b.protocol == volumeConfig.Protocol {
				log.WithFields(
					log.Fields{"backendName": name, "protocol": b.protocol},
				).Info("Found match.")
				mockBackends[name] = b
			}
		}
	}
	if _, ok := m.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
	}
	if len(mockBackends) == 0 {
		log.Panic("No mock backends available; something is wrong.")
	}
	index := rand.Intn(len(mockBackends))
	backendName := reflect.ValueOf(mockBackends).MapKeys()[index].String()
	mockBackend := mockBackends[backendName]
	//backend := m.backends[backendName]
	backend, err := m.getBackendByName(backendName)
	if err != nil {
		return nil, err
	}
	backendUUID := backend.BackendUUID
	// Use something other than the volume config name itself.
	volumeConfig.InternalName = GetFakeInternalName(volumeConfig.Name)
	if mockBackend.protocol == config.File {
		volumeConfig.AccessInfo.NfsServerIP = mockBackend.accessInfo.NfsServerIP
	}
	volumeConfig.AccessInfo.NfsPath = fmt.Sprintf("/%s",
		GetFakeInternalName(volumeConfig.Name))
	volume := &storage.Volume{
		Config:      volumeConfig,
		BackendUUID: backendUUID,
		Pool:        "fake",
	}
	mockBackend.volumes[volumeConfig.Name] = volume
	m.volumes[volumeConfig.Name] = volume
	return volume.ConstructExternal(), nil
}

func (m *MockOrchestrator) CloneVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error) {
	// TODO: write this method to enable CloneVolume unit tests
	return nil, nil
}

func (m *MockOrchestrator) GetVolumeExternal(ctx context.Context, volumeName string, backendName string) (*storage.VolumeExternal, error) {
	// TODO: write this method to enable GetVolumeExternal unit tests
	return nil, nil
}

func (m *MockOrchestrator) LegacyImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig, backendName string, notManaged bool, createPVandPVC VolumeCallback) (externalVol *storage.VolumeExternal, err error) {

	// TODO: write this method to enable GetVolumeExternal unit tests
	return nil, nil
}

func (m *MockOrchestrator) ImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (externalVol *storage.VolumeExternal, err error) {

	// TODO: write this method to enable GetVolumeExternal unit tests
	return nil, nil
}

func (m *MockOrchestrator) ValidateVolumes(
	t *testing.T,
	expectedConfigs []*storage.VolumeConfig,
) bool {
	correct := true
	for _, volConfig := range expectedConfigs {
		vol, ok := m.volumes[volConfig.Name]
		if !ok {
			t.Errorf("no volumes found for %s", volConfig.Name)
			correct = false
			continue
		}
		if !reflect.DeepEqual(vol.Config, volConfig) {
			t.Errorf("volume configs differ for %s:\n"+
				"\tExpected: %v\n\tActual: %v", volConfig.Name, volConfig, vol.Config)
			correct = false
		}
	}
	return correct
}

func (m *MockOrchestrator) GetVolume(ctx context.Context, volume string) (*storage.VolumeExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	vol, found := m.volumes[volume]
	if !found {
		return nil, utils.NotFoundError("not found")
	}
	return vol.ConstructExternal(), nil
}

func (m *MockOrchestrator) SetVolumeState(ctx context.Context, volumeName string, state storage.VolumeState) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	vol, found := m.volumes[volumeName]
	if !found {
		return utils.NotFoundError("not found")
	}
	vol.State = state
	return nil
}

// Copied verbatim from TridentOrchestrator
func (m *MockOrchestrator) GetDriverTypeForVolume(ctx context.Context, vol *storage.VolumeExternal) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	//if b, ok := m.backends[vol.BackendUUID]; ok {
	if b, ok := m.backendsByUUID[vol.BackendUUID]; ok {
		return b.Driver.Name(), nil
	}
	return config.UnknownDriver, nil
}

// Copied verbatim from TridentOrchestrator
func (m *MockOrchestrator) GetVolumeType(ctx context.Context, vol *storage.VolumeExternal) (config.VolumeType, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	//driver := m.backends[vol.BackendUUID].GetDriverName()
	driver := m.backendsByUUID[vol.BackendUUID].GetDriverName()
	switch {
	case driver == drivers.OntapNASStorageDriverName:
		return config.OntapNFS, nil
	case driver == drivers.OntapNASFlexGroupStorageDriverName:
		return config.OntapNFS, nil
	case driver == drivers.OntapSANStorageDriverName:
		return config.OntapISCSI, nil
	case driver == drivers.SolidfireSANStorageDriverName:
		return config.SolidFireISCSI, nil
	case driver == drivers.EseriesIscsiStorageDriverName:
		return config.ESeriesISCSI, nil
	default:
		return config.UnknownVolumeType, nil
	}
}

func (m *MockOrchestrator) ListVolumes(context.Context) ([]*storage.VolumeExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	volumes := make([]*storage.VolumeExternal, 0, len(m.volumes))
	for _, vol := range m.volumes {
		volumes = append(volumes, vol.ConstructExternal())
	}
	return volumes, nil
}

func (m *MockOrchestrator) DeleteVolume(ctx context.Context, volumeName string) error {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Copied verbatim from orchestrator_core so that error returns are identical
	volume, ok := m.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	//delete(m.mockBackends[volume.BackendUUID].volumes, volume.Config.Name)
	delete(m.mockBackendsByUUID[volume.BackendUUID].volumes, volume.Config.Name)
	delete(m.volumes, volume.Config.Name)
	return nil
}

func (m *MockOrchestrator) ListVolumesByPlugin(ctx context.Context, pluginName string) ([]*storage.VolumeExternal, error) {
	// Currently returns nil, since this is backend agnostic.  Change this
	// if we ever have non-apiserver functionality depend on this function.
	return nil, nil
}

func (m *MockOrchestrator) AttachVolume(ctx context.Context, volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo) error {
	return nil
}

func (m *MockOrchestrator) DetachVolume(ctx context.Context, volumeName, mountpoint string) error {
	return nil
}

func (m *MockOrchestrator) PublishVolume(ctx context.Context, volumeName string, publishInfo *utils.VolumePublishInfo) error {
	return nil
}

func (m *MockOrchestrator) CreateSnapshot(ctx context.Context, snapshotConfig *storage.SnapshotConfig) (*storage.SnapshotExternal, error) {
	return nil, nil
}

func (m *MockOrchestrator) GetSnapshot(ctx context.Context, volumeName, snapshotName string) (*storage.SnapshotExternal, error) {
	return nil, nil
}

func (m *MockOrchestrator) ListSnapshots(context.Context) ([]*storage.SnapshotExternal, error) {
	return make([]*storage.SnapshotExternal, 0), nil
}

func (m *MockOrchestrator) ListSnapshotsByName(ctx context.Context, snapshotName string) ([]*storage.SnapshotExternal, error) {
	return make([]*storage.SnapshotExternal, 0), nil
}

func (m *MockOrchestrator) ListSnapshotsForVolume(ctx context.Context, volumeName string) ([]*storage.SnapshotExternal, error) {
	return make([]*storage.SnapshotExternal, 0), nil
}

func (m *MockOrchestrator) ReadSnapshotsForVolume(ctx context.Context, volumeName string) ([]*storage.SnapshotExternal, error) {
	return make([]*storage.SnapshotExternal, 0), nil
}

func (m *MockOrchestrator) DeleteSnapshot(ctx context.Context, volumeName, snapshotName string) error {
	return nil
}

func (m *MockOrchestrator) ReloadVolumes(context.Context) error {
	return nil
}

func (m *MockOrchestrator) ResizeVolume(ctx context.Context, volumeName, newSize string) error {
	return nil
}

func NewMockOrchestrator() *MockOrchestrator {
	return &MockOrchestrator{
		backendsByUUID:     make(map[string]*storage.Backend),
		mockBackendsByUUID: make(map[string]*mockBackend),
		// backends:       make(map[string]*storage.Backend),
		// mockBackends:   make(map[string]*mockBackend),
		storageClasses: make(map[string]*storageclass.StorageClass),
		volumes:        make(map[string]*storage.Volume),
		mutex:          &sync.Mutex{},
	}
}

func (m *MockOrchestrator) AddStorageClass(ctx context.Context, scConfig *storageclass.Config) (*storageclass.External, error) {
	sc := storageclass.New(scConfig)
	m.storageClasses[sc.GetName()] = sc
	return sc.ConstructExternal(), nil
}

func (m *MockOrchestrator) GetStorageClass(ctx context.Context, scName string) (*storageclass.External, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	sc, found := m.storageClasses[scName]
	if !found {
		return nil, utils.NotFoundError("not found")
	}
	return sc.ConstructExternal(), nil
}

func (m *MockOrchestrator) ListStorageClasses(context.Context) ([]*storageclass.External, error) {
	ret := make([]*storageclass.External, 0, len(m.storageClasses))
	for _, sc := range m.storageClasses {
		ret = append(ret, sc.ConstructExternal())
	}
	return ret, nil
}

func (m *MockOrchestrator) DeleteStorageClass(ctx context.Context, scName string) error {
	if _, ok := m.storageClasses[scName]; !ok {
		return utils.NotFoundError(fmt.Sprintf("storage class %s not found", scName))
	}
	delete(m.storageClasses, scName)
	return nil
}

func (m *MockOrchestrator) AddNode(ctx context.Context, node *utils.Node) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.nodes[node.Name] = node
	return nil
}

func (m *MockOrchestrator) GetNode(ctx context.Context, nName string) (*utils.Node, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	node, found := m.nodes[nName]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("node %s not found", nName))
	}
	return node, nil
}

func (m *MockOrchestrator) ListNodes(context.Context) ([]*utils.Node, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	ret := make([]*utils.Node, 0, len(m.nodes))
	for _, node := range m.nodes {
		ret = append(ret, node)
	}
	return ret, nil
}

func (m *MockOrchestrator) DeleteNode(ctx context.Context, nName string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, ok := m.nodes[nName]; !ok {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nName))
	}
	delete(m.nodes, nName)
	return nil
}

func (m *MockOrchestrator) AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error {
	return nil
}

func (m *MockOrchestrator) GetVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) (*storage.VolumeTransaction, error) {
	return nil, nil
}

func (m *MockOrchestrator) DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error {
	return nil
}
