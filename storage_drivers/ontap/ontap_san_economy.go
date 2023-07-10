// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/crypto"
	"github.com/netapp/trident/utils/errors"
)

const (
	maxLunNameLength       = 254
	minLUNsPerFlexvol      = 50
	defaultLUNsPerFlexvol  = 100
	maxLUNsPerFlexvol      = 200
	snapshotNameSeparator  = "_snapshot_"
	snapshotCopyNameSuffix = "_copy"
)

func GetLUNPathEconomy(bucketName, volNameInternal string) string {
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

// internalVolName is expected to have the storage prefix included
// parameters: internalVolName=storagePrefix_my-Lun snapName=my-Snapshot
// output: storagePrefix_my_Lun_snapshot_my_Snapshot
func (o *LUNHelper) GetSnapshotName(internalVolName, snapName string) string {
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
	return snapshotName != ""
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
	API               api.OntapAPI
	telemetry         *Telemetry
	flexvolNamePrefix string
	helper            *LUNHelper
	lunsPerFlexvol    int

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool
}

func (d *SANEconomyStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *SANEconomyStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *SANEconomyStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d *SANEconomyStorageDriver) Name() string {
	return tridentconfig.OntapSANEconomyStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *SANEconomyStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		lif0 := "noLIFs"
		if len(d.ips) > 0 {
			lif0 = d.ips[0]
		}
		return CleanBackendName("ontapsaneco_" + lif0)
	} else {
		return d.Config.BackendName
	}
}

func (d *SANEconomyStorageDriver) FlexvolNamePrefix() string {
	return d.flexvolNamePrefix
}

// Initialize from the provided config
func (d *SANEconomyStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "SANEconomyStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	// Initialize the driver's CommonStorageDriverConfig
	d.Config.CommonStorageDriverConfig = commonConfig

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		d.API, err = InitializeOntapDriver(ctx, config)
		if err != nil {
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}
	}
	d.Config = *config
	d.helper = NewLUNHelper(d.Config, driverContext)

	d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, "iscsi")
	if err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no iSCSI data LIFs found on SVM %s", d.API.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
	}

	// Remap driverContext for artifact naming so the names remain stable over time
	var artifactPrefix string
	switch driverContext {
	case tridentconfig.ContextDocker:
		artifactPrefix = artifactPrefixDocker
	case tridentconfig.ContextCSI:
		artifactPrefix = artifactPrefixKubernetes
	default:
		return fmt.Errorf("unknown driver context: %s", driverContext)
	}

	// Set up internal driver state
	d.flexvolNamePrefix = fmt.Sprintf("%s_lun_pool_%s_", artifactPrefix, *d.Config.StoragePrefix)
	d.flexvolNamePrefix = strings.Replace(d.flexvolNamePrefix, "__", "_", -1)

	// ensure lun cap is valid
	if config.LUNsPerFlexvol == "" {
		d.lunsPerFlexvol = defaultLUNsPerFlexvol
	} else {
		errstr := "invalid config value for lunsPerFlexvol"
		if d.lunsPerFlexvol, err = strconv.Atoi(config.LUNsPerFlexvol); err != nil {
			return fmt.Errorf("%v: %v", errstr, err)
		}
		if d.lunsPerFlexvol < minLUNsPerFlexvol {
			return fmt.Errorf(
				"%v: using %d lunsPerFlexvol (minimum is %d)", errstr, d.lunsPerFlexvol, minLUNsPerFlexvol,
			)
		} else if d.lunsPerFlexvol > maxLUNsPerFlexvol {
			return fmt.Errorf(
				"%v: using %d lunsPerFlexvol (maximum is %d)", errstr, d.lunsPerFlexvol, maxLUNsPerFlexvol,
			)
		}
	}

	Logc(ctx).WithFields(
		LogFields{
			"FlexvolNamePrefix": d.flexvolNamePrefix,
			"LUNsPerFlexvol":    d.lunsPerFlexvol,
		},
	).Debugf("SAN Economy driver settings.")

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(
		ctx, d, d.getStoragePoolAttributes(), d.BackendName(),
	)
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = InitializeSANDriver(ctx, driverContext, d.API, &d.Config, d.validate, backendUUID)

	// clean up igroup for failed driver
	if err != nil {
		if d.Config.DriverContext == tridentconfig.ContextCSI {
			err := d.API.IgroupDestroy(ctx, d.Config.IgroupName)
			if err != nil {
				Logc(ctx).WithError(err).WithField("igroup", d.Config.IgroupName).Warn("Error deleting igroup.")
			}
		}
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	d.initialized = true
	return nil
}

