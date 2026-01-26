// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring/v2"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// //////////////////////////////////////////////////////////////////////////////////////////
// /             _____________________
// /            |   <<Interface>>    |
// /            |       ONTAPI       |
// /            |____________________|
// /                ^             ^
// /     Implements |             | Implements
// /   ____________________    ____________________
// /  |  ONTAPAPIREST     |   |  ONTAPAPIZAPI     |
// /  |___________________|   |___________________|
// /  | +API: RestClient  |   | +API: *Client     |
// /  |___________________|   |___________________|
// /
// //////////////////////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////////////////////
// Drivers that offer dual support are to call ONTAP REST or ZAPI's
// via abstraction layer (ONTAPI interface)
// //////////////////////////////////////////////////////////////////////////////////////////

// NASStorageDriver is for NFS and SMB storage provisioning
type NASStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         api.OntapAPI
	AWSAPI      awsapi.AWSAPI
	telemetry   *Telemetry

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers *sync.Map
}

func (d *NASStorageDriver) GetConfig() drivers.DriverConfig {
	return d.Config.SmartCopy()
}

func (d *NASStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
	return d.Config.SmartCopy()
}

func (d *NASStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *NASStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d *NASStorageDriver) Name() string {
	return tridentconfig.OntapNASStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *NASStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapnas_" + d.Config.DataLIF)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *NASStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "NASStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	var err error

	if d.Config.CommonStorageDriverConfig == nil {

		// Initialize the driver's CommonStorageDriverConfig
		d.Config.CommonStorageDriverConfig = commonConfig

		// Parse the config
		config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
		if err != nil {
			return fmt.Errorf("error initializing %s driver; %v", d.Name(), err)
		}

		d.Config = *config
	}

	// Initialize AWS API if applicable.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.AWSAPI == nil {
		d.AWSAPI, err = initializeAWSDriver(ctx, &d.Config)
		if err != nil {
			return fmt.Errorf("error initializing %s AWS driver; %v", d.Name(), err)
		}
	}

	// Initialize the ONTAP API.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = InitializeOntapDriver(ctx, &d.Config); err != nil {
			return fmt.Errorf("error initializing %s driver; %v", d.Name(), err)
		}
	}

	// Load default config parameters
	if err = PopulateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	// Validate the none, true/false values
	err = d.validate(ctx)
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

	// Identify non-overlapping storage backend pools on the driver backend.
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %v", err)
	}
	d.Config.BackendPools = pools

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	// Set up the clone split timers
	d.cloneSplitTimers = &sync.Map{}

	d.initialized = true
	return nil
}

