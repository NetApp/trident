// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
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
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

// NVMeNamespaceRegExp RegExp to match the namespace path either empty string or
// string of the form /vol/<flexVolName>/<Namespacename>
var NVMeNamespaceRegExp = regexp.MustCompile(`[^(\/vol\/.+\/.+)?$]`)

// NVMeStorageDriver is for NVMe storage provisioning.
type NVMeStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	API         api.OntapAPI
	AWSAPI      awsapi.AWSAPI
	telemetry   *Telemetry

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers *sync.Map
}

const (
	// defaultNamespaceBlockSize represents the default block size used to create a namespace.
	defaultNamespaceBlockSize = 4096
	// maximumSubsystemNameLength represent the max length of subsystem name
	maximumSubsystemNameLength = 64
	// nvmeSubsystemPrefix Subsystem prefix for ONTAP NVMe driver (empty for legacy compatibility).
	nvmeSubsystemPrefix = ""
)

// Namespace attributes stored in its comment field. These fields are useful for docker context.
const (
	nsMaxCommentLength   = 254
	nsAttribute          = "nsAttribute"
	nsAttributeFSType    = "com.netapp.ndvp.fstype"
	nsAttributeLUKS      = "LUKS"
	nsAttributeDriverCtx = "driverContext"
	nsLabels             = "nsLabels"
)

