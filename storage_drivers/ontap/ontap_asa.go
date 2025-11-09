// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

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
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

// ASAStorageDriver is for iSCSI storage provisioning
type ASAStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	wwpns       []string
	API         api.OntapAPI
	telemetry   *Telemetry
	iscsi       iscsi.ISCSI

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers *sync.Map
}

func (d *ASAStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

func (d *ASAStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *ASAStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *ASAStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d ASAStorageDriver) Name() string {
	return tridentconfig.OntapSANStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *ASAStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		lif0 := "noLIFs"
		if len(d.ips) > 0 {
			lif0 = d.ips[0]
		} else if len(d.wwpns) > 0 {
			lif0 = strings.ReplaceAll(d.wwpns[0], ":", ".")
		}
		return CleanBackendName("ontapasa_" + lif0)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *ASAStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "ASAStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	var err error

	d.iscsi, err = iscsi.New()
	if err != nil {
		return fmt.Errorf("error initializing iSCSI client: %w", err)
	}

	if d.Config.CommonStorageDriverConfig == nil {

		// Initialize the driver's CommonStorageDriverConfig
		d.Config.CommonStorageDriverConfig = commonConfig

		// Parse the config
		config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
		if err != nil {
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}

		d.Config = *config
	}

	// Initialize the ONTAP API.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = InitializeOntapDriver(ctx, &d.Config); err != nil {
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}
	}

	// Load default config parameters
	if err = PopulateASAConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}
	if d.Config.SANType == sa.FCP {
		if d.wwpns, err = d.API.NetFcpInterfaceGetDataLIFs(ctx, d.Config.SANType); err != nil {
			return err
		}

		if len(d.wwpns) == 0 {
			return fmt.Errorf("no FC data LIFs found on SVM %s", d.API.SVMName())
		} else {
			Logc(ctx).WithField("dataLIFs", d.wwpns).Debug("Found FC LIFs.")
		}
	} else {
		d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, d.Config.SANType)
		if err != nil {
			return err
		}

		if len(d.ips) == 0 {
			return fmt.Errorf("no iSCSI data LIFs found on SVM %s", d.API.SVMName())
		} else {
			Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
		}
	}
	d.physicalPools, d.virtualPools, err = InitializeManagedStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = InitializeSANDriver(ctx, driverContext, d.API, &d.Config, d.validate, backendUUID)
	if err != nil {
		if d.Config.DriverContext == tridentconfig.ContextCSI {
			// Clean up igroup for failed driver.
			if igroupErr := d.API.IgroupDestroy(ctx, d.Config.IgroupName); igroupErr != nil {
				Logc(ctx).WithError(igroupErr).WithField("igroup", d.Config.IgroupName).Warn("Error deleting igroup.")
			}
		}
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
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

	Logc(ctx).Debug("Initialized All-SAN Array backend.")

	d.initialized = true
	return nil
}

func (d *ASAStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *ASAStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "ASAStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	if d.Config.DriverContext == tridentconfig.ContextCSI {
		// clean up igroup for terminated driver
		err := d.API.IgroupDestroy(ctx, d.Config.IgroupName)
		if err != nil {
			Logc(ctx).WithError(err).Warn("Error deleting igroup.")
		}
	}

	if d.telemetry != nil {
		d.telemetry.Stop()
	}
	d.API.Terminate()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *ASAStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "ASAStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := ValidateSANDriver(ctx, &d.Config, d.ips, d.iscsi); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateASAStoragePools(ctx, d.physicalPools, d.virtualPools, d, api.MaxSANLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a LUN with the specified options
