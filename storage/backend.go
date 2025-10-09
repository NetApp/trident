// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

//go:generate mockgen -destination=../mocks/mock_storage/mock_storage_driver.go github.com/netapp/trident/storage Driver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/brunoga/deep"
	"github.com/cenkalti/backoff/v4"

	"github.com/netapp/trident/acp"
	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Driver provides a common interface for storage related operations
type Driver interface {
	Name() string
	BackendName() string
	Initialize(
		context.Context, tridentconfig.DriverContext, string, *drivers.CommonStorageDriverConfig,
		map[string]string, string,
	) error
	Initialized() bool
	// Terminate tells the driver to clean up, as it won't be called again.
	Terminate(ctx context.Context, backendUUID string)
	Create(ctx context.Context, volConfig *VolumeConfig, storagePool Pool, volAttributes map[string]sa.Request) error
	CreatePrepare(ctx context.Context, volConfig *VolumeConfig, storagePool Pool)
	// CreateFollowup adds necessary information for accessing the volume to VolumeConfig.
	CreateFollowup(ctx context.Context, volConfig *VolumeConfig) error
	CreateClone(ctx context.Context, sourceVolConfig, cloneVolConfig *VolumeConfig, storagePool Pool) error
	Import(ctx context.Context, volConfig *VolumeConfig, originalName string) error
	Destroy(ctx context.Context, volConfig *VolumeConfig) error
	Rename(ctx context.Context, name, newName string) error
	Resize(ctx context.Context, volConfig *VolumeConfig, sizeBytes uint64) error
	Get(ctx context.Context, name string) error
	GetInternalVolumeName(ctx context.Context, volConfig *VolumeConfig, storagePool Pool) string
	GetStorageBackendSpecs(ctx context.Context, backend Backend) error
	GetStorageBackendPhysicalPoolNames(ctx context.Context) []string
	GetProtocol(ctx context.Context) tridentconfig.Protocol
	Publish(ctx context.Context, volConfig *VolumeConfig, publishInfo *models.VolumePublishInfo) error
	CanSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	GetSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error)
	CreateSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	RestoreSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	DeleteSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	GetConfig() drivers.DriverConfig
	StoreConfig(ctx context.Context, b *PersistentStorageBackendConfig)
	// GetExternalConfig returns a version of the driver configuration that
	// lacks confidential information, such as usernames and passwords.
	GetExternalConfig(ctx context.Context) interface{}
	// GetVolumeForImport accepts a string that uniquely identifies a volume in a backend-specific
	// format and returns a VolumeExternal object.
	GetVolumeForImport(ctx context.Context, volumeID string) (*VolumeExternal, error)
	// GetVolumeExternalWrappers reads all volumes owned by this driver from the storage backend and
	// writes them to the supplied channel as VolumeExternalWrapper objects.  This method is only
	// available if using the passthrough store (i.e. Docker).
	GetVolumeExternalWrappers(context.Context, chan *VolumeExternalWrapper)
	GetUpdateType(ctx context.Context, driver Driver) *roaring.Bitmap
	ReconcileNodeAccess(ctx context.Context, nodes []*models.Node, backendUUID, tridentUUID string) error
	ReconcileVolumeNodeAccess(ctx context.Context, volConfig *VolumeConfig, nodes []*models.Node) error
	GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig
}

type Unpublisher interface {
	Unpublish(ctx context.Context, volConfig *VolumeConfig, publishInfo *models.VolumePublishInfo) error
}

// Mirrorer provides a common interface for backends that support mirror replication
type Mirrorer interface {
	EstablishMirror(
		ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule string,
	) error
	ReestablishMirror(
		ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule string,
	) error
	PromoteMirror(ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotName string) (bool, error)
	GetMirrorStatus(ctx context.Context, localInternalVolumeName, remoteVolumeHandle string) (string, error)
	ReleaseMirror(ctx context.Context, localInternalVolumeName string) error
	GetReplicationDetails(ctx context.Context, localInternalVolumeName, remoteVolumeHandle string) (string, string,
		string, error)
	UpdateMirror(ctx context.Context, localInternalVolumeName, snapshotName string) error
	CheckMirrorTransferState(ctx context.Context, pvcVolumeName string) (*time.Time, error)
	GetMirrorTransferTime(ctx context.Context, pvcVolumeName string) (*time.Time, error)
}

type GroupSnapshotter interface {
	GetGroupSnapshotTarget(ctx context.Context, volConfigs []*VolumeConfig) (*GroupSnapshotTargetInfo, error)
	CreateGroupSnapshot(ctx context.Context, config *GroupSnapshotConfig, target *GroupSnapshotTargetInfo) error
	ProcessGroupSnapshot(
		ctx context.Context, config *GroupSnapshotConfig, volConfigs []*VolumeConfig,
	) ([]*Snapshot, error)
	ConstructGroupSnapshot(
		ctx context.Context, config *GroupSnapshotConfig, snapshots []*Snapshot,
	) (*GroupSnapshot, error)
}

// StateGetter provides a common interface for backends that support polling backend for state information.
type StateGetter interface {
	GetBackendState(ctx context.Context) (string, *roaring.Bitmap)
}

// VolumeUpdater provides a common interface for backends that support updating the volume
type VolumeUpdater interface {
	Update(
		ctx context.Context, volConfig *VolumeConfig,
		updateInfo *models.VolumeUpdateInfo, allVolumes map[string]*Volume,
	) (map[string]*Volume, error)
}

type StorageBackend struct {
	driver       Driver
	name         string
	backendUUID  string
	storagePools *sync.Map
	volumes      *sync.Map
	configRef    string

	stateLock          *sync.RWMutex
	online             bool
	state              BackendState
	stateReason        string
	userState          UserBackendState
	nodeAccessUpToDate bool
}

func (b *StorageBackend) UpdateVolume(
	ctx context.Context, volConfig *VolumeConfig, updateInfo *models.VolumeUpdateInfo,
) (map[string]*Volume, error) {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
		"updateInfo":     updateInfo,
	}).Debug("Attempting volume update.")

	// Ensure driver supports volume update operation
	volUpdateDriver, ok := b.driver.(VolumeUpdater)
	if !ok {
		return nil, errors.UnsupportedError(
			fmt.Sprintf("volume update is not supported for backend of type %v", b.driver.Name()))
	}

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return nil, errors.NotManagedError("source volume %s is not managed by Trident", volConfig.InternalName)
	}

	allVolumes := make(map[string]*Volume)
	b.volumes.Range(func(k, v interface{}) bool {
		allVolumes[k.(string)] = v.(*Volume)
		return true
	})

	return volUpdateDriver.Update(ctx, volConfig, updateInfo, allVolumes)
}