func (d *NASStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASStorageDriver) Terminate(ctx context.Context, backendUUID string) {
	fields := LogFields{"Method": "Terminate", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	if d.Config.AutoExportPolicy {
		policyName := getExportPolicyName(backendUUID)

		if err := destroyExportPolicy(ctx, policyName, d.API); err != nil {
			Logc(ctx).Warn(err)
		}
	}
	if d.telemetry != nil {
		d.telemetry.Stop()
	}
	d.API.Terminate()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *NASStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := validateReplicationConfig(ctx, d.Config.ReplicationPolicy, d.Config.ReplicationSchedule,
		d.API); err != nil {
		return fmt.Errorf("replication validation failed: %v", err)
	}

	if err := ValidateNASDriver(ctx, d.API, &d.Config); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePools(
		ctx, d.physicalPools, d.virtualPools, d, api.MaxNASLabelLength,
	); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

func (d *NASStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Create",
		"Type":   "NASStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// If the volume already exists, bail out
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return drivers.NewVolumeExistsError(name)
	}

	// If volume shall be mirrored, check that the SVM is peered with the other side
	if volConfig.PeerVolumeHandle != "" {
		if err = checkSVMPeered(ctx, volConfig, d.API.SVMName(), d.API); err != nil {
			return err
		}
	}

	// Get candidate physical pools
	physicalPools, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
	if err != nil {
		return err
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	var (
		spaceReserve      = collection.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = collection.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = collection.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		preserveUnlink    = collection.GetV(opts, "preserveUnlink", storagePool.InternalAttributes()[PreserveUnlink])
		unixPermissions   = collection.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		snapshotDir       = collection.GetV(opts, "snapshotDir", storagePool.InternalAttributes()[SnapshotDir])
		exportPolicy      = collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = collection.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = collection.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = collection.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		skipRecoveryQueue = collection.GetV(opts, "skipRecoveryQueue", storagePool.InternalAttributes()[SkipRecoveryQueue])
		adAdminUser       = collection.GetV(opts, "adAdminUser", storagePool.InternalAttributes()[ADAdminUser])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
	)
	
	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	// Determine volume size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	sizeBytes = GetVolumeSize(sizeBytes, storagePool.InternalAttributes()[Size])

	// Get the flexvol size based on the snapshot reserve
	flexvolSize := drivers.CalculateVolumeSizeBytes(ctx, name, sizeBytes, snapshotReserveInt)

	size := strconv.FormatUint(flexvolSize, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	enablePreserveUnlink, err := strconv.ParseBool(preserveUnlink)
	if err != nil {
		return fmt.Errorf("invalid boolean value for preserveUnlink: %v", err)
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if adAdminUser != "" {
		if _, exists := volConfig.SMBShareACL[adAdminUser]; !exists {
			volConfig.SMBShareACL[adAdminUser] = ADAdminUserPermission
		}
	}

	if _, err = strconv.ParseBool(skipRecoveryQueue); skipRecoveryQueue != "" && err != nil {
		return fmt.Errorf("invalid boolean value for skipRecoveryQueue: %v", err)
	}

	if d.Config.AutoExportPolicy {
		// set empty export policy on flexVol creation
		exportPolicy = getEmptyExportPolicyName(*d.Config.StoragePrefix)

		// only create the empty policy if autoExportPolicy = true
		err = ensureExportPolicyExists(ctx, exportPolicy, d.API)
		if err != nil {
			return fmt.Errorf("error ensuring export policy exists: %v", err)
		}

		volConfig.AccessInfo.PublishEnforcement = true
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.PreserveUnlink = preserveUnlink
	volConfig.UnixPermissions = unixPermissions
	volConfig.SnapshotDir = snapshotDir
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = configEncryption
	volConfig.SkipRecoveryQueue = skipRecoveryQueue
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"size":              size,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"preserveUnlink":    enablePreserveUnlink,
		"unixPermissions":   unixPermissions,
		"snapshotDir":       enableSnapshotDir,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"encryption":        convert.ToPrintableBoolPtr(enableEncryption),
		"tieringPolicy":     tieringPolicy,
		"skipRecoveryQueue": skipRecoveryQueue,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error: %v", storagePool.Name(), aggregate, aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		// Make comment field from labels
		labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
		if labelErr != nil {
			return labelErr
		}

		// Create the volume
		volumeCreateRequest := api.Volume{
			Comment:         labels,
			Encrypt:         enableEncryption,
			ExportPolicy:    exportPolicy,
			Name:            name,
			Qos:             qosPolicyGroup,
			Size:            size,
			SpaceReserve:    spaceReserve,
			SnapshotPolicy:  snapshotPolicy,
			SecurityStyle:   securityStyle,
			SnapshotReserve: snapshotReserveInt,
			TieringPolicy:   tieringPolicy,
			UnixPermissions: unixPermissions,
			DPVolume:        volConfig.IsMirrorDestination,
		}

		// Only set aggregates for non-disaggregated systems
		if !d.API.IsDisaggregated() {
			volumeCreateRequest.Aggregates = []string{aggregate}
		}

		err = d.API.VolumeCreate(ctx, volumeCreateRequest)
		if err != nil {
			if api.IsVolumeCreateJobExistsError(err) {
				return nil
			}

			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error creating volume %s: %v", storagePool.Name(),
				aggregate, name, err)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))
			continue
		}

		if !enableSnapshotDir {
			if err := d.API.VolumeModifySnapshotDirectoryAccess(ctx, name, false); err != nil {
				createErrors = append(createErrors,
					fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error disabling snapshot directory access for volume %v: %v",
						storagePool.Name(), name, err))
				return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
			}
		}
		// If a DP volume, skip mounting the volume
		if volConfig.IsMirrorDestination {
			return nil
		}

		// Mount the volume at the specified junction
		if err := d.API.VolumeMount(ctx, name, "/"+name); err != nil {
			return err
		}

		if d.Config.NASType == sa.SMB {
			if err := d.EnsureSMBShare(ctx, name, "/"+name, volConfig.SMBShareACL, volConfig.SecureSMBEnabled); err != nil {
				return err
			}
		}

		// Set is-unlink-preserve-enabled if required
		if enablePreserveUnlink {
			setPresrveUnlink(ctx, d.API, volConfig.InternalName)
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *NASStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	// Ensure the volume exists
	flexvol, err := d.API.VolumeInfo(ctx, cloneVolConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	}

	fields := LogFields{
		"Method":      "CreateClone",
		"Type":        "NASStorageDriver",
		"name":        cloneVolConfig.InternalName,
		"source":      cloneVolConfig.CloneSourceVolumeInternal,
		"snapshot":    cloneVolConfig.CloneSourceSnapshotInternal,
		"storagePool": storagePool,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> CreateClone")
	defer Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< CreateClone")

	opts := d.GetVolumeOpts(context.Background(), cloneVolConfig, make(map[string]sa.Request))

	labels := flexvol.Comment

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value and adAdminUser value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string
	var adAdminUser string

	var labelErr error
	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

		// Make comment field from labels
		labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePoolTemp, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
		if labelErr != nil {
			return labelErr
		}
		if d.Config.NASType == sa.SMB {
			adAdminUser = d.Config.ADAdminUser
		}
	} else {
		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePool, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength); labelErr != nil {
			return labelErr
		}
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]

		if d.Config.NASType == sa.SMB {
			// Update the clone volume config with the admin user details from the storage pool
			adAdminUser = storagePool.InternalAttributes()[ADAdminUser]
		}
	}

	if d.Config.NASType == sa.SMB {
		if adAdminUser != "" {
			if _, exists := cloneVolConfig.SMBShareACL[adAdminUser]; !exists {
				cloneVolConfig.SMBShareACL[adAdminUser] = ADAdminUserPermission
			}
		}
	}

	if cloneVolConfig.ReadOnlyClone {
		if flexvol.SnapshotDir == nil {
			return fmt.Errorf("snapshot directory access is undefined on flexvol %s", flexvol.Name)
		}

		if *flexvol.SnapshotDir == false {
			return fmt.Errorf("snapshot directory access is set to %t and readOnly clone is set to %t ",
				*flexvol.SnapshotDir, cloneVolConfig.ReadOnlyClone)
		}

		if d.Config.NASType == sa.SMB && cloneVolConfig.SecureSMBEnabled {
			if err := d.EnsureSMBShare(ctx, cloneVolConfig.InternalName, "/"+cloneVolConfig.CloneSourceVolumeInternal,
				cloneVolConfig.SMBShareACL, cloneVolConfig.SecureSMBEnabled); err != nil {
				return err
			}
		}
		return nil
	}

	// If storagePoolSplitOnCloneVal is still unknown, set it to backend's default value
	if storagePoolSplitOnCloneVal == "" {
		storagePoolSplitOnCloneVal = d.Config.SplitOnClone
	}

	split, err := strconv.ParseBool(collection.GetV(opts, "splitOnClone", storagePoolSplitOnCloneVal))
	if err != nil {
		return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
	}

	qosPolicy := collection.GetV(opts, "qosPolicy", "")
	adaptiveQosPolicy := collection.GetV(opts, "adaptiveQosPolicy", "")
	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating volume clone.")

	if err = cloneFlexvol(ctx, cloneVolConfig, labels, split, &d.Config, d.GetAPI(), qosPolicyGroup); err != nil {
		return err
	}

	// Cloned volume in ontap by default will use the same export policy as the source volume.
	// We need to make sure the correct export policy is set on the clone.
	exportPolicy := collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])

	// only create the empty policy if autoExportPolicy = true
	if d.Config.AutoExportPolicy {
		// set empty export policy on flexVol creation
		exportPolicy = getEmptyExportPolicyName(*d.Config.StoragePrefix)

		err = ensureExportPolicyExists(ctx, exportPolicy, d.API)
		if err != nil {
			return fmt.Errorf("error ensuring export policy exists: %v", err)
		}
	}

	// Set export policy on the new cloned volume
	err = d.API.VolumeModifyExportPolicy(ctx, cloneVolConfig.InternalName, exportPolicy)
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error setting volume %s to empty export policy.", cloneVolConfig.InternalName)
		return err
	}

	cloneVolConfig.ExportPolicy = exportPolicy

	if d.Config.NASType == sa.SMB {
		if err := d.EnsureSMBShare(ctx, cloneVolConfig.InternalName, "/"+cloneVolConfig.InternalName,
			cloneVolConfig.SMBShareACL, cloneVolConfig.SecureSMBEnabled); err != nil {
			return err
		}
	}
	return nil
}

