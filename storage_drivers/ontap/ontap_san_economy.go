// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/RoaringBitmap/roaring/v2"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

var LUNInternalIDRegex = regexp.MustCompile(`^/svm/(?P<svm>[^/]+)/flexvol/(?P<flexvol>[^/]+)/lun/(?P<lun>[^/]+)$`)

const (
	maxLunNameLength       = 254
	minLUNsPerFlexvol      = 50
	defaultLUNsPerFlexvol  = 100
	maxLUNsPerFlexvol      = 200
	snapshotNameSeparator  = "_snapshot_"
	snapshotCopyNameSuffix = "_copy"
)

// GetLUNPathEconomy accepts a "bucketName" (the flexVol in ONTAP), the internal volume name (the LUN in ONTAP),
// and returns the LUN path.
// Example:
//
//	"/vol/trident_lun_pool_MACNWIILZO/pvc_954876af_3a32_4ca4_b507_d5b5424e337e"
func GetLUNPathEconomy(bucketName, volNameInternal string) string {
	return fmt.Sprintf("/vol/%s/%s", bucketName, volNameInternal)
}

// GetEconomyLUNPathInSnapshot accepts a "bucketName" (the flexVol in ONTAP), a flexVol snapshot name, and the name of a
// target LUN and returns the complete path of that LUN within the snapshot.
// Example:
//
//	"/vol/trident_lun_pool_MACNWIILZO/.snapshot/snap.2025-07-29_161024/pvc_954876af_3a32_4ca4_b507_d5b5424e337e"
func GetEconomyLUNPathInSnapshot(bucketName, snapName, lunName string) string {
	return fmt.Sprintf("/vol/%s/.snapshot/%s/%s", bucketName, snapName, lunName)
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

// parameter: volName=my-Vol
// output: /vol/*/storagePrefix_my_Vol_snapshot_*
func (o *LUNHelper) GetSnapPathPatternForVolume(externalVolumeName string) string {
	externalVolumeName = strings.ReplaceAll(externalVolumeName, "-", "_")
	snapPattern := fmt.Sprintf("/vol/*/*%v"+snapshotNameSeparator+"*", externalVolumeName)
	return snapPattern
}

// parameter: volName=my-Lun
// output: storagePrefix_my_Lun
// parameter: volName=storagePrefix_my-Lun
// output: storagePrefix_my_Lun
func (o *LUNHelper) GetInternalVolumeName(volName string) string {
	return strings.ReplaceAll(volName, "-", "_")
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

// IsValidSnapLUNPath identifies if the given snapLunPath has a valid snapshot name
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
	initialized         bool
	Config              drivers.OntapStorageDriverConfig
	ips                 []string
	API                 api.OntapAPI
	AWSAPI              awsapi.AWSAPI
	telemetry           *Telemetry
	flexvolNamePrefix   string
	helper              *LUNHelper
	lunsPerFlexvol      int
	denyNewFlexvols     bool
	iscsi               iscsi.ISCSI
	flexvolLocks        *locks.GCNamedMutex // Locks for FlexVol-level operations
	flexvolCreationLock sync.Mutex          // Global lock for FlexVol find-or-create decisions

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool
}

// lockFlexvol locks the specified FlexVol and returns a locked wrapper
func (d *SANEconomyStorageDriver) lockFlexvol(name string) *locks.LockedResource {
	return d.flexvolLocks.LockWithGuard(name)
}

func (d *SANEconomyStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

func (d *SANEconomyStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
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

	// Initialize the iSCSI client
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
			return fmt.Errorf("error initializing %s driver: %w", d.Name(), err)
		}

		d.Config = *config
	}

	// Initialize AWS API if applicable.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.AWSAPI == nil {
		d.AWSAPI, err = initializeAWSDriver(ctx, &d.Config)
		if err != nil {
			return fmt.Errorf("error initializing %s AWS driver; %w", d.Name(), err)
		}
	}

	// Initialize the ONTAP API.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = InitializeOntapDriver(ctx, &d.Config); err != nil {
			return fmt.Errorf("error initializing %s driver: %w", d.Name(), err)
		}
	}

	// Load default config parameters
	if err = PopulateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %w", err)
	}

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
	d.denyNewFlexvols, _ = strconv.ParseBool(d.Config.DenyNewVolumePools)

	// ensure lun cap is valid
	if d.Config.LUNsPerFlexvol == "" {
		d.lunsPerFlexvol = defaultLUNsPerFlexvol
	} else {
		errstr := "invalid config value for lunsPerFlexvol"
		if d.lunsPerFlexvol, err = strconv.Atoi(d.Config.LUNsPerFlexvol); err != nil {
			return fmt.Errorf("%s: %w", errstr, err)
		}
		if d.lunsPerFlexvol < minLUNsPerFlexvol {
			return fmt.Errorf(
				"%s: using %d lunsPerFlexvol (minimum is %d)", errstr, d.lunsPerFlexvol, minLUNsPerFlexvol,
			)
		} else if d.lunsPerFlexvol > maxLUNsPerFlexvol {
			return fmt.Errorf(
				"%s: using %d lunsPerFlexvol (maximum is %d)", errstr, d.lunsPerFlexvol, maxLUNsPerFlexvol,
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
		return fmt.Errorf("could not configure storage pools: %w", err)
	}

	err = InitializeSANDriver(ctx, driverContext, d.API, &d.Config, d.validate, backendUUID)
	if err != nil {
		if d.Config.DriverContext == tridentconfig.ContextCSI {
			// Clean up igroup for failed driver.
			if igroupErr := d.API.IgroupDestroy(ctx, d.Config.IgroupName); igroupErr != nil {
				Logc(ctx).WithError(igroupErr).WithField("igroup", d.Config.IgroupName).Warn("Error deleting igroup.")
			}
		}
		return fmt.Errorf("error initializing %s driver: %w", d.Name(), err)
	}

	// Identify non-overlapping storage backend pools on the driver backend.
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %w", err)
	}
	d.Config.BackendPools = pools

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	// Initialize FlexVol-level locks for concurrency protection
	d.flexvolLocks = locks.NewGCNamedMutex()

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

	d.API.Terminate()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *SANEconomyStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "SANEconomyStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := ValidateSANDriver(ctx, &d.Config, d.ips, d.iscsi); err != nil {
		return fmt.Errorf("driver validation failed: %w", err)
	}

	if err := ValidateStoragePrefixEconomy(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d, 0); err != nil {
		return fmt.Errorf("storage pool validation failed: %w", err)
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
	createError := errors.New("volume creation failed")

	// Determine a way to see if the volume already exists
	exists, existsInFlexvol, err := d.LUNExists(ctx, name, "", d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing volume: %w", err)
		return createError
	}
	if exists {
		Logc(ctx).WithFields(LogFields{"LUN": name, "bucketVol": existsInFlexvol}).Debug("LUN already exists.")
		// If LUN exists, update the volConfig.InternalID in case it was not set.  This is useful for
		// "legacy" volumes which do not have InternalID set when they were created.
		if volConfig.InternalID == "" {
			volConfig.InternalID = d.CreateLUNInternalID(d.Config.SVM, existsInFlexvol, name)
			Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("Setting InternalID.")
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Get candidate physical pools
	physicalPools, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
	if err != nil {
		return err
	}

	// Determine volume size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %w", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %w", volConfig.Size, err)
	}
	sizeBytes = GetVolumeSize(sizeBytes, storagePool.InternalAttributes()[Size])

	// Ensure LUN name isn't too long
	if len(name) > maxLunNameLength {
		return fmt.Errorf("volume %s name exceeds the limit of %d characters", name, maxLunNameLength)
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get Flexvol options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	spaceAllocation, _ := strconv.ParseBool(
		collection.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation]),
	)
	var (
		spaceReserve      = collection.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = collection.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = collection.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = collection.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy      = collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = collection.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = collection.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = collection.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		formatOptions     = collection.GetV(opts, "formatOptions", storagePool.InternalAttributes()[FormatOptions])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
		luksEncryption    = storagePool.InternalAttributes()[LUKSEncryption]
	)

	// Add a constant overhead for LUKS volumes to account for LUKS metadata. This overhead is
	// part of the LUN but is not reported to the orchestrator.
	reportedSize := sizeBytes
	sizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, sizeBytes, luksEncryption)

	lunSize := strconv.FormatUint(sizeBytes, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %w", err)
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %w", err)
	}

	// Check for a supported file system type
	fstype, err := drivers.CheckSupportedFilesystem(
		ctx, collection.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name,
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
	volConfig.FormatOptions = formatOptions

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
			createErrors = append(createErrors, errors.New(errMessage))

			// Move on to the next pool
			continue
		}

		var (
			bucketVol     string
			lunPathEco    string
			newVol        bool
			lockedFlexvol *locks.LockedResource
		)

		// Track FlexVols that have already been tried (to avoid retrying the same one)
		ignoredVols := make(map[string]struct{})

		// Retry loop for finding/creating a FlexVol with capacity
		// This handles the case where ONTAP's LUN limit is reached during creation
		var lunCreated bool

		for !lunCreated {
			volAttrs := &api.Volume{
				Aggregates:      []string{aggregate},
				Encrypt:         enableEncryption,
				Name:            d.FlexvolNamePrefix() + "*",
				SnapshotPolicy:  snapshotPolicy,
				SpaceReserve:    spaceReserve,
				SnapshotReserve: snapshotReserveInt,
				TieringPolicy:   tieringPolicy,
			}

			// Lock Hierarchy Rule: Never hold per-FlexVol lock while trying to acquire flexvolCreationMux lock
			// ensureFlexvolForLUN() releases flexvolCreationMux lock BEFORE acquiring per-FlexVol lock
			// Caller receives per-FlexVol lock already acquired
			// Caller NEVER tries to call ensureFlexvolForLUN() again while holding per-FlexVol lock
			// In retry scenario (TooManyLunsError), caller explicitly unlocks per-FlexVol lock BEFORE calling ensureFlexvolForLUN() again

			// Make sure we have a Flexvol for the new LUN (with lock)
			lockedFlexvol, newVol, err = d.ensureFlexvolForLUN(
				ctx, volAttrs, sizeBytes, opts, &d.Config, storagePool, ignoredVols,
			)
			if err != nil {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN-ECONOMY pool %s/%s; BucketVol location/creation failed %s: %v",
					storagePool.Name(), aggregate, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, errors.New(errMessage))
				break // Try next aggregate
			}
			bucketVol = lockedFlexvol.Name()

			// Grow or shrink the Flexvol as needed (lock is held)
			if err = d.resizeFlexvol(ctx, bucketVol, sizeBytes); err != nil {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN-ECONOMY pool %s/%s; Flexvol resize failed %s/%s: %v",
					storagePool.Name(), aggregate, bucketVol, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, errors.New(errMessage))

				// Don't leave the new Flexvol around if we just created it
				if newVol {
					if err := d.API.VolumeDestroy(ctx, bucketVol, true, true); err != nil {
						Logc(ctx).WithField("volume", bucketVol).WithError(err).Error("Could not clean up volume.")
					} else {
						Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after resize error.")
					}
				}
				lockedFlexvol.Unlock() // Release lock before trying next aggregate
				break                  // Try next aggregate
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
					SpaceReserved:  convert.ToPtr(false),
					SpaceAllocated: convert.ToPtr(spaceAllocation),
				})
			if err != nil {
				if api.IsTooManyLunsError(err) {
					Logc(ctx).WithError(err).Warn(
						"ONTAP limit for LUNs/Flexvol reached; retrying with different Flexvol")
					ignoredVols[bucketVol] = struct{}{}
					lockedFlexvol.Unlock() // Explicit unlock before retry
					// NOTE: With lock-before-check pattern, this race is rare but still possible
					// if LUNs are created between check and LunCreate. Simply retry - the
					// next call to ensureFlexvolForLUN will skip this FlexVol via ignoredVols.
					continue // Retry with next FlexVol
				}

				errMessage := fmt.Sprintf(
					"ONTAP-SAN-ECONOMY pool %s/%s; error creating LUN %s/%s: %v",
					storagePool.Name(), aggregate, bucketVol, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, errors.New(errMessage))

				// Don't leave the new Flexvol around if we just created it
				if newVol {
					if err := d.API.VolumeDestroy(ctx, bucketVol, true, true); err != nil {
						Logc(ctx).WithField("volume", bucketVol).WithError(err).Error("Could not clean up volume.")
					} else {
						Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after LUN create error.")
					}
				}
				lockedFlexvol.Unlock() // Release lock before trying next aggregate
				break                  // Try next aggregate
			}

			// LUN created successfully
			lunCreated = true
		}

		// If we exhausted retries or hit an error, move to next aggregate
		if !lunCreated {
			continue
		}

		// Ensure the FlexVol lock is released when we're done (after all operations on this FlexVol)
		// Note: lockedFlexvol.Unlock() is idempotent (safe to call multiple times)
		defer lockedFlexvol.Unlock()

		actualSize, err := d.getLUNSize(ctx, name, bucketVol)
		if err != nil {
			Logc(ctx).WithError(err).Error("Failed to determine LUN size")
			return err
		}
		// Remove LUKS metadata size from actual size of LUN
		actualSize = decrementWithLUKSMetadataIfLUKSEnabled(ctx, actualSize, luksEncryption)
		volConfig.Size = strconv.FormatUint(actualSize, 10)

		// Save the fstype in a LUN attribute so we know what to do in Attach
		err = d.API.LunSetAttribute(ctx, lunPathEco, LUNAttributeFSType, fstype, string(d.Config.DriverContext),
			luksEncryption, formatOptions)
		if err != nil {

			errMessage := fmt.Sprintf(
				"ONTAP-SAN-ECONOMY pool %s/%s; error saving file system type for LUN %s/%s: %v",
				storagePool.Name(), aggregate, bucketVol, name, err,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))

			// Don't leave the new LUN around
			if err = d.API.LunDestroy(ctx, lunPathEco); err != nil {
				Logc(ctx).WithField("LUN", lunPathEco).WithError(err).Error("Could not clean up LUN.")
			} else {
				Logc(ctx).WithField("volume", name).Debugf("Cleaned up LUN after set attribute error.")
			}

			// Don't leave the new Flexvol around if we just created it
			if newVol {
				if err := d.API.VolumeDestroy(ctx, bucketVol, true, true); err != nil {
					Logc(ctx).WithField("volume", bucketVol).WithError(err).Error("Could not clean up volume.")
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after set attribute error.")
				}
			}

			// Release lock before moving to next aggregate
			lockedFlexvol.Unlock()
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

		volConfig.InternalID = d.CreateLUNInternalID(d.Config.SVM, bucketVol, name)

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

	internalID, err := d.createLUNClone(
		ctx, name, source, snapshot, &d.Config, d.API, d.FlexvolNamePrefix(), isFromSnapshot, qosPolicyGroup,
	)
	if err != nil {
		return err
	}

	cloneVolConfig.InternalID = internalID

	return nil
}

