// Copyright 2024 NetApp, Inc. All Rights Reserved.

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
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

func lunPath(name string) string {
	return fmt.Sprintf("/vol/%v/lun0", name)
}

// SANStorageDriver is for iSCSI storage provisioning
type SANStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	API         api.OntapAPI
	AWSAPI      awsapi.AWSAPI
	telemetry   *Telemetry
	iscsi       iscsi.ISCSI

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers map[string]time.Time
}

func (d *SANStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

func (d *SANStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *SANStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *SANStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d SANStorageDriver) Name() string {
	return tridentconfig.OntapSANStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *SANStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		lif0 := "noLIFs"
		if len(d.ips) > 0 {
			lif0 = d.ips[0]
		}
		return CleanBackendName("ontapsan_" + lif0)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *SANStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "SANStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	// Initialize the iSCSI client
	d.iscsi = iscsi.New(utils.NewOSClient(), utils.NewDevicesClient(), utils.NewFilesystemClient(),
		utils.NewMountClient())

	// Initialize the driver's CommonStorageDriverConfig
	d.Config.CommonStorageDriverConfig = commonConfig

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

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
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}
	}
	d.Config = *config

	d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, "iscsi")
	if err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no iSCSI data LIFs found on SVM %s", d.API.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
	}

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = InitializeSANDriver(ctx, driverContext, d.API, &d.Config, d.validate, backendUUID)
	if err != nil {
		if d.Config.DriverContext == tridentconfig.ContextCSI {
			// Clean up igroup for failed driver.
			err := d.API.IgroupDestroy(ctx, d.Config.IgroupName)
			if err != nil {
				Logc(ctx).WithError(err).WithField("igroup", d.Config.IgroupName).Warn("Error deleting igroup.")
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
	d.cloneSplitTimers = make(map[string]time.Time)

	d.initialized = true
	return nil
}

func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "SANStorageDriver"}
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
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *SANStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := validateReplicationConfig(ctx, d.Config.ReplicationPolicy, d.Config.ReplicationSchedule,
		d.API); err != nil {
		return fmt.Errorf("replication validation failed: %v", err)
	}

	if err := ValidateSANDriver(ctx, &d.Config, d.ips); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d,
		api.MaxSANLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// destroyVolumeIfNoLUN attempts to destroy volume if there exists a volume with no associated LUN.
// This is used to make Create() idempotent by cleaning up a Flexvol with no LUN.
// Returns (Volume State, error)
//
//	Caller can check for:
//	non-nil error, indicates one of the following.
//	   - Error checking volume existence.
//	   - Error checking LUN existence.
//	   - Could not destroy the required volume for an error.
//	Volume state:true indicating both volume and required LUN exist.
//	Volume state:false indicating no volume existed or cleaned up now.
func (d *SANStorageDriver) destroyVolumeIfNoLUN(ctx context.Context, volConfig *storage.VolumeConfig) (bool, error) {
	name := volConfig.InternalName
	fields := LogFields{
		"Method": "destroyVolumeIfNoLUN",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> destroyVolumeIfNoLUN")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< destroyVolumeIfNoLUN")

	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return false, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		// No volume exists, no clean up.
		return false, nil
	}
	if volConfig.IsMirrorDestination {
		// No clean up required, this is DP volume.
		return true, nil
	}

	// Verify if LUN exists.
	newLUNPath := lunPath(name)
	extantLUN, err := d.API.LunGetByName(ctx, newLUNPath)
	if extantLUN != nil {
		// Volume and LUN both exist. No clean up needed.
		return true, nil
	}
	if !errors.IsNotFoundError(err) {
		// Could not verify if LUN exists. Clean up pending.
		return false, fmt.Errorf("error checking for existing LUN %s: %v", newLUNPath, err)
	}
	// LUN does not exist, but volume. Initiate clean-up.
	if err = d.API.VolumeDestroy(ctx, name, true); err != nil {
		Logc(ctx).WithField("volume", name).Errorf("Could not clean up volume: %v", err)
		return true, fmt.Errorf("could not clean up partial create of vol/lun: %v", err)
	}
	Logc(ctx).WithField("volume", name).Debug("Cleaned up volume since LUN create failed.")
	return false, nil
}

