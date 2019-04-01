// Copyright 2018 NetApp, Inc. All Rights Reserved.

package fake

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes = 1048576 // 1 MiB

	defaultLimitVolumeSize = ""

	// Constants for internal pool attributes
	Size   = "size"
	Region = "region"
	Zone   = "zone"
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

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func NewFakeStorageBackend(configJSON string) (sb *storage.Backend, err error) {

	// Parse the common config struct from JSON
	commonConfig, err := drivers.ValidateCommonSettings(configJSON)
	if err != nil {
		err = fmt.Errorf("input failed validation: %v", err)
		return nil, err
	}

	storageDriver := &StorageDriver{}

	if initializeErr := storageDriver.Initialize(
		tridentconfig.CurrentDriverContext, configJSON, commonConfig); initializeErr != nil {
		err = fmt.Errorf("problem initializing storage driver '%s': %v",
			commonConfig.StorageDriverName, initializeErr)
		return nil, err
	}

	return storage.NewStorageBackend(storageDriver)
}

func NewFakeStorageDriver(config drivers.FakeStorageDriverConfig) *StorageDriver {
	driver := &StorageDriver{
		initialized:      true,
		Config:           config,
		Volumes:          make(map[string]fake.Volume),
		DestroyedVolumes: make(map[string]bool),
	}
	driver.populateConfigurationDefaults(&config)
	driver.initializeStoragePools()
	return driver
}

func NewFakeStorageDriverConfigJSON(
	name string, protocol tridentconfig.Protocol, pools map[string]*fake.StoragePool,
) (string, error) {
	prefix := ""
	jsonBytes, err := json.Marshal(
		&drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: drivers.FakeStorageDriverName,
				StoragePrefixRaw:  json.RawMessage("\"\""),
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

func NewFakeStorageDriverConfigJSONWithVirtualPools(
	name string, protocol tridentconfig.Protocol, pools map[string]*fake.StoragePool,
	vpool drivers.FakeStorageDriverPool, vpools []drivers.FakeStorageDriverPool,
) (string, error) {
	prefix := ""
	jsonBytes, err := json.Marshal(
		&drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: drivers.FakeStorageDriverName,
				StoragePrefixRaw:  json.RawMessage("\"\""),
				StoragePrefix:     &prefix,
			},
			Protocol:              protocol,
			Pools:                 pools,
			InstanceName:          name,
			FakeStorageDriverPool: vpool,
			Storage:               vpools,
		},
	)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func (d *StorageDriver) Name() string {
	return drivers.FakeStorageDriverName
}

// poolName returns the name of the pool reported by this driver instance
func (d *StorageDriver) poolName(region string) string {
	name := fmt.Sprintf("%s_%s", d.Name(), strings.Replace(region, "-", "", -1))
	return strings.Replace(name, "__", "_", -1)
}

func (d *StorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	d.Config.CommonStorageDriverConfig = commonConfig
	err := json.Unmarshal([]byte(configJSON), &d.Config)
	if err != nil {
		return fmt.Errorf("unable to initialize fake driver: %v", err)
	}

	err = d.populateConfigurationDefaults(&d.Config)
	if err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	err = d.initializeStoragePools()
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	d.Volumes = make(map[string]fake.Volume)
	d.DestroyedVolumes = make(map[string]bool)
	d.Config.SerialNumbers = []string{d.Config.InstanceName + "_SN"}

	s, _ := json.Marshal(d.Config)
	log.Debugf("FakeStorageDriverConfig: %s", string(s))

	err = d.validate()
	if err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	for _, volume := range d.Config.Volumes {

		var requestedPool *storage.Pool
		if pool, ok := d.virtualPools[volume.RequestedPool]; ok {
			requestedPool = pool
		} else if pool, ok = d.physicalPools[volume.RequestedPool]; ok {
			requestedPool = pool
		} else {
			return fmt.Errorf("requested pool %s for volume %s does not exist", volume.RequestedPool, volume.Name)
		}

		volConfig := &storage.VolumeConfig{
			Version:      "1",
			InternalName: volume.Name,
			Size:         strconv.FormatUint(volume.SizeBytes, 10),
		}
		if err = d.Create(volConfig, requestedPool, make(map[string]sa.Request)); err != nil {
			return fmt.Errorf("error creating volume %s; %v", volume.Name, err)
		}

		newVolume := d.Volumes[volume.Name]
		log.WithFields(log.Fields{
			"Name":          newVolume.Name,
			"Size":          newVolume.SizeBytes,
			"RequestedPool": newVolume.RequestedPool,
			"PhysicalPool":  newVolume.PhysicalPool,
		}).Debug("Added new volume.")
	}

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

	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		config.StoragePrefix = &prefix
		config.StoragePrefixRaw = json.RawMessage("\"" + *config.StoragePrefix + "\"")
	}

	// Ensure the default volume size is valid, using a "default default" of 1G if not set
	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	log.WithFields(log.Fields{
		"Size": config.Size,
	}).Debugf("Configuration defaults")

	return nil
}

