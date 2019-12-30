// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

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

const (
	maxLunNameLength      = 254
	maxLunsPerFlexvol     = 100
	snapshotNameSeparator = "_snapshot_"
)

func GetLUNPathEconomy(bucketName string, volNameInternal string) string {
	return fmt.Sprintf("/vol/%s/%s", bucketName, volNameInternal)
}

type LUNHelper struct {
	Config         drivers.OntapStorageDriverConfig
	Context        tridentconfig.DriverContext
	SnapshotRegexp *regexp.Regexp
}

func NewLUNHelper(config drivers.OntapStorageDriverConfig, context tridentconfig.DriverContext) *LUNHelper {

	helper := LUNHelper{}
	helper.Config = config
	helper.Context = context
	regexString := fmt.Sprintf("(?m)/vol/(.+)/%v(.+?)($|_snapshot_(.+))", *helper.Config.StoragePrefix)
	helper.SnapshotRegexp = regexp.MustCompile(regexString)

	return &helper
}

// volName is expected not to have the storage prefix included
// parameters: volName=my-Lun snapName=my-Snapshot
// output: storagePrefix_my_Lun_snapshot_my_Snapshot
// parameters: volName=my-Lun snapName=snapshot-123
// output: storagePrefix_my_Lun_snapshot_snapshot_123
func (o *LUNHelper) GetSnapshotName(volName, snapName string) string {
	volName = strings.ReplaceAll(volName, "-", "_")
	snapName = o.getInternalSnapshotName(snapName)
	name := fmt.Sprintf("%v%v%v", *o.Config.StoragePrefix, volName, snapName)
	return name
}

// internalVolName is expected to have the storage prefix included
// parameters: internalVolName=storagePrefix_my-Lun snapName=my-Snapshot
// output: storagePrefix_my_Lun_snapshot_my_Snapshot
func (o *LUNHelper) GetInternalSnapshotName(internalVolName, snapName string) string {
	internalVolName = strings.ReplaceAll(internalVolName, "-", "_")
	snapName = o.getInternalSnapshotName(snapName)
	name := fmt.Sprintf("%v%v", internalVolName, snapName)
	return name
}

// parameters: bucketName=my-Bucket internalVolName=storagePrefix_my-Lun snapName=snap-1
// output: /vol/my_Bucket/storagePrefix_my_Lun_snapshot_snap_1
func (o *LUNHelper) GetSnapPath(bucketName, internalVolName, snapName string) string {
	bucketName = strings.ReplaceAll(bucketName, "-", "_")
	internalVolName = strings.ReplaceAll(internalVolName, "-", "_")
	snapName = o.getInternalSnapshotName(snapName)
	snapPath := fmt.Sprintf("/vol/%v/%v%v", bucketName, internalVolName, snapName)
	return snapPath
}

// parameter: bucketName=my-Bucket
// output: /vol/my_Bucket/storagePrefix_*_snapshot_*
func (o *LUNHelper) GetSnapPathPattern(bucketName string) string {
	bucketName = strings.ReplaceAll(bucketName, "-", "_")
	snapPattern := fmt.Sprintf("/vol/%v/%v*"+snapshotNameSeparator+"*", bucketName, *o.Config.StoragePrefix)
	return snapPattern
}

// parameter: volName=my-Vol
// output: /vol/*/storagePrefix_my_Vol_snapshot_*
func (o *LUNHelper) GetSnapPathPatternForVolume(externalVolumeName string) string {
	externalVolumeName = strings.ReplaceAll(externalVolumeName, "-", "_")
	snapPattern := fmt.Sprintf("/vol/*/%v%v"+snapshotNameSeparator+"*", *o.Config.StoragePrefix, externalVolumeName)
	return snapPattern
}

// parameter: volName=my-Lun
// output: storagePrefix_my_Lun
// parameter: volName=storagePrefix_my-Lun
// output: storagePrefix_my_Lun
func (o *LUNHelper) GetInternalVolumeName(volName string) string {
	volName = strings.ReplaceAll(volName, "-", "_")
	if !strings.HasPrefix(volName, *o.Config.StoragePrefix) {
		name := fmt.Sprintf("%v%v", *o.Config.StoragePrefix, volName)
		return name
	}
	return volName
}

// parameter: /vol/my_Bucket/storagePrefix_my-Lun
// output: storagePrefix_my-Lun
func (o *LUNHelper) GetInternalVolumeNameFromPath(path string) string {
	pathElements := strings.Split(path, "/")
	if len(pathElements) == 0 {
		return ""
	}
	return pathElements[len(pathElements)-1]
}

// parameter: snapName=snapshot-123
// output: _snapshot_snapshot_123
// parameter: snapName=snapshot
// output: _snapshot_snapshot
// parameter: snapName=_____snapshot
// output: _snapshot______snapshot
func (o *LUNHelper) getInternalSnapshotName(snapName string) string {
	snapName = strings.ReplaceAll(snapName, "-", "_")
	name := fmt.Sprintf("%v%v", snapshotNameSeparator, snapName)
	return name
}

// parameters: bucketName=my-Bucket volName=my-Lun
// output: /vol/my_Bucket/storagePrefix_my_Lun
// parameters: bucketName=my-Bucket volName=storagePrefix_my-Lun
// output: /vol/my_Bucket/storagePrefix_my_Lun
func (o *LUNHelper) GetLUNPath(bucketName, volName string) string {
	bucketName = strings.ReplaceAll(bucketName, "-", "_")
	volName = o.GetInternalVolumeName(volName)
	snapPath := fmt.Sprintf("/vol/%v/%v", bucketName, volName)
	return snapPath
}

// parameter: volName=my-Lun
// output: /vol/*/storagePrefix_my_Vol
func (o *LUNHelper) GetLUNPathPattern(volName string) string {
	volName = strings.ReplaceAll(volName, "-", "_")
	snapPattern := fmt.Sprintf("/vol/*/%v%v", *o.Config.StoragePrefix, volName)
	return snapPattern
}

