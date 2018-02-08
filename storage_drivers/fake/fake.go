// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
)

const (
	FakePoolAttribute          = "pool"
	FakeMinimumVolumeSizeBytes = 1048576 // 1 MiB
)

type FakeStorageDriver struct {
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

func NewFakeStorageDriver(config drivers.FakeStorageDriverConfig) *FakeStorageDriver {
	return &FakeStorageDriver{
		initialized:      true,
		Config:           config,
		Volumes:          make(map[string]fake.Volume),
		DestroyedVolumes: make(map[string]bool),
	}
}

func newFakeStorageDriverConfigJSON(
	name string,
	protocol config.Protocol,
	pools map[string]*fake.StoragePool,
	destroyIgnoreNotPresent bool,
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
	protocol config.Protocol,
	pools map[string]*fake.StoragePool,
) (string, error) {
	return newFakeStorageDriverConfigJSON(name, protocol, pools, false)
}

func (d *FakeStorageDriver) Name() string {
	return drivers.FakeStorageDriverName
}

func (d *FakeStorageDriver) Initialize(
	context config.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	err := json.Unmarshal([]byte(configJSON), &d.Config)
	if err != nil {
		return fmt.Errorf("Unable to initialize fake driver: %v", err)
	}

	d.Volumes = make(map[string]fake.Volume)
	d.DestroyedVolumes = make(map[string]bool)
	d.Config.SerialNumbers = []string{d.Config.InstanceName + "_SN"}

	s, err := json.Marshal(d.Config)
	log.Debugf("FakeStorageDriverConfig: %s", string(s))

	d.initialized = true
	return nil
}

func (d *FakeStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *FakeStorageDriver) Terminate() {
	d.initialized = false
}

func (d *FakeStorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	poolName, ok := opts[FakePoolAttribute]
	if !ok {
		return fmt.Errorf("No pool specified.  Expected %s in opts map", FakePoolAttribute)
	}

	pool, ok := d.Config.Pools[poolName]
	if !ok {
		return fmt.Errorf("Could not find pool %s.", pool)
	}

	if _, ok = d.Volumes[name]; ok {
		return fmt.Errorf("Volume %s already exists", name)
	}

	if sizeBytes < FakeMinimumVolumeSizeBytes {
		return fmt.Errorf("Requested volume size (%d bytes) is too small.  The minimum volume size is %d bytes.",
			sizeBytes, FakeMinimumVolumeSizeBytes)
	}

	if sizeBytes > pool.Bytes {
		return fmt.Errorf("Requested volume is too large.  Requested %d bytes; "+
			"have %d available in pool %s.", sizeBytes, pool.Bytes, poolName)
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

func (d *FakeStorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {

	// Ensure source volume exists
	sourceVolume, ok := d.Volumes[source]
	if !ok {
		return fmt.Errorf("Source volume %s not found", name)
	}

	// Ensure clone volume doesn't exist
	if _, ok := d.Volumes[name]; ok {
		return fmt.Errorf("Volume %s already exists", name)
	}

	// Use the same pool as the source
	poolName := sourceVolume.PoolName
	pool, ok := d.Config.Pools[poolName]
	if !ok {
		return fmt.Errorf("Could not find pool %s.", pool)
	}

	// Use the same size as the source
	sizeBytes := sourceVolume.SizeBytes
	if sizeBytes > pool.Bytes {
		return fmt.Errorf("Requested clone is too large.  Requested %d bytes; "+
			"have %d available in pool %s.", sizeBytes, pool.Bytes, poolName)
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

func (d *FakeStorageDriver) Destroy(name string) error {

	d.DestroyedVolumes[name] = true

	volume, ok := d.Volumes[name]
	if !ok {
		return nil
	}

	pool, ok := d.Config.Pools[volume.PoolName]
	if !ok {
		return fmt.Errorf("Could not find pool %s.", volume.PoolName)
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

func (d *FakeStorageDriver) Attach(name, mountpoint string, opts map[string]string) error {
	return errors.New("Fake driver does not support attaching.")
}

func (d *FakeStorageDriver) Detach(name, mountpoint string) error {
	return errors.New("Fake driver does not support detaching.")
}

func (d *FakeStorageDriver) SnapshotList(name string) ([]storage.Snapshot, error) {
	return nil, errors.New("Fake driver does not support SnapshotList")
}

func (d *FakeStorageDriver) List() ([]string, error) {
	vols := []string{}
	for vol := range d.Volumes {
		vols = append(vols, vol)
	}
	return vols, nil
}

func (d *FakeStorageDriver) Get(name string) error {

	_, ok := d.Volumes[name]
	if !ok {
		return fmt.Errorf("Could not find volume %s.", name)
	}

	return nil
}

func (d *FakeStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {
	backend.Name = d.Config.InstanceName
	for name, pool := range d.Config.Pools {
		vc := &storage.StoragePool{
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

func (d *FakeStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	opts := make(map[string]string)
	if pool != nil {
		opts[FakePoolAttribute] = pool.Name
	}
	return opts, nil
}

func (d *FakeStorageDriver) GetInternalVolumeName(name string) string {
	return drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
}

func (d *FakeStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal =
			d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}
	return true
}

func (d *FakeStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	switch d.Config.Protocol {
	case config.File:
		volConfig.AccessInfo.NfsServerIP = "192.0.2.1" // unrouteable test address, see RFC 5737
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
	case config.Block:
		volConfig.AccessInfo.IscsiTargetPortal = "192.0.2.1"
		volConfig.AccessInfo.IscsiTargetIQN = "iqn.2017-06.com.netapp:fake"
		volConfig.AccessInfo.IscsiLunNumber = 0
	}
	return nil
}

func (d *FakeStorageDriver) GetProtocol() config.Protocol {
	return d.Config.Protocol
}

func (d *FakeStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {

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

func (d *FakeStorageDriver) GetExternalConfig() interface{} {

	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)

	return &struct {
		*drivers.CommonStorageDriverConfigExternal
		Protocol     config.Protocol              `json:"protocol"`
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

func (d *FakeStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volume, ok := d.Volumes[name]
	if !ok {
		return nil, fmt.Errorf("Fake volume %s not found.", name)
	}

	return d.getVolumeExternal(volume), nil
}

func (d *FakeStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range d.Volumes {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(volume), nil}
	}
}

func (d *FakeStorageDriver) getVolumeExternal(volume fake.Volume) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:      config.OrchestratorAPIVersion,
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

func Clone(a, b interface{}) {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	enc.Encode(a)
	dec.Decode(b)
}
