// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

// NASStorageDriverRest is for NFS storage provisioning
type NASStorageDriverRest struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	//API         *api.Client
	API       *api.RestClient
	Telemetry *Telemetry

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func (d *NASStorageDriverRest) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASStorageDriverRest) GetAPI() *api.RestClient {
	return d.API
}

func (d *NASStorageDriverRest) GetTelemetry() *Telemetry {
	d.Telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *NASStorageDriverRest) Name() string {
	return drivers.OntapNASStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *NASStorageDriverRest) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapnas_" + d.Config.DataLIF)
	} else {
		return d.Config.BackendName
	}
}

// InitializeOntapDriverRest sets up the API client and performs all other initialization tasks
// that are common to all the ONTAP drivers.
func InitializeOntapDriverRest(ctx context.Context, config *drivers.OntapStorageDriverConfig) (*api.RestClient, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapDriverRest", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> InitializeOntapDriverRest")
		defer Logc(ctx).WithFields(fields).Debug("<<<< InitializeOntapDriverRest")
	}

	// Splitting config.ManagementLIF with colon allows to provide managementLIF value as address:port format
	mgmtLIF := ""
	if utils.IPv6Check(config.ManagementLIF) {
		// This is an IPv6 address

		mgmtLIF = strings.Split(config.ManagementLIF, "[")[1]
		mgmtLIF = strings.Split(mgmtLIF, "]")[0]
	} else {
		mgmtLIF = strings.Split(config.ManagementLIF, ":")[0]
	}

	addressesFromHostname, err := net.LookupHost(mgmtLIF)
	if err != nil {
		Logc(ctx).WithField("ManagementLIF", mgmtLIF).Error("Host lookup failed for ManagementLIF. ", err)
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"hostname":  mgmtLIF,
		"addresses": addressesFromHostname,
	}).Debug("Addresses found from ManagementLIF lookup.")

	// Get the API client
	client, err := api.NewRestClientFromOntapConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not create Data ONTAP REST API client: %v", err)
	}

	// TODO IMPLEMENT THIS
	// Make sure we're using a valid ONTAP version
	ontapVersion, err := client.SystemGetOntapVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not determine Data ONTAP version: %v", err)
	}
	Logc(ctx).WithField("ontapVersion", ontapVersion).Debug("ONTAP version.")

	ontapSemVer, err := utils.ParseSemantic(ontapVersion)
	if err != nil {
		return nil, err
	}
	if !ontapSemVer.AtLeast(utils.MustParseSemantic("9.8.0")) { // TODO make a constant
		return nil, fmt.Errorf("ONTAP 9.8 or later is required, found %v", ontapVersion)
	}

	// Log cluster node serial numbers if we can get them
	config.SerialNumbers, err = client.NodeListSerialNumbers(ctx)
	if err != nil {
		Logc(ctx).Warnf("Could not determine controller serial numbers. %v", err)
	} else {
		Logc(ctx).WithFields(log.Fields{
			"serialNumbers": strings.Join(config.SerialNumbers, ","),
		}).Info("Controller serial numbers.")
	}

	return client, nil
}

// Initialize from the provided config
func (d *NASStorageDriverRest) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NASStorageDriverRest"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.API, err = InitializeOntapDriverRest(ctx, config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	// TODO implement
	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommonRest(ctx, d, d.getStoragePoolAttributes(), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	// Validate the none, true/false values
	err = d.validate(ctx)
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	// TODO implement
	// d.Telemetry = NewOntapTelemetry(ctx, d)
	// d.Telemetry.Start(ctx)

	d.initialized = true
	return nil
}

