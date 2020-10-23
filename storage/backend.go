// Copyright 2020 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff/v4"
	"github.com/mitchellh/copystructure"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

// Driver provides a common interface for storage related operations
type Driver interface {
	Name() string
	Initialize(context.Context, tridentconfig.DriverContext, string, *drivers.CommonStorageDriverConfig) error
	Initialized() bool
	// Terminate tells the driver to clean up, as it won't be called again.
	Terminate(ctx context.Context, backendUUID string)
	Create(ctx context.Context, volConfig *VolumeConfig, storagePool *Pool, volAttributes map[string]sa.Request) error
	CreatePrepare(ctx context.Context, volConfig *VolumeConfig)
	// CreateFollowup adds necessary information for accessing the volume to VolumeConfig.
	CreateFollowup(ctx context.Context, volConfig *VolumeConfig) error
	// GetInternalVolumeName will return a name that satisfies any character
	// constraints present on the backend and that will be unique to Trident.
	// The latter requirement should generally be done by prepending the
	// value of CommonStorageDriver.SnapshotPrefix to the name.
	CreateClone(ctx context.Context, volConfig *VolumeConfig, storagePool *Pool) error
	Import(ctx context.Context, volConfig *VolumeConfig, originalName string) error
	Destroy(ctx context.Context, name string) error
	Rename(ctx context.Context, name, newName string) error
	Resize(ctx context.Context, volConfig *VolumeConfig, sizeBytes uint64) error
	Get(ctx context.Context, name string) error
	GetInternalVolumeName(ctx context.Context, name string) string
	GetStorageBackendSpecs(ctx context.Context, backend *Backend) error
	GetStorageBackendPhysicalPoolNames(ctx context.Context) []string
	GetProtocol(ctx context.Context) tridentconfig.Protocol
	Publish(ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo) error
	GetSnapshot(ctx context.Context, snapConfig *SnapshotConfig) (*Snapshot, error)
	GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error)
	CreateSnapshot(ctx context.Context, snapConfig *SnapshotConfig) (*Snapshot, error)
	RestoreSnapshot(ctx context.Context, snapConfig *SnapshotConfig) error
	DeleteSnapshot(ctx context.Context, snapConfig *SnapshotConfig) error
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
	ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, backendUUID string) error
}

type Backend struct {
	Driver      Driver
	Name        string
	BackendUUID string
	Online      bool
	State       BackendState
	Storage     map[string]*Pool
	Volumes     map[string]*Volume
}

type UpdateBackendStateRequest struct {
	State string `json:"state"`
}

type NotManagedError struct {
	volumeName string
}

