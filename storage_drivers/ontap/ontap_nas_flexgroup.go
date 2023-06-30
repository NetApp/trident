// Copyright 2022 NetApp, Inc. All Rights Reserved.
package ontap

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/cenkalti/backoff/v4"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

const flexgroupCreateTimeout = 60 * time.Second

// NASFlexGroupStorageDriver is for NFS and SMB FlexGroup storage provisioning
type NASFlexGroupStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         api.OntapAPI
	telemetry   *Telemetry

	physicalPool storage.Pool
	virtualPools map[string]storage.Pool
	timeout      time.Duration
}

func (d *NASFlexGroupStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASFlexGroupStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *NASFlexGroupStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d *NASFlexGroupStorageDriver) Name() string {
	return tridentconfig.OntapNASFlexGroupStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *NASFlexGroupStorageDriver) BackendName() string {
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
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "NASFlexGroupStorageDriver"}
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

	// Identify Virtual Pools
	if err := d.initializeStoragePools(ctx); err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = d.validate(ctx)
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	d.initialized = true
	return nil
}

func (d *NASFlexGroupStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASFlexGroupStorageDriver) Terminate(ctx context.Context, backendUUID string) {
	fields := LogFields{"Method": "Terminate", "Type": "NASFlexGroupStorageDriver"}
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

func (d *NASFlexGroupStorageDriver) initializeStoragePools(ctx context.Context) error {
	config := d.GetConfig()

	vserverAggrs, err := d.vserverAggregates(ctx, d.API.SVMName())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"svm":        d.API.SVMName(),
		"aggregates": vserverAggrs,
	}).Debug("Read aggregates assigned to SVM.")

	// Get list of media types supported by the Vserver aggregates
	mediaOffers, err := d.getVserverAggrMediaType(ctx, vserverAggrs)
	if len(mediaOffers) > 1 {
		Logc(ctx).Info(
			"All the aggregates do not have same media type, which is desirable for consistent FlexGroup performance.")
	}

	// Define physical pools
	// For a FlexGroup all aggregates that belong to the SVM represent the storage pool.
	pool := storage.NewStoragePool(nil, d.API.SVMName())

	// Update pool with attributes set by default for this backend
	// We do not set internal attributes with these values as this
	// merely means that pools supports these capabilities like
	// encryption, cloning, thick/thin provisioning
	for attrName, offer := range d.getStoragePoolAttributes() {
		pool.Attributes()[attrName] = offer
	}

	if config.Region != "" {
		pool.Attributes()[sa.Region] = sa.NewStringOffer(config.Region)
	}
	if config.Zone != "" {
		pool.Attributes()[sa.Zone] = sa.NewStringOffer(config.Zone)
	}

	pool.Attributes()[sa.Labels] = sa.NewLabelOffer(config.Labels)
	pool.Attributes()[sa.NASType] = sa.NewStringOffer(config.NASType)

	if len(mediaOffers) > 0 {
		pool.Attributes()[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
		pool.InternalAttributes()[Media] = pool.Attributes()[sa.Media].ToString()
	}

	pool.InternalAttributes()[Size] = config.Size
	pool.InternalAttributes()[Region] = config.Region
	pool.InternalAttributes()[Zone] = config.Zone
	pool.InternalAttributes()[SpaceReserve] = config.SpaceReserve
	pool.InternalAttributes()[SnapshotPolicy] = config.SnapshotPolicy
	pool.InternalAttributes()[SnapshotReserve] = config.SnapshotReserve
	pool.InternalAttributes()[Encryption] = config.Encryption
	pool.InternalAttributes()[UnixPermissions] = config.UnixPermissions
	pool.InternalAttributes()[SnapshotDir] = config.SnapshotDir
	pool.InternalAttributes()[SplitOnClone] = config.SplitOnClone
	pool.InternalAttributes()[ExportPolicy] = config.ExportPolicy
	pool.InternalAttributes()[SecurityStyle] = config.SecurityStyle
	pool.InternalAttributes()[TieringPolicy] = config.TieringPolicy
	pool.InternalAttributes()[QosPolicy] = config.QosPolicy
	pool.InternalAttributes()[AdaptiveQosPolicy] = config.AdaptiveQosPolicy

	d.physicalPool = pool

	d.virtualPools = make(map[string]storage.Pool)

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

			splitOnClone := config.SplitOnClone
			if vpool.SplitOnClone != "" {
				splitOnClone = vpool.SplitOnClone
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

			qosPolicy := config.QosPolicy
			if vpool.QosPolicy != "" {
				qosPolicy = vpool.QosPolicy
			}

			adaptiveQosPolicy := config.AdaptiveQosPolicy
			if vpool.AdaptiveQosPolicy != "" {
				adaptiveQosPolicy = vpool.AdaptiveQosPolicy
			}

			pool := storage.NewStoragePool(nil, poolName(fmt.Sprintf("pool_%d", index), d.BackendName()))

			// Update pool with attributes set by default for this backend
			// We do not set internal attributes with these values as this
			// merely means that pools supports these capabilities like
			// encryption, cloning, thick/thin provisioning
			for attrName, offer := range d.getStoragePoolAttributes() {
				pool.Attributes()[attrName] = offer
			}

			nasType := config.NASType
			if vpool.NASType != "" {
				nasType = vpool.NASType
			}

			pool.Attributes()[sa.Labels] = sa.NewLabelOffer(config.Labels, vpool.Labels)
			pool.Attributes()[sa.NASType] = sa.NewStringOffer(nasType)

			if region != "" {
				pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
			}
			if len(mediaOffers) > 0 {
				pool.Attributes()[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
				pool.InternalAttributes()[Media] = pool.Attributes()[sa.Media].ToString()
			}
			if encryption != "" {
				enableEncryption, err := strconv.ParseBool(encryption)
				if err != nil {
					return fmt.Errorf("invalid boolean value for encryption: %v in virtual pool: %s", err, pool.Name())
				}
				pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(enableEncryption)
				pool.InternalAttributes()[Encryption] = encryption
			}

			pool.InternalAttributes()[Size] = size
			pool.InternalAttributes()[Region] = region
			pool.InternalAttributes()[Zone] = zone
			pool.InternalAttributes()[SpaceReserve] = spaceReserve
			pool.InternalAttributes()[SnapshotPolicy] = snapshotPolicy
			pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes()[SplitOnClone] = splitOnClone
			pool.InternalAttributes()[UnixPermissions] = unixPermissions
			pool.InternalAttributes()[SnapshotDir] = snapshotDir
			pool.InternalAttributes()[ExportPolicy] = exportPolicy
			pool.InternalAttributes()[SecurityStyle] = securityStyle
			pool.InternalAttributes()[TieringPolicy] = tieringPolicy
			pool.InternalAttributes()[QosPolicy] = qosPolicy
			pool.InternalAttributes()[AdaptiveQosPolicy] = adaptiveQosPolicy

			d.virtualPools[pool.Name()] = pool
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

	aggrList, err := d.GetAPI().GetSVMAggregateAttributes(ctx)
	if err != nil {
		return nil, err
	}

	if aggrList != nil {
		for aggrName, aggrType := range aggrList {

			// Find matching aggregate.
			_, ok := aggrMap[aggrName]
			if !ok {
				continue
			}

			// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
			storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
			if !ok {
				Logc(ctx).WithFields(LogFields{
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
	fields := LogFields{"Method": "validate", "Type": "NASFlexGroupStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if !d.API.SupportsFeature(ctx, api.NetAppFlexGroups) {
		return fmt.Errorf("ONTAP version does not support FlexGroups")
	}

	if err := ValidateNASDriver(ctx, d.API, &d.Config); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Create a list `physicalPools` containing 1 entry
	physicalPools := map[string]storage.Pool{
		d.physicalPool.Name(): d.physicalPool,
	}
	if err := ValidateStoragePools(ctx, physicalPools, d.virtualPools, d,
		api.MaxNASLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.GetSVMAggregateNames(ctx)
	if err != nil {
		return err
	}

	if len(vserverAggrs) == 0 {
		return fmt.Errorf("no assigned aggregates found")
	}

	if len(d.Config.FlexGroupAggregateList) > 0 {
		containsAll, _ := utils.SliceContainsElements(vserverAggrs, d.Config.FlexGroupAggregateList)
		if !containsAll {
			return fmt.Errorf("not all aggregates specified in the flexgroupAggregateList are assigned to the SVM;  flexgroupAggregateList: %v assigned aggregates: %v",
				d.Config.FlexGroupAggregateList, vserverAggrs)
		}
	}

	return nil
}

// Create a volume with the specified options
func (d *NASFlexGroupStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Create",
		"Type":   "NASFlexGroupStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// If the volume already exists, bail out
	volExists, err := d.API.FlexgroupExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing FlexGroup: %v", err)
	}
	if volExists {
		return drivers.NewVolumeExistsError(name)
	}

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.GetSVMAggregateNames(ctx)
	if err != nil {
		return err
	}

	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("no assigned aggregates found")
		return err
	}

	var flexGroupAggregateList []string
	if len(d.Config.FlexGroupAggregateList) == 0 {
		// by default, the list of aggregates to use when creating the FlexGroup is derived from those assigned to the SVM
		vserverAggrNames := make([]azgo.AggrNameType, 0)
		vserverAggrNames = append(vserverAggrNames, vserverAggrs...)
		flexGroupAggregateList = vserverAggrNames
	} else {
		// allow the user to override the list of aggregates to use when creating the FlexGroup
		flexGroupAggregateList = d.Config.FlexGroupAggregateList
	}

	Logc(ctx).WithFields(LogFields{
		"aggregates": vserverAggrs,
	}).Debug("Read aggregates assigned to SVM.")

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
	// limits checks are not currently applicable to the Flexgroups driver, omitted here on purpose

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

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
	// get the flexgroup size based on the snapshot reserve
	flexgroupSize := calculateFlexvolSizeBytes(ctx, name, sizeBytes, snapshotReserveInt)
	sizeBytes, err = GetVolumeSize(flexgroupSize, storagePool.InternalAttributes()[Size])
	if err != nil {
		return err
	}

	if sizeBytes > math.MaxInt64 {
		return errors.UnsupportedCapacityRangeError(errors.New("invalid size requested"))
	}
	size := int(sizeBytes)

	if tieringPolicy == "" {
		tieringPolicy = "none"
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend().BackendUUID())
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

	Logc(ctx).WithFields(LogFields{
		"name":            name,
		"size":            size,
		"spaceReserve":    spaceReserve,
		"snapshotPolicy":  snapshotPolicy,
		"snapshotReserve": snapshotReserveInt,
		"unixPermissions": unixPermissions,
		"snapshotDir":     enableSnapshotDir,
		"exportPolicy":    exportPolicy,
		"aggregates":      flexGroupAggregateList,
		"securityStyle":   securityStyle,
		"encryption":      utils.GetPrintableBoolPtrValue(enableEncryption),
		"qosPolicy":       qosPolicy,
	}).Debug("Creating FlexGroup.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)
	physicalPoolNames = append(physicalPoolNames, d.physicalPool.Name())

	labels, err := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxNASLabelLength)
	if err != nil {
		return err
	}

	// Create the FlexGroup
	checkVolumeCreated := func() error {
		// Create the FlexGroup
		err = d.API.FlexgroupCreate(
			ctx, api.Volume{
				Aggregates:      flexGroupAggregateList,
				Comment:         labels,
				Encrypt:         enableEncryption,
				ExportPolicy:    exportPolicy,
				Name:            name,
				Qos:             qosPolicyGroup,
				Size:            strconv.FormatUint(sizeBytes, 10),
				SpaceReserve:    spaceReserve,
				SnapshotPolicy:  snapshotPolicy,
				SecurityStyle:   securityStyle,
				SnapshotReserve: snapshotReserveInt,
				TieringPolicy:   tieringPolicy,
				UnixPermissions: unixPermissions,
				DPVolume:        volConfig.IsMirrorDestination,
			})
		return err
	}

	volumeCreateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"increment": duration,
		}).Debug("FlexGroup not yet created, waiting.")
	}

	if d.timeout == 0 {
		d.timeout = flexgroupCreateTimeout
	}

	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 3 * time.Second
	volumeBackoff.Multiplier = 1.414
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = d.timeout
	volumeBackoff.MaxInterval = 10 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeCreated, volumeBackoff, volumeCreateNotify); err != nil {
		createErrors = append(createErrors, fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error creating FlexGroup %v: %v",
			storagePool.Name(), name, err))
		return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
	}

	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	if !enableSnapshotDir {
		if err := d.API.FlexgroupDisableSnapshotDirectoryAccess(ctx, name); err != nil {
			createErrors = append(createErrors,
				fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error disabling snapshot directory access for volume %v: %v",
					storagePool.Name(), name, err))
			return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
		}
	}

	// Mount the volume at the specified junction
	if err := d.API.FlexgroupMount(ctx, name, "/"+name); err != nil {
		createErrors = append(createErrors,
			fmt.Errorf("ONTAP-NAS-FLEXGROUP pool %s; error mounting volume %s to junction: %v; %v", storagePool.Name(),
				name, "/"+name, err))
		return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
	}

	// Create an SMB share for an SMB volume.
	if d.Config.NASType == sa.SMB {
		if err := d.EnsureSMBShare(ctx, volConfig.InternalName, "/"+volConfig.InternalName); err != nil {
			return err
		}
	}

	return nil
}

