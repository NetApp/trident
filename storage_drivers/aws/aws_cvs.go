// Copyright 2020 NetApp, Inc. All Rights Reserved.

package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
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
	"github.com/netapp/trident/storage_drivers/aws/api"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes    = 1000000000   // 1 GB
	MinimumCVSVolumeSizeBytes = 100000000000 // 100 GB
	MinimumAPIVersion         = "1.1.8"
	MinimumSDEVersion         = "2020.4.0"

	defaultServiceLevel    = api.ServiceLevelStandard
	defaultNfsMountOptions = "-o nfsvers=3"
	defaultSecurityStyle   = "unix"
	defaultSnapshotDir     = "false"
	defaultSnapshotReserve = ""
	defaultLimitVolumeSize = ""
	defaultExportRule      = "0.0.0.0/0"

	// Constants for internal pool attributes
	Size            = "size"
	ServiceLevel    = "serviceLevel"
	SnapshotDir     = "snapshotDir"
	SnapshotReserve = "snapshotReserve"
	ExportRule      = "exportRule"
	Region          = "region"
	Zone            = "zone"
)

// NFSStorageDriver is for storage provisioning using Cloud Volumes Service in AWS
type NFSStorageDriver struct {
	initialized         bool
	Config              drivers.AWSNFSStorageDriverConfig
	API                 *api.Client
	apiVersion          *utils.Version
	sdeVersion          *utils.Version
	tokenRegexp         *regexp.Regexp
	csiRegexp           *regexp.Regexp
	apiRegions          []string
	pools               map[string]*storage.Pool
	volumeCreateTimeout time.Duration
}

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin string `json:"plugin"`
}

func (d *NFSStorageDriver) GetConfig() *drivers.AWSNFSStorageDriverConfig {
	return &d.Config
}

func (d *NFSStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *NFSStorageDriver) getTelemetry() *Telemetry {
	return &Telemetry{
		Telemetry: tridentconfig.OrchestratorTelemetry,
		Plugin:    d.Name(),
	}
}

// Name returns the name of this driver
func (d *NFSStorageDriver) Name() string {
	return drivers.AWSNFSStorageDriverName
}

// defaultBackendName returns the default name of the backend managed by this driver instance
func (d *NFSStorageDriver) defaultBackendName() string {
	return fmt.Sprintf("%s_%s",
		strings.Replace(d.Name(), "-", "", -1),
		d.Config.APIKey[0:5])
}

// BackendName returns the name of the backend managed by this driver instance
func (d *NFSStorageDriver) BackendName() string {
	if d.Config.BackendName != "" {
		return d.Config.BackendName
	} else {
		// Use the old naming scheme if no name is specified
		return d.defaultBackendName()
	}
}

// poolName constructs the name of the pool reported by this driver instance
func (d *NFSStorageDriver) poolName(name string) string {

	return fmt.Sprintf("%s_%s",
		d.BackendName(),
		strings.Replace(name, "-", "", -1))
}

// validateName checks that the name of a new volume matches the requirements of a creation token
func (d *NFSStorageDriver) validateName(name string) error {
	if !d.tokenRegexp.MatchString(name) {
		return fmt.Errorf("volume name '%s' is not allowed; it must be at most 80 characters long, "+
			"begin with a character, and contain only characters, digits, hyphens and underscores", name)
	}
	return nil
}

// defaultCreateTimeout sets the driver timeout for volume create/delete operations.  Docker gets more time, since
// it doesn't have a mechanism to retry.
func (d *NFSStorageDriver) defaultCreateTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerCreateTimeout
	default:
		return api.VolumeCreateTimeout
	}
}

// defaultTimeout controls the driver timeout for most API operations.
func (d *NFSStorageDriver) defaultTimeout() time.Duration {
	switch d.Config.DriverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerDefaultTimeout
	default:
		return api.DefaultTimeout
	}
}