// GetConfig is to get the driver's configuration.
func (d *NVMeStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

func (d *NVMeStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

// GetAPI returns the ONTAP API interface.
func (d *NVMeStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

// GetTelemetry returns the telemetry details of this driver.
func (d *NVMeStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver.
func (d *NVMeStorageDriver) Name() string {
	return tridentconfig.OntapSANStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance.
func (d *NVMeStorageDriver) BackendName() string {
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

// Initialize is to validate and configure the driver using the provided config.
func (d *NVMeStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"method": "Initialize", "type": "NVMeStorageDriver"}
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
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
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
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}
	}

	// Load default config parameters
	if err = PopulateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	// Check NVMe feature support
	if !d.API.SupportsFeature(ctx, api.NVMeProtocol) {
		return fmt.Errorf("error initializing %s driver: ONTAP doesn't support NVMe", d.Name())
	}

	if d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport); err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no NVMe data LIFs found on SVM %s", d.API.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found LIFs.")
	}

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	if err = d.validate(ctx); err != nil {
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

// Initialized returns the state of the driver.
func (d *NVMeStorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver processes and updates the driver state to uninitialized.
func (d *NVMeStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"method": "Terminate", "type": "NVMeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	if d.telemetry != nil {
		d.telemetry.Stop()
	}

	d.API.Terminate()
	d.initialized = false
}

// Validate the driver configuration and execution environment.
func (d *NVMeStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"method": "validate", "type": "NVMeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := ValidateSANDriver(ctx, &d.Config, d.ips, nil); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	err := validateReplicationConfig(ctx, d.Config.ReplicationPolicy, d.Config.ReplicationSchedule, d.API)
	if err != nil {
		return fmt.Errorf("replication validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	err = ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d, api.MaxSANLabelLength)
	if err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a Volume+Namespace with the specified options.
func (d *NVMeStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"method": "Create",
		"type":   "NVMeStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// If the volume already exists, bail out.
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return drivers.NewVolumeExistsError(name)
	}

	// If volume shall be mirrored, check that the SVM is peered with the other side.
	if volConfig.PeerVolumeHandle != "" {
		if err = checkSVMPeered(ctx, volConfig, d.API.SVMName(), d.API); err != nil {
			return err
		}
	}

	// Get candidate physical pools.
	physicalPools, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
	if err != nil {
		return err
	}

	// Get options.
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults

	spaceAllocation, _ := strconv.ParseBool(
		collection.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation]))
	var (
		spaceReserve      = collection.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = collection.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = collection.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = collection.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		snapshotDir       = "false"
		exportPolicy      = collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = collection.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = collection.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = collection.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		skipRecoveryQueue = collection.GetV(opts, "skipRecoveryQueue", storagePool.InternalAttributes()[SkipRecoveryQueue])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
		luksEncryption    = storagePool.InternalAttributes()[LUKSEncryption]
	)

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	// Determine volume size in bytes.
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}

	requestedSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	namespaceSizeBytes := GetVolumeSize(requestedSizeBytes, storagePool.InternalAttributes()[Size])

	// Add a constant overhead for LUKS volumes to account for LUKS metadata. This overhead is
	// part of the LUN but is not reported to the orchestrator.
	reportedSize := namespaceSizeBytes
	namespaceSizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, namespaceSizeBytes, luksEncryption)
	namespaceSize := strconv.FormatUint(namespaceSizeBytes, 10)

	// Get the FlexVol size based on the snapshot reserve.
	flexVolSize := drivers.CalculateVolumeSizeBytes(ctx, name, namespaceSizeBytes, snapshotReserveInt)
	// Add extra 10% to the FlexVol to account for Namespace metadata.
	flexVolBufferSize := uint64(LUNMetadataBufferMultiplier * float64(flexVolSize))

	volumeSize := strconv.FormatUint(flexVolBufferSize, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, namespaceSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	fstype, err := drivers.CheckSupportedFilesystem(
		ctx, collection.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name)
	if err != nil {
		return err
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if _, err = strconv.ParseBool(skipRecoveryQueue); skipRecoveryQueue != "" && err != nil {
		return fmt.Errorf("invalid boolean value for skipRecoveryQueue: %v", err)
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	// Update config to reflect values used to create volume.
	volConfig.Size = strconv.FormatUint(reportedSize, 10)
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.UnixPermissions = unixPermissions
	volConfig.SnapshotDir = snapshotDir
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = configEncryption
	volConfig.SkipRecoveryQueue = skipRecoveryQueue
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy
	volConfig.LUKSEncryption = luksEncryption
	volConfig.FileSystem = fstype

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"namespaceSize":     namespaceSize,
		"flexvolSize":       flexVolBufferSize,
		"spaceAllocation":   spaceAllocation,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"snapshotDir":       snapshotDir,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"LUKSEncryption":    luksEncryption,
		"encryption":        convert.ToPrintableBoolPtr(enableEncryption),
		"tieringPolicy":     tieringPolicy,
		"skipRecoveryQueue": skipRecoveryQueue,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
	}).Debug("Creating FlexVol with NVMe namespace.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, flexVolBufferSize, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-NVMe pool %s/%s; error: %v", storagePool.Name(), aggregate, aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))

			// Move on to the next pool.
			continue
		}

		// Make comment field from labels
		labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
		if labelErr != nil {
			return labelErr
		}

		// Create the volume.
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
				Qos:             qosPolicyGroup,
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
			if api.IsVolumeCreateJobExistsError(err) {
				// TODO(sphadnis): If it was decided that iSCSI has a bug here, make similar changes for NVMe.
				return nil
			}

			errMessage := fmt.Sprintf(
				"ONTAP-NVMe pool %s/%s; error creating volume %s: %v", storagePool.Name(),
				aggregate, name, err,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))

			// Move on to the next pool.
			continue
		}

		osType := "linux"
		flexVolName := volConfig.InternalName
		namespaceName := extractNamespaceName(volConfig.InternalID)
		nsPath := createNamespacePath(flexVolName, namespaceName)

		// If a DP volume, do not create the Namespace, it will be copied over by snapmirror.
		if !volConfig.IsMirrorDestination {
			// Attributes stored in the namespace comment field.
			nsComment := map[string]string{
				nsAttributeFSType:    fstype,
				nsAttributeLUKS:      luksEncryption,
				nsAttributeDriverCtx: string(d.Config.DriverContext),
			}

			nsCommentString, err := d.createNVMeNamespaceCommentString(ctx, nsComment, nsMaxCommentLength)
			if err != nil {
				// If we come here due to any failure, namespace creation will fail for all the pools as this is a
				// necessary step before we call NVMeNamespaceCreate. So, we return from here itself.
				return err
			}

			// Create namespace. If this fails, clean up and move on to the next pool.
			err = d.API.NVMeNamespaceCreate(
				ctx, api.NVMeNamespace{
					Name:      nsPath,
					Size:      namespaceSize,
					OsType:    osType,
					BlockSize: defaultNamespaceBlockSize,
					Comment:   nsCommentString,
				})
			if err != nil {
				errMessage := fmt.Sprintf(
					"ONTAP-NVMe pool %s/%s; error creating NVMe Namespace %s: %v", storagePool.Name(),
					aggregate, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, errors.New(errMessage))

				// Don't leave the new FlexVol around.
				if err := d.API.VolumeDestroy(ctx, name, true, true); err != nil {
					Logc(ctx).WithField("volume", name).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after Namespace create error.")
				}

				// Move on to the next pool.
				continue
			}

			// Get the newly created namespace and save the UUID
			newNamespace, err := d.API.NVMeNamespaceGetByName(ctx, nsPath)
			if err != nil {
				return fmt.Errorf("failure checking for existence of volume: %v", err)
			}

			if newNamespace == nil {
				return fmt.Errorf("newly created volume %s not found", name)
			}

			// Store the Namespace UUID and Namespace Path for future operations.
			volConfig.AccessInfo.NVMeNamespaceUUID = newNamespace.UUID
			volConfig.InternalID = nsPath

			Logc(ctx).WithFields(LogFields{
				"name":          name,
				"namespaceUUID": volConfig.AccessInfo.NVMeNamespaceUUID,
				"internalName":  volConfig.InternalName,
				"internalID":    volConfig.InternalID,
			}).Debug("Created FlexVol with NVMe namespace.")
		}
		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone.
