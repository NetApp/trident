// Copyright 2025 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
)

type InMemoryClient struct {
	backends                map[string]*storage.BackendPersistent
	backendsAdded           int
	volumes                 map[string]*storage.VolumeExternal
	volumesAdded            int
	storageClasses          map[string]*sc.Persistent
	storageClassesAdded     int
	volumeTxns              map[string]*storage.VolumeTransaction
	volumeTxnsAdded         int
	volumePublications      map[string]*models.VolumePublication
	volumePublicationsAdded int
	version                 *config.PersistentStateVersion
	nodes                   map[string]*models.Node
	nodesAdded              int
	snapshots               map[string]*storage.SnapshotPersistent
	snapshotsAdded          int
	groupSnapshots          map[string]*storage.GroupSnapshotPersistent
	groupSnapshotsAdded     int
	autogrowPolicies        map[string]*storage.AutogrowPolicyPersistent
	uuid                    string
}

func NewInMemoryClient() *InMemoryClient {
	return &InMemoryClient{
		backends:           make(map[string]*storage.BackendPersistent),
		volumes:            make(map[string]*storage.VolumeExternal),
		storageClasses:     make(map[string]*sc.Persistent),
		volumeTxns:         make(map[string]*storage.VolumeTransaction),
		volumePublications: make(map[string]*models.VolumePublication),
		nodes:              make(map[string]*models.Node),
		snapshots:          make(map[string]*storage.SnapshotPersistent),
		autogrowPolicies:   make(map[string]*storage.AutogrowPolicyPersistent),
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: "memory", OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
		uuid: uuid.NewString(),
	}
}

func (c *InMemoryClient) GetType() StoreType {
	return MemoryStore
}

func (c *InMemoryClient) Stop() error {
	c.backends = make(map[string]*storage.BackendPersistent)
	c.backendsAdded = 0
	c.volumes = make(map[string]*storage.VolumeExternal)
	c.volumesAdded = 0
	c.storageClasses = make(map[string]*sc.Persistent)
	c.storageClassesAdded = 0
	c.volumeTxns = make(map[string]*storage.VolumeTransaction)
	c.volumeTxnsAdded = 0
	c.volumePublications = make(map[string]*models.VolumePublication)
	c.volumePublicationsAdded = 0
	c.nodes = make(map[string]*models.Node)
	c.nodesAdded = 0
	c.snapshots = make(map[string]*storage.SnapshotPersistent)
	c.snapshotsAdded = 0
	c.autogrowPolicies = make(map[string]*storage.AutogrowPolicyPersistent)
	return nil
}

func (c *InMemoryClient) GetConfig() *ClientConfig {
	return &ClientConfig{}
}

func (c *InMemoryClient) GetTridentUUID(_ context.Context) (string, error) {
	return c.uuid, nil
}

func (c *InMemoryClient) GetVersion(context.Context) (*config.PersistentStateVersion, error) {
	return c.version, nil
}

func (c *InMemoryClient) SetVersion(context.Context, *config.PersistentStateVersion) error {
	return nil
}

func (c *InMemoryClient) AddBackend(ctx context.Context, b storage.Backend) error {
	backend := b.ConstructPersistent(ctx)
	if _, ok := c.backends[backend.Name]; ok {
		return fmt.Errorf("backend %s already exists", backend.Name)
	}
	c.backends[backend.Name] = backend
	c.backendsAdded++
	return nil
}

func (c *InMemoryClient) GetBackend(_ context.Context, backendName string) (*storage.BackendPersistent, error) {
	ret, ok := c.backends[backendName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}
	return ret, nil
}

func (c *InMemoryClient) GetBackendSecret(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}

func (c *InMemoryClient) UpdateBackend(ctx context.Context, b storage.Backend) error {
	// UpdateBackend requires the backend to already exist.
	if _, ok := c.backends[b.Name()]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, b.Name())
	}
	c.backends[b.Name()] = b.ConstructPersistent(ctx)
	return nil
}

func (c *InMemoryClient) DeleteBackend(ctx context.Context, b storage.Backend) error {
	delete(c.backends, b.Name())
	return nil
}

func (c *InMemoryClient) IsBackendDeleting(ctx context.Context, backend storage.Backend) bool {
	return false
}

