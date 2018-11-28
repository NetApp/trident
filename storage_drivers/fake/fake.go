// Copyright 2018 NetApp, Inc. All Rights Reserved.

package fake

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

const (
	FakePoolAttribute      = "pool"
	MinimumVolumeSizeBytes = 1048576 // 1 MiB
)

type StorageDriver struct {
	initialized bool
	Config      drivers.FakeStorageDriverConfig

	// Volumes saves info about Volumes created on this driver
	Volumes map[string]fake.Volume

	// DestroyedVolumes is here so that tests can check whether destroy
	// has been called on a volume during or after bootstrapping, since
	// different driver instances with the same config won't actually share
	// state.
	DestroyedVolumes map[string]bool
}

func NewFakeStorageDriver(config drivers.FakeStorageDriverConfig) *StorageDriver {
	return &StorageDriver{
		initialized:      true,
		Config:           config,
		Volumes:          make(map[string]fake.Volume),
		DestroyedVolumes: make(map[string]bool),
	}
}

func newFakeStorageDriverConfigJSON(
	name string,
	protocol tridentconfig.Protocol,
	pools map[string]*fake.StoragePool,
) (string, error) {
	prefix := ""
	jsonBytes, err := json.Marshal(
		&drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: drivers.FakeStorageDriverName,
				StoragePrefixRaw:  json.RawMessage("{}"),
				StoragePrefix:     &prefix,
			},
			Protocol:     protocol,
			Pools:        pools,
			InstanceName: name,
		},
	)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func NewFakeStorageDriverConfigJSON(
	name string,
	protocol tridentconfig.Protocol,
	pools map[string]*fake.StoragePool,
) (string, error) {
	return newFakeStorageDriverConfigJSON(name, protocol, pools)
}

func (d *StorageDriver) Name() string {
	return drivers.FakeStorageDriverName
}

func (d *StorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	err := json.Unmarshal([]byte(configJSON), &d.Config)
	if err != nil {
		return fmt.Errorf("unable to initialize fake driver: %v", err)
	}

	err = d.populateConfigurationDefaults(&d.Config)
	if err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	d.Volumes = make(map[string]fake.Volume)
	d.DestroyedVolumes = make(map[string]bool)
	d.Config.SerialNumbers = []string{d.Config.InstanceName + "_SN"}

	s, _ := json.Marshal(d.Config)
	log.Debugf("FakeStorageDriverConfig: %s", string(s))

	d.initialized = true
	return nil
}

func (d *StorageDriver) Initialized() bool {
	return d.initialized
}

func (d *StorageDriver) Terminate() {
	d.initialized = false
}

// PopulateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *StorageDriver) populateConfigurationDefaults(config *drivers.FakeStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "populateConfigurationDefaults", "Type": "StorageDriver"}
		log.WithFields(fields).Debug(">>>> populateConfigurationDefaults")
		defer log.WithFields(fields).Debug("<<<< populateConfigurationDefaults")
	}

	// Ensure the default volume size is valid, using a "default default" of 1G if not set
	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	} else {
		_, err := utils.ConvertSizeToBytes(config.Size)
		if err != nil {
			return fmt.Errorf("invalid config value for default volume size: %v", err)
		}
	}

	log.WithFields(log.Fields{
		"Size": config.Size,
	}).Debugf("Configuration defaults")

	return nil
}

func (d *StorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	poolName, ok := opts[FakePoolAttribute]
	if !ok {
		return fmt.Errorf("no pool specified; expected %s in opts map", FakePoolAttribute)
	}

	pool, ok := d.Config.Pools[poolName]
	if !ok {
		return fmt.Errorf("could not find pool %s", poolName)
	}

	if _, ok = d.Volumes[name]; ok {
		return fmt.Errorf("volume %s already exists", name)
	}

	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(d.Config.Size)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return fmt.Errorf("requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes)
	}

	if sizeBytes > pool.Bytes {
		return fmt.Errorf("requested volume is too large; requested %d bytes; have %d available in pool %s",
			sizeBytes, pool.Bytes, poolName)
	}

	d.Volumes[name] = fake.Volume{
		Name:      name,
		PoolName:  poolName,
		SizeBytes: sizeBytes,
	}
	d.DestroyedVolumes[name] = false
	pool.Bytes -= sizeBytes

	log.WithFields(log.Fields{
		"backend":   d.Config.InstanceName,
		"Name":      name,
		"PoolName":  poolName,
		"SizeBytes": sizeBytes,
	}).Debug("Created fake volume.")

	return nil
}

