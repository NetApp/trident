// Copyright 2016 NetApp, Inc. All Rights Reserved.

package core

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/ontap"
	"github.com/netapp/trident/storage_class"
)

type mockBackend struct {
	volumes  map[string]*storage.Volume
	protocol config.Protocol
	// Store non-volume specific access info here
	accessInfo storage.VolumeAccessInfo
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
// tridentOrchestrator, since their functionality is not inherently interesting
// or testable.
type MockOrchestrator struct {
	backends       map[string]*storage.StorageBackend
	mockBackends   map[string]*mockBackend
	storageClasses map[string]*storage_class.StorageClass
	volumes        map[string]*storage.Volume
	mutex          *sync.Mutex
}

func (m *MockOrchestrator) Bootstrap() error {
	return nil
}

func (m *MockOrchestrator) AddFrontend(f frontend.FrontendPlugin) {
	// NOP for the time being, since users of MockOrchestrator don't need this
}

func (o *MockOrchestrator) GetVersion() string {
	return config.OrchestratorFullVersion
}

// TODO:  Add extra methods to add backends without needing to provide a valid,
// stringified JSON config.
func (m *MockOrchestrator) AddStorageBackend(configJSON string) (*storage.StorageBackendExternal, error) {
	// We need to do this to determine if the backend is NFS or not.
	backend := &storage.StorageBackend{
		Name:    fmt.Sprintf("mock-%d", len(m.backends)),
		Driver:  nil,
		Online:  true,
		Storage: make(map[string]*storage.StoragePool),
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
) *storage.StorageBackend {
	mock := newMockBackend(protocol)
	backend := &storage.StorageBackend{
		Name:    name,
		Driver:  nil,
		Online:  true,
		Storage: make(map[string]*storage.StoragePool),
	}
	m.backends[backend.Name] = backend
	m.mockBackends[backend.Name] = mock
	return backend
}

func (m *MockOrchestrator) AddMockONTAPNFSBackend(name, lif string) *storage.StorageBackendExternal {
	backend := m.addMockBackend(name, config.File)
	backend.Driver = &ontap.OntapNASStorageDriver{
		OntapNASStorageDriver: dvp.OntapNASStorageDriver{
			Config: dvp.OntapStorageDriverConfig{
				CommonStorageDriverConfig: &dvp.CommonStorageDriverConfig{
					StorageDriverName: "ontap-nas",
				},
				DataLIF: lif,
			},
		},
	}
	mock := m.mockBackends[backend.Name]
	mock.accessInfo.NfsServerIP = lif
	return backend.ConstructExternal()
}

//TODO:  Add other mock backends here as necessary.

func (m *MockOrchestrator) GetBackend(backend string) *storage.StorageBackendExternal {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	b, found := m.backends[backend]
	if !found {
		return nil
	}
	return b.ConstructExternal()
}

func (m *MockOrchestrator) ListBackends() []*storage.StorageBackendExternal {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	backends := make([]*storage.StorageBackendExternal, 0, len(m.backends))
	for _, b := range m.backends {
		backends = append(backends, b.ConstructExternal())
	}
	return backends
}

func (m *MockOrchestrator) OfflineBackend(backend string) (bool, error) {
	// Implement this if it becomes necessary to test.
	return false, nil
}

func (m *MockOrchestrator) AddVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error) {
	var mockBackends map[string]*mockBackend

	// Don't bother with actually getting the backends from the storage class;
	// to test that logic, use an instance of the real orchestrator.  Perform
	// a sanity check on the storage class, though, to catch odd behavior,
	// like passing in something not intended.
	if _, ok := m.storageClasses[volumeConfig.StorageClass]; !ok {
		return nil, fmt.Errorf("Storage class %s not found for volume %s",
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
		return nil, fmt.Errorf("Volume %s already exists.", volumeConfig.Name)
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
		Backend: m.backends[backendName],
		Pool:    &storage.StoragePool{Name: "fake"},
	}
	mockBackend.volumes[volumeConfig.Name] = volume
	m.volumes[volumeConfig.Name] = volume
	return volume.ConstructExternal(), nil
}

func (m *MockOrchestrator) ValidateVolumes(
	t *testing.T,
	expectedConfigs []*storage.VolumeConfig,
) bool {
	correct := true
	for _, config := range expectedConfigs {
		vol, ok := m.volumes[config.Name]
		if !ok {
			t.Errorf("No volumes found for %s", config.Name)
			correct = false
			continue
		}
		if !reflect.DeepEqual(vol.Config, config) {
			t.Errorf("Volume configs differ for %s:\n"+
				"\tExpected:  %v\n\tActual:  %v", config.Name, config,
				vol.Config)
			correct = false
		}
	}
	return correct
}

func (m *MockOrchestrator) GetVolume(volume string) *storage.VolumeExternal {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	vol, found := m.volumes[volume]
	if !found {
		return nil
	}
	return vol.ConstructExternal()
}

// Copied verbatim from tridentOrchestrator
func (m *MockOrchestrator) GetDriverTypeForVolume(
	vol *storage.VolumeExternal,
) string {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if b, ok := m.backends[vol.Backend]; ok {
		return b.Driver.Name()
	}
	return config.UnknownDriver
}

// Copied verbatim from tridentOrchestrator
func (m *MockOrchestrator) GetVolumeType(vol *storage.VolumeExternal) config.VolumeType {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	driver := m.backends[vol.Backend].GetDriverName()
	switch {
	case driver == dvp.OntapNASStorageDriverName:
		return config.ONTAP_NFS
	case driver == dvp.OntapSANStorageDriverName:
		return config.ONTAP_iSCSI
	case driver == dvp.SolidfireSANStorageDriverName:
		return config.SolidFire_iSCSI
	case driver == dvp.EseriesIscsiStorageDriverName:
		return config.Eseries_iSCSI
	default:
		return config.UnknownVolumeType
	}
}

func (m *MockOrchestrator) ListVolumes() []*storage.VolumeExternal {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	volumes := make([]*storage.VolumeExternal, 0, len(m.volumes))
	for _, vol := range m.volumes {
		volumes = append(volumes, vol.ConstructExternal())
	}
	return volumes
}

func (m *MockOrchestrator) DeleteVolume(volumeName string) (found bool, err error) {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Copied verbatim from orchestrator_core so that error returns are identical
	volume, ok := m.volumes[volumeName]
	if !ok {
		return false, fmt.Errorf("Volume %s not found.", volumeName)
	}

	delete(m.mockBackends[volume.Backend.Name].volumes, volume.Config.Name)
	delete(m.volumes, volume.Config.Name)
	return true, nil
}

func (m *MockOrchestrator) ListVolumesByPlugin(pluginName string) []*storage.VolumeExternal {
	// Currently returns nil, since this is backend agnostic.  Change this
	// if we ever have non-apiserver functionality depend on this function.
	return nil
}

func NewMockOrchestrator() *MockOrchestrator {
	return &MockOrchestrator{
		backends:       make(map[string]*storage.StorageBackend),
		mockBackends:   make(map[string]*mockBackend),
		storageClasses: make(map[string]*storage_class.StorageClass),
		volumes:        make(map[string]*storage.Volume),
		mutex:          &sync.Mutex{},
	}
}

func (m *MockOrchestrator) AddStorageClass(
	scConfig *storage_class.Config,
) (*storage_class.StorageClassExternal, error) {
	sc := storage_class.New(scConfig)
	m.storageClasses[sc.GetName()] = sc
	return sc.ConstructExternal(), nil
}

func (m *MockOrchestrator) GetStorageClass(scName string) *storage_class.StorageClassExternal {
	if sc, ok := m.storageClasses[scName]; ok {
		return sc.ConstructExternal()
	}
	return nil
}

func (m *MockOrchestrator) ListStorageClasses() []*storage_class.StorageClassExternal {
	ret := make([]*storage_class.StorageClassExternal, 0, len(m.storageClasses))
	for _, sc := range m.storageClasses {
		ret = append(ret, sc.ConstructExternal())
	}
	return ret
}

func (m *MockOrchestrator) DeleteStorageClass(scName string) (bool, error) {
	_, ok := m.storageClasses[scName]
	if !ok {
		return false, fmt.Errorf("Storage class %s not found.", scName)
	}
	delete(m.storageClasses, scName)
	return true, nil
}