// Destroy the volume
func (d *NASStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "NASStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// Handle ReadOnlyClone case early
	if volConfig.ReadOnlyClone {
		if d.Config.NASType == sa.SMB && volConfig.SecureSMBEnabled {
			if err := d.DestroySMBShare(ctx, name); err != nil {
				return err
			}
		}
		return nil // Return early for all ReadOnlyClone cases
	}

	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.

	// First, check to see if the volume has already been deleted out of band
	volumeExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for volume %v: %v", name, err)
	}
	if !volumeExists {
		// Not an error if the volume no longer exists
		Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		return nil
	}

	defer func() {
		deleteAutomaticSnapshot(ctx, d, err, volConfig, d.API.VolumeSnapshotDelete)
	}()

	skipRecoveryQueue := false
	if skipRecoveryQueueValue, err := strconv.ParseBool(volConfig.SkipRecoveryQueue); err == nil {
		skipRecoveryQueue = skipRecoveryQueueValue
	}

	// If volume exists and this is FSx, try the FSx SDK first so that any backup mirror relationship
	// is cleaned up.  If the volume isn't found, then FSx may not know about it yet, so just try the
	// underlying ONTAP delete call.  Any race condition with FSx will be resolved on a retry.
	if d.AWSAPI != nil {
		err = destroyFSxVolume(ctx, d.AWSAPI, volConfig, &d.Config)
		if skipRecoveryQueue && (err == nil || errors.IsNotFoundError(err)) {
			purgeRecoveryQueueVolume(ctx, d.API, volConfig.InternalName)
		}
		if err == nil || !errors.IsNotFoundError(err) {
			return err
		}
	}

	// If flexvol has been a snapmirror destination
	if err = d.API.SnapmirrorDeleteViaDestination(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	// If flexvol has been a snapmirror source
	if err = d.API.SnapmirrorRelease(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	if err = d.API.VolumeDestroy(ctx, name, true, skipRecoveryQueue); err != nil {
		return err
	}

	if d.Config.NASType == sa.SMB {
		if err = d.DestroySMBShare(ctx, name); err != nil {
			return err
		}
	}

	return nil
}

func (d *NASStorageDriver) Import(
	ctx context.Context, volConfig *storage.VolumeConfig, originalName string,
) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "NASStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
		"noRename":     volConfig.ImportNoRename,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Ensure the volume exists
	flexvol, err := d.API.VolumeInfo(ctx, originalName)
	if api.IsVolumeReadError(err) {
		return err
	}

	// Validate the volume is what it should be
	if !api.IsVolumeIdAttributesReadError(err) {
		if flexvol.AccessType != "" && flexvol.AccessType != "rw" {
			Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("volume %s type is %s, not rw", originalName, flexvol.AccessType)
		}

		// Make sure we're not importing a volume without a junction path when not managed
		if volConfig.ImportNotManaged {
			if flexvol.JunctionPath == "" {
				return fmt.Errorf("junction path is not set for volume %s", originalName)
			}
		}
	} else {
		if volConfig.ImportNotManaged {
			return err
		}
	}

	// Rename the volume if Trident will manage its lifecycle and the names are different
	if !volConfig.ImportNotManaged && !volConfig.ImportNoRename {
		if err := d.API.VolumeRename(ctx, originalName, volConfig.InternalName); err != nil {
			return err
		}
	}

	// If autoExportPolicy is turned on and trident is managing the volume, set volume to empty export policy so that
	// subsequent volume publish will set the correct per-volume export policy.
	if d.Config.AutoExportPolicy && !volConfig.ImportNotManaged {
		if err = d.setVolToEmptyPolicy(ctx, volConfig.InternalName); err != nil {
			return err
		}

		volConfig.ExportPolicy = getEmptyExportPolicyName(*d.Config.StoragePrefix)
	}

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexvol.Comment) {
			// Set the base label
			storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

			// Make comment field from labels
			labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
				d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
			if labelErr != nil {
				return labelErr
			}
			if err := d.API.VolumeSetComment(ctx, volConfig.InternalName, originalName, labels); err != nil {
				return err
			}
		}
	}

	// Modify unix-permissions of the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		// unixPermissions specified in PVC annotation takes precedence over backend's unixPermissions config
		unixPerms := volConfig.UnixPermissions

		// ONTAP supports unix permissions only with security style "mixed" on SMB volume.
		switch d.Config.NASType {
		case sa.SMB:
			if unixPerms == "" && d.Config.SecurityStyle == "mixed" {
				unixPerms = d.Config.UnixPermissions
			} else if d.Config.SecurityStyle == "ntfs" {
				unixPerms = ""
			}
		case sa.NFS:
			if unixPerms == "" {
				unixPerms = d.Config.UnixPermissions
			}
		}

		if err := d.API.VolumeModifyUnixPermissions(
			ctx, volConfig.InternalName, originalName, unixPerms,
		); err != nil {
			return err
		}
	}

	// Update the snapshot directory access for managed volume import
	if !volConfig.ImportNotManaged && volConfig.SnapshotDir != "" {
		if enable, err := strconv.ParseBool(volConfig.SnapshotDir); err != nil {
			return fmt.Errorf("could not import volume %s, invalid snapshotDirectory annotation; %s",
				originalName, volConfig.SnapshotDir)
		} else {
			if err := d.API.VolumeModifySnapshotDirectoryAccess(ctx, volConfig.InternalName, enable); err != nil {
				return err
			}
		}
	}

	// Disable Secure SMB if the volume to be imported is not managed by Trident
	if volConfig.ImportNotManaged {
		volConfig.SecureSMBEnabled = false
	}

	if d.Config.NASType == sa.SMB {
		// Update the import volume config with the admin user details from the Trident backend
		adAdminUser := d.Config.ADAdminUser
		if adAdminUser != "" {
			if _, exists := volConfig.SMBShareACL[adAdminUser]; !exists {
				volConfig.SMBShareACL[adAdminUser] = ADAdminUserPermission
			}
		}

		shareName := originalName
		sharePath := "/" + originalName

		if flexvol.JunctionPath != "" {
			// If secure SMB is enabled, create a new share with the internal name
			if volConfig.SecureSMBEnabled {
				shareName = volConfig.InternalName
			}
			if err := d.EnsureSMBShare(ctx, shareName, sharePath,
				volConfig.SMBShareACL, volConfig.SecureSMBEnabled); err != nil {
				return err
			}
		}
	}

	return nil
}

