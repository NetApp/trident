// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
)

const (
	deletedQtreeNamePrefix                      = "deleted_"
	maxQtreeNameLength                          = 64
	minQtreesPerFlexvol                         = 50
	defaultQtreesPerFlexvol                     = 200
	maxQtreesPerFlexvol                         = 300
	defaultPruneFlexvolsPeriodSecs              = uint64(600)   // default to 10 minutes
	defaultResizeQuotasPeriodSecs               = uint64(60)    // default to 1 minute
	defaultEmptyFlexvolDeferredDeletePeriodSecs = uint64(28800) // default to 8 hours
	pruneTask                                   = "prune"
	resizeTask                                  = "resize"
)

// NASQtreeStorageDriver is for NFS storage provisioning of qtrees
type NASQtreeStorageDriver struct {
	initialized                      bool
	Config                           drivers.OntapStorageDriverConfig
	API                              api.OntapAPI
	telemetry                        *TelemetryAbstraction
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
}

func (d *NASQtreeStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASQtreeStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *NASQtreeStorageDriver) GetTelemetry() *TelemetryAbstraction {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d *NASQtreeStorageDriver) Name() string {
	return drivers.OntapNASQtreeStorageDriverName
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

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NASQtreeStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.API, err = InitializeOntapDriverAbstraction(ctx, config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
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
	d.flexvolNamePrefix = strings.Replace(d.flexvolNamePrefix, "__", "_", -1)
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

	Logc(ctx).WithFields(log.Fields{
		"FlexvolNamePrefix":   d.flexvolNamePrefix,
		"FlexvolExportPolicy": d.flexvolExportPolicy,
		"QtreesPerFlexvol":    d.qtreesPerFlexvol,
		"SharedLockID":        d.sharedLockID,
	}).Debugf("Qtree driver settings.")

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommonAbstraction(ctx, d,
		d.getStoragePoolAttributes(), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = d.validate(ctx)
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

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
	d.telemetry = NewOntapTelemetryAbstraction(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	d.initialized = true
	return nil
}

func (d *NASQtreeStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASQtreeStorageDriver) Terminate(ctx context.Context, backendUUID string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASQtreeStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}

	if d.housekeepingWaitGroup != nil {
		for _, task := range d.housekeepingTasks {
			task.Stop(ctx)
		}
	}

	if d.Config.AutoExportPolicy {
		policyName := getExportPolicyName(backendUUID)
		if err := deleteExportPolicyAbstraction(ctx, policyName, d.API); err != nil {
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

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NASQtreeStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	if err := ValidateNASDriverAbstraction(ctx, d.API, &d.Config); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefixEconomy(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePoolsAbstraction(ctx, d.physicalPools, d.virtualPools, d, 0); err != nil {
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

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NASQtreeStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Create")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Create")
	}

	// Ensure any Flexvol we create won't be pruned before we place a qtree on it
	utils.Lock(ctx, "create", d.sharedLockID)
	defer utils.Unlock(ctx, "create", d.sharedLockID)

	// Generic user-facing message
	createError := errors.New("volume creation failed")

	// Ensure volume doesn't already exist
	exists, existsInFlexvol, err := d.API.QtreeExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing volume: %v.", err)
		return createError
	}
	if exists {
		Logc(ctx).WithFields(log.Fields{"qtree": name, "flexvol": existsInFlexvol}).Debug("Qtree already exists.")
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
	opts, err := d.GetVolumeOpts(ctx, volConfig, volAttributes)
	if err != nil {
		return err
	}

	// Get Flexvol options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	var (
		spaceReserve    = utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy  = utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotDir     = utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes()[SnapshotDir])
		encryption      = utils.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		snapshotReserve = storagePool.InternalAttributes()[SnapshotReserve]
		qosPolicy       = storagePool.InternalAttributes()[QosPolicy]
		// Get qtree options with default fallback values
		unixPermissions = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy    = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle   = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		tieringPolicy   = utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
	)

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend().BackendUUID())
	}

	volConfig.QosPolicy = qosPolicy

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimitsAbstraction(
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

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *NASQtreeStorageDriver) CreateClone(
	ctx context.Context, volConfig *storage.VolumeConfig, _ storage.Pool,
) error {

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "NASQtreeStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateClone")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateClone")
	}

	return fmt.Errorf("cloning is not supported by backend type %s", d.Name())
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
func (d *NASQtreeStorageDriver) Destroy(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NASQtreeStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	// Ensure the deleted qtree reaping job doesn't interfere with this workflow
	utils.Lock(ctx, "destroy", d.sharedLockID)
	defer utils.Unlock(ctx, "destroy", d.sharedLockID)

	// Generic user-facing message
	deleteError := errors.New("volume deletion failed")

	exists, flexvol, err := d.API.QtreeExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing qtree. %s", err.Error())
		return deleteError
	}
	if !exists {
		Logc(ctx).WithField("qtree", name).Warn("Qtree not found.")
		return nil
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

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "NASQtreeStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Publish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Publish")
	}

	// Check if qtree exists, and find its Flexvol so we can build the export location
	exists, flexvol, err := d.API.QtreeExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing qtree. %s", err.Error())
		return errors.New("volume mount failed")
	}
	if !exists {
		Logc(ctx).WithField("qtree", name).Debug("Qtree not found.")
		return fmt.Errorf("volume %s not found", name)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = fmt.Sprintf("/%s/%s", flexvol, name)
	publishInfo.NfsServerIP = d.Config.DataLIF
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return d.publishQtreeShare(ctx, name, flexvol, publishInfo)
}

