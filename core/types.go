// Copyright 2020 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type Orchestrator interface {
	Bootstrap() error
	AddFrontend(f frontend.Plugin)
	GetFrontend(ctx context.Context, name string) (frontend.Plugin, error)
	GetVersion(ctx context.Context) (string, error)

	AddBackend(ctx context.Context, configJSON string) (*storage.BackendExternal, error)
	DeleteBackend(ctx context.Context, backend string) error
	DeleteBackendByBackendUUID(ctx context.Context, backendName, backendUUID string) error
	GetBackend(ctx context.Context, backend string) (*storage.BackendExternal, error)
	GetBackendByBackendUUID(ctx context.Context, backendUUID string) (*storage.BackendExternal, error)
	ListBackends(ctx context.Context) ([]*storage.BackendExternal, error)
	UpdateBackend(ctx context.Context, backendName, configJSON string) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendByBackendUUID(ctx context.Context, backendName, configJSON, backendUUID string) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendState(ctx context.Context, backendName, backendState string) (storageBackendExternal *storage.BackendExternal, err error)

	AddVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	AttachVolume(ctx context.Context, volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo) error
	CloneVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	DetachVolume(ctx context.Context, volumeName, mountpoint string) error
	DeleteVolume(ctx context.Context, volume string) error
	GetVolume(ctx context.Context, volume string) (*storage.VolumeExternal, error)
	GetVolumeExternal(ctx context.Context, volumeName string, backendName string) (*storage.VolumeExternal, error)
	GetVolumeType(ctx context.Context, vol *storage.VolumeExternal) (config.VolumeType, error)
	LegacyImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig, backendName string, notManaged bool, createPVandPVC VolumeCallback) (*storage.VolumeExternal, error)
	ImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	ListVolumes(ctx context.Context) ([]*storage.VolumeExternal, error)
	ListVolumesByPlugin(ctx context.Context, pluginName string) ([]*storage.VolumeExternal, error)
	PublishVolume(ctx context.Context, volumeName string, publishInfo *utils.VolumePublishInfo) error
	ResizeVolume(ctx context.Context, volumeName, newSize string) error
	SetVolumeState(ctx context.Context, volumeName string, state storage.VolumeState) error

	CreateSnapshot(ctx context.Context, snapshotConfig *storage.SnapshotConfig) (*storage.SnapshotExternal, error)
	GetSnapshot(ctx context.Context, volumeName, snapshotName string) (*storage.SnapshotExternal, error)
	ListSnapshots(ctx context.Context) ([]*storage.SnapshotExternal, error)
	ListSnapshotsByName(ctx context.Context, snapshotName string) ([]*storage.SnapshotExternal, error)
	ListSnapshotsForVolume(ctx context.Context, volumeName string) ([]*storage.SnapshotExternal, error)
	ReadSnapshotsForVolume(ctx context.Context, volumeName string) ([]*storage.SnapshotExternal, error)
	DeleteSnapshot(ctx context.Context, volumeName, snapshotName string) error

	GetDriverTypeForVolume(ctx context.Context, vol *storage.VolumeExternal) (string, error)
	ReloadVolumes(ctx context.Context) error

	AddStorageClass(ctx context.Context, scConfig *storageclass.Config) (*storageclass.External, error)
	DeleteStorageClass(ctx context.Context, scName string) error
	GetStorageClass(ctx context.Context, scName string) (*storageclass.External, error)
	ListStorageClasses(ctx context.Context) ([]*storageclass.External, error)

	AddNode(ctx context.Context, node *utils.Node) error
	GetNode(ctx context.Context, nName string) (*utils.Node, error)
	ListNodes(ctx context.Context) ([]*utils.Node, error)
	DeleteNode(ctx context.Context, nName string) error

	AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
	GetVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) (*storage.VolumeTransaction, error)
	DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
}

type VolumeCallback func(*storage.VolumeExternal, string) error