func (d *SANEconomyStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANEconomyStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "SANEconomyStorageDriver"}
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
func (d *SANEconomyStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "SANEconomyStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := ValidateSANDriver(ctx, &d.Config, d.ips); err != nil {
		return fmt.Errorf("error driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefixEconomy(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d, 0); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume+LUN with the specified options
func (d *SANEconomyStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method": "Create",
		"Type":   "SANEconomyStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Generic user-facing message
	createError := errors.New("error volume creation failed")

	// Determine a way to see if the volume already exists
	exists, existsInFlexvol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing volume: %v", err)
		return createError
	}
	if exists {
		Logc(ctx).WithFields(LogFields{"LUN": name, "bucketVol": existsInFlexvol}).Debug("LUN already exists.")
		return drivers.NewVolumeExistsError(name)
	}

	// Get candidate physical pools
	physicalPools, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
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
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes()[Size])
	if err != nil {
		return err
	}

	lunSize := strconv.FormatUint(sizeBytes, 10)

	// Ensure LUN name isn't too long
	if len(name) > maxLunNameLength {
		return fmt.Errorf("volume %s name exceeds the limit of %d characters", name, maxLunNameLength)
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get Flexvol options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	spaceAllocation, _ := strconv.ParseBool(
		utils.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation]),
	)
	var (
		spaceReserve      = utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		snapshotDir       = "false"
		exportPolicy      = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = utils.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
		luksEncryption    = storagePool.InternalAttributes()[LUKSEncryption]
	)

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	// Check for a supported file system type
	fstype, err := drivers.CheckSupportedFilesystem(
		ctx, utils.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name,
	)
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
	volConfig.LUKSEncryption = luksEncryption

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf(
				"ONTAP-SAN-ECONOMY pool %s/%s; error: %v", storagePool.Name(), aggregate, aggrLimitsErr,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Move on to the next pool
			continue
		}

		var (
			ignoredVols = make(map[string]struct{})
			bucketVol   string
			lunPathEco  string
			newVol      bool
		)
	prepareBucket:

		volAttrs := &api.Volume{
			Aggregates:      []string{aggregate},
			Encrypt:         enableEncryption,
			Name:            d.FlexvolNamePrefix() + "*",
			SnapshotDir:     false,
			SnapshotPolicy:  snapshotPolicy,
			SpaceReserve:    spaceReserve,
			SnapshotReserve: snapshotReserveInt,
			TieringPolicy:   tieringPolicy,
		}

		// Make sure we have a Flexvol for the new LUN
		bucketVol, newVol, err = d.ensureFlexvolForLUN(
			ctx, volAttrs, sizeBytes, opts, d.Config, storagePool, ignoredVols,
		)
		if err != nil {
			errMessage := fmt.Sprintf(
				"ONTAP-SAN-ECONOMY pool %s/%s; BucketVol location/creation failed %s: %v",
				storagePool.Name(), aggregate, name, err,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Move on to the next pool
			continue
		}

		// Grow or shrink the Flexvol as needed
		if err = d.resizeFlexvol(ctx, bucketVol, sizeBytes); err != nil {

			errMessage := fmt.Sprintf(
				"ONTAP-SAN-ECONOMY pool %s/%s; Flexvol resize failed %s/%s: %v",
				storagePool.Name(), aggregate, bucketVol, name, err,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Don't leave the new Flexvol around if we just created it
			if newVol {
				if err := d.API.VolumeDestroy(ctx, bucketVol, true); err != nil {
					Logc(ctx).WithField("volume", bucketVol).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after resize error.")
				}
			}

			// Move on to the next pool
			continue
		}

		lunPathEco = GetLUNPathEconomy(bucketVol, name)
		osType := "linux"

		// Create the LUN
		// For the ONTAP SAN economy driver, we set the QoS policy at the LUN layer.

		err = d.API.LunCreate(
			ctx, api.Lun{
				Name:           lunPathEco,
				Qos:            qosPolicyGroup,
				Size:           lunSize,
				OsType:         osType,
				SpaceReserved:  utils.Ptr(false),
				SpaceAllocated: utils.Ptr(spaceAllocation),
			})

		if err != nil {
			if api.IsTooManyLunsError(err) {
				Logc(ctx).WithError(err).Warn("ONTAP limit for LUNs/Flexvol reached; finding a new Flexvol")
				ignoredVols[bucketVol] = struct{}{}
				goto prepareBucket
			} else {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN-ECONOMY pool %s/%s; error creating LUN %s/%s: %v",
					storagePool.Name(), aggregate, bucketVol, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, fmt.Errorf(errMessage))

				// Don't leave the new Flexvol around if we just created it
				if newVol {
					if err := d.API.VolumeDestroy(ctx, bucketVol, true); err != nil {
						Logc(ctx).WithField("volume", bucketVol).Errorf("Could not clean up volume; %v", err)
					} else {
						Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after LUN create error.")
					}
				}

				// Move on to the next pool
				continue
			}
		}

		actualSize, err := d.getLUNSize(ctx, name, bucketVol)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Failed to determine LUN size")
			return err
		}
		volConfig.Size = strconv.FormatUint(actualSize, 10)

		// Save the fstype in a LUN attribute so we know what to do in Attach
		err = d.API.LunSetAttribute(ctx, lunPathEco, LUNAttributeFSType, fstype, string(d.Config.DriverContext),
			luksEncryption)
		if err != nil {

			errMessage := fmt.Sprintf(
				"ONTAP-SAN-ECONOMY pool %s/%s; error saving file system type for LUN %s/%s: %v",
				storagePool.Name(), aggregate, bucketVol, name, err,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Don't leave the new LUN around
			if err = d.API.LunDestroy(ctx, lunPathEco); err != nil {
				Logc(ctx).WithField("LUN", lunPathEco).Errorf("Could not clean up LUN; %v", err)
			} else {
				Logc(ctx).WithField("volume", name).Debugf("Cleaned up LUN after set attribute error.")
			}

			// Don't leave the new Flexvol around if we just created it
			if newVol {
				if err := d.API.VolumeDestroy(ctx, bucketVol, true); err != nil {
					Logc(ctx).WithField("volume", bucketVol).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after set attribute error.")
				}
			}

			// Move on to the next pool
			continue
		}

		// Resize Flexvol to be the same size or bigger than sum of constituent LUNs because ONTAP creates
		// larger LUNs sometimes based on internal geometry
		if actualSize > sizeBytes {
			if initialVolumeSize, err := d.API.VolumeSize(ctx, bucketVol); err != nil {
				Logc(ctx).WithField("name", bucketVol).Warning("Failed to get volume size.")
			} else {
				err = d.resizeFlexvol(ctx, bucketVol, 0)
				if err != nil {
					Logc(ctx).WithFields(
						LogFields{
							"name":               bucketVol,
							"initialVolumeSize":  initialVolumeSize,
							"adjustedVolumeSize": initialVolumeSize + actualSize - sizeBytes,
						},
					).Warning("Failed to resize new volume to exact sum of LUNs' size.")
				} else {
					if adjustedVolumeSize, err := d.API.VolumeSize(ctx, bucketVol); err != nil {
						Logc(ctx).WithField("name", bucketVol).
							Warning("Failed to get volume size after the second resize operation.")
					} else {
						Logc(ctx).WithFields(
							LogFields{
								"name":               bucketVol,
								"initialVolumeSize":  initialVolumeSize,
								"adjustedVolumeSize": adjustedVolumeSize,
							},
						).Debug("FlexVol resized.")
					}
				}
			}
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *SANEconomyStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, _ storage.Pool,
) error {
	source := cloneVolConfig.CloneSourceVolumeInternal
	name := cloneVolConfig.InternalName
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal
	isFromSnapshot := snapshot != ""
	qosPolicy := cloneVolConfig.QosPolicy
	adaptiveQosPolicy := cloneVolConfig.AdaptiveQosPolicy
	luksEncryption := cloneVolConfig.LUKSEncryption

	fields := LogFields{
		"Method":            "CreateClone",
		"Type":              "SANEconomyStorageDriver",
		"name":              name,
		"source":            source,
		"snapshot":          snapshot,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
		"LUKSEncryption":    luksEncryption,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	return d.createLUNClone(
		ctx, name, source, snapshot, &d.Config, d.API, d.FlexvolNamePrefix(), isFromSnapshot, qosPolicyGroup,
	)
}

// Create a volume clone
func (d *SANEconomyStorageDriver) createLUNClone(
	ctx context.Context, lunName, source, snapshot string, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, prefix string, isLunCreateFromSnapshot bool, qosPolicyGroup api.QosPolicyGroup,
) error {
	fields := LogFields{
		"Method":                  "createLUNClone",
		"Type":                    "ontap_san_economy",
		"lunName":                 lunName,
		"source":                  source,
		"snapshot":                snapshot,
		"prefix":                  prefix,
		"isLunCreateFromSnapshot": isLunCreateFromSnapshot,
		"qosPolicyGroup":          qosPolicyGroup,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createLUNClone")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createLUNClone")

	// If the specified LUN copy already exists, return an error
	destinationLunExists, _, err := d.LUNExists(ctx, lunName, prefix)
	if err != nil {
		return fmt.Errorf("error checking for existing destination LUN: %v", err)
	}
	if destinationLunExists {
		return fmt.Errorf("error destination LUN %s already exists", lunName)
	}

	// Check if called from CreateClone and is from a snapshot
	if isLunCreateFromSnapshot {
		source = d.helper.GetSnapshotName(source, snapshot)
	}

	// If the source doesn't exist, return an error
	sourceLunExists, flexvol, err := d.LUNExists(ctx, source, prefix)
	if err != nil {
		return fmt.Errorf("error checking for existing source LUN: %v", err)
	}
	if !sourceLunExists {
		return fmt.Errorf("error source LUN %s does not exist", source)
	}

	// Create the clone based on given LUN
	// For the ONTAP SAN economy driver, we set the QoS policy at the LUN layer.
	err = client.LunCloneCreate(ctx, flexvol, source, lunName, qosPolicyGroup)
	if err != nil {
		return err
	}
	// Grow or shrink the Flexvol as needed
	return d.resizeFlexvol(ctx, flexvol, 0)
}

func (d *SANEconomyStorageDriver) Import(
	ctx context.Context, volConfig *storage.VolumeConfig, originalName string,
) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "SANEconomyStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	return errors.New("import is not implemented")
}

func (d *SANEconomyStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "SANEconomyStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	return errors.New("rename is not implemented")
}

// Destroy the LUN
func (d *SANEconomyStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "SANEconomyStorageDriver",
		"Name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	// Generic user-facing message
	deleteError := errors.New("volume deletion failed")

	exists, bucketVol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		Logc(ctx).Warnf("LUN %v does not exist", name)
		return nil
	}

	lunPathEco := GetLUNPathEconomy(bucketVol, name)
	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Could not get target info")
			return err
		}

		// Get the LUN ID
		lunID, err = d.API.LunMapInfo(ctx, d.Config.IgroupName, lunPathEco)
		if err != nil {
			return fmt.Errorf("error reading LUN maps for volume %s: %v", name, err)
		}
		if lunID >= 0 {
			// Inform the host about the device removal
			if _, err := utils.PrepareDeviceForRemoval(ctx, lunID, iSCSINodeName, true, false); err != nil {
				Logc(ctx).Error(err)
			}
		}
	}

	// Before deleting the LUN, check if a LUN has associated snapshots. If so, delete all associated snapshots
	externalVolumeName := d.helper.GetExternalVolumeNameFromPath(lunPathEco)
	snapList, err := d.getSnapshotsEconomy(ctx, name, externalVolumeName)
	if err != nil {
		Logc(ctx).Errorf("Error enumerating snapshots: %v", err)
		return deleteError
	}
	for _, snap := range snapList {
		err = d.DeleteSnapshot(ctx, snap.Config, volConfig)
		if err != nil {
			Logc(ctx).Errorf("Error snap-LUN delete failed: %v", err)
			return err
		}
	}

	if err = LunUnmapAllIgroups(ctx, d.GetAPI(), lunPathEco); err != nil {
		msg := "error removing all mappings from LUN"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}

	if err = d.API.LunDestroy(ctx, lunPathEco); err != nil {
		fields := LogFields{
			"Method": "Destroy",
			"Type":   "SANEconomyStorageDriver",
			"LUN":    lunPathEco,
		}
		Logc(ctx).WithFields(fields).Errorf("Error LUN delete failed: %v", err)

		return deleteError
	}

	// Check if a bucket volume has no more LUNs. If none left, delete the bucketVol. Else, call for resize
	return d.DeleteBucketIfEmpty(ctx, bucketVol)
}