func (e *NotManagedError) Error() string {
	return fmt.Sprintf("volume %s is not managed by Trident", e.volumeName)
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

func NewStorageBackend(ctx context.Context, driver Driver) (*Backend, error) {
	backend := Backend{
		Driver:  driver,
		State:   Online,
		Online:  true,
		Storage: make(map[string]*Pool),
		Volumes: make(map[string]*Volume),
	}

	// retrieve backend specs
	if err := backend.Driver.GetStorageBackendSpecs(ctx, &backend); err != nil {
		return nil, err
	}

	return &backend, nil
}

func NewFailedStorageBackend(ctx context.Context, driver Driver) *Backend {

	backend := Backend{
		Driver:  driver,
		State:   Failed,
		Storage: make(map[string]*Pool),
		Volumes: make(map[string]*Volume),
	}

	Logc(ctx).WithFields(log.Fields{
		"backendUUID": backend.BackendUUID,
		"backendName": backend.Name,
		"driver":      driver.Name(),
	}).Debug("Failed storage backend.")

	return &backend
}

func (b *Backend) AddStoragePool(pool *Pool) {
	b.Storage[pool.Name] = pool
}

func (b *Backend) GetPhysicalPoolNames(ctx context.Context) []string {
	return b.Driver.GetStorageBackendPhysicalPoolNames(ctx)
}

func (b *Backend) GetDriverName() string {
	return b.Driver.Name()
}

func (b *Backend) GetProtocol(ctx context.Context) tridentconfig.Protocol {
	return b.Driver.GetProtocol(ctx)
}

func (b *Backend) AddVolume(
	ctx context.Context, volConfig *VolumeConfig, storagePool *Pool, volAttributes map[string]sa.Request, retry bool,
) (*Volume, error) {

	var err error

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"backendUUID":    b.BackendUUID,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
		"storage_pool":   storagePool.Name,
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
	if err = b.Driver.Create(ctx, volConfig, storagePool, volAttributes); err != nil {

		if drivers.IsVolumeExistsError(err) {

			// Implement idempotency by ignoring the error if the volume exists already
			volumeExists = true

			Logc(ctx).WithFields(log.Fields{
				"backend": b.Name,
				"volume":  volConfig.InternalName,
			}).Warning("Volume already exists.")

		} else {
			// If the volume doesn't exist but the create failed, return the error
			return nil, err
		}
	}

	// Always perform the follow-up steps
	if err = b.Driver.CreateFollowup(ctx, volConfig); err != nil {

		Logc(ctx).WithFields(log.Fields{
			"backend":      b.Name,
			"volume":       volConfig.InternalName,
			"volumeExists": volumeExists,
			"retry":        retry,
		}).Errorf("CreateFollowup failed; %v", err)

		// If follow-up fails and we just created the volume, clean up by deleting it
		if !volumeExists || retry {

			Logc(ctx).WithFields(log.Fields{
				"backend": b.Name,
				"volume":  volConfig.InternalName,
			}).Errorf("CreateFollowup failed for newly created volume, deleting the volume.")

			errDestroy := b.Driver.Destroy(ctx, volConfig.InternalName)
			if errDestroy != nil {
				Logc(ctx).WithFields(log.Fields{
					"backend": b.Name,
					"volume":  volConfig.InternalName,
				}).Warnf("Mapping the created volume failed "+
					"and %s wasn't able to delete it afterwards: %s. "+
					"Volume must be manually deleted.",
					tridentconfig.OrchestratorName, errDestroy)
			}
		}

		// In all cases where follow-up fails, return the follow-up error
		return nil, err
	}

	vol := NewVolume(volConfig, b.BackendUUID, storagePool.Name, false)
	b.Volumes[vol.Config.Name] = vol
	return vol, nil
}

func (b *Backend) GetDebugTraceFlags(ctx context.Context) map[string]bool {

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

	return emptyMap
}

func (b *Backend) CloneVolume(
	ctx context.Context, volConfig *VolumeConfig, storagePool *Pool, retry bool,
) (*Volume, error) {

	Logc(ctx).WithFields(log.Fields{
		"backend":                volConfig.Name,
		"backendUUID":            b.BackendUUID,
		"storage_class":          volConfig.StorageClass,
		"source_volume":          volConfig.CloneSourceVolume,
		"source_volume_internal": volConfig.CloneSourceVolumeInternal,
		"source_snapshot":        volConfig.CloneSourceSnapshot,
		"clone_volume":           volConfig.Name,
		"clone_volume_internal":  volConfig.InternalName,
	}).Debug("Attempting volume clone.")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return nil, &NotManagedError{volConfig.InternalName}
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	// Ensure the internal names exist
	if volConfig.InternalName == "" {
		return nil, errors.New("internal name not set")
	}
	if volConfig.CloneSourceVolumeInternal == "" {
		return nil, errors.New("clone source volume internal name not set")
	}

	// Clone volume on the backend
	volumeExists := false
	if err := b.Driver.CreateClone(ctx, volConfig, storagePool); err != nil {

		if drivers.IsVolumeExistsError(err) {

			// Implement idempotency by ignoring the error if the volume exists already
			volumeExists = true

			Logc(ctx).WithFields(log.Fields{
				"backend": b.Name,
				"volume":  volConfig.InternalName,
			}).Warning("Volume already exists.")

		} else {
			// If the volume doesn't exist but the create failed, return the error
			return nil, err
		}
	}

	// The clone may not be fully created when the clone API returns, so wait here until it exists.
	checkCloneExists := func() error {
		return b.Driver.Get(ctx, volConfig.InternalName)
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
		Logc(ctx).WithField("clone_volume", volConfig.Name).Warnf("Could not find clone after %3.2f seconds.",
			float64(cloneBackoff.MaxElapsedTime))
	} else {
		Logc(ctx).WithField("clone_volume", volConfig.Name).Debug("Clone found.")
	}

	if err := b.Driver.CreateFollowup(ctx, volConfig); err != nil {

		// If follow-up fails and we just created the volume, clean up by deleting it
		if !volumeExists || retry {
			errDestroy := b.Driver.Destroy(ctx, volConfig.InternalName)
			if errDestroy != nil {
				Logc(ctx).WithFields(log.Fields{
					"backend": b.Name,
					"volume":  volConfig.InternalName,
				}).Warnf("Mapping the created volume failed "+
					"and %s wasn't able to delete it afterwards: %s. "+
					"Volume must be manually deleted.",
					tridentconfig.OrchestratorName, errDestroy)
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
		poolName = storagePool.Name
	}

	vol := NewVolume(volConfig, b.BackendUUID, poolName, false)
	b.Volumes[vol.Config.Name] = vol
	return vol, nil
}

func (b *Backend) PublishVolume(
	ctx context.Context, volConfig *VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"backendUUID":    b.BackendUUID,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("Attempting volume publish.")

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}

	return b.Driver.Publish(ctx, volConfig, publishInfo)
}

func (b *Backend) GetVolumeExternal(ctx context.Context, volumeName string) (*VolumeExternal, error) {

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	if b.Driver.Get(ctx, volumeName) != nil {
		return nil, fmt.Errorf("volume %s was not found", volumeName)
	}

	volExternal, err := b.Driver.GetVolumeExternal(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("error requesting volume size: %v", err)
	}
	volExternal.Backend = b.Name
	volExternal.BackendUUID = b.BackendUUID
	return volExternal, nil
}

func (b *Backend) ImportVolume(ctx context.Context, volConfig *VolumeConfig) (*Volume, error) {

	Logc(ctx).WithFields(log.Fields{
		"backend":    b.Name,
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
		b.Driver.CreatePrepare(ctx, volConfig)
	}

	err := b.Driver.Import(ctx, volConfig, volConfig.ImportOriginalName)
	if err != nil {
		return nil, fmt.Errorf("driver import volume failed: %v", err)
	}

	err = b.Driver.CreateFollowup(ctx, volConfig)
	if err != nil {
		return nil, fmt.Errorf("failed post import volume operations : %v", err)
	}

	volume := NewVolume(volConfig, b.BackendUUID, drivers.UnsetPool, false)
	b.Volumes[volume.Config.Name] = volume
	return volume, nil
}

func (b *Backend) ResizeVolume(ctx context.Context, volConfig *VolumeConfig, newSize string) error {

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return &NotManagedError{volConfig.InternalName}
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

	Logc(ctx).WithFields(log.Fields{
		"backend":     b.Name,
		"volume":      volConfig.InternalName,
		"volume_size": newSizeBytes,
	}).Debug("Attempting volume resize.")
	return b.Driver.Resize(ctx, volConfig, newSizeBytes)
}

func (b *Backend) RenameVolume(ctx context.Context, volConfig *VolumeConfig, newName string) error {

	oldName := volConfig.InternalName

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return &NotManagedError{oldName}
	}

	if b.State != Online {
		Logc(ctx).WithFields(log.Fields{
			"state":         b.State,
			"expectedState": string(Online),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online", b.Name)
	}

	if err := b.Driver.Get(ctx, oldName); err != nil {
		return fmt.Errorf("volume %s not found on backend %s; %v", oldName, b.Name, err)
	}
	if err := b.Driver.Rename(ctx, oldName, newName); err != nil {
		return fmt.Errorf("error attempting to rename volume %s on backend %s: %v", oldName, b.Name, err)
	}
	return nil
}

func (b *Backend) RemoveVolume(ctx context.Context, volConfig *VolumeConfig) error {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("Backend#RemoveVolume")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		b.RemoveCachedVolume(volConfig.Name)
		return &NotManagedError{volConfig.InternalName}
	}

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}

	if err := b.Driver.Destroy(ctx, volConfig.InternalName); err != nil {
		// TODO:  Check the error being returned once the nDVP throws errors
		// for volumes that aren't found.
		return err
	}
	b.RemoveCachedVolume(volConfig.Name)
	return nil
}

func (b *Backend) RemoveCachedVolume(volumeName string) {
	delete(b.Volumes, volumeName)
}

func (b *Backend) GetSnapshot(ctx context.Context, snapConfig *SnapshotConfig) (*Snapshot, error) {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"volume":         snapConfig.Name,
		"volumeInternal": snapConfig.InternalName,
		"snapshotName":   snapConfig.Name,
	}).Debug("GetSnapshot.")

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	if snapshot, err := b.Driver.GetSnapshot(ctx, snapConfig); err != nil {
		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return nil, err
	} else if snapshot == nil {
		// No error and no snapshot means the snapshot doesn't exist.
		return nil, fmt.Errorf("snapshot %s on volume %s not found", snapConfig.Name, snapConfig.VolumeName)
	} else {
		return snapshot, nil
	}
}

