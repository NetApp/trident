package ontap

import (
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"strconv"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

// NASFlexGroupStorageDriver is for NFS FlexGroup storage provisioning
type NASFlexGroupStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         *api.Client
	Telemetry   *Telemetry
}

func (d *NASFlexGroupStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASFlexGroupStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *NASFlexGroupStorageDriver) GetTelemetry() *Telemetry {
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *NASFlexGroupStorageDriver) Name() string {
	return drivers.OntapNASFlexGroupStorageDriverName
}

// Initialize from the provided config
func (d *NASFlexGroupStorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NASFlexGroupStorageDriver"}
		log.WithFields(fields).Debug(">>>> Initialize")
		defer log.WithFields(fields).Debug("<<<< Initialize")
	}

	// Parse the config
	config, err := InitializeOntapConfig(context, configJSON, commonConfig)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}

	d.API, err = InitializeOntapDriver(config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	err = d.validate()
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.Telemetry = NewOntapTelemetry(d)
	d.Telemetry.Start()

	d.initialized = true
	return nil
}

func (d *NASFlexGroupStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASFlexGroupStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASFlexGroupStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}
	d.Telemetry.Stop()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *NASFlexGroupStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NASFlexGroupStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	if !d.API.SupportsFeature(api.NetAppFlexGroups) {
		return fmt.Errorf("ONTAP version does not support FlexGroups")
	}

	err := ValidateNASDriver(d.API, &d.Config)
	if err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	return nil
}

// Create a volume with the specified options
func (d *NASFlexGroupStorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Create",
			"Type":      "NASFlexGroupStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
			"opts":      opts,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
	}

	// If the volume already exists, bail out
	volExists, err := d.API.FlexGroupExists(name)
	if err != nil {
		return fmt.Errorf("error checking for existing FlexGroup: %v", err)
	}
	if volExists {
		return fmt.Errorf("FlexGroup %s already exists", name)
	}

	sizeBytes, err = GetVolumeSize(sizeBytes, d.Config)
	if err != nil {
		return err
	}
	if sizeBytes > math.MaxInt64 {
		return errors.New("invalid size requested")
	}
	size := int(sizeBytes)

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.GetVserverAggregateNames()
	if err != nil {
		return err
	}

	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("no assigned aggregates found")
		return err
	}

	vserverAggrNames := make([]azgo.AggrNameType, 0)
	for _, aggrName := range vserverAggrs {
		vserverAggrNames = append(vserverAggrNames, azgo.AggrNameType(aggrName))
	}

	log.WithFields(log.Fields{
		"aggregates": vserverAggrs,
	}).Debug("Read aggregates assigned to SVM.")

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	spaceReserve := utils.GetV(opts, "spaceReserve", d.Config.SpaceReserve)
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", d.Config.SnapshotPolicy)
	snapshotReserve := utils.GetV(opts, "snapshotReserve", d.Config.SnapshotReserve)
	unixPermissions := utils.GetV(opts, "unixPermissions", d.Config.UnixPermissions)
	snapshotDir := utils.GetV(opts, "snapshotDir", d.Config.SnapshotDir)
	exportPolicy := utils.GetV(opts, "exportPolicy", d.Config.ExportPolicy)
	securityStyle := utils.GetV(opts, "securityStyle", d.Config.SecurityStyle)
	encryption := utils.GetV(opts, "encryption", d.Config.Encryption)

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	encrypt, err := ValidateEncryptionAttribute(encryption, d.API)
	if err != nil {
		return err
	}

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	log.WithFields(log.Fields{
		"name":            name,
		"size":            size,
		"spaceReserve":    spaceReserve,
		"snapshotPolicy":  snapshotPolicy,
		"snapshotReserve": snapshotReserveInt,
		"unixPermissions": unixPermissions,
		"snapshotDir":     enableSnapshotDir,
		"exportPolicy":    exportPolicy,
		"aggregates":      vserverAggrNames,
		"securityStyle":   securityStyle,
		"encryption":      encryption,
	}).Debug("Creating FlexGroup.")

	// Create the FlexGroup
	_, err = d.API.FlexGroupCreate(
		name, size, vserverAggrNames, spaceReserve, snapshotPolicy,
		unixPermissions, exportPolicy, securityStyle, encrypt, snapshotReserveInt)

	if err != nil {
		return fmt.Errorf("error creating FlexGroup %v: %v", name, err)
	}

	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	if !enableSnapshotDir {
		snapDirResponse, err := d.API.FlexGroupVolumeDisableSnapshotDirectoryAccess(name)
		if err = api.GetError(snapDirResponse, err); err != nil {
			return fmt.Errorf("error disabling snapshot directory access: %v", err)
		}
	}

	// Mount the volume at the specified junction
	mountResponse, err := d.API.VolumeMount(name, "/"+name)
	if err = api.GetError(mountResponse, err); err != nil {
		return fmt.Errorf("error mounting volume to junction: %v", err)
	}

	// If LS mirrors are present on the SVM root volume, update them
	UpdateLoadSharingMirrors(d.API)

	return nil
}

