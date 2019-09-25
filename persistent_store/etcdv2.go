// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	etcdclientv2 "github.com/coreos/etcd/client"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type EtcdClientV2 struct {
	clientV2  *etcdclientv2.Client
	keysAPI   etcdclientv2.KeysAPI
	endpoints string
}

func NewEtcdClientV2(endpoints string) (*EtcdClientV2, error) {
	cfg := etcdclientv2.Config{
		Endpoints: []string{endpoints}, //TODO: support for multiple IP addresses
	}
	c, err := etcdclientv2.New(cfg)
	if err != nil {
		return nil, err
	}
	keysAPI := etcdclientv2.NewKeysAPI(c)

	// Making sure the etcd server is up
	for tries := 0; tries <= config.PersistentStoreBootstrapAttempts; tries++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)

		_, err := keysAPI.Get(ctx, "/trident", &etcdclientv2.GetOptions{Recursive: true, Sort: true, Quorum: true})
		cancel()
		if err == nil {
			// etcd is working
			if tries > 0 {
				log.Infof("Persistent store is up after %d second(s).", tries)
			}
			break
		} else if strings.Contains(err.Error(), etcdclientv2.ErrClusterUnavailable.Error()) && tries > 0 {
			log.Warnf("etcd not yet online (attempt #%v).", tries)
			time.Sleep(time.Second)
		} else if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			// etcd is working
			if tries > 0 {
				log.Infof("Persistent store is up after %d second(s).", tries)
			}
			break
		}
		if tries == config.PersistentStoreBootstrapAttempts {
			log.Warnf("Persistent store failed to come online after %d seconds.", tries)
			return nil, NewPersistentStoreError(UnavailableClusterErr, "/trident")
		}
	}

	client := &EtcdClientV2{
		clientV2:  &c,
		keysAPI:   keysAPI,
		endpoints: endpoints,
	}

	// Warn if etcd version is not what we expect
	client.checkEtcdVersion()

	return client, nil
}

func NewEtcdClientV2FromConfig(etcdConfig *ClientConfig) (*EtcdClientV2, error) {
	return NewEtcdClientV2(etcdConfig.endpoints)
}

func (p *EtcdClientV2) checkEtcdVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := *p.clientV2
	version, err := client.GetVersion(ctx)
	if err != nil {
		log.Errorf("Could not get etcd version: %v", err)
		return
	}

	buildEtcdVersion := utils.MustParseSemantic(config.BuildEtcdVersion)

	if comparison, err := buildEtcdVersion.Compare(version.Server); err != nil {
		log.Errorf("Could not parse etcd version '%s': %v", version.Server, err)
	} else if comparison != 0 {
		log.Warningf("Trident was qualified with etcd %s. You appear to be using a different version.", config.BuildEtcdVersion)
	} else {
		log.WithFields(log.Fields{
			"etcdVersion": version.Server,
		}).Debug("The detected etcd version matches the version Trident is tested with.")
	}
}

// Create is the abstract CRUD interface
func (p *EtcdClientV2) Create(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Create(ctx, key, value)
	cancel()
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV2) Read(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.keysAPI.Get(ctx, key, &etcdclientv2.GetOptions{Recursive: true, Sort: true, Quorum: true})
	cancel()
	if err != nil {
		if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			return "", NewPersistentStoreError(KeyNotFoundErr, key)
		}
		return "", err
	}
	return resp.Node.Value, nil
}