// cloneLUNFromSnapshot clones a LUN out of an ONTAP snapshot and returns the internalID (svm path) of that new LUN.
func (d *SANEconomyStorageDriver) cloneLUNFromSnapshot(
	ctx context.Context, bucketVolume, sourcePath, lunName string, qosPolicyGroup api.QosPolicyGroup,
) (string, error) {
	fields := LogFields{
		"Method":     "cloneLUNFromSnapshot",
		"Type":       "SANEconomyStorageDriver",
		"parentVol":  bucketVolume,
		"sourcePath": sourcePath,
		"lunName":    lunName,
	}
	Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> cloneLUNFromSnapshot")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< cloneLUNFromSnapshot")

	// Lock the FlexVol to ensure atomic clone and resize operations
	lockedFlexvol := d.lockFlexvol(bucketVolume)
	defer lockedFlexvol.Unlock()

	// Create the clone based on given snapshot (lock is held)
	if err := d.API.LunCloneCreate(ctx, bucketVolume, sourcePath, lunName, qosPolicyGroup); err != nil {
		return "", err
	}
	internalID := d.CreateLUNInternalID(d.Config.SVM, bucketVolume, lunName)

	// Grow or shrink the FlexVol as needed (lock is still held)
	if err := d.resizeFlexvol(ctx, bucketVolume, 0); err != nil {
		return "", err
	}

	return internalID, nil
}