func (b *StorageBackend) Driver() Driver {
	return b.driver
}

func (b *StorageBackend) SetDriver(Driver Driver) {
	b.driver = Driver
}

func (b *StorageBackend) MarshalDriverConfig() ([]byte, error) {
	driverConfig := b.driver.GetConfig()
	return driverConfig.Marshal()
}

func (b *StorageBackend) Name() string {
	return b.name
}

func (b *StorageBackend) SetName(Name string) {
	b.name = Name
}

func (b *StorageBackend) BackendUUID() string {
	return b.backendUUID
}

func (b *StorageBackend) SetBackendUUID(BackendUUID string) {
	b.backendUUID = BackendUUID
}

func (b *StorageBackend) Online() bool {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	return b.online
}

func (b *StorageBackend) SetOnline(Online bool) {
	b.stateLock.Lock()
	defer b.stateLock.Unlock()

	b.online = Online
}

func (b *StorageBackend) State() BackendState {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	return b.state
}

func (b *StorageBackend) UserState() UserBackendState {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	return b.userState
}

func (b *StorageBackend) StateReason() string {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	return b.stateReason
}

// SetState sets the 'state' and 'online' fields of StorageBackend accordingly.
func (b *StorageBackend) SetState(state BackendState) {
	b.stateLock.Lock()
	defer b.stateLock.Unlock()

	b.state = state
	b.online = state.IsOnline()
}

func (b *StorageBackend) SetUserState(state UserBackendState) {
	b.stateLock.Lock()
	defer b.stateLock.Unlock()

	b.userState = state
}

func (b *StorageBackend) StoragePools() *sync.Map {
	return b.storagePools
}

func (b *StorageBackend) ClearStoragePools() {
	b.storagePools = new(sync.Map)
}

func (b *StorageBackend) Volumes() *sync.Map {
	return b.volumes
}

func (b *StorageBackend) ClearVolumes() {
	b.volumes = new(sync.Map)
}

func (b *StorageBackend) ConfigRef() string {
	return b.configRef
}

func (b *StorageBackend) SetConfigRef(ConfigRef string) {
	b.configRef = ConfigRef
}

type UpdateBackendStateRequest struct {
	BackendState     string `json:"state"`
	UserBackendState string `json:"userState"`
}

type BackendState string

const (
	Unknown  = BackendState("unknown")
	Online   = BackendState("online")
	Offline  = BackendState("offline")
	Deleting = BackendState("deleting")
	Failed   = BackendState("failed")
)

func (s BackendState) String() string {
	switch s {
	case Unknown, Online, Offline, Deleting, Failed:
		return string(s)
	default:
		return "unknown"
	}
}

func (s BackendState) IsUnknown() bool {
	switch s {
	case Online, Offline, Deleting, Failed:
		return false
	case Unknown:
		return true
	default:
		return true
	}
}

func (s BackendState) IsOnline() bool {
	return s == Online
}

func (s BackendState) IsOffline() bool {
	return s == Offline
}

func (s BackendState) IsDeleting() bool {
	return s == Deleting
}

func (s BackendState) IsFailed() bool {
	return s == Failed
}

type UserBackendState string

const (
	UserSuspended = UserBackendState("suspended")
	UserNormal    = UserBackendState("normal")
)

func (s UserBackendState) String() string {
	switch s {
	case UserSuspended, UserNormal:
		return string(s)
	default:
		return "unknown"
	}
}

func (s UserBackendState) IsSuspended() bool {
	return s == UserSuspended
}

func (s UserBackendState) IsNormal() bool {
	return s == UserNormal
}

func (s UserBackendState) Validate() error {
	switch s {
	case UserSuspended, UserNormal:
		return nil
	default:
		return fmt.Errorf("invalid UserBackendState: %v", s)
	}
}

func NewStorageBackend(ctx context.Context, driver Driver) (*StorageBackend, error) {
	backend := StorageBackend{
		driver:       driver,
		state:        Online,
		online:       true,
		userState:    UserNormal,
		storagePools: new(sync.Map),
		volumes:      new(sync.Map),
		stateLock:    new(sync.RWMutex),
	}

	// Retrieve backend specs.
	if err := backend.Driver().GetStorageBackendSpecs(ctx, &backend); err != nil {
		return nil, err
	}

	return &backend, nil
}

func NewFailedStorageBackend(ctx context.Context, driver Driver) Backend {
	backend := StorageBackend{
		name:         driver.BackendName(),
		driver:       driver,
		state:        Failed,
		storagePools: new(sync.Map),
		volumes:      new(sync.Map),
		stateLock:    new(sync.RWMutex),
	}

	Logc(ctx).WithFields(LogFields{
		"backendUUID": backend.BackendUUID(),
		"backendName": backend.Name(),
		"driver":      driver.Name(),
	}).Debug("Failed storagePools backend.")

	return &backend
}

func NewTestStorageBackend() Backend {
	return &StorageBackend{
		state:        Online,
		online:       true,
		userState:    UserNormal,
		storagePools: new(sync.Map),
		volumes:      new(sync.Map),
		stateLock:    new(sync.RWMutex),
	}
}

func (b *StorageBackend) AddStoragePool(pool Pool) {
	b.storagePools.Store(pool.Name(), pool)
}

func (b *StorageBackend) GetPhysicalPoolNames(ctx context.Context) []string {
	return b.driver.GetStorageBackendPhysicalPoolNames(ctx)
}

func (b *StorageBackend) GetDriverName() string {
	return b.driver.Name()
}

func (b *StorageBackend) GetProtocol(ctx context.Context) tridentconfig.Protocol {
	return b.driver.GetProtocol(ctx)
}

func (b *StorageBackend) IsCredentialsFieldSet(ctx context.Context) bool {
	commonConfig := b.driver.GetCommonConfig(ctx)
	if commonConfig != nil {
		return commonConfig.Credentials != nil
	}

	return false
}

func (b *StorageBackend) CreatePrepare(ctx context.Context, volConfig *VolumeConfig, storagePool Pool) {
	if b.driver != nil {
		b.driver.CreatePrepare(ctx, volConfig, storagePool)
	}
}