func (d *NVMeStorageDriver) CreateClone(
	ctx context.Context, volConfig, cloneVolConfig *storage.VolumeConfig,
	storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"method":      "CreateClone",
		"type":        "NVMeStorageDriver",
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
	var err error
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

		// Ensure the volume exists
		flexvol, err := d.API.VolumeInfo(ctx, cloneVolConfig.CloneSourceVolumeInternal)
		if err != nil {
			return err
		} else if flexvol == nil {
			return fmt.Errorf("volume %s not found", cloneVolConfig.CloneSourceVolumeInternal)
		}

		// Get the source volume's label
		if flexvol.Comment != "" {
			labels = flexvol.Comment
		}

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

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating volume clone.")
	if err = cloneFlexvol(
		ctx, cloneVolConfig, labels, split, &d.Config, d.API, api.QosPolicyGroup{},
	); err != nil {
		return err
	}

	// Extract the namespace name from volConfig.InternalID because
	// Namespace name for clone is going to be the same as parent volume
	cloneFlexVolName := cloneVolConfig.InternalName
	cloneNamespaceName := extractNamespaceName(volConfig.InternalID)
	nsPath := createNamespacePath(cloneFlexVolName, cloneNamespaceName)

	ns, err := d.API.NVMeNamespaceGetByName(ctx, nsPath)
	if err != nil {
		return fmt.Errorf("Problem fetching namespace %v. Error:%v", nsPath, err)
	}
	// Populate access info in the cloneVolConfig
	cloneVolConfig.AccessInfo.NVMeNamespaceUUID = ns.UUID
	cloneVolConfig.InternalID = nsPath
	return nil
}

// Import adds non managed ONTAP volume to trident.
func (d *NVMeStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"method":       "Import",
		"type":         "NVMeStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
		"noRename":     volConfig.ImportNoRename,
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

	// Set the filesystem type to backend's, if it hasn't been set via annotation
	// in the provided pvc during import.
	if volConfig.FileSystem == "" {
		volConfig.FileSystem = d.Config.FileSystemType
	}

	// Get the namespace info from the volume
	nsInfo, err := d.API.NVMeNamespaceGetByName(ctx, "/vol/"+originalName+"/*")
	if err != nil {
		return err
	} else if nsInfo == nil {
		return fmt.Errorf("nvme namespace not found in volume %s", originalName)
	}

	// Lock the namespace until import is done.
	namespaceMutex.Lock(nsInfo.UUID)
	defer namespaceMutex.Unlock(nsInfo.UUID)

	// The Namespace should be online
	if nsInfo.State != "online" {
		return fmt.Errorf("Namespace %s is not online", nsInfo.Name)
	}

	// The Namespace should not be mapped to any subsystem
	nsMapped, err := d.API.NVMeIsNamespaceMapped(ctx, "", nsInfo.UUID)
	if err != nil {
		return err
	} else if nsMapped == true {
		return fmt.Errorf("namespace %s is mapped to a subsystem", nsInfo.Name)
	}

	// Use the Namespace size
	volConfig.Size = nsInfo.Size

	// If the import is a LUKS encrypted volume, then remove the LUKS metadata overhead from the reported
	// size on the volConfig.
	if convert.ToBool(volConfig.LUKSEncryption) {
		newSize, err := subtractUintFromSizeString(volConfig.Size, luks.MetadataSize)
		if err != nil {
			return err
		}
		volConfig.Size = newSize
	}

	// Rename the volume if Trident will manage its lifecycle and the names are different
	if !volConfig.ImportNotManaged && !volConfig.ImportNoRename {
		err = d.API.VolumeRename(ctx, originalName, volConfig.InternalName)
		if err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf(
				"Could not import volume, rename volume failed: %v", err)
			return fmt.Errorf("volume %s rename failed: %v", originalName, err)
		}
	}

	// Update volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexvol.Comment) {
			// Set the base label
			storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

			// Make comment field from labels
			labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
				d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
			if labelErr != nil {
				return labelErr
			}

			err = d.API.VolumeSetComment(ctx, volConfig.InternalName, originalName, labels)
			if err != nil {
				Logc(ctx).WithField("originalName", originalName).Errorf("Modifying comment failed: %v", err)
				return fmt.Errorf("volume %s modify failed: %v", originalName, err)
			}
		}
	}
	importedFlexVolName := volConfig.InternalName
	importedNamespaceName := extractNamespaceName(nsInfo.Name)
	volConfig.InternalID = createNamespacePath(importedFlexVolName, importedNamespaceName)
	volConfig.AccessInfo.NVMeNamespaceUUID = nsInfo.UUID
	return nil
}

// Rename changes the volume name.
func (d *NVMeStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"method":  "Rename",
		"type":    "NVMeStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	return errors.UnsupportedError(fmt.Sprintf("renaming volumes is not supported by backend type %s", d.Name()))
}