// Create a volume clone
func (d *NASFlexGroupStorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {
	return errors.New("clones are not supported for FlexGroups")
}

// Destroy the volume
func (d *NASFlexGroupStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NASFlexGroupStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	// Needed once FlexGroups support clones
	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.

	volDestroyResponse, err := d.API.FlexGroupDestroy(name, true)
	if err != nil {
		return fmt.Errorf("error destroying FlexGroup %v: %v", name, err)
	}
	if zerr := api.NewZapiError(volDestroyResponse); !zerr.IsPassed() {

		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			log.WithField("volume", name).Warn("FlexGroup already deleted.")
		} else {
			return fmt.Errorf("error destroying FlexGroup %v: %v", name, zerr)
		}
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASFlexGroupStorageDriver) Publish(name string, publishInfo *utils.VolumePublishInfo) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "NASFlexGroupStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Publish")
		defer log.WithFields(fields).Debug("<<<< Publish")
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = fmt.Sprintf("/%s", name)
	publishInfo.NfsServerIP = d.Config.DataLIF
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = d.Config.NfsMountOptions

	return nil
}

// Return the list of snapshots associated with the named volume
func (d *NASFlexGroupStorageDriver) SnapshotList(name string) ([]storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "SnapshotList",
			"Type":   "NASFlexGroupStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> SnapshotList")
		defer log.WithFields(fields).Debug("<<<< SnapshotList")
	}

	return GetSnapshotList(name, &d.Config, d.API)
}

// Tests the existence of a FlexGroup. Returns nil if the FlexGroup
// exists and an error otherwise.
func (d *NASFlexGroupStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NASFlexGroupStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	volExists, err := d.API.FlexGroupExists(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		log.WithField("FlexGroup", name).Debug("FlexGroup not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// Retrieve storage backend capabilities
func (d *NASFlexGroupStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		backend.Name = "ontapnas_" + d.Config.DataLIF
	} else {
		backend.Name = d.Config.BackendName
	}
	poolAttrs := d.GetStoragePoolAttributes()
	return d.getStorageBackendSpecsCommon(backend, poolAttrs)
}

// getStorageBackendSpecsCommon discovers the aggregates assigned to the configured SVM. The aggregates assigned to
// a SVM represent a single StoragePool for a FlexGroup. The default attributes for a FlexGroup are assigned to the pool.
func (d *NASFlexGroupStorageDriver) getStorageBackendSpecsCommon(
	backend *storage.Backend, poolAttributes map[string]sa.Offer) (err error) {

	config := d.GetConfig()

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.GetVserverAggregateNames()
	if err != nil {
		return
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates", config.SVM)
		return
	}

	log.WithFields(log.Fields{
		"svm":        config.SVM,
		"aggregates": vserverAggrs,
	}).Debug("Read aggregates assigned to SVM.")

	// For a FlexGroup all aggregates that belong to the SVM represent the storage pool.
	storagePools := make(map[string]*storage.Pool)

	pool := storage.NewStoragePool(backend, config.SVM)

	pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
	pool.Attributes[sa.Clones] = sa.NewBoolOffer(false)
	pool.Attributes[sa.Encryption] = sa.NewBoolOffer(true)

	storagePools[backend.Name] = pool
	for attrName, offer := range poolAttributes {
		pool.Attributes[attrName] = offer
	}
	backend.AddStoragePool(pool)

	return
}

func (d *NASFlexGroupStorageDriver) GetStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType: sa.NewStringOffer(d.Name()),
		sa.Snapshots:   sa.NewBoolOffer(true),
		sa.Encryption:  sa.NewBoolOffer(d.API.SupportsFeature(api.NetAppVolumeEncryption)),
	}
}

func (d *NASFlexGroupStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.Pool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, pool, requests), nil
}

func (d *NASFlexGroupStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASFlexGroupStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	return createPrepareCommon(d, volConfig)
}

func (d *NASFlexGroupStorageDriver) CreateFollowup(
	volConfig *storage.VolumeConfig,
) error {
	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
	volConfig.FileSystem = ""
	return nil
}

func (d *NASFlexGroupStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NASFlexGroupStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASFlexGroupStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NASFlexGroupStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttributes, err := d.API.FlexGroupGet(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&volumeAttributes), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASFlexGroupStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.FlexGroupGetAll(*d.Config.StoragePrefix)
	if err = api.GetError(volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range volumesResponse.Result.AttributesList() {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(&volume), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASFlexGroupStorageDriver) getVolumeExternal(
	volumeAttrs *azgo.VolumeAttributesType) *storage.VolumeExternal {

	volumeExportAttrs := volumeAttrs.VolumeExportAttributesPtr
	volumeIDAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSecurityAttrs := volumeAttrs.VolumeSecurityAttributesPtr
	volumeSecurityUnixAttrs := volumeSecurityAttrs.VolumeSecurityUnixAttributesPtr
	volumeSpaceAttrs := volumeAttrs.VolumeSpaceAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := string(volumeIDAttrs.Name())
	name := internalName[len(*d.Config.StoragePrefix):]

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    volumeExportAttrs.Policy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: volumeSecurityUnixAttrs.Permissions(),
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volumeIDAttrs.OwningVserverName(),
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASFlexGroupStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASFlexGroupStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.DataLIF != dOrig.Config.DataLIF {
		bitmap.Add(storage.VolumeAccessInfoChange)
	}

	return bitmap
}