// cloneFlexgroup creates a flexgroup clone
func cloneFlexgroup(
	ctx context.Context, name, source, snapshot, labels string, split bool, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, useAsync bool, qosPolicyGroup api.QosPolicyGroup,
) error {
	fields := LogFields{
		"Method":   "cloneFlexgroup",
		"Type":     "NASFlexgroupStorageDriver",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
		"split":    split,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> cloneFlexgroup")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< cloneFlexgroup")

	// If the specified volume already exists, return an error
	volExists, err := client.FlexgroupExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return fmt.Errorf("volume %s already exists", name)
	}

	// If no specific snapshot was requested, create one
	if snapshot == "" {
		snapshot = time.Now().UTC().Format(storage.SnapshotNameFormat)
		if err = client.FlexgroupSnapshotCreate(ctx, snapshot, source); err != nil {
			return err
		}
	}

	// Create the clone based on a snapshot
	if err = client.VolumeCloneCreate(ctx, name, source, snapshot, useAsync); err != nil {
		return err
	}

	if err = waitForFlexgroup(ctx, client, name); err != nil {
		return err
	}

	if err = client.FlexgroupSetComment(ctx, name, name, labels); err != nil {
		return err
	}

	if config.StorageDriverName == tridentconfig.OntapNASFlexGroupStorageDriverName {
		// Mount the new volume
		if err = client.FlexgroupMount(ctx, name, "/"+name); err != nil {
			return err
		}
	}

	// Set the QoS Policy if necessary
	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		if err := client.FlexgroupSetQosPolicyGroupName(ctx, name, qosPolicyGroup); err != nil {
			return err
		}
	}

	// Split the clone if requested
	if split {
		if err := client.FlexgroupCloneSplitStart(ctx, name); err != nil {
			return fmt.Errorf("error splitting clone: %v", err)
		}
	}

	return nil
}