func (d *ASAStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	var fstype string

	fields := LogFields{
		"Method": "Create",
		"Type":   "ASAStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Early exit if LUN already exists.
	volExists, err := d.API.LunExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failure checking for existence of LUN: %v", err)
	}
	if volExists {
		// If LUN exists, update the volConfig.InternalID in case it was not set.  This is useful for
		// "legacy" volumes which do not have InternalID set when they were created.
		if volConfig.InternalID == "" {
			volConfig.InternalID = d.CreateASALUNInternalID(d.Config.SVM, name)
			Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("Setting InternalID.")
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	var (
		spaceAllocation   = collection.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation])
		spaceReserve      = collection.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = collection.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = collection.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = collection.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy      = collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = collection.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = collection.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = collection.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		skipRecoveryQueue = collection.GetV(opts, "skipRecoveryQueue", storagePool.InternalAttributes()[SkipRecoveryQueue])
		formatOptions     = collection.GetV(opts, "formatOptions", storagePool.InternalAttributes()[FormatOptions])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
		luksEncryption    = storagePool.InternalAttributes()[LUKSEncryption]
	)

	// Validate overridden values from the volume config
	if spaceAllocation != DefaultSpaceAllocation {
		return fmt.Errorf("spaceAllocation must be set to %s", DefaultSpaceAllocation)
	}
	if spaceReserve != DefaultSpaceReserve {
		return fmt.Errorf("spaceReserve must be set to %s", DefaultSpaceReserve)
	}
	if snapshotPolicy != DefaultSnapshotPolicy {
		return fmt.Errorf("snapshotPolicy must be set to %s", DefaultSnapshotPolicy)
	}
	if snapshotReserve != "" {
		return errors.New("snapshotReserve must not be set")
	}
	if unixPermissions != "" {
		return errors.New("unixPermissions must not be set")
	}
	if exportPolicy != "" {
		return errors.New("exportPolicy must not be set")
	}
	if securityStyle != "" {
		return errors.New("securityStyle must not be set")
	}
	if encryption != DefaultASAEncryption {
		return fmt.Errorf("encryption must be set to %s", DefaultASAEncryption)
	}
	if tieringPolicy != DefaultTieringPolicy {
		return errors.New("tieringPolicy must not be set")
	}
	if luksEncryption != DefaultLuksEncryption {
		return fmt.Errorf("luksEncryption must be set to %s", DefaultLuksEncryption)
	}

	if _, err = strconv.ParseBool(skipRecoveryQueue); skipRecoveryQueue != "" && err != nil {
		return fmt.Errorf("invalid boolean value for skipRecoveryQueue: %w", err)
	}

	// Determine LUN size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert size %s: %v", volConfig.Size, err)
	}
	requestedSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid size: %v", volConfig.Size, err)
	}
	// Apply minimum size rule
	lunSizeBytes := GetVolumeSize(requestedSizeBytes, storagePool.InternalAttributes()[Size])
	lunSize := strconv.FormatUint(lunSizeBytes, 10)
	// Apply maximum size rule
	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, lunSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	fstype, err = drivers.CheckSupportedFilesystem(
		ctx, collection.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name)
	if err != nil {
		return err
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	// Validate ASA-specific options

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(lunSizeBytes, 10)
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.UnixPermissions = unixPermissions
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = encryption
	volConfig.SkipRecoveryQueue = skipRecoveryQueue
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy
	volConfig.LUKSEncryption = luksEncryption
	volConfig.FileSystem = fstype
	volConfig.FormatOptions = formatOptions

	// Make comment field from labels
	labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
		d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
	if labelErr != nil {
		return labelErr
	}

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"lunSize":           lunSize,
		"spaceAllocation":   spaceAllocation,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"unixPermissions":   unixPermissions,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"LUKSEncryption":    luksEncryption,
		"encryption":        encryption,
		"skipRecoveryQueue": skipRecoveryQueue,
		"formatOptions":     formatOptions,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
		"labels":            labels,
	}).Debug("Creating ASA LUN.")

	err = d.API.LunCreate(
		ctx, api.Lun{
			Name:   name,
			Qos:    qosPolicyGroup,
			Size:   lunSize,
			OsType: "linux",
		})
	if err != nil {
		errMessage := fmt.Sprintf("error creating LUN %s; %v", name, err)
		Logc(ctx).Error(errMessage)
		return err
	}

	// Set labels if present
	if labels != "" {
		err = d.API.LunSetComment(ctx, name, labels)
		if err != nil {
			return fmt.Errorf("error setting labels on the LUN: %v", err)
		}
	}

	// Save the fstype in a LUN attribute so we know what to do in Attach.  If this fails, clean up and
	// move on to the next pool.
	// Save the context, fstype, and LUKS value in LUN comment
	err = d.API.LunSetAttribute(ctx, name, LUNAttributeFSType, fstype, string(d.Config.DriverContext),
		luksEncryption, formatOptions)
	if err != nil {

		errMessage := fmt.Sprintf("error saving file system type for LUN %s: %v", name, err)
		Logc(ctx).Error(errMessage)

		// Don't leave the new LUN around.
		// If the following LunDestroy() fails for any reason, LUN must be manually deleted.
		if err := d.API.LunDestroy(ctx, name); err != nil {
			Logc(ctx).WithField("LUN", name).Errorf("Could not clean up LUN; %v", err)
		} else {
			Logc(ctx).WithField("LUN", name).Debugf("Cleaned up LUN after set attribute error.")
		}
	}

	// Set internalID field in volume config
	volConfig.InternalID = d.CreateASALUNInternalID(d.Config.SVM, name)

	Logc(ctx).WithFields(LogFields{
		"name":         name,
		"internalName": volConfig.InternalName,
		"internalID":   volConfig.InternalID,
	}).Debug("Created ASA LUN.")

	return nil
}