// ReadKeys returns all the keys with the designated prefix
func (p *EtcdClientV2) ReadKeys(keyPrefix string) ([]string, error) {
	keys := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.keysAPI.Get(ctx, keyPrefix, &etcdclientv2.GetOptions{Recursive: true, Sort: true, Quorum: true})
	cancel()
	if err != nil {
		if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			err = NewPersistentStoreError(KeyNotFoundErr, keyPrefix)
		}
		return keys, err
	}
	if !resp.Node.Dir {
		return keys, fmt.Errorf("etcdv2 requires a directory prefix")
	}
	for _, node := range resp.Node.Nodes {
		if node.Dir {
			childKeys, err := p.ReadKeys(node.Key)
			if err != nil && MatchKeyNotFoundErr(err) {
				continue
			} else if err != nil {
				return keys, err
			}
			keys = append(keys, childKeys...)
		} else {
			keys = append(keys, node.Key)
		}
	}
	if len(keys) == 0 {
		return keys, NewPersistentStoreError(KeyNotFoundErr, keyPrefix)
	}
	return keys, nil
}

func (p *EtcdClientV2) Update(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Update(ctx, key, value)
	cancel()
	if err != nil {
		if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			return NewPersistentStoreError(KeyNotFoundErr, key)
		}
		return err
	}
	return nil
}

func (p *EtcdClientV2) Set(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Set(ctx, key, value, &etcdclientv2.SetOptions{})
	cancel()
	if err != nil {
		if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			return NewPersistentStoreError(KeyNotFoundErr, key)
		}
		return err
	}
	return nil
}

func (p *EtcdClientV2) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Delete(ctx, key, &etcdclientv2.DeleteOptions{Recursive: true})
	cancel()
	if err != nil {
		if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			return NewPersistentStoreError(KeyNotFoundErr, key)
		}
		return err
	}
	return nil
}

// DeleteKeys deletes all the keys with the designated prefix
func (p *EtcdClientV2) DeleteKeys(keyPrefix string) error {
	return p.deleteKeys(keyPrefix)
}