// InitializeStoragePoolsCommonRest
func InitializeStoragePoolsCommonRest(
	ctx context.Context, d *NASStorageDriverRest, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]*storage.Pool, map[string]*storage.Pool, error) {

	// TODO merge this

	config := d.GetConfig()
	physicalPools := make(map[string]*storage.Pool)
	virtualPools := make(map[string]*storage.Pool)

	// To identify list of media types supported by physical pools
	mediaOffers := make([]sa.Offer, 0)

	// Get name of the physical storage pools which in case of ONTAP is list of aggregates
	physicalStoragePoolNames, err := discoverBackendAggrNamesCommonRest(ctx, d)
	if err != nil || len(physicalStoragePoolNames) == 0 {
		return physicalPools, virtualPools, fmt.Errorf("could not get storage pools from array: %v", err)
	}

	// Create a map of Physical storage pool name to their attributes map
	physicalStoragePoolAttributes := make(map[string]map[string]sa.Offer)
	for _, physicalStoragePoolName := range physicalStoragePoolNames {
		physicalStoragePoolAttributes[physicalStoragePoolName] = make(map[string]sa.Offer)
	}

	// Update physical pool attributes map with aggregate info (i.e. MediaType)
	aggrErr := getVserverAggrAttributesRest(ctx, d, &physicalStoragePoolAttributes)

	if zerr, ok := aggrErr.(api.ZapiError); ok && zerr.IsScopeError() {
		Logc(ctx).WithFields(log.Fields{
			"username": config.Username,
		}).Warn("User has insufficient privileges to obtain aggregate info. " +
			"Storage classes with physical attributes such as 'media' will not match pools on this backend.")
	} else if aggrErr != nil {
		Logc(ctx).Errorf("Could not obtain aggregate info; storage classes with physical attributes such as 'media' will"+
			" not match pools on this backend: %v.", aggrErr)
	}

	// Define physical pools
	for _, physicalStoragePoolName := range physicalStoragePoolNames {

		pool := storage.NewStoragePool(nil, physicalStoragePoolName)

		// Update pool with attributes set by default for this backend
		// We do not set internal attributes with these values as this
		// merely means that pools supports these capabilities like
		// encryption, cloning, thick/thin provisioning
		for attrName, offer := range poolAttributes {
			pool.Attributes[attrName] = offer
		}

		attrMap := physicalStoragePoolAttributes[physicalStoragePoolName]

		// Update pool with attributes based on aggregate attributes discovered on the backend
		for attrName, attrValue := range attrMap {
			pool.Attributes[attrName] = attrValue
			pool.InternalAttributes[attrName] = attrValue.ToString()

			if attrName == sa.Media {
				mediaOffers = append(mediaOffers, attrValue)
			}
		}

		if config.Region != "" {
			pool.Attributes[sa.Region] = sa.NewStringOffer(config.Region)
		}
		if config.Zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(config.Zone)
		}

		pool.Attributes[sa.Labels] = sa.NewLabelOffer(config.Labels)

		pool.InternalAttributes[Size] = config.Size
		pool.InternalAttributes[Region] = config.Region
		pool.InternalAttributes[Zone] = config.Zone
		pool.InternalAttributes[SpaceReserve] = config.SpaceReserve
		pool.InternalAttributes[SnapshotPolicy] = config.SnapshotPolicy
		pool.InternalAttributes[SnapshotReserve] = config.SnapshotReserve
		pool.InternalAttributes[SplitOnClone] = config.SplitOnClone
		pool.InternalAttributes[Encryption] = config.Encryption
		pool.InternalAttributes[UnixPermissions] = config.UnixPermissions
		pool.InternalAttributes[SnapshotDir] = config.SnapshotDir
		pool.InternalAttributes[ExportPolicy] = config.ExportPolicy
		pool.InternalAttributes[SecurityStyle] = config.SecurityStyle
		pool.InternalAttributes[TieringPolicy] = config.TieringPolicy
		pool.InternalAttributes[QosPolicy] = config.QosPolicy
		pool.InternalAttributes[AdaptiveQosPolicy] = config.AdaptiveQosPolicy

		pool.SupportedTopologies = config.SupportedTopologies

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes[SpaceAllocation] = config.SpaceAllocation
			pool.InternalAttributes[FileSystemType] = config.FileSystemType
		}

		physicalPools[pool.Name] = pool
	}

	// Define virtual pools
	for index, vpool := range config.Storage {

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
		supportedTopologies := config.SupportedTopologies
		if vpool.SupportedTopologies != nil {
			supportedTopologies = vpool.SupportedTopologies
		}

		spaceAllocation := config.SpaceAllocation
		if vpool.SpaceAllocation != "" {
			spaceAllocation = vpool.SpaceAllocation
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

		fileSystemType := config.FileSystemType
		if vpool.FileSystemType != "" {
			fileSystemType = vpool.FileSystemType
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

		pool := storage.NewStoragePool(nil, poolName(fmt.Sprintf("pool_%d", index), backendName))

		// Update pool with attributes set by default for this backend
		// We do not set internal attributes with these values as this
		// merely means that pools supports these capabilities like
		// encryption, cloning, thick/thin provisioning
		for attrName, offer := range poolAttributes {
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
				return nil, nil, fmt.Errorf("invalid boolean value for encryption: %v in virtual pool: %s", err,
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
		pool.InternalAttributes[SplitOnClone] = splitOnClone
		pool.InternalAttributes[UnixPermissions] = unixPermissions
		pool.InternalAttributes[SnapshotDir] = snapshotDir
		pool.InternalAttributes[ExportPolicy] = exportPolicy
		pool.InternalAttributes[SecurityStyle] = securityStyle
		pool.InternalAttributes[TieringPolicy] = tieringPolicy
		pool.InternalAttributes[QosPolicy] = qosPolicy
		pool.InternalAttributes[AdaptiveQosPolicy] = adaptiveQosPolicy
		pool.SupportedTopologies = supportedTopologies

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes[SpaceAllocation] = spaceAllocation
			pool.InternalAttributes[FileSystemType] = fileSystemType
		}

		virtualPools[pool.Name] = pool
	}

	return physicalPools, virtualPools, nil
}

// discoverBackendAggrNamesCommon discovers names of the aggregates assigned to the configured SVM
func discoverBackendAggrNamesCommonRest(ctx context.Context, d *NASStorageDriverRest) ([]string, error) {

	// TODO merge this

	client := d.GetAPI()
	config := d.GetConfig()
	driverName := d.Name()
	var err error

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := client.VserverGetAggregateNames()
	if err != nil {
		return nil, err
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates", config.SVM)
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"svm":   config.SVM,
		"pools": vserverAggrs,
	}).Debug("Read storage pools assigned to SVM.")

	var aggrNames []string
	for _, aggrName := range vserverAggrs {
		if config.Aggregate != "" {
			if aggrName != config.Aggregate {
				continue
			}

			Logc(ctx).WithFields(log.Fields{
				"driverName": driverName,
				"aggregate":  config.Aggregate,
			}).Debug("Provisioning will be restricted to the aggregate set in the backend config.")
		}

		aggrNames = append(aggrNames, aggrName)
	}

	// Make sure the configured aggregate is available to the SVM
	if config.Aggregate != "" && (len(aggrNames) == 0) {
		err = fmt.Errorf("the assigned aggregates for SVM %s do not include the configured aggregate %s",
			config.SVM, config.Aggregate)
		return nil, err
	}

	return aggrNames, nil
}

// getVserverAggrAttributes gets pool attributes using vserver-show-aggr-get-iter,
// which will only succeed on Data ONTAP 9 and later.
// If the aggregate attributes are read successfully, the pools passed to this function are updated accordingly.
func getVserverAggrAttributesRest(
	ctx context.Context, d *NASStorageDriverRest, poolsAttributeMap *map[string]map[string]sa.Offer,
) (err error) {

	// TODO merge this

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	/*
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
				aggrName := string(aggr.AggregateName())
				aggrType := aggr.AggregateType()

				// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
				_, ok := (*poolsAttributeMap)[aggrName]
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

				Logc(ctx).WithFields(log.Fields{
					"aggregate": aggrName,
					"mediaType": aggrType,
				}).Debug("Read aggregate attributes.")

				// Update the pool with the aggregate storage attributes
				for attrName, attr := range storageAttrs {
					(*poolsAttributeMap)[aggrName][attrName] = attr
				}
			}
		}
	*/

	result, err := d.GetAPI().AggregateList(ctx, "*")
	if result == nil || result.Payload.NumRecords == 0 || result.Payload.Records == nil {
		return fmt.Errorf("could not retrieve aggregate information")
	}

	for _, aggr := range result.Payload.Records {
		aggrName := aggr.Name
		aggrType := aggr.BlockStorage.Primary.DiskType // TODO validate this is right

		// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
		_, ok := (*poolsAttributeMap)[aggrName]
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

		Logc(ctx).WithFields(log.Fields{
			"aggregate": aggrName,
			"mediaType": aggrType,
		}).Debug("Read aggregate attributes.")

		// Update the pool with the aggregate storage attributes
		for attrName, attr := range storageAttrs {
			(*poolsAttributeMap)[aggrName][attrName] = attr
		}
	}

	return
}

func (d *NASStorageDriverRest) Initialized() bool {
	return d.initialized
}

func (d *NASStorageDriverRest) Terminate(ctx context.Context, backendUUID string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASStorageDriverRest"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}
	if d.Config.AutoExportPolicy {
		// TODO implement
		// policyName := getExportPolicyName(backendUUID)
		// if err := deleteExportPolicy(ctx, policyName, d.API); err != nil {
		// 	Logc(ctx).Warn(err)
		// }
	}
	if d.Telemetry != nil {
		d.Telemetry.Stop()
	}
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *NASStorageDriverRest) validate(ctx context.Context) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NASStorageDriverRest"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	// TODO implement
	// if err := ValidateNASDriver(ctx, d.API, &d.Config); err != nil {
	// 	return fmt.Errorf("driver validation failed: %v", err)
	// }

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// TODO implement
	// if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d, api.MaxNASLabelLength); err != nil {
	// 	return fmt.Errorf("storage pool validation failed: %v", err)
	// }

	return nil
}