// CreateClone creates a volume clone
func (d *ASAStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":      "CreateClone",
		"Type":        "ASAStorageDriver",
		"name":        name,
		"source":      source,
		"snapshot":    snapshot,
		"storagePool": storagePool,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	opts := d.GetVolumeOpts(ctx, cloneVolConfig, make(map[string]sa.Request))

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string

	// Ensure the LUN exists
	sourceLun, err := d.API.LunGetByName(ctx, cloneVolConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	} else if sourceLun == nil {
		return tridenterrors.NotFoundError("LUN %s not found", cloneVolConfig.CloneSourceVolumeInternal)
	}

	// Construct labels for the clone
	labels := ""
	var labelErr error
	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePoolTemp, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength); labelErr != nil {
			return labelErr
		}
	} else {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]

		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePool, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength); labelErr != nil {
			return labelErr
		}
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

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating LUN clone.")
	if err = cloneASAvol(ctx, cloneVolConfig, split, &d.Config, d.API); err != nil {
		return err
	}

	// Set labels if present
	if labels != "" {
		err = d.API.LunSetComment(ctx, name, labels)
		if err != nil {
			return fmt.Errorf("error setting labels on the LUN: %v", err)
		}
	}

	// Set QoS policy group if present
	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		err = d.API.LunSetQosPolicyGroup(ctx, name, qosPolicyGroup)
		if err != nil {
			return fmt.Errorf("error setting QoS policy group: %v", err)
		}
	}

	return nil
}

func (d *ASAStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "ASAStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
		"noRename":     volConfig.ImportNoRename,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Ensure the LUN exists
	lunInfo, err := d.API.LunGetByName(ctx, originalName)
	if err != nil {
		return err
	} else if lunInfo == nil {
		return tridenterrors.NotFoundError("LUN %s not found", originalName)
	}

	// Get flexvol corresponding to the LUN and ensure it is "rw" volume
	flexvol, err := d.API.VolumeInfo(ctx, originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return tridenterrors.NotFoundError("LUN %s not found", originalName)
	}

	if flexvol.AccessType != "rw" {
		Logc(ctx).WithField("originalName", originalName).Error("Could not import LUN, type is not rw.")
		return fmt.Errorf("LUN %s type is %s, not rw", originalName, flexvol.AccessType)
	}

	// The LUN should be online
	if lunInfo.State != "online" {
		return fmt.Errorf("LUN %s is not online", lunInfo.Name)
	}

	// Use the LUN size
	volConfig.Size = lunInfo.Size

	// Rename the LUN if Trident will manage its lifecycle and the names are different
	if !volConfig.ImportNotManaged && !volConfig.ImportNoRename {
		err = d.API.LunRename(ctx, originalName, volConfig.InternalName)
		if err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf(
				"Could not import LUN, rename LUN failed: %v", err)
			return fmt.Errorf("LUN %s rename failed: %v", originalName, err)
		}
	}

	// Update LUN labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, lunInfo.Comment) {
			// Set the base label
			storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

			// Make comment field from labels
			labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
				d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
			if labelErr != nil {
				return labelErr
			}

			err = d.API.LunSetComment(ctx, volConfig.InternalName, labels)
			if err != nil {
				Logc(ctx).WithField("originalName", originalName).Warnf("Modifying comment failed: %v", err)
				return fmt.Errorf("LUN %s modify failed: %v", originalName, err)
			}
		}

		err = LunUnmapAllIgroups(ctx, d.GetAPI(), volConfig.InternalName)
		if err != nil {
			Logc(ctx).WithField("LUN", lunInfo.Name).Warnf("Unmapping of igroups failed: %v", err)
			return fmt.Errorf("failed to unmap igroups for LUN %s: %v", lunInfo.Name, err)
		}
	}

	return nil
}

