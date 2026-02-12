// Copyright 2025 NetApp, Inc. All Rights Reserved.

package persistentstore

//go:generate mockgen -destination=../mocks/mock_persistent_store/mock_persistent_store.go -mock_names Client=MockStoreClient github.com/netapp/trident/persistent_store Client

import (
	"context"
	"crypto/tls"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
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
	GetTridentUUID(ctx context.Context) (string, error)
	GetVersion(ctx context.Context) (*config.PersistentStateVersion, error)
	SetVersion(ctx context.Context, version *config.PersistentStateVersion) error
	GetConfig() *ClientConfig
	GetType() StoreType
	Stop() error

	AddBackend(ctx context.Context, b storage.Backend) error
	GetBackend(ctx context.Context, backendName string) (*storage.BackendPersistent, error)
	UpdateBackend(ctx context.Context, b storage.Backend) error
	DeleteBackend(ctx context.Context, backend storage.Backend) error
	IsBackendDeleting(ctx context.Context, backend storage.Backend) bool
	GetBackends(ctx context.Context) ([]*storage.BackendPersistent, error)
	DeleteBackends(ctx context.Context) error
	ReplaceBackendAndUpdateVolumes(ctx context.Context, origBackend, newBackend storage.Backend) error
	GetBackendSecret(ctx context.Context, secretName string) (map[string]string, error)

	AddVolume(ctx context.Context, vol *storage.Volume) error
	GetVolume(ctx context.Context, volName string) (*storage.VolumeExternal, error)
	GetVolumes(ctx context.Context) ([]*storage.VolumeExternal, error)
	UpdateVolume(ctx context.Context, vol *storage.Volume) error
	DeleteVolume(ctx context.Context, vol *storage.Volume) error
	DeleteVolumes(ctx context.Context) error

	AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
	GetVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) (*storage.VolumeTransaction, error)
	GetVolumeTransactions(ctx context.Context) ([]*storage.VolumeTransaction, error)
	UpdateVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
	DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error

	AddStorageClass(ctx context.Context, sc *storageclass.StorageClass) error
	UpdateStorageClass(ctx context.Context, sc *storageclass.StorageClass) error
	GetStorageClass(ctx context.Context, scName string) (*storageclass.Persistent, error)
	GetStorageClasses(ctx context.Context) ([]*storageclass.Persistent, error)
	DeleteStorageClass(ctx context.Context, sc *storageclass.StorageClass) error

	AddOrUpdateNode(ctx context.Context, n *models.Node) error
	GetNode(ctx context.Context, nName string) (*models.Node, error)
	GetNodes(ctx context.Context) ([]*models.Node, error)
	DeleteNode(ctx context.Context, n *models.Node) error

	AddVolumePublication(ctx context.Context, vp *models.VolumePublication) error
	UpdateVolumePublication(ctx context.Context, vp *models.VolumePublication) error
	GetVolumePublication(ctx context.Context, vpName string) (*models.VolumePublication, error)
	GetVolumePublications(ctx context.Context) ([]*models.VolumePublication, error)
	DeleteVolumePublication(ctx context.Context, vp *models.VolumePublication) error

	AddSnapshot(ctx context.Context, snapshot *storage.Snapshot) error
	GetSnapshot(ctx context.Context, volumeName, snapshotName string) (*storage.SnapshotPersistent, error)
	GetSnapshots(ctx context.Context) ([]*storage.SnapshotPersistent, error)
	UpdateSnapshot(ctx context.Context, snapshot *storage.Snapshot) error
	DeleteSnapshot(ctx context.Context, snapshot *storage.Snapshot) error
	DeleteSnapshots(ctx context.Context) error

	AddGroupSnapshot(ctx context.Context, groupSnapshot *storage.GroupSnapshot) error
	GetGroupSnapshot(ctx context.Context, groupSnapshotName string) (*storage.GroupSnapshotPersistent, error)
	GetGroupSnapshots(ctx context.Context) ([]*storage.GroupSnapshotPersistent, error)
	UpdateGroupSnapshot(ctx context.Context, groupSnapshot *storage.GroupSnapshot) error
	DeleteGroupSnapshot(ctx context.Context, groupSnapshot *storage.GroupSnapshot) error
	DeleteGroupSnapshots(ctx context.Context) error

	GetAutogrowPolicy(ctx context.Context, agPolicyName string) (*storage.AutogrowPolicyPersistent, error)
	GetAutogrowPolicies(ctx context.Context) ([]*storage.AutogrowPolicyPersistent, error)
}

type CRDClient interface {
	Client
	HasBackends(ctx context.Context) (bool, error)
	HasStorageClasses(ctx context.Context) (bool, error)
	HasVolumes(ctx context.Context) (bool, error)
	HasVolumeTransactions(ctx context.Context) (bool, error)
}