// Create a volume+LUN with the specified options
func (d *SANStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	var fstype string

	fields := LogFields{
		"Method": "Create",
		"Type":   "SANStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Early exit if volume+LUN exist. Clean up volume if no LUN exists.
	volExists, err := d.destroyVolumeIfNoLUN(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("failure checking for existence of volume and cleaning if any: %v", err)
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

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults

	spaceAllocation, _ := strconv.ParseBool(
		utils.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation]))
	var (
		spaceReserve      = utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy      = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = utils.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		formatOptions     = utils.GetV(opts, "formatOptions", storagePool.InternalAttributes()[FormatOptions])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
		luksEncryption    = storagePool.InternalAttributes()[LUKSEncryption]
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
	requestedSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	lunSizeBytes := GetVolumeSize(requestedSizeBytes, storagePool.InternalAttributes()[Size])

	// Add a constant overhead for LUKS volumes to account for LUKS metadata. This overhead is
	// part of the LUN but is not reported to the orchestrator.
	reportedSize := lunSizeBytes
	lunSizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, lunSizeBytes, luksEncryption)

	lunSize := strconv.FormatUint(lunSizeBytes, 10)
	// Get the flexvol size based on the snapshot reserve
	flexvolSize := drivers.CalculateVolumeSizeBytes(ctx, name, lunSizeBytes, snapshotReserveInt)
	// Add extra 10% to the Flexvol to account for LUN metadata
	flexvolBufferSize := uint64(LUNMetadataBufferMultiplier * float64(flexvolSize))

	volumeSize := strconv.FormatUint(flexvolBufferSize, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, lunSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	fstype, err = drivers.CheckSupportedFilesystem(
		ctx, utils.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name)
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

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(reportedSize, 10)
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.UnixPermissions = unixPermissions
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = configEncryption
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy
	volConfig.LUKSEncryption = luksEncryption
	volConfig.FileSystem = fstype
	volConfig.FormatOptions = formatOptions

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"lunSize":           lunSize,
		"flexvolSize":       flexvolBufferSize,
		"spaceAllocation":   spaceAllocation,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"LUKSEncryption":    luksEncryption,
		"encryption":        utils.GetPrintableBoolPtrValue(enableEncryption),
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
		"formatOptions":     formatOptions,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, flexvolBufferSize, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error: %v", storagePool.Name(), aggregate, aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Move on to the next pool
			continue
		}

		// Make comment field from labels
		labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
		if labelErr != nil {
			return labelErr
		}
		// Create the volume
		err = d.API.VolumeCreate(
			ctx, api.Volume{
				AccessType: "",
				Aggregates: []string{
					aggregate,
				},
				Comment:         labels,
				Encrypt:         enableEncryption,
				ExportPolicy:    exportPolicy,
				JunctionPath:    "",
				Name:            name,
				Qos:             api.QosPolicyGroup{},
				SecurityStyle:   securityStyle,
				Size:            volumeSize,
				SnapshotPolicy:  snapshotPolicy,
				SnapshotReserve: snapshotReserveInt,
				SpaceReserve:    spaceReserve,
				TieringPolicy:   tieringPolicy,
				UnixPermissions: unixPermissions,
				UUID:            "",
				DPVolume:        volConfig.IsMirrorDestination,
			})
		if err != nil {
			if !api.IsVolumeCreateJobExistsError(err) {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN pool %s/%s; error creating volume %s: %v", storagePool.Name(),
					aggregate, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, fmt.Errorf(errMessage))

				// Move on to the next pool
				continue
			}
			// Log a message. Proceed to create LUN, hoping volume would have been created by the time we send
			// LUN create request.
			Logc(ctx).WithField("volume", name).Debug("Volume create is already in progress.")
		}

		// If a DP volume, do not create the LUN, it will be copied over by snapmirror
		if !volConfig.IsMirrorDestination {
			lunPath := lunPath(name)
			osType := "linux"
			// Create the LUN.  If this fails, clean up and move on to the next pool.
			// QoS policy is set at the LUN layer
			err = d.API.LunCreate(
				ctx, api.Lun{
					Name:           lunPath,
					Qos:            qosPolicyGroup,
					Size:           lunSize,
					OsType:         osType,
					SpaceReserved:  utils.Ptr(false),
					SpaceAllocated: utils.Ptr(spaceAllocation),
				})
			if err != nil {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN pool %s/%s; error creating LUN %s: %v", storagePool.Name(),
					aggregate, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, fmt.Errorf(errMessage))

				// Don't leave the new Flexvol around.
				// If VolumeDestroy() fails for any reason, volume must be manually deleted.
				if err := d.API.VolumeDestroy(ctx, name, true); err != nil {
					Logc(ctx).WithField("volume", name).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after LUN create error.")
				}

				// Move on to the next pool
				continue
			}

			// Save the fstype in a LUN attribute so we know what to do in Attach.  If this fails, clean up and
			// move on to the next pool.
			// Save the context, fstype, and LUKS value in LUN comment
			err = d.API.LunSetAttribute(ctx, lunPath, LUNAttributeFSType, fstype, string(d.Config.DriverContext),
				luksEncryption, formatOptions)
			if err != nil {

				errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error saving file system type for LUN %s: %v",
					storagePool.Name(), aggregate, name, err)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, fmt.Errorf(errMessage))

				// Don't leave the new LUN around.
				// If the following LunDestroy() fails for any reason, LUN and volume must be manually deleted.
				if err := d.API.LunDestroy(ctx, lunPath); err != nil {
					Logc(ctx).WithField("LUN", lunPath).Errorf("Could not clean up LUN; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up LUN after set attribute error.")
				}

				// Don't leave the new Flexvol around.
				// If the following VolumeDestroy() fails for any reason, volume must be manually deleted.
				if err := d.API.VolumeDestroy(ctx, name, true); err != nil {
					Logc(ctx).WithField("volume", name).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after set attribute error.")
				}

				// Move on to the next pool
				continue
			}
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *SANStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":      "CreateClone",
		"Type":        "SANStorageDriver",
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

	// Ensure the volume exists
	sourceFlexvol, err := d.API.VolumeInfo(ctx, cloneVolConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	} else if sourceFlexvol == nil {
		return fmt.Errorf("volume %s not found", cloneVolConfig.CloneSourceVolumeInternal)
	}

	// Get the source volume's label
	labels := ""
	if sourceFlexvol.Comment != "" {
		labels = sourceFlexvol.Comment
	}

	var labelErr error
	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePoolTemp, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength); labelErr != nil {
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
	if err = cloneFlexvol(
		ctx, cloneVolConfig, labels, split, &d.Config, d.API, api.QosPolicyGroup{},
	); err != nil {
		return err
	}

	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		err = d.API.LunSetQosPolicyGroup(ctx, lunPath(name), qosPolicyGroup)
		if err != nil {
			return fmt.Errorf("error setting QoS policy group: %v", err)
		}
	}

	return nil
}