// ReplaceBackendAndUpdateVolumes renames a backend and updates all volumes to
// reflect the new backend name
func (c *InMemoryClient) ReplaceBackendAndUpdateVolumes(
	context.Context, storage.Backend, storage.Backend,
) error {
	// TODO
	return NewPersistentStoreError(NotSupported, "")
}

func (c *InMemoryClient) GetBackends(context.Context) ([]*storage.BackendPersistent, error) {
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

func (c *InMemoryClient) DeleteBackends(context.Context) error {
	if c.backendsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return NewPersistentStoreError(KeyNotFoundErr, "Backends")
	}
	c.backends = make(map[string]*storage.BackendPersistent)
	return nil
}

func (c *InMemoryClient) AddVolume(_ context.Context, vol *storage.Volume) error {
	volume := vol.ConstructExternal()
	if _, ok := c.volumes[volume.Config.Name]; ok {
		return fmt.Errorf("volume %s already exists", volume.Config.Name)
	}
	c.volumes[volume.Config.Name] = volume
	c.volumesAdded++
	return nil
}

func (c *InMemoryClient) GetVolume(_ context.Context, volumeName string) (*storage.VolumeExternal, error) {
	ret, ok := c.volumes[volumeName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, volumeName)
	}
	return ret, nil
}

func (c *InMemoryClient) UpdateVolume(_ context.Context, vol *storage.Volume) error {
	// UpdateVolume requires the volume to already exist.
	if _, ok := c.volumes[vol.Config.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, vol.Config.Name)
	}
	c.volumes[vol.Config.Name] = vol.ConstructExternal()
	return nil
}

func (c *InMemoryClient) DeleteVolume(_ context.Context, vol *storage.Volume) error {
	delete(c.volumes, vol.Config.Name)
	return nil
}

func (c *InMemoryClient) GetVolumes(context.Context) ([]*storage.VolumeExternal, error) {
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

func (c *InMemoryClient) DeleteVolumes(context.Context) error {
	if c.volumesAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return NewPersistentStoreError(KeyNotFoundErr, "Volumes")
	}
	c.volumes = make(map[string]*storage.VolumeExternal)
	return nil
}

func (c *InMemoryClient) AddVolumeTransaction(_ context.Context, volTxn *storage.VolumeTransaction) error {
	// AddVolumeTransaction overwrites existing keys, unlike the other methods
	c.volumeTxns[volTxn.Name()] = volTxn
	c.volumeTxnsAdded++
	return nil
}

func (c *InMemoryClient) GetVolumeTransactions(context.Context) ([]*storage.VolumeTransaction, error) {
	if c.volumeTxnsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return nil, NewPersistentStoreError(KeyNotFoundErr, "VolumesTransactions")
	}
	ret := make([]*storage.VolumeTransaction, 0, len(c.volumeTxns))
	for _, v := range c.volumeTxns {
		ret = append(ret, v)
	}
	return ret, nil
}

func (c *InMemoryClient) UpdateVolumeTransaction(_ context.Context, volTxn *storage.VolumeTransaction) error {
	c.volumeTxns[volTxn.Name()] = volTxn
	return nil
}

func (c *InMemoryClient) GetVolumeTransaction(
	_ context.Context, volTxn *storage.VolumeTransaction,
) (*storage.VolumeTransaction, error) {
	vt, ok := c.volumeTxns[volTxn.Name()]
	if !ok {
		return nil, nil
	}
	return vt, nil
}

func (c *InMemoryClient) DeleteVolumeTransaction(_ context.Context, volTxn *storage.VolumeTransaction) error {
	delete(c.volumeTxns, volTxn.Name())
	return nil
}

func (c *InMemoryClient) AddStorageClass(_ context.Context, s *sc.StorageClass) error {
	storageClass := s.ConstructPersistent()
	if _, ok := c.storageClasses[storageClass.GetName()]; ok {
		return fmt.Errorf("storage class %s already exists", storageClass.GetName())
	}
	c.storageClasses[storageClass.GetName()] = storageClass
	c.storageClassesAdded++
	return nil
}