// Create a volume with the specified options
func (d *NASStorageDriverRest) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NASStorageDriverRest",
			"name":   name,
			"attrs":  volAttributes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Create")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Create")
	}

	// If the volume already exists, bail out
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
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
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes[Size])
	if err != nil {
		return err
	}

	// Get options
	opts, err := d.GetVolumeOpts(ctx, volConfig, volAttributes)
	if err != nil {
		return err
	}

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	size := strconv.FormatUint(sizeBytes, 10)
	spaceReserve := utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes[SpaceReserve])
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes[SnapshotPolicy])
	snapshotReserve := utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes[SnapshotReserve])
	unixPermissions := utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes[UnixPermissions])
	snapshotDir := utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes[SnapshotDir])
	exportPolicy := utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes[ExportPolicy])
	securityStyle := utils.GetV(opts, "securityStyle", storagePool.InternalAttributes[SecurityStyle])
	encryption := utils.GetV(opts, "encryption", storagePool.InternalAttributes[Encryption])
	tieringPolicy := utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes[TieringPolicy])
	qosPolicy := storagePool.InternalAttributes[QosPolicy]
	adaptiveQosPolicy := storagePool.InternalAttributes[AdaptiveQosPolicy]

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

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
		// TODO implement
		//tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend.BackendUUID)
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy

	Logc(ctx).WithFields(log.Fields{
		"name":              name,
		"size":              size,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"snapshotDir":       enableSnapshotDir,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"encryption":        enableEncryption,
		"tieringPolicy":     tieringPolicy,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name
		physicalPoolNames = append(physicalPoolNames, aggregate)

		// TODO implement
		// if aggrLimitsErr := checkAggregateLimits(
		// 	ctx, aggregate, spaceReserve, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
		// 	errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error: %v", storagePool.Name, aggregate, aggrLimitsErr)
		// 	Logc(ctx).Error(errMessage)
		// 	createErrors = append(createErrors, fmt.Errorf(errMessage))
		// 	continue
		// }

		labels, err := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxNASLabelLength)
		if err != nil {
			return err
		}

		// Create the volume
		createResult, err := d.API.VolumeCreate(
			ctx, name, aggregate, size, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle,
			tieringPolicy, labels, qosPolicyGroup, enableEncryption, snapshotReserveInt)

		if err != nil {
			return fmt.Errorf("error creating volume %v: %v", name, err)
		}
		if createResult == nil {
			return fmt.Errorf("error creating volume %v", name)
		}

		// Check status of the create job
		// TODO refactor (and maybe use the backoff retry?)
		if createResult.Payload != nil {
			if createResult.Payload.Job != nil {
				jobUUID := createResult.Payload.Job.UUID
				fmt.Printf("jobUUID %v\n", jobUUID)

				isTerminalState := false
				for !isTerminalState {
					jobResult, err := d.API.JobGet(string(jobUUID))
					if err != nil {
						fmt.Printf("%v %v\n", jobResult, err.Error())
						return err
					}

					fmt.Printf("%v\n", jobResult)
					switch jobResult.Payload.State {
					case models.JobStateSuccess:
						isTerminalState = true
						fmt.Printf("DONE!\n")
					case models.JobStateFailure:
						isTerminalState = true
						fmt.Printf("DONE!\n")
					default:
						// looping
						fmt.Printf("Sleeping...\n")
						time.Sleep(5 * time.Second) // sleep for 5 seconds
					}
				}
			}
		}

		// if err = api.GetError(ctx, volCreateResponse, err); err != nil {
		// 	if zerr, ok := err.(api.ZapiError); ok {
		// 		// Handle case where the Create is passed to every Docker Swarm node
		// 		if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
		// 			Logc(ctx).WithField("volume", name).Warn("Volume create job already exists, " +
		// 				"skipping volume create on this node.")
		// 			return nil
		// 		}
		// 	}

		// 	errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error creating volume %s: %v", storagePool.Name,
		// 		aggregate, name, err)
		// 	Logc(ctx).Error(errMessage)
		// 	createErrors = append(createErrors, fmt.Errorf(errMessage))
		// 	continue
		// }

		// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
		if !enableSnapshotDir {
			// TODO implement
			// snapDirResponse, err := d.API.VolumeDisableSnapshotDirectoryAccess(name)
			// if err = api.GetError(ctx, snapDirResponse, err); err != nil {
			// 	return fmt.Errorf("error disabling snapshot directory access: %v", err)
			// }
		}

		// Mount the volume at the specified junction
		// mountResponse, err := d.API.VolumeMount(name, "/"+name)
		// if err = api.GetError(ctx, mountResponse, err); err != nil {
		// 	return fmt.Errorf("error mounting volume to junction: %v", err)
		// }
		// TODO validate
		junction := "/" + name
		err = d.API.VolumeMount(ctx, name, junction)
		if err != nil {
			return fmt.Errorf("error mounting volume %v to junction %v: %v", name, junction, err)
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// Create a volume clone
func (d *NASStorageDriverRest) CreateClone(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool,
) error {

	// Ensure the volume exists
	volume, err := d.API.VolumeGetByName(ctx, volConfig.CloneSourceVolumeInternal)
	if err != nil {
		return fmt.Errorf("error looking up volume: %v", err)
	}
	if volume == nil {
		return fmt.Errorf("error looking up volume: %v", volConfig.CloneSourceVolumeInternal)
	}

	// Get the source volume's label
	sourceLabel := ""
	if volume.Comment != nil {
		sourceLabel = *volume.Comment
	}

	return CreateCloneNASRest(ctx, d, volConfig, storagePool, sourceLabel, api.MaxNASLabelLength, false)
}

func CreateCloneNASRest(
	ctx context.Context, d *NASStorageDriverRest, volConfig *storage.VolumeConfig, storagePool *storage.Pool, sourceLabel string,
	labelLimit int, useAsync bool) error {

	// TODO implement
	// if cloning a FlexGroup, useAsync will be true
	// if useAsync && !d.GetAPI().SupportsFeature(ctx, api.NetAppFlexGroupsClone) {
	// 	return errors.New("the ONTAPI version does not support FlexGroup cloning")
	// }

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.GetConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":      "CreateCloneNASRest",
			"Type":        "NASStorageDriver",
			"name":        name,
			"source":      source,
			"snapshot":    snapshot,
			"storagePool": storagePool,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateCloneNASRest")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateCloneNASRest")
	}

	opts, err := d.GetVolumeOpts(context.Background(), volConfig, make(map[string]sa.Request))
	if err != nil {
		return err
	}

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string

	labels := sourceLabel

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.Pool{
			Attributes: map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
			},
		}
		labels, err = storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, labelLimit)
		if err != nil {
			return err
		}
	} else {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes[SplitOnClone]
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
	return CreateOntapCloneRest(ctx, name, source, snapshot, labels, split, d.GetConfig(), d.GetAPI(), useAsync, qosPolicyGroup)
}