func (d *ASAStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "ASAStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	err := d.API.LunRename(ctx, name, newName)
	if err != nil {
		Logc(ctx).WithField("name", name).Warnf("Could not rename LUN: %v", err)
		return fmt.Errorf("could not rename LUN %s: %v", name, err)
	}

	return nil
}

// Destroy the requested LUN
func (d *ASAStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "ASAStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	// Validate LUN exists before trying to destroy
	volExists, err := d.API.LunExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing LUN: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("lun", name).Debug("LUN already deleted, skipping destroy.")
		return nil
	}

	defer func() {
		deleteAutomaticASASnapshot(ctx, &d.Config, d.API, volConfig)
	}()

	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Could not get target info.")
			return err
		}

		// Get the LUN ID
		lunPath := name
		lunID, err = d.API.LunMapInfo(ctx, d.Config.IgroupName, lunPath)
		if err != nil {
			return fmt.Errorf("error reading maps for LUN %s: %v", name, err)
		}
		if lunID >= 0 {
			publishInfo := models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: iSCSINodeName,
						IscsiLunNumber: int32(lunID),
					},
				},
			}
			drivers.RemoveSCSIDeviceByPublishInfo(ctx, &publishInfo, d.iscsi)
		}
	}

	// skipRecoveryQueue is not supported yet. Log it and continue.
	if skipRecoveryQueueValue, err := strconv.ParseBool(volConfig.SkipRecoveryQueue); err == nil {
		Logc(ctx).WithField("skipRecoveryQueue", skipRecoveryQueueValue).Warn(
			"SkipRecoveryQueue is not supported. It will be ignored when deleting the LUN.")
	}

	// Delete the LUN
	err = d.API.LunDestroy(ctx, name)
	if err != nil {
		return fmt.Errorf("error destroying LUN %v: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *ASAStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName
	var nodeName string
	fields := LogFields{
		"Method": "Publish",
		"Type":   "ASAStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Get flexvol corresponding to the LUN and ensure it is "rw"
	flexvol, err := d.API.VolumeInfo(ctx, name)
	if err != nil {
		return err
	} else if flexvol == nil {
		return tridenterrors.NotFoundError("LUN %s not found", name)
	}

	if flexvol.AccessType != "rw" {
		Logc(ctx).WithField("originalName", name).Error("Could not import LUN, type is not rw.")
		return fmt.Errorf("LUN %s type is %s, not rw", name, flexvol.AccessType)
	}

	lunPath := name
	igroupName := d.Config.IgroupName

	// Use the node specific igroup if publish enforcement is enabled and this is for CSI.
	if d.Config.SANType == sa.FCP {
		// Handle FCP protocol
		if tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
			igroupName = getNodeSpecificFCPIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
			err = ensureIGroupExists(ctx, d.GetAPI(), igroupName, d.Config.SANType)
			if err != nil {
				return err
			}
		}

		// Get FCP target info
		FCPNodeName, _, err := GetFCPTargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			return err
		}
		nodeName = FCPNodeName
	} else {
		// Handle iSCSI protocol
		if tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
			igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
			err = ensureIGroupExists(ctx, d.GetAPI(), igroupName, d.Config.SANType)
			if err != nil {
				return err
			}
		}

		// Get iSCSI target info
		iSCSINodeName, _, err := GetISCSITargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			return err
		}
		nodeName = iSCSINodeName
	}
	err = PublishLUN(ctx, d.API, &d.Config, d.ips, publishInfo, lunPath, igroupName, nodeName)
	if err != nil {
		return fmt.Errorf("error publishing %s driver: %v", d.Name(), err)
	}
	// Fill in the volume access fields as well.
	volConfig.AccessInfo = publishInfo.VolumeAccessInfo

	return nil
}