// Create a volume clone
func (d *SANEconomyStorageDriver) createLUNClone(
	ctx context.Context, lunName, source, snapshot string, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, prefix string, isFromSnapLUN bool, qosPolicyGroup api.QosPolicyGroup,
) (string, error) {
	fields := LogFields{
		"Method":         "createLUNClone",
		"Type":           "SANEconomyStorageDriver",
		"lunName":        lunName,
		"source":         source,
		"snapshot":       snapshot,
		"prefix":         prefix,
		"isFromSnapLUN":  isFromSnapLUN,
		"qosPolicyGroup": qosPolicyGroup,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createLUNClone")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createLUNClone")

	// If the specified LUN copy already exists, return an error
	destinationLunExists, _, err := d.LUNExists(ctx, lunName, "", prefix)
	if err != nil {
		return "", fmt.Errorf("error checking for existing destination LUN: %w", err)
	}
	if destinationLunExists {
		return "", fmt.Errorf("destination LUN %s already exists", lunName)
	}

	// Check if called from CreateClone and is from a snapshot LUN.
	if isFromSnapLUN {
		source = d.helper.GetSnapshotName(source, snapshot)
	}

	// If the source doesn't exist, return an error
	sourceLunExists, flexvol, err := d.LUNExists(ctx, source, "", prefix)
	if err != nil {
		return "", fmt.Errorf("error checking for existing source LUN: %w", err)
	}
	if !sourceLunExists {
		return "", errors.NotFoundError("source LUN %s does not exist", source)
	}

	sourceLunSizeBytes, lunSizeErr := d.getLUNSize(ctx, source, flexvol)
	if lunSizeErr != nil {
		Logc(ctx).WithError(err).Error("Failed to determine LUN size")
		return "", lunSizeErr
	}

	// Lock the source FlexVol to ensure atomic clone and resize operations
	locked := d.lockFlexvol(flexvol)
	defer locked.Unlock()

	shouldLimitFlexvolSize, flexvolSizeLimit, checkFlexvolSizeLimitsError := CheckVolumePoolSizeLimits(
		ctx, sourceLunSizeBytes, config)
	if checkFlexvolSizeLimitsError != nil {
		return "", checkFlexvolSizeLimitsError
	}

	if shouldLimitFlexvolSize {
		sizeWithRequest, flexvolSizeErr := d.getOptimalSizeForFlexvol(ctx, flexvol, sourceLunSizeBytes)
		if flexvolSizeErr != nil {
			return "", fmt.Errorf("could not calculate optimal Flexvol size; %w", flexvolSizeErr)
		}
		if sizeWithRequest > flexvolSizeLimit {
			return "", errors.UnsupportedCapacityRangeError(fmt.Errorf(
				"requested size %d would exceed the pool size limit %d", sourceLunSizeBytes, flexvolSizeLimit))
		}
	}

	// Create the clone based on given LUN (lock is held)
	// For the ONTAP SAN economy driver, we set the QoS policy at the LUN layer.
	if err = client.LunCloneCreate(ctx, flexvol, source, lunName, qosPolicyGroup); err != nil {
		return "", err
	}

	internalID := d.CreateLUNInternalID(config.SVM, flexvol, lunName)

	// Grow or shrink the Flexvol as needed (lock is still held)
	if err = d.resizeFlexvol(ctx, flexvol, 0); err != nil {
		return "", err
	}

	return internalID, nil
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
		"noRename":     volConfig.ImportNoRename,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	pathElements := strings.Split(originalName, "/")
	if len(pathElements) < 2 {
		return fmt.Errorf("%s is not a valid import vol/LUN path", originalName)
	}
	originalFlexvolName := pathElements[0]
	originalLUNName := pathElements[1]

	flexvol, err := d.API.VolumeInfo(ctx, originalFlexvolName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return errors.NotFoundError("volume %s from vol/LUN %s is not found", originalFlexvolName, originalName)
	}

	// Validate the volume is what it should be.
	if flexvol.AccessType != "" && flexvol.AccessType != "rw" {
		Logc(ctx).WithField("originalName", originalFlexvolName).Error("Could not import volume, type is not rw.")
		return fmt.Errorf("volume %s type is %s, expects rw", originalFlexvolName, flexvol.AccessType)
	}

	// Set the volume to LUKS if backend has LUKS set to true as default.
	if volConfig.LUKSEncryption == "" {
		volConfig.LUKSEncryption = d.Config.LUKSEncryption
	}

	// Ensure LUN found.
	extantLUN, err := d.API.LunGetByName(ctx, "/vol/"+originalFlexvolName+"/"+originalLUNName)
	if err != nil {
		return err
	} else if extantLUN == nil {
		return errors.NotFoundError("LUN %s not found in volume %s", originalLUNName, originalFlexvolName)
	}
	Logc(ctx).WithField("LUN", extantLUN.Name).Trace("Import - LUN found.")

	// LUN should be online.
	if extantLUN.State != "online" {
		return fmt.Errorf("LUN %s is not online", extantLUN.Name)
	}
	volConfig.Size = extantLUN.Size

	if convert.ToBool(volConfig.LUKSEncryption) {
		newSize, err := subtractUintFromSizeString(volConfig.Size, luks.MetadataSize)
		if err != nil {
			return err
		}
		volConfig.Size = newSize
	}

	// Lock the FlexVol to prevent race conditions during import operations.
	// This prevents concurrent Create/Delete/Resize/Clone operations from
	// interfering with LUN rename, FlexVol rename, and igroup unmapping.
	lockedFlexvol := d.lockFlexvol(originalFlexvolName)
	defer lockedFlexvol.Unlock()

	if volConfig.ImportNotManaged {
		// Volume/LUN import is not managed by Trident
		if !strings.HasPrefix(flexvol.Name, d.FlexvolNamePrefix()) {
			// Reject import if the Flexvol is not following naming conventions.
			return fmt.Errorf("could not import volume/LUN, volume is named incorrectly: %s, expected pattern: %s*",
				flexvol.Name, d.FlexvolNamePrefix())
		}
		// Adjust internalName to LUN name. Trim the Flexvol name.
		volConfig.InternalName = d.helper.GetInternalVolumeNameFromPath(volConfig.InternalName)
		return nil
	}
	var targetPath string

	// Managed import with no rename only supported for csi workflow
	if volConfig.ImportNoRename && d.Config.DriverContext != tridentconfig.ContextDocker {
		if !strings.HasPrefix(flexvol.Name, d.FlexvolNamePrefix()) {
			// Reject import if the Flexvol is not following naming conventions.
			return fmt.Errorf("could not import volume/LUN, volume is named incorrectly: %s, expected pattern: %s*",
				flexvol.Name, d.FlexvolNamePrefix())
		}
		volConfig.InternalName = originalLUNName
		targetPath = "/vol/" + originalFlexvolName + "/" + volConfig.InternalName
		// This is critical so that subsequent operations can find the LUN in case of no rename import
		volConfig.InternalID = d.CreateLUNInternalID(d.Config.SVM, originalFlexvolName, originalLUNName)
	} else {
		// Managed import with rename
		targetPath = "/vol/" + originalFlexvolName + "/" + volConfig.InternalName
		newFlexvolName := d.FlexvolNamePrefix() + crypto.RandomString(10)
		volRenamed := false
		if extantLUN.Name != targetPath {
			// Ensure LUN name isn't too long
			if len(volConfig.InternalName) > maxLunNameLength {
				return fmt.Errorf("volume %s name exceeds the limit of %d characters", volConfig.InternalName,
					maxLunNameLength)
			}

			err = d.API.LunRename(ctx, extantLUN.Name, targetPath)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"path":    extantLUN.Name,
					"newPath": targetPath,
				}).WithError(err).Debug("Could not import volume, renaming LUN failed.")
				return fmt.Errorf("LUN path %s rename failed: %w", extantLUN.Name, err)
			}

			// Rename Flexvol if it does not follow naming convention
			if !strings.Contains(flexvol.Name, d.FlexvolNamePrefix()) {
				err = d.API.VolumeRename(ctx, originalFlexvolName, newFlexvolName)
				if err != nil {
					Logc(ctx).WithFields(LogFields{
						"originalName": originalFlexvolName,
						"newName":      newFlexvolName,
					}).WithError(err).Debug("Could not import volume, rename Flexvol failed.")

					// Restore LUN to old name.
					if renameErr := d.API.LunRename(ctx, targetPath, extantLUN.Name); renameErr != nil {
						Logc(ctx).WithFields(LogFields{
							"originalName": extantLUN.Name,
							"newName":      targetPath,
						}).WithError(renameErr).Warn("Failed restoring LUN name to original name.")
					}
					return fmt.Errorf("volume %s rename failed: %w", originalFlexvolName, err)
				}
				volRenamed = true
			}

			// Update target path if volume was renamed
			if volRenamed {
				targetPath = "/vol/" + newFlexvolName + "/" + volConfig.InternalName
			}
		}
	}

	if err = LunUnmapAllIgroups(ctx, d.GetAPI(), targetPath); err != nil {
		Logc(ctx).WithField("LUN", targetPath).WithError(err).Warn("Unmapping of igroups failed.")
		return fmt.Errorf("failed to unmap igroups for LUN %s: %w", targetPath, err)
	}

	return nil
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

	exists, bucketVol, err := d.LUNExists(ctx, name, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s.", name)
		return err
	}
	if !exists {
		Logc(ctx).Warnf("LUN %s does not exist", name)
		return nil
	}

	lunPathEco := GetLUNPathEconomy(bucketVol, name)
	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			Logc(ctx).WithError(err).Error("Could not get target info.")
			return err
		}

		// Get the LUN ID
		lunID, err = d.API.LunMapInfo(ctx, d.Config.IgroupName, lunPathEco)
		if err != nil {
			return fmt.Errorf("error reading LUN maps for volume %s: %w", name, err)
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

	// Before deleting the LUN, check if a LUN has associated snapshots. If so, delete all associated snapshots
	// Note: DeleteSnapshot acquires its own FlexVol lock, so we don't hold the lock here
	externalVolumeName := d.helper.GetExternalVolumeNameFromPath(lunPathEco)
	snapList, err := d.getSnapshotsEconomy(ctx, name, externalVolumeName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Error enumerating snapshots.")
		return deleteError
	}
	for _, snap := range snapList {
		err = d.DeleteSnapshot(ctx, snap.Config, volConfig)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Snapshot LUN delete failed.")
			return err
		}
	}

	// Lock the FlexVol to prevent race conditions during LUN deletion and FlexVol cleanup
	lockedFlexvol := d.lockFlexvol(bucketVol)
	defer lockedFlexvol.Unlock()

	if err = LunUnmapAllIgroups(ctx, d.GetAPI(), lunPathEco); err != nil {
		msg := "error removing all mappings from LUN"
		Logc(ctx).WithError(err).Error(msg)
		return errors.New(msg)
	}

	if err = d.API.LunDestroy(ctx, lunPathEco); err != nil {
		fields := LogFields{
			"Method": "Destroy",
			"Type":   "SANEconomyStorageDriver",
			"LUN":    lunPathEco,
		}
		Logc(ctx).WithFields(fields).WithError(err).Error("LUN delete failed.")

		return deleteError
	}

	// Check if a bucket volume has no more LUNs. If none left, delete the bucketVol. Else, call for resize
	return d.deleteBucketIfEmpty(ctx, bucketVol)
}