// deleteKeys deletes all the keys with the designated prefix
func (p *EtcdClientV2) deleteKeys(keyPrefix string) error {
	keys, err := p.ReadKeys(keyPrefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err = p.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

// GetType returns the persistent store type
func (p *EtcdClientV2) GetType() StoreType {
	return EtcdV2Store
}

// Stop shuts down the etcd client
func (p *EtcdClientV2) Stop() error {
	return nil
}

// GetConfig returns the configuration for the etcd client
func (p *EtcdClientV2) GetConfig() *ClientConfig {
	return &ClientConfig{
		endpoints: p.endpoints,
	}
}

// GetVersion returns the version of the persistent data
func (p *EtcdClientV2) GetVersion() (*config.PersistentStateVersion, error) {
	versionJSON, err := p.Read(config.StoreURL)
	if err != nil {
		return nil, err
	}
	version := &config.PersistentStateVersion{}
	err = json.Unmarshal([]byte(versionJSON), version)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// SetVersion sets the version of the persistent data
func (p *EtcdClientV2) SetVersion(version *config.PersistentStateVersion) error {
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return err
	}
	return p.Set(config.StoreURL, string(versionJSON))
}

// AddBackend saves the minimally required backend state to the persistent store
func (p *EtcdClientV2) AddBackend(b *storage.Backend) error {
	backend := b.ConstructPersistent()
	backendJSON, err := json.Marshal(backend)
	if err != nil {
		return err
	}
	err = p.Create(config.BackendURL+"/"+backend.Name, string(backendJSON))
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV2) AddBackendPersistent(backend *storage.BackendPersistent) error {
	return NewPersistentStoreError(NotSupported, "")
}

// GetBackend retrieves a backend from the persistent store
func (p *EtcdClientV2) GetBackend(backendName string) (*storage.BackendPersistent, error) {
	var backend storage.BackendPersistent
	backendJSON, err := p.Read(config.BackendURL + "/" + backendName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(backendJSON), &backend)
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

// UpdateBackend updates the backend state on the persistent store
func (p *EtcdClientV2) UpdateBackend(b *storage.Backend) error {
	backend := b.ConstructPersistent()
	backendJSON, err := json.Marshal(backend)
	if err != nil {
		return err
	}
	err = p.Update(config.BackendURL+"/"+backend.Name, string(backendJSON))
	if err != nil {
		return err
	}
	return nil
}

// UpdateBackendPersistent updates a backend's persistent state
func (p *EtcdClientV2) UpdateBackendPersistent(update *storage.BackendPersistent) error {
	return NewPersistentStoreError(NotSupported, "")
}

// DeleteBackend deletes the backend state on the persistent store
func (p *EtcdClientV2) DeleteBackend(backend *storage.Backend) error {
	err := p.Delete(config.BackendURL + "/" + backend.Name)
	if err != nil {
		return err
	}
	return nil
}

// ReplaceBackendAndUpdateVolumes renames a backend and updates all volumes to
// reflect the new backend name
func (p *EtcdClientV2) ReplaceBackendAndUpdateVolumes(
	origBackend, newBackend *storage.Backend) error {
	// Because etcdv2 doesn't support atomic transactions, this method
	// returns an error.
	return NewPersistentStoreError(NotSupported, "")
}

// GetBackends retrieves all backends
func (p *EtcdClientV2) GetBackends() ([]*storage.BackendPersistent, error) {
	backendList := make([]*storage.BackendPersistent, 0)
	keys, err := p.ReadKeys(config.BackendURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return backendList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		backend, err := p.GetBackend(strings.TrimPrefix(key, config.BackendURL+"/"))
		if err != nil {
			return nil, err
		}
		backendList = append(backendList, backend)
	}
	return backendList, nil
}

// DeleteBackends deletes all backends
func (p *EtcdClientV2) DeleteBackends() error {
	return p.deleteKeys(config.BackendURL)
}

// AddVolume saves a volume's state to the persistent store
func (p *EtcdClientV2) AddVolume(vol *storage.Volume) error {
	volExternal := vol.ConstructExternal()
	volJSON, err := json.Marshal(volExternal)
	if err != nil {
		return err
	}
	err = p.Create(config.VolumeURL+"/"+vol.Config.Name, string(volJSON))
	if err != nil {
		return err
	}
	return nil
}

// AddVolumePersistent saves a volume's persistent state to the persistent store
func (p *EtcdClientV2) AddVolumePersistent(volume *storage.VolumeExternal) error {
	return NewPersistentStoreError(NotSupported, "")
}

// UpdateVolumePersistent updates a volume's persistent state
func (p *EtcdClientV2) UpdateVolumePersistent(volume *storage.VolumeExternal) error {
	return NewPersistentStoreError(NotSupported, "")
}

// GetVolume retrieves a volume's state from the persistent store
func (p *EtcdClientV2) GetVolume(volName string) (*storage.VolumeExternal, error) {
	volJSON, err := p.Read(config.VolumeURL + "/" + volName)
	if err != nil {
		return nil, err
	}
	volExternal := &storage.VolumeExternal{}
	err = json.Unmarshal([]byte(volJSON), volExternal)
	if err != nil {
		return nil, err
	}
	return volExternal, nil
}

// UpdateVolume updates a volume's state on the persistent store
func (p *EtcdClientV2) UpdateVolume(vol *storage.Volume) error {
	volExternal := vol.ConstructExternal()
	volJSON, err := json.Marshal(volExternal)
	if err != nil {
		return err
	}
	err = p.Update(config.VolumeURL+"/"+vol.Config.Name, string(volJSON))
	if err != nil {
		return err
	}
	return nil
}

// DeleteVolume deletes a volume's state from the persistent store
func (p *EtcdClientV2) DeleteVolume(vol *storage.Volume) error {
	err := p.Delete(config.VolumeURL + "/" + vol.Config.Name)
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV2) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	err := p.DeleteVolume(vol)
	if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
		return nil
	}
	return err
}

// GetVolumes retrieves all volumes
func (p *EtcdClientV2) GetVolumes() ([]*storage.VolumeExternal, error) {
	volumeList := make([]*storage.VolumeExternal, 0)
	keys, err := p.ReadKeys(config.VolumeURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return volumeList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		vol, err := p.GetVolume(strings.TrimPrefix(key, config.VolumeURL))
		if err != nil {
			return nil, err
		}
		volumeList = append(volumeList, vol)
	}
	return volumeList, nil
}

// DeleteVolumes deletes all volumes
func (p *EtcdClientV2) DeleteVolumes() error {
	return p.deleteKeys(config.VolumeURL)
}

// AddVolumeTransaction logs an AddVolume operation
func (p *EtcdClientV2) AddVolumeTransaction(volTxn *storage.VolumeTransaction) error {
	if volTxnJSON, err := json.Marshal(volTxn); err != nil {
		return err
	} else {
		return p.Set(config.TransactionURL+"/"+volTxn.Name(), string(volTxnJSON))
	}
}

// GetVolumeTransactions retrieves AddVolume logs
func (p *EtcdClientV2) GetVolumeTransactions() ([]*storage.VolumeTransaction, error) {
	volTxnList := make([]*storage.VolumeTransaction, 0)
	keys, err := p.ReadKeys(config.TransactionURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return volTxnList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		volTxn := &storage.VolumeTransaction{}
		volTxnJSON, err := p.Read(key)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal([]byte(volTxnJSON), volTxn)
		if err != nil {
			return nil, err
		}
		volTxnList = append(volTxnList, volTxn)
	}
	return volTxnList, nil
}

func (p *EtcdClientV2) UpdateVolumeTransaction(volTxn *storage.VolumeTransaction) error {
	if volTxnJSON, err := json.Marshal(volTxn); err != nil {
		return err
	} else {
		return p.Update(config.TransactionURL+"/"+volTxn.Name(), string(volTxnJSON))
	}
}

// GetExistingVolumeTransaction returns an existing version of the current
// volume transaction, if it exists.  If no volume transaction with the same
// key exists, it returns nil.
func (p *EtcdClientV2) GetExistingVolumeTransaction(
	volTxn *storage.VolumeTransaction,
) (*storage.VolumeTransaction, error) {
	var ret storage.VolumeTransaction

	key := volTxn.Name()
	txnJSON, err := p.Read(config.TransactionURL + "/" + key)
	if err != nil {
		if !MatchKeyNotFoundErr(err) {
			return nil, fmt.Errorf("unable to read volume transaction key %s from etcd: %v", key, err)
		} else {
			return nil, nil
		}
	}
	if err = json.Unmarshal([]byte(txnJSON), &ret); err != nil {
		return nil, fmt.Errorf("unable to unmarshal volume transaction JSON for %s: %v", key, err)
	}
	return &ret, nil
}

// DeleteVolumeTransaction deletes an AddVolume log
func (p *EtcdClientV2) DeleteVolumeTransaction(volTxn *storage.VolumeTransaction) error {
	err := p.Delete(config.TransactionURL + "/" + volTxn.Name())
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV2) AddStorageClass(sc *storageclass.StorageClass) error {
	sClass := sc.ConstructPersistent()
	storageClassJSON, err := json.Marshal(sClass)
	if err != nil {
		return err
	}
	err = p.Create(config.StorageClassURL+"/"+sClass.GetName(),
		string(storageClassJSON))
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV2) GetStorageClass(scName string) (*storageclass.Persistent, error) {
	var sc storageclass.Persistent
	scJSON, err := p.Read(config.StorageClassURL + "/" + scName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(scJSON), &sc)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (p *EtcdClientV2) GetStorageClasses() ([]*storageclass.Persistent, error) {
	storageClassList := make([]*storageclass.Persistent, 0)
	keys, err := p.ReadKeys(config.StorageClassURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return storageClassList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		sc, err := p.GetStorageClass(strings.TrimPrefix(key, config.StorageClassURL+"/"))
		if err != nil {
			return nil, err
		}
		storageClassList = append(storageClassList, sc)
	}
	return storageClassList, nil
}

// DeleteStorageClass deletes a storage class's state from the persistent store
func (p *EtcdClientV2) DeleteStorageClass(sc *storageclass.StorageClass) error {
	err := p.Delete(config.StorageClassURL + "/" + sc.GetName())
	if err != nil {
		return err
	}
	return nil
}

// AddOrUpdateNode adds a CSI node object to the persistent store
func (p *EtcdClientV2) AddOrUpdateNode(n *utils.Node) error {
	nodeJSON, err := json.Marshal(n)
	if err != nil {
		return err
	}
	err = p.Set(config.NodeURL+"/"+n.Name, string(nodeJSON))
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV2) GetNode(nName string) (*utils.Node, error) {
	var node utils.Node
	nodeJSON, err := p.Read(config.NodeURL + "/" + nName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(nodeJSON), &node)
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func (p *EtcdClientV2) GetNodes() ([]*utils.Node, error) {
	nodeList := make([]*utils.Node, 0)
	keys, err := p.ReadKeys(config.NodeURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return nodeList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		node, err := p.GetNode(strings.TrimPrefix(key, config.NodeURL+"/"))
		if err != nil {
			return nil, err
		}
		nodeList = append(nodeList, node)
	}
	return nodeList, nil
}

// DeleteNode deletes a node from the persistent store
func (p *EtcdClientV2) DeleteNode(n *utils.Node) error {
	err := p.Delete(config.NodeURL + "/" + n.Name)
	if err != nil {
		return err
	}
	return nil
}

// AddSnapshot adds a snapshot's state to the persistent store
func (p *EtcdClientV2) AddSnapshot(snapshot *storage.Snapshot) error {
	if snapJSON, err := json.Marshal(snapshot.ConstructPersistent()); err != nil {
		return err
	} else {
		return p.Create(config.SnapshotURL+"/"+snapshot.ID(), string(snapJSON))
	}
}

// GetSnapshot fetches a snapshot's state from the persistent store
func (p *EtcdClientV2) GetSnapshot(volumeName, snapshotName string) (*storage.SnapshotPersistent, error) {
	return p.getSnapshotByID(storage.MakeSnapshotID(volumeName, snapshotName))
}

// getSnapshotByID fetches a snapshot's state from the persistent store given its ID
func (p *EtcdClientV2) getSnapshotByID(snapshotID string) (*storage.SnapshotPersistent, error) {
	snapJSON, err := p.Read(config.SnapshotURL + "/" + snapshotID)
	if err != nil {
		return nil, err
	}
	snapPersistent := &storage.SnapshotPersistent{}
	if err = json.Unmarshal([]byte(snapJSON), snapPersistent); err != nil {
		return nil, err
	}
	return snapPersistent, nil
}

// GetSnapshots retrieves all snapshots
func (p *EtcdClientV2) GetSnapshots() ([]*storage.SnapshotPersistent, error) {
	snapshotList := make([]*storage.SnapshotPersistent, 0)
	keys, err := p.ReadKeys(config.SnapshotURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return snapshotList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		snapshot, err := p.getSnapshotByID(strings.TrimPrefix(key, config.SnapshotURL+"/"))
		if err != nil {
			return nil, err
		}
		snapshotList = append(snapshotList, snapshot)
	}
	return snapshotList, nil
}

// DeleteSnapshot deletes a snapshot from the persistent store
func (p *EtcdClientV2) DeleteSnapshot(snapshot *storage.Snapshot) error {
	return p.Delete(config.SnapshotURL + "/" + snapshot.ID())
}

// DeleteVolumeIgnoreNotFound deletes a snapshot from the persistent store,
// returning no error if the record does not exist.
func (p *EtcdClientV2) DeleteSnapshotIgnoreNotFound(snapshot *storage.Snapshot) error {
	err := p.DeleteSnapshot(snapshot)
	if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
		return nil
	}
	return err
}

// DeleteSnapshots deletes all snapshots
func (p *EtcdClientV2) DeleteSnapshots() error {
	return p.deleteKeys(config.SnapshotURL)
}
