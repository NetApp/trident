// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"strconv"
	"strings"

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

// NASStorageDriver is for NFS storage provisioning
type NASStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         *api.Client
	Telemetry   *Telemetry

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func (d *NASStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *NASStorageDriver) GetTelemetry() *Telemetry {
	d.Telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *NASStorageDriver) Name() string {
	return drivers.OntapNASStorageDriverName
}

// backendName returns the name of the backend managed by this driver instance
func (d *NASStorageDriver) backendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapnas_" + d.Config.DataLIF)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *NASStorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
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
	d.Config = *config

	d.API, err = InitializeOntapDriver(config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(d, d.getStoragePoolAttributes(),
		d.backendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	// Validate the none, true/false values
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

func (d *NASStorageDriver) Terminate(backendUUID string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}
	if d.Config.AutoExportPolicy {
		policyName := getExportPolicyName(backendUUID)
		if err := deleteExportPolicy(policyName, d.API); err != nil {
			log.Warn(err)
		}
	}
	if d.Telemetry != nil {
		d.Telemetry.Stop()
	}
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

	if err := ValidateStoragePools(d.physicalPools, d.virtualPools, d.Name()); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume with the specified options
func (d *NASStorageDriver) Create(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NASStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
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
		return drivers.NewVolumeExistsError(name)
	}

	// Get candidate physical pools
	physicalPools, err := getPoolsForCreate(volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
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
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes[Size])
	if err != nil {
		return err
	}

	// Get options
	opts, err := d.GetVolumeOpts(volConfig, volAttributes)
	if err != nil {
		return err
	}

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	size := strconv.FormatUint(sizeBytes, 10)
	spaceReserve := utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes[SpaceReserve])
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes[SnapshotPolicy])
	snapshotReserve := utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes[SnapshotReserve])
	unixPermissions := utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes[UnixPermissions])
	snapshotDir := utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes[SnapshotDir])
	exportPolicy := utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes[ExportPolicy])
	securityStyle := utils.GetV(opts, "securityStyle", storagePool.InternalAttributes[SecurityStyle])
	encryption := utils.GetV(opts, "encryption", storagePool.InternalAttributes[Encryption])
	tieringPolicy := utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes[TieringPolicy])

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue()
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend.BackendUUID)
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
		"securityStyle":   securityStyle,
		"encryption":      enableEncryption,
		"tieringPolicy":   tieringPolicy,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error: %v", storagePool.Name, aggregate, aggrLimitsErr)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Create the volume
		volCreateResponse, err := d.API.VolumeCreate(
			name, aggregate, size, spaceReserve, snapshotPolicy, unixPermissions,
			exportPolicy, securityStyle, tieringPolicy, enableEncryption, snapshotReserveInt)

		if err = api.GetError(volCreateResponse, err); err != nil {
			if zerr, ok := err.(api.ZapiError); ok {
				// Handle case where the Create is passed to every Docker Swarm node
				if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
					log.WithField("volume", name).Warn("Volume create job already exists, skipping volume create on this node.")
					return nil
				}
			}

			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error creating volume %s: %v", storagePool.Name, aggregate, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
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

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// Create a volume clone
func (d *NASStorageDriver) CreateClone(volConfig *storage.VolumeConfig, storagePool *storage.Pool) error {

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":      "CreateClone",
			"Type":        "NASStorageDriver",
			"name":        name,
			"source":      source,
			"snapshot":    snapshot,
			"storagePool": storagePool,
		}
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
	}

	opts, err := d.GetVolumeOpts(volConfig, make(map[string]sa.Request))
	if err != nil {
		return err
	}

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string
	if storagePool != nil {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes[SplitOnClone]
	}

	// If storagePoolSplitOnCloneVal is still unknown, set it to backend's default value
	if storagePoolSplitOnCloneVal == "" {
		storagePoolSplitOnCloneVal = d.Config.SplitOnClone
	}

	split, err := strconv.ParseBool(utils.GetV(opts, "splitOnClone", storagePoolSplitOnCloneVal))
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

func (d *NASStorageDriver) Import(volConfig *storage.VolumeConfig, originalName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "NASStorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
			"notManaged":   volConfig.ImportNotManaged,
		}
		log.WithFields(fields).Debug(">>>> Import")
		defer log.WithFields(fields).Debug("<<<< Import")
	}

	// Ensure the volume exists
	flexvol, err := d.API.VolumeGet(originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", originalName)
	}

	// Validate the volume is what it should be
	if flexvol.VolumeIdAttributesPtr != nil {
		volumeIdAttrs := flexvol.VolumeIdAttributes()
		if volumeIdAttrs.TypePtr != nil && volumeIdAttrs.Type() != "rw" {
			log.WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("volume %s type is %s, not rw", originalName, volumeIdAttrs.Type())
		}
	}

	// Get the volume size
	if flexvol.VolumeSpaceAttributesPtr == nil || flexvol.VolumeSpaceAttributesPtr.SizePtr == nil {
		log.WithField("originalName", originalName).Errorf("Could not import volume, size not available")
		return fmt.Errorf("volume %s size not available", originalName)
	}
	volConfig.Size = strconv.FormatInt(int64(flexvol.VolumeSpaceAttributesPtr.Size()), 10)

	// Rename the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		renameResponse, err := d.API.VolumeRename(originalName, volConfig.InternalName)
		if err = api.GetError(renameResponse, err); err != nil {
			log.WithField("originalName", originalName).Errorf("Could not import volume, rename failed: %v", err)
			return fmt.Errorf("volume %s rename failed: %v", originalName, err)
		}
	}

	// Make sure we're not importing a volume without a junction path when not managed
	if volConfig.ImportNotManaged {
		if flexvol.VolumeIdAttributesPtr == nil {
			return fmt.Errorf("unable to read volume id attributes of volume %s", originalName)
		} else if flexvol.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexvol.VolumeIdAttributesPtr.JunctionPath() == "" {
			return fmt.Errorf("junction path is not set for volume %s", originalName)
		}
	}

	return nil
}