func (d *SANStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "SANStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Ensure the volume exists
	flexvol, err := d.API.VolumeInfo(ctx, originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", originalName)
	}

	// Validate the volume is what it should be
	if !api.IsVolumeIdAttributesReadError(err) {
		if flexvol.AccessType != "" && flexvol.AccessType != "rw" {
			Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("volume %s type is %s, not rw", originalName, flexvol.AccessType)
		}
	}

	// Set the volume to LUKS if backend has LUKS true as default
	if volConfig.LUKSEncryption == "" {
		volConfig.LUKSEncryption = d.Config.LUKSEncryption
	}

	// Ensure the volume has only one LUN
	lunInfo, err := d.API.LunGetByName(ctx, "/vol/"+originalName+"/*")
	if err != nil {
		return err
	} else if lunInfo == nil {
		return fmt.Errorf("lun not found in volume %s", originalName)
	}
	targetPath := "/vol/" + originalName + "/lun0"

	// The LUN should be online
	if lunInfo.State != "online" {
		return fmt.Errorf("LUN %s is not online", lunInfo.Name)
	}

	// Use the LUN size
	volConfig.Size = lunInfo.Size

	if utils.ParseBool(volConfig.LUKSEncryption) {
		newSize, err := subtractUintFromSizeString(volConfig.Size, utils.LUKSMetadataSize)
		if err != nil {
			return err
		}
		volConfig.Size = newSize
	}

	// Rename the volume or LUN if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if lunInfo.Name != targetPath {
			err = d.API.LunRename(ctx, lunInfo.Name, targetPath)
			if err != nil {
				Logc(ctx).WithField("path", lunInfo.Name).Errorf("Could not import volume, rename LUN failed: %v", err)
				return fmt.Errorf("LUN path %s rename failed: %v", lunInfo.Name, err)
			}
		}

		err = d.API.VolumeRename(ctx, originalName, volConfig.InternalName)
		if err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf(
				"Could not import volume, rename volume failed: %v", err)
			return fmt.Errorf("volume %s rename failed: %v", originalName, err)
		}
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexvol.Comment) {
			// Set the base label
			storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

			// Make comment field from labels
			labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
				d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
			if labelErr != nil {
				return labelErr
			}

			err = d.API.VolumeSetComment(ctx, volConfig.InternalName, originalName, labels)
			if err != nil {
				Logc(ctx).WithField("originalName", originalName).Warnf("Modifying comment failed: %v", err)
				return fmt.Errorf("volume %s modify failed: %v", originalName, err)
			}
		}
		err = LunUnmapAllIgroups(ctx, d.GetAPI(), lunPath(volConfig.InternalName))
		if err != nil {
			Logc(ctx).WithField("LUN", lunInfo.Name).Warnf("Unmapping of igroups failed: %v", err)
			return fmt.Errorf("failed to unmap igroups for LUN %s: %v", lunInfo.Name, err)
		}
	} else {
		// Volume import is not managed by Trident
		if lunInfo.Name != targetPath {
			return fmt.Errorf("could not import volume, LUN is nammed incorrectly: %s", lunInfo.Name)
		}
	}

	return nil
}

