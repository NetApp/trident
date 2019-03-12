// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

// Driver provides a common interface for storage related operations
type Driver interface {
	Name() string
	Initialize(tridentconfig.DriverContext, string, *drivers.CommonStorageDriverConfig) error
	Initialized() bool
	// Terminate tells the driver to clean up, as it won't be called again.
	Terminate()
	Create(volConfig *VolumeConfig, storagePool *Pool, volAttributes map[string]sa.Request) error
	CreatePrepare(volConfig *VolumeConfig) error
	// CreateFollowup adds necessary information for accessing the volume to VolumeConfig.
	CreateFollowup(volConfig *VolumeConfig) error
	// GetInternalVolumeName will return a name that satisfies any character
	// constraints present on the backend and that will be unique to Trident.
	// The latter requirement should generally be done by prepending the
	// value of CommonStorageDriver.SnapshotPrefix to the name.
	CreateClone(volConfig *VolumeConfig) error
	Import(volConfig *VolumeConfig, originalName string, notManaged bool) error
	Destroy(name string) error
	Rename(name string, newName string) error
	Resize(name string, sizeBytes uint64) error
	Get(name string) error
	GetInternalVolumeName(name string) string
	GetStorageBackendSpecs(backend *Backend) error
	GetProtocol() tridentconfig.Protocol
	Publish(name string, publishInfo *utils.VolumePublishInfo) error
	GetSnapshot(snapConfig *SnapshotConfig) (*Snapshot, error)
	GetSnapshots(volConfig *VolumeConfig) ([]*Snapshot, error)
	CreateSnapshot(snapConfig *SnapshotConfig) (*Snapshot, error)
	RestoreSnapshot(snapConfig *SnapshotConfig) error
	DeleteSnapshot(snapConfig *SnapshotConfig) error
	StoreConfig(b *PersistentStorageBackendConfig)
	// GetExternalConfig returns a version of the driver configuration that
	// lacks confidential information, such as usernames and passwords.
	GetExternalConfig() interface{}
	// GetVolumeExternal accepts the internal name of a volume and returns a VolumeExternal
	// object.  This method is only available if using the passthrough store (i.e. Docker).
	GetVolumeExternal(name string) (*VolumeExternal, error)
	// GetVolumeExternalWrappers reads all volumes owned by this driver from the storage backend and
	// writes them to the supplied channel as VolumeExternalWrapper objects.  This method is only
	// available if using the passthrough store (i.e. Docker).
	GetVolumeExternalWrappers(chan *VolumeExternalWrapper)
	GetUpdateType(driver Driver) *roaring.Bitmap
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

func NewStorageBackend(driver Driver) (*Backend, error) {
	backend := Backend{
		Driver:  driver,
		State:   Online,
		Online:  true,
		Storage: make(map[string]*Pool),
		Volumes: make(map[string]*Volume),
	}

	// retrieve backend specs
	if err := backend.Driver.GetStorageBackendSpecs(&backend); err != nil {
		return nil, err
	}

	return &backend, nil
}

func NewFailedStorageBackend(driver Driver) *Backend {
	backend := Backend{
		Driver:  driver,
		State:   Failed,
		Storage: make(map[string]*Pool),
		Volumes: make(map[string]*Volume),
	}

	log.WithFields(log.Fields{
		"backend": backend,
		"driver":  driver,
	}).Debug("NewFailedStorageBackend.")

	return &backend
}

func (b *Backend) AddStoragePool(pool *Pool) {
	b.Storage[pool.Name] = pool
}

func (b *Backend) GetDriverName() string {
	return b.Driver.Name()
}

func (b *Backend) GetProtocol() tridentconfig.Protocol {
	return b.Driver.GetProtocol()
}

func (b *Backend) AddVolume(
	volConfig *VolumeConfig, storagePool *Pool, volAttributes map[string]sa.Request,
) (*Volume, error) {

	var err error

	log.WithFields(log.Fields{
		"backend":       b.Name,
		"backendUUID":   b.BackendUUID,
		"volume":        volConfig.InternalName,
		"storage_pool":  storagePool.Name,
		"size":          volConfig.Size,
		"storage_class": volConfig.StorageClass,
	}).Debug("Attempting volume create.")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	// CreatePrepare should perform the following tasks:
	// 1. Generate the internal volume name
	// 2. Optionally perform any other steps that could veto volume creation
	if err = b.Driver.CreatePrepare(volConfig); err != nil {
		return nil, err
	}

	// Add volume to the backend
	volumeExists := false
	if err = b.Driver.Create(volConfig, storagePool, volAttributes); err != nil {

		if drivers.IsVolumeExistsError(err) {

			// Implement idempotency by ignoring the error if the volume exists already
			volumeExists = true

			log.WithFields(log.Fields{
				"backend": b.Name,
				"volume":  volConfig.InternalName,
			}).Warning("Volume already exists.")

		} else {
			// If the volume doesn't exist but the create failed, return the error
			return nil, err
		}
	}

	// Always perform the follow-up steps
	if err = b.Driver.CreateFollowup(volConfig); err != nil {

		// If follow-up fails and we just created the volume, clean up by deleting it
		if !volumeExists {
			errDestroy := b.Driver.Destroy(volConfig.InternalName)
			if errDestroy != nil {
				log.WithFields(log.Fields{
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

func (b *Backend) CloneVolume(volConfig *VolumeConfig) (*Volume, error) {

	log.WithFields(log.Fields{
		"backend":                volConfig.Name,
		"backendUUID":            b.BackendUUID,
		"storage_class":          volConfig.StorageClass,
		"source_volume":          volConfig.CloneSourceVolume,
		"source_volume_internal": volConfig.CloneSourceVolumeInternal,
		"source_snapshot":        volConfig.CloneSourceSnapshot,
		"clone_volume":           volConfig.Name,
		"clone_volume_internal":  volConfig.InternalName,
	}).Debug("Attempting volume clone.")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	// CreatePrepare should perform the following tasks:
	// 1. Sanitize the volume name
	// 2. Ensure no volume with the same name exists on that backend
	if err := b.Driver.CreatePrepare(volConfig); err != nil {
		return nil, fmt.Errorf("failed to prepare clone create: %v", err)
	}

	err := b.Driver.CreateClone(volConfig)
	if err != nil {
		return nil, err
	}

	// The clone may not be fully created when the clone API returns, so wait here until it exists.
	checkCloneExists := func() error {
		return b.Driver.Get(volConfig.InternalName)
	}
	cloneExistsNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Clone not yet present, waiting.")
	}
	cloneBackoff := backoff.NewExponentialBackOff()
	cloneBackoff.InitialInterval = 1 * time.Second
	cloneBackoff.Multiplier = 2
	cloneBackoff.RandomizationFactor = 0.1
	cloneBackoff.MaxElapsedTime = 90 * time.Second

	// Run the clone check using an exponential backoff
	if err := backoff.RetryNotify(checkCloneExists, cloneBackoff, cloneExistsNotify); err != nil {
		log.WithField("clone_volume", volConfig.Name).Warnf("Could not find clone after %3.2f seconds.",
			float64(cloneBackoff.MaxElapsedTime))
	} else {
		log.WithField("clone_volume", volConfig.Name).Debug("Clone found.")
	}

	err = b.Driver.CreateFollowup(volConfig)
	if err != nil {
		errDestroy := b.Driver.Destroy(volConfig.InternalName)
		if errDestroy != nil {
			log.WithFields(log.Fields{
				"backend": b.Name,
				"volume":  volConfig.InternalName,
			}).Warnf("Mapping the created volume failed "+
				"and %s wasn't able to delete it afterwards: %s. "+
				"Volume needs to be manually deleted.",
				tridentconfig.OrchestratorName, errDestroy)
		}
		return nil, err
	}
	vol := NewVolume(volConfig, b.BackendUUID, drivers.UnsetPool, false)
	b.Volumes[vol.Config.Name] = vol
	return vol, nil
}

func (b *Backend) GetVolumeExternal(volumeName string) (*VolumeExternal, error) {

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	if b.Driver.Get(volumeName) != nil {
		return nil, fmt.Errorf("volume %s was not found", volumeName)
	}

	volExternal, err := b.Driver.GetVolumeExternal(volumeName)
	if err != nil {
		return nil, fmt.Errorf("error requesting volume size: %v", err)
	}
	volExternal.Backend = b.Name
	volExternal.BackendUUID = b.BackendUUID
	return volExternal, nil
}

func (b *Backend) ImportVolume(volConfig *VolumeConfig, notManaged bool) (*Volume, error) {

	log.WithFields(log.Fields{
		"backend":    b.Name,
		"volume":     volConfig.ImportOriginalName,
		"NotManaged": notManaged,
	}).Debug("Backend#ImportVolume")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	if notManaged {
		// The volume is not managed and will not be renamed during import.
		volConfig.InternalName = volConfig.ImportOriginalName
	} else {
		// CreatePrepare should perform the following tasks:
		// 1. Sanitize the volume name
		// 2. Ensure no volume with the same name exists on that backend
		if err := b.Driver.CreatePrepare(volConfig); err != nil {
			return nil, fmt.Errorf("failed to prepare import volume: %v", err)
		}
	}

	err := b.Driver.Import(volConfig, volConfig.ImportOriginalName, notManaged)
	if err != nil {
		return nil, fmt.Errorf("driver import volume failed: %v", err)
	}

	err = b.Driver.CreateFollowup(volConfig)
	if err != nil {
		return nil, fmt.Errorf("failed post import volume operations : %v", err)
	}

	volume := NewVolume(volConfig, b.BackendUUID, drivers.UnsetPool, false)
	if !notManaged {
		// The volume is managed
		b.Volumes[volume.Config.Name] = volume
	}
	return volume, nil
}

func (b *Backend) ResizeVolume(volName, newSize string) error {

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
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

	log.WithFields(log.Fields{
		"backend":     b.Name,
		"volume":      volName,
		"volume_size": newSizeBytes,
	}).Debug("Attempting volume resize.")
	return b.Driver.Resize(volName, newSizeBytes)
}

func (b *Backend) RemoveVolume(vol *Volume) error {

	log.WithFields(log.Fields{
		"backend": b.Name,
		"volume":  vol.Config.Name,
	}).Debug("Backend#RemoveVolume")

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(); err != nil {
		return err
	}

	if err := b.Driver.Destroy(vol.Config.InternalName); err != nil {
		// TODO:  Check the error being returned once the nDVP throws errors
		// for volumes that aren't found.
		return err
	}
	b.RemoveCachedVolume(vol.Config.Name)
	return nil
}

func (b *Backend) RemoveCachedVolume(volumeName string) {

	if _, ok := b.Volumes[volumeName]; ok {
		delete(b.Volumes, volumeName)
	}
}

func (b *Backend) GetSnapshot(snapConfig *SnapshotConfig) (*Snapshot, error) {

	log.WithFields(log.Fields{
		"backend":      b.Name,
		"volumeName":   snapConfig.VolumeName,
		"snapshotName": snapConfig.Name,
	}).Debug("GetSnapshot.")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	if snapshot, err := b.Driver.GetSnapshot(snapConfig); err != nil {
		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return nil, err
	} else if snapshot == nil {
		// No error and no snapshot means the snapshot doesn't exist.
		return nil, fmt.Errorf("snapshot %s on volume %s not found", snapConfig.Name, snapConfig.VolumeName)
	} else {
		return snapshot, nil
	}
}

func (b *Backend) GetSnapshots(volConfig *VolumeConfig) ([]*Snapshot, error) {

	log.WithFields(log.Fields{
		"backend":    b.Name,
		"volumeName": volConfig.Name,
	}).Debug("GetSnapshots.")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	return b.Driver.GetSnapshots(volConfig)
}

func (b *Backend) CreateSnapshot(snapConfig *SnapshotConfig) (*Snapshot, error) {

	log.WithFields(log.Fields{
		"backend":      b.Name,
		"volumeName":   snapConfig.VolumeName,
		"snapshotName": snapConfig.Name,
	}).Debug("Attempting snapshot create.")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return nil, err
	}

	// Set the default internal snapshot name to match the snapshot name.  Drivers
	// may override this value in the SnapshotConfig structure if necessary.
	snapConfig.InternalName = snapConfig.Name

	// Implement idempotency by checking for the snapshot first
	if existingSnapshot, err := b.Driver.GetSnapshot(snapConfig); err != nil {

		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return nil, err

	} else if existingSnapshot != nil {

		log.WithFields(log.Fields{
			"backend":      b.Name,
			"volumeName":   snapConfig.VolumeName,
			"snapshotName": snapConfig.Name,
		}).Warning("Snapshot already exists.")

		// Snapshot already exists, so just return it
		return existingSnapshot, nil
	}

	// Create snapshot
	return b.Driver.CreateSnapshot(snapConfig)
}

func (b *Backend) RestoreSnapshot(snapConfig *SnapshotConfig) error {

	log.WithFields(log.Fields{
		"backend":      b.Name,
		"volumeName":   snapConfig.VolumeName,
		"snapshotName": snapConfig.Name,
	}).Debug("Attempting snapshot restore.")

	// Ensure backend is ready
	if err := b.ensureOnline(); err != nil {
		return err
	}

	// Restore snapshot
	return b.Driver.RestoreSnapshot(snapConfig)
}

func (b *Backend) DeleteSnapshot(snapConfig *SnapshotConfig) error {

	log.WithFields(log.Fields{
		"backend":      b.Name,
		"volumeName":   snapConfig.VolumeName,
		"snapshotName": snapConfig.Name,
	}).Debug("Attempting snapshot delete.")

	// Ensure backend is ready
	if err := b.ensureOnlineOrDeleting(); err != nil {
		return err
	}

	// Implement idempotency by checking for the snapshot first
	if existingSnapshot, err := b.Driver.GetSnapshot(snapConfig); err != nil {

		// An error here means we couldn't check for the snapshot.  It does not mean the snapshot doesn't exist.
		return err

	} else if existingSnapshot == nil {

		log.WithFields(log.Fields{
			"backend":      b.Name,
			"volumeName":   snapConfig.VolumeName,
			"snapshotName": snapConfig.Name,
		}).Warning("Snapshot not found.")

		// Snapshot does not exist, so just return without error.
		return nil
	}

	// Delete snapshot
	return b.Driver.DeleteSnapshot(snapConfig)
}

const (
	BackendRename = iota
	VolumeAccessInfoChange
	InvalidUpdate
	UsernameChange
	PasswordChange
)

func (b *Backend) GetUpdateType(origBackend *Backend) *roaring.Bitmap {
	updateCode := b.Driver.GetUpdateType(origBackend.Driver)
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
func (b *Backend) Terminate() {

	logFields := log.Fields{
		"backend":     b.Name,
		"backendUUID": b.BackendUUID,
		"driver":      b.GetDriverName(),
		"state":       string(b.State),
	}

	if !b.Driver.Initialized() {
		log.WithFields(logFields).Warning("Cannot terminate an uninitialized backend.")
	} else {
		log.WithFields(logFields).Debug("Terminating backend.")
		b.Driver.Terminate()
	}
}

func (b *Backend) ensureOnline() error {
	if b.State != Online {
		log.WithFields(log.Fields{
			"state":         b.State,
			"expectedState": string(Online),
		}).Error("Invalid backend state.")
		return fmt.Errorf("backend %s is not Online", b.Name)
	}
	return nil
}

func (b *Backend) ensureOnlineOrDeleting() error {
	if b.State != Online && b.State != Deleting {
		log.WithFields(log.Fields{
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

func (b *Backend) ConstructExternal() *BackendExternal {
	backendExternal := BackendExternal{
		Name:        b.Name,
		BackendUUID: b.BackendUUID,
		Protocol:    b.GetProtocol(),
		Config:      b.Driver.GetExternalConfig(),
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

// Used to store the requisite info for a backend in etcd.  Other than
// the configuration, all other data will be reconstructed during the bootstrap
// phase

type PersistentStorageBackendConfig struct {
	OntapConfig             *drivers.OntapStorageDriverConfig     `json:"ontap_config,omitempty"`
	SolidfireConfig         *drivers.SolidfireStorageDriverConfig `json:"solidfire_config,omitempty"`
	EseriesConfig           *drivers.ESeriesStorageDriverConfig   `json:"eseries_config,omitempty"`
	AWSConfig               *drivers.AWSNFSStorageDriverConfig    `json:"aws_config,omitempty"`
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

func (b *Backend) ConstructPersistent() *BackendPersistent {
	persistentBackend := &BackendPersistent{
		Version:     tridentconfig.OrchestratorAPIVersion,
		Config:      PersistentStorageBackendConfig{},
		Name:        b.Name,
		Online:      b.Online,
		State:       b.State,
		BackendUUID: b.BackendUUID,
	}
	b.Driver.StoreConfig(&persistentBackend.Config)
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