// DeleteBucketIfEmpty will check if the given bucket volume is empty, if the bucket is empty it will be deleted.
// Otherwise, it will be resized.
func (d *SANEconomyStorageDriver) DeleteBucketIfEmpty(ctx context.Context, bucketVol string) error {
	fields := LogFields{
		"Method":    "Destroy",
		"Type":      "SANEconomyStorageDriver",
		"bucketVol": bucketVol,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteBucketIfEmpty")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteBucketIfEmpty")

	lunPathPattern := fmt.Sprintf("/vol/%s/*", bucketVol)
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		return fmt.Errorf("error enumerating LUNs for volume %v: %v", bucketVol, err)
	}

	count := len(luns)
	if count == 0 {
		// Delete the bucketVol
		err := d.API.VolumeDestroy(ctx, bucketVol, true)
		if err != nil {
			return fmt.Errorf("error destroying volume %v: %v", bucketVol, err)
		}
	} else {
		// Grow or shrink the Flexvol as needed
		err = d.resizeFlexvol(ctx, bucketVol, 0)
		if err != nil {
			return err
		}
	}
	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANEconomyStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method":      "Publish",
		"Type":        "SANEconomyStorageDriver",
		"name":        name,
		"publishInfo": publishInfo,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	exists, bucketVol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		return fmt.Errorf("error LUN %v does not exist", name)
	}

	lunPath := d.helper.GetLUNPath(bucketVol, name)
	igroupName := d.Config.IgroupName

	// Use the node specific igroup if publish enforcement is enabled and this is for CSI.
	if volConfig.AccessInfo.PublishEnforcement && tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
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
func (d *SANEconomyStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method":      "Unpublish",
		"Type":        "SANEconomyStorageDriver",
		"name":        name,
		"publishInfo": publishInfo,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	if !volConfig.AccessInfo.PublishEnforcement || tridentconfig.CurrentDriverContext != tridentconfig.ContextCSI {
		// Nothing to do if publish enforcement is not enabled
		return nil
	}

	// Ensure the LUN and parent bucket volume exist before attempting to unpublish.
	exists, bucketVol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		// If the LUN doesn't exist at this point, there's nothing left to do for unpublish at this level.
		// However, this scenario could indicate unexpected tampering or unknown states with the backend,
		// so log a warning and return a NotFoundError.
		err := errors.NotFoundError("LUN %v does not exist", name)
		Logc(ctx).WithError(err).Warningf("Unable to unpublish LUN: %s.", name)
		return err
	}

	// Attempt to unmap the LUN from the per-node igroup; the lunPath is where the LUN resides within the flexvol.
	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := d.helper.GetLUNPath(bucketVol, name)
	if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath); err != nil {
		return fmt.Errorf("error unmapping LUN %s from igroup %s; %v", lunPath, igroupName, err)
	}

	// Remove igroup from volume config's access Info
	volConfig.AccessInfo.IscsiIgroup = removeIgroupFromIscsiIgroupList(volConfig.AccessInfo.IscsiIgroup, igroupName)

	// Remove igroup if no LUNs are mapped.
	if err := DestroyUnmappedIgroup(ctx, d.API, igroupName); err != nil {
		return fmt.Errorf("error removing empty igroup; %v", err)
	}

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *SANEconomyStorageDriver) CanSnapshot(
	_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *SANEconomyStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "SANEconomyStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return d.getSnapshotEconomy(ctx, snapConfig, &d.Config)
}

