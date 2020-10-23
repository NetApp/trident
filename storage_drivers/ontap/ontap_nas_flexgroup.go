// Copyright 2020 NetApp, Inc. All Rights Reserved.
package ontap

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

// NASFlexGroupStorageDriver is for NFS FlexGroup storage provisioning
type NASFlexGroupStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         *api.Client
	Telemetry   *Telemetry

	physicalPool *storage.Pool
	virtualPools map[string]*storage.Pool
}

func (d *NASFlexGroupStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASFlexGroupStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *NASFlexGroupStorageDriver) GetTelemetry() *Telemetry {
	d.Telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *NASFlexGroupStorageDriver) Name() string {
	return drivers.OntapNASFlexGroupStorageDriverName
}

// backendName returns the name of the backend managed by this driver instance
func (d *NASFlexGroupStorageDriver) backendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapnasfg_" + d.Config.DataLIF)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *NASFlexGroupStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NASFlexGroupStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.API, err = InitializeOntapDriver(ctx, config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	// Identify Virtual Pools
	if err := d.initializeStoragePools(ctx); err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = d.validate(ctx)
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.Telemetry = NewOntapTelemetry(ctx, d)
	d.Telemetry.Start(ctx)

	d.initialized = true
	return nil
}

func (d *NASFlexGroupStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASFlexGroupStorageDriver) Terminate(ctx context.Context, backendUUID string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASFlexGroupStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}
	if d.Config.AutoExportPolicy {
		policyName := getExportPolicyName(backendUUID)
		if err := deleteExportPolicy(ctx, policyName, d.API); err != nil {
			Logc(ctx).Warn(err)
		}
	}
	if d.Telemetry != nil {
		d.Telemetry.Stop()
	}
	d.initialized = false
}

