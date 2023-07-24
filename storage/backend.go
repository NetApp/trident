// Copyright 2023 NetApp, Inc. All Rights Reserved.

package storage

//go:generate mockgen -destination=../mocks/mock_storage/mock_storage_driver.go github.com/netapp/trident/storage Driver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff/v4"
	"github.com/mitchellh/copystructure"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
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
	CreatePrepare(ctx context.Context, volConfig *VolumeConfig)
	// CreateFollowup adds necessary information for accessing the volume to VolumeConfig.
	CreateFollowup(ctx context.Context, volConfig *VolumeConfig) error
	CreateClone(ctx context.Context, sourceVolConfig, cloneVolConfig *VolumeConfig, storagePool Pool) error
	Import(ctx context.Context, volConfig *VolumeConfig, originalName string) error
	Destroy(ctx context.Context, volConfig *VolumeConfig) error
	Rename(ctx context.Context, name, newName string) error
	Resize(ctx context.Context, volConfig *VolumeConfig, sizeBytes uint64) error
	Get(ctx context.Context, name string) error
	GetInternalVolumeName(ctx context.Context, name string) string
	GetStorageBackendSpecs(ctx context.Context, backend Backend) error
	GetStorageBackendPhysicalPoolNames(ctx context.Context) []string
	GetProtocol(ctx context.Context) tridentconfig.Protocol
	Publish(ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo) error
	CanSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	GetSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error)
	CreateSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) (*Snapshot, error)
	RestoreSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	DeleteSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error
	StoreConfig(ctx context.Context, b *PersistentStorageBackendConfig)
	// GetExternalConfig returns a version of the driver configuration that
	// lacks confidential information, such as usernames and passwords.
	GetExternalConfig(ctx context.Context) interface{}
	// GetVolumeExternal accepts the internal name of a volume and returns a VolumeExternal
	// object.  This method is only available if using the passthrough store (i.e. Docker).
	GetVolumeExternal(ctx context.Context, name string) (*VolumeExternal, error)
	// GetVolumeExternalWrappers reads all volumes owned by this driver from the storage backend and
	// writes them to the supplied channel as VolumeExternalWrapper objects.  This method is only
	// available if using the passthrough store (i.e. Docker).
	GetVolumeExternalWrappers(context.Context, chan *VolumeExternalWrapper)
	GetUpdateType(ctx context.Context, driver Driver) *roaring.Bitmap
	ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, backendUUID, tridentUUID string) error
	GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig
}