// identifies if the given snapLunPath has a valid snapshot name
func (o *LUNHelper) IsValidSnapLUNPath(snapLunPath string) bool {
	snapLunPath = strings.ReplaceAll(snapLunPath, "-", "_")
	snapshotName := o.GetSnapshotNameFromSnapLUNPath(snapLunPath)
	if snapshotName == "" {
		return false
	}
	return true
}

func (o *LUNHelper) getLunPathComponents(snapLunPath string) []string {
	result := o.SnapshotRegexp.FindStringSubmatch(snapLunPath)
	// result [0] is the full string: /vol/myBucket/storagePrefix_myLun_snapshot_mySnap
	// result [1] is the bucket name: myBucket
	// result [2] is the volume name: myLun
	// result [3] is _snapshot_mySnap (unused)
	// result [4] is the snapshot name: mySnap
	return result
}

// parameter: snapLunPath=/vol/myBucket/storagePrefix_myLun_snapshot_mySnap
// result [4] is the snapshot name: mySnap
func (o *LUNHelper) GetSnapshotNameFromSnapLUNPath(snapLunPath string) string {
	result := o.getLunPathComponents(snapLunPath)
	if len(result) > 4 {
		return result[4]
	}
	return ""
}

// parameter: snapLunPath=/vol/myBucket/storagePrefix_myLun_snapshot_mySnap
// result [2] is the volume name: myLun
func (o *LUNHelper) GetExternalVolumeNameFromPath(lunPath string) string {
	result := o.getLunPathComponents(lunPath)
	if len(result) > 2 {
		return result[2]
	}
	return ""
}

// parameter: snapLunPath=/vol/myBucket/storagePrefix_myLun_snapshot_mySnap
// result [1] is the bucket name: myBucket
func (o *LUNHelper) GetBucketName(lunPath string) string {
	result := o.getLunPathComponents(lunPath)
	if len(result) > 1 {
		return result[1]
	}
	return ""
}

// SANEconomyStorageDriver is for iSCSI storage provisioning of LUNs
type SANEconomyStorageDriver struct {
	initialized       bool
	Config            drivers.OntapStorageDriverConfig
	ips               []string
	API               *api.Client
	Telemetry         *Telemetry
	flexvolNamePrefix string
	helper            *LUNHelper

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func (d *SANEconomyStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *SANEconomyStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *SANEconomyStorageDriver) GetTelemetry() *Telemetry {
	d.Telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *SANEconomyStorageDriver) Name() string {
	return drivers.OntapSANEconomyStorageDriverName
}

// backendName returns the name of the backend managed by this driver instance
func (d *SANEconomyStorageDriver) backendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return "ontapsaneco_" + d.ips[0]
	} else {
		return d.Config.BackendName
	}
}

func (d *SANEconomyStorageDriver) FlexvolNamePrefix() string {
	return d.flexvolNamePrefix
}

// Initialize from the provided config
func (d *SANEconomyStorageDriver) Initialize(
	context tridentconfig.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "SANEconomyStorageDriver"}
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
	d.helper = NewLUNHelper(d.Config, context)

	d.ips, err = d.API.NetInterfaceGetDataLIFs("iscsi")
	if err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no iSCSI data LIFs found on SVM %s", config.SVM)
	} else {
		log.WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
	}

	// Remap context for artifact naming so the names remain stable over time
	var artifactPrefix string
	switch context {
	case tridentconfig.ContextDocker:
		artifactPrefix = artifactPrefixDocker
	case tridentconfig.ContextKubernetes, tridentconfig.ContextCSI:
		artifactPrefix = artifactPrefixKubernetes
	default:
		return fmt.Errorf("unknown driver context: %s", context)
	}

	// Set up internal driver state
	d.flexvolNamePrefix = fmt.Sprintf("%s_lun_pool_%s_", artifactPrefix, *d.Config.StoragePrefix)
	d.flexvolNamePrefix = strings.Replace(d.flexvolNamePrefix, "__", "_", -1)

	log.WithFields(log.Fields{
		"FlexvolNamePrefix": d.flexvolNamePrefix,
	}).Debugf("SAN Economy driver settings.")

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(d, d.getStoragePoolAttributes(),
		d.backendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	if err = InitializeSANDriver(context, d.API, &d.Config, d.validate); err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.Telemetry = NewOntapTelemetry(d)
	d.Telemetry.Start()

	d.initialized = true
	return nil
}

func (d *SANEconomyStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANEconomyStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "SANEconomyStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}

	if d.Telemetry != nil {
		d.Telemetry.Stop()
	}

	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *SANEconomyStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "SANEconomyStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	if err := ValidateSANDriver(d.API, &d.Config, d.ips); err != nil {
		return fmt.Errorf("error driver validation failed: %v", err)
	}

	if err := ValidateStoragePools(d.physicalPools, d.virtualPools, d.Name()); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume+LUN with the specified options
