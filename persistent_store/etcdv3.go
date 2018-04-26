// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/coreos/etcd/clientv3"
	//TODO: Change for the later versions of etcd (etcd v3.1.5 doesn't return any error for unfound keys but later versions do)
	//"github.com/coreos/etcd/etcdserver"
	conc "github.com/coreos/etcd/clientv3/concurrency"
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
	tlsConfig *tls.Config
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
	if err.Error() == grpc.ErrClientConnTimeout.Error() ||
		strings.Contains(err.Error(), "dial tcp") {
		return nil, NewPersistentStoreError(UnavailableClusterErr, "")
	}
	return nil, err
}

func NewEtcdClientV3WithTLS(endpoints, etcdV3Cert, etcdV3CACert, etcdV3Key string) (*EtcdClientV3, error) {
	//TODO: error handling if a v2 server specified (ErrOldCluster https://godoc.org/github.com/coreos/etcd/clientv3#pkg-variables)
	// Set up etcdv3 client
	tlsCert, err := tls.LoadX509KeyPair(etcdV3Cert, etcdV3Key)
	if err != nil {
		return nil, fmt.Errorf("loading TLS certificate failed: %v", err)
	}
	caCertBytes, err := ioutil.ReadFile(etcdV3CACert)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate failed: %v", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertBytes) {
		return nil, fmt.Errorf("parsing CA certificate failed: %v", err)
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		Certificates:       []tls.Certificate{tlsCert},
		RootCAs:            caCertPool,
	}
	clientV3, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoints}, //TODO: support for multiple IP addresses
		DialTimeout: config.PersistentStoreBootstrapTimeout,
		TLS:         tlsConfig,
	})

	if err == nil {
		return &EtcdClientV3{
			clientV3:  clientV3,
			endpoints: endpoints,
			tlsConfig: tlsConfig,
		}, nil
	}
	if err.Error() == grpc.ErrClientConnTimeout.Error() ||
		strings.Contains(err.Error(), "dial tcp") {
		return nil, NewPersistentStoreError(UnavailableClusterErr, "")
	}
	return nil, err
}

func NewEtcdClientV3FromConfig(etcdConfig *ClientConfig) (*EtcdClientV3, error) {
	//TODO: error handling if a v2 server specified (ErrOldCluster https://godoc.org/github.com/coreos/etcd/clientv3#pkg-variables)
	// Set up etcdv3 client
	clientV3, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcdConfig.endpoints}, //TODO: support for multiple IP addresses
		DialTimeout: config.PersistentStoreBootstrapTimeout,
		TLS:         etcdConfig.TLSConfig,
	})

	if err == nil {
		return &EtcdClientV3{
			clientV3:  clientV3,
			endpoints: etcdConfig.endpoints,
			tlsConfig: etcdConfig.TLSConfig,
		}, nil
	}
	if err.Error() == grpc.ErrClientConnTimeout.Error() ||
		strings.Contains(err.Error(), "dial tcp") {
		return nil, NewPersistentStoreError(UnavailableClusterErr, "")
	}
	return nil, err
}

// Create creates a key in etcd
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

// Create creates a key in etcd using STM
func (p *EtcdClientV3) CreateSTM(s conc.STM, key, value string) error {
	_, err := p.ReadSTM(s, key)
	if err != nil && !MatchKeyNotFoundErr(err) {
		return err
	}
	if err == nil {
		return NewPersistentStoreError(KeyExistsErr, key)
	}
	return p.SetSTM(s, key, value)
}

// Read reads a key from etcd
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
		return "", fmt.Errorf("too many keys were returned")
	}
	// TODO: Validate for the later versions of etcd
	// This version of etcd doesn't return any error for nonexistent keys
	if len(resp.Kvs) == 0 {
		return "", NewPersistentStoreError(KeyNotFoundErr, key)
	}
	return string(resp.Kvs[0].Value[:]), nil
}

