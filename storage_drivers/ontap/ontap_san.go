// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"errors"
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

func lunPath(name string) string {
	return fmt.Sprintf("/vol/%v/lun0", name)
}

// SANStorageDriver is for iSCSI storage provisioning
type SANStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	API         *api.Client
	Telemetry   *Telemetry

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func (d *SANStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *SANStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *SANStorageDriver) GetTelemetry() *Telemetry {
	d.Telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d SANStorageDriver) Name() string {
	return drivers.OntapSANStorageDriverName
}

// backendName returns the name of the backend managed by this driver instance
func (d *SANStorageDriver) backendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapsan_" + d.ips[0])
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *SANStorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "SANStorageDriver"}
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

	d.ips, err = d.API.NetInterfaceGetDataLIFs("iscsi")
	if err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no iSCSI data LIFs found on SVM %s", config.SVM)
	} else {
		log.WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
	}

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(d, d.getStoragePoolAttributes(),
		d.backendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = InitializeSANDriver(context, d.API, &d.Config, d.validate)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.Telemetry = NewOntapTelemetry(d)
	d.Telemetry.Start()

	d.initialized = true
	return nil
}

func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANStorageDriver) Terminate(string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}
	if d.Telemetry != nil {
		d.Telemetry.Stop()
	}
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *SANStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	if err := ValidateSANDriver(d.API, &d.Config, d.ips); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePools(d.physicalPools, d.virtualPools, d.Name()); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume+LUN with the specified options
func (d *SANStorageDriver) Create(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	var fstype string

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "SANStorageDriver",
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

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	size := strconv.FormatUint(sizeBytes, 10)
	spaceAllocation, _ := strconv.ParseBool(utils.GetV(opts, "spaceAllocation", storagePool.InternalAttributes[SpaceAllocation]))
	spaceReserve := utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes[SpaceReserve])
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes[SnapshotPolicy])
	snapshotReserve := utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes[SnapshotReserve])
	unixPermissions := utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes[UnixPermissions])
	snapshotDir := "false"
	exportPolicy := utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes[ExportPolicy])
	securityStyle := utils.GetV(opts, "securityStyle", storagePool.InternalAttributes[SecurityStyle])
	encryption := utils.GetV(opts, "encryption", storagePool.InternalAttributes[Encryption])
	tieringPolicy := utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes[TieringPolicy])

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	fstype, err = drivers.CheckSupportedFilesystem(utils.GetV(opts, "fstype|fileSystemType",
		storagePool.InternalAttributes[FileSystemType]), name)
	if err != nil {
		return err
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue()
	}

	log.WithFields(log.Fields{
		"name":            name,
		"size":            size,
		"spaceAllocation": spaceAllocation,
		"spaceReserve":    spaceReserve,
		"snapshotPolicy":  snapshotPolicy,
		"snapshotReserve": snapshotReserveInt,
		"unixPermissions": unixPermissions,
		"snapshotDir":     snapshotDir,
		"exportPolicy":    exportPolicy,
		"securityStyle":   securityStyle,
		"encryption":      enableEncryption,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error: %v", storagePool.Name, aggregate, aggrLimitsErr)
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
					log.WithField("volume", name).Warn("Volume create job already exists, " +
						"skipping volume create on this node.")
					return nil
				}
			}

			errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error creating volume %s: %v", storagePool.Name,
				aggregate, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		lunPath := lunPath(name)
		osType := "linux"

		// Create the LUN
		lunCreateResponse, err := d.API.LunCreate(lunPath, int(sizeBytes), osType, false, spaceAllocation)
		if err = api.GetError(lunCreateResponse, err); err != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error creating LUN %s: %v", storagePool.Name,
				aggregate, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Save the fstype in a LUN attribute so we know what to do in Attach
		attrResponse, err := d.API.LunSetAttribute(lunPath, LUNAttributeFSType, fstype)
		if err = api.GetError(attrResponse, err); err != nil {
			defer d.API.LunDestroy(lunPath)
			return fmt.Errorf("ONTAP-SAN pool %s/%s; error saving file system type for LUN %s: %v", storagePool.Name,
				aggregate, name, err)
		}
		// Save the context
		attrResponse, err = d.API.LunSetAttribute(lunPath, "context", string(d.Config.DriverContext))
		if err = api.GetError(attrResponse, err); err != nil {
			log.WithField("name", name).Warning("Failed to save the driver context attribute for new volume.")
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// Create a volume clone
func (d *SANStorageDriver) CreateClone(volConfig *storage.VolumeConfig, storagePool *storage.Pool) error {

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":      "CreateClone",
			"Type":        "SANStorageDriver",
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

func (d *SANStorageDriver) Import(volConfig *storage.VolumeConfig, originalName string) error {
	return errors.New("import is not implemented")
}

func (d *SANStorageDriver) Rename(name string, newName string) error {
	return errors.New("rename is not implemented")
}

// Destroy the requested (volume,lun) storage tuple
func (d *SANStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	// Validate Flexvol exists before trying to destroy
	volExists, err := d.API.VolumeExists(name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		log.WithField("volume", name).Debug("Volume already deleted, skipping destroy.")
		return nil
	}

	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfo(d.API, &d.Config)
		if err != nil {
			log.WithField("error", err).Error("Could not get target info.")
			return err
		}

		// Get the LUN ID
		lunPath := fmt.Sprintf("/vol/%s/lun0", name)
		lunMapResponse, err := d.API.LunMapListInfo(lunPath)
		if err != nil {
			return fmt.Errorf("error reading LUN maps for volume %s: %v", name, err)
		}
		lunID = -1
		if lunMapResponse.Result.InitiatorGroupsPtr != nil {
			for _, lunMapResponse := range lunMapResponse.Result.InitiatorGroupsPtr.InitiatorGroupInfoPtr {
				if lunMapResponse.InitiatorGroupName() == d.Config.IgroupName {
					lunID = lunMapResponse.LunId()
				}
			}
		}
		if lunID >= 0 {
			// Inform the host about the device removal
			utils.PrepareDeviceForRemoval(lunID, iSCSINodeName)
		}
	}

	// Delete the Flexvol & LUN
	volDestroyResponse, err := d.API.VolumeDestroy(name, true)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}
	if zerr := api.NewZapiError(volDestroyResponse); !zerr.IsPassed() {
		// Handle case where the Destroy is passed to every Docker Swarm node
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			log.WithField("volume", name).Warn("Volume already deleted.")
		} else {
			return fmt.Errorf("error destroying volume %v: %v", name, zerr)
		}
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Publish(volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Publish")
		defer log.WithFields(fields).Debug("<<<< Publish")
	}

	lunPath := lunPath(name)
	igroupName := d.Config.IgroupName

	// Get target info
	iSCSINodeName, _, err := GetISCSITargetInfo(d.API, &d.Config)
	if err != nil {
		return err
	}

	err = PublishLUN(d.API, &d.Config, d.ips, publishInfo, lunPath, igroupName, iSCSINodeName)
	if err != nil {
		return fmt.Errorf("error publishing %s driver: %v", d.Name(), err)
	}

	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *SANStorageDriver) GetSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshot")
		defer log.WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return GetSnapshot(snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// Return the list of snapshots associated with the specified volume
func (d *SANStorageDriver) GetSnapshots(volConfig *storage.VolumeConfig) ([]*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "SANStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshots")
		defer log.WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return GetSnapshots(volConfig, &d.Config, d.API, d.API.VolumeSize)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *SANStorageDriver) CreateSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": internalSnapName,
			"sourceVolume": internalVolName,
		}
		log.WithFields(fields).Debug(">>>> CreateSnapshot")
		defer log.WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return CreateSnapshot(snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANStorageDriver) RestoreSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer log.WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return RestoreSnapshot(snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *SANStorageDriver) DeleteSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer log.WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return DeleteSnapshot(snapConfig, &d.Config, d.API)
}