// waitForFlexgroup polls for the ONTAP flexgroup to exist, with backoff retry logic
func waitForFlexgroup(ctx context.Context, c api.OntapAPI, volumeName string) error {
	checkStatus := func() error {
		// this checks using ZAPI or REST via the supplied OntapAPI instance
		exists, err := c.FlexgroupExists(ctx, volumeName)
		if !exists {
			return fmt.Errorf("FlexGroup '%v' does not exit, will continue checking", volumeName)
		}
		return err
	}
	statusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("FlexGroup not found, waiting.")
	}
	statusBackoff := backoff.NewExponentialBackOff()
	statusBackoff.InitialInterval = 1 * time.Second
	statusBackoff.Multiplier = 2
	statusBackoff.RandomizationFactor = 0.1
	statusBackoff.MaxElapsedTime = 1 * time.Minute

	// Run the existence check using an exponential backoff
	if err := backoff.RetryNotify(checkStatus, statusBackoff, statusNotify); err != nil {
		Logc(ctx).WithField("name", volumeName).Warnf("FlexGroup not found after %3.2f seconds.",
			statusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	return nil
}

// CreateClone creates a flexgroup clone
func (d *NASFlexGroupStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	// Ensure the volume exists
	flexgroup, err := d.API.FlexgroupInfo(ctx, cloneVolConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	} else if flexgroup == nil {
		return fmt.Errorf("volume %s not found", cloneVolConfig.CloneSourceVolumeInternal)
	}

	// if cloning a FlexGroup, useAsync will be true
	if !d.GetAPI().SupportsFeature(ctx, api.NetAppFlexGroupsClone) {
		return errors.New("the ONTAPI version does not support FlexGroup cloning")
	}

	fields := LogFields{
		"Method":      "CreateClone",
		"Type":        "NASStorageDriver",
		"name":        cloneVolConfig.InternalName,
		"source":      cloneVolConfig.CloneSourceVolumeInternal,
		"snapshot":    cloneVolConfig.CloneSourceSnapshot,
		"storagePool": storagePool,
	}

	Logd(ctx, d.GetConfig().StorageDriverName, d.GetConfig().DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> CreateClone")
	defer Logd(ctx, d.GetConfig().StorageDriverName, d.GetConfig().DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< CreateClone")

	opts := d.GetVolumeOpts(context.Background(), cloneVolConfig, make(map[string]sa.Request))

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string

	labels := flexgroup.Comment

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.StoragePool{}
		storagePoolTemp.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
		})
		labels, err = storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxNASLabelLength)
		if err != nil {
			return err
		}
	} else {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]
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

	if err := cloneFlexgroup(ctx, cloneVolConfig.InternalName, cloneVolConfig.CloneSourceVolumeInternal,
		cloneVolConfig.CloneSourceSnapshotInternal, labels, split, d.GetConfig(), d.GetAPI(), true,
		qosPolicyGroup); err != nil {
		return err
	}

	// Create an SMB share for an SMB volume.
	if d.Config.NASType == sa.SMB {
		if err := d.EnsureSMBShare(ctx, cloneVolConfig.InternalName, "/"+cloneVolConfig.InternalName); err != nil {
			return err
		}
	}

	return nil
}