func (d *SANEconomyStorageDriver) getSnapshotEconomy(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolumeName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "getSnapshotEconomy",
		"Type":         "SANEconomyStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolumeName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getSnapshotEconomy")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getSnapshotEconomy")

	fullSnapshotName := d.helper.GetSnapshotName(internalVolumeName, internalSnapName)
	exists, bucketVol, err := d.LUNExists(ctx, fullSnapshotName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	snapPath := d.helper.GetSnapPath(bucketVol, internalVolumeName, internalSnapName)
	lunInfo, err := d.API.LunGetByName(ctx, snapPath)
	if err != nil {
		return nil, err
	}

	sizeBytes, err := strconv.ParseUint(lunInfo.Size, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%v is an invalid volume size: %v", lunInfo.Size, err)
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   lunInfo.CreateTime,
		SizeBytes: int64(sizeBytes),
		State:     storage.SnapshotStateOnline,
	}, nil
}

func (d *SANEconomyStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":             "GetSnapshots",
		"Type":               "SANEconomyStorageDriver",
		"internalVolumeName": volConfig.InternalName,
		"volumeName":         volConfig.Name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	exists, _, err := d.LUNExists(ctx, volConfig.InternalName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("error LUN %v does not exist", volConfig.Name)
	}

	return d.getSnapshotsEconomy(ctx, volConfig.InternalName, volConfig.Name)
}

