// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"context"
	"crypto/tls"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type StoreType string

const (
	MemoryStore      StoreType = "memory"
	PassthroughStore StoreType = "passthrough"
	CRDV1Store       StoreType = "crdv1"
)

type ClientConfig struct {
	endpoints string
	TLSConfig *tls.Config
}

type Client interface {
	GetVersion(ctx context.Context) (*config.PersistentStateVersion, error)
	SetVersion(ctx context.Context, version *config.PersistentStateVersion) error
	GetConfig() *ClientConfig
	GetType() StoreType
	Stop() error

	AddBackend(ctx context.Context, b *storage.Backend) error
	AddBackendPersistent(ctx context.Context, b *storage.BackendPersistent) error
	GetBackend(ctx context.Context, backendName string) (*storage.BackendPersistent, error)
	UpdateBackend(ctx context.Context, b *storage.Backend) error
	UpdateBackendPersistent(ctx context.Context, b *storage.BackendPersistent) error
	DeleteBackend(ctx context.Context, backend *storage.Backend) error
	GetBackends(ctx context.Context) ([]*storage.BackendPersistent, error)
	DeleteBackends(ctx context.Context) error
	ReplaceBackendAndUpdateVolumes(ctx context.Context, origBackend, newBackend *storage.Backend) error

	AddVolume(ctx context.Context, vol *storage.Volume) error
	AddVolumePersistent(ctx context.Context, vol *storage.VolumeExternal) error
	GetVolume(ctx context.Context, volName string) (*storage.VolumeExternal, error)
	UpdateVolume(ctx context.Context, vol *storage.Volume) error
	UpdateVolumePersistent(ctx context.Context, vol *storage.VolumeExternal) error
	DeleteVolume(ctx context.Context, vol *storage.Volume) error
	DeleteVolumeIgnoreNotFound(ctx context.Context, vol *storage.Volume) error
	GetVolumes(ctx context.Context) ([]*storage.VolumeExternal, error)
	DeleteVolumes(ctx context.Context) error

	AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
	GetVolumeTransactions(ctx context.Context) ([]*storage.VolumeTransaction, error)
	UpdateVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
	GetExistingVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) (
		*storage.VolumeTransaction, error,
	)
	DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error

	AddStorageClass(ctx context.Context, sc *storageclass.StorageClass) error
	GetStorageClass(ctx context.Context, scName string) (*storageclass.Persistent, error)
	GetStorageClasses(ctx context.Context) ([]*storageclass.Persistent, error)
	DeleteStorageClass(ctx context.Context, sc *storageclass.StorageClass) error

	AddOrUpdateNode(ctx context.Context, n *utils.Node) error
	GetNode(ctx context.Context, nName string) (*utils.Node, error)
	GetNodes(ctx context.Context) ([]*utils.Node, error)
	DeleteNode(ctx context.Context, n *utils.Node) error

	AddSnapshot(ctx context.Context, snapshot *storage.Snapshot) error
	GetSnapshot(ctx context.Context, volumeName, snapshotName string) (*storage.SnapshotPersistent, error)
	GetSnapshots(ctx context.Context) ([]*storage.SnapshotPersistent, error)
	UpdateSnapshot(ctx context.Context, snapshot *storage.Snapshot) error
	DeleteSnapshot(ctx context.Context, snapshot *storage.Snapshot) error
	DeleteSnapshotIgnoreNotFound(ctx context.Context, snapshot *storage.Snapshot) error
	DeleteSnapshots(ctx context.Context) error
}

type CRDClient interface {
	Client
	HasBackends(ctx context.Context) (bool, error)
	AddStorageClassPersistent(ctx context.Context, b *storageclass.Persistent) error
	HasStorageClasses(ctx context.Context) (bool, error)
	HasVolumes(ctx context.Context) (bool, error)
	HasVolumeTransactions(ctx context.Context) (bool, error)
}
