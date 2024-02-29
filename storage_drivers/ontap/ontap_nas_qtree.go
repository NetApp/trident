// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/google/uuid"

	"github.com/netapp/trident/acp"
	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/crypto"
	"github.com/netapp/trident/utils/errors"
)

var QtreeInternalIDRegex = regexp.MustCompile(`^/svm/(?P<svm>[^/]+)/flexvol/(?P<flexvol>[^/]+)/qtree/(?P<qtree>[^/]+)$`)

const (
	deletedQtreeNamePrefix                  = "deleted_"
	maxQtreeNameLength                      = 64
	minQtreesPerFlexvol                     = 50
	defaultQtreesPerFlexvol                 = 200
	maxQtreesPerFlexvol                     = 300
	defaultPruneFlexvolsPeriod              = 10 * time.Minute // default to 10 minutes
	defaultResizeQuotasPeriod               = 1 * time.Minute  // default to 1 minute
	defaultEmptyFlexvolDeferredDeletePeriod = 8 * time.Hour    // default to 8 hours
	pruneTask                               = "prune"
	resizeTask                              = "resize"
)

// NASQtreeStorageDriver is for NFS and SMB storage provisioning of qtrees
type NASQtreeStorageDriver struct {
	initialized                      bool
	Config                           drivers.OntapStorageDriverConfig
	API                              api.OntapAPI
	AWSAPI                           awsapi.AWSAPI
	telemetry                        *Telemetry
	quotaResizeMap                   map[string]bool
	flexvolNamePrefix                string
	flexvolExportPolicy              string
	housekeepingTasks                map[string]*HousekeepingTask
	housekeepingWaitGroup            *sync.WaitGroup
	sharedLockID                     string
	emptyFlexvolMap                  map[string]time.Time
	emptyFlexvolDeferredDeletePeriod time.Duration
	qtreesPerFlexvol                 int

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers map[string]time.Time
}

func (d *NASQtreeStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASQtreeStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *NASQtreeStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d *NASQtreeStorageDriver) Name() string {
	return tridentconfig.OntapNASQtreeStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *NASQtreeStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapnaseco_" + d.Config.DataLIF)
	} else {
		return d.Config.BackendName
	}
}

func (d *NASQtreeStorageDriver) FlexvolNamePrefix() string {
	return d.flexvolNamePrefix
}

// Initialize from the provided config
func (d *NASQtreeStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "NASQtreeStorageDriver"}
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

	// Initialize AWS API if applicable.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.AWSAPI == nil {
		d.AWSAPI, err = initializeAWSDriver(ctx, config)
		if err != nil {
			return fmt.Errorf("error initializing %s AWS driver; %v", d.Name(), err)
		}
	}

	// Initialize the ONTAP API.
	// Initialize ONTAP driver. Unit test uses mock driver, so initialize only if driver not already set
	if d.API == nil {
		d.API, err = InitializeOntapDriver(ctx, config)
		if err != nil {
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}
	}

	d.Config = *config

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
	d.quotaResizeMap = make(map[string]bool)
	d.flexvolNamePrefix = fmt.Sprintf("%s_qtree_pool_%s_", artifactPrefix, *d.Config.StoragePrefix)
	d.flexvolNamePrefix = strings.Replace(d.flexvolNamePrefix, "-", "_", -1)  // ONTAP disallows hyphens
	d.flexvolNamePrefix = strings.Replace(d.flexvolNamePrefix, "__", "_", -1) // Remove any double underscores
	if d.Config.AutoExportPolicy {
		d.flexvolExportPolicy = "<automatic>"
	} else {
		d.flexvolExportPolicy = fmt.Sprintf("%s_qtree_pool_export_policy", artifactPrefix)
	}
	d.sharedLockID = d.API.GetSVMUUID() + "-" + *d.Config.StoragePrefix
	d.emptyFlexvolMap = make(map[string]time.Time)

	// Ensure the qtree cap is valid
	if config.QtreesPerFlexvol == "" {
		d.qtreesPerFlexvol = defaultQtreesPerFlexvol
	} else {
		if d.qtreesPerFlexvol, err = strconv.Atoi(config.QtreesPerFlexvol); err != nil {
			return fmt.Errorf("invalid config value for qtreesPerFlexvol: %v", err)
		}
		if d.qtreesPerFlexvol < minQtreesPerFlexvol {
			return fmt.Errorf("invalid config value for qtreesPerFlexvol (minimum is %d)", minQtreesPerFlexvol)
		}
		if d.qtreesPerFlexvol > maxQtreesPerFlexvol {
			return fmt.Errorf("invalid config value for qtreesPerFlexvol (maximum is %d)", maxQtreesPerFlexvol)
		}
	}

	Logc(ctx).WithFields(LogFields{
		"FlexvolNamePrefix":   d.flexvolNamePrefix,
		"FlexvolExportPolicy": d.flexvolExportPolicy,
		"QtreesPerFlexvol":    d.qtreesPerFlexvol,
		"SharedLockID":        d.sharedLockID,
	}).Debugf("Qtree driver settings.")

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

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

	// Ensure all quotas are in force after a driver restart
	d.queueAllFlexvolsForQuotaResize(ctx)

	// Start periodic housekeeping tasks like cleaning up unused Flexvols
	d.housekeepingWaitGroup = &sync.WaitGroup{}
	d.housekeepingTasks = make(map[string]*HousekeepingTask, 2)
	// pruneTasks := []func(){d.pruneUnusedFlexvols, d.reapDeletedQtrees}
	// d.housekeepingTasks[pruneTask] = NewPruneTask(d, pruneTasks)
	resizeTasks := []func(context.Context){d.resizeQuotas}
	d.housekeepingTasks[resizeTask] = NewResizeTask(ctx, d, resizeTasks)
	for _, task := range d.housekeepingTasks {
		task.Start(ctx)
	}

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	// Set up the clone split timers.
	d.cloneSplitTimers = make(map[string]time.Time)

	d.initialized = true
	return nil
}

func (d *NASQtreeStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASQtreeStorageDriver) Terminate(ctx context.Context, backendUUID string) {
	fields := LogFields{"Method": "Terminate", "Type": "NASQtreeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	if d.housekeepingWaitGroup != nil {
		for _, task := range d.housekeepingTasks {
			task.Stop(ctx)
		}
	}

	if d.Config.AutoExportPolicy {
		policyName := getExportPolicyName(backendUUID)
		if err := deleteExportPolicy(ctx, policyName, d.API); err != nil {
			Logc(ctx).Warn(err)
		}
	}

	if d.telemetry != nil {
		d.telemetry.Stop()
	}

	if d.housekeepingWaitGroup != nil {
		Logc(ctx).Debug("Waiting for housekeeping tasks to exit.")
		d.housekeepingWaitGroup.Wait()
	}

	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *NASQtreeStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "NASQtreeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := ValidateNASDriver(ctx, d.API, &d.Config); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefixEconomy(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d, 0); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	if !d.Config.AutoExportPolicy {
		// Make sure we have an export policy for all the Flexvols we create
		if err := d.ensureDefaultExportPolicy(ctx); err != nil {
			return fmt.Errorf("error configuring export policy: %v", err)
		}
	}

	return nil
}

