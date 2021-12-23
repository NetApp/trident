// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

//go:generate mockgen -destination=../mocks/mock_storage/mock_storage.go github.com/netapp/trident/storage Backend,Pool

import (
	"context"

	"github.com/RoaringBitmap/roaring"

	"github.com/netapp/trident/config"
	storageattribute "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
)

type Backend interface {
	Driver() Driver
	SetDriver(Driver Driver)
	Name() string
	SetName(Name string)
	BackendUUID() string
	SetBackendUUID(BackendUUID string)
	Online() bool
	SetOnline(Online bool)
	State() BackendState
	SetState(State BackendState)
	Storage() map[string]Pool
	SetStorage(Storage map[string]Pool)
	Volumes() map[string]*Volume
	SetVolumes(Volumes map[string]*Volume)
	ConfigRef() string
	SetConfigRef(ConfigRef string)
	AddStoragePool(pool Pool)
	GetPhysicalPoolNames(ctx context.Context) []string
	GetDriverName() string
	GetProtocol(ctx context.Context) config.Protocol
	IsCredentialsFieldSet(ctx context.Context) bool
	AddVolume(
		ctx context.Context, volConfig *VolumeConfig, storagePool Pool,
		volAttributes map[string]storageattribute.Request, retry bool,
	) (*Volume, error)
	GetDebugTraceFlags(ctx context.Context) map[string]bool
	CloneVolume(
		ctx context.Context, sourceVolConfig, cloneVolConfig *VolumeConfig, storagePool Pool, retry bool,
	) (*Volume, error)
	PublishVolume(
		ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo,
	) error
	UnpublishVolume(ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo) error
	GetVolumeExternal(ctx context.Context, volumeName string) (*VolumeExternal, error)
	ImportVolume(ctx context.Context, volConfig *VolumeConfig) (*Volume, error)
	ResizeVolume(ctx context.Context, volConfig *VolumeConfig, newSize string) error
	RenameVolume(ctx context.Context, volConfig *VolumeConfig, newName string) error
	RemoveVolume(ctx context.Context, volConfig *VolumeConfig) error
	RemoveCachedVolume(volumeName string)
	CanSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	GetSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error)
	CreateSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	RestoreSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	DeleteSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	GetUpdateType(ctx context.Context, origBackend Backend) *roaring.Bitmap
	HasVolumes() bool
	Terminate(ctx context.Context)
	InvalidateNodeAccess()
	ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node) error
	ConstructExternal(ctx context.Context) *BackendExternal
	ConstructPersistent(ctx context.Context) *BackendPersistent
	CanMirror() bool
}

type Pool interface {
	Name() string
	SetName(name string)
	StorageClasses() []string
	SetStorageClasses(storageClasses []string)
	Backend() Backend
	SetBackend(backend Backend)
	Attributes() map[string]storageattribute.Offer
	SetAttributes(attributes map[string]storageattribute.Offer)
	InternalAttributes() map[string]string
	SetInternalAttributes(internalAttributes map[string]string)
	SupportedTopologies() []map[string]string
	SetSupportedTopologies(supportedTopologies []map[string]string)
	AddStorageClass(class string)
	RemoveStorageClass(class string) bool
	ConstructExternal() *PoolExternal
	GetLabelsJSON(ctx context.Context, key string, labelLimit int) (string, error)
	GetLabels(ctx context.Context, prefix string) map[string]string
}