func (d *SANEconomyStorageDriver) getSnapshotsEconomy(
	ctx context.Context, internalVolumeName, externalVolumeName string,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":          "getSnapshotsEconomy",
		"Type":            "SANEconomyStorageDriver",
		"internalVolName": internalVolumeName,
		"externalVolName": externalVolumeName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getSnapshotsEconomy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getSnapshotsEconomy")

	snapPathPattern := d.helper.GetSnapPathPatternForVolume(externalVolumeName)

	snapList, err := d.API.LunList(ctx, snapPathPattern)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	Logc(ctx).Debugf("Found %v snapshots.", len(snapList))
	snapshots := make([]*storage.Snapshot, 0)

	for _, snap := range snapList {
		snapLunPath := snap.Name

		sizeBytes, err := strconv.ParseUint(snap.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error %v is an invalid volume size: %v", snap.Size, err)
		}
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
				Created:   snap.VolumeName,
				SizeBytes: int64(sizeBytes),
				State:     storage.SnapshotStateOnline,
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *SANEconomyStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "SANEconomyStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
		"snapConfig":   snapConfig,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Info(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Info("<<<< CreateSnapshot")

	internalSnapName := snapConfig.InternalName
	internalVolumeName := snapConfig.VolumeInternalName

	// Check to see if source LUN exists
	exists, bucketVol, err := d.LUNExists(ctx, internalVolumeName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("LUN %v does not exist", internalVolumeName)
	}

	lunPath := GetLUNPathEconomy(bucketVol, internalVolumeName)

	// If the specified volume doesn't exist, return error
	lunInfo, err := d.API.LunGetByName(ctx, lunPath)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}

	size := lunInfo.Size

	// Create the snapshot name/string
	lunName := d.helper.GetSnapshotName(snapConfig.VolumeInternalName, internalSnapName)

	// Create the "snap-LUN" where the snapshot is a LUN clone of the source LUN
	err = d.createLUNClone(
		ctx, lunName, snapConfig.VolumeInternalName, snapConfig.Name, &d.Config, d.API, d.FlexvolNamePrefix(), false,
		api.QosPolicyGroup{},
	)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// Fetching list of snapshots to get snapshot creation time
	snapListResponse, err := d.getSnapshotsEconomy(ctx, internalVolumeName, snapConfig.VolumeName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	for _, snap := range snapListResponse {
		Logc(ctx).WithFields(LogFields{
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}).Info("Snapshot created.")

		sizeBytes, err := strconv.ParseUint(size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error %v is an invalid volume size: %v", size, err)
		}

		return &storage.Snapshot{
			Config:    snapConfig,
			Created:   snap.Created,
			SizeBytes: int64(sizeBytes),
			State:     storage.SnapshotStateOnline,
		}, nil
	}
	return nil, fmt.Errorf("could not find snapshot %s for source volume %s", internalSnapName, internalVolumeName)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANEconomyStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	volLunName := snapConfig.VolumeInternalName
	snapLunName := d.helper.GetSnapshotName(snapConfig.VolumeInternalName, snapConfig.InternalName)
	snapLunCopyName := snapLunName + snapshotCopyNameSuffix

	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "SANEconomyStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	// Check to see if volume LUN exists
	volLunExists, volBucketVol, err := d.LUNExists(ctx, volLunName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing volume LUN: %v", err)
		return err
	}
	if !volLunExists {
		message := fmt.Sprintf("volume LUN %v does not exist", volLunName)
		Logc(ctx).Warnf(message)
		return errors.NotFoundError(message)
	}

	// Check to see if the snapshot LUN exists
	snapLunExists, snapBucketVol, err := d.LUNExists(ctx, snapLunName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing snapshot LUN: %v", err)
		return err
	}
	if !snapLunExists {
		return fmt.Errorf("snapshot LUN %v does not exist", snapLunName)
	}

	// Sanity check to ensure both LUNs are in the same Flexvol
	if volBucketVol != snapBucketVol {
		return fmt.Errorf("snapshot LUN %s and volume LUN %s are in different Flexvols", snapLunName, volLunName)
	}

	// Check to see if the snapshot LUN copy exists
	snapLunCopyExists, snapCopyBucketVol, err := d.LUNExists(ctx, snapLunCopyName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing snapshot copy LUN: %v", err)
		return err
	}

	// Delete any copy so we can create a fresh one
	if snapLunCopyExists {

		// Sanity check to ensure both LUNs are in the same Flexvol
		if snapCopyBucketVol != snapBucketVol {
			return fmt.Errorf("snapshot LUN %s and snapshot copy LUN %s are in different Flexvols",
				snapLunName, snapLunCopyName)
		}

		snapLunCopyPath := GetLUNPathEconomy(snapCopyBucketVol, snapLunCopyName)

		if err = LunUnmapAllIgroups(ctx, d.GetAPI(), snapLunCopyPath); err != nil {
			msg := "error removing all mappings from LUN"
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}

		if err = d.API.LunDestroy(ctx, snapLunCopyPath); err != nil {
			return fmt.Errorf("could not delete snapshot copy LUN %s", snapLunCopyName)
		}
	}

	// Get the snapshot LUN
	snapLunPath := GetLUNPathEconomy(snapBucketVol, snapLunName)
	snapLunInfo, err := d.API.LunGetByName(ctx, snapLunPath)
	if err != nil {
		return fmt.Errorf("could not get existing LUN %s; %v", snapLunPath, err)
	}

	// Clone the snapshot LUN
	if err = d.API.LunCloneCreate(ctx, snapBucketVol, snapLunName, snapLunCopyName, snapLunInfo.Qos); err != nil {
		return fmt.Errorf("could not clone snapshot LUN %s: %v", snapLunPath, err)
	}

	// Rename the original LUN
	volLunPath := GetLUNPathEconomy(volBucketVol, volLunName)
	tempVolLunPath := volLunPath + "_original"
	if err = d.API.LunRename(ctx, volLunPath, tempVolLunPath); err != nil {
		return fmt.Errorf("could not rename LUN %s: %v", volLunPath, err)
	}

	// Rename snapshot copy to original LUN path
	snapLunCopyPath := GetLUNPathEconomy(snapBucketVol, snapLunCopyName)
	if err = d.API.LunRename(ctx, snapLunCopyPath, volLunPath); err != nil {

		// Attempt to recover by restoring the original LUN
		if recoverErr := d.API.LunRename(ctx, tempVolLunPath, volLunPath); recoverErr != nil {
			Logc(ctx).WithError(recoverErr).Errorf("Could not recover RestoreSnapshot by renaming LUN %s to %s.",
				tempVolLunPath, volLunPath)
		}

		return fmt.Errorf("could not rename LUN %s: %v", snapLunCopyPath, err)
	}

	if err = LunUnmapAllIgroups(ctx, d.GetAPI(), tempVolLunPath); err != nil {
		Logc(ctx).WithError(err).Warning("Could not remove all mappings from original LUN %s after RestoreSnapshot",
			tempVolLunPath)
	}

	// Delete original LUN
	if err = d.API.LunDestroy(ctx, tempVolLunPath); err != nil {
		Logc(ctx).WithError(err).Warningf("Could not delete original LUN %s after RestoreSnapshot", tempVolLunPath)
	}

	return nil
}

// DeleteSnapshot deletes a LUN snapshot.
func (d *SANEconomyStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":                "DeleteSnapshot",
		"Type":                  "SANEconomyStorageDriver",
		"snapshotName":          snapConfig.InternalName,
		"volumeName":            snapConfig.VolumeInternalName,
		"snapConfig.VolumeName": snapConfig.VolumeName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	internalSnapName := snapConfig.InternalName
	// Creating the path string pattern
	snapLunName := d.helper.GetSnapshotName(snapConfig.VolumeInternalName, internalSnapName)

	// Check to see if the source LUN exists
	exists, bucketVol, err := d.LUNExists(ctx, snapLunName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		return fmt.Errorf("LUN %v does not exist", snapLunName)
	}

	snapPath := GetLUNPathEconomy(bucketVol, snapLunName)

	// Don't leave the new LUN around
	if err = d.API.LunDestroy(ctx, snapPath); err != nil {
		Logc(ctx).WithField("LUN", snapPath).Errorf("Snap-LUN delete failed; %v", err)
		return fmt.Errorf("error deleting snapshot: %v", err)
	}

	// Check if a bucket volume has no more LUNs. If none left, delete the bucketVol. Else, call for resize
	return d.DeleteBucketIfEmpty(ctx, bucketVol)
}

// Get tests for the existence of a volume
func (d *SANEconomyStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "SANEconomyStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	// Generic user-facing message
	getError := fmt.Errorf("volume %s not found", name)

	exists, bucketVol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing LUN: %v", err)
		return getError
	}
	if !exists {
		Logc(ctx).WithField("LUN", name).Debug("LUN not found.")
		return getError
	}

	Logc(ctx).WithFields(LogFields{"LUN": name, "bucketVol": bucketVol}).Debug("Volume found.")

	return nil
}

// ensureFlexvolForLUN accepts a set of Flexvol characteristics and either finds one to contain a new
// LUN or it creates a new Flexvol with the needed attributes.  The name of the matching volume is returned,
// as is a boolean indicating whether the volume was newly created to satisfy this request.
func (d *SANEconomyStorageDriver) ensureFlexvolForLUN(
	ctx context.Context, volAttrs *api.Volume, sizeBytes uint64, opts map[string]string,
	config drivers.OntapStorageDriverConfig,
	storagePool storage.Pool, ignoredVols map[string]struct{},
) (string, bool, error) {
	shouldLimitVolumeSize, flexvolSizeLimit, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, config.CommonStorageDriverConfig,
	)
	if checkVolumeSizeLimitsError != nil {
		return "", false, checkVolumeSizeLimitsError
	}

	// Check if a suitable Flexvol already exists
	flexvol, err := d.getFlexvolForLUN(ctx, volAttrs, sizeBytes, shouldLimitVolumeSize, flexvolSizeLimit, ignoredVols)
	if err != nil {
		return "", false, fmt.Errorf("error finding Flexvol for LUN: %v", err)
	}

	// Found one
	if flexvol != "" {
		return flexvol, false, nil
	}

	// Nothing found, so create a suitable Flexvol
	flexvol, err = d.createFlexvolForLUN(ctx, volAttrs, opts, storagePool)
	if err != nil {
		return "", false, fmt.Errorf("error creating Flexvol for LUN: %v", err)
	}

	return flexvol, true, nil
}