// Create a qtree-backed volume with the specified options
func (d *NASQtreeStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Create",
		"Type":   "NASQtreeStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Ensure any Flexvol we create won't be pruned before we place a qtree on it
	utils.Lock(ctx, "create", d.sharedLockID)
	defer utils.Unlock(ctx, "create", d.sharedLockID)

	// Generic user-facing message
	createError := errors.New("volume creation failed")

	volumePattern, name, err := d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
		d.FlexvolNamePrefix())
	if err != nil {
		return err
	}
	// Ensure volume doesn't already exist
	exists, existsInFlexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing volume: %v.", err)
		return createError
	}
	if exists {
		Logc(ctx).WithFields(LogFields{"qtree": name, "flexvol": existsInFlexvol}).Debug("Qtree already exists.")
		// If qtree exists, update the volConfig.InternalID in case it was not set
		// This is useful for "legacy" volumes which do not have InternalID set when they were created
		if volConfig.InternalID == "" {
			volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, existsInFlexvol, name)
			Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
		}
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
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes()[Size])
	if err != nil {
		return err
	}

	// Ensure qtree name isn't too long
	if len(name) > maxQtreeNameLength {
		return fmt.Errorf("volume %s name exceeds the limit of %d characters", name, maxQtreeNameLength)
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get Flexvol options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	var (
		spaceReserve    = utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy  = utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve = storagePool.InternalAttributes()[SnapshotReserve]
		snapshotDir     = utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes()[SnapshotDir])
		encryption      = utils.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])

		// Get qtree options with default fallback values
		unixPermissions = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy    = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle   = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		tieringPolicy   = utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		qosPolicy       = storagePool.InternalAttributes()[QosPolicy]
	)

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

	// Update config to reflect values used to create volume
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.SnapshotDir = snapshotDir
	volConfig.Encryption = configEncryption
	volConfig.UnixPermissions = unixPermissions
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.QosPolicy = qosPolicy

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS-QTREE pool %s/%s; error: %v", storagePool.Name(), aggregate,
				aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Make sure we have a Flexvol for the new qtree
		flexvol, err := d.ensureFlexvolForQtree(
			ctx, aggregate, spaceReserve, snapshotPolicy, tieringPolicy, enableSnapshotDir, enableEncryption, sizeBytes,
			d.Config, snapshotReserve, exportPolicy)
		if err != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS-QTREE pool %s/%s; Flexvol location/creation failed %s: %v",
				storagePool.Name(), aggregate, name, err)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("Created new qtree, setting InternalID")

		// Grow or shrink the Flexvol as needed
		err = d.resizeFlexvol(ctx, flexvol, sizeBytes)
		if err != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS-QTREE pool %s/%s; Flexvol resize failed %s/%s: %v", storagePool.Name(),
				aggregate, flexvol, name, err)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Create the qtree
		err = d.API.QtreeCreate(ctx, name, flexvol, unixPermissions, exportPolicy, securityStyle, qosPolicy)
		if err != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS-QTREE pool %s/%s; Qtree creation failed %s/%s: %v",
				storagePool.Name(), aggregate, flexvol, name, err)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Add the quota
		err = d.setQuotaForQtree(ctx, name, flexvol, sizeBytes)
		if err != nil {
			Logc(ctx).Errorf("Qtree quota definition failed. %v", err)
			return fmt.Errorf("ONTAP-NAS-QTREE pool %s/%s; Qtree quota definition failed %s/%s: %v", storagePool.Name(),
				aggregate, flexvol, name, err)
		}

		if d.Config.NASType == sa.SMB {
			if err = d.EnsureSMBShare(ctx, flexvol); err != nil {
				return err
			}
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *NASQtreeStorageDriver) CreateClone(
	ctx context.Context, sourceVolConfig, cloneVolConfig *storage.VolumeConfig, _ storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":   "CreateClone",
		"Type":     "NASQtreeStorageDriver",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	// If RO clone is requested, validate the snapshot directory access and return
	if cloneVolConfig.ReadOnlyClone {
		_, flexvol, _, err := d.ParseQtreeInternalID(sourceVolConfig.InternalID)
		if err != nil {
			return errors.WrapWithNotFoundError(err, "error while getting flexvol")
		}
		storageVolume, err := d.API.VolumeInfo(ctx, flexvol)
		if err != nil {
			return errors.WrapWithNotFoundError(err, "error while getting flexvol attributes")
		}

		// Return error, if snapshot directory visibility is not enabled for the source volume
		if !storageVolume.SnapshotDir {
			return fmt.Errorf("snapshot directory access is set to %t and readOnly clone is set to %t ",
				storageVolume.SnapshotDir, cloneVolConfig.ReadOnlyClone)
		}
	} else {
		return fmt.Errorf("cloning is not supported by backend type %s", d.Name())
	}

	return nil
}

func (d *NASQtreeStorageDriver) Import(
	context.Context, *storage.VolumeConfig, string,
) error {
	return errors.New("import is not implemented")
}

func (d *NASQtreeStorageDriver) Rename(context.Context, string, string) error {
	return errors.New("rename is not implemented")
}

// Destroy the volume
func (d *NASQtreeStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "NASQtreeStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// Ensure the deleted qtree reaping job doesn't interfere with this workflow
	utils.Lock(ctx, "destroy", d.sharedLockID)
	defer utils.Unlock(ctx, "destroy", d.sharedLockID)

	// Generic user-facing message
	deleteError := errors.New("volume deletion failed")

	volumePattern, name, err := d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
		d.FlexvolNamePrefix())
	if err != nil {
		return err
	}
	// Check that volume exists
	exists, flexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing qtree. %s", err.Error())
		return deleteError
	}
	if !exists {
		Logc(ctx).WithField("qtree", name).Warn("Qtree not found.")
		return nil
	}

	// If qtree exists, update the volConfig.InternalID in case it was not set
	// This is useful for "legacy" volumes which do not have InternalID set when they were created
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
	}

	// Rename qtree so it doesn't show up in lists while ONTAP is deleting it in the background.
	// Ensure the deleted name doesn't exceed the qtree name length limit of 64 characters.
	path := fmt.Sprintf("/vol/%s/%s", flexvol, name)
	deletedName := deletedQtreeNamePrefix + name + "_" + utils.RandomString(5)
	if len(deletedName) > maxQtreeNameLength {
		trimLength := len(deletedQtreeNamePrefix) + 10
		deletedName = deletedQtreeNamePrefix + name[trimLength:] + "_" + utils.RandomString(5)
	}
	deletedPath := fmt.Sprintf("/vol/%s/%s", flexvol, deletedName)

	err = d.API.QtreeRename(ctx, path, deletedPath)
	if err != nil {
		Logc(ctx).Errorf("Qtree rename failed. %v", err)
		return deleteError
	}

	// Destroy the qtree in the background.  If this fails, try to restore the original qtree name.
	err = d.API.QtreeDestroyAsync(ctx, deletedPath, true)
	if err != nil {
		Logc(ctx).Errorf("Qtree async delete failed. %v", err)
		if err := d.API.QtreeRename(ctx, deletedPath, path); err != nil {
			Logc(ctx).Error(err)
		}
		return deleteError
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASQtreeStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Publish",
		"Type":   "NASQtreeStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	volumePattern, name, err := d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
		d.FlexvolNamePrefix())
	if err != nil {
		return err
	}
	// Check if qtree exists, and find its Flexvol so we can build the export location
	exists, flexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing qtree. %s", err.Error())
		return errors.New("volume mount failed")
	}
	if !exists {
		Logc(ctx).WithField("qtree", name).Debug("Qtree not found.")
		return fmt.Errorf("volume %s not found", name)
	}

	// If qtree exists, update the volConfig.InternalID in case it was not set
	// This is useful for "legacy" volumes which do not have InternalID set when they were created
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
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

	return d.publishQtreeShare(ctx, name, flexvol, publishInfo)
}

func (d *NASQtreeStorageDriver) publishQtreeShare(
	ctx context.Context, qtree, flexvol string, publishInfo *utils.VolumePublishInfo,
) error {
	fields := LogFields{
		"Method": "publishQtreeShare",
		"Type":   "ontap_nas_qtree",
		"Share":  qtree,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> publishQtreeShare")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< publishQtreeShare")

	if !d.Config.AutoExportPolicy || publishInfo.Unmanaged {
		return nil
	}

	if err := ensureNodeAccess(ctx, publishInfo, d.API, &d.Config); err != nil {
		return err
	}

	// Ensure the qtree has the correct export policy applied
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	err := d.API.QtreeModifyExportPolicy(ctx, qtree, flexvol, policyName)
	if err != nil {
		err = fmt.Errorf("error modifying qtree export policy; %v", err)
		Logc(ctx).WithFields(LogFields{
			"Qtree":        qtree,
			"FlexVol":      flexvol,
			"ExportPolicy": policyName,
		}).Error(err)
		return err
	}

	// Ensure the qtree's volume has the correct export policy applied
	return publishShare(ctx, d.API, &d.Config, publishInfo, flexvol, d.API.VolumeModifyExportPolicy)
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASQtreeStorageDriver) CanSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "CanSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CanSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CanSnapshot")

	snapshotDirBool, err := strconv.ParseBool(volConfig.SnapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir; %v", err)
	}
	if !snapshotDirBool {
		return errors.UnsupportedError(fmt.Sprintf("snapshots cannot be taken if snapdir access is disabled"))
	}

	return nil
}

// getQtreeSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func getQtreeSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, flexvol string,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName

	fields := LogFields{
		"Method":       "getQtreeSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getQtreeSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getQtreeSnapshot")

	snap, err := client.VolumeSnapshotInfo(ctx, internalSnapName, flexvol)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   snapConfig.VolumeInternalName,
		"created":      snap.CreateTime,
	}).Debug("Found snapshot.")
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snap.CreateTime,
		SizeBytes: int64(0),
		State:     storage.SnapshotStateOnline,
	}, nil
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *NASQtreeStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	_, flexvol, _, err := d.ParseQtreeInternalID(volConfig.InternalID)
	if err != nil {
		return nil, errors.WrapWithNotFoundError(err, "error while getting flexvol")
	}

	return getQtreeSnapshot(ctx, snapConfig, flexvol, &d.Config, d.API)
}

// getQtreeSnapshotList returns the list of snapshots associated with the named volume.
func getQtreeSnapshotList(
	ctx context.Context, volConfig *storage.VolumeConfig, flexvol string,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":     "getQtreeSnapshotList",
		"Type":       "NASQtreeStorageDriver",
		"volumeName": flexvol,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getQtreeSnapshotList")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getQtreeSnapshotList")

	// In the Docker context, Trident does not maintain a snapshot to qtree association.
	// Hence, in the Docker context, we should not return any qtree snapshots.
	if config.DriverContext == tridentconfig.ContextDocker {
		return nil, nil
	}

	snapshots, err := client.VolumeSnapshotList(ctx, flexvol)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Snapshot, 0)

	for _, snap := range snapshots {

		Logc(ctx).WithFields(LogFields{
			"name":       snap.Name,
			"accessTime": snap.CreateTime,
		}).Debug("Found snapshot.")

		snapshot := &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               snap.Name,
				InternalName:       snap.Name,
				VolumeName:         volConfig.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			Created:   snap.CreateTime,
			SizeBytes: int64(0),
			State:     storage.SnapshotStateOnline,
		}

		result = append(result, snapshot)
	}

	return result, nil
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *NASQtreeStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "NASQtreeStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	_, flexvol, _, err := d.ParseQtreeInternalID(volConfig.InternalID)
	if err != nil {
		return nil, errors.WrapWithNotFoundError(err, "error while getting flexvol")
	}

	return getQtreeSnapshotList(ctx, volConfig, flexvol, &d.Config, d.API)
}

// createQtreeSnapshot creates a snapshot for the given volume.
func createQtreeSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, flexvol string,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	fields := LogFields{
		"Method":       "createQtreeSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   flexvol,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createQtreeSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createQtreeSnapshot")

	// If the specified volume doesn't exist, return error
	volExists, err := client.VolumeExists(ctx, flexvol)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		return nil, fmt.Errorf("volume %s does not exist", flexvol)
	}

	if err = client.VolumeSnapshotCreate(ctx, internalSnapName, flexvol); err != nil {
		return nil, err
	}

	snap, err := client.VolumeSnapshotInfo(ctx, internalSnapName, flexvol)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   snapConfig.VolumeInternalName,
		"created":      snap.CreateTime,
	}).Debug("Snapshot created.")
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snap.CreateTime,
		SizeBytes: int64(0),
		State:     storage.SnapshotStateOnline,
	}, nil
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASQtreeStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"sourceVolume": snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureReadOnlyClone); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not create snapshot.")
		return nil, err
	}

	if volConfig.ReadOnlyClone {
		// This is a read-only volume and hence do not create snapshot of it
		return nil, fmt.Errorf("snapshot is not supported for a read-only volume")
	}
	_, flexvol, _, err := d.ParseQtreeInternalID(volConfig.InternalID)
	if err != nil {
		return nil, errors.WrapWithNotFoundError(err, "error while getting flexvol")
	}

	return createQtreeSnapshot(ctx, snapConfig, flexvol, &d.Config, d.API)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASQtreeStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"sourceVolume": snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return errors.UnsupportedError(fmt.Sprintf("snapshot restore is not supported by backend type %s", d.Name()))
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *NASQtreeStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, volConfig *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "NASQtreeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	_, flexvol, _, err := d.ParseQtreeInternalID(volConfig.InternalID)
	if err != nil {
		return errors.WrapWithNotFoundError(err, "error while getting flexvol")
	}

	if err = d.API.VolumeSnapshotDelete(ctx, snapConfig.InternalName, flexvol); err != nil {
		if api.IsSnapshotBusyError(err) {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, &d.Config, d.API,
				d.API.VolumeCloneSplitStart, d.cloneSplitTimers)
		}

		// We must return the error, even if we started a split, so the snapshot delete is retried.
		return err
	}

	// Clean up any split timer.
	delete(d.cloneSplitTimers, snapConfig.ID())

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// Get tests for the existence of a volume
func (d *NASQtreeStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "NASQtreeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	// Generic user-facing message
	getError := fmt.Errorf("volume %s not found", name)

	volConfig := &storage.VolumeConfig{
		InternalName: name,
	}

	volumePattern, name, err := d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
		d.FlexvolNamePrefix())
	if err != nil {
		return err
	}
	// Check that volume exists
	exists, flexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing qtree. %s", err.Error())
		return getError
	}
	if !exists {
		Logc(ctx).WithField("qtree", name).Debug("Qtree not found.")
		return getError
	}

	// If qtree exists, update the volConfig.InternalID in case it was not set
	// This is useful for "legacy" volumes which do not have InternalID set when they were created
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
	}

	Logc(ctx).WithFields(LogFields{"qtree": name, "flexvol": flexvol}).Debug("Qtree found.")

	return nil
}

// ensureFlexvolForQtree accepts a set of Flexvol characteristics and either finds one to contain a new
// qtree or it creates a new Flexvol with the needed attributes.
func (d *NASQtreeStorageDriver) ensureFlexvolForQtree(
	ctx context.Context, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, enableSnapshotDir bool,
	enableEncryption *bool, sizeBytes uint64, config drivers.OntapStorageDriverConfig, snapshotReserve,
	exportPolicy string,
) (string, error) {
	shouldLimitVolumeSize, flexvolQuotaSizeLimit, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, config.CommonStorageDriverConfig)
	if checkVolumeSizeLimitsError != nil {
		return "", checkVolumeSizeLimitsError
	}

	// Check if a suitable Flexvol already exists
	flexvol, err := d.findFlexvolForQtree(ctx, aggregate, spaceReserve, snapshotPolicy, tieringPolicy, snapshotReserve,
		enableSnapshotDir, enableEncryption, shouldLimitVolumeSize, sizeBytes, flexvolQuotaSizeLimit)
	if err != nil {
		return "", fmt.Errorf("error finding Flexvol for qtree: %v", err)
	}

	// Found one!
	if flexvol != "" {
		return flexvol, nil
	}

	// Nothing found, so create a suitable Flexvol
	flexvol, err = d.createFlexvolForQtree(
		ctx, aggregate, spaceReserve, snapshotPolicy, tieringPolicy, enableSnapshotDir, enableEncryption,
		snapshotReserve, exportPolicy)
	if err != nil {
		return "", fmt.Errorf("error creating Flexvol for qtree: %v", err)
	}

	return flexvol, nil
}