func (d *NASQtreeStorageDriver) publishQtreeShare(
	ctx context.Context, qtree, flexvol string, publishInfo *utils.VolumePublishInfo,
) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "publishQtreeShare",
			"Type":   "ontap_nas_qtree",
			"Share":  qtree,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> publishQtreeShare")
		defer Logc(ctx).WithFields(fields).Debug("<<<< publishQtreeShare")
	}

	if !d.Config.AutoExportPolicy || publishInfo.Unmanaged {
		return nil
	}

	if err := ensureNodeAccessAbstraction(ctx, publishInfo, d.API, &d.Config); err != nil {
		return err
	}

	// Ensure the qtree has the correct export policy applied
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	err := d.API.QtreeModifyExportPolicy(ctx, qtree, flexvol, policyName)
	if err != nil {
		err = fmt.Errorf("error modifying qtree export policy; %v", err)
		Logc(ctx).WithFields(log.Fields{
			"Qtree":        qtree,
			"FlexVol":      flexvol,
			"ExportPolicy": policyName,
		}).Error(err)
		return err
	}

	// Ensure the qtree's volume has the correct export policy applied
	return publishShareAbstraction(ctx, d.API, &d.Config, publishInfo, flexvol, d.API.VolumeModifyExportPolicy)
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASQtreeStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig) error {
	return utils.UnsupportedError(fmt.Sprintf("snapshots are not supported by backend type %s", d.Name()))
}

// GetSnapshot returns a snapshot of a volume, or an error if it does not exist.
func (d *NASQtreeStorageDriver) GetSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) (
	*storage.Snapshot, error,
) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NASQtreeStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return nil, utils.UnsupportedError(fmt.Sprintf("snapshots are not supported by backend type %s", d.Name()))
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *NASQtreeStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NASQtreeStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	// Qtrees can't have snapshots, so return an empty list
	return []*storage.Snapshot{}, nil
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASQtreeStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NASQtreeStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"sourceVolume": snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return nil, utils.UnsupportedError(fmt.Sprintf("snapshots are not supported by backend type %s", d.Name()))
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASQtreeStorageDriver) RestoreSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "NASQtreeStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"sourceVolume": snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return utils.UnsupportedError(fmt.Sprintf("snapshots are not supported by backend type %s", d.Name()))
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASQtreeStorageDriver) DeleteSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NASQtreeStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return utils.UnsupportedError(fmt.Sprintf("snapshots are not supported by backend type %s", d.Name()))
}

// Get tests for the existence of a volume
func (d *NASQtreeStorageDriver) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NASQtreeStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	// Generic user-facing message
	getError := fmt.Errorf("volume %s not found", name)

	exists, flexvol, err := d.API.QtreeExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).Errorf("Error checking for existing qtree. %s", err.Error())
		return getError
	}
	if !exists {
		Logc(ctx).WithField("qtree", name).Debug("Qtree not found.")
		return getError
	}

	Logc(ctx).WithFields(log.Fields{"qtree": name, "flexvol": flexvol}).Debug("Qtree found.")

	return nil
}