// Test for the existence of a volume
func (d *SANStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	return GetVolume(name, d.API, &d.Config)
}

// Retrieve storage backend capabilities
func (d *SANStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.backendName())
}

// Retrieve storage backend physical pools
func (d *SANStorageDriver) GetStorageBackendPhysicalPoolNames() []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

func (d *SANStorageDriver) getStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *SANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, requests), nil
}

func (d *SANStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *SANStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) {
	createPrepareCommon(d, volConfig)
}

func (d *SANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateFollowup",
			"Type":         "SANStorageDriver",
			"name":         volConfig.Name,
			"internalName": volConfig.InternalName,
		}
		log.WithFields(fields).Debug(">>>> CreateFollowup")
		defer log.WithFields(fields).Debug("<<<< CreateFollowup")
	}

	if d.Config.DriverContext == tridentconfig.ContextDocker {
		log.Debug("No follow-up create actions for Docker.")
		return nil
	}

	return d.mapOntapSANLun(volConfig)
}

func (d *SANStorageDriver) mapOntapSANLun(volConfig *storage.VolumeConfig) error {

	// get the lunPath and lunID
	lunPath := fmt.Sprintf("/vol/%v/lun0", volConfig.InternalName)
	lunID, err := d.API.LunMapIfNotMapped(d.Config.IgroupName, lunPath)
	if err != nil {
		return err
	}

	err = PopulateOntapLunMapping(d.API, &d.Config, d.ips, volConfig, lunID, lunPath, d.Config.IgroupName)
	if err != nil {
		return fmt.Errorf("error mapping LUN for %s driver: %v", d.Name(), err)
	}

	return nil
}