// Rename changes the name of a volume
func (d *NASStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "NASStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	return d.API.VolumeRename(ctx, name, newName)
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method":  "Publish",
		"DataLIF": d.Config.DataLIF,
		"Type":    "NASStorageDriver",
		"name":    name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	// TODO (akerr) Figure out if this is the behavior we want or if we should be changing the junction path for
	//  managed imports
	// publishInfo.NfsPath = fmt.Sprintf("/%s", name)

	if d.Config.NASType == sa.SMB {
		publishInfo.SMBPath = volConfig.AccessInfo.SMBPath
		publishInfo.SMBServer = d.Config.DataLIF
		publishInfo.FilesystemType = sa.SMB
	} else {
		publishInfo.NfsPath = volConfig.AccessInfo.NfsPath
		publishInfo.NfsServerIP = d.Config.DataLIF
		publishInfo.FilesystemType = sa.NFS
		publishInfo.MountOptions = mountOptions
	}

	return d.publishFlexVolShare(ctx, name, volConfig, publishInfo)
}

// Unpublish the volume from the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, export policies, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	if !d.Config.AutoExportPolicy {
		Logc(ctx).Debug("Auto export policies are not turned on.")
		return nil
	}

	volExportPolicyName := volConfig.InternalName

	fields := LogFields{
		"Method":  "Unpublish",
		"Type":    "NASStorageDriver",
		"volName": volExportPolicyName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	if tridentconfig.CurrentDriverContext != tridentconfig.ContextCSI {
		return nil
	}

	volume, err := d.API.VolumeInfo(ctx, volConfig.InternalName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Error checking for existing volume.")
		return err
	}
	if volume == nil {
		Logc(ctx).WithField("volume", volConfig.InternalName).Debug("Volume not found.")
		return errors.NotFoundError("volume not found")
	}

	// Trident versions <23.04 may not have the exportPolicy field set in the volume config,
	// if this is the case we need to use the export policy from the volume info from ONTAP.
	if volConfig.ExportPolicy == "" {
		volConfig.ExportPolicy = volume.ExportPolicy
	}
	exportPolicy := volConfig.ExportPolicy
	if exportPolicy == volExportPolicyName {
		// Remove export policy rules matching the node IP address from the volume level policy
		if err = removeExportPolicyRules(ctx, exportPolicy, publishInfo, d.API); err != nil {
			Logc(ctx).WithError(err).Errorf("Error cleaning up export policy rules in %s.", exportPolicy)
			return err
		}

		// Check for other rules in the export policy
		allExportRules, err := d.API.ExportRuleList(ctx, exportPolicy)
		if err != nil {
			Logc(ctx).Errorf("Could not list export rules for policy %s.", exportPolicy)
			return err
		}
		if len(allExportRules) == 0 {
			// Set volume to the empty policy
			if err = d.setVolToEmptyPolicy(ctx, volConfig.InternalName); err != nil {
				return err
			}

			volConfig.ExportPolicy = getEmptyExportPolicyName(*d.Config.StoragePrefix)

			// Remove export policy if no rules exist
			if err = destroyExportPolicy(ctx, exportPolicy, d.API); err != nil {
				Logc(ctx).WithError(err).Errorf("Could not delete export policy %s.", exportPolicy)
				return err
			}
		}

	} else {
		// Volume is using a backend-based policy or migrating to using autoExportPolicies.
		if len(publishInfo.Nodes) == 0 {
			if err = d.setVolToEmptyPolicy(ctx, volConfig.InternalName); err != nil {
				return err
			}
			volConfig.ExportPolicy = getEmptyExportPolicyName(*d.Config.StoragePrefix)
		}
	}

	return nil
}