func (d *NASFlexGroupStorageDriver) initializeStoragePools(ctx context.Context) error {

	config := d.GetConfig()

	vserverAggrs, err := d.vserverAggregates(config.SVM)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"svm":        config.SVM,
		"aggregates": vserverAggrs,
	}).Debug("Read aggregates assigned to SVM.")

	// Get list of media types supported by the Vserver aggregates
	mediaOffers, err := d.getVserverAggrMediaType(ctx, vserverAggrs)
	if len(mediaOffers) > 1 {
		Logc(ctx).Info("All the aggregates do not have same media type, " +
			"which is desirable for consistent FlexGroup performance.")
	}

	// Define physical pools
	// For a FlexGroup all aggregates that belong to the SVM represent the storage pool.
	pool := storage.NewStoragePool(nil, config.SVM)

	// Update pool with attributes set by default for this backend
	// We do not set internal attributes with these values as this
	// merely means that pools supports these capabilities like
	// encryption, cloning, thick/thin provisioning
	for attrName, offer := range d.getStoragePoolAttributes() {
		pool.Attributes[attrName] = offer
	}

	if config.Region != "" {
		pool.Attributes[sa.Region] = sa.NewStringOffer(config.Region)
	}
	if config.Zone != "" {
		pool.Attributes[sa.Zone] = sa.NewStringOffer(config.Zone)
	}
	if len(mediaOffers) > 0 {
		pool.Attributes[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
		pool.InternalAttributes[Media] = pool.Attributes[sa.Media].ToString()
	}

	pool.InternalAttributes[Size] = config.Size
	pool.InternalAttributes[Region] = config.Region
	pool.InternalAttributes[Zone] = config.Zone
	pool.InternalAttributes[SpaceReserve] = config.SpaceReserve
	pool.InternalAttributes[SnapshotPolicy] = config.SnapshotPolicy
	pool.InternalAttributes[SnapshotReserve] = config.SnapshotReserve
	pool.InternalAttributes[Encryption] = config.Encryption
	pool.InternalAttributes[UnixPermissions] = config.UnixPermissions
	pool.InternalAttributes[SnapshotDir] = config.SnapshotDir
	pool.InternalAttributes[ExportPolicy] = config.ExportPolicy
	pool.InternalAttributes[SecurityStyle] = config.SecurityStyle
	pool.InternalAttributes[TieringPolicy] = config.TieringPolicy

	d.physicalPool = pool

	d.virtualPools = make(map[string]*storage.Pool)

	if len(d.Config.Storage) != 0 {
		Logc(ctx).Debug("Defining Virtual Pools based on Virtual Pools definition in the backend file.")

		for index, vpool := range d.Config.Storage {
			region := config.Region
			if vpool.Region != "" {
				region = vpool.Region
			}

			zone := config.Zone
			if vpool.Zone != "" {
				zone = vpool.Zone
			}

			size := config.Size
			if vpool.Size != "" {
				size = vpool.Size
			}

			spaceReserve := config.SpaceReserve
			if vpool.SpaceReserve != "" {
				spaceReserve = vpool.SpaceReserve
			}

			snapshotPolicy := config.SnapshotPolicy
			if vpool.SnapshotPolicy != "" {
				snapshotPolicy = vpool.SnapshotPolicy
			}

			snapshotReserve := config.SnapshotReserve
			if vpool.SnapshotReserve != "" {
				snapshotReserve = vpool.SnapshotReserve
			}

			unixPermissions := config.UnixPermissions
			if vpool.UnixPermissions != "" {
				unixPermissions = vpool.UnixPermissions
			}

			snapshotDir := config.SnapshotDir
			if vpool.SnapshotDir != "" {
				snapshotDir = vpool.SnapshotDir
			}

			exportPolicy := config.ExportPolicy
			if vpool.ExportPolicy != "" {
				exportPolicy = vpool.ExportPolicy
			}

			securityStyle := config.SecurityStyle
			if vpool.SecurityStyle != "" {
				securityStyle = vpool.SecurityStyle
			}

			encryption := config.Encryption
			if vpool.Encryption != "" {
				encryption = vpool.Encryption
			}

			tieringPolicy := config.TieringPolicy
			if vpool.TieringPolicy != "" {
				tieringPolicy = vpool.TieringPolicy
			}

			pool := storage.NewStoragePool(nil, poolName(fmt.Sprintf("pool_%d", index), d.backendName()))

			// Update pool with attributes set by default for this backend
			// We do not set internal attributes with these values as this
			// merely means that pools supports these capabilities like
			// encryption, cloning, thick/thin provisioning
			for attrName, offer := range d.getStoragePoolAttributes() {
				pool.Attributes[attrName] = offer
			}

			pool.Attributes[sa.Labels] = sa.NewLabelOffer(config.Labels, vpool.Labels)

			if region != "" {
				pool.Attributes[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes[sa.Zone] = sa.NewStringOffer(zone)
			}
			if len(mediaOffers) > 0 {
				pool.Attributes[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
				pool.InternalAttributes[Media] = pool.Attributes[sa.Media].ToString()
			}
			if encryption != "" {
				enableEncryption, err := strconv.ParseBool(encryption)
				if err != nil {
					return fmt.Errorf("invalid boolean value for encryption: %v in virtual pool: %s", err,
						pool.Name)
				}
				pool.Attributes[sa.Encryption] = sa.NewBoolOffer(enableEncryption)
				pool.InternalAttributes[Encryption] = encryption
			}

			pool.InternalAttributes[Size] = size
			pool.InternalAttributes[Region] = region
			pool.InternalAttributes[Zone] = zone
			pool.InternalAttributes[SpaceReserve] = spaceReserve
			pool.InternalAttributes[SnapshotPolicy] = snapshotPolicy
			pool.InternalAttributes[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes[UnixPermissions] = unixPermissions
			pool.InternalAttributes[SnapshotDir] = snapshotDir
			pool.InternalAttributes[ExportPolicy] = exportPolicy
			pool.InternalAttributes[SecurityStyle] = securityStyle
			pool.InternalAttributes[TieringPolicy] = tieringPolicy

			d.virtualPools[pool.Name] = pool
		}
	}

	return nil
}

// getVserverAggrMediaType gets vservers' media type attribute using vserver-show-aggr-get-iter,
// which will only succeed on Data ONTAP 9 and later.
func (d *NASFlexGroupStorageDriver) getVserverAggrMediaType(
	ctx context.Context, aggrNames []string,
) (mediaOffers []sa.Offer, err error) {

	aggrMediaTypes := make(map[sa.Offer]struct{})

	aggrMap := make(map[string]struct{})
	for _, s := range aggrNames {
		aggrMap[s] = struct{}{}
	}

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	result, err := d.GetAPI().VserverShowAggrGetIterRequest()
	if err != nil {
		return
	}

	if zerr := api.NewZapiError(result.Result); !zerr.IsPassed() {
		err = zerr
		return
	}

	if result.Result.AttributesListPtr != nil {
		for _, aggr := range result.Result.AttributesListPtr.ShowAggregatesPtr {
			aggrName := aggr.AggregateName()
			aggrType := aggr.AggregateType()

			// Find matching aggregate.
			_, ok := aggrMap[aggrName]
			if !ok {
				continue
			}

			// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
			storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
			if !ok {
				Logc(ctx).WithFields(log.Fields{
					"aggregate": aggrName,
					"mediaType": aggrType,
				}).Debug("Aggregate has unknown performance characteristics.")

				continue
			}

			if storageAttrs != nil {
				aggrMediaTypes[storageAttrs[sa.Media]] = struct{}{}
			}
		}
	}

	for key := range aggrMediaTypes {
		mediaOffers = append(mediaOffers, key)
	}

	return
}

// Validate the driver configuration and execution environment
func (d *NASFlexGroupStorageDriver) validate(ctx context.Context) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NASFlexGroupStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	if !d.API.SupportsFeature(ctx, api.NetAppFlexGroups) {
		return fmt.Errorf("ONTAP version does not support FlexGroups")
	}

	err := ValidateNASDriver(ctx, d.API, &d.Config)
	if err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	// Create a list `physicalPools` containing 1 entry
	var physicalPools = map[string]*storage.Pool{
		d.physicalPool.Name: d.physicalPool,
	}
	if err := ValidateStoragePools(ctx, physicalPools, d.virtualPools, d.Name()); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume with the specified options
func (d *NASFlexGroupStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NASFlexGroupStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Create")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Create")
	}

	// If the volume already exists, bail out
	volExists, err := d.API.FlexGroupExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing FlexGroup: %v", err)
	}
	if volExists {
		return drivers.NewVolumeExistsError(name)
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
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes[Size])
	if err != nil {
		return err
	}
	if sizeBytes > math.MaxInt64 {
		return errors.New("invalid size requested")
	}
	size := int(sizeBytes)

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.VserverGetAggregateNames()
	if err != nil {
		return err
	}

	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("no assigned aggregates found")
		return err
	}

	vserverAggrNames := make([]azgo.AggrNameType, 0)
	for _, aggrName := range vserverAggrs {
		vserverAggrNames = append(vserverAggrNames, aggrName)
	}

	Logc(ctx).WithFields(log.Fields{
		"aggregates": vserverAggrs,
	}).Debug("Read aggregates assigned to SVM.")

	// Get options
	opts, err := d.GetVolumeOpts(ctx, volConfig, volAttributes)
	if err != nil {
		return err
	}

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	spaceReserve := utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes[SpaceReserve])
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes[SnapshotPolicy])
	snapshotReserve := utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes[SnapshotReserve])
	unixPermissions := utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes[UnixPermissions])
	snapshotDir := utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes[SnapshotDir])
	exportPolicy := utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes[ExportPolicy])
	securityStyle := utils.GetV(opts, "securityStyle", storagePool.InternalAttributes[SecurityStyle])
	encryption := utils.GetV(opts, "encryption", storagePool.InternalAttributes[Encryption])
	tieringPolicy := utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes[TieringPolicy])

	// limits checks are not currently applicable to the Flexgroups driver, omitted here on purpose

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	if tieringPolicy == "" {
		tieringPolicy = "none"
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend.BackendUUID)
	}

	Logc(ctx).WithFields(log.Fields{
		"name":            name,
		"size":            size,
		"spaceReserve":    spaceReserve,
		"snapshotPolicy":  snapshotPolicy,
		"snapshotReserve": snapshotReserveInt,
		"unixPermissions": unixPermissions,
		"snapshotDir":     enableSnapshotDir,
		"exportPolicy":    exportPolicy,
		"aggregates":      vserverAggrNames,
		"securityStyle":   securityStyle,
		"encryption":      enableEncryption,
	}).Debug("Creating FlexGroup.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)
	physicalPoolNames = append(physicalPoolNames, d.physicalPool.Name)

	// Create the FlexGroup
	checkVolumeCreated := func() error {
		_, err = d.API.FlexGroupCreate(
			ctx, name, size, vserverAggrNames, spaceReserve, snapshotPolicy, unixPermissions,
			exportPolicy, securityStyle, tieringPolicy, enableEncryption, snapshotReserveInt)

		return err
	}

	volumeCreateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
			"name":      name,
			"increment": duration}).Debug("FlexGroup not yet created, waiting.")
	}

	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 3 * time.Second
	volumeBackoff.Multiplier = 1.414
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = 60 * time.Second
	volumeBackoff.MaxInterval = 10 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeCreated, volumeBackoff, volumeCreateNotify); err != nil {
		createErrors = append(createErrors, fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error creating FlexGroup %v: %v", storagePool.Name, name, err))
		return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
	}

	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	if !enableSnapshotDir {
		_, err := d.API.FlexGroupVolumeDisableSnapshotDirectoryAccess(ctx, name)
		if err != nil {
			createErrors = append(createErrors, fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error disabling snapshot directory access for volume %v: %v", storagePool.Name, name, err))
			return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
		}
	}

	// Mount the volume at the specified junction
	mountResponse, err := d.API.VolumeMount(name, "/"+name)
	if err = api.GetError(ctx, mountResponse, err); err != nil {
		createErrors = append(createErrors, fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error mounting volume %s to junction: %v", storagePool.Name, name, err))
		return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
	}

	return nil
}

// CreateClone creates a flexgroup clone
func (d *NASFlexGroupStorageDriver) CreateClone(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool,
) error {
	return CreateCloneNAS(ctx, d, volConfig, storagePool, true)
}

// Import brings an existing volume under trident's control
func (d *NASFlexGroupStorageDriver) Import(
	ctx context.Context, volConfig *storage.VolumeConfig, originalName string,
) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "NASFlexGroupStorageDriver",
			"originalName": originalName,
			"notManaged":   volConfig.ImportNotManaged,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Import")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Import")
	}

	// Ensure the volume exists
	flexgroup, err := d.API.FlexGroupGet(originalName)
	if err != nil {
		return err
	} else if flexgroup == nil {
		return fmt.Errorf("could not import volume %s, volume not found", originalName)
	}

	// Validate the volume is what it should be
	if flexgroup.VolumeIdAttributesPtr != nil {
		volumeIdAttrs := flexgroup.VolumeIdAttributes()
		if volumeIdAttrs.TypePtr != nil && volumeIdAttrs.Type() != "rw" {
			Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("could not import volume %s, type is %s, not rw", originalName, volumeIdAttrs.Type())
		}
	}

	// Get the volume size
	if flexgroup.VolumeSpaceAttributesPtr == nil || flexgroup.VolumeSpaceAttributesPtr.SizePtr == nil {
		Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, size not available")
		return fmt.Errorf("could not import volume %s, size not available", originalName)
	}
	volConfig.Size = strconv.FormatInt(int64(flexgroup.VolumeSpaceAttributesPtr.Size()), 10)

	// We cannot rename flexgroups, so internal name should match the imported originalName
	volConfig.InternalName = originalName

	// Modify unix-permissions of the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		// unixPermissions specified in PVC annotation takes precedence over backend's unixPermissions config
		unixPerms := volConfig.UnixPermissions
		if unixPerms == "" {
			unixPerms = d.Config.UnixPermissions
		}
		modifyUnixPermResponse, err := d.API.FlexGroupModifyUnixPermissions(ctx, volConfig.InternalName, unixPerms)
		if err = api.GetError(ctx, modifyUnixPermResponse, err); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, modifying unix permissions failed: %v", err)
			return fmt.Errorf("volume %s modify failed: %v", originalName, err)
		}
	}

	// Make sure we're not importing a volume without a junction path when not managed
	if volConfig.ImportNotManaged {
		if flexgroup.VolumeIdAttributesPtr == nil {
			return fmt.Errorf("unable to read volume id attributes of volume %s", originalName)
		} else if flexgroup.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexgroup.VolumeIdAttributesPtr.
			JunctionPath() == "" {
			return fmt.Errorf("junction path is not set for volume %s", originalName)
		}
	}

	return nil
}