// Create a volume clone
func CreateOntapCloneRest(
	ctx context.Context, name, source, snapshot, labels string, split bool, config *drivers.OntapStorageDriverConfig,
	client *api.RestClient, useAsync bool, qosPolicyGroup api.QosPolicyGroup,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateOntapCloneRest",
			"Type":     "ontap_common",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
			"split":    split,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateOntapCloneRest")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateOntapCloneRest")
	}

	// TODO implement
	disabled := true
	if disabled {
		return fmt.Errorf("disabled")
	}
	return nil

	/*
		// If the specified volume already exists, return an error
		volExists, err := client.VolumeExists(ctx, name)
		if err != nil {
			return fmt.Errorf("error checking for existing volume: %v", err)
		}
		if volExists {
			return fmt.Errorf("volume %s already exists", name)
		}

		sourceVolume, err := client.VolumeGetByName(ctx, source)
		if err != nil {
			return fmt.Errorf("error looking up source volume: %v", err)
		}
		if sourceVolume == nil {
			return fmt.Errorf("error looking up source volume: %v", source)
		}

		// If no specific snapshot was requested, create one
		if snapshot == "" {
			snapshot = time.Now().UTC().Format(storage.SnapshotNameFormat)
			err = client.SnapshotCreateAndWait(ctx, sourceVolume.UUID, snapshot)
			if err != nil {
				return fmt.Errorf("could not create snapshot: %v", err)
			}
		}

		// Create the clone based on a snapshot
		if useAsync {
			cloneResponse, err := client.VolumeCloneCreateAsync(name, source, snapshot)
			err = client.WaitForAsyncResponse(ctx, cloneResponse, maxFlexGroupCloneWait)
			if err != nil {
				return errors.New("waiting for async response failed")
			}
		} else {
			cloneResponse, err := client.VolumeCloneCreate(name, source, snapshot)
			if err != nil {
				return fmt.Errorf("error creating clone: %v", err)
			}
			if zerr := api.NewZapiError(cloneResponse); !zerr.IsPassed() {
				return handleCreateOntapCloneErr(ctx, zerr, client, snapshot, source, name)
			}
		}

		err = client.VolumeSetComment(ctx, name, labels)
		if err != nil {
			Logc(ctx).WithField("name", name).Errorf("Modifying comment failed: %v", err)
			return fmt.Errorf("volume %s modify failed: %v", name, err)
		}

		if config.StorageDriverName == drivers.OntapNASStorageDriverName {
			// Mount the new volume
			err = client.VolumeMount(ctx, name, "/"+name)
			if err != nil {
				return fmt.Errorf("error mounting volume to junction: %v", err)
			}
		}

		// Set the QoS Policy if necessary
		if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
			qosResponse, err := client.VolumeSetQosPolicyGroupName(name, qosPolicyGroup)
			if err = api.GetError(ctx, qosResponse, err); err != nil {
				return fmt.Errorf("error setting QoS policy: %v", err)
			}
		}

		// Split the clone if requested
		if split {
			splitResponse, err := client.VolumeCloneSplitStart(name)
			if err = api.GetError(ctx, splitResponse, err); err != nil {
				return fmt.Errorf("error splitting clone: %v", err)
			}
		}

		return nil
	*/
}