func (d *SANEconomyStorageDriver) Create(
	volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "SANEconomyStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
	}

	// Generic user-facing message
	createError := errors.New("error volume creation failed")

	// Determine a way to see if the volume already exists
	exists, existsInFlexvol, err := d.LUNExists(name, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing volume: %v", err)
		return createError
	}
	if exists {
		log.WithFields(log.Fields{"LUN": name, "bucketVol": existsInFlexvol}).Debug("LUN already exists.")
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
		return fmt.Errorf("error could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("error %v is an invalid volume size: %v", volConfig.Size, err)
	}
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes[Size])
	if err != nil {
		return err
	}

	// Ensure LUN name isn't too long
	if len(name) > maxLunNameLength {
		return fmt.Errorf("volume %s name exceeds the limit of %d characters", name, maxLunNameLength)
	}

	// Get options
	opts, err := d.GetVolumeOpts(volConfig, volAttributes)
	if err != nil {
		return err
	}

	// Get Flexvol options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	spaceAllocation, _ := strconv.ParseBool(utils.GetV(opts, "spaceAllocation", storagePool.InternalAttributes[SpaceAllocation]))
	spaceReserve := utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes[SpaceReserve])
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes[SnapshotPolicy])
	encryption := utils.GetV(opts, "encryption", storagePool.InternalAttributes[Encryption])

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	// Check for a supported file system type
	fstype, err := drivers.CheckSupportedFilesystem(utils.GetV(opts, "fstype|fileSystemType",
		storagePool.InternalAttributes[FileSystemType]), name)
	if err != nil {
		return err
	}

	createErrors := make([]error, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name

		if aggrLimitsErr := checkAggregateLimits(aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN-ECONOMY pool %s/%s; error: %v", storagePool.Name, aggregate,
				aggrLimitsErr)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Make sure we have a Flexvol for the new LUN
		bucketVol, err := d.ensureFlexvolForLUN(aggregate, spaceReserve, snapshotPolicy, false,
			enableEncryption, sizeBytes, opts, d.Config, storagePool)
		if err != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN-ECONOMY pool %s/%s; BucketVol location/creation failed %s: %v",
				storagePool.Name,
				aggregate, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Grow or shrink the Flexvol as needed
		err = d.resizeFlexvol(bucketVol, sizeBytes)
		if err != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN-ECONOMY pool %s/%s; Flexvol resize failed %s/%s: %v",
				storagePool.Name, aggregate, bucketVol, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		lunPath := GetLUNPathEconomy(bucketVol, name)
		osType := "linux"

		// Create the LUN
		lunCreateResponse, err := d.API.LunCreate(lunPath, int(sizeBytes), osType, false, spaceAllocation)
		if err = api.GetError(lunCreateResponse, err); err != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN-ECONOMY pool %s/%s; error creating LUN %s/%s: %v", storagePool.Name,
				aggregate, bucketVol, name, err)
			log.Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Save the fstype in a LUN attribute so we know what to do in Attach
		attrResponse, err := d.API.LunSetAttribute(lunPath, LUNAttributeFSType, fstype)
		if err = api.GetError(attrResponse, err); err != nil {
			d.API.LunDestroy(lunPath)
			return fmt.Errorf("ONTAP-SAN-ECONOMY pool %s/%s; error saving file system type for LUN %s/%s: %v",
				storagePool.Name, aggregate, bucketVol, name, err)
		}
		// Save the context
		attrResponse, err = d.API.LunSetAttribute(lunPath, "context", string(d.Config.DriverContext))
		if err = api.GetError(attrResponse, err); err != nil {
			log.WithField("name", name).Warning("Failed to save the driver context attribute for new volume.")
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors)
}

// Create a volume clone
func (d *SANEconomyStorageDriver) CreateClone(volConfig *storage.VolumeConfig, _ *storage.Pool) error {

	source := volConfig.CloneSourceVolumeInternal
	name := volConfig.InternalName
	snapshot := volConfig.CloneSourceSnapshot
	isFromSnapshot := snapshot != ""

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "SANEconomyStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
		}
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
	}

	return d.createLUNClone(name, source, snapshot, &d.Config, d.API, d.FlexvolNamePrefix(), isFromSnapshot)
}

// Create a volume clone
func (d *SANEconomyStorageDriver) createLUNClone(
	lunName, source, snapshot string, config *drivers.OntapStorageDriverConfig, client *api.Client, prefix string, isLunCreateFromSnapshot bool,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "createLUNClone",
			"Type":     "ontap_san_economy",
			"lunName":  lunName,
			"source":   source,
			"snapshot": snapshot,
			"prefix":   prefix,
		}
		log.WithFields(fields).Debug(">>>> createLUNClone")
		defer log.WithFields(fields).Debug("<<<< createLUNClone")
	}

	// If the specified LUN copy already exists, return an error
	destinationLunExists, _, err := d.LUNExists(lunName, prefix)
	if err != nil {
		return fmt.Errorf("error checking for existing LUN: %v", err)
	}
	if destinationLunExists {
		return fmt.Errorf("error LUN %s already exists", lunName)
	}

	// Check if called from CreateClone and is from a snapshot
	if isLunCreateFromSnapshot {
		source = d.helper.GetInternalSnapshotName(source, snapshot)
	}

	// If the source doesn't exist, return an error
	sourceLunExists, flexvol, err := d.LUNExists(source, prefix)
	if err != nil {
		return fmt.Errorf("error checking for existing LUN: %v", err)
	}
	if !sourceLunExists {
		return fmt.Errorf("error LUN %s does not exist", source)
	}

	// Create the clone based on given LUN
	cloneResponse, err := client.LunCloneCreate(flexvol, source, lunName)
	if err != nil {
		return fmt.Errorf("error creating clone: %v", err)
	}
	if zerr := api.NewZapiError(cloneResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			return fmt.Errorf("snapshot %s does not exist in volume %s", snapshot, source)
		} else if zerr.IsFailedToLoadJobError() {
			fields := log.Fields{
				"zerr": zerr,
			}
			log.WithFields(fields).Warn("Problem encountered during the clone create operation," +
				" attempting to verify the clone was actually created")
			if volumeLookupError := probeForVolume(lunName, client); volumeLookupError != nil {
				return volumeLookupError
			}
		} else {
			return fmt.Errorf("error creating clone: %v", zerr)
		}
	}

	// Grow or shrink the Flexvol as needed
	return d.resizeFlexvol(flexvol, 0)
}