// Rename changes the name of a volume
func (d *NASFlexGroupStorageDriver) Rename(context.Context, string, string) error {
	// Flexgroups cannot be renamed
	return nil
}

// Destroy the volume
func (d *NASFlexGroupStorageDriver) Destroy(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NASFlexGroupStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	// Needed once FlexGroups support clones
	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.

	if volExists, err := UnmountAndOfflineVolume(ctx, d.GetAPI(), name); err != nil {
		return err
	} else if !volExists {
		return nil
	}

	// This call is async, but we will receive an immediate error back for anything but very rare volume deletion
	// failures. Failures in this category are almost certainly likely to be beyond our capability to fix or even
	// diagnose, so we defer to the ONTAP cluster admin
	if _, err := d.API.FlexGroupDestroy(ctx, name, true); err != nil {
		return fmt.Errorf("error destroying FlexGroup %v: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASFlexGroupStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "NASFlexGroupStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Publish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Publish")
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = fmt.Sprintf("/%s", name)
	publishInfo.NfsServerIP = d.Config.DataLIF
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return publishFlexVolShare(ctx, d.API, &d.Config, publishInfo, name)
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NASFlexGroupStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NASFlexGroupStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return GetSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.FlexGroupSize)
}

// Return the list of snapshots associated with the specified volume
func (d *NASFlexGroupStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NASFlexGroupStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return GetSnapshots(ctx, volConfig, &d.Config, d.API, d.API.FlexGroupSize)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASFlexGroupStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NASFlexGroupStorageDriver",
			"snapshotName": internalSnapName,
			"sourceVolume": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return CreateSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.FlexGroupSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASFlexGroupStorageDriver) RestoreSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "NASFlexGroupStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASFlexGroupStorageDriver) DeleteSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NASFlexGroupStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return DeleteSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// Tests the existence of a FlexGroup. Returns nil if the FlexGroup
// exists and an error otherwise.
func (d *NASFlexGroupStorageDriver) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NASFlexGroupStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	volExists, err := d.API.FlexGroupExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("FlexGroup", name).Debug("FlexGroup not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// getStorageBackendSpecsCommon updates the specified Backend object with StoragePools.
func (d *NASFlexGroupStorageDriver) GetStorageBackendSpecs(_ context.Context, backend *storage.Backend) error {
	backend.Name = d.backendName()

	virtual := len(d.virtualPools) > 0

	if !virtual {
		d.physicalPool.Backend = backend
		backend.AddStoragePool(d.physicalPool)
	}

	for _, pool := range d.virtualPools {
		pool.Backend = backend
		if virtual {
			backend.AddStoragePool(pool)
		}
	}

	return nil
}

// Retrieve storage backend physical pools
func (d *NASFlexGroupStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	physicalPoolNames := make([]string, 0)
	physicalPoolNames = append(physicalPoolNames, d.physicalPool.Name)
	return physicalPoolNames
}

func (d *NASFlexGroupStorageDriver) vserverAggregates(svmName string) ([]string, error) {
	var err error
	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.VserverGetAggregateNames()
	if err != nil {
		return nil, err
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates", svmName)
		return nil, err
	}

	return vserverAggrs, nil
}

func (d *NASFlexGroupStorageDriver) getStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASFlexGroupStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(ctx, volConfig, requests), nil
}

func (d *NASFlexGroupStorageDriver) GetInternalVolumeName(_ context.Context, name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASFlexGroupStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	createPrepareCommon(ctx, d, volConfig)
}

func (d *NASFlexGroupStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {

	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
	volConfig.FileSystem = ""

	// Set correct junction path
	flexgroup, err := d.API.FlexGroupGet(volConfig.InternalName)
	if err != nil {
		return err
	} else if flexgroup == nil {
		return fmt.Errorf("could not create volume %s, volume not found", volConfig.InternalName)
	}

	if flexgroup.VolumeIdAttributesPtr == nil {
		return errors.New("error reading volume id attributes")
	}
	if flexgroup.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexgroup.VolumeIdAttributesPtr.JunctionPath() == "" {
		// Flexgroup is not mounted, we need to mount it
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
		mountResponse, err := d.API.VolumeMount(volConfig.InternalName, volConfig.AccessInfo.NfsPath)
		if err = api.GetError(ctx, mountResponse, err); err != nil {
			return fmt.Errorf("error mounting volume to junction %s; %v", volConfig.AccessInfo.NfsPath, err)
		}
	} else {
		volConfig.AccessInfo.NfsPath = flexgroup.VolumeIdAttributesPtr.JunctionPath()
	}

	return nil
}

func (d *NASFlexGroupStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NASFlexGroupStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASFlexGroupStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NASFlexGroupStorageDriver) GetVolumeExternal(_ context.Context, name string) (
	*storage.VolumeExternal, error,
) {

	volumeAttributes, err := d.API.FlexGroupGet(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volumeAttributes), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASFlexGroupStorageDriver) GetVolumeExternalWrappers(
	ctx context.Context, channel chan *storage.VolumeExternalWrapper,
) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.FlexGroupGetAll(*d.Config.StoragePrefix)
	if err = api.GetError(ctx, volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volume := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASFlexGroupStorageDriver) getVolumeExternal(
	volumeAttrs *azgo.VolumeAttributesType) *storage.VolumeExternal {

	volumeExportAttrs := volumeAttrs.VolumeExportAttributesPtr
	volumeIDAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSecurityAttrs := volumeAttrs.VolumeSecurityAttributesPtr
	volumeSecurityUnixAttrs := volumeSecurityAttrs.VolumeSecurityUnixAttributesPtr
	volumeSpaceAttrs := volumeAttrs.VolumeSpaceAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := volumeIDAttrs.Name()
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    volumeExportAttrs.Policy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: volumeSecurityUnixAttrs.Permissions(),
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volumeIDAttrs.OwningVserverName(),
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASFlexGroupStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASFlexGroupStorageDriver)
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

	if !reflect.DeepEqual(d.Config.StoragePrefix, dOrig.Config.StoragePrefix) {
		bitmap.Add(storage.PrefixChange)
	}

	return bitmap
}

// Resize expands the FlexGroup size.
func (d *NASFlexGroupStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64,
) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "NASFlexGroupStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	flexvolSize, err := resizeValidation(ctx, name, sizeBytes, d.API.FlexGroupExists, d.API.FlexGroupSize)
	if err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(flexvolSize, 10)
	if flexvolSize == sizeBytes {
		return nil
	}

	_, err = d.API.FlexGroupSetSize(ctx, name, strconv.FormatUint(sizeBytes, 10))
	if err != nil {
		Logc(ctx).WithField("error", err).Error("FlexGroup resize failed.")
		return fmt.Errorf("flexgroup resize failed")
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

func (d *NASFlexGroupStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*utils.Node, backendUUID string,
) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NASFlexGroupStorageDriver",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccess(ctx, nodes, &d.Config, d.API, policyName)
}

// String makes NASFlexGroupStorageDriver satisfy the Stringer interface.
func (d NASFlexGroupStorageDriver) String() string {
	sensitive := d.Config.DebugTraceFlags["sensitive"]
	return drivers.ToString(sensitive, &d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASFlexGroupStorageDriver satisfy the GoStringer interface.
func (d NASFlexGroupStorageDriver) GoString() string {
	return d.String()
}