// Destroy the requested (volume,namespace) storage tuple.
func (d *NVMeStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"method": "Destroy",
		"type":   "NVMeStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// Validate if FlexVol exists before trying to destroy it.
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
		// TODO(sphadnis): Check if we need to do anything for docker.
		Logc(ctx).Debug("No actions for Destroy for Docker.")
	}

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

	// If flexVol has been a snapmirror destination.
	if err = d.API.SnapmirrorDeleteViaDestination(ctx, name, d.API.SVMName()); err != nil {
		if !errors.IsNotFoundError(err) {
			return err
		}
	}

	// If flexvol has been a snapmirror source
	if err = d.API.SnapmirrorRelease(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	// Delete the FlexVol and Namespace.
	err = d.API.VolumeDestroy(ctx, name, true, skipRecoveryQueue)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	return nil
}

// Publish prepares the volume to attach/mount it to the pod.
func (d *NVMeStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	// InternalName is of the format <storagePrefix><pvc_UUID>
	name := volConfig.InternalName
	// volConfig.Name is same as <pvc-UUID>
	pvName := volConfig.Name

	fields := LogFields{
		"method": "Publish",
		"type":   "NVMeStorageDriver",
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
		return errors.UnsupportedError(fmt.Sprintf("the volume %v is not enabled for read or writes", name))
	}

	if publishInfo.Localhost {
		// Get its HostNQN and populate it in publishInfo.
		nvmeHandler := nvme.NewNVMeHandler()
		nqn, err := nvmeHandler.GetHostNqn(ctx)
		if err != nil {
			return err
		}
		publishInfo.HostNQN = nqn
	}

	if volConfig.IsMirrorDestination {
		// In this case, InternalID and NVMeNamespaceUUID would be empty
		volConfig.InternalID = createNamespacePath(volConfig.InternalName, extractNamespaceName(""))
		ns, err := d.API.NVMeNamespaceGetByName(ctx, volConfig.InternalID)
		if err != nil {
			return fmt.Errorf("problem fetching namespace %v. Error:%v", volConfig.InternalID, err)
		}

		volConfig.AccessInfo.NVMeNamespaceUUID = ns.UUID
	}

	nsPath := volConfig.InternalID
	nsUUID := volConfig.AccessInfo.NVMeNamespaceUUID

	// For docker context, some of the attributes like fsType, luks needs to be
	// fetched from namespace where they were stored while creating the namespace.
	if d.Config.DriverContext == tridentconfig.ContextDocker {
		ns, err := d.API.NVMeNamespaceGetByName(ctx, nsPath)
		if err != nil {
			return fmt.Errorf("Problem fetching namespace %v. Error:%v", nsPath, err)
		}
		if ns == nil {
			return fmt.Errorf("Namespace %v not found", nsPath)
		}
		nsAttrs, err := d.ParseNVMeNamespaceCommentString(ctx, ns.Comment)
		publishInfo.FilesystemType = nsAttrs[nsAttributeFSType]
		publishInfo.LUKSEncryption = nsAttrs[nsAttributeLUKS]
	} else {
		publishInfo.FilesystemType = volConfig.FileSystem
		publishInfo.LUKSEncryption = volConfig.LUKSEncryption
	}

	// Get host nqn
	if publishInfo.HostNQN == "" {
		Logc(ctx).Error("Host NQN is empty")
		return fmt.Errorf("hostNQN not found")
	} else {
		Logc(ctx).Debug("Host NQN is ", publishInfo.HostNQN)
	}

	// When FS type is RAW, we create a new subsystem per namespace,
	// else we use the subsystem created for that particular node
	var ssName string
	var completeSSName string
	if volConfig.FileSystem == filesystem.Raw {
		ssName = getNamespaceSpecificSubsystemName(name, pvName)
	} else {
		// For Docker plugin mode, use storage prefix for stable subsystem naming
		// This allows user control over subsystem sharing via storage prefix configuration
		if tridentconfig.CurrentDriverContext == tridentconfig.ContextDocker {
			ssName = d.getStoragePrefixSubsystemName()
		} else {
			if completeSSName, ssName, err = getUniqueNodeSpecificSubsystemName(
				publishInfo.HostName, publishInfo.TridentUUID, nvmeSubsystemPrefix, maximumSubsystemNameLength); err != nil {
				return fmt.Errorf("failed to create node specific subsystem name: %w", err)
			}
		}
	}

	// Update the subsystem comment
	var ssComment string
	if completeSSName == "" {
		ssComment = ssName
	} else {
		ssComment = completeSSName
	}

	// If 2 concurrent requests try to create the same subsystem, one will succeed and ONTAP is guaranteed to return
	// "already exists" error code for the other. So no need for locking around subsystem creation.
	subsystem, err := d.createOrGetSubsystem(ctx, ssName, ssComment)
	if err != nil {
		return err
	}

	if subsystem == nil {
		return fmt.Errorf("No subsystem returned after subsystem create")
	}

	unlock := lockNamespaceAndSubsystem(nsUUID, subsystem.UUID)
	defer unlock()

	// Fill important info in publishInfo
	publishInfo.NVMeSubsystemNQN = subsystem.NQN
	publishInfo.NVMeSubsystemUUID = subsystem.UUID
	publishInfo.NVMeNamespaceUUID = nsUUID
	publishInfo.SANType = d.Config.SANType

	// Add HostNQN to the subsystem using api call
	if err := d.API.NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID); err != nil {
		Logc(ctx).Errorf("add host to subsystem failed, %v", err)
		return err
	}

	if err := d.API.NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, nsUUID); err != nil {
		return err
	}

	publishInfo.VolumeAccessInfo.NVMeTargetIPs = d.ips

	// xfs volumes are always mounted with '-o nouuid' to allow clones to be mounted to the same node as the source
	if publishInfo.FilesystemType == filesystem.Xfs {
		publishInfo.MountOptions = drivers.EnsureMountOption(publishInfo.MountOptions, drivers.MountOptionNoUUID)
	}

	// Fill in the volume config fields as well
	volConfig.AccessInfo = publishInfo.VolumeAccessInfo

	return nil
}