func (b *Backend) GetSnapshots(ctx context.Context, volConfig *VolumeConfig) ([]*Snapshot, error) {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"volume":         volConfig.Name,
		"volumeInternal": volConfig.InternalName,
	}).Debug("GetSnapshots.")

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	return b.Driver.GetSnapshots(ctx, volConfig)
}

func (b *Backend) CreateSnapshot(
	ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig,
) (*Snapshot, error) {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"volume":         snapConfig.Name,
		"volumeInternal": snapConfig.InternalName,
		"snapshot":       snapConfig.Name,
	}).Debug("Attempting snapshot create.")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return nil, &NotManagedError{volConfig.InternalName}
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return nil, err
	}

	// Set the default internal snapshot name to match the snapshot name.  Drivers
	// may override this value in the SnapshotConfig structure if necessary.
	snapConfig.InternalName = snapConfig.Name

	// Implement idempotency by checking for the snapshot first
	if existingSnapshot, err := b.Driver.GetSnapshot(ctx, snapConfig); err != nil {

		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return nil, err

	} else if existingSnapshot != nil {

		Logc(ctx).WithFields(log.Fields{
			"backend":      b.Name,
			"volumeName":   snapConfig.VolumeName,
			"snapshotName": snapConfig.Name,
		}).Warning("Snapshot already exists.")

		// Snapshot already exists, so just return it
		return existingSnapshot, nil
	}

	// Create snapshot
	return b.Driver.CreateSnapshot(ctx, snapConfig)
}