// createFlexvolForQtree creates a new Flexvol matching the specified attributes for
// the purpose of containing qtrees supplied as container volumes by this driver.
// Once this method returns, the Flexvol exists, is mounted, and has a default tree
// quota.
func (d *NASQtreeStorageDriver) createFlexvolForQtree(
	ctx context.Context, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, enableSnapshotDir bool,
	enableEncryption *bool, snapshotReserve, exportPolicy string,
) (string, error) {
	var unixPermissions string
	var securityStyle string

	flexvol := d.FlexvolNamePrefix() + utils.RandomString(10)
	size := "1g"

	if !d.Config.AutoExportPolicy {
		exportPolicy = d.flexvolExportPolicy
	}

	// Set Unix permission for NFS volume only.
	switch d.Config.NASType {
	case sa.SMB:
		unixPermissions = ""
		securityStyle = DefaultSecurityStyleSMB
	case sa.NFS:
		unixPermissions = "0711"
		securityStyle = DefaultSecurityStyleNFS
	}

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return "", fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	Logc(ctx).WithFields(LogFields{
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
		"encryption":      utils.GetPrintableBoolPtrValue(enableEncryption),
	}).Debug("Creating Flexvol for qtrees.")

	// Create the Flexvol
	err = d.API.VolumeCreate(ctx, api.Volume{
		Aggregates: []string{
			aggregate,
		},
		Encrypt:         enableEncryption,
		ExportPolicy:    exportPolicy,
		Name:            flexvol,
		Qos:             api.QosPolicyGroup{},
		SecurityStyle:   securityStyle,
		Size:            size,
		SnapshotDir:     enableSnapshotDir,
		SnapshotPolicy:  snapshotPolicy,
		SnapshotReserve: snapshotReserveInt,
		SpaceReserve:    spaceReserve,
		TieringPolicy:   tieringPolicy,
		UnixPermissions: unixPermissions,
		DPVolume:        false,
	})
	if err != nil {
		return "", fmt.Errorf("error creating Flexvol: %v", err)
	}

	// Disable '.snapshot' as needed
	if !enableSnapshotDir {
		err := d.API.VolumeModifySnapshotDirectoryAccess(ctx, flexvol, false)
		if err != nil {
			if err := d.API.VolumeDestroy(ctx, flexvol, true); err != nil {
				Logc(ctx).Error(err)
			}
			return "", fmt.Errorf("error disabling snapshot directory access: %v", err)
		}
	}

	// Mount the volume at the specified junction
	err = d.API.VolumeMount(ctx, flexvol, "/"+flexvol)
	if err != nil {
		if err := d.API.VolumeDestroy(ctx, flexvol, true); err != nil {
			Logc(ctx).Error(err)
		}
		return "", fmt.Errorf("error mounting Flexvol: %v", err)
	}

	// Create the default quota rule so we can use quota-resize for new qtrees
	err = d.addDefaultQuotaForFlexvol(ctx, flexvol)
	if err != nil {
		if err := d.API.VolumeDestroy(ctx, flexvol, true); err != nil {
			Logc(ctx).Error(err)
		}
		return "", fmt.Errorf("error adding default quota to Flexvol: %v", err)
	}

	return flexvol, nil
}

// findFlexvolForQtree returns a Flexvol (from the set of existing Flexvols) that
// matches the specified Flexvol attributes and does not already contain more
// than the maximum configured number of qtrees.  No matching Flexvols is not
// considered an error.  If more than one matching Flexvol is found, one of those
// is returned at random.
func (d *NASQtreeStorageDriver) findFlexvolForQtree(
	ctx context.Context, aggregate, spaceReserve, snapshotPolicy, tieringPolicy, snapshotReserve string,
	enableSnapshotDir bool, enableEncryption *bool, shouldLimitFlexvolQuotaSize bool,
	sizeBytes, flexvolQuotaSizeLimit uint64,
) (string, error) {
	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return "", fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	// Get all volumes matching the specified attributes
	volAttrs := &api.Volume{
		Aggregates:      []string{aggregate},
		Encrypt:         enableEncryption,
		Name:            d.FlexvolNamePrefix() + "*",
		SnapshotDir:     enableSnapshotDir,
		SnapshotPolicy:  snapshotPolicy,
		SpaceReserve:    spaceReserve,
		SnapshotReserve: snapshotReserveInt,
		TieringPolicy:   tieringPolicy,
	}
	volumes, err := d.API.VolumeListByAttrs(ctx, volAttrs)
	if err != nil {
		return "", fmt.Errorf("error listing volumes; %v", err)
	}

	// Weed out the Flexvols:
	// 1) already having too many qtrees
	// 2) exceeding size limits
	var eligibleVolumeNames []string
	for _, volume := range volumes {
		volName := volume.Name

		// skip flexvols over the size limit
		if shouldLimitFlexvolQuotaSize {
			sizeWithRequest, err := d.getOptimalSizeForFlexvol(ctx, volName, sizeBytes)
			if err != nil {
				Logc(ctx).Errorf("Error checking size for existing qtree. %v %v", volName, err)
				continue
			}
			if sizeWithRequest > flexvolQuotaSizeLimit {
				Logc(ctx).Debugf("Flexvol quota size for %v is over the limit of %v", volName,
					flexvolQuotaSizeLimit)
				continue
			}
		}

		count, err := d.API.QtreeCount(ctx, volName)
		if err != nil {
			return "", fmt.Errorf("error enumerating qtrees: %v", err)
		}

		if count < d.qtreesPerFlexvol {
			eligibleVolumeNames = append(eligibleVolumeNames, volName)
		}
	}

	// Pick a Flexvol.  If there are multiple matches, pick one at random.
	switch len(eligibleVolumeNames) {
	case 0:
		return "", nil
	case 1:
		return eligibleVolumeNames[0], nil
	default:
		return eligibleVolumeNames[crypto.GetRandomNumber(len(eligibleVolumeNames))], nil
	}
}

// getOptimalSizeForFlexvol sums up all the disk limit quota rules on a Flexvol and adds the size of
// the new qtree being added as well as the current Flexvol snapshot reserve.  This value may be used
// to grow (or shrink) the Flexvol as new qtrees are being added.
func (d *NASQtreeStorageDriver) getOptimalSizeForFlexvol(
	ctx context.Context, flexvol string, newQtreeSizeBytes uint64,
) (uint64, error) {
	// Get more info about the Flexvol
	volAttrs, err := d.API.VolumeInfo(ctx, flexvol)
	if err != nil {
		return 0, err
	}
	totalDiskLimitBytes, err := d.getTotalHardDiskLimitQuota(ctx, flexvol)
	if err != nil {
		return 0, err
	}

	return calculateFlexvolEconomySizeBytes(ctx, flexvol, volAttrs, newQtreeSizeBytes,
		totalDiskLimitBytes), nil
}

// addDefaultQuotaForFlexvol adds a default quota rule to a Flexvol so that quotas for
// new qtrees may be added on demand with simple quota resize instead of a heavyweight
// quota reinitialization.
func (d *NASQtreeStorageDriver) addDefaultQuotaForFlexvol(ctx context.Context, flexvol string) error {
	err := d.API.QuotaSetEntry(ctx, "", flexvol, "tree", "")
	if err != nil {
		return fmt.Errorf("error adding default quota: %v", err)
	}

	if err := d.disableQuotas(ctx, flexvol, true); err != nil {
		Logc(ctx).Warningf("Could not disable quotas after adding a default quota: %v", err)
	}

	if err := d.enableQuotas(ctx, flexvol, true); err != nil {
		Logc(ctx).Warningf("Could not enable quotas after adding a default quota: %v", err)
	}

	return nil
}

// setQuotaForQtree adds a tree quota to a Flexvol/qtree with a hard disk size limit if it doesn't exist.
// If the quota already exists the hard disk size limit is updated.
func (d *NASQtreeStorageDriver) setQuotaForQtree(ctx context.Context, qtree, flexvol string, sizeBytes uint64) error {
	sizeKB := strconv.FormatUint(sizeBytes/1024, 10)

	err := d.API.QuotaSetEntry(ctx, qtree, flexvol, "tree", sizeKB)
	if err != nil {
		return fmt.Errorf("error adding qtree quota: %v", err)
	}

	// Mark this Flexvol as needing a quota resize
	d.quotaResizeMap[flexvol] = true

	return nil
}

// getQuotaDiskLimitSize returns the disk limit size for the specified quota.
func (d *NASQtreeStorageDriver) getQuotaDiskLimitSize(ctx context.Context, name, flexvol string) (int64, error) {
	quota, err := d.API.QuotaGetEntry(ctx, flexvol, name, "tree")
	if err != nil {
		return 0, err
	}

	quotaSize := quota.DiskLimitBytes

	return quotaSize, nil
}

// enableQuotas disables quotas on a Flexvol, optionally waiting for the operation to finish.
func (d *NASQtreeStorageDriver) disableQuotas(ctx context.Context, flexvol string, wait bool) error {
	status, err := d.API.QuotaStatus(ctx, flexvol)
	if err != nil {
		return fmt.Errorf("error disabling quotas: %v", err)
	}
	if status == "corrupt" {
		return fmt.Errorf("error disabling quotas: quotas are corrupt on Flexvol %s", flexvol)
	}

	if status != "off" {
		err := d.API.QuotaOff(ctx, flexvol)
		if err != nil {
			return fmt.Errorf("error disabling quotas: %v", err)
		}
	}

	if wait {
		for status != "off" {
			time.Sleep(1 * time.Second)

			status, err = d.API.QuotaStatus(ctx, flexvol)
			if err != nil {
				return fmt.Errorf("error disabling quotas: %v", err)
			}
			if status == "corrupt" {
				return fmt.Errorf("error disabling quotas: quotas are corrupt on flexvol %s", flexvol)
			}
		}
	}

	return nil
}