// Destroy the volume
func (d *NASStorageDriverRest) Destroy(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NASStorageDriverRest",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.

	volume, err := d.API.VolumeGetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("error looking up volume %v: %v", name, err)
	}

	deleteResult, err := d.API.VolumeDelete(ctx, volume.UUID)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	if deleteResult == nil {
		return fmt.Errorf("error destroying volume %v", name)
	}

	// INSTEAD OF THIS, CHECK JOB STATUS
	// if volDestroyResponse.Error() != "" {
	// 	return fmt.Errorf("error destroying volume %v: %v", name, volDestroyResponse.Error())
	// }
	// TODO refactor (and maybe use the backoff retry?)
	if deleteResult.Payload != nil {
		if deleteResult.Payload.Job != nil {
			jobUUID := deleteResult.Payload.Job.UUID
			fmt.Printf("jobUUID %v\n", jobUUID)

			isTerminalState := false
			for !isTerminalState {
				jobResult, err := d.API.JobGet(string(jobUUID))
				if err != nil {
					fmt.Printf("%v %v\n", jobResult, err.Error())
					return err
				}

				fmt.Printf("%v\n", jobResult)
				switch jobResult.Payload.State {
				case models.JobStateSuccess:
					isTerminalState = true
					fmt.Printf("DONE!\n")
				case models.JobStateFailure:
					isTerminalState = true
					fmt.Printf("DONE!\n")
				default:
					// looping
					fmt.Printf("Sleeping...\n")
					time.Sleep(5 * time.Second) // sleep for 5 seconds
				}
			}
		}
	}

	// TODO do we need to look any deeper at the volDestroyResponse?
	return nil
}

func (d *NASStorageDriverRest) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {

	// TODO implement
	disabled := true
	if disabled {
		fmt.Errorf("disabled")
	}

	// TODO implement
	/*
		if d.Config.DebugTraceFlags["method"] {
			fields := log.Fields{
				"Method":       "Import",
				"Type":         "NASStorageDriverRest",
				"originalName": originalName,
				"newName":      volConfig.InternalName,
				"notManaged":   volConfig.ImportNotManaged,
			}
			Logc(ctx).WithFields(fields).Debug(">>>> Import")
			defer Logc(ctx).WithFields(fields).Debug("<<<< Import")
		}

		// Ensure the volume exists
		flexvol, err := d.API.VolumeGet(originalName)
		if err != nil {
			return err
		} else if flexvol == nil {
			return fmt.Errorf("volume %s not found", originalName)
		}

		// Validate the volume is what it should be
		if flexvol.VolumeIdAttributesPtr != nil {
			volumeIdAttrs := flexvol.VolumeIdAttributes()
			if volumeIdAttrs.TypePtr != nil && volumeIdAttrs.Type() != "rw" {
				Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
				return fmt.Errorf("volume %s type is %s, not rw", originalName, volumeIdAttrs.Type())
			}
		}

		// Get the volume size
		if flexvol.VolumeSpaceAttributesPtr == nil || flexvol.VolumeSpaceAttributesPtr.SizePtr == nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, size not available")
			return fmt.Errorf("volume %s size not available", originalName)
		}
		volConfig.Size = strconv.FormatInt(int64(flexvol.VolumeSpaceAttributesPtr.Size()), 10)

		// Rename the volume if Trident will manage its lifecycle
		if !volConfig.ImportNotManaged {
			renameResponse, err := d.API.VolumeRename(originalName, volConfig.InternalName)
			if err = api.GetError(ctx, renameResponse, err); err != nil {
				Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, rename failed: %v", err)
				return fmt.Errorf("volume %s rename failed: %v", originalName, err)
			}
		}

		// Update the volume labels if Trident will manage its lifecycle
		if !volConfig.ImportNotManaged {
			volumeIdAttrs := flexvol.VolumeIdAttributes()
			if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, volumeIdAttrs.Comment()) {
				modifyCommentResponse, err := d.API.VolumeSetComment(ctx, volConfig.InternalName, "")
				if err = api.GetError(ctx, modifyCommentResponse, err); err != nil {
					Logc(ctx).WithField("originalName", originalName).Errorf("Modifying comment failed: %v", err)
					return fmt.Errorf("volume %s modify failed: %v", originalName, err)
				}
			}
		}

		// Modify unix-permissions of the volume if Trident will manage its lifecycle
		if !volConfig.ImportNotManaged {
			// unixPermissions specified in PVC annotation takes precedence over backend's unixPermissions config
			unixPerms := volConfig.UnixPermissions
			if unixPerms == "" {
				unixPerms = d.Config.UnixPermissions
			}
			modifyUnixPermResponse, err := d.API.VolumeModifyUnixPermissions(volConfig.InternalName, unixPerms)
			if err = api.GetError(ctx, modifyUnixPermResponse, err); err != nil {
				Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, "+
					"modifying unix permissions failed: %v", err)
				return fmt.Errorf("volume %s modify failed: %v", originalName, err)
			}
		}

		// Make sure we're not importing a volume without a junction path when not managed
		if volConfig.ImportNotManaged {
			if flexvol.VolumeIdAttributesPtr == nil {
				return fmt.Errorf("unable to read volume id attributes of volume %s", originalName)
			} else if flexvol.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexvol.VolumeIdAttributesPtr.JunctionPath() == "" {
				return fmt.Errorf("junction path is not set for volume %s", originalName)
			}
		}
	*/

	return nil
}