// setVolToEmptyPolicy  set export policy to the empty policy.
func (d *NASStorageDriver) setVolToEmptyPolicy(ctx context.Context, volName string) error {
	fields := LogFields{"flexvol": volName}
	emptyExportPolicy := getEmptyExportPolicyName(*d.Config.StoragePrefix)

	fields["exportPolicy"] = emptyExportPolicy
	exists, err := d.API.ExportPolicyExists(ctx, emptyExportPolicy)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Errorf("Could not check if export policy %s exists.", emptyExportPolicy)
		return err
	}

	if !exists {
		Logc(ctx).WithFields(fields).Debug("Export policy not found, attempting to create it.")
		if err = ensureExportPolicyExists(ctx, emptyExportPolicy, d.API); err != nil {
			Logc(ctx).WithFields(fields).
				WithError(err).Errorf("Could not create empty export policy %s.", emptyExportPolicy)
			return err
		}
	}

	err = d.API.VolumeModifyExportPolicy(ctx, volName, emptyExportPolicy)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Errorf("Error setting volume %s to empty export policy.", volName)
		return err
	}

	return nil
}

func (d *NASStorageDriver) publishFlexVolShare(
	ctx context.Context, flexvol string, volConfig *storage.VolumeConfig,
	publishInfo *models.VolumePublishInfo,
) error {
	fields := LogFields{
		"Method": "publishShare",
		"Type":   "ontap_nas",
		"Share":  flexvol,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> publishFlexVolShare")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< publishFlexVolShare")

	if !d.Config.AutoExportPolicy || publishInfo.Unmanaged {
		return nil
	}

	var targetNode *models.Node
	for _, node := range publishInfo.Nodes {
		if node.Name == publishInfo.HostName {
			targetNode = node
			break
		}
	}
	if targetNode == nil {
		err := fmt.Errorf("node %s has not registered with Trident", publishInfo.HostName)
		Logc(ctx).Error(err)
		return err
	}

	// Ensure the flexVol has the correct export policy and rules applied.
	backendPolicyName := getExportPolicyName(publishInfo.BackendUUID)
	flexVolPolicyName := flexvol
	// Trident versions <23.04 may not have the exportPolicy field set in the volume config,
	// if this is the case we need to use the export policy from the volume info from ONTAP.
	if volConfig.ExportPolicy != backendPolicyName && volConfig.ExportPolicy != flexvol &&
		volConfig.ExportPolicy != getEmptyExportPolicyName(*d.Config.StoragePrefix) {
		volume, err := d.API.VolumeInfo(ctx, flexvol)
		if err != nil {
			return err
		}
		if volume == nil {
			return errors.NotFoundError("volume not found")
		}
		volConfig.ExportPolicy = volume.ExportPolicy
	}

	// If the flexVol already have backend policy, leave it as it is. We will have an opportunity to migrate it
	// to flexVol policy during unpublish.
	switch volConfig.ExportPolicy {
	case getEmptyExportPolicyName(*d.Config.StoragePrefix), flexvol:
		flexVolPolicyName = flexvol
	case backendPolicyName:
		flexVolPolicyName = backendPolicyName
	default:
		// This can happen if the customer switched from autoExportPolicy=false to true and the volume is still using
		// default or user supplied export policy.
		Logc(ctx).Debugf("Export policy %s is not managed by Trident.", volConfig.ExportPolicy)
		return nil
	}

	volConfig.ExportPolicy = flexVolPolicyName

	if err := ensureNodeAccessForPolicy(ctx, targetNode, d.API, &d.Config, flexVolPolicyName); err != nil {
		return err
	}

	err := d.API.VolumeModifyExportPolicy(ctx, flexvol, flexVolPolicyName)
	if err != nil {
		err = fmt.Errorf("error modifying flexvol export policy; %v", err)
		Logc(ctx).WithFields(LogFields{
			"FlexVol":      flexvol,
			"ExportPolicy": flexVolPolicyName,
		}).Error(err)
		return err
	}

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// getFlexvolSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func getFlexvolSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "getFlexvolSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getFlexvolSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getFlexvolSnapshot")

	size, err := client.VolumeUsedSize(ctx, internalVolName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, err
		} else {
			return nil, fmt.Errorf("error reading volume size: %v", err)
		}
	}

	snap, err := client.VolumeSnapshotInfo(ctx, internalSnapName, internalVolName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
		"created":      snap.CreateTime,
	}).Debug("Found snapshot.")
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snap.CreateTime,
		SizeBytes: int64(size),
		State:     storage.SnapshotStateOnline,
	}, nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NASStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// getFlexvolSnapshotList returns the list of snapshots associated with the named volume.
func getFlexvolSnapshotList(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName

	fields := LogFields{
		"Method":     "getFlexvolSnapshotList",
		"Type":       "NASStorageDriver",
		"volumeName": internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getFlexvolSnapshotList")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getFlexvolSnapshotList")

	size, err := client.VolumeUsedSize(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Snapshot, 0)

	for _, snap := range snapshots {

		Logc(ctx).WithFields(LogFields{
			"name":       snap.Name,
			"accessTime": snap.CreateTime,
		}).Debug("Snapshot")

		snapshot := &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               snap.Name,
				InternalName:       snap.Name,
				VolumeName:         volConfig.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			Created:   snap.CreateTime,
			SizeBytes: int64(size),
			State:     storage.SnapshotStateOnline,
		}

		result = append(result, snapshot)
	}

	return result, nil
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *NASStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "NASStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getFlexvolSnapshotList(ctx, volConfig, &d.Config, d.API)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"sourceVolume": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	return createFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API, d.volumeUsedSize)
}