// enableQuotas enables quotas on a Flexvol, optionally waiting for the operation to finish.
func (d *NASQtreeStorageDriver) enableQuotas(ctx context.Context, flexvol string, wait bool) error {
	status, err := d.API.QuotaStatus(ctx, flexvol)
	if err != nil {
		return fmt.Errorf("error enabling quotas: %v", err)
	}
	if status == "corrupt" {
		return fmt.Errorf("error enabling quotas: quotas are corrupt on flexvol %s", flexvol)
	}

	if status == "off" {
		err := d.API.QuotaOn(ctx, flexvol)
		if err != nil {
			return fmt.Errorf("error enabling quotas: %v", err)
		}
	}

	if wait {
		for status != "on" {
			time.Sleep(1 * time.Second)

			status, err = d.API.QuotaStatus(ctx, flexvol)
			if err != nil {
				return fmt.Errorf("error enabling quotas: %v", err)
			}
			if status == "corrupt" {
				return fmt.Errorf("error enabling quotas: quotas are corrupt on flexvol %s", flexvol)
			}
		}
	}

	return nil
}

// queueAllFlexvolsForQuotaResize flags every Flexvol managed by this driver as
// needing a quota resize.  This is called once on driver startup to handle the
// case where the driver was shut down with pending quota resize operations.
func (d *NASQtreeStorageDriver) queueAllFlexvolsForQuotaResize(ctx context.Context) {
	// Get list of Flexvols managed by this driver
	volumes, err := d.API.VolumeListByPrefix(ctx, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error listing Flexvols: %v", err)
	}

	for _, volume := range volumes {
		d.quotaResizeMap[volume.Name] = true
	}
}

// resizeQuotas may be called by a background task, or by a method that changed
// the qtree population on a Flexvol.  Flexvols needing an update must be flagged
// in quotaResizeMap.  Any failures that occur are simply logged, and the resize
// operation will be attempted each time this method is called until it succeeds.
func (d *NASQtreeStorageDriver) resizeQuotas(ctx context.Context) {
	// Ensure we don't forget any Flexvol that is involved in a qtree provisioning workflow
	utils.Lock(ctx, "resize", d.sharedLockID)
	defer utils.Unlock(ctx, "resize", d.sharedLockID)

	Logc(ctx).Debug("Housekeeping, resizing quotas.")

	for flexvol, resize := range d.quotaResizeMap {
		if resize {
			err := d.API.QuotaResize(ctx, flexvol)
			if err != nil {
				if errors.IsNotFoundError(err) {
					// Volume gone, so no need to try again
					Logc(ctx).WithField("flexvol", flexvol).Debug("Volume does not exist.")
					delete(d.quotaResizeMap, flexvol)
				}
				Logc(ctx).WithFields(LogFields{"flexvol": flexvol, "error": err}).Debug("Error resizing quotas.")
				continue
			}

			Logc(ctx).WithField("flexvol", flexvol).Debug("Started quota resize.")

			// Resize start succeeded, so no need to try again
			delete(d.quotaResizeMap, flexvol)
		}
	}
}

// getTotalHardDiskLimitQuota returns the sum of all disk limit quota rules on a Flexvol
func (d *NASQtreeStorageDriver) getTotalHardDiskLimitQuota(ctx context.Context, flexvol string) (uint64, error) {
	quotaEntries, err := d.API.QuotaEntryList(ctx, flexvol)
	if err != nil {
		return 0, err
	}

	var totalDiskLimitBytes uint64
	for _, rule := range quotaEntries {
		totalDiskLimitBytes += uint64(rule.DiskLimitBytes)
	}

	return totalDiskLimitBytes, nil
}

// pruneUnusedFlexvols is called periodically by a background task.  Any Flexvols
// that are managed by this driver (discovered by virtue of having a well-known
// hardcoded prefix on their names) that have no qtrees are deleted.
func (d *NASQtreeStorageDriver) pruneUnusedFlexvols(ctx context.Context) {
	// Ensure we don't prune any Flexvol that is involved in a qtree provisioning workflow
	utils.Lock(ctx, "prune", d.sharedLockID)
	defer utils.Unlock(ctx, "prune", d.sharedLockID)

	Logc(ctx).Debug("Housekeeping, checking for managed Flexvols with no qtrees.")

	// Get list of Flexvols managed by this driver
	volumes, err := d.API.VolumeListByPrefix(ctx, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithError(err).Error("Could not list Flexvols.")
		return
	}

	var flexvols []string
	for _, volume := range volumes {
		flexvols = append(flexvols, volume.Name)
	}

	// Update map of empty Flexvols
	for _, flexvol := range flexvols {

		qtreeCount, err := d.API.QtreeCount(ctx, flexvol)
		if err != nil {
			// Couldn't count qtrees, so remove Flexvol from deletion map as a precaution
			Logc(ctx).WithFields(LogFields{
				"flexvol": flexvol,
				"error":   err,
			}).Warning("Could not count qtrees in Flexvol.")
			delete(d.emptyFlexvolMap, flexvol)
		} else if qtreeCount == 0 {
			// No qtrees exist, so add Flexvol to map if it isn't there already
			if _, ok := d.emptyFlexvolMap[flexvol]; !ok {
				Logc(ctx).WithField("flexvol", flexvol).Debug("Flexvol has no qtrees, saving to delete deferral map.")
				d.emptyFlexvolMap[flexvol] = time.Now()
			} else {
				Logc(ctx).WithField("flexvol", flexvol).Debug("Flexvol has no qtrees, already in delete deferral map.")
			}
		} else {
			// Qtrees exist, so ensure Flexvol isn't in deletion map
			Logc(ctx).WithFields(LogFields{"flexvol": flexvol, "qtrees": qtreeCount}).Debug("Flexvol has qtrees.")
			delete(d.emptyFlexvolMap, flexvol)
		}
	}

	// Destroy any Flexvol if it is devoid of qtrees and has remained empty for the configured time to live
	for flexvol, initialEmptyTime := range d.emptyFlexvolMap {

		// If Flexvol is no longer known to the driver, remove from map and move on
		if !utils.StringInSlice(flexvol, flexvols) {
			Logc(ctx).WithField("flexvol", flexvol).Debug(
				"Flexvol no longer extant, removing from delete deferral map.")
			delete(d.emptyFlexvolMap, flexvol)
			continue
		}

		now := time.Now()
		expirationTime := initialEmptyTime.Add(d.emptyFlexvolDeferredDeletePeriod)
		if expirationTime.Before(now) {
			Logc(ctx).WithField("flexvol", flexvol).Debug("Deleting managed Flexvol with no qtrees.")
			err := d.API.VolumeDestroy(ctx, flexvol, true)
			if err != nil {
				Logc(ctx).WithFields(LogFields{"flexvol": flexvol, "error": err}).Error("Could not delete Flexvol.")
			} else {
				delete(d.emptyFlexvolMap, flexvol)
			}
		} else {
			Logc(ctx).WithFields(LogFields{
				"flexvol":          flexvol,
				"timeToExpiration": expirationTime.Sub(now),
			}).Debug("Flexvol with no qtrees not past expiration time.")
		}
	}
}

// reapDeletedQtrees is called periodically by a background task.  Any qtrees
// that have been deleted (discovered by virtue of having a well-known hardcoded
// prefix on their names) are destroyed.  This is only needed for the exceptional case
// in which a qtree was renamed (prior to being destroyed) but the subsequent
// destroy call failed or was never made due to a process interruption.
func (d *NASQtreeStorageDriver) reapDeletedQtrees(ctx context.Context) {
	// Ensure we don't reap any qtree that is involved in a qtree delete workflow
	utils.Lock(ctx, "reap", d.sharedLockID)
	defer utils.Unlock(ctx, "reap", d.sharedLockID)

	Logc(ctx).Debug("Housekeeping, checking for deleted qtrees.")

	// Get all deleted qtrees in all Flexvols managed by this driver
	prefix := deletedQtreeNamePrefix + *d.Config.StoragePrefix
	qtrees, err := d.API.QtreeListByPrefix(ctx, prefix, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error listing deleted qtrees. %v", err)
		return
	}

	for _, qtree := range qtrees {
		qtreePath := fmt.Sprintf("/vol/%s/%s", qtree.Volume, qtree.Name)
		Logc(ctx).WithField("qtree", qtreePath).Debug("Housekeeping, reaping deleted qtree.")
		if err := d.API.QtreeDestroyAsync(ctx, qtreePath, true); err != nil {
			Logc(ctx).Error(err)
		}
	}
}