func (b *StorageBackend) AddVolume(
	ctx context.Context, volConfig *VolumeConfig, storagePool Pool, volAttributes map[string]sa.Request, retry bool,
) (*Volume, error) {
	var err error

	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"backendUUID":    b.backendUUID,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
		"storage_pool":   storagePool.Name(),
		"size":           volConfig.Size,
		"storage_class":  volConfig.StorageClass,
	}).Debug("Attempting volume create.")

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	// Ensure provisioning is allowed
	if !b.isProvisioningAllowed() {
		return nil, fmt.Errorf("volume is not created because the backend %s is suspended", b.name)
	}

	// Ensure the internal name exists
	if volConfig.InternalName == "" {
		return nil, errors.New("internal name not set")
	}

	// Add volume to the backend
	volumeExists := false
	if err = b.driver.Create(ctx, volConfig, storagePool, volAttributes); err != nil {
		if drivers.IsVolumeExistsError(err) {

			// Implement idempotency by ignoring the error if the volume exists already
			volumeExists = true

			Logc(ctx).WithFields(LogFields{
				"backend": b.name,
				"volume":  volConfig.InternalName,
			}).Warning("Volume already exists.")

		} else {
			// If the volume doesn't exist but the create failed, return the error
			return nil, err
		}
	}

	// Always perform the follow-up steps
	if err = b.driver.CreateFollowup(ctx, volConfig); err != nil {

		Logc(ctx).WithFields(LogFields{
			"backend":      b.name,
			"volume":       volConfig.InternalName,
			"volumeExists": volumeExists,
			"retry":        retry,
		}).Errorf("CreateFollowup failed; %v", err)

		// If follow-up fails and we just created the volume, clean up by deleting it
		if !volumeExists || retry {

			Logc(ctx).WithFields(LogFields{
				"backend": b.name,
				"volume":  volConfig.InternalName,
			}).Errorf("CreateFollowup failed for newly created volume, deleting the volume.")

			errDestroy := b.driver.Destroy(ctx, volConfig)
			if errDestroy != nil {
				Logc(ctx).WithFields(LogFields{
					"backend": b.name,
					"volume":  volConfig.InternalName,
				}).Warnf("Mapping the created volume failed and %s wasn't able to delete it afterwards: %s. "+
					"Volume must be manually deleted.", tridentconfig.OrchestratorName, errDestroy)
			}
		}

		// In all cases where follow-up fails, return the follow-up error
		return nil, err
	}

	vol := NewVolume(volConfig, b.backendUUID, storagePool.Name(), false, VolumeStateOnline)
	b.volumes.Store(vol.Config.Name, vol)
	return vol, nil
}

func (b *StorageBackend) GetDebugTraceFlags(ctx context.Context) map[string]bool {
	var emptyMap map[string]bool
	if b == nil {
		return emptyMap
	}

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Warn("Panicked while getting debug trace flags.")
		}
	}()

	// The backend configs are all different, so use reflection to pull out the debug trace flags map
	cfg := b.ConstructExternal(ctx).Config
	v := reflect.ValueOf(cfg)
	field := v.FieldByName("DebugTraceFlags")
	if field.IsZero() {
		return emptyMap
	} else if flags, ok := field.Interface().(map[string]bool); !ok {
		return emptyMap
	} else {
		return flags
	}
}

func (b *StorageBackend) CloneVolume(
	ctx context.Context, sourceVolConfig, cloneVolConfig *VolumeConfig, storagePool Pool, retry bool,
) (*Volume, error) {
	fields := LogFields{
		"backend":                cloneVolConfig.Name,
		"backendUUID":            b.backendUUID,
		"storageClass":           cloneVolConfig.StorageClass,
		"sourceVolume":           cloneVolConfig.CloneSourceVolume,
		"sourceVolumeInternal":   cloneVolConfig.CloneSourceVolumeInternal,
		"sourceSnapshot":         cloneVolConfig.CloneSourceSnapshot,
		"sourceSnapshotInternal": cloneVolConfig.CloneSourceSnapshotInternal,
		"cloneVolume":            cloneVolConfig.Name,
		"cloneVolumeInternal":    cloneVolConfig.InternalName,
	}
	Logc(ctx).WithFields(fields).Debug("Attempting volume clone.")

	if cloneVolConfig.ReadOnlyClone {
		if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureReadOnlyClone); err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Could not clone a read-only volume.")
			return nil, err
		}
	}

	// Ensure volume is managed
	if cloneVolConfig.ImportNotManaged {
		return nil, errors.NotManagedError("volume %s is not managed by Trident", cloneVolConfig.InternalName)
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	// Ensure the internal names exist
	if cloneVolConfig.InternalName == "" {
		return nil, errors.New("internal name not set")
	}
	if cloneVolConfig.CloneSourceVolumeInternal == "" {
		return nil, errors.New("clone source volume internal name not set")
	}

	// Clone volume on the backend
	volumeExists := false
	if err := b.driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool); err != nil {
		if drivers.IsVolumeExistsError(err) {

			// Implement idempotency by ignoring the error if the volume exists already
			volumeExists = true

			Logc(ctx).WithFields(LogFields{
				"backend": b.name,
				"volume":  cloneVolConfig.InternalName,
			}).Warning("Volume already exists.")

		} else {
			// If the volume doesn't exist but the create failed, return the error
			return nil, err
		}
	}

	// If it's RO clone, skip clone volume check in the driver
	if !cloneVolConfig.ReadOnlyClone {
		// The clone may not be fully created when the clone API returns, so wait here until it exists.
		checkCloneExists := func() error {
			return b.driver.Get(ctx, cloneVolConfig.InternalName)
		}
		cloneExistsNotify := func(err error, duration time.Duration) {
			Logc(ctx).WithField("increment", duration).Debug("Clone not yet present, waiting.")
		}
		cloneBackoff := backoff.NewExponentialBackOff()
		cloneBackoff.InitialInterval = 1 * time.Second
		cloneBackoff.Multiplier = 2
		cloneBackoff.RandomizationFactor = 0.1
		cloneBackoff.MaxElapsedTime = 90 * time.Second

		// Run the clone check using an exponential backoff
		if err := backoff.RetryNotify(checkCloneExists, cloneBackoff, cloneExistsNotify); err != nil {
			Logc(ctx).WithField("clone_volume", cloneVolConfig.Name).Warnf("Could not find clone after %3.2f seconds.",
				float64(cloneBackoff.MaxElapsedTime.Seconds()))
		} else {
			Logc(ctx).WithField("clone_volume", cloneVolConfig.Name).Debug("Clone found.")
		}

	}

	if err := b.driver.CreateFollowup(ctx, cloneVolConfig); err != nil {

		// If follow-up fails and we just created the volume, clean up by deleting it
		if !volumeExists || retry {
			errDestroy := b.driver.Destroy(ctx, cloneVolConfig)
			if errDestroy != nil {
				Logc(ctx).WithFields(LogFields{
					"backend": b.name,
					"volume":  cloneVolConfig.InternalName,
				}).Warnf("Mapping the created volume failed and %s wasn't able to delete it afterwards: %s. "+
					"Volume must be manually deleted.", tridentconfig.OrchestratorName, errDestroy)
			}
		}

		// In all cases where follow-up fails, return the follow-up error
		return nil, err
	}

	// Traditionally, cloned volumes never got assigned to a storage pool, however, after introduction
	// of Virtual Pools we try to assign cloned volume to the same storage pool as source volume.
	// It should be noted that imported volumes do not get assigned to any storage pools, therefore,
	// clones of imported volumes also gets assigned to unset storage pools.
	poolName := drivers.UnsetPool
	if storagePool != nil {
		poolName = storagePool.Name()
	}

	vol := NewVolume(cloneVolConfig, b.backendUUID, poolName, false, VolumeStateOnline)
	b.volumes.Store(vol.Config.Name, vol)
	return vol, nil
}

