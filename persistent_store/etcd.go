// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"

	etcdclientv2 "github.com/coreos/etcd/client"
	"golang.org/x/net/context"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type EtcdClient struct {
	clientV2 *etcdclientv2.Client
	keysAPI  etcdclientv2.KeysAPI
}

func NewEtcdClient(etcdIP string) (*EtcdClient, error) {
	cfg := etcdclientv2.Config{
		Endpoints: []string{etcdIP}, //TODO: support for an etcd cluster
	}
	c, err := etcdclientv2.New(cfg)
	if err != nil {
		return nil, err
	}
	return &EtcdClient{
		clientV2: &c,
		keysAPI:  etcdclientv2.NewKeysAPI(c),
	}, nil
}

// the abstract CRUD interface
func (p *EtcdClient) Create(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Create(ctx, key, value)
	cancel()
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClient) Read(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.keysAPI.Get(ctx, key, &etcdclientv2.GetOptions{true, true, true})
	cancel()
	if err != nil {
		if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			return "", KeyError{Key: key}
		}
		return "", err
	}
	return resp.Node.Value, nil
}

// This method returns all the keys with the designated prefix
func (p *EtcdClient) ReadKeys(keyPrefix string) ([]string, error) {
	keys := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	resp, err := p.keysAPI.Get(ctx, keyPrefix, &etcdclientv2.GetOptions{true, true, true})
	cancel()
	if err != nil {
		if err.Error() == etcdclientv2.ErrClusterUnavailable.Error() {
			log.Warn("etcd not yet online; waiting.")
			// This is somewhat sleazy, but we need a way
			// for OrchestratorCore to know that the error is likely to
			// be transient during the bootstrap phase.
			err = context.DeadlineExceeded
		} else if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
			log.Info("Unable to find key ", keyPrefix)
			err = KeyError{Key: keyPrefix}
		}
		return keys, err
	}
	if !resp.Node.Dir {
		return keys, fmt.Errorf("etcd v2 requires a directory prefix!")
	}
	for _, node := range resp.Node.Nodes {
		keys = append(keys, node.Key)
	}
	return keys, nil
}

func (p *EtcdClient) Update(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Update(ctx, key, value)
	cancel()
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClient) Set(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Set(ctx, key, value, &etcdclientv2.SetOptions{})
	cancel()
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClient) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.PersistentStoreTimeout)
	_, err := p.keysAPI.Delete(ctx, key, &etcdclientv2.DeleteOptions{Recursive: true})
	cancel()
	if err != nil {
		return err
	}
	return nil
}

// This method saves the minimally required backend state to the persistent store
func (p *EtcdClient) AddBackend(b *storage.StorageBackend) error {
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
func (p *EtcdClient) GetBackend(backendName string) (*storage.StorageBackendPersistent, error) {
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
func (p *EtcdClient) UpdateBackend(b *storage.StorageBackend) error {
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
func (p *EtcdClient) DeleteBackend(backend *storage.StorageBackend) error {
	err := p.Delete(config.BackendURL + "/" + backend.Name)
	if err != nil {
		return err
	}
	return nil
}

// This method retrieves all backends
func (p *EtcdClient) GetBackends() ([]*storage.StorageBackendPersistent, error) {
	backendList := make([]*storage.StorageBackendPersistent, 0)
	keys, err := p.ReadKeys(config.BackendURL)
	if err != nil {
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
func (p *EtcdClient) DeleteBackends() error {
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
func (p *EtcdClient) AddVolume(vol *storage.Volume) error {
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
func (p *EtcdClient) GetVolume(volName string) (*storage.VolumeExternal, error) {
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
func (p *EtcdClient) UpdateVolume(vol *storage.Volume) error {
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
func (p *EtcdClient) DeleteVolume(vol *storage.Volume) error {
	err := p.Delete(config.VolumeURL + "/" + vol.Config.Name)
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClient) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	err := p.DeleteVolume(vol)
	if etcdErr, ok := err.(etcdclientv2.Error); ok && etcdErr.Code == etcdclientv2.ErrorCodeKeyNotFound {
		return nil
	}
	return err
}

// This method retrieves all volumes
func (p *EtcdClient) GetVolumes() ([]*storage.VolumeExternal, error) {
	volumeList := make([]*storage.VolumeExternal, 0)
	keys, err := p.ReadKeys(config.VolumeURL)
	if err != nil {
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

// This method deletes all volumes
func (p *EtcdClient) DeleteVolumes() error {
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
func (p *EtcdClient) AddVolumeTransaction(volTxn *VolumeTransaction) error {
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
func (p *EtcdClient) GetVolumeTransactions() ([]*VolumeTransaction, error) {
	volTxnList := make([]*VolumeTransaction, 0)
	keys, err := p.ReadKeys(config.TransactionURL)
	if err != nil {
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
func (p *EtcdClient) GetExistingVolumeTransaction(
	volTxn *VolumeTransaction,
) (*VolumeTransaction, error) {
	var ret VolumeTransaction

	key := volTxn.getKey()
	txnJSON, err := p.Read(config.TransactionURL + "/" + key)
	if err != nil {
		if err.Error() != KeyErrorMsg {
			return nil, fmt.Errorf("Unable to read volume transaction key %s "+
				"from etcd:  %v ", key, err)
		} else {
			return nil, nil
		}
	}
	if err = json.Unmarshal([]byte(txnJSON), &ret); err != nil {
		return nil, fmt.Errorf("Unable to unmarshal volume transaction JSON "+
			"for %s:  %v", key, err)
	}
	return &ret, nil
}

// This method deletes an AddVolume log
func (p *EtcdClient) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	err := p.Delete(config.TransactionURL + "/" + volTxn.getKey())
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClient) AddStorageClass(sc *storage_class.StorageClass) error {
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

func (p *EtcdClient) GetStorageClass(scName string) (*storage_class.StorageClassPersistent, error) {
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

func (p *EtcdClient) GetStorageClasses() ([]*storage_class.StorageClassPersistent, error) {
	keys, err := p.ReadKeys(config.StorageClassURL)
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"method": "GetStorageClasses",
	}).Debugf("Got %d keys:  %s", len(keys), strings.Join(keys, ","))
	ret := make([]*storage_class.StorageClassPersistent, 0, len(keys))
	for _, key := range keys {
		sc, err := p.GetStorageClass(strings.TrimPrefix(key,
			config.StorageClassURL+"/"))
		if err != nil {
			return nil, err
		}
		ret = append(ret, sc)
	}
	log.WithFields(log.Fields{
		"method": "GetStorageClasses",
	}).Debugf("Returning %d storage classes.", len(ret))
	return ret, nil
}

// DeleteStorageClass deletes a storage class's state from the persistent store
func (p *EtcdClient) DeleteStorageClass(sc *storage_class.StorageClass) error {
	err := p.Delete(config.StorageClassURL + "/" + sc.GetName())
	if err != nil {
		return err
	}
	return nil
}