func (c *InMemoryClient) UpdateStorageClass(_ context.Context, sc *sc.StorageClass) error {
	scName := sc.GetName()
	if _, ok := c.storageClasses[scName]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, scName)
	}
	c.storageClasses[scName] = sc.ConstructPersistent()
	return nil
}

func (c *InMemoryClient) GetStorageClass(_ context.Context, scName string) (*sc.Persistent, error) {
	ret, ok := c.storageClasses[scName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, scName)
	}
	return ret, nil
}

func (c *InMemoryClient) GetStorageClasses(context.Context) ([]*sc.Persistent, error) {
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

func (c *InMemoryClient) DeleteStorageClass(_ context.Context, s *sc.StorageClass) error {
	delete(c.storageClasses, s.GetName())
	return nil
}

func (c *InMemoryClient) AddOrUpdateNode(_ context.Context, n *models.Node) error {
	exists := false
	if _, ok := c.nodes[n.Name]; ok {
		exists = true
	}
	c.nodes[n.Name] = n
	if !exists {
		c.nodesAdded++
	}
	return nil
}

func (c *InMemoryClient) GetNode(_ context.Context, nName string) (*models.Node, error) {
	ret, ok := c.nodes[nName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, nName)
	}
	return ret, nil
}

func (c *InMemoryClient) GetNodes(context.Context) ([]*models.Node, error) {
	ret := make([]*models.Node, 0, len(c.nodes))
	if c.nodesAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return ret, nil
	}
	for _, v := range c.nodes {
		ret = append(ret, v)
	}
	return ret, nil
}

func (c *InMemoryClient) DeleteNode(_ context.Context, n *models.Node) error {
	delete(c.nodes, n.Name)
	return nil
}

func (c *InMemoryClient) AddVolumePublication(_ context.Context, vp *models.VolumePublication) error {
	if _, exists := c.volumePublications[vp.Name]; !exists {
		c.volumePublicationsAdded++
	}
	c.volumePublications[vp.Name] = vp
	return nil
}

func (c *InMemoryClient) UpdateVolumePublication(_ context.Context, vp *models.VolumePublication) error {
	return c.AddVolumePublication(context.TODO(), vp)
}

func (c *InMemoryClient) GetVolumePublication(_ context.Context, vpName string) (*models.VolumePublication, error) {
	ret, ok := c.volumePublications[vpName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, vpName)
	}
	return ret, nil
}

func (c *InMemoryClient) GetVolumePublications(context.Context) ([]*models.VolumePublication, error) {
	ret := make([]*models.VolumePublication, 0, len(c.volumePublications))
	if c.volumePublicationsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return ret, nil
	}
	for _, v := range c.volumePublications {
		ret = append(ret, v)
	}
	return ret, nil
}

func (c *InMemoryClient) DeleteVolumePublication(_ context.Context, vp *models.VolumePublication) error {
	delete(c.volumePublications, vp.Name)
	return nil
}

func (c *InMemoryClient) AddSnapshot(_ context.Context, snapshot *storage.Snapshot) error {
	snapPersistent := snapshot.ConstructPersistent()
	c.snapshots[snapshot.ID()] = snapPersistent
	c.snapshotsAdded++
	return nil
}

// GetSnapshot retrieves a snapshot state from the persistent store
func (c *InMemoryClient) GetSnapshot(_ context.Context, volumeName, snapshotName string) (
	*storage.SnapshotPersistent, error,
) {
	ret, ok := c.snapshots[storage.MakeSnapshotID(volumeName, snapshotName)]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, snapshotName)
	}
	return ret, nil
}

// GetSnapshots retrieves all snapshots for all volumes
func (c *InMemoryClient) GetSnapshots(context.Context) ([]*storage.SnapshotPersistent, error) {
	ret := make([]*storage.SnapshotPersistent, 0, len(c.snapshots))
	if c.snapshotsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return ret, nil
	}
	for _, s := range c.snapshots {
		ret = append(ret, s)
	}
	return ret, nil
}

func (c *InMemoryClient) UpdateSnapshot(_ context.Context, snapshot *storage.Snapshot) error {
	// UpdateSnapshot requires the snapshot to already exist.
	if _, ok := c.snapshots[snapshot.ID()]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, snapshot.Config.Name)
	}
	c.snapshots[snapshot.ID()] = snapshot.ConstructPersistent()
	return nil
}

