// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/etcd/clientv3"
	//TODO: Change for the later versions of etcd (etcd v3.1.5 doesn't return any error for unfound keys but later versions do)
	//"github.com/coreos/etcd/etcdserver"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

var (
	//TODO: Change for the later versions of etcd (etcd v3.1.5 doesn't return any error unfound keys but later versions do)
	ErrKeyNotFound = errors.New("etcdserver: key not found")
)

type EtcdClientV3 struct {
	clientV3  *clientv3.Client
	endpoints string
}

func NewEtcdClientV3(endpoints string) (*EtcdClientV3, error) {
	//TODO: error handling if a v2 server specified (ErrOldCluster https://godoc.org/github.com/coreos/etcd/clientv3#pkg-variables)
	// Set up etcdv3 client
	clientV3, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoints}, //TODO: support for multiple IP addresses
		DialTimeout: config.PersistentStoreBootstrapTimeout,
	})

	if err == nil {
		return &EtcdClientV3{
			clientV3:  clientV3,
			endpoints: endpoints,
		}, nil
	}
	if err.Error() == grpc.ErrClientConnTimeout.Error() {
		return nil, NewPersistentStoreError(UnavailableClusterErr, "")
	}
	return nil, err
}

// the abstract CRUD interface
func (p *EtcdClientV3) Create(key, value string) error {
	_, err := p.Read(key)
	if err != nil && !MatchKeyNotFoundErr(err) {
		return err
	}
	if err == nil {
		return NewPersistentStoreError(KeyExistsErr, key)
	}
	return p.Set(key, value)
}

func (p *EtcdClientV3) Read(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.clientV3.Get(ctx, key)
	cancel()
	if err != nil {
		//TODO: Change for the later versions of etcd
		if err == ErrKeyNotFound {
			return "", NewPersistentStoreError(KeyNotFoundErr, key)
		}
		return "", err
	}
	if len(resp.Kvs) > 1 {
		return "", fmt.Errorf("Too many keys were returned!")
	}
	// TODO: Validate for the later versions of etcd
	// This version of etcd doesn't return any error for nonexistent keys
	if len(resp.Kvs) == 0 {
		return "", NewPersistentStoreError(KeyNotFoundErr, key)
	}
	return string(resp.Kvs[0].Value[:]), nil
}

// This method returns all the keys with the designated prefix
func (p *EtcdClientV3) ReadKeys(keyPrefix string) ([]string, error) {
	keys := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.clientV3.Get(ctx, keyPrefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		clientv3.WithKeysOnly())
	cancel()
	if err != nil {
		//TODO: Change for the later versions of etcd (etcd v3.1.5 doesn't return any error for unfound keys but later versions do)
		if MatchKeyNotFoundErr(err) {
			return keys, NewPersistentStoreError(KeyNotFoundErr, keyPrefix)
		}
		return keys, err
	}
	for _, key := range resp.Kvs {
		keys = append(keys, string(key.Key[:]))
	}
	if len(keys) == 0 {
		return keys, NewPersistentStoreError(KeyNotFoundErr, keyPrefix)
	}
	return keys, nil
}

func (p *EtcdClientV3) Update(key, value string) error {
	_, err := p.Read(key)
	if err != nil {
		return err
	}
	return p.Set(key, value)
}

func (p *EtcdClientV3) Set(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.clientV3.Put(ctx, key, value)
	cancel()
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV3) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.clientV3.Delete(ctx, key)
	cancel()
	if err != nil {
		return err
	}
	if err == nil && resp.Deleted == 0 {
		return NewPersistentStoreError(KeyNotFoundErr, key)
	}
	return nil
}

