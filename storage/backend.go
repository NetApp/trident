// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

// Driver provides a common interface for storage related operations
type Driver interface {
	Name() string
	Initialize(config.DriverContext, string, *drivers.CommonStorageDriverConfig) error
	Initialized() bool
	// Terminate tells the driver to clean up, as it won't be called again.
	Terminate()
	Create(name string, sizeBytes uint64, opts map[string]string) error
	CreateClone(name, source, snapshot string, opts map[string]string) error
	Destroy(name string) error
	Attach(name, mountpoint string, opts map[string]string) error
	Detach(name, mountpoint string) error
	SnapshotList(name string) ([]Snapshot, error)
	List() ([]string, error)
	Get(name string) error
	CreatePrepare(volConfig *VolumeConfig) bool
	// CreateFollowup adds necessary information for accessing the volume to VolumeConfig.
	CreateFollowup(volConfig *VolumeConfig) error
	// GetInternalVolumeName will return a name that satisfies any character
	// constraints present on the backend and that will be unique to Trident.
	// The latter requirement should generally be done by prepending the
	// value of CommonStorageDriver.SnapshotPrefix to the name.
	GetInternalVolumeName(name string) string
	GetStorageBackendSpecs(backend *Backend) error
	GetVolumeOpts(
		volConfig *VolumeConfig,
		pool *Pool,
		requests map[string]storageattribute.Request,
	) (map[string]string, error)
	GetProtocol() config.Protocol
	StoreConfig(b *PersistentStorageBackendConfig)
	// GetExternalConfig returns a version of the driver configuration that
	// lacks confidential information, such as usernames and passwords.
	GetExternalConfig() interface{}
	GetVolumeExternal(name string) (*VolumeExternal, error)
	GetVolumeExternalWrappers(chan *VolumeExternalWrapper)
	GetUpdateType(driver Driver) *roaring.Bitmap
}

type Backend struct {
	Driver  Driver
	Name    string
	Online  bool
	Storage map[string]*Pool
	Volumes map[string]*Volume
}

func NewStorageBackend(driver Driver) (*Backend, error) {
	backend := Backend{
		Driver:  driver,
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

func (b *Backend) AddStoragePool(pool *Pool) {
	b.Storage[pool.Name] = pool
}

func (b *Backend) GetDriverName() string {
	return b.Driver.Name()
}

func (b *Backend) GetProtocol() config.Protocol {
	return b.Driver.GetProtocol()
}

func (b *Backend) AddVolume(
	volConfig *VolumeConfig,
	storagePool *Pool,
	volumeAttributes map[string]storageattribute.Request,
) (*Volume, error) {

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return nil, fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	volSize, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}

	log.WithFields(log.Fields{
		"storagePool": storagePool.Name,
		"size":        volSize,
		"volConfig.StorageClass": volConfig.StorageClass,
	}).Debug("Attempting volume create.")

	// CreatePrepare should perform the following tasks:
	// 1. Sanitize the volume name
	// 2. Ensure no volume with the same name exists on that backend
	if b.Driver.CreatePrepare(volConfig) {

		// add volume to the backend
		args, err := b.Driver.GetVolumeOpts(volConfig, storagePool,
			volumeAttributes)
		if err != nil {
			// An error on GetVolumeOpts is almost certainly going to indicate
			// a formatting mistake, so go ahead and return an error, rather
			// than just log a warning.
			return nil, err
		}

		if err := b.Driver.Create(volConfig.InternalName, volSize, args); err != nil {
			// Implement idempotency at the Trident layer
			// Ignore the error if the volume exists already
			if b.Driver.Get(volConfig.InternalName) != nil {
				return nil, err
			}
		}

		if err = b.Driver.CreateFollowup(volConfig); err != nil {
			errDestroy := b.Driver.Destroy(volConfig.InternalName)
			if errDestroy != nil {
				log.WithFields(log.Fields{
					"backend": b.Name,
					"volume":  volConfig.InternalName,
				}).Warnf("Mapping the created volume failed "+
					"and %s wasn't able to delete it afterwards: %s. "+
					"Volume needs to be manually deleted.",
					config.OrchestratorName, errDestroy)
			}
			return nil, err
		}
		vol := NewVolume(volConfig, b.Name, storagePool.Name, false)
		b.Volumes[vol.Config.Name] = vol
		return vol, err
	} else {
		log.WithFields(log.Fields{
			"storagePoolName":       storagePool.Name,
			"requestedBytes":        volSize,
			"requestedStorageClass": volConfig.StorageClass,
		}).Debug("Storage pool does not match volume request.")
	}
	return nil, nil
}

func (b *Backend) CloneVolume(volConfig *VolumeConfig) (*Volume, error) {

	log.WithFields(log.Fields{
		"storageClass":   volConfig.StorageClass,
		"sourceVolume":   volConfig.CloneSourceVolume,
		"sourceSnapshot": volConfig.CloneSourceSnapshot,
		"cloneVolume":    volConfig.Name,
	}).Debug("Attempting volume clone.")

	// CreatePrepare should perform the following tasks:
	// 1. Sanitize the volume name
	// 2. Ensure no volume with the same name exists on that backend
	if !b.Driver.CreatePrepare(volConfig) {
		return nil, errors.New("failed to prepare clone create")
	}

	nilAttributes := make(map[string]storageattribute.Request)
	args, err := b.Driver.GetVolumeOpts(volConfig, nil, nilAttributes)
	if err != nil {
		// An error on GetVolumeOpts is almost certainly going to indicate
		// a formatting mistake, so go ahead and return an error, rather
		// than just log a warning.
		return nil, err
	}

	err = b.Driver.CreateClone(volConfig.InternalName,
		volConfig.CloneSourceVolumeInternal, volConfig.CloneSourceSnapshot,
		args)
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
		log.WithField("cloneVolume", volConfig.Name).Warnf("Could not find clone after %3.2f seconds.",
			cloneBackoff.MaxElapsedTime)
	} else {
		log.WithField("cloneVolume", volConfig.Name).Debug("Clone found.")
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
				config.OrchestratorName, errDestroy)
		}
		return nil, err
	}
	vol := NewVolume(volConfig, b.Name, drivers.UnsetPool, false)
	b.Volumes[vol.Config.Name] = vol
	return vol, nil
}