// deleteBucketIfEmpty will check if the given bucket volume is empty, if the bucket is empty it will be deleted.
// Otherwise, it will be resized. Callers must hold the FlexVol lock before calling this method.
func (d *SANEconomyStorageDriver) deleteBucketIfEmpty(ctx context.Context, bucketVol string) error {
	fields := LogFields{
		"Method":    "deleteBucketIfEmpty",
		"Type":      "SANEconomyStorageDriver",
		"bucketVol": bucketVol,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> deleteBucketIfEmpty")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< deleteBucketIfEmpty")

	lunPathPattern := fmt.Sprintf("/vol/%s/*", bucketVol)
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		return fmt.Errorf("error enumerating LUNs for volume %s: %w", bucketVol, err)
	}

	count := len(luns)
	if count == 0 {
		// If volume exists and this is FSx, try the FSx SDK first so that any backup mirror relationship
		// is cleaned up.  If the volume isn't found, then FSx may not know about it yet, so just try the
		// underlying ONTAP delete call.  Any race condition with FSx will be resolved on a retry.
		if d.AWSAPI != nil {
			volConfig := &storage.VolumeConfig{
				InternalName: bucketVol,
			}
			err = destroyFSxVolume(ctx, d.AWSAPI, volConfig, &d.Config)
			if err == nil || !errors.IsNotFoundError(err) {
				return err
			}
		}

		// Delete the bucketVol (lock is held by caller)
		err := d.API.VolumeDestroy(ctx, bucketVol, true, true)
		if err != nil {
			return fmt.Errorf("error destroying volume %s: %w", bucketVol, err)
		}
	} else {
		// Grow or shrink the Flexvol as needed (lock is held by caller)
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
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
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

	exists, bucketVol, err := d.LUNExists(ctx, name, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Error("Error checking for existing LUN.")
		return err
	}
	if !exists {
		return errors.NotFoundError("LUN %s does not exist", name)
	}

	extantLUNPath := d.helper.GetLUNPath(bucketVol, name)
	if volConfig.ImportNotManaged {
		// In case of unmanaged Import, we should not change the name.
		extantLUNPath = GetLUNPathEconomy(bucketVol, name)
	}
	igroupName := d.Config.IgroupName

	// Use the node specific igroup if publish enforcement is enabled and this is for CSI.
	if tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		err = ensureIGroupExists(ctx, d.GetAPI(), igroupName, d.Config.SANType)
		if err != nil {
			return fmt.Errorf("failed to ensure igroup %s exists for publish operation: %w", igroupName, err)
		}
	}

	// Get target info.
	iSCSINodeName, _, err := GetISCSITargetInfo(ctx, d.API, &d.Config)
	if err != nil {
		return err
	}

	err = PublishLUN(ctx, d.API, &d.Config, d.ips, publishInfo, extantLUNPath, igroupName, iSCSINodeName)
	if err != nil {
		return fmt.Errorf("error publishing LUN %s: %w", name, err)
	}
	// Fill in the volume access fields as well.
	volConfig.AccessInfo = publishInfo.VolumeAccessInfo

	// Update the volConfig.InternalID in case it was not set.  This is useful for "legacy"
	// volumes which do not have InternalID set when they were created.
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateLUNInternalID(d.Config.SVM, bucketVol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("Setting InternalID.")
	}

	return nil
}