func (d *NASStorageDriver) volumeUsedSize(ctx context.Context, volumeName string) (int, error) {
	fields := LogFields{
		"Method":       "volumeUsedSize",
		"Type":         "NASStorageDriver",
		"sourceVolume": volumeName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> volumeUsedSize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< volumeUsedSize")

	size, err := d.API.VolumeUsedSize(ctx, volumeName)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *NASStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	if err := d.API.VolumeSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName); err != nil {
		if api.IsSnapshotBusyError(err) {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, &d.Config, d.API,
				d.API.VolumeCloneSplitStart, d.cloneSplitTimers)
		}

		// We must return the error, even if we started a split, so the snapshot delete is retried.
		return err
	}

	// Clean up any split timer
	d.cloneSplitTimers.Delete(snapConfig.ID())

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// GetGroupSnapshotTarget returns a set of information about the target of a group snapshot.
// This information is used to gather information in a consistent way across storage drivers.
func (d *NASStorageDriver) GetGroupSnapshotTarget(
	ctx context.Context, volConfigs []*storage.VolumeConfig,
) (*storage.GroupSnapshotTargetInfo, error) {
	fields := LogFields{
		"Method": "GetGroupSnapshotTarget",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetGroupSnapshotTarget")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetGroupSnapshotTarget")

	return GetGroupSnapshotTarget(ctx, volConfigs, &d.Config, d.API)
}

func (d *NASStorageDriver) CreateGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, target *storage.GroupSnapshotTargetInfo,
) error {
	fields := LogFields{
		"Method": "CreateGroupSnapshot",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateGroupSnapshot")

	return CreateGroupSnapshot(ctx, config, target, &d.Config, d.API)
}

func (d *NASStorageDriver) ProcessGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, volConfigs []*storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method": "ProcessGroupSnapshot",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ProcessGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ProcessGroupSnapshot")

	return ProcessGroupSnapshot(ctx, config, volConfigs, &d.Config, d.API, d.volumeUsedSize)
}

func (d *NASStorageDriver) ConstructGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, snapshotIDs []*storage.Snapshot,
) (*storage.GroupSnapshot, error) {
	fields := LogFields{
		"Method": "ConstructGroupSnapshot",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ConstructGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ConstructGroupSnapshot")

	return ConstructGroupSnapshot(ctx, config, snapshotIDs, &d.Config)
}

// Get tests for the existence of a volume
func (d *NASStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "NASStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("Volume", name).Debug("Volume not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *NASStorageDriver) GetStorageBackendSpecs(
	_ context.Context, backend storage.Backend,
) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NASStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *NASStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "NASStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. SVM UUID
	// 2. Aggregate (physical pool)
	svmUUID := d.GetAPI().GetSVMUUID()

	backendPools := make([]drivers.OntapStorageBackendPool, 0)
	for _, pool := range d.physicalPools {
		backendPool := drivers.OntapStorageBackendPool{
			SvmUUID:   svmUUID,
			Aggregate: pool.Name(),
		}
		backendPools = append(backendPools, backendPool)
	}

	return backendPools
}

func (d *NASStorageDriver) getStoragePoolAttributes(ctx context.Context) map[string]sa.Offer {
	client := d.GetAPI()
	mirroring, _ := client.IsSVMDRCapable(ctx)
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(mirroring),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *NASStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

func (d *NASStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
	// If no pool is specified, a new pool is created and assigned a name template and label from the common configuration.
	// The process of generating a custom volume name necessitates a name template and label.
	if storage.IsStoragePoolUnset(pool) {
		pool = ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)
	}
	createPrepareCommon(ctx, d, volConfig, pool)
}

func (d *NASStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	var accessPath string
	var flexvol *api.Volume
	var err error

	fields := LogFields{
		"Method":       "CreateFollowup",
		"Type":         "NASStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")

	if d.Config.NASType == sa.SMB {
		volConfig.AccessInfo.SMBServer = d.Config.DataLIF
		volConfig.FileSystem = sa.SMB
	} else {
		volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
		volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
		volConfig.FileSystem = sa.NFS
	}

	// Set correct junction path
	// If it's a RO clone, get source volume
	if volConfig.ReadOnlyClone {
		flexvol, err = d.API.VolumeInfo(ctx, volConfig.CloneSourceVolumeInternal)
		if err != nil {
			return err
		}
	} else {
		flexvol, err = d.API.VolumeInfo(ctx, volConfig.InternalName)
		if err != nil {
			return err
		}
	}

	if flexvol.JunctionPath == "" {
		if flexvol.AccessType == "rw" || flexvol.AccessType == "dp" {
			// Flexvol will not be mounted in the following scenarios, we need to mount it,
			// 1. During Import of volume without Junction path.
			// 2. During Create/CreateClone there is a failure and mount is not performed.

			if d.Config.NASType == sa.SMB {
				volConfig.AccessInfo.SMBPath = ConstructOntapNASVolumeAccessPath(ctx, d.Config.SMBShare,
					volConfig.InternalName, volConfig, sa.SMB)
				// Overwriting mount path, mounting at root instead of admin share
				volConfig.AccessInfo.SMBPath = "/" + volConfig.InternalName
				accessPath = volConfig.AccessInfo.SMBPath
			} else {
				volConfig.AccessInfo.NfsPath = ConstructOntapNASVolumeAccessPath(ctx, d.Config.SMBShare,
					volConfig.InternalName, volConfig, sa.NFS)
				accessPath = volConfig.AccessInfo.NfsPath
			}

			err = d.MountVolume(ctx, volConfig.InternalName, accessPath, flexvol)
			if err != nil {
				return err
			}

			// If smbShare is omitted in the backend configuration then,
			// Trident tries to create smbShare with the same name as volume InternalName.
			// This check ensures that volume is mounted before the share is created.
			if d.Config.NASType == sa.SMB {
				if err := d.EnsureSMBShare(ctx, volConfig.InternalName, "/"+volConfig.InternalName,
					volConfig.SMBShareACL, volConfig.SecureSMBEnabled); err != nil {
					return err
				}
			}
		}
	} else {
		if d.Config.NASType == sa.SMB {
			volConfig.AccessInfo.SMBPath = ConstructOntapNASVolumeAccessPath(ctx, d.Config.SMBShare,
				flexvol.JunctionPath, volConfig, sa.SMB)
		} else {
			volConfig.AccessInfo.NfsPath = ConstructOntapNASVolumeAccessPath(ctx, d.Config.SMBShare,
				flexvol.JunctionPath, volConfig, sa.NFS)
		}
	}
	return nil
}