// Rename changes the name of a volume
func (d *NASStorageDriverRest) Rename(ctx context.Context, name, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "NASStorageDriverRest",
			"name":    name,
			"newName": newName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Rename")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Rename")
	}

	// TODO validate
	// renameResponse, err := d.API.VolumeRename(name, newName)
	// if err = api.GetError(ctx, renameResponse, err); err != nil {
	// 	Logc(ctx).WithField("name", name).Warnf("Could not rename volume: %v", err)
	// 	return fmt.Errorf("could not rename volume %s: %v", name, err)
	// }
	err := d.API.VolumeRename(ctx, name, newName)
	if err != nil {
		Logc(ctx).WithField("name", name).Warnf("Could not rename volume: %v", err)
		return fmt.Errorf("could not rename volume %s: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASStorageDriverRest) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Publish",
			"DataLIF": d.Config.DataLIF,
			"Type":    "NASStorageDriverRest",
			"name":    name,
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

	return publishFlexVolShareRest(ctx, d.API, &d.Config, publishInfo, name)
}

// publishFlexVolShare ensures that the volume has the correct export policy applied.
func publishFlexVolShareRest(
	ctx context.Context, clientAPI *api.RestClient, config *drivers.OntapStorageDriverConfig,
	publishInfo *utils.VolumePublishInfo, volumeName string,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "publishFlexVolShareRest",
			"Type":   "ontap_common",
			"Share":  volumeName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> publishFlexVolShareRest")
		defer Logc(ctx).WithFields(fields).Debug("<<<< publishFlexVolShareRest")
	}

	if !config.AutoExportPolicy || publishInfo.Unmanaged {
		// Nothing to do if we're not configuring export policies automatically or volume is not managed
		return nil
	}

	// TODO implement
	disabled := true
	if disabled {
		return fmt.Errorf("disabled")
	}
	return nil

	/*
		if err := ensureNodeAccess(ctx, publishInfo, clientAPI, config); err != nil {
			return err
		}

		// Update volume to use the correct export policy
		policyName := getExportPolicyName(publishInfo.BackendUUID)
		volumeModifyResponse, err := clientAPI.VolumeModifyExportPolicy(volumeName, policyName)
		if err = api.GetError(ctx, volumeModifyResponse, err); err != nil {
			err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
			Logc(ctx).Error(err)
			return err
		}
		return nil
	*/
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASStorageDriverRest) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NASStorageDriverRest) GetSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) (
	*storage.Snapshot, error,
) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NASStorageDriverRest",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return GetSnapshotRest(ctx, snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func GetSnapshotRest(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.RestClient, sizeGetter func(string) (int, error),
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	volume, err := client.VolumeGetByName(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error looking up volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up volume: %v", err)
	}

	size, err := sizeGetter(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapListResponse, err := client.SnapshotList(volume.UUID)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse == nil {
		return nil, fmt.Errorf("error enumerating snapshots")
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Payload.NumRecords)

	for _, snap := range snapListResponse.Payload.Records {

		Logc(ctx).WithFields(log.Fields{
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
			"created":      snap.CreateTime.String(), // TODO do we need to format this?
			//"created":      snap.AccessTime(),
		}).Debug("Found snapshot.")

		return &storage.Snapshot{
			Config: snapConfig,
			//Created:   time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
			Created:   snap.CreateTime.String(), // TODO do we need to format this?
			SizeBytes: int64(size),
			State:     storage.SnapshotStateOnline,
		}, nil
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Warning("Snapshot not found.")

	return nil, nil
}

// Return the list of snapshots associated with the specified volume
func (d *NASStorageDriverRest) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NASStorageDriverRest",
			"volumeName": volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return GetSnapshotsRest(ctx, volConfig, &d.Config, d.API, d.API.VolumeSize)
}