// createFlexvolForLUN creates a new Flexvol matching the specified attributes for
// the purpose of containing LUN supplied as container volumes by this driver.
// Once this method returns, the Flexvol exists, is mounted
func (d *SANEconomyStorageDriver) createFlexvolForLUN(
	ctx context.Context, volumeAttributes *api.Volume, opts map[string]string, storagePool storage.Pool,
) (string, error) {
	var (
		flexvol         = d.FlexvolNamePrefix() + utils.RandomString(10)
		size            = "1g"
		unixPermissions = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy    = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle   = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption      = volumeAttributes.Encrypt
	)

	snapshotReserveInt, err := GetSnapshotReserve(volumeAttributes.SnapshotPolicy,
		storagePool.InternalAttributes()[SnapshotReserve])
	if err != nil {
		return "", fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	Logc(ctx).WithFields(
		LogFields{
			"name":            flexvol,
			"aggregate":       volumeAttributes.Aggregates[0],
			"size":            size,
			"spaceReserve":    volumeAttributes.SpaceReserve,
			"snapshotPolicy":  volumeAttributes.SnapshotPolicy,
			"snapshotReserve": snapshotReserveInt,
			"unixPermissions": unixPermissions,
			"snapshotDir":     volumeAttributes.SnapshotDir,
			"exportPolicy":    exportPolicy,
			"securityStyle":   securityStyle,
			"encryption":      utils.GetPrintableBoolPtrValue(encryption),
		},
	).Debug("Creating Flexvol for LUNs.")

	// Create the flexvol
	err = d.API.VolumeCreate(
		ctx, api.Volume{
			AccessType: "",
			Aggregates: []string{
				volumeAttributes.Aggregates[0],
			},
			Comment:         "",
			Encrypt:         encryption,
			ExportPolicy:    exportPolicy,
			JunctionPath:    "",
			Name:            flexvol,
			Qos:             api.QosPolicyGroup{},
			SecurityStyle:   securityStyle,
			Size:            size,
			SnapshotDir:     false,
			SnapshotPolicy:  volumeAttributes.SnapshotPolicy,
			SnapshotReserve: snapshotReserveInt,
			SpaceReserve:    volumeAttributes.SpaceReserve,
			TieringPolicy:   volumeAttributes.TieringPolicy,
			UnixPermissions: unixPermissions,
			UUID:            "",
			DPVolume:        false,
		})

	if err != nil {
		return "", fmt.Errorf("error creating volume: %v", err)
	}

	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	if !volumeAttributes.SnapshotDir {
		err := d.API.VolumeDisableSnapshotDirectoryAccess(ctx, flexvol)
		if err != nil {
			if err := d.API.VolumeDestroy(ctx, flexvol, true); err != nil {
				Logc(ctx).Error(err)
			}
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
	ctx context.Context, volumeAttributes *api.Volume, sizeBytes uint64, shouldLimitFlexvolSize bool,
	flexvolSizeLimit uint64, ignoredVols map[string]struct{},
) (string, error) {
	// Get all volumes matching the specified attributes
	volumes, err := d.API.VolumeListByAttrs(ctx, volumeAttributes)
	if err != nil {
		return "", fmt.Errorf("error listing volumes; %v", err)
	}

	// Weed out the Flexvols:
	// 1) already having too many LUNs
	// 2) exceeding size limits
	var eligibleVolumes []string
	for _, volume := range volumes {
		volName := volume.Name
		// skip flexvols over the size limit
		if shouldLimitFlexvolSize {
			sizeWithRequest, err := d.getOptimalSizeForFlexvol(ctx, volName, sizeBytes)
			if err != nil {
				Logc(ctx).Errorf("Error checking size for existing LUN %v: %v", volName, err)
				continue
			}
			if sizeWithRequest > flexvolSizeLimit {
				Logc(ctx).Debugf("Flexvol size for %v is over the limit of %v", volName, flexvolSizeLimit)
				continue
			}
		}

		count := 0
		lunPathPattern := fmt.Sprintf("/vol/%s/*", volName)
		luns, err := d.API.LunList(ctx, lunPathPattern)
		if err != nil {
			return "", fmt.Errorf("error enumerating LUNs for volume %v: %v", volName, err)
		}

		for _, lunInfo := range luns {
			lunPath := GetLUNPathEconomy(volName, lunInfo.Name)
			if !d.helper.IsValidSnapLUNPath(lunPath) {
				count++
			}
		}

		if count < d.lunsPerFlexvol {
			// Only add to the list if the volume is not being explicitly ignored
			if _, ignored := ignoredVols[volName]; !ignored {
				eligibleVolumes = append(eligibleVolumes, volName)
			}
		}
	}

	// Pick a Flexvol.  If there are multiple matches, pick one at random.
	switch len(eligibleVolumes) {
	case 0:
		return "", nil
	case 1:
		return eligibleVolumes[0], nil
	default:
		return eligibleVolumes[crypto.GetRandomNumber(len(eligibleVolumes))], nil
	}
}

// getOptimalSizeForFlexvol sums up all the LUN sizes on a Flexvol and adds the size of
// the new LUN being added as well as the current Flexvol snapshot reserve.  This value may be used
// to grow the Flexvol as new LUNs are being added.
func (d *SANEconomyStorageDriver) getOptimalSizeForFlexvol(
	ctx context.Context, flexvol string, newLunSizeBytes uint64,
) (uint64, error) {
	// Get more info about the Flexvol
	volAttrs, err := d.API.VolumeInfo(ctx, flexvol)
	if err != nil {
		return 0, err
	}
	totalDiskLimitBytes, err := d.getTotalLUNSize(ctx, flexvol)
	if err != nil {
		return 0, err
	}

	flexvolSizeBytes := calculateFlexvolEconomySizeBytes(ctx, flexvol, volAttrs, newLunSizeBytes, totalDiskLimitBytes)

	return flexvolSizeBytes, nil
}

// getLUNSize returns the size of the LUN
func (d *SANEconomyStorageDriver) getLUNSize(ctx context.Context, name, flexvol string) (uint64, error) {
	lunTarget := GetLUNPathEconomy(flexvol, name)
	lun, err := d.API.LunGetByName(ctx, lunTarget)
	if err != nil {
		return 0, err
	}

	lunSize, err := strconv.ParseUint(lun.Size, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error determining LUN size: %v ", err)
	}
	if lunSize == 0 {
		return 0, fmt.Errorf("unable to determine LUN size")
	}
	return lunSize, nil
}

// getTotalLUNSize returns the sum of all LUN sizes on a Flexvol
func (d *SANEconomyStorageDriver) getTotalLUNSize(ctx context.Context, flexvol string) (uint64, error) {
	lunPathPattern := fmt.Sprintf("/vol/%s/*", flexvol)
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		return 0, fmt.Errorf("error enumerating LUNs for volume %v: %v", flexvol, err)
	}

	var totalDiskLimit uint64

	for _, lun := range luns {
		diskLimitSize, _ := strconv.ParseUint(lun.Size, 10, 64)
		if diskLimitSize == 0 {
			return 0, fmt.Errorf("unable to determine LUN size")
		}
		totalDiskLimit += diskLimitSize
	}
	return totalDiskLimit, nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *SANEconomyStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *SANEconomyStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

func (d *SANEconomyStorageDriver) getStoragePoolAttributes() map[string]sa.Offer {
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(false),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *SANEconomyStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *SANEconomyStorageDriver) GetInternalVolumeName(_ context.Context, name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *SANEconomyStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	if !volConfig.ImportNotManaged && tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		// All new CSI ONTAP SAN volumes start with publish enforcement on, unless they're unmanaged imports
		volConfig.AccessInfo.PublishEnforcement = true
	}
	createPrepareCommon(ctx, d, volConfig)
}

func (d *SANEconomyStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"Method":       "CreateFollowup",
		"Type":         "SANEconomyStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")
	Logc(ctx).Debug("No follow-up create actions for ontap-san-economy volume.")

	return nil
}

func (d *SANEconomyStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANEconomyStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *SANEconomyStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SANEconomyStorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {
	_, flexvol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		return nil, err
	}

	volumeAttrs, err := d.API.VolumeInfo(ctx, name)
	if err != nil {
		return nil, err
	}

	lunPath := GetLUNPathEconomy(flexvol, name)
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
func (d *SANEconomyStorageDriver) GetVolumeExternalWrappers(
	ctx context.Context, channel chan *storage.VolumeExternalWrapper,
) {
	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumes, err := d.API.VolumeListByPrefix(ctx, d.FlexvolNamePrefix())
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Bail out early if there aren't any Flexvols

	if len(volumes) == 0 {
		return
	}

	// Get all LUNs in volumes matching the storage prefix
	lunPathPattern := fmt.Sprintf("/vol/%v/*", d.flexvolNamePrefix+"*")
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with LUNs
	volumeMap := make(map[string]api.Volume)
	for _, volume := range volumes {
		volumeMap[volume.Name] = *volume
	}

	// Convert all LUNs to VolumeExternal and write them to the channel

	for _, lun := range luns {

		lun := lun
		lunPath := GetLUNPathEconomy(lun.VolumeName, lun.Name)
		volume, ok := volumeMap[lun.VolumeName]
		if !ok {
			Logc(ctx).WithField("path", lunPath).Warning("Flexvol not found for LUN.")
			continue
		}

		// Skip over LUNs which are snapshots of other LUNs
		if d.helper.GetSnapshotNameFromSnapLUNPath(lunPath) != "" {
			Logc(ctx).WithField("path", lunPath).Debug("Skipping LUN snapshot.")
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&lun, &volume), Error: nil}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SANEconomyStorageDriver) getVolumeExternal(
	lun *api.Lun, volume *api.Volume,
) *storage.VolumeExternal {
	externalVolumeName := d.helper.GetExternalVolumeNameFromPath(lun.Name)
	internalVolumeName := d.helper.GetInternalVolumeNameFromPath(lun.Name)

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            externalVolumeName,
		InternalName:    internalVolumeName,
		Size:            lun.Size,
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volume.SnapshotPolicy,
		ExportPolicy:    "",
		SnapshotDir:     "",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      utils.VolumeAccessInfo{},
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
func (d *SANEconomyStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANEconomyStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.DataLIF != dOrig.Config.DataLIF {
		bitmap.Add(storage.InvalidVolumeAccessInfoChange)
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

// LUNExists returns true if the named LUN exists across all buckets.  This should be called with the
// actual LUN name, i.e. the internal volume name or snap-LUN name.
func (d *SANEconomyStorageDriver) LUNExists(ctx context.Context, name, bucketPrefix string) (bool, string, error) {
	Logc(ctx).WithFields(
		LogFields{
			"name":         name,
			"bucketPrefix": bucketPrefix,
		},
	).Debug("Checking if LUN exists.")

	lunPathPattern := fmt.Sprintf("/vol/%s*/%s", bucketPrefix, name)
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		return false, "", err
	}

	for _, lun := range luns {
		Logc(ctx).WithFields(
			LogFields{
				"lun.Name": lun.Name,
			},
		).Debug("LUNExists")
		if strings.HasSuffix(lun.Name, name) {
			Logc(ctx).WithField("flexvol", lun.VolumeName).Debug("Found LUN.")
			return true, lun.VolumeName, nil
		}
	}

	return false, "", nil
}

// Resize a LUN with the specified options and find (or create) a bucket volume for the LUN
func (d *SANEconomyStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":    "Resize",
		"Type":      "SANEconomyStorageDriver",
		"name":      name,
		"sizeBytes": sizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	// Generic user-facing message
	resizeError := errors.New("storage driver failed to resize the volume")

	// Validation checks
	// get the volume where the lun exists
	exists, bucketVol, err := d.LUNExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing volume: %v", err)
		return resizeError
	}
	if !exists {
		return fmt.Errorf("error LUN %s does not exist", name)
	}

	// Calculate the delta size needed to resize the bucketVol
	totalLunSize, err := d.getTotalLUNSize(ctx, bucketVol)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Failed to determine total LUN size")
		return resizeError
	}

	currentLunSize, err := d.getLUNSize(ctx, name, bucketVol)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Failed to determine LUN size")
		return resizeError
	}

	flexvolSize, err := d.getOptimalSizeForFlexvol(ctx, bucketVol, sizeBytes-currentLunSize)
	if err != nil {
		Logc(ctx).Warnf("Could not calculate optimal Flexvol size. %v", err)
		flexvolSize = totalLunSize + sizeBytes - currentLunSize
	}

	sameSize := utils.VolumeSizeWithinTolerance(
		int64(flexvolSize), int64(totalLunSize), tridentconfig.SANResizeDelta,
	)

	if sameSize {
		Logc(ctx).WithFields(
			LogFields{
				"requestedSize":     flexvolSize,
				"currentVolumeSize": totalLunSize,
				"name":              name,
				"delta":             tridentconfig.SANResizeDelta,
			},
		).Info(
			"Requested size and current volume size are within the delta and therefore considered the same size for" +
				" SAN resize operations.",
		)
		volConfig.Size = strconv.FormatUint(currentLunSize, 10)
		return nil
	}

	if flexvolSize < totalLunSize {
		return fmt.Errorf("requested size %d is less than existing volume size %d", flexvolSize, totalLunSize)
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
		ctx, bucketVol, flexvolSize, d.Config, d.GetAPI(),
	); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, flexvolSize, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Resize operations
	lunPath := d.helper.GetLUNPath(bucketVol, name)
	if !d.API.SupportsFeature(ctx, api.LunGeometrySkip) {
		// Check LUN geometry and verify LUN max size.
		lunMaxSize, err := d.API.LunGetGeometry(ctx, lunPath)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}

		if lunMaxSize < sizeBytes {
			Logc(ctx).WithFields(
				LogFields{
					"error":      err,
					"sizeBytes":  sizeBytes,
					"lunMaxSize": lunMaxSize,
					"lunPath":    lunPath,
				},
			).Error("Requested size is larger than LUN's maximum capacity.")
			return errors.UnsupportedCapacityRangeError(fmt.Errorf(
				"volume resize failed as requested size is larger than LUN's maximum capacity"))
		}
	}

	// Resize FlexVol
	err = d.API.VolumeSetSize(ctx, bucketVol, strconv.FormatUint(flexvolSize, 10))
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")

	}

	// Resize LUN
	returnSize, err := d.API.LunSetSize(ctx, lunPath, strconv.FormatUint(sizeBytes, 10))
	if err != nil {
		Logc(ctx).WithField("error", err).Error("LUN resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	volConfig.Size = strconv.FormatUint(returnSize, 10)

	// Resize Flexvol to be the same size or bigger than LUN because ONTAP creates
	// larger LUNs sometimes based on internal geometry
	if returnSize > sizeBytes {
		err = d.resizeFlexvol(ctx, bucketVol, 0)
		if err != nil {
			Logc(ctx).WithFields(
				LogFields{
					"name":               bucketVol,
					"initialVolumeSize":  flexvolSize,
					"adjustedVolumeSize": flexvolSize + returnSize - sizeBytes,
				},
			).Warning("Failed to resize new volume to exact sum of LUNs' size.")
		} else {
			if adjustedVolumeSize, err := d.API.VolumeSize(ctx, bucketVol); err != nil {
				Logc(ctx).WithField("name", bucketVol).
					Warning("Failed to get volume size after the second resize operation.")
			} else {
				Logc(ctx).WithFields(
					LogFields{
						"name":               bucketVol,
						"initialVolumeSize":  flexvolSize,
						"adjustedVolumeSize": adjustedVolumeSize,
					},
				).Debug("FlexVol resized.")
			}
		}
	}
	Logc(ctx).WithField("size", returnSize).Debug("Returning.")

	return nil
}

// resizeFlexvol grows or shrinks the Flexvol to an optimal size if possible. Otherwise
// the Flexvol is expanded by the value of sizeBytes
func (d *SANEconomyStorageDriver) resizeFlexvol(ctx context.Context, flexvol string, sizeBytes uint64) error {
	flexvolSizeBytes, err := d.getOptimalSizeForFlexvol(ctx, flexvol, sizeBytes)
	if err != nil {
		Logc(ctx).Warnf("Could not calculate optimal Flexvol size. %v", err)
		// Lacking the optimal size, just grow the Flexvol to contain the new LUN
		size := strconv.FormatUint(sizeBytes, 10)
		err := d.API.VolumeSetSize(ctx, flexvol, "+"+size)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	} else {
		// Got optimal size, so just set the Flexvol to that value
		flexvolSizeStr := strconv.FormatUint(flexvolSizeBytes, 10)
		err := d.API.VolumeSetSize(ctx, flexvol, flexvolSizeStr)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}
	return nil
}

func (d *SANEconomyStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*utils.Node, backendUUID, tridentUUID string,
) error {
	// Discover known nodes
	nodeNames := make([]string, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "SANEconomyStorageDriver",
		"Nodes":  nodeNames,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return reconcileSANNodeAccess(ctx, d.API, nodeNames, backendUUID, tridentUUID)
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *SANEconomyStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "iscsi", d.GetStorageBackendPhysicalPoolNames(ctx))
}

// String makes SANEconomyStorageDriver satisfy the Stringer interface.
func (d SANEconomyStorageDriver) String() string {
	return utils.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes SANEconomyStorageDriver satisfy the GoStringer interface.
func (d SANEconomyStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d SANEconomyStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func (d *SANEconomyStorageDriver) GetChapInfo(_ context.Context, _, _ string) (*utils.IscsiChapInfo, error) {
	return &utils.IscsiChapInfo{
		UseCHAP:              d.Config.UseCHAP,
		IscsiUsername:        d.Config.ChapUsername,
		IscsiInitiatorSecret: d.Config.ChapInitiatorSecret,
		IscsiTargetUsername:  d.Config.ChapTargetUsername,
		IscsiTargetSecret:    d.Config.ChapTargetInitiatorSecret,
	}, nil
}

// EnablePublishEnforcement prepares a volume for per-node igroup mapping allowing greater access control.
func (d *SANEconomyStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	internalName := volume.Config.InternalName
	exists, flexVol, err := d.LUNExists(ctx, internalName, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"flexVol":    flexVol,
			"lunName":    internalName,
			"volumeName": volume.Config.Name,
		}).Errorf("Error checking for existing LUN: %v", err)
		return err
	}
	if !exists {
		return fmt.Errorf("error LUN %v does not exist", internalName)
	}
	return EnableSANPublishEnforcement(ctx, d.GetAPI(), volume.Config, GetLUNPathEconomy(flexVol, internalName))
}

func (d *SANEconomyStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}