// Unpublish the volume from the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANEconomyStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
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
	exists, bucketVol, err := d.LUNExists(ctx, name, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s.", name)
		return err
	}
	if !exists {
		// If the LUN doesn't exist at this point, there's nothing left to do for unpublish at this level.
		// However, this scenario could indicate unexpected tampering or unknown states with the backend,
		// so log a warning and return a NotFoundError.
		err := errors.NotFoundError("LUN %s does not exist", name)
		Logc(ctx).WithError(err).Warningf("Unable to unpublish LUN %s.", name)
		return err
	}

	// Attempt to unmap the LUN from the per-node igroup; the lunPath is where the LUN resides within the flexvol.
	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := d.helper.GetLUNPath(bucketVol, name)

	// Acquire locks in the correct order: lunMutex first, then igroupMutex
	lunMutex.Lock(lunPath)
	defer lunMutex.Unlock(lunPath)
	igroupMutex.Lock(igroupName)
	defer igroupMutex.Unlock(igroupName)

	if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath); err != nil {
		return fmt.Errorf("error unmapping LUN %s from igroup %s; %w", lunPath, igroupName, err)
	}

	// Remove igroup from volume config's access Info
	volConfig.AccessInfo.IscsiIgroup = removeIgroupFromIscsiIgroupList(volConfig.AccessInfo.IscsiIgroup, igroupName)

	// Remove igroup if no LUNs are mapped.
	if err := DestroyUnmappedIgroup(ctx, d.API, igroupName); err != nil {
		return fmt.Errorf("error removing empty igroup; %w", err)
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
	exists, bucketVol, err := d.LUNExists(ctx, fullSnapshotName, "", d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing snapshot LUN %s.", fullSnapshotName)
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

	sizeBytes, err := convert.ToPositiveInt64(lunInfo.Size)
	if err != nil {
		return nil, fmt.Errorf("%v is an invalid volume size: %w", lunInfo.Size, err)
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   lunInfo.CreateTime,
		SizeBytes: sizeBytes,
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

	exists, _, err := d.LUNExists(ctx, volConfig.InternalName, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s", volConfig.InternalName)
		return nil, err
	}
	if !exists {
		return nil, errors.NotFoundError("LUN %s does not exist", volConfig.InternalName)
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
		return nil, fmt.Errorf("error enumerating snapshots: %w", err)
	}

	Logc(ctx).Debugf("Found %v snapshots.", len(snapList))
	snapshots := make([]*storage.Snapshot, 0)

	for _, snap := range snapList {
		snapLunPath := snap.Name

		sizeBytes, err := convert.ToPositiveInt64(snap.Size)
		if err != nil {
			return nil, fmt.Errorf("%v is an invalid volume size: %w", snap.Size, err)
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
				Created:   snap.CreateTime,
				SizeBytes: sizeBytes,
				State:     storage.SnapshotStateOnline,
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func (d *SANEconomyStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
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
	exists, bucketVol, err := d.LUNExists(ctx, internalVolumeName, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s", internalVolumeName)
		return nil, err
	}
	if !exists {
		return nil, errors.NotFoundError("LUN %s does not exist", internalVolumeName)
	}

	lunPath := GetLUNPathEconomy(bucketVol, internalVolumeName)

	// If the specified volume doesn't exist, return error
	lunInfo, err := d.API.LunGetByName(ctx, lunPath)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %w", err)
	}

	size := lunInfo.Size

	// Create the snapshot name/string
	lunName := d.helper.GetSnapshotName(snapConfig.VolumeInternalName, internalSnapName)

	// Create the "snap-LUN" where the snapshot is a LUN clone of the source LUN
	_, err = d.createLUNClone(
		ctx, lunName, snapConfig.VolumeInternalName, snapConfig.Name, &d.Config, d.API, d.FlexvolNamePrefix(), false,
		api.QosPolicyGroup{},
	)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %w", err)
	}

	// Fetching list of snapshots to get snapshot creation time
	snapListResponse, err := d.getSnapshotsEconomy(ctx, internalVolumeName, snapConfig.VolumeName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %w", err)
	}

	for _, snap := range snapListResponse {
		Logc(ctx).WithFields(LogFields{
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}).Info("Snapshot created.")

		sizeBytes, err := convert.ToPositiveInt64(size)
		if err != nil {
			return nil, fmt.Errorf("error %v is an invalid volume size: %w", size, err)
		}

		return &storage.Snapshot{
			Config:    snapConfig,
			Created:   snap.Created,
			SizeBytes: sizeBytes,
			State:     storage.SnapshotStateOnline,
		}, nil
	}
	return nil, fmt.Errorf("could not find snapshot %s for source volume %s", internalSnapName, internalVolumeName)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANEconomyStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
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

	// PRELIMINARY check to determine which FlexVol contains the volume LUN.
	// This is necessary to know which lock to acquire. We will re-validate inside the lock.
	volLunExists, volBucketVol, err := d.LUNExists(ctx, volLunName, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing volume LUN %s", volLunName)
		return err
	}
	if !volLunExists {
		message := fmt.Sprintf("volume LUN %s does not exist", volLunName)
		Logc(ctx).Warnf(message)
		return errors.NotFoundError(message)
	}

	// Lock the FlexVol EARLY to prevent TOCTOU races. All subsequent checks and operations
	// are serialized per FlexVol, ensuring atomic multi-step restore operation.
	lockedFlexvol := d.lockFlexvol(volBucketVol)
	defer lockedFlexvol.Unlock()

	// RE-VALIDATE: Check volume LUN still exists (could have been deleted after preliminary check)
	volLunExists, volBucketVolRecheck, err := d.LUNExists(ctx, volLunName, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error re-checking for existing volume LUN %s", volLunName)
		return err
	}
	if !volLunExists {
		message := fmt.Sprintf("volume LUN %s does not exist", volLunName)
		Logc(ctx).Warnf(message)
		return errors.NotFoundError(message)
	}
	// Verify the bucket volume hasn't changed
	if volBucketVolRecheck != volBucketVol {
		return fmt.Errorf("volume LUN %s moved from FlexVol %s to %s during operation",
			volLunName, volBucketVol, volBucketVolRecheck)
	}

	// Check to see if the snapshot LUN exists
	snapLunExists, snapBucketVol, err := d.LUNExists(ctx, snapLunName, "", d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing snapshot LUN %s", snapLunName)
		return err
	}
	if !snapLunExists {
		return errors.NotFoundError("snapshot LUN %s does not exist", snapLunName)
	}

	// Sanity check to ensure both LUNs are in the same Flexvol
	if volBucketVol != snapBucketVol {
		return fmt.Errorf("snapshot LUN %s and volume LUN %s are in different Flexvols", snapLunName, volLunName)
	}

	// Check to see if the snapshot LUN copy exists
	snapLunCopyExists, snapCopyBucketVol, err := d.LUNExists(ctx, snapLunCopyName, "", d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing snapshot copy LUN %s", snapLunCopyName)
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
			return errors.New(msg)
		}

		if err = d.API.LunDestroy(ctx, snapLunCopyPath); err != nil {
			return fmt.Errorf("could not delete snapshot copy LUN %s", snapLunCopyName)
		}
	}

	// Get the snapshot LUN
	snapLunPath := GetLUNPathEconomy(snapBucketVol, snapLunName)
	snapLunInfo, err := d.API.LunGetByName(ctx, snapLunPath)
	if err != nil {
		return fmt.Errorf("could not get existing LUN %s; %w", snapLunPath, err)
	}

	// Clone the snapshot LUN
	if err = d.API.LunCloneCreate(ctx, snapBucketVol, snapLunName, snapLunCopyName, snapLunInfo.Qos); err != nil {
		return fmt.Errorf("could not clone snapshot LUN %s: %w", snapLunPath, err)
	}

	// Rename the original LUN
	volLunPath := GetLUNPathEconomy(volBucketVol, volLunName)
	tempVolLunPath := volLunPath + "_original"
	if err = d.API.LunRename(ctx, volLunPath, tempVolLunPath); err != nil {
		return fmt.Errorf("could not rename LUN %s: %w", volLunPath, err)
	}

	// Rename snapshot copy to original LUN path
	snapLunCopyPath := GetLUNPathEconomy(snapBucketVol, snapLunCopyName)
	if err = d.API.LunRename(ctx, snapLunCopyPath, volLunPath); err != nil {

		// Attempt to recover by restoring the original LUN
		if recoverErr := d.API.LunRename(ctx, tempVolLunPath, volLunPath); recoverErr != nil {
			Logc(ctx).WithError(recoverErr).Errorf("Could not recover RestoreSnapshot by renaming LUN %s to %s.",
				tempVolLunPath, volLunPath)
		}

		return fmt.Errorf("could not rename LUN %s: %w", snapLunCopyPath, err)
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

	// Check to see if the snapshot LUN exists
	exists, bucketVol, err := d.LUNExists(ctx, snapLunName, "", d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s", snapLunName)
		return err
	}
	if !exists {
		return errors.NotFoundError("snapshot LUN %s does not exist", snapLunName)
	}

	// Lock the FlexVol early to prevent race conditions during concurrent deletes
	lockedFlexvol := d.lockFlexvol(bucketVol)
	defer lockedFlexvol.Unlock()

	snapPath := GetLUNPathEconomy(bucketVol, snapLunName)

	// Don't leave the new LUN around
	if err = d.API.LunDestroy(ctx, snapPath); err != nil {
		Logc(ctx).WithField("LUN", snapPath).WithError(err).Error("Snap LUN delete failed.")
		return fmt.Errorf("error deleting snapshot: %w", err)
	}

	// Check if a bucket volume has no more LUNs. If none left, delete the bucketVol. Else, call for resize
	return d.deleteBucketIfEmpty(ctx, bucketVol)
}

// GetGroupSnapshotTarget returns a set of information about the target of a group snapshot.
// This information is used to gather information in a consistent way across storage drivers.
func (d *SANEconomyStorageDriver) GetGroupSnapshotTarget(
	ctx context.Context, volConfigs []*storage.VolumeConfig,
) (*storage.GroupSnapshotTargetInfo, error) {
	fields := LogFields{
		"Method": "GetGroupSnapshotTarget",
		"Type":   "SANEconomyStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetGroupSnapshotTarget")
	defer Logd(ctx, d.Name(),
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetGroupSnapshotTarget")

	targetType := PersonalityUnified
	if d.Config.Flags != nil && d.Config.Flags[FlagPersonality] != "" {
		targetType = d.Config.Flags[FlagPersonality]
	}
	targetUUID := d.API.GetSVMUUID()

	// For this driver, there can be 1-many volume configs per source name.
	// Construct a set of unique bucket volume IDs to volume names to configs for the group snapshot.
	targetVolumes := make(storage.GroupSnapshotTargetVolumes)
	for _, volumeConfig := range volConfigs {
		volumeName := volumeConfig.Name
		internalID := volumeConfig.InternalID
		internalName := volumeConfig.InternalName

		// Check if the parent volume and LUN exists; if they do, add it to the target volumes.
		exists, bucketVol, err := d.LUNExists(ctx, internalName, internalID, d.FlexvolNamePrefix())
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s.", volumeName)
			return nil, err
		}
		if !exists {
			return nil, errors.NotFoundError("LUN %s does not exist", volumeName)
		}

		if targetVolumes[bucketVol] == nil {
			targetVolumes[bucketVol] = make(map[string]*storage.VolumeConfig)
		}

		// bucket volume name -> pv name -> volume config.
		targetVolumes[bucketVol][volumeName] = volumeConfig
	}

	return storage.NewGroupSnapshotTargetInfo(targetType, targetUUID, targetVolumes), nil
}

func (d *SANEconomyStorageDriver) CreateGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, target *storage.GroupSnapshotTargetInfo,
) error {
	fields := LogFields{
		"Method": "CreateGroupSnapshot",
		"Type":   "SANEconomyStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateGroupSnapshot")

	return CreateGroupSnapshot(ctx, config, target, &d.Config, d.API)
}

// destroyBucketSnapshots destroys a set of bucket volume snapshots, keyed by bucket volume name.
func (d *SANEconomyStorageDriver) destroyBucketSnapshots(ctx context.Context, snapshots map[string]api.Snapshot) error {
	fields := LogFields{
		"Method": "destroyBucketSnapshots",
		"Type":   "SANEconomyStorageDriver",
	}
	Logd(ctx, d.Name(),
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> destroyBucketSnapshots")
	defer Logd(ctx, d.Name(),
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< destroyBucketSnapshots")

	// Delete the bucket volume snapshots.
	// The backoff retry make take time; spawn go routines to handle deleting each bucket volume snapshot.
	var errs error
	errCh := make(chan error, len(snapshots))
	wg := sync.WaitGroup{}
	for volume, snapshot := range snapshots {
		snap := snapshot.Name
		wg.Add(1)

		go func(snap, vol string) {
			defer wg.Done()

			err := d.API.VolumeSnapshotDeleteWithRetry(ctx, snap, vol, maxSnapshotDeleteRetry, maxSnapshotDeleteWait)
			if err != nil {
				// The function call above should handle transient ONTAP errors like "snapshot is busy".
				// An error at this point means there was a terminal error. Capture the error.
				Logc(ctx).WithFields(LogFields{
					"bucketVolume":   vol,
					"bucketSnapshot": snap,
				}).WithError(err).Warn("Could not remove snapshot for volume. Snapshot may require removal.")
				errCh <- err
			}

			Logc(ctx).Debugf("Removed bucket snapshot %s for bucket volume %s", snap, vol)
		}(snap, volume)
	}

	// Wait for all deletes to complete.
	wg.Wait()
	close(errCh)

	// Read all errors from the channel.
	for deleteErr := range errCh {
		errs = errors.Join(errs, deleteErr)
	}
	return errs
}

// ProcessGroupSnapshot accepts a group snapshot config, a slice of volumes relative to this driver,
// and processes as much of the group snapshot by volumes as it can. It returns a set of snapshots
// relative to the slice of volumes that were owned by this driver.
func (d *SANEconomyStorageDriver) ProcessGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, volConfigs []*storage.VolumeConfig,
) (snapshots []*storage.Snapshot, errs error) {
	fields := LogFields{
		"Method": "ProcessGroupSnapshot",
		"Type":   "SANEconomyStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ProcessGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ProcessGroupSnapshot")

	// Assign an internal name at the driver level.
	config.InternalName = config.ID()
	groupName := config.ID()
	snapName, err := storage.ConvertGroupSnapshotID(groupName)
	if err != nil {
		return nil, err
	}

	snapshots = make([]*storage.Snapshot, 0)
	bucketSnapshots := make(map[string]api.Snapshot)

	defer func() {
		// Always delete the bucket volume snapshots.
		if err := d.destroyBucketSnapshots(ctx, bucketSnapshots); err != nil {
			Logc(ctx).WithError(err).Error("Failed to cleanup bucket volume snapshots in failed group snapshot.")
			errs = errors.Join(errs, err)
		}
	}()

	// Clone snap LUNs directly out of the bucket volume snapshots.
	// NOTE: If any failures occur, handling them should be deferred to the higher levels of Trident.
	// Each driver should clean up the bits that the core won't know about.
	for _, volConfig := range volConfigs {
		volumeName := volConfig.Name
		internalID := volConfig.InternalID
		internalVolumeName := volConfig.InternalName
		fields := LogFields{
			"volumeName":         volumeName,
			"internalVolumeID":   internalID,
			"internalVolumeName": internalVolumeName,
			"snapshotName":       snapName,
			"groupName":          groupName,
		}

		// Get the bucket volume and source LUN name from the internal ID.
		_, bucketVol, lunName, err := d.ParseLunInternalID(internalID)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		fields["bucketVolume"], fields["lunName"] = bucketVol, lunName

		// A single bucket volume snapshot may contain many LUNs.
		// Cache the snapshots as we move through the volumes to save API calls.
		snap, ok := bucketSnapshots[bucketVol]
		if !ok {
			snap, err = d.API.VolumeSnapshotInfo(ctx, snapName, bucketVol)
			if err != nil {
				Logc(ctx).WithFields(fields).WithError(err).Error("Could not find snapshot for bucket volume.")
				errs = errors.Join(errs, err)
				continue
			}
			fields["bucketSnapshot"] = snap.Name
			bucketSnapshots[bucketVol] = snap
			Logc(ctx).WithFields(fields).Debug("Found snapshot for bucket volume.")
		}
		// Creation time is typically taken from the time a snapLUN is created, but for group snapshots
		// this value should be set to the time the parent group snapshot was taken, which should
		// be congruent across all snapshots at the FlexVol. Hence, pull the creation time from the snapshot.
		creationTime := snap.CreateTime

		// Build the desired clone LUN name and source path for ONTAP.
		lunCloneName := d.helper.GetSnapshotName(internalVolumeName, snapName)
		cloneSourcePath := GetEconomyLUNPathInSnapshot(bucketVol, snapName, lunName)
		cloneLUNID, err := d.cloneLUNFromSnapshot(ctx, bucketVol, cloneSourcePath, lunCloneName, api.QosPolicyGroup{})
		if err != nil {
			// We must continue in case there are other bucketVol snapshots that need cleansing.
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to create clone from LUN in snapshot.")
			errs = errors.Join(errs, err)
			continue
		}
		fields["cloneLUNID"] = cloneLUNID

		// Check for the existence of the LUN now and get the size.
		// The size of the original LUN may have changed from the time
		// the group snapshot was initiated, so we can't rely on it here.
		// Pull the size directly from the cloned LUN.
		lunClonePath := GetLUNPathEconomy(bucketVol, lunCloneName)
		fields["lunClonePath"] = lunClonePath
		lunInfo, err := d.API.LunGetByName(ctx, lunClonePath)
		if err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to find LUN clone from snapshot.")
			errs = errors.Join(errs, err)
			continue
		}
		sizeBytes, err := convert.ToPositiveInt64(lunInfo.Size)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%v is an invalid LUN size: %w", lunInfo.Size, err))
			continue
		}

		// Build the snapshot and add it to the list.
		snapConfig := &storage.SnapshotConfig{
			Name: snapName,
			// This is a misnomer carried over by standard snapshots.
			// The actual internal name is the name of the LUN that plays the snapshot role.
			// internalID of snapLUN:
			//  "/vol/trident_lun_pool_prefix_MACNWIILZO" +
			//  "/prefix_pvc_6ac12cb7_4224_4370_8e2d_fddb0be18e10_snapshot_snapshot_cf53c1d1_b2f8_4c6b_aa5d_1262aedc432b"
			// internalName should be:
			//  "prefix_pvc_6ac12cb7_4224_4370_8e2d_fddb0be18e10_snapshot_snapshot_cf53c1d1_b2f8_4c6b_aa5d_1262aedc432b"
			InternalName:       snapName,
			VolumeName:         volumeName,
			VolumeInternalName: internalVolumeName,
			ImportNotManaged:   false,
			GroupSnapshotName:  groupName,
		}
		snapshot := &storage.Snapshot{
			Config:    snapConfig,
			Created:   creationTime,
			SizeBytes: sizeBytes,
			State:     storage.SnapshotStateOnline,
		}
		snapshots = append(snapshots, snapshot)
	}

	if errs != nil {
		Logc(ctx).WithFields(fields).Error("Failed to process group snapshot.")
		return snapshots, errs
	}
	return snapshots, nil
}

func (d *SANEconomyStorageDriver) ConstructGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, snapshots []*storage.Snapshot,
) (*storage.GroupSnapshot, error) {
	fields := LogFields{
		"Method": "ConstructGroupSnapshot",
		"Type":   "SANEconomyStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ConstructGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ConstructGroupSnapshot")

	return ConstructGroupSnapshot(ctx, config, snapshots, &d.Config)
}