// Initialize initializes this driver from the provided config
func (d *NFSStorageDriver) Initialize(
	ctx context.Context, context tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, _ string,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	commonConfig.DriverContext = context
	d.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-_]{0,79}$`)
	d.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// Parse the config
	config, err := d.initializeAWSConfig(ctx, configJSON, commonConfig)
	if err != nil {
		return fmt.Errorf("error initializing %s driver. %v", d.Name(), err)
	}
	d.Config = *config

	if err = d.populateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	if err = d.initializeStoragePools(ctx); err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	if d.API, err = d.initializeAWSAPIClient(ctx, &d.Config); err != nil {
		return fmt.Errorf("error initializing %s API client. %v", d.Name(), err)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver. %v", d.Name(), err)
	}

	d.initialized = true
	return nil
}

// Initialized returns whether this driver has been initialized (and not terminated)
func (d *NFSStorageDriver) Initialized() bool {
	return d.initialized
}

// Terminate stops the driver prior to its being unloaded
func (d *NFSStorageDriver) Terminate(ctx context.Context, _ string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

// populateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func (d *NFSStorageDriver) populateConfigurationDefaults(
	ctx context.Context, config *drivers.AWSNFSStorageDriverConfig,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "populateConfigurationDefaults", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> populateConfigurationDefaults")
		defer Logc(ctx).WithFields(fields).Debug("<<<< populateConfigurationDefaults")
	}

	if config.StoragePrefix == nil {
		defaultPrefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		defaultPrefix = strings.Replace(defaultPrefix, "_", "-", -1)
		config.StoragePrefix = &defaultPrefix
	}

	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	}

	if config.ServiceLevel == "" {
		config.ServiceLevel = defaultServiceLevel
	}

	if config.NfsMountOptions == "" {
		config.NfsMountOptions = defaultNfsMountOptions
	}

	if config.SnapshotDir == "" {
		config.SnapshotDir = defaultSnapshotDir
	}

	if config.SnapshotReserve == "" {
		config.SnapshotReserve = defaultSnapshotReserve
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = defaultLimitVolumeSize
	}

	if config.ExportRule == "" {
		config.ExportRule = defaultExportRule
	}

	// VolumeCreateTimeoutSeconds is the timeout value in seconds.
	volumeCreateTimeout := d.defaultCreateTimeout()
	if config.VolumeCreateTimeout != "" {
		i, err := strconv.ParseUint(d.Config.VolumeCreateTimeout, 10, 64)
		if err != nil {
			Logc(ctx).WithField("interval", d.Config.VolumeCreateTimeout).Errorf(
				"Invalid volume create timeout period. %v", err)
			return err
		}
		volumeCreateTimeout = time.Duration(i) * time.Second
	}
	d.volumeCreateTimeout = volumeCreateTimeout

	Logc(ctx).WithFields(log.Fields{
		"StoragePrefix":              *config.StoragePrefix,
		"Size":                       config.Size,
		"ServiceLevel":               config.ServiceLevel,
		"NfsMountOptions":            config.NfsMountOptions,
		"SnapshotDir":                config.SnapshotDir,
		"SnapshotReserve":            config.SnapshotReserve,
		"LimitVolumeSize":            config.LimitVolumeSize,
		"ExportRule":                 config.ExportRule,
		"VolumeCreateTimeoutSeconds": config.VolumeCreateTimeout,
	}).Debugf("Configuration defaults")

	return nil
}

// initializeStoragePools defines the pools reported to Trident, whether physical or virtual.
func (d *NFSStorageDriver) initializeStoragePools(ctx context.Context) error {

	d.pools = make(map[string]*storage.Pool)

	if len(d.Config.Storage) == 0 {

		Logc(ctx).Debug("No vpools defined, reporting single pool.")

		// No vpools defined, so report region/zone as a single pool
		pool := storage.NewStoragePool(nil, d.poolName("pool"))

		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
		pool.Attributes[sa.Clones] = sa.NewBoolOffer(true)
		pool.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		pool.Attributes[sa.Labels] = sa.NewLabelOffer(d.Config.Labels)
		if d.Config.Region != "" {
			pool.Attributes[sa.Region] = sa.NewStringOffer(d.Config.Region)
		}
		if d.Config.Zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(d.Config.Zone)
		}

		pool.InternalAttributes[Size] = d.Config.Size
		pool.InternalAttributes[ServiceLevel] = d.Config.ServiceLevel
		pool.InternalAttributes[SnapshotDir] = d.Config.SnapshotDir
		pool.InternalAttributes[SnapshotReserve] = d.Config.SnapshotReserve
		pool.InternalAttributes[ExportRule] = d.Config.ExportRule
		pool.InternalAttributes[Region] = d.Config.Region
		pool.InternalAttributes[Zone] = d.Config.Zone

		pool.SupportedTopologies = d.Config.SupportedTopologies

		d.pools[pool.Name] = pool

	} else {

		Logc(ctx).Debug("One or more vpools defined.")

		// Report a pool for each virtual pool in the config
		for index, vpool := range d.Config.Storage {

			region := d.Config.Region
			if vpool.Region != "" {
				region = vpool.Region
			}

			zone := d.Config.Zone
			if vpool.Zone != "" {
				zone = vpool.Zone
			}

			size := d.Config.Size
			if vpool.Size != "" {
				size = vpool.Size
			}

			supportedTopologies := d.Config.SupportedTopologies
			if vpool.SupportedTopologies != nil {
				supportedTopologies = vpool.SupportedTopologies
			}

			serviceLevel := d.Config.ServiceLevel
			if vpool.ServiceLevel != "" {
				serviceLevel = vpool.ServiceLevel
			}

			snapshotDir := d.Config.SnapshotDir
			if vpool.SnapshotDir != "" {
				snapshotDir = vpool.SnapshotDir
			}

			snapshotReserve := d.Config.SnapshotReserve
			if vpool.SnapshotReserve != "" {
				snapshotReserve = vpool.SnapshotReserve
			}

			exportRule := d.Config.ExportRule
			if vpool.ExportRule != "" {
				exportRule = vpool.ExportRule
			}

			pool := storage.NewStoragePool(nil, d.poolName(fmt.Sprintf("pool_%d", index)))

			pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
			pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
			pool.Attributes[sa.Clones] = sa.NewBoolOffer(true)
			pool.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
			pool.Attributes[sa.Labels] = sa.NewLabelOffer(d.Config.Labels, vpool.Labels)
			if region != "" {
				pool.Attributes[sa.Region] = sa.NewStringOffer(region)
			}
			if zone != "" {
				pool.Attributes[sa.Zone] = sa.NewStringOffer(zone)
			}

			pool.InternalAttributes[Size] = size
			pool.InternalAttributes[ServiceLevel] = serviceLevel
			pool.InternalAttributes[SnapshotDir] = snapshotDir
			pool.InternalAttributes[SnapshotReserve] = snapshotReserve
			pool.InternalAttributes[ExportRule] = exportRule
			pool.InternalAttributes[Region] = region
			pool.InternalAttributes[Zone] = zone

			pool.SupportedTopologies = supportedTopologies

			d.pools[pool.Name] = pool
		}
	}

	return nil
}

// initializeAWSConfig parses the AWS config, mixing in the specified common config.
func (d *NFSStorageDriver) initializeAWSConfig(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) (*drivers.AWSNFSStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAWSConfig", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeAWSConfig")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeAWSConfig")
	}

	config := &drivers.AWSNFSStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into AWSNFSStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration. %v", err)
	}

	return config, nil
}

// initializeAWSAPIClient returns an AWS API client.
func (d *NFSStorageDriver) initializeAWSAPIClient(
	ctx context.Context, config *drivers.AWSNFSStorageDriverConfig,
) (*api.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "initializeAWSAPIClient", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> initializeAWSAPIClient")
		defer Logc(ctx).WithFields(fields).Debug("<<<< initializeAWSAPIClient")
	}

	// Ensure the API URL is well-formed
	_, err := url.Parse(config.APIURL)
	if err != nil {
		return nil, fmt.Errorf("config field apiURL (%s) is malformed: %v", config.APIURL, err)
	}

	client := api.NewDriver(api.ClientConfig{
		APIURL:          config.APIURL,
		APIKey:          config.APIKey,
		SecretKey:       config.SecretKey,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	return client, nil
}

// validate ensures the driver configuration and execution environment are valid and working
func (d *NFSStorageDriver) validate(ctx context.Context) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	var err error

	// Validate API version
	if d.apiVersion, d.sdeVersion, err = d.API.GetVersion(ctx); err != nil {
		return err
	}

	if d.apiVersion.LessThan(utils.MustParseSemantic(MinimumAPIVersion)) {
		return fmt.Errorf("API version is %s, at least %s is required", d.apiVersion.String(), MinimumAPIVersion)
	}
	if d.sdeVersion.LessThan(utils.MustParseSemantic(MinimumSDEVersion)) {
		return fmt.Errorf("SDE version is %s, at least %s is required", d.sdeVersion.String(), MinimumSDEVersion)
	}

	// Get available regions
	regions, err := d.API.GetRegions(ctx)
	if err != nil {
		return err
	}

	d.apiRegions = make([]string, 0)
	for _, region := range *regions {
		d.apiRegions = append(d.apiRegions, region.Name)
	}

	Logc(ctx).WithField("regions", d.apiRegions).Debug("REST API access OK.")

	// Validate driver-level attributes

	// Ensure API region is set
	if d.Config.APIRegion == "" {
		return errors.New("apiRegion in config must be specified")
	}

	// Ensure API region matches one of the regions returned by the API
	if !utils.StringInSlice(d.Config.APIRegion, d.apiRegions) {
		return fmt.Errorf("apiRegion %s in config not found; available regions include %s",
			d.Config.APIRegion, strings.Join(d.apiRegions, ","))
	}

	// Ensure storage prefix is compatible with cloud service
	if err := validateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	// Validate pool-level attributes
	for poolName, pool := range d.pools {

		// Validate service level
		switch pool.InternalAttributes[ServiceLevel] {
		case api.ServiceLevelStandard, api.ServiceLevelPremium, api.ServiceLevelExtreme:
			break
		default:
			return fmt.Errorf("invalid service level in pool %s: %s",
				poolName, pool.InternalAttributes[ServiceLevel])
		}

		// Validate export rules
		for _, rule := range strings.Split(pool.InternalAttributes[ExportRule], ",") {
			ipAddr := net.ParseIP(rule)
			_, netAddr, _ := net.ParseCIDR(rule)
			if ipAddr == nil && netAddr == nil {
				return fmt.Errorf("invalid address/CIDR for exportRule in pool %s: %s", poolName, rule)
			}
		}

		// Validate snapshot dir
		if pool.InternalAttributes[SnapshotDir] != "" {
			_, err := strconv.ParseBool(pool.InternalAttributes[SnapshotDir])
			if err != nil {
				return fmt.Errorf("invalid value for snapshotDir in pool %s: %v", poolName, err)
			}
		}

		// Validate snapshot reserve
		if pool.InternalAttributes[SnapshotReserve] != "" {
			snapshotReserve, err := strconv.ParseInt(pool.InternalAttributes[SnapshotReserve], 10, 0)
			if err != nil {
				return fmt.Errorf("invalid value for snapshotReserve in pool %s: %v", poolName, err)
			}
			if snapshotReserve < 0 || snapshotReserve > 90 {
				return fmt.Errorf("invalid value for snapshotReserve in pool %s: %s",
					poolName, pool.InternalAttributes[SnapshotReserve])
			}
		}

		// Validate default size
		if _, err = utils.ConvertSizeToBytes(pool.InternalAttributes[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", poolName, err)
		}

		if _, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength); err != nil {
			return fmt.Errorf("invalid value for label in pool %s: %v", poolName, err)
		}
	}

	return nil
}

// Create a volume with the specified options
func (d *NFSStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NFSStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Create")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Create")
	}

	// Make sure we got a valid name
	if err := d.validateName(name); err != nil {
		return err
	}

	// Get the pool since most default values are pool-specific
	if storagePool == nil {
		return errors.New("pool not specified")
	}
	pool, ok := d.pools[storagePool.Name]
	if !ok {
		return fmt.Errorf("pool %s does not exist", storagePool.Name)
	}

	// If the volume already exists, bail out
	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volumeExists {
		if extantVolume.Region != d.Config.APIRegion {
			return fmt.Errorf("volume %s requested in region %s, but it already exists in region %s",
				name, d.Config.APIRegion, extantVolume.Region)
		}
		if extantVolume.LifeCycleState == api.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}
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
	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(pool.InternalAttributes[Size])
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return fmt.Errorf("requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes)
	}

	// TODO: remove this code once CVS can handle smaller volumes
	if sizeBytes < MinimumCVSVolumeSizeBytes {

		Logc(ctx).WithFields(log.Fields{
			"name": name,
			"size": sizeBytes,
		}).Warningf("Requested size is too small. Setting volume size to the minimum allowable (100 GB).")

		sizeBytes = MinimumCVSVolumeSizeBytes
		volConfig.Size = fmt.Sprintf("%d", sizeBytes)
	}

	if _, _, err := drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Take service level from volume config first (handles Docker case), then from pool
	serviceLevel := volConfig.ServiceLevel
	if serviceLevel == "" {
		serviceLevel = pool.InternalAttributes[ServiceLevel]
	}
	switch serviceLevel {
	case api.ServiceLevelStandard, api.ServiceLevelPremium, api.ServiceLevelExtreme:
		break
	default:
		return fmt.Errorf("invalid service level: %s", serviceLevel)
	}

	// Take snapshot directory from volume config first (handles Docker case), then from pool
	snapshotDir := volConfig.SnapshotDir
	if snapshotDir == "" {
		snapshotDir = pool.InternalAttributes[SnapshotDir]
	}
	snapshotDirBool, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotDir: %v", err)
	}

	// Take snapshot reserve from volume config first (handles Docker case), then from pool
	snapshotReserve := volConfig.SnapshotReserve
	if snapshotReserve == "" {
		snapshotReserve = pool.InternalAttributes[SnapshotReserve]
	}
	var snapshotReservePtr *int64
	if snapshotReserve != "" {
		snapshotReserveInt, err := strconv.ParseInt(snapshotReserve, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid value for snapshotReserve: %v", err)
		}
		snapshotReservePtr = &snapshotReserveInt
	}

	protocolTypes := []string{api.ProtocolTypeNFSv3, api.ProtocolTypeNFSv4}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":   name,
		"size":            sizeBytes,
		"serviceLevel":    serviceLevel,
		"snapshotDir":     snapshotDirBool,
		"snapshotReserve": snapshotReserve,
		"protocolTypes":   protocolTypes,
	}).Debug("Creating volume.")

	apiExportRule := api.ExportRule{
		AllowedClients: pool.InternalAttributes[ExportRule],
		Cifs:           false,
		Nfsv3:          true,
		Nfsv4:          true,
		RuleIndex:      1,
		UnixReadOnly:   false,
		UnixReadWrite:  true,
	}
	exportPolicy := api.ExportPolicy{
		Rules: []api.ExportRule{apiExportRule},
	}

	labels := []string{d.getTelemetryLabels(ctx)}
	poolLabels, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
	if err != nil {
		return err
	}
	if poolLabels != "" {
		labels = append(labels, poolLabels)
	}

	snapshotPolicy := api.SnapshotPolicy{
		Enabled: false,
		MonthlySchedule: api.MonthlySchedule{
			DaysOfMonth: "1",
		},
		WeeklySchedule: api.WeeklySchedule{
			Day: "Sunday",
		},
	}

	createRequest := &api.FilesystemCreateRequest{
		Name:              volConfig.Name,
		Region:            d.Config.APIRegion,
		CreationToken:     name,
		ExportPolicy:      exportPolicy,
		Labels:            labels,
		ProtocolTypes:     protocolTypes,
		QuotaInBytes:      int64(sizeBytes),
		SecurityStyle:     defaultSecurityStyle,
		ServiceLevel:      serviceLevel,
		SnapshotPolicy:    snapshotPolicy,
		SnapshotDirectory: snapshotDirBool,
		SnapReserve:       snapshotReservePtr,
	}

	// Create the volume
	volume, err := d.API.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, volume, name)
}

// CreateClone clones an existing volume.  If a snapshot is not specified, one is created.
func (d *NFSStorageDriver) CreateClone(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool,
) error {

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "NFSStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateClone")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateClone")
	}

	// ensure new volume doesn't exist, fail if so
	// get source volume, fail if nonexistent or if wrong region
	// if snapshot specified, read snapshots from source, fail if nonexistent
	// if no snap specified, create one, fail if error
	// create volume from snapshot

	// Make sure we got a valid name
	if err := d.validateName(name); err != nil {
		return err
	}

	// If the volume already exists, bail out
	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volumeExists {
		if extantVolume.LifeCycleState == api.StateCreating {
			// This is a retry and the volume still isn't ready, so no need to wait further.
			return utils.VolumeCreatingError(
				fmt.Sprintf("volume state is still %s, not %s", api.StateCreating, api.StateAvailable))
		}
		return drivers.NewVolumeExistsError(name)
	}

	// Get the source volume by creation token (efficient query) to get its ID
	sourceVolume, err := d.API.GetVolumeByCreationToken(ctx, source)
	if err != nil {
		return fmt.Errorf("could not find source volume: %v", err)
	}
	if sourceVolume.Region != d.Config.APIRegion {
		return fmt.Errorf("volume %s requested in region %s, but it already exists in region %s",
			name, d.Config.APIRegion, sourceVolume.Region)
	}

	// Get the source volume by ID to get all details
	sourceVolume, err = d.API.GetVolumeByID(ctx, sourceVolume.FileSystemID)
	if err != nil {
		return fmt.Errorf("could not get source volume details: %v", err)
	}

	var sourceSnapshot *api.Snapshot

	if snapshot != "" {

		// Get the source snapshot
		sourceSnapshot, err = d.API.GetSnapshotForVolume(ctx, sourceVolume, snapshot)
		if err != nil {
			return fmt.Errorf("could not find source snapshot: %v", err)
		}

		// Ensure snapshot is in a usable state
		if sourceSnapshot.LifeCycleState != api.StateAvailable {
			return fmt.Errorf("source snapshot state is '%s', it must be '%s'",
				sourceSnapshot.LifeCycleState, api.StateAvailable)
		}

	} else {

		// No source snapshot specified, so create one
		snapshotCreateRequest := &api.SnapshotCreateRequest{
			FileSystemID: sourceVolume.FileSystemID,
			Name:         time.Now().UTC().Format(storage.SnapshotNameFormat),
			Region:       sourceVolume.Region,
		}

		sourceSnapshot, err = d.API.CreateSnapshot(ctx, snapshotCreateRequest)
		if err != nil {
			return fmt.Errorf("could not create source snapshot: %v", err)
		}

		// Wait for snapshot creation to complete
		err = d.API.WaitForSnapshotState(
			ctx, sourceSnapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
		if err != nil {
			return err
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"creationToken":  name,
		"sourceVolume":   sourceVolume.CreationToken,
		"sourceSnapshot": sourceSnapshot.Name,
	}).Debug("Cloning volume.")

	var labels []string
	labels = d.updateTelemetryLabels(ctx, sourceVolume)

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.Pool{
			Attributes: map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
			},
		}
		poolLabels, err := storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxLabelLength)
		if err != nil {
			return err
		}

		labels = storage.UpdateProvisioningLabels(poolLabels, labels)
	}

	createRequest := &api.FilesystemCreateRequest{
		Name:              volConfig.Name,
		Region:            sourceVolume.Region,
		CreationToken:     name,
		ExportPolicy:      sourceVolume.ExportPolicy,
		Labels:            labels,
		ProtocolTypes:     sourceVolume.ProtocolTypes,
		QuotaInBytes:      sourceVolume.QuotaInBytes,
		ServiceLevel:      sourceVolume.ServiceLevel,
		SnapshotDirectory: sourceVolume.SnapshotDirectory,
		SnapshotPolicy:    sourceVolume.SnapshotPolicy,
		SnapshotID:        sourceSnapshot.SnapshotID,
	}

	// Clone the volume
	clone, err := d.API.CreateVolume(ctx, createRequest)
	if err != nil {
		return err
	}

	// Wait for creation to complete so that the mount targets are available
	return d.waitForVolumeCreate(ctx, clone, name)
}

func (d *NFSStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "NFSStorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Import")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Import")
	}

	// Get the volume
	creationToken := originalName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Get the volume size
	volConfig.Size = strconv.FormatInt(volume.QuotaInBytes, 10)

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		labels := d.updateTelemetryLabels(ctx, volume)
		labels = storage.DeleteProvisioningLabels(labels)

		if _, err := d.API.RelabelVolume(ctx, volume, labels); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, "+
				"relabel failed: %v", err)
			return fmt.Errorf("could not import volume %s, relabel failed: %v", originalName, err)
		}
		_, err = d.API.WaitForVolumeState(ctx, volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
		if err != nil {
			return fmt.Errorf("could not import volume %s: %v", originalName, err)
		}
	}

	// The CVS creation token cannot be changed, so use it as the internal name
	volConfig.InternalName = creationToken

	return nil
}

func (d *NFSStorageDriver) Rename(ctx context.Context, name, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "NFSStorageDriver",
			"name":    name,
			"newName": newName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Rename")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Rename")
	}

	// Rename is only needed for the import workflow, and we aren't currently renaming the
	// CVS volume when importing, so do nothing here lest we set the volume name incorrectly
	// during an import failure cleanup.
	return nil
}

// getTelemetryLabels builds the labels that are set on each volume.
func (d *NFSStorageDriver) getTelemetryLabels(ctx context.Context) string {

	telemetry := map[string]Telemetry{drivers.TridentLabelTag: *d.getTelemetry()}

	telemetryJSON, err := json.Marshal(telemetry)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal telemetry: %+v", telemetry)
	}

	return strings.ReplaceAll(string(telemetryJSON), " ", "")
}

func (d *NFSStorageDriver) isTelemetryLabel(label string) bool {

	var telemetry map[string]Telemetry
	err := json.Unmarshal([]byte(label), &telemetry)
	if err != nil {
		return false
	}
	if _, ok := telemetry[drivers.TridentLabelTag]; !ok {
		return false
	}
	return true
}

// updateTelemetryLabels updates the labels that are set on each volume.
func (d *NFSStorageDriver) updateTelemetryLabels(ctx context.Context, volume *api.FileSystem) []string {

	newLabels := []string{d.getTelemetryLabels(ctx)}

	for _, label := range volume.Labels {
		if !d.isTelemetryLabel(label) {
			newLabels = append(newLabels, label)
		}
	}

	return newLabels
}

// waitForVolumeCreate waits for volume creation to complete by reaching the Available state.  If the
// volume reaches a terminal state (Error), the volume is deleted.  If the wait times out and the volume
// is still creating, a VolumeCreatingError is returned so the caller may try again.
// This method is used by both Create & CreateClone.
func (d *NFSStorageDriver) waitForVolumeCreate(ctx context.Context, volume *api.FileSystem, volumeName string) error {

	state, err := d.API.WaitForVolumeState(
		ctx, volume, api.StateAvailable, []string{api.StateError}, d.volumeCreateTimeout)
	if err != nil {

		if state == api.StateCreating {
			Logc(ctx).WithFields(log.Fields{
				"volume": volumeName,
			}).Debug("Volume is in creating state.")
			return utils.VolumeCreatingError(err.Error())
		}

		// Don't leave a CVS volume in a non-transitional state laying around in error state
		if !api.IsTransitionalState(state) {
			errDelete := d.API.DeleteVolume(ctx, volume)
			if errDelete != nil {
				Logc(ctx).WithFields(log.Fields{
					"volume": volumeName,
				}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDelete)
			} else {
				// Wait for deletion to complete
				_, errDeleteWait := d.API.WaitForVolumeState(
					ctx, volume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
				if errDeleteWait != nil {
					Logc(ctx).WithFields(log.Fields{
						"volume": volumeName,
					}).Warnf("Volume could not be cleaned up and must be manually deleted: %v.", errDeleteWait)
				}
			}
		}

		return err
	}

	return nil
}

// Destroy deletes a volume.
func (d *NFSStorageDriver) Destroy(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NFSStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	// If volume doesn't exist, return success
	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(ctx, name)
	if err != nil {
		return err
	}
	if !volumeExists {
		Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		return nil
	} else if extantVolume.LifeCycleState == api.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = d.API.WaitForVolumeState(
			ctx, extantVolume, api.StateDeleted, []string{api.StateError}, d.volumeCreateTimeout)
		return err
	}

	// Get the volume
	volume, err := d.API.GetVolumeByCreationToken(ctx, name)
	if err != nil {
		return err
	}

	// Delete the volume
	if err = d.API.DeleteVolume(ctx, volume); err != nil {
		return err
	}

	// Wait for deletion to complete
	_, err = d.API.WaitForVolumeState(ctx, volume, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
	return err
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NFSStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "NFSStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Publish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Publish")
	}

	// Get the volume
	creationToken := name

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	mountTargets, err := d.API.GetMountTargetsForVolume(ctx, volume)
	if err != nil {
		return fmt.Errorf("could not read mount targets for volume %s: %v", name, err)
	}
	if len(*mountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", name)
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = "/" + volume.CreationToken
	publishInfo.NfsServerIP = (*mountTargets)[0].IPAddress
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NFSStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NFSStorageDriver) GetSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) (
	*storage.Snapshot, error,
) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	// Get the volume
	creationToken := internalVolName

	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(ctx, creationToken)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume %s: %v", creationToken, err)
	}
	if !volumeExists {
		// The AWS volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil, nil
	}

	snapshots, err := d.API.GetSnapshotsForVolume(ctx, extantVolume)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range *snapshots {
		if snapshot.Name == internalSnapName {

			created := snapshot.Created.UTC().Format(storage.SnapshotTimestampFormat)

			Logc(ctx).WithFields(log.Fields{
				"snapshotName": internalSnapName,
				"volumeName":   internalVolName,
				"created":      created,
			}).Debug("Found snapshot.")

			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   created,
				SizeBytes: extantVolume.QuotaInBytes,
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Warning("Snapshot not found.")

	return nil, nil
}

// Return the list of snapshots associated with the specified volume
func (d *NFSStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {

	internalVolName := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NFSStorageDriver",
			"volumeName": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	// Get the volume
	creationToken := internalVolName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return nil, fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshots, err := d.API.GetSnapshotsForVolume(ctx, volume)
	if err != nil {
		return nil, err
	}

	snapshotList := make([]*storage.Snapshot, 0)

	for _, snapshot := range *snapshots {

		// Filter out snapshots in an unavailable state
		if snapshot.LifeCycleState != api.StateAvailable {
			continue
		}

		snapshotList = append(snapshotList, &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               snapshot.Name,
				InternalName:       snapshot.Name,
				VolumeName:         volConfig.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			Created:   snapshot.Created.Format(storage.SnapshotTimestampFormat),
			SizeBytes: volume.QuotaInBytes,
			State:     storage.SnapshotStateOnline,
		})
	}

	return snapshotList, nil
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NFSStorageDriver) CreateSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) (
	*storage.Snapshot, error,
) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	// Check if volume exists
	volumeExists, sourceVolume, err := d.API.VolumeExistsByCreationToken(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volumeExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	// Get the source volume by ID to get all details
	sourceVolume, err = d.API.GetVolumeByID(ctx, sourceVolume.FileSystemID)
	if err != nil {
		return nil, fmt.Errorf("could not get source volume details: %v", err)
	}

	// Construct a snapshot request
	snapshotCreateRequest := &api.SnapshotCreateRequest{
		FileSystemID: sourceVolume.FileSystemID,
		Name:         internalSnapName,
		Region:       sourceVolume.Region,
	}

	snapshot, err := d.API.CreateSnapshot(ctx, snapshotCreateRequest)
	if err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// Wait for snapshot creation to complete
	err = d.API.WaitForSnapshotState(ctx, snapshot, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return nil, err
	}

	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snapshot.Created.Format(storage.SnapshotTimestampFormat),
		SizeBytes: sourceVolume.QuotaInBytes,
		State:     storage.SnapshotStateOnline,
	}, nil
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NFSStorageDriver) RestoreSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	// Get the volume
	creationToken := internalVolName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	snapshot, err := d.API.GetSnapshotForVolume(ctx, volume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	return d.API.RestoreSnapshot(ctx, volume, snapshot)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NFSStorageDriver) DeleteSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NFSStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	// Get the volume
	creationToken := internalVolName

	volumeExists, extantVolume, err := d.API.VolumeExistsByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("error checking for existing volume %s: %v", creationToken, err)
	}
	if !volumeExists {
		// The AWS volume is backed by ONTAP, so if the volume doesn't exist, neither does the snapshot.
		return nil
	}

	snapshot, err := d.API.GetSnapshotForVolume(ctx, extantVolume, internalSnapName)
	if err != nil {
		return fmt.Errorf("unable to find snapshot %s: %v", internalSnapName, err)
	}

	if err := d.API.DeleteSnapshot(ctx, extantVolume, snapshot); err != nil {
		return err
	}

	// Wait for snapshot deletion to complete
	return d.API.WaitForSnapshotState(ctx, snapshot, api.StateDeleted, []string{api.StateError}, d.defaultTimeout())
}

// Return the list of volumes associated with this tenant
func (d *NFSStorageDriver) List(ctx context.Context) ([]string, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> List")
		defer Logc(ctx).WithFields(fields).Debug("<<<< List")
	}

	volumes, err := d.API.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}

	prefix := *d.Config.StoragePrefix
	var volumeNames []string

	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.LifeCycleState {
		case api.StateDisabled, api.StateDeleting, api.StateDeleted, api.StateError:
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.CreationToken, prefix) {
			continue
		}

		volumeName := volume.CreationToken[len(prefix):]
		volumeNames = append(volumeNames, volumeName)
	}
	return volumeNames, nil
}

// Test for the existence of a volume
func (d *NFSStorageDriver) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NFSStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	_, err := d.API.GetVolumeByCreationToken(ctx, name)

	return err
}

// Resize increases a volume's quota
func (d *NFSStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, sizeBytes uint64) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Resize",
			"Type":      "NFSStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	// Get the volume
	creationToken := name

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// If the volume state isn't Available, return an error
	if volume.LifeCycleState != api.StateAvailable {
		return fmt.Errorf("volume %s state is %s, not available", creationToken, volume.LifeCycleState)
	}

	volConfig.Size = strconv.FormatUint(uint64(volume.QuotaInBytes), 10)

	// If the volume is already the requested size, there's nothing to do
	if int64(sizeBytes) == volume.QuotaInBytes {
		return nil
	}

	// Make sure we're not shrinking the volume
	if int64(sizeBytes) < volume.QuotaInBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, volume.QuotaInBytes)
	}

	// Make sure the request isn't above the configured maximum volume size (if any)
	if _, _, err = drivers.CheckVolumeSizeLimits(ctx, sizeBytes, d.Config.CommonStorageDriverConfig); err != nil {
		return err
	}

	// Resize the volume
	_, err = d.API.ResizeVolume(ctx, volume, int64(sizeBytes))
	if err != nil {
		return err
	}

	// Wait for resize operation to complete
	_, err = d.API.WaitForVolumeState(ctx, volume, api.StateAvailable, []string{api.StateError}, d.defaultTimeout())
	if err != nil {
		return fmt.Errorf("could not resize volume %s: %v", name, err)
	}

	volConfig.Size = strconv.FormatUint(sizeBytes, 10)
	return nil
}

// Retrieve storage capabilities and register pools with specified backend.
func (d *NFSStorageDriver) GetStorageBackendSpecs(_ context.Context, backend *storage.Backend) error {

	backend.Name = d.BackendName()

	for _, pool := range d.pools {
		pool.Backend = backend
		backend.AddStoragePool(pool)
	}

	return nil
}

func (d *NFSStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

// Retrieve storage backend physical pools
func (d *NFSStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return []string{}
}

func (d *NFSStorageDriver) GetInternalVolumeName(ctx context.Context, name string) string {

	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else if d.csiRegexp.MatchString(name) {
		// If the name is from CSI (i.e. contains a UUID), just use it as-is
		Logc(ctx).WithField("volumeInternal", name).Debug("Using volume name as internal name.")
		return name
	} else {
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = strings.Replace(internal, ".", "-", -1) // CVS disallows periods
		Logc(ctx).WithField("volumeInternal", internal).Debug("Modified volume name for internal name.")
		return internal
	}
}

func (d *NFSStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "CreateFollowup",
			"Type":   "NFSStorageDriver",
			"name":   volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateFollowup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateFollowup")
	}

	// Get the volume
	creationToken := volConfig.InternalName

	volume, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return fmt.Errorf("could not find volume %s: %v", creationToken, err)
	}

	// Ensure volume is in a good state
	if volume.LifeCycleState == api.StateError {
		return fmt.Errorf("volume %s is in %s state: %s", creationToken, api.StateError, volume.LifeCycleStateDetails)
	}

	mountTargets, err := d.API.GetMountTargetsForVolume(ctx, volume)
	if err != nil {
		return fmt.Errorf("could not read mount targets for volume %s: %v", volConfig.InternalName, err)
	}
	if len(*mountTargets) == 0 {
		return fmt.Errorf("volume %s has no mount targets", volConfig.InternalName)
	}

	// Just use the first mount target found
	volConfig.AccessInfo.NfsServerIP = (*mountTargets)[0].IPAddress
	volConfig.AccessInfo.NfsPath = "/" + volume.CreationToken
	volConfig.FileSystem = ""

	return nil
}

func (d *NFSStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NFSStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.AWSConfig = &d.Config
}

func (d *NFSStorageDriver) GetExternalConfig(ctx context.Context) interface{} {

	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.AWSNFSStorageDriverConfig
	drivers.Clone(ctx, d.Config, &cloneConfig)
	cloneConfig.APIKey = "<REDACTED>"    // redact the API key
	cloneConfig.SecretKey = "<REDACTED>" // redact the Secret key
	return cloneConfig
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NFSStorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {

	creationToken := name
	volumeAttrs, err := d.API.GetVolumeByCreationToken(ctx, creationToken)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volumeAttrs), nil
}

// Implement stringer interface for the NFSStorageDriver driver
func (d NFSStorageDriver) String() string {
	return drivers.ToString(&d, []string{"API"}, d.GetExternalConfig(context.Background()))
}

// Implement GoStringer interface for the NFSStorageDriver driver
func (d NFSStorageDriver) GoString() string {
	return d.String()
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NFSStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.API.GetVolumes(ctx)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	prefix := *d.Config.StoragePrefix

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range *volumes {

		// Filter out volumes in an unavailable state
		switch volume.LifeCycleState {
		case api.StateDisabled, api.StateDeleting, api.StateDeleted, api.StateError:
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.CreationToken, prefix) {
			continue
		}

		channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&volume), Error: nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NFSStorageDriver) getVolumeExternal(volumeAttrs *api.FileSystem) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            volumeAttrs.Name,
		InternalName:    volumeAttrs.CreationToken,
		Size:            strconv.FormatInt(volumeAttrs.QuotaInBytes, 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     "true",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
		ServiceLevel:    volumeAttrs.ServiceLevel,
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   drivers.UnsetPool,
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NFSStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NFSStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if !reflect.DeepEqual(d.Config.StoragePrefix, dOrig.Config.StoragePrefix) {
		bitmap.Add(storage.PrefixChange)
	}

	return bitmap
}

func (d *NFSStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, _ string) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NFSStorageDriver",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	return nil
}

func validateStoragePrefix(storagePrefix string) error {
	matched, err := regexp.MatchString(`[^a-zA-Z-_]+`, storagePrefix)
	if err != nil {
		return fmt.Errorf("could not check storage prefix; %v", err)
	} else if matched {
		return fmt.Errorf("storage prefix may only contain letters, hyphens and underscores")
	}
	return nil
}