func (d *SANEconomyStorageDriver) Import(volConfig *storage.VolumeConfig, originalName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "SANEconomyStorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
			"notManaged":   volConfig.ImportNotManaged,
		}
		log.WithFields(fields).Debug(">>>> Import")
		defer log.WithFields(fields).Debug("<<<< Import")
	}

	return errors.New("import is not implemented")
}

func (d *SANEconomyStorageDriver) Rename(name string, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "SANEconomyStorageDriver",
			"name":    name,
			"newName": newName,
		}
		log.WithFields(fields).Debug(">>>> Rename")
		defer log.WithFields(fields).Debug("<<<< Rename")
	}

	return errors.New("rename is not implemented")
}

// Destroy the LUN
func (d *SANEconomyStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "SANEconomyStorageDriver",
			"Name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	// Generic user-facing message
	deleteError := errors.New("volume deletion failed")

	exists, bucketVol, err := d.LUNExists(name, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		log.Warnf("LUN %v does not exist", name)
		return nil
	}

	lunPath := GetLUNPathEconomy(bucketVol, name)
	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfo(d.API, &d.Config)
		if err != nil {
			log.WithField("error", err).Error("Could not get target info")
			return err
		}

		// Get the LUN ID
		lunMapResponse, err := d.API.LunMapListInfo(lunPath)
		if err != nil {
			return fmt.Errorf("error reading LUN maps for volume %s, path %s: %v", name, lunPath, err)
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

	// Before deleting the LUN, check if a LUN has associated snapshots. If so, delete all associated snapshots
	externalVolumeName := d.helper.GetExternalVolumeNameFromPath(lunPath)
	snapList, err := d.getSnapshotsEconomy(name, externalVolumeName)
	if err != nil {
		log.Errorf("Error enumerating snapshots: %v", err)
		return deleteError
	}
	for _, snap := range snapList {
		err = d.DeleteSnapshot(snap.Config)
		if err != nil {
			log.Errorf("Error snap-LUN delete failed: %v", err)
			return err
		}
	}

	offlineResponse, err := d.API.LunOffline(lunPath)
	if err != nil {
		fields := log.Fields{
			"Method":   "Destroy",
			"Type":     "SANEconomyStorageDriver",
			"LUN":      lunPath,
			"Response": offlineResponse,
			"Error":    err,
		}
		log.WithFields(fields)
	}

	destroyResponse, err := d.API.LunDestroy(lunPath)
	if err = api.GetError(destroyResponse, err); err != nil {
		log.Errorf("Error LUN delete failed: %v", err)
		return deleteError
	}
	// Check if a bucket volume has no more LUNs. If none left, delete the bucketVol. Else, call for resize
	return d.DeleteBucketIfEmpty(bucketVol)
}

// DeleteBucketIfEmpty will check if the given bucket volume is empty, if the bucket is empty it will be deleted.
// Otherwise, it will be resized.
func (d *SANEconomyStorageDriver) DeleteBucketIfEmpty(bucketVol string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Destroy",
			"Type":      "SANEconomyStorageDriver",
			"bucketVol": bucketVol,
		}
		log.WithFields(fields).Debug(">>>> DeleteBucketIfEmpty")
		defer log.WithFields(fields).Debug("<<<< DeleteBucketIfEmpty")
	}

	count, err := d.API.LunCount(bucketVol)
	if err != nil {
		return fmt.Errorf("error enumerating LUNs for volume %s: %v", bucketVol, err)
	}
	if count == 0 {
		// Delete the bucketVol
		volDestroyResponse, err := d.API.VolumeDestroy(bucketVol, true)
		if err != nil {
			return fmt.Errorf("error destroying volume %v: %v", bucketVol, err)
		}
		if zerr := api.NewZapiError(volDestroyResponse); !zerr.IsPassed() {
			if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
				log.WithField("volume", bucketVol).Warn("Volume already deleted.")
			} else {
				return fmt.Errorf("error destroying volume %v: %v", bucketVol, zerr)
			}
		}
	} else {
		// Grow or shrink the Flexvol as needed
		err = d.resizeFlexvol(bucketVol, 0)
		if err != nil {
			return err
		}
	}
	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANEconomyStorageDriver) Publish(name string, publishInfo *utils.VolumePublishInfo) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":      "Publish",
			"Type":        "SANEconomyStorageDriver",
			"name":        name,
			"publishInfo": publishInfo,
		}
		log.WithFields(fields).Debug(">>>> Publish")
		defer log.WithFields(fields).Debug("<<<< Publish")
	}

	exists, bucketVol, err := d.LUNExists(name, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		return fmt.Errorf("error LUN %v does not exist", name)
	}

	lunPath := d.helper.GetLUNPath(bucketVol, name)
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
func (d *SANEconomyStorageDriver) GetSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "SANEconomyStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshot")
		defer log.WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return d.getSnapshotEconomy(snapConfig, &d.Config)
}

func (d *SANEconomyStorageDriver) getSnapshotEconomy(
	snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolumeName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "getSnapshotEconomy",
			"Type":         "SANEconomyStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolumeName,
		}
		log.WithFields(fields).Debug(">>>> getSnapshotEconomy")
		defer log.WithFields(fields).Debug("<<<< getSnapshotEconomy")
	}

	fullSnapshotName := d.helper.GetInternalSnapshotName(internalVolumeName, internalSnapName)
	exists, bucketVol, err := d.LUNExists(fullSnapshotName, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	snapPath := d.helper.GetSnapPath(bucketVol, internalVolumeName, internalSnapName)
	lunInfo, err := d.API.LunGet(snapPath)
	if err != nil {
		return nil, fmt.Errorf("error reading volume: %v", err)
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   time.Unix(int64(lunInfo.CreationTimestamp()), 0).UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: int64(lunInfo.Size()),
	}, nil

}