// ensureDefaultExportPolicy checks for an export policy with a well-known name that will be suitable
// for setting on a Flexvol and will enable access to all qtrees therein.  If the policy exists, the
// method assumes it created the policy itself and that all is good.  If the policy does not exist,
// it is created and populated with a rule that allows access to NFS qtrees.  This method should be
// called once during driver initialization.
func (d *NASQtreeStorageDriver) ensureDefaultExportPolicy(ctx context.Context) error {
	err := d.API.ExportPolicyCreate(ctx, d.flexvolExportPolicy)
	if err != nil {
		return fmt.Errorf("error creating export policy %s: %v", d.flexvolExportPolicy, err)
	}
	return d.ensureDefaultExportPolicyRule(ctx)
}

// ensureDefaultExportPolicyRule guarantees that the export policy used on Flexvols managed by this
// driver has at least one rule, which is necessary (but not always sufficient) to enable qtrees
// to be mounted by clients.
func (d *NASQtreeStorageDriver) ensureDefaultExportPolicyRule(ctx context.Context) error {
	ruleList, err := d.API.ExportRuleList(ctx, d.flexvolExportPolicy)
	if err != nil {
		return fmt.Errorf("error listing export policy rules: %v", err)
	}

	if len(ruleList) == 0 {

		// No rules, so create one for IPv4 and IPv6
		rules := []string{"0.0.0.0/0", "::/0"}
		for _, rule := range rules {
			err := d.API.ExportRuleCreate(ctx, d.flexvolExportPolicy, rule, d.Config.NASType)
			if err != nil {
				return fmt.Errorf("error creating export rule: %v", err)
			}
		}
	} else {
		Logc(ctx).WithField("exportPolicy", d.flexvolExportPolicy).Debug("Export policy has at least one rule.")
	}

	return nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *NASQtreeStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NASQtreeStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *NASQtreeStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapEconomyStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "NASQtreeStorageDriver"}
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

func (d *NASQtreeStorageDriver) getStoragePoolAttributes() map[string]sa.Offer {
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(false),
		sa.Clones:           sa.NewBoolOffer(false),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(false),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASQtreeStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *NASQtreeStorageDriver) GetInternalVolumeName(ctx context.Context, name string) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// With an external store, any transformation of the name is fine
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = strings.Replace(internal, "-", "_", -1)  // ONTAP disallows hyphens
		internal = strings.Replace(internal, ".", "_", -1)  // ONTAP disallows periods
		internal = strings.Replace(internal, "__", "_", -1) // Remove any double underscores

		if len(internal) > 64 {
			// ONTAP imposes a 64-character limit on qtree names.  We are unlikely to exceed
			// that with CSI unless the storage prefix is really long, but non-CSI can hit the
			// limit more easily.  If the computed name is over the limit, the safest approach is
			// simply to generate a new name.
			internal = fmt.Sprintf("%s_%s",
				strings.Replace(drivers.GetDefaultStoragePrefix(d.Config.DriverContext), "_", "", -1),
				strings.Replace(uuid.New().String(), "-", "", -1))

			Logc(ctx).WithFields(LogFields{
				"Name":         name,
				"InternalName": internal,
			}).Debug("Created UUID-based name for ontap-nas-economy volume.")
		}

		return internal
	}
}

func (d *NASQtreeStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	createPrepareCommon(ctx, d, volConfig)
}

func (d *NASQtreeStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	// Determine which Flexvol contains the qtree
	var volumePattern, name string
	var err error

	if volConfig.ReadOnlyClone {
		volumePattern, name, err = d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID,
			volConfig.CloneSourceVolumeInternal,
			d.FlexvolNamePrefix())
	} else {
		volumePattern, name, err = d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
			d.FlexvolNamePrefix())
	}
	if err != nil {
		return err
	}

	// Check that volume exists
	exists, flexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		return fmt.Errorf("could not determine if qtree %s exists: %v", volConfig.InternalName, err)
	}
	if !exists {
		return fmt.Errorf("could not find qtree %s", volConfig.InternalName)
	}
	// If qtree exists, update the volConfig.InternalID in case it was not set
	// This is useful for "legacy" volumes which do not have InternalID set when they were created
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
	}

	// Set export path info on the volume config
	if d.Config.NASType == sa.SMB {
		volConfig.AccessInfo.SMBServer = d.Config.DataLIF
		volConfig.AccessInfo.SMBPath = ConstructOntapNASQTreeVolumePath(ctx, d.Config.SMBShare, flexvol,
			volConfig, sa.SMB)
		volConfig.FileSystem = sa.SMB
	} else {
		volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
		volConfig.AccessInfo.NfsPath = ConstructOntapNASQTreeVolumePath(ctx, "", flexvol, volConfig, sa.NFS)
		volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
		volConfig.FileSystem = sa.NFS
	}

	return nil
}

