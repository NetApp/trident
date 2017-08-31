// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type StoreType string

const (
	MemoryStore StoreType = "memory"
	EtcdV2Store StoreType = "etcdv2"
	EtcdV3Store StoreType = "etcdv3"
)

type PersistentStateVersion struct {
	PersistentStoreVersion string `json:"store_version"`
	OrchestratorVersion    string `json:"orchestration_version"`
}

type EtcdClient interface {
	Create(key, value string) error
	Read(key string) (string, error)
	ReadKeys(keyPrefix string) ([]string, error)
	Update(key, value string) error
	Set(key, value string) error
	Delete(key string) error
	DeleteKeys(keyPrefix string) error
	GetType() StoreType
	Stop() error
}

type Client interface {
	EtcdClient

	GetEndpoints() string
	GetVersion() (*PersistentStateVersion, error)
	SetVersion(version *PersistentStateVersion) error

	AddBackend(b *storage.StorageBackend) error
	GetBackend(backendName string) (*storage.StorageBackendPersistent, error)
	UpdateBackend(b *storage.StorageBackend) error
	DeleteBackend(backend *storage.StorageBackend) error
	GetBackends() ([]*storage.StorageBackendPersistent, error)
	DeleteBackends() error

	AddVolume(vol *storage.Volume) error
	GetVolume(volName string) (*storage.VolumeExternal, error)
	UpdateVolume(vol *storage.Volume) error
	DeleteVolume(vol *storage.Volume) error
	DeleteVolumeIgnoreNotFound(vol *storage.Volume) error
	GetVolumes() ([]*storage.VolumeExternal, error)
	DeleteVolumes() error

	AddVolumeTransaction(volTxn *VolumeTransaction) error
	GetVolumeTransactions() ([]*VolumeTransaction, error)
	GetExistingVolumeTransaction(volTxn *VolumeTransaction) (*VolumeTransaction,
		error)
	DeleteVolumeTransaction(volTxn *VolumeTransaction) error

	AddStorageClass(sc *storage_class.StorageClass) error
	GetStorageClass(scName string) (*storage_class.StorageClassPersistent, error)
	GetStorageClasses() ([]*storage_class.StorageClassPersistent, error)
	DeleteStorageClass(sc *storage_class.StorageClass) error
}