func (d *SANEconomyStorageDriver) GetSnapshots(volConfig *storage.VolumeConfig) ([]*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "GetSnapshots",
			"Type":               "SANEconomyStorageDriver",
			"internalVolumeName": volConfig.InternalName,
			"volumeName":         volConfig.Name,
		}
		log.WithFields(fields).Debug(">>>> GetSnapshots")
		defer log.WithFields(fields).Debug("<<<< GetSnapshots")
	}

	exists, _, err := d.LUNExists(volConfig.InternalName, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("error LUN %v does not exist", volConfig.Name)
	}

	return d.getSnapshotsEconomy(volConfig.InternalName, volConfig.Name)
}

func (d *SANEconomyStorageDriver) getSnapshotsEconomy(
	internalVolumeName string, externalVolumeName string,
) ([]*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":          "getSnapshotsEconomy",
			"Type":            "SANEconomyStorageDriver",
			"internalVolName": internalVolumeName,
			"externalVolName": externalVolumeName,
		}
		log.WithFields(fields).Debug(">>>> getSnapshotsEconomy")
		defer log.WithFields(fields).Debug("<<<< getSnapshotsEconomy")
	}

	snapPathPattern := d.helper.GetSnapPathPatternForVolume(externalVolumeName)

	snapListResponse, err := d.API.LunGetAll(snapPathPattern)
	if err = api.GetError(snapListResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	log.Debugf("Found %v snapshots.", snapListResponse.Result.NumRecords())
	snapshots := make([]*storage.Snapshot, 0)

	if snapListResponse.Result.AttributesListPtr == nil {
		return nil, fmt.Errorf("error snapshot attribute pointer nil")
	}

	for _, snapLunInfo := range snapListResponse.Result.AttributesListPtr.LunInfoPtr {
		snapLunPath := snapLunInfo.Path()
		// Check to see if it has the following string pattern. If so, add to snapshot List. Else, skip.
		if d.helper.IsValidSnapLUNPath(snapLunPath) {
			snapLunName := d.helper.GetSnapshotNameFromSnapLUNPath(snapLunPath)
			snapshot := &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Version:            tridentconfig.OrchestratorAPIVersion,
					Name:               snapLunName,
					InternalName:       snapLunName,
					VolumeName:         externalVolumeName,
					VolumeInternalName: internalVolumeName,
				},
				Created: time.Unix(int64(snapLunInfo.CreationTimestamp()), 0).
					UTC().Format(storage.SnapshotTimestampFormat),
				SizeBytes: int64(snapLunInfo.Size()),
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *SANEconomyStorageDriver) CreateSnapshot(snapConfig *storage.SnapshotConfig) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "SANEconomyStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
			"snapConfig":   snapConfig,
		}
		log.WithFields(fields).Info(">>>> CreateSnapshot")
		defer log.WithFields(fields).Info("<<<< CreateSnapshot")
	}

	internalSnapName := snapConfig.InternalName
	internalVolumeName := snapConfig.VolumeInternalName

	// Check to see if source LUN exists
	exists, bucketVol, err := d.LUNExists(internalVolumeName, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("LUN %v does not exist", internalVolumeName)
	}

	lunPath := GetLUNPathEconomy(bucketVol, internalVolumeName)

	// If the specified volume doesn't exist, return error
	lunInfo, err := d.API.LunGet(lunPath)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}

	if lunInfo.SizePtr == nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}
	size := lunInfo.Size()

	// Create the snapshot name/string
	lunName := d.helper.GetSnapshotName(snapConfig.VolumeName, internalSnapName)

	// Create the "snap-LUN" where the snapshot is a LUN clone of the source LUN
	err = d.createLUNClone(lunName, snapConfig.VolumeInternalName, snapConfig.Name,
		&d.Config, d.API, d.FlexvolNamePrefix(), false)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// Fetching list of snapshots to get snapshot creation time
	snapListResponse, err := d.getSnapshotsEconomy(internalVolumeName, snapConfig.VolumeName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	if snapListResponse != nil {
		for _, snap := range snapListResponse {
			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.Created,
				SizeBytes: int64(size),
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find snapshot %s for souce volume %s", internalSnapName, internalVolumeName)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANEconomyStorageDriver) RestoreSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "SANEconomyStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		log.WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer log.WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return drivers.NewSnapshotsNotSupportedError(d.Name())
}

// DeleteSnapshot deletes a LUN snapshot.
func (d *SANEconomyStorageDriver) DeleteSnapshot(snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":                "DeleteSnapshot",
			"Type":                  "SANEconomyStorageDriver",
			"snapshotName":          snapConfig.InternalName,
			"volumeName":            snapConfig.VolumeInternalName,
			"snapConfig.VolumeName": snapConfig.VolumeName,
		}
		log.WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer log.WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	internalSnapName := snapConfig.InternalName
	// Creating the path string pattern
	snapLunName := d.helper.GetSnapshotName(snapConfig.VolumeName, internalSnapName)

	// Check to see if the source LUN exists
	exists, bucketVol, err := d.LUNExists(snapLunName, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		return fmt.Errorf("LUN %v does not exist", snapLunName)
	}

	snapPath := GetLUNPathEconomy(bucketVol, snapLunName)

	offlineResponse, err := d.API.LunOffline(snapPath)
	if err != nil {
		log.WithFields(log.Fields{
			"Method":   "DeleteSnapshot",
			"Type":     "SANEconomyStorageDriver",
			"snap-LUN": snapPath,
			"Response": offlineResponse,
			"Error":    err,
		}).Warn("Error attempting to offline snap-LUN.")
	}

	destroyResponse, err := d.API.LunDestroy(snapPath)
	if err = api.GetError(destroyResponse, err); err != nil {
		log.Errorf("Snap-LUN delete failed: %v", err)
		return fmt.Errorf("error deleting snapshot: %v", err)
	}

	// Check if a bucket volume has no more LUNs. If none left, delete the bucketVol. Else, call for resize
	return d.DeleteBucketIfEmpty(bucketVol)
}

// Test for the existence of a volume
func (d *SANEconomyStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "SANEconomyStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	// Generic user-facing message
	getError := fmt.Errorf("volume %s not found", name)

	exists, bucketVol, err := d.LUNExists(name, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing LUN: %v", err)
		return getError
	}
	if !exists {
		log.WithField("LUN", name).Debug("LUN not found.")
		return getError
	}

	log.WithFields(log.Fields{"LUN": name, "bucketVol": bucketVol}).Debug("Volume found.")

	return nil
}

// ensureFlexvolForLUN accepts a set of Flexvol characteristics and either finds one to contain a new
// LUN or it creates a new Flexvol with the needed attributes.
func (d *SANEconomyStorageDriver) ensureFlexvolForLUN(
	aggregate, spaceReserve, snapshotPolicy string, enableSnapshotDir bool, encrypt bool,
	sizeBytes uint64, opts map[string]string, config drivers.OntapStorageDriverConfig, storagePool *storage.Pool,
) (string, error) {

	shouldLimitVolumeSize, flexvolSizeLimit, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes,
		config.CommonStorageDriverConfig)
	if checkVolumeSizeLimitsError != nil {
		return "", checkVolumeSizeLimitsError
	}

	// Check if a suitable Flexvol already exists
	flexvol, err := d.getFlexvolForLUN(aggregate, spaceReserve, snapshotPolicy, enableSnapshotDir, encrypt, sizeBytes,
		shouldLimitVolumeSize, flexvolSizeLimit)

	if err != nil {
		return "", fmt.Errorf("error finding Flexvol for LUN: %v", err)
	}

	// Found one
	if flexvol != "" {
		return flexvol, nil
	}

	// Nothing found, so create a suitable Flexvol
	flexvol, err = d.createFlexvolForLUN(aggregate, spaceReserve, snapshotPolicy, enableSnapshotDir, encrypt, opts,
		storagePool)
	if err != nil {
		return "", fmt.Errorf("error creating Flexvol for LUN: %v", err)
	}

	return flexvol, nil
}