// This method deletes all the keys with the designated prefix
func (p *EtcdClientV3) DeleteKeys(keyPrefix string) error {
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

// Returns the persistent store type
func (p *EtcdClientV3) GetType() StoreType {
	return EtcdV3Store
}

// Shuts down the etcd client
func (p *EtcdClientV3) Stop() error {
	return p.clientV3.Close()
}

// Returns etcd endpoints
func (p *EtcdClientV3) GetEndpoints() string {
	return p.endpoints
}

// Returns the version of the persistent data
func (p *EtcdClientV3) GetVersion() (*PersistentStateVersion, error) {
	versionJSON, err := p.Read(config.StoreURL)
	if err != nil {
		return nil, err
	}
	version := &PersistentStateVersion{}
	err = json.Unmarshal([]byte(versionJSON), version)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// Sets the version of the persistent data
func (p *EtcdClientV3) SetVersion(version *PersistentStateVersion) error {
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return err
	}
	if err = p.Set(config.StoreURL, string(versionJSON)); err != nil {
		return err
	}
	return nil
}

// This method saves the minimally required backend state to the persistent store
func (p *EtcdClientV3) AddBackend(b *storage.StorageBackend) error {
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

// This method retrieves a backend from the persistent store
func (p *EtcdClientV3) GetBackend(backendName string) (*storage.StorageBackendPersistent, error) {
	var backend storage.StorageBackendPersistent
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

// This method updates the backend state on the persistent store
func (p *EtcdClientV3) UpdateBackend(b *storage.StorageBackend) error {
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

// This method deletes the backend state on the persistent store
func (p *EtcdClientV3) DeleteBackend(backend *storage.StorageBackend) error {
	err := p.Delete(config.BackendURL + "/" + backend.Name)
	if err != nil {
		return err
	}
	return nil
}

// This method retrieves all backends
func (p *EtcdClientV3) GetBackends() ([]*storage.StorageBackendPersistent, error) {
	backendList := make([]*storage.StorageBackendPersistent, 0)
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

// This method deletes all backends
func (p *EtcdClientV3) DeleteBackends() error {
	backends, err := p.ReadKeys(config.BackendURL)
	if err != nil {
		return err
	}
	for _, backend := range backends {
		if err = p.Delete(backend); err != nil {
			return err
		}
	}
	return nil
}

// This method saves a volume's state to the persistent store
func (p *EtcdClientV3) AddVolume(vol *storage.Volume) error {
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

// This method retrieves a volume's state from the persistent store
func (p *EtcdClientV3) GetVolume(volName string) (*storage.VolumeExternal, error) {
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

// This method updates a volume's state on the persistent store
func (p *EtcdClientV3) UpdateVolume(vol *storage.Volume) error {
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

// This method deletes a volume's state from the persistent store
func (p *EtcdClientV3) DeleteVolume(vol *storage.Volume) error {
	err := p.Delete(config.VolumeURL + "/" + vol.Config.Name)
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV3) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	err := p.DeleteVolume(vol)
	if err != nil && MatchKeyNotFoundErr(err) {
		return nil
	}
	return err
}

// This method retrieves all volumes
func (p *EtcdClientV3) GetVolumes() ([]*storage.VolumeExternal, error) {
	volumeList := make([]*storage.VolumeExternal, 0)
	keys, err := p.ReadKeys(config.VolumeURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return volumeList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		vol, err := p.GetVolume(strings.TrimPrefix(key, config.VolumeURL+"/"))
		if err != nil {
			return nil, err
		}
		volumeList = append(volumeList, vol)
	}
	return volumeList, nil
}

// This method deletes all volumes
func (p *EtcdClientV3) DeleteVolumes() error {
	volumes, err := p.ReadKeys(config.VolumeURL)
	if err != nil {
		return err
	}
	for _, vol := range volumes {
		if err = p.Delete(vol); err != nil {
			return err
		}
	}
	return nil
}

// This method logs an AddVolume operation
func (p *EtcdClientV3) AddVolumeTransaction(volTxn *VolumeTransaction) error {
	volTxnJSON, err := json.Marshal(volTxn)
	if err != nil {
		return err
	}
	err = p.Set(config.TransactionURL+"/"+volTxn.getKey(),
		string(volTxnJSON))
	if err != nil {
		return err
	}
	return nil
}

// This method retrieves AddVolume logs
func (p *EtcdClientV3) GetVolumeTransactions() ([]*VolumeTransaction, error) {
	volTxnList := make([]*VolumeTransaction, 0)
	keys, err := p.ReadKeys(config.TransactionURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return volTxnList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		volTxn := &VolumeTransaction{}
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

// GetExistingVolumeTransaction returns an existing version of the current
// volume transaction, if it exists.  If no volume transaction with the same
// key exists, it returns nil.
func (p *EtcdClientV3) GetExistingVolumeTransaction(
	volTxn *VolumeTransaction,
) (*VolumeTransaction, error) {
	var ret VolumeTransaction

	key := volTxn.getKey()
	txnJSON, err := p.Read(config.TransactionURL + "/" + key)
	if err != nil {
		if !MatchKeyNotFoundErr(err) {
			return nil, fmt.Errorf("Unable to read volume transaction key %s "+
				"from etcd: %v", key, err)
		} else {
			return nil, nil
		}
	}
	if err = json.Unmarshal([]byte(txnJSON), &ret); err != nil {
		return nil, fmt.Errorf("Unable to unmarshal volume transaction JSON "+
			"for %s: %v", key, err)
	}
	return &ret, nil
}

// This method deletes an AddVolume log
func (p *EtcdClientV3) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	err := p.Delete(config.TransactionURL + "/" + volTxn.getKey())
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV3) AddStorageClass(sc *storage_class.StorageClass) error {
	storageClass := sc.ConstructPersistent()
	storageClassJSON, err := json.Marshal(storageClass)
	if err != nil {
		return err
	}
	err = p.Create(config.StorageClassURL+"/"+storageClass.GetName(),
		string(storageClassJSON))
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV3) GetStorageClass(scName string) (*storage_class.StorageClassPersistent, error) {
	var persistent storage_class.StorageClassPersistent
	scJSON, err := p.Read(config.StorageClassURL + "/" + scName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(scJSON), &persistent)
	if err != nil {
		return nil, err
	}
	return &persistent, nil
}

func (p *EtcdClientV3) GetStorageClasses() ([]*storage_class.StorageClassPersistent, error) {
	storageClassList := make([]*storage_class.StorageClassPersistent, 0)
	keys, err := p.ReadKeys(config.StorageClassURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return storageClassList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		sc, err := p.GetStorageClass(strings.TrimPrefix(key,
			config.StorageClassURL+"/"))
		if err != nil {
			return nil, err
		}
		storageClassList = append(storageClassList, sc)
	}
	return storageClassList, nil
}

// DeleteStorageClass deletes a storage class's state from the persistent store
func (p *EtcdClientV3) DeleteStorageClass(sc *storage_class.StorageClass) error {
	err := p.Delete(config.StorageClassURL + "/" + sc.GetName())
	if err != nil {
		return err
	}
	return nil
}