// Rename changes the name of a volume
func (d *NASStorageDriver) Rename(name string, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "NASStorageDriver",
			"name":    name,
			"newName": newName,
		}
		log.WithFields(fields).Debug(">>>> Rename")
		defer log.WithFields(fields).Debug("<<<< Rename")
	}

	renameResponse, err := d.API.VolumeRename(name, newName)
	if err = api.GetError(renameResponse, err); err != nil {
		log.WithField("name", name).Warnf("Could not rename volume: %v", err)
		return fmt.Errorf("could not rename volume %s: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASStorageDriver) Publish(volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Publish",
			"DataLIF": d.Config.DataLIF,
			"Type":    "NASStorageDriver",
			"name":    name,
		}
		log.WithFields(fields).Debug(">>>> Publish")
		defer log.WithFields(fields).Debug("<<<< Publish")
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = fmt.Sprintf("/%s", name)
	publishInfo.NfsServerIP = d.Config.DataLIF
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return publishFlexVolShare(d.API, &d.Config, publishInfo, name)
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NASStorageDriver) GetSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshot")
		defer log.WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return GetSnapshot(snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// Return the list of snapshots associated with the specified volume
func (d *NASStorageDriver) GetSnapshots(volConfig *storage.VolumeConfig) ([]*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NASStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshots")
		defer log.WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return GetSnapshots(volConfig, &d.Config, d.API, d.API.VolumeSize)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASStorageDriver) CreateSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": internalSnapName,
			"sourceVolume": internalVolName,
		}
		log.WithFields(fields).Debug(">>>> CreateSnapshot")
		defer log.WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return CreateSnapshot(snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASStorageDriver) RestoreSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer log.WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return RestoreSnapshot(snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASStorageDriver) DeleteSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer log.WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return DeleteSnapshot(snapConfig, &d.Config, d.API)
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
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.backendName())
}

// Retrieve storage backend physical pools
func (d *NASStorageDriver) GetStorageBackendPhysicalPoolNames() []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

func (d *NASStorageDriver) getStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, requests), nil
}

func (d *NASStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) {
	createPrepareCommon(d, volConfig)
}

func (d *NASStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
	volConfig.FileSystem = ""

	// Set correct junction path
	flexvol, err := d.API.VolumeGet(volConfig.InternalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", volConfig.InternalName)
	}

	if flexvol.VolumeIdAttributesPtr == nil {
		return fmt.Errorf("error reading volume id attributes for volume %s", volConfig.InternalName)
	}
	if flexvol.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexvol.VolumeIdAttributesPtr.JunctionPath() == "" {
		// Flexvol is not mounted, we need to mount it
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
		mountResponse, err := d.API.VolumeMount(volConfig.InternalName, volConfig.AccessInfo.NfsPath)
		if err = api.GetError(mountResponse, err); err != nil {
			return fmt.Errorf("error mounting volume to junction %s; %v", volConfig.AccessInfo.NfsPath, err)
		}
	} else {
		volConfig.AccessInfo.NfsPath = flexvol.VolumeIdAttributesPtr.JunctionPath()
	}
	return nil
}

func (d *NASStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.File
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

	return d.getVolumeExternal(volumeAttributes), nil
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
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volume := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
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
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

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
		Pool:   volumeIDAttrs.ContainingAggregateName(),
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetUpdateType",
			"Type":   "NASStorageDriver",
		}
		log.WithFields(fields).Debug(">>>> GetUpdateType")
		defer log.WithFields(fields).Debug("<<<< GetUpdateType")
	}

	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.DataLIF != dOrig.Config.DataLIF {
		bitmap.Add(storage.VolumeAccessInfoChange)
	}

	if d.Config.Password != dOrig.Config.Password {
		bitmap.Add(storage.PasswordChange)
	}

	if d.Config.Username != dOrig.Config.Username {
		bitmap.Add(storage.UsernameChange)
	}

	return bitmap
}

// Resize expands the volume size.
func (d *NASStorageDriver) Resize(volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "NASStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		log.WithFields(fields).Debug(">>>> Resize")
		defer log.WithFields(fields).Debug("<<<< Resize")
	}

	flexvolSize, err := resizeValidation(name, sizeBytes, d.API.VolumeExists, d.API.VolumeSize)
	if err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(flexvolSize, 10)
	if flexvolSize == sizeBytes {
		return nil
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(name, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	response, err := d.API.VolumeSetSize(name, strconv.FormatUint(sizeBytes, 10))
	if err = api.GetError(response.Result, err); err != nil {
		log.WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

func (d *NASStorageDriver) ReconcileNodeAccess(nodes []*utils.Node, backendUUID string) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NASStorageDriver",
			"Nodes":  nodeNames,
		}
		log.WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer log.WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccess(nodes, &d.Config, d.API, policyName)
}