// Unpublish removes the attach publication of the volume.
func (d *NVMeStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"method":            "Unpublish",
		"type":              "NVMeStorageDriver",
		"name":              name,
		"NVMeNamespaceUUID": volConfig.AccessInfo.NVMeNamespaceUUID,
		"NVMeSubsystemUUID": volConfig.AccessInfo.NVMeSubsystemUUID,
		"hostNQN":           publishInfo.HostNQN,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	subsystemUUID := volConfig.AccessInfo.NVMeSubsystemUUID
	namespaceUUID := volConfig.AccessInfo.NVMeNamespaceUUID

	unlock := lockNamespaceAndSubsystem(namespaceUUID, subsystemUUID)
	defer unlock()

	removePublishInfo, err := d.API.NVMeEnsureNamespaceUnmapped(ctx, publishInfo.HostNQN, subsystemUUID, namespaceUUID)
	if removePublishInfo {
		volConfig.AccessInfo.NVMeTargetIPs = []string{}
		volConfig.AccessInfo.NVMeSubsystemNQN = ""
		volConfig.AccessInfo.NVMeSubsystemUUID = ""
	}
	return err
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NVMeStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NVMeStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"method":       "GetSnapshot",
		"type":         "NVMeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getVolumeSnapshot(ctx, snapConfig, &d.Config, d.API, d.namespaceSize)
}

