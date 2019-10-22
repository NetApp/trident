// Copyright 2019 NetApp, Inc. All Rights Reserved.

package core

import (
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type Orchestrator interface {
	Bootstrap() error
	AddFrontend(f frontend.Plugin)
	GetFrontend(name string) (frontend.Plugin, error)
	GetVersion() (string, error)

	AddBackend(configJSON string) (*storage.BackendExternal, error)
	DeleteBackend(backend string) error
	DeleteBackendByBackendUUID(backendName, backendUUID string) error
	GetBackend(backend string) (*storage.BackendExternal, error)
	GetBackendByBackendUUID(backendUUID string) (*storage.BackendExternal, error)
	ListBackends() ([]*storage.BackendExternal, error)
	UpdateBackend(backendName, configJSON string) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendByBackendUUID(backendName, configJSON, backendUUID string) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendState(backendName, backendState string) (storageBackendExternal *storage.BackendExternal, err error)

	AddVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	AttachVolume(volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo) error
	CloneVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	DetachVolume(volumeName, mountpoint string) error
	DeleteVolume(volume string) error
	GetVolume(volume string) (*storage.VolumeExternal, error)
	GetVolumeExternal(volumeName string, backendName string) (*storage.VolumeExternal, error)
	GetVolumeType(vol *storage.VolumeExternal) (config.VolumeType, error)
	LegacyImportVolume(volumeConfig *storage.VolumeConfig, backendName string, notManaged bool, createPVandPVC VolumeCallback) (*storage.VolumeExternal, error)
	ImportVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	ListVolumes() ([]*storage.VolumeExternal, error)
	ListVolumesByPlugin(pluginName string) ([]*storage.VolumeExternal, error)
	PublishVolume(volumeName string, publishInfo *utils.VolumePublishInfo) error
	ResizeVolume(volumeName, newSize string) error
	SetVolumeState(volumeName string, state storage.VolumeState) error

	CreateSnapshot(snapshotConfig *storage.SnapshotConfig) (*storage.SnapshotExternal, error)
	GetSnapshot(volumeName, snapshotName string) (*storage.SnapshotExternal, error)
	ListSnapshots() ([]*storage.SnapshotExternal, error)
	ListSnapshotsByName(snapshotName string) ([]*storage.SnapshotExternal, error)
	ListSnapshotsForVolume(volumeName string) ([]*storage.SnapshotExternal, error)
	ReadSnapshotsForVolume(volumeName string) ([]*storage.SnapshotExternal, error)
	DeleteSnapshot(volumeName, snapshotName string) error

	GetDriverTypeForVolume(vol *storage.VolumeExternal) (string, error)
	ReloadVolumes() error

	AddStorageClass(scConfig *storageclass.Config) (*storageclass.External, error)
	DeleteStorageClass(scName string) error
	GetStorageClass(scName string) (*storageclass.External, error)
	ListStorageClasses() ([]*storageclass.External, error)

	AddNode(node *utils.Node) error
	GetNode(nName string) (*utils.Node, error)
	ListNodes() ([]*utils.Node, error)
	DeleteNode(nName string) error

	AddVolumeTransaction(volTxn *storage.VolumeTransaction) error
	GetVolumeTransaction(volTxn *storage.VolumeTransaction) (*storage.VolumeTransaction, error)
	DeleteVolumeTransaction(volTxn *storage.VolumeTransaction) error
}

type NotReadyError struct {
	message string
}

func (e *NotReadyError) Error() string { return e.message }

type BootstrapError struct {
	message string
}

func (e *BootstrapError) Error() string { return e.message }

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string { return e.Message }

type FoundError struct {
	message string
}

func (e *FoundError) Error() string { return e.message }

type UnsupportedError struct {
	message string
}

func (e *UnsupportedError) Error() string { return e.message }

type VolumeDeletingError struct {
	message string
}

func (e *VolumeDeletingError) Error() string { return e.message }

type VolumeCallback func(*storage.VolumeExternal, string) error
