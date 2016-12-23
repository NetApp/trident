// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type Client interface {
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