// Unpublish the volume from the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *ASAStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	var igroupName string
	fields := LogFields{
		"Method": "Unpublish",
		"Type":   "ASAStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	if tridentconfig.CurrentDriverContext != tridentconfig.ContextCSI {
		return nil
	}
	if d.Config.SANType == sa.FCP {
		igroupName = getNodeSpecificFCPIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		lunPath := name
		if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath); err != nil {
			return fmt.Errorf("error unmapping LUN %s from igroup %s; %v", lunPath, igroupName, err)
		}

		// Remove igroup from volume config's FCP access info
		volConfig.AccessInfo.FCPIgroup = removeIgroupFromFCPIgroupList(volConfig.AccessInfo.FCPIgroup,
			igroupName)
	} else {
		// Attempt to unmap the LUN from the per-node igroup.
		igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		lunPath := name
		if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath); err != nil {
			return fmt.Errorf("error unmapping LUN %s from igroup %s; %v", lunPath, igroupName, err)
		}

		// Remove igroup from volume config's iscsi access Info
		volConfig.AccessInfo.IscsiIgroup = removeIgroupFromIscsiIgroupList(volConfig.AccessInfo.IscsiIgroup, igroupName)
	}
	// Remove igroup if no LUNs are mapped.
	if err := DestroyUnmappedIgroup(ctx, d.API, igroupName); err != nil {
		return fmt.Errorf("error removing empty igroup; %v", err)
	}

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *ASAStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *ASAStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "ASAStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getASASnapshot(ctx, snapConfig, &d.Config, d.API)
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *ASAStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "ASAStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getASASnapshotList(ctx, volConfig, &d.Config, d.API)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *ASAStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "ASAStorageDriver",
		"snapshotName": internalSnapName,
		"sourceVolume": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	snapshot, err := createASASnapshot(ctx, snapConfig, &d.Config, d.API)

	return snapshot, err
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *ASAStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "ASAStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return restoreASASnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *ASAStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "ASAStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	err := deleteASASnapshot(ctx, snapConfig, &d.Config, d.API, d.cloneSplitTimers)
	if err != nil {
		Logc(ctx).WithField("snapshotName", snapConfig.InternalName).WithError(err).Error("Error deleting snapshot.")
		return err
	}

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// Get tests for the existence of a volume
func (d *ASAStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "ASAStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	exists, err := d.API.LunExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing LUN: %v", err)
	}
	if !exists {
		Logc(ctx).WithField("LUN", name).Debug("LUN not found.")
		return tridenterrors.NotFoundError("LUN %s does not exist", name)
	}

	return nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *ASAStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *ASAStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *ASAStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "ASAStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. SVM UUID
	// ASA backends will only report 1 storage pool.
	svmUUID := d.GetAPI().GetSVMUUID()
	return []drivers.OntapStorageBackendPool{{SvmUUID: svmUUID}}
}

func (d *ASAStorageDriver) getStoragePoolAttributes(_ context.Context) map[string]sa.Offer {
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(false),
		sa.ProvisioningType: sa.NewStringOffer("thin"),
		sa.Media:            sa.NewStringOffer(sa.SSD),
	}
}

func (d *ASAStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *ASAStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

func (d *ASAStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
	if !volConfig.ImportNotManaged && tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		// All new CSI ONTAP SAN volumes start with publish enforcement on, unless they're unmanaged imports
		volConfig.AccessInfo.PublishEnforcement = true
	}

	// If no pool is specified, a new pool is created and assigned a name template and label from the common configuration.
	// The process of generating a custom volume name necessitates a name template and label.
	if storage.IsStoragePoolUnset(pool) {
		pool = ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)
	}
	createPrepareCommon(ctx, d, volConfig, pool)
}

func (d *ASAStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"Method":       "CreateFollowup",
		"Type":         "ASAStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")
	Logc(ctx).Debug("No follow-up create actions for All-SAN Array LUN.")

	return nil
}