// GetSnapshotsRest returns the list of snapshots associated with the named volume.
func GetSnapshotsRest(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig, client *api.RestClient,
	sizeGetter func(string) (int, error),
) ([]*storage.Snapshot, error) {

	// TODO merge this
	internalVolName := volConfig.InternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshotsRest",
			"Type":       "ontap_common",
			"volumeName": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshotsRest")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshotsRest")
	}

	volume, err := client.VolumeGetByName(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error looking up volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up volume: %v", err)
	}

	// TODO maybe this size getter logic should be changed to get the volume uuid too
	size, err := sizeGetter(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapListResponse, err := client.SnapshotList(volume.UUID)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse == nil {
		return nil, fmt.Errorf("error enumerating snapshots")
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Payload.NumRecords)
	snapshots := make([]*storage.Snapshot, 0)

	for _, snap := range snapListResponse.Payload.Records {

		Logc(ctx).WithFields(log.Fields{
			"name":       snap.Name,
			"createTime": snap.CreateTime,
		}).Debug("Snapshot")

		snapshot := &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               snap.Name,
				InternalName:       snap.Name,
				VolumeName:         volConfig.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			//Created:   time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
			Created:   snap.CreateTime.String(), // TODO do we need to format this?
			SizeBytes: int64(size),
			State:     storage.SnapshotStateOnline,
		}

		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASStorageDriverRest) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NASStorageDriverRest",
			"snapshotName": internalSnapName,
			"sourceVolume": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return CreateSnapshotRest(ctx, snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// CreateSnapshotRest creates a snapshot for the given volume.
func CreateSnapshotRest(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.RestClient, sizeGetter func(string) (int, error),
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshotRest",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshotRest")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshotRest")
	}

	volume, err := client.VolumeGetByName(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up volume: %v", internalVolName)
	}

	size, err := sizeGetter(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	err = client.SnapshotCreateAndWait(ctx, volume.UUID, internalSnapName)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// TODO here, and in a few other places in this file, refactor?
	snapListResponse, err := client.SnapshotList(volume.UUID)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse == nil {
		return nil, fmt.Errorf("error enumerating snapshots")
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Payload.NumRecords)

	for _, snap := range snapListResponse.Payload.Records {
		Logc(ctx).WithFields(log.Fields{
			"name":       snap.Name,
			"createTime": snap.CreateTime,
		}).Debug("Snapshot")
		if snap.Name == internalSnapName {
			return &storage.Snapshot{
				Config: snapConfig,
				//Created:   time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
				Created:   snap.CreateTime.String(), // TODO do we need to format this?
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}

	return nil, fmt.Errorf("could not find snapshot %s for souce volume %s", internalSnapName, internalVolName)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASStorageDriverRest) RestoreSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	// TODO implement
	disabled := true
	if disabled {
		return fmt.Errorf("disabled")
	}
	return nil

	/*
		if d.Config.DebugTraceFlags["method"] {
			fields := log.Fields{
				"Method":       "RestoreSnapshot",
				"Type":         "NASStorageDriverRest",
				"snapshotName": snapConfig.InternalName,
				"volumeName":   snapConfig.VolumeInternalName,
			}
			Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
			defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
		}

		return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
	*/
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASStorageDriverRest) DeleteSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NASStorageDriverRest",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return DeleteSnapshoRestt(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshoRestt deletes a single snapshot.
func DeleteSnapshoRestt(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.RestClient,
) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshoRestt",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshoRestt")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshoRestt")
	}

	// GET the volume by name
	volume, err := client.VolumeGetByName(ctx, internalVolName)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volume == nil {
		return fmt.Errorf("error looking up volume: %v", internalVolName)
	}
	volumeUUID := volume.UUID

	// GET the snapshot by name
	snapshot, err := client.SnapshotGetByName(volumeUUID, internalSnapName)
	if err != nil {
		return fmt.Errorf("error checking for snapshot: %v", err)
	}
	if snapshot == nil {
		return fmt.Errorf("error looking up snapshot: %v", internalSnapName)
	}
	snapshotUUID := snapshot.UUID

	// DELETE the snapshot
	snapshotDeleteResult, err := client.SnapshotDelete(volumeUUID, snapshotUUID)
	if err != nil {
		return fmt.Errorf("error while deleting snapshot: %v", err)
	}
	if snapshotDeleteResult == nil {
		return fmt.Errorf("error while deleting snapshot: %v", internalSnapName)
	}

	// check snapshot delete job status
	isDone := false
	for !isDone {
		isDone, err = client.IsJobFinished(snapshotDeleteResult.Payload)
		if err != nil {
			isDone = true
			return fmt.Errorf("error deleting snapshot: %v", err)
		}
		if !isDone {
			Logc(ctx).Debug("Sleeping")
			time.Sleep(5 * time.Second) // TODO use backoff retry? make a helper function?
		}
	}

	// TODO handle this volume split on busy in REST
	// if zerr := api.NewZapiError(snapResponse); !zerr.IsPassed() {
	// 	if zerr.Code() == azgo.ESNAPSHOTBUSY {
	// 		// Start a split here before returning the error so a subsequent delete attempt may succeed.
	// 		_ = SplitVolumeFromBusySnapshot(ctx, snapConfig, config, client)
	// 	}
	// 	return fmt.Errorf("error deleting snapshot: %v", zerr)
	// }

	Logc(ctx).WithField("snapshotName", internalSnapName).Debug("Deleted snapshot.")
	return nil
}

// Test for the existence of a volume
func (d *NASStorageDriverRest) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NASStorageDriverRest"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	//return GetVolume(ctx, name, d.API, &d.Config)
	volume, err := d.API.VolumeGetByName(ctx, *d.Config.StoragePrefix+name)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("error looking up volume %v", name)
	}
	return nil
}

// Retrieve storage backend capabilities
func (d *NASStorageDriverRest) GetStorageBackendSpecs(_ context.Context, backend *storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// Retrieve storage backend physical pools
func (d *NASStorageDriverRest) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

func (d *NASStorageDriverRest) getStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASStorageDriverRest) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(ctx, volConfig, requests), nil
}

func (d *NASStorageDriverRest) GetInternalVolumeName(_ context.Context, name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASStorageDriverRest) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	createPrepareCommon(ctx, d, volConfig)
}

func (d *NASStorageDriverRest) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {

	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
	volConfig.FileSystem = ""

	// Set correct junction path
	volume, err := d.API.VolumeGetByName(ctx, volConfig.InternalName)
	if err != nil {
		return err
	} else if volume == nil {
		return fmt.Errorf("volume %s not found", volConfig.InternalName)
	}

	//if flexvol.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexvol.VolumeIdAttributesPtr.JunctionPath() == "" {
	if volume.Nas.Path == "" {
		// Flexvol is not mounted, we need to mount it
		// volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
		// mountResponse, err := d.API.VolumeMount(volConfig.InternalName, volConfig.AccessInfo.NfsPath)
		// if err = api.GetError(ctx, mountResponse, err); err != nil {
		// 	return fmt.Errorf("error mounting volume to junction %s; %v", volConfig.AccessInfo.NfsPath, err)
		// }
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
		err := d.API.VolumeMount(ctx, volConfig.InternalName, volConfig.AccessInfo.NfsPath)
		if err != nil {
			return fmt.Errorf("error mounting volume to junction %s; %v", volConfig.AccessInfo.NfsPath, err)
		}
	} else {
		//volConfig.AccessInfo.NfsPath = flexvol.VolumeIdAttributesPtr.JunctionPath()
		volConfig.AccessInfo.NfsPath = volume.Nas.Path
	}
	return nil
}

