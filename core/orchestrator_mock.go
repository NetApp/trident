// Copyright 2019 NetApp, Inc. All Rights Reserved.

package core

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/utils"
)

type mockBackend struct {
	volumes  map[string]*storage.Volume
	protocol config.Protocol
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
	backends       map[string]*storage.Backend
	mockBackends   map[string]*mockBackend
	storageClasses map[string]*storageclass.StorageClass
	volumes        map[string]*storage.Volume
	nodes          map[string]*utils.Node
	mutex          *sync.Mutex
}

func (m *MockOrchestrator) Bootstrap() error {
	return nil
}

func (m *MockOrchestrator) AddFrontend(f frontend.Plugin) {
	// NOP for the time being, since users of MockOrchestrator don't need this
}

func (m *MockOrchestrator) GetVersion() (string, error) {
	return config.OrchestratorVersion.String(), nil
}

// TODO:  Add extra methods to add backends without needing to provide a valid,
// stringified JSON config.
func (m *MockOrchestrator) AddBackend(configJSON string) (*storage.BackendExternal, error) {
	// We need to do this to determine if the backend is NFS or not.
	backend := &storage.Backend{
		Name:    fmt.Sprintf("mock-%d", len(m.backends)),
		Driver:  nil,
		Online:  true,
		State:   storage.Online,
		Storage: make(map[string]*storage.Pool),
	}
	mock := newMockBackend(backend.GetProtocol())
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.backends[backend.Name] = backend
	m.mockBackends[backend.Name] = mock
	return backend.ConstructExternal(), nil
}

// Convenience method for test harnesses to avoid having to create a
// backend config JSON.
func (m *MockOrchestrator) addMockBackend(
	name string, protocol config.Protocol,
) *storage.Backend {
	mock := newMockBackend(protocol)
	backend := &storage.Backend{
		Name:    name,
		Driver:  nil,
		Online:  true,
		State:   storage.Online,
		Storage: make(map[string]*storage.Pool),
	}
	m.backends[backend.Name] = backend
	m.mockBackends[backend.Name] = mock
	return backend
}

func (m *MockOrchestrator) AddMockONTAPNFSBackend(name, lif string) *storage.BackendExternal {
	backend := m.addMockBackend(name, config.File)
	backend.Driver = &ontap.NASStorageDriver{
		Config: drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: "ontap-nas",
			},
			DataLIF: lif,
		},
	}
	mock := m.mockBackends[backend.Name]
	mock.accessInfo.NfsServerIP = lif
	return backend.ConstructExternal()
}

//TODO:  Add other mock backends here as necessary.

// UpdateBackend updates an existing backend
func (m *MockOrchestrator) UpdateBackend(backendName, configJSON string) (
	storageBackendExternal *storage.BackendExternal, err error) {
	//TODO
	return nil, fmt.Errorf("operation not currently supported")
}

// UpdateBackendState updates an existing backend
func (m *MockOrchestrator) UpdateBackendState(backendName, backendState string) (
	storageBackendExternal *storage.BackendExternal, err error) {
	//TODO
	return nil, fmt.Errorf("operation not currently supported")
}

func (m *MockOrchestrator) GetBackend(backend string) (*storage.BackendExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	b, found := m.backends[backend]
	if !found {
		return nil, notFoundError("not found")
	}
	return b.ConstructExternal(), nil
}

func (m *MockOrchestrator) ListBackends() ([]*storage.BackendExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	backends := make([]*storage.BackendExternal, 0, len(m.backends))
	for _, b := range m.backends {
		backends = append(backends, b.ConstructExternal())
	}
	return backends, nil
}

func (m *MockOrchestrator) DeleteBackend(backend string) error {
	// Implement this if it becomes necessary to test.
	return nil
}

func (m *MockOrchestrator) AddVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error) {
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
		mockBackends = m.mockBackends
	} else {
		mockBackends = make(map[string]*mockBackend)
		for name, b := range m.mockBackends {
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
	// Use something other than the volume config name itself.
	volumeConfig.InternalName = GetFakeInternalName(volumeConfig.Name)
	if mockBackend.protocol == config.File {
		volumeConfig.AccessInfo.NfsServerIP = mockBackend.accessInfo.NfsServerIP
	}
	volumeConfig.AccessInfo.NfsPath = fmt.Sprintf("/%s",
		GetFakeInternalName(volumeConfig.Name))
	volume := &storage.Volume{
		Config:  volumeConfig,
		Backend: backendName,
		Pool:    "fake",
	}
	mockBackend.volumes[volumeConfig.Name] = volume
	m.volumes[volumeConfig.Name] = volume
	return volume.ConstructExternal(), nil
}

func (m *MockOrchestrator) CloneVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error) {
	// TODO: write this method to enable CloneVolume unit tests
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

func (m *MockOrchestrator) GetVolume(volume string) (*storage.VolumeExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	vol, found := m.volumes[volume]
	if !found {
		return nil, notFoundError("not found")
	}
	return vol.ConstructExternal(), nil
}

// Copied verbatim from TridentOrchestrator
func (m *MockOrchestrator) GetDriverTypeForVolume(
	vol *storage.VolumeExternal,
) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if b, ok := m.backends[vol.Backend]; ok {
		return b.Driver.Name(), nil
	}
	return config.UnknownDriver, nil
}