// createFlexvolForLUN creates a new Flexvol matching the specified attributes for
// the purpose of containing LUN supplied as container volumes by this driver.
// Once this method returns, the Flexvol exists, is mounted
func (d *SANEconomyStorageDriver) createFlexvolForLUN(
	aggregate, spaceReserve, snapshotPolicy string, enableSnapshotDir bool, encrypt bool, opts map[string]string,
	storagePool *storage.Pool) (string, error) {

	flexvol := d.FlexvolNamePrefix() + utils.RandomString(10)
	size := "1g"
	unixPermissions := utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes[UnixPermissions])
	exportPolicy := utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes[ExportPolicy])
	securityStyle := utils.GetV(opts, "securityStyle", storagePool.InternalAttributes[SecurityStyle])

	encryption := encrypt

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, storagePool.InternalAttributes[SnapshotReserve])
	if err != nil {
		return "", fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	log.WithFields(log.Fields{
		"name":            flexvol,
		"aggregate":       aggregate,
		"size":            size,
		"spaceReserve":    spaceReserve,
		"snapshotPolicy":  snapshotPolicy,
		"snapshotReserve": snapshotReserveInt,
		"unixPermissions": unixPermissions,
		"snapshotDir":     enableSnapshotDir,
		"exportPolicy":    exportPolicy,
		"securityStyle":   securityStyle,
		"encryption":      encryption,
	}).Debug("Creating Flexvol for LUNs.")

	// Create the flexvol
	volCreateResponse, err := d.API.VolumeCreate(
		flexvol, aggregate, size, spaceReserve, snapshotPolicy,
		unixPermissions, exportPolicy, securityStyle, encrypt, snapshotReserveInt)

	if err = api.GetError(volCreateResponse, err); err != nil {
		return "", fmt.Errorf("error creating volume: %v", err)
	}

	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	if !enableSnapshotDir {
		snapDirResponse, err := d.API.VolumeDisableSnapshotDirectoryAccess(flexvol)
		if err = api.GetError(snapDirResponse, err); err != nil {
			return "", fmt.Errorf("error disabling snapshot directory access: %v", err)
		}
	}

	return flexvol, nil
}

// getFlexvolForLUN returns a Flexvol (from the set of existing Flexvols) that
// matches the specified Flexvol attributes and does not already contain more
// than the maximum configured number of LUNs.  No matching Flexvols is not
// considered an error.  If more than one matching Flexvol is found, one of those
// is returned at random.
func (d *SANEconomyStorageDriver) getFlexvolForLUN(
	aggregate, spaceReserve, snapshotPolicy string, enableSnapshotDir bool, encrypt bool,
	sizeBytes uint64, shouldLimitFlexvolSize bool, flexvolSizeLimit uint64,
) (string, error) {

	// Get all volumes matching the specified attributes
	volListResponse, err := d.API.VolumeListByAttrs(
		d.FlexvolNamePrefix(), aggregate, spaceReserve, snapshotPolicy, enableSnapshotDir, encrypt)

	if err = api.GetError(volListResponse, err); err != nil {
		return "", fmt.Errorf("error enumerating Flexvols: %v", err)
	}

	// Weed out the Flexvols:
	// 1) already having too many LUNs
	// 2) exceeding size limits
	var volumes []string
	if volListResponse.Result.AttributesListPtr != nil {
		for _, volAttrs := range volListResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			volIDAttrs := volAttrs.VolumeIdAttributes()
			volName := volIDAttrs.Name()
			// skip flexvols over the size limit
			if shouldLimitFlexvolSize {
				sizeWithRequest, err := d.getOptimalSizeForFlexvol(volName, sizeBytes)
				if err != nil {
					log.Errorf("Error checking size for existing LUN %v: %v", volName, err)
					continue
				}
				if sizeWithRequest > flexvolSizeLimit {
					log.Debugf("Flexvol size for %v is over the limit of %v", volName, flexvolSizeLimit)
					continue
				}
			}

			count := 0
			listResponse, err := d.API.LunGetAllForVolume(volName)
			if err != nil {
				return "", fmt.Errorf("error enumerating LUNs: %v", err)
			}
			if listResponse.Result.AttributesListPtr != nil {
				for _, lunInfo := range listResponse.Result.AttributesListPtr.LunInfoPtr {
					lunPath := lunInfo.Path()
					if !d.helper.IsValidSnapLUNPath(lunPath) {
						count++
					}
				}
			}

			if count < maxLunsPerFlexvol {
				volumes = append(volumes, volName)
			}
		}
	}

	// Pick a Flexvol.  If there are multiple matches, pick one at random.
	switch len(volumes) {
	case 0:
		return "", nil
	case 1:
		return volumes[0], nil
	default:
		return volumes[rand.Intn(len(volumes))], nil
	}
}