func (d *NASStorageDriverRest) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NASStorageDriverRest) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASStorageDriverRest) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NASStorageDriverRest) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {

	// volumeAttributes, err := d.API.VolumeGet(ctx, name)
	// if err != nil {
	// 	return nil, err
	// }

	// return d.getVolumeExternal(volumeAttributes), nil

	volume, err := d.API.VolumeGetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volume), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASStorageDriverRest) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	/*
		// Get all volumes matching the storage prefix
		volumesResponse, err := d.API.VolumeGetAll(*d.Config.StoragePrefix)
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
	*/

	// Get all volumes matching the storage prefix
	volumesListResponse, err := d.API.VolumeList(*d.Config.StoragePrefix + "*")
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	for _, volume := range volumesListResponse.Payload.Records {
		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(volume), Error: nil}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASStorageDriverRest) getVolumeExternal(
	volume *models.Volume,
) *storage.VolumeExternal {

	internalName := volume.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:        tridentconfig.OrchestratorAPIVersion,
		Name:           name,
		InternalName:   internalName,
		Size:           strconv.FormatInt(int64(volume.Size), 10),
		Protocol:       tridentconfig.File,
		SnapshotPolicy: volume.SnapshotPolicy.Name,
		ExportPolicy:   volume.Nas.ExportPolicy.Name,
		//SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		SnapshotDir:     "false",                            // TODO fix this, figure it out for real
		UnixPermissions: string(volume.Nas.UnixPermissions), // TODO fix this, needs to be converted properly
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volume.Aggregates[0].Name, // TODO validate
		//Pool:   volumeIDAttrs.ContainingAggregateName(),
	}

	// volumeConfig := &storage.VolumeConfig{
	// 	Version:         tridentconfig.OrchestratorAPIVersion,
	// 	Name:            name,
	// 	InternalName:    internalName,
	// 	Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
	// 	Protocol:        tridentconfig.File,
	// 	SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
	// 	ExportPolicy:    volumeExportAttrs.Policy(),
	// 	SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
	// 	UnixPermissions: volumeSecurityUnixAttrs.Permissions(),
	// 	StorageClass:    "",
	// 	AccessMode:      tridentconfig.ReadWriteMany,
	// 	AccessInfo:      utils.VolumeAccessInfo{},
	// 	BlockSize:       "",
	// 	FileSystem:      "",
	// }

	// return &storage.VolumeExternal{
	// 	Config: volumeConfig,
	// 	Pool:   volumeIDAttrs.ContainingAggregateName(),
	// }
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASStorageDriverRest) GetUpdateType(ctx context.Context, driverOrig storage.Driver) *roaring.Bitmap {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetUpdateType",
			"Type":   "NASStorageDriverRest",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetUpdateType")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetUpdateType")
	}

	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASStorageDriver)
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

// Resize expands the volume size.
func (d *NASStorageDriverRest) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "NASStorageDriverRest",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	flexvolSize, err := resizeValidation(ctx, name, sizeBytes, d.API.VolumeExists, d.API.VolumeSize)
	if err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(flexvolSize, 10)
	if flexvolSize == sizeBytes {
		return nil
	}

	// TODO implement
	// if aggrLimitsErr := checkAggregateLimitsForFlexvol(
	// 	ctx, name, sizeBytes, d.Config, d.GetAPI()); aggrLimitsErr != nil {
	// 	return aggrLimitsErr
	// }

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// TODO validate
	// response, err := d.API.VolumeSetSize(name, strconv.FormatUint(sizeBytes, 10))
	// if err = api.GetError(ctx, response.Result, err); err != nil {
	// 	Logc(ctx).WithField("error", err).Error("Volume resize failed.")
	// 	return fmt.Errorf("volume resize failed")
	// }
	err = d.API.VolumeSetSize(ctx, name, strconv.FormatUint(sizeBytes, 10))
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

func (d *NASStorageDriverRest) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, backendUUID string) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NASStorageDriverRest",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccessRest(ctx, nodes, &d.Config, d.API, policyName)
}

func reconcileNASNodeAccessRest(
	ctx context.Context, nodes []*utils.Node, config *drivers.OntapStorageDriverConfig, clientAPI *api.RestClient,
	policyName string,
) error {

	if !config.AutoExportPolicy {
		return nil
	}

	// TODO implement
	disabled := true
	if disabled {
		return fmt.Errorf("disabled")
	}
	return nil

	/*
		err := ensureExportPolicyExists(ctx, policyName, clientAPI)
		if err != nil {
			return err
		}
		desiredRules, err := getDesiredExportPolicyRules(ctx, nodes, config)
		if err != nil {
			err = fmt.Errorf("unable to determine desired export policy rules; %v", err)
			Logc(ctx).Error(err)
			return err
		}
		err = reconcileExportPolicyRules(ctx, policyName, desiredRules, clientAPI)
		if err != nil {
			err = fmt.Errorf("unabled to reconcile export policy rules; %v", err)
			Logc(ctx).WithField("ExportPolicy", policyName).Error(err)
			return err
		}
		return nil
	*/
}

// String makes NASStorageDriverRest satisfy the Stringer interface.
func (d NASStorageDriverRest) String() string {
	return drivers.ToString(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASStorageDriverRest satisfy the GoStringer interface.
func (d NASStorageDriverRest) GoString() string {
	return d.String()
}