func (d *SANStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "SANStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	err := d.API.VolumeRename(ctx, name, newName)
	if err != nil {
		Logc(ctx).WithField("name", name).Warnf("Could not rename volume: %v", err)
		return fmt.Errorf("could not rename volume %s: %v", name, err)
	}

	return nil
}

// Destroy the requested (volume,lun) storage tuple
func (d *SANStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	// Validate Flexvol exists before trying to destroy
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("volume", name).Debug("Volume already deleted, skipping destroy.")
		return nil
	}

	defer func() {
		deleteAutomaticSnapshot(ctx, d, err, volConfig, d.API.VolumeSnapshotDelete)
	}()

	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Could not get target info.")
			return err
		}

		// Get the LUN ID
		lunPath := lunPath(name)
		lunID, err = d.API.LunMapInfo(ctx, d.Config.IgroupName, lunPath)
		if err != nil {
			return fmt.Errorf("error reading LUN maps for volume %s: %v", name, err)
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
			drivers.RemoveSCSIDeviceByPublishInfo(ctx, &publishInfo)
		}
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

	// Delete the Flexvol & LUN
	err = d.API.VolumeDestroy(ctx, name, true)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Publish",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Check if the volume is DP or RW and don't publish if DP
	volIsRW, err := isFlexvolRW(ctx, d.GetAPI(), name)
	if err != nil {
		return err
	}
	if !volIsRW {
		return fmt.Errorf("volume is not read-write")
	}

	lunPath := lunPath(name)
	igroupName := d.Config.IgroupName

	// Use the node specific igroup if publish enforcement is enabled and this is for CSI.
	if tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		err = ensureIGroupExists(ctx, d.GetAPI(), igroupName)
	}

	// Get target info.
	iSCSINodeName, _, err := GetISCSITargetInfo(ctx, d.API, &d.Config)
	if err != nil {
		return err
	}

	err = PublishLUN(ctx, d.API, &d.Config, d.ips, publishInfo, lunPath, igroupName, iSCSINodeName)
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
func (d *SANStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Unpublish",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	if tridentconfig.CurrentDriverContext != tridentconfig.ContextCSI {
		return nil
	}

	// Attempt to unmap the LUN from the per-node igroup.
	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := lunPath(name)
	if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath); err != nil {
		return fmt.Errorf("error unmapping LUN %s from igroup %s; %v", lunPath, igroupName, err)
	}

	// Remove igroup from volume config's iscsi access Info
	volConfig.AccessInfo.IscsiIgroup = removeIgroupFromIscsiIgroupList(volConfig.AccessInfo.IscsiIgroup, igroupName)

	// Remove igroup if no LUNs are mapped.
	if err := DestroyUnmappedIgroup(ctx, d.API, igroupName); err != nil {
		return fmt.Errorf("error removing empty igroup; %v", err)
	}

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *SANStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *SANStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getVolumeSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.LunSize)
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *SANStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "SANStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getVolumeSnapshotList(ctx, volConfig, &d.Config, d.API, d.API.LunSize)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *SANStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": internalSnapName,
		"sourceVolume": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	return createFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.LunSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *SANStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	err := d.API.VolumeSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName)
	if err != nil {
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
func (d *SANStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("Flexvol", name).Debug("Flexvol not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *SANStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *SANStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *SANStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "SANStorageDriver"}
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

func (d *SANStorageDriver) getStoragePoolAttributes(ctx context.Context) map[string]sa.Offer {
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

func (d *SANStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *SANStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

func (d *SANStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
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

func (d *SANStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"Method":       "CreateFollowup",
		"Type":         "SANStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")
	Logc(ctx).Debug("No follow-up create actions for ontap-san volume.")

	return nil
}

func (d *SANStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *SANStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name of the
// Flexvol on the storage system.
func (d *SANStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	volumeAttrs, err := d.API.VolumeInfo(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	lunPath := fmt.Sprintf("/vol/%v/*", volumeID)
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
func (d *SANStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumes, err := d.API.VolumeListByPrefix(ctx, *d.Config.StoragePrefix)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all LUNs named 'lun0' in volumes matching the storage prefix
	lunPathPattern := lunPath(*d.Config.StoragePrefix + "*")
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
func (d *SANStorageDriver) getVolumeExternal(
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
func (d *SANStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANStorageDriver)
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
func (d *SANStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":             "Resize",
		"Type":               "SANStorageDriver",
		"name":               name,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Validation checks
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"name":  name,
		}).Error("Error checking for existing volume.")
		return fmt.Errorf("error occurred checking for existing volume")
	}
	if !volExists {
		return fmt.Errorf("volume %s does not exist", name)
	}

	currentFlexvolSize, err := d.API.VolumeSize(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"name":  name,
		}).Error("Error checking volume size.")
		return fmt.Errorf("error occurred when checking volume size")
	}

	currentLunSize, err := d.API.LunSize(ctx, name)
	if err != nil {
		return fmt.Errorf("error occurred when checking lun size")
	}

	lunSizeBytes := uint64(currentLunSize)
	if requestedSizeBytes < lunSizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", requestedSizeBytes, lunSizeBytes)
	}

	// Add a constant overhead for LUKS volumes to account for LUKS metadata.
	requestedSizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, requestedSizeBytes, volConfig.LUKSEncryption)

	snapshotReserveInt, err := getSnapshotReserveFromOntap(ctx, name, d.API.VolumeInfo)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume")
	}

	newFlexvolSize := drivers.CalculateVolumeSizeBytes(ctx, name, requestedSizeBytes, snapshotReserveInt)

	newFlexvolSize = uint64(LUNMetadataBufferMultiplier * float64(newFlexvolSize))

	sameLUNSize := utils.VolumeSizeWithinTolerance(int64(requestedSizeBytes), int64(currentLunSize),
		tridentconfig.SANResizeDelta)

	sameFlexvolSize := utils.VolumeSizeWithinTolerance(int64(newFlexvolSize), int64(currentFlexvolSize),
		tridentconfig.SANResizeDelta)

	if sameLUNSize && sameFlexvolSize {
		Logc(ctx).WithFields(LogFields{
			"requestedSize":  requestedSizeBytes,
			"currentLUNSize": currentLunSize,
			"name":           name,
			"delta":          tridentconfig.SANResizeDelta,
		}).Info("Requested size and current LUN size are within the delta and therefore considered the same size" +
			" for SAN resize operations.")
		volConfig.Size = strconv.FormatUint(uint64(currentLunSize), 10)
		return nil
	}

	if !sameFlexvolSize && (currentFlexvolSize > newFlexvolSize) {
		Logc(ctx).WithFields(LogFields{
			"requestedSize":      requestedSizeBytes,
			"currentFlexvolSize": currentFlexvolSize,
			"name":               name,
		}).Info("Requested size is less than current FlexVol size. FlexVol will not be resized.")
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

	// Resize FlexVol
	if !sameFlexvolSize && (currentFlexvolSize < newFlexvolSize) {
		err := d.API.VolumeSetSize(ctx, name, strconv.FormatUint(newFlexvolSize, 10))
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Volume resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}

	// Resize LUN0
	returnSize := lunSizeBytes
	if !sameLUNSize {
		returnSize, err = d.API.LunSetSize(ctx, lunPath(name), strconv.FormatUint(requestedSizeBytes, 10))
		if err != nil {
			Logc(ctx).WithField("error", err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}

	// LUKS metadata size is not reported so remove it from LUN size
	returnSize = decrementWithLUKSMetadataIfLUKSEnabled(ctx, returnSize, volConfig.LUKSEncryption)

	volConfig.Size = strconv.FormatUint(returnSize, 10)
	return nil
}

func (d *SANStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*models.Node, backendUUID, tridentUUID string,
) error {
	nodeNames := make([]string, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "SANStorageDriver",
		"Nodes":  nodeNames,
	}

	Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return reconcileSANNodeAccess(ctx, d.API, nodeNames, backendUUID, tridentUUID)
}

func (d *SANStorageDriver) ReconcileVolumeNodeAccess(ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *SANStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "iscsi", d.GetStorageBackendPhysicalPoolNames(ctx), d.Config.Aggregate)
}

// String makes SANStorageDriver satisfy the Stringer interface.
func (d SANStorageDriver) String() string {
	return utils.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes SANStorageDriver satisfy the GoStringer interface.
func (d *SANStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d SANStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// EstablishMirror will create a new mirror relationship between a RW and a DP volume that have not previously
// had a relationship
func (d *SANStorageDriver) EstablishMirror(
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
func (d *SANStorageDriver) ReestablishMirror(
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
func (d *SANStorageDriver) PromoteMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotName string,
) (bool, error) {
	return promoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotName,
		d.Config.ReplicationPolicy, d.API)
}

// GetMirrorStatus returns the current state of a mirror relationship
func (d *SANStorageDriver) GetMirrorStatus(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, error) {
	return getMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// ReleaseMirror will release the mirror relationship data of the source volume
func (d *SANStorageDriver) ReleaseMirror(ctx context.Context, localInternalVolumeName string) error {
	return releaseMirror(ctx, localInternalVolumeName, d.API)
}

// GetReplicationDetails returns the replication policy and schedule of a mirror relationship
func (d *SANStorageDriver) GetReplicationDetails(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, string, string, error) {
	return getReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// UpdateMirror will attempt a mirror update for the given destination volume
func (d *SANStorageDriver) UpdateMirror(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	return mirrorUpdate(ctx, localInternalVolumeName, snapshotName, d.API)
}

// CheckMirrorTransferState will look at the transfer state of the mirror relationship to determine if it is failed,
// succeeded or in progress
func (d *SANStorageDriver) CheckMirrorTransferState(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return checkMirrorTransferState(ctx, localInternalVolumeName, d.API)
}

// GetMirrorTransferTime will return the transfer time of the mirror relationship
func (d *SANStorageDriver) GetMirrorTransferTime(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return getMirrorTransferTime(ctx, localInternalVolumeName, d.API)
}

func (d *SANStorageDriver) GetChapInfo(_ context.Context, _, _ string) (*models.IscsiChapInfo, error) {
	return &models.IscsiChapInfo{
		UseCHAP:              d.Config.UseCHAP,
		IscsiUsername:        d.Config.ChapUsername,
		IscsiInitiatorSecret: d.Config.ChapInitiatorSecret,
		IscsiTargetUsername:  d.Config.ChapTargetUsername,
		IscsiTargetSecret:    d.Config.ChapTargetInitiatorSecret,
	}, nil
}

// EnablePublishEnforcement prepares a volume for per-node igroup mapping allowing greater access control.
func (d *SANStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	return EnableSANPublishEnforcement(ctx, d.GetAPI(), volume.Config, lunPath(volume.Config.InternalName))
}

func (d *SANStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}
