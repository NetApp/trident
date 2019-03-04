// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
)

type InMemoryClient struct {
	backends            map[string]*storage.BackendPersistent
	backendsAdded       int
	volumes             map[string]*storage.VolumeExternal
	volumesAdded        int
	storageClasses      map[string]*sc.Persistent
	storageClassesAdded int
	volumeTxns          map[string]*VolumeTransaction
	volumeTxnsAdded     int
	version             *PersistentStateVersion
	snapshots           map[string][]*storage.SnapshotExternal
	snapshotsAdded      int
}

func NewInMemoryClient() *InMemoryClient {
	return &InMemoryClient{
		backends:       make(map[string]*storage.BackendPersistent),
		volumes:        make(map[string]*storage.VolumeExternal),
		storageClasses: make(map[string]*sc.Persistent),
		volumeTxns:     make(map[string]*VolumeTransaction),
		snapshots:      make(map[string][]*storage.SnapshotExternal),
		version: &PersistentStateVersion{
			"memory", config.OrchestratorAPIVersion,
		},
	}
}

func (c *InMemoryClient) GetType() StoreType {
	return MemoryStore
}

func (c *InMemoryClient) Stop() error {
	c.backendsAdded = 0
	c.volumesAdded = 0
	c.storageClassesAdded = 0
	c.volumeTxnsAdded = 0
	c.snapshotsAdded = 0
	return nil
}

func (c *InMemoryClient) GetConfig() *ClientConfig {
	return &ClientConfig{}
}

func (c *InMemoryClient) GetVersion() (*PersistentStateVersion, error) {
	return c.version, nil
}

func (c *InMemoryClient) SetVersion(version *PersistentStateVersion) error {
	return nil
}

func (c *InMemoryClient) AddBackend(b *storage.Backend) error {
	backend := b.ConstructPersistent()
	if _, ok := c.backends[backend.Name]; ok {
		return fmt.Errorf("backend %s already exists", backend.Name)
	}
	c.backends[backend.Name] = backend
	c.backendsAdded++
	return nil
}

func (c *InMemoryClient) GetBackend(backendName string) (*storage.BackendPersistent, error) {
	ret, ok := c.backends[backendName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}
	return ret, nil
}

func (c *InMemoryClient) UpdateBackend(b *storage.Backend) error {
	// UpdateBackend requires the backend to already exist.
	if _, ok := c.backends[b.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, b.Name)
	}
	c.backends[b.Name] = b.ConstructPersistent()
	return nil
}

func (c *InMemoryClient) DeleteBackend(b *storage.Backend) error {
	if _, ok := c.backends[b.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, b.Name)
	}
	delete(c.backends, b.Name)
	return nil
}

// ReplaceBackendAndUpdateVolumes renames a backend and updates all volumes to
// reflect the new backend name
func (c *InMemoryClient) ReplaceBackendAndUpdateVolumes(
	origBackend, newBackend *storage.Backend) error {
	//TODO
	return NewPersistentStoreError(NotSupported, "")
}

func (c *InMemoryClient) GetBackends() ([]*storage.BackendPersistent, error) {
	backendList := make([]*storage.BackendPersistent, 0)
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
	c.backends = make(map[string]*storage.BackendPersistent)
	return nil
}

func (c *InMemoryClient) AddVolume(vol *storage.Volume) error {
	volume := vol.ConstructExternal()
	if _, ok := c.volumes[volume.Config.Name]; ok {
		return fmt.Errorf("volume %s already exists", volume.Config.Name)
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
		return fmt.Errorf("storage class %s already exists", storageClass.GetName())
	}
	c.storageClasses[storageClass.GetName()] = storageClass
	c.storageClassesAdded++
	return nil
}

func (c *InMemoryClient) GetStorageClass(scName string) (
	*sc.Persistent, error,
) {
	ret, ok := c.storageClasses[scName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, scName)
	}
	return ret, nil
}

func (c *InMemoryClient) GetStorageClasses() (
	[]*sc.Persistent, error,
) {
	ret := make([]*sc.Persistent, 0, len(c.storageClasses))
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

func (c *InMemoryClient) AddSnapshot(volName string, snapshot *storage.Snapshot) error {
	if _, ok := c.snapshots[volName]; !ok {
		c.snapshots[volName] = make([]*storage.SnapshotExternal, 0)
	}
	snapExternal := snapshot.ConstructExternal()
	c.snapshots[volName] = append(c.snapshots[volName], snapExternal)
	c.snapshotsAdded++
	return nil
}

// GetSnapshot retrieves a snapshot state from the persistent store
func (c *InMemoryClient) GetSnapshot(volName, snapshotName string) (*storage.SnapshotExternal, error) {
	snapshots, ok := c.snapshots[volName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, volName)
	}
	var ret *storage.SnapshotExternal
	for _, snapExternal := range snapshots {
		if snapExternal.Name == snapshotName {
			ret = snapExternal
		}
	}
	if ret == nil {
		return nil, NewPersistentStoreError(KeyNotFoundErr, snapshotName)
	}
	return ret, nil
}