// Copied verbatim from TridentOrchestrator
func (m *MockOrchestrator) GetVolumeType(vol *storage.VolumeExternal) (config.VolumeType, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	driver := m.backends[vol.Backend].GetDriverName()
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

func (m *MockOrchestrator) ListVolumes() ([]*storage.VolumeExternal, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	volumes := make([]*storage.VolumeExternal, 0, len(m.volumes))
	for _, vol := range m.volumes {
		volumes = append(volumes, vol.ConstructExternal())
	}
	return volumes, nil
}

func (m *MockOrchestrator) DeleteVolume(volumeName string) error {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Copied verbatim from orchestrator_core so that error returns are identical
	volume, ok := m.volumes[volumeName]
	if !ok {
		return notFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	delete(m.mockBackends[volume.Backend].volumes, volume.Config.Name)
	delete(m.volumes, volume.Config.Name)
	return nil
}

func (m *MockOrchestrator) ListVolumesByPlugin(pluginName string) ([]*storage.VolumeExternal, error) {
	// Currently returns nil, since this is backend agnostic.  Change this
	// if we ever have non-apiserver functionality depend on this function.
	return nil, nil
}

func (m *MockOrchestrator) AttachVolume(volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo) error {
	return nil
}

func (m *MockOrchestrator) DetachVolume(volumeName, mountpoint string) error {
	return nil
}

func (m *MockOrchestrator) PublishVolume(
	volumeName string, publishInfo *utils.VolumePublishInfo) error {
	return nil
}

func (m *MockOrchestrator) ListVolumeSnapshots(volumeName string) ([]*storage.SnapshotExternal, error) {
	return make([]*storage.SnapshotExternal, 0), nil
}

func (m *MockOrchestrator) ReloadVolumes() error {
	return nil
}

func (m *MockOrchestrator) ResizeVolume(volumeName, newSize string) error {
	return nil
}

func NewMockOrchestrator() *MockOrchestrator {
	return &MockOrchestrator{
		backends:       make(map[string]*storage.Backend),
		mockBackends:   make(map[string]*mockBackend),
		storageClasses: make(map[string]*storageclass.StorageClass),
		volumes:        make(map[string]*storage.Volume),
		mutex:          &sync.Mutex{},
	}
}

func (m *MockOrchestrator) AddStorageClass(
	scConfig *storageclass.Config,
) (*storageclass.External, error) {
	sc := storageclass.New(scConfig)
	m.storageClasses[sc.GetName()] = sc
	return sc.ConstructExternal(), nil
}

func (m *MockOrchestrator) GetStorageClass(scName string) (*storageclass.External, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	sc, found := m.storageClasses[scName]
	if !found {
		return nil, notFoundError("not found")
	}
	return sc.ConstructExternal(), nil
}

func (m *MockOrchestrator) ListStorageClasses() ([]*storageclass.External, error) {
	ret := make([]*storageclass.External, 0, len(m.storageClasses))
	for _, sc := range m.storageClasses {
		ret = append(ret, sc.ConstructExternal())
	}
	return ret, nil
}

func (m *MockOrchestrator) DeleteStorageClass(scName string) error {
	if _, ok := m.storageClasses[scName]; !ok {
		return notFoundError(fmt.Sprintf("storage class %s not found", scName))
	}
	delete(m.storageClasses, scName)
	return nil
}

func (m *MockOrchestrator) AddNode(node *utils.Node) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.nodes[node.Name] = node
	return nil
}

func (m *MockOrchestrator) GetNode(nName string) (*utils.Node, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	node, found := m.nodes[nName]
	if !found {
		return nil, notFoundError(fmt.Sprintf("node %s not found", nName))
	}
	return node, nil
}

func (m *MockOrchestrator) ListNodes() ([]*utils.Node, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	ret := make([]*utils.Node, 0, len(m.nodes))
	for _, node := range m.nodes {
		ret = append(ret, node)
	}
	return ret, nil
}

func (m *MockOrchestrator) DeleteNode(nName string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, ok := m.nodes[nName]; !ok {
		return notFoundError(fmt.Sprintf("node %s not found", nName))
	}
	delete(m.nodes, nName)
	return nil
}