func (d *StorageDriver) initializeStoragePools() error {

	d.physicalPools = make(map[string]*storage.Pool)
	d.virtualPools = make(map[string]*storage.Pool)

	snapshotOffers := make([]sa.Offer, 0)
	cloneOffers := make([]sa.Offer, 0)
	encryptionOffers := make([]sa.Offer, 0)
	provisioningTypeOffers := make([]sa.Offer, 0)
	mediaOffers := make([]sa.Offer, 0)

	// Define physical pools
	for name, fakeStoragePool := range d.Config.Pools {

		pool := storage.NewStoragePool(nil, name)

		pool.Attributes = fakeStoragePool.Attrs
		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		pool.Attributes[sa.Region] = sa.NewStringOffer(d.Config.Region)
		if d.Config.Zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		if snapshotOffer, ok := pool.Attributes[sa.Snapshots]; ok {
			snapshotOffers = append(snapshotOffers, snapshotOffer)
		}
		if cloneOffer, ok := pool.Attributes[sa.Clones]; ok {
			cloneOffers = append(cloneOffers, cloneOffer)
		}
		if encryptionOffer, ok := pool.Attributes[sa.Encryption]; ok {
			encryptionOffers = append(encryptionOffers, encryptionOffer)
		}
		if provisioningTypeOffer, ok := pool.Attributes[sa.ProvisioningType]; ok {
			provisioningTypeOffers = append(provisioningTypeOffers, provisioningTypeOffer)
		}
		if mediaOffer, ok := pool.Attributes[sa.Media]; ok {
			mediaOffers = append(mediaOffers, mediaOffer)
		}

		pool.InternalAttributes[Size] = d.Config.Size
		pool.InternalAttributes[Region] = d.Config.Region
		pool.InternalAttributes[Zone] = d.Config.Zone

		d.physicalPools[pool.Name] = pool
	}

	// Define virtual pools
	for index, vpool := range d.Config.Storage {

		region := d.Config.Region
		if vpool.Region != "" {
			region = vpool.Region
		}

		zone := d.Config.Zone
		if vpool.Zone != "" {
			zone = vpool.Zone
		}

		size := d.Config.Size
		if vpool.Size != "" {
			size = vpool.Size
		}

		pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf(region+"_pool_%d", index)))

		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		pool.Attributes[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)
		pool.Attributes[sa.Region] = sa.NewStringOffer(region)
		if zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(zone)
		}

		if len(snapshotOffers) > 0 {
			pool.Attributes[sa.Snapshots] = sa.NewBoolOfferFromOffers(snapshotOffers...)
		}
		if len(cloneOffers) > 0 {
			pool.Attributes[sa.Clones] = sa.NewBoolOfferFromOffers(cloneOffers...)
		}
		if len(encryptionOffers) > 0 {
			pool.Attributes[sa.Encryption] = sa.NewBoolOfferFromOffers(encryptionOffers...)
		}
		if len(provisioningTypeOffers) > 0 {
			pool.Attributes[sa.ProvisioningType] = sa.NewStringOfferFromOffers(provisioningTypeOffers...)
		}
		if len(mediaOffers) > 0 {
			pool.Attributes[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
		}

		pool.InternalAttributes[Size] = size
		pool.InternalAttributes[Region] = region
		pool.InternalAttributes[Zone] = zone

		d.virtualPools[pool.Name] = pool
	}

	return nil
}

