// Copyright 2018 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	trident "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

// NASStorageDriver is for NFS storage provisioning
type NASStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         *api.Client
	Telemetry   *Telemetry
}

func (d *NASStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *NASStorageDriver) GetTelemetry() *Telemetry {
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *NASStorageDriver) Name() string {
	return drivers.OntapNASStorageDriverName
}

// Initialize from the provided config
func (d *NASStorageDriver) Initialize(
	context trident.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NASStorageDriver"}
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

func (d *NASStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}
	d.Telemetry.Stop()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *NASStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NASStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	err := ValidateNASDriver(d.API, &d.Config)
	if err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	return nil
}

// Create a volume with the specified options
func (d *NASStorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Create",
			"Type":      "NASStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
			"opts":      opts,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
	}

	// If the volume already exists, bail out
	volExists, err := d.API.VolumeExists(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return fmt.Errorf("volume %s already exists", name)
	}

	sizeBytes, err = GetVolumeSize(sizeBytes, d.Config)
	if err != nil {
		return err
	}

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	size := strconv.FormatUint(sizeBytes, 10)
	spaceReserve := utils.GetV(opts, "spaceReserve", d.Config.SpaceReserve)
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", d.Config.SnapshotPolicy)
	unixPermissions := utils.GetV(opts, "unixPermissions", d.Config.UnixPermissions)
	snapshotDir := utils.GetV(opts, "snapshotDir", d.Config.SnapshotDir)
	exportPolicy := utils.GetV(opts, "exportPolicy", d.Config.ExportPolicy)
	aggregate := utils.GetV(opts, "aggregate", d.Config.Aggregate)
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

	log.WithFields(log.Fields{
		"name":            name,
		"size":            size,
		"spaceReserve":    spaceReserve,
		"snapshotPolicy":  snapshotPolicy,
		"unixPermissions": unixPermissions,
		"snapshotDir":     enableSnapshotDir,
		"exportPolicy":    exportPolicy,
		"aggregate":       aggregate,
		"securityStyle":   securityStyle,
		"encryption":      encryption,
	}).Debug("Creating Flexvol.")

	// Create the volume
	volCreateResponse, err := d.API.VolumeCreate(
		name, aggregate, size, spaceReserve, snapshotPolicy,
		unixPermissions, exportPolicy, securityStyle, encrypt)

	if err = api.GetError(volCreateResponse, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			// Handle case where the Create is passed to every Docker Swarm node
			if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
				log.WithField("volume", name).Warn("Volume create job already exists, skipping volume create on this node.")
				return nil
			}
		}
		return fmt.Errorf("error creating volume: %v", err)
	}

	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	if !enableSnapshotDir {
		snapDirResponse, err := d.API.VolumeDisableSnapshotDirectoryAccess(name)
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
func (d *NASStorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "NASStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
			"opts":     opts,
		}
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
	}

	split, err := strconv.ParseBool(utils.GetV(opts, "splitOnClone", d.Config.SplitOnClone))
	if err != nil {
		return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
	}

	log.WithField("splitOnClone", split).Debug("Creating volume clone.")
	return CreateOntapClone(name, source, snapshot, split, &d.Config, d.API)
}

// Destroy the volume
func (d *NASStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NASStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.

	volDestroyResponse, err := d.API.VolumeDestroy(name, true)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}
	if zerr := api.NewZapiError(volDestroyResponse); !zerr.IsPassed() {

		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			log.WithField("volume", name).Warn("Volume already deleted.")
		} else {
			return fmt.Errorf("error destroying volume %v: %v", name, zerr)
		}
	}

	return nil
}

// Attach the volume
func (d *NASStorageDriver) Attach(name, mountpoint string, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "Attach",
			"Type":       "NASStorageDriver",
			"name":       name,
			"mountpoint": mountpoint,
			"opts":       opts,
		}
		log.WithFields(fields).Debug(">>>> Attach")
		defer log.WithFields(fields).Debug("<<<< Attach")
	}

	exportPath := fmt.Sprintf("%s:/%s", d.Config.DataLIF, name)

	return MountVolume(exportPath, mountpoint, &d.Config)
}

// Detach the volume
func (d *NASStorageDriver) Detach(name, mountpoint string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "Detach",
			"Type":       "NASStorageDriver",
			"name":       name,
			"mountpoint": mountpoint,
		}
		log.WithFields(fields).Debug(">>>> Detach")
		defer log.WithFields(fields).Debug("<<<< Detach")
	}

	return UnmountVolume(mountpoint, &d.Config)
}

// Return the list of snapshots associated with the named volume
func (d *NASStorageDriver) SnapshotList(name string) ([]storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "SnapshotList",
			"Type":   "NASStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> SnapshotList")
		defer log.WithFields(fields).Debug("<<<< SnapshotList")
	}

	return GetSnapshotList(name, &d.Config, d.API)
}

// Return the list of volumes associated with this tenant
func (d *NASStorageDriver) List() ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "NASStorageDriver"}
		log.WithFields(fields).Debug(">>>> List")
		defer log.WithFields(fields).Debug("<<<< List")
	}

	return GetVolumeList(d.API, &d.Config)
}

// Test for the existence of a volume
func (d *NASStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NASStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	return GetVolume(name, d.API, &d.Config)
}

// Retrieve storage backend capabilities
func (d *NASStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		backend.Name = "ontapnas_" + d.Config.DataLIF
	} else {
		backend.Name = d.Config.BackendName
	}
	poolAttrs := d.GetStoragePoolAttributes()
	return getStorageBackendSpecsCommon(d, backend, poolAttrs)
}

func (d *NASStorageDriver) GetStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(d.API.SupportsFeature(api.NetAppVolumeEncryption)),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.Pool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, pool, requests), nil
}

func (d *NASStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	return createPrepareCommon(d, volConfig)
}

func (d *NASStorageDriver) CreateFollowup(
	volConfig *storage.VolumeConfig,
) error {
	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
	volConfig.FileSystem = ""
	return nil
}

func (d *NASStorageDriver) GetProtocol() trident.Protocol {
	return trident.File
}

func (d *NASStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NASStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttributes, err := d.API.VolumeGet(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&volumeAttributes), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(*d.Config.StoragePrefix)
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
func (d *NASStorageDriver) getVolumeExternal(
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
		Version:         trident.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
		Protocol:        trident.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    volumeExportAttrs.Policy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: volumeSecurityUnixAttrs.Permissions(),
		StorageClass:    "",
		AccessMode:      trident.ReadWriteMany,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volumeIDAttrs.ContainingAggregateName(),
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.DataLIF != dOrig.Config.DataLIF {
		bitmap.Add(storage.VolumeAccessInfoChange)
	}

	return bitmap
}