func (b *StorageBackend) PublishVolume(
	ctx context.Context, volConfig *VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"backendUUID":    b.backendUUID,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("Attempting volume publish.")

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}
	// This is to ensure all backend volume mounting has occurred
	// FIXME(ameade): Should probably be renamed from createfollowup
	if err := b.driver.CreateFollowup(ctx, volConfig); err != nil {
		// TODO: Should this error be obfuscated to a more general error?
		return err
	}

	return b.driver.Publish(ctx, volConfig, publishInfo)
}

func (b *StorageBackend) UnpublishVolume(
	ctx context.Context, volConfig *VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"backendUUID":    b.backendUUID,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("Attempting volume unpublish.")

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}

	publishInfo.BackendUUID = b.backendUUID

	// Call the driver if it supports Unpublish, otherwise just return success as there is nothing to do
	if unpublisher, ok := b.driver.(Unpublisher); !ok {
		return nil
	} else {
		return unpublisher.Unpublish(ctx, volConfig, publishInfo)
	}
}

func (b *StorageBackend) GetVolumeForImport(ctx context.Context, volumeID string) (*VolumeExternal, error) {
	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	volExternal, err := b.driver.GetVolumeForImport(ctx, volumeID)
	if err != nil {
		return nil, fmt.Errorf("error seeking volume for import: %v", err)
	}
	volExternal.Backend = b.name
	volExternal.BackendUUID = b.backendUUID
	return volExternal, nil
}

func (b *StorageBackend) ImportVolume(ctx context.Context, volConfig *VolumeConfig) (*Volume, error) {
	Logc(ctx).WithFields(LogFields{
		"backend":    b.name,
		"volume":     volConfig.ImportOriginalName,
		"NotManaged": volConfig.ImportNotManaged,
	}).Debug("Backend#ImportVolume")

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	// Ensure provisioning is allowed
	if !b.isProvisioningAllowed() {
		return nil, fmt.Errorf("volume is not imported because the backend %s is suspended", b.name)
	}

	if volConfig.ImportNotManaged {
		// The volume is not managed and will not be renamed during import.
		volConfig.InternalName = volConfig.ImportOriginalName
	} else {
		// Sanitize the volume name
		b.driver.CreatePrepare(ctx, volConfig, nil)
	}

	err := b.driver.Import(ctx, volConfig, volConfig.ImportOriginalName)
	if err != nil {
		return nil, fmt.Errorf("driver import volume failed: %v", err)
	}

	// Ensure snapshot reserve is set to empty in case of volume import
	volConfig.SnapshotReserve = ""

	err = b.driver.CreateFollowup(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("failed post import volume operations : %v", err)
	}

	volume := NewVolume(volConfig, b.backendUUID, drivers.UnsetPool, false, VolumeStateOnline)
	b.volumes.Store(volume.Config.Name, volume)
	return volume, nil
}

func (b *StorageBackend) ResizeVolume(ctx context.Context, volConfig *VolumeConfig, newSize string) error {
	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return errors.NotManagedError("volume %s is not managed by Trident", volConfig.InternalName)
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return err
	}

	// Determine volume size in bytes
	requestedSize, err := capacity.ToBytes(newSize)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", newSize, err)
	}
	newSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", newSize, err)
	}

	Logc(ctx).WithFields(LogFields{
		"backend":     b.name,
		"volume":      volConfig.InternalName,
		"volume_size": newSizeBytes,
	}).Debug("Attempting volume resize.")
	return b.driver.Resize(ctx, volConfig, newSizeBytes)
}

func (b *StorageBackend) RenameVolume(ctx context.Context, volConfig *VolumeConfig, newName string) error {
	oldName := volConfig.InternalName

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return errors.NotManagedError("volume %s is not managed by Trident", oldName)
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return err
	}

	if err := b.driver.Get(ctx, oldName); err != nil {
		return fmt.Errorf("volume %s not found on backend %s; %v", oldName, b.name, err)
	}
	if err := b.driver.Rename(ctx, oldName, newName); err != nil {
		return fmt.Errorf("error attempting to rename volume %s on backend %s: %v", oldName, b.name, err)
	}
	return nil
}

func (b *StorageBackend) RemoveVolume(ctx context.Context, volConfig *VolumeConfig) error {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("Backend#RemoveVolume")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		b.RemoveCachedVolume(volConfig.Name)
		return errors.NotManagedError("volume %s is not managed by Trident", volConfig.InternalName)
	}

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}

	if err := b.driver.Destroy(ctx, volConfig); err != nil {
		// TODO:  Check the error being returned once the nDVP throws errors
		// for volumes that aren't found.
		return err
	}

	b.RemoveCachedVolume(volConfig.Name)
	return nil
}