// HasVolumes returns true if the Backend has one or more volumes
// provisioned on it.
func (b *Backend) HasVolumes() bool {
	return len(b.Volumes) > 0
}

func (b *Backend) RemoveVolume(vol *Volume) error {
	if err := b.Driver.Destroy(vol.Config.InternalName); err != nil {
		// TODO:  Check the error being returned once the nDVP throws errors
		// for volumes that aren't found.
		return err
	}
	if _, ok := b.Volumes[vol.Config.Name]; ok {
		delete(b.Volumes, vol.Config.Name)
	}
	return nil
}

const (
	BackendRename = iota
	VolumeAccessInfoChange
	InvalidUpdate
)

func (b *Backend) GetUpdateType(origBackend *Backend) *roaring.Bitmap {
	updateCode := b.Driver.GetUpdateType(origBackend.Driver)
	if b.Name != origBackend.Name {
		updateCode.Add(BackendRename)
	}
	return updateCode
}

// Terminate informs the backend that it is being deleted from the core
// and will not be called again.  This may be a signal to the storage
// driver to clean up and stop any ongoing operations.
func (b *Backend) Terminate() {

	log.WithFields(log.Fields{
		"backendName": b.Name,
		"driverName":  b.GetDriverName(),
	}).Debug("Terminating backend.")

	b.Driver.Terminate()
}

type BackendExternal struct {
	Name    string                   `json:"name"`
	Config  interface{}              `json:"config"`
	Storage map[string]*PoolExternal `json:"storage"`
	Online  bool                     `json:"online"`
	Volumes []string                 `json:"volumes"`
}

func (b *Backend) ConstructExternal() *BackendExternal {
	backendExternal := BackendExternal{
		Name:    b.Name,
		Config:  b.Driver.GetExternalConfig(),
		Storage: make(map[string]*PoolExternal),
		Online:  b.Online,
		Volumes: make([]string, 0),
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
	FakeStorageDriverConfig *drivers.FakeStorageDriverConfig      `json:"fake_config,omitempty"`
}

type BackendPersistent struct {
	Version string                         `json:"version"`
	Config  PersistentStorageBackendConfig `json:"config"`
	Name    string                         `json:"name"`
	Online  bool                           `json:"online"`
}

func (b *Backend) ConstructPersistent() *BackendPersistent {
	persistentBackend := &BackendPersistent{
		Version: config.OrchestratorAPIVersion,
		Config:  PersistentStorageBackendConfig{},
		Name:    b.Name,
		Online:  b.Online,
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