func (d *SANStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *SANStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SANStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.API.VolumeGet(name)
	if err != nil {
		return nil, err
	}

	lunPath := fmt.Sprintf("/vol/%v/lun0", name)
	lunAttrs, err := d.API.LunGet(lunPath)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(lunAttrs, volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *SANStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(*d.Config.StoragePrefix)
	if err = api.GetError(volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all LUNs named 'lun0' in volumes matching the storage prefix
	lunPathPattern := fmt.Sprintf("/vol/%v/lun0", *d.Config.StoragePrefix+"*")
	lunsResponse, err := d.API.LunGetAll(lunPathPattern)
	if err = api.GetError(lunsResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with LUNs
	volumeMap := make(map[string]azgo.VolumeAttributesType)
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volumeAttrs := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			internalName := volumeAttrs.VolumeIdAttributesPtr.Name()
			volumeMap[internalName] = volumeAttrs
		}
	}

	// Convert all LUNs to VolumeExternal and write them to the channel
	if lunsResponse.Result.AttributesListPtr != nil {
		for _, lun := range lunsResponse.Result.AttributesListPtr.LunInfoPtr {

			volume, ok := volumeMap[lun.Volume()]
			if !ok {
				log.WithField("path", lun.Path()).Warning("Flexvol not found for LUN.")
				continue
			}

			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&lun, &volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SANStorageDriver) getVolumeExternal(
	lunAttrs *azgo.LunInfoType, volumeAttrs *azgo.VolumeAttributesType,
) *storage.VolumeExternal {

	volumeIDAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := volumeIDAttrs.Name()
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(lunAttrs.Size()), 10),
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
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
func (d *SANStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANStorageDriver)
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
func (d *SANStorageDriver) Resize(volConfig *storage.VolumeConfig, sizeBytes uint64) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "SANStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		log.WithFields(fields).Debug(">>>> Resize")
		defer log.WithFields(fields).Debug("<<<< Resize")
	}

	// Validation checks
	volExists, err := d.API.VolumeExists(name)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"name":  name,
		}).Error("Error checking for existing volume.")
		return fmt.Errorf("error occurred checking for existing volume")
	}
	if !volExists {
		return fmt.Errorf("volume %s does not exist", name)
	}

	volSize, err := d.API.VolumeSize(name)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"name":  name,
		}).Error("Error checking volume size.")
		return fmt.Errorf("error occurred when checking volume size")
	}

	sameSize, err := utils.VolumeSizeWithinTolerance(int64(sizeBytes), int64(volSize), tridentconfig.SANResizeDelta)
	if err != nil {
		return err
	}

	if sameSize {
		log.WithFields(log.Fields{
			"requestedSize":     sizeBytes,
			"currentVolumeSize": volSize,
			"name":              name,
			"delta":             tridentconfig.SANResizeDelta,
		}).Info("Requested size and current volume size are within the delta and therefore considered the same size for SAN resize operations.")
		volConfig.Size = strconv.FormatUint(uint64(volSize), 10)
		return nil
	}

	volSizeBytes := uint64(volSize)
	if sizeBytes < volSizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, volSizeBytes)
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(name, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Resize operations
	lunPath := fmt.Sprintf("/vol/%v/lun0", name)
	if !d.API.SupportsFeature(api.LunGeometrySkip) {
		// Check LUN geometry and verify LUN max size.
		lunGeometry, err := d.API.LunGetGeometry(lunPath)
		if err != nil {
			log.WithField("error", err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}

		lunMaxSize := lunGeometry.Result.MaxResizeSize()
		if lunMaxSize < int(sizeBytes) {
			log.WithFields(log.Fields{
				"error":      err,
				"sizeBytes":  sizeBytes,
				"lunMaxSize": lunMaxSize,
				"lunPath":    lunPath,
			}).Error("Requested size is larger than LUN's maximum capacity.")
			return fmt.Errorf("volume resize failed as requested size is larger than LUN's maximum capacity")
		}
	}

	// Resize FlexVol
	response, err := d.API.VolumeSetSize(name, strconv.FormatUint(sizeBytes, 10))
	if err = api.GetError(response.Result, err); err != nil {
		log.WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	// Resize LUN0
	returnSize, err := d.API.LunResize(lunPath, int(sizeBytes))
	if err != nil {
		log.WithField("error", err).Error("LUN resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	volConfig.Size = strconv.FormatUint(returnSize, 10)
	return nil
}

func (d *SANStorageDriver) ReconcileNodeAccess(nodes []*utils.Node, backendUUID string) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "SANStorageDriver",
			"Nodes":  nodeNames,
		}
		log.WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer log.WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	return nil
}