func (b *StorageBackend) RemoveCachedVolume(volumeName string) {
	b.volumes.Delete(volumeName)
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (b *StorageBackend) CanSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error {
	return b.driver.CanSnapshot(ctx, snapConfig, volConfig)
}

func (b *StorageBackend) GetSnapshot(
	ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig,
) (*Snapshot, error) {
	fields := LogFields{
		"backend":              b.name,
		"volumeName":           snapConfig.VolumeName,
		"snapshotName":         snapConfig.Name,
		"snapshotInternalName": snapConfig.InternalName,
	}
	Logc(ctx).WithFields(fields).Debug("GetSnapshot.")

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	if snapshot, err := b.driver.GetSnapshot(ctx, snapConfig, volConfig); err != nil {
		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return nil, err
	} else if snapshot == nil {
		// No error and no snapshot means the snapshot doesn't exist.
		Logc(ctx).WithFields(fields).Debug("Snapshot not found.")

		return nil, errors.NotFoundError(
			fmt.Sprintf("snapshot %s of volume %s not found", snapConfig.Name, snapConfig.VolumeName),
		)
	} else {
		return snapshot, nil
	}
}

func (b *StorageBackend) GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error) {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("GetSnapshots.")

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	return b.driver.GetSnapshots(ctx, volConfig)
}

func (b *StorageBackend) CreateSnapshot(
	ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig,
) (*Snapshot, error) {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
		"snapshot":       snapConfig.Name,
	}).Debug("Attempting snapshot create.")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return nil, errors.NotManagedError("source volume %s is not managed by Trident", volConfig.InternalName)
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	// Set the default internal snapshot name to match the snapshot name.  Drivers
	// may override this value in the SnapshotConfig structure if necessary.
	snapConfig.InternalName = snapConfig.Name

	// Implement idempotency by checking for the snapshot first
	if existingSnapshot, err := b.driver.GetSnapshot(ctx, snapConfig, volConfig); err != nil {
		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return nil, err
	} else if existingSnapshot != nil {

		Logc(ctx).WithFields(LogFields{
			"backend":      b.name,
			"volumeName":   snapConfig.VolumeName,
			"snapshotName": snapConfig.Name,
		}).Warning("Snapshot already exists.")

		// Snapshot already exists, so just return it
		return existingSnapshot, nil
	}

	// Create snapshot
	return b.driver.CreateSnapshot(ctx, snapConfig, volConfig)
}

func (b *StorageBackend) RestoreSnapshot(
	ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig,
) error {
	Logc(ctx).WithFields(LogFields{
		"backend":        b.name,
		"volume":         snapConfig.Name,
		"volumeInternal": snapConfig.InternalName,
		"snapshot":       snapConfig.Name,
	}).Debug("Attempting snapshot restore.")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return errors.NotManagedError("source volume %s is not managed by Trident", volConfig.InternalName)
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return err
	}

	// Restore snapshot
	return b.driver.RestoreSnapshot(ctx, snapConfig, volConfig)
}

func (b *StorageBackend) DeleteSnapshot(
	ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig,
) error {
	Logc(ctx).WithFields(LogFields{
		"backend":          b.name,
		"volumeName":       snapConfig.VolumeName,
		"snapInternalName": snapConfig.InternalName,
		"snapshotName":     snapConfig.Name,
	}).Debug("Attempting snapshot delete.")

	// Ensure snapshot is managed
	if snapConfig.ImportNotManaged {
		return errors.NotManagedError("snapshot %s is not managed by Trident", snapConfig.InternalName)
	}

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return errors.NotManagedError("source volume %s is not managed by Trident", volConfig.InternalName)
	}

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}

	// Implement idempotency by checking for the snapshot first
	if existingSnapshot, err := b.driver.GetSnapshot(ctx, snapConfig, volConfig); err != nil {
		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return err
	} else if existingSnapshot == nil {

		Logc(ctx).WithFields(LogFields{
			"backend":      b.name,
			"volumeName":   snapConfig.VolumeName,
			"snapshotName": snapConfig.Name,
		}).Warning("Snapshot not found.")

		// Snapshot does not exist, so just return without error.
		return nil
	}

	// Delete snapshot
	return b.driver.DeleteSnapshot(ctx, snapConfig, volConfig)
}

func (b *StorageBackend) CanGroupSnapshot() bool {
	_, ok := b.driver.(GroupSnapshotter)
	return ok
}

func (b *StorageBackend) GetGroupSnapshotTarget(
	ctx context.Context, volConfigs []*VolumeConfig,
) (*GroupSnapshotTargetInfo, error) {
	snapshotter, ok := b.driver.(GroupSnapshotter)
	if !ok {
		return nil, errors.UnsupportedError(
			fmt.Sprintf("group snapshot is not supported for backend of type %v", b.driver.Name()))
	}

	return snapshotter.GetGroupSnapshotTarget(ctx, volConfigs)
}

func (b *StorageBackend) CreateGroupSnapshot(
	ctx context.Context, config *GroupSnapshotConfig, target *GroupSnapshotTargetInfo,
) error {
	snapshotter, ok := b.driver.(GroupSnapshotter)
	if !ok {
		return errors.UnsupportedError(
			fmt.Sprintf("group snapshot is not supported for backend of type %v", b.driver.Name()))
	}

	return snapshotter.CreateGroupSnapshot(ctx, config, target)
}

func (b *StorageBackend) ProcessGroupSnapshot(
	ctx context.Context, config *GroupSnapshotConfig, volConfigs []*VolumeConfig,
) ([]*Snapshot, error) {
	snapshotter, ok := b.driver.(GroupSnapshotter)
	if !ok {
		return nil, errors.UnsupportedError(
			fmt.Sprintf("group snapshot is not supported for backend of type %v", b.driver.Name()))
	}

	return snapshotter.ProcessGroupSnapshot(ctx, config, volConfigs)
}

func (b *StorageBackend) ConstructGroupSnapshot(
	ctx context.Context, config *GroupSnapshotConfig, snapshots []*Snapshot,
) (*GroupSnapshot, error) {
	snapshotter, ok := b.driver.(GroupSnapshotter)
	if !ok {
		return nil, errors.UnsupportedError(
			fmt.Sprintf("group snapshot is not supported for backend of type %v", b.driver.Name()))
	}

	return snapshotter.ConstructGroupSnapshot(ctx, config, snapshots)
}

const (
	BackendRename = iota
	InvalidVolumeAccessInfoChange
	InvalidUpdate
	UsernameChange
	PasswordChange
	PrefixChange
	CredentialsChange
)

const (
	BackendStateReasonChange = iota
	BackendStatePoolsChange
	BackendStateAPIVersionChange
)