type Unpublisher interface {
	Unpublish(ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo) error
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

// StateGetter provides a common interface for backends that support polling backend for state information.
type StateGetter interface {
	GetBackendState(ctx context.Context) (string, *roaring.Bitmap)
}

type StorageBackend struct {
	driver             Driver
	name               string
	backendUUID        string
	online             bool
	state              BackendState
	stateReason        string
	storage            map[string]Pool
	volumes            map[string]*Volume
	configRef          string
	nodeAccessUpToDate bool
}

func (b *StorageBackend) Driver() Driver {
	return b.driver
}

func (b *StorageBackend) SetDriver(Driver Driver) {
	b.driver = Driver
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
	return b.online
}

func (b *StorageBackend) SetOnline(Online bool) {
	b.online = Online
}

func (b *StorageBackend) State() BackendState {
	return b.state
}

func (b *StorageBackend) StateReason() string {
	return b.stateReason
}

// SetState sets the 'state' and 'online' fields of StorageBackend accordingly.
func (b *StorageBackend) SetState(state BackendState) {
	b.state = state
	if state.IsOnline() {
		b.online = true
	} else {
		b.online = false
	}
}

func (b *StorageBackend) Storage() map[string]Pool {
	return b.storage
}

func (b *StorageBackend) SetStorage(Storage map[string]Pool) {
	b.storage = Storage
}

func (b *StorageBackend) Volumes() map[string]*Volume {
	return b.volumes
}

func (b *StorageBackend) SetVolumes(Volumes map[string]*Volume) {
	b.volumes = Volumes
}

func (b *StorageBackend) ConfigRef() string {
	return b.configRef
}

func (b *StorageBackend) SetConfigRef(ConfigRef string) {
	b.configRef = ConfigRef
}

type UpdateBackendStateRequest struct {
	State string `json:"state"`
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

func NewStorageBackend(ctx context.Context, driver Driver) (*StorageBackend, error) {
	backend := StorageBackend{
		driver:  driver,
		state:   Online,
		online:  true,
		storage: make(map[string]Pool),
		volumes: make(map[string]*Volume),
	}

	// retrieve backend specs
	if err := backend.Driver().GetStorageBackendSpecs(ctx, &backend); err != nil {
		return nil, err
	}

	return &backend, nil
}

func NewFailedStorageBackend(ctx context.Context, driver Driver) Backend {
	backend := StorageBackend{
		name:    driver.BackendName(),
		driver:  driver,
		state:   Failed,
		storage: make(map[string]Pool),
		volumes: make(map[string]*Volume),
	}

	Logc(ctx).WithFields(LogFields{
		"backendUUID": backend.BackendUUID(),
		"backendName": backend.Name(),
		"driver":      driver.Name(),
	}).Debug("Failed storage backend.")

	return &backend
}

func (b *StorageBackend) AddStoragePool(pool Pool) {
	b.storage[pool.Name()] = pool
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
	b.volumes[vol.Config.Name] = vol
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
	Logc(ctx).WithFields(LogFields{
		"backend":                cloneVolConfig.Name,
		"backendUUID":            b.backendUUID,
		"storageClass":           cloneVolConfig.StorageClass,
		"sourceVolume":           cloneVolConfig.CloneSourceVolume,
		"sourceVolumeInternal":   cloneVolConfig.CloneSourceVolumeInternal,
		"sourceSnapshot":         cloneVolConfig.CloneSourceSnapshot,
		"sourceSnapshotInternal": cloneVolConfig.CloneSourceSnapshotInternal,
		"cloneVolume":            cloneVolConfig.Name,
		"cloneVolumeInternal":    cloneVolConfig.InternalName,
	}).Debug("Attempting volume clone.")

	if cloneVolConfig.ReadOnlyClone {
		if tridentconfig.DisableExtraFeatures {
			return nil, errors.UnsupportedError("read only clone is not supported")
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
				float64(cloneBackoff.MaxElapsedTime))
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
	b.volumes[vol.Config.Name] = vol
	return vol, nil
}

func (b *StorageBackend) PublishVolume(
	ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo,
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
	ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo,
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

func (b *StorageBackend) GetVolumeExternal(ctx context.Context, volumeName string) (*VolumeExternal, error) {
	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	if err := b.driver.Get(ctx, volumeName); err != nil {
		return nil, fmt.Errorf("failed to get volume %s: %v", volumeName, err)
	}

	volExternal, err := b.driver.GetVolumeExternal(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("error requesting volume size: %v", err)
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

	if volConfig.ImportNotManaged {
		// The volume is not managed and will not be renamed during import.
		volConfig.InternalName = volConfig.ImportOriginalName
	} else {
		// Sanitize the volume name
		b.driver.CreatePrepare(ctx, volConfig)
	}

	err := b.driver.Import(ctx, volConfig, volConfig.ImportOriginalName)
	if err != nil {
		return nil, fmt.Errorf("driver import volume failed: %v", err)
	}

	err = b.driver.CreateFollowup(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("failed post import volume operations : %v", err)
	}

	volume := NewVolume(volConfig, b.backendUUID, drivers.UnsetPool, false, VolumeStateOnline)
	b.volumes[volume.Config.Name] = volume
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
	requestedSize, err := utils.ConvertSizeToBytes(newSize)
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

	if b.state != Online {
		Logc(ctx).WithFields(LogFields{
			"state":         b.state,
			"expectedState": string(Online),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online", b.name)
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

	// If it's a RO clone, no need to delete the volume in the driver
	if !volConfig.ReadOnlyClone {
		if err := b.driver.Destroy(ctx, volConfig); err != nil {
			// TODO:  Check the error being returned once the nDVP throws errors
			// for volumes that aren't found.
			return err
		}
	}

	b.RemoveCachedVolume(volConfig.Name)
	return nil
}

func (b *StorageBackend) RemoveCachedVolume(volumeName string) {
	delete(b.volumes, volumeName)
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
		"volume":         snapConfig.Name,
		"volumeInternal": snapConfig.InternalName,
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
	return len(b.volumes) > 0
}

// Terminate informs the backend that it is being deleted from the core
// and will not be called again.  This may be a signal to the storage
// driver to clean up and stop any ongoing operations.
func (b *StorageBackend) Terminate(ctx context.Context) {
	logFields := LogFields{
		"backend":     b.name,
		"backendUUID": b.backendUUID,
		"driver":      b.GetDriverName(),
		"state":       string(b.state),
	}

	if !b.driver.Initialized() {
		Logc(ctx).WithFields(logFields).Warning("Cannot terminate an uninitialized backend.")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Terminating backend.")
		b.driver.Terminate(ctx, b.backendUUID)
	}
}

// InvalidateNodeAccess marks the backend as needing the node access rule reconciled
func (b *StorageBackend) InvalidateNodeAccess() {
	b.nodeAccessUpToDate = false
}

// ReconcileNodeAccess will ensure that the driver only has allowed access
// to its volumes from active nodes in the k8s cluster. This is usually
// handled via export policies or initiators
func (b *StorageBackend) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, tridentUUID string) error {
	if err := b.ensureOnlineOrDeleting(ctx); err == nil {
		// Only reconcile backends that need it
		if b.nodeAccessUpToDate {
			Logc(ctx).WithField("backend", b.name).Trace("Backend node access rules are already up-to-date, skipping.")
			return nil
		}
		Logc(ctx).WithField("backend", b.name).Trace("Backend node access rules are out-of-date, updating.")
		err = b.driver.ReconcileNodeAccess(ctx, nodes, b.backendUUID, tridentUUID)
		if err == nil {
			b.nodeAccessUpToDate = true
		}
		return err
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

	if reason == "" {
		b.state = Online
		b.online = true
	} else {
		b.state = Offline
		b.online = false
	}
	b.stateReason = reason

	return reason, changeMap
}

func (b *StorageBackend) ensureOnline(ctx context.Context) error {
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
	if b.state != Online && b.state != Deleting {
		Logc(ctx).WithFields(LogFields{
			"state":         b.state,
			"expectedState": string(Online) + "/" + string(Deleting),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online or Deleting", b.name)
	}
	return nil
}

type BackendExternal struct {
	Name        string                 `json:"name"`
	BackendUUID string                 `json:"backendUUID"`
	Protocol    tridentconfig.Protocol `json:"protocol"`
	Config      interface{}            `json:"config"`
	Storage     map[string]interface{} `json:"storage"`
	State       BackendState           `json:"state"`
	Online      bool                   `json:"online"`
	StateReason string                 `json:"StateReason"`
	Volumes     []string               `json:"volumes"`
	ConfigRef   string                 `json:"configRef"`
}

func (b *StorageBackend) ConstructExternal(ctx context.Context) *BackendExternal {
	backendExternal := BackendExternal{
		Name:        b.name,
		BackendUUID: b.backendUUID,
		Protocol:    b.GetProtocol(ctx),
		Config:      b.driver.GetExternalConfig(ctx),
		Storage:     make(map[string]interface{}),
		Online:      b.online,
		State:       b.state,
		StateReason: b.stateReason,
		Volumes:     make([]string, 0),
		ConfigRef:   b.configRef,
	}

	for name, pool := range b.storage {
		backendExternal.Storage[name] = pool.ConstructExternal()
	}
	for volName := range b.volumes {
		backendExternal.Volumes = append(backendExternal.Volumes, volName)
	}

	return &backendExternal
}

// Used to store the requisite info for a backend in the persistent store.  Other than
// the configuration, all other data will be reconstructed during the bootstrap phase.

type PersistentStorageBackendConfig struct {
	OntapConfig             *drivers.OntapStorageDriverConfig     `json:"ontap_config,omitempty"`
	SolidfireConfig         *drivers.SolidfireStorageDriverConfig `json:"solidfire_config,omitempty"`
	AzureConfig             *drivers.AzureNASStorageDriverConfig  `json:"azure_config,omitempty"`
	GCPConfig               *drivers.GCPNFSStorageDriverConfig    `json:"gcp_config,omitempty"`
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
	StateReason string                         `json:"stateReason"`
	ConfigRef   string                         `json:"configRef"`
}

func (b *StorageBackend) ConstructPersistent(ctx context.Context) *BackendPersistent {
	persistentBackend := &BackendPersistent{
		Version:     tridentconfig.OrchestratorAPIVersion,
		Config:      PersistentStorageBackendConfig{},
		Name:        b.name,
		Online:      b.online,
		State:       b.state,
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

	clone, err := copystructure.Copy(*p)
	if err != nil {
		return nil, nil, usingTridentSecretName, err
	}

	backend, ok := clone.(BackendPersistent)
	if !ok {
		return nil, nil, usingTridentSecretName, err
	}

	// Check if user-provided credentials field is set
	if backendSecretName, backendSecretType, err := p.GetBackendCredentials(); err != nil {
		Log().Errorf("Could not determined if backend credentials field exist; %v", err)
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

func (b *StorageBackend) GetChapInfo(ctx context.Context, volumeName, nodeName string) (*utils.IscsiChapInfo, error) {
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
