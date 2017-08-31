// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"fmt"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
)

type InMemoryClient struct {
	backends            map[string]*storage.StorageBackendPersistent
	backendsAdded       int
	volumes             map[string]*storage.VolumeExternal
	volumesAdded        int
	storageClasses      map[string]*sc.StorageClassPersistent
	storageClassesAdded int
	volumeTxns          map[string]*VolumeTransaction
	volumeTxnsAdded     int
	version             *PersistentStateVersion
}

func NewInMemoryClient() *InMemoryClient {
	return &InMemoryClient{
		backends:       make(map[string]*storage.StorageBackendPersistent),
		volumes:        make(map[string]*storage.VolumeExternal),
		storageClasses: make(map[string]*sc.StorageClassPersistent),
		volumeTxns:     make(map[string]*VolumeTransaction),
		version: &PersistentStateVersion{
			"memory", config.OrchestratorAPIVersion,
		},
	}
}

func (c *InMemoryClient) Create(key, value string) error {
	return nil
}

func (c *InMemoryClient) Read(key string) (string, error) {
	return "", nil
}

func (c *InMemoryClient) ReadKeys(keyPrefix string) ([]string, error) {
	return make([]string, 0), nil
}

func (c *InMemoryClient) Update(key, value string) error {
	return nil
}

func (c *InMemoryClient) Set(key, value string) error {
	return nil
}

func (c *InMemoryClient) Delete(key string) error {
	return nil
}

func (c *InMemoryClient) DeleteKeys(keyPrefix string) error {
	return nil
}

func (p *InMemoryClient) GetType() StoreType {
	return MemoryStore
}

func (c *InMemoryClient) Stop() error {
	c.backendsAdded = 0
	c.volumesAdded = 0
	c.storageClassesAdded = 0
	c.volumeTxnsAdded = 0
	return nil
}

func (p *InMemoryClient) GetEndpoints() string {
	return ""
}

func (p *InMemoryClient) GetVersion() (*PersistentStateVersion, error) {
	return p.version, nil
}

func (p *InMemoryClient) SetVersion(version *PersistentStateVersion) error {
	return nil
}

func (c *InMemoryClient) AddBackend(b *storage.StorageBackend) error {
	backend := b.ConstructPersistent()
	if _, ok := c.backends[backend.Name]; ok {
		return fmt.Errorf("Backend %s already exists.", backend.Name)
	}
	c.backends[backend.Name] = backend
	c.backendsAdded++
	return nil
}

func (c *InMemoryClient) GetBackend(backendName string) (*storage.StorageBackendPersistent, error) {
	ret, ok := c.backends[backendName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}
	return ret, nil
}

func (c *InMemoryClient) UpdateBackend(b *storage.StorageBackend) error {
	// UpdateBackend requires the backend to already exist.
	if _, ok := c.backends[b.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, b.Name)
	}
	c.backends[b.Name] = b.ConstructPersistent()
	return nil
}

func (c *InMemoryClient) DeleteBackend(b *storage.StorageBackend) error {
	if _, ok := c.backends[b.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, b.Name)
	}
	delete(c.backends, b.Name)
	return nil
}

func (c *InMemoryClient) GetBackends() ([]*storage.StorageBackendPersistent, error) {
	backendList := make([]*storage.StorageBackendPersistent, 0)
	if c.backendsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return backendList, nil
	}
	for _, b := range c.backends {
		backendList = append(backendList, b)
	}
	return backendList, nil
}

func (c *InMemoryClient) DeleteBackends() error {
	if c.backendsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return NewPersistentStoreError(KeyNotFoundErr, "Backends")
	}
	c.backends = make(map[string]*storage.StorageBackendPersistent)
	return nil
}

func (c *InMemoryClient) AddVolume(vol *storage.Volume) error {
	volume := vol.ConstructExternal()
	if _, ok := c.volumes[volume.Config.Name]; ok {
		return fmt.Errorf("Volume %s already exists.", volume.Config.Name)
	}
	c.volumes[volume.Config.Name] = volume
	c.volumesAdded++
	return nil
}