func (b *StorageBackend) GetUpdateType(ctx context.Context, origBackend Backend) *roaring.Bitmap {
	updateCode := b.driver.GetUpdateType(ctx, origBackend.Driver())
	if b.name != origBackend.Name() {
		updateCode.Add(BackendRename)
	}
	return updateCode
}

// HasVolumes returns true if the Backend has one or more volumes
// provisioned on it.
func (b *StorageBackend) HasVolumes() bool {
	count := 0
	b.volumes.Range(func(_, _ interface{}) bool {
		count++
		return false
	})
	return count > 0
}

// Terminate informs the backend that it is being deleted from the core
// and will not be called again.  This may be a signal to the storage
// driver to clean up and stop any ongoing operations.
func (b *StorageBackend) Terminate(ctx context.Context) {
	b.stateLock.RLock()
	logFields := LogFields{
		"backend":     b.name,
		"backendUUID": b.backendUUID,
		"driver":      b.GetDriverName(),
		"state":       string(b.state),
	}
	b.stateLock.RUnlock()

	if !b.driver.Initialized() {
		Logc(ctx).WithFields(logFields).Warning("Cannot terminate an uninitialized backend.")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Terminating backend.")
		b.driver.Terminate(ctx, b.backendUUID)
	}
}

// InvalidateNodeAccess marks the backend as needing the node access rule reconciled
func (b *StorageBackend) InvalidateNodeAccess() {
	b.stateLock.Lock()
	defer b.stateLock.Unlock()
	b.nodeAccessUpToDate = false
}

// SetNodeAccessUpToDate marks the backend as node access up to date
func (b *StorageBackend) SetNodeAccessUpToDate() {
	b.stateLock.Lock()
	defer b.stateLock.Unlock()
	b.nodeAccessUpToDate = true
}

// IsNodeAccessUpToDate returns true if the backend's node access rules are up to date
func (b *StorageBackend) IsNodeAccessUpToDate() bool {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()
	return b.nodeAccessUpToDate
}