func (d *NASStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NASStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name of the
// Flexvol on the storage system.
func (d *NASStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	volume, err := d.API.VolumeInfo(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	return getVolumeExternalCommon(*volume, *d.Config.StoragePrefix, d.API.SVMName()), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASStorageDriver) GetVolumeExternalWrappers(
	ctx context.Context, channel chan *storage.VolumeExternalWrapper,
) {
	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumes, err := d.API.VolumeListByPrefix(ctx, *d.Config.StoragePrefix)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range volumes {
		channel <- &storage.VolumeExternalWrapper{
			Volume: getVolumeExternalCommon(*volume, *d.Config.StoragePrefix, d.API.SVMName()),
			Error:  nil,
		}
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASStorageDriver) GetUpdateType(ctx context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	fields := LogFields{
		"Method": "GetUpdateType",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetUpdateType")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetUpdateType")

	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.Password != dOrig.Config.Password {
		bitmap.Add(storage.PasswordChange)
	}

	if d.Config.Username != dOrig.Config.Username {
		bitmap.Add(storage.UsernameChange)
	}

	if !drivers.AreSameCredentials(d.Config.Credentials, dOrig.Config.Credentials) {
		bitmap.Add(storage.CredentialsChange)
	}

	if !reflect.DeepEqual(d.Config.StoragePrefix, dOrig.Config.StoragePrefix) {
		bitmap.Add(storage.PrefixChange)
	}

	return bitmap
}

// Resize expands the volume size.
func (d *NASStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":             "Resize",
		"Type":               "NASStorageDriver",
		"name":               name,
		"volConfig.Size":     volConfig.Size,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Validation checks
	newFlexvolSize, err := resizeValidation(ctx, volConfig, requestedSizeBytes,
		d.API.VolumeExists, d.API.VolumeSize, d.API.VolumeInfo)
	if err != nil {
		return err
	}
	if newFlexvolSize == 0 && err == nil {
		// nothing to do
		return nil
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
		ctx, name, newFlexvolSize, d.Config, d.GetAPI(),
	); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	if err := d.API.VolumeSetSize(ctx, name, strconv.FormatUint(newFlexvolSize, 10)); err != nil {
		return err
	}

	// update with the resized volume size
	volConfig.Size = strconv.FormatUint(requestedSizeBytes, 10)
	return nil
}

func (d *NASStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*models.Node, backendUUID, _ string,
) error {
	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "NASStorageDriver",
		"Nodes":  nodeNames,
	}
	Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	policyName := getExportPolicyName(backendUUID)

	return d.reconcileNodeAccessForBackendPolicy(ctx, nodes, policyName)
}

func (d *NASStorageDriver) reconcileNodeAccessForBackendPolicy(
	ctx context.Context, nodes []*models.Node, policyName string,
) error {
	if !d.Config.AutoExportPolicy {
		return nil
	}

	if exists, err := d.API.ExportPolicyExists(ctx, policyName); err != nil {
		return err
	} else if !exists {
		Logc(ctx).WithField("ExportPolicy", policyName).Debug("Export policy not found.")
		return nil
	}

	desiredRules, err := getDesiredExportPolicyRules(ctx, nodes, &d.Config)
	if err != nil {
		err = fmt.Errorf("unable to determine desired export policy rules; %v", err)
		Logc(ctx).Error(err)
		return err
	}

	err = reconcileExportPolicyRules(ctx, policyName, desiredRules, d.API, &d.Config)
	if err != nil {
		err = fmt.Errorf("unabled to reconcile export policy rules; %v", err)
		Logc(ctx).WithField("ExportPolicy", policyName).Error(err)
		return err
	}
	return nil
}

func (d *NASStorageDriver) ReconcileVolumeNodeAccess(
	ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node,
) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "NASStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *NASStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "nfs", d.GetStorageBackendPhysicalPoolNames(ctx), d.Config.Aggregate)
}

// String makes NASStorageDriver satisfy the Stringer interface.
func (d NASStorageDriver) String() string {
	return convert.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASStorageDriver satisfy the GoStringer interface.
func (d NASStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d NASStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// EstablishMirror will create a new mirror relationship between a RW and a DP volume that have not previously
// had a relationship
func (d *NASStorageDriver) EstablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
) error {
	// If replication policy in TMR is empty use the backend policy
	if replicationPolicy == "" {
		replicationPolicy = d.Config.ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicy(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.Config.ReplicationPolicy)
		replicationPolicy = d.Config.ReplicationPolicy
		isAsync, err = validateReplicationPolicy(ctx, replicationPolicy, d.API)
		if err != nil {
			Logc(ctx).Debugf("Replication policy %s in backend should be valid.", replicationPolicy)
		}
	}

	// If replication policy is async type, validate the replication schedule from TMR or use backend schedule
	if isAsync {
		if replicationSchedule != "" {
			if err := validateReplicationSchedule(ctx, replicationSchedule, d.API); err != nil {
				Logc(ctx).Debugf("Replication schedule given in TMR %s is invalid, using schedule %s from backend.",
					replicationSchedule, d.Config.ReplicationSchedule)
				replicationSchedule = d.Config.ReplicationSchedule
			}
		} else {
			replicationSchedule = d.Config.ReplicationSchedule
		}
	} else {
		replicationSchedule = ""
	}

	return establishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		d.API)
}

// ReestablishMirror will attempt to resync a mirror relationship, if and only if the relationship existed previously
func (d *NASStorageDriver) ReestablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
) error {
	// If replication policy in TMR is empty use the backend policy
	if replicationPolicy == "" {
		replicationPolicy = d.Config.ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicy(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.Config.ReplicationPolicy)
		replicationPolicy = d.Config.ReplicationPolicy
		isAsync, err = validateReplicationPolicy(ctx, replicationPolicy, d.API)
		if err != nil {
			Logc(ctx).Debugf("Replication policy %s in backend should be valid.", replicationPolicy)
		}
	}

	// If replication policy is async type, validate the replication schedule from TMR or use backend schedule
	if isAsync {
		if replicationSchedule != "" {
			if err := validateReplicationSchedule(ctx, replicationSchedule, d.API); err != nil {
				Logc(ctx).Debugf("Replication schedule given in TMR %s is invalid, using schedule %s from backend.",
					replicationSchedule, d.Config.ReplicationSchedule)
				replicationSchedule = d.Config.ReplicationSchedule
			}
		} else {
			replicationSchedule = d.Config.ReplicationSchedule
		}
	} else {
		replicationSchedule = ""
	}

	return reestablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		d.API)
}