func (b *Backend) RestoreSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"volume":         snapConfig.Name,
		"volumeInternal": snapConfig.InternalName,
		"snapshot":       snapConfig.Name,
	}).Debug("Attempting snapshot restore.")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return &NotManagedError{volConfig.InternalName}
	}

	// Ensure backend is ready
	if err := b.ensureOnline(ctx); err != nil {
		return err
	}

	// Restore snapshot
	return b.Driver.RestoreSnapshot(ctx, snapConfig)
}

func (b *Backend) DeleteSnapshot(ctx context.Context, snapConfig *SnapshotConfig, volConfig *VolumeConfig) error {

	Logc(ctx).WithFields(log.Fields{
		"backend":        b.Name,
		"volume":         snapConfig.Name,
		"volumeInternal": snapConfig.InternalName,
		"snapshot":       snapConfig.Name,
	}).Debug("Attempting snapshot delete.")

	// Ensure volume is managed
	if volConfig.ImportNotManaged {
		return &NotManagedError{volConfig.InternalName}
	}

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(ctx); err != nil {
		return err
	}

	// Implement idempotency by checking for the snapshot first
	if existingSnapshot, err := b.Driver.GetSnapshot(ctx, snapConfig); err != nil {

		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return err

	} else if existingSnapshot == nil {

		Logc(ctx).WithFields(log.Fields{
			"backend":      b.Name,
			"volumeName":   snapConfig.VolumeName,
			"snapshotName": snapConfig.Name,
		}).Warning("Snapshot not found.")

		// Snapshot does not exist, so just return without error.
		return nil
	}

	// Delete snapshot
	return b.Driver.DeleteSnapshot(ctx, snapConfig)
}