// ReconcileNodeAccess will ensure that the driver only has allowed access
// to its volumes from active nodes in the k8s cluster. This is usually
// handled via export policies or initiators
func (b *StorageBackend) ReconcileNodeAccess(
	ctx context.Context, nodes []*models.Node, tridentUUID string,
) error {
	if err := b.ensureOnlineOrDeleting(ctx); err == nil {
		// Only reconcile backends that need it
		if b.IsNodeAccessUpToDate() {
			Logc(ctx).WithField("backend", b.name).Trace("Backend node access rules are already up-to-date, skipping.")
			return nil
		}
		Logc(ctx).WithField("backend", b.name).Trace("Backend node access rules are out-of-date, updating.")
		err = b.driver.ReconcileNodeAccess(ctx, nodes, b.backendUUID, tridentUUID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *StorageBackend) ReconcileVolumeNodeAccess(
	ctx context.Context, volConfig *VolumeConfig, nodes []*models.Node,
) error {
	if err := b.ensureOnlineOrDeleting(ctx); err == nil {
		// Only reconcile backends that need it
		if b.IsNodeAccessUpToDate() {
			Logc(ctx).WithField("backend", b.name).Trace("Backend volume node access rules are already up-to-date, skipping.")
			return nil
		}
		Logc(ctx).WithField("backend", b.name).Trace("Backend volume node access rules are out-of-date, updating.")
		err = b.driver.ReconcileVolumeNodeAccess(ctx, volConfig, nodes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *StorageBackend) CanGetState() bool {
	_, ok := b.driver.(StateGetter)
	return ok
}

// GetBackendState gets the up-to-date state of SVM and updates associated Trident's data structures.
func (b *StorageBackend) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	stateDriver, ok := b.driver.(StateGetter)
	if !ok {
		Logc(ctx).WithError(errors.UnsupportedError(
			fmt.Sprintf("polling is not implemented by backends of type %v", b.driver.Name())))
		return "", nil
	}

	reason, changeMap := stateDriver.GetBackendState(ctx)

	if reason != b.StateReason() || (reason == "" && !b.Online()) {
		// Inform caller that update is needed as there is change in either state or StateReason.
		// Being defensive here, as changeMap could never be nil.
		if changeMap == nil {
			changeMap = roaring.New()
		}
		changeMap.Add(BackendStateReasonChange)
	}

	return reason, changeMap
}

// UpdateBackendState updates the backend state and state reason without polling the storage system.  It
// is expected to be called with the results from a call to GetBackendState after acquiring the necessary locks.
func (b *StorageBackend) UpdateBackendState(_ context.Context, stateReason string) {
	b.stateLock.Lock()
	defer b.stateLock.Unlock()

	if stateReason == "" {
		b.state = Online
		b.online = true
	} else {
		b.state = Offline
		b.online = false
	}
	b.stateReason = stateReason
}

func (b *StorageBackend) ensureOnline(ctx context.Context) error {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	if b.state != Online {
		Logc(ctx).WithFields(LogFields{
			"state":         b.state,
			"expectedState": string(Online),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online", b.name)
	}
	return nil
}

func (b *StorageBackend) ensureOnlineOrDeleting(ctx context.Context) error {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	if b.state != Online && b.state != Deleting {
		Logc(ctx).WithFields(LogFields{
			"state":         b.state,
			"expectedState": string(Online) + "/" + string(Deleting),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online or Deleting", b.name)
	}
	return nil
}

func (b *StorageBackend) isProvisioningAllowed() bool {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	return b.userState != UserSuspended
}

type BackendExternal struct {
	Name        string                 `json:"name"`
	BackendUUID string                 `json:"backendUUID"`
	Protocol    tridentconfig.Protocol `json:"protocol"`
	Config      interface{}            `json:"config"`
	Storage     map[string]interface{} `json:"storage"`
	State       BackendState           `json:"state"`
	UserState   UserBackendState       `json:"userState"`
	Online      bool                   `json:"online"`
	StateReason string                 `json:"StateReason"`
	Volumes     []string               `json:"volumes"`
	ConfigRef   string                 `json:"configRef"`
}

func (b *StorageBackend) ConstructExternal(ctx context.Context) *BackendExternal {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	backendExternal := BackendExternal{
		Name:        b.name,
		BackendUUID: b.backendUUID,
		Protocol:    b.GetProtocol(ctx),
		Config:      b.driver.GetExternalConfig(ctx),
		Storage:     make(map[string]interface{}),
		Online:      b.online,
		State:       b.state,
		UserState:   b.userState,
		StateReason: b.stateReason,
		Volumes:     make([]string, 0),
		ConfigRef:   b.configRef,
	}

	b.storagePools.Range(func(k, v interface{}) bool {
		backendExternal.Storage[k.(string)] = v.(*StoragePool).ConstructExternal()
		return true
	})
	b.volumes.Range(func(k, v interface{}) bool {
		backendExternal.Volumes = append(backendExternal.Volumes, k.(string))
		return true
	})

	return &backendExternal
}

// ConstructExternalWithPoolMap returns the external form of a backend.  The storage class information
// is passed in as a map of pool names to storage classes.
func (b *StorageBackend) ConstructExternalWithPoolMap(
	ctx context.Context, poolMap map[string][]string,
) *BackendExternal {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	backendExternal := BackendExternal{
		Name:        b.name,
		BackendUUID: b.backendUUID,
		Protocol:    b.GetProtocol(ctx),
		Config:      b.driver.GetExternalConfig(ctx),
		Storage:     make(map[string]interface{}),
		Online:      b.online,
		State:       b.state,
		UserState:   b.userState,
		StateReason: b.stateReason,
		Volumes:     make([]string, 0),
		ConfigRef:   b.configRef,
	}

	b.storagePools.Range(func(k, v interface{}) bool {
		backendExternal.Storage[k.(string)] = v.(*StoragePool).ConstructExternalWithPoolMap(poolMap)
		return true
	})
	b.volumes.Range(func(k, v interface{}) bool {
		backendExternal.Volumes = append(backendExternal.Volumes, k.(string))
		return true
	})

	return &backendExternal
}

// Used to store the requisite info for a backend in the persistent store.  Other than
// the configuration, all other data will be reconstructed during the bootstrap phase.

type PersistentStorageBackendConfig struct {
	OntapConfig             *drivers.OntapStorageDriverConfig     `json:"ontap_config,omitempty"`
	SolidfireConfig         *drivers.SolidfireStorageDriverConfig `json:"solidfire_config,omitempty"`
	AzureConfig             *drivers.AzureNASStorageDriverConfig  `json:"azure_config,omitempty"`
	GCPConfig               *drivers.GCPNFSStorageDriverConfig    `json:"gcp_config,omitempty"`
	GCNVConfig              *drivers.GCNVNASStorageDriverConfig   `json:"gcnv_config,omitempty"`
	FakeStorageDriverConfig *drivers.FakeStorageDriverConfig      `json:"fake_config,omitempty"`
}

func (psbc *PersistentStorageBackendConfig) GetDriverConfig() (drivers.DriverConfig, error) {
	var driverConfig drivers.DriverConfig

	switch {
	case psbc.OntapConfig != nil:
		driverConfig = psbc.OntapConfig
	case psbc.SolidfireConfig != nil:
		driverConfig = psbc.SolidfireConfig
	case psbc.AzureConfig != nil:
		driverConfig = psbc.AzureConfig
	case psbc.GCPConfig != nil:
		driverConfig = psbc.GCPConfig
	case psbc.GCNVConfig != nil:
		driverConfig = psbc.GCNVConfig
	case psbc.FakeStorageDriverConfig != nil:
		driverConfig = psbc.FakeStorageDriverConfig
	default:
		return nil, errors.New("unknown backend type")
	}

	return driverConfig, nil
}

type BackendPersistent struct {
	Version     string                         `json:"version"`
	Config      PersistentStorageBackendConfig `json:"config"`
	Name        string                         `json:"name"`
	BackendUUID string                         `json:"backendUUID"`
	Online      bool                           `json:"online"`
	State       BackendState                   `json:"state"`
	UserState   UserBackendState               `json:"userState"`
	StateReason string                         `json:"stateReason"`
	ConfigRef   string                         `json:"configRef"`
}

func (b *StorageBackend) ConstructPersistent(ctx context.Context) *BackendPersistent {
	b.stateLock.RLock()
	defer b.stateLock.RUnlock()

	persistentBackend := &BackendPersistent{
		Version:     tridentconfig.OrchestratorAPIVersion,
		Config:      PersistentStorageBackendConfig{},
		Name:        b.name,
		Online:      b.online,
		State:       b.state,
		UserState:   b.userState,
		StateReason: b.stateReason,
		BackendUUID: b.backendUUID,
		ConfigRef:   b.configRef,
	}
	b.driver.StoreConfig(ctx, &persistentBackend.Config)
	return persistentBackend
}

// MarshalConfig returns a persistent backend config as JSON.
// Unfortunately, this method appears to be necessary to avoid arbitrary values
// ending up in the json.RawMessage fields of CommonStorageDriverConfig.
// Ideally, BackendPersistent would just store a serialized config, but
// doing so appears to cause problems with the json.RawMessage fields.
func (p *BackendPersistent) MarshalConfig() (string, error) {
	var (
		bytes []byte
		err   error
	)
	switch {
	case p.Config.OntapConfig != nil:
		bytes, err = json.Marshal(p.Config.OntapConfig)
	case p.Config.SolidfireConfig != nil:
		bytes, err = json.Marshal(p.Config.SolidfireConfig)
	case p.Config.AzureConfig != nil:
		bytes, err = json.Marshal(p.Config.AzureConfig)
	case p.Config.GCPConfig != nil:
		bytes, err = json.Marshal(p.Config.GCPConfig)
	case p.Config.GCNVConfig != nil:
		bytes, err = json.Marshal(p.Config.GCNVConfig)
	case p.Config.FakeStorageDriverConfig != nil:
		bytes, err = json.Marshal(p.Config.FakeStorageDriverConfig)
	default:
		return "", fmt.Errorf("no recognized config found for backend %s", p.Name)
	}
	if err != nil {
		return "", err
	}
	return string(bytes), err
}

// GetBackendCredentials identifies the storage driver and returns the credentials field name and type (if set)
func (p *BackendPersistent) GetBackendCredentials() (string, string, error) {
	driverConfig, err := p.Config.GetDriverConfig()
	if err != nil {
		return "", "", fmt.Errorf("cannot get credentials: %v", err)
	}

	return driverConfig.GetCredentials()
}

// ExtractBackendSecrets clones itself (a BackendPersistent struct), identified if the backend is using
// trident created secret (tbe-<backendUUID>) or user-provided secret (via credentials field),
// if these values are same or not and accordingly sets usingTridentSecretName boolean field.
// From the clone it builds a map of secret data it contains (username, password, etc.),
// replaces those fields with the correct secret name, and returns the clone,
// the secret data map (or empty map if using credentials field) and usingTridentSecretName field.
func (p *BackendPersistent) ExtractBackendSecrets(
	secretName string,
) (*BackendPersistent, map[string]string, bool, error) {
	var secretType string
	var credentialsFieldSet, usingTridentSecretName bool

	backend, err := deep.Copy(*p)
	if err != nil {
		return nil, nil, usingTridentSecretName, err
	}

	// Check if user-provided credentials field is set
	if backendSecretName, backendSecretType, err := p.GetBackendCredentials(); err != nil {
		Log().Errorf("Could not determine if backend credentials field exist; %v", err)
		return nil, nil, usingTridentSecretName, err
	} else if backendSecretName != "" {
		if backendSecretName == secretName {
			usingTridentSecretName = true
		}

		secretName = backendSecretName
		secretType = backendSecretType
		credentialsFieldSet = true
	}

	if secretType == "" {
		secretType = "secret"
	}

	secretName = fmt.Sprintf("%s:%s", secretType, secretName)

	driverConfig, err := backend.Config.GetDriverConfig()
	if err != nil {
		return nil, nil, usingTridentSecretName, fmt.Errorf("cannot extract secrets: %v", err)
	}

	secretMap := driverConfig.GetAndHideSensitive(secretName)

	if credentialsFieldSet {
		return &backend, nil, usingTridentSecretName, nil
	}

	return &backend, secretMap, usingTridentSecretName, nil
}

func (p *BackendPersistent) InjectBackendSecrets(secretMap map[string]string) error {
	driverConfig, err := p.Config.GetDriverConfig()
	if err != nil {
		return fmt.Errorf("cannot inject secrets: %v", err)
	}

	// After secret has been extracted, reset credentials
	driverConfig.ResetSecrets()

	return driverConfig.InjectSecrets(secretMap)
}

func (b *StorageBackend) EstablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string,
) error {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return errors.UnsupportedError(
			fmt.Sprintf("mirroring is not implemented by backends of type %v", b.driver.Name()))
	}

	return mirrorDriver.EstablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule)
}

func (b *StorageBackend) PromoteMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotHandle string,
) (bool, error) {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return false, errors.UnsupportedError(
			fmt.Sprintf("mirroring is not implemented by backends of type %v", b.driver.Name()))
	}

	return mirrorDriver.PromoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotHandle)
}

func (b *StorageBackend) ReestablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle,
	replicationPolicy, replicationSchedule string,
) error {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return errors.UnsupportedError(
			fmt.Sprintf("mirroring is not implemented by backends of type %v", b.driver.Name()))
	}

	return mirrorDriver.ReestablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule)
}