func (c *InMemoryClient) GetVolume(volumeName string) (
	*storage.VolumeExternal, error,
) {
	ret, ok := c.volumes[volumeName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, volumeName)
	}
	return ret, nil
}

func (c *InMemoryClient) UpdateVolume(vol *storage.Volume) error {
	// UpdateVolume requires the volume to already exist.
	if _, ok := c.volumes[vol.Config.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, vol.Config.Name)
	}
	c.volumes[vol.Config.Name] = vol.ConstructExternal()
	return nil
}

func (c *InMemoryClient) DeleteVolume(vol *storage.Volume) error {
	if _, ok := c.volumes[vol.Config.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, vol.Config.Name)
	}
	delete(c.volumes, vol.Config.Name)
	return nil
}

func (c *InMemoryClient) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	delete(c.volumes, vol.Config.Name)
	return nil
}

func (c *InMemoryClient) GetVolumes() ([]*storage.VolumeExternal, error) {
	ret := make([]*storage.VolumeExternal, 0, len(c.volumes))
	if c.volumesAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return ret, nil
	}
	for _, v := range c.volumes {
		ret = append(ret, v)
	}
	return ret, nil
}

func (c *InMemoryClient) DeleteVolumes() error {
	if c.volumesAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return NewPersistentStoreError(KeyNotFoundErr, "Volumes")
	}
	c.volumes = make(map[string]*storage.VolumeExternal)
	return nil
}

func (c *InMemoryClient) AddVolumeTransaction(volTxn *VolumeTransaction) error {
	// AddVolumeTransaction overwrites existing keys, unlike the other methods
	c.volumeTxns[volTxn.getKey()] = volTxn
	c.volumeTxnsAdded++
	return nil
}

func (c *InMemoryClient) GetVolumeTransactions() ([]*VolumeTransaction, error) {
	if c.volumeTxnsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return nil, NewPersistentStoreError(KeyNotFoundErr, "VolumesTransactions")
	}
	ret := make([]*VolumeTransaction, 0, len(c.volumeTxns))
	for _, v := range c.volumeTxns {
		ret = append(ret, v)
	}
	return ret, nil
}

func (c *InMemoryClient) GetExistingVolumeTransaction(
	volTxn *VolumeTransaction) (*VolumeTransaction, error,
) {
	vt, ok := c.volumeTxns[volTxn.getKey()]
	if !ok {
		return nil, nil
	}
	return vt, nil
}

func (c *InMemoryClient) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	if _, ok := c.volumeTxns[volTxn.getKey()]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, "VolumesTransactions")
	}
	delete(c.volumeTxns, volTxn.getKey())
	return nil
}

func (c *InMemoryClient) AddStorageClass(s *sc.StorageClass) error {
	storageClass := s.ConstructPersistent()
	if _, ok := c.storageClasses[storageClass.GetName()]; ok {
		return fmt.Errorf("Storage class %s already exists.", storageClass.GetName())
	}
	c.storageClasses[storageClass.GetName()] = storageClass
	c.storageClassesAdded++
	return nil
}

func (c *InMemoryClient) GetStorageClass(scName string) (
	*sc.StorageClassPersistent, error,
) {
	ret, ok := c.storageClasses[scName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, scName)
	}
	return ret, nil
}

func (c *InMemoryClient) GetStorageClasses() (
	[]*sc.StorageClassPersistent, error,
) {
	ret := make([]*sc.StorageClassPersistent, 0, len(c.storageClasses))
	if c.storageClassesAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return ret, nil
	}
	for _, v := range c.storageClasses {
		ret = append(ret, v)
	}
	return ret, nil
}

func (c *InMemoryClient) DeleteStorageClass(s *sc.StorageClass) error {
	if _, ok := c.storageClasses[s.GetName()]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, s.GetName())
	}
	delete(c.storageClasses, s.GetName())
	return nil
}