// getOptimalSizeForFlexvol sums up all the LUN sizes on a Flexvol and adds the size of
// the new LUN being added as well as the current Flexvol snapshot reserve.  This value may be used
// to grow the Flexvol as new LUNs are being added.
func (d *SANEconomyStorageDriver) getOptimalSizeForFlexvol(flexvol string, newLunSizeBytes uint64) (uint64, error) {

	// Get more info about the Flexvol
	volAttrs, err := d.API.VolumeGet(flexvol)
	if err != nil {
		return 0, err
	}
	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()
	snapReserveDivisor := 1.0 - (float64(volSpaceAttrs.PercentageSnapshotReserve()) / 100.0)

	totalDiskLimitBytes, err := d.getTotalLUNSize(flexvol)
	if err != nil {
		return 0, err
	}

	usableSpaceBytes := float64(newLunSizeBytes + totalDiskLimitBytes)
	flexvolSizeBytes := uint64(usableSpaceBytes / snapReserveDivisor)

	log.WithFields(log.Fields{
		"flexvol":             flexvol,
		"snapReserveDivisor":  snapReserveDivisor,
		"totalDiskLimitBytes": totalDiskLimitBytes,
		"newLUNSizeBytes":     newLunSizeBytes,
		"flexvolSizeBytes":    flexvolSizeBytes,
	}).Debug("Calculated optimal size for Flexvol with new LUN.")

	return flexvolSizeBytes, nil
}

// getLUNSize returns the size of the LUN
func (d *SANEconomyStorageDriver) getLUNSize(name string, flexvol string) (uint64, error) {

	lunTarget := GetLUNPathEconomy(flexvol, name)
	lun, err := d.API.LunGet(lunTarget)
	if err != nil {
		return 0, err
	}

	lunSize := uint64(lun.Size())
	if lunSize == 0 {
		return 0, fmt.Errorf("unable to determine LUN size")
	}
	return lunSize, nil
}

// getTotalLUNSize returns the sum of all LUN sizes on a Flexvol
func (d *SANEconomyStorageDriver) getTotalLUNSize(flexvol string) (uint64, error) {

	listResponse, err := d.API.LunGetAllForVolume(flexvol)
	if err != nil {
		return 0, err
	}

	var totalDiskLimit uint64

	if listResponse.Result.AttributesListPtr != nil {
		for _, lunInfo := range listResponse.Result.AttributesListPtr.LunInfoPtr {
			diskLimitSize := lunInfo.Size()
			totalDiskLimit += uint64(diskLimitSize)
		}
	}
	return totalDiskLimit, nil
}

// Retrieve storage backend capabilities
func (d *SANEconomyStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.backendName())
}

func (d *SANEconomyStorageDriver) getStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *SANEconomyStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, requests), nil
}

func (d *SANEconomyStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *SANEconomyStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) {
	createPrepareCommon(d, volConfig)
}

func (d *SANEconomyStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateFollowup",
			"Type":         "SANEconomyStorageDriver",
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

	return d.mapOntapSANLUN(volConfig)
}

func (d *SANEconomyStorageDriver) mapOntapSANLUN(volConfig *storage.VolumeConfig) error {

	// Determine which flexvol contains the LUN
	exists, flexvol, err := d.LUNExists(volConfig.InternalName, d.FlexvolNamePrefix())
	if err != nil {
		return fmt.Errorf("could not determine if LUN %s exists: %v", volConfig.InternalName, err)
	}
	if !exists {
		return fmt.Errorf("could not find LUN %s", volConfig.InternalName)
	}
	// Map LUN
	lunPath := GetLUNPathEconomy(flexvol, volConfig.InternalName)
	lunID, err := d.API.LunMapIfNotMapped(d.Config.IgroupName, lunPath)
	if err != nil {
		return err
	}

	err = PopulateOntapLunMapping(d.API, &d.Config, d.ips, volConfig, lunID)
	if err != nil {
		return fmt.Errorf("error mapping LUN for %s driver: %v", d.Name(), err)
	}

	return nil
}

