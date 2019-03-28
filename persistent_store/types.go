// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"crypto/tls"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type StoreType string

const (
	MemoryStore      StoreType = "memory"
	EtcdV2Store      StoreType = "etcdv2"
	EtcdV3Store      StoreType = "etcdv3"
	PassthroughStore StoreType = "passthrough"
)

type PersistentStateVersion struct {
	PersistentStoreVersion string `json:"store_version"`
	OrchestratorAPIVersion string `json:"orchestrator_api_version"`
}

type ClientConfig struct {
	endpoints string
	TLSConfig *tls.Config
}

type Client interface {
	GetVersion() (*PersistentStateVersion, error)
	SetVersion(version *PersistentStateVersion) error
	GetConfig() *ClientConfig
	GetType() StoreType
	Stop() error

	AddBackend(b *storage.Backend) error
	GetBackend(backendName string) (*storage.BackendPersistent, error)
	UpdateBackend(b *storage.Backend) error
	DeleteBackend(backend *storage.Backend) error
	GetBackends() ([]*storage.BackendPersistent, error)
	DeleteBackends() error
	ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error

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

	AddStorageClass(sc *storageclass.StorageClass) error
	GetStorageClass(scName string) (*storageclass.Persistent, error)
	GetStorageClasses() ([]*storageclass.Persistent, error)
	DeleteStorageClass(sc *storageclass.StorageClass) error

	AddSnapshot(snapshot *storage.Snapshot) error
	GetSnapshot(snapshotID string) (*storage.SnapshotExternal, error)
}

type EtcdClient interface {
	Client
	Create(key, value string) error
	Read(key string) (string, error)
	ReadKeys(keyPrefix string) ([]string, error)
	Update(key, value string) error
	Set(key, value string) error
	Delete(key string) error
	DeleteKeys(keyPrefix string) error
}