const (
	BackendRename = iota
	VolumeAccessInfoChange
	InvalidUpdate
	UsernameChange
	PasswordChange
	PrefixChange
)

func (b *Backend) GetUpdateType(ctx context.Context, origBackend *Backend) *roaring.Bitmap {
	updateCode := b.Driver.GetUpdateType(ctx, origBackend.Driver)
	if b.Name != origBackend.Name {
		updateCode.Add(BackendRename)
	}
	return updateCode
}

// HasVolumes returns true if the Backend has one or more volumes
// provisioned on it.
func (b *Backend) HasVolumes() bool {
	return len(b.Volumes) > 0
}

// Terminate informs the backend that it is being deleted from the core
// and will not be called again.  This may be a signal to the storage
// driver to clean up and stop any ongoing operations.
func (b *Backend) Terminate(ctx context.Context) {

	logFields := log.Fields{
		"backend":     b.Name,
		"backendUUID": b.BackendUUID,
		"driver":      b.GetDriverName(),
		"state":       string(b.State),
	}

	if !b.Driver.Initialized() {
		Logc(ctx).WithFields(logFields).Warning("Cannot terminate an uninitialized backend.")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Terminating backend.")
		b.Driver.Terminate(ctx, b.BackendUUID)
	}
}

// ReconcileNodeAccess will ensure that the driver only has allowed access
// to its volumes from active nodes in the k8s cluster. This is usually
// handled via export policies or initiators
func (b *Backend) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node) error {
	if b.State == Online || b.State == Deleting {
		return b.Driver.ReconcileNodeAccess(ctx, nodes, b.BackendUUID)
	}
	return nil
}

