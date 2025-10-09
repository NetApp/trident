// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

//go:generate mockgen -destination=../mocks/mock_storage/mock_storage.go github.com/netapp/trident/storage Backend,Pool

import (
	"context"
	"sync"

	"github.com/RoaringBitmap/roaring/v2"

	"github.com/netapp/trident/config"
	storageattribute "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/models"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/netapp/trident/storage.Driver,github.com/netapp/trident/storage.Pool
type Backend interface {
	Driver() Driver
	SetDriver(Driver Driver)
	MarshalDriverConfig() ([]byte, error)
	Name() string
	SetName(Name string)
	BackendUUID() string
	SetBackendUUID(BackendUUID string)
	Online() bool
	SetOnline(Online bool)
	State() BackendState
	UserState() UserBackendState
	StateReason() string
	SetState(State BackendState)
	SetUserState(State UserBackendState)
	StoragePools() *sync.Map
	ClearStoragePools()
	Volumes() *sync.Map
	ClearVolumes()
	ConfigRef() string
	SetConfigRef(ConfigRef string)
	AddStoragePool(pool Pool)
	GetPhysicalPoolNames(ctx context.Context) []string
	GetDriverName() string
	GetProtocol(ctx context.Context) config.Protocol
	IsCredentialsFieldSet(ctx context.Context) bool
	CreatePrepare(ctx context.Context, volConfig *VolumeConfig, storagePool Pool)
	AddVolume(
		ctx context.Context, volConfig *VolumeConfig, storagePool Pool,
		volAttributes map[string]storageattribute.Request, retry bool,
	) (*Volume, error)
	GetDebugTraceFlags(ctx context.Context) map[string]bool
	CloneVolume(
		ctx context.Context, sourceVolConfig, cloneVolConfig *VolumeConfig, storagePool Pool, retry bool,
	) (*Volume, error)
	PublishVolume(
		ctx context.Context, volConfig *VolumeConfig, publishInfo *models.VolumePublishInfo,
	) error
	UnpublishVolume(ctx context.Context, volConfig *VolumeConfig, publishInfo *models.VolumePublishInfo) error
	UpdateVolume(ctx context.Context, volConfig *VolumeConfig, updateInfo *models.VolumeUpdateInfo) (map[string]*Volume, error)
	GetVolumeForImport(ctx context.Context, volumeID string) (*VolumeExternal, error)
	ImportVolume(ctx context.Context, volConfig *VolumeConfig) (*Volume, error)
	ResizeVolume(ctx context.Context, volConfig *VolumeConfig, newSize string) error
	RenameVolume(ctx context.Context, volConfig *VolumeConfig, newName string) error
	RemoveVolume(ctx context.Context, volConfig *VolumeConfig) error
	RemoveCachedVolume(volumeName string)
	CanSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	CanGroupSnapshot() bool
	GetSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error)
	CreateSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	RestoreSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	DeleteSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	GetUpdateType(ctx context.Context, origBackend Backend) *roaring.Bitmap
	HasVolumes() bool
	Terminate(ctx context.Context)
	InvalidateNodeAccess()
	SetNodeAccessUpToDate()
	IsNodeAccessUpToDate() bool
	ReconcileNodeAccess(ctx context.Context, nodes []*models.Node, tridentUUID string) error
	ReconcileVolumeNodeAccess(ctx context.Context, volConfig *VolumeConfig, nodes []*models.Node) error
	CanGetState() bool
	GetBackendState(ctx context.Context) (string, *roaring.Bitmap)
	UpdateBackendState(ctx context.Context, stateReason string)
	ConstructExternal(ctx context.Context) *BackendExternal
	ConstructExternalWithPoolMap(ctx context.Context, poolMap map[string][]string) *BackendExternal
	ConstructPersistent(ctx context.Context) *BackendPersistent
	CanMirror() bool
	SmartCopy() interface{}
	DeepCopyType() Backend
	GetUniqueKey() string
	Mirrorer
	ChapEnabled
	PublishEnforceable
	GroupSnapshotter
}

type PublishEnforceable interface {
	EnablePublishEnforcement(ctx context.Context, volume *Volume) error
	CanEnablePublishEnforcement() bool
}

type ChapEnabled interface {
	GetChapInfo(ctx context.Context, volumeName, nodeName string) (*models.IscsiChapInfo, error)
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
	ConstructExternalWithPoolMap(poolMap map[string][]string) *PoolExternal
	GetLabelsJSON(ctx context.Context, key string, labelLimit int) (string, error)
	GetTemplatizedLabelsJSON(ctx context.Context, key string, labelLimit int, templateData map[string]interface{}) (string, error)
	GetLabels(ctx context.Context, prefix string) map[string]string
	GetLabelMapFromTemplate(ctx context.Context, templateData map[string]interface{}) map[string]string
}