func (d *NASQtreeStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NASQtreeStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASQtreeStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NASQtreeStorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {
	qtree, err := d.API.QtreeGetByName(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		return nil, err
	}

	volume, err := d.API.VolumeInfo(ctx, qtree.Volume)
	if err != nil {
		return nil, err
	}

	quota, err := d.API.QuotaGetEntry(ctx, qtree.Volume, qtree.Name, "tree")
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(qtree, volume, quota), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASQtreeStorageDriver) GetVolumeExternalWrappers(
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

	// Get all qtrees in all Flexvols matching the storage prefix
	qtrees, err := d.API.QtreeListByPrefix(ctx, "", d.FlexvolNamePrefix())
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Bail out early if there aren't any qtrees
	if len(qtrees) == 0 {
		return
	}

	// Get all quotas in all Flexvols matching the storage prefix
	quotaEntries, err := d.API.QuotaEntryList(ctx, d.FlexvolNamePrefix()+"*")
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with qtrees
	volumeMap := make(map[string]*api.Volume)
	for _, volume := range volumes {
		volumeMap[volume.Name] = volume
	}

	// Make a map of quotas for faster correlation with qtrees
	quotaMap := make(map[string]*api.QuotaEntry)
	for _, quotaEntry := range quotaEntries {
		quotaMap[quotaEntry.Target] = quotaEntry
	}

	// Convert all qtrees to VolumeExternal and write them to the channel
	for _, qtree := range qtrees {

		// Ignore Flexvol-level qtrees
		if qtree.Name == "" {
			continue
		}

		// Don't include deleted qtrees
		if strings.HasPrefix(qtree.Name, deletedQtreeNamePrefix) {
			continue
		}

		volume, ok := volumeMap[qtree.Volume]
		if !ok {
			Logc(ctx).WithField("qtree", qtree.Name).Warning("Flexvol not found for qtree.")
			continue
		}

		quotaTarget := fmt.Sprintf("/vol/%s/%s", qtree.Volume, qtree.Name)
		quota, ok := quotaMap[quotaTarget]
		if !ok {
			Logc(ctx).WithField("qtree", qtree.Name).Warning("Quota rule not found for qtree.")
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(qtree, volume, quota), Error: nil}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASQtreeStorageDriver) getVolumeExternal(
	qtree *api.Qtree, volume *api.Volume, quota *api.QuotaEntry,
) *storage.VolumeExternal {
	internalName := qtree.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	size := quota.DiskLimitBytes

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(size, 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  volume.SnapshotPolicy,
		ExportPolicy:    qtree.ExportPolicy,
		SnapshotDir:     strconv.FormatBool(volume.SnapshotDir),
		UnixPermissions: qtree.UnixPermissions,
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
		InternalID:      d.CreateQtreeInternalID(d.Config.SVM, qtree.Volume, qtree.Name),
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volume.Aggregates[0],
	}
}

func convertDiskLimitToBytes(diskLimitKB int64) int64 {
	size := diskLimitKB * 1024 // convert KB to bytes
	return size
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASQtreeStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASQtreeStorageDriver)
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

type HousekeepingTask struct {
	Name         string
	Ticker       *time.Ticker
	InitialDelay time.Duration
	Done         chan struct{}
	Tasks        []func(context.Context)
	Driver       *NASQtreeStorageDriver
	stopped      bool
}

func (t *HousekeepingTask) Start(ctx context.Context) {
	go func() {
		t.Driver.housekeepingWaitGroup.Add(1)
		defer t.Driver.housekeepingWaitGroup.Done()
		time.Sleep(t.InitialDelay)
		t.run(ctx, time.Now())
		for {
			select {
			case tick := <-t.Ticker.C:
				t.run(ctx, tick)
			case <-t.Done:
				Logc(ctx).WithFields(LogFields{
					"driver": t.Driver.Name(),
					"task":   t.Name,
				}).Debugf("Shut down housekeeping tasks for the driver.")
				return
			}
		}
	}()
}

func (t *HousekeepingTask) Stop(ctx context.Context) {
	if !t.stopped {
		if t.Ticker != nil {
			t.Ticker.Stop()
		}
		close(t.Done)
		t.stopped = true
		// Run the housekeeping tasks one last time
		for _, task := range t.Tasks {
			task(ctx)
		}
	}
}

func (t *HousekeepingTask) run(ctx context.Context, tick time.Time) {
	for i, task := range t.Tasks {
		Logc(ctx).WithFields(LogFields{
			"tick":   tick,
			"driver": t.Driver.Name(),
			"task":   t.Name,
		}).Debugf("Performing housekeeping task %d.", i)
		task(ctx)
	}
}

func NewPruneTask(ctx context.Context, d *NASQtreeStorageDriver, tasks []func(context.Context)) *HousekeepingTask {
	// Read background task timings from config file, use defaults if missing or invalid
	pruneFlexvolsPeriod := defaultPruneFlexvolsPeriod
	if d.Config.QtreePruneFlexvolsPeriod != "" {
		i, err := strconv.ParseInt(d.Config.QtreePruneFlexvolsPeriod, 10, 64)
		if i < 0 {
			err = fmt.Errorf("negative interval")
		}
		if err != nil {
			Logc(ctx).WithField("defaultInterval", pruneFlexvolsPeriod).WithError(err).Warnf(
				"Invalid Flexvol pruning interval, using default interval.")
		} else {
			pruneFlexvolsPeriod = time.Duration(i) * time.Second
		}
	}
	emptyFlexvolDeferredDeletePeriod := defaultEmptyFlexvolDeferredDeletePeriod
	if d.Config.EmptyFlexvolDeferredDeletePeriod != "" {
		i, err := strconv.ParseInt(d.Config.EmptyFlexvolDeferredDeletePeriod, 10, 64)
		if i < 0 {
			err = fmt.Errorf("negative interval")
		}
		if err != nil {
			Logc(ctx).WithField("defaultInterval", emptyFlexvolDeferredDeletePeriod).WithError(err).Warnf(
				"Invalid Flexvol deferred delete period, using default interval.")
		} else {
			emptyFlexvolDeferredDeletePeriod = time.Duration(i) * time.Second
		}
	}
	d.emptyFlexvolDeferredDeletePeriod = emptyFlexvolDeferredDeletePeriod
	Logc(ctx).WithFields(LogFields{
		"IntervalSeconds": pruneFlexvolsPeriod,
		"EmptyFlexvolTTL": emptyFlexvolDeferredDeletePeriod,
	}).Debug("Configured Flexvol pruning period.")

	task := &HousekeepingTask{
		Name:         pruneTask,
		Ticker:       time.NewTicker(pruneFlexvolsPeriod),
		InitialDelay: HousekeepingStartupDelaySecs * time.Second,
		Done:         make(chan struct{}),
		Tasks:        tasks,
		Driver:       d,
	}

	return task
}

func NewResizeTask(ctx context.Context, d *NASQtreeStorageDriver, tasks []func(context.Context)) *HousekeepingTask {
	// Read background task timings from config file, use defaults if missing or invalid
	resizeQuotasPeriod := defaultResizeQuotasPeriod
	if d.Config.QtreeQuotaResizePeriod != "" {
		i, err := strconv.ParseInt(d.Config.QtreeQuotaResizePeriod, 10, 64)
		if err != nil {
			Logc(ctx).WithField("interval", d.Config.QtreeQuotaResizePeriod).Warnf(
				"Invalid quota resize interval. %v", err)
		} else {
			resizeQuotasPeriod = time.Duration(i) * time.Second
		}
	}
	Logc(ctx).WithFields(LogFields{
		"IntervalSeconds": resizeQuotasPeriod,
	}).Debug("Configured quota resize period.")

	task := &HousekeepingTask{
		Name:         resizeTask,
		Ticker:       time.NewTicker(resizeQuotasPeriod),
		InitialDelay: HousekeepingStartupDelaySecs * time.Second,
		Done:         make(chan struct{}),
		Tasks:        tasks,
		Driver:       d,
	}

	return task
}

// Resize expands the Flexvol containing the Qtree and updates the Qtree quota.
func (d *NASQtreeStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":    "Resize",
		"Type":      "NASQtreeStorageDriver",
		"name":      name,
		"sizeBytes": sizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	if volConfig.ReadOnlyClone {
		return fmt.Errorf("resizing is not supported on a read-only volume")
	}

	if sizeBytes > math.MaxInt64 {
		return fmt.Errorf("invalid volume size")
	}

	// Ensure any Flexvol won't be pruned before resize is completed.
	utils.Lock(ctx, "resize", d.sharedLockID)
	defer utils.Unlock(ctx, "resize", d.sharedLockID)

	// Generic user-facing message
	resizeError := errors.New("storage driver failed to resize the volume")

	volumePattern, name, err := d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
		d.FlexvolNamePrefix())
	if err != nil {
		return err
	}
	// Check that volume exists
	exists, flexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Error checking for existing volume.")
		return resizeError
	}
	if !exists {
		Logc(ctx).WithFields(LogFields{"qtree": name, "flexvol": flexvol}).Debug("Qtree does not exist.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	// If qtree exists, update the volConfig.InternalID in case it was not set
	// This is useful for "legacy" volumes which do not have InternalID set when they were created
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
	}

	// Calculate the delta size needed to resize the Qtree quota
	quotaSize, err := d.getQuotaDiskLimitSize(ctx, name, flexvol)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Failed to determine quota size.")
		return resizeError
	}

	volConfig.Size = strconv.FormatInt(quotaSize, 10)
	if int64(sizeBytes) == quotaSize {
		Logc(ctx).Infof("Requested size and existing volume size are the same for volume %s.", name)
		return nil
	}

	if quotaSize >= 0 && int64(sizeBytes) < quotaSize {
		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, quotaSize)
	}
	deltaQuotaSize := int64(sizeBytes) - quotaSize

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
		ctx, flexvol, sizeBytes, d.Config, d.GetAPI(),
	); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	err = d.resizeFlexvol(ctx, flexvol, uint64(deltaQuotaSize))
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Failed to resize flexvol.")
		return resizeError
	}

	// Update the quota
	err = d.setQuotaForQtree(ctx, name, flexvol, sizeBytes)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Qtree quota update failed.")
		return resizeError
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

// resizeFlexvol grows or shrinks the Flexvol to an optimal size if possible. Otherwise
// the Flexvol is expanded by the value of sizeBytes
func (d *NASQtreeStorageDriver) resizeFlexvol(ctx context.Context, flexvol string, sizeBytes uint64) error {
	flexvolSizeBytes, err := d.getOptimalSizeForFlexvol(ctx, flexvol, sizeBytes)
	if err != nil {
		Logc(ctx).Warnf("Could not calculate optimal Flexvol size. %v", err)
		// Lacking the optimal size, just grow the Flexvol to contain the new qtree
		size := strconv.FormatUint(sizeBytes, 10)
		err := d.API.VolumeSetSize(ctx, flexvol, "+"+size)
		if err != nil {
			return fmt.Errorf("flexvol resize failed: %v", err)
		}
	} else {
		// Got optimal size, so just set the Flexvol to that value
		flexvolSizeStr := strconv.FormatUint(flexvolSizeBytes, 10)
		err := d.API.VolumeSetSize(ctx, flexvol, flexvolSizeStr)
		if err != nil {
			return fmt.Errorf("flexvol resize failed: %v", err)
		}
	}
	return nil
}

