// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"crypto/tls"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type StoreType string

const (
	MemoryStore      StoreType = "memory"
	EtcdV2Store      StoreType = "etcdv2"
	EtcdV3Store      StoreType = "etcdv3"
	EtcdV3bStore     StoreType = "etcdv3b"
	PassthroughStore StoreType = "passthrough"
	CRDV1Store       StoreType = "crdv1"
)

type ClientConfig struct {
	endpoints string
	TLSConfig *tls.Config
}

type Client interface {
	GetVersion() (*config.PersistentStateVersion, error)
	SetVersion(version *config.PersistentStateVersion) error
	GetConfig() *ClientConfig
	GetType() StoreType
	Stop() error

	AddBackend(b *storage.Backend) error
	AddBackendPersistent(b *storage.BackendPersistent) error
	GetBackend(backendName string) (*storage.BackendPersistent, error)
	UpdateBackend(b *storage.Backend) error
	UpdateBackendPersistent(b *storage.BackendPersistent) error
	DeleteBackend(backend *storage.Backend) error
	GetBackends() ([]*storage.BackendPersistent, error)
	DeleteBackends() error
	ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error

	AddVolume(vol *storage.Volume) error
	AddVolumePersistent(vol *storage.VolumeExternal) error
	GetVolume(volName string) (*storage.VolumeExternal, error)
	UpdateVolume(vol *storage.Volume) error
	UpdateVolumePersistent(vol *storage.VolumeExternal) error
	DeleteVolume(vol *storage.Volume) error
	DeleteVolumeIgnoreNotFound(vol *storage.Volume) error
	GetVolumes() ([]*storage.VolumeExternal, error)
	DeleteVolumes() error

	AddVolumeTransaction(volTxn *storage.VolumeTransaction) error
	GetVolumeTransactions() ([]*storage.VolumeTransaction, error)
	GetExistingVolumeTransaction(volTxn *storage.VolumeTransaction) (*storage.VolumeTransaction, error)
	DeleteVolumeTransaction(volTxn *storage.VolumeTransaction) error

	AddStorageClass(sc *storageclass.StorageClass) error
	GetStorageClass(scName string) (*storageclass.Persistent, error)
	GetStorageClasses() ([]*storageclass.Persistent, error)
	DeleteStorageClass(sc *storageclass.StorageClass) error

	AddOrUpdateNode(n *utils.Node) error
	GetNode(nName string) (*utils.Node, error)
	GetNodes() ([]*utils.Node, error)
	DeleteNode(n *utils.Node) error

	AddSnapshot(snapshot *storage.Snapshot) error
	GetSnapshot(volumeName, snapshotName string) (*storage.SnapshotPersistent, error)
	GetSnapshots() ([]*storage.SnapshotPersistent, error)
	DeleteSnapshot(snapshot *storage.Snapshot) error
	DeleteSnapshotIgnoreNotFound(snapshot *storage.Snapshot) error
	DeleteSnapshots() error
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

type CRDClient interface {
	Client
	HasBackends() (bool, error)
	AddStorageClassPersistent(b *storageclass.Persistent) error
	HasStorageClasses() (bool, error)
	HasVolumes() (bool, error)
	HasVolumeTransactions() (bool, error)
}