// GetSnapshots returns the list of snapshots associated with the specified volume.
func (d *NVMeStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {
	fields := LogFields{
		"method":     "GetSnapshots",
		"type":       "NVMeStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getVolumeSnapshotList(ctx, volConfig, &d.Config, d.API, d.namespaceSize)
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *NVMeStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"method":       "CreateSnapshot",
		"type":         "NVMeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"sourceVolume": snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	return createFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API, d.namespaceSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NVMeStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"method":       "RestoreSnapshot",
		"type":         "NVMeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *NVMeStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"method":       "DeleteSnapshot",
		"type":         "NVMeStorageDriver",
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

		// we must return the error, even if we started a split, so the snapshot delete is retried.
		return err
	}

	// Clean up any split timer.
	d.cloneSplitTimers.Delete(snapConfig.ID())

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// Get tests for the existence of a volume.
func (d *NVMeStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{
		"method": "Get",
		"type":   "NVMeStorageDriver",
	}
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

// GetStorageBackendSpecs retrieves storage backend capabilities.
func (d *NVMeStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools.
func (d *NVMeStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *NVMeStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "NVMeStorageDriver"}
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

// getStoragePoolAttributes returns the map for storage pool attributes.
func (d *NVMeStorageDriver) getStoragePoolAttributes(ctx context.Context) map[string]sa.Offer {
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

func (d *NVMeStorageDriver) createOrGetSubsystem(
	ctx context.Context, ssName, comment string,
) (*api.NVMeSubsystem, error) {
	// This checks if subsystem exists and creates it if not.
	ss, err := d.API.NVMeSubsystemCreate(ctx, ssName, comment)
	if err != nil {
		Logc(ctx).Errorf("subsystem create failed, %v", err)
		return nil, err
	}
	return ss, nil
}

// GetVolumeOpts populates the volume properties required in volume create.
func (d *NVMeStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

// GetInternalVolumeName returns ONTAP specific volume name.
func (d *NVMeStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

// CreatePrepare sets appropriate config/attributes values before calling volume create.
func (d *NVMeStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
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

// CreateFollowup sets up additional attributes once a volume is created.
func (d *NVMeStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"method":       "CreateFollowup",
		"type":         "SANNVMeStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")

	if d.Config.DriverContext == tridentconfig.ContextDocker {
		// gbhatnag TODO: see if we need to do anything in createFollowup for docker context
		Logc(ctx).Trace("No follow-up create actions for Docker.")
		return nil
	}
	return nil
}

// GetProtocol returns the protocol type for this driver.
func (d *NVMeStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

// StoreConfig saves the driver configuration to persistent store.
func (d *NVMeStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

// GetExternalConfig returns the driver configuration attributes exposed to the user.
func (d *NVMeStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name of the
// Flexvol on the storage system.
func (d *NVMeStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	volumeAttrs, err := d.API.VolumeInfo(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	nsAttrs, err := d.API.NVMeNamespaceGetByName(ctx, "/vol/"+volumeID+"/*")
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(nsAttrs, volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver. It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NVMeStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	// Let the caller know we're done by closing the channel.
	defer close(channel)

	// Get all volumes matching the storage prefix.
	volumes, err := d.API.VolumeListByPrefix(ctx, *d.Config.StoragePrefix)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	volConfig := &storage.VolumeConfig{
		InternalName: *d.Config.StoragePrefix + "*",
	}
	// Get all namespaces in volumes matching the storage prefix.
	flexvolName := volConfig.InternalName
	namespaceName := extractNamespaceName(volConfig.InternalID)
	nsPathPattern := createNamespacePath(flexvolName, namespaceName)
	namespaces, err := d.API.NVMeNamespaceList(ctx, nsPathPattern)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with namespaces.
	volumeMap := make(map[string]api.Volume)
	if volumes != nil {
		for _, volumeAttrs := range volumes {
			internalName := volumeAttrs.Name
			volumeMap[internalName] = *volumeAttrs
		}
	}

	// Convert all namespaces to VolumeExternal and write them to the channel.
	if namespaces != nil {
		for idx := range namespaces {
			ns := namespaces[idx]
			volume, ok := volumeMap[ns.VolumeName]
			if !ok {
				Logc(ctx).WithField("path", ns.Name).Warning("FlexVol not found for namespace.")
				continue
			}

			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(ns, &volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NVMeStorageDriver) getVolumeExternal(
	ns *api.NVMeNamespace, volume *api.Volume,
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
		Size:            ns.Size,
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volume.SnapshotPolicy,
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      models.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}
	volumeConfig.AccessInfo.NVMeAccessInfo.NVMeNamespaceUUID = ns.UUID
	volumeConfig.InternalID = ns.Name
	pool := drivers.UnsetPool
	if len(volume.Aggregates) > 0 {
		pool = volume.Aggregates[0]
	}
	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   pool,
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver.
func (d *NVMeStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NVMeStorageDriver)
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
func (d *NVMeStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"method":             "Resize",
		"type":               "NVMeStorageDriver",
		"name":               name,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	if requestedSizeBytes > math.MaxInt64 {
		Logc(ctx).WithFields(fields).Error("Invalid volume size.")
		return fmt.Errorf("invalid volume size")
	}

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

	currentFlexVolSize, err := d.API.VolumeSize(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"name":  name,
		}).Error("Error checking volume size.")
		return fmt.Errorf("error occurred when checking volume size")
	}

	nsPath := volConfig.InternalID
	ns, err := d.API.NVMeNamespaceGetByName(ctx, nsPath)
	if err != nil {
		return fmt.Errorf("error while checking namespace size, %v", err)
	}

	nsSizeBytes, err := convert.ToPositiveInt64(ns.Size)
	if err != nil {
		return fmt.Errorf("error while parsing namespace size, %v", err)
	}

	if int64(requestedSizeBytes) < nsSizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", requestedSizeBytes, nsSizeBytes)
	}

	// Add a constant overhead for LUKS volumes to account for LUKS metadata.
	requestedSizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, requestedSizeBytes, volConfig.LUKSEncryption)

	snapshotReserveInt, err := getSnapshotReserveFromOntap(ctx, name, d.API.VolumeInfo)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume.")
	}

	newFlexVolSize := drivers.CalculateVolumeSizeBytes(ctx, name, requestedSizeBytes, snapshotReserveInt)
	newFlexVolSize = uint64(LUNMetadataBufferMultiplier * float64(newFlexVolSize))

	sameNamespaceSize := capacity.VolumeSizeWithinTolerance(int64(requestedSizeBytes), nsSizeBytes,
		tridentconfig.SANResizeDelta)

	sameFlexVolSize := capacity.VolumeSizeWithinTolerance(int64(newFlexVolSize), int64(currentFlexVolSize),
		tridentconfig.SANResizeDelta)

	if sameNamespaceSize && sameFlexVolSize {
		Logc(ctx).WithFields(LogFields{
			"requestedSize": requestedSizeBytes,
			"currentNsSize": nsSizeBytes,
			"name":          name,
			"delta":         tridentconfig.SANResizeDelta,
		}).Info("Requested size and current namespace size are within the delta and therefore considered" +
			" the same size for SAN resize operations.")
		volConfig.Size = strconv.FormatInt(nsSizeBytes, 10)
		return nil
	}

	if !sameFlexVolSize && (currentFlexVolSize > newFlexVolSize) {
		Logc(ctx).WithFields(LogFields{
			"requestedSize":      requestedSizeBytes,
			"currentFlexvolSize": currentFlexVolSize,
			"name":               name,
		}).Info("Requested size is less than current FlexVol size. FlexVol will not be resized.")
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
		ctx, name, newFlexVolSize, d.Config, d.GetAPI(),
	); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Resize FlexVol.
	if !sameFlexVolSize && (currentFlexVolSize < newFlexVolSize) {
		if err = d.API.VolumeSetSize(ctx, name, strconv.FormatUint(newFlexVolSize, 10)); err != nil {
			Logc(ctx).WithField("error", err).Error("Volume resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}

	// Resize namespace.
	if !sameNamespaceSize {
		if err = d.API.NVMeNamespaceSetSize(ctx, ns.UUID, int64(requestedSizeBytes)); err != nil {
			Logc(ctx).WithField("error", err).Error("Namespace resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}

	// LUKS metadata size is not reported so remove it from LUN size
	requestedSizeBytes = decrementWithLUKSMetadataIfLUKSEnabled(ctx, requestedSizeBytes, volConfig.LUKSEncryption)

	// Setting the new size in the volume config.
	volConfig.Size = strconv.FormatUint(requestedSizeBytes, 10)
	return nil
}

// ReconcileNodeAccess manages the k8s node access related changes for the driver.
func (d *NVMeStorageDriver) ReconcileNodeAccess(
	_ context.Context, _ []*models.Node, _, _ string,
) error {
	// Note(sphadnis):
	// 1. NAS drivers takes care export policy rules.
	// 2. SAN iSCSI driver needs to take care of per backend IGroup in reconcile node access.
	// 3. Couldn't find anything to be taken care of for this driver in reconcile node yet!
	return nil
}

func (d *NVMeStorageDriver) ReconcileVolumeNodeAccess(ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "NVMeStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *NVMeStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, sa.NVMeTransport, d.GetStorageBackendPhysicalPoolNames(ctx), d.Config.Aggregate)
}

// String makes NVMeStorageDriver satisfy the Stringer interface.
func (d *NVMeStorageDriver) String() string {
	return convert.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes SANStorageDriver satisfy the GoStringer interface.
func (d *NVMeStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig.
func (d *NVMeStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// EstablishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship.
func (d *NVMeStorageDriver) EstablishMirror(
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

// ReestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously.
func (d *NVMeStorageDriver) ReestablishMirror(
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

// PromoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced.
func (d *NVMeStorageDriver) PromoteMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotName string,
) (bool, error) {
	return promoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotName, d.Config.ReplicationPolicy,
		d.API)
}

// GetMirrorStatus returns the current state of a snapmirror relationship.
func (d *NVMeStorageDriver) GetMirrorStatus(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, error) {
	return getMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// ReleaseMirror will release the snapmirror relationship data of the source volume.
func (d *NVMeStorageDriver) ReleaseMirror(ctx context.Context, localInternalVolumeName string) error {
	return releaseMirror(ctx, localInternalVolumeName, d.API)
}

// GetReplicationDetails returns the replication policy and schedule of a snapmirror relationship.
func (d *NVMeStorageDriver) GetReplicationDetails(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, string, string, error) {
	return getReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// UpdateMirror will attempt a mirror update for the given destination volume
func (d *NVMeStorageDriver) UpdateMirror(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	return mirrorUpdate(ctx, localInternalVolumeName, snapshotName, d.API)
}

// CheckMirrorTransferState will look at the transfer state of the mirror relationship to determine if it is failed,
// succeeded or in progress
func (d *NVMeStorageDriver) CheckMirrorTransferState(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return checkMirrorTransferState(ctx, localInternalVolumeName, d.API)
}

// GetMirrorTransferTime will return the transfer time of the mirror relationship
func (d *NVMeStorageDriver) GetMirrorTransferTime(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return getMirrorTransferTime(ctx, localInternalVolumeName, d.API)
}

// CreateNVMeNamespaceCommentString returns the string that needs to be stored in namespace comment field.
func (d *NVMeStorageDriver) createNVMeNamespaceCommentString(ctx context.Context, nsAttributeMap map[string]string, maxCommentLength int) (string, error) {
	nsCommentMap := map[string]map[string]string{}
	nsCommentMap[nsAttribute] = nsAttributeMap

	nsCommentJSON, err := json.Marshal(nsCommentMap)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal namespace comments: %+v.", nsCommentMap)
		return "", err
	}

	commentsJSONBytes := new(bytes.Buffer)
	if err = json.Compact(commentsJSONBytes, nsCommentJSON); err != nil {
		Logc(ctx).Errorf("Failed to compact namespace comments: %s.", string(nsCommentJSON))
		return "", err
	}

	commentsJSONString := commentsJSONBytes.String()

	if maxCommentLength != 0 && len(commentsJSONString) > maxCommentLength {
		Logc(ctx).WithFields(LogFields{
			"commentsJSON":       commentsJSONString,
			"commentsJSONLength": len(commentsJSONString),
			"maxCommentLength":   maxCommentLength,
		}).Error("Comment length exceeds the character limit.")
		return "", fmt.Errorf("comment length %v exceeds the character limit of %v characters",
			len(commentsJSONString), maxCommentLength)
	}

	return commentsJSONString, nil
}

// ParseNVMeNamespaceCommentString returns the map of attributes that were stored in namespace comment field.
func (d *NVMeStorageDriver) ParseNVMeNamespaceCommentString(_ context.Context, comment string) (map[string]string, error) {
	// Parse the comment
	nsComment := map[string]map[string]string{}

	err := json.Unmarshal([]byte(comment), &nsComment)
	if err != nil {
		return nil, err
	}

	nsAttrs := nsComment[nsAttribute]
	if nsAttrs != nil {
		return nsAttrs, nil
	}
	return nil, fmt.Errorf("nsAttrs field not found in Namespace comment")
}

// getStoragePrefixSubsystemName generates a stable subsystem name using storage prefix for Docker mode
func (d *NVMeStorageDriver) getStoragePrefixSubsystemName() string {
	// Default for Docker is "netappdvp_", users can customize for isolation/sharing
	storagePrefix := *d.Config.StoragePrefix

	// Create subsystem name using storage prefix (similar to igroup naming)
	subsystemName := fmt.Sprintf("%s_subsystem", storagePrefix)

	// Ensure it fits within the length limit
	if len(subsystemName) > maximumSubsystemNameLength {
		maxPrefixLength := maximumSubsystemNameLength - len("_subsystem") - 1
		subsystemName = fmt.Sprintf("%s_subsystem", storagePrefix[:maxPrefixLength])
	}

	return subsystemName
}

// getNamespaceSpecificSubsystemName constructs the subsystem name using the name passed.
func getNamespaceSpecificSubsystemName(name, pvName string) string {
	subSystemPrefix := "s"
	// Check if the subsystem name length is greater than 64 chars.
	// 2 is added to the length to account for "s_" in the subsystem name
	if (len(name) + 2) > maximumSubsystemNameLength {
		// Truncate the storagePrefix so that the entire subsystem doesn't exceed 64 chars
		// The -2 at the end is done to account for two additional "_" in the subsystem name
		truncatedStoragePrefixIndex := maximumSubsystemNameLength - len(subSystemPrefix) - len(pvName) - 2

		storagePrefix := name[:truncatedStoragePrefixIndex]

		return (fmt.Sprintf("%s_%s_%s", subSystemPrefix, storagePrefix, pvName))
	}
	return (fmt.Sprintf("%s_%s", subSystemPrefix, name))
}

// extractNamespaceName extracts the namespace name from the given string if nsStr is set
// if nsStr is not set, return default namespace name "namespace0"
// if nsStr has malformed namespacePath, return "MalformedNamespace"
func extractNamespaceName(nsStr string) string {
	if nsStr == "" {
		return "namespace0"
	} else if NVMeNamespaceRegExp.MatchString(nsStr) {
		namespaceName := strings.Split(nsStr, "/")
		if len(namespaceName) == 4 {
			return namespaceName[3]
		}
	}
	// If we end up here, the namespace Path in nsStr is malformed.
	// return a string that will cause the operation to fail
	return "MalformedNamespace"
}

// createNamespacePath returns the namespace path in a FlexVol.
func createNamespacePath(flexvolName, namespaceName string) string {
	return ("/vol/" + flexvolName + "/" + namespaceName)
}

func (d *NVMeStorageDriver) namespaceSize(ctx context.Context, name string) (int, error) {
	fields := LogFields{
		"Method": "namespaceSize",
		"Type":   "NVMeStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> namespaceSize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< namespaceSize")

	nsPath := "/vol/" + name + "/*"
	return d.API.NVMeNamespaceGetSize(ctx, nsPath)
}

// EnablePublishEnforcement sets the publishEnforcement on older NVMe volumes.
func (d *NVMeStorageDriver) EnablePublishEnforcement(_ context.Context, volume *storage.Volume) error {
	volume.Config.AccessInfo.PublishEnforcement = true
	return nil
}

// CanEnablePublishEnforcement dictates if any NVMe volume will get published on a node, depending on the node state.
func (d *NVMeStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}

func (d *NVMeStorageDriver) HealVolumePublishEnforcement(ctx context.Context, volume *storage.Volume) bool {
	return HealSANPublishEnforcement(ctx, d, volume)
}