func (d *SANEconomyStorageDriver) GetProtocol() tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANEconomyStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *SANEconomyStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SANEconomyStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	_, flexvol, err := d.LUNExists(name, d.FlexvolNamePrefix())
	if err != nil {
		return nil, err
	}

	volumeAttrs, err := d.API.VolumeGet(flexvol)
	if err != nil {
		return nil, err
	}

	lunPath := GetLUNPathEconomy(flexvol, name)
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
func (d *SANEconomyStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(d.FlexvolNamePrefix())
	if err = api.GetError(volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Bail out early if there aren't any Flexvols
	if volumesResponse.Result.AttributesListPtr == nil {
		return
	}
	if len(volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr) == 0 {
		return
	}

	// Get all LUNs in volumes matching the storage prefix
	lunPathPattern := fmt.Sprintf("/vol/%v/*", d.flexvolNamePrefix+"*")
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

			// Skip over LUNs which are snapshots of other LUNs
			if d.helper.GetSnapshotNameFromSnapLUNPath(lun.Path()) != "" {
				log.WithField("path", lun.Path()).Debug("Skipping LUN snapshot.")
				continue
			}

			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&lun, &volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SANEconomyStorageDriver) getVolumeExternal(
	lunAttrs *azgo.LunInfoType, volumeAttrs *azgo.VolumeAttributesType,
) *storage.VolumeExternal {

	volumeIDAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr
	externalVolumeName := d.helper.GetExternalVolumeNameFromPath(lunAttrs.Path())
	internalVolumeName := d.helper.GetInternalVolumeNameFromPath(lunAttrs.Path())

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            externalVolumeName,
		InternalName:    internalVolumeName,
		Size:            strconv.FormatInt(int64(lunAttrs.Size()), 10),
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    "",
		SnapshotDir:     "",
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
func (d *SANEconomyStorageDriver) GetUpdateType(driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANEconomyStorageDriver)
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

// LUNExists returns true if the named LUN exists across all buckets.  This should be called with the
// actual LUN name, i.e. the internal volume name or snap-LUN name.
func (d *SANEconomyStorageDriver) LUNExists(name, bucketPrefix string) (bool, string, error) {

	log.WithFields(log.Fields{
		"name":         name,
		"bucketPrefix": bucketPrefix,
	}).Debug("LUNExists")

	listResponse, err := d.API.LunGetAll(fmt.Sprintf("/vol/%s*/%s", bucketPrefix, name))
	if err != nil {
		return false, "", err
	}
	if listResponse.Result.AttributesListPtr != nil {
		for _, lunInfo := range listResponse.Result.AttributesListPtr.LunInfoPtr {
			log.WithFields(log.Fields{
				"lunInfo":        lunInfo,
				"lunInfo.Path()": lunInfo.Path(),
			}).Debug("LUNExists")
			if strings.HasSuffix(lunInfo.Path(), name) {
				flexvol := listResponse.Result.AttributesListPtr.LunInfoPtr[0].Volume()
				log.WithField("flexvol", flexvol).Debug("LUNExists")
				return true, flexvol, nil
			}
		}
	}
	return false, "", nil
}

// Resize expands the Flexvol containing the LUN and updates the LUN size
func (d *SANEconomyStorageDriver) Resize(volConfig *storage.VolumeConfig, sizeBytes uint64) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "SANEconomyStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		log.WithFields(fields).Debug(">>>> Resize")
		defer log.WithFields(fields).Debug("<<<< Resize")
	}

	// Generic user-facing message
	resizeError := errors.New("storage driver failed to resize the volume")

	// Validation checks
	// get the volume where the lun exists
	exists, bucketVol, err := d.LUNExists(name, d.FlexvolNamePrefix())
	if err != nil {
		log.Errorf("Error checking for existing volume: %v", err)
		return resizeError
	}
	if !exists {
		return fmt.Errorf("error LUN %s does not exist", name)
	}

	// Calculate the delta size needed to resize the bucketVol
	totalLunSize, err := d.getTotalLUNSize(bucketVol)
	if err != nil {
		log.WithField("error", err).Error("Failed to determine total LUN size")
		return resizeError
	}

	sameSize, err := utils.VolumeSizeWithinTolerance(int64(sizeBytes), int64(totalLunSize), tridentconfig.SANResizeDelta)
	if err != nil {
		return err
	}

	if sameSize {
		log.WithFields(log.Fields{
			"requestedSize":     sizeBytes,
			"currentVolumeSize": totalLunSize,
			"name":              name,
			"delta":             tridentconfig.SANResizeDelta,
		}).Info("Requested size and current volume size are within the delta and therefore considered the same size for SAN resize operations.")
		return nil
	}

	totalLunSizeBytes := totalLunSize
	if sizeBytes < totalLunSizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, totalLunSizeBytes)
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(bucketVol, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Resize operations
	lunPath := d.helper.GetLUNPath(bucketVol, name)
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
	response, err := d.API.VolumeSetSize(bucketVol, strconv.FormatUint(sizeBytes, 10))
	if err = api.GetError(response, err); err != nil {
		log.WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	// Resize LUN
	returnSize, err := d.API.LunResize(lunPath, int(sizeBytes))
	if err != nil {
		log.WithField("error", err).Error("LUN resize failed.")
		return fmt.Errorf("volume resize failed")
	}
	log.WithField("size", returnSize).Debug("Returning.")
	volConfig.Size = strconv.FormatUint(returnSize, 10)

	return nil
}

// resizeFlexvol grows or shrinks the Flexvol to an optimal size if possible. Otherwise
// the Flexvol is expanded by the value of sizeBytes
func (d *SANEconomyStorageDriver) resizeFlexvol(flexvol string, sizeBytes uint64) error {
	flexvolSizeBytes, err := d.getOptimalSizeForFlexvol(flexvol, sizeBytes)
	if err != nil {
		log.Warnf("Could not calculate optimal Flexvol size. %v", err)
		// Lacking the optimal size, just grow the Flexvol to contain the new LUN
		size := strconv.FormatUint(sizeBytes, 10)
		resizeResponse, err := d.API.VolumeSetSize(flexvol, "+"+size)
		if err = api.GetError(resizeResponse, err); err != nil {
			return fmt.Errorf("flexvol resize failed: %v", err)
		}
	} else {
		// Got optimal size, so just set the Flexvol to that value
		flexvolSizeStr := strconv.FormatUint(flexvolSizeBytes, 10)
		resizeResponse, err := d.API.VolumeSetSize(flexvol, flexvolSizeStr)
		if err = api.GetError(resizeResponse, err); err != nil {
			return fmt.Errorf("flexvol resize failed: %v", err)
		}
	}
	return nil
}