// PromoteMirror will break the mirror relationship and make the destination volume RW,
// optionally after a given snapshot has synced
func (d *NASStorageDriver) PromoteMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotName string,
) (bool, error) {
	return promoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotName,
		d.Config.ReplicationPolicy, d.API)
}

// GetMirrorStatus returns the current state of a mirror relationship
func (d *NASStorageDriver) GetMirrorStatus(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, error) {
	return getMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// ReleaseMirror will release the mirror relationship data of the source volume
func (d *NASStorageDriver) ReleaseMirror(ctx context.Context, localInternalVolumeName string) error {
	return releaseMirror(ctx, localInternalVolumeName, d.API)
}

// GetReplicationDetails returns the replication policy and schedule of a mirror relationship
func (d *NASStorageDriver) GetReplicationDetails(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, string, string, error) {
	return getReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// UpdateMirror will attempt a mirror update for the given destination volume
func (d *NASStorageDriver) UpdateMirror(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	return mirrorUpdate(ctx, localInternalVolumeName, snapshotName, d.API)
}

// CheckMirrorTransferState will look at the transfer state of the mirror relationship to determine if it is failed,
// succeeded or in progress
func (d *NASStorageDriver) CheckMirrorTransferState(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return checkMirrorTransferState(ctx, localInternalVolumeName, d.API)
}

// GetMirrorTransferTime will return the transfer time of the mirror relationship
func (d *NASStorageDriver) GetMirrorTransferTime(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return getMirrorTransferTime(ctx, localInternalVolumeName, d.API)
}

// MountVolume returns the volume mount error(if any)
func (d *NASStorageDriver) MountVolume(
	ctx context.Context, name, junctionPath string, flexVol *api.Volume,
) error {
	if err := d.API.VolumeMount(ctx, name, junctionPath); err != nil {
		// An API error is returned if we attempt to mount a DP volume that has not yet been snapmirrored,
		// we expect this to be the case.

		if api.IsApiError(err) && flexVol.DPVolume {
			Logc(ctx).Debugf("Received expected API error when mounting DP volume to junction; %v", err)
		} else {
			return fmt.Errorf("error mounting volume to junction %s; %v", junctionPath, err)
		}
	}
	return nil
}

// EnsureSMBShare ensures that required SMB share is made available.
func (d *NASStorageDriver) EnsureSMBShare(
	ctx context.Context, name, path string, smbShareACL map[string]string, secureSMBEnabled bool,
) error {
	shareName := name
	sharePath := path

	// Determine share name and path based on secureSMBEnabled and configuration
	if !secureSMBEnabled && d.Config.SMBShare != "" {
		shareName = d.Config.SMBShare
		sharePath = "/"
	}

	// Check if the share exists
	shareExists, err := d.API.SMBShareExists(ctx, shareName)
	if err != nil {
		return err
	}

	// Create the share if it does not exist
	if !shareExists {
		if err = d.API.SMBShareCreate(ctx, shareName, sharePath); err != nil {
			return err
		}
	}

	// If secure SMB is enabled, configure access control
	if secureSMBEnabled {
		// Delete the Everyone Access Control created by default during share creation by ONTAP
		if err = d.API.SMBShareAccessControlDelete(ctx, shareName, smbShareDeleteACL); err != nil {
			return err
		}

		if err = d.API.SMBShareAccessControlCreate(ctx, shareName, smbShareACL); err != nil {
			return err
		}
	}
	return nil
}

// DestroySMBShare destroys an SMB share
func (d *NASStorageDriver) DestroySMBShare(
	ctx context.Context, name string,
) error {
	// If the share being deleted matches with the backend config, Trident will not delete the SMB share.
	if d.Config.SMBShare == name {
		return nil
	}

	shareExists, err := d.API.SMBShareExists(ctx, name)
	if err != nil {
		return err
	}

	if shareExists {
		if err := d.API.SMBShareDestroy(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// EnablePublishEnforcement prepares a volume for per-volume export policy allowing greater access control.
func (d *NASStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	// NOTE: This logic is currently handled in the Unpublish code.
	return nil
}

func (d *NASStorageDriver) CanEnablePublishEnforcement() bool {
	// Only do publish enforcement if the auto export policy is turned on
	return d.Config.AutoExportPolicy
}

func (d *NASStorageDriver) HealVolumePublishEnforcement(
	ctx context.Context, vol *storage.Volume,
) bool {
	return HealNASPublishEnforcement(ctx, d, vol)
}