// Get tests for the existence of a volume
func (d *SANEconomyStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "SANEconomyStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	// Generic user-facing message
	getError := errors.NotFoundError("volume %s not found", name)

	exists, bucketVol, err := d.LUNExists(ctx, name, "", d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s", name)
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
// LUN or it creates a new Flexvol with the needed attributes.
//
// IMPORTANT: This method returns a lockedFlexVol object with a lock held. The caller MUST call
// Unlock() on the returned object (typically via defer) after completing the operation.
func (d *SANEconomyStorageDriver) ensureFlexvolForLUN(
	ctx context.Context, volAttrs *api.Volume, sizeBytes uint64, opts map[string]string,
	config *drivers.OntapStorageDriverConfig, storagePool storage.Pool, ignoredVols map[string]struct{},
) (*locks.LockedResource, bool, error) {
	shouldLimitFlexvolSize, flexvolSizeLimit, checkFlexvolSizeLimitsError := CheckVolumePoolSizeLimits(
		ctx, sizeBytes, config)
	if checkFlexvolSizeLimitsError != nil {
		return nil, false, checkFlexvolSizeLimitsError
	}

	// Check if a suitable Flexvol already exists
	// NOTE: findFlexvolForLUN returns a lockedFlexVol with lock held if it finds a suitable FlexVol
	lockedFlexvol, err := d.findFlexvolForLUN(ctx, volAttrs, sizeBytes, shouldLimitFlexvolSize, flexvolSizeLimit, ignoredVols)
	if err != nil {
		return nil, false, fmt.Errorf("error finding Flexvol for LUN: %w", err)
	}

	// Found one (lock is already held)!
	if lockedFlexvol != nil {
		return lockedFlexvol, false, nil
	}

	// No suitable FlexVol found - need to create one
	// Use flexvolCreationMuxtex to prevent concurrent creation races (double-check locking pattern)
	// This allows the "find FlexVol" decision to happen in parallel across all threads, only serializing
	// the actual Flexvol creation.
	d.flexvolCreationLock.Lock()

	// Double-check: another goroutine may have created a FlexVol while we waited for the lock
	lockedFlexvol, err = d.findFlexvolForLUN(ctx, volAttrs, sizeBytes, shouldLimitFlexvolSize, flexvolSizeLimit, ignoredVols)
	if err != nil {
		d.flexvolCreationLock.Unlock()
		return nil, false, fmt.Errorf("error finding Flexvol for LUN: %w", err)
	}
	if lockedFlexvol != nil {
		// Another goroutine created one - use it (lock already held by findFlexvolForLUN)
		// Release global lock before returning with per-FlexVol lock
		d.flexvolCreationLock.Unlock()
		return lockedFlexvol, false, nil
	}

	// If we can't create a new Flexvol, fail
	if d.denyNewFlexvols {
		d.flexvolCreationLock.Unlock()
		return nil, false, errors.New("new Flexvol creation not permitted")
	}

	// Nothing found, so create a suitable Flexvol
	flexvol, err := d.createFlexvolForLUN(ctx, volAttrs, opts, storagePool)
	if err != nil {
		d.flexvolCreationLock.Unlock()
		return nil, false, fmt.Errorf("error creating Flexvol for LUN: %w", err)
	}

	// Release global lock BEFORE acquiring per-FlexVol lock to maintain proper lock hierarchy
	d.flexvolCreationLock.Unlock()

	// Now lock the newly created FlexVol and return
	lockedFlexvol = d.lockFlexvol(flexvol)
	return lockedFlexvol, true, nil
}

// createFlexvolForLUN creates a new Flexvol matching the specified attributes for
// the purpose of containing LUN supplied as container volumes by this driver.
// Once this method returns, the Flexvol exists, is mounted
func (d *SANEconomyStorageDriver) createFlexvolForLUN(
	ctx context.Context, volumeAttributes *api.Volume, opts map[string]string, storagePool storage.Pool,
) (string, error) {
	var (
		flexvol         = d.FlexvolNamePrefix() + crypto.RandomString(10)
		size            = "1g"
		unixPermissions = collection.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy    = collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle   = collection.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption      = volumeAttributes.Encrypt
	)

	snapshotReserveInt, err := GetSnapshotReserve(volumeAttributes.SnapshotPolicy,
		storagePool.InternalAttributes()[SnapshotReserve])
	if err != nil {
		return "", fmt.Errorf("invalid value for snapshotReserve: %w", err)
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
			"exportPolicy":    exportPolicy,
			"securityStyle":   securityStyle,
			"encryption":      convert.ToPrintableBoolPtr(encryption),
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
			SnapshotPolicy:  volumeAttributes.SnapshotPolicy,
			SnapshotReserve: snapshotReserveInt,
			SpaceReserve:    volumeAttributes.SpaceReserve,
			TieringPolicy:   volumeAttributes.TieringPolicy,
			UnixPermissions: unixPermissions,
			UUID:            "",
			DPVolume:        false,
		})
	if err != nil {
		return "", fmt.Errorf("error creating volume: %w", err)
	}

	return flexvol, nil
}

// findFlexvolForLUN returns a locked FlexVol (from the set of existing Flexvols) that
// matches the specified Flexvol attributes and does not already contain more
// than the maximum configured number of LUNs. No matching Flexvols is not
// considered an error.
//
// IMPORTANT: This method acquires a lock on the FlexVol before checking its capacity
// and LUN count to prevent TOCTOU races. If a suitable FlexVol is found, this method
// returns a lockedFlexVol object with the lock held. The caller MUST call Unlock() on
// the returned object (typically via defer). If no FlexVol is found, nil is returned.
func (d *SANEconomyStorageDriver) findFlexvolForLUN(
	ctx context.Context, volumeAttributes *api.Volume, sizeBytes uint64, shouldLimitFlexvolSize bool,
	flexvolSizeLimit uint64, ignoredVols map[string]struct{},
) (*locks.LockedResource, error) {
	// Get all volumes matching the specified attributes
	volumes, err := d.API.VolumeListByAttrs(ctx, volumeAttributes)
	if err != nil {
		return nil, fmt.Errorf("error listing volumes; %w", err)
	}

	// Shuffle the candidate FlexVols to distribute load and avoid thundering herd
	if len(volumes) > 1 {
		for i := len(volumes) - 1; i > 0; i-- {
			j := crypto.RandomNumber(i + 1)
			volumes[i], volumes[j] = volumes[j], volumes[i]
		}
	}

	// Check each candidate FlexVol for eligibility.
	// Lock each FlexVol BEFORE reading its size/LUN data to prevent TOCTOU races.
	// If a FlexVol is suitable, return with the lock still held (caller inherits it).
	// If not suitable, unlock and continue to the next candidate.
	for _, volume := range volumes {
		volName := volume.Name

		// Skip volumes that have been marked as ignored (full)
		if _, ignored := ignoredVols[volName]; ignored {
			continue
		}

		// Lock this FlexVol before checking capacity/LUN count
		lockedFlexvol := d.lockFlexvol(volName)

		// Check if flexvol is over the size limit (with proposed new LUN)
		if shouldLimitFlexvolSize {
			sizeWithRequest, err := d.getOptimalSizeForFlexvol(ctx, volName, sizeBytes)
			if err != nil {
				Logc(ctx).WithError(err).Errorf("Error checking size for existing LUN %s", volName)
				lockedFlexvol.Unlock()
				continue
			}
			if sizeWithRequest > flexvolSizeLimit {
				Logc(ctx).Debugf("Flexvol size for %v is over the limit of %v", volName, flexvolSizeLimit)
				lockedFlexvol.Unlock()
				continue
			}
		}

		// Check LUN count
		lunPathPattern := fmt.Sprintf("/vol/%s/*", volName)
		luns, err := d.API.LunList(ctx, lunPathPattern)
		if err != nil {
			lockedFlexvol.Unlock()
			return nil, fmt.Errorf("error enumerating LUNs for volume %v: %w", volName, err)
		}

		count := 0
		for _, lunInfo := range luns {
			lunPath := GetLUNPathEconomy(volName, lunInfo.Name)
			if !d.helper.IsValidSnapLUNPath(lunPath) {
				count++
			}
		}

		// Found a suitable FlexVol - return with lock held
		if count < d.lunsPerFlexvol {
			Logc(ctx).WithFields(LogFields{
				"flexvol":  volName,
				"lunCount": count,
				"maxLUNs":  d.lunsPerFlexvol,
			}).Debug("Selected FlexVol for new LUN (lock held).")
			return lockedFlexvol, nil
		}

		// This FlexVol is full, unlock and try next
		lockedFlexvol.Unlock()
	}

	// No suitable FlexVol found (no lock held)
	return nil, nil
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
		return 0, fmt.Errorf("error determining LUN size: %w ", err)
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
		return 0, fmt.Errorf("error enumerating LUNs for volume %v: %w", flexvol, err)
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

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *SANEconomyStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapEconomyStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "SANEconomyStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. SVM UUID
	// 2. Aggregate (physical pool)
	// 3. FlexVol Name Prefix
	svmUUID := d.GetAPI().GetSVMUUID()
	flexVolPrefix := d.FlexvolNamePrefix()
	backendPools := make([]drivers.OntapEconomyStorageBackendPool, 0)
	for _, pool := range d.physicalPools {
		backendPool := drivers.OntapEconomyStorageBackendPool{
			SvmUUID:       svmUUID,
			Aggregate:     pool.Name(),
			FlexVolPrefix: flexVolPrefix,
		}
		backendPools = append(backendPools, backendPool)
	}

	return backendPools
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

func (d *SANEconomyStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

func (d *SANEconomyStorageDriver) CreatePrepare(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) {
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

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the path of the
// LUN on the storage system in the format <flexvol>/<lun>.
func (d *SANEconomyStorageDriver) GetVolumeForImport(
	ctx context.Context, volumeID string,
) (*storage.VolumeExternal, error) {
	pathElements := strings.Split(volumeID, "/")
	if len(pathElements) < 2 {
		return nil, fmt.Errorf("%s is not a valid import vol/LUN path", volumeID)
	}
	originalFlexvolName := pathElements[0]
	originalLUNName := pathElements[1]

	volumeAttrs, err := d.API.VolumeInfo(ctx, originalFlexvolName)
	if err != nil {
		return nil, err
	}

	ecoLUNPath := GetLUNPathEconomy(originalFlexvolName, originalLUNName)
	extantLUN, err := d.API.LunGetByName(ctx, ecoLUNPath)
	if err != nil {
		return nil, err
	}
	Logc(ctx).WithFields(LogFields{
		"name":   extantLUN.Name,
		"volume": extantLUN.VolumeName,
		"size":   extantLUN.Size,
	}).Trace("SANEconomyStorageDriver#GetVolumeForImport - LUN found.")

	return d.getVolumeExternal(extantLUN, volumeAttrs), nil
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
func (d *SANEconomyStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANEconomyStorageDriver)
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

// LUNExists returns true if the named LUN exists across all buckets.  This should be called with the
// actual LUN name, i.e. the internal volume name or snap-LUN name.
func (d *SANEconomyStorageDriver) LUNExists(
	ctx context.Context, name, internalID, bucketPrefix string,
) (bool, string, error) {
	Logc(ctx).WithFields(
		LogFields{
			"name":         name,
			"bucketPrefix": bucketPrefix,
		},
	).Debug("Checking if LUN exists.")

	var lunPathPattern string
	if internalID == "" {
		lunPathPattern = fmt.Sprintf("/vol/%s*/%s", bucketPrefix, name)
	} else {
		if _, flexVolumeName, _, err := d.ParseLunInternalID(internalID); err == nil {
			lunPathPattern = fmt.Sprintf("/vol/%s/%s", flexVolumeName, name)
		} else {
			lunPathPattern = fmt.Sprintf("/vol/%s*/%s", bucketPrefix, name)
		}
	}

	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		return false, "", err
	}

	for _, lun := range luns {
		Logc(ctx).WithField("name", lun.Name).Debug("LUN exists.")
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

	// Add a constant overhead for LUKS volumes to account for LUKS metadata. This overhead is
	// part of the LUN but is not reported to the orchestrator.
	sizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, sizeBytes, volConfig.LUKSEncryption)

	// Generic user-facing message
	resizeError := errors.New("storage driver failed to resize the volume")

	// Validation checks
	// get the volume where the lun exists
	exists, bucketVol, err := d.LUNExists(ctx, name, volConfig.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Error checking for existing LUN %s.", name)
		return resizeError
	}
	if !exists {
		return errors.NotFoundError("LUN %s does not exist", name)
	}

	// Lock the FlexVol to ensure atomic resize operations
	lockedFlexvol := d.lockFlexvol(bucketVol)
	defer lockedFlexvol.Unlock()

	// Calculate the delta size needed to resize the bucketVol (now under lock)
	totalLunSize, err := d.getTotalLUNSize(ctx, bucketVol)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to determine total LUN size.")
		return resizeError
	}

	currentLunSize, err := d.getLUNSize(ctx, name, bucketVol)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to determine LUN size.")
		return resizeError
	}

	flexvolSize, err := d.getOptimalSizeForFlexvol(ctx, bucketVol, sizeBytes-currentLunSize)
	if err != nil {
		Logc(ctx).WithError(err).Warnf("Could not calculate optimal Flexvol size.")
		flexvolSize = totalLunSize + sizeBytes - currentLunSize
	}

	sameSize := capacity.VolumeSizeWithinTolerance(
		int64(flexvolSize), int64(totalLunSize), tridentconfig.SANResizeDelta,
	)

	// If LUN exists, update the volConfig.InternalID in case it was not set.  This is useful for
	// "legacy" volumes which do not have InternalID set when they were created.
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateLUNInternalID(d.Config.SVM, bucketVol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("Setting InternalID.")
	}

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

	// Enforce aggregate usage limit
	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
		ctx, bucketVol, flexvolSize, d.Config, d.GetAPI(),
	); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	// Enforce volume size limit
	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Enforce Flexvol pool size limit
	shouldLimitFlexvolSize, flexvolSizeLimit, checkFlexvolSizeLimitsError := CheckVolumePoolSizeLimits(
		ctx, sizeBytes, &d.Config)
	if checkFlexvolSizeLimitsError != nil {
		return checkFlexvolSizeLimitsError
	}
	if shouldLimitFlexvolSize && flexvolSize > flexvolSizeLimit {
		return errors.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested size %d would exceed the pool size limit %d", sizeBytes, flexvolSizeLimit))
	}

	// Resize operations

	lunPath := d.helper.GetLUNPath(bucketVol, name)

	// Resize FlexVol
	err = d.API.VolumeSetSize(ctx, bucketVol, strconv.FormatUint(flexvolSize, 10))
	if err != nil {
		Logc(ctx).WithError(err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	// Resize LUN
	returnSize, err := d.API.LunSetSize(ctx, lunPath, strconv.FormatUint(sizeBytes, 10))
	if err != nil {
		Logc(ctx).WithError(err).Error("LUN resize failed.")
		return fmt.Errorf("volume resize failed")
	}
	// LUKS metadata is not reported to orchestrator
	returnSize = decrementWithLUKSMetadataIfLUKSEnabled(ctx, returnSize, volConfig.LUKSEncryption)

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
		Logc(ctx).WithError(err).Warn("Could not calculate optimal Flexvol size.")
		// Lacking the optimal size, just grow the Flexvol to contain the new LUN
		size := strconv.FormatUint(sizeBytes, 10)
		err := d.API.VolumeSetSize(ctx, flexvol, "+"+size)
		if err != nil {
			Logc(ctx).WithError(err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	} else {
		// Got optimal size, so just set the Flexvol to that value
		flexvolSizeStr := strconv.FormatUint(flexvolSizeBytes, 10)
		err := d.API.VolumeSetSize(ctx, flexvol, flexvolSizeStr)
		if err != nil {
			Logc(ctx).WithError(err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}
	return nil
}

func (d *SANEconomyStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*models.Node, backendUUID, tridentUUID string,
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

func (d *SANEconomyStorageDriver) ReconcileVolumeNodeAccess(
	ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node,
) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "SANEconomyStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *SANEconomyStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "iscsi", d.GetStorageBackendPhysicalPoolNames(ctx), d.Config.Aggregate)
}

// String makes SANEconomyStorageDriver satisfy the Stringer interface.
func (d *SANEconomyStorageDriver) String() string {
	return convert.ToStringRedacted(d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes SANEconomyStorageDriver satisfy the GoStringer interface.
func (d *SANEconomyStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d *SANEconomyStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

func (d *SANEconomyStorageDriver) GetChapInfo(_ context.Context, _, _ string) (*models.IscsiChapInfo, error) {
	return &models.IscsiChapInfo{
		UseCHAP:              d.Config.UseCHAP,
		IscsiUsername:        d.Config.ChapUsername,
		IscsiInitiatorSecret: d.Config.ChapInitiatorSecret,
		IscsiTargetUsername:  d.Config.ChapTargetUsername,
		IscsiTargetSecret:    d.Config.ChapTargetInitiatorSecret,
	}, nil
}

// EnablePublishEnforcement prepares a volume for per-node igroup mapping allowing greater access control.
func (d *SANEconomyStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	if volume == nil || volume.Config == nil {
		return errors.New("volume or volume config is nil")
	}

	internalName := volume.Config.InternalName
	exists, flexVol, err := d.LUNExists(ctx, internalName, volume.Config.InternalID, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"flexVol":    flexVol,
			"lunName":    internalName,
			"volumeName": volume.Config.Name,
		}).WithError(err).Error("Error checking for existing LUN.")
		return err
	}
	if !exists {
		return errors.NotFoundError("LUN %s does not exist", internalName)
	}
	return EnableSANPublishEnforcement(ctx, d.GetAPI(), volume.Config, GetLUNPathEconomy(flexVol, internalName))
}

func (d *SANEconomyStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}

func (d *SANEconomyStorageDriver) HealVolumePublishEnforcement(ctx context.Context, volume *storage.Volume) bool {
	return HealSANPublishEnforcement(ctx, d, volume)
}

// ParseLunInternalID parses the passed string which is in the format /svm/<svm_name>/flexvol/<flexvol_name>/lun/<lun_name>
// and returns svm, flexvol and LUN name.
func (d *SANEconomyStorageDriver) ParseLunInternalID(internalId string) (svm, flexvol, lun string, err error) {
	match := LUNInternalIDRegex.FindStringSubmatch(internalId)
	if match == nil {
		err = fmt.Errorf("internalId ID %s is invalid", internalId)
		return
	}

	paramsMap := make(map[string]string)
	for idx, subExpName := range LUNInternalIDRegex.SubexpNames() {
		if idx > 0 && idx <= len(match) {
			paramsMap[subExpName] = match[idx]
		}
	}

	svm = paramsMap["svm"]
	flexvol = paramsMap["flexvol"]
	lun = paramsMap["lun"]
	return
}

// CreateLUNInternalID creates a string in the format /svm/<svm_name>/flexvol/<flexvol_name>/lun/<lun_name>
func (d *SANEconomyStorageDriver) CreateLUNInternalID(svm, flexvol, name string) string {
	return fmt.Sprintf("/svm/%s/flexvol/%s/lun/%s", svm, flexvol, name)
}
