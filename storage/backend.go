// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

// StorageDriver provides a common interface for storage related operations
type StorageDriver interface {
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
	GetStorageBackendSpecs(backend *StorageBackend) error
	GetVolumeOpts(
		volConfig *VolumeConfig,
		pool *StoragePool,
		requests map[string]storage_attribute.Request,
	) (map[string]string, error)
	GetProtocol() config.Protocol
	StoreConfig(b *PersistentStorageBackendConfig)
	// GetExternalConfig returns a version of the driver configuration that
	// lacks confidential information, such as usernames and passwords.
	GetExternalConfig() interface{}
	GetVolumeExternal(name string) (*VolumeExternal, error)
	GetVolumeExternalWrappers(chan *VolumeExternalWrapper)
}

type StorageBackend struct {
	Driver  StorageDriver
	Name    string
	Online  bool
	Storage map[string]*StoragePool
	Volumes map[string]*Volume
}

func NewStorageBackend(driver StorageDriver) (*StorageBackend, error) {
	backend := StorageBackend{
		Driver:  driver,
		Online:  true,
		Storage: make(map[string]*StoragePool),
		Volumes: make(map[string]*Volume),
	}

	// retrieve backend specs
	if err := backend.Driver.GetStorageBackendSpecs(&backend); err != nil {
		return nil, err
	}

	return &backend, nil
}

func (b *StorageBackend) AddStoragePool(pool *StoragePool) {
	b.Storage[pool.Name] = pool
}

func (b *StorageBackend) GetDriverName() string {
	return b.Driver.Name()
}

func (b *StorageBackend) GetProtocol() config.Protocol {
	return b.Driver.GetProtocol()
}

func (b *StorageBackend) AddVolume(
	volConfig *VolumeConfig,
	storagePool *StoragePool,
	volumeAttributes map[string]storage_attribute.Request,
) (*Volume, error) {

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return nil, fmt.Errorf("Could not convert volume size %s: %v", volConfig.Size, err)
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

func (b *StorageBackend) CloneVolume(volConfig *VolumeConfig) (*Volume, error) {

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
		return nil, errors.New("Failed to prepare clone create.")
	}

	nilAttributes := make(map[string]storage_attribute.Request)
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

// HasVolumes returns true if the StorageBackend has one or more volumes
// provisioned on it.
func (b *StorageBackend) HasVolumes() bool {
	return len(b.Volumes) > 0
}

func (b *StorageBackend) RemoveVolume(vol *Volume) error {
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

// Terminate informs the backend that it is being deleted from the core
// and will not be called again.  This may be a signal to the storage
// driver to clean up and stop any ongoing operations.
func (b *StorageBackend) Terminate() {

	log.WithFields(log.Fields{
		"backendName": b.Name,
		"driverName":  b.GetDriverName(),
	}).Debug("Terminating backend.")

	b.Driver.Terminate()
}

type StorageBackendExternal struct {
	Name    string                          `json:"name"`
	Config  interface{}                     `json:"config"`
	Storage map[string]*StoragePoolExternal `json:"storage"`
	Online  bool                            `json:"online"`
	Volumes []string                        `json:"volumes"`
}

func (b *StorageBackend) ConstructExternal() *StorageBackendExternal {
	backendExternal := StorageBackendExternal{
		Name:    b.Name,
		Config:  b.Driver.GetExternalConfig(),
		Storage: make(map[string]*StoragePoolExternal),
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

type StorageBackendPersistent struct {
	Version string                         `json:"version"`
	Config  PersistentStorageBackendConfig `json:"config"`
	Name    string                         `json:"name"`
	Online  bool                           `json:"online"`
}

func (b *StorageBackend) ConstructPersistent() *StorageBackendPersistent {
	persistentBackend := &StorageBackendPersistent{
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
// Ideally, StorageBackendPersistent would just store a serialized config, but
// doing so appears to cause problems with the json.RawMessage fields.
func (p *StorageBackendPersistent) MarshalConfig() (string, error) {
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
		return "", fmt.Errorf("No recognized config found for backend %s.", p.Name)
	}
	if err != nil {
		return "", err
	}
	return string(bytes), err
}