// DeleteSnapshot deletes a snapshot from the persistent store
func (c *InMemoryClient) DeleteSnapshot(_ context.Context, snapshot *storage.Snapshot) error {
	delete(c.snapshots, snapshot.ID())
	c.snapshotsAdded--
	return nil
}

// DeleteSnapshots deletes all snapshots
func (c *InMemoryClient) DeleteSnapshots(context.Context) error {
	if c.snapshotsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return NewPersistentStoreError(KeyNotFoundErr, "Snapshots")
	}
	c.snapshots = make(map[string]*storage.SnapshotPersistent)
	c.snapshotsAdded = 0
	return nil
}

func (c *InMemoryClient) AddGroupSnapshot(_ context.Context, groupSnapshot *storage.GroupSnapshot) error {
	groupSnapPersistent := groupSnapshot.ConstructPersistent()
	if _, ok := c.groupSnapshots[groupSnapPersistent.ID()]; !ok {
		c.groupSnapshotsAdded++
	}
	if len(c.groupSnapshots) == 0 {
		c.groupSnapshots = make(map[string]*storage.GroupSnapshotPersistent)
	}
	c.groupSnapshots[groupSnapPersistent.ID()] = groupSnapPersistent
	return nil
}

func (c *InMemoryClient) GetGroupSnapshot(_ context.Context, groupSnapshotName string) (*storage.GroupSnapshotPersistent, error) {
	groupSnapshot, ok := c.groupSnapshots[groupSnapshotName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, groupSnapshotName)
	}
	return groupSnapshot, nil
}

func (c *InMemoryClient) GetGroupSnapshots(_ context.Context) ([]*storage.GroupSnapshotPersistent, error) {
	tgsnapList := make([]*storage.GroupSnapshotPersistent, 0, len(c.groupSnapshots))
	if c.groupSnapshotsAdded == 0 {
		// Try to match etcd semantics as closely as possible.
		return tgsnapList, nil
	}

	for _, tgsnap := range c.groupSnapshots {
		tgsnapList = append(tgsnapList, tgsnap)
	}
	return tgsnapList, nil
}

func (c *InMemoryClient) UpdateGroupSnapshot(_ context.Context, groupSnapshot *storage.GroupSnapshot) error {
	// UpdateGroupSnapshot requires the groupSnapshot to already exist.
	if _, ok := c.groupSnapshots[groupSnapshot.ID()]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, groupSnapshot.ID())
	}
	c.groupSnapshots[groupSnapshot.ID()] = groupSnapshot.ConstructPersistent()
	return nil
}

func (c *InMemoryClient) DeleteGroupSnapshot(_ context.Context, groupSnapshot *storage.GroupSnapshot) error {
	if _, ok := c.groupSnapshots[groupSnapshot.ID()]; !ok {
		// Try to match etcd semantics as closely as possible.
		return NewPersistentStoreError(KeyNotFoundErr, "GroupSnapshots")
	}
	delete(c.groupSnapshots, groupSnapshot.ID())
	c.groupSnapshotsAdded--
	return nil
}

// DeleteGroupSnapshots deletes all group snapshots
func (c *InMemoryClient) DeleteGroupSnapshots(_ context.Context) error {
	if c.groupSnapshotsAdded == 0 {
		return NewPersistentStoreError(KeyNotFoundErr, "GroupSnapshots")
	}
	c.groupSnapshots = make(map[string]*storage.GroupSnapshotPersistent)
	c.groupSnapshotsAdded = 0
	return nil
}

func (c *InMemoryClient) GetAutogrowPolicy(_ context.Context, agPolicyName string) (*storage.AutogrowPolicyPersistent, error) {
	agPolicy, ok := c.autogrowPolicies[agPolicyName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, agPolicyName)
	}
	return agPolicy, nil
}

func (c *InMemoryClient) GetAutogrowPolicies(_ context.Context) ([]*storage.AutogrowPolicyPersistent, error) {
	agPolicies := make([]*storage.AutogrowPolicyPersistent, 0, len(c.autogrowPolicies))
	for _, agPolicy := range c.autogrowPolicies {
		agPolicies = append(agPolicies, agPolicy)
	}
	return agPolicies, nil
}