// validate ensures the driver configuration and execution environment are valid and working
func (d *StorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "StorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	// Validate driver-level attributes

	// Validate pool-level attributes
	allPools := make([]*storage.Pool, 0, len(d.physicalPools)+len(d.virtualPools))

	for _, pool := range d.physicalPools {
		allPools = append(allPools, pool)
	}
	for _, pool := range d.virtualPools {
		allPools = append(allPools, pool)
	}

	for _, pool := range allPools {

		// Validate default size
		if _, err := utils.ConvertSizeToBytes(pool.InternalAttributes[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", pool.Name, err)
		}
	}

	return nil
}

func (d *StorageDriver) Create(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName
	if _, ok := d.Volumes[name]; ok {
		return drivers.NewVolumeExistsError(name)
	}

	// Get candidate physical pools
	physicalPools, err := d.getPoolsForCreate(volConfig, storagePool, volAttributes)
	if err != nil {
		return err
	}

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(d.Config.Size)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return fmt.Errorf("requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes)
	}

	if _, _, err = drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	createErrors := make([]error, 0)

	for _, physicalPool := range physicalPools {

		fakePoolName := physicalPool.Name
		fakePool, ok := d.Config.Pools[physicalPool.Name]
		if !ok {
			errMessage := fmt.Sprintf("fake pool %s not found.", fakePoolName)
			log.Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		if sizeBytes > fakePool.Bytes {
			errMessage := fmt.Sprintf("requested volume is too large, requested %d bytes, "+
				"have %d available in pool %s", sizeBytes, fakePool.Bytes, fakePoolName)
			log.Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		d.Volumes[name] = fake.Volume{
			Name:          name,
			RequestedPool: storagePool.Name,
			PhysicalPool:  fakePoolName,
			SizeBytes:     sizeBytes,
		}
		d.DestroyedVolumes[name] = false
		fakePool.Bytes -= sizeBytes

		log.WithFields(log.Fields{
			"backend":       d.Config.InstanceName,
			"name":          name,
			"requestedPool": storagePool.Name,
			"physicalPool":  fakePoolName,
			"sizeBytes":     sizeBytes,
		}).Debug("Created fake volume.")

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors)
}

func (d *StorageDriver) getPoolsForCreate(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) ([]*storage.Pool, error) {

	// If a physical pool was requested, just use it
	if _, ok := d.physicalPools[storagePool.Name]; ok {
		return []*storage.Pool{storagePool}, nil
	}

	// If a virtual pool was requested, find a physical pool to satisfy it
	if _, ok := d.virtualPools[storagePool.Name]; !ok {
		return nil, fmt.Errorf("could not find pool %s", storagePool.Name)
	}

	// Make a storage class from the volume attributes to simplify pool matching
	attributesCopy := make(map[string]sa.Request)
	for k, v := range volAttributes {
		attributesCopy[k] = v
	}
	delete(attributesCopy, sa.Selector)
	storageClass := sc.NewFromAttributes(attributesCopy)

	// Find matching pools
	candidatePools := make([]*storage.Pool, 0)

	for _, pool := range d.physicalPools {
		if storageClass.Matches(pool) {
			candidatePools = append(candidatePools, pool)
		}
	}

	if len(candidatePools) == 0 {
		err := errors.New("backend has no physical pools that can satisfy request")
		return nil, drivers.NewBackendIneligibleError(volConfig.InternalName, []error{err})
	}

	// Shuffle physical pools
	rand.Shuffle(len(candidatePools), func(i, j int) {
		candidatePools[i], candidatePools[j] = candidatePools[j], candidatePools[i]
	})

	return candidatePools, nil
}

func (d *StorageDriver) CreateClone(volConfig *storage.VolumeConfig) error {

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

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
	physicalPool := sourceVolume.PhysicalPool
	fakePool, ok := d.Config.Pools[physicalPool]
	if !ok {
		return fmt.Errorf("could not find pool %s", physicalPool)
	}

	// Use the same size as the source
	sizeBytes := sourceVolume.SizeBytes
	if sizeBytes > fakePool.Bytes {
		return fmt.Errorf("requested clone is too large: requested %d bytes; have %d available in pool %s",
			sizeBytes, fakePool.Bytes, physicalPool)
	}

	d.Volumes[name] = fake.Volume{
		Name:          name,
		RequestedPool: sourceVolume.RequestedPool,
		PhysicalPool:  physicalPool,
		SizeBytes:     sizeBytes,
	}
	d.DestroyedVolumes[name] = false
	fakePool.Bytes -= sizeBytes

	log.WithFields(log.Fields{
		"backend":       d.Config.InstanceName,
		"Name":          name,
		"source":        sourceVolume.Name,
		"snapshot":      snapshot,
		"requestedPool": sourceVolume.RequestedPool,
		"physicalPool":  physicalPool,
		"SizeBytes":     sizeBytes,
	}).Debug("Cloned fake volume.")

	return nil
}

func (d *StorageDriver) Import(volumeConfig *storage.VolumeConfig, originalName string, notManaged bool) error {

	log.WithFields(log.Fields{
		"volumeConfig": volumeConfig,
		"originalName": originalName,
	}).Debug("Import")

	importVolume, ok := d.Volumes[originalName]
	if !ok {
		return fmt.Errorf("import volume %s not found", originalName)
	}

	volumeConfig.Size = strconv.FormatUint(importVolume.SizeBytes, 10)

	if !notManaged {
		d.Volumes[volumeConfig.InternalName] = importVolume
		delete(d.Volumes, originalName)
	}

	return nil
}

func (d *StorageDriver) CreateVolumeFromSnapshot(snapshot *storage.Snapshot, volConfig *storage.VolumeConfig,
	storagePool *storage.Pool, volAttributes map[string]sa.Request) error {

	name := volConfig.InternalName
	if _, ok := d.Volumes[name]; ok {
		return drivers.NewVolumeExistsError(name)
	}

	// Check if the snapshot exists
	if _, ok := d.Snapshots[snapshot.Name]; !ok {
		return fmt.Errorf("source snapshot %s not found", snapshot.Name)
	}

	// Get candidate physical pools
	physicalPools, err := d.getPoolsForCreate(volConfig, storagePool, volAttributes)
	if err != nil {
		return err
	}

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(d.Config.Size)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return fmt.Errorf("requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes)
	}

	if _, _, err = drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	createErrors := make([]error, 0)

	for _, physicalPool := range physicalPools {

		fakePoolName := physicalPool.Name
		fakePool, ok := d.Config.Pools[physicalPool.Name]
		if !ok {
			errMessage := fmt.Sprintf("fake pool %s not found.", fakePoolName)
			log.Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		if sizeBytes > fakePool.Bytes {
			errMessage := fmt.Sprintf("requested volume is too large, requested %d bytes, "+
				"have %d available in pool %s", sizeBytes, fakePool.Bytes, fakePoolName)
			log.Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		d.Volumes[name] = fake.Volume{
			Name:          name,
			RequestedPool: storagePool.Name,
			PhysicalPool:  fakePoolName,
			SizeBytes:     sizeBytes,
		}
		d.DestroyedVolumes[name] = false
		fakePool.Bytes -= sizeBytes

		log.WithFields(log.Fields{
			"backend":        d.Config.InstanceName,
			"name":           name,
			"sourceSnapshot": snapshot.Name,
			"requestedPool":  storagePool.Name,
			"physicalPool":   fakePoolName,
			"sizeBytes":      sizeBytes,
		}).Info("Created fake volume from snapshot.")

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors)
}

func (d *StorageDriver) Rename(name string, newName string) error {

	log.WithFields(log.Fields{
		"name":    name,
		"newName": newName,
	}).Debug("Rename")

	volume, ok := d.Volumes[name]
	if !ok {
		return fmt.Errorf("volume to rename %s not found", name)
	}
	d.Volumes[newName] = volume
	delete(d.Volumes, name)

	return nil
}

func (d *StorageDriver) Destroy(name string) error {

	d.DestroyedVolumes[name] = true

	volume, ok := d.Volumes[name]
	if !ok {
		return nil
	}

	physicalPool := volume.PhysicalPool
	fakePool, ok := d.Config.Pools[physicalPool]
	if !ok {
		return fmt.Errorf("could not find pool %s", physicalPool)
	}

	fakePool.Bytes += volume.SizeBytes
	delete(d.Volumes, name)

	log.WithFields(log.Fields{
		"backend":       d.Config.InstanceName,
		"Name":          name,
		"requestedPool": volume.RequestedPool,
		"physicalPool":  physicalPool,
		"sizeBytes":     volume.SizeBytes,
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

func (d *StorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {

	if d.Config.BackendName == "" {
		// Use the old naming scheme if no backend is specified
		backend.Name = d.Config.InstanceName
	} else {
		backend.Name = d.Config.BackendName
	}

	virtual := len(d.virtualPools) > 0

	for _, pool := range d.physicalPools {
		pool.Backend = backend
		if !virtual {
			backend.AddStoragePool(pool)
		}
	}

	for _, pool := range d.virtualPools {
		pool.Backend = backend
		if virtual {
			backend.AddStoragePool(pool)
		}
	}

	return nil
}

func (d *StorageDriver) GetInternalVolumeName(name string) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// With an external store, any transformation of the name is fine
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = strings.Replace(internal, "--", "-", -1) // Remove any double hyphens
		internal = strings.Replace(internal, "__", "_", -1) // Remove any double underscores
		internal = strings.Replace(internal, "_-", "-", -1) // Remove any strange delimiter
		return internal
	}
}

func (d *StorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) error {
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal = d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}
	return nil
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
	if cloneCommonConfig.StoragePrefix == nil {
		cloneCommonConfig.StoragePrefixRaw = json.RawMessage("{}")
	} else {
		cloneCommonConfig.StoragePrefixRaw = json.RawMessage("\"" + *cloneCommonConfig.StoragePrefix + "\"")
	}
	cloneCommonConfig.StoragePrefix = nil

	var cloneFakePool drivers.FakeStorageDriverPool
	Clone(&d.Config.FakeStorageDriverPool, &cloneFakePool)

	var cloneFakePools []drivers.FakeStorageDriverPool
	Clone(&d.Config.Storage, &cloneFakePools)

	b.FakeStorageDriverConfig = &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &cloneCommonConfig,
		Protocol:                  d.Config.Protocol,
		Pools:                     d.Config.Pools,
		InstanceName:              d.Config.InstanceName,
		Storage:                   cloneFakePools,
		FakeStorageDriverPool:     cloneFakePool,
	}
}

func (d *StorageDriver) GetExternalConfig() interface{} {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	return d.Config
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
		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(volume), Error: nil}
	}
}

func (d *StorageDriver) getVolumeExternal(volume fake.Volume) *storage.VolumeExternal {

	internalName := volume.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:      tridentconfig.OrchestratorAPIVersion,
		Name:         name,
		InternalName: internalName,
		Size:         strconv.FormatUint(volume.SizeBytes, 10),
	}

	volumeExternal := &storage.VolumeExternal{
		Config:  volumeConfig,
		Backend: d.Name(),
		Pool:    drivers.UnsetPool,
	}

	return volumeExternal
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *StorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {

	bitmap := roaring.New()
	_, ok := driverOrig.(*StorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	return roaring.New()
}

func Clone(a, b interface{}) {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	enc.Encode(a)
	dec.Decode(b)
}