func (b *StorageBackend) GetMirrorStatus(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, error) {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return "", errors.UnsupportedError(fmt.Sprintf(
			"mirroring is not implemented by backends of type %v", b.driver.Name()))
	}
	return mirrorDriver.GetMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle)
}

func (b *StorageBackend) CanMirror() bool {
	_, ok := b.driver.(Mirrorer)
	return ok
}

func (b *StorageBackend) ReleaseMirror(ctx context.Context, localInternalVolumeName string) error {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return errors.UnsupportedError(
			fmt.Sprintf("mirroring is not implemented by backends of type %v", b.driver.Name()))
	}

	return mirrorDriver.ReleaseMirror(ctx, localInternalVolumeName)
}

func (b *StorageBackend) GetReplicationDetails(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, string, string, error) {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return "", "", "", errors.UnsupportedError(fmt.Sprintf(
			"mirroring is not implemented by backends of type %v", b.driver.Name()))
	}
	return mirrorDriver.GetReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle)
}

func (b *StorageBackend) UpdateMirror(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return errors.UnsupportedError(fmt.Sprintf(
			"mirroring is not implemented by backends of type %v", b.driver.Name()))
	}
	return mirrorDriver.UpdateMirror(ctx, localInternalVolumeName, snapshotName)
}

func (b *StorageBackend) CheckMirrorTransferState(ctx context.Context, localInternalVolumeName string) (*time.Time, error) {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return nil, errors.UnsupportedError(fmt.Sprintf(
			"mirroring is not implemented by backends of type %v", b.driver.Name()))
	}
	return mirrorDriver.CheckMirrorTransferState(ctx, localInternalVolumeName)
}

func (b *StorageBackend) GetMirrorTransferTime(ctx context.Context, localInternalVolumeName string) (*time.Time,
	error,
) {
	mirrorDriver, ok := b.driver.(Mirrorer)
	if !ok {
		return nil, errors.UnsupportedError(fmt.Sprintf(
			"mirroring is not implemented by backends of type %v", b.driver.Name()))
	}
	return mirrorDriver.GetMirrorTransferTime(ctx, localInternalVolumeName)
}

func (b *StorageBackend) GetChapInfo(ctx context.Context, volumeName, nodeName string) (*models.IscsiChapInfo, error) {
	chapEnabledDriver, ok := b.driver.(ChapEnabled)
	if !ok {
		return nil, errors.UnsupportedError(fmt.Sprintf(
			"retrieving chap credentials is not supported on backends of type %v", b.driver.Name()))
	}
	return chapEnabledDriver.GetChapInfo(ctx, volumeName, nodeName)
}

func (b *StorageBackend) EnablePublishEnforcement(ctx context.Context, volume *Volume) error {
	driver, ok := b.driver.(PublishEnforceable)
	if !ok {
		return errors.UnsupportedError(fmt.Sprintf(
			"publish enforcement is not supported on backends of type %v", b.driver.Name()))
	}
	return driver.EnablePublishEnforcement(ctx, volume)
}

func (b *StorageBackend) CanEnablePublishEnforcement() bool {
	_, ok := b.driver.(PublishEnforceable)
	return ok
}

// SmartCopy implements a shallow copy of StorageBackend because it satisfies interior mutability. This means the volume
// and pool maps, and the driver, are shared between all copies of the StorageBackend.
func (b *StorageBackend) SmartCopy() interface{} {
	cpy := *b
	return &cpy
}

func (b *StorageBackend) DeepCopyType() Backend {
	cpy := *b
	return &cpy
}

func (b *StorageBackend) GetUniqueKey() string {
	return b.name
}