// Read reads a key from etcd using STM
func (p *EtcdClientV3) ReadSTM(s conc.STM, key string) (string, error) {
	value := s.Get(key)
	if value == "" {
		return "", NewPersistentStoreError(KeyNotFoundErr, key)
	}

	return value, nil
}

// ReadKeys returns all the keys with the designated prefix
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

func (p *EtcdClientV3) UpdateSTM(s conc.STM, key, value string) error {
	_, err := p.ReadSTM(s, key)
	if err != nil {
		return err
	}
	return p.SetSTM(s, key, value)
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

func (p *EtcdClientV3) SetSTM(s conc.STM, key, value string) error {
	s.Put(key, value)
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

func (p *EtcdClientV3) DeleteSTM(s conc.STM, key string) error {
	s.Del(key)
	return nil
}

// DeleteKeys deletes all the keys with the designated prefix
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

// GetType returns the persistent store type
func (p *EtcdClientV3) GetType() StoreType {
	return EtcdV3Store
}

// Stop shuts down the etcd client
func (p *EtcdClientV3) Stop() error {
	return p.clientV3.Close()
}

// GetConfig returns the configuration for the etcd client
func (p *EtcdClientV3) GetConfig() *ClientConfig {
	return &ClientConfig{
		endpoints: p.endpoints,
		TLSConfig: p.tlsConfig,
	}
}

// GetVersion returns the version of the persistent data
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

// SetVersion sets the version of the persistent data
func (p *EtcdClientV3) SetVersion(version *PersistentStateVersion) error {
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return err
	}
	return p.Set(config.StoreURL, string(versionJSON))
}

// AddBackend saves the minimally required backend state to the persistent store
func (p *EtcdClientV3) AddBackend(b *storage.Backend) error {
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

// AddBackendSTM saves the minimally required backend state to the persistent store using STM
func (p *EtcdClientV3) AddBackendSTM(s conc.STM, b *storage.Backend) error {
	backend := b.ConstructPersistent()
	backendJSON, err := json.Marshal(backend)
	if err != nil {
		return err
	}
	err = p.CreateSTM(s, config.BackendURL+"/"+backend.Name, string(backendJSON))
	if err != nil {
		return err
	}
	return nil
}

// GetBackend retrieves a backend from the persistent store
func (p *EtcdClientV3) GetBackend(backendName string) (*storage.BackendPersistent, error) {
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
func (p *EtcdClientV3) UpdateBackend(b *storage.Backend) error {
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

// DeleteBackend deletes the backend state on the persistent store
func (p *EtcdClientV3) DeleteBackend(backend *storage.Backend) error {
	err := p.Delete(config.BackendURL + "/" + backend.Name)
	if err != nil {
		return err
	}
	return nil
}

// DeleteBackendSTM deletes the backend state on the persistent store using STM
func (p *EtcdClientV3) DeleteBackendSTM(s conc.STM, backend *storage.Backend) error {
	err := p.DeleteSTM(s, config.BackendURL+"/"+backend.Name)
	if err != nil {
		return err
	}
	return nil
}

// GetBackends retrieves all backends
func (p *EtcdClientV3) GetBackends() ([]*storage.BackendPersistent, error) {
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

// ReplaceBackendAndUpdateVolumes replaces a backend and updates all volumes to
// reflect the new backend.
func (p *EtcdClientV3) ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error {
	// It's important to update the persistent store objects in an atomic way.
	_, err := conc.NewSTMSerializable(context.TODO(), p.clientV3,
		func(s conc.STM) error {

			// First, create the new backend.
			err := p.AddBackendSTM(s, newBackend)
			if err != nil {
				return err
			}

			// Second, find all the volumes.
			volExternalList, err := p.GetVolumesSTM(s)
			if err != nil {
				return err
			}

			// Third, update the volumes mapped to the old backend to the new backend.
			for _, volExternal := range volExternalList {
				if volExternal.Backend == origBackend.Name {
					vol := storage.NewVolume(volExternal.Config,
						newBackend.Name, volExternal.Pool, volExternal.Orphaned)
					err = p.UpdateVolumeSTM(s, vol)
					if err != nil {
						return err
					}
				}
			}

			// Forth, delete the old backend.
			return p.DeleteBackendSTM(s, origBackend)
		})
	return err
}

// failedReplaceBackendAndUpdateVolumes simulates a transaction failure in
// replacing a backend and updating all the volumes to reflect the new backend
// state. This method is intended to be used by unit tests only.
func (p *EtcdClientV3) failedReplaceBackendAndUpdateVolumes(
	origBackend, newBackend *storage.Backend) error {
	// It's important to update the persistent store objects in an atomic way.
	_, err := conc.NewSTMSerializable(context.TODO(), p.clientV3,
		func(s conc.STM) error {
			// First, create the new backend.
			err := p.AddBackendSTM(s, newBackend)
			if err != nil {
				return err
			}

			// Second, find all the volumes.
			_, err = p.GetVolumesSTM(s)
			if err != nil {
				return err
			}

			// Third, simulate a failure in updating the volumes or deleting
			// the old backend. Transaction failure should undo the new backend
			// creation.
			return fmt.Errorf("failedReplaceBackendAndUpdateVolumes failed")
		})
	return err
}

// AddVolume saves a volume's state to the persistent store
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

// GetVolume retrieves a volume's state from the persistent store
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

// GetVolumeSTM retrieves a volume's state from the persistent store using STM
func (p *EtcdClientV3) GetVolumeSTM(s conc.STM, volName string) (*storage.VolumeExternal, error) {
	volJSON, err := p.ReadSTM(s, config.VolumeURL+"/"+volName)
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

// UpdateVolumeSTM updates a volume's state on the persistent store using STM
func (p *EtcdClientV3) UpdateVolumeSTM(s conc.STM, vol *storage.Volume) error {
	volExternal := vol.ConstructExternal()
	volJSON, err := json.Marshal(volExternal)
	if err != nil {
		return err
	}
	err = p.UpdateSTM(s, config.VolumeURL+"/"+vol.Config.Name, string(volJSON))
	if err != nil {
		return err
	}
	return nil
}

// DeleteVolume deletes a volume's state from the persistent store
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

// GetVolumes retrieves all volumes
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

// GetVolumesSTM retrieves all volumes using STM
func (p *EtcdClientV3) GetVolumesSTM(s conc.STM) ([]*storage.VolumeExternal, error) {
	volumeList := make([]*storage.VolumeExternal, 0)
	// TODO: The following needs to change once we remove the global lock in the core.
	keys, err := p.ReadKeys(config.VolumeURL)
	if err != nil && MatchKeyNotFoundErr(err) {
		return volumeList, nil
	} else if err != nil {
		return nil, err
	}
	for _, key := range keys {
		vol, err := p.GetVolumeSTM(s,
			strings.TrimPrefix(key, config.VolumeURL+"/"))
		if err != nil {
			return nil, err
		}
		volumeList = append(volumeList, vol)
	}
	return volumeList, nil
}

// DeleteVolumes deletes all volumes
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

// AddVolumeTransaction logs an AddVolume operation
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

// GetVolumeTransactions retrieves AddVolume logs
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
func (p *EtcdClientV3) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	err := p.Delete(config.TransactionURL + "/" + volTxn.getKey())
	if err != nil {
		return err
	}
	return nil
}

func (p *EtcdClientV3) AddStorageClass(sc *storageclass.StorageClass) error {
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

func (p *EtcdClientV3) GetStorageClass(scName string) (*storageclass.Persistent, error) {
	var persistent storageclass.Persistent
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

func (p *EtcdClientV3) GetStorageClasses() ([]*storageclass.Persistent, error) {
	storageClassList := make([]*storageclass.Persistent, 0)
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
func (p *EtcdClientV3) DeleteStorageClass(sc *storageclass.StorageClass) error {
	err := p.Delete(config.StorageClassURL + "/" + sc.GetName())
	if err != nil {
		return err
	}
	return nil
}
