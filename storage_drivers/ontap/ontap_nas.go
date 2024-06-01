// Copyright 2023 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
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

	cloneSplitTimers map[string]time.Time
}

func (d *NASStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
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

	// Initialize the driver's CommonStorageDriverConfig
	d.Config.CommonStorageDriverConfig = commonConfig

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver; %v", d.Name(), err)
	}

	// Initialize AWS API if applicable.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.AWSAPI == nil {
		d.AWSAPI, err = initializeAWSDriver(ctx, config)
		if err != nil {
			return fmt.Errorf("error initializing %s AWS driver; %v", d.Name(), err)
		}
	}

	// Initialize the ONTAP API.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		d.API, err = InitializeOntapDriver(ctx, config)
		if err != nil {
			return fmt.Errorf("error initializing %s driver; %v", d.Name(), err)
		}
	}
	d.Config = *config

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
	d.cloneSplitTimers = make(map[string]time.Time)

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

		if err := d.API.ExportPolicyDestroy(ctx, policyName); err != nil {
			Logc(ctx).Warn(err)
		}
	}
	if d.telemetry != nil {
		d.telemetry.Stop()
	}
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

// Create a volume with the specified options
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
		spaceReserve      = utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		snapshotDir       = utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes()[SnapshotDir])
		exportPolicy      = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = utils.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
	)

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
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
	sizeBytes = GetVolumeSize(sizeBytes, storagePool.InternalAttributes()[Size])

	// Get the flexvol size based on the snapshot reserve
	flexvolSize := calculateFlexvolSizeBytes(ctx, name, sizeBytes, snapshotReserveInt)

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

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend().BackendUUID())
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
	volConfig.UnixPermissions = unixPermissions
	volConfig.SnapshotDir = snapshotDir
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = configEncryption
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"size":              size,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"snapshotDir":       enableSnapshotDir,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"encryption":        utils.GetPrintableBoolPtrValue(enableEncryption),
		"tieringPolicy":     tieringPolicy,
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
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Make comment field from labels
		labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
		if labelErr != nil {
			return labelErr
		}

		// Create the volume
		err = d.API.VolumeCreate(
			ctx, api.Volume{
				Aggregates: []string{
					aggregate,
				},
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
			})
		if err != nil {
			if api.IsVolumeCreateJobExistsError(err) {
				return nil
			}

			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error creating volume %s: %v", storagePool.Name(),
				aggregate, name, err)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
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
			if err := d.EnsureSMBShare(ctx, name, "/"+name); err != nil {
				return err
			}
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
	Logd(ctx, d.GetConfig().StorageDriverName, d.GetConfig().DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> CreateClone")
	defer Logd(ctx, d.GetConfig().StorageDriverName, d.GetConfig().DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< CreateClone")

	opts := d.GetVolumeOpts(context.Background(), cloneVolConfig, make(map[string]sa.Request))

	labels := flexvol.Comment

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string

	var labelErr error
	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.GetConfig().Labels)

		// Make comment field from labels
		labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePoolTemp, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
		if labelErr != nil {
			return labelErr
		}
	} else {
		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePool, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength); labelErr != nil {
			return labelErr
		}
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]
	}

	if cloneVolConfig.ReadOnlyClone {
		if flexvol.SnapshotDir == nil {
			return fmt.Errorf("snapshot directory access is undefined on flexvol %s", flexvol.Name)
		}
		if *flexvol.SnapshotDir == false {
			return fmt.Errorf("snapshot directory access is set to %t and readOnly clone is set to %t ",
				*flexvol.SnapshotDir, cloneVolConfig.ReadOnlyClone)
		}
		return nil
	}

	// If storagePoolSplitOnCloneVal is still unknown, set it to backend's default value
	if storagePoolSplitOnCloneVal == "" {
		storagePoolSplitOnCloneVal = d.GetConfig().SplitOnClone
	}

	split, err := strconv.ParseBool(utils.GetV(opts, "splitOnClone", storagePoolSplitOnCloneVal))
	if err != nil {
		return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
	}

	qosPolicy := utils.GetV(opts, "qosPolicy", "")
	adaptiveQosPolicy := utils.GetV(opts, "adaptiveQosPolicy", "")
	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating volume clone.")

	if err = cloneFlexvol(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
		cloneVolConfig.CloneSourceSnapshotInternal, labels, split, d.GetConfig(), d.GetAPI(), qosPolicyGroup,
	); err != nil {
		return err
	}

	if d.Config.NASType == sa.SMB {
		if err := d.EnsureSMBShare(ctx, cloneVolConfig.InternalName, "/"+cloneVolConfig.InternalName); err != nil {
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

	// If volume exists and this is FSx, try the FSx SDK first so that any backup mirror relationship
	// is cleaned up.  If the volume isn't found, then FSx may not know about it yet, so just try the
	// underlying ONTAP delete call.  Any race condition with FSx will be resolved on a retry.
	if d.AWSAPI != nil {
		err = destroyFSxVolume(ctx, d.AWSAPI, volConfig, &d.Config)
		if err == nil || !errors.IsNotFoundError(err) {
			return err
		}
	}

	// If flexvol has been a snapmirror destination
	if err := d.API.SnapmirrorDeleteViaDestination(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	// If flexvol has been a snapmirror source
	if err := d.API.SnapmirrorRelease(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	if err := d.API.VolumeDestroy(ctx, name, true); err != nil {
		return err
	}
	if d.Config.NASType == sa.SMB {
		if err := d.DestroySMBShare(ctx, name); err != nil {
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

	// Rename the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if err := d.API.VolumeRename(ctx, originalName, volConfig.InternalName); err != nil {
			return err
		}
	}

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexvol.Comment) {
			// Set the base label
			storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.GetConfig().Labels)

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

	if d.Config.NASType == sa.SMB {
		if flexvol.JunctionPath != "" {
			if err := d.EnsureSMBShare(ctx, originalName, "/"+originalName); err != nil {
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
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
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

	return publishShare(ctx, d.API, &d.Config, publishInfo, name, d.API.VolumeModifyExportPolicy)
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

	return createFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.VolumeUsedSize)
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

// DeleteSnapshot creates a snapshot of a volume.
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
	delete(d.cloneSplitTimers, snapConfig.ID())

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
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
		pool = ConstructPoolForLabels(d.Config.NameTemplate, d.GetConfig().Labels)
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
			// Trident tries to create sbmShare with the same name as volume InternalName.
			// This check ensures that volume is mounted before the share is created.
			if d.Config.NASType == sa.SMB {
				if err := d.EnsureSMBShare(ctx, volConfig.InternalName, "/"+volConfig.InternalName); err != nil {
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
	ctx context.Context, nodes []*utils.Node, backendUUID, _ string,
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

	return reconcileNASNodeAccess(ctx, nodes, &d.Config, d.API, policyName)
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *NASStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "nfs", d.GetStorageBackendPhysicalPoolNames(ctx))
}

// String makes NASStorageDriver satisfy the Stringer interface.
func (d NASStorageDriver) String() string {
	return utils.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
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
		replicationPolicy = d.GetConfig().ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicy(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.GetConfig().ReplicationPolicy)
		replicationPolicy = d.GetConfig().ReplicationPolicy
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
					replicationSchedule, d.GetConfig().ReplicationSchedule)
				replicationSchedule = d.GetConfig().ReplicationSchedule
			}
		} else {
			replicationSchedule = d.GetConfig().ReplicationSchedule
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
		replicationPolicy = d.GetConfig().ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicy(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.GetConfig().ReplicationPolicy)
		replicationPolicy = d.GetConfig().ReplicationPolicy
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
					replicationSchedule, d.GetConfig().ReplicationSchedule)
				replicationSchedule = d.GetConfig().ReplicationSchedule
			}
		} else {
			replicationSchedule = d.GetConfig().ReplicationSchedule
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
		d.GetConfig().ReplicationPolicy, d.API)
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
	ctx context.Context, name, path string,
) error {
	if d.Config.SMBShare != "" {
		// If user did specify SMB share, and it does not exist, create an SMB share with the specified name.
		share, err := d.API.SMBShareExists(ctx, d.Config.SMBShare)
		if err != nil {
			return err
		}

		// If share is not present create it.
		if !share {
			if err = d.API.SMBShareCreate(ctx, d.Config.SMBShare, "/"); err != nil {
				return err
			}
		}
	} else {
		// If user did not specify SMB share in backend configuration, create an SMB share with the name passed.
		share, err := d.API.SMBShareExists(ctx, name)
		if err != nil {
			return err
		}

		// If share is not present create it.
		if !share {
			if err = d.API.SMBShareCreate(ctx, name, path); err != nil {
				return err
			}
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