func (d *ASAStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *ASAStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *ASAStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name of the
// LUN (which also is name of the container Flexvol) on the storage system.
func (d *ASAStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	volumeAttrs, err := d.API.VolumeInfo(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	lunPath := volumeID
	lunAttrs, err := d.API.LunGetByName(ctx, lunPath)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(lunAttrs, volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *ASAStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	// Let the caller know we're done by closing the channel
	defer close(channel)

	// TODO (aparna0508): Replace volume API with Consistency Group API when available to fetch Snapshot Policy
	// Get all volumes matching the storage prefix
	volumes, err := d.API.VolumeListByPrefix(ctx, *d.Config.StoragePrefix)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all LUNs named 'lun0' in volumes matching the storage prefix
	lunPathPattern := *d.Config.StoragePrefix + "*"
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with LUNs
	volumeMap := make(map[string]api.Volume)
	if volumes != nil {
		for _, volumeAttrs := range volumes {
			internalName := volumeAttrs.Name
			volumeMap[internalName] = *volumeAttrs
		}
	}

	// Convert all LUNs to VolumeExternal and write them to the channel
	if luns != nil {
		for idx := range luns {
			lun := &luns[idx]
			volume, ok := volumeMap[lun.VolumeName]
			if !ok {
				Logc(ctx).WithField("path", lun.Name).Warning("Flexvol not found for LUN.")
				continue
			}

			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(lun, &volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *ASAStorageDriver) getVolumeExternal(
	lun *api.Lun, volume *api.Volume,
) *storage.VolumeExternal {
	internalName := volume.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            lun.Size,
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volume.SnapshotPolicy,
		ExportPolicy:    "",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      models.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	pool := drivers.UnsetPool
	if len(volume.Aggregates) > 0 {
		pool = volume.Aggregates[0]
	}
	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   pool,
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *ASAStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*ASAStorageDriver)
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

// Resize expands the LUN size.
func (d *ASAStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":             "Resize",
		"Type":               "ASAStorageDriver",
		"name":               name,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Validation checks
	volExists, err := d.API.LunExists(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"name":  name,
		}).Error("Error checking for existing LUN.")
		return errors.New("error occurred checking for existing LUN")
	}
	if !volExists {
		return tridenterrors.NotFoundError("LUN %s does not exist", name)
	}

	// Check if the requested size falls within limitVolumeSize config
	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Get current size
	currentLunSize, err := d.API.LunSize(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"name":  name,
		}).Error("Error checking LUN size.")
		return fmt.Errorf("error occurred when checking LUN size; %v", err)
	}

	lunSizeBytes := uint64(currentLunSize)
	if requestedSizeBytes < lunSizeBytes {
		return fmt.Errorf("requested size %d is less than existing LUN size %d", requestedSizeBytes, lunSizeBytes)
	}

	// Resize LUN
	returnSize, err := d.API.LunSetSize(ctx, name, strconv.FormatUint(requestedSizeBytes, 10))
	if err != nil {
		Logc(ctx).WithField("error", err).Error("LUN resize failed.")
		return fmt.Errorf("LUN resize failed; %v", err)
	}

	volConfig.Size = strconv.FormatUint(returnSize, 10)

	return nil
}

func (d *ASAStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*models.Node,
	backendUUID, tridentUUID string,
) error {
	nodeNames := make([]string, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "ASAStorageDriver",
		"Nodes":  nodeNames,
	}

	Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return reconcileSANNodeAccess(ctx, d.API, nodeNames, backendUUID, tridentUUID)
}

func (d *ASAStorageDriver) ReconcileVolumeNodeAccess(ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "ASAStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *ASAStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, d.Config.SANType, d.GetStorageBackendPhysicalPoolNames(ctx))
}

// String makes ASAStorageDriver satisfy the Stringer interface.
func (d ASAStorageDriver) String() string {
	return convert.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes ASAStorageDriver satisfy the GoStringer interface.
func (d *ASAStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d ASAStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func (d *ASAStorageDriver) GetChapInfo(_ context.Context, _, _ string) (*models.IscsiChapInfo, error) {
	return &models.IscsiChapInfo{
		UseCHAP:              d.Config.UseCHAP,
		IscsiUsername:        d.Config.ChapUsername,
		IscsiInitiatorSecret: d.Config.ChapInitiatorSecret,
		IscsiTargetUsername:  d.Config.ChapTargetUsername,
		IscsiTargetSecret:    d.Config.ChapTargetInitiatorSecret,
	}, nil
}

// EnablePublishEnforcement prepares a volume for per-node igroup mapping allowing greater access control.
func (d *ASAStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	lunPath := volume.Config.InternalName
	return EnableSANPublishEnforcement(ctx, d.GetAPI(), volume.Config, lunPath)
}

func (d *ASAStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}

func (d *ASAStorageDriver) HealVolumePublishEnforcement(ctx context.Context, volume *storage.Volume) bool {
	return HealSANPublishEnforcement(ctx, d, volume)
}

// CreateASALUNInternalID creates a string in the format /svm/<svm_name>/lun/<lun_name>
func (d *ASAStorageDriver) CreateASALUNInternalID(svm, name string) string {
	return fmt.Sprintf("/svm/%s/lun/%s", svm, name)
}