// ensureFlexvolForQtree accepts a set of Flexvol characteristics and either finds one to contain a new
// qtree or it creates a new Flexvol with the needed attributes.
func (d *NASQtreeStorageDriver) ensureFlexvolForQtree(
	ctx context.Context, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, enableSnapshotDir,
	enableEncryption bool, sizeBytes uint64, config drivers.OntapStorageDriverConfig, snapshotReserve,
	exportPolicy string,
) (string, error) {

	shouldLimitVolumeSize, flexvolQuotaSizeLimit, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, config.CommonStorageDriverConfig)
	if checkVolumeSizeLimitsError != nil {
		return "", checkVolumeSizeLimitsError
	}

	// Check if a suitable Flexvol already exists
	flexvol, err := d.getFlexvolForQtree(ctx, aggregate, spaceReserve, snapshotPolicy, tieringPolicy, snapshotReserve,
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
	ctx context.Context, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, enableSnapshotDir,
	enableEncryption bool, snapshotReserve, exportPolicy string,
) (string, error) {

	flexvol := d.FlexvolNamePrefix() + utils.RandomString(10)
	size := "1g"
	unixPermissions := "0711"
	if !d.Config.AutoExportPolicy {
		exportPolicy = d.flexvolExportPolicy
	}
	securityStyle := "unix"

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return "", fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
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
		"encryption":      enableEncryption,
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
		err := d.API.VolumeDisableSnapshotDirectoryAccess(ctx, flexvol)
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

// getFlexvolForQtree returns a Flexvol (from the set of existing Flexvols) that
// matches the specified Flexvol attributes and does not already contain more
// than the maximum configured number of qtrees.  No matching Flexvols is not
// considered an error.  If more than one matching Flexvol is found, one of those
// is returned at random.
func (d *NASQtreeStorageDriver) getFlexvolForQtree(
	ctx context.Context, aggregate, spaceReserve, snapshotPolicy, tieringPolicy, snapshotReserve string,
	enableSnapshotDir, enableEncryption, shouldLimitFlexvolQuotaSize bool, sizeBytes, flexvolQuotaSizeLimit uint64,
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
		return eligibleVolumeNames[rand.Intn(len(eligibleVolumeNames))], nil
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

	return calculateOptimalSizeForFlexvolAbstraction(ctx, flexvol, volAttrs, newQtreeSizeBytes,
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
				if utils.IsNotFoundError(err) {
					// Volume gone, so no need to try again
					Logc(ctx).WithField("flexvol", flexvol).Debug("Volume does not exist.")
					delete(d.quotaResizeMap, flexvol)
				}
				Logc(ctx).WithFields(log.Fields{"flexvol": flexvol, "error": err}).Debug("Error resizing quotas.")
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
			Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{"flexvol": flexvol, "qtrees": qtreeCount}).Debug("Flexvol has qtrees.")
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
				Logc(ctx).WithFields(log.Fields{"flexvol": flexvol, "error": err}).Error("Could not delete Flexvol.")
			} else {
				delete(d.emptyFlexvolMap, flexvol)
			}
		} else {
			Logc(ctx).WithFields(log.Fields{
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
			err := d.API.ExportRuleCreate(ctx, d.flexvolExportPolicy, rule)
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
) (map[string]string, error) {
	return getVolumeOptsCommon(ctx, volConfig, requests), nil
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

			Logc(ctx).WithFields(log.Fields{
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
	exists, flexvol, err := d.API.QtreeExists(ctx, volConfig.InternalName, d.FlexvolNamePrefix())
	if err != nil {
		return fmt.Errorf("could not determine if qtree %s exists: %v", volConfig.InternalName, err)
	}
	if !exists {
		return fmt.Errorf("could not find qtree %s", volConfig.InternalName)
	}

	// Set export path info on the volume config
	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.NfsPath = fmt.Sprintf("/%s/%s", flexvol, volConfig.InternalName)
	volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")

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

	if d.Config.DataLIF != dOrig.Config.DataLIF {
		bitmap.Add(storage.VolumeAccessInfoChange)
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
				Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
			"tick":   tick,
			"driver": t.Driver.Name(),
			"task":   t.Name,
		}).Debugf("Performing housekeeping task %d.", i)
		task(ctx)
	}
}

func NewPruneTask(ctx context.Context, d *NASQtreeStorageDriver, tasks []func(context.Context)) *HousekeepingTask {

	// Read background task timings from config file, use defaults if missing or invalid
	pruneFlexvolsPeriodSecs := defaultPruneFlexvolsPeriodSecs
	if d.Config.QtreePruneFlexvolsPeriod != "" {
		i, err := strconv.ParseUint(d.Config.QtreePruneFlexvolsPeriod, 10, 64)
		if err != nil {
			Logc(ctx).WithField("interval", d.Config.QtreePruneFlexvolsPeriod).Warnf(
				"Invalid Flexvol pruning interval. %v", err)
		} else {
			pruneFlexvolsPeriodSecs = i
		}
	}
	emptyFlexvolDeferredDeletePeriodSecs := defaultEmptyFlexvolDeferredDeletePeriodSecs
	if d.Config.EmptyFlexvolDeferredDeletePeriod != "" {
		i, err := strconv.ParseUint(d.Config.EmptyFlexvolDeferredDeletePeriod, 10, 64)
		if err != nil {
			Logc(ctx).WithField("interval", d.Config.EmptyFlexvolDeferredDeletePeriod).Warnf(
				"Invalid Flexvol deferred delete period. %v", err)
		} else {
			emptyFlexvolDeferredDeletePeriodSecs = i
		}
	}
	d.emptyFlexvolDeferredDeletePeriod = time.Duration(emptyFlexvolDeferredDeletePeriodSecs) * time.Second
	Logc(ctx).WithFields(log.Fields{
		"IntervalSeconds": pruneFlexvolsPeriodSecs,
		"EmptyFlexvolTTL": emptyFlexvolDeferredDeletePeriodSecs,
	}).Debug("Configured Flexvol pruning period.")

	task := &HousekeepingTask{
		Name:         pruneTask,
		Ticker:       time.NewTicker(time.Duration(pruneFlexvolsPeriodSecs) * time.Second),
		InitialDelay: HousekeepingStartupDelaySecs * time.Second,
		Done:         make(chan struct{}),
		Tasks:        tasks,
		Driver:       d,
	}

	return task
}

func NewResizeTask(ctx context.Context, d *NASQtreeStorageDriver, tasks []func(context.Context)) *HousekeepingTask {

	// Read background task timings from config file, use defaults if missing or invalid
	resizeQuotasPeriodSecs := defaultResizeQuotasPeriodSecs
	if d.Config.QtreeQuotaResizePeriod != "" {
		i, err := strconv.ParseUint(d.Config.QtreeQuotaResizePeriod, 10, 64)
		if err != nil {
			Logc(ctx).WithField("interval", d.Config.QtreeQuotaResizePeriod).Warnf(
				"Invalid quota resize interval. %v", err)
		} else {
			resizeQuotasPeriodSecs = i
		}
	}
	Logc(ctx).WithFields(log.Fields{
		"IntervalSeconds": resizeQuotasPeriodSecs,
	}).Debug("Configured quota resize period.")

	task := &HousekeepingTask{
		Name:         resizeTask,
		Ticker:       time.NewTicker(time.Duration(resizeQuotasPeriodSecs) * time.Second),
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
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "NASQtreeStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	// Ensure any Flexvol won't be pruned before resize is completed.
	utils.Lock(ctx, "resize", d.sharedLockID)
	defer utils.Unlock(ctx, "resize", d.sharedLockID)

	// Generic user-facing message
	resizeError := errors.New("storage driver failed to resize the volume")

	// Check that volume exists
	exists, flexvol, err := d.API.QtreeExists(ctx, name, d.FlexvolNamePrefix())
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Error checking for existing volume.")
		return resizeError
	}
	if !exists {
		Logc(ctx).WithFields(log.Fields{"qtree": name, "flexvol": flexvol}).Debug("Qtree does not exist.")
		return fmt.Errorf("volume %s does not exist", name)
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

	if aggrLimitsErr := checkAggregateLimitsForFlexvolAbstraction(
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
	ctx context.Context, nodes []*utils.Node, backendUUID string,
) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NASQtreeStorageDriver",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccessAbstraction(ctx, nodes, &d.Config, d.API, policyName)
}

// String makes NASQtreeStorageDriver satisfy the Stringer interface.
func (d NASQtreeStorageDriver) String() string {
	return drivers.ToString(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASQtreeStorageDriver satisfy the GoStringer interface.
func (d NASQtreeStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d NASQtreeStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}