func (b *Backend) ensureOnline(ctx context.Context) error {

	if b.State != Online {
		Logc(ctx).WithFields(log.Fields{
			"state":         b.State,
			"expectedState": string(Online),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online", b.Name)
	}
	return nil
}

func (b *Backend) ensureOnlineOrDeleting(ctx context.Context) error {

	if b.State != Online && b.State != Deleting {
		Logc(ctx).WithFields(log.Fields{
			"state":         b.State,
			"expectedState": string(Online) + "/" + string(Deleting),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online or Deleting", b.Name)
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
	Volumes     []string               `json:"volumes"`
}

func (b *Backend) ConstructExternal(ctx context.Context) *BackendExternal {
	backendExternal := BackendExternal{
		Name:        b.Name,
		BackendUUID: b.BackendUUID,
		Protocol:    b.GetProtocol(ctx),
		Config:      b.Driver.GetExternalConfig(ctx),
		Storage:     make(map[string]interface{}),
		Online:      b.Online,
		State:       b.State,
		Volumes:     make([]string, 0),
	}

	for name, pool := range b.Storage {
		backendExternal.Storage[name] = pool.ConstructExternal()
	}
	for volName := range b.Volumes {
		backendExternal.Volumes = append(backendExternal.Volumes, volName)
	}
	return &backendExternal
}

// Used to store the requisite info for a backend in the persistent store.  Other than
// the configuration, all other data will be reconstructed during the bootstrap phase.

type PersistentStorageBackendConfig struct {
	OntapConfig             *drivers.OntapStorageDriverConfig     `json:"ontap_config,omitempty"`
	SolidfireConfig         *drivers.SolidfireStorageDriverConfig `json:"solidfire_config,omitempty"`
	EseriesConfig           *drivers.ESeriesStorageDriverConfig   `json:"eseries_config,omitempty"`
	AWSConfig               *drivers.AWSNFSStorageDriverConfig    `json:"aws_config,omitempty"`
	AzureConfig             *drivers.AzureNFSStorageDriverConfig  `json:"azure_config,omitempty"`
	GCPConfig               *drivers.GCPNFSStorageDriverConfig    `json:"gcp_config,omitempty"`
	FakeStorageDriverConfig *drivers.FakeStorageDriverConfig      `json:"fake_config,omitempty"`
}

type BackendPersistent struct {
	Version     string                         `json:"version"`
	Config      PersistentStorageBackendConfig `json:"config"`
	Name        string                         `json:"name"`
	BackendUUID string                         `json:"backendUUID"`
	Online      bool                           `json:"online"`
	State       BackendState                   `json:"state"`
}

func (b *Backend) ConstructPersistent(ctx context.Context) *BackendPersistent {
	persistentBackend := &BackendPersistent{
		Version:     tridentconfig.OrchestratorAPIVersion,
		Config:      PersistentStorageBackendConfig{},
		Name:        b.Name,
		Online:      b.Online,
		State:       b.State,
		BackendUUID: b.BackendUUID,
	}
	b.Driver.StoreConfig(ctx, &persistentBackend.Config)
	return persistentBackend
}

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
	case p.Config.EseriesConfig != nil:
		bytes, err = json.Marshal(p.Config.EseriesConfig)
	case p.Config.AWSConfig != nil:
		bytes, err = json.Marshal(p.Config.AWSConfig)
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

// ExtractBackendSecrets clones itself (a BackendPersistent struct), builds a map of any secret data it
// contains (credentials, etc.), clears those fields in the clone, and returns the clone and the map.
func (p *BackendPersistent) ExtractBackendSecrets(secretName string) (*BackendPersistent, map[string]string, error) {

	clone, err := copystructure.Copy(*p)
	if err != nil {
		return nil, nil, err
	}

	backend, ok := clone.(BackendPersistent)
	if !ok {
		return nil, nil, err
	}

	secretName = fmt.Sprintf("secret:%s", secretName)
	secretMap := make(map[string]string)

	switch {
	case backend.Config.OntapConfig != nil:
		secretMap["Username"] = backend.Config.OntapConfig.Username
		secretMap["Password"] = backend.Config.OntapConfig.Password
		backend.Config.OntapConfig.Username = secretName
		backend.Config.OntapConfig.Password = secretName
		// CHAP settings
		if p.Config.OntapConfig.UseCHAP {
			secretMap["ChapUsername"] = backend.Config.OntapConfig.ChapUsername
			secretMap["ChapInitiatorSecret"] = backend.Config.OntapConfig.ChapInitiatorSecret
			secretMap["ChapTargetUsername"] = backend.Config.OntapConfig.ChapTargetUsername
			secretMap["ChapTargetInitiatorSecret"] = backend.Config.OntapConfig.ChapTargetInitiatorSecret
			backend.Config.OntapConfig.ChapUsername = secretName
			backend.Config.OntapConfig.ChapInitiatorSecret = secretName
			backend.Config.OntapConfig.ChapTargetUsername = secretName
			backend.Config.OntapConfig.ChapTargetInitiatorSecret = secretName
		}
	case p.Config.SolidfireConfig != nil:
		secretMap["EndPoint"] = backend.Config.SolidfireConfig.EndPoint
		backend.Config.SolidfireConfig.EndPoint = secretName
	case p.Config.EseriesConfig != nil:
		secretMap["Username"] = backend.Config.EseriesConfig.Username
		secretMap["Password"] = backend.Config.EseriesConfig.Password
		secretMap["PasswordArray"] = backend.Config.EseriesConfig.PasswordArray
		backend.Config.EseriesConfig.Username = secretName
		backend.Config.EseriesConfig.Password = secretName
		backend.Config.EseriesConfig.PasswordArray = secretName
	case p.Config.AWSConfig != nil:
		secretMap["APIKey"] = backend.Config.AWSConfig.APIKey
		secretMap["SecretKey"] = backend.Config.AWSConfig.SecretKey
		backend.Config.AWSConfig.APIKey = secretName
		backend.Config.AWSConfig.SecretKey = secretName
	case p.Config.AzureConfig != nil:
		secretMap["ClientID"] = backend.Config.AzureConfig.ClientID
		secretMap["ClientSecret"] = backend.Config.AzureConfig.ClientSecret
		backend.Config.AzureConfig.ClientID = secretName
		backend.Config.AzureConfig.ClientSecret = secretName
	case p.Config.GCPConfig != nil:
		secretMap["Private_Key"] = backend.Config.GCPConfig.APIKey.PrivateKey
		secretMap["Private_Key_ID"] = backend.Config.GCPConfig.APIKey.PrivateKeyID
		backend.Config.GCPConfig.APIKey.PrivateKey = secretName
		backend.Config.GCPConfig.APIKey.PrivateKeyID = secretName
	case p.Config.FakeStorageDriverConfig != nil:
		// Nothing to do
	default:
		return nil, nil, errors.New("cannot extract secrets, unknown backend type")
	}

	return &backend, secretMap, nil
}

func (p *BackendPersistent) InjectBackendSecrets(secretMap map[string]string) error {

	makeError := func(fieldName string) error {
		return fmt.Errorf("%s field missing from backend secrets", fieldName)
	}

	var ok bool

	switch {
	case p.Config.OntapConfig != nil:
		if p.Config.OntapConfig.Username, ok = secretMap["Username"]; !ok {
			return makeError("Username")
		}
		if p.Config.OntapConfig.Password, ok = secretMap["Password"]; !ok {
			return makeError("Password")
		}
		// CHAP settings
		if p.Config.OntapConfig.UseCHAP {
			if p.Config.OntapConfig.ChapUsername, ok = secretMap["ChapUsername"]; !ok {
				return makeError("ChapUsername")
			}
			if p.Config.OntapConfig.ChapInitiatorSecret, ok = secretMap["ChapInitiatorSecret"]; !ok {
				return makeError("ChapInitiatorSecret")
			}
			if p.Config.OntapConfig.ChapTargetUsername, ok = secretMap["ChapTargetUsername"]; !ok {
				return makeError("ChapTargetUsername")
			}
			if p.Config.OntapConfig.ChapTargetInitiatorSecret, ok = secretMap["ChapTargetInitiatorSecret"]; !ok {
				return makeError("ChapTargetInitiatorSecret")
			}
		}
	case p.Config.SolidfireConfig != nil:
		if p.Config.SolidfireConfig.EndPoint, ok = secretMap["EndPoint"]; !ok {
			return makeError("EndPoint")
		}
	case p.Config.EseriesConfig != nil:
		if p.Config.EseriesConfig.Username, ok = secretMap["Username"]; !ok {
			return makeError("Username")
		}
		if p.Config.EseriesConfig.Password, ok = secretMap["Password"]; !ok {
			return makeError("Password")
		}
		if p.Config.EseriesConfig.PasswordArray, ok = secretMap["PasswordArray"]; !ok {
			return makeError("PasswordArray")
		}
	case p.Config.AWSConfig != nil:
		if p.Config.AWSConfig.APIKey, ok = secretMap["APIKey"]; !ok {
			return makeError("APIKey")
		}
		if p.Config.AWSConfig.SecretKey, ok = secretMap["SecretKey"]; !ok {
			return makeError("SecretKey")
		}
	case p.Config.AzureConfig != nil:
		if p.Config.AzureConfig.ClientID, ok = secretMap["ClientID"]; !ok {
			return makeError("ClientID")
		}
		if p.Config.AzureConfig.ClientSecret, ok = secretMap["ClientSecret"]; !ok {
			return makeError("ClientSecret")
		}
	case p.Config.GCPConfig != nil:
		if p.Config.GCPConfig.APIKey.PrivateKey, ok = secretMap["Private_Key"]; !ok {
			return makeError("Private_Key")
		}
		if p.Config.GCPConfig.APIKey.PrivateKeyID, ok = secretMap["Private_Key_ID"]; !ok {
			return makeError("Private_Key_ID")
		}
	case p.Config.FakeStorageDriverConfig != nil:
		// Nothing to do
	default:
		return errors.New("cannot inject secrets, unknown backend type")
	}

	return nil
}