func (d *StorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {

	// Ensure source volume exists
	sourceVolume, ok := d.Volumes[source]
	if !ok {
		return fmt.Errorf("source volume %s not found", name)
	}

	// Ensure clone volume doesn't exist
	if _, ok := d.Volumes[name]; ok {
		return fmt.Errorf("volume %s already exists", name)
	}

	// Use the same pool as the source
	poolName := sourceVolume.PoolName
	pool, ok := d.Config.Pools[poolName]
	if !ok {
		return fmt.Errorf("could not find pool %s", poolName)
	}

	// Use the same size as the source
	sizeBytes := sourceVolume.SizeBytes
	if sizeBytes > pool.Bytes {
		return fmt.Errorf("requested clone is too large: requested %d bytes; have %d available in pool %s",
			sizeBytes, pool.Bytes, poolName)
	}

	d.Volumes[name] = fake.Volume{
		Name:      name,
		PoolName:  poolName,
		SizeBytes: sizeBytes,
	}
	d.DestroyedVolumes[name] = false
	pool.Bytes -= sizeBytes

	log.WithFields(log.Fields{
		"backend":   d.Config.InstanceName,
		"Name":      name,
		"source":    sourceVolume.Name,
		"snapshot":  snapshot,
		"PoolName":  poolName,
		"SizeBytes": sizeBytes,
	}).Debug("Cloned fake volume.")

	return nil
}

func (d *StorageDriver) Destroy(name string) error {

	d.DestroyedVolumes[name] = true

	volume, ok := d.Volumes[name]
	if !ok {
		return nil
	}

	pool, ok := d.Config.Pools[volume.PoolName]
	if !ok {
		return fmt.Errorf("could not find pool %s", volume.PoolName)
	}

	pool.Bytes += volume.SizeBytes
	delete(d.Volumes, name)

	log.WithFields(log.Fields{
		"backend":   d.Config.InstanceName,
		"Name":      name,
		"PoolName":  volume.PoolName,
		"SizeBytes": volume.SizeBytes,
	}).Debug("Deleted fake volume.")

	return nil
}

func (d *StorageDriver) Publish(name string, publishInfo *utils.VolumePublishInfo) error {
	return errors.New("fake driver does not support Publish")
}

func (d *StorageDriver) SnapshotList(name string) ([]storage.Snapshot, error) {
	return nil, errors.New("fake driver does not support SnapshotList")
}

func (d *StorageDriver) Get(name string) error {

	_, ok := d.Volumes[name]
	if !ok {
		return fmt.Errorf("could not find volume %s", name)
	}

	return nil
}

func (d *StorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no backend is specified
		backend.Name = d.Config.InstanceName
	} else {
		backend.Name = d.Config.BackendName
	}

	for name, pool := range d.Config.Pools {
		vc := &storage.Pool{
			Name:           name,
			StorageClasses: make([]string, 0),
			Backend:        backend,
			Attributes:     pool.Attrs,
		}
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		backend.AddStoragePool(vc)
	}
	return nil
}

func (d *StorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.Pool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	opts := make(map[string]string)
	if pool != nil {
		opts[FakePoolAttribute] = pool.Name
	}
	return opts, nil
}

func (d *StorageDriver) GetInternalVolumeName(name string) string {
	return drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
}

func (d *StorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal =
			d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}
	return true
}

func (d *StorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	switch d.Config.Protocol {
	case tridentconfig.File:
		volConfig.AccessInfo.NfsServerIP = "192.0.2.1" // unrouteable test address, see RFC 5737
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
	case tridentconfig.Block:
		volConfig.AccessInfo.IscsiTargetPortal = "192.0.2.1"
		volConfig.AccessInfo.IscsiPortals = []string{"192.0.2.2"}
		volConfig.AccessInfo.IscsiTargetIQN = "iqn.2017-06.com.netapp:fake"
		volConfig.AccessInfo.IscsiLunNumber = 0
	}
	return nil
}

func (d *StorageDriver) GetProtocol() tridentconfig.Protocol {
	return d.Config.Protocol
}

func (d *StorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {

	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)

	// Clone the config so we don't alter the original
	var cloneCommonConfig drivers.CommonStorageDriverConfig
	Clone(d.Config.CommonStorageDriverConfig, &cloneCommonConfig)
	cloneCommonConfig.SerialNumbers = nil

	b.FakeStorageDriverConfig = &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &cloneCommonConfig,
		Protocol:                  d.Config.Protocol,
		Pools:                     d.Config.Pools,
		InstanceName:              d.Config.InstanceName,
	}
}

func (d *StorageDriver) GetExternalConfig() interface{} {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)

	return &struct {
		*drivers.CommonStorageDriverConfigExternal
		Protocol     tridentconfig.Protocol       `json:"protocol"`
		Pools        map[string]*fake.StoragePool `json:"pools"`
		InstanceName string
	}{
		drivers.GetCommonStorageDriverConfigExternal(
			d.Config.CommonStorageDriverConfig),
		d.Config.Protocol,
		d.Config.Pools,
		d.Config.InstanceName,
	}
}

func (d *StorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volume, ok := d.Volumes[name]
	if !ok {
		return nil, fmt.Errorf("fake volume %s not found", name)
	}

	return d.getVolumeExternal(volume), nil
}

func (d *StorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range d.Volumes {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(volume), nil}
	}
}

func (d *StorageDriver) getVolumeExternal(volume fake.Volume) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:      tridentconfig.OrchestratorAPIVersion,
		Name:         volume.Name,
		InternalName: volume.Name,
		Size:         strconv.FormatUint(volume.SizeBytes, 10),
	}

	volumeExternal := &storage.VolumeExternal{
		Config:  volumeConfig,
		Backend: d.Name(),
		Pool:    volume.PoolName,
	}

	return volumeExternal
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *StorageDriver) GetUpdateType(dOrig storage.Driver) *roaring.Bitmap {
	//TODO
	return roaring.New()
}

func Clone(a, b interface{}) {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	enc.Encode(a)
	dec.Decode(b)
}

// Resize expands the volume size.
func (d *StorageDriver) Resize(name string, sizeBytes uint64) error {
	vol := d.Volumes[name]

	if vol.SizeBytes == sizeBytes {
		return nil
	}

	if sizeBytes < vol.SizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, vol.SizeBytes)
	} else {
		vol.SizeBytes = sizeBytes
		d.Volumes[name] = vol
	}

	return nil
}