func (d *NASQtreeStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*utils.Node, backendUUID, _ string,
) error {
	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "NASQtreeStorageDriver",
		"Nodes":  nodeNames,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccess(ctx, nodes, &d.Config, d.API, policyName)
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *NASQtreeStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "nfs", d.GetStorageBackendPhysicalPoolNames(ctx))
}

// String makes NASQtreeStorageDriver satisfy the Stringer interface.
func (d NASQtreeStorageDriver) String() string {
	return utils.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASQtreeStorageDriver satisfy the GoStringer interface.
func (d NASQtreeStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d NASQtreeStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// ParseQtreeInternalID parses the passed string which is in the format /svm/<svm_name>/flexvol/<flexvol_name>/qtree/<qtree_name>
// and returns svm, flaexvol and qtree name
func (d NASQtreeStorageDriver) ParseQtreeInternalID(internalId string) (svm, flexvol, qtree string, err error) {
	match := QtreeInternalIDRegex.FindStringSubmatch(internalId)
	if match == nil {
		err = fmt.Errorf("internalId ID %s is invalid", internalId)
		return
	}

	paramsMap := make(map[string]string)
	for idx, subExpName := range QtreeInternalIDRegex.SubexpNames() {
		if idx > 0 && idx <= len(match) {
			paramsMap[subExpName] = match[idx]
		}
	}

	svm = paramsMap["svm"]
	flexvol = paramsMap["flexvol"]
	qtree = paramsMap["qtree"]
	return
}

// CreateQtreeInternalID creates a string in the format /svm/<svm_name>/flexvol/<flexvol_name>/qtree/<qtree_name>
func (d NASQtreeStorageDriver) CreateQtreeInternalID(svm, flexvol, name string) string {
	return fmt.Sprintf("/svm/%s/flexvol/%s/qtree/%s", svm, flexvol, name)
}

// SetVolumePatternToFindQtree appropriately sets volumePattern with '*' or with flexvol name depending on internalId
func (d NASQtreeStorageDriver) SetVolumePatternToFindQtree(
	ctx context.Context, internalID, internalName, volumePrefix string,
) (string, string, error) {
	volumePattern := "*"
	qtreeName := internalName
	flexVolumeName := ""
	var err error
	if internalID == "" {
		if volumePrefix != "" {
			volumePattern = volumePrefix + "*"
		}
	} else {
		if _, flexVolumeName, qtreeName, err = d.ParseQtreeInternalID(internalID); err == nil {
			volumePattern = flexVolumeName
		} else {
			return "", "", fmt.Errorf("error in parsing Internal ID %s", internalID)
		}
	}
	fields := LogFields{
		"volumePattern": volumePattern,
		"qtreeName":     qtreeName,
	}
	Logc(ctx).WithFields(fields).Debug("setting volumePattern")

	return volumePattern, qtreeName, nil
}

func (d NASQtreeStorageDriver) getQtreesInPool(poolName string, allVolumes map[string]*storage.Volume) map[string]*storage.Volume {
	poolVolumes := make(map[string]*storage.Volume)

	for _, vol := range allVolumes {
		_, flexvol, _, err := d.ParseQtreeInternalID(vol.Config.InternalID)
		if err == nil && flexvol == poolName {
			poolVolumes[vol.Config.Name] = vol
		}
	}

	return poolVolumes
}

// EnsureSMBShare ensures that required SMB share is made available.
func (d *NASQtreeStorageDriver) EnsureSMBShare(
	ctx context.Context, name string,
) error {
	if d.Config.SMBShare != "" {
		// If user did specify SMB share, and it does not exist, create an SMB share with the specified name.
		share, err := d.API.SMBShareExists(ctx, d.Config.SMBShare)
		if err != nil {
			return err
		}

		// If share is not present create it.
		if !share {
			if err := d.API.SMBShareCreate(ctx, d.Config.SMBShare, "/"); err != nil {
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
			if err := d.API.SMBShareCreate(ctx, name, "/"+name); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *NASQtreeStorageDriver) Update(
	ctx context.Context, volConfig *storage.VolumeConfig,
	updateInfo *utils.VolumeUpdateInfo, allVolumes map[string]*storage.Volume,
) (map[string]*storage.Volume, error) {
	fields := LogFields{
		"Method":     "Update",
		"Type":       "NASQtreeStorageDriver",
		"name":       volConfig.Name,
		"updateInfo": updateInfo,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Update")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Update")

	updateGenericError := fmt.Sprintf("failed to update volume %v", volConfig.Name)

	if updateInfo == nil {
		msg := fmt.Sprintf("nothing to update for volume %v", volConfig.Name)
		err := errors.InvalidInputError(msg)
		Logc(ctx).WithError(err).Error(updateGenericError)
		return nil, err
	}

	// Get the qtree and parent flexvol
	volumePattern, name, err := d.SetVolumePatternToFindQtree(ctx, volConfig.InternalID, volConfig.InternalName,
		d.FlexvolNamePrefix())
	if err != nil {
		return nil, err
	}

	exists, flexvol, err := d.API.QtreeExists(ctx, name, volumePattern)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Error checking for existing volume.")
		return nil, err
	}
	if !exists {
		Logc(ctx).WithFields(LogFields{"qtree": name, "flexvol": flexvol}).Debug("Qtree does not exist.")
		return nil, fmt.Errorf("volume %s does not exist", name)
	}

	var updatedVols map[string]*storage.Volume
	var updateErr error

	// Update snapshot directory
	if updateInfo.SnapshotDirectory != "" {
		updatedVols, updateErr = d.updateSnapshotDirectory(ctx, volConfig, updateInfo.SnapshotDirectory, updateInfo.PoolLevel, flexvol, allVolumes)
		if updateErr != nil {
			Logc(ctx).WithError(updateErr).Error(updateGenericError)
			return nil, updateErr
		}
	}

	// If qtree exists, update the volConfig.InternalID in case it was not set
	// This is useful for "legacy" volumes which do not have InternalID set when they were created
	if volConfig.InternalID == "" {
		volConfig.InternalID = d.CreateQtreeInternalID(d.Config.SVM, flexvol, name)
		Logc(ctx).WithFields(LogFields{"InternalID": volConfig.InternalID}).Debug("setting InternalID")
	}

	return updatedVols, updateErr
}

func (d *NASQtreeStorageDriver) updateSnapshotDirectory(
	ctx context.Context, volConfig *storage.VolumeConfig,
	snapshotDir string, poolLevel bool,
	poolName string, allVolumes map[string]*storage.Volume,
) (map[string]*storage.Volume, error) {
	fields := LogFields{
		"Method":      "updateSnapshotDirectory",
		"Type":        "NASQtreeStorageDriver",
		"name":        volConfig.Name,
		"snapshotDir": snapshotDir,
		"poolLevel":   poolLevel,
		"poolName":    poolName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> updateSnapshotDirectory")
	defer Logc(ctx).WithFields(fields).Debug("<<<< updateSnapshotDirectory")

	genericErrMsg := fmt.Sprintf("Failed to update snapshot directory for %v.", volConfig.Name)

	// Ensure ACP is enabled
	if err := acp.API().IsFeatureEnabled(ctx, acp.FeatureReadOnlyClone); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error(genericErrMsg)
		return nil, err
	}

	// Validate request
	snapDirRequested, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		msg := fmt.Sprintf("invalid value for snapshot directory %v", snapshotDir)
		Logc(ctx).WithError(err).Error(msg)
		inputErr := errors.InvalidInputError(msg)
		return nil, inputErr
	}

	// Ensure poolLevel is always set to true for modifying snapshot directory
	if !poolLevel {
		msg := fmt.Sprintf("pool level must be set to true for updating snapshot directory of %v volume", d.Config.StorageDriverName)
		inputErr := errors.InvalidInputError(msg)
		Logc(ctx).WithError(inputErr).Error(genericErrMsg)
		return nil, inputErr
	}

	// Modify snapshotDirectory access for the pool i.e parent flexvol
	err = d.API.VolumeModifySnapshotDirectoryAccess(ctx, poolName, snapDirRequested)
	if err != nil {
		Logc(ctx).WithError(err).Error(genericErrMsg)
		return nil, err
	}

	// Once snapshot directory is modified for the parent flexvol aka qtree pool,
	// find all volumes belonging to same qtree pool and update their value
	allQtreePoolVols := d.getQtreesInPool(poolName, allVolumes)
	for _, vol := range allQtreePoolVols {
		vol.Config.SnapshotDir = snapshotDir
	}

	return allQtreePoolVols, nil
}