// Import brings an existing volume under trident's control
func (d *NASFlexGroupStorageDriver) Import(
	ctx context.Context, volConfig *storage.VolumeConfig, originalName string,
) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "NASFlexGroupStorageDriver",
		"originalName": originalName,
		"notManaged":   volConfig.ImportNotManaged,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	flexgroup, err := d.API.FlexgroupInfo(ctx, originalName)
	if err != nil {
		return fmt.Errorf("could not import volume; %v", err)
	} else if flexgroup == nil {
		return fmt.Errorf("could not import volume %s, volume not found", originalName)
	}

	// Validate the volume is what it should be
	if flexgroup.AccessType != "rw" {
		Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
		return fmt.Errorf("could not import volume %s, type is %s, not rw", originalName, flexgroup.AccessType)
	}

	// Get the volume size
	volConfig.Size = flexgroup.Size

	// We cannot rename flexgroups, so internal name should match the imported originalName
	volConfig.InternalName = originalName

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexgroup.Comment) {
			if err := d.API.FlexgroupSetComment(ctx, volConfig.InternalName, originalName, ""); err != nil {
				return err
			}
		}
	}

	// Modify unix-permissions of the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		// unixPermissions specified in PVC annotation takes precedence over backend's unixPermissions config
		unixPerms := volConfig.UnixPermissions

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

		if err := d.API.FlexgroupModifyUnixPermissions(ctx, volConfig.InternalName, originalName,
			unixPerms); err != nil {
			return err
		}
	}

	// Make sure we're not importing a volume without a junction path when not managed
	if volConfig.ImportNotManaged {
		if flexgroup.JunctionPath == "" {
			return fmt.Errorf("junction path is not set for volume %s", originalName)
		}
	}

	// Create an SMB share for an SMB volume
	if d.Config.NASType == sa.SMB {
		if flexgroup.JunctionPath != "" {
			if err := d.EnsureSMBShare(ctx, originalName, "/"+originalName); err != nil {
				return err
			}
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
func (d *NASFlexGroupStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "NASFlexGroupStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// This call is async, but we will receive an immediate error back for anything but very rare volume deletion
	// failures. Failures in this category are almost certainly likely to be beyond our capability to fix or even
	// diagnose, so we defer to the ONTAP cluster admin
	if err := d.API.FlexgroupDestroy(ctx, name, true); err != nil {
		return err
	}

	// Delete an SMB share for an SMB volume.
	if d.Config.NASType == sa.SMB {
		if err := d.DestroySMBShare(ctx, name); err != nil {
			return err
		}
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

	fields := LogFields{
		"Method": "Publish",
		"Type":   "NASFlexGroupStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

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
		publishInfo.NfsPath = fmt.Sprintf("/%s", name)
		publishInfo.NfsServerIP = d.Config.DataLIF
		publishInfo.FilesystemType = sa.NFS
		publishInfo.MountOptions = mountOptions
	}

	return publishShare(ctx, d.API, &d.Config, publishInfo, name, d.API.FlexgroupModifyExportPolicy)
}

// getFlexgroupSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func getFlexgroupSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "getFlexgroupSnapshot",
		"Type":         "NASFlexGroupStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getFlexgroupSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getFlexgroupSnapshot")

	size, err := client.FlexgroupUsedSize(ctx, snapConfig.VolumeInternalName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.FlexgroupSnapshotList(ctx, snapConfig.VolumeInternalName)
	if err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		Logc(ctx).WithFields(LogFields{
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
			"created":      snap.CreateTime,
		}).Debug("Found snapshot.")
		if snap.Name == snapConfig.InternalName {
			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.CreateTime,
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}

	return nil, nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASFlexGroupStorageDriver) CanSnapshot(
	_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NASFlexGroupStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "NASFlexGroupStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getFlexgroupSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// getFlexgroupSnapshotList returns the list of snapshots associated with the named volume.
func getFlexgroupSnapshotList(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":     "getFlexgroupSnapshotList",
		"Type":       "NASFlexGroupStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getFlexgroupSnapshotList")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getFlexgroupSnapshotList")

	size, err := client.FlexgroupUsedSize(ctx, volConfig.InternalName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.FlexgroupSnapshotList(ctx, volConfig.InternalName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
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
func (d *NASFlexGroupStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "NASFlexGroupStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getFlexgroupSnapshotList(ctx, volConfig, &d.Config, d.API)
}

// createFlexgroupSnapshot creates a snapshot for the given flexgroup.
func createFlexgroupSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "createFlexgroupSnapshot",
		"Type":         "NASFlexGroupStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createFlexgroupSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createFlexgroupSnapshot")

	// If the specified volume doesn't exist, return error
	volExists, err := client.FlexgroupExists(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	size, err := client.FlexgroupUsedSize(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	if err := client.FlexgroupSnapshotCreate(ctx, internalSnapName, internalVolName); err != nil {
		return nil, err
	}

	snapshots, err := client.FlexgroupSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		if snap.Name == internalSnapName {
			Logc(ctx).WithFields(LogFields{
				"snapshotName": snapConfig.InternalName,
				"volumeName":   snapConfig.VolumeInternalName,
			}).Info("Snapshot created.")

			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.CreateTime,
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find snapshot %s for source volume %s", internalSnapName, internalVolName)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASFlexGroupStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "NASFlexGroupStorageDriver",
		"snapshotName": internalSnapName,
		"sourceVolume": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	return createFlexgroupSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASFlexGroupStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "NASFlexGroupStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	if err := d.API.SnapshotRestoreFlexgroup(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}).Debug("Restored snapshot.")

	return nil
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASFlexGroupStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "NASFlexGroupStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	if err := d.API.FlexgroupSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName); err != nil {
		if api.IsSnapshotBusyError(err) {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			_ = SplitVolumeFromBusySnapshot(ctx, snapConfig, &d.Config, d.API,
				d.API.FlexgroupCloneSplitStart)
		}
		// we must return the err, even if we started a split, so the snapshot delete is retried
		return err
	}

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// Get tests the existence of a FlexGroup. Returns nil if the FlexGroup
// exists and an error otherwise.
func (d *NASFlexGroupStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "NASFlexGroupStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	volExists, err := d.API.FlexgroupExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("FlexGroup", name).Debug("FlexGroup not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// GetStorageBackendSpecs updates the specified Backend object with StoragePools.
func (d *NASFlexGroupStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	backend.SetName(d.BackendName())

	virtual := len(d.virtualPools) > 0

	if !virtual {
		d.physicalPool.SetBackend(backend)
		backend.AddStoragePool(d.physicalPool)
	}

	for _, pool := range d.virtualPools {
		pool.SetBackend(backend)
		if virtual {
			backend.AddStoragePool(pool)
		}
	}

	return nil
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NASFlexGroupStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	physicalPoolNames := make([]string, 0)
	physicalPoolNames = append(physicalPoolNames, d.physicalPool.Name())
	return physicalPoolNames
}

func (d *NASFlexGroupStorageDriver) vserverAggregates(ctx context.Context, svmName string) ([]string, error) {
	var err error
	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := d.API.GetSVMAggregateNames(ctx)
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
		sa.Replication:      sa.NewBoolOffer(false),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASFlexGroupStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *NASFlexGroupStorageDriver) GetInternalVolumeName(_ context.Context, name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASFlexGroupStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	createPrepareCommon(ctx, d, volConfig)
}

func (d *NASFlexGroupStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	if d.Config.NASType == sa.SMB {
		volConfig.AccessInfo.SMBServer = d.Config.DataLIF
		volConfig.FileSystem = sa.SMB
	} else {
		volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
		volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
		volConfig.FileSystem = sa.NFS
	}

	// Set correct junction path
	flexgroup, err := d.API.FlexgroupInfo(ctx, volConfig.InternalName)
	if err != nil {
		return fmt.Errorf("could not create volume; %v", err)
	} else if flexgroup == nil {
		return fmt.Errorf("could not create volume %s, volume not found", volConfig.InternalName)
	}

	if flexgroup.JunctionPath == "" {
		// Flexvol will not be mounted in the following scenarios, we need to mount it,
		// 1. During Import of volume without Junction path.
		// 2. During Create/CreateClone there is a failure and mount is not performed.

		if d.Config.NASType == sa.SMB {
			volConfig.AccessInfo.SMBPath = ConstructOntapNASFlexGroupSMBVolumePath(ctx, d.Config.SMBShare,
				volConfig.InternalName)
			// Overwriting mount path, mounting at root instead of admin share
			volConfig.AccessInfo.SMBPath = "/" + volConfig.InternalName
			err = d.MountFlexgroup(ctx, volConfig.InternalName, volConfig.AccessInfo.SMBPath)
			if err != nil {
				return err
			}

			// If smbShare is omitted in the backend configuration then,
			// Trident tries to create sbmShare with the same name as volume InternalName.
			// This check ensures that volume is mounted before the share is created.
			if err := d.EnsureSMBShare(ctx, volConfig.InternalName, "/"+volConfig.InternalName); err != nil {
				return err
			}
		} else {
			volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
			err = d.MountFlexgroup(ctx, volConfig.InternalName, volConfig.AccessInfo.NfsPath)
			if err != nil {
				return err
			}
		}
	} else {
		if d.Config.NASType == sa.SMB {
			volConfig.AccessInfo.SMBPath = ConstructOntapNASFlexGroupSMBVolumePath(ctx, d.Config.SMBShare,
				flexgroup.JunctionPath)
		} else {
			volConfig.AccessInfo.NfsPath = flexgroup.JunctionPath
		}
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
func (d *NASFlexGroupStorageDriver) GetVolumeExternal(ctx context.Context, name string) (
	*storage.VolumeExternal, error,
) {
	flexgroup, err := d.API.FlexgroupInfo(ctx, name)
	if err != nil {
		return nil, err
	}

	return getVolumeExternalCommon(*flexgroup, *d.Config.StoragePrefix, d.API.SVMName()), nil
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
	volumes, err := d.API.FlexgroupListByPrefix(ctx, *d.Config.StoragePrefix)
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

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASFlexGroupStorageDriver) getVolumeExternal(volumeAttrs *azgo.VolumeAttributesType) *storage.VolumeExternal {
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

// Resize expands the FlexGroup size.
func (d *NASFlexGroupStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":             "Resize",
		"Type":               "NASFlexGroupStorageDriver",
		"name":               name,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	flexgroupSize, err := resizeValidation(ctx, name, requestedSizeBytes, d.API.FlexgroupExists,
		d.API.FlexgroupSize)
	if err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(flexgroupSize, 10)
	if flexgroupSize == requestedSizeBytes {
		return nil
	}

	snapshotReserveInt, err := getSnapshotReserveFromOntap(ctx, name, d.API.FlexgroupInfo)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume")
	}

	newFlexgroupSize := calculateFlexvolSizeBytes(ctx, name, requestedSizeBytes, snapshotReserveInt)

	if err := d.API.FlexgroupSetSize(ctx, name, strconv.FormatUint(newFlexgroupSize, 10)); err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(requestedSizeBytes, 10)
	return nil
}

func (d *NASFlexGroupStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*utils.Node, backendUUID, _ string,
) error {
	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "NASFlexGroupStorageDriver",
		"Nodes":  nodeNames,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccess(ctx, nodes, &d.Config, d.API, policyName)
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *NASFlexGroupStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debugf("<<<< GetBackendState")

	return getSVMState(ctx, d.API, "nfs", d.GetStorageBackendPhysicalPoolNames(ctx))
}

// String makes NASFlexGroupStorageDriver satisfy the Stringer interface.
func (d NASFlexGroupStorageDriver) String() string {
	return utils.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASFlexGroupStorageDriver satisfy the GoStringer interface.
func (d NASFlexGroupStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d NASFlexGroupStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// MountFlexgroup returns the flexgroup volume mount error(if any)
func (d NASFlexGroupStorageDriver) MountFlexgroup(ctx context.Context, name, junctionPath string) error {
	if err := d.API.FlexgroupMount(ctx, name, junctionPath); err != nil {
		return fmt.Errorf("error mounting volume to junction %s; %v", junctionPath, err)
	}
	return nil
}

// EnsureSMBShare ensures that required SMB share is made available.
func (d *NASFlexGroupStorageDriver) EnsureSMBShare(
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
			if err := d.API.SMBShareCreate(ctx, name, path); err != nil {
				return err
			}
		}
	}

	return nil
}

// DestroySMBShare destroys an SMB share
func (d *NASFlexGroupStorageDriver) DestroySMBShare(
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
